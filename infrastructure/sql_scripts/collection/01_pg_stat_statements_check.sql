-- Metric: pg_stat_statements_check
-- Source: backend/internal/repository/pg_stats.go:931
-- Target Table: N/A (extension check)
-- Description: Checks if pg_stat_statements extension is installed

SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements');
