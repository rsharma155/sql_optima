-- Metric: ts_insert_query_store_stats
-- Source: backend/internal/storage/hot/ts_logger.go:640
-- Target Table: sqlserver_query_store_stats
-- Description: Batch inserts Query Store statistics from source database polling

INSERT INTO sqlserver_query_store_stats (
    capture_timestamp, server_instance_name, database_name, query_hash, query_text,
    executions, avg_duration_ms, avg_cpu_ms, avg_logical_reads, total_cpu_ms
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
