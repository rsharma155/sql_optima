/*
================================================================================
ADDITIONAL POSTGRESQL BEST PRACTICE RULES — SQL Optima
================================================================================
Purpose : Adds missing PostgreSQL-specific best practice rules to the
          ruleengine.rules table.  Uses INSERT … ON CONFLICT DO UPDATE so the
          script is idempotent (safe to re-run after the base 02_rule_engine.sql
          has already been applied).

Run order : After 02_rule_engine.sql.

Author   : Ravi Sharma
Copyright: (c) 2026 Ravi Sharma
License  : MIT
================================================================================
*/

-- ---------------------------------------------------------------------------
-- Helper: upsert function avoids write errors on repeated runs.
-- ---------------------------------------------------------------------------
DO $block$
BEGIN
  -- Ensure modified_date column exists (older schema may lack it).
  IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema = 'ruleengine' AND table_name = 'rules' AND column_name = 'modified_date'
  ) THEN
      ALTER TABLE ruleengine.rules ADD COLUMN modified_date TIMESTAMPTZ DEFAULT NULL;
  END IF;
END $block$;

-- ---------------------------------------------------------------------------
-- PostgreSQL session / connection hygiene rules
-- ---------------------------------------------------------------------------
INSERT INTO ruleengine.rules (
    rule_id, rule_name, category, applies_to, severity, dashboard_placement,
    description, detection_sql, detection_sql_pg,
    evaluation_logic, expected_calc, recommended_value,
    fix_script, fix_script_pg,
    comparison_type, threshold_value, priority, target_db_type, is_enabled
) VALUES

-- 1. idle_in_transaction_session_timeout — protect against forgotten transactions
(
    'PG_IDLE_TX_TIMEOUT_021',
    'Idle-in-Transaction Session Timeout',
    'Connection', 'Instance', 'Warning', 'BestPractice',
    'Sessions stuck "idle in transaction" hold locks and prevent VACUUM. Set a timeout (e.g. 5 min).',
    NULL,
    'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''idle_in_transaction_session_timeout'';',
    'setting == 0 ? "Warning" : "OK"',
    '300000',
    '300000 (5 min)',
    NULL,
    'ALTER SYSTEM SET idle_in_transaction_session_timeout = ''5min''; SELECT pg_reload_conf();',
    'exact', '{"value":0,"invert":true}', 21, 'postgres', TRUE
),

-- 2. lock_timeout — stop runaway lock-waiters
(
    'PG_LOCK_TIMEOUT_022',
    'Lock Timeout',
    'Connection', 'Instance', 'Warning', 'BestPractice',
    'Without a lock timeout a session can wait forever. Recommended: 30–60 s.',
    NULL,
    'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''lock_timeout'';',
    'setting == 0 ? "Warning" : "OK"',
    '30000',
    '30000 (30 s)',
    NULL,
    'ALTER SYSTEM SET lock_timeout = ''30s''; SELECT pg_reload_conf();',
    'exact', '{"value":0,"invert":true}', 22, 'postgres', TRUE
),

-- 3. log_min_duration_statement — catch slow queries automatically
(
    'PG_LOG_SLOW_QUERY_023',
    'Log Slow Queries (log_min_duration_statement)',
    'Observability', 'Instance', 'Warning', 'BestPractice',
    'Slow query logging is disabled (-1). Recommended: 1000–5000 ms to surface regressions.',
    NULL,
    'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''log_min_duration_statement'';',
    'setting == -1 ? "Warning" : "OK"',
    '1000',
    '1000 (1 s)',
    NULL,
    'ALTER SYSTEM SET log_min_duration_statement = ''1000ms''; SELECT pg_reload_conf();',
    'exact', '{"value":-1,"invert":true}', 23, 'postgres', TRUE
),

