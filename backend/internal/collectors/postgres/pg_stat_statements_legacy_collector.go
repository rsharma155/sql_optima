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
	var deltas []hot.PostgresQueryStatsSnapRow
	for _, r := range rows {
		// Map repository.PgQueryStat to hot.PostgresQueryStatsSnapRow
		curr := hot.PostgresQueryStatsSnapRow{
			QueryID:           r.QueryID,
			QueryText:         r.Query,
			DbName:            r.DbName,
			UserName:          r.UserName,
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

		// Calculate deltas using in-memory state in PgRepository
		_, _ = c.pgRepo.GetPreviousSnapshot(instanceName, r.QueryID)
		c.pgRepo.UpdatePreviousSnapshot(instanceName, r.QueryID, r)
		deltas = append(deltas, curr)
	}

	// 3. Store snapshots in TimescaleDB (postgres_query_stats)
	if err := c.tsLogger.LogPostgresQueryStats(ctx, serverID, deltas); err != nil {
		slog.Error("[PgStatStatementsLegacyCollector] ERROR: Failed to log query stats", "target", instanceName, "err", err)
		return err
	}

	// 4. Update query dimension
	if err := c.tsLogger.UpsertPgssQueryDim(ctx, serverID, deltas); err != nil {
		slog.Error("[PgStatStatementsLegacyCollector] ERROR: Failed to update query dim", "target", instanceName, "err", err)
	}

	// 5. Compute and store 1m deltas (mimicking pg_stat_monitor behavior)
	if err := c.tsLogger.ComputeAndStorePgssDelta1m(ctx, serverID, now); err != nil {
		slog.Error("[PgStatStatementsLegacyCollector] ERROR: Failed to compute 1m deltas", "target", instanceName, "err", err)
	}

	return nil
}
