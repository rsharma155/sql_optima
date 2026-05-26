// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Helper functions that compute dynamic template data for the HTML intelligence report.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package reports

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/rsharma155/sql_optima/internal/models"
)

func getFloatRaw(raw map[string]interface{}, key string, def float64) float64 {
	if v, ok := raw[key].(float64); ok {
		return v
	}
	return def
}

// countPopulatedMetrics mirrors the service-layer logic so the generator can compute
// data completeness without depending on the service package.
func countPopulatedMetrics(rawData map[string]interface{}) int {
	keys := []string{
		"avg_cpu_load", "sql_process", "ple_seconds", "memory_usage",
		"free_disk_mb", "delta_data_mb", "blocking_sessions", "tempdb_used_percent",
		"failed_jobs_24h", "total_runnable_tasks_count", "read_latency_ms",
		"secondary_lag_seconds", "performance_debt_count", "cpu_count", "total_ram_gb",
		"avg_cpu_load_series", "free_disk_mb_series", "ple_seconds_series",
		"batch_requests_per_sec_series", "blocking_sessions_series",
	}
	count := 0
	for _, k := range keys {
		if v, ok := rawData[k]; ok && v != nil {
			switch val := v.(type) {
			case float64:
				count++
				_ = val
			case []float64:
				if len(val) > 0 {
					count++
				}
			case bool:
				count++
			}
		}
	}
	return count
}

// buildTemplateIncidentTimeline derives timeline events from triggered rules.
// When rawData contains "first_breach_timestamps" (populated by the service layer via
// a conditional-aggregation query against sqlserver_risk_health), real breach times are
// used for matching rules. Remaining rules fall back to a relative "~Xm before report"
// label so the timeline never shows fake absolute timestamps.
func buildTemplateIncidentTimeline(rules []models.RuleTriggerResult, genAt string, rawData ...map[string]interface{}) []map[string]string {
	genTime, err := time.Parse(time.RFC3339, genAt)
	if err != nil || genTime.IsZero() {
		genTime = time.Now()
	}

	// Extract real breach timestamps if available.
	var breachMap map[string]interface{}
	if len(rawData) > 0 {
		if bm, ok := rawData[0]["first_breach_timestamps"].(map[string]interface{}); ok {
			breachMap = bm
		}
	}

	sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	sorted := make([]models.RuleTriggerResult, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sevOrder[sorted[i].Severity] < sevOrder[sorted[j].Severity]
	})

	var timeline []map[string]string
	for i, rule := range sorted {
		if rule.Severity == "low" {
			continue
		}
		label := cases.Title(language.Und).String(strings.ReplaceAll(rule.RuleName, "_", " "))
		detail := rule.Message
		if len(detail) > 120 {
			detail = detail[:117] + "..."
		}

		// Prefer real breach timestamp from the DB query.
		timeStr := ""
		if breachMap != nil {
			if ts, ok := breachMap[rule.RuleName].(string); ok {
				if bt, parseErr := time.Parse(time.RFC3339, ts); parseErr == nil {
					ago := genTime.Sub(bt)
					switch {
					case ago < 2*time.Minute:
						timeStr = "just now"
					case ago < 60*time.Minute:
						timeStr = fmt.Sprintf("~%dm ago", int(ago.Minutes()))
					case ago < 24*time.Hour:
						timeStr = bt.Format("15:04")
					default:
						timeStr = bt.Format("Jan 2 15:04")
					}
				}
			}
		}

		// Fall back to relative position label (honest about not having exact time).
		if timeStr == "" {
			position := len(sorted) - i
			if position <= 1 {
				timeStr = "current"
			} else {
				timeStr = fmt.Sprintf("~%dh window", position/4+1)
			}
		}

		timeline = append(timeline, map[string]string{
			"time":           timeStr,
			"event":          label,
			"detail":         detail,
			"severity":       rule.Severity,
			"metric_context": buildMetricContext(rule.MetricValues, rule.RuleName),
			"rule_name":      rule.RuleName,
		})
		if len(timeline) >= 8 {
			break
		}
	}
	return timeline
}

