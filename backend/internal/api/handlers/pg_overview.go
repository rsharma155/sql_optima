// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL instance overview API handlers.
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

type PgOverviewHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgOverviewHandlers(svc *service.MetricsService) *PgOverviewHandlers {
	return &PgOverviewHandlers{metricsSvc: svc}
}

func (h *PgOverviewHandlers) GetOverview(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res := h.metricsSvc.GetPostgresOverview(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgOverviewHandlers) GetDatabases(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := h.metricsSvc.GetTimescalePostgresDatabases(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
