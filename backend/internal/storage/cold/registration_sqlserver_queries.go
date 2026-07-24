// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration_sqlserver_queries.go
// Purpose: Cold storage export registrations for SQL Server query-level tables:
//          long-running queries, procedure stats, buffer pool, CPU scheduler stats.
//          Query Store, latches, spinlocks, and memory grant waiters are in
//          registration_sqlserver_advanced.go.
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

// registerSQLServerQueries adds SQL Server query and performance diagnostic tables
// to the cold storage exporter.
func registerSQLServerQueries(e *Exporter) {
	registerSQLServerLongRunningQueries(e)
	registerSQLServerProcedureStats(e)
	registerSQLServerBufferPoolDB(e)
	registerSQLServerCPUSchedulerStats(e)
	registerSQLServerAdvanced(e)
}

func registerSQLServerLongRunningQueries(e *Exporter) {
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
					&r.CaptureTimestampMs, &r.ServerID,
					&r.SessionID, &r.RequestID, &r.DatabaseName, &r.LoginName,
					&r.HostName, &r.ProgramName, &r.QueryHash,
					&r.WaitType, &r.BlockingSessionID, &r.Status,
					&r.CPUTimeMs, &r.TotalElapsedTimeMs,
					&r.Reads, &r.Writes, &r.GrantedQueryMemoryMB,
					&r.RowCount, &r.PercentComplete,
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
}

func registerSQLServerProcedureStats(e *Exporter) {
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
					&r.CaptureTimestampMs, &r.ServerID,
					&r.DatabaseName, &r.SchemaName, &r.ObjectName, &r.QueryHash,
					&r.ExecutionCount, &r.TotalWorkerTimeMs, &r.TotalElapsedTimeMs,
					&r.TotalLogicalReads, &r.TotalPhysicalReads,
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
}

func registerSQLServerBufferPoolDB(e *Exporter) {
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
}

func registerSQLServerCPUSchedulerStats(e *Exporter) {
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
}
