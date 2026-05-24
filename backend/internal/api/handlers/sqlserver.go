// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for SQL Server dashboard endpoints including overview, CPU drilldown, memory, waits, jobs, and performance debt metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"log/slog"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	ha_repo "github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/repository"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SqlServerHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *SqlServerHandlers {
	return &SqlServerHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *SqlServerHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	return ParseServerID(r, h.cfg)
}


// DashboardTimeSeries returns historical risk health snapshots for a time window.
func (h *SqlServerHandlers) DashboardTimeSeries(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
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

	hours := int(to.Sub(from).Hours())
	if hours < 1 {
		hours = 1
	}

	history, err := h.metricsSvc.GetSQLServerRiskHealthHistory(r.Context(), serverID, hours)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"server_id": serverID,
		"from":      from.Format(time.RFC3339),
		"to":        to.Format(time.RFC3339),
		"series":    history,
	})
}

// TableDrilldown returns a comprehensive analytical package for a specific table (SQL Server).
func (h *SqlServerHandlers) TableDrilldown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	db := r.URL.Query().Get("db")
	table := r.URL.Query().Get("table")

	serverID, ok := h.parseID(r)
	if !ok || db == "" || table == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance/server_id, db, and table are required"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"server_id":     serverID,
		"database":      db,
		"table":         table,
		"growth_series": []interface{}{},
		"index_usage":   []interface{}{},
		"fragmentation": []interface{}{},
	})
}

// EnterpriseDashboardV2 returns time-series standardized enterprise metrics for a specific window.
func (h *SqlServerHandlers) EnterpriseDashboardV2(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.parseID(r)
	if !ok {
		http.Error(w, "instance name or server_id required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	fromT, toT := now.Add(-1*time.Hour), now
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			fromT = t.UTC()
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			toT = t.UTC()
		}
	}

	ctx := r.Context()
	res := make(map[string]interface{})

	// 1. Wait Stats (key must match JS: data.wait_trends)
	waits, _ := h.metricsSvc.GetSqlServerWaitStatsTimeSeries(ctx, serverID, fromT, toT)
	res["wait_trends"] = waits

	// 2. Perf Counters
	perf, _ := h.metricsSvc.GetSqlServerPerfCountersTimeSeries(ctx, serverID, fromT, toT)
	res["perf_counters"] = perf

	// 3. File IO (database-level detail for Enterprise Metrics)
	io, _ := h.metricsSvc.GetSqlServerFileIO(ctx, serverID, fromT.Format(time.RFC3339), toT.Format(time.RFC3339))
	res["file_io"] = io

	// 4. Plan Cache
	cache, _ := h.metricsSvc.GetSqlServerPlanCacheTimeSeries(ctx, serverID, fromT, toT)
	res["plan_cache"] = cache

	// 5. Memory Clerks
	clerks, _ := h.metricsSvc.GetSqlServerMemoryClerksTimeSeries(ctx, serverID, fromT, toT)
	res["memory_clerks"] = clerks

	// 6. Memory Grants
	grants, _ := h.metricsSvc.GetSqlServerMemoryGrantsTimeSeries(ctx, serverID, fromT, toT)
	res["memory_grants"] = grants

	// 7. Throughput (batch requests, connections, logins/sec)
	throughput, _ := h.metricsSvc.GetSqlServerThroughputTimeSeries(ctx, serverID, fromT, toT)
	res["throughput"] = throughput

	// 8. Latest KPI snapshot from TimescaleDB (replaces stale in-memory cache)
	if h.metricsSvc.IsTimescaleConnected() {
		if kpis, _, err := h.metricsSvc.GetLatestSqlServerHealthKPIs(ctx, serverID); err == nil {
			res["snapshot"] = kpis
		} else {
			res["snapshot"] = map[string]interface{}{}
		}
	} else {
		res["snapshot"] = map[string]interface{}{}
	}

	// 9. TempDB Top Consumers
	consumers, _ := h.metricsSvc.GetSqlServerTempdbConsumers(ctx, serverID, fromT.Format(time.RFC3339), toT.Format(time.RFC3339))
	if consumers == nil {
		consumers = []map[string]interface{}{}
	}
	res["tempdb_consumers"] = consumers

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(res)
}

