-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Add continuous aggregates (1h, 1d) on top of pgss_delta_1m and
--          system_metrics_1min for long-range dashboard queries, plus tiered
--          retention policies (raw → 14d, 1m → 30d, 1h → 90d, 1d → 365d).
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

-- +goose Up

-- ═══════════════════════════════════════════════════════════════
-- 1. PGSS hourly continuous aggregate on pgss_delta_1m
-- ═══════════════════════════════════════════════════════════════
CREATE MATERIALIZED VIEW IF NOT EXISTS pgss_delta_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', capture_timestamp) AS bucket,
    server_instance_name,
    query_id,
    SUM(calls)              AS calls,
    SUM(total_exec_time)    AS total_exec_time,
    SUM(rows)               AS rows,
    SUM(shared_blks_hit)    AS shared_blks_hit,
    SUM(shared_blks_read)   AS shared_blks_read,
    SUM(temp_blks_written)  AS temp_blks_written,
    SUM(wal_bytes)          AS wal_bytes,
    SUM(total_plan_time)    AS total_plan_time,
    AVG(mean_exec_time)     AS mean_exec_time
FROM pgss_delta_1m
GROUP BY bucket, server_instance_name, query_id
WITH NO DATA;

DO $$
BEGIN
    CALL add_continuous_aggregate_policy('pgss_delta_1h',
        start_offset  => INTERVAL '2 hours',
        end_offset    => INTERVAL '1 hour',
        schedule_interval => INTERVAL '1 hour',
        if_not_exists => TRUE
    );
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;

-- ═══════════════════════════════════════════════════════════════
-- 2. PGSS daily continuous aggregate on pgss_delta_1h
-- ═══════════════════════════════════════════════════════════════
CREATE MATERIALIZED VIEW IF NOT EXISTS pgss_delta_1d
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', bucket) AS bucket,
    server_instance_name,
    query_id,
    SUM(calls)              AS calls,
    SUM(total_exec_time)    AS total_exec_time,
    SUM(rows)               AS rows,
    SUM(shared_blks_hit)    AS shared_blks_hit,
    SUM(shared_blks_read)   AS shared_blks_read,
    SUM(temp_blks_written)  AS temp_blks_written,
    SUM(wal_bytes)          AS wal_bytes,
    SUM(total_plan_time)    AS total_plan_time,
    AVG(mean_exec_time)     AS mean_exec_time
FROM pgss_delta_1h
GROUP BY bucket, server_instance_name, query_id
WITH NO DATA;

DO $$
BEGIN
    CALL add_continuous_aggregate_policy('pgss_delta_1d',
        start_offset  => INTERVAL '2 days',
        end_offset    => INTERVAL '1 day',
        schedule_interval => INTERVAL '1 day',
        if_not_exists => TRUE
    );
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;

-- ═══════════════════════════════════════════════════════════════
-- 3. System metrics hourly continuous aggregate on system_metrics_1min
-- ═══════════════════════════════════════════════════════════════
CREATE MATERIALIZED VIEW IF NOT EXISTS system_metrics_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', bucket) AS bucket,
    server_name,
    metric_name,
    AVG(metric_value_avg)   AS metric_value_avg,
    MIN(metric_value_min)   AS metric_value_min,
    MAX(metric_value_max)   AS metric_value_max,
    SUM(sample_count)       AS sample_count
FROM system_metrics_1min
GROUP BY bucket, server_name, metric_name
WITH NO DATA;

DO $$
BEGIN
    CALL add_continuous_aggregate_policy('system_metrics_1h',
        start_offset  => INTERVAL '2 hours',
        end_offset    => INTERVAL '1 hour',
        schedule_interval => INTERVAL '1 hour',
        if_not_exists => TRUE
    );
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;

-- ═══════════════════════════════════════════════════════════════
-- 4. System metrics daily continuous aggregate on system_metrics_1h
-- ═══════════════════════════════════════════════════════════════
CREATE MATERIALIZED VIEW IF NOT EXISTS system_metrics_1d
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', bucket) AS bucket,
    server_name,
    metric_name,
    AVG(metric_value_avg)   AS metric_value_avg,
    MIN(metric_value_min)   AS metric_value_min,
    MAX(metric_value_max)   AS metric_value_max,
    SUM(sample_count)       AS sample_count
FROM system_metrics_1h
GROUP BY bucket, server_name, metric_name
WITH NO DATA;

DO $$
BEGIN
    CALL add_continuous_aggregate_policy('system_metrics_1d',
        start_offset  => INTERVAL '2 days',
        end_offset    => INTERVAL '1 day',
        schedule_interval => INTERVAL '1 day',
        if_not_exists => TRUE
    );
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;

-- ═══════════════════════════════════════════════════════════════
-- 5. Tiered retention policies
-- ═══════════════════════════════════════════════════════════════

-- Raw system_metrics: 14 days
SELECT add_retention_policy('system_metrics', INTERVAL '14 days', if_not_exists => TRUE);

-- system_metrics_1min continuous aggregate: 30 days
SELECT add_retention_policy('system_metrics_1min', INTERVAL '30 days', if_not_exists => TRUE);

-- system_metrics_1h continuous aggregate: 90 days
SELECT add_retention_policy('system_metrics_1h', INTERVAL '90 days', if_not_exists => TRUE);

-- system_metrics_1d continuous aggregate: 365 days
SELECT add_retention_policy('system_metrics_1d', INTERVAL '365 days', if_not_exists => TRUE);

-- pgss_delta_1m already has 30-day retention from migration 00001 — no change needed

-- pgss_delta_1h continuous aggregate: 90 days
SELECT add_retention_policy('pgss_delta_1h', INTERVAL '90 days', if_not_exists => TRUE);

-- pgss_delta_1d continuous aggregate: 365 days
SELECT add_retention_policy('pgss_delta_1d', INTERVAL '365 days', if_not_exists => TRUE);


-- +goose Down

-- Remove retention policies (reverse order)
SELECT remove_retention_policy('pgss_delta_1d', if_exists => TRUE);
SELECT remove_retention_policy('pgss_delta_1h', if_exists => TRUE);
SELECT remove_retention_policy('system_metrics_1d', if_exists => TRUE);
SELECT remove_retention_policy('system_metrics_1h', if_exists => TRUE);
SELECT remove_retention_policy('system_metrics_1min', if_exists => TRUE);
SELECT remove_retention_policy('system_metrics', if_exists => TRUE);

-- Drop continuous aggregates (reverse dependency order)
DROP MATERIALIZED VIEW IF EXISTS system_metrics_1d CASCADE;
DROP MATERIALIZED VIEW IF EXISTS system_metrics_1h CASCADE;
DROP MATERIALIZED VIEW IF EXISTS pgss_delta_1d CASCADE;
DROP MATERIALIZED VIEW IF EXISTS pgss_delta_1h CASCADE;
