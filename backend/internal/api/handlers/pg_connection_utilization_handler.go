// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handler for PostgreSQL connection utilization history.
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

// ConnectionUtilizationHistory handles GET /api/postgres/connection-utilization
func (h *PostgresHandlers) ConnectionUtilizationHistory(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance parameter is required", http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	limitStr := r.URL.Query().Get("limit")
	limit := 180
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			limit = val
		}
	}

	// Sourced from Control Center History which now includes connections_usage_pct
	history, err := h.metricsSvc.GetPostgresControlCenterHistory(r.Context(), instance, from, to, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"labels":                history.Labels,
		"connections_usage_pct": history.ConnectionsUsagePct,
	})
}
