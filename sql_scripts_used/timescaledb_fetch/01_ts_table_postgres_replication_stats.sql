-- Metric: ts_table_postgres_replication_stats
-- Source: backend/internal/storage/hot/storage.go:262
-- Target Table: postgres_replication_stats (CREATE TABLE)
-- Description: Stores PostgreSQL replication and cluster state metrics

CREATE TABLE IF NOT EXISTS postgres_replication_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    is_primary BOOLEAN DEFAULT false,
    cluster_state TEXT,
    max_lag_mb DOUBLE PRECISION DEFAULT 0,
    wal_gen_rate_mbps DOUBLE PRECISION DEFAULT 0,
    bgwriter_eff_pct DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
