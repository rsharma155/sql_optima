// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handler for the PostgreSQL unified incident feed.
//          Reads exclusively from monitor.pg_incident_feed_ts (TimescaleDB).
//          Never queries monitored servers directly.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// IncidentFeed handles GET /api/postgres/incident-feed
func (h *PostgresHandlers) IncidentFeed(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance parameter is required", http.StatusBadRequest)
		return
	}

	lookbackStr := r.URL.Query().Get("lookback_mins")
	lookback := 1440 // default 24h
	if lookbackStr != "" {
		if val, err := strconv.Atoi(lookbackStr); err == nil {
			lookback = val
		}
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			limit = val
		}
	}

	incidents, err := h.metricsSvc.GetPgIncidentFeed(r.Context(), instance, lookback, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":  instance,
		"incidents": incidents,
		"total":     len(incidents),
	})
}
