-- Metric: ts_table_postgres_archiver_stats
-- Source: backend/internal/storage/hot/storage.go:224
-- Target Table: postgres_archiver_stats (CREATE TABLE)
-- Description: Stores PostgreSQL WAL archiver statistics

CREATE TABLE IF NOT EXISTS postgres_archiver_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id TEXT NOT NULL,
    archived_count BIGINT DEFAULT 0,
    failed_count BIGINT DEFAULT 0,
    last_archived_wal TEXT,
    last_archived_time TIMESTAMPTZ,
    last_failed_wal TEXT,
    last_failed_time TIMESTAMPTZ,
    stats_reset TIMESTAMPTZ,
    failed_count_delta BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
