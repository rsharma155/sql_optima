-- Metric: mssql_query_store_stats
-- Source: backend/internal/repository/mssql_stats.go:183
-- Target Table: sqlserver_query_store_stats (TimescaleDB)
-- Description: Fetches top 50 queries from Query Store for the last completed 15-minute interval

SELECT TOP 50
    CONVERT(VARCHAR(64), q.query_hash, 1) AS query_hash,
    LEFT(qt.query_sql_text, 500) AS query_text,
    ISNULL(rs.count_executions, 0) AS executions,
    ISNULL(rs.avg_duration, 0) / 1000.0 AS avg_duration_ms,
    ISNULL(rs.avg_cpu_time, 0) / 1000.0 AS avg_cpu_ms,
    ISNULL(rs.avg_logical_io_reads, 0) AS avg_logical_reads,
    (ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 1)) / 1000.0 AS total_cpu_ms
FROM sys.query_store_query q
INNER JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
INNER JOIN sys.query_store_plan p ON q.query_id = p.query_id
INNER JOIN sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
INNER JOIN sys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
WHERE rsi.end_time >= DATEADD(minute, -16, GETUTCDATE())
  AND q.is_internal_query = 0
  AND ISNULL(rs.count_executions, 0) > 0
ORDER BY (rs.avg_cpu_time * rs.count_executions) DESC;
