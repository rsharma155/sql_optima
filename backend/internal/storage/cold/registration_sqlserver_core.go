// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration_sqlserver_core.go
// Purpose: Cold storage export registrations for core SQL Server high-frequency
//          metrics: CPU history, wait history, core metrics, connections, lock history.
//          Memory, disk, and throughput tables are in registration_sqlserver_storage.go.
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

// registerSQLServerCore adds the high-frequency SQL Server core metric tables
// to the cold storage exporter.
func registerSQLServerCore(e *Exporter) {
	registerSQLServerCPUHistory(e)
	registerSQLServerWaitHistory(e)
	registerSQLServerMetrics(e)
	registerSQLServerConnectionHistory(e)
	registerSQLServerLockHistory(e)
	registerSQLServerStorage(e)
}

func registerSQLServerCPUHistory(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_cpu_history",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM h.capture_timestamp)::BIGINT * 1000  AS capture_timestamp_ms,
					h.server_id::TEXT,
					ms.server_name,
					COALESCE(h.sql_process, 0),
					COALESCE(h.other_process, 0),
					COALESCE(h.system_idle, 0),
					0
				FROM sqlserver_cpu_history h
				JOIN optima_servers ms ON ms.id = h.server_id
				WHERE h.server_id = $1::UUID
				  AND h.capture_timestamp >= $2
				  AND h.capture_timestamp <  $3
				ORDER BY h.capture_timestamp
				LIMIT  $4
				OFFSET $5`

			rows, err := db.Query(ctx, q, serverID, from, to, limit, offset)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var res []any
			for rows.Next() {
				var r schemas.SQLServerCPURow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID, &r.ServerName,
					&r.SQLCPUUtilization, &r.SystemCPUUtilization, &r.IdleCPU, &r.SchedulerCount,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerCPURow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerCPURow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerWaitHistory(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_wait_history",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(disk_read_ms_per_sec, 0),
					COALESCE(blocking_ms_per_sec, 0),
					COALESCE(parallelism_ms_per_sec, 0),
					COALESCE(other_ms_per_sec, 0)
				FROM sqlserver_wait_history
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
				var r schemas.SQLServerWaitRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.DiskReadMsPerSec, &r.BlockingMsPerSec,
					&r.ParallelismMsPerSec, &r.OtherMsPerSec,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerWaitRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerWaitRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerMetrics(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_metrics",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(avg_cpu_load, 0),
					COALESCE(memory_usage, 0),
					COALESCE(active_users, 0),
					COALESCE(total_locks, 0),
					COALESCE(deadlocks, 0),
					COALESCE(data_disk_mb, 0),
					COALESCE(log_disk_mb, 0),
					COALESCE(free_disk_mb, 0)
				FROM sqlserver_metrics
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
				var r schemas.SQLServerMetricsRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.AvgCPULoad, &r.MemoryUsage, &r.ActiveUsers,
					&r.TotalLocks, &r.Deadlocks,
					&r.DataDiskMB, &r.LogDiskMB, &r.FreeDiskMB,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerMetricsRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerMetricsRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerConnectionHistory(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_connection_history",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(login_name, ''),
					COALESCE(database_name, ''),
					COALESCE(active_connections, 0),
					COALESCE(active_requests, 0)
				FROM sqlserver_connection_history
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
				var r schemas.SQLServerConnectionRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.LoginName, &r.DatabaseName,
					&r.ActiveConnections, &r.ActiveRequests,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerConnectionRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerConnectionRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerLockHistory(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_lock_history",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(database_name, ''),
					COALESCE(total_locks, 0),
					COALESCE(deadlocks, 0)
				FROM sqlserver_lock_history
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
				var r schemas.SQLServerLockRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.DatabaseName, &r.TotalLocks, &r.Deadlocks,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerLockRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerLockRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}