// buildMetricContext returns a short human-readable metric summary for an incident card.
func buildMetricContext(mv map[string]float64, ruleName string) string {
	// Priority metric per rule — pick the most meaningful signal for the card subtitle.
	priority := map[string]string{
		"cpu_saturation":         "avg_cpu_load",
		"cpu_burst":              "avg_cpu_load",
		"scheduler_starvation":   "avg_cpu_load",
		"ple_collapse":           "ple_seconds",
		"memory_grant_pressure":  "memory_grants_pending",
		"low_disk_space":         "free_disk_mb",
		"rapid_disk_growth":      "delta_data_mb",
		"blocking_chains":        "blocking_sessions",
		"tempdb_pressure":        "tempdb_used_percent",
		"io_latency_high":        "read_latency_ms",
		"replication_lag":        "replication_lag_seconds",
		"ha_replication_lag":     "replication_lag_seconds",
	}

	key := priority[ruleName]
	if key == "" {
		// Fall back to first non-zero metric in the map
		for k, v := range mv {
			if v > 0 {
				key = k
				break
			}
		}
	}
	if key == "" {
		return ""
	}
	v, exists := mv[key]
	if !exists || v <= 0 {
		return ""
	}
	switch key {
	case "avg_cpu_load":
		return fmt.Sprintf("CPU: %.0f%%", v)
	case "ple_seconds":
		return fmt.Sprintf("PLE: %.0fs", v)
	case "free_disk_mb":
		return fmt.Sprintf("Free: %.1fGB", v/1024)
	case "delta_data_mb":
		return fmt.Sprintf("Growth: %.0fMB/cycle", v)
	case "blocking_sessions":
		return fmt.Sprintf("Blocking: %.0f sessions", v)
	case "tempdb_used_percent":
		return fmt.Sprintf("TempDB: %.0f%%", v)
	case "read_latency_ms":
		return fmt.Sprintf("Read: %.0fms", v)
	case "replication_lag_seconds":
		return fmt.Sprintf("Lag: %.0fs", v)
	case "memory_grants_pending":
		return fmt.Sprintf("Grants pending: %.0f", v)
	default:
		label := cases.Title(language.Und).String(strings.ReplaceAll(key, "_", " "))
		if v == float64(int(v)) {
			return fmt.Sprintf("%s: %.0f", label, v)
		}
		return fmt.Sprintf("%s: %.1f", label, v)
	}
}