// WaitStatsDashboardV2 returns the full data package for the new Wait Stats Dashboard.
// When TimescaleDB is not connected or has no data yet, it returns HTTP 200 with an
// empty-but-valid payload so the frontend can show "collecting…" states instead of errors.
func (h *SqlServerHandlers) WaitStatsDashboardV2(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.parseID(r)
	if !ok {
		http.Error(w, "instance name or server_id required", http.StatusBadRequest)
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	w.Header().Set("Content-Type", "application/json")

	if !h.metricsSvc.IsTimescaleConnected() {
		// Return a graceful empty payload — frontend shows "collecting" states.
		w.Header().Set("X-Data-Source", "none")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance_name":      r.URL.Query().Get("instance"),
			"kpis":               map[string]interface{}{},
			"wait_trends_hourly": []interface{}{},
			"wait_trends_daily":  []interface{}{},
			"top_wait_types":     []interface{}{},
			"active_waits":       []interface{}{},
			"timescale_ready":    false,
		})
		return
	}

	data, err := h.metricsSvc.GetWaitStatsDashboardV2(r.Context(), serverID, from, to)
	if err != nil {
		// Log and return empty rather than 500 — collector may not have run yet.
		slog.Error("[WaitStatsDashboardV2] serverID=", "target", serverID, "err", err)
		w.Header().Set("X-Data-Source", "error")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance_name":      r.URL.Query().Get("instance"),
			"kpis":               map[string]interface{}{},
			"wait_trends_hourly": []interface{}{},
			"wait_trends_daily":  []interface{}{},
			"top_wait_types":     []interface{}{},
			"active_waits":       []interface{}{},
			"timescale_ready":    true,
			"error":              err.Error(),
		})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) Overview(w http.ResponseWriter, r *http.Request) {
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
	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}
	serverID, _ := h.parseID(r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.metricsSvc.GetSqlServerOverview(serverID))
}

func (h *SqlServerHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	source := r.URL.Query().Get("source")

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

	serverID, _ := h.parseID(r)
	w.Header().Set("Content-Type", "application/json")

	cached := h.metricsSvc.GetCachedDashboard(serverID)

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

	tsData, err := h.metricsSvc.GetDashboardFromTimescale(serverID)
	if err != nil {
		slog.Error("[Router] TimescaleDB fetch failed", "target", instance, "err", err)
		w.Header().Set("X-Data-Source", "live_cache_fallback")
		json.NewEncoder(w).Encode(cached)
		return
	}

	merged, err := mergeDashboardCacheWithTimescale(cached, tsData)
	if err != nil {
		slog.Error("[Router] Failed to merge Timescale dashboard data", "target", instance, "err", err)
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
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid server identifier"})
		return
	}
	// On-demand refresh disabled to strictly enforce data from TimescaleDB/background collector.
	// This ensures dashboard loads are fast and only serve persisted data.

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	out, src := h.metricsSvc.GetDashboardHomepageV2WithSource(id, from, to)
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
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
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

	serverID, ok := h.parseID(r)
	if !ok {
		w.Header().Set("X-Data-Source", "timescale")
		json.NewEncoder(w).Encode(map[string]any{"findings": []any{}})
		return
	}

	findings, err := h.metricsSvc.GetSqlServerPerformanceDebt(r.Context(), serverID, lookback, dbFilter)
	if err != nil {
		slog.Error("[PerformanceDebt] error", "err", err)
	}

	// Default to 24 hours if no findings in 2 hours to avoid empty page
	if len(findings.Findings) == 0 && lookback == 2 {
		findings, _ = h.metricsSvc.GetSqlServerPerformanceDebt(r.Context(), serverID, 24, dbFilter)
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(findings)
}

func (h *SqlServerHandlers) TriggerPerformanceDebtScan(w http.ResponseWriter, r *http.Request) {
	h.metricsSvc.TriggerPerformanceDebtCollector()
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"message": "Performance debt scan triggered successfully"})
}

