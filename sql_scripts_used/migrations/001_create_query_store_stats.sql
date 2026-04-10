-- Create TimescaleDB hypertable for Query Store statistics
-- Partitioned by capture_timestamp with 1-day chunks

-- Create the table
CREATE TABLE IF NOT EXISTS query_store_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    query_hash TEXT NOT NULL,
    query_text TEXT NOT NULL,
    executions BIGINT NOT NULL DEFAULT 0,
    avg_duration_ms DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_cpu_ms DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_logical_reads DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_cpu_ms DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_query_store_server_time 
    ON query_store_stats (server_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_query_store_query_hash 
    ON query_store_stats (query_hash);
CREATE INDEX IF NOT EXISTS idx_query_store_database 
    ON query_store_stats (database_name, capture_timestamp DESC);

-- Convert to hypertable (this will fail if already a hypertable)
SELECT create_hypertable('query_store_stats', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

-- Enable compression for older chunks (older than 7 days)
ALTER TABLE query_store_stats SET (
    timescaledb.compression,
    timescaledb.compression_segmentby = 'server_name,database_name,query_hash'
);

-- Add compression policy (compress chunks older than 7 days)
SELECT add_compression_policy('query_store_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- Add a retention policy (drop data older than 30 days) - optional
-- SELECT add_retention_policy('query_store_stats', INTERVAL '30 days', if_not_exists => TRUE);

-- Create a continuous aggregate for hourly summaries (optional but useful)
CREATE MATERIALIZED VIEW IF NOT EXISTS query_store_stats_hourly
WITH (timescaledb.continuous) AS
SELECT 
    time_bucket('1 hour', capture_timestamp) AS bucket,
    server_name,
    database_name,
    query_hash,
    query_text,
    SUM(executions) AS total_executions,
    AVG(avg_duration_ms) AS avg_duration_ms,
    AVG(avg_cpu_ms) AS avg_cpu_ms,
    AVG(avg_logical_reads) AS avg_logical_reads,
    SUM(total_cpu_ms) AS total_cpu_ms
FROM query_store_stats
GROUP BY 1, 2, 3, 4, 5;

-- Add refresh policy for the continuous aggregate
SELECT add_continuous_aggregate_policy('query_store_stats_hourly',
    start_offset => INTERVAL '3 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE);

COMMENT ON TABLE query_store_stats IS 'Stores aggregated Query Store statistics from SQL Server instances';
