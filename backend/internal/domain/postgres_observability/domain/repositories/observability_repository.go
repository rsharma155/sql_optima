// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Observability data repositories.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repositories

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_observability/domain/entities"
	"time"
)

type PostgresObservabilityRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresObservabilityRepository(pool *pgxpool.Pool) *PostgresObservabilityRepository {
	return &PostgresObservabilityRepository{pool: pool}
}

func (r *PostgresObservabilityRepository) SaveSessionActivity(ctx context.Context, activities []entities.SessionActivity) error {
	if len(activities) == 0 {
		return nil
	}
	// Use batch for better performance
	batch := &pgx.Batch{}
	for _, a := range activities {
		var clientAddr interface{}
		if a.ClientAddr != "" {
			clientAddr = a.ClientAddr
		}

		// 1. Log to history table
		batch.Queue(`
			INSERT INTO monitor.pg_session_activity_ts
			(capture_timestamp, server_id, dbname, pid, usename, application_name, client_addr, state, wait_event_type, wait_event, backend_type, query_id, query, xact_start, query_start, state_change, backend_start)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			a.TS, a.ServerID, a.DBName, a.PID, a.Username, a.ApplicationName, clientAddr, a.State, a.WaitEventType, a.WaitEvent, a.BackendType, a.QueryID, a.Query, a.XactStart, a.QueryStart, a.StateChange, a.BackendStart)

		// 2. Log to staging table for hierarchical blocking analysis (STRICT STAGING)
		// Columns match schema exactly: no query_id or backend_start (not in staging schema)
		batch.Queue(`
			INSERT INTO staging.postgres_activity_locks_raw
			(capture_timestamp, server_id, pid, datname, usename, application_name, client_addr, backend_type, state, wait_event_type, wait_event, query_start, xact_start, state_change, blocking_pids, query)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			a.TS, a.ServerID, a.PID, a.DBName, a.Username, a.ApplicationName, clientAddr, a.BackendType, a.State, a.WaitEventType, a.WaitEvent, a.QueryStart, a.XactStart, a.StateChange, a.BlockingPids, a.Query)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(activities)*2; i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("failed to save session activity in batch: %w", err)
		}
	}
	return nil
}

func (r *PostgresObservabilityRepository) SaveWaitEventSummary(ctx context.Context, summaries []entities.WaitEventSummary) error {
	for _, s := range summaries {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO monitor.pg_wait_event_summary_ts (capture_timestamp, server_id, wait_event_type, wait_event, sessions)
			VALUES ($1, $2, $3, $4, $5)`,
			s.TS, s.ServerID, s.WaitEventType, s.WaitEvent, s.Sessions)
		if err != nil {
			return fmt.Errorf("failed to save wait summary: %w", err)
		}
	}
	return nil
}

func (r *PostgresObservabilityRepository) SaveDBLoad(ctx context.Context, load entities.DBLoad) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO monitor.pg_db_load_ts (capture_timestamp, server_id, active_sessions, cpu_sessions, waiting_sessions, io_sessions, lock_sessions, idle_in_txn)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		load.TS, load.ServerID, load.ActiveSessions, load.CPUSessions, load.WaitingSessions, load.IOWaitSessions, load.LockWaitSessions, load.IdleInTxn)
	return err
}

func (r *PostgresObservabilityRepository) SaveQueryWaitProfile(ctx context.Context, profiles []entities.QueryWaitProfile) error {
	for _, p := range profiles {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO monitor.pg_query_wait_profile_ts (capture_timestamp, server_id, queryid, calls, total_exec_time, mean_exec_time, rows, shared_blks_hit, shared_blks_read, temp_blks_written, query, usename)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			p.TS, p.ServerID, p.QueryID, p.Calls, p.TotalExecTime, p.MeanExecTime, p.Rows, p.SharedBlksHit, p.SharedBlksRead, p.TempBlksWritten, p.Query, p.Username)
		if err != nil {
			return fmt.Errorf("failed to save query wait profile: %w", err)
		}
	}
	return nil
}

func (r *PostgresObservabilityRepository) GetKPIData(ctx context.Context, serverID uuid.UUID, from, to string) (map[string]interface{}, error) {
	query := `
		SELECT 
			COALESCE(round(avg(active_sessions),2), 0) as avg_active,
			COALESCE(round(avg(waiting_sessions),2), 0) as avg_waiting,
			COALESCE(round(avg(idle_in_txn),2), 0) as avg_idle,
			COALESCE(max(active_sessions + waiting_sessions + idle_in_txn), 0) as max_conn
		FROM monitor.pg_db_load_ts
		WHERE capture_timestamp BETWEEN $1 AND $2 AND server_id = $3`

	var avgActive, avgWaiting, avgIdle float64
	var maxConn int
	err := r.pool.QueryRow(ctx, query, from, to, serverID).Scan(&avgActive, &avgWaiting, &avgIdle, &maxConn)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"avg_active":  avgActive,
		"avg_waiting": avgWaiting,
		"avg_idle":    avgIdle,
		"max_conn":    maxConn,
	}, nil
}

func (r *PostgresObservabilityRepository) GetLoadTrend(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT capture_timestamp, 
		       COALESCE(active_sessions, 0), 
		       COALESCE(cpu_sessions, 0), 
		       COALESCE(waiting_sessions, 0), 
		       COALESCE(io_sessions, 0), 
		       COALESCE(lock_sessions, 0), 
		       COALESCE(idle_in_txn, 0)
		FROM monitor.pg_db_load_ts
		WHERE capture_timestamp BETWEEN $1 AND $2 AND server_id = $3
		ORDER BY capture_timestamp ASC`

	rows, err := r.pool.Query(ctx, query, from, to, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var active, cpu, waiting, io, lock, idle int
		if err := rows.Scan(&ts, &active, &cpu, &waiting, &io, &lock, &idle); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"ts":               ts,
			"active_sessions":  active,
			"cpu_sessions":     cpu,
			"waiting_sessions": waiting,
			"io_sessions":      io,
			"lock_sessions":    lock,
			"idle_in_txn":      idle,
		})
	}
	return results, nil
}

