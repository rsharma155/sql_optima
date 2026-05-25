// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_cpu.go
// Purpose: Typed Parquet schema for SQL Server CPU utilization history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerCPURow is the Parquet schema for sqlserver_cpu_history.
type SQLServerCPURow struct {
	CaptureTimestampMs   int64   `parquet:"name=capture_timestamp_ms,   type=INT64"`
	ServerID             string  `parquet:"name=server_id,              type=BYTE_ARRAY, converted=STRING"`
	ServerName           string  `parquet:"name=server_name,            type=BYTE_ARRAY, converted=STRING"`
	SQLCPUUtilization    float64 `parquet:"name=sql_cpu_utilization,    type=DOUBLE"`
	SystemCPUUtilization float64 `parquet:"name=system_cpu_utilization, type=DOUBLE"`
	IdleCPU              float64 `parquet:"name=idle_cpu,               type=DOUBLE"`
	SchedulerCount       int32   `parquet:"name=scheduler_count,        type=INT32"`
}
