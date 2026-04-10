-- Metric: pg_idle_in_transaction_alert
-- Source: backend/internal/repository/pg_stats.go:1243
-- Target Table: N/A (alerting)
-- Description: Checks for long-running idle-in-transaction sessions from pg_stat_activity

SELECT
    pid,
    EXTRACT(EPOCH FROM (now() - xact_start)) / 60 as duration_minutes,
    query
FROM pg_stat_activity
WHERE xact_start IS NOT NULL
  AND state = 'idle in transaction'
  AND EXTRACT(EPOCH FROM (now() - xact_start)) > 900
ORDER BY xact_start
LIMIT 5;
