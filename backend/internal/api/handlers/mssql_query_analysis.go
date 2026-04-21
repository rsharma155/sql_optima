// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for the SQL Server Query Analysis Dashboard — summary,
//
//	regressions, plan instability, top queries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

// MssqlQueryAnalysisHandlers groups HTTP handlers for the Query Analysis dashboard.
type MssqlQueryAnalysisHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

// NewMssqlQueryAnalysisHandlers constructs a new handler set.
func NewMssqlQueryAnalysisHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *MssqlQueryAnalysisHandlers {
	return &MssqlQueryAnalysisHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

// Summary returns KPI card data for the query analysis dashboard.
func (h *MssqlQueryAnalysisHandlers) Summary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	hours := 24
	if h, err2 := strconv.Atoi(r.URL.Query().Get("hours")); err2 == nil && h > 0 && h <= 168 {
		hours = h
	}
	summary, err := h.metricsSvc.GetMssqlQueryAnalysisSummary(r.Context(), instance, hours)
	if err != nil {
		slog.Error("mssql_query_analysis_summary", "instance", instance, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch summary"})
		return
	}
	json.NewEncoder(w).Encode(summary)
}

// Regressions returns recent query regressions from TimescaleDB.
func (h *MssqlQueryAnalysisHandlers) Regressions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}

	rows, err := h.metricsSvc.GetMssqlQueryRegressions(r.Context(), instance, limit)
	if err != nil {
		slog.Error("mssql_query_regressions", "instance", instance, "error", err)
		rows = nil
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":    instance,
		"regressions": rows,
	})
}

// PlanInstability returns queries with multiple execution plans.
func (h *MssqlQueryAnalysisHandlers) PlanInstability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}

	rows, err := h.metricsSvc.GetMssqlPlanInstability(r.Context(), instance, limit)
	if err != nil {
		slog.Error("mssql_plan_instability", "instance", instance, "error", err)
		rows = nil
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":         instance,
		"plan_instability": rows,
	})
}

// TopQueries returns top queries from the existing interval table.
func (h *MssqlQueryAnalysisHandlers) TopQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	sortBy := r.URL.Query().Get("sort")
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	hours := 24
	if h, err2 := strconv.Atoi(r.URL.Query().Get("hours")); err2 == nil && h > 0 && h <= 168 {
		hours = h
	}

	rows, err := h.metricsSvc.GetMssqlTopQueriesAnalysis(r.Context(), instance, sortBy, limit, hours)
	if err != nil {
		slog.Error("mssql_top_queries", "instance", instance, "error", err)
		rows = nil
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"queries":  rows,
	})
}

// QueryPlans returns plan metadata from Query Store for a specific query hash.
func (h *MssqlQueryAnalysisHandlers) QueryPlans(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	dbName := r.URL.Query().Get("database")
	queryHash := r.URL.Query().Get("query_hash")
	if dbName == "" || queryHash == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "database and query_hash required"})
		return
	}

	plans, err := h.metricsSvc.GetMssqlQueryPlans(r.Context(), instance, dbName, queryHash)
	if err != nil {
		slog.Error("mssql_query_plans", "instance", instance, "error", err)
		plans = nil
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"plans":    plans,
	})
}

// QueryWaitStats returns wait statistics from Query Store for a specific query hash.
func (h *MssqlQueryAnalysisHandlers) QueryWaitStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	dbName := r.URL.Query().Get("database")
	queryHash := r.URL.Query().Get("query_hash")
	if dbName == "" || queryHash == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "database and query_hash required"})
		return
	}

	waits, err := h.metricsSvc.GetMssqlQueryWaitStats(r.Context(), instance, dbName, queryHash)
	if err != nil {
		slog.Error("mssql_query_wait_stats", "instance", instance, "error", err)
		waits = nil
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":   instance,
		"wait_stats": waits,
	})
}
