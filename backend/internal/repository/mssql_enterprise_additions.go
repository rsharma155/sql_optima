// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Enterprise-tier SQL Server metrics including additional performance indicators.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
)

// FetchPlanCacheHealth returns plan cache size and single-use plan pressure.
func (c *MssqlRepository) FetchPlanCacheHealth(instanceName string) (map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectPlanCacheHealth(db)
}

func (c *MssqlRepository) CollectPlanCacheHealth(db *sql.DB) (map[string]interface{}, error) {
	// Cache sizes are pages*8KB; convert to MB.
	// single-use pressure is a common DBA signal for "Optimize for ad hoc workloads" / parameterization.
	q := `
		WITH plans AS (
			SELECT
				objtype,
				usecounts,
				size_in_bytes
			FROM sys.dm_exec_cached_plans WITH (NOLOCK)
		),
		agg AS (
			SELECT
				SUM(size_in_bytes) / 1048576.0 AS total_cache_mb,
				SUM(CASE WHEN usecounts = 1 THEN size_in_bytes ELSE 0 END) / 1048576.0 AS single_use_cache_mb,
				SUM(CASE WHEN objtype = 'Adhoc' THEN size_in_bytes ELSE 0 END) / 1048576.0 AS adhoc_cache_mb,
				SUM(CASE WHEN objtype = 'Prepared' THEN size_in_bytes ELSE 0 END) / 1048576.0 AS prepared_cache_mb,
				SUM(CASE WHEN objtype = 'Proc' THEN size_in_bytes ELSE 0 END) / 1048576.0 AS proc_cache_mb
			FROM plans
		)
		SELECT
			ISNULL(total_cache_mb, 0),
			ISNULL(single_use_cache_mb, 0),
			CASE WHEN ISNULL(total_cache_mb,0) > 0 THEN (ISNULL(single_use_cache_mb,0) / total_cache_mb) * 100.0 ELSE 0 END,
			ISNULL(adhoc_cache_mb, 0),
			ISNULL(prepared_cache_mb, 0),
			ISNULL(proc_cache_mb, 0)
		FROM agg;
	`
	var total, single, pct, adhoc, prep, proc float64
	if err := db.QueryRow(q).Scan(&total, &single, &pct, &adhoc, &prep, &proc); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"total_cache_mb":        total,
		"single_use_cache_mb":   single,
		"single_use_cache_pct":  pct,
		"adhoc_cache_mb":        adhoc,
		"prepared_cache_mb":     prep,
		"proc_cache_mb":         proc,
	}, nil
}

// FetchMemoryGrantWaiters returns queries waiting for a memory grant.
func (c *MssqlRepository) FetchMemoryGrantWaiters(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectMemoryGrantWaiters(db)
}

func (c *MssqlRepository) CollectMemoryGrantWaiters(db *sql.DB) ([]map[string]interface{}, error) {
	// User databases only (database_id > 4), user sessions, exclude typical system sp_/xp_ batches.
	q := `
		SELECT TOP 20
			mg.session_id,
			mg.request_id,
			DB_NAME(ISNULL(r.database_id, s.database_id)) AS database_name,
			s.login_name,
			mg.requested_memory_kb,
			mg.granted_memory_kb,
			mg.required_memory_kb,
			mg.wait_time_ms,
			ISNULL(r.dop, 1) AS dop,
			SUBSTRING(txt.text, 1, 4000) AS query_text
		FROM sys.dm_exec_query_memory_grants mg WITH (NOLOCK)
		INNER JOIN sys.dm_exec_sessions s WITH (NOLOCK)
			ON mg.session_id = s.session_id
		LEFT JOIN sys.dm_exec_requests r WITH (NOLOCK)
			ON mg.session_id = r.session_id AND mg.request_id = r.request_id
		OUTER APPLY sys.dm_exec_sql_text(mg.sql_handle) txt
		WHERE mg.grant_time IS NULL
		  AND s.is_user_process = 1
		  AND ISNULL(r.database_id, s.database_id) > 4
		  AND LOWER(ISNULL(DB_NAME(ISNULL(r.database_id, s.database_id)), N'')) <> N'distribution'
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND (
			txt.text IS NULL OR (
				LTRIM(txt.text) NOT LIKE N'sp\_%' ESCAPE '\'
				AND LTRIM(txt.text) NOT LIKE N'xp\_%' ESCAPE '\'
			)
		  )
		ORDER BY mg.wait_time_ms DESC;
	`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var sid, rid, reqKB, grKB, needKB, waitMs, dop interface{}
		var dbName, login, qtxt interface{}
		if err := rows.Scan(&sid, &rid, &dbName, &login, &reqKB, &grKB, &needKB, &waitMs, &dop, &qtxt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"session_id":           sid,
			"request_id":           rid,
			"database_name":        dbName,
			"login_name":           login,
			"requested_memory_kb":  reqKB,
			"granted_memory_kb":    grKB,
			"required_memory_kb":   needKB,
			"wait_time_ms":         waitMs,
			"dop":                  dop,
			"query_text":           qtxt,
		})
	}
	return out, rows.Err()
}

