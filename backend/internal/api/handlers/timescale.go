package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type TimescaleHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewTimescaleHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *TimescaleHandlers {
	return &TimescaleHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *TimescaleHandlers) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	connected := h.metricsSvc.IsTimescaleConnected()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": connected,
		"message":   map[string]string{"status": "TimescaleDB connection"},
	})
}

func (h *TimescaleHandlers) MssqlMetrics(w http.ResponseWriter, r *http.Request) {
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

	metrics, err := h.metricsSvc.GetTimescaleSQLServerMetrics(instance, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"metrics": metrics})
}

func (h *TimescaleHandlers) PostgresThroughput(w http.ResponseWriter, r *http.Request) {
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

	metrics, err := h.metricsSvc.GetTimescalePostgresThroughput(instance, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"metrics": metrics})
}

func (h *TimescaleHandlers) PostgresConnections(w http.ResponseWriter, r *http.Request) {
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

	stats, err := h.metricsSvc.GetTimescalePostgresConnections(instance, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"connections": stats})
}

func (h *TimescaleHandlers) MssqlTopQueries(w http.ResponseWriter, r *http.Request) {
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

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	queries, err := h.metricsSvc.GetTimescaleSQLServerTopQueries(instance, 100, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"top_queries": queries})
}

func (h *TimescaleHandlers) MssqlLongRunningQueries(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	log.Printf("[API] /api/timescale/mssql/long-running-queries called with instance=%s", instance)
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		log.Printf("[API] instance %s NOT in config", instance)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	database := strings.TrimSpace(r.URL.Query().Get("database"))
	log.Printf("[API] calling GetTimescaleSQLServerLongRunningQueries for instance=%s from=%s to=%s database=%q", instance, from, to, database)

	queries, err := h.metricsSvc.GetTimescaleSQLServerLongRunningQueries(instance, 100, from, to, database)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[API] returning %d long running queries for instance=%s", len(queries), instance)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"long_running_queries": queries})
}
