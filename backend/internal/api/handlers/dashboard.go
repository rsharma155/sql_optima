// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Dashboard-related API handlers for widget registry and custom queries.
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

type DashboardHandlers struct {
	metricsSvc *service.MetricsService
}

func NewDashboardHandlers(svc *service.MetricsService) *DashboardHandlers {
	return &DashboardHandlers{metricsSvc: svc}
}

func (h *DashboardHandlers) GetDashboardWidgets(w http.ResponseWriter, r *http.Request) {
	serverIDStr := r.URL.Query().Get("server_id")
	if serverIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(serverIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	widgets, err := h.metricsSvc.GetDashboardWidgets(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(widgets)
}

func (h *DashboardHandlers) ExecuteWidgetQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServerID   uuid.UUID         `json:"server_id"`
		WidgetID   string            `json:"widget_id"`
		Parameters map[string]string `json:"parameters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id := req.ServerID

	data, err := h.metricsSvc.ExecuteWidgetQuery(r.Context(), id, req.WidgetID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
