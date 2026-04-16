// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background collector daemon for historical storage, query store, long-running queries, and AG health stats.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/collector/pghostcpu"
	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

const (
	HistoricalInterval = 60 * time.Second
)

func (s *MetricsService) sihDue(m map[string]time.Time, instanceName string, now time.Time, interval time.Duration) bool {
	s.sihMu.Lock()
	defer s.sihMu.Unlock()
	last, ok := m[instanceName]
	if !ok || now.Sub(last) >= interval {
		m[instanceName] = now
		return true
	}
	return false
}

func (s *MetricsService) StartBackgroundCollector(ctx context.Context) {
	log.Printf("[Collector] Split-Speed Background Daemon starting...")
	log.Printf("[Collector]   - Historical Storage ticker: every %v", HistoricalInterval)
	log.Printf("[Collector]   - Long Running Queries: every 60s (within historical)")
	log.Printf("[Collector]   - Top CPU Queries: every 60s (within historical)")
	log.Printf("[Collector]   - Live Diagnostics ticker: DISABLED (RTD is on-demand only)")
	log.Printf("[Collector]   - PG Locks/Blocking incidents: adaptive 15s/5s")

	s.dashboardCache = make(map[string]models.DashboardMetrics)
	s.pgDashboardCache = make(map[string]models.PgCoreDashboardCache)

	historyTicker := time.NewTicker(HistoricalInterval)
	defer historyTicker.Stop()

	// Run one live scrape on startup only (warm cache), then rely on on-demand RTD endpoints.
	s.runLiveDiagnosticsWithContext(ctx)
	// Start stateful incident monitoring in the background (if Timescale schema is ready).
	go s.StartPgLocksBlockingCollector(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Collector] Background daemon shutting down")
			return

		case <-historyTicker.C:
			s.runHistoricalStorageWithContext(ctx)

		}
	}
}

func (s *MetricsService) runLiveDiagnosticsWithContext(ctx context.Context) {
	var wg sync.WaitGroup

	for _, inst := range s.Config.Instances {
		wg.Add(1)
		go func(instanceName string, instanceType string) {
			defer wg.Done()
			t0 := time.Now()

			if instanceType == "postgres" && !s.PgRepo.HasConnection(instanceName) {
				return
			}
			if instanceType == "sqlserver" && !s.MsRepo.HasConnection(instanceName) {
				return
			}

			s.cacheMutex.RLock()
			prevMsTick := s.dashboardCache[instanceName]
			s.cacheMutex.RUnlock()

			if instanceType == "sqlserver" {
				currentMs := s.MsRepo.FetchLiveTelemetry(instanceName, prevMsTick)
				currentMs.Timestamp = time.Now().Format("15:04:05")

				s.cacheMutex.Lock()
				s.dashboardCache[instanceName] = currentMs
				s.cacheMutex.Unlock()

				slog.Info("collector_live_scrape",
					"instance", instanceName,
					"engine", instanceType,
					"duration_ms", time.Since(t0).Milliseconds(),
				)
			} else if instanceType == "postgres" {
				s.cacheMutex.RLock()
				prevPgTick := s.pgDashboardCache[instanceName]
				s.cacheMutex.RUnlock()

				currentPg := s.PgRepo.FetchPgCoreThroughputTelemetry(instanceName, prevPgTick)
				currentPg.Timestamp = time.Now().Format("15:04:05")

				s.cacheMutex.Lock()
				s.pgDashboardCache[instanceName] = currentPg
				s.cacheMutex.Unlock()

				slog.Info("collector_live_scrape",
					"instance", instanceName,
					"engine", instanceType,
					"duration_ms", time.Since(t0).Milliseconds(),
				)
			}
		}(inst.Name, inst.Type)
	}

	wg.Wait()
}

func (s *MetricsService) runLiveDiagnosticsForInstance(ctx context.Context, instanceName string) {
	var instanceType string
	for _, inst := range s.Config.Instances {
		if inst.Name == instanceName {
			instanceType = inst.Type
			break
		}
	}
	if instanceType == "" {
		return
	}

	t0 := time.Now()
	if instanceType == "postgres" && !s.PgRepo.HasConnection(instanceName) {
		return
	}
	if instanceType == "sqlserver" && !s.MsRepo.HasConnection(instanceName) {
		return
	}

	if instanceType == "sqlserver" {
		s.cacheMutex.RLock()
		prevMsTick := s.dashboardCache[instanceName]
		s.cacheMutex.RUnlock()

		currentMs := s.MsRepo.FetchLiveTelemetry(instanceName, prevMsTick)
		currentMs.Timestamp = time.Now().Format("15:04:05")

		s.cacheMutex.Lock()
		s.dashboardCache[instanceName] = currentMs
		s.cacheMutex.Unlock()
		slog.Info("collector_live_scrape",
			"instance", instanceName,
			"engine", instanceType,
			"duration_ms", time.Since(t0).Milliseconds(),
		)
		return
	}

	// postgres
	s.cacheMutex.RLock()
	prevPgTick := s.pgDashboardCache[instanceName]
	s.cacheMutex.RUnlock()

	currentPg := s.PgRepo.FetchPgCoreThroughputTelemetry(instanceName, prevPgTick)
	currentPg.Timestamp = time.Now().Format("15:04:05")

	s.cacheMutex.Lock()
	s.pgDashboardCache[instanceName] = currentPg
	s.cacheMutex.Unlock()
	slog.Info("collector_live_scrape",
		"instance", instanceName,
		"engine", instanceType,
		"duration_ms", time.Since(t0).Milliseconds(),
	)
}

