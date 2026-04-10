-- Metric: pg_blocking_tree
-- Source: backend/internal/repository/pg_stats.go:840
-- Target Table: N/A (blocking analysis)
-- Description: Returns all active sessions with blocking hierarchy from pg_stat_activity

SELECT 
    a.pid,
    a.state,
    EXTRACT(EPOCH FROM (now() - a.state_change)) as duration_seconds,
    COALESCE(a.wait_event_type || ':' || a.wait_event, '') as wait_event,
    LEFT(a.query, 100) as query,
    pg_blocking_pids(a.pid) as blocking_pids
FROM pg_stat_activity a
WHERE a.pid <> pg_backend_pid()
  AND a.state = 'active'
ORDER BY a.state_change DESC;
