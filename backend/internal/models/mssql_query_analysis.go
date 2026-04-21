// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Domain models for the SQL Server Query Analysis Dashboard including
//
//	query regressions, plan instability, watched queries, and KPI summaries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import "time"

// MssqlQueryRegression represents a query that regressed in the last 24h vs previous 24h.
type MssqlQueryRegression struct {
	CaptureTime    time.Time `json:"capture_time"`
	InstanceName   string    `json:"instance_name"`
	DatabaseName   string    `json:"database_name"`
	QueryHash      string    `json:"query_hash"`
	QueryText      string    `json:"query_text"`
	RegressionType string    `json:"regression_type"` // "duration", "cpu", "reads"
	PreviousAvg    float64   `json:"previous_avg"`
	CurrentAvg     float64   `json:"current_avg"`
	PercentChange  float64   `json:"percent_change"`
	PlanChanged    bool      `json:"plan_changed"`
}

// MssqlPlanInstability represents a query with multiple execution plans (>3).
type MssqlPlanInstability struct {
	CaptureTime       time.Time `json:"capture_time"`
	InstanceName      string    `json:"instance_name"`
	DatabaseName      string    `json:"database_name"`
	QueryHash         string    `json:"query_hash"`
	QueryText         string    `json:"query_text"`
	PlanCount         int       `json:"plan_count"`
	LastExecutionTime time.Time `json:"last_execution_time"`
}

// MssqlWatchedQuery represents a user-tracked query or stored procedure.
type MssqlWatchedQuery struct {
	ID             int       `json:"id"`
	InstanceName   string    `json:"instance_name"`
	DatabaseName   string    `json:"database_name"`
	QueryHash      string    `json:"query_hash,omitempty"`
	ObjectID       int       `json:"object_id,omitempty"`
	Name           string    `json:"name"`
	QueryText      string    `json:"query_text,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	LastExecutedAt time.Time `json:"last_executed_at"`
}

// MssqlWatchedQuerySnapshot represents a point-in-time metric snapshot for a watched query.
type MssqlWatchedQuerySnapshot struct {
	SnapshotTime      time.Time            `json:"snapshot_time"`
	WatchedID         int                  `json:"watched_id"`
	Executions        int64                `json:"executions"`
	AvgDurationMs     float64              `json:"avg_duration_ms"`
	AvgCpuMs          float64              `json:"avg_cpu_ms"`
	AvgReads          float64              `json:"avg_reads"`
	TotalDurationMs   float64              `json:"total_duration_ms"`
	TotalCpuMs        float64              `json:"total_cpu_ms"`
	PlanCount         int                  `json:"plan_count"`
	LastExecutionTime time.Time            `json:"last_execution_time"`
	QueryPlan         string               `json:"query_plan,omitempty"`
	QueryText         string               `json:"query_text,omitempty"`
	WaitStats         []MssqlQueryWaitStat `json:"wait_stats,omitempty"`
}

// MssqlWatchedQueryEvent represents an optimization event marker on a watched query.
type MssqlWatchedQueryEvent struct {
	ID        int       `json:"id"`
	WatchedID int       `json:"watched_id"`
	EventTime time.Time `json:"event_time"`
	EventType string    `json:"event_type"`
	Notes     string    `json:"notes"`
}

// MssqlQueryAnalysisSummary represents the KPI card data for the Query Analysis dashboard.
type MssqlQueryAnalysisSummary struct {
	TotalExecutions int64   `json:"total_executions"`
	AvgDuration     float64 `json:"avg_duration_ms"`
	AvgCPU          float64 `json:"avg_cpu_ms"`
	AvgReads        float64 `json:"avg_reads"`
	Regressions24h  int     `json:"regressions_24h"`
	PlanChanges24h  int     `json:"plan_changes_24h"`
}

// MssqlQueryPlanInfo represents plan metadata from Query Store for a watched query.
type MssqlQueryPlanInfo struct {
	PlanID            int64     `json:"plan_id"`
	AvgDurationMs     float64   `json:"avg_duration_ms"`
	AvgCpuMs          float64   `json:"avg_cpu_ms"`
	AvgReads          float64   `json:"avg_reads"`
	Executions        int64     `json:"executions"`
	IsForcedPlan      bool      `json:"is_forced_plan"`
	LastExecutionTime time.Time `json:"last_execution_time"`
	CreatedAt         time.Time `json:"created_at"`
	QueryPlan         string    `json:"query_plan"`
}

// MssqlQueryWaitStat represents wait stats from Query Store for a watched query.
type MssqlQueryWaitStat struct {
	WaitCategory string  `json:"wait_category"`
	AvgWaitMs    float64 `json:"avg_wait_ms"`
	TotalWaitMs  float64 `json:"total_wait_ms"`
}

// MssqlTopQueryRow represents a single row in the top queries table.
type MssqlTopQueryRow struct {
	QueryHash     string  `json:"query_hash"`
	QueryText     string  `json:"query_text"`
	DatabaseName  string  `json:"database_name"`
	Executions    int64   `json:"executions"`
	AvgCpuMs      float64 `json:"avg_cpu_ms"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	AvgReads      float64 `json:"avg_reads"`
	TotalCpuMs    float64 `json:"total_cpu_ms"`
	PlanCount     int     `json:"plan_count"`
}
