// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL replication lag and slots.
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



func (h *PostgresHandlers) ReplicationLagHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 120
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1440 {
			limit = n
		}
	}
	series, err := h.metricsSvc.GetPostgresReplicationLagDetail(r.Context(), instance, from, to, limit)
	if err != nil {
		log.Printf("[API] PG replication lag history error for %s: %v", instance, err)
		series = map[string]hot.PostgresReplicationLagSeries{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"series":   series,
	})
}

func (h *PostgresHandlers) ReplicationSlots(w http.ResponseWriter, r *http.Request) {
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

	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}

	// Strictly TimescaleDB as per requirements
	if h.metricsSvc.IsTimescaleConnected() {
		rows, err := h.metricsSvc.GetTimescalePostgresReplicationSlots(instance, limit)
		if err != nil {
			log.Printf("[API] PG replication-slots error for %s: %v", instance, err)
			rows = []hot.PostgresReplicationSlotRow{}
		}
		w.Header().Set("X-Data-Source", "timescale")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"slots":    rows,
			"source":   "timescale",
		})
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{"error": "timescale not connected"})
}

func (h *PostgresHandlers) Replication(w http.ResponseWriter, r *http.Request) {
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

	stats, err := h.metricsSvc.GetLatestPostgresReplicationStats(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG replication (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"stats": map[string]interface{}{
				"is_primary":    true,
				"cluster_state": "unknown",
				"max_lag_mb":    0,
				"standbys":      []interface{}{},
			},
			"source": "none",
		})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    stats,
		"source":   "timescale",
	})
}


