// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Query Store statistics collection and persistence worker.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_monitoring/instance_health"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
	"github.com/rsharma155/sql_optima/pkg/dashboard"
)

// StartQueryStoreCollector starts a background worker that extracts Query Store data
func (s *MetricsService) StartQueryStoreCollector(ctx context.Context) {
	interval := s.FetchInterval(ctx, "SQL Server Query Store", config.QueryStoreCollectionInterval)
	log.Printf("[QueryStoreCollector] Starting Query Store collector (interval: %v)...", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	s.collectQueryStoreData()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[QueryStoreCollector] Shutting down Query Store collector")
			return
		case <-ticker.C:
			s.collectQueryStoreData()

			// Refresh interval from DB
			newInterval := s.FetchInterval(ctx, "SQL Server Query Store", config.QueryStoreCollectionInterval)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
			}
		}
	}
}

func (s *MetricsService) collectQueryStoreData() {
	if s.tsLogger == nil {
		log.Printf("[QueryStoreCollector] WARNING: TimescaleDB not connected, skipping collection")
		return
	}

	var wg sync.WaitGroup

	for _, inst := range s.Config.Instances {
		if inst.Type != "sqlserver" {
			continue
		}

		wg.Add(1)
		go func(instanceName string) {
			defer wg.Done()
			s.EnqueueCollection(instanceName, func() {
				s.collectQueryStoreForInstance(instanceName)
			})
		}(inst.Name)
	}

	wg.Wait()
}

func (s *MetricsService) collectQueryStoreForInstance(instanceName string) {
	t0 := time.Now()

	// Fetch Query Store data from SQLSERVER using the repository
	queryStats, err := s.MsRepo.FetchQueryStoreStats(instanceName)
	if err != nil {
		log.Printf("[QueryStoreCollector] ERROR: Failed to fetch Query Store data for %s: %v", instanceName, err)
		return
	}

	if len(queryStats) == 0 {
		log.Printf("[QueryStoreCollector] No Query Store data found for %s (Query Store may not be enabled)", instanceName)
		return
	}

	// Convert to the format expected by ts_logger
	timestamp := time.Now().UTC()
	rows := make([]hot.QueryStoreStatsRow, len(queryStats))

	for i, qs := range queryStats {
		dbn := strings.TrimSpace(qs.DatabaseName)
		if dbn == "" {
			dbn = "unknown"
		}
		rows[i] = hot.QueryStoreStatsRow{
			CaptureTimestamp:  timestamp,
			ServerName:        instanceName,
			DatabaseName:      dbn,
			QueryHash:         qs.QueryHash,
			QueryText:         qs.QueryText,
			PlanID:            qs.PlanID,
			IntervalID:        qs.IntervalID,
			Executions:        qs.Executions,
			AvgDurationMs:     qs.AvgDurationMs,
			AvgCpuMs:          qs.AvgCpuMs,
			AvgLogicalReads:   qs.AvgLogicalReads,
			TotalCpuMs:        qs.TotalCpuMs,
			LastExecutionTime: qs.LastExecutionTime,
		}
	}

	// Insert into TimescaleDB
	ctx := context.Background()
	if err := s.tsLogger.LogQueryStoreStatsDirect(ctx, rows); err != nil {
		log.Printf("[QueryStoreCollector] ERROR: Failed to log Query Store data for %s: %v", instanceName, err)
		return
	}

	log.Printf("[QueryStoreCollector] Collected %d queries from %s in %v", len(queryStats), instanceName, time.Since(t0))
}

// GetQueryStoreBottlenecks returns aggregated Query Store statistics for bottlenecks analysis
func (s *MetricsService) GetQueryStoreBottlenecks(ctx context.Context, instanceName, timeRange string, limit int, database string, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetQueryStoreBottlenecks(ctx, instanceName, timeRange, limit, database, from, to)
}

// StartEnterpriseCollector starts the AG Health and Database Throughput collector
func (s *MetricsService) StartEnterpriseCollector(ctx context.Context) {
	interval := s.FetchInterval(ctx, "SQL Server Enterprise Metrics", config.QueryStoreCollectionInterval)
	log.Printf("[EnterpriseCollector] Starting AG Health & DB Throughput collector (interval: %v)...", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	s.collectEnterpriseMetrics()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[EnterpriseCollector] Shutting down AG Health & DB Throughput collector")
			return
		case <-ticker.C:
			s.collectEnterpriseMetrics()
			// Refresh interval
			newInterval := s.FetchInterval(ctx, "SQL Server Enterprise Metrics", config.QueryStoreCollectionInterval)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
			}
		}
	}
}

// StartEnterpriseMetricsCollector logs advanced DMV snapshots to TimescaleDB.
// This enables most non-realtime dashboards to read from TimescaleDB instead of querying DMVs per-request.
func (s *MetricsService) StartEnterpriseMetricsCollector(ctx context.Context) {
	if s.tsLogger == nil {
		return
	}

	interval := s.FetchInterval(ctx, "SQL Server Enterprise Metrics", 120*time.Second)
	log.Printf("[EnterpriseMetricsCollector] Starting advanced metrics collector (interval: %v)...", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	s.collectAdvancedEnterpriseMetrics()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[EnterpriseMetricsCollector] Shutting down advanced metrics collector")
			return
		case <-ticker.C:
			s.collectAdvancedEnterpriseMetrics()
			// Refresh interval
			newInterval := s.FetchInterval(ctx, "SQL Server Enterprise Metrics", 120*time.Second)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
			}
		}
	}
}

func (s *MetricsService) collectAdvancedEnterpriseMetrics() {
	if s.tsLogger == nil {
		return
	}
	var wg sync.WaitGroup
	for _, inst := range s.Config.Instances {
		if inst.Type != "sqlserver" {
			continue
		}
		wg.Add(1)
		go func(instanceName string) {
			defer wg.Done()
			s.collectAdvancedEnterpriseMetricsForInstance(instanceName)
		}(inst.Name)
	}
	wg.Wait()
}

