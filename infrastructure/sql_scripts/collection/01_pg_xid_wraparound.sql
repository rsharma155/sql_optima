-- Metric: pg_xid_wraparound
-- Source: backend/internal/repository/pg_stats.go:1554
-- Target Table: N/A (DBA health observation)
-- Description: Calculates XID wraparound percentage toward forced read-only mode

SELECT 
    COALESCE(MAX(age(datfrozenxid)), 0),
    COALESCE((MAX(age(datfrozenxid))::float / NULLIF(current_setting('autovacuum_freeze_max_age')::float, 0)) * 100, 0)
FROM pg_database 
WHERE datistemplate = false;
