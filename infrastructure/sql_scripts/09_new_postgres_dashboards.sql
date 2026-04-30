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
    ts timestamptz NOT NULL DEFAULT now(),
    instance_id text NOT NULL,
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
SELECT create_hypertable('monitor.pg_session_activity_ts', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_session_activity_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- 2. Wait event aggregation
CREATE TABLE IF NOT EXISTS monitor.pg_wait_event_summary_ts (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
    wait_event_type text,
    wait_event text,
    sessions int,
    state text
);
SELECT create_hypertable('monitor.pg_wait_event_summary_ts', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_wait_event_summary_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- 3. Database load (AAS approximation)
CREATE TABLE IF NOT EXISTS monitor.pg_db_load_ts (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
    active_sessions int,
    cpu_sessions int,
    waiting_sessions int,
    idle_in_txn int
);
SELECT create_hypertable('monitor.pg_db_load_ts', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_db_load_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- 4. Top queries by waits (pg_stat_statements)
CREATE TABLE IF NOT EXISTS monitor.pg_query_wait_profile_ts (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
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
SELECT create_hypertable('monitor.pg_query_wait_profile_ts', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_query_wait_profile_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- DASHBOARD 2: Backup & Disaster Recovery
-- --------------------------------------------------------------------------

-- 1. Backup history (pg_stat_archiver)
CREATE TABLE IF NOT EXISTS monitor.pg_backup_archiver_ts (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
    archived_count bigint,
    failed_count bigint,
    last_archived_time timestamptz,
    last_failed_time timestamptz
);
SELECT create_hypertable('monitor.pg_backup_archiver_ts', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_backup_archiver_ts', INTERVAL '90 days', if_not_exists => TRUE);

-- 2. WAL generation rate
CREATE TABLE IF NOT EXISTS monitor.pg_wal_rate_ts (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
    wal_bytes numeric
);
SELECT create_hypertable('monitor.pg_wal_rate_ts', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_wal_rate_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- 3. Base backup detection (from pg_stat_bgwriter/checkpoints)
CREATE TABLE IF NOT EXISTS monitor.pg_basebackup_history (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
    checkpoint_time timestamptz,
    checkpoint_write_time double precision
);
SELECT create_hypertable('monitor.pg_basebackup_history', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_basebackup_history', INTERVAL '90 days', if_not_exists => TRUE);

-- --------------------------------------------------------------------------
-- DASHBOARD 3: Security Monitoring
-- --------------------------------------------------------------------------

-- 1. Role & privilege snapshot
CREATE TABLE IF NOT EXISTS monitor.pg_roles_snapshot (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
    rolname text,
    rolsuper bool,
    rolcreatedb bool,
    rolcreaterole bool,
    rolreplication bool,
    rolcanlogin bool
);
SELECT create_hypertable('monitor.pg_roles_snapshot', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_roles_snapshot', INTERVAL '90 days', if_not_exists => TRUE);

-- 2. Failed logins
CREATE TABLE IF NOT EXISTS monitor.pg_failed_login_events (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
    username text,
    client_addr text,
    message text
);
SELECT create_hypertable('monitor.pg_failed_login_events', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_failed_login_events', INTERVAL '90 days', if_not_exists => TRUE);

-- 3. DDL audit snapshot
CREATE TABLE IF NOT EXISTS monitor.pg_ddl_activity_ts (
    ts timestamptz NOT NULL,
    instance_id text NOT NULL,
    schemaname text,
    relname text,
    n_tup_ins bigint,
    n_tup_upd bigint,
    n_tup_del bigint
);
SELECT create_hypertable('monitor.pg_ddl_activity_ts', 'ts', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.pg_ddl_activity_ts', INTERVAL '30 days', if_not_exists => TRUE);

-- Grant permissions
GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA monitor TO PUBLIC;
