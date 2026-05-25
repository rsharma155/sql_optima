// Package service implements the SQL Optima autonomous health analysis service.
// This file orchestrates data aggregation, analysis pipeline execution, and report generation
// for SQL Server intelligence reports.
//
// Design context:
//   - GetRawDataSnapshot fetches all required metrics from TimescaleDB.
//   - v2 additions: peak window heatmap, utilization profile percentiles, top wait types,
//     per-database growth, and true 24h daily disk growth rate.
//   - Snapshot persistence: Analyze() optionally writes to intelreport.intel_snapshots.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	intelanalysis "github.com/rsharma155/sql_optima/internal/intel/analysis"
	"github.com/rsharma155/sql_optima/internal/intel/forecasting"
	"github.com/rsharma155/sql_optima/internal/intel/reports"
	"github.com/rsharma155/sql_optima/internal/models"
)

// IntelligenceReportService handles autonomous health analysis using the internal engine.
type IntelligenceReportService struct {
	pool           DBPool
	analysisEngine *intelanalysis.AnalysisEngine
	snapshotStore  *reports.SnapshotStore
	reportGen      *reports.ReportGenerator
	cache          *intelReportCache
}

// DBPool is an interface that matches the parts of *pgxpool.Pool we use.
type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// NewIntelligenceReportService creates a new instance of the service.
func NewIntelligenceReportService(pool DBPool) *IntelligenceReportService {
	// Identify template directory (relative to backend root or from env)
	templateDir := os.Getenv("SQLOPTIMA_TEMPLATE_DIR")
	if templateDir == "" {
		// Default to internal path
		templateDir = "internal/intel/templates"
	}

	return &IntelligenceReportService{
		pool:           pool,
		analysisEngine: intelanalysis.NewAnalysisEngine(),
		snapshotStore:  reports.NewSnapshotStore(pool),
		reportGen:      reports.NewReportGenerator(templateDir),
		cache:          newIntelReportCache(),
	}
}

