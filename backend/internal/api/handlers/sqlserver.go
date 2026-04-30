// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for SQL Server dashboard endpoints including overview, CPU drilldown, memory, waits, jobs, and performance debt metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SqlServerHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *SqlServerHandlers {
	return &SqlServerHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

// sqlserverPreferLiveSource is true when the client requests direct DMV/live SQL Server data (e.g. emergency override).
// Default is TimescaleDB-first for all non–Real-Time Diagnostics pages.
func sqlserverPreferLiveSource(r *http.Request) bool {
	return strings.EqualFold(r.URL.Query().Get("source"), "live")
}

// DashboardTimeSeries returns historical risk health snapshots for a time window.
func (h *SqlServerHandlers) DashboardTimeSeries(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name required"})
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	now := time.Now().UTC()
	from := now.Add(-1 * time.Hour)
	to := now

	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t.UTC()
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t.UTC()
		}
	}

	history, err := h.metricsSvc.GetSQLServerRiskHealthHistory(r.Context(), instance, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"from":     from.Format(time.RFC3339),
		"to":       to.Format(time.RFC3339),
		"series":   history,
	})
}

// TableDrilldown returns a comprehensive analytical package for a specific table (SQL Server).
func (h *SqlServerHandlers) TableDrilldown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	engine := r.URL.Query().Get("engine")
	instance := r.URL.Query().Get("instance")
	db := r.URL.Query().Get("db")
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if instance == "" || db == "" || table == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance, db, and table are required"})
		return
	}

	// Default to sqlserver if engine not provided
	if engine == "" {
		engine = "sqlserver"
	}

	// Default 24h range if missing
	if from == "" || to == "" {
		end := time.Now().UTC()
		start := end.Add(-24 * time.Hour)
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}

	// 1. Table Growth History
	growth, err := h.metricsSvc.GetTableSizeHistory(r.Context(), engine, instance, from, to, db, schema, table)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "growth: " + err.Error()})
		return
	}

	// 2. Index Usage History
	indices, _ := h.metricsSvc.GetIndexUsageHistory(r.Context(), engine, instance, from, to, db, schema, table)

	// 3. Fragmentation History
	frag, _ := h.metricsSvc.GetIndexFragmentationHistory(r.Context(), engine, instance, from, to, db, schema, table)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":      instance,
		"database":      db,
		"table":         table,
		"growth_series": growth,
		"index_usage":   indices,
		"fragmentation": frag,
	})
}

// EnterpriseDashboardV2 returns time-series standardized enterprise metrics for a specific window.
func (h *SqlServerHandlers) EnterpriseDashboardV2(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance name required", http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		now := time.Now().UTC()
		to = now.Format(time.RFC3339)
		from = now.Add(-1 * time.Hour).Format(time.RFC3339)
	}

	ctx := r.Context()
	res := make(map[string]interface{})

	// 1. Wait Stats
	waits, _ := h.metricsSvc.GetSqlServerWaitStatsTimeSeries(ctx, instance, from, to)
	res["wait_stats"] = waits

	// 2. Perf Counters (Batch Requests, Compilations, etc.)
	perf, _ := h.metricsSvc.GetSqlServerPerfCountersTimeSeries(ctx, instance, from, to, []string{
		"Batch Requests/sec", "SQL Compilations/sec", "SQL Re-Compilations/sec", "Latch Waits/sec",
	})
	res["perf_counters"] = perf

	// 3. File IO
	io, _ := h.metricsSvc.GetSqlServerFileIOTimeSeries(ctx, instance, from, to)
	res["file_io"] = io

	// 4. Plan Cache
	cache, _ := h.metricsSvc.GetSqlServerPlanCacheTimeSeries(ctx, instance, from, to)
	res["plan_cache"] = cache

	// 5. Memory Clerks
	clerks, _ := h.metricsSvc.GetSqlServerMemoryClerksTimeSeries(ctx, instance, from, to)
	res["memory_clerks"] = clerks

	// 6. Memory Grants
	grants, _ := h.metricsSvc.GetSqlServerMemoryGrantsTimeSeries(ctx, instance, from, to)
	res["memory_grants"] = grants

	// 7. TempDB Consumers
	consumers, _ := h.metricsSvc.GetSqlServerTempdbConsumersTimeSeries(ctx, instance, from, to)
	res["tempdb_consumers"] = consumers

	// 8. Add Current Snapshot (Runnable tasks, etc.) from live cache
	res["snapshot"] = h.metricsSvc.GetCachedDashboard(instance)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(res)
}