func (r *PostgresObservabilityRepository) GetWaitCategoryTrend(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('5 min', capture_timestamp) AS bucket,
		       wait_event_type,
		       sum(sessions) as sessions
		FROM monitor.pg_wait_event_summary_ts
		WHERE capture_timestamp BETWEEN $1 AND $2 AND server_id = $3
		GROUP BY bucket, wait_event_type
		ORDER BY bucket ASC`

	rows, err := r.pool.Query(ctx, query, from, to, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var wet string
		var sessions float64
		if err := rows.Scan(&bucket, &wet, &sessions); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"bucket":          bucket,
			"wait_event_type": wet,
			"sessions":        sessions,
		})
	}
	return results, nil
}

func (r *PostgresObservabilityRepository) GetWaitCategoryDistribution(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT wait_event_type, avg(sessions) as avg_sessions
		FROM monitor.pg_wait_event_summary_ts
		WHERE capture_timestamp BETWEEN $1 AND $2 AND server_id = $3
		GROUP BY wait_event_type`

	rows, err := r.pool.Query(ctx, query, from, to, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var wet string
		var avg float64
		if err := rows.Scan(&wet, &avg); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"wait_event_type": wet,
			"avg_sessions":    avg,
		})
	}
	return results, nil
}

func (r *PostgresObservabilityRepository) GetConnectionsByApp(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT application_name, count(*) as count
		FROM monitor.pg_session_activity_ts
		WHERE capture_timestamp BETWEEN $1 AND $2 AND server_id = $3
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
		GROUP BY application_name
		ORDER BY count DESC LIMIT 10`

	rows, err := r.pool.Query(ctx, query, from, to, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var app string
		var count int
		if err := rows.Scan(&app, &count); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"application_name": app,
			"count":            count,
		})
	}
	return results, nil
}

func (r *PostgresObservabilityRepository) GetLongRunningSessions(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	// Use the most recent capture snapshot within the requested window for a point-in-time view.
	query := `
		WITH latest AS (
		    SELECT MAX(capture_timestamp) AS ts
		    FROM monitor.pg_session_activity_ts
		    WHERE server_id = $3 AND capture_timestamp BETWEEN $1 AND $2
		)
		SELECT s.capture_timestamp,
		       s.pid,
		       COALESCE(s.usename, ''),
		       EXTRACT(EPOCH FROM (now() - s.query_start))::bigint AS duration_seconds,
		       COALESCE(s.wait_event, ''),
		       COALESCE(s.query, ''),
		       COALESCE(s.application_name, '')
		FROM monitor.pg_session_activity_ts s
		JOIN latest ON s.capture_timestamp = latest.ts
		WHERE s.server_id = $3
		  AND s.state = 'active'
		  AND (s.usename IS NULL OR s.usename <> 'dbmonitor_user')
		  AND s.query NOT LIKE '%/* SQL_OPTIMA%'
		  AND (s.backend_type = 'client backend' OR s.backend_type IS NULL)
		ORDER BY duration_seconds DESC LIMIT 20`

	rows, err := r.pool.Query(ctx, query, from, to, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var captureTs time.Time
		var pid int
		var user, waitEvent, queryStr, appName string
		var durationSec int64
		if err := rows.Scan(&captureTs, &pid, &user, &durationSec, &waitEvent, &queryStr, &appName); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"pid":               pid,
			"usename":           user,
			"duration":          formatDuration(durationSec),
			"capture_timestamp": captureTs,
			"wait_event":        waitEvent,
			"query":             queryStr,
			"application_name":  appName,
		})
	}
	return results, nil
}

func (r *PostgresObservabilityRepository) GetTopQueries(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT capture_timestamp, queryid, calls, total_exec_time, mean_exec_time, temp_blks_written, query, usename
		FROM monitor.pg_query_wait_profile_ts
		WHERE capture_timestamp BETWEEN $1 AND $2 AND server_id = $3
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY total_exec_time DESC LIMIT 20`

	rows, err := r.pool.Query(ctx, query, from, to, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var qid int64
		var calls int64
		var total, mean float64
		var temp int64
		var queryStr, usename *string
		if err := rows.Scan(&ts, &qid, &calls, &total, &mean, &temp, &queryStr, &usename); err != nil {
			return nil, err
		}

		qVal := ""
		if queryStr != nil {
			qVal = *queryStr
		}
		uVal := ""
		if usename != nil {
			uVal = *usename
		}

		results = append(results, map[string]interface{}{
			"ts":                ts,
			"queryid":           qid,
			"calls":             calls,
			"total_exec_time":   total,
			"mean_exec_time":    mean,
			"temp_blks_written": temp,
			"query":             qVal,
			"usename":           uVal,
		})
	}
	return results, nil
}

func (r *PostgresObservabilityRepository) GetSessionStateTrend(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('5 min', capture_timestamp) AS bucket,
		       state,
		       count(*) as count
		FROM monitor.pg_session_activity_ts
		WHERE capture_timestamp BETWEEN $1 AND $2 AND server_id = $3
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
		GROUP BY bucket, state
		ORDER BY bucket ASC`

	rows, err := r.pool.Query(ctx, query, from, to, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var state string
		var count float64
		if err := rows.Scan(&bucket, &state, &count); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"bucket": bucket,
			"state":  state,
			"count":  count,
		})
	}
	return results, nil
}

func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