// GetRawDataSnapshot gathers all required metrics from TimescaleDB for a given server.
func (s *IntelligenceReportService) GetRawDataSnapshot(ctx context.Context, serverID uuid.UUID) (map[string]interface{}, error) {
	raw := make(map[string]interface{})

	// 1. sqlserver_metrics (CPU, Memory, Disk)
	var avgCPU, memoryUsage, freeDiskMB, deadlocks, dataDiskMB, logDiskMB float64
	err := s.pool.QueryRow(ctx, `
		SELECT avg_cpu_load, memory_usage, free_disk_mb, deadlocks, data_disk_mb, log_disk_mb
		FROM sqlserver_metrics
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC LIMIT 1
	`, serverID).Scan(&avgCPU, &memoryUsage, &freeDiskMB, &deadlocks, &dataDiskMB, &logDiskMB)
	if err == nil {
		raw["avg_cpu_load"] = avgCPU
		raw["memory_usage"] = memoryUsage
		raw["deadlocks"] = deadlocks
		// Only propagate disk metrics when non-zero — a 0 value means the column was
		// never populated, not that the disk is actually full.
		if freeDiskMB > 0 {
			raw["free_disk_mb"] = freeDiskMB
		}
		if dataDiskMB > 0 {
			raw["data_disk_mb"] = dataDiskMB
		}
		if logDiskMB > 0 {
			raw["log_disk_mb"] = logDiskMB
		}
	}

	// 2. sqlserver_cpu_history
	var sqlProcess, systemIdle, otherProcess float64
	err = s.pool.QueryRow(ctx, `
		SELECT sql_process, system_idle, other_process
		FROM sqlserver_cpu_history
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC LIMIT 1
	`, serverID).Scan(&sqlProcess, &systemIdle, &otherProcess)
	if err == nil {
		raw["sql_process"] = sqlProcess
		raw["system_idle"] = systemIdle
		raw["other_process"] = otherProcess
	}

	// 3. sqlserver_memory_metrics (OS pressure, spills)
	var sqlMemoryUsed, pleSeconds, memoryGrantsPending, osAvailMemMB, sortWarn, hashWarn float64
	err = s.pool.QueryRow(ctx, `
		SELECT sql_memory_used_mb, ple_seconds, memory_grants_pending, os_available_memory_mb, sort_warnings_per_sec, hash_warnings_per_sec
		FROM sqlserver_memory_metrics
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC LIMIT 1
	`, serverID).Scan(&sqlMemoryUsed, &pleSeconds, &memoryGrantsPending, &osAvailMemMB, &sortWarn, &hashWarn)
	if err == nil {
		raw["sql_memory_used_mb"] = sqlMemoryUsed
		raw["ple_seconds"] = pleSeconds
		raw["memory_grants_pending"] = memoryGrantsPending
		raw["os_available_memory_mb"] = osAvailMemMB
		raw["sort_warnings_per_sec"] = sortWarn
		raw["hash_warnings_per_sec"] = hashWarn
	}

	// 4. sqlserver_risk_health (Blocking, TempDB)
	var blockingSessions, tempdbUsedPercent, pleRisk, bufferCacheHitRatio, batchRequestsPerSec float64
	err = s.pool.QueryRow(ctx, `
		SELECT blocking_sessions, tempdb_used_percent, ple, buffer_cache_hit_ratio, batch_requests_per_sec
		FROM sqlserver_risk_health
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC LIMIT 1
	`, serverID).Scan(&blockingSessions, &tempdbUsedPercent, &pleRisk, &bufferCacheHitRatio, &batchRequestsPerSec)
	if err == nil {
		raw["blocking_sessions"] = blockingSessions
		raw["tempdb_used_percent"] = tempdbUsedPercent
		raw["buffer_cache_hit_ratio"] = bufferCacheHitRatio
		if raw["ple_seconds"] == nil {
			raw["ple_seconds"] = pleRisk
		}
		if raw["batch_requests_per_sec"] == nil {
			raw["batch_requests_per_sec"] = batchRequestsPerSec
		}
	}

	// 5. sqlserver_disk_history (Total capacity & Delta growth)
	var dataMB, logMB, freeMB, deltaDataMB float64
	err = s.pool.QueryRow(ctx, `
		SELECT SUM(data_mb), SUM(log_mb), SUM(free_mb), SUM(delta_data_mb)
		FROM sqlserver_disk_history
		WHERE server_id = $1
		AND capture_timestamp = (SELECT MAX(capture_timestamp) FROM sqlserver_disk_history WHERE server_id = $1)
		GROUP BY capture_timestamp
	`, serverID).Scan(&dataMB, &logMB, &freeMB, &deltaDataMB)
	if err == nil {
		raw["total_data_mb"] = dataMB
		raw["total_log_mb"] = logMB
		raw["total_free_mb"] = freeMB
		raw["delta_data_mb"] = deltaDataMB
	}

	// 6. sqlserver_cpu_scheduler_stats (Worker exhaustion, Pressure)
	var workerExhaust, memPressure bool
	var availPhysMemKB, runnableTasks float64
	err = s.pool.QueryRow(ctx, `
		SELECT worker_thread_exhaustion_warning, physical_memory_pressure_warning, available_physical_memory_kb, total_runnable_tasks_count
		FROM sqlserver_cpu_scheduler_stats
		WHERE server_id = $1 ORDER BY capture_timestamp DESC LIMIT 1
	`, serverID).Scan(&workerExhaust, &memPressure, &availPhysMemKB, &runnableTasks)
	if err == nil {
		raw["worker_thread_exhaustion_warning"] = workerExhaust
		raw["physical_memory_pressure_warning"] = memPressure
		raw["available_physical_memory_kb"] = availPhysMemKB
		raw["total_runnable_tasks_count"] = runnableTasks
	}

	// 7. HA Replica State (Replication) — reads from canonical monitor schema
	var secondaryLagSec, logSendQueueKB, redoQueueKB float64
	var longRunningTxCount int
	err = s.pool.QueryRow(ctx, `
		SELECT MAX(secondary_lag_seconds), MAX(log_send_queue_kb), MAX(redo_queue_kb),
		       COALESCE(MAX(long_running_tx_count), 0)
		FROM monitor.sqlserver_ha_replica_state
		WHERE server_id = $1
		  AND capture_timestamp >= now() - INTERVAL '5 minutes'
	`, serverID).Scan(&secondaryLagSec, &logSendQueueKB, &redoQueueKB, &longRunningTxCount)
	if err == nil {
		raw["secondary_lag_seconds"] = secondaryLagSec
		raw["log_send_queue_kb"] = logSendQueueKB
		raw["redo_queue_kb"] = redoQueueKB
		raw["long_running_tx_count"] = float64(longRunningTxCount)
	}

	// 8. sqlserver_database_throughput (IO Latency)
	var readLat, writeLat float64
	err = s.pool.QueryRow(ctx, `
		SELECT AVG(read_latency_ms), AVG(write_latency_ms)
		FROM sqlserver_database_throughput
		WHERE server_id = $1
		AND capture_timestamp = (SELECT MAX(capture_timestamp) FROM sqlserver_database_throughput WHERE server_id = $1)
	`, serverID).Scan(&readLat, &writeLat)
	if err == nil {
		raw["read_latency_ms"] = readLat
		raw["write_latency_ms"] = writeLat
	}

	// 9. sqlserver_job_metrics (Failures)
	var failedJobs float64
	err = s.pool.QueryRow(ctx, `
		SELECT failed_jobs_24h
		FROM sqlserver_job_metrics
		WHERE server_id = $1 ORDER BY capture_timestamp DESC LIMIT 1
	`, serverID).Scan(&failedJobs)
	if err == nil {
		raw["failed_jobs_24h"] = failedJobs
	}

	// 10. sqlserver_performance_debt_findings (Maintenance)
	var perfDebtCount float64
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sqlserver_performance_debt_findings
		WHERE server_id = $1 AND severity IN ('CRITICAL','HIGH')
	`, serverID).Scan(&perfDebtCount)
	if err == nil {
		raw["performance_debt_count"] = perfDebtCount
	}

	// 11. sqlserver_server_properties (Full hardware profile)
	// All fields are needed so DefaultServerConfig can compute accurate MAXDOP,
	// worker thread, and PLE thresholds. Previously only cpu_count/physical_memory_gb
	// were fetched, leaving MaxWorkers=1024, NumaNodes=2, etc. at wrong defaults.
	var cpuCount, totalRAMGB, hyperthreadRatio, socketCount, coresPerSocket, numaNodes, maxWorkersCount int
	var hyperthreadEnabled bool
	var virtualMemoryGB float64
	var cpuType string
	err = s.pool.QueryRow(ctx, `
		SELECT cpu_count, physical_memory_gb,
		       hyperthread_ratio, socket_count, cores_per_socket,
		       numa_nodes, max_workers_count, hyperthread_enabled,
		       COALESCE(virtual_memory_gb, 0), COALESCE(cpu_type, '')
		FROM sqlserver_server_properties
		WHERE server_id = $1 ORDER BY capture_timestamp DESC LIMIT 1
	`, serverID).Scan(
		&cpuCount, &totalRAMGB,
		&hyperthreadRatio, &socketCount, &coresPerSocket,
		&numaNodes, &maxWorkersCount, &hyperthreadEnabled,
		&virtualMemoryGB, &cpuType,
	)
	if err == nil {
		raw["cpu_count"] = float64(cpuCount)
		raw["total_ram_gb"] = float64(totalRAMGB)
		if hyperthreadRatio > 0 {
			raw["hyperthread_ratio"] = float64(hyperthreadRatio)
		}
		if socketCount > 0 {
			raw["socket_count"] = float64(socketCount)
		}
		if coresPerSocket > 0 {
			raw["cores_per_socket"] = float64(coresPerSocket)
		}
		if numaNodes > 0 {
			raw["numa_nodes"] = float64(numaNodes)
		}
		if maxWorkersCount > 0 {
			raw["max_workers_count"] = float64(maxWorkersCount)
		}
		if virtualMemoryGB > 0 {
			raw["virtual_memory_gb"] = virtualMemoryGB
		}
		if cpuType != "" {
			raw["cpu_type"] = cpuType
		}
		raw["is_virtual"] = inferIsVirtualHost(cpuType, virtualMemoryGB, float64(totalRAMGB), hyperthreadEnabled)
	}

	// 12b. Wait category breakdown from latest sqlserver_wait_history snapshot.
	// The table stores aggregate ms/sec values per category (disk_read, blocking,
	// parallelism, other) — one row per collection cycle with the full breakdown.
	// We take the most recent row and store the four category values for the Waits chart.
	var waitDisk, waitBlocking, waitParallelism, waitOther float64
	waitErr := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(disk_read_ms_per_sec, 0),
			COALESCE(blocking_ms_per_sec, 0),
			COALESCE(parallelism_ms_per_sec, 0),
			COALESCE(other_ms_per_sec, 0)
		FROM sqlserver_wait_history
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`, serverID).Scan(&waitDisk, &waitBlocking, &waitParallelism, &waitOther)
	if waitErr == nil && (waitDisk+waitBlocking+waitParallelism+waitOther) > 0 {
		raw["wait_disk_ms_per_sec"]        = waitDisk
		raw["wait_blocking_ms_per_sec"]    = waitBlocking
		raw["wait_parallelism_ms_per_sec"] = waitParallelism
		raw["wait_other_ms_per_sec"]       = waitOther
	}

	// 12c. First-breach timestamps are applied via applyDynamicBreachTimestamps after
	// dynamic thresholds are computed (Analyze / GetReport) so the timeline matches the engine.

	// 13. Fetch historical series (for forecasting & trend charts)
	raw["avg_cpu_load_series"], _ = s.querySeries(ctx, serverID, "sqlserver_metrics", "avg_cpu_load", 60)
	raw["free_disk_mb_series"], _ = s.querySeries(ctx, serverID, "sqlserver_metrics", "free_disk_mb", 60)
	raw["ple_seconds_series"], _ = s.querySeries(ctx, serverID, "sqlserver_memory_metrics", "ple_seconds", 60)
	raw["batch_requests_per_sec_series"], _ = s.querySeries(ctx, serverID, "sqlserver_risk_health", "batch_requests_per_sec", 60)
	raw["tempdb_used_percent_series"], _ = s.querySeries(ctx, serverID, "sqlserver_risk_health", "tempdb_used_percent", 60)
	raw["blocking_sessions_series"], _ = s.querySeries(ctx, serverID, "sqlserver_risk_health", "blocking_sessions", 60)

	// Runnable tasks series — single row per timestamp in sqlserver_cpu_scheduler_stats.
	raw["runnable_tasks_series"], _ = s.querySeries(ctx, serverID, "sqlserver_cpu_scheduler_stats", "total_runnable_tasks_count", 60)

	// I/O latency series — sqlserver_database_throughput has one row per database per
	// timestamp, so we aggregate with AVG across databases to get a single trend value.
	// We collect read and write into parallel slices so the template can render both lines.
	ioRows, ioErr := s.pool.Query(ctx, `
		SELECT AVG(read_latency_ms)::float8, AVG(write_latency_ms)::float8
		FROM sqlserver_database_throughput
		WHERE server_id = $1
		GROUP BY capture_timestamp
		ORDER BY capture_timestamp DESC
		LIMIT 60
	`, serverID)
	if ioErr == nil {
		defer ioRows.Close()
		var readSeries, writeSeries []float64
		for ioRows.Next() {
			var r, w float64
			if scanErr := ioRows.Scan(&r, &w); scanErr == nil {
				readSeries = append(readSeries, r)
				writeSeries = append(writeSeries, w)
			}
		}
		// Reverse to chronological order (query returned DESC).
		for i, j := 0, len(readSeries)-1; i < j; i, j = i+1, j-1 {
			readSeries[i], readSeries[j] = readSeries[j], readSeries[i]
			writeSeries[i], writeSeries[j] = writeSeries[j], writeSeries[i]
		}
		if len(readSeries) > 0 {
			raw["read_latency_ms_series"]  = readSeries
			raw["write_latency_ms_series"] = writeSeries
		}
	}

	// 13b. Memory history — 7-day PLE series from dedicated hypertable (90-day retention).
	// This gives a much longer trend than sqlserver_memory_metrics which only retains ~2h.
	memHistRows, memHistErr := s.pool.Query(ctx, `
		SELECT page_life_expectancy::float8
		FROM sqlserver_memory_history
		WHERE server_id = $1
		  AND capture_timestamp > NOW() - INTERVAL '7 days'
		ORDER BY capture_timestamp ASC
	`, serverID)
	if memHistErr == nil {
		defer memHistRows.Close()
		var pleSeries []float64
		for memHistRows.Next() {
			var ple float64
			if scanErr := memHistRows.Scan(&ple); scanErr == nil {
				pleSeries = append(pleSeries, ple)
			}
		}
		if len(pleSeries) > 0 {
			raw["memory_history_ple_series"] = pleSeries
		}
	}

	// 13c. Memory grants pending + spill warnings (24h from sqlserver_memory_metrics).
	// Used in the Memory tab pressure chart alongside the PLE trend.
	mgRows, mgErr := s.pool.Query(ctx, `
		SELECT
			COALESCE(memory_grants_pending, 0)::float8,
			COALESCE(sort_warnings_per_sec, 0)::float8,
			COALESCE(hash_warnings_per_sec, 0)::float8
		FROM sqlserver_memory_metrics
		WHERE server_id = $1
		  AND capture_timestamp > NOW() - INTERVAL '24 hours'
		ORDER BY capture_timestamp ASC
	`, serverID)
	if mgErr == nil {
		defer mgRows.Close()
		var grantsSeries, sortSeries, hashSeries []float64
		for mgRows.Next() {
			var g, sv, h float64
			if scanErr := mgRows.Scan(&g, &sv, &h); scanErr == nil {
				grantsSeries = append(grantsSeries, g)
				sortSeries = append(sortSeries, sv)
				hashSeries = append(hashSeries, h)
			}
		}
		if len(grantsSeries) > 0 {
			raw["memory_grants_pending_series"] = grantsSeries
			raw["sort_warnings_series"] = sortSeries
			raw["hash_warnings_series"] = hashSeries
		}
	}

	// 14. Fetch 7-day historical snapshots for trend calculation
	rows, err := s.pool.Query(ctx, `
		SELECT capture_timestamp, blocking_sessions, memory_grants_pending, tempdb_used_percent, ple, batch_requests_per_sec
		FROM sqlserver_risk_health
		WHERE server_id = $1 AND capture_timestamp >= NOW() - INTERVAL '7 days'
		ORDER BY capture_timestamp ASC
	`, serverID)
	if err == nil {
		defer rows.Close()
		var snapshots []map[string]interface{}
		for rows.Next() {
			var ts time.Time
			var block, grants, tempdb, ple, batch float64
			if err := rows.Scan(&ts, &block, &grants, &tempdb, &ple, &batch); err == nil {
				snapshots = append(snapshots, map[string]interface{}{
					"timestamp":             ts.Format(time.RFC3339),
					"blocking_sessions":     block,
					"memory_grants_pending": grants,
					"tempdb_used_percent":   tempdb,
					"ple":                   ple,
					"batch_requests_per_sec": batch,
				})
			}
		}
		raw["risk_history_snapshots"] = snapshots
	}

	// 15. HA replica status — latest state per replica for HA tab.
	haRows, haErr := s.pool.Query(ctx, `
		SELECT DISTINCT ON (ag_name, replica_server_name)
			ag_name, replica_server_name, role_desc,
			synchronization_state_desc, synchronization_health_desc,
			availability_mode_desc, log_send_queue_kb::float8,
			redo_queue_kb::float8, secondary_lag_seconds::float8,
			is_failover_ready
		FROM monitor.sqlserver_ha_replica_state
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '5 minutes'
		ORDER BY ag_name, replica_server_name, capture_timestamp DESC
	`, serverID)
	if haErr == nil {
		defer haRows.Close()
		type replicaRow struct {
			AGName            string  `json:"ag_name"`
			ReplicaName       string  `json:"replica_name"`
			Role              string  `json:"role"`
			SyncState         string  `json:"sync_state"`
			SyncHealth        string  `json:"sync_health"`
			AvailMode         string  `json:"avail_mode"`
			LogSendQueueKB    float64 `json:"log_send_queue_kb"`
			RedoQueueKB       float64 `json:"redo_queue_kb"`
			LagSeconds        float64 `json:"lag_seconds"`
			IsFailoverReady   bool    `json:"is_failover_ready"`
		}
		var replicas []replicaRow
		for haRows.Next() {
			var r replicaRow
			if scanErr := haRows.Scan(
				&r.AGName, &r.ReplicaName, &r.Role,
				&r.SyncState, &r.SyncHealth,
				&r.AvailMode, &r.LogSendQueueKB,
				&r.RedoQueueKB, &r.LagSeconds,
				&r.IsFailoverReady,
			); scanErr == nil {
				replicas = append(replicas, r)
			}
		}
		if len(replicas) > 0 {
			raw["ha_replica_states"] = replicas
			raw["ha_configured"] = true
		}
	}
	if _, ok := raw["ha_configured"].(bool); !ok {
		raw["ha_not_configured"] = true
	}

	// 16. RPO lag trend — 24h from monitor.sqlserver_rpo_1min cagg.
	rpoRows, rpoErr := s.pool.Query(ctx, `
		SELECT rpo_seconds::float8
		FROM monitor.sqlserver_rpo_1min
		WHERE server_id = $1
		  AND bucket > NOW() - INTERVAL '24 hours'
		ORDER BY bucket ASC
	`, serverID)
	if rpoErr == nil {
		defer rpoRows.Close()
		var rpoSeries []float64
		for rpoRows.Next() {
			var v float64
			if scanErr := rpoRows.Scan(&v); scanErr == nil {
				rpoSeries = append(rpoSeries, v)
			}
		}
		if len(rpoSeries) > 0 {
			raw["ha_rpo_seconds_series"] = rpoSeries
		}
	}

	// 17. RTO estimate trend — 24h from monitor.sqlserver_rto_1min cagg.
	rtoRows, rtoErr := s.pool.Query(ctx, `
		SELECT estimated_rto_seconds::float8
		FROM monitor.sqlserver_rto_1min
		WHERE server_id = $1
		  AND bucket > NOW() - INTERVAL '24 hours'
		ORDER BY bucket ASC
	`, serverID)
	if rtoErr == nil {
		defer rtoRows.Close()
		var rtoSeries []float64
		for rtoRows.Next() {
			var v float64
			if scanErr := rtoRows.Scan(&v); scanErr == nil {
				rtoSeries = append(rtoSeries, v)
			}
		}
		if len(rtoSeries) > 0 {
			raw["ha_rto_seconds_series"] = rtoSeries
		}
	}

	// 18. CXPACKET / CXCONSUMER wait percentage (DEFECT-15 Phase B)
	// Computes parallel-wait % of total wait time over the last hour from
	// sqlserver_wait_stats_delta. Feeds the feature_cxpacket_pressure rule.
	var cxWaitMS, totalWaitMS float64
	cxErr := s.pool.QueryRow(ctx, `
		SELECT
			SUM(CASE WHEN wait_type IN ('CXPACKET','CXCONSUMER') THEN delta_wait_ms ELSE 0 END)::float8,
			NULLIF(SUM(delta_wait_ms), 0)::float8
		FROM sqlserver_wait_stats_delta
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
		  AND restart_detected = FALSE
	`, serverID).Scan(&cxWaitMS, &totalWaitMS)
	if cxErr == nil && totalWaitMS > 0 {
		raw["cxpacket_wait_pct"] = (cxWaitMS / totalWaitMS) * 100
	}

	// 19. TempDB data file count (DEFECT-15 Phase B)
	// Counts distinct DATA files for TempDB from the latest snapshot in sqlserver_tempdb_files.
	// Feeds the tempdb_configuration.tempdb_data_files_below feature check.
	var tempdbFileCount int
	tfErr := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT file_name)
		FROM sqlserver_tempdb_files
		WHERE server_id = $1
		  AND file_type = 'DATA'
		  AND capture_timestamp >= (
			SELECT MAX(capture_timestamp)
			FROM sqlserver_tempdb_files
			WHERE server_id = $1
		  ) - INTERVAL '5 minutes'
	`, serverID).Scan(&tempdbFileCount)
	if tfErr == nil && tempdbFileCount > 0 {
		raw["tempdb_data_files_count"] = float64(tempdbFileCount)
	}

	// 20. Critical jobs disabled (DEFECT-15 Phase B)
	// Counts disabled SQL Agent jobs in backup, maintenance, or database-management
	// categories from the most recent job snapshot in sqlserver_job_details.
	// These are jobs that DBA teams typically consider "critical" to leave running.
	var critJobsDisabled int
	cjErr := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT DISTINCT ON (job_name)
				job_name, job_enabled, job_category
			FROM sqlserver_job_details
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - INTERVAL '24 hours'
			ORDER BY job_name, capture_timestamp DESC
		) latest
		WHERE job_enabled = FALSE
		  AND (
			lower(job_category) LIKE '%backup%'    OR
			lower(job_category) LIKE '%maintenance%' OR
			lower(job_category) LIKE '%database%'
		  )
	`, serverID).Scan(&critJobsDisabled)
	if cjErr == nil && critJobsDisabled > 0 {
		raw["critical_jobs_disabled"] = float64(critJobsDisabled)
	}

	return raw, nil
}

