// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handler for PostgreSQL checkpoint and BGWriter health history.
//          Reads from postgres_bgwriter_stats (TimescaleDB).
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
)

// CheckpointHealthHistory handles GET /api/postgres/checkpoint-health/history
func (h *PostgresHandlers) CheckpointHealthHistory(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance parameter is required", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 180
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			limit = val
		}
	}

	// We use the existing GetPostgresCheckpointSummary but maybe we need a more specific one for CC charts.
	// For now let's use what we have or add a new service method if needed.
	// Actually GetPostgresCheckpointSummary is what the MD suggests.
	history, err := h.metricsSvc.GetPostgresCheckpointSummary(r.Context(), instance, time.Time{}, time.Time{}, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}
