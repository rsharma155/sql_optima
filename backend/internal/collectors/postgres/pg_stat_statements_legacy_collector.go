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
	"log/slog"
	"time"

	"github.com/rsharma155/sql_optima/internal/reliability"
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

	stats, err := c.pgRepo.GetQueryStatsForSnapshot(instanceName)
	if err != nil {
		slog.Warn("pgss_snapshot_skip", "instance", instanceName, "error", err)
		return err
	}
	if len(stats) == 0 {
		slog.Debug("pgss_snapshot_empty", "instance", instanceName, "hint", "pg_stat_statements may be empty or extension not installed")
		return nil
	}

	ts := time.Now().UTC()
	rows := make([]hot.PostgresQueryStatsSnapRow, 0, len(stats))
	for _, q := range stats {
		rows = append(rows, hot.PostgresQueryStatsSnapRow{
			QueryID:           q.QueryID,
			QueryText:         q.Query,
			Calls:             q.Calls,
			TotalTimeMs:       q.TotalTime,
			MeanTimeMs:        q.MeanTime,
			Rows:              q.Rows,
			TempBlksRead:      q.TempBlksRead,
			TempBlksWritten:   q.TempBlksWritten,
			BlkReadTimeMs:     q.BlkReadTime,
			BlkWriteTimeMs:    q.BlkWriteTime,
			SharedBlksHit:     q.SharedBlksHit,
			SharedBlksRead:    q.SharedBlksRead,
			SharedBlksDirtied: q.SharedBlksDirtied,
			SharedBlksWritten: q.SharedBlksWritten,
			WalBytes:          q.WalBytes,
			WalRecords:        q.WalRecords,
			WalFpi:            q.WalFpi,
			TotalPlanTime:     q.TotalPlanTime,
			MeanPlanTime:      q.MeanPlanTime,
			Plans:             q.Plans,
		})
	}

	retryCfg := reliability.RetryConfig{
		MaxRetries:      3,
		InitialInterval: 1 * time.Second,
		MaxElapsed:      15 * time.Second,
	}

	// Upsert query text dimension table
	if err := reliability.Do(ctx, retryCfg, "pgss_query_dim_upsert", func() error {
		return c.tsLogger.UpsertPgssQueryDim(ctx, instanceName, rows)
	}); err != nil {
		slog.Error("pgss_query_dim_upsert_error", "instance", instanceName, "error", err)
	}

	if err := c.tsLogger.LogPostgresQueryStatsSnapshot(ctx, instanceName, ts, rows); err != nil {
		slog.Error("pgss_snapshot_error", "instance", instanceName, "error", err)
		return err
	}

	// Compute and store per-minute deltas for the dashboard
	if err := reliability.Do(ctx, retryCfg, "pgss_delta_1m_compute", func() error {
		return c.tsLogger.ComputeAndStorePgssDelta1m(ctx, instanceName, ts)
	}); err != nil {
		slog.Error("pgss_delta_1m_error", "instance", instanceName, "error", err)
	}

	return nil
}
