-- Metric: ts_insert_sqlserver_connection_history
-- Source: backend/internal/storage/hot/ts_logger.go:405
-- Target Table: sqlserver_connection_history
-- Description: Batch inserts connection stats grouped by login and database

INSERT INTO sqlserver_connection_history (capture_timestamp, server_instance_name, login_name, database_name, active_connections, active_requests)
VALUES ($1, $2, $3, $4, $5, $6);