func (s *MetricsService) runHistoricalStorageWithContext(ctx context.Context) {
	var wg sync.WaitGroup

	for _, inst := range s.Config.Instances {
		wg.Add(1)
		go func(instanceName string, instanceType string) {
			defer wg.Done()
			t0 := time.Now()

			if instanceType == "postgres" && !s.PgRepo.HasConnection(instanceName) {
				return
			}
			if instanceType == "sqlserver" && !s.MsRepo.HasConnection(instanceName) {
				return
			}

			if instanceType == "sqlserver" {
				if s.tsLogger != nil {
					if err := s.logSQLServerHistoricalToTimescaleWithContext(ctx, instanceName); err != nil {
						log.Printf("[Collector] ERROR: Failed to log SQLServer historical metrics for %s: %v", instanceName, err)
					} else {
						log.Printf("[Collector] Successfully logged SQLServer historical metrics for %s to TimescaleDB", instanceName)
					}
				} else {
					log.Printf("[Collector] WARNING: tsLogger is nil, TimescaleDB logging disabled for %s", instanceName)
				}

				if s.tsLogger != nil {
					if err := s.fetchAndLogQueryStoreStatsWithContext(ctx, instanceName); err != nil {
						log.Printf("[Collector] ERROR: Failed to fetch Query Store stats for %s: %v", instanceName, err)
					}
				}

				if s.tsLogger != nil {
					if err := s.fetchAndLogLongRunningQueriesWithContext(ctx, instanceName, 30); err != nil {
						log.Printf("[Collector] ERROR: Failed to fetch Long Running Queries for %s: %v", instanceName, err)
					}
				}

				if s.tsLogger != nil {
					if err := s.fetchAndLogAGHealthStatsWithContext(ctx, instanceName); err != nil {
						log.Printf("[Collector] ERROR: Failed to fetch AG Health stats for %s: %v", instanceName, err)
					}
				}

				if s.tsLogger != nil {
					if err := s.fetchAndLogAgentJobsMetricsWithContext(ctx, instanceName); err != nil {
						log.Printf("[Collector] ERROR: Failed to fetch Agent Jobs metrics for %s: %v", instanceName, err)
					}
				}

				if s.tsLogger != nil {
					if err := s.fetchAndLogCPUSchedulerStatsWithContext(ctx, instanceName); err != nil {
						log.Printf("[Collector] ERROR: Failed to fetch CPU Scheduler stats for %s: %v", instanceName, err)
					} else {
						log.Printf("[Collector] Successfully logged CPU Scheduler stats for %s to TimescaleDB", instanceName)
					}
				}

				if s.tsLogger != nil {
					if err := s.fetchAndLogServerPropertiesWithContext(ctx, instanceName); err != nil {
						log.Printf("[Collector] ERROR: Failed to fetch Server Properties for %s: %v", instanceName, err)
					} else {
						log.Printf("[Collector] Successfully logged Server Properties for %s to TimescaleDB", instanceName)
					}
				}

				slog.Info("collector_historical_scrape",
					"instance", instanceName,
					"engine", instanceType,
					"duration_ms", time.Since(t0).Milliseconds(),
				)
			} else if instanceType == "postgres" {
				if s.tsLogger != nil {
					s.cacheMutex.RLock()
					pgCache := s.pgDashboardCache[instanceName]
					s.cacheMutex.RUnlock()

					if err := s.logPostgresMetricsToTimescaleWithContext(ctx, instanceName, pgCache); err != nil {
						log.Printf("[Collector] ERROR: Failed to log Postgres metrics to TimescaleDB for %s: %v", instanceName, err)
					} else {
						log.Printf("[Collector] Successfully logged Postgres metrics for %s to TimescaleDB", instanceName)
					}
				}

				slog.Info("collector_historical_scrape",
					"instance", instanceName,
					"engine", instanceType,
					"duration_ms", time.Since(t0).Milliseconds(),
				)
			}
		}(inst.Name, inst.Type)
	}

	wg.Wait()
}

