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

func (g *ReportGenerator) GenerateHTML(analysis *models.IntelligenceReportResponse, reportType string, rawData map[string]interface{}, dynamicThresholds map[string]interface{}, serverConfig map[string]interface{}, instanceName ...string) (string, error) {
	// Use report_v4 if available, fall back to v3 for backward compatibility.
	tmplName := "report_v4.html"
	tmplPath := filepath.Join(g.templateDir, tmplName)
	if _, statErr := os.Stat(tmplPath); os.IsNotExist(statErr) {
		tmplPath = filepath.Join(g.templateDir, "report_v3.html")
	}
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

	// DEFECT-12 fix: delta_data_mb is per ~15-min collection cycle; multiply by 96 to get
	// true daily growth. Use pre-aggregated daily_data_growth_mb_24h if available (set by
	// the service when it queries true 24h delta from sqlserver_disk_history).
	var dailyGrowthMB float64
	if v, ok := rawData["daily_data_growth_mb_24h"].(float64); ok && v > 0 {
		dailyGrowthMB = v
	} else if v, ok := rawData["delta_data_mb"].(float64); ok && v >= 0 {
		dailyGrowthMB = v * 96 // 96 cycles × 15 min = 24 h
	}
	dailyStorageGrowthGB := fmt.Sprintf("%.1f", dailyGrowthMB/1024.0)
	weeklyGrowthPct := "0"
	if total, ok := rawData["total_data_mb"].(float64); ok && total > 0 && dailyGrowthMB > 0 {
		weeklyGrowthPct = fmt.Sprintf("%.1f", (dailyGrowthMB*7/total)*100)
	}

	// Pre-compute dynamic template sections
	populated := countPopulatedMetrics(rawData)
	incidentTimeline := buildTemplateIncidentTimeline(analysis.TriggeredRules, genAt, rawData)
	capacityRows := buildTemplateCapacityRows(rawData)
	historyRows := buildTemplateHistoricalComparison(rawData)
	confidenceDiag := buildTemplateConfidenceDiagnostics(analysis.TriggeredRules, rawData, populated)
	rootCauseProbs := buildTemplateRootCauseProbabilities(analysis.TriggeredRules)
	top3Actions := buildTemplateTop3Actions(analysis.RecommendedActions, analysis.TriggeredRules)
	haNotConfigured, _ := rawData["ha_not_configured"].(bool)

	// Series data for Performance Analysis charts (null when not collected)
	cpuSeriesJSON     := rawSeriesJSON(rawData, "avg_cpu_load_series")
	pleSeriesJSON     := rawSeriesJSON(rawData, "ple_seconds_series")
	blockingSeriesJSON := rawSeriesJSON(rawData, "blocking_sessions_series")
	batchSeriesJSON   := rawSeriesJSON(rawData, "batch_requests_per_sec_series")
	freeDiskSeriesJSON := rawSeriesJSON(rawData, "free_disk_mb_series")
	runnableSeriesJSON := rawSeriesJSON(rawData, "runnable_tasks_series")
	readLatSeriesJSON  := rawSeriesJSON(rawData, "read_latency_ms_series")
	writeLatSeriesJSON := rawSeriesJSON(rawData, "write_latency_ms_series")

	// Wait category breakdown for the Waits tab (null when wait history not collected).
	waitCategoriesJSON := buildWaitCategoriesJSON(rawData)

	// CPU heatmap (7×24 Plotly heatmap grid — full slot matrix from PeakWindow.HeatmapSlots).
	var cpuHeatmapJSON []byte
	if analysis.PeakWindow != nil {
		cpuHeatmapJSON, _ = json.Marshal(analysis.PeakWindow.HeatmapSlots)
	} else {
		cpuHeatmapJSON = []byte("[]")
	}

	// HA replica status + trend series (populated in GetRawDataSnapshot items 15-17).
	haReplicaStatusJSON := func() string {
		if v, ok := rawData["ha_replica_states"]; ok {
			b, _ := json.Marshal(v)
			return string(b)
		}
		return "[]"
	}()

	// Wait trend (7-day hourly stacked area chart — from WaitTrend v2 field).
	waitTrendJSON, _ := json.Marshal(analysis.WaitTrend)

	// Memory timeline series (query 7.8 — 7-day PLE from history hypertable + 24h grants).
	memHistPLEJSON := rawSeriesJSON(rawData, "memory_history_ple_series")
	memGrantsJSON := rawSeriesJSON(rawData, "memory_grants_pending_series")
	sortWarnsJSON := rawSeriesJSON(rawData, "sort_warnings_series")
	hashWarnsJSON := rawSeriesJSON(rawData, "hash_warnings_series")

	// Disk used % for storage gauge (0 means no data)
	storageUsedPct := 0.0
	if freeV, ok := rawData["free_disk_mb"].(float64); ok && freeV >= 0 {
		dataV, _ := rawData["data_disk_mb"].(float64)
		logV, _ := rawData["log_disk_mb"].(float64)
		total := dataV + logV + freeV
		if total > 0 {
			storageUsedPct = (total - freeV) / total * 100.0
		}
	}

	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"safeJS": func(s string) template.JS {
			// SEC-2 / DEFECT-14: Only allow strings that are valid, non-null JSON objects or arrays.
			// The string "null" looks like it might pass a naive prefix check but is explicitly
			// rejected here — Plotly silently fails on null input, producing blank charts.
			trimmed := strings.TrimSpace(s)
			if trimmed == "null" || trimmed == "" {
				return template.JS("[]")
			}
			if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) || (strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
				return template.JS(s)
			}
			return template.JS("[]")
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
		"float64": func(v interface{}) float64 {
			switch val := v.(type) {
			case int:
				return float64(val)
			case int64:
				return float64(val)
			case float64:
				return val
			default:
				return 0
			}
		},
		"round0": func(v float64) string { return fmt.Sprintf("%.0f", v) },
		"int": func(v float64) int { return int(math.Round(v)) },
		"printf": func(format string, v interface{}) string {
			return fmt.Sprintf(format, v)
		},
		"divf": func(a, b float64) float64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"absf": math.Abs,
		"drillRoute": RuleDrillRoute,
	}

	tmpl := template.Must(template.New("report").Funcs(funcMap).Parse(string(tmplContent)))

	capacityTrendBadge, capacityTrendStyle := computeCapacityTrendBadge(weeklyGrowthPct)
	serverLabel := analysis.RunID
	if len(instanceName) > 0 && instanceName[0] != "" {
		serverLabel = instanceName[0]
	} else if len(serverLabel) > 8 {
		serverLabel = serverLabel[:8]
	}

	// V2 fields for the redesigned template
	var utilizationClass, utilizationMsg string
	if analysis.UtilizationProfile != nil {
		utilizationClass = analysis.UtilizationProfile.Class
		utilizationMsg = analysis.UtilizationProfile.Message
	}
	var peakHours, peakDays string
	if analysis.PeakWindow != nil {
		peakHours = analysis.PeakWindow.PeakHours
		peakDays = strings.Join(analysis.PeakWindow.PeakDays, ", ")
	}
	topWaitTypesJSON, _ := json.Marshal(analysis.TopWaitTypes)
	dbGrowthJSON := "[]"
	historyTrendJSON := "[]"
	if analysis.CapacityForecastV2 != nil {
		b, _ := json.Marshal(analysis.CapacityForecastV2.Databases)
		dbGrowthJSON = string(b)
	}
	if len(analysis.HistoryTrend) > 0 {
		b, _ := json.Marshal(analysis.HistoryTrend)
		historyTrendJSON = string(b)
	}

	data := map[string]interface{}{
		"RunID":                     analysis.RunID,
		"ServerLabel":               serverLabel,
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
		"ImmediateRecs":             filterRecsByUrgency(analysis.RecommendedActions, "immediate"),
		"ShortTermRecs":             filterRecsByUrgency(analysis.RecommendedActions, "short-term"),
		"StrategicRecs":             filterRecsByUrgency(analysis.RecommendedActions, "strategic"),
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
		"CapacityTrendBadge":        capacityTrendBadge,
		"CapacityTrendStyle":        capacityTrendStyle,
		// Dynamic sections (previously hardcoded in template)
		"DataStatus":               analysis.DataStatus,
		"DataNote":                 analysis.DataNote,
		"IncidentTimeline":         incidentTimeline,
		"CapacityTableRows":        capacityRows,
		"HistoricalComparisonRows": historyRows,
		"ConfidenceDiagnostics":    confidenceDiag,
		"RootCauseProbabilities":   rootCauseProbs,
		"Top3Actions":              top3Actions,
		"HANotConfigured":          haNotConfigured,
		// Performance Analysis chart series
		"CPUSeriesJSON":            cpuSeriesJSON,
		"PLESeriesJSON":            pleSeriesJSON,
		"BlockingSeriesJSON":       blockingSeriesJSON,
		"BatchSeriesJSON":          batchSeriesJSON,
		"FreeDiskSeriesJSON":       freeDiskSeriesJSON,
		"RunnableSeriesJSON":       runnableSeriesJSON,
		"ReadLatSeriesJSON":        readLatSeriesJSON,
		"WriteLatSeriesJSON":       writeLatSeriesJSON,
		"WaitCategoriesJSON":       waitCategoriesJSON,
		"WaitTrendJSON":            string(waitTrendJSON),
		"CPUHeatmapJSON":           string(cpuHeatmapJSON),
		"HAReplicaStatusJSON":      haReplicaStatusJSON,
		"HARPOSeriesJSON":          rawSeriesJSON(rawData, "ha_rpo_seconds_series"),
		"HARTOSeriesJSON":          rawSeriesJSON(rawData, "ha_rto_seconds_series"),
		"MemHistoryPLEJSON":        memHistPLEJSON,
		"MemGrantsPendingJSON":     memGrantsJSON,
		"SortWarningsJSON":         sortWarnsJSON,
		"HashWarningsJSON":         hashWarnsJSON,
		"StorageUsedPct":           storageUsedPct,
		// V2 fields
		"UtilizationClass":         utilizationClass,
		"UtilizationMessage":       utilizationMsg,
		"PeakHours":                peakHours,
		"PeakDays":                 peakDays,
		"PeakWindow":               analysis.PeakWindow,
		"TopWaitTypesJSON":         string(topWaitTypesJSON),
		"DatabaseGrowthJSON":       dbGrowthJSON,
		"HistoryTrendJSON":         historyTrendJSON,
		"CapacityForecastV2":       analysis.CapacityForecastV2,
		"UtilizationProfile":       analysis.UtilizationProfile,
		"QueryWorkload":            analysis.QueryWorkload,
		"IndexHealth":              analysis.IndexHealth,
		"ServerConfiguration":    analysis.ServerConfiguration,
		"PerformanceDebtItems":   analysis.PerformanceDebtItems,
		"AppName":                  "Intelligence Report Sqloptima",
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
			"margin":        map[string]interface{}{"t": 10, "b": 65, "l": 40, "r": 10},
			"paper_bgcolor": "rgba(0,0,0,0)",
			"plot_bgcolor":  "rgba(0,0,0,0)",
			"font":          map[string]interface{}{"color": "#94a3b8", "size": 11},
			"xaxis":         map[string]interface{}{"gridcolor": "#2d3154", "title": "Historical Sequence"},
			"yaxis":         map[string]interface{}{"gridcolor": "#2d3154", "title": "Risk Score", "range": []float64{0, 100}},
			"legend":        map[string]interface{}{"orientation": "h", "y": -0.28, "x": 0, "xanchor": "left"},
			"hovermode":     "x unified",
		},
	}
}

