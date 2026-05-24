// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: CPU and settings-related PostgresHandlers implementations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgssExcludedUsers are usernames excluded from user-query dashboards.
// Mirrors repository.pgStatStatementsExcludedRoleNames defaults.
var pgssExcludedUsers = []string{
	"dbmonitor_user", "dbmonitor", "sql_optima", "sqloptima",
	"pgbouncer", "repmgr", "barman", "patroni", "pg_monitor", "streaming_replica",
}

// CPUTopQueries returns the top user queries by total execution time for the selected time window,
// sourced from pgss_delta_1m joined with pgss_query_dim. Monitoring/infrastructure users are excluded.
func (h *PostgresHandlers) CPUTopQueries(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"queries": []interface{}{}})
		return
	}

	includeSystem := r.URL.Query().Get("include_system") == "true"

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	queries, err := queryTopQueriesByCPU(r.Context(), pool, sid, from, to, 20, includeSystem)
	if err != nil {
		queries = []map[string]interface{}{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"queries": queries})
}

// CPUDatabase returns per-database total execution time for the selected time window,
// sourced from pgss_delta_1m. Monitoring/infrastructure users are excluded.
func (h *PostgresHandlers) CPUDatabase(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"rows": []interface{}{}})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	rows, err := queryDBLoadByCPU(r.Context(), pool, sid, from, to)
	if err != nil {
		rows = []map[string]interface{}{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"rows": rows})
}

// CPUPgTimeSeries returns time-bucketed PostgreSQL execution metrics (proxy for CPU load)
// from pgss_delta_1m. Bucket width adapts to the requested time range.
func (h *PostgresHandlers) CPUPgTimeSeries(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"points": []interface{}{}})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	points, err := queryPgExecTimeSeries(r.Context(), pool, sid, from, to)
	if err != nil {
		points = []map[string]interface{}{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"points": points})
}

// CPUQueryTypes returns total calls and execution time grouped by query type (S/I/U/D/E/O)
// for the selected time window.
func (h *PostgresHandlers) CPUQueryTypes(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"types": []interface{}{}})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	types, err := queryQueryTypeBreakdown(r.Context(), pool, sid, from, to)
	if err != nil {
		types = []map[string]interface{}{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"types": types})
}

// SettingsDrift returns settings that changed between consecutive snapshots within the last 7 days,
// sourced from postgres_settings_snapshot.
func (h *PostgresHandlers) SettingsDrift(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"changes": []interface{}{}})
		return
	}
	changes, err := querySettingsDrift(r.Context(), pool, sid)
	if err != nil {
		changes = []map[string]interface{}{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"changes": changes})
}

// queryTopQueriesByCPU returns user queries ranked by total execution time.
// Excludes monitoring/infrastructure usernames and internal SQL_OPTIMA queries.
func queryTopQueriesByCPU(ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID, from, to time.Time, limit int, includeSystem bool) ([]map[string]interface{}, error) {
	where := "d.server_id = $1 AND d.capture_timestamp BETWEEN $2 AND $3"
	args := []interface{}{serverID, from, to}

	if !includeSystem {
		args = append(args, pgssExcludedUsers)
		where += fmt.Sprintf(" AND d.username != ALL($%d::text[])", len(args))
		where += " AND COALESCE(q.query_text, '') NOT LIKE '%/* SQL_OPTIMA */%'"
		where += " AND COALESCE(q.query_text, '') NOT LIKE '%pg_stat_statements%'"
	}

	args = append(args, limit)
	q := fmt.Sprintf(`
		SELECT
			max(d.capture_timestamp)   AS captured_at,
			COALESCE(d.username, '')   AS user_name,
			COALESCE(q.query_text, '') AS query,
			sum(d.total_exec_time)     AS total_exec_time,
			sum(d.calls)               AS calls
		FROM pgss_delta_1m d
		LEFT JOIN pgss_query_dim q
			ON q.server_id = d.server_id AND q.query_id = d.query_id
		WHERE %s
		GROUP BY d.query_id, d.username, q.query_text
		ORDER BY total_exec_time DESC
		LIMIT $%d`, where, len(args))

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var capturedAt time.Time
		var userName, query string
		var totalExec float64
		var calls int64
		if err := rows.Scan(&capturedAt, &userName, &query, &totalExec, &calls); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"captured_at":     capturedAt,
			"user_name":       userName,
			"query":           query,
			"total_exec_time": totalExec,
			"calls":           calls,
		})
	}
	return results, nil
}

