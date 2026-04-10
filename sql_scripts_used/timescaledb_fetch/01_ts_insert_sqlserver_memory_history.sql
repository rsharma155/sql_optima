-- Metric: ts_insert_sqlserver_memory_history
-- Source: backend/internal/storage/hot/ts_logger.go:361
-- Target Table: sqlserver_memory_history
-- Description: Inserts Page Life Expectancy value into memory history

INSERT INTO sqlserver_memory_history (capture_timestamp, server_instance_name, page_life_expectancy)
VALUES ($1, $2, $3);
