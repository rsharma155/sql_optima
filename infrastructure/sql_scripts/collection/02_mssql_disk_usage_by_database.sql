-- Metric: mssql_disk_usage_by_database
-- Source: backend/internal/repository/mssql_dashboard.go:292
-- Target Table: N/A (disk usage profile)
-- Description: Aggregates Data vs Log file sizes per database from sys.master_files

SELECT 
    ISNULL(DB_NAME(database_id), 'Unknown'),
    SUM(CASE WHEN type=0 THEN size * 8.0/1024.0 ELSE 0 END) as Data,
    SUM(CASE WHEN type=1 THEN size * 8.0/1024.0 ELSE 0 END) as Log
FROM sys.master_files
GROUP BY database_id;
