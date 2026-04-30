// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Collector for PostgreSQL instance snapshots and trend metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/domain/postgres_monitoring/instance_health"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PgSnapshotCollector struct {
	pgRepo        *repository.PgRepository
	healthService *instance_health.InstanceHealthService
	healthRepo    *instance_health.InstanceHealthRepository
	tsLogger      *hot.TimescaleLogger
}

func NewPgSnapshotCollector(pgRepo *repository.PgRepository, healthService *instance_health.InstanceHealthService, healthRepo *instance_health.InstanceHealthRepository, tsLogger *hot.TimescaleLogger) *PgSnapshotCollector {
	return &PgSnapshotCollector{
		pgRepo:        pgRepo,
		healthService: healthService,
		healthRepo:    healthRepo,
		tsLogger:      tsLogger,
	}
}

func (c *PgSnapshotCollector) Collect(ctx context.Context, instanceName string) error {
	// 1. Fetch metrics from PostgreSQL
	// TPS
	tps := 0.0
	xactTotal, err := c.pgRepo.FetchXactTotal(instanceName)
	if err == nil && c.tsLogger != nil {
		interval := 15.0 // default
		if r, ok := c.tsLogger.ComputePgTps(instanceName, xactTotal, interval); ok {
			tps = r
		}
	}

	// Sessions
	active, waiting, _ := c.pgRepo.FetchActiveWaitingSessions(instanceName)
	
	// Fetch detailed session counts
	cnt, _ := c.pgRepo.GetSessionStateCounts(instanceName)
	idle := 0
	idleInTx := 0
	if cnt != nil {
		idle = cnt.Idle
		idleInTx = cnt.IdleInTxn
	}

	// CPU & Memory
	cpuUsage := 0.0
	sharedBuffersPct := 0.0
	cacheHitRatio, _ := c.pgRepo.FetchCacheHitRatioPct(instanceName)

	// Estimate CPU usage based on active connections if no OS collector is present
	// (Simple approximation: 10% per active session, capped at 100%)
	if active > 0 {
		cpuUsage = float64(active) * 10.0
		if cpuUsage > 100.0 {
			cpuUsage = 100.0
		}
	}
	
	// WAL MB/min
	walRate := 0.0
	walBytes, err := c.pgRepo.FetchWalBytesTotal(instanceName)
	if err == nil && c.tsLogger != nil {
		interval := 15.0 // default
		if r, ok := c.tsLogger.ComputeWalRateMBPerMin(instanceName, walBytes, interval); ok {
			walRate = r
		}
	}
	
	// XID & Storage
	obs, _ := c.pgRepo.FetchDBObservationMetrics(instanceName)
	maxXidAge := int64(0)
	if obs != nil {
		maxXidAge = obs.XIDAge
	}
	
	deadTuplePct, _ := c.pgRepo.FetchDeadTupleRatioPct(instanceName)

	sizeStats := c.pgRepo.GetDatabaseSizeStats(instanceName)
	databaseSizeGB := float64(sizeStats.TotalBytes) / 1024 / 1024 / 1024

	tempBytesMB := 0.0
	dbStats, err := c.pgRepo.GetDbIOStats(instanceName)
	if err == nil {
		for _, db := range dbStats {
			tempBytesMB += float64(db.TempBytes) / 1024 / 1024
		}
	}

	// Vacuum
	autovacWorkers, _ := c.pgRepo.FetchAutovacuumWorkers(instanceName)

	// Checkpoints
	cp, _ := c.pgRepo.FetchBGWriterStats(instanceName)
	cpTimed := int64(0)
	cpReq := int64(0)
	cpWriteTime := 0.0
	cpRatio := 0.0
	if cp != nil {
		cpTimed = cp.CheckpointsTimed
		cpReq = cp.CheckpointsReq
		cpWriteTime = cp.CheckpointWriteTime
		total := float64(cpTimed + cpReq)
		if total > 0 {
			cpRatio = float64(cpReq) / total
		}
	}

	// Replication
	replicaLagSec, _ := c.pgRepo.FetchReplicaLagSec(instanceName)

	// Server Info (Version & Uptime)
	version, uptime, _ := c.pgRepo.GetServerInfo(instanceName)

	// 2. Build Snapshot
	snapshot := &instance_health.PgInstanceSnapshot{
		InstanceID:           instanceName,
		CollectedAt:          time.Now().UTC(),
		TPS:                  tps,
		ActiveSessions:       active,
		IdleSessions:         idle,
		IdleInTxSessions:     idleInTx,
		BlockedSessions:      waiting,
		CPUUsage:             cpuUsage,
		SharedBuffersUsedPct: sharedBuffersPct,
		CacheHitRatio:        cacheHitRatio,
		WALMBPerMin:          walRate,
		CheckpointReqRatio:   cpRatio,
		CheckpointsTimed:     int(cpTimed),
		CheckpointsReq:       int(cpReq),
		CheckpointWriteTime:  cpWriteTime,
		MaxXIDAge:            maxXidAge,
		DatabaseSizeGB:     databaseSizeGB,
		TempBytesMB:        tempBytesMB,
		AutovacuumWorkers:  autovacWorkers,
		DeadTuplePct:       deadTuplePct,
		ReplicaLagSec:      replicaLagSec,
		Version:            version,
		Uptime:             uptime,
	}

	// 3. Calculate Health Score
	snapshot.HealthScore = c.healthService.CalculateHealthScore(snapshot)

	// 4. Persist to Snapshot Table
	if err := c.healthRepo.UpsertSnapshot(ctx, snapshot); err != nil {
		log.Printf("[PgSnapshotCollector] UpsertSnapshot error for %s: %v", instanceName, err)
		return err
	}

	// 5. Persist to Timeseries Hypertables
	_ = c.healthRepo.LogMetric(ctx, instanceName, "tps", snapshot.TPS)
	_ = c.healthRepo.LogMetric(ctx, instanceName, "tps_total", snapshot.TPS)
	_ = c.healthRepo.LogMetric(ctx, instanceName, "tps_read", snapshot.TPS*0.7) // Approximation
	_ = c.healthRepo.LogMetric(ctx, instanceName, "tps_write", snapshot.TPS*0.3) // Approximation
	_ = c.healthRepo.LogMetric(ctx, instanceName, "wal_mb_per_min", snapshot.WALMBPerMin)
	_ = c.healthRepo.LogMetric(ctx, instanceName, "dead_tuple_pct", snapshot.DeadTuplePct)
	_ = c.healthRepo.LogMetric(ctx, instanceName, "replica_lag_sec", snapshot.ReplicaLagSec)
	_ = c.healthRepo.LogMetric(ctx, instanceName, "cache_hit_ratio", snapshot.CacheHitRatio)
	_ = c.healthRepo.LogMetric(ctx, instanceName, "checkpoint_req_ratio", snapshot.CheckpointReqRatio)
	_ = c.healthRepo.LogMetric(ctx, instanceName, "health_score", float64(snapshot.HealthScore))
	_ = c.healthRepo.LogMetric(ctx, instanceName, "database_size_gb", snapshot.DatabaseSizeGB)
	_ = c.healthRepo.LogMetric(ctx, instanceName, "temp_bytes_mb", snapshot.TempBytesMB)

	// Additional metrics for Control Center graphs
	_ = c.healthRepo.LogMetric(ctx, instanceName, "active_sessions_ts", float64(snapshot.ActiveSessions))
	_ = c.healthRepo.LogMetric(ctx, instanceName, "idle_sessions_ts", float64(snapshot.IdleSessions))
	_ = c.healthRepo.LogMetric(ctx, instanceName, "cpu_load", float64(snapshot.ActiveSessions))
	_ = c.healthRepo.LogMetric(ctx, instanceName, "waiting_load", float64(snapshot.BlockedSessions))
	_ = c.healthRepo.LogMetric(ctx, instanceName, "idle_in_txn_load", float64(snapshot.IdleInTxSessions))

	return nil
}