func (h *SqlServerHandlers) Overview(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.metricsSvc.GetSqlServerOverview(instance))
}

func (h *SqlServerHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	source := r.URL.Query().Get("source")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	cached := h.metricsSvc.GetCachedDashboard(instance)

	if source == "live" {
		w.Header().Set("X-Data-Source", "live_cache")
		json.NewEncoder(w).Encode(cached)
		return
	}

	if !h.metricsSvc.IsTimescaleConnected() {
		w.Header().Set("X-Data-Source", "timescale_unavailable")
		json.NewEncoder(w).Encode(cached)
		return
	}

	tsData, err := h.metricsSvc.GetDashboardFromTimescale(instance)
	if err != nil {
		log.Printf("[Router] TimescaleDB fetch failed for %s, falling back to cache: %v", instance, err)
		w.Header().Set("X-Data-Source", "live_cache_fallback")
		json.NewEncoder(w).Encode(cached)
		return
	}

	merged, err := mergeDashboardCacheWithTimescale(cached, tsData)
	if err != nil {
		log.Printf("[Router] Failed to merge Timescale dashboard data for %s: %v", instance, err)
		w.Header().Set("X-Data-Source", "live_cache_fallback")
		json.NewEncoder(w).Encode(cached)
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(merged)
}

func mergeDashboardCacheWithTimescale(cached interface{}, tsData map[string]interface{}) (map[string]interface{}, error) {
	payload, err := json.Marshal(cached)
	if err != nil {
		return nil, err
	}

	var merged map[string]interface{}
	if err := json.Unmarshal(payload, &merged); err != nil {
		return nil, err
	}

	for k, v := range tsData {
		merged[k] = v
	}

	return merged, nil
}

// DashboardV2 returns the Phase-1 DBA homepage payload.
// It is intentionally cached-only in Phase-1 to keep latency low and behavior predictable.
func (h *SqlServerHandlers) DashboardV2(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}
	// On-demand refresh: live dashboard cache is updated only at startup by default.
	// Refreshing here ensures charts like PLE and Disk I/O Latency are not blank when users load the page.
	{
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		h.metricsSvc.RunLiveCollectorForInstance(ctx, instance)
		cancel()
	}
	// Disk I/O Latency chart reads sqlserver_file_io_latency; record one snapshot per dashboard load
	// so the trend is not empty when the Enterprise metrics interval has not fired yet.
	if h.metricsSvc.IsTimescaleConnected() {
		ctxIO, cancelIO := context.WithTimeout(context.Background(), 5*time.Second)
		h.metricsSvc.WarmFileIOLatencyToTimescale(ctxIO, instance)
		cancelIO()
	}
	out, src := h.metricsSvc.GetDashboardHomepageV2WithSource(instance)
	w.Header().Set("X-Data-Source", src)
	json.NewEncoder(w).Encode(out)
}

// PerformanceDebt returns maintenance/risk findings collected into TimescaleDB (hourly snapshots).
func (h *SqlServerHandlers) PerformanceDebt(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	dbFilter := strings.TrimSpace(r.URL.Query().Get("database"))
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !h.metricsSvc.IsTimescaleConnected() {
		w.Header().Set("X-Data-Source", "timescale_unavailable")
		json.NewEncoder(w).Encode(map[string]any{
			"findings": []any{},
		})
		return
	}

	lookback := 2 // hours
	if v := r.URL.Query().Get("lookback_hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 168 {
			lookback = n
		}
	}

	out, err := h.metricsSvc.GetTimescalePerformanceDebtFindings(r.Context(), instance, time.Duration(lookback)*time.Hour, dbFilter)
	if err != nil {
		w.Header().Set("X-Data-Source", "timescale_error")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to load performance debt findings"})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]any{
		"findings": out,
	})
}