func (s *MetricsService) logSQLServerHistoricalToTimescaleWithContext(ctx context.Context, instanceName string) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	s.cacheMutex.RLock()
	currentMs := s.dashboardCache[instanceName]
	s.cacheMutex.RUnlock()

	sysData := map[string]interface{}{
		"avg_cpu_load": currentMs.AvgCPULoad,
		"memory_usage": currentMs.MemoryUsage,
		"active_users": currentMs.ActiveUsers,
		"total_locks":  currentMs.TotalLocks,
		"deadlocks":    currentMs.Deadlocks,
		"data_disk_mb": currentMs.DiskUsage.DataMB,
		"log_disk_mb":  currentMs.DiskUsage.LogMB,
		"free_disk_mb": currentMs.DiskUsage.FreeMB,
	}
	if err := s.tsLogger.LogSQLServerMetrics(ctx, instanceName, sysData); err != nil {
		return fmt.Errorf("LogSQLServerMetrics: %w", err)
	}

	if len(currentMs.CPUHistory) > 0 {
		tick := currentMs.CPUHistory[len(currentMs.CPUHistory)-1]
		cpuTicks := []map[string]interface{}{
			{
				"sql_process":   tick.SQLProcess,
				"system_idle":   tick.SystemIdle,
				"other_process": tick.OtherProcess,
			},
		}
		if err := s.tsLogger.LogSQLServerCPUHistory(ctx, instanceName, cpuTicks); err != nil {
			log.Printf("[Collector] WARNING: LogSQLServerCPUHistory failed: %v", err)
		}
	}

	if len(currentMs.WaitHistory) > 0 {
		ws := currentMs.WaitHistory[len(currentMs.WaitHistory)-1]
		waits := []map[string]interface{}{
			{
				"disk_read":   ws.DiskRead,
				"blocking":    ws.Blocking,
				"parallelism": ws.Parallelism,
				"other":       ws.Other,
			},
		}
		if err := s.tsLogger.LogSQLServerWaitHistory(ctx, instanceName, waits); err != nil {
			log.Printf("[Collector] WARNING: LogSQLServerWaitHistory failed: %v", err)
		}
	}

	conns := make(map[string]map[string]interface{})
	for _, c := range currentMs.ConnectionStats {
		conns[c.DatabaseName] = map[string]interface{}{
			"login_name":         c.LoginName,
			"database_name":      c.DatabaseName,
			"active_connections": c.ActiveConnections,
			"active_requests":    c.ActiveRequests,
		}
	}
	if err := s.tsLogger.LogSQLServerConnectionHistory(ctx, instanceName, conns); err != nil {
		log.Printf("[Collector] WARNING: LogSQLServerConnectionHistory failed: %v", err)
	}

	locks := make(map[string]map[string]interface{})
	for dbName, l := range currentMs.LocksByDB {
		locks[dbName] = map[string]interface{}{
			"total_locks": l.TotalLocks,
			"deadlocks":   l.Deadlocks,
		}
	}
	if err := s.tsLogger.LogSQLServerLockHistory(ctx, instanceName, locks); err != nil {
		log.Printf("[Collector] WARNING: LogSQLServerLockHistory failed: %v", err)
	}

	disk := make(map[string]map[string]interface{})
	for dbName, d := range currentMs.DiskByDB {
		disk[dbName] = map[string]interface{}{
			"data_mb": d.DataMB,
			"log_mb":  d.LogMB,
			"free_mb": d.FreeMB,
		}
	}
	if err := s.tsLogger.LogSQLServerDiskHistory(ctx, instanceName, disk); err != nil {
		log.Printf("[Collector] WARNING: LogSQLServerDiskHistory failed: %v", err)
	}

	topQueries, err := s.MsRepo.FetchTopCPUQueries(instanceName, 20, "")
	if err != nil {
		log.Printf("[Collector] WARNING: FetchTopCPUQueries failed: %v", err)
	} else {
		log.Printf("[Collector] FetchTopCPUQueries returned %d queries for %s", len(topQueries), instanceName)
		if len(topQueries) > 0 {
			log.Printf("[Collector] Sample query data: %+v", topQueries[0])
			for i, q := range topQueries {
				log.Printf("[Collector] Query[%d]: hash=%v, Executions=%v, Total_CPU_ms=%v, Total_Logical_Reads=%v",
					i, q["Query_Hash"], q["Executions"], q["Total_CPU_ms"], q["Total_Logical_Reads"])
			}
		}
		if err := s.tsLogger.LogSQLServerTopQueries(ctx, instanceName, topQueries); err != nil {
			log.Printf("[Collector] WARNING: LogSQLServerTopQueries failed: %v", err)
		} else {
			log.Printf("[Collector] Successfully logged %d top queries for %s", len(topQueries), instanceName)
		}
	}

	db, ok := s.MsRepo.GetConn(instanceName)
	if ok && db != nil {
		statsRows, err := s.MsRepo.CollectTopQueries(db, 200)
		if err != nil {
			log.Printf("[Collector] WARNING: CollectTopQueries (query stats pipeline) failed for %s: %v", instanceName, err)
		} else if len(statsRows) > 0 {
			if err := s.tsLogger.LogQueryStatsStaging(ctx, instanceName, statsRows); err != nil {
				log.Printf("[Collector] WARNING: LogQueryStatsStaging failed: %v", err)
			} else if err := s.tsLogger.ProcessQueryStatsSnapshot(ctx, instanceName); err != nil {
				log.Printf("[Collector] WARNING: ProcessQueryStatsSnapshot failed: %v", err)
			} else if err := s.tsLogger.ProcessQueryStatsDelta(ctx, instanceName); err != nil {
				log.Printf("[Collector] WARNING: ProcessQueryStatsDelta failed: %v", err)
			} else {
				log.Printf("[Collector] Query stats pipeline OK for %s (%d staging rows → snapshot + delta)", instanceName, len(statsRows))
			}
		}
	}

	// Storage & Index Health (delta stats)
	if ok && db != nil {
		capture := time.Now().UTC()
		now := capture
		due15mIndex := s.sihDue(s.sihLastIndex15m, instanceName, now, 15*time.Minute)
		due15mTable := s.sihDue(s.sihLastTable15m, instanceName, now, 15*time.Minute)
		due6hGrowth := s.sihDue(s.sihLastGrowth6h, instanceName, now, 6*time.Hour)
		dueDailyDefs := s.sihDue(s.sihLastDefsDaily, instanceName, now, 24*time.Hour)

		// For each user DB configured, switch context and collect.
		// We intentionally scope to configured DB list to avoid accidental access to system DBs.
		var dbs []string
		for _, inst := range s.Config.Instances {
			if inst.Name == instanceName && inst.Type == "sqlserver" {
				dbs = inst.Databases
				break
			}
		}
		if len(dbs) == 0 {
			discovered, derr := s.MsRepo.ListSQLServerUserDatabases(instanceName)
			if derr != nil {
				log.Printf("[Collector][SIH] instance %q: Instances[].databases empty and auto-discover failed: %v", instanceName, derr)
			} else {
				dbs = discovered
				const maxAutoDB = 64
				if len(dbs) > maxAutoDB {
					log.Printf("[Collector][SIH] instance %q: auto-discovered %d databases; capping to first %d for SIH tick", instanceName, len(dbs), maxAutoDB)
					dbs = dbs[:maxAutoDB]
				}
				if len(dbs) > 0 {
					log.Printf("[Collector][SIH] instance %q: Instances[].databases empty; auto-discovered %d user database(s) for SIH", instanceName, len(dbs))
				}
			}
		}
		if len(dbs) == 0 && (due15mIndex || due15mTable || due6hGrowth || dueDailyDefs) {
			log.Printf("[Collector][SIH] instance %q: no databases to scan (set Instances[].databases or grant access to user DBs)", instanceName)
		}
		for _, dbName := range dbs {
			if strings.TrimSpace(dbName) == "" {
				continue
			}
			conn, err := db.Conn(ctx)
			if err != nil {
				continue
			}
			// Bracket identifier; double any closing bracket inside the name.
			useSQL := "USE [" + strings.ReplaceAll(dbName, "]", "]]") + "]"
			if _, err := conn.ExecContext(ctx, useSQL); err != nil {
				_ = conn.Close()
				log.Printf("[Collector][SIH] USE database failed for %s db=%q: %v", instanceName, dbName, err)
				continue
			}

			if due15mIndex {
				idxRows, err := collectors.CollectSQLServerIndexUsage(ctx, conn)
				if err != nil {
					log.Printf("[Collector][SIH] CollectSQLServerIndexUsage failed for %s db=%s: %v", instanceName, dbName, err)
				} else if len(idxRows) == 0 {
					log.Printf("[Collector][SIH] CollectSQLServerIndexUsage returned 0 rows for %s db=%s", instanceName, dbName)
				} else if n, perr := collectors.PersistSQLServerIndexUsageDeltas(ctx, s.tsLogger, instanceName, dbName, idxRows, capture); perr != nil {
					log.Printf("[Collector][SIH] PersistSQLServerIndexUsageDeltas failed for %s db=%s: %v", instanceName, dbName, perr)
				} else {
					log.Printf("[Collector][SIH] index usage persisted for %s db=%s rows=%d inserted=%d", instanceName, dbName, len(idxRows), n)
				}
			}

			// Table size snapshot query powers both 15m table usage and 6h growth history; collect once if either is due.
			if due15mTable || due6hGrowth {
				tblRows, err := collectors.CollectSQLServerTableSizeSnapshot(ctx, conn)
				if err == nil && len(tblRows) > 0 {
					if due15mTable {
						// table_usage_stats (sizes snapshot; scan counters are 0 for SQL Server)
						_, _ = collectors.PersistSQLServerTableUsageDeltas(ctx, s.tsLogger, instanceName, tblRows, capture)
					}
					if due6hGrowth {
						// table_size_history (growth snapshot)
						_, _ = collectors.PersistSQLServerTableGrowthHistory(ctx, s.tsLogger, instanceName, tblRows, capture)
					}
				}
			}

			// Index definitions snapshot (daily cadence).
			if dueDailyDefs {
				dayBucket := time.Date(capture.Year(), capture.Month(), capture.Day(), 0, 0, 0, 0, time.UTC)
				defRows, err := collectors.CollectSQLServerIndexDefinitions(ctx, conn)
				if err == nil && len(defRows) > 0 {
					_, _ = collectors.PersistSQLServerIndexDefinitions(ctx, s.tsLogger, instanceName, defRows, dayBucket)
				}
			}
			_ = conn.Close()
		}

		// Daily unused-index snapshot: once per instance (not per database).
		if dueDailyDefs && s.tsLogger != nil {
			if n, err := s.tsLogger.RefreshIndexUnusedCandidatesDaily(ctx, "sqlserver", instanceName, capture, 100); err != nil {
				log.Printf("[Collector][SIH] Daily unused index snapshot failed for %s: %v", instanceName, err)
			} else {
				log.Printf("[Collector][SIH] Daily unused index snapshot rows for %s: %d", instanceName, n)
			}
		}
	}

	return nil
}

