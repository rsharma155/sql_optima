-- Metric: mssql_secondary_lag_column_check
-- Source: backend/internal/repository/mssql_stats.go:282
-- Target Table: N/A (AG capability check)
-- Description: Checks if secondary_lag_seconds column exists (SQL Server 2016 SP1+)

SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'secondary_lag_seconds';
