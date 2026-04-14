// Package repository handles all database connections and queries.
// It provides data access layer for both SQL Server and PostgreSQL databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server live and historical telemetry fetcher including CPU, memory, PLE, waits, and file I/O metrics.
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
	"sort"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
)

func (c *MssqlRepository) FetchLiveTelemetry(instanceName string, prev models.DashboardMetrics) models.DashboardMetrics {
	var metrics models.DashboardMetrics
	metrics.InstanceName = instanceName
	metrics.DiskByDB = make(map[string]models.DiskStat)
	metrics.LocksByDB = make(map[string]models.LockStat)
	metrics.PrevWaitStats = make(map[string]float64)
	metrics.PrevFileStats = make(map[string]models.FileIOStat)

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return metrics
	}

	metrics.MemHistory = prev.MemHistory
	metrics.PLEHistory = prev.PLEHistory
	// Preserve slower-moving histories computed on the historical ticker.
	// The live ticker runs more frequently and would otherwise wipe these fields
	// from the shared dashboard cache, causing charts (e.g. Disk I/O Latency) to appear empty.
	metrics.WaitHistory = prev.WaitHistory
	metrics.FileHistory = prev.FileHistory
	for k, v := range prev.PrevWaitStats {
		metrics.PrevWaitStats[k] = v
	}
	for k, v := range prev.PrevFileStats {
		metrics.PrevFileStats[k] = v
	}

	cpuQuery := `
		DECLARE @ts_now bigint = (SELECT cpu_ticks/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info WITH (NOLOCK)); 
		SELECT TOP(256)
		    SQLProcessUtilization AS [SQL_Server_CPU], 
		    SystemIdle AS [System_Idle_CPU], 
		    100 - SystemIdle - SQLProcessUtilization AS [Other_Process_CPU],
		    CONVERT(varchar, DATEADD(ms, -1 * (@ts_now - [timestamp]), GETDATE()), 120) AS [Event_Time]
		FROM ( 
		    SELECT record.value('(./Record/@id)[1]', 'int') AS record_id, 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int') 
		        AS [SystemIdle], 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') 
		        AS [SQLProcessUtilization], [timestamp] 
		    FROM ( 
		        SELECT [timestamp], CONVERT(xml, record) AS [record] 
		        FROM sys.dm_os_ring_buffers WITH (NOLOCK)
		        WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR' 
		        AND record LIKE N'%<SystemHealth>%'
		    ) AS x 
		) AS y 
		ORDER BY record_id DESC;
	`
	cpuRows, errCpu := db.Query(cpuQuery)
	if errCpu == nil {
		defer cpuRows.Close()
		var buffer []models.CPUTick
		for cpuRows.Next() {
			var tick models.CPUTick
			if err := cpuRows.Scan(&tick.SQLProcess, &tick.SystemIdle, &tick.OtherProcess, &tick.EventTime); err == nil {
				buffer = append(buffer, tick)
			}
		}
		for i, j := 0, len(buffer)-1; i < j; i, j = i+1, j-1 {
			buffer[i], buffer[j] = buffer[j], buffer[i]
		}
		metrics.CPUHistory = buffer
		if len(buffer) > 0 {
			metrics.AvgCPULoad = buffer[len(buffer)-1].SQLProcess
		}
	}

	sessionQuery := `SELECT COUNT(*) FROM sys.dm_exec_sessions WHERE is_user_process = 1 AND status = 'running' AND LOWER(ISNULL(login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb') AND LOWER(ISNULL(program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')`
	db.QueryRow(sessionQuery).Scan(&metrics.ActiveUsers)

	connQuery := `
		SELECT 
			ISNULL(s.login_name, 'Unknown'),
			ISNULL(DB_NAME(s.database_id), 'Unknown'),
			COUNT(s.session_id) as active_connections,
			SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END) as active_requests
		FROM sys.dm_exec_sessions s WITH (NOLOCK)
		WHERE is_user_process = 1
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		GROUP BY s.login_name, s.database_id
	`
	connRows, _ := db.Query(connQuery)
	if connRows != nil {
		defer connRows.Close()
		for connRows.Next() {
			var conn models.ConnectionStat
			if err := connRows.Scan(&conn.LoginName, &conn.DatabaseName, &conn.ActiveConnections, &conn.ActiveRequests); err == nil {
				metrics.ConnectionStats = append(metrics.ConnectionStats, conn)
			}
		}
	}

	metrics.LocksByDB = make(map[string]models.LockStat)
	lockQuery := `
		SELECT 
			ISNULL(DB_NAME(resource_database_id), 'Unknown'),
			COUNT(*),
			SUM(CASE WHEN request_status = 'CONVERT' THEN 1 ELSE 0 END)
		FROM sys.dm_tran_locks WITH (NOLOCK)
		WHERE resource_database_id > 0
		GROUP BY resource_database_id
	`
	lRows, errL := db.Query(lockQuery)
	if errL == nil {
		defer lRows.Close()
		for lRows.Next() {
			var dbName string
			var l models.LockStat
			if err := lRows.Scan(&dbName, &l.TotalLocks, &l.Deadlocks); err == nil {
				metrics.LocksByDB[dbName] = l
				metrics.TotalLocks += l.TotalLocks
				metrics.Deadlocks += l.Deadlocks
			}
		}
	}

	blockDetQuery := `
		SELECT
			r.session_id as blocked_session_id,
			ISNULL(r.blocking_session_id, 0) as blocking_session_id,
			ISNULL(DB_NAME(r.database_id), 'Unknown') as database_name,
			ISNULL(r.wait_type, 'ONLINE') as wait_type,
			r.wait_time as wait_time_ms,
			ISNULL(t.text, 'Internal Pointer Buffer') as query_text,
			ISNULL(s.status, 'running') as status,
			ISNULL(s.host_name, 'Unknown') as host_name,
			ISNULL(s.program_name, 'Unknown') as program_name
		FROM sys.dm_exec_requests r WITH (NOLOCK)
		JOIN sys.dm_exec_sessions s WITH (NOLOCK) ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE r.session_id > 50 AND r.session_id <> @@SPID
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
	`
	blockRows, errBlk := db.Query(blockDetQuery)
	if errBlk == nil {
		defer blockRows.Close()
		for blockRows.Next() {
			var b models.BlockStat
			if err := blockRows.Scan(&b.BlockedSessionID, &b.BlockingSessionID, &b.DatabaseName, &b.WaitType, &b.WaitTimeMs, &b.QueryText, &b.Status, &b.HostName, &b.ProgramName); err == nil {
				metrics.ActiveBlocks = append(metrics.ActiveBlocks, b)
			}
		}
	}

	memQuery := `
		SELECT 
			ISNULL(100.0 * (CAST(total_physical_memory_kb AS FLOAT) - CAST(available_physical_memory_kb AS FLOAT)) / 
			CAST(total_physical_memory_kb AS FLOAT), 0)
		FROM sys.dm_os_sys_memory
	`
	errMem := db.QueryRow(memQuery).Scan(&metrics.MemoryUsage)
	if errMem != nil {
		metrics.MemoryUsage = 0
	}

	metrics.MemHistory = append(metrics.MemHistory, metrics.MemoryUsage)
	if len(metrics.MemHistory) > 20 {
		metrics.MemHistory = metrics.MemHistory[1:]
	}

	metrics.MemoryClerks = make([]models.MemoryStat, 0)
	clerkQuery := `
		SELECT 
			RTRIM(counter_name) as type,
			CAST(cntr_value AS FLOAT) / 1024.0 as size_mb
		FROM sys.dm_os_performance_counters
		WHERE object_name LIKE '%Memory Manager%'
		AND counter_name IN ('Total Server Memory (KB)', 'Target Server Memory (KB)', 'Connection Memory (KB)', 'Lock Memory (KB)')
	`
	memRows, errMemClerks := db.Query(clerkQuery)
	if errMemClerks == nil {
		defer memRows.Close()
		for memRows.Next() {
			var ms models.MemoryStat
			if err := memRows.Scan(&ms.Type, &ms.SizeMB); err == nil {
				metrics.MemoryClerks = append(metrics.MemoryClerks, ms)
			}
		}
	}

	pleQuery := `SELECT ISNULL(CAST(cntr_value AS FLOAT), 0) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE [counter_name] = N'Page life expectancy' AND [object_name] LIKE '%Buffer Manager%'`
	var currentPLE float64
	if err := db.QueryRow(pleQuery).Scan(&currentPLE); err == nil {
		metrics.PLEHistory = append(metrics.PLEHistory, currentPLE)
		if len(metrics.PLEHistory) > 960 {
			metrics.PLEHistory = metrics.PLEHistory[1:]
		}
	}

	if metrics.DiskByDB == nil {
		metrics.DiskByDB = make(map[string]models.DiskStat)
	}
	diskQuery := `
		SELECT 
			ISNULL(DB_NAME(database_id), 'Unknown'),
			SUM(CASE WHEN type=0 THEN size * 8.0/1024.0 ELSE 0 END) as Data,
			SUM(CASE WHEN type=1 THEN size * 8.0/1024.0 ELSE 0 END) as Log
		FROM sys.master_files
		GROUP BY database_id
	`
	dRows, errD := db.Query(diskQuery)
	if errD == nil {
		defer dRows.Close()
		for dRows.Next() {
			var dbName string
			var d models.DiskStat
			if err := dRows.Scan(&dbName, &d.DataMB, &d.LogMB); err == nil {
				metrics.DiskByDB[dbName] = d
				metrics.DiskUsage.DataMB += d.DataMB
				metrics.DiskUsage.LogMB += d.LogMB
			}
		}
	}

	metrics.TopQueries = []models.QueryStat{}

	runningSQL := `
		SELECT TOP 20
			ISNULL(s.login_name, 'System') as login_name,
			ISNULL(s.program_name, 'System') as program_name,
			ISNULL(DB_NAME(r.database_id), 'Unknown') as database_name,
			CASE WHEN r.sql_handle IS NOT NULL THEN ISNULL(LEFT(t.text, 500), 'Unknown') ELSE 'Unknown' END as query_text,
			ISNULL(r.wait_type, 'RUNNING') as wait_type,
			ISNULL(r.cpu_time, 0) as cpu_time_ms,
			ISNULL(r.total_elapsed_time, 0) / 1000.0 as exec_time_ms,
			ISNULL(r.logical_reads, 0) as logical_reads,
			1 as execution_count
		FROM sys.dm_exec_requests r
		INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE s.is_user_process = 1
		  AND r.session_id <> @@SPID
		  AND (r.cpu_time > 50 OR r.total_elapsed_time > 5000000 OR r.logical_reads > 5000)
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		ORDER BY r.total_elapsed_time DESC
	`
	rows, err := db.Query(runningSQL)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var q models.QueryStat
			var login, program, dbName sql.NullString

			if err := rows.Scan(&login, &program, &dbName, &q.QueryText, &q.WaitType, &q.CPUTimeMs, &q.ExecTimeMs, &q.LogicalReads, &q.ExecutionCount); err == nil {
				q.LoginName = login.String
				q.ProgramName = program.String
				q.DatabaseName = dbName.String
				metrics.TopQueries = append(metrics.TopQueries, q)
			}
		}
	}

	cacheSQL := `
		SELECT TOP 20
			ISNULL(s.login_name, 'System') as login_name,
			ISNULL(s.program_name, 'System') as program_name,
			ISNULL(DB_NAME(CAST(f.value AS INT)), 'Unknown') as database_name,
			ISNULL(LEFT(t.text, 500), 'Unknown') as query_text,
			'PLAN_CACHE' as wait_type,
			ISNULL(qs.total_worker_time / NULLIF(qs.execution_count, 0), 0) as cpu_time_ms,
			ISNULL(qs.total_elapsed_time / NULLIF(qs.execution_count, 0), 0) / 1000.0 as exec_time_ms,
			ISNULL(qs.total_logical_reads / NULLIF(qs.execution_count, 0), 0) as logical_reads,
			ISNULL(qs.execution_count, 1) as execution_count
		FROM sys.dm_exec_query_stats qs
		CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) t
		OUTER APPLY sys.dm_exec_plan_attributes(qs.plan_handle) f
		WHERE t.text IS NOT NULL
		  AND (qs.total_worker_time / NULLIF(qs.execution_count, 0) > 50
		       OR qs.total_elapsed_time / NULLIF(qs.execution_count, 0) > 5000
		       OR qs.total_logical_reads / NULLIF(qs.execution_count, 0) > 5000)
		ORDER BY qs.total_worker_time DESC
	`
	cacheRows, cacheErr := db.Query(cacheSQL)
	if cacheErr == nil {
		defer cacheRows.Close()
		for cacheRows.Next() {
			var q models.QueryStat
			var login, program, dbName sql.NullString

			if err := cacheRows.Scan(&login, &program, &dbName, &q.QueryText, &q.WaitType, &q.CPUTimeMs, &q.ExecTimeMs, &q.LogicalReads, &q.ExecutionCount); err == nil {
				q.LoginName = login.String
				q.ProgramName = program.String
				q.DatabaseName = dbName.String
				metrics.TopQueries = append(metrics.TopQueries, q)
			}
		}
	}

	if config.GlobalQueries != nil && len(config.GlobalQueries.Metrics) > 0 {
		metrics.PrometheusData = make(map[string][]map[string]interface{})

		for _, q := range config.GlobalQueries.Metrics {
			if q.MetricName == "mssql_cpu_usage_percent" || q.MetricName == "mssql_long_running_queries" {
				continue
			}

			rows, err := db.Query(q.Query)
			if err != nil {
				continue
			}

			cols, _ := rows.Columns()
			var queryResults []map[string]interface{}

			for rows.Next() {
				columns := make([]interface{}, len(cols))
				columnPointers := make([]interface{}, len(cols))
				for i := range columns {
					columnPointers[i] = &columns[i]
				}

				if err := rows.Scan(columnPointers...); err == nil {
					rowMap := make(map[string]interface{})
					for i, colName := range cols {
						val := columnPointers[i].(*interface{})
						switch v := (*val).(type) {
						case []byte:
							rowMap[colName] = string(v)
						default:
							rowMap[colName] = v
						}
					}
					queryResults = append(queryResults, rowMap)
				}
			}
			rows.Close()

			if len(queryResults) > 0 {
				metrics.PrometheusData[q.MetricName] = queryResults
			}
		}
	}

	return metrics
}

