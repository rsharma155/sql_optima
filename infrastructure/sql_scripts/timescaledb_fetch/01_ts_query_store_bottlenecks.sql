-- Metric: ts_query_store_bottlenecks
-- Source: backend/internal/storage/hot/ts_logger.go:771
-- Target Table: sqlserver_query_store_stats
-- Description: Identifies query bottlenecks by aggregating Query Store stats by query_hash

SELECT 
    query_hash,
    MAX(query_text) AS query_text,
    SUM(executions) AS total_executions,
    AVG(avg_duration_ms) AS avg_duration_ms,
    AVG(avg_cpu_ms) AS avg_cpu_ms,
    AVG(avg_logical_reads) AS avg_logical_reads,
    SUM(total_cpu_ms) AS total_cpu_ms,
    COUNT(DISTINCT database_name) AS database_count,
    COUNT(*) AS sample_count
FROM sqlserver_query_store_stats
WHERE server_instance_name = $1
  AND capture_timestamp >= NOW() - INTERVAL '24 hours'
GROUP BY query_hash
ORDER BY SUM(total_cpu_ms) DESC
LIMIT $2;
