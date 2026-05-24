// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration.go
// Purpose: Central registry for mapping TimescaleDB metric tables to their cold storage Parquet schemas and export queries.
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

// RegisterCoreTables adds all high-priority tables to the cold storage exporter.
func RegisterCoreTables(e *Exporter) {
	// 1. SQL Server CPU History
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
					COALESCE(h.sql_cpu_utilization, 0)                      AS sql_cpu_utilization,
					COALESCE(h.system_cpu_utilization, 0)                   AS system_cpu_utilization,
					COALESCE(h.idle_cpu, 0)                                 AS idle_cpu,
					COALESCE(h.scheduler_count, 0)                          AS scheduler_count
				FROM sqlserver_cpu_history h
				JOIN monitored_servers ms ON ms.id = h.server_id
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.ServerName,
					&r.SQLCPUUtilization,
					&r.SystemCPUUtilization,
					&r.IdleCPU,
					&r.SchedulerCount,
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

	// 2. SQL Server Wait History
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
					wait_type,
					waiting_tasks_count,
					COALESCE(wait_time_ms_delta, 0),
					COALESCE(signal_wait_ms_delta, 0)
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.WaitType,
					&r.WaitingTasksCount,
					&r.WaitTimeMsDelta,
					&r.SignalWaitMsDelta,
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

	// 3. SQL Server Core Metrics
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.AvgCPULoad,
					&r.MemoryUsage,
					&r.ActiveUsers,
					&r.TotalLocks,
					&r.Deadlocks,
					&r.DataDiskMB,
					&r.LogDiskMB,
					&r.FreeDiskMB,
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

	// 4. SQL Server Connection History
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.LoginName,
					&r.DatabaseName,
					&r.ActiveConnections,
					&r.ActiveRequests,
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

	// 5. PostgreSQL Wait Event Stats
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
					wait_event_type,
					wait_event,
					count,
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.WaitEventType,
					&r.WaitEvent,
					&r.Count,
					&r.TotalWaitMs,
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

	// 6. SQL Server Disk History
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
					database_name,
					data_mb,
					log_mb,
					free_mb,
					delta_data_mb,
					delta_log_mb
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.DatabaseName,
					&r.DataMB,
					&r.LogMB,
					&r.FreeMB,
					&r.DeltaDataMB,
					&r.DeltaLogMB,
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

	// 7. SQL Server Database Throughput
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
					user_seeks,
					user_scans,
					user_lookups,
					user_writes,
					total_reads,
					total_writes,
					tps,
					batch_requests_per_sec,
					reads,
					writes,
					bytes_read,
					bytes_written,
					read_latency_ms,
					write_latency_ms
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.DatabaseName,
					&r.UserSeeks,
					&r.UserScans,
					&r.UserLookups,
					&r.UserWrites,
					&r.TotalReads,
					&r.TotalWrites,
					&r.TPS,
					&r.BatchRequests,
					&r.Reads,
					&r.Writes,
					&r.BytesRead,
					&r.BytesWritten,
					&r.ReadLatencyMs,
					&r.WriteLatencyMs,
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

	// 8. PostgreSQL DB IO Stats
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
					database_name,
					blks_read,
					blks_hit,
					temp_files,
					temp_bytes
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.DatabaseName,
					&r.BlksRead,
					&r.BlksHit,
					&r.TempFiles,
					&r.TempBytes,
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

	// 9. SQL Server AG Health
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_ag_health",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(ag_name, ''),
					COALESCE(replica_server_name, ''),
					COALESCE(database_name, ''),
					COALESCE(replica_role, ''),
					COALESCE(operational_state, ''),
					COALESCE(connected_state, ''),
					COALESCE(synchronization_state, ''),
					COALESCE(synchronization_state_desc, ''),
					COALESCE(is_primary_replica, false),
					COALESCE(log_send_queue_kb, 0),
					COALESCE(redo_queue_kb, 0),
					COALESCE(log_send_rate_kb, 0),
					COALESCE(redo_rate_kb, 0),
					COALESCE(secondary_lag_seconds, 0)
				FROM sqlserver_ag_health
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
				var r schemas.SQLServerAGHealthRow
				if err := rows.Scan(
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.AGName,
					&r.ReplicaServerName,
					&r.DatabaseName,
					&r.ReplicaRole,
					&r.OperationalState,
					&r.ConnectedState,
					&r.SynchronizationState,
					&r.SyncStateDesc,
					&r.IsPrimaryReplica,
					&r.LogSendQueueKB,
					&r.RedoQueueKB,
					&r.LogSendRateKB,
					&r.RedoRateKB,
					&r.SecondaryLagSeconds,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerAGHealthRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerAGHealthRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})

	// 10. SQL Server Risk Health
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_risk_health",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(blocking_sessions, 0),
					COALESCE(memory_grants_pending, 0),
					COALESCE(failed_logins_5m, 0),
					COALESCE(tempdb_used_percent, 0),
					COALESCE(max_log_db_name, ''),
					COALESCE(max_log_used_percent, 0),
					COALESCE(ple, 0),
					COALESCE(compilations_per_sec, 0),
					COALESCE(batch_requests_per_sec, 0),
					COALESCE(buffer_cache_hit_ratio, 0)
				FROM sqlserver_risk_health
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
				var r schemas.SQLServerRiskHealthRow
				if err := rows.Scan(
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.BlockingSessions,
					&r.MemoryGrantsPending,
					&r.FailedLogins5m,
					&r.TempdbUsedPercent,
					&r.MaxLogDBName,
					&r.MaxLogUsedPercent,
					&r.PLE,
					&r.CompilationsPerSec,
					&r.BatchRequestsPerSec,
					&r.BufferCacheHitRatio,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerRiskHealthRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerRiskHealthRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})

	// 11. SQL Server Lock History
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.DatabaseName,
					&r.TotalLocks,
					&r.Deadlocks,
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

	// 12. SQL Server Long Running Queries
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_long_running_queries",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					session_id,
					COALESCE(request_id, 0),
					COALESCE(database_name, ''),
					COALESCE(login_name, ''),
					COALESCE(host_name, ''),
					COALESCE(program_name, ''),
					COALESCE(query_hash, 0),
					COALESCE(wait_type, ''),
					COALESCE(blocking_session_id, 0),
					COALESCE(status, ''),
					COALESCE(cpu_time_ms, 0),
					COALESCE(total_elapsed_time_ms, 0),
					COALESCE(reads, 0),
					COALESCE(writes, 0),
					COALESCE(granted_query_memory_mb, 0),
					COALESCE(row_count, 0),
					COALESCE(percent_complete, '')
				FROM sqlserver_long_running_queries
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
				var r schemas.SQLServerLongRunningQueryRow
				if err := rows.Scan(
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.SessionID,
					&r.RequestID,
					&r.DatabaseName,
					&r.LoginName,
					&r.HostName,
					&r.ProgramName,
					&r.QueryHash,
					&r.WaitType,
					&r.BlockingSessionID,
					&r.Status,
					&r.CPUTimeMs,
					&r.TotalElapsedTimeMs,
					&r.Reads,
					&r.Writes,
					&r.GrantedQueryMemoryMB,
					&r.RowCount,
					&r.PercentComplete,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerLongRunningQueryRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerLongRunningQueryRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})

	// 13. PostgreSQL Backup Archiver
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.ArchivedCount,
					&r.FailedCount,
					&r.LastArchivedMs,
					&r.LastFailedMs,
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

	// 14. SQL Server Memory Metrics
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.SQLMemoryUsedMB,
					&r.SQLMemoryTargetMB,
					&r.OSTotalMemoryMB,
					&r.OSAvailableMemoryMB,
					&r.ProcessPhysicalLow,
					&r.ProcessVirtualLow,
					&r.MemoryGrantsPending,
					&r.ActiveMemoryGrants,
					&r.WaitingMemoryGrants,
					&r.GrantedWorkspaceMB,
					&r.RequestedWorkspaceMB,
					&r.PLESeconds,
					&r.PlanCacheMB,
					&r.SortWarningsTotal,
					&r.HashWarningsTotal,
					&r.SortWarningsPerSec,
					&r.HashWarningsPerSec,
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

	// 15. PostgreSQL Basebackup History
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_basebackup_history",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.CheckpointTimeMs,
					&r.CheckpointWriteTime,
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

	// 16. PostgreSQL Failed Login Events
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.pg_failed_login_events",
		Engine:          "postgres",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.Username,
					&r.ClientAddr,
					&r.Message,
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

	// 17. SQL Server Procedure Stats
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_procedure_stats",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(database_name, ''),
					COALESCE(schema_name, ''),
					COALESCE(object_name, ''),
					query_hash,
					COALESCE(execution_count, 0),
					COALESCE(total_worker_time_ms, 0),
					COALESCE(total_elapsed_time_ms, 0),
					COALESCE(total_logical_reads, 0),
					COALESCE(total_physical_reads, 0)
				FROM sqlserver_procedure_stats
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
				var r schemas.SQLServerProcedureStatsRow
				if err := rows.Scan(
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.DatabaseName,
					&r.SchemaName,
					&r.ObjectName,
					&r.QueryHash,
					&r.ExecutionCount,
					&r.TotalWorkerTimeMs,
					&r.TotalElapsedTimeMs,
					&r.TotalLogicalReads,
					&r.TotalPhysicalReads,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerProcedureStatsRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerProcedureStatsRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})

	// 18. PostgreSQL Query Wait Profile
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.QueryID,
					&r.Calls,
					&r.TotalExecTime,
					&r.MeanExecTime,
					&r.Rows,
					&r.SharedBlksHit,
					&r.SharedBlksRead,
					&r.TempBlksWritten,
					&r.Query,
					&r.Username,
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

	// 19. PostgreSQL DDL Activity
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.SchemaName,
					&r.RelName,
					&r.NTupIns,
					&r.NTupUpd,
					&r.NTupDel,
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

	// 20. PostgreSQL Session Activity
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.DBName,
					&r.PID,
					&r.Username,
					&r.AppName,
					&r.ClientAddr,
					&r.State,
					&r.WaitEventType,
					&r.WaitEvent,
					&r.BackendType,
					&r.QueryID,
					&r.Query,
					&r.XactStartMs,
					&r.QueryStartMs,
					&r.StateChangeMs,
					&r.BackendStartMs,
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

	// 21. PostgreSQL Wait Event Summary
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.WaitEventType,
					&r.WaitEvent,
					&r.Sessions,
					&r.State,
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

	// 22. PostgreSQL DB Load
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.ActiveSessions,
					&r.CPUSessions,
					&r.WaitingSessions,
					&r.IOSessions,
					&r.LockSessions,
					&r.IdleInTxn,
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

	// 23. SQL Server Job Metrics
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.TotalJobs,
					&r.EnabledJobs,
					&r.DisabledJobs,
					&r.RunningJobs,
					&r.FailedJobs24h,
					&r.CriticalJobsDisabled,
					&r.ErrorMessage,
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

	// 24. SQL Server AG Cluster Info
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.sqlserver_ag_cluster_info",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(cluster_name, ''),
					COALESCE(quorum_type, ''),
					COALESCE(quorum_state, ''),
					COALESCE(members_json::text, '{}')
				FROM monitor.sqlserver_ag_cluster_info
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
				var r schemas.SQLServerAGClusterRow
				if err := rows.Scan(
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.ClusterName,
					&r.QuorumType,
					&r.QuorumState,
					&r.MembersJSON,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerAGClusterRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerAGClusterRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})

	// 25. SQL Server Memory History (PLE)
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
				if err := rows.Scan(
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.PageLifeExpectancy,
				); err != nil {
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

	// 26. SQL Server Query Store Snapshot
	e.RegisterTable(TableExportConfig{
		TableName:       "monitor.sqlserver_query_store_snapshot",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					database_name,
					query_hash,
					query_text,
					plan_id,
					runtime_stats_interval_id,
					total_executions,
					total_cpu_ms,
					total_duration_ms,
					total_logical_reads
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.DatabaseName,
					&r.QueryHash,
					&r.QueryText,
					&r.PlanID,
					&r.RuntimeStatsID,
					&r.TotalExecutions,
					&r.TotalCPUMs,
					&r.TotalDurationMs,
					&r.TotalLogicalReads,
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

	// 27. SQL Server Query Store Interval
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
					database_name,
					query_hash,
					query_text,
					plan_id,
					runtime_stats_interval_id,
					delta_executions,
					delta_cpu_ms,
					delta_duration_ms,
					delta_logical_reads
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
					&r.BucketStartMs,
					&r.BucketEndMs,
					&r.ServerID,
					&r.DatabaseName,
					&r.QueryHash,
					&r.QueryText,
					&r.PlanID,
					&r.RuntimeStatsID,
					&r.DeltaExecutions,
					&r.DeltaCPUMs,
					&r.DeltaDurationMs,
					&r.DeltaLogicalReads,
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

	// 28. PostgreSQL Stat Statements (Delta)
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
					calls,
					total_exec_time,
					rows,
					shared_blks_hit,
					shared_blks_read,
					temp_blks_written,
					COALESCE(wal_bytes, 0),
					total_plan_time,
					mean_exec_time
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
					&r.CaptureTimestampMs,
					&r.ServerID,
					&r.QueryID,
					&r.DBName,
					&r.Username,
					&r.AppName,
					&r.QueryType,
					&r.Calls,
					&r.TotalExecTime,
					&r.Rows,
					&r.SharedBlksHit,
					&r.SharedBlksRead,
					&r.TempBlksWritten,
					&r.WALBytes,
					&r.TotalPlanTime,
					&r.MeanExecTime,
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

	// 29. SQL Server Buffer Pool by DB
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_buffer_pool_db",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					COALESCE(database_name, ''),
					COALESCE(buffer_mb, 0)
				FROM sqlserver_buffer_pool_db
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
				var r schemas.SQLServerBufferPoolRow
				if err := rows.Scan(&r.CaptureTimestampMs, &r.ServerID, &r.DatabaseName, &r.BufferMB); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerBufferPoolRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerBufferPoolRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})

	// 30. SQL Server CPU Scheduler Stats
	e.RegisterTable(TableExportConfig{
		TableName:       "sqlserver_cpu_scheduler_stats",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error) {
			const q = `
				SELECT
					EXTRACT(EPOCH FROM capture_timestamp)::BIGINT * 1000,
					server_id::TEXT,
					max_workers_count, scheduler_count, cpu_count,
					total_runnable_tasks_count, total_work_queue_count,
					total_current_workers_count, active_workers_count,
					pending_disk_io_count, avg_runnable_tasks_count,
					total_active_request_count, total_queued_request_count,
					total_blocked_task_count, total_active_parallel_thread_count,
					runnable_request_count, total_request_count, runnable_percent,
					worker_thread_exhaustion_warning, runnable_tasks_warning,
					blocked_tasks_warning, queued_requests_warning,
					total_physical_memory_kb, available_physical_memory_kb,
					COALESCE(system_memory_state_desc, ''),
					physical_memory_pressure_warning,
					total_node_count, nodes_online_count, offline_cpu_count,
					offline_cpu_warning
				FROM sqlserver_cpu_scheduler_stats
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
				var r schemas.SQLServerSchedulerRow
				if err := rows.Scan(
					&r.CaptureTimestampMs, &r.ServerID,
					&r.MaxWorkersCount, &r.SchedulerCount, &r.CPUCount,
					&r.TotalRunnableTasksCount, &r.TotalWorkQueueCount,
					&r.TotalCurrentWorkersCount, &r.ActiveWorkersCount,
					&r.PendingDiskIOCount, &r.AvgRunnableTasksCount,
					&r.TotalActiveRequestCount, &r.TotalQueuedRequestCount,
					&r.TotalBlockedTaskCount, &r.TotalActiveParallelThreadCount,
					&r.RunnableRequestCount, &r.TotalRequestCount, &r.RunnablePercent,
					&r.WorkerThreadExhaustionWarning, &r.RunnableTasksWarning,
					&r.BlockedTasksWarning, &r.QueuedRequestsWarning,
					&r.TotalPhysicalMemoryKB, &r.AvailablePhysicalMemoryKB,
					&r.SystemMemoryStateDesc, &r.PhysicalMemoryPressureWarning,
					&r.TotalNodeCount, &r.NodesOnlineCount, &r.OfflineCPUCount,
					&r.OfflineCPUWarning,
				); err != nil {
					return nil, err
				}
				res = append(res, r)
			}
			return res, rows.Err()
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			typed := make([]schemas.SQLServerSchedulerRow, len(rows))
			for i, r := range rows {
				typed[i] = r.(schemas.SQLServerSchedulerRow)
			}
			return WriteTypedParquet(path, typed)
		},
	})

	// 31. PostgreSQL Settings Snapshot
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
					name, COALESCE(setting, ''), COALESCE(unit, ''), COALESCE(source, '')
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
				if err := rows.Scan(&r.CaptureTimestampMs, &r.ServerID, &r.Name, &r.Setting, &r.Unit, &r.Source); err != nil {
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

	// 32. PostgreSQL Roles Snapshot
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

	// 33. SQL Server Latch Waits
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
				if err := rows.Scan(&r.CaptureTimestampMs, &r.ServerID, &r.WaitType, &r.WaitingTasksCount, &r.WaitTimeMs, &r.SignalWaitTimeMs); err != nil {
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

	// 34. SQL Server Spinlock Stats
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
				if err := rows.Scan(&r.CaptureTimestampMs, &r.ServerID, &r.SpinlockType, &r.Collisions, &r.Spins, &r.SleepTimeMs); err != nil {
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

	// 35. SQL Server Memory Grant Waiters
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

	// 36. Collector Execution Runs
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
					status, rows_inserted, COALESCE(error_message, ''), duration_ms
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
