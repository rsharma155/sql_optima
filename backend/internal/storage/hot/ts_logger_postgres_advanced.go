// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Advanced PostgreSQL metrics logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresWaitEventRow struct {
	Timestamp     time.Time `json:"timestamp"`
	ServerID      uuid.UUID `json:"server_id"`
	WaitEventType string    `json:"wait_event_type"`
	WaitEvent     string    `json:"wait_event"`
	SessionsCount int       `json:"sessions_count"`
}

type PostgresDbIORow struct {
	Timestamp    time.Time `json:"timestamp"`
	ServerID     uuid.UUID `json:"server_id"`
	DatabaseName string    `json:"database_name"`
	BlksRead     int64     `json:"blks_read"`
	BlksHit      int64     `json:"blks_hit"`
	TempFiles    int64     `json:"temp_files"`
	TempBytes    int64     `json:"temp_bytes"`
}

type PostgresSettingSnapshotRow struct {
	Timestamp time.Time `json:"timestamp"`
	ServerID  uuid.UUID `json:"server_id"`
	Name      string    `json:"name"`
	Setting   string    `json:"setting"`
	Unit      string    `json:"unit"`
	Source    string    `json:"source"`
}

func (tl *TimescaleLogger) LogPostgresWaitEvents(ctx context.Context, serverID uuid.UUID, rows []PostgresWaitEventRow) error {
	// Stable signature for dedup.
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.WaitEventType+"|"+r.WaitEvent+"|"+itoa(r.SessionsCount))
	}
	sort.Strings(keys)
	sig := pgFnv64(serverID, keys)

	tl.mu.Lock()
	if prev := tl.prevPgWaitEventsHash[serverID]; prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgWaitEventsHash[serverID] = sig
	tl.mu.Unlock()

	const q = `INSERT INTO postgres_wait_event_stats (
		capture_timestamp, server_id, wait_event_type, wait_event, sessions_count
	) VALUES ($1,$2,$3,$4,$5)`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.Timestamp, serverID, r.WaitEventType, r.WaitEvent, r.SessionsCount)
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

func (tl *TimescaleLogger) GetPostgresWaitEventsHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]PostgresWaitEventRow, error) {
	if limit <= 0 {
		limit = 180
	}
	var q string
	var args []interface{}
	args = append(args, serverID)

	if !from.IsZero() && !to.IsZero() {
		q = `
			SELECT capture_timestamp, server_id, COALESCE(wait_event_type,''), COALESCE(wait_event,''), sessions_count
			FROM postgres_wait_event_stats
			WHERE server_id = $1
			  AND capture_timestamp >= $2 AND capture_timestamp <= $3
			ORDER BY capture_timestamp DESC
			LIMIT 2000
		`
		args = append(args, from, to)
	} else {
		q = `
			SELECT capture_timestamp, server_id, COALESCE(wait_event_type,''), COALESCE(wait_event,''), sessions_count
			FROM postgres_wait_event_stats
			WHERE server_id = $1
			ORDER BY capture_timestamp DESC
			LIMIT $2
		`
		args = append(args, limit)
	}

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostgresWaitEventRow
	for rows.Next() {
		var r PostgresWaitEventRow
		if err := rows.Scan(&r.Timestamp, &r.ServerID, &r.WaitEventType, &r.WaitEvent, &r.SessionsCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPostgresLockWaitHistory returns a time series of session counts waiting with wait_event_type = 'Lock'
// (aggregated across specific lock wait_event values per collector snapshot).
func (tl *TimescaleLogger) GetPostgresLockWaitHistory(ctx context.Context, serverID uuid.UUID, windowMinutes, maxPoints int) ([]string, []int, error) {
	if maxPoints <= 0 {
		maxPoints = 500
	}
	if windowMinutes <= 0 {
		windowMinutes = 180
	}
	from := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute)
	q := `
		SELECT capture_timestamp, COALESCE(SUM(sessions_count), 0)::int
		FROM postgres_wait_event_stats
		WHERE server_id = $1
		  AND wait_event_type = 'Lock'
		  AND capture_timestamp >= $2
		GROUP BY capture_timestamp
		ORDER BY capture_timestamp ASC
		LIMIT $3
	`
	rows, err := tl.pool.Query(ctx, q, serverID, from, maxPoints)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var labels []string
	var counts []int
	for rows.Next() {
		var ts time.Time
		var n int
		if err := rows.Scan(&ts, &n); err != nil {
			continue
		}
		labels = append(labels, ts.UTC().Format(time.RFC3339))
		counts = append(counts, n)
	}
	return labels, counts, rows.Err()
}

func (tl *TimescaleLogger) LogPostgresDbIOStats(ctx context.Context, serverID uuid.UUID, rows []PostgresDbIORow) error {
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.DatabaseName+"|"+itoa64(r.BlksRead)+"|"+itoa64(r.BlksHit)+"|"+itoa64(r.TempFiles)+"|"+itoa64(r.TempBytes))
	}
	sort.Strings(keys)
	sig := pgFnv64(serverID, keys)

	tl.mu.Lock()
	if prev := tl.prevPgDbIOHash[serverID]; prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgDbIOHash[serverID] = sig
	tl.mu.Unlock()

	const q = `INSERT INTO postgres_db_io_stats (
		capture_timestamp, server_id, database_name, blks_read, blks_hit, temp_files, temp_bytes
	) VALUES ($1,$2,$3,$4,$5,$6,$7)`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.Timestamp, serverID, r.DatabaseName, r.BlksRead, r.BlksHit, r.TempFiles, r.TempBytes)
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

