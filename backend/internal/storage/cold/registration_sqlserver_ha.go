// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration_sqlserver_ha.go
// Purpose: Cold storage export registrations for SQL Server high-availability tables
//          (AG health, risk health, AG cluster info).
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

// registerSQLServerHA adds SQL Server HA/AG tables to the cold storage exporter.
func registerSQLServerHA(e *Exporter) {
	registerSQLServerAGHealth(e)
	registerSQLServerRiskHealth(e)
	registerSQLServerAGClusterInfo(e)
}

func registerSQLServerAGHealth(e *Exporter) {
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
					&r.CaptureTimestampMs, &r.ServerID,
					&r.AGName, &r.ReplicaServerName, &r.DatabaseName,
					&r.ReplicaRole, &r.OperationalState, &r.ConnectedState,
					&r.SynchronizationState, &r.SyncStateDesc, &r.IsPrimaryReplica,
					&r.LogSendQueueKB, &r.RedoQueueKB,
					&r.LogSendRateKB, &r.RedoRateKB, &r.SecondaryLagSeconds,
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
}

func registerSQLServerRiskHealth(e *Exporter) {
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
					&r.CaptureTimestampMs, &r.ServerID,
					&r.BlockingSessions, &r.MemoryGrantsPending, &r.FailedLogins5m,
					&r.TempdbUsedPercent, &r.MaxLogDBName, &r.MaxLogUsedPercent,
					&r.PLE, &r.CompilationsPerSec, &r.BatchRequestsPerSec, &r.BufferCacheHitRatio,
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
}

func registerSQLServerAGClusterInfo(e *Exporter) {
	e.RegisterTable(TableExportConfig{
		TableName:            "monitor.sqlserver_ag_cluster_info",
		Engine:               "sqlserver",
		TimestampColumn:      "capture_timestamp",
		ServerIDColumn:       "server_id",
		SkipCompressionCheck: true, // cluster-scoped non-hypertable; no TimescaleDB compression
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
					&r.CaptureTimestampMs, &r.ServerID,
					&r.ClusterName, &r.QuorumType, &r.QuorumState, &r.MembersJSON,
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
}
