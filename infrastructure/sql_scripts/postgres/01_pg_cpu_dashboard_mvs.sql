-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Optional materialized views for PostgreSQL CPU dashboard (run on each monitored PostgreSQL instance).
--          The application queries pg_stat_statements directly with the same logic; these MVs are for DBA refresh/cron.
--
-- Prerequisites: pg_stat_statements extension, appropriate privileges (pg_read_all_stats recommended).
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_pg_cpu_by_db AS
SELECT
    d.datname::text AS datname,
    SUM(s.total_exec_time)::float8 AS total_exec_time_ms
FROM pg_stat_statements s
JOIN pg_database d ON d.oid = s.dbid
GROUP BY d.datname;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_pg_top_cpu_queries AS
SELECT
    s.queryid,
    LEFT(s.query, 400) AS query,
    s.total_exec_time::float8 AS total_exec_time,
    s.calls::bigint AS calls,
    CASE WHEN s.calls > 0 THEN (s.total_exec_time / s.calls)::float8 ELSE 0 END AS avg_ms
FROM pg_stat_statements s
ORDER BY s.total_exec_time DESC
LIMIT 20;

CREATE OR REPLACE FUNCTION pg_refresh_cpu_mvs()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    REFRESH MATERIALIZED VIEW mv_pg_cpu_by_db;
    REFRESH MATERIALIZED VIEW mv_pg_top_cpu_queries;
END;
$$;

COMMENT ON MATERIALIZED VIEW mv_pg_cpu_by_db IS 'SQL Optima: cumulative execution time by database (CPU dashboard donut).';
COMMENT ON MATERIALIZED VIEW mv_pg_top_cpu_queries IS 'SQL Optima: top 20 statements by total_exec_time.';
