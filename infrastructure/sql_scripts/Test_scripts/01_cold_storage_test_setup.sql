-- =============================================================================
-- Cold Storage Pipeline — Test Setup
-- Script 01: Register test servers and verify pre-conditions
--
-- Run this FIRST before any data seeding scripts.
-- Safe to re-run (all inserts are ON CONFLICT DO NOTHING).
--
-- Test UUIDs:
--   SQL Server test server : aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
--   PostgreSQL test server : bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb
-- =============================================================================

\echo '=== Cold Storage Test Setup ==='
\echo 'Registering test servers in optima_servers...'

-- Register SQL Server test server
INSERT INTO optima_servers (
    id, name, db_type, host, port, username,
    encrypted_secret, encrypted_dek, is_active
) VALUES (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'COLD-TEST-SQLSERVER',
    'sqlserver',
    '127.0.0.1',
    1433,
    'sa',
    '\x00',
    '\x00',
    TRUE
) ON CONFLICT (id) DO UPDATE SET
    name      = EXCLUDED.name,
    is_active = TRUE;

-- Register PostgreSQL test server
INSERT INTO optima_servers (
    id, name, db_type, host, port, username,
    encrypted_secret, encrypted_dek, is_active
) VALUES (
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
    'COLD-TEST-POSTGRES',
    'postgres',
    '127.0.0.1',
    5432,
    'postgres',
    '\x00',
    '\x00',
    TRUE
) ON CONFLICT (id) DO UPDATE SET
    name      = EXCLUDED.name,
    is_active = TRUE;

\echo 'Test servers registered.'

-- Verify servers are visible
SELECT
    id,
    name,
    db_type,
    is_active,
    created_at
FROM optima_servers
WHERE id IN (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
)
ORDER BY name;

-- Verify coldstorage schema exists
SELECT
    table_schema,
    table_name
FROM information_schema.tables
WHERE table_schema = 'coldstorage'
ORDER BY table_name;

-- Confirm no existing watermarks for test servers (fresh state)
SELECT
    COUNT(*) AS existing_watermarks,
    'If > 0, run 08_cold_storage_test_cleanup.sql first' AS note
FROM coldstorage.watermarks
WHERE server_id IN (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
);

-- Show current compression and retention policies for registered tables
\echo ''
\echo '=== Compression & Retention Policies for Cold Storage Tables ==='
SELECT
    hypertable_name,
    compress_after,
    policy_interval_type
FROM timescaledb_information.compression_settings
WHERE hypertable_name IN (
    'sqlserver_cpu_history', 'sqlserver_wait_history', 'sqlserver_metrics',
    'sqlserver_connection_history', 'sqlserver_lock_history', 'sqlserver_disk_history',
    'sqlserver_database_throughput', 'sqlserver_memory_metrics', 'sqlserver_buffer_pool_db',
    'sqlserver_cpu_scheduler_stats', 'sqlserver_memory_history', 'sqlserver_risk_health',
    'sqlserver_latch_waits', 'sqlserver_spinlock_stats', 'sqlserver_procedure_stats',
    'sqlserver_long_running_queries', 'sqlserver_memory_grant_waiters', 'sqlserver_waiting_tasks',
    'sqlserver_ag_health', 'postgres_wait_event_stats', 'postgres_db_io_stats',
    'postgres_settings_snapshot'
)
ORDER BY hypertable_name;

\echo ''
\echo '=== Setup complete. Proceed to 02_cold_storage_test_90day_data.sql ==='