// queryDBLoadByCPU returns per-database total execution time, excluding monitoring users.
func queryDBLoadByCPU(ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	const q = `
		SELECT
			COALESCE(db_name, 'unknown') AS datname,
			sum(total_exec_time)         AS total_exec_time_ms
		FROM pgss_delta_1m
		WHERE server_id = $1
		  AND capture_timestamp BETWEEN $2 AND $3
		  AND username != ALL($4::text[])
		GROUP BY db_name
		ORDER BY total_exec_time_ms DESC
		LIMIT 10`

	rows, err := pool.Query(ctx, q, serverID, from, to, pgssExcludedUsers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var datname string
		var execTime float64
		if err := rows.Scan(&datname, &execTime); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"datname":            datname,
			"total_exec_time_ms": execTime,
		})
	}
	return results, nil
}

// computeBucketInterval returns a TimescaleDB time_bucket interval string based on the query range.
// Larger ranges use wider buckets to keep the result set manageable (≤ ~200 rows).
func computeBucketInterval(from, to time.Time) string {
	d := to.Sub(from)
	switch {
	case d <= 3*time.Hour:
		return "1 minute"
	case d <= 24*time.Hour:
		return "5 minutes"
	case d <= 7*24*time.Hour:
		return "30 minutes"
	default:
		return "1 hour"
	}
}

// queryPgExecTimeSeries returns time-bucketed PostgreSQL execution metrics from pgss_delta_1m.
// total_exec_time_ms is the primary load proxy; avg_exec_time_ms tracks per-query latency;
// cache_hit_pct shows buffer cache efficiency.
func queryPgExecTimeSeries(ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	bucket := computeBucketInterval(from, to)
	// bucket is computed from hardcoded switch cases — no user input, no injection risk.
	q := fmt.Sprintf(`
		SELECT
			time_bucket('%s', capture_timestamp)              AS bucket,
			SUM(total_exec_time)                              AS total_exec_time_ms,
			SUM(calls)                                        AS calls,
			CASE WHEN SUM(calls) > 0
			     THEN SUM(total_exec_time) / SUM(calls)
			     ELSE 0 END                                   AS avg_exec_time_ms,
			SUM(shared_blks_hit)                              AS cache_hits,
			SUM(shared_blks_read)                             AS cache_reads,
			SUM(temp_blks_written)                            AS temp_writes
		FROM pgss_delta_1m
		WHERE server_id = $1
		  AND capture_timestamp BETWEEN $2 AND $3
		  AND username != ALL($4::text[])
		GROUP BY bucket
		ORDER BY bucket
		LIMIT 200`, bucket)

	rows, err := pool.Query(ctx, q, serverID, from, to, pgssExcludedUsers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var totalExec, avgExec float64
		var calls, cacheHits, cacheReads, tempWrites int64
		if err := rows.Scan(&bucket, &totalExec, &calls, &avgExec, &cacheHits, &cacheReads, &tempWrites); err != nil {
			continue
		}
		total := cacheHits + cacheReads
		cacheHitPct := 0.0
		if total > 0 {
			cacheHitPct = float64(cacheHits) / float64(total) * 100
		}
		results = append(results, map[string]interface{}{
			"timestamp":         bucket,
			"total_exec_time_ms": totalExec,
			"calls":             calls,
			"avg_exec_time_ms":  avgExec,
			"cache_hit_pct":     cacheHitPct,
			"temp_writes":       tempWrites,
		})
	}
	return results, nil
}

