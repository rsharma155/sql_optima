-- ============================================================================
-- SQL Optima: Historical Data Generator for TimescaleDB Hypertables (Expanded)
-- Use this to test compression, retention, and performance across high-volume tables.
-- ============================================================================

DO $$
DECLARE
    v_server_id UUID := '00000000-0000-0000-0000-000000000001'; -- Dummy Test Server
    v_server_name TEXT := 'TEST-SQL-SERVER-HISTORICAL';
    v_num_days INTEGER := 7; -- Change to 90 for 3-month test
    v_interval_seconds INTEGER := 60; -- 1 minute frequency for high-density tables
    v_batch_ts TIMESTAMPTZ;
BEGIN
    -- 1. Ensure the test server exists in the registry
    INSERT INTO optima_servers (id, name, db_type, host, port, username, encrypted_secret, encrypted_dek, is_active)
    VALUES (
        v_server_id,
        v_server_name,
        'sqlserver',
        'localhost',
        1433,
        'sa',
        '\x00', -- Dummy
        '\x00', -- Dummy
        TRUE
    ) ON CONFLICT (id) DO NOTHING;

    RAISE NOTICE 'Generating data for server: % (%)', v_server_name, v_server_id;
    RAISE NOTICE 'Days: %, Base Frequency: %s', v_num_days, v_interval_seconds;

    -- ========================================================================
    -- HIGH VOLUME TABLES (Top Growth Candidates)
    -- ========================================================================

    -- 1. sqlserver_metrics (Core Metrics - 1m interval)
    INSERT INTO sqlserver_metrics (capture_timestamp, server_id, avg_cpu_load, memory_usage, active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb)
    SELECT ts, v_server_id, random()*50+10, random()*80+20, (random()*100)::int, (random()*1000)::bigint, 0, 50000, 5000, 10000
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), (v_interval_seconds || ' seconds')::INTERVAL) ts ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_metrics';

    -- 2. sqlserver_wait_stats_cumulative (Wait Stats - 1m interval)
    INSERT INTO sqlserver_wait_stats_cumulative (capture_timestamp, server_id, wait_type, waiting_tasks_count, wait_time_ms, signal_wait_time_ms)
    SELECT ts, v_server_id, w.wait_type, (random()*10000)::bigint, (random()*100000)::bigint, (random()*10000)::bigint
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), (v_interval_seconds || ' seconds')::INTERVAL) ts
    CROSS JOIN (VALUES ('PAGEIOLATCH_SH'), ('SOS_SCHEDULER_YIELD'), ('WRITELOG'), ('CXPACKET'), ('LCK_M_X')) w(wait_type)
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_wait_stats_cumulative';

    -- 3. sqlserver_wait_stats_delta (Wait Stats Deltas - 1m interval)
    INSERT INTO sqlserver_wait_stats_delta (capture_timestamp, server_id, wait_type, wait_category, delta_wait_ms, delta_signal_wait_ms, delta_resource_wait_ms, delta_waiting_tasks)
    SELECT ts, v_server_id, w.wait_type, w.cat, (random()*1000)::bigint, (random()*100)::bigint, (random()*900)::bigint, (random()*100)::bigint
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), (v_interval_seconds || ' seconds')::INTERVAL) ts
    CROSS JOIN (VALUES ('PAGEIOLATCH_SH', 'IO'), ('SOS_SCHEDULER_YIELD', 'CPU'), ('WRITELOG', 'IO'), ('CXPACKET', 'Parallelism'), ('LCK_M_X', 'Locking')) w(wait_type, cat)
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_wait_stats_delta';

    -- 4. sqlserver_session_snapshots (Full Session details - 30s interval)
    INSERT INTO sqlserver_session_snapshots (capture_timestamp, server_id, session_id, login_name, database_name, status, cpu_time_ms, total_elapsed_time_ms, logical_reads, wait_type)
    SELECT ts, v_server_id, (s.id)::int, 'test_user', 'master', 'running', random()*1000, random()*2000, random()*5000, 'PAGEIOLATCH_SH'
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), '30 seconds'::INTERVAL) ts
    CROSS JOIN generate_series(51, 60) s(id) -- Simulating 10 active sessions
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_session_snapshots';

    -- 5. sqlserver_perf_counters (Perf Counters - 30s interval)
    INSERT INTO sqlserver_perf_counters (capture_timestamp, server_id, counter_name, value_per_sec)
    SELECT ts, v_server_id, c.name, random()*1000
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), '30 seconds'::INTERVAL) ts
    CROSS JOIN (VALUES ('Batch Requests/sec'), ('SQL Compilations/sec'), ('Page Life Expectancy'), ('Buffer Cache Hit Ratio')) c(name)
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_perf_counters';

    -- 6. sqlserver_query_metrics_v2 (Query Stats - 1m interval)
    INSERT INTO sqlserver_query_metrics_v2 (capture_timestamp, server_id, database_name, query_hash, plan_handle, total_executions, total_cpu_ms, total_elapsed_ms, statement_text)
    SELECT ts, v_server_id, 'master', (random()*1000000)::bigint, '\x010203', (random()*100)::bigint, random()*5000, random()*10000, 'SELECT * FROM test_table WHERE id = ' || (random()*100)::int
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), '1 minute'::INTERVAL) ts
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_query_metrics_v2';

    -- 7. sqlserver_blocking_snapshots (Blocking snapshots - 30s interval)
    INSERT INTO sqlserver_blocking_snapshots (capture_timestamp, server_id, session_id, blocking_session_id, wait_type, wait_duration_ms, database_name)
    SELECT ts, v_server_id, 100, 99, 'LCK_M_X', random()*5000, 'master'
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), '30 seconds'::INTERVAL) ts
    WHERE random() > 0.95 -- Simulating intermittent blocking
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_blocking_snapshots';

    -- 8. staging.sqlserver_session_request_raw (Staging Pulse - 15s interval)
    INSERT INTO staging.sqlserver_session_request_raw (capture_timestamp, server_id, session_id, login_name, db_name, session_status, wait_type, cpu_time_ms)
    SELECT ts, v_server_id, 55, 'pulse_user', 'master', 'running', 'SOS_SCHEDULER_YIELD', random()*100
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), '15 seconds'::INTERVAL) ts
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: staging.sqlserver_session_request_raw';

    -- 9. sqlserver_file_io (File IO Stats - 1m interval)
    INSERT INTO sqlserver_file_io (capture_timestamp, server_id, database_name, file_type, read_latency_ms, write_latency_ms, read_bytes_per_sec)
    SELECT ts, v_server_id, 'tempdb', 'DATA', random()*20, random()*10, random()*1000000
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), '1 minute'::INTERVAL) ts
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_file_io';

    -- 10. sqlserver_procedure_stats (Procedure Stats - 2m interval)
    INSERT INTO sqlserver_procedure_stats (capture_timestamp, server_id, database_name, object_name, query_hash, execution_count, total_worker_time_ms)
    SELECT ts, v_server_id, 'master', 'usp_GetReport', (random()*1000000)::bigint, (random()*100)::bigint, random()*5000
    FROM generate_series(now() - (v_num_days || ' days')::INTERVAL, now(), '2 minutes'::INTERVAL) ts
    ON CONFLICT DO NOTHING;
    RAISE NOTICE 'Populated: sqlserver_procedure_stats';

END $$;
