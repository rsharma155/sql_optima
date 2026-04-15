-- Metric: mssql_ag_database_states_check
-- Source: backend/internal/repository/mssql_stats.go:274
-- Target Table: N/A (AG capability check)
-- Description: Checks if database states DMV is available

SELECT COUNT(*) FROM sys.dm_hadr_availability_database_states;
