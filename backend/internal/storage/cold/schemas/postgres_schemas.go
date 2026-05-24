// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/postgres_schemas.go
// Purpose: Typed Parquet schemas for core PostgreSQL metrics (Waits and DB I/O).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// PGWaitEventRow is the Parquet schema for postgres_wait_event_stats.
type PGWaitEventRow struct {
	CaptureTimestampMs int64   `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string  `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	WaitEventType      string  `parquet:"name=wait_event_type,      type=BYTE_ARRAY, converted=STRING"`
	WaitEvent          string  `parquet:"name=wait_event,           type=BYTE_ARRAY, converted=STRING"`
	Count              int64   `parquet:"name=count,                type=INT64"`
	TotalWaitMs        float64 `parquet:"name=total_wait_ms,        type=DOUBLE"`
}

// PGDBIOStatsRow is the Parquet schema for postgres_db_io_stats.
type PGDBIOStatsRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	DatabaseName       string `parquet:"name=database_name,        type=BYTE_ARRAY, converted=STRING"`
	BlksRead           int64  `parquet:"name=blks_read,            type=INT64"`
	BlksHit            int64  `parquet:"name=blks_hit,             type=INT64"`
	TempFiles          int64  `parquet:"name=temp_files,           type=INT64"`
	TempBytes          int64  `parquet:"name=temp_bytes,           type=INT64"`
}
