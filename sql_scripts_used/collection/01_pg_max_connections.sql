-- Metric: pg_max_connections
-- Source: backend/internal/repository/pg_stats.go:478
-- Target Table: N/A (system stats estimation)
-- Description: Gets max_connections setting from pg_settings

SELECT setting::int FROM pg_settings WHERE name = 'max_connections';
