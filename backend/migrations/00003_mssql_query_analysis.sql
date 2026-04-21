-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Create TimescaleDB tables for the SQL Server Query Analysis Dashboard:
--          mssql_watched_queries, mssql_watched_query_snapshots, mssql_watched_query_events,
--          mssql_query_regressions, mssql_plan_instability.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

-- +goose Up

-- 1. Watched queries registry (max 10 per instance, enforced at application layer)
CREATE TABLE IF NOT EXISTS mssql_watched_queries (
    id                    SERIAL PRIMARY KEY,
    server_instance_name  TEXT NOT NULL,
    query_hash            TEXT,
    object_id             INT,
    name                  TEXT NOT NULL,
    query_text            TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (server_instance_name, query_hash)
);

-- 2. Watched query time-series snapshots (collected every 5 min)
CREATE TABLE IF NOT EXISTS mssql_watched_query_snapshots (
    snapshot_time         TIMESTAMPTZ NOT NULL,
    watched_id            INT NOT NULL REFERENCES mssql_watched_queries(id) ON DELETE CASCADE,
    server_instance_name  TEXT NOT NULL,
    executions            BIGINT,
    avg_duration_ms       DOUBLE PRECISION,
    avg_cpu_ms            DOUBLE PRECISION,
    avg_reads             DOUBLE PRECISION,
    total_duration_ms     DOUBLE PRECISION,
    total_cpu_ms          DOUBLE PRECISION,
    plan_count            INT,
    last_execution_time   TIMESTAMPTZ
);

SELECT create_hypertable('mssql_watched_query_snapshots', 'snapshot_time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_mssql_wqs_watched ON mssql_watched_query_snapshots (watched_id, snapshot_time DESC);
CREATE INDEX IF NOT EXISTS idx_mssql_wqs_instance ON mssql_watched_query_snapshots (server_instance_name, snapshot_time DESC);

ALTER TABLE mssql_watched_query_snapshots SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'watched_id,server_instance_name',
    timescaledb.compress_orderby = 'snapshot_time DESC'
);
SELECT add_compression_policy('mssql_watched_query_snapshots', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('mssql_watched_query_snapshots', INTERVAL '90 days', if_not_exists => TRUE);

-- 3. Watched query optimization event markers
CREATE TABLE IF NOT EXISTS mssql_watched_query_events (
    id           SERIAL PRIMARY KEY,
    watched_id   INT NOT NULL REFERENCES mssql_watched_queries(id) ON DELETE CASCADE,
    event_time   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type   TEXT NOT NULL,
    notes        TEXT
);
CREATE INDEX IF NOT EXISTS idx_mssql_wqe_watched ON mssql_watched_query_events (watched_id, event_time DESC);

-- 4. Query regression detection results (collected every 30 min)
CREATE TABLE IF NOT EXISTS mssql_query_regressions (
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

SELECT create_hypertable('mssql_query_regressions', 'capture_time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_mssql_qr_instance ON mssql_query_regressions (server_instance_name, capture_time DESC);

ALTER TABLE mssql_query_regressions SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_time DESC'
);
SELECT add_compression_policy('mssql_query_regressions', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('mssql_query_regressions', INTERVAL '30 days', if_not_exists => TRUE);

-- 5. Plan instability detection results (collected every 30 min)
CREATE TABLE IF NOT EXISTS mssql_plan_instability (
    capture_time          TIMESTAMPTZ NOT NULL,
    server_instance_name  TEXT NOT NULL,
    database_name         TEXT,
    query_hash            TEXT NOT NULL,
    query_text            TEXT,
    plan_count            INT NOT NULL,
    last_execution_time   TIMESTAMPTZ
);

SELECT create_hypertable('mssql_plan_instability', 'capture_time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_mssql_pi_instance ON mssql_plan_instability (server_instance_name, capture_time DESC);

ALTER TABLE mssql_plan_instability SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name',
    timescaledb.compress_orderby = 'capture_time DESC'
);
SELECT add_compression_policy('mssql_plan_instability', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('mssql_plan_instability', INTERVAL '30 days', if_not_exists => TRUE);

-- +goose Down
SELECT remove_retention_policy('mssql_plan_instability', if_exists => TRUE);
SELECT remove_compression_policy('mssql_plan_instability', if_exists => TRUE);
DROP TABLE IF EXISTS mssql_plan_instability;

SELECT remove_retention_policy('mssql_query_regressions', if_exists => TRUE);
SELECT remove_compression_policy('mssql_query_regressions', if_exists => TRUE);
DROP TABLE IF EXISTS mssql_query_regressions;

DROP TABLE IF EXISTS mssql_watched_query_events;

SELECT remove_retention_policy('mssql_watched_query_snapshots', if_exists => TRUE);
SELECT remove_compression_policy('mssql_watched_query_snapshots', if_exists => TRUE);
DROP TABLE IF EXISTS mssql_watched_query_snapshots;

DROP TABLE IF EXISTS mssql_watched_queries;
