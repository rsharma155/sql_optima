-- Metric: pg_database_discovery
-- Source: backend/internal/repository/pg_repository_core.go
-- Target Table: N/A (collects from pg_database)
-- Description: Auto-discovers user databases by listing non-template databases

SELECT /* SQL_OPTIMA */   datname 
FROM pg_database 
WHERE datistemplate = false 
  AND datname NOT IN ('postgres');
