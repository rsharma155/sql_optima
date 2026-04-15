-- Metric: ts_insert_sqlserver_disk_history
-- Source: backend/internal/storage/hot/ts_logger.go:463
-- Target Table: sqlserver_disk_history
-- Description: Batch inserts disk usage (data, log, free MB) per database

INSERT INTO sqlserver_disk_history (capture_timestamp, server_instance_name, database_name, data_mb, log_mb, free_mb)
VALUES ($1, $2, $3, $4, $5, $6);
