-- Metric: pg_stat_statements_check
-- Source: backend/internal/repository/pg_statement_metrics.go
-- Target Table: N/A (extension check)
-- Description: Checks if pg_stat_statements extension is installed

SELECT /* SQL_OPTIMA */   EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements');
