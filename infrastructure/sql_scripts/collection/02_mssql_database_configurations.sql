-- Metric: mssql_database_configurations
-- Source: backend/internal/repository/mssql_best_practices.go:84
-- Target Table: N/A (best practices audit)
-- Description: Fetches database-level configuration settings for health audit

SELECT
    name AS [Database_Name],
    page_verify_option_desc AS [Page_Verify],
    is_auto_shrink_on AS [Auto_Shrink],
    is_auto_close_on AS [Auto_Close],
    target_recovery_time_in_seconds AS [Target_Recovery_Time]
FROM sys.databases WITH (NOLOCK)
WHERE database_id > 4
  AND state_desc = 'ONLINE';
