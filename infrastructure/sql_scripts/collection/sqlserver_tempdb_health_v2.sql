-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: TempDB health metrics including version store and space usage for V2 Dashboard.
-- Metadata:
--   - Version: 1.0
--   - Scope: Instance
--   - Frequency: 60s
--   - Source: sys.dm_db_file_space_usage, sys.dm_tran_version_store, sys.dm_os_waiting_tasks
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

SELECT 
    ISNULL(SUM(user_object_reserved_page_count) * 8 / 1024, 0) as user_obj_mb,
    ISNULL(SUM(internal_object_reserved_page_count) * 8 / 1024, 0) as internal_obj_mb,
    ISNULL(SUM(version_store_reserved_page_count) * 8 / 1024, 0) as version_store_mb,
    ISNULL(SUM(unallocated_extent_page_count) * 8 / 1024, 0) as free_mb,
    ISNULL((SELECT cntr_value FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE counter_name = 'Log File(s) Used Size (KB)' AND instance_name = 'tempdb'), 0) / 1024 as log_used_mb,
    CAST(CASE WHEN EXISTS (
        SELECT 1 FROM sys.dm_os_waiting_tasks 
        WHERE wait_type IN ('PAGELATCH_UP', 'PAGELATCH_SH', 'PAGELATCH_EX', 'PAGELATCH_DT')
        AND resource_description LIKE '2:%' -- Database 2 is tempdb
        AND (resource_description LIKE '%:1' OR resource_description LIKE '%:2' OR resource_description LIKE '%:3') -- PFS, GAM, SGAM
    ) THEN 1 ELSE 0 END AS BIT) as contention_found
FROM tempdb.sys.dm_db_file_space_usage WITH (NOLOCK);
