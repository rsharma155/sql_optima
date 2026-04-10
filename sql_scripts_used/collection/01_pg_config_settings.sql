-- Metric: pg_config_settings
-- Source: backend/internal/repository/pg_stats.go:1197
-- Target Table: N/A (configuration audit)
-- Description: Returns PostgreSQL configuration settings for key categories from pg_settings

SELECT
    name,
    setting,
    unit,
    category,
    source,
    boot_val,
    reset_val
FROM pg_settings
WHERE category IN ('Autovacuum', 'Client Connection Defaults', 'Connections and Authentication', 'Resource Usage', 'Write-Ahead Log')
ORDER BY category, name;
