-- ============================================================================
-- SQL Optima: Best Practices - PostgreSQL Table Bloat Signal Collection
-- Purpose: Collect table bloat estimation for evaluation
-- Version: 1.0.0
-- Last Updated: 2026-04-22
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- Related Rule: pg_table_bloat
-- ============================================================================
SELECT
    schemaname,
    relname,
    pg_total_relation_size(relid) AS total_size,
    pg_relation_size(relid) AS table_size,
    CASE WHEN pg_relation_size(relid) > 0
         THEN (pg_total_relation_size(relid)::NUMERIC / pg_relation_size(relid))
         ELSE 0
    END AS bloat_ratio
FROM pg_catalog.pg_statio_user_tables
ORDER BY bloat_ratio DESC
LIMIT 10;