-- 4. log_lock_waits — surface lock contention automatically
(
    'PG_LOG_LOCK_WAITS_024',
    'Log Lock Waits (log_lock_waits)',
    'Observability', 'Instance', 'Warning', 'BestPractice',
    'Lock-wait events are not logged. Enable log_lock_waits to surface blocking in logs.',
    NULL,
    'SELECT setting AS setting FROM pg_settings WHERE name = ''log_lock_waits'';',
    'setting == ''on'' ? "OK" : "Warning"',
    'on',
    'on',
    NULL,
    'ALTER SYSTEM SET log_lock_waits = on; SELECT pg_reload_conf();',
    'exact', '{"value":"on"}', 24, 'postgres', TRUE
),

-- 5. track_io_timing — required for I/O breakdown in pg_stat_statements
(
    'PG_TRACK_IO_TIMING_025',
    'Track I/O Timing (track_io_timing)',
    'Observability', 'Instance', 'Warning', 'BestPractice',
    'track_io_timing is off. Enable it to get I/O latency data in pg_stat_statements.',
    NULL,
    'SELECT setting AS setting FROM pg_settings WHERE name = ''track_io_timing'';',
    'setting == ''on'' ? "OK" : "Warning"',
    'on',
    'on',
    NULL,
    'ALTER SYSTEM SET track_io_timing = on; SELECT pg_reload_conf();',
    'exact', '{"value":"on"}', 25, 'postgres', TRUE
),

-- 6. wal_level — must be at least replica for logical replication / PITR
(
    'PG_WAL_LEVEL_026',
    'WAL Level',
    'High Availability', 'Instance', 'Critical', 'BestPractice',
    'wal_level=minimal disables replication and PITR. Recommended: replica or logical.',
    NULL,
    'SELECT setting AS setting FROM pg_settings WHERE name = ''wal_level'';',
    'setting == ''minimal'' ? "Critical" : "OK"',
    'replica',
    'replica or logical',
    NULL,
    'ALTER SYSTEM SET wal_level = ''replica''; -- requires restart',
    'exact', '{"value":"minimal","invert":true}', 26, 'postgres', TRUE
),

-- 7. archive_mode — needed for WAL archiving / PITR
(
    'PG_ARCHIVE_MODE_027',
    'WAL Archive Mode (archive_mode)',
    'Backup', 'Instance', 'Warning', 'BestPractice',
    'archive_mode is off. Enable for point-in-time recovery (PITR) capability.',
    NULL,
    'SELECT setting AS setting FROM pg_settings WHERE name = ''archive_mode'';',
    'setting == ''off'' ? "Warning" : "OK"',
    'on',
    'on',
    NULL,
    'ALTER SYSTEM SET archive_mode = on; ALTER SYSTEM SET archive_command = ''/path/to/archive %f %p''; -- requires restart',
    'exact', '{"value":"off","invert":true}', 27, 'postgres', TRUE
),

-- 8. track_counts — must be on for autovacuum to work correctly
(
    'PG_TRACK_COUNTS_028',
    'Track Row Counts (track_counts)',
    'Maintenance', 'Instance', 'Critical', 'BestPractice',
    'track_counts is off — autovacuum cannot make decisions without live/dead row counts.',
    NULL,
    'SELECT setting AS setting FROM pg_settings WHERE name = ''track_counts'';',
    'setting == ''on'' ? "OK" : "Critical"',
    'on',
    'on',
    NULL,
    'ALTER SYSTEM SET track_counts = on; SELECT pg_reload_conf();',
    'exact', '{"value":"on"}', 28, 'postgres', TRUE
),

-- 9. default_statistics_target — low value (100) yields poor query plans on large tables
(
    'PG_STATS_TARGET_029',
    'Default Statistics Target',
    'Performance', 'Instance', 'Warning', 'BestPractice',
    'default_statistics_target=100 is the default. Increase to 200–500 for better query plans on large tables.',
    NULL,
    'SELECT setting::int AS setting FROM pg_settings WHERE name = ''default_statistics_target'';',
    'setting < 200 ? "Warning" : "OK"',
    '200',
    '200',
    NULL,
    'ALTER SYSTEM SET default_statistics_target = 200; SELECT pg_reload_conf(); ANALYZE;',
    'threshold', '{"min":200}', 29, 'postgres', TRUE
),

