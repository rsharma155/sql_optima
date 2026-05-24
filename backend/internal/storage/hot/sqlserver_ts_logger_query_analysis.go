// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB persistence layer for the SQL Server Query Analysis Dashboard —
//
//	regressions, plan instability, watched queries CRUD, and watched query snapshots/events.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Row types (storage-layer DTOs)

type SqlServerQueryRegressionRow struct {
	CaptureTime    time.Time `json:"capture_time"`
	ServerID       uuid.UUID `json:"server_id"`
	DatabaseName   string    `json:"database_name"`
	QueryHash      string    `json:"query_hash"`
	QueryText      string    `json:"query_text"`
	RegressionType string    `json:"regression_type"`
	PreviousAvg    float64   `json:"previous_avg"`
	CurrentAvg     float64   `json:"current_avg"`
	PercentChange  float64   `json:"percent_change"`
	PlanChanged    bool      `json:"plan_changed"`
}

type SqlServerPlanInstabilityRow struct {
	CaptureTime       time.Time `json:"capture_time"`
	ServerID          uuid.UUID `json:"server_id"`
	DatabaseName      string    `json:"database_name"`
	QueryHash         string    `json:"query_hash"`
	QueryText         string    `json:"query_text"`
	PlanCount         int       `json:"plan_count"`
	LastExecutionTime time.Time `json:"last_execution_time"`
}

type SqlServerWatchedQueryRow struct {
	ID             int           `json:"id"`
	ServerID       uuid.UUID     `json:"server_id"`
	DatabaseName   string        `json:"database_name"`
	QueryHash      string        `json:"query_hash"`
	ObjectID       sql.NullInt32 `json:"object_id,omitempty"`
	Name           string        `json:"name"`
	QueryText      string        `json:"query_text"`
	CreatedAt      time.Time     `json:"created_at"`
	LastExecutedAt sql.NullTime  `json:"last_executed_at,omitempty"`
}

type SqlServerWatchedSnapshotRow struct {
	CaptureTimestamp  time.Time   `json:"timestamp"`
	WatchedID         int         `json:"watched_id"`
	ServerID          uuid.UUID   `json:"server_id"`
	Executions        int64       `json:"executions"`
	AvgDurationMs     float64     `json:"avg_duration_ms"`
	AvgCpuMs          float64     `json:"avg_cpu_ms"`
	AvgReads          float64     `json:"avg_reads"`
	TotalDurationMs   float64     `json:"total_duration_ms"`
	TotalCpuMs        float64     `json:"total_cpu_ms"`
	PlanCount         int         `json:"plan_count"`
	LastExecutionTime time.Time   `json:"last_execution_time"`
	QueryPlan         string      `json:"query_plan"`
	QueryText         string      `json:"query_text"`
	WaitStats         interface{} `json:"wait_stats"`
}

type SqlServerWatchedEventRow struct {
	ID        int
	WatchedID int
	EventTime time.Time
	EventType string
	Notes     string
}

// Regression writes & reads

