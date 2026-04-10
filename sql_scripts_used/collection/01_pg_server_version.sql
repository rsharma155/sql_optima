-- Metric: pg_server_version
-- Source: backend/internal/repository/pg_stats.go:428
-- Target Table: N/A (server info)
-- Description: Gets PostgreSQL version string

SELECT version();
