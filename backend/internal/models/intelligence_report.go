// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Models for SQL Server Health Intelligence Report (Autonomous Analysis).
// Extended in v2 to support utilization classification, peak window detection,
// named wait type analysis, per-database capacity forecasting, and snapshot persistence.
//
// Design context:
//   - All new fields are optional (omitempty) to preserve backward compatibility.
//   - ForecastResult gains ReliabilityTier (DEFECT-10 fix).
//   - IntelligenceReportResponse gains UtilizationProfile, PeakWindow, TopWaitTypes,
//     DatabaseGrowth — populated by the v2 analysis pipeline.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT
package models

// IntelligenceReportRequest represents the payload to trigger analysis.
type IntelligenceReportRequest struct {
	TargetDatabase string                 `json:"target_database"`
	TimeRangeHours int                    `json:"time_range_hours"`
	RunAsync       bool                   `json:"run_async"`
	RawData        map[string]interface{} `json:"raw_data,omitempty"`
}

// RiskScoreResult holds the multi-dimensional risk scores.
type RiskScoreResult struct {
	PerformanceRisk  float64 `json:"performance_risk"`
	CapacityRisk     float64 `json:"capacity_risk"`
	AvailabilityRisk float64 `json:"availability_risk"`
	ReplicationRisk  float64 `json:"replication_risk"`
	MaintenanceRisk  float64 `json:"maintenance_risk"`
	QueryRisk        float64 `json:"query_risk"`
	OverallScore     float64 `json:"overall_score"`
	Category         string  `json:"category"`
	Confidence       float64 `json:"confidence"`
	// DataGaps lists dimension names where data was absent at scoring time.
	// DEFECT-2 fix: missing data is surfaced explicitly rather than silently zeroed.
	DataGaps []string `json:"data_gaps,omitempty"`
}

// RuleTriggerResult represents a triggered expert rule.
type RuleTriggerResult struct {
	RuleName       string             `json:"rule_name"`
	Severity       string             `json:"severity"`
	Message        string             `json:"message"`
	MetricValues   map[string]float64 `json:"metric_values"`
	Recommendation []string           `json:"recommendation"`
}

// RuleCondition represents a condition for a rule.
type RuleCondition struct {
	Metric   string  `json:"metric" yaml:"metric"`
	Operator string  `json:"operator" yaml:"operator"`
	Value    float64 `json:"value" yaml:"value"`
}

// RuleDefinition represents a rule.
type RuleDefinition struct {
	Name           string          `json:"name" yaml:"name"`
	Description    string          `json:"description,omitempty" yaml:"description,omitempty"`
	Conditions     []RuleCondition `json:"conditions" yaml:"conditions"`
	Logic          string          `json:"logic" yaml:"logic"`
	Severity       string          `json:"severity" yaml:"severity"`
	Message        string          `json:"message" yaml:"message"`
	Recommendation []string        `json:"recommendation" yaml:"recommendation"`
}

// ServerConfig holds hardware and system configuration.
type ServerConfig struct {
	CPUCount         int  `json:"cpu_count"`
	TotalRAMGB       int  `json:"total_ram_gb"`
	TotalDiskGB      int  `json:"total_disk_gb"`
	HyperthreadRatio int  `json:"hyperthread_ratio"`
	SocketCount      int  `json:"socket_count"`
	CoresPerSocket   int  `json:"cores_per_socket"`
	NumaNodes        int  `json:"numa_nodes"`
	MaxWorkers       int  `json:"max_workers"`
	IsVirtual        bool `json:"is_virtual"`
}

// DynamicThresholds holds dynamically computed health thresholds.
type DynamicThresholds struct {
	CPUMaxThreshold           float64 `json:"cpu_max_threshold"`
	CPUSustainedThreshold     float64 `json:"cpu_sustained_threshold"`
	CPURunnablePerCore        float64 `json:"cpu_runnable_per_core"`
	CPURunnableAbsolute       int     `json:"cpu_runnable_absolute"`
	MemoryPLEMinSeconds       float64 `json:"memory_ple_min_seconds"`
	MemoryGrantsPendingMax    int     `json:"memory_grants_pending_max"`
	MemoryUsedPctMax          float64 `json:"memory_used_pct_max"`
	DiskFreeMBMin             float64 `json:"disk_free_mb_min"`
	DiskGrowthRateMaxMBPerDay float64 `json:"disk_growth_rate_max_mb_per_day"`
	IOLatencyMaxMS            float64 `json:"io_latency_max_ms"`
	ReplicationLagMaxSeconds  float64 `json:"replication_lag_max_seconds"`
	TempDBUsedPctMax          float64 `json:"tempdb_used_pct_max"`
	BlockingSessionMax        int     `json:"blocking_session_max"`
	QueryDurationMaxSeconds   float64 `json:"query_duration_max_seconds"`
	BatchRequestsPerCoreMax   float64 `json:"batch_requests_per_core_max"`
	ConnectionCountMax        int     `json:"connection_count_max"`
}

