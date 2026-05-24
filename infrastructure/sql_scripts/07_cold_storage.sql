-- SQL Optima — Cold Storage Control Schema
--
-- File: infrastructure/sql_scripts/07_cold_storage.sql
-- Purpose: Schema for tracking export progress (watermarking) and audit logging for the cold storage archival pipeline.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- Date: May 2026
--
-- Dependencies:
--   - 01_timescale_schema.sql (requires monitored_servers table)
--
-- Description:
--   Introduces the 'coldstorage' schema to isolate archival control data.
--   - coldstorage.watermarks: Tracks the high-water mark (timestamp) per (table, server).
--   - coldstorage.runs: Audit log of every archival execution cycle.
--   - coldstorage.status_view: Real-time visibility into archival lag and age.

CREATE SCHEMA IF NOT EXISTS coldstorage;

COMMENT ON SCHEMA coldstorage IS 'Control and metadata for the cold storage archival pipeline.';

-- coldstorage.watermarks: tracks the last successfully exported timestamp
-- for each (table, server) combination.
-- This table is NOT a hypertable — it is a small control table.
CREATE TABLE IF NOT EXISTS coldstorage.watermarks (
    table_name       TEXT        NOT NULL,
    server_id        UUID        NOT NULL,
    last_exported_at TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    export_rows_last INTEGER,         -- rows written in the last run (informational)
    export_bytes_last BIGINT,         -- bytes uploaded in the last run
    CONSTRAINT cold_export_watermarks_pkey PRIMARY KEY (table_name, server_id)
);

COMMENT ON TABLE coldstorage.watermarks IS
    'Tracks the high-water mark for cold storage exports per table and server. '
    'The exporter reads from last_exported_at and writes up to the cutoff '
    '(NOW() - lag_days). On failure the watermark is not advanced, ensuring '
    'at-least-once delivery to cold storage.';

-- coldstorage.runs: audit log of each export cycle
CREATE TABLE IF NOT EXISTS coldstorage.runs (
    cold_export_run_id            BIGSERIAL   PRIMARY KEY,
    run_started   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    run_finished  TIMESTAMPTZ,
    status        TEXT        NOT NULL DEFAULT 'running',  -- running | success | partial | failed | skipped
    tables_ok     INTEGER,
    tables_failed INTEGER,
    total_rows    BIGINT,
    total_bytes   BIGINT,
    error_detail  TEXT
);

CREATE INDEX IF NOT EXISTS idx_cold_export_runs_started ON coldstorage.runs (run_started DESC);

-- Helper view: shows which tables are behind on cold export
CREATE OR REPLACE VIEW coldstorage.status_view AS
SELECT
    cew.table_name,
    cew.server_id,
    ms.server_name,
    cew.last_exported_at,
    NOW() - cew.last_exported_at              AS age,
    cew.updated_at                            AS watermark_updated_at
FROM coldstorage.watermarks cew
LEFT JOIN public.monitored_servers ms ON ms.id = cew.server_id
ORDER BY age DESC NULLS FIRST;
