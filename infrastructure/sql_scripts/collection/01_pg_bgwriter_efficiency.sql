-- Metric: pg_bgwriter_efficiency
-- Source: backend/internal/repository/pg_stats.go:1179
-- Target Table: postgres_bgwriter_stats (TimescaleDB)
-- Description: Gets BGWriter buffers_backend and maxwritten_clean for efficiency calculation

SELECT buffers_backend, maxwritten_clean FROM pg_stat_bgwriter;
