-- Metric: pg_database_discovery
-- Source: backend/internal/repository/pg_stats.go:119
-- Target Table: N/A (collects from pg_database)
-- Description: Auto-discovers user databases by listing non-template databases

SELECT datname 
FROM pg_database 
WHERE datistemplate = false 
  AND datname NOT IN ('postgres');