func filterRecsByUrgency(recs []map[string]string, urgency string) []map[string]string {
	var result []map[string]string
	for _, r := range recs {
		if r["urgency"] == urgency {
			result = append(result, r)
		}
	}
	return result
}

func computeCapacityTrendBadge(weeklyGrowthPct string) (string, string) {
	var pct float64
	_, _ = fmt.Sscanf(weeklyGrowthPct, "%f", &pct)
	switch {
	case pct >= 5:
		return "CRITICAL", "background:var(--danger-bg);color:var(--red)"
	case pct >= 2:
		return "ELEVATED", "background:var(--warning-bg);color:var(--orange)"
	case pct > 0:
		return "GROWING", "background:var(--warning-bg);color:var(--yellow)"
	default:
		return "STABLE", "background:var(--success);color:var(--green)"
	}
}

// buildWaitCategoriesJSON serialises the four wait-category scalars from rawData into a
// JSON object the template can use directly for a Plotly bar chart.
// Returns "null" when no wait history has been collected for this server.
func buildWaitCategoriesJSON(rawData map[string]interface{}) string {
	disk, hasDisk := rawData["wait_disk_ms_per_sec"].(float64)
	blocking, hasBlocking := rawData["wait_blocking_ms_per_sec"].(float64)
	parallelism, hasParallelism := rawData["wait_parallelism_ms_per_sec"].(float64)
	other, hasOther := rawData["wait_other_ms_per_sec"].(float64)

	if !hasDisk && !hasBlocking && !hasParallelism && !hasOther {
		return "null"
	}

	type waitEntry struct {
		Category string  `json:"category"`
		ValueMS  float64 `json:"value_ms_per_sec"`
		Color    string  `json:"color"`
	}
	entries := []waitEntry{
		{"Disk I/O", disk, "#ef4444"},
		{"Blocking / Locks", blocking, "#f97316"},
		{"Parallelism (CXPACKET)", parallelism, "#eab308"},
		{"Other", other, "#3b82f6"},
	}

	b, err := json.Marshal(entries)
	if err != nil {
		return "null"
	}
	return string(b)
}

// rawSeriesJSON returns a JSON array string for a named series in rawData, or "null" if absent.
func rawSeriesJSON(rawData map[string]interface{}, key string) string {
	switch v := rawData[key].(type) {
	case []float64:
		if len(v) > 0 {
			b, _ := json.Marshal(v)
			return string(b)
		}
	case []interface{}:
		if len(v) > 0 {
			b, _ := json.Marshal(v)
			return string(b)
		}
	}
	return "null"
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

