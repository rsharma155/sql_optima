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
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ────────────────────────────────────────────────
// Row types (storage-layer DTOs)
// ────────────────────────────────────────────────

// SqlServerQueryRegressionRow is the storage-layer DTO for sqlserver_query_regressions.
type SqlServerQueryRegressionRow struct {
	CaptureTime    time.Time
	InstanceName   string
	DatabaseName   string
	QueryHash      string
	QueryText      string
	RegressionType string
	PreviousAvg    float64
	CurrentAvg     float64
	PercentChange  float64
	PlanChanged    bool
}

// SqlServerPlanInstabilityRow is the storage-layer DTO for sqlserver_plan_instability.
type SqlServerPlanInstabilityRow struct {
	CaptureTime       time.Time
	InstanceName      string
	DatabaseName      string
	QueryHash         string
	QueryText         string
	PlanCount         int
	LastExecutionTime time.Time
}

// SqlServerWatchedQueryRow is the storage-layer DTO for sqlserver_watched_queries.
type SqlServerWatchedQueryRow struct {
	ID             int
	InstanceName   string
	DatabaseName   string
	QueryHash      string
	ObjectID       sql.NullInt32
	Name           string
	QueryText      string
	CreatedAt      time.Time
	LastExecutedAt sql.NullTime
}

// SqlServerWatchedSnapshotRow is the storage-layer DTO for sqlserver_watched_query_snapshots.
type SqlServerWatchedSnapshotRow struct {
	SnapshotTime      time.Time
	WatchedID         int
	InstanceName      string
	Executions        int64
	AvgDurationMs     float64
	AvgCpuMs          float64
	AvgReads          float64
	TotalDurationMs   float64
	TotalCpuMs        float64
	PlanCount         int
	LastExecutionTime time.Time
	QueryPlan         string
	QueryText         string
	WaitStats         interface{} // Map or Slice for JSONB
}

// SqlServerWatchedEventRow is the storage-layer DTO for sqlserver_watched_query_events.
type SqlServerWatchedEventRow struct {
	ID        int
	WatchedID int
	EventTime time.Time
	EventType string
	Notes     string
}

// ────────────────────────────────────────────────
// Regression writes & reads
// ────────────────────────────────────────────────

