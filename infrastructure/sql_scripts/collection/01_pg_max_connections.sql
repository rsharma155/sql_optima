-- Metric: pg_max_connections
-- Source: backend/internal/repository/pg_session_metrics.go
-- Target Table: N/A (system stats estimation)
-- Description: Gets max_connections setting from pg_settings

SELECT /* SQL_OPTIMA */   setting::int FROM pg_settings WHERE name = 'max_connections';
