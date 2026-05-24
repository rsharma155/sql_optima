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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// LogSQLServerMemoryMetrics inserts one snapshot row (append-only).
func (tl *TimescaleLogger) LogSQLServerMemoryMetrics(ctx context.Context, serverID uuid.UUID, row map[string]interface{}) error {
	if row == nil {
		return nil
	}
	// Compute spill rates from cumulative counters.
	sortTot := getInt64FromMap(row, "sort_warnings_total")
	hashTot := getInt64FromMap(row, "hash_warnings_total")
	sortRate := 0.0
	hashRate := 0.0
	now := time.Now().UTC()

	tl.mu.Lock()
	if tl.prevSpillByInstance == nil {
		tl.prevSpillByInstance = make(map[uuid.UUID]spillDeltaState)
	}
	prev := tl.prevSpillByInstance[serverID]
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
	tl.prevSpillByInstance[serverID] = spillDeltaState{
		lastTS:   now,
		lastSort: sortTot,
		lastHash: hashTot,
	}
	tl.mu.Unlock()

	_, err := tl.pool.Exec(ctx, `
		INSERT INTO sqlserver_memory_metrics (
			capture_timestamp, server_id,
			sql_memory_used_mb, sql_memory_target_mb,
			os_total_memory_mb, os_available_memory_mb,
			os_system_memory_state,
			process_physical_low, process_virtual_low,
			memory_grants_pending, active_memory_grants, waiting_memory_grants,
			granted_workspace_mb, requested_workspace_mb,
			ple_seconds, plan_cache_mb,
			sort_warnings_total, hash_warnings_total,
			sort_warnings_per_sec, hash_warnings_per_sec,
			sql_physical_memory_in_use_mb, sql_memory_utilization_pct,
			sql_page_fault_count, sql_locked_page_alloc_mb, sql_large_page_alloc_mb
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20, $21, $22, $23, $24, $25)
	`, now, serverID,
		getInt64FromMap(row, "sql_memory_used_mb"),
		getInt64FromMap(row, "sql_memory_target_mb"),
		getInt64FromMap(row, "os_total_memory_mb"),
		getInt64FromMap(row, "os_available_memory_mb"),
		getStr(row, "os_system_memory_state"),
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
		getInt64FromMap(row, "sql_physical_memory_in_use_mb"),
		getInt64FromMap(row, "sql_memory_utilization_pct"),
		getInt64FromMap(row, "sql_page_fault_count"),
		getInt64FromMap(row, "sql_locked_page_alloc_mb"),
		getInt64FromMap(row, "sql_large_page_alloc_mb"),
	)
	return err
}

