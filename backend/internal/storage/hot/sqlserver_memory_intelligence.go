// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server Memory Intelligence and analyzer results logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// LogSQLServerMemoryMetrics inserts one snapshot row (append-only).
func (tl *TimescaleLogger) LogSQLServerMemoryMetrics(ctx context.Context, instanceName string, row map[string]interface{}) error {
	if row == nil {
		return nil
	}
	// Compute spill rates from cumulative counters.
	// We do this here (logger-side) so the collector stays “must-have only” and storage gets per-sec series.
	sortTot := getInt64FromMap(row, "sort_warnings_total")
	hashTot := getInt64FromMap(row, "hash_warnings_total")
	sortRate := 0.0
	hashRate := 0.0
	now := time.Now().UTC()

	tl.mu.Lock()
	if tl.prevSpillByInstance == nil {
		tl.prevSpillByInstance = make(map[string]spillDeltaState)
	}
	prev := tl.prevSpillByInstance[instanceName]
	dt := now.Sub(prev.lastTS).Seconds()
	if !prev.lastTS.IsZero() && dt > 0 {
		ds := float64(sortTot - prev.lastSort)
		dh := float64(hashTot - prev.lastHash)
		if ds >= 0 {
			sortRate = ds / dt
		}
		if dh >= 0 {
			hashRate = dh / dt
		}
	}
	tl.prevSpillByInstance[instanceName] = spillDeltaState{
		lastTS:   now,
		lastSort: sortTot,
		lastHash: hashTot,
	}
	tl.mu.Unlock()

	_, err := tl.pool.Exec(ctx, `
		INSERT INTO sqlserver_memory_metrics (
			capture_timestamp, server_instance_name,
			sql_memory_used_mb, sql_memory_target_mb,
			os_total_memory_mb, os_available_memory_mb,
			process_physical_low, process_virtual_low,
			memory_grants_pending, active_memory_grants, waiting_memory_grants,
			granted_workspace_mb, requested_workspace_mb,
			ple_seconds, plan_cache_mb,
			sort_warnings_total, hash_warnings_total,
			sort_warnings_per_sec, hash_warnings_per_sec
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
	`, now, instanceName,
		getInt64FromMap(row, "sql_memory_used_mb"),
		getInt64FromMap(row, "sql_memory_target_mb"),
		getInt64FromMap(row, "os_total_memory_mb"),
		getInt64FromMap(row, "os_available_memory_mb"),
		getBool(row, "process_physical_low"),
		getBool(row, "process_virtual_low"),
		int32(getInt64FromMap(row, "memory_grants_pending")),
		int32(getInt64FromMap(row, "active_memory_grants")),
		int32(getInt64FromMap(row, "waiting_memory_grants")),
		getInt64FromMap(row, "granted_workspace_mb"),
		getInt64FromMap(row, "requested_workspace_mb"),
		getInt64FromMap(row, "ple_seconds"),
		getInt64FromMap(row, "plan_cache_mb"),
		sortTot,
		hashTot,
		sortRate,
		hashRate,
	)
	return err
}

func (tl *TimescaleLogger) LogSQLServerBufferPoolByDB(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_buffer_pool_db (
				capture_timestamp, server_instance_name, database_name, buffer_mb
			) VALUES ($1,$2,$3,$4)
		`, now, instanceName,
			getStr(r, "database_name"),
			getInt64FromMap(r, "buffer_mb"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("buffer pool by db insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSQLServerMemoryMetricsRange(ctx context.Context, instanceName, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 2000
	}
	q := `
		SELECT capture_timestamp,
		       sql_memory_used_mb, sql_memory_target_mb,
		       os_total_memory_mb, os_available_memory_mb,
		       process_physical_low, process_virtual_low,
		       memory_grants_pending, active_memory_grants, waiting_memory_grants,
		       granted_workspace_mb, requested_workspace_mb,
		       ple_seconds, plan_cache_mb,
		       sort_warnings_total, hash_warnings_total,
		       sort_warnings_per_sec, hash_warnings_per_sec
		FROM sqlserver_memory_metrics
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
		LIMIT $4
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var ts time.Time
		var sqlUsed, sqlTarget, osTotal, osAvail, grantsPending, activeGrants, waitingGrants int64
		var grantedWS, requestedWS, pleSec, planMB, sortTot, hashTot int64
		var sortRate, hashRate float64
		var procPhys, procVirt sql.NullBool
		if err := rows.Scan(&ts,
			&sqlUsed, &sqlTarget,
			&osTotal, &osAvail,
			&procPhys, &procVirt,
			&grantsPending, &activeGrants, &waitingGrants,
			&grantedWS, &requestedWS,
			&pleSec, &planMB,
			&sortTot, &hashTot,
			&sortRate, &hashRate,
		); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp":      ts,
			"event_time":             ts.Format(time.RFC3339),
			"sql_memory_used_mb":     sqlUsed,
			"sql_memory_target_mb":   sqlTarget,
			"os_total_memory_mb":     osTotal,
			"os_available_memory_mb": osAvail,
			"process_physical_low":   procPhys.Valid && procPhys.Bool,
			"process_virtual_low":    procVirt.Valid && procVirt.Bool,
			"memory_grants_pending":  grantsPending,
			"active_memory_grants":   activeGrants,
			"waiting_memory_grants":  waitingGrants,
			"granted_workspace_mb":   grantedWS,
			"requested_workspace_mb": requestedWS,
			"ple_seconds":            pleSec,
			"plan_cache_mb":          planMB,
			"sort_warnings_total":    sortTot,
			"hash_warnings_total":    hashTot,
			"sort_warnings_per_sec":  sortRate,
			"hash_warnings_per_sec":  hashRate,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) GetSQLServerBufferPoolByDBRange(ctx context.Context, instanceName, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	q := `
		SELECT capture_timestamp, database_name, buffer_mb
		FROM sqlserver_buffer_pool_db
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC, buffer_mb DESC
		LIMIT $4
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var ts time.Time
		var dbName sql.NullString
		var mb int64
		if err := rows.Scan(&ts, &dbName, &mb); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp": ts,
			"event_time":        ts.Format(time.RFC3339),
			"database_name":     dbName.String,
			"buffer_mb":         mb,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) GetPlanCacheHealthRange(ctx context.Context, instanceName, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 2000
	}
	q := `
		SELECT capture_timestamp, total_cache_mb, single_use_cache_mb, single_use_cache_pct,
		       adhoc_cache_mb, prepared_cache_mb, proc_cache_mb
		FROM sqlserver_plan_cache
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
		LIMIT $4
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var ts time.Time
		var total, single, pct, adhoc, prep, proc float64
		if err := rows.Scan(&ts, &total, &single, &pct, &adhoc, &prep, &proc); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp":    ts,
			"event_time":           ts.Format(time.RFC3339),
			"total_cache_mb":       total,
			"single_use_cache_mb":  single,
			"single_use_cache_pct": pct,
			"adhoc_cache_mb":       adhoc,
			"prepared_cache_mb":    prep,
			"proc_cache_mb":        proc,
		})
	}
	return out, rows.Err()
}

