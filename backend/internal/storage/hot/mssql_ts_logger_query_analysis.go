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

// MssqlQueryRegressionRow is the storage-layer DTO for mssql_query_regressions.
type MssqlQueryRegressionRow struct {
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

// MssqlPlanInstabilityRow is the storage-layer DTO for mssql_plan_instability.
type MssqlPlanInstabilityRow struct {
	CaptureTime       time.Time
	InstanceName      string
	DatabaseName      string
	QueryHash         string
	QueryText         string
	PlanCount         int
	LastExecutionTime time.Time
}

// MssqlWatchedQueryRow is the storage-layer DTO for mssql_watched_queries.
type MssqlWatchedQueryRow struct {
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

// MssqlWatchedSnapshotRow is the storage-layer DTO for mssql_watched_query_snapshots.
type MssqlWatchedSnapshotRow struct {
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

// MssqlWatchedEventRow is the storage-layer DTO for mssql_watched_query_events.
type MssqlWatchedEventRow struct {
	ID        int
	WatchedID int
	EventTime time.Time
	EventType string
	Notes     string
}

// ────────────────────────────────────────────────
// Regression writes & reads
// ────────────────────────────────────────────────

// LogMssqlQueryRegressions batch-inserts regression detection results.
func (tl *TimescaleLogger) LogMssqlQueryRegressions(ctx context.Context, rows []MssqlQueryRegressionRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO mssql_query_regressions
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
			return fmt.Errorf("LogMssqlQueryRegressions batch exec: %w", err)
		}
	}
	return nil
}

// GetMssqlQueryRegressions returns recent regression rows for an instance.
func (tl *TimescaleLogger) GetMssqlQueryRegressions(ctx context.Context, instance string, limit int) ([]MssqlQueryRegressionRow, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT capture_time, server_instance_name, database_name, query_hash, query_text,
		regression_type, previous_avg, current_avg, percent_change, plan_changed
		FROM mssql_query_regressions
		WHERE server_instance_name = $1
		ORDER BY capture_time DESC
		LIMIT $2`

	rows, err := tl.pool.Query(ctx, q, instance, limit)
	if err != nil {
		return nil, fmt.Errorf("GetMssqlQueryRegressions: %w", err)
	}
	defer rows.Close()

	var results []MssqlQueryRegressionRow
	for rows.Next() {
		var r MssqlQueryRegressionRow
		if err := rows.Scan(&r.CaptureTime, &r.InstanceName, &r.DatabaseName, &r.QueryHash,
			&r.QueryText, &r.RegressionType, &r.PreviousAvg, &r.CurrentAvg, &r.PercentChange, &r.PlanChanged); err != nil {
			log.Printf("[TSLogger] GetMssqlQueryRegressions scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// CountMssqlRegressionsInWindow returns the number of regressions for an instance in a time window.
func (tl *TimescaleLogger) CountMssqlRegressionsInWindow(ctx context.Context, instance string, since time.Time) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM mssql_query_regressions WHERE server_instance_name = $1 AND capture_time >= $2`,
		instance, since).Scan(&count)
	return count, err
}

// ────────────────────────────────────────────────
// Plan instability writes & reads
// ────────────────────────────────────────────────

// LogMssqlPlanInstability batch-inserts plan instability detection results.
func (tl *TimescaleLogger) LogMssqlPlanInstability(ctx context.Context, rows []MssqlPlanInstabilityRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO mssql_plan_instability
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
			return fmt.Errorf("LogMssqlPlanInstability batch exec: %w", err)
		}
	}
	return nil
}

// GetMssqlPlanInstability returns recent plan instability rows for an instance.
func (tl *TimescaleLogger) GetMssqlPlanInstability(ctx context.Context, instance string, limit int) ([]MssqlPlanInstabilityRow, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT capture_time, server_instance_name, database_name, query_hash, query_text, plan_count, last_execution_time
		FROM mssql_plan_instability
		WHERE server_instance_name = $1
		ORDER BY capture_time DESC
		LIMIT $2`

	rows, err := tl.pool.Query(ctx, q, instance, limit)
	if err != nil {
		return nil, fmt.Errorf("GetMssqlPlanInstability: %w", err)
	}
	defer rows.Close()

	var results []MssqlPlanInstabilityRow
	for rows.Next() {
		var r MssqlPlanInstabilityRow
		if err := rows.Scan(&r.CaptureTime, &r.InstanceName, &r.DatabaseName, &r.QueryHash,
			&r.QueryText, &r.PlanCount, &r.LastExecutionTime); err != nil {
			log.Printf("[TSLogger] GetMssqlPlanInstability scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// CountMssqlPlanInstabilityInWindow returns the number of unstable queries for an instance in a time window.
func (tl *TimescaleLogger) CountMssqlPlanInstabilityInWindow(ctx context.Context, instance string, since time.Time) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT query_hash) FROM mssql_plan_instability WHERE server_instance_name = $1 AND capture_time >= $2`,
		instance, since).Scan(&count)
	return count, err
}