func (c *MssqlRepository) FetchHistoricalTelemetry(instanceName string, prev models.DashboardMetrics) models.DashboardMetrics {
	var metrics models.DashboardMetrics
	metrics.InstanceName = instanceName
	metrics.MemHistory = prev.MemHistory
	metrics.PLEHistory = prev.PLEHistory
	metrics.WaitHistory = prev.WaitHistory
	metrics.FileHistory = prev.FileHistory

	metrics.DiskByDB = make(map[string]models.DiskStat)
	metrics.LocksByDB = make(map[string]models.LockStat)
	metrics.PrevWaitStats = make(map[string]float64)
	metrics.PrevFileStats = make(map[string]models.FileIOStat)
	for k, v := range prev.PrevWaitStats {
		metrics.PrevWaitStats[k] = v
	}
	for k, v := range prev.PrevFileStats {
		metrics.PrevFileStats[k] = v
	}

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return metrics
	}

	wQuery := `SELECT wait_type, CAST(wait_time_ms AS FLOAT) FROM sys.dm_os_wait_stats WITH (NOLOCK) WHERE wait_type NOT IN ('DIRTY_PAGE_POLL', 'HADR_FILESTREAM_IOMGR_IOCOMPLETION', 'LAZYWRITER_SLEEP', 'LOGMGR_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 'XE_DISPATCHER_WAIT', 'XE_TIMER_EVENT', 'SQLTRACE_BUFFER_FLUSH', 'SLEEP_TASK', 'BROKER_TO_FLUSH', 'SP_SERVER_DIAGNOSTICS_SLEEP') AND wait_time_ms > 0`
	wRows, _ := db.Query(wQuery)
	var wSnap models.WaitSnapshot
	wSnap.Timestamp = time.Now().Format("2006-01-02 15:04:05")
	if wRows != nil {
		defer wRows.Close()
		for wRows.Next() {
			var wt string
			var wtMs float64
			if err := wRows.Scan(&wt, &wtMs); err == nil {
				prevMs, exists := metrics.PrevWaitStats[wt]
				metrics.PrevWaitStats[wt] = wtMs
				if exists && wtMs >= prevMs {
					deltaSec := (wtMs - prevMs) / 60.0
					if strings.HasPrefix(wt, "PAGEIOLATCH") {
						wSnap.DiskRead += deltaSec
					} else if strings.HasPrefix(wt, "LCK_") {
						wSnap.Blocking += deltaSec
					} else if strings.HasPrefix(wt, "CXPACKET") || strings.HasPrefix(wt, "CXCONSUMER") {
						wSnap.Parallelism += deltaSec
					} else {
						wSnap.Other += deltaSec
					}
				}
			}
		}
	}
	metrics.WaitHistory = append(metrics.WaitHistory, wSnap)
	if len(metrics.WaitHistory) > 960 {
		metrics.WaitHistory = metrics.WaitHistory[1:]
	}

	fQuery := `SELECT DB_NAME(vfs.database_id), mf.physical_name, mf.type_desc, vfs.num_of_reads, vfs.num_of_writes, vfs.io_stall_read_ms, vfs.io_stall_write_ms FROM sys.dm_io_virtual_file_stats(NULL, NULL) AS vfs JOIN sys.master_files AS mf WITH (NOLOCK) ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id`
	fRows, _ := db.Query(fQuery)
	var fSnap models.FileIOSnapshot
	fSnap.Timestamp = time.Now().Format("2006-01-02 15:04:05")
	if fRows != nil {
		defer fRows.Close()
		for fRows.Next() {
			var f models.FileIOStat
			var r, w, sr, sw int64
			if err := fRows.Scan(&f.DatabaseName, &f.PhysicalName, &f.FileType, &r, &w, &sr, &sw); err == nil {
				key := f.DatabaseName + ":" + f.PhysicalName
				f.NumOfReads = r
				f.NumOfWrites = w
				f.IoStallReadMs = sr
				f.IoStallWriteMs = sw
				metrics.PrevFileStats[key] = f
				if prevF, exists := prev.PrevFileStats[key]; exists {
					deltaR := float64(r - prevF.NumOfReads)
					deltaW := float64(w - prevF.NumOfWrites)
					deltaSR := float64(sr - prevF.IoStallReadMs)
					deltaSW := float64(sw - prevF.IoStallWriteMs)
					if deltaR > 0 {
						f.ReadLatencyMs = deltaSR / deltaR
					}
					if deltaW > 0 {
						f.WriteLatencyMs = deltaSW / deltaW
					}
					fSnap.Files = append(fSnap.Files, f)
					metrics.FileStats = append(metrics.FileStats, f)
				}
			}
		}
	}
	metrics.FileHistory = append(metrics.FileHistory, fSnap)
	if len(metrics.FileHistory) > 240 {
		metrics.FileHistory = metrics.FileHistory[1:]
	}

	return metrics
}