// LogSqlServerQueryRegressions batch-inserts regression detection results.
func (tl *TimescaleLogger) LogSqlServerQueryRegressions(ctx context.Context, rows []SqlServerQueryRegressionRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO sqlserver_query_regressions
		(capture_time, server_instance_name, database_name, query_hash, query_text,
		 regression_type, previous_avg, current_avg, percent_change, plan_changed)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	batch := &pgx.Batch{}
	for _, r := range rows {
		var qh int64
		if h, err := strconv.ParseUint(strings.TrimPrefix(r.QueryHash, "0x"), 16, 64); err == nil {
			qh = int64(h)
		}
		batch.Queue(q, r.CaptureTime, r.InstanceName, r.DatabaseName, qh,
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

// GetSqlServerQueryRegressions returns recent regression rows for an instance.
func (tl *TimescaleLogger) GetSqlServerQueryRegressions(ctx context.Context, instance string, limit int) ([]SqlServerQueryRegressionRow, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT capture_time, server_instance_name, database_name, query_hash, query_text,
		regression_type, previous_avg, current_avg, percent_change, plan_changed
		FROM sqlserver_query_regressions
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND query_text NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY capture_time DESC
		LIMIT $2`

	rows, err := tl.pool.Query(ctx, q, instance, limit)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerQueryRegressions: %w", err)
	}
	defer rows.Close()

	var results []SqlServerQueryRegressionRow
	for rows.Next() {
		var r SqlServerQueryRegressionRow
		var qh int64
		if err := rows.Scan(&r.CaptureTime, &r.InstanceName, &r.DatabaseName, &qh,
			&r.QueryText, &r.RegressionType, &r.PreviousAvg, &r.CurrentAvg, &r.PercentChange, &r.PlanChanged); err != nil {
			log.Printf("[TSLogger] GetSqlServerQueryRegressions scan: %v", err)
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		results = append(results, r)
	}
	return results, rows.Err()
}

// CountSqlServerRegressionsInWindow returns the number of regressions for an instance in a time window.
func (tl *TimescaleLogger) CountSqlServerRegressionsInWindow(ctx context.Context, instance string, since time.Time) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sqlserver_query_regressions WHERE UPPER(server_instance_name) = UPPER($1) AND capture_time >= $2`,
		instance, since).Scan(&count)
	return count, err
}

// ────────────────────────────────────────────────
// Plan instability writes & reads
// ────────────────────────────────────────────────

// LogSqlServerPlanInstability batch-inserts plan instability detection results.
func (tl *TimescaleLogger) LogSqlServerPlanInstability(ctx context.Context, rows []SqlServerPlanInstabilityRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO sqlserver_plan_instability
		(capture_time, server_instance_name, database_name, query_hash, query_text, plan_count, last_execution_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`

	batch := &pgx.Batch{}
	for _, r := range rows {
		var qh int64
		if h, err := strconv.ParseUint(strings.TrimPrefix(r.QueryHash, "0x"), 16, 64); err == nil {
			qh = int64(h)
		}
		batch.Queue(q, r.CaptureTime, r.InstanceName, r.DatabaseName, qh,
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

// GetSqlServerPlanInstability returns recent plan instability rows for an instance.
func (tl *TimescaleLogger) GetSqlServerPlanInstability(ctx context.Context, instance string, limit int) ([]SqlServerPlanInstabilityRow, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT capture_time, server_instance_name, database_name, query_hash, query_text, plan_count, last_execution_time
		FROM (
			SELECT DISTINCT ON (query_hash) 
				capture_time, server_instance_name, database_name, query_hash, query_text, plan_count, last_execution_time
			FROM sqlserver_plan_instability
			WHERE UPPER(server_instance_name) = UPPER($1)
			  AND query_text NOT LIKE '%/* SQL_OPTIMA */%'
			ORDER BY query_hash, capture_time DESC
		) sub
		ORDER BY capture_time DESC
		LIMIT $2`

	rows, err := tl.pool.Query(ctx, q, instance, limit)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerPlanInstability: %w", err)
	}
	defer rows.Close()

	var results []SqlServerPlanInstabilityRow
	for rows.Next() {
		var r SqlServerPlanInstabilityRow
		var qh int64
		if err := rows.Scan(&r.CaptureTime, &r.InstanceName, &r.DatabaseName, &qh,
			&r.QueryText, &r.PlanCount, &r.LastExecutionTime); err != nil {
			log.Printf("[TSLogger] GetSqlServerPlanInstability scan: %v", err)
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		results = append(results, r)
	}
	return results, rows.Err()
}

// CountSqlServerPlanInstabilityInWindow returns the number of unstable queries for an instance in a time window.
func (tl *TimescaleLogger) CountSqlServerPlanInstabilityInWindow(ctx context.Context, instance string, since time.Time) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT query_hash) FROM sqlserver_plan_instability WHERE UPPER(server_instance_name) = UPPER($1) AND capture_time >= $2`,
		instance, since).Scan(&count)
	return count, err
}

// ────────────────────────────────────────────────
// Watched queries CRUD
// ────────────────────────────────────────────────

// InsertSqlServerWatchedQuery adds a query to the watch list and returns the new ID.
func (tl *TimescaleLogger) InsertSqlServerWatchedQuery(ctx context.Context, row SqlServerWatchedQueryRow) (int, error) {
	var id int
	var qh int64
	if h, err := strconv.ParseUint(strings.TrimPrefix(row.QueryHash, "0x"), 16, 64); err == nil {
		qh = int64(h)
	}
	err := tl.pool.QueryRow(ctx,
		`INSERT INTO sqlserver_watched_queries (server_instance_name, database_name, query_hash, object_id, name, query_text)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		row.InstanceName, row.DatabaseName, qh, row.ObjectID, row.Name, row.QueryText,
	).Scan(&id)

	if err != nil && strings.Contains(err.Error(), "database_name") {
		// Fallback for missing database_name column (migration not yet applied)
		return tl.insertSqlServerWatchedQueryLegacy(ctx, row)
	}

	if err != nil {
		return 0, fmt.Errorf("InsertSqlServerWatchedQuery: %w", err)
	}
	return id, nil
}