func (s *MetricsService) collectAdvancedEnterpriseMetricsForInstance(instanceName string) {
	s.EnqueueCollection(instanceName, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Increased timeout for sequential execution
		defer cancel()

		// Memory analyzer (Timescale-backed drilldown)
	if row, err := s.MsRepo.FetchMemoryAnalyzerSnapshot(ctx, instanceName); err == nil {
		_ = s.tsLogger.LogSQLServerMemoryMetrics(ctx, instanceName, row)
	}
	if rows, err := s.MsRepo.FetchBufferPoolByDB(ctx, instanceName, 20); err == nil {
		_ = s.tsLogger.LogSQLServerBufferPoolByDB(ctx, instanceName, rows)
	}

	if rows, err := s.MsRepo.FetchLatchStats(instanceName); err == nil {
		_ = s.tsLogger.LogLatchWaits(ctx, instanceName, rows)
	}
	if rows, err := s.MsRepo.FetchWaitingTasks(instanceName); err == nil {
		_ = s.tsLogger.LogWaitingTasks(ctx, instanceName, rows)
	}
	if rows, err := s.MsRepo.FetchMemoryGrants(instanceName); err == nil {
		_ = s.tsLogger.LogMemoryGrants(ctx, instanceName, rows)
	}
	if row, err := s.MsRepo.FetchPlanCacheHealth(instanceName); err == nil {
		_ = s.tsLogger.LogPlanCacheHealth(ctx, instanceName, row)
	}
	if rows, err := s.MsRepo.FetchMemoryGrantWaiters(instanceName); err == nil {
		_ = s.tsLogger.LogMemoryGrantWaiters(ctx, instanceName, rows)
	}
	if rows, err := s.MsRepo.FetchProcedureStats(instanceName); err == nil {
		_ = s.tsLogger.LogProcedureStats(ctx, instanceName, rows)
	}
	if rows, err := s.MsRepo.FetchFileIOLatency(instanceName); err != nil {
		log.Printf("[EnterpriseMetricsCollector] FetchFileIOLatency failed for %s: %v", instanceName, err)
	} else if err := s.tsLogger.LogFileIOLatency(ctx, instanceName, rows); err != nil {
		log.Printf("[EnterpriseMetricsCollector] LogFileIOLatency failed for %s: %v", instanceName, err)
	}
	if rows, err := s.MsRepo.FetchSpinlockStats(instanceName); err == nil {
		_ = s.tsLogger.LogSpinlockStats(ctx, instanceName, rows)
	}
	if rows, err := s.MsRepo.FetchMemoryClerks(instanceName); err == nil {
		_ = s.tsLogger.LogMemoryClerks(ctx, instanceName, rows)
	}
	// TempDB endpoint expects file-level stats; log into sqlserver_tempdb_files.
	if rows, err := s.MsRepo.FetchTempdbStats(instanceName); err == nil {
		_ = s.tsLogger.LogTempdbFiles(ctx, instanceName, rows)
	}
	if rows, err := s.MsRepo.FetchTempdbTopConsumers(instanceName); err == nil {
		_ = s.tsLogger.LogTempdbTopConsumers(ctx, instanceName, rows)
	}

	// Scheduler/workload group stats (Resource Governor)
	if rows, err := s.MsRepo.FetchSchedulerWG(instanceName); err == nil {
		_ = s.tsLogger.LogSchedulerWG(ctx, instanceName, rows)
	}

	// -------------------------
	// Phase 2 DBA homepage data
	// -------------------------
	// These are bounded, small queries intended for Timescale-backed homepage widgets.
	blocking, _ := s.MsRepo.FetchBlockingSessionsCount(ctx, instanceName)
	grantsPending, _ := s.MsRepo.FetchMemoryGrantsPending(ctx, instanceName)
	tempdbPct, _ := s.MsRepo.FetchTempdbUsagePercent(ctx, instanceName)
	failedLogins5m, _ := s.MsRepo.FetchFailedLoginsLast5Min(ctx, instanceName)

	// Perf counters
	counters, _ := s.MsRepo.FetchPerfCounters(ctx, instanceName, []string{
		"Batch Requests/sec",
		"SQL Compilations/sec",
		"Page life expectancy",
		"Buffer cache hit ratio",
		"Buffer cache hit ratio base",
	})
	
	// Compute deltas for cumulative counters
	rates := s.tsLogger.ComputePerfCounterDeltas(instanceName, counters)
	
	batch := rates["Batch Requests/sec"]
	comp := rates["SQL Compilations/sec"]
	ple := rates["Page life expectancy"]

	bchr := 0.0
	// Buffer cache hit ratio base is instantaneous (usually), but the ratio itself 
	// should be calculated from the raw values if we want precision, 
	// or just take the pre-calculated one if available.
	// Actually BCHR base is often used with the numerator.
	rawBase := counters["Buffer cache hit ratio base"].Value
	rawRatio := counters["Buffer cache hit ratio"].Value
	if rawBase > 0 {
		bchr = (rawRatio / rawBase) * 100.0
	}

	// Max log usage %
	maxLog := ""
	maxLogPct := 0.0
	if lu, err := s.MsRepo.FetchMaxDBLogUsagePercent(ctx, instanceName); err == nil {
		maxLog = lu.DatabaseName
		maxLogPct = lu.UsedPercent
	}

	_ = s.tsLogger.LogSQLServerRiskHealth(ctx, instanceName, hot.RiskHealthRow{
		CaptureTimestamp:    time.Now().UTC(),
		ServerInstanceName:  instanceName,
		BlockingSessions:    blocking,
		MemoryGrantsPending: grantsPending,
		FailedLogins5m:      failedLogins5m,
		TempdbUsedPercent:   tempdbPct,
		MaxLogDbName:        maxLog,
		MaxLogUsedPercent:   maxLogPct,
		PLE:                 ple,
		CompilationsPerSec:  comp,
		BatchReqPerSec:      batch,
		BufferCacheHitPct:   bchr,
	})

	// Also log to separate history table for charts
	_ = s.tsLogger.LogSQLServerMemoryHistory(ctx, instanceName, ple)

	// Wait deltas + categorization
	if currWaits, err := s.MsRepo.FetchWaitStatsCumulative(ctx, instanceName); err == nil {
		// Legacy v1 delta logging (categorized)
		_ = s.tsLogger.ComputeAndLogWaitDeltas(ctx, instanceName, currWaits)

		// New standardized v2 delta logging
		prevWaits := s.tsLogger.GetPrevWaitHistory(instanceName)
		deltas, _ := dashboard.ComputeWaitDeltas(prevWaits, currWaits)
		v2Rows := make([]map[string]interface{}, 0, len(deltas))
		for _, d := range deltas {
			if d.DeltaMs > 0 {
				v2Rows = append(v2Rows, map[string]interface{}{
					"wait_category":             string(d.Category),
					"wait_time_ms_delta":        d.DeltaMs,
					"signal_wait_time_ms_delta": 0, // Need to update MsRepo to fetch signal delta
					"waiting_tasks_delta":       0,
				})
			}
		}
		_ = s.tsLogger.LogSqlServerWaitStats(ctx, instanceName, v2Rows)
	}

	// Standardized V2 - Perf Counters
	if perfMap, err := s.MsRepo.FetchPerfCounters(ctx, instanceName, []string{
		"Batch Requests/sec", "SQL Compilations/sec", "SQL Re-Compilations/sec", "Latch Waits/sec",
	}); err == nil {
		v2Counters := make(map[string]float64)
		for k, v := range perfMap {
			v2Counters[k] = v.Value
		}
		_ = s.tsLogger.LogSqlServerPerfCounters(ctx, instanceName, v2Counters)
	}

	// Standardized V2 - File IO
	if ioRows, err := s.MsRepo.FetchFileIOLatency(instanceName); err == nil {
		_ = s.tsLogger.LogSqlServerFileIO(ctx, instanceName, ioRows)
	}

	// Standardized V2 - Plan Cache
	if pcRows, err := s.MsRepo.FetchPlanCacheHealth(instanceName); err == nil {
		// Map the single row into the standardized format
		v2Cache := []map[string]interface{}{
			{"cache_type": "Adhoc", "size_mb": pcRows["adhoc_cache_mb"]},
			{"cache_type": "Prepared", "size_mb": pcRows["prepared_cache_mb"]},
			{"cache_type": "Proc", "size_mb": pcRows["proc_cache_mb"]},
		}
		_ = s.tsLogger.LogSqlServerPlanCache(ctx, instanceName, v2Cache)
	}

	// Standardized V2 - Memory Clerks
	if mcRows, err := s.MsRepo.FetchMemoryClerks(instanceName); err == nil {
		v2Clerks := make([]map[string]interface{}, 0, len(mcRows))
		for _, r := range mcRows {
			v2Clerks = append(v2Clerks, map[string]interface{}{
				"clerk_name": r["clerk_name"],
				"pages_mb":   r["pages_mb"],
			})
		}
		_ = s.tsLogger.LogSqlServerMemoryClerksV2(ctx, instanceName, v2Clerks)
	}

	// Standardized V2 - Memory Grants
	if mgSum, err := s.MsRepo.FetchMemoryGrantsSummary(ctx, instanceName); err == nil {
		_ = s.tsLogger.LogSqlServerMemoryGrantsV2(ctx, instanceName, mgSum)
	}

	// Standardized V2 - TempDB Consumers
	if tdcRows, err := s.MsRepo.FetchTempdbTopConsumers(instanceName); err == nil {
		_ = s.tsLogger.LogSqlServerTempdbConsumers(ctx, instanceName, tdcRows)
	}
	})
}

func (s *MetricsService) collectEnterpriseMetrics() {
	if s.tsLogger == nil {
		return
	}

	var wg sync.WaitGroup

	for _, inst := range s.Config.Instances {
		if inst.Type != "sqlserver" {
			continue
		}

		wg.Add(1)
		go func(instanceName string) {
			defer wg.Done()
			s.EnqueueCollection(instanceName, func() {
				s.collectAGHealthForInstance(instanceName)
				s.collectDatabaseThroughputForInstance(instanceName)
				s.collectJobsForInstance(instanceName)
				s.collectLogShippingForInstance(instanceName)
			})
		}(inst.Name)
	}

	wg.Wait()
}

