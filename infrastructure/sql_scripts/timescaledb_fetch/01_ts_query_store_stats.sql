-- Metric: ts_query_store_stats
-- Source: backend/internal/storage/hot/ts_logger.go:710
-- Target Table: sqlserver_query_store_stats
-- Description: Fetches Query Store stats aggregated by 1-minute buckets with time_bucket

SELECT 
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_instance_name,
    database_name,
    query_hash,
    MAX(query_text) AS query_text,
    SUM(executions) AS total_executions,
    AVG(avg_duration_ms) AS avg_duration_ms,
    AVG(avg_cpu_ms) AS avg_cpu_ms,
    AVG(avg_logical_reads) AS avg_logical_reads,
    SUM(total_cpu_ms) AS total_cpu_ms
FROM sqlserver_query_store_stats
WHERE server_instance_name = $1
  AND capture_timestamp >= NOW() - INTERVAL '24 hours'
GROUP BY bucket, server_instance_name, database_name, query_hash
ORDER BY SUM(total_cpu_ms) DESC
LIMIT $2;
