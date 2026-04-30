// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL control center views.
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


func (h *PostgresHandlers) ControlCenter(w http.ResponseWriter, r *http.Request) {
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

	//from := r.URL.Query().Get("from")
	//to := r.URL.Query().Get("to")

	row, err := h.metricsSvc.GetLatestPostgresControlCenterStats(r.Context(), instance)
	// If timescale is connected and we have from/to, maybe we should get stats from history?
	// But usually these KPIs are "Right Now".
	if err != nil {
		log.Printf("[API] PG control-center error for %s: %v", instance, err)
		row = nil
	}
	if h.metricsSvc.IsTimescaleConnected() {
		w.Header().Set("X-Data-Source", "timescale")
	} else {
		w.Header().Set("X-Data-Source", "live_cache_fallback")
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    row,
	})
}

func (h *PostgresHandlers) ControlCenterHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

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
	limit := 60
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 720 {
			limit = n
		}
	}
	hist, err := h.metricsSvc.GetPostgresControlCenterHistory(r.Context(), instance, from, to, limit)
	if err != nil {
		log.Printf("[API] PG control-center history error for %s: %v", instance, err)
		hist = &hot.PostgresControlCenterHistory{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history":  hist,
	})
}
