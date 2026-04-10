-- Metric: mssql_active_sessions
-- Source: backend/internal/repository/mssql_dashboard.go:92
-- Target Table: N/A (dashboard telemetry)
-- Description: Counts active user sessions that are currently running

SELECT COUNT(*) FROM sys.dm_exec_sessions WHERE is_user_process = 1 AND status = 'running';
