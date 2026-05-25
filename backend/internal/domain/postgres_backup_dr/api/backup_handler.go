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

	"github.com/rsharma155/sql_optima/internal/api/handlers"
	drdomain "github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PostgresBackupHandler struct {
	metricsSvc *service.MetricsService
}

func NewPostgresBackupHandler(svc *service.MetricsService) *PostgresBackupHandler {
	return &PostgresBackupHandler{metricsSvc: svc}
}

func (h *PostgresBackupHandler) GetDashboardData(w http.ResponseWriter, r *http.Request) {
	serverID, ok := handlers.ParseServerID(r, h.metricsSvc.Config)
	if !ok {
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "backup repository not initialized (TimescaleDB disconnected)",
		})
		return
	}

	data := make(map[string]interface{})

	kpis, _ := repo.GetKPIData(ctx, serverID, from, to)
	data["kpis"] = kpis

	walTrend, _ := repo.GetWALTrend(ctx, serverID, from, to)
	data["wal_trend"] = walTrend

	lagTrend, _ := repo.GetReplicationLagTrend(ctx, serverID, from, to)
	data["replication_lag_trend"] = lagTrend

	archiveHealth, _ := repo.GetArchiveHealth(ctx, serverID, from, to)
	data["archive_health"] = archiveHealth

	checkpointTrend, _ := repo.GetCheckpointTrend(ctx, serverID, from, to)
	data["checkpoint_trend"] = checkpointTrend

	replicationDetails, _ := repo.GetReplicationDetails(ctx, serverID)
	data["replication_details"] = replicationDetails

	archiverFailures, _ := repo.GetArchiverFailures(ctx, serverID)
	data["archiver_failures"] = archiverFailures

	slots, _ := repo.GetReplicationSlots(ctx, serverID)
	data["replication_slots"] = slots

	var latestBackup *hot.PostgresBackupRunRow
	if tl := h.metricsSvc.GetTimescaleDBLogger(); tl != nil {
		if row, err := tl.GetLatestPostgresBackupRun(ctx, serverID); err == nil && row != nil {
			latestBackup = row
			data["backup_latest"] = row
		}
		if hist, err := tl.GetPostgresBackupRunHistory(ctx, serverID, 10); err == nil {
			data["backup_history"] = hist
		}
	}

	policy := h.metricsSvc.GetDRPolicy(ctx, serverID)
	data["dr_policy"] = policy
	readiness := drdomain.EvaluateReadiness(policy, kpis, slots, latestBackup)
	data["readiness"] = readiness

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
