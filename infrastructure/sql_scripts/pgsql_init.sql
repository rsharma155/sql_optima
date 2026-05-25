-- ============================================================================
-- PostgreSQL Monitoring User Initialization Script
-- ============================================================================
-- Purpose: Creates a dedicated monitoring user with minimal required permissions
--          for the SQL Optima monitoring system.
--
-- Usage:   Execute this script as a superuser (e.g., postgres)
--          psql -U postgres -d postgres -f pgsql_init.sql
--
-- Note:    Change the value for v_username and v_password before running the script
--
-- Dynamic / Production Version
-- Compatible: PostgreSQL 10+
-- ============================================================================

DO $MAIN$
DECLARE
    ---------------------------------------------------------------------------
    -- CONFIGURATION
    ---------------------------------------------------------------------------
    v_username              TEXT := 'dbmonitor_user';
    v_password              TEXT := 'ChangeThisStrongPassword!';
    v_connection_limit      INTEGER := 100;

    v_db                    RECORD;
    v_schema                RECORD;
    v_function              RECORD;

BEGIN

    ---------------------------------------------------------------------------
    -- CREATE ROLE
    ---------------------------------------------------------------------------
    IF NOT EXISTS (
        SELECT 1
        FROM pg_roles
        WHERE rolname = v_username
    )
    THEN

        EXECUTE format($SQL$
            CREATE ROLE %I
            WITH
                LOGIN
                NOSUPERUSER
                NOCREATEDB
                NOCREATEROLE
                NOINHERIT
                NOREPLICATION
                CONNECTION LIMIT %s
                PASSWORD %L
        $SQL$,
            v_username,
            v_connection_limit,
            v_password
        );

        RAISE NOTICE 'Role [%] created.', v_username;

    ELSE

        RAISE NOTICE 'Role [%] already exists.', v_username;

    END IF;

    ---------------------------------------------------------------------------
    -- CORE MONITORING ROLES
    ---------------------------------------------------------------------------
    EXECUTE format(
        'GRANT pg_monitor TO %I',
        v_username
    );

    ---------------------------------------------------------------------------
    -- PostgreSQL 14+
    ---------------------------------------------------------------------------
    IF EXISTS (
        SELECT 1
        FROM pg_roles
        WHERE rolname = 'pg_read_all_data'
    )
    THEN
        EXECUTE format(
            'GRANT pg_read_all_data TO %I',
            v_username
        );
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_roles
        WHERE rolname = 'pg_read_all_settings'
    )
    THEN
        EXECUTE format(
            'GRANT pg_read_all_settings TO %I',
            v_username
        );
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_roles
        WHERE rolname = 'pg_read_all_stats'
    )
    THEN
        EXECUTE format(
            'GRANT pg_read_all_stats TO %I',
            v_username
        );
    END IF;

    ---------------------------------------------------------------------------
    -- CONNECT TO ALL NON-TEMPLATE DATABASES
    ---------------------------------------------------------------------------
    FOR v_db IN
        SELECT datname
        FROM pg_database
        WHERE datistemplate = false
    LOOP

        EXECUTE format(
            'GRANT CONNECT ON DATABASE %I TO %I',
            v_db.datname,
            v_username
        );

    END LOOP;

    ---------------------------------------------------------------------------
    -- REPLICATION MONITORING
    ---------------------------------------------------------------------------
    BEGIN

        EXECUTE format(
            'GRANT SELECT ON pg_stat_replication TO %I',
            v_username
        );

    EXCEPTION
        WHEN undefined_table THEN
            NULL;
    END;

    BEGIN

        EXECUTE format(
            'GRANT SELECT ON pg_replication_slots TO %I',
            v_username
        );

    EXCEPTION
        WHEN undefined_table THEN
            NULL;
    END;

    ---------------------------------------------------------------------------
    -- WAL / ARCHIVE MONITORING
    ---------------------------------------------------------------------------
    BEGIN

        EXECUTE format(
            'GRANT SELECT ON pg_stat_wal TO %I',
            v_username
        );

    EXCEPTION
        WHEN undefined_table THEN
            NULL;
    END;

    ---------------------------------------------------------------------------
    -- SESSION MANAGEMENT
    ---------------------------------------------------------------------------
    BEGIN

        EXECUTE format(
            'GRANT EXECUTE ON FUNCTION pg_cancel_backend(integer) TO %I',
            v_username
        );

        EXECUTE format(
            'GRANT EXECUTE ON FUNCTION pg_terminate_backend(integer) TO %I',
            v_username
        );

    EXCEPTION
        WHEN undefined_function THEN
            NULL;
    END;

    ---------------------------------------------------------------------------
    -- EXTENSION: pg_stat_statements
    ---------------------------------------------------------------------------
    IF EXISTS (
        SELECT 1
        FROM pg_extension
        WHERE extname = 'pg_stat_statements'
    )
    THEN

        BEGIN

            EXECUTE format(
                'GRANT SELECT ON pg_stat_statements TO %I',
                v_username
            );

        EXCEPTION
            WHEN undefined_table THEN
                NULL;
        END;

        -----------------------------------------------------------------------
        -- Dynamic reset function signature handling
        -----------------------------------------------------------------------
        FOR v_function IN
            SELECT
                n.nspname,
                p.proname,
                pg_get_function_identity_arguments(p.oid) AS args
            FROM pg_proc p
            JOIN pg_namespace n
                ON n.oid = p.pronamespace
            WHERE p.proname = 'pg_stat_statements_reset'
        LOOP

            EXECUTE format(
                'GRANT EXECUTE ON FUNCTION %I.%I(%s) TO %I',
                v_function.nspname,
                v_function.proname,
                v_function.args,
                v_username
            );

        END LOOP;

        RAISE NOTICE 'pg_stat_statements permissions granted.';

    END IF;

    ---------------------------------------------------------------------------
    -- EXTENSION: pg_stat_kcache
    ---------------------------------------------------------------------------
    IF EXISTS (
        SELECT 1
        FROM pg_extension
        WHERE extname = 'pg_stat_kcache'
    )
    THEN

        BEGIN

            EXECUTE format(
                'GRANT SELECT ON pg_stat_kcache TO %I',
                v_username
            );

        EXCEPTION
            WHEN undefined_table THEN
                NULL;
        END;

    END IF;

    ---------------------------------------------------------------------------
    -- EXTENSION: TimescaleDB
    ---------------------------------------------------------------------------
    IF EXISTS (
        SELECT 1
        FROM pg_extension
        WHERE extname = 'timescaledb'
    )
    THEN

        IF EXISTS (
            SELECT 1
            FROM information_schema.schemata
            WHERE schema_name = 'timescaledb_information'
        )
        THEN

            EXECUTE format(
                'GRANT USAGE ON SCHEMA timescaledb_information TO %I',
                v_username
            );

            EXECUTE format(
                'GRANT SELECT ON ALL TABLES IN SCHEMA timescaledb_information TO %I',
                v_username
            );

        END IF;

    END IF;

    ---------------------------------------------------------------------------
    -- OPTIONAL: ACCESS TO USER SCHEMAS
    ---------------------------------------------------------------------------
    FOR v_schema IN
        SELECT schema_name
        FROM information_schema.schemata
        WHERE schema_name NOT IN (
            'pg_catalog',
            'information_schema'
        )
        AND schema_name NOT LIKE 'pg_toast%'
        AND schema_name NOT LIKE 'pg_temp%'
    LOOP

        BEGIN

            EXECUTE format(
                'GRANT USAGE ON SCHEMA %I TO %I',
                v_schema.schema_name,
                v_username
            );

            EXECUTE format(
                'GRANT SELECT ON ALL TABLES IN SCHEMA %I TO %I',
                v_schema.schema_name,
                v_username
            );

        EXCEPTION
            WHEN insufficient_privilege THEN
                RAISE NOTICE 'Skipping schema [%]', v_schema.schema_name;
        END;

    END LOOP;

    ---------------------------------------------------------------------------
    -- SUMMARY
    ---------------------------------------------------------------------------
    RAISE NOTICE '';
    RAISE NOTICE '========================================';
    RAISE NOTICE 'PostgreSQL monitoring user setup complete';
    RAISE NOTICE 'Role : %', v_username;
    RAISE NOTICE '========================================';

END
$MAIN$;