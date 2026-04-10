-- Metric: ts_insert_postgres_connection_stats
-- Source: backend/internal/storage/hot/ts_logger.go:510
-- Target Table: postgres_connection_stats
-- Description: Inserts PostgreSQL connection statistics (total, active, idle)

INSERT INTO postgres_connection_stats (capture_timestamp, server_instance_name, total_connections, active_connections, idle_connections)
VALUES ($1, $2, $3, $4, $5);
