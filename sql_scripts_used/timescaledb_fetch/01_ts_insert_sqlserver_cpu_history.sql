-- Metric: ts_insert_sqlserver_cpu_history
-- Source: backend/internal/storage/hot/ts_logger.go:337
-- Target Table: sqlserver_cpu_history
-- Description: Batch inserts CPU history ticks from ring buffer data

INSERT INTO sqlserver_cpu_history (capture_timestamp, server_instance_name, sql_process, system_idle, other_process)
VALUES ($1, $2, $3, $4, $5);
