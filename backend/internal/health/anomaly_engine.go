// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Anomaly detection engine for identifying unusual metric patterns and alerting.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package health

import (
	"fmt"
	"log/slog"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/config"
)

type AnomalyEngine struct {
	pool     *pgxpool.Pool
	config   *config.Config
	interval time.Duration
	stopCh   chan struct{}
}

type Incident struct {
	Time            time.Time
	ServerName      string
	Severity        string
	Category        string
	Description     string
	Recommendations string
}

// NewAnomalyEngine creates a new anomaly detection engine
func NewAnomalyEngine(pool *pgxpool.Pool, cfg *config.Config) *AnomalyEngine {
	return &AnomalyEngine{
		pool:     pool,
		config:   cfg,
		interval: 5 * time.Minute,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background anomaly detection
func (ae *AnomalyEngine) Start(ctx context.Context) {
	slog.Info("[AnomalyEngine] Starting background workers...")
	go ae.runDetectionLoop(ctx)
}

// Stop stops the anomaly detection engine
func (ae *AnomalyEngine) Stop() {
	slog.Info("[AnomalyEngine] Stopping...")
	close(ae.stopCh)
}

func (ae *AnomalyEngine) runDetectionLoop(ctx context.Context) {
	ticker := time.NewTicker(ae.interval)
	defer ticker.Stop()

	// Run once immediately on start
	ae.detectAnomalies(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("[AnomalyEngine] Context cancelled, stopping")
			return
		case <-ae.stopCh:
			slog.Info("[AnomalyEngine] Stop signal received")
			return
		case <-ticker.C:
			ae.detectAnomalies(ctx)
		}
	}
}

func (ae *AnomalyEngine) detectAnomalies(ctx context.Context) {
	slog.Info("[AnomalyEngine] Running anomaly detection...")

	for _, inst := range ae.config.Instances {
		serverName := inst.Name
		slog.Info("[AnomalyEngine] Analyzing server", "val", serverName)

		// Run detection checks using new dedicated functions
		ae.DetectWaitSpikes(ctx, serverName)
		ae.DetectQueryRegressions(ctx, serverName)
		ae.detectResourcePressure(ctx, serverName)
		ae.detectHealthScoreDegradation(ctx, serverName)
	}
}

// DetectWaitSpikes compares last 15 mins vs exactly 7 days ago hourly baseline
// If spike ratio > 2, log an incident
func (ae *AnomalyEngine) DetectWaitSpikes(ctx context.Context, serverName string) {
	query := `
		WITH last_15_mins AS (
			SELECT wait_type,
			       SUM(disk_read_ms_per_sec + blocking_ms_per_sec + parallelism_ms_per_sec + other_ms_per_sec) AS total_wait_ms
			FROM sqlserver_wait_history
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
			GROUP BY wait_type
		),
		baseline_7days_ago AS (
			SELECT wait_type,
			       AVG(avg_disk_read_ms + avg_blocking_ms + avg_parallelism_ms + avg_other_ms) AS baseline_avg_wait
			FROM hourly_wait_stats_baseline
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - INTERVAL '7 days'
			  AND capture_timestamp < NOW() - INTERVAL '6 days'
			GROUP BY wait_type
		)
		SELECT l.wait_type, l.total_wait_ms, b.baseline_avg_wait,
		       CASE WHEN b.baseline_avg_wait > 0 THEN l.total_wait_ms / b.baseline_avg_wait ELSE 0 END AS spike_ratio
		FROM last_15_mins l
		JOIN baseline_7days_ago b ON l.wait_type = b.wait_type
		WHERE b.baseline_avg_wait > 0
		  AND (l.total_wait_ms / b.baseline_avg_wait) > 2
	`

	rows, err := ae.pool.Query(ctx, query, serverName)
	if err != nil {
		slog.Error("[DetectWaitSpikes] Error", "target", serverName, "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var waitType string
		var currentWait, baselineWait, spikeRatio float64
		if err := rows.Scan(&waitType, &currentWait, &baselineWait, &spikeRatio); err != nil {
			continue
		}

		ae.createIncident(ctx, serverName, "WARNING", "Wait Spike",
			"Wait type '"+waitType+"' spike ratio: %.2f (current: %.2fms, 7-day baseline: %.2fms)",
			"Compare with historical patterns. Check for blocking, parallelism, or I/O issues.")
		slog.Info(fmt.Sprintf("[DetectWaitSpikes] SPIKE detected on %s: %s ratio=%.2f", serverName, waitType, spikeRatio))
	}
}

// DetectQueryRegressions compares current avg_duration_ms against 7-day baseline
// If current > baseline * 2, log an incident
func (ae *AnomalyEngine) DetectQueryRegressions(ctx context.Context, serverName string) {
	query := `
		WITH current_15_mins AS (
			SELECT query_hash,
			       AVG(total_elapsed_ms / NULLIF(total_executions, 0)) AS current_avg_duration_ms,
			       SUM(total_executions) AS total_executions
			FROM sqlserver_query_metrics_v2
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
			GROUP BY query_hash
			HAVING SUM(total_executions) >= 5
		),
		baseline_7days_ago AS (
			SELECT query_hash,
			       AVG(avg_exec_time_ms) AS baseline_avg_duration_ms
			FROM hourly_query_performance_baseline
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - INTERVAL '7 days'
			  AND capture_timestamp < NOW() - INTERVAL '6 days'
			GROUP BY query_hash
		)
		SELECT c.query_hash, c.current_avg_duration_ms, b.baseline_avg_duration_ms, c.total_executions,
		       CASE WHEN b.baseline_avg_duration_ms > 0 THEN c.current_avg_duration_ms / b.baseline_avg_duration_ms ELSE 0 END AS regression_ratio
		FROM current_15_mins c
		JOIN baseline_7days_ago b ON c.query_hash = b.query_hash
		WHERE b.baseline_avg_duration_ms > 0
		  AND c.current_avg_duration_ms > b.baseline_avg_duration_ms * 2
		  AND c.current_avg_duration_ms > 100
	`

	rows, err := ae.pool.Query(ctx, query, serverName)
	if err != nil {
		slog.Error("[DetectQueryRegressions] Error", "target", serverName, "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var queryHash string
		var currentDuration, baselineDuration, regressionRatio, execCount float64
		if err := rows.Scan(&queryHash, &currentDuration, &baselineDuration, &execCount, &regressionRatio); err != nil {
			continue
		}

		ae.createIncident(ctx, serverName, "WARNING", "Query Regression",
			"Query hash "+queryHash[:16]+" regressed by %.0f%% (current: %.0fms, baseline: %.0fms, %d executions)",
			"Analyze execution plan changes, statistics updates, or index fragmentation.")
		slog.Info(fmt.Sprintf("[DetectQueryRegressions] REGRESSION on %s: query %s ratio=%.2f", serverName, queryHash[:16], regressionRatio))
	}
}

// detectResourcePressure detects CPU, memory, and worker thread pressure
func (ae *AnomalyEngine) detectResourcePressure(ctx context.Context, serverName string) {
	query := `
		SELECT 
			cpu_count, scheduler_count, max_workers_count, total_current_workers_count,
			total_physical_memory_kb, available_physical_memory_kb,
			total_runnable_tasks_count, total_work_queue_count
		FROM sqlserver_cpu_scheduler_stats
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`

	var cpuCount, schedulerCount, maxWorkers, currentWorkers int
	var totalMemKB, availMemKB, runnableTasks int64
	var workQueue int64

	err := ae.pool.QueryRow(ctx, query, serverName).Scan(
		&cpuCount, &schedulerCount, &maxWorkers, &currentWorkers,
		&totalMemKB, &availMemKB, &runnableTasks, &workQueue,
	)
	if err != nil {
		return // No data available yet
	}

	// Check worker thread exhaustion
	if maxWorkers > 0 {
		workerPct := float64(currentWorkers) / float64(maxWorkers) * 100
		if workerPct >= 90 {
			ae.createIncident(ctx, serverName, "CRITICAL", "Worker Thread Exhaustion",
				"Worker threads at %.1f%% capacity",
				"Increase max worker threads or optimize workload.")
			slog.Info("[AnomalyEngine] CRITICAL: Worker thread exhaustion on", "arg1", serverName, "arg2", workerPct)
		}
	}

	// Check memory pressure
	if totalMemKB > 0 {
		memUsedPct := float64(totalMemKB-availMemKB) / float64(totalMemKB) * 100
		if memUsedPct >= 95 {
			ae.createIncident(ctx, serverName, "CRITICAL", "Memory Pressure",
				"Memory at %.1f%% utilization",
				"Review memory-consuming queries, clear cache, or add RAM.")
			slog.Info("[AnomalyEngine] CRITICAL: Memory pressure on", "arg1", serverName, "arg2", memUsedPct)
		} else if memUsedPct >= 85 {
			ae.createIncident(ctx, serverName, "WARNING", "Memory Warning",
				"Memory at %.1f%% utilization",
				"Monitor memory usage trends.")
		}
	}

	// Check runnable tasks pressure
	if cpuCount > 0 && runnableTasks >= int64(cpuCount) {
		ae.createIncident(ctx, serverName, "WARNING", "CPU Pressure",
			"Runnable tasks exceeds CPU count",
			"Optimize CPU-intensive queries, consider query hints.")
		slog.Warn("[AnomalyEngine] WARNING: CPU pressure on", "arg1", serverName, "arg2", runnableTasks)
	}
}

// CalculateHealthScore computes the health score (0-100) for a server
// Uses 15-minute current data vs 7-day baseline for CPU deviation
func (ae *AnomalyEngine) CalculateHealthScore(ctx context.Context, serverName string) float64 {
	// Start at 100
	score := 100.0

	// Calculate CPU deviation: current 15 min vs 7-day baseline
	cpuDeviation := ae.calculateCPUDeviation(ctx, serverName)
	if cpuDeviation > 2.0 {
		score -= 20 // CRITICAL - deviation > 2.0
		slog.Info("[HealthScore]", "arg1", serverName, "arg2", cpuDeviation)
	} else if cpuDeviation > 1.5 {
		score -= 10 // WARNING - deviation > 1.5
		slog.Warn("[HealthScore]", "arg1", serverName, "arg2", cpuDeviation)
	}

	// Check Active Blocking
	blockedSessions := ae.getActiveBlockingCount(ctx, serverName)
	if blockedSessions > 0 {
		score -= 15
		slog.Info("[HealthScore]", "arg1", serverName, "arg2", blockedSessions)
	}

	// Ensure score is within bounds
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// calculateCPUDeviation compares current 15-min avg CPU vs 7-day baseline
func (ae *AnomalyEngine) calculateCPUDeviation(ctx context.Context, serverName string) float64 {
	// Current 15-minute average from sqlserver_metrics
	currentQuery := `
		SELECT COALESCE(AVG(avg_cpu_load), 0)
		FROM sqlserver_metrics
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
	`

	// 7-day hourly baseline average (same hour of day)
	baselineQuery := `
		SELECT COALESCE(AVG(avg_hourly_cpu), 0)
		FROM (
			SELECT time_bucket('1 hour', capture_timestamp) AS hour_bucket,
			       AVG(avg_cpu_load) AS avg_hourly_cpu
			FROM sqlserver_metrics
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - INTERVAL '7 days'
			  AND capture_timestamp < NOW() - INTERVAL '15 minutes'
			GROUP BY time_bucket('1 hour', capture_timestamp)
		) hourly_avg
	`

	var currentCPU, baselineCPU float64

	err := ae.pool.QueryRow(ctx, currentQuery, serverName).Scan(&currentCPU)
	if err != nil {
		slog.Error("[HealthScore] Error getting current CPU", "err", err)
		return 0
	}

	err = ae.pool.QueryRow(ctx, baselineQuery, serverName).Scan(&baselineCPU)
	if err != nil || baselineCPU == 0 {
		// If baseline is unavailable, use a default of 50% as fallback
		baselineCPU = 50.0
	}

	// Calculate deviation ratio
	if baselineCPU > 0 {
		deviation := currentCPU / baselineCPU
		slog.Info(fmt.Sprintf("[HealthScore] %s: Current CPU: %.2f, Baseline CPU: %.2f, Deviation: %.2f", serverName, currentCPU, baselineCPU, deviation))
		return deviation
	}

	return 0
}

// getActiveBlockingCount returns the number of currently blocked sessions
func (ae *AnomalyEngine) getActiveBlockingCount(ctx context.Context, serverName string) int {
	query := `
		SELECT COUNT(DISTINCT blocked_session_id)
		FROM sqlserver_connection_history
		WHERE server_id = $1
		  AND active_requests > 0
		  AND blocked_session_id IS NOT NULL
		  AND blocked_session_id > 0
		  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
	`

	var count int
	err := ae.pool.QueryRow(ctx, query, serverName).Scan(&count)
	if err != nil {
		slog.Error("[HealthScore] Error getting blocking count", "err", err)
		return 0
	}

	return count
}

// detectHealthScoreDegradation monitors overall health score using CalculateHealthScore
func (ae *AnomalyEngine) detectHealthScoreDegradation(ctx context.Context, serverName string) {
	healthScore := ae.CalculateHealthScore(ctx, serverName)

	if healthScore < 50 {
		ae.createIncident(ctx, serverName, "CRITICAL", "Health Score",
			"Health score dropped to %.0f",
			"Immediate attention required - review all metrics.")
		slog.Info("[AnomalyEngine] CRITICAL: Health score on", "arg1", serverName, "arg2", healthScore)
	} else if healthScore < 75 {
		ae.createIncident(ctx, serverName, "WARNING", "Health Score",
			"Health score at %.0f",
			"Monitor closely for further degradation.")
	}
}

func (ae *AnomalyEngine) createIncident(ctx context.Context, serverName, severity, category, description, recommendations string) {
	var serverID uuid.UUID
	if err := ae.pool.QueryRow(ctx, "SELECT id FROM optima_servers WHERE name = $1", serverName).Scan(&serverID); err != nil {
		slog.Error("[AnomalyEngine] Cannot resolve server", "target", serverName, "err", err)
		return
	}
	_, err := ae.pool.Exec(ctx,
		`INSERT INTO optima_incidents (capture_timestamp, server_id, severity, category, description, recommendations) VALUES ($1, $2, $3, $4, $5, $6)`,
		time.Now().UTC(), serverID, severity, category, description, recommendations,
	)
	if err != nil {
		slog.Error("[AnomalyEngine] Error creating incident", "err", err)
	}
}

// LogIncident is a helper function to insert findings from Task 2 and Task 3 into optima_incidents table
// Used by Wait Spike and Query Regression detectors
func (ae *AnomalyEngine) LogIncident(ctx context.Context, serverName, severity, category, description, recommendations string) {
	ae.createIncident(ctx, serverName, severity, category, description, recommendations)
	slog.Info(fmt.Sprintf("[LogIncident] %s - %s - %s: %s", severity, category, serverName, description))
}
