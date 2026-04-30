-- Metric: pg_idle_in_transaction_count
-- Source: backend/internal/repository/pg_dba_metrics.go
-- Target Table: N/A (DBA health observation)
-- Description: Counts dangerous idle-in-transaction connections from pg_stat_activity

SELECT /* SQL_OPTIMA */   COUNT(*) FROM pg_stat_activity
WHERE state IN ('idle in transaction', 'idle in transaction (aborted)');
