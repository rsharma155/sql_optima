-- Metric: pg_query_stats
-- Source: backend/internal/repository/pg_stats.go:936
-- Target Table: N/A (query performance from pg_stat_statements)
-- Description: Returns top queries by total execution time from pg_stat_statements

SELECT 
    queryid,
    LEFT(query, 100) as query,
    calls,
    total_exec_time,
    mean_exec_time,
    rows,
    temp_blks_read,
    temp_blks_written,
    blk_read_time,
    blk_write_time
FROM pg_stat_statements 
WHERE query NOT LIKE '%pg_stat_statements%'
ORDER BY total_exec_time DESC
LIMIT 50;
