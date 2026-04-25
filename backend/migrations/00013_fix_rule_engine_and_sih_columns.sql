-- Fix for evaluation errors in replication lag rules
UPDATE ruleengine.rules
SET detection_sql = 'SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0)::BIGINT AS lag_bytes FROM pg_stat_replication LIMIT 1;',
    detection_sql_pg = 'SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0)::BIGINT AS lag_bytes FROM pg_stat_replication LIMIT 1;'
WHERE rule_id IN ('PG_REPL_LAG_001', 'PG_REPLICATION_LAG_019');

-- Ensure SIH state tables have required size columns (addressing "column does not exist" errors)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'monitor' AND table_name = 'index_usage_state' AND column_name = 'index_size_mb') THEN
        ALTER TABLE monitor.index_usage_state ADD COLUMN index_size_mb NUMERIC;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'monitor' AND table_name = 'table_usage_state' AND column_name = 'table_size_mb') THEN
        ALTER TABLE monitor.table_usage_state ADD COLUMN table_size_mb NUMERIC;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'monitor' AND table_name = 'table_usage_state' AND column_name = 'index_size_mb') THEN
        ALTER TABLE monitor.table_usage_state ADD COLUMN index_size_mb NUMERIC;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'monitor' AND table_name = 'table_usage_state' AND column_name = 'row_count') THEN
        ALTER TABLE monitor.table_usage_state ADD COLUMN row_count BIGINT;
    END IF;
END $$;
