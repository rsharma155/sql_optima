-- Metric: mssql_server_configurations
-- Source: backend/internal/repository/mssql_best_practices.go:50
-- Target Table: N/A (best practices audit)
-- Description: Fetches key server-level configuration settings for health audit

SELECT
    name AS [Configuration_Name],
    CAST(value_in_use AS VARCHAR(50)) AS [Current_Value]
FROM sys.configurations WITH (NOLOCK)
WHERE name IN (
    'max server memory (MB)',
    'max degree of parallelism',
    'cost threshold for parallelism',
    'optimize for ad hoc workloads',
    'backup compression default'
);
