-- =============================================================================
-- Cold Storage Pipeline — DuckDB Validation Queries
-- Script 07: Ad-hoc queries against cold Parquet files in MinIO
--
-- Run with DuckDB CLI after the cold storage exporter has uploaded files:
--   duckdb -init 07_cold_storage_duckdb_validation.sql
--
-- Or paste into an interactive DuckDB session.
--
-- Prerequisites:
--   - DuckDB installed (brew install duckdb or https://duckdb.org/docs/installation)
--   - MinIO running and accessible at localhost:9000
--   - Cold storage export has completed at least one successful cycle
-- =============================================================================

-- ─────────────────────────────────────────────────────────────────────────────
-- SETUP: Install and configure S3 extension for MinIO
-- ─────────────────────────────────────────────────────────────────────────────
INSTALL httpfs;
LOAD httpfs;

-- Configure MinIO credentials (match docker/.env.example defaults)
SET s3_endpoint='localhost:9000';
SET s3_access_key_id='sqloptima';
SET s3_secret_access_key='change_me_in_production';
SET s3_use_ssl=false;
SET s3_url_style='path';
SET s3_region='us-east-1';

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 1: Count all Parquet files in the cold bucket
-- Expected: > 0 files; one or more per table per server per day
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 1: File inventory' AS test_name;

-- List all table-level partitions present
SELECT
    regexp_extract(file, 'table=([^/]+)', 1) AS table_name,
    regexp_extract(file, 'server_id=([^/]+)', 1) AS server_id,
    COUNT(*) AS file_count,
    round(SUM(size) / 1024.0 / 1024.0, 2) AS total_mb
FROM glob('s3://sql-optima-cold/metrics/**/*.parquet')
GROUP BY 1, 2
ORDER BY table_name, server_id;

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 2: sqlserver_cpu_history row count and time range
-- Expected: rows spanning ~90 days for SQL Server test server
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 2: sqlserver_cpu_history' AS test_name;

SELECT
    COUNT(*)                                                       AS total_rows,
    to_timestamp(MIN(capture_timestamp_ms) / 1000.0)::DATE        AS min_date,
    to_timestamp(MAX(capture_timestamp_ms) / 1000.0)::DATE        AS max_date,
    round(AVG(sql_cpu_utilization), 2)                            AS avg_sql_cpu_pct,
    round(MAX(sql_cpu_utilization), 2)                            AS peak_sql_cpu_pct
FROM read_parquet(
    's3://sql-optima-cold/metrics/engine=sqlserver/table=sqlserver_cpu_history/**/*.parquet',
    hive_partitioning = true
)
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 3: sqlserver_wait_history — all 5 wait types present
-- Expected: exactly 5 distinct wait_type values
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 3: sqlserver_wait_history wait types' AS test_name;

SELECT
    wait_type,
    COUNT(*)                                     AS row_count,
    round(AVG(wait_time_ms_total), 0)            AS avg_wait_ms_total
FROM read_parquet(
    's3://sql-optima-cold/metrics/engine=sqlserver/table=sqlserver_wait_history/**/*.parquet',
    hive_partitioning = true
)
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
GROUP BY wait_type
ORDER BY wait_type;

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 4: sqlserver_disk_history — all 3 databases present
-- Expected: ProductionDB, ReportingDB, master
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 4: sqlserver_disk_history databases' AS test_name;

SELECT
    database_name,
    COUNT(*)                              AS row_count,
    round(AVG(data_mb), 0)              AS avg_data_mb,
    round(MAX(data_mb), 0)              AS max_data_mb,
    to_timestamp(MIN(capture_timestamp_ms) / 1000.0)::DATE AS min_date,
    to_timestamp(MAX(capture_timestamp_ms) / 1000.0)::DATE AS max_date
FROM read_parquet(
    's3://sql-optima-cold/metrics/engine=sqlserver/table=sqlserver_disk_history/**/*.parquet',
    hive_partitioning = true
)
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
GROUP BY database_name
ORDER BY database_name;

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 5: sqlserver_memory_metrics — PLE and memory pressure
-- Expected: ple_seconds 300–4300, no nulls in key columns
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 5: sqlserver_memory_metrics' AS test_name;

SELECT
    COUNT(*)                                  AS total_rows,
    round(AVG(ple_seconds), 0)               AS avg_ple_seconds,
    MIN(ple_seconds)                         AS min_ple_seconds,
    round(AVG(sql_memory_utilization_pct), 1) AS avg_mem_util_pct,
    COUNT(*) FILTER (WHERE ple_seconds IS NULL) AS null_ple_count
FROM read_parquet(
    's3://sql-optima-cold/metrics/engine=sqlserver/table=sqlserver_memory_metrics/**/*.parquet',
    hive_partitioning = true
)
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 6: postgres_wait_event_stats — PostgreSQL server
-- Expected: 6 distinct event types
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 6: postgres_wait_event_stats' AS test_name;

SELECT
    wait_event_type,
    wait_event,
    COUNT(*)              AS row_count,
    round(AVG(count), 1) AS avg_sessions
FROM read_parquet(
    's3://sql-optima-cold/metrics/engine=postgres/table=postgres_wait_event_stats/**/*.parquet',
    hive_partitioning = true
)
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
GROUP BY 1, 2
ORDER BY 1, 2;

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 7: monitor.pg_failed_login_events — security audit
-- Expected: sparse rows across 90-day window
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 7: monitor.pg_failed_login_events' AS test_name;

SELECT
    COUNT(*)                                                       AS total_events,
    COUNT(DISTINCT username)                                       AS distinct_usernames,
    to_timestamp(MIN(capture_timestamp_ms) / 1000.0)::DATE        AS first_event,
    to_timestamp(MAX(capture_timestamp_ms) / 1000.0)::DATE        AS last_event
FROM read_parquet(
    's3://sql-optima-cold/metrics/engine=postgres/table=monitor.pg_failed_login_events/**/*.parquet',
    hive_partitioning = true
)
WHERE server_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb';

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 8: Cross-table correlation — CPU spikes vs wait types
-- Expected: query runs without error; top wait types during high-CPU windows
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 8: Cross-table correlation (CPU vs wait types)' AS test_name;

WITH cpu AS (
    SELECT
        date_trunc('hour', to_timestamp(capture_timestamp_ms / 1000.0)) AS hour,
        server_id,
        AVG(sql_cpu_utilization) AS avg_cpu
    FROM read_parquet(
        's3://sql-optima-cold/metrics/engine=sqlserver/table=sqlserver_cpu_history/**/*.parquet',
        hive_partitioning = true
    )
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    GROUP BY 1, 2
),
waits AS (
    SELECT
        date_trunc('hour', to_timestamp(capture_timestamp_ms / 1000.0)) AS hour,
        server_id,
        wait_type,
        SUM(wait_time_ms_total) AS total_wait_ms
    FROM read_parquet(
        's3://sql-optima-cold/metrics/engine=sqlserver/table=sqlserver_wait_history/**/*.parquet',
        hive_partitioning = true
    )
    WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
    GROUP BY 1, 2, 3
)
SELECT
    cpu.hour::DATE             AS date,
    cpu.avg_cpu               AS avg_cpu_pct,
    waits.wait_type,
    round(waits.total_wait_ms / 1000.0, 1) AS total_wait_sec
FROM cpu
JOIN waits ON cpu.hour = waits.hour AND cpu.server_id = waits.server_id
WHERE cpu.avg_cpu > 40  -- hours with meaningful CPU usage
ORDER BY cpu.avg_cpu DESC, waits.total_wait_ms DESC
LIMIT 20;

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 9: Parquet file quality — check for corrupt files or nulls
-- Expected: 0 corrupt files; nulls only in nullable columns
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 9: Data quality check (sqlserver_cpu_history)' AS test_name;

SELECT
    COUNT(*)                                              AS total_rows,
    COUNT(*) FILTER (WHERE capture_timestamp_ms IS NULL) AS null_timestamps,
    COUNT(*) FILTER (WHERE server_id IS NULL)            AS null_server_ids,
    COUNT(*) FILTER (WHERE sql_cpu_utilization IS NULL)  AS null_cpu_values,
    COUNT(*) FILTER (WHERE sql_cpu_utilization < 0)      AS negative_cpu_values,
    COUNT(*) FILTER (WHERE sql_cpu_utilization > 100)    AS cpu_over_100_pct
FROM read_parquet(
    's3://sql-optima-cold/metrics/engine=sqlserver/table=sqlserver_cpu_history/**/*.parquet',
    hive_partitioning = true
);

-- ─────────────────────────────────────────────────────────────────────────────
-- TEST 10: Hive partition pruning — time-filtered query
-- Expected: DuckDB reads only the relevant year/month/day partitions (fast)
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'TEST 10: Partition pruning (last 30 days of cold data)' AS test_name;

SELECT
    EXTRACT(YEAR  FROM to_timestamp(capture_timestamp_ms / 1000.0)) AS year,
    EXTRACT(MONTH FROM to_timestamp(capture_timestamp_ms / 1000.0)) AS month,
    COUNT(*)                                                         AS rows_in_month
FROM read_parquet(
    's3://sql-optima-cold/metrics/engine=sqlserver/table=sqlserver_cpu_history/**/*.parquet',
    hive_partitioning = true
)
WHERE server_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
  AND capture_timestamp_ms >= epoch_ms(NOW() - INTERVAL '30 days')
GROUP BY 1, 2
ORDER BY 1, 2;

-- ─────────────────────────────────────────────────────────────────────────────
-- SUMMARY
-- ─────────────────────────────────────────────────────────────────────────────
SELECT 'VALIDATION COMPLETE' AS result,
       'All 10 tests executed. Review output above for PASS/FAIL.' AS note;