func (tl *TimescaleLogger) GetPostgresDbIOHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]PostgresDbIORow, error) {
	if limit <= 0 {
		limit = 500
	}
	var q string
	var args []interface{}
	args = append(args, serverID)

	if !from.IsZero() && !to.IsZero() {
		q = `
			SELECT capture_timestamp, server_id, database_name, blks_read, blks_hit, temp_files, temp_bytes
			FROM postgres_db_io_stats
			WHERE server_id = $1
			  AND capture_timestamp >= $2 AND capture_timestamp <= $3
			ORDER BY capture_timestamp DESC
			LIMIT 2000
		`
		args = append(args, from, to)
	} else {
		q = `
			SELECT capture_timestamp, server_id, database_name, blks_read, blks_hit, temp_files, temp_bytes
			FROM postgres_db_io_stats
			WHERE server_id = $1
			ORDER BY capture_timestamp DESC
			LIMIT $2
		`
		args = append(args, limit)
	}

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostgresDbIORow
	for rows.Next() {
		var r PostgresDbIORow
		if err := rows.Scan(&r.Timestamp, &r.ServerID, &r.DatabaseName, &r.BlksRead, &r.BlksHit, &r.TempFiles, &r.TempBytes); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogPostgresSettingsSnapshot(ctx context.Context, serverID uuid.UUID, rows []PostgresSettingSnapshotRow) error {
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.Name+"|"+r.Setting+"|"+r.Unit+"|"+r.Source)
	}
	sort.Strings(keys)
	sig := pgFnv64(serverID, keys)

	tl.mu.Lock()
	if prev := tl.prevPgSettingsHash[serverID]; prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgSettingsHash[serverID] = sig
	tl.mu.Unlock()

	const q = `INSERT INTO postgres_settings_snapshot (
		capture_timestamp, server_id, name, setting, unit, source
	) VALUES ($1,$2,$3,$4,$5,$6)`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.Timestamp, serverID, r.Name, r.Setting, r.Unit, r.Source)
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

func (tl *TimescaleLogger) GetPostgresSettingsSnapshotLatestTwo(ctx context.Context, serverID uuid.UUID) (latestTs time.Time, prevTs time.Time, latest []PostgresSettingSnapshotRow, prev []PostgresSettingSnapshotRow, err error) {
	// Find two most recent distinct capture timestamps.
	tsRows, err := tl.pool.Query(ctx, `
		SELECT DISTINCT capture_timestamp
		FROM postgres_settings_snapshot
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT 2
	`, serverID)
	if err != nil {
		return time.Time{}, time.Time{}, nil, nil, err
	}
	defer tsRows.Close()
	var tss []time.Time
	for tsRows.Next() {
		var t time.Time
		if err := tsRows.Scan(&t); err == nil {
			tss = append(tss, t)
		}
	}
	if len(tss) == 0 {
		return time.Time{}, time.Time{}, []PostgresSettingSnapshotRow{}, []PostgresSettingSnapshotRow{}, nil
	}
	latestTs = tss[0]
	if len(tss) > 1 {
		prevTs = tss[1]
	}

	load := func(ts time.Time) ([]PostgresSettingSnapshotRow, error) {
		if ts.IsZero() {
			return []PostgresSettingSnapshotRow{}, nil
		}
		rows, err := tl.pool.Query(ctx, `
			SELECT capture_timestamp, server_id, name, COALESCE(setting,''), COALESCE(unit,''), COALESCE(source,'')
			FROM postgres_settings_snapshot
			WHERE server_id = $1 AND capture_timestamp = $2
		`, serverID, ts)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []PostgresSettingSnapshotRow
		for rows.Next() {
			var r PostgresSettingSnapshotRow
			if err := rows.Scan(&r.Timestamp, &r.ServerID, &r.Name, &r.Setting, &r.Unit, &r.Source); err == nil {
				out = append(out, r)
			}
		}
		return out, rows.Err()
	}
	latest, err = load(latestTs)
	if err != nil {
		return
	}
	prev, err = load(prevTs)
	return
}

