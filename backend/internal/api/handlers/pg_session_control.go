// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL session control (cancel/terminate backend).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

type sessionControlRequest struct {
	Instance string `json:"instance"`
	PID      int    `json:"pid"`
}

// CancelBackend calls pg_cancel_backend(pid) on the monitored PostgreSQL instance.
// Returns {"ok":true,"cancelled":true/false} — false means the PID was already gone.
func (h *PostgresHandlers) CancelBackend(w http.ResponseWriter, r *http.Request) {
	h.runSessionControl(w, r, "SELECT pg_cancel_backend($1)", "cancel")
}

// TerminateBackend calls pg_terminate_backend(pid) on the monitored PostgreSQL instance.
func (h *PostgresHandlers) TerminateBackend(w http.ResponseWriter, r *http.Request) {
	h.runSessionControl(w, r, "SELECT pg_terminate_backend($1)", "terminate")
}

func (h *PostgresHandlers) runSessionControl(w http.ResponseWriter, r *http.Request, query, action string) {
	w.Header().Set("Content-Type", "application/json")

	var req sessionControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request: instance and pid required"})
		return
	}

	if err := validateInstanceName(req.Instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, req.Instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, req.Instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "postgres instance not found"})
		return
	}

	db, ok := h.metricsSvc.PgRepo.GetConn(req.Instance)
	if !ok || db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no active connection to instance"})
		return
	}

	var result bool
	err := db.QueryRowContext(r.Context(), query, req.PID).Scan(&result)
	if err != nil {
		slog.Error(fmt.Sprintf("[SessionControl] %s pid=%d instance=%s error: %v", action, req.PID, req.Instance, err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "pg_signal_backend may not be granted to the monitoring role; grant it with: GRANT pg_signal_backend TO <monitoring_user>",
		})
		return
	}

	slog.Info(fmt.Sprintf("[SessionControl] %s pid=%d instance=%s result=%v", action, req.PID, req.Instance, result))
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, action + "d": result})
}

// KillSession terminates a backend by PID (query params: instance, pid).
// Used by the sessions dashboard; returns {success: true} for UI compatibility.
func (h *PostgresHandlers) KillSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	pidStr := strings.TrimSpace(r.URL.Query().Get("pid"))
	if instance == "" || pidStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance and pid are required"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid pid"})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "postgres instance not found"})
		return
	}

	db, ok := h.metricsSvc.PgRepo.GetConn(instance)
	if !ok || db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no active connection to instance"})
		return
	}

	var terminated bool
	if err := db.QueryRowContext(r.Context(), "SELECT pg_terminate_backend($1)", pid).Scan(&terminated); err != nil {
		slog.Error("[SessionControl] kill-session failed", "instance", instance, "pid", pid, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "pg_signal_backend may not be granted; GRANT pg_signal_backend TO <monitoring_user>",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "terminated": terminated})
}

// ResetQueries clears pg_stat_statements counters on the monitored instance.
func (h *PostgresHandlers) ResetQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instance is required"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "postgres instance not found"})
		return
	}
	if h.metricsSvc == nil || h.metricsSvc.PgRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "postgres repository unavailable"})
		return
	}

	if err := h.metricsSvc.PgRepo.ResetQueryStats(r.Context(), instance); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}