func (h *SqlServerHandlers) Jobs(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	// Dashboard is now purely time-series driven from TimescaleDB.
	// Live polling is disabled for this view to ensure consistency and performance.
	if !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB connection required for job history"})
		return
	}

	jobData, err := h.metricsSvc.GetJobsFromTimescale(ctx, instance, from, to)
	if err != nil {
		log.Printf("[Router] Timescale jobs failed for %s: %v", instance, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to retrieve job data from historical storage"})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(jobData)
}

// LogShipping returns log shipping health — Timescale-first with live MSDB fallback.
func (h *SqlServerHandlers) LogShipping(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()

	rows, source, err := h.metricsSvc.GetLogShippingHealth(ctx, instance)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to retrieve log shipping health"})
		return
	}
	w.Header().Set("X-Data-Source", source)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"log_shipping_enabled": len(rows) > 0,
		"log_shipping":         rows,
	})
}

func (h *SqlServerHandlers) XEvents(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	events, err := h.metricsSvc.GetRecentXEvents(instance)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to retrieve extended events"})
		return
	}

	json.NewEncoder(w).Encode(events)
}

func (h *SqlServerHandlers) BestPractices(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	result := h.metricsSvc.GetBestPractices(instance)
	json.NewEncoder(w).Encode(result)
}

func (h *SqlServerHandlers) Guardrails(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	result := h.metricsSvc.GetGuardrails(instance)
	json.NewEncoder(w).Encode(result)
}

func (h *SqlServerHandlers) CPUDrilldown(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	limitStr := r.URL.Query().Get("limit")
	dbFilter := strings.TrimSpace(r.URL.Query().Get("database"))

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	fromQ := strings.TrimSpace(r.URL.Query().Get("from"))
	toQ := strings.TrimSpace(r.URL.Query().Get("to"))

	normalizeTopQueryTimestamps := func(queries []map[string]interface{}) {
		for _, q := range queries {
			if q == nil {
				continue
			}
			if _, ok := q["capture_timestamp"]; !ok {
				if ts, ok2 := q["timestamp"]; ok2 {
					q["capture_timestamp"] = ts
				}
			}
		}
	}

	// If user specifically requests "live" data, we trigger one on-demand collector run
	// to ensure the TimescaleDB hypertable has the latest deltas.
	if preferLive {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		h.metricsSvc.RunLiveCollectorForInstance(ctx, instance)
		cancel()
	}

	if h.metricsSvc.IsTimescaleConnected() && fromQ != "" && toQ != "" {
		queries, err := h.metricsSvc.GetTimescaleSQLServerTopQueries(instance, limit, fromQ, toQ, dbFilter)
		if err == nil {
			normalizeTopQueryTimestamps(queries)
			w.Header().Set("X-Data-Source", "timescale_range")
			json.NewEncoder(w).Encode(map[string]interface{}{"queries": queries, "count": len(queries)})
			return
		}
		log.Printf("[Router] Timescale top queries (range) failed for %s: %v", instance, err)
	}

	if h.metricsSvc.IsTimescaleConnected() {
		queries, err := h.metricsSvc.GetTimescaleSQLServerTopQueriesLatest(instance, limit, dbFilter)
		if err == nil {
			normalizeTopQueryTimestamps(queries)
			if preferLive {
				w.Header().Set("X-Data-Source", "timescale_on_demand_live")
			} else {
				w.Header().Set("X-Data-Source", "timescale_latest")
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"queries": queries, "count": len(queries)})
			return
		}
		log.Printf("[Router] Timescale top queries failed for %s: %v", instance, err)
	}

	// If Timescale fails, we no longer fallback to direct DMV in this handler
	// as per the mandate of having a single ingestion point (the Collector).
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{"error": "Query metrics currently unavailable via TimescaleDB pipeline"})
}

