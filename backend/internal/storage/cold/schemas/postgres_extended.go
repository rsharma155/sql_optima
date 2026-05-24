// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/postgres_extended.go
// Purpose: Typed Parquet schemas for extended PostgreSQL metrics (Backups, Logins, DDL, Sessions, Load).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// PGBackupArchiverRow is the Parquet schema for monitor.pg_backup_archiver_ts.
type PGBackupArchiverRow struct {
	CaptureTimestampMs int64 `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	ArchivedCount      int64 `parquet:"name=archived_count,       type=INT64"`
	FailedCount        int64 `parquet:"name=failed_count,         type=INT64"`
	LastArchivedMs     int64 `parquet:"name=last_archived_ms,     type=INT64"`
	LastFailedMs       int64 `parquet:"name=last_failed_ms,       type=INT64"`
}

// PGBasebackupHistoryRow is the Parquet schema for monitor.pg_basebackup_history.
type PGBasebackupHistoryRow struct {
	CaptureTimestampMs   int64   `parquet:"name=capture_timestamp_ms,    type=INT64"`
	ServerID             string  `parquet:"name=server_id,               type=BYTE_ARRAY, converted=STRING"`
	CheckpointTimeMs     int64   `parquet:"name=checkpoint_time_ms,      type=INT64"`
	CheckpointWriteTime  float64 `parquet:"name=checkpoint_write_time,   type=DOUBLE"`
}

// PGFailedLoginRow is the Parquet schema for monitor.pg_failed_login_events.
type PGFailedLoginRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	Username           string `parquet:"name=username,             type=BYTE_ARRAY, converted=STRING"`
	ClientAddr         string `parquet:"name=client_addr,          type=BYTE_ARRAY, converted=STRING"`
	Message            string `parquet:"name=message,              type=BYTE_ARRAY, converted=STRING"`
}

// PGQueryWaitProfileRow is the Parquet schema for monitor.pg_query_wait_profile_ts.
type PGQueryWaitProfileRow struct {
	CaptureTimestampMs int64   `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string  `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	QueryID            int64   `parquet:"name=query_id,             type=INT64"`
	Calls              int64   `parquet:"name=calls,                type=INT64"`
	TotalExecTime      float64 `parquet:"name=total_exec_time,      type=DOUBLE"`
	MeanExecTime       float64 `parquet:"name=mean_exec_time,       type=DOUBLE"`
	Rows               int64   `parquet:"name=rows,                 type=INT64"`
	SharedBlksHit      int64   `parquet:"name=shared_blks_hit,      type=INT64"`
	SharedBlksRead     int64   `parquet:"name=shared_blks_read,     type=INT64"`
	TempBlksWritten    int64   `parquet:"name=temp_blks_written,    type=INT64"`
	Query              string  `parquet:"name=query,                type=BYTE_ARRAY, converted=STRING"`
	Username           string  `parquet:"name=username,             type=BYTE_ARRAY, converted=STRING"`
}

// PGDDLActivityRow is the Parquet schema for monitor.pg_ddl_activity_ts.
type PGDDLActivityRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	SchemaName         string `parquet:"name=schema_name,          type=BYTE_ARRAY, converted=STRING"`
	RelName            string `parquet:"name=rel_name,             type=BYTE_ARRAY, converted=STRING"`
	NTupIns            int64  `parquet:"name=n_tup_ins,            type=INT64"`
	NTupUpd            int64  `parquet:"name=n_tup_upd,            type=INT64"`
	NTupDel            int64  `parquet:"name=n_tup_del,            type=INT64"`
}

// PGSessionActivityRow is the Parquet schema for monitor.pg_session_activity_ts.
type PGSessionActivityRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	DBName             string `parquet:"name=dbname,               type=BYTE_ARRAY, converted=STRING"`
	PID                int32  `parquet:"name=pid,                  type=INT32"`
	Username           string `parquet:"name=usename,              type=BYTE_ARRAY, converted=STRING"`
	AppName            string `parquet:"name=application_name,     type=BYTE_ARRAY, converted=STRING"`
	ClientAddr         string `parquet:"name=client_addr,          type=BYTE_ARRAY, converted=STRING"`
	State              string `parquet:"name=state,                type=BYTE_ARRAY, converted=STRING"`
	WaitEventType      string `parquet:"name=wait_event_type,      type=BYTE_ARRAY, converted=STRING"`
	WaitEvent          string `parquet:"name=wait_event,           type=BYTE_ARRAY, converted=STRING"`
	BackendType        string `parquet:"name=backend_type,         type=BYTE_ARRAY, converted=STRING"`
	QueryID            int64  `parquet:"name=query_id,             type=INT64"`
	Query              string `parquet:"name=query,                type=BYTE_ARRAY, converted=STRING"`
	XactStartMs        int64  `parquet:"name=xact_start_ms,        type=INT64"`
	QueryStartMs       int64  `parquet:"name=query_start_ms,       type=INT64"`
	StateChangeMs      int64  `parquet:"name=state_change_ms,      type=INT64"`
	BackendStartMs     int64  `parquet:"name=backend_start_ms,     type=INT64"`
}

// PGWaitEventSummaryRow is the Parquet schema for monitor.pg_wait_event_summary_ts.
type PGWaitEventSummaryRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	WaitEventType      string `parquet:"name=wait_event_type,      type=BYTE_ARRAY, converted=STRING"`
	WaitEvent          string `parquet:"name=wait_event,           type=BYTE_ARRAY, converted=STRING"`
	Sessions           int32  `parquet:"name=sessions,             type=INT32"`
	State              string `parquet:"name=state,                type=BYTE_ARRAY, converted=STRING"`
}

// PGDBLoadRow is the Parquet schema for monitor.pg_db_load_ts.
type PGDBLoadRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	ActiveSessions     int32  `parquet:"name=active_sessions,      type=INT32"`
	CPUSessions        int32  `parquet:"name=cpu_sessions,         type=INT32"`
	WaitingSessions    int32  `parquet:"name=waiting_sessions,     type=INT32"`
	IOSessions         int32  `parquet:"name=io_sessions,          type=INT32"`
	LockSessions       int32  `parquet:"name=lock_sessions,        type=INT32"`
	IdleInTxn          int32  `parquet:"name=idle_in_txn,          type=INT32"`
}