func (c *MssqlRepository) FetchDashboardTelemetry(instanceName string, prev models.DashboardMetrics) models.DashboardMetrics {
	var metrics models.DashboardMetrics
	metrics.InstanceName = instanceName
	metrics.MemHistory = prev.MemHistory
	metrics.PLEHistory = prev.PLEHistory
	metrics.WaitHistory = prev.WaitHistory
	metrics.FileHistory = prev.FileHistory

	metrics.PrevWaitStats = make(map[string]float64)
	if prev.PrevWaitStats != nil {
		for k, v := range prev.PrevWaitStats {
			metrics.PrevWaitStats[k] = v
		}
	}

	metrics.PrevFileStats = make(map[string]models.FileIOStat)
	if prev.PrevFileStats != nil {
		for k, v := range prev.PrevFileStats {
			metrics.PrevFileStats[k] = v
		}
	}

	metrics.DiskByDB = make(map[string]models.DiskStat)
	metrics.LocksByDB = make(map[string]models.LockStat)

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return metrics
	}

	// 1. CPU Usage (Extract fully chronological 256-minute Ring Buffer history dynamically dropping arbitrary Go array bindings)
	cpuQuery := `
		DECLARE @ts_now bigint = (SELECT cpu_ticks/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info WITH (NOLOCK)); 
		SELECT TOP(256)
		    SQLProcessUtilization AS [SQL_Server_CPU], 
		    SystemIdle AS [System_Idle_CPU], 
		    100 - SystemIdle - SQLProcessUtilization AS [Other_Process_CPU],
		    CONVERT(varchar, DATEADD(ms, -1 * (@ts_now - [timestamp]), GETDATE()), 120) AS [Event_Time]
		FROM ( 
		    SELECT record.value('(./Record/@id)[1]', 'int') AS record_id, 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int') 
		        AS [SystemIdle], 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') 
		        AS [SQLProcessUtilization], [timestamp] 
		    FROM ( 
		        SELECT [timestamp], CONVERT(xml, record) AS [record] 
		        FROM sys.dm_os_ring_buffers WITH (NOLOCK)
		        WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR' 
		        AND record LIKE N'%<SystemHealth>%'
		    ) AS x 
		) AS y 
		ORDER BY record_id DESC;
	`
	cpuRows, errCpu := db.Query(cpuQuery)
	if errCpu == nil {
		defer cpuRows.Close()
		var buffer []models.CPUTick
		for cpuRows.Next() {
			var tick models.CPUTick
			if err := cpuRows.Scan(&tick.SQLProcess, &tick.SystemIdle, &tick.OtherProcess, &tick.EventTime); err == nil {
				buffer = append(buffer, tick)
			}
		}

		// Map chronological sorting order (Reverse the DESC query output inside memory L-to-R for charting safely)
		for i, j := 0, len(buffer)-1; i < j; i, j = i+1, j-1 {
			buffer[i], buffer[j] = buffer[j], buffer[i]
		}
		metrics.CPUHistory = buffer
		if len(buffer) > 0 {
			metrics.AvgCPULoad = buffer[len(buffer)-1].SQLProcess
		}
	}

	// 2. Active Sessions (mssql_active_sessions_by_status)
	sessionQuery := `SELECT COUNT(*) FROM sys.dm_exec_sessions WHERE is_user_process = 1 AND status = 'running' AND LOWER(ISNULL(login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb') AND LOWER(ISNULL(program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')`
	db.QueryRow(sessionQuery).Scan(&metrics.ActiveUsers)

	// 2b. Connections grouping natively over physically bounded user target logical pools
	connQuery := `
		SELECT 
			ISNULL(s.login_name, 'Unknown'),
			ISNULL(DB_NAME(s.database_id), 'Unknown'),
			COUNT(s.session_id) as active_connections,
			SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END) as active_requests
		FROM sys.dm_exec_sessions s WITH (NOLOCK)
		WHERE is_user_process = 1
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		GROUP BY s.login_name, s.database_id
	`
	connRows, _ := db.Query(connQuery)
	if connRows != nil {
		defer connRows.Close()
		for connRows.Next() {
			var c models.ConnectionStat
			if err := connRows.Scan(&c.LoginName, &c.DatabaseName, &c.ActiveConnections, &c.ActiveRequests); err == nil {
				metrics.ConnectionStats = append(metrics.ConnectionStats, c)
			}
		}
	}

	// 3. Locks and Deadlocks grouped purely across target logical volumes dynamically
	metrics.LocksByDB = make(map[string]models.LockStat)
	lockQuery := `
		SELECT 
			ISNULL(DB_NAME(resource_database_id), 'Unknown'),
			COUNT(*),
			SUM(CASE WHEN request_status = 'CONVERT' THEN 1 ELSE 0 END)
		FROM sys.dm_tran_locks WITH (NOLOCK)
		WHERE resource_database_id > 0
		GROUP BY resource_database_id
	`
	lRows, errL := db.Query(lockQuery)
	if errL == nil {
		defer lRows.Close()
		for lRows.Next() {
			var dbName string
			var l models.LockStat
			if err := lRows.Scan(&dbName, &l.TotalLocks, &l.Deadlocks); err == nil {
				metrics.LocksByDB[dbName] = l
				metrics.TotalLocks += l.TotalLocks
				metrics.Deadlocks += l.Deadlocks
			}
		}
	}

	// 3b. Active Long Running & Blockers Hierarchy (Replaces rigid wait filtering)
	blockDetQuery := `
		SELECT
			r.session_id as blocked_session_id,
			ISNULL(r.blocking_session_id, 0) as blocking_session_id,
			ISNULL(DB_NAME(r.database_id), 'Unknown') as database_name,
			ISNULL(r.wait_type, 'ONLINE') as wait_type,
			r.wait_time as wait_time_ms,
			ISNULL(t.text, 'Internal Pointer Buffer') as query_text,
			ISNULL(s.status, 'running') as status,
			ISNULL(s.host_name, 'Unknown') as host_name,
			ISNULL(s.program_name, 'Unknown') as program_name
		FROM sys.dm_exec_requests r WITH (NOLOCK)
		JOIN sys.dm_exec_sessions s WITH (NOLOCK) ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE r.session_id > 50 AND r.session_id <> @@SPID
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
	`
	blockRows, errBlk := db.Query(blockDetQuery)
	if errBlk == nil {
		defer blockRows.Close()
		for blockRows.Next() {
			var b models.BlockStat
			if err := blockRows.Scan(&b.BlockedSessionID, &b.BlockingSessionID, &b.DatabaseName, &b.WaitType, &b.WaitTimeMs, &b.QueryText, &b.Status, &b.HostName, &b.ProgramName); err == nil {
				metrics.ActiveBlocks = append(metrics.ActiveBlocks, b)
			}
		}
	}

	// 4. Memory Pct (OS-based calculation)
	memQuery := `
		SELECT 
			ISNULL(100.0 * (CAST(total_physical_memory_kb AS FLOAT) - CAST(available_physical_memory_kb AS FLOAT)) / 
			CAST(total_physical_memory_kb AS FLOAT), 0)
		FROM sys.dm_os_sys_memory
	`
	errMem := db.QueryRow(memQuery).Scan(&metrics.MemoryUsage)
	if errMem != nil {
		metrics.MemoryUsage = 0
	}

	metrics.MemHistory = append(metrics.MemHistory, metrics.MemoryUsage)
	if len(metrics.MemHistory) > 20 {
		metrics.MemHistory = metrics.MemHistory[1:]
	}

	// Capture memory clerks exactly explicitly tracking native internal consumption footprints.
	clerkQuery := `
		SELECT 
			RTRIM(counter_name) as type,
			CAST(cntr_value AS FLOAT) / 1024.0 as size_mb
		FROM sys.dm_os_performance_counters
		WHERE object_name LIKE '%Memory Manager%'
		AND counter_name IN ('Total Server Memory (KB)', 'Target Server Memory (KB)', 'Connection Memory (KB)', 'Lock Memory (KB)')
	`
	memRows, errMemClerks := db.Query(clerkQuery)
	if errMemClerks == nil {
		defer memRows.Close()
		for memRows.Next() {
			var ms models.MemoryStat
			if err := memRows.Scan(&ms.Type, &ms.SizeMB); err == nil {
				metrics.MemoryClerks = append(metrics.MemoryClerks, ms)
			}
		}
	}

	// 5b. Page Life Expectancy (PLE)
	pleQuery := `SELECT ISNULL(CAST(cntr_value AS FLOAT), 0) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE [counter_name] = N'Page life expectancy' AND [object_name] LIKE '%Buffer Manager%'`
	var currentPLE float64
	if err := db.QueryRow(pleQuery).Scan(&currentPLE); err == nil {
		metrics.PLEHistory = append(metrics.PLEHistory, currentPLE)
		if len(metrics.PLEHistory) > 960 {
			metrics.PLEHistory = metrics.PLEHistory[1:]
		}
	}

	// Wait Stats (Cumulative Deltas converting to ms/sec scaling across 60 sec loops)
	wQuery := `SELECT wait_type, CAST(wait_time_ms AS FLOAT) FROM sys.dm_os_wait_stats WITH (NOLOCK) WHERE wait_type NOT IN ('DIRTY_PAGE_POLL', 'HADR_FILESTREAM_IOMGR_IOCOMPLETION', 'LAZYWRITER_SLEEP', 'LOGMGR_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 'XE_DISPATCHER_WAIT', 'XE_TIMER_EVENT', 'SQLTRACE_BUFFER_FLUSH', 'SLEEP_TASK', 'BROKER_TO_FLUSH', 'SP_SERVER_DIAGNOSTICS_SLEEP') AND wait_time_ms > 0`
	wRows, _ := db.Query(wQuery)
	var wSnap models.WaitSnapshot
	wSnap.Timestamp = time.Now().Format("2006-01-02 15:04:05")
	if wRows != nil {
		defer wRows.Close()
		for wRows.Next() {
			var wt string
			var wtMs float64
			if err := wRows.Scan(&wt, &wtMs); err == nil {
				prevMs, exists := metrics.PrevWaitStats[wt]
				metrics.PrevWaitStats[wt] = wtMs
				if exists && wtMs >= prevMs {
					deltaSec := (wtMs - prevMs) / 60.0
					if strings.HasPrefix(wt, "PAGEIOLATCH") {
						wSnap.DiskRead += deltaSec
					} else if strings.HasPrefix(wt, "LCK_") {
						wSnap.Blocking += deltaSec
					} else if strings.HasPrefix(wt, "CXPACKET") || strings.HasPrefix(wt, "CXCONSUMER") {
						wSnap.Parallelism += deltaSec
					} else {
						wSnap.Other += deltaSec
					}
				}
			}
		}
	}
	metrics.WaitHistory = append(metrics.WaitHistory, wSnap)
	if len(metrics.WaitHistory) > 960 {
		metrics.WaitHistory = metrics.WaitHistory[1:]
	}

	// Virtual File IO Stats (Cumulative Deltas measuring latency scaling across 15 sec reads)
	fQuery := `SELECT DB_NAME(vfs.database_id), mf.physical_name, mf.type_desc, vfs.num_of_reads, vfs.num_of_writes, vfs.io_stall_read_ms, vfs.io_stall_write_ms FROM sys.dm_io_virtual_file_stats(NULL, NULL) AS vfs JOIN sys.master_files AS mf WITH (NOLOCK) ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id`
	fRows, _ := db.Query(fQuery)
	var fSnap models.FileIOSnapshot
	fSnap.Timestamp = time.Now().Format("2006-01-02 15:04:05")
	if fRows != nil {
		defer fRows.Close()
		for fRows.Next() {
			var f models.FileIOStat
			var r, w, sr, sw int64
			if err := fRows.Scan(&f.DatabaseName, &f.PhysicalName, &f.FileType, &r, &w, &sr, &sw); err == nil {
				key := f.DatabaseName + ":" + f.PhysicalName
				f.NumOfReads = r
				f.NumOfWrites = w
				f.IoStallReadMs = sr
				f.IoStallWriteMs = sw
				metrics.PrevFileStats[key] = f
				if prevF, exists := prev.PrevFileStats[key]; exists {
					deltaR := float64(r - prevF.NumOfReads)
					deltaW := float64(w - prevF.NumOfWrites)
					deltaSR := float64(sr - prevF.IoStallReadMs)
					deltaSW := float64(sw - prevF.IoStallWriteMs)
					if deltaR > 0 {
						f.ReadLatencyMs = deltaSR / deltaR
					}
					if deltaW > 0 {
						f.WriteLatencyMs = deltaSW / deltaW
					}
					fSnap.Files = append(fSnap.Files, f)
					metrics.FileStats = append(metrics.FileStats, f)
				}
			}
		}
	}
	metrics.FileHistory = append(metrics.FileHistory, fSnap)
	if len(metrics.FileHistory) > 240 {
		metrics.FileHistory = metrics.FileHistory[1:]
	}

	// 5. Disk Usage Profile (Aggregate of Data vs Log vs Free mapped entirely across physically isolated Data sources)
	if metrics.DiskByDB == nil {
		metrics.DiskByDB = make(map[string]models.DiskStat)
	}
	diskQuery := `
		SELECT 
			ISNULL(DB_NAME(database_id), 'Unknown'),
			SUM(CASE WHEN type=0 THEN size * 8.0/1024.0 ELSE 0 END) as Data,
			SUM(CASE WHEN type=1 THEN size * 8.0/1024.0 ELSE 0 END) as Log
		FROM sys.master_files
		GROUP BY database_id
	`
	dRows, errD := db.Query(diskQuery)
	if errD == nil {
		defer dRows.Close()
		for dRows.Next() {
			var dbName string
			var d models.DiskStat
			if err := dRows.Scan(&dbName, &d.DataMB, &d.LogMB); err == nil {
				metrics.DiskByDB[dbName] = d
				metrics.DiskUsage.DataMB += d.DataMB
				metrics.DiskUsage.LogMB += d.LogMB
			}
		}
	}

	// 6. Top Active Queries - Significant Query Filter
	// Capture queries meeting ANY of these criteria:
	//   - cpu_time > 50 ms
	//   - total_elapsed_time > 5 seconds (5000000 microseconds)
	//   - logical_reads > 5000
	//   - blocked sessions with wait_time > 5 seconds
	// Note: total_elapsed_time is in MICROseconds in dm_exec_requests
	// Filter out system sessions (is_user_process = 1) and own SPID
	metrics.TopQueries = []models.QueryStat{}

	// First, capture currently running queries with significant resource usage
	runningSQL := `
		SELECT TOP 20
			ISNULL(s.login_name, 'System') as login_name,
			ISNULL(s.program_name, 'System') as program_name,
			ISNULL(DB_NAME(r.database_id), 'Unknown') as database_name,
			CASE WHEN r.sql_handle IS NOT NULL THEN ISNULL(LEFT(t.text, 500), 'Unknown') ELSE 'Unknown' END as query_text,
			ISNULL(r.wait_type, 'RUNNING') as wait_type,
			ISNULL(r.cpu_time, 0) as cpu_time_ms,
			ISNULL(r.total_elapsed_time, 0) / 1000.0 as exec_time_ms,
			ISNULL(r.logical_reads, 0) as logical_reads,
			1 as execution_count
		FROM sys.dm_exec_requests r
		INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE s.is_user_process = 1
		  AND r.session_id <> @@SPID
		  AND (r.cpu_time > 50 OR r.total_elapsed_time > 5000000 OR r.logical_reads > 5000)
		ORDER BY r.total_elapsed_time DESC
	`
	rows, err := db.Query(runningSQL)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var q models.QueryStat
			var login, program, dbName sql.NullString

			if err := rows.Scan(&login, &program, &dbName, &q.QueryText, &q.WaitType, &q.CPUTimeMs, &q.ExecTimeMs, &q.LogicalReads, &q.ExecutionCount); err == nil {
				q.LoginName = login.String
				q.ProgramName = program.String
				q.DatabaseName = dbName.String
				metrics.TopQueries = append(metrics.TopQueries, q)
			}
		}
	}

	// Also capture from plan cache for recent high-CPU queries meeting significant criteria
	cacheSQL := `
		SELECT TOP 20
			ISNULL(s.login_name, 'System') as login_name,
			ISNULL(s.program_name, 'System') as program_name,
			ISNULL(DB_NAME(CAST(f.value AS INT)), 'Unknown') as database_name,
			ISNULL(LEFT(t.text, 500), 'Unknown') as query_text,
			'PLAN_CACHE' as wait_type,
			ISNULL(qs.total_worker_time / NULLIF(qs.execution_count, 0), 0) as cpu_time_ms,
			ISNULL(qs.total_elapsed_time / NULLIF(qs.execution_count, 0), 0) / 1000.0 as exec_time_ms,
			ISNULL(qs.total_logical_reads / NULLIF(qs.execution_count, 0), 0) as logical_reads,
			ISNULL(qs.execution_count, 1) as execution_count
		FROM sys.dm_exec_query_stats qs
		CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) t
		OUTER APPLY sys.dm_exec_plan_attributes(qs.plan_handle) f
		WHERE t.text IS NOT NULL
		  AND (qs.total_worker_time / NULLIF(qs.execution_count, 0) > 50
		       OR qs.total_elapsed_time / NULLIF(qs.execution_count, 0) > 5000
		       OR qs.total_logical_reads / NULLIF(qs.execution_count, 0) > 5000)
		ORDER BY qs.total_worker_time DESC
	`
	cacheRows, cacheErr := db.Query(cacheSQL)
	if cacheErr == nil {
		defer cacheRows.Close()
		for cacheRows.Next() {
			var q models.QueryStat
			var login, program, dbName sql.NullString

			if err := cacheRows.Scan(&login, &program, &dbName, &q.QueryText, &q.WaitType, &q.CPUTimeMs, &q.ExecTimeMs, &q.LogicalReads, &q.ExecutionCount); err == nil {
				q.LoginName = login.String
				q.ProgramName = program.String
				q.DatabaseName = dbName.String
				metrics.TopQueries = append(metrics.TopQueries, q)
			}
		}
	}

	// 7. Dynamic Prometheus Sweep (queries.yml Engine)
	if config.GlobalQueries != nil && len(config.GlobalQueries.Metrics) > 0 {
		metrics.PrometheusData = make(map[string][]map[string]interface{})

		for _, q := range config.GlobalQueries.Metrics {
			// Skip physical overlaps explicitly bounded by the UI logic above
			if q.MetricName == "mssql_cpu_usage_percent" || q.MetricName == "mssql_long_running_queries" {
				continue
			}

			rows, err := db.Query(q.Query)
			if err != nil {
				continue // SQL version compatibility fail or permissions lock
			}

			cols, _ := rows.Columns()
			var queryResults []map[string]interface{}

			for rows.Next() {
				columns := make([]interface{}, len(cols))
				columnPointers := make([]interface{}, len(cols))
				for i := range columns {
					columnPointers[i] = &columns[i]
				}

				if err := rows.Scan(columnPointers...); err == nil {
					rowMap := make(map[string]interface{})
					for i, colName := range cols {
						val := columnPointers[i].(*interface{})
						switch v := (*val).(type) {
						case []byte:
							rowMap[colName] = string(v)
						default:
							rowMap[colName] = v
						}
					}
					queryResults = append(queryResults, rowMap)
				}
			}
			rows.Close()

			if len(queryResults) > 0 {
				metrics.PrometheusData[q.MetricName] = queryResults
			}
		}
	}

	return metrics
}

