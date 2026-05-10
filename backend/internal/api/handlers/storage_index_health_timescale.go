// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unified Storage & Index Health HTTP handlers (Timescale reads).
// Supports both PostgreSQL and SQL Server via 'engine' query parameter.
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

type StorageIndexHealthTimescaleHandlers struct {
	metricsSvc *service.MetricsService
}

func NewStorageIndexHealthTimescaleHandlers(metricsSvc *service.MetricsService) *StorageIndexHealthTimescaleHandlers {
	return &StorageIndexHealthTimescaleHandlers{metricsSvc: metricsSvc}
}

func (h *StorageIndexHealthTimescaleHandlers) getParams(r *http.Request) (engine, instance, from, to string) {
	engine = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("engine")))
	instance = strings.TrimSpace(r.URL.Query().Get("instance"))
	from = strings.TrimSpace(r.URL.Query().Get("from"))
	to = strings.TrimSpace(r.URL.Query().Get("to"))

	if from == "" || to == "" {
		end := time.Now().UTC()
		start := end.Add(-24 * time.Hour)
		from = start.Format(time.RFC3339)
		to = end.Format(time.RFC3339)
	}
	return
}

func (h *StorageIndexHealthTimescaleHandlers) IndexUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine, instance, from, to := h.getParams(r)
	if engine == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine is required"})
		return
	}
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
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

	engine, instance, from, to := h.getParams(r)
	if engine == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine is required"})
		return
	}
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
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

	engine, instance, from, to := h.getParams(r)
	if engine == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine is required"})
		return
	}
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
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

func (h *StorageIndexHealthTimescaleHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine, instance, from, to := h.getParams(r)
	if engine == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "engine is required"})
		return
	}
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
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

func (h *StorageIndexHealthTimescaleHandlers) Filters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("engine")))
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	dbName := strings.TrimSpace(r.URL.Query().Get("db"))
	schemaName := strings.TrimSpace(r.URL.Query().Get("schema"))

	if engine == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "engine is required"})
		return
	}
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
		return
	}

	filters, err := h.metricsSvc.TimescaleStorageIndexHealthFilterOptions(r.Context(), engine, instance, "", "", dbName, schemaName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(filters)
}

func (h *StorageIndexHealthTimescaleHandlers) IndexDefinition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "timescale not configured"})
		return
	}

	engine := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("engine")))
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	db := strings.TrimSpace(r.URL.Query().Get("db"))
	schema := strings.TrimSpace(r.URL.Query().Get("schema"))
	// table := strings.TrimSpace(r.URL.Query().Get("table")) // Not used in service method
	index := strings.TrimSpace(r.URL.Query().Get("index"))

	if engine == "" || instance == "" || db == "" || index == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "engine, instance, db, and index are required"})
		return
	}

	definitionRows, err := h.metricsSvc.TimescaleStorageIndexDefinition(r.Context(), engine, instance, db, schema, index)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"rows": definitionRows})
}
