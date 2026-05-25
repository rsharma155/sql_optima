// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for GET and PUT /api/postgres/dr-policy (per-instance
//          RPO/RTO targets used by the Backup & DR dashboard and alert engine).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"encoding/json"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/api/handlers"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/entities"
	"github.com/rsharma155/sql_optima/internal/middleware"
)

func (h *PostgresBackupHandler) GetDRPolicy(w http.ResponseWriter, r *http.Request) {
	serverID, ok := handlers.ParseServerID(r, h.metricsSvc.Config)
	if !ok {
		return
	}
	repo := h.metricsSvc.PgDRPolicyRepo
	if repo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	p, err := repo.Get(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *PostgresBackupHandler) PutDRPolicy(w http.ResponseWriter, r *http.Request) {
	serverID, ok := handlers.ParseServerID(r, h.metricsSvc.Config)
	if !ok {
		return
	}
	repo := h.metricsSvc.PgDRPolicyRepo
	if repo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	var body entities.DRPolicy
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	body.ServerID = serverID
	if body.RPOBackupHours <= 0 {
		body.RPOBackupHours = 24
	}
	if body.RPOArchiveMinutes <= 0 {
		body.RPOArchiveMinutes = 5
	}
	if body.RPOReplaySeconds <= 0 {
		body.RPOReplaySeconds = 60
	}
	if body.MaxSlotRetentionGB <= 0 {
		body.MaxSlotRetentionGB = 10
	}
	actor := "system"
	if c := middleware.GetAuthClaims(r); c != nil && c.Username != "" {
		actor = c.Username
	}
	if err := repo.Upsert(r.Context(), body, actor); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	p, _ := repo.Get(r.Context(), serverID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}
