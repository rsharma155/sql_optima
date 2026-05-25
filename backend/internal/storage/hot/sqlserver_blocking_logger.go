// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: sqlserver_blocking_logger.go
// Purpose: SQL Server blocking and deadlock logger for TimescaleDB.
// Metadata:
//   - Version: 1.1
//   - Package: hot
//   - Targets: TimescaleDB Hypertables (ms_blocking_snapshots, ms_deadlock_events, etc.)
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/models"
)

// LogSQLServerBlockingSnapshots persists blocking snapshots to TimescaleDB
func (tl *TimescaleLogger) LogSQLServerBlockingSnapshots(ctx context.Context, serverID uuid.UUID, snapshots []models.SQLServerBlockingSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sqlserver_blocking_snapshots (
			capture_timestamp, server_id, session_id, blocking_session_id, wait_type, wait_duration_ms,
			database_name, sql_hash, plan_hash, login_name, host_name, program_name,
			open_transaction_count, transaction_start_time, cpu_time, reads, writes, logical_reads,
			transaction_isolation_level, memory_usage, transaction_log_bytes_used,
			transaction_log_bytes_reserved, wait_resource, percent_complete
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
	`
	for _, s := range snapshots {
		var sqlHash, planHash int64
		if h, err := strconv.ParseUint(strings.TrimPrefix(s.SqlHash, "0x"), 16, 64); err == nil {
			sqlHash = int64(h)
		}
		if h, err := strconv.ParseUint(strings.TrimPrefix(s.PlanHash, "0x"), 16, 64); err == nil {
			planHash = int64(h)
		}

		batch.Queue(query,
			s.Timestamp, serverID, s.SessionID, s.BlockingSessionID, s.WaitType, s.WaitDurationMs,
			s.DatabaseName, sqlHash, planHash, s.LoginName, s.HostName, s.ProgramName,
			s.OpenTransactionCount, s.TransactionStartTime, s.CpuTime, s.Reads, s.Writes, s.LogicalReads,
			s.TransactionIsolationLevel, s.MemoryUsage, s.TransactionLogBytesUsed, s.TransactionLogBytesReserved,
			s.WaitResource, s.PercentComplete,
		)

		// Also upsert the SQL text to the dimension table if available
		if s.QueryText != "" && s.SqlHash != "" {
			batch.Queue(`
				INSERT INTO sqlserver_text_dim (sql_hash, sql_text)
				VALUES ($1, $2)
				ON CONFLICT (sql_hash) DO NOTHING`,
				sqlHash, s.QueryText,
			)
		}
	}

	res := tl.pool.SendBatch(ctx, batch)
	if err := res.Close(); err != nil {
		return fmt.Errorf("LogSQLServerBlockingSnapshots batch: %w", err)
	}

	return nil
}

// LogSQLServerBlockingLocks persists detailed lock info to TimescaleDB
func (tl *TimescaleLogger) LogSQLServerBlockingLocks(ctx context.Context, serverID uuid.UUID, locks []models.SQLServerBlockingLock) error {
	if len(locks) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sqlserver_blocking_locks (
			capture_timestamp, server_id, session_id, resource_type, request_mode, request_status, resource_description, wait_duration_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	for _, l := range locks {
		batch.Queue(query,
			l.Timestamp, serverID, l.SessionID, l.ResourceType, l.RequestMode, l.RequestStatus, l.ResourceDescription, l.WaitDurationMs,
		)
	}

	res := tl.pool.SendBatch(ctx, batch)
	if err := res.Close(); err != nil {
		return fmt.Errorf("LogSQLServerBlockingLocks batch: %w", err)
	}

	return nil
}

// LogSQLServerDeadlockEvents persists deadlock events to TimescaleDB.
// Uses ON CONFLICT DO NOTHING so re-runs after a service restart within the 24h XE
// window do not produce duplicate rows (backed by idx_sqlserver_deadlock_events_dedup).
func (tl *TimescaleLogger) LogSQLServerDeadlockEvents(ctx context.Context, serverID uuid.UUID, events []models.SQLServerDeadlockEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sqlserver_deadlock_events (
			capture_timestamp, server_id, database_name, victim_session_id, victim_sql_hash, deadlock_graph
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (capture_timestamp, server_id, victim_session_id) DO NOTHING
	`
	for _, e := range events {
		var victimHash int64
		if h, err := strconv.ParseUint(strings.TrimPrefix(e.VictimSqlHash, "0x"), 16, 64); err == nil {
			victimHash = int64(h)
		}

		batch.Queue(query,
			e.Timestamp, serverID, e.DatabaseName, e.VictimSessionID, victimHash, e.DeadlockGraph,
		)
	}

	res := tl.pool.SendBatch(ctx, batch)
	if err := res.Close(); err != nil {
		return fmt.Errorf("LogSQLServerDeadlockEvents batch: %w", err)
	}

	return nil
}

// UpsertSQLServerTextDim deduplicates SQL text inserts
func (tl *TimescaleLogger) UpsertSQLServerTextDim(ctx context.Context, sqlHash, sqlText string) error {
	var h64 int64
	if h, err := strconv.ParseUint(strings.TrimPrefix(sqlHash, "0x"), 16, 64); err == nil {
		h64 = int64(h)
	}
	query := `
		INSERT INTO sqlserver_text_dim (sql_hash, sql_text)
		VALUES ($1, $2)
		ON CONFLICT (sql_hash) DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query, h64, sqlText)
	return err
}

// UpsertSQLServerQueryPlanDim deduplicates query plan inserts
func (tl *TimescaleLogger) UpsertSQLServerQueryPlanDim(ctx context.Context, planHash, queryPlan string) error {
	var h64 int64
	if h, err := strconv.ParseUint(strings.TrimPrefix(planHash, "0x"), 16, 64); err == nil {
		h64 = int64(h)
	}
	query := `
		INSERT INTO sqlserver_query_plan_dim (plan_hash, query_plan)
		VALUES ($1, $2)
		ON CONFLICT (plan_hash) DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query, h64, queryPlan)
	return err
}