// buildTemplateCapacityRows computes current disk/memory usage and 30/90-day projections
// from raw collected metrics. Returns nil when disk history data is unavailable.
func buildTemplateCapacityRows(rawData map[string]interface{}) []map[string]interface{} {
	totalDataMB := getFloatRaw(rawData, "total_data_mb", 0)
	totalLogMB  := getFloatRaw(rawData, "total_log_mb", 0)
	totalFreeMB := getFloatRaw(rawData, "total_free_mb", 0)
	deltaDataMB := getFloatRaw(rawData, "delta_data_mb", 0)
	tempdbPct   := getFloatRaw(rawData, "tempdb_used_percent", 0)
	memPct      := getFloatRaw(rawData, "memory_usage", 0)
	totalRAMGB  := getFloatRaw(rawData, "total_ram_gb", 0)

	totalDiskMB := totalDataMB + totalLogMB + totalFreeMB
	if totalDiskMB <= 0 {
		return nil
	}

	usedMB      := totalDiskMB - totalFreeMB
	dataUsedPct := usedMB / totalDiskMB * 100

	// Prefer the pre-aggregated 24h daily growth value from the service (accurate).
	// Fall back to delta_data_mb * 96 collection cycles when the history query is absent.
	dailyGrowthMB := getFloatRaw(rawData, "daily_data_growth_mb_24h", 0)
	if dailyGrowthMB <= 0 {
		dailyGrowthMB = deltaDataMB * 96
	}
	if dailyGrowthMB < 0 {
		dailyGrowthMB = 0
	}

	projectPct := func(baseUsedMB, growth, capacity, days float64) string {
		if capacity <= 0 {
			return "N/A"
		}
		projUsed := baseUsedMB + (growth * days)
		pct := (projUsed / capacity) * 100
		if pct > 100 {
			pct = 100
		}
		return fmt.Sprintf("%.0f%%", pct)
	}

	pctColor := func(pct float64) string {
		switch {
		case pct >= 90:
			return "var(--red)"
		case pct >= 75:
			return "var(--orange)"
		case pct >= 60:
			return "var(--yellow)"
		default:
			return "var(--green)"
		}
	}

	growthLabel := func(mbPerDay float64) string {
		if mbPerDay <= 0 {
			return "Stable"
		} else if mbPerDay < 100 {
			return fmt.Sprintf("%.0f MB/day", mbPerDay)
		}
		return fmt.Sprintf("%.1f GB/day", mbPerDay/1024)
	}

	priorityFor := func(currentPct, d30Pct, d90Pct float64) (string, string) {
		switch {
		case currentPct >= 90 || d30Pct >= 90:
			return "Critical", "background:var(--danger-bg);color:var(--red);font-weight:700"
		case currentPct >= 75 || d30Pct >= 85:
			return "High", "background:var(--warning-bg);color:var(--orange);font-weight:700"
		case d90Pct >= 75:
			return "Medium", "background:rgba(234,179,8,0.15);color:var(--yellow);font-weight:700"
		default:
			return "Low", "background:rgba(34,197,94,0.15);color:var(--green);font-weight:700"
		}
	}

	actionFor := func(currentPct, d30Pct, d90Pct float64, growthMB float64) string {
		switch {
		case currentPct >= 90:
			return "Emergency: immediate storage expansion or data purge required"
		case d30Pct >= 90:
			return fmt.Sprintf("Urgent: breach in ~30 days at %.0f MB/day growth — add capacity now", growthMB)
		case d90Pct >= 85:
			return "Plan storage expansion within 60 days; implement archiving"
		case d90Pct >= 70:
			return "Schedule capacity review this quarter; enable data compression"
		case d90Pct >= 55:
			return "Monitor growth weekly; set up automated disk alerts"
		default:
			return "No action required — headroom sufficient for 90+ days"
		}
	}

	var d30PctVal, d90PctVal float64
	if totalDiskMB > 0 {
		d30PctVal = math.Min((usedMB+dailyGrowthMB*30)/totalDiskMB*100, 100)
		d90PctVal = math.Min((usedMB+dailyGrowthMB*90)/totalDiskMB*100, 100)
	}
	d30 := projectPct(usedMB, dailyGrowthMB, totalDiskMB, 30)
	d90 := projectPct(usedMB, dailyGrowthMB, totalDiskMB, 90)
	pLabel, pStyle := priorityFor(dataUsedPct, d30PctVal, d90PctVal)

	rows := []map[string]interface{}{
		{
			"resource":       "Data + Log Files",
			"current_gb":     usedMB / 1024,
			"current_pct":    fmt.Sprintf("%.0f%%", dataUsedPct),
			"pct_color":      pctColor(dataUsedPct),
			"growth_rate":    growthLabel(dailyGrowthMB),
			"d30_pct":        d30,
			"d90_pct":        d90,
			"priority":       pLabel,
			"priority_style": pStyle,
			"action":         actionFor(dataUsedPct, d30PctVal, d90PctVal, dailyGrowthMB),
		},
	}

	if totalLogMB > 0 {
		logPct := totalLogMB / totalDiskMB * 100
		// Log files grow with write workload; approximate at 30% of total disk growth.
		logGrowthMB := dailyGrowthMB * 0.3
		l30V := math.Min((totalLogMB+logGrowthMB*30)/totalDiskMB*100, 100)
		l90V := math.Min((totalLogMB+logGrowthMB*90)/totalDiskMB*100, 100)
		l30 := projectPct(totalLogMB, logGrowthMB, totalDiskMB, 30)
		l90 := projectPct(totalLogMB, logGrowthMB, totalDiskMB, 90)
		lLabel, lStyle := priorityFor(logPct, l30V, l90V)
		rows = append(rows, map[string]interface{}{
			"resource":       "Transaction Log",
			"current_gb":     totalLogMB / 1024,
			"current_pct":    fmt.Sprintf("%.0f%%", logPct),
			"pct_color":      pctColor(logPct),
			"growth_rate":    growthLabel(logGrowthMB),
			"d30_pct":        l30,
			"d90_pct":        l90,
			"priority":       lLabel,
			"priority_style": lStyle,
			"action":         actionFor(logPct, l30V, l90V, logGrowthMB),
		})
	}

	if tempdbPct > 0 {
		// TempDB is typically dynamic (recreated on restart); project conservatively.
		t30V := math.Min(tempdbPct*1.15, 100)
		t90V := math.Min(tempdbPct*1.40, 100)
		tLabel, tStyle := priorityFor(tempdbPct, t30V, t90V)
		rows = append(rows, map[string]interface{}{
			"resource":       "TempDB",
			"current_gb":     0.0,
			"current_pct":    fmt.Sprintf("%.0f%%", tempdbPct),
			"pct_color":      pctColor(tempdbPct),
			"growth_rate":    "Workload-driven",
			"d30_pct":        fmt.Sprintf("%.0f%%", t30V),
			"d90_pct":        fmt.Sprintf("%.0f%%", t90V),
			"priority":       tLabel,
			"priority_style": tStyle,
			"action":         actionFor(tempdbPct, t30V, t90V, 0),
		})
	}

	if memPct > 0 && totalRAMGB > 0 {
		memUsedGB := totalRAMGB * memPct / 100
		// Memory grows with active workload; meaningful only for trend.
		m30V := math.Min(memPct+3, 100)
		m90V := math.Min(memPct+8, 100)
		mLabel, mStyle := priorityFor(memPct, m30V, m90V)
		rows = append(rows, map[string]interface{}{
			"resource":       "Memory (RAM)",
			"current_gb":     memUsedGB,
			"current_pct":    fmt.Sprintf("%.0f%%", memPct),
			"pct_color":      pctColor(memPct),
			"growth_rate":    "Workload-driven",
			"d30_pct":        fmt.Sprintf("%.0f%%", m30V),
			"d90_pct":        fmt.Sprintf("%.0f%%", m90V),
			"priority":       mLabel,
			"priority_style": mStyle,
			"action":         actionFor(memPct, m30V, m90V, 0),
		})
	}

	return rows
}

