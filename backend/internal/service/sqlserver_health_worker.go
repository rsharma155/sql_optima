// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for SQL Server Health Trend metrics (v2).
// Metadata:
//   - Feature: SQL Server Health Dashboard V2
//   - Layer: Service (Worker)
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// perfCounterState holds the last raw counter read per server.
// Access is protected by perfCounterMu.
type perfCounterState struct {
	values     map[string]int64 // counter_name → raw cumulative value
	capturedAt time.Time
}

var (
	perfCounterMu        sync.Mutex
	perfCounterSnapshots = make(map[uuid.UUID]*perfCounterState)
)

// readPerfCounterRatesFromCache reads pre-computed rates from sqlserver_perf_counters
// (written by StartPerfCountersCollector) and avoids a DMV round-trip.
// Returns zeros when tsLogger is nil or when no fresh row exists (first boot).
func readPerfCounterRatesFromCache(ctx context.Context, serverID uuid.UUID, tsLogger *hot.TimescaleLogger) (
	pageReadsPerSec, batchReqPerSec, compilationsPerSec, loginsPerSec float64, err error,
) {
	if tsLogger == nil {
		return 0, 0, 0, 0, nil
	}
	pr, prOk, _ := tsLogger.GetLatestPerfCounter(ctx, serverID, "Page Reads/sec", "")
	br, brOk, _ := tsLogger.GetLatestPerfCounter(ctx, serverID, "Batch Requests/sec", "")
	sc, scOk, _ := tsLogger.GetLatestPerfCounter(ctx, serverID, "SQL Compilations/sec", "")
	ls, lsOk, _ := tsLogger.GetLatestPerfCounter(ctx, serverID, "Logins/sec", "")
	if prOk || brOk || scOk || lsOk {
		return pr, br, sc, ls, nil
	}
	return 0, 0, 0, 0, nil
}

// readPerfCounterRates reads the four rate counters from a single
// fast DMV query and computes per-second rates using the previous snapshot.
// On the first call for a given serverID, all rates are 0 (no previous).
// No WAITFOR delay. No blocking. Query runs in < 5ms.
func readPerfCounterRates(ctx context.Context, db *sql.DB, serverID uuid.UUID) (
	pageReadsPerSec, batchReqPerSec, compilationsPerSec, loginsPerSec float64, err error,
) {
	const q = `/* SQL_OPTIMA */
    SELECT counter_name, cntr_value
    FROM sys.dm_os_performance_counters WITH (NOLOCK)
    WHERE (object_name LIKE '%Buffer Manager%' AND counter_name = 'Page Reads/sec')
       OR (object_name LIKE '%SQL Statistics%'  AND counter_name IN (
           'Batch Requests/sec', 'SQL Compilations/sec', 'Logins/sec'
       ))`

	rows, queryErr := db.QueryContext(ctx, q)
	if queryErr != nil {
		return 0, 0, 0, 0, queryErr
	}
	defer rows.Close()

	current := make(map[string]int64, 4)
	for rows.Next() {
		var name string
		var val int64
		if scanErr := rows.Scan(&name, &val); scanErr == nil {
			current[strings.TrimSpace(name)] = val
		}
	}
	if err = rows.Err(); err != nil {
		return
	}

	now := time.Now().UTC()

	perfCounterMu.Lock()
	prev := perfCounterSnapshots[serverID]
	perfCounterSnapshots[serverID] = &perfCounterState{values: current, capturedAt: now}
	perfCounterMu.Unlock()

	if prev == nil || prev.capturedAt.IsZero() {
		// First run — store snapshot, return zeros (no previous to diff against)
		return 0, 0, 0, 0, nil
	}

	elapsed := now.Sub(prev.capturedAt).Seconds()
	if elapsed <= 0 {
		return 0, 0, 0, 0, nil
	}

	delta := func(name string) float64 {
		d := float64(current[name] - prev.values[name])
		if d < 0 {
			d = 0
		} // counter reset on SQL Server restart
		return d / elapsed
	}

	return delta("Page Reads/sec"),
		delta("Batch Requests/sec"),
		delta("SQL Compilations/sec"),
		delta("Logins/sec"),
		nil
}

func (s *MetricsService) StartSqlServerHealthCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "SQL Server Health KPIs", 30*time.Second)
	slog.Info("[SQLServerHealth] Starting background collector (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Health KPIs", 30*time.Second)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[SQLServerHealth] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.collectSqlServerHealthStats(ctx)
				}
			}
		}
	}()
}

