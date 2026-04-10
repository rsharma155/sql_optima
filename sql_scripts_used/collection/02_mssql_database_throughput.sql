-- Metric: mssql_database_throughput
-- Source: backend/internal/repository/mssql_stats.go:468
-- Target Table: sqlserver_database_throughput (TimescaleDB)
-- Description: Fetches per-database index usage throughput statistics

SELECT 
    DB_NAME(s.database_id) AS database_name,
    ISNULL(SUM(s.user_seeks), 0) AS idx_seeks,
    ISNULL(SUM(s.user_scans), 0) AS idx_scans,
    ISNULL(SUM(s.user_lookups), 0) AS idx_lookups,
    ISNULL(SUM(s.user_updates), 0) AS idx_updates,
    ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups), 0) AS total_idx_reads,
    ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups + s.user_updates), 0) AS total_idx_activity
FROM sys.dm_db_index_usage_stats s
WHERE s.database_id > 4
GROUP BY s.database_id
HAVING ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups + s.user_updates), 0) > 0
ORDER BY total_idx_activity DESC;
