// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unified collector for PostgreSQL Phase 8 migration to TimescaleDB.
// Prefix: pg_ (as requested)
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package postgres

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/domain/postgres_monitoring/instance_health"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PgTimescaleCollector struct {
	pgRepo        *repository.PgRepository
	healthService *instance_health.InstanceHealthService
	tsLogger      *hot.TimescaleLogger

	// State for delta calculation of pg_stat_statements
	prevStats map[string]map[int64]repository.PGStatStatementsInternal // instance -> queryid -> last seen stats
	mu        sync.Mutex
}

func NewPgTimescaleCollector(pgRepo *repository.PgRepository, healthService *instance_health.InstanceHealthService, tsLogger *hot.TimescaleLogger) *PgTimescaleCollector {
	return &PgTimescaleCollector{
		pgRepo:        pgRepo,
		healthService: healthService,
		tsLogger:      tsLogger,
		prevStats:     make(map[string]map[int64]repository.PGStatStatementsInternal),
	}
}

func (c *PgTimescaleCollector) Collect(ctx context.Context, instanceName string) error {
	ts := time.Now().UTC()
	slog.Info("pg_timescale_collect_start", "instance", instanceName)

	// 1. COLLECT & LOG DETAILED LOCKS
	locks, err := c.pgRepo.FetchDetailedLocks(instanceName)
	if err == nil && len(locks) > 0 {
		lockRows := make([]hot.PostgresLockRow, 0, len(locks))
		for _, l := range locks {
			lockRows = append(lockRows, hot.PostgresLockRow{
				PID:            l.PID,
				DatabaseName:   l.DatabaseName,
				WaitEventType:  l.WaitEventType,
				WaitEvent:      l.WaitEvent,
				LockType:       l.LockType,
				Mode:           l.Mode,
				Granted:        l.Granted,
				QueryText:      l.QueryText,
				BlockedBy:      l.BlockedBy,
				WaitDurationMs: l.WaitDurationMs,
			})
		}
		if err := c.tsLogger.LogPGTimescaleLock(ctx, instanceName, ts, lockRows); err != nil {
			slog.Warn("pg_ts_locks_log_error", "instance", instanceName, "error", err)
		}
	}

	// 2. COLLECT & LOG STAT STATEMENTS DELTAS
	c.collectStatStatementsDeltas(ctx, instanceName, ts)

	// 3. COLLECT & LOG ENGINE SNAPSHOT
	c.collectInstanceSnapshot(ctx, instanceName, ts)

	return nil
}