func (h *SqlServerHandlers) AGHealth(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	ctx := r.Context()

	if !preferLive && h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleAGHealthSummary(ctx, instance, 100)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"hadr_enabled": len(stats) > 0,
				"ag_health":    stats,
				"ag_stats":     stats,
			})
			return
		}
		log.Printf("[Router] Timescale AG health failed for %s, using live DMV: %v", instance, err)
	}

	stats, err := h.metricsSvc.MsRepo.FetchAGHealthStats(instance)
	if err != nil {
		log.Printf("[Router] AG Health error: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"hadr_enabled": false,
			"ag_health":    []interface{}{},
			"ag_stats":     []interface{}{},
		})
		return
	}
	if preferLive {
		w.Header().Set("X-Data-Source", "live_dmv")
	} else {
		w.Header().Set("X-Data-Source", "live_dmv_fallback")
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hadr_enabled": len(stats) > 0,
		"ag_health":    stats,
		"ag_stats":     stats,
	})
}

func (h *SqlServerHandlers) AGHealthTimeSeries(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance required"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"history": []any{}})
		return
	}

	history, err := h.metricsSvc.GetTimescaleDBLogger().GetAGHealthTimeSeries(r.Context(), instance, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"history": history})
}

func (h *SqlServerHandlers) ReplicationStatus(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance required"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	stats, err := h.metricsSvc.MsRepo.FetchReplicationStatus(instance)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"replication": stats})
}

func (h *SqlServerHandlers) DBThroughput(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	ctx := r.Context()

	if !preferLive && h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleDatabaseThroughputSummary(ctx, instance, 100)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"db_throughput": stats,
				"db_stats":      stats,
			})
			return
		}
		log.Printf("[Router] Timescale DB throughput failed for %s, using live DMV: %v", instance, err)
	}

	stats, err := h.metricsSvc.MsRepo.FetchDatabaseThroughput(instance)
	if err != nil {
		log.Printf("[Router] DB Throughput error: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"db_throughput": []interface{}{},
			"db_stats":      []interface{}{},
		})
		return
	}
	if preferLive {
		w.Header().Set("X-Data-Source", "live_dmv")
	} else {
		w.Header().Set("X-Data-Source", "live_dmv_fallback")
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"db_throughput": stats,
		"db_stats":      stats,
	})
}

func (h *SqlServerHandlers) LatchStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchLatchStats(instance)
		if err != nil {
			log.Printf("[Router] Latch stats error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": []interface{}{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": stats})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleLatchWaits(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": stats})
			return
		}
		log.Printf("[Router] Timescale latch stats failed for %s: %v", instance, err)
	}
	stats, err := h.metricsSvc.MsRepo.FetchLatchStats(instance)
	if err != nil {
		log.Printf("[Router] Latch stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": stats})
}

func (h *SqlServerHandlers) WaitingTasks(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchWaitingTasks(instance)
		if err != nil {
			log.Printf("[Router] Waiting tasks error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": []interface{}{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": stats})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleWaitingTasks(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": stats})
			return
		}
		log.Printf("[Router] Timescale waiting tasks failed for %s: %v", instance, err)
	}
	stats, err := h.metricsSvc.MsRepo.FetchWaitingTasks(instance)
	if err != nil {
		log.Printf("[Router] Waiting tasks error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": stats})
}

func (h *SqlServerHandlers) MemoryGrants(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchMemoryGrants(instance)
		if err != nil {
			log.Printf("[Router] Memory grants error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": []interface{}{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": stats})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleMemoryGrants(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": stats})
			return
		}
		log.Printf("[Router] Timescale memory grants failed for %s: %v", instance, err)
	}
	stats, err := h.metricsSvc.MsRepo.FetchMemoryGrants(instance)
	if err != nil {
		log.Printf("[Router] Memory grants error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": stats})
}

