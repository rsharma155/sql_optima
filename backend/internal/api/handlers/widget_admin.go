// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Widget administration for dashboard customization, widget CRUD operations, and restoration.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/service"
)

type WidgetAdminHandlers struct {
	metricsSvc *service.MetricsService
}

func NewWidgetAdminHandlers(metricsSvc *service.MetricsService) *WidgetAdminHandlers {
	return &WidgetAdminHandlers{metricsSvc: metricsSvc}
}

func (h *WidgetAdminHandlers) UpdateWidget(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.WidgetRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Widget registry not configured"})
		return
	}

	widgetID := mux.Vars(r)["id"]
	var req models.WidgetUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid request body", err, "handler", "UpdateWidget")
		return
	}
	if req.CurrentSQL == "" {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "current_sql is required", nil, "handler", "UpdateWidget")
		return
	}

	if err := h.metricsSvc.WidgetRepo.UpdateWidgetSQL(r.Context(), widgetID, req.CurrentSQL); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "failed to update widget SQL", err, "handler", "UpdateWidget", "widget_id", widgetID)
		return
	}

	middleware.AuditAction(slog.Default(), r, "admin_update_widget_sql",
		slog.String("widget_id", widgetID),
	)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"widget_id": widgetID,
		"message":   "Widget SQL updated successfully",
	})
}

func (h *WidgetAdminHandlers) RestoreWidget(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.WidgetRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Widget registry not configured"})
		return
	}

	widgetID := mux.Vars(r)["id"]
	if err := h.metricsSvc.WidgetRepo.RestoreWidgetDefault(r.Context(), widgetID); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "failed to restore widget", err, "handler", "RestoreWidget", "widget_id", widgetID)
		return
	}

	middleware.AuditAction(slog.Default(), r, "admin_restore_widget_sql",
		slog.String("widget_id", widgetID),
	)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"widget_id": widgetID,
		"message":   "Widget SQL restored to default",
	})
}

func (h *WidgetAdminHandlers) GetWidget(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.WidgetRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Widget registry not configured"})
		return
	}

	widgetID := mux.Vars(r)["id"]
	widget, err := h.metricsSvc.WidgetRepo.GetWidgetByID(r.Context(), widgetID)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusNotFound, "widget not found", err, "handler", "GetWidget", "widget_id", widgetID)
		return
	}

	json.NewEncoder(w).Encode(widget)
}

func (h *WidgetAdminHandlers) ListWidgets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.WidgetRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Widget registry not configured"})
		return
	}

	query := `SELECT widget_id, dashboard_section, title, chart_type, current_sql, default_sql, updated_at FROM optima_ui_widgets ORDER BY dashboard_section, widget_id`
	rows, err := h.metricsSvc.WidgetRepo.Pool().Query(r.Context(), query)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to list widgets", err, "handler", "ListWidgets")
		return
	}
	defer rows.Close()

	type WidgetFull struct {
		WidgetID         string `json:"widget_id"`
		DashboardSection string `json:"dashboard_section"`
		Title            string `json:"title"`
		ChartType        string `json:"chart_type"`
		CurrentSQL       string `json:"current_sql"`
		DefaultSQL       string `json:"default_sql"`
		UpdatedAt        string `json:"updated_at"`
	}
	var widgets []WidgetFull
	for rows.Next() {
		var w WidgetFull
		if err := rows.Scan(&w.WidgetID, &w.DashboardSection, &w.Title, &w.ChartType, &w.CurrentSQL, &w.DefaultSQL, &w.UpdatedAt); err != nil {
			continue
		}
		widgets = append(widgets, w)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"widgets": widgets})
}
