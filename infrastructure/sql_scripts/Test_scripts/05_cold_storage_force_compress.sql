-- =============================================================================
-- Cold Storage Pipeline — Force Compression of Aged Chunks
-- Script 05: Manually compress chunks older than 7 days
--
-- In production, TimescaleDB automatically compresses chunks after
-- add_compression_policy() triggers (default: 7 days). In testing, we force
-- compression immediately so the cold storage exporter's isCompressed()
-- gate does not block the export of test data.
--
-- This script is SAFE to run on production — it only compresses chunks
-- that already exceed the compression threshold. It does NOT change
-- what data is retained or exported.
-- =============================================================================

\echo '=== Forcing compression on aged chunks (> 7 days old) ==='
\echo 'This is safe: matches production compression policy threshold.'
\echo ''

DO $$
DECLARE
    r RECORD;
    v_compressed INT := 0;
    v_skipped    INT := 0;
    v_errors     INT := 0;
BEGIN
    -- Iterate over all uncompressed chunks older than 7 days across
    -- tables registered in the cold storage pipeline.
    FOR r IN
        SELECT
            chunk_schema,
            chunk_name,
            hypertable_schema,
            hypertable_name,
            range_end
        FROM timescaledb_information.chunks
        WHERE is_compressed = FALSE
          AND range_end < NOW() - INTERVAL '7 days'
          AND hypertable_name IN (
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
            'pg_backup_archiver_ts',
            'pg_basebackup_history',
            'pg_roles_snapshot',
            'pg_failed_login_events',
            'pg_session_activity_ts',
            'pg_wait_event_summary_ts',
            'pg_db_load_ts',
            'pg_query_wait_profile_ts',
            'pg_ddl_activity_ts'
          )
        ORDER BY hypertable_name, range_end
    LOOP
        BEGIN
            PERFORM compress_chunk(format('%I.%I', r.chunk_schema, r.chunk_name));
            v_compressed := v_compressed + 1;
            RAISE NOTICE 'Compressed: %.% (hypertable: %, end: %)',
                r.chunk_schema, r.chunk_name, r.hypertable_name, r.range_end::DATE;
        EXCEPTION WHEN OTHERS THEN
            v_errors := v_errors + 1;
            RAISE WARNING 'Could not compress %.%: %',
                r.chunk_schema, r.chunk_name, SQLERRM;
        END;
    END LOOP;

    RAISE NOTICE '';
    RAISE NOTICE '=== Compression summary ===';
    RAISE NOTICE 'Compressed: %  |  Errors: %', v_compressed, v_errors;
END $$;

\echo ''
\echo '--- Chunk compression status after force-compress ---'

SELECT
    hypertable_schema || '.' || hypertable_name AS table_name,
    COUNT(*) FILTER (WHERE is_compressed = TRUE)  AS compressed_chunks,
    COUNT(*) FILTER (WHERE is_compressed = FALSE) AS uncompressed_chunks,
    COUNT(*)                                       AS total_old_chunks,
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
HAVING COUNT(*) > 0
ORDER BY uncompressed_chunks DESC, 1;

\echo ''
\echo '=== Target: uncompressed_chunks = 0 for all tables listed ==='
\echo 'If any remain uncompressed, re-run this script or check for open transactions.'
\echo ''
\echo 'Next: trigger the cold storage exporter, then run 06_cold_storage_verify_watermarks.sql'
