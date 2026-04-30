// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server CPU utilization metrics from ring buffer and scheduler monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"log"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectKPIs fetches key performance indicators
func (c *SqlServerRepository) CollectKPIs(ctx context.Context, db *sql.DB) (map[string]interface{}, error) {
	query := `
		/* SQL_OPTIMA */ SELECT  
			(SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.dm_exec_sessions s
			 WHERE s.status = 'running'
			   AND s.is_user_process = 1
			   AND s.database_id > 4
			   AND LOWER(ISNULL(DB_NAME(s.database_id), '')) <> 'distribution'
			   AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
			   AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')) AS active_sessions,
			(SELECT   total_physical_memory_kb/1024 FROM sys.dm_os_sys_memory) AS total_memory_mb,
			(SELECT    available_physical_memory_kb/1024 FROM sys.dm_os_sys_memory) AS available_memory_mb,
			(SELECT   ISNULL(cntr_value, 0) FROM sys.dm_os_performance_counters WHERE counter_name='Batch Requests/sec') AS batch_requests_sec
		OPTION (RECOMPILE)`

	var activeSessions, totalMemory, availableMemory, batchRequests int
	err := db.QueryRowContext(ctx, query).Scan(&activeSessions, &totalMemory, &availableMemory, &batchRequests)
	if err != nil {
		log.Printf("[SQLSERVER] CollectKPIs Error: %v", err)
		return nil, err
	}

	return map[string]interface{}{
		"active_sessions":     activeSessions,
		"total_memory_mb":     totalMemory,
		"available_memory_mb": availableMemory,
		"batch_requests_sec":  batchRequests,
		"used_memory_mb":      totalMemory - availableMemory,
		"memory_usage_pct":    0.0,
	}, nil
}

// CollectCPUMetrics fetches CPU usage from sys.dm_os_ring_buffers
// Returns CPU history array and current CPU load
func (c *SqlServerRepository) CollectCPUMetrics(db *sql.DB) ([]models.CPUTick, float64, error) {
	cpuQuery := `
		DECLARE @ts_now bigint = (SELECT /* SQL_OPTIMA */   cpu_ticks/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info WITH (NOLOCK)); 
		SELECT /* SQL_OPTIMA */   TOP(256)
		    SQLProcessUtilization AS [SQL_Server_CPU], 
		    SystemIdle AS [System_Idle_CPU], 
		    100 - SystemIdle - SQLProcessUtilization AS [Other_Process_CPU],
		    CONVERT(varchar, DATEADD(ms, -1 * (@ts_now - [timestamp]), GETDATE()), 120) AS [Event_Time]
		FROM ( 
		    SELECT /* SQL_OPTIMA */   record.value('(./Record/@id)[1]', 'int') AS record_id, 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int') AS [SystemIdle], 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS [SQLProcessUtilization], [timestamp] 
		    FROM ( 
		        SELECT /* SQL_OPTIMA */   [timestamp], CONVERT(xml, record) AS [record] 
		        FROM sys.dm_os_ring_buffers WITH (NOLOCK)
		        WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR' 
		        AND record LIKE N'%<SystemHealth>%'
		    ) AS x 
		) AS y 
		ORDER BY record_id DESC;
	`
	cpuRows, err := db.Query(cpuQuery)
	if err != nil {
		log.Printf("[SQLSERVER] CPU Query Error: %v", err)
		return nil, 0, err
	}
	defer cpuRows.Close()

	var buffer []models.CPUTick
	for cpuRows.Next() {
		var tick models.CPUTick
		if err := cpuRows.Scan(&tick.SQLProcess, &tick.SystemIdle, &tick.OtherProcess, &tick.EventTime); err == nil {
			buffer = append(buffer, tick)
		}
	}

	// Reverse for chronological order
	for i, j := 0, len(buffer)-1; i < j; i, j = i+1, j-1 {
		buffer[i], buffer[j] = buffer[j], buffer[i]
	}

	var avgCPU float64
	if len(buffer) > 0 {
		avgCPU = buffer[len(buffer)-1].SQLProcess
	}

	return buffer, avgCPU, nil
}

// CollectActiveSessions counts currently running user sessions
func (c *SqlServerRepository) CollectActiveSessions(db *sql.DB) (int, error) {
	sessionQuery := `SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.dm_exec_sessions WHERE is_user_process = 1 AND status = 'running' AND LOWER(ISNULL(login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb') AND LOWER(ISNULL(program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')`
	var count int
	err := db.QueryRow(sessionQuery).Scan(&count)
	return count, err
}

