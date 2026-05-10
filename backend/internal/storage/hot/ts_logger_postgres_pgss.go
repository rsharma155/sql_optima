// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB read/write methods for the enhanced pg_stat_statements dashboard
//
//	including pgss_query_dim, pgss_delta_1m, workload time-series, top queries,
//	latency approximation, and regression detection.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// latencyEntry holds a single query's call count and mean latency for percentile computation.
type latencyEntry struct {
	calls  int64
	meanMs float64
}

// UpsertPgssQueryDim upserts query dimension metadata into pgss_query_dim.
// On conflict it updates last_seen and enriches db_name/username/query_type if not yet set.
func (tl *TimescaleLogger) UpsertPgssQueryDim(ctx context.Context, instanceName string, rows []PostgresQueryStatsSnapRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO pgss_query_dim
		    (server_instance_name, query_id, query_text, db_name, username, query_type, first_seen, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (server_instance_name, query_id) DO UPDATE SET
		    last_seen  = NOW(),
		    db_name    = COALESCE(EXCLUDED.db_name,    pgss_query_dim.db_name),
		    username   = COALESCE(EXCLUDED.username,   pgss_query_dim.username),
		    query_type = COALESCE(EXCLUDED.query_type, pgss_query_dim.query_type)`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, instanceName, r.QueryID, r.QueryText, r.DbName, r.UserName, r.QueryType)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ComputeAndStorePgssDelta1m computes per-query deltas between the latest two snapshots
// and writes them to pgss_delta_1m for the dashboard time-series.
func (tl *TimescaleLogger) ComputeAndStorePgssDelta1m(ctx context.Context, instanceName string, currentTS time.Time) error {
	// Find the previous snapshot timestamp
	var prevTS *time.Time
	err := tl.pool.QueryRow(ctx,
		`SELECT MAX(capture_timestamp) FROM postgres_query_stats
		 WHERE UPPER(server_instance_name) = UPPER($1) AND capture_timestamp < $2`,
		instanceName, currentTS,
	).Scan(&prevTS)
	if err != nil || prevTS == nil {
		return nil // No previous snapshot yet, skip delta
	}

	prevMap, err := tl.loadPostgresQueryStatsSnapshot(ctx, instanceName, *prevTS)
	if err != nil {
		return fmt.Errorf("loading previous snapshot: %w", err)
	}
	currMap, err := tl.loadPostgresQueryStatsSnapshot(ctx, instanceName, currentTS)
	if err != nil {
		return fmt.Errorf("loading current snapshot: %w", err)
	}

	const q = `INSERT INTO pgss_delta_1m (
		capture_timestamp, server_instance_name, query_id,
		db_name, username, app_name, query_type,
		calls, total_exec_time, rows,
		shared_blks_hit, shared_blks_read, temp_blks_written,
		wal_bytes, total_plan_time, mean_exec_time
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`

	batch := &pgx.Batch{}
	count := 0
	for qid, curr := range currMap {
		base, ok := prevMap[qid]
		if !ok {
			continue // skip first-seen queries to avoid inflated deltas from full cumulative values
		}
		d := subSnap(base, curr)
		if d.Calls <= 0 && d.TotalTimeMs <= 0 {
			continue
		}
		batch.Queue(q,
			currentTS, instanceName, qid,
			d.DbName, d.UserName, "", d.QueryType,
			d.Calls, d.TotalTimeMs, d.Rows,
			d.SharedBlksHit, d.SharedBlksRead, d.TempBlksWritten,
			d.WalBytes, d.TotalPlanTime, d.MeanTimeMs,
		)
		count++
	}
	if count == 0 {
		return nil
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < count; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// PgssWorkloadPoint is a single time-series point for the workload overview charts.
type PgssWorkloadPoint struct {
	Timestamp          time.Time `json:"ts"`
	QueryLoadMsSec     float64   `json:"query_load_ms_sec"`
	QPS                float64   `json:"qps"`
	RowsSec            float64   `json:"rows_sec"`
	CacheHitRatio      float64   `json:"cache_hit_ratio"`
	WalBytesSec        float64   `json:"wal_bytes_sec"`
	PlanningMsSec      float64   `json:"planning_ms_sec"`
	ExecPct            float64   `json:"exec_pct"`
	PlanPct            float64   `json:"plan_pct"`
	RowsPerQuery       float64   `json:"rows_per_query"`
	TempMbSec          float64   `json:"temp_mb_sec"`
	BlksReadSec        float64   `json:"blks_read_sec"`
	TempBlksWrittenSec float64   `json:"temp_blks_written_sec"`
	CpuSaturationMsSec float64   `json:"cpu_saturation_ms_sec"`
}

// GetPgssWorkloadTimeSeries returns per-minute workload metrics from pgss_delta_1m.
func (tl *TimescaleLogger) GetPgssWorkloadTimeSeries(ctx context.Context, instanceName string, from, to time.Time) ([]PgssWorkloadPoint, error) {
	const q = `
		SELECT time_bucket('1 minute', capture_timestamp) AS bucket,
			SUM(total_exec_time),
			SUM(calls),
			SUM(rows),
			SUM(shared_blks_hit),
			SUM(shared_blks_read),
			SUM(wal_bytes),
			SUM(total_plan_time),
			SUM(temp_blks_written)
		FROM pgss_delta_1m
		WHERE UPPER(server_instance_name) = UPPER($1) AND capture_timestamp >= $2 AND capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket`
	rows, err := tl.pool.Query(ctx, q, instanceName, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	const defaultCPUCores = 4

	var points []PgssWorkloadPoint
	for rows.Next() {
		var bucket time.Time
		var totalExec, totalCalls, totalRows, blksHit, blksRead, walBytes, planTime, tempBlksWritten float64
		if err := rows.Scan(&bucket, &totalExec, &totalCalls, &totalRows, &blksHit, &blksRead, &walBytes, &planTime, &tempBlksWritten); err != nil {
			continue
		}
		p := PgssWorkloadPoint{
			Timestamp:          bucket,
			QueryLoadMsSec:     totalExec / 60.0,
			QPS:                totalCalls / 60.0,
			RowsSec:            totalRows / 60.0,
			WalBytesSec:        walBytes / 60.0,
			PlanningMsSec:      planTime / 60.0,
			TempMbSec:          (tempBlksWritten * 8 / 1024) / 60.0,
			BlksReadSec:        blksRead / 60.0,
			TempBlksWrittenSec: tempBlksWritten / 60.0,
			CpuSaturationMsSec: float64(defaultCPUCores) * 1000.0,
		}
		if p.QPS > 0 {
			p.RowsPerQuery = p.RowsSec / p.QPS
		}
		if blksHit+blksRead > 0 {
			p.CacheHitRatio = (blksHit / (blksHit + blksRead)) * 100.0
		} else {
			p.CacheHitRatio = 100.0
		}
		totalTime := totalExec + planTime
		if totalTime > 0 {
			p.ExecPct = (totalExec / totalTime) * 100.0
			p.PlanPct = (planTime / totalTime) * 100.0
		} else {
			p.ExecPct = 100.0
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// PgssTopQuery represents a single row in the top queries table.
type PgssTopQuery struct {
	QueryID      int64    `json:"query_id"`
	Query        string   `json:"query"`
	DbName       string   `json:"db_name"`
	UserName     string   `json:"username"`
	AppName      string   `json:"app_name"`
	QueryType    string   `json:"query_type"`
	TotalTime    float64  `json:"total_time_ms"`
	PctDBTime    float64  `json:"pct_db_time"`
	Calls        int64    `json:"calls"`
	AvgMs        float64  `json:"avg_ms"`
	RowsPerCall  float64  `json:"rows_per_call"`
	HitPct       float64  `json:"hit_pct"`
	TempMB       float64  `json:"temp_mb"`
	WalMB        float64  `json:"wal_mb"`
	ReadsPerCall float64  `json:"reads_per_call"`
	PlanRatio    float64  `json:"plan_ratio"`
	Flags        []string `json:"flags"`
}

// PgssFilterOptions holds distinct dimension values for populating filter dropdowns.
type PgssFilterOptions struct {
	Databases []string `json:"databases"`
	Users     []string `json:"users"`
	AppNames  []string `json:"app_names"`
}

// PgssDbBreakdown is a per-database workload summary row.
type PgssDbBreakdown struct {
	DbName         string  `json:"db_name"`
	TotalExecMs    float64 `json:"total_exec_ms"`
	PctOfServer    float64 `json:"pct_of_server"`
	TotalCalls     int64   `json:"total_calls"`
	AvgMs          float64 `json:"avg_ms"`
	CacheHitPct    float64 `json:"cache_hit_pct"`
	UniqueQueryIDs int64   `json:"unique_query_ids"`
}

// PgssUserBreakdown is a per-login workload summary row.
type PgssUserBreakdown struct {
	UserName       string  `json:"username"`
	TotalExecMs    float64 `json:"total_exec_ms"`
	PctOfServer    float64 `json:"pct_of_server"`
	TotalCalls     int64   `json:"total_calls"`
	AvgMs          float64 `json:"avg_ms"`
	UniqueQueryIDs int64   `json:"unique_query_ids"`
}

// GetPgssTopQueries returns top queries from the delta table for a time window, sorted by the given metric.
// Optional filters: dbName, userName, appName, queryType (empty = all).
func (tl *TimescaleLogger) GetPgssTopQueries(
	ctx context.Context,
	instanceName string,
	from, to time.Time,
	sortBy string,
	limit int,
	dbName, userName, appName, queryType string,
	hideSystem bool,
) ([]PgssTopQuery, error) {
	if limit <= 0 {
		limit = 50
	}

	orderCol := "total_exec_time"
	switch sortBy {
	case "mean_time":
		orderCol = "avg_ms"
	case "calls":
		orderCol = "total_calls"
	case "io":
		orderCol = "total_blks_read"
	case "temp":
		orderCol = "total_temp"
	case "wal":
		orderCol = "total_wal"
	case "planning":
		orderCol = "total_plan"
	}

	// Build optional filter predicates
	args := []interface{}{instanceName, from, to}
	argIdx := 4
	var filterClauses []string
	if dbName != "" {
		filterClauses = append(filterClauses, fmt.Sprintf("d.db_name = $%d", argIdx))
		args = append(args, dbName)
		argIdx++
	}
	if userName != "" {
		filterClauses = append(filterClauses, fmt.Sprintf("d.username = $%d", argIdx))
		args = append(args, userName)
		argIdx++
	}
	if appName != "" {
		filterClauses = append(filterClauses, fmt.Sprintf("d.app_name = $%d", argIdx))
		args = append(args, appName)
		argIdx++
	}
	if queryType != "" {
		filterClauses = append(filterClauses, fmt.Sprintf("d.query_type = $%d", argIdx))
		args = append(args, queryType)
		argIdx++
	}
	extraWhere := ""
	if len(filterClauses) > 0 {
		extraWhere = "AND " + strings.Join(filterClauses, " AND ")
	}
	
	hideSysIdx := argIdx
	args = append(args, hideSystem)
	argIdx++
	
	limitArg := argIdx
	args = append(args, limit)

	q := fmt.Sprintf(`
		WITH agg AS (
			SELECT d.query_id,
				COALESCE(d.db_name, '')    AS db_name,
				COALESCE(d.username, '')   AS username,
				COALESCE(d.app_name, '')   AS app_name,
				COALESCE(d.query_type,'O') AS query_type,
				SUM(d.total_exec_time) AS total_exec_time,
				SUM(d.calls) AS total_calls,
				SUM(d.rows) AS total_rows,
				SUM(d.shared_blks_hit) AS total_hit,
				SUM(d.shared_blks_read) AS total_blks_read,
				SUM(d.temp_blks_written) AS total_temp,
				SUM(d.wal_bytes) AS total_wal,
				SUM(d.total_plan_time) AS total_plan
			FROM pgss_delta_1m d
			WHERE UPPER(d.server_instance_name) = UPPER($1)
			  AND d.capture_timestamp >= $2 AND d.capture_timestamp <= $3
			  %s
			GROUP BY d.query_id, d.db_name, d.username, d.app_name, d.query_type
			HAVING SUM(d.calls) > 0
		),
		grand AS (
			SELECT SUM(total_exec_time) AS db_total FROM agg
		)
		SELECT a.query_id,
			COALESCE(q.query_text, ''),
			a.db_name, a.username, a.app_name, a.query_type,
			a.total_exec_time,
			CASE WHEN g.db_total > 0 THEN (a.total_exec_time / g.db_total) * 100.0 ELSE 0 END,
			a.total_calls,
			a.total_exec_time / NULLIF(a.total_calls, 0) AS avg_ms,
			a.total_rows::float / NULLIF(a.total_calls, 0),
			CASE WHEN a.total_hit + a.total_blks_read > 0
				THEN (a.total_hit::float / (a.total_hit + a.total_blks_read)) * 100.0
				ELSE 100.0 END,
			a.total_temp::float * 8.0 / 1024.0,
			a.total_wal::float / (1024.0 * 1024.0),
			a.total_blks_read::float / NULLIF(a.total_calls, 0),
			a.total_plan / NULLIF(a.total_exec_time, 0)
		FROM agg a
		CROSS JOIN grand g
		LEFT JOIN pgss_query_dim q ON UPPER(q.server_instance_name) = UPPER($1) AND q.query_id = a.query_id
		WHERE (NOT $%d OR (
			COALESCE(q.query_text, '') NOT LIKE '%%%%/* SQL_OPTIMA */%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%pg_catalog%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%information_schema%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%pg_stat_%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%pg_locks%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%fetch %%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%declare %%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%begin%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%commit%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%rollback%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%savepoint%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%autovacuum:%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%analyze %%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%vacuum %%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%checkpoint%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%set %%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%show %%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%SELECT 1%%%%'
			AND COALESCE(q.query_text, '') NOT ILIKE '%%%%pg_is_in_recovery%%%%'
		))
		ORDER BY %s DESC
		LIMIT $%d`, extraWhere, hideSysIdx, orderCol, limitArg)

	pgRows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()

	var results []PgssTopQuery
	for pgRows.Next() {
		var r PgssTopQuery
		if err := pgRows.Scan(&r.QueryID, &r.Query,
			&r.DbName, &r.UserName, &r.AppName, &r.QueryType,
			&r.TotalTime, &r.PctDBTime, &r.Calls,
			&r.AvgMs, &r.RowsPerCall, &r.HitPct, &r.TempMB, &r.WalMB,
			&r.ReadsPerCall, &r.PlanRatio); err != nil {
			continue
		}
		var flags []string
		if r.HitPct < 95.0 {
			flags = append(flags, "IO")
		}
		if r.TempMB > 0 {
			flags = append(flags, "TEMP")
		}
		if r.PlanRatio > 0.1 {
			flags = append(flags, "PLAN")
		}
		if r.WalMB > 10.0 {
			flags = append(flags, "WAL")
		}
		r.Flags = flags
		results = append(results, r)
	}
	return results, pgRows.Err()
}

// GetPgssFilterOptions returns distinct db_name, username, app_name values for a time window.
func (tl *TimescaleLogger) GetPgssFilterOptions(ctx context.Context, instanceName string, from, to time.Time) (*PgssFilterOptions, error) {
	if tl == nil || tl.pool == nil {
		return &PgssFilterOptions{}, nil
	}
	const q = `
		SELECT
			ARRAY_AGG(DISTINCT db_name  ORDER BY db_name)  FILTER (WHERE db_name  IS NOT NULL AND db_name  <> ''),
			ARRAY_AGG(DISTINCT username ORDER BY username) FILTER (WHERE username IS NOT NULL AND username <> ''),
			ARRAY_AGG(DISTINCT app_name ORDER BY app_name) FILTER (WHERE app_name IS NOT NULL AND app_name <> '')
		FROM pgss_delta_1m
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp >= $2 AND capture_timestamp <= $3`
	var dbs, users, apps []string
	err := tl.pool.QueryRow(ctx, q, instanceName, from, to).Scan(&dbs, &users, &apps)
	if err != nil {
		return &PgssFilterOptions{}, nil
	}
	return &PgssFilterOptions{
		Databases: orEmpty(dbs),
		Users:     orEmpty(users),
		AppNames:  orEmpty(apps),
	}, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// GetPgssDbBreakdown returns per-database workload totals for the By Database tab.
func (tl *TimescaleLogger) GetPgssDbBreakdown(ctx context.Context, instanceName string, from, to time.Time) ([]PgssDbBreakdown, error) {
	const q = `
		WITH agg AS (
			SELECT
				COALESCE(db_name, 'unknown')                                   AS db_name,
				SUM(total_exec_time)                                           AS total_exec_ms,
				SUM(calls)                                                     AS total_calls,
				SUM(shared_blks_hit)                                           AS total_hit,
				SUM(shared_blks_read)                                          AS total_read,
				COUNT(DISTINCT query_id)                                       AS unique_queries
			FROM pgss_delta_1m
			WHERE UPPER(server_instance_name) = UPPER($1)
			  AND capture_timestamp >= $2 AND capture_timestamp <= $3
			GROUP BY db_name
		),
		grand AS (SELECT NULLIF(SUM(total_exec_ms), 0) AS total FROM agg)
		SELECT
			a.db_name,
			a.total_exec_ms,
			COALESCE(a.total_exec_ms / g.total * 100.0, 0) AS pct_of_server,
			a.total_calls,
			a.total_exec_ms / NULLIF(a.total_calls, 0)     AS avg_ms,
			CASE WHEN a.total_hit + a.total_read > 0
			     THEN a.total_hit::float / (a.total_hit + a.total_read) * 100.0
			     ELSE 100.0 END                             AS cache_hit_pct,
			a.unique_queries
		FROM agg a CROSS JOIN grand g
		ORDER BY a.total_exec_ms DESC
		LIMIT 20`

	pgRows, err := tl.pool.Query(ctx, q, instanceName, from, to)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()
	var out []PgssDbBreakdown
	for pgRows.Next() {
		var r PgssDbBreakdown
		if err := pgRows.Scan(&r.DbName, &r.TotalExecMs, &r.PctOfServer, &r.TotalCalls, &r.AvgMs, &r.CacheHitPct, &r.UniqueQueryIDs); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}

// GetPgssUserBreakdown returns per-login workload totals for the By User tab.
func (tl *TimescaleLogger) GetPgssUserBreakdown(ctx context.Context, instanceName string, from, to time.Time) ([]PgssUserBreakdown, error) {
	const q = `
		WITH agg AS (
			SELECT
				COALESCE(username, 'unknown')                                  AS username,
				SUM(total_exec_time)                                           AS total_exec_ms,
				SUM(calls)                                                     AS total_calls,
				COUNT(DISTINCT query_id)                                       AS unique_queries
			FROM pgss_delta_1m
			WHERE UPPER(server_instance_name) = UPPER($1)
			  AND capture_timestamp >= $2 AND capture_timestamp <= $3
			GROUP BY username
		),
		grand AS (SELECT NULLIF(SUM(total_exec_ms), 0) AS total FROM agg)
		SELECT
			a.username,
			a.total_exec_ms,
			COALESCE(a.total_exec_ms / g.total * 100.0, 0) AS pct_of_server,
			a.total_calls,
			a.total_exec_ms / NULLIF(a.total_calls, 0)     AS avg_ms,
			a.unique_queries
		FROM agg a CROSS JOIN grand g
		ORDER BY a.total_exec_ms DESC
		LIMIT 20`

	pgRows, err := tl.pool.Query(ctx, q, instanceName, from, to)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()
	var out []PgssUserBreakdown
	for pgRows.Next() {
		var r PgssUserBreakdown
		if err := pgRows.Scan(&r.UserName, &r.TotalExecMs, &r.PctOfServer, &r.TotalCalls, &r.AvgMs, &r.UniqueQueryIDs); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}

// PgssLatencyPoint holds latency percentile approximations for a time bucket.
type PgssLatencyPoint struct {
	Timestamp time.Time `json:"ts"`
	P50       float64   `json:"p50"`
	P95       float64   `json:"p95"`
	P99       float64   `json:"p99"`
	MaxExec   float64   `json:"max_exec"`
}

// GetPgssLatencyTimeSeries approximates latency percentiles from mean_exec_time per query.
// True percentiles require PG17+ histogram buckets; this uses weighted approximation.
func (tl *TimescaleLogger) GetPgssLatencyTimeSeries(ctx context.Context, instanceName string, from, to time.Time) ([]PgssLatencyPoint, error) {
	const q = `
		SELECT capture_timestamp, query_id, calls, mean_exec_time
		FROM pgss_delta_1m
		WHERE UPPER(server_instance_name) = UPPER($1) AND capture_timestamp >= $2 AND capture_timestamp <= $3
			AND calls > 0
		ORDER BY capture_timestamp, mean_exec_time`

	rows, err := tl.pool.Query(ctx, q, instanceName, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Group by timestamp bucket
	buckets := make(map[time.Time][]latencyEntry)
	for rows.Next() {
		var ts time.Time
		var qid int64
		var calls int64
		var meanMs float64
		if err := rows.Scan(&ts, &qid, &calls, &meanMs); err != nil {
			continue
		}
		buckets[ts] = append(buckets[ts], latencyEntry{calls: calls, meanMs: meanMs})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort bucket keys
	keys := make([]time.Time, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	var points []PgssLatencyPoint
	for _, ts := range keys {
		entries := buckets[ts]
		// Sort by mean latency for percentile approximation
		sort.Slice(entries, func(i, j int) bool { return entries[i].meanMs < entries[j].meanMs })

		var totalCalls int64
		var maxMs float64
		for _, e := range entries {
			totalCalls += e.calls
			if e.meanMs > maxMs {
				maxMs = e.meanMs
			}
		}
		if totalCalls == 0 {
			continue
		}

		p := PgssLatencyPoint{
			Timestamp: ts,
			MaxExec:   maxMs,
			P50:       weightedPercentile(entries, totalCalls, 0.50),
			P95:       weightedPercentile(entries, totalCalls, 0.95),
			P99:       weightedPercentile(entries, totalCalls, 0.99),
		}
		points = append(points, p)
	}
	return points, nil
}

// weightedPercentile computes an approximate percentile from sorted (by meanMs) entries weighted by calls.
func weightedPercentile(entries []latencyEntry, totalCalls int64, pct float64) float64 {
	target := int64(math.Ceil(float64(totalCalls) * pct))
	var cumulative int64
	for _, e := range entries {
		cumulative += e.calls
		if cumulative >= target {
			return e.meanMs
		}
	}
	if len(entries) > 0 {
		return entries[len(entries)-1].meanMs
	}
	return 0
}

// PgssRegression represents a query that degraded between two time windows.
type PgssRegression struct {
	QueryID    int64     `json:"query_id"`
	Query      string    `json:"query"`
	PrevAvgMs  float64   `json:"prev_avg_ms"`
	CurrAvgMs  float64   `json:"curr_avg_ms"`
	ChangePct  float64   `json:"change_pct"`
	Status     string    `json:"status"`
	DetectedAt time.Time `json:"detected_at"`
}

// GetPgssRegressions compares the last 30m vs previous 30m and returns queries where mean_exec_time increased >50%.
func (tl *TimescaleLogger) GetPgssRegressions(ctx context.Context, instanceName string) ([]PgssRegression, error) {
	now := time.Now().UTC()
	mid := now.Add(-30 * time.Minute)
	start := now.Add(-60 * time.Minute)

	const q = `
		WITH prev AS (
			SELECT query_id,
				SUM(total_exec_time) / NULLIF(SUM(calls), 0) AS avg_ms,
				SUM(calls) AS total_calls
			FROM pgss_delta_1m
			WHERE UPPER(server_instance_name) = UPPER($1) AND capture_timestamp >= $2 AND capture_timestamp < $3
			GROUP BY query_id
			HAVING SUM(calls) > 0
		),
		curr AS (
			SELECT query_id,
				SUM(total_exec_time) / NULLIF(SUM(calls), 0) AS avg_ms,
				SUM(calls) AS total_calls
			FROM pgss_delta_1m
			WHERE UPPER(server_instance_name) = UPPER($1) AND capture_timestamp >= $3 AND capture_timestamp <= $4
			GROUP BY query_id
			HAVING SUM(calls) > 0
		)
		SELECT c.query_id,
			COALESCE(q.query_text, ''),
			p.avg_ms,
			c.avg_ms,
			((c.avg_ms - p.avg_ms) / NULLIF(p.avg_ms, 0)) * 100.0
		FROM curr c
		JOIN prev p ON p.query_id = c.query_id
		LEFT JOIN pgss_query_dim q ON UPPER(q.server_instance_name) = UPPER($1) AND q.query_id = c.query_id
		WHERE p.avg_ms > 0 AND ((c.avg_ms - p.avg_ms) / p.avg_ms) > 0.5
		  AND COALESCE(q.query_text, '') NOT LIKE '%%/* SQL_OPTIMA */%%'
		ORDER BY ((c.avg_ms - p.avg_ms) / NULLIF(p.avg_ms, 0)) DESC
		LIMIT 20`

	rows, err := tl.pool.Query(ctx, q, instanceName, start, mid, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PgssRegression
	for rows.Next() {
		var r PgssRegression
		if err := rows.Scan(&r.QueryID, &r.Query, &r.PrevAvgMs, &r.CurrAvgMs, &r.ChangePct); err != nil {
			continue
		}
		if r.ChangePct > 100 {
			r.Status = "Degraded"
		} else {
			r.Status = "Warning"
		}
		r.DetectedAt = now
		results = append(results, r)
	}
	return results, rows.Err()
}

// PgssSummary holds aggregate KPI values for the summary strip.
type PgssSummary struct {
	QueryLoadMsSec     float64
	QPS                float64
	P99Ms              float64
	CacheHitPct        float64
	TempMbSec          float64
	WalMbSec           float64
	CpuSaturationMsSec float64
	UniqueQueryCount   int64
}

// GetPgssSummary returns aggregate KPI metrics for the incident summary strip.
func (tl *TimescaleLogger) GetPgssSummary(ctx context.Context, instanceName string, from, to time.Time) (*PgssSummary, error) {
	intervalSec := to.Sub(from).Seconds()
	if intervalSec <= 0 {
		intervalSec = 3600
	}

	const q = `
		SELECT
			COALESCE(SUM(total_exec_time), 0),
			COALESCE(SUM(calls), 0),
			COALESCE(SUM(shared_blks_hit), 0),
			COALESCE(SUM(shared_blks_read), 0),
			COALESCE(SUM(temp_blks_written), 0),
			COALESCE(SUM(wal_bytes), 0),
			COUNT(DISTINCT query_id)
		FROM pgss_delta_1m
		WHERE UPPER(server_instance_name) = UPPER($1) AND capture_timestamp >= $2 AND capture_timestamp <= $3`

	var totalExec, totalCalls, blksHit, blksRead, tempBlks, walBytes float64
	var uniqueQueries int64
	if err := tl.pool.QueryRow(ctx, q, instanceName, from, to).Scan(
		&totalExec, &totalCalls, &blksHit, &blksRead, &tempBlks, &walBytes, &uniqueQueries,
	); err != nil {
		return nil, err
	}

	s := &PgssSummary{
		QueryLoadMsSec:     totalExec / intervalSec,
		QPS:                totalCalls / intervalSec,
		TempMbSec:          (tempBlks * 8 / 1024) / intervalSec,
		WalMbSec:           (walBytes / (1024 * 1024)) / intervalSec,
		CpuSaturationMsSec: 4 * 1000.0, // default 4 cores
		UniqueQueryCount:   uniqueQueries,
	}

	if blksHit+blksRead > 0 {
		s.CacheHitPct = (blksHit / (blksHit + blksRead)) * 100.0
	} else {
		s.CacheHitPct = 100.0
	}

	// Approximate P99 from per-query mean latencies weighted by calls
	const p99q = `
		SELECT calls, mean_exec_time
		FROM pgss_delta_1m
		WHERE UPPER(server_instance_name) = UPPER($1) AND capture_timestamp >= $2 AND capture_timestamp <= $3
			AND calls > 0
		ORDER BY mean_exec_time`
	rows, err := tl.pool.Query(ctx, p99q, instanceName, from, to)
	if err != nil {
		return s, nil // return partial summary without p99
	}
	defer rows.Close()

	var entries []latencyEntry
	var totalCallsP99 int64
	for rows.Next() {
		var e latencyEntry
		if err := rows.Scan(&e.calls, &e.meanMs); err != nil {
			continue
		}
		entries = append(entries, e)
		totalCallsP99 += e.calls
	}
	if totalCallsP99 > 0 {
		s.P99Ms = weightedPercentile(entries, totalCallsP99, 0.99)
	}

	return s, nil
}
