// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Standardized SQL Server enterprise metrics storage implementation.
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

// Standardized SQL Server Enterprise V2 Logging

func (tl *TimescaleLogger) LogSqlServerWaitStats(ctx context.Context, instanceID string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_wait_stats (
				capture_timestamp, server_instance_name, wait_category, wait_time_ms, signal_wait_time_ms, waiting_tasks
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, now, instanceID,
			getStr(r, "wait_category"),
			getInt64FromMap(r, "wait_time_ms"),
			getInt64FromMap(r, "signal_wait_time_ms"),
			getInt64FromMap(r, "waiting_tasks_delta"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("wait stats v2 insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSqlServerWaitStats(ctx context.Context, instanceID string, from, to string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}

	q := `
		SELECT capture_timestamp, wait_category, wait_time_ms, signal_wait_time_ms, waiting_tasks
		FROM sqlserver_wait_stats
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var cat string
		var wms, swms, wt int64
		if err := rows.Scan(&ts, &cat, &wms, &swms, &wt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":           ts,
			"event_time":          ts.Format(time.RFC3339),
			"wait_category":       cat,
			"wait_time_ms":        wms,
			"signal_wait_time_ms": swms,
			"waiting_tasks":       wt,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSqlServerPerfCounters(ctx context.Context, instanceID string, counters map[string]float64) error {
	if len(counters) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for name, val := range counters {
		batch.Queue(`
			INSERT INTO sqlserver_perf_counters (
				capture_timestamp, server_instance_name, counter_name, value_per_sec
			) VALUES ($1, $2, $3, $4)
		`, now, instanceID, name, val)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(counters); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("perf counters v2 insert failed: %w", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSqlServerPerfCounters(ctx context.Context, instanceID string, from, to string, counterNames []string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}

	q := `
		SELECT capture_timestamp, counter_name, value_per_sec
		FROM sqlserver_perf_counters
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
	`
	var args []interface{}
	args = append(args, instanceID, start, end)
	if len(counterNames) > 0 {
		q += " AND counter_name = ANY($4)"
		args = append(args, counterNames)
	}
	q += " ORDER BY capture_timestamp ASC"

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var name string
		var val float64
		if err := rows.Scan(&ts, &name, &val); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":    ts,
			"event_time":   ts.Format(time.RFC3339),
			"counter_name": name,
			"value":        val,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSqlServerFileIO(ctx context.Context, instanceID string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_file_io (
				capture_timestamp, server_instance_name, database_name, file_type, read_latency_ms, write_latency_ms
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, now, instanceID,
			getStr(r, "database_name"),
			getStr(r, "file_type"),
			getFloat64(r, "read_latency_ms"),
			getFloat64(r, "write_latency_ms"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("file io v2 insert failed: %w", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSqlServerFileIO(ctx context.Context, instanceID string, from, to string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}

	q := `
		SELECT capture_timestamp, database_name, file_type, read_latency_ms, write_latency_ms
		FROM sqlserver_file_io
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var db, ft string
		var rl, wl float64
		if err := rows.Scan(&ts, &db, &ft, &rl, &wl); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":        ts,
			"event_time":       ts.Format(time.RFC3339),
			"database_name":    db,
			"file_type":        ft,
			"read_latency_ms":  rl,
			"write_latency_ms": wl,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSqlServerPlanCache(ctx context.Context, instanceID string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	// Dedup using fingerprint
	sig := fingerprintMapRows(instanceID, "plan_cache_v2", rows, []string{"cache_type"}, []string{"size_mb"})
	if tl.EnterpriseSnapshotUnchanged(instanceID, "plan_cache_v2", sig) {
		return nil
	}

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_plan_cache (
				capture_timestamp, server_instance_name, cache_type, size_mb
			) VALUES ($1, $2, $3, $4)
		`, now, instanceID, getStr(r, "cache_type"), getFloat64(r, "size_mb"))
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("plan cache v2 insert failed: %w", err)
		}
	}
	tl.RememberEnterpriseSnapshot(instanceID, "plan_cache_v2", sig)
	return nil
}

func (tl *TimescaleLogger) GetSqlServerPlanCache(ctx context.Context, instanceID string, from, to string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	q := `
		SELECT capture_timestamp, cache_type, size_mb
		FROM sqlserver_plan_cache
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var ct string
		var sm float64
		if err := rows.Scan(&ts, &ct, &sm); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":  ts,
			"event_time": ts.Format(time.RFC3339),
			"cache_type": ct,
			"size_mb":    sm,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSqlServerMemoryClerksV2(ctx context.Context, instanceID string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	sig := fingerprintMapRows(instanceID, "memory_clerks_v2", rows, []string{"clerk_name"}, []string{"pages_mb"})
	if tl.EnterpriseSnapshotUnchanged(instanceID, "memory_clerks_v2", sig) {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_memory_clerks (
				capture_timestamp, server_instance_name, clerk_name, pages_mb
			) VALUES ($1, $2, $3, $4)
		`, now, instanceID, getStr(r, "clerk_name"), getFloat64(r, "pages_mb"))
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("memory clerks v2 insert failed: %w", err)
		}
	}
	tl.RememberEnterpriseSnapshot(instanceID, "memory_clerks_v2", sig)
	return nil
}

func (tl *TimescaleLogger) GetSqlServerMemoryClerksV2(ctx context.Context, instanceID string, from, to string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	q := `
		SELECT capture_timestamp, clerk_name, pages_mb
		FROM sqlserver_memory_clerks
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var cn string
		var pm float64
		if err := rows.Scan(&ts, &cn, &pm); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":  ts,
			"event_time": ts.Format(time.RFC3339),
			"clerk_name": cn,
			"pages_mb":   pm,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSqlServerMemoryGrantsV2(ctx context.Context, instanceID string, data map[string]interface{}) error {
	now := time.Now().UTC()
	q := `
		INSERT INTO sqlserver_memory_grants (
			capture_timestamp, server_instance_name, pending_grants, active_grants, granted_memory_mb
		) VALUES ($1, $2, $3, $4, $5)
	`
	_, err := tl.pool.Exec(ctx, q, now, instanceID,
		getInt(data, "pending_grants"),
		getInt(data, "active_grants"),
		getFloat64(data, "granted_memory_mb"),
	)
	return err
}

func (tl *TimescaleLogger) GetSqlServerMemoryGrantsV2(ctx context.Context, instanceID string, from, to string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	q := `
		SELECT capture_timestamp, pending_grants, active_grants, granted_memory_mb
		FROM sqlserver_memory_grants
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var pg, ag int
		var gm float64
		if err := rows.Scan(&ts, &pg, &ag, &gm); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":         ts,
			"event_time":        ts.Format(time.RFC3339),
			"pending_grants":    pg,
			"active_grants":     ag,
			"granted_memory_mb": gm,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSqlServerTempdbConsumers(ctx context.Context, instanceID string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_tempdb_consumers (
				capture_timestamp, server_instance_name, session_id, request_id, allocated_mb, user_object_mb, internal_object_mb, query_text
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, now, instanceID,
			getInt(r, "session_id"),
			getInt(r, "request_id"),
			getFloat64(r, "allocated_mb"),
			getFloat64(r, "user_objects_mb"),
			getFloat64(r, "internal_objects_mb"),
			getStr(r, "query_text"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("tempdb consumers insert failed: %w", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSqlServerTempdbConsumers(ctx context.Context, instanceID string, from, to string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	q := `
		SELECT capture_timestamp, session_id, request_id, allocated_mb, user_object_mb, internal_object_mb, query_text
		FROM sqlserver_tempdb_consumers
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND query_text NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY capture_timestamp ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var sid, rid int
		var amb, uomb, iomb float64
		var qt string
		if err := rows.Scan(&ts, &sid, &rid, &amb, &uomb, &iomb, &qt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":           ts,
			"event_time":          ts.Format(time.RFC3339),
			"session_id":          sid,
			"request_id":          rid,
			"allocated_mb":        amb,
			"user_objects_mb":     uomb,
			"internal_objects_mb": iomb,
			"query_text":          qt,
		})
	}
	return out, rows.Err()
}
