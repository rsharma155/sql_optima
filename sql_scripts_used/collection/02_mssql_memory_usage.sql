-- Metric: mssql_memory_usage
-- Source: backend/internal/repository/mssql_dashboard.go:171
-- Target Table: sqlserver_metrics (TimescaleDB)
-- Description: Calculates memory usage as Total Server Memory / Target Server Memory percentage

SELECT 
    ISNULL((CAST(MAX(CASE WHEN counter_name = 'Total Server Memory (KB)' THEN cntr_value END) AS FLOAT) / 
    NULLIF(MAX(CASE WHEN counter_name = 'Target Server Memory (KB)' THEN cntr_value END), 0)) * 100.0, 0)
FROM sys.dm_os_performance_counters 
WHERE object_name LIKE '%Memory Manager%';
