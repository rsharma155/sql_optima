-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Create historical snapshot hypertables for SQL Server Storage & Index Health.
--          Supports time-series trends, forecasting, and efficiency analysis.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

CREATE SCHEMA IF NOT EXISTS snapshot;

-- 1) Database Storage History
CREATE TABLE IF NOT EXISTS snapshot.mssql_db_storage_history (
    snapshot_time      TIMESTAMPTZ NOT NULL,
    server_name        TEXT NOT NULL,
    instance_name      TEXT NOT NULL,
    database_name      TEXT NOT NULL,
    total_size_mb      NUMERIC(18,2),
    data_size_mb       NUMERIC(18,2),
    log_size_mb        NUMERIC(18,2)
);

SELECT create_hypertable(
    'snapshot.mssql_db_storage_history',
    'snapshot_time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_mssql_db_storage_history_lookup ON snapshot.mssql_db_storage_history
(server_name, instance_name, database_name, snapshot_time DESC);

-- 2) Table Size History
CREATE TABLE IF NOT EXISTS snapshot.mssql_table_size_history (
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

SELECT create_hypertable(
    'snapshot.mssql_table_size_history',
    'snapshot_time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_mssql_table_size_history_lookup ON snapshot.mssql_table_size_history
(server_name, instance_name, database_name, schema_name, table_name, snapshot_time DESC);

-- 3) Index Usage + Size History
CREATE TABLE IF NOT EXISTS snapshot.mssql_index_usage_history (
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

SELECT create_hypertable(
    'snapshot.mssql_index_usage_history',
    'snapshot_time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_mssql_index_usage_history_lookup ON snapshot.mssql_index_usage_history
(server_name, instance_name, database_name, schema_name, table_name, index_name, snapshot_time DESC);

-- 4) Index Fragmentation History
CREATE TABLE IF NOT EXISTS snapshot.mssql_index_fragmentation_history (
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

SELECT create_hypertable(
    'snapshot.mssql_index_fragmentation_history',
    'snapshot_time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_mssql_index_frag_history_lookup ON snapshot.mssql_index_fragmentation_history
(server_name, instance_name, database_name, schema_name, table_name, index_name, snapshot_time DESC);

-- 5) Table Structure / Risk Snapshot
CREATE TABLE IF NOT EXISTS snapshot.mssql_table_structure_history (
    snapshot_time       TIMESTAMPTZ NOT NULL,
    server_name         TEXT NOT NULL,
    instance_name       TEXT NOT NULL,
    database_name       TEXT NOT NULL,
    schema_name         TEXT NOT NULL,
    table_name          TEXT NOT NULL,
    has_clustered_index BOOLEAN,
    has_primary_key     BOOLEAN
);

SELECT create_hypertable(
    'snapshot.mssql_table_structure_history',
    'snapshot_time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_mssql_table_struct_history_lookup ON snapshot.mssql_table_structure_history
(server_name, instance_name, database_name, schema_name, table_name, snapshot_time DESC);

-- 6) Compression Policies
ALTER TABLE snapshot.mssql_db_storage_history SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'server_name,instance_name,database_name'
);
SELECT add_compression_policy('snapshot.mssql_db_storage_history', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE snapshot.mssql_table_size_history SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'server_name,instance_name,database_name'
);
SELECT add_compression_policy('snapshot.mssql_table_size_history', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE snapshot.mssql_index_usage_history SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'server_name,instance_name,database_name'
);
SELECT add_compression_policy('snapshot.mssql_index_usage_history', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE snapshot.mssql_index_fragmentation_history SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'server_name,instance_name,database_name'
);
SELECT add_compression_policy('snapshot.mssql_index_fragmentation_history', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE snapshot.mssql_table_structure_history SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'server_name,instance_name,database_name'
);
SELECT add_compression_policy('snapshot.mssql_table_structure_history', INTERVAL '7 days', if_not_exists => TRUE);

-- 7) Retention Policies
SELECT add_retention_policy('snapshot.mssql_db_storage_history', INTERVAL '2 years', if_not_exists => TRUE);
SELECT add_retention_policy('snapshot.mssql_table_size_history', INTERVAL '2 years', if_not_exists => TRUE);
SELECT add_retention_policy('snapshot.mssql_index_usage_history', INTERVAL '2 years', if_not_exists => TRUE);
SELECT add_retention_policy('snapshot.mssql_index_fragmentation_history', INTERVAL '2 years', if_not_exists => TRUE);
SELECT add_retention_policy('snapshot.mssql_table_structure_history', INTERVAL '2 years', if_not_exists => TRUE);
