// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Typed Parquet schema for SQL Server wait category rate history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package schemas

// SQLServerWaitRow is the Parquet schema for sqlserver_wait_history (category rates).
type SQLServerWaitRow struct {
	CaptureTimestampMs  int64   `parquet:"name=capture_timestamp_ms,     type=INT64"`
	ServerID            string  `parquet:"name=server_id,               type=BYTE_ARRAY, converted=STRING"`
	DiskReadMsPerSec    float64 `parquet:"name=disk_read_ms_per_sec,    type=DOUBLE"`
	BlockingMsPerSec    float64 `parquet:"name=blocking_ms_per_sec,     type=DOUBLE"`
	ParallelismMsPerSec float64 `parquet:"name=parallelism_ms_per_sec,  type=DOUBLE"`
	OtherMsPerSec       float64 `parquet:"name=other_ms_per_sec,        type=DOUBLE"`
}
