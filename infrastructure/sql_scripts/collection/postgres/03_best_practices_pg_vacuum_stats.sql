-- ============================================================================
-- SQL Optima: Best Practices - PostgreSQL Vacuum Stats Signal Collection
-- Purpose: Collect vacuum statistics for health evaluation
-- Version: 1.0.0
-- Last Updated: 2026-04-22
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- Related Rule: pg_vacuum_health
-- ============================================================================
SELECT
    CASE WHEN SUM(n_live_tup + n_dead_tup) > 0
         THEN (SUM(n_dead_tup) * 100.0) / SUM(n_live_tup + n_dead_tup)
         ELSE 0
    END AS dead_tuple_pct,
    EXTRACT(EPOCH FROM (NOW() - MAX(COALESCE(last_autovacuum, last_vacuum))) AS last_vacuum_hours
FROM pg_stat_user_tables;