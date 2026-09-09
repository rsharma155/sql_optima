-- =============================================================================
-- Cold Storage Pipeline — Pre-Export Verification
-- Script 04: Verify data counts and time ranges before triggering cold export
--
-- Run after scripts 02 and 03. Confirms all tables have data in the expected
-- time windows and that chunks older than 7 days exist for compression.
-- =============================================================================

\echo '=== Pre-Export Data Verification ==='
\echo ''

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. Complete row count summary across all cold-storage registered tables
-- ─────────────────────────────────────────────────────────────────────────────
\echo '--- Row counts for all registered cold-storage tables ---'

SELECT
    table_name,
    total_rows,
    rows_archiveable,
    min_date,
    max_date,
    CASE
        WHEN total_rows = 0        THEN '❌ NO DATA'
        WHEN rows_archiveable = 0  THEN '⚠️ NO ARCHIVEABLE DATA (all < 2 days old)'
        ELSE '✅ READY'
    END AS status
FROM (
    -- Group A SQL Server tables
    SELECT 'sqlserver_cpu_history' AS table_name,
        COUNT(*) AS total_rows,
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days') AS rows_archiveable,
        MIN(capture_timestamp)::DATE AS min_date,
        MAX(capture_timestamp)::DATE AS max_date
    FROM sqlserver_cpu_history
    WHERE server_id IN (
        'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
        'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    )
    UNION ALL
    SELECT 'sqlserver_memory_history', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_memory_history
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_wait_history', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_wait_history
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_metrics', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_metrics
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_connection_history', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_connection_history
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_lock_history', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_lock_history
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_disk_history', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_disk_history
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_database_throughput', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_database_throughput
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_memory_metrics', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_memory_metrics
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_buffer_pool_db', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_buffer_pool_db
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_cpu_scheduler_stats', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_cpu_scheduler_stats
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_risk_health', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_risk_health
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    -- Group B SQL Server tables
    UNION ALL
    SELECT 'sqlserver_ag_health', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_ag_health
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_latch_waits', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_latch_waits
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_spinlock_stats', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_spinlock_stats
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_procedure_stats', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_procedure_stats
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_long_running_queries', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_long_running_queries
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_memory_grant_waiters', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_memory_grant_waiters
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    UNION ALL
    SELECT 'sqlserver_waiting_tasks', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM sqlserver_waiting_tasks
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    -- PostgreSQL tables
    UNION ALL
    SELECT 'postgres_settings_snapshot', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM postgres_settings_snapshot
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'postgres_wait_event_stats', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM postgres_wait_event_stats
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'postgres_db_io_stats', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM postgres_db_io_stats
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_backup_archiver_ts', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_backup_archiver_ts
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_basebackup_history', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_basebackup_history
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_roles_snapshot', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_roles_snapshot
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_failed_login_events', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_failed_login_events
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_session_activity_ts', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_session_activity_ts
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_wait_event_summary_ts', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_wait_event_summary_ts
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_db_load_ts', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_db_load_ts
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_query_wait_profile_ts', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_query_wait_profile_ts
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
    UNION ALL
    SELECT 'monitor.pg_ddl_activity_ts', COUNT(*),
        COUNT(*) FILTER (WHERE capture_timestamp < NOW() - INTERVAL '2 days'),
        MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
    FROM monitor.pg_ddl_activity_ts
    WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
) counts
ORDER BY table_name;

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. Chunk compression status — how many chunks are ready for export
-- ─────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '--- Chunk compression status (chunks > 7 days old) ---'

SELECT
    hypertable_schema || '.' || hypertable_name AS table_name,
    COUNT(*) FILTER (WHERE is_compressed = TRUE)  AS compressed_chunks,
    COUNT(*) FILTER (WHERE is_compressed = FALSE) AS uncompressed_chunks,
    COUNT(*)                                       AS total_chunks,
    round(
        100.0 * COUNT(*) FILTER (WHERE is_compressed = TRUE) / NULLIF(COUNT(*), 0),
        1
    ) AS pct_compressed
FROM timescaledb_information.chunks
WHERE range_end < NOW() - INTERVAL '7 days'
  AND hypertable_name IN (
    'sqlserver_cpu_history', 'sqlserver_memory_history', 'sqlserver_wait_history',
    'sqlserver_metrics', 'sqlserver_connection_history', 'sqlserver_lock_history',
    'sqlserver_disk_history', 'sqlserver_database_throughput', 'sqlserver_memory_metrics',
    'sqlserver_buffer_pool_db', 'sqlserver_cpu_scheduler_stats', 'sqlserver_risk_health',
    'sqlserver_ag_health', 'sqlserver_latch_waits', 'sqlserver_spinlock_stats',
    'sqlserver_procedure_stats', 'sqlserver_long_running_queries',
    'sqlserver_memory_grant_waiters', 'sqlserver_waiting_tasks',
    'postgres_settings_snapshot', 'postgres_wait_event_stats', 'postgres_db_io_stats',
    'pg_backup_archiver_ts', 'pg_basebackup_history', 'pg_roles_snapshot',
    'pg_failed_login_events', 'pg_session_activity_ts', 'pg_wait_event_summary_ts',
    'pg_db_load_ts', 'pg_query_wait_profile_ts', 'pg_ddl_activity_ts'
)
GROUP BY 1
ORDER BY uncompressed_chunks DESC, 1;

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. Confirm no existing watermarks for test servers (clean state)
-- ─────────────────────────────────────────────────────────────────────────────
\echo ''
\echo '--- coldstorage.watermarks for test servers ---'
\echo '(Should be empty before first export run)'

SELECT
    table_name,
    server_id,
    last_exported_at,
    updated_at,
    NOW() - last_exported_at AS age
FROM coldstorage.watermarks
WHERE server_id IN (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
)
ORDER BY table_name;

\echo ''
\echo '=== Pre-export verification complete ==='
\echo 'Next: run 05_cold_storage_force_compress.sql to compress aged chunks,'
\echo 'then trigger the cold storage exporter.'
