-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Extend Timescale hypertable postgres_system_stats with host vs Postgres CPU,
--          load averages, and core count (replaces a separate pg_host_cpu_history table).
--          Numbered 009 here (was 005 in sql_scripts_used) to avoid collision with
--          005_enterprise_metrics_collection.sql in this migrations folder.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

ALTER TABLE postgres_system_stats
    ADD COLUMN IF NOT EXISTS host_cpu_percent DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS postgres_cpu_percent DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS load_1m DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS load_5m DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS load_15m DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cpu_cores INTEGER DEFAULT 0;

COMMENT ON COLUMN postgres_system_stats.host_cpu_percent IS 'Host OS CPU utilization % (e.g. from /proc or pg_stat_cpu).';
COMMENT ON COLUMN postgres_system_stats.postgres_cpu_percent IS 'Sum of postgres backend process CPU % on host when collector runs locally.';
COMMENT ON COLUMN postgres_system_stats.load_1m IS 'Load average 1m.';
COMMENT ON COLUMN postgres_system_stats.cpu_cores IS 'Logical CPU count used for saturation.';
