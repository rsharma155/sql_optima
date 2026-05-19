// Package models defines domain entities for SQL Server observability.
// sqlserver_wait_stats_v2.go contains models for the new Wait Stats Dashboard.
// Metadata:
//   - Feature: SQL Server Wait Stats V2
//   - Layer: Models (API Response)
package models

import "time"

// WaitStatsDashboardResponse is the top-level container for the Wait Stats page.
type WaitStatsDashboardResponse struct {
	InstanceName      string               `json:"instance_name"`
	KPIs              WaitStatsKPIs        `json:"kpis"`
	WaitTrendsHourly  []WaitTrendPoint     `json:"wait_trends_hourly"`
	WaitTrendsDaily   []WaitTrendPoint     `json:"wait_trends_daily"` // Heatmap source
	TopWaitTypes      []TopWaitType        `json:"top_wait_types"`
	ActiveWaits       []ActiveWaitSession  `json:"active_waits"`
	CPUPressure       []CPUPressurePoint   `json:"cpu_pressure"`
	DatabaseImpact    []DatabaseWaitImpact `json:"database_impact"`
	BlockingTree      []WaitStatsBlockingNode `json:"blocking_tree"`
	LastUpdate        time.Time            `json:"last_update"`
}

// WaitStatsKPIs provides the primary bottleneck signals.
type WaitStatsKPIs struct {
	TotalWaitTimeMs    int64   `json:"total_wait_time_ms"`
	SignalWaitTimeMs   int64   `json:"signal_wait_time_ms"`
	SignalWaitPct      float64 `json:"signal_wait_pct"` // (Signal / Total) * 100
	ResourceWaitTimeMs int64   `json:"resource_wait_time_ms"`
	TotalWaitingTasks  int64   `json:"total_waiting_tasks"`
	TopWaitCategory    string  `json:"top_wait_category,omitempty"`
	RestartDetected    bool    `json:"restart_detected"`
}

// CPUPressurePoint relates signal waits to scheduler yields.
type CPUPressurePoint struct {
	Timestamp         time.Time `json:"timestamp"`
	SignalWaitMs      int64     `json:"signal_wait_ms"`
	SchedulerYieldMs  int64     `json:"scheduler_yield_ms"`
}

// DatabaseWaitImpact shows which DB is suffering the most waits.
type DatabaseWaitImpact struct {
	DatabaseName string `json:"database_name"`
	TotalWaitMs  int64  `json:"total_wait_ms"`
}

// WaitStatsBlockingNode is a single element in a blocking hierarchy.
type WaitStatsBlockingNode struct {
	SessionID         int                     `json:"session_id"`
	BlockingSessionID int                     `json:"blocking_session_id"`
	WaitType          string                  `json:"wait_type"`
	WaitDurationMs    int64                   `json:"wait_duration_ms"`
	DatabaseName      string                  `json:"database_name"`
	QueryText         string                  `json:"query_text"`
	Children          []WaitStatsBlockingNode `json:"children,omitempty"`
}

// WaitTypeHelp provides DBA guidance for a specific wait type.
type WaitTypeHelp struct {
	WaitType           string `json:"wait_type"`
	Description        string `json:"description"`
	LikelyCause        string `json:"likely_cause"`
	RecommendedAction  string `json:"recommended_action"`
}

// TopWaitType gives granular detail on one wait type.
type TopWaitType struct {
	WaitType           string  `json:"wait_type"`
	Category           string  `json:"category"`
	WaitTimeMs         int64   `json:"wait_time_ms"`
	WaitingTasksCount  int64   `json:"waiting_tasks_count"`
	AvgWaitMs          float64 `json:"avg_wait_ms"`
	PercentOfTotal     float64 `json:"percent_of_total"`
	Description        string  `json:"description"`
	RecommendedAction  string  `json:"recommended_action"`
}

// ActiveWaitSession (extended for dashboard display).
type ActiveWaitSession struct {
	SessionID         int       `json:"session_id"`
	WaitType          string    `json:"wait_type"`
	WaitDurationMs    int64     `json:"wait_duration_ms"`
	BlockingSessionID *int      `json:"blocking_session_id"`
	DatabaseName      string    `json:"database_name"`
	HostName          string    `json:"host_name"`
	ProgramName       string    `json:"program_name"`
	LoginName         string    `json:"login_name"`
	QueryText         string    `json:"query_text"`
	CaptureTimestamp  time.Time `json:"capture_timestamp"`
}
