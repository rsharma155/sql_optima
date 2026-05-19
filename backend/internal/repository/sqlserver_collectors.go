// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server metrics collectors (Sessions, Blocking).
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

	"github.com/rsharma155/sql_optima/internal/models"
)

func CollectSessionSnapshot(ctx context.Context, db *sql.DB) ([]models.SQLServerSessionSnapshot, error) {
	now := time.Now().UTC()
	query := `
		/* SQL_OPTIMA */ SELECT   
			s.session_id,
			COALESCE(s.login_name, '') as login_name,
			COALESCE(s.original_login_name, '') as original_login_name,
			COALESCE(s.host_name, '') as host_name,
			DB_NAME(r.database_id) as database_name,
			COALESCE(s.program_name, '') as program_name,
			s.status,
			s.is_user_process,
			COALESCE(r.cpu_time, 0) as cpu_time_ms,
			COALESCE(r.total_elapsed_time, 0) as total_elapsed_time_ms,
			s.memory_usage as memory_usage_pages,
			s.reads,
			s.writes,
			s.logical_reads,
			COALESCE(r.open_transaction_count, 0) as open_transaction_count,
			COALESCE(r.wait_type, '') as wait_type,
			COALESCE(r.wait_time, 0) as wait_time_ms,
			COALESCE(r.last_wait_type, '') as last_wait_type,
			COALESCE(r.wait_resource, '') as wait_resource,
			COALESCE(r.blocking_session_id, 0) as blocking_session_id,
			CAST(r.query_hash as VARBINARY(8)) as query_hash,
			CAST(r.query_plan_hash as VARBINARY(8)) as query_plan_hash,
			COALESCE(st.text, '') as query_text
		FROM sys.dm_exec_sessions s WITH (NOLOCK)
		LEFT JOIN sys.dm_exec_requests r WITH (NOLOCK) ON s.session_id = r.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
		WHERE s.session_id > 50 AND s.session_id <> @@SPID
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SQLServerSessionSnapshot
	for rows.Next() {
		var s models.SQLServerSessionSnapshot
		var qh, qph []byte
		s.SampleTime = now
		s.CaptureTimestamp = now
		err := rows.Scan(
			&s.SessionID, &s.LoginName, &s.OriginalLoginName, &s.HostName, &s.DatabaseName, &s.ProgramName,
			&s.Status, &s.IsUserProcess, &s.CPUTimeMs, &s.TotalElapsedTimeMs, &s.MemoryUsagePages,
			&s.Reads, &s.Writes, &s.LogicalReads, &s.OpenTransactionCount,
			&s.WaitType, &s.WaitTimeMs, &s.LastWaitType, &s.WaitResource, &s.BlockingSessionID,
			&qh, &qph, &s.QueryText,
		)
		if err != nil {
			continue
		}
		s.QueryHash = fmt.Sprintf("0x%X", qh)
		s.QueryPlanHash = fmt.Sprintf("0x%X", qph)
		results = append(results, s)
	}
	return results, nil
}

func CollectLongRunningQueries(ctx context.Context, db *sql.DB) ([]models.LongRunningQuery, error) {
	query := `
		/* SQL_OPTIMA */ SELECT   
			r.session_id,
			r.request_id,
			DB_NAME(r.database_id) as database_name,
			s.login_name,
			s.host_name,
			s.program_name,
			r.wait_type,
			r.blocking_session_id,
			r.status,
			r.cpu_time as cpu_time_ms,
			r.total_elapsed_time as total_elapsed_time_ms,
			r.reads,
			r.writes,
			r.granted_query_memory * 8 / 1024 as granted_query_memory_mb,
			r.row_count,
			COALESCE(st.text, '') as query_text
		FROM sys.dm_exec_requests r WITH (NOLOCK)
		JOIN sys.dm_exec_sessions s WITH (NOLOCK) ON r.session_id = s.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
		WHERE r.total_elapsed_time > 10000 -- 10s
		  AND s.is_user_process = 1
	`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.LongRunningQuery
	for rows.Next() {
		var q models.LongRunningQuery
		err := rows.Scan(
			&q.SessionID, &q.RequestID, &q.DatabaseName, &q.LoginName, &q.HostName, &q.ProgramName,
			&q.WaitType, &q.BlockingSessionID, &q.Status, &q.CPUTimeMs,
			&q.TotalElapsedTimeMs, &q.Reads, &q.Writes, &q.GrantedQueryMemoryMB, &q.RowCount, &q.QueryText,
		)
		if err != nil {
			continue
		}
		results = append(results, q)
	}
	return results, nil
}

func CollectBlockingLocks(ctx context.Context, db *sql.DB) ([]models.BlockingNode, error) {
	query := `
		/* SQL_OPTIMA */ SELECT   
			r.session_id,
			r.blocking_session_id,
			s.login_name,
			s.host_name,
			s.program_name,
			DB_NAME(r.database_id) as database_name,
			COALESCE(st.text, '') as query_text,
			r.status,
			r.wait_type,
			r.wait_time as wait_time_ms,
			r.cpu_time as cpu_time_ms,
			r.total_elapsed_time as total_elapsed_time_ms,
			r.row_count
		FROM sys.dm_exec_requests r WITH (NOLOCK)
		JOIN sys.dm_exec_sessions s WITH (NOLOCK) ON r.session_id = s.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
		WHERE r.blocking_session_id <> 0
	`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.BlockingNode
	for rows.Next() {
		var n models.BlockingNode
		err := rows.Scan(
			&n.SessionID, &n.BlockingSessionID, &n.LoginName, &n.HostName, &n.ProgramName, &n.DatabaseName,
			&n.QueryText, &n.Status, &n.WaitType, &n.WaitTimeMs, &n.CPUTimeMs, &n.TotalElapsedTimeMs, &n.RowCount,
		)
		if err != nil {
			continue
		}
		results = append(results, n)
	}
	return results, nil
}

func (c *SqlServerRepository) CollectSessionSnapshotStaging(ctx context.Context, instanceName string) ([]models.SQLServerSessionSnapshot, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return CollectSessionSnapshot(ctx, db)
}

func CollectActiveQueries(ctx context.Context, db *sql.DB) ([]models.ActiveQuery, error) {
	query := `
		/* SQL_OPTIMA */ SELECT   
			r.session_id,
			r.request_id,
			DB_NAME(r.database_id) as database_name,
			s.login_name,
			s.host_name,
			s.program_name,
			COALESCE(st.text, '') as query_text,
			r.status,
			r.command,
			r.wait_type,
			r.wait_time as wait_time_ms,
			r.cpu_time as cpu_time_ms,
			r.total_elapsed_time as total_elapsed_time_ms,
			r.reads,
			r.writes,
			r.granted_query_memory * 8 / 1024 as granted_memory_mb,
			r.row_count,
			CAST(r.percent_complete as VARCHAR(10)) + '%' as percent_complete
		FROM sys.dm_exec_requests r WITH (NOLOCK)
		JOIN sys.dm_exec_sessions s WITH (NOLOCK) ON r.session_id = s.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
		WHERE s.is_user_process = 1 AND r.session_id <> @@SPID
	`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.ActiveQuery
	for rows.Next() {
		var q models.ActiveQuery
		err := rows.Scan(
			&q.SessionID, &q.RequestID, &q.DatabaseName, &q.LoginName, &q.HostName, &q.ProgramName,
			&q.QueryText, &q.Status, &q.Command, &q.WaitType, &q.WaitTimeMs, &q.CPUTimeMs,
			&q.TotalElapsedTimeMs, &q.Reads, &q.Writes, &q.GrantedMemoryMB, &q.RowCount, &q.PercentComplete,
		)
		if err != nil {
			continue
		}
		results = append(results, q)
	}
	return results, nil
}

func CollectCPUMemory(ctx context.Context, db *sql.DB) (*models.CPUTick, *models.MemoryStats, error) {
	// Re-implementation placeholder for refactor fix
	return &models.CPUTick{}, &models.MemoryStats{}, nil
}

func CollectWaitStats(ctx context.Context, db *sql.DB) ([]models.WaitStat, error) {
	// Re-implementation placeholder for refactor fix
	return []models.WaitStat{}, nil
}

func CollectStorageIO(ctx context.Context, db *sql.DB) ([]models.FileIOStat, *models.TempDBStats, error) {
	// Re-implementation placeholder for refactor fix
	return []models.FileIOStat{}, &models.TempDBStats{}, nil
}