func (s *IntelligenceReportService) querySeries(ctx context.Context, serverID uuid.UUID, table, column string, limit int) ([]float64, error) {
	// Whitelist validation for dynamic SQL (SEC-1)
	allowedTables := map[string]bool{
		"sqlserver_metrics":             true,
		"sqlserver_memory_metrics":      true,
		"sqlserver_risk_health":         true,
		"sqlserver_disk_history":        true,
		"sqlserver_cpu_scheduler_stats": true,
		"monitor.sqlserver_ha_replica_state": true,
	}
	allowedColumns := map[string]bool{
		"avg_cpu_load":                true,
		"free_disk_mb":                true,
		"ple_seconds":                 true,
		"batch_requests_per_sec":      true,
		"tempdb_used_percent":         true,
		"blocking_sessions":           true,
		"read_latency_ms":             true,
		"write_latency_ms":            true,
		"secondary_lag_seconds":       true,
		"memory_usage_pct":            true,
		"total_runnable_tasks_count":  true,
	}

	if !allowedTables[table] || !allowedColumns[column] {
		return nil, fmt.Errorf("forbidden table/column for series query: %s.%s", table, column)
	}

	query := fmt.Sprintf(`
		SELECT %s FROM %s
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC LIMIT $2
	`, column, table)
	rows, err := s.pool.Query(ctx, query, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err == nil {
			vals = append(vals, v)
		}
	}

	// Reverse to chronological order
	for i, j := 0, len(vals)-1; i < j; i, j = i+1, j-1 {
		vals[i], vals[j] = vals[j], vals[i]
	}

	return vals, nil
}

