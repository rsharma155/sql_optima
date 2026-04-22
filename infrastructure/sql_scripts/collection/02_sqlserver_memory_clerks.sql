-- Metric: sqlserver_memory_clerks
-- Source: backend/internal/repository/sqlserver_dashboard.go:189
-- Target Table: sqlserver_memory_history (TimescaleDB)
-- Description: Captures memory clerk breakdown for tracking consumption footprints

SELECT /* SQL_OPTIMA */   
    RTRIM(counter_name) as type,
    CAST(cntr_value AS FLOAT) / 1024.0 as size_mb
FROM sys.dm_os_performance_counters
WHERE object_name LIKE '%Memory Manager%'
AND counter_name IN ('Total Server Memory (KB)', 'Target Server Memory (KB)', 'Connection Memory (KB)', 'Lock Memory (KB)');
