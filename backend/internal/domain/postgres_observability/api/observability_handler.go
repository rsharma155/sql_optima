// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Observability API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"encoding/json"
	"net/http"
	"time"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PostgresObservabilityHandler struct {
	metricsSvc *service.MetricsService
}

func NewPostgresObservabilityHandler(svc *service.MetricsService) *PostgresObservabilityHandler {
	return &PostgresObservabilityHandler{metricsSvc: svc}
}

func (h *PostgresObservabilityHandler) GetDashboardData(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance parameter is required", http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		from = time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().Format(time.RFC3339)
	}

	ctx := r.Context()
	repo := h.metricsSvc.PgObservabilityRepo
	if repo == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "observability repository not initialized (TimescaleDB disconnected)",
		})
		return
	}

	data := make(map[string]interface{})

	kpis, _ := repo.GetKPIData(ctx, instance, from, to)
	data["kpis"] = kpis

	loadTrend, _ := repo.GetLoadTrend(ctx, instance, from, to)
	data["load_trend"] = loadTrend

	waitTrend, _ := repo.GetWaitCategoryTrend(ctx, instance, from, to)
	data["wait_trend"] = waitTrend

	waitDist, _ := repo.GetWaitCategoryDistribution(ctx, instance, from, to)
	data["wait_distribution"] = waitDist

	connByApp, _ := repo.GetConnectionsByApp(ctx, instance, from, to)
	data["connections_by_app"] = connByApp

	longSessions, _ := repo.GetLongRunningSessions(ctx, instance, from, to)
	data["long_running_sessions"] = longSessions

	topQueries, _ := repo.GetTopQueries(ctx, instance, from, to)
	data["top_queries"] = topQueries

	sessionStateTrend, _ := repo.GetSessionStateTrend(ctx, instance, from, to)
	data["session_state_trend"] = sessionStateTrend

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
