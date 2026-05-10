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
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_observability/domain/entities"
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
	// Use batch or simple insert for now. Given it's a few rows, simple insert in loop or one multi-value insert.
	// For production we'd use CopyFrom.
	for _, a := range activities {
		var clientAddr interface{}
		if a.ClientAddr != "" {
			clientAddr = a.ClientAddr
		}

		_, err := r.pool.Exec(ctx, `
			INSERT INTO monitor.pg_session_activity_ts
			(ts, instance_id, dbname, pid, usename, application_name, client_addr, state, wait_event_type, wait_event, backend_type, query_id, query, xact_start, query_start, state_change, backend_start)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			a.TS, a.InstanceID, a.DBName, a.PID, a.Username, a.ApplicationName, clientAddr, a.State, a.WaitEventType, a.WaitEvent, a.BackendType, a.QueryID, a.Query, a.XactStart, a.QueryStart, a.StateChange, a.BackendStart)
		if err != nil {
			return fmt.Errorf("failed to save session activity: %w", err)
		}
	}
	return nil
}

func (r *PostgresObservabilityRepository) SaveWaitEventSummary(ctx context.Context, summaries []entities.WaitEventSummary) error {
	for _, s := range summaries {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO monitor.pg_wait_event_summary_ts (ts, instance_id, wait_event_type, wait_event, sessions)
			VALUES ($1, $2, $3, $4, $5)`,
			s.TS, s.InstanceID, s.WaitEventType, s.WaitEvent, s.Sessions)
		if err != nil {
			return fmt.Errorf("failed to save wait summary: %w", err)
		}
	}
	return nil
}

func (r *PostgresObservabilityRepository) SaveDBLoad(ctx context.Context, load entities.DBLoad) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO monitor.pg_db_load_ts (ts, instance_id, active_sessions, cpu_sessions, waiting_sessions, io_sessions, lock_sessions, idle_in_txn)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		load.TS, load.InstanceID, load.ActiveSessions, load.CPUSessions, load.WaitingSessions, load.IOWaitSessions, load.LockWaitSessions, load.IdleInTxn)
	return err
}

func (r *PostgresObservabilityRepository) SaveQueryWaitProfile(ctx context.Context, profiles []entities.QueryWaitProfile) error {
	for _, p := range profiles {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO monitor.pg_query_wait_profile_ts (ts, instance_id, queryid, calls, total_exec_time, mean_exec_time, rows, shared_blks_hit, shared_blks_read, temp_blks_written, query, usename)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			p.TS, p.InstanceID, p.QueryID, p.Calls, p.TotalExecTime, p.MeanExecTime, p.Rows, p.SharedBlksHit, p.SharedBlksRead, p.TempBlksWritten, p.Query, p.Username)
		if err != nil {
			return fmt.Errorf("failed to save query wait profile: %w", err)
		}
	}
	return nil
}

func (r *PostgresObservabilityRepository) GetKPIData(ctx context.Context, instance string, from, to string) (map[string]interface{}, error) {
	query := `
		SELECT 
			COALESCE(round(avg(active_sessions),2), 0) as avg_active,
			COALESCE(round(avg(waiting_sessions),2), 0) as avg_waiting,
			COALESCE(round(avg(idle_in_txn),2), 0) as avg_idle,
			COALESCE(max(active_sessions + waiting_sessions + idle_in_txn), 0) as max_conn
		FROM monitor.pg_db_load_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3`
	
	var avgActive, avgWaiting, avgIdle float64
	var maxConn int
	err := r.pool.QueryRow(ctx, query, from, to, instance).Scan(&avgActive, &avgWaiting, &avgIdle, &maxConn)
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

func (r *PostgresObservabilityRepository) GetLoadTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT ts, 
		       COALESCE(active_sessions, 0), 
		       COALESCE(cpu_sessions, 0), 
		       COALESCE(waiting_sessions, 0), 
		       COALESCE(io_sessions, 0), 
		       COALESCE(lock_sessions, 0), 
		       COALESCE(idle_in_txn, 0)
		FROM monitor.pg_db_load_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		ORDER BY ts ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
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
			"ts":                ts,
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

func (r *PostgresObservabilityRepository) GetWaitCategoryTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('5 min', ts) AS bucket,
		       wait_event_type,
		       sum(sessions) as sessions
		FROM monitor.pg_wait_event_summary_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY bucket, wait_event_type
		ORDER BY bucket ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
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

func (r *PostgresObservabilityRepository) GetWaitCategoryDistribution(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT wait_event_type, avg(sessions) as avg_sessions
		FROM monitor.pg_wait_event_summary_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY wait_event_type`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
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

func (r *PostgresObservabilityRepository) GetConnectionsByApp(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT application_name, count(*) as count
		FROM monitor.pg_session_activity_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
		GROUP BY application_name
		ORDER BY count DESC LIMIT 10`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
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

func (r *PostgresObservabilityRepository) GetLongRunningSessions(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT pid, usename, now() - query_start as duration, wait_event, query, application_name
		FROM monitor.pg_session_activity_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3 AND state='active'
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY duration DESC LIMIT 20`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var pid int
		var user, waitEvent, queryStr, appName string
		var duration interface{} // Interval
		if err := rows.Scan(&pid, &user, &duration, &waitEvent, &queryStr, &appName); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"pid":              pid,
			"usename":          user,
			"duration":         duration,
			"wait_event":       waitEvent,
			"query":            queryStr,
			"application_name": appName,
		})
	}
	return results, nil
}

func (r *PostgresObservabilityRepository) GetTopQueries(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT ts, queryid, calls, total_exec_time, mean_exec_time, temp_blks_written, query, usename
		FROM monitor.pg_query_wait_profile_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY total_exec_time DESC LIMIT 20`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
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
		if queryStr != nil { qVal = *queryStr }
		uVal := ""
		if usename != nil { uVal = *usename }

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

func (r *PostgresObservabilityRepository) GetSessionStateTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('5 min', ts) AS bucket,
		       state,
		       count(*) as count
		FROM monitor.pg_session_activity_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
		GROUP BY bucket, state
		ORDER BY bucket ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
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

