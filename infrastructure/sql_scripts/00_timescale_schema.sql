-- ============================================================================
-- SQL Optima: Unified TimescaleDB Schema
-- Consolidated from: init-scripts, sql_scripts, and migrations
-- Version: 1.0.0
-- Last Updated: 2026-04-04
-- 
-- This is the SINGLE SOURCE OF TRUTH for all TimescaleDB tables.
-- All tables are idempotent (IF NOT EXISTS) and safe to run multiple times.
-- ============================================================================

-- Enable TimescaleDB extension (idempotent)
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- ============================================================================
-- SECTION 1: CORE METRICS TABLES
-- ============================================================================

-- --------------------------------------------------------------------------
-- 1.1: Generic System Metrics (Hot Storage)
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS system_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_name TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value DOUBLE PRECISION NOT NULL,
    tags JSONB DEFAULT '{}'
);
SELECT create_hypertable('system_metrics', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_system_metrics_server_name ON system_metrics (server_name);
CREATE INDEX IF NOT EXISTS idx_system_metrics_metric_name ON system_metrics (metric_name);
CREATE INDEX IF NOT EXISTS idx_system_metrics_server_time ON system_metrics (server_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_system_metrics_tags ON system_metrics USING GIN (tags);
ALTER TABLE system_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_name,metric_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
DO $$
BEGIN
    CALL remove_compression_policy('system_metrics', if_exists => TRUE);
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;
SELECT add_compression_policy('system_metrics', INTERVAL '7 days', if_not_exists => TRUE);

-- Create materialized view for 1-minute aggregates
CREATE MATERIALIZED VIEW IF NOT EXISTS system_metrics_1min
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_name,
    metric_name,
    AVG(metric_value) AS metric_value_avg,
    MIN(metric_value) AS metric_value_min,
    MAX(metric_value) AS metric_value_max,
    COUNT(*) AS sample_count
FROM system_metrics
GROUP BY bucket, server_name, metric_name
WITH NO DATA;
-- NOTE:
-- Some clients (e.g., DBeaver + pgjdbc pipeline mode) execute scripts using a "pipeline",
-- where `CREATE MATERIALIZED VIEW ... WITH DATA` (or implicit population) may fail with:
--   "cannot be executed within a pipeline"
-- Continuous aggregates are created without an immediate refresh here; Timescale policies
-- will populate them after creation. If you need data immediately, run a manual refresh:
--   CALL refresh_continuous_aggregate('system_metrics_1min', NOW() - INTERVAL '1 hour', NOW());
DO $$
BEGIN
    CALL add_continuous_aggregate_policy('system_metrics_1min',
        start_offset => INTERVAL '1 hour',
        end_offset => INTERVAL '1 minute',
        schedule_interval => INTERVAL '1 minute',
        if_not_exists => TRUE
    );
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;

-- --------------------------------------------------------------------------
-- 1.2: SQL SERVER - Core Metrics
-- --------------------------------------------------------------------------

-- SQL Server System Metrics (main table)
CREATE TABLE IF NOT EXISTS sqlserver_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    avg_cpu_load DOUBLE PRECISION,
    memory_usage DOUBLE PRECISION,
    active_users INTEGER,
    total_locks INTEGER,
    deadlocks INTEGER,
    data_disk_mb DOUBLE PRECISION,
    log_disk_mb DOUBLE PRECISION,
    free_disk_mb DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_metrics', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_metrics_server ON sqlserver_metrics (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_metrics', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server CPU History
CREATE TABLE IF NOT EXISTS sqlserver_cpu_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    sql_process DOUBLE PRECISION,
    system_idle DOUBLE PRECISION,
    other_process DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_cpu_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_cpu_server ON sqlserver_cpu_history (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_cpu_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_cpu_history', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Memory History (Page Life Expectancy)
CREATE TABLE IF NOT EXISTS sqlserver_memory_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    page_life_expectancy DOUBLE PRECISION,
    memory_type TEXT,
    size_mb DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_memory_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_memory_server ON sqlserver_memory_history (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_memory_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_memory_history', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Wait Statistics History
CREATE TABLE IF NOT EXISTS sqlserver_wait_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    wait_type TEXT,
    wait_time_ms_total DOUBLE PRECISION,
    disk_read_ms_per_sec DOUBLE PRECISION,
    blocking_ms_per_sec DOUBLE PRECISION,
    parallelism_ms_per_sec DOUBLE PRECISION,
    other_ms_per_sec DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_wait_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wait_server ON sqlserver_wait_history (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wait_type ON sqlserver_wait_history (wait_type);
ALTER TABLE sqlserver_wait_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_wait_history', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server File I/O History
CREATE TABLE IF NOT EXISTS sqlserver_file_io_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    physical_name TEXT,
    file_type TEXT,
    read_latency_ms DOUBLE PRECISION,
    write_latency_ms DOUBLE PRECISION,
    num_reads BIGINT,
    num_writes BIGINT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_file_io_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_fileio_server ON sqlserver_file_io_history (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_fileio_db ON sqlserver_file_io_history (database_name);
ALTER TABLE sqlserver_file_io_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_file_io_history', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Connection History
CREATE TABLE IF NOT EXISTS sqlserver_connection_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    login_name TEXT,
    database_name TEXT,
    active_connections INTEGER,
    active_requests INTEGER,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_connection_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_conn_server ON sqlserver_connection_history (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_connection_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_connection_history', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Lock History
CREATE TABLE IF NOT EXISTS sqlserver_lock_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    total_locks INTEGER,
    deadlocks INTEGER,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_lock_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lock_server ON sqlserver_lock_history (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_lock_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_lock_history', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Disk Usage History
CREATE TABLE IF NOT EXISTS sqlserver_disk_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    data_mb DOUBLE PRECISION,
    log_mb DOUBLE PRECISION,
    free_mb DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_disk_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_disk_server ON sqlserver_disk_history (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_disk_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_disk_history', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Top Queries (with Delta CPU tracking)
CREATE TABLE IF NOT EXISTS sqlserver_top_queries (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    login_name TEXT,
    program_name TEXT,
    database_name TEXT,
    query_text TEXT,
    --wait_type TEXT,
    cpu_time_ms BIGINT DEFAULT 0,
    exec_time_ms BIGINT DEFAULT 0,
    logical_reads BIGINT DEFAULT 0,
    execution_count BIGINT DEFAULT 0,
    query_hash TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_top_queries', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_topq_server ON sqlserver_top_queries (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_topq_hash ON sqlserver_top_queries (query_hash) WHERE query_hash IS NOT NULL;
ALTER TABLE sqlserver_top_queries SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_top_queries', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_top_queries IS 'Tracks CPU-intensive queries with delta metrics. Login/Client App may show Unknown/Disconnected for quick background queries.';

-- Query Store Stats (legacy table name)
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
SELECT create_hypertable('query_store_stats', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_query_store_server_time ON query_store_stats (server_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_query_store_query_hash ON query_store_stats (query_hash);
CREATE INDEX IF NOT EXISTS idx_query_store_database ON query_store_stats (database_name, capture_timestamp DESC);
ALTER TABLE query_store_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_name,database_name,query_hash',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('query_store_stats', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE query_store_stats IS 'Stores aggregated Query Store statistics from SQL Server instances';

-- --------------------------------------------------------------------------
-- 1.2.x: POSTGRES - Advanced (Contention / IO / Config drift)
-- --------------------------------------------------------------------------

-- Wait event snapshots (contention taxonomy) from pg_stat_activity
CREATE TABLE IF NOT EXISTS postgres_wait_event_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    wait_event_type TEXT,
    wait_event TEXT,
    sessions_count INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_wait_event_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_waits_server_time ON postgres_wait_event_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_wait_event_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,wait_event_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_wait_event_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- Per-database IO counters from pg_stat_database (UI computes deltas)
CREATE TABLE IF NOT EXISTS postgres_db_io_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    blks_read BIGINT DEFAULT 0,
    blks_hit BIGINT DEFAULT 0,
    temp_files BIGINT DEFAULT 0,
    temp_bytes BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_db_io_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_io_server_time ON postgres_db_io_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_io_db_time ON postgres_db_io_stats (server_instance_name, database_name, capture_timestamp DESC);
ALTER TABLE postgres_db_io_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_db_io_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- Curated pg_settings snapshot for drift tracking
CREATE TABLE IF NOT EXISTS postgres_settings_snapshot (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    name TEXT NOT NULL,
    setting TEXT,
    unit TEXT,
    source TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_settings_snapshot', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_settings_server_time ON postgres_settings_snapshot (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_settings_name ON postgres_settings_snapshot (server_instance_name, name, capture_timestamp DESC);
ALTER TABLE postgres_settings_snapshot SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_settings_snapshot', INTERVAL '14 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 1.3: SQL SERVER - Enterprise Metrics (AG, Throughput, etc.)
-- --------------------------------------------------------------------------

-- SQL Server Database Throughput
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
SELECT create_hypertable('sqlserver_database_throughput', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_db_throughput_server_time ON sqlserver_database_throughput (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_db_throughput_db ON sqlserver_database_throughput (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_database_throughput SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_database_throughput', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_database_throughput IS 'Tracks database-level throughput metrics including TPS, batch requests, and I/O statistics';

-- SQL Server Query Store Stats (Historical)
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
SELECT create_hypertable('sqlserver_query_store_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_qs_stats_server_time ON sqlserver_query_store_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_qs_stats_query_hash ON sqlserver_query_store_stats (query_hash);
CREATE INDEX IF NOT EXISTS idx_qs_stats_database ON sqlserver_query_store_stats (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_query_store_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,query_hash',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_query_store_stats', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_query_store_stats IS 'Stores historical Query Store statistics for bottleneck analysis';

-- SQL Server Availability Group Health
CREATE TABLE IF NOT EXISTS sqlserver_ag_health (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    ag_name TEXT,
    replica_server_name TEXT,
    database_name TEXT,
    replica_role TEXT,
    synchronization_state TEXT,
    synchronization_state_desc TEXT,
    is_primary_replica BOOLEAN,
    log_send_queue_kb BIGINT DEFAULT 0,
    redo_queue_kb BIGINT DEFAULT 0,
    log_send_rate_kb BIGINT DEFAULT 0,
    redo_rate_kb BIGINT DEFAULT 0,
    last_sent_time TIMESTAMPTZ,
    last_received_time TIMESTAMPTZ,
    last_hardened_time TIMESTAMPTZ,
    last_redone_time TIMESTAMPTZ,
    secondary_lag_seconds BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_ag_health', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_ag_health_server_time ON sqlserver_ag_health (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ag_health_ag_name ON sqlserver_ag_health (ag_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ag_health_db ON sqlserver_ag_health (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_ag_health SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,ag_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_ag_health', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_ag_health IS 'Tracks AlwaysOn Availability Group health metrics including sync state and queue sizes';

-- --------------------------------------------------------------------------
-- 1.4: SQL SERVER - Agent Jobs
-- --------------------------------------------------------------------------

-- SQL Server Agent Jobs Summary
CREATE TABLE IF NOT EXISTS sqlserver_job_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    total_jobs INTEGER,
    enabled_jobs INTEGER,
    disabled_jobs INTEGER,
    running_jobs INTEGER,
    failed_jobs_24h INTEGER,
    error_message TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT sqlserver_job_metrics_unique UNIQUE (capture_timestamp, server_instance_name)
);
SELECT create_hypertable('sqlserver_job_metrics', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_job_server ON sqlserver_job_metrics (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_job_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_job_metrics', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Job Details
CREATE TABLE IF NOT EXISTS sqlserver_job_details (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    job_name TEXT NOT NULL,
    job_enabled BOOLEAN,
    job_owner TEXT,
    created_date TEXT,
    current_status TEXT,
    last_run_date INTEGER,
    last_run_time INTEGER,
    last_run_status TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_job_details', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_jobdetail_server ON sqlserver_job_details (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_jobdetail_name ON sqlserver_job_details (job_name);
ALTER TABLE sqlserver_job_details SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,job_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_job_details', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Agent Schedules
CREATE TABLE IF NOT EXISTS sqlserver_agent_schedules (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    job_name TEXT NOT NULL,
    next_run_datetime TEXT,
    job_enabled BOOLEAN,
    schedule_name TEXT,
    status TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_agent_schedules', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_sched_server ON sqlserver_agent_schedules (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_agent_schedules SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,job_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_agent_schedules', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Job Failures
CREATE TABLE IF NOT EXISTS sqlserver_job_failures (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    job_name TEXT,
    step_name TEXT,
    error_message TEXT,
    run_date INTEGER,
    run_time INTEGER,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_job_failures', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_jobfail_server ON sqlserver_job_failures (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_job_failures SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,job_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_job_failures', INTERVAL '7 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- SQL SERVER - CPU Enhancements (merged from 006_cpu_enhancement.sql)
-- --------------------------------------------------------------------------

-- SQL Server Server Properties (hardware / static-ish)
CREATE TABLE IF NOT EXISTS sqlserver_server_properties (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    cpu_count INTEGER DEFAULT 0,
    hyperthread_ratio INTEGER DEFAULT 0,
    socket_count INTEGER DEFAULT 0,
    cores_per_socket INTEGER DEFAULT 0,
    physical_memory_gb DOUBLE PRECISION DEFAULT 0,
    virtual_memory_gb DOUBLE PRECISION DEFAULT 0,
    cpu_type TEXT,
    hyperthread_enabled BOOLEAN DEFAULT FALSE,
    numa_nodes INTEGER DEFAULT 0,
    max_workers_count INTEGER DEFAULT 0,
    properties_hash TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_server_properties', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_server_props_server_time ON sqlserver_server_properties (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_server_properties SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_server_properties', INTERVAL '365 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- SQL SERVER - DBA Homepage (Phase 2)
-- --------------------------------------------------------------------------

-- Risk & Health strip signals (computed from collectors)
CREATE TABLE IF NOT EXISTS sqlserver_risk_health (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    blocking_sessions INTEGER DEFAULT 0,
    memory_grants_pending INTEGER DEFAULT 0,
    failed_logins_5m INTEGER DEFAULT 0,
    tempdb_used_percent DOUBLE PRECISION DEFAULT 0,
    max_log_db_name TEXT DEFAULT '',
    max_log_used_percent DOUBLE PRECISION DEFAULT 0,
    ple DOUBLE PRECISION DEFAULT 0,
    compilations_per_sec DOUBLE PRECISION DEFAULT 0,
    batch_requests_per_sec DOUBLE PRECISION DEFAULT 0,
    buffer_cache_hit_ratio DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_risk_health', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_risk_health_server_time ON sqlserver_risk_health (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_risk_health SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_risk_health', INTERVAL '7 days', if_not_exists => TRUE);

-- Wait deltas by type/category (for wait-category donut)
CREATE TABLE IF NOT EXISTS sqlserver_waits_delta (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    wait_type TEXT NOT NULL,
    wait_category TEXT NOT NULL,
    wait_time_ms_delta DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_waits_delta', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_waits_delta_server_time ON sqlserver_waits_delta (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_waits_delta_category ON sqlserver_waits_delta (server_instance_name, wait_category, capture_timestamp DESC);
ALTER TABLE sqlserver_waits_delta SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,wait_category,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_waits_delta', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server CPU Scheduler Stats (pressure signals)
CREATE TABLE IF NOT EXISTS sqlserver_cpu_scheduler_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    max_workers_count INTEGER DEFAULT 0,
    scheduler_count INTEGER DEFAULT 0,
    cpu_count INTEGER DEFAULT 0,
    total_runnable_tasks_count INTEGER DEFAULT 0,
    total_work_queue_count BIGINT DEFAULT 0,
    total_current_workers_count INTEGER DEFAULT 0,
    avg_runnable_tasks_count DOUBLE PRECISION DEFAULT 0,
    total_active_request_count INTEGER DEFAULT 0,
    total_queued_request_count INTEGER DEFAULT 0,
    total_blocked_task_count INTEGER DEFAULT 0,
    total_active_parallel_thread_count BIGINT DEFAULT 0,
    runnable_request_count INTEGER DEFAULT 0,
    total_request_count INTEGER DEFAULT 0,
    runnable_percent DOUBLE PRECISION DEFAULT 0,
    worker_thread_exhaustion_warning BOOLEAN DEFAULT FALSE,
    runnable_tasks_warning BOOLEAN DEFAULT FALSE,
    blocked_tasks_warning BOOLEAN DEFAULT FALSE,
    queued_requests_warning BOOLEAN DEFAULT FALSE,
    total_physical_memory_kb BIGINT DEFAULT 0,
    available_physical_memory_kb BIGINT DEFAULT 0,
    system_memory_state_desc TEXT,
    physical_memory_pressure_warning BOOLEAN DEFAULT FALSE,
    total_node_count INTEGER DEFAULT 0,
    nodes_online_count INTEGER DEFAULT 0,
    offline_cpu_count INTEGER DEFAULT 0,
    offline_cpu_warning BOOLEAN DEFAULT FALSE,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_cpu_scheduler_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_cpu_scheduler_server_time ON sqlserver_cpu_scheduler_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_cpu_scheduler_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_cpu_scheduler_stats', INTERVAL '30 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- SQL SERVER - Long Running Queries (merged from 03_add_long_running_queries.sql)
-- --------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS sqlserver_long_running_queries (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    session_id INTEGER NOT NULL,
    request_id INTEGER,
    database_name TEXT,
    login_name TEXT,
    host_name TEXT,
    program_name TEXT,
    query_hash TEXT,
    query_text TEXT,
    wait_type TEXT,
    blocking_session_id INTEGER,
    status TEXT,
    cpu_time_ms BIGINT DEFAULT 0,
    total_elapsed_time_ms BIGINT DEFAULT 0,
    reads BIGINT DEFAULT 0,
    writes BIGINT DEFAULT 0,
    granted_query_memory_mb INTEGER DEFAULT 0,
    row_count BIGINT DEFAULT 0,
    percent_complete TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
ALTER TABLE IF EXISTS sqlserver_long_running_queries
    ADD COLUMN IF NOT EXISTS query_hash TEXT;
SELECT create_hypertable('sqlserver_long_running_queries', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_server_time ON sqlserver_long_running_queries (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_database ON sqlserver_long_running_queries (database_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_session ON sqlserver_long_running_queries (session_id);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_queryhash ON sqlserver_long_running_queries (server_instance_name, query_hash, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_blocking ON sqlserver_long_running_queries (blocking_session_id) WHERE blocking_session_id > 0;
ALTER TABLE sqlserver_long_running_queries SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_long_running_queries', INTERVAL '7 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 1.5: SQL SERVER - Advanced Enterprise Metrics
-- --------------------------------------------------------------------------

-- --------------------------------------------------------------------------
-- SQL SERVER - Performance Debt / Maintenance & Risk (hourly snapshot)
-- --------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS sqlserver_performance_debt_findings (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL DEFAULT 'master',
    section TEXT NOT NULL,              -- Index Health | Statistics Health | Storage & Growth | Backup & Recovery | SQL Agent | Engine Config
    finding_type TEXT NOT NULL,         -- e.g. unused_index | missing_index | index_fragmentation | stale_stats | vlf_high | backup_age | job_failed | config_risk
    severity TEXT NOT NULL,             -- CRITICAL | WARNING | INFO
    title TEXT NOT NULL,
    object_name TEXT DEFAULT '',
    object_type TEXT DEFAULT '',        -- table | index | stats | database | job | config
    finding_key TEXT NOT NULL,          -- stable identifier for grouping (e.g. db.schema.table:index)
    details JSONB NOT NULL DEFAULT '{}'::jsonb,  -- metric fields (updates, reads, frag%, vlf_count, etc.)
    recommendation TEXT DEFAULT '',
    fix_script TEXT DEFAULT '',
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_performance_debt_findings', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_perfdebt_server_time ON sqlserver_performance_debt_findings (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_perfdebt_server_section_time ON sqlserver_performance_debt_findings (server_instance_name, section, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_perfdebt_server_db_type ON sqlserver_performance_debt_findings (server_instance_name, database_name, finding_type);
CREATE INDEX IF NOT EXISTS idx_perfdebt_server_findingkey ON sqlserver_performance_debt_findings (server_instance_name, database_name, finding_key, capture_timestamp DESC);
ALTER TABLE sqlserver_performance_debt_findings SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,section,finding_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_performance_debt_findings', INTERVAL '30 days', if_not_exists => TRUE);

-- Latch Wait Statistics
CREATE TABLE IF NOT EXISTS sqlserver_latch_waits (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    wait_type TEXT NOT NULL,
    waiting_tasks_count BIGINT DEFAULT 0,
    wait_time_ms BIGINT DEFAULT 0,
    signal_wait_time_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_latch_waits', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_latch_waits_server_time ON sqlserver_latch_waits (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_latch_waits_type ON sqlserver_latch_waits (wait_type, capture_timestamp DESC);
ALTER TABLE sqlserver_latch_waits SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_latch_waits', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_latch_waits IS 'Tracks latch wait statistics for internal synchronization objects';

-- Memory Clerks Detailed
CREATE TABLE IF NOT EXISTS sqlserver_memory_clerks (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    clerk_type TEXT NOT NULL,
    memory_node SMALLINT DEFAULT 0,
    pages_mb DOUBLE PRECISION DEFAULT 0,
    virtual_memory_reserved_mb DOUBLE PRECISION DEFAULT 0,
    virtual_memory_committed_mb DOUBLE PRECISION DEFAULT 0,
    awe_memory_mb DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_memory_clerks', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_memory_clerks_server_time ON sqlserver_memory_clerks (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_memory_clerks_type ON sqlserver_memory_clerks (clerk_type, capture_timestamp DESC);
ALTER TABLE sqlserver_memory_clerks SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,clerk_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_memory_clerks', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_memory_clerks IS 'Tracks detailed memory clerk breakdown by type';

-- Waiting Tasks
CREATE TABLE IF NOT EXISTS sqlserver_waiting_tasks (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    wait_type TEXT NOT NULL,
    resource_description TEXT,
    waiting_tasks_count BIGINT DEFAULT 0,
    wait_duration_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_waiting_tasks', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_waiting_tasks_server_time ON sqlserver_waiting_tasks (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_waiting_tasks_type ON sqlserver_waiting_tasks (wait_type, capture_timestamp DESC);
ALTER TABLE sqlserver_waiting_tasks SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_waiting_tasks', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_waiting_tasks IS 'Tracks currently waiting tasks for blocking analysis';

-- Procedure Stats
CREATE TABLE IF NOT EXISTS sqlserver_procedure_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    schema_name TEXT,
    object_name TEXT,
    execution_count BIGINT DEFAULT 0,
    total_worker_time_ms DOUBLE PRECISION DEFAULT 0,
    total_elapsed_time_ms DOUBLE PRECISION DEFAULT 0,
    total_logical_reads BIGINT DEFAULT 0,
    total_physical_reads BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_procedure_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_proc_stats_server_time ON sqlserver_procedure_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_proc_stats_object ON sqlserver_procedure_stats (database_name, object_name, capture_timestamp DESC);
ALTER TABLE sqlserver_procedure_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,object_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_procedure_stats', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_procedure_stats IS 'Tracks stored procedure execution statistics';

-- Spinlock Stats
CREATE TABLE IF NOT EXISTS sqlserver_spinlock_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    spinlock_type TEXT,
    collisions BIGINT DEFAULT 0,
    spins BIGINT DEFAULT 0,
    sleep_time_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_spinlock_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_spinlock_stats_server_time ON sqlserver_spinlock_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_spinlock_stats_type ON sqlserver_spinlock_stats (spinlock_type, capture_timestamp DESC);
ALTER TABLE sqlserver_spinlock_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,spinlock_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_spinlock_stats', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_spinlock_stats IS 'Tracks spinlock contention statistics for internal synchronization';

-- --------------------------------------------------------------------------
-- SQL SERVER - Enterprise Metrics Additions (DBA-high value)
-- --------------------------------------------------------------------------

-- Plan Cache Health (single-use plans / cache pressure)
CREATE TABLE IF NOT EXISTS sqlserver_plan_cache_health (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    total_cache_mb DOUBLE PRECISION DEFAULT 0,
    single_use_cache_mb DOUBLE PRECISION DEFAULT 0,
    single_use_cache_pct DOUBLE PRECISION DEFAULT 0,
    adhoc_cache_mb DOUBLE PRECISION DEFAULT 0,
    prepared_cache_mb DOUBLE PRECISION DEFAULT 0,
    proc_cache_mb DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_plan_cache_health', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_plan_cache_health_server_time ON sqlserver_plan_cache_health (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_plan_cache_health SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_plan_cache_health', INTERVAL '14 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_plan_cache_health IS 'Tracks plan cache size and single-use plan pressure (optimize for ad hoc workloads)';

-- Memory Grant Waiters (RESOURCE_SEMAPHORE pressure)
CREATE TABLE IF NOT EXISTS sqlserver_memory_grant_waiters (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    session_id INTEGER,
    request_id INTEGER,
    database_name TEXT,
    login_name TEXT,
    requested_memory_kb BIGINT DEFAULT 0,
    granted_memory_kb BIGINT DEFAULT 0,
    required_memory_kb BIGINT DEFAULT 0,
    wait_time_ms BIGINT DEFAULT 0,
    dop INTEGER DEFAULT 1,
    query_text TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_memory_grant_waiters', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_memgrant_waiters_server_time ON sqlserver_memory_grant_waiters (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_memgrant_waiters_server_sid ON sqlserver_memory_grant_waiters (server_instance_name, session_id, capture_timestamp DESC);
ALTER TABLE sqlserver_memory_grant_waiters SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_memory_grant_waiters', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_memory_grant_waiters IS 'Tracks memory grant waiters (grant_time IS NULL) for diagnosing workspace memory pressure';

-- TempDB Top Consumers (per-session)
CREATE TABLE IF NOT EXISTS sqlserver_tempdb_top_consumers (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    session_id INTEGER,
    database_name TEXT,
    login_name TEXT,
    host_name TEXT,
    program_name TEXT,
    tempdb_mb DOUBLE PRECISION DEFAULT 0,
    user_objects_mb DOUBLE PRECISION DEFAULT 0,
    internal_objects_mb DOUBLE PRECISION DEFAULT 0,
    query_text TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_tempdb_top_consumers', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_tempdb_consumers_server_time ON sqlserver_tempdb_top_consumers (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_tempdb_consumers_server_sid ON sqlserver_tempdb_top_consumers (server_instance_name, session_id, capture_timestamp DESC);
ALTER TABLE sqlserver_tempdb_top_consumers SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_tempdb_top_consumers', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_tempdb_top_consumers IS 'Tracks top tempdb consumers by session for troubleshooting tempdb pressure and spills';

-- Tempdb Stats
CREATE TABLE IF NOT EXISTS sqlserver_tempdb_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    version_store_size_kb BIGINT DEFAULT 0,
    user_objects_alloc_kb BIGINT DEFAULT 0,
    user_objects_dealloc_kb BIGINT DEFAULT 0,
    internal_objects_alloc_kb BIGINT DEFAULT 0,
    internal_objects_dealloc_kb BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_tempdb_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_tempdb_stats_server_time ON sqlserver_tempdb_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_tempdb_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_tempdb_stats', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_tempdb_stats IS 'Tracks tempdb space usage including version store and user/internal objects';

-- TempDB File Usage (used by Enterprise Metrics dashboard)
CREATE TABLE IF NOT EXISTS sqlserver_tempdb_files (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    file_name TEXT,
    file_type TEXT,
    allocated_mb DOUBLE PRECISION DEFAULT 0,
    used_mb DOUBLE PRECISION DEFAULT 0,
    free_mb DOUBLE PRECISION DEFAULT 0,
    max_size_mb DOUBLE PRECISION DEFAULT 0,
    growth_mb DOUBLE PRECISION DEFAULT 0,
    used_percent DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_tempdb_files', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_tempdb_files_server_time ON sqlserver_tempdb_files (server_instance_name, capture_timestamp DESC);
ALTER TABLE sqlserver_tempdb_files SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,file_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_tempdb_files', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_tempdb_files IS 'Tracks tempdb file-level usage for Enterprise Metrics dashboard';

-- Database Size Growth
CREATE TABLE IF NOT EXISTS sqlserver_database_size (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    data_size_gb DOUBLE PRECISION DEFAULT 0,
    log_size_gb DOUBLE PRECISION DEFAULT 0,
    total_size_gb DOUBLE PRECISION DEFAULT 0,
    space_used_gb DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_database_size', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_db_size_server_time ON sqlserver_database_size (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_db_size_database ON sqlserver_database_size (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_database_size SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_database_size', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_database_size IS 'Tracks database size for growth trending';

-- Server Configuration
CREATE TABLE IF NOT EXISTS sqlserver_server_config (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    config_name TEXT NOT NULL,
    config_value TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_server_config', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_server_config_server_time ON sqlserver_server_config (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_server_config_name ON sqlserver_server_config (config_name, capture_timestamp DESC);
ALTER TABLE sqlserver_server_config SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,config_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_server_config', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_server_config IS 'Tracks server-level configuration settings';

-- Database Configuration
CREATE TABLE IF NOT EXISTS sqlserver_database_config (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    config_name TEXT NOT NULL,
    config_value TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_database_config', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_db_config_server_time ON sqlserver_database_config (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_db_config_database ON sqlserver_database_config (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_database_config SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,config_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_database_config', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_database_config IS 'Tracks database-level configuration settings';

-- Session Memory Grants
CREATE TABLE IF NOT EXISTS sqlserver_memory_grants (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    session_id SMALLINT NOT NULL,
    database_name TEXT,
    login_name TEXT,
    granted_memory_kb BIGINT DEFAULT 0,
    used_memory_kb BIGINT DEFAULT 0,
    dop SMALLINT DEFAULT 0,
    query_duration_sec DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_memory_grants', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_memory_grants_server_time ON sqlserver_memory_grants (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_memory_grants_session ON sqlserver_memory_grants (session_id, capture_timestamp DESC);
ALTER TABLE sqlserver_memory_grants SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,session_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_memory_grants', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_memory_grants IS 'Tracks query memory grants detail';

-- Scheduler Workload Groups
CREATE TABLE IF NOT EXISTS sqlserver_scheduler_wg (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    pool_name TEXT NOT NULL,
    group_name TEXT NOT NULL,
    active_requests BIGINT DEFAULT 0,
    queued_requests BIGINT DEFAULT 0,
    cpu_usage_percent DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_scheduler_wg', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_scheduler_wg_server_time ON sqlserver_scheduler_wg (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_scheduler_wg_group ON sqlserver_scheduler_wg (group_name, capture_timestamp DESC);
ALTER TABLE sqlserver_scheduler_wg SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,pool_name,group_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_scheduler_wg', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_scheduler_wg IS 'Tracks CPU scheduler and workload group statistics';

-- File I/O Latency
CREATE TABLE IF NOT EXISTS sqlserver_file_io_latency (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    file_name TEXT,
    file_type TEXT,
    read_latency_ms DOUBLE PRECISION DEFAULT 0,
    write_latency_ms DOUBLE PRECISION DEFAULT 0,
    read_bytes_per_sec DOUBLE PRECISION DEFAULT 0,
    write_bytes_per_sec DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_file_io_latency', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_file_io_server_time ON sqlserver_file_io_latency (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_file_io_database ON sqlserver_file_io_latency (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_file_io_latency SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,file_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_file_io_latency', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_file_io_latency IS 'Tracks file I/O latency statistics';

-- Query Store Runtime Stats
CREATE TABLE IF NOT EXISTS sqlserver_qs_runtime (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    query_id BIGINT NOT NULL,
    execution_count BIGINT DEFAULT 0,
    avg_duration_ms DOUBLE PRECISION DEFAULT 0,
    avg_cpu_ms DOUBLE PRECISION DEFAULT 0,
    avg_logical_reads DOUBLE PRECISION DEFAULT 0,
    total_cpu_ms DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_qs_runtime', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_qs_runtime_server_time ON sqlserver_qs_runtime (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_qs_runtime_query ON sqlserver_qs_runtime (database_name, query_id, capture_timestamp DESC);
ALTER TABLE sqlserver_qs_runtime SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,query_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_qs_runtime', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_qs_runtime IS 'Tracks Query Store runtime statistics';

-- --------------------------------------------------------------------------
-- 1.6: POSTGRESQL - Core Metrics
-- --------------------------------------------------------------------------

-- PostgreSQL Throughput Metrics
CREATE TABLE IF NOT EXISTS postgres_throughput_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    tps DOUBLE PRECISION DEFAULT 0,
    cache_hit_pct DOUBLE PRECISION DEFAULT 0,
    txn_delta BIGINT DEFAULT 0,
    blks_read_delta BIGINT DEFAULT 0,
    blks_hit_delta BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_throughput_metrics', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_tp_server_db ON postgres_throughput_metrics (server_instance_name, database_name, capture_timestamp DESC);
ALTER TABLE postgres_throughput_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_throughput_metrics', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Connection Statistics
CREATE TABLE IF NOT EXISTS postgres_connection_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    total_connections INTEGER DEFAULT 0,
    active_connections INTEGER DEFAULT 0,
    idle_connections INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_connection_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_conn_server ON postgres_connection_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_connection_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_connection_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Replication Statistics
CREATE TABLE IF NOT EXISTS postgres_replication_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    is_primary BOOLEAN DEFAULT false,
    cluster_state TEXT,
    max_lag_mb DOUBLE PRECISION DEFAULT 0,
    wal_gen_rate_mbps DOUBLE PRECISION DEFAULT 0,
    bgwriter_eff_pct DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_replication_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_repl_server ON postgres_replication_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_replication_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_replication_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL System Stats (CPU/Memory)
CREATE TABLE IF NOT EXISTS postgres_system_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    cpu_usage DOUBLE PRECISION DEFAULT 0,
    memory_usage DOUBLE PRECISION DEFAULT 0,
    active_connections INTEGER DEFAULT 0,
    idle_connections INTEGER DEFAULT 0,
    total_connections INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_system_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_sys_server ON postgres_system_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_system_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_system_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Database Statistics
CREATE TABLE IF NOT EXISTS postgres_database_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT,
    xact_commit BIGINT DEFAULT 0,
    xact_rollback BIGINT DEFAULT 0,
    blks_read BIGINT DEFAULT 0,
    blks_hit BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_database_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_dbstat_server ON postgres_database_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_database_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_database_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Session Statistics
CREATE TABLE IF NOT EXISTS postgres_session_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    pid INTEGER,
    username TEXT,
    database_name TEXT,
    client_addr TEXT,
    client_port INTEGER,
    backend_start TIMESTAMPTZ,
    query_start TIMESTAMPTZ,
    state_change TIMESTAMPTZ,
    wait_event_type TEXT,
    wait_event TEXT,
    state TEXT,
    query TEXT,
    duration_seconds DOUBLE PRECISION,
    blocked_by INTEGER[],
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_session_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_sess_server ON postgres_session_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_postgres_sess_state ON postgres_session_stats (state);
ALTER TABLE postgres_session_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_session_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Lock Statistics
CREATE TABLE IF NOT EXISTS postgres_lock_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    pid INTEGER,
    lock_type TEXT,
    relation TEXT,
    mode TEXT,
    granted BOOLEAN,
    waiting_seconds DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_lock_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_lock_server ON postgres_lock_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_lock_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_lock_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Table Statistics
CREATE TABLE IF NOT EXISTS postgres_table_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    schema_name TEXT,
    table_name TEXT,
    total_size TEXT,
    dead_tuples BIGINT DEFAULT 0,
    bloat_pct DOUBLE PRECISION DEFAULT 0,
    seq_scans BIGINT DEFAULT 0,
    idx_scans BIGINT DEFAULT 0,
    last_vacuum TIMESTAMPTZ,
    last_analyze TIMESTAMPTZ,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_table_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_tablestat_server ON postgres_table_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_table_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,schema_name,table_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_table_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Index Statistics
CREATE TABLE IF NOT EXISTS postgres_index_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    index_name TEXT,
    table_name TEXT,
    size TEXT,
    scans BIGINT DEFAULT 0,
    reason TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_index_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_idxstat_server ON postgres_index_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_index_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,index_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_index_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Query Statistics
CREATE TABLE IF NOT EXISTS postgres_query_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    query_id BIGINT,
    query_text TEXT,
    calls BIGINT DEFAULT 0,
    total_time_ms DOUBLE PRECISION DEFAULT 0,
    mean_time_ms DOUBLE PRECISION DEFAULT 0,
    rows BIGINT DEFAULT 0,
    temp_blks_read BIGINT DEFAULT 0,
    temp_blks_written BIGINT DEFAULT 0,
    blk_read_time_ms DOUBLE PRECISION DEFAULT 0,
    blk_write_time_ms DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_query_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_qrystat_server ON postgres_query_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_query_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,query_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_query_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Configuration Settings
CREATE TABLE IF NOT EXISTS postgres_config_settings (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    config_name TEXT,
    config_value TEXT,
    config_unit TEXT,
    config_category TEXT,
    config_source TEXT,
    boot_val TEXT,
    reset_val TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_config_settings', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_cfg_server ON postgres_config_settings (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_config_settings SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,config_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_config_settings', INTERVAL '30 days', if_not_exists => TRUE);

-- PostgreSQL Long Running Queries
CREATE TABLE IF NOT EXISTS postgres_long_running_queries (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    pid INTEGER,
    username TEXT,
    database_name TEXT,
    client_addr TEXT,
    client_port INTEGER,
    backend_start TIMESTAMPTZ,
    query_start TIMESTAMPTZ,
    state_change TIMESTAMPTZ,
    wait_event_type TEXT,
    wait_event TEXT,
    state TEXT,
    query TEXT,
    duration_seconds DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_long_running_queries', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_longrun_server ON postgres_long_running_queries (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_long_running_queries SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_long_running_queries', INTERVAL '7 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 1.7: POSTGRESQL - Enterprise Metrics (BGWriter, Archiver, Query Dictionary)
-- --------------------------------------------------------------------------

-- PostgreSQL Background Writer & Checkpointer Stats
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
SELECT create_hypertable('postgres_bgwriter_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_bgw_server_time ON postgres_bgwriter_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_bgwriter_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_bgwriter_stats', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_bgwriter_stats IS 'Tracks PostgreSQL checkpointer and background writer activity';

-- PostgreSQL WAL Archiver Stats
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
SELECT create_hypertable('postgres_archiver_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_arch_server_time ON postgres_archiver_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_postgres_arch_failed ON postgres_archiver_stats (server_instance_name, failed_count DESC) WHERE failed_count > 0;
ALTER TABLE postgres_archiver_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_archiver_stats', INTERVAL '7 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_archiver_stats IS 'Tracks WAL archiving status - failed_count > 0 indicates archiving problems';

-- PostgreSQL Query Dictionary (with PRIMARY KEY for ON CONFLICT upserts)
CREATE TABLE IF NOT EXISTS postgres_query_dictionary (
    server_instance_name TEXT NOT NULL,
    query_id BIGINT NOT NULL,
    query_text TEXT,
    first_seen TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL,
    execution_count BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (server_instance_name, query_id)
);
CREATE INDEX IF NOT EXISTS idx_postgres_query_dict_text ON postgres_query_dictionary USING gin(to_tsvector('english', query_text));
CREATE INDEX IF NOT EXISTS idx_postgres_query_dict_recent ON postgres_query_dictionary (last_seen DESC);
COMMENT ON TABLE postgres_query_dictionary IS 'Normalizes query text storage - maps query_id to actual SQL text';

-- --------------------------------------------------------------------------
-- 1.8: POSTGRESQL - Control Center (DBA-first derived metrics)
-- --------------------------------------------------------------------------
-- A compact snapshot table that powers the PostgreSQL Control Center strips.
-- Writes are delta/deduped by the collector to avoid storing identical snapshots.
CREATE TABLE IF NOT EXISTS postgres_control_center_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    -- Safety & durability
    wal_rate_mb_per_min DOUBLE PRECISION DEFAULT 0,
    wal_size_mb DOUBLE PRECISION DEFAULT 0,
    max_replication_lag_mb DOUBLE PRECISION DEFAULT 0,
    max_replication_lag_seconds DOUBLE PRECISION DEFAULT 0,
    checkpoint_req_ratio DOUBLE PRECISION DEFAULT 0,
    xid_age BIGINT DEFAULT 0,
    xid_wraparound_pct DOUBLE PRECISION DEFAULT 0,
    -- Workload
    tps DOUBLE PRECISION DEFAULT 0,
    active_sessions INTEGER DEFAULT 0,
    waiting_sessions INTEGER DEFAULT 0,
    slow_queries_count INTEGER DEFAULT 0,
    blocking_sessions INTEGER DEFAULT 0,
    autovacuum_workers INTEGER DEFAULT 0,
    dead_tuple_ratio_pct DOUBLE PRECISION DEFAULT 0,
    health_score INTEGER DEFAULT 0,
    health_status TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_control_center_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_cc_server_time ON postgres_control_center_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_control_center_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_control_center_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_control_center_stats IS 'PostgreSQL Control Center derived metrics (WAL/replication/checkpoints/xid/workload).';

ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS blocking_sessions INTEGER DEFAULT 0;
ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS autovacuum_workers INTEGER DEFAULT 0;
ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS dead_tuple_ratio_pct DOUBLE PRECISION DEFAULT 0;
ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS health_score INTEGER DEFAULT 0;
ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS health_status TEXT;

-- Per-replica lag time series for Control Center replication chart.
CREATE TABLE IF NOT EXISTS postgres_replication_lag_detail (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    replica_name TEXT NOT NULL,
    lag_mb DOUBLE PRECISION DEFAULT 0,
    state TEXT,
    sync_state TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_replication_lag_detail', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_repl_lag_detail ON postgres_replication_lag_detail (server_instance_name, replica_name, capture_timestamp DESC);
ALTER TABLE postgres_replication_lag_detail SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,replica_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_replication_lag_detail', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_replication_lag_detail IS 'Per-replica lag (MB) for Control Center charts.';

-- Replication slots risk: retained WAL can fill disks if consumers lag/disconnect.
CREATE TABLE IF NOT EXISTS postgres_replication_slot_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    slot_name TEXT NOT NULL,
    slot_type TEXT,
    active BOOLEAN DEFAULT false,
    temporary BOOLEAN DEFAULT false,
    retained_wal_mb DOUBLE PRECISION DEFAULT 0,
    restart_lsn TEXT,
    confirmed_flush_lsn TEXT,
    xmin_txid BIGINT,
    catalog_xmin_txid BIGINT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_replication_slot_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_repl_slot_server_time ON postgres_replication_slot_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_repl_slot_server_slot_time ON postgres_replication_slot_stats (server_instance_name, slot_name, capture_timestamp DESC);
ALTER TABLE postgres_replication_slot_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,slot_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_replication_slot_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_replication_slot_stats IS 'Replication slot retention and activity for WAL/slot disk risk.';

-- Local disk (filesystem) free space snapshots for PostgreSQL nodes (when configured).
CREATE TABLE IF NOT EXISTS postgres_disk_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    mount_name TEXT NOT NULL,
    path TEXT NOT NULL,
    total_bytes BIGINT DEFAULT 0,
    free_bytes BIGINT DEFAULT 0,
    avail_bytes BIGINT DEFAULT 0,
    used_pct DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_disk_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_disk_server_time ON postgres_disk_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_disk_server_mount_time ON postgres_disk_stats (server_instance_name, mount_name, capture_timestamp DESC);
ALTER TABLE postgres_disk_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,mount_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_disk_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_disk_stats IS 'Filesystem free space snapshots for PGDATA/WAL mounts (local-only when configured).';

-- Backup run events reported by external backup jobs (pgBackRest/Barman/pg_dump/etc.).
-- This is a webhook-style ingestion point: your backup job POSTs results to the API.
CREATE TABLE IF NOT EXISTS postgres_backup_runs (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    tool TEXT NOT NULL,                 -- e.g. pgbackrest | barman | pg_dump | custom
    backup_type TEXT NOT NULL,          -- e.g. full | incr | diff | logical | physical
    status TEXT NOT NULL,               -- success | failed | warning
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration_seconds BIGINT DEFAULT 0,
    wal_archived_until TIMESTAMPTZ,     -- optional: last WAL archived timestamp (RPO signal)
    repo TEXT,                          -- optional: repo name/path
    size_bytes BIGINT DEFAULT 0,
    error_message TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_backup_runs', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_backup_runs_server_time ON postgres_backup_runs (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_backup_runs_server_status_time ON postgres_backup_runs (server_instance_name, status, capture_timestamp DESC);
ALTER TABLE postgres_backup_runs SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,tool,backup_type,status',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_backup_runs', INTERVAL '60 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_backup_runs IS 'Backup run results (reported by external tools) for DBA daily checks and RPO posture.';

-- PostgreSQL log events reported by external shippers/agents (webhook-style ingestion).
-- The monitoring server does NOT read remote log files directly; instead, an agent posts parsed events.
CREATE TABLE IF NOT EXISTS postgres_log_events (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    severity TEXT NOT NULL,             -- debug|info|notice|warning|error|fatal|panic
    sqlstate TEXT,
    message TEXT NOT NULL,
    user_name TEXT,
    database_name TEXT,
    application_name TEXT,
    client_addr TEXT,
    pid BIGINT,
    context TEXT,
    detail TEXT,
    hint TEXT,
    raw JSONB DEFAULT '{}'::jsonb,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_log_events', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_log_events_server_time ON postgres_log_events (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_log_events_server_sev_time ON postgres_log_events (server_instance_name, severity, capture_timestamp DESC);
ALTER TABLE postgres_log_events SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,severity,sqlstate',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_log_events', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_log_events IS 'PostgreSQL log events (FATAL/PANIC/ERROR/auth failures/OOM) reported by external agents.';

-- Vacuum progress snapshots (pg_stat_progress_vacuum). Useful for "is vacuum running" and "which table is stuck".
CREATE TABLE IF NOT EXISTS postgres_vacuum_progress (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    pid BIGINT,
    database_name TEXT,
    user_name TEXT,
    relation_name TEXT,
    phase TEXT,
    heap_blks_total BIGINT DEFAULT 0,
    heap_blks_scanned BIGINT DEFAULT 0,
    heap_blks_vacuumed BIGINT DEFAULT 0,
    index_vacuum_count BIGINT DEFAULT 0,
    max_dead_tuples BIGINT DEFAULT 0,
    num_dead_tuples BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_vacuum_progress', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_vac_prog_server_time ON postgres_vacuum_progress (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_vac_prog_server_pid_time ON postgres_vacuum_progress (server_instance_name, pid, capture_timestamp DESC);
ALTER TABLE postgres_vacuum_progress SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,pid,relation_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_vacuum_progress', INTERVAL '14 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_vacuum_progress IS 'Vacuum progress snapshots from pg_stat_progress_vacuum.';

-- Table-level maintenance stats (dead/live tuples, vacuum/analyze timestamps) for MVCC/autovacuum health.
CREATE TABLE IF NOT EXISTS postgres_table_maintenance_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    total_bytes BIGINT DEFAULT 0,
    live_tuples BIGINT DEFAULT 0,
    dead_tuples BIGINT DEFAULT 0,
    dead_pct DOUBLE PRECISION DEFAULT 0,
    seq_scans BIGINT DEFAULT 0,
    idx_scans BIGINT DEFAULT 0,
    last_vacuum TIMESTAMPTZ,
    last_autovacuum TIMESTAMPTZ,
    last_analyze TIMESTAMPTZ,
    last_autoanalyze TIMESTAMPTZ,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_table_maintenance_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_tblmaint_server_time ON postgres_table_maintenance_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_tblmaint_server_table_time ON postgres_table_maintenance_stats (server_instance_name, schema_name, table_name, capture_timestamp DESC);
ALTER TABLE postgres_table_maintenance_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,schema_name,table_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_table_maintenance_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_table_maintenance_stats IS 'Table-level maintenance stats for vacuum/analyze/bloat monitoring.';

-- Session state time-series (for Sessions & Activity trends).
CREATE TABLE IF NOT EXISTS postgres_session_state_counts (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    active_count INTEGER DEFAULT 0,
    idle_count INTEGER DEFAULT 0,
    idle_in_txn_count INTEGER DEFAULT 0,
    waiting_count INTEGER DEFAULT 0,
    total_count INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_session_state_counts', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_sess_state_server_time ON postgres_session_state_counts (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_session_state_counts SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_session_state_counts', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_session_state_counts IS 'Aggregated session state counts (active/idle/idle-in-txn/waiting) for trend charts.';

-- PgBouncer (pooler) health snapshots. Only collected if configured.
CREATE TABLE IF NOT EXISTS postgres_pooler_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    pooler_type TEXT DEFAULT 'pgbouncer',
    cl_active INTEGER DEFAULT 0,
    cl_waiting INTEGER DEFAULT 0,
    sv_active INTEGER DEFAULT 0,
    sv_idle INTEGER DEFAULT 0,
    sv_used INTEGER DEFAULT 0,
    maxwait_seconds DOUBLE PRECISION DEFAULT 0,
    total_pools INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_pooler_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_pooler_server_time ON postgres_pooler_stats (server_instance_name, capture_timestamp DESC);
ALTER TABLE postgres_pooler_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_pooler_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_pooler_stats IS 'PgBouncer pool totals (clients/servers/waiting/maxwait) for pooler monitoring.';

-- Deadlocks counter deltas (from pg_stat_database.deadlocks) for history charts.
CREATE TABLE IF NOT EXISTS postgres_deadlock_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    deadlocks_total BIGINT DEFAULT 0,
    deadlocks_delta BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_deadlock_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_deadlocks_server_time ON postgres_deadlock_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_deadlocks_server_db_time ON postgres_deadlock_stats (server_instance_name, database_name, capture_timestamp DESC);
ALTER TABLE postgres_deadlock_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_deadlock_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_deadlock_stats IS 'Deadlocks total and delta per database for troubleshooting lock contention.';

-- ============================================================================
-- SECTION 2: APPLICATION TABLES (Dashboards, Alerts, Users)
-- ============================================================================

-- --------------------------------------------------------------------------
-- 2.1: User Management
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS optima_users (
    user_id       SERIAL PRIMARY KEY,
    username      VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(50)  NOT NULL DEFAULT 'viewer',
    created_at    TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_optima_users_username ON optima_users (username);

-- --------------------------------------------------------------------------
-- 2.2: Widget Registry
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS optima_ui_widgets (
    widget_id        VARCHAR(100) PRIMARY KEY,
    dashboard_section VARCHAR(100) NOT NULL,
    title            VARCHAR(200) NOT NULL,
    chart_type       VARCHAR(50)  NOT NULL,
    current_sql      TEXT         NOT NULL,
    default_sql      TEXT         NOT NULL,
    updated_at       TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_optima_widgets_section ON optima_ui_widgets (dashboard_section);

-- --------------------------------------------------------------------------
-- 2.3: Custom Dashboards
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_dashboards (
    id SERIAL,
    user_id INTEGER NOT NULL,
    dashboard_name TEXT NOT NULL,
    dashboard_type TEXT NOT NULL DEFAULT 'custom',
    layout_config JSONB NOT NULL DEFAULT '{}',
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
);

DO $$
BEGIN
    ALTER TABLE user_dashboards DROP CONSTRAINT IF EXISTS user_dashboards_pkey;
    ALTER TABLE user_dashboards ADD CONSTRAINT user_dashboards_pkey PRIMARY KEY (id, created_at);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Primary key may already be correct: %', SQLERRM;
END $$;

DO $$
BEGIN
    ALTER TABLE user_dashboards ADD CONSTRAINT user_dashboards_id_unique UNIQUE (id);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Unique constraint may already exist: %', SQLERRM;
END $$;

CREATE INDEX IF NOT EXISTS idx_user_dashboards_user ON user_dashboards (user_id);
CREATE INDEX IF NOT EXISTS idx_user_dashboards_name ON user_dashboards (dashboard_name);

-- --------------------------------------------------------------------------
-- 2.4: Dashboard Widgets Configuration
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS dashboard_widgets (
    id SERIAL PRIMARY KEY,
    dashboard_id INTEGER REFERENCES user_dashboards(id) ON DELETE CASCADE,
    widget_type TEXT NOT NULL,
    widget_title TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    chart_type TEXT DEFAULT 'line',
    position_x INTEGER DEFAULT 0,
    position_y INTEGER DEFAULT 0,
    width INTEGER DEFAULT 4,
    height INTEGER DEFAULT 3,
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dashboard_widgets_dashboard ON dashboard_widgets (dashboard_id);

-- --------------------------------------------------------------------------
-- 2.5: Alert System
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_thresholds (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    metric_name TEXT NOT NULL,
    threshold_name TEXT NOT NULL,
    threshold_type TEXT NOT NULL CHECK (threshold_type IN ('cpu', 'memory', 'disk', 'connections', 'tps', 'wait', 'custom')),
    condition_type TEXT NOT NULL CHECK (condition_type IN ('above', 'below', 'equals', 'between')),
    warning_threshold FLOAT NOT NULL,
    critical_threshold FLOAT,
    evaluation_interval TEXT DEFAULT '5m',
    evaluation_window TEXT DEFAULT '5m',
    is_enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_alert_thresholds_user ON alert_thresholds (user_id);
CREATE INDEX IF NOT EXISTS idx_alert_thresholds_metric ON alert_thresholds (metric_name);
CREATE INDEX IF NOT EXISTS idx_alert_thresholds_enabled ON alert_thresholds (is_enabled) WHERE is_enabled = TRUE;

-- --------------------------------------------------------------------------
-- 2.6: Notification Channels
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_channels (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    channel_name TEXT NOT NULL,
    channel_type TEXT NOT NULL CHECK (channel_type IN ('email', 'slack', 'webhook', 'pagerduty')),
    config JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notification_channels_user ON notification_channels (user_id);

-- --------------------------------------------------------------------------
-- 2.7: Alert Subscriptions
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_subscriptions (
    id SERIAL PRIMARY KEY,
    threshold_id INTEGER REFERENCES alert_thresholds(id) ON DELETE CASCADE,
    channel_id INTEGER REFERENCES notification_channels(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(threshold_id, channel_id)
);

-- --------------------------------------------------------------------------
-- 2.8: Alert History
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_history (
    id SERIAL,
    threshold_id INTEGER REFERENCES alert_thresholds(id),
    instance_name TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value FLOAT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('warning', 'critical')),
    message TEXT,
    acknowledged BOOLEAN DEFAULT FALSE,
    acknowledged_by INTEGER,
    acknowledged_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
);

DO $$
BEGIN
    ALTER TABLE alert_history DROP CONSTRAINT IF EXISTS alert_history_pkey;
    ALTER TABLE alert_history ADD CONSTRAINT alert_history_pkey PRIMARY KEY (id, created_at);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Primary key may already be correct: %', SQLERRM;
END $$;

SELECT create_hypertable('alert_history', 'created_at', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_alert_history_instance ON alert_history (instance_name);
CREATE INDEX IF NOT EXISTS idx_alert_history_created ON alert_history (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_history_threshold ON alert_history (threshold_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_acknowledged ON alert_history (acknowledged) WHERE acknowledged = FALSE;
DO $$
BEGIN
    ALTER TABLE alert_history SET (
        timescaledb.compress = true,
        timescaledb.compress_orderby = 'created_at DESC',
        timescaledb.compress_segmentby = 'instance_name, metric_name'
    );
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Compression settings may already be applied: %', SQLERRM;
END $$;

DO $$
BEGIN
    SELECT add_compression_policy('alert_history', INTERVAL '30 days', if_not_exists => TRUE);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Compression policy may already exist: %', SQLERRM;
END $$;

-- --------------------------------------------------------------------------
-- 2.9: Monitored Servers
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS monitored_servers (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    server_name TEXT NOT NULL,
    server_type TEXT NOT NULL CHECK (server_type IN ('sqlserver', 'postgres')),
    host TEXT NOT NULL,
    port INTEGER DEFAULT 1433,
    database_name TEXT,
    connection_string_encrypted TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    collection_enabled BOOLEAN DEFAULT TRUE,
    collection_interval TEXT DEFAULT '15s',
    tags JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, server_name)
);
CREATE INDEX IF NOT EXISTS idx_monitored_servers_user ON monitored_servers (user_id);
CREATE INDEX IF NOT EXISTS idx_monitored_servers_active ON monitored_servers (is_active) WHERE is_active = TRUE;

-- --------------------------------------------------------------------------
-- 2.10: Custom Metric Collection Settings
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS metric_collection_settings (
    id SERIAL PRIMARY KEY,
    server_id INTEGER REFERENCES monitored_servers(id) ON DELETE CASCADE,
    metric_category TEXT NOT NULL,
    is_enabled BOOLEAN DEFAULT TRUE,
    collection_interval TEXT DEFAULT '30s',
    retention_period TEXT DEFAULT '7 days',
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_metric_collection_server ON metric_collection_settings (server_id);

-- --------------------------------------------------------------------------
-- 2.11: Dashboard Exports
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS dashboard_exports (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    export_name TEXT NOT NULL,
    export_type TEXT NOT NULL CHECK (export_type IN ('dashboard', 'alerts', 'servers', 'full')),
    export_data JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dashboard_exports_user ON dashboard_exports (user_id);

-- ============================================================================
-- SECTION 3: COLLECTION MANAGEMENT TABLES
-- ============================================================================

-- --------------------------------------------------------------------------
-- 3.1: Collection Schedule
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sqlserver_collection_schedule (
    schedule_id SERIAL PRIMARY KEY,
    collector_name TEXT NOT NULL UNIQUE,
    enabled BOOLEAN DEFAULT TRUE,
    frequency_minutes INTEGER DEFAULT 15,
    last_run_time TIMESTAMPTZ,
    next_run_time TIMESTAMPTZ,
    max_duration_minutes INTEGER DEFAULT 5,
    retention_days INTEGER DEFAULT 30,
    description TEXT,
    created_date TIMESTAMPTZ DEFAULT NOW(),
    modified_date TIMESTAMPTZ DEFAULT NOW()
);

-- --------------------------------------------------------------------------
-- 3.2: Collection Log
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sqlserver_collection_log (
    log_id BIGSERIAL PRIMARY KEY,
    collection_time TIMESTAMPTZ DEFAULT NOW(),
    collector_name TEXT NOT NULL,
    collection_status TEXT NOT NULL,
    rows_collected BIGINT DEFAULT 0,
    duration_ms BIGINT DEFAULT 0,
    error_message TEXT
);
CREATE INDEX IF NOT EXISTS idx_collection_log_time ON sqlserver_collection_log (collection_time DESC);
CREATE INDEX IF NOT EXISTS idx_collection_log_collector ON sqlserver_collection_log (collector_name, collection_time DESC);

-- ============================================================================
-- SECTION 4: GRANT PERMISSIONS
-- ============================================================================
GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA public TO PUBLIC;

DO $$
DECLARE
    ht_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO ht_count FROM timescaledb_information.hypertables;
    RAISE NOTICE '============================================';
    RAISE NOTICE 'SQL Optima Unified Schema created successfully!';
    RAISE NOTICE 'Total hypertable count: %', ht_count;
    RAISE NOTICE '============================================';
END $$;

-- ============================================================================
-- MIGRATION: Add error_message column to existing sqlserver_job_metrics table
-- Run this only if you already have the schema and need to add the column
-- ============================================================================
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'sqlserver_job_metrics' 
        AND column_name = 'error_message'
    ) THEN
        ALTER TABLE sqlserver_job_metrics ADD COLUMN error_message TEXT;
        RAISE NOTICE 'Migration: Added error_message column to sqlserver_job_metrics';
    ELSE
        RAISE NOTICE 'Migration: error_message column already exists in sqlserver_job_metrics';
    END IF;
END $$;

COMMENT ON COLUMN sqlserver_job_metrics.error_message IS 'Stores error message if job collection failed (e.g., permission denied on msdb tables)';
