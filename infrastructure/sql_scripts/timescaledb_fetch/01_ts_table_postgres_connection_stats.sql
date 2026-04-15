-- Metric: ts_table_postgres_connection_stats
-- Source: backend/internal/storage/hot/storage.go:254
-- Target Table: postgres_connection_stats (CREATE TABLE)
-- Description: Stores PostgreSQL connection statistics

CREATE TABLE IF NOT EXISTS postgres_connection_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    total_connections INTEGER DEFAULT 0,
    active_connections INTEGER DEFAULT 0,
    idle_connections INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
