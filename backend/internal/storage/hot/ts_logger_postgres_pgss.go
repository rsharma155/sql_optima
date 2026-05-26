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
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// latencyEntry holds a single query's call count and mean latency for percentile computation.
type latencyEntry struct {
	calls  int64
	meanMs float64
}

// LogPostgresQueryStats logs raw query performance snapshots.
func (tl *TimescaleLogger) LogPostgresQueryStats(ctx context.Context, serverID uuid.UUID, rows []PostgresQueryStatsSnapRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO postgres_query_stats (
			capture_timestamp, server_id, query_id, db_name, username, query_type,
			calls, total_time_ms, mean_time_ms, rows, temp_blks_read, temp_blks_written,
			blk_read_time_ms, blk_write_time_ms, shared_blks_hit, shared_blks_read,
			shared_blks_dirtied, shared_blks_written, wal_bytes, wal_records, wal_fpi,
			total_plan_time, mean_plan_time, plans
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			now, serverID, r.QueryID, r.DbName, r.UserName, r.QueryType,
			r.Calls, r.TotalTimeMs, r.MeanTimeMs, r.Rows, r.TempBlksRead, r.TempBlksWritten,
			r.BlkReadTimeMs, r.BlkWriteTimeMs, r.SharedBlksHit, r.SharedBlksRead,
			r.SharedBlksDirtied, r.SharedBlksWritten, r.WalBytes, r.WalRecords, r.WalFpi,
			r.TotalPlanTime, r.MeanPlanTime, r.Plans,
		)
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

// LogPGTimescaleLock logs detailed lock events.
func (tl *TimescaleLogger) LogPGTimescaleLock(ctx context.Context, serverID uuid.UUID, rows []PostgresLockRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.pg_lock_snapshot (
			capture_timestamp, server_id, pid, locktype, mode, granted,
			relation_oid, relation_name, transactionid, waiting_seconds
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			r.CollectedAt, serverID, r.PID, r.LockType, r.Mode, r.Granted,
			r.RelationOID, r.RelationName, r.TransactionID, r.WaitingSeconds)
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

// UpsertPgssQueryDim upserts query dimension metadata into pgss_query_dim.
func (tl *TimescaleLogger) UpsertPgssQueryDim(ctx context.Context, serverID uuid.UUID, rows []PostgresQueryStatsSnapRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO pgss_query_dim
		    (server_id, query_id, query_text, db_name, username, app_name, query_type, first_seen, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (server_id, query_id) DO UPDATE SET
		    last_seen  = NOW(),
		    query_text = CASE WHEN EXCLUDED.query_text IS NOT NULL AND EXCLUDED.query_text != ''
		                      THEN EXCLUDED.query_text
		                      ELSE pgss_query_dim.query_text END,
		    db_name    = COALESCE(EXCLUDED.db_name,    pgss_query_dim.db_name),
		    username   = COALESCE(EXCLUDED.username,   pgss_query_dim.username),
		    app_name   = CASE WHEN EXCLUDED.app_name != '' THEN EXCLUDED.app_name ELSE pgss_query_dim.app_name END,
		    query_type = COALESCE(EXCLUDED.query_type, pgss_query_dim.query_type)`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, serverID, r.QueryID, r.QueryText, r.DbName, r.UserName, r.AppName, r.QueryType)
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

// LogPgssDelta1m logs pre-computed query deltas directly to pgss_delta_1m.
func (tl *TimescaleLogger) LogPgssDelta1m(ctx context.Context, serverID uuid.UUID, captureTS time.Time, rows []PostgresQueryStatsSnapRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO pgss_delta_1m (
		capture_timestamp, server_id, query_id,
		db_name, username, app_name, query_type,
		calls, total_exec_time, rows,
		shared_blks_hit, shared_blks_read, temp_blks_written,
		wal_bytes, total_plan_time, mean_exec_time
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`

	batch := &pgx.Batch{}
	for _, d := range rows {
		batch.Queue(q,
			captureTS, serverID, d.QueryID,
			d.DbName, d.UserName, d.AppName, d.QueryType,
			d.Calls, d.TotalTimeMs, d.Rows,
			d.SharedBlksHit, d.SharedBlksRead, d.TempBlksWritten,
			d.WalBytes, d.TotalPlanTime, d.MeanTimeMs,
		)
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

// ComputeAndStorePgssDelta1m computes per-query deltas between the latest two snapshots.
// The currentTS hint is unused — we always resolve the two most recent distinct
// capture_timestamps directly from the table, because LogPostgresQueryStats captures
// time.Now() independently and an exact-match lookup against a separately-captured
// time.Now() will always return an empty map.
func (tl *TimescaleLogger) ComputeAndStorePgssDelta1m(ctx context.Context, serverID uuid.UUID, _ time.Time) error {
	tsRows, err := tl.pool.Query(ctx,
		`SELECT DISTINCT capture_timestamp FROM postgres_query_stats
		 WHERE server_id = $1 ORDER BY capture_timestamp DESC LIMIT 2`,
		serverID)
	if err != nil {
		return err
	}
	defer tsRows.Close()
	var timestamps []time.Time
	for tsRows.Next() {
		var ts time.Time
		if err := tsRows.Scan(&ts); err != nil {
			continue
		}
		timestamps = append(timestamps, ts)
	}
	if len(timestamps) < 2 {
		slog.Debug("[PGSS] ComputeAndStorePgssDelta1m: fewer than 2 snapshots in postgres_query_stats", "server_id", serverID, "found", len(timestamps))
		return nil
	}
	currTS, prevTS := timestamps[0], timestamps[1]

	prevMap, err := tl.loadPostgresQueryStatsSnapshot(ctx, serverID, prevTS)
	if err != nil {
		return fmt.Errorf("loading previous snapshot: %w", err)
	}
	currMap, err := tl.loadPostgresQueryStatsSnapshot(ctx, serverID, currTS)
	if err != nil {
		return fmt.Errorf("loading current snapshot: %w", err)
	}
	slog.Debug("[PGSS] ComputeAndStorePgssDelta1m: loaded snapshots", "server_id", serverID, "prev_rows", len(prevMap), "curr_rows", len(currMap))

	const q = `INSERT INTO pgss_delta_1m (
		capture_timestamp, server_id, query_id,
		db_name, username, app_name, query_type,
		calls, total_exec_time, rows,
		shared_blks_hit, shared_blks_read, temp_blks_written,
		wal_bytes, total_plan_time, mean_exec_time
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`

	batch := &pgx.Batch{}
	count := 0
	for key, curr := range currMap {
		base, ok := prevMap[key]
		if !ok {
			continue
		}
		d := subSnap(base, curr)
		if d.Calls <= 0 && d.TotalTimeMs <= 0 {
			continue
		}
		batch.Queue(q,
			currTS, serverID, curr.QueryID,
			d.DbName, d.UserName, curr.AppName, d.QueryType,
			d.Calls, d.TotalTimeMs, d.Rows,
			d.SharedBlksHit, d.SharedBlksRead, d.TempBlksWritten,
			d.WalBytes, d.TotalPlanTime, d.MeanTimeMs,
		)
		count++
	}
	if count == 0 {
		slog.Debug("[PGSS] ComputeAndStorePgssDelta1m: no active deltas between snapshots", "server_id", serverID, "curr_ts", currTS, "prev_ts", prevTS)
		return nil
	}
	slog.Debug("[PGSS] ComputeAndStorePgssDelta1m: writing deltas", "server_id", serverID, "count", count)
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < count; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) loadPostgresQueryStatsSnapshot(ctx context.Context, serverID uuid.UUID, ts time.Time) (map[string]PostgresQueryStatsSnapRow, error) {
	// Cast wal_bytes to BIGINT in SQL so pgx receives it as int8 OID (int64-compatible).
	// The column is NUMERIC, and pgx v5 binary protocol cannot reliably scan NUMERIC→int64.
	// Join pgss_query_dim to carry app_name forward (populated by UpsertPgssQueryDim which runs first).
	rows, err := tl.pool.Query(ctx, `
		SELECT pqs.query_id, COALESCE(pqs.db_name,''), COALESCE(pqs.username,''), COALESCE(pqs.query_type,''),
		       pqs.calls, pqs.total_time_ms, pqs.rows, pqs.shared_blks_hit, pqs.shared_blks_read, pqs.temp_blks_written,
		       pqs.wal_bytes::bigint, pqs.total_plan_time, pqs.shared_blks_dirtied, pqs.shared_blks_written,
		       pqs.temp_blks_read, pqs.wal_records, pqs.wal_fpi, pqs.plans,
		       COALESCE(qd.app_name, '')
		FROM postgres_query_stats pqs
		LEFT JOIN pgss_query_dim qd ON qd.server_id = pqs.server_id AND qd.query_id = pqs.query_id
		WHERE pqs.server_id = $1 AND pqs.capture_timestamp = $2`, serverID, ts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]PostgresQueryStatsSnapRow)
	scanErrors := 0
	for rows.Next() {
		var r PostgresQueryStatsSnapRow
		if err := rows.Scan(
			&r.QueryID, &r.DbName, &r.UserName, &r.QueryType,
			&r.Calls, &r.TotalTimeMs, &r.Rows, &r.SharedBlksHit, &r.SharedBlksRead, &r.TempBlksWritten,
			&r.WalBytes, &r.TotalPlanTime, &r.SharedBlksDirtied, &r.SharedBlksWritten,
			&r.TempBlksRead, &r.WalRecords, &r.WalFpi, &r.Plans,
			&r.AppName,
		); err != nil {
			scanErrors++
			slog.Warn("[PGSS] snapshot scan error", "server_id", serverID, "ts", ts, "err", err)
			continue
		}
		// Use composite key because query_id is NOT unique per server (it's unique per db/user/query)
		key := fmt.Sprintf("%d|%s|%s", r.QueryID, r.DbName, r.UserName)
		out[key] = r
	}
	if scanErrors > 0 {
		slog.Warn("[PGSS] snapshot loaded with scan errors", "server_id", serverID, "loaded", len(out), "scan_errors", scanErrors)
	}
	return out, nil
}

func subSnap(base, curr PostgresQueryStatsSnapRow) PostgresQueryStatsSnapRow {
	d := curr
	d.Calls = curr.Calls - base.Calls
	d.TotalTimeMs = curr.TotalTimeMs - base.TotalTimeMs
	d.Rows = curr.Rows - base.Rows
	d.SharedBlksHit = curr.SharedBlksHit - base.SharedBlksHit
	d.SharedBlksRead = curr.SharedBlksRead - base.SharedBlksRead
	d.SharedBlksDirtied = curr.SharedBlksDirtied - base.SharedBlksDirtied
	d.SharedBlksWritten = curr.SharedBlksWritten - base.SharedBlksWritten
	d.TempBlksRead = curr.TempBlksRead - base.TempBlksRead
	d.TempBlksWritten = curr.TempBlksWritten - base.TempBlksWritten
	d.WalBytes = curr.WalBytes - base.WalBytes
	d.WalRecords = curr.WalRecords - base.WalRecords
	d.WalFpi = curr.WalFpi - base.WalFpi
	d.TotalPlanTime = curr.TotalPlanTime - base.TotalPlanTime
	d.Plans = curr.Plans - base.Plans

	// Clamp negatives (happens if stats are reset)
	if d.Calls < 0 { d.Calls = 0 }
	if d.TotalTimeMs < 0 { d.TotalTimeMs = 0 }
	if d.Rows < 0 { d.Rows = 0 }
	if d.SharedBlksHit < 0 { d.SharedBlksHit = 0 }
	if d.SharedBlksRead < 0 { d.SharedBlksRead = 0 }
	if d.SharedBlksDirtied < 0 { d.SharedBlksDirtied = 0 }
	if d.SharedBlksWritten < 0 { d.SharedBlksWritten = 0 }
	if d.TempBlksRead < 0 { d.TempBlksRead = 0 }
	if d.TempBlksWritten < 0 { d.TempBlksWritten = 0 }
	if d.WalBytes < 0 { d.WalBytes = 0 }
	if d.WalRecords < 0 { d.WalRecords = 0 }
	if d.WalFpi < 0 { d.WalFpi = 0 }
	if d.TotalPlanTime < 0 { d.TotalPlanTime = 0 }
	if d.Plans < 0 { d.Plans = 0 }

	if d.Calls > 0 {
		d.MeanTimeMs = d.TotalTimeMs / float64(d.Calls)
		d.MeanPlanTime = d.TotalPlanTime / float64(d.Calls)
	}
	return d
}

func (tl *TimescaleLogger) LogPgIndexDefinitions(ctx context.Context, serverID uuid.UUID, rows []IndexDefinitionCatalogRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO pg_index_definitions (
			server_id, database_name, schema_name, table_name, index_name,
			key_columns, include_columns, filter_definition, is_unique, is_pk, index_type, capture_timestamp
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (server_id, database_name, schema_name, index_name) DO UPDATE SET
			table_name = EXCLUDED.table_name,
			key_columns = EXCLUDED.key_columns,
			include_columns = EXCLUDED.include_columns,
			filter_definition = EXCLUDED.filter_definition,
			is_unique = EXCLUDED.is_unique,
			is_pk = EXCLUDED.is_pk,
			index_type = EXCLUDED.index_type,
			capture_timestamp = EXCLUDED.capture_timestamp`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName,
			r.KeyColumns, r.IncludeColumns, r.FilterDefinition.String, r.IsUnique, r.IsPK, r.IndexType)
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

type PgssWorkloadPoint struct {
	Timestamp          time.Time `json:"capture_timestamp"`
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

func (tl *TimescaleLogger) GetPgssWorkloadTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]PgssWorkloadPoint, error) {
	const q = `
		SELECT time_bucket('1 minute', capture_timestamp) AS bucket,
			SUM(total_exec_time),
			SUM(calls),
			SUM(rows),
			SUM(shared_blks_hit),
			SUM(shared_blks_read),
			COALESCE(SUM(wal_bytes), 0)::float8,
			SUM(total_plan_time),
			SUM(temp_blks_written)
		FROM pgss_delta_1m
		WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket`
	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
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

type PgssTopQuery struct {
	QueryID      int64     `json:"query_id"`
	Query        string    `json:"query"`
	DbName       string    `json:"db_name"`
	UserName     string    `json:"username"`
	AppName      string    `json:"app_name"`
	QueryType    string    `json:"query_type"`
	TotalTime    float64   `json:"total_time_ms"`
	PctDBTime    float64   `json:"pct_db_time"`
	Calls        int64     `json:"calls"`
	AvgMs        float64   `json:"avg_ms"`
	RowsPerCall  float64   `json:"rows_per_call"`
	HitPct       float64   `json:"hit_pct"`
	TempMB       float64   `json:"temp_mb"`
	WalMB        float64   `json:"wal_mb"`
	ReadsPerCall float64   `json:"reads_per_call"`
	PlanRatio    float64   `json:"plan_ratio"`
	Flags        []string  `json:"flags"`
	CapturedAt   time.Time `json:"captured_at"`
}

type PgssFilterOptions struct {
	Databases []string `json:"databases"`
	Users     []string `json:"users"`
	AppNames  []string `json:"app_names"`
}

type PgssDbBreakdown struct {
	DbName         string  `json:"db_name"`
	TotalExecMs    float64 `json:"total_exec_ms"`
	PctOfServer    float64 `json:"pct_of_server"`
	TotalCalls     int64   `json:"total_calls"`
	AvgMs          float64 `json:"avg_ms"`
	CacheHitPct    float64 `json:"cache_hit_pct"`
	UniqueQueryIDs int64   `json:"unique_query_ids"`
}

type PgssUserBreakdown struct {
	UserName       string  `json:"username"`
	TotalExecMs    float64 `json:"total_exec_ms"`
	PctOfServer    float64 `json:"pct_of_server"`
	TotalCalls     int64   `json:"total_calls"`
	AvgMs          float64 `json:"avg_ms"`
	UniqueQueryIDs int64   `json:"unique_query_ids"`
}

func (tl *TimescaleLogger) GetPgssTopQueries(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
	sortBy string,
	limit int,
	dbName, userName, appName, queryType string,
	hideSystem bool,
	excludeUsers []string,
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

	args := []interface{}{serverID, from, to}
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
		// app_name filter uses the dim table (via outer WHERE) rather than the raw delta column
		filterClauses = append(filterClauses, fmt.Sprintf("COALESCE(d.app_name,'') = $%d", argIdx))
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

	excludeIdx := argIdx
	if len(excludeUsers) == 0 {
		excludeUsers = []string{"postgres"}
	}
	args = append(args, excludeUsers)
	argIdx++

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
				SUM(d.total_plan_time) AS total_plan,
				MAX(d.capture_timestamp) AS last_seen
			FROM pgss_delta_1m d
			WHERE d.server_id = $1
			  AND d.capture_timestamp >= $2 AND d.capture_timestamp <= $3
			  AND NOT (d.username = ANY($%d::text[]))
			  %s
			GROUP BY d.query_id, d.db_name, d.username, d.app_name, d.query_type
			HAVING SUM(d.calls) > 0
		),
		grand AS (
			SELECT SUM(total_exec_time) AS db_total FROM agg
		)
		SELECT a.query_id,
			COALESCE(q.query_text, ''),
			a.db_name, a.username, COALESCE(NULLIF(q.app_name,''), a.app_name, '') AS app_name, a.query_type,
			a.total_exec_time,
			CASE WHEN g.db_total > 0 THEN (a.total_exec_time / g.db_total) * 100.0 ELSE 0 END,
			a.total_calls,
			COALESCE(a.total_exec_time / NULLIF(a.total_calls, 0), 0) AS avg_ms,
			COALESCE(a.total_rows::float / NULLIF(a.total_calls, 0), 0),
			CASE WHEN a.total_hit + a.total_blks_read > 0
				THEN (a.total_hit::float / (a.total_hit + a.total_blks_read)) * 100.0
				ELSE 100.0 END,
			a.total_temp::float * 8.0 / 1024.0,
			a.total_wal::float / (1024.0 * 1024.0),
			COALESCE(a.total_blks_read::float / NULLIF(a.total_calls, 0), 0),
			COALESCE(a.total_plan / NULLIF(a.total_exec_time, 0), 0),
			a.last_seen
		FROM agg a
		CROSS JOIN grand g
		LEFT JOIN pgss_query_dim q ON q.server_id = $1 AND q.query_id = a.query_id
		WHERE COALESCE(q.query_text, '') NOT LIKE '%%%%/* SQL_OPTIMA%%%%'
		AND (NOT $%d OR (
			COALESCE(q.query_text, '') NOT ILIKE '%%%%pg_catalog%%%%'
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
		LIMIT $%d`, excludeIdx, extraWhere, hideSysIdx, orderCol, limitArg)

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
			&r.ReadsPerCall, &r.PlanRatio, &r.CapturedAt); err != nil {
			slog.Warn("pgss: top query scan error", "err", err)
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

func (tl *TimescaleLogger) GetPgssFilterOptions(ctx context.Context, serverID uuid.UUID, from, to time.Time) (*PgssFilterOptions, error) {
	if tl == nil || tl.pool == nil {
		return &PgssFilterOptions{}, nil
	}
	// app_name is sourced from pgss_query_dim (enriched at collection time) because
	// pgss_delta_1m.app_name may be empty for older rows.
	const q = `
		SELECT
			COALESCE(ARRAY_AGG(DISTINCT d.db_name  ORDER BY d.db_name)  FILTER (WHERE d.db_name  IS NOT NULL AND d.db_name  <> ''), '{}'),
			COALESCE(ARRAY_AGG(DISTINCT d.username ORDER BY d.username) FILTER (WHERE d.username IS NOT NULL AND d.username <> ''), '{}'),
			COALESCE(ARRAY_AGG(DISTINCT COALESCE(NULLIF(qd.app_name,''), d.app_name)) FILTER (WHERE COALESCE(qd.app_name, d.app_name, '') <> ''), '{}')
		FROM pgss_delta_1m d
		LEFT JOIN pgss_query_dim qd ON qd.server_id = d.server_id AND qd.query_id = d.query_id
		WHERE d.server_id = $1
		  AND d.capture_timestamp >= $2 AND d.capture_timestamp <= $3`
	var dbs, users, apps []string
	err := tl.pool.QueryRow(ctx, q, serverID, from, to).Scan(&dbs, &users, &apps)
	if err != nil {
		slog.Error("[PGSS] GetPgssFilterOptions scan failed", "server_id", serverID, "err", err)
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

// GetPgssHasData returns true if pgss_delta_1m has any rows for the given server
// within the last 24 hours, indicating the collector has run at least twice.
func (tl *TimescaleLogger) GetPgssHasData(ctx context.Context, serverID uuid.UUID) bool {
	if tl == nil || tl.pool == nil {
		return false
	}
	var n int
	err := tl.pool.QueryRow(ctx,
		`SELECT 1 FROM pgss_delta_1m WHERE server_id = $1 AND capture_timestamp >= NOW() - INTERVAL '24 hours' LIMIT 1`,
		serverID,
	).Scan(&n)
	return err == nil
}

// GetPgssRawSnapshotCount returns the number of raw snapshot entries in postgres_query_stats
// within the last 24 hours. Used to distinguish "collector not running" from "delta computation empty".
func (tl *TimescaleLogger) GetPgssRawSnapshotCount(ctx context.Context, serverID uuid.UUID) int {
	if tl == nil || tl.pool == nil {
		return 0
	}
	var n int
	err := tl.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT capture_timestamp) FROM postgres_query_stats WHERE server_id = $1 AND capture_timestamp >= NOW() - INTERVAL '24 hours'`,
		serverID,
	).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func (tl *TimescaleLogger) GetPgssDbBreakdown(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]PgssDbBreakdown, error) {
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
			WHERE server_id = $1
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

	pgRows, err := tl.pool.Query(ctx, q, serverID, from, to)
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

func (tl *TimescaleLogger) GetPgssUserBreakdown(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]PgssUserBreakdown, error) {
	const q = `
		WITH agg AS (
			SELECT
				COALESCE(username, 'unknown')                                  AS username,
				SUM(total_exec_time)                                           AS total_exec_ms,
				SUM(calls)                                                     AS total_calls,
				COUNT(DISTINCT query_id)                                       AS unique_queries
			FROM pgss_delta_1m
			WHERE server_id = $1
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

	pgRows, err := tl.pool.Query(ctx, q, serverID, from, to)
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

type PgssLatencyPoint struct {
	Timestamp time.Time `json:"capture_timestamp"`
	P50       float64   `json:"p50"`
	P95       float64   `json:"p95"`
	P99       float64   `json:"p99"`
	MaxExec   float64   `json:"max_exec"`
}

func (tl *TimescaleLogger) GetPgssLatencyTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]PgssLatencyPoint, error) {
	const q = `
		SELECT capture_timestamp, query_id, calls, mean_exec_time
		FROM pgss_delta_1m
		WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
			AND calls > 0
		ORDER BY capture_timestamp, mean_exec_time`

	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

	keys := make([]time.Time, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	var points []PgssLatencyPoint
	for _, ts := range keys {
		entries := buckets[ts]
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

type PgssRegression struct {
	QueryID    int64     `json:"query_id"`
	Query      string    `json:"query"`
	PrevAvgMs  float64   `json:"prev_avg_ms"`
	CurrAvgMs  float64   `json:"curr_avg_ms"`
	ChangePct  float64   `json:"change_pct"`
	PrevCalls  int64     `json:"prev_calls"`
	CurrCalls  int64     `json:"curr_calls"`
	Status     string    `json:"status"`
	DetectedAt time.Time `json:"detected_at"`
}

func (tl *TimescaleLogger) GetPgssRegressions(ctx context.Context, serverID uuid.UUID) ([]PgssRegression, error) {
	now := time.Now().UTC()
	mid := now.Add(-30 * time.Minute)
	start := now.Add(-60 * time.Minute)

	const q = `
		WITH prev AS (
			SELECT query_id,
				SUM(total_exec_time) / NULLIF(SUM(calls), 0) AS avg_ms,
				SUM(calls) AS total_calls
			FROM pgss_delta_1m
			WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp < $3
			GROUP BY query_id
			HAVING SUM(calls) > 0
		),
		curr AS (
			SELECT query_id,
				SUM(total_exec_time) / NULLIF(SUM(calls), 0) AS avg_ms,
				SUM(calls) AS total_calls
			FROM pgss_delta_1m
			WHERE server_id = $1 AND capture_timestamp >= $3 AND capture_timestamp <= $4
			GROUP BY query_id
			HAVING SUM(calls) > 0
		)
		SELECT c.query_id,
			COALESCE(q.query_text, ''),
			p.avg_ms,
			c.avg_ms,
			((c.avg_ms - p.avg_ms) / NULLIF(p.avg_ms, 0)) * 100.0,
			p.total_calls,
			c.total_calls
		FROM curr c
		JOIN prev p ON p.query_id = c.query_id
		LEFT JOIN pgss_query_dim q ON q.server_id = $1 AND q.query_id = c.query_id
		WHERE p.avg_ms >= 1.0
		  AND p.total_calls >= 2 AND c.total_calls >= 2
		  AND (c.avg_ms - p.avg_ms) >= 5.0
		  AND ((c.avg_ms - p.avg_ms) / p.avg_ms) > 0.5
		  AND COALESCE(q.query_text, '') NOT LIKE '%%/* SQL_OPTIMA */%%'
		ORDER BY ((c.avg_ms - p.avg_ms) / NULLIF(p.avg_ms, 0)) DESC
		LIMIT 20`

	rows, err := tl.pool.Query(ctx, q, serverID, start, mid, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PgssRegression
	for rows.Next() {
		var r PgssRegression
		if err := rows.Scan(&r.QueryID, &r.Query, &r.PrevAvgMs, &r.CurrAvgMs, &r.ChangePct, &r.PrevCalls, &r.CurrCalls); err != nil {
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

func (tl *TimescaleLogger) GetPgssSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time) (*PgssSummary, error) {
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
			COALESCE(SUM(wal_bytes), 0)::float8,
			COUNT(DISTINCT query_id)
		FROM pgss_delta_1m
		WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3`

	var totalExec, totalCalls, blksHit, blksRead, tempBlks, walBytes float64
	var uniqueQueries int64
	if err := tl.pool.QueryRow(ctx, q, serverID, from, to).Scan(
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

	const p99q = `
		SELECT calls, mean_exec_time
		FROM pgss_delta_1m
		WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
			AND calls > 0
		ORDER BY mean_exec_time`
	rows, err := tl.pool.Query(ctx, p99q, serverID, from, to)
	if err != nil {
		return s, nil
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