func (s *MetricsService) logPostgresMetricsToTimescaleWithContext(ctx context.Context, instanceName string, cache models.PgCoreDashboardCache) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	for dbName, points := range cache.HistoryByDB {
		if len(points) == 0 {
			continue
		}
		p := points[len(points)-1]
		if err := s.tsLogger.LogPostgresThroughput(ctx, instanceName, dbName, p.Tps, p.CacheHitPct, p.TxnDelta, p.BlksReadDelta, p.BlksHitDelta); err != nil {
			log.Printf("[Collector] ERROR: LogPostgresThroughput failed for %s: %v", instanceName, err)
			return fmt.Errorf("LogPostgresThroughput: %w", err)
		}
	}

	active, idle, total, connErr := s.PgRepo.GetConnectionStats(instanceName)
	if connErr != nil {
		active, idle, total = 0, 0, 0
		log.Printf("[Collector] GetConnectionStats failed for %s: %v", instanceName, connErr)
	} else {
		if err := s.tsLogger.LogPostgresConnectionStats(ctx, instanceName, total, active, idle); err != nil {
			log.Printf("[Collector] ERROR: LogPostgresConnectionStats failed for %s: %v", instanceName, err)
			return fmt.Errorf("LogPostgresConnectionStats: %w", err)
		}
	}

	memPct := 0.0
	hostPct := 0.0
	if detail, derr := s.PgRepo.GetSystemStatsDetail(instanceName); derr == nil && detail != nil {
		hostPct = detail.CPUUsagePct
		memPct = detail.MemoryUsedPct
	} else {
		cu, mu, err := s.PgRepo.GetSystemStats(instanceName)
		if err != nil {
			log.Printf("[Collector] system stats for %s: %v", instanceName, err)
		} else {
			hostPct = cu
			memPct = mu
		}
	}
	snap := pghostcpu.Collect()
	if snap.HostCpuPercent > 0 {
		hostPct = snap.HostCpuPercent
	}
	pgPct := snap.PostgresCpuPercent

	row := hot.PgSystemStatsInsert{
		CPUUsage:           hostPct,
		MemoryUsage:        memPct,
		ActiveConnections:  active,
		IdleConnections:    idle,
		TotalConnections:   total,
		HostCpuPercent:     hostPct,
		PostgresCpuPercent: pgPct,
		Load1m:             snap.Load1m,
		Load5m:             snap.Load5m,
		Load15m:            snap.Load15m,
		CpuCores:           snap.CpuCores,
	}
	if err := s.tsLogger.LogPostgresSystemStats(ctx, instanceName, row); err != nil {
		log.Printf("[Collector] ERROR: LogPostgresSystemStats failed for %s: %v", instanceName, err)
	}

	bgStats, err := s.PgRepo.FetchBGWriterStats(instanceName)
	if err == nil && bgStats != nil {
		bgRow := hot.PostgresBGWriterRow{
			CaptureTimestamp:    time.Now().UTC(),
			ServerInstanceName:  instanceName,
			CheckpointsTimed:    bgStats.CheckpointsTimed,
			CheckpointsReq:      bgStats.CheckpointsReq,
			CheckpointWriteTime: bgStats.CheckpointWriteTime,
			CheckpointSyncTime:  bgStats.CheckpointSyncTime,
			BuffersCheckpoint:   bgStats.BuffersCheckpoint,
			BuffersClean:        bgStats.BuffersClean,
			MaxwrittenClean:     bgStats.MaxwrittenClean,
			BuffersBackend:      bgStats.BuffersBackend,
			BuffersAlloc:        bgStats.BuffersAlloc,
		}
		if err := s.tsLogger.LogPostgresBGWriter(ctx, instanceName, bgRow); err != nil {
			log.Printf("[Collector] ERROR: LogPostgresBGWriter failed for %s: %v", instanceName, err)
		}
	}

	archStats, err := s.PgRepo.FetchArchiverStats(instanceName)
	if err == nil && archStats != nil {
		archRow := hot.PostgresArchiverRow{
			CaptureTimestamp:   time.Now().UTC(),
			ServerInstanceName: instanceName,
			ArchivedCount:      archStats.ArchivedCount,
			FailedCount:        archStats.FailedCount,
			LastArchivedWal:    archStats.LastArchivedWal,
			LastFailedWal:      archStats.LastFailedWal,
		}
		if err := s.tsLogger.LogPostgresArchiver(ctx, instanceName, archRow); err != nil {
			log.Printf("[Collector] ERROR: LogPostgresArchiver failed for %s: %v", instanceName, err)
		}
	}

	replStats, err := s.PgRepo.GetReplicationStats(instanceName)
	if err == nil && replStats != nil {
		replData := map[string]interface{}{
			"is_primary":        replStats.IsPrimary,
			"cluster_state":     replStats.ClusterState,
			"max_lag_mb":        replStats.MaxLagMB,
			"wal_gen_rate_mbps": replStats.WalGenRateMBps,
			"bgwriter_eff_pct":  replStats.BgWriterEffPct,
		}
		if err := s.tsLogger.LogPostgresReplicationStats(ctx, instanceName, replData); err != nil {
			log.Printf("[Collector] ERROR: LogPostgresReplicationStats failed for %s: %v", instanceName, err)
		}
	}

	s.runPostgresStorageIndexHealthTick(ctx, instanceName)

	return nil
}

