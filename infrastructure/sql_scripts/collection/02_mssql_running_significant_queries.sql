-- Metric: mssql_running_significant_queries
-- Source: backend/internal/repository/mssql_dashboard.go:325
-- Target Table: sqlserver_top_queries (TimescaleDB)
-- Description: Captures currently running queries with significant resource usage (CPU > 50ms, elapsed > 5s, or logical reads > 5000)

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
ORDER BY r.total_elapsed_time DESC;
