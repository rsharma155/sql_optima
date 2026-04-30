-- Metric: pg_query_stats
-- Source: backend/internal/repository/pg_statement_metrics.go
-- Target Table: N/A (query performance from pg_stat_statements)
-- Description: Returns top queries by total execution time from pg_stat_statements

SELECT /* SQL_OPTIMA */   
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
  AND query NOT LIKE '%/* SQL_OPTIMA */%'
ORDER BY total_exec_time DESC
LIMIT 50;
