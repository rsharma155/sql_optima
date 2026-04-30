-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Enhanced PostgreSQL Memory Intelligence schema.
--          Includes hypertables for raw metrics, OS-level telemetry, and derived analytics.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

CREATE SCHEMA IF NOT EXISTS monitor;

-- 1. Host Memory Metrics Hypertable
CREATE TABLE IF NOT EXISTS monitor.host_memory_samples (
    ts                  TIMESTAMPTZ NOT NULL,
    server_id           TEXT NOT NULL,
    server_instance_name TEXT NOT NULL,
    total_memory_mb     BIGINT,
    used_memory_mb      BIGINT,
    free_memory_mb      BIGINT,
    cached_memory_mb    BIGINT,
    buffered_memory_mb  BIGINT,
    swap_total_mb       BIGINT,
    swap_used_mb        BIGINT,
    page_faults         BIGINT,
    major_page_faults   BIGINT
);
SELECT create_hypertable('monitor.host_memory_samples', 'ts', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_host_mem_instance_ts ON monitor.host_memory_samples (server_instance_name, ts DESC);

-- 2. PostgreSQL Process & Connection Metrics Hypertable
CREATE TABLE IF NOT EXISTS monitor.pg_memory_samples (
    ts                      TIMESTAMPTZ NOT NULL,
    server_instance_name    TEXT NOT NULL,
    postgres_rss_mb         BIGINT,
    postgres_vsz_mb         BIGINT,
    active_connections      INT,
    idle_connections        INT,
    total_connections       INT,
    blks_hit                BIGINT,
    blks_read               BIGINT,
    temp_files              BIGINT,
    temp_bytes              BIGINT,
    buffers_checkpoint      BIGINT,
    buffers_clean           BIGINT,
    buffers_backend         BIGINT
);
SELECT create_hypertable('monitor.pg_memory_samples', 'ts', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_mem_instance_ts ON monitor.pg_memory_samples (server_instance_name, ts DESC);

-- 3. PostgreSQL Memory Configuration (Snapshot)
CREATE TABLE IF NOT EXISTS monitor.pg_memory_components (
    ts                      TIMESTAMPTZ NOT NULL,
    server_instance_name    TEXT NOT NULL,
    shared_buffers_mb       BIGINT,
    work_mem_mb             BIGINT,
    maintenance_work_mem_mb BIGINT,
    wal_buffers_mb          BIGINT,
    temp_buffers_mb         BIGINT,
    max_connections         INTEGER DEFAULT 100
);
SELECT create_hypertable('monitor.pg_memory_components', 'ts', chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_comp_instance_ts ON monitor.pg_memory_components (server_instance_name, ts DESC);

-- 4. Derived Memory Intelligence Metrics
CREATE TABLE IF NOT EXISTS monitor.pg_memory_derived (
    ts                          TIMESTAMPTZ NOT NULL,
    server_instance_name        TEXT NOT NULL,
    pg_memory_percent           DOUBLE PRECISION,
    cache_hit_ratio             DOUBLE PRECISION,
    temp_spill_rate_mb_s        DOUBLE PRECISION,
    swap_used_percent           DOUBLE PRECISION,
    connection_memory_est_mb    DOUBLE PRECISION,
    memory_pressure_percent     DOUBLE PRECISION,
    health_score                INT
);
SELECT create_hypertable('monitor.pg_memory_derived', 'ts', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_der_instance_ts ON monitor.pg_memory_derived (server_instance_name, ts DESC);

-- 5. OS Collector Registry & Metrics (Agent Integration)
CREATE TABLE IF NOT EXISTS monitor.pg_os_host_instance (
    host_id        BIGSERIAL PRIMARY KEY,
    hostname       TEXT NOT NULL UNIQUE,
    ip_address     INET,
    environment    TEXT,
    cpu_cores      INT,
    total_memory_bytes BIGINT,
    created_at     TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS monitor.pg_os_memory_samples (
    ts TIMESTAMPTZ NOT NULL,
    host_id BIGINT NOT NULL REFERENCES monitor.pg_os_host_instance(host_id),
    total_bytes BIGINT,
    available_bytes BIGINT,
    used_bytes BIGINT,
    free_bytes BIGINT,
    cached_bytes BIGINT,
    buffers_bytes BIGINT,
    shared_bytes BIGINT,
    slab_bytes BIGINT,
    swap_total_bytes BIGINT,
    swap_used_bytes BIGINT,
    swap_free_bytes BIGINT,
    dirty_bytes BIGINT,
    writeback_bytes BIGINT,
    page_faults BIGINT,
    major_page_faults BIGINT,
    oom_kill_count BIGINT
);
SELECT create_hypertable('monitor.pg_os_memory_samples', 'ts', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_os_mem_host_ts ON monitor.pg_os_memory_samples (host_id, ts DESC);

CREATE TABLE IF NOT EXISTS monitor.pg_os_memory_pressure (
    ts TIMESTAMPTZ NOT NULL,
    host_id BIGINT NOT NULL REFERENCES monitor.pg_os_host_instance(host_id),
    memory_used_pct NUMERIC(5,2),
    memory_available_pct NUMERIC(5,2),
    swap_used_pct NUMERIC(5,2),
    pressure_score DOUBLE PRECISION,
    commit_limit_bytes BIGINT,
    committed_as_bytes BIGINT
);
SELECT create_hypertable('monitor.pg_os_memory_pressure', 'ts', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS monitor.pg_os_process_memory (
    ts TIMESTAMPTZ NOT NULL,
    host_id BIGINT NOT NULL REFERENCES monitor.pg_os_host_instance(host_id),
    postgres_rss_bytes BIGINT,
    postgres_vsz_bytes BIGINT,
    backend_count INT
);
SELECT create_hypertable('monitor.pg_os_process_memory', 'ts', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS monitor.pg_os_cpu_samples (
    ts TIMESTAMPTZ NOT NULL,
    host_id BIGINT NOT NULL REFERENCES monitor.pg_os_host_instance(host_id),
    cpu_user_pct NUMERIC(5,2),
    cpu_system_pct NUMERIC(5,2),
    cpu_idle_pct NUMERIC(5,2),
    cpu_iowait_pct NUMERIC(5,2),
    cpu_steal_pct NUMERIC(5,2),
    load_1m NUMERIC(5,2),
    load_5m NUMERIC(5,2),
    load_15m NUMERIC(5,2),
    context_switches BIGINT,
    interrupts BIGINT
);
SELECT create_hypertable('monitor.pg_os_cpu_samples', 'ts', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_os_cpu_host_ts ON monitor.pg_os_cpu_samples (host_id, ts DESC);

-- 6. Compression Policies (TimescaleDB)
ALTER TABLE monitor.host_memory_samples SET (timescaledb.compress, timescaledb.compress_segmentby = 'server_instance_name');
SELECT add_compression_policy('monitor.host_memory_samples', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE monitor.pg_memory_samples SET (timescaledb.compress, timescaledb.compress_segmentby = 'server_instance_name');
SELECT add_compression_policy('monitor.pg_memory_samples', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE monitor.pg_memory_derived SET (timescaledb.compress, timescaledb.compress_segmentby = 'server_instance_name');
SELECT add_compression_policy('monitor.pg_memory_derived', INTERVAL '7 days', if_not_exists => TRUE);
