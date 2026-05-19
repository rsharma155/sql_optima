-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Consolidated lightweight KPI collection for SQL Server Health Dashboard v2.
-- Metadata:
--   - Version: 1.1
--   - Scope: Instance
--   - Frequency: 15s
--   - Source: sys.dm_os_schedulers, sys.dm_exec_query_memory_grants, sys.dm_os_performance_counters, sys.dm_os_ring_buffers, sys.dm_io_virtual_file_stats
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

SELECT 
    (SELECT TOP(1) [SQLProcessUtilization]
     FROM ( 
        SELECT record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS [SQLProcessUtilization], [timestamp] 
        FROM ( 
            SELECT [timestamp], CONVERT(xml, record) AS [record] 
            FROM sys.dm_os_ring_buffers WITH (NOLOCK)
            WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR' 
            AND record LIKE N'%<SystemHealth>%'
        ) AS x 
     ) AS y 
     ORDER BY [timestamp] DESC) as sql_cpu_pct,
    (SELECT ISNULL(SUM(runnable_tasks_count), 0) FROM sys.dm_os_schedulers WITH (NOLOCK) WHERE status = 'VISIBLE ONLINE') as runnable_tasks,
    (SELECT COUNT(*) FROM sys.dm_exec_query_memory_grants WITH (NOLOCK) WHERE grant_time IS NULL) as grants_pending,
    ISNULL((SELECT CAST(cntr_value AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Page Reads/sec' AND object_name LIKE '%Buffer Manager%'), 0) as page_reads_delta,
    ISNULL((SELECT CAST(SUM(io_stall_write_ms) / CASE WHEN SUM(num_of_writes) = 0 THEN 1 ELSE SUM(num_of_writes) END AS DOUBLE PRECISION)
     FROM sys.dm_io_virtual_file_stats(NULL, NULL)
     WHERE file_id = 2), 0) as log_write_wait_ms,
    ISNULL((SELECT CAST(cntr_value AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Batch Requests/sec' AND object_name LIKE '%SQL Statistics%'), 0) as batch_requests_delta,
    ISNULL((SELECT CAST(cntr_value AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'SQL Compilations/sec' AND object_name LIKE '%SQL Statistics%'), 0) as compilations_delta,
    ISNULL((SELECT CAST(cntr_value AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Logins/sec' AND object_name LIKE '%SQL Statistics%'), 0) as logins_delta,
    ISNULL((SELECT CAST(cntr_value/1024.0 AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Target Server Memory (KB)' AND object_name LIKE '%Memory Manager%'), 0) as target_mem_mb,
    ISNULL((SELECT CAST(cntr_value/1024.0 AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Total Server Memory (KB)' AND object_name LIKE '%Memory Manager%'), 0) as total_mem_mb,
    (SELECT COUNT(*) FROM sys.dm_exec_requests WITH (NOLOCK) WHERE blocking_session_id <> 0) as blocked_sessions,
    (SELECT COUNT(*) FROM sys.dm_exec_sessions WITH (NOLOCK) WHERE is_user_process = 1) as user_connections,
    CAST(SERVERPROPERTY('Edition') AS NVARCHAR(128)) as edition,
    (SELECT sqlserver_start_time FROM sys.dm_os_sys_info WITH (NOLOCK)) as start_time;
