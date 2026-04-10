package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/config"
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

	var req struct {
		SQL     string `json:"sql"`
		Timeout int    `json:"timeout"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.SQL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "sql is required"})
		return
	}

	if req.Timeout <= 0 {
		req.Timeout = 30
	}

	results, err := h.metricsSvc.ExecuteQuery(instance, req.SQL, req.Timeout)
	if err != nil {
		log.Printf("[API] Query execution error for %s: %v", instance, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   err.Error(),
			"success": false,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"results": results,
		"count":   len(results),
	})
}
