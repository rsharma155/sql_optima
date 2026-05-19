// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: OS-level metrics API handlers for ingestion from external agents.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/service"
)

type OSMetricsHandlers struct {
	metricsSvc *service.MetricsService
}

func NewOSMetricsHandlers(svc *service.MetricsService) *OSMetricsHandlers {
	return &OSMetricsHandlers{metricsSvc: svc}
}

func (h *OSMetricsHandlers) SaveMetrics(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ServerID uuid.UUID              `json:"server_id"`
		Metrics  map[string]interface{} `json:"metrics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.metricsSvc.SaveOSMetrics(r.Context(), p.ServerID, uuid.Nil, p.Metrics); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *OSMetricsHandlers) ReceiveMetrics(w http.ResponseWriter, r *http.Request) { h.SaveMetrics(w, r) }
