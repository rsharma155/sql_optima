-- Metric: ts_sqlserver_metrics_latest
-- Source: backend/internal/storage/hot/ts_logger.go:74
-- Target Table: sqlserver_metrics, sqlserver_wait_history
-- Description: Fetches latest SQL Server metrics joined with wait history aggregated by minute

WITH latest_metrics AS (
    SELECT capture_timestamp, server_instance_name, avg_cpu_load, memory_usage,
           active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
    FROM sqlserver_metrics
    WHERE server_instance_name = $1
    ORDER BY capture_timestamp DESC
    LIMIT $2
),
wait_agg AS (
    SELECT 
        date_trunc('minute', capture_timestamp) as bucket,
        server_instance_name,
        SUM(disk_read_ms_per_sec) as disk_wait,
        SUM(blocking_ms_per_sec) as lock_wait,
        SUM(parallelism_ms_per_sec) as cpu_wait,
        SUM(other_ms_per_sec) as network_wait
    FROM sqlserver_wait_history
    WHERE server_instance_name = $1
    GROUP BY date_trunc('minute', capture_timestamp), server_instance_name
)
SELECT 
    m.capture_timestamp, m.server_instance_name, m.avg_cpu_load, m.memory_usage,
    m.active_users, m.total_locks, m.deadlocks, m.data_disk_mb, m.log_disk_mb, m.free_disk_mb,
    COALESCE(w.cpu_wait, 0) as cpu_wait,
    COALESCE(w.disk_wait, 0) as disk_wait,
    COALESCE(w.lock_wait, 0) as lock_wait,
    COALESCE(w.network_wait, 0) as network_wait
FROM latest_metrics m
LEFT JOIN wait_agg w ON date_trunc('minute', m.capture_timestamp) = w.bucket AND m.server_instance_name = w.server_instance_name
ORDER BY m.capture_timestamp DESC;
