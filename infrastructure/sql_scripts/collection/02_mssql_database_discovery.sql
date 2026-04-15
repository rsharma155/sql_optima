-- Metric: mssql_database_discovery
-- Source: backend/internal/repository/mssql_stats.go:76
-- Target Table: N/A (auto-discovers databases)
-- Description: Auto-discovers online user databases excluding system databases

SELECT name FROM sys.databases WHERE database_id > 4 AND state_desc = 'ONLINE';