func (s *MetricsService) collectSqlServerHealthStats(ctx context.Context) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		serverID := inst.ServerID

		// Update status via Ping before collection
		s.MsRepo.UpdateInstanceStatus(inst.Name)

		// Fetch basic metrics
		db, ok := s.MsRepo.GetConn(inst.Name)
		if !ok || s.MsRepo.GetInstanceStatus(inst.Name) != "online" {
			continue
		}

		// Comprehensive query for Health V2 Dashboard (KPIs that don't need delta in Go)
		query := `
			/* SQL_OPTIMA */ SELECT 
				ISNULL((SELECT TOP(1) [SQLProcessUtilization]
				 FROM ( 
					SELECT record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS [SQLProcessUtilization], [timestamp] 
					FROM ( 
						SELECT [timestamp], CONVERT(xml, record) AS [record] 
						FROM sys.dm_os_ring_buffers WITH (NOLOCK)
						WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR' 
						AND record LIKE N'%<SystemHealth>%'
					) AS x 
				 ) AS y 
				 ORDER BY [timestamp] DESC), 0) as sql_cpu_pct,
				(SELECT ISNULL(SUM(runnable_tasks_count), 0) FROM sys.dm_os_schedulers WITH (NOLOCK) WHERE status = 'VISIBLE ONLINE') as runnable_tasks,
				(SELECT COUNT(*) FROM sys.dm_exec_query_memory_grants WITH (NOLOCK) WHERE grant_time IS NULL) as grants_pending,
				(SELECT ISNULL(CAST(SUM(io_stall_write_ms) / CASE WHEN SUM(num_of_writes) = 0 THEN 1 ELSE SUM(num_of_writes) END AS DOUBLE PRECISION), 0) 
				 FROM sys.dm_io_virtual_file_stats(NULL, NULL) 
				 WHERE file_id = 2) as log_write_wait_ms,
				(SELECT COUNT(*) FROM sys.dm_exec_requests WITH (NOLOCK) WHERE blocking_session_id <> 0) as blocked_sessions,
				(SELECT COUNT(*) FROM sys.dm_exec_sessions WITH (NOLOCK) WHERE is_user_process = 1) as user_connections,
				ISNULL((SELECT CAST(value_in_use AS INT) FROM sys.configurations WITH (NOLOCK) WHERE name = N'user connections'), 0) as max_connections,
				ISNULL((SELECT CAST(cntr_value/1024.0 AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Target Server Memory (KB)' AND object_name LIKE '%Memory Manager%'), 0) as target_mem_mb,
				ISNULL((SELECT CAST(cntr_value/1024.0 AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Total Server Memory (KB)' AND object_name LIKE '%Memory Manager%'), 0) as total_mem_mb,
				CAST(ISNULL(SERVERPROPERTY('Edition'), 'Unknown') AS NVARCHAR(128)) as edition,
				CAST(ISNULL(SERVERPROPERTY('EngineEdition'), 0) AS INT) as engine_edition,
				ISNULL((SELECT DATEDIFF(SECOND, sqlserver_start_time, GETDATE()) FROM sys.dm_os_sys_info WITH (NOLOCK)), 0) as uptime_seconds,
				ISNULL((SELECT TOP 1 cntr_value FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Page life expectancy' AND object_name LIKE '%Buffer Manager%'), 0) as ple;
		`

		var k models.HealthV2KPIs
		var uptimeSeconds int64
		var engineEdition int
		scanHealth := func(d *sql.DB) error {
			return d.QueryRowContext(ctx, query).Scan(
				&k.SqlCpuPct, &k.RunnableTasks, &k.MemGrantsPending,
				&k.LogWriteWaitMs, &k.BlockedSessions,
				&k.UserConnections, &k.MaxConnections, &k.TargetServerMemoryMB, &k.TotalServerMemoryMB,
				&k.Edition, &engineEdition, &uptimeSeconds, &k.DataCacheLifeLifeSec,
			)
		}
		err := scanHealth(db)
		if err != nil && repository.IsMSSQLConnError(err) {
			// Transport-level disconnect — try to reopen the pool and retry once.
			if s.MsRepo.ReconnectInstance(ctx, inst.Name) {
				if db2, ok2 := s.MsRepo.GetConn(inst.Name); ok2 {
					db = db2
					err = scanHealth(db)
				}
			}
		}
		if err != nil {
			slog.Error("[SQLServerHealth] ERROR: Failed to collect health metrics", "target", inst.Name, "err", err)
			continue
		}

		// Fill in rate metrics — prefer TimescaleDB cache to avoid a DMV round-trip.
		// Falls back to direct DMV read on first boot (before StartPerfCountersCollector
		// has written its first snapshot).
		pr, br, sc, ls, cacheErr := readPerfCounterRatesFromCache(ctx, serverID, s.tsLogger)
		if cacheErr == nil && (pr > 0 || br > 0 || sc > 0 || ls > 0) {
			k.PageReadsPerSec = pr
			k.BatchRequests = br
			k.Compilations = sc
			k.LoginsPerSec = ls
		} else {
			if pr2, br2, sc2, ls2, err := readPerfCounterRates(ctx, db, serverID); err == nil {
				k.PageReadsPerSec = pr2
				k.BatchRequests = br2
				k.Compilations = sc2
				k.LoginsPerSec = ls2
			}
		}

		k.InstanceStatus = "Healthy"
		if k.SqlCpuPct > 90 || k.RunnableTasks > 10 || k.MemGrantsPending > 0 || k.BlockedSessions > 5 {
			k.InstanceStatus = "Warning"
		}
		if k.SqlCpuPct > 98 || k.RunnableTasks > 50 || k.BlockedSessions > 20 {
			k.InstanceStatus = "Critical"
		}

		_ = s.tsLogger.LogSqlServerHealthV2KPIs(ctx, serverID, k, uptimeSeconds)

		// 13b. Update Dashboard Cache for DashboardV2/Triage UI
		cached := s.GetCachedDashboard(serverID)
		cached.InstanceName = inst.Name
		cached.AvgCPULoad = float64(k.SqlCpuPct)
		cached.MemoryUsage = k.TotalServerMemoryMB / 1024.0 // Show in GB for the card
		if k.TargetServerMemoryMB > 0 {
			cached.MemoryUtilizationPct = (k.TotalServerMemoryMB / k.TargetServerMemoryMB) * 100.0
		}
		cached.PLE = k.DataCacheLifeLifeSec
		cached.ActiveUsers = int(k.UserConnections)
		cached.TotalLocks = int(k.BlockedSessions)
		cached.Timestamp = time.Now().Format(time.RFC3339)

		// Aggregate storage for the "Storage Capacity" KPI card
		vols, _ := s.MsRepo.FetchVolumeStats(ctx, inst.Name)
		if len(vols) > 0 {
			minFree := 100.0
			for _, v := range vols {
				if v.VolumeFreePct < minFree {
					minFree = v.VolumeFreePct
				}
			}
			cached.StorageMinFreePct = minFree
			cached.VolumesTracked = len(vols)
		}
		s.SetCachedDashboard(serverID, cached)

		if engineEdition > 0 && s.ServerRepo != nil {
			_ = s.ServerRepo.SetEngineEdition(ctx, serverID, engineEdition)
		}

		// 14. Server Properties (Log on first collection per server, on SQL Server restart, or hourly)
		s.hwSlowMu.Lock()
		propsAlreadyLogged := s.hwPropsLogged[serverID]
		s.hwSlowMu.Unlock()
		if !propsAlreadyLogged || uptimeSeconds < 60 || time.Now().Minute() == 0 {
			propQuery := `
				/* SQL_OPTIMA */
				SELECT
					cpu_count, hyperthread_ratio, physical_memory_kb/1024 as physical_memory_mb,
					virtual_memory_kb/1024 as virtual_memory_mb, socket_count, cores_per_socket,
					max_workers_count, scheduler_count, ms_ticks,
					CONVERT(VARCHAR(30), sqlserver_start_time, 126) AS sqlserver_start_time
				FROM sys.dm_os_sys_info WITH (NOLOCK);`
			var cc, sc, cps, mwc, schedCount int
			var hr, pm, vm float64
			var msTicks int64
			var startTimeStr string
			if err := db.QueryRowContext(ctx, propQuery).Scan(&cc, &hr, &pm, &vm, &sc, &cps, &mwc, &schedCount, &msTicks, &startTimeStr); err == nil {
				props := map[string]interface{}{
					"cpu_count":            cc,
					"hyperthread_ratio":    hr,
					"physical_memory_gb":   pm / 1024.0,
					"virtual_memory_gb":    vm / 1024.0,
					"socket_count":         sc,
					"cores_per_socket":     cps,
					"max_workers_count":    mwc,
					"scheduler_count":      schedCount,
					"ms_ticks":             msTicks,
					"sqlserver_start_time": startTimeStr,
					"cpu_type":             "Unknown",
					"properties_hash":      fmt.Sprintf("%d-%d-%d", cc, sc, int(pm)),
				}
				if logErr := s.tsLogger.LogServerProperties(ctx, serverID, props); logErr == nil {
					s.hwSlowMu.Lock()
					s.hwPropsLogged[serverID] = true
					s.hwSlowMu.Unlock()
				}
			}
		}

		// 14b. CPU Scheduler Stats (every collection cycle — feeds Scheduler Status & Pressure KPIs)
		if sched, err := s.MsRepo.CollectCPUSchedulerStats(ctx, db); err == nil {
			_ = s.tsLogger.LogCPUSchedulerStats(ctx, serverID, map[string]interface{}{
				"max_workers_count":                sched.MaxWorkersCount,
				"scheduler_count":                  sched.SchedulerCount,
				"cpu_count":                        sched.CPUCount,
				"total_runnable_tasks_count":       sched.TotalRunnableTasksCount,
				"total_work_queue_count":           sched.TotalWorkQueueCount,
				"total_current_workers_count":      sched.TotalCurrentWorkersCount,
				"active_workers_count":             sched.ActiveWorkersCount,
				"pending_disk_io_count":            sched.PendingDiskIoCount,
				"avg_runnable_tasks_count":         sched.AvgRunnableTasksCount,
				"total_active_request_count":       sched.TotalActiveRequestCount,
				"total_queued_request_count":       sched.TotalQueuedRequestCount,
				"total_blocked_task_count":         sched.TotalBlockedTaskCount,
				"total_active_parallel_thread_count": sched.TotalActiveParallelThreadCount,
				"runnable_request_count":           sched.RunnableRequestCount,
				"total_request_count":              sched.TotalRequestCount,
				"runnable_percent":                 sched.RunnablePercent,
				"worker_thread_exhaustion_warning": sched.WorkerThreadExhaustionWarning,
				"runnable_tasks_warning":           sched.RunnableTasksWarning,
				"blocked_tasks_warning":            sched.BlockedTasksWarning,
				"queued_requests_warning":          sched.QueuedRequestsWarning,
				"total_physical_memory_kb":         sched.TotalPhysicalMemoryKB,
				"available_physical_memory_kb":     sched.AvailablePhysicalMemoryKB,
				"system_memory_state_desc":         sched.SystemMemoryStateDesc,
				"physical_memory_pressure_warning": sched.PhysicalMemoryPressureWarning,
				"total_node_count":                 sched.TotalNodeCount,
				"nodes_online_count":               sched.NodesOnlineCount,
				"offline_cpu_count":                sched.OfflineCPUCount,
				"offline_cpu_warning":              sched.OfflineCPUWarning,
			})
		}

		// 15. Risk Health Metrics
		riskQuery := `
			/* SQL_OPTIMA */
			SELECT 
				ISNULL((SELECT cntr_value FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Page life expectancy' AND object_name LIKE '%Buffer Manager%'), 0) as ple,
				ISNULL((SELECT (a.cntr_value * 1.0 / b.cntr_value) * 100.0 FROM sys.dm_os_performance_counters a JOIN sys.dm_os_performance_counters b ON a.object_name = b.object_name WHERE a.counter_name = 'Buffer cache hit ratio' AND b.counter_name = 'Buffer cache hit ratio base' AND a.object_name LIKE '%Buffer Manager%'), 0) as bch_pct,
				ISNULL((SELECT MAX(used_log_space_in_percent) FROM sys.dm_db_log_space_usage), 0) as max_log_pct,
				(SELECT ISNULL(COUNT(*), 0) FROM sys.dm_os_ring_buffers WHERE ring_buffer_type = 'RING_BUFFER_SECURITY_ERROR' AND DATEADD(ms, -1 * ((SELECT cpu_tick/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info) - [timestamp]), GETUTCDATE()) > DATEADD(minute, -5, GETUTCDATE())) as failed_logins;`
		var ple, bch, mlp float64
		var fl int
		if err := db.QueryRowContext(ctx, riskQuery).Scan(&ple, &bch, &mlp, &fl); err == nil {
			_ = s.tsLogger.LogSQLServerRiskHealth(ctx, serverID, hot.RiskHealthRow{
				CaptureTimestamp:    time.Now().UTC(),
				ServerID:            serverID,
				BlockingSessions:    k.BlockedSessions,
				MemoryGrantsPending: k.MemGrantsPending,
				FailedLogins5m:      fl,
				TempdbUsedPercent:   0, // Filled if tfList is available below
				MaxLogUsedPercent:   mlp,
				PLE:                 ple,
				CompilationsPerSec:  k.Compilations,
				BatchReqPerSec:      k.BatchRequests,
				BufferCacheHitPct:   bch,
			})
			_ = s.tsLogger.LogSQLServerMemoryHistory(ctx, serverID, ple)
		}

		// 16. Agent Job Metrics
		jobMetrics := s.MsRepo.FetchAgentJobs(ctx, inst.Name)
		if jobMetrics.LastError == "" {
			var jobDetails []map[string]interface{}
			for _, j := range jobMetrics.Jobs {
				jobDetails = append(jobDetails, map[string]interface{}{
					"job_name":        j.JobName,
					"category":        j.Category,
					"description":     j.Description,
					"enabled":          j.Enabled,
					"owner":            j.Owner,
					"created_date":     j.CreatedDate,
					"current_status":   j.CurrentStatus,
					"last_run_date":    j.LastRunDate,
					"last_run_time":    j.LastRunTime,
					"last_run_status":  j.LastRunStatus,
				})
			}
			_ = s.tsLogger.LogSQLServerJobDetails(ctx, serverID, jobDetails)

			var failDetails []map[string]interface{}
			for _, f := range jobMetrics.Failures {
				failDetails = append(failDetails, map[string]interface{}{
					"job_name":  f.JobName,
					"step_name": f.StepName,
					"message":   f.Message,
					"run_date":  f.RunDate,
					"run_time":  f.RunTime,
				})
			}
			_ = s.tsLogger.LogSQLServerJobFailures(ctx, serverID, failDetails)

			_ = s.tsLogger.LogSQLServerJobMetrics(ctx, serverID, map[string]interface{}{
				"total_jobs":      jobMetrics.Summary.TotalJobs,
				"enabled_jobs":    jobMetrics.Summary.EnabledJobs,
				"disabled_jobs":   jobMetrics.Summary.DisabledJobs,
				"running_jobs":    jobMetrics.Summary.RunningJobs,
				"failed_jobs_24h": jobMetrics.Summary.FailedJobs,
			})

			// 16b. Agent job schedules
			if len(jobMetrics.Schedules) > 0 {
				var schedRows []map[string]interface{}
				for _, sched := range jobMetrics.Schedules {
					schedRows = append(schedRows, map[string]interface{}{
						"job_name":    sched.JobName,
						"job_enabled": sched.JobEnabled,
						"schedule_name": sched.ScheduleName,
						"status":      sched.Status,
					})
				}
				_ = s.tsLogger.LogSQLServerJobSchedules(ctx, serverID, schedRows)
			}
		}

		// --- Additional DMV collections ---

		// 1. CPU History
		cpuQuery := `
			/* SQL_OPTIMA */
			DECLARE @ts_now bigint = (SELECT cpu_tick/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info WITH (NOLOCK)); 
			SELECT TOP(30)
				SQLProcessUtilization AS sql_process, 
				SystemIdle AS system_idle, 
				100 - SystemIdle - SQLProcessUtilization AS other_process,
				DATEADD(ms, -1 * (@ts_now - [timestamp]), GETUTCDATE()) AS event_time
			FROM ( 
				SELECT record.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int') AS [SystemIdle], 
					record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS [SQLProcessUtilization], [timestamp] 
				FROM ( 
					SELECT [timestamp], CONVERT(xml, record) AS [record] 
					FROM sys.dm_os_ring_buffers WITH (NOLOCK)
					WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR' 
					AND record LIKE N'%<SystemHealth>%'
				) AS x 
			) AS y 
			ORDER BY timestamp DESC;`
		cRows, err := db.QueryContext(ctx, cpuQuery)
		if err == nil {
			var cpuTicks []map[string]interface{}
			for cRows.Next() {
				var sp, si, op int
				var et time.Time
				if err := cRows.Scan(&sp, &si, &op, &et); err == nil {
					cpuTicks = append(cpuTicks, map[string]interface{}{
						"sql_process":   sp,
						"system_idle":   si,
						"other_process": op,
						"event_time":    et,
					})
				}
			}
			cRows.Close()
			if len(cpuTicks) > 0 {
				_ = s.tsLogger.LogSQLServerCPUHistory(ctx, serverID, cpuTicks)
			}
		}

		// 2. Connection stats
		connQuery := `
			/* SQL_OPTIMA */
			SELECT 
				ISNULL(s.login_name, 'Unknown'),
				ISNULL(DB_NAME(s.database_id), 'Unknown'),
				COUNT(s.session_id) as active_connections,
				SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END) as active_requests
			FROM sys.dm_exec_sessions s WITH (NOLOCK)
			WHERE is_user_process = 1
			GROUP BY s.login_name, s.database_id;`
		connRows, err := db.QueryContext(ctx, connQuery)
		if err == nil {
			connMap := make(map[string]map[string]interface{})
			for connRows.Next() {
				var login, dbname string
				var ac, ar int
				if err := connRows.Scan(&login, &dbname, &ac, &ar); err == nil {
					// We use dbname:login as key to avoid collisions, though LogSQLServerConnectionHistory 
					// will use this as database_name in the table.
					key := dbname + ":" + login
					connMap[key] = map[string]interface{}{
						"login_name":         login,
						"active_connections": ac,
						"active_requests":    ar,
					}
				}
			}
			connRows.Close()
			if len(connMap) > 0 {
				_ = s.tsLogger.LogSQLServerConnectionHistory(ctx, serverID, connMap)
			}
		}

		// 3. Disk usage
		diskQuery := `
			/* SQL_OPTIMA */
			SELECT 
				ISNULL(DB_NAME(database_id), 'Unknown'),
				SUM(CASE WHEN type=0 THEN size * 8.0/1024.0 ELSE 0 END) as Data,
				SUM(CASE WHEN type=1 THEN size * 8.0/1024.0 ELSE 0 END) as Log
			FROM sys.master_files
			GROUP BY database_id;`
		diskRows, err := db.QueryContext(ctx, diskQuery)
		if err == nil {
			diskMap := make(map[string]map[string]interface{})
			for diskRows.Next() {
				var dbname string
				var data, logVal float64
				if err := diskRows.Scan(&dbname, &data, &logVal); err == nil {
					diskMap[dbname] = map[string]interface{}{
						"data_mb": data,
						"log_mb":  logVal,
						"free_mb": 0.0,
					}
				}
			}
			diskRows.Close()
			if len(diskMap) > 0 {
				_ = s.tsLogger.LogSQLServerDiskHistory(ctx, serverID, diskMap)

				// Log to staging for Pulse-level granularity
				var stagingRows []map[string]interface{}
				for dbn, d := range diskMap {
					stagingRows = append(stagingRows, map[string]interface{}{
						"database_name": dbn,
						"total_size_mb": d["data_mb"].(float64) + d["log_mb"].(float64),
					})
				}
				_ = s.tsLogger.LogStagingTier2SQLServer(ctx, serverID, stagingRows)
			}
		}

		// 4. Wait stats history (Categorized summary with wait_type and total time)
		waitQuery := `
			/* SQL_OPTIMA */ SELECT 
				'DISK_READ' as wait_type,
				ISNULL(SUM(CASE WHEN wait_type LIKE 'PAGEIOLATCH_%' OR wait_type = 'WRITELOG' THEN wait_time_ms ELSE 0 END), 0) as wait_time_ms_total,
				ISNULL(SUM(CASE WHEN wait_type LIKE 'PAGEIOLATCH_%' OR wait_type = 'WRITELOG' THEN wait_time_ms ELSE 0 END), 0) as disk_read,
				0.0 as blocking, 0.0 as parallelism, 0.0 as other
			FROM sys.dm_os_wait_stats
			UNION ALL
			SELECT 
				'BLOCKING',
				ISNULL(SUM(CASE WHEN wait_type LIKE 'LCK_M_%' THEN wait_time_ms ELSE 0 END), 0),
				0.0, ISNULL(SUM(CASE WHEN wait_type LIKE 'LCK_M_%' THEN wait_time_ms ELSE 0 END), 0), 0.0, 0.0
			FROM sys.dm_os_wait_stats
			UNION ALL
			SELECT 
				'PARALLELISM',
				ISNULL(SUM(CASE WHEN wait_type = 'CXPACKET' THEN wait_time_ms ELSE 0 END), 0),
				0.0, 0.0, ISNULL(SUM(CASE WHEN wait_type = 'CXPACKET' THEN wait_time_ms ELSE 0 END), 0), 0.0
			FROM sys.dm_os_wait_stats
			UNION ALL
			SELECT 
				'OTHER',
				ISNULL(SUM(CASE WHEN wait_type NOT LIKE 'PAGEIOLATCH_%' AND wait_type <> 'WRITELOG' AND wait_type NOT LIKE 'LCK_M_%' AND wait_type <> 'CXPACKET' THEN wait_time_ms ELSE 0 END), 0),
				0.0, 0.0, 0.0, ISNULL(SUM(CASE WHEN wait_type NOT LIKE 'PAGEIOLATCH_%' AND wait_type <> 'WRITELOG' AND wait_type NOT LIKE 'LCK_M_%' AND wait_type <> 'CXPACKET' THEN wait_time_ms ELSE 0 END), 0)
			FROM sys.dm_os_wait_stats
			WHERE wait_type NOT IN ('DIRTY_PAGE_POLL', 'HADR_FILESTREAM_IOMGR_IOCOMPLETION', 'LAZYWRITER_SLEEP', 'LOGMGR_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 'XE_DISPATCHER_WAIT', 'XE_TIMER_EVENT', 'SQLTRACE_BUFFER_FLUSH', 'SLEEP_TASK', 'BROKER_TO_FLUSH', 'SP_SERVER_DIAGNOSTICS_SLEEP');`
		wRows, err := db.QueryContext(ctx, waitQuery)
		if err == nil {
			var wList []map[string]interface{}
			for wRows.Next() {
				var wt string
				var wtms int64
				var dr, bl, pa, ot float64
				if err := wRows.Scan(&wt, &wtms, &dr, &bl, &pa, &ot); err == nil {
					wList = append(wList, map[string]interface{}{
						"wait_type":          wt,
						"wait_time_ms_total": wtms,
						"disk_read":          dr,
						"blocking":          bl,
						"parallelism":       pa,
						"other":             ot,
					})
				}
			}
			wRows.Close()
			if len(wList) > 0 {
				_ = s.tsLogger.LogSQLServerWaitHistory(ctx, serverID, wList)
			}
		}

		// 5. TempDB File Usage (Executed in tempdb context for accurate FILEPROPERTY values)
		tempdbFilesQuery := `
			/* SQL_OPTIMA */ 
			EXEC tempdb.sys.sp_executesql N'
			SELECT 
				''tempdb'' as database_name,
				name as file_name,
				type_desc as file_type,
				size * 8.0 / 1024.0 as allocated_mb,
				FILEPROPERTY(name, ''SpaceUsed'') * 8.0 / 1024.0 as used_mb,
				(size - FILEPROPERTY(name, ''SpaceUsed'')) * 8.0 / 1024.0 as free_mb,
				CAST(max_size AS float) * 8.0 / 1024.0 as max_size_mb,
				growth * 8.0 / 1024.0 as growth_mb,
				CAST(FILEPROPERTY(name, ''SpaceUsed'') AS float) / CAST(size AS float) * 100.0 as used_percent
			FROM sys.database_files WITH (NOLOCK)';`
		tfRows, err := db.QueryContext(ctx, tempdbFilesQuery)
		if err == nil {
			var tfList []map[string]interface{}
			for tfRows.Next() {
				var dbn, fn, ft string
				var am, um, fm, msm, gm, up float64
				if err := tfRows.Scan(&dbn, &fn, &ft, &am, &um, &fm, &msm, &gm, &up); err == nil {
					tfList = append(tfList, map[string]interface{}{
						"database_name": dbn,
						"file_name":     fn,
						"file_type":     ft,
						"allocated_mb":  am,
						"used_mb":       um,
						"free_mb":       fm,
						"max_size_mb":   msm,
						"growth_mb":     gm,
						"used_percent":  up,
					})
				}
			}
			tfRows.Close()
			if len(tfList) > 0 {
				_ = s.tsLogger.LogTempdbFiles(ctx, serverID, tfList)
			}
		}

		// 6. TempDB Top Consumers
		tempdbConsumersQuery := `
			/* SQL_OPTIMA */ SELECT TOP 10
				s.session_id,
				DB_NAME(r.database_id) as database_name,
				s.login_name,
				s.host_name,
				s.program_name,
				(user_objects_alloc_page_count + internal_objects_alloc_page_count) * 8.0 / 1024.0 as tempdb_mb,
				user_objects_alloc_page_count * 8.0 / 1024.0 as user_objects_mb,
				internal_objects_alloc_page_count * 8.0 / 1024.0 as internal_objects_mb,
				SUBSTRING(st.text, (r.statement_start_offset/2)+1, 
					((CASE r.statement_end_offset WHEN -1 THEN DATALENGTH(st.text) ELSE r.statement_end_offset END - r.statement_start_offset)/2) + 1) as query_text
			FROM sys.dm_db_session_space_usage s WITH (NOLOCK)
			JOIN sys.dm_exec_sessions es WITH (NOLOCK) ON s.session_id = es.session_id
			LEFT JOIN sys.dm_exec_requests r WITH (NOLOCK) ON s.session_id = r.session_id
			OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
			WHERE s.session_id > 50 AND (user_objects_alloc_page_count + internal_objects_alloc_page_count) > 0
			ORDER BY tempdb_mb DESC;`
		tcRows, err := db.QueryContext(ctx, tempdbConsumersQuery)
		if err == nil {
			var tcList []map[string]interface{}
			for tcRows.Next() {
				var sid int
				var dbn, ln, hn, pn string
				var tmb, umb, imb float64
				var qt *string
				if err := tcRows.Scan(&sid, &dbn, &ln, &hn, &pn, &tmb, &umb, &imb, &qt); err == nil {
					qtext := ""
					if qt != nil {
						qtext = *qt
					}
					tcList = append(tcList, map[string]interface{}{
						"session_id":          sid,
						"database_name":       dbn,
						"login_name":          ln,
						"host_name":           hn,
						"program_name":        pn,
						"tempdb_mb":           tmb,
						"user_objects_mb":     umb,
						"internal_objects_mb": imb,
						"query_text":          qtext,
					})
				}
			}
			tcRows.Close()
			if len(tcList) > 0 {
				_ = s.tsLogger.LogTempdbTopConsumers(ctx, serverID, tcList)
			}
		}

		// 13. Procedure Stats — slow-changing; collect every 5 minutes
		s.hwSlowMu.Lock()
		runSlowCollect := time.Since(s.hwLastSlowCollect[serverID]) >= 5*time.Minute
		s.hwSlowMu.Unlock()

		if runSlowCollect {
			procStatsQuery := `
				/* SQL_OPTIMA */
				SELECT TOP 20
					DB_NAME(ps.database_id) as database_name,
					OBJECT_SCHEMA_NAME(ps.object_id, ps.database_id) as schema_name,
					OBJECT_NAME(ps.object_id, ps.database_id) as object_name,
					ps.execution_count,
					ps.total_worker_time / 1000.0 as total_worker_time_ms,
					ps.total_elapsed_time / 1000.0 as total_elapsed_time_ms,
					ps.total_logical_reads,
					ps.total_physical_reads,
					CAST(ps.query_hash AS VARBINARY(8)) as query_hash
				FROM sys.dm_exec_procedure_stats ps WITH (NOLOCK)
				ORDER BY ps.total_worker_time DESC;`
			pRows, err := db.QueryContext(ctx, procStatsQuery)
			if err == nil {
				var pList []map[string]interface{}
				for pRows.Next() {
					var dbn, sn, on string
					var ec, tlr, tpr int64
					var twt, tet float64
					var qh []byte
					if err := pRows.Scan(&dbn, &sn, &on, &ec, &twt, &tet, &tlr, &tpr, &qh); err == nil {
						pList = append(pList, map[string]interface{}{
							"database_name":         dbn,
							"schema_name":           sn,
							"object_name":           on,
							"execution_count":       ec,
							"total_worker_time_ms":  twt,
							"total_elapsed_time_ms": tet,
							"total_logical_reads":   tlr,
							"total_physical_reads":  tpr,
							"query_hash":            fmt.Sprintf("0x%X", qh),
						})
					}
				}
				pRows.Close()
				if len(pList) > 0 {
					_ = s.tsLogger.LogProcedureStats(ctx, serverID, pList)
				}
			}
		}

		// 8. Perf counters are written by StartPerfCountersCollector (LogSqlServerPerfCountersV2).
		// Health KPIs above read from that cache; do not duplicate inserts here.

		// 9. File IO latency is handled by the dedicated StartFileIOLatencyCollector (60s interval,
		// hash-based dedup, zero-row skipping). Do not duplicate it here.

		// 10 & 11. Plan cache and memory clerks — slow-changing; share the 5-minute cadence
		if runSlowCollect {
			planCacheQuery := `
				/* SQL_OPTIMA */ SELECT
					objtype as cache_type,
					SUM(size_in_bytes) / 1048576.0 as size_mb
				FROM sys.dm_exec_cached_plans WITH (NOLOCK)
				GROUP BY objtype;`
			pcRows, err := db.QueryContext(ctx, planCacheQuery)
			if err == nil {
				var pcList []map[string]interface{}
				for pcRows.Next() {
					var ct string
					var sm float64
					if err := pcRows.Scan(&ct, &sm); err == nil {
						pcList = append(pcList, map[string]interface{}{
							"cache_type": ct,
							"size_mb":    sm,
						})
					}
				}
				pcRows.Close()
				if len(pcList) > 0 {
					_ = s.tsLogger.LogSqlServerPlanCache(ctx, serverID, pcList)
				}
			}

			// 11. Memory clerks (full schema with virtual/committed/awe fields)
			if clerkRows, err := s.MsRepo.FetchMemoryClerks(ctx, inst.Name); err == nil && len(clerkRows) > 0 {
				_ = s.tsLogger.LogMemoryClerks(ctx, serverID, clerkRows)
			}

			// 21. Spinlock stats
			if spinRows, err := s.MsRepo.FetchSpinlockStats(ctx, inst.Name); err == nil && len(spinRows) > 0 {
				_ = s.tsLogger.LogSpinlockStats(ctx, serverID, spinRows)
			}

			// 22. Resource Governor workload group stats (scheduler WG)
			if wgRows, err := s.MsRepo.FetchSchedulerWG(ctx, inst.Name); err == nil && len(wgRows) > 0 {
				_ = s.tsLogger.LogSchedulerWG(ctx, serverID, wgRows)
			}

			// 23. Memory grant waiters
			if waitRows, err := s.MsRepo.FetchMemoryGrantWaiters(ctx, inst.Name); err == nil && len(waitRows) > 0 {
				_ = s.tsLogger.LogMemoryGrantWaiters(ctx, serverID, waitRows)
			}

			// 24. Memory grants (per-session detail: session_id, granted_memory_kb, etc.)
			if mgRows, err := s.MsRepo.FetchMemoryGrants(ctx, inst.Name); err == nil && len(mgRows) > 0 {
				_ = s.tsLogger.LogMemoryGrants(ctx, serverID, mgRows)
			}

			// 25. Plan cache health (total_cache_mb, single_use_cache_pct, etc.)
			if pcHealth, err := s.MsRepo.FetchPlanCacheHealth(ctx, inst.Name); err == nil && pcHealth != nil {
				_ = s.tsLogger.LogPlanCacheHealth(ctx, serverID, pcHealth)
			}

			// 26. File I/O latency per database file
			if fioRows, err := s.MsRepo.FetchFileIOLatency(ctx, inst.Name); err == nil && len(fioRows) > 0 {
				_ = s.tsLogger.LogFileIOLatency(ctx, serverID, fioRows)
			}

			s.hwSlowMu.Lock()
			s.hwLastSlowCollect[serverID] = time.Now()
			s.hwSlowMu.Unlock()
		}

		// 12. Memory grants summary
		memGrantsQuery := `
			/* SQL_OPTIMA */ SELECT
				COUNT(CASE WHEN grant_time IS NULL THEN 1 END) as pending_grants,
				COUNT(CASE WHEN grant_time IS NOT NULL THEN 1 END) as active_grants,
				ISNULL(SUM(CASE WHEN grant_time IS NOT NULL THEN granted_memory_kb END) / 1024.0, 0) as granted_memory_mb
			FROM sys.dm_exec_query_memory_grants WITH (NOLOCK);`
		var pgrant, agrant int
		var gmb float64
		err = db.QueryRowContext(ctx, memGrantsQuery).Scan(&pgrant, &agrant, &gmb)
		if err == nil {
			_ = s.tsLogger.LogSqlServerMemoryGrantsV2(ctx, serverID, map[string]interface{}{
				"pending_grants":    pgrant,
				"active_grants":     agrant,
				"granted_memory_mb": gmb,
			})
		}

		// 7. AG Health
		agQuery := `
			/* SQL_OPTIMA */ SELECT 
				ag.name as ag_name,
				ar.replica_server_name,
				db_name(drs.database_id) as database_name,
				drs.is_primary_replica,
				drs.synchronization_state_desc,
				drs.log_send_queue_size as log_send_queue_kb,
				drs.redo_queue_size as redo_queue_kb,
				drs.log_send_rate as log_send_rate_kb,
				drs.redo_rate as redo_rate_kb,
				ISNULL(drs.secondary_lag_seconds, 0) as secondary_lag_seconds
			FROM sys.dm_hadr_database_replica_states drs WITH (NOLOCK)
			JOIN sys.availability_groups ag WITH (NOLOCK) ON drs.group_id = ag.group_id
			JOIN sys.availability_replicas ar WITH (NOLOCK) ON drs.replica_id = ar.replica_id;`
		agRows, err := db.QueryContext(ctx, agQuery)
		if err == nil {
			var agList []map[string]interface{}
			for agRows.Next() {
				var agn, rsn, dbn, ssd string
				var ipr bool
				var lsq, rq, lsr, rr, sls int64
				if err := agRows.Scan(&agn, &rsn, &dbn, &ipr, &ssd, &lsq, &rq, &lsr, &rr, &sls); err == nil {
					agList = append(agList, map[string]interface{}{
						"ag_name":                    agn,
						"replica_server_name":        rsn,
						"database_name":              dbn,
						"is_primary_replica":         ipr,
						"synchronization_state_desc": ssd,
						"log_send_queue_kb":          lsq,
						"redo_queue_kb":               rq,
						"log_send_rate_kb":           lsr,
						"redo_rate_kb":               rr,
						"secondary_lag_seconds":      sls,
					})
				}
			}
			agRows.Close()
			if len(agList) > 0 {
				_ = s.tsLogger.LogAGHealthFromMap(ctx, serverID, agList)
			}
		}

		// 17. Database Throughput
		throughput, err := s.MsRepo.FetchDatabaseThroughput(ctx, serverID, inst.Name)
		if err == nil && len(throughput) > 0 {
			var stats []map[string]interface{}
			for _, t := range throughput {
				stats = append(stats, map[string]interface{}{
					"database_name":          t.DatabaseName,
					"user_seeks":             t.UserSeeks,
					"user_scans":             t.UserScans,
					"user_lookups":           t.UserLookups,
					"user_writes":            t.UserWrites,
					"total_reads":            t.TotalReads,
					"total_writes":           t.TotalWrites,
					"tps":                    t.TPS,
					"batch_requests_per_sec": t.BatchRequestsPerSec,
					"reads":                  t.Reads,
					"writes":                 t.Writes,
					"bytes_read":             t.BytesRead,
					"bytes_written":          t.BytesWritten,
					"read_latency_ms":        t.ReadLatencyMs,
					"write_latency_ms":       t.WriteLatencyMs,
				})
			}
			_ = s.tsLogger.LogDatabaseThroughputFromMap(ctx, serverID, stats)
		}

		// 18. Latch Stats
		latches, err := s.MsRepo.CollectLatchStats(ctx, db)
		if err == nil && len(latches) > 0 {
			_ = s.tsLogger.LogLatchWaits(ctx, serverID, latches)
		}

		// 19. Waiting Tasks
		tasks, err := s.MsRepo.CollectWaitingTasks(ctx, db)
		if err == nil && len(tasks) > 0 {
			_ = s.tsLogger.LogWaitingTasks(ctx, serverID, tasks)
		}

		// 20. Long Running Queries
		longQueries, err := s.MsRepo.CollectLongRunningQueries(ctx, db, 5000)
		if err == nil && len(longQueries) > 0 {
			_ = s.tsLogger.LogLongRunningQueries(ctx, serverID, longQueries)
		}
	}
}
