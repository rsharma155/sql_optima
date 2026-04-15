-- Metric: pg_archiver_stats
-- Source: backend/internal/repository/pg_stats.go:1439
-- Target Table: postgres_archiver_stats (TimescaleDB)
-- Description: Retrieves WAL archiver statistics from pg_stat_archiver

SELECT 
    archived_count,
    failed_count,
    last_archived_wal,
    last_failed_wal
FROM pg_stat_archiver;
