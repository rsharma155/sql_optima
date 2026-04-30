-- ============================================================================
-- SQL Optima Cleanup: Drop Obsolete/Unused Tables
-- Purpose: Remove tables with no references or dependencies in the codebase.
-- ============================================================================

-- 1. PostgreSQL Obsolete Tables
DROP TABLE IF EXISTS pg_ts_bloat CASCADE;
DROP TABLE IF EXISTS pg_ts_replication_lag CASCADE;
DROP TABLE IF EXISTS pg_ts_tps CASCADE;
DROP TABLE IF EXISTS pg_ts_wal_metrics CASCADE;

-- 2. SQL Server Obsolete Tables
DROP TABLE IF EXISTS sqlserver_collection_schedule CASCADE;
DROP TABLE IF EXISTS sqlserver_file_io_history CASCADE;
DROP TABLE IF EXISTS sqlserver_query_store_stats CASCADE;
DROP TABLE IF EXISTS query_store_stats CASCADE;
DROP TABLE IF EXISTS sqlserver_database_size CASCADE;
DROP TABLE IF EXISTS sqlserver_qs_runtime CASCADE;
DROP TABLE IF EXISTS sqlserver_tempdb_stats CASCADE;

-- 3. Rule Engine Obsolete Tables
DROP TABLE IF EXISTS ruleengine.signal_snapshots CASCADE;
DROP TABLE IF EXISTS ruleengine.signals CASCADE;

-- 4. Application/Configuration Obsolete Tables
DROP TABLE IF EXISTS alert_subscriptions CASCADE;
DROP TABLE IF EXISTS alert_thresholds CASCADE;
DROP TABLE IF EXISTS dashboard_exports CASCADE;
DROP TABLE IF EXISTS dashboard_widgets CASCADE;
DROP TABLE IF EXISTS metric_collection_settings CASCADE;
DROP TABLE IF EXISTS monitored_servers CASCADE;
DROP TABLE IF EXISTS notification_channels CASCADE;
DROP TABLE IF EXISTS user_dashboards CASCADE;

-- 5. Additional identified obsolete tables from cleanup_inactive_tables.sql for completeness
DROP TABLE IF EXISTS postgres_database_stats CASCADE;
DROP TABLE IF EXISTS postgres_session_stats CASCADE;
DROP TABLE IF EXISTS postgres_lock_stats CASCADE;
DROP TABLE IF EXISTS postgres_table_stats CASCADE;
DROP TABLE IF EXISTS postgres_index_stats CASCADE;
DROP TABLE IF EXISTS postgres_config_settings CASCADE;
DROP TABLE IF EXISTS postgres_long_running_queries CASCADE;
DROP TABLE IF EXISTS system_metrics CASCADE;
DROP MATERIALIZED VIEW IF EXISTS system_metrics_1min CASCADE;
DROP MATERIALIZED VIEW IF EXISTS system_metrics_1h CASCADE;
DROP MATERIALIZED VIEW IF EXISTS system_metrics_1d CASCADE;
DROP TABLE IF EXISTS sqlserver_server_config CASCADE;
DROP TABLE IF EXISTS sqlserver_database_config CASCADE;
DROP TABLE IF EXISTS sqlserver_collection_log CASCADE;
DROP TABLE IF EXISTS alert_history CASCADE;