// GetSQLServerMetricsRange returns sqlserver_metrics rows for [from, to] (RFC3339), ordered by time.
func (tl *TimescaleLogger) GetSQLServerMetricsRange(ctx context.Context, instanceName, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 2000
	}
	query := `
		SELECT capture_timestamp, server_instance_name, avg_cpu_load, memory_usage,
		       active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
		FROM sqlserver_metrics
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
		LIMIT $4`
	rows, err := tl.pool.Query(ctx, query, instanceName, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var srv string
		var cpu, mem, dataMB, logMB, freeMB float64
		var users, locks, dead int
		if err := rows.Scan(&ts, &srv, &cpu, &mem, &users, &locks, &dead, &dataMB, &logMB, &freeMB); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp": ts,
			"event_time":        ts.Format(time.RFC3339),
			"server_name":       srv,
			"avg_cpu_load":      cpu,
			"memory_usage":      mem,
			"active_users":      users,
			"total_locks":       locks,
			"deadlocks":         dead,
			"data_disk_mb":      dataMB,
			"log_disk_mb":       logMB,
			"free_disk_mb":      freeMB,
		})
	}
	return out, rows.Err()
}

// GetSQLServerMemoryHistoryRange returns sqlserver_memory_history PLE samples for [from, to] (RFC3339).
func (tl *TimescaleLogger) GetSQLServerMemoryHistoryRange(ctx context.Context, instanceName, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 2000
	}
	q := `
		SELECT capture_timestamp, page_life_expectancy
		FROM sqlserver_memory_history
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
		LIMIT $4`
	rows, err := tl.pool.Query(ctx, q, instanceName, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var ple sql.NullFloat64
		if err := rows.Scan(&ts, &ple); err != nil {
			continue
		}
		pv := 0.0
		if ple.Valid {
			pv = ple.Float64
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp":            ts,
			"event_time":                   ts.Format(time.RFC3339),
			"page_life_expectancy_seconds": pv,
			"page_life_expectancy":         pv,
			"ple":                          pv,
		})
	}
	return out, rows.Err()
}

// GetSQLServerSchedulerMemoryRange returns sqlserver_cpu_scheduler_stats OS memory fields for [from, to] (RFC3339).
func (tl *TimescaleLogger) GetSQLServerSchedulerMemoryRange(ctx context.Context, instanceName, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 2000
	}
	q := `
		SELECT capture_timestamp,
		       total_physical_memory_kb, available_physical_memory_kb,
		       system_memory_state_desc, physical_memory_pressure_warning
		FROM sqlserver_cpu_scheduler_stats
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
		LIMIT $4`
	rows, err := tl.pool.Query(ctx, q, instanceName, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var totalPhys, availPhys sql.NullInt64
		var stateDesc sql.NullString
		var pressure sql.NullBool
		if err := rows.Scan(&ts, &totalPhys, &availPhys, &stateDesc, &pressure); err != nil {
			continue
		}
		tpk := int64(0)
		if totalPhys.Valid {
			tpk = totalPhys.Int64
		}
		apk := int64(0)
		if availPhys.Valid {
			apk = availPhys.Int64
		}
		pctFree := 0.0
		if tpk > 0 {
			pctFree = (float64(apk) / float64(tpk)) * 100.0
		}
		m := map[string]interface{}{
			"capture_timestamp":                ts,
			"event_time":                       ts.Format(time.RFC3339),
			"total_physical_memory_kb":         tpk,
			"available_physical_memory_kb":     apk,
			"physical_memory_free_percent":     pctFree,
			"physical_memory_pressure_warning": pressure.Valid && pressure.Bool,
			"system_memory_state_desc":         "",
		}
		if stateDesc.Valid {
			m["system_memory_state_desc"] = stateDesc.String
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
