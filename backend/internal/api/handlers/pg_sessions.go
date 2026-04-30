// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL session management and transaction monitoring.
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
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)


func (h *PostgresHandlers) SessionStateHistory(w http.ResponseWriter, r *http.Request) {
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

	limit := 180
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"history":  map[string]interface{}{},
			"source":   "none",
		})
		return
	}

	rows, err := h.metricsSvc.GetPostgresSessionStateCountsHistory(r.Context(), instance, limit)
	if err != nil {
		rows = []hot.PostgresSessionStateCountRow{}
	}
	// Return arrays for charting convenience (similar to control-center/history).
	labels := make([]string, 0, len(rows))
	active := make([]int, 0, len(rows))
	idle := make([]int, 0, len(rows))
	idleInTxn := make([]int, 0, len(rows))
	waiting := make([]int, 0, len(rows))
	total := make([]int, 0, len(rows))

	for i := len(rows) - 1; i >= 0; i-- { // oldest -> newest
		rw := rows[i]
		labels = append(labels, rw.CaptureTimestamp.UTC().Format(time.RFC3339))
		active = append(active, rw.ActiveCount)
		idle = append(idle, rw.IdleCount)
		idleInTxn = append(idleInTxn, rw.IdleInTxnCount)
		waiting = append(waiting, rw.WaitingCount)
		total = append(total, rw.TotalCount)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history": map[string]interface{}{
			"labels":      labels,
			"active":      active,
			"idle":        idle,
			"idle_in_txn": idleInTxn,
			"waiting":     waiting,
			"total":       total,
		},
		"source": "timescale",
	})
}

func (h *PostgresHandlers) Sessions(w http.ResponseWriter, r *http.Request) {
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

	sessions, err := h.metricsSvc.GetLatestPostgresSessions(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG sessions (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"sessions": []interface{}{}, "error": err.Error()})
		return
	}
	w.Header().Set("X-Data-Source", "timescale")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"sessions": sessions,
		"count":    len(sessions),
	})
}


func (h *PostgresHandlers) KillSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	instance := r.URL.Query().Get("instance")
	pidStr := r.URL.Query().Get("pid")
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

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid pid"})
		return
	}

	err = h.metricsSvc.PgRepo.TerminateSession(instance, pid)
	if err != nil {
		log.Printf("[API] PG kill-session error for %s pid %d: %v", instance, pid, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *PostgresHandlers) IdleInTransaction(w http.ResponseWriter, r *http.Request) {
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

	sessions, err := h.metricsSvc.GetLatestPostgresSessions(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG idle-in-transaction error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"sessions": []interface{}{}, "error": err.Error()})
		return
	}

	// Filter for idle in transaction
	var filtered []hot.PgSessionSnapshotRow
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.State), "idle in transaction") {
			filtered = append(filtered, s)
		}
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"sessions": filtered,
		"count":    len(filtered),
	})
}

// LongRunningTransactions returns transactions active for a significant duration.
func (h *PostgresHandlers) LongRunningTransactions(w http.ResponseWriter, r *http.Request) {
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

	sessions, err := h.metricsSvc.GetLatestPostgresSessions(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG long-running-transactions error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"transactions": []interface{}{}, "error": err.Error()})
		return
	}

	// Filter for transactions older than 1 minute
	var filtered []hot.PgSessionSnapshotRow
	now := time.Now().UTC()
	for _, s := range sessions {
		if s.XactStart != nil && now.Sub(*s.XactStart) > 1*time.Minute {
			filtered = append(filtered, s)
		}
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":     instance,
		"transactions": filtered,
		"count":        len(filtered),
	})
}


// IndexBloat returns index size and usage signals to help identify bloated or unused indexes.
