-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: OPT-IN reduction of Group A hot-tier retention from 90 → 60 days
--          after cold-storage exports have been validated for ≥2 weeks.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
--
-- DO NOT run automatically. Operators must:
--   1. Confirm COLD_STORAGE_ENABLED=true and watermarks are current
--      (GET /api/cold-storage/status — no server lagging export cutoff)
--   2. Spot-check Parquet in MinIO/S3 for the tables below
--   3. Keep a Timescale backup before applying
--
-- Apply (example):
--   psql "$DATABASE_URL" -f infrastructure/sql_scripts/migrations/011_cold_storage_reduce_hot_retention_60d.sql
--
-- Rollback to 90 days: change INTERVAL '60 days' → '90 days' and re-run.

SET search_path TO public, monitor, pg_catalog;

DO $$
DECLARE
  t text;
  tables text[] := ARRAY[
    'sqlserver_cpu_history',
    'sqlserver_memory_history',
    'sqlserver_wait_history',
    'sqlserver_metrics',
    'sqlserver_connection_history',
    'sqlserver_lock_history',
    'sqlserver_disk_history',
    'sqlserver_database_throughput',
    'sqlserver_memory_metrics',
    'sqlserver_buffer_pool_db',
    'sqlserver_cpu_scheduler_stats',
    'postgres_settings_snapshot'
  ];
BEGIN
  FOREACH t IN ARRAY tables LOOP
    IF to_regclass(t) IS NULL THEN
      RAISE NOTICE 'skip missing table %', t;
      CONTINUE;
    END IF;
    -- Drop existing retention policy if present, then add 60-day policy.
    BEGIN
      PERFORM remove_retention_policy(t, if_exists => true);
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE 'remove_retention_policy(%) skipped: %', t, SQLERRM;
    END;
    PERFORM add_retention_policy(t, INTERVAL '60 days', if_not_exists => true);
    RAISE NOTICE 'retention set to 60 days for %', t;
  END LOOP;
END $$;

-- monitor schema Group A tables (qualified)
DO $$
DECLARE
  t text;
  tables text[] := ARRAY[
    'monitor.pg_backup_archiver_ts',
    'monitor.pg_basebackup_history',
    'monitor.pg_roles_snapshot',
    'monitor.pg_failed_login_events'
  ];
BEGIN
  FOREACH t IN ARRAY tables LOOP
    IF to_regclass(t) IS NULL THEN
      RAISE NOTICE 'skip missing table %', t;
      CONTINUE;
    END IF;
    BEGIN
      PERFORM remove_retention_policy(t, if_exists => true);
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE 'remove_retention_policy(%) skipped: %', t, SQLERRM;
    END;
    PERFORM add_retention_policy(t, INTERVAL '60 days', if_not_exists => true);
    RAISE NOTICE 'retention set to 60 days for %', t;
  END LOOP;
END $$;
