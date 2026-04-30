-- Metric: pg_bgwriter_efficiency
-- Source: backend/internal/repository/pg_replication_metrics.go
-- Target Table: postgres_bgwriter_stats (TimescaleDB)
-- Description: Gets BGWriter buffers_backend and maxwritten_clean for efficiency calculation

SELECT /* SQL_OPTIMA */   buffers_backend, maxwritten_clean FROM pg_stat_bgwriter;
