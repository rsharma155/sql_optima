// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package analysis


import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/intel/forecasting"
	"github.com/rsharma155/sql_optima/internal/intel/anomaly"
	"github.com/rsharma155/sql_optima/internal/intel/recommendations"
	"github.com/rsharma155/sql_optima/internal/intel/risk"
	"github.com/rsharma155/sql_optima/internal/intel/rule_engine"
	"github.com/rsharma155/sql_optima/internal/models"
)


// dynamicOnlyRuleNames are evaluated exclusively in evaluateDynamicRules with
// hardware- and history-tuned thresholds. YAML copies are ignored even if reintroduced.
var dynamicOnlyRuleNames = map[string]bool{
	"cpu_saturation":              true,
	"scheduler_starvation":        true,
	"ple_collapse":                true,
	"memory_grant_pressure":       true,
	"buffer_cache_hit_ratio_low":  true,
	"low_disk_space":              true,
	"rapid_disk_growth":           true,
	"io_latency_high":             true,
	"replication_lag":             true,
	"ha_replication_lag":          true,
	"ha_replication_lag_critical": true,
	"blocking_chains":             true,
	"tempdb_pressure":             true,
	"query_spills":                true,
	"backup_failure_risk":         true,
	"storage_exhaustion":          true,
	"database_growth_acceleration": true,
	"cpu_burst":                   true,
}

type AnalysisEngine struct {
	thresholdCalc     *DynamicThresholdCalculator
	riskScorer        *risk.RiskScorer
	recommendationGen *recommendations.RecommendationGenerator
	anomalyDetector   *anomaly.AnomalyDetector
	rules             []rule_engine.CompiledRule
}

func NewAnalysisEngine() *AnalysisEngine {
	rules, _ := rule_engine.LoadRulePacks()
	return &AnalysisEngine{
		thresholdCalc:     NewDynamicThresholdCalculator(),
		riskScorer:        risk.NewRiskScorer(),
		recommendationGen: recommendations.NewRecommendationGenerator(),
		anomalyDetector:   anomaly.NewAnomalyDetector(2.5),
		rules:             rules,
	}
}

func (e *AnalysisEngine) Analyze(rawData map[string]interface{}, serverConfig *models.ServerConfig, runID string) *models.IntelligenceReportResponse {
	if runID == "" {
		runID = uuid.New().String()
	}
	now := time.Now().UTC()

	if serverConfig == nil {
		cfg := DefaultServerConfig(rawData)
		serverConfig = &cfg
	}

	histories := ExtractHistoriesFromRaw(rawData)

	thresholds := e.thresholdCalc.Compute(*serverConfig, histories)

	triggeredRules := e.evaluateDynamicRules(rawData, thresholds, *serverConfig, histories)

	// 1. Evaluate YAML rules (ARCH-1, DEFECT-7 fix)
	// Design boundary: YAML packs must only contain binary/configuration checks (e.g.
	// "buffer_cache_hit_ratio < 90") that have no dynamic equivalent. Threshold-sensitive
	// rules for metrics already covered by evaluateDynamicRules (CPU, memory, disk, replication)
	// must NOT appear in YAML packs — they fire with different rule names and bypass the
	// name-based dedup below, producing duplicate CurrentIssues entries with conflicting severities.
	//
	// Dedup by rule name: if a dynamic rule with the same name already fired, the YAML rule
	// is skipped. Dynamic rules take precedence because they use hardware-tuned thresholds.
	// dynamicOnlyRuleNames lists rules that must NEVER be evaluated from YAML packs (fixed
	// thresholds like PLE < 300 were removed — see packs/*.yaml headers).
	dynamicNames := make(map[string]bool, len(triggeredRules))
	for _, r := range triggeredRules {
		dynamicNames[r.RuleName] = true
	}
	yamlRules := rule_engine.EvaluateRulesFromRaw(e.rules, rawData)
	for _, yr := range yamlRules {
		if dynamicOnlyRuleNames[yr.RuleName] {
			continue
		}
		if !dynamicNames[yr.RuleName] {
			triggeredRules = append(triggeredRules, yr)
		}
	}

	// 2. Feature-aware configuration checks (ARCH-3)
	// Uses hardware fields now fetched from sqlserver_server_properties to evaluate
	// configuration best practices (MAXDOP, max server memory, worker threads).
	configChecks := e.evaluateConfigChecks(rawData, thresholds, *serverConfig)
	triggeredRules = append(triggeredRules, configChecks...)

	// 2b. Feature library checks (DEFECT-15 Phase A)
	// Wires metrics already present in rawData to thresholds defined in SQLServerFeatures.
	// Adds net-new coverage for: AG long-running transactions, available memory quantitative
	// threshold, CXPACKET parallelism pressure, and disabled critical SQL Agent jobs.
	featureChecks := e.evaluateFeatureLibraryChecks(rawData)
	triggeredRules = append(triggeredRules, featureChecks...)

	// 3. Run Anomaly Detection (ARCH-4)
	anomalies := e.runAnomalyDetection(rawData, histories)
	triggeredRules = append(triggeredRules, anomalies...)

	ruleDicts := make([]map[string]string, len(triggeredRules))
	for i, r := range triggeredRules {
		ruleDicts[i] = map[string]string{
			"rule_name": r.RuleName,
			"severity":  r.Severity,
			"name":      r.RuleName,
		}
	}

	// Calculate base risk dimensions from actual metrics (DEFECT-1)
	perfRisk := computeBasePerformanceRisk(rawData, &thresholds)
	capRisk := computeBaseCapacityRisk(rawData, &thresholds)
	availRisk := computeBaseAvailabilityRisk(rawData, &thresholds)
	replRisk := computeBaseReplicationRisk(rawData, &thresholds)
	maintRisk := computeBaseMaintenanceRisk(rawData, &thresholds)
	queryRisk := computeBaseQueryRisk(rawData, &thresholds)

	riskResult := e.riskScorer.Compute(perfRisk, capRisk, availRisk, replRisk, maintRisk, queryRisk, ruleDicts)

	// Calculate 7-day Risk Trend from historical snapshots (DEFECT-3)
	riskTrend := e.calculateRiskTrend(rawData, &thresholds)

	forecasts := e.generateForecasts(rawData, histories)

	failureForecast := e.buildFailureForecast(forecasts)

	recs := e.recommendationGen.Generate(triggeredRules, rawData, thresholds, *serverConfig)

	workingWell := e.buildWorkingWell(rawData, thresholds, triggeredRules)

	var whatCouldGoWrong []string
	for _, t := range triggeredRules {
		if t.Severity == "critical" || t.Severity == "high" {
			whatCouldGoWrong = append(whatCouldGoWrong, t.Message)
		}
	}

	rootCauses := e.buildRootCauses(triggeredRules, rawData)

	narrative := BuildNarrativeSummary(riskResult, triggeredRules, forecasts)

	var currentIssues []map[string]interface{}
	for _, t := range triggeredRules {
		currentIssues = append(currentIssues, map[string]interface{}{
			"title":          t.RuleName,
			"description":    t.Message,
			"severity":       t.Severity,
			"recommendation": t.Recommendation,
			"metric_values":  t.MetricValues,
		})
	}

	var recommendedActions []map[string]string
	recommendedActions = append(recommendedActions, recs...)

	return &models.IntelligenceReportResponse{
		RunID:               runID,
		OverallRisk:         riskResult,
		RiskTrend:           riskTrend,
		WorkingWell:         workingWell,
		CurrentIssues:       currentIssues,
		WhatCouldGoWrong:    whatCouldGoWrong,
		FailureForecast:     failureForecast,
		RootCauseHypotheses: rootCauses,
		RecommendedActions:  recommendedActions,
		TriggeredRules:      triggeredRules,
		Forecasts:           forecasts,
		ConfidenceScore:     riskResult.Confidence,
		NarrativeSummary:    narrative,
		GeneratedAt:         now.Format(time.RFC3339),
	}
}

