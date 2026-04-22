-- ============================================================================
-- SQL Optima: Best Practices - SQL Server Parallelism Signal Collection
-- Purpose: Collect MAXDOP and cost threshold for evaluation
-- Version: 1.0.0
-- Last Updated: 2026-04-22
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- Related Rule: maxdop, cost_threshold
-- ============================================================================
SELECT
    (SELECT CAST(value_in_use AS INT) FROM sys.configurations WHERE name = 'max degree of parallelism') AS maxdop,
    (SELECT CAST(value_in_use AS INT) FROM sys.configurations WHERE name = 'cost threshold for parallelism') AS cost_threshold,
    (SELECT cpu_count FROM sys.dm_os_sys_info) AS cpu_count,
    0 AS parallel_wait_pct;