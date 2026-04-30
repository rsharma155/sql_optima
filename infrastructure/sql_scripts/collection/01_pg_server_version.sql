-- Metric: pg_server_version
-- Source: backend/internal/repository/pg_system_metrics.go
-- Target Table: N/A (server info)
-- Description: Gets PostgreSQL version string

SELECT /* SQL_OPTIMA */   version();
