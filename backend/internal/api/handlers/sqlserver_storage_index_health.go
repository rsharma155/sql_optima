// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server-specific Storage & Index Health HTTP handlers (Timescale reads).
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

type SqlServerStorageIndexHealthHandlers struct {
	metricsSvc *service.MetricsService
}

func NewSqlServerStorageIndexHealthHandlers(metricsSvc *service.MetricsService) *SqlServerStorageIndexHealthHandlers {
	return &SqlServerStorageIndexHealthHandlers{metricsSvc: metricsSvc}
}

func (h *SqlServerStorageIndexHealthHandlers) IndexUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		end := time.Now().UTC()
		start := end.Add(-24 * time.Hour)
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	points, err := h.metricsSvc.TimescaleStorageIndexHealthIndexUsage(r.Context(), "sqlserver", instance, from, to, 2000)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"engine":   "sqlserver",
		"instance": instance,
		"from":     from,
		"to":       to,
		"points":   points,
	})
}

func (h *SqlServerStorageIndexHealthHandlers) TableUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		end := time.Now().UTC()
		start := end.Add(-24 * time.Hour)
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	points, err := h.metricsSvc.TimescaleStorageIndexHealthTableUsage(r.Context(), "sqlserver", instance, from, to, 2000)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"engine":   "sqlserver",
		"instance": instance,
		"from":     from,
		"to":       to,
		"points":   points,
	})
}

func (h *SqlServerStorageIndexHealthHandlers) Growth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		end := time.Now().UTC()
		start := end.Add(-7 * 24 * time.Hour)
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	points, err := h.metricsSvc.TimescaleStorageIndexHealthGrowth(r.Context(), "sqlserver", instance, from, to, 2000)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"engine":   "sqlserver",
		"instance": instance,
		"from":     from,
		"to":       to,
		"points":   points,
	})
}

func (h *SqlServerStorageIndexHealthHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

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
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
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

	payload, err := h.metricsSvc.TimescaleStorageIndexHealthDashboard(r.Context(), "sqlserver", instance, from, to, dbNames, schemaNames, tableLike)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(payload)
}

func (h *SqlServerStorageIndexHealthHandlers) Filters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	timeRange := strings.TrimSpace(r.URL.Query().Get("time_range"))
	dbName := strings.TrimSpace(r.URL.Query().Get("db"))
	schemaName := strings.TrimSpace(r.URL.Query().Get("schema"))

	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
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

	opts, err := h.metricsSvc.TimescaleStorageIndexHealthFilterOptions(r.Context(), "sqlserver", instance, from, to, dbName, schemaName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(opts)
}

func (h *SqlServerStorageIndexHealthHandlers) IndexDefinition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	dbName := strings.TrimSpace(r.URL.Query().Get("db"))
	schemaName := strings.TrimSpace(r.URL.Query().Get("schema"))
	indexName := strings.TrimSpace(r.URL.Query().Get("index_name"))

	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	rows, err := h.metricsSvc.TimescaleStorageIndexDefinition(r.Context(), "sqlserver", instance, dbName, schemaName, indexName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"definitions": rows})
}