func (s *MetricsService) collectAGHealthForInstance(instanceName string) {
	t0 := time.Now()

	// Fetch AG health from SQLSERVER
	agStats, err := s.MsRepo.FetchAGHealthStats(instanceName)
	if err != nil {
		log.Printf("[EnterpriseCollector] AG Health error for %s: %v", instanceName, err)
		return
	}

	if len(agStats) == 0 {
		// No AG configured or not Enterprise edition
		return
	}

	// Convert to the format expected by ts_logger
	timestamp := time.Now().UTC()
	rows := make([]hot.AGHealthRow, len(agStats))

	for i, ag := range agStats {
		rows[i] = hot.AGHealthRow{
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
		}
	}

	// Insert into TimescaleDB
	ctx := context.Background()
	if err := s.tsLogger.LogAGHealth(ctx, instanceName, rows); err != nil {
		log.Printf("[EnterpriseCollector] AG Health insert error for %s: %v", instanceName, err)
		return
	}

	log.Printf("[EnterpriseCollector] Collected %d AG health records from %s in %v", len(agStats), instanceName, time.Since(t0))
}

func (s *MetricsService) collectDatabaseThroughputForInstance(instanceName string) {
	t0 := time.Now()

	// Fetch database throughput from SQLSERVER
	dbStats, err := s.MsRepo.FetchDatabaseThroughput(instanceName)
	if err != nil {
		log.Printf("[EnterpriseCollector] DB Throughput error for %s: %v", instanceName, err)
		return
	}

	if len(dbStats) == 0 {
		return
	}

	// Convert to the format expected by ts_logger
	timestamp := time.Now().UTC()
	rows := make([]hot.DatabaseThroughputRow, len(dbStats))

	for i, db := range dbStats {
		rows[i] = hot.DatabaseThroughputRow{
			CaptureTimestamp:    timestamp,
			ServerInstanceName:  instanceName,
			DatabaseName:        db.DatabaseName,
			UserSeeks:           db.UserSeeks,
			UserScans:           db.UserScans,
			UserLookups:         db.UserLookups,
			UserWrites:          db.UserWrites,
			TotalReads:          db.TotalReads,
			TotalWrites:         db.TotalWrites,
			TPS:                 db.TPS,
			BatchRequestsPerSec: db.BatchRequestsPerSec,
			Reads:               db.Reads,
			Writes:              db.Writes,
			BytesRead:           db.BytesRead,
			BytesWritten:        db.BytesWritten,
			ReadLatencyMs:       db.ReadLatencyMs,
			WriteLatencyMs:      db.WriteLatencyMs,
		}
	}

	// Insert into TimescaleDB
	ctx := context.Background()
	if err := s.tsLogger.LogDatabaseThroughput(ctx, instanceName, rows); err != nil {
		log.Printf("[EnterpriseCollector] DB Throughput insert error for %s: %v", instanceName, err)
		return
	}

	log.Printf("[EnterpriseCollector] Collected DB throughput for %d databases from %s in %v", len(dbStats), instanceName, time.Since(t0))
}

// GetAGHealthSummary returns AG health summary
func (s *MetricsService) GetAGHealthSummary(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetAGHealthSummary(ctx, instanceName, "", "", limit)
}

// GetJobsFromTimescale reconstructs a job view from hot storage.
// Returns empty metrics (with instance/timestamp) when no recent data is available.
func (s *MetricsService) GetJobsFromTimescale(ctx context.Context, instanceName string, from, to string) (models.JobMetrics, error) {
	var metrics models.JobMetrics
	metrics.InstanceName = instanceName
	metrics.Timestamp = time.Now().UTC().Format(time.RFC3339)

	if s.tsLogger == nil {
		return metrics, fmt.Errorf("timescale logger not initialized")
	}

	tsMetrics, err := s.tsLogger.GetSQLServerJobMetrics(ctx, instanceName, from, to, 1)
	if err != nil {
		return metrics, err
	}

	if len(tsMetrics) > 0 {
		m := tsMetrics[0]
		metrics.Summary = models.JobSummary{
			TotalJobs:    getInt(m, "total_jobs"),
			EnabledJobs:  getInt(m, "enabled_jobs"),
			DisabledJobs: getInt(m, "disabled_jobs"),
			RunningJobs:  getInt(m, "running_jobs"),
			FailedJobs:   getInt(m, "failed_jobs_24h"),
		}
		if t, ok := m["timestamp"].(time.Time); ok {
			metrics.Timestamp = t.Format(time.RFC3339)
		}
	}

	details, _ := s.tsLogger.GetSQLServerJobDetails(ctx, instanceName, from, to)
	for _, d := range details {
		metrics.Jobs = append(metrics.Jobs, models.JobDetail{
			JobName:       getStr(d, "job_name"),
			Category:      getStr(d, "category"),
			Description:   getStr(d, "description"),
			Enabled:       getBool(d, "enabled"),
			Owner:         getStr(d, "owner"),
			CreatedDate:   getStr(d, "created_date"),
			CurrentStatus: getStr(d, "current_status"),
			LastRunDate:   getInt(d, "last_run_date"),
			LastRunTime:   getInt(d, "last_run_time"),
			LastRunStatus: getStr(d, "last_run_status"),
		})
	}

	schedules, _ := s.tsLogger.GetSQLServerJobSchedules(ctx, instanceName, from, to)
	for _, sc := range schedules {
		var nextRun *string
		if nr, ok := sc["next_run_datetime"].(*string); ok {
			nextRun = nr
		}
		metrics.Schedules = append(metrics.Schedules, models.JobSchedule{
			JobName:         getStr(sc, "job_name"),
			JobEnabled:      getBool(sc, "job_enabled"),
			ScheduleName:    getStr(sc, "schedule_name"),
			Status:          getStr(sc, "status"),
			NextRunDateTime: nextRun,
		})
	}

	failures, _ := s.tsLogger.GetSQLServerJobFailures(ctx, instanceName, from, to, 100)
	for _, f := range failures {
		metrics.Failures = append(metrics.Failures, models.JobFailure{
			JobName:  getStr(f, "job_name"),
			StepName: getStr(f, "step_name"),
			Message:  getStr(f, "message"),
			RunDate:  getInt(f, "run_date"),
			RunTime:  getInt(f, "run_time"),
		})
	}

	return metrics, nil
}

// getInt, getStr, getBool helpers to extract values from maps.
func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// GetDatabaseThroughputSummary returns database throughput summary
func (s *MetricsService) GetDatabaseThroughputSummary(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetDatabaseThroughputSummary(ctx, instanceName, limit)
}

// =============================================================================
// PostgreSQL Enterprise Collectors
// =============================================================================

// StartPostgresEnterpriseCollector starts the PostgreSQL BGWriter, Archiver, and Query Dictionary collector
func (s *MetricsService) StartPostgresEnterpriseCollector(ctx context.Context) {
	interval := s.FetchInterval(ctx, "Postgres Query Stats", 60*time.Second)
	log.Printf("[PostgresEnterpriseCollector] Starting PostgreSQL enterprise collector (interval: %v)...", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	s.collectPostgresEnterpriseMetrics()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[PostgresEnterpriseCollector] Shutting down PostgreSQL enterprise collector")
			return
		case <-ticker.C:
			s.collectPostgresEnterpriseMetrics()
			// Refresh interval
			newInterval := s.FetchInterval(ctx, "Postgres Query Stats", 60*time.Second)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
			}
		}
	}
}

// safeRunCollector executes a collector function with panic recovery so a single
// failing collector does not prevent subsequent collectors from running.
func safeRunCollector(name, instanceName string, fn func(string)) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("collector_panic",
				"collector", name,
				"instance", instanceName,
				"panic", fmt.Sprintf("%v", r),
			)
		}
	}()
	fn(instanceName)
}

