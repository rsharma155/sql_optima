// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server locks, deadlocks, and blocking API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SqlServerLockHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerLockHandlers(svc *service.MetricsService, cfg *config.Config) *SqlServerLockHandlers {
	return &SqlServerLockHandlers{metricsSvc: svc, cfg: cfg}
}

func (h *SqlServerLockHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	return ParseServerID(r, h.cfg)
}

func (h *SqlServerLockHandlers) GetBlockingKpis(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	res, err := h.metricsSvc.GetSQLServerBlockingKPIs(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerLockHandlers) GetBlockingTimeline(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSQLServerBlockingTimeline(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerLockHandlers) GetBlockingDetails(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSQLServerBlockingDetails(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerLockHandlers) GetBlockingLocks(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSQLServerBlockingLocks(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerLockHandlers) GetTopBlockingQueries(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSQLServerTopBlockingQueries(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerLockHandlers) GetMostBlockedDatabases(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSQLServerMostBlockedDatabases(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerLockHandlers) GetMostBlockedObjects(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSQLServerMostBlockedObjects(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerLockHandlers) GetBlockingRecurrence(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	sqlHash := r.URL.Query().Get("sql_hash")
	login := r.URL.Query().Get("login_name")

	res, err := h.metricsSvc.GetSQLServerBlockingRecurrence(r.Context(), id, sqlHash, login)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
