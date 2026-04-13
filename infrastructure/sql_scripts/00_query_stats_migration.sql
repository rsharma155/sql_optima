-- ============================================================================
-- Migration: Add Query Stats (Change-Only Snapshot + Delta Pipeline)
-- Version: 1.0.0
-- Run after Timescale schema init if sqlserver_query_stats_* tables are missing.
-- ============================================================================

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'sqlserver_query_stats_staging'
    ) THEN
        CREATE TABLE sqlserver_query_stats_staging (
            capture_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            server_instance_name TEXT NOT NULL,
            database_name TEXT,
            login_name TEXT,
            client_app TEXT,
            query_hash TEXT,
            query_text TEXT,
            total_executions BIGINT DEFAULT 0,
            total_cpu_ms BIGINT DEFAULT 0,
            total_elapsed_ms BIGINT DEFAULT 0,
            total_logical_reads BIGINT DEFAULT 0,
            total_physical_reads BIGINT DEFAULT 0,
            total_rows BIGINT DEFAULT 0,
            inserted_at TIMESTAMPTZ DEFAULT NOW()
        );
        RAISE NOTICE 'Created sqlserver_query_stats_staging table';
    ELSE
        RAISE NOTICE 'sqlserver_query_stats_staging already exists';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'sqlserver_query_stats_snapshot'
    ) THEN
        CREATE TABLE sqlserver_query_stats_snapshot (
            capture_time TIMESTAMPTZ NOT NULL,
            server_instance_name TEXT NOT NULL,
            database_name TEXT,
            login_name TEXT,
            client_app TEXT,
            query_hash TEXT,
            query_text TEXT,
            total_executions BIGINT DEFAULT 0,
            total_cpu_ms BIGINT DEFAULT 0,
            total_elapsed_ms BIGINT DEFAULT 0,
            total_logical_reads BIGINT DEFAULT 0,
            total_physical_reads BIGINT DEFAULT 0,
            total_rows BIGINT DEFAULT 0,
            row_fingerprint TEXT,
            inserted_at TIMESTAMPTZ DEFAULT NOW(),
            PRIMARY KEY (server_instance_name, query_hash, database_name, login_name, client_app, capture_time)
        );

        IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
            EXECUTE 'SELECT create_hypertable(''sqlserver_query_stats_snapshot'', ''capture_time'', if_not_exists => TRUE, migrate_data => FALSE)';
        END IF;

        CREATE INDEX IF NOT EXISTS idx_query_stats_snapshot_hash ON sqlserver_query_stats_snapshot (query_hash);
        CREATE INDEX IF NOT EXISTS idx_query_stats_snapshot_time ON sqlserver_query_stats_snapshot (capture_time DESC);
        RAISE NOTICE 'Created sqlserver_query_stats_snapshot table with hypertable';
    ELSE
        RAISE NOTICE 'sqlserver_query_stats_snapshot already exists';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'sqlserver_query_stats_interval'
    ) THEN
        CREATE TABLE sqlserver_query_stats_interval (
            bucket_start TIMESTAMPTZ NOT NULL,
            bucket_end TIMESTAMPTZ NOT NULL,
            server_instance_name TEXT NOT NULL,
            database_name TEXT,
            login_name TEXT,
            client_app TEXT,
            query_hash TEXT,
            query_text TEXT,
            executions BIGINT DEFAULT 0,
            cpu_ms BIGINT DEFAULT 0,
            duration_ms BIGINT DEFAULT 0,
            logical_reads BIGINT DEFAULT 0,
            physical_reads BIGINT DEFAULT 0,
            rows BIGINT DEFAULT 0,
            avg_cpu_ms NUMERIC DEFAULT 0,
            avg_duration_ms NUMERIC DEFAULT 0,
            avg_reads NUMERIC DEFAULT 0,
            is_reset BOOLEAN DEFAULT FALSE,
            inserted_at TIMESTAMPTZ DEFAULT NOW(),
            PRIMARY KEY (bucket_end, query_hash, database_name, login_name, client_app, server_instance_name)
        );

        IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
            EXECUTE 'SELECT create_hypertable(''sqlserver_query_stats_interval'', ''bucket_end'', if_not_exists => TRUE, migrate_data => FALSE)';
        END IF;

        CREATE INDEX IF NOT EXISTS idx_query_stats_interval_hash ON sqlserver_query_stats_interval (query_hash);
        CREATE INDEX IF NOT EXISTS idx_query_stats_interval_time ON sqlserver_query_stats_interval (bucket_end DESC);
        CREATE INDEX IF NOT EXISTS idx_query_stats_interval_server ON sqlserver_query_stats_interval (server_instance_name, bucket_end DESC);
        RAISE NOTICE 'Created sqlserver_query_stats_interval table with hypertable';
    ELSE
        RAISE NOTICE 'sqlserver_query_stats_interval already exists';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM add_retention_policy('sqlserver_query_stats_snapshot', INTERVAL '7 days', if_not_exists => TRUE);
        PERFORM add_compression_policy('sqlserver_query_stats_interval', INTERVAL '7 days', if_not_exists => TRUE);
        RAISE NOTICE 'Added retention and compression policies';
    END IF;
END $$;

