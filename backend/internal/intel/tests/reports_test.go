// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package tests


import (
	"strings"
	"testing"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/intel/reports"
)

func makeSampleAnalysis() *models.IntelligenceReportResponse {
	risk := models.RiskScoreResult{
		OverallScore:    45.0,
		Category:        "Elevated",
		PerformanceRisk: 50.0,
		CapacityRisk:    60.0,
	}

	return &models.IntelligenceReportResponse{
		RunID:       "test-run-123",
		OverallRisk: risk,
		WorkingWell: []string{"Backups completing successfully", "Replication stable"},
		CurrentIssues: []map[string]interface{}{
			{"title": "CPU Pressure", "description": "High CPU utilization", "severity": "high"},
		},
		WhatCouldGoWrong: []string{"Disk may fill in 14 days"},
		FailureForecast: []models.FailurePrediction{
			{Component: "Disk Storage", PredictedFailureWindow: "14 days", Confidence: "High", RiskType: "capacity"},
		},
		RootCauseHypotheses: []string{"Rising PAGEIOLATCH waits correlate with storage queue depth"},
		RecommendedActions: []map[string]string{
			{"action": "Add tempdb files", "urgency": "immediate", "reason": "TempDB spill at 1.3GB"},
			{"action": "Increase log disk capacity", "urgency": "short-term", "reason": "Log growing 800MB/day"},
		},
		TriggeredRules: []models.RuleTriggerResult{
			{
				RuleName:      "cpu_pressure",
				Severity:      "high",
				Message:       "CPU pressure detected",
				MetricValues:  map[string]float64{"avg_cpu_load": 92.0},
				Recommendation: []string{"Check expensive queries"},
			},
		},
		Forecasts: []models.ForecastResult{
			{
				MetricName:           "disk_usage",
				HorizonDays:          30,
				PredictedValues:      []float64{65, 70, 75, 80, 85},
				Confidence:           0.85,
				PredictedFailureDays: intPtr(14),
			},
		},
		ConfidenceScore:  0.72,
		NarrativeSummary: "Environment health is elevated with high CPU pressure.",
		GeneratedAt:      "2026-01-01T00:00:00",
	}
}

func intPtr(i int) *int { return &i }

func TestHTMLGenerationExecutive(t *testing.T) {
	gen := reports.NewReportGenerator("../templates")
	analysis := makeSampleAnalysis()
	html, err := gen.GenerateHTML(analysis, "executive", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	if !strings.Contains(html, "IntelligenceReportSqloptima") {
		t.Error("expected IntelligenceReportSqloptima in HTML")
	}
	if !strings.Contains(strings.ToLower(html), "score") {
		t.Error("expected score in HTML")
	}
}

func TestHTMLGenerationTechnical(t *testing.T) {
	gen := reports.NewReportGenerator("../templates")
	analysis := makeSampleAnalysis()
	html, err := gen.GenerateHTML(analysis, "technical", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	if !strings.Contains(html, "IntelligenceReportSqloptima") {
		t.Error("expected IntelligenceReportSqloptima in HTML")
	}
}

func TestJSONGeneration(t *testing.T) {
	gen := reports.NewReportGenerator("../templates")
	analysis := makeSampleAnalysis()
	jsonStr, err := gen.GenerateJSON(analysis)
	if err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}
	if !strings.Contains(jsonStr, `"run_id"`) {
		t.Error("expected run_id in JSON")
	}
	if !strings.Contains(jsonStr, `"overall_risk"`) {
		t.Error("expected overall_risk in JSON")
	}
}

func TestHTMLWithChartData(t *testing.T) {
	gen := reports.NewReportGenerator("../templates")
	analysis := makeSampleAnalysis()
	rawData := map[string]interface{}{
		"avg_cpu_load_series": []float64{40, 42, 44, 46, 48, 50},
		"tps_series":          []float64{100, 110, 120, 130, 140, 150},
		"free_disk_mb":        50000.0,
	}
	html, err := gen.GenerateHTML(analysis, "", rawData, nil, nil)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	if !strings.Contains(html, "Plotly") && !strings.Contains(html, "plotly") {
		t.Error("expected Plotly in HTML with chart data")
	}
}
