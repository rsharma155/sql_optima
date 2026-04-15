-- Metric: ts_postgres_throughput_time_range
-- Source: backend/internal/storage/hot/ts_logger.go:194
-- Target Table: postgres_throughput_metrics
-- Description: Fetches PostgreSQL throughput metrics within a specific time range

SELECT capture_timestamp, server_instance_name, database_name, tps, cache_hit_pct,
       txn_delta, blks_read_delta, blks_hit_delta
FROM postgres_throughput_metrics
WHERE server_instance_name = $1
  AND capture_timestamp >= $2
  AND capture_timestamp <= $3
ORDER BY capture_timestamp ASC;
