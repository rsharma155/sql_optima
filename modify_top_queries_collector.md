Upgrade the monitoring system to capture SQL Server query workload using a change-only snapshot + delta pipeline and expose fully filterable dashboards (CPU / Duration / Logical Reads / Executions).

System must be restart-safe, storage-efficient, and support long-term analytics.

1. Architecture Overview

Pipeline must become:

SQL Server DMV
      ↓
Staging table (raw pull)
      ↓
Change-only Snapshot Table (deduplicated)
      ↓
Delta Processor
      ↓
Interval Metrics Table (dashboard source)
      ↓
API → Dashboard

Key rule:

Dashboards must NEVER read snapshot table.
2. SQL Server Collector Query (FINAL VERSION)

Run every 2–5 minutes.
⚠️ No time filtering allowed.

SELECT
    DB_NAME(COALESCE(pa.dbid, st.dbid)) AS database_name,
    ISNULL(s.login_name, 'Unknown') AS login_name,
    ISNULL(s.program_name, 'Unknown') AS client_app,
    qs.creation_time,
    qs.last_execution_time,

    qs.execution_count AS total_executions,
    qs.total_worker_time / 1000 AS total_cpu_ms,
    qs.total_elapsed_time / 1000 AS total_elapsed_ms,
    qs.total_logical_reads,
    qs.total_physical_reads,
    qs.total_rows,

    CASE 
        WHEN st.objectid IS NOT NULL
        THEN 'EXEC ' + QUOTENAME(OBJECT_SCHEMA_NAME(st.objectid, COALESCE(pa.dbid, st.dbid)))
             + '.' + QUOTENAME(OBJECT_NAME(st.objectid, COALESCE(pa.dbid, st.dbid)))
        ELSE SUBSTRING(st.text,
            (qs.statement_start_offset/2)+1,
            ((CASE qs.statement_end_offset
                WHEN -1 THEN DATALENGTH(st.text)
                ELSE qs.statement_end_offset END
             - qs.statement_start_offset)/2) + 1)
    END AS query_text,

    CONVERT(VARCHAR(64), qs.query_hash, 1) AS query_hash
FROM sys.dm_exec_query_stats qs
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
OUTER APPLY (
    SELECT CONVERT(INT,value) dbid
    FROM sys.dm_exec_plan_attributes(qs.plan_handle)
    WHERE attribute = N'dbid'
) pa
OUTER APPLY (
    SELECT TOP 1 ses.original_login_name login_name, ses.program_name
    FROM sys.dm_exec_sessions ses
    JOIN sys.dm_exec_connections con ON ses.session_id = con.session_id
    WHERE con.most_recent_sql_handle = qs.sql_handle
) s
WHERE COALESCE(pa.dbid, st.dbid) > 4;
3. TimescaleDB Schema
3.1 Staging Table (temporary ingest)
CREATE TABLE sqlserver.query_stats_staging
(
    capture_time timestamptz,
    database_name text,
    login_name text,
    client_app text,
    query_hash text,
    query_text text,

    total_executions bigint,
    total_cpu_ms bigint,
    total_elapsed_ms bigint,
    total_logical_reads bigint,
    total_physical_reads bigint,
    total_rows bigint
);
3.2 Snapshot Table (CHANGE-ONLY STORAGE)
CREATE TABLE sqlserver.query_stats_snapshot
(
    capture_time timestamptz NOT NULL,

    database_name text,
    login_name text,
    client_app text,
    query_hash text,
    query_text text,

    total_executions bigint,
    total_cpu_ms bigint,
    total_elapsed_ms bigint,
    total_logical_reads bigint,
    total_physical_reads bigint,
    total_rows bigint,

    row_fingerprint text NOT NULL,

    PRIMARY KEY (
        capture_time,
        query_hash,
        database_name,
        login_name,
        client_app
    )
);

SELECT create_hypertable('sqlserver.query_stats_snapshot','capture_time');
3.3 Interval Table (DASHBOARD SOURCE)
CREATE TABLE sqlserver.query_stats_interval
(
    bucket_start timestamptz,
    bucket_end timestamptz,

    database_name text,
    login_name text,
    client_app text,
    query_hash text,
    query_text text,

    executions bigint,
    cpu_ms bigint,
    duration_ms bigint,
    logical_reads bigint,
    physical_reads bigint,
    rows bigint,

    avg_cpu_ms numeric,
    avg_duration_ms numeric,
    avg_reads numeric,

    is_reset boolean,

    PRIMARY KEY (
        bucket_end,
        query_hash,
        database_name,
        login_name,
        client_app
    )
);

SELECT create_hypertable('sqlserver.query_stats_interval','bucket_end');
4. Change-Only Snapshot Insert

Compute fingerprint from cumulative counters.

