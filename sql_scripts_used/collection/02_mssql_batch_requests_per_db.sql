-- Metric: mssql_batch_requests_per_db
-- Source: backend/internal/repository/mssql_stats.go:510
-- Target Table: N/A (database throughput enrichment)
-- Description: Gets current batch request count per database from active requests

SELECT 
    DB_NAME(r.database_id) AS database_name,
    COUNT(*) AS batch_count
FROM sys.dm_exec_requests r
WHERE r.database_id > 4
GROUP BY r.database_id;
