-- Metric: ts_postgres_archive_summary
-- Source: backend/internal/storage/hot/ts_logger.go:1183
-- Target Table: postgres_archiver_stats
-- Description: Fetches WAL archive statistics aggregated into 5-minute buckets over 24 hours

SELECT 
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    SUM(archived_count) AS total_archived,
    SUM(failed_count) AS total_failed,
    MAX(failed_count) AS max_failed_in_period,
    MAX(CASE WHEN failed_count > 0 THEN 1 ELSE 0 END) AS has_failures
FROM postgres_archiver_stats
WHERE server_instance_name = $1
  AND capture_timestamp >= NOW() - INTERVAL '24 hours'
GROUP BY bucket
ORDER BY bucket DESC
LIMIT $2;