func (tl *TimescaleLogger) LogSqlServerQueryRegressions(ctx context.Context, rows []SqlServerQueryRegressionRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO sqlserver_query_regressions
		(capture_timestamp, server_id, database_name, query_hash, query_text,
		 regression_type, previous_avg, current_avg, percent_change, plan_changed)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	batch := &pgx.Batch{}
	for _, r := range rows {
		var qh int64
		if h, err := strconv.ParseUint(strings.TrimPrefix(r.QueryHash, "0x"), 16, 64); err == nil {
			qh = int64(h)
		}
		batch.Queue(q, r.CaptureTime, r.ServerID, r.DatabaseName, qh,
			r.QueryText, r.RegressionType, r.PreviousAvg, r.CurrentAvg, r.PercentChange, r.PlanChanged)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("LogSqlServerQueryRegressions batch exec: %w", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSqlServerQueryRegressions(ctx context.Context, serverID uuid.UUID, limit int) ([]SqlServerQueryRegressionRow, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT capture_timestamp, server_id, database_name, query_hash, query_text,
		regression_type, previous_avg, current_avg, percent_change, plan_changed
		FROM sqlserver_query_regressions
		WHERE server_id = $1
		  AND query_text NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY capture_timestamp DESC
		LIMIT $2`

	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerQueryRegressions: %w", err)
	}
	defer rows.Close()

	results := make([]SqlServerQueryRegressionRow, 0)
	for rows.Next() {
		var r SqlServerQueryRegressionRow
		var qh int64
		if err := rows.Scan(&r.CaptureTime, &r.ServerID, &r.DatabaseName, &qh,
			&r.QueryText, &r.RegressionType, &r.PreviousAvg, &r.CurrentAvg, &r.PercentChange, &r.PlanChanged); err != nil {
			slog.Info("[TSLogger] GetSqlServerQueryRegressions scan", "err", err)
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) CountSqlServerRegressionsInWindow(ctx context.Context, serverID uuid.UUID, since time.Time) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sqlserver_query_regressions WHERE server_id = $1 AND capture_timestamp >= $2`,
		serverID, since).Scan(&count)
	return count, err
}

// Plan instability writes & reads

func (tl *TimescaleLogger) LogSqlServerPlanInstability(ctx context.Context, rows []SqlServerPlanInstabilityRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO sqlserver_plan_instability
		(capture_timestamp, server_id, database_name, query_hash, query_text, plan_count, last_execution_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`

	batch := &pgx.Batch{}
	for _, r := range rows {
		var qh int64
		if h, err := strconv.ParseUint(strings.TrimPrefix(r.QueryHash, "0x"), 16, 64); err == nil {
			qh = int64(h)
		}
		batch.Queue(q, r.CaptureTime, r.ServerID, r.DatabaseName, qh,
			r.QueryText, r.PlanCount, r.LastExecutionTime)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("LogSqlServerPlanInstability batch exec: %w", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSqlServerPlanInstability(ctx context.Context, serverID uuid.UUID, limit int) ([]SqlServerPlanInstabilityRow, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT capture_timestamp, server_id, database_name, query_hash, query_text, plan_count, last_execution_time
		FROM (
			SELECT DISTINCT ON (query_hash) 
				capture_timestamp, server_id, database_name, query_hash, query_text, plan_count, last_execution_time
			FROM sqlserver_plan_instability
			WHERE server_id = $1
			  AND query_text NOT LIKE '%/* SQL_OPTIMA */%'
			ORDER BY query_hash, capture_timestamp DESC
		) sub
		ORDER BY capture_timestamp DESC
		LIMIT $2`

	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerPlanInstability: %w", err)
	}
	defer rows.Close()

	results := make([]SqlServerPlanInstabilityRow, 0)
	for rows.Next() {
		var r SqlServerPlanInstabilityRow
		var qh int64
		if err := rows.Scan(&r.CaptureTime, &r.ServerID, &r.DatabaseName, &qh,
			&r.QueryText, &r.PlanCount, &r.LastExecutionTime); err != nil {
			slog.Info("[TSLogger] GetSqlServerPlanInstability scan", "err", err)
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) CountSqlServerPlanInstabilityInWindow(ctx context.Context, serverID uuid.UUID, since time.Time) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT query_hash) FROM sqlserver_plan_instability WHERE server_id = $1 AND capture_timestamp >= $2`,
		serverID, since).Scan(&count)
	return count, err
}

// Watched queries CRUD

func (tl *TimescaleLogger) InsertSqlServerWatchedQuery(ctx context.Context, row SqlServerWatchedQueryRow) (int, error) {
	var id int
	var qh int64
	if h, err := strconv.ParseUint(strings.TrimPrefix(row.QueryHash, "0x"), 16, 64); err == nil {
		qh = int64(h)
	}
	err := tl.pool.QueryRow(ctx,
		`INSERT INTO sqlserver_watched_queries (server_id, database_name, query_hash, object_id, name, query_text)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		row.ServerID, row.DatabaseName, qh, row.ObjectID, row.Name, row.QueryText,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("InsertSqlServerWatchedQuery: %w", err)
	}
	return id, nil
}

func (tl *TimescaleLogger) UpdateSqlServerWatchedQueryText(ctx context.Context, id int, text string) error {
	_, err := tl.pool.Exec(ctx, `UPDATE sqlserver_watched_queries SET query_text = $1 WHERE id = $2`, text, id)
	return err
}

func (tl *TimescaleLogger) DeleteSqlServerWatchedQuery(ctx context.Context, id int) error {
	tag, err := tl.pool.Exec(ctx, `DELETE FROM sqlserver_watched_queries WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("DeleteSqlServerWatchedQuery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("watched query %d not found", id)
	}
	return nil
}

func (tl *TimescaleLogger) ListSqlServerWatchedQueries(ctx context.Context, serverID uuid.UUID) ([]SqlServerWatchedQueryRow, error) {
	const q = `
		SELECT q.id, q.server_id, COALESCE(q.database_name,'') as database_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(q.query_text,'') as query_text, q.created_at,
		       (SELECT MAX(last_execution_time) FROM sqlserver_watched_query_snapshots s WHERE s.watched_id = q.id) as last_executed
		FROM sqlserver_watched_queries q
		WHERE q.server_id = $1
		ORDER BY q.created_at DESC`

	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, fmt.Errorf("ListSqlServerWatchedQueries: %w", err)
	}
	defer rows.Close()

	var results []SqlServerWatchedQueryRow
	for rows.Next() {
		var r SqlServerWatchedQueryRow
		var qh int64
		if err := rows.Scan(&r.ID, &r.ServerID, &r.DatabaseName, &qh, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt); err != nil {
			slog.Info("[TSLogger] ListSqlServerWatchedQueries scan", "err", err)
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetSqlServerWatchedQuery(ctx context.Context, id int) (*SqlServerWatchedQueryRow, error) {
	var r SqlServerWatchedQueryRow
	const q = `
		SELECT q.id, q.server_id, COALESCE(q.database_name,'') as database_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(q.query_text,'') as query_text,
		       q.created_at,
		       (SELECT MAX(last_execution_time) FROM sqlserver_watched_query_snapshots snap WHERE snap.watched_id = q.id) as last_executed
		FROM sqlserver_watched_queries q
		WHERE q.id = $1`

	var qh int64
	err := tl.pool.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.ServerID, &r.DatabaseName, &qh, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWatchedQuery(%d): %w", id, err)
	}
	r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
	return &r, nil
}

func (tl *TimescaleLogger) CountSqlServerWatchedQueries(ctx context.Context, serverID uuid.UUID) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sqlserver_watched_queries WHERE server_id = $1`, serverID,
	).Scan(&count)
	return count, err
}

// GetLastWatchedQueryPlans returns the most recent query_plan for each watched ID.
// Used by the collector to skip inserting a snapshot when the plan is unchanged.
func (tl *TimescaleLogger) GetLastWatchedQueryPlans(ctx context.Context, watchedIDs []int) (map[int]string, error) {
	if len(watchedIDs) == 0 {
		return nil, nil
	}
	rows, err := tl.pool.Query(ctx,
		`SELECT DISTINCT ON (watched_id) watched_id, COALESCE(query_plan, '')
		 FROM sqlserver_watched_query_snapshots
		 WHERE watched_id = ANY($1)
		 ORDER BY watched_id, capture_timestamp DESC`,
		watchedIDs)
	if err != nil {
		return nil, fmt.Errorf("GetLastWatchedQueryPlans: %w", err)
	}
	defer rows.Close()

	result := make(map[int]string, len(watchedIDs))
	for rows.Next() {
		var id int
		var plan string
		if err := rows.Scan(&id, &plan); err != nil {
			continue
		}
		result[id] = plan
	}
	return result, rows.Err()
}

func (tl *TimescaleLogger) LogSqlServerWatchedQuerySnapshot(ctx context.Context, rows []SqlServerWatchedSnapshotRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO sqlserver_watched_query_snapshots
		(capture_timestamp, watched_id, server_id, executions, avg_duration_ms, avg_cpu_ms,
		 avg_reads, total_duration_ms, total_cpu_ms, plan_count, last_execution_time, query_plan, wait_stats)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.CaptureTimestamp, r.WatchedID, r.ServerID, r.Executions, r.AvgDurationMs, r.AvgCpuMs,
			r.AvgReads, r.TotalDurationMs, r.TotalCpuMs, r.PlanCount, r.LastExecutionTime, r.QueryPlan, r.WaitStats)
	}

	res := tl.pool.SendBatch(ctx, batch)
	return res.Close()
}

