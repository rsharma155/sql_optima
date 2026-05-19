// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server workload and throughput API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"strconv"
	"time"
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SqlServerWorkloadHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerWorkloadHandlers(svc *service.MetricsService, cfg *config.Config) *SqlServerWorkloadHandlers {
	return &SqlServerWorkloadHandlers{metricsSvc: svc, cfg: cfg}
}

func (h *SqlServerWorkloadHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	return ParseServerID(r, h.cfg)
}

func (h *SqlServerWorkloadHandlers) GetSummary(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to, _ := parseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSqlServerWorkloadSummary(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerWorkloadHandlers) GetTrends(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to, _ := parseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSqlServerWorkloadTrends(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"trends": res})
}

func (h *SqlServerWorkloadHandlers) GetTopOffenders(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to, _ := parseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSqlServerWorkloadTopOffenders(r.Context(), id, from, to, 20)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"top_offenders": res})
}

func (h *SqlServerWorkloadHandlers) GetAppLoadTimeline(w http.ResponseWriter, r *http.Request) {
	h.handleTimeline(w, r, "app_load", h.metricsSvc.GetSqlServerWorkloadAppLoadTimeline)
}

func (h *SqlServerWorkloadHandlers) GetLoginLoadTimeline(w http.ResponseWriter, r *http.Request) {
	h.handleTimeline(w, r, "login_load", h.metricsSvc.GetSqlServerWorkloadLoginLoadTimeline)
}

func (h *SqlServerWorkloadHandlers) GetTopApps(w http.ResponseWriter, r *http.Request) {
	h.handleTopN(w, r, "top_apps", h.metricsSvc.GetSqlServerWorkloadTopApps)
}

func (h *SqlServerWorkloadHandlers) GetTopLogins(w http.ResponseWriter, r *http.Request) {
	h.handleTopN(w, r, "top_logins", h.metricsSvc.GetSqlServerWorkloadTopLogins)
}

func (h *SqlServerWorkloadHandlers) handleTimeline(w http.ResponseWriter, r *http.Request, wrapKey string, fn func(context.Context, uuid.UUID, time.Time, time.Time) (interface{}, error)) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to, _ := parseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := fn(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{wrapKey: res})
}

func (h *SqlServerWorkloadHandlers) handleTopN(w http.ResponseWriter, r *http.Request, wrapKey string, fn func(context.Context, uuid.UUID, time.Time, time.Time, int) (interface{}, error)) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to, _ := parseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}

	res, err := fn(r.Context(), id, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{wrapKey: res})
}
