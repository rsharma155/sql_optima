-- Metric: pg_wal_generation_rate
-- Source: backend/internal/repository/pg_replication_metrics.go
-- Target Table: postgres_replication_stats (TimescaleDB)
-- Description: Gets WAL generation rate (simplified calculation)

SELECT /* SQL_OPTIMA */   
    CASE 
        WHEN pg_current_wal_lsn() IS NOT NULL 
        THEN EXTRACT(EPOCH FROM (pg_current_wal_lsn() - pg_current_wal_lsn())) / 60
        ELSE 0 
    END;