func (tl *TimescaleLogger) insertSqlServerWatchedQueryLegacy(ctx context.Context, row SqlServerWatchedQueryRow) (int, error) {
	var id int
	var qh int64
	if h, err := strconv.ParseUint(strings.TrimPrefix(row.QueryHash, "0x"), 16, 64); err == nil {
		qh = int64(h)
	}
	err := tl.pool.QueryRow(ctx,
		`INSERT INTO sqlserver_watched_queries (server_instance_name, query_hash, object_id, name, query_text)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		row.InstanceName, qh, row.ObjectID, row.Name, row.QueryText,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("InsertSqlServerWatchedQueryLegacy: %w", err)
	}
	return id, nil
}

// UpdateSqlServerWatchedQueryText updates the full SQL text for a watched query.
func (tl *TimescaleLogger) UpdateSqlServerWatchedQueryText(ctx context.Context, id int, text string) error {
	_, err := tl.pool.Exec(ctx, `UPDATE sqlserver_watched_queries SET query_text = $1 WHERE id = $2`, text, id)
	return err
}

// DeleteSqlServerWatchedQuery removes a watched query by ID (cascades to snapshots + events).
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

func (tl *TimescaleLogger) ListSqlServerWatchedQueries(ctx context.Context, instance string) ([]SqlServerWatchedQueryRow, error) {
	// Primary attempt with database_name
	const q = `
		SELECT q.id, q.server_instance_name, COALESCE(q.database_name,'') as database_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(q.query_text,'') as query_text, q.created_at,
		       (SELECT MAX(last_execution_time) FROM sqlserver_watched_query_snapshots s WHERE s.watched_id = q.id) as last_executed
		FROM sqlserver_watched_queries q
		WHERE UPPER(q.server_instance_name) = UPPER($1)
		ORDER BY q.created_at DESC`

	rows, err := tl.pool.Query(ctx, q, instance)
	if err != nil {
		if strings.Contains(err.Error(), "database_name") {
			return tl.listSqlServerWatchedQueriesLegacy(ctx, instance)
		}
		return nil, fmt.Errorf("ListSqlServerWatchedQueries: %w", err)
	}
	defer rows.Close()

	var results []SqlServerWatchedQueryRow
	for rows.Next() {
		var r SqlServerWatchedQueryRow
		if err := rows.Scan(&r.ID, &r.InstanceName, &r.DatabaseName, &r.QueryHash, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt); err != nil {
			log.Printf("[TSLogger] ListSqlServerWatchedQueries scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) listSqlServerWatchedQueriesLegacy(ctx context.Context, instance string) ([]SqlServerWatchedQueryRow, error) {
	const q = `
		SELECT q.id, q.server_instance_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(q.query_text,'') as query_text, q.created_at,
		       (SELECT MAX(last_execution_time) FROM sqlserver_watched_query_snapshots s WHERE s.watched_id = q.id) as last_executed
		FROM sqlserver_watched_queries q
		WHERE UPPER(q.server_instance_name) = UPPER($1)
		ORDER BY q.created_at DESC`

	rows, err := tl.pool.Query(ctx, q, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SqlServerWatchedQueryRow
	for rows.Next() {
		var r SqlServerWatchedQueryRow
		if err := rows.Scan(&r.ID, &r.InstanceName, &r.QueryHash, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt); err != nil {
			continue
		}
		r.DatabaseName = "" // Field empty in legacy mode
		results = append(results, r)
	}
	return results, nil
}

// GetSqlServerWatchedQuery returns a single watched query by ID.
func (tl *TimescaleLogger) GetSqlServerWatchedQuery(ctx context.Context, id int) (*SqlServerWatchedQueryRow, error) {
	var r SqlServerWatchedQueryRow
	const q = `
		SELECT q.id, q.server_instance_name, COALESCE(q.database_name,'') as database_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(q.query_text,'') as query_text,
		       q.created_at,
		       (SELECT MAX(last_execution_time) FROM sqlserver_watched_query_snapshots snap WHERE snap.watched_id = q.id) as last_executed
		FROM sqlserver_watched_queries q
		WHERE q.id = $1`

	var qh int64
	err := tl.pool.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.InstanceName, &r.DatabaseName, &qh, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "database_name") {
			return tl.getSqlServerWatchedQueryLegacy(ctx, id)
		}
		return nil, fmt.Errorf("GetSqlServerWatchedQuery(%d): %w", id, err)
	}
	r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
	return &r, nil
}

func (tl *TimescaleLogger) getSqlServerWatchedQueryLegacy(ctx context.Context, id int) (*SqlServerWatchedQueryRow, error) {
	var r SqlServerWatchedQueryRow
	const q = `
		SELECT q.id, q.server_instance_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(q.query_text,'') as query_text,
		       q.created_at,
		       (SELECT MAX(last_execution_time) FROM sqlserver_watched_query_snapshots snap WHERE snap.watched_id = q.id) as last_executed
		FROM sqlserver_watched_queries q
		WHERE q.id = $1`

	var qh int64
	err := tl.pool.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.InstanceName, &qh, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt,
	)
	if err != nil {
		return nil, err
	}
	r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
	r.DatabaseName = ""
	return &r, nil
}

// CountSqlServerWatchedQueries returns the number of watched queries for an instance.
func (tl *TimescaleLogger) CountSqlServerWatchedQueries(ctx context.Context, instance string) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sqlserver_watched_queries WHERE UPPER(server_instance_name) = UPPER($1)`, instance,
	).Scan(&count)
	return count, err
}

