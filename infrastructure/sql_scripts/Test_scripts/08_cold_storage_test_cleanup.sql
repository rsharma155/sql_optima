-- =============================================================================
-- Cold Storage Pipeline — Test Cleanup
-- Script 08: Remove all test data for the two test server UUIDs
--
-- SAFE: Only removes rows WHERE server_id IN (test UUIDs).
-- Run this after validating the cold storage pipeline, or to reset
-- and re-run the seeding scripts from scratch.
--
-- Does NOT delete uploaded Parquet files from MinIO — use the MinIO console
-- or `mc rm --recursive local/sql-optima-cold/` to clear the bucket.
-- =============================================================================

\echo '=== Cold Storage Test Cleanup ==='
\echo 'Removing test data for UUIDs:'
\echo '  SQL Server: aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
\echo '  PostgreSQL: bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
\echo ''
\echo 'This will NOT delete MinIO Parquet files.'
\echo ''

DO $$
DECLARE
    v_ss_id UUID := 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
    v_pg_id UUID := 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb';
    v_deleted BIGINT;
BEGIN

    -- coldstorage watermarks
    DELETE FROM coldstorage.watermarks WHERE server_id IN (v_ss_id, v_pg_id);
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'coldstorage.watermarks: % rows deleted', v_deleted;

    -- Group A SQL Server tables
    DELETE FROM sqlserver_cpu_history WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_cpu_history: % rows deleted', v_deleted;

    DELETE FROM sqlserver_memory_history WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_memory_history: % rows deleted', v_deleted;

    DELETE FROM sqlserver_wait_history WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_wait_history: % rows deleted', v_deleted;

    DELETE FROM sqlserver_metrics WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_metrics: % rows deleted', v_deleted;

    DELETE FROM sqlserver_connection_history WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_connection_history: % rows deleted', v_deleted;

    DELETE FROM sqlserver_lock_history WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_lock_history: % rows deleted', v_deleted;

    DELETE FROM sqlserver_disk_history WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_disk_history: % rows deleted', v_deleted;

    DELETE FROM sqlserver_database_throughput WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_database_throughput: % rows deleted', v_deleted;

    DELETE FROM sqlserver_memory_metrics WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_memory_metrics: % rows deleted', v_deleted;

    DELETE FROM sqlserver_buffer_pool_db WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_buffer_pool_db: % rows deleted', v_deleted;

    DELETE FROM sqlserver_cpu_scheduler_stats WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_cpu_scheduler_stats: % rows deleted', v_deleted;

    DELETE FROM sqlserver_risk_health WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_risk_health: % rows deleted', v_deleted;

    -- Group B SQL Server tables
    DELETE FROM sqlserver_ag_health WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_ag_health: % rows deleted', v_deleted;

    DELETE FROM sqlserver_latch_waits WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_latch_waits: % rows deleted', v_deleted;

    DELETE FROM sqlserver_spinlock_stats WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_spinlock_stats: % rows deleted', v_deleted;

    DELETE FROM sqlserver_procedure_stats WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_procedure_stats: % rows deleted', v_deleted;

    DELETE FROM sqlserver_long_running_queries WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_long_running_queries: % rows deleted', v_deleted;

    DELETE FROM sqlserver_memory_grant_waiters WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_memory_grant_waiters: % rows deleted', v_deleted;

    DELETE FROM sqlserver_waiting_tasks WHERE server_id = v_ss_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'sqlserver_waiting_tasks: % rows deleted', v_deleted;

    -- PostgreSQL tables
    DELETE FROM postgres_settings_snapshot WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'postgres_settings_snapshot: % rows deleted', v_deleted;

    DELETE FROM postgres_wait_event_stats WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'postgres_wait_event_stats: % rows deleted', v_deleted;

    DELETE FROM postgres_db_io_stats WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'postgres_db_io_stats: % rows deleted', v_deleted;

    -- monitor schema tables
    DELETE FROM monitor.pg_backup_archiver_ts WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_backup_archiver_ts: % rows deleted', v_deleted;

    DELETE FROM monitor.pg_basebackup_history WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_basebackup_history: % rows deleted', v_deleted;

    DELETE FROM monitor.pg_roles_snapshot WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_roles_snapshot: % rows deleted', v_deleted;

    DELETE FROM monitor.pg_failed_login_events WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_failed_login_events: % rows deleted', v_deleted;

    DELETE FROM monitor.pg_session_activity_ts WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_session_activity_ts: % rows deleted', v_deleted;

    DELETE FROM monitor.pg_wait_event_summary_ts WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_wait_event_summary_ts: % rows deleted', v_deleted;

    DELETE FROM monitor.pg_db_load_ts WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_db_load_ts: % rows deleted', v_deleted;

    DELETE FROM monitor.pg_query_wait_profile_ts WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_query_wait_profile_ts: % rows deleted', v_deleted;

    DELETE FROM monitor.pg_ddl_activity_ts WHERE server_id = v_pg_id;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_ddl_activity_ts: % rows deleted', v_deleted;

    -- Remove test server registrations
    DELETE FROM optima_servers WHERE id IN (v_ss_id, v_pg_id);
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE 'optima_servers: % test server rows deleted', v_deleted;

    RAISE NOTICE '';
    RAISE NOTICE '=== Cleanup complete ===';
    RAISE NOTICE 'Note: Parquet files in MinIO are NOT deleted by this script.';
    RAISE NOTICE 'To clear MinIO: docker exec minio mc rm --recursive --force local/sql-optima-cold/';

END $$;

-- Confirm zero rows remain for test UUIDs
\echo ''
\echo '--- Confirmation: test server rows in optima_servers ---'
SELECT COUNT(*) AS remaining_rows,
    CASE WHEN COUNT(*) = 0 THEN '✅ Clean' ELSE '❌ Rows remain' END AS status
FROM optima_servers
WHERE id IN (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
);

\echo ''
\echo '--- Confirmation: watermarks removed ---'
SELECT COUNT(*) AS remaining_watermarks,
    CASE WHEN COUNT(*) = 0 THEN '✅ Clean' ELSE '❌ Watermarks remain' END AS status
FROM coldstorage.watermarks
WHERE server_id IN (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
);

\echo ''
\echo '=== To clear MinIO Parquet files (optional): ==='
\echo 'docker exec <minio-container> mc rm --recursive --force local/sql-optima-cold/metrics/'
