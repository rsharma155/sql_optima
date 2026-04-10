-- Metric: pg_sessions
-- Source: backend/internal/repository/pg_stats.go:704
-- Target Table: N/A (session monitoring)
-- Description: Returns active sessions with blocking information from pg_stat_activity

SELECT 
    pid,
    usename,
    datname,
    application_name,
    state,
    EXTRACT(EPOCH FROM (now() - state_change)) as duration_seconds,
    wait_event_type || ':' || wait_event as wait_event,
    pg_blocking_pids(pid) as blocked_by,
    query
FROM pg_stat_activity 
WHERE pid <> pg_backend_pid()
ORDER BY state_change DESC
LIMIT 100;
