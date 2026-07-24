-- =============================================================================
-- Cold Storage Pipeline — 30-Day Data Seeding (Group B Tables + 60-day top-up)
-- Script 03: Insert Group B tables and add higher-frequency data for recent window
--
-- Two windows:
--   Deep tier:   62 days ago → 32 days ago at 30-minute intervals (Group B catch-up)
--   Recent tier: 32 days ago → 3 days ago at 15-minute intervals (higher frequency)
--
-- Group B tables: 30-day hot retention; important for compliance / forensic archives
-- Also adds higher-frequency data on Group A tables for the 30-day window.
-- =============================================================================

\echo '=== Seeding 30-Day Archive Tier (Group B tables + high-frequency top-up) ==='

DO $$
DECLARE
    v_ss_id   UUID    := 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
    v_pg_id   UUID    := 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb';
    -- Deep tier: 62d → 32d (catch-up for Group B tables)
    v_from_deep TIMESTAMPTZ := NOW() - INTERVAL '62 days';
    v_to_deep   TIMESTAMPTZ := NOW() - INTERVAL '32 days';
    -- Recent tier: 32d → 3d  (within hot retention, will be archived soon)
    v_from_recent TIMESTAMPTZ := NOW() - INTERVAL '32 days';
    v_to_recent   TIMESTAMPTZ := NOW() - INTERVAL '3 days';
    v_step_deep   INTERVAL := INTERVAL '30 minutes';
    v_step_recent INTERVAL := INTERVAL '15 minutes';
    v_row_count BIGINT;
