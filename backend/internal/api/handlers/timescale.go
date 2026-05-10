// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB-specific handlers for historical metric queries including throughput, connections, query stats, and long-running queries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type TimescaleHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewTimescaleHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *TimescaleHandlers {
	return &TimescaleHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *TimescaleHandlers) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	connected := h.metricsSvc.IsTimescaleConnected()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": connected,
		"message":   map[string]string{"status": "TimescaleDB connection"},
	})
}

func (h *TimescaleHandlers) SqlServerMetrics(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	var metrics []map[string]interface{}
	var err error

	if from != "" && to != "" {
		metrics, err = h.metricsSvc.GetTimescaleSQLServerMetricsRange(instance, from, to, 2000)
	} else {
		// Existing behavior: latest 100
		var rows []hot.SQLServerMetricRow
		rows, err = h.metricsSvc.GetTimescaleSQLServerMetrics(instance, 100)
		// Convert to slice of maps for consistency if needed, but the original code encoded them directly.
		// Wait, GetTimescaleSQLServerMetrics returns []hot.SQLServerMetricRow.
		// If I use GetTimescaleSQLServerMetricsRange, it returns []map[string]interface{}.
		// I should probably make them consistent.
		metrics = make([]map[string]interface{}, len(rows))
		for i, r := range rows {
			metrics[i] = map[string]interface{}{
				"capture_timestamp": r.CaptureTimestamp,
				"server_name":       r.ServerName,
				"avg_cpu_load":      r.AvgCpuLoad,
				"memory_usage":      r.MemoryUsage,
				"active_users":      r.ActiveUsers,
				"total_locks":       r.TotalLocks,
				"deadlocks":         r.Deadlocks,
				"data_disk_mb":      r.DataDiskMB,
				"log_disk_mb":       r.LogDiskMB,
				"free_disk_mb":      r.FreeDiskMB,
			}
		}
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"metrics": metrics})
}

func (h *TimescaleHandlers) PostgresThroughput(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	metrics, err := h.metricsSvc.GetTimescalePostgresThroughput(instance, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"metrics": metrics})
}

func (h *TimescaleHandlers) PostgresConnections(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	stats, err := h.metricsSvc.GetTimescalePostgresConnections(instance, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"connections": stats})
}

func (h *TimescaleHandlers) SqlServerTopQueries(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	dbFilter := strings.TrimSpace(r.URL.Query().Get("database"))

	var queries []map[string]interface{}
	var err error
	if from != "" && to != "" {
		queries, err = h.metricsSvc.GetTimescaleSQLServerTopQueries(instance, 100, from, to, dbFilter)
	} else {
		// Default behavior: no explicit range, uses last hour in underlying repo if not provided
		queries, err = h.metricsSvc.GetTimescaleSQLServerTopQueries(instance, 100, "", "", dbFilter)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"top_queries": queries})
}

func (h *TimescaleHandlers) SqlServerQueryStatsDashboard(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	metric := r.URL.Query().Get("metric")
	timeRange := r.URL.Query().Get("time_range")
	dimension := r.URL.Query().Get("dimension")
	limit := 20
	if ls := strings.TrimSpace(r.URL.Query().Get("limit")); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
			if limit > 200 {
				limit = 200
			}
		}
	}

	if metric == "" {
		metric = "cpu"
	}
	if timeRange == "" {
		timeRange = "1h"
	}
	if dimension == "" {
		dimension = "query"
	}

	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))

	results, err := h.metricsSvc.GetQueryStatsDashboard(instance, metric, timeRange, dimension, limit, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results":    results,
		"instance":   instance,
		"metric":     metric,
		"time_range": timeRange,
		"dimension":  dimension,
		"from":       from,
		"to":         to,
	})
}

func (h *TimescaleHandlers) SqlServerCPUHistory(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "from and to (RFC3339) are required"})
		return
	}

	limit := 2000
	if ls := strings.TrimSpace(r.URL.Query().Get("limit")); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
			if limit > 10000 {
				limit = 10000
			}
		}
	}

	points, err := h.metricsSvc.GetTimescaleSQLServerCPUHistory(instance, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"points":   points,
		"instance": instance,
		"from":     from,
		"to":       to,
	})
}

func (h *TimescaleHandlers) SqlServerMemoryDrilldown(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "from and to (RFC3339) are required"})
		return
	}

	limit := 2000
	if ls := strings.TrimSpace(r.URL.Query().Get("limit")); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
			if limit > 10000 {
				limit = 10000
			}
		}
	}

	payload, err := h.metricsSvc.GetTimescaleSQLServerMemoryDrilldown(instance, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(payload)
}

func (h *TimescaleHandlers) SqlServerQueryStatsTimeSeries(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	metric := r.URL.Query().Get("metric")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if metric == "" {
		metric = "cpu"
	}
	
	// If from/to not provided, default to last 1 hour
	if from == "" || to == "" {
		now := time.Now().UTC()
		to = now.Format(time.RFC3339)
		from = now.Add(-1 * time.Hour).Format(time.RFC3339)
	}

	dbName := r.URL.Query().Get("database")

	results, err := h.metricsSvc.GetQueryStatsTimeSeries(instance, metric, from, to, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"series":   results,
		"instance": instance,
		"metric":   metric,
		"from":     from,
		"to":       to,
	})
}

func (h *TimescaleHandlers) SqlServerLongRunningQueries(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	log.Printf("[API] /api/timescale/sqlserver/long-running-queries called with instance=%s", instance)
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		log.Printf("[API] instance %s NOT in config", instance)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	database := strings.TrimSpace(r.URL.Query().Get("database"))
	log.Printf("[API] calling GetTimescaleSQLServerLongRunningQueries for instance=%s from=%s to=%s database=%q", instance, from, to, database)

	queries, err := h.metricsSvc.GetTimescaleSQLServerLongRunningQueries(instance, 100, from, to, database)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[API] returning %d long running queries for instance=%s", len(queries), instance)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"long_running_queries": queries})
}
