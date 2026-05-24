// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_queries.go
// Purpose: Typed Parquet schemas for SQL Server lock history and long-running query snapshots.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerLockRow is the Parquet schema for sqlserver_lock_history.
type SQLServerLockRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	DatabaseName       string `parquet:"name=database_name,       type=BYTE_ARRAY, converted=STRING"`
	TotalLocks         int64  `parquet:"name=total_locks,          type=INT64"`
	Deadlocks          int64  `parquet:"name=deadlocks,            type=INT64"`
}

// SQLServerLongRunningQueryRow is the Parquet schema for sqlserver_long_running_queries.
type SQLServerLongRunningQueryRow struct {
	CaptureTimestampMs   int64  `parquet:"name=capture_timestamp_ms,    type=INT64"`
	ServerID             string `parquet:"name=server_id,               type=BYTE_ARRAY, converted=STRING"`
	SessionID            int32  `parquet:"name=session_id,              type=INT32"`
	RequestID            int32  `parquet:"name=request_id,              type=INT32"`
	DatabaseName         string `parquet:"name=database_name,           type=BYTE_ARRAY, converted=STRING"`
	LoginName            string `parquet:"name=login_name,              type=BYTE_ARRAY, converted=STRING"`
	HostName             string `parquet:"name=host_name,               type=BYTE_ARRAY, converted=STRING"`
	ProgramName          string `parquet:"name=program_name,            type=BYTE_ARRAY, converted=STRING"`
	QueryHash            int64  `parquet:"name=query_hash,              type=INT64"`
	WaitType             string `parquet:"name=wait_type,               type=BYTE_ARRAY, converted=STRING"`
	BlockingSessionID    int32  `parquet:"name=blocking_session_id,     type=INT32"`
	Status               string `parquet:"name=status,                  type=BYTE_ARRAY, converted=STRING"`
	CPUTimeMs            int64  `parquet:"name=cpu_time_ms,             type=INT64"`
	TotalElapsedTimeMs   int64  `parquet:"name=total_elapsed_time_ms,   type=INT64"`
	Reads                int64  `parquet:"name=reads,                   type=INT64"`
	Writes               int64  `parquet:"name=writes,                  type=INT64"`
	GrantedQueryMemoryMB int32  `parquet:"name=granted_query_memory_mb, type=INT32"`
	RowCount             int64  `parquet:"name=row_count,               type=INT64"`
	PercentComplete      string `parquet:"name=percent_complete,        type=BYTE_ARRAY, converted=STRING"`
}
