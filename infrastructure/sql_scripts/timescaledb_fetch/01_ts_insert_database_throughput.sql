-- Metric: ts_insert_database_throughput
-- Source: backend/internal/storage/hot/ts_logger.go:903
-- Target Table: sqlserver_database_throughput
-- Description: Batch inserts per-database throughput metrics (seeks, scans, lookups, writes, TPS)

INSERT INTO sqlserver_database_throughput (
    capture_timestamp, server_instance_name, database_name,
    user_seeks, user_scans, user_lookups, user_writes,
    total_reads, total_writes, tps, batch_requests_per_sec
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
