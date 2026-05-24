// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_wait.go
// Purpose: Typed Parquet schema for SQL Server wait statistics history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerWaitRow is the Parquet schema for sqlserver_wait_history.
type SQLServerWaitRow struct {
	CaptureTimestampMs int64   `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string  `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	WaitType           string  `parquet:"name=wait_type,            type=BYTE_ARRAY, converted=STRING"`
	WaitingTasksCount  int64   `parquet:"name=waiting_tasks_count,  type=INT64"`
	WaitTimeMsDelta    float64 `parquet:"name=wait_time_ms_delta,   type=DOUBLE"`
	SignalWaitMsDelta  float64 `parquet:"name=signal_wait_ms_delta, type=DOUBLE"`
}
