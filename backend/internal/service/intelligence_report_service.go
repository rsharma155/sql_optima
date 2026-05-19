// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service for interacting with the Python intelligence engine for SQL Server health analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/intel/analysis"
	"github.com/rsharma155/sql_optima/internal/intel/reports"
	"github.com/rsharma155/sql_optima/internal/models"
)

// IntelligenceReportService handles autonomous health analysis using the internal engine.
type IntelligenceReportService struct {
	pool           DBPool
	analysisEngine *analysis.AnalysisEngine
	reportGen      *reports.ReportGenerator
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
		analysisEngine: analysis.NewAnalysisEngine(),
		reportGen:      reports.NewReportGenerator(templateDir),
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
		raw["free_disk_mb"] = freeDiskMB
		raw["deadlocks"] = deadlocks
		raw["data_disk_mb"] = dataDiskMB
		raw["log_disk_mb"] = logDiskMB
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

	// 7. sqlserver_ag_health (Replication)
	var secondaryLagSec, logSendQueueKB, redoQueueKB float64
	err = s.pool.QueryRow(ctx, `
		SELECT MAX(secondary_lag_seconds), MAX(log_send_queue_kb), MAX(redo_queue_kb)
		FROM sqlserver_ag_health
		WHERE server_id = $1
		AND capture_timestamp = (SELECT MAX(capture_timestamp) FROM sqlserver_ag_health WHERE server_id = $1)
	`, serverID).Scan(&secondaryLagSec, &logSendQueueKB, &redoQueueKB)
	if err == nil {
		raw["secondary_lag_seconds"] = secondaryLagSec
		raw["log_send_queue_kb"] = logSendQueueKB
		raw["redo_queue_kb"] = redoQueueKB
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

	// 11. sqlserver_server_properties (Hardware)
	var cpuCount, totalRAMGB int
	err = s.pool.QueryRow(ctx, `
		SELECT cpu_count, physical_memory_gb
		FROM sqlserver_server_properties
		WHERE server_id = $1 ORDER BY capture_timestamp DESC LIMIT 1
	`, serverID).Scan(&cpuCount, &totalRAMGB)
	if err == nil {
		raw["cpu_count"] = float64(cpuCount)
		raw["total_ram_gb"] = float64(totalRAMGB)
	}

	// 12. Fetch historical series (for forecasting & trend charts)
	raw["avg_cpu_load_series"], _ = s.querySeries(ctx, serverID, "sqlserver_metrics", "avg_cpu_load", 60)
	raw["free_disk_mb_series"], _ = s.querySeries(ctx, serverID, "sqlserver_metrics", "free_disk_mb", 60)
	raw["ple_seconds_series"], _ = s.querySeries(ctx, serverID, "sqlserver_memory_metrics", "ple_seconds", 60)
	raw["batch_requests_per_sec_series"], _ = s.querySeries(ctx, serverID, "sqlserver_risk_health", "batch_requests_per_sec", 60)
	raw["tempdb_used_percent_series"], _ = s.querySeries(ctx, serverID, "sqlserver_risk_health", "tempdb_used_percent", 60)
	raw["blocking_sessions_series"], _ = s.querySeries(ctx, serverID, "sqlserver_risk_health", "blocking_sessions", 60)

	// 13. Fetch 7-day historical snapshots for trend calculation
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

	return raw, nil
}

func (s *IntelligenceReportService) querySeries(ctx context.Context, serverID uuid.UUID, table, column string, limit int) ([]float64, error) {
	// Whitelist validation for dynamic SQL (SEC-1)
	allowedTables := map[string]bool{
		"sqlserver_metrics":        true,
		"sqlserver_memory_metrics": true,
		"sqlserver_risk_health":    true,
		"sqlserver_disk_history":   true,
		"sqlserver_ag_health":      true,
	}
	allowedColumns := map[string]bool{
		"avg_cpu_load":           true,
		"free_disk_mb":           true,
		"ple_seconds":           true,
		"batch_requests_per_sec": true,
		"tempdb_used_percent":   true,
		"blocking_sessions":     true,
		"read_latency_ms":       true,
		"write_latency_ms":      true,
		"secondary_lag_seconds":  true,
		"memory_usage_pct":      true,
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

// Analyze triggers the health analysis using the internal engine.
func (s *IntelligenceReportService) Analyze(ctx context.Context, serverID uuid.UUID) (*models.IntelligenceReportResponse, error) {
	rawData, err := s.GetRawDataSnapshot(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to gather raw data: %w", err)
	}

	// Perform analysis
	result := s.analysisEngine.Analyze(rawData, nil, "")

	// Set DataStatus based on telemetry coverage
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

	// Overwrite RunID with serverID to allow stateless report retrieval
	result.RunID = serverID.String()

	return result, nil
}

// GetReport fetches the generated report in the specified format by performing a fresh analysis.
// It uses the runID as the serverID for stateless execution.
func (s *IntelligenceReportService) GetReport(ctx context.Context, runID string, format string) ([]byte, error) {
	serverID, err := uuid.Parse(runID)
	if err != nil {
		return nil, fmt.Errorf("invalid run_id (expected server uuid): %w", err)
	}

	rawData, err := s.GetRawDataSnapshot(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to gather raw data: %w", err)
	}

	// 1. Run analysis
	result := s.analysisEngine.Analyze(rawData, nil, runID)

	// Set DataStatus
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

	// 2. Generate report
	switch format {
	case "json":
		content, err := s.reportGen.GenerateJSON(result)
		if err != nil {
			return nil, err
		}
		return []byte(content), nil
	case "html":
		// Get dynamic thresholds and server config for the template
		thresholds := analysis.NewDynamicThresholdCalculator().Compute(analysis.DefaultServerConfig(rawData), analysis.ExtractHistoriesFromRaw(rawData))
		serverCfg := analysis.DefaultServerConfig(rawData)

		content, err := s.reportGen.GenerateHTML(result, "executive", rawData, thresholds.ToMap(), structToMap(serverCfg))
		if err != nil {
			return nil, err
		}
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
