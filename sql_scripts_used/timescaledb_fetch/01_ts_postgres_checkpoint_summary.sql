-- Metric: ts_postgres_checkpoint_summary
-- Source: backend/internal/storage/hot/ts_logger.go:1131
-- Target Table: postgres_bgwriter_stats
-- Description: Fetches checkpoint statistics aggregated into 5-minute buckets over 24 hours

SELECT 
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    AVG(checkpoints_timed) AS avg_checkpoints_timed,
    AVG(checkpoints_req) AS avg_checkpoints_req,
    SUM(CASE WHEN checkpoints_req > 0 THEN 1 ELSE 0 END) AS req_checkpoint_events,
    AVG(checkpoint_write_time) AS avg_checkpoint_write_time,
    AVG(buffers_checkpoint) AS avg_buffers_checkpoint,
    MAX(buffers_checkpoint) AS max_buffers_checkpoint
FROM postgres_bgwriter_stats
WHERE server_instance_name = $1
  AND capture_timestamp >= NOW() - INTERVAL '24 hours'
GROUP BY bucket
ORDER BY bucket DESC
LIMIT $2;