// evaluateConfigChecks fires rules that require hardware context from server_properties —
// primarily MAXDOP, max server memory headroom, worker thread capacity, and AG queue thresholds.
// These cannot be expressed as simple YAML rules because their thresholds are computed from
// the actual hardware configuration fetched at report time.
func (e *AnalysisEngine) evaluateConfigChecks(raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []models.RuleTriggerResult {
	var triggered []models.RuleTriggerResult

	// Only run config checks when we have real hardware data (not defaults).
	// cpu_count > 8 is a conservative proxy for "we got real server_properties data"
	// since the default is 8, and most real servers differ from it.
	haveHardwareData := hasAnyKey(raw, "socket_count", "numa_nodes", "max_workers_count")
	if !haveHardwareData {
		return triggered
	}

	optimalMAXDOP := computeOptimalMAXDOP(config)

	// MAXDOP from telemetry (performance_debt Engine Config details) when available.
	if maxdop := getFloat(raw, "max_degree_of_parallelism", "", -1); maxdop >= 0 && config.CPUCount > 8 {
		switch {
		case maxdop == 0:
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "maxdop_misconfiguration",
				Severity: "high",
				Message: fmt.Sprintf(
					"MAXDOP is 0 (unrestricted) on a %d-core server. Recommended MAXDOP: %d.",
					config.CPUCount, optimalMAXDOP),
				MetricValues: map[string]float64{
					"max_degree_of_parallelism": maxdop,
					"optimal_maxdop":            float64(optimalMAXDOP),
				},
				Recommendation: []string{
					fmt.Sprintf("EXEC sp_configure 'max degree of parallelism', %d; RECONFIGURE", optimalMAXDOP),
				},
			})
		case maxdop > float64(optimalMAXDOP) && maxdop > 8:
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "maxdop_above_recommended",
				Severity: "medium",
				Message: fmt.Sprintf(
					"MAXDOP is %.0f on a %d-core server (recommended ≤%d for this NUMA layout).",
					maxdop, config.CPUCount, optimalMAXDOP),
				MetricValues: map[string]float64{
					"max_degree_of_parallelism": maxdop,
					"optimal_maxdop":            float64(optimalMAXDOP),
				},
			})
		}
	}

	// Infer MAXDOP=0 from worker exhaustion when setting is not in telemetry.
	if getBool(raw, "worker_thread_exhaustion_warning") && config.CPUCount > 8 &&
		getFloat(raw, "max_degree_of_parallelism", "", -1) < 0 {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "maxdop_misconfiguration",
			Severity: "high",
			Message: fmt.Sprintf(
				"Worker thread exhaustion on %d-core server suggests MAXDOP=0 (unrestricted). "+
					"Recommended MAXDOP for this NUMA topology (%d nodes, %d cores/node): %d.",
				config.CPUCount, config.NumaNodes, config.CoresPerSocket, optimalMAXDOP),
			MetricValues: map[string]float64{
				"cpu_count":       float64(config.CPUCount),
				"numa_nodes":      float64(config.NumaNodes),
				"cores_per_socket": float64(config.CoresPerSocket),
				"optimal_maxdop":  float64(optimalMAXDOP),
			},
			Recommendation: []string{
				fmt.Sprintf("Set MAXDOP to %d via sp_configure (EXEC sp_configure 'max degree of parallelism', %d; RECONFIGURE)", optimalMAXDOP, optimalMAXDOP),
				"Review CXPACKET / CXCONSUMER waits for parallel plan contention",
				"Raise cost threshold for parallelism to 25–50 to reduce unnecessary parallel plans",
			},
		})
	}

	// Max Server Memory headroom check.
	// Formula: Leave OS headroom of 4GB for <=64GB RAM, 8GB for 64–256GB, 16GB for >256GB.
	totalRAMGB := config.TotalRAMGB
	if totalRAMGB > 0 {
		osHeadroomGB := computeOSHeadroomGB(totalRAMGB)
		maxSQLMemGB := float64(totalRAMGB) - float64(osHeadroomGB)

		// If OS pressure was flagged but we have adequate total RAM, the max_server_memory
		// setting is likely too high (SQL is starving the OS).
		if getBool(raw, "physical_memory_pressure_warning") {
			availKB := getFloat(raw, "available_physical_memory_kb", "", 0)
			availGB := availKB / (1024 * 1024)
			if availGB < float64(osHeadroomGB)*0.5 {
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "max_server_memory_too_high",
					Severity: "high",
					Message: fmt.Sprintf(
						"OS memory pressure detected: only %.1f GB available of %d GB total. "+
							"SQL Server max_server_memory should leave %d GB for OS (recommended SQL ceiling: %.0f GB).",
						availGB, totalRAMGB, osHeadroomGB, maxSQLMemGB),
					MetricValues: map[string]float64{
						"available_physical_gb":  availGB,
						"total_ram_gb":           float64(totalRAMGB),
						"os_headroom_gb":         float64(osHeadroomGB),
						"recommended_max_sql_gb": maxSQLMemGB,
					},
					Recommendation: []string{
						fmt.Sprintf("Set max server memory to %.0f GB: EXEC sp_configure 'max server memory (MB)', %.0f; RECONFIGURE", maxSQLMemGB, maxSQLMemGB*1024),
						"Enable Lock Pages In Memory (LPIM) to prevent OS from paging SQL buffer pool",
						"Monitor available_physical_memory_kb trend before and after the change",
					},
				})
			}
		}
	}

	// Worker thread capacity check: compare actual max_workers_count against the
	// SQL Server formula to detect manual misconfiguration.
	if config.MaxWorkers > 0 && config.CPUCount > 0 {
		formulaMax := computeFormulaMaxWorkers(config.CPUCount)
		// Alert if manually configured below the formula-derived value (under-provisioned)
		// or more than 3× above it (over-provisioned, wastes memory and masks exhaustion).
		if config.MaxWorkers < formulaMax {
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "worker_threads_under_provisioned",
				Severity: "medium",
				Message: fmt.Sprintf(
					"max_workers_count=%d is below the SQL Server formula-derived value of %d for %d cores. "+
						"Under-provisioning accelerates thread exhaustion under burst load.",
					config.MaxWorkers, formulaMax, config.CPUCount),
				MetricValues: map[string]float64{
					"max_workers_count": float64(config.MaxWorkers),
					"formula_max":       float64(formulaMax),
					"cpu_count":         float64(config.CPUCount),
				},
				Recommendation: []string{
					fmt.Sprintf("Set max worker threads to %d or 0 (auto): EXEC sp_configure 'max worker threads', %d; RECONFIGURE", formulaMax, formulaMax),
				},
			})
		}
	}

	// HA / Always On queue thresholds using real data already collected.
	// These use the feature library thresholds from sqlserver_features.go for the first time.
	logSendQ := getFloat(raw, "log_send_queue_kb", "", 0)
	redoQ := getFloat(raw, "redo_queue_kb", "", 0)
	if logSendQ > 0 || redoQ > 0 {
		if logSendQ > 200000 {
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "ag_log_send_queue_critical",
				Severity: "critical",
				Message: fmt.Sprintf(
					"Always On log send queue at %.0f KB (critical threshold: 200,000 KB). "+
						"Secondary is falling dangerously behind. RPO is at risk.",
					logSendQ),
				MetricValues: map[string]float64{"log_send_queue_kb": logSendQ},
				Recommendation: []string{
					"Check network bandwidth between primary and secondary",
					"Review secondary I/O pressure (may be slow on redo thread)",
					"Evaluate whether synchronous commit mode is appropriate for this network",
				},
			})
		} else if logSendQ > 50000 {
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "ag_log_send_queue_warning",
				Severity: "medium",
				Message:  fmt.Sprintf("Always On log send queue at %.0f KB (warning threshold: 50,000 KB). Monitor for growth.", logSendQ),
				MetricValues: map[string]float64{"log_send_queue_kb": logSendQ},
				Recommendation: []string{"Monitor log send queue trend", "Check for long-running transactions blocking log truncation"},
			})
		}

		if redoQ > 200000 {
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "ag_redo_queue_critical",
				Severity: "critical",
				Message: fmt.Sprintf(
					"Always On redo queue at %.0f KB (critical threshold: 200,000 KB). "+
						"Secondary I/O cannot keep up. Failover to this replica will require long recovery time.",
					redoQ),
				MetricValues: map[string]float64{"redo_queue_kb": redoQ},
				Recommendation: []string{
					"Move secondary database files to faster storage",
					"Check for lock contention on secondary (readable secondary blocking redo)",
					"Consider switching to asynchronous commit mode temporarily to reduce primary impact",
				},
			})
		}
	}

	return triggered
}

