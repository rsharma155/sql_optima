-- =============================================================================
-- Cold Storage Pipeline — 90-Day Data Seeding (Group A Tables)
-- Script 02: Insert SQL Server and PostgreSQL core metric history
--
-- Time window: 92 days ago → 32 days ago (deep archive tier)
-- Interval:    30 minutes (manageable volume; ~4,320 rows per single-row table)
-- Server:      aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa (SQL Server)
--              bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb (PostgreSQL)
--
-- All timestamps are > 7 days old → TimescaleDB will compress them.
-- All timestamps are > 2 days old → cold storage exporter will pick them up.
-- =============================================================================

\echo '=== Seeding 90-Day Archive Tier (Group A tables) ==='
\echo 'Window: 92 days ago → 32 days ago at 30-minute intervals'
\echo 'Estimated rows per single-row table: ~4,320'
\echo ''

DO $$
DECLARE
    v_ss_id   UUID    := 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
    v_pg_id   UUID    := 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb';
    v_from    TIMESTAMPTZ := NOW() - INTERVAL '92 days';
    v_to      TIMESTAMPTZ := NOW() - INTERVAL '32 days';
    v_step    INTERVAL    := INTERVAL '30 minutes';
    v_row_count BIGINT;
BEGIN
    RAISE NOTICE '==========================================================';
    RAISE NOTICE 'Window: % → %', v_from::DATE, v_to::DATE;
    RAISE NOTICE 'Step: %', v_step;
    RAISE NOTICE '==========================================================';

    -- -------------------------------------------------------------------------
    -- 1. sqlserver_cpu_history
    --    Priority: HIGH | Retention: 90 days | Cadence: 1 min (seeding 30 min)
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_cpu_history (
        capture_timestamp, server_id,
        sql_process, system_idle, other_process, scheduler_count
    )
    SELECT
        ts,
        v_ss_id,
        round((random() * 60 + 5)::NUMERIC, 2),   -- sql_process 5–65%
        round((random() * 30 + 20)::NUMERIC, 2),  -- system_idle 20–50%
        round((random() * 15)::NUMERIC, 2),        -- other_process 0–15%
        8 + (floor(random() * 4))::INT             -- scheduler_count 8–11
    FROM generate_series(v_from, v_to, v_step) ts
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_cpu_history: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 2. sqlserver_memory_history  (PLE – Page Life Expectancy)
    --    Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_memory_history (
        capture_timestamp, server_id, page_life_expectancy
    )
    SELECT
        ts,
        v_ss_id,
        round((random() * 4000 + 300)::NUMERIC, 0)  -- PLE 300–4300 seconds
    FROM generate_series(v_from, v_to, v_step) ts
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_memory_history: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 3. sqlserver_wait_history  (5 wait categories per timestamp)
    --    Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_wait_history (
        capture_timestamp, server_id, wait_type,
        wait_time_ms_total, disk_read_ms_per_sec,
        blocking_ms_per_sec, parallelism_ms_per_sec, other_ms_per_sec
    )
    SELECT
        ts,
        v_ss_id,
        wt.wait_type,
        round((random() * 500000 + 1000)::NUMERIC, 0)::BIGINT,
        round((random() * 200)::NUMERIC, 2),
        round((random() * 50)::NUMERIC, 2),
        round((random() * 100)::NUMERIC, 2),
        round((random() * 150)::NUMERIC, 2)
    FROM generate_series(v_from, v_to, v_step) ts
    CROSS JOIN (VALUES
        ('PAGEIOLATCH_SH'),
        ('SOS_SCHEDULER_YIELD'),
        ('WRITELOG'),
        ('CXPACKET'),
        ('LCK_M_X')
    ) wt(wait_type)
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_wait_history: % rows inserted (5 wait types × timestamps)', v_row_count;

    -- -------------------------------------------------------------------------
    -- 4. sqlserver_metrics  (core overview)
    --    Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_metrics (
        capture_timestamp, server_id,
        avg_cpu_load, memory_usage, active_users,
        total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
    )
    SELECT
        ts,
        v_ss_id,
        round((random() * 70 + 5)::NUMERIC, 1),
        round((random() * 40 + 50)::NUMERIC, 1),
        (random() * 150 + 10)::INT,
        (random() * 2000 + 100)::BIGINT,
        CASE WHEN random() < 0.02 THEN 1 ELSE 0 END::BIGINT,
        round((random() * 20000 + 50000)::NUMERIC, 0),
        round((random() * 5000 + 5000)::NUMERIC, 0),
        round((random() * 100000 + 20000)::NUMERIC, 0)
    FROM generate_series(v_from, v_to, v_step) ts
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_metrics: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 5. sqlserver_connection_history  (3 login/db combos per timestamp)
    --    Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_connection_history (
        capture_timestamp, server_id,
        login_name, database_name, active_connections, active_requests
    )
    SELECT
        ts,
        v_ss_id,
        conn.login_name,
        conn.db_name,
        (random() * 50 + 5)::INT,
        (random() * 10)::INT
    FROM generate_series(v_from, v_to, v_step) ts
    CROSS JOIN (VALUES
        ('app_user',  'ProductionDB'),
        ('sa',        'master'),
        ('report_ro', 'ReportingDB')
    ) conn(login_name, db_name)
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_connection_history: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 6. sqlserver_lock_history  (3 databases per timestamp)
    --    Priority: MEDIUM | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_lock_history (
        capture_timestamp, server_id,
        database_name, total_locks, deadlocks
    )
    SELECT
        ts,
        v_ss_id,
        db.name,
        (random() * 2000 + 100)::BIGINT,
        CASE WHEN random() < 0.03 THEN 1 ELSE 0 END::BIGINT
    FROM generate_series(v_from, v_to, v_step) ts
    CROSS JOIN (VALUES ('ProductionDB'), ('master'), ('ReportingDB')) db(name)
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_lock_history: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 7. sqlserver_disk_history  (3 databases × 5-min cadence)
    --    Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_disk_history (
        capture_timestamp, server_id,
        database_name, data_mb, log_mb, free_mb,
        delta_data_mb, delta_log_mb
    )
    SELECT
        ts,
        v_ss_id,
        db.name,
        round((db.base_data_mb + random() * 500)::NUMERIC, 1),
        round((db.base_log_mb  + random() * 200)::NUMERIC, 1),
        round((random() * 50000 + 10000)::NUMERIC, 1),
        round((random() * 10)::NUMERIC, 2),
        round((random() * 5)::NUMERIC, 2)
    FROM generate_series(v_from, v_to, v_step) ts
    CROSS JOIN (VALUES
        ('ProductionDB',  50000, 5000),
        ('ReportingDB',   20000, 2000),
        ('master',         5000,  500)
    ) db(name, base_data_mb, base_log_mb)
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_disk_history: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 8. sqlserver_database_throughput  (2 databases per timestamp)
    --    Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_database_throughput (
        capture_timestamp, server_id, database_name,
        user_seeks, user_scans, user_lookups, user_writes,
        total_reads, total_writes, tps, batch_requests_per_sec,
        reads, writes, bytes_read, bytes_written,
        read_latency_ms, write_latency_ms
    )
    SELECT
        ts,
        v_ss_id,
        db.name,
        (random() * 50000 + 1000)::BIGINT,
        (random() * 10000 + 500)::BIGINT,
        (random() * 5000 + 100)::BIGINT,
        (random() * 20000 + 500)::BIGINT,
        (random() * 60000 + 2000)::BIGINT,
        (random() * 20000 + 500)::BIGINT,
        round((random() * 2000 + 100)::NUMERIC, 1),
        round((random() * 3000 + 200)::NUMERIC, 1),
        (random() * 5000 + 100)::BIGINT,
        (random() * 2000 + 50)::BIGINT,
        (random() * 1000000000 + 10000000)::BIGINT,
        (random() * 500000000 + 5000000)::BIGINT,
        (random() * 15)::BIGINT,
        (random() * 8)::BIGINT
    FROM generate_series(v_from, v_to, v_step) ts
    CROSS JOIN (VALUES ('ProductionDB'), ('ReportingDB')) db(name)
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_database_throughput: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 9. sqlserver_memory_metrics
    --    Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_memory_metrics (
        capture_timestamp, server_id,
        sql_memory_used_mb, sql_memory_target_mb,
        os_total_memory_mb, os_available_memory_mb,
        process_physical_low, process_virtual_low,
        memory_grants_pending, active_memory_grants,
        waiting_memory_grants, granted_workspace_mb, requested_workspace_mb,
        ple_seconds, plan_cache_mb,
        sort_warnings_total, hash_warnings_total,
        sort_warnings_per_sec, hash_warnings_per_sec,
        os_system_memory_state, sql_physical_memory_in_use_mb,
        sql_memory_utilization_pct, sql_page_fault_count,
        sql_locked_page_alloc_mb, sql_large_page_alloc_mb
    )
    SELECT
        ts,
        v_ss_id,
        (random() * 32768 + 8192)::BIGINT,    -- sql_memory_used_mb
        (random() * 4096 + 32768)::BIGINT,    -- sql_memory_target_mb
        65536::BIGINT,                         -- os_total_memory_mb
        (random() * 8192 + 4096)::BIGINT,     -- os_available_memory_mb
        FALSE, FALSE,
        (random() * 3)::INT,                   -- memory_grants_pending
        (random() * 20 + 5)::INT,             -- active_memory_grants
        (random() * 2)::INT,                   -- waiting_memory_grants
        (random() * 2048 + 512)::BIGINT,      -- granted_workspace_mb
        (random() * 2048 + 512)::BIGINT,      -- requested_workspace_mb
        (random() * 4000 + 300)::BIGINT,      -- ple_seconds
        (random() * 4096 + 1024)::BIGINT,     -- plan_cache_mb
        (random() * 10000)::BIGINT,            -- sort_warnings_total
        (random() * 5000)::BIGINT,             -- hash_warnings_total
        round((random() * 2)::NUMERIC, 4),    -- sort_warnings_per_sec
        round((random() * 1)::NUMERIC, 4),    -- hash_warnings_per_sec
        'Available',
        (random() * 32768 + 8192)::BIGINT,   -- sql_physical_memory_in_use_mb
        (random() * 50 + 40)::INT,            -- sql_memory_utilization_pct
        (random() * 100000)::BIGINT,          -- sql_page_fault_count
        0::BIGINT, 0::BIGINT
    FROM generate_series(v_from, v_to, v_step) ts
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_memory_metrics: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 10. sqlserver_buffer_pool_db  (3 databases per timestamp)
    --     Priority: MEDIUM | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_buffer_pool_db (
        capture_timestamp, server_id, database_name, buffer_mb
    )
    SELECT
        ts,
        v_ss_id,
        db.name,
        (random() * db.base_mb + db.base_mb * 0.5)::BIGINT
    FROM generate_series(v_from, v_to, v_step) ts
    CROSS JOIN (VALUES
        ('ProductionDB', 10000),
        ('ReportingDB',   4000),
        ('master',         500)
    ) db(name, base_mb)
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_buffer_pool_db: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 11. sqlserver_cpu_scheduler_stats
    --     Priority: MEDIUM | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_cpu_scheduler_stats (
        capture_timestamp, server_id,
        max_workers_count, scheduler_count, cpu_count,
        total_runnable_tasks_count, total_work_queue_count,
        total_current_workers_count, active_workers_count,
        pending_disk_io_count, avg_runnable_tasks_count,
        total_active_request_count, total_queued_request_count,
        total_blocked_task_count, total_active_parallel_thread_count,
        runnable_request_count, total_request_count, runnable_percent,
        worker_thread_exhaustion_warning, runnable_tasks_warning,
        blocked_tasks_warning, queued_requests_warning,
        total_physical_memory_kb, available_physical_memory_kb,
        system_memory_state_desc, total_node_count, nodes_online_count,
        offline_cpu_count, offline_cpu_warning
    )
    SELECT
        ts,
        v_ss_id,
        1024, 8, 8,
        (random() * 5)::INT,
        (random() * 20)::BIGINT,
        (random() * 200 + 50)::INT,
        (random() * 150 + 40)::INT,
        (random() * 3)::INT,
        round((random() * 2)::NUMERIC, 2),
        (random() * 100 + 10)::INT,
        (random() * 5)::INT,
        (random() * 3)::INT,
        (random() * 200)::BIGINT,
        (random() * 5)::INT,
        (random() * 110 + 10)::INT,
        round((random() * 5)::NUMERIC, 2),
        FALSE, FALSE, FALSE, FALSE,
        65536000::BIGINT,
        (random() * 8388608 + 4194304)::BIGINT,
        'Available',
        1, 1, 0, FALSE
    FROM generate_series(v_from, v_to, v_step) ts
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_cpu_scheduler_stats: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 12. sqlserver_risk_health
    --     Priority: HIGH | Retention: 30 days (also seeding 90d window)
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_risk_health (
        capture_timestamp, server_id,
        blocking_sessions, memory_grants_pending, failed_logins_5m,
        tempdb_used_percent, max_log_db_name, max_log_used_percent,
        ple, compilations_per_sec, batch_requests_per_sec,
        buffer_cache_hit_ratio
    )
    SELECT
        ts,
        v_ss_id,
        (random() * 3)::INT,
        (random() * 2)::INT,
        (random() * 5)::INT,
        round((random() * 60 + 10)::NUMERIC, 1),
        'ProductionDB',
        round((random() * 70 + 10)::NUMERIC, 1),
        round((random() * 4000 + 300)::NUMERIC, 0),
        round((random() * 100 + 5)::NUMERIC, 2),
        round((random() * 3000 + 200)::NUMERIC, 2),
        round((random() * 10 + 89)::NUMERIC, 2)
    FROM generate_series(v_from, v_to, v_step) ts
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_risk_health: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 13. postgres_settings_snapshot  (10 key settings per snapshot, 6h cadence)
    --     Priority: LOW | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO postgres_settings_snapshot (
        capture_timestamp, server_id, name, setting, unit, source
    )
    SELECT
        ts,
        v_pg_id,
        s.name,
        s.setting,
        s.unit,
        'configuration file'
    FROM generate_series(v_from, v_to, INTERVAL '6 hours') ts
    CROSS JOIN (VALUES
        ('shared_buffers',            '2097152',  '8kB'),
        ('work_mem',                  '65536',    'kB'),
        ('max_connections',           '200',      NULL),
        ('wal_level',                 'replica',  NULL),
        ('checkpoint_completion_target', '0.9',   NULL),
        ('random_page_cost',          '1.1',      NULL),
        ('effective_cache_size',      '6291456',  '8kB'),
        ('max_wal_size',              '4096',     'MB'),
        ('log_min_duration_statement','5000',     'ms'),
        ('autovacuum',                'on',       NULL)
    ) s(name, setting, unit)
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'postgres_settings_snapshot: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 14. monitor.pg_backup_archiver_ts  (every 5 minutes)
    --     Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_backup_archiver_ts (
        capture_timestamp, server_id,
        archived_count, failed_count,
        last_archived_time, last_failed_time
    )
    SELECT
        ts,
        v_pg_id,
        (random() * 200 + 1000)::BIGINT + EXTRACT(EPOCH FROM (ts - v_from))::BIGINT / 300,
        (random() * 3)::BIGINT,
        ts - INTERVAL '5 minutes',
        CASE WHEN random() < 0.05 THEN ts - INTERVAL '1 hour' ELSE NULL END
    FROM generate_series(v_from, v_to, INTERVAL '5 minutes') ts
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_backup_archiver_ts: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 15. monitor.pg_basebackup_history  (one entry per day)
    --     Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_basebackup_history (
        capture_timestamp, server_id,
        checkpoint_time, checkpoint_write_time
    )
    SELECT
        ts,
        v_pg_id,
        ts - INTERVAL '30 seconds',
        round((random() * 120 + 30)::NUMERIC, 2)
    FROM generate_series(v_from, v_to, INTERVAL '1 day') ts
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_basebackup_history: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 16. monitor.pg_roles_snapshot  (6 roles per daily snapshot)
    --     Priority: LOW | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_roles_snapshot (
        capture_timestamp, server_id,
        rolname, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolcanlogin
    )
    SELECT
        ts,
        v_pg_id,
        r.rolname,
        r.rolsuper,
        r.rolcreatedb,
        r.rolcreaterole,
        r.rolreplication,
        r.rolcanlogin
    FROM generate_series(v_from, v_to, INTERVAL '1 day') ts
    CROSS JOIN (VALUES
        ('postgres',         TRUE,  TRUE,  TRUE,  TRUE,  TRUE),
        ('app_user',         FALSE, FALSE, FALSE, FALSE, TRUE),
        ('report_ro',        FALSE, FALSE, FALSE, FALSE, TRUE),
        ('replication_user', FALSE, FALSE, FALSE, TRUE,  TRUE),
        ('monitoring_user',  FALSE, FALSE, FALSE, FALSE, TRUE),
        ('backup_user',      FALSE, FALSE, FALSE, FALSE, TRUE)
    ) r(rolname, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolcanlogin)
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_roles_snapshot: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- 17. monitor.pg_failed_login_events  (random sparse events)
    --     Priority: HIGH | Retention: 90 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_failed_login_events (
        capture_timestamp, server_id,
        username, client_addr, message
    )
    SELECT
        ts,
        v_pg_id,
        CASE (random() * 3)::INT
            WHEN 0 THEN 'hacker_attempt'
            WHEN 1 THEN 'app_user'
            ELSE 'postgres'
        END,
        '192.168.' || (random() * 254 + 1)::INT || '.' || (random() * 254 + 1)::INT,
        'FATAL:  password authentication failed for user "test_user"'
    FROM generate_series(v_from, v_to, INTERVAL '4 hours') ts
    WHERE random() < 0.3  -- ~30% of 4h windows have a failed login event
    ON CONFLICT DO NOTHING;

    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_failed_login_events: % rows inserted', v_row_count;

    RAISE NOTICE '';
    RAISE NOTICE '=== 90-day seeding complete ===';

END $$;

\echo ''
\echo '=== Verification: Row counts for Group A tables ==='
SELECT
    'sqlserver_cpu_history'    AS table_name,
    COUNT(*) AS rows_inserted,
    MIN(capture_timestamp)::DATE AS min_date,
    MAX(capture_timestamp)::DATE AS max_date
FROM sqlserver_cpu_history
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_memory_history', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_memory_history
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_wait_history', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_wait_history
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_metrics', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_metrics
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_connection_history', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_connection_history
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_disk_history', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_disk_history
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_database_throughput', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_database_throughput
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_memory_metrics', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_memory_metrics
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_buffer_pool_db', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_buffer_pool_db
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_cpu_scheduler_stats', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_cpu_scheduler_stats
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_risk_health', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_risk_health
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'postgres_settings_snapshot', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM postgres_settings_snapshot
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_backup_archiver_ts', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_backup_archiver_ts
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_basebackup_history', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_basebackup_history
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_roles_snapshot', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_roles_snapshot
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_failed_login_events', COUNT(*),
       MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_failed_login_events
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
ORDER BY table_name;

\echo ''
\echo '=== Proceed to 03_cold_storage_test_30day_data.sql ==='
