// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL metrics logger for TimescaleDB including throughput and connections.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func pgFnv64(parts ...any) uint64 {
	h := fnv.New64a()
	for i, p := range parts {
		if i > 0 {
			_, _ = h.Write([]byte("|"))
		}
		_, _ = fmt.Fprintf(h, "%v", p)
	}
	return h.Sum64()
}

func (tl *TimescaleLogger) GetPostgresThroughput(ctx context.Context, instanceName string, limit int) ([]PostgresThroughputRow, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT capture_timestamp, server_instance_name, database_name, tps, cache_hit_pct,
		       txn_delta, blks_read_delta, blks_hit_delta
		FROM postgres_throughput_metrics
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		log.Printf("[TSLogger] Failed to query Postgres throughput: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []PostgresThroughputRow
	for rows.Next() {
		var r PostgresThroughputRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerName, &r.DatabaseName, &r.Tps,
			&r.CacheHitPct, &r.TxnDelta, &r.BlksReadDelta, &r.BlksHitDelta); err != nil {
			log.Printf("[TSLogger] Failed to scan row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetPostgresThroughputTimeRange(ctx context.Context, instanceName string, start, end time.Time) ([]PostgresThroughputRow, error) {
	query := `
		SELECT capture_timestamp, server_instance_name, database_name, tps, cache_hit_pct,
		       txn_delta, blks_read_delta, blks_hit_delta
		FROM postgres_throughput_metrics
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, start, end)
	if err != nil {
		log.Printf("[TSLogger] Failed to query Postgres throughput: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []PostgresThroughputRow
	for rows.Next() {
		var r PostgresThroughputRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerName, &r.DatabaseName, &r.Tps,
			&r.CacheHitPct, &r.TxnDelta, &r.BlksReadDelta, &r.BlksHitDelta); err != nil {
			log.Printf("[TSLogger] Failed to scan row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetPostgresConnections(ctx context.Context, instanceName string, limit int) ([]PostgresConnectionRow, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT capture_timestamp, server_instance_name, total_connections, active_connections, idle_connections
		FROM postgres_connection_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		log.Printf("[TSLogger] Failed to query Postgres connections: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []PostgresConnectionRow
	for rows.Next() {
		var r PostgresConnectionRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerName, &r.TotalConnections, &r.ActiveConnections, &r.IdleConnections); err != nil {
			log.Printf("[TSLogger] Failed to scan row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetPostgresSystemStats(ctx context.Context, instanceName string, limit int) ([]PostgresSystemStatsRow, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT capture_timestamp, server_instance_name, cpu_usage, memory_usage,
		       active_connections, idle_connections, total_connections,
		       COALESCE(host_cpu_percent, 0), COALESCE(postgres_cpu_percent, 0),
		       COALESCE(load_1m, 0), COALESCE(load_5m, 0), COALESCE(load_15m, 0), COALESCE(cpu_cores, 0)
		FROM postgres_system_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		log.Printf("[TSLogger] Failed to query Postgres system stats: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []PostgresSystemStatsRow
	for rows.Next() {
		var r PostgresSystemStatsRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerName, &r.CPUUsage, &r.MemoryUsage,
			&r.ActiveConnections, &r.IdleConnections, &r.TotalConnections,
			&r.HostCpuPercent, &r.PostgresCpuPercent, &r.Load1m, &r.Load5m, &r.Load15m, &r.CpuCores); err != nil {
			log.Printf("[TSLogger] Failed to scan row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) LogPostgresThroughput(ctx context.Context, instanceName string, databaseName string, tps, cacheHitPct float64, txnDelta, blksRead, blksHit int64) error {
	// Append-only: no UNIQUE matching this ON CONFLICT on default schema (42P10).
	query := `INSERT INTO postgres_throughput_metrics (capture_timestamp, server_instance_name, database_name, tps, cache_hit_pct, txn_delta, blks_read_delta, blks_hit_delta)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName, databaseName, tps, cacheHitPct, txnDelta, blksRead, blksHit)
	return err
}

func (tl *TimescaleLogger) LogPostgresConnectionStats(ctx context.Context, instanceName string, total, active, idle int) error {
	sig := pgFnv64(instanceName, total, active, idle)
	tl.mu.Lock()
	if prev, ok := tl.prevPgConnectionStatsHash[instanceName]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgConnectionStatsHash[instanceName] = sig
	tl.mu.Unlock()

	query := `INSERT INTO postgres_connection_stats (capture_timestamp, server_instance_name, total_connections, active_connections, idle_connections)
		VALUES ($1, $2, $3, $4, $5)`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName, total, active, idle)
	return err
}

func (tl *TimescaleLogger) LogPostgresReplicationStats(ctx context.Context, instanceName string, data map[string]interface{}) error {
	query := `INSERT INTO postgres_replication_stats (
		capture_timestamp, server_instance_name,
		is_primary, cluster_state, max_lag_mb,
		wal_gen_rate_mbps, bgwriter_eff_pct
	) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName,
		getBool(data, "is_primary"),
		getStr(data, "cluster_state"),
		getFloat64(data, "max_lag_mb"),
		getFloat64(data, "wal_gen_rate_mbps"),
		getFloat64(data, "bgwriter_eff_pct"),
	)
	return err
}

func (tl *TimescaleLogger) LogPostgresSystemStats(ctx context.Context, instanceName string, row PgSystemStatsInsert) error {
	sig := pgFnv64(instanceName, row.CPUUsage, row.MemoryUsage, row.ActiveConnections, row.IdleConnections, row.TotalConnections,
		row.HostCpuPercent, row.PostgresCpuPercent, row.Load1m, row.Load5m, row.Load15m, row.CpuCores)
	tl.mu.Lock()
	if prev, ok := tl.prevPgSystemStatsHash[instanceName]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgSystemStatsHash[instanceName] = sig
	tl.mu.Unlock()

	query := `INSERT INTO postgres_system_stats (
			capture_timestamp, server_instance_name, cpu_usage, memory_usage,
			active_connections, idle_connections, total_connections,
			host_cpu_percent, postgres_cpu_percent, load_1m, load_5m, load_15m, cpu_cores
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName,
		row.CPUUsage, row.MemoryUsage, row.ActiveConnections, row.IdleConnections, row.TotalConnections,
		row.HostCpuPercent, row.PostgresCpuPercent, row.Load1m, row.Load5m, row.Load15m, row.CpuCores)
	return err
}

func (tl *TimescaleLogger) ensurePostgresReplicationSlotStatsSchema(ctx context.Context) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS postgres_replication_slot_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			slot_name TEXT NOT NULL,
			slot_type TEXT,
			active BOOLEAN DEFAULT false,
			temporary BOOLEAN DEFAULT false,
			retained_wal_mb DOUBLE PRECISION DEFAULT 0,
			restart_lsn TEXT,
			confirmed_flush_lsn TEXT,
			xmin_txid BIGINT,
			catalog_xmin_txid BIGINT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`SELECT create_hypertable('postgres_replication_slot_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE)`,
		`CREATE INDEX IF NOT EXISTS idx_pg_repl_slot_server_time ON postgres_replication_slot_stats (server_instance_name, capture_timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pg_repl_slot_server_slot_time ON postgres_replication_slot_stats (server_instance_name, slot_name, capture_timestamp DESC)`,
	}
	for _, s := range stmts {
		if _, err := tl.pool.Exec(ctx, s); err != nil {
			// Best-effort only; main migrations should create these.
			continue
		}
	}
}

