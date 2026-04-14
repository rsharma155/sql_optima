// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server enterprise metrics logger for advanced monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// This file logs/reads advanced MSSQL “enterprise metrics” into TimescaleDB tables.
// The goal is to serve dashboards from TimescaleDB (historical + low load) and only
// fall back to direct DMV queries when TimescaleDB is unavailable.

func (tl *TimescaleLogger) LogLatchWaits(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	sig := fingerprintMapRows(instanceName, enterpriseKindLatchWaits, rows,
		[]string{"wait_type"},
		[]string{"waiting_tasks_count", "wait_time_ms", "signal_wait_time_ms"})
	if tl.enterpriseSnapshotUnchanged(instanceName, enterpriseKindLatchWaits, sig) {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_latch_waits (
				capture_timestamp, server_instance_name, wait_type, waiting_tasks_count, wait_time_ms, signal_wait_time_ms
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, now, instanceName,
			getStr(r, "wait_type"),
			getInt64FromMap(r, "waiting_tasks_count"),
			getInt64FromMap(r, "wait_time_ms"),
			getInt64FromMap(r, "signal_wait_time_ms"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("latch waits insert failed at row %d: %w", i, err)
		}
	}
	tl.rememberEnterpriseSnapshot(instanceName, enterpriseKindLatchWaits, sig)
	return nil
}

func (tl *TimescaleLogger) GetLatchWaits(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT wait_type, waiting_tasks_count, wait_time_ms, signal_wait_time_ms
		FROM sqlserver_latch_waits
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var waitType string
		var waitingTasks, waitMs, signalMs int64
		if err := rows.Scan(&waitType, &waitingTasks, &waitMs, &signalMs); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"wait_type":             waitType,
			"waiting_tasks_count":   waitingTasks,
			"wait_time_ms":          waitMs,
			"signal_wait_time_ms":   signalMs,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogWaitingTasks(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_waiting_tasks (
				capture_timestamp, server_instance_name, wait_type, resource_description, waiting_tasks_count, wait_duration_ms
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, now, instanceName,
			getStr(r, "wait_type"),
			getStr(r, "resource_description"),
			getInt64FromMap(r, "waiting_tasks_count"),
			getInt64FromMap(r, "wait_duration_ms"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("waiting tasks insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetWaitingTasks(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT wait_type, resource_description, waiting_tasks_count, wait_duration_ms
		FROM sqlserver_waiting_tasks
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var waitType, resourceDesc string
		var waitingTasks, waitMs int64
		if err := rows.Scan(&waitType, &resourceDesc, &waitingTasks, &waitMs); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"wait_type":            waitType,
			"resource_description": resourceDesc,
			"waiting_tasks_count":  waitingTasks,
			"wait_duration_ms":     waitMs,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogMemoryGrants(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_memory_grants (
				capture_timestamp, server_instance_name, session_id, database_name, login_name,
				granted_memory_kb, used_memory_kb, dop, query_duration_sec
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, now, instanceName,
			int16(getInt64FromMap(r, "session_id")),
			getStr(r, "database_name"),
			getStr(r, "login_name"),
			getInt64FromMap(r, "granted_memory_kb"),
			getInt64FromMap(r, "used_memory_kb"),
			int16(getInt64FromMap(r, "dop")),
			getFloat64(r, "query_duration_sec"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("memory grants insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetMemoryGrants(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT session_id, database_name, login_name, granted_memory_kb, used_memory_kb, dop, query_duration_sec
		FROM sqlserver_memory_grants
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var sid int16
		var dbName, login string
		var granted, used int64
		var dop int16
		var dur float64
		if err := rows.Scan(&sid, &dbName, &login, &granted, &used, &dop, &dur); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"session_id":         sid,
			"database_name":      dbName,
			"login_name":         login,
			"granted_memory_kb":  granted,
			"used_memory_kb":     used,
			"dop":                dop,
			"query_duration_sec": dur,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogProcedureStats(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	sig := fingerprintMapRows(instanceName, enterpriseKindProcedure, rows,
		[]string{"database_name", "schema_name", "object_name"},
		[]string{"execution_count", "total_worker_time_ms", "total_elapsed_time_ms", "total_logical_reads", "total_physical_reads"})
	if tl.enterpriseSnapshotUnchanged(instanceName, enterpriseKindProcedure, sig) {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_procedure_stats (
				capture_timestamp, server_instance_name, database_name, schema_name, object_name,
				execution_count, total_worker_time_ms, total_elapsed_time_ms, total_logical_reads, total_physical_reads
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, now, instanceName,
			getStr(r, "database_name"),
			getStr(r, "schema_name"),
			getStr(r, "object_name"),
			getInt64FromMap(r, "execution_count"),
			getFloat64(r, "total_worker_time_ms"),
			getFloat64(r, "total_elapsed_time_ms"),
			getInt64FromMap(r, "total_logical_reads"),
			getInt64FromMap(r, "total_physical_reads"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("procedure stats insert failed at row %d: %w", i, err)
		}
	}
	tl.rememberEnterpriseSnapshot(instanceName, enterpriseKindProcedure, sig)
	return nil
}

func (tl *TimescaleLogger) GetProcedureStats(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT database_name, schema_name, object_name, execution_count, total_worker_time_ms, total_elapsed_time_ms, total_logical_reads
		FROM sqlserver_procedure_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var db, schema, obj string
		var execCount int64
		var worker, elapsed float64
		var logical int64
		if err := rows.Scan(&db, &schema, &obj, &execCount, &worker, &elapsed, &logical); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"database_name":        db,
			"schema_name":          schema,
			"object_name":          obj,
			"execution_count":      execCount,
			"total_worker_time_ms": worker,
			"total_elapsed_time_ms": elapsed,
			"total_logical_reads":  logical,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogFileIOLatency(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	// Do not apply enterprise batch dedup here: DMV averages often stay identical between scrapes,
	// which would suppress inserts and leave disk_latency_trend_1h empty in Timescale.
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_file_io_latency (
				capture_timestamp, server_instance_name, database_name, file_name, file_type,
				read_latency_ms, write_latency_ms, read_bytes_per_sec, write_bytes_per_sec
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, now, instanceName,
			getStr(r, "database_name"),
			getStr(r, "file_name"),
			getStr(r, "file_type"),
			getFloat64(r, "read_latency_ms"),
			getFloat64(r, "write_latency_ms"),
			getFloat64(r, "read_bytes_per_sec"),
			getFloat64(r, "write_bytes_per_sec"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("file io latency insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetFileIOLatency(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT database_name, file_name, file_type, read_latency_ms, write_latency_ms
		FROM sqlserver_file_io_latency
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var db, file, ft string
		var rl, wl float64
		if err := rows.Scan(&db, &file, &ft, &rl, &wl); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"database_name":    db,
			"file_name":        file,
			"file_type":        ft,
			"read_latency_ms":  rl,
			"write_latency_ms": wl,
		})
	}
	return out, rows.Err()
}

// GetFileIOLatencyTrend returns 1-minute bucketed avg read/write latency (ms) across files.
func (tl *TimescaleLogger) GetFileIOLatencyTrend(ctx context.Context, instanceName string, minutes int) ([]map[string]interface{}, error) {
	if minutes <= 0 {
		minutes = 60
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT time_bucket('1 minute', capture_timestamp) AS bucket,
		       AVG(read_latency_ms) AS read_latency_ms,
		       AVG(write_latency_ms) AS write_latency_ms
		FROM sqlserver_file_io_latency
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - ($2::int * INTERVAL '1 minute')
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, minutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, minutes)
	for rows.Next() {
		var ts time.Time
		var rl, wl float64
		if err := rows.Scan(&ts, &rl, &wl); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":        ts,
			"read_latency_ms":  rl,
			"write_latency_ms": wl,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSpinlockStats(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	sig := fingerprintMapRows(instanceName, enterpriseKindSpinlock, rows,
		[]string{"spinlock_type"},
		[]string{"collisions", "spins", "sleep_time_ms"})
	if tl.enterpriseSnapshotUnchanged(instanceName, enterpriseKindSpinlock, sig) {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_spinlock_stats (
				capture_timestamp, server_instance_name, spinlock_type, collisions, spins, sleep_time_ms
			) VALUES ($1,$2,$3,$4,$5,$6)
		`, now, instanceName,
			getStr(r, "spinlock_type"),
			getInt64FromMap(r, "collisions"),
			getInt64FromMap(r, "spins"),
			getInt64FromMap(r, "sleep_time_ms"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("spinlock stats insert failed at row %d: %w", i, err)
		}
	}
	tl.rememberEnterpriseSnapshot(instanceName, enterpriseKindSpinlock, sig)
	return nil
}

func (tl *TimescaleLogger) GetSpinlockStats(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT spinlock_type, collisions, spins, sleep_time_ms
		FROM sqlserver_spinlock_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var t string
		var c, s, sl int64
		if err := rows.Scan(&t, &c, &s, &sl); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"spinlock_type": t,
			"collisions":    c,
			"spins":         s,
			"sleep_time_ms": sl,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogMemoryClerks(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	sig := fingerprintMapRows(instanceName, enterpriseKindMemoryClerks, rows,
		[]string{"clerk_type", "memory_node"},
		[]string{"pages_mb", "virtual_memory_reserved_mb", "virtual_memory_committed_mb", "awe_memory_mb"})
	if tl.enterpriseSnapshotUnchanged(instanceName, enterpriseKindMemoryClerks, sig) {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_memory_clerks (
				capture_timestamp, server_instance_name, clerk_type, memory_node,
				pages_mb, virtual_memory_reserved_mb, virtual_memory_committed_mb, awe_memory_mb
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, now, instanceName,
			getStr(r, "clerk_type"),
			int16(getInt64FromMap(r, "memory_node")),
			getFloat64(r, "pages_mb"),
			getFloat64(r, "virtual_memory_reserved_mb"),
			getFloat64(r, "virtual_memory_committed_mb"),
			getFloat64(r, "awe_memory_mb"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("memory clerks insert failed at row %d: %w", i, err)
		}
	}
	tl.rememberEnterpriseSnapshot(instanceName, enterpriseKindMemoryClerks, sig)
	return nil
}

func (tl *TimescaleLogger) GetMemoryClerks(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT capture_timestamp, clerk_type, memory_node, pages_mb, virtual_memory_reserved_mb, virtual_memory_committed_mb, awe_memory_mb
		FROM sqlserver_memory_clerks
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var ts time.Time
		var ct string
		var node int16
		var pages, rsv, com, awe float64
		if err := rows.Scan(&ts, &ct, &node, &pages, &rsv, &com, &awe); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp":          ts,
			"event_time":                ts.Format(time.RFC3339),
			"clerk_type":                 ct,
			"memory_node":                node,
			"pages_mb":                   pages,
			"virtual_memory_reserved_mb": rsv,
			"virtual_memory_committed_mb": com,
			"awe_memory_mb":              awe,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogTempdbFiles(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		allocated := getFloat64(r, "allocated_mb")
		used := getFloat64(r, "used_mb")
		free := getFloat64(r, "free_mb")
		usedPct := 0.0
		if allocated > 0 {
			usedPct = (used / allocated) * 100.0
		}
		batch.Queue(`
			INSERT INTO sqlserver_tempdb_files (
				capture_timestamp, server_instance_name, database_name, file_name, file_type,
				allocated_mb, used_mb, free_mb, max_size_mb, growth_mb, used_percent
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		`, now, instanceName,
			getStr(r, "database_name"),
			getStr(r, "file_name"),
			getStr(r, "file_type"),
			allocated,
			used,
			free,
			getFloat64(r, "max_size_mb"),
			getFloat64(r, "growth_mb"),
			usedPct,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("tempdb files insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetTempdbFiles(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT database_name, file_name, file_type, allocated_mb, used_mb, free_mb, max_size_mb, growth_mb, used_percent
		FROM sqlserver_tempdb_files
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var db, file, ft string
		var alloc, used, free, max, growth, pct float64
		if err := rows.Scan(&db, &file, &ft, &alloc, &used, &free, &max, &growth, &pct); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"database_name": db,
			"file_name":     file,
			"file_type":     ft,
			"allocated_mb":  alloc,
			"used_mb":       used,
			"free_mb":       free,
			"max_size_mb":   max,
			"growth_mb":     growth,
			"used_percent":  pct,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSchedulerWG(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_scheduler_wg (
				capture_timestamp, server_instance_name, pool_name, group_name, active_requests, queued_requests, cpu_usage_percent
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, now, instanceName,
			getStr(r, "pool_name"),
			getStr(r, "group_name"),
			getInt64FromMap(r, "active_requests"),
			getInt64FromMap(r, "queued_requests"),
			getFloat64(r, "cpu_usage_percent"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("scheduler wg insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSchedulerWG(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT pool_name, group_name, active_requests, queued_requests, cpu_usage_percent
		FROM sqlserver_scheduler_wg
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var pool, group string
		var active, queued int64
		var cpuPct float64
		if err := rows.Scan(&pool, &group, &active, &queued, &cpuPct); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"pool_name":         pool,
			"group_name":        group,
			"active_requests":   active,
			"queued_requests":   queued,
			"cpu_usage_percent": cpuPct,
		})
	}
	return out, rows.Err()
}

