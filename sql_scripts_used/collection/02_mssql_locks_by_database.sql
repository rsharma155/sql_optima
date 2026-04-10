-- Metric: mssql_locks_by_database
-- Source: backend/internal/repository/mssql_dashboard.go:119
-- Target Table: sqlserver_lock_history (TimescaleDB)
-- Description: Groups locks and potential deadlocks by database

SELECT 
    ISNULL(DB_NAME(resource_database_id), 'Unknown'),
    COUNT(*),
    SUM(CASE WHEN request_status = 'CONVERT' THEN 1 ELSE 0 END)
FROM sys.dm_tran_locks WITH (NOLOCK)
WHERE resource_database_id > 0
GROUP BY resource_database_id;
