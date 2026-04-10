-- Metric: ts_insert_sqlserver_top_queries
-- Source: backend/internal/storage/hot/ts_logger.go:537
-- Target Table: sqlserver_top_queries
-- Description: Batch inserts top query snapshots with CPU, elapsed time, and logical reads

INSERT INTO sqlserver_top_queries (capture_timestamp, server_instance_name, login_name, program_name, database_name, query_text, wait_type, cpu_time_ms, exec_time_ms, logical_reads, execution_count)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
