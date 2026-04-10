-- Metric: ts_sqlserver_metrics_time_range
-- Source: backend/internal/storage/hot/ts_logger.go:129
-- Target Table: sqlserver_metrics
-- Description: Fetches SQL Server metrics within a specific time range

SELECT capture_timestamp, server_instance_name, avg_cpu_load, memory_usage,
       active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
FROM sqlserver_metrics
WHERE server_instance_name = $1
  AND capture_timestamp >= $2
  AND capture_timestamp <= $3
ORDER BY capture_timestamp ASC;
