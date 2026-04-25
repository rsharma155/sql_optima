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
    metric_time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    wait_category TEXT NOT NULL,
    wait_time_ms BIGINT DEFAULT 0,
    signal_wait_time_ms BIGINT DEFAULT 0,
    waiting_tasks BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_wait_stats', 'metric_time', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wait_stats_instance_time ON sqlserver_wait_stats (server_instance_name, metric_time DESC);

-- 2) PERF COUNTERS (DELTA)
DROP TABLE IF EXISTS sqlserver_perf_counters CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_perf_counters (
    metric_time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    counter_name TEXT NOT NULL,
    value_per_sec DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_perf_counters', 'metric_time', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_perf_counters_instance_time ON sqlserver_perf_counters (server_instance_name, metric_time DESC);

-- 3) FILE IO STATS (DELTA)
DROP TABLE IF EXISTS sqlserver_file_io CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_file_io (
    metric_time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    database_name TEXT NOT NULL,
    file_type TEXT NOT NULL,
    read_latency_ms DOUBLE PRECISION DEFAULT 0,
    write_latency_ms DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_file_io', 'metric_time', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_file_io_instance_time ON sqlserver_file_io (server_instance_name, metric_time DESC);

-- 4) PLAN CACHE SNAPSHOT
DROP TABLE IF EXISTS sqlserver_plan_cache CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_plan_cache (
    metric_time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    cache_type TEXT NOT NULL,
    size_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_plan_cache', 'metric_time', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_plan_cache_instance_time ON sqlserver_plan_cache (server_instance_name, metric_time DESC);

-- 5) MEMORY CLERKS SNAPSHOT
DROP TABLE IF EXISTS sqlserver_memory_clerks CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_memory_clerks (
    metric_time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    clerk_name TEXT NOT NULL,
    pages_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_memory_clerks', 'metric_time', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_memory_clerks_instance_time ON sqlserver_memory_clerks (server_instance_name, metric_time DESC);

-- 6) MEMORY GRANTS SNAPSHOT
DROP TABLE IF EXISTS sqlserver_memory_grants CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_memory_grants (
    metric_time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    pending_grants INTEGER DEFAULT 0,
    active_grants INTEGER DEFAULT 0,
    granted_memory_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_memory_grants', 'metric_time', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_memory_grants_instance_time ON sqlserver_memory_grants (server_instance_name, metric_time DESC);

-- 7) TEMPDB TOP CONSUMERS SNAPSHOT
DROP TABLE IF EXISTS sqlserver_tempdb_consumers CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_tempdb_consumers (
    metric_time TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    session_id INTEGER NOT NULL,
    request_id INTEGER,
    allocated_mb NUMERIC(19,4) DEFAULT 0,
    user_object_mb NUMERIC(19,4) DEFAULT 0,
    internal_object_mb NUMERIC(19,4) DEFAULT 0,
    query_text TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_tempdb_consumers', 'metric_time', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_tempdb_consumers_instance_time ON sqlserver_tempdb_consumers (server_instance_name, metric_time DESC);

-- Retention Policy (7 days for raw consumers)
SELECT add_retention_policy('sqlserver_tempdb_consumers', INTERVAL '7 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- CONTINUOUS AGGREGATES (Standard Downsampling)
-- --------------------------------------------------------------------------

-- Wait Stats Hourly
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_ca_wait_stats_hourly
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 hour', metric_time) AS bucket,
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
    timescaledb.compress_orderby = 'metric_time DESC'
);
SELECT add_compression_policy('sqlserver_wait_stats', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE sqlserver_perf_counters SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,counter_name',
    timescaledb.compress_orderby = 'metric_time DESC'
);
SELECT add_compression_policy('sqlserver_perf_counters', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE sqlserver_file_io SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,database_name',
    timescaledb.compress_orderby = 'metric_time DESC'
);
SELECT add_compression_policy('sqlserver_file_io', INTERVAL '7 days', if_not_exists => TRUE);
