-- Metric: pg_index_stats
-- Source: backend/internal/repository/pg_stats.go:1048
-- Target Table: N/A (index usage analysis)
-- Description: Returns potentially unused indexes (idx_scan = 0) from pg_stat_user_indexes

SELECT 
    indexrelname,
    tablename,
    pg_size_pretty(pg_relation_size(indexrelid)) as size,
    idx_scan,
    CASE 
        WHEN idx_scan = 0 THEN 'Unused'
        ELSE 'OK'
    END as reason
FROM pg_stat_user_indexes
WHERE idx_scan = 0
ORDER BY pg_relation_size(indexrelid) DESC
LIMIT 10;
