-- ============================================================================
-- SQL Optima: Unified TimescaleDB Schema
-- Consolidated from: init-scripts, sql_scripts, and migrations
-- Version: 1.0.1
-- Last Updated: 2026-04-13
-- 
-- This is the SINGLE SOURCE OF TRUTH for all TimescaleDB tables.
-- All tables are idempotent (IF NOT EXISTS) and safe to run multiple times.
-- ============================================================================

-- 0. INITIALIZATION: Create application role if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sql_optima_app') THEN
        CREATE ROLE sql_optima_app WITH LOGIN PASSWORD 'optima_app_pwd';
        RAISE NOTICE 'Role [sql_optima_app] created.';
    ELSE
        RAISE NOTICE 'Role [sql_optima_app] already exists.';
    END IF;
END
$$;

-- Enable TimescaleDB extension (idempotent)
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;
-- UUID helpers for server registry IDs
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- Trigram support for fast text filtering
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ============================================================================
-- SECTION 1: CORE METRICS TABLES
-- ============================================================================

-- --------------------------------------------------------------------------
-- 1.2: SQL SERVER - Core Metrics
-- --------------------------------------------------------------------------

-- sqlserver_active_sessions — unified session snapshot
-- One row per session per collection cycle. Used by blocking detection,
-- connection stats, and health workers instead of hitting DMVs directly.
CREATE TABLE IF NOT EXISTS sqlserver_active_sessions (
    capture_timestamp       TIMESTAMPTZ     NOT NULL,
    server_id               UUID            NOT NULL,
    session_id              INT             NOT NULL,
    login_name              TEXT            NOT NULL DEFAULT '',
    host_name               TEXT            NOT NULL DEFAULT '',
    program_name            TEXT            NOT NULL DEFAULT '',
    database_name           TEXT            NOT NULL DEFAULT '',
    request_status          TEXT            NOT NULL DEFAULT '',
    wait_type               TEXT            NOT NULL DEFAULT '',
    wait_time_ms            BIGINT          NOT NULL DEFAULT 0,
    blocking_session_id     INT             NOT NULL DEFAULT 0,
    cpu_time_ms             BIGINT          NOT NULL DEFAULT 0,
    total_elapsed_ms        BIGINT          NOT NULL DEFAULT 0,
    logical_reads           BIGINT          NOT NULL DEFAULT 0,
    reads                   BIGINT          NOT NULL DEFAULT 0,
    writes                  BIGINT          NOT NULL DEFAULT 0,
    granted_query_memory_kb BIGINT          NOT NULL DEFAULT 0,
    dop                     INT             NOT NULL DEFAULT 0,
    query_hash              TEXT            NOT NULL DEFAULT '',
    query_text              TEXT            NOT NULL DEFAULT ''
);

