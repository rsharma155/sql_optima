-- Metric: ts_table_sqlserver_database_throughput
-- Source: backend/internal/storage/hot/storage.go:156
-- Target Table: sqlserver_database_throughput (CREATE TABLE)
-- Description: Stores per-database throughput metrics for SQL Server

CREATE TABLE IF NOT EXISTS sqlserver_database_throughput (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    user_seeks BIGINT DEFAULT 0,
    user_scans BIGINT DEFAULT 0,
    user_lookups BIGINT DEFAULT 0,
    user_writes BIGINT DEFAULT 0,
    total_reads BIGINT DEFAULT 0,
    total_writes BIGINT DEFAULT 0,
    tps DOUBLE PRECISION DEFAULT 0,
    batch_requests_per_sec DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
