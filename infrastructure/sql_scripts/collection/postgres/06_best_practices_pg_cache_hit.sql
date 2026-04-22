-- ============================================================================
-- SQL Optima: Best Practices - PostgreSQL Cache Hit Ratio Signal Collection
-- Purpose: Collect cache hit ratio for efficiency evaluation
-- Version: 1.0.0
-- Last Updated: 2026-04-22
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- Related Rule: pg_cache_hit
-- ============================================================================
SELECT
    CASE WHEN sum(heap_blks_hit + heap_blks_read) > 0
         THEN sum(heap_blks_hit)::NUMERIC / sum(heap_blks_hit + heap_blks_read)
         ELSE 0
    END AS cache_hit_ratio
FROM pg_statio_user_tables;