-- Metric: ts_insert_postgres_throughput
-- Source: backend/internal/storage/hot/ts_logger.go:502
-- Target Table: postgres_throughput_metrics
-- Description: Inserts PostgreSQL throughput metrics (TPS, cache hit %, deltas)

INSERT INTO postgres_throughput_metrics (capture_timestamp, server_instance_name, database_name, tps, cache_hit_pct, txn_delta, blks_read_delta, blks_hit_delta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