BEGIN
    RAISE NOTICE '==========================================================';
    RAISE NOTICE 'Deep tier:   % → %', v_from_deep::DATE, v_to_deep::DATE;
    RAISE NOTICE 'Recent tier: % → %', v_from_recent::DATE, v_to_recent::DATE;
    RAISE NOTICE '==========================================================';

    -- -------------------------------------------------------------------------
    -- Group A top-up: add recent 32d→3d data at 15-min cadence for key tables
    -- -------------------------------------------------------------------------

    -- sqlserver_cpu_history (recent)
    INSERT INTO sqlserver_cpu_history (
        capture_timestamp, server_id,
        sql_process, system_idle, other_process, scheduler_count
    )
    SELECT ts, v_ss_id,
        round((random() * 70 + 5)::NUMERIC, 2),
        round((random() * 30 + 15)::NUMERIC, 2),
        round((random() * 20)::NUMERIC, 2),
        8 + (floor(random() * 4))::INT
    FROM generate_series(v_from_recent, v_to_recent, v_step_recent) ts
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_cpu_history (recent 32d): % rows', v_row_count;

    -- sqlserver_memory_history (recent)
    INSERT INTO sqlserver_memory_history (capture_timestamp, server_id, page_life_expectancy)
    SELECT ts, v_ss_id, round((random() * 4000 + 300)::NUMERIC, 0)
    FROM generate_series(v_from_recent, v_to_recent, v_step_recent) ts
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_memory_history (recent 32d): % rows', v_row_count;

    -- sqlserver_wait_history (recent – 5 wait types)
    INSERT INTO sqlserver_wait_history (
        capture_timestamp, server_id, wait_type,
        wait_time_ms_total, disk_read_ms_per_sec,
        blocking_ms_per_sec, parallelism_ms_per_sec, other_ms_per_sec
    )
    SELECT ts, v_ss_id, wt.wait_type,
        round((random() * 600000 + 1000)::NUMERIC, 0)::BIGINT,
        round((random() * 250)::NUMERIC, 2),
        round((random() * 80)::NUMERIC, 2),
        round((random() * 120)::NUMERIC, 2),
        round((random() * 200)::NUMERIC, 2)
    FROM generate_series(v_from_recent, v_to_recent, v_step_recent) ts
    CROSS JOIN (VALUES
        ('PAGEIOLATCH_SH'), ('SOS_SCHEDULER_YIELD'), ('WRITELOG'),
        ('CXPACKET'), ('LCK_M_X')
    ) wt(wait_type)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_wait_history (recent 32d): % rows', v_row_count;

    -- sqlserver_metrics (recent)
    INSERT INTO sqlserver_metrics (
        capture_timestamp, server_id,
        avg_cpu_load, memory_usage, active_users,
        total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
    )
    SELECT ts, v_ss_id,
        round((random() * 80 + 5)::NUMERIC, 1),
        round((random() * 45 + 50)::NUMERIC, 1),
        (random() * 200 + 10)::INT,
        (random() * 3000 + 100)::BIGINT,
        CASE WHEN random() < 0.02 THEN 1 ELSE 0 END::BIGINT,
        round((random() * 20000 + 55000)::NUMERIC, 0),
        round((random() * 5000 + 5500)::NUMERIC, 0),
        round((random() * 80000 + 25000)::NUMERIC, 0)
    FROM generate_series(v_from_recent, v_to_recent, v_step_recent) ts
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_metrics (recent 32d): % rows', v_row_count;

    -- sqlserver_disk_history (recent – 3 databases)
    INSERT INTO sqlserver_disk_history (
        capture_timestamp, server_id, database_name,
        data_mb, log_mb, free_mb, delta_data_mb, delta_log_mb
    )
    SELECT ts, v_ss_id, db.name,
        round((db.base_data_mb + random() * 1000)::NUMERIC, 1),
        round((db.base_log_mb + random() * 400)::NUMERIC, 1),
        round((random() * 45000 + 12000)::NUMERIC, 1),
        round((random() * 15)::NUMERIC, 2),
        round((random() * 8)::NUMERIC, 2)
    FROM generate_series(v_from_recent, v_to_recent, v_step_recent) ts
    CROSS JOIN (VALUES
        ('ProductionDB', 51000, 5100),
        ('ReportingDB',  21000, 2100),
        ('master',        5100,  510)
    ) db(name, base_data_mb, base_log_mb)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_disk_history (recent 32d): % rows', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: sqlserver_ag_health  (2 replicas per timestamp, 30-min cadence)
    -- Priority: HIGH | Retention: 30 days
    -- -------------------------------------------------------------------------
    -- Deep tier
    INSERT INTO sqlserver_ag_health (
        capture_timestamp, server_id,
        ag_name, replica_server_name, database_name,
        replica_role, operational_state, connected_state,
        synchronization_state, synchronization_state_desc,
        is_primary_replica, log_send_queue_kb, redo_queue_kb,
        log_send_rate_kb, redo_rate_kb,
        last_sent_time, last_received_time, last_hardened_time, last_redone_time,
        secondary_lag_seconds
    )
    SELECT ts, v_ss_id,
        'AG-Prod',
        r.replica_name,
        db.db_name,
        r.role,
        'ONLINE',
        'CONNECTED',
        CASE WHEN r.role = 'PRIMARY' THEN 'SYNCHRONIZED' ELSE 'SYNCHRONIZING' END,
        CASE WHEN r.role = 'PRIMARY' THEN 'SYNCHRONIZED' ELSE 'SYNCHRONIZING' END,
        r.role = 'PRIMARY',
        CASE WHEN r.role = 'PRIMARY' THEN 0 ELSE (random() * 1024)::BIGINT END,
        CASE WHEN r.role = 'PRIMARY' THEN 0 ELSE (random() * 512)::BIGINT END,
        CASE WHEN r.role = 'PRIMARY' THEN 0 ELSE (random() * 2048)::BIGINT END,
        CASE WHEN r.role = 'PRIMARY' THEN 0 ELSE (random() * 1024)::BIGINT END,
        ts - INTERVAL '5 seconds',
        ts - INTERVAL '6 seconds',
        ts - INTERVAL '7 seconds',
        ts - INTERVAL '8 seconds',
        CASE WHEN r.role = 'PRIMARY' THEN 0 ELSE (random() * 10)::BIGINT END
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    CROSS JOIN (VALUES ('SQL-PRIMARY', 'PRIMARY'), ('SQL-SECONDARY', 'SECONDARY')) r(replica_name, role)
    CROSS JOIN (VALUES ('ProductionDB'), ('ReportingDB')) db(db_name)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_ag_health: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: sqlserver_latch_waits  (5 latch types per timestamp)
    -- Priority: LOW | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_latch_waits (
        capture_timestamp, server_id, wait_type,
        waiting_tasks_count, wait_time_ms, signal_wait_time_ms
    )
    SELECT ts, v_ss_id, lt.wait_type,
        (random() * 1000 + 10)::BIGINT,
        (random() * 50000 + 1000)::BIGINT,
        (random() * 5000 + 100)::BIGINT
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    CROSS JOIN (VALUES
        ('BUFFER'),
        ('DATABASE'),
        ('IO_COMPLETION'),
        ('CACHESTORE_OBJCP'),
        ('ACCESS_METHODS_DATASET_PARENT')
    ) lt(wait_type)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_latch_waits: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: sqlserver_spinlock_stats  (3 spinlock types)
    -- Priority: LOW | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_spinlock_stats (
        capture_timestamp, server_id, spinlock_type,
        collisions, spins, sleep_time_ms
    )
    SELECT ts, v_ss_id, sl.spinlock_type,
        (random() * 100000 + 1000)::BIGINT,
        (random() * 1000000 + 10000)::BIGINT,
        (random() * 100)::BIGINT
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    CROSS JOIN (VALUES ('LOCK_RW_RESOURCE_GROUP'), ('OBJECTSTORE'), ('DBREPL')) sl(spinlock_type)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_spinlock_stats: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: sqlserver_procedure_stats  (3 procedures per timestamp, 30-min)
    -- Priority: HIGH | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_procedure_stats (
        capture_timestamp, server_id,
        database_name, schema_name, object_name, query_hash,
        execution_count, total_worker_time_ms, total_elapsed_time_ms,
        total_logical_reads, total_physical_reads
    )
    SELECT ts, v_ss_id, 'ProductionDB', 'dbo', p.proc_name, p.q_hash,
        (random() * 1000 + 10)::BIGINT,
        round((random() * 50000 + 1000)::NUMERIC, 2),
        round((random() * 60000 + 1500)::NUMERIC, 2),
        (random() * 500000 + 10000)::BIGINT,
        (random() * 1000)::BIGINT
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    CROSS JOIN (VALUES
        ('usp_GetOrderHistory', 12345678::BIGINT),
        ('usp_UpdateCustomer',  87654321::BIGINT),
        ('fn_CalculateRisk',    11223344::BIGINT)
    ) p(proc_name, q_hash)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_procedure_stats: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: sqlserver_long_running_queries  (sparse, when random() < 0.2)
    -- Priority: HIGH | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_long_running_queries (
        capture_timestamp, server_id,
        session_id, request_id, database_name, login_name, host_name,
        program_name, query_hash, wait_type, blocking_session_id,
        status, cpu_time_ms, total_elapsed_time_ms,
        reads, writes, granted_query_memory_mb, row_count, percent_complete
    )
    SELECT ts, v_ss_id,
        (random() * 100 + 55)::INT,
        0,
        'ProductionDB',
        'app_user',
        'APP-SERVER-01',
        'Enterprise App',
        (random() * 9999999 + 1000000)::BIGINT,
        CASE (random() * 4)::INT
            WHEN 0 THEN 'PAGEIOLATCH_SH'
            WHEN 1 THEN 'LCK_M_X'
            WHEN 2 THEN 'CXPACKET'
            ELSE NULL
        END,
        CASE WHEN random() < 0.2 THEN (random() * 50 + 51)::INT ELSE 0 END,
        'running',
        (random() * 30000 + 5000)::BIGINT,
        (random() * 40000 + 6000)::BIGINT,
        (random() * 1000000 + 10000)::BIGINT,
        (random() * 10000)::BIGINT,
        (random() * 512 + 32)::INT,
        (random() * 100000)::BIGINT,
        NULL
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    WHERE random() < 0.25  -- sparse: ~25% of intervals have a long-running query
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_long_running_queries: % rows inserted (sparse)', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: sqlserver_memory_grant_waiters  (sparse events)
    -- Priority: LOW | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_memory_grant_waiters (
        capture_timestamp, server_id,
        session_id, request_id, database_name, login_name,
        requested_memory_kb, granted_memory_kb, required_memory_kb,
        wait_time_ms, dop, query_text
    )
    SELECT ts, v_ss_id,
        (random() * 50 + 55)::INT,
        0,
        'ProductionDB',
        'app_user',
        (random() * 262144 + 32768)::BIGINT,
        0::BIGINT,
        (random() * 65536 + 16384)::BIGINT,
        (random() * 30000 + 1000)::BIGINT,
        4,
        'SELECT o.*, c.* FROM Orders o JOIN Customers c ON o.customer_id = c.id WHERE ...'
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    WHERE random() < 0.05  -- rare: memory grant waiters are uncommon
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_memory_grant_waiters: % rows inserted (sparse)', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: sqlserver_waiting_tasks  (3 wait types per timestamp)
    -- Priority: LOW | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO sqlserver_waiting_tasks (
        capture_timestamp, server_id, wait_type,
        resource_description, waiting_tasks_count, wait_duration_ms
    )
    SELECT ts, v_ss_id, wt.wait_type, wt.resource_desc,
        (random() * 10 + 1)::BIGINT,
        (random() * 5000 + 100)::BIGINT
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    CROSS JOIN (VALUES
        ('PAGEIOLATCH_SH',   'pageid: 1:12345'),
        ('LCK_M_X',         'objectid: 98765321'),
        ('SOS_SCHEDULER_YIELD', '')
    ) wt(wait_type, resource_desc)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'sqlserver_waiting_tasks: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: postgres_wait_event_stats  (6 event types per timestamp)
    -- Priority: MEDIUM | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO postgres_wait_event_stats (
        capture_timestamp, server_id,
        wait_event_type, wait_event, sessions_count
    )
    SELECT ts, v_pg_id, we.event_type, we.event_name,
        (random() * 20 + 1)::INT
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    CROSS JOIN (VALUES
        ('Lock',   'relation'),
        ('Lock',   'tuple'),
        ('IO',     'DataFileRead'),
        ('IO',     'WALWrite'),
        ('Client', 'ClientRead'),
        ('IPC',    'BgWorkerShutdown')
    ) we(event_type, event_name)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'postgres_wait_event_stats: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: postgres_db_io_stats  (3 databases per timestamp)
    -- Priority: MEDIUM | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO postgres_db_io_stats (
        capture_timestamp, server_id, database_name,
        blks_read, blks_hit, temp_files, temp_bytes
    )
    SELECT ts, v_pg_id, db.db_name,
        (random() * 100000 + 1000)::BIGINT + EXTRACT(EPOCH FROM (ts - v_from_deep))::BIGINT / 60,
        (random() * 900000 + 50000)::BIGINT + EXTRACT(EPOCH FROM (ts - v_from_deep))::BIGINT / 10,
        (random() * 10)::BIGINT,
        (random() * 104857600)::BIGINT
    FROM generate_series(v_from_deep, v_to_recent, v_step_deep) ts
    CROSS JOIN (VALUES ('app_db'), ('analytics'), ('postgres')) db(db_name)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'postgres_db_io_stats: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: monitor.pg_session_activity_ts  (5 sessions per snapshot, 5-min)
    -- Priority: MEDIUM | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_session_activity_ts (
        capture_timestamp, server_id,
        dbname, pid, usename, application_name,
        client_addr, state, wait_event_type, wait_event,
        backend_type, query_id, query,
        xact_start, query_start, state_change, backend_start
    )
    SELECT ts, v_pg_id,
        'app_db',
        (55000 + s.sid)::INT,
        'app_user',
        'prod-app-server',
        '10.0.1.' || (s.sid % 254 + 1)::TEXT::INET,
        CASE (random() * 3)::INT WHEN 0 THEN 'active' WHEN 1 THEN 'idle' ELSE 'idle in transaction' END,
        CASE WHEN random() < 0.3 THEN 'Lock' ELSE NULL END,
        CASE WHEN random() < 0.3 THEN 'relation' ELSE NULL END,
        'client backend',
        (random() * 999999)::BIGINT,
        'SELECT * FROM orders WHERE status = $1 AND created_at > $2',
        ts - INTERVAL '500 ms',
        ts - INTERVAL '200 ms',
        ts - INTERVAL '100 ms',
        ts - INTERVAL '1 hour'
    FROM generate_series(v_from_deep, v_to_recent, INTERVAL '5 minutes') ts
    CROSS JOIN generate_series(1, 5) s(sid)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_session_activity_ts: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: monitor.pg_wait_event_summary_ts  (4 states per timestamp, 5-min)
    -- Priority: MEDIUM | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_wait_event_summary_ts (
        capture_timestamp, server_id,
        wait_event_type, wait_event, sessions, state
    )
    SELECT ts, v_pg_id,
        we.event_type, we.event_name,
        (random() * 15 + 1)::INT,
        we.state
    FROM generate_series(v_from_deep, v_to_recent, INTERVAL '5 minutes') ts
    CROSS JOIN (VALUES
        ('Lock',   'relation',   'active'),
        ('IO',     'DataFileRead','active'),
        ('Client', 'ClientRead', 'idle'),
        ('IPC',    'BgWorkerShutdown', 'idle in transaction')
    ) we(event_type, event_name, state)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_wait_event_summary_ts: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: monitor.pg_db_load_ts  (AAS approximation, every 5 min)
    -- Priority: MEDIUM | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_db_load_ts (
        capture_timestamp, server_id,
        active_sessions, cpu_sessions, waiting_sessions,
        io_sessions, lock_sessions, idle_in_txn
    )
    SELECT ts, v_pg_id,
        (random() * 30 + 5)::INT,
        (random() * 15 + 2)::INT,
        (random() * 8 + 1)::INT,
        (random() * 5 + 1)::INT,
        (random() * 3)::INT,
        (random() * 4)::INT
    FROM generate_series(v_from_deep, v_to_recent, INTERVAL '5 minutes') ts
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_db_load_ts: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: monitor.pg_query_wait_profile_ts  (top 5 queries per snapshot, 5-min)
    -- Priority: HIGH | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_query_wait_profile_ts (
        capture_timestamp, server_id,
        queryid, calls, total_exec_time, mean_exec_time,
        rows, shared_blks_hit, shared_blks_read, temp_blks_written,
        query, usename
    )
    SELECT ts, v_pg_id,
        q.queryid,
        (random() * 5000 + 100)::BIGINT + EXTRACT(EPOCH FROM (ts - v_from_deep))::BIGINT / 300,
        round((random() * 500000 + 10000)::NUMERIC, 2),
        round((random() * 200 + 1)::NUMERIC, 4),
        (random() * 1000000 + 10000)::BIGINT,
        (random() * 10000000 + 100000)::BIGINT,
        (random() * 100000 + 1000)::BIGINT,
        (random() * 10000)::BIGINT,
        q.query_text,
        'app_user'
    FROM generate_series(v_from_deep, v_to_recent, INTERVAL '5 minutes') ts
    CROSS JOIN (VALUES
        (1234567890::BIGINT, 'SELECT * FROM orders WHERE customer_id = $1'),
        (9876543210::BIGINT, 'UPDATE inventory SET quantity = $1 WHERE product_id = $2'),
        (1122334455::BIGINT, 'SELECT o.id, c.name FROM orders o JOIN customers c ON o.customer_id = c.id'),
        (5544332211::BIGINT, 'DELETE FROM sessions WHERE expires_at < $1'),
        (1357924680::BIGINT, 'INSERT INTO audit_log (event, user_id, ts) VALUES ($1, $2, $3)')
    ) q(queryid, query_text)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_query_wait_profile_ts: % rows inserted', v_row_count;

    -- -------------------------------------------------------------------------
    -- GROUP B: monitor.pg_ddl_activity_ts  (5 tables per snapshot, hourly)
    -- Priority: HIGH | Retention: 30 days
    -- -------------------------------------------------------------------------
    INSERT INTO monitor.pg_ddl_activity_ts (
        capture_timestamp, server_id,
        schemaname, relname, n_tup_ins, n_tup_upd, n_tup_del
    )
    SELECT ts, v_pg_id,
        'public',
        tbl.tbl_name,
        (random() * 10000 + 100)::BIGINT + EXTRACT(EPOCH FROM (ts - v_from_deep))::BIGINT / 3600,
        (random() * 5000 + 50)::BIGINT + EXTRACT(EPOCH FROM (ts - v_from_deep))::BIGINT / 3600,
        (random() * 1000 + 10)::BIGINT + EXTRACT(EPOCH FROM (ts - v_from_deep))::BIGINT / 7200
    FROM generate_series(v_from_deep, v_to_recent, INTERVAL '1 hour') ts
    CROSS JOIN (VALUES
        ('orders'), ('customers'), ('inventory'), ('audit_log'), ('sessions')
    ) tbl(tbl_name)
    ON CONFLICT DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    RAISE NOTICE 'monitor.pg_ddl_activity_ts: % rows inserted', v_row_count;

    RAISE NOTICE '';
    RAISE NOTICE '=== 30-day and Group B seeding complete ===';

