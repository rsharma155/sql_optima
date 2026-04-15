-- SQL Optima: PostgreSQL Enterprise Monitoring Improvements
-- Run this migration against TimescaleDB

-- ============================================
-- 1. Query Dictionary (Normalize Query Text)
-- ============================================
-- This table stores unique query texts to avoid duplicating massive TEXT blocks
CREATE TABLE IF NOT EXISTS postgres_query_dictionary (
    server_instance_name TEXT NOT NULL,
    query_id BIGINT NOT NULL,
    query_text TEXT NOT NULL,
    first_seen TIMESTAMPTZ DEFAULT NOW(),
    last_seen TIMESTAMPTZ DEFAULT NOW(),
    execution_count BIGINT DEFAULT 0,
    PRIMARY KEY (server_instance_name, query_id)
);

-- Index for finding queries by text search
CREATE INDEX IF NOT EXISTS idx_postgres_query_dict_text 
    ON postgres_query_dictionary USING gin(to_tsvector('english', query_text));

-- Index for recent queries
CREATE INDEX IF NOT EXISTS idx_postgres_query_dict_recent 
    ON postgres_query_dictionary (last_seen DESC);

COMMENT ON TABLE postgres_query_dictionary IS 'Normalizes query text storage - maps query_id to actual SQL text';

-- ============================================
-- 2. PostgreSQL Background Writer & Checkpointer Stats
-- ============================================
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
    avg_checkpoint_interval DOUBLE PRECISION DEFAULT 0,
    avg_bgwriter_interval DOUBLE PRECISION DEFAULT 0,
    checkpoint_completion_time DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('postgres_bgwriter_stats', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_postgres_bgw_server_time 
    ON postgres_bgwriter_stats (server_instance_name, capture_timestamp DESC);

-- Compression with correct segmentby and orderby (TimescaleDB 2.x syntax)
ALTER TABLE postgres_bgwriter_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_bgwriter_stats', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE postgres_bgwriter_stats IS 'Tracks PostgreSQL checkpointer and background writer activity';

-- ============================================
-- PostgreSQL WAL Archiver Stats
-- ============================================
-- 3. PostgreSQL WAL Archiver Stats
-- ============================================
CREATE TABLE IF NOT EXISTS postgres_archiver_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    archived_count BIGINT DEFAULT 0,
    failed_count BIGINT DEFAULT 0,
    last_archived_wal TEXT,
    last_failed_wal TEXT,
    failed_count_delta BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('postgres_archiver_stats', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_postgres_arch_server_time 
    ON postgres_archiver_stats (server_instance_name, capture_timestamp DESC);

-- Index for failed WALs (critical for alerting)
CREATE INDEX IF NOT EXISTS idx_postgres_arch_failed 
    ON postgres_archiver_stats (server_instance_name, failed_count DESC) 
    WHERE failed_count > 0;

-- Compression (TimescaleDB 2.x syntax)
ALTER TABLE postgres_archiver_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_archiver_stats', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE postgres_archiver_stats IS 'Tracks WAL archiving status - failed_count > 0 indicates archiving problems';

-- ============================================
-- 4. Fixed Compression for Existing Tables
-- ============================================

-- sqlserver_metrics compression fix (TimescaleDB 2.x syntax)
ALTER TABLE sqlserver_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);

-- sqlserver_ag_health compression
ALTER TABLE sqlserver_ag_health SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,ag_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);

-- sqlserver_database_throughput compression
ALTER TABLE sqlserver_database_throughput SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);

-- sqlserver_query_store_stats compression
ALTER TABLE sqlserver_query_store_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,query_hash',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);

-- ============================================
-- 5. Materialized Views for Fast Aggregation
-- Note: Continuous aggregates need to be created separately
-- Run these commands individually or in a separate transaction
-- ============================================

-- Checkpoint Activity Summary
CREATE MATERIALIZED VIEW IF NOT EXISTS postgres_checkpoint_summary AS
SELECT 
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    server_instance_name,
    AVG(checkpoints_timed) AS avg_checkpoints_timed,
    AVG(checkpoints_req) AS avg_checkpoints_req,
    SUM(CASE WHEN checkpoints_req > 0 THEN 1 ELSE 0 END) AS req_checkpoint_count,
    AVG(checkpoint_write_time) AS avg_checkpoint_write_time,
    AVG(buffers_checkpoint) AS avg_buffers_checkpoint,
    MAX(buffers_checkpoint) AS max_buffers_checkpoint
FROM postgres_bgwriter_stats
WHERE capture_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY 1, 2;

-- WAL Archive Summary
CREATE MATERIALIZED VIEW IF NOT EXISTS postgres_archive_summary AS
SELECT 
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    server_instance_name,
    SUM(archived_count) AS total_archived,
    SUM(failed_count) AS total_failed,
    MAX(failed_count) AS max_failed_in_period,
    AVG(failed_count_delta) AS avg_failure_rate
FROM postgres_archiver_stats
WHERE capture_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY 1, 2;