// loadPeakWindowSlots returns the 7×24 CPU heatmap data from sqlserver_metrics.
func (s *IntelligenceReportService) loadPeakWindowSlots(ctx context.Context, serverID uuid.UUID) ([]models.HourlySlot, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			EXTRACT(ISODOW FROM capture_timestamp)::int AS day_of_week,
			EXTRACT(HOUR FROM capture_timestamp)::int   AS hour_of_day,
			AVG(avg_cpu_load)::float8                   AS avg_cpu_pct,
			COUNT(*)::int                               AS sample_count
		FROM sqlserver_metrics
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '14 days'
		GROUP BY day_of_week, hour_of_day
		ORDER BY day_of_week, hour_of_day
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slots []models.HourlySlot
	for rows.Next() {
		var s models.HourlySlot
		if err := rows.Scan(&s.DayOfWeek, &s.HourOfDay, &s.AvgCPUPct, &s.SampleCount); err == nil {
			slots = append(slots, s)
		}
	}
	return slots, nil
}

// loadUtilizationPercentiles returns 14-day CPU/memory percentiles for utilization classification.
func (s *IntelligenceReportService) loadUtilizationPercentiles(ctx context.Context, serverID uuid.UUID) (map[string]float64, error) {
	var cpuP50, cpuP90, cpuP95, cpuP99, cpuAvg, memP50, memP95, memAvg, sampleCount float64
	err := s.pool.QueryRow(ctx, `
		SELECT
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY avg_cpu_load) AS cpu_p50,
			PERCENTILE_CONT(0.90) WITHIN GROUP (ORDER BY avg_cpu_load) AS cpu_p90,
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY avg_cpu_load) AS cpu_p95,
			PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY avg_cpu_load) AS cpu_p99,
			AVG(avg_cpu_load)                                           AS cpu_avg,
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY memory_usage) AS mem_p50,
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY memory_usage) AS mem_p95,
			AVG(memory_usage)                                           AS mem_avg,
			COUNT(*)::float8                                            AS sample_count
		FROM sqlserver_metrics
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '14 days'
	`, serverID).Scan(&cpuP50, &cpuP90, &cpuP95, &cpuP99, &cpuAvg, &memP50, &memP95, &memAvg, &sampleCount)
	if err != nil {
		return nil, err
	}
	return map[string]float64{
		"cpu_p50": cpuP50, "cpu_p90": cpuP90, "cpu_p95": cpuP95, "cpu_p99": cpuP99, "cpu_avg": cpuAvg,
		"mem_p50": memP50, "mem_p95": memP95, "mem_avg": memAvg,
		"sample_count": sampleCount,
	}, nil
}

