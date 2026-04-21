-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Extend postgres_query_stats with additional pg_stat_statements columns,
--          create pgss_query_dim dimension table, and pgss_delta_1m hypertable
--          for the enhanced PostgreSQL Query Performance dashboard.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

-- +goose Up

-- 1. Extend postgres_query_stats with missing pg_stat_statements columns
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS shared_blks_hit BIGINT DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS shared_blks_read BIGINT DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS shared_blks_dirtied BIGINT DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS shared_blks_written BIGINT DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS wal_bytes NUMERIC DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS wal_records BIGINT DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS wal_fpi BIGINT DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS total_plan_time DOUBLE PRECISION DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS mean_plan_time DOUBLE PRECISION DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS plans BIGINT DEFAULT 0;
ALTER TABLE postgres_query_stats ADD COLUMN IF NOT EXISTS userid OID;

-- 2. Query text dimension table (stores query text once per instance + queryid)
CREATE TABLE IF NOT EXISTS pgss_query_dim (
    server_instance_name TEXT NOT NULL,
    query_id BIGINT NOT NULL,
    query_text TEXT,
    first_seen TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (server_instance_name, query_id)
);

-- 3. Pre-aggregated per-minute per-query delta table for fast dashboard queries
CREATE TABLE IF NOT EXISTS pgss_delta_1m (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    query_id BIGINT NOT NULL,
    calls BIGINT DEFAULT 0,
    total_exec_time DOUBLE PRECISION DEFAULT 0,
    rows BIGINT DEFAULT 0,
    shared_blks_hit BIGINT DEFAULT 0,
    shared_blks_read BIGINT DEFAULT 0,
    temp_blks_written BIGINT DEFAULT 0,
    wal_bytes NUMERIC DEFAULT 0,
    total_plan_time DOUBLE PRECISION DEFAULT 0,
    mean_exec_time DOUBLE PRECISION DEFAULT 0
);

SELECT create_hypertable('pgss_delta_1m', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pgss_delta_1m_server ON pgss_delta_1m (server_instance_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pgss_delta_1m_query ON pgss_delta_1m (server_instance_name, query_id, capture_timestamp DESC);

ALTER TABLE pgss_delta_1m SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_instance_name,query_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('pgss_delta_1m', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('pgss_delta_1m', INTERVAL '30 days', if_not_exists => TRUE);

-- +goose Down
SELECT remove_retention_policy('pgss_delta_1m', if_exists => TRUE);
SELECT remove_compression_policy('pgss_delta_1m', if_exists => TRUE);
DROP TABLE IF EXISTS pgss_delta_1m;
DROP TABLE IF EXISTS pgss_query_dim;

ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS shared_blks_hit;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS shared_blks_read;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS shared_blks_dirtied;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS shared_blks_written;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS wal_bytes;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS wal_records;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS wal_fpi;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS total_plan_time;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS mean_plan_time;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS plans;
ALTER TABLE postgres_query_stats DROP COLUMN IF EXISTS userid;
