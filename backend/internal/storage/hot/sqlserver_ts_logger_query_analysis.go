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
		batch.Queue(q, r.CaptureTime, r.InstanceName, r.DatabaseName, r.QueryHash,
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
		WHERE server_instance_name = $1
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
		if err := rows.Scan(&r.CaptureTime, &r.InstanceName, &r.DatabaseName, &r.QueryHash,
			&r.QueryText, &r.RegressionType, &r.PreviousAvg, &r.CurrentAvg, &r.PercentChange, &r.PlanChanged); err != nil {
			log.Printf("[TSLogger] GetSqlServerQueryRegressions scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// CountSqlServerRegressionsInWindow returns the number of regressions for an instance in a time window.
func (tl *TimescaleLogger) CountSqlServerRegressionsInWindow(ctx context.Context, instance string, since time.Time) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sqlserver_query_regressions WHERE server_instance_name = $1 AND capture_time >= $2`,
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
		batch.Queue(q, r.CaptureTime, r.InstanceName, r.DatabaseName, r.QueryHash,
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
		FROM sqlserver_plan_instability
		WHERE server_instance_name = $1
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
		if err := rows.Scan(&r.CaptureTime, &r.InstanceName, &r.DatabaseName, &r.QueryHash,
			&r.QueryText, &r.PlanCount, &r.LastExecutionTime); err != nil {
			log.Printf("[TSLogger] GetSqlServerPlanInstability scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// CountSqlServerPlanInstabilityInWindow returns the number of unstable queries for an instance in a time window.
func (tl *TimescaleLogger) CountSqlServerPlanInstabilityInWindow(ctx context.Context, instance string, since time.Time) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT query_hash) FROM sqlserver_plan_instability WHERE server_instance_name = $1 AND capture_time >= $2`,
		instance, since).Scan(&count)
	return count, err
}

// ────────────────────────────────────────────────
// Watched queries CRUD
// ────────────────────────────────────────────────

// InsertSqlServerWatchedQuery adds a query to the watch list and returns the new ID.
func (tl *TimescaleLogger) InsertSqlServerWatchedQuery(ctx context.Context, row SqlServerWatchedQueryRow) (int, error) {
	var id int
	err := tl.pool.QueryRow(ctx,
		`INSERT INTO sqlserver_watched_queries (server_instance_name, database_name, query_hash, object_id, name, query_text)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		row.InstanceName, row.DatabaseName, row.QueryHash, row.ObjectID, row.Name, row.QueryText,
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
	err := tl.pool.QueryRow(ctx,
		`INSERT INTO sqlserver_watched_queries (server_instance_name, query_hash, object_id, name, query_text)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		row.InstanceName, row.QueryHash, row.ObjectID, row.Name, row.QueryText,
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
		WHERE q.server_instance_name = $1
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
		WHERE q.server_instance_name = $1
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

	err := tl.pool.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.InstanceName, &r.DatabaseName, &r.QueryHash, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "database_name") {
			return tl.getSqlServerWatchedQueryLegacy(ctx, id)
		}
		return nil, fmt.Errorf("GetSqlServerWatchedQuery(%d): %w", id, err)
	}
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

	err := tl.pool.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.InstanceName, &r.QueryHash, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt,
	)
	if err != nil {
		return nil, err
	}
	r.DatabaseName = ""
	return &r, nil
}

// CountSqlServerWatchedQueries returns the number of watched queries for an instance.
func (tl *TimescaleLogger) CountSqlServerWatchedQueries(ctx context.Context, instance string) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sqlserver_watched_queries WHERE server_instance_name = $1`, instance,
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

// GetSqlServerQueryAnalysisSummary aggregates KPI data from sqlserver_query_stats_interval + regression/instability counts.
func (tl *TimescaleLogger) GetSqlServerQueryAnalysisSummary(ctx context.Context, instance string, hours int) (*SqlServerQueryAnalysisSummaryRow, error) {
	var s SqlServerQueryAnalysisSummaryRow
	if hours <= 0 {
		hours = 24
	}

	// Aggregate from existing sqlserver_query_stats_interval over the requested time window.
	err := tl.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(executions), 0),
		       COALESCE(AVG(avg_duration_ms), 0),
		       COALESCE(AVG(avg_cpu_ms), 0),
		       COALESCE(AVG(avg_reads), 0)
		FROM sqlserver_query_stats_interval
		WHERE server_instance_name = $1
		  AND bucket_end >= NOW() - ($2 * INTERVAL '1 hour')`,
		instance, hours,
	).Scan(&s.TotalExecutions, &s.AvgDuration, &s.AvgCPU, &s.AvgReads)
	if err != nil {
		log.Printf("[TSLogger] GetSqlServerQueryAnalysisSummary interval query: %v", err)
		// Non-fatal — continue with zero values
	}

	// Regression count (last 24h)
	since24h := time.Now().UTC().Add(-24 * time.Hour)
	s.Regressions24h, _ = tl.CountSqlServerRegressionsInWindow(ctx, instance, since24h)

	// Plan instability count (last 24h)
	s.PlanChanges24h, _ = tl.CountSqlServerPlanInstabilityInWindow(ctx, instance, since24h)

	return &s, nil
}

// SqlServerQueryAnalysisSummaryRow is the storage-layer DTO for the summary endpoint.
type SqlServerQueryAnalysisSummaryRow struct {
	TotalExecutions int64
	AvgDuration     float64
	AvgCPU          float64
	AvgReads        float64
	Regressions24h  int
	PlanChanges24h  int
}

// GetSqlServerTopQueriesFromInterval returns top queries from the existing sqlserver_query_stats_interval table.
func (tl *TimescaleLogger) GetSqlServerTopQueriesFromInterval(ctx context.Context, instance, sortBy string, limit, hours int) ([]SqlServerTopQueryIntervalRow, error) {
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

	q := fmt.Sprintf(`
		SELECT query_hash, MAX(statement_text), database_name,
		       MAX(login_name), MAX(application_name),
		       SUM(total_executions), AVG(total_cpu_ms / NULLIF(total_executions, 0)), AVG(total_elapsed_ms / NULLIF(total_executions, 0)),
		       AVG(total_logical_reads / NULLIF(total_executions, 0)), SUM(total_cpu_ms)
		FROM sqlserver_query_metrics_v2
		WHERE UPPER(instance_id) = UPPER($1)
		  AND ts >= NOW() - ($3 * INTERVAL '1 hour')
		GROUP BY query_hash, database_name
		ORDER BY %s
		LIMIT $2`, orderClause)

	rows, err := tl.pool.Query(ctx, q, instance, limit, hours)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerTopQueriesFromInterval: %w", err)
	}
	defer rows.Close()

	var results []SqlServerTopQueryIntervalRow
	for rows.Next() {
		var r SqlServerTopQueryIntervalRow
		if err := rows.Scan(&r.QueryHash, &r.QueryText, &r.DatabaseName,
			&r.LoginName, &r.ApplicationName,
			&r.Executions, &r.AvgCpuMs, &r.AvgDurationMs, &r.AvgReads, &r.TotalCpuMs); err != nil {
			log.Printf("[TSLogger] GetSqlServerTopQueriesFromInterval scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SqlServerTopQueryIntervalRow is the storage-layer DTO for top queries from the interval table.
type SqlServerTopQueryIntervalRow struct {
	QueryHash       string
	QueryText       string
	DatabaseName    string
	LoginName       string
	ApplicationName string
	Executions      int64
	AvgCpuMs        float64
	AvgDurationMs   float64
	AvgReads        float64
	TotalCpuMs      float64
}
