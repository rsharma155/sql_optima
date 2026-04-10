-- Metric: pg_replication_standbys
-- Source: backend/internal/repository/pg_stats.go:1105
-- Target Table: postgres_replication_stats (TimescaleDB)
-- Description: Gets standby replication information from pg_stat_replication on primary

SELECT 
    application_name as app_name,
    client_addr,
    state,
    sync_state,
    pg_wal_lsn_diff(sent_lsn, '0/0') as wal_sent_bytes,
    pg_wal_lsn_diff(sent_lsn, replay_lsn) as replay_lag_bytes,
    replay_lsn,
    write_lsn,
    flush_lsn,
    sent_lsn
FROM pg_stat_replication
ORDER BY application_name;