// CollectCPUSchedulerStats collects CPU scheduler and workload group metrics with pressure warnings
func (c *SqlServerRepository) CollectCPUSchedulerStats(ctx context.Context, db *sql.DB) (*models.CPUSchedulerStats, error) {
	query := `
		/* SQL_OPTIMA */
		WITH scheduler_stats AS (
			SELECT
				SUM(runnable_tasks_count) AS runnable_tasks_count,
				SUM(work_queue_count) AS work_queue_count,
				SUM(current_workers_count) AS current_workers_count,
				SUM(active_workers_count) AS active_workers_count,
				SUM(pending_disk_io_count) AS pending_disk_io_count
			FROM sys.dm_os_schedulers
			WHERE status = 'VISIBLE ONLINE'
		),
		request_stats AS (
			SELECT
				COUNT(*) AS total_request_count,
				SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END) AS active_request_count,
				SUM(CASE WHEN status = 'runnable' THEN 1 ELSE 0 END) AS runnable_request_count,
				SUM(CASE WHEN blocking_session_id > 0 THEN 1 ELSE 0 END) AS blocked_task_count
			FROM sys.dm_exec_requests
		),
		node_stats AS (
			SELECT
				COUNT(*) AS total_node_count,
				SUM(CASE WHEN node_state_desc LIKE '%ONLINE%' THEN 1 ELSE 0 END) AS nodes_online_count
			FROM sys.dm_os_nodes
			WHERE node_id < 64
		)
		SELECT
			GETUTCDATE() AS capture_timestamp,
			osi.cpu_count,
			osi.scheduler_count,
			osi.max_workers_count,
			ss.runnable_tasks_count,
			ss.work_queue_count,
			ss.current_workers_count,
			ss.active_workers_count,
			ss.pending_disk_io_count,
			rs.total_request_count,
			rs.active_request_count,
			rs.runnable_request_count,
			rs.blocked_task_count,
			CAST(ss.runnable_tasks_count * 100.0 / NULLIF(osi.cpu_count,0) AS DECIMAL(8,2)) AS runnable_percent,
			CASE WHEN ss.active_workers_count >= osi.max_workers_count * 0.90 THEN 1 ELSE 0 END AS worker_exhaustion_warning,
			CASE WHEN ss.runnable_tasks_count >= osi.cpu_count * 2 THEN 1 ELSE 0 END AS cpu_pressure_warning,
			osm.total_physical_memory_kb,
			osm.available_physical_memory_kb,
			osm.system_memory_state_desc,
			ns.total_node_count,
			ns.nodes_online_count,
			CASE WHEN osm.available_physical_memory_kb < osm.total_physical_memory_kb * 0.10 THEN 1 ELSE 0 END AS memory_pressure_warning
		FROM sys.dm_os_sys_info osi
		CROSS JOIN scheduler_stats ss
		CROSS JOIN request_stats rs
		CROSS JOIN node_stats ns
		CROSS JOIN sys.dm_os_sys_memory osm
		OPTION (RECOMPILE)`

	stats := &models.CPUSchedulerStats{}
	var workerThreadWarning, runnableWarning, memPressureWarning sql.NullInt64

	err := db.QueryRowContext(ctx, query).Scan(
		&stats.CaptureTimestamp,
		&stats.CPUCount,
		&stats.SchedulerCount,
		&stats.MaxWorkersCount,
		&stats.TotalRunnableTasksCount,
		&stats.TotalWorkQueueCount,
		&stats.TotalCurrentWorkersCount,
		&stats.ActiveWorkersCount,
		&stats.PendingDiskIoCount,
		&stats.TotalRequestCount,
		&stats.TotalActiveRequestCount,
		&stats.RunnableRequestCount,
		&stats.TotalBlockedTaskCount,
		&stats.RunnablePercent,
		&workerThreadWarning,
		&runnableWarning,
		&stats.TotalPhysicalMemoryKB,
		&stats.AvailablePhysicalMemoryKB,
		&stats.SystemMemoryStateDesc,
		&stats.TotalNodeCount,
		&stats.NodesOnlineCount,
		&memPressureWarning,
	)
	if err != nil {
		log.Printf("[SQLSERVER] CollectCPUSchedulerStats Error: %v", err)
		return nil, err
	}

	stats.WorkerThreadExhaustionWarning = workerThreadWarning.Int64 == 1
	stats.RunnableTasksWarning = runnableWarning.Int64 == 1
	stats.PhysicalMemoryPressureWarning = memPressureWarning.Int64 == 1
	stats.BlockedTasksWarning = stats.TotalBlockedTaskCount > 0

	return stats, nil
}

// CollectServerProperties collects server hardware properties
func (c *SqlServerRepository) CollectServerProperties(ctx context.Context, db *sql.DB) (*models.ServerProperties, error) {
	query := `		
		/* SQL_OPTIMA */ SELECT   
			GETDATE() AS capture_timestamp,
			osi.cpu_count,
			osi.hyperthread_ratio,
			osi.socket_count,
			osi.cores_per_socket,
			osi.physical_memory_kb / 1024.0 / 1024.0 AS physical_memory_gb,
			osi.virtual_memory_kb / 1024.0 / 1024.0 AS virtual_memory_gb,
			'' AS cpu_type,
			CASE WHEN osi.hyperthread_ratio < osi.cpu_count THEN 1 ELSE 0 END AS hyperthread_enabled,
			(SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.dm_os_nodes WHERE node_id < 64) AS numa_nodes,
			osi.max_workers_count,
			CONVERT(VARCHAR(64), HASHBYTES('SHA2_256', 
				CONCAT(osi.cpu_count, osi.hyperthread_ratio, osi.socket_count, osi.cores_per_socket, 
					   osi.physical_memory_kb)), 2) AS properties_hash
		FROM sys.dm_os_sys_info osi
		OPTION (RECOMPILE)`

	props := &models.ServerProperties{}
	var hyperthreadEnabled sql.NullInt64

	err := db.QueryRowContext(ctx, query).Scan(
		&props.CaptureTimestamp,
		&props.CPUCount,
		&props.HyperthreadRatio,
		&props.SocketCount,
		&props.CoresPerSocket,
		&props.PhysicalMemoryGB,
		&props.VirtualMemoryGB,
		&props.CPUType,
		&hyperthreadEnabled,
		&props.NUMANodes,
		&props.MaxWorkersCount,
		&props.PropertiesHash,
	)
	if err != nil {
		log.Printf("[SQLSERVER] CollectServerProperties Error: %v", err)
		return nil, err
	}

	props.HyperthreadEnabled = hyperthreadEnabled.Int64 == 1

	return props, nil
}

func (c *SqlServerRepository) GetDB(instanceName string) (*sql.DB, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	db, ok := c.conns[instanceName]
	return db, ok
}