// GetSQLServerBlockingKPIs returns current blocking KPIs.
// snapshot_age_sec measures how many seconds ago the latest snapshot was captured so
// the frontend can distinguish a stale "no blocking" snapshot from a live one.
func (tl *TimescaleLogger) GetSQLServerBlockingKPIs(ctx context.Context, serverID uuid.UUID) (map[string]interface{}, error) {
	query := `
		WITH latest AS (
			SELECT capture_timestamp FROM sqlserver_blocking_snapshots WHERE server_id = $1 ORDER BY capture_timestamp DESC LIMIT 1
		),
		current_snaps AS (
			SELECT * FROM sqlserver_blocking_snapshots WHERE server_id = $1 AND capture_timestamp = (SELECT capture_timestamp FROM latest)
		)
		SELECT
			COALESCE((SELECT COUNT(*) FROM current_snaps WHERE blocking_session_id != 0), 0) AS active_blocked_sessions,
			COALESCE((SELECT COUNT(DISTINCT blocking_session_id) FROM current_snaps WHERE blocking_session_id != 0 AND blocking_session_id NOT IN (SELECT session_id FROM current_snaps WHERE blocking_session_id != 0)), 0) AS root_blockers,
			COALESCE((SELECT MAX(wait_duration_ms) FROM current_snaps), 0) AS max_wait_duration_ms,
			COALESCE((SELECT COUNT(*) FROM current_snaps WHERE transaction_start_time <= (SELECT capture_timestamp FROM latest) - INTERVAL '1 minute'), 0) AS open_tran_over_1min,
			COALESCE((SELECT COUNT(*) FROM sqlserver_deadlock_events WHERE server_id = $1 AND capture_timestamp >= NOW() - INTERVAL '24 hours'), 0) AS deadlocks_24h,
			COALESCE((SELECT COUNT(DISTINCT capture_timestamp) FROM sqlserver_blocking_snapshots WHERE server_id = $1 AND capture_timestamp >= NOW() - INTERVAL '24 hours' AND blocking_session_id != 0), 0) AS blocking_incidents_24h,
			COALESCE(EXTRACT(EPOCH FROM (NOW() - (SELECT capture_timestamp FROM latest)))::int, -1) AS snapshot_age_sec
	`

	var kpi struct {
		ActiveBlockedSessions int64
		RootBlockers          int64
		MaxWaitDurationMs     int64
		OpenTranOver1Min      int64
		Deadlocks24h          int64
		Incidents24h          int64
		SnapshotAgeSec        int64
	}

	err := tl.pool.QueryRow(ctx, query, serverID).Scan(
		&kpi.ActiveBlockedSessions, &kpi.RootBlockers, &kpi.MaxWaitDurationMs,
		&kpi.OpenTranOver1Min, &kpi.Deadlocks24h, &kpi.Incidents24h, &kpi.SnapshotAgeSec,
	)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	return map[string]interface{}{
		"active_blocked_sessions": kpi.ActiveBlockedSessions,
		"root_blockers":           kpi.RootBlockers,
		"max_wait_duration_ms":    kpi.MaxWaitDurationMs,
		"open_tran_over_1min":     kpi.OpenTranOver1Min,
		"deadlocks_24h":           kpi.Deadlocks24h,
		"blocking_incidents_24h":  kpi.Incidents24h,
		"snapshot_age_sec":        kpi.SnapshotAgeSec,
	}, nil
}

