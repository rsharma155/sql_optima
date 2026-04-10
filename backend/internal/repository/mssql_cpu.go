package repository

import (
	"context"
	"database/sql"
	"log"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectKPIs fetches key performance indicators
func (c *MssqlRepository) CollectKPIs(ctx context.Context, db *sql.DB) (map[string]interface{}, error) {
	query := `
		SELECT 
			(SELECT COUNT(*) FROM sys.dm_exec_sessions WHERE status='running') AS active_sessions,
			(SELECT total_physical_memory_kb/1024 FROM sys.dm_os_sys_memory) AS total_memory_mb,
			(SELECT available_physical_memory_kb/1024 FROM sys.dm_os_sys_memory) AS available_memory_mb,
			(SELECT ISNULL(cntr_value, 0) FROM sys.dm_os_performance_counters WHERE counter_name='Batch Requests/sec') AS batch_requests_sec
		OPTION (RECOMPILE)`

	var activeSessions, totalMemory, availableMemory, batchRequests int
	err := db.QueryRowContext(ctx, query).Scan(&activeSessions, &totalMemory, &availableMemory, &batchRequests)
	if err != nil {
		log.Printf("[MSSQL] CollectKPIs Error: %v", err)
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
func (c *MssqlRepository) CollectCPUMetrics(db *sql.DB) ([]models.CPUTick, float64, error) {
	cpuQuery := `
		DECLARE @ts_now bigint = (SELECT cpu_ticks/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info WITH (NOLOCK)); 
		SELECT TOP(256)
		    SQLProcessUtilization AS [SQL_Server_CPU], 
		    SystemIdle AS [System_Idle_CPU], 
		    100 - SystemIdle - SQLProcessUtilization AS [Other_Process_CPU],
		    CONVERT(varchar, DATEADD(ms, -1 * (@ts_now - [timestamp]), GETDATE()), 120) AS [Event_Time]
		FROM ( 
		    SELECT record.value('(./Record/@id)[1]', 'int') AS record_id, 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int') AS [SystemIdle], 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS [SQLProcessUtilization], [timestamp] 
		    FROM ( 
		        SELECT [timestamp], CONVERT(xml, record) AS [record] 
		        FROM sys.dm_os_ring_buffers WITH (NOLOCK)
		        WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR' 
		        AND record LIKE N'%<SystemHealth>%'
		    ) AS x 
		) AS y 
		ORDER BY record_id DESC;
	`
	cpuRows, err := db.Query(cpuQuery)
	if err != nil {
		log.Printf("[MSSQL] CPU Query Error: %v", err)
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
func (c *MssqlRepository) CollectActiveSessions(db *sql.DB) (int, error) {
	sessionQuery := `SELECT COUNT(*) FROM sys.dm_exec_sessions WHERE is_user_process = 1 AND status = 'running' AND login_name NOT IN ('dbmonitor_user', 'go-mssqldb') AND program_name NOT IN ('dbmonitor_user', 'go-mssqldb')`
	var count int
	err := db.QueryRow(sessionQuery).Scan(&count)
	return count, err
}

// CollectCPUSchedulerStats collects CPU scheduler and workload group metrics with pressure warnings
func (c *MssqlRepository) CollectCPUSchedulerStats(ctx context.Context, db *sql.DB) (*models.CPUSchedulerStats, error) {
	query := `
		SELECT 
			GETDATE() AS capture_timestamp,
			osi.max_workers_count,
			osi.scheduler_count,
			osi.cpu_count,
			COALESCE(sched.runnable_tasks_count, 0) AS total_runnable_tasks_count,
			COALESCE(sched.work_queue_count, 0) AS total_work_queue_count,
			COALESCE(sched.current_workers_count, 0) AS total_current_workers_count,
			0.0 AS avg_runnable_tasks_count,
			0 AS total_active_request_count,
			0 AS total_queued_request_count,
			0 AS total_blocked_task_count,
			0 AS total_active_parallel_thread_count,
			0 AS runnable_request_count,
			0 AS total_request_count,
			0.0 AS runnable_percent,
			CASE WHEN COALESCE(sched.current_workers_count, 0) >= osi.max_workers_count * 0.90 THEN 1 ELSE 0 END AS worker_thread_exhaustion_warning,
			CASE WHEN COALESCE(sched.runnable_tasks_count, 0) >= osi.cpu_count THEN 1 ELSE 0 END AS runnable_tasks_warning,
			0 AS blocked_tasks_warning,
			0 AS queued_requests_warning,
			osm.total_physical_memory_kb AS total_physical_memory_kb,
			osm.available_physical_memory_kb AS available_physical_memory_kb,
			osm.system_memory_state_desc AS system_memory_state_desc,
			CASE WHEN osm.available_physical_memory_kb < osm.total_physical_memory_kb * 0.10 THEN 1 ELSE 0 END AS physical_memory_pressure_warning,
			(SELECT COUNT(*) FROM sys.dm_os_nodes WHERE node_id < 64) AS total_node_count,
			(SELECT COUNT(*) FROM sys.dm_os_nodes WHERE node_id < 64 AND node_state_desc LIKE '%ONLINE%') AS nodes_online_count,
			(SELECT COUNT(*) FROM sys.dm_os_schedulers WHERE is_online = 0) AS offline_cpu_count,
			CASE WHEN EXISTS (SELECT 1 FROM sys.dm_os_schedulers WHERE is_online = 0) THEN 1 ELSE 0 END AS offline_cpu_warning
		FROM sys.dm_os_sys_info osi
		CROSS JOIN (
			SELECT 
				SUM(runnable_tasks_count) AS runnable_tasks_count,
				SUM(work_queue_count) AS work_queue_count,
				SUM(current_workers_count) AS current_workers_count
			FROM sys.dm_os_schedulers
			WHERE is_online = 1
		) sched
		CROSS JOIN sys.dm_os_sys_memory osm
		OPTION (RECOMPILE)`

	stats := &models.CPUSchedulerStats{}
	var workerThreadWarning, runnableWarning, blockedWarning, queuedWarning, memPressureWarning, offlineWarning sql.NullInt64

	err := db.QueryRowContext(ctx, query).Scan(
		&stats.CaptureTimestamp,
		&stats.MaxWorkersCount,
		&stats.SchedulerCount,
		&stats.CPUCount,
		&stats.TotalRunnableTasksCount,
		&stats.TotalWorkQueueCount,
		&stats.TotalCurrentWorkersCount,
		&stats.AvgRunnableTasksCount,
		&stats.TotalActiveRequestCount,
		&stats.TotalQueuedRequestCount,
		&stats.TotalBlockedTaskCount,
		&stats.TotalActiveParallelThreadCount,
		&stats.RunnableRequestCount,
		&stats.TotalRequestCount,
		&stats.RunnablePercent,
		&workerThreadWarning,
		&runnableWarning,
		&blockedWarning,
		&queuedWarning,
		&stats.TotalPhysicalMemoryKB,
		&stats.AvailablePhysicalMemoryKB,
		&stats.SystemMemoryStateDesc,
		&memPressureWarning,
		&stats.TotalNodeCount,
		&stats.NodesOnlineCount,
		&stats.OfflineCPUCount,
		&offlineWarning,
	)
	if err != nil {
		log.Printf("[MSSQL] CollectCPUSchedulerStats Error: %v", err)
		return nil, err
	}

	stats.WorkerThreadExhaustionWarning = workerThreadWarning.Int64 == 1
	stats.RunnableTasksWarning = runnableWarning.Int64 == 1
	stats.BlockedTasksWarning = blockedWarning.Int64 == 1
	stats.QueuedRequestsWarning = queuedWarning.Int64 == 1
	stats.PhysicalMemoryPressureWarning = memPressureWarning.Int64 == 1
	stats.OfflineCPUWarning = offlineWarning.Int64 == 1

	return stats, nil
}

// CollectServerProperties collects server hardware properties
func (c *MssqlRepository) CollectServerProperties(ctx context.Context, db *sql.DB) (*models.ServerProperties, error) {
	query := `
		SELECT 
			GETDATE() AS capture_timestamp,
			osi.cpu_count,
			osi.hyperthread_ratio,
			osi.socket_count,
			osi.cores_per_socket,
			osi.physical_memory_kb / 1024.0 / 1024.0 AS physical_memory_gb,
			osi.virtual_memory_kb / 1024.0 / 1024.0 AS virtual_memory_gb,
			'' AS cpu_type,
			CASE WHEN osi.hyperthread_ratio < osi.cpu_count THEN 1 ELSE 0 END AS hyperthread_enabled,
			(SELECT COUNT(*) FROM sys.dm_os_nodes WHERE node_id < 64) AS numa_nodes,
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
		log.Printf("[MSSQL] CollectServerProperties Error: %v", err)
		return nil, err
	}

	props.HyperthreadEnabled = hyperthreadEnabled.Int64 == 1

	return props, nil
}

func (c *MssqlRepository) GetDB(instanceName string) (*sql.DB, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	db, ok := c.conns[instanceName]
	return db, ok
}