END $$;

\echo ''
\echo '=== Verification: Row counts for Group B tables ==='
SELECT
    'sqlserver_ag_health'                   AS table_name,
    COUNT(*) AS rows_inserted,
    MIN(capture_timestamp)::DATE AS min_date,
    MAX(capture_timestamp)::DATE AS max_date
FROM sqlserver_ag_health
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_latch_waits', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_latch_waits
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_spinlock_stats', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_spinlock_stats
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_procedure_stats', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_procedure_stats
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_long_running_queries', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_long_running_queries
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_waiting_tasks', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_waiting_tasks
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'sqlserver_memory_grant_waiters', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM sqlserver_memory_grant_waiters
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
UNION ALL
SELECT 'postgres_wait_event_stats', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM postgres_wait_event_stats
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'postgres_db_io_stats', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM postgres_db_io_stats
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_session_activity_ts', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_session_activity_ts
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_wait_event_summary_ts', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_wait_event_summary_ts
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_db_load_ts', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_db_load_ts
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_query_wait_profile_ts', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_query_wait_profile_ts
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
UNION ALL
SELECT 'monitor.pg_ddl_activity_ts', COUNT(*),
    MIN(capture_timestamp)::DATE, MAX(capture_timestamp)::DATE
FROM monitor.pg_ddl_activity_ts
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
ORDER BY table_name;

\echo ''
\echo '=== Proceed to 04_cold_storage_verify_pre_export.sql ==='