// buildTemplateHistoricalComparison compares the current snapshot against the oldest
// available point in the 7-day risk history and metric series.
func buildTemplateHistoricalComparison(rawData map[string]interface{}) []map[string]interface{} {
	currentCPU    := getFloatRaw(rawData, "avg_cpu_load", 0)
	currentPLE    := getFloatRaw(rawData, "ple_seconds", getFloatRaw(rawData, "ple", 0))
	currentFreeMB := getFloatRaw(rawData, "total_free_mb", getFloatRaw(rawData, "free_disk_mb", 0))
	currentBlock  := getFloatRaw(rawData, "blocking_sessions", 0)
	currentTempDB := getFloatRaw(rawData, "tempdb_used_percent", 0)

	var prevCPU, prevPLE, prevFreeMB, prevBlock, prevTempDB float64

	if series, ok := rawData["avg_cpu_load_series"].([]float64); ok && len(series) > 1 {
		prevCPU = series[0]
	}
	if series, ok := rawData["free_disk_mb_series"].([]float64); ok && len(series) > 1 {
		prevFreeMB = series[0]
	}
	if series, ok := rawData["ple_seconds_series"].([]float64); ok && len(series) > 1 {
		prevPLE = series[0]
	}

	if snaps, ok := rawData["risk_history_snapshots"].([]map[string]interface{}); ok && len(snaps) > 1 {
		first := snaps[0]
		if v, ok2 := first["blocking_sessions"].(float64); ok2 {
			prevBlock = v
		}
		if v, ok2 := first["tempdb_used_percent"].(float64); ok2 {
			prevTempDB = v
		}
		if v, ok2 := first["ple"].(float64); ok2 && prevPLE == 0 {
			prevPLE = v
		}
	}

	type compRow struct {
		metric         string
		current        string
		previous       string
		currentVal     float64
		prevVal        float64
		higherIsBetter bool
	}

	rows := []compRow{
		{"Avg CPU", fmt.Sprintf("%.1f%%", currentCPU), fmt.Sprintf("%.1f%%", prevCPU), currentCPU, prevCPU, false},
		{"Page Life Expectancy", fmt.Sprintf("%.0fs", currentPLE), fmt.Sprintf("%.0fs", prevPLE), currentPLE, prevPLE, true},
		{"Free Disk", fmt.Sprintf("%.1f GB", currentFreeMB/1024), fmt.Sprintf("%.1f GB", prevFreeMB/1024), currentFreeMB, prevFreeMB, true},
		{"Blocking Sessions", fmt.Sprintf("%.0f", currentBlock), fmt.Sprintf("%.0f", prevBlock), currentBlock, prevBlock, false},
		{"TempDB Usage", fmt.Sprintf("%.1f%%", currentTempDB), fmt.Sprintf("%.1f%%", prevTempDB), currentTempDB, prevTempDB, false},
	}

	var result []map[string]interface{}
	for _, r := range rows {
		if r.currentVal == 0 && r.prevVal == 0 {
			continue
		}
		var deltaStr, color, status string
		if r.prevVal == 0 {
			deltaStr = "No prior data"
			color = "var(--text2)"
			status = "N/A"
		} else {
			diff := r.currentVal - r.prevVal
			pct := diff / r.prevVal * 100
			arrow := "→"
			if diff > 0.01 {
				arrow = "↑"
			} else if diff < -0.01 {
				arrow = "↓"
			}
			deltaStr = fmt.Sprintf("%.1f%% %s", math.Abs(pct), arrow)
			improved := r.higherIsBetter == (r.currentVal > r.prevVal)
			if math.Abs(pct) < 2 {
				color = "var(--text2)"
				status = "Stable"
			} else if improved {
				color = "var(--green)"
				status = "Improved"
			} else {
				color = "var(--red)"
				status = "Worsened"
			}
		}
		result = append(result, map[string]interface{}{
			"metric":        r.metric,
			"previous":      r.previous,
			"current":       r.current,
			"current_color": color,
			"delta":         deltaStr,
			"delta_color":   color,
			"status":        status,
			"status_color":  color,
		})
	}
	return result
}

