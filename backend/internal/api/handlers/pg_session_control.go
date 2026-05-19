// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL session control (cancel/terminate backend).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"fmt"
	"log/slog"
	"encoding/json"
	"net/http"
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