// computeOptimalMAXDOP returns the Microsoft-recommended MAXDOP value for the given
// server configuration. Logic follows the official guidance:
// - NUMA with cores_per_node ≤ 8: use cores_per_NUMA_node
// - NUMA with cores_per_node > 8: cap at 8
// - No NUMA / single socket ≤ 8 cores: use all cores
// - No NUMA / single socket > 8 cores: cap at 8
func computeOptimalMAXDOP(config models.ServerConfig) int {
	if config.CPUCount <= 0 {
		return 1
	}
	numaNodes := config.NumaNodes
	if numaNodes <= 0 {
		numaNodes = 1
	}
	coresPerNode := config.CPUCount / numaNodes
	// Account for hyperthreading: physical cores matter for MAXDOP, not logical
	if config.HyperthreadRatio > 1 && config.CoresPerSocket > 0 {
		physicalCoresPerNode := config.CoresPerSocket / numaNodes
		if physicalCoresPerNode > 0 {
			coresPerNode = physicalCoresPerNode
		}
	}
	if coresPerNode > 8 {
		return 8
	}
	return coresPerNode
}

// computeFormulaMaxWorkers returns the SQL Server formula-derived max worker threads.
// Source: https://docs.microsoft.com/en-us/sql/database-engine/configure-windows/configure-the-max-worker-threads-server-configuration-option
// Formula: 512 + (logical_cores - 4) × 16 for cores > 4; 512 for cores ≤ 4.
func computeFormulaMaxWorkers(cpuCount int) int {
	if cpuCount <= 4 {
		return 512
	}
	return 512 + (cpuCount-4)*16
}

// computeOSHeadroomGB returns the recommended OS memory reservation in GB.
// Follows Microsoft guidance: 4 GB for < 64 GB, 8 GB for 64–256 GB, 16 GB for > 256 GB.
func computeOSHeadroomGB(totalRAMGB int) int {
	switch {
	case totalRAMGB >= 256:
		return 16
	case totalRAMGB >= 64:
		return 8
	default:
		return 4
	}
}

func (e *AnalysisEngine) runAnomalyDetection(raw map[string]interface{}, histories map[string][]float64) []models.RuleTriggerResult {
	var triggered []models.RuleTriggerResult

	targets := []struct {
		key   string
		label string
	}{
		{"avg_cpu_load", "CPU Load"},
		{"ple_seconds", "Page Life Expectancy"},
		{"free_disk_mb", "Free Disk Space"},
		{"batch_requests_per_sec", "Batch Requests"},
		{"tempdb_used_percent", "TempDB Usage"},
	}

	for _, t := range targets {
		series := histories[t.key]
		if len(series) < 10 {
			continue
		}

		current := series[len(series)-1]
		isAnomaly, score := e.anomalyDetector.DetectPointAnomaly(current, series[:len(series)-1])
		if isAnomaly {
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "anomaly_detected",
				Severity: "medium",
				Message:  fmt.Sprintf("Statistical anomaly detected in %s: current value %.1f is %.1f standard deviations from mean.", t.label, current, score),
				MetricValues: map[string]float64{
					"current_value": current,
					"z_score":       score,
				},
				Recommendation: []string{"Investigate sudden change in workload pattern", "Check for correlated events in timeline"},
			})
		}

		if e.anomalyDetector.DetectTrendAnomaly(series) {
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "trend_shift",
				Severity: "low",
				Message:  fmt.Sprintf("Significant trend shift detected in %s over the last 60 points.", t.label),
				MetricValues: map[string]float64{
					"current_value": current,
				},
				Recommendation: []string{"Monitor for continued acceleration or baseline shift"},
			})
		}
	}

	return triggered
}

func computeBasePerformanceRisk(raw map[string]interface{}, t *models.DynamicThresholds) float64 {
	cpu := getFloat(raw, "avg_cpu_load", "", 0)
	cpuRisk := (cpu / t.CPUMaxThreshold) * 60

	mem := getFloat(raw, "memory_usage", "", 0)
	memRisk := (mem / t.MemoryUsedPctMax) * 40

	return clamp(cpuRisk+memRisk, 0, 100)
}

func computeBaseCapacityRisk(raw map[string]interface{}, t *models.DynamicThresholds) float64 {
	// DEFECT-2 fix: when disk metrics are absent, return a data-gap floor of 10 (uncertain)
	// rather than 0 (safe). Missing disk telemetry ≠ healthy disk.
	if !hasAnyKey(raw, "total_free_mb", "free_disk_mb") {
		return 10.0
	}
	free := getFloat(raw, "total_free_mb", "free_disk_mb", 0)
	dataMB := getFloat(raw, "total_data_mb", "data_disk_mb", 0)
	logMB := getFloat(raw, "total_log_mb", "log_disk_mb", 0)
	total := dataMB + logMB + free
	if total <= 0 {
		return 0
	}
	usedPct := (total - free) / total
	// Scale risk linearly: 0% used → 0 risk, 100% used → 100 risk.
	// The old formula (usedPct*80) capped at 80 even when disk was completely full,
	// which systematically under-reported capacity emergencies.
	return clamp(usedPct*100, 0, 100)
}

func hasAnyKey(raw map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		if _, ok := raw[k]; ok {
			return true
		}
	}
	return false
}

func computeBaseAvailabilityRisk(raw map[string]interface{}, t *models.DynamicThresholds) float64 {
	ioLat := math.Max(getFloat(raw, "read_latency_ms", "", 0), getFloat(raw, "write_latency_ms", "", 0))
	ioRisk := (ioLat / t.IOLatencyMaxMS) * 50
	return clamp(ioRisk, 0, 100)
}

