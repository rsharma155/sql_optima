-- Metric: ts_ag_health_summary
-- Source: backend/internal/storage/hot/ts_logger.go:934
-- Target Table: sqlserver_ag_health
-- Description: Fetches AG health summary with averages and maxes over the last hour

SELECT 
    ag_name,
    replica_server_name,
    database_name,
    replica_role,
    synchronization_state,
    is_primary_replica,
    AVG(log_send_queue_kb) AS avg_log_send_queue_kb,
    AVG(redo_queue_kb) AS avg_redo_queue_kb,
    MAX(log_send_queue_kb) AS max_log_send_queue_kb,
    MAX(redo_queue_kb) AS max_redo_queue_kb,
    COUNT(*) AS sample_count
FROM sqlserver_ag_health
WHERE server_instance_name = $1
  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
GROUP BY ag_name, replica_server_name, database_name, replica_role, synchronization_state, is_primary_replica
ORDER BY MAX(log_send_queue_kb) DESC, MAX(redo_queue_kb) DESC
LIMIT $2;
