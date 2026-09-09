// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for SQL Server Health Intelligence Report (Autonomous Analysis).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

// IntelligenceReportHandlers provides API endpoints for the health intelligence report.
type IntelligenceReportHandlers struct {
	svc *service.IntelligenceReportService
	cfg *config.Config
}

// NewIntelligenceReportHandlers creates a new instance of the handlers.
func NewIntelligenceReportHandlers(svc *service.IntelligenceReportService, cfg *config.Config) *IntelligenceReportHandlers {
	return &IntelligenceReportHandlers{svc: svc, cfg: cfg}
}

// Analyze triggers health analysis for a given server.
func (h *IntelligenceReportHandlers) Analyze(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "intelligence report engine is not available (check TimescaleDB connection)"})
		return
	}
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server_id or instance name required"})
		return
	}

	result, err := h.svc.Analyze(r.Context(), serverID)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to run intelligence analysis", err, "handler", "intelligence")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// Status returns whether the intelligence engine service is active and reachable.
func (h *IntelligenceReportHandlers) Status(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"active": false})
		return
	}
	active := h.svc.CheckStatus(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"active": active})
}

// Latest returns metadata for the most recent persisted intelligence snapshot (24h window).
func (h *IntelligenceReportHandlers) Latest(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "intelligence report engine is not available"})
		return
	}
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server_id or instance name required"})
		return
	}

	meta, err := h.svc.GetLatestMeta(r.Context(), serverID)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load intelligence snapshot", err, "handler", "intelligence")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}

// GetReport fetches the generated report (HTML, JSON, PDF).
func (h *IntelligenceReportHandlers) GetReport(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "intelligence report engine is not available"})
		return
	}
	vars := mux.Vars(r)
	runID := vars["run_id"]
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "html"
	}
	instanceName := r.URL.Query().Get("instance_name")

	content, err := h.svc.GetReport(r.Context(), runID, format, instanceName, r.URL.Query())
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load intelligence report", err, "handler", "intelligence")
		return
	}

	switch format {
	case "pdf":
		w.Header().Set("Content-Type", "application/pdf")
	case "json":
		w.Header().Set("Content-Type", "application/json")
	default:
		w.Header().Set("Content-Type", "text/html")
	}

	_, _ = w.Write(content)
}
