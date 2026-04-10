-- Metric: mssql_page_life_expectancy
-- Source: backend/internal/repository/mssql_dashboard.go:209
-- Target Table: sqlserver_memory_history (TimescaleDB)
-- Description: Gets current Page Life Expectancy value from Buffer Manager

SELECT ISNULL(CAST(cntr_value AS FLOAT), 0) FROM sys.dm_os_performance_counters WITH (NOLOCK) WHERE [counter_name] = N'Page life expectancy' AND [object_name] LIKE '%Buffer Manager%';
