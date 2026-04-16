// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Cross-engine Storage & Index Health HTTP handlers (Timescale reads). Use query parameter engine=sqlserver|postgres; PostgreSQL-specific collectors live under internal/collectors/pg_*.go.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/service"
)

// StorageIndexHealthTimescaleHandlers provides Timescale-backed reads for the cross-engine Storage & Index Health dashboard.
type StorageIndexHealthTimescaleHandlers struct {
	metricsSvc *service.MetricsService
}

func NewStorageIndexHealthTimescaleHandlers(metricsSvc *service.MetricsService) *StorageIndexHealthTimescaleHandlers {
	return &StorageIndexHealthTimescaleHandlers{metricsSvc: metricsSvc}
}

func (h *StorageIndexHealthTimescaleHandlers) IndexUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		// default 24h window to mirror dashboard range options
		end := time.Now().UTC()
		start := end.Add(-24 * time.Hour)
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}
	if engine == "" || instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine and instance are required"})
		return
	}
	if engine != "sqlserver" && engine != "postgres" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine must be sqlserver or postgres"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	points, err := h.metricsSvc.TimescaleStorageIndexHealthIndexUsage(r.Context(), engine, instance, from, to, 2000)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"engine":   engine,
		"instance": instance,
		"from":     from,
		"to":       to,
		"points":   points,
	})
}

func (h *StorageIndexHealthTimescaleHandlers) TableUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		end := time.Now().UTC()
		start := end.Add(-24 * time.Hour)
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}
	if engine == "" || instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine and instance are required"})
		return
	}
	if engine != "sqlserver" && engine != "postgres" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine must be sqlserver or postgres"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	points, err := h.metricsSvc.TimescaleStorageIndexHealthTableUsage(r.Context(), engine, instance, from, to, 2000)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"engine":   engine,
		"instance": instance,
		"from":     from,
		"to":       to,
		"points":   points,
	})
}

func (h *StorageIndexHealthTimescaleHandlers) Growth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		end := time.Now().UTC()
		start := end.Add(-7 * 24 * time.Hour)
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}
	if engine == "" || instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine and instance are required"})
		return
	}
	if engine != "sqlserver" && engine != "postgres" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine must be sqlserver or postgres"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	points, err := h.metricsSvc.TimescaleStorageIndexHealthGrowth(r.Context(), engine, instance, from, to, 2000)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"engine":   engine,
		"instance": instance,
		"from":     from,
		"to":       to,
		"points":   points,
	})
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Dashboard returns a pre-aggregated payload for the Storage & Index Health dashboard.
// Filters:
// - db: comma-separated (multi)
// - schema: comma-separated (multi)
// - table: substring match
func (h *StorageIndexHealthTimescaleHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	timeRange := strings.TrimSpace(r.URL.Query().Get("time_range"))
	if from == "" || to == "" {
		end := time.Now().UTC()
		start := end.Add(-24 * time.Hour)
		switch timeRange {
		case "1h":
			start = end.Add(-1 * time.Hour)
		case "24h", "":
			start = end.Add(-24 * time.Hour)
		case "7d":
			start = end.Add(-7 * 24 * time.Hour)
		case "30d":
			start = end.Add(-30 * 24 * time.Hour)
		}
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}
	if engine == "" || instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine and instance are required"})
		return
	}
	if engine != "sqlserver" && engine != "postgres" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine must be sqlserver or postgres"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	dbNames := splitCSV(r.URL.Query().Get("db"))
	schemaNames := splitCSV(r.URL.Query().Get("schema"))
	tableLike := strings.TrimSpace(r.URL.Query().Get("table"))

	payload, err := h.metricsSvc.TimescaleStorageIndexHealthDashboard(r.Context(), engine, instance, from, to, dbNames, schemaNames, tableLike)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(payload)
}

// Filters returns distinct database/schema/table options for the selected instance (Timescale-backed).
// Query:
// - engine, instance required
// - time_range optional (1h/24h/7d/30d)
// - db optional (single)
// - schema optional (single)
func (h *StorageIndexHealthTimescaleHandlers) Filters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	timeRange := strings.TrimSpace(r.URL.Query().Get("time_range"))
	dbName := strings.TrimSpace(r.URL.Query().Get("db"))
	schemaName := strings.TrimSpace(r.URL.Query().Get("schema"))

	if engine == "" || instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "engine and instance are required"})
		return
	}
	if engine != "sqlserver" && engine != "postgres" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "engine must be sqlserver or postgres"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	switch timeRange {
	case "1h":
		start = end.Add(-1 * time.Hour)
	case "24h", "":
		start = end.Add(-24 * time.Hour)
	case "7d":
		start = end.Add(-7 * 24 * time.Hour)
	case "30d":
		start = end.Add(-30 * 24 * time.Hour)
	}
	from := start.Format(time.RFC3339)
	to := end.Format(time.RFC3339)

	opts, err := h.metricsSvc.TimescaleStorageIndexHealthFilterOptions(r.Context(), engine, instance, from, to, dbName, schemaName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(opts)
}

// IndexDefinition returns the latest stored index definition(s) from monitor.index_definitions.
// Query params: engine, instance, db (optional), schema (optional), index_name (optional).
func (h *StorageIndexHealthTimescaleHandlers) IndexDefinition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	dbName := strings.TrimSpace(r.URL.Query().Get("db"))
	schemaName := strings.TrimSpace(r.URL.Query().Get("schema"))
	indexName := strings.TrimSpace(r.URL.Query().Get("index_name"))

	if engine == "" || instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "engine and instance are required"})
		return
	}
	if engine != "sqlserver" && engine != "postgres" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "engine must be sqlserver or postgres"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	rows, err := h.metricsSvc.TimescaleStorageIndexDefinition(r.Context(), engine, instance, dbName, schemaName, indexName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"definitions": rows})
}
