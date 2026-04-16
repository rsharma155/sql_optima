// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health scoring and anomaly detection handlers providing health score, anomalies, regressed queries, and wait spikes.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/recommendations"
	"github.com/rsharma155/sql_optima/internal/service"
)

type HealthHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewHealthHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *HealthHandlers {
	return &HealthHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *HealthHandlers) Score(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("server")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	var score = 100.0
	var cpuDeviation = 1.0
	var blockedCount int

	var currentCPU, baselineCPU float64
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB not available"})
		return
	}

	err := pool.QueryRow(r.Context(), `
		SELECT COALESCE(AVG(avg_cpu_load), 0)
		FROM sqlserver_metrics
		WHERE server_instance_name = $1 AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
	`, instance).Scan(&currentCPU)

	if err == nil && currentCPU > 0 {
		_ = pool.QueryRow(r.Context(), `
			SELECT COALESCE(AVG(avg_hourly_cpu), 50)
			FROM (SELECT time_bucket('1 hour', capture_timestamp) AS hb, AVG(avg_cpu_load) AS avg_hourly_cpu
			      FROM sqlserver_metrics WHERE server_instance_name = $1 AND capture_timestamp >= NOW() - INTERVAL '7 days'
			      GROUP BY hb) t
		`, instance).Scan(&baselineCPU)
		if baselineCPU > 0 {
			cpuDeviation = currentCPU / baselineCPU
		}
	}

	if cpuDeviation > 2.0 {
		score -= 20
	} else if cpuDeviation > 1.5 {
		score -= 10
	}

	_ = pool.QueryRow(r.Context(), `
		SELECT COUNT(DISTINCT blocked_session_id)
		FROM sqlserver_connection_history
		WHERE server_instance_name = $1 AND active_requests > 0
		  AND blocked_session_id IS NOT NULL AND blocked_session_id > 0
		  AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
	`, instance).Scan(&blockedCount)

	if blockedCount > 0 {
		score -= 15
	}

	if score < 0 {
		score = 0
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"score":            score,
		"cpu_deviation":    cpuDeviation,
		"blocked_sessions": blockedCount,
		"timestamp":        time.Now().UTC(),
	})
}

func (h *HealthHandlers) Anomalies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("server")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"anomalies": []interface{}{}, "error": "TimescaleDB not available"})
		return
	}

	rows, err := pool.Query(r.Context(), `
		SELECT time, server_instance_name, severity, category, description, recommendations
		FROM optima_incidents
		WHERE server_instance_name = $1
		  AND resolved_at IS NULL
		  AND time >= NOW() - INTERVAL '24 hours'
		ORDER BY time DESC
		LIMIT 50
	`, instance)

	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"anomalies": []interface{}{}})
		return
	}
	defer rows.Close()

	var anomalies []map[string]interface{}
	for rows.Next() {
		var t time.Time
		var server, severity, category, desc, recs string
		if err := rows.Scan(&t, &server, &severity, &category, &desc, &recs); err != nil {
			continue
		}
		anomalies = append(anomalies, map[string]interface{}{
			"timestamp":       t,
			"server":          server,
			"severity":        severity,
			"category":        category,
			"description":     desc,
			"recommendations": recs,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"anomalies": anomalies, "count": len(anomalies)})
}

