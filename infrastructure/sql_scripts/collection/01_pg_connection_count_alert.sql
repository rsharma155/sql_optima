-- Metric: pg_connection_count_alert
-- Source: backend/internal/repository/pg_system_metrics.go
-- Target Table: N/A (alerting)
-- Description: Gets current connection count for threshold alerting

SELECT /* SQL_OPTIMA */   count(*) FROM pg_stat_activity;
