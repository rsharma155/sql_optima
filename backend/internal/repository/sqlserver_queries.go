// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Query performance data from plan cache and query stats.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// CollectLongRunningQueries fetches currently running long queries from dm_exec_requests
func (c *SqlServerRepository) CollectLongRunningQueries(ctx context.Context, db *sql.DB, minDurationMs int) ([]map[string]interface{}, error) {
	if minDurationMs <= 0 {
		minDurationMs = 5000
	}

	query := fmt.Sprintf(`
		SELECT /* SQL_OPTIMA */   TOP 50
			r.session_id,
			r.request_id,
			DB_NAME(r.database_id) AS database_name,
			s.login_name,
			s.host_name,
			s.program_name,
			CASE 
				WHEN qp.objectid IS NOT NULL 
				THEN QUOTENAME(OBJECT_SCHEMA_NAME(qp.objectid, r.database_id)) 
					 + '.' + QUOTENAME(OBJECT_NAME(qp.objectid, r.database_id))
				ELSE
					SUBSTRING(
						qt.text,
						(r.statement_start_offset/2) + 1,
						(
							CASE r.statement_end_offset
								WHEN -1 THEN DATALENGTH(qt.text)
								ELSE r.statement_end_offset
							END - r.statement_start_offset
						) / 2 + 1
					)
			END AS query_text,
			r.wait_type,
			r.blocking_session_id,
			r.status,
			r.cpu_time AS cpu_time_ms,
			r.total_elapsed_time AS total_elapsed_time_ms,
			r.reads,
			r.writes,
			(r.granted_query_memory * 8) / 1024 AS granted_query_memory_mb,
			r.row_count
		FROM sys.dm_exec_requests r
		JOIN sys.dm_exec_sessions s 
			ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) qt
		OUTER APPLY sys.dm_exec_query_plan(r.plan_handle) qp
		WHERE r.session_id <> @@SPID AND r.session_id > 50
		AND r.total_elapsed_time >= %d
		AND s.is_user_process = 1
		AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		ORDER BY r.total_elapsed_time DESC`, minDurationMs)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[SQLSERVER] CollectLongRunningQueries Error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	cols, _ := rows.Columns()
	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}
		if err := rows.Scan(columnPointers...); err != nil {
			continue
		}
		rowMap := make(map[string]interface{})
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			switch v := (*val).(type) {
			case []byte:
				rowMap[colName] = string(v)
			case nil:
				rowMap[colName] = nil
			default:
				rowMap[colName] = v
			}
		}
		results = append(results, rowMap)
	}
	return results, nil
}

// CollectLiveRunningQueries fetches currently running queries (Real-Time Diagnostics).
// If database is non-empty, scopes to that DB only.
func (c *SqlServerRepository) CollectLiveRunningQueries(ctx context.Context, db *sql.DB, database string) ([]map[string]interface{}, error) {
	query := `
		SELECT /* SQL_OPTIMA */   TOP 50
			r.session_id, 
			s.login_name, 
			s.program_name, 
			r.status, 
			r.cpu_time, 
			r.total_elapsed_time,
			r.logical_reads, 
			ISNULL(r.wait_type, 'RUNNING') AS wait_type, 
			r.blocking_session_id,
			SUBSTRING(t.text, (r.statement_start_offset/2)+1, 
				((CASE r.statement_end_offset WHEN -1 THEN DATALENGTH(t.text) ELSE r.statement_end_offset END - r.statement_start_offset)/2)+1) AS query_text
		FROM sys.dm_exec_requests r
		JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE r.session_id > 50 AND s.is_user_process = 1
		AND s.database_id > 4
		AND LOWER(ISNULL(DB_NAME(s.database_id), '')) <> 'distribution'
		AND (@p1 = '' OR DB_NAME(s.database_id) = @p1)
		AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		ORDER BY r.total_elapsed_time DESC`

	rows, err := db.QueryContext(ctx, query, strings.TrimSpace(database))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	cols, _ := rows.Columns()
	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}
		if err := rows.Scan(columnPointers...); err != nil {
			continue
		}
		rowMap := make(map[string]interface{})
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			switch v := (*val).(type) {
			case []byte:
				rowMap[colName] = string(v)
			case nil:
				rowMap[colName] = nil
			default:
				rowMap[colName] = v
			}
		}
		results = append(results, rowMap)
	}
	return results, nil
}

func (c *SqlServerRepository) CollectProcedureStats(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT /* SQL_OPTIMA */   TOP 20 
			DB_NAME(ps.database_id) AS database_name,
			OBJECT_SCHEMA_NAME(ps.object_id, ps.database_id) AS schema_name,
			OBJECT_NAME(ps.object_id, ps.database_id) AS object_name,
			SUM(ps.execution_count) AS execution_count,
			SUM(ps.total_worker_time) / 1000.0 AS total_worker_time_ms,
			SUM(ps.total_elapsed_time) / 1000.0 AS total_elapsed_time_ms,
			SUM(ps.total_logical_reads) AS total_logical_reads,
			SUM(ps.total_physical_reads) AS total_physical_reads
		FROM sys.dm_exec_procedure_stats ps
		WHERE ps.database_id > 4 AND ps.object_id > 0
		  AND LOWER(ISNULL(DB_NAME(ps.database_id), N'')) <> N'distribution'
		  AND OBJECT_NAME(ps.object_id, ps.database_id) NOT LIKE N'sp[_]%' ESCAPE '\'
		  AND OBJECT_NAME(ps.object_id, ps.database_id) NOT LIKE N'xp[_]%' ESCAPE '\'
		GROUP BY ps.object_id, ps.database_id
		ORDER BY SUM(ps.total_worker_time) DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dbName, schema, obj interface{}
		var execCount, totalWorker, totalElapsed, totalReads, totalPhysical interface{}
		if err := rows.Scan(&dbName, &schema, &obj, &execCount, &totalWorker, &totalElapsed, &totalReads, &totalPhysical); err == nil {
			results = append(results, map[string]interface{}{
				"database_name":         dbName,
				"schema_name":           schema,
				"object_name":           obj,
				"execution_count":       execCount,
				"total_worker_time_ms":  totalWorker,
				"total_elapsed_time_ms": totalElapsed,
				"total_logical_reads":   totalReads,
				"total_physical_reads":  totalPhysical,
			})
		}
	}
	return results, nil
}
