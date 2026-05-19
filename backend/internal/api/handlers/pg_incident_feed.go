// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL incident feed API handlers.
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

type PgIncidentHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgIncidentHandlers(svc *service.MetricsService) *PgIncidentHandlers {
	return &PgIncidentHandlers{metricsSvc: svc}
}

func (h *PgIncidentHandlers) GetFeed(w http.ResponseWriter, r *http.Request) {
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

	severity := 0
	if s := r.URL.Query().Get("severity"); s != "" {
		if val, err := strconv.Atoi(s); err == nil {
			severity = val
		}
	}

	res, err := h.metricsSvc.GetPgIncidentFeed(r.Context(), id, severity, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
