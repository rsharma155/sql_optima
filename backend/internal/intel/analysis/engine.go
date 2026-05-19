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

	// 1. Evaluate YAML rules (ARCH-1)
	yamlRules := rule_engine.EvaluateRulesFromRaw(e.rules, rawData)
	triggeredRules = append(triggeredRules, yamlRules...)

	// 2. Run Anomaly Detection (ARCH-2)
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
			"title":       t.RuleName,
			"description": t.Message,
			"severity":    t.Severity,
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
	free := getFloat(raw, "total_free_mb", "free_disk_mb", 0)
	total := getFloat(raw, "total_data_mb", "data_disk_mb", 1) + getFloat(raw, "total_log_mb", "log_disk_mb", 1) + free
	if total <= 1 {
		return 0
	}
	usedPct := (total - free) / total
	return clamp(usedPct*80, 0, 100)
}

func computeBaseAvailabilityRisk(raw map[string]interface{}, t *models.DynamicThresholds) float64 {
	ioLat := math.Max(getFloat(raw, "read_latency_ms", "", 0), getFloat(raw, "write_latency_ms", "", 0))
	ioRisk := (ioLat / t.IOLatencyMaxMS) * 50
	return clamp(ioRisk, 0, 100)
}

func computeBaseReplicationRisk(raw map[string]interface{}, t *models.DynamicThresholds) float64 {
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
	freeMB := getFloat(raw, "free_disk_mb", "free_mb", 50000)
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

	if freeMB < thresholds.DiskFreeMBMin {
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
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "tempdb_pressure",
			Severity: "medium",
			Message:  fmt.Sprintf("TempDB at %.1f%% (threshold: %.0f%%). Sort warnings: %.1f/s, Hash warnings: %.1f/s.", tempdb, thresholds.TempDBUsedPctMax, sortWarn, hashWarn),
			MetricValues: map[string]float64{
				"tempdb_used_pct":  tempdb,
				"tempdb_threshold": thresholds.TempDBUsedPctMax,
			},
		})
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

	if sortWarn > 1 || hashWarn > 1 {
		triggered = append(triggered, models.RuleTriggerResult{
			RuleName: "query_spills",
			Severity: "medium",
			Message:  fmt.Sprintf("Query spills detected: sort warnings %.1f/s, hash warnings %.1f/s. Queries spilling to tempdb impact performance.", sortWarn, hashWarn),
			MetricValues: map[string]float64{
				"sort_warnings_per_sec": sortWarn,
				"hash_warnings_per_sec": hashWarn,
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
			})
		}
	}

	return results
}

func (e *AnalysisEngine) buildFailureForecast(forecasts []models.ForecastResult) []models.FailurePrediction {
	var failures []models.FailurePrediction
	for _, f := range forecasts {
		if f.PredictedFailureDays != nil {
			conf := "Low"
			if f.Confidence > 0.8 {
				conf = "High"
			} else if f.Confidence > 0.5 {
				conf = "Medium"
			}
			failures = append(failures, models.FailurePrediction{
				Component:              f.MetricName,
				PredictedFailureWindow: fmt.Sprintf("%d days", *f.PredictedFailureDays),
				Confidence:             conf,
				RiskType:               "capacity",
			})
		}
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

	if !triggeredNames["replication_lag"] {
		lag := getFloat(raw, "secondary_lag_seconds", "", 0)
		working = append(working, fmt.Sprintf("Replication stable with %.0fs lag (threshold: %.0fs)", lag, thresholds.ReplicationLagMaxSeconds))
	}

	freeMB := getFloat(raw, "free_disk_mb", "free_mb", 0)
	if freeMB > thresholds.DiskFreeMBMin*3 {
		working = append(working, fmt.Sprintf("Disk space adequate at %.0f GB free (threshold: %.0f GB)", freeMB/1024, thresholds.DiskFreeMBMin/1024))
	}

	if !triggeredNames["backup_failure_risk"] {
		working = append(working, "SQL Agent jobs completed successfully in last 24 hours")
	}

	if !triggeredNames["blocking_chains"] {
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
	var causes []string
	for _, t := range triggered {
		if t.Severity != "critical" && t.Severity != "high" {
			continue
		}
		mv := t.MetricValues
		if cpu, ok := mv["avg_cpu_load"]; ok {
			causes = append(causes, fmt.Sprintf("Sustained CPU at %.1f%% with %.0f runnable tasks indicates compute-bound workload rather than I/O wait", cpu, mv["runnable_tasks"]))
		}
		if ple, ok := mv["ple_seconds"]; ok {
			causes = append(causes, fmt.Sprintf("PLE collapse to %.0fs suggests memory pressure from large buffer pool scans or insufficient RAM", ple))
		}
		if disk, ok := mv["free_disk_mb"]; ok {
			causes = append(causes, fmt.Sprintf("Disk space at %.0f MB with growth rate %.0f MB/interval indicates storage planning gap", disk, mv["delta_growth_mb"]))
		}
		if repl, ok := mv["replication_lag_seconds"]; ok {
			causes = append(causes, fmt.Sprintf("Replication lag of %.0fs suggests network throughput or subscriber apply speed issue", repl))
		}
		if block, ok := mv["blocking_sessions"]; ok {
			causes = append(causes, fmt.Sprintf("Blocking chains with %.0f sessions indicate application concurrency or transaction length issues", block))
		}
		if t.RuleName == "backup_failure_risk" {
			if jobs, ok := mv["failed_jobs_24h"]; ok {
				causes = append(causes, fmt.Sprintf("SQL Agent job failures (%.0f in 24h) indicate potential backup/recovery gaps", jobs))
			}
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
