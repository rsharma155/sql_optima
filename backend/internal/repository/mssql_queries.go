package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// CollectLongRunningQueries fetches currently running long queries from dm_exec_requests
func (c *MssqlRepository) CollectLongRunningQueries(ctx context.Context, db *sql.DB, minDurationMs int) ([]map[string]interface{}, error) {
	if minDurationMs <= 0 {
		minDurationMs = 5000
	}

	query := fmt.Sprintf(`
		SELECT TOP 50
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
		AND s.login_name NOT IN ('dbmonitor_user', 'go-mssqldb')
		AND s.program_name NOT IN ('dbmonitor_user', 'go-mssqldb')
		ORDER BY r.total_elapsed_time DESC`, minDurationMs)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[MSSQL] CollectLongRunningQueries Error: %v", err)
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

// CollectLiveRunningQueries fetches currently running queries
func (c *MssqlRepository) CollectLiveRunningQueries(ctx context.Context, db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT TOP 50
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
		AND s.login_name NOT IN ('dbmonitor_user', 'go-mssqldb')
		AND s.program_name NOT IN ('dbmonitor_user', 'go-mssqldb')
		ORDER BY r.total_elapsed_time DESC`

	rows, err := db.QueryContext(ctx, query)
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

// CollectTopQueries fetches top CPU-consuming queries from sys.dm_exec_query_stats
func (c *MssqlRepository) CollectTopQueries(db *sql.DB, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf(`
		SELECT TOP %d
			ISNULL(DB_NAME(pa.dbid), ISNULL(DB_NAME(qt.dbid), 'Unknown')) AS Database_Name,
			ISNULL(s.login_name, 'Unknown/Disconnected') AS Login_Name,
			ISNULL(s.program_name, 'Unknown/Disconnected') AS Client_App,
			qs.execution_count AS Total_Executions,
			qs.total_worker_time / 1000.0 AS Total_CPU_ms,
			qs.total_logical_reads AS Total_Logical_Reads,
			qs.total_elapsed_time / 1000.0 AS Total_Elapsed_Time_ms,
			CASE 
				WHEN qt.objectid IS NOT NULL AND OBJECT_NAME(qt.objectid, qt.dbid) IS NOT NULL 
				THEN 'EXEC ' + QUOTENAME(ISNULL(OBJECT_SCHEMA_NAME(qt.objectid, qt.dbid), 'dbo')) + '.' + QUOTENAME(OBJECT_NAME(qt.objectid, qt.dbid))
				ELSE SUBSTRING(qt.text, (qs.statement_start_offset/2) + 1,
					((CASE qs.statement_end_offset 
						WHEN -1 THEN DATALENGTH(qt.text) 
						ELSE qs.statement_end_offset 
					END - qs.statement_start_offset)/2) + 1)
			END AS Query_Text,
			CONVERT(VARCHAR(64), qs.query_hash, 1) AS Query_Hash
		FROM sys.dm_exec_query_stats qs
		CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) qt
		OUTER APPLY (
			SELECT CAST(value AS INT) AS dbid 
			FROM sys.dm_exec_plan_attributes(qs.plan_handle) 
			WHERE attribute = 'dbid'
		) pa
		OUTER APPLY (
			SELECT TOP 1 ses.login_name, ses.program_name
			FROM sys.dm_exec_connections con
			INNER JOIN sys.dm_exec_sessions ses ON con.session_id = ses.session_id
			WHERE con.most_recent_sql_handle = qs.sql_handle
		) s
		WHERE qt.text IS NOT NULL
		  AND qt.text NOT LIKE '%%sys.dm_exec_query_stats%%'
		  AND qt.text NOT LIKE '%%SQLOptima%%'
		  AND qt.text NOT LIKE '%%DeltaCollector%%'
		  AND qs.query_hash IS NOT NULL
		AND qs.last_execution_time >= DATEADD(minute, -5, GETDATE())
		  AND (pa.dbid > 4 OR qt.dbid > 4)
		ORDER BY qs.total_worker_time DESC`, limit)

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] Top Queries Query Error: %v", err)
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

// CollectProcedureStats fetches stored procedure execution stats
func (c *MssqlRepository) CollectProcedureStats(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT TOP 20 
			DB_NAME(database_id) AS database_name,
			OBJECT_SCHEMA_NAME(object_id, database_id) AS schema_name,
			OBJECT_NAME(object_id, database_id) AS object_name,
			SUM(execution_count) AS execution_count,
			SUM(total_worker_time) / 1000.0 AS total_worker_time_ms,
			SUM(total_elapsed_time) / 1000.0 AS total_elapsed_time_ms,
			SUM(total_logical_reads) AS total_logical_reads,
			SUM(total_physical_reads) AS total_physical_reads
		FROM sys.dm_exec_procedure_stats
		WHERE database_id > 4 AND object_id > 0
		GROUP BY object_id, database_id
		ORDER BY SUM(total_worker_time) DESC
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
