-- Metric: pg_idle_in_transaction_count
-- Source: backend/internal/repository/pg_stats.go:1577
-- Target Table: N/A (DBA health observation)
-- Description: Counts dangerous idle-in-transaction connections from pg_stat_activity

SELECT COUNT(*) FROM pg_stat_activity
WHERE state IN ('idle in transaction', 'idle in transaction (aborted)');
