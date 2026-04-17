// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Dashboard widget management and custom query execution endpoints for dynamic dashboard configurations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/security/redact"
	"github.com/rsharma155/sql_optima/internal/service"
)

type DashboardHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewDashboardHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *DashboardHandlers {
	return &DashboardHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *DashboardHandlers) Widgets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	widgets, err := h.metricsSvc.GetDashboardWidgets(instance)
	if err != nil {
		log.Printf("[API] Widgets fetch error for %s: %v", instance, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch widgets"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"widgets": widgets})
}

func (h *DashboardHandlers) ExecuteQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		WidgetID    string            `json:"widget_id"`
		Parameters  map[string]string `json:"parameters"`
		TimeoutSecs int               `json:"timeout_secs"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.WidgetID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "widget_id is required"})
		return
	}

	// Default params and enforce server-side timeout.
	if req.Parameters == nil {
		req.Parameters = map[string]string{}
	}
	if req.TimeoutSecs <= 0 {
		req.TimeoutSecs = 30
	}
	if req.TimeoutSecs > 60 {
		req.TimeoutSecs = 60
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(req.TimeoutSecs)*time.Second)
	defer cancel()

	rows, err := h.metricsSvc.ExecuteWidgetQuery(ctx, req.WidgetID, req.Parameters)
	if err != nil {
		log.Printf("[API] Widget query execution error for widget=%s: %s", req.WidgetID, redact.String(err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    false,
			"error":      "widget query failed",
			"request_id": middleware.RequestIDFromContext(r.Context()),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"rows":    rows,
		"count":   len(rows),
	})
}
