-- Metric: pg_index_hit_rate
-- Source: backend/internal/repository/pg_stats.go:1566
-- Target Table: N/A (DBA health observation)
-- Description: Calculates index lookup vs sequential scan ratio from pg_stat_user_tables

SELECT CASE WHEN (SUM(idx_tup_fetch) + SUM(seq_tup_read)) > 0 
    THEN (SUM(idx_tup_fetch)::float / NULLIF((SUM(idx_tup_fetch) + SUM(seq_tup_read)), 0)) * 100 
    ELSE 0 END
FROM pg_stat_user_tables;
