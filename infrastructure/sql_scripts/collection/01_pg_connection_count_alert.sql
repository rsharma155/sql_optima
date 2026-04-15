-- Metric: pg_connection_count_alert
-- Source: backend/internal/repository/pg_stats.go:1277
-- Target Table: N/A (alerting)
-- Description: Gets current connection count for threshold alerting

SELECT count(*) FROM pg_stat_activity;
