-- ============================================================================
-- PostgreSQL Monitoring User Initialization Script
-- ============================================================================
-- Purpose: Creates a dedicated monitoring user with minimal required permissions
--          for the SQL Optima monitoring system.
--
-- Usage:   Execute this script as a superuser (e.g., postgres)
--          psql -U postgres -d postgres -f pgsql_init.sql
--
-- Note:    No default password is set in-repo. After CREATE ROLE, run:
--            ALTER ROLE dbmonitor_user PASSWORD 'your-secret-from-vault';
--          Or use: psql -v dbpass="'$(openssl rand -base64 24)'" -f ... (wrap in your automation).
--          Grant usage on specific schemas as needed for your databases.
-- ============================================================================

-- ============================================================================
-- PostgreSQL Monitoring User Initialization Script (Final Resilient Version)
-- ============================================================================

DO $$
BEGIN
    -- 1. Create role if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dbmonitor_user') THEN
        CREATE ROLE dbmonitor_user WITH
            LOGIN
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            NOREPLICATION
            CONNECTION LIMIT 100
            PASSWORD 'ChangeMe_123!'; --Change the password after creation, or set via automation
        RAISE NOTICE 'Role [dbmonitor_user] created.';
    ELSE
        RAISE NOTICE 'Role [dbmonitor_user] already exists.';
    END IF;

    -- 2. Grant pg_monitor (Standard for PG 10+)
    -- Covers pg_read_all_settings and pg_read_all_stats
    GRANT pg_monitor TO dbmonitor_user;

    -- 3. Grant Connect to the current database
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO dbmonitor_user', current_database());
END
$$;

-- 4. Global System Catalog Access
GRANT SELECT ON ALL TABLES IN SCHEMA pg_catalog TO dbmonitor_user;
GRANT SELECT ON ALL TABLES IN SCHEMA information_schema TO dbmonitor_user;

-- 5. Extension-specific Logic with Dynamic Signature Resolution
DO $$
DECLARE
    func_record RECORD;
BEGIN
    -- Handle pg_stat_statements
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements') THEN
        GRANT SELECT ON pg_stat_statements TO dbmonitor_user;
        
        -- Dynamically find and grant EXECUTE on the reset function(s)
        -- This fixes the [42883] error by resolving the exact signature
        FOR func_record IN 
            SELECT n.nspname, p.proname, pg_get_function_identity_arguments(p.oid) as args
            FROM pg_proc p
            JOIN pg_namespace n ON n.oid = p.pronamespace
            WHERE p.proname = 'pg_stat_statements_reset'
        LOOP
            EXECUTE format('GRANT EXECUTE ON FUNCTION %I.%I(%s) TO dbmonitor_user', 
                           func_record.nspname, func_record.proname, func_record.args);
        END LOOP;
        RAISE NOTICE 'pg_stat_statements permissions updated.';
    END IF;

    -- Handle pg_stat_kcache
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_kcache') THEN
        GRANT SELECT ON pg_stat_kcache TO dbmonitor_user;
        RAISE NOTICE 'pg_stat_kcache permissions updated.';
    END IF;

    -- Handle TimescaleDB
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        IF EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'timescaledb_information') THEN
            GRANT USAGE ON SCHEMA timescaledb_information TO dbmonitor_user;
            GRANT SELECT ON ALL TABLES IN SCHEMA timescaledb_information TO dbmonitor_user;
            RAISE NOTICE 'TimescaleDB permissions updated.';
        END IF;
    END IF;
END
$$;

-- 6. Application Schema Access
GRANT USAGE ON SCHEMA public TO dbmonitor_user;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO dbmonitor_user;
-- Ensure future tables are also readable
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO dbmonitor_user;

DO $$
BEGIN
    RAISE NOTICE '========================================';
    RAISE NOTICE 'PostgreSQL monitoring user setup complete.';
    RAISE NOTICE 'Role: dbmonitor_user';
    RAISE NOTICE '========================================';
END
$$;