// FetchTempdbTopConsumers returns sessions currently consuming tempdb space.
func (c *MssqlRepository) FetchTempdbTopConsumers(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectTempdbTopConsumers(db)
}

func (c *MssqlRepository) CollectTempdbTopConsumers(db *sql.DB) ([]map[string]interface{}, error) {
	// dm_db_task_space_usage is per-task; aggregate by session_id to find top consumers.
	// Page counts are 8KB pages -> MB = pages*8/1024.
	q := `
		WITH usage AS (
			SELECT
				tsu.session_id,
				SUM(tsu.user_objects_alloc_page_count - tsu.user_objects_dealloc_page_count) AS user_pages,
				SUM(tsu.internal_objects_alloc_page_count - tsu.internal_objects_dealloc_page_count) AS internal_pages
			FROM tempdb.sys.dm_db_task_space_usage tsu WITH (NOLOCK)
			GROUP BY tsu.session_id
		)
		SELECT TOP 20
			u.session_id,
			DB_NAME(r.database_id) AS database_name,
			s.login_name,
			s.host_name,
			s.program_name,
			((u.user_pages + u.internal_pages) * 8.0) / 1024.0 AS tempdb_mb,
			(u.user_pages * 8.0) / 1024.0 AS user_objects_mb,
			(u.internal_pages * 8.0) / 1024.0 AS internal_objects_mb,
			SUBSTRING(txt.text, 1, 4000) AS query_text
		FROM usage u
		LEFT JOIN sys.dm_exec_requests r WITH (NOLOCK)
			ON u.session_id = r.session_id
		LEFT JOIN sys.dm_exec_sessions s WITH (NOLOCK)
			ON u.session_id = s.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) txt
		WHERE (u.user_pages + u.internal_pages) > 0
		  AND u.session_id > 50
		  AND s.is_user_process = 1
		  AND ISNULL(r.database_id, s.database_id) > 4
		  AND LOWER(ISNULL(DB_NAME(ISNULL(r.database_id, s.database_id)), N'')) <> N'distribution'
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND (
			txt.text IS NULL OR (
				LTRIM(txt.text) NOT LIKE N'sp\_%' ESCAPE '\'
				AND LTRIM(txt.text) NOT LIKE N'xp\_%' ESCAPE '\'
			)
		  )
		ORDER BY tempdb_mb DESC;
	`
	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[MSSQL] TempdbTopConsumers query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var sid interface{}
		var dbName, login, host, program interface{}
		var totalMB, userMB, internalMB interface{}
		var qtxt interface{}
		if err := rows.Scan(&sid, &dbName, &login, &host, &program, &totalMB, &userMB, &internalMB, &qtxt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"session_id":          sid,
			"database_name":       dbName,
			"login_name":          login,
			"host_name":           host,
			"program_name":        program,
			"tempdb_mb":           totalMB,
			"user_objects_mb":     userMB,
			"internal_objects_mb": internalMB,
			"query_text":          qtxt,
		})
	}
	return out, rows.Err()
}

