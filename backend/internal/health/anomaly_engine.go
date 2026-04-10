package health

import (
	"context"
	"log"
	"time"

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
	log.Printf("[AnomalyEngine] Starting background workers...")
	go ae.runDetectionLoop(ctx)
}

// Stop stops the anomaly detection engine
func (ae *AnomalyEngine) Stop() {
	log.Printf("[AnomalyEngine] Stopping...")
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
			log.Printf("[AnomalyEngine] Context cancelled, stopping")
			return
		case <-ae.stopCh:
			log.Printf("[AnomalyEngine] Stop signal received")
			return
		case <-ticker.C:
			ae.detectAnomalies(ctx)
		}
	}
}

func (ae *AnomalyEngine) detectAnomalies(ctx context.Context) {
	log.Printf("[AnomalyEngine] Running anomaly detection...")

	for _, inst := range ae.config.Instances {
		serverName := inst.Name
		log.Printf("[AnomalyEngine] Analyzing server: %s", serverName)

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
			WHERE server_instance_name = $1
			  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
			GROUP BY wait_type
		),
		baseline_7days_ago AS (
			SELECT wait_type,
			       AVG(avg_disk_read_ms + avg_blocking_ms + avg_parallelism_ms + avg_other_ms) AS baseline_avg_wait
			FROM hourly_wait_stats_baseline
			WHERE server_instance_name = $1
			  AND time >= NOW() - INTERVAL '7 days'
			  AND time < NOW() - INTERVAL '6 days'
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
		log.Printf("[DetectWaitSpikes] Error for %s: %v", serverName, err)
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
		log.Printf("[DetectWaitSpikes] SPIKE detected on %s: %s ratio=%.2f", serverName, waitType, spikeRatio)
	}
}

// DetectQueryRegressions compares current avg_duration_ms against 7-day baseline
// If current > baseline * 2, log an incident
func (ae *AnomalyEngine) DetectQueryRegressions(ctx context.Context, serverName string) {
	query := `
		WITH current_15_mins AS (
			SELECT query_hash,
			       AVG(exec_time_ms) AS current_avg_duration_ms,
			       SUM(execution_count) AS total_executions
			FROM sqlserver_top_queries
			WHERE server_instance_name = $1
			  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
			GROUP BY query_hash
			HAVING SUM(execution_count) >= 5
		),
		baseline_7days_ago AS (
			SELECT query_hash,
			       AVG(avg_exec_time_ms) AS baseline_avg_duration_ms
			FROM hourly_query_performance_baseline
			WHERE server_instance_name = $1
			  AND time >= NOW() - INTERVAL '7 days'
			  AND time < NOW() - INTERVAL '6 days'
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
		log.Printf("[DetectQueryRegressions] Error for %s: %v", serverName, err)
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
		log.Printf("[DetectQueryRegressions] REGRESSION on %s: query %s ratio=%.2f", serverName, queryHash[:16], regressionRatio)
	}
}
func (ae *AnomalyEngine) detectWaitSpikes(ctx context.Context, serverName string) {
	query := `
		WITH current_hour AS (
			SELECT wait_type, 
			       AVG(avg_disk_read_ms + avg_blocking_ms + avg_parallelism_ms + avg_other_ms) AS current_avg_wait
			FROM hourly_wait_stats_baseline
			WHERE server_instance_name = $1
			  AND time >= NOW() - INTERVAL '1 hour'
			GROUP BY wait_type
		),
		baseline AS (
			SELECT wait_type,
			       AVG(avg_disk_read_ms + avg_blocking_ms + avg_parallelism_ms + avg_other_ms) AS baseline_avg_wait
			FROM hourly_wait_stats_baseline
			WHERE server_instance_name = $1
			  AND time >= NOW() - INTERVAL '24 hours'
			  AND time < NOW() - INTERVAL '1 hour'
			GROUP BY wait_type
		)
		SELECT c.wait_type, c.current_avg_wait, b.baseline_avg_wait
		FROM current_hour c
		JOIN baseline b ON c.wait_type = b.wait_type
		WHERE c.current_avg_wait > b.baseline_avg_wait * 3
		  AND c.current_avg_wait > 100
	`

	rows, err := ae.pool.Query(ctx, query, serverName)
	if err != nil {
		log.Printf("[AnomalyEngine] Error detecting wait spikes for %s: %v", serverName, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var waitType string
		var currentWait, baselineWait float64
		if err := rows.Scan(&waitType, &currentWait, &baselineWait); err != nil {
			continue
		}

		increasePct := ((currentWait - baselineWait) / baselineWait) * 100
		ae.createIncident(ctx, serverName, "WARNING", "Wait Spike",
			"Wait type increased by %.0f%%",
			"Monitor this wait type. Check for blocking queries, parallelism issues, or disk I/O bottlenecks.")
		log.Printf("[AnomalyEngine] WAIT SPIKE detected on %s: %s increased by %.0f%%", serverName, waitType, increasePct)
	}
}

// detectQueryRegressions detects queries that are running slower than baseline
func (ae *AnomalyEngine) detectQueryRegressions(ctx context.Context, serverName string) {
	query := `
		WITH current_hour AS (
			SELECT query_hash, 
			       AVG(avg_exec_time_ms) AS current_avg_exec_time,
			       SUM(total_execution_count) AS current_exec_count
			FROM hourly_query_performance_baseline
			WHERE server_instance_name = $1
			  AND time >= NOW() - INTERVAL '1 hour'
			GROUP BY query_hash
			HAVING SUM(total_execution_count) >= 10
		),
		baseline AS (
			SELECT query_hash,
			       AVG(avg_exec_time_ms) AS baseline_avg_exec_time
			FROM hourly_query_performance_baseline
			WHERE server_instance_name = $1
			  AND time >= NOW() - INTERVAL '24 hours'
			  AND time < NOW() - INTERVAL '1 hour'
			GROUP BY query_hash
		)
		SELECT c.query_hash, c.current_avg_exec_time, b.baseline_avg_exec_time, c.current_exec_count
		FROM current_hour c
		JOIN baseline b ON c.query_hash = b.query_hash
		WHERE c.current_avg_exec_time > b.baseline_avg_exec_time * 2
		  AND c.current_avg_exec_time > 1000
	`

	rows, err := ae.pool.Query(ctx, query, serverName)
	if err != nil {
		log.Printf("[AnomalyEngine] Error detecting query regressions for %s: %v", serverName, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var queryHash string
		var currentTime, baselineTime, execCount float64
		if err := rows.Scan(&queryHash, &currentTime, &baselineTime, &execCount); err != nil {
			continue
		}

		increasePct := ((currentTime - baselineTime) / baselineTime) * 100
		ae.createIncident(ctx, serverName, "WARNING", "Query Regression",
			"Query performance degraded by %.0f%%",
			"Analyze execution plan changes, statistics updates, or index fragmentation.")
		log.Printf("[AnomalyEngine] QUERY REGRESSION detected on %s: query slower by %.0f%%", serverName, increasePct)
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
		WHERE server_instance_name = $1
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
			log.Printf("[AnomalyEngine] CRITICAL: Worker thread exhaustion on %s: %.1f%%", serverName, workerPct)
		}
	}

	// Check memory pressure
	if totalMemKB > 0 {
		memUsedPct := float64(totalMemKB-availMemKB) / float64(totalMemKB) * 100
		if memUsedPct >= 95 {
			ae.createIncident(ctx, serverName, "CRITICAL", "Memory Pressure",
				"Memory at %.1f%% utilization",
				"Review memory-consuming queries, clear cache, or add RAM.")
			log.Printf("[AnomalyEngine] CRITICAL: Memory pressure on %s: %.1f%%", serverName, memUsedPct)
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
		log.Printf("[AnomalyEngine] WARNING: CPU pressure on %s: %d runnable tasks", serverName, runnableTasks)
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
		log.Printf("[HealthScore] %s: CPU deviation %.2f > 2.0 (CRITICAL), -20 points", serverName, cpuDeviation)
	} else if cpuDeviation > 1.5 {
		score -= 10 // WARNING - deviation > 1.5
		log.Printf("[HealthScore] %s: CPU deviation %.2f > 1.5 (WARNING), -10 points", serverName, cpuDeviation)
	}

	// Check Active Blocking
	blockedSessions := ae.getActiveBlockingCount(ctx, serverName)
	if blockedSessions > 0 {
		score -= 15
		log.Printf("[HealthScore] %s: %d active blocked sessions, -15 points", serverName, blockedSessions)
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
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
	`

	// 7-day hourly baseline average (same hour of day)
	baselineQuery := `
		SELECT COALESCE(AVG(avg_hourly_cpu), 0)
		FROM (
			SELECT time_bucket('1 hour', capture_timestamp) AS hour_bucket,
			       AVG(avg_cpu_load) AS avg_hourly_cpu
			FROM sqlserver_metrics
			WHERE server_instance_name = $1
			  AND capture_timestamp >= NOW() - INTERVAL '7 days'
			  AND capture_timestamp < NOW() - INTERVAL '15 minutes'
			GROUP BY time_bucket('1 hour', capture_timestamp)
		) hourly_avg
	`

	var currentCPU, baselineCPU float64

	err := ae.pool.QueryRow(ctx, currentQuery, serverName).Scan(&currentCPU)
	if err != nil {
		log.Printf("[HealthScore] Error getting current CPU: %v", err)
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
		log.Printf("[HealthScore] %s: Current CPU: %.2f, Baseline CPU: %.2f, Deviation: %.2f",
			serverName, currentCPU, baselineCPU, deviation)
		return deviation
	}

	return 0
}

