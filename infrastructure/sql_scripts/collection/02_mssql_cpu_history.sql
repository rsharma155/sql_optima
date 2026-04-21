-- Metric: mssql_cpu_history
-- Source: backend/internal/repository/mssql_dashboard.go:48
-- Target Table: sqlserver_cpu_history (TimescaleDB)
-- Description: Extracts 256-minute chronological CPU history from ring buffers

DECLARE @ts_now bigint = (SELECT /* SQL_OPTIMA */   cpu_tick/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info WITH (NOLOCK)); 
SELECT /* SQL_OPTIMA */   TOP(256)
    SQLProcessUtilization AS [SQL_Server_CPU], 
    SystemIdle AS [System_Idle_CPU], 
    100 - SystemIdle - SQLProcessUtilization AS [Other_Process_CPU],
    CONVERT(varchar, DATEADD(ms, -1 * (@ts_now - [timestamp]), GETDATE()), 120) AS [Event_Time]
FROM ( 
    SELECT /* SQL_OPTIMA */   record.value('(./Record/@id)[1]', 'int') AS record_id, 
        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int') 
        AS [SystemIdle], 
        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') 
        AS [SQLProcessUtilization], [timestamp] 
    FROM ( 
        SELECT /* SQL_OPTIMA */   [timestamp], CONVERT(xml, record) AS [record] 
        FROM sys.dm_os_ring_buffers WITH (NOLOCK)
        WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR' 
        AND record LIKE N'%<SystemHealth>%'
    ) AS x 
) AS y 
ORDER BY record_id DESC;
