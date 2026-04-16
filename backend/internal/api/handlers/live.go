// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Real-Time Diagnostics (RTD) handlers for live KPI queries, running queries, blocking chains, I/O latency, and wait statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/security/redact"
	"github.com/rsharma155/sql_optima/internal/service"
)

type LiveHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewLiveHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *LiveHandlers {
	return &LiveHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *LiveHandlers) KPIs(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instance not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data := h.metricsSvc.MsRepo.FetchLiveKPIs(instance)
	if errMsg, ok := data["error"].(string); ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": errMsg})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data})
}

func (h *LiveHandlers) RunningQueries(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	dbFilter := strings.TrimSpace(r.URL.Query().Get("database"))
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instance not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data, err := h.metricsSvc.MsRepo.FetchLiveRunningQueries(instance, dbFilter)
	if err != nil {
		log.Printf("[Router] Live running queries failed: %s", redact.String(err.Error()))
		rid := middleware.RequestIDFromContext(r.Context())
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Query failed",
			"timeout": strings.Contains(err.Error(), "context deadline exceeded"),
			"request_id": rid,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data, "count": len(data)})
}

func (h *LiveHandlers) Blocking(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	dbFilter := strings.TrimSpace(r.URL.Query().Get("database"))
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instance not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data, err := h.metricsSvc.MsRepo.FetchLiveBlockingChains(instance, dbFilter)
	if err != nil {
		log.Printf("[Router] Live blocking chains failed: %s", redact.String(err.Error()))
		rid := middleware.RequestIDFromContext(r.Context())
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Query failed",
			"timeout": strings.Contains(err.Error(), "context deadline exceeded"),
			"request_id": rid,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data, "count": len(data)})
}

func (h *LiveHandlers) IOLatency(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instance not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data, err := h.metricsSvc.MsRepo.FetchLiveIOLatency(instance)
	if err != nil {
		log.Printf("[Router] Live IO latency failed: %s", redact.String(err.Error()))
		rid := middleware.RequestIDFromContext(r.Context())
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Query failed",
			"timeout": strings.Contains(err.Error(), "context deadline exceeded"),
			"request_id": rid,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data, "count": len(data)})
}

func (h *LiveHandlers) TempDB(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instance not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data, err := h.metricsSvc.MsRepo.FetchLiveTempDBUsage(instance)
	if err != nil {
		log.Printf("[Router] Live tempdb usage failed: %s", redact.String(err.Error()))
		rid := middleware.RequestIDFromContext(r.Context())
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Query failed",
			"timeout": strings.Contains(err.Error(), "context deadline exceeded"),
			"request_id": rid,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data})
}

func (h *LiveHandlers) Waits(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	dbFilter := strings.TrimSpace(r.URL.Query().Get("database"))
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instance not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data, err := h.metricsSvc.MsRepo.FetchLiveWaitStats(instance, dbFilter)
	if err != nil {
		log.Printf("[Router] Live wait stats failed: %s", redact.String(err.Error()))
		rid := middleware.RequestIDFromContext(r.Context())
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Query failed",
			"timeout": strings.Contains(err.Error(), "context deadline exceeded"),
			"request_id": rid,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data, "count": len(data)})
}

func (h *LiveHandlers) Connections(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	dbFilter := strings.TrimSpace(r.URL.Query().Get("database"))
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instance not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data, err := h.metricsSvc.MsRepo.FetchLiveConnectionsByApp(instance, dbFilter)
	if err != nil {
		log.Printf("[Router] Live connections by app failed: %s", redact.String(err.Error()))
		rid := middleware.RequestIDFromContext(r.Context())
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Query failed",
			"timeout": strings.Contains(err.Error(), "context deadline exceeded"),
			"request_id": rid,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data, "count": len(data)})
}
