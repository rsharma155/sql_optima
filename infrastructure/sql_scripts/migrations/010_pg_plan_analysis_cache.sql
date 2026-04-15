-- ============================================================================
-- SQL Optima Migration: PostgreSQL plan analysis cache (pg_)
-- File: 010_pg_plan_analysis_cache.sql
-- Version: 010
-- Last Updated: 2026-04-15
--
-- Purpose:
--   Adds `plan_analysis_cache` to store deterministic EXPLAIN plan analysis reports.
--   Cache key is canonical JSON SHA-256 hash of the EXPLAIN (FORMAT JSON) payload.
--
-- Notes:
--   Runtime migrations are disabled in the API; prefer applying `00_timescale_schema.sql`
--   as the single source of truth. This file exists for environments that apply
--   incremental migrations manually.
-- ============================================================================

CREATE TABLE IF NOT EXISTS plan_analysis_cache (
    plan_hash TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL DEFAULT 1,
    query_text TEXT NULL,
    raw_plan_json JSONB NOT NULL,
    report_json JSONB NOT NULL,
    total_execution_time_ms DOUBLE PRECISION NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_plan_analysis_cache_updated_at ON plan_analysis_cache (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_plan_analysis_cache_exec_time ON plan_analysis_cache (total_execution_time_ms DESC);

COMMENT ON TABLE plan_analysis_cache IS 'Cache of deterministic EXPLAIN plan analysis reports (canonical JSON hash → report JSON).';

