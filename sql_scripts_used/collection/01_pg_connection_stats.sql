-- Metric: pg_connection_stats
-- Source: backend/internal/repository/pg_stats.go:354
-- Target Table: postgres_connection_stats (TimescaleDB)
-- Description: Returns active, idle, and total connection counts from pg_stat_activity

SELECT 
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE state = 'active') as active,
    COUNT(*) FILTER (WHERE state = 'idle') as idle
FROM pg_stat_activity
WHERE datname IS NOT NULL;
