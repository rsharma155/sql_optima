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
	"github.com/rsharma155/sql_optima/internal/domain"
)

func queryAnalysisUsesDatabase(database string) bool {
	db := strings.TrimSpace(database)
	return db != "" && !strings.EqualFold(db, "all")
}

// queryAnalysisHistoryDatabaseExists filters history rows to hashes seen in metrics_v2 for the database.
func queryAnalysisHistoryDatabaseExists(argPos int) string {
	return fmt.Sprintf(` AND EXISTS (
		SELECT 1 FROM sqlserver_query_metrics_v2 qm
		WHERE qm.server_id = qh.server_id AND qm.query_hash = qh.query_hash
		  AND LOWER(TRIM(COALESCE(qm.database_name, ''))) = LOWER(TRIM($%d))
		LIMIT 1
	)`, argPos)
}

func (tl *TimescaleLogger) fillQueryAnalysisDiagnostics(ctx context.Context, serverID uuid.UUID, from, to time.Time, database string, s *SqlServerQueryAnalysisSummaryRow) {
	var latestHist, latestMet *time.Time
	_ = tl.pool.QueryRow(ctx, `
		SELECT MAX(capture_timestamp) FROM sqlserver_query_stats_history WHERE server_id = $1`, serverID).Scan(&latestHist)
	_ = tl.pool.QueryRow(ctx, `
		SELECT MAX(capture_timestamp) FROM sqlserver_query_metrics_v2 WHERE server_id = $1`, serverID).Scan(&latestMet)
	if latestHist != nil {
		s.LatestHistoryCapture = latestHist
	}

	countQ := `SELECT COUNT(*) FROM sqlserver_query_stats_history WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3`
	countArgs := []interface{}{serverID, from, to}
	if queryAnalysisUsesDatabase(database) {
		countQ += ` AND EXISTS (
			SELECT 1 FROM sqlserver_query_metrics_v2 qm
			WHERE qm.server_id = $1 AND qm.query_hash = sqlserver_query_stats_history.query_hash
			  AND LOWER(TRIM(COALESCE(qm.database_name, ''))) = LOWER(TRIM($4))
			LIMIT 1
		)`
		countArgs = append(countArgs, database)
	}
	_ = tl.pool.QueryRow(ctx, countQ, countArgs...).Scan(&s.HistoryRowsInRange)

	metQ := `SELECT COUNT(*) FROM sqlserver_query_metrics_v2 WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3` +
		sqlServerQueryAnalysisDistributionExcludeSQL("")
	metArgs := []interface{}{serverID, from, to}
	if queryAnalysisUsesDatabase(database) {
		metQ += workloadDatabaseClause(4)
		metArgs = append(metArgs, database)
	}
	_ = tl.pool.QueryRow(ctx, metQ, metArgs...).Scan(&s.MetricsRowsInRange)

	if s.Message != "" || (s.TotalExecutions > 0 && s.QueriesExecutedInRange > 0) {
		return
	}
	if latestHist == nil && latestMet == nil {
		s.Message = "No query metrics in TimescaleDB yet. Ensure the SQL Server collector is running and the instance is registered."
		return
	}
	if latestHist != nil && latestHist.Before(from) {
		s.Message = fmt.Sprintf("Latest collector capture is %s, before your selected range. Try a more recent time window.", latestHist.UTC().Format(time.RFC3339))
		return
	}
	if s.HistoryRowsInRange == 0 && s.MetricsRowsInRange == 0 {
		s.Message = "No query activity in the selected time range. Widen the range, pick All databases, or set exclude_system=false."
	}
}

// Row types (storage-layer DTOs)

