// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for the enhanced pg_stat_statements dashboard API
//
//	including workload time-series, latency, top queries, regressions, and status.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
	_ "github.com/rsharma155/sql_optima/pkg/logger" // ensure redact handler is initialised
)

// PgssStatus checks whether pg_stat_statements is active on the target instance.
func (h *PostgresHandlers) PgssStatus(w http.ResponseWriter, r *http.Request) {
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

	status := h.metricsSvc.GetPgssStatus(r.Context(), instance)
	json.NewEncoder(w).Encode(status)
}

// PgssWorkload returns per-minute workload time-series for the dashboard charts.
func (h *PostgresHandlers) PgssWorkload(w http.ResponseWriter, r *http.Request) {
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
	points, err := h.metricsSvc.GetPgssWorkload(r.Context(), instance, from, to)
	if err != nil {
		slog.Error("pgss_workload_error", "instance", instance, "error", err)
		points = nil
	}
	json.NewEncoder(w).Encode(models.PgssWorkloadResponse{Instance: instance, Points: points})
}

// PgssLatency returns latency percentile time-series.
func (h *PostgresHandlers) PgssLatency(w http.ResponseWriter, r *http.Request) {
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
	points, err := h.metricsSvc.GetPgssLatency(r.Context(), instance, from, to)
	if err != nil {
		slog.Error("pgss_latency_error", "instance", instance, "error", err)
		points = nil
	}
	json.NewEncoder(w).Encode(models.PgssLatencyResponse{Instance: instance, Points: points})
}

// PgssTop returns top queries sorted by the requested metric.
func (h *PostgresHandlers) PgssTop(w http.ResponseWriter, r *http.Request) {
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
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortBy == "" {
		sortBy = "total_time"
	}
	// Allowlist sort values
	switch sortBy {
	case "total_time", "mean_time", "calls", "io", "temp", "wal", "planning":
	default:
		sortBy = "total_time"
	}

	queries, err := h.metricsSvc.GetPgssTopQueries(r.Context(), instance, from, to, sortBy, 50)
	if err != nil {
		slog.Error("pgss_top_queries_error", "instance", instance, "sort", sortBy, "error", err)
		queries = nil
	}
	json.NewEncoder(w).Encode(models.PgssTopQueriesResponse{Instance: instance, SortBy: sortBy, Queries: queries})
}

// PgssRegressions returns queries with degraded performance (last 30m vs previous 30m).
func (h *PostgresHandlers) PgssRegressions(w http.ResponseWriter, r *http.Request) {
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

	regressions, err := h.metricsSvc.GetPgssRegressions(r.Context(), instance)
	if err != nil {
		slog.Error("pgss_regressions_error", "instance", instance, "error", err)
		regressions = nil
	}
	json.NewEncoder(w).Encode(models.PgssRegressionsResponse{Instance: instance, Regressions: regressions})
}

// PgssSummary returns aggregate KPI metrics for the incident summary strip.
func (h *PostgresHandlers) PgssSummary(w http.ResponseWriter, r *http.Request) {
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
	summary, err := h.metricsSvc.GetPgssSummary(r.Context(), instance, from, to)
	if err != nil {
		slog.Error("pgss_summary_error", "instance", instance, "error", err)
		json.NewEncoder(w).Encode(models.PgssSummaryResponse{Instance: instance})
		return
	}
	if summary == nil {
		json.NewEncoder(w).Encode(models.PgssSummaryResponse{Instance: instance})
		return
	}
	json.NewEncoder(w).Encode(summary)
}

// parseTimeRange extracts from/to from query params with 1-hour default.
func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	q := r.URL.Query()
	toT := time.Now().UTC()
	fromT := toT.Add(-1 * time.Hour)
	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			fromT = t
		}
	}
	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			toT = t
		}
	}
	if fromT.After(toT) {
		fromT, toT = toT, fromT
	}
	// Cap range to 7 days to prevent expensive wide-range scans
	const maxRange = 7 * 24 * time.Hour
	if toT.Sub(fromT) > maxRange {
		fromT = toT.Add(-maxRange)
	}
	return fromT, toT
}
