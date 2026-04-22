-- ============================================================================
-- SQL Optima: Best Practices - PostgreSQL Autovacuum Signal Collection
-- Purpose: Collect autovacuum lag for evaluation
-- Version: 1.0.0
-- Last Updated: 2026-04-22
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- Related Rule: pg_autovacuum
-- ============================================================================
SELECT
    COALESCE(
        EXTRACT(EPOCH FROM (NOW() - MAX(last_autovacuum))),
        EXTRACT(EPOCH FROM (NOW() - MAX(last_vacuum))))
    AS autovacuum_lag
FROM pg_stat_user_tables
WHERE last_autovacuum IS NOT NULL OR last_vacuum IS NOT NULL;