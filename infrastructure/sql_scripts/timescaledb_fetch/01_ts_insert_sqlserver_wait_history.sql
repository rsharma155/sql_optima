-- Metric: ts_insert_sqlserver_wait_history
-- Source: backend/internal/storage/hot/ts_logger.go:374
-- Target Table: sqlserver_wait_history
-- Description: Batch inserts wait stats categorized into disk_read, blocking, parallelism, and other

INSERT INTO sqlserver_wait_history (capture_timestamp, server_instance_name, wait_type, disk_read_ms_per_sec, blocking_ms_per_sec, parallelism_ms_per_sec, other_ms_per_sec)
VALUES ($1, $2, $3, $4, $5, $6, $7);
