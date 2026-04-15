-- Metric: pg_is_standby
-- Source: backend/internal/repository/pg_stats.go:379
-- Target Table: N/A (checks replication role)
-- Description: Checks if this instance is a standby using pg_is_in_recovery()

SELECT pg_is_in_recovery();
