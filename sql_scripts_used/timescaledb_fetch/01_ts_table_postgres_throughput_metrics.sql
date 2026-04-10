-- Metric: ts_table_postgres_throughput_metrics
-- Source: backend/internal/storage/hot/storage.go:243
-- Target Table: postgres_throughput_metrics (CREATE TABLE)
-- Description: Stores PostgreSQL per-database throughput metrics (TPS, cache hit %)

CREATE TABLE IF NOT EXISTS postgres_throughput_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    tps DOUBLE PRECISION DEFAULT 0,
    cache_hit_pct DOUBLE PRECISION DEFAULT 0,
    txn_delta BIGINT DEFAULT 0,
    blks_read_delta BIGINT DEFAULT 0,
    blks_hit_delta BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
