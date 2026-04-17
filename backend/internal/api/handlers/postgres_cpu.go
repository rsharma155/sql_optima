// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: REST handlers for PostgreSQL CPU dashboard (history, saturation, per-DB and top query attribution).
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

// CPUHistory serves Timescale postgres_system_stats for host vs Postgres CPU over time.
// GET /api/cpu/history?instance=...&limit=60
func (h *PostgresHandlers) CPUHistory(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.metricsSvc.GetPostgresCpuHistory(instance, limit)
	if err != nil {
		log.Printf("[API] CPU history error for %s: %v", instance, err)
		rows = []hot.PostgresSystemStatsRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"points":   rows,
	})
}

// CPUSaturation returns load-based saturation % and CPU per active connection.
// GET /api/cpu/saturation?instance=...
func (h *PostgresHandlers) CPUSaturation(w http.ResponseWriter, r *http.Request) {
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

	json.NewEncoder(w).Encode(h.metricsSvc.GetPostgresCpuSaturation(instance))
}

// CPUDatabase returns cumulative execution time by database (pg_stat_statements; same data as mv_pg_cpu_by_db).
// GET /api/cpu/database?instance=...
func (h *PostgresHandlers) CPUDatabase(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.metricsSvc.PgRepo.GetCpuTimeByDatabase(instance)
	if err != nil {
		log.Printf("[API] CPU by database error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"rows":     []interface{}{},
			"error":    err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"rows":     rows,
	})
}

// CPUTopQueries returns top statements by total_exec_time (mv_pg_top_cpu_queries shape).
// GET /api/cpu/top-queries?instance=...&limit=20
func (h *PostgresHandlers) CPUTopQueries(w http.ResponseWriter, r *http.Request) {
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

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := h.metricsSvc.PgRepo.GetTopCpuQueries(instance, limit)
	if err != nil {
		log.Printf("[API] CPU top queries error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"queries":  []interface{}{},
			"error":    err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"queries":  rows,
	})
}