type SqlServerQueryRegressionRow struct {
	CaptureTime     time.Time `json:"capture_time"`
	ServerID        uuid.UUID `json:"server_id"`
	DatabaseName    string    `json:"database_name"`
	QueryHash       string    `json:"query_hash"`
	QueryText       string    `json:"query_text"`
	RegressionType  string    `json:"regression_type"`
	PreviousAvg     float64   `json:"previous_avg"`
	CurrentAvg      float64   `json:"current_avg"`
	PercentChange   float64   `json:"percent_change"`
	PlanChanged     bool      `json:"plan_changed"`
	LoginName       string    `json:"login_name"`
	ApplicationName string    `json:"application_name"`
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

func (tl *TimescaleLogger) GetSqlServerQueryRegressions(ctx context.Context, serverID uuid.UUID, limit int, monitoringLogins []string) ([]SqlServerQueryRegressionRow, error) {
	if limit <= 0 {
		limit = 50
	}
	// Deduplicate by query_hash (keep latest capture), join latest login/app from query_metrics_v2,
	// and filter out monitoring tool queries.
	q := `
		WITH deduped AS (
			SELECT DISTINCT ON (r.query_hash)
				r.capture_timestamp, r.server_id, r.database_name, r.query_hash, r.query_text,
				r.regression_type, r.previous_avg, r.current_avg, r.percent_change, r.plan_changed,
				COALESCE(qm.login_name, '')       AS login_name,
				COALESCE(qm.application_name, '') AS application_name
			FROM sqlserver_query_regressions r
			LEFT JOIN LATERAL (
				SELECT login_name, application_name
				FROM sqlserver_query_metrics_v2
				WHERE server_id = r.server_id AND query_hash = r.query_hash
				ORDER BY capture_timestamp DESC
				LIMIT 1
			) qm ON true
			WHERE r.server_id = $1
			ORDER BY r.query_hash, r.capture_timestamp DESC
		)
		SELECT capture_timestamp, server_id, database_name, query_hash, query_text,
		       regression_type, previous_avg, current_avg, percent_change, plan_changed,
		       login_name, application_name
		FROM deduped
		WHERE 1=1` + sqlServerOptimaTaggedSQLExcludeSQL("", "query_text") + sqlServerMonitoringAppExcludeSQL("") + sqlServerMonitoringLoginExcludeSQL("", monitoringLogins) + sqlServerQueryAnalysisDistributionExcludeSQL("") + `
		ORDER BY percent_change DESC
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
			&r.QueryText, &r.RegressionType, &r.PreviousAvg, &r.CurrentAvg, &r.PercentChange, &r.PlanChanged,
			&r.LoginName, &r.ApplicationName); err != nil {
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
	q := `SELECT capture_timestamp, server_id, database_name, query_hash, query_text, plan_count, last_execution_time
		FROM (
			SELECT DISTINCT ON (query_hash) 
				capture_timestamp, server_id, database_name, query_hash, query_text, plan_count, last_execution_time
			FROM sqlserver_plan_instability
			WHERE server_id = $1` + sqlServerOptimaTaggedSQLExcludeSQL("", "query_text") + sqlServerQueryAnalysisDistributionExcludeSQL("") + `
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

func (tl *TimescaleLogger) GetSqlServerPrimaryAnalysisDatabase(ctx context.Context, serverID uuid.UUID, from, to time.Time) (string, error) {
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() || !from.Before(to) {
		from = to.Add(-24 * time.Hour)
	}
	var name string
	err := tl.pool.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(TRIM(database_name), ''), '')
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2 AND capture_timestamp <= $3
		  AND TRIM(COALESCE(database_name, '')) <> ''
		GROUP BY database_name
		ORDER BY SUM(total_cpu_ms) DESC NULLS LAST
		LIMIT 1`, serverID, from, to).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil
}

func (tl *TimescaleLogger) GetSqlServerQueryAnalysisSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time, database string, excludeSystem bool, monitoringLogins []string) (*SqlServerQueryAnalysisSummaryRow, error) {
	var s SqlServerQueryAnalysisSummaryRow
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() || !from.Before(to) {
		from = to.Add(-24 * time.Hour)
	}

	histMetricsJoin, metricsFilter := sqlServerQueryAnalysisMetricsLateralJoin(excludeSystem, "qh", monitoringLogins)
	snapMetricsJoin, snapMetricsFilter := sqlServerQueryAnalysisMetricsLateralJoin(excludeSystem, "s", monitoringLogins)
	histClassFilter := sqlServerQueryAnalysisClassificationFilter(excludeSystem, "qh.")
	snapClassFilter := sqlServerQueryAnalysisClassificationFilter(excludeSystem, "s.")

	historyDB := ""
	histArgs := []interface{}{serverID, from, to}
	if queryAnalysisUsesDatabase(database) {
		historyDB = queryAnalysisHistoryDatabaseExists(4)
		histArgs = append(histArgs, database)
	}

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
		%s
		WHERE qh.server_id = $1
		  AND qh.capture_timestamp >= $2 AND qh.capture_timestamp <= $3
		  %s%s%s`, histMetricsJoin, historyDB, metricsFilter, histClassFilter)

	err := tl.pool.QueryRow(ctx, q1, histArgs...).Scan(&s.TotalExecutions, &s.AvgCPU, &s.AvgDuration, &s.AvgReads, &s.QueriesExecutedInRange)
	if err != nil {
		slog.Info("[TSLogger] GetSqlServerQueryAnalysisSummary history", "err", err)
	}

	top10MetricsLateral := `
			LEFT JOIN sqlserver_query_classification_dim class
			  ON class.server_id = qh.server_id AND class.query_hash = qh.query_hash
			` + histMetricsJoin
	top10Q := fmt.Sprintf(`
		WITH total AS (
			SELECT SUM(qh.cpu_delta_ms) as total_cpu
			FROM sqlserver_query_stats_history qh
			%s
			WHERE qh.server_id = $1 AND qh.capture_timestamp >= $2 AND qh.capture_timestamp <= $3
			  %s%s%s
		),
		top10 AS (
			SELECT SUM(sub.query_cpu) as top_cpu
			FROM (
				SELECT SUM(qh.cpu_delta_ms) as query_cpu
				FROM sqlserver_query_stats_history qh
				%s
				WHERE qh.server_id = $1 AND qh.capture_timestamp >= $2 AND qh.capture_timestamp <= $3
				  %s%s%s
				GROUP BY qh.query_hash
				ORDER BY query_cpu DESC
				LIMIT 10
			) sub
		)
		SELECT COALESCE((top_cpu::float8 / NULLIF(total_cpu, 0)) * 100, 0)
		FROM total, top10`, top10MetricsLateral, historyDB, metricsFilter, histClassFilter,
		top10MetricsLateral, historyDB, metricsFilter, histClassFilter)
	if err := tl.pool.QueryRow(ctx, top10Q, histArgs...).Scan(&s.Top10CpuSharePct); err != nil {
		slog.Info("[TSLogger] GetSqlServerQueryAnalysisSummary top10 cpu share", "err", err)
	}

	snapDB := ""
	snapArgs := []interface{}{serverID}
	if queryAnalysisUsesDatabase(database) {
		snapDB = ` AND LOWER(TRIM(COALESCE(s.database_name, ''))) = LOWER(TRIM($2))`
		snapArgs = append(snapArgs, database)
	}
	q3 := fmt.Sprintf(`
		SELECT COUNT(DISTINCT s.query_hash)
		FROM sqlserver_query_stats_snapshot_v2 s
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.server_id = s.server_id AND class.query_hash = s.query_hash
		%s
		WHERE s.server_id = $1
		  %s%s%s`, snapMetricsJoin, snapDB, snapMetricsFilter, snapClassFilter)
	_ = tl.pool.QueryRow(ctx, q3, snapArgs...).Scan(&s.TotalQueriesInQS)

	singleQ := fmt.Sprintf(`
		SELECT COUNT(*) FROM (
			SELECT qh.query_hash
			FROM sqlserver_query_stats_history qh
			LEFT JOIN sqlserver_query_classification_dim class
			  ON class.server_id = qh.server_id AND class.query_hash = qh.query_hash
			%s
			WHERE qh.server_id = $1 AND qh.capture_timestamp >= $2 AND qh.capture_timestamp <= $3
			  %s%s%s
			GROUP BY qh.query_hash
			HAVING SUM(qh.exec_delta) = 1
		) sub`, histMetricsJoin, historyDB, metricsFilter, histClassFilter)
	_ = tl.pool.QueryRow(ctx, singleQ, histArgs...).Scan(&s.QueriesSingleExecution)

	since24h := time.Now().UTC().Add(-24 * time.Hour)
	s.Regressions24h, _ = tl.CountSqlServerRegressionsInWindow(ctx, serverID, since24h)
	s.PlanChanges24h, _ = tl.CountSqlServerPlanInstabilityInWindow(ctx, serverID, since24h)
	s.QueriesWithMultiPlans = int64(s.PlanChanges24h)

	tl.fillQueryAnalysisDiagnostics(ctx, serverID, from, to, database, &s)
	return &s, nil
}

type SqlServerQueryAnalysisSummaryRow struct {
	TotalExecutions        int64      `json:"total_executions"`
	AvgDuration            float64    `json:"avg_duration_ms"`
	AvgCPU                 float64    `json:"avg_cpu_ms"`
	AvgReads               float64    `json:"avg_reads"`
	Regressions24h         int        `json:"regressions_24h"`
	PlanChanges24h         int        `json:"plan_changes_24h"`
	Top10CpuSharePct       float64    `json:"top_10_cpu_share_pct"`
	TotalQueriesInQS       int64      `json:"total_queries_in_qs"`
	QueriesExecutedInRange int64      `json:"queries_executed_in_range"`
	QueriesWithMultiPlans  int64      `json:"queries_with_multi_plans"`
	QueriesSingleExecution int64      `json:"queries_single_execution"`
	HistoryRowsInRange     int64      `json:"history_rows_in_range"`
	MetricsRowsInRange     int64      `json:"metrics_rows_in_range"`
	LatestHistoryCapture   *time.Time `json:"latest_history_capture,omitempty"`
	Message                string     `json:"message,omitempty"`
}

func (tl *TimescaleLogger) GetSqlServerTopQueriesFromInterval(ctx context.Context, serverID uuid.UUID, sortBy string, limit int, from, to time.Time, database string, excludeSystem bool, monitoringLogins []string) ([]SqlServerTopQueryIntervalRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() || !from.Before(to) {
		from = to.Add(-24 * time.Hour)
	}

	orderClause := "SUM(q.total_cpu_ms) DESC"
	switch sortBy {
	case "duration":
		orderClause = `CASE WHEN SUM(q.total_executions) > 0
			THEN SUM(COALESCE(q.total_elapsed_ms, 0))::float8 / SUM(q.total_executions) ELSE 0 END DESC`
	case "reads":
		orderClause = `CASE WHEN SUM(q.total_executions) > 0
			THEN SUM(q.total_logical_reads)::float8 / SUM(q.total_executions) ELSE 0 END DESC`
	case "executions":
		orderClause = "SUM(q.total_executions) DESC"
	case "cpu":
		orderClause = "SUM(q.total_cpu_ms) DESC"
	}

	metricsFilter := sqlServerQueryAnalysisScopeSQL(excludeSystem, "q.", monitoringLogins)
	classFilter := sqlServerQueryAnalysisClassificationFilter(excludeSystem, "q.")
	dbScoped := queryAnalysisUsesDatabase(database)

	dbClause := ""
	args := []interface{}{serverID, limit, from, to}
	if dbScoped {
		dbClause = workloadDatabaseClause(5)
		args = append(args, database)
	}

	q := fmt.Sprintf(`
		SELECT`+sqlServerQueryAnalysisFingerprintSelectSQL("q.")+`
		FROM sqlserver_query_metrics_v2 q
		WHERE q.server_id = $1
		  AND q.capture_timestamp >= $3 AND q.capture_timestamp <= $4
		  %s%s%s`+sqlServerQueryAnalysisGroupByFingerprintSQL("q.", dbScoped)+`
		HAVING SUM(q.total_executions) > 0 OR SUM(q.total_cpu_ms) > 0
		ORDER BY %s
		LIMIT $2`, dbClause, metricsFilter, classFilter, orderClause)

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerTopQueriesFromInterval: %w", err)
	}
	defer rows.Close()

	results := make([]SqlServerTopQueryIntervalRow, 0)
	for rows.Next() {
		var r SqlServerTopQueryIntervalRow
		var qh int64
		var planCount, hashVariants int64
		var totalElapsed, totalReads int64
		if err := rows.Scan(&r.StatementFingerprint, &qh, &hashVariants, &r.QueryText, &r.DatabaseName,
			&r.LoginName, &r.ApplicationName, &planCount,
			&r.Executions, &r.TotalCpuMs, &totalElapsed, &totalReads,
			&r.AvgCpuMs, &r.AvgDurationMs, &r.AvgReads, &r.LastExecutionTime); err != nil {
			slog.Info("[TSLogger] GetSqlServerTopQueriesFromInterval scan", "err", err)
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		r.PlanCount = int(planCount)
		r.HashVariantCount = int(hashVariants)
		results = append(results, r)
	}
	return results, rows.Err()
}

type SqlServerTopQueryIntervalRow struct {
	QueryHash            string     `json:"query_hash"`
	StatementFingerprint string     `json:"statement_fingerprint"`
	HashVariantCount     int        `json:"hash_variant_count"`
	QueryText            string     `json:"query_text"`
	DatabaseName         string     `json:"database_name"`
	LoginName            string     `json:"login_name"`
	ApplicationName      string     `json:"application_name"`
	PlanCount            int        `json:"plan_count"`
	Executions           int64      `json:"executions"`
	AvgCpuMs          float64    `json:"avg_cpu_ms"`
	AvgDurationMs     float64    `json:"avg_duration_ms"`
	AvgReads          float64    `json:"avg_reads"`
	TotalCpuMs        float64    `json:"total_cpu_ms"`
	LastExecutionTime *time.Time `json:"last_execution_time"`
}

func isQueryAnalysisExcludedDatabase(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "distribution")
}

