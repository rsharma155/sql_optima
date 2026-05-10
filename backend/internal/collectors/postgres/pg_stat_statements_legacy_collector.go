// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Legacy collector for pg_stat_statements data.
//
// Metadata:
//   Type: Collector
//   Package: postgres
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package postgres

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"time"

	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PgStatStatementsLegacyCollector struct {
	pgRepo   *repository.PgRepository
	tsLogger *hot.TimescaleLogger
}

func NewPgStatStatementsLegacyCollector(pgRepo *repository.PgRepository, tsLogger *hot.TimescaleLogger) *PgStatStatementsLegacyCollector {
	return &PgStatStatementsLegacyCollector{
		pgRepo:   pgRepo,
		tsLogger: tsLogger,
	}
}

func (c *PgStatStatementsLegacyCollector) Collect(ctx context.Context, instanceName string, db *sql.DB) error {
	if c.tsLogger == nil {
		return nil
	}

	t0 := time.Now()
	// 1. Fetch raw snapshot
	stats, err := c.pgRepo.FetchQuerySnapshot(instanceName)
	if err != nil {
		slog.Warn("pgss_snapshot_skip", "instance", instanceName, "error", err)
		return err
	}
	if len(stats) == 0 {
		return nil
	}

	// 2. Compute deltas
	var deltaRows []hot.PGQueryMetricsDelta
	ts := time.Now().UTC()

	fetchedCount := len(stats)
	insertedCount := 0
	skippedCount := 0
	resetDetected := false

	for _, current := range stats {
		previous, found := c.pgRepo.GetPreviousSnapshot(instanceName, current.QueryID)
		if !found {
			// If queryID not seen before: Store baseline only
			c.pgRepo.UpdatePreviousSnapshot(instanceName, current.QueryID, current)
			skippedCount++
			continue
		}

		// Compute delta
		delta, changed := repository.ComputeDelta(current, previous)
		
		if !changed {
			// If calls < previous.Calls, ComputeDelta returns changed=false
			// This covers the "Detect pg_stat_statements Reset" requirement
			if current.Calls < previous.Calls || current.TotalTime < previous.TotalTime {
				resetDetected = true
				c.pgRepo.UpdatePreviousSnapshot(instanceName, current.QueryID, current)
			}
			skippedCount++
			continue
		}

		// Delta exists: Prepare for insertion
		deltaRows = append(deltaRows, hot.PGQueryMetricsDelta{
			SampleTime:       ts,
			InstanceName:     instanceName,
			QueryID:          delta.QueryID,
			DbName:           current.DbName,
			UserName:         current.UserName,
			AppName:          "",
			QueryType:        repository.ClassifyQueryType(current.Query),
			DeltaCalls:       delta.Calls,
			DeltaExecMs:      delta.TotalTime,
			DeltaRows:        delta.Rows,
			DeltaSharedReads: delta.SharedBlksRead,
			DeltaSharedHits:  delta.SharedBlksHit,
			DeltaTempWritten: delta.TempBlksWritten,
			DeltaWalBytes:    delta.WalBytes,
			DeltaPlanTime:    delta.TotalPlanTime,
			MeanExecMs:       delta.MeanTime,
		})
		
		// Update baseline
		c.pgRepo.UpdatePreviousSnapshot(instanceName, current.QueryID, current)
		insertedCount++
	}

	if resetDetected {
		slog.Info("pg_stat_statements_reset_detected", "instance", instanceName)
	}

	// 3. Insert delta rows
	if len(deltaRows) > 0 {
		if err := c.tsLogger.LogPGQueryMetricsDelta(ctx, deltaRows); err != nil {
			slog.Error("pgss_delta_insert_error", "instance", instanceName, "error", err)
		}
	}

	// 4. Log changed snapshots for Query Performance dashboard (Incremental Snapshots)
	var snapRows []hot.PostgresQueryStatsSnapRow
	for _, s := range stats {
		previous, found := c.pgRepo.GetPreviousSnapshot(instanceName, s.QueryID)
		
		// Only insert if it's the first time seeing this query OR if metrics have changed.
		// This prevents redundant "yesterday's data" from filling up the table.
		if !found || s.Calls > previous.Calls || s.TotalTime > previous.TotalTime {
			snapRows = append(snapRows, hot.PostgresQueryStatsSnapRow{
				QueryID:           s.QueryID,
				QueryText:         s.Query,
				DbName:            s.DbName,
				UserName:          s.UserName,
				QueryType:         repository.ClassifyQueryType(s.Query),
				Calls:             s.Calls,
				TotalTimeMs:       s.TotalTime,
				MeanTimeMs:        s.MeanTime,
				Rows:              s.Rows,
				TempBlksRead:      s.TempBlksRead,
				TempBlksWritten:   s.TempBlksWritten,
				BlkReadTimeMs:     s.BlkReadTime,
				BlkWriteTimeMs:    s.BlkWriteTime,
				SharedBlksHit:     s.SharedBlksHit,
				SharedBlksRead:    s.SharedBlksRead,
				SharedBlksDirtied: s.SharedBlksDirtied,
				SharedBlksWritten: s.SharedBlksWritten,
				WalBytes:          s.WalBytes,
				WalRecords:        s.WalRecords,
				WalFpi:            s.WalFpi,
				TotalPlanTime:     s.TotalPlanTime,
				MeanPlanTime:      s.MeanPlanTime,
				Plans:             s.Plans,
			})
		}
	}

	if len(snapRows) > 0 {
		if err := c.tsLogger.LogPostgresQueryStatsSnapshot(ctx, instanceName, ts, snapRows); err != nil {
			slog.Error("pg_query_stats_snap_error", "instance", instanceName, "error", err)
		}
	}

	// 5. Structured logging
	log.Printf("[COLLECTOR] instance=%s fetched=%d changed=%d inserted=%d skipped=%d duration=%v", 
		instanceName, fetchedCount, insertedCount, insertedCount, skippedCount, time.Since(t0))

	return nil
}
