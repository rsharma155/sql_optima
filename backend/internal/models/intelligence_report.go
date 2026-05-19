// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Models for SQL Server Health Intelligence Report (Autonomous Analysis).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
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
type ForecastResult struct {
	MetricName           string    `json:"metric_name"`
	HorizonDays          int       `json:"horizon_days"`
	PredictedValues      []float64 `json:"predicted_values"`
	PredictedTimestamps  []string  `json:"predicted_timestamps"`
	Confidence           float64   `json:"confidence"`
	PredictedFailureDays *int      `json:"predicted_failure_days,omitempty"`
}

// FailurePrediction represents an estimated failure event.
type FailurePrediction struct {
	Component              string `json:"component"`
	PredictedFailureWindow string `json:"predicted_failure_window"`
	Confidence             string `json:"confidence"`
	RiskType               string `json:"risk_type"`
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
}

// ReportRequest represents the request to generate a specific report format.
type ReportRequest struct {
	RunID      string `json:"run_id"`
	ReportType string `json:"report_type"` // executive, technical, etc.
	Format     string `json:"format"`      // html, json, pdf
}
