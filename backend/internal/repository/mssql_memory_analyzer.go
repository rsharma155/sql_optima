// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Memory analyzer for detailed memory clerk breakdown and recommendations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// FetchMemoryAnalyzerSnapshot collects the must-have memory troubleshooting metrics for Timescale ingestion.
// All values are returned as best-effort (missing permissions/DMVs may result in zeros).
func (c *MssqlRepository) FetchMemoryAnalyzerSnapshot(ctx context.Context, instanceName string) (map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	out := map[string]interface{}{}

	// SQL Memory vs Target (KB -> MB)
	{
		q := `
			WITH mem AS (
				SELECT 
					MAX(CASE WHEN counter_name='Total Server Memory (KB)' THEN cntr_value END)/1024 AS total_mb,
					MAX(CASE WHEN counter_name='Target Server Memory (KB)' THEN cntr_value END)/1024 AS target_mb
				FROM sys.dm_os_performance_counters WITH (NOLOCK)
				WHERE counter_name IN ('Total Server Memory (KB)', 'Target Server Memory (KB)')
			)
			SELECT ISNULL(total_mb, 0), ISNULL(target_mb, 0) FROM mem;
		`
		var totalMB, targetMB int64
		_ = db.QueryRowContext(ctx, q).Scan(&totalMB, &targetMB)
		out["sql_memory_used_mb"] = totalMB
		out["sql_memory_target_mb"] = targetMB
	}

	// OS memory total/available
	{
		q := `
			SELECT 
				ISNULL(total_physical_memory_kb, 0)/1024 AS total_os_mb,
				ISNULL(available_physical_memory_kb, 0)/1024 AS available_os_mb
			FROM sys.dm_os_sys_memory WITH (NOLOCK);
		`
		var totalOS, availOS int64
		_ = db.QueryRowContext(ctx, q).Scan(&totalOS, &availOS)
		out["os_total_memory_mb"] = totalOS
		out["os_available_memory_mb"] = availOS
	}

	// Process memory low flags
	{
		q := `SELECT process_physical_memory_low, process_virtual_memory_low FROM sys.dm_os_process_memory WITH (NOLOCK);`
		var physLow, virtLow sql.NullBool
		_ = db.QueryRowContext(ctx, q).Scan(&physLow, &virtLow)
		out["process_physical_low"] = physLow.Valid && physLow.Bool
		out["process_virtual_low"] = virtLow.Valid && virtLow.Bool
	}

	// Memory grants pending
	{
		q := `
			SELECT ISNULL(cntr_value, 0)
			FROM sys.dm_os_performance_counters WITH (NOLOCK)
			WHERE counter_name='Memory Grants Pending';
		`
		var pending int64
		_ = db.QueryRowContext(ctx, q).Scan(&pending)
		out["memory_grants_pending"] = pending
	}

	// Active grants + workspace totals
	{
		q := `
			SELECT 
				COUNT(*) AS active_grants,
				ISNULL(SUM(granted_memory_kb), 0)/1024 AS granted_mb,
				ISNULL(SUM(requested_memory_kb), 0)/1024 AS requested_mb
			FROM sys.dm_exec_query_memory_grants WITH (NOLOCK);
		`
		var active, grantedMB, requestedMB int64
		_ = db.QueryRowContext(ctx, q).Scan(&active, &grantedMB, &requestedMB)
		out["active_memory_grants"] = active
		out["granted_workspace_mb"] = grantedMB
		out["requested_workspace_mb"] = requestedMB
	}

	// Waiting memory grants (grant_time IS NULL)
	{
		q := `
			SELECT COUNT(*)
			FROM sys.dm_exec_query_memory_grants WITH (NOLOCK)
			WHERE grant_time IS NULL;
		`
		var waiting int64
		_ = db.QueryRowContext(ctx, q).Scan(&waiting)
		out["waiting_memory_grants"] = waiting
	}

	// PLE
	{
		q := `
			SELECT ISNULL(cntr_value, 0)
			FROM sys.dm_os_performance_counters WITH (NOLOCK)
			WHERE counter_name='Page life expectancy';
		`
		var ple int64
		_ = db.QueryRowContext(ctx, q).Scan(&ple)
		out["ple_seconds"] = ple
	}

	// Plan cache size (MB)
	{
		q := `SELECT ISNULL(SUM(size_in_bytes)/1024/1024, 0) FROM sys.dm_exec_cached_plans WITH (NOLOCK);`
		var mb int64
		_ = db.QueryRowContext(ctx, q).Scan(&mb)
		out["plan_cache_mb"] = mb
	}

	// TempDB spill indicators (cumulative perf counters)
	{
		q := `
			SELECT 
				MAX(CASE WHEN counter_name='Sort Warnings' THEN cntr_value END) AS sort_warn,
				MAX(CASE WHEN counter_name='Hash Warnings' THEN cntr_value END) AS hash_warn
			FROM sys.dm_os_performance_counters WITH (NOLOCK)
			WHERE counter_name IN ('Sort Warnings', 'Hash Warnings');
		`
		var sortWarn, hashWarn sql.NullInt64
		_ = db.QueryRowContext(ctx, q).Scan(&sortWarn, &hashWarn)
		if sortWarn.Valid {
			out["sort_warnings_total"] = sortWarn.Int64
		} else {
			out["sort_warnings_total"] = int64(0)
		}
		if hashWarn.Valid {
			out["hash_warnings_total"] = hashWarn.Int64
		} else {
			out["hash_warnings_total"] = int64(0)
		}
	}

	return out, nil
}

// FetchBufferPoolByDB returns buffer pool consumption (MB) per database (top N).
func (c *MssqlRepository) FetchBufferPoolByDB(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	if limit <= 0 {
		limit = 20
	}
	q := fmt.Sprintf(`
		SELECT TOP (%d)
			DB_NAME(database_id) AS database_name,
			COUNT(*)*8/1024 AS buffer_mb
		FROM sys.dm_os_buffer_descriptors WITH (NOLOCK)
		GROUP BY database_id
		ORDER BY buffer_mb DESC;
	`, limit)

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var dbName sql.NullString
		var mb int64
		if err := rows.Scan(&dbName, &mb); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"database_name": dbName.String,
			"buffer_mb":     mb,
		})
	}
	return out, rows.Err()
}