func (c *MssqlRepository) FetchTopCPUQueries(instanceName string, limit int) ([]map[string]interface{}, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	if limit <= 0 {
		limit = 20
	}

	// Pull TOP 200 queries by CPU time executed in the last 5 minutes
	query := `
	SELECT TOP 200
		DB_NAME(COALESCE(pa.dbid, st.dbid)) AS [Database_Name],
		ISNULL(s.login_name, 'Unknown') AS [Login_Name],
		ISNULL(s.program_name, 'Unknown') AS [Client_App],
		qs.execution_count AS [Total_Executions],
		qs.total_worker_time / 1000 AS [Total_CPU_ms],
		qs.total_elapsed_time / 1000 AS [Total_Elapsed_ms],
		qs.total_logical_reads AS [Total_Logical_Reads],
		CASE 
			WHEN st.objectid IS NOT NULL 
				 AND OBJECT_NAME(st.objectid, COALESCE(pa.dbid, st.dbid)) IS NOT NULL
			THEN 'EXEC ' 
				 + QUOTENAME(OBJECT_SCHEMA_NAME(st.objectid, COALESCE(pa.dbid, st.dbid))) 
				 + '.' 
				 + QUOTENAME(OBJECT_NAME(st.objectid, COALESCE(pa.dbid, st.dbid)))
			ELSE 
				SUBSTRING(st.text,
					(qs.statement_start_offset/2)+1,
					((CASE qs.statement_end_offset 
						WHEN -1 THEN DATALENGTH(st.text)
						ELSE qs.statement_end_offset 
					 END - qs.statement_start_offset)/2) + 1)
		END AS [Query_Text],
		CONVERT(VARCHAR(64), qs.query_hash, 1) AS [Query_Hash],
		qs.last_execution_time
	FROM sys.dm_exec_query_stats qs
	CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
	OUTER APPLY (
		SELECT CONVERT(INT, value) AS dbid
		FROM sys.dm_exec_plan_attributes(qs.plan_handle)
		WHERE attribute = 'dbid'
	) pa
	OUTER APPLY (
		SELECT TOP 1 
			ses.login_name,
			ses.program_name
		FROM sys.dm_exec_sessions ses
		JOIN sys.dm_exec_connections con 
			ON ses.session_id = con.session_id
		WHERE con.most_recent_sql_handle = qs.sql_handle
	) s
	WHERE 
		qs.last_execution_time >= DATEADD(MINUTE,-5,GETDATE())
		AND COALESCE(pa.dbid, st.dbid) > 4
		AND st.text NOT LIKE '%sys.%'
		AND st.text NOT LIKE '%sp_server_diagnostics%'
		AND st.text NOT LIKE '%sp_readerrorlog%'
		AND st.text NOT LIKE '%BACKUP DATABASE%'
		AND st.text NOT LIKE '%DBCC %'
		AND st.text NOT LIKE '%ALTER INDEX%'
		AND st.text NOT LIKE '%UPDATE STATISTICS%'
		AND st.text NOT LIKE '%dm_exec_query_stats%'
		AND st.text NOT LIKE '%QueryPerformanceSnapshot%'
	ORDER BY qs.total_worker_time DESC`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] FetchTopCPUQueries Error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var rawResults []map[string]interface{}
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
		rawResults = append(rawResults, rowMap)
	}

	log.Printf("[MSSQL] FetchTopCPUQueries: raw query returned %d rows for %s", len(rawResults), instanceName)

	// Swapping Map Delta Logic
	c.mutex.Lock()
	if c.prevQueryCache == nil {
		c.prevQueryCache = make(map[string]map[string]QueryState)
	}
	oldCache, hasOldCache := c.prevQueryCache[instanceName]
	c.mutex.Unlock()

	newCache := make(map[string]QueryState)
	var deltaResults []map[string]interface{}

	for _, row := range rawResults {
		hash, ok := row["Query_Hash"].(string)
		if !ok || hash == "" {
			continue
		}

		var currentExecs int64
		if v, ok := row["Total_Executions"].(int64); ok {
			currentExecs = v
		} else if v, ok := row["Total_Executions"].(int32); ok {
			currentExecs = int64(v)
		}

		var currentCPU float64
		if v, ok := row["Total_CPU_ms"].(float64); ok {
			currentCPU = v
		} else if v, ok := row["Total_CPU_ms"].(float32); ok {
			currentCPU = float64(v)
		} else if v, ok := row["Total_CPU_ms"].(int64); ok {
			currentCPU = float64(v)
		} else if v, ok := row["Total_CPU_ms"].(int32); ok {
			currentCPU = float64(v)
		}

		var currentReads int64
		if v, ok := row["Total_Logical_Reads"].(int64); ok {
			currentReads = v
		} else if v, ok := row["Total_Logical_Reads"].(int32); ok {
			currentReads = int64(v)
		} else if v, ok := row["Total_Logical_Reads"].(float64); ok {
			currentReads = int64(v)
		}

		var currentElapsed float64
		if v, ok := row["Total_Elapsed_ms"].(float64); ok {
			currentElapsed = v
		} else if v, ok := row["Total_Elapsed_ms"].(float32); ok {
			currentElapsed = float64(v)
		} else if v, ok := row["Total_Elapsed_ms"].(int64); ok {
			currentElapsed = float64(v)
		} else if v, ok := row["Total_Elapsed_ms"].(int32); ok {
			currentElapsed = float64(v)
		}

		// Populate the NEW cache for the next polling cycle
		newCache[hash] = QueryState{Executions: currentExecs, CPUMs: currentCPU}

		// If we have previous state, return deltas; otherwise return cumulative so UI isn't empty.
		if hasOldCache {
			if prevState, exists := oldCache[hash]; exists {
				deltaExecs := currentExecs - prevState.Executions
				deltaCPU := currentCPU - prevState.CPUMs

				// Only record if query executed and isn't trivial noise
				if deltaExecs > 0 {
					if deltaCPU > 10.0 || deltaExecs > 5 {
						row["Executions"] = deltaExecs
						row["Total_CPU_ms"] = deltaCPU
						row["Avg_CPU_ms"] = deltaCPU / float64(deltaExecs)
						row["Total_Logical_Reads"] = currentReads
						row["Total_Elapsed_ms"] = currentElapsed
						deltaResults = append(deltaResults, row)
					}
				}
			} else {
				// New query since last scrape - insert cumulative
				row["Executions"] = currentExecs
				row["Total_CPU_ms"] = currentCPU
				if currentExecs > 0 {
					row["Avg_CPU_ms"] = currentCPU / float64(currentExecs)
				} else {
					row["Avg_CPU_ms"] = 0.0
				}
				row["Total_Logical_Reads"] = currentReads
				row["Total_Elapsed_ms"] = currentElapsed
				deltaResults = append(deltaResults, row)
			}
		} else {
			// Baseline call: return cumulative so the table shows results immediately.
			row["Executions"] = currentExecs
			row["Total_CPU_ms"] = currentCPU
			if currentExecs > 0 {
				row["Avg_CPU_ms"] = currentCPU / float64(currentExecs)
			} else {
				row["Avg_CPU_ms"] = 0.0
			}
			row["Total_Logical_Reads"] = currentReads
			row["Total_Elapsed_ms"] = currentElapsed
			deltaResults = append(deltaResults, row)
		}
	}

	// Sort delta results by Total_CPU_ms (delta) descending, then return top 20
	if len(deltaResults) > 1 {
		sort.Slice(deltaResults, func(i, j int) bool {
			iCPU := 0.0
			jCPU := 0.0
			if v, ok := deltaResults[i]["Total_CPU_ms"].(float64); ok {
				iCPU = v
			}
			if v, ok := deltaResults[j]["Total_CPU_ms"].(float64); ok {
				jCPU = v
			}
			return iCPU > jCPU
		})
	}

	// Return only top 20 by delta CPU
	if len(deltaResults) > limit {
		deltaResults = deltaResults[:limit]
	}

	// Swap the maps: The Go Garbage Collector will automatically clean up oldCache
	c.mutex.Lock()
	c.prevQueryCache[instanceName] = newCache
	c.mutex.Unlock()

	return deltaResults, nil
}

