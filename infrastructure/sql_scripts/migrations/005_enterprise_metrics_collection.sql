-- SQL Optima: Enterprise Metrics Collection Tables
-- Run this migration against TimescaleDB to add new metrics tables
-- Based on Performance Monitor collector patterns

-- ============================================
-- Latch Wait Statistics
-- ============================================
CREATE TABLE IF NOT EXISTS sqlserver_latch_waits (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    wait_type TEXT NOT NULL,
    waiting_tasks_count BIGINT DEFAULT 0,
    wait_time_ms BIGINT DEFAULT 0,
    signal_wait_time_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_latch_waits', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_latch_waits_server_time 
    ON sqlserver_latch_waits (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_latch_waits_type 
    ON sqlserver_latch_waits (wait_type, capture_timestamp DESC);

ALTER TABLE sqlserver_latch_waits SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_latch_waits', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_latch_waits IS 'Tracks latch wait statistics for internal synchronization objects';

-- ============================================
-- Memory Clerks Detailed
-- ============================================
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

SELECT create_hypertable('sqlserver_memory_clerks', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_memory_clerks_server_time 
    ON sqlserver_memory_clerks (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_memory_clerks_type 
    ON sqlserver_memory_clerks (clerk_type, capture_timestamp DESC);

ALTER TABLE sqlserver_memory_clerks SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,clerk_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_memory_clerks', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_memory_clerks IS 'Tracks detailed memory clerk breakdown by type';

-- ============================================
-- Waiting Tasks
-- ============================================
CREATE TABLE IF NOT EXISTS sqlserver_waiting_tasks (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    wait_type TEXT NOT NULL,
    resource_description TEXT,
    waiting_tasks_count BIGINT DEFAULT 0,
    wait_duration_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_waiting_tasks', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_waiting_tasks_server_time 
    ON sqlserver_waiting_tasks (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_waiting_tasks_type 
    ON sqlserver_waiting_tasks (wait_type, capture_timestamp DESC);

ALTER TABLE sqlserver_waiting_tasks SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_waiting_tasks', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_waiting_tasks IS 'Tracks currently waiting tasks for blocking analysis';

-- ============================================
-- Procedure Stats
-- ============================================
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

SELECT create_hypertable('sqlserver_procedure_stats', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_proc_stats_server_time 
    ON sqlserver_procedure_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_proc_stats_object 
    ON sqlserver_procedure_stats (database_name, object_name, capture_timestamp DESC);

ALTER TABLE sqlserver_procedure_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,object_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_procedure_stats', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_procedure_stats IS 'Tracks stored procedure execution statistics';

-- ============================================
-- Database Size Growth (Daily)
-- ============================================
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

SELECT create_hypertable('sqlserver_database_size', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_db_size_server_time 
    ON sqlserver_database_size (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_db_size_database 
    ON sqlserver_database_size (database_name, capture_timestamp DESC);

ALTER TABLE sqlserver_database_size SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_database_size', INTERVAL '30 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_database_size IS 'Tracks database size for growth trending';

-- ============================================
-- Server Configuration
-- ============================================
CREATE TABLE IF NOT EXISTS sqlserver_server_config (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    config_name TEXT NOT NULL,
    config_value TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_server_config', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_server_config_server_time 
    ON sqlserver_server_config (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_server_config_name 
    ON sqlserver_server_config (config_name, capture_timestamp DESC);

ALTER TABLE sqlserver_server_config SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,config_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_server_config', INTERVAL '30 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_server_config IS 'Tracks server-level configuration settings';

-- ============================================
-- Database Configuration
-- ============================================
CREATE TABLE IF NOT EXISTS sqlserver_database_config (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    config_name TEXT NOT NULL,
    config_value TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_database_config', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_db_config_server_time 
    ON sqlserver_database_config (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_db_config_database 
    ON sqlserver_database_config (database_name, capture_timestamp DESC);

ALTER TABLE sqlserver_database_config SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,config_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_database_config', INTERVAL '30 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_database_config IS 'Tracks database-level configuration settings';

-- ============================================
-- Session Memory Grants
-- ============================================
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

SELECT create_hypertable('sqlserver_memory_grants', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_memory_grants_server_time 
    ON sqlserver_memory_grants (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_memory_grants_session 
    ON sqlserver_memory_grants (session_id, capture_timestamp DESC);

ALTER TABLE sqlserver_memory_grants SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,session_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_memory_grants', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_memory_grants IS 'Tracks query memory grants detail';

-- ============================================
-- Scheduler Workload Groups
-- ============================================
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

SELECT create_hypertable('sqlserver_scheduler_wg', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_scheduler_wg_server_time 
    ON sqlserver_scheduler_wg (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_scheduler_wg_group 
    ON sqlserver_scheduler_wg (group_name, capture_timestamp DESC);

ALTER TABLE sqlserver_scheduler_wg SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,pool_name,group_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_scheduler_wg', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_scheduler_wg IS 'Tracks CPU scheduler and workload group statistics';

-- ============================================
-- File I/O Latency
-- ============================================
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

SELECT create_hypertable('sqlserver_file_io_latency', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_file_io_server_time 
    ON sqlserver_file_io_latency (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_file_io_database 
    ON sqlserver_file_io_latency (database_name, capture_timestamp DESC);

ALTER TABLE sqlserver_file_io_latency SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,file_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_file_io_latency', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_file_io_latency IS 'Tracks file I/O latency statistics';

-- ============================================
-- Query Store Runtime Stats
-- ============================================
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

SELECT create_hypertable('sqlserver_qs_runtime', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_qs_runtime_server_time 
    ON sqlserver_qs_runtime (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_qs_runtime_query 
    ON sqlserver_qs_runtime (database_name, query_id, capture_timestamp DESC);

ALTER TABLE sqlserver_qs_runtime SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,query_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_qs_runtime', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_qs_runtime IS 'Tracks Query Store runtime statistics';

-- ============================================
-- Collection Schedule (for tracking collector frequency)
-- ============================================
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

-- Initialize default collection schedule
INSERT INTO sqlserver_collection_schedule (collector_name, enabled, frequency_minutes, max_duration_minutes, retention_days, description)
SELECT * FROM (VALUES
    ('wait_stats', TRUE, 1, 2, 30, 'Wait statistics - high frequency for trending'),
    ('query_stats', TRUE, 2, 5, 30, 'Plan cache queries - recent activity focused'),
    ('memory_stats', TRUE, 1, 2, 30, 'Memory pressure monitoring'),
    ('blocking', TRUE, 1, 2, 30, 'Fast blocked process collection'),
    ('deadlocks', TRUE, 1, 3, 30, 'Fast deadlock XML collection'),
    ('cpu_utilization', TRUE, 1, 2, 30, 'CPU utilization from ring buffer'),
    ('perfmon_stats', TRUE, 5, 2, 30, 'Performance counter statistics'),
    ('file_io_stats', TRUE, 1, 2, 30, 'File I/O statistics'),
    ('memory_grant_stats', TRUE, 1, 2, 30, 'Memory grant semaphore pressure'),
    ('cpu_scheduler_stats', TRUE, 1, 2, 30, 'CPU scheduler statistics'),
    ('latch_stats', TRUE, 1, 3, 30, 'Latch contention statistics'),
    ('spinlock_stats', TRUE, 1, 3, 30, 'Spinlock contention statistics'),
    ('tempdb_stats', TRUE, 1, 2, 30, 'TempDB space usage'),
    ('session_stats', TRUE, 1, 2, 30, 'Session and connection statistics'),
    ('waiting_tasks', TRUE, 1, 2, 30, 'Currently waiting tasks'),
    ('running_jobs', TRUE, 1, 2, 7, 'SQL Agent jobs'),
    ('query_store', TRUE, 15, 10, 30, 'Query Store data'),
    ('procedure_stats', TRUE, 2, 10, 30, 'Procedure statistics'),
    ('memory_clerks_stats', TRUE, 5, 3, 30, 'Memory clerk allocation'),
    ('plan_cache_stats', TRUE, 5, 5, 30, 'Plan cache composition'),
    ('query_snapshots', TRUE, 1, 2, 10, 'Currently executing queries'),
    ('server_configuration', TRUE, 1440, 5, 30, 'Server config (daily)'),
    ('database_configuration', TRUE, 1440, 10, 30, 'Database config (daily)'),
    ('server_properties', TRUE, 1440, 5, 365, 'Server properties (daily)'),
    ('database_size_stats', TRUE, 60, 10, 90, 'Database size (hourly)')
) AS v(collector_name, enabled, frequency_minutes, max_duration_minutes, retention_days, description)
ON CONFLICT (collector_name) DO NOTHING;

-- ============================================
-- Collection Log
-- ============================================
CREATE TABLE IF NOT EXISTS sqlserver_collection_log (
    log_id BIGSERIAL PRIMARY KEY,
    collection_time TIMESTAMPTZ DEFAULT NOW(),
    collector_name TEXT NOT NULL,
    collection_status TEXT NOT NULL,
    rows_collected BIGINT DEFAULT 0,
    duration_ms BIGINT DEFAULT 0,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_collection_log_time 
    ON sqlserver_collection_log (collection_time DESC);
CREATE INDEX IF NOT EXISTS idx_collection_log_collector 
    ON sqlserver_collection_log (collector_name, collection_time DESC);

COMMENT ON TABLE sqlserver_collection_log IS 'Tracks all collection runs with status, timing, and error information';

-- ============================================
-- Materialized Views for Fast Aggregation
-- ============================================

-- Latch Summary (last hour)
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_latch_summary AS
SELECT 
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_instance_name,
    wait_type,
    AVG(wait_time_ms) AS avg_wait_time_ms,
    MAX(wait_time_ms) AS max_wait_time_ms,
    SUM(waiting_tasks_count) AS total_waiting_tasks
FROM sqlserver_latch_waits
WHERE capture_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY 1, 2, 3;

-- Memory Clerks Summary
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_memory_clerks_summary AS
SELECT 
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    server_instance_name,
    clerk_type,
    AVG(pages_mb) AS avg_pages_mb,
    MAX(pages_mb) AS max_pages_mb
FROM sqlserver_memory_clerks
WHERE capture_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY 1, 2, 3;

-- File I/O Summary
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_file_io_summary AS
SELECT 
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_instance_name,
    database_name,
    AVG(read_latency_ms) AS avg_read_latency_ms,
    AVG(write_latency_ms) AS avg_write_latency_ms,
    MAX(read_latency_ms) AS max_read_latency_ms,
    MAX(write_latency_ms) AS max_write_latency_ms
FROM sqlserver_file_io_latency
WHERE capture_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY 1, 2, 3;

-- Database Size Growth Trend
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_db_growth_summary AS
SELECT 
    time_bucket('1 day', capture_timestamp) AS bucket,
    server_instance_name,
    database_name,
    MAX(total_size_gb) AS total_size_gb,
    MAX(space_used_gb) AS space_used_gb
FROM sqlserver_database_size
WHERE capture_timestamp >= NOW() - INTERVAL '30 days'
GROUP BY 1, 2, 3;

PRINT 'SQL Optima Enterprise Metrics Migration completed successfully';