func computeBaseReplicationRisk(raw map[string]interface{}, t *models.DynamicThresholds) float64 {
	if v, ok := raw["ha_not_configured"].(bool); ok && v {
		return 0
	}
	if _, ok := raw["ha_replica_states"]; !ok && !hasAnyKey(raw, "secondary_lag_seconds") {
		return 0
	}
	lag := getFloat(raw, "secondary_lag_seconds", "", 0)
	if lag == 0 {
		return 0
	}
	return clamp((lag/t.ReplicationLagMaxSeconds)*70, 0, 100)
}

func computeBaseMaintenanceRisk(raw map[string]interface{}, t *models.DynamicThresholds) float64 {
	failedJobs := getFloat(raw, "failed_jobs_24h", "", 0)
	return clamp(failedJobs*20, 0, 100)
}

func computeBaseQueryRisk(raw map[string]interface{}, t *models.DynamicThresholds) float64 {
	blocking := getFloat(raw, "blocking_sessions", "", 0)
	return clamp(blocking*20, 0, 100)
}

func (e *AnalysisEngine) calculateRiskTrend(raw map[string]interface{}, t *models.DynamicThresholds) []models.RiskScoreResult {
	snapshots, ok := raw["risk_history_snapshots"].([]map[string]interface{})
	if !ok || len(snapshots) == 0 {
		return nil
	}

	var trend []models.RiskScoreResult
	for _, snap := range snapshots {
		// Recompute risk for this point in time
		p := computeBasePerformanceRisk(snap, t)
		c := computeBaseCapacityRisk(snap, t)
		a := computeBaseAvailabilityRisk(snap, t)
		r := computeBaseReplicationRisk(snap, t)
		m := computeBaseMaintenanceRisk(snap, t)
		q := computeBaseQueryRisk(snap, t)

		// Note: we don't have historical rules easily available here, 
		// so it's a base metric trend which is still infinitely better than fake data.
		trend = append(trend, e.riskScorer.Compute(p, c, a, r, m, q, nil))
	}
	return trend
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func BuildNarrativeSummary(riskResult models.RiskScoreResult, triggeredRules []models.RuleTriggerResult, forecasts []models.ForecastResult) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Environment health is %s with a risk score of %.1f out of 100.", strings.ToLower(riskResult.Category), riskResult.OverallScore))

	if len(triggeredRules) > 0 {
		critical := 0
		high := 0
		for _, r := range triggeredRules {
			if r.Severity == "critical" {
				critical++
			}
			if r.Severity == "high" {
				high++
			}
		}
		if critical > 0 {
			parts = append(parts, fmt.Sprintf("%d critical and %d high-severity issues require immediate attention.", critical, high))
		} else if high > 0 {
			parts = append(parts, fmt.Sprintf("%d high-severity issues detected that should be addressed.", high))
		}
	}

	for _, f := range forecasts {
		if f.PredictedFailureDays != nil {
			parts = append(parts, fmt.Sprintf("%s projected to reach critical levels in approx. %d days (%.0f%% confidence).", f.MetricName, *f.PredictedFailureDays, f.Confidence*100))
		}
	}

	if len(parts) <= 1 {
		parts = append(parts, "All monitored metrics are within dynamically computed thresholds.")
	}

	return strings.Join(parts, " ")
}

