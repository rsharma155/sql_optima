-- Metric: mssql_top_cpu_queries
-- Source: backend/internal/repository/mssql_dashboard.go:460
-- Target Table: sqlserver_top_queries (TimescaleDB)
-- Description: Returns top CPU-consuming queries from plan cache with statement-level text extraction

SELECT TOP 20
    DB_NAME(qt.dbid) AS [Database_Name],
    qs.execution_count AS [Executions],
    qs.total_worker_time / 1000.0 AS [Total_CPU_ms],
    CASE WHEN qs.execution_count > 0 THEN (qs.total_worker_time / qs.execution_count) / 1000.0 ELSE 0 END AS [Avg_CPU_ms],
    qs.total_logical_reads AS [Total_Logical_Reads],
    SUBSTRING(qt.text, (qs.statement_start_offset/2) + 1,
        ((CASE qs.statement_end_offset 
            WHEN -1 THEN DATALENGTH(qt.text) 
            ELSE qs.statement_end_offset 
        END - qs.statement_start_offset)/2) + 1) AS [Query_Text]
FROM sys.dm_exec_query_stats qs
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) qt
WHERE qt.text IS NOT NULL
ORDER BY qs.total_worker_time DESC;