func (s *MetricsService) fetchAndLogQueryStoreStatsWithContext(ctx context.Context, instanceName string) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	stats, err := s.MsRepo.FetchQueryStoreStats(instanceName)
	if err != nil {
		return fmt.Errorf("FetchQueryStoreStats: %w", err)
	}

	if len(stats) == 0 {
		log.Printf("[Collector] No Query Store stats found for %s", instanceName)
		return nil
	}

	timestamp := time.Now().UTC()

	rows := make([]hot.QueryStoreStatsRow, 0, len(stats))
	for _, qs := range stats {
		dbn := strings.TrimSpace(qs.DatabaseName)
		if dbn == "" {
			dbn = "unknown"
		}
		rows = append(rows, hot.QueryStoreStatsRow{
			CaptureTimestamp: timestamp,
			ServerName:       instanceName,
			DatabaseName:     dbn,
			QueryHash:        qs.QueryHash,
			QueryText:        qs.QueryText,
			Executions:       qs.Executions,
			AvgDurationMs:    qs.AvgDurationMs,
			AvgCpuMs:         qs.AvgCpuMs,
			AvgLogicalReads:  qs.AvgLogicalReads,
			TotalCpuMs:       qs.TotalCpuMs,
		})
	}

	if err := s.tsLogger.LogQueryStoreStatsDirect(ctx, rows); err != nil {
		return fmt.Errorf("LogQueryStoreStatsDirect: %w", err)
	}

	log.Printf("[Collector] Successfully logged %d Query Store stats for %s", len(rows), instanceName)
	return nil
}

