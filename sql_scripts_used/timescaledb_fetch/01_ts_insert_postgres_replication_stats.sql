-- Metric: ts_insert_postgres_replication_stats
-- Source: backend/internal/storage/hot/ts_logger.go:524
-- Target Table: postgres_replication_stats
-- Description: Inserts PostgreSQL replication statistics (primary/standby, lag, WAL rate, BGWriter efficiency)

INSERT INTO postgres_replication_stats (capture_timestamp, server_instance_name, is_primary, cluster_state, max_lag_mb, wal_gen_rate_mbps, bgwriter_eff_pct)
VALUES ($1, $2, $3, $4, $5, $6, $7);
