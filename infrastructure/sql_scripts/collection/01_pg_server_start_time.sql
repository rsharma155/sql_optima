-- Metric: pg_server_start_time
-- Source: backend/internal/repository/pg_system_metrics.go
-- Target Table: N/A (server info)
-- Description: Gets PostgreSQL postmaster start time for uptime calculation

SELECT /* SQL_OPTIMA */   pg_postmaster_start_time();