func (tl *TimescaleLogger) GetSqlServerWatchedQuerySnapshots(ctx context.Context, watchedID int, from, to time.Time) ([]SqlServerWatchedSnapshotRow, error) {
	const q = `SELECT capture_timestamp, watched_id, server_id, executions,
		avg_duration_ms, avg_cpu_ms, avg_reads, total_duration_ms, total_cpu_ms,
		plan_count, last_execution_time, COALESCE(query_plan, ''), wait_stats
		FROM sqlserver_watched_query_snapshots
		WHERE watched_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC`

	rows, err := tl.pool.Query(ctx, q, watchedID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWatchedQuerySnapshots: %w", err)
	}
	defer rows.Close()

	var results []SqlServerWatchedSnapshotRow
	for rows.Next() {
		var r SqlServerWatchedSnapshotRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.WatchedID, &r.ServerID, &r.Executions,
			&r.AvgDurationMs, &r.AvgCpuMs, &r.AvgReads, &r.TotalDurationMs,
			&r.TotalCpuMs, &r.PlanCount, &r.LastExecutionTime, &r.QueryPlan, &r.WaitStats); err != nil {
			slog.Info("[TSLogger] GetSqlServerWatchedQuerySnapshots scan", "err", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// Watched query events

func (tl *TimescaleLogger) InsertSqlServerWatchedQueryEvent(ctx context.Context, row SqlServerWatchedEventRow) error {
	_, err := tl.pool.Exec(ctx,
		`INSERT INTO sqlserver_watched_query_events (watched_id, event_time, event_type, notes)
		 VALUES ($1, $2, $3, $4)`,
		row.WatchedID, row.EventTime, row.EventType, row.Notes,
	)
	if err != nil {
		return fmt.Errorf("InsertSqlServerWatchedQueryEvent: %w", err)
	}
	return nil
}

func (tl *TimescaleLogger) GetSqlServerWatchedQueryEvents(ctx context.Context, watchedID int) ([]SqlServerWatchedEventRow, error) {
	const q = `SELECT id, watched_id, event_time, event_type, COALESCE(notes,'')
		FROM sqlserver_watched_query_events
		WHERE watched_id = $1
		ORDER BY event_time DESC`

	rows, err := tl.pool.Query(ctx, q, watchedID)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWatchedQueryEvents: %w", err)
	}
	defer rows.Close()

	var results []SqlServerWatchedEventRow
	for rows.Next() {
		var r SqlServerWatchedEventRow
		if err := rows.Scan(&r.ID, &r.WatchedID, &r.EventTime, &r.EventType, &r.Notes); err != nil {
			slog.Info("[TSLogger] GetSqlServerWatchedQueryEvents scan", "err", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// Query Analysis Summary (aggregated from existing tables)

func (tl *TimescaleLogger) GetSqlServerQueryAnalysisSummary(ctx context.Context, serverID uuid.UUID, hours int, excludeSystem bool) (*SqlServerQueryAnalysisSummaryRow, error) {
	var s SqlServerQueryAnalysisSummaryRow
	if hours <= 0 {
		hours = 24
	}

	filterClause := ""
	if excludeSystem {
		filterClause = `
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'SELECT % FROM SYS.%'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'SELECT % FROM MSDB.%'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'SELECT % FROM INFORMATION_SCHEMA.%'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'EXEC %'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'FETCH %'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE '(@%'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'SET %'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'DECLARE %'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'CHECKPOINT%'
		  AND UPPER(COALESCE(qm.statement_text, '')) NOT LIKE 'DBCC %'
		  AND COALESCE(qm.query_text_raw, '') NOT LIKE '%/* SQL_OPTIMA%'
		  AND COALESCE(qm.query_text_raw, '') NOT LIKE '%sys.all_objects%'
		  AND COALESCE(qm.query_text_raw, '') NOT LIKE '(@_msparam_0%'
		  AND (qm.total_cpu_ms IS NULL OR qm.total_cpu_ms > 1 OR qm.total_elapsed_ms > 1)
		  AND (qm.application_name IS NULL OR (
		       UPPER(qm.application_name) NOT LIKE 'SQL SERVER PROFILER%' AND
		       UPPER(qm.application_name) NOT LIKE 'MICROSOFT SQL SERVER MANAGEMENT STUDIO%' AND
		       UPPER(qm.application_name) NOT LIKE 'SQLAGENT%'
		  ))
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		`
	}

	// Core KPIs from history.
	// Use LATERAL to pick one metrics_v2 row per query_hash (most recent), avoiding a
	// Cartesian product between the hypertable and the unconstrained LEFT JOIN.
	q1 := fmt.Sprintf(`
		SELECT
			COALESCE(SUM(qh.exec_delta), 0),
			COALESCE(SUM(qh.cpu_delta_ms) / NULLIF(SUM(qh.exec_delta), 0), 0),
			COALESCE(SUM(qh.cpu_delta_ms) / NULLIF(SUM(qh.exec_delta), 0), 0),
			COALESCE(SUM(qh.reads_delta) / NULLIF(SUM(qh.exec_delta), 0), 0),
			COALESCE(COUNT(DISTINCT qh.query_hash), 0)
		FROM sqlserver_query_stats_history qh
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.server_id = qh.server_id
		 AND class.query_hash = qh.query_hash
		LEFT JOIN LATERAL (
			SELECT query_text_raw, statement_text, application_name,
			       total_cpu_ms, total_elapsed_ms, is_user_workload
			FROM sqlserver_query_metrics_v2
			WHERE server_id = qh.server_id AND query_hash = qh.query_hash
			ORDER BY capture_timestamp DESC
			LIMIT 1
		) qm ON true
		WHERE qh.server_id = $1
		  AND qh.capture_timestamp >= NOW() - ($2 * INTERVAL '1 hour')
		  AND COALESCE(qm.is_user_workload, 1) = 1
		  %s`, filterClause)

	err := tl.pool.QueryRow(ctx, q1, serverID, hours).Scan(&s.TotalExecutions, &s.AvgCPU, &s.AvgDuration, &s.AvgReads, &s.QueriesExecutedInRange)
	if err != nil {
		slog.Info("[TSLogger] GetSqlServerQueryAnalysisSummary history", "err", err)
	}

	// Top 10 CPU Share
	if err := tl.pool.QueryRow(ctx, `
		WITH total AS (
			SELECT SUM(qh.cpu_delta_ms) as total_cpu
			FROM sqlserver_query_stats_history qh
			LEFT JOIN LATERAL (
				SELECT is_user_workload FROM sqlserver_query_metrics_v2
				WHERE server_id = qh.server_id AND query_hash = qh.query_hash
				ORDER BY capture_timestamp DESC LIMIT 1
			) qm ON true
			WHERE qh.server_id = $1 AND qh.capture_timestamp >= NOW() - ($2 * INTERVAL '1 hour')
			  AND COALESCE(qm.is_user_workload, 1) = 1
		),
		top10 AS (
			SELECT SUM(sub.query_cpu) as top_cpu
			FROM (
				SELECT SUM(qh.cpu_delta_ms) as query_cpu
				FROM sqlserver_query_stats_history qh
				LEFT JOIN LATERAL (
					SELECT is_user_workload FROM sqlserver_query_metrics_v2
					WHERE server_id = qh.server_id AND query_hash = qh.query_hash
					ORDER BY capture_timestamp DESC LIMIT 1
				) qm ON true
				WHERE qh.server_id = $1 AND qh.capture_timestamp >= NOW() - ($2 * INTERVAL '1 hour')
				  AND COALESCE(qm.is_user_workload, 1) = 1
				GROUP BY qh.query_hash
				ORDER BY query_cpu DESC
				LIMIT 10
			) sub
		)
		SELECT COALESCE((top_cpu::float8 / NULLIF(total_cpu, 0)) * 100, 0)
		FROM total, top10`,
		serverID, hours,
	).Scan(&s.Top10CpuSharePct); err != nil {
		slog.Info("[TSLogger] GetSqlServerQueryAnalysisSummary top10 cpu share", "err", err)
	}

	// Total Queries in Snapshot (User queries)
	q3 := fmt.Sprintf(`
		SELECT COUNT(DISTINCT s.query_hash)
		FROM sqlserver_query_stats_snapshot_v2 s
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.server_id = s.server_id AND class.query_hash = s.query_hash
		LEFT JOIN LATERAL (
			SELECT query_text_raw, statement_text, application_name,
			       total_cpu_ms, total_elapsed_ms, is_user_workload
			FROM sqlserver_query_metrics_v2
			WHERE server_id = s.server_id AND query_hash = s.query_hash
			ORDER BY capture_timestamp DESC LIMIT 1
		) qm ON true
		WHERE s.server_id = $1
		  AND COALESCE(qm.is_user_workload, 1) = 1
		  %s`, filterClause)
	_ = tl.pool.QueryRow(ctx, q3, serverID).Scan(&s.TotalQueriesInQS)

	// Queries with Single Execution in range
	_ = tl.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT qh.query_hash
			FROM sqlserver_query_stats_history qh
			LEFT JOIN LATERAL (
				SELECT is_user_workload FROM sqlserver_query_metrics_v2
				WHERE server_id = qh.server_id AND query_hash = qh.query_hash
				ORDER BY capture_timestamp DESC LIMIT 1
			) qm ON true
			WHERE qh.server_id = $1 AND qh.capture_timestamp >= NOW() - ($2 * INTERVAL '1 hour')
			  AND COALESCE(qm.is_user_workload, 1) = 1
			GROUP BY qh.query_hash
			HAVING SUM(qh.exec_delta) = 1
		) sub`,
		serverID, hours,
	).Scan(&s.QueriesSingleExecution)

	// Regression count (last 24h)
	since24h := time.Now().UTC().Add(-24 * time.Hour)
	s.Regressions24h, _ = tl.CountSqlServerRegressionsInWindow(ctx, serverID, since24h)

	// Plan instability count (last 24h)
	s.PlanChanges24h, _ = tl.CountSqlServerPlanInstabilityInWindow(ctx, serverID, since24h)
	s.QueriesWithMultiPlans = int64(s.PlanChanges24h)

	// Query Store status check for message (Phase 2 request)
	// If no user queries are in QS, check if it's because it's disabled globally.
	if s.TotalQueriesInQS == 0 {
		var qsEnabledCount int
		err := tl.pool.QueryRow(ctx, 
			"SELECT COUNT(*) FROM sqlserver_database_catalog WHERE server_id = $1 AND is_query_store_on = true AND database_id > 4",
			serverID).Scan(&qsEnabledCount)
		if err == nil && qsEnabledCount == 0 {
			s.Message = "No Query Store data detected: Ensure Query Store is enabled on your target databases"
		}
	}

	return &s, nil
}

type SqlServerQueryAnalysisSummaryRow struct {
	TotalExecutions        int64   `json:"total_executions"`
	AvgDuration            float64 `json:"avg_duration_ms"`
	AvgCPU                 float64 `json:"avg_cpu_ms"`
	AvgReads               float64 `json:"avg_reads"`
	Regressions24h         int     `json:"regressions_24h"`
	PlanChanges24h         int     `json:"plan_changes_24h"`
	Top10CpuSharePct       float64 `json:"top_10_cpu_share_pct"`
	TotalQueriesInQS       int64   `json:"total_queries_in_qs"`
	QueriesExecutedInRange int64   `json:"queries_executed_in_range"`
	QueriesWithMultiPlans  int64   `json:"queries_with_multi_plans"`
	QueriesSingleExecution int64   `json:"queries_single_execution"`
	Message                string  `json:"message,omitempty"`
}

func (tl *TimescaleLogger) GetSqlServerTopQueriesFromInterval(ctx context.Context, serverID uuid.UUID, sortBy string, limit, hours int, excludeSystem bool) ([]SqlServerTopQueryIntervalRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if hours <= 0 {
		hours = 24
	}

	orderClause := "SUM(total_cpu_ms) DESC"
	switch sortBy {
	case "duration":
		orderClause = "AVG(total_elapsed_ms / NULLIF(total_executions, 0)) DESC"
	case "reads":
		orderClause = "AVG(total_logical_reads / NULLIF(total_executions, 0)) DESC"
	case "executions":
		orderClause = "SUM(total_executions) DESC"
	case "cpu":
		orderClause = "SUM(total_cpu_ms) DESC"
	}

	filterClause := ""
	if excludeSystem {
		filterClause = `
		  AND UPPER(q.query_text_raw) NOT LIKE 'SELECT %% FROM SYS.%%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'SELECT %% FROM [SYS].%%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'SELECT %% FROM MSDB.%%'
		  AND UPPER(q.query_text_raw) NOT LIKE '(@%%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'DBCC %%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'CHECKPOINT%%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'DECLARE %%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'CREATE %%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'ALTER %%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'BACKUP %%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'RESTORE %%'
		  AND UPPER(q.query_text_raw) NOT LIKE '%%WAITFOR%%'
		  AND q.query_text_raw NOT LIKE '%%/* SQL_OPTIMA%%'
		  AND q.query_text_raw NOT LIKE '%%sys.all_objects%%'
		  AND q.query_text_raw NOT LIKE '%%[sys].all_objects%%'
		  AND q.query_text_raw NOT LIKE '(@_msparam_0%%'
		  AND (q.total_cpu_ms > 20 OR q.total_elapsed_ms > 20)
		  AND (q.application_name IS NULL OR (
		       UPPER(q.application_name) NOT LIKE 'SQL SERVER PROFILER%%' AND
		       UPPER(q.application_name) NOT LIKE 'SQLAGENT%%' AND
		       UPPER(q.application_name) NOT LIKE 'CORE DATA SERVICES%%' AND
		       UPPER(q.application_name) NOT LIKE 'SQLSERVERCE%%'
		  ))
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		`
	}

	q := fmt.Sprintf(`
		SELECT q.query_hash as query_hash, 
		       MAX(q.statement_text) as statement_text, 
		       q.database_name as database_name,
		       MAX(q.login_name) as login_name, 
		       MAX(q.application_name) as application_name,
		       SUM(q.total_executions)::bigint as total_executions, 
		       COALESCE(AVG(q.total_cpu_ms::float8 / NULLIF(q.total_executions, 0)), 0) as avg_cpu, 
		       COALESCE(AVG(q.total_elapsed_ms::float8 / NULLIF(q.total_executions, 0)), 0) as avg_elapsed,
		       COALESCE(AVG(q.total_logical_reads::float8 / NULLIF(q.total_executions, 0)), 0) as avg_logical_reads, 
		       COALESCE(SUM(q.total_cpu_ms)::float8, 0) as total_cpu,
		       MAX(q.last_execution_time) as last_execution_time
		FROM sqlserver_query_metrics_v2 q
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.server_id = q.server_id
		 AND class.query_hash = q.query_hash
		WHERE q.server_id = $1
		  AND q.capture_timestamp >= NOW() - ($3 * INTERVAL '1 hour')
		  AND q.query_text_raw NOT LIKE '%%/* SQL_OPTIMA%%'
		  AND q.statement_text NOT LIKE '%%sys.all_objects%%'
		  AND q.statement_text NOT LIKE '%%sp_MShistory_cleanup%%'
		  AND (q.total_cpu_ms > 20 OR q.total_elapsed_ms > 20)
		  AND (q.login_name IS NULL OR q.login_name <> 'dbmonitor_user')
		  AND COALESCE(q.is_user_workload, 1) = 1
		  %s
		GROUP BY q.query_hash, q.database_name
		ORDER BY %s
		LIMIT $2`, filterClause, orderClause)

	rows, err := tl.pool.Query(ctx, q, serverID, limit, hours)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerTopQueriesFromInterval: %w", err)
	}
	defer rows.Close()

	results := make([]SqlServerTopQueryIntervalRow, 0)
	for rows.Next() {
		var r SqlServerTopQueryIntervalRow
		var qh int64
		if err := rows.Scan(&qh, &r.QueryText, &r.DatabaseName,
			&r.LoginName, &r.ApplicationName,
			&r.Executions, &r.AvgCpuMs, &r.AvgDurationMs, &r.AvgReads, &r.TotalCpuMs, &r.LastExecutionTime); err != nil {
			slog.Info("[TSLogger] GetSqlServerTopQueriesFromInterval scan", "err", err)
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		results = append(results, r)
	}
	return results, rows.Err()
}

type SqlServerTopQueryIntervalRow struct {
	QueryHash         string     `json:"query_hash"`
	QueryText         string     `json:"query_text"`
	DatabaseName      string     `json:"database_name"`
	LoginName         string     `json:"login_name"`
	ApplicationName   string     `json:"application_name"`
	Executions        int64      `json:"executions"`
	AvgCpuMs          float64    `json:"avg_cpu_ms"`
	AvgDurationMs     float64    `json:"avg_duration_ms"`
	AvgReads          float64    `json:"avg_reads"`
	TotalCpuMs        float64    `json:"total_cpu_ms"`
	LastExecutionTime *time.Time `json:"last_execution_time"`
}

func (tl *TimescaleLogger) GetSQLServerQueryTrend(ctx context.Context, serverID uuid.UUID, sqlHash string, from, to time.Time) (map[string]interface{}, error) {
	var qh int64
	if h, err := strconv.ParseUint(strings.TrimPrefix(sqlHash, "0x"), 16, 64); err == nil {
		qh = int64(h)
	}

	// Dynamic bucket size for zoom support
	dur := to.Sub(from)
	bucket := "1 minute"
	if dur > 24*time.Hour {
		bucket = "15 minutes"
	} else if dur > 6*time.Hour {
		bucket = "5 minutes"
	}

	q := fmt.Sprintf(`
		SELECT time_bucket('%s', capture_timestamp) AS bucket, 
		       SUM(exec_delta), SUM(cpu_delta_ms), SUM(duration_delta_ms), SUM(reads_delta)
		FROM sqlserver_query_stats_history
		WHERE server_id = $1 AND query_hash = $2 AND capture_timestamp BETWEEN $3 AND $4
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucket)
	rows, err := tl.pool.Query(ctx, q, serverID, qh, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var exec, cpu, dur, reads int64
		if err := rows.Scan(&ts, &exec, &cpu, &dur, &reads); err != nil {
			continue
		}
		points = append(points, map[string]interface{}{
			"timestamp":   ts,
			"executions":  exec,
			"cpu_ms":      cpu,
			"duration_ms": dur,
			"reads":       reads,
		})
	}
	return map[string]interface{}{"time_series": points}, rows.Err()
}