func (tl *TimescaleLogger) LogPostgresBGWriterStats(ctx context.Context, row PostgresBGWriterRow) error {
	const q = `INSERT INTO postgres_bgwriter_stats (
		capture_timestamp, server_id, checkpoints_timed, checkpoints_req, 
		checkpoint_write_time, checkpoint_sync_time, buffers_checkpoint, 
		buffers_clean, maxwritten_clean, buffers_backend, 
		buffers_backend_fsync, buffers_alloc, stats_reset
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

	_, err := tl.pool.Exec(ctx, q,
		row.CaptureTimestamp, row.ServerID, row.CheckpointsTimed, row.CheckpointsReq,
		row.CheckpointWriteTime, row.CheckpointSyncTime, row.BuffersCheckpoint,
		row.BuffersClean, row.MaxwrittenClean, row.BuffersBackend,
		row.BuffersBackendFsync, row.BuffersAlloc, row.StatsReset,
	)
	return err
}

func (tl *TimescaleLogger) LogPostgresArchiverStats(ctx context.Context, row PostgresArchiverRow) error {
	const q = `INSERT INTO postgres_archiver_stats (
		capture_timestamp, server_id, archived_count, last_archived_wal, 
		last_archived_time, failed_count, last_failed_wal, 
		last_failed_time, stats_reset
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

	_, err := tl.pool.Exec(ctx, q,
		row.CaptureTimestamp, row.ServerID, row.ArchivedCount, row.LastArchivedWal,
		row.LastArchivedTime, row.FailedCount, row.LastFailedWal,
		row.LastFailedTime, row.StatsReset,
	)
	return err
}

func (tl *TimescaleLogger) GetPostgresBGWriterHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT 
			capture_timestamp, checkpoints_timed, checkpoints_req, 
			checkpoint_write_time, checkpoint_sync_time, buffers_clean, maxwritten_clean
		FROM postgres_bgwriter_stats
		WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
		ORDER BY capture_timestamp DESC
		LIMIT $4`

	rows, err := tl.pool.Query(ctx, query, serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var timed, req, clean, maxwritten int64
		var write, sync float64
		if err := rows.Scan(&ts, &timed, &req, &write, &sync, &clean, &maxwritten); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"timestamp":             ts,
			"checkpoints_timed":     timed,
			"checkpoints_req":       req,
			"checkpoint_write_time": write,
			"checkpoint_sync_time":  sync,
			"buffers_clean":         clean,
			"maxwritten_clean":      maxwritten,
		})
	}
	return results, nil
}

func (tl *TimescaleLogger) GetPostgresArchiverHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT
			capture_timestamp, archived_count, failed_count, last_archived_wal, last_failed_wal
		FROM postgres_archiver_stats
		WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
		ORDER BY capture_timestamp DESC
		LIMIT $4`

	rows, err := tl.pool.Query(ctx, query, serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var archived, failed int64
		var lastArchivedWAL, lastFailedWAL *string
		if err := rows.Scan(&ts, &archived, &failed, &lastArchivedWAL, &lastFailedWAL); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"timestamp":         ts,
			"archived_count":    archived,
			"failed_count":      failed,
			"last_archived_wal": lastArchivedWAL,
			"last_failed_wal":   lastFailedWAL,
		})
	}
	return results, nil
}

func (tl *TimescaleLogger) LogPostgresBackupDR(ctx context.Context, serverID uuid.UUID, row PostgresBackupDRRow) error {
	const q = `
		INSERT INTO snapshot.pg_backup_dr_timeseries (
			capture_timestamp, server_id, wal_bytes_total, wal_records_total, wal_fpi_total,
			archived_count, archive_failed_count, last_archived_time, last_failed_time,
			checkpoints_timed, checkpoints_req, checkpoint_write_time_ms, checkpoint_sync_time_ms,
			is_in_recovery
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err := tl.pool.Exec(ctx, q,
		row.CaptureTimestamp, serverID, row.WalBytesTotal, row.WalRecordsTotal, row.WalFPITotal,
		row.ArchivedCount, row.ArchiveFailedCount, row.LastArchivedTime, row.LastFailedTime,
		row.CheckpointsTimed, row.CheckpointsReq, row.CheckpointWriteTimeMs, row.CheckpointSyncTimeMs,
		row.IsInRecovery,
	)
	return err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [64]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