-- 10. effective_io_concurrency — SSD optimisation (default 1 is for HDD)
(
    'PG_EFFECTIVE_IO_CONCURRENCY_030',
    'Effective I/O Concurrency',
    'Performance', 'Instance', 'Warning', 'BestPractice',
    'effective_io_concurrency=1 is tuned for HDDs. Set to 100–200 for SSDs/NVMe.',
    NULL,
    'SELECT setting::int AS setting FROM pg_settings WHERE name = ''effective_io_concurrency'';',
    'setting < 100 ? "Warning" : "OK"',
    '100',
    '100',
    NULL,
    'ALTER SYSTEM SET effective_io_concurrency = 100; SELECT pg_reload_conf();',
    'threshold', '{"min":100}', 30, 'postgres', TRUE
),

-- 11. Unused indexes — find indexes that have never been scanned
(
    'PG_UNUSED_INDEXES_031',
    'Unused Indexes',
    'Performance', 'Database', 'Warning', 'BestPractice',
    'Indexes with zero scans waste storage and slow down writes.',
    NULL,
    'SELECT COUNT(*) AS cnt FROM pg_stat_user_indexes WHERE idx_scan = 0 AND schemaname NOT IN (''pg_catalog'',''information_schema'');',
    'cnt == 0 ? "OK" : "Warning"',
    '0',
    '0',
    NULL,
    '-- Review with: SELECT schemaname, relname, indexrelname, pg_size_pretty(pg_relation_size(indexrelid)) FROM pg_stat_user_indexes WHERE idx_scan = 0 ORDER BY pg_relation_size(indexrelid) DESC;',
    'threshold', '{"max":0}', 31, 'postgres', TRUE
),

-- 12. Tables without a primary key — can cause logical replication issues and bloat
(
    'PG_NO_PK_TABLES_032',
    'Tables Without Primary Key',
    'Schema', 'Database', 'Warning', 'BestPractice',
    'Tables without a primary key cannot use logical replication row identity and are harder to maintain.',
    NULL,
    E'SELECT COUNT(*) AS cnt FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = ''r'' AND n.nspname NOT IN (''pg_catalog'',''information_schema'') AND NOT EXISTS ( SELECT 1 FROM pg_constraint con WHERE con.conrelid = c.oid AND con.contype = ''p'');',
    'cnt == 0 ? "OK" : "Warning"',
    '0',
    '0',
    NULL,
    '-- List with: SELECT n.nspname, c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = ''r'' AND n.nspname NOT IN (''pg_catalog'',''information_schema'') AND NOT EXISTS (SELECT 1 FROM pg_constraint con WHERE con.conrelid = c.oid AND con.contype = ''p'');',
    'threshold', '{"max":0}', 32, 'postgres', TRUE
),

-- 13. Bloated sequences — sequences near exhaustion are a P0 production incident
(
    'PG_SEQUENCE_EXHAUSTION_033',
    'Sequence Exhaustion Risk',
    'Schema', 'Database', 'Critical', 'BestPractice',
    'Sequences that have consumed ≥ 80 % of their range will soon fail with integer overflow.',
    NULL,
    E'SELECT COUNT(*) AS cnt FROM ( SELECT seqrelid, (last_value::numeric - minimum_value::numeric + 1) / (maximum_value::numeric - minimum_value::numeric + 1) * 100 AS used_pct FROM pg_sequences, LATERAL (SELECT last_value, minimum_value, maximum_value FROM pg_sequences WHERE schemaname || ''.'' || sequencename = schemaname || ''.'' || sequencename) sub WHERE (last_value::numeric - minimum_value::numeric + 1) / (maximum_value::numeric - minimum_value::numeric + 1) * 100 >= 80) sq;',
    'cnt == 0 ? "OK" : "Critical"',
    '0',
    '0',
    NULL,
    '-- Find near-exhausted sequences: SELECT schemaname, sequencename, last_value, maximum_value FROM pg_sequences WHERE (last_value::numeric / NULLIF(maximum_value::numeric,0)) >= 0.8 ORDER BY last_value::numeric / NULLIF(maximum_value::numeric,0) DESC;',
    'threshold', '{"max":0}', 33, 'postgres', TRUE
),

