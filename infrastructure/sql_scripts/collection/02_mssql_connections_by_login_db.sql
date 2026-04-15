-- Metric: mssql_connections_by_login_db
-- Source: backend/internal/repository/mssql_dashboard.go:96
-- Target Table: sqlserver_connection_history (TimescaleDB)
-- Description: Groups active connections by login name and database

SELECT 
    ISNULL(s.login_name, 'Unknown'),
    ISNULL(DB_NAME(s.database_id), 'Unknown'),
    COUNT(s.session_id) as active_connections,
    SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END) as active_requests
FROM sys.dm_exec_sessions s WITH (NOLOCK)
WHERE is_user_process = 1
GROUP BY s.login_name, s.database_id;
