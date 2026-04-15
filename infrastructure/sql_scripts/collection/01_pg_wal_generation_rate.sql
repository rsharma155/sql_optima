-- Metric: pg_wal_generation_rate
-- Source: backend/internal/repository/pg_stats.go:1165
-- Target Table: postgres_replication_stats (TimescaleDB)
-- Description: Gets WAL generation rate (simplified calculation)

SELECT 
    CASE 
        WHEN pg_current_wal_lsn() IS NOT NULL 
        THEN EXTRACT(EPOCH FROM (pg_current_wal_lsn() - pg_current_wal_lsn())) / 60
        ELSE 0 
    END;