// collectPostgresEnterpriseMetrics collects PostgreSQL enterprise metrics for all PG instances
func (s *MetricsService) collectPostgresEnterpriseMetrics() {
	if s.tsLogger == nil {
		return
	}

	var wg sync.WaitGroup

	for _, inst := range s.Config.Instances {
		if inst.Type != "postgres" {
			continue
		}

		wg.Add(1)
		go func(instanceName string) {
			defer wg.Done()
			pgCollectors := []struct {
				name string
				fn   func(string)
			}{
				{"pg_snapshot", s.collectPostgresSnapshotForInstance},
				{"pg_bgwriter", s.collectPostgresBGWriterForInstance},
				{"pg_archiver", s.collectPostgresArchiverForInstance},				{"pg_wait_events", s.collectPostgresWaitEventsForInstance},
				{"pg_db_io", s.collectPostgresDbIOForInstance},
				{"pg_settings_snapshot", s.collectPostgresSettingsSnapshotForInstance},
				{"pg_query_dictionary", s.collectPostgresQueryDictionaryForInstance},
				{"pg_query_stats_snapshot", s.collectPostgresQueryStatsSnapshotForInstance},
				{"pg_control_center", s.collectPostgresControlCenterForInstance},
				{"pg_replication_slots", s.collectPostgresReplicationSlotsForInstance},
				{"pg_disk_stats", s.collectPostgresDiskStatsForInstance},
				{"pg_vacuum_progress", s.collectPostgresVacuumProgressForInstance},
				{"pg_table_maintenance", s.collectPostgresTableMaintenanceForInstance},
				{"pg_session_state_counts", s.collectPostgresSessionStateCountsForInstance},
				{"pg_pooler_stats", s.collectPostgresPoolerStatsForInstance},
				{"pg_deadlocks", s.collectPostgresDeadlocksForInstance},
				{"pg_incidents", s.CollectPgIncidents},
			}
			for _, c := range pgCollectors {
				safeRunCollector(c.name, instanceName, c.fn)
			}
		}(inst.Name)
	}

	wg.Wait()
}

func (s *MetricsService) collectPostgresTableMaintenanceForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}

	dbs, err := s.PgRepo.GetDatabases(instanceName)
	if err != nil || len(dbs) == 0 {
		return
	}

	for _, dbName := range dbs {
		// Avoid blocking on one bad DB
		dbCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		pgdb, derr := s.PgRepo.OpenConnForDatabase(dbCtx, instanceName, dbName)
		if derr != nil {
			cancel()
			continue
		}

		// Capture top N largest tables for this DB
		stats, err := s.PgRepo.GetTableMaintenanceStatsForDB(pgdb, 100)
		_ = pgdb.Close()
		cancel()

		if err != nil || len(stats) == 0 {
			continue
		}

		rows := make([]hot.PostgresTableMaintRow, 0, len(stats))
		for _, r := range stats {
			rows = append(rows, hot.PostgresTableMaintRow{
				CaptureTimestamp:   r.CaptureTimestamp,
				ServerInstanceName: instanceName,
				SchemaName:         r.SchemaName,
				TableName:          r.TableName,
				DatabaseName:       dbName,
				TotalBytes:         r.TotalBytes,
				LiveTuples:         r.LiveTuples,
				DeadTuples:         r.DeadTuples,
				DeadPct:            r.DeadPct,
				SeqScans:           r.SeqScans,
				IdxScans:           r.IdxScans,
				LastVacuum:         r.LastVacuum,
				LastAutovacuum:     r.LastAutovacuum,
				LastAnalyze:        r.LastAnalyze,
				LastAutoanalyze:    r.LastAutoanalyze,
			})
		}
		if len(rows) > 0 {
			_ = s.tsLogger.LogPostgresTableMaintenance(context.Background(), instanceName, rows)
		}
	}
}

func (s *MetricsService) collectPostgresVacuumProgressForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}
	rows, err := s.PgRepo.GetVacuumProgress(instanceName)
	if err != nil {
		// Not fatal; pg_stat_progress_vacuum may require permissions or be empty most of the time.
		return
	}
	if len(rows) == 0 {
		return
	}
	out := make([]hot.PostgresVacuumProgressRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, hot.PostgresVacuumProgressRow{
			CaptureTimestamp:   r.CaptureTimestamp,
			ServerInstanceName: instanceName,
			PID:                r.PID,
			DatabaseName:       r.DatabaseName,
			UserName:           r.UserName,
			RelationName:       r.RelationName,
			Phase:              r.Phase,
			HeapBlksTotal:      r.HeapBlksTotal,
			HeapBlksScanned:    r.HeapBlksScanned,
			HeapBlksVacuumed:   r.HeapBlksVacuumed,
			IndexVacuumCount:   r.IndexVacuumCount,
			MaxDeadTuples:      r.MaxDeadTuples,
			NumDeadTuples:      r.NumDeadTuples,
		})
	}
	_ = s.tsLogger.LogPostgresVacuumProgress(context.Background(), instanceName, out)
}

func computeUsedPct(total, free uint64) float64 {
	if total == 0 || free >= total {
		return 0
	}
	used := float64(total-free) / float64(total) * 100.0
	if used < 0 {
		return 0
	}
	if used > 100 {
		return 100
	}
	return used
}

func (s *MetricsService) collectPostgresDiskStatsForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}
	// Local-only: requires pg_disk_paths configured on the monitoring host.
	var diskPaths map[string]string
	for _, inst := range s.Config.Instances {
		if strings.ToUpper(inst.Name) == strings.ToUpper(instanceName) {
			diskPaths = inst.PGDiskPaths
			break
		}
	}
	if len(diskPaths) == 0 {
		return
	}

	type stat struct {
		mount string
		path  string
		total uint64
		free  uint64
		avail uint64
	}

	stats := make([]stat, 0, len(diskPaths))
	for mount, path := range diskPaths {
		total, free, avail, err := statfsBytes(path)
		if err != nil {
			log.Printf("[PostgresEnterpriseCollector] Disk statfs error for %s mount=%s path=%s: %v", instanceName, mount, path, err)
			continue
		}
		stats = append(stats, stat{mount: mount, path: path, total: total, free: free, avail: avail})
	}
	if len(stats) == 0 {
		return
	}

	rows := make([]hot.PostgresDiskStatRow, 0, len(stats))
	for _, st := range stats {
		rows = append(rows, hot.PostgresDiskStatRow{
			CaptureTimestamp:   time.Now().UTC(),
			ServerInstanceName: instanceName,
			MountName:          st.mount,
			Path:               st.path,
			TotalBytes:         int64(st.total),
			FreeBytes:          int64(st.free),
			AvailBytes:         int64(st.avail),
			UsedPct:            computeUsedPct(st.total, st.free),
		})
	}
	if err := s.tsLogger.LogPostgresDiskStats(context.Background(), instanceName, rows); err != nil {
		log.Printf("[PostgresEnterpriseCollector] Disk stats insert error for %s: %v", instanceName, err)
	}
}

func (s *MetricsService) collectPostgresReplicationSlotsForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}
	slots, err := s.PgRepo.GetReplicationSlotStats(instanceName)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] Replication slots error for %s: %v", instanceName, err)
		return
	}
	if len(slots) == 0 {
		return
	}
	rows := make([]hot.PostgresReplicationSlotRow, 0, len(slots))
	for _, s0 := range slots {
		rows = append(rows, hot.PostgresReplicationSlotRow{
			CaptureTimestamp:   s0.CaptureTimestamp,
			ServerInstanceName: instanceName,
			SlotName:           s0.SlotName,
			SlotType:           s0.SlotType,
			Active:             s0.Active,
			Temporary:          s0.Temporary,
			RetainedWalMB:      s0.RetainedWalMB,
			RestartLSN:         s0.RestartLSN,
			ConfirmedFlushLSN:  s0.ConfirmedFlushLSN,
			Xmin:               s0.Xmin,
			CatalogXmin:        s0.CatalogXmin,
		})
	}
	if err := s.tsLogger.LogPostgresReplicationSlots(context.Background(), instanceName, rows); err != nil {
		log.Printf("[PostgresEnterpriseCollector] Replication slots insert error for %s: %v", instanceName, err)
	}
}

func (s *MetricsService) collectPostgresSessionStateCountsForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}
	t0 := time.Now()
	cnt, err := s.PgRepo.GetSessionStateCounts(instanceName)
	if err != nil || cnt == nil {
		return
	}
	if err := s.tsLogger.LogPostgresSessionStateCounts(context.Background(), instanceName, cnt.Active, cnt.Idle, cnt.IdleInTxn, cnt.Waiting, cnt.Total); err != nil {
		log.Printf("[PostgresEnterpriseCollector] Session state counts insert error for %s: %v", instanceName, err)
		return
	}
	log.Printf("[PostgresEnterpriseCollector] Collected session state counts for %s in %v", instanceName, time.Since(t0))
}