func (s *MetricsService) fetchAndLogLongRunningQueriesWithContext(ctx context.Context, instanceName string, minDurationSeconds int) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	stats, err := s.MsRepo.FetchLongRunningQueries(instanceName, minDurationSeconds)
	if err != nil {
		return fmt.Errorf("FetchLongRunningQueries: %w", err)
	}

	if len(stats) == 0 {
		return nil
	}

	timestamp := time.Now().UTC()

	rows := make([]hot.LongRunningQueryRow, 0, len(stats))
	for _, q := range stats {
		rows = append(rows, hot.LongRunningQueryRow{
			CaptureTimestamp:     timestamp,
			ServerInstanceName:   instanceName,
			SessionID:            q.SessionID,
			RequestID:            q.RequestID,
			DatabaseName:         q.DatabaseName,
			LoginName:            q.LoginName,
			HostName:             q.HostName,
			ProgramName:          q.ProgramName,
			QueryHash:            q.QueryHash,
			QueryText:            q.QueryText,
			WaitType:             q.WaitType,
			BlockingSessionID:    q.BlockingSessionID,
			Status:               q.Status,
			CPUTimeMs:            q.CPUTimeMs,
			TotalElapsedTimeMs:   q.TotalElapsedTimeMs,
			Reads:                q.Reads,
			Writes:               q.Writes,
			GrantedQueryMemoryMB: q.GrantedQueryMemoryMB,
			RowCount:             q.RowCount,
		})
	}

	if err := s.tsLogger.LogSQLServerLongRunningQueries(ctx, instanceName, rows); err != nil {
		return fmt.Errorf("LogSQLServerLongRunningQueries: %w", err)
	}

	log.Printf("[Collector] Successfully logged %d long-running queries for %s", len(rows), instanceName)
	return nil
}

func (s *MetricsService) fetchAndLogAGHealthStatsWithContext(ctx context.Context, instanceName string) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	stats, err := s.MsRepo.FetchAGHealthStats(instanceName)
	if err != nil {
		return fmt.Errorf("FetchAGHealthStats: %w", err)
	}

	if len(stats) == 0 {
		log.Printf("[Collector] No AG Health stats found for %s (may not have AG configured)", instanceName)
		return nil
	}

	timestamp := time.Now().UTC()

	rows := make([]hot.AGHealthRow, 0, len(stats))
	for _, ag := range stats {
		rows = append(rows, hot.AGHealthRow{
			CaptureTimestamp:   timestamp,
			ServerInstanceName: instanceName,
			AGName:             ag.AGName,
			ReplicaServerName:  ag.ReplicaServerName,
			DatabaseName:       ag.DatabaseName,
			ReplicaRole:        ag.ReplicaRole,
			SyncState:          ag.SynchronizationState,
			SyncStateDesc:      ag.SyncStateDesc,
			IsPrimaryReplica:   ag.IsPrimaryReplica,
			LogSendQueueKB:     ag.LogSendQueueKB,
			RedoQueueKB:        ag.RedoQueueKB,
			LogSendRateKB:      ag.LogSendRateKB,
			RedoRateKB:         ag.RedoRateKB,
			LastSentTime:       ag.LastSentTime,
			LastReceivedTime:   ag.LastReceivedTime,
			LastHardenedTime:   ag.LastHardenedTime,
			LastRedoneTime:     ag.LastRedoneTime,
			SecondaryLagSecs:   ag.SecondaryLagSecs,
		})
	}

	if err := s.tsLogger.LogAGHealth(ctx, instanceName, rows); err != nil {
		return fmt.Errorf("LogAGHealth: %w", err)
	}

	log.Printf("[Collector] Successfully logged %d AG Health stats for %s", len(rows), instanceName)
	return nil
}

