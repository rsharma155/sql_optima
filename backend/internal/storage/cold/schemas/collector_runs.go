// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/collector_runs.go
// Purpose: Typed Parquet schema for background collector execution history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// CollectorRunRow is the Parquet schema for monitor.collector_runs.
type CollectorRunRow struct {
	RunID              int64  `parquet:"name=run_id,               type=INT64"`
	CollectorName      string `parquet:"name=collector_name,       type=BYTE_ARRAY, converted=STRING"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	EndTimeMs          int64  `parquet:"name=end_time_ms,          type=INT64"`
	Status             string `parquet:"name=status,               type=BYTE_ARRAY, converted=STRING"`
	RowsInserted       int32  `parquet:"name=rows_inserted,        type=INT32"`
	ErrorMessage       string `parquet:"name=error_message,        type=BYTE_ARRAY, converted=STRING"`
	DurationMs         int32  `parquet:"name=duration_ms,          type=INT32"`
}
