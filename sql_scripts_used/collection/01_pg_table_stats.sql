-- Metric: pg_table_stats
-- Source: backend/internal/repository/pg_stats.go:995
-- Target Table: N/A (table storage analysis)
-- Description: Returns table statistics including size, bloat, and vacuum info from pg_stat_user_tables

SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as total_size,
    n_dead_tup,
    CASE WHEN n_live_tup + n_dead_tup > 0 THEN (n_dead_tup::float / (n_live_tup + n_dead_tup)) * 100 ELSE 0 END as bloat_pct,
    seq_scan,
    idx_scan,
    last_vacuum,
    last_analyze
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
LIMIT 20;
