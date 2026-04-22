-- ============================================================================
-- SQL Optima: Best Practices - PostgreSQL shared_buffers Signal Collection
-- Purpose: Collect shared_buffers configuration for evaluation
-- Version: 1.0.0
-- Last Updated: 2026-04-22
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- Related Rule: pg_shared_buffers
-- ============================================================================
SELECT
    CAST(current_setting('shared_buffers') AS NUMERIC) / 1024 AS shared_buffers_mb,
    (SELECT CAST(sum(ko.value)::NUMERIC / 1024 AS NUMERIC)
     FROM pg_catalog.pg_settings ko
     WHERE ko.name = 'shared_buffers') AS total_memory_mb;