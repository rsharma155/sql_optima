-- Metric: pg_list_databases
-- Source: backend/internal/repository/pg_stats.go:527
-- Target Table: N/A (database listing)
-- Description: Lists user databases ordered by name, excluding templates and postgres

SELECT datname 
FROM pg_database 
WHERE datistemplate = false 
  AND datname NOT IN ('postgres') 
ORDER BY datname;
