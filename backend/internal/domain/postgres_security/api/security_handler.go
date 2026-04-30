// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Security API handlers.
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

type PostgresSecurityHandler struct {
	metricsSvc *service.MetricsService
}

func NewPostgresSecurityHandler(svc *service.MetricsService) *PostgresSecurityHandler {
	return &PostgresSecurityHandler{metricsSvc: svc}
}

func (h *PostgresSecurityHandler) GetDashboardData(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance parameter is required", http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		from = time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().Format(time.RFC3339)
	}

	ctx := r.Context()
	repo := h.metricsSvc.PgSecurityRepo
	if repo == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "security repository not initialized (TimescaleDB disconnected)",
		})
		return
	}

	data := make(map[string]interface{})

	kpis, _ := repo.GetKPIData(ctx, instance, from, to)
	data["kpis"] = kpis

	loginTrend, _ := repo.GetFailedLoginTrend(ctx, instance, from, to)
	data["login_trend"] = loginTrend

	superusers, _ := repo.GetSuperusers(ctx, instance)
	data["superusers"] = superusers

	elevatedRoles, _ := repo.GetElevatedRoles(ctx, instance)
	data["elevated_roles"] = elevatedRoles

	dmlTrend, _ := repo.GetDMLActivityTrend(ctx, instance, from, to)
	data["dml_trend"] = dmlTrend

	origins, _ := repo.GetConnectionOrigins(ctx, instance, from, to)
	data["connection_origins"] = origins

	roleTrend, _ := repo.GetRoleModificationsTrend(ctx, instance, from, to)
	data["role_trend"] = roleTrend

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