func (e *AnalysisEngine) evaluateDynamicRules(raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig, histories map[string][]float64) []models.RuleTriggerResult {
	var triggered []models.RuleTriggerResult

	cpuLoad := getFloat(raw, "avg_cpu_load", "sql_process", 0)
	runnable := getFloat(raw, "total_runnable_tasks_count", "avg_runnable_tasks_count", 0)
	ple := getFloat(raw, "ple_seconds", "ple", 500)
	grants := getFloat(raw, "memory_grants_pending", "waiting_memory_grants", 0)
	memPct := getFloat(raw, "memory_usage", "", 0)
	// Use -1 as sentinel so disk rules are skipped entirely when no disk data was collected.
	// The old default of 50000 MB was just below the 51200 MB default threshold, always
	// triggering low_disk_space for every instance with missing data.
	freeMB := getFloat(raw, "total_free_mb", "free_disk_mb", -1)
	deltaGrowth := getFloat(raw, "delta_data_mb", "delta_log_mb", 0)
	readLat := getFloat(raw, "read_latency_ms", "", 0)
	writeLat := getFloat(raw, "write_latency_ms", "", 0)
	replLag := getFloat(raw, "secondary_lag_seconds", "latency_seconds", 0)
	blocking := getFloat(raw, "blocking_sessions", "", 0)
	tempdb := getFloat(raw, "tempdb_used_percent", "tempdb_used_pct", 30)
	failedJobs := getFloat(raw, "failed_jobs_24h", "", 0)
	sortWarn := getFloat(raw, "sort_warnings_per_sec", "", 0)
	hashWarn := getFloat(raw, "hash_warnings_per_sec", "", 0)
	workerExhaust := getBool(raw, "worker_thread_exhaustion_warning")
	memPressure := getBool(raw, "physical_memory_pressure_warning")
	deadlocks := getFloat(raw, "deadlocks", "", 0)
	logSendQ := getFloat(raw, "log_send_queue_kb", "", 0)
	redoQ := getFloat(raw, "redo_queue_kb", "", 0)
	perfDebt := getFloat(raw, "performance_debt_count", "", 0)

	cpuSeries := histories["avg_cpu_load"]
	if cpuLoad > thresholds.CPUSustainedThreshold {
		runnableVal := runnable
		if runnableVal > float64(thresholds.CPURunnableAbsolute)/2 {
			sev := "medium"
			if cpuLoad > thresholds.CPUMaxThreshold {
				sev = "high"
			}
			trend := cpuTrendDescription(cpuSeries)
			msg := fmt.Sprintf("CPU at %.1f%% (threshold: %.0f%%) with %.0f runnable tasks on %d cores. %s", cpuLoad, thresholds.CPUSustainedThreshold, runnableVal, config.CPUCount, trend)
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "cpu_saturation",
				Severity: sev,
				Message:  msg,
				MetricValues: map[string]float64{
					"avg_cpu_load":   cpuLoad,
					"runnable_tasks": runnableVal,
					"cpu_threshold":  thresholds.CPUSustainedThreshold,
				},
			})
		}
	}

	if runnable > float64(thresholds.CPURunnableAbsolute) {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "scheduler_starvation",
			Severity: "critical",
			Message:  fmt.Sprintf("Scheduler starvation: %.0f runnable tasks exceeds threshold of %d for %d cores. Worker thread exhaustion risk.", runnable, thresholds.CPURunnableAbsolute, config.CPUCount),
			MetricValues: map[string]float64{
				"total_runnable_tasks":   runnable,
				"cpu_runnable_threshold": float64(thresholds.CPURunnableAbsolute),
			},
		})
	}

	if workerExhaust {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "worker_thread_exhaustion",
			Severity: "critical",
			Message:  fmt.Sprintf("Worker thread exhaustion warning flagged. Max workers: %d.", config.MaxWorkers),
			MetricValues: map[string]float64{
				"max_workers": float64(config.MaxWorkers),
				"cpu_count":   float64(config.CPUCount),
			},
		})
	}

	if ple < thresholds.MemoryPLEMinSeconds {
		sev := "high"
		if ple < thresholds.MemoryPLEMinSeconds*0.5 {
			sev = "critical"
		}
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "ple_collapse",
			Severity: sev,
			Message:  fmt.Sprintf("Page Life Expectancy at %.0fs (threshold: %.0fs). Buffer pool under pressure on %dGB server.", ple, thresholds.MemoryPLEMinSeconds, config.TotalRAMGB),
			MetricValues: map[string]float64{
				"ple_seconds":   ple,
				"ple_threshold": thresholds.MemoryPLEMinSeconds,
				"total_ram_gb":  float64(config.TotalRAMGB),
			},
		})
	}

	if grants > float64(thresholds.MemoryGrantsPendingMax) {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "memory_grant_pressure",
			Severity: "high",
			Message:  fmt.Sprintf("%.0f memory grants pending (threshold: %d). Queries competing for workspace memory. %dGB total RAM.", grants, thresholds.MemoryGrantsPendingMax, config.TotalRAMGB),
			MetricValues: map[string]float64{
				"memory_grants_pending": grants,
				"grants_threshold":      float64(thresholds.MemoryGrantsPendingMax),
			},
		})
	}

	hit := getFloat(raw, "buffer_cache_hit_ratio", "", 0)
	if hit > 0 && hit < thresholds.BufferCacheHitRatioMin {
		sev := "high"
		if hit < thresholds.BufferCacheHitRatioMin-5 {
			sev = "critical"
		}
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "buffer_cache_hit_ratio_low",
			Severity: sev,
			Message:  fmt.Sprintf("Buffer cache hit ratio at %.1f%% (dynamic threshold: %.1f%%).", hit, thresholds.BufferCacheHitRatioMin),
			MetricValues: map[string]float64{
				"buffer_cache_hit_ratio": hit,
				"bchr_threshold":         thresholds.BufferCacheHitRatioMin,
			},
			Recommendation: []string{
				"Correlate with PLE and memory grants",
				"Identify large scans or data loads evicting the buffer pool",
			},
		})
	}

	if memPct > thresholds.MemoryUsedPctMax {
		osMem := getFloat(raw, "os_available_memory_mb", "", 0)
		msg := fmt.Sprintf("Memory at %.1f%% (threshold: %.0f%%).", memPct, thresholds.MemoryUsedPctMax)
		if osMem > 0 {
			msg = fmt.Sprintf("Memory at %.1f%% (threshold: %.0f%%). OS available: %.0f MB.", memPct, thresholds.MemoryUsedPctMax, osMem)
		}
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "high_memory_usage",
			Severity: "medium",
			Message:  msg,
			MetricValues: map[string]float64{
				"memory_usage_pct": memPct,
				"mem_threshold":    thresholds.MemoryUsedPctMax,
			},
		})
	}

	if memPressure {
		availKb := getFloat(raw, "available_physical_memory_kb", "", 0)
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "os_memory_pressure",
			Severity: "high",
			Message:  fmt.Sprintf("OS-level physical memory pressure detected. Available: %.0f MB. SQL may be over-allocated.", availKb/1024),
			MetricValues: map[string]float64{
				"available_physical_memory_kb": availKb,
			},
		})
	}

	if freeMB >= 0 && freeMB < thresholds.DiskFreeMBMin {
		sev := "high"
		if freeMB < thresholds.DiskFreeMBMin*0.3 {
			sev = "critical"
		}
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "low_disk_space",
			Severity: sev,
			Message:  fmt.Sprintf("Free disk: %.0f MB (threshold: %.0f MB). Growth rate: %.0f MB/interval.", freeMB, thresholds.DiskFreeMBMin, deltaGrowth),
			MetricValues: map[string]float64{
				"free_disk_mb":    freeMB,
				"disk_threshold":  thresholds.DiskFreeMBMin,
				"delta_growth_mb": deltaGrowth,
			},
		})
	}

	if deltaGrowth > thresholds.DiskGrowthRateMaxMBPerDay/96 {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "rapid_disk_growth",
			Severity: "medium",
			Message:  fmt.Sprintf("Data growth rate: %.0f MB/interval (~%.0f MB/day).", deltaGrowth, deltaGrowth*96),
			MetricValues: map[string]float64{
				"delta_data_mb":          deltaGrowth,
				"growth_threshold_daily": thresholds.DiskGrowthRateMaxMBPerDay,
			},
		})
	}

	ioVal := readLat
	if writeLat > ioVal {
		ioVal = writeLat
	}
	if ioVal > thresholds.IOLatencyMaxMS {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "io_latency_high",
			Severity: "high",
			Message:  fmt.Sprintf("I/O latency at %.1fms (threshold: %.1fms).", ioVal, thresholds.IOLatencyMaxMS),
			MetricValues: map[string]float64{
				"read_latency_ms":  readLat,
				"write_latency_ms": writeLat,
				"io_threshold":     thresholds.IOLatencyMaxMS,
			},
		})
	}

	if replLag > thresholds.ReplicationLagMaxSeconds {
		sev := "high"
		if replLag > thresholds.ReplicationLagMaxSeconds*2 {
			sev = "critical"
		}
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "replication_lag",
			Severity: sev,
			Message:  fmt.Sprintf("Replication lag at %.0fs (threshold: %.0fs). Log send queue: %.0f KB, Redo queue: %.0f KB.", replLag, thresholds.ReplicationLagMaxSeconds, logSendQ, redoQ),
			MetricValues: map[string]float64{
				"replication_lag_seconds": replLag,
				"lag_threshold":           thresholds.ReplicationLagMaxSeconds,
			},
		})
	}

	if blocking > float64(thresholds.BlockingSessionMax) {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "blocking_chains",
			Severity: "high",
			Message:  fmt.Sprintf("%.0f blocked sessions (threshold: %d). Check sqlserver_long_running_queries for head blocker.", blocking, thresholds.BlockingSessionMax),
			MetricValues: map[string]float64{
				"blocking_sessions":  blocking,
				"blocking_threshold": float64(thresholds.BlockingSessionMax),
			},
		})
	}

	if deadlocks > 0 {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "deadlocks_detected",
			Severity: "high",
			Message:  fmt.Sprintf("%.0f deadlocks detected. Check sqlserver_lock_history and application error logs.", deadlocks),
			MetricValues: map[string]float64{
				"deadlocks": deadlocks,
			},
		})
	}

	if tempdb > thresholds.TempDBUsedPctMax {
		sev := "medium"
		if tempdb > 90 {
			sev = "high"
		}
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "tempdb_pressure",
			Severity: sev,
			Message:  fmt.Sprintf("TempDB at %.1f%% (threshold: %.0f%%). Sort warnings: %.1f/s, Hash warnings: %.1f/s.", tempdb, thresholds.TempDBUsedPctMax, sortWarn, hashWarn),
			MetricValues: map[string]float64{
				"tempdb_used_pct":  tempdb,
				"tempdb_threshold": thresholds.TempDBUsedPctMax,
			},
			Recommendation: []string{
				"Identify sessions generating large temp objects via sys.dm_db_session_space_usage",
				"Check for query spills (sort/hash warnings indicate insufficient work memory)",
				"Verify TempDB has equal-sized data files (1 per core, up to 8)",
			},
		})
	}

	// TempDB acceleration alert: fires when TempDB is growing rapidly even if not yet at threshold.
	// Uses the existing tempdb_used_percent_series to detect a steep upward trend.
	if tempdbSeries := histories["tempdb_used_percent"]; len(tempdbSeries) >= 5 {
		recent := tempdbSeries[len(tempdbSeries)-5:]
		slope := (recent[len(recent)-1] - recent[0]) / float64(len(recent)-1)
		if slope > 3.0 && tempdb > 40 {
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "tempdb_growth_acceleration",
				Severity: "medium",
				Message:  fmt.Sprintf("TempDB growing rapidly: %.1f%% gain per collection cycle (current: %.1f%%). Proactive action recommended before threshold breach.", slope, tempdb),
				MetricValues: map[string]float64{
					"tempdb_used_pct":   tempdb,
					"growth_rate_pct":   slope,
				},
				Recommendation: []string{
					"Find the session driving rapid TempDB growth via sys.dm_db_session_space_usage",
					"Check for a runaway sort or hash join spilling to disk",
				},
			})
		}
	}

	if failedJobs > 0 {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "backup_failure_risk",
			Severity: "critical",
			Message:  fmt.Sprintf("%.0f SQL Agent job failures in 24h threatening backup integrity.", failedJobs),
			MetricValues: map[string]float64{
				"failed_jobs_24h": failedJobs,
			},
		})
	}

	if perfDebt > 0 {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "performance_debt",
			Severity: "medium",
			Message:  fmt.Sprintf("%.0f performance debt findings from sqlserver_performance_debt_findings require review.", perfDebt),
			MetricValues: map[string]float64{
				"performance_debt_count": perfDebt,
			},
		})
	}

	if sortWarn > thresholds.SortWarningsPerSecMax || hashWarn > thresholds.HashWarningsPerSecMax {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "query_spills",
			Severity: "medium",
			Message: fmt.Sprintf(
				"Query spills detected: sort warnings %.1f/s (threshold %.1f/s), hash warnings %.1f/s (threshold %.1f/s).",
				sortWarn, thresholds.SortWarningsPerSecMax, hashWarn, thresholds.HashWarningsPerSecMax,
			),
			MetricValues: map[string]float64{
				"sort_warnings_per_sec":      sortWarn,
				"hash_warnings_per_sec":      hashWarn,
				"sort_warnings_threshold":    thresholds.SortWarningsPerSecMax,
				"hash_warnings_threshold":    thresholds.HashWarningsPerSecMax,
			},
		})
	}

	sort.SliceStable(triggered, func(i, j int) bool {
		order := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return order[triggered[i].Severity] < order[triggered[j].Severity]
	})

	return triggered
}