// GetSQLServerTopBlockingQueries returns top blocking queries by wait time
func (tl *TimescaleLogger) GetSQLServerTopBlockingQueries(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			s.sql_hash,
			SUM(s.wait_duration_ms) AS total_wait_duration_ms,
			COUNT(DISTINCT s.capture_timestamp) AS incident_count,
			AVG(s.wait_duration_ms) AS avg_wait_duration_ms,
			MAX(t.sql_text) AS query_text
		FROM sqlserver_blocking_snapshots s
		LEFT JOIN sqlserver_text_dim t ON s.sql_hash = t.sql_hash
		WHERE s.server_id = $1 AND s.capture_timestamp BETWEEN $2 AND $3 AND s.blocking_session_id != 0
		GROUP BY s.sql_hash
		ORDER BY total_wait_duration_ms DESC
		LIMIT 20
	`
	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var sqlHash sql.NullInt64
		var queryText sql.NullString
		var totalWait, incidentCount int64
		var avgWait float64
		if err := rows.Scan(&sqlHash, &totalWait, &incidentCount, &avgWait, &queryText); err != nil {
			continue
		}
		hashStr := ""
		if sqlHash.Valid {
			hashStr = fmt.Sprintf("0x%X", uint64(sqlHash.Int64))
		}
		results = append(results, map[string]interface{}{
			"sql_hash":               hashStr,
			"total_wait_duration_ms": totalWait,
			"incident_count":         incidentCount,
			"avg_wait_duration_ms":   avgWait,
			"query_text":             queryText.String,
		})
	}
	return results, rows.Err()
}

// GetSQLServerMostBlockedDatabases returns databases with most blocking incidents
func (tl *TimescaleLogger) GetSQLServerMostBlockedDatabases(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			database_name,
			COUNT(*) AS incident_count
		FROM sqlserver_blocking_snapshots
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3 AND blocking_session_id != 0
		GROUP BY database_name
		ORDER BY incident_count DESC
	`
	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dbName sql.NullString
		var count int64
		if err := rows.Scan(&dbName, &count); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"database_name":  dbName.String,
			"incident_count": count,
		})
	}
	return results, rows.Err()
}

// GetSQLServerMostBlockedObjects returns objects with most blocking incidents.
// Uses sqlserver_blocking_snapshots (wait_resource + wait_type) since sqlserver_blocking_locks
// is not populated by the current collector path.
func (tl *TimescaleLogger) GetSQLServerMostBlockedObjects(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]map[string]interface{}, error) {
	query := `
		SELECT
			COALESCE(NULLIF(wait_resource, ''), database_name, 'Unknown') AS object_name,
			COALESCE(NULLIF(wait_type, ''), 'Unknown') AS lock_type,
			CAST(SUM(COALESCE(wait_duration_ms, 0)) AS BIGINT) AS total_wait,
			COUNT(*) AS incident_count
		FROM sqlserver_blocking_snapshots
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3 AND blocking_session_id != 0
		GROUP BY object_name, lock_type
		ORDER BY incident_count DESC
		LIMIT 20
	`
	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var objName, lockType string
		var totalWait, count int64
		if err := rows.Scan(&objName, &lockType, &totalWait, &count); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"object_name":    objName,
			"lock_type":      lockType,
			"total_wait":     totalWait,
			"incident_count": count,
		})
	}
	return results, nil
}

