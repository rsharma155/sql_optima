// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL query analysis and statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
)


func (h *PostgresHandlers) Queries(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	toT := time.Now().UTC()
	fromT := toT.Add(-1 * time.Hour)
	if fromStr != "" && toStr != "" {
		var perr error
		fromT, perr = time.Parse(time.RFC3339, fromStr)
		if perr != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid from (use RFC3339, e.g. 2026-04-10T12:00:00Z)"})
			return
		}
		toT, perr = time.Parse(time.RFC3339, toStr)
		if perr != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid to (use RFC3339)"})
			return
		}
	}

	queries, meta, err := h.metricsSvc.GetPostgresQueriesForAPI(r.Context(), instance, fromT, toT)
	
	// Check if pg_stat_statements is actually enabled on the instance
	enabled := h.metricsSvc.PgRepo.GetPgssSupported(instance)

	if queries == nil {
		queries = []repository.PgQueryStat{}
	}
	if err != nil {
		log.Printf("[API] PG queries error for %s: %v", instance, err)
	}

	// Fetch currently active queries for Control Center's Long Running Queries and Incident Feed
	activeQueries, _ := h.metricsSvc.PgRepo.GetActiveQueries(instance)
	if activeQueries == nil {
		activeQueries = []models.PgSession{}
	}

	resp := map[string]interface{}{
		"instance":                   instance,
		"queries":                    queries,
		"active_queries":             activeQueries,
		"pg_stat_statements_enabled": enabled,
		"collected_at":               time.Now().UTC(),
	}
	for k, v := range meta {
		resp[k] = v
	}
	if err == nil && meta != nil {
		if ec, ok := meta["end_capture"].(string); ok && ec != "" {
			if t, perr := time.Parse(time.RFC3339, ec); perr == nil {
				resp["collected_at"] = t
			}
		}
	}
	if err != nil {
		resp["error"] = err.Error()
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *PostgresHandlers) ResetQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
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

	err := h.metricsSvc.PgRepo.ResetQueryStats(instance)
	if err != nil {
		log.Printf("[API] PG reset-queries error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}
