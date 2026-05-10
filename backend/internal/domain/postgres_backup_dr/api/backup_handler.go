// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Backup & DR API handlers.
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

type PostgresBackupHandler struct {
	metricsSvc *service.MetricsService
}

func NewPostgresBackupHandler(svc *service.MetricsService) *PostgresBackupHandler {
	return &PostgresBackupHandler{metricsSvc: svc}
}

func (h *PostgresBackupHandler) GetDashboardData(w http.ResponseWriter, r *http.Request) {
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
	repo := h.metricsSvc.PgBackupRepo
	if repo == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "backup repository not initialized (TimescaleDB disconnected)",
		})
		return
	}

	data := make(map[string]interface{})

	// KPIs (Row 1)
	kpis, _ := repo.GetKPIData(ctx, instance, from, to)
	data["kpis"] = kpis

	// Trends (Row 2 & 3)
	walTrend, _ := repo.GetWALTrend(ctx, instance, from, to)
	data["wal_trend"] = walTrend

	replicationLagTrend, _ := repo.GetReplicationLagTrend(ctx, instance, from, to)
	data["replication_lag_trend"] = replicationLagTrend

	archiveHealth, _ := repo.GetArchiveHealth(ctx, instance, from, to)
	data["archive_health"] = archiveHealth

	checkpointTrend, _ := repo.GetCheckpointTrend(ctx, instance, from, to)
	data["checkpoint_trend"] = checkpointTrend

	// Operational Details (Row 4)
	replicationDetails, _ := repo.GetReplicationDetails(ctx, instance)
	data["replication_details"] = replicationDetails

	archiverFailures, _ := repo.GetArchiverFailures(ctx, instance)
	data["archiver_failures"] = archiverFailures

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
