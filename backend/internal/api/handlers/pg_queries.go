// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL query analysis (pg_stat_statements/pg_stat_monitor) API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"time"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PgQueryHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgQueryHandlers(svc *service.MetricsService) *PgQueryHandlers {
	return &PgQueryHandlers{metricsSvc: svc}
}

func (h *PgQueryHandlers) GetTopQueries(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	from, _ := time.Parse(time.RFC3339, fromStr)
	to, _ := time.Parse(time.RFC3339, toStr)
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-1 * time.Hour)
	}

	res, _, err := h.metricsSvc.GetPostgresQueriesForAPI(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
