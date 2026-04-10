-- Metric: mssql_cpu_usage
-- Source: backend/internal/repository/mssql_stats.go:142
-- Target Table: sqlserver_metrics (TimescaleDB)
-- Description: Fetches physical CPU utilization from ring buffers via XML parsing

SELECT TOP 1 
    record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS [SQLProcessUtilization]
FROM (
    SELECT [timestamp], CONVERT(xml, record) AS [record]
    FROM sys.dm_os_ring_buffers
    WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR'
    AND record LIKE '%<SystemHealth>%'
) AS x ORDER BY [timestamp] DESC;
