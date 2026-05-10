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

	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/collectors/pghostcpu"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
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

func (s *MetricsService) FetchInterval(ctx context.Context, name string, defaultVal time.Duration) time.Duration {
	if pool := s.GetTimescaleDBPool(); pool != nil {
		repo := repository.NewCollectorConfigRepository(pool)
		cfg, err := repo.GetByName(ctx, name)
		if err == nil && cfg != nil && cfg.IsActive {
			return time.Duration(cfg.FrequencySeconds) * time.Second
		}
	}
	return defaultVal
}

func (s *MetricsService) StartBackgroundCollector(ctx context.Context) {
	// Wait 20 seconds before starting initial collection to allow system to stabilize
	log.Printf("[Collector] Background daemon standby... (waiting 20s for system stabilization)")
	select {
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
		return
	}

	msHistInterval := s.FetchInterval(ctx, "SQL Server System Metrics", 60*time.Second)
	pgHistInterval := s.FetchInterval(ctx, "Postgres CPU and Memory", 60*time.Second)

	log.Printf("[Collector] Split-Speed Background Daemon starting...")
	log.Printf("[Collector]   - SQLServer Historical ticker: every %v", msHistInterval)
	log.Printf("[Collector]   - Postgres Historical ticker: every %v", pgHistInterval)
	log.Printf("[Collector]   - Live Diagnostics ticker: Adaptive (respects table configuration)")
	log.Printf("[Collector]   - PG Locks/Blocking incidents: adaptive 15s/5s (configurable)")

	s.dashboardCache = make(map[string]models.DashboardMetrics)
	s.pgDashboardCache = make(map[string]models.PgCoreDashboardCache)

	// Run one initial scrape of everything to ensure data is present on startup
	log.Printf("[Collector] Performing initial background collection...")
	s.runLiveDiagnosticsWithContext(ctx)
	s.runHistoricalStorageWithContext(ctx)

	// We use a dynamic base ticker for the loop, default 15s
	tickerInterval := s.FetchInterval(ctx, "Base Collector Ticker", 15*time.Second)
	loopTicker := time.NewTicker(tickerInterval)
	defer loopTicker.Stop()

	msLiveInterval := s.FetchInterval(ctx, "SQL Server Active Queries", 15*time.Second)
	pgLiveInterval := s.FetchInterval(ctx, "Postgres Active Queries", 60*time.Second)

	lastMsHist := time.Now()
	lastPgHist := time.Now()
	lastMsLive := time.Now()
	lastPgLive := time.Now()
	lastIdentityUpsert := time.Now()

	// Start stateful incident monitoring in the background
	go s.StartPgLocksBlockingCollector(ctx)
	go s.StartMsLocksBlockingCollector(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Collector] Background daemon shutting down")
			return

		case <-loopTicker.C:
			now := time.Now()

			// Check SQL Server Identity Upsert (every 5 minutes)
			if now.Sub(lastIdentityUpsert) >= 5*time.Minute {
				if s.tsLogger != nil {
					if err := s.tsLogger.RunSQLServerIdentityUpsertJob(ctx); err != nil {
						log.Printf("[Collector] ERROR: RunSQLServerIdentityUpsertJob failed: %v", err)
					} else {
						log.Printf("[Collector] Successfully completed SQL Server Identity Upsert Job")

						// Immediately run Classification Job after Identity Upsert
						if err := s.tsLogger.RunSQLServerClassificationJob(ctx); err != nil {
							log.Printf("[Collector] ERROR: RunSQLServerClassificationJob failed: %v", err)
						} else {
							log.Printf("[Collector] Successfully completed SQL Server Query Classification Job")
						}
					}
				}
				lastIdentityUpsert = now
			}

			// Check SQL Server Historical
			if now.Sub(lastMsHist) >= msHistInterval {
				s.runHistoricalStorageWithContext(ctx) 
				lastMsHist = now
			}

			// Check Postgres Historical
			if now.Sub(lastPgHist) >= pgHistInterval {
				// We currently reuse runHistoricalStorageWithContext but it handles both.
				// In a future refactor, we should split them.
				if now.Sub(lastMsHist) >= 1*time.Second { // avoid double-run if intervals match
				    s.runHistoricalStorageWithContext(ctx)
				}
				lastPgHist = now
			}

			// Check SQL Server Live Diagnostics
			if now.Sub(lastMsLive) >= msLiveInterval {
				s.runLiveDiagnosticsWithContext(ctx)
				lastMsLive = now
			}

			// Check Postgres Live Diagnostics
			if now.Sub(lastPgLive) >= pgLiveInterval {
				s.runLiveDiagnosticsWithContext(ctx)
				lastPgLive = now
			}

			// Refresh all intervals for next iteration
			msHistInterval = s.FetchInterval(ctx, "SQL Server System Metrics", 60*time.Second)
			pgHistInterval = s.FetchInterval(ctx, "Postgres CPU and Memory", 60*time.Second)
			msLiveInterval = s.FetchInterval(ctx, "SQL Server Active Queries", 15*time.Second)
			pgLiveInterval = s.FetchInterval(ctx, "Postgres Active Queries", 60*time.Second)

			// Refresh base ticker if it changed
			newTickerInterval := s.FetchInterval(ctx, "Base Collector Ticker", 15*time.Second)
			if newTickerInterval != tickerInterval {
				tickerInterval = newTickerInterval
				loopTicker.Reset(tickerInterval)
			}
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
				s.EnqueueCollection(instanceName, func() {
					currentMs := s.MsRepo.FetchLiveTelemetry(instanceName, prevMsTick)
					currentMs.Timestamp = time.Now().Format("15:04:05")

					// Persist session snapshots if Timescale is connected
					if s.tsLogger != nil && len(currentMs.SessionSnapshots) > 0 {
						// Add instance ID to snapshots
						for i := range currentMs.SessionSnapshots {
							currentMs.SessionSnapshots[i].InstanceID = instanceName
						}
						if err := s.tsLogger.LogSQLServerSessionSnapshots(ctx, currentMs.SessionSnapshots); err != nil {
							log.Printf("[Collector] ERROR: LogSQLServerSessionSnapshots failed for %s: %v", instanceName, err)
						}
					}

					s.cacheMutex.Lock()
					s.dashboardCache[instanceName] = currentMs
					s.cacheMutex.Unlock()

					slog.Info("collector_live_scrape",
						"instance", instanceName,
						"engine", instanceType,
						"duration_ms", time.Since(t0).Milliseconds(),
					)
				})
			} else if instanceType == "postgres" {
				s.EnqueueCollection(instanceName, func() {
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
				})
			}
		}(inst.Name, inst.Type)
	}

	wg.Wait()
}