func (s *MetricsService) collectPostgresPoolerStatsForInstance(instanceName string) {
	if s.tsLogger == nil || s.Config == nil {
		return
	}
	var dsn string
	for i := range s.Config.Instances {
		if strings.ToUpper(s.Config.Instances[i].Name) == strings.ToUpper(instanceName) {
			dsn = s.Config.Instances[i].PGBouncerAdminDSN
			break
		}
	}
	if strings.TrimSpace(dsn) == "" {
		return // not configured
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] PgBouncer connect error for %s: %v", instanceName, err)
		return
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, "SHOW POOLS")
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] PgBouncer SHOW POOLS error for %s: %v", instanceName, err)
		return
	}
	defer rows.Close()

	colIdx := map[string]int{}
	fds := rows.FieldDescriptions()
	for i, fd := range fds {
		colIdx[strings.ToLower(string(fd.Name))] = i
	}

	getInt := func(vals []any, name string) int {
		i, ok := colIdx[name]
		if !ok || i < 0 || i >= len(vals) {
			return 0
		}
		v := vals[i]
		switch t := v.(type) {
		case int32:
			return int(t)
		case int64:
			return int(t)
		case float64:
			return int(t)
		case []byte:
			n, _ := strconv.Atoi(string(t))
			return n
		case string:
			n, _ := strconv.Atoi(t)
			return n
		default:
			n, _ := strconv.Atoi(fmt.Sprintf("%v", v))
			return n
		}
	}
	getFloat := func(vals []any, name string) float64 {
		i, ok := colIdx[name]
		if !ok || i < 0 || i >= len(vals) {
			return 0
		}
		v := vals[i]
		switch t := v.(type) {
		case float32:
			return float64(t)
		case float64:
			return t
		case int32:
			return float64(t)
		case int64:
			return float64(t)
		case []byte:
			n, _ := strconv.ParseFloat(string(t), 64)
			return n
		case string:
			n, _ := strconv.ParseFloat(t, 64)
			return n
		default:
			n, _ := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
			return n
		}
	}

	var agg hot.PostgresPoolerStatRow
	agg.PoolerType = "pgbouncer"
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			continue
		}
		agg.ClActive += getInt(vals, "cl_active")
		agg.ClWaiting += getInt(vals, "cl_waiting")
		agg.SvActive += getInt(vals, "sv_active")
		agg.SvIdle += getInt(vals, "sv_idle")
		agg.SvUsed += getInt(vals, "sv_used")
		// PgBouncer usually returns maxwait in seconds.
		mw := getFloat(vals, "maxwait")
		if mw > agg.MaxwaitSeconds {
			agg.MaxwaitSeconds = mw
		}
		agg.TotalPools++
	}
	_ = rows.Err()

	// Only log if we actually got any pool rows.
	if agg.TotalPools <= 0 {
		return
	}
	if err := s.tsLogger.LogPostgresPoolerStats(context.Background(), instanceName, agg); err != nil {
		log.Printf("[PostgresEnterpriseCollector] Pooler stats insert error for %s: %v", instanceName, err)
	}
}

func (s *MetricsService) collectPostgresDeadlocksForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}
	rows, err := s.PgRepo.GetDeadlocksTotalByDB(instanceName)
	if err != nil || len(rows) == 0 {
		return
	}
	m := make(map[string]int64, len(rows))
	for _, r := range rows {
		if r.DatabaseName == "" {
			continue
		}
		m[r.DatabaseName] = r.DeadlocksTotal
	}
	if err := s.tsLogger.LogPostgresDeadlocksDelta(context.Background(), instanceName, m); err != nil {
		log.Printf("[PostgresEnterpriseCollector] Deadlocks insert error for %s: %v", instanceName, err)
	}
}

func (s *MetricsService) collectPostgresSnapshotForInstance(instanceName string) {
	if s.PgHealthRepo == nil || s.PgHealthService == nil {
		return
	}

	ctx := context.Background()

	// 1. Run legacy snapshot collector
	legacyCollector := collectors.NewPgSnapshotCollector(s.PgRepo, s.PgHealthService, s.PgHealthRepo, s.tsLogger)
	if err := legacyCollector.Collect(ctx, instanceName); err != nil {
		log.Printf("[PostgresEnterpriseCollector] legacy collect error for %s: %v", instanceName, err)
	}

	// 2. Run Phase 8 Timescale collector
	if s.PgTimescaleColl != nil {
		if err := s.PgTimescaleColl.Collect(ctx, instanceName); err != nil {
			log.Printf("[PostgresEnterpriseCollector] Phase 8 collect error for %s: %v", instanceName, err)
		}
	}
}

// ComputePostgresControlCenterRow builds a live control-center snapshot from PostgreSQL.
// WAL rate uses TimescaleLogger's WAL byte baseline when tsLogger is non-nil; otherwise it is 0.
// When a delta is not yet available (first sample), WAL rate is 0 so history/charts can still populate.
func (s *MetricsService) ComputePostgresControlCenterRow(instanceName string) (*hot.PostgresControlCenterRow, *models.PgReplicationStats, error) {
	if s.PgRepo == nil {
		return nil, nil, fmt.Errorf("pg repository not configured")
	}
	walBytes, err := s.PgRepo.FetchWalBytesTotal(instanceName)
	if err != nil {
		return nil, nil, err
	}
	walSizeMB, _ := s.PgRepo.FetchWalDirSizeMB(instanceName)

	repl, err := s.PgRepo.GetReplicationStats(instanceName)
	if err != nil {
		log.Printf("[MetricsService] ControlCenter replication (degraded) for %s: %v", instanceName, err)
		repl = &models.PgReplicationStats{}
	}

	bg, _ := s.PgRepo.FetchBGWriterStats(instanceName)
	checkpointReqRatio := 0.0
	if bg != nil {
		den := float64(bg.CheckpointsTimed)
		if den <= 0 {
			den = 1
		}
		checkpointReqRatio = float64(bg.CheckpointsReq) / den
	}

	obs, _ := s.PgRepo.FetchDBObservationMetrics(instanceName)
	var xidAge int64
	var xidPct float64
	if obs != nil {
		xidAge = obs.XIDAge
		xidPct = obs.XIDWraparoundPct
	}

	thr := s.GetCachedPgThroughputDashboard(instanceName, "all")
	tps := 0.0
	if n := len(thr.Tps); n > 0 {
		tps = thr.Tps[n-1]
	}
	active, waiting, _ := s.PgRepo.FetchActiveWaitingSessions(instanceName)
	slowCnt, _ := s.PgRepo.FetchSlowQueriesCount(instanceName, 500)
	blockingCnt, _ := s.PgRepo.FetchBlockingSessionsCount(instanceName)
	autovacCnt, _ := s.PgRepo.FetchAutovacuumWorkers(instanceName)
	deadTuplePct, _ := s.PgRepo.FetchDeadTupleRatioPct(instanceName)

	// v2 additions — 2026-05-07

	// Session state full breakdown (5-way, client backends only)
	sessState, _ := s.PgRepo.FetchSessionStateFull(instanceName)
	idleSessions, idleInTxn := 0, 0
	if sessState != nil {
		idleSessions = sessState.Idle
		idleInTxn = sessState.IdleInTxn + sessState.IdleInTxnAborted
	}

	// Connection utilization (max_connections vs used)
	connUtil, _ := s.PgRepo.FetchConnectionUtilization(instanceName)
	connMax, connUsed, connPct := 0, 0, 0.0
	if connUtil != nil {
		connMax = connUtil.MaxConnections
		connUsed = connUtil.UsedConnections
		connPct = connUtil.UsagePct
	}

	// Cache hit ratio (already computed via FetchCacheHitRatioPct — store it now)
	cacheHit, _ := s.PgRepo.FetchCacheHitRatioPct(instanceName)

	now := time.Now().UTC()
	intervalSec := 60.0
	if v := strings.TrimSpace(os.Getenv("POSTGRES_CONTROL_CENTER_INTERVAL_SEC")); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			intervalSec = float64(n)
		}
	}
	walRateMBMin := 0.0
	if s.tsLogger != nil {
		if r, ok := s.tsLogger.ComputeWalRateMBPerMin(instanceName, walBytes, intervalSec); ok {
			walRateMBMin = r
		}
	}

	// Deadlock rate — delta from cumulative counter across all databases
	dlTotal, _ := s.PgRepo.FetchDeadlocksTotalAllDBs(instanceName)
	dlPerMin := 0.0
	if s.tsLogger != nil {
		if r, ok := s.tsLogger.ComputePgDeadlockRate(instanceName, dlTotal, intervalSec); ok {
			dlPerMin = r
		}
	}

	row := hot.PostgresControlCenterRow{
		CaptureTimestamp:   now,
		ServerInstanceName: instanceName,
		WALMBPerMin:        walRateMBMin,
		WALSizeMB:          walSizeMB,
		ReplicaLagMB:       repl.MaxLagMB,
		ReplicaLagSec:      0,
		CheckpointReqRatio: checkpointReqRatio,
		XIDAge:             xidAge,
		XIDWraparoundPct:   xidPct,
		TPS:                tps,
		ActiveSessions:     active,
		WaitingSessions:    waiting,
		SlowQueriesCount:   slowCnt,
		BlockingSessions:   blockingCnt,
		AutovacuumWorkers:  autovacCnt,
		DeadTuplePct:       deadTuplePct,

		// v2 additions
		IdleSessions:        idleSessions,
		IdleInTxnSessions:   idleInTxn,
		ConnectionsMax:      connMax,
		ConnectionsUsed:     connUsed,
		ConnectionsUsagePct: connPct,
		CacheHitRatioPct:    cacheHit,
		DeadlocksPerMin:     dlPerMin,
	}
	if repl.WalGenRateMBps > 0 {
		row.ReplicaLagSec = row.ReplicaLagMB / repl.WalGenRateMBps
	}
	row.HealthScore, row.HealthStatus = hot.ComputePgHealthScore(hot.PgHealthInputs{
		ReplicationLagSeconds: row.ReplicaLagSec,
		XIDWraparoundPct:      row.XIDWraparoundPct,
		DeadTupleRatioPct:     row.DeadTuplePct,
		CheckpointReqRatio:    row.CheckpointReqRatio,
		WALRateMBPerMin:       row.WALMBPerMin,
		BlockingSessions:      row.BlockingSessions,
		// v2 additions
		CacheHitRatioPct:    row.CacheHitRatioPct,
		ConnectionsUsagePct: row.ConnectionsUsagePct,
		DeadlocksPerMin:     row.DeadlocksPerMin,
	})
	return &row, repl, nil
}