// LogSqlServerWatchedQuerySnapshot batch-inserts snapshot rows for watched queries.
func (tl *TimescaleLogger) LogSqlServerWatchedQuerySnapshot(ctx context.Context, rows []SqlServerWatchedSnapshotRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO sqlserver_watched_query_snapshots
		(snapshot_time, watched_id, server_instance_name, executions, avg_duration_ms, avg_cpu_ms,
		 avg_reads, total_duration_ms, total_cpu_ms, plan_count, last_execution_time, query_plan, wait_stats)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.SnapshotTime, r.WatchedID, r.InstanceName, r.Executions, r.AvgDurationMs, r.AvgCpuMs,
			r.AvgReads, r.TotalDurationMs, r.TotalCpuMs, r.PlanCount, r.LastExecutionTime, r.QueryPlan, r.WaitStats)
	}

	res := tl.pool.SendBatch(ctx, batch)
	return res.Close()
}

// GetSqlServerWatchedQuerySnapshots returns time-series snapshots for a watched query in a time range.
func (tl *TimescaleLogger) GetSqlServerWatchedQuerySnapshots(ctx context.Context, watchedID int, from, to time.Time) ([]SqlServerWatchedSnapshotRow, error) {
	const q = `SELECT snapshot_time, watched_id, server_instance_name, executions,
		avg_duration_ms, avg_cpu_ms, avg_reads, total_duration_ms, total_cpu_ms,
		plan_count, last_execution_time, COALESCE(query_plan, ''), wait_stats
		FROM sqlserver_watched_query_snapshots
		WHERE watched_id = $1 AND snapshot_time >= $2 AND snapshot_time <= $3
		ORDER BY snapshot_time ASC`

	rows, err := tl.pool.Query(ctx, q, watchedID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWatchedQuerySnapshots: %w", err)
	}
	defer rows.Close()

	var results []SqlServerWatchedSnapshotRow
	for rows.Next() {
		var r SqlServerWatchedSnapshotRow
		if err := rows.Scan(&r.SnapshotTime, &r.WatchedID, &r.InstanceName, &r.Executions,
			&r.AvgDurationMs, &r.AvgCpuMs, &r.AvgReads, &r.TotalDurationMs,
			&r.TotalCpuMs, &r.PlanCount, &r.LastExecutionTime, &r.QueryPlan, &r.WaitStats); err != nil {
			log.Printf("[TSLogger] GetSqlServerWatchedQuerySnapshots scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ────────────────────────────────────────────────
// Watched query events
// ────────────────────────────────────────────────

// InsertSqlServerWatchedQueryEvent records an optimization event marker.
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

// GetSqlServerWatchedQueryEvents returns all events for a watched query.
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
			log.Printf("[TSLogger] GetSqlServerWatchedQueryEvents scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ────────────────────────────────────────────────
// Query Analysis Summary (aggregated from existing tables)
// ────────────────────────────────────────────────

// GetSqlServerQueryAnalysisSummary aggregates KPI data from sqlserver_query_stats_history + regression/instability counts.
func (tl *TimescaleLogger) GetSqlServerQueryAnalysisSummary(ctx context.Context, instance string, hours int, excludeSystem bool) (*SqlServerQueryAnalysisSummaryRow, error) {
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
		  AND qm.query_text_raw NOT LIKE '%/* SQL_OPTIMA%'
		  AND qm.query_text_raw NOT LIKE '%sys.all_objects%'
		  AND qm.query_text_raw NOT LIKE '(@_msparam_0%'
		  AND (qm.total_cpu_ms > 1 OR qm.total_elapsed_ms > 1)
		  AND (qm.application_name IS NULL OR (
		       UPPER(qm.application_name) NOT LIKE 'SQL SERVER PROFILER%' AND
		       UPPER(qm.application_name) NOT LIKE 'MICROSOFT SQL SERVER MANAGEMENT STUDIO%' AND
		       UPPER(qm.application_name) NOT LIKE 'SQLAGENT%'
		  ))
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		`
	}

	// 1. Core KPIs from history
	q1 := fmt.Sprintf(`
		SELECT 
			COALESCE(SUM(qh.exec_delta), 0),
			COALESCE(SUM(qh.cpu_delta_ms) / NULLIF(SUM(qh.exec_delta), 0), 0),
			COALESCE(SUM(qh.cpu_delta_ms) / NULLIF(SUM(qh.exec_delta), 0), 0),
			COALESCE(SUM(qh.reads_delta) / NULLIF(SUM(qh.exec_delta), 0), 0),
			COALESCE(COUNT(DISTINCT qh.query_hash), 0)
		FROM sqlserver_query_stats_history qh
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = qh.instance_id
		 AND class.query_hash = qh.query_hash
		LEFT JOIN sqlserver_query_metrics_v2 qm
		  ON qm.instance_id = qh.instance_id
		 AND qm.query_hash = qh.query_hash
		WHERE qh.instance_id = $1
		  AND qh.ts >= NOW() - ($2 * INTERVAL '1 hour')
		  AND COALESCE(qm.is_user_workload, 1) = 1
		  %s`, filterClause)

	err := tl.pool.QueryRow(ctx, q1, instance, hours).Scan(&s.TotalExecutions, &s.AvgCPU, &s.AvgDuration, &s.AvgReads, &s.QueriesExecutedInRange)
	if err != nil {
		log.Printf("[TSLogger] GetSqlServerQueryAnalysisSummary history: %v", err)
	}

	// 2. Top 10 CPU Share
	err = tl.pool.QueryRow(ctx, `
		WITH total AS (
			SELECT SUM(qh.cpu_delta_ms) as total_cpu
			FROM sqlserver_query_stats_history qh
			LEFT JOIN sqlserver_query_metrics_v2 qm
			  ON qm.instance_id = qh.instance_id
			 AND qm.query_hash = qh.query_hash
			WHERE qh.instance_id = $1 AND qh.ts >= NOW() - ($2 * INTERVAL '1 hour')
			  AND COALESCE(qm.is_user_workload, 1) = 1
		),
		top10 AS (
			SELECT SUM(sub.query_cpu) as top_cpu
			FROM (
				SELECT SUM(qh.cpu_delta_ms) as query_cpu
				FROM sqlserver_query_stats_history qh
				LEFT JOIN sqlserver_query_metrics_v2 qm
				  ON qm.instance_id = qh.instance_id
				 AND qm.query_hash = qh.query_hash
				WHERE qh.instance_id = $1 AND qh.ts >= NOW() - ($2 * INTERVAL '1 hour')
				  AND COALESCE(qm.is_user_workload, 1) = 1
				GROUP BY qh.query_hash
				ORDER BY query_cpu DESC
				LIMIT 10
			) sub
		)
		SELECT COALESCE((top_cpu::float8 / NULLIF(total_cpu, 0)) * 100, 0)
		FROM total, top10`,
		instance, hours,
	).Scan(&s.Top10CpuSharePct)

	// 3. Total Queries in Snapshot (User queries)
	q3 := fmt.Sprintf(`
		SELECT COUNT(DISTINCT s.query_hash)
		FROM sqlserver_query_stats_snapshot_v2 s
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = s.instance_id AND class.query_hash = s.query_hash
		LEFT JOIN sqlserver_query_metrics_v2 qm
		  ON qm.instance_id = s.instance_id 
		 AND qm.query_hash = s.query_hash
		WHERE s.instance_id = $1
		  AND COALESCE(qm.is_user_workload, 1) = 1
		  %s`, filterClause)
	_ = tl.pool.QueryRow(ctx, q3, instance).Scan(&s.TotalQueriesInQS)

	// 4. Queries with Single Execution in range
	_ = tl.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT qh.query_hash
			FROM sqlserver_query_stats_history qh
			LEFT JOIN sqlserver_query_metrics_v2 qm
			  ON qm.instance_id = qh.instance_id
			 AND qm.query_hash = qh.query_hash
			WHERE qh.instance_id = $1 AND qh.ts >= NOW() - ($2 * INTERVAL '1 hour')
			  AND COALESCE(qm.is_user_workload, 1) = 1
			GROUP BY qh.query_hash
			HAVING SUM(qh.exec_delta) = 1
		) sub`,
		instance, hours,
	).Scan(&s.QueriesSingleExecution)

	// Regression count (last 24h)
	since24h := time.Now().UTC().Add(-24 * time.Hour)
	s.Regressions24h, _ = tl.CountSqlServerRegressionsInWindow(ctx, instance, since24h)

	// Plan instability count (last 24h)
	s.PlanChanges24h, _ = tl.CountSqlServerPlanInstabilityInWindow(ctx, instance, since24h)
	s.QueriesWithMultiPlans = int64(s.PlanChanges24h)

	return &s, nil
}

// SqlServerQueryAnalysisSummaryRow is the storage-layer DTO for the summary endpoint.
type SqlServerQueryAnalysisSummaryRow struct {
	TotalExecutions        int64
	AvgDuration            float64
	AvgCPU                 float64
	AvgReads               float64
	Regressions24h         int
	PlanChanges24h         int
	Top10CpuSharePct       float64
	TotalQueriesInQS       int64
	QueriesExecutedInRange int64
	QueriesWithMultiPlans  int64
	QueriesSingleExecution int64
}

// GetSqlServerTopQueriesFromInterval returns top queries from the existing sqlserver_query_stats_interval table.
func (tl *TimescaleLogger) GetSqlServerTopQueriesFromInterval(ctx context.Context, instance, sortBy string, limit, hours int, excludeSystem bool) ([]SqlServerTopQueryIntervalRow, error) {
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
		  ON class.instance_id = q.instance_id
		 AND class.query_hash = q.query_hash
		WHERE q.instance_id = $1
		  AND q.ts >= NOW() - ($3 * INTERVAL '1 hour')
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

	rows, err := tl.pool.Query(ctx, q, instance, limit, hours)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerTopQueriesFromInterval: %w", err)
	}
	defer rows.Close()

	var results []SqlServerTopQueryIntervalRow
	for rows.Next() {
		var r SqlServerTopQueryIntervalRow
		var qh int64
		if err := rows.Scan(&qh, &r.QueryText, &r.DatabaseName,
			&r.LoginName, &r.ApplicationName,
			&r.Executions, &r.AvgCpuMs, &r.AvgDurationMs, &r.AvgReads, &r.TotalCpuMs, &r.LastExecutionTime); err != nil {
			log.Printf("[TSLogger] GetSqlServerTopQueriesFromInterval scan: %v", err)
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		results = append(results, r)
	}
	return results, rows.Err()
}

// SqlServerTopQueryIntervalRow is the storage-layer DTO for top queries from the interval table.
type SqlServerTopQueryIntervalRow struct {
	QueryHash          string
	QueryText          string
	DatabaseName       string
	LoginName          string
	ApplicationName    string
	Executions         int64
	AvgCpuMs           float64
	AvgDurationMs      float64
	AvgReads           float64
	TotalCpuMs         float64
	LastExecutionTime  *time.Time
}
