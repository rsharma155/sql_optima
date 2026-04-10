-- SQL Optima: Enterprise SQL Server Monitoring Tables
-- Run this migration against TimescaleDB

-- ============================================
-- SQL Server Availability Group Health
-- ============================================
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

SELECT create_hypertable('sqlserver_ag_health', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_ag_health_server_time 
    ON sqlserver_ag_health (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ag_health_ag_name 
    ON sqlserver_ag_health (ag_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ag_health_db 
    ON sqlserver_ag_health (database_name, capture_timestamp DESC);

-- Compression for older data (TimescaleDB 2.x syntax)
ALTER TABLE sqlserver_ag_health SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,ag_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_ag_health', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_ag_health IS 'Tracks AlwaysOn Availability Group health metrics including sync state and queue sizes';

-- ============================================
-- SQL Server Database Throughput (TPS)
-- ============================================
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

SELECT create_hypertable('sqlserver_database_throughput', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_db_throughput_server_time 
    ON sqlserver_database_throughput (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_db_throughput_db 
    ON sqlserver_database_throughput (database_name, capture_timestamp DESC);

-- Compression for older data (TimescaleDB 2.x syntax)
ALTER TABLE sqlserver_database_throughput SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_database_throughput', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_database_throughput IS 'Tracks database-level throughput metrics including TPS, batch requests, and I/O statistics';

-- ============================================
-- SQL Server Query Store Stats (Historical)
-- ============================================
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

SELECT create_hypertable('sqlserver_query_store_stats', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_qs_stats_server_time 
    ON sqlserver_query_store_stats (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_qs_stats_query_hash 
    ON sqlserver_query_store_stats (query_hash);
CREATE INDEX IF NOT EXISTS idx_qs_stats_database 
    ON sqlserver_query_store_stats (database_name, capture_timestamp DESC);

-- Compression for older data (TimescaleDB 2.x syntax)
ALTER TABLE sqlserver_query_store_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name,query_hash',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_query_store_stats', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_query_store_stats IS 'Stores historical Query Store statistics for bottleneck analysis';

-- ============================================
-- SQL Server Top Queries (Live Significant Queries)
-- ============================================
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

SELECT create_hypertable('sqlserver_top_queries', 'capture_timestamp', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_top_queries_server_time 
    ON sqlserver_top_queries (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_top_queries_query_text 
    ON sqlserver_top_queries USING gin (to_tsvector('english', query_text));

ALTER TABLE sqlserver_top_queries SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_top_queries', INTERVAL '7 days', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_top_queries IS 'Tracks significant live queries (>5s elapsed, >50ms CPU, >5000 logical reads)';

-- ============================================
-- Materialized Views for Fast Aggregation
-- Note: Continuous aggregates need to be created separately
-- Run these commands individually or in a separate transaction
-- ============================================

-- AG Health Summary (last 24 hours per AG)
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_ag_health_summary AS
SELECT 
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    server_instance_name,
    ag_name,
    replica_server_name,
    database_name,
    AVG(CASE WHEN synchronization_state = 'SYNCHRONIZING' THEN 1 ELSE 0 END) AS sync_pct,
    AVG(CASE WHEN synchronization_state = 'SYNCHRONIZED' THEN 1 ELSE 0 END) AS healthy_pct,
    AVG(log_send_queue_kb) AS avg_log_send_queue_kb,
    AVG(redo_queue_kb) AS avg_redo_queue_kb,
    MAX(log_send_queue_kb) AS max_log_send_queue_kb,
    MAX(redo_queue_kb) AS max_redo_queue_kb
FROM sqlserver_ag_health
WHERE capture_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY 1, 2, 3, 4, 5;

-- Database Throughput Summary
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_db_throughput_summary AS
SELECT 
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_instance_name,
    database_name,
    AVG(tps) AS avg_tps,
    AVG(batch_requests_per_sec) AS avg_batch_requests,
    SUM(total_reads) AS total_reads,
    SUM(total_writes) AS total_writes
FROM sqlserver_database_throughput
WHERE capture_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY 1, 2, 3;