func (s *MetricsService) fetchAndLogAgentJobsMetricsWithContext(ctx context.Context, instanceName string) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	metrics := s.MsRepo.FetchAgentJobs(instanceName)

	jobMetrics := map[string]interface{}{
		"total_jobs":      metrics.Summary.TotalJobs,
		"enabled_jobs":    metrics.Summary.EnabledJobs,
		"disabled_jobs":   metrics.Summary.DisabledJobs,
		"running_jobs":    metrics.Summary.RunningJobs,
		"failed_jobs_24h": metrics.Summary.FailedJobs,
	}

	hasError := metrics.Summary.TotalJobs == -1
	if hasError {
		jobMetrics["error_message"] = metrics.LastError
		log.Printf("[Collector] Agent Jobs fetch returned error state for %s: %s", instanceName, metrics.LastError)
	}

	if err := s.tsLogger.LogSQLServerJobMetrics(ctx, instanceName, jobMetrics); err != nil {
		return fmt.Errorf("LogSQLServerJobMetrics: %w", err)
	}

	if !hasError {
		jobDetails := make([]map[string]interface{}, 0, len(metrics.Jobs))
		for _, j := range metrics.Jobs {
			jobDetails = append(jobDetails, map[string]interface{}{
				"job_name":        j.JobName,
				"enabled":         j.Enabled,
				"owner":           j.Owner,
				"created_date":    j.CreatedDate,
				"current_status":  j.CurrentStatus,
				"last_run_date":   j.LastRunDate,
				"last_run_time":   j.LastRunTime,
				"last_run_status": j.LastRunStatus,
			})
		}
		if err := s.tsLogger.LogSQLServerJobDetails(ctx, instanceName, jobDetails); err != nil {
			log.Printf("[Collector] Warning: failed to log job details for %s: %v", instanceName, err)
		}

		schedules := make([]map[string]interface{}, 0, len(metrics.Schedules))
		for _, sc := range metrics.Schedules {
			schedules = append(schedules, map[string]interface{}{
				"job_name":      sc.JobName,
				"job_enabled":   sc.JobEnabled,
				"schedule_name": sc.ScheduleName,
				"status":        sc.Status,
			})
		}
		if err := s.tsLogger.LogSQLServerJobSchedules(ctx, instanceName, schedules); err != nil {
			log.Printf("[Collector] Warning: failed to log job schedules for %s: %v", instanceName, err)
		}

		failures := make([]map[string]interface{}, 0, len(metrics.Failures))
		for _, f := range metrics.Failures {
			failures = append(failures, map[string]interface{}{
				"job_name":  f.JobName,
				"step_name": f.StepName,
				"message":   f.Message,
				"run_date":  f.RunDate,
				"run_time":  f.RunTime,
			})
		}
		if err := s.tsLogger.LogSQLServerJobFailures(ctx, instanceName, failures); err != nil {
			log.Printf("[Collector] Warning: failed to log job failures for %s: %v", instanceName, err)
		}
	}

	log.Printf("[Collector] Successfully logged Agent Jobs metrics for %s: %d total, %d running, %d failed",
		instanceName, metrics.Summary.TotalJobs, metrics.Summary.RunningJobs, metrics.Summary.FailedJobs)
	return nil
}

func (s *MetricsService) GetCachedDashboard(instanceName string) models.DashboardMetrics {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.dashboardCache[instanceName]
}

func (s *MetricsService) GetCachedPgCoreDashboard(instanceName string) models.PgCoreDashboardCache {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.pgDashboardCache[instanceName]
}

func (s *MetricsService) GetCachedPgThroughputDashboard(instanceName string, databaseName string) models.PgThroughputDashboardResponse {
	cache := s.GetCachedPgCoreDashboard(instanceName)

	n := models.MaxPgThroughputHistoryMinutes
	labels := make([]string, n)
	for i := 0; i < n; i++ {
		labels[i] = "-" + strconv.Itoa(n-1-i) + "m"
	}
	labels[n-1] = "Now"

	resp := models.PgThroughputDashboardResponse{
		InstanceName: cache.InstanceName,
		DatabaseName: databaseName,
		Timestamp:    cache.Timestamp,
		Labels:       labels,
		Tps:          make([]float64, n),
		CacheHitPct:  make([]float64, n),
	}

	historyLen := 0
	if databaseName != "all" {
		if points := cache.HistoryByDB[databaseName]; points != nil {
			historyLen = len(points)
		}
	} else {
		for _, points := range cache.HistoryByDB {
			historyLen = len(points)
			break
		}
	}

	if historyLen == 0 {
		return resp
	}

	offset := n - historyLen
	if offset < 0 {
		offset = 0
	}

	if databaseName == "all" {
		for outIdx := 0; outIdx < n; outIdx++ {
			cacheIdx := outIdx - offset
			if cacheIdx < 0 || cacheIdx >= historyLen {
				continue
			}

			var sumTxn int64
			var sumRead int64
			var sumHit int64

			for _, points := range cache.HistoryByDB {
				if cacheIdx >= len(points) {
					continue
				}
				p := points[cacheIdx]
				sumTxn += p.TxnDelta
				sumRead += p.BlksReadDelta
				sumHit += p.BlksHitDelta
			}

			resp.Tps[outIdx] = float64(sumTxn) / 60.0
			denom := sumHit + sumRead
			if denom > 0 {
				resp.CacheHitPct[outIdx] = (float64(sumHit) / float64(denom)) * 100.0
			}
		}

		return resp
	}

	points := cache.HistoryByDB[databaseName]
	if points == nil {
		return resp
	}

	for outIdx := 0; outIdx < n; outIdx++ {
		cacheIdx := outIdx - offset
		if cacheIdx < 0 || cacheIdx >= historyLen {
			continue
		}
		p := points[cacheIdx]
		resp.Tps[outIdx] = p.Tps
		resp.CacheHitPct[outIdx] = p.CacheHitPct
	}

	return resp
}