// GetSQLServerBlockingTimeline returns blocking incidents over time.
// Returns both blocked_sessions count and total_wait_ms per minute bucket
// so the frontend can toggle between "count" and "wait time" views.
func (tl *TimescaleLogger) GetSQLServerBlockingTimeline(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]map[string]interface{}, error) {
	query := `
		SELECT
			time_bucket('1 minute', capture_timestamp) AS bucket,
			COUNT(DISTINCT session_id) AS blocked_sessions,
			SUM(wait_duration_ms) AS total_wait_ms
		FROM sqlserver_blocking_snapshots
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3 AND blocking_session_id != 0
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var count, totalWaitMs int64
		if err := rows.Scan(&bucket, &count, &totalWaitMs); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"capture_timestamp": bucket,
			"blocked_sessions":  count,
			"total_wait_ms":     totalWaitMs,
		})
	}
	return results, rows.Err()
}

// GetSQLServerBlockingDetails returns detailed snapshots for a time window
func (tl *TimescaleLogger) GetSQLServerBlockingDetails(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]models.SQLServerBlockingSnapshot, error) {
	query := `
		SELECT 
			s.capture_timestamp, s.session_id, s.blocking_session_id, s.wait_type, s.wait_duration_ms,
			s.database_name, s.sql_hash, s.plan_hash, s.login_name, s.host_name, s.program_name,
			s.open_transaction_count, s.transaction_start_time, s.cpu_time, s.reads, s.writes, s.logical_reads,
			s.transaction_isolation_level, s.memory_usage, s.transaction_log_bytes_used,
			s.transaction_log_bytes_reserved, s.wait_resource, s.percent_complete, t.sql_text
		FROM sqlserver_blocking_snapshots s
		LEFT JOIN sqlserver_text_dim t ON s.sql_hash = t.sql_hash
		WHERE s.server_id = $1 AND s.capture_timestamp BETWEEN $2 AND $3
		ORDER BY s.capture_timestamp DESC, s.wait_duration_ms DESC
	`
	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SQLServerBlockingSnapshot
	for rows.Next() {
		var r models.SQLServerBlockingSnapshot
		var sqlText, waitType, dbName, loginName, hostName, progName, isoLevel, waitRes sql.NullString
		var sqlHash, planHash sql.NullInt64
		var xactStart sql.NullTime

		if err := rows.Scan(
			&r.Timestamp, &r.SessionID, &r.BlockingSessionID, &waitType, &r.WaitDurationMs,
			&dbName, &sqlHash, &planHash, &loginName, &hostName, &progName,
			&r.OpenTransactionCount, &xactStart, &r.CpuTime, &r.Reads, &r.Writes, &r.LogicalReads,
			&isoLevel, &r.MemoryUsage, &r.TransactionLogBytesUsed, &r.TransactionLogBytesReserved,
			&waitRes, &r.PercentComplete, &sqlText,
		); err != nil {
			return results, fmt.Errorf("GetSQLServerBlockingDetails scan: %w", err)
		}

		r.ServerID = serverID
		r.WaitType = waitType.String
		r.DatabaseName = dbName.String
		r.LoginName = loginName.String
		r.HostName = hostName.String
		r.ProgramName = progName.String
		r.TransactionIsolationLevel = isoLevel.String
		r.WaitResource = waitRes.String
		if sqlHash.Valid {
			r.SqlHash = fmt.Sprintf("0x%X", uint64(sqlHash.Int64))
		}
		if planHash.Valid {
			r.PlanHash = fmt.Sprintf("0x%X", uint64(planHash.Int64))
		}

		if xactStart.Valid {
			t := xactStart.Time
			r.TransactionStartTime = &t
		}
		if sqlText.Valid {
			r.QueryText = sqlText.String
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetSQLServerBlockingLocks returns lock details for a time window
func (tl *TimescaleLogger) GetSQLServerBlockingLocks(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]models.SQLServerBlockingLock, error) {
	query := `
		SELECT capture_timestamp, session_id, resource_type, request_mode, request_status, resource_description, wait_duration_ms
		FROM sqlserver_blocking_locks
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
		ORDER BY capture_timestamp DESC
	`
	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SQLServerBlockingLock
	for rows.Next() {
		var l models.SQLServerBlockingLock
		if err := rows.Scan(&l.Timestamp, &l.SessionID, &l.ResourceType, &l.RequestMode, &l.RequestStatus, &l.ResourceDescription, &l.WaitDurationMs); err != nil {
			continue
		}
		l.ServerID = serverID
		results = append(results, l)
	}
	return results, nil
}

// GetSQLServerBlockingRecurrence returns historical blocking incidents for a specific SQL hash or login
func (tl *TimescaleLogger) GetSQLServerBlockingRecurrence(ctx context.Context, serverID uuid.UUID, sqlHash string, loginName string) ([]map[string]interface{}, error) {
	var sqlHashVal int64
	if strings.HasPrefix(sqlHash, "0x") {
		if h, err := strconv.ParseUint(strings.TrimPrefix(sqlHash, "0x"), 16, 64); err == nil {
			sqlHashVal = int64(h)
		}
	} else if sqlHash != "" {
		sqlHashVal, _ = strconv.ParseInt(sqlHash, 10, 64)
	}

	query := `
		SELECT 
			time_bucket('1 hour', capture_timestamp) AS bucket,
			COUNT(DISTINCT capture_timestamp) AS incident_count,
			MAX(wait_duration_ms) AS max_wait_ms
		FROM sqlserver_blocking_snapshots
		WHERE server_id = $1 
		  AND (sql_hash = $2 OR login_name = $3)
		  AND blocking_session_id = 0 -- We look for when they were the ROOT blocker
		  AND capture_timestamp >= NOW() - INTERVAL '7 days'
		GROUP BY bucket
		ORDER BY bucket DESC
	`
	rows, err := tl.pool.Query(ctx, query, serverID, sqlHashVal, loginName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var count, maxWait int64
		if err := rows.Scan(&bucket, &count, &maxWait); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"capture_timestamp": bucket,
			"incident_count":    count,
			"max_wait_ms":       maxWait,
		})
	}
	return results, nil
}

// GetSQLServerDeadlockHistory returns deadlock events for a time window
func (tl *TimescaleLogger) GetSQLServerDeadlockHistory(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]models.SQLServerDeadlockEvent, error) {
	query := `
		SELECT s.capture_timestamp, s.database_name, s.victim_session_id, s.victim_sql_hash, s.deadlock_graph, t.sql_text
		FROM sqlserver_deadlock_events s
		LEFT JOIN sqlserver_text_dim t ON s.victim_sql_hash = t.sql_hash
		WHERE s.server_id = $1 AND s.capture_timestamp BETWEEN $2 AND $3
		ORDER BY s.capture_timestamp DESC
	`
	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SQLServerDeadlockEvent
	for rows.Next() {
		var e models.SQLServerDeadlockEvent
		var dbName, graph, sqlText sql.NullString
		var sqlHashVal sql.NullInt64
		if err := rows.Scan(&e.Timestamp, &dbName, &e.VictimSessionID, &sqlHashVal, &graph, &sqlText); err != nil {
			continue
		}
		e.ServerID = serverID
		e.DatabaseName = dbName.String
		if sqlHashVal.Valid {
			e.VictimSqlHash = fmt.Sprintf("0x%X", uint64(sqlHashVal.Int64))
		}
		if sqlText.Valid {
			e.VictimSqlText = sqlText.String
		}
		e.DeadlockGraph = graph.String
		results = append(results, e)
	}
	return results, rows.Err()
}
