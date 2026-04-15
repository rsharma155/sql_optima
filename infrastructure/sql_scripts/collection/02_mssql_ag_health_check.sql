-- Metric: mssql_ag_health_check
-- Source: backend/internal/repository/mssql_stats.go:260
-- Target Table: N/A (availability group check)
-- Description: Checks if AlwaysOn Availability Groups are configured

SELECT COUNT(*) FROM sys.dm_hadr_availability_group_states;