func (h *SqlServerHandlers) Jobs(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB connection required for job history"})
		return
	}

	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	ctx := r.Context()
	jobs, _ := h.metricsSvc.GetSQLServerJobDetails(ctx, serverID, from, to)
	schedules, _ := h.metricsSvc.GetSQLServerJobSchedules(ctx, serverID, from, to)
	failures, _ := h.metricsSvc.GetSQLServerJobFailures(ctx, serverID, from, to, 100)
	metrics, _ := h.metricsSvc.GetSQLServerJobMetrics(ctx, serverID, from, to, 1)

	if jobs == nil {
		jobs = []map[string]interface{}{}
	}
	if schedules == nil {
		schedules = []map[string]interface{}{}
	}
	if failures == nil {
		failures = []map[string]interface{}{}
	}

	summary := map[string]interface{}{
		"total_jobs": 0, "enabled_jobs": 0, "disabled_jobs": 0, "running_jobs": 0, "failed_jobs": 0,
	}
	if len(metrics) > 0 {
		m := metrics[0]
		summary["total_jobs"] = m["total_jobs"]
		summary["enabled_jobs"] = m["enabled_jobs"]
		summary["disabled_jobs"] = m["disabled_jobs"]
		summary["running_jobs"] = m["running_jobs"]
		summary["failed_jobs"] = m["failed_jobs_24h"]
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"summary":   summary,
		"jobs":      jobs,
		"schedules": schedules,
		"failures":  failures,
	})
}

// LogShipping returns log shipping health — Timescale-first with live MSDB fallback.
func (h *SqlServerHandlers) LogShipping(w http.ResponseWriter, r *http.Request) {
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

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	serverID, _ := h.parseID(r)
	w.Header().Set("Content-Type", "application/json")

	rows, err := h.metricsSvc.GetSQLServerLogShippingHealth(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to retrieve log shipping health"})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
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

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.Header().Set("X-Data-Source", "not_implemented")
	json.NewEncoder(w).Encode([]interface{}{})
}

func (h *SqlServerHandlers) BestPractices(w http.ResponseWriter, r *http.Request) {
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

	serverID, _ := h.parseID(r)
	w.Header().Set("Content-Type", "application/json")
	result, _ := h.metricsSvc.GetBestPractices(r.Context(), serverID)
	json.NewEncoder(w).Encode(result)
}

func (h *SqlServerHandlers) Guardrails(w http.ResponseWriter, r *http.Request) {
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

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	serverID, _ := h.parseID(r)
	w.Header().Set("Content-Type", "application/json")
	result, _ := h.metricsSvc.GetGuardrails(r.Context(), serverID)
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

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
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
	// source=live triggers an on-demand collector refresh before reading TimescaleDB
	preferLive := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("source")), "live")
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
	serverID, _ := h.parseID(r)
	tsLogger := h.metricsSvc.GetTimescaleDBLogger()

	if preferLive {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		h.metricsSvc.RunLiveCollectorForInstance(ctx, serverID)
		cancel()
	}

	if h.metricsSvc.IsTimescaleConnected() && fromQ != "" && toQ != "" {
		queries, err := tsLogger.GetSQLServerTopQueriesWithRange(r.Context(), serverID, limit, fromQ, toQ, dbFilter)
		if err == nil {
			normalizeTopQueryTimestamps(queries)
			w.Header().Set("X-Data-Source", "timescale_range")
			json.NewEncoder(w).Encode(map[string]interface{}{"queries": queries, "count": len(queries)})
			return
		}
		slog.Error("[Router] Timescale top queries (range) failed", "target", instance, "err", err)
	}

	if h.metricsSvc.IsTimescaleConnected() {
		queries, err := tsLogger.GetSQLServerTopQueries(r.Context(), serverID, limit, dbFilter)
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
		slog.Error("[Router] Timescale top queries failed", "target", instance, "err", err)
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

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
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
	serverID, _ := h.parseID(r)
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"hadr_enabled": false, "ag_health": []interface{}{}, "ag_stats": []interface{}{}})
		return
	}
	stats, err := h.metricsSvc.GetSQLServerAGHealth(r.Context(), serverID)
	if err != nil {
		slog.Error("[Router] AG Health error", "err", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"hadr_enabled": false, "ag_health": []interface{}{}, "ag_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
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

	serverID, _ := h.parseID(r)
	history, err := h.metricsSvc.GetTimescaleDBLogger().GetAGHealthTimeSeries(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"history": history})
}

