// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration_postgres_activity.go
// Purpose: Cold storage export registrations for PostgreSQL session-activity,
//          compliance, and backup tables: session activity, wait event summary,
//          DB load, DDL activity, backup archiver, basebackup history, failed logins.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/cold/schemas"
)

// registerPostgresActivity adds PostgreSQL session-activity and compliance tables.
// Called by registerPostgres.
func registerPostgresActivity(e *Exporter) {
	registerPGSessionActivity(e)
	registerPGWaitEventSummary(e)
	registerPGDBLoad(e)
	registerPGDDLActivity(e)
	registerPGBackupArchiver(e)
	registerPGBasebackupHistory(e)
	registerPGFailedLoginEvents(e)
}

func registerPGSessionActivity(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_session_activity_ts",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(dbname, ''),
					COALESCE(pid, 0),
					COALESCE(usename, ''),
					COALESCE(application_name, ''),
					COALESCE(client_addr::text, ''),
					COALESCE(state, ''),
					COALESCE(wait_event_type, ''),
					COALESCE(wait_event, ''),
					COALESCE(backend_type, ''),
					COALESCE(query_id, 0),
					COALESCE(query, ''),
					COALESCE(EXTRACT(EPOCH FROM xact_start)::BIGINT * 1000, 0),
					COALESCE(EXTRACT(EPOCH FROM query_start)::BIGINT * 1000, 0),
					COALESCE(EXTRACT(EPOCH FROM state_change)::BIGINT * 1000, 0),
					COALESCE(EXTRACT(EPOCH FROM backend_start)::BIGINT * 1000, 0)
				FROM monitor.pg_session_activity_ts
				WHERE server_id = $1::UUID
				  AND capture_timestamp >= $2
				  AND capture_timestamp <  $3
				ORDER BY capture_timestamp
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.PGSessionActivityRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.DBName, &r.PID, &r.Username, &r.AppName, &r.ClientAddr,
					&r.State, &r.WaitEventType, &r.WaitEvent, &r.BackendType,
					&r.QueryID, &r.Query,
					&r.XactStartMs, &r.QueryStartMs, &r.StateChangeMs, &r.BackendStartMs,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGSessionActivityRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGSessionActivityRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGWaitEventSummary(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_wait_event_summary_ts",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(wait_event_type, ''),
					COALESCE(wait_event, ''),
					COALESCE(sessions, 0),
					COALESCE(state, '')
				FROM monitor.pg_wait_event_summary_ts
				WHERE server_id = $1::UUID
				  AND capture_timestamp >= $2
				  AND capture_timestamp <  $3
				ORDER BY capture_timestamp
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.PGWaitEventSummaryRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.WaitEventType, &r.WaitEvent, &r.Sessions, &r.State,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGWaitEventSummaryRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGWaitEventSummaryRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGDBLoad(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_db_load_ts",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(active_sessions, 0),
					COALESCE(cpu_sessions, 0),
					COALESCE(waiting_sessions, 0),
					COALESCE(io_sessions, 0),
					COALESCE(lock_sessions, 0),
					COALESCE(idle_in_txn, 0)
				FROM monitor.pg_db_load_ts
				WHERE server_id = $1::UUID
				  AND capture_timestamp >= $2
				  AND capture_timestamp <  $3
				ORDER BY capture_timestamp
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.PGDBLoadRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.ActiveSessions, &r.CPUSessions, &r.WaitingSessions,
					&r.IOSessions, &r.LockSessions, &r.IdleInTxn,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGDBLoadRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGDBLoadRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGDDLActivity(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_ddl_activity_ts",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(schemaname, ''),
					COALESCE(relname, ''),
					COALESCE(n_tup_ins, 0),
					COALESCE(n_tup_upd, 0),
					COALESCE(n_tup_del, 0)
				FROM monitor.pg_ddl_activity_ts
				WHERE server_id = $1::UUID
				  AND capture_timestamp >= $2
				  AND capture_timestamp <  $3
				ORDER BY capture_timestamp
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.PGDDLActivityRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.SchemaName, &r.RelName,
					&r.NTupIns, &r.NTupUpd, &r.NTupDel,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGDDLActivityRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGDDLActivityRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGBackupArchiver(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_backup_archiver_ts",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(archived_count, 0),
					COALESCE(failed_count, 0),
					COALESCE(EXTRACT(EPOCH FROM last_archived_time)::BIGINT * 1000, 0),
					COALESCE(EXTRACT(EPOCH FROM last_failed_time)::BIGINT * 1000, 0)
				FROM monitor.pg_backup_archiver_ts
				WHERE server_id = $1::UUID
				  AND capture_timestamp >= $2
				  AND capture_timestamp <  $3
				ORDER BY capture_timestamp
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.PGBackupArchiverRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.ArchivedCount, &r.FailedCount, &r.LastArchivedMs, &r.LastFailedMs,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGBackupArchiverRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGBackupArchiverRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGBasebackupHistory(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:            "monitor.pg_basebackup_history",
		Engine:               "postgres",
		TimestampColumn:      "capture_timestamp",
		ServerIDColumn:       "server_id",
		SkipCompressionCheck: true, // event/history table — not a compressed hypertable
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(EXTRACT(EPOCH FROM checkpoint_time)::BIGINT * 1000, 0),
					COALESCE(checkpoint_write_time, 0)
				FROM monitor.pg_basebackup_history
				WHERE server_id = $1::UUID
				  AND capture_timestamp >= $2
				  AND capture_timestamp <  $3
				ORDER BY capture_timestamp
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.PGBasebackupHistoryRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.CheckpointTimeMs, &r.CheckpointWriteTime,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGBasebackupHistoryRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGBasebackupHistoryRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGFailedLoginEvents(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:            "monitor.pg_failed_login_events",
		Engine:               "postgres",
		TimestampColumn:      "capture_timestamp",
		ServerIDColumn:       "server_id",
		SkipCompressionCheck: true, // event table — not a compressed hypertable
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(username, ''),
					COALESCE(client_addr, ''),
					COALESCE(message, '')
				FROM monitor.pg_failed_login_events
				WHERE server_id = $1::UUID
				  AND capture_timestamp >= $2
				  AND capture_timestamp <  $3
				ORDER BY capture_timestamp
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.PGFailedLoginRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.Username, &r.ClientAddr, &r.Message,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGFailedLoginRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGFailedLoginRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}
