-- Metric: mssql_blocking_details
-- Source: backend/internal/repository/mssql_dashboard.go:143
-- Target Table: N/A (blocking hierarchy)
-- Description: Returns active requests with blocking session details

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
WHERE r.session_id > 50 AND r.session_id <> @@SPID;
