// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Legacy collector for pg_stat_statements query metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package postgres

import (
	"log/slog"
	"context"
	"database/sql"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
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

func (c *PgStatStatementsLegacyCollector) Collect(ctx context.Context, inst config.Instance, db *sql.DB) error {
	instanceName := inst.Name
	serverID := inst.ServerID

	// 1. Fetch query snapshot from instance
	rows, err := c.pgRepo.FetchQuerySnapshot(ctx, instanceName, db)
	if err != nil {
		slog.Error("[PgStatStatementsLegacyCollector] ERROR: Failed to fetch query snapshot", "target", instanceName, "err", err)
		return err
	}

	if len(rows) == 0 {
		return nil
	}

	// 2. Map and compute deltas
	now := time.Now().UTC()
	var snapshots []hot.PostgresQueryStatsSnapRow
	var deltas []hot.PostgresQueryStatsSnapRow

	for _, r := range rows {
		// Calculate deltas using in-memory state in PgRepository
		// Keyed by (dbid, userid, queryid) to prevent collisions across DBs/users.
		prev, ok := c.pgRepo.GetPreviousSnapshot(instanceName, r.DbID, r.UserID, r.QueryID)
		c.pgRepo.UpdatePreviousSnapshot(instanceName, r)

		// Map repository.PgQueryStat to hot.PostgresQueryStatsSnapRow (Snapshot)
		curr := hot.PostgresQueryStatsSnapRow{
			QueryID:           r.QueryID,
			QueryText:         r.Query,
			DbName:            r.DbName,
			UserName:          r.UserName,
			AppName:           r.AppName,
			QueryType:         r.QueryType,
			Calls:             r.Calls,
			TotalTimeMs:       r.TotalTime,
			MeanTimeMs:        r.MeanTime,
			Rows:              r.Rows,
			TempBlksRead:      r.TempBlksRead,
			TempBlksWritten:   r.TempBlksWritten,
			BlkReadTimeMs:     r.BlkReadTime,
			BlkWriteTimeMs:    r.BlkWriteTime,
			SharedBlksHit:     r.SharedBlksHit,
			SharedBlksRead:    r.SharedBlksRead,
			SharedBlksDirtied: r.SharedBlksDirtied,
			SharedBlksWritten: r.SharedBlksWritten,
			WalBytes:          int64(r.WalBytes),
			WalRecords:        r.WalRecords,
			WalFpi:            r.WalFpi,
			TotalPlanTime:     r.TotalPlanTime,
			MeanPlanTime:      r.MeanPlanTime,
			Plans:             r.Plans,
		}

		if ok {
			d, changed := repository.ComputeDelta(r, prev)
			if !changed {
				continue // Skip if no meaningful change (no new calls)
			}
			// Map delta to hot row
			deltas = append(deltas, hot.PostgresQueryStatsSnapRow{
				QueryID:           d.QueryID,
				DbName:            d.DbName,
				UserName:          d.UserName,
				AppName:           r.AppName,
				QueryType:         d.QueryType,
				Calls:             d.Calls,
				TotalTimeMs:       d.TotalTime,
				MeanTimeMs:        d.MeanTime,
				Rows:              d.Rows,
				TempBlksRead:      d.TempBlksRead,
				TempBlksWritten:   d.TempBlksWritten,
				BlkReadTimeMs:     d.BlkReadTime,
				BlkWriteTimeMs:    d.BlkWriteTime,
				SharedBlksHit:     d.SharedBlksHit,
				SharedBlksRead:    d.SharedBlksRead,
				SharedBlksDirtied: d.SharedBlksDirtied,
				SharedBlksWritten: d.SharedBlksWritten,
				WalBytes:          int64(d.WalBytes),
				WalRecords:        d.WalRecords,
				WalFpi:            d.WalFpi,
				TotalPlanTime:     d.TotalPlanTime,
				MeanPlanTime:      d.MeanPlanTime,
				Plans:             d.Plans,
			})
		}

		// Always store the raw cumulative snapshot if it changed (or first time seen)
		snapshots = append(snapshots, curr)
	}

	// 3. Store CHANGED snapshots in TimescaleDB (postgres_query_stats)
	if len(snapshots) > 0 {
		if err := c.tsLogger.LogPostgresQueryStats(ctx, serverID, snapshots); err != nil {
			slog.Error("[PgStatStatementsLegacyCollector] ERROR: Failed to log query stats", "target", instanceName, "err", err)
			return err
		}
	}

	// 4. Update query dimension
	if len(snapshots) > 0 {
		if err := c.tsLogger.UpsertPgssQueryDim(ctx, serverID, snapshots); err != nil {
			slog.Error("[PgStatStatementsLegacyCollector] ERROR: Failed to update query dim", "target", instanceName, "err", err)
		}
	}

	// 5. Store deltas directly in pgss_delta_1m
	if len(deltas) > 0 {
		if err := c.tsLogger.LogPgssDelta1m(ctx, serverID, now, deltas); err != nil {
			slog.Error("[PgStatStatementsLegacyCollector] ERROR: Failed to log 1m deltas", "target", instanceName, "err", err)
		}
	}

	return nil
}

