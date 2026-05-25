// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_metrics.go
// Purpose: Typed Parquet schemas for SQL Server core metrics and connection history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerMetricsRow is the Parquet schema for sqlserver_metrics.
type SQLServerMetricsRow struct {
	CaptureTimestampMs int64   `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string  `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	AvgCPULoad         float64 `parquet:"name=avg_cpu_load,         type=DOUBLE"`
	MemoryUsage        float64 `parquet:"name=memory_usage,        type=DOUBLE"`
	ActiveUsers        int32   `parquet:"name=active_users,         type=INT32"`
	TotalLocks         int64   `parquet:"name=total_locks,          type=INT64"`
	Deadlocks          int64   `parquet:"name=deadlocks,            type=INT64"`
	DataDiskMB         float64 `parquet:"name=data_disk_mb,         type=DOUBLE"`
	LogDiskMB          float64 `parquet:"name=log_disk_mb,          type=DOUBLE"`
	FreeDiskMB         float64 `parquet:"name=free_disk_mb,         type=DOUBLE"`
}

// SQLServerConnectionRow is the Parquet schema for sqlserver_connection_history.
type SQLServerConnectionRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	LoginName          string `parquet:"name=login_name,           type=BYTE_ARRAY, converted=STRING"`
	DatabaseName       string `parquet:"name=database_name,        type=BYTE_ARRAY, converted=STRING"`
	ActiveConnections  int32  `parquet:"name=active_connections,   type=INT32"`
	ActiveRequests     int32  `parquet:"name=active_requests,      type=INT32"`
}
