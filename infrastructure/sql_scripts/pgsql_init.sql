-- ============================================================================
-- PostgreSQL Monitoring User Initialization Script
-- ============================================================================
-- Purpose: Creates a dedicated monitoring user with minimal required permissions
--          for the SQL Optima monitoring system.
--
-- Usage:   Execute this script as a superuser (e.g., postgres)
--          psql -U postgres -d postgres -f pgsql_init.sql
--
-- Note:    Adjust the password according to your security policy.
--          Grant usage on specific schemas as needed for your databases.
-- ============================================================================

-- Create monitoring role if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_roles WHERE rolname = 'dbmonitor_user'
    ) THEN
        CREATE ROLE dbmonitor_user WITH
            LOGIN
            PASSWORD = 'MonitorPass123!'
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            NOREPLICATION
            CONNECTION LIMIT 100;
        
        RAISE NOTICE 'Role [dbmonitor_user] created successfully.';
    ELSE
        RAISE NOTICE 'Role [dbmonitor_user] already exists.';
    END IF;
END
$$;

-- Grant required PostgreSQL system catalog permissions
GRANT pg_read_all_settings TO dbmonitor_user;
GRANT pg_read_all_stats TO dbmonitor_user;
GRANT pg_stat_scan_tables TO dbmonitor_user;
GRANT pg_monitor TO dbmonitor_user;

-- Grant execution on PostgreSQL monitoring functions
GRANT EXECUTE ON FUNCTION pg_stat_activity TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_bgwriter TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_database TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_user_indexes TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_activity TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_bgwriter TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_show_all_settings TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_lock_status TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_locks TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_replication TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_replication_slots TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_control_checkpoint TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_control_system TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_control_database TO dbmonitor_user;

-- Grant access to system catalogs for monitoring
GRANT SELECT ON pg_catalog.pg_stat_activity TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_stat_bgwriter TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_stat_database TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_stat_user_tables TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_stat_user_indexes TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_stat_sys_tables TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_statio_user_tables TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_statio_sys_tables TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_locks TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_stat_replication TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_replication_origin_status TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_replication_slots TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_settings TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_roles TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_database TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_namespace TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_class TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_attribute TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_proc TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_type TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_index TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_inherits TO dbmonitor_user;
GRANT SELECT ON pg_catalog.pg_tablespace TO dbmonitor_user;

-- Grant access to information schema
GRANT SELECT ON information_schema.tables TO dbmonitor_user;
GRANT SELECT ON information_schema.columns TO dbmonitor_user;
GRANT SELECT ON information_schema.views TO dbmonitor_user;
GRANT SELECT ON information_schema.routines TO dbmonitor_user;
GRANT SELECT ON information_schema.table_privileges TO dbmonitor_user;
GRANT SELECT ON information_schema.column_privileges TO dbmonitor_user;

-- Function for getting connection stats
GRANT EXECUTE ON FUNCTION pg_stat_get_db_connections(oid) TO dbmonitor_user;

-- Functions for index stats
GRANT EXECUTE ON FUNCTION pg_stat_get_numscans(oid) TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_tup_read(oid) TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_tup_fetch(oid) TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_tup_inserted(oid) TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_tup_updated(oid) TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_tup_deleted(oid) TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_live_tup(oid) TO dbmonitor_user;
GRANT EXECUTE ON FUNCTION pg_stat_get_dead_tup(oid) TO dbmonitor_user;

-- For pg_stat_statements extension (if installed)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements') THEN
        GRANT EXECUTE ON FUNCTION pg_stat_statements_reset() TO dbmonitor_user;
        GRANT SELECT ON pg_stat_statements TO dbmonitor_user;
        RAISE NOTICE 'Granted permissions for pg_stat_statements extension.';
    ELSE
        RAISE NOTICE 'pg_stat_statements extension not installed (optional).';
    END IF;
END
$$;

-- For pg_stat_kcache extension (if installed)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_kcache') THEN
        GRANT SELECT ON pg_stat_kcache TO dbmonitor_user;
        RAISE NOTICE 'Granted permissions for pg_stat_kcache extension.';
    ELSE
        RAISE NOTICE 'pg_stat_kcache extension not installed (optional).';
    END IF;
END
$$;

-- For TimescaleDB (if installed) - check hypertable access
DO $$
DECLARE
    is_timescaledb boolean;
BEGIN
    SELECT EXISTS(
        SELECT 1 FROM pg_extension WHERE extname = 'timescaledb'
    ) INTO is_timescaledb;
    
    IF is_timescaledb THEN
        GRANT SELECT ON timescaledb_information.hypertables TO dbmonitor_user;
        GRANT SELECT ON timescaledb_information.chunks TO dbmonitor_user;
        GRANT SELECT ON timescaledb_information.dimensions TO dbmonitor_user;
        GRANT SELECT ON timescaledb_information.compression_settings TO dbmonitor_user;
        GRANT SELECT ON timescaledb_information.job_stats TO dbmonitor_user;
        RAISE NOTICE 'Granted permissions for TimescaleDB information views.';
    ELSE
        RAISE NOTICE 'TimescaleDB extension not installed (optional).';
    END IF;
END
$$;

-- ============================================================================
-- Per-Database Configuration
-- ============================================================================
-- Run the following for each database you want to monitor:
--
-- CREATE USER dbmonitor_user FOR LOGIN dbmonitor_user;  -- For Azure DB / RDS
-- GRANT CONNECT ON DATABASE your_database TO dbmonitor_user;
-- GRANT USAGE ON SCHEMA public TO dbmonitor_user;
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO dbmonitor_user;
-- GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO dbmonitor_user;
-- ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO dbmonitor_user;
--
-- For Cloud-native PostgreSQL (CNPG) clusters, additional monitoring views
-- are typically available. Grant access to them if needed.
-- ============================================================================

\echo ''
\echo '========================================'
\echo 'PostgreSQL monitoring user setup complete.'
\echo '========================================'
\echo 'Role: dbmonitor_user'
\echo 'Password: MonitorPass123! (change this!)'
\echo ''
\echo 'Permissions granted:'
\echo '  - pg_monitor, pg_read_all_settings'
\echo '  - pg_read_all_stats, pg_stat_scan_tables'
\echo '  - System catalogs and information_schema'
\echo '  - pg_stat_statements (if available)'
\echo '  - TimescaleDB views (if available)'
\echo '========================================'
