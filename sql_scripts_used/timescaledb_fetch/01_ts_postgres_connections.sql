-- Metric: ts_postgres_connections
-- Source: backend/internal/storage/hot/ts_logger.go:229
-- Target Table: postgres_connection_stats
-- Description: Fetches latest PostgreSQL connection statistics

SELECT capture_timestamp, server_instance_name, total_connections, active_connections, idle_connections
FROM postgres_connection_stats
WHERE server_instance_name = $1
ORDER BY capture_timestamp DESC
LIMIT $2;
