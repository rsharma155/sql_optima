-- ============================================================================
-- SQL Optima: Best Practices - SQL Server Memory Signal Collection
-- Purpose: Collect memory configuration for evaluation
-- Version: 1.0.0
-- Last Updated: 2026-04-22
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- Related Rule: max_server_memory
-- ============================================================================
SELECT
    CAST(cntr_value AS BIGINT) / 1024 AS total_memory_mb,
    (SELECT CAST(value AS BIGINT) / 1024
     FROM sys.configuration
     WHERE name = 'max server memory (MB)') AS max_server_memory_mb
FROM sys.dm_os_performance_counters
WHERE counter_name = 'Target Server Memory (KB)';