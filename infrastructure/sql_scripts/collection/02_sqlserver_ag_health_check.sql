-- Metric: sqlserver_ag_health_check
-- Source: backend/internal/repository/sqlserver_stats.go:260
-- Target Table: N/A (availability group check)
-- Description: Checks if AlwaysOn Availability Groups are configured

SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.dm_hadr_availability_group_states;
