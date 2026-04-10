-- Metric: ts_insert_sqlserver_lock_history
-- Source: backend/internal/storage/hot/ts_logger.go:435
-- Target Table: sqlserver_lock_history
-- Description: Batch inserts lock counts and deadlock counts per database

INSERT INTO sqlserver_lock_history (capture_timestamp, server_instance_name, database_name, total_locks, deadlocks)
VALUES ($1, $2, $3, $4, $5);