INSERT INTO sqlserver.query_stats_snapshot
SELECT
    capture_time,
    database_name,
    login_name,
    client_app,
    query_hash,
    query_text,
    total_executions,
    total_cpu_ms,
    total_elapsed_ms,
    total_logical_reads,
    total_physical_reads,
    total_rows,

    md5(
        total_executions || '-' ||
        total_cpu_ms || '-' ||
        total_elapsed_ms || '-' ||
        total_logical_reads || '-' ||
        total_physical_reads || '-' ||
        total_rows
    ) AS row_fingerprint

FROM sqlserver.query_stats_staging s
WHERE NOT EXISTS (
    SELECT 1
    FROM sqlserver.query_stats_snapshot last
    WHERE last.query_hash = s.query_hash
      AND last.database_name = s.database_name
      AND last.login_name = s.login_name
      AND last.client_app = s.client_app
    ORDER BY capture_time DESC
    LIMIT 1
    HAVING last.row_fingerprint =
        md5(
            s.total_executions || '-' ||
            s.total_cpu_ms || '-' ||
            s.total_elapsed_ms || '-' ||
            s.total_logical_reads || '-' ||
            s.total_physical_reads || '-' ||
            s.total_rows
        )
);

Result:
Only store rows when counters change.

5. Delta Processor (interval metrics)
INSERT INTO sqlserver.query_stats_interval
SELECT
    prev.capture_time,
    curr.capture_time,

    curr.database_name,
    curr.login_name,
    curr.client_app,
    curr.query_hash,
    curr.query_text,

    CASE WHEN reset THEN 0 ELSE exec_delta END,
    CASE WHEN reset THEN 0 ELSE cpu_delta END,
    CASE WHEN reset THEN 0 ELSE dur_delta END,
    CASE WHEN reset THEN 0 ELSE reads_delta END,
    CASE WHEN reset THEN 0 ELSE phys_delta END,
    CASE WHEN reset THEN 0 ELSE rows_delta END,

    cpu_delta / NULLIF(exec_delta,0)::numeric,
    dur_delta / NULLIF(exec_delta,0)::numeric,
    reads_delta / NULLIF(exec_delta,0)::numeric,

    reset
FROM (
    SELECT
        curr.*,
        prev.capture_time prev_time,

        curr.total_executions - prev.total_executions exec_delta,
        curr.total_cpu_ms - prev.total_cpu_ms cpu_delta,
        curr.total_elapsed_ms - prev.total_elapsed_ms dur_delta,
        curr.total_logical_reads - prev.total_logical_reads reads_delta,
        curr.total_physical_reads - prev.total_physical_reads phys_delta,
        curr.total_rows - prev.total_rows rows_delta,

        (curr.total_executions < prev.total_executions
         OR curr.total_cpu_ms < prev.total_cpu_ms) reset

    FROM sqlserver.query_stats_snapshot curr
    JOIN LATERAL (
        SELECT *
        FROM sqlserver.query_stats_snapshot p
        WHERE p.query_hash = curr.query_hash
        AND p.database_name = curr.database_name
        AND p.login_name = curr.login_name
        AND p.client_app = curr.client_app
        AND p.capture_time < curr.capture_time
        ORDER BY capture_time DESC
        LIMIT 1
    ) prev ON true
) t;
6. Retention + Compression
SELECT add_retention_policy('sqlserver.query_stats_snapshot', INTERVAL '7 days');
SELECT add_compression_policy('sqlserver.query_stats_interval', INTERVAL '7 days');
7. Dashboard Requirements (NEW)

The dashboard must support user-selectable sorting + filtering by metric.

User can choose metric:

CPU
Duration
Logical Reads
Executions
8. Generic Dashboard Query (Dynamic Sorting)

Backend API must accept:

metric = cpu | duration | reads | executions
time_range = 15m | 1h | 24h
dimension = query | database | login | app
Example API query template
SELECT
    :dimension,
    SUM(
        CASE
            WHEN :metric = 'cpu' THEN cpu_ms
            WHEN :metric = 'duration' THEN duration_ms
            WHEN :metric = 'reads' THEN logical_reads
            WHEN :metric = 'executions' THEN executions
        END
    ) AS metric_value
FROM sqlserver.query_stats_interval
WHERE bucket_end > now() - :time_range::interval
GROUP BY :dimension
ORDER BY metric_value DESC
LIMIT 20;

This powers all leaderboard widgets.

9. Time-Series Chart Query
SELECT
    time_bucket('5 min', bucket_end) AS time,
    SUM(cpu_ms) cpu_ms,
    SUM(duration_ms) duration_ms,
    SUM(logical_reads) reads,
    SUM(executions) executions
FROM sqlserver.query_stats_interval
GROUP BY time
ORDER BY time;

Frontend switches which metric to plot.

10. Collector Application Flow

Every 2–5 minutes:

Execute DMV query
Insert rows → staging table
Insert changed rows → snapshot table
Run delta processor → interval table
Clear staging table