// ────────────────────────────────────────────────
// Watched queries CRUD
// ────────────────────────────────────────────────

// InsertMssqlWatchedQuery adds a query to the watch list and returns the new ID.
func (tl *TimescaleLogger) InsertMssqlWatchedQuery(ctx context.Context, row MssqlWatchedQueryRow) (int, error) {
	var id int
	err := tl.pool.QueryRow(ctx,
		`INSERT INTO mssql_watched_queries (server_instance_name, database_name, query_hash, object_id, name, query_text)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		row.InstanceName, row.DatabaseName, row.QueryHash, row.ObjectID, row.Name, row.QueryText,
	).Scan(&id)

	if err != nil && strings.Contains(err.Error(), "database_name") {
		// Fallback for missing database_name column (migration not yet applied)
		return tl.insertMssqlWatchedQueryLegacy(ctx, row)
	}

	if err != nil {
		return 0, fmt.Errorf("InsertMssqlWatchedQuery: %w", err)
	}
	return id, nil
}

func (tl *TimescaleLogger) insertMssqlWatchedQueryLegacy(ctx context.Context, row MssqlWatchedQueryRow) (int, error) {
	var id int
	err := tl.pool.QueryRow(ctx,
		`INSERT INTO mssql_watched_queries (server_instance_name, query_hash, object_id, name, query_text)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		row.InstanceName, row.QueryHash, row.ObjectID, row.Name, row.QueryText,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("InsertMssqlWatchedQueryLegacy: %w", err)
	}
	return id, nil
}

// UpdateMssqlWatchedQueryText updates the full SQL text for a watched query.
func (tl *TimescaleLogger) UpdateMssqlWatchedQueryText(ctx context.Context, id int, text string) error {
	_, err := tl.pool.Exec(ctx, `UPDATE mssql_watched_queries SET query_text = $1 WHERE id = $2`, text, id)
	return err
}

