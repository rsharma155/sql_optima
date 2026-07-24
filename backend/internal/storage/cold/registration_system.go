// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration_system.go
// Purpose: Cold storage export registrations for system and metadata tables
//          (SQL Server job metrics, PostgreSQL settings snapshot,
//           PostgreSQL roles snapshot, collector execution runs).
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

// registerSystem adds system and metadata tables to the cold storage exporter.
func registerSystem(e *Exporter) {
	registerSQLServerJobMetrics(e)
	registerPGSettingsSnapshot(e)
	registerPGRolesSnapshot(e)
	registerCollectorRuns(e)
}

func registerSQLServerJobMetrics(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_job_metrics",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(total_jobs, 0),
					COALESCE(enabled_jobs, 0),
					COALESCE(disabled_jobs, 0),
					COALESCE(running_jobs, 0),
					COALESCE(failed_jobs_24h, 0),
					COALESCE(critical_jobs_disabled, 0),
					COALESCE(error_message, '')
				FROM sqlserver_job_metrics
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
				var r schemas.SQLServerJobMetricsRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.TotalJobs, &r.EnabledJobs, &r.DisabledJobs, &r.RunningJobs,
					&r.FailedJobs24h, &r.CriticalJobsDisabled, &r.ErrorMessage,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerJobMetricsRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerJobMetricsRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGSettingsSnapshot(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "postgres_settings_snapshot",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					name,
					COALESCE(setting, ''),
					COALESCE(unit, ''),
					COALESCE(source, '')
				FROM postgres_settings_snapshot
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
				var r schemas.PGSettingsSnapshotRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.Name, &r.Setting, &r.Unit, &r.Source,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGSettingsSnapshotRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGSettingsSnapshotRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerPGRolesSnapshot(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_roles_snapshot",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					rolname, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolcanlogin
				FROM monitor.pg_roles_snapshot
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
				var r schemas.PGRolesSnapshotRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.RolName, &r.RolSuper, &r.RolCreateDB,
					&r.RolCreateRole, &r.RolReplication, &r.RolCanLogin,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.PGRolesSnapshotRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.PGRolesSnapshotRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerCollectorRuns(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.collector_runs",
		Engine:          "system",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					run_id, collector_name, server_id::TEXT,
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					COALESCE(EXTRACT(EPOCH FROM end_time)::BIGINT * 1000, 0),
					status, rows_inserted,
					COALESCE(error_message, ''),
					duration_ms
				FROM monitor.collector_runs
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
				var r schemas.CollectorRunRow
				if err := rows.Scan(
					&r.RunID, &r.CollectorName, &r.ServerID,
					&r.CaptureTimestampMs, &r.EndTimeMs,
					&r.Status, &r.RowsInserted, &r.ErrorMessage, &r.DurationMs,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.CollectorRunRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.CollectorRunRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}
