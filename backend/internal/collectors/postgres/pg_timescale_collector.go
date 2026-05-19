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

	// 2. Query Stats (via pg_stat_statements)
	stats, err := c.pgRepo.FetchStatStatements(ctx, instanceName, db)
	if err == nil {
		now := time.Now().UTC()
		var deltas []hot.PostgresStatStatementsDeltaRow
		for _, s := range stats {
			prev, ok := c.pgRepo.GetPreviousSnapshot(instanceName, s.QueryID)
			if ok {
				if s.Calls > prev.Calls {
					deltas = append(deltas, hot.PostgresStatStatementsDeltaRow{
						CaptureTimestamp:       now,
						ServerID:               serverID,
						QueryID:                s.QueryID,
						DatabaseName:           s.DbName,
						UserName:               s.UserName,
						CallsDelta:             s.Calls - prev.Calls,
						TotalTimeDeltaMs:       s.TotalTime - prev.TotalTime,
						RowsDelta:              s.Rows - prev.Rows,
						SharedBlksHitDelta:     s.SharedBlksHit - prev.SharedBlksHit,
						SharedBlksReadDelta:    s.SharedBlksRead - prev.SharedBlksRead,
						SharedBlksDirtiedDelta: s.SharedBlksDirtied - prev.SharedBlksDirtied,
						SharedBlksWrittenDelta: s.SharedBlksWritten - prev.SharedBlksWritten,
						TempBlksReadDelta:      s.TempBlksRead - prev.TempBlksRead,
						TempBlksWrittenDelta:   s.TempBlksWritten - prev.TempBlksWritten,
						BlkReadTimeDeltaMs:     s.BlkReadTime - prev.BlkReadTime,
						BlkWriteTimeDeltaMs:    s.BlkWriteTime - prev.BlkWriteTime,
						WalBytesDelta:          int64(s.WalBytes) - prev.WalBytes,
					})
				}
			}
			// Map internal struct back to repository expected struct
			c.pgRepo.UpdatePreviousSnapshot(instanceName, s.QueryID, repository.PgQueryStat{
				QueryID:           s.QueryID,
				DbName:            s.DbName,
				UserName:          s.UserName,
				Query:             s.Query,
				QueryType:         s.QueryType,
				Calls:             s.Calls,
				TotalTime:         s.TotalTime,
				MeanTime:          s.MeanTime,
				Rows:              s.Rows,
				SharedBlksHit:     s.SharedBlksHit,
				SharedBlksRead:    s.SharedBlksRead,
				SharedBlksDirtied: s.SharedBlksDirtied,
				SharedBlksWritten: s.SharedBlksWritten,
				TempBlksRead:      s.TempBlksRead,
				TempBlksWritten:   s.TempBlksWritten,
				BlkReadTime:       s.BlkReadTime,
				BlkWriteTime:      s.BlkWriteTime,
				WalBytes:          int64(s.WalBytes),
				TotalPlanTime:     s.TotalPlanTime,
				MeanPlanTime:      s.MeanPlanTime,
				Plans:             s.Plans,
			})
		}
		if len(deltas) > 0 {
			if err := c.tsLogger.LogPostgresStatStatementsDelta(ctx, serverID, deltas); err != nil {
				slog.Error("[PgTimescaleCollector] ERROR: LogPostgresStatStatementsDelta failed", "target", instanceName, "err", err)
			}
		}
	}

	// 3. Table & Index Usage (for SIH Dashboard)
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