// getActiveBlockingCount returns the number of currently blocked sessions
func (ae *AnomalyEngine) getActiveBlockingCount(ctx context.Context, serverName string) int {
	query := `
		SELECT COUNT(DISTINCT blocked_session_id)
		FROM sqlserver_connection_history
		WHERE server_instance_name = $1
		  AND active_requests > 0
		  AND blocked_session_id IS NOT NULL
		  AND blocked_session_id > 0
		  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
	`

	var count int
	err := ae.pool.QueryRow(ctx, query, serverName).Scan(&count)
	if err != nil {
		log.Printf("[HealthScore] Error getting blocking count: %v", err)
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
		log.Printf("[AnomalyEngine] CRITICAL: Health score on %s: %.0f", serverName, healthScore)
	} else if healthScore < 75 {
		ae.createIncident(ctx, serverName, "WARNING", "Health Score",
			"Health score at %.0f",
			"Monitor closely for further degradation.")
	}
}

func (ae *AnomalyEngine) createIncident(ctx context.Context, serverName, severity, category, description, recommendations string) {
	query := `INSERT INTO optima_incidents (time, server_instance_name, severity, category, description, recommendations) VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := ae.pool.Exec(ctx, query, time.Now().UTC(), serverName, severity, category, description, recommendations)
	if err != nil {
		log.Printf("[AnomalyEngine] Error creating incident: %v", err)
	}
}

// LogIncident is a helper function to insert findings from Task 2 and Task 3 into optima_incidents table
// Used by Wait Spike and Query Regression detectors
func (ae *AnomalyEngine) LogIncident(ctx context.Context, serverName, severity, category, description, recommendations string) {
	ae.createIncident(ctx, serverName, severity, category, description, recommendations)
	log.Printf("[LogIncident] %s - %s - %s: %s", severity, category, serverName, description)
}
