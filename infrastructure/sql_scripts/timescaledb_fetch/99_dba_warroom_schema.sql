-- DBA War Room - SQL Optima
-- Phase 1: Database Schema Setup
-- Run these scripts against your TimescaleDB instance

-- ============================================================================
-- Task 1: Incidents Log Table
-- ============================================================================

-- Create incidents table for incident timeline tracking
CREATE TABLE IF NOT EXISTS optima_incidents (
    time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    severity TEXT NOT NULL,
    category TEXT NOT NULL,
    description TEXT,
    recommendations TEXT,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Convert to hypertable partitioned by time
SELECT create_hypertable('optima_incidents', 'time', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

-- Create index for efficient querying by server and time
CREATE INDEX IF NOT EXISTS idx_incidents_server_time 
    ON optima_incidents (server_instance_name, time DESC);

-- Create index for severity-based queries
CREATE INDEX IF NOT EXISTS idx_incidents_severity 
    ON optima_incidents (severity, time DESC);

-- Create index for category queries
CREATE INDEX IF NOT EXISTS idx_incidents_category 
    ON optima_incidents (category, time DESC);

-- Add compression for older data
ALTER TABLE optima_incidents SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name, severity, category',
    timescaledb.compress_orderby = 'time DESC'
);

-- Add compression policy (compress data older than 7 days)
SELECT add_compression_policy('optima_incidents', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE optima_incidents IS 'DBA War Room: Incident timeline for health scores, wait spikes, and regressed queries';

-- ============================================================================
-- Task 2: Continuous Aggregates for Baselines
-- ============================================================================

-- Verify sqlserver_wait_history table exists, if not create it
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'sqlserver_wait_history') THEN
        CREATE TABLE IF NOT EXISTS sqlserver_wait_history (
            capture_timestamp TIMESTAMPTZ NOT NULL,
            server_instance_name TEXT NOT NULL,
            wait_type TEXT,
            disk_read_ms_per_sec DOUBLE PRECISION DEFAULT 0,
            blocking_ms_per_sec DOUBLE PRECISION DEFAULT 0,
            parallelism_ms_per_sec DOUBLE PRECISION DEFAULT 0,
            other_ms_per_sec DOUBLE PRECISION DEFAULT 0,
            inserted_at TIMESTAMPTZ DEFAULT NOW()
        );
        
        SELECT create_hypertable('sqlserver_wait_history', 'capture_timestamp', 
            chunk_time_interval => INTERVAL '1 day',
            if_not_exists => TRUE);
            
        CREATE INDEX IF NOT EXISTS idx_wait_history_server_time 
            ON sqlserver_wait_history (server_instance_name, capture_timestamp DESC);
    END IF;
END
$$;


-- View 1: Hourly Wait Stats Baseline
-- Groups wait statistics by 1-hour buckets to establish baseline patterns
CREATE MATERIALIZED VIEW IF NOT EXISTS hourly_wait_stats_baseline
WITH (timescaledb.continuous) AS
SELECT 
    time_bucket('1 hour', capture_timestamp) AS time,
    server_instance_name,
    wait_type,
    AVG(disk_read_ms_per_sec) AS avg_disk_read_ms,
    AVG(blocking_ms_per_sec) AS avg_blocking_ms,
    AVG(parallelism_ms_per_sec) AS avg_parallelism_ms,
    AVG(other_ms_per_sec) AS avg_other_ms,
    COUNT(*) AS sample_count
FROM sqlserver_wait_history
GROUP BY 
    time_bucket('1 hour', capture_timestamp),
    server_instance_name,
    wait_type
WITH NO DATA;

-- Add refresh policy to automatically refresh the last 2 hours every 5 minutes
SELECT add_continuous_aggregate_policy('hourly_wait_stats_baseline',
    start_offset => INTERVAL '2 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '5 minutes',
    if_not_exists => TRUE);

-- View 2: Hourly Query Performance Baseline
-- Groups query performance by 1-hour buckets to detect regressions
CREATE MATERIALIZED VIEW IF NOT EXISTS hourly_query_performance_baseline
WITH (timescaledb.continuous) AS
SELECT 
    time_bucket('1 hour', ts) AS time,
    instance_id AS server_instance_name,
    query_hash,
    AVG(total_elapsed_ms / NULLIF(total_executions, 0)) AS avg_exec_time_ms,
    SUM(total_executions) AS total_execution_count,
    AVG(total_cpu_ms / NULLIF(total_executions, 0)) AS avg_cpu_time_ms,
    AVG(total_logical_reads / NULLIF(total_executions, 0)) AS avg_logical_reads,
    COUNT(*) AS sample_count,
    MIN(statement_text) AS sample_query_text
FROM sqlserver_query_metrics_v2
GROUP BY 
    time_bucket('1 hour', ts),
    instance_id,
    query_hash
WITH NO DATA;

-- Add refresh policy for query performance baseline
SELECT add_continuous_aggregate_policy('hourly_query_performance_baseline',
    start_offset => INTERVAL '2 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '5 minutes',
    if_not_exists => TRUE);

-- Create indexes on the materialized views for faster queries
CREATE INDEX IF NOT EXISTS idx_hourly_wait_baseline_time 
    ON hourly_wait_stats_baseline (time DESC, server_instance_name);

CREATE INDEX IF NOT EXISTS idx_hourly_wait_baseline_type 
    ON hourly_wait_stats_baseline (wait_type, time DESC);

CREATE INDEX IF NOT EXISTS idx_hourly_query_baseline_time 
    ON hourly_query_performance_baseline (time DESC, server_instance_name);

CREATE INDEX IF NOT EXISTS idx_hourly_query_baseline_hash 
    ON hourly_query_performance_baseline (query_hash, time DESC);

COMMENT ON MATERIALIZED VIEW hourly_wait_stats_baseline IS 'DBA War Room: Hourly baseline for wait statistics by wait type';
COMMENT ON MATERIALIZED VIEW hourly_query_performance_baseline IS 'DBA War Room: Hourly baseline for query performance by query hash';

-- ============================================================================
-- Additional: Helper function to manually refresh aggregates
-- ============================================================================

-- Function to manually refresh wait stats baseline
CREATE OR REPLACE FUNCTION refresh_wait_baseline()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    CALL refresh_continuous_aggregate('hourly_wait_stats_baseline', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '1 hour');
END
$$;

-- Function to manually refresh query performance baseline  
CREATE OR REPLACE FUNCTION refresh_query_baseline()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    CALL refresh_continuous_aggregate('hourly_query_performance_baseline', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '1 hour');
END
$$;

-- Function to refresh both baselines
CREATE OR REPLACE FUNCTION refresh_all_baselines()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    CALL refresh_continuous_aggregate('hourly_wait_stats_baseline', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '1 hour');
    CALL refresh_continuous_aggregate('hourly_query_performance_baseline', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '1 hour');
    RAISE NOTICE 'Baselines refreshed successfully';
END
$$;

-- ============================================================================
-- Verification: Check the tables and views
-- ============================================================================

SELECT 'optima_incidents' AS table_name, 
       COUNT(*) AS row_count, 
       CASE WHEN COUNT(*) > 0 THEN 'EXISTS' ELSE 'EMPTY' END AS status
FROM optima_incidents
UNION ALL
SELECT 'hourly_wait_stats_baseline', 
       COUNT(*)::bigint, 
       'EXISTS' 
FROM hourly_wait_stats_baseline
UNION ALL
SELECT 'hourly_query_performance_baseline', 
       COUNT(*)::bigint, 
       'EXISTS' 
FROM hourly_query_performance_baseline;