func (e *AnalysisEngine) generateForecasts(raw map[string]interface{}, histories map[string][]float64) []models.ForecastResult {
	var results []models.ForecastResult

	type forecastDef struct {
		key   string
		label string
	}

	defs := []forecastDef{
		{key: "avg_cpu_load", label: "CPU Utilization"},
		{key: "free_disk_mb", label: "Free Disk Space"},
		{key: "memory_usage", label: "Memory Usage"},
		{key: "tempdb_used_percent", label: "TempDB Usage"},
		{key: "batch_requests_per_sec", label: "Workload (Batch Req/sec)"},
		{key: "secondary_lag_seconds", label: "Replication Lag"},
	}

	for _, fd := range defs {
		series := histories[fd.key]
		if len(series) >= 3 {
			fc := forecasting.ForecastLinear(series, fd.label, 30, 0.95)
			var timestamps []string
			for _, p := range fc.Points {
				timestamps = append(timestamps, p.Timestamp.Format(time.RFC3339))
			}
			results = append(results, models.ForecastResult{
				MetricName:           fd.label,
				HorizonDays:          fc.HorizonDays,
				PredictedValues:      extractValues(fc.Points),
				PredictedTimestamps:  timestamps,
				Confidence:           fc.Confidence,
				PredictedFailureDays: fc.PredictedFailureDays,
				ReliabilityTier:      fc.ReliabilityTier,
			})
		}
	}

	return results
}

func (e *AnalysisEngine) buildFailureForecast(forecasts []models.ForecastResult) []models.FailurePrediction {
	var failures []models.FailurePrediction
	for _, f := range forecasts {
		if f.PredictedFailureDays == nil {
			continue
		}
		// Do not surface failure windows from statistically unreliable linear fits.
		if f.ReliabilityTier == "Unreliable" {
			continue
		}
		conf := "Low"
		switch f.ReliabilityTier {
		case "Reliable":
			if f.Confidence > 0.8 {
				conf = "High"
			} else {
				conf = "Medium"
			}
		case "Indicative":
			conf = "Medium"
		default:
			if f.Confidence > 0.8 {
				conf = "High"
			} else if f.Confidence > 0.5 {
				conf = "Medium"
			}
		}
		window := fmt.Sprintf("%d days", *f.PredictedFailureDays)
		if f.ReliabilityTier == "Indicative" {
			window += " (indicative)"
		}
		failures = append(failures, models.FailurePrediction{
			Component:              f.MetricName,
			PredictedFailureWindow: window,
			Confidence:             conf,
			RiskType:               "capacity",
		})
	}
	return failures
}

func (e *AnalysisEngine) buildWorkingWell(raw map[string]interface{}, thresholds models.DynamicThresholds, triggered []models.RuleTriggerResult) []string {
	var working []string
	triggeredNames := make(map[string]bool)
	for _, t := range triggered {
		triggeredNames[t.RuleName] = true
	}

	cpu := getFloat(raw, "avg_cpu_load", "", 0)
	if cpu > 0 && cpu < thresholds.CPUSustainedThreshold*0.7 {
		working = append(working, fmt.Sprintf("CPU utilization at %.1f%% is well below dynamic threshold of %.0f%%", cpu, thresholds.CPUSustainedThreshold))
	}

	ple := getFloat(raw, "ple_seconds", "ple", 0)
	if ple > thresholds.MemoryPLEMinSeconds*2 {
		working = append(working, fmt.Sprintf("Page Life Expectancy at %.0fs is well above minimum threshold of %.0fs", ple, thresholds.MemoryPLEMinSeconds))
	}

	hit := getFloat(raw, "buffer_cache_hit_ratio", "", 0)
	if hit > 95 {
		working = append(working, fmt.Sprintf("Buffer cache hit ratio at %.1f%% indicates efficient memory utilization", hit))
	}

	// Only report replication health when Always On / mirroring telemetry is present.
	if haNA, _ := raw["ha_not_configured"].(bool); !triggeredNames["replication_lag"] && !haNA {
		if haConfigured, _ := raw["ha_configured"].(bool); haConfigured || hasAnyKey(raw, "ha_replica_states") {
			lag := getFloat(raw, "secondary_lag_seconds", "", 0)
			working = append(working, fmt.Sprintf("Replication stable with %.0fs lag (threshold: %.0fs)", lag, thresholds.ReplicationLagMaxSeconds))
		}
	}

	freeMB := getFloat(raw, "free_disk_mb", "free_mb", 0)
	if freeMB > thresholds.DiskFreeMBMin*3 {
		working = append(working, fmt.Sprintf("Disk space adequate at %.0f GB free (threshold: %.0f GB)", freeMB/1024, thresholds.DiskFreeMBMin/1024))
	}

	// Only report jobs OK / no blocking when the underlying metrics were actually collected.
	if !triggeredNames["backup_failure_risk"] && hasAnyKey(raw, "failed_jobs_24h") {
		working = append(working, "SQL Agent jobs completed successfully in last 24 hours")
	}

	if !triggeredNames["blocking_chains"] && hasAnyKey(raw, "blocking_sessions") {
		working = append(working, "No significant blocking chains detected")
	}

	if len(working) == 0 {
		working = append(working, "System is operational within dynamically computed thresholds")
	}

	if len(working) > 6 {
		working = working[:6]
	}

	return working
}