// DeleteMssqlWatchedQuery removes a watched query by ID (cascades to snapshots + events).
func (tl *TimescaleLogger) DeleteMssqlWatchedQuery(ctx context.Context, id int) error {
	tag, err := tl.pool.Exec(ctx, `DELETE FROM mssql_watched_queries WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("DeleteMssqlWatchedQuery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("watched query %d not found", id)
	}
	return nil
}

func (tl *TimescaleLogger) ListMssqlWatchedQueries(ctx context.Context, instance string) ([]MssqlWatchedQueryRow, error) {
	// Primary attempt with database_name
	const q = `
		SELECT q.id, q.server_instance_name, COALESCE(q.database_name,'') as database_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(q.query_text,'') as query_text, q.created_at,
		       (SELECT MAX(last_execution_time) FROM mssql_watched_query_snapshots s WHERE s.watched_id = q.id) as last_executed
		FROM mssql_watched_queries q
		WHERE q.server_instance_name = $1
		ORDER BY q.created_at DESC`

	rows, err := tl.pool.Query(ctx, q, instance)
	if err != nil {
		if strings.Contains(err.Error(), "database_name") {
			return tl.listMssqlWatchedQueriesLegacy(ctx, instance)
		}
		return nil, fmt.Errorf("ListMssqlWatchedQueries: %w", err)
	}
	defer rows.Close()

	var results []MssqlWatchedQueryRow
	for rows.Next() {
		var r MssqlWatchedQueryRow
		if err := rows.Scan(&r.ID, &r.InstanceName, &r.DatabaseName, &r.QueryHash, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt); err != nil {
			log.Printf("[TSLogger] ListMssqlWatchedQueries scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) listMssqlWatchedQueriesLegacy(ctx context.Context, instance string) ([]MssqlWatchedQueryRow, error) {
	const q = `
		SELECT q.id, q.server_instance_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(q.query_text,'') as query_text, q.created_at,
		       (SELECT MAX(last_execution_time) FROM mssql_watched_query_snapshots s WHERE s.watched_id = q.id) as last_executed
		FROM mssql_watched_queries q
		WHERE q.server_instance_name = $1
		ORDER BY q.created_at DESC`

	rows, err := tl.pool.Query(ctx, q, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MssqlWatchedQueryRow
	for rows.Next() {
		var r MssqlWatchedQueryRow
		if err := rows.Scan(&r.ID, &r.InstanceName, &r.QueryHash, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt); err != nil {
			continue
		}
		r.DatabaseName = "" // Field empty in legacy mode
		results = append(results, r)
	}
	return results, nil
}

// GetMssqlWatchedQuery returns a single watched query by ID.
func (tl *TimescaleLogger) GetMssqlWatchedQuery(ctx context.Context, id int) (*MssqlWatchedQueryRow, error) {
	var r MssqlWatchedQueryRow
	const q = `
		SELECT q.id, q.server_instance_name, COALESCE(q.database_name,'') as database_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(NULLIF(q.query_text,''), s.query_text, '') as query_text,
		       q.created_at,
		       (SELECT MAX(last_execution_time) FROM mssql_watched_query_snapshots snap WHERE snap.watched_id = q.id) as last_executed
		FROM mssql_watched_queries q
		LEFT JOIN (
			SELECT DISTINCT ON (query_hash) query_hash, query_text 
			FROM sqlserver_query_stats_interval
		) s ON s.query_hash = q.query_hash
		WHERE q.id = $1`

	err := tl.pool.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.InstanceName, &r.DatabaseName, &r.QueryHash, &r.ObjectID, &r.Name, &r.QueryText, &r.CreatedAt, &r.LastExecutedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "database_name") {
			return tl.getMssqlWatchedQueryLegacy(ctx, id)
		}
		return nil, fmt.Errorf("GetMssqlWatchedQuery(%d): %w", id, err)
	}
	return &r, nil
}

func (tl *TimescaleLogger) getMssqlWatchedQueryLegacy(ctx context.Context, id int) (*MssqlWatchedQueryRow, error) {
	var r MssqlWatchedQueryRow
	const q = `
		SELECT q.id, q.server_instance_name, q.query_hash, q.object_id, q.name, 
		       COALESCE(NULLIF(q.query_text,''), s.query_text, '') as query_text,
		       q.created_at,
		       (SELECT MAX(last_execution_time) FROM mssql_watched_query_snapshots snap WHERE snap.watched_id = q.id) as last_executed
		FROM mssql_watched_queries q
		LEFT JOIN (
			SELECT DISTINCT ON (query_hash) query_hash, query_text 
			FROM sqlserver_query_stats_interval
		) s ON s.query_hash = q.query_hash
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

// CountMssqlWatchedQueries returns the number of watched queries for an instance.
func (tl *TimescaleLogger) CountMssqlWatchedQueries(ctx context.Context, instance string) (int, error) {
	var count int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM mssql_watched_queries WHERE server_instance_name = $1`, instance,
	).Scan(&count)
	return count, err
}

// ────────────────────────────────────────────────
// Watched query snapshots
// ────────────────────────────────────────────────
// LogMssqlWatchedQuerySnapshot batch-inserts snapshot rows for watched queries.
func (tl *TimescaleLogger) LogMssqlWatchedQuerySnapshot(ctx context.Context, rows []MssqlWatchedSnapshotRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO mssql_watched_query_snapshots
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

// GetMssqlWatchedQuerySnapshots returns time-series snapshots for a watched query in a time range.
func (tl *TimescaleLogger) GetMssqlWatchedQuerySnapshots(ctx context.Context, watchedID int, from, to time.Time) ([]MssqlWatchedSnapshotRow, error) {
	const q = `SELECT snapshot_time, watched_id, server_instance_name, executions,
		avg_duration_ms, avg_cpu_ms, avg_reads, total_duration_ms, total_cpu_ms,
		plan_count, last_execution_time, COALESCE(query_plan, ''), wait_stats
		FROM mssql_watched_query_snapshots
		WHERE watched_id = $1 AND snapshot_time >= $2 AND snapshot_time <= $3
		ORDER BY snapshot_time ASC`

	rows, err := tl.pool.Query(ctx, q, watchedID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetMssqlWatchedQuerySnapshots: %w", err)
	}
	defer rows.Close()

	var results []MssqlWatchedSnapshotRow
	for rows.Next() {
		var r MssqlWatchedSnapshotRow
		if err := rows.Scan(&r.SnapshotTime, &r.WatchedID, &r.InstanceName, &r.Executions,
			&r.AvgDurationMs, &r.AvgCpuMs, &r.AvgReads, &r.TotalDurationMs,
			&r.TotalCpuMs, &r.PlanCount, &r.LastExecutionTime, &r.QueryPlan, &r.WaitStats); err != nil {
			log.Printf("[TSLogger] GetMssqlWatchedQuerySnapshots scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ────────────────────────────────────────────────
// Watched query events
// ────────────────────────────────────────────────

// InsertMssqlWatchedQueryEvent records an optimization event marker.
func (tl *TimescaleLogger) InsertMssqlWatchedQueryEvent(ctx context.Context, row MssqlWatchedEventRow) error {
	_, err := tl.pool.Exec(ctx,
		`INSERT INTO mssql_watched_query_events (watched_id, event_time, event_type, notes)
		 VALUES ($1, $2, $3, $4)`,
		row.WatchedID, row.EventTime, row.EventType, row.Notes,
	)
	if err != nil {
		return fmt.Errorf("InsertMssqlWatchedQueryEvent: %w", err)
	}
	return nil
}

// GetMssqlWatchedQueryEvents returns all events for a watched query.
func (tl *TimescaleLogger) GetMssqlWatchedQueryEvents(ctx context.Context, watchedID int) ([]MssqlWatchedEventRow, error) {
	const q = `SELECT id, watched_id, event_time, event_type, COALESCE(notes,'')
		FROM mssql_watched_query_events
		WHERE watched_id = $1
		ORDER BY event_time DESC`

	rows, err := tl.pool.Query(ctx, q, watchedID)
	if err != nil {
		return nil, fmt.Errorf("GetMssqlWatchedQueryEvents: %w", err)
	}
	defer rows.Close()

	var results []MssqlWatchedEventRow
	for rows.Next() {
		var r MssqlWatchedEventRow
		if err := rows.Scan(&r.ID, &r.WatchedID, &r.EventTime, &r.EventType, &r.Notes); err != nil {
			log.Printf("[TSLogger] GetMssqlWatchedQueryEvents scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ────────────────────────────────────────────────
// Query Analysis Summary (aggregated from existing tables)
// ────────────────────────────────────────────────

// GetMssqlQueryAnalysisSummary aggregates KPI data from sqlserver_query_stats_interval + regression/instability counts.
func (tl *TimescaleLogger) GetMssqlQueryAnalysisSummary(ctx context.Context, instance string, hours int) (*MssqlQueryAnalysisSummaryRow, error) {
	var s MssqlQueryAnalysisSummaryRow
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
		log.Printf("[TSLogger] GetMssqlQueryAnalysisSummary interval query: %v", err)
		// Non-fatal — continue with zero values
	}

	// Regression count (last 24h)
	since24h := time.Now().UTC().Add(-24 * time.Hour)
	s.Regressions24h, _ = tl.CountMssqlRegressionsInWindow(ctx, instance, since24h)

	// Plan instability count (last 24h)
	s.PlanChanges24h, _ = tl.CountMssqlPlanInstabilityInWindow(ctx, instance, since24h)

	return &s, nil
}

// MssqlQueryAnalysisSummaryRow is the storage-layer DTO for the summary endpoint.
type MssqlQueryAnalysisSummaryRow struct {
	TotalExecutions int64
	AvgDuration     float64
	AvgCPU          float64
	AvgReads        float64
	Regressions24h  int
	PlanChanges24h  int
}

// GetMssqlTopQueriesFromInterval returns top queries from the existing sqlserver_query_stats_interval table.
func (tl *TimescaleLogger) GetMssqlTopQueriesFromInterval(ctx context.Context, instance, sortBy string, limit, hours int) ([]MssqlTopQueryIntervalRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if hours <= 0 {
		hours = 24
	}

	orderClause := "SUM(delta_cpu_ms) DESC"
	switch sortBy {
	case "duration":
		orderClause = "AVG(avg_duration_ms) DESC"
	case "reads":
		orderClause = "AVG(delta_logical_reads) DESC"
	case "executions":
		orderClause = "SUM(delta_executions) DESC"
	case "cpu":
		orderClause = "SUM(delta_cpu_ms) DESC"
	}

	q := fmt.Sprintf(`
		SELECT query_hash, MAX(query_text), database_name,
		       SUM(delta_executions), AVG(avg_cpu_ms), AVG(avg_duration_ms),
		       AVG(delta_logical_reads), SUM(delta_cpu_ms)
		FROM monitor.mssql_query_store_interval
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND bucket_end >= NOW() - ($3 * INTERVAL '1 hour')
		GROUP BY query_hash, database_name
		ORDER BY %s
		LIMIT $2`, orderClause)

	rows, err := tl.pool.Query(ctx, q, instance, limit, hours)
	if err != nil {
		return nil, fmt.Errorf("GetMssqlTopQueriesFromInterval: %w", err)
	}
	defer rows.Close()

	var results []MssqlTopQueryIntervalRow
	for rows.Next() {
		var r MssqlTopQueryIntervalRow
		if err := rows.Scan(&r.QueryHash, &r.QueryText, &r.DatabaseName,
			&r.Executions, &r.AvgCpuMs, &r.AvgDurationMs, &r.AvgReads, &r.TotalCpuMs); err != nil {
			log.Printf("[TSLogger] GetMssqlTopQueriesFromInterval scan: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// MssqlTopQueryIntervalRow is the storage-layer DTO for top queries from the interval table.
type MssqlTopQueryIntervalRow struct {
	QueryHash     string
	QueryText     string
	DatabaseName  string
	Executions    int64
	AvgCpuMs      float64
	AvgDurationMs float64
	AvgReads      float64
	TotalCpuMs    float64
}