-- 14. checkpoint_warning — if checkpoints happen too often, max_wal_size is too small
(
    'PG_CHKPT_WARNING_034',
    'Checkpoint Frequency (max_wal_size)',
    'Performance', 'Instance', 'Warning', 'BestPractice',
    'If pg_log shows "checkpoints occurring too frequently" increase max_wal_size.',
    NULL,
    'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''max_wal_size'';',
    'setting < 2048 ? "Warning" : "OK"',
    '2048',
    '2048 MB',
    NULL,
    'ALTER SYSTEM SET max_wal_size = ''2GB''; SELECT pg_reload_conf();',
    'threshold', '{"min":2048}', 34, 'postgres', TRUE
),

-- 15. logging_collector — required for log_min_duration_statement to write files
(
    'PG_LOGGING_COLLECTOR_035',
    'Logging Collector Enabled',
    'Observability', 'Instance', 'Warning', 'BestPractice',
    'logging_collector is off. Without it, CSV/text log files are not written on most platforms.',
    NULL,
    'SELECT setting AS setting FROM pg_settings WHERE name = ''logging_collector'';',
    'setting == ''on'' ? "OK" : "Warning"',
    'on',
    'on',
    NULL,
    'ALTER SYSTEM SET logging_collector = on; -- requires restart',
    'exact', '{"value":"on"}', 35, 'postgres', TRUE
)

ON CONFLICT (rule_id) DO UPDATE SET
    rule_name          = EXCLUDED.rule_name,
    category           = EXCLUDED.category,
    applies_to         = EXCLUDED.applies_to,
    severity           = EXCLUDED.severity,
    dashboard_placement= EXCLUDED.dashboard_placement,
    description        = EXCLUDED.description,
    detection_sql      = EXCLUDED.detection_sql,
    detection_sql_pg   = EXCLUDED.detection_sql_pg,
    evaluation_logic   = EXCLUDED.evaluation_logic,
    expected_calc      = EXCLUDED.expected_calc,
    recommended_value  = EXCLUDED.recommended_value,
    fix_script         = EXCLUDED.fix_script,
    fix_script_pg      = EXCLUDED.fix_script_pg,
    comparison_type    = EXCLUDED.comparison_type,
    threshold_value    = EXCLUDED.threshold_value,
    priority           = EXCLUDED.priority,
    target_db_type     = EXCLUDED.target_db_type,
    is_enabled         = EXCLUDED.is_enabled,
    modified_date      = CURRENT_TIMESTAMP;

-- ---------------------------------------------------------------------------
-- Verification
-- ---------------------------------------------------------------------------
DO $$
DECLARE
    v_new_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_new_count
    FROM ruleengine.rules
    WHERE target_db_type = 'postgres'
      AND rule_id LIKE 'PG_%'
      AND priority >= 21;

    RAISE NOTICE '=================================================';
    RAISE NOTICE 'Additional PostgreSQL rules applied: %', v_new_count;
    RAISE NOTICE '=================================================';
END $$;

-- Show summary of all PostgreSQL rules
SELECT rule_id, rule_name, category, severity, dashboard_placement, is_enabled
FROM ruleengine.rules
WHERE target_db_type = 'postgres'
ORDER BY priority, rule_id;
