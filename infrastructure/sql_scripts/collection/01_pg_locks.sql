-- Metric: pg_locks
-- Source: backend/internal/repository/pg_stats.go:774
-- Target Table: N/A (lock monitoring)
-- Description: Returns all current locks with waiting time from pg_locks joined with pg_class and pg_stat_activity

SELECT 
    l.pid,
    l.locktype,
    COALESCE(r.relname || ' (' || r.oid || ')', 'virtual') as relation,
    l.mode,
    l.granted,
    CASE WHEN l.granted = false THEN EXTRACT(EPOCH FROM (now() - a.state_change)) ELSE 0 END as waiting_seconds
FROM pg_locks l
LEFT JOIN pg_class r ON l.relation = r.oid
LEFT JOIN pg_stat_activity a ON l.pid = a.pid
WHERE l.pid <> pg_backend_pid()
ORDER BY l.granted, waiting_seconds DESC
LIMIT 100;
