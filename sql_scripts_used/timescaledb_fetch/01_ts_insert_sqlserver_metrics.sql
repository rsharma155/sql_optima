-- Metric: ts_insert_sqlserver_metrics
-- Source: backend/internal/storage/hot/ts_logger.go:321
-- Target Table: sqlserver_metrics
-- Description: Inserts a snapshot of SQL Server dashboard metrics

INSERT INTO sqlserver_metrics (
    capture_timestamp, server_instance_name, avg_cpu_load, memory_usage,
    active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
