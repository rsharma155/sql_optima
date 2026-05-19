// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL metrics and alerts API handlers.
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

type PgAlertHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgAlertHandlers(svc *service.MetricsService) *PgAlertHandlers {
	return &PgAlertHandlers{metricsSvc: svc}
}

func (h *PgAlertHandlers) GetPgAlerts(w http.ResponseWriter, r *http.Request) {
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

	kpis, err := h.metricsSvc.GetPgLocksBlockingKpis(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(kpis)
}
