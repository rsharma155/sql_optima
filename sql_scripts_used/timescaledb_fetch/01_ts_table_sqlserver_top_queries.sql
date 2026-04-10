-- Metric: ts_table_sqlserver_top_queries
-- Source: backend/internal/storage/hot/storage.go:196
-- Target Table: sqlserver_top_queries (CREATE TABLE)
-- Description: Stores top query snapshots for SQL Server

CREATE TABLE IF NOT EXISTS sqlserver_top_queries (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    login_name TEXT,
    program_name TEXT,
    database_name TEXT,
    query_text TEXT,
    wait_type TEXT,
    cpu_time_ms DOUBLE PRECISION DEFAULT 0,
    exec_time_ms DOUBLE PRECISION DEFAULT 0,
    logical_reads BIGINT DEFAULT 0,
    execution_count BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