func filterQueryAnalysisDatabases(dbs []domain.SqlServerWorkloadDatabaseActivity) []domain.SqlServerWorkloadDatabaseActivity {
	out := make([]domain.SqlServerWorkloadDatabaseActivity, 0, len(dbs))
	for _, d := range dbs {
		if isQueryAnalysisExcludedDatabase(d.DatabaseName) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// GetSqlServerPrimaryQueryAnalysisDatabase picks the busiest user database, excluding distribution.
func (tl *TimescaleLogger) GetSqlServerPrimaryQueryAnalysisDatabase(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (string, error) {
	dbs, err := tl.GetSqlServerDatabasesInRange(ctx, serverID, from, to, filter)
	if err != nil {
		return "", err
	}
	if db := pickWorkloadDatabaseName(filterQueryAnalysisDatabases(dbs)); db != "" {
		return db, nil
	}
	unfiltered := domain.WorkloadQueryFilter{
		ExcludeSystem:    false,
		MonitoringLogins: filter.MonitoringLogins,
	}
	raw, err := tl.GetSqlServerDatabasesInRange(ctx, serverID, from, to, unfiltered)
	if err != nil {
		return "", err
	}
	return pickWorkloadDatabaseName(filterQueryAnalysisDatabases(raw)), nil
}

type SqlServerTopQueryTrendPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	Executions int64     `json:"executions"`
	CpuMs      int64     `json:"cpu_ms"`
}

type SqlServerTopQueryTrendSeries struct {
	StatementFingerprint string                        `json:"statement_fingerprint"`
	QueryHash            string                        `json:"query_hash"`
	Label                string                        `json:"label"`
	TotalExecutions      int64                         `json:"total_executions"`
	TotalCpuMs           int64                         `json:"total_cpu_ms"`
	Points               []SqlServerTopQueryTrendPoint `json:"points"`
}

type SqlServerTopQueryTrendsResponse struct {
	Bucket string                       `json:"bucket"`
	Series []SqlServerTopQueryTrendSeries `json:"series"`
}

func queryAnalysisTrendBucket(from, to time.Time) string {
	dur := to.Sub(from)
	if dur > 48*time.Hour {
		return "15 minutes"
	}
	if dur > 12*time.Hour {
		return "5 minutes"
	}
	return "1 minute"
}

// GetSqlServerTopQueryTrends returns time-bucketed execution/CPU trends for the top statements (by fingerprint).
// When fingerprints is non-empty, only those statement groups are returned (same keys as the top-queries table).
func (tl *TimescaleLogger) GetSqlServerTopQueryTrends(ctx context.Context, serverID uuid.UUID, from, to time.Time, database string, excludeSystem bool, monitoringLogins []string, limit int, fingerprints []string) (*SqlServerTopQueryTrendsResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 15 {
		limit = 15
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() || !from.Before(to) {
		from = to.Add(-24 * time.Hour)
	}
	bucket := queryAnalysisTrendBucket(from, to)
	fpExpr := sqlServerQueryAnalysisFingerprintExpr("s.")
	textExpr := sqlServerQueryAnalysisStatementTextExpr("s.")
	scope := sqlServerQueryAnalysisScopeSQL(excludeSystem, "s.", monitoringLogins)
	classFilter := sqlServerQueryAnalysisClassificationFilter(excludeSystem, "s.")
	dbScoped := queryAnalysisUsesDatabase(database)

	dbClause := ""
	args := []interface{}{serverID, from, to}
	if dbScoped {
		dbClause = workloadDatabaseClause(4)
		args = append(args, database)
	}

	fpFilter := ""
	if len(fingerprints) > 0 {
		fpFilter = fmt.Sprintf(` AND %s = ANY($%d::text[])`, fpExpr, len(args)+1)
		args = append(args, fingerprints)
	}

	topSelect := `
			SELECT statement_fingerprint,
			       (array_agg(query_hash ORDER BY total_cpu_ms DESC NULLS LAST))[1] AS rep_hash,
			       (array_agg(stmt_text ORDER BY length(COALESCE(stmt_text, '')) DESC NULLS LAST))[1] AS label_text,
			       SUM(total_executions)::bigint AS total_executions,
			       SUM(total_cpu_ms)::bigint AS total_cpu_ms
			FROM scoped
			GROUP BY statement_fingerprint`

	var topCTE string
	if len(fingerprints) > 0 {
		topCTE = `top AS (` + topSelect + `)`
	} else {
		args = append(args, limit)
		topCTE = fmt.Sprintf(`top AS (%s
			ORDER BY total_cpu_ms DESC NULLS LAST
			LIMIT $%d)`, topSelect, len(args))
	}

	q := fmt.Sprintf(`
		WITH scoped AS (
			SELECT s.capture_timestamp, s.query_hash, s.total_executions, s.total_cpu_ms,
			       %s AS statement_fingerprint,
			       %s AS stmt_text
			FROM sqlserver_query_metrics_v2 s
			WHERE s.server_id = $1
			  AND s.capture_timestamp >= $2 AND s.capture_timestamp <= $3
			  %s%s%s%s
		),
		%s
		SELECT time_bucket('%s', sc.capture_timestamp) AS bucket,
		       sc.statement_fingerprint,
		       t.label_text,
		       t.rep_hash,
		       t.total_executions,
		       t.total_cpu_ms,
		       SUM(sc.total_executions)::bigint AS bucket_executions,
		       SUM(sc.total_cpu_ms)::bigint AS bucket_cpu_ms
		FROM scoped sc
		INNER JOIN top t ON t.statement_fingerprint = sc.statement_fingerprint
		GROUP BY bucket, sc.statement_fingerprint, t.label_text, t.rep_hash, t.total_executions, t.total_cpu_ms
		ORDER BY bucket ASC`, fpExpr, textExpr, dbClause, scope, classFilter, fpFilter, topCTE, bucket)

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerTopQueryTrends: %w", err)
	}
	defer rows.Close()

	byFP := make(map[string]*SqlServerTopQueryTrendSeries)
	order := make([]string, 0)
	for rows.Next() {
		var ts time.Time
		var fp, label string
		var repHash, windowExec, windowCPU, bucketExec, bucketCPU int64
		if err := rows.Scan(&ts, &fp, &label, &repHash, &windowExec, &windowCPU, &bucketExec, &bucketCPU); err != nil {
			continue
		}
		ser, ok := byFP[fp]
		if !ok {
			ser = &SqlServerTopQueryTrendSeries{
				StatementFingerprint: fp,
				QueryHash:            fmt.Sprintf("0x%X", uint64(repHash)),
				Label:                label,
				TotalExecutions:      windowExec,
				TotalCpuMs:           windowCPU,
				Points:               make([]SqlServerTopQueryTrendPoint, 0),
			}
			byFP[fp] = ser
			order = append(order, fp)
		}
		ser.Points = append(ser.Points, SqlServerTopQueryTrendPoint{
			Timestamp:  ts,
			Executions: bucketExec,
			CpuMs:      bucketCPU,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	series := make([]SqlServerTopQueryTrendSeries, 0, len(order))
	for _, fp := range order {
		if s := byFP[fp]; s != nil {
			series = append(series, *s)
		}
	}
	return &SqlServerTopQueryTrendsResponse{Bucket: bucket, Series: series}, nil
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
