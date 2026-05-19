// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server memory metrics including clerk usage, memory grants, and buffer pool.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
)

// CollectMemoryMetrics fetches memory statistics from sys.dm_os_performance_counters and sys.dm_os_sys_memory
func (c *SqlServerRepository) CollectMemoryMetrics(ctx context.Context, db *sql.DB) (float64, error) {
	// Page Life Expectancy (PLE)
	pleQuery := `/* SQL_OPTIMA */ SELECT   cntr_value FROM sys.dm_os_performance_counters WHERE counter_name = 'Page life expectancy' AND object_name LIKE '%Buffer Manager%'`
	var ple float64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, pleQuery).Scan(&ple); err != nil {
		slog.Error("[SQLSERVER] PLE Query Error", "err", err)
	}

	// Buffer Pool Size
	bufQuery := `/* SQL_OPTIMA */ SELECT   cntr_value / 1024 FROM sys.dm_os_performance_counters WHERE counter_name = 'Buffer Pool Size (KB)'`
	var bufPoolSize float64
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, bufQuery).Scan(&bufPoolSize); err != nil {
		slog.Error("[SQLSERVER] Buffer Pool Query Error", "err", err)
	}

	// Memory Clerk Count
	clerkQuery := `/* SQL_OPTIMA */ SELECT   COUNT(DISTINCT memory_clerk_address) FROM sys.dm_os_memory_clerks`
	var clerkCount int
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, clerkQuery).Scan(&clerkCount); err != nil {
		slog.Error("[SQLSERVER] Memory Clerk Query Error", "err", err)
	}

	// Calculate memory usage percentage
	var memUsage float64 = 0
	memQuery := `
		/* SQL_OPTIMA */ SELECT   
			(CAST(total_physical_memory_kb AS FLOAT) - CAST(available_physical_memory_kb AS FLOAT)) / 
			CAST(total_physical_memory_kb AS FLOAT) * 100
		FROM sys.dm_os_sys_memory
	`
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, memQuery).Scan(&memUsage); err != nil {
		slog.Error("[SQLSERVER] Memory Usage Query Error", "err", err)
	}

	return memUsage, nil
}

// CollectMemoryClerks fetches memory clerk information
func (c *SqlServerRepository) CollectMemoryClerks(ctx context.Context, db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		/* SQL_OPTIMA */ SELECT   
			type AS clerk_name,
			memory_node_id AS memory_node,
			CAST(SUM(pages_kb) / 1024.0 AS FLOAT) AS pages_mb,
			CAST(SUM(virtual_memory_reserved_kb) / 1024.0 AS FLOAT) AS virtual_memory_reserved_mb,
			CAST(SUM(virtual_memory_committed_kb) / 1024.0 AS FLOAT) AS virtual_memory_committed_mb,
			CAST(SUM(awe_allocated_kb) / 1024.0 AS FLOAT) AS awe_memory_mb
		FROM sys.dm_os_memory_clerks
		GROUP BY type, memory_node_id
		HAVING SUM(pages_kb) > 1024
		ORDER BY pages_mb DESC
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var clerkType string
		var node int64
		var pagesMB, rsvMB, comMB, aweMB float64
		if err := rows.Scan(&clerkType, &node, &pagesMB, &rsvMB, &comMB, &aweMB); err == nil {
			results = append(results, map[string]interface{}{
				"clerk_name":                  clerkType,
				"memory_node":                 node,
				"pages_mb":                    pagesMB,
				"virtual_memory_reserved_mb":  rsvMB,
				"virtual_memory_committed_mb": comMB,
				"awe_memory_mb":               aweMB,
			})
		}
	}
	return results, nil
}

// CollectMemoryGrants fetches memory grant information
func (c *SqlServerRepository) CollectMemoryGrants(ctx context.Context, db *sql.DB) ([]map[string]interface{}, error) {
	// Shape matches Timescale LogMemoryGrants: user DBs only (database_id > 4), user sessions.
	query := `
		/* SQL_OPTIMA */ SELECT   TOP 20
			mg.session_id,
			mg.request_id,
			DB_NAME(ISNULL(r.database_id, s.database_id)) AS database_name,
			s.login_name,
			mg.granted_memory_kb,
			ISNULL(COALESCE(mg.used_memory_kb, mg.max_used_memory_kb), 0) AS used_memory_kb,
			ISNULL(r.dop, 1) AS dop,
			CASE WHEN r.start_time IS NOT NULL
				THEN DATEDIFF(SECOND, r.start_time, SYSDATETIME())
				ELSE 0 END AS query_duration_sec
		FROM sys.dm_exec_query_memory_grants mg WITH (NOLOCK)
		INNER JOIN sys.dm_exec_sessions s WITH (NOLOCK)
			ON mg.session_id = s.session_id
		LEFT JOIN sys.dm_exec_requests r WITH (NOLOCK)
			ON mg.session_id = r.session_id AND mg.request_id = r.request_id
		OUTER APPLY sys.dm_exec_sql_text(mg.sql_handle) txt
		WHERE mg.granted_memory_kb > 0
		  AND s.is_user_process = 1
		  AND ISNULL(r.database_id, s.database_id) > 4
		  AND LOWER(ISNULL(DB_NAME(ISNULL(r.database_id, s.database_id)), N'')) <> N'distribution'
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		  AND (
			txt.text IS NULL OR (
				LTRIM(txt.text) NOT LIKE N'sp\_%' ESCAPE '\'
				AND LTRIM(txt.text) NOT LIKE N'xp\_%' ESCAPE '\'
			)
		  )
		ORDER BY mg.granted_memory_kb DESC
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var resultsMap = make(map[string]interface{})
		columns, _ := rows.Columns()
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err == nil {
			for i, col := range columns {
				resultsMap[col] = values[i]
			}
			results = append(results, resultsMap)
		}
	}
	return results, nil
}
