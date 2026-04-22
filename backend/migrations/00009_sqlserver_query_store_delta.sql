-- Migration: 00009_sqlserver_query_store_delta.sql
-- Description: Pipeline for change-only Query Store snapshots and delta interval calculation.
-- Author: Ravi Sharma

CREATE SCHEMA IF NOT EXISTS monitor;

-- 1. Staging table for raw Query Store fetches (truncated every poll)
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

-- 2. Snapshot table (stores unique snapshots, change-only via fingerprint)
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
    row_fingerprint TEXT NOT NULL, -- md5 of cumulative stats + interval_id
    PRIMARY KEY (capture_time, server_instance_name, query_hash, plan_id, runtime_stats_interval_id)
);

SELECT create_hypertable('monitor.sqlserver_query_store_snapshot', 'capture_time', if_not_exists => TRUE);

-- 3. Interval table (True deltas between polls)
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
    
    is_reset BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (bucket_end, server_instance_name, query_hash, plan_id, runtime_stats_interval_id)
);

SELECT create_hypertable('monitor.sqlserver_query_store_interval', 'bucket_end', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_qs_interval_query ON monitor.sqlserver_query_store_interval (server_instance_name, query_hash, bucket_end DESC);