// ToMap converts thresholds to a generic map.
func (t *DynamicThresholds) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"cpu_max_threshold":               t.CPUMaxThreshold,
		"cpu_sustained_threshold":         t.CPUSustainedThreshold,
		"cpu_runnable_per_core":           t.CPURunnablePerCore,
		"cpu_runnable_absolute":           t.CPURunnableAbsolute,
		"memory_ple_min_seconds":          t.MemoryPLEMinSeconds,
		"memory_grants_pending_max":       t.MemoryGrantsPendingMax,
		"memory_used_pct_max":             t.MemoryUsedPctMax,
		"disk_free_mb_min":                t.DiskFreeMBMin,
		"disk_growth_rate_max_mb_per_day": t.DiskGrowthRateMaxMBPerDay,
		"io_latency_max_ms":               t.IOLatencyMaxMS,
		"replication_lag_max_seconds":     t.ReplicationLagMaxSeconds,
		"tempdb_used_pct_max":             t.TempDBUsedPctMax,
		"blocking_session_max":            t.BlockingSessionMax,
		"query_duration_max_seconds":      t.QueryDurationMaxSeconds,
		"batch_requests_per_core_max":     t.BatchRequestsPerCoreMax,
		"connection_count_max":            t.ConnectionCountMax,
	}
}

// ForecastResult represents a time-series forecast for a metric.
// ReliabilityTier is set based on R²: Reliable (>0.7), Indicative (0.3–0.7), Unreliable (<0.3).
// DEFECT-10 fix: low-confidence forecasts are now explicitly labelled.
type ForecastResult struct {
	MetricName           string    `json:"metric_name"`
	HorizonDays          int       `json:"horizon_days"`
	PredictedValues      []float64 `json:"predicted_values"`
	PredictedTimestamps  []string  `json:"predicted_timestamps"`
	Confidence           float64   `json:"confidence"`
	PredictedFailureDays *int      `json:"predicted_failure_days,omitempty"`
	// ReliabilityTier: "Reliable" | "Indicative" | "Unreliable"
	ReliabilityTier string `json:"reliability_tier,omitempty"`
}

// FailurePrediction represents an estimated failure event.
type FailurePrediction struct {
	Component              string `json:"component"`
	PredictedFailureWindow string `json:"predicted_failure_window"`
	Confidence             string `json:"confidence"`
	RiskType               string `json:"risk_type"`
}

// ---- V2 Types (new in redesign) ----

// UtilizationProfile holds 14-day CPU/memory percentile bands and a utilization class.
type UtilizationProfile struct {
	// CPU percentiles
	CPUP50 float64 `json:"cpu_p50"`
	CPUP90 float64 `json:"cpu_p90"`
	CPUP95 float64 `json:"cpu_p95"`
	CPUP99 float64 `json:"cpu_p99"`
	CPUAvg float64 `json:"cpu_avg"`
	// Memory percentiles
	MemP50 float64 `json:"mem_p50"`
	MemP95 float64 `json:"mem_p95"`
	MemAvg float64 `json:"mem_avg"`
	// Classification
	Class   string `json:"class"`   // "Under-utilized" | "Optimal" | "Elevated" | "Over-utilized"
	Message string `json:"message"` // human-readable explanation
	// Sample count used for computation
	SampleCount int `json:"sample_count"`
}

// HourlySlot is one cell in the 7×24 CPU heatmap.
type HourlySlot struct {
	DayOfWeek  int     `json:"day_of_week"`  // 1=Monday, 7=Sunday (ISO)
	HourOfDay  int     `json:"hour_of_day"`  // 0–23
	AvgCPUPct  float64 `json:"avg_cpu_pct"`
	PeakCPUPct float64 `json:"peak_cpu_pct"`
	SampleCount int    `json:"sample_count"`
}

// PeakWindow describes the server's busiest time block.
type PeakWindow struct {
	// Top 3 busiest slots ranked by avg CPU
	TopSlots     []HourlySlot `json:"top_slots"`
	HeatmapSlots []HourlySlot `json:"heatmap_slots"` // full 7×24 matrix for Plotly heatmap
	PeakDays     []string     `json:"peak_days"`     // e.g., ["Monday","Tuesday","Wednesday"]
	PeakHours    string       `json:"peak_hours"`    // e.g., "09:00-11:00"
	OffPeakDesc  string       `json:"off_peak_desc"` // e.g., "22:00-06:00 on weekdays"
	PeakCPUAvg   float64      `json:"peak_cpu_avg"`
	OffPeakAvg   float64      `json:"off_peak_avg"`
}

// WaitTypeEntry holds detail for one named SQL Server wait type.
type WaitTypeEntry struct {
	Rank             int     `json:"rank"`
	WaitType         string  `json:"wait_type"`
	WaitCategory     string  `json:"wait_category"`
	TotalWaitMS      float64 `json:"total_wait_ms"`
	PctOfTotal       float64 `json:"pct_of_total"`
	WaitingTasks     float64 `json:"waiting_tasks"`
	// Interpretation and DBA guidance
	Category         string  `json:"category"`     // CPU | Lock | IO | Memory | Network | Other
	Implication      string  `json:"implication"`  // plain-English DBA note
	WaitScore        float64 `json:"wait_score"`   // 0–100, pctOfTotal × 2
}

