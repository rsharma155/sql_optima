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

import (
	"github.com/google/uuid"
	"time"
)

// WatchedQueryStats represents stats for a user-watched query over a time range.
type WatchedQueryStats struct {
	CaptureTimestamp time.Time `json:"timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	QueryHash        string    `json:"query_hash"`
	DatabaseName     string    `json:"database_name"`
	ExecutionCount   int64     `json:"executions"`
	CPUTimeMs        int64     `json:"cpu_time_ms"`
	ElapsedTimeMs    int64     `json:"elapsed_time_ms"`
	LogicalReads     int64     `json:"logical_reads"`
	WaitTimeMs       int64     `json:"wait_time_ms"`
}

// QueryWaitStats represents Query Store wait stats for a specific query.
type QueryWaitStats struct {
	CaptureTimestamp time.Time `json:"timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	QueryHash        string    `json:"query_hash"`
	WaitType         string    `json:"wait_type"`
	WaitTimeMs       int64     `json:"wait_time_ms"`
	WaitTasks        int64     `json:"wait_tasks"`
}

// SqlServerQueryRegression represents a query that regressed in the last 24h vs previous 24h.
type SqlServerQueryRegression struct {
	CaptureTime    time.Time `json:"timestamp"`
	ServerID       uuid.UUID `json:"server_id"`
	DatabaseName   string    `json:"database_name"`
	QueryHash      string    `json:"query_hash"`
	QueryText      string    `json:"query_text"`
	RegressionType string    `json:"regression_type"` // "duration", "cpu", "reads"
	PreviousAvg    float64   `json:"previous_avg"`
	CurrentAvg     float64   `json:"current_avg"`
	PercentChange  float64   `json:"percent_change"`
	PlanChanged    bool      `json:"plan_changed"`
}

// SqlServerPlanInstability represents a query with multiple execution plans (>3).
type SqlServerPlanInstability struct {
	CaptureTime       time.Time `json:"timestamp"`
	ServerID          uuid.UUID `json:"server_id"`
	DatabaseName      string    `json:"database_name"`
	QueryHash         string    `json:"query_hash"`
	QueryText         string    `json:"query_text"`
	PlanCount         int       `json:"plan_count"`
	LastExecutionTime time.Time `json:"last_execution_time"`
}

// SqlServerWatchedQuery represents a user-tracked query or stored procedure.
type SqlServerWatchedQuery struct {
	ID             int       `json:"id"`
	ServerID       uuid.UUID `json:"server_id"`
	DatabaseName   string    `json:"database_name"`
	QueryHash      string    `json:"query_hash,omitempty"`
	ObjectID       int       `json:"object_id,omitempty"`
	Name           string    `json:"name"`
	QueryText      string    `json:"query_text,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	LastExecutedAt time.Time `json:"last_executed_at"`
}

// SqlServerWatchedQuerySnapshot represents a point-in-time metric snapshot for a watched query.
type SqlServerWatchedQuerySnapshot struct {
	SnapshotTime      time.Time                `json:"timestamp"`
	ServerID          uuid.UUID                `json:"server_id"`
	WatchedID         int                      `json:"watched_id"`
	Executions        int64                    `json:"executions"`
	AvgDurationMs     float64                  `json:"avg_duration_ms"`
	AvgCpuMs          float64                  `json:"avg_cpu_ms"`
	AvgReads          float64                  `json:"avg_reads"`
	TotalDurationMs   float64                  `json:"total_duration_ms"`
	TotalCpuMs        float64                  `json:"total_cpu_ms"`
	PlanCount         int                      `json:"plan_count"`
	LastExecutionTime time.Time                `json:"last_execution_time"`
	QueryPlan         string                   `json:"query_plan,omitempty"`
	QueryText         string                   `json:"query_text,omitempty"`
	WaitStats         []SqlServerQueryWaitStat `json:"wait_stats,omitempty"`
}

// SqlServerWatchedQueryEvent represents an optimization event marker on a watched query.
type SqlServerWatchedQueryEvent struct {
	ID        int       `json:"id"`
	WatchedID int       `json:"watched_id"`
	EventTime time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	Notes     string    `json:"notes"`
}

// SqlServerQueryAnalysisSummary represents the KPI card data for the Query Analysis dashboard.
type SqlServerQueryAnalysisSummary struct {
	TotalExecutions        int64   `json:"total_executions"`
	AvgDuration            float64 `json:"avg_duration_ms"`
	AvgCPU                 float64 `json:"avg_cpu_ms"`
	AvgReads               float64 `json:"avg_reads"`
	Regressions24h         int     `json:"regressions_24h"`
	PlanChanges24h         int     `json:"plan_changes_24h"`
	Top10CpuSharePct       float64 `json:"top_10_cpu_share_pct"`
	TotalQueriesInQS       int64   `json:"total_queries_in_qs"`
	QueriesExecutedInRange int64   `json:"queries_executed_in_range"`
	QueriesWithMultiPlans  int64   `json:"queries_with_multi_plans"`
	QueriesSingleExecution int64   `json:"queries_single_execution"`
	Message                string  `json:"message,omitempty"`
}

// SqlServerQueryPlanInfo represents plan metadata from Query Store for a watched query.
type SqlServerQueryPlanInfo struct {
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

// SqlServerQueryWaitStat represents wait stats from Query Store for a watched query.
type SqlServerQueryWaitStat struct {
	WaitCategory string  `json:"wait_category"`
	AvgWaitMs    float64 `json:"avg_wait_ms"`
	TotalWaitMs  float64 `json:"total_wait_ms"`
}

// SqlServerTopQueryRow represents a single row in the top queries table.
type SqlServerTopQueryRow struct {
	QueryHash         string    `json:"query_hash"`
	QueryText         string    `json:"query_text"`
	DatabaseName      string    `json:"database_name"`
	LoginName         string    `json:"login_name"`
	ApplicationName   string    `json:"application_name"`
	Executions        int64     `json:"executions"`
	AvgCpuMs          float64   `json:"avg_cpu_ms"`
	AvgDurationMs     float64   `json:"avg_duration_ms"`
	AvgReads          float64   `json:"avg_reads"`
	TotalCpuMs        float64   `json:"total_cpu_ms"`
	PlanCount         int       `json:"plan_count"`
	LastExecutionTime time.Time `json:"last_execution_time"`
}