func (s *MetricsService) collectPostgresControlCenterForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}
	t0 := time.Now()
	row, repl, err := s.ComputePostgresControlCenterRow(instanceName)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] ControlCenter compute error for %s: %v", instanceName, err)
		return
	}
	now := row.CaptureTimestamp

	if err := s.tsLogger.LogPostgresControlCenterStats(context.Background(), *row); err != nil {
		log.Printf("[PostgresEnterpriseCollector] ControlCenter insert error for %s: %v", instanceName, err)
		return
	}

	// Store per-replica lag rows for the replication lag chart (only when primary has standbys).
	if repl != nil && repl.IsPrimary && len(repl.Standbys) > 0 {
		var lagRows []hot.PostgresReplicationLagDetailRow
		for _, st := range repl.Standbys {
			lagRows = append(lagRows, hot.PostgresReplicationLagDetailRow{
				CaptureTimestamp:   now,
				ServerInstanceName: instanceName,
				ReplicaName:        st.ReplicaPodName,
				LagMB:              st.ReplayLagMB,
				State:              st.State,
				SyncState:          st.SyncState,
			})
		}
		if err := s.tsLogger.LogPostgresReplicationLagDetail(context.Background(), lagRows); err != nil {
			log.Printf("[PostgresEnterpriseCollector] Replication lag detail insert error for %s: %v", instanceName, err)
		}
	}
	log.Printf("[PostgresEnterpriseCollector] Collected Control Center snapshot for %s in %v", instanceName, time.Since(t0))
}

// collectPostgresBGWriterForInstance collects BGWriter statistics for a PostgreSQL instance
func (s *MetricsService) collectPostgresBGWriterForInstance(instanceName string) {
	t0 := time.Now()

	// Fetch BGWriter stats from PostgreSQL
	bgStats, err := s.PgRepo.FetchBGWriterStats(instanceName)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] BGWriter error for %s: %v", instanceName, err)
		return
	}

	// Convert to the format expected by ts_logger
	row := hot.PostgresBGWriterRow{
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

	// Insert into TimescaleDB
	ctx := context.Background()
	if err := s.tsLogger.LogPostgresBGWriter(ctx, instanceName, row); err != nil {
		log.Printf("[PostgresEnterpriseCollector] BGWriter insert error for %s: %v", instanceName, err)
		return
	}

	log.Printf("[PostgresEnterpriseCollector] Collected BGWriter stats from %s in %v", instanceName, time.Since(t0))
}

// collectPostgresArchiverForInstance collects WAL Archiver statistics for a PostgreSQL instance
func (s *MetricsService) collectPostgresArchiverForInstance(instanceName string) {
	t0 := time.Now()

	// Fetch Archiver stats from PostgreSQL
	archStats, err := s.PgRepo.FetchArchiverStats(instanceName)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] Archiver error for %s: %v", instanceName, err)
		return
	}

	// Convert to the format expected by ts_logger
	row := hot.PostgresArchiverRow{
		CaptureTimestamp:   time.Now().UTC(),
		ServerInstanceName: instanceName,
		ArchivedCount:      archStats.ArchivedCount,
		FailedCount:        archStats.FailedCount,
		LastArchivedWal:    archStats.LastArchivedWal,
		LastFailedWal:      archStats.LastFailedWal,
		FailedCountDelta:   0, // Delta calculated on dashboard
	}

	// Insert into TimescaleDB
	ctx := context.Background()
	if err := s.tsLogger.LogPostgresArchiver(ctx, instanceName, row); err != nil {
		log.Printf("[PostgresEnterpriseCollector] Archiver insert error for %s: %v", instanceName, err)
		return
	}

	log.Printf("[PostgresEnterpriseCollector] Collected Archiver stats from %s in %v", instanceName, time.Since(t0))
}

func (s *MetricsService) collectPostgresWaitEventsForInstance(instanceName string) {
	t0 := time.Now()
	waitRows, err := s.PgRepo.GetWaitEventCounts(instanceName)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] Wait events error for %s: %v", instanceName, err)
		return
	}
	if len(waitRows) == 0 {
		return
	}
	now := time.Now().UTC()
	out := make([]hot.PostgresWaitEventRow, 0, len(waitRows))
	for _, r := range waitRows {
		out = append(out, hot.PostgresWaitEventRow{
			CaptureTimestamp:   now,
			ServerInstanceName: instanceName,
			WaitEventType:      r.WaitEventType,
			WaitEvent:          r.WaitEvent,
			SessionsCount:      r.SessionsCount,
		})
	}
	if err := s.tsLogger.LogPostgresWaitEvents(context.Background(), instanceName, out); err != nil {
		log.Printf("[PostgresEnterpriseCollector] Wait events insert error for %s: %v", instanceName, err)
		return
	}
	log.Printf("[PostgresEnterpriseCollector] Collected wait events from %s in %v", instanceName, time.Since(t0))
}

func (s *MetricsService) collectPostgresDbIOForInstance(instanceName string) {
	t0 := time.Now()
	dbRows, err := s.PgRepo.GetDbIOStats(instanceName)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] DB IO error for %s: %v", instanceName, err)
		return
	}
	if len(dbRows) == 0 {
		return
	}
	now := time.Now().UTC()
	out := make([]hot.PostgresDbIORow, 0, len(dbRows))
	for _, r := range dbRows {
		out = append(out, hot.PostgresDbIORow{
			CaptureTimestamp:   now,
			ServerInstanceName: instanceName,
			DatabaseName:       r.DatabaseName,
			BlksRead:           r.BlksRead,
			BlksHit:            r.BlksHit,
			TempFiles:          r.TempFiles,
			TempBytes:          r.TempBytes,
		})
	}
	if err := s.tsLogger.LogPostgresDbIOStats(context.Background(), instanceName, out); err != nil {
		log.Printf("[PostgresEnterpriseCollector] DB IO insert error for %s: %v", instanceName, err)
		return
	}
	log.Printf("[PostgresEnterpriseCollector] Collected DB IO stats from %s in %v", instanceName, time.Since(t0))
}

