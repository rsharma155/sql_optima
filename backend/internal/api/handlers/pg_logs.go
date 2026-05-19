// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL log ingestion and retrieval API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PgLogHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgLogHandlers(svc *service.MetricsService) *PgLogHandlers {
	return &PgLogHandlers{metricsSvc: svc}
}

type pgLogReportRequest struct {
	ServerID uuid.UUID   `json:"server_id"`
	Events   interface{} `json:"events"`
}

func (h *PgLogHandlers) LogEvents(w http.ResponseWriter, r *http.Request) {
	var req pgLogReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id := req.ServerID

	if err := h.metricsSvc.LogPostgresLogEvents(r.Context(), id, req.Events); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *PgLogHandlers) GetLogSummary(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	window := 60
	if val := r.URL.Query().Get("window"); val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			window = v
		}
	}

	res, err := h.metricsSvc.GetPostgresLogSummary(r.Context(), id, window)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgLogHandlers) GetLogEvents(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	limit := 100
	if val := r.URL.Query().Get("limit"); val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			limit = v
		}
	}
	severity := r.URL.Query().Get("severity")

	res, err := h.metricsSvc.GetPostgresLogEvents(r.Context(), id, limit, severity)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
