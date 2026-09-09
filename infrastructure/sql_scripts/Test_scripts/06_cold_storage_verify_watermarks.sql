-- =============================================================================
-- Cold Storage Pipeline — Post-Export Watermark Verification
-- Script 06: Confirm watermarks advanced and run completed successfully
--
-- Run this AFTER the cold storage exporter has completed a cycle.
-- Checks watermarks, run audit log, and per-table progress.
-- =============================================================================

\echo '=== Post-Export Watermark Verification ==='
\echo ''

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. Export run summary (most recent runs)
-- ─────────────────────────────────────────────────────────────────────────────
\echo '--- Recent cold export runs ---'

SELECT
    cold_export_run_id,
    run_started::TIMESTAMP(0)          AS started,
    run_finished::TIMESTAMP(0)         AS finished,
    EXTRACT(EPOCH FROM (run_finished - run_started))::INT || 's' AS duration,
    status,
    tables_ok,
    tables_failed,
    to_char(total_rows, 'FM999,999,999')   AS total_rows,
    pg_size_pretty(total_bytes::BIGINT)    AS total_size,
    error_detail
FROM coldstorage.runs
ORDER BY run_started DESC
LIMIT 10;

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. Watermark status for test servers
-- ─────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '--- Watermarks for test servers ---'

SELECT
    w.table_name,
    w.server_id,
    s.name                                          AS server_name,
    w.last_exported_at::TIMESTAMP(0)                AS last_exported_at,
    (NOW() - w.last_exported_at)::INTERVAL(0)       AS age,
    CASE
        WHEN w.last_exported_at >= NOW() - INTERVAL '3 days'
            THEN '✅ CURRENT'
        WHEN w.last_exported_at >= NOW() - INTERVAL '7 days'
            THEN '⚠️ SLIGHTLY BEHIND'
        ELSE '❌ STALE'
    END AS watermark_status,
    w.updated_at::TIMESTAMP(0)                      AS watermark_updated
FROM coldstorage.watermarks w
LEFT JOIN optima_servers s ON s.id = w.server_id
WHERE w.server_id IN (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
)
ORDER BY w.table_name, w.server_id;

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. Tables without any watermark (not yet exported)
-- ─────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '--- Tables with NO watermark for test servers (not yet exported) ---'

WITH registered AS (
    SELECT unnest(ARRAY[
        'sqlserver_cpu_history',
        'sqlserver_memory_history',
        'sqlserver_wait_history',
        'sqlserver_metrics',
        'sqlserver_connection_history',
        'sqlserver_lock_history',
        'sqlserver_disk_history',
        'sqlserver_database_throughput',
        'sqlserver_memory_metrics',
        'sqlserver_buffer_pool_db',
        'sqlserver_cpu_scheduler_stats',
        'sqlserver_risk_health',
        'sqlserver_ag_health',
        'sqlserver_latch_waits',
        'sqlserver_spinlock_stats',
        'sqlserver_procedure_stats',
        'sqlserver_long_running_queries',
        'sqlserver_memory_grant_waiters',
        'sqlserver_waiting_tasks',
        'postgres_settings_snapshot',
        'postgres_wait_event_stats',
        'postgres_db_io_stats',
        'monitor.pg_backup_archiver_ts',
        'monitor.pg_basebackup_history',
        'monitor.pg_roles_snapshot',
        'monitor.pg_failed_login_events',
        'monitor.pg_session_activity_ts',
        'monitor.pg_wait_event_summary_ts',
        'monitor.pg_db_load_ts',
        'monitor.pg_query_wait_profile_ts',
        'monitor.pg_ddl_activity_ts'
    ]) AS table_name
)
SELECT
    r.table_name,
    'NOT EXPORTED' AS status
FROM registered r
WHERE NOT EXISTS (
    SELECT 1 FROM coldstorage.watermarks w
    WHERE w.table_name = r.table_name
      AND w.server_id IN (
        'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
        'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
      )
)
ORDER BY r.table_name;

-- ─────────────────────────────────────────────────────────────────────────────
-- 4. Data gaps: rows in hot tier that should have been exported but weren't
-- ─────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '--- Rows in hot tier that predate the watermark (should have been exported) ---'

SELECT
    'sqlserver_cpu_history' AS table_name,
    w.last_exported_at,
    COUNT(*) AS rows_before_watermark,
    CASE WHEN COUNT(*) > 0 THEN '⚠️ EXPORT GAP' ELSE '✅ OK' END AS status
FROM sqlserver_cpu_history h
JOIN coldstorage.watermarks w
    ON w.table_name = 'sqlserver_cpu_history'
   AND w.server_id = h.server_id
WHERE h.server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
  AND h.capture_timestamp < w.last_exported_at
GROUP BY w.last_exported_at

UNION ALL

SELECT
    'sqlserver_wait_history',
    w.last_exported_at,
    COUNT(*),
    CASE WHEN COUNT(*) > 0 THEN '⚠️ EXPORT GAP' ELSE '✅ OK' END
FROM sqlserver_wait_history h
JOIN coldstorage.watermarks w
    ON w.table_name = 'sqlserver_wait_history'
   AND w.server_id = h.server_id
WHERE h.server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
  AND h.capture_timestamp < w.last_exported_at
GROUP BY w.last_exported_at

UNION ALL

SELECT
    'sqlserver_disk_history',
    w.last_exported_at,
    COUNT(*),
    CASE WHEN COUNT(*) > 0 THEN '⚠️ EXPORT GAP' ELSE '✅ OK' END
FROM sqlserver_disk_history h
JOIN coldstorage.watermarks w
    ON w.table_name = 'sqlserver_disk_history'
   AND w.server_id = h.server_id
WHERE h.server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
  AND h.capture_timestamp < w.last_exported_at
GROUP BY w.last_exported_at
ORDER BY 1;

-- ─────────────────────────────────────────────────────────────────────────────
-- 5. coldstorage.status_view  (production view showing all servers)
-- ─────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '--- coldstorage.status_view (all servers, ordered by staleness) ---'

SELECT *
FROM coldstorage.status_view
ORDER BY age DESC NULLS FIRST
LIMIT 20;

\echo ''
\echo '=== Post-export verification complete ==='
\echo ''
\echo 'Expected pass criteria:'
\echo '  - coldstorage.runs: status = success or partial (not failed)'
\echo '  - All watermarks updated within the last 24 hours'
\echo '  - No tables in "NOT EXPORTED" list (or only tables with no test data)'
\echo '  - No rows_before_watermark > 0 (export gap check)'
\echo ''
\echo 'Next: run 07_cold_storage_duckdb_validation.sql to validate Parquet files in MinIO.'
