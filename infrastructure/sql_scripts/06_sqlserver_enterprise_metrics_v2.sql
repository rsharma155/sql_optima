-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Standardized SQL Server enterprise metrics for time-series driven diagnostics.
-- This schema supports trend-first observability for wait stats, perf counters, I/O, and memory.
--
-- Metadata:
-- Engine: SQL Server
-- Tier: Enterprise / Advanced Monitoring
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

-- Robust cleanup of existing objects
DROP MATERIALIZED VIEW IF EXISTS sqlserver_ca_wait_stats_hourly CASCADE;

-- 1) WAIT STATS (DELTA)
DROP TABLE IF EXISTS sqlserver_wait_stats CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_wait_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_instance_name TEXT NOT NULL,
    wait_category TEXT NOT NULL,
    wait_time_ms BIGINT DEFAULT 0,
    signal_wait_time_ms BIGINT DEFAULT 0,
    waiting_tasks BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_wait_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wait_stats_instance_time ON sqlserver_wait_stats (server_instance_name, capture_timestamp DESC);

-- 2) PERF COUNTERS (DELTA)
DROP TABLE IF EXISTS sqlserver_perf_counters CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_perf_counters (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_instance_name TEXT NOT NULL,
    counter_name TEXT NOT NULL,
    value_per_sec DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_perf_counters', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_perf_counters_instance_time ON sqlserver_perf_counters (server_instance_name, capture_timestamp DESC);

-- 3) FILE IO STATS (DELTA)
DROP TABLE IF EXISTS sqlserver_file_io CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_file_io (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    file_name TEXT,
    file_type TEXT NOT NULL,
    read_latency_ms DOUBLE PRECISION DEFAULT 0,
    write_latency_ms DOUBLE PRECISION DEFAULT 0,
    read_bytes_per_sec DOUBLE PRECISION DEFAULT 0,
    write_bytes_per_sec DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_file_io', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_file_io_instance_time ON sqlserver_file_io (server_instance_name, capture_timestamp DESC);

-- 4) PLAN CACHE SNAPSHOT
DROP TABLE IF EXISTS sqlserver_plan_cache CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_plan_cache (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_instance_name TEXT NOT NULL,
    cache_type TEXT NOT NULL,
    size_mb NUMERIC(19,4) DEFAULT 0,
    total_cache_mb NUMERIC(19,4) DEFAULT 0,
    single_use_cache_mb NUMERIC(19,4) DEFAULT 0,
    single_use_cache_pct NUMERIC(19,4) DEFAULT 0,
    adhoc_cache_mb NUMERIC(19,4) DEFAULT 0,
    prepared_cache_mb NUMERIC(19,4) DEFAULT 0,
    proc_cache_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_plan_cache', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_plan_cache_instance_time ON sqlserver_plan_cache (server_instance_name, capture_timestamp DESC);

-- 5) MEMORY CLERKS SNAPSHOT
DROP TABLE IF EXISTS sqlserver_memory_clerks CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_memory_clerks (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_instance_name TEXT NOT NULL,
    clerk_name TEXT NOT NULL,
    clerk_type TEXT GENERATED ALWAYS AS (clerk_name) STORED,
    pages_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_memory_clerks', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_memory_clerks_instance_time ON sqlserver_memory_clerks (server_instance_name, capture_timestamp DESC);

-- 6) MEMORY GRANTS SNAPSHOT
DROP TABLE IF EXISTS sqlserver_memory_grants CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_memory_grants (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_instance_name TEXT NOT NULL,
    pending_grants INTEGER DEFAULT 0,
    active_grants INTEGER DEFAULT 0,
    granted_memory_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_memory_grants', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_memory_grants_instance_time ON sqlserver_memory_grants (server_instance_name, capture_timestamp DESC);

-- 7) TEMPDB TOP CONSUMERS SNAPSHOT
DROP TABLE IF EXISTS sqlserver_tempdb_consumers CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_tempdb_consumers (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED, -- Alias for backward compatibility
    server_instance_name TEXT NOT NULL,
    session_id INTEGER NOT NULL,
    request_id INTEGER,
    allocated_mb NUMERIC(19,4) DEFAULT 0,
    user_object_mb NUMERIC(19,4) DEFAULT 0,
    internal_object_mb NUMERIC(19,4) DEFAULT 0,
    query_text TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_tempdb_consumers', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_tempdb_consumers_instance_time ON sqlserver_tempdb_consumers (server_instance_name, capture_timestamp DESC);

-- Retention Policy (7 days for raw consumers)
SELECT add_retention_policy('sqlserver_tempdb_consumers', INTERVAL '7 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- LEGACY ALIASES (VIEWS)
-- These allow existing Go code to work while we transition to capture_timestamp naming.
-- --------------------------------------------------------------------------

CREATE OR REPLACE VIEW sqlserver_waits_delta AS 
SELECT capture_timestamp, server_instance_name, wait_category, wait_time_ms as wait_time_ms_delta, signal_wait_time_ms, waiting_tasks
FROM sqlserver_wait_stats;

CREATE OR REPLACE VIEW sqlserver_file_io_latency AS
SELECT capture_timestamp, server_instance_name, database_name, file_name, file_type, read_latency_ms, write_latency_ms, read_bytes_per_sec, write_bytes_per_sec
FROM sqlserver_file_io;

CREATE OR REPLACE VIEW sqlserver_plan_cache_health AS
SELECT capture_timestamp, server_instance_name, total_cache_mb, single_use_cache_mb, single_use_cache_pct, adhoc_cache_mb, prepared_cache_mb, proc_cache_mb
FROM sqlserver_plan_cache;

-- --------------------------------------------------------------------------
-- CONTINUOUS AGGREGATES (Standard Downsampling)
-- --------------------------------------------------------------------------

-- Wait Stats Hourly
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_ca_wait_stats_hourly
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 hour', capture_timestamp) AS bucket,
  server_instance_name,
  wait_category,
  avg(wait_time_ms) AS avg_wait_ms,
  sum(wait_time_ms) AS total_wait_ms
FROM sqlserver_wait_stats
GROUP BY bucket, server_instance_name, wait_category
WITH NO DATA;

-- --------------------------------------------------------------------------
-- Compression Policies
-- --------------------------------------------------------------------------
ALTER TABLE sqlserver_wait_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,wait_category',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_wait_stats', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE sqlserver_perf_counters SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,counter_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_perf_counters', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE sqlserver_file_io SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_file_io', INTERVAL '7 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 8) WATCHED QUERIES (Drilldown Analysis)
-- --------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS sqlserver_watched_queries (
    id                    SERIAL PRIMARY KEY,
    server_instance_name  TEXT NOT NULL,
    database_name         TEXT NOT NULL DEFAULT 'master',
    query_hash            TEXT,
    object_id             INT,
    name                  TEXT NOT NULL,
    query_text            TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sqlserver_watched_queries_scoped_unique UNIQUE (server_instance_name, database_name, query_hash)
);

CREATE TABLE IF NOT EXISTS sqlserver_watched_query_snapshots (
    snapshot_time         TIMESTAMPTZ NOT NULL,
    watched_id            INT NOT NULL REFERENCES sqlserver_watched_queries(id) ON DELETE CASCADE,
    server_instance_name  TEXT NOT NULL,
    executions            BIGINT,
    avg_duration_ms       DOUBLE PRECISION,
    avg_cpu_ms            DOUBLE PRECISION,
    avg_reads             DOUBLE PRECISION,
    total_duration_ms     DOUBLE PRECISION,
    total_cpu_ms          DOUBLE PRECISION,
    plan_count            INT,
    last_execution_time   TIMESTAMPTZ,
    query_plan            TEXT,
    wait_stats            JSONB
);

SELECT create_hypertable('sqlserver_watched_query_snapshots', 'snapshot_time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wqs_watched ON sqlserver_watched_query_snapshots (watched_id, snapshot_time DESC);

CREATE TABLE IF NOT EXISTS sqlserver_watched_query_events (
    id           SERIAL PRIMARY KEY,
    watched_id   INT NOT NULL REFERENCES sqlserver_watched_queries(id) ON DELETE CASCADE,
    event_time   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type   TEXT NOT NULL,
    notes        TEXT
);

-- --------------------------------------------------------------------------
-- 9) QUERY REGRESSION & INSTABILITY
-- --------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS sqlserver_query_regressions (
    capture_time          TIMESTAMPTZ NOT NULL,
    server_instance_name  TEXT NOT NULL,
    database_name         TEXT,
    query_hash            TEXT NOT NULL,
    query_text            TEXT,
    regression_type       TEXT NOT NULL,
    previous_avg          DOUBLE PRECISION,
    current_avg           DOUBLE PRECISION,
    percent_change        DOUBLE PRECISION,
    plan_changed          BOOLEAN DEFAULT FALSE
);
SELECT create_hypertable('sqlserver_query_regressions', 'capture_time', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS sqlserver_plan_instability (
    capture_time          TIMESTAMPTZ NOT NULL,
    server_instance_name  TEXT NOT NULL,
    database_name         TEXT,
    query_hash            TEXT NOT NULL,
    query_text            TEXT,
    plan_count            INT NOT NULL,
    last_execution_time   TIMESTAMPTZ
);
SELECT create_hypertable('sqlserver_plan_instability', 'capture_time', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 10) QUERY STORE PIPELINE (monitor schema)
-- --------------------------------------------------------------------------

CREATE SCHEMA IF NOT EXISTS monitor;

CREATE TABLE IF NOT EXISTS monitor.sqlserver_query_store_staging (
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    query_hash TEXT NOT NULL,
    query_text TEXT NOT NULL,
    plan_id BIGINT NOT NULL,
    runtime_stats_interval_id BIGINT NOT NULL,
    executions BIGINT NOT NULL,
    avg_duration_ms DOUBLE PRECISION NOT NULL,
    avg_cpu_ms DOUBLE PRECISION NOT NULL,
    avg_logical_reads DOUBLE PRECISION NOT NULL,
    total_cpu_ms DOUBLE PRECISION NOT NULL,
    last_execution_time TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS monitor.sqlserver_query_store_snapshot (
    capture_time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    query_hash TEXT NOT NULL,
    query_text TEXT NOT NULL,
    plan_id BIGINT NOT NULL,
    runtime_stats_interval_id BIGINT NOT NULL,
    total_executions BIGINT NOT NULL,
    total_cpu_ms DOUBLE PRECISION NOT NULL,
    total_duration_ms DOUBLE PRECISION NOT NULL,
    total_logical_reads DOUBLE PRECISION NOT NULL,
    row_fingerprint TEXT NOT NULL,
    PRIMARY KEY (capture_time, server_instance_name, query_hash, plan_id, runtime_stats_interval_id)
);
SELECT create_hypertable('monitor.sqlserver_query_store_snapshot', 'capture_time', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS monitor.sqlserver_query_store_interval (
    bucket_start TIMESTAMPTZ NOT NULL,
    bucket_end TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    query_hash TEXT NOT NULL,
    query_text TEXT NOT NULL,
    plan_id BIGINT NOT NULL,
    runtime_stats_interval_id BIGINT NOT NULL,
    delta_executions BIGINT NOT NULL,
    delta_cpu_ms DOUBLE PRECISION NOT NULL,
    delta_duration_ms DOUBLE PRECISION NOT NULL,
    delta_logical_reads DOUBLE PRECISION NOT NULL,
    avg_cpu_ms DOUBLE PRECISION,
    avg_duration_ms DOUBLE PRECISION,
    avg_reads DOUBLE PRECISION,
    is_reset BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (bucket_end, server_instance_name, query_hash, plan_id, runtime_stats_interval_id)
);
SELECT create_hypertable('monitor.sqlserver_query_store_interval', 'bucket_end', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 11) STORAGE & INDEX HEALTH HISTORY (snapshot schema)
-- --------------------------------------------------------------------------

CREATE SCHEMA IF NOT EXISTS snapshot;

CREATE TABLE IF NOT EXISTS snapshot.sqlserver_db_storage_history (
    snapshot_time      TIMESTAMPTZ NOT NULL,
    server_name        TEXT NOT NULL,
    instance_name      TEXT NOT NULL,
    database_name      TEXT NOT NULL,
    total_size_mb      NUMERIC(18,2),
    data_size_mb       NUMERIC(18,2),
    log_size_mb        NUMERIC(18,2)
);
SELECT create_hypertable('snapshot.sqlserver_db_storage_history', 'snapshot_time', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS snapshot.sqlserver_table_size_history (
    snapshot_time   TIMESTAMPTZ NOT NULL,
    server_name     TEXT NOT NULL,
    instance_name   TEXT NOT NULL,
    database_name   TEXT NOT NULL,
    schema_name     TEXT NOT NULL,
    table_name      TEXT NOT NULL,
    row_count       BIGINT,
    total_mb        NUMERIC(18,2),
    data_mb         NUMERIC(18,2),
    index_mb        NUMERIC(18,2)
);
SELECT create_hypertable('snapshot.sqlserver_table_size_history', 'snapshot_time', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS snapshot.sqlserver_index_usage_history (
    snapshot_time   TIMESTAMPTZ NOT NULL,
    server_name     TEXT NOT NULL,
    instance_name   TEXT NOT NULL,
    database_name   TEXT NOT NULL,
    schema_name     TEXT NOT NULL,
    table_name      TEXT NOT NULL,
    index_name      TEXT NOT NULL,
    index_type      TEXT,
    index_size_mb   NUMERIC(18,2),
    user_seeks      BIGINT,
    user_scans      BIGINT,
    user_lookups    BIGINT,
    user_updates    BIGINT
);
SELECT create_hypertable('snapshot.sqlserver_index_usage_history', 'snapshot_time', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS snapshot.sqlserver_index_fragmentation_history (
    snapshot_time   TIMESTAMPTZ NOT NULL,
    server_name     TEXT NOT NULL,
    instance_name   TEXT NOT NULL,
    database_name   TEXT NOT NULL,
    schema_name     TEXT NOT NULL,
    table_name      TEXT NOT NULL,
    index_name      TEXT NOT NULL,
    avg_fragmentation_pct DOUBLE PRECISION,
    page_count      BIGINT
);
SELECT create_hypertable('snapshot.sqlserver_index_fragmentation_history', 'snapshot_time', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS snapshot.sqlserver_table_structure_history (
    snapshot_time       TIMESTAMPTZ NOT NULL,
    server_name         TEXT NOT NULL,
    instance_name       TEXT NOT NULL,
    database_name       TEXT NOT NULL,
    schema_name         TEXT NOT NULL,
    table_name          TEXT NOT NULL,
    has_clustered_index BOOLEAN,
    has_primary_key     BOOLEAN
);
SELECT create_hypertable('snapshot.sqlserver_table_structure_history', 'snapshot_time', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 12) QUERY METRICS V2
-- --------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS sqlserver_query_metrics_v2(
 ts timestamptz NOT NULL,
 instance_id text,
 database_name text,
 login_name text,
 application_name text,
 query_hash bigint,
 plan_hash bigint,
 total_executions bigint,
 total_cpu_ms bigint,
 total_elapsed_ms bigint,
 total_logical_reads bigint,
 total_physical_reads bigint,
 total_rows bigint,
 statement_text text
);
SELECT create_hypertable('sqlserver_query_metrics_v2','ts',if_not_exists=>TRUE);