func (e *AnalysisEngine) buildRootCauses(triggered []models.RuleTriggerResult, raw map[string]interface{}) []string {
	// Sort by severity so the primary (most critical) factor always leads the chain.
	sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	sorted := make([]models.RuleTriggerResult, len(triggered))
	copy(sorted, triggered)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sevOrder[sorted[i].Severity] < sevOrder[sorted[j].Severity]
	})

	seen := map[string]bool{} // deduplicate by domain
	var causes []string

	for _, t := range sorted {
		if t.Severity == "low" {
			continue
		}
		mv := t.MetricValues
		var cause string

		if cpu, ok := mv["avg_cpu_load"]; ok && !seen["cpu"] {
			seen["cpu"] = true
			runnable := mv["runnable_tasks"]
			if runnable > 0 {
				cause = fmt.Sprintf("CPU pressure at %.1f%% with %.0f runnable tasks — workload is compute-bound; queries are competing for CPU cycles", cpu, runnable)
			} else {
				cause = fmt.Sprintf("CPU at %.1f%% — sustained high utilization; investigate top consumers in sys.dm_exec_requests and wait stats", cpu)
			}
		} else if ple, ok := mv["ple_seconds"]; ok && !seen["memory"] {
			seen["memory"] = true
			cause = fmt.Sprintf("Page Life Expectancy at %.0fs — buffer pool is evicting pages faster than normal, indicating insufficient RAM for current workload", ple)
		} else if disk, ok := mv["free_disk_mb"]; ok && !seen["disk"] {
			seen["disk"] = true
			growth := mv["delta_growth_mb"]
			if growth == 0 {
				growth = getFloat(raw, "delta_data_mb", "", 0)
			}
			if disk > 0 && growth > 0 {
				cause = fmt.Sprintf("Disk free space at %.1fGB with active growth (%.1f MB/cycle) — storage will reach threshold within forecast window", disk/1024, growth)
			} else if disk > 0 {
				cause = fmt.Sprintf("Disk free space at %.1fGB — below safety threshold; identify large tables, logs, or backup files to reclaim space", disk/1024)
			} else {
				cause = "Low disk space detected — review sqlserver_disk_history for top growth contributors"
			}
		} else if repl, ok := mv["replication_lag_seconds"]; ok && !seen["replication"] {
			seen["replication"] = true
			cause = fmt.Sprintf("Replication lag at %.0fs — secondary replica is falling behind; check network bandwidth and transaction log generation rate", repl)
		} else if block, ok := mv["blocking_sessions"]; ok && !seen["blocking"] {
			seen["blocking"] = true
			cause = fmt.Sprintf("%.0f blocking session(s) detected — long-running transactions or missing indexes are creating a concurrency bottleneck", block)
		} else if t.RuleName == "backup_failure_risk" && !seen["backup"] {
			seen["backup"] = true
			if jobs, ok2 := mv["failed_jobs_24h"]; ok2 {
				cause = fmt.Sprintf("%.0f SQL Agent job failure(s) in the last 24h — backup integrity and recoverability may be at risk", jobs)
			}
		} else if t.RuleName == "tempdb_pressure" && !seen["tempdb"] {
			seen["tempdb"] = true
			cause = "TempDB contention detected — sort/hash spills or allocation page hotspots; add TempDB data files or fix spill-causing queries"
		} else if !seen[t.RuleName] && t.Message != "" {
			seen[t.RuleName] = true
			cause = t.Message
		}

		if cause != "" {
			causes = append(causes, cause)
		}
		if len(causes) >= 5 {
			break
		}
	}
	return causes
}

func cpuTrendDescription(series []float64) string {
	if len(series) < 5 {
		return ""
	}
	slope := (series[len(series)-1] - series[0]) / float64(len(series))
	if slope > 0.5 {
		return "Trend: increasing."
	} else if slope < -0.5 {
		return "Trend: decreasing."
	}
	return "Trend: stable."
}

func getFloat(raw map[string]interface{}, primary, secondary string, def float64) float64 {
	if v, ok := raw[primary].(float64); ok {
		return v
	}
	if secondary != "" {
		if v, ok := raw[secondary].(float64); ok {
			return v
		}
	}
	return def
}


func getBool(raw map[string]interface{}, key string) bool {
	v, ok := raw[key]
	if !ok {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case int:
		return b != 0
	case float64:
		return b != 0
	}
	return false
}

func extractValues(points []forecasting.ForecastPoint) []float64 {
	vals := make([]float64, len(points))
	for i, p := range points {
		vals[i] = p.Value
	}
	return vals
}

