-- Metric: pg_server_start_time
-- Source: backend/internal/repository/pg_stats.go:443
-- Target Table: N/A (server info)
-- Description: Gets PostgreSQL postmaster start time for uptime calculation

SELECT pg_postmaster_start_time();