func (tl *TimescaleLogger) logPostgresReplicationSlots(ctx context.Context, instanceName string, rows []PostgresReplicationSlotRow, retried bool) error {
	// We snapshot the slot set. If identical to last snapshot for the instance, skip insert.
	// Sort for stable hashing (avoid duplicates from row order changes).
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].SlotName < rows[j].SlotName
	})
	sig := pgFnv64(instanceName, len(rows))
	for _, r := range rows {
		sig = pgFnv64(sig,
			r.SlotName, r.SlotType, r.Active, r.Temporary,
			fmt.Sprintf("%.3f", r.RetainedWalMB),
			r.RestartLSN, r.ConfirmedFlushLSN,
			func() any { if r.Xmin == nil { return "" }; return *r.Xmin }(),
			func() any { if r.CatalogXmin == nil { return "" }; return *r.CatalogXmin }(),
		)
	}
	tl.mu.Lock()
	if prev, ok := tl.prevPgReplicationSlotsHash[instanceName]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgReplicationSlotsHash[instanceName] = sig
	tl.mu.Unlock()

	if len(rows) == 0 {
		return nil
	}

	q := `
		INSERT INTO postgres_replication_slot_stats (
			capture_timestamp, server_instance_name,
			slot_name, slot_type, active, temporary,
			retained_wal_mb, restart_lsn, confirmed_flush_lsn,
			xmin_txid, catalog_xmin_txid
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q,
			now, instanceName,
			r.SlotName, r.SlotType, r.Active, r.Temporary,
			r.RetainedWalMB, r.RestartLSN, r.ConfirmedFlushLSN,
			r.Xmin, r.CatalogXmin,
		)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			var pgErr *pgconn.PgError
			if !retried && errors.As(err, &pgErr) && pgErr.Code == "42P01" {
				// Schema missing (e.g., Timescale migrations not applied). Try to self-heal once.
				tl.ensurePostgresReplicationSlotStatsSchema(ctx)
				return tl.logPostgresReplicationSlots(ctx, instanceName, rows, true)
			}
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogPostgresReplicationSlots(ctx context.Context, instanceName string, rows []PostgresReplicationSlotRow) error {
	return tl.logPostgresReplicationSlots(ctx, instanceName, rows, false)
}

func (tl *TimescaleLogger) GetPostgresReplicationSlots(ctx context.Context, instanceName string, limit int) ([]PostgresReplicationSlotRow, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `
		WITH latest AS (
			SELECT DISTINCT ON (slot_name)
			       capture_timestamp, server_instance_name,
			       slot_name, COALESCE(slot_type,''), active, temporary,
			       retained_wal_mb, COALESCE(restart_lsn,''), COALESCE(confirmed_flush_lsn,''),
			       xmin_txid, catalog_xmin_txid
			FROM postgres_replication_slot_stats
			WHERE server_instance_name = $1
			ORDER BY slot_name, capture_timestamp DESC
		)
		SELECT *
		FROM latest
		ORDER BY retained_wal_mb DESC, slot_name
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresReplicationSlotRow
	for rows.Next() {
		var r PostgresReplicationSlotRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerInstanceName,
			&r.SlotName, &r.SlotType, &r.Active, &r.Temporary,
			&r.RetainedWalMB, &r.RestartLSN, &r.ConfirmedFlushLSN,
			&r.Xmin, &r.CatalogXmin,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogPostgresBGWriter(ctx context.Context, instanceName string, stats PostgresBGWriterRow) error {
	query := `INSERT INTO postgres_bgwriter_stats (
		capture_timestamp, server_instance_name,
		checkpoints_timed, checkpoints_req, checkpoint_write_time, checkpoint_sync_time,
		buffers_checkpoint, buffers_clean, maxwritten_clean, buffers_backend, buffers_alloc
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := tl.pool.Exec(ctx, query,
		stats.CaptureTimestamp, stats.ServerInstanceName,
		stats.CheckpointsTimed, stats.CheckpointsReq, stats.CheckpointWriteTime, stats.CheckpointSyncTime,
		stats.BuffersCheckpoint, stats.BuffersClean, stats.MaxwrittenClean, stats.BuffersBackend, stats.BuffersAlloc,
	)
	return err
}

func (tl *TimescaleLogger) LogPostgresArchiver(ctx context.Context, instanceName string, stats PostgresArchiverRow) error {
	query := `INSERT INTO postgres_archiver_stats (
		capture_timestamp, server_instance_name,
		archived_count, failed_count, last_archived_wal, last_failed_wal
	) VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := tl.pool.Exec(ctx, query,
		stats.CaptureTimestamp, stats.ServerInstanceName,
		stats.ArchivedCount, stats.FailedCount, stats.LastArchivedWal, stats.LastFailedWal,
	)
	return err
}

func (tl *TimescaleLogger) UpsertPostgresQueryDictionary(ctx context.Context, entry PostgresQueryDictionaryRow) error {
	// NOTE: postgres_query_dictionary is NOT a time-series table; it stores one row per (instance, query_id)
	// and keeps first/last seen timestamps plus a cumulative execution_count.
	query := `
		INSERT INTO postgres_query_dictionary (
			server_instance_name, query_id, query_text, first_seen, last_seen, execution_count
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (server_instance_name, query_id) DO UPDATE SET
			query_text = COALESCE(NULLIF(EXCLUDED.query_text, ''), postgres_query_dictionary.query_text),
			first_seen = LEAST(postgres_query_dictionary.first_seen, EXCLUDED.first_seen),
			last_seen = GREATEST(postgres_query_dictionary.last_seen, EXCLUDED.last_seen),
			execution_count = postgres_query_dictionary.execution_count + EXCLUDED.execution_count
	`

	_, err := tl.pool.Exec(ctx, query,
		entry.ServerInstanceName,
		entry.QueryID,
		entry.QueryText,
		entry.FirstSeen,
		entry.LastSeen,
		entry.ExecutionCount,
	)
	return err
}

func (tl *TimescaleLogger) GetPostgresCheckpointSummary(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}

	// Dedupe any duplicate inserts for the same capture_timestamp.
	query := `
		SELECT DISTINCT ON (capture_timestamp)
		       capture_timestamp,
		       checkpoints_timed, checkpoints_req, 
		       checkpoint_write_time, checkpoint_sync_time,
		       buffers_checkpoint, buffers_clean, maxwritten_clean,
		       buffers_backend, buffers_alloc
		FROM postgres_bgwriter_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var checkpointsTimed, checkpointsReq int
		var checkpointWriteTime, checkpointSyncTime float64
		var buffersCheckpoint, buffersClean, maxwrittenClean, buffersBackend, buffersAlloc int

		if err := rows.Scan(&ts, &checkpointsTimed, &checkpointsReq,
			&checkpointWriteTime, &checkpointSyncTime,
			&buffersCheckpoint, &buffersClean, &maxwrittenClean,
			&buffersBackend, &buffersAlloc); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp":             ts,
			"checkpoints_timed":     checkpointsTimed,
			"checkpoints_req":       checkpointsReq,
			"checkpoint_write_time": checkpointWriteTime,
			"checkpoint_sync_time":  checkpointSyncTime,
			"buffers_checkpoint":    buffersCheckpoint,
			"buffers_clean":         buffersClean,
			"maxwritten_clean":      maxwrittenClean,
			"buffers_backend":       buffersBackend,
			"buffers_alloc":         buffersAlloc,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetPostgresArchiveSummary(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT capture_timestamp,
		       archived_count, failed_count, 
		       last_archived_wal, last_failed_wal
		FROM postgres_archiver_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var archivedCount, failedCount int
		var lastArchivedWal, lastFailedWal string

		if err := rows.Scan(&ts, &archivedCount, &failedCount, &lastArchivedWal, &lastFailedWal); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp":         ts,
			"archived_count":    archivedCount,
			"failed_count":      failedCount,
			"last_archived_wal": lastArchivedWal,
			"last_failed_wal":   lastFailedWal,
		})
	}
	return results, rows.Err()
}

// LogPostgresQueryStatsSnapshot writes a full snapshot of pg_stat_statements counters for delta windows.
func (tl *TimescaleLogger) LogPostgresQueryStatsSnapshot(ctx context.Context, instanceName string, captureTS time.Time, rows []PostgresQueryStatsSnapRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO postgres_query_stats (
		capture_timestamp, server_instance_name, query_id, query_text,
		calls, total_time_ms, mean_time_ms, rows,
		temp_blks_read, temp_blks_written, blk_read_time_ms, blk_write_time_ms
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			captureTS, instanceName, r.QueryID, r.QueryText,
			r.Calls, r.TotalTimeMs, r.MeanTimeMs, r.Rows,
			r.TempBlksRead, r.TempBlksWritten, r.BlkReadTimeMs, r.BlkWriteTimeMs,
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

func (tl *TimescaleLogger) loadPostgresQueryStatsSnapshot(ctx context.Context, instanceName string, ts time.Time) (map[int64]PostgresQueryStatsSnapRow, error) {
	const q = `
		SELECT query_id, query_text, calls, total_time_ms, mean_time_ms, rows,
		       temp_blks_read, temp_blks_written, blk_read_time_ms, blk_write_time_ms
		FROM postgres_query_stats
		WHERE server_instance_name = $1 AND capture_timestamp = $2`
	rows, err := tl.pool.Query(ctx, q, instanceName, ts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]PostgresQueryStatsSnapRow)
	for rows.Next() {
		var r PostgresQueryStatsSnapRow
		if err := rows.Scan(&r.QueryID, &r.QueryText, &r.Calls, &r.TotalTimeMs, &r.MeanTimeMs, &r.Rows,
			&r.TempBlksRead, &r.TempBlksWritten, &r.BlkReadTimeMs, &r.BlkWriteTimeMs); err != nil {
			continue
		}
		out[r.QueryID] = r
	}
	return out, rows.Err()
}

func subSnap(a, b PostgresQueryStatsSnapRow) PostgresQueryStatsSnapRow {
	// b - a with reset detection (counters went backwards)
	if b.Calls < a.Calls || b.TotalTimeMs+1e-6 < a.TotalTimeMs {
		return b
	}
	r := PostgresQueryStatsSnapRow{
		QueryID:         b.QueryID,
		QueryText:       b.QueryText,
		Calls:           b.Calls - a.Calls,
		TotalTimeMs:     b.TotalTimeMs - a.TotalTimeMs,
		Rows:            b.Rows - a.Rows,
		TempBlksRead:    b.TempBlksRead - a.TempBlksRead,
		TempBlksWritten: b.TempBlksWritten - a.TempBlksWritten,
		BlkReadTimeMs:   b.BlkReadTimeMs - a.BlkReadTimeMs,
		BlkWriteTimeMs:  b.BlkWriteTimeMs - a.BlkWriteTimeMs,
	}
	if r.Calls < 0 {
		r.Calls = 0
	}
	if r.TotalTimeMs < 0 {
		r.TotalTimeMs = 0
	}
	if r.Rows < 0 {
		r.Rows = 0
	}
	if r.TempBlksRead < 0 {
		r.TempBlksRead = 0
	}
	if r.TempBlksWritten < 0 {
		r.TempBlksWritten = 0
	}
	if r.BlkReadTimeMs < 0 {
		r.BlkReadTimeMs = 0
	}
	if r.BlkWriteTimeMs < 0 {
		r.BlkWriteTimeMs = 0
	}
	if r.Calls > 0 {
		r.MeanTimeMs = r.TotalTimeMs / float64(r.Calls)
	}
	return r
}

// GetPostgresQueryStatsWindowDelta compares two snapshots and returns per-query deltas for the window [from, to].
func (tl *TimescaleLogger) GetPostgresQueryStatsWindowDelta(ctx context.Context, instanceName string, from, to time.Time, topN int) ([]PostgresQueryStatsDelta, time.Time, time.Time, string, error) {
	if topN <= 0 {
		topN = 50
	}
	if from.After(to) {
		from, to = to, from
	}

	var endTS sql.NullTime
	err := tl.pool.QueryRow(ctx,
		`SELECT MAX(capture_timestamp) FROM postgres_query_stats WHERE server_instance_name = $1 AND capture_timestamp <= $2`,
		instanceName, to,
	).Scan(&endTS)
	if err != nil || !endTS.Valid {
		return nil, time.Time{}, time.Time{}, "", fmt.Errorf("no postgres_query_stats snapshots for instance (need Timescale + enterprise collector)")
	}

	var startTS sql.NullTime
	_ = tl.pool.QueryRow(ctx,
		`SELECT MAX(capture_timestamp) FROM postgres_query_stats WHERE server_instance_name = $1 AND capture_timestamp <= $2`,
		instanceName, from,
	).Scan(&startTS)

	note := ""
	startT := time.Time{}
	if startTS.Valid && startTS.Time.Before(endTS.Time) {
		startT = startTS.Time
	} else {
		_ = tl.pool.QueryRow(ctx,
			`SELECT MIN(capture_timestamp) FROM postgres_query_stats WHERE server_instance_name = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3`,
			instanceName, from, endTS.Time,
		).Scan(&startTS)
		if startTS.Valid && startTS.Time.Before(endTS.Time) {
			startT = startTS.Time
			note = "baseline_first_snapshot_in_window"
		} else {
			var prev sql.NullTime
			_ = tl.pool.QueryRow(ctx,
				`SELECT MAX(capture_timestamp) FROM postgres_query_stats WHERE server_instance_name = $1 AND capture_timestamp < $2`,
				instanceName, endTS.Time,
			).Scan(&prev)
			if prev.Valid && prev.Time.Before(endTS.Time) {
				startT = prev.Time
				note = "using_consecutive_snapshots"
			} else {
				return nil, time.Time{}, endTS.Time, "", fmt.Errorf("insufficient postgres_query_stats history")
			}
		}
	}

	startMap, err := tl.loadPostgresQueryStatsSnapshot(ctx, instanceName, startT)
	if err != nil {
		return nil, time.Time{}, endTS.Time, note, err
	}
	endMap, err := tl.loadPostgresQueryStatsSnapshot(ctx, instanceName, endTS.Time)
	if err != nil {
		return nil, startT, endTS.Time, note, err
	}

	var deltas []PostgresQueryStatsDelta
	for qid, e := range endMap {
		base, ok := startMap[qid]
		if !ok {
			base = PostgresQueryStatsSnapRow{QueryID: qid}
		}
		d := subSnap(base, e)
		if d.Calls <= 0 && d.TotalTimeMs <= 0 {
			continue
		}
		deltas = append(deltas, PostgresQueryStatsDelta{
			QueryID:         d.QueryID,
			QueryText:       d.QueryText,
			Calls:           d.Calls,
			TotalTimeMs:     d.TotalTimeMs,
			MeanTimeMs:      d.MeanTimeMs,
			Rows:            d.Rows,
			TempBlksRead:    d.TempBlksRead,
			TempBlksWritten: d.TempBlksWritten,
			BlkReadTimeMs:   d.BlkReadTimeMs,
			BlkWriteTimeMs:  d.BlkWriteTimeMs,
		})
	}

	sort.Slice(deltas, func(i, j int) bool {
		return deltas[i].TotalTimeMs > deltas[j].TotalTimeMs
	})
	if len(deltas) > topN {
		deltas = deltas[:topN]
	}
	return deltas, startT, endTS.Time, note, nil
}