func (s *MetricsService) collectPostgresSettingsSnapshotForInstance(instanceName string) {
	t0 := time.Now()
	rows, err := s.PgRepo.GetSettingsSnapshot(instanceName)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] Settings snapshot error for %s: %v", instanceName, err)
		return
	}
	if len(rows) == 0 {
		return
	}
	now := time.Now().UTC()
	out := make([]hot.PostgresSettingSnapshotRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, hot.PostgresSettingSnapshotRow{
			CaptureTimestamp:   now,
			ServerInstanceName: instanceName,
			Name:               r.Name,
			Setting:            r.Setting,
			Unit:               r.Unit,
			Source:             r.Source,
		})
	}
	if err := s.tsLogger.LogPostgresSettingsSnapshot(context.Background(), instanceName, out); err != nil {
		log.Printf("[PostgresEnterpriseCollector] Settings snapshot insert error for %s: %v", instanceName, err)
		return
	}
	log.Printf("[PostgresEnterpriseCollector] Collected settings snapshot from %s in %v", instanceName, time.Since(t0))
}

// collectPostgresQueryDictionaryForInstance collects query dictionary statistics for a PostgreSQL instance
func (s *MetricsService) collectPostgresQueryDictionaryForInstance(instanceName string) {
	t0 := time.Now()

	// Fetch Query stats from PostgreSQL
	queryStats, err := s.PgRepo.FetchNormalizedQueryStats(instanceName)
	if err != nil {
		log.Printf("[PostgresEnterpriseCollector] Query Dictionary error for %s: %v", instanceName, err)
		return
	}

	if len(queryStats) == 0 {
		return
	}

	// Insert query dictionary entries into TimescaleDB
	ctx := context.Background()
	for _, qs := range queryStats {
		entry := hot.PostgresQueryDictionaryRow{
			ServerInstanceName: instanceName,
			QueryID:            qs.QueryID,
			QueryText:          qs.QueryText,
			FirstSeen:          time.Now().UTC(),
			LastSeen:           time.Now().UTC(),
			ExecutionCount:     qs.Calls,
		}

		if err := s.tsLogger.UpsertPostgresQueryDictionary(ctx, entry); err != nil {
			log.Printf("[PostgresEnterpriseCollector] Query Dictionary insert error for %s (query_id=%d): %v", instanceName, qs.QueryID, err)
		}
	}

	log.Printf("[PostgresEnterpriseCollector] Collected %d queries from %s in %v", len(queryStats), instanceName, time.Since(t0))
}

// collectPostgresQueryStatsSnapshotForInstance stores full pg_stat_statements or pg_stat_monitor snapshots.
func (s *MetricsService) collectPostgresQueryStatsSnapshotForInstance(instanceName string) {
	if s.tsLogger == nil || s.PgQueryRouter == nil {
		return
	}

	db, ok := s.PgRepo.GetConn(instanceName)
	if !ok {
		slog.Error("pg_query_stats_conn_error", "instance", instanceName, "error", "connection not found")
		return
	}

	ctx := context.Background()
	if err := s.PgQueryRouter.CollectQueryMetrics(ctx, instanceName, db); err != nil {
		slog.Error("pg_query_stats_router_error", "instance", instanceName, "error", err)
	}
}

// GetPostgresCheckpointSummary returns checkpoint statistics summary from TimescaleDB
func (s *MetricsService) GetPostgresCheckpointSummary(ctx context.Context, instanceName string, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPostgresCheckpointSummary(ctx, instanceName, from, to, limit)
}

// GetPostgresArchiveSummary returns archive statistics summary from TimescaleDB
func (s *MetricsService) GetPostgresArchiveSummary(ctx context.Context, instanceName string, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPostgresArchiveSummary(ctx, instanceName, from, to, limit)
}

func (s *MetricsService) GetLatestPostgresControlCenterStats(ctx context.Context, instanceName string) (*hot.PostgresControlCenterRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	row, err := s.tsLogger.GetLatestPostgresControlCenterStats(ctx, instanceName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &hot.PostgresControlCenterRow{}, nil
		}
		return nil, err
	}
	return row, nil
}

func (s *MetricsService) GetLatestPostgresReplicationStats(ctx context.Context, instanceName string) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetLatestPostgresReplicationStats(ctx, instanceName)
}

func (s *MetricsService) GetPostgresControlCenterHistory(ctx context.Context, instanceName string, from, to string, limit int) (*hot.PostgresControlCenterHistory, error) {
	if s.tsLogger == nil {
		return &hot.PostgresControlCenterHistory{}, fmt.Errorf("TimescaleDB not connected")
	}
	hist, err := s.tsLogger.GetPostgresControlCenterHistory(ctx, instanceName, from, to, limit)
	if err != nil {
		log.Printf("[MetricsService] GetPostgresControlCenterHistory timescale: %v", err)
		return &hot.PostgresControlCenterHistory{}, nil
	}
	return hist, nil
}


func (s *MetricsService) GetPostgresMetricHistory(ctx context.Context, instanceName, metric string, from, to time.Time, limit int) ([]instance_health.MetricDataPoint, error) {
	// AAS 4-way decomposition metrics redirection
	if metric == "cpu_load" || metric == "io_wait_load" || metric == "lock_wait_load" || metric == "idle_in_txn_load" {
		if s.PgObservabilityRepo == nil {
			return nil, fmt.Errorf("PgObservabilityRepo is nil")
		}
		fromStr := from.Format(time.RFC3339)
		toStr := to.Format(time.RFC3339)
		if from.IsZero() || to.IsZero() {
			toStr = time.Now().Format(time.RFC3339)
			fromStr = time.Now().Add(-time.Hour).Format(time.RFC3339)
		}

		trend, err := s.PgObservabilityRepo.GetLoadTrend(ctx, instanceName, fromStr, toStr)
		if err != nil {
			return nil, err
		}

		var results []instance_health.MetricDataPoint
		for _, row := range trend {
			ts, ok := row["ts"].(time.Time)
			if !ok {
				continue
			}
			val := 0.0
			switch metric {
			case "cpu_load":
				if v, ok := row["cpu_sessions"].(int); ok { val = float64(v) }
			case "io_wait_load":
				if v, ok := row["io_sessions"].(int); ok { val = float64(v) }
			case "lock_wait_load":
				if v, ok := row["lock_sessions"].(int); ok { val = float64(v) }
			case "idle_in_txn_load":
				if v, ok := row["idle_in_txn"].(int); ok { val = float64(v) }
			}
			results = append(results, instance_health.MetricDataPoint{
				Time:   ts,
				Metric: metric,
				Value:  val,
			})
		}
		// Sort by time ascending then take latest limit
		if len(results) > limit {
			results = results[len(results)-limit:]
		}
		return results, nil
	}

	if s.PgHealthRepo == nil {
		return nil, fmt.Errorf("PgHealthRepo is nil")
	}
	return s.PgHealthRepo.GetMetricHistory(ctx, instanceName, metric, from, to, limit)
}

func (s *MetricsService) GetPostgresReplicationLagDetail(ctx context.Context, instanceName string, from, to string, limit int) (map[string]hot.PostgresReplicationLagSeries, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("tsLogger is nil")
	}
	return s.tsLogger.GetPostgresReplicationLagDetail(ctx, instanceName, from, to, limit)
}

func (s *MetricsService) GetPostgresWaitEventsHistory(ctx context.Context, instanceName string, from, to time.Time, limit int) ([]hot.PostgresWaitEventRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresWaitEventRow{}, nil
	}
	return s.tsLogger.GetPostgresWaitEventsHistory(ctx, instanceName, from, to, limit)
}

func (s *MetricsService) GetPgIncidentFeed(ctx context.Context, instanceName string, lookbackMins int, limit int) ([]hot.PgIncidentFeedRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgIncidentFeed(ctx, instanceName, lookbackMins, limit)
}