// loadTopWaitTypes returns the top 15 wait types with delta columns from the last 24h.
// Uses delta_wait_ms (not wait_time_ms) and excludes rows where a SQL Server restart
// was detected mid-window (which would inflate the delta values artificially).
func (s *IntelligenceReportService) loadTopWaitTypes(ctx context.Context, serverID uuid.UUID) ([]intelanalysis.WaitRawRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			wait_type,
			wait_category,
			SUM(delta_wait_ms)::float8                                          AS total_wait_ms,
			SUM(delta_resource_wait_ms)::float8                                 AS total_resource_wait_ms,
			SUM(delta_waiting_tasks)::float8                                    AS total_waiting_tasks,
			ROUND(100.0 * SUM(delta_wait_ms) / NULLIF(SUM(SUM(delta_wait_ms)) OVER (), 0), 2) AS pct_of_total,
			RANK() OVER (ORDER BY SUM(delta_wait_ms) DESC)::int                 AS wait_rank
		FROM sqlserver_wait_stats_delta
		WHERE server_id = $1
		  AND capture_timestamp > NOW() - INTERVAL '24 hours'
		  AND restart_detected = FALSE
		GROUP BY wait_type, wait_category
		ORDER BY total_wait_ms DESC
		LIMIT 15
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []intelanalysis.WaitRawRow
	for rows.Next() {
		var r intelanalysis.WaitRawRow
		if err := rows.Scan(&r.WaitType, &r.WaitCategory, &r.TotalWaitMS, &r.TotalResourceWaitMS, &r.TotalWaitingTasks, &r.PctOfTotal, &r.Rank); err == nil {
			result = append(result, r)
		}
	}
	return result, nil
}

