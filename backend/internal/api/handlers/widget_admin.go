package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
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
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if req.CurrentSQL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "current_sql is required"})
		return
	}

	if err := h.metricsSvc.WidgetRepo.UpdateWidgetSQL(r.Context(), widgetID, req.CurrentSQL); err != nil {
		log.Printf("[API] Widget update error for %s: %v", widgetID, err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

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
		log.Printf("[API] Widget restore error for %s: %v", widgetID, err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

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
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
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
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
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
