-- ============================================================================
-- SQL Optima: Best Practices - PostgreSQL Index Usage Signal Collection
-- Purpose: Collect index usage statistics for evaluation
-- Version: 1.0.0
-- Last Updated: 2026-04-22
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- Related Rule: pg_unused_index
-- ============================================================================
SELECT
    relname AS table_name,
    indexrelname AS index_name,
    idx_scan,
    pg_relation_size(indexrelid) / 1024 / 1024 AS index_size_mb
FROM pg_stat_user_indexes
WHERE indexrelid > 0
ORDER BY pg_relation_size(indexrelid) DESC
LIMIT 20;