// loadDailyDiskGrowth returns per-database 30-day daily growth from sqlserver_disk_history.
// System databases are excluded — they grow on DDL/maintenance events, not application data,
// which would skew the per-database growth projections shown to DBAs.
func (s *IntelligenceReportService) loadDailyDiskGrowth(ctx context.Context, serverID uuid.UUID) ([]forecasting.DBDailyGrowth, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			database_name,
			DATE_TRUNC('day', capture_timestamp)   AS growth_day,
			SUM(delta_data_mb)::float8             AS daily_data_growth,
			SUM(delta_log_mb)::float8              AS daily_log_growth,
			MAX(data_mb)::float8                   AS eod_data_mb,
			MAX(log_mb)::float8                    AS eod_log_mb
		FROM sqlserver_disk_history
		WHERE server_id = $1
		  AND capture_timestamp > NOW() - INTERVAL '30 days'
		  AND delta_data_mb IS NOT NULL
		  AND database_name IS NOT NULL
		  AND database_name NOT IN ('master', 'model', 'msdb', 'tempdb')
		GROUP BY database_name, DATE_TRUNC('day', capture_timestamp)
		ORDER BY database_name, growth_day DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []forecasting.DBDailyGrowth
	for rows.Next() {
		var r forecasting.DBDailyGrowth
		if err := rows.Scan(&r.DatabaseName, &r.GrowthDay, &r.DailyDataGrowth, &r.DailyLogGrowth, &r.EODDataMB, &r.EODLogMB); err == nil {
			result = append(result, r)
		}
	}
	return result, nil
}

// loadCapacityTimeline returns the 30-day disk capacity time series for breach forecasting.
// Source is sqlserver_metrics (instance-level DMV snapshot), not sqlserver_disk_history
// (per-database detail), so the regression operates on the same aggregate the OS reports.
func (s *IntelligenceReportService) loadCapacityTimeline(ctx context.Context, serverID uuid.UUID) ([]forecasting.InstanceCapacityPoint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			capture_timestamp,
			data_disk_mb::float8,
			log_disk_mb::float8,
			free_disk_mb::float8,
			ROUND(
				(data_disk_mb + log_disk_mb) * 100.0
				/ NULLIF(data_disk_mb + log_disk_mb + free_disk_mb, 0),
			2)::float8 AS used_pct
		FROM sqlserver_metrics
		WHERE server_id = $1
		  AND capture_timestamp > NOW() - INTERVAL '30 days'
		  AND data_disk_mb > 0
		ORDER BY capture_timestamp ASC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []forecasting.InstanceCapacityPoint
	for rows.Next() {
		var p forecasting.InstanceCapacityPoint
		if err := rows.Scan(&p.CaptureTimestamp, &p.DataDiskMB, &p.LogDiskMB, &p.FreeDiskMB, &p.UsedPct); err == nil {
			points = append(points, p)
		}
	}
	return points, nil
}

// loadWaitTrend returns a 7-day hourly wait trend from sqlserver_cagg_wait_delta_1h, pivoted
// into WaitTrendPoint slices (one per hour bucket). Rows where a SQL Server restart was
// detected (had_restart=true) are excluded to avoid inflated deltas corrupting the trend.
func (s *IntelligenceReportService) loadWaitTrend(ctx context.Context, serverID uuid.UUID) ([]models.WaitTrendPoint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			bucket                      AS hour,
			wait_category,
			SUM(total_wait_ms)::float8  AS wait_ms
		FROM sqlserver_cagg_wait_delta_1h
		WHERE server_id = $1
		  AND bucket > NOW() - INTERVAL '7 days'
		  AND NOT had_restart
		GROUP BY hour, wait_category
		ORDER BY hour ASC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type bucket struct {
		ts    time.Time
		cpu   float64
		io    float64
		mem   float64
		lock  float64
		par   float64
		other float64
	}
	pivotMap := make(map[time.Time]*bucket)
	var order []time.Time

	for rows.Next() {
		var ts time.Time
		var cat string
		var ms float64
		if err := rows.Scan(&ts, &cat, &ms); err != nil {
			continue
		}
		if _, ok := pivotMap[ts]; !ok {
			pivotMap[ts] = &bucket{ts: ts}
			order = append(order, ts)
		}
		b := pivotMap[ts]
		switch strings.ToUpper(strings.TrimSpace(cat)) {
		case "CPU":
			b.cpu += ms
		case "IO":
			b.io += ms
		case "MEMORY":
			b.mem += ms
		case "LOCK", "LOCKING":
			b.lock += ms
		case "PARALLELISM":
			b.par += ms
		default:
			b.other += ms
		}
	}

	result := make([]models.WaitTrendPoint, 0, len(order))
	for _, ts := range order {
		b := pivotMap[ts]
		result = append(result, models.WaitTrendPoint{
			Timestamp: b.ts,
			CPU:       b.cpu,
			IO:        b.io,
			Memory:    b.mem,
			Locking:   b.lock,
			Parallel:  b.par,
			Other:     b.other,
		})
	}
	return result, nil
}

// enrichWithV2Analysis runs the new analysis modules and attaches results to the report response.
func (s *IntelligenceReportService) enrichWithV2Analysis(ctx context.Context, serverID uuid.UUID, result *models.IntelligenceReportResponse) {
	// Utilization profile
	if pcts, err := s.loadUtilizationPercentiles(ctx, serverID); err == nil {
		result.UtilizationProfile = intelanalysis.ClassifyUtilization(pcts)
	}

	// Peak window detection — also attach full 7×24 heatmap slots for the Plotly heatmap.
	if slots, err := s.loadPeakWindowSlots(ctx, serverID); err == nil {
		result.PeakWindow = intelanalysis.DetectPeakWindow(slots)
		if result.PeakWindow != nil {
			result.PeakWindow.HeatmapSlots = slots
		}
	}

	// Top wait types (24h snapshot)
	if waitRows, err := s.loadTopWaitTypes(ctx, serverID); err == nil {
		result.TopWaitTypes = intelanalysis.AnalyzeWaitTypes(waitRows)
	}

	// Wait trend (7-day hourly from continuous aggregate)
	if trend, err := s.loadWaitTrend(ctx, serverID); err == nil && len(trend) > 0 {
		result.WaitTrend = trend
	}

	// Capacity forecast v2
	dbGrowth, _ := s.loadDailyDiskGrowth(ctx, serverID)
	timeline, _ := s.loadCapacityTimeline(ctx, serverID)
	result.CapacityForecastV2 = forecasting.ForecastCapacity(dbGrowth, timeline)

	// History trend from snapshot store + DEFECT-3 fix: override RiskTrend with persisted
	// scores so the trend chart and live score use identical computation paths.
	if trend, err := s.snapshotStore.LoadHistoryTrend(ctx, serverID); err == nil && len(trend) > 0 {
		result.HistoryTrend = trend
		riskTrend := make([]models.RiskScoreResult, 0, len(trend))
		for _, e := range trend {
			riskTrend = append(riskTrend, models.RiskScoreResult{
				PerformanceRisk:  e.PerformanceRisk,
				CapacityRisk:     e.CapacityRisk,
				AvailabilityRisk: e.AvailabilityRisk,
				ReplicationRisk:  e.ReplicationRisk,
				MaintenanceRisk:  e.MaintenanceRisk,
				QueryRisk:        e.QueryRisk,
				OverallScore:     e.OverallRisk,
				Category:         snapshotRiskCategory(e.OverallRisk),
			})
		}
		result.RiskTrend = riskTrend
	}

	// P1: Query Store workload, performance debt detail, index health, configuration snapshot.
	if qw, err := s.loadQueryWorkloadSummary(ctx, serverID); err == nil {
		result.QueryWorkload = qw
		attachQueryWorkloadRules(result, qw)
	}
	if debt, err := s.loadPerformanceDebtFindings(ctx, serverID, 15); err == nil && len(debt) > 0 {
		result.PerformanceDebtItems = debt
		mergePerformanceDebtIntoIssues(result, debt)
	}
	if idx, err := s.loadIndexHealthTopN(ctx, serverID, 10); err == nil && len(idx) > 0 {
		result.IndexHealth = idx
	}
	if cfgSnap, _ := s.loadServerConfigurationSnapshot(ctx, serverID); cfgSnap != nil {
		result.ServerConfiguration = cfgSnap
	}
}

// finalizeReportPresentation applies post-analysis enrichments that depend on v2 modules.
func (s *IntelligenceReportService) finalizeReportPresentation(raw map[string]interface{}, result *models.IntelligenceReportResponse) {
	if result.QueryWorkload != nil {
		raw["query_regressions_24h"] = float64(result.QueryWorkload.Regressions24h)
		raw["plan_instability_queries"] = float64(result.QueryWorkload.PlanInstabilityQueries)
	}
	result.NarrativeSummary = intelanalysis.AppendWaitAndMetricContext(
		result.NarrativeSummary, raw, result.TopWaitTypes)
}

// snapshotRiskCategory maps an overall risk score to its category label.
func snapshotRiskCategory(score float64) string {
	switch {
	case score >= 75:
		return "Critical"
	case score >= 50:
		return "High"
	case score >= 25:
		return "Elevated"
	default:
		return "Healthy"
	}
}

// IntelligenceReportLatestMeta describes the newest persisted snapshot for auto-run UX.
type IntelligenceReportLatestMeta struct {
	HasSnapshot  bool    `json:"has_snapshot"`
	GeneratedAt  string  `json:"generated_at,omitempty"`
	OverallScore float64 `json:"overall_score,omitempty"`
	Category     string  `json:"category,omitempty"`
	DataStatus   string  `json:"data_status,omitempty"`
	AgeHours     float64 `json:"age_hours,omitempty"`
}

// GetLatestMeta returns metadata for the newest snapshot within 24h, if any.
func (s *IntelligenceReportService) GetLatestMeta(ctx context.Context, serverID uuid.UUID) (*IntelligenceReportLatestMeta, error) {
	meta := &IntelligenceReportLatestMeta{}
	snap, err := s.snapshotStore.LoadLatest(ctx, serverID, 24*time.Hour)
	if err != nil || snap == nil || snap.ReportHTML == "" {
		return meta, nil
	}
	meta.HasSnapshot = true
	meta.GeneratedAt = snap.GeneratedAt.UTC().Format(time.RFC3339)
	meta.OverallScore = snap.OverallScore
	meta.Category = snap.Category
	meta.DataStatus = snap.DataStatus
	meta.AgeHours = time.Since(snap.GeneratedAt).Hours()
	return meta, nil
}

func (s *IntelligenceReportService) runFullAnalysis(ctx context.Context, serverID uuid.UUID, runID string) (*models.IntelligenceReportResponse, map[string]interface{}, error) {
	rawData, err := s.GetRawDataSnapshot(ctx, serverID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to gather raw data: %w", err)
	}
	if _, cfgRaw := s.loadServerConfigurationSnapshot(ctx, serverID); len(cfgRaw) > 0 {
		for k, v := range cfgRaw {
			rawData[k] = v
		}
	}

	result := s.analysisEngine.Analyze(rawData, nil, runID)
	s.applyDataStatus(result, rawData)
	if runID == "" {
		result.RunID = serverID.String()
	}

	s.applyDynamicBreachTimestamps(ctx, serverID, rawData)
	s.enrichWithV2Analysis(ctx, serverID, result)
	s.finalizeReportPresentation(rawData, result)

	return result, rawData, nil
}

func (s *IntelligenceReportService) applyDataStatus(result *models.IntelligenceReportResponse, rawData map[string]interface{}) {
	populated := countPopulated(rawData)
	switch {
	case populated >= 15:
		result.DataStatus = "Full"
	case populated >= 8:
		result.DataStatus = "Partial"
		result.DataNote = fmt.Sprintf("Only %d of 20 metric sources available. Some rules may not fire.", populated)
	default:
		result.DataStatus = "Insufficient"
		result.DataNote = "Insufficient telemetry data. Run the collector for at least 10 minutes first."
	}
}

func (s *IntelligenceReportService) buildReportHTML(result *models.IntelligenceReportResponse, rawData map[string]interface{}, instanceName string) (string, error) {
	thresholds := intelanalysis.NewDynamicThresholdCalculator().Compute(
		intelanalysis.DefaultServerConfig(rawData),
		intelanalysis.ExtractHistoriesFromRaw(rawData),
	)
	serverCfg := intelanalysis.DefaultServerConfig(rawData)
	return s.reportGen.GenerateHTML(result, "executive", rawData, thresholds.ToMap(), structToMap(serverCfg), instanceName)
}

// Analyze triggers the health analysis using the internal engine and caches HTML for GetReport.
func (s *IntelligenceReportService) Analyze(ctx context.Context, serverID uuid.UUID) (*models.IntelligenceReportResponse, error) {
	result, rawData, err := s.runFullAnalysis(ctx, serverID, serverID.String())
	if err != nil {
		return nil, err
	}

	html, htmlErr := s.buildReportHTML(result, rawData, "")
	if htmlErr == nil && html != "" {
		s.cache.set(serverID, result, rawData, html)
		_, _ = s.snapshotStore.Save(ctx, serverID, result, html)
	} else {
		_, _ = s.snapshotStore.Save(ctx, serverID, result, "")
	}

	return result, nil
}

// GetReport returns HTML/JSON/PDF using in-memory cache or persisted snapshot when possible.
func (s *IntelligenceReportService) GetReport(ctx context.Context, runID string, format string, instanceName string, q url.Values) ([]byte, error) {
	serverID, err := uuid.Parse(runID)
	if err != nil {
		return nil, fmt.Errorf("invalid run_id (expected server uuid): %w", err)
	}
	if q == nil {
		q = url.Values{}
	}
	refresh := q.Get("refresh") == "1"
	fromSnapshot := q.Get("from_snapshot") == "1"

	if !refresh {
		if fromSnapshot && format == "html" {
			if snap, snapErr := s.snapshotStore.LoadLatest(ctx, serverID, 24*time.Hour); snapErr == nil && snap != nil && snap.ReportHTML != "" {
				return []byte(snap.ReportHTML), nil
			}
		}
		if cached := s.cache.get(serverID); cached != nil {
			switch format {
			case "html":
				if cached.html != "" {
					return []byte(cached.html), nil
				}
			case "json":
				if cached.result != nil {
					content, genErr := s.reportGen.GenerateJSON(cached.result)
					if genErr != nil {
						return nil, genErr
					}
					return []byte(content), nil
				}
			}
		}
	}

	result, rawData, err := s.runFullAnalysis(ctx, serverID, runID)
	if err != nil {
		return nil, err
	}

	thresholds := intelanalysis.NewDynamicThresholdCalculator().Compute(
		intelanalysis.DefaultServerConfig(rawData),
		intelanalysis.ExtractHistoriesFromRaw(rawData),
	)
	serverCfg := intelanalysis.DefaultServerConfig(rawData)

	switch format {
	case "json":
		content, genErr := s.reportGen.GenerateJSON(result)
		if genErr != nil {
			return nil, genErr
		}
		return []byte(content), nil
	case "pdf":
		pdf, pdfErr := s.reportGen.GeneratePDF(result, "executive", rawData, thresholds.ToMap(), structToMap(serverCfg))
		if pdfErr != nil {
			return nil, pdfErr
		}
		return pdf, nil
	case "html":
		content, genErr := s.buildReportHTML(result, rawData, instanceName)
		if genErr != nil {
			return nil, genErr
		}
		s.cache.set(serverID, result, rawData, content)
		_, _ = s.snapshotStore.Save(ctx, serverID, result, content)
		return []byte(content), nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// CheckStatus verifies if the internal intelligence engine is initialized.
func (s *IntelligenceReportService) CheckStatus(ctx context.Context) bool {
	return s.analysisEngine != nil
}

func structToMap(obj interface{}) map[string]interface{} {
	data, _ := json.Marshal(obj)
	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)
	return m
}

// inferIsVirtualHost classifies the host for I/O threshold defaults.
// Prefer VM signatures (cpu_type, virtual vs physical RAM) over hyperthread_enabled alone.
func inferIsVirtualHost(cpuType string, virtualMemoryGB, physicalMemoryGB float64, hyperthreadEnabled bool) bool {
	if physicalMemoryGB > 0 && virtualMemoryGB >= physicalMemoryGB*1.25 {
		return true
	}
	lower := strings.ToLower(cpuType)
	if strings.Contains(lower, "virtual") || strings.Contains(lower, "vmware") ||
		strings.Contains(lower, "hyper-v") || strings.Contains(lower, "kvm") ||
		strings.Contains(lower, "xen") || strings.Contains(lower, "microsoft corporation virtual") {
		return true
	}
	if hyperthreadEnabled && physicalMemoryGB > 0 && virtualMemoryGB > 0 && virtualMemoryGB <= physicalMemoryGB*1.1 {
		return false
	}
	return true
}

// applyDynamicBreachTimestamps populates first_breach_timestamps using the same thresholds as the analysis engine.
func (s *IntelligenceReportService) applyDynamicBreachTimestamps(ctx context.Context, serverID uuid.UUID, raw map[string]interface{}) {
	cfg := intelanalysis.DefaultServerConfig(raw)
	hist := intelanalysis.ExtractHistoriesFromRaw(raw)
	thresholds := intelanalysis.NewDynamicThresholdCalculator().Compute(cfg, hist)

	var firstBlocking, firstPLE, firstTempDB *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT
			MIN(capture_timestamp) FILTER (WHERE blocking_sessions > $2),
			MIN(capture_timestamp) FILTER (WHERE ple < $3),
			MIN(capture_timestamp) FILTER (WHERE tempdb_used_percent > $4)
		FROM sqlserver_risk_health
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '24 hours'
	`, serverID,
		float64(thresholds.BlockingSessionMax),
		thresholds.MemoryPLEMinSeconds,
		thresholds.TempDBUsedPctMax,
	).Scan(&firstBlocking, &firstPLE, &firstTempDB)
	if err != nil {
		return
	}

	var firstCPU *time.Time
	_ = s.pool.QueryRow(ctx, `
		SELECT MIN(capture_timestamp)
		FROM sqlserver_metrics
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '24 hours'
		  AND avg_cpu_load > $2
	`, serverID, thresholds.CPUSustainedThreshold).Scan(&firstCPU)

	breachTimestamps := map[string]interface{}{}
	if firstBlocking != nil {
		breachTimestamps["blocking_chains"] = firstBlocking.Format(time.RFC3339)
	}
	if firstPLE != nil {
		breachTimestamps["ple_collapse"] = firstPLE.Format(time.RFC3339)
	}
	if firstTempDB != nil {
		breachTimestamps["tempdb_pressure"] = firstTempDB.Format(time.RFC3339)
	}
	if firstCPU != nil {
		breachTimestamps["cpu_saturation"] = firstCPU.Format(time.RFC3339)
	}
	if len(breachTimestamps) > 0 {
		raw["first_breach_timestamps"] = breachTimestamps
	}
}

func countPopulated(raw map[string]interface{}) int {
	count := 0
	// List of key metric sources we expect
	keys := []string{
		"avg_cpu_load", "sql_process", "ple_seconds", "memory_usage",
		"free_disk_mb", "delta_data_mb", "blocking_sessions", "tempdb_used_percent",
		"failed_jobs_24h", "total_runnable_tasks_count", "read_latency_ms",
		"secondary_lag_seconds", "performance_debt_count", "cpu_count", "total_ram_gb",
		"avg_cpu_load_series", "free_disk_mb_series", "ple_seconds_series",
		"batch_requests_per_sec_series", "blocking_sessions_series",
	}

	for _, k := range keys {
		if v, ok := raw[k]; ok && v != nil {
			switch val := v.(type) {
			case float64:
				// Any value (including 0) counts as data presence if it was retrieved from DB
				count++
			case []float64:
				if len(val) > 0 {
					count++
				}
			case bool:
				count++
			case int:
				count++
			}
		}
	}
	return count
}
