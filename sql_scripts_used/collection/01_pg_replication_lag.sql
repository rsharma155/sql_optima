-- Metric: pg_replication_lag
-- Source: backend/internal/repository/pg_stats.go:389
-- Target Table: postgres_replication_stats (TimescaleDB)
-- Description: Calculates replication lag in MB on standby using LSN difference

SELECT 
    CASE 
        WHEN pg_last_wal_replay_lsn() IS NOT NULL AND pg_last_wal_receive_lsn() IS NOT NULL 
        THEN EXTRACT(EPOCH FROM (pg_last_wal_receive_lsn() - pg_last_wal_replay_lsn())) / 1024 / 1024
        ELSE 0 
    END as lag_mb;
