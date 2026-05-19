// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL checkpoint health and background writer API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"strconv"
	"time"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PgCheckpointHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgCheckpointHandlers(svc *service.MetricsService) *PgCheckpointHandlers {
	return &PgCheckpointHandlers{metricsSvc: svc}
}

func (h *PgCheckpointHandlers) GetCheckpointSummary(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}

	res, err := h.metricsSvc.GetPostgresCheckpointSummary(r.Context(), id, time.Now().Add(-24*time.Hour), time.Now(), limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(res)
}
