// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration_postgres.go
// Purpose: Cold storage export registrations for core PostgreSQL metric tables:
//          wait events, DB IO stats, PGSS delta, query wait profile.
//          Activity, backup, and compliance tables are in
//          registration_postgres_activity.go.
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

// registerPostgres adds all PostgreSQL metric tables to the cold storage exporter.
func registerPostgres(e *Exporter) {
	registerPGWaitEventStats(e)
	registerPGDBIOStats(e)
	registerPGSSDelta(e)
	registerPGQueryWaitProfile(e)
	registerPostgresActivity(e)
}

func registerPGWaitEventStats(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "postgres_wait_event_stats",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					wait_event_type, wait_event, count,
					COALESCE(total_wait_ms, 0)
				FROM postgres_wait_event_stats
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
				var r schemas.PGWaitEventRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.WaitEventType, &r.WaitEvent, &r.Count, &r.TotalWaitMs,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGWaitEventRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGWaitEventRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGDBIOStats(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "postgres_db_io_stats",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					database_name, blks_read, blks_hit, temp_files, temp_bytes
				FROM postgres_db_io_stats
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
				var r schemas.PGDBIOStatsRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.DatabaseName, &r.BlksRead, &r.BlksHit, &r.TempFiles, &r.TempBytes,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGDBIOStatsRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGDBIOStatsRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGSSDelta(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "pgss_delta_1m",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					query_id,
					COALESCE(db_name, ''),
					COALESCE(username, ''),
					COALESCE(app_name, ''),
					COALESCE(query_type, ''),
					calls, total_exec_time, rows,
					shared_blks_hit, shared_blks_read, temp_blks_written,
					COALESCE(wal_bytes, 0),
					total_plan_time, mean_exec_time
				FROM pgss_delta_1m
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
				var r schemas.PGSSDeltaRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.QueryID, &r.DBName, &r.Username, &r.AppName, &r.QueryType,
					&r.Calls, &r.TotalExecTime, &r.Rows,
					&r.SharedBlksHit, &r.SharedBlksRead, &r.TempBlksWritten,
					&r.WALBytes, &r.TotalPlanTime, &r.MeanExecTime,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGSSDeltaRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGSSDeltaRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGQueryWaitProfile(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_query_wait_profile_ts",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(queryid, 0),
					COALESCE(calls, 0),
					COALESCE(total_exec_time, 0),
					COALESCE(mean_exec_time, 0),
					COALESCE(rows, 0),
					COALESCE(shared_blks_hit, 0),
					COALESCE(shared_blks_read, 0),
					COALESCE(temp_blks_written, 0),
					COALESCE(query, ''),
					COALESCE(usename, '')
				FROM monitor.pg_query_wait_profile_ts
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
				var r schemas.PGQueryWaitProfileRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.QueryID, &r.Calls, &r.TotalExecTime, &r.MeanExecTime, &r.Rows,
					&r.SharedBlksHit, &r.SharedBlksRead, &r.TempBlksWritten,
					&r.Query, &r.Username,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGQueryWaitProfileRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGQueryWaitProfileRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}
