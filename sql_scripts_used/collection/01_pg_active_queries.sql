-- Metric: pg_active_queries
-- Source: backend/internal/repository/pg_stats.go:624
-- Target Table: N/A (session monitoring)
-- Description: Returns all currently active queries from pg_stat_activity

SELECT
    pid,
    COALESCE(usename::text, '') AS usename,
    COALESCE(datname::text, '') AS datname,
    COALESCE(client_addr::text, '') AS client_addr,
    client_port,
    backend_start,
    query_start,
    state_change,
    wait_event_type,
    wait_event,
    state,
    query
FROM pg_stat_activity
WHERE pid <> pg_backend_pid()
  AND state = 'active'
  AND COALESCE(query_start, xact_start) IS NOT NULL
  AND query NOT ILIKE '%pg_stat_activity%'
  AND query NOT ILIKE 'autovacuum:%'
ORDER BY COALESCE(query_start, xact_start) ASC NULLS LAST
LIMIT 100;
