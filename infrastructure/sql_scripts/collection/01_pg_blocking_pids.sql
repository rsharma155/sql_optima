-- Metric: pg_blocking_pids
-- Source: backend/internal/repository/pg_blocking_metrics.go
-- Target Table: N/A (blocking analysis)
-- Description: Gets blocking PIDs for a specific session

SELECT /* SQL_OPTIMA */   pg_blocking_pids($1);