// evaluateFeatureLibraryChecks fires rules for metrics already present in rawData whose
// thresholds are defined in SQLServerFeatures but were not previously wired to the engine
// (DEFECT-15 Phase A). All checks are guarded so they only fire when the metric is present.
//
// Phase A coverage (metrics confirmed in rawData snapshot):
//   - hadr_always_on: long_running_tx_count
//   - memory_configuration: available_physical_memory_kb (quantitative MB threshold)
//   - parallelism: cxpacket_wait_pct
//   - sql_agent_jobs: critical_jobs_disabled
func (e *AnalysisEngine) evaluateFeatureLibraryChecks(raw map[string]interface{}) []models.RuleTriggerResult {
	var triggered []models.RuleTriggerResult

	// --- hadr_always_on: long_running_tx_count ---
	// Long-running transactions on the AG primary block log truncation, growing log send queue.
	// Sentinel -1 means "metric not in rawData" — do not fire in that case.
	if longTx := getFloat(raw, "long_running_tx_count", "", -1); longTx >= 0 {
		ft := GetFeatureThreshold("hadr_always_on", "long_running_tx_count")
		if ft != nil {
			critVal, _ := toFloat64(ft.Critical) // 15
			warnVal, _ := toFloat64(ft.Warning)  // 5
			switch {
			case critVal > 0 && longTx >= critVal:
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "feature_ag_long_running_tx",
					Severity: "critical",
					Message: fmt.Sprintf(
						"%.0f long-running transactions on AG primary (critical: %.0f). "+
							"These block log truncation and inflate the log send queue.",
						longTx, critVal),
					MetricValues: map[string]float64{"long_running_tx_count": longTx},
					Recommendation: []string{
						"Identify long-running transactions via sys.dm_tran_active_transactions",
						"Review application transaction scoping — avoid holding transactions open across network calls",
						"Use KILL with caution after confirming the session in sys.dm_exec_sessions",
					},
				})
			case warnVal > 0 && longTx >= warnVal:
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "feature_ag_long_running_tx",
					Severity: "medium",
					Message: fmt.Sprintf(
						"%.0f long-running transactions on AG primary (warning: %.0f). Monitor log send queue growth.",
						longTx, warnVal),
					MetricValues: map[string]float64{"long_running_tx_count": longTx},
					Recommendation: []string{
						"Monitor log send queue trend via sys.dm_hadr_database_replica_states",
						"Identify open transactions via sys.dm_exec_sessions WHERE open_transaction_count > 0",
					},
				})
			}
		}
	}

	// --- memory_configuration: available_physical_memory_kb below threshold ---
	// Quantitative check against feature library MB thresholds. This complements the
	// existing boolean physical_memory_pressure_warning flag with a metric-based alert.
	if availKB := getFloat(raw, "available_physical_memory_kb", "", -1); availKB >= 0 {
		availMB := availKB / 1024
		ft := GetFeatureThreshold("memory_configuration", "available_physical_memory_mb_below")
		if ft != nil {
			critMB, _ := toFloat64(ft.Critical) // 512 MB
			warnMB, _ := toFloat64(ft.Warning)  // 1024 MB
			switch {
			case critMB > 0 && availMB < critMB:
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "feature_avail_memory_critical",
					Severity: "critical",
					Message: fmt.Sprintf(
						"Only %.0f MB physical memory available (critical threshold: %.0f MB). "+
							"OS and SQL Server are competing for RAM — buffer pool paging imminent.",
						availMB, critMB),
					MetricValues: map[string]float64{
						"available_physical_memory_mb": availMB,
						"threshold_mb":                 critMB,
					},
					Recommendation: []string{
						"Immediately reduce max server memory to leave OS headroom",
						"Enable Lock Pages In Memory (LPIM) to prevent SQL buffer pool paging",
						"Identify memory-consuming processes via sys.dm_os_ring_buffers (RING_BUFFER_OOM)",
					},
				})
			case warnMB > 0 && availMB < warnMB:
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "feature_avail_memory_warning",
					Severity: "medium",
					Message: fmt.Sprintf(
						"%.0f MB physical memory available (warning threshold: %.0f MB). Monitor for OS pressure.",
						availMB, warnMB),
					MetricValues: map[string]float64{
						"available_physical_memory_mb": availMB,
						"threshold_mb":                 warnMB,
					},
					Recommendation: []string{
						"Verify max server memory leaves adequate OS headroom (4 GB for < 64 GB RAM)",
						"Check for non-SQL processes consuming memory on this host",
					},
				})
			}
		}
	}

	// --- parallelism: CXPACKET wait percentage ---
	// High CXPACKET wait % indicates parallel query contention, often caused by MAXDOP=0
	// on many-core servers or a cost threshold for parallelism that is too low.
	if cxPct := getFloat(raw, "cxpacket_wait_pct", "", -1); cxPct >= 0 {
		ft := GetFeatureThreshold("parallelism", "cxpacket_wait_pct_above")
		if ft != nil {
			critPct, _ := toFloat64(ft.Critical) // 30
			warnPct, _ := toFloat64(ft.Warning)  // 15
			switch {
			case critPct > 0 && cxPct >= critPct:
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "feature_cxpacket_pressure",
					Severity: "critical",
					Message: fmt.Sprintf(
						"CXPACKET waits at %.1f%% of total wait time (critical: %.0f%%). "+
							"Parallel query contention is severely impacting throughput.",
						cxPct, critPct),
					MetricValues: map[string]float64{"cxpacket_wait_pct": cxPct},
					Recommendation: []string{
						"Set MAXDOP to optimal value for NUMA topology — do not leave at 0 (unrestricted)",
						"Raise cost threshold for parallelism from 5 to 25–50",
						"Identify high-CXPACKET queries via sys.dm_exec_query_stats ORDER BY total_elapsed_time DESC",
					},
				})
			case warnPct > 0 && cxPct >= warnPct:
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "feature_cxpacket_pressure",
					Severity: "medium",
					Message: fmt.Sprintf(
						"CXPACKET waits at %.1f%% of total wait time (warning: %.0f%%). Monitor for parallel plan contention.",
						cxPct, warnPct),
					MetricValues: map[string]float64{"cxpacket_wait_pct": cxPct},
					Recommendation: []string{
						"Review MAXDOP setting relative to NUMA topology and logical core count",
						"Check cost threshold for parallelism — default 5 is too low for most workloads",
					},
				})
			}
		}
	}

	// --- tempdb_configuration: data file count below recommendation ---
	// SQL Server recommends 1 TempDB data file per CPU core (up to 8) to reduce PAGELATCH
	// contention. Feature library: warning if < 4 files, critical if < 2.
	// Only fires when tempdb_data_files_count is present in rawData (Phase B).
	if fileCount := getFloat(raw, "tempdb_data_files_count", "", -1); fileCount >= 0 {
		ft := GetFeatureThreshold("tempdb_configuration", "tempdb_data_files_below")
		if ft != nil {
			warnBelow, _ := toFloat64(ft.Warning)  // 4
			critBelow, _ := toFloat64(ft.Critical) // 2
			switch {
			case critBelow > 0 && fileCount < critBelow:
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "feature_tempdb_files_critical",
					Severity: "critical",
					Message: fmt.Sprintf(
						"TempDB has only %.0f data file(s) (critical: must have ≥ %.0f). "+
							"Severe PAGELATCH contention risk on GAM, SGAM, and PFS pages.",
						fileCount, critBelow),
					MetricValues: map[string]float64{"tempdb_data_files_count": fileCount},
					Recommendation: []string{
						"Add TempDB data files to reach 1 per CPU core (up to 8): ALTER DATABASE tempdb ADD FILE (NAME=...)",
						"All files must be equal size with fixed-MB autogrowth (not percent)",
						"Place TempDB files on fast dedicated storage, not on C: drive",
					},
				})
			case warnBelow > 0 && fileCount < warnBelow:
				triggered = append(triggered, models.RuleTriggerResult{
					RuleName: "feature_tempdb_files_warning",
					Severity: "medium",
					Message: fmt.Sprintf(
						"TempDB has %.0f data file(s) — below recommended minimum of %.0f. "+
							"PAGELATCH contention may occur under concurrent temp-object workloads.",
						fileCount, warnBelow),
					MetricValues: map[string]float64{"tempdb_data_files_count": fileCount},
					Recommendation: []string{
						"Add TempDB data files to reach 1 per CPU core (up to 8)",
						"Ensure all data files are equal initial size and autogrowth setting",
					},
				})
			}
		}
	}

	// --- sql_agent_jobs: critical_jobs_disabled ---
	// Disabled critical jobs may include backup, integrity check, or replication tasks.
	// Feature library: warning=1, critical=3.
	if disabled := getFloat(raw, "critical_jobs_disabled", "", -1); disabled > 0 {
		ft := GetFeatureThreshold("sql_agent_jobs", "critical_jobs_disabled")
		if ft != nil {
			critVal, _ := toFloat64(ft.Critical) // 3
			warnVal, _ := toFloat64(ft.Warning)  // 1
			sev := "medium"
			switch {
			case critVal > 0 && disabled >= critVal:
				sev = "critical"
			case warnVal > 0 && disabled >= warnVal:
				sev = "high"
			}
			triggered = append(triggered, models.RuleTriggerResult{
				RuleName: "feature_critical_jobs_disabled",
				Severity: sev,
				Message: fmt.Sprintf(
					"%.0f critical SQL Agent job(s) are disabled (warning: %.0f, critical: %.0f). "+
						"Disabled jobs may include backup, integrity check, or replication tasks.",
					disabled, warnVal, critVal),
				MetricValues: map[string]float64{"critical_jobs_disabled": disabled},
				Recommendation: []string{
					"Review disabled jobs in SQL Server Agent (filter Enabled = No)",
					"Confirm each disabled job has a corresponding change record — accidental disablement is common after maintenance",
					"Re-enable and run each critical job once to verify it completes successfully",
				},
			})
		}
	}

	return triggered
}
