// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server plan analyzer API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sqlplan-analyzer"
)

type SqlServerPlanHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerPlanHandlers(svc *service.MetricsService, cfg *config.Config) *SqlServerPlanHandlers {
	return &SqlServerPlanHandlers{metricsSvc: svc, cfg: cfg}
}

func (h *SqlServerPlanHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	return ParseServerID(r, h.cfg)
}

func (h *SqlServerPlanHandlers) AnalyzePlan(w http.ResponseWriter, r *http.Request) {
	// Instance is optional for XML-based analysis
	_, _ = h.parseID(r)

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB limit
	if err != nil || len(body) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "empty or unreadable plan XML"})
		return
	}

	analysis, err := sqlplan.AnalyzeBytes(body)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": "plan parse error: " + err.Error()})
		return
	}

	htmlReport, err := sqlplan.GenerateHTML(analysis)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "report generation error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"html_report":    htmlReport,
		"health_score":   analysis.HealthScore.OverallScore,
		"findings_count": len(analysis.Findings),
	})
}
