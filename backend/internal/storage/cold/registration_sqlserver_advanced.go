// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration_sqlserver_advanced.go
// Purpose: Cold storage export registrations for advanced SQL Server diagnostic tables:
//          Query Store snapshot/interval, latch waits, spinlock stats,
//          memory grant waiters.
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

// registerSQLServerAdvanced adds advanced SQL Server diagnostic tables.
// Called by registerSQLServerQueries.
func registerSQLServerAdvanced(e *Exporter) {
	registerSQLServerQueryStoreSnapshot(e)
	registerSQLServerQueryStoreInterval(e)
	registerSQLServerLatchWaits(e)
	registerSQLServerSpinlockStats(e)
	registerSQLServerMemoryGrantWaiters(e)
}

func registerSQLServerQueryStoreSnapshot(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:            "monitor.sqlserver_query_store_snapshot",
		Engine:               "sqlserver",
		TimestampColumn:      "capture_timestamp",
		ServerIDColumn:       "server_id",
		SkipCompressionCheck: true, // snapshot table in monitor schema — not a compressed hypertable
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					database_name, query_hash, query_text, plan_id,
					runtime_stats_interval_id, total_executions,
					total_cpu_ms, total_duration_ms, total_logical_reads
				FROM monitor.sqlserver_query_store_snapshot
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
				var r schemas.SQLServerQSYSnapshotRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.DatabaseName, &r.QueryHash, &r.QueryText, &r.PlanID,
					&r.RuntimeStatsID, &r.TotalExecutions,
					&r.TotalCPUMs, &r.TotalDurationMs, &r.TotalLogicalReads,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerQSYSnapshotRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerQSYSnapshotRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerQueryStoreInterval(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.sqlserver_query_store_interval",
		Engine:          "sqlserver",
		TimestampColumn: "bucket_start",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM bucket_start)::BIGINT * 1000,
					EXTRACT(EPOCH FROM bucket_end)::BIGINT * 1000,
					server_id::TEXT,
					database_name, query_hash, query_text, plan_id,
					runtime_stats_interval_id,
					delta_executions, delta_cpu_ms, delta_duration_ms, delta_logical_reads
				FROM monitor.sqlserver_query_store_interval
				WHERE server_id = $1::UUID
				  AND bucket_start >= $2
				  AND bucket_start <  $3
				ORDER BY bucket_start
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.SQLServerQSYIntervalRow
				if err := rows.Scan(
					&r.BucketStartMs, &r.BucketEndMs, &r.ServerID,
					&r.DatabaseName, &r.QueryHash, &r.QueryText, &r.PlanID,
					&r.RuntimeStatsID,
					&r.DeltaExecutions, &r.DeltaCPUMs, &r.DeltaDurationMs, &r.DeltaLogicalReads,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerQSYIntervalRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerQSYIntervalRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerLatchWaits(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_latch_waits",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					wait_type, waiting_tasks_count, wait_time_ms, signal_wait_time_ms
				FROM sqlserver_latch_waits
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
				var r schemas.SQLServerLatchRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.WaitType, &r.WaitingTasksCount, &r.WaitTimeMs, &r.SignalWaitTimeMs,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerLatchRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerLatchRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerSpinlockStats(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_spinlock_stats",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					spinlock_type, collisions, spins, sleep_time_ms
				FROM sqlserver_spinlock_stats
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
				var r schemas.SQLServerSpinlockRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.SpinlockType, &r.Collisions, &r.Spins, &r.SleepTimeMs,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerSpinlockRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerSpinlockRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerMemoryGrantWaiters(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_memory_grant_waiters",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					session_id, request_id, database_name, login_name,
					requested_memory_kb, granted_memory_kb, required_memory_kb,
					wait_time_ms, dop, query_text
				FROM sqlserver_memory_grant_waiters
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
				var r schemas.SQLServerMemoryGrantWaiterRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.SessionID, &r.RequestID, &r.DatabaseName, &r.LoginName,
					&r.RequestedMemoryKB, &r.GrantedMemoryKB, &r.RequiredMemoryKB,
					&r.WaitTimeMs, &r.DOP, &r.QueryText,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerMemoryGrantWaiterRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerMemoryGrantWaiterRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}