func (h *SqlServerHandlers) SchedulerWorkers(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchSchedulerWG(instance)
		if err != nil {
			log.Printf("[Router] Scheduler worker stats error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"scheduler_wg":      []interface{}{},
				"scheduler_workers": []interface{}{},
			})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"scheduler_wg":      stats,
			"scheduler_workers": stats,
		})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleSchedulerWG(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"scheduler_wg":      stats,
				"scheduler_workers": stats,
			})
			return
		}
		log.Printf("[Router] Timescale scheduler WG failed for %s: %v", instance, err)
	}

	stats, err := h.metricsSvc.MsRepo.FetchSchedulerWG(instance)
	if err != nil {
		log.Printf("[Router] Scheduler worker stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"scheduler_wg":      []interface{}{},
			"scheduler_workers": []interface{}{},
		})
		return
	}

	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scheduler_wg":      stats,
		"scheduler_workers": stats,
	})
}

func (h *SqlServerHandlers) ProcedureStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchProcedureStats(instance)
		if err != nil {
			log.Printf("[Router] Procedure stats error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": []interface{}{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": stats})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleProcedureStats(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": stats})
			return
		}
		log.Printf("[Router] Timescale procedure stats failed for %s: %v", instance, err)
	}
	stats, err := h.metricsSvc.MsRepo.FetchProcedureStats(instance)
	if err != nil {
		log.Printf("[Router] Procedure stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": stats})
}

func (h *SqlServerHandlers) FileIOLatency(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchFileIOLatency(instance)
		if err != nil {
			log.Printf("[Router] File IO latency error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": []interface{}{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": stats})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleFileIOLatency(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": stats})
			return
		}
		log.Printf("[Router] Timescale file IO latency failed for %s: %v", instance, err)
	}
	stats, err := h.metricsSvc.MsRepo.FetchFileIOLatency(instance)
	if err != nil {
		log.Printf("[Router] File IO latency error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": stats})
}

func (h *SqlServerHandlers) SpinlockStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchSpinlockStats(instance)
		if err != nil {
			log.Printf("[Router] Spinlock stats error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": []interface{}{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": stats})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleSpinlockStats(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": stats})
			return
		}
		log.Printf("[Router] Timescale spinlock stats failed for %s: %v", instance, err)
	}
	stats, err := h.metricsSvc.MsRepo.FetchSpinlockStats(instance)
	if err != nil {
		log.Printf("[Router] Spinlock stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": stats})
}

func (h *SqlServerHandlers) MemoryClerks(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchMemoryClerks(instance)
		if err != nil {
			log.Printf("[Router] Memory clerks error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": []interface{}{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": stats})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleMemoryClerks(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": stats})
			return
		}
		log.Printf("[Router] Timescale memory clerks failed for %s: %v", instance, err)
	}
	stats, err := h.metricsSvc.MsRepo.FetchMemoryClerks(instance)
	if err != nil {
		log.Printf("[Router] Memory clerks error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": stats})
}

func (h *SqlServerHandlers) TempdbStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		stats, err := h.metricsSvc.MsRepo.FetchTempdbStats(instance)
		if err != nil {
			log.Printf("[Router] Tempdb stats error: %v", err)
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": []interface{}{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": stats})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		stats, err := h.metricsSvc.GetTimescaleTempdbFiles(r.Context(), instance, 50)
		if err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": stats})
			return
		}
		log.Printf("[Router] Timescale tempdb files failed for %s: %v", instance, err)
	}
	stats, err := h.metricsSvc.MsRepo.FetchTempdbStats(instance)
	if err != nil {
		log.Printf("[Router] Tempdb stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": stats})
}