func (c *PgTimescaleCollector) collectStatStatementsDeltas(ctx context.Context, instanceName string, ts time.Time) {
	currStats, err := c.pgRepo.FetchStatStatements(instanceName)
	if err != nil {
		slog.Warn("pg_ts_stat_fetch_error", "instance", instanceName, "error", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	prev, ok := c.prevStats[instanceName]
	if !ok {
		// First run: just seed the state
		newPrev := make(map[int64]repository.PGStatStatementsInternal)
		for _, s := range currStats {
			newPrev[s.QueryID] = s
		}
		c.prevStats[instanceName] = newPrev
		return
	}

	deltaRows := make([]hot.PostgresStatStatementsDeltaRow, 0)
	for _, curr := range currStats {
		p, exists := prev[curr.QueryID]
		if !exists || curr.Calls < p.Calls {
			// New query or stats reset: skip this interval
			prev[curr.QueryID] = curr
			continue
		}

		deltaRows = append(deltaRows, hot.PostgresStatStatementsDeltaRow{
			QueryID:           curr.QueryID,
			DatabaseName:      curr.DatabaseName,
			UserName:          curr.UserName,
			Calls:             curr.Calls - p.Calls,
			TotalTimeMs:       curr.TotalTime - p.TotalTime,
			Rows:              curr.Rows - p.Rows,
			SharedBlksHit:     curr.SharedBlksHit - p.SharedBlksHit,
			SharedBlksRead:    curr.SharedBlksRead - p.SharedBlksRead,
			SharedBlksDirtied: curr.SharedBlksDirtied - p.SharedBlksDirtied,
			SharedBlksWritten: curr.SharedBlksWritten - p.SharedBlksWritten,
			TempBlksRead:      curr.TempBlksRead - p.TempBlksRead,
			TempBlksWritten:   curr.TempBlksWritten - p.TempBlksWritten,
			BlkReadTimeMs:     curr.BlkReadTime - p.BlkReadTime,
			BlkWriteTimeMs:    curr.BlkWriteTime - p.BlkWriteTime,
			WalBytes:          curr.WalBytes - p.WalBytes,
		})
		prev[curr.QueryID] = curr
	}

	if len(deltaRows) > 0 {
		if err := c.tsLogger.LogPGStatStatementsDelta(ctx, instanceName, ts, deltaRows); err != nil {
			slog.Warn("pg_ts_stat_delta_log_error", "instance", instanceName, "error", err)
		}
	}
}

func (c *PgTimescaleCollector) collectInstanceSnapshot(ctx context.Context, instanceName string, ts time.Time) {
	// Re-using logic from PgSnapshotCollector but logging to the new Phase 8 table
	active, waiting, _ := c.pgRepo.FetchActiveWaitingSessions(instanceName)
	tps := 0.0
	xactTotal, err := c.pgRepo.FetchXactTotal(instanceName)
	if err == nil {
		if r, ok := c.tsLogger.ComputePgTps(instanceName, xactTotal, 15.0); ok {
			tps = r
		}
	}

	cacheHit, _ := c.pgRepo.FetchCacheHitRatioPct(instanceName)
	replicaLag, _ := c.pgRepo.FetchReplicaLagSec(instanceName)
	walBytes, err := c.pgRepo.FetchWalBytesTotal(instanceName)
	walRate := 0.0
	if err == nil {
		if r, ok := c.tsLogger.ComputeWalRateMBPerMin(instanceName, walBytes, 15.0); ok {
			walRate = r / 60.0 // convert MB/min to MB/s for MBPS
		}
	}

	obs, _ := c.pgRepo.FetchDBObservationMetrics(instanceName)
	maxXid := int64(0)
	if obs != nil {
		maxXid = obs.XIDAge
	}

	cp, _ := c.pgRepo.FetchBGWriterStats(instanceName)
	cpRatio := 0.0
	if cp != nil {
		total := float64(cp.CheckpointsTimed + cp.CheckpointsReq)
		if total > 0 {
			cpRatio = float64(cp.CheckpointsReq) / total
		}
	}

	// Calculate a real-time health score using the existing service
	dummySnap := &instance_health.PgInstanceSnapshot{
		ActiveSessions:     active,
		BlockedSessions:    waiting,
		TPS:                tps,
		CacheHitRatio:      cacheHit,
		ReplicaLagSec:      replicaLag,
		MaxXIDAge:          maxXid,
		CheckpointReqRatio: cpRatio,
	}
	healthScore := c.healthService.CalculateHealthScore(dummySnap)

	row := hot.PostgresInstanceSnapshotRow{
		HealthScore:        healthScore,
		TotalConnections:   active + waiting, // Simple approx
		ActiveSessions:     active,
		WaitingSessions:    waiting,
		LongestActiveMs:    0, // Not easily available in one call
		TPS:                tps,
		CacheHitRatio:      cacheHit,
		ReplicaLagSec:      replicaLag,
		MaxXidAge:          maxXid,
		WalGenRateMbps:    walRate,
		CheckpointReqRatio: cpRatio,
	}

	if err := c.tsLogger.LogPGInstanceSnapshot(ctx, instanceName, ts, row); err != nil {
		slog.Warn("pg_ts_instance_snap_log_error", "instance", instanceName, "error", err)
	}
}
