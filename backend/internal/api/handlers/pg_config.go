// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL configuration, settings, and best practices API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PgConfigHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgConfigHandlers(svc *service.MetricsService) *PgConfigHandlers {
	return &PgConfigHandlers{metricsSvc: svc}
}

func (h *PgConfigHandlers) GetSettingsDiff(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	t1, t2, latest, prev, err := h.metricsSvc.GetPostgresSettingsSnapshotLatestTwo(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res := map[string]interface{}{
		"latest_ts": t1,
		"prev_ts":   t2,
		"latest":    latest,
		"prev":      prev,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgConfigHandlers) GetBestPractices(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := h.metricsSvc.FetchPgBestPracticesWithTimescale(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
