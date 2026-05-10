package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sqlplan-analyzer/plugin"
)

type SqlServerPlanAnalyzerHandlers struct {
	metricsSvc *service.MetricsService
	plugin     *plugin.Plugin
}

func NewSqlServerPlanAnalyzerHandlers(metricsSvc *service.MetricsService) *SqlServerPlanAnalyzerHandlers {
	return &SqlServerPlanAnalyzerHandlers{
		metricsSvc: metricsSvc,
		plugin: plugin.New(plugin.Config{
			EnableRules:        true,
			EnableScoring:      true,
			EnableNarrative:    true,
			EnableCostAnalysis: true,
		}),
	}
}

// Analyze handles POST /api/sqlserver/plan/analyze
func (h *SqlServerPlanAnalyzerHandlers) Analyze(w http.ResponseWriter, r *http.Request) {
	// Support both multi-part file upload and raw body (XML)
	var planData []byte
	var err error

	if contentType := r.Header.Get("Content-Type"); contentType == "application/xml" || contentType == "text/xml" {
		planData, err = io.ReadAll(r.Body)
	} else {
		// Assume multi-part or form
		file, _, err2 := r.FormFile("plan_file")
		if err2 == nil {
			defer file.Close()
			planData, err = io.ReadAll(file)
		} else {
			// Try reading from a "plan_xml" form field
			xmlStr := r.FormValue("plan_xml")
			if xmlStr != "" {
				planData = []byte(xmlStr)
			} else {
				// Fallback to reading the whole body if no specific format is detected
				planData, err = io.ReadAll(r.Body)
			}
		}
	}

	if err != nil || len(planData) == 0 {
		http.Error(w, "Failed to read plan data: "+err.Error(), http.StatusBadRequest)
		return
	}

	analysis, err := h.plugin.AnalyzeBytes(r.Context(), planData)
	if err != nil {
		http.Error(w, "Failed to analyze plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	htmlReport := h.plugin.GenerateHTMLReport(analysis)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"analysis":    analysis,
		"html_report": htmlReport,
	})
}

// WatchedQueryPlanAnalysis handles GET /api/sqlserver/watched-queries/plan-analysis
func (h *SqlServerPlanAnalyzerHandlers) WatchedQueryPlanAnalysis(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	snapshotTimeStr := r.URL.Query().Get("snapshot_time")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid watched query ID", http.StatusBadRequest)
		return
	}

	snapshotTime, err := time.Parse(time.RFC3339, snapshotTimeStr)
	if err != nil {
		http.Error(w, "Invalid snapshot time (expected RFC3339)", http.StatusBadRequest)
		return
	}

	tsLogger := h.metricsSvc.GetTimescaleDBLogger()
	if tsLogger == nil {
		http.Error(w, "TimescaleDB not configured", http.StatusServiceUnavailable)
		return
	}

	// Fetch snapshots for the specific ID and time range around that snapshot
	// We need a specific snapshot, so we might need a more targeted storage method
	// For now, let's use GetSqlServerWatchedQuerySnapshots and find the closest match
	snapshots, err := tsLogger.GetSqlServerWatchedQuerySnapshots(r.Context(), id, snapshotTime.Add(-1*time.Minute), snapshotTime.Add(1*time.Minute))
	if err != nil {
		http.Error(w, "Failed to fetch snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var targetPlan string
	for _, s := range snapshots {
		if s.SnapshotTime.Equal(snapshotTime) || (s.SnapshotTime.After(snapshotTime.Add(-5*time.Second)) && s.SnapshotTime.Before(snapshotTime.Add(5*time.Second))) {
			targetPlan = s.QueryPlan
			break
		}
	}

	if targetPlan == "" {
		http.Error(w, "Query plan not found for the specified snapshot", http.StatusNotFound)
		return
	}

	analysis, err := h.plugin.AnalyzeBytes(r.Context(), []byte(targetPlan))
	if err != nil {
		http.Error(w, "Failed to analyze plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	htmlReport := h.plugin.GenerateHTMLReport(analysis)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"analysis":    analysis,
		"html_report": htmlReport,
	})
}
