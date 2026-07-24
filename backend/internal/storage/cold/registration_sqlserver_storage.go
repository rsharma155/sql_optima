// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration_sqlserver_storage.go
// Purpose: Cold storage export registrations for SQL Server storage-layer tables:
//          memory history (PLE), memory metrics, disk history, database throughput.
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

// registerSQLServerStorage adds SQL Server memory and disk tables.
// Called by registerSQLServerCore.
func registerSQLServerStorage(e *Exporter) {
	registerSQLServerMemoryHistory(e)
	registerSQLServerMemoryMetrics(e)
	registerSQLServerDiskHistory(e)
	registerSQLServerDatabaseThroughput(e)
}

func registerSQLServerMemoryHistory(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_memory_history",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(page_life_expectancy, 0)
				FROM sqlserver_memory_history
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
				var r schemas.SQLServerMemoryHistoryRow
				if err := rows.Scan(&r.CaptureTimestampMs, &r.ServerID, &r.PageLifeExpectancy); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerMemoryHistoryRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerMemoryHistoryRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerMemoryMetrics(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_memory_metrics",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(sql_memory_used_mb, 0),
					COALESCE(sql_memory_target_mb, 0),
					COALESCE(os_total_memory_mb, 0),
					COALESCE(os_available_memory_mb, 0),
					COALESCE(process_physical_low, false),
					COALESCE(process_virtual_low, false),
					COALESCE(memory_grants_pending, 0),
					COALESCE(active_memory_grants, 0),
					COALESCE(waiting_memory_grants, 0),
					COALESCE(granted_workspace_mb, 0),
					COALESCE(requested_workspace_mb, 0),
					COALESCE(ple_seconds, 0),
					COALESCE(plan_cache_mb, 0),
					COALESCE(sort_warnings_total, 0),
					COALESCE(hash_warnings_total, 0),
					COALESCE(sort_warnings_per_sec, 0),
					COALESCE(hash_warnings_per_sec, 0)
				FROM sqlserver_memory_metrics
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
				var r schemas.SQLServerMemoryMetricsRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.SQLMemoryUsedMB, &r.SQLMemoryTargetMB,
					&r.OSTotalMemoryMB, &r.OSAvailableMemoryMB,
					&r.ProcessPhysicalLow, &r.ProcessVirtualLow,
					&r.MemoryGrantsPending, &r.ActiveMemoryGrants, &r.WaitingMemoryGrants,
					&r.GrantedWorkspaceMB, &r.RequestedWorkspaceMB,
					&r.PLESeconds, &r.PlanCacheMB,
					&r.SortWarningsTotal, &r.HashWarningsTotal,
					&r.SortWarningsPerSec, &r.HashWarningsPerSec,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerMemoryMetricsRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerMemoryMetricsRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerDiskHistory(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_disk_history",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					database_name, data_mb, log_mb, free_mb, delta_data_mb, delta_log_mb
				FROM sqlserver_disk_history
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
				var r schemas.SQLServerDiskRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.DatabaseName, &r.DataMB, &r.LogMB, &r.FreeMB,
					&r.DeltaDataMB, &r.DeltaLogMB,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerDiskRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerDiskRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}

func registerSQLServerDatabaseThroughput(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_database_throughput",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					database_name,
					user_seeks, user_scans, user_lookups, user_writes,
					total_reads, total_writes, tps, batch_requests_per_sec,
					reads, writes, bytes_read, bytes_written,
					read_latency_ms, write_latency_ms
				FROM sqlserver_database_throughput
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
				var r schemas.SQLServerThroughputRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID, &r.DatabaseName,
					&r.UserSeeks, &r.UserScans, &r.UserLookups, &r.UserWrites,
					&r.TotalReads, &r.TotalWrites, &r.TPS, &r.BatchRequests,
					&r.Reads, &r.Writes, &r.BytesRead, &r.BytesWritten,
					&r.ReadLatencyMs, &r.WriteLatencyMs,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerThroughputRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerThroughputRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})
}
