// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Main collector for all PostgreSQL-related metrics (sessions, locks, throughput).
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

	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PgTimescaleCollector struct {
	pgRepo   *repository.PgRepository
	tsLogger *hot.TimescaleLogger
}

func NewPgTimescaleCollector(pgRepo *repository.PgRepository, tsLogger *hot.TimescaleLogger) *PgTimescaleCollector {
	return &PgTimescaleCollector{
		pgRepo:   pgRepo,
		tsLogger: tsLogger,
	}
}

func (c *PgTimescaleCollector) Collect(ctx context.Context, inst config.Instance, db *sql.DB) error {
	instanceName := inst.Name
	serverID := inst.ServerID

	// 1. Locks
	rows, err := c.pgRepo.FetchDetailedLocks(ctx, instanceName, db)
	if err == nil {
		var hotRows []hot.PostgresLockRow
		for _, r := range rows {
			hotRows = append(hotRows, hot.PostgresLockRow{
				CollectedAt:    r.CollectedAt,
				ServerID:       serverID,
				PID:            r.PID,
				LockType:       r.LockType,
				Mode:           r.Mode,
				Granted:        r.Granted,
				RelationOID:    r.RelationOID,
				RelationName:   r.RelationName,
				TransactionID:  r.TransactionID,
				WaitingSeconds: r.WaitDurationMs / 1000.0,
			})
		}
		if err := c.tsLogger.LogPGTimescaleLock(ctx, serverID, hotRows); err != nil {
			slog.Error("[PgTimescaleCollector] ERROR: LogPGTimescaleLock failed", "target", instanceName, "err", err)
		}
	}

	// 2. Table & Index Usage (for SIH Dashboard)
	// Note: pgss_delta_1m writes are handled exclusively by StartPostgresQueryStatsCollector
	// via ComputeAndStorePgssDelta1m, which correctly matches the table schema.
	tableUsage, err := collectors.CollectPostgresTableUsageAndSize(ctx, db)
	if err == nil {
		_, _ = collectors.PersistPostgresTableUsageDeltas(ctx, c.tsLogger, serverID, tableUsage, time.Now())
		_, _ = collectors.PersistPostgresTableSizeHistory(ctx, c.tsLogger, serverID, tableUsage, time.Now())
	}

	indexUsage, err := collectors.CollectPostgresIndexUsage(ctx, db)
	if err == nil {
		_, _ = collectors.PersistPostgresIndexUsageDeltas(ctx, c.tsLogger, serverID, indexUsage, time.Now())
	}

	return nil
}
