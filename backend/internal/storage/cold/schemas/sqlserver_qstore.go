// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_qstore.go
// Purpose: Typed Parquet schemas for SQL Server Query Store snapshots and interval statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerMemoryHistoryRow is the Parquet schema for sqlserver_memory_history.
type SQLServerMemoryHistoryRow struct {
	CaptureTimestampMs int64   `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string  `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	PageLifeExpectancy float64 `parquet:"name=page_life_expectancy, type=DOUBLE"`
}

// SQLServerQSYSnapshotRow is the Parquet schema for monitor.sqlserver_query_store_snapshot.
type SQLServerQSYSnapshotRow struct {
	CaptureTimestampMs    int64   `parquet:"name=capture_timestamp_ms,      type=INT64"`
	ServerID              string  `parquet:"name=server_id,                 type=BYTE_ARRAY, converted=STRING"`
	DatabaseName          string  `parquet:"name=database_name,              type=BYTE_ARRAY, converted=STRING"`
	QueryHash             int64   `parquet:"name=query_hash,                type=INT64"`
	QueryText             string  `parquet:"name=query_text,                type=BYTE_ARRAY, converted=STRING"`
	PlanID                int64   `parquet:"name=plan_id,                   type=INT64"`
	RuntimeStatsID        int64   `parquet:"name=runtime_stats_id,          type=INT64"`
	TotalExecutions       int64   `parquet:"name=total_executions,          type=INT64"`
	TotalCPUMs            float64 `parquet:"name=total_cpu_ms,              type=DOUBLE"`
	TotalDurationMs       float64 `parquet:"name=total_duration_ms,         type=DOUBLE"`
	TotalLogicalReads     float64 `parquet:"name=total_logical_reads,       type=DOUBLE"`
}

// SQLServerQSYIntervalRow is the Parquet schema for monitor.sqlserver_query_store_interval.
type SQLServerQSYIntervalRow struct {
	BucketStartMs     int64   `parquet:"name=bucket_start_ms,     type=INT64"`
	BucketEndMs       int64   `parquet:"name=bucket_end_ms,       type=INT64"`
	ServerID          string  `parquet:"name=server_id,           type=BYTE_ARRAY, converted=STRING"`
	DatabaseName      string  `parquet:"name=database_name,       type=BYTE_ARRAY, converted=STRING"`
	QueryHash         int64   `parquet:"name=query_hash,          type=INT64"`
	QueryText         string  `parquet:"name=query_text,          type=BYTE_ARRAY, converted=STRING"`
	PlanID            int64   `parquet:"name=plan_id,             type=INT64"`
	RuntimeStatsID    int64   `parquet:"name=runtime_stats_id,    type=INT64"`
	DeltaExecutions   int64   `parquet:"name=delta_executions,    type=INT64"`
	DeltaCPUMs         float64 `parquet:"name=delta_cpu_ms,         type=DOUBLE"`
	DeltaDurationMs    float64 `parquet:"name=delta_duration_ms,    type=DOUBLE"`
	DeltaLogicalReads  float64 `parquet:"name=delta_logical_reads, type=DOUBLE"`
}
