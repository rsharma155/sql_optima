// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL core statistics and dashboard metrics.
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

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)



func (h *PostgresHandlers) DashboardV2(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	data, err := h.metricsSvc.GetPgDashboardV2(r.Context(), instance)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) DBObservation(w http.ResponseWriter, r *http.Request) {
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

	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}

	metrics := h.metricsSvc.GetPostgresDBObservationMetrics(instance)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (h *PostgresHandlers) Overview(w http.ResponseWriter, r *http.Request) {
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

	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		w.Header().Set("X-Data-Source", "timescale")
	} else {
		w.Header().Set("X-Data-Source", "cache_only")
	}
	json.NewEncoder(w).Encode(h.metricsSvc.GetPostgresOverview(instance))
}

func (h *PostgresHandlers) ServerInfo(w http.ResponseWriter, r *http.Request) {
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

	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}

	// Try timescale first
	var version, uptime string
	if h.metricsSvc.IsTimescaleConnected() {
		if snap, err := h.metricsSvc.PgHealthRepo.GetLatestSnapshot(r.Context(), instance); err == nil && snap != nil {
			version = snap.Version
			uptime = snap.Uptime
			w.Header().Set("X-Data-Source", "timescale")
		}
	}

	if version == "" {
		// Only if absolutely necessary for static info, but user said NO direct hits for live.
		// However, version/uptime are relatively static. Let's return empty if not in timescale
		// to strictly follow the user request.
		version = "Unknown (Check Timescale)"
		uptime = "Unknown"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"version": version,
		"uptime":  uptime,
	})
}

func (h *PostgresHandlers) SystemStats(w http.ResponseWriter, r *http.Request) {
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

	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		w.Header().Set("X-Data-Source", "timescale")
	}

	// Use the refactored saturation method which sources from Timescale
	stats := h.metricsSvc.GetPostgresCpuSaturation(instance)
	
	// Map to expected UI fields if different
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cpu_usage":              stats["host_cpu_percent"],
		"memory_usage":           stats["memory_usage"], // Might need careful mapping from saturation map
		"total_memory_bytes":     0,
		"available_memory_bytes": 0,
		"shared_buffers_bytes":   0,
	})
}


func (h *PostgresHandlers) SystemStatsHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}

	limit := 120
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1440 {
			limit = n
		}
	}

	from, to := parseTimeRange(r)
	var rows []hot.PostgresSystemStatsRow
	var err error

	if from.IsZero() || to.IsZero() {
		rows, err = h.metricsSvc.GetTimescalePostgresSystemStats(instance, limit)
	} else {
		rows, err = h.metricsSvc.GetTimescalePostgresSystemStatsRange(instance, from, to)
	}

	if err != nil {
		log.Printf("[API] PG system-stats history error for %s: %v", instance, err)
		rows = []hot.PostgresSystemStatsRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    rows,
	})
}

func (h *PostgresHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	database := r.URL.Query().Get("database")
	if database == "" {
		database = "all"
	}

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Allow dashboard for any instance, even if not in config (for newly added servers)
	// if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
	// 	w.WriteHeader(http.StatusNotFound)
	// 	json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
	// 	return
	// }

	// if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
	// 	w.WriteHeader(http.StatusBadRequest)
	// 	json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
	// 	return
	// }

	thr := h.metricsSvc.GetCachedPgThroughputDashboard(instance, database)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"labels":        thr.Labels,
		"tps":           thr.Tps,
		"cache_hit_pct": thr.CacheHitPct,
	})
}

func (h *PostgresHandlers) BGWriter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}

	from, to := parseTimeRange(r)
	stats, err := h.metricsSvc.GetPostgresCheckpointSummary(r.Context(), instance, from, to, 50)
	if err != nil {
		log.Printf("[API] PG bgwriter error for %s: %v", instance, err)
		stats = []map[string]interface{}{}
	}
	if stats == nil {
		stats = []map[string]interface{}{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    stats,
	})
}

func (h *PostgresHandlers) Archiver(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}

	from, to := parseTimeRange(r)
	stats, err := h.metricsSvc.GetPostgresArchiveSummary(r.Context(), instance, from, to, 50)
	if err != nil {
		log.Printf("[API] PG archiver error for %s: %v", instance, err)
		stats = []map[string]interface{}{}
	}
	if stats == nil {
		stats = []map[string]interface{}{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    stats,
	})
}

func (h *PostgresHandlers) WaitEventsHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid instance"})
		return
	}
	limit := 400
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "rows": []interface{}{}, "source": "none"})
		return
	}

	from, to := parseTimeRange(r)
	rows, err := h.metricsSvc.GetPostgresWaitEventsHistory(r.Context(), instance, from, to, limit)
	if err != nil {
		log.Printf("[API] PG waits/history error for %s: %v", instance, err)
		rows = []hot.PostgresWaitEventRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"rows":     rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) DbIOHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid instance"})
		return
	}
	limit := 500
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "rows": []interface{}{}, "source": "none"})
		return
	}

	from, to := parseTimeRange(r)
	rows, err := h.metricsSvc.GetPostgresDbIOHistory(r.Context(), instance, from, to, limit)
	if err != nil {
		log.Printf("[API] PG io/history error for %s: %v", instance, err)
		rows = []hot.PostgresDbIORow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"rows":     rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) TimeseriesMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	metric := r.URL.Query().Get("metric")

	if instance == "" || metric == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing instance or metric"})
		return
	}

	limit := 60
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	from, to := ParseTimeRange(fromStr, toStr)

	data, err := h.metricsSvc.GetPostgresMetricHistory(r.Context(), instance, metric, from, to, limit)
	if err != nil {
		log.Printf("[API] TimeseriesMetrics error for %s (%s): %v", instance, metric, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"metric":   metric,
		"data":     data,
		"series": []map[string]interface{}{
			{
				"metric": metric,
				"points": data,
			},
		},
	})
}

func (h *PostgresHandlers) PgMemoryIntelligence(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	fromT, toT := ParseTimeRange(fromStr, toStr)

	log.Printf("[PostgresHandlers] PgMemoryIntelligence: instance=%s from=%v to=%v", instance, fromT, toT)
	data, err := h.metricsSvc.GetPgMemoryDashboardData(r.Context(), instance, fromT, toT)
	if err != nil {
		log.Printf("[PostgresHandlers] PgMemoryIntelligence error for %s: %v", instance, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(data)
}