// buildTemplateConfidenceDiagnostics computes the five confidence sub-factors from
// actual data coverage and series variance rather than using hardcoded values.
func buildTemplateConfidenceDiagnostics(triggered []models.RuleTriggerResult, rawData map[string]interface{}, populated int) map[string]interface{} {
	// Data completeness: 20 tracked keys, each = 5%
	dataCompleteness := math.Min(float64(populated)*5, 100)

	// Trend stability: inverse of CPU series standard deviation
	trendStability := 75.0
	if series, ok := rawData["avg_cpu_load_series"].([]float64); ok && len(series) >= 5 {
		mean := 0.0
		for _, v := range series {
			mean += v
		}
		mean /= float64(len(series))
		variance := 0.0
		for _, v := range series {
			d := v - mean
			variance += d * d
		}
		variance /= float64(len(series))
		stdDev := math.Sqrt(variance)
		switch {
		case stdDev > 20:
			trendStability = 50
		case stdDev > 10:
			trendStability = 65
		case stdDev > 5:
			trendStability = 80
		default:
			trendStability = 92
		}
	}

	// Historical coverage: how many 7-day snapshots are available
	historicalCoverage := 30.0
	if snaps, ok := rawData["risk_history_snapshots"].([]map[string]interface{}); ok {
		n := float64(len(snaps))
		switch {
		case n >= 500:
			historicalCoverage = 90
		case n >= 200:
			historicalCoverage = 75
		case n >= 50:
			historicalCoverage = 60
		case n > 0:
			historicalCoverage = 40
		}
	}

	// Rule correlation: number of distinct firing domains increases confidence
	domains := map[string]bool{}
	for _, r := range triggered {
		switch {
		case strings.Contains(r.RuleName, "cpu") || strings.Contains(r.RuleName, "scheduler"):
			domains["performance"] = true
		case strings.Contains(r.RuleName, "ple") || strings.Contains(r.RuleName, "memory"):
			domains["memory"] = true
		case strings.Contains(r.RuleName, "disk") || strings.Contains(r.RuleName, "growth"):
			domains["capacity"] = true
		case strings.Contains(r.RuleName, "blocking") || strings.Contains(r.RuleName, "deadlock"):
			domains["query"] = true
		case strings.Contains(r.RuleName, "replication"):
			domains["replication"] = true
		}
	}
	ruleCorrelation := math.Min(50+float64(len(domains))*8, 90)
	if len(triggered) == 0 {
		ruleCorrelation = 70
	}

	// Metric coverage: forecast-ready series (need ≥3 points)
	seriesKeys := []string{"avg_cpu_load_series", "free_disk_mb_series", "ple_seconds_series", "batch_requests_per_sec_series", "blocking_sessions_series"}
	ready := 0
	for _, k := range seriesKeys {
		if s, ok := rawData[k].([]float64); ok && len(s) >= 3 {
			ready++
		}
	}
	metricCoverage := float64(ready) * 20

	return map[string]interface{}{
		"data_completeness":   dataCompleteness,
		"trend_stability":     trendStability,
		"historical_coverage": historicalCoverage,
		"rule_correlation":    ruleCorrelation,
		"metric_coverage":     metricCoverage,
		"missing_sources":     buildTemplateMissingTelemetry(rawData),
	}
}

