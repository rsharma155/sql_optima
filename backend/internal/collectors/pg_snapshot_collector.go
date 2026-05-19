// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Collector for PostgreSQL instance snapshots and trend metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"log/slog"
	"context"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
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

func (c *PgSnapshotCollector) Collect(ctx context.Context, inst config.Instance) error {
	instanceName := inst.Name
	serverID := inst.ServerID

	// 1. Fetch metrics from PostgreSQL
	// TPS
	tps := 0.0
	xactTotal, err := c.pgRepo.FetchXactTotal(ctx, instanceName)
	if err == nil && c.tsLogger != nil {
		interval := 15.0 // default
		if r, ok := c.tsLogger.ComputePgTps(serverID, xactTotal, interval); ok {
			tps = r
		}
	}

	// Sessions
	active, waiting, _ := c.pgRepo.FetchActiveWaitingSessions(ctx, instanceName)
	idle, idleInTx, _ := c.pgRepo.FetchIdleSessions(ctx, instanceName)

	// CPU/Memory
	cpuUsage, _ := c.pgRepo.FetchCPUUsage(ctx, instanceName)
	_, _ = c.pgRepo.FetchMemoryUsage(instanceName)
	sharedBuffersPct, _ := c.pgRepo.FetchSharedBuffersUtilization(instanceName)

	// Cache Hit Ratio
	cacheHitRatio, _ := c.pgRepo.FetchCacheHitRatio(ctx, instanceName)

	// WAL Rate (real delta-based computation, replaces dummy FetchWALGenerationRate)
	walRate := 0.0
	walBytesTotal, walErr := c.pgRepo.FetchWalBytesTotal(ctx, instanceName)
	if walErr == nil && c.tsLogger != nil {
		if r, ok := c.tsLogger.ComputeWalRateMBPerMin(serverID, walBytesTotal, 15.0); ok {
			walRate = r
		}
	}

	// XID Age
	maxXidAge, _ := c.pgRepo.FetchMaxXidAge(ctx, instanceName)

	// Table Stats (real dead-tuple ratio, replaces dummy FetchDeadTuplePercentage)
	deadTuplePct, _ := c.pgRepo.FetchDeadTupleRatioPct(ctx, instanceName)

	// Storage Stats
	sizeStats := c.pgRepo.GetDatabaseSizeStats(ctx, instanceName)
	databaseSizeGB := float64(sizeStats.TotalBytes) / 1024 / 1024 / 1024

	tempBytesMB := 0.0
	dbStats, err := c.pgRepo.GetDbIOStats(ctx, instanceName)
	if err == nil {
		for _, db := range dbStats {
			tempBytesMB += float64(db.TempBytes) / 1024 / 1024
		}
	}

	// Vacuum
	autovacWorkers, _ := c.pgRepo.FetchAutovacuumWorkers(ctx, instanceName)

	// Checkpoints
	cp, _ := c.pgRepo.FetchBGWriterStats(ctx, instanceName)
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
		// Log detailed BGWriter stats
		_ = c.tsLogger.LogPostgresBGWriterStats(ctx, hot.PostgresBGWriterRow{
			CaptureTimestamp:    time.Now().UTC(),
			ServerID:            serverID,
			CheckpointsTimed:    cp.CheckpointsTimed,
			CheckpointsReq:      cp.CheckpointsReq,
			CheckpointWriteTime: cp.CheckpointWriteTime,
			CheckpointSyncTime:  cp.CheckpointSyncTime,
			BuffersCheckpoint:   cp.BuffersCheckpoint,
			BuffersClean:        cp.BuffersClean,
			MaxwrittenClean:     cp.MaxwrittenClean,
			BuffersBackend:      cp.BuffersBackend,
			BuffersBackendFsync: cp.BuffersBackendFsync,
			BuffersAlloc:        cp.BuffersAlloc,
			StatsReset:          cp.StatsReset,
		})
	}

	// Archiver
	arch, _ := c.pgRepo.FetchArchiverStats(ctx, instanceName)
	if arch != nil {
		lastArch := time.Time{}
		if arch.LastArchivedTime != nil {
			lastArch = *arch.LastArchivedTime
		}
		lastFail := time.Time{}
		if arch.LastFailedTime != nil {
			lastFail = *arch.LastFailedTime
		}

		_ = c.tsLogger.LogPostgresArchiverStats(ctx, hot.PostgresArchiverRow{
			CaptureTimestamp: time.Now().UTC(),
			ServerID:         serverID,
			ArchivedCount:    arch.ArchivedCount,
			LastArchivedWal:  arch.LastArchivedWal.String,
			LastArchivedTime: lastArch,
			FailedCount:      arch.FailedCount,
			LastFailedWal:    arch.LastFailedWal.String,
			LastFailedTime:   lastFail,
			StatsReset:       arch.StatsReset,
		})
	}

	// Replication
	replicaLagSec, _ := c.pgRepo.FetchReplicaLagSec(ctx, instanceName)

	// Server Info (Version & Uptime)
	version, uptime, _ := c.pgRepo.GetServerInfo(ctx, instanceName)

	// Control Center extras
	walSizeMB, _ := c.pgRepo.FetchWalDirSizeMB(ctx, instanceName)
	replicaLagMB, _, _ := c.pgRepo.GetReplicationLag(ctx, instanceName)
	slowQueriesCount, _ := c.pgRepo.FetchSlowQueriesCount(ctx, instanceName, 1000.0)
	blockingSessionsCount, _ := c.pgRepo.FetchBlockingSessionsCount(ctx, instanceName)
	connUtil, _ := c.pgRepo.FetchConnectionUtilization(ctx, instanceName)
	var connMax, connUsed int
	var connUsagePct float64
	if connUtil != nil {
		connMax = connUtil.MaxConnections
		connUsed = connUtil.UsedConnections
		connUsagePct = connUtil.UsagePct
	}
	xidRisks, _ := c.pgRepo.GetXIDWraparoundRisk(ctx, instanceName)
	var xidWraparoundPct float64
	for _, r := range xidRisks {
		if r.UsedPct > xidWraparoundPct {
			xidWraparoundPct = r.UsedPct
		}
	}
	totalDeadlocks, _ := c.pgRepo.FetchDeadlocksTotalAllDBs(ctx, instanceName)
	deadlocksPerMin := 0.0
	if c.tsLogger != nil {
		if r, ok := c.tsLogger.ComputePgDeadlockRate(serverID, totalDeadlocks, 15.0); ok {
			deadlocksPerMin = r
		}
	}

	// 2. Build Snapshot
	snapshot := &instance_health.PgInstanceSnapshot{
		ServerID:             serverID,
		CollectedAt:          time.Now().UTC(),
		TPS:                  tps,
		ActiveSessions:       active,
		IdleSessions:         idle,
		IdleInTxSessions:     idleInTx,
		BlockedSessions:      waiting,
		CPUUsage:             cpuUsage,
		SharedBuffersUsedPct: sharedBuffersPct,
		CacheHitRatio:        cacheHitRatio,
		ConnectionsUsagePct:  connUsagePct,
		WALMBPerMin:          walRate,
		CheckpointReqRatio:   cpRatio,
		CheckpointsTimed:     int(cpTimed),
		CheckpointsReq:       int(cpReq),
		CheckpointWriteTime:  cpWriteTime,
		MaxXIDAge:            maxXidAge,
		DatabaseSizeGB:       databaseSizeGB,
		TempBytesMB:          tempBytesMB,
		AutovacuumWorkers:    autovacWorkers,
		DeadTuplePct:         deadTuplePct,
		ReplicaLagSec:        replicaLagSec,
		Version:              version,
		Uptime:               uptime,
	}

	// 3. Calculate Health Score
	if c.healthService == nil || c.healthRepo == nil {
		slog.Warn("[PgSnapshotCollector] healthService or healthRepo is nil for %s; skipping snapshot persist", "val", instanceName)
		return nil
	}
	snapshot.HealthScore = c.healthService.CalculateHealthScore(snapshot)

	// 4. Persist to Snapshot Table
	if err := c.healthRepo.UpsertSnapshot(ctx, snapshot); err != nil {
		slog.Error("[PgSnapshotCollector] UpsertSnapshot error", "target", instanceName, "err", err)
		return err
	}

	// 5. Persist to Timeseries Hypertables (Wide-row optimized)
	if err := c.healthRepo.LogSnapshotMetrics(ctx, snapshot); err != nil {
		slog.Error("[PgSnapshotCollector] LogSnapshotMetrics error", "target", instanceName, "err", err)
	}

	// 5.1 Persist to Backup & DR Domain
	wal, _ := c.pgRepo.FetchPgWALStats(ctx, instanceName)
	if wal != nil && cp != nil {
		// Re-fetch is_in_recovery specifically
		var isInRecovery bool
		db, ok := c.pgRepo.GetConn(instanceName)
		if ok {
			_ = db.QueryRow("SELECT pg_is_in_recovery()").Scan(&isInRecovery)
		}

		archCount := int64(0)
		archFailed := int64(0)
		var lastArch, lastFail *time.Time
		if arch != nil {
			archCount = arch.ArchivedCount
			archFailed = arch.FailedCount
			lastArch = arch.LastArchivedTime
			lastFail = arch.LastFailedTime
		}

		_ = c.tsLogger.LogPostgresBackupDR(ctx, serverID, hot.PostgresBackupDRRow{
			CaptureTimestamp:      time.Now().UTC(),
			WalBytesTotal:         wal.WalBytes,
			WalRecordsTotal:       wal.WalRecords,
			WalFPITotal:           wal.WalFpi,
			ArchivedCount:         archCount,
			ArchiveFailedCount:    archFailed,
			LastArchivedTime:      lastArch,
			LastFailedTime:        lastFail,
			CheckpointsTimed:      cp.CheckpointsTimed,
			CheckpointsReq:        cp.CheckpointsReq,
			CheckpointWriteTimeMs: cp.CheckpointWriteTime,
			CheckpointSyncTimeMs:  cp.CheckpointSyncTime,
			IsInRecovery:          isInRecovery,
		})
	}

	// Bug 5: Advanced Postgres Metrics
	// 1. Wait event stats
	waitCounts, err := c.pgRepo.GetWaitEventCounts(ctx, instanceName)
	if err == nil && len(waitCounts) > 0 {
		var waitRows []hot.PostgresWaitEventRow
		now := time.Now().UTC()
		for _, w := range waitCounts {
			waitRows = append(waitRows, hot.PostgresWaitEventRow{
				CaptureTimestamp: now,
				ServerID:         serverID,
				WaitEventType:    w.WaitEventType,
				WaitEvent:        w.WaitEvent,
				SessionsCount:    w.SessionsCount,
			})
		}
		_ = c.tsLogger.LogPostgresWaitEvents(ctx, serverID, waitRows)
	}

	// 2. DB I/O stats
	// dbStats already fetched above
	if len(dbStats) > 0 {
		var ioRows []hot.PostgresDbIORow
		now := time.Now().UTC()
		for _, d := range dbStats {
			ioRows = append(ioRows, hot.PostgresDbIORow{
				CaptureTimestamp: now,
				ServerID:         serverID,
				DatabaseName:     d.DatabaseName,
				BlksRead:         d.BlksRead,
				BlksHit:          d.BlksHit,
				TempFiles:        d.TempFiles,
				TempBytes:        d.TempBytes,
			})
		}
		_ = c.tsLogger.LogPostgresDbIOStats(ctx, serverID, ioRows)
	}

	// 3. Control Center Stats
	healthStatus := "healthy"
	switch {
	case snapshot.HealthScore < 60:
		healthStatus = "critical"
	case snapshot.HealthScore < 80:
		healthStatus = "warning"
	}
	_ = c.tsLogger.LogPostgresControlCenterStats(ctx, hot.PostgresControlCenterRow{
		CaptureTimestamp:    time.Now().UTC(),
		ServerID:            serverID,
		WALMBPerMin:         walRate,
		WALSizeMB:           walSizeMB,
		ReplicaLagMB:        replicaLagMB,
		ReplicaLagSec:       replicaLagSec,
		CheckpointReqRatio:  cpRatio,
		XIDAge:              maxXidAge,
		XIDWraparoundPct:    xidWraparoundPct,
		TPS:                 tps,
		ActiveSessions:      active,
		WaitingSessions:     waiting,
		SlowQueriesCount:    slowQueriesCount,
		BlockingSessions:    blockingSessionsCount,
		AutovacuumWorkers:   autovacWorkers,
		DeadTuplePct:        deadTuplePct,
		HealthScore:         snapshot.HealthScore,
		HealthStatus:        healthStatus,
		IdleSessions:        idle,
		IdleInTxnSessions:   idleInTx,
		ConnectionsMax:      connMax,
		ConnectionsUsed:     connUsed,
		ConnectionsUsagePct: connUsagePct,
		CacheHitRatioPct:    cacheHitRatio,
		DeadlocksPerMin:     deadlocksPerMin,
	})

	// 4. Session State Counts
	sessionTotal := active + idle + idleInTx + waiting
	_ = c.tsLogger.LogPostgresSessionStateCounts(ctx, serverID, active, idle, idleInTx, waiting, sessionTotal)

	return nil
}