func (c *MssqlRepository) FetchLiveKPIs(instanceName string) map[string]interface{} {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return map[string]interface{}{"error": "no connection"}
	}

	result, err := c.CollectKPIs(context.Background(), db)
	if err != nil {
		log.Printf("[Live] KPIs query failed: %v", err)
		return map[string]interface{}{"error": err.Error()}
	}
	return result
}

func (c *MssqlRepository) FetchLiveRunningQueries(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection")
	}
	return c.CollectLiveRunningQueries(context.Background(), db)
}

func (c *MssqlRepository) FetchLiveBlockingChains(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection")
	}
	return c.CollectBlockingChains(db)
}

func (c *MssqlRepository) FetchLiveIOLatency(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection")
	}
	return c.CollectFileIOLatencyForRTD(db)
}

func (c *MssqlRepository) FetchLiveTempDBUsage(instanceName string) (map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection")
	}
	results, err := c.CollectTempDBUsage(db)
	if err != nil {
		return nil, err
	}

	summary := map[string]interface{}{
		"files": results,
	}
	return summary, nil
}

func (c *MssqlRepository) FetchLiveWaitStats(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection")
	}
	return c.CollectWaitStats(db)
}

func (c *MssqlRepository) FetchLiveConnectionsByApp(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection")
	}
	return c.CollectConnectionStats(db)
}
