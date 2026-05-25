// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_memory.go
// Purpose: Typed Parquet schema for SQL Server core memory history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerMemoryRow is the Parquet schema for sqlserver_memory_history.
type SQLServerMemoryRow struct {
	CaptureTimestampMs   int64   `parquet:"name=capture_timestamp_ms,       type=INT64"`
	ServerID             string  `parquet:"name=server_id,                  type=BYTE_ARRAY, converted=STRING"`
	TotalServerMemoryMB  float64 `parquet:"name=total_server_memory_mb,    type=DOUBLE"`
	TargetServerMemoryMB float64 `parquet:"name=target_server_memory_mb,  type=DOUBLE"`
	SQLCacheMemoryMB     float64 `parquet:"name=sql_cache_memory_mb,       type=DOUBLE"`
	FreeMemoryMB         float64 `parquet:"name=free_memory_mb,            type=DOUBLE"`
	MemoryUtilizationPct float64 `parquet:"name=memory_utilization_pct,   type=DOUBLE"`
}
