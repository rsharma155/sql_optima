-- Metric: pg_active_connection_count
-- Source: backend/internal/repository/pg_session_metrics.go
-- Target Table: N/A (system stats estimation)
-- Description: Counts active connections for CPU usage estimation

SELECT /* SQL_OPTIMA */   count(*) FROM pg_stat_activity WHERE state = 'active';