// DatabaseGrowthEntry is per-database storage growth data.
type DatabaseGrowthEntry struct {
	DatabaseName      string  `json:"database_name"`
	CurrentDataMB     float64 `json:"current_data_mb"`
	CurrentLogMB      float64 `json:"current_log_mb"`
	Daily7AvgGrowthMB float64 `json:"daily_7d_avg_growth_mb"` // true 24h daily average
	Projected30dMB    float64 `json:"projected_30d_mb"`
	Projected90dMB    float64 `json:"projected_90d_mb"`
	GrowthPattern     string  `json:"growth_pattern"` // "Steady" | "Variable" | "Spikey"
	ReliabilityTier   string  `json:"reliability_tier"`
}

// CapacityForecastV2 holds the redesigned capacity projection with breach date.
type CapacityForecastV2 struct {
	TotalDiskMB       float64 `json:"total_disk_mb"`
	UsedDiskMB        float64 `json:"used_disk_mb"`
	FreeDiskMB        float64 `json:"free_disk_mb"`
	UsedPct           float64 `json:"used_pct"`
	DailyGrowthMBAvg  float64 `json:"daily_growth_mb_avg"`  // true 24h aggregate
	GrowthPattern     string  `json:"growth_pattern"`
	ReliabilityTier   string  `json:"reliability_tier"`
	Projected30dMB    float64 `json:"projected_30d_mb"`
	Projected60dMB    float64 `json:"projected_60d_mb"`
	Projected90dMB    float64 `json:"projected_90d_mb"`
	DaysUntilBreach   int     `json:"days_until_breach"`   // -1 = never/unknown
	BreachDateApprox  string  `json:"breach_date_approx"`  // "Sep 2026" or "No breach in 90 days"
	RSquared          float64 `json:"r_squared"`
	Databases         []DatabaseGrowthEntry `json:"databases,omitempty"`
}

// IntelSnapshotEntry is a historical point from intelreport.intel_snapshots.
type IntelSnapshotEntry struct {
	Timestamp        string  `json:"timestamp"`
	OverallRisk      float64 `json:"overall_risk"`
	PerformanceRisk  float64 `json:"performance_risk"`
	CapacityRisk     float64 `json:"capacity_risk"`
	AvailabilityRisk float64 `json:"availability_risk"`
	ReplicationRisk  float64 `json:"replication_risk"`
	MaintenanceRisk  float64 `json:"maintenance_risk"`
	QueryRisk        float64 `json:"query_risk"`
	UtilizationClass string  `json:"utilization_class"`
	CPUP95           float64 `json:"cpu_p95"`
	PLECurrent       float64 `json:"ple_current"`
	DiskUsedPct      float64 `json:"disk_used_pct"`
	CriticalCount    int     `json:"rule_count_critical"`
	HighCount        int     `json:"rule_count_high"`
}

// IntelligenceReportResponse represents the full health analysis result.
type IntelligenceReportResponse struct {
	RunID               string              `json:"run_id"`
	OverallRisk         RiskScoreResult     `json:"overall_risk"`
	RiskTrend           []RiskScoreResult   `json:"risk_trend,omitempty"`
	DataStatus          string              `json:"data_status"`
	DataNote            string              `json:"data_note"`
	WorkingWell         []string            `json:"working_well"`
	CurrentIssues       []map[string]any    `json:"current_issues"`
	WhatCouldGoWrong    []string            `json:"what_could_go_wrong"`
	FailureForecast     []FailurePrediction `json:"failure_forecast"`
	RootCauseHypotheses []string            `json:"root_cause_hypotheses"`
	RecommendedActions  []map[string]string `json:"recommended_actions"`
	TriggeredRules      []RuleTriggerResult `json:"triggered_rules"`
	Forecasts           []ForecastResult    `json:"forecasts"`
	ConfidenceScore     float64             `json:"confidence_score"`
	NarrativeSummary    string              `json:"narrative_summary"`
	GeneratedAt         string              `json:"generated_at"`

	// V2 fields — populated by redesigned pipeline. Nil when insufficient data.
	UtilizationProfile *UtilizationProfile  `json:"utilization_profile,omitempty"`
	PeakWindow         *PeakWindow          `json:"peak_window,omitempty"`
	TopWaitTypes       []WaitTypeEntry      `json:"top_wait_types,omitempty"`
	WaitTrend          []WaitTrendPoint     `json:"wait_trend,omitempty"`
	CapacityForecastV2 *CapacityForecastV2  `json:"capacity_forecast_v2,omitempty"`
	HistoryTrend       []IntelSnapshotEntry `json:"history_trend,omitempty"`
}

// ReportRequest represents the request to generate a specific report format.
type ReportRequest struct {
	RunID      string `json:"run_id"`
	ReportType string `json:"report_type"` // executive, technical, etc.
	Format     string `json:"format"`      // html, json, pdf
}
