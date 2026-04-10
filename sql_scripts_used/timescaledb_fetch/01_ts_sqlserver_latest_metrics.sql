-- Metric: ts_sqlserver_latest_metrics
-- Source: backend/internal/storage/hot/ts_logger.go:261
-- Target Table: sqlserver_metrics
-- Description: Fetches the single most recent SQL Server metrics row

SELECT capture_timestamp, avg_cpu_load, memory_usage, active_users, total_locks, deadlocks
FROM sqlserver_metrics
WHERE server_instance_name = $1
ORDER BY capture_timestamp DESC
LIMIT 1;
