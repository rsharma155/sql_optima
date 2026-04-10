-- Metric: ts_insert_postgres_archiver
-- Source: backend/internal/storage/hot/ts_logger.go:1087
-- Target Table: postgres_archiver_stats
-- Description: Inserts PostgreSQL WAL archiver statistics with failed count delta

INSERT INTO postgres_archiver_stats (
    capture_timestamp, server_instance_name,
    archived_count, failed_count, last_archived_wal, last_failed_wal, failed_count_delta
) VALUES ($1, $2, $3, $4, $5, $6, $7);
