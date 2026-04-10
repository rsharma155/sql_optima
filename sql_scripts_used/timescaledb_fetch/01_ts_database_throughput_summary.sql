-- Metric: ts_database_throughput_summary
-- Source: backend/internal/storage/hot/ts_logger.go:996
-- Target Table: sqlserver_database_throughput
-- Description: Fetches database throughput summary with averages over the last hour

SELECT 
    database_name,
    AVG(tps) AS avg_tps,
    AVG(batch_requests_per_sec) AS avg_batch_requests,
    SUM(total_reads) AS total_reads,
    SUM(total_writes) AS total_writes,
    MAX(tps) AS max_tps,
    COUNT(*) AS sample_count
FROM sqlserver_database_throughput
WHERE server_instance_name = $1
  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
GROUP BY database_name
ORDER BY AVG(tps) DESC
LIMIT $2;
