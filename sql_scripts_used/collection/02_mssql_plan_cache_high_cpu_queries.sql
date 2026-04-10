-- Metric: mssql_plan_cache_high_cpu_queries
-- Source: backend/internal/repository/mssql_dashboard.go:361
-- Target Table: sqlserver_top_queries (TimescaleDB)
-- Description: Captures recent high-CPU queries from plan cache meeting significant criteria

SELECT TOP 20
    ISNULL(s.login_name, 'System') as login_name,
    ISNULL(s.program_name, 'System') as program_name,
    ISNULL(DB_NAME(CAST(f.value AS INT)), 'Unknown') as database_name,
    ISNULL(LEFT(t.text, 500), 'Unknown') as query_text,
    'PLAN_CACHE' as wait_type,
    ISNULL(qs.total_worker_time / NULLIF(qs.execution_count, 0), 0) as cpu_time_ms,
    ISNULL(qs.total_elapsed_time / NULLIF(qs.execution_count, 0), 0) / 1000.0 as exec_time_ms,
    ISNULL(qs.total_logical_reads / NULLIF(qs.execution_count, 0), 0) as logical_reads,
    ISNULL(qs.execution_count, 1) as execution_count
FROM sys.dm_exec_query_stats qs
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) t
OUTER APPLY sys.dm_exec_plan_attributes(qs.plan_handle) f
WHERE t.text IS NOT NULL
  AND (qs.total_worker_time / NULLIF(qs.execution_count, 0) > 50
       OR qs.total_elapsed_time / NULLIF(qs.execution_count, 0) > 5000
       OR qs.total_logical_reads / NULLIF(qs.execution_count, 0) > 5000)
ORDER BY qs.total_worker_time DESC;
