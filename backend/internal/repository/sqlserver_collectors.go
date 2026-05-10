// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Low-level SQL Server collection functions relocated from collectors to repository to break import cycles.
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
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectActiveQueries returns currently executing queries (Top 50 live queries)
func CollectActiveQueries(ctx context.Context, db *sql.DB) ([]models.ActiveQuery, error) {
	query := ` /* SQL_OPTIMA */ 
		SELECT TOP 50
			r.session_id,
			r.request_id,
			ISNULL(DB_NAME(r.database_id), 'Unknown') AS database_name,
			ISNULL(s.login_name, 'Unknown') AS login_name,
			ISNULL(s.host_name, 'Unknown') AS host_name,
			ISNULL(s.program_name, 'Unknown') AS program_name,
			ISNULL(LEFT(t.text, 2000), 'N/A') AS query_text,
			r.status,
			r.command,
			ISNULL(r.wait_type, 'RUNNING') AS wait_type,
			r.wait_time AS wait_time_ms,
			r.cpu_time AS cpu_time_ms,
			r.total_elapsed_time AS total_elapsed_time_ms,
			r.reads,
			r.writes,
			ISNULL(r.granted_query_memory * 8 / 1024, 0) AS granted_query_memory_mb,
			r.row_count,
			ISNULL(r.percent_complete, '0') AS percent_complete
		FROM sys.dm_exec_requests r
		INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE r.session_id > 50
		  AND r.session_id <> @@SPID
		  AND r.status IN ('running', 'runnable', 'suspended')
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		  AND DB_NAME(r.database_id) NOT IN ('master','model','msdb','tempdb')
		  AND s.program_name NOT LIKE '%SQLAgent%'
		  AND s.program_name NOT LIKE '%Monitoring%'
		  AND s.program_name NOT LIKE '%Telegraf%'
		  AND s.program_name NOT LIKE '%Grafana%'
		ORDER BY r.cpu_time DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[Repository] CollectActiveQueries Error: %v", err)
		return nil, fmt.Errorf("failed to fetch active queries: %w", err)
	}
	defer rows.Close()

	var results []models.ActiveQuery
	for rows.Next() {
		var q models.ActiveQuery
		var cpuTime, totalElapsed, reads, writes, grantedMemory, rowCount sql.NullInt64
		var waitTime sql.NullInt64

		err := rows.Scan(
			&q.SessionID, &q.RequestID, &q.DatabaseName, &q.LoginName,
			&q.HostName, &q.ProgramName, &q.QueryText, &q.Status,
			&q.Command, &q.WaitType, &waitTime, &cpuTime, &totalElapsed,
			&reads, &writes, &grantedMemory, &rowCount, &q.PercentComplete,
		)
		if err != nil {
			log.Printf("[Repository] CollectActiveQueries Scan Error: %v", err)
			continue
		}

		if cpuTime.Valid {
			q.CPUTimeMs = cpuTime.Int64
		}
		if totalElapsed.Valid {
			q.TotalElapsedTimeMs = totalElapsed.Int64
		}
		if waitTime.Valid {
			q.WaitTimeMs = waitTime.Int64
		}
		if reads.Valid {
			q.Reads = reads.Int64
		}
		if writes.Valid {
			q.Writes = writes.Int64
		}
		if grantedMemory.Valid {
			q.GrantedMemoryMB = int(grantedMemory.Int64)
		}
		if rowCount.Valid {
			q.RowCount = rowCount.Int64
		}

		results = append(results, q)
	}

	return results, rows.Err()
}

// CollectLongRunningQueries returns queries running longer than threshold
func CollectLongRunningQueries(ctx context.Context, db *sql.DB) ([]models.LongRunningQuery, error) {
	query := ` /* SQL_OPTIMA */ 
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
		AND r.total_elapsed_time >= 5000
		AND s.is_user_process = 1
		AND DB_NAME(r.database_id) NOT IN ('master','model','msdb','tempdb')
		AND s.program_name NOT LIKE '%SQLAgent%'
		AND s.program_name NOT LIKE '%Monitoring%'
		AND s.program_name NOT LIKE '%Telegraf%'
		AND s.program_name NOT LIKE '%Grafana%'
		ORDER BY r.total_elapsed_time DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[Repository] CollectLongRunningQueries Error: %v", err)
		return nil, fmt.Errorf("failed to fetch long running queries: %w", err)
	}
	defer rows.Close()

	var results []models.LongRunningQuery
	for rows.Next() {
		var q models.LongRunningQuery
		var waitType sql.NullString
		var cpuTime, totalElapsed, reads, writes, grantedMemory, rowCount sql.NullInt64
		var blockingSessionID sql.NullInt64

		err := rows.Scan(
			&q.SessionID, &q.RequestID, &q.DatabaseName, &q.LoginName,
			&q.HostName, &q.ProgramName, &q.QueryText, &waitType,
			&blockingSessionID, &q.Status, &cpuTime, &totalElapsed,
			&reads, &writes, &grantedMemory, &rowCount,
		)
		if err != nil {
			log.Printf("[Repository] CollectLongRunningQueries Scan Error: %v", err)
			continue
		}

		if waitType.Valid {
			q.WaitType = waitType.String
		}

		if cpuTime.Valid {
			q.CPUTimeMs = cpuTime.Int64
		}
		if totalElapsed.Valid {
			q.TotalElapsedTimeMs = totalElapsed.Int64
		}
		if reads.Valid {
			q.Reads = reads.Int64
		}
		if writes.Valid {
			q.Writes = writes.Int64
		}
		if blockingSessionID.Valid {
			q.BlockingSessionID = int(blockingSessionID.Int64)
		}
		if grantedMemory.Valid {
			q.GrantedQueryMemoryMB = int(grantedMemory.Int64)
		}
		if rowCount.Valid {
			q.RowCount = rowCount.Int64
		}

		results = append(results, q)
	}

	return results, rows.Err()
}

