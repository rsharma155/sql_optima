// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server query analysis API handlers (regressions, plan instability).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SqlServerQueryAnalysisHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerQueryAnalysisHandlers(svc *service.MetricsService, cfg *config.Config) *SqlServerQueryAnalysisHandlers {
	return &SqlServerQueryAnalysisHandlers{metricsSvc: svc, cfg: cfg}
}

func (h *SqlServerQueryAnalysisHandlers) resolve(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, _, outcome := resolveInstanceParam(r, h.cfg)
	switch outcome {
	case lookupMissing:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return uuid.Nil, false
	case lookupNotFound:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return uuid.Nil, false
	}
	return id, true
}

func (h *SqlServerQueryAnalysisHandlers) requireSqlServer(w http.ResponseWriter, serverID uuid.UUID) bool {
	if h.cfg == nil {
		return true
	}
	for _, inst := range h.cfg.Instances {
		if inst.ServerID == serverID && strings.ToLower(inst.Type) != "sqlserver" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance is not SQL Server"})
			return false
		}
	}
	return true
}

func (h *SqlServerQueryAnalysisHandlers) GetSummary(w http.ResponseWriter, r *http.Request) {
	id, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if !h.requireSqlServer(w, id) {
		return
	}

	hours := 24
	if hStr := r.URL.Query().Get("hours"); hStr != "" {
		if val, err := strconv.Atoi(hStr); err == nil {
			hours = val
		}
	}

	res, err := h.metricsSvc.GetSqlServerQueryAnalysisSummary(r.Context(), id, hours, true)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerQueryAnalysisHandlers) GetRegressions(w http.ResponseWriter, r *http.Request) {
	id, ok := h.resolve(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	res, err := h.metricsSvc.GetSqlServerQueryRegressions(r.Context(), id, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerQueryAnalysisHandlers) GetPlanInstability(w http.ResponseWriter, r *http.Request) {
	id, ok := h.resolve(w, r)
	if !ok {
		return
	}

	res, err := h.metricsSvc.GetSqlServerPlanInstability(r.Context(), id, 50)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerQueryAnalysisHandlers) GetTopQueries(w http.ResponseWriter, r *http.Request) {
	id, ok := h.resolve(w, r)
	if !ok {
		return
	}

	sortBy := r.URL.Query().Get("sort")
	hours := 24
	if hStr := r.URL.Query().Get("hours"); hStr != "" {
		if val, err := strconv.Atoi(hStr); err == nil {
			hours = val
		}
	}

	res, err := h.metricsSvc.GetSqlServerTopQueriesAnalysis(r.Context(), id, sortBy, 50, hours, true)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerQueryAnalysisHandlers) GetQueryPlans(w http.ResponseWriter, r *http.Request) {
	id, ok := h.resolve(w, r)
	if !ok {
		return
	}
	dbName := r.URL.Query().Get("database")
	if dbName == "" {
		dbName = "master"
	}
	sqlHash := r.URL.Query().Get("sql_hash")
	if sqlHash == "" {
		sqlHash = r.URL.Query().Get("query_hash")
	}
	if sqlHash == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "sql_hash or query_hash required"})
		return
	}

	res, err := h.metricsSvc.GetSqlServerQueryPlans(r.Context(), id, dbName, sqlHash, time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *SqlServerQueryAnalysisHandlers) GetQueryWaitStats(w http.ResponseWriter, r *http.Request) {
	id, ok := h.resolve(w, r)
	if !ok {
		return
	}
	dbName := r.URL.Query().Get("database")
	if dbName == "" {
		dbName = "master"
	}
	sqlHash := r.URL.Query().Get("sql_hash")
	if sqlHash == "" {
		sqlHash = r.URL.Query().Get("query_hash")
	}
	if sqlHash == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "sql_hash or query_hash required"})
		return
	}

	res, err := h.metricsSvc.GetSqlServerQueryWaitStats(r.Context(), id, dbName, sqlHash, time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
