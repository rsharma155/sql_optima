// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Domain models for SQL Server Workload Observability.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package domain

import "time"

// WorkloadQueryFilter scopes workload dashboard reads to TimescaleDB hypertables only.
type WorkloadQueryFilter struct {
	Database         string   // required user database scope (handlers resolve default when omitted)
	ExcludeSystem    bool     // when true, apply full user-workload filters; false = metrics minus monitoring self-noise
	MonitoringLogins []string // optima_servers.username + defaults for login_name exclusion
}

// SqlServerWorkloadDatabaseActivity summarizes collector rows per database in a time window.
type SqlServerWorkloadDatabaseActivity struct {
	DatabaseName string `json:"database_name"`
	RowCount     int64  `json:"row_count"`
	TotalCPUms   int64  `json:"total_cpu_ms"`
}

// SqlServerWorkloadDiagnostics helps explain empty charts (TimescaleDB coverage vs selected range).
type SqlServerWorkloadDiagnostics struct {
	LatestHistoryCapture  *time.Time                          `json:"latest_history_capture,omitempty"`
	LatestMetricsCapture  *time.Time                          `json:"latest_metrics_capture,omitempty"`
	CollectorLastPoll     *time.Time                          `json:"collector_last_poll,omitempty"`
	HistoryRowsInRange    int64                               `json:"history_rows_in_range"`
	MetricsRowsInRange    int64                               `json:"metrics_rows_in_range"`
	MetricsRowsUnfiltered int64                               `json:"metrics_rows_unfiltered,omitempty"`
	DatabasesInRange      []SqlServerWorkloadDatabaseActivity `json:"databases_in_range,omitempty"`
}

// SqlServerWorkloadSummary represents the top-level KPI metrics for a given period.
type SqlServerWorkloadSummary struct {
	TotalCPUms        int64                        `json:"total_cpu_ms"`
	TotalExecutions   int64                        `json:"total_executions"`
	TotalLogicalReads int64                        `json:"total_logical_reads"`
	TotalRows         int64                        `json:"total_rows"`
	MaxMemoryGrantKB  int64                        `json:"max_memory_grant_kb"`
	AvgCPUPerExec     float64                      `json:"avg_cpu_per_exec"`
	AvgReadsPerExec   float64                      `json:"avg_reads_per_exec"`
	Diagnostics       SqlServerWorkloadDiagnostics `json:"diagnostics"`
}

// SqlServerWorkloadTrendPoint represents a single data point in a time-series trend.
type SqlServerWorkloadTrendPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	CPUms         int64     `json:"cpu_ms"`
	Executions    int64     `json:"executions"`
	LogicalReads  int64     `json:"logical_reads"`
	RowsProcessed int64     `json:"rows_processed"`
	MaxGrantKB    int64     `json:"max_grant_kb"`
	MaxDOP        int32     `json:"max_dop"`
	WorstQueryms  int64     `json:"worst_query_ms"`
	AvgCPUms      float64   `json:"avg_cpu_ms"`
	AvgRows       float64   `json:"avg_rows"`
}

// SqlServerWorkloadTopQuery represents a high-impact query identified in a workload interval.
type SqlServerWorkloadTopQuery struct {
	QueryHash            string    `json:"query_hash"`
	StatementFingerprint string    `json:"statement_fingerprint"`
	HashVariantCount     int       `json:"hash_variant_count"`
	QueryText            string    `json:"query_text"`
	DatabaseName    string    `json:"database_name"`
	LoginName       string    `json:"login_name"`
	ProgramName     string    `json:"program_name"`
	TotalCPUms      int64     `json:"total_cpu_ms"`
	TotalExecutions int64     `json:"total_executions"`
	TotalReads      int64     `json:"total_reads"`
	TotalRows        int64     `json:"total_rows"`
	TotalElapsedMs   int64     `json:"total_elapsed_ms"`
	AvgCPUms         float64   `json:"avg_cpu_ms"`
	AvgElapsedMs     float64   `json:"avg_elapsed_ms"`
	LastSeen         time.Time `json:"last_seen"`
}
