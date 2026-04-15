-- Metric: ts_table_sqlserver_query_store_stats
-- Source: backend/internal/storage/hot/storage.go:170
-- Target Table: sqlserver_query_store_stats (CREATE TABLE)
-- Description: Stores Query Store statistics from SQL Server

CREATE TABLE IF NOT EXISTS sqlserver_query_store_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    query_hash TEXT,
    query_text TEXT,
    plan_id BIGINT,
    is_internal_query BOOLEAN DEFAULT FALSE,
    executions BIGINT DEFAULT 0,
    avg_duration_ms DOUBLE PRECISION DEFAULT 0,
    min_duration_ms DOUBLE PRECISION DEFAULT 0,
    max_duration_ms DOUBLE PRECISION DEFAULT 0,
    stddev_duration_ms DOUBLE PRECISION DEFAULT 0,
    avg_cpu_ms DOUBLE PRECISION DEFAULT 0,
    min_cpu_ms DOUBLE PRECISION DEFAULT 0,
    max_cpu_ms DOUBLE PRECISION DEFAULT 0,
    avg_logical_reads DOUBLE PRECISION DEFAULT 0,
    avg_physical_reads DOUBLE PRECISION DEFAULT 0,
    avg_rowcount DOUBLE PRECISION DEFAULT 0,
    total_cpu_ms DOUBLE PRECISION DEFAULT 0,
    total_duration_ms DOUBLE PRECISION DEFAULT 0,
    total_logical_reads DOUBLE PRECISION DEFAULT 0,
    total_physical_reads DOUBLE PRECISION DEFAULT 0,
    runtime_stats_interval_id BIGINT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
