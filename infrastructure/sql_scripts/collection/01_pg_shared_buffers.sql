-- Metric: pg_shared_buffers
-- Source: backend/internal/repository/pg_system_metrics.go
-- Target Table: N/A (system stats estimation)
-- Description: Gets shared_buffers setting for memory usage estimation

SELECT /* SQL_OPTIMA */   (setting::bigint * 8192) / 1024 / 1024
FROM pg_settings
WHERE name = 'shared_buffers';