// RuleDrillRoute maps a fired rule to a SQL Optima SPA route for drill-down.
func RuleDrillRoute(ruleName string) string {
	routes := map[string]string{
		"cpu_saturation":               "sqlserver-health-v2",
		"cpu_burst":                    "sqlserver-health-v2",
		"scheduler_starvation":         "sqlserver-health-v2",
		"ple_collapse":                 "drilldown-memory",
		"memory_grant_pressure":        "drilldown-memory",
		"low_disk_space":               "sqlserver-health-v2",
		"rapid_disk_growth":            "sqlserver-health-v2",
		"io_latency_high":              "sqlserver-health-v2",
		"blocking_chains":              "sqlserver-locks",
		"replication_lag":              "sqlserver-ha-dashboard",
		"ha_replication_lag":           "sqlserver-ha-dashboard",
		"query_store_regressions":      "query-analysis",
		"query_plan_instability":       "query-analysis",
		"tempdb_pressure":              "sqlserver-health-v2",
		"backup_failure_risk":          "sqlserver-best-practices",
		"maxdop_misconfiguration":      "sqlserver-settings",
	}
	if r, ok := routes[ruleName]; ok {
		return r
	}
	return "sqlserver-health-v2"
}

func buildTemplateMissingTelemetry(raw map[string]interface{}) []map[string]string {
	type src struct {
		key, label string
	}
	sources := []src{
		{"avg_cpu_load_series", "CPU time series (sqlserver_metrics)"},
		{"ple_seconds_series", "PLE history (sqlserver_memory_metrics)"},
		{"blocking_sessions_series", "Blocking history (sqlserver_risk_health)"},
		{"free_disk_mb_series", "Disk free space history"},
		{"batch_requests_per_sec_series", "Batch requests history"},
		{"read_latency_ms_series", "Read latency history (sqlserver_database_throughput)"},
		{"wait_category_breakdown", "Wait stats (sqlserver_wait_stats)"},
		{"ha_replica_states", "Always On replica state (monitor.sqlserver_ha_replica_state)"},
		{"risk_history_snapshots", "Intel risk snapshots (intelreport.intel_snapshots)"},
		{"performance_debt_count", "Performance debt findings"},
	}
	var missing []map[string]string
	for _, s := range sources {
		if !rawHasTelemetry(raw, s.key) {
			missing = append(missing, map[string]string{"key": s.key, "label": s.label})
		}
	}
	return missing
}

func rawHasTelemetry(raw map[string]interface{}, key string) bool {
	v, ok := raw[key]
	if !ok || v == nil {
		return false
	}
	switch val := v.(type) {
	case []float64:
		return len(val) > 0
	case []map[string]interface{}:
		return len(val) > 0
	case []interface{}:
		return len(val) > 0
	case float64:
		return true
	case bool:
		return true
	case string:
		return val != ""
	default:
		return true
	}
}

