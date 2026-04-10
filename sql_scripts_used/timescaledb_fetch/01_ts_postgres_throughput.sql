-- Metric: ts_postgres_throughput
-- Source: backend/internal/storage/hot/ts_logger.go:164
-- Target Table: postgres_throughput_metrics
-- Description: Fetches latest PostgreSQL throughput metrics per database

SELECT capture_timestamp, server_instance_name, database_name, tps, cache_hit_pct,
       txn_delta, blks_read_delta, blks_hit_delta
FROM postgres_throughput_metrics
WHERE server_instance_name = $1
ORDER BY capture_timestamp DESC
LIMIT $2;
