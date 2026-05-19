// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL locks, deadlocks, and blocking API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"strconv"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PgLockHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgLockHandlers(svc *service.MetricsService) *PgLockHandlers {
	return &PgLockHandlers{metricsSvc: svc}
}

func (h *PgLockHandlers) GetLocks(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := h.metricsSvc.GetLatestPostgresLocks(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgLockHandlers) GetDeadlocks(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	minutes := 60
	if m := r.URL.Query().Get("minutes"); m != "" {
		if val, err := strconv.Atoi(m); err == nil {
			minutes = val
		}
	}

	res, err := h.metricsSvc.GetPostgresDeadlocksHistory(r.Context(), id, minutes, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgLockHandlers) GetBlockingKpis(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := h.metricsSvc.GetPgLocksBlockingKpis(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