// buildTemplateTop3Actions surfaces the highest-priority remediation steps for executives.
func buildTemplateTop3Actions(recs []map[string]string, rules []models.RuleTriggerResult) []map[string]string {
	var out []map[string]string
	seen := map[string]bool{}
	add := func(title, urgency, route, reason string) {
		if title == "" || seen[title] || len(out) >= 3 {
			return
		}
		seen[title] = true
		out = append(out, map[string]string{
			"title":   title,
			"urgency": urgency,
			"route":   route,
			"reason":  reason,
		})
	}
	for _, r := range recs {
		if r["urgency"] != "immediate" {
			continue
		}
		add(r["action"], r["urgency"], "sqlserver-intelligence-report", r["reason"])
	}
	sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2}
	sorted := make([]models.RuleTriggerResult, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sevOrder[sorted[i].Severity] < sevOrder[sorted[j].Severity]
	})
	for _, rule := range sorted {
		if rule.Severity != "critical" && rule.Severity != "high" {
			continue
		}
		title := cases.Title(language.Und).String(strings.ReplaceAll(rule.RuleName, "_", " "))
		reason := rule.Message
		if len(reason) > 120 {
			reason = reason[:117] + "..."
		}
		if len(rule.Recommendation) > 0 {
			add(rule.Recommendation[0], rule.Severity, RuleDrillRoute(rule.RuleName), reason)
		} else {
			add("Investigate "+title, rule.Severity, RuleDrillRoute(rule.RuleName), reason)
		}
	}
	for _, r := range recs {
		if len(out) >= 3 {
			break
		}
		if r["urgency"] == "immediate" {
			continue
		}
		add(r["action"], r["urgency"], RuleDrillRoute(""), r["reason"])
	}
	return out
}

// buildTemplateRootCauseProbabilities maps the top triggered rules to probability
// estimates derived from actual metric values, not static per-severity constants.
func buildTemplateRootCauseProbabilities(rules []models.RuleTriggerResult) []map[string]interface{} {
	// Base probabilities anchored to severity, then adjusted by metric signal strength.
	baseProbFor := map[string]float64{"critical": 80, "high": 60, "medium": 38, "low": 18}
	colorFor := map[string]string{"critical": "var(--red)", "high": "var(--orange)", "medium": "var(--yellow)", "low": "var(--green)"}

	// Count how many distinct domains are firing — corroborating evidence increases probability.
	domainHits := map[string]int{}
	for _, r := range rules {
		switch {
		case strings.Contains(r.RuleName, "cpu") || strings.Contains(r.RuleName, "scheduler"):
			domainHits["performance"]++
		case strings.Contains(r.RuleName, "ple") || strings.Contains(r.RuleName, "memory") || strings.Contains(r.RuleName, "grant"):
			domainHits["memory"]++
		case strings.Contains(r.RuleName, "disk") || strings.Contains(r.RuleName, "growth"):
			domainHits["capacity"]++
		case strings.Contains(r.RuleName, "block") || strings.Contains(r.RuleName, "deadlock"):
			domainHits["query"]++
		case strings.Contains(r.RuleName, "replication") || strings.Contains(r.RuleName, "replica"):
			domainHits["replication"]++
		case strings.Contains(r.RuleName, "tempdb"):
			domainHits["tempdb"]++
		}
	}
	totalDomains := float64(len(domainHits))

	seen := map[string]bool{}
	var result []map[string]interface{}
	for _, r := range rules {
		if seen[r.RuleName] {
			continue
		}
		seen[r.RuleName] = true

		prob := baseProbFor[r.Severity]

		// Boost probability based on how many metric_values exceed normal ranges.
		highSignals := 0
		for _, v := range r.MetricValues {
			if v > 0 {
				highSignals++
			}
		}
		if highSignals >= 3 {
			prob += 10
		} else if highSignals >= 1 {
			prob += 5
		}

		// Boost if multiple corroborating domains also triggered.
		if totalDomains >= 3 {
			prob += 8
		} else if totalDomains >= 2 {
			prob += 4
		}

		if prob > 96 {
			prob = 96
		}

		// Build a compact evidence string from the rule's metric values.
		evidenceParts := []string{}
		for k, v := range r.MetricValues {
			if v > 0 && len(evidenceParts) < 3 {
				label := cases.Title(language.Und).String(strings.ReplaceAll(k, "_", " "))
				evidenceParts = append(evidenceParts, fmt.Sprintf("%s=%.1f", label, v))
			}
		}
		evidenceStr := r.Message
		if len(evidenceParts) > 0 {
			evidenceStr = strings.Join(evidenceParts, "; ")
		}
		if len(evidenceStr) > 70 {
			evidenceStr = evidenceStr[:67] + "..."
		}

		candidate := cases.Title(language.Und).String(strings.ReplaceAll(r.RuleName, "_", " "))
		result = append(result, map[string]interface{}{
			"candidate":   candidate,
			"probability": prob,
			"prob_color":  colorFor[r.Severity],
			"evidence":    evidenceStr,
			"signals":     highSignals,
		})
		if len(result) >= 6 {
			break
		}
	}
	return result
}
