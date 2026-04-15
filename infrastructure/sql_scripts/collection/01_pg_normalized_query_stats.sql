-- Metric: pg_normalized_query_stats
-- Source: backend/internal/repository/pg_stats.go:1490
-- Target Table: N/A (query performance from pg_stat_statements)
-- Description: Retrieves query statistics with query ID from pg_stat_statements for normalization

SELECT 
    queryid,
    query,
    calls,
    total_exec_time,
    mean_exec_time,
    rows
FROM pg_stat_statements 
WHERE query NOT LIKE '%pg_stat_statements%'
ORDER BY total_exec_time DESC
LIMIT 100;
