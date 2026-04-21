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
	"time"

	"github.com/jackc/pgx/v5"
)

// latencyEntry holds a single query's call count and mean latency for percentile computation.
type latencyEntry struct {
	calls  int64
	meanMs float64
}

// UpsertPgssQueryDim inserts new query text into the dimension table (INSERT ON CONFLICT DO NOTHING).
func (tl *TimescaleLogger) UpsertPgssQueryDim(ctx context.Context, instanceName string, rows []PostgresQueryStatsSnapRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO pgss_query_dim (server_instance_name, query_id, query_text)
		VALUES ($1, $2, $3) ON CONFLICT (server_instance_name, query_id) DO NOTHING`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, instanceName, r.QueryID, r.QueryText)
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
		 WHERE server_instance_name = $1 AND capture_timestamp < $2`,
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
		calls, total_exec_time, rows,
		shared_blks_hit, shared_blks_read, temp_blks_written,
		wal_bytes, total_plan_time, mean_exec_time
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`

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
		WHERE server_instance_name = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
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

// GetPgssTopQueries returns top queries from the delta table for a time window, sorted by the given metric.
func (tl *TimescaleLogger) GetPgssTopQueries(ctx context.Context, instanceName string, from, to time.Time, sortBy string, limit int) ([]PgssTopQuery, error) {
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

	q := fmt.Sprintf(`
		WITH agg AS (
			SELECT d.query_id,
				SUM(d.total_exec_time) AS total_exec_time,
				SUM(d.calls) AS total_calls,
				SUM(d.rows) AS total_rows,
				SUM(d.shared_blks_hit) AS total_hit,
				SUM(d.shared_blks_read) AS total_blks_read,
				SUM(d.temp_blks_written) AS total_temp,
				SUM(d.wal_bytes) AS total_wal,
				SUM(d.total_plan_time) AS total_plan
			FROM pgss_delta_1m d
			WHERE d.server_instance_name = $1 AND d.capture_timestamp >= $2 AND d.capture_timestamp <= $3
			GROUP BY d.query_id
			HAVING SUM(d.calls) > 0
		),
		grand AS (
			SELECT SUM(total_exec_time) AS db_total FROM agg
		)
		SELECT a.query_id,
			COALESCE(q.query_text, ''),
			a.total_exec_time,
			CASE WHEN g.db_total > 0 THEN (a.total_exec_time / g.db_total) * 100.0 ELSE 0 END,
			a.total_calls,
			a.total_exec_time / NULLIF(a.total_calls, 0) AS avg_ms,
			a.total_rows::float / NULLIF(a.total_calls, 0),
			CASE WHEN a.total_hit + a.total_blks_read > 0
				THEN (a.total_hit::float / (a.total_hit + a.total_blks_read)) * 100.0
				ELSE 100.0 END,
			a.total_temp::float * 8.0 / 1024.0, -- Convert 8KB blocks to MB
			a.total_wal::float / (1024.0 * 1024.0), -- Convert bytes to MB
			a.total_blks_read::float / NULLIF(a.total_calls, 0),
			a.total_plan / NULLIF(a.total_exec_time, 0)
		FROM agg a
		CROSS JOIN grand g
		LEFT JOIN pgss_query_dim q ON q.server_instance_name = $1 AND q.query_id = a.query_id
		ORDER BY %s DESC
		LIMIT $4`, orderCol)

	rows, err := tl.pool.Query(ctx, q, instanceName, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PgssTopQuery
	for rows.Next() {
		var r PgssTopQuery
		if err := rows.Scan(&r.QueryID, &r.Query, &r.TotalTime, &r.PctDBTime, &r.Calls,
			&r.AvgMs, &r.RowsPerCall, &r.HitPct, &r.TempMB, &r.WalMB,
			&r.ReadsPerCall, &r.PlanRatio); err != nil {
			continue
		}
		// Compute tuning flags
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
	return results, rows.Err()
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
		WHERE server_instance_name = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
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
	QueryID   int64   `json:"query_id"`
	Query     string  `json:"query"`
	PrevAvgMs float64 `json:"prev_avg_ms"`
	CurrAvgMs float64 `json:"curr_avg_ms"`
	ChangePct float64 `json:"change_pct"`
	Status    string  `json:"status"`
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
			WHERE server_instance_name = $1 AND capture_timestamp >= $2 AND capture_timestamp < $3
			GROUP BY query_id
			HAVING SUM(calls) > 0
		),
		curr AS (
			SELECT query_id,
				SUM(total_exec_time) / NULLIF(SUM(calls), 0) AS avg_ms,
				SUM(calls) AS total_calls
			FROM pgss_delta_1m
			WHERE server_instance_name = $1 AND capture_timestamp >= $3 AND capture_timestamp <= $4
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
		LEFT JOIN pgss_query_dim q ON q.server_instance_name = $1 AND q.query_id = c.query_id
		WHERE p.avg_ms > 0 AND ((c.avg_ms - p.avg_ms) / p.avg_ms) > 0.5
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
			COALESCE(SUM(wal_bytes), 0)
		FROM pgss_delta_1m
		WHERE server_instance_name = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3`

	var totalExec, totalCalls, blksHit, blksRead, tempBlks, walBytes float64
	if err := tl.pool.QueryRow(ctx, q, instanceName, from, to).Scan(
		&totalExec, &totalCalls, &blksHit, &blksRead, &tempBlks, &walBytes,
	); err != nil {
		return nil, err
	}

	s := &PgssSummary{
		QueryLoadMsSec:     totalExec / intervalSec,
		QPS:                totalCalls / intervalSec,
		TempMbSec:          (tempBlks * 8 / 1024) / intervalSec,
		WalMbSec:           (walBytes / (1024 * 1024)) / intervalSec,
		CpuSaturationMsSec: 4 * 1000.0, // default 4 cores
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
		WHERE server_instance_name = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
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
