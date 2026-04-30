// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL connection pooler statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)


func (h *PostgresHandlers) PoolerLatest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "latest": nil, "source": "none"})
		return
	}
	row, err := h.metricsSvc.GetLatestPostgresPoolerStats(r.Context(), instance)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "latest": nil, "source": "timescale"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "latest": row, "source": "timescale"})
}

func (h *PostgresHandlers) PoolerHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	limit := 180
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "history": map[string]interface{}{}, "source": "none"})
		return
	}
	rows, err := h.metricsSvc.GetPostgresPoolerStatsHistory(r.Context(), instance, limit)
	if err != nil {
		rows = []hot.PostgresPoolerStatRow{}
	}
	labels := make([]string, 0, len(rows))
	clActive := make([]int, 0, len(rows))
	clWaiting := make([]int, 0, len(rows))
	svUsed := make([]int, 0, len(rows))
	maxwait := make([]float64, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		rw := rows[i]
		labels = append(labels, rw.CaptureTimestamp.UTC().Format(time.RFC3339))
		clActive = append(clActive, rw.ClActive)
		clWaiting = append(clWaiting, rw.ClWaiting)
		svUsed = append(svUsed, rw.SvUsed)
		maxwait = append(maxwait, rw.MaxwaitSeconds)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history": map[string]interface{}{
			"labels":     labels,
			"cl_active":  clActive,
			"cl_waiting": clWaiting,
			"sv_used":    svUsed,
			"maxwait_s":  maxwait,
		},
		"source": "timescale",
	})
}