func (tl *TimescaleLogger) LogSQLServerBufferPoolByDB(ctx context.Context, serverID uuid.UUID, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_buffer_pool_db (
				capture_timestamp, server_id, database_name, buffer_mb
			) VALUES ($1,$2,$3,$4)
		`, now, serverID,
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

func (tl *TimescaleLogger) GetSQLServerMemoryMetricsRange(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 2000
	}

	// Dynamic bucket size for zoom support
	dur := end.Sub(start)
	bucket := "1 minute"
	if dur > 24*time.Hour {
		bucket = "15 minutes"
	} else if dur > 6*time.Hour {
		bucket = "5 minutes"
	}

	q := fmt.Sprintf(`
		SELECT time_bucket('%s', capture_timestamp) AS bucket,
		       AVG(sql_memory_used_mb), AVG(sql_memory_target_mb),
		       AVG(os_total_memory_mb), AVG(os_available_memory_mb),
		       BOOL_OR(process_physical_low), BOOL_OR(process_virtual_low),
		       MAX(memory_grants_pending), MAX(active_memory_grants), MAX(waiting_memory_grants),
		       AVG(granted_workspace_mb), AVG(requested_workspace_mb),
		       AVG(ple_seconds), AVG(plan_cache_mb),
		       MAX(sort_warnings_total), MAX(hash_warnings_total),
		       AVG(sort_warnings_per_sec), AVG(hash_warnings_per_sec)
		FROM sqlserver_memory_metrics
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket ASC
		LIMIT $4
	`, bucket)

	rows, err := tl.pool.Query(ctx, q, serverID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var ts time.Time
		var sqlUsed, sqlTarget, osTotal, osAvail, grantsPending, activeGrants, waitingGrants *float64
		var grantedWS, requestedWS, pleSec, planMB, sortTot, hashTot *float64
		var sortRate, hashRate *float64
		var procPhys, procVirt *bool
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
			"timestamp":              ts,
			"sql_memory_used_mb":     ptrToFloat64(sqlUsed),
			"sql_memory_target_mb":   ptrToFloat64(sqlTarget),
			"os_total_memory_mb":     ptrToFloat64(osTotal),
			"os_available_memory_mb": ptrToFloat64(osAvail),
			"process_physical_low":   ptrToBool(procPhys),
			"process_virtual_low":    ptrToBool(procVirt),
			"memory_grants_pending":  ptrToFloat64(grantsPending),
			"active_memory_grants":   ptrToFloat64(activeGrants),
			"waiting_memory_grants":  ptrToFloat64(waitingGrants),
			"granted_workspace_mb":   ptrToFloat64(grantedWS),
			"requested_workspace_mb": ptrToFloat64(requestedWS),
			"ple_seconds":            ptrToFloat64(pleSec),
			"plan_cache_mb":          ptrToFloat64(planMB),
			"sort_warnings_total":    ptrToFloat64(sortTot),
			"hash_warnings_total":    ptrToFloat64(hashTot),
			"sort_warnings_per_sec":  ptrToFloat64(sortRate),
			"hash_warnings_per_sec":  ptrToFloat64(hashRate),
		})
	}
	return out, rows.Err()
}

func ptrToFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func ptrToBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func (tl *TimescaleLogger) GetSQLServerBufferPoolByDBRange(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}

	dur := end.Sub(start)
	bucket := "1 minute"
	if dur > 24*time.Hour {
		bucket = "15 minutes"
	} else if dur > 6*time.Hour {
		bucket = "5 minutes"
	}

	q := fmt.Sprintf(`
		SELECT time_bucket('%s', capture_timestamp) AS bucket, database_name, AVG(buffer_mb) as buffer_mb
		FROM sqlserver_buffer_pool_db
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		GROUP BY bucket, database_name
		ORDER BY bucket ASC, buffer_mb DESC
		LIMIT $4
	`, bucket)

	rows, err := tl.pool.Query(ctx, q, serverID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var ts time.Time
		var dbName sql.NullString
		var mb float64
		if err := rows.Scan(&ts, &dbName, &mb); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":     ts,
			"database_name": dbName.String,
			"buffer_mb":     mb,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) GetPlanCacheHealthRange(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 2000
	}

	dur := end.Sub(start)
	bucket := "1 minute"
	if dur > 24*time.Hour {
		bucket = "15 minutes"
	} else if dur > 6*time.Hour {
		bucket = "5 minutes"
	}

	q := fmt.Sprintf(`
		SELECT time_bucket('%s', capture_timestamp) AS bucket,
		       AVG(total_cache_mb), AVG(single_use_cache_mb), AVG(single_use_cache_pct),
		       AVG(adhoc_cache_mb), AVG(prepared_cache_mb), AVG(proc_cache_mb)
		FROM sqlserver_plan_cache
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket ASC
		LIMIT $4
	`, bucket)

	rows, err := tl.pool.Query(ctx, q, serverID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var ts time.Time
		var total, single, pct, adhoc, prep, proc *float64
		if err := rows.Scan(&ts, &total, &single, &pct, &adhoc, &prep, &proc); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":            ts,
			"total_cache_mb":       ptrToFloat64(total),
			"single_use_cache_mb":  ptrToFloat64(single),
			"single_use_cache_pct": ptrToFloat64(pct),
			"adhoc_cache_mb":       ptrToFloat64(adhoc),
			"prepared_cache_mb":    ptrToFloat64(prep),
			"proc_cache_mb":        ptrToFloat64(proc),
		})
	}
	return out, rows.Err()
}
