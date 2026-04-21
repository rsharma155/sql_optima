-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Schema for host-level OS metrics collector (agent data) for PostgreSQL.
--          Includes hypertables for raw metrics and derived analytics.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

CREATE SCHEMA IF NOT EXISTS monitor;

-- 1. Host registry table
-- Stores physical/virtual host metadata linked to monitored instances.
CREATE TABLE IF NOT EXISTS monitor.pg_os_host_instance (
    host_id        BIGSERIAL PRIMARY KEY,
    hostname       TEXT NOT NULL UNIQUE,
    ip_address     INET,
    environment    TEXT,
    cpu_cores      INT,
    total_memory_bytes BIGINT,
    created_at     TIMESTAMPTZ DEFAULT now()
);

-- 2. OS memory snapshot hypertable
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

SELECT create_hypertable('monitor.pg_os_memory_samples', 'ts', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_os_mem_host_ts ON monitor.pg_os_memory_samples (host_id, ts DESC);

-- 3. Derived memory pressure metrics table
CREATE TABLE IF NOT EXISTS monitor.pg_os_memory_pressure (
    ts TIMESTAMPTZ NOT NULL,
    host_id BIGINT NOT NULL REFERENCES monitor.pg_os_host_instance(host_id),

    memory_used_pct NUMERIC(5,2),
    memory_available_pct NUMERIC(5,2),
    swap_used_pct NUMERIC(5,2),

    commit_limit_bytes BIGINT,
    committed_as_bytes BIGINT,

    pressure_score NUMERIC(5,2)
);

SELECT create_hypertable('monitor.pg_os_memory_pressure', 'ts', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_os_mem_press_host_ts ON monitor.pg_os_memory_pressure (host_id, ts DESC);

-- 4. Postgres process memory table
CREATE TABLE IF NOT EXISTS monitor.pg_os_process_memory (
    ts TIMESTAMPTZ NOT NULL,
    host_id BIGINT NOT NULL REFERENCES monitor.pg_os_host_instance(host_id),

    postgres_rss_bytes BIGINT,
    postgres_vsz_bytes BIGINT,
    postgres_shared_bytes BIGINT,
    postgres_private_bytes BIGINT,
    backend_count INT
);

SELECT create_hypertable('monitor.pg_os_process_memory', 'ts', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_os_proc_mem_host_ts ON monitor.pg_os_process_memory (host_id, ts DESC);

-- 5. CPU metrics table
CREATE TABLE IF NOT EXISTS monitor.pg_os_cpu_samples (
    ts TIMESTAMPTZ NOT NULL,
    host_id BIGINT NOT NULL REFERENCES monitor.pg_os_host_instance(host_id),
    cpu_user_pct NUMERIC(5,2),
    cpu_system_pct NUMERIC(5,2),
    cpu_idle_pct NUMERIC(5,2),
    cpu_iowait_pct NUMERIC(5,2),
    load_1m NUMERIC(5,2),
    load_5m NUMERIC(5,2),
    load_15m NUMERIC(5,2)
);

SELECT create_hypertable('monitor.pg_os_cpu_samples', 'ts', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_os_cpu_host_ts ON monitor.pg_os_cpu_samples (host_id, ts DESC);

-- 6. Continuous Aggregate for Dashboards
-- Fixed: use WITH NO DATA to avoid transaction/pipeline issues during creation
DROP MATERIALIZED VIEW IF EXISTS monitor.ca_pg_os_memory_5m;
CREATE MATERIALIZED VIEW monitor.ca_pg_os_memory_5m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('5 min', ts) AS bucket,
    host_id,
    avg(memory_used_pct) AS avg_used_pct,
    max(memory_used_pct) AS peak_used_pct,
    avg(swap_used_pct) AS avg_swap_pct,
    avg(pressure_score) AS pressure_score
FROM monitor.pg_os_memory_pressure
GROUP BY bucket, host_id
WITH NO DATA;
