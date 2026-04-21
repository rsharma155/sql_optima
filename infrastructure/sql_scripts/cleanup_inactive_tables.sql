-- ============================================================================
-- SQL Optima Cleanup: Drop Inactive/Legacy Tables
-- Purpose: Remove tables identified as inactive or superseded by newer logic.
-- Note: This script is for REVIEW ONLY and should be executed manually.
-- ============================================================================

-- 1. PostgreSQL Inactive Tables (Superseded by 'monitor.*' schema or newer implementations)
DROP TABLE IF EXISTS postgres_database_stats CASCADE;    -- No active Go collector
DROP TABLE IF EXISTS postgres_session_stats CASCADE;     -- Superseded by monitor.pg_session_snapshot
DROP TABLE IF EXISTS postgres_lock_stats CASCADE;        -- Superseded by monitor.pg_lock_snapshot
DROP TABLE IF EXISTS postgres_table_stats CASCADE;       -- Superseded by monitor.table_usage_stats
DROP TABLE IF EXISTS postgres_index_stats CASCADE;       -- Superseded by monitor.index_usage_stats
DROP TABLE IF EXISTS postgres_config_settings CASCADE;   -- Superseded by postgres_settings_snapshot
DROP TABLE IF EXISTS postgres_long_running_queries CASCADE; -- Unused (Postgres)
DROP TABLE IF EXISTS system_metrics CASCADE;             -- Superseded by postgres_system_stats
DROP MATERIALIZED VIEW IF EXISTS system_metrics_1min CASCADE;
DROP MATERIALIZED VIEW IF EXISTS system_metrics_1h CASCADE;
DROP MATERIALIZED VIEW IF EXISTS system_metrics_1d CASCADE;

-- 2. SQL Server Inactive Tables (User identified as inactive / Missing implementation)
DROP TABLE IF EXISTS mssql_database_size CASCADE;        -- Superseded by snapshot.mssql_db_storage_history
DROP TABLE IF EXISTS mssql_server_config CASCADE;        -- No active Go implementation
DROP TABLE IF EXISTS mssql_database_config CASCADE;      -- No active Go implementation
DROP TABLE IF EXISTS mssql_qs_runtime CASCADE;           -- Superseded by query_store_stats
DROP TABLE IF EXISTS mssql_collection_log CASCADE;       -- No active Go implementation
DROP TABLE IF EXISTS mssql_tempdb_stats CASCADE;         -- No active background collector

-- 3. Application/Configuration Tables (Superseded by 'optima_*' prefix or unused)
DROP TABLE IF EXISTS alert_thresholds CASCADE;           -- Superseded by optima_alert_rules
DROP TABLE IF EXISTS alert_history CASCADE;              -- Superseded by optima_alert_history
DROP TABLE IF EXISTS notification_channels CASCADE;      -- Missing implementation
DROP TABLE IF EXISTS alert_subscriptions CASCADE;        -- Missing implementation
DROP TABLE IF EXISTS dashboard_widgets CASCADE;          -- Missing implementation
DROP TABLE IF EXISTS user_dashboards CASCADE;            -- Missing implementation
DROP TABLE IF EXISTS monitored_servers CASCADE;          -- Superseded by optima_servers
DROP TABLE IF EXISTS dashboard_exports CASCADE;          -- Missing implementation
DROP TABLE IF EXISTS metric_collection_settings CASCADE; -- Missing implementation

-- ============================================================================
-- END OF CLEANUP SCRIPT
-- ============================================================================
