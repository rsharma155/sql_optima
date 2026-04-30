// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Domain models for SQL Server Workload Observability.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package domain

import "time"

// SqlServerWorkloadSummary represents the top-level KPI metrics for a given period.
type SqlServerWorkloadSummary struct {
	TotalCPUms       int64   `json:"total_cpu_ms"`
	TotalExecutions  int64   `json:"total_executions"`
	TotalLogicalReads int64  `json:"total_logical_reads"`
	TotalRows        int64   `json:"total_rows"`
	MaxMemoryGrantKB int64   `json:"max_memory_grant_kb"`
	AvgCPUPerExec    float64 `json:"avg_cpu_per_exec"`
	AvgReadsPerExec  float64 `json:"avg_reads_per_exec"`
}

// SqlServerWorkloadTrendPoint represents a single data point in a time-series trend.
type SqlServerWorkloadTrendPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	CPUms          int64     `json:"cpu_ms"`
	Executions     int64     `json:"executions"`
	LogicalReads   int64     `json:"logical_reads"`
	RowsProcessed  int64     `json:"rows_processed"`
	MaxGrantKB     int64     `json:"max_grant_kb"`
	MaxDOP         int32     `json:"max_dop"`
	WorstQueryms   int64     `json:"worst_query_ms"`
	AvgCPUms       float64   `json:"avg_cpu_ms"`
	AvgRows        float64   `json:"avg_rows"`
}

// SqlServerWorkloadTopQuery represents a high-impact query identified in a workload interval.
type SqlServerWorkloadTopQuery struct {
	QueryHash       string  `json:"query_hash"`
	QueryText       string  `json:"query_text"`
	DatabaseName    string  `json:"database_name"`
	LoginName       string  `json:"login_name"`
	ProgramName     string  `json:"program_name"`
	TotalCPUms      int64   `json:"total_cpu_ms"`
	TotalExecutions int64   `json:"total_executions"`
	TotalReads      int64     `json:"total_reads"`
	TotalRows       int64     `json:"total_rows"`
	AvgCPUms        float64   `json:"avg_cpu_ms"`
	LastSeen        time.Time `json:"last_seen"`
}
