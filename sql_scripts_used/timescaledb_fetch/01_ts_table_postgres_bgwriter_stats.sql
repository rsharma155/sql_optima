-- Metric: ts_table_postgres_bgwriter_stats
-- Source: backend/internal/storage/hot/storage.go:210
-- Target Table: postgres_bgwriter_stats (CREATE TABLE)
-- Description: Stores PostgreSQL BGWriter and checkpointer statistics

CREATE TABLE IF NOT EXISTS postgres_bgwriter_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    checkpoints_timed BIGINT DEFAULT 0,
    checkpoints_req BIGINT DEFAULT 0,
    checkpoint_write_time DOUBLE PRECISION DEFAULT 0,
    checkpoint_sync_time DOUBLE PRECISION DEFAULT 0,
    buffers_checkpoint BIGINT DEFAULT 0,
    buffers_clean BIGINT DEFAULT 0,
    maxwritten_clean BIGINT DEFAULT 0,
    buffers_backend BIGINT DEFAULT 0,
    buffers_alloc BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
