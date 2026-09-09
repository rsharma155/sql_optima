-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Consolidated lightweight KPI collection for SQL Server Health Dashboard v2.
-- Metadata:
--   - Version: 1.3
--   - Scope: Instance
--   - Frequency: 15s
--   - Source: sys.dm_os_schedulers, sys.dm_exec_query_memory_grants, sys.dm_os_performance_counters, sys.dm_os_ring_buffers, sys.dm_io_virtual_file_stats
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

SELECT 
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
     ORDER BY [timestamp] DESC), 0) as sql_cpu_pct, -- 1
    (SELECT ISNULL(SUM(runnable_tasks_count), 0) FROM sys.dm_os_schedulers WITH (NOLOCK) WHERE status = 'VISIBLE ONLINE') as runnable_tasks, -- 2
    (SELECT COUNT(*) FROM sys.dm_exec_query_memory_grants WITH (NOLOCK) WHERE grant_time IS NULL) as grants_pending, -- 3
    ISNULL((SELECT CAST(cntr_value AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Page Reads/sec' AND object_name LIKE '%Buffer Manager%'), 0) as page_reads_delta, -- 4
    ISNULL((SELECT CAST(SUM(io_stall_write_ms) / CASE WHEN SUM(num_of_writes) = 0 THEN 1 ELSE SUM(num_of_writes) END AS DOUBLE PRECISION)
     FROM sys.dm_io_virtual_file_stats(NULL, NULL)
     WHERE file_id = 2), 0) as log_write_wait_ms, -- 5
    ISNULL((SELECT CAST(cntr_value AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Batch Requests/sec' AND object_name LIKE '%SQL Statistics%'), 0) as batch_requests_delta, -- 6
    ISNULL((SELECT CAST(cntr_value AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'SQL Compilations/sec' AND object_name LIKE '%SQL Statistics%'), 0) as compilations_delta, -- 7
    ISNULL((SELECT CAST(cntr_value AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Logins/sec' AND object_name LIKE '%SQL Statistics%'), 0) as logins_delta, -- 8
    ISNULL((SELECT CAST(cntr_value/1024.0 AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Target Server Memory (KB)' AND object_name LIKE '%Memory Manager%'), 0) as target_mem_mb, -- 9
    ISNULL((SELECT CAST(cntr_value/1024.0 AS DOUBLE PRECISION) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Total Server Memory (KB)' AND object_name LIKE '%Memory Manager%'), 0) as total_mem_mb, -- 10
    (SELECT COUNT(*) FROM sys.dm_exec_requests WITH (NOLOCK) WHERE blocking_session_id <> 0) as blocked_sessions, -- 11
    (SELECT COUNT(*) FROM sys.dm_exec_sessions WITH (NOLOCK) WHERE is_user_process = 1) as user_connections, -- 12
    ISNULL((SELECT CAST(value_in_use AS INT) FROM sys.configurations WITH (NOLOCK) WHERE name = N'user connections'), 0) as max_connections, -- 12b
    CAST(ISNULL(SERVERPROPERTY('Edition'), 'Unknown') AS NVARCHAR(128)) as edition, -- 13
    ISNULL((SELECT sqlserver_start_time FROM sys.dm_os_sys_info WITH (NOLOCK)), GETDATE()) as start_time, -- 14
    ISNULL((SELECT TOP 1 cntr_value FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Page life expectancy' AND object_name LIKE '%Buffer Manager%'), 0) as ple; -- 15