func (s *MetricsService) fetchAndLogCPUSchedulerStatsWithContext(ctx context.Context, instanceName string) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	db, ok := s.MsRepo.GetDB(instanceName)
	if !ok || db == nil {
		return fmt.Errorf("no connection available for %s", instanceName)
	}

	stats, err := s.MsRepo.CollectCPUSchedulerStats(ctx, db)
	if err != nil {
		return fmt.Errorf("CollectCPUSchedulerStats: %w", err)
	}

	statsMap := map[string]interface{}{
		"max_workers_count":                  stats.MaxWorkersCount,
		"scheduler_count":                    stats.SchedulerCount,
		"cpu_count":                          stats.CPUCount,
		"total_runnable_tasks_count":         stats.TotalRunnableTasksCount,
		"total_work_queue_count":             stats.TotalWorkQueueCount,
		"total_current_workers_count":        stats.TotalCurrentWorkersCount,
		"avg_runnable_tasks_count":           stats.AvgRunnableTasksCount,
		"total_active_request_count":         stats.TotalActiveRequestCount,
		"total_queued_request_count":         stats.TotalQueuedRequestCount,
		"total_blocked_task_count":           stats.TotalBlockedTaskCount,
		"total_active_parallel_thread_count": stats.TotalActiveParallelThreadCount,
		"runnable_request_count":             stats.RunnableRequestCount,
		"total_request_count":                stats.TotalRequestCount,
		"runnable_percent":                   stats.RunnablePercent,
		"worker_thread_exhaustion_warning":   stats.WorkerThreadExhaustionWarning,
		"runnable_tasks_warning":             stats.RunnableTasksWarning,
		"blocked_tasks_warning":              stats.BlockedTasksWarning,
		"queued_requests_warning":            stats.QueuedRequestsWarning,
		"total_physical_memory_kb":           stats.TotalPhysicalMemoryKB,
		"available_physical_memory_kb":       stats.AvailablePhysicalMemoryKB,
		"system_memory_state_desc":           stats.SystemMemoryStateDesc,
		"physical_memory_pressure_warning":   stats.PhysicalMemoryPressureWarning,
		"total_node_count":                   stats.TotalNodeCount,
		"nodes_online_count":                 stats.NodesOnlineCount,
		"offline_cpu_count":                  stats.OfflineCPUCount,
		"offline_cpu_warning":                stats.OfflineCPUWarning,
	}

	if err := s.tsLogger.LogCPUSchedulerStats(ctx, instanceName, statsMap); err != nil {
		return fmt.Errorf("LogCPUSchedulerStats: %w", err)
	}

	log.Printf("[Collector] Successfully logged CPU Scheduler stats for %s", instanceName)
	return nil
}

func (s *MetricsService) fetchAndLogServerPropertiesWithContext(ctx context.Context, instanceName string) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	db, ok := s.MsRepo.GetDB(instanceName)
	if !ok || db == nil {
		return fmt.Errorf("no connection available for %s", instanceName)
	}

	props, err := s.MsRepo.CollectServerProperties(ctx, db)
	if err != nil {
		return fmt.Errorf("CollectServerProperties: %w", err)
	}

	propsMap := map[string]interface{}{
		"cpu_count":           props.CPUCount,
		"hyperthread_ratio":   props.HyperthreadRatio,
		"socket_count":        props.SocketCount,
		"cores_per_socket":    props.CoresPerSocket,
		"physical_memory_gb":  props.PhysicalMemoryGB,
		"virtual_memory_gb":   props.VirtualMemoryGB,
		"cpu_type":            props.CPUType,
		"hyperthread_enabled": props.HyperthreadEnabled,
		"numa_nodes":          props.NUMANodes,
		"max_workers_count":   props.MaxWorkersCount,
		"properties_hash":     props.PropertiesHash,
	}

	if err := s.tsLogger.LogServerProperties(ctx, instanceName, propsMap); err != nil {
		log.Printf("[Collector] Warning: failed to log server properties for %s: %v", instanceName, err)
	} else {
		log.Printf("[Collector] Successfully logged Server Properties for %s", instanceName)
	}
	return nil
}
