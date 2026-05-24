// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_proc.go
// Purpose: Typed Parquet schema for SQL Server stored procedure execution statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerProcedureStatsRow is the Parquet schema for sqlserver_procedure_stats.
type SQLServerProcedureStatsRow struct {
	CaptureTimestampMs   int64   `parquet:"name=capture_timestamp_ms,    type=INT64"`
	ServerID             string  `parquet:"name=server_id,               type=BYTE_ARRAY, converted=STRING"`
	DatabaseName         string  `parquet:"name=database_name,           type=BYTE_ARRAY, converted=STRING"`
	SchemaName           string  `parquet:"name=schema_name,             type=BYTE_ARRAY, converted=STRING"`
	ObjectName           string  `parquet:"name=object_name,             type=BYTE_ARRAY, converted=STRING"`
	QueryHash            int64   `parquet:"name=query_hash,              type=INT64"`
	ExecutionCount       int64   `parquet:"name=execution_count,         type=INT64"`
	TotalWorkerTimeMs    float64 `parquet:"name=total_worker_time_ms,    type=DOUBLE"`
	TotalElapsedTimeMs   float64 `parquet:"name=total_elapsed_time_ms,   type=DOUBLE"`
	TotalLogicalReads    int64   `parquet:"name=total_logical_reads,     type=INT64"`
	TotalPhysicalReads   int64   `parquet:"name=total_physical_reads,    type=INT64"`
}
