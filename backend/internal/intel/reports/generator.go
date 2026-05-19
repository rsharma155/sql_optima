// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package reports


import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/rsharma155/sql_optima/internal/models"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type ReportGenerator struct {
	templateDir string
}

func NewReportGenerator(templateDir string) *ReportGenerator {
	return &ReportGenerator{templateDir: templateDir}
}

func (g *ReportGenerator) GenerateHTML(analysis *models.IntelligenceReportResponse, reportType string, rawData map[string]interface{}, dynamicThresholds map[string]interface{}, serverConfig map[string]interface{}) (string, error) {
	tmplPath := filepath.Join(g.templateDir, "report_v3.html")
	tmplContent, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("reading template: %w", err)
	}

	criticalCount := 0
	highCount := 0
	for _, r := range analysis.TriggeredRules {
		if r.Severity == "critical" {
			criticalCount++
		}
		if r.Severity == "high" {
			highCount++
		}
	}

	currentIssuesJSON, _ := json.Marshal(analysis.CurrentIssues)
	riskTrendJSON, _ := json.Marshal(g.buildRiskTrendFromData(analysis))
	capacityForecastJSON, _ := json.Marshal(g.buildCapacityForecastFromData(analysis))
	fullAnalysisJSON, _ := json.Marshal(analysis)

	genAt := analysis.GeneratedAt
	genAtShort := genAt
	if len(genAtShort) > 16 {
		genAtShort = genAtShort[:16]
	}

	confidencePct := int(math.Round(analysis.ConfidenceScore * 100))

	var earliestFailureDays int
	var earliestFailureComponent string
	var earliestFailureRiskType string
	var earliestFailureColor string
	var failureBadgeStyle string
	var failureBadgeText string
	if len(analysis.FailureForecast) > 0 {
		ff := analysis.FailureForecast[0]
		earliestFailureComponent = ff.Component
		earliestFailureRiskType = ff.RiskType
		days := 0
		_, _ = fmt.Sscanf(ff.PredictedFailureWindow, "%d", &days)
		earliestFailureDays = days
		if days < 10 {
			earliestFailureColor = "var(--red)"
			failureBadgeStyle = "background:var(--danger-bg);color:var(--red)"
			failureBadgeText = "HIGH"
		} else if days < 21 {
			earliestFailureColor = "var(--orange)"
			failureBadgeStyle = "background:var(--warning-bg);color:var(--orange)"
			failureBadgeText = "MEDIUM"
		} else {
			earliestFailureColor = "var(--yellow)"
			failureBadgeStyle = "background:var(--success);color:var(--green)"
			failureBadgeText = "LOW"
		}
	} else {
		failureBadgeStyle = "background:var(--success);color:var(--green)"
		failureBadgeText = "OK"
	}

	overallRiskBarColor := "#22c55e"
	overallRiskColor := "var(--green)"
	switch {
	case analysis.OverallRisk.OverallScore > 75:
		overallRiskBarColor = "#ef4444"
		overallRiskColor = "var(--red)"
	case analysis.OverallRisk.OverallScore > 50:
		overallRiskBarColor = "#f97316"
		overallRiskColor = "var(--orange)"
	case analysis.OverallRisk.OverallScore > 25:
		overallRiskBarColor = "#eab308"
		overallRiskColor = "var(--yellow)"
	}

	deltaMB := 0.0
	if v, ok := rawData["delta_data_mb"].(float64); ok {
		deltaMB = v
	}
	dailyStorageGrowthGB := fmt.Sprintf("%.1f", deltaMB/1024.0)
	weeklyGrowthPct := "0"
	if total, ok := rawData["total_data_mb"].(float64); ok && total > 0 {
		weeklyGrowthPct = fmt.Sprintf("%.1f", (deltaMB*7/total)*100)
	}

	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"safeJS": func(s string) template.JS {
			// SEC-2: Only allow strings that look like JSON objects/arrays to pass through template.JS
			trimmed := strings.TrimSpace(s)
			if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) || (strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
				return template.JS(s)
			}
			return template.JS("null")
		},
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"titleStr": func(s string) string {
			return cases.Title(language.Und).String(s)
		},
		"toUpper": strings.ToUpper,
		"toLower": strings.ToLower,
		"replaceAll": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"truncate": func(max int, s string) string {
			if len(s) <= max {
				return s
			}
			return s[:max] + "..."
		},
		"add": func(a, b float64) float64 { return a + b },
		"sub": func(a, b float64) float64 { return a - b },
		"round0": func(v float64) string { return fmt.Sprintf("%.0f", v) },
		"int": func(v float64) int { return int(math.Round(v)) },
		"printf": func(format string, v interface{}) string {
			return fmt.Sprintf(format, v)
		},
	}

	tmpl := template.Must(template.New("report").Funcs(funcMap).Parse(string(tmplContent)))

	data := map[string]interface{}{
		"RunID":                     analysis.RunID,
		"GeneratedAt":               genAt,
		"GeneratedAtShort":          genAtShort,
		"OverallRisk":               analysis.OverallRisk,
		"OverallRiskColor":          overallRiskColor,
		"OverallRiskBarColor":       overallRiskBarColor,
		"NarrativeSummary":          analysis.NarrativeSummary,
		"WorkingWell":               analysis.WorkingWell,
		"CurrentIssues":             analysis.CurrentIssues,
		"WhatCouldGoWrong":          analysis.WhatCouldGoWrong,
		"FailureForecast":           analysis.FailureForecast,
		"RecommendedActions":        analysis.RecommendedActions,
		"TriggeredRules":            analysis.TriggeredRules,
		"Forecasts":                 analysis.Forecasts,
		"RootCauseHypotheses":       analysis.RootCauseHypotheses,
		"ConfidenceScore":           analysis.ConfidenceScore,
		"ConfidencePct":             confidencePct,
		"CriticalCount":             criticalCount,
		"HighCount":                 highCount,
		"CurrentIssuesJSON":         string(currentIssuesJSON),
		"RiskTrendJSON":             string(riskTrendJSON),
		"CapacityForecastJSON":      string(capacityForecastJSON),
		"FullAnalysisJSON":          string(fullAnalysisJSON),
		"DynamicThresholds":         dynamicThresholds,
		"ServerConfig":              serverConfig,
		"EarliestFailureDays":       earliestFailureDays,
		"EarliestFailureComponent":  earliestFailureComponent,
		"EarliestFailureRiskType":   earliestFailureRiskType,
		"EarliestFailureColor":      earliestFailureColor,
		"FailureBadgeStyle":         failureBadgeStyle,
		"FailureBadgeText":          failureBadgeText,
		"WeeklyGrowthPct":           weeklyGrowthPct,
		"DailyStorageGrowthGB":      dailyStorageGrowthGB,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

func (g *ReportGenerator) GenerateJSON(analysis *models.IntelligenceReportResponse) (string, error) {
	b, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (g *ReportGenerator) buildRiskTrendFromData(analysis *models.IntelligenceReportResponse) map[string]interface{} {
	if len(analysis.RiskTrend) == 0 {
		return map[string]interface{}{"empty": true}
	}

	x := make([]int, len(analysis.RiskTrend))
	overall := make([]float64, len(analysis.RiskTrend))
	capacity := make([]float64, len(analysis.RiskTrend))
	perf := make([]float64, len(analysis.RiskTrend))
	avail := make([]float64, len(analysis.RiskTrend))

	for i, r := range analysis.RiskTrend {
		x[i] = i - len(analysis.RiskTrend) + 1
		overall[i] = r.OverallScore
		capacity[i] = r.CapacityRisk
		perf[i] = r.PerformanceRisk
		avail[i] = r.AvailabilityRisk
	}

	return map[string]interface{}{
		"data": []map[string]interface{}{
			{"x": x, "y": overall, "type": "scatter", "mode": "lines+markers", "name": "Overall Risk", "line": map[string]interface{}{"color": "#ef4444", "width": 2}},
			{"x": x, "y": capacity, "type": "scatter", "mode": "lines+markers", "name": "Capacity Risk", "line": map[string]interface{}{"color": "#f97316", "width": 2}},
			{"x": x, "y": perf, "type": "scatter", "mode": "lines+markers", "name": "Performance Risk", "line": map[string]interface{}{"color": "#eab308", "width": 2}},
			{"x": x, "y": avail, "type": "scatter", "mode": "lines+markers", "name": "Availability Risk", "line": map[string]interface{}{"color": "#22c55e", "width": 2}},
		},
		"layout": map[string]interface{}{
			"margin":        map[string]interface{}{"t": 10, "b": 30, "l": 40, "r": 10},
			"paper_bgcolor": "rgba(0,0,0,0)",
			"plot_bgcolor":  "rgba(0,0,0,0)",
			"font":          map[string]interface{}{"color": "#94a3b8", "size": 11},
			"xaxis":         map[string]interface{}{"gridcolor": "#2d3154", "title": "Historical Sequence"},
			"yaxis":         map[string]interface{}{"gridcolor": "#2d3154", "title": "Risk Score", "range": []float64{0, 100}},
			"legend":        map[string]interface{}{"orientation": "h", "y": 1.1, "x": 0},
			"hovermode":      "x unified",
		},
	}
}

func (g *ReportGenerator) buildCapacityForecastFromData(analysis *models.IntelligenceReportResponse) map[string]interface{} {
	var diskFC *models.ForecastResult
	for _, f := range analysis.Forecasts {
		if strings.Contains(strings.ToLower(f.MetricName), "disk") || strings.Contains(strings.ToLower(f.MetricName), "free") {
			diskFC = &f
			break
		}
	}

	if diskFC == nil || len(diskFC.PredictedValues) == 0 {
		return map[string]interface{}{"empty": true}
	}

	days := make([]int, len(diskFC.PredictedValues))
	for i := range days {
		days[i] = i + 1
	}

	return map[string]interface{}{
		"data": []map[string]interface{}{
			{"x": days, "y": diskFC.PredictedValues, "type": "scatter", "mode": "lines", "name": diskFC.MetricName, "line": map[string]interface{}{"color": "#3b82f6", "width": 2}, "fill": "tozeroy", "fillcolor": "rgba(59,130,246,0.1)"},
		},
		"layout": map[string]interface{}{
			"margin":        map[string]interface{}{"t": 10, "b": 30, "l": 40, "r": 10},
			"paper_bgcolor": "rgba(0,0,0,0)",
			"plot_bgcolor":  "rgba(0,0,0,0)",
			"font":          map[string]interface{}{"color": "#94a3b8", "size": 11},
			"xaxis":         map[string]interface{}{"gridcolor": "#2d3154", "title": "Days Forecast"},
			"yaxis":         map[string]interface{}{"gridcolor": "#2d3154", "title": "Predicted Value", "range": []float64{0, 100}},
			"hovermode":      "x unified",
			"legend":        map[string]interface{}{"orientation": "h", "y": 1.1},
		},
	}
}

