-- Metric: pg_table_bloat_alert
-- Source: backend/internal/repository/pg_stats.go:1307
-- Target Table: N/A (alerting)
-- Description: Checks for tables with high dead tuple counts from pg_stat_user_tables

SELECT
    schemaname || '.' || tablename as table_name,
    n_dead_tup as dead_tuples,
    CASE WHEN n_live_tup > 0 THEN (n_dead_tup::float / n_live_tup) * 100 ELSE 0 END as bloat_pct
FROM pg_stat_user_tables
WHERE n_dead_tup > 1000
ORDER BY n_dead_tup DESC
LIMIT 3;