func (h *SqlServerHandlers) AGClusterStatus(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.parseID(r)
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"hadr_cluster": nil, "members": []interface{}{}})
		return
	}

	repo := ha_repo.NewHAReplicationRepository(pool)
	info, err := repo.GetLatestAGClusterInfo(r.Context(), serverID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"hadr_cluster": nil, "members": []interface{}{}})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hadr_cluster": info,
		"members":      info.Members,
	})
}

func (h *SqlServerHandlers) ReplicationStatus(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.parseID(r)
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"replication": []interface{}{}})
		return
	}

	repo := ha_repo.NewHAReplicationRepository(pool)
	rows, err := repo.GetReplicationTopology(r.Context(), serverID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"replication": []interface{}{}})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"replication": rows})
}



func (h *SqlServerHandlers) DBThroughput(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	serverID, _ := h.parseID(r)
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"db_throughput": []interface{}{}, "db_stats": []interface{}{}})
		return
	}
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetDatabaseThroughputSummary(r.Context(), serverID, 100)
	if err != nil {
		slog.Error("[Router] DB Throughput error", "err", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"db_throughput": []interface{}{}, "db_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"db_throughput": stats, "db_stats": stats})
}

func (h *SqlServerHandlers) LatchStats(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	serverID, _ := h.parseID(r)
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": []interface{}{}})
		return
	}
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetLatchWaits(r.Context(), serverID, 50)
	if err != nil {
		slog.Error("[Router] Latch stats error", "err", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": stats})
}

func (h *SqlServerHandlers) WaitingTasks(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	serverID, _ := h.parseID(r)
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": []interface{}{}})
		return
	}
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetWaitingTasks(r.Context(), serverID, 50)
	if err != nil {
		slog.Error("[Router] Waiting tasks error", "err", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": stats})
}

func (h *SqlServerHandlers) MemoryGrants(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	serverID, _ := h.parseID(r)
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": []interface{}{}})
		return
	}
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetMemoryGrants(r.Context(), serverID, 50)
	if err != nil {
		slog.Error("[Router] Memory grants error", "err", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": stats})
}

func (h *SqlServerHandlers) SchedulerWorkers(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	serverID, _ := h.parseID(r)
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"scheduler_wg": []interface{}{}, "scheduler_workers": []interface{}{}})
		return
	}
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetSchedulerWG(r.Context(), serverID, 50)
	if err != nil {
		slog.Error("[Router] Scheduler worker stats error", "err", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"scheduler_wg": []interface{}{}, "scheduler_workers": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"scheduler_wg": stats, "scheduler_workers": stats})
}

func (h *SqlServerHandlers) ProcedureStats(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": []interface{}{}})
		return
	}
	serverID, _ := h.parseID(r)
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetProcedureStats(r.Context(), serverID, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": stats})
}

func (h *SqlServerHandlers) FileIOLatency(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": []interface{}{}})
		return
	}
	serverID, _ := h.parseID(r)
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetFileIOLatency(r.Context(), serverID, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": stats})
}

func (h *SqlServerHandlers) SpinlockStats(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": []interface{}{}})
		return
	}
	serverID, _ := h.parseID(r)
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetSpinlockStats(r.Context(), serverID, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": stats})
}

func (h *SqlServerHandlers) MemoryClerks(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": []interface{}{}})
		return
	}
	serverID, _ := h.parseID(r)
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetMemoryClerks(r.Context(), serverID, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": stats})
}

func (h *SqlServerHandlers) TempdbStats(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": []interface{}{}})
		return
	}
	serverID, _ := h.parseID(r)
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetTempdbFiles(r.Context(), serverID, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": stats})
}

func (h *SqlServerHandlers) PlanCacheHealth(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]any{"plan_cache_health": []any{}})
		return
	}
	serverID, _ := h.parseID(r)
	rows, err := h.metricsSvc.GetTimescaleDBLogger().GetPlanCacheHealth(r.Context(), serverID, 60)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]any{"plan_cache_health": rows})
}

