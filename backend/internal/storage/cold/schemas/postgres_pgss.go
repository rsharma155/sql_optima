// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/postgres_pgss.go
// Purpose: Typed Parquet schemas for PostgreSQL pg_stat_statements (Query Performance) metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// PGSSDeltaRow is the Parquet schema for pgss_delta_1m.
type PGSSDeltaRow struct {
	CaptureTimestampMs int64   `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string  `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	QueryID            int64   `parquet:"name=query_id,             type=INT64"`
	DBName             string  `parquet:"name=db_name,              type=BYTE_ARRAY, converted=STRING"`
	Username           string  `parquet:"name=username,             type=BYTE_ARRAY, converted=STRING"`
	AppName            string  `parquet:"name=app_name,             type=BYTE_ARRAY, converted=STRING"`
	QueryType          string  `parquet:"name=query_type,           type=BYTE_ARRAY, converted=STRING"`
	Calls              int64   `parquet:"name=calls,                type=INT64"`
	TotalExecTime      float64 `parquet:"name=total_exec_time,      type=DOUBLE"`
	Rows               int64   `parquet:"name=rows,                 type=INT64"`
	SharedBlksHit      int64   `parquet:"name=shared_blks_hit,      type=INT64"`
	SharedBlksRead     int64   `parquet:"name=shared_blks_read,     type=INT64"`
	TempBlksWritten    int64   `parquet:"name=temp_blks_written,    type=INT64"`
	WALBytes           float64 `parquet:"name=wal_bytes,            type=DOUBLE"`
	TotalPlanTime      float64 `parquet:"name=total_plan_time,      type=DOUBLE"`
	MeanExecTime       float64 `parquet:"name=mean_exec_time,       type=DOUBLE"`
}

// PGSSQueryDimRow is the Parquet schema for pgss_query_dim.
type PGSSQueryDimRow struct {
	ServerID   string `parquet:"name=server_id,  type=BYTE_ARRAY, converted=STRING"`
	QueryID    int64  `parquet:"name=query_id,   type=INT64"`
	QueryText  string `parquet:"name=query_text, type=BYTE_ARRAY, converted=STRING"`
	DBName     string `parquet:"name=db_name,    type=BYTE_ARRAY, converted=STRING"`
	Username   string `parquet:"name=username,   type=BYTE_ARRAY, converted=STRING"`
	AppName    string `parquet:"name=app_name,   type=BYTE_ARRAY, converted=STRING"`
	QueryType  string `parquet:"name=query_type, type=BYTE_ARRAY, converted=STRING"`
	FirstSeenMs int64 `parquet:"name=first_seen, type=INT64"`
	LastSeenMs  int64 `parquet:"name=last_seen,  type=INT64"`
}
