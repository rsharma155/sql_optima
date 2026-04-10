-- Metric: ts_sqlserver_top_queries
-- Source: backend/internal/storage/hot/ts_logger.go:571
-- Target Table: sqlserver_top_queries
-- Description: Fetches top queries by CPU time from TimescaleDB

SELECT capture_timestamp, server_instance_name, login_name, program_name, database_name, 
       query_text, wait_type, cpu_time_ms, exec_time_ms, execution_count
FROM sqlserver_top_queries
WHERE server_instance_name = $1
ORDER BY capture_timestamp DESC, cpu_time_ms DESC
LIMIT $2;
