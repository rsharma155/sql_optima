// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unified "Mega-Query" pulse methods for SQL Server to consolidate DMV hits.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SQLServerTier1Row represents a comprehensive session + request snapshot
type SQLServerTier1Row struct {
	SessionID             int
	LoginName             string
	HostName              string
	ProgramName           string
	ClientInterface       string
	SessionStatus         string
	DatabaseID            int
	DatabaseName          string
	OpenTransactionCount  int
	RequestStatus         sql.NullString
	Command               sql.NullString
	StartTime             sql.NullTime
	WaitType              sql.NullString
	WaitTimeMs            sql.NullInt64
	LastWaitType          sql.NullString
	BlockingSessionID     sql.NullInt64
	CPUTimeMs             sql.NullInt64
	ElapsedTimeMs         sql.NullInt64
	LogicalReads          sql.NullInt64
	Reads                 sql.NullInt64
	Writes                sql.NullInt64
	RowCount              sql.NullInt64
	GrantedMemory         sql.NullInt64
	SchedulerID           sql.NullInt64
	DOP                   sql.NullInt64
	ParallelWorkerCount   sql.NullInt64
	PercentComplete       sql.NullFloat64
	EstimatedCompletionMs sql.NullInt64
	TransactionIsolation  sql.NullInt64
	SQLHandle             sql.NullString
	PlanHandle            sql.NullString
	QueryHash             sql.NullString
	QueryPlanHash         sql.NullString
	StatementStartOffset  sql.NullInt64
	StatementEndOffset    sql.NullInt64
	CaptureTimestamp      time.Time
}

// FetchTier1MegaQuery executes the consolidated sessions/requests sweep (T1)
func (c *SqlServerRepository) FetchTier1MegaQuery(ctx context.Context, instanceName string) ([]SQLServerTier1Row, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
	/* SQL_OPTIMA PULSE T1 */
	SELECT
		s.session_id, s.login_name, s.host_name, s.program_name, s.client_interface_name,
		s.status, s.database_id, DB_NAME(s.database_id), s.open_transaction_count,
		r.status, r.command, r.start_time, r.wait_type, r.wait_time, r.last_wait_type,
		r.blocking_session_id, r.cpu_time, r.total_elapsed_time, r.logical_reads,
		r.reads, r.writes, r.row_count, r.granted_query_memory, r.scheduler_id,
		r.dop, r.parallel_worker_count, r.percent_complete, r.estimated_completion_time,
		r.transaction_isolation_level,
		CONVERT(VARCHAR(MAX), s.sql_handle, 1), CONVERT(VARCHAR(MAX), r.plan_handle, 1),
		CONVERT(VARCHAR(MAX), r.query_hash, 1), CONVERT(VARCHAR(MAX), r.query_plan_hash, 1),
		r.statement_start_offset, r.statement_end_offset,
		SYSUTCDATETIME()
	FROM sys.dm_exec_sessions s
	LEFT JOIN sys.dm_exec_requests r ON s.session_id = r.session_id
	WHERE s.is_user_process = 1;`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerTier1Row
	for rows.Next() {
		var r SQLServerTier1Row
		err := rows.Scan(
			&r.SessionID, &r.LoginName, &r.HostName, &r.ProgramName, &r.ClientInterface,
			&r.SessionStatus, &r.DatabaseID, &r.DatabaseName, &r.OpenTransactionCount,
			&r.RequestStatus, &r.Command, &r.StartTime, &r.WaitType, &r.WaitTimeMs, &r.LastWaitType,
			&r.BlockingSessionID, &r.CPUTimeMs, &r.ElapsedTimeMs, &r.LogicalReads,
			&r.Reads, &r.Writes, &r.RowCount, &r.GrantedMemory, &r.SchedulerID,
			&r.DOP, &r.ParallelWorkerCount, &r.PercentComplete, &r.EstimatedCompletionMs,
			&r.TransactionIsolation, &r.SQLHandle, &r.PlanHandle, &r.QueryHash, &r.QueryPlanHash,
			&r.StatementStartOffset, &r.StatementEndOffset, &r.CaptureTimestamp,
		)
		if err == nil {
			results = append(results, r)
		}
	}
	return results, rows.Err()
}

// SQLServerTier2Row represents a KPI/Perf/Memory metric
type SQLServerTier2Row struct {
	Category         string
	MetricName       string
	MetricValue      int64
	Unit             string
	InstanceName     string
	CaptureTimestamp time.Time
}

// FetchTier2MegaQuery executes the consolidated performance/memory sweep (T2)
func (c *SqlServerRepository) FetchTier2MegaQuery(ctx context.Context, instanceName string) ([]SQLServerTier2Row, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
	/* SQL_OPTIMA PULSE T2 */
	;WITH PerfCounters AS (
		SELECT 'PERF' AS category, counter_name, CAST(cntr_value AS BIGINT) AS val, instance_name, 'raw' as unit
		FROM sys.dm_os_performance_counters
		WHERE counter_name IN ('Batch Requests/sec', 'SQL Compilations/sec', 'Page life expectancy', 'User Connections')
	),
	SystemMem AS (
		SELECT 'SYS_MEM' AS category, m.n, CAST(m.v AS BIGINT), '', 'KB'
		FROM sys.dm_os_sys_memory sm
		CROSS APPLY (VALUES ('Total Physical Memory', sm.total_physical_memory_kb), ('Available Physical Memory', sm.available_physical_memory_kb)) m(n,v)
	)
	SELECT category, counter_name, val, unit, instance_name, SYSUTCDATETIME() FROM (SELECT * FROM PerfCounters UNION ALL SELECT * FROM SystemMem) x`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerTier2Row
	for rows.Next() {
		var r SQLServerTier2Row
		if err := rows.Scan(&r.Category, &r.MetricName, &r.MetricValue, &r.Unit, &r.InstanceName, &r.CaptureTimestamp); err == nil {
			results = append(results, r)
		}
	}
	return results, rows.Err()
}

// SQLServerIORow represents file-level IO stats
type SQLServerIORow struct {
	DatabaseID        int
	DatabaseName      string
	FileID            int
	TypeDesc          string
	PhysicalName      string
	NumOfReads        int64
	NumOfWrites       int64
	NumOfBytesRead    int64
	NumOfBytesWritten int64
	IoStallReadMs     int64
	IoStallWriteMs    int64
	IoStall           int64
	SizeOnDiskBytes   int64
	CaptureTimestamp  time.Time
}

// FetchTier3MegaQueryIO executes the consolidated IO sweep
func (c *SqlServerRepository) FetchTier3MegaQueryIO(ctx context.Context, instanceName string) ([]SQLServerIORow, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
	/* SQL_OPTIMA PULSE T3 IO */
	SELECT
		vfs.database_id, DB_NAME(vfs.database_id), vfs.file_id,
		mf.type_desc, mf.physical_name, vfs.num_of_reads, vfs.num_of_writes,
		vfs.num_of_bytes_read, vfs.num_of_bytes_written, vfs.io_stall_read_ms,
		vfs.io_stall_write_ms, vfs.io_stall, vfs.size_on_disk_bytes,
		SYSUTCDATETIME()
	FROM sys.dm_io_virtual_file_stats(NULL, NULL) vfs
	JOIN sys.master_files mf ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id
	WHERE vfs.database_id > 4;`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerIORow
	for rows.Next() {
		var r SQLServerIORow
		err := rows.Scan(
			&r.DatabaseID, &r.DatabaseName, &r.FileID, &r.TypeDesc, &r.PhysicalName,
			&r.NumOfReads, &r.NumOfWrites, &r.NumOfBytesRead, &r.NumOfBytesWritten,
			&r.IoStallReadMs, &r.IoStallWriteMs, &r.IoStall, &r.SizeOnDiskBytes,
			&r.CaptureTimestamp,
		)
		if err == nil {
			results = append(results, r)
		}
	}
	return results, rows.Err()
}

// SQLServerMemoryGrantRow represents a query memory grant snapshot
type SQLServerMemoryGrantRow struct {
	SessionID         int
	RequestedMemoryKB int64
	GrantedMemoryKB   sql.NullInt64
	UsedMemoryKB      sql.NullInt64
	MaxUsedMemoryKB   sql.NullInt64
	RequestTime       time.Time
	GrantTime         sql.NullTime
	QueryCost         float64
	Status            string
	WaitType          sql.NullString
	BlockingSessionID sql.NullInt64
	DatabaseName      string
	CaptureTimestamp  time.Time
}

// FetchTier3MegaQueryMemoryGrants executes the memory grant sweep
func (c *SqlServerRepository) FetchTier3MegaQueryMemoryGrants(ctx context.Context, instanceName string) ([]SQLServerMemoryGrantRow, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
	/* SQL_OPTIMA PULSE T3 MEMORY */
	SELECT
		mg.session_id, mg.requested_memory_kb, mg.granted_memory_kb, mg.used_memory_kb,
		mg.max_used_memory_kb, mg.request_time, mg.grant_time, mg.query_cost,
		r.status, r.wait_type, r.blocking_session_id, DB_NAME(r.database_id),
		SYSUTCDATETIME()
	FROM sys.dm_exec_query_memory_grants mg
	LEFT JOIN sys.dm_exec_requests r ON mg.session_id = r.session_id;`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerMemoryGrantRow
	for rows.Next() {
		var r SQLServerMemoryGrantRow
		err := rows.Scan(
			&r.SessionID, &r.RequestedMemoryKB, &r.GrantedMemoryKB, &r.UsedMemoryKB,
			&r.MaxUsedMemoryKB, &r.RequestTime, &r.GrantTime, &r.QueryCost,
			&r.Status, &r.WaitType, &r.BlockingSessionID, &r.DatabaseName,
			&r.CaptureTimestamp,
		)
		if err == nil {
			results = append(results, r)
		}
	}
	return results, rows.Err()
}

// SQLServerLockRow represents an aggregated lock snapshot
type SQLServerLockRow struct {
	DatabaseName     string
	TotalLocks       int64
	ConvertLocks     int64
	CaptureTimestamp time.Time
}

// FetchTier1MegaQueryLocks executes the consolidated lock sweep
func (c *SqlServerRepository) FetchTier1MegaQueryLocks(ctx context.Context, instanceName string) ([]SQLServerLockRow, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
	/* SQL_OPTIMA PULSE T1 LOCKS */
	SELECT
		ISNULL(DB_NAME(resource_database_id), 'Unknown'),
		COUNT(*),
		SUM(CASE WHEN request_status = 'CONVERT' THEN 1 ELSE 0 END),
		SYSUTCDATETIME()
	FROM sys.dm_tran_locks WITH (NOLOCK)
	WHERE resource_database_id > 0
	GROUP BY resource_database_id`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerLockRow
	for rows.Next() {
		var r SQLServerLockRow
		if err := rows.Scan(&r.DatabaseName, &r.TotalLocks, &r.ConvertLocks, &r.CaptureTimestamp); err == nil {
			results = append(results, r)
		}
	}
	return results, rows.Err()
}
