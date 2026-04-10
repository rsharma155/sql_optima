-- Metric: ts_table_postgres_query_dictionary
-- Source: backend/internal/storage/hot/storage.go:234
-- Target Table: postgres_query_dictionary (CREATE TABLE)
-- Description: Stores query dictionary entries for PostgreSQL (query ID to text mapping)

CREATE TABLE IF NOT EXISTS postgres_query_dictionary (
    server_instance_name TEXT NOT NULL,
    query_id BIGINT NOT NULL,
    query_text TEXT,
    first_seen TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL,
    execution_count BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