func (h *SqlServerHandlers) PlanCacheHealth(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		row, err := h.metricsSvc.MsRepo.FetchPlanCacheHealth(instance)
		if err == nil && row != nil && len(row) > 0 {
			w.Header().Set("X-Data-Source", "live_dmv")
			json.NewEncoder(w).Encode(map[string]any{"plan_cache_health": []any{row}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]any{"plan_cache_health": []any{}})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		if rows, err := h.metricsSvc.GetTimescalePlanCacheHealth(r.Context(), instance, 60); err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]any{"plan_cache_health": rows})
			return
		}
	}

	w.Header().Set("X-Data-Source", "timescale_unavailable")
	json.NewEncoder(w).Encode(map[string]any{"plan_cache_health": []any{}})
}

func (h *SqlServerHandlers) MemoryGrantWaiters(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		rows, err := h.metricsSvc.MsRepo.FetchMemoryGrantWaiters(instance)
		if err != nil {
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]any{"memory_grant_waiters": []any{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]any{"memory_grant_waiters": rows})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		if rows, err := h.metricsSvc.GetTimescaleMemoryGrantWaiters(r.Context(), instance, 50); err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]any{"memory_grant_waiters": rows})
			return
		}
	}

	w.Header().Set("X-Data-Source", "timescale_unavailable")
	json.NewEncoder(w).Encode(map[string]any{"memory_grant_waiters": []any{}})
}

func (h *SqlServerHandlers) TempdbTopConsumers(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	preferLive := sqlserverPreferLiveSource(r)
	if preferLive {
		rows, err := h.metricsSvc.MsRepo.FetchTempdbTopConsumers(instance)
		if err != nil {
			w.Header().Set("X-Data-Source", "live_dmv_error")
			json.NewEncoder(w).Encode(map[string]any{"tempdb_top_consumers": []any{}})
			return
		}
		w.Header().Set("X-Data-Source", "live_dmv")
		json.NewEncoder(w).Encode(map[string]any{"tempdb_top_consumers": rows})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		if rows, err := h.metricsSvc.GetTimescaleTempdbTopConsumers(r.Context(), instance, 50); err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]any{"tempdb_top_consumers": rows})
			return
		}
	}

	w.Header().Set("X-Data-Source", "timescale_unavailable")
	json.NewEncoder(w).Encode(map[string]any{"tempdb_top_consumers": []any{}})
}

func (h *SqlServerHandlers) WaitCategories(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if sqlserverPreferLiveSource(r) {
		// Wait categories are derived from Timescale wait deltas; no cheap live DMV equivalent.
		w.Header().Set("X-Data-Source", "live_unsupported")
		json.NewEncoder(w).Encode(map[string]any{"wait_categories_15m": []any{}})
		return
	}
	if h.metricsSvc.IsTimescaleConnected() {
		if rows, err := h.metricsSvc.GetTimescaleWaitCategoryAgg(r.Context(), instance, 15); err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]any{"wait_categories_15m": rows})
			return
		}
	}
	w.Header().Set("X-Data-Source", "timescale_unavailable")
	json.NewEncoder(w).Encode(map[string]any{"wait_categories_15m": []any{}})
}

func (h *SqlServerHandlers) CPUSchedulerStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	stats, err := h.metricsSvc.GetTimescaleCPUSchedulerStats(instance, 50)
	if err != nil {
		log.Printf("[Router] CPU Scheduler stats error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Data-Source", "timescale_unavailable")
		json.NewEncoder(w).Encode(map[string]interface{}{"cpu_scheduler_stats": []interface{}{}})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"cpu_scheduler_stats": stats})
}

func (h *SqlServerHandlers) ServerProperties(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	props, err := h.metricsSvc.GetTimescaleServerProperties(instance)
	if err != nil {
		log.Printf("[Router] Server properties error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Data-Source", "timescale_unavailable")
		json.NewEncoder(w).Encode(map[string]interface{}{"server_properties": nil})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"server_properties": props})
}