-- DB-side snapshot/delta helpers (optional; app uses equivalent SQL in Go)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_proc WHERE proname = 'process_query_stats_snapshot'
    ) THEN
        CREATE OR REPLACE FUNCTION process_query_stats_snapshot(p_instance_name TEXT)
        RETURNS void LANGUAGE plpgsql AS $fn$
        BEGIN
            INSERT INTO sqlserver_query_stats_snapshot
                (capture_time, server_instance_name, database_name, login_name, client_app, query_hash, query_text,
                 total_executions, total_cpu_ms, total_elapsed_ms, total_logical_reads, total_physical_reads, total_rows, row_fingerprint)
            SELECT
                s.capture_time,
                s.server_instance_name,
                s.database_name,
                s.login_name,
                s.client_app,
                s.query_hash,
                s.query_text,
                s.total_executions,
                s.total_cpu_ms,
                s.total_elapsed_ms,
                s.total_logical_reads,
                s.total_physical_reads,
                s.total_rows,
                md5(s.total_executions::text || '-' || s.total_cpu_ms::text || '-' || s.total_elapsed_ms::text || '-' ||
                    s.total_logical_reads::text || '-' || s.total_physical_reads::text || '-' || s.total_rows::text)
            FROM sqlserver_query_stats_staging s
            WHERE s.server_instance_name = p_instance_name
            AND NOT EXISTS (
                SELECT 1
                FROM (
                    SELECT last.row_fingerprint
                    FROM sqlserver_query_stats_snapshot last
                    WHERE last.server_instance_name = s.server_instance_name
                      AND last.query_hash = s.query_hash
                      AND last.database_name IS NOT DISTINCT FROM s.database_name
                      AND last.login_name IS NOT DISTINCT FROM s.login_name
                      AND last.client_app IS NOT DISTINCT FROM s.client_app
                    ORDER BY last.capture_time DESC
                    LIMIT 1
                ) prev
                WHERE prev.row_fingerprint = md5(s.total_executions::text || '-' || s.total_cpu_ms::text || '-' || s.total_elapsed_ms::text || '-' ||
                      s.total_logical_reads::text || '-' || s.total_physical_reads::text || '-' || s.total_rows::text)
            )
            ON CONFLICT DO NOTHING;

            DELETE FROM sqlserver_query_stats_staging WHERE server_instance_name = p_instance_name;
        END;
        $fn$;
        RAISE NOTICE 'Created process_query_stats_snapshot function';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_proc WHERE proname = 'process_query_stats_delta'
    ) THEN
        CREATE OR REPLACE FUNCTION process_query_stats_delta(p_instance_name TEXT)
        RETURNS void LANGUAGE plpgsql AS $fn$
        BEGIN
            INSERT INTO sqlserver_query_stats_interval
                (bucket_start, bucket_end, server_instance_name, database_name, login_name, client_app, query_hash, query_text,
                 executions, cpu_ms, duration_ms, logical_reads, physical_reads, rows,
                 avg_cpu_ms, avg_duration_ms, avg_reads, is_reset)
            SELECT
                prev.capture_time,
                curr.capture_time,
                curr.server_instance_name,
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
                cpu_delta / NULLIF(exec_delta, 0)::numeric,
                dur_delta / NULLIF(exec_delta, 0)::numeric,
                reads_delta / NULLIF(exec_delta, 0)::numeric,
                reset
            FROM (
                SELECT
                    curr.*,
                    curr.total_executions - COALESCE(prev.total_executions, 0) AS exec_delta,
                    curr.total_cpu_ms - COALESCE(prev.total_cpu_ms, 0) AS cpu_delta,
                    curr.total_elapsed_ms - COALESCE(prev.total_elapsed_ms, 0) AS dur_delta,
                    curr.total_logical_reads - COALESCE(prev.total_logical_reads, 0) AS reads_delta,
                    curr.total_physical_reads - COALESCE(prev.total_physical_reads, 0) AS phys_delta,
                    curr.total_rows - COALESCE(prev.total_rows, 0) AS rows_delta,
                    (curr.total_executions < COALESCE(prev.total_executions, 0)
                     OR curr.total_cpu_ms < COALESCE(prev.total_cpu_ms, 0)) AS reset
                FROM sqlserver_query_stats_snapshot curr
                JOIN LATERAL (
                    SELECT total_executions, total_cpu_ms, total_elapsed_ms, total_logical_reads, total_physical_reads, total_rows, capture_time
                    FROM sqlserver_query_stats_snapshot p
                    WHERE p.server_instance_name = curr.server_instance_name
                      AND p.query_hash = curr.query_hash
                      AND p.database_name IS NOT DISTINCT FROM curr.database_name
                      AND p.login_name IS NOT DISTINCT FROM curr.login_name
                      AND p.client_app IS NOT DISTINCT FROM curr.client_app
                      AND p.capture_time < curr.capture_time
                    ORDER BY capture_time DESC
                    LIMIT 1
                ) prev ON true
                WHERE curr.server_instance_name = p_instance_name
            ) t
            WHERE exec_delta > 0 OR cpu_delta > 0 OR dur_delta > 0
            ON CONFLICT (bucket_end, query_hash, database_name, login_name, client_app, server_instance_name) DO UPDATE SET
                executions = sqlserver_query_stats_interval.executions + EXCLUDED.executions,
                cpu_ms = sqlserver_query_stats_interval.cpu_ms + EXCLUDED.cpu_ms,
                duration_ms = sqlserver_query_stats_interval.duration_ms + EXCLUDED.duration_ms,
                logical_reads = sqlserver_query_stats_interval.logical_reads + EXCLUDED.logical_reads,
                physical_reads = sqlserver_query_stats_interval.physical_reads + EXCLUDED.physical_reads,
                rows = sqlserver_query_stats_interval.rows + EXCLUDED.rows;
        END;
        $fn$;
        RAISE NOTICE 'Created process_query_stats_delta function';
    END IF;
END $$;