SELECT create_hypertable('sqlserver_active_sessions', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_sessions_server_time
    ON sqlserver_active_sessions (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_sessions_blocking
    ON sqlserver_active_sessions (server_id, capture_timestamp DESC, blocking_session_id)
    WHERE blocking_session_id > 0;

SELECT add_retention_policy('sqlserver_active_sessions',
    INTERVAL '30 days', if_not_exists => TRUE);

-- sqlserver_memory_grants_detail — per-session memory grant detail
CREATE TABLE IF NOT EXISTS sqlserver_memory_grants_detail (
    capture_timestamp       TIMESTAMPTZ     NOT NULL,
    server_id               UUID            NOT NULL,
    session_id              INT             NOT NULL,
    login_name              TEXT            NOT NULL DEFAULT '',
    database_name           TEXT            NOT NULL DEFAULT '',
    query_cost              NUMERIC(19,4)   NOT NULL DEFAULT 0,
    requested_memory_kb     BIGINT          NOT NULL DEFAULT 0,
    granted_memory_kb       BIGINT          NOT NULL DEFAULT 0,
    used_memory_kb          BIGINT          NOT NULL DEFAULT 0,
    max_used_memory_kb      BIGINT          NOT NULL DEFAULT 0,
    dop                     INT             NOT NULL DEFAULT 0,
    grant_time              TIMESTAMPTZ,
    queue_time              TIMESTAMPTZ
);

SELECT create_hypertable('sqlserver_memory_grants_detail', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_grants_detail_server_time
    ON sqlserver_memory_grants_detail (server_id, capture_timestamp DESC);

SELECT add_retention_policy('sqlserver_memory_grants_detail',
    INTERVAL '30 days', if_not_exists => TRUE);

-- sqlserver_index_fragmentation — physical index health
-- Collected every 6 hours using SAMPLED mode. Only stores indexes where
-- avg_fragmentation_in_percent >= 5.0 AND page_count >= 100.
CREATE TABLE IF NOT EXISTS sqlserver_index_fragmentation (
    capture_timestamp           TIMESTAMPTZ     NOT NULL,
    server_id                   UUID            NOT NULL,
    database_name               TEXT            NOT NULL DEFAULT '',
    schema_name                 TEXT            NOT NULL DEFAULT '',
    table_name                  TEXT            NOT NULL DEFAULT '',
    index_name                  TEXT            NOT NULL DEFAULT '',
    index_id                    INT             NOT NULL DEFAULT 0,
    index_type_desc             TEXT            NOT NULL DEFAULT '',
    avg_fragmentation_pct       NUMERIC(8,2)    NOT NULL DEFAULT 0,
    page_count                  BIGINT          NOT NULL DEFAULT 0,
    avg_page_space_used_pct     NUMERIC(8,2)    NOT NULL DEFAULT 0,
    record_count                BIGINT          NOT NULL DEFAULT 0,
    fragment_count              BIGINT          NOT NULL DEFAULT 0,
    avg_fragment_size_pages     NUMERIC(8,2)    NOT NULL DEFAULT 0
);

SELECT create_hypertable('sqlserver_index_fragmentation', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_idx_frag_server_time
    ON sqlserver_index_fragmentation (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_idx_frag_db
    ON sqlserver_index_fragmentation (server_id, database_name, capture_timestamp DESC);

SELECT add_retention_policy('sqlserver_index_fragmentation',
    INTERVAL '14 days', if_not_exists => TRUE);

-- sqlserver_missing_indexes — advisor output from dm_db_missing_index_* DMVs
-- Collected every 6 hours. Only stores top 100 by improvement_score.
CREATE TABLE IF NOT EXISTS sqlserver_missing_indexes (
    capture_timestamp           TIMESTAMPTZ     NOT NULL,
    server_id                   UUID            NOT NULL,
    database_name               TEXT            NOT NULL DEFAULT '',
    schema_name                 TEXT            NOT NULL DEFAULT '',
    table_name                  TEXT            NOT NULL DEFAULT '',
    equality_columns            TEXT            NOT NULL DEFAULT '',
    inequality_columns          TEXT            NOT NULL DEFAULT '',
    included_columns            TEXT            NOT NULL DEFAULT '',
    user_seeks                  BIGINT          NOT NULL DEFAULT 0,
    user_scans                  BIGINT          NOT NULL DEFAULT 0,
    avg_total_user_cost         NUMERIC(19,4)   NOT NULL DEFAULT 0,
    avg_user_impact             NUMERIC(8,2)    NOT NULL DEFAULT 0,
    improvement_score           NUMERIC(19,4)   NOT NULL DEFAULT 0,
    last_user_seek              TIMESTAMPTZ,
    last_user_scan              TIMESTAMPTZ
);

SELECT create_hypertable('sqlserver_missing_indexes', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_missing_idx_server_time
    ON sqlserver_missing_indexes (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_missing_idx_db
    ON sqlserver_missing_indexes (server_id, database_name, capture_timestamp DESC);

SELECT add_retention_policy('sqlserver_missing_indexes',
    INTERVAL '14 days', if_not_exists => TRUE);

-- SQL Server System Metrics (main table)
CREATE TABLE IF NOT EXISTS sqlserver_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    avg_cpu_load DOUBLE PRECISION,
    memory_usage DOUBLE PRECISION,
    active_users INTEGER,
    total_locks BIGINT,
    deadlocks BIGINT,
    data_disk_mb DOUBLE PRECISION,
    log_disk_mb DOUBLE PRECISION,
    free_disk_mb DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_metrics', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_metrics_server ON sqlserver_metrics (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_metrics', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_metrics', INTERVAL '90 days', if_not_exists => TRUE);

-- SQL Server CPU History
CREATE TABLE IF NOT EXISTS sqlserver_cpu_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    sql_process DOUBLE PRECISION,
    system_idle DOUBLE PRECISION,
    other_process DOUBLE PRECISION,
    scheduler_count INT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_cpu_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sqlserver_cpu_dedup ON sqlserver_cpu_history (capture_timestamp, server_id);
CREATE INDEX IF NOT EXISTS idx_sqlserver_cpu_server ON sqlserver_cpu_history (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_cpu_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_cpu_history', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_cpu_history', INTERVAL '90 days', if_not_exists => TRUE);

-- SQL Server Wait Statistics History
CREATE TABLE IF NOT EXISTS sqlserver_wait_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    wait_type TEXT,
    wait_time_ms_total BIGINT,
    disk_read_ms_per_sec DOUBLE PRECISION,
    blocking_ms_per_sec DOUBLE PRECISION,
    parallelism_ms_per_sec DOUBLE PRECISION,
    other_ms_per_sec DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_wait_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wait_server ON sqlserver_wait_history (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wait_type ON sqlserver_wait_history (wait_type);
ALTER TABLE sqlserver_wait_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_wait_history', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_wait_history', INTERVAL '90 days', if_not_exists => TRUE);

-- SQL Server Connection History
CREATE TABLE IF NOT EXISTS sqlserver_connection_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    login_name TEXT,
    database_name TEXT,
    active_connections INTEGER,
    active_requests INTEGER,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_connection_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_conn_server ON sqlserver_connection_history (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_connection_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_connection_history', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_connection_history', INTERVAL '90 days', if_not_exists => TRUE);

-- SQL Server Lock History
CREATE TABLE IF NOT EXISTS sqlserver_lock_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT,
    total_locks BIGINT,
    deadlocks BIGINT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_lock_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lock_server ON sqlserver_lock_history (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_lock_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_lock_history', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_lock_history', INTERVAL '90 days', if_not_exists => TRUE);

-- SQL Server Disk Usage History
CREATE TABLE IF NOT EXISTS sqlserver_disk_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT,
    data_mb DOUBLE PRECISION,
    log_mb DOUBLE PRECISION,
    free_mb DOUBLE PRECISION,
    delta_data_mb DOUBLE PRECISION DEFAULT 0,
    delta_log_mb  DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
ALTER TABLE sqlserver_disk_history
    ADD COLUMN IF NOT EXISTS delta_data_mb DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delta_log_mb  DOUBLE PRECISION DEFAULT 0;
SELECT create_hypertable('sqlserver_disk_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_disk_server ON sqlserver_disk_history (server_id, capture_timestamp DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sqlserver_disk_history_dedup
    ON sqlserver_disk_history (server_id, database_name, capture_timestamp);
ALTER TABLE sqlserver_disk_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_disk_history', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_disk_history', INTERVAL '90 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 1.2.x: POSTGRES - Advanced (Contention / IO / Config drift)
-- --------------------------------------------------------------------------

-- Wait event snapshots (contention taxonomy) from pg_stat_activity
CREATE TABLE IF NOT EXISTS postgres_wait_event_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    wait_event_type TEXT,
    wait_event TEXT,
    sessions_count INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_wait_event_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_waits_server_time ON postgres_wait_event_stats (server_id, capture_timestamp DESC);
ALTER TABLE postgres_wait_event_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,wait_event_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_wait_event_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_wait_event_stats', INTERVAL '30 days', if_not_exists => TRUE);

-- Per-database IO counters from pg_stat_database (UI computes deltas)
CREATE TABLE IF NOT EXISTS postgres_db_io_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT NOT NULL,
    blks_read BIGINT DEFAULT 0,
    blks_hit BIGINT DEFAULT 0,
    temp_files BIGINT DEFAULT 0,
    temp_bytes BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_db_io_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_io_server_time ON postgres_db_io_stats (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_io_db_time ON postgres_db_io_stats (server_id, database_name, capture_timestamp DESC);
ALTER TABLE postgres_db_io_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_db_io_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_db_io_stats', INTERVAL '30 days', if_not_exists => TRUE);

-- Curated pg_settings snapshot for drift tracking
CREATE TABLE IF NOT EXISTS postgres_settings_snapshot (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    name TEXT NOT NULL,
    setting TEXT,
    unit TEXT,
    source TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_settings_snapshot', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_settings_server_time ON postgres_settings_snapshot (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_settings_name ON postgres_settings_snapshot (server_id, name, capture_timestamp DESC);
ALTER TABLE postgres_settings_snapshot SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_settings_snapshot', INTERVAL '30 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_settings_snapshot', INTERVAL '90 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 1.3: SQL SERVER - Enterprise Metrics (AG, Throughput, etc.)
-- --------------------------------------------------------------------------

-- SQL Server Database Throughput
CREATE TABLE IF NOT EXISTS sqlserver_database_throughput (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT NOT NULL,
    user_seeks BIGINT DEFAULT 0,
    user_scans BIGINT DEFAULT 0,
    user_lookups BIGINT DEFAULT 0,
    user_writes BIGINT DEFAULT 0,
    total_reads BIGINT DEFAULT 0,
    total_writes BIGINT DEFAULT 0,
    tps DOUBLE PRECISION DEFAULT 0,
    batch_requests_per_sec DOUBLE PRECISION DEFAULT 0,
    reads BIGINT DEFAULT 0,
    writes BIGINT DEFAULT 0,
    bytes_read BIGINT DEFAULT 0,
    bytes_written BIGINT DEFAULT 0,
    read_latency_ms BIGINT DEFAULT 0,
    write_latency_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

-- Ensure all columns exist (migration for existing tables)
DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_database_throughput' AND column_name='reads') THEN
        ALTER TABLE sqlserver_database_throughput ADD COLUMN reads BIGINT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_database_throughput' AND column_name='writes') THEN
        ALTER TABLE sqlserver_database_throughput ADD COLUMN writes BIGINT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_database_throughput' AND column_name='bytes_read') THEN
        ALTER TABLE sqlserver_database_throughput ADD COLUMN bytes_read BIGINT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_database_throughput' AND column_name='bytes_written') THEN
        ALTER TABLE sqlserver_database_throughput ADD COLUMN bytes_written BIGINT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_database_throughput' AND column_name='read_latency_ms') THEN
        ALTER TABLE sqlserver_database_throughput ADD COLUMN read_latency_ms BIGINT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_database_throughput' AND column_name='write_latency_ms') THEN
        ALTER TABLE sqlserver_database_throughput ADD COLUMN write_latency_ms BIGINT DEFAULT 0;
    END IF;
END $$;

SELECT create_hypertable('sqlserver_database_throughput', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_db_throughput_server_time ON sqlserver_database_throughput (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_db_throughput_db ON sqlserver_database_throughput (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_database_throughput SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_database_throughput', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_database_throughput', INTERVAL '90 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_database_throughput IS 'Tracks database-level throughput metrics including TPS, batch requests, and I/O statistics';

-- SQL Server Availability Group Health (LEGACY - use monitor.sqlserver_ha_replica_state for new collectors)
CREATE TABLE IF NOT EXISTS sqlserver_ag_health (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    ag_name TEXT,
    replica_server_name TEXT,
    database_name TEXT,
    replica_role TEXT,
    operational_state TEXT,
    connected_state TEXT,
    synchronization_state TEXT,
    synchronization_state_desc TEXT,
    is_primary_replica BOOLEAN,
    log_send_queue_kb BIGINT DEFAULT 0,
    redo_queue_kb BIGINT DEFAULT 0,
    log_send_rate_kb BIGINT DEFAULT 0,
    redo_rate_kb BIGINT DEFAULT 0,
    last_sent_time TIMESTAMPTZ,
    last_received_time TIMESTAMPTZ,
    last_hardened_time TIMESTAMPTZ,
    last_redone_time TIMESTAMPTZ,
    secondary_lag_seconds BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_ag_health', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_ag_health_server_time ON sqlserver_ag_health (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ag_health_ag_name ON sqlserver_ag_health (ag_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ag_health_db ON sqlserver_ag_health (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_ag_health SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,ag_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_ag_health', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_ag_health', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_ag_health IS 'Tracks AlwaysOn Availability Group health metrics including sync state and queue sizes';

-- --------------------------------------------------------------------------
-- 1.3.1: SQL SERVER - HA and Replication (monitor schema)
-- --------------------------------------------------------------------------
CREATE SCHEMA IF NOT EXISTS monitor;

CREATE TABLE IF NOT EXISTS monitor.sqlserver_feature_detection (
    server_id              uuid        NOT NULL,
    capture_timestamp      timestamptz NOT NULL,
    ha_enabled             boolean     NOT NULL DEFAULT false,
    replication_enabled    boolean     NOT NULL DEFAULT false,
    ag_enabled             boolean     NOT NULL DEFAULT false,
    fci_enabled            boolean     NOT NULL DEFAULT false,
    log_shipping_enabled   boolean     NOT NULL DEFAULT false,
    mirroring_enabled      boolean     NOT NULL DEFAULT false,
    replication_types      text[]      NOT NULL DEFAULT '{}',
    PRIMARY KEY (server_id, capture_timestamp)
);

SELECT create_hypertable('monitor.sqlserver_feature_detection', 'capture_timestamp', if_not_exists => true);
ALTER TABLE monitor.sqlserver_feature_detection SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_feature_detection', INTERVAL '7 days', if_not_exists => true);
SELECT add_retention_policy('monitor.sqlserver_feature_detection', INTERVAL '90 days', if_not_exists => true);

-- 2. HA Replica State Snapshot (30 s interval)
CREATE TABLE IF NOT EXISTS monitor.sqlserver_ha_replica_state (
    capture_timestamp           timestamptz NOT NULL,
    server_id                   uuid        NOT NULL,
    ag_name                     text        NOT NULL DEFAULT '',
    replica_server_name         text        NOT NULL DEFAULT '',
    role_desc                   text        NOT NULL DEFAULT 'UNKNOWN',
    synchronization_state_desc  text        NOT NULL DEFAULT 'UNKNOWN',
    synchronization_health_desc text        NOT NULL DEFAULT 'UNKNOWN',
    availability_mode_desc      text        NOT NULL DEFAULT 'UNKNOWN',
    log_send_queue_kb           bigint      NOT NULL DEFAULT 0,
    redo_queue_kb               bigint      NOT NULL DEFAULT 0,
    log_send_rate_kbps          bigint      NOT NULL DEFAULT 0,
    redo_rate_kbps              bigint      NOT NULL DEFAULT 0,
    last_commit_time            timestamptz,
    secondary_lag_seconds       bigint      NOT NULL DEFAULT 0,
    connected_state_desc        text        NOT NULL DEFAULT 'UNKNOWN',
    is_failover_ready           boolean     NOT NULL DEFAULT false,
    long_running_tx_count       int         NOT NULL DEFAULT 0,
    quorum_state_desc           text        NOT NULL DEFAULT 'UNKNOWN',
    PRIMARY KEY (server_id, capture_timestamp, ag_name, replica_server_name)
);

SELECT create_hypertable('monitor.sqlserver_ha_replica_state', 'capture_timestamp', if_not_exists => true);
SELECT add_retention_policy('monitor.sqlserver_ha_replica_state', INTERVAL '30 days', if_not_exists => true);

ALTER TABLE monitor.sqlserver_ha_replica_state SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,ag_name,replica_server_name',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_ha_replica_state', INTERVAL '7 days', if_not_exists => true);

-- 3. HA Database State Snapshot (30 s interval)
CREATE TABLE IF NOT EXISTS monitor.sqlserver_ha_database_state (
    capture_timestamp           timestamptz NOT NULL,
    server_id                   uuid        NOT NULL,
    ag_name                     text        NOT NULL DEFAULT '',
    database_name               text        NOT NULL DEFAULT '',
    replica_server_name         text        NOT NULL DEFAULT '',
    synchronization_state_desc  text        NOT NULL DEFAULT 'UNKNOWN',
    is_suspended                boolean     NOT NULL DEFAULT false,
    log_send_queue_kb           bigint      NOT NULL DEFAULT 0,
    redo_queue_kb               bigint      NOT NULL DEFAULT 0,
    last_commit_time            timestamptz,
    backup_fresh_ok             boolean     NOT NULL DEFAULT false,
    PRIMARY KEY (server_id, capture_timestamp, ag_name, database_name, replica_server_name)
);

SELECT create_hypertable('monitor.sqlserver_ha_database_state', 'capture_timestamp', if_not_exists => true);
ALTER TABLE monitor.sqlserver_ha_database_state SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,ag_name,database_name',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_ha_database_state', INTERVAL '7 days', if_not_exists => true);
SELECT add_retention_policy('monitor.sqlserver_ha_database_state', INTERVAL '14 days', if_not_exists => true);

-- 4. Failover Events
CREATE TABLE IF NOT EXISTS monitor.sqlserver_ha_failover_events (
    capture_timestamp           timestamptz NOT NULL,
    server_id                   uuid        NOT NULL,
    ag_name                     text        NOT NULL,
    previous_primary            text        NOT NULL,
    new_primary                 text        NOT NULL,
    failover_type               text        NOT NULL, -- AUTOMATIC | MANUAL
    failover_duration_seconds   int         NOT NULL DEFAULT 0,
    PRIMARY KEY (server_id, capture_timestamp, ag_name, previous_primary, new_primary)
);

SELECT create_hypertable('monitor.sqlserver_ha_failover_events', 'capture_timestamp', if_not_exists => true);
ALTER TABLE monitor.sqlserver_ha_failover_events SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,ag_name',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_ha_failover_events', INTERVAL '7 days', if_not_exists => true);
SELECT add_retention_policy('monitor.sqlserver_ha_failover_events', INTERVAL '365 days', if_not_exists => true);

-- 5. AG Cluster Info (non-hypertable, cluster-scoped, low write frequency)
CREATE TABLE IF NOT EXISTS monitor.sqlserver_ag_cluster_info (
    capture_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    server_id         UUID        NOT NULL,
    cluster_name      TEXT,
    quorum_type       TEXT,
    quorum_state      TEXT,
    members_json      JSONB
);
CREATE INDEX IF NOT EXISTS idx_ag_cluster_server_time
    ON monitor.sqlserver_ag_cluster_info (server_id, capture_timestamp DESC);

-- 6. Replication Topology (15 min interval)
CREATE TABLE IF NOT EXISTS monitor.sqlserver_replication_topology (
    capture_timestamp   timestamptz NOT NULL,
    server_id           uuid        NOT NULL,
    publisher           text        NOT NULL,
    subscriber          text        NOT NULL,
    publication         text        NOT NULL,
    publication_db      text        NOT NULL,
    subscriber_db       text        NOT NULL,
    replication_type    text        NOT NULL, -- Transactional | Merge | Snapshot
    sync_type           text        NOT NULL,
    agent_status        text        NOT NULL,
    last_sync_time      timestamptz,
    PRIMARY KEY (server_id, capture_timestamp, publisher, subscriber, publication)
);

SELECT create_hypertable('monitor.sqlserver_replication_topology', 'capture_timestamp', if_not_exists => true);
ALTER TABLE monitor.sqlserver_replication_topology SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,publisher,publication',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_replication_topology', INTERVAL '7 days', if_not_exists => true);
SELECT add_retention_policy('monitor.sqlserver_replication_topology', INTERVAL '90 days', if_not_exists => true);

-- 6. Replication Latency (1 min interval)
CREATE TABLE IF NOT EXISTS monitor.sqlserver_replication_latency (
    capture_timestamp       timestamptz NOT NULL,
    server_id               uuid        NOT NULL,
    publisher               text        NOT NULL,
    subscriber              text        NOT NULL,
    publication             text        NOT NULL,
    latency_seconds         bigint      NOT NULL DEFAULT 0,
    undistributed_commands  bigint      NOT NULL DEFAULT 0,
    delivery_rate_cmds_sec  bigint      NOT NULL DEFAULT 0,
    status                  text        NOT NULL,
    PRIMARY KEY (server_id, capture_timestamp, publisher, subscriber, publication)
);

SELECT create_hypertable('monitor.sqlserver_replication_latency', 'capture_timestamp', if_not_exists => true);
ALTER TABLE monitor.sqlserver_replication_latency SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,publisher,subscriber',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_replication_latency', INTERVAL '7 days', if_not_exists => true);
SELECT add_retention_policy('monitor.sqlserver_replication_latency', INTERVAL '30 days', if_not_exists => true);

-- 7. Replication Articles (15 min interval)
CREATE TABLE IF NOT EXISTS monitor.sqlserver_replication_articles (
    capture_timestamp   timestamptz NOT NULL,
    server_id           uuid        NOT NULL,
    publication         text        NOT NULL,
    database_name       text        NOT NULL,
    schema_name         text        NOT NULL,
    table_name          text        NOT NULL,
    subscriber          text        NOT NULL,
    rows_per_sec        bigint      NOT NULL DEFAULT 0,
    latency_seconds     bigint      NOT NULL DEFAULT 0,
    conflicts_detected  bigint      NOT NULL DEFAULT 0,
    status              text        NOT NULL,
    PRIMARY KEY (server_id, capture_timestamp, publication, database_name, schema_name, table_name, subscriber)
);

SELECT create_hypertable('monitor.sqlserver_replication_articles', 'capture_timestamp', if_not_exists => true);
ALTER TABLE monitor.sqlserver_replication_articles SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,publication,database_name',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_replication_articles', INTERVAL '7 days', if_not_exists => true);
SELECT add_retention_policy('monitor.sqlserver_replication_articles', INTERVAL '30 days', if_not_exists => true);

-- ---------------------------------------------------------------------------
-- CONTINUOUS AGGREGATES (HA & Replication)
-- ---------------------------------------------------------------------------

-- 8. RPO 1-minute rollup
CREATE MATERIALIZED VIEW IF NOT EXISTS monitor.sqlserver_rpo_1min
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_id,
    MAX(secondary_lag_seconds)                AS rpo_seconds,
    AVG(secondary_lag_seconds)                AS avg_rpo_seconds,
    COUNT(DISTINCT replica_server_name)       AS replica_count
FROM monitor.sqlserver_ha_replica_state
GROUP BY bucket, server_id
WITH NO DATA;

SELECT add_continuous_aggregate_policy('monitor.sqlserver_rpo_1min',
    start_offset => INTERVAL '1 hour', end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute', if_not_exists => true);

-- 9. RTO 1-minute rollup
CREATE MATERIALIZED VIEW IF NOT EXISTS monitor.sqlserver_rto_1min
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_id,
    MAX(redo_queue_kb)                        AS max_redo_queue_kb,
    AVG(redo_rate_kbps)                       AS avg_redo_rate_kbps,
    MAX(secondary_lag_seconds + 30)           AS estimated_rto_seconds
FROM monitor.sqlserver_ha_replica_state
GROUP BY bucket, server_id
WITH NO DATA;

SELECT add_continuous_aggregate_policy('monitor.sqlserver_rto_1min',
    start_offset => INTERVAL '1 hour', end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute', if_not_exists => true);

-- 10. Replication Backlog 1-minute rollup
CREATE MATERIALIZED VIEW IF NOT EXISTS monitor.sqlserver_replication_backlog_1min
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_id,
    publisher,
    subscriber,
    publication,
    MAX(undistributed_commands)               AS peak_backlog,
    AVG(undistributed_commands)               AS avg_backlog,
    MAX(latency_seconds)                      AS max_latency_seconds
FROM monitor.sqlserver_replication_latency
GROUP BY bucket, server_id, publisher, subscriber, publication
WITH NO DATA;

SELECT add_continuous_aggregate_policy('monitor.sqlserver_replication_backlog_1min',
    start_offset => INTERVAL '1 hour', end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute', if_not_exists => true);

-- 11. Collector Configurations (HA & Replication)
INSERT INTO optima_collector_configs (collector_name, module, frequency_seconds, is_active) VALUES
('sqlserver_ha_discovery', 'SQLSERVER', 900, true),
('sqlserver_ha_health', 'SQLSERVER', 30, true),
('sqlserver_replication_performance', 'SQLSERVER', 60, true),
('sqlserver_replication_topology', 'SQLSERVER', 900, true)
ON CONFLICT (collector_name) DO UPDATE SET
    frequency_seconds = EXCLUDED.frequency_seconds,
    is_active = true;

-- Query V2 pipeline jobs (consumed by startQueryV2Collector / RunCycle)
INSERT INTO optima_collector_configs (collector_name, module, frequency_seconds, is_active) VALUES
('sqlserver_query_snapshot',     'SQLSERVER', 60, true),
('sqlserver_session_enrichment', 'SQLSERVER', 30, true),
('pg_queries_v2',                'Postgres',  60, true)
ON CONFLICT (collector_name) DO UPDATE SET
    frequency_seconds = EXCLUDED.frequency_seconds,
    is_active = true;

-- --------------------------------------------------------------------------
-- 1.4: SQL SERVER - Agent Jobs
-- --------------------------------------------------------------------------

-- SQL Server Agent Jobs Summary
CREATE TABLE IF NOT EXISTS sqlserver_job_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    total_jobs INTEGER,
    enabled_jobs INTEGER,
    disabled_jobs INTEGER,
    running_jobs INTEGER,
    failed_jobs_24h INTEGER,
    critical_jobs_disabled INTEGER DEFAULT 0,
    error_message TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT sqlserver_job_metrics_unique UNIQUE (capture_timestamp, server_id)
);
SELECT create_hypertable('sqlserver_job_metrics', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_job_server ON sqlserver_job_metrics (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_job_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_job_metrics', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Job Details
CREATE TABLE IF NOT EXISTS sqlserver_job_details (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    job_name TEXT NOT NULL,
    job_category TEXT,
    job_description TEXT,
    job_enabled BOOLEAN,
    job_owner TEXT,
    created_date TEXT,
    current_status TEXT,
    last_run_date INTEGER,
    last_run_time INTEGER,
    last_run_status TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_job_details', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_jobdetail_server ON sqlserver_job_details (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_jobdetail_name ON sqlserver_job_details (job_name);
ALTER TABLE sqlserver_job_details SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,job_name,job_category',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_job_details', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Agent Schedules
CREATE TABLE IF NOT EXISTS sqlserver_agent_schedules (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    job_name TEXT NOT NULL,
    next_run_datetime TEXT,
    job_enabled BOOLEAN,
    schedule_name TEXT,
    status TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_agent_schedules', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_sched_server ON sqlserver_agent_schedules (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_agent_schedules SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,job_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_agent_schedules', INTERVAL '7 days', if_not_exists => TRUE);

-- SQL Server Job Failures
CREATE TABLE IF NOT EXISTS sqlserver_job_failures (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    job_name TEXT,
    step_name TEXT,
    error_message TEXT,
    run_date INTEGER,
    run_time INTEGER,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_job_failures', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_jobfail_server ON sqlserver_job_failures (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_job_failures SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,job_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_job_failures', INTERVAL '7 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- SQL SERVER - CPU Enhancements (merged from 006_cpu_enhancement.sql)
-- --------------------------------------------------------------------------

-- SQL Server Server Properties (hardware / static-ish)
CREATE TABLE IF NOT EXISTS sqlserver_server_properties (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    cpu_count INTEGER DEFAULT 0,
    hyperthread_ratio INTEGER DEFAULT 0,
    socket_count INTEGER DEFAULT 0,
    cores_per_socket INTEGER DEFAULT 0,
    physical_memory_gb DOUBLE PRECISION DEFAULT 0,
    virtual_memory_gb DOUBLE PRECISION DEFAULT 0,
    cpu_type TEXT,
    hyperthread_enabled BOOLEAN DEFAULT FALSE,
    numa_nodes INTEGER DEFAULT 0,
    max_workers_count INTEGER DEFAULT 0,
    properties_hash TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_server_properties', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_server_props_server_time ON sqlserver_server_properties (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_server_properties SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_server_properties', INTERVAL '30 days', if_not_exists => TRUE);

-- Idempotent additions for existing deployments
ALTER TABLE sqlserver_server_properties ADD COLUMN IF NOT EXISTS sqlserver_start_time TIMESTAMPTZ;
ALTER TABLE sqlserver_server_properties ADD COLUMN IF NOT EXISTS ms_ticks             BIGINT DEFAULT 0;
ALTER TABLE sqlserver_server_properties ADD COLUMN IF NOT EXISTS scheduler_count      INT    DEFAULT 0;

-- --------------------------------------------------------------------------
-- SQL SERVER - DBA Homepage (Phase 2)
-- --------------------------------------------------------------------------

-- Risk & Health strip signals (computed from collectors)
CREATE TABLE IF NOT EXISTS sqlserver_risk_health (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    blocking_sessions INTEGER DEFAULT 0,
    memory_grants_pending INTEGER DEFAULT 0,
    failed_logins_5m INTEGER DEFAULT 0,
    tempdb_used_percent DOUBLE PRECISION DEFAULT 0,
    max_log_db_name TEXT DEFAULT '',
    max_log_used_percent DOUBLE PRECISION DEFAULT 0,
    ple DOUBLE PRECISION DEFAULT 0,
    compilations_per_sec DOUBLE PRECISION DEFAULT 0,
    batch_requests_per_sec DOUBLE PRECISION DEFAULT 0,
    buffer_cache_hit_ratio DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_risk_health', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_risk_health_server_time ON sqlserver_risk_health (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_risk_health SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_risk_health', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_risk_health', INTERVAL '30 days', if_not_exists => TRUE);

-- SQL Server CPU Scheduler Stats (pressure signals)
CREATE TABLE IF NOT EXISTS sqlserver_cpu_scheduler_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    max_workers_count INTEGER DEFAULT 0,
    scheduler_count INTEGER DEFAULT 0,
    cpu_count INTEGER DEFAULT 0,
    total_runnable_tasks_count INTEGER DEFAULT 0,
    total_work_queue_count BIGINT DEFAULT 0,
    total_current_workers_count INTEGER DEFAULT 0,
    active_workers_count INTEGER DEFAULT 0,
    pending_disk_io_count INTEGER DEFAULT 0,
    avg_runnable_tasks_count DOUBLE PRECISION DEFAULT 0,
    total_active_request_count INTEGER DEFAULT 0,
    total_queued_request_count INTEGER DEFAULT 0,
    total_blocked_task_count INTEGER DEFAULT 0,
    total_active_parallel_thread_count BIGINT DEFAULT 0,
    runnable_request_count INTEGER DEFAULT 0,
    total_request_count INTEGER DEFAULT 0,
    runnable_percent DOUBLE PRECISION DEFAULT 0,
    worker_thread_exhaustion_warning BOOLEAN DEFAULT FALSE,
    runnable_tasks_warning BOOLEAN DEFAULT FALSE,
    blocked_tasks_warning BOOLEAN DEFAULT FALSE,
    queued_requests_warning BOOLEAN DEFAULT FALSE,
    total_physical_memory_kb BIGINT DEFAULT 0,
    available_physical_memory_kb BIGINT DEFAULT 0,
    system_memory_state_desc TEXT,
    physical_memory_pressure_warning BOOLEAN DEFAULT FALSE,
    total_node_count INTEGER DEFAULT 0,
    nodes_online_count INTEGER DEFAULT 0,
    offline_cpu_count INTEGER DEFAULT 0,
    offline_cpu_warning BOOLEAN DEFAULT FALSE,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_cpu_scheduler_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_cpu_scheduler_server_time ON sqlserver_cpu_scheduler_stats (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_cpu_scheduler_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_cpu_scheduler_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_cpu_scheduler_stats', INTERVAL '90 days', if_not_exists => TRUE);

-- Resource Governor Scheduler Workload Groups (written by LogSchedulerWG)
CREATE TABLE IF NOT EXISTS sqlserver_scheduler_wg (
    capture_timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    server_id           UUID NOT NULL,
    pool_name           TEXT,
    group_name          TEXT,
    active_requests     BIGINT,
    queued_requests     BIGINT,
    cpu_usage_percent   NUMERIC
);
SELECT create_hypertable('sqlserver_scheduler_wg', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_scheduler_wg_server_ts
    ON sqlserver_scheduler_wg (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_scheduler_wg SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_scheduler_wg', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_scheduler_wg', INTERVAL '30 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- SQL SERVER - Long Running Queries (merged from 03_add_long_running_queries.sql)
-- --------------------------------------------------------------------------

-- SQL Server Query Dictionary (Normalization)
-- public.sqlserver_query_dictionary: UNIMPLEMENTED (Reserved for future ontology mapping)
CREATE TABLE IF NOT EXISTS sqlserver_query_dictionary (
    server_id UUID NOT NULL,
    query_hash BIGINT NOT NULL,
    query_text TEXT,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_sqlserver_query_dictionary ON sqlserver_query_dictionary (server_id, query_hash);

CREATE TABLE IF NOT EXISTS sqlserver_long_running_queries (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    session_id INTEGER NOT NULL,
    request_id INTEGER,
    database_name TEXT,
    login_name TEXT,
    host_name TEXT,
    program_name TEXT,
    query_hash BIGINT,
    wait_type TEXT,
    blocking_session_id INTEGER,
    status TEXT,
    cpu_time_ms BIGINT DEFAULT 0,
    total_elapsed_time_ms BIGINT DEFAULT 0,
    reads BIGINT DEFAULT 0,
    writes BIGINT DEFAULT 0,
    granted_query_memory_mb INTEGER DEFAULT 0,
    row_count BIGINT DEFAULT 0,
    percent_complete TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_long_running_queries', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_server_time ON sqlserver_long_running_queries (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_database ON sqlserver_long_running_queries (database_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_queryhash ON sqlserver_long_running_queries (server_id, query_hash, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_lrq_blocking ON sqlserver_long_running_queries (blocking_session_id) WHERE blocking_session_id > 0;
ALTER TABLE sqlserver_long_running_queries SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_long_running_queries', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_long_running_queries', INTERVAL '30 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 1.5: SQL SERVER - Advanced Enterprise Metrics
-- --------------------------------------------------------------------------

-- --------------------------------------------------------------------------
-- SQL SERVER - Performance Debt / Maintenance & Risk (hourly snapshot)
-- --------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS sqlserver_performance_debt_findings (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT NOT NULL DEFAULT 'master',
    section TEXT NOT NULL,              -- Index Health | Statistics Health | Storage & Growth | Backup & Recovery | SQL Agent | Engine Config
    finding_type TEXT NOT NULL,         -- e.g. unused_index | missing_index | index_fragmentation | stale_stats | vlf_high | backup_age | job_failed | config_risk
    severity TEXT NOT NULL,             -- CRITICAL | WARNING | INFO
    title TEXT NOT NULL,
    object_name TEXT DEFAULT '',
    object_type TEXT DEFAULT '',        -- table | index | stats | database | job | config
    finding_key TEXT NOT NULL,          -- stable identifier for grouping (e.g. db.schema.table:index)
    impact_score DOUBLE PRECISION DEFAULT 0,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,  -- metric fields (updates, reads, frag%, vlf_count, etc.)
    recommendation TEXT DEFAULT '',
    fix_script TEXT DEFAULT '',
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_performance_debt_findings', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_perfdebt_server_time ON sqlserver_performance_debt_findings (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_perfdebt_server_section_time ON sqlserver_performance_debt_findings (server_id, section, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_perfdebt_server_db_type ON sqlserver_performance_debt_findings (server_id, database_name, finding_type);
CREATE INDEX IF NOT EXISTS idx_perfdebt_server_findingkey ON sqlserver_performance_debt_findings (server_id, database_name, finding_key, capture_timestamp DESC);
ALTER TABLE sqlserver_performance_debt_findings SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_performance_debt_findings', INTERVAL '30 days', if_not_exists => TRUE);

-- Latch Wait Statistics
CREATE TABLE IF NOT EXISTS sqlserver_latch_waits (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    wait_type TEXT NOT NULL,
    waiting_tasks_count BIGINT DEFAULT 0,
    wait_time_ms BIGINT DEFAULT 0,
    signal_wait_time_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_latch_waits', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_latch_waits_server_time ON sqlserver_latch_waits (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_latch_waits_type ON sqlserver_latch_waits (wait_type, capture_timestamp DESC);
ALTER TABLE sqlserver_latch_waits SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_latch_waits', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_latch_waits', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_latch_waits IS 'Tracks latch wait statistics for internal synchronization objects';

-- --------------------------------------------------------------------------
-- SQL SERVER - Memory Performance Analyzer (Timescale-backed)
-- --------------------------------------------------------------------------

-- Memory Metrics (single-row per scrape; must-have production signals)
CREATE TABLE IF NOT EXISTS sqlserver_memory_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    -- OS & SQL overview
    sql_memory_used_mb BIGINT DEFAULT 0,
    sql_memory_target_mb BIGINT DEFAULT 0,
    os_total_memory_mb BIGINT DEFAULT 0,
    os_available_memory_mb BIGINT DEFAULT 0,
    process_physical_low BOOLEAN DEFAULT false,
    process_virtual_low BOOLEAN DEFAULT false,
    -- Memory grants / workspace
    memory_grants_pending INTEGER DEFAULT 0,
    active_memory_grants INTEGER DEFAULT 0,
    waiting_memory_grants INTEGER DEFAULT 0,
    granted_workspace_mb BIGINT DEFAULT 0,
    requested_workspace_mb BIGINT DEFAULT 0,
    -- Buffer pool health
    ple_seconds BIGINT DEFAULT 0,
    -- Plan cache
    plan_cache_mb BIGINT DEFAULT 0,
    -- Spill indicators (cumulative perf counters)
    sort_warnings_total BIGINT DEFAULT 0,
    hash_warnings_total BIGINT DEFAULT 0,
    -- Spill indicators (rates computed from counter deltas)
    sort_warnings_per_sec DOUBLE PRECISION DEFAULT 0,
    hash_warnings_per_sec DOUBLE PRECISION DEFAULT 0,
    os_system_memory_state VARCHAR(60) DEFAULT '',
    sql_physical_memory_in_use_mb  BIGINT  DEFAULT 0,
    sql_memory_utilization_pct     INT     DEFAULT 0,
    sql_page_fault_count           BIGINT  DEFAULT 0,
    sql_locked_page_alloc_mb       BIGINT  DEFAULT 0,
    sql_large_page_alloc_mb        BIGINT  DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
    );
    SELECT create_hypertable('sqlserver_memory_metrics', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
    CREATE INDEX IF NOT EXISTS idx_memory_metrics_server_time ON sqlserver_memory_metrics (server_id, capture_timestamp DESC);
    ALTER TABLE sqlserver_memory_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
    );
    SELECT add_compression_policy('sqlserver_memory_metrics', INTERVAL '7 days', if_not_exists => TRUE);
    SELECT add_retention_policy('sqlserver_memory_metrics', INTERVAL '90 days', if_not_exists => TRUE);
    COMMENT ON TABLE sqlserver_memory_metrics IS 'Memory Performance Analyzer: SQL vs target, OS pressure, workspace grants, PLE, plan cache, and spill indicators';

    -- sqlserver_volume_stats — per-volume storage metrics
    CREATE TABLE IF NOT EXISTS sqlserver_volume_stats (
    capture_timestamp   TIMESTAMPTZ   NOT NULL,
    server_id           UUID          NOT NULL, -- REFERENCES monitored_servers(id) -- Omitted for flexibility in schema script
    database_name       VARCHAR(128)  NOT NULL,
    logical_file_name   VARCHAR(128)  NOT NULL,
    physical_name       TEXT          NOT NULL,
    file_type           VARCHAR(10)   NOT NULL,   -- 'ROWS' or 'LOG'
    file_size_mb        FLOAT,
    volume_mount_point  VARCHAR(512)  NOT NULL,
    volume_label        VARCHAR(256),
    volume_total_gb     FLOAT,
    volume_available_gb FLOAT,
    volume_free_pct     FLOAT
    );
    SELECT create_hypertable('sqlserver_volume_stats', 'capture_timestamp', if_not_exists => TRUE);
    SELECT add_retention_policy('sqlserver_volume_stats', INTERVAL '90 days', if_not_exists => TRUE);
    CREATE INDEX IF NOT EXISTS idx_vs_server_ts
    ON sqlserver_volume_stats (server_id, capture_timestamp DESC);
    ALTER TABLE sqlserver_volume_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
    );
    SELECT add_compression_policy('sqlserver_volume_stats', INTERVAL '7 days', if_not_exists => TRUE);

    -- Page Life Expectancy history (written by ts_logger_sqlserver LogSQLServerPLE)

CREATE TABLE IF NOT EXISTS sqlserver_memory_history (
    capture_timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    server_id               UUID NOT NULL,
    page_life_expectancy    NUMERIC
);
SELECT create_hypertable('sqlserver_memory_history', 'capture_timestamp', if_not_exists => TRUE);
ALTER TABLE sqlserver_memory_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_memory_history', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_memory_history', INTERVAL '90 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_memory_history_server_ts
    ON sqlserver_memory_history (server_id, capture_timestamp DESC);

-- Buffer Pool by Database (multi-row per scrape)
CREATE TABLE IF NOT EXISTS sqlserver_buffer_pool_db (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT,
    buffer_mb BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_buffer_pool_db', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_buffer_pool_db_server_time ON sqlserver_buffer_pool_db (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_buffer_pool_db_db_time ON sqlserver_buffer_pool_db (server_id, database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_buffer_pool_db SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_buffer_pool_db', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_buffer_pool_db', INTERVAL '90 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_buffer_pool_db IS 'Memory Performance Analyzer: buffer pool usage by database (MB) per scrape';

-- Waiting Tasks
CREATE TABLE IF NOT EXISTS sqlserver_waiting_tasks (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    wait_type TEXT NOT NULL,
    resource_description TEXT,
    waiting_tasks_count BIGINT DEFAULT 0,
    wait_duration_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_waiting_tasks', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_waiting_tasks_server_time ON sqlserver_waiting_tasks (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_waiting_tasks_type ON sqlserver_waiting_tasks (wait_type, capture_timestamp DESC);
ALTER TABLE sqlserver_waiting_tasks SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,wait_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_waiting_tasks', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_waiting_tasks', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_waiting_tasks IS 'Tracks currently waiting tasks for blocking analysis';

-- Procedure Stats
CREATE TABLE IF NOT EXISTS sqlserver_procedure_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT,
    schema_name TEXT,
    object_name TEXT,
	query_hash BIGINT NOT NULL,
    execution_count BIGINT DEFAULT 0,
    total_worker_time_ms DOUBLE PRECISION DEFAULT 0,
    total_elapsed_time_ms DOUBLE PRECISION DEFAULT 0,
    total_logical_reads BIGINT DEFAULT 0,
    total_physical_reads BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_procedure_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_proc_stats_server_time ON sqlserver_procedure_stats (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_proc_stats_object ON sqlserver_procedure_stats (database_name, object_name, capture_timestamp DESC);
ALTER TABLE sqlserver_procedure_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_procedure_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_procedure_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_procedure_stats IS 'Tracks stored procedure execution statistics';

-- Spinlock Stats
CREATE TABLE IF NOT EXISTS sqlserver_spinlock_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    spinlock_type TEXT,
    collisions BIGINT DEFAULT 0,
    spins BIGINT DEFAULT 0,
    sleep_time_ms BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_spinlock_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_spinlock_stats_server_time ON sqlserver_spinlock_stats (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_spinlock_stats_type ON sqlserver_spinlock_stats (spinlock_type, capture_timestamp DESC);
ALTER TABLE sqlserver_spinlock_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,spinlock_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_spinlock_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_spinlock_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_spinlock_stats IS 'Tracks spinlock contention statistics for internal synchronization';

-- --------------------------------------------------------------------------
-- SQL SERVER - Enterprise Metrics Additions (DBA-high value)
-- --------------------------------------------------------------------------

-- Memory Grant Waiters (RESOURCE_SEMAPHORE pressure)
CREATE TABLE IF NOT EXISTS sqlserver_memory_grant_waiters (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    session_id INTEGER,
    request_id INTEGER,
    database_name TEXT,
    login_name TEXT,
    requested_memory_kb BIGINT DEFAULT 0,
    granted_memory_kb BIGINT DEFAULT 0,
    required_memory_kb BIGINT DEFAULT 0,
    wait_time_ms BIGINT DEFAULT 0,
    dop INTEGER DEFAULT 1,
    query_text TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_memory_grant_waiters', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_memgrant_waiters_server_time ON sqlserver_memory_grant_waiters (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_memgrant_waiters_server_sid ON sqlserver_memory_grant_waiters (server_id, session_id, capture_timestamp DESC);
ALTER TABLE sqlserver_memory_grant_waiters SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_memory_grant_waiters', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_memory_grant_waiters', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_memory_grant_waiters IS 'Tracks memory grant waiters (grant_time IS NULL) for diagnosing workspace memory pressure';

-- TempDB Top Consumers (per-session)
CREATE TABLE IF NOT EXISTS sqlserver_tempdb_top_consumers (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    session_id INTEGER,
    database_name TEXT,
    login_name TEXT,
    host_name TEXT,
    program_name TEXT,
    tempdb_mb DOUBLE PRECISION DEFAULT 0,
    user_objects_mb DOUBLE PRECISION DEFAULT 0,
    internal_objects_mb DOUBLE PRECISION DEFAULT 0,
    query_text TEXT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_tempdb_top_consumers', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_tempdb_consumers_server_time ON sqlserver_tempdb_top_consumers (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_tempdb_consumers_server_sid ON sqlserver_tempdb_top_consumers (server_id, session_id, capture_timestamp DESC);
ALTER TABLE sqlserver_tempdb_top_consumers SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_tempdb_top_consumers', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_tempdb_top_consumers', INTERVAL '14 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_tempdb_top_consumers IS 'Tracks top tempdb consumers by session for troubleshooting tempdb pressure and spills';

-- TempDB File Usage (used by Enterprise Metrics dashboard)
CREATE TABLE IF NOT EXISTS sqlserver_tempdb_files (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT,
    file_name TEXT,
    file_type TEXT,
    allocated_mb DOUBLE PRECISION DEFAULT 0,
    used_mb DOUBLE PRECISION DEFAULT 0,
    free_mb DOUBLE PRECISION DEFAULT 0,
    max_size_mb DOUBLE PRECISION DEFAULT 0,
    growth_mb DOUBLE PRECISION DEFAULT 0,
    used_percent DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_tempdb_files', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_tempdb_files_server_time ON sqlserver_tempdb_files (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_tempdb_files SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,file_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_tempdb_files', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_tempdb_files', INTERVAL '14 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_tempdb_files IS 'Tracks tempdb file-level usage for Enterprise Metrics dashboard';

-- SQL Server Database Catalog
-- Periodic snapshot of sys.databases settings per instance: state, recovery
-- model, isolation, features, ownership, and replication flags.
CREATE TABLE IF NOT EXISTS sqlserver_database_catalog (
    capture_timestamp                    TIMESTAMPTZ NOT NULL,
    server_id                            UUID        NOT NULL,
    -- identity
    database_id                          INT         NOT NULL,
    database_name                        TEXT        NOT NULL,
    create_date                          TIMESTAMPTZ,
    compatibility_level                  INT,
    collation_name                       TEXT,
    -- state & access
    state_desc                           TEXT,
    user_access_desc                     TEXT,
    is_read_only                         BOOLEAN,
    is_cleanly_shutdown                  BOOLEAN,
    -- recovery & durability
    recovery_model_desc                  TEXT,
    log_reuse_wait_desc                  TEXT,
    delayed_durability_desc              TEXT,
    target_recovery_time_in_seconds      INT,
    is_accelerated_database_recovery_on  BOOLEAN,
    -- risky auto features
    is_auto_close_on                     BOOLEAN,
    is_auto_shrink_on                    BOOLEAN,
    page_verify_option_desc              TEXT,
    -- isolation
    is_read_committed_snapshot_on        BOOLEAN,
    snapshot_isolation_state_desc        TEXT,
    -- features
    is_encrypted                         BOOLEAN,
    is_cdc_enabled                       BOOLEAN,
    is_broker_enabled                    BOOLEAN,
    is_fulltext_enabled                  BOOLEAN,
    is_memory_optimized_enabled          BOOLEAN,
    -- security / ownership
    owner_name                           TEXT,
    containment_desc                     TEXT,
    is_trustworthy_on                    BOOLEAN,
    -- replication flags
    is_published                         BOOLEAN,
    is_subscribed                        BOOLEAN,
    is_distributor                       BOOLEAN,
    -- availability group
    group_database_id                    TEXT,
    is_query_store_on                    BOOLEAN DEFAULT false
);
SELECT create_hypertable('sqlserver_database_catalog', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_db_catalog_server_time ON sqlserver_database_catalog (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_db_catalog_db ON sqlserver_database_catalog (database_name, capture_timestamp DESC);
ALTER TABLE sqlserver_database_catalog SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name'
);
SELECT add_compression_policy('sqlserver_database_catalog', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_database_catalog', INTERVAL '90 days', if_not_exists => TRUE);
COMMENT ON TABLE sqlserver_database_catalog IS 'Snapshot of SQL Server sys.databases settings: state, recovery model, isolation, features, and security configuration';

-- --------------------------------------------------------------------------
-- 1.6: POSTGRESQL - Core Metrics
-- --------------------------------------------------------------------------

-- PostgreSQL Query Statistics
CREATE TABLE IF NOT EXISTS postgres_query_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    query_id BIGINT,
    db_name    TEXT,
    username   TEXT,
    query_type CHAR(1),
    calls BIGINT DEFAULT 0,
    total_time_ms DOUBLE PRECISION DEFAULT 0,
    mean_time_ms DOUBLE PRECISION DEFAULT 0,
    rows BIGINT DEFAULT 0,
    temp_blks_read BIGINT DEFAULT 0,
    temp_blks_written BIGINT DEFAULT 0,
    blk_read_time_ms DOUBLE PRECISION DEFAULT 0,
    blk_write_time_ms DOUBLE PRECISION DEFAULT 0,
    shared_blks_hit BIGINT DEFAULT 0,
    shared_blks_read BIGINT DEFAULT 0,
    shared_blks_dirtied BIGINT DEFAULT 0,
    shared_blks_written BIGINT DEFAULT 0,
    wal_bytes NUMERIC DEFAULT 0,
    wal_records BIGINT DEFAULT 0,
    wal_fpi BIGINT DEFAULT 0,
    total_plan_time DOUBLE PRECISION DEFAULT 0,
    mean_plan_time DOUBLE PRECISION DEFAULT 0,
    plans BIGINT DEFAULT 0,
    userid OID,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_query_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_qrystat_server ON postgres_query_stats (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_postgres_qrystat_inc_reconstruction ON postgres_query_stats (server_id, query_id, capture_timestamp DESC);
ALTER TABLE postgres_query_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,query_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_query_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_query_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- PostgreSQL Throughput Metrics
CREATE TABLE IF NOT EXISTS postgres_throughput_metrics (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT,
    tps DOUBLE PRECISION DEFAULT 0,
    cache_hit_pct DOUBLE PRECISION DEFAULT 0,
    txn_delta BIGINT DEFAULT 0,
    blks_read_delta BIGINT DEFAULT 0,
    blks_hit_delta BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_throughput_metrics', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_tp_server_db ON postgres_throughput_metrics (server_id, database_name, capture_timestamp DESC);
ALTER TABLE postgres_throughput_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_throughput_metrics', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_throughput_metrics', INTERVAL '90 days', if_not_exists => TRUE);

-- PostgreSQL Connection Statistics
CREATE TABLE IF NOT EXISTS postgres_connection_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    total_connections INTEGER DEFAULT 0,
    active_connections INTEGER DEFAULT 0,
    idle_connections INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_connection_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_conn_server ON postgres_connection_stats (server_id, capture_timestamp DESC);
ALTER TABLE postgres_connection_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_connection_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_connection_stats', INTERVAL '90 days', if_not_exists => TRUE);

-- PostgreSQL Replication Statistics
CREATE TABLE IF NOT EXISTS postgres_replication_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    is_primary BOOLEAN DEFAULT false,
    cluster_state TEXT,
    max_lag_mb DOUBLE PRECISION DEFAULT 0,
    wal_gen_rate_mbps DOUBLE PRECISION DEFAULT 0,
    bgwriter_eff_pct DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_replication_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_repl_server ON postgres_replication_stats (server_id, capture_timestamp DESC);
ALTER TABLE postgres_replication_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_replication_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_replication_stats', INTERVAL '90 days', if_not_exists => TRUE);

-- PostgreSQL System Statistics (CPU, Memory, Load)
CREATE TABLE IF NOT EXISTS postgres_system_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    cpu_usage DOUBLE PRECISION,
    memory_usage DOUBLE PRECISION,
    load_1m DOUBLE PRECISION,
    load_5m DOUBLE PRECISION,
    load_15m DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_system_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_sys_server ON postgres_system_stats (server_id, capture_timestamp DESC);
ALTER TABLE postgres_system_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_system_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_system_stats', INTERVAL '90 days', if_not_exists => TRUE);

-- PostgreSQL Query Text Dimension (stores query text once per instance + queryid)
CREATE TABLE IF NOT EXISTS pgss_query_dim (
    server_id UUID NOT NULL,
    query_id BIGINT NOT NULL,
    query_text TEXT,
    db_name    TEXT,
    username   TEXT,
    app_name   TEXT,
    query_type CHAR(1),
    first_seen TIMESTAMPTZ DEFAULT NOW(),
    last_seen  TIMESTAMPTZ DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_pgss_query_dim ON pgss_query_dim (server_id, query_id);
CREATE INDEX IF NOT EXISTS idx_pgss_qdim_db_user ON pgss_query_dim (server_id, db_name, username);

-- PostgreSQL Query Stats Pre-Aggregated Per-Minute Deltas
CREATE TABLE IF NOT EXISTS pgss_delta_1m (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    query_id BIGINT NOT NULL,
    db_name    TEXT,
    username   TEXT,
    app_name   TEXT,
    query_type CHAR(1),
    calls BIGINT DEFAULT 0,
    total_exec_time DOUBLE PRECISION DEFAULT 0,
    rows BIGINT DEFAULT 0,
    shared_blks_hit BIGINT DEFAULT 0,
    shared_blks_read BIGINT DEFAULT 0,
    temp_blks_written BIGINT DEFAULT 0,
    wal_bytes NUMERIC DEFAULT 0,
    total_plan_time DOUBLE PRECISION DEFAULT 0,
    mean_exec_time DOUBLE PRECISION DEFAULT 0
);
SELECT create_hypertable('pgss_delta_1m', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pgss_delta_1m_server ON pgss_delta_1m (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pgss_delta_1m_query ON pgss_delta_1m (server_id, query_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pgss_delta_1m_db_user ON pgss_delta_1m (server_id, db_name, username, capture_timestamp DESC);
ALTER TABLE pgss_delta_1m SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,query_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('pgss_delta_1m', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('pgss_delta_1m', INTERVAL '90 days', if_not_exists => TRUE);

-- PostgreSQL index catalog snapshot (written by LogPgIndexDefinitions)
CREATE TABLE IF NOT EXISTS pg_index_definitions (
    server_id           UUID NOT NULL,
    database_name       TEXT,
    schema_name         TEXT,
    table_name          TEXT,
    index_name          TEXT,
    key_columns         TEXT,
    include_columns     TEXT,
    filter_definition   TEXT,
    is_unique           BOOLEAN,
    is_pk               BOOLEAN,
    index_type          TEXT,
    capture_timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_pg_index_definitions UNIQUE (server_id, database_name, schema_name, index_name)
);
CREATE INDEX IF NOT EXISTS idx_pg_index_definitions_server
    ON pg_index_definitions (server_id, capture_timestamp DESC);

-- Materialized Views (Updated to use server_id)
DROP MATERIALIZED VIEW IF EXISTS pgss_delta_1d CASCADE;
DROP MATERIALIZED VIEW IF EXISTS pgss_delta_1h CASCADE;

CREATE MATERIALIZED VIEW pgss_delta_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', capture_timestamp) AS bucket,
    server_id,
    query_id,
    COALESCE(db_name, '')    AS db_name,
    COALESCE(username, '')   AS username,
    COALESCE(app_name, '')   AS app_name,
    COALESCE(query_type,'O') AS query_type,
    SUM(calls)              AS calls,
    SUM(total_exec_time)    AS total_exec_time,
    SUM(rows)               AS rows,
    SUM(shared_blks_hit)    AS shared_blks_hit,
    SUM(shared_blks_read)   AS shared_blks_read,
    SUM(temp_blks_written)  AS temp_blks_written,
    SUM(wal_bytes)          AS wal_bytes,
    SUM(total_plan_time)    AS total_plan_time,
    AVG(mean_exec_time)     AS mean_exec_time
FROM pgss_delta_1m
GROUP BY time_bucket('1 hour', capture_timestamp), server_id, query_id,
         db_name, username, app_name, query_type
WITH NO DATA;

SELECT add_continuous_aggregate_policy('pgss_delta_1h',
    start_offset  => INTERVAL '3 hours',
    end_offset    => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);
SELECT add_retention_policy('pgss_delta_1h', INTERVAL '90 days', if_not_exists => TRUE);

-- PostgreSQL Query Stats Per-Day Deltas (Chained from pgss_delta_1h)
-- Dependency: pgss_delta_1h must refresh before this view to ensure data availability.
-- Scheduling: end_offset => INTERVAL '1 day' ensures it only materializes complete days.
CREATE MATERIALIZED VIEW pgss_delta_1d
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', bucket) AS bucket,
    server_id,
    query_id,
    db_name,
    username,
    app_name,
    query_type,
    SUM(calls)              AS calls,
    SUM(total_exec_time)    AS total_exec_time,
    SUM(rows)               AS rows,
    SUM(shared_blks_hit)    AS shared_blks_hit,
    SUM(shared_blks_read)   AS shared_blks_read,
    SUM(temp_blks_written)  AS temp_blks_written,
    SUM(wal_bytes)          AS wal_bytes,
    SUM(total_plan_time)    AS total_plan_time,
    AVG(mean_exec_time)     AS mean_exec_time
FROM pgss_delta_1h
GROUP BY time_bucket('1 day', bucket), server_id, query_id,
         db_name, username, app_name, query_type
WITH NO DATA;

SELECT add_continuous_aggregate_policy('pgss_delta_1d',
    start_offset  => INTERVAL '3 days',
    end_offset    => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);
SELECT add_retention_policy('pgss_delta_1d', INTERVAL '365 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 1.7: POSTGRESQL - Enterprise Metrics (BGWriter, Archiver, Query Dictionary)
-- --------------------------------------------------------------------------

-- PostgreSQL BGWriter & Checkpointer Statistics
CREATE TABLE IF NOT EXISTS postgres_bgwriter_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    checkpoints_timed BIGINT DEFAULT 0,
    checkpoints_req BIGINT DEFAULT 0,
    checkpoint_write_time DOUBLE PRECISION DEFAULT 0,
    checkpoint_sync_time DOUBLE PRECISION DEFAULT 0,
    buffers_checkpoint BIGINT DEFAULT 0,
    buffers_clean BIGINT DEFAULT 0,
    maxwritten_clean BIGINT DEFAULT 0,
    buffers_backend BIGINT DEFAULT 0,
    buffers_backend_fsync BIGINT DEFAULT 0,
    buffers_alloc BIGINT DEFAULT 0,
    stats_reset TIMESTAMPTZ,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_bgwriter_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_bgw_server_time ON postgres_bgwriter_stats (server_id, capture_timestamp DESC);
ALTER TABLE postgres_bgwriter_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_bgwriter_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_bgwriter_stats', INTERVAL '90 days', if_not_exists => TRUE);

-- PostgreSQL WAL Archiver Statistics
CREATE TABLE IF NOT EXISTS postgres_archiver_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    archived_count BIGINT DEFAULT 0,
    failed_count BIGINT DEFAULT 0,
    last_archived_wal TEXT,
    last_archived_time TIMESTAMPTZ,
    last_failed_wal TEXT,
    last_failed_time TIMESTAMPTZ,
    stats_reset TIMESTAMPTZ,
    failed_count_delta BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_archiver_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_postgres_arch_server_time ON postgres_archiver_stats (server_id, capture_timestamp DESC);
ALTER TABLE postgres_archiver_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_archiver_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_archiver_stats', INTERVAL '90 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 1.8: POSTGRESQL - Control Center (DBA-first derived metrics)
-- --------------------------------------------------------------------------
-- A compact snapshot table that powers the PostgreSQL Control Center strips.
-- Writes are delta/deduped by the collector to avoid storing identical snapshots.
CREATE TABLE IF NOT EXISTS postgres_control_center_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    -- Safety & durability
    wal_mb_per_min DOUBLE PRECISION DEFAULT 0,
    wal_size_mb DOUBLE PRECISION DEFAULT 0,
    max_replication_lag_mb DOUBLE PRECISION DEFAULT 0,
    replica_lag_sec DOUBLE PRECISION DEFAULT 0,
    checkpoint_req_ratio DOUBLE PRECISION DEFAULT 0,
    xid_age BIGINT DEFAULT 0,
    xid_wraparound_pct DOUBLE PRECISION DEFAULT 0,
    -- Workload
    tps DOUBLE PRECISION DEFAULT 0,
    active_sessions INTEGER DEFAULT 0,
    waiting_sessions INTEGER DEFAULT 0,
    slow_queries_count INTEGER DEFAULT 0,
    blocking_sessions INTEGER DEFAULT 0,
    autovacuum_workers INTEGER DEFAULT 0,
    dead_tuple_pct DOUBLE PRECISION DEFAULT 0,
    health_score INTEGER DEFAULT 0,
    health_status TEXT,
    -- V2 operational signals (2026-05-07)
    idle_sessions INTEGER DEFAULT 0,
    idle_in_txn_sessions INTEGER DEFAULT 0,
    connections_max INTEGER DEFAULT 0,
    connections_used INTEGER DEFAULT 0,
    connections_usage_pct DOUBLE PRECISION DEFAULT 0,
    cache_hit_ratio_pct DOUBLE PRECISION DEFAULT 0,
    deadlocks_per_min DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_control_center_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_cc_server_time ON postgres_control_center_stats (server_id, capture_timestamp DESC);
ALTER TABLE postgres_control_center_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_control_center_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_control_center_stats', INTERVAL '90 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_control_center_stats IS 'PostgreSQL Control Center derived metrics (WAL/replication/checkpoints/xid/workload).';

-- Per-replica lag time series for Control Center replication chart.
CREATE TABLE IF NOT EXISTS postgres_replication_lag_detail (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    replica_name TEXT NOT NULL,
    lag_mb DOUBLE PRECISION DEFAULT 0,
    state TEXT,
    sync_state TEXT,
    write_lag_sec  DOUBLE PRECISION DEFAULT 0,
    flush_lag_sec  DOUBLE PRECISION DEFAULT 0,
    replay_lag_sec DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
ALTER TABLE postgres_replication_lag_detail
    ADD COLUMN IF NOT EXISTS write_lag_sec  DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS flush_lag_sec  DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS replay_lag_sec DOUBLE PRECISION DEFAULT 0;
SELECT create_hypertable('postgres_replication_lag_detail', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_repl_lag_detail ON postgres_replication_lag_detail (server_id, replica_name, capture_timestamp DESC);
ALTER TABLE postgres_replication_lag_detail SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,replica_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_replication_lag_detail', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_replication_lag_detail IS 'Per-replica lag (MB) for Control Center charts.';

-- Replication slots risk: retained WAL can fill disks if consumers lag/disconnect.
CREATE TABLE IF NOT EXISTS postgres_replication_slot_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    slot_name TEXT NOT NULL,
    slot_type TEXT,
    active BOOLEAN DEFAULT false,
    temporary BOOLEAN DEFAULT false,
    retained_wal_mb DOUBLE PRECISION DEFAULT 0,
    restart_lsn TEXT,
    confirmed_flush_lsn TEXT,
    xmin_txid BIGINT,
    catalog_xmin_txid BIGINT,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_replication_slot_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_repl_slot_server_time ON postgres_replication_slot_stats (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_repl_slot_server_slot_time ON postgres_replication_slot_stats (server_id, slot_name, capture_timestamp DESC);
ALTER TABLE postgres_replication_slot_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,slot_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_replication_slot_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_replication_slot_stats IS 'Replication slot retention and activity for WAL/slot disk risk.';

-- Local disk (filesystem) free space snapshots for PostgreSQL nodes (when configured).
CREATE TABLE IF NOT EXISTS postgres_disk_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    mount_name TEXT NOT NULL,
    path TEXT NOT NULL,
    total_bytes BIGINT DEFAULT 0,
    free_bytes BIGINT DEFAULT 0,
    avail_bytes BIGINT DEFAULT 0,
    used_pct DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_disk_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_disk_server_time ON postgres_disk_stats (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_disk_server_mount_time ON postgres_disk_stats (server_id, mount_name, capture_timestamp DESC);
ALTER TABLE postgres_disk_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,mount_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_disk_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_disk_stats', INTERVAL '90 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_disk_stats IS 'Filesystem free space snapshots for PGDATA/WAL mounts (local-only when configured).';

-- Backup run events reported by external backup jobs (pgBackRest/Barman/pg_dump/etc.).
-- This is a webhook-style ingestion point: your backup job POSTs results to the API.
CREATE TABLE IF NOT EXISTS postgres_backup_runs (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    tool TEXT NOT NULL,                 -- e.g. pgbackrest | barman | pg_dump | custom
    backup_type TEXT NOT NULL,          -- e.g. full | incr | diff | logical | physical
    status TEXT NOT NULL,               -- success | failed | warning
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration_seconds BIGINT DEFAULT 0,
    wal_archived_until TIMESTAMPTZ,     -- optional: last WAL archived timestamp (RPO signal)
    repo TEXT,                          -- optional: repo name/path
    size_bytes BIGINT DEFAULT 0,
    error_message TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_backup_runs', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_backup_runs_server_time ON postgres_backup_runs (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_backup_runs_server_status_time ON postgres_backup_runs (server_id, status, capture_timestamp DESC);
ALTER TABLE postgres_backup_runs SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,backup_type',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_backup_runs', INTERVAL '60 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_backup_runs', INTERVAL '365 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_backup_runs IS 'Backup run results (reported by external tools) for DBA daily checks and RPO posture.';

-- PostgreSQL log events reported by external shippers/agents (webhook-style ingestion).
-- The monitoring server does NOT read remote log files directly; instead, an agent posts parsed events.
CREATE TABLE IF NOT EXISTS postgres_log_events (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    severity TEXT NOT NULL,             -- debug|info|notice|warning|error|fatal|panic
    sqlstate TEXT,
    message TEXT NOT NULL,
    user_name TEXT,
    database_name TEXT,
    application_name TEXT,
    client_addr TEXT,
    pid BIGINT,
    context TEXT,
    detail TEXT,
    hint TEXT,
    raw JSONB DEFAULT '{}'::jsonb,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_log_events', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_log_events_server_time ON postgres_log_events (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_log_events_server_sev_time ON postgres_log_events (server_id, severity, capture_timestamp DESC);
ALTER TABLE postgres_log_events SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,severity',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_log_events', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_log_events IS 'PostgreSQL log events (FATAL/PANIC/ERROR/auth failures/OOM) reported by external agents.';

-- Vacuum progress snapshots (pg_stat_progress_vacuum). Useful for "is vacuum running" and "which table is stuck".
CREATE TABLE IF NOT EXISTS postgres_vacuum_progress (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    pid BIGINT,
    database_name TEXT,
    user_name TEXT,
    relation_name TEXT,
    phase TEXT,
    heap_blks_total BIGINT DEFAULT 0,
    heap_blks_scanned BIGINT DEFAULT 0,
    heap_blks_vacuumed BIGINT DEFAULT 0,
    index_vacuum_count BIGINT DEFAULT 0,
    max_dead_tuples BIGINT DEFAULT 0,
    num_dead_tuples BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_vacuum_progress', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_vac_prog_server_time ON postgres_vacuum_progress (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_vac_prog_server_pid_time ON postgres_vacuum_progress (server_id, pid, capture_timestamp DESC);
ALTER TABLE postgres_vacuum_progress SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_vacuum_progress', INTERVAL '14 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_vacuum_progress IS 'Vacuum progress snapshots from pg_stat_progress_vacuum.';

-- Table-level maintenance stats (dead/live tuples, vacuum/analyze timestamps) for MVCC/autovacuum health.
CREATE TABLE IF NOT EXISTS postgres_table_maintenance_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT DEFAULT 'postgres',
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    total_bytes BIGINT DEFAULT 0,
    live_tuples BIGINT DEFAULT 0,
    dead_tuples BIGINT DEFAULT 0,
    dead_pct DOUBLE PRECISION DEFAULT 0,
    seq_scans BIGINT DEFAULT 0,
    idx_scans BIGINT DEFAULT 0,
    last_vacuum TIMESTAMPTZ,
    last_autovacuum TIMESTAMPTZ,
    last_analyze TIMESTAMPTZ,
    last_autoanalyze TIMESTAMPTZ,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('postgres_table_maintenance_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_tblmaint_server_time ON postgres_table_maintenance_stats (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_table_maint_db ON postgres_table_maintenance_stats (server_id, database_name, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_tblmaint_server_table_time ON postgres_table_maintenance_stats (server_id, schema_name, table_name, capture_timestamp DESC);
ALTER TABLE postgres_table_maintenance_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,schema_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_table_maintenance_stats', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_table_maintenance_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_table_maintenance_stats IS 'Table-level maintenance stats for vacuum/analyze/bloat monitor.';

-- Session state time-series (for Sessions & Activity trends).
CREATE TABLE IF NOT EXISTS postgres_session_state_counts (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    active_count INTEGER DEFAULT 0,
    idle_count INTEGER DEFAULT 0,
    idle_in_txn_count INTEGER DEFAULT 0,
    waiting_count INTEGER DEFAULT 0,
    total_count INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_session_state_counts', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_sess_state_server_time ON postgres_session_state_counts (server_id, capture_timestamp DESC);
ALTER TABLE postgres_session_state_counts SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_session_state_counts', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_session_state_counts IS 'Aggregated session state counts (active/idle/idle-in-txn/waiting) for trend charts.';

-- PgBouncer (pooler) health snapshots. Only collected if configured.
CREATE TABLE IF NOT EXISTS postgres_pooler_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    pooler_type TEXT DEFAULT 'pgbouncer',
    cl_active INTEGER DEFAULT 0,
    cl_waiting INTEGER DEFAULT 0,
    sv_active INTEGER DEFAULT 0,
    sv_idle INTEGER DEFAULT 0,
    sv_used INTEGER DEFAULT 0,
    maxwait_seconds DOUBLE PRECISION DEFAULT 0,
    total_pools INTEGER DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_pooler_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_pooler_server_time ON postgres_pooler_stats (server_id, capture_timestamp DESC);
ALTER TABLE postgres_pooler_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_pooler_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_pooler_stats IS 'PgBouncer pool totals (clients/servers/waiting/maxwait) for pooler monitor.';

-- Deadlocks counter deltas (from pg_stat_database.deadlocks) for history charts.
CREATE TABLE IF NOT EXISTS postgres_deadlock_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT NOT NULL,
    deadlocks_total BIGINT DEFAULT 0,
    deadlocks_delta BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('postgres_deadlock_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_deadlocks_server_time ON postgres_deadlock_stats (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_deadlocks_server_db_time ON postgres_deadlock_stats (server_id, database_name, capture_timestamp DESC);
ALTER TABLE postgres_deadlock_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_deadlock_stats', INTERVAL '30 days', if_not_exists => TRUE);
COMMENT ON TABLE postgres_deadlock_stats IS 'Deadlocks total and delta per database for troubleshooting lock contention.';

-- ============================================================================
-- SECTION 2: APPLICATION TABLES (Dashboards, Alerts, Users)
-- ============================================================================

-- --------------------------------------------------------------------------
-- 2.0: Server Registry + Audit Logs (secure credential storage)
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS optima_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    db_type TEXT NOT NULL CHECK (db_type IN ('postgres','sqlserver')),
    host TEXT NOT NULL,
    port INT NOT NULL,
    username TEXT NOT NULL,
    auth_type TEXT NOT NULL DEFAULT 'static',

    encrypted_secret BYTEA NOT NULL,
    encrypted_dek BYTEA NOT NULL,

    ssl_mode TEXT DEFAULT 'disable',
    is_active BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    created_by TEXT,
    last_test_at TIMESTAMPTZ,
    engine_edition INT
);
CREATE INDEX IF NOT EXISTS idx_optima_servers_active ON optima_servers (is_active);
CREATE INDEX IF NOT EXISTS idx_optima_servers_name ON optima_servers (name);
CREATE INDEX IF NOT EXISTS idx_optima_servers_type ON optima_servers (db_type);
-- Idempotent migration: add engine_edition if this table was created before this column existed.
ALTER TABLE optima_servers ADD COLUMN IF NOT EXISTS engine_edition INT;

CREATE TABLE IF NOT EXISTS optima_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    server_id UUID NULL,
    actor TEXT,
    ip_address TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_optima_audit_logs_time ON optima_audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_optima_audit_logs_server_time ON optima_audit_logs (server_id, created_at DESC);

-- --------------------------------------------------------------------------
-- 2.1: User Management
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS optima_users (
    user_id       SERIAL PRIMARY KEY,
    username      VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(50)  NOT NULL DEFAULT 'viewer',
    created_at    TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_optima_users_username ON optima_users (username);

-- --------------------------------------------------------------------------
-- 2.2: Widget Registry
-- --------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS optima_ui_widgets (
    widget_id        VARCHAR(100) PRIMARY KEY,
    dashboard_section VARCHAR(100) NOT NULL,
    title            VARCHAR(200) NOT NULL,
    chart_type       VARCHAR(50)  NOT NULL,
    current_sql      TEXT         NOT NULL,
    default_sql      TEXT         NOT NULL,
    updated_at       TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_optima_widgets_section ON optima_ui_widgets (dashboard_section);

-- --------------------------------------------------------------------------
-- 2.2.1: Plan Analysis Cache (EXPLAIN Plan Analyzer)
-- --------------------------------------------------------------------------
-- Stores deterministic performance reports derived from EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
-- to avoid recomputation for identical plans. Cache keys use a canonical JSON SHA-256 hash.
CREATE TABLE IF NOT EXISTS plan_analysis_cache (
    plan_hash TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL DEFAULT 1,
    query_text TEXT NULL,
    raw_plan_json JSONB NOT NULL,
    report_json JSONB NOT NULL,
    total_execution_time_ms DOUBLE PRECISION NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plan_analysis_cache_updated_at ON plan_analysis_cache (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_plan_analysis_cache_exec_time ON plan_analysis_cache (total_execution_time_ms DESC);
COMMENT ON TABLE plan_analysis_cache IS 'Cache of deterministic EXPLAIN plan analysis reports (canonical JSON hash → report JSON).';

-- ============================================================================
-- SECTION 3: COLLECTION MANAGEMENT TABLES
-- ============================================================================

-- ============================================================================
-- SECTION 3.3: STORAGE & INDEX HEALTH (Cross-engine, unified)
-- ============================================================================

-- Keep these objects in a dedicated schema to avoid name collisions with legacy tables.
CREATE SCHEMA IF NOT EXISTS monitor;

-- ============================================================================
-- SECTION 3.4: PostgreSQL Locks & Blocking (Stateful incidents)
-- ============================================================================
-- Design notes:
-- - These tables live in TimescaleDB (not the monitored Postgres instance).
-- - We persist "snapshots" + derived blocking pairs so we can reconstruct incidents over time.
-- - `server_id` maps to your configured instance name.

-- 3.4.1: Session state snapshot (pg_stat_activity)
CREATE TABLE IF NOT EXISTS monitor.pg_session_snapshot (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    pid INT,
    usename TEXT,
    datname TEXT,
    application_name TEXT,
    client_addr TEXT,
    state TEXT,
    wait_event_type TEXT,
    wait_event TEXT,
    xact_start TIMESTAMPTZ,
    query_start TIMESTAMPTZ,
    state_change TIMESTAMPTZ,
    query TEXT
);
SELECT create_hypertable('monitor.pg_session_snapshot', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_session_snapshot_lookup ON monitor.pg_session_snapshot (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_session_snapshot_state ON monitor.pg_session_snapshot (server_id, state, capture_timestamp DESC);
ALTER TABLE monitor.pg_session_snapshot SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
DO $$
BEGIN
    CALL remove_compression_policy('monitor.pg_session_snapshot', if_exists => TRUE);
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;
SELECT add_compression_policy('monitor.pg_session_snapshot', INTERVAL '7 days', if_not_exists => TRUE);

-- 3.4.2: Locks snapshot (pg_locks + pg_class for relation_name)
CREATE TABLE IF NOT EXISTS monitor.pg_lock_snapshot (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    pid INT,
    locktype TEXT,
    mode TEXT,
    granted BOOLEAN,
    relation_oid OID NOT NULL DEFAULT 0,
    relation_name TEXT,
    transactionid TEXT,
    waiting_seconds DOUBLE PRECISION
);
-- MIGRATION: ensure relation_oid is non-null (PK cannot contain NULLs).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema='monitor' AND table_name='pg_lock_snapshot' AND column_name='relation_oid'
    ) THEN
        -- Set any existing NULLs to 0 and enforce NOT NULL with default.
        EXECUTE 'UPDATE monitor.pg_lock_snapshot SET relation_oid = 0 WHERE relation_oid IS NULL';
        BEGIN
            EXECUTE 'ALTER TABLE monitor.pg_lock_snapshot ALTER COLUMN relation_oid SET DEFAULT 0';
        EXCEPTION WHEN OTHERS THEN
            NULL;
        END;
        BEGIN
            EXECUTE 'ALTER TABLE monitor.pg_lock_snapshot ALTER COLUMN relation_oid SET NOT NULL';
        EXCEPTION WHEN OTHERS THEN
            NULL;
        END;
    END IF;
END $$;
SELECT create_hypertable('monitor.pg_lock_snapshot', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_lock_snapshot_lookup ON monitor.pg_lock_snapshot (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_lock_snapshot_waiting ON monitor.pg_lock_snapshot (server_id, granted, capture_timestamp DESC) WHERE granted = FALSE;
ALTER TABLE monitor.pg_lock_snapshot SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,granted',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
DO $$
BEGIN
    CALL remove_compression_policy('monitor.pg_lock_snapshot', if_exists => TRUE);
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;
SELECT add_compression_policy('monitor.pg_lock_snapshot', INTERVAL '7 days', if_not_exists => TRUE);

-- 3.4.3: Blocking pairs (dependency graph edges)
CREATE TABLE IF NOT EXISTS monitor.pg_blocking_pairs (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    blocked_pid INT NOT NULL,
    blocking_pid INT NOT NULL
);
SELECT create_hypertable('monitor.pg_blocking_pairs', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_blocking_pairs_lookup ON monitor.pg_blocking_pairs (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_blocking_pairs_edge ON monitor.pg_blocking_pairs (server_id, blocking_pid, capture_timestamp DESC);
ALTER TABLE monitor.pg_blocking_pairs SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
DO $$
BEGIN
    CALL remove_compression_policy('monitor.pg_blocking_pairs', if_exists => TRUE);
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;
SELECT add_compression_policy('monitor.pg_blocking_pairs', INTERVAL '14 days', if_not_exists => TRUE);

-- 3.4.4: Incident tracking (stateful; not a hypertable)
CREATE TABLE IF NOT EXISTS monitor.pg_blocking_incident (
    incident_id BIGSERIAL PRIMARY KEY,
    server_id UUID NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    root_blocker_pid INT,
    root_blocker_query TEXT,
    peak_blocked_sessions INT DEFAULT 0,
    status TEXT DEFAULT 'active' -- 'active' | 'resolved'
);
CREATE INDEX IF NOT EXISTS idx_pg_blocking_incident_server_started ON monitor.pg_blocking_incident (server_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_pg_blocking_incident_status ON monitor.pg_blocking_incident (server_id, status, started_at DESC) WHERE status = 'active';

-- 3.3.1: Index usage stats (delta snapshot)
CREATE TABLE IF NOT EXISTS monitor.index_usage_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    engine TEXT NOT NULL, -- 'sqlserver' | 'postgres'
    server_id UUID NOT NULL,
    db_name TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    index_name TEXT NOT NULL,
    seeks BIGINT,
    scans BIGINT,
    lookups BIGINT,
    updates BIGINT,
    index_size_mb NUMERIC,
    is_unique BOOLEAN,
    is_pk BOOLEAN,
    fillfactor INT,
    last_user_seek TIMESTAMPTZ,
    last_user_scan TIMESTAMPTZ,
    last_user_lookup TIMESTAMPTZ
);
SELECT create_hypertable('monitor.index_usage_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_index_usage_stats_lookup ON monitor.index_usage_stats (engine, server_id, db_name, schema_name, table_name, index_name, capture_timestamp DESC);
ALTER TABLE monitor.index_usage_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,engine',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
DO $$
BEGIN
    CALL remove_compression_policy('monitor.index_usage_stats', if_exists => TRUE);
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;
SELECT add_compression_policy('monitor.index_usage_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- 3.3.5: Index definitions snapshot (for duplicate/overlap analysis; daily cadence)
CREATE TABLE IF NOT EXISTS monitor.index_definitions (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    engine TEXT NOT NULL,
    server_id UUID NOT NULL,
    db_name TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    index_name TEXT NOT NULL,
    key_columns TEXT,
    include_columns TEXT,
    filter_definition TEXT,
    is_unique BOOLEAN,
    is_pk BOOLEAN,
    index_type TEXT
);
SELECT create_hypertable('monitor.index_definitions', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_index_definitions_lookup ON monitor.index_definitions (engine, server_id, db_name, schema_name, table_name, capture_timestamp DESC);
ALTER TABLE monitor.index_definitions SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'engine,server_id,db_name,schema_name,table_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.index_definitions', INTERVAL '30 days', if_not_exists => TRUE);

-- 3.3.6: Daily unused-index analysis snapshot (for alerts / UI; one refresh per instance per UTC day)
CREATE TABLE IF NOT EXISTS monitor.index_unused_candidates_daily (
    run_at TIMESTAMPTZ NOT NULL,
    engine TEXT NOT NULL,
    server_id UUID NOT NULL,
    db_name TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    index_name TEXT NOT NULL,
    updates_24h BIGINT NOT NULL DEFAULT 0,
    index_size_mb NUMERIC,
    last_user_seek TIMESTAMPTZ,
    rank SMALLINT
);
SELECT create_hypertable('monitor.index_unused_candidates_daily', 'run_at', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_index_unused_daily_lookup ON monitor.index_unused_candidates_daily (engine, server_id, run_at DESC);

-- 3.3.2: Table usage + size (delta snapshot)
CREATE TABLE IF NOT EXISTS monitor.table_usage_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    engine TEXT NOT NULL, -- 'sqlserver' | 'postgres'
    server_id UUID NOT NULL,
    db_name TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    seq_scans BIGINT,
    idx_scans BIGINT,
    rows_read BIGINT,
    rows_modified BIGINT,
    table_size_mb NUMERIC,
    index_size_mb NUMERIC,
    row_count BIGINT
);
SELECT create_hypertable('monitor.table_usage_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_table_usage_stats_lookup ON monitor.table_usage_stats (engine, server_id, db_name, schema_name, table_name, capture_timestamp DESC);
ALTER TABLE monitor.table_usage_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'engine,server_id,db_name,schema_name,table_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
DO $$
BEGIN
    CALL remove_compression_policy('monitor.table_usage_stats', if_exists => TRUE);
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;
SELECT add_compression_policy('monitor.table_usage_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- 3.3.3: Table + index growth history (size snapshot; typically 6h cadence)
CREATE TABLE IF NOT EXISTS monitor.table_size_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    engine TEXT NOT NULL, -- 'sqlserver' | 'postgres'
    server_id UUID NOT NULL,
    db_name TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    table_size_mb NUMERIC,
    index_size_mb NUMERIC,
    row_count BIGINT
);
SELECT create_hypertable('monitor.table_size_history', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_table_size_history_lookup ON monitor.table_size_history (engine, server_id, db_name, schema_name, table_name, capture_timestamp DESC);
ALTER TABLE monitor.table_size_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'engine,server_id,db_name,schema_name,table_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
DO $$
BEGIN
    CALL remove_compression_policy('monitor.table_size_history', if_exists => TRUE);
EXCEPTION WHEN OTHERS THEN
    NULL;
END $$;
SELECT add_compression_policy('monitor.table_size_history', INTERVAL '30 days', if_not_exists => TRUE);

-- 3.3.4: Persistent cumulative state tables (to compute deltas across API restarts)
-- These tables store the last observed *cumulative* counters from source engines.
-- Hypertables above store delta snapshots for trending.
CREATE TABLE IF NOT EXISTS monitor.index_usage_state (
    engine TEXT NOT NULL,
    server_id UUID NOT NULL,
    db_name TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    index_name TEXT NOT NULL,
    seeks_total BIGINT NOT NULL DEFAULT 0,
    scans_total BIGINT NOT NULL DEFAULT 0,
    lookups_total BIGINT NOT NULL DEFAULT 0,
    updates_total BIGINT NOT NULL DEFAULT 0,
    index_size_mb NUMERIC,
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (server_id, engine, db_name, schema_name, table_name, index_name)
);

CREATE TABLE IF NOT EXISTS monitor.table_usage_state (
    engine TEXT NOT NULL,
    server_id UUID NOT NULL,
    db_name TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    seq_scans_total BIGINT NOT NULL DEFAULT 0,
    idx_scans_total BIGINT NOT NULL DEFAULT 0,
    rows_read_total BIGINT NOT NULL DEFAULT 0,
    rows_modified_total BIGINT NOT NULL DEFAULT 0,
    table_size_mb NUMERIC,
    index_size_mb NUMERIC,
    row_count BIGINT,
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (server_id, engine, db_name, schema_name, table_name)
);    

-- ============================================================================
-- SECTION 4: GRANT PERMISSIONS
-- ============================================================================
-- Replace broad PUBLIC grants with targeted role grants for security
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO sql_optima_app;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA monitor TO sql_optima_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO sql_optima_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA monitor TO sql_optima_app;

DO $$
DECLARE
    ht_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO ht_count FROM timescaledb_information.hypertables;
    RAISE NOTICE '============================================';
    RAISE NOTICE 'SQL Optima Unified Schema created successfully!';
    RAISE NOTICE 'Total hypertable count: %', ht_count;
    RAISE NOTICE '============================================';
END $$;

-- MIGRATION: Add error_message and critical_jobs_disabled columns to sqlserver_job_metrics
-- Uses ADD COLUMN IF NOT EXISTS (PostgreSQL 9.6+) — safe to re-run on any existing schema.
ALTER TABLE sqlserver_job_metrics ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE sqlserver_job_metrics ADD COLUMN IF NOT EXISTS critical_jobs_disabled INTEGER DEFAULT 0;

COMMENT ON COLUMN sqlserver_job_metrics.error_message IS 'Stores error message if job collection failed (e.g., permission denied on msdb tables)';
COMMENT ON COLUMN sqlserver_job_metrics.critical_jobs_disabled IS 'Count of disabled SQL Agent jobs in backup/maintenance/database categories (DEFECT-15 Phase B)';

-- ============================================================================
-- LEGACY MIGRATION COMPATIBILITY OBJECTS
-- Consolidated from infrastructure/sql_scripts/migrations/*.sql
-- ============================================================================

-- Legacy migration blocks removed for deprecated tables.

-- Continuous Aggregate for HA replica health — reads from canonical monitor schema
DROP MATERIALIZED VIEW IF EXISTS sqlserver_ag_health_summary CASCADE;
CREATE MATERIALIZED VIEW sqlserver_ag_health_summary
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    server_id,
    ag_name,
    replica_server_name,
    AVG(log_send_queue_kb)     AS avg_log_send_queue_kb,
    AVG(redo_queue_kb)         AS avg_redo_queue_kb,
    MAX(log_send_queue_kb)     AS max_log_send_queue_kb,
    MAX(redo_queue_kb)         AS max_redo_queue_kb,
    MAX(secondary_lag_seconds) AS max_secondary_lag_secs
FROM monitor.sqlserver_ha_replica_state
GROUP BY 1, 2, 3, 4
WITH NO DATA;

SELECT add_continuous_aggregate_policy('sqlserver_ag_health_summary',
    start_offset => INTERVAL '1 hour', end_offset => INTERVAL '5 minutes',
    schedule_interval => INTERVAL '5 minutes', if_not_exists => TRUE);

COMMENT ON TABLE sqlserver_ag_health IS 'DEPRECATED: legacy AG health table (public schema). No new writes. Retained for existing data rolloff. Use monitor.sqlserver_ha_replica_state for all new queries.';

DROP MATERIALIZED VIEW IF EXISTS sqlserver_db_throughput_summary CASCADE;
CREATE MATERIALIZED VIEW sqlserver_db_throughput_summary
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    server_id,
    database_name,
    AVG(tps) AS avg_tps,
    AVG(batch_requests_per_sec) AS avg_batch_requests,
    SUM(total_reads) AS total_reads,
    SUM(total_writes) AS total_writes
FROM sqlserver_database_throughput
GROUP BY 1, 2, 3
WITH NO DATA;

SELECT add_continuous_aggregate_policy('sqlserver_db_throughput_summary',
    start_offset => INTERVAL '1 hour', end_offset => INTERVAL '5 minutes',
    schedule_interval => INTERVAL '5 minutes', if_not_exists => TRUE);

ALTER MATERIALIZED VIEW sqlserver_db_throughput_summary SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby   = 'bucket DESC'
);
SELECT add_compression_policy('sqlserver_db_throughput_summary', INTERVAL '7 days', if_not_exists => TRUE);

DROP MATERIALIZED VIEW IF EXISTS postgres_checkpoint_summary CASCADE;
CREATE MATERIALIZED VIEW postgres_checkpoint_summary
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    server_id,
    AVG(checkpoints_timed) AS avg_checkpoints_timed,
    AVG(checkpoints_req) AS avg_checkpoints_req,
    SUM(CASE WHEN checkpoints_req > 0 THEN 1 ELSE 0 END) AS req_checkpoint_count,
    AVG(checkpoint_write_time) AS avg_checkpoint_write_time,
    AVG(buffers_checkpoint) AS avg_buffers_checkpoint,
    MAX(buffers_checkpoint) AS max_buffers_checkpoint
FROM postgres_bgwriter_stats
GROUP BY 1, 2
WITH NO DATA;

SELECT add_continuous_aggregate_policy('postgres_checkpoint_summary',
    start_offset => INTERVAL '1 hour', end_offset => INTERVAL '5 minutes',
    schedule_interval => INTERVAL '5 minutes', if_not_exists => TRUE);

DROP MATERIALIZED VIEW IF EXISTS postgres_archive_summary CASCADE;
CREATE MATERIALIZED VIEW postgres_archive_summary
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('5 minutes', capture_timestamp) AS bucket,
    server_id,
    SUM(archived_count) AS total_archived,
    SUM(failed_count) AS total_failed,
    MAX(failed_count) AS max_failed_in_period,
    AVG(failed_count_delta) AS avg_failure_rate
FROM postgres_archiver_stats
GROUP BY 1, 2
WITH NO DATA;

SELECT add_continuous_aggregate_policy('postgres_archive_summary',
    start_offset => INTERVAL '1 hour', end_offset => INTERVAL '5 minutes',
    schedule_interval => INTERVAL '5 minutes', if_not_exists => TRUE);

-- ============================================================================
-- DBA War Room: Incident timeline
-- ============================================================================
CREATE TABLE IF NOT EXISTS optima_incidents (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    severity TEXT NOT NULL,
    category TEXT NOT NULL,
    description TEXT,
    recommendations TEXT,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('optima_incidents', 'capture_timestamp',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_incidents_server_time
    ON optima_incidents (server_id, capture_timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_incidents_severity
    ON optima_incidents (severity, capture_timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_incidents_category
    ON optima_incidents (category, capture_timestamp DESC);

ALTER TABLE optima_incidents SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id, severity, category',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);

SELECT add_compression_policy('optima_incidents', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('optima_incidents', INTERVAL '14 days', if_not_exists => TRUE);

-- ============================================================================
-- SECTION: SQL Server Query Analysis Dashboard Tables
-- sqlserver_watched_queries, sqlserver_watched_query_snapshots,
-- sqlserver_watched_query_events, sqlserver_query_regressions, sqlserver_plan_instability
-- ============================================================================

-- 1. Watched queries registry (max 10 per instance, enforced at application layer)
CREATE TABLE IF NOT EXISTS sqlserver_watched_queries (
    id                    SERIAL PRIMARY KEY,
    server_id             UUID NOT NULL,
    database_name         TEXT NOT NULL DEFAULT 'master',
    query_hash            BIGINT,
    object_id             INT,
    name                  TEXT NOT NULL DEFAULT '',
    query_text            TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Watched query time-series snapshots (collected every 5 min)
CREATE TABLE IF NOT EXISTS sqlserver_watched_query_snapshots (
    capture_timestamp     TIMESTAMPTZ NOT NULL,
    watched_id            INT NOT NULL REFERENCES sqlserver_watched_queries(id) ON DELETE CASCADE,
    server_id             UUID NOT NULL,
    executions            BIGINT,
    avg_duration_ms       DOUBLE PRECISION,
    avg_cpu_ms            DOUBLE PRECISION,
    avg_reads             DOUBLE PRECISION,
    total_duration_ms     DOUBLE PRECISION,
    total_cpu_ms          DOUBLE PRECISION,
    plan_count            INT,
    last_execution_time   TIMESTAMPTZ,
    query_plan            TEXT,
    wait_stats            JSONB
);

SELECT create_hypertable('sqlserver_watched_query_snapshots', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wqs_watched ON sqlserver_watched_query_snapshots (watched_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wqs_instance ON sqlserver_watched_query_snapshots (server_id, capture_timestamp DESC);

ALTER TABLE sqlserver_watched_query_snapshots SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'watched_id,server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_watched_query_snapshots', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_watched_query_snapshots', INTERVAL '90 days', if_not_exists => TRUE);

-- 3. Watched query optimization event markers
CREATE TABLE IF NOT EXISTS sqlserver_watched_query_events (
    id           SERIAL PRIMARY KEY,
    watched_id   INT NOT NULL REFERENCES sqlserver_watched_queries(id) ON DELETE CASCADE,
    capture_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type   TEXT NOT NULL,
    notes        TEXT
);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wqe_watched ON sqlserver_watched_query_events (watched_id, capture_timestamp DESC);

-- 4. Query regression detection results (collected every 30 min)
CREATE TABLE IF NOT EXISTS sqlserver_query_regressions (
    capture_timestamp     TIMESTAMPTZ NOT NULL,
    server_id             UUID NOT NULL,
    database_name         TEXT,
    query_hash            BIGINT NOT NULL,
    query_text            TEXT,
    regression_type       TEXT NOT NULL,
    previous_avg          DOUBLE PRECISION,
    current_avg           DOUBLE PRECISION,
    percent_change        DOUBLE PRECISION,
    plan_changed          BOOLEAN DEFAULT FALSE
);

SELECT create_hypertable('sqlserver_query_regressions', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_qr_instance ON sqlserver_query_regressions (server_id, capture_timestamp DESC);

ALTER TABLE sqlserver_query_regressions SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_query_regressions', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_query_regressions', INTERVAL '30 days', if_not_exists => TRUE);

-- 5. Plan instability detection results (collected every 30 min)
CREATE TABLE IF NOT EXISTS sqlserver_plan_instability (
    capture_timestamp     TIMESTAMPTZ NOT NULL,
    server_id             UUID NOT NULL,
    database_name         TEXT,
    query_hash            BIGINT NOT NULL,
    query_text            TEXT,
    plan_count            INT NOT NULL,
    last_execution_time   TIMESTAMPTZ
);

SELECT create_hypertable('sqlserver_plan_instability', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_pi_instance ON sqlserver_plan_instability (server_id, capture_timestamp DESC);

ALTER TABLE sqlserver_plan_instability SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_plan_instability', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_plan_instability', INTERVAL '30 days', if_not_exists => TRUE);

-- ============================================================================
-- SQL Server Log Shipping Health
-- ============================================================================
CREATE TABLE IF NOT EXISTS sqlserver_log_shipping_health (
    capture_timestamp        TIMESTAMPTZ     NOT NULL,
    server_id                UUID            NOT NULL,
    primary_server           VARCHAR(255)    NOT NULL DEFAULT '',
    primary_database         VARCHAR(255)    NOT NULL DEFAULT '',
    secondary_server         VARCHAR(255)    NOT NULL DEFAULT '',
    secondary_database       VARCHAR(255)    NOT NULL DEFAULT '',
    last_backup_date         TIMESTAMPTZ,
    last_backup_file         VARCHAR(512)    NOT NULL DEFAULT '',
    last_restore_date        TIMESTAMPTZ,
    last_copied_date         TIMESTAMPTZ,
    restore_delay_minutes    INT             NOT NULL DEFAULT 0,
    restore_threshold_minutes INT            NOT NULL DEFAULT 0,
    status                   SMALLINT        NOT NULL DEFAULT 0,
    is_primary               BOOLEAN         NOT NULL DEFAULT FALSE
);

SELECT create_hypertable('sqlserver_log_shipping_health', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => FALSE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_logship_server
    ON sqlserver_log_shipping_health (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_logship_primary_db
    ON sqlserver_log_shipping_health (primary_database, capture_timestamp DESC);

ALTER TABLE sqlserver_log_shipping_health SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_log_shipping_health', INTERVAL '7 days', if_not_exists => TRUE);

-- ============================================================================
-- SQL Server Backup & Recovery (Backup & Recovery dashboard)
-- ============================================================================
CREATE TABLE IF NOT EXISTS monitor.sqlserver_backup_database_posture (
    capture_timestamp           TIMESTAMPTZ NOT NULL,
    server_id                   UUID        NOT NULL,
    database_name               TEXT        NOT NULL DEFAULT '',
    recovery_model_desc         TEXT        NOT NULL DEFAULT '',
    last_full_finish            TIMESTAMPTZ,
    last_diff_finish            TIMESTAMPTZ,
    last_log_finish             TIMESTAMPTZ,
    minutes_since_full          INT         NOT NULL DEFAULT 0,
    minutes_since_log           INT         NOT NULL DEFAULT 0,
    last_full_size_mb           DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_log_size_mb            DOUBLE PRECISION NOT NULL DEFAULT 0,
    full_fresh_ok               BOOLEAN     NOT NULL DEFAULT false,
    log_fresh_ok                BOOLEAN     NOT NULL DEFAULT false,
    has_full_backup             BOOLEAN     NOT NULL DEFAULT false,
    backup_compression_default  BOOLEAN     NOT NULL DEFAULT false,
    PRIMARY KEY (server_id, capture_timestamp, database_name)
);

SELECT create_hypertable('monitor.sqlserver_backup_database_posture', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_backup_posture_server
    ON monitor.sqlserver_backup_database_posture (server_id, capture_timestamp DESC);
ALTER TABLE monitor.sqlserver_backup_database_posture SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_backup_database_posture', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.sqlserver_backup_database_posture', INTERVAL '90 days', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS monitor.sqlserver_backup_history (
    capture_timestamp           TIMESTAMPTZ NOT NULL,
    server_id                   UUID        NOT NULL,
    backup_set_uuid             UUID        NOT NULL,
    database_name               TEXT        NOT NULL DEFAULT '',
    backup_type                 CHAR(1)     NOT NULL DEFAULT '',
    backup_start_date           TIMESTAMPTZ,
    backup_finish_date          TIMESTAMPTZ,
    backup_size_mb              DOUBLE PRECISION NOT NULL DEFAULT 0,
    compressed_backup_size_mb   DOUBLE PRECISION NOT NULL DEFAULT 0,
    duration_seconds            INT         NOT NULL DEFAULT 0,
    is_copy_only                BOOLEAN     NOT NULL DEFAULT false,
    has_checksum                BOOLEAN     NOT NULL DEFAULT false,
    is_compressed               BOOLEAN     NOT NULL DEFAULT false,
    first_lsn                   VARCHAR(64) NOT NULL DEFAULT '',
    last_lsn                    VARCHAR(64) NOT NULL DEFAULT '',
    user_name                   TEXT        NOT NULL DEFAULT '',
    physical_device             TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (server_id, capture_timestamp, backup_set_uuid)
);

SELECT create_hypertable('monitor.sqlserver_backup_history', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_backup_hist_server
    ON monitor.sqlserver_backup_history (server_id, backup_finish_date DESC);
ALTER TABLE monitor.sqlserver_backup_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_backup_history', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.sqlserver_backup_history', INTERVAL '365 days', if_not_exists => TRUE);

COMMENT ON TABLE monitor.sqlserver_backup_database_posture IS
    'Per-database backup posture snapshot for SQL Server Backup & Recovery dashboard.';
COMMENT ON TABLE monitor.sqlserver_backup_history IS
    'Recent msdb.dbo.backupset operations collected for backup history and trend charts.';

-- ============================================================================
-- DBA War Room: Continuous Aggregates for Baselines
-- ============================================================================

-- Hourly Wait Stats Baseline
DROP MATERIALIZED VIEW IF EXISTS hourly_wait_stats_baseline CASCADE;
CREATE MATERIALIZED VIEW hourly_wait_stats_baseline
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', capture_timestamp) AS capture_timestamp,
    server_id,
    wait_type,
    AVG(disk_read_ms_per_sec) AS avg_disk_read_ms,
    AVG(blocking_ms_per_sec) AS avg_blocking_ms,
    AVG(parallelism_ms_per_sec) AS avg_parallelism_ms,
    AVG(other_ms_per_sec) AS avg_other_ms,
    COUNT(*) AS sample_count
FROM sqlserver_wait_history
GROUP BY
    time_bucket('1 hour', capture_timestamp),
    server_id,
    wait_type
WITH NO DATA;

SELECT add_continuous_aggregate_policy('hourly_wait_stats_baseline',
    start_offset => INTERVAL '3 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '5 minutes',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_hourly_wait_baseline_time
    ON hourly_wait_stats_baseline (capture_timestamp DESC, server_id);

-- Hourly Query Performance Baseline
DROP MATERIALIZED VIEW IF EXISTS hourly_query_performance_baseline CASCADE;
CREATE MATERIALIZED VIEW hourly_query_performance_baseline
WITH (timescaledb.continuous) AS
SELECT 
    time_bucket('1 hour', capture_timestamp) AS capture_timestamp,
    server_id,
    query_hash,
    AVG(total_elapsed_time_ms / NULLIF(execution_count, 0)) AS avg_exec_time_ms,
    SUM(execution_count) AS total_execution_count,
    AVG(total_worker_time_ms / NULLIF(execution_count, 0)) AS avg_cpu_time_ms,
    AVG(total_logical_reads / NULLIF(execution_count, 0)) AS avg_logical_reads,
    COUNT(*) AS sample_count
FROM sqlserver_procedure_stats
GROUP BY 
    time_bucket('1 hour', capture_timestamp),
    server_id,
    query_hash
WITH NO DATA;

SELECT add_continuous_aggregate_policy('hourly_query_performance_baseline',
    start_offset => INTERVAL '3 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '5 minutes',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_hourly_query_baseline_time
    ON hourly_query_performance_baseline (capture_timestamp DESC, server_id);

-- ============================================================================
-- SQL Server Storage & Index Health History
-- ============================================================================

CREATE SCHEMA IF NOT EXISTS snapshot;

-- 1) Database Storage History
-- 2) Table Size History
CREATE TABLE IF NOT EXISTS snapshot.sqlserver_table_size_history (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id       UUID NOT NULL,
    database_name   TEXT NOT NULL,
    schema_name     TEXT NOT NULL,
    table_name      TEXT NOT NULL,
    row_count       BIGINT,
    total_mb        NUMERIC(18,2),
    data_mb         NUMERIC(18,2),
    index_mb        NUMERIC(18,2)
);

SELECT create_hypertable(
    'snapshot.sqlserver_table_size_history',
    'capture_timestamp',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_sqlserver_table_size_history_lookup ON snapshot.sqlserver_table_size_history
(server_id, database_name, schema_name, table_name, capture_timestamp DESC);

-- 6) Compression Policies
ALTER TABLE snapshot.sqlserver_table_size_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('snapshot.sqlserver_table_size_history', INTERVAL '7 days', if_not_exists => TRUE);

-- 7) Retention Policies
SELECT add_retention_policy('snapshot.sqlserver_table_size_history', INTERVAL '2 years', if_not_exists => TRUE);
-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Enhanced PostgreSQL Memory Intelligence schema.
--          Supports time-series metrics, derived analytics, and forecasting.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

-- 1. Create monitoring schema if not exists
CREATE SCHEMA IF NOT EXISTS monitor;

-- 2. Host Memory Metrics Hypertable
CREATE TABLE IF NOT EXISTS monitor.host_memory_samples (
    capture_timestamp   TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,

    total_memory_mb     BIGINT,
    used_memory_mb      BIGINT,
    free_memory_mb      BIGINT,
    cached_memory_mb    BIGINT,
    buffered_memory_mb  BIGINT,

    swap_total_mb       BIGINT,
    swap_used_mb        BIGINT,

    page_faults         BIGINT,
    major_page_faults   BIGINT
);

SELECT create_hypertable(
    'monitor.host_memory_samples',
    'capture_timestamp',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_host_mem_instance_ts ON monitor.host_memory_samples (server_id, capture_timestamp DESC);

-- 3. PostgreSQL Process & Connection Metrics Hypertable
CREATE TABLE IF NOT EXISTS monitor.pg_memory_samples (
    capture_timestamp       TIMESTAMPTZ NOT NULL,
    server_id    UUID NOT NULL,
    -- postgres process memory
    postgres_rss_mb         BIGINT,
    postgres_vsz_mb         BIGINT,
    -- connections
    active_connections      INT,
    idle_connections        INT,
    total_connections       INT,
    -- buffer/cache stats
    blks_hit                BIGINT,
    blks_read               BIGINT,
    -- temp spill stats
    temp_files              BIGINT,
    temp_bytes              BIGINT,
    -- bgwriter stats
    buffers_checkpoint      BIGINT,
    buffers_clean           BIGINT,
    buffers_backend         BIGINT
);

SELECT create_hypertable(
    'monitor.pg_memory_samples',
    'capture_timestamp',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_pg_mem_instance_ts ON monitor.pg_memory_samples (server_id, capture_timestamp DESC);

-- 4. PostgreSQL Memory Configuration Components (Captured less frequently)
CREATE TABLE IF NOT EXISTS monitor.pg_memory_components (
    capture_timestamp       TIMESTAMPTZ NOT NULL,
    server_id    UUID NOT NULL,
    shared_buffers_mb       BIGINT,
    work_mem_mb             BIGINT,
    maintenance_work_mem_mb BIGINT,
    wal_buffers_mb          BIGINT,
    temp_buffers_mb         BIGINT,
    effective_cache_size_mb BIGINT,
    max_connections         INTEGER DEFAULT 100
);

SELECT create_hypertable(
    'monitor.pg_memory_components',
    'capture_timestamp',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_pg_comp_instance_ts ON monitor.pg_memory_components (server_id, capture_timestamp DESC);

-- 5. Derived Metrics Table (Materialized Layer for UI performance)
CREATE TABLE IF NOT EXISTS monitor.pg_memory_derived (
    capture_timestamp           TIMESTAMPTZ NOT NULL,
    server_id        UUID NOT NULL,
    pg_memory_percent           DOUBLE PRECISION,
    cache_hit_ratio             DOUBLE PRECISION,
    temp_spill_rate_mb_s        DOUBLE PRECISION,
    swap_used_percent           DOUBLE PRECISION,
    connection_memory_est_mb    DOUBLE PRECISION,
    memory_pressure_percent     DOUBLE PRECISION,
    health_score                INT
);

SELECT create_hypertable(
    'monitor.pg_memory_derived',
    'capture_timestamp',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pg_der_instance_ts ON monitor.pg_memory_derived (server_id, capture_timestamp DESC);

-- 6. Compression Policies
ALTER TABLE monitor.host_memory_samples SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'server_id'
);
SELECT add_compression_policy('monitor.host_memory_samples', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE monitor.pg_memory_samples SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'server_id'
);
SELECT add_compression_policy('monitor.pg_memory_samples', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE monitor.pg_memory_derived SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'server_id'
);
SELECT add_compression_policy('monitor.pg_memory_derived', INTERVAL '7 days', if_not_exists => TRUE);

-- 7. Retention Policies
SELECT add_retention_policy('monitor.host_memory_samples', INTERVAL '180 days', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_memory_samples', INTERVAL '180 days', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_memory_derived', INTERVAL '180 days', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_memory_components', INTERVAL '365 days', if_not_exists => TRUE);

-- Collector Configs (Controls frequency and status of background metrics collection)
CREATE TABLE IF NOT EXISTS optima_collector_configs (
    id SERIAL PRIMARY KEY,
    collector_name VARCHAR(100) UNIQUE NOT NULL,
    module VARCHAR(100) NOT NULL,
    frequency_seconds INTEGER NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by VARCHAR(100),
    -- Startup launch sequence: lower = earlier goroutine in appserver.go.
    -- 10=Pulse, 20=Background, 30-180=dedicated goroutines, NULL=system/framework.
    run_order INT
);

-- Notification Config (Admin-managed outbound alert destinations)
CREATE TABLE IF NOT EXISTS optima_notification_config (
    id              SERIAL PRIMARY KEY,
    channel         VARCHAR(50) UNIQUE NOT NULL,  -- 'webhook' | 'slack'
    url             TEXT NOT NULL DEFAULT '',
    is_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      VARCHAR(100)
);
INSERT INTO optima_notification_config (channel, url, is_enabled)
VALUES ('webhook', '', false), ('slack', '', false)
ON CONFLICT (channel) DO NOTHING;

-- Platform settings (UI-managed toggles; e.g. os_metrics_ingest_enabled from OS Collector UI)
CREATE TABLE IF NOT EXISTS optima_platform_settings (
    setting_key   VARCHAR(100) PRIMARY KEY,
    setting_value TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by    VARCHAR(100)
);
GRANT SELECT, INSERT, UPDATE ON optima_platform_settings TO sql_optima_app;
-- Default off; enable from UI (Enable ingest) or download OS collector bundle.
INSERT INTO optima_platform_settings (setting_key, setting_value, updated_by)
VALUES ('os_metrics_ingest_enabled', 'false', 'schema')
ON CONFLICT (setting_key) DO NOTHING;

-- SQLSERVER Query Store Deltas
CREATE TABLE IF NOT EXISTS monitor.sqlserver_query_store_staging (
    server_id UUID NOT NULL,
    database_name TEXT NOT NULL,
    query_hash BIGINT NOT NULL,
    query_text TEXT NOT NULL,
    plan_id BIGINT NOT NULL,
    runtime_stats_interval_id BIGINT NOT NULL,
    executions BIGINT NOT NULL,
    avg_duration_ms DOUBLE PRECISION NOT NULL,
    avg_cpu_ms DOUBLE PRECISION NOT NULL,
    avg_logical_reads DOUBLE PRECISION NOT NULL,
    total_cpu_ms DOUBLE PRECISION NOT NULL,
    last_execution_time TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS monitor.sqlserver_query_store_snapshot (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT NOT NULL,
    query_hash BIGINT NOT NULL,
    query_text TEXT NOT NULL,
    plan_id BIGINT NOT NULL,
    runtime_stats_interval_id BIGINT NOT NULL,
    total_executions BIGINT NOT NULL,
    total_cpu_ms DOUBLE PRECISION NOT NULL,
    total_duration_ms DOUBLE PRECISION NOT NULL,
    total_logical_reads DOUBLE PRECISION NOT NULL,
    row_fingerprint TEXT NOT NULL
);
SELECT create_hypertable('monitor.sqlserver_query_store_snapshot', 'capture_timestamp', if_not_exists => TRUE);

-- Unique index to support ON CONFLICT DO NOTHING (deduplication)
CREATE UNIQUE INDEX IF NOT EXISTS idx_sqlserver_qs_snapshot_unique 
ON monitor.sqlserver_query_store_snapshot (capture_timestamp, server_id, query_hash, plan_id, runtime_stats_interval_id);

CREATE TABLE IF NOT EXISTS monitor.sqlserver_query_store_interval (
    bucket_start TIMESTAMPTZ NOT NULL,
    bucket_end TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT NOT NULL,
    query_hash BIGINT NOT NULL,
    query_text TEXT NOT NULL,
    plan_id BIGINT NOT NULL,
    runtime_stats_interval_id BIGINT NOT NULL,
    delta_executions BIGINT NOT NULL,
    delta_cpu_ms DOUBLE PRECISION NOT NULL,
    delta_duration_ms DOUBLE PRECISION NOT NULL,
    delta_logical_reads DOUBLE PRECISION NOT NULL,
    avg_cpu_ms DOUBLE PRECISION,
    avg_duration_ms DOUBLE PRECISION,
    avg_reads DOUBLE PRECISION,
    is_reset BOOLEAN DEFAULT FALSE
);
SELECT create_hypertable('monitor.sqlserver_query_store_interval', 'bucket_end', if_not_exists => TRUE);

-- Unique index to support ON CONFLICT (bucket_end, server_id, query_hash, plan_id, runtime_stats_interval_id) DO NOTHING
CREATE UNIQUE INDEX IF NOT EXISTS idx_sqlserver_qs_interval_unique 
ON monitor.sqlserver_query_store_interval (bucket_end, server_id, query_hash, plan_id, runtime_stats_interval_id);

CREATE INDEX IF NOT EXISTS idx_sqlserver_qs_interval_query ON monitor.sqlserver_query_store_interval (server_id, query_hash, bucket_end DESC);

-- --------------------------------------------------------------------------
-- 4.0: COMPATIBILITY VIEWS (Legacy Support)
-- --------------------------------------------------------------------------

-- Instance state for collector watermarks and restart detection
CREATE TABLE IF NOT EXISTS sqlserver_collector_instance_state (
    server_id          UUID PRIMARY KEY,
    last_poll_time_utc   TIMESTAMPTZ,
    sqlserver_start_time TIMESTAMPTZ,
    last_successful_run  TIMESTAMPTZ,
    last_error           TEXT
);

-- Staging table for bulk ingestion (UNLOGGED for performance)
CREATE UNLOGGED TABLE IF NOT EXISTS sqlserver_query_stats_staging_v2 (
    poll_time_utc        TIMESTAMPTZ NOT NULL,
    sqlserver_start_time TIMESTAMPTZ NOT NULL,
    server_id          UUID NOT NULL,
    db_id                INT,
    database_name        TEXT,
    query_hash           BIGINT,
    plan_handle          BYTEA,
    query_text_raw       TEXT,
    statement_text       TEXT,
    total_worker_time    BIGINT,
    total_logical_reads  BIGINT,
    total_logical_writes BIGINT,
    execution_count      BIGINT,
    total_rows           BIGINT,
    total_grant_kb       BIGINT,
    max_worker_time      BIGINT,
    max_logical_reads    BIGINT,
    max_dop              INT,
    max_grant_kb         BIGINT,
    max_rows             BIGINT,
    last_execution_time  TIMESTAMPTZ,
    total_elapsed_ms     BIGINT DEFAULT 0,
    total_physical_reads BIGINT DEFAULT 0
);

ALTER TABLE sqlserver_query_stats_staging_v2 ADD COLUMN IF NOT EXISTS total_elapsed_ms BIGINT DEFAULT 0;
ALTER TABLE sqlserver_query_stats_staging_v2 ADD COLUMN IF NOT EXISTS total_physical_reads BIGINT DEFAULT 0;

-- Snapshot state table for delta calculation
CREATE TABLE IF NOT EXISTS sqlserver_query_stats_snapshot_v2 (
    server_id                UUID NOT NULL,
    db_id                      INT NOT NULL,
    query_hash                 BIGINT NOT NULL,
    plan_handle                BYTEA NOT NULL,
    database_name              TEXT,
    query_text_raw             TEXT,
    statement_text             TEXT,
    last_total_worker_time     BIGINT,
    last_total_logical_reads   BIGINT,
    last_total_logical_writes  BIGINT,
    last_total_execution_count BIGINT,
    last_total_rows            BIGINT,
    last_total_grant_kb        BIGINT,
    max_worker_time            BIGINT,
    max_logical_reads          BIGINT,
    max_dop                    INT,
    max_grant_kb               BIGINT,
    max_rows                   BIGINT,
    last_execution_time        TIMESTAMPTZ,
    last_seen_poll_time        TIMESTAMPTZ,
    last_total_elapsed_ms      BIGINT DEFAULT 0,
    last_total_physical_reads  BIGINT DEFAULT 0,
    CONSTRAINT uq_sqlserver_query_stats_snapshot_v2 UNIQUE (server_id, db_id, query_hash, plan_handle)
);

ALTER TABLE sqlserver_query_stats_snapshot_v2 ADD COLUMN IF NOT EXISTS last_total_elapsed_ms BIGINT DEFAULT 0;
ALTER TABLE sqlserver_query_stats_snapshot_v2 ADD COLUMN IF NOT EXISTS last_total_physical_reads BIGINT DEFAULT 0;

-- History hypertable for persistent metrics
CREATE TABLE IF NOT EXISTS sqlserver_query_stats_history (
    capture_timestamp  TIMESTAMPTZ NOT NULL,
    server_id        UUID NOT NULL,
    db_id              INT,
    query_hash         BIGINT,
    cpu_delta_ms       BIGINT,
    reads_delta        BIGINT,
    writes_delta       BIGINT,
    exec_delta         BIGINT,
    rows_delta         BIGINT,
    period_max_cpu_ms  BIGINT,
    period_max_reads   BIGINT,
    period_max_grant_kb BIGINT,
    period_max_dop     INT
);

SELECT create_hypertable('sqlserver_query_stats_history', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_query_stats_history_instance_ts ON sqlserver_query_stats_history (server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_query_stats_history SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_query_stats_history', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_query_stats_history', INTERVAL '90 days', if_not_exists => TRUE);

-- SQL Server Session Snapshot (Identity Fact Table)
CREATE TABLE IF NOT EXISTS sqlserver_session_snapshot (
    capture_timestamp      TIMESTAMPTZ NOT NULL,
    server_id            UUID NOT NULL,
    session_id             INT,
    login_name             TEXT,
    original_login_name    TEXT,
    host_name              TEXT,
    program_name           TEXT,
    database_name          TEXT,
    is_user_process        BOOLEAN,
    status                 TEXT,
    query_hash             BIGINT,
    query_plan_hash        BIGINT,
    total_elapsed_time_ms  BIGINT,
    cpu_time_ms            BIGINT,
    wait_type              TEXT,
    blocking_session_id    INT,
    query_text             TEXT,
    inserted_at            TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_session_snapshot', 'capture_timestamp', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_session_snapshot_instance_time ON sqlserver_session_snapshot (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_session_snapshot_query_hash ON sqlserver_session_snapshot (query_hash) WHERE query_hash IS NOT NULL;
ALTER TABLE sqlserver_session_snapshot SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_session_snapshot', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_session_snapshot', INTERVAL '14 days', if_not_exists => TRUE);

-- Full 25-column session snapshot used by sqlserver_session_logger.go (plural form)
CREATE TABLE IF NOT EXISTS sqlserver_session_snapshots (
    capture_timestamp           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    server_id                   UUID NOT NULL,
    session_id                  INT,
    login_name                  TEXT,
    original_login_name         TEXT,
    host_name                   TEXT,
    database_name               TEXT,
    program_name                TEXT,
    status                      TEXT,
    is_user_process             BOOLEAN,
    cpu_time_ms                 BIGINT,
    total_elapsed_time_ms       BIGINT,
    memory_usage_pages          INT,
    reads                       BIGINT,
    writes                      BIGINT,
    logical_reads               BIGINT,
    open_transaction_count      INT,
    wait_type                   TEXT,
    wait_time_ms                BIGINT,
    last_wait_type              TEXT,
    wait_resource               TEXT,
    blocking_session_id         INT,
    query_hash                  BIGINT,
    query_plan_hash             BIGINT,
    query_text                  TEXT
);
SELECT create_hypertable('sqlserver_session_snapshots', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_session_snapshots_server_ts
    ON sqlserver_session_snapshots (server_id, capture_timestamp DESC);

-- SQL Server Query Identity Dimension (Identity Bridge)
-- public.sqlserver_query_identity_dim: UNIMPLEMENTED (Reserved for future ontology mapping)
CREATE TABLE IF NOT EXISTS sqlserver_query_identity_dim (
    server_id       UUID NOT NULL,
    query_hash        BIGINT NOT NULL,
    database_name     TEXT,
    login_name        TEXT NOT NULL,
    host_name         TEXT NOT NULL,
    program_name      TEXT NOT NULL,
    first_seen        TIMESTAMPTZ DEFAULT NOW(),
    last_seen         TIMESTAMPTZ DEFAULT NOW(),
    seen_count        BIGINT DEFAULT 1
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_sqlserver_query_identity_dim
    ON sqlserver_query_identity_dim (server_id, query_hash);
CREATE INDEX IF NOT EXISTS idx_sqlserver_query_identity_dim_query_hash ON sqlserver_query_identity_dim (query_hash);

-- SQL Server Query Classification Dimension
CREATE TABLE IF NOT EXISTS sqlserver_query_classification_dim
(
    server_id     UUID NOT NULL,
    query_hash      BIGINT NOT NULL,
    classification  TEXT,      -- USER | SYSTEM | UNKNOWN
    first_seen      TIMESTAMPTZ DEFAULT NOW(),
    last_seen       TIMESTAMPTZ DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_sqlserver_query_class_dim ON sqlserver_query_classification_dim (server_id, query_hash);
CREATE INDEX IF NOT EXISTS idx_sqlserver_query_class_hash ON sqlserver_query_classification_dim (query_hash);

-- Enriched Query Stats View (Joining History with Identity and Classification)
-- Note: Using a standard view for flexibility with joins, or materialized view if needed.
-- For the dashboard charts, we will use this to attribute workload.
CREATE OR REPLACE VIEW sqlserver_query_stats_enriched AS
SELECT
    qh.capture_timestamp AS bucket,
    qh.server_id,
    qh.query_hash,
    qh.cpu_delta_ms,
    qh.reads_delta,
    qh.writes_delta,
    qh.exec_delta,
    qh.rows_delta,
    dim.login_name,
    dim.program_name,
    dim.host_name,
    dim.database_name,
    s.statement_text,
    COALESCE(class.classification, 'UNKNOWN') as classification
FROM sqlserver_query_stats_history qh
LEFT JOIN sqlserver_query_identity_dim dim
    ON qh.server_id = dim.server_id 
   AND qh.query_hash = dim.query_hash
LEFT JOIN sqlserver_query_stats_snapshot_v2 s
    ON qh.server_id = s.server_id
   AND qh.query_hash = s.query_hash
LEFT JOIN sqlserver_query_classification_dim class
    ON qh.server_id = class.server_id
   AND qh.query_hash = class.query_hash
   WHERE s.query_text_raw NOT LIKE '%/* SQL_OPTIMA */ %'
   AND s.query_text_raw NOT LIKE '(@_msparam%'
   AND qh.cpu_delta_ms > 20;

-- SQL Server Query Metrics V2
CREATE TABLE IF NOT EXISTS sqlserver_query_metrics_v2(
 capture_timestamp timestamptz NOT NULL,
 server_id UUID NOT NULL,
 database_name text,
 login_name text,
 application_name text,
 query_hash bigint,
 plan_hash bigint,
 plan_handle BYTEA NOT NULL,
 total_executions bigint,
 total_cpu_ms bigint,
 total_elapsed_ms bigint,
 total_logical_reads bigint,
 total_physical_reads bigint,
 total_rows bigint,
 statement_text text,
 query_text_raw text,
 last_execution_time timestamptz,
 is_user_workload int DEFAULT 1
);
SELECT create_hypertable('sqlserver_query_metrics_v2','capture_timestamp',if_not_exists=>TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_query_metrics_v2_instance_ts ON sqlserver_query_metrics_v2 (server_id, capture_timestamp DESC);
-- Add GIN Trigram indexes for fast filtering on text columns
CREATE INDEX IF NOT EXISTS idx_sqlserver_query_metrics_v2_statement_trgm ON sqlserver_query_metrics_v2 USING GIN (statement_text gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_sqlserver_query_metrics_v2_raw_trgm ON sqlserver_query_metrics_v2 USING GIN (query_text_raw gin_trgm_ops);
ALTER TABLE sqlserver_query_metrics_v2 SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_query_metrics_v2', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_query_metrics_v2', INTERVAL '90 days', if_not_exists => TRUE);
ALTER TABLE sqlserver_query_metrics_v2 ADD COLUMN IF NOT EXISTS total_elapsed_ms BIGINT DEFAULT 0;
ALTER TABLE sqlserver_query_metrics_v2 ADD COLUMN IF NOT EXISTS total_physical_reads BIGINT DEFAULT 0;
ALTER TABLE sqlserver_query_metrics_v2 ADD COLUMN IF NOT EXISTS is_user_workload INT DEFAULT 1;

-- SQL Server Plan Enrichment (Identity mapping for plan_handle)
CREATE TABLE IF NOT EXISTS sqlserver_plan_enrichment (
    server_id      UUID NOT NULL,
    plan_handle      BYTEA NOT NULL,
    login_name       TEXT,
    application_name TEXT,
    database_name    TEXT,
    is_user_workload INT,
    last_seen        TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_sqlserver_plan_enrichment UNIQUE (server_id, plan_handle)
);
CREATE INDEX IF NOT EXISTS idx_sqlserver_plan_enrichment_instance ON sqlserver_plan_enrichment (server_id);

-- PostgreSQL Query Metrics V2
CREATE TABLE IF NOT EXISTS pg_query_metrics_v2(
 capture_timestamp timestamptz NOT NULL,
 server_id UUID NOT NULL,
 datname text,
 usename text,
 application_name text,
 queryid bigint,
 query text,
 calls bigint,
 total_exec_time double precision,
 rows bigint,
 shared_blks_hit bigint,
 shared_blks_read bigint,
 temp_blks_written bigint
);
SELECT create_hypertable('pg_query_metrics_v2','capture_timestamp',if_not_exists=>TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_query_metrics_v2_instance_ts ON pg_query_metrics_v2 (server_id, capture_timestamp DESC);
ALTER TABLE pg_query_metrics_v2 SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('pg_query_metrics_v2', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('pg_query_metrics_v2', INTERVAL '90 days', if_not_exists => TRUE);

-- system_stats_detail view for PgDiskSpaceEvaluator
CREATE OR REPLACE VIEW system_stats_detail AS
SELECT
    capture_timestamp,
    server_id,
    data_disk_mb / 1024.0 AS disk_total_gb,
    (data_disk_mb - free_disk_mb) / 1024.0 AS disk_used_gb
FROM sqlserver_metrics
UNION ALL
SELECT
    now() as capture_timestamp,
    id as server_id,
    0.0 as disk_total_gb,
    0.0 as disk_used_gb
FROM optima_servers
WHERE db_type = 'postgres';



-- PostgreSQL Snapshot Metrics (Wide-row replacement for pg_ts_metrics)
CREATE TABLE IF NOT EXISTS postgres_snapshot_metrics (
    capture_timestamp     TIMESTAMPTZ NOT NULL,
    server_id             UUID NOT NULL,
    tps                   DOUBLE PRECISION DEFAULT 0,
    wal_mb_per_min        DOUBLE PRECISION DEFAULT 0,
    dead_tuple_pct        DOUBLE PRECISION DEFAULT 0,
    replica_lag_sec       DOUBLE PRECISION DEFAULT 0,
    cache_hit_ratio       DOUBLE PRECISION DEFAULT 0,
    checkpoint_req_ratio  DOUBLE PRECISION DEFAULT 0,
    database_size_gb      DOUBLE PRECISION DEFAULT 0,
    temp_bytes_mb         DOUBLE PRECISION DEFAULT 0,
    cpu_usage_pct         DOUBLE PRECISION DEFAULT 0,
    memory_usage_pct      DOUBLE PRECISION DEFAULT 0,
    active_sessions       INTEGER DEFAULT 0,
    idle_sessions         INTEGER DEFAULT 0,
    idle_in_txn_sessions  INTEGER DEFAULT 0,
    waiting_sessions      INTEGER DEFAULT 0,
    health_score          INTEGER DEFAULT 0
);
SELECT create_hypertable('postgres_snapshot_metrics', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_snap_metrics_server_time
    ON postgres_snapshot_metrics (server_id, capture_timestamp DESC);
ALTER TABLE postgres_snapshot_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('postgres_snapshot_metrics', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_snapshot_metrics', INTERVAL '90 days', if_not_exists => TRUE);

-- PostgreSQL Unified Time-Series Metrics (LEGACY - use postgres_snapshot_metrics)
CREATE TABLE IF NOT EXISTS pg_ts_metrics (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    metric text NOT NULL,
    value numeric NOT NULL
);

-- Create hypertable for efficient time-series storage
SELECT create_hypertable('pg_ts_metrics', 'capture_timestamp', if_not_exists => TRUE);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_pg_ts_metrics_lookup ON pg_ts_metrics (server_id, metric, capture_timestamp DESC);
ALTER TABLE pg_ts_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,metric',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('pg_ts_metrics', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('pg_ts_metrics', INTERVAL '30 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- 5.0: POSTGRESQL ENHANCED METRICS (pg_stat_monitor)
-- --------------------------------------------------------------------------

-- Collector state table for pg_stat_monitor to ensure idempotency.
CREATE TABLE IF NOT EXISTS pg_collector_bucket_state (
    server_id UUID PRIMARY KEY,
    last_bucket_collected bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- TimescaleDB hypertable for storing pg_stat_monitor bucketed query metrics.
CREATE TABLE IF NOT EXISTS pg_query_bucket_metrics (
    bucket_start timestamptz NOT NULL,
    bucket_end timestamptz NOT NULL,
    server_id UUID NOT NULL,
    dbid oid,
    userid oid,
    queryid bigint,
    query text,
    application_name text,
    client_ip inet,
    calls bigint,
    total_exec_time double precision,
    mean_exec_time double precision,
    min_exec_time double precision,
    max_exec_time double precision,
    stddev_exec_time double precision,
    rows bigint,
    shared_blks_hit bigint,
    shared_blks_read bigint,
    temp_blks_written bigint,
    wal_bytes numeric
);

-- Convert to hypertable
SELECT create_hypertable(
 'pg_query_bucket_metrics','bucket_start', if_not_exists=>TRUE
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_pg_query_bucket_instance_time ON pg_query_bucket_metrics (server_id, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_pg_query_bucket_queryid ON pg_query_bucket_metrics (queryid);

-- Instance metadata table for PostgreSQL to store capabilities and selected sources.
CREATE TABLE IF NOT EXISTS pg_instance (
    server_id UUID PRIMARY KEY,
    query_stats_source text,
    last_detected_at timestamptz DEFAULT now()
);

-- --------------------------------------------------------------------------
-- Phase 8: PostgreSQL TimescaleDB Migration (High Performance)
-- --------------------------------------------------------------------------

-- 2.1: PostgreSQL Locks & Contention (pg_ts_locks)
CREATE TABLE IF NOT EXISTS pg_ts_locks (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT,
    pid INTEGER,
    wait_event_type TEXT,
    wait_event TEXT,
    lock_type TEXT,
    mode TEXT,
    granted BOOLEAN,
    query_text TEXT,
    blocked_by INTEGER,
    wait_duration_ms DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('pg_ts_locks', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_ts_locks_server_time ON pg_ts_locks (server_id, capture_timestamp DESC);

ALTER TABLE pg_ts_locks SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('pg_ts_locks', INTERVAL '7 days', if_not_exists => TRUE);

-- 2.2: PostgreSQL Stat Statements Deltas (pg_ts_stat_statements_delta)
CREATE TABLE IF NOT EXISTS pg_ts_stat_statements_delta (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    query_id BIGINT NOT NULL,
    database_name TEXT,
    user_name TEXT,
    calls_delta BIGINT,
    total_time_delta_ms DOUBLE PRECISION,
    rows_delta BIGINT,
    shared_blks_hit_delta BIGINT,
    shared_blks_read_delta BIGINT,
    shared_blks_dirtied_delta BIGINT,
    shared_blks_written_delta BIGINT,
    temp_blks_read_delta BIGINT,
    temp_blks_written_delta BIGINT,
    blk_read_time_delta_ms DOUBLE PRECISION,
    blk_write_time_delta_ms DOUBLE PRECISION,
    wal_bytes_delta NUMERIC,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('pg_ts_stat_statements_delta', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_ts_stat_server_query ON pg_ts_stat_statements_delta (server_id, query_id, capture_timestamp DESC);

ALTER TABLE pg_ts_stat_statements_delta SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,query_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('pg_ts_stat_statements_delta', INTERVAL '7 days', if_not_exists => TRUE);

-- 2.3: PostgreSQL Instance Engine Snapshot (pg_ts_instance_snapshot)
CREATE TABLE IF NOT EXISTS pg_ts_instance_snapshot (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    health_score INTEGER,
    total_connections INTEGER,
    active_sessions INTEGER,
    idle_sessions INTEGER,
    waiting_sessions INTEGER,
    blocked_sessions INTEGER,
    longest_active_ms DOUBLE PRECISION,
    tps DOUBLE PRECISION,
    cache_hit_ratio DOUBLE PRECISION,
    rw_ratio DOUBLE PRECISION,
    avg_query_latency_ms DOUBLE PRECISION,
    wal_generation_rate_mbps DOUBLE PRECISION,
    replica_lag_sec DOUBLE PRECISION,
    max_xid_age BIGINT,
    checkpoint_req_ratio DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('pg_ts_instance_snapshot', 'capture_timestamp', if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_pg_ts_snap_server_time ON pg_ts_instance_snapshot (server_id, capture_timestamp DESC);

ALTER TABLE pg_ts_instance_snapshot SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('pg_ts_instance_snapshot', INTERVAL '30 days', if_not_exists => TRUE);

COMMENT ON TABLE pg_ts_locks IS 'Historical PostgreSQL lock contention telemetry.';
COMMENT ON TABLE pg_ts_stat_statements_delta IS 'Differential query performance metrics from pg_stat_statements.';
COMMENT ON TABLE pg_ts_instance_snapshot IS 'Unified historical engine health snapshots for PostgreSQL dashboards.';


-- PostgreSQL Delta-based Query Metrics
-- Created for the refactored pg_stat_statements collector

CREATE TABLE IF NOT EXISTS pg_query_metrics (
    capture_timestamp        TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    queryid            BIGINT,
    delta_calls        BIGINT,
    delta_exec_ms      DOUBLE PRECISION,
    delta_rows         BIGINT,
    delta_shared_reads BIGINT,
    delta_shared_hits  BIGINT,
    delta_temp_written BIGINT,
    delta_wal_bytes    BIGINT
);

-- Convert to hypertable for efficient time-series storage
SELECT create_hypertable('pg_query_metrics', 'capture_timestamp', if_not_exists => TRUE);

-- Add index for dashboard performance
CREATE INDEX IF NOT EXISTS idx_pg_query_metrics_instance_time ON pg_query_metrics (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_query_metrics_queryid ON pg_query_metrics (queryid);
-- ============================================================================
-- SECTION 1.3: SQL SERVER - Blocking & Deadlock (Re-Architecture)
-- ============================================================================

-- Core blocking snapshots (Hypertables)
CREATE TABLE IF NOT EXISTS sqlserver_blocking_snapshots (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    session_id INT,
    blocking_session_id INT,
    wait_type TEXT,
    wait_duration_ms BIGINT,
    database_name TEXT,
    sql_hash BIGINT,
    plan_hash BIGINT,
    login_name TEXT,
    host_name TEXT,
    program_name TEXT,
    open_transaction_count INT,
    transaction_start_time TIMESTAMPTZ,
    transaction_isolation_level TEXT,
    cpu_time BIGINT,
    reads BIGINT,
    writes BIGINT,
    logical_reads BIGINT,
    memory_usage BIGINT,
    transaction_log_bytes_used BIGINT,
    transaction_log_bytes_reserved BIGINT,
    wait_resource TEXT,
    percent_complete DOUBLE PRECISION
);
SELECT create_hypertable('sqlserver_blocking_snapshots', 'capture_timestamp', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_blocking_snaps_sqlhash
ON sqlserver_blocking_snapshots (server_id, sql_hash, capture_timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_sqlserver_blocking_snaps_sid
ON sqlserver_blocking_snapshots (server_id, session_id, capture_timestamp DESC);

-- Partial index for root-blocker recurrence queries (7-day login lookup)
CREATE INDEX IF NOT EXISTS idx_sqlserver_blocking_snaps_login
ON sqlserver_blocking_snapshots (server_id, login_name, capture_timestamp DESC)
WHERE blocking_session_id = 0;

CREATE TABLE IF NOT EXISTS sqlserver_blocking_locks (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    session_id INT,
    resource_type TEXT,
    request_mode TEXT,
    request_status TEXT,
    resource_description TEXT,
    wait_duration_ms BIGINT
);
SELECT create_hypertable('sqlserver_blocking_locks', 'capture_timestamp', if_not_exists => TRUE);

-- Index for time-range scans used by GetSQLServerMostBlockedObjects
CREATE INDEX IF NOT EXISTS idx_sqlserver_blocking_locks_instance_ts
ON sqlserver_blocking_locks (server_id, capture_timestamp DESC);

-- Deadlock events (Hypertable)
CREATE TABLE IF NOT EXISTS sqlserver_deadlock_events (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_name TEXT,
    victim_session_id INT,
    victim_sql_hash BIGINT,
    deadlock_graph XML
);
SELECT create_hypertable('sqlserver_deadlock_events', 'capture_timestamp', if_not_exists => TRUE);

-- Unique index enabling ON CONFLICT DO NOTHING for idempotent deadlock inserts.
-- Prevents duplicate rows when the collector restarts and re-reads the same 24h XE window.
-- capture_timestamp is the TimescaleDB partition key and must be included in the unique index.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sqlserver_deadlock_events_dedup
ON sqlserver_deadlock_events (capture_timestamp, server_id, victim_session_id);

-- Dimension tables (Regular tables for deduplication)
CREATE TABLE IF NOT EXISTS sqlserver_text_dim (
    sql_hash BIGINT PRIMARY KEY,
    sql_text TEXT
);

CREATE TABLE IF NOT EXISTS sqlserver_query_plan_dim (
    plan_hash BIGINT PRIMARY KEY,
    query_plan XML
);

-- Retention & Compression Policies
ALTER TABLE sqlserver_blocking_snapshots SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id'
);
SELECT add_compression_policy('sqlserver_blocking_snapshots', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_blocking_snapshots', INTERVAL '30 days', if_not_exists => TRUE);

ALTER TABLE sqlserver_blocking_locks SET (
    timescaledb.compress = true
);
SELECT add_compression_policy('sqlserver_blocking_locks', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_blocking_locks', INTERVAL '14 days', if_not_exists => TRUE);

ALTER TABLE sqlserver_deadlock_events SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id'
);
SELECT add_compression_policy('sqlserver_deadlock_events', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_deadlock_events', INTERVAL '180 days', if_not_exists => TRUE);

-- 3.3.4: Incident tracking for SQL Server (stateful; not a hypertable)
CREATE TABLE IF NOT EXISTS sqlserver_blocking_incidents (
    incident_id BIGSERIAL PRIMARY KEY,
    server_id UUID NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    root_blocker_pid INT,
    root_blocker_query TEXT,
    peak_blocked_sessions INT DEFAULT 0,
    status TEXT DEFAULT 'active' -- 'active' | 'resolved'
);
CREATE INDEX IF NOT EXISTS idx_sqlserver_blocking_incidents_server_started ON sqlserver_blocking_incidents (server_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_blocking_incidents_status ON sqlserver_blocking_incidents (server_id, status, started_at DESC) WHERE status = 'active';

-- 3.3.5: Blocking pairs for SQL Server (dependency graph edges; hypertable)
CREATE TABLE IF NOT EXISTS sqlserver_blocking_pairs (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    blocked_spid INT NOT NULL,
    blocking_spid INT NOT NULL
);
SELECT create_hypertable('sqlserver_blocking_pairs', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_blocking_pairs_lookup ON sqlserver_blocking_pairs (server_id, capture_timestamp DESC);

-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Standardized SQL Server enterprise metrics for time-series driven diagnostics.
-- This schema supports trend-first observability for wait stats, perf counters, I/O, and memory.
--
-- Metadata:
-- Engine: SQL Server
-- Tier: Enterprise / Advanced Monitoring
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

-- Robust cleanup of existing objects
DROP MATERIALIZED VIEW IF EXISTS sqlserver_ca_wait_stats_hourly CASCADE;

-- 1) WAIT STATS (DELTA)
DROP TABLE IF EXISTS sqlserver_wait_stats CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_wait_stats (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_id UUID NOT NULL,
    wait_category TEXT NOT NULL,
    wait_time_ms BIGINT DEFAULT 0,
    signal_wait_time_ms BIGINT DEFAULT 0,
    waiting_tasks BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_wait_stats', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_wait_stats_instance_time ON sqlserver_wait_stats (server_id, capture_timestamp DESC);

-- 2) PERF COUNTERS (DELTA)
DROP TABLE IF EXISTS sqlserver_perf_counters CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_perf_counters (
    capture_timestamp TIMESTAMPTZ       NOT NULL,
    metric_time       TIMESTAMPTZ       GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_id         UUID              NOT NULL,
    counter_name      TEXT              NOT NULL,
    instance_name     VARCHAR(128)      NOT NULL DEFAULT '',
    cntr_value        BIGINT            NOT NULL DEFAULT 0,
    cntr_type         INT               NOT NULL DEFAULT 0,
    value_per_sec     DOUBLE PRECISION  DEFAULT 0,   -- kept for backward compat; stores rate_per_sec
    inserted_at       TIMESTAMPTZ       DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_perf_counters', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_perf_counters_instance_time ON sqlserver_perf_counters (server_id, counter_name, capture_timestamp DESC);
-- Unique index enables ON CONFLICT DO NOTHING deduplication
CREATE UNIQUE INDEX IF NOT EXISTS idx_pc_dedup
    ON sqlserver_perf_counters (server_id, counter_name, instance_name, capture_timestamp);

-- 3) HEALTH KPIs (V2)
DROP TABLE IF EXISTS sqlserver_health_kpis_v2 CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_health_kpis_v2 (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    sql_cpu_pct DOUBLE PRECISION,
    runnable_tasks INTEGER,
    mem_grants_pending INTEGER,
    page_reads_per_sec DOUBLE PRECISION,
    log_write_wait_ms DOUBLE PRECISION,
    batch_requests DOUBLE PRECISION,
    compilations DOUBLE PRECISION,
    blocked_sessions INTEGER,
    user_connections INTEGER,
    instance_status TEXT,
    edition TEXT,
    uptime_seconds BIGINT,
    logins_per_sec DOUBLE PRECISION,
    target_server_memory_mb DOUBLE PRECISION,
    total_server_memory_mb DOUBLE PRECISION,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
SELECT create_hypertable('sqlserver_health_kpis_v2', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_health_kpis_instance_time ON sqlserver_health_kpis_v2 (server_id, capture_timestamp DESC);

-- 3) FILE IO STATS (DELTA)
DROP TABLE IF EXISTS sqlserver_file_io CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_file_io (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_id UUID NOT NULL,
    database_name TEXT NOT NULL,
    file_name TEXT,
    file_type TEXT NOT NULL,
    read_latency_ms DOUBLE PRECISION DEFAULT 0,
    write_latency_ms DOUBLE PRECISION DEFAULT 0,
    read_bytes_per_sec DOUBLE PRECISION DEFAULT 0,
    write_bytes_per_sec DOUBLE PRECISION DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_file_io', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_file_io_instance_time ON sqlserver_file_io (server_id, capture_timestamp DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sqlserver_file_io_dedup
    ON sqlserver_file_io (server_id, database_name, file_name, capture_timestamp);

-- File catalog: static per-file metadata (physical path, size, type).
-- Refreshed every 6 hours; ON CONFLICT deduplication avoids re-inserting unchanged rows.
CREATE TABLE IF NOT EXISTS sqlserver_file_catalog (
    capture_timestamp   TIMESTAMPTZ  NOT NULL,
    server_id           UUID         NOT NULL,
    database_id         INT          NOT NULL,
    db_name             TEXT         NOT NULL,
    file_id             INT          NOT NULL,
    type_desc           TEXT         NOT NULL DEFAULT '',
    physical_name       TEXT         NOT NULL DEFAULT '',
    size_mb             NUMERIC(19,4) DEFAULT 0,
    max_size_mb         NUMERIC(19,4) DEFAULT -1,
    is_percent_growth   BOOLEAN      DEFAULT false,
    growth              INT          DEFAULT 0
);
SELECT create_hypertable('sqlserver_file_catalog', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_file_catalog_server_time ON sqlserver_file_catalog (server_id, capture_timestamp DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_file_catalog_dedup
    ON sqlserver_file_catalog (server_id, database_id, file_id, capture_timestamp);
SELECT add_retention_policy('sqlserver_file_catalog', INTERVAL '30 days', if_not_exists => TRUE);

-- 4) PLAN CACHE SNAPSHOT
DROP TABLE IF EXISTS sqlserver_plan_cache CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_plan_cache (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_id UUID NOT NULL,
    cache_type TEXT NOT NULL,
    size_mb NUMERIC(19,4) DEFAULT 0,
    total_cache_mb NUMERIC(19,4) DEFAULT 0,
    single_use_cache_mb NUMERIC(19,4) DEFAULT 0,
    single_use_cache_pct NUMERIC(19,4) DEFAULT 0,
    adhoc_cache_mb NUMERIC(19,4) DEFAULT 0,
    prepared_cache_mb NUMERIC(19,4) DEFAULT 0,
    proc_cache_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_plan_cache', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_plan_cache_instance_time ON sqlserver_plan_cache (server_id, capture_timestamp DESC);

-- 5) MEMORY CLERKS SNAPSHOT
DROP TABLE IF EXISTS sqlserver_memory_clerks CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_memory_clerks (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_id UUID NOT NULL,
    clerk_name TEXT NOT NULL,
    clerk_type TEXT GENERATED ALWAYS AS (clerk_name) STORED,
    memory_node SMALLINT DEFAULT 0,
    pages_mb NUMERIC(19,4) DEFAULT 0,
    virtual_memory_reserved_mb NUMERIC(19,4) DEFAULT 0,
    virtual_memory_committed_mb NUMERIC(19,4) DEFAULT 0,
    awe_memory_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_memory_clerks', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_memory_clerks_instance_time ON sqlserver_memory_clerks (server_id, capture_timestamp DESC);

-- 6) MEMORY GRANTS SNAPSHOT
DROP TABLE IF EXISTS sqlserver_memory_grants CASCADE;
CREATE TABLE IF NOT EXISTS sqlserver_memory_grants (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    metric_time TIMESTAMPTZ GENERATED ALWAYS AS (capture_timestamp) STORED,
    server_id UUID NOT NULL,
    pending_grants INTEGER DEFAULT 0,
    active_grants INTEGER DEFAULT 0,
    granted_memory_mb NUMERIC(19,4) DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('sqlserver_memory_grants', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_memory_grants_instance_time ON sqlserver_memory_grants (server_id, capture_timestamp DESC);

-- 7) TEMPDB TOP CONSUMERS SNAPSHOT
-- ============================================================================
-- WAIT STATS DASHBOARD V2 — New parallel pipeline
-- Does NOT modify sqlserver_wait_stats, sqlserver_wait_history, or any
-- existing table. Safe to run as an idempotent migration.
-- ============================================================================

-- Table 1: Raw cumulative snapshot (per wait_type)
CREATE TABLE IF NOT EXISTS sqlserver_wait_stats_cumulative (
    capture_timestamp      TIMESTAMPTZ  NOT NULL,
    server_id              UUID         NOT NULL,
    wait_type              TEXT         NOT NULL,
    waiting_tasks_count    BIGINT       NOT NULL DEFAULT 0,
    wait_time_ms           BIGINT       NOT NULL DEFAULT 0,
    signal_wait_time_ms    BIGINT       NOT NULL DEFAULT 0,
    resource_wait_time_ms  BIGINT       GENERATED ALWAYS AS
                               (GREATEST(wait_time_ms - signal_wait_time_ms, 0)) STORED
);
SELECT create_hypertable('sqlserver_wait_stats_cumulative','capture_timestamp',
    if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_ws_cum_server_time
    ON sqlserver_wait_stats_cumulative (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ws_cum_type_server
    ON sqlserver_wait_stats_cumulative (wait_type, server_id, capture_timestamp DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sqlserver_wait_cumulative_dedup
    ON sqlserver_wait_stats_cumulative (server_id, wait_type, capture_timestamp);
ALTER TABLE sqlserver_wait_stats_cumulative SET (
    timescaledb.compress           = true,
    timescaledb.compress_segmentby = 'server_id, wait_type',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_wait_stats_cumulative',
    INTERVAL '3 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_wait_stats_cumulative',
    INTERVAL '30 days', if_not_exists => TRUE);

-- Table 2: Computed interval deltas (per wait_type, restart-safe)
CREATE TABLE IF NOT EXISTS sqlserver_wait_stats_delta (
    capture_timestamp      TIMESTAMPTZ  NOT NULL,
    server_id              UUID         NOT NULL,
    wait_type              TEXT         NOT NULL,
    wait_category          TEXT         NOT NULL DEFAULT 'OTHER',
    delta_wait_ms          BIGINT       NOT NULL DEFAULT 0,
    delta_signal_wait_ms   BIGINT       NOT NULL DEFAULT 0,
    delta_resource_wait_ms BIGINT       NOT NULL DEFAULT 0,
    delta_waiting_tasks    BIGINT       NOT NULL DEFAULT 0,
    restart_detected       BOOLEAN      NOT NULL DEFAULT FALSE
);
SELECT create_hypertable('sqlserver_wait_stats_delta','capture_timestamp',
    if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_ws_delta_server_time
    ON sqlserver_wait_stats_delta (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ws_delta_category
    ON sqlserver_wait_stats_delta (wait_category, server_id, capture_timestamp DESC);
ALTER TABLE sqlserver_wait_stats_delta SET (
    timescaledb.compress           = true,
    timescaledb.compress_segmentby = 'server_id, wait_type',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_wait_stats_delta',
    INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_wait_stats_delta',
    INTERVAL '90 days', if_not_exists => TRUE);

-- Table 3: Enriched active wait sessions (real-time, 7-day retention)
CREATE TABLE IF NOT EXISTS sqlserver_active_wait_sessions (
    capture_timestamp   TIMESTAMPTZ  NOT NULL,
    server_id           UUID         NOT NULL,
    session_id          INTEGER      NOT NULL,
    wait_type           TEXT         NOT NULL,
    wait_duration_ms    BIGINT       NOT NULL DEFAULT 0,
    blocking_session_id INTEGER,
    database_name       TEXT,
    host_name           TEXT,
    program_name        TEXT,
    login_name          TEXT,
    query_text          TEXT
);
SELECT create_hypertable('sqlserver_active_wait_sessions','capture_timestamp',
    if_not_exists => TRUE, migrate_data => FALSE);
CREATE INDEX IF NOT EXISTS idx_ws_active_server_time
    ON sqlserver_active_wait_sessions (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ws_active_blocker
    ON sqlserver_active_wait_sessions (blocking_session_id, capture_timestamp DESC)
    WHERE blocking_session_id IS NOT NULL;
ALTER TABLE sqlserver_active_wait_sessions SET (
    timescaledb.compress           = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_active_wait_sessions',
    INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_active_wait_sessions',
    INTERVAL '7 days', if_not_exists => TRUE);

-- Table 4: Wait type → category mapping (lookup)
CREATE TABLE IF NOT EXISTS sqlserver_wait_type_category (
    wait_type  TEXT PRIMARY KEY,
    category   TEXT NOT NULL,
    sort_order SMALLINT NOT NULL DEFAULT 99
);

-- Table 5: Wait type help text for dashboard tooltips
CREATE TABLE IF NOT EXISTS sqlserver_wait_type_help (
    wait_type             TEXT PRIMARY KEY,
    description           TEXT NOT NULL,
    likely_cause          TEXT NOT NULL,
    recommended_action    TEXT NOT NULL,
    threshold_warning_ms  INTEGER,
    threshold_critical_ms INTEGER
);

-- Continuous Aggregate 1: Hourly rollup (per wait_type)
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_cagg_wait_delta_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', capture_timestamp) AS bucket,
    server_id,
    wait_type,
    wait_category,
    SUM(delta_wait_ms)          AS total_wait_ms,
    SUM(delta_signal_wait_ms)   AS total_signal_wait_ms,
    SUM(delta_resource_wait_ms) AS total_resource_wait_ms,
    SUM(delta_waiting_tasks)    AS total_waiting_tasks,
    MAX(delta_wait_ms)          AS peak_wait_ms,
    BOOL_OR(restart_detected)   AS had_restart
FROM sqlserver_wait_stats_delta
GROUP BY bucket, server_id, wait_type, wait_category
WITH NO DATA;

SELECT add_continuous_aggregate_policy('sqlserver_cagg_wait_delta_1h',
    start_offset => INTERVAL '2 days', end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour', if_not_exists => TRUE);
ALTER MATERIALIZED VIEW sqlserver_cagg_wait_delta_1h SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id, wait_type');
SELECT add_compression_policy('sqlserver_cagg_wait_delta_1h',
    INTERVAL '30 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_cagg_wait_delta_1h',
    INTERVAL '365 days', if_not_exists => TRUE);

-- Continuous Aggregate 2: Daily rollup (heatmap source)
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_cagg_wait_delta_1d
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', bucket)  AS bucket,
    server_id,
    wait_type,
    wait_category,
    SUM(total_wait_ms)            AS total_wait_ms,
    SUM(total_signal_wait_ms)     AS total_signal_wait_ms,
    SUM(total_resource_wait_ms)   AS total_resource_wait_ms,
    SUM(total_waiting_tasks)      AS total_waiting_tasks,
    MAX(peak_wait_ms)             AS peak_wait_ms,
    BOOL_OR(had_restart)          AS had_restart
FROM sqlserver_cagg_wait_delta_1h
GROUP BY time_bucket('1 day', bucket), server_id, wait_type, wait_category
WITH NO DATA;

SELECT add_continuous_aggregate_policy('sqlserver_cagg_wait_delta_1d',
    start_offset => INTERVAL '90 days', end_offset => INTERVAL '7 days',
    schedule_interval => INTERVAL '7 days', if_not_exists => TRUE);
ALTER MATERIALIZED VIEW sqlserver_cagg_wait_delta_1d SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id, wait_type');
SELECT add_compression_policy('sqlserver_cagg_wait_delta_1d',
    INTERVAL '180 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_cagg_wait_delta_1d',
    INTERVAL '1825 days', if_not_exists => TRUE); -- 5 years

-- Convenience view: last 5 minutes of active waits (dashboard live table)
CREATE OR REPLACE VIEW sqlserver_vw_active_waits_live AS
    SELECT * FROM sqlserver_active_wait_sessions
    WHERE capture_timestamp > NOW() - INTERVAL '5 minutes';

-- --------------------------------------------------------------------------
-- LEGACY ALIASES (VIEWS)
-- These allow existing Go code to work while we transition to capture_timestamp naming.
-- --------------------------------------------------------------------------

CREATE OR REPLACE VIEW sqlserver_waits_delta AS 
SELECT capture_timestamp, server_id, wait_category, wait_time_ms as wait_time_ms_delta, signal_wait_time_ms, waiting_tasks
FROM sqlserver_wait_stats;

CREATE OR REPLACE VIEW sqlserver_file_io_latency AS
SELECT capture_timestamp, server_id, database_name, file_name, file_type, read_latency_ms, write_latency_ms, read_bytes_per_sec, write_bytes_per_sec
FROM sqlserver_file_io;

CREATE OR REPLACE VIEW sqlserver_plan_cache_health AS
SELECT capture_timestamp, server_id, total_cache_mb, single_use_cache_mb, single_use_cache_pct, adhoc_cache_mb, prepared_cache_mb, proc_cache_mb
FROM sqlserver_plan_cache;

-- --------------------------------------------------------------------------
-- CONTINUOUS AGGREGATES (Standard Downsampling)
-- --------------------------------------------------------------------------

-- Wait Stats Hourly
CREATE MATERIALIZED VIEW IF NOT EXISTS sqlserver_ca_wait_stats_hourly
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 hour', capture_timestamp) AS bucket,
  server_id,
  wait_category,
  avg(wait_time_ms) AS avg_wait_ms,
  sum(wait_time_ms) AS total_wait_ms
FROM sqlserver_wait_stats
GROUP BY bucket, server_id, wait_category
WITH NO DATA;

-- --------------------------------------------------------------------------
-- Compression Policies
-- --------------------------------------------------------------------------
ALTER TABLE sqlserver_wait_stats SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,wait_category',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_wait_stats', INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE sqlserver_perf_counters SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,counter_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_perf_counters', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_perf_counters', INTERVAL '90 days', if_not_exists => TRUE);

ALTER TABLE sqlserver_file_io SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,database_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('sqlserver_file_io', INTERVAL '7 days', if_not_exists => TRUE);


-- SQL Optima — PostgreSQL Control Center Revamp
-- Purpose: Schema for high-speed dashboard snapshot and trend tracking.

-- 1. Snapshot Table (Single Row Per Instance)
CREATE TABLE IF NOT EXISTS pg_instance_snapshot (
    server_id UUID PRIMARY KEY,
    capture_timestamp timestamptz DEFAULT now(),
    -- workload
    tps numeric,
    active_sessions int,
    idle_sessions int,
    idle_in_tx_sessions int,
    blocked_sessions int,
    -- cpu & memory
    cpu_usage numeric,
    shared_buffers_used_pct numeric,
    cache_hit_ratio numeric,
    -- durability
    wal_mb_per_min numeric,
    checkpoints_timed int,
    checkpoints_req int,
    checkpoint_write_time numeric,
    -- risk
    max_xid_age bigint,
    oldest_tx_age_sec bigint,
    -- storage
    database_size_gb numeric,
    temp_bytes_mb numeric,
    -- vacuum
    autovacuum_workers int,
    dead_tuple_pct numeric,
    -- replication
    replica_lag_sec numeric,
    replication_slots int,
    -- health
    health_score int,
    version text,
    uptime text,
    checkpoint_req_ratio numeric DEFAULT 0
);

-- 2. Time-Series Hypertables

-- ============================================================================
-- SQL Optima: New Postgres Dashboards (Waits, Backup, Security)
-- ============================================================================

CREATE SCHEMA IF NOT EXISTS monitor;
SET search_path TO monitor, public;

-- --------------------------------------------------------------------------
-- DASHBOARD 1: Waits, Bottlenecks & Sessions
-- --------------------------------------------------------------------------

-- 1. Active session snapshot
CREATE TABLE IF NOT EXISTS monitor.pg_session_activity_ts (
    capture_timestamp timestamptz NOT NULL DEFAULT now(),
    server_id UUID NOT NULL,
    dbname text,
    pid int,
    usename text,
    application_name text,
    client_addr inet,
    state text,
    wait_event_type text,
    wait_event text,
    backend_type text,
    query_id bigint,
    query text,
    xact_start timestamptz,
    query_start timestamptz,
    state_change timestamptz,
    backend_start timestamptz
);
SELECT create_hypertable('monitor.pg_session_activity_ts', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_session_activity_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- 2. Wait event aggregation
CREATE TABLE IF NOT EXISTS monitor.pg_wait_event_summary_ts (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    wait_event_type text,
    wait_event text,
    sessions int,
    state text
);
SELECT create_hypertable('monitor.pg_wait_event_summary_ts', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_wait_event_summary_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- 3. Database load (AAS approximation)
CREATE TABLE IF NOT EXISTS monitor.pg_db_load_ts (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    active_sessions int,
    cpu_sessions int,
    waiting_sessions int,
    io_sessions int,
    lock_sessions int,
    idle_in_txn int
);
SELECT create_hypertable('monitor.pg_db_load_ts', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_db_load_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- Unified operational incident feed — backing store for the dashboard Incident Feed widget.
-- Replaces live blocking-tree + queries API calls from the frontend.
-- Retention: 7 days (incidents older than this have no operational value).
CREATE TABLE IF NOT EXISTS monitor.pg_incident_feed_ts (
    capture_timestamp TIMESTAMPTZ      NOT NULL DEFAULT now(),
    server_id     UUID             NOT NULL,
    incident_type   TEXT             NOT NULL,  -- 'blocking','long_query','deadlock','replica_lag','connection_saturation'
    severity        TEXT             NOT NULL,  -- 'critical','warning','info'
    pid             INT,
    blocked_count   INT              DEFAULT 0,
    duration_ms     DOUBLE PRECISION DEFAULT 0,
    usename         TEXT,
    datname         TEXT,
    query_snippet   TEXT,
    detail_json     JSONB
);
SELECT create_hypertable('monitor.pg_incident_feed_ts', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_incident_feed_ts', INTERVAL '7 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_incident_feed_lookup
    ON monitor.pg_incident_feed_ts (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_incident_feed_severity
    ON monitor.pg_incident_feed_ts (server_id, severity, capture_timestamp DESC);
COMMENT ON TABLE monitor.pg_incident_feed_ts IS
    'Unified operational incident feed — blocking events, long queries, deadlock spikes — captured at collection time. No live dashboard queries.';

-- 4. Top queries by waits (pg_stat_statements)
CREATE TABLE IF NOT EXISTS monitor.pg_query_wait_profile_ts (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    queryid bigint,
    calls bigint,
    total_exec_time double precision,
    mean_exec_time double precision,
    rows bigint,
    shared_blks_hit bigint,
    shared_blks_read bigint,
    temp_blks_written bigint,
    query text,
    usename text
);
SELECT create_hypertable('monitor.pg_query_wait_profile_ts', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_query_wait_profile_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- DASHBOARD 2: Backup & Disaster Recovery
-- --------------------------------------------------------------------------

-- 1. Backup history (pg_stat_archiver)
CREATE TABLE IF NOT EXISTS monitor.pg_backup_archiver_ts (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    archived_count bigint,
    failed_count bigint,
    last_archived_time timestamptz,
    last_failed_time timestamptz
);
SELECT create_hypertable('monitor.pg_backup_archiver_ts', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_backup_archiver_ts', INTERVAL '90 days', if_not_exists => TRUE);

-- 2. WAL generation rate
CREATE TABLE IF NOT EXISTS monitor.pg_wal_rate_ts (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    wal_bytes numeric
);
SELECT create_hypertable('monitor.pg_wal_rate_ts', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_wal_rate_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- 3. Base backup detection (from pg_stat_bgwriter/checkpoints)
CREATE TABLE IF NOT EXISTS monitor.pg_basebackup_history (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    checkpoint_time timestamptz,
    checkpoint_write_time double precision
);
SELECT create_hypertable('monitor.pg_basebackup_history', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_basebackup_history', INTERVAL '90 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- DASHBOARD 3: Security Monitoring
-- --------------------------------------------------------------------------

-- 1. Role & privilege snapshot
CREATE TABLE IF NOT EXISTS monitor.pg_roles_snapshot (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    rolname text,
    rolsuper bool,
    rolcreatedb bool,
    rolcreaterole bool,
    rolreplication bool,
    rolcanlogin bool
);
SELECT create_hypertable('monitor.pg_roles_snapshot', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_roles_snapshot', INTERVAL '90 days', if_not_exists => TRUE);

-- 2. Failed logins
CREATE TABLE IF NOT EXISTS monitor.pg_failed_login_events (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    username text,
    client_addr text,
    message text
);
SELECT create_hypertable('monitor.pg_failed_login_events', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_failed_login_events', INTERVAL '90 days', if_not_exists => TRUE);

-- 3. DDL audit snapshot
CREATE TABLE IF NOT EXISTS monitor.pg_ddl_activity_ts (
    capture_timestamp timestamptz NOT NULL,
    server_id UUID NOT NULL,
    schemaname text,
    relname text,
    n_tup_ins bigint,
    n_tup_upd bigint,
    n_tup_del bigint
);
SELECT create_hypertable('monitor.pg_ddl_activity_ts', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_ddl_activity_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- Grant permissions
GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA monitor TO sql_optima_app;
-- ============================================================================
-- SQL Optima: Postgres Control Center Dashboards (Aggregations)
-- ============================================================================
-- Purpose: Provides materialized views and real-time aggregations for the 
--          v2 PostgreSQL Control Center observability dashboard.
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE SCHEMA IF NOT EXISTS monitor;
SET search_path TO monitor, public;

-- --------------------------------------------------------------------------
-- 1. Database Load (AAS Approximation View)
-- Supports the Hero "Database Pressure" Stacked Area Chart.
-- --------------------------------------------------------------------------
CREATE OR REPLACE VIEW monitor.pg_db_load_summary AS
SELECT 
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_id,
    ROUND(AVG(active_sessions), 2) AS avg_active_sessions,
    ROUND(AVG(cpu_sessions), 2) AS avg_cpu_sessions,
    ROUND(AVG(waiting_sessions), 2) AS avg_waiting_sessions,
    ROUND(AVG(idle_in_txn), 2) AS avg_idle_in_txn
FROM 
    monitor.pg_db_load_ts
WHERE 
    capture_timestamp > NOW() - INTERVAL '24 hours'
GROUP BY 
    bucket, server_id
ORDER BY 
    bucket DESC;

-- --------------------------------------------------------------------------
-- 2. Top Wait Categories Summary
-- Supports the "Wait Categories" Pie Chart
-- --------------------------------------------------------------------------
CREATE OR REPLACE VIEW monitor.pg_wait_categories_summary AS
SELECT 
    server_id,
    CASE 
        WHEN wait_event_type = 'LWLock' THEN 'LWLock'
        WHEN wait_event_type = 'Lock' THEN 'Lock'
        WHEN wait_event_type LIKE 'IO%' THEN 'IO'
        WHEN wait_event_type IS NULL AND state = 'active' THEN 'CPU'
        ELSE 'Other'
    END AS wait_category,
    SUM(sessions) as total_wait_time_approx
FROM 
    monitor.pg_wait_event_summary_ts
WHERE 
    capture_timestamp > NOW() - INTERVAL '1 hour'
GROUP BY 
    server_id, wait_category;

-- --------------------------------------------------------------------------
-- 3. Top Wait Events Summary
-- Supports the "Top Wait Events" Bar Chart
-- --------------------------------------------------------------------------
CREATE OR REPLACE VIEW monitor.pg_top_wait_events AS
SELECT 
    server_id,
    wait_event,
    SUM(sessions) as wait_occurrences
FROM 
    monitor.pg_wait_event_summary_ts
WHERE 
    capture_timestamp > NOW() - INTERVAL '1 hour'
    AND wait_event IS NOT NULL
GROUP BY 
    server_id, wait_event
ORDER BY 
    wait_occurrences DESC
LIMIT 10;

-- --------------------------------------------------------------------------
-- 4. Session State Trends
-- Supports the "Session States" Stacked Area Chart
-- --------------------------------------------------------------------------
CREATE OR REPLACE VIEW monitor.pg_session_states_trend AS
SELECT 
    time_bucket('1 minute', capture_timestamp) AS bucket,
    server_id,
    COUNT(*) FILTER (WHERE state = 'active') AS active_count,
    COUNT(*) FILTER (WHERE state = 'idle') AS idle_count,
    COUNT(*) FILTER (WHERE state = 'idle in transaction') AS idle_in_txn_count
FROM 
    monitor.pg_session_activity_ts
WHERE 
    capture_timestamp > NOW() - INTERVAL '24 hours'
GROUP BY 
    bucket, server_id
ORDER BY 
    bucket DESC;

-- Grant access
GRANT SELECT ON monitor.pg_db_load_summary TO sql_optima_app;
GRANT SELECT ON monitor.pg_wait_categories_summary TO sql_optima_app;
GRANT SELECT ON monitor.pg_top_wait_events TO sql_optima_app;
GRANT SELECT ON monitor.pg_session_states_trend TO sql_optima_app;

-- ============================================================================
-- SQL Optima: Postgres Backup & DR Implementation (Merged from 07_pg_backup_dr.sql)
-- ============================================================================
-- Purpose: Implements the redesigned Backup & DR dashboard requirements.
-- ============================================================================

-- 1. Schemas
CREATE SCHEMA IF NOT EXISTS monitor;
CREATE SCHEMA IF NOT EXISTS snapshot;

-- 2. Time-series Tables
-- Core snapshot table
CREATE TABLE IF NOT EXISTS snapshot.pg_backup_dr_timeseries (
    capture_timestamp              timestamptz NOT NULL,
    server_id               UUID NOT NULL,
    -- WAL
    wal_bytes_total           bigint,
    wal_records_total         bigint,
    wal_fpi_total             bigint,
    -- Archiver
    archived_count            bigint,
    archive_failed_count      bigint,
    last_archived_time        timestamptz,
    last_failed_time          timestamptz,
    -- Checkpoints
    checkpoints_timed         bigint,
    checkpoints_req           bigint,
    checkpoint_write_time_ms  double precision,
    checkpoint_sync_time_ms   double precision,
    -- Instance role
    is_in_recovery            boolean
);

-- Convert to hypertable
SELECT create_hypertable('snapshot.pg_backup_dr_timeseries', 'capture_timestamp',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE,
    migrate_data => FALSE);

CREATE INDEX IF NOT EXISTS idx_pg_backup_dr_ts_server_time
    ON snapshot.pg_backup_dr_timeseries (server_id, capture_timestamp DESC);

ALTER TABLE snapshot.pg_backup_dr_timeseries SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('snapshot.pg_backup_dr_timeseries', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('snapshot.pg_backup_dr_timeseries', INTERVAL '90 days', if_not_exists => TRUE);

-- Replication node time-series
CREATE TABLE IF NOT EXISTS snapshot.pg_replication_timeseries (
    capture_timestamp      timestamptz NOT NULL,
    server_id       UUID NOT NULL,
    application_name  text,
    client_addr       inet,
    state             text,
    sync_state        text,
    write_lag         interval,
    flush_lag         interval,
    replay_lag        interval,
    slot_name         text,
    retained_bytes    bigint
);

-- Convert to hypertable
SELECT create_hypertable('snapshot.pg_replication_timeseries', 'capture_timestamp',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE,
    migrate_data => FALSE);

CREATE INDEX IF NOT EXISTS idx_pg_repl_ts_server_time
    ON snapshot.pg_replication_timeseries (server_id, capture_timestamp DESC);

SELECT add_retention_policy('snapshot.pg_replication_timeseries', INTERVAL '90 days', if_not_exists => TRUE);

-- Archiver error audit (Additional table from requirement)
CREATE TABLE IF NOT EXISTS snapshot.pg_archive_error_log (
    capture_timestamp      timestamptz NOT NULL,
    server_id       UUID NOT NULL,
    failed_count      bigint,
    last_failed_time  timestamptz
);

-- Convert to hypertable
SELECT create_hypertable('snapshot.pg_archive_error_log', 'capture_timestamp',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE,
    migrate_data => FALSE);

CREATE INDEX IF NOT EXISTS idx_pg_archive_err_server_time
    ON snapshot.pg_archive_error_log (server_id, capture_timestamp DESC);

SELECT add_retention_policy('snapshot.pg_archive_error_log', INTERVAL '90 days', if_not_exists => TRUE);

-- 3. Indexes (time-only; server+time indexes above)
CREATE INDEX IF NOT EXISTS idx_pg_backup_dr_ts_coll_at ON snapshot.pg_backup_dr_timeseries (capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pg_repl_ts_coll_at ON snapshot.pg_replication_timeseries (capture_timestamp DESC);

-- 4. Retention (30 days)
CREATE OR REPLACE FUNCTION monitor.pg_retention_backup_dr()
RETURNS void LANGUAGE sql AS $$
    DELETE FROM snapshot.pg_backup_dr_timeseries
    WHERE capture_timestamp < now() - interval '30 days';

    DELETE FROM snapshot.pg_replication_timeseries
    WHERE capture_timestamp < now() - interval '30 days';
    
    DELETE FROM snapshot.pg_archive_error_log
    WHERE capture_timestamp < now() - interval '30 days';
$$;

-- 5. Collector Function
CREATE OR REPLACE FUNCTION monitor.pg_collect_backup_dr(p_server_id UUID)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE v_is_recovery bool;
BEGIN
  SELECT pg_is_in_recovery() INTO v_is_recovery;

  -- Collect WAL stats (PG 14+)
  IF EXISTS (SELECT 1 FROM pg_views WHERE viewname = 'pg_stat_wal') THEN
      INSERT INTO snapshot.pg_backup_dr_timeseries (
        capture_timestamp,
        server_id,
        wal_bytes_total,
        wal_records_total,
        wal_fpi_total,
        archived_count,
        archive_failed_count,
        last_archived_time,
        last_failed_time,
        checkpoints_timed,
        checkpoints_req,
        checkpoint_write_time_ms,
        checkpoint_sync_time_ms,
        is_in_recovery
      )
      SELECT
        now(),
        p_server_id,
        wal_bytes,
        wal_records,
        wal_fpi,
        a.archived_count,
        a.failed_count,
        a.last_archived_time,
        a.last_failed_time,
        b.checkpoints_timed,
        b.checkpoints_req,
        b.checkpoint_write_time,
        b.checkpoint_sync_time,
        v_is_recovery
      FROM pg_stat_wal w
      CROSS JOIN pg_stat_archiver a
      CROSS JOIN pg_stat_bgwriter b;
  ELSE
      -- Fallback for older versions (missing wal stats)
      INSERT INTO snapshot.pg_backup_dr_timeseries (
        capture_timestamp,
        server_id,
        archived_count,
        archive_failed_count,
        last_archived_time,
        last_failed_time,
        checkpoints_timed,
        checkpoints_req,
        checkpoint_write_time_ms,
        checkpoint_sync_time_ms,
        is_in_recovery
      )
      SELECT
        now(),
        p_server_id,
        a.archived_count,
        a.failed_count,
        a.last_archived_time,
        a.last_failed_time,
        b.checkpoints_timed,
        b.checkpoints_req,
        b.checkpoint_write_time,
        b.checkpoint_sync_time,
        v_is_recovery
      FROM pg_stat_archiver a
      CROSS JOIN pg_stat_bgwriter b;
  END IF;

  -- replication snapshot
  INSERT INTO snapshot.pg_replication_timeseries (
    capture_timestamp,
    server_id,
    application_name,
    client_addr,
    state,
    sync_state,
    write_lag,
    flush_lag,
    replay_lag,
    slot_name,
    retained_bytes
  )
  SELECT
    now(),
    p_server_id,
    application_name,
    client_addr,
    state,
    sync_state,
    write_lag,
    flush_lag,
    replay_lag,
    s.slot_name,
    pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)
  FROM pg_stat_replication r
  LEFT JOIN pg_replication_slots s
    ON r.pid = s.active_pid;

  -- Audit archiver failures
  INSERT INTO snapshot.pg_archive_error_log (capture_timestamp, server_id, failed_count, last_failed_time)
  SELECT now(), p_server_id, failed_count, last_failed_time
  FROM pg_stat_archiver
  WHERE failed_count > 0;
END;
$$;


-- ============================================================================
-- MIGRATION: Add new columns to ruleengine.rules
-- ============================================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'ruleengine' AND table_name = 'rules' AND column_name = 'applicability_sql') THEN
        ALTER TABLE ruleengine.rules ADD COLUMN applicability_sql TEXT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'ruleengine' AND table_name = 'rules' AND column_name = 'context_tags') THEN
        ALTER TABLE ruleengine.rules ADD COLUMN context_tags JSONB;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'ruleengine' AND table_name = 'rules' AND column_name = 'confidence') THEN
        ALTER TABLE ruleengine.rules ADD COLUMN confidence VARCHAR(20);
    END IF;
END $$;

-- ============================================================================
-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Optimized functional indexes for case-insensitive instance lookups.
--          Replaces standard B-tree indexes with UPPER() expression indexes
--          to match the application's query patterns.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

-- 1. PostgreSQL Memory Intelligence (monitor schema)
DROP INDEX IF EXISTS monitor.idx_host_mem_instance_ts;
CREATE INDEX idx_host_mem_instance_ts ON monitor.host_memory_samples (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS monitor.idx_pg_mem_instance_ts;
CREATE INDEX idx_pg_mem_instance_ts ON monitor.pg_memory_samples (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS monitor.idx_pg_comp_instance_ts;
CREATE INDEX idx_pg_comp_instance_ts ON monitor.pg_memory_components (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS monitor.idx_pg_der_instance_ts;
CREATE UNIQUE INDEX idx_pg_der_instance_ts ON monitor.pg_memory_derived (server_id, capture_timestamp DESC);

-- 2. PostgreSQL Core Stats (server_id UUID — keep as-is)
DROP INDEX IF EXISTS idx_postgres_tp_server_db;
CREATE INDEX idx_postgres_tp_server_db ON postgres_throughput_metrics (server_id, database_name, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_conn_server;
CREATE INDEX idx_postgres_conn_server ON postgres_connection_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_repl_server;
CREATE INDEX idx_postgres_repl_server ON postgres_replication_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_sys_server;
CREATE INDEX idx_postgres_sys_server ON postgres_system_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_qrystat_server;
CREATE INDEX idx_postgres_qrystat_server ON postgres_query_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pgss_qdim_db_user;
CREATE INDEX idx_pgss_qdim_db_user ON pgss_query_dim (server_id, db_name, username);

DROP INDEX IF EXISTS idx_pgss_delta_1m_server;
CREATE INDEX idx_pgss_delta_1m_server ON pgss_delta_1m (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_bgw_server_time;
CREATE INDEX idx_postgres_bgw_server_time ON postgres_bgwriter_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_arch_server_time;
CREATE INDEX idx_postgres_arch_server_time ON postgres_archiver_stats (server_id, capture_timestamp DESC);

-- 3. PostgreSQL Control Center tier (server_id TEXT)
DROP INDEX IF EXISTS idx_pg_cc_server_time;
CREATE INDEX idx_pg_cc_server_time ON postgres_control_center_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_repl_lag_detail;
CREATE INDEX idx_pg_repl_lag_detail ON postgres_replication_lag_detail (server_id, replica_name, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_repl_slot_server_time;
CREATE INDEX idx_pg_repl_slot_server_time ON postgres_replication_slot_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_disk_server_time;
CREATE INDEX idx_pg_disk_server_time ON postgres_disk_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_backup_runs_server_time;
CREATE INDEX idx_pg_backup_runs_server_time ON postgres_backup_runs (server_id, capture_timestamp DESC);

-- Per-instance PostgreSQL DR policy (current-state config; one row per server — not a hypertable).
CREATE TABLE IF NOT EXISTS optima_server_dr_policy (
    server_id UUID PRIMARY KEY REFERENCES optima_servers(id) ON DELETE CASCADE,
    rpo_backup_hours      INT NOT NULL DEFAULT 24,
    rpo_archive_minutes   INT NOT NULL DEFAULT 5,
    rpo_replay_seconds    INT NOT NULL DEFAULT 60,
    max_slot_retention_gb NUMERIC(10, 2) NOT NULL DEFAULT 10,
    rto_failover_minutes  INT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by TEXT
);

ALTER TABLE optima_server_dr_policy ADD COLUMN IF NOT EXISTS rpo_log_backup_minutes INT NOT NULL DEFAULT 15;
COMMENT ON COLUMN optima_server_dr_policy.rpo_log_backup_minutes IS
    'SQL Server: max minutes since last log backup for FULL/BULK_LOGGED databases (Backup & Recovery dashboard).';

COMMENT ON TABLE optima_server_dr_policy IS
    'Current RPO/RTO thresholds per monitored server for Backup & DR readiness and alerts. '
    'Dimension table (not time-series); use postgres_backup_runs and snapshot.pg_backup_dr_timeseries for history.';

CREATE INDEX IF NOT EXISTS idx_optima_server_dr_policy_updated
    ON optima_server_dr_policy (updated_at DESC);

DROP INDEX IF EXISTS idx_pg_log_events_server_time;
CREATE INDEX idx_pg_log_events_server_time ON postgres_log_events (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_vac_prog_server_time;
CREATE INDEX idx_pg_vac_prog_server_time ON postgres_vacuum_progress (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_tblmaint_server_time;
CREATE INDEX idx_pg_tblmaint_server_time ON postgres_table_maintenance_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_sess_state_server_time;
CREATE INDEX idx_pg_sess_state_server_time ON postgres_session_state_counts (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_pooler_server_time;
CREATE INDEX idx_pg_pooler_server_time ON postgres_pooler_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_deadlocks_server_time;
CREATE INDEX idx_pg_deadlocks_server_time ON postgres_deadlock_stats (server_id, capture_timestamp DESC);

-- 4. SQL Server Stats (server_id UUID — keep as-is)
DROP INDEX IF EXISTS idx_sqlserver_metrics_server;
CREATE INDEX idx_sqlserver_metrics_server ON sqlserver_metrics (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_cpu_server;
CREATE INDEX idx_sqlserver_cpu_server ON sqlserver_cpu_history (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_wait_server;
CREATE INDEX idx_sqlserver_wait_server ON sqlserver_wait_history (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_conn_server;
CREATE INDEX idx_sqlserver_conn_server ON sqlserver_connection_history (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_lock_server;
CREATE INDEX idx_sqlserver_lock_server ON sqlserver_lock_history (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_disk_server;
CREATE INDEX idx_sqlserver_disk_server ON sqlserver_disk_history (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_db_throughput_server_time;
CREATE INDEX idx_db_throughput_server_time ON sqlserver_database_throughput (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_ag_health_server_time;
CREATE INDEX idx_ag_health_server_time ON sqlserver_ag_health (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_job_server;
CREATE INDEX idx_sqlserver_job_server ON sqlserver_job_metrics (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_server_props_server_time;
CREATE INDEX idx_server_props_server_time ON sqlserver_server_properties (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_cpu_scheduler_server_time;
CREATE INDEX idx_cpu_scheduler_server_time ON sqlserver_cpu_scheduler_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_lrq_server_time;
CREATE INDEX idx_sqlserver_lrq_server_time ON sqlserver_long_running_queries (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_perfdebt_server_time;
CREATE INDEX idx_perfdebt_server_time ON sqlserver_performance_debt_findings (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_latch_waits_server_time;
CREATE INDEX idx_latch_waits_server_time ON sqlserver_latch_waits (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_memory_metrics_server_time;
CREATE INDEX idx_memory_metrics_server_time ON sqlserver_memory_metrics (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_waiting_tasks_server_time;
CREATE INDEX idx_waiting_tasks_server_time ON sqlserver_waiting_tasks (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_proc_stats_server_time;
CREATE INDEX idx_proc_stats_server_time ON sqlserver_procedure_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_spinlock_stats_server_time;
CREATE INDEX idx_spinlock_stats_server_time ON sqlserver_spinlock_stats (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_tempdb_files_server_time;
CREATE INDEX idx_tempdb_files_server_time ON sqlserver_tempdb_files (server_id, capture_timestamp DESC);

-- 5. Tables with server_id
-- sqlserver_query_stats_history (server_id UUID)
DROP INDEX IF EXISTS idx_sqlserver_query_stats_history_instance_ts;
CREATE INDEX idx_sqlserver_query_stats_history_instance_ts ON sqlserver_query_stats_history (server_id, capture_timestamp DESC);

-- sqlserver_session_snapshot (server_id UUID)
DROP INDEX IF EXISTS idx_sqlserver_session_snapshot_instance_time;
CREATE INDEX idx_sqlserver_session_snapshot_instance_time ON sqlserver_session_snapshot (server_id, capture_timestamp DESC);

-- sqlserver_query_metrics_v2 (server_id UUID)
DROP INDEX IF EXISTS idx_sqlserver_query_metrics_v2_instance_ts;
CREATE INDEX idx_sqlserver_query_metrics_v2_instance_ts ON sqlserver_query_metrics_v2 (server_id, capture_timestamp DESC);

-- pg_query_metrics_v2 (server_id UUID)
DROP INDEX IF EXISTS idx_pg_query_metrics_v2_instance_ts;
CREATE INDEX idx_pg_query_metrics_v2_instance_ts ON pg_query_metrics_v2 (server_id, capture_timestamp DESC);

-- pg_ts_metrics (server_id UUID)
DROP INDEX IF EXISTS idx_pg_ts_metrics_lookup;
CREATE INDEX idx_pg_ts_metrics_lookup ON pg_ts_metrics (server_id, metric, capture_timestamp DESC);

-- pg_query_bucket_metrics (server_id UUID)
DROP INDEX IF EXISTS idx_pg_query_bucket_instance_time;
CREATE INDEX idx_pg_query_bucket_instance_time ON pg_query_bucket_metrics (server_id, bucket_start DESC);

-- pg_ts_locks (server_id UUID)
DROP INDEX IF EXISTS pg_ts_locks_server_id_idx; -- some might have auto-gen names
DROP INDEX IF EXISTS idx_pg_ts_locks_server_time;
CREATE INDEX idx_pg_ts_locks_server_time ON pg_ts_locks (server_id, capture_timestamp DESC);

-- pg_ts_stat_statements_delta (server_id UUID)
DROP INDEX IF EXISTS idx_pg_ts_stat_server_query;
CREATE INDEX idx_pg_ts_stat_server_query ON pg_ts_stat_statements_delta (server_id, query_id, capture_timestamp DESC);

-- pg_ts_instance_snapshot (server_id UUID)
DROP INDEX IF EXISTS idx_pg_ts_snap_server_time;
CREATE INDEX idx_pg_ts_snap_server_time ON pg_ts_instance_snapshot (server_id, capture_timestamp DESC);

-- pg_query_metrics (server_id UUID)
DROP INDEX IF EXISTS idx_pg_query_metrics_instance_time;
CREATE INDEX idx_pg_query_metrics_instance_time ON pg_query_metrics (server_id, capture_timestamp DESC);

-- sqlserver_blocking_snapshots (server_id UUID)
DROP INDEX IF EXISTS idx_sqlserver_blocking_snaps_sqlhash;
CREATE INDEX idx_sqlserver_blocking_snaps_sqlhash ON sqlserver_blocking_snapshots (server_id, sql_hash, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_blocking_snaps_sid;
CREATE INDEX idx_sqlserver_blocking_snaps_sid ON sqlserver_blocking_snapshots (server_id, session_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_blocking_snaps_login;
CREATE INDEX idx_sqlserver_blocking_snaps_login ON sqlserver_blocking_snapshots (server_id, login_name, capture_timestamp DESC) WHERE blocking_session_id = 0;

-- sqlserver_blocking_locks (server_id UUID)
DROP INDEX IF EXISTS idx_sqlserver_blocking_locks_instance_ts;
CREATE INDEX idx_sqlserver_blocking_locks_instance_ts ON sqlserver_blocking_locks (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_qr_instance;
CREATE INDEX idx_sqlserver_qr_instance ON sqlserver_query_regressions (server_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_pi_instance;
CREATE INDEX idx_sqlserver_pi_instance ON sqlserver_plan_instability (server_id, capture_timestamp DESC);
-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Enhancements for SQL Server AG Health monitoring.
--          Adds operational and connected state tracking for better diagnostics.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

DO $$
BEGIN
    -- Add operational_state to sqlserver_ag_health
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_ag_health' AND column_name='operational_state') THEN
        ALTER TABLE sqlserver_ag_health ADD COLUMN operational_state TEXT;
    END IF;

    -- Add connected_state to sqlserver_ag_health
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_ag_health' AND column_name='connected_state') THEN
        ALTER TABLE sqlserver_ag_health ADD COLUMN connected_state TEXT;
    END IF;
END $$;
-- ============================================================================
-- SQL Optima: DMV Optimization Staging Schema
-- Purpose: Unified staging tables for high-frequency pulses.
-- ============================================================================

CREATE SCHEMA IF NOT EXISTS staging;

-- ----------------------------------------------------------------------------
-- SQL SERVER STAGING
-- ----------------------------------------------------------------------------

-- T1: Unified Sessions & Requests
CREATE TABLE IF NOT EXISTS staging.sqlserver_session_request_raw (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    session_id INT,
    login_name TEXT,
    host_name TEXT,
    program_name TEXT,
    client_interface_name TEXT,
    session_status TEXT,
    database_id INT,
    db_name TEXT,
    open_transaction_count INT,
    request_status TEXT,
    command TEXT,
    start_time TIMESTAMPTZ,
    wait_type TEXT,
    wait_time INT,
    last_wait_type TEXT,
    blocking_session_id INT,
    cpu_time_ms INT,
    elapsed_time_ms INT,
    logical_reads BIGINT,
    reads BIGINT,
    writes BIGINT,
    row_count BIGINT,
    granted_query_memory BIGINT,
    scheduler_id INT,
    dop INT,
    parallel_worker_count INT,
    percent_complete REAL,
    estimated_completion_time BIGINT,
    transaction_isolation_level INT,
    sql_handle TEXT,
    plan_handle TEXT,
    query_hash TEXT,
    query_plan_hash TEXT,
    statement_start_offset INT,
    statement_end_offset INT
);
SELECT create_hypertable('staging.sqlserver_session_request_raw', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_stg_ss_sess_lookup ON staging.sqlserver_session_request_raw (server_id, capture_timestamp DESC);
ALTER TABLE staging.sqlserver_session_request_raw SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('staging.sqlserver_session_request_raw', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('staging.sqlserver_session_request_raw', INTERVAL '7 days', if_not_exists => TRUE);

-- T2: Unified Performance Counters & Memory
CREATE TABLE IF NOT EXISTS staging.sqlserver_perf_system_raw (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    category TEXT,
    metric_name TEXT,
    metric_value BIGINT,
    unit TEXT,
    instance_name TEXT
);
SELECT create_hypertable('staging.sqlserver_perf_system_raw', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_stg_ss_perf_lookup ON staging.sqlserver_perf_system_raw (server_id, category, capture_timestamp DESC);
ALTER TABLE staging.sqlserver_perf_system_raw SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('staging.sqlserver_perf_system_raw', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('staging.sqlserver_perf_system_raw', INTERVAL '7 days', if_not_exists => TRUE);

-- T3: Unified I/O Stats
CREATE TABLE IF NOT EXISTS staging.sqlserver_io_raw (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    database_id INT,
    db_name TEXT,
    file_id INT,
    type_desc TEXT,
    physical_name TEXT,
    num_of_reads BIGINT,
    num_of_writes BIGINT,
    num_of_bytes_read BIGINT,
    num_of_bytes_written BIGINT,
    io_stall_read_ms BIGINT,
    io_stall_write_ms BIGINT,
    io_stall BIGINT,
    size_on_disk_bytes BIGINT
);
SELECT create_hypertable('staging.sqlserver_io_raw', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_stg_ss_io_lookup ON staging.sqlserver_io_raw (server_id, database_id, capture_timestamp DESC);
-- Dedup guard: prevents duplicate (server, file, timestamp) rows from concurrent writers.
CREATE UNIQUE INDEX IF NOT EXISTS idx_stg_ss_io_dedup
    ON staging.sqlserver_io_raw (server_id, database_id, file_id, capture_timestamp);
ALTER TABLE staging.sqlserver_io_raw SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('staging.sqlserver_io_raw', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('staging.sqlserver_io_raw', INTERVAL '7 days', if_not_exists => TRUE);

-- ----------------------------------------------------------------------------
-- POSTGRES STAGING
-- ----------------------------------------------------------------------------

-- T1: Unified Activity & Locks
CREATE TABLE IF NOT EXISTS staging.postgres_activity_locks_raw (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_id UUID NOT NULL,
    pid INT,
    datname TEXT,
    usename TEXT,
    application_name TEXT,
    client_addr INET,
    backend_type TEXT,
    state TEXT,
    wait_event_type TEXT,
    wait_event TEXT,
    query_start TIMESTAMPTZ,
    xact_start TIMESTAMPTZ,
    state_change TIMESTAMPTZ,
    query_duration INTERVAL,
    transaction_duration INTERVAL,
    blocking_pids INT[],
    locktype TEXT,
    mode TEXT,
    granted BOOLEAN,
    table_name TEXT,
    query TEXT
);
SELECT create_hypertable('staging.postgres_activity_locks_raw', 'capture_timestamp', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_stg_pg_act_lookup ON staging.postgres_activity_locks_raw (server_id, capture_timestamp DESC);
ALTER TABLE staging.postgres_activity_locks_raw SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('staging.postgres_activity_locks_raw', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('staging.postgres_activity_locks_raw', INTERVAL '7 days', if_not_exists => TRUE);

-- ============================================================================
-- SQL Optima: Optimized Dashboard Views
-- Purpose: Provides a clean abstraction layer for frontend dashboards
--          to query the latest pulse data from staging hypertables.
-- ============================================================================

CREATE SCHEMA IF NOT EXISTS dashboard;

-- ----------------------------------------------------------------------------
-- SQL SERVER VIEWS
-- ----------------------------------------------------------------------------

-- Latest Sessions & Requests
CREATE OR REPLACE VIEW dashboard.sqlserver_current_sessions AS
WITH LatestPulse AS (
    SELECT server_id, MAX(capture_timestamp) as last_ts
    FROM staging.sqlserver_session_request_raw
    GROUP BY server_id
)
SELECT s.*
FROM staging.sqlserver_session_request_raw s
JOIN LatestPulse lp ON s.server_id = lp.server_id 
                   AND s.capture_timestamp = lp.last_ts;

-- Blocking Tree Analysis
CREATE OR REPLACE VIEW dashboard.sqlserver_blocking_tree AS
SELECT * 
FROM dashboard.sqlserver_current_sessions
WHERE blocking_session_id <> 0 OR session_id IN (SELECT DISTINCT blocking_session_id FROM dashboard.sqlserver_current_sessions WHERE blocking_session_id <> 0);

-- Latest Perf KPIs
CREATE OR REPLACE VIEW dashboard.sqlserver_latest_kpis AS
WITH LatestPulse AS (
    SELECT server_id, MAX(capture_timestamp) as last_ts
    FROM staging.sqlserver_perf_system_raw
    GROUP BY server_id
)
SELECT s.server_id, s.category, s.metric_name, s.metric_value, s.unit, s.instance_name
FROM staging.sqlserver_perf_system_raw s
JOIN LatestPulse lp ON s.server_id = lp.server_id 
                   AND s.capture_timestamp = lp.last_ts;

-- ----------------------------------------------------------------------------
-- POSTGRES VIEWS
-- ----------------------------------------------------------------------------

-- Latest Activity & Locks
CREATE OR REPLACE VIEW dashboard.postgres_current_activity AS
WITH LatestPulse AS (
    SELECT server_id, MAX(capture_timestamp) as last_ts
    FROM staging.postgres_activity_locks_raw
    GROUP BY server_id
)
SELECT a.*
FROM staging.postgres_activity_locks_raw a
JOIN LatestPulse lp ON a.server_id = lp.server_id 
                   AND a.capture_timestamp = lp.last_ts;

-- Postgres Incident Feed (Active blocking or long-running)
CREATE OR REPLACE VIEW dashboard.postgres_active_incidents AS
SELECT *
FROM dashboard.postgres_current_activity
WHERE blocking_pids <> '{}' OR query_duration > '5 seconds'::interval;
-- ============================================================================
-- SQL Optima: Telemetry Health Plane
-- Purpose: Tracks collector status and data freshness SLA.
-- ============================================================================

CREATE SCHEMA IF NOT EXISTS monitor;

-- Tracks every collector execution
CREATE TABLE IF NOT EXISTS monitor.collector_runs (
    run_id BIGSERIAL,
    collector_name TEXT NOT NULL,
    server_id UUID NOT NULL,
    capture_timestamp TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    status TEXT NOT NULL, -- 'success', 'failed'
    rows_inserted INT DEFAULT 0,
    error_message TEXT,
    duration_ms INT
);
SELECT create_hypertable('monitor.collector_runs', 'capture_timestamp', if_not_exists => TRUE, migrate_data => TRUE);
CREATE INDEX IF NOT EXISTS idx_coll_runs_lookup ON monitor.collector_runs (server_id, collector_name, capture_timestamp DESC);
ALTER TABLE monitor.collector_runs SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'collector_name',
    timescaledb.compress_orderby = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.collector_runs', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.collector_runs', INTERVAL '30 days', if_not_exists => TRUE);

CREATE TABLE IF NOT EXISTS monitor.collector_run_log (
    run_id BIGSERIAL,
    capture_timestamp TIMESTAMPTZ ,
    server_id UUID NOT NULL,
    engine TEXT NOT NULL,
    status TEXT NOT NULL,
    duration_ms INT
);


-- Precomputed freshness per metric/server
CREATE TABLE IF NOT EXISTS monitor.metric_freshness (
    server_id UUID NOT NULL,
    metric_name TEXT NOT NULL,
    last_capture_timestamp TIMESTAMPTZ,
    freshness_seconds INT,
    status TEXT, -- 'healthy', 'stale', 'missing'
    PRIMARY KEY (server_id, metric_name)
);

-- Dashboard view for global pipeline health
CREATE OR REPLACE VIEW monitor.pipeline_health AS
SELECT
    server_id,
    COUNT(*) FILTER (WHERE status = 'healthy') as healthy_metrics,
    COUNT(*) FILTER (WHERE status <> 'healthy') as troubled_metrics,
    MAX(last_capture_timestamp) as last_ingestion
FROM monitor.metric_freshness
GROUP BY server_id;

-- ============================================================================
-- INTELLIGENCE REPORT — Persistence Schema (v2 redesign)
-- Stores computed analysis snapshots for trend charts and historical comparison.
-- Only IntelligenceReportService writes here — no collector writes.
-- Added: 2026-05-21 (Intelligence_report_redesign.md §7.0)
-- ============================================================================
CREATE SCHEMA IF NOT EXISTS intelreport;

CREATE TABLE IF NOT EXISTS intelreport.intel_snapshots (
    run_id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    server_id           UUID        NOT NULL,
    capture_timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    overall_risk        FLOAT8      NOT NULL DEFAULT 0,
    performance_risk    FLOAT8      NOT NULL DEFAULT 0,
    capacity_risk       FLOAT8      NOT NULL DEFAULT 0,
    availability_risk   FLOAT8      NOT NULL DEFAULT 0,
    replication_risk    FLOAT8      NOT NULL DEFAULT 0,
    maintenance_risk    FLOAT8      NOT NULL DEFAULT 0,
    query_risk          FLOAT8      NOT NULL DEFAULT 0,
    utilization_class   TEXT        NOT NULL DEFAULT 'unknown',
    -- Headline metrics captured at report time (avoids re-querying for trend charts)
    cpu_p50             FLOAT8      NOT NULL DEFAULT 0,
    cpu_p95             FLOAT8      NOT NULL DEFAULT 0,
    cpu_avg             FLOAT8      NOT NULL DEFAULT 0,
    mem_p95             FLOAT8      NOT NULL DEFAULT 0,
    ple_current         FLOAT8      NOT NULL DEFAULT 0,
    disk_used_pct       FLOAT8      NOT NULL DEFAULT 0,
    disk_days_remaining INT         NOT NULL DEFAULT 0,
    -- Report quality
    data_completeness   FLOAT8      NOT NULL DEFAULT 0,   -- 0.0–1.0
    data_coverage_days  INT         NOT NULL DEFAULT 0,
    rule_count_critical INT         NOT NULL DEFAULT 0,
    rule_count_high     INT         NOT NULL DEFAULT 0,
    -- Full report output
    report_json         JSONB,
    report_html         TEXT,
    PRIMARY KEY (server_id, capture_timestamp, run_id)
);

SELECT create_hypertable('intelreport.intel_snapshots', 'capture_timestamp',
    if_not_exists => TRUE);

ALTER TABLE intelreport.intel_snapshots SET (
    timescaledb.compress           = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('intelreport.intel_snapshots',
    INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('intelreport.intel_snapshots',
    INTERVAL '90 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_intel_snapshots_server_time
    ON intelreport.intel_snapshots (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_intel_snapshots_run_id
    ON intelreport.intel_snapshots (run_id);

-- ============================================================================
-- BACKWARD-COMPATIBLE COLUMN MIGRATIONS
-- These ADD COLUMN IF NOT EXISTS statements are safe to re-run on existing
-- deployments that were initialized before the schema was redesigned.
-- Each is idempotent and does NOT drop or truncate any table.
-- ============================================================================


-- ============================================================================
-- BACKWARD-COMPATIBLE COLUMN MIGRATIONS
-- These ADD COLUMN IF NOT EXISTS statements are safe to re-run on existing
-- deployments that were initialized before the schema was redesigned.
-- Each is idempotent and does NOT drop or truncate any table.
-- ============================================================================

-- sqlserver_file_io: add throughput columns (added alongside File IO delta collector)
ALTER TABLE IF EXISTS sqlserver_file_io ADD COLUMN IF NOT EXISTS read_bytes_per_sec  DOUBLE PRECISION;
ALTER TABLE IF EXISTS sqlserver_file_io ADD COLUMN IF NOT EXISTS write_bytes_per_sec DOUBLE PRECISION;
ALTER TABLE IF EXISTS sqlserver_file_io ADD COLUMN IF NOT EXISTS inserted_at         TIMESTAMPTZ;

-- sqlserver_perf_counters: add instance_name, cntr_type, value_per_sec (added for full DMV capture)
ALTER TABLE IF EXISTS sqlserver_perf_counters ADD COLUMN IF NOT EXISTS instance_name VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS sqlserver_perf_counters ADD COLUMN IF NOT EXISTS cntr_type     INT          NOT null DEFAULT 0;;
ALTER TABLE IF EXISTS sqlserver_perf_counters ADD COLUMN IF NOT EXISTS value_per_sec DOUBLE PRECISION;
ALTER TABLE IF EXISTS sqlserver_perf_counters ADD COLUMN IF NOT EXISTS inserted_at   TIMESTAMPTZ;

-- sqlserver_plan_cache: add aggregated cache health columns (added for plan cache health view)
ALTER TABLE IF EXISTS sqlserver_plan_cache ADD COLUMN IF NOT EXISTS total_cache_mb       NUMERIC(19,4);
ALTER TABLE IF EXISTS sqlserver_plan_cache ADD COLUMN IF NOT EXISTS single_use_cache_mb  NUMERIC(19,4);
ALTER TABLE IF EXISTS sqlserver_plan_cache ADD COLUMN IF NOT EXISTS single_use_cache_pct NUMERIC(19,4);
ALTER TABLE IF EXISTS sqlserver_plan_cache ADD COLUMN IF NOT EXISTS adhoc_cache_mb       NUMERIC(19,4);
ALTER TABLE IF EXISTS sqlserver_plan_cache ADD COLUMN IF NOT EXISTS prepared_cache_mb    NUMERIC(19,4);
ALTER TABLE IF EXISTS sqlserver_plan_cache ADD COLUMN IF NOT EXISTS proc_cache_mb        NUMERIC(19,4);
ALTER TABLE IF EXISTS sqlserver_plan_cache ADD COLUMN IF NOT EXISTS inserted_at          TIMESTAMPTZ;

-- sqlserver_health_kpis_v2: add new KPI columns (added for extended health worker)
ALTER TABLE IF EXISTS sqlserver_health_kpis_v2 ADD COLUMN IF NOT EXISTS logins_per_sec           DOUBLE PRECISION;
ALTER TABLE IF EXISTS sqlserver_health_kpis_v2 ADD COLUMN IF NOT EXISTS target_server_memory_mb  DOUBLE PRECISION;
ALTER TABLE IF EXISTS sqlserver_health_kpis_v2 ADD COLUMN IF NOT EXISTS total_server_memory_mb   DOUBLE PRECISION;
ALTER TABLE IF EXISTS sqlserver_health_kpis_v2 ADD COLUMN IF NOT EXISTS inserted_at              TIMESTAMPTZ;

-- Recreate perf_counters dedup index with instance_name (only if column was just added).
-- Uses DO block to avoid failure when duplicate rows prevent unique index creation.
DO $$
BEGIN
    -- Only attempt if idx_pc_dedup doesn't already include instance_name
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes
        WHERE tablename = 'sqlserver_perf_counters'
          AND indexname = 'idx_pc_dedup'
          AND indexdef LIKE '%instance_name%'
    ) THEN
        DROP INDEX IF EXISTS idx_pc_dedup;
        BEGIN
            CREATE UNIQUE INDEX idx_pc_dedup
                ON sqlserver_perf_counters (server_id, counter_name, instance_name, capture_timestamp);
        EXCEPTION WHEN others THEN
            RAISE NOTICE 'Could not create idx_pc_dedup: %. Deduplication index will be missing.', SQLERRM;
        END;
    END IF;
END $$;

-- ============================================================================
-- Cold storage archival control (merged from 07_cold_storage.sql)
-- ============================================================================
-- Purpose: Schema for tracking export progress (watermarks) and audit logging
--          for the cold storage archival pipeline.
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

CREATE SCHEMA IF NOT EXISTS coldstorage;

COMMENT ON SCHEMA coldstorage IS 'Control and metadata for the cold storage archival pipeline.';

-- Control table: one row per (table_name, server_id) — not a hypertable.
CREATE TABLE IF NOT EXISTS coldstorage.watermarks (
    table_name       TEXT        NOT NULL,
    server_id        UUID        NOT NULL,
    last_exported_at TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    export_rows_last INTEGER,
    export_bytes_last BIGINT,
    CONSTRAINT cold_export_watermarks_pkey PRIMARY KEY (table_name, server_id)
);

COMMENT ON TABLE coldstorage.watermarks IS
    'Tracks the high-water mark for cold storage exports per table and server. '
    'The exporter reads from last_exported_at and writes up to the cutoff '
    '(NOW() - lag_days). On failure the watermark is not advanced, ensuring '
    'at-least-once delivery to cold storage.';

CREATE INDEX IF NOT EXISTS idx_cold_watermarks_server
    ON coldstorage.watermarks (server_id);

CREATE INDEX IF NOT EXISTS idx_cold_watermarks_updated
    ON coldstorage.watermarks (updated_at DESC);

-- Append-only audit log of export cycles (hypertable on run_started).
CREATE TABLE IF NOT EXISTS coldstorage.runs (
    run_started        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cold_export_run_id BIGSERIAL NOT NULL,
    run_finished       TIMESTAMPTZ,
    status             TEXT        NOT NULL DEFAULT 'running',
    tables_ok          INTEGER,
    tables_failed      INTEGER,
    total_rows         BIGINT,
    total_bytes        BIGINT,
    error_detail       TEXT,
    PRIMARY KEY (run_started, cold_export_run_id)
);

SELECT create_hypertable('coldstorage.runs', 'run_started',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE,
    migrate_data => FALSE);

CREATE INDEX IF NOT EXISTS idx_cold_export_runs_started
    ON coldstorage.runs (run_started DESC);

CREATE INDEX IF NOT EXISTS idx_cold_export_runs_status_time
    ON coldstorage.runs (status, run_started DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cold_export_runs_id
    ON coldstorage.runs (cold_export_run_id, run_started);

ALTER TABLE coldstorage.runs SET (
    timescaledb.compress = true,
    timescaledb.compress_orderby = 'run_started DESC'
);

SELECT add_compression_policy('coldstorage.runs', INTERVAL '30 days', if_not_exists => TRUE);
SELECT add_retention_policy('coldstorage.runs', INTERVAL '180 days', if_not_exists => TRUE);

COMMENT ON TABLE coldstorage.runs IS
    'Audit log of each cold storage export cycle (hypertable; 180-day retention).';

CREATE OR REPLACE VIEW coldstorage.status_view AS
SELECT
    cew.table_name,
    cew.server_id,
    os.name AS server_name,
    cew.last_exported_at,
    NOW() - cew.last_exported_at AS age,
    cew.updated_at               AS watermark_updated_at
FROM coldstorage.watermarks cew
LEFT JOIN optima_servers os ON os.id = cew.server_id
ORDER BY age DESC NULLS FIRST;