func (s *MetricsService) GetPostgresDbIOHistory(ctx context.Context, instanceName string, from, to time.Time, limit int) ([]hot.PostgresDbIORow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresDbIORow{}, nil
	}
	return s.tsLogger.GetPostgresDbIOHistory(ctx, instanceName, from, to, limit)
}

func (s *MetricsService) GetPostgresSettingsSnapshotLatestTwo(ctx context.Context, instanceName string) (time.Time, time.Time, []hot.PostgresSettingSnapshotRow, []hot.PostgresSettingSnapshotRow, error) {
	if s.tsLogger == nil {
		return time.Time{}, time.Time{}, []hot.PostgresSettingSnapshotRow{}, []hot.PostgresSettingSnapshotRow{}, nil
	}
	return s.tsLogger.GetPostgresSettingsSnapshotLatestTwo(ctx, instanceName)
}

// collectJobsForInstance persists SQL Agent job metrics, details, schedules, and failures
// to TimescaleDB. Change detection in the TimescaleDB logger keeps insert volume low.
func (s *MetricsService) collectJobsForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}
	t0 := time.Now()

	jobData := s.MsRepo.FetchAgentJobs(instanceName)
	ctx := context.Background()

	summaryMap := map[string]interface{}{
		"total_jobs":      jobData.Summary.TotalJobs,
		"enabled_jobs":    jobData.Summary.EnabledJobs,
		"disabled_jobs":   jobData.Summary.DisabledJobs,
		"running_jobs":    jobData.Summary.RunningJobs,
		"failed_jobs_24h": jobData.Summary.FailedJobs,
		"error_message":   jobData.LastError,
	}
	if err := s.tsLogger.LogSQLServerJobMetrics(ctx, instanceName, summaryMap); err != nil {
		log.Printf("[EnterpriseCollector] Job metrics insert error for %s: %v", instanceName, err)
	}

	if len(jobData.Jobs) > 0 {
		jobMaps := make([]map[string]interface{}, len(jobData.Jobs))
		for i, j := range jobData.Jobs {
			jobMaps[i] = map[string]interface{}{
				"job_name":        j.JobName,
				"enabled":         j.Enabled,
				"owner":           j.Owner,
				"created_date":    j.CreatedDate,
				"current_status":  j.CurrentStatus,
				"last_run_date":   j.LastRunDate,
				"last_run_time":   j.LastRunTime,
				"last_run_status": j.LastRunStatus,
			}
		}
		if err := s.tsLogger.LogSQLServerJobDetails(ctx, instanceName, jobMaps); err != nil {
			log.Printf("[EnterpriseCollector] Job details insert error for %s: %v", instanceName, err)
		}
	}

	if len(jobData.Schedules) > 0 {
		schedMaps := make([]map[string]interface{}, len(jobData.Schedules))
		for i, sc := range jobData.Schedules {
			var nextRun interface{}
			if sc.NextRunDateTime != nil {
				nextRun = *sc.NextRunDateTime
			}
			schedMaps[i] = map[string]interface{}{
				"job_name":          sc.JobName,
				"job_enabled":       sc.JobEnabled,
				"schedule_name":     sc.ScheduleName,
				"status":            sc.Status,
				"next_run_datetime": nextRun,
			}
		}
		if err := s.tsLogger.LogSQLServerJobSchedules(ctx, instanceName, schedMaps); err != nil {
			log.Printf("[EnterpriseCollector] Job schedules insert error for %s: %v", instanceName, err)
		}
	}

	if len(jobData.Failures) > 0 {
		failMaps := make([]map[string]interface{}, len(jobData.Failures))
		for i, f := range jobData.Failures {
			failMaps[i] = map[string]interface{}{
				"job_name":  f.JobName,
				"step_name": f.StepName,
				"message":   f.Message,
				"run_date":  f.RunDate,
				"run_time":  f.RunTime,
			}
		}
		if err := s.tsLogger.LogSQLServerJobFailures(ctx, instanceName, failMaps); err != nil {
			log.Printf("[EnterpriseCollector] Job failures insert error for %s: %v", instanceName, err)
		}
	}

	log.Printf("[EnterpriseCollector] Collected job data for %s (%d jobs, %d failures) in %v",
		instanceName, len(jobData.Jobs), len(jobData.Failures), time.Since(t0))
}

// collectLogShippingForInstance persists log shipping health to TimescaleDB.
// On instances without log shipping configured this is a fast no-op.
func (s *MetricsService) collectLogShippingForInstance(instanceName string) {
	if s.tsLogger == nil {
		return
	}
	t0 := time.Now()

	lsStats, err := s.MsRepo.FetchLogShippingHealth(instanceName)
	if err != nil {
		log.Printf("[EnterpriseCollector] Log shipping health error for %s: %v", instanceName, err)
		return
	}
	if len(lsStats) == 0 {
		return
	}

	timestamp := time.Now().UTC()
	rows := make([]hot.LogShippingRow, len(lsStats))
	for i, ls := range lsStats {
		r := hot.LogShippingRow{
			CaptureTimestamp:        timestamp,
			ServerInstanceName:      instanceName,
			PrimaryServer:           ls.PrimaryServer,
			PrimaryDatabase:         ls.PrimaryDatabase,
			SecondaryServer:         ls.SecondaryServer,
			SecondaryDatabase:       ls.SecondaryDatabase,
			LastBackupFile:          ls.LastBackupFile,
			RestoreDelayMinutes:     ls.RestoreDelayMinutes,
			RestoreThresholdMinutes: ls.RestoreThresholdMinutes,
			Status:                  ls.Status,
			IsPrimary:               ls.IsPrimary,
		}
		if ls.LastBackupDate.Valid {
			t := ls.LastBackupDate.Time
			r.LastBackupDate = &t
		}
		if ls.LastRestoreDate.Valid {
			t := ls.LastRestoreDate.Time
			r.LastRestoreDate = &t
		}
		if ls.LastCopiedDate.Valid {
			t := ls.LastCopiedDate.Time
			r.LastCopiedDate = &t
		}
		rows[i] = r
	}

	ctx := context.Background()
	if err := s.tsLogger.LogLogShippingHealth(ctx, instanceName, rows); err != nil {
		log.Printf("[EnterpriseCollector] Log shipping insert error for %s: %v", instanceName, err)
		return
	}

	log.Printf("[EnterpriseCollector] Collected %d log shipping rows for %s in %v", len(rows), instanceName, time.Since(t0))
}

// GetLogShippingHealth returns the most recent log shipping health from TimescaleDB,
// falling back to a live MSDB query when Timescale is unavailable.
func (s *MetricsService) GetLogShippingHealth(ctx context.Context, instanceName string) ([]map[string]interface{}, string, error) {
	if s.tsLogger != nil {
		rows, err := s.tsLogger.GetLogShippingHealth(ctx, instanceName)
		if err == nil && len(rows) > 0 {
			return rows, "timescale", nil
		}
	}

	// Live fallback
	lsStats, err := s.MsRepo.FetchLogShippingHealth(instanceName)
	if err != nil {
		return nil, "live_error", err
	}
	if len(lsStats) == 0 {
		return []map[string]interface{}{}, "live_dmv", nil
	}

	rows := make([]map[string]interface{}, len(lsStats))
	for i, ls := range lsStats {
		row := map[string]interface{}{
			"primary_server":            ls.PrimaryServer,
			"primary_database":          ls.PrimaryDatabase,
			"secondary_server":          ls.SecondaryServer,
			"secondary_database":        ls.SecondaryDatabase,
			"last_backup_file":          ls.LastBackupFile,
			"restore_delay_minutes":     ls.RestoreDelayMinutes,
			"restore_threshold_minutes": ls.RestoreThresholdMinutes,
			"status":                    ls.Status,
			"is_primary":                ls.IsPrimary,
		}
		if ls.LastBackupDate.Valid {
			row["last_backup_date"] = ls.LastBackupDate.Time
		}
		if ls.LastRestoreDate.Valid {
			row["last_restore_date"] = ls.LastRestoreDate.Time
		}
		if ls.LastCopiedDate.Valid {
			row["last_copied_date"] = ls.LastCopiedDate.Time
		}
		rows[i] = row
	}
	return rows, "live_dmv", nil
}