// CollectBlockingLocks returns active blocking tree and "Idle in Transaction" queries
func CollectBlockingLocks(ctx context.Context, db *sql.DB) ([]models.BlockingNode, error) {
	query := ` /* SQL_OPTIMA */ 
		SELECT TOP 50
			r.session_id,
			r.blocking_session_id,
			ISNULL(s.login_name, 'Unknown') AS login_name,
			ISNULL(s.host_name, 'Unknown') AS host_name,
			ISNULL(s.program_name, 'Unknown') AS program_name,
			ISNULL(DB_NAME(r.database_id), 'Unknown') AS database_name,
			ISNULL(LEFT(t.text, 2000), 'N/A') AS query_text,
			r.status,
			r.command,
			ISNULL(r.wait_type, 'NONE') AS wait_type,
			r.wait_time AS wait_time_ms,
			r.cpu_time AS cpu_time_ms,
			r.total_elapsed_time AS total_elapsed_time_ms,
			r.row_count,
			0 AS level
		FROM sys.dm_exec_requests r
		INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE r.session_id > 50
		  AND r.blocking_session_id > 0
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		UNION ALL
		SELECT TOP 50
			s.session_id,
			0 AS blocking_session_id,
			s.login_name,
			s.host_name,
			s.program_name,
			DB_NAME(s.database_id) AS database_name,
			'Idle in Transaction' AS query_text,
			s.status,
			'' AS command,
			'' AS wait_type,
			0 AS wait_time_ms,
			0 AS cpu_time_ms,
			0 AS total_elapsed_time_ms,
			0 AS row_count,
			0 AS level
		FROM sys.dm_exec_sessions s
		WHERE s.status = 'idle_in_transaction'
		  AND s.session_id > 50
		  AND s.session_id <> @@SPID
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		ORDER BY level DESC, wait_time_ms DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[Repository] CollectBlockingLocks Error: %v", err)
		return nil, fmt.Errorf("failed to fetch blocking locks: %w", err)
	}
	defer rows.Close()

	var results []models.BlockingNode
	for rows.Next() {
		var b models.BlockingNode
		var cpuTime, totalElapsed, waitTime, rowCount sql.NullInt64

		err := rows.Scan(
			&b.SessionID, &b.BlockingSessionID, &b.LoginName, &b.HostName,
			&b.ProgramName, &b.DatabaseName, &b.QueryText, &b.Status,
			&b.Command, &b.WaitType, &waitTime, &cpuTime, &totalElapsed,
			&rowCount, &b.Level,
		)
		if err != nil {
			log.Printf("[Repository] CollectBlockingLocks Scan Error: %v", err)
			continue
		}

		if cpuTime.Valid {
			b.CPUTimeMs = cpuTime.Int64
		}
		if totalElapsed.Valid {
			b.TotalElapsedTimeMs = totalElapsed.Int64
		}
		if waitTime.Valid {
			b.WaitTimeMs = waitTime.Int64
		}
		if rowCount.Valid {
			b.RowCount = rowCount.Int64
		}

		results = append(results, b)
	}

	return results, rows.Err()
}

// CollectCPUMemory returns CPU usage from sys.dm_os_ring_buffers and memory from sys.dm_os_sys_memory
func CollectCPUMemory(ctx context.Context, db *sql.DB) (*models.CPUTick, *models.MemoryStats, error) {
	cpuQuery := ` /* SQL_OPTIMA */ 
		DECLARE @ts_now bigint = (SELECT ms_ticks FROM sys.dm_os_sys_info WITH (NOLOCK)); 
		SELECT TOP(1)
		    ISNULL(SQLProcessUtilization, 0) AS SQL_Server_CPU, 
		    ISNULL(SystemIdle, 0) AS System_Idle_CPU, 
		    100 - ISNULL(SystemIdle, 0) - ISNULL(SQLProcessUtilization, 0) AS Other_Process_CPU,
		    CONVERT(varchar, DATEADD(ms, -1 * (@ts_now - [timestamp]), GETUTCDATE()), 120) AS Event_Time
		FROM ( 
		    SELECT record.value('(./Record/@id)[1]', 'int') AS record_id, 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int') AS SystemIdle, 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS SQLProcessUtilization, [timestamp] 
		    FROM ( 
		        SELECT [timestamp], CONVERT(xml, record) AS [record]
		        FROM sys.dm_os_ring_buffers WITH (NOLOCK)
		        WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR'
		        AND record LIKE N'%<SystemHealth>%'
		    ) AS x 
		) AS y
		ORDER BY record_id DESC`

	var cpu models.CPUTick
	var eventTime sql.NullString

	err := db.QueryRowContext(ctx, cpuQuery).Scan(
		&cpu.SQLProcess, &cpu.SystemIdle, &cpu.OtherProcess, &eventTime,
	)
	if err != nil {
		log.Printf("[Repository] CollectCPUMemory CPU Error: %v", err)
	} else if eventTime.Valid && eventTime.String != "" {
		cpu.EventTime = eventTime.String
	}

	memQuery := `
		/* SQL_OPTIMA */ 
		SELECT 
			ISNULL(available_physical_memory_kb, 0) / 1024 AS available_mb,
			ISNULL(total_physical_memory_kb, 0) / 1024 AS total_mb
		FROM sys.dm_os_sys_memory`

	var mem models.MemoryStats
	var avail, total sql.NullInt64
	err = db.QueryRowContext(ctx, memQuery).Scan(&avail, &total)
	if err != nil {
		log.Printf("[Repository] CollectCPUMemory Memory Error: %v", err)
	} else {
		mem.AvailableMB = int(avail.Int64)
		mem.TotalMB = int(total.Int64)
		if total.Valid && total.Int64 > 0 {
			mem.UsagePercent = float64(total.Int64-avail.Int64) / float64(total.Int64) * 100
		}
		mem.CapturedAt = time.Now().Format("2006-01-02 15:04:05")
	}

	return &cpu, &mem, nil
}

// CollectSessionSnapshot gathers identity metrics from sys.dm_exec_sessions and sys.dm_exec_requests
func CollectSessionSnapshot(ctx context.Context, db *sql.DB) ([]models.SQLServerSessionSnapshot, error) {
	query := `
	/* SQL_OPTIMA */ 
	SELECT
		GETUTCDATE() AS sample_time,
		s.session_id,
		s.login_name,
		s.original_login_name,
		s.host_name,
		s.program_name,
		ISNULL(DB_NAME(r.database_id), 'Unknown') AS database_name,
		s.is_user_process,
		s.status,
		r.query_hash,
		r.query_plan_hash,
		ISNULL(r.total_elapsed_time, 0) AS total_elapsed_time_ms,
		ISNULL(r.cpu_time, 0) AS cpu_time_ms,
		ISNULL(r.wait_type, '') AS wait_type,
		ISNULL(r.blocking_session_id, 0) AS blocking_session_id,
		ISNULL(st.text, '') AS query_text
	FROM sys.dm_exec_sessions s
	INNER JOIN sys.dm_exec_requests r
		ON s.session_id = r.session_id
	OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
	WHERE s.session_id <> @@SPID
	AND s.is_user_process = 1;`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying session snapshot: %w", err)
	}
	defer rows.Close()

	var snapshots []models.SQLServerSessionSnapshot
	for rows.Next() {
		var s models.SQLServerSessionSnapshot
		var queryHash, queryPlanHash []byte
		var loginName, origLoginName, hostName, programName, status, dbName, waitType, queryText sql.NullString
		var elapsedTime, cpuTime float64
		var blockingID int

		err := rows.Scan(
			&s.SampleTime,
			&s.SessionID,
			&loginName,
			&origLoginName,
			&hostName,
			&programName,
			&dbName,
			&s.IsUserProcess,
			&status,
			&queryHash,
			&queryPlanHash,
			&elapsedTime,
			&cpuTime,
			&waitType,
			&blockingID,
			&queryText,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning session snapshot row: %w", err)
		}

		s.LoginName = loginName.String
		s.OriginalLoginName = origLoginName.String
		s.HostName = hostName.String
		s.ProgramName = programName.String
		s.Status = status.String
		s.DatabaseName = dbName.String
		s.QueryHash = queryHash
		s.QueryPlanHash = queryPlanHash

		s.TotalElapsedTimeMs = &elapsedTime
		s.CPUTimeMs = &cpuTime
		s.WaitType = waitType.String
		s.BlockingSessionID = &blockingID
		s.QueryText = queryText.String

		snapshots = append(snapshots, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating session snapshot rows: %w", err)
	}

	return snapshots, nil
}

// CollectStorageIO returns disk I/O latency and tempdb pressure metrics
func CollectStorageIO(ctx context.Context, db *sql.DB) ([]models.FileIOStat, *models.TempDBStats, error) {
	fileIOQuery := ` /* SQL_OPTIMA */ 
		SELECT 
			DB_NAME(mf.database_id) AS database_name,
			mf.physical_name,
			mf.type_desc AS file_type,
			CAST(vfs.io_stall_read_ms AS FLOAT) / NULLIF(vfs.num_of_reads, 0) AS read_latency_ms,
			CAST(vfs.io_stall_write_ms AS FLOAT) / NULLIF(vfs.num_of_writes, 0) AS write_latency_ms,
			vfs.num_of_reads,
			vfs.num_of_writes,
			mf.size * 8 / 1024 AS size_mb
		FROM sys.dm_io_virtual_file_stats(NULL, NULL) vfs
		INNER JOIN sys.master_files mf ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id
		WHERE mf.type_desc IN ('DATA', 'LOG')
		ORDER BY database_name, file_type`

	rows, err := db.QueryContext(ctx, fileIOQuery)
	if err != nil {
		log.Printf("[Repository] CollectStorageIO FileIO Error: %v", err)
		return nil, nil, fmt.Errorf("failed to fetch file I/O stats: %w", err)
	}
	defer rows.Close()

	var fileStats []models.FileIOStat
	var sizeMB int64
	for rows.Next() {
		var f models.FileIOStat
		var readLat, writeLat sql.NullFloat64

		err := rows.Scan(
			&f.DatabaseName, &f.PhysicalName, &f.FileType,
			&readLat, &writeLat, &f.NumOfReads, &f.NumOfWrites, &sizeMB,
		)
		if err != nil {
			log.Printf("[Repository] CollectStorageIO Scan Error: %v", err)
			continue
		}

		if readLat.Valid {
			f.ReadLatencyMs = readLat.Float64
		}
		if writeLat.Valid {
			f.WriteLatencyMs = writeLat.Float64
		}
		f.IoStallReadMs = 0
		f.IoStallWriteMs = 0

		fileStats = append(fileStats, f)
	}

	tempDBQuery := ` /* SQL_OPTIMA */ 
		SELECT 
			DB_NAME(database_id) AS database_name,
			COUNT(*) AS total_data_files,
			SUM(size * 8 / 1024) AS total_size_mb,
			0 AS used_space_mb,
			0.0 AS pfs_contention_pct,
			0.0 AS gam_contention_pct,
			0.0 AS sgam_contention_pct
		FROM sys.master_files
		WHERE database_id = 2 AND type_desc = 'DATA'
		GROUP BY database_id`

	/*
		SELECT
		    GETDATE() AS capture_time,
		    DB_NAME(2) AS database_name,
		    COUNT(*) AS total_data_files,
		    SUM(size * 8.0 / 1024) AS total_size_mb,
		    SUM(FILEPROPERTY(name, 'SpaceUsed') * 8.0 / 1024) AS used_space_mb,
		    SUM(size * 8.0 / 1024)
		      - SUM(FILEPROPERTY(name, 'SpaceUsed') * 8.0 / 1024) AS free_space_mb,
		    CAST(
		        (
		            SUM(FILEPROPERTY(name, 'SpaceUsed') * 100.0)
		            / NULLIF(SUM(size), 0)
		        ) AS DECIMAL(8,2)
		    ) AS used_pct
		FROM tempdb.sys.database_files
		WHERE type_desc = 'ROWS';
	*/

	var tempDB models.TempDBStats
	err = db.QueryRowContext(ctx, tempDBQuery).Scan(
		&tempDB.DatabaseName, &tempDB.TotalDataFiles, &tempDB.TotalSizeMB,
		&tempDB.UsedSpaceMB, &tempDB.PFSContentionPct, &tempDB.GAMContentionPct, &tempDB.SGAMContentionPct,
	)
	if err != nil {
		log.Printf("[Repository] CollectStorageIO TempDB Error: %v", err)
	}

	return fileStats, &tempDB, nil
}

// CollectWaitStats returns wait statistics from sys.dm_os_wait_stats
func CollectWaitStats(ctx context.Context, db *sql.DB) ([]models.WaitStat, error) {
	query := ` /* SQL_OPTIMA */ 
		SELECT wait_type, CAST(wait_time_ms AS FLOAT), CAST(waiting_tasks_count AS BIGINT)
		FROM sys.dm_os_wait_stats WITH (NOLOCK) 
		WHERE wait_type NOT IN (
			'DIRTY_PAGE_POLL', 'HADR_FILESTREAM_IOMGR_IOCOMPLETION', 
			'LAZYWRITER_SLEEP', 'LOGMGR_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 
			'XE_DISPATCHER_WAIT', 'XE_TIMER_EVENT', 'SQLTRACE_BUFFER_FLUSH', 
			'SLEEP_TASK', 'BROKER_TO_FLUSH', 'SP_SERVER_DIAGNOSTICS_SLEEP'
		) 
		AND wait_time_ms > 0
		ORDER BY wait_time_ms DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[Repository] CollectWaitStats Error: %v", err)
		return nil, fmt.Errorf("failed to fetch wait stats: %w", err)
	}
	defer rows.Close()

	var results []models.WaitStat
	for rows.Next() {
		var w models.WaitStat
		var waitTime sql.NullFloat64
		var waitingTasks sql.NullInt64

		err := rows.Scan(&w.WaitType, &waitTime, &waitingTasks)
		if err != nil {
			log.Printf("[Repository] CollectWaitStats Scan Error: %v", err)
			continue
		}

		if waitTime.Valid {
			w.WaitTimeMs = waitTime.Float64
		}
		if waitingTasks.Valid {
			w.WaitingTasks = waitingTasks.Int64
		}

		results = append(results, w)
	}

	return results, rows.Err()
}