func (s *MetricsService) runLiveDiagnosticsForInstance(ctx context.Context, instanceName string) {
	var instanceType string
	for _, inst := range s.Config.Instances {
		if strings.EqualFold(inst.Name, instanceName) {
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

		// Populate Top Queries from TimescaleDB (sqlserver_query_metrics_v2) instead of direct DMV
		if s.tsLogger != nil {
			top, err := s.tsLogger.GetSqlServerTopQueriesFromInterval(ctx, instanceName, "cpu", 20, 1, true)
			if err == nil {
				for _, r := range top {
					currentMs.TopQueries = append(currentMs.TopQueries, models.QueryStat{
						LoginName:      r.LoginName,
						ProgramName:    r.ApplicationName,
						DatabaseName:   r.DatabaseName,
						QueryText:      r.QueryText,
						WaitType:       "HISTORICAL",
						CPUTimeMs:      r.AvgCpuMs,
						ExecTimeMs:     r.AvgDurationMs,
						LogicalReads:   int64(r.AvgReads),
						ExecutionCount: r.Executions,
					})
				}
			}
		}

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
				s.EnqueueCollection(instanceName, func() {
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
				})

				slog.Info("collector_historical_scrape",
					"instance", instanceName,
					"engine", instanceType,
					"duration_ms", time.Since(t0).Milliseconds(),
				)
			} else if instanceType == "postgres" {
				s.EnqueueCollection(instanceName, func() {
					if s.tsLogger != nil {
						s.cacheMutex.RLock()
						pgCache := s.pgDashboardCache[instanceName]
						s.cacheMutex.RUnlock()

						if err := s.logPostgresMetricsToTimescaleWithContext(ctx, instanceName, pgCache); err != nil {
							log.Printf("[Collector] ERROR: Failed to log Postgres metrics to TimescaleDB for %s: %v", instanceName, err)
						} else {
							log.Printf("[Collector] Successfully logged Postgres metrics for %s to TimescaleDB", instanceName)
						}

						// Enhanced Memory Intelligence collection
						if err := s.CollectAndLogPgMemory(ctx, instanceName); err != nil {
							log.Printf("[Collector] ERROR: Enhanced PgMemory collection failed for %s: %v", instanceName, err)
						}
					}
				})

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

	// Log PLE to history table
	if currentMs.PLE >= 0 {
		_ = s.tsLogger.LogSQLServerMemoryHistory(ctx, instanceName, currentMs.PLE)
	}

	// Collect Database Throughput more frequently (every 60s) for accurate TPS
	s.collectDatabaseThroughputForInstance(instanceName)

	if len(currentMs.CPUHistory) > 0 {
		cpuTicks := make([]map[string]interface{}, len(currentMs.CPUHistory))
		for i, tick := range currentMs.CPUHistory {
			cpuTicks[i] = map[string]interface{}{
				"sql_process":   tick.SQLProcess,
				"system_idle":   tick.SystemIdle,
				"other_process": tick.OtherProcess,
				"event_time":    tick.EventTime,
			}
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

	// Storage & Index Health (delta stats)
	db, ok := s.MsRepo.GetConn(instanceName)
	if ok && db != nil {
		capture := time.Now().UTC()
		now := capture

		idxInterval := s.FetchInterval(ctx, "SQL Server Index Usage", 15*time.Minute)
		tblInterval := s.FetchInterval(ctx, "SQL Server Table Usage", 15*time.Minute)
		growthInterval := s.FetchInterval(ctx, "SQL Server Storage", 24*time.Hour)     // Change to Daily
		defInterval := s.FetchInterval(ctx, "SQL Server Configuration", 24*time.Hour) // Use 'SQL Server Configuration' for definitions

		s.sihMu.Lock()
		lastDefTime := s.sihLastDefsDaily[instanceName]
		s.sihMu.Unlock()
		due15mIndex := s.sihDue(s.sihLastIndex15m, instanceName, now, idxInterval)
		due15mTable := s.sihDue(s.sihLastTable15m, instanceName, now, tblInterval)
		dueDailyGrowth := s.sihDue(s.sihLastGrowth6h, instanceName, now, growthInterval)
		dueDailyDefs := s.sihDue(s.sihLastDefsDaily, instanceName, now, defInterval)

		// For each user DB configured, switch context and collect.
		// We intentionally scope to configured DB list to avoid accidental access to system DBs.
		var dbs []string
		for _, inst := range s.Config.Instances {
			if strings.EqualFold(inst.Name, instanceName) && inst.Type == "sqlserver" {
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
		if len(dbs) == 0 && (due15mIndex || due15mTable || dueDailyGrowth || dueDailyDefs) {
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
			if due15mTable || dueDailyGrowth {
				tblRows, err := collectors.CollectSQLServerTableSizeSnapshot(ctx, conn)
				if err == nil && len(tblRows) > 0 {
					if due15mTable {
						// table_usage_stats (sizes snapshot; scan counters are 0 for SQL Server)
						_, _ = collectors.PersistSQLServerTableUsageDeltas(ctx, s.tsLogger, instanceName, tblRows, capture)
					}
					if dueDailyGrowth {
						// table_size_history (growth snapshot)
						_, _ = collectors.PersistSQLServerTableGrowthHistory(ctx, s.tsLogger, instanceName, tblRows, capture)
					}
				}
			}

			// Index definitions snapshot (daily cadence).
			if dueDailyDefs {
				dayBucket := time.Date(capture.Year(), capture.Month(), capture.Day(), 0, 0, 0, 0, time.UTC)
				defRows, err := collectors.CollectSQLServerIndexDefinitions(ctx, conn, lastDefTime)
				if err == nil && len(defRows) > 0 {
					// Using the daily check but change-detection in persist helper (if we had one) 
					// or just stick to the daily collect as is since SQL Server CollectSQLServerIndexDefinitions 
					// already has some 'since' time filtering.
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
			// Don't return here, continue with other stats even if connection stats fail
		} else {
		    log.Printf("[Collector] Successfully logged Postgres connection stats for %s", instanceName)
		}
	}

	memPct := 0.0
	hostPct := 0.0
	if detail, derr := s.PgRepo.GetSystemStatsDetail(instanceName); derr == nil && detail != nil {
		hostPct = detail.CPUUsagePct
		memPct = detail.MemoryUsedPct
		log.Printf("[Collector] Using detailed system stats for %s: CPU=%.1f%%, MEM=%.1f%%", instanceName, hostPct, memPct)
	} else {
		cu, mu, err := s.PgRepo.GetSystemStats(instanceName)
		if err != nil {
			log.Printf("[Collector] system stats for %s: %v", instanceName, err)
		} else {
			hostPct = cu
			memPct = mu
			log.Printf("[Collector] Using approximated system stats for %s: CPU=%.1f%% (fallback)", instanceName, hostPct)
		}
	}
	snap := pghostcpu.Collect()
	if snap.HostCpuPercent > 0 {
		hostPct = snap.HostCpuPercent
		log.Printf("[Collector] Using host probe CPU for %s: %.1f%%", instanceName, hostPct)
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
	} else {
	    log.Printf("[Collector] Successfully logged Postgres system stats for %s", instanceName)
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
			OperationalState:   ag.OperationalState,
			ConnectedState:     ag.ConnectedState,
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
				"category":        j.Category,
				"description":     j.Description,
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
		"active_workers_count":               stats.ActiveWorkersCount,
		"pending_disk_io_count":              stats.PendingDiskIoCount,
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