func (h *SqlServerHandlers) MemoryGrantWaiters(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]any{"memory_grant_waiters": []any{}})
		return
	}
	serverID, _ := h.parseID(r)
	rows, err := h.metricsSvc.GetTimescaleDBLogger().GetMemoryGrantWaiters(r.Context(), serverID, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]any{"memory_grant_waiters": rows})
}

func (h *SqlServerHandlers) TempdbTopConsumers(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]any{"tempdb_top_consumers": []any{}})
		return
	}
	serverID, _ := h.parseID(r)
	rows, err := h.metricsSvc.GetTimescaleDBLogger().GetTempdbTopConsumers(r.Context(), serverID, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]any{"tempdb_top_consumers": rows})
}

func (h *SqlServerHandlers) WaitCategories(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]any{"wait_categories_15m": []any{}})
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	serverID, _ := h.parseID(r)
	rows, err := h.metricsSvc.GetTimescaleDBLogger().GetWaitCategoryAgg(r.Context(), serverID, 15, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]any{"wait_categories_15m": rows})
}

func (h *SqlServerHandlers) CPUSchedulerStats(w http.ResponseWriter, r *http.Request) {
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

	serverID, _ := h.parseID(r)
	stats, err := h.metricsSvc.GetTimescaleDBLogger().GetCPUSchedulerStats(r.Context(), serverID, 50)
	if err != nil {
		slog.Error("[Router] CPU Scheduler stats error", "err", err)
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

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	serverID, _ := h.parseID(r)
	props, err := h.metricsSvc.GetTimescaleDBLogger().GetServerProperties(r.Context(), serverID)
	if err != nil {
		slog.Error("[Router] Server properties error", "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Data-Source", "timescale_unavailable")
		json.NewEncoder(w).Encode(map[string]interface{}{"server_properties": nil})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{"server_properties": props})
}

// HealthV2 returns the unified SQL Server Health Dashboard v2 data.
func (h *SqlServerHandlers) HealthV2(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	data, err := h.metricsSvc.GetSQLServerHealthV2(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) ExportBestPracticesCSV(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (h *SqlServerHandlers) ExportGuardrailsCSV(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (h *SqlServerHandlers) BlockingKPIs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.GetSQLServerBlockingKPIs(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) BlockingTimeline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetSQLServerBlockingTimeline(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) BlockingDetails(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetSQLServerBlockingDetails(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) BlockingLocks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetSQLServerBlockingLocks(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) MostBlockedDatabases(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetSQLServerMostBlockedDatabases(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) MostBlockedObjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetSQLServerMostBlockedObjects(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) BlockingRecurrence(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sqlHash := r.URL.Query().Get("sql_hash")
	loginName := r.URL.Query().Get("login")
	data, err := h.metricsSvc.GetTimescaleDBLogger().GetSQLServerBlockingRecurrence(r.Context(), serverID, sqlHash, loginName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) TopBlockingQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetSQLServerTopBlockingQueries(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (h *SqlServerHandlers) DeadlockHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetTimescaleDBLogger().GetSQLServerDeadlockHistory(r.Context(), serverID, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	
	// Wrap in object for frontend which expects { events: [], enabled: true, message: "" }
	res := map[string]interface{}{
		"events":  data,
		"enabled": true,
		"message": "",
	}
	if data == nil {
		res["events"] = []interface{}{}
	}

	json.NewEncoder(w).Encode(res)
}


func (h *SqlServerHandlers) GetServerVitals(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	vitals, err := h.metricsSvc.GetSqlServerVitals(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(vitals)
}

func (h *SqlServerHandlers) GetLiveVolumeStats(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	instanceName := h.metricsSvc.GetServerName(serverID)
	vols, err := h.metricsSvc.MsRepo.FetchVolumeStats(r.Context(), instanceName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"volumes": vols})
}
