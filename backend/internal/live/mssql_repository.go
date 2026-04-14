// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Live SQL Server repository for real-time query monitoring and KPI calculations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package live

import (
	"context"
	"fmt"
	"strings"
)

type MsSQLLiveRepository struct {
	getDB func(instanceName string) interface{}
}

func NewMsSQLLiveRepository(getDB func(instanceName string) interface{}) *MsSQLLiveRepository {
	return &MsSQLLiveRepository{getDB: getDB}
}

type KPIsResult struct {
	ActiveSessions    int `json:"active_sessions"`
	TotalMemoryMB     int `json:"total_memory_mb"`
	AvailableMemoryMB int `json:"available_memory_mb"`
	BatchRequestsSec  int `json:"batch_requests_sec"`
}

func (r *MsSQLLiveRepository) GetKPIs(ctx context.Context, instanceName string) QueryResult {
	query := `
		SELECT 
			(SELECT COUNT(*) FROM sys.dm_exec_sessions WHERE status='running') AS active_sessions,
			(SELECT total_physical_memory_kb/1024 FROM sys.dm_os_sys_memory) AS total_memory_mb,
			(SELECT available_physical_memory_kb/1024 FROM sys.dm_os_sys_memory) AS available_memory_mb,
			(SELECT cntr_value FROM sys.dm_os_performance_counters WHERE counter_name='Batch Requests/sec') AS batch_requests_sec
		OPTION (RECOMPILE)`

	return ExecuteQuery(ctx, instanceName, query, r.getDB)
}

func (r *MsSQLLiveRepository) GetRunningQueries(ctx context.Context, instanceName string) QueryResult {
	query := `
		SELECT TOP 50
			r.session_id, 
			s.login_name, 
			s.program_name, 
			r.status, 
			r.cpu_time, 
			r.total_elapsed_time,
			r.logical_reads, 
			r.wait_type, 
			r.blocking_session_id,
			SUBSTRING(t.text, (r.statement_start_offset/2)+1, 
				((CASE r.statement_end_offset WHEN -1 THEN DATALENGTH(t.text) ELSE r.statement_end_offset END - r.statement_start_offset)/2)+1) AS query_text
		FROM sys.dm_exec_requests r
		INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE s.is_user_process = 1 AND r.session_id <> @@SPID
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		ORDER BY r.cpu_time DESC 
		OPTION (RECOMPILE)`

	return ExecuteQuery(ctx, instanceName, query, r.getDB)
}

func (r *MsSQLLiveRepository) GetBlockingChains(ctx context.Context, instanceName string) QueryResult {
	query := `
		WITH BlockingTree AS (
			SELECT r.session_id AS Blocked_SPID, r.blocking_session_id AS Blocking_SPID, r.wait_type, r.wait_time
			FROM sys.dm_exec_requests r
			INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
			WHERE r.blocking_session_id <> 0
			  AND s.is_user_process = 1
			  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
			  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		)
		SELECT 
			b.Blocking_SPID AS Lead_Blocker, 
			s.login_name AS Blocker_Login, 
			s.program_name AS Blocker_App,
			COUNT(b.Blocked_SPID) AS Total_Victims, 
			MAX(b.wait_time) AS Max_Wait_Time_ms
		FROM BlockingTree b
		INNER JOIN sys.dm_exec_sessions s ON b.Blocking_SPID = s.session_id
		GROUP BY b.Blocking_SPID, s.login_name, s.program_name
		OPTION (RECOMPILE)`

	return ExecuteQuery(ctx, instanceName, query, r.getDB)
}

func (r *MsSQLLiveRepository) GetIOLatency(ctx context.Context, instanceName string) QueryResult {
	query := `
		SELECT 
			DB_NAME(database_id) AS database_name, 
			file_id,
			io_stall_read_ms/NULLIF(num_of_reads,0) AS read_latency_ms,
			io_stall_write_ms/NULLIF(num_of_writes,0) AS write_latency_ms
		FROM sys.dm_io_virtual_file_stats(NULL,NULL)
		OPTION (RECOMPILE)`

	return ExecuteQuery(ctx, instanceName, query, r.getDB)
}

func (r *MsSQLLiveRepository) GetTempDBUsage(ctx context.Context, instanceName string) QueryResult {
	query := `
		SELECT 
			SUM(user_object_reserved_page_count)*8/1024 AS user_mb,
			SUM(internal_object_reserved_page_count)*8/1024 AS internal_mb,
			SUM(version_store_reserved_page_count)*8/1024 AS version_store_mb
		FROM sys.dm_db_file_space_usage
		OPTION (RECOMPILE)`

	return ExecuteQuery(ctx, instanceName, query, r.getDB)
}

func (r *MsSQLLiveRepository) GetWaitStats(ctx context.Context, instanceName string) QueryResult {
	query := `
		SELECT TOP 10 
			wait_type, 
			waiting_tasks_count, 
			wait_time_ms
		FROM sys.dm_os_wait_stats 
		ORDER BY wait_time_ms DESC 
		OPTION (RECOMPILE)`

	return ExecuteQuery(ctx, instanceName, query, r.getDB)
}

func (r *MsSQLLiveRepository) GetConnectionsByApp(ctx context.Context, instanceName string) QueryResult {
	query := `
		SELECT 
			program_name, 
			COUNT(*) AS connection_count,
			COUNT(DISTINCT login_name) AS unique_logins
		FROM sys.dm_exec_sessions 
		WHERE is_user_process = 1
		  AND LOWER(ISNULL(login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		GROUP BY program_name 
		ORDER BY connection_count DESC
		OPTION (RECOMPILE)`

	return ExecuteQuery(ctx, instanceName, query, r.getDB)
}

func ExecuteQuery(ctx context.Context, instanceName string, query string, getDB func(instanceName string) interface{}) QueryResult {
	dbInterface := getDB(instanceName)
	if dbInterface == nil {
		return QueryResult{
			Success: false,
			Error: &QueryError{
				Code:    "CONNECTION_NOT_FOUND",
				Message: fmt.Sprintf("No database connection found for instance: %s", instanceName),
				Timeout: false,
			},
		}
	}

	db, ok := dbInterface.(*interface{})
	if !ok {
		return QueryResult{
			Success: false,
			Error: &QueryError{
				Code:    "INVALID_CONNECTION",
				Message: "Invalid database connection type",
				Timeout: false,
			},
		}
	}

	_ = ctx
	_ = query
	_ = db
	_ = strings.TrimSpace(query)

	return QueryResult{
		Success: false,
		Error: &QueryError{
			Code:    "NOT_IMPLEMENTED",
			Message: "Database driver not properly integrated",
			Timeout: false,
		},
	}
}
