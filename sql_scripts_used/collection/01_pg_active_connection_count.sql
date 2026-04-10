-- Metric: pg_active_connection_count
-- Source: backend/internal/repository/pg_stats.go:472
-- Target Table: N/A (system stats estimation)
-- Description: Counts active connections for CPU usage estimation

SELECT count(*) FROM pg_stat_activity WHERE state = 'active';