func (h *HealthHandlers) RegressedQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("server")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"regressed_queries": []interface{}{}, "error": "TimescaleDB not available"})
		return
	}

	rows, err := pool.Query(r.Context(), `
		WITH current_15_mins AS (
			SELECT query_hash, AVG(exec_time_ms) AS current_avg_duration_ms,
			       SUM(execution_count) AS exec_count, MIN(query_text) AS sample_text
			FROM sqlserver_top_queries
			WHERE server_instance_name = $1 AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
			GROUP BY query_hash HAVING SUM(execution_count) >= 5
		),
		baseline_7days AS (
			SELECT query_hash, AVG(avg_exec_time_ms) AS baseline_duration_ms
			FROM hourly_query_performance_baseline
			WHERE server_instance_name = $1 AND time >= NOW() - INTERVAL '7 days'
			GROUP BY query_hash
		)
		SELECT c.query_hash, c.current_avg_duration_ms, b.baseline_duration_ms, c.exec_count, c.sample_text,
		       (c.current_avg_duration_ms / NULLIF(b.baseline_duration_ms, 0)) AS regression_ratio
		FROM current_15_mins c
		JOIN baseline_7days b ON c.query_hash = b.query_hash
		WHERE c.current_avg_duration_ms > b.baseline_duration_ms * 2 AND c.current_avg_duration_ms > 100
		ORDER BY regression_ratio DESC
		LIMIT 20
	`, instance)

	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"regressed_queries": []interface{}{}})
		return
	}
	defer rows.Close()

	var queries []map[string]interface{}
	for rows.Next() {
		var qHash string
		var currentMs, baselineMs, execCount, ratio float64
		var sampleText sql.NullString
		if err := rows.Scan(&qHash, &currentMs, &baselineMs, &execCount, &sampleText, &ratio); err != nil {
			continue
		}
		queries = append(queries, map[string]interface{}{
			"query_hash":        qHash,
			"query_text":        sampleText.String,
			"exec_count":        int(execCount),
			"avg_duration_ms":   currentMs,
			"baseline_duration": baselineMs,
			"regression_pct":    (ratio - 1) * 100,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"regressed_queries": queries, "count": len(queries)})
}

func (h *HealthHandlers) IncidentsTimeline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("server")
	hoursStr := r.URL.Query().Get("hours")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	hours := 24
	if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 && h <= 168 {
		hours = h
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"incidents": []interface{}{}, "error": "TimescaleDB not available"})
		return
	}

	rows, err := pool.Query(r.Context(), `
		SELECT time, server_instance_name, severity, category, description, recommendations, resolved_at
		FROM optima_incidents
		WHERE server_instance_name = $1 AND time >= NOW() - INTERVAL '1 hour' * $2
		ORDER BY time DESC
		LIMIT 100
	`, instance, hours)

	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"incidents": []interface{}{}})
		return
	}
	defer rows.Close()

	var incidents []map[string]interface{}
	for rows.Next() {
		var t time.Time
		var server, severity, category, desc, recs string
		var resolvedAt sql.NullTime
		if err := rows.Scan(&t, &server, &severity, &category, &desc, &recs, &resolvedAt); err != nil {
			continue
		}
		incidents = append(incidents, map[string]interface{}{
			"timestamp":       t,
			"server":          server,
			"severity":        severity,
			"category":        category,
			"description":     desc,
			"recommendations": recs,
			"resolved_at":     resolvedAt.Time,
			"is_resolved":     resolvedAt.Valid,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"incidents": incidents, "count": len(incidents)})
}

func (h *HealthHandlers) WaitSpikes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("server")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"wait_spikes": []interface{}{}, "error": "TimescaleDB not available"})
		return
	}

	rows, err := pool.Query(r.Context(), `
		WITH last_15_mins AS (
			SELECT wait_type, SUM(disk_read_ms_per_sec + blocking_ms_per_sec + parallelism_ms_per_sec + other_ms_per_sec) AS total_wait
			FROM sqlserver_wait_history
			WHERE server_instance_name = $1 AND capture_timestamp >= NOW() - INTERVAL '15 minutes'
			GROUP BY wait_type
		),
		baseline_7d AS (
			SELECT wait_type, AVG(avg_disk_read_ms + avg_blocking_ms + avg_parallelism_ms + avg_other_ms) AS baseline
			FROM hourly_wait_stats_baseline
			WHERE server_instance_name = $1 AND time >= NOW() - INTERVAL '7 days'
			GROUP BY wait_type
		)
		SELECT l.wait_type, l.total_wait, b.baseline,
		       CASE WHEN b.baseline > 0 THEN l.total_wait / b.baseline ELSE 0 END AS spike_ratio
		FROM last_15_mins l
		JOIN baseline_7d b ON l.wait_type = b.wait_type
		WHERE b.baseline > 0 AND l.total_wait / b.baseline > 2
		ORDER BY spike_ratio DESC
		LIMIT 20
	`, instance)

	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"wait_spikes": []interface{}{}})
		return
	}
	defer rows.Close()

	var spikes []map[string]interface{}
	for rows.Next() {
		var waitType string
		var currentWait, baselineWait, spikeRatio float64
		if err := rows.Scan(&waitType, &currentWait, &baselineWait, &spikeRatio); err != nil {
			continue
		}
		spikes = append(spikes, map[string]interface{}{
			"wait_type":       waitType,
			"current_wait_ms": currentWait,
			"baseline_ms":     baselineWait,
			"spike_ratio":     spikeRatio,
			"recommendation":  recommendations.GetAdviceForWaitType(waitType),
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"wait_spikes": spikes, "count": len(spikes)})
}

