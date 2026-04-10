-- Metric: ts_postgres_latest_metrics
-- Source: backend/internal/storage/hot/ts_logger.go:283
-- Target Table: postgres_throughput_metrics
-- Description: Fetches the single most recent PostgreSQL throughput row

SELECT capture_timestamp, tps, cache_hit_pct
FROM postgres_throughput_metrics
WHERE server_instance_name = $1
ORDER BY capture_timestamp DESC
LIMIT 1;