// queryQueryTypeBreakdown returns total calls and execution time grouped by query type.
// Types: S=SELECT, I=INSERT, U=UPDATE, D=DELETE, E=DDL, O=Other.
func queryQueryTypeBreakdown(ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	const q = `
		SELECT
			COALESCE(d.query_type, 'O') AS query_type,
			SUM(d.calls)                AS calls,
			SUM(d.total_exec_time)      AS total_exec_time_ms
		FROM pgss_delta_1m d
		WHERE d.server_id = $1
		  AND d.capture_timestamp BETWEEN $2 AND $3
		  AND d.username != ALL($4::text[])
		GROUP BY d.query_type
		ORDER BY total_exec_time_ms DESC`

	rows, err := pool.Query(ctx, q, serverID, from, to, pgssExcludedUsers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var qType string
		var calls int64
		var execTime float64
		if err := rows.Scan(&qType, &calls, &execTime); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"query_type":         qType,
			"calls":              calls,
			"total_exec_time_ms": execTime,
		})
	}
	return results, nil
}

// queryPgssTopQueries aggregates per-query execution stats from pgss_delta_1m for the Queries endpoint.
func queryPgssTopQueries(ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	const q = `
		SELECT
			d.query_id,
			COALESCE(MAX(q.username), MAX(d.username), '')  AS username,
			SUM(d.calls)                                     AS calls,
			SUM(d.total_exec_time)                           AS total_time,
			CASE WHEN SUM(d.calls) > 0
			     THEN SUM(d.total_exec_time) / SUM(d.calls)
			     ELSE 0 END                                  AS mean_time,
			SUM(d.rows)                                      AS rows,
			SUM(d.shared_blks_hit)                           AS shared_blks_hit,
			SUM(d.shared_blks_read)                          AS shared_blks_read,
			SUM(d.temp_blks_written)                         AS temp_blks_written,
			COALESCE(MAX(q.query_text), '')                  AS query
		FROM pgss_delta_1m d
		LEFT JOIN pgss_query_dim q
			ON q.server_id = d.server_id AND q.query_id = d.query_id
		WHERE d.server_id = $1
		  AND d.capture_timestamp BETWEEN $2 AND $3
		GROUP BY d.query_id
		ORDER BY total_time DESC
		LIMIT 50`

	rows, err := pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var queryID int64
		var username, query string
		var calls, rowCount, sharedHit, sharedRead, tempWrite int64
		var totalTime, meanTime float64
		if err := rows.Scan(&queryID, &username, &calls, &totalTime, &meanTime, &rowCount, &sharedHit, &sharedRead, &tempWrite, &query); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"query_id":          queryID,
			"username":          username,
			"calls":             calls,
			"total_time":        totalTime,
			"mean_time":         meanTime,
			"rows":              rowCount,
			"shared_blks_hit":   sharedHit,
			"shared_blks_read":  sharedRead,
			"temp_blks_written": tempWrite,
			"query":             query,
		})
	}
	return results, rows.Err()
}

func querySettingsDrift(ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID) ([]map[string]interface{}, error) {
	// Find settings whose value changed between consecutive snapshots in the last 7 days.
	const q = `
		WITH windowed AS (
			SELECT name, setting,
			       lag(setting) OVER (PARTITION BY name ORDER BY capture_timestamp) AS prev_setting
			FROM postgres_settings_snapshot
			WHERE server_id = $1
			  AND capture_timestamp > now() - INTERVAL '7 days'
		),
		changed AS (
			SELECT DISTINCT ON (name) name, prev_setting AS old_value, setting AS new_value
			FROM windowed
			WHERE prev_setting IS NOT NULL
			  AND setting IS DISTINCT FROM prev_setting
			ORDER BY name
		)
		SELECT name, old_value, new_value
		FROM changed
		ORDER BY name
		LIMIT 50`

	rows, err := pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name, oldVal, newVal string
		if err := rows.Scan(&name, &oldVal, &newVal); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"name":      name,
			"old_value": oldVal,
			"new_value": newVal,
		})
	}
	return results, nil
}