func (h *HealthHandlers) MetricsHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("server")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "TimescaleDB not available"})
		return
	}

	cpuRows, err := pool.Query(r.Context(), `
		SELECT capture_timestamp, avg_cpu_load, memory_usage
		FROM sqlserver_metrics
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
		ORDER BY capture_timestamp DESC
		LIMIT 60
	`, instance)

	var cpuHistory []map[string]interface{}
	if err == nil {
		defer cpuRows.Close()
		for cpuRows.Next() {
			var t time.Time
			var cpu, mem float64
			if err := cpuRows.Scan(&t, &cpu, &mem); err == nil {
				cpuHistory = append(cpuHistory, map[string]interface{}{
					"timestamp": t,
					"cpu":       cpu,
					"memory":    mem,
				})
			}
		}
	}

	batchRows, err := pool.Query(r.Context(), `
		SELECT time_bucket('1 minute', capture_timestamp) AS minute,
		       SUM(avg_tps) AS total_tps
		FROM sqlserver_db_throughput_metrics
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
		GROUP BY minute
		ORDER BY minute DESC
		LIMIT 60
	`, instance)

	var batchHistory []map[string]interface{}
	if err == nil {
		defer batchRows.Close()
		for batchRows.Next() {
			var t time.Time
			var tps float64
			if err := batchRows.Scan(&t, &tps); err == nil {
				batchHistory = append(batchHistory, map[string]interface{}{
					"timestamp": t,
					"tps":       tps,
				})
			}
		}
	}

	diskRows, err := pool.Query(r.Context(), `
		SELECT time_bucket('1 minute', capture_timestamp) AS minute,
		       AVG(disk_read_ms_per_sec) AS avg_read_latency
		FROM sqlserver_wait_history
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
		GROUP BY minute
		ORDER BY minute DESC
		LIMIT 60
	`, instance)

	var diskHistory []map[string]interface{}
	if err == nil {
		defer diskRows.Close()
		for diskRows.Next() {
			var t time.Time
			var latency float64
			if err := diskRows.Scan(&t, &latency); err == nil {
				diskHistory = append(diskHistory, map[string]interface{}{
					"timestamp": t,
					"latency":   latency,
				})
			}
		}
	}

	var cpuBaseline, batchBaseline, diskBaseline float64
	_ = pool.QueryRow(r.Context(), `
		SELECT COALESCE(AVG(avg_cpu_load), 50) FROM sqlserver_metrics
		WHERE server_instance_name = $1 AND capture_timestamp >= NOW() - INTERVAL '7 days'
	`, instance).Scan(&cpuBaseline)

	_ = pool.QueryRow(r.Context(), `
		SELECT COALESCE(AVG(total_tps), 100) FROM (
			SELECT time_bucket('1 hour', capture_timestamp) AS hour, SUM(avg_tps) AS total_tps
			FROM sqlserver_db_throughput_metrics
			WHERE server_instance_name = $1 AND capture_timestamp >= NOW() - INTERVAL '7 days'
			GROUP BY hour
		) t
	`, instance).Scan(&batchBaseline)

	_ = pool.QueryRow(r.Context(), `
		SELECT COALESCE(AVG(avg_read_latency), 10) FROM (
			SELECT time_bucket('1 hour', capture_timestamp) AS hour, AVG(disk_read_ms_per_sec) AS avg_read_latency
			FROM sqlserver_wait_history
			WHERE server_instance_name = $1 AND capture_timestamp >= NOW() - INTERVAL '7 days'
			GROUP BY hour
		) t
	`, instance).Scan(&diskBaseline)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"cpu_history":   cpuHistory,
		"batch_history": batchHistory,
		"disk_history":  diskHistory,
		"baselines": map[string]float64{
			"cpu":   cpuBaseline,
			"batch": batchBaseline,
			"disk":  diskBaseline,
		},
	})
}
