-- ============================================================================
-- SQL Optima: Seed Data
-- Default users, widgets, and collection schedules
-- Version: 1.0.0
-- ============================================================================

/*
-- ============================================================================
-- SEED DATA: Default Users
-- Password: admin123 (bcrypt hash pre-computed)
-- ============================================================================
INSERT INTO optima_users (username, password_hash, role)
VALUES (
    'admin',
    '$2a$10$XI9xAr9NzIbsTqZZaixXXubexWasheZi/cjQmSO4V0lwr4T4CzCAu',
    'admin'
) ON CONFLICT (username) DO UPDATE SET password_hash = EXCLUDED.password_hash;

INSERT INTO optima_users (username, password_hash, role)
VALUES (
    'viewer',
    '$2a$10$XI9xAr9NzIbsTqZZaixXXubexWasheZi/cjQmSO4V0lwr4T4CzCAu',
    'viewer'
) ON CONFLICT (username) DO UPDATE SET password_hash = EXCLUDED.password_hash;

*/

-- ============================================================================
-- SEED DATA: UI Widgets
-- ============================================================================

-- Widget 1: Active Sessions (grid/table)
INSERT INTO optima_ui_widgets (widget_id, dashboard_section, title, chart_type, current_sql, default_sql)
VALUES (
    'pg_active_sessions',
    'sessions_activity',
    'Active Sessions',
    'grid',
    $$SELECT pid, usename AS username, datname AS database, application_name, state, wait_event_type, wait_event, query_start, now() - query_start AS duration FROM pg_stat_activity WHERE state != 'idle' ORDER BY query_start ASC LIMIT 50$$,
    $$SELECT pid, usename AS username, datname AS database, application_name, state, wait_event_type, wait_event, query_start, now() - query_start AS duration FROM pg_stat_activity WHERE state != 'idle' ORDER BY query_start ASC LIMIT 50$$
) ON CONFLICT (widget_id) DO NOTHING;

-- Widget 2: CPU History (line chart)
INSERT INTO optima_ui_widgets (widget_id, dashboard_section, title, chart_type, current_sql, default_sql)
VALUES (
    'pg_cpu_history',
    'performance',
    'CPU History (Last 30 min)',
    'line',
    $$SELECT capture_timestamp AS time, cpu_usage AS value FROM postgres_system_stats WHERE server_instance_name = '{{server_name}}' ORDER BY capture_timestamp DESC LIMIT 30$$,
    $$SELECT capture_timestamp AS time, cpu_usage AS value FROM postgres_system_stats WHERE server_instance_name = '{{server_name}}' ORDER BY capture_timestamp DESC LIMIT 30$$
) ON CONFLICT (widget_id) DO NOTHING;

-- Widget 3: Disk Usage (gauge)
INSERT INTO optima_ui_widgets (widget_id, dashboard_section, title, chart_type, current_sql, default_sql)
VALUES (
    'pg_disk_usage',
    'storage',
    'Database Disk Usage',
    'gauge',
    $$SELECT pg_database.datname AS database, pg_database_size(pg_database.datname) / 1024.0 / 1024.0 AS size_mb FROM pg_database WHERE pg_database.datname NOT LIKE 'template%' ORDER BY size_mb DESC$$,
    $$SELECT pg_database.datname AS database, pg_database_size(pg_database.datname) / 1024.0 / 1024.0 AS size_mb FROM pg_database WHERE pg_database.datname NOT LIKE 'template%' ORDER BY size_mb DESC$$
) ON CONFLICT (widget_id) DO NOTHING;

-- Widget 4: Throughput TPS (line chart)
INSERT INTO optima_ui_widgets (widget_id, dashboard_section, title, chart_type, current_sql, default_sql)
VALUES (
    'pg_throughput_tps',
    'performance',
    'Throughput (TPS)',
    'line',
    $$SELECT capture_timestamp AS time, tps AS value FROM postgres_throughput_metrics WHERE server_instance_name = '{{server_name}}' AND database_name = '{{database}}' ORDER BY capture_timestamp DESC LIMIT 30$$,
    $$SELECT capture_timestamp AS time, tps AS value FROM postgres_throughput_metrics WHERE server_instance_name = '{{server_name}}' AND database_name = '{{database}}' ORDER BY capture_timestamp DESC LIMIT 30$$
) ON CONFLICT (widget_id) DO NOTHING;

-- Widget 5: Cache Hit Ratio (doughnut)
INSERT INTO optima_ui_widgets (widget_id, dashboard_section, title, chart_type, current_sql, default_sql)
VALUES (
    'pg_cache_hit',
    'performance',
    'Cache Hit Ratio',
    'doughnut',
    $$SELECT 'Cache Hit' AS label, blks_hit AS value FROM pg_stat_database WHERE datname = '{{database}}' UNION ALL SELECT 'Cache Miss', blks_read FROM pg_stat_database WHERE datname = '{{database}}'$$,
    $$SELECT 'Cache Hit' AS label, blks_hit AS value FROM pg_stat_database WHERE datname = '{{database}}' UNION ALL SELECT 'Cache Miss', blks_read FROM pg_stat_database WHERE datname = '{{database}}'$$
) ON CONFLICT (widget_id) DO NOTHING;

-- Widget 6: Connection Stats (bar chart)
INSERT INTO optima_ui_widgets (widget_id, dashboard_section, title, chart_type, current_sql, default_sql)
VALUES (
    'pg_connection_stats',
    'sessions_activity',
    'Connection Statistics',
    'bar',
    $$SELECT state AS label, COUNT(*) AS value FROM pg_stat_activity WHERE datname = '{{database}}' GROUP BY state ORDER BY value DESC$$,
    $$SELECT state AS label, COUNT(*) AS value FROM pg_stat_activity WHERE datname = '{{database}}' GROUP BY state ORDER BY value DESC$$
) ON CONFLICT (widget_id) DO NOTHING;

-- Widget 7: Replication Lag (line chart)
INSERT INTO optima_ui_widgets (widget_id, dashboard_section, title, chart_type, current_sql, default_sql)
VALUES (
    'pg_replication_lag',
    'replication',
    'Replication Lag (MB)',
    'line',
    $$SELECT capture_timestamp AS time, max_lag_mb AS value FROM postgres_replication_stats WHERE server_instance_name = '{{server_name}}' ORDER BY capture_timestamp DESC LIMIT 30$$,
    $$SELECT capture_timestamp AS time, max_lag_mb AS value FROM postgres_replication_stats WHERE server_instance_name = '{{server_name}}' ORDER BY capture_timestamp DESC LIMIT 30$$
) ON CONFLICT (widget_id) DO NOTHING;

-- Widget 8: BGWriter Stats (grid)
INSERT INTO optima_ui_widgets (widget_id, dashboard_section, title, chart_type, current_sql, default_sql)
VALUES (
    'pg_bgwriter_stats',
    'enterprise',
    'BGWriter Statistics',
    'grid',
    $$SELECT checkpoints_timed, checkpoints_req, buffers_checkpoint, buffers_clean, buffers_backend, maxwritten_clean FROM pg_stat_bgwriter$$,
    $$SELECT checkpoints_timed, checkpoints_req, buffers_checkpoint, buffers_clean, buffers_backend, maxwritten_clean FROM pg_stat_bgwriter$$
) ON CONFLICT (widget_id) DO NOTHING;

/*
-- ============================================================================
-- SEED DATA: Collection Schedule
-- ============================================================================
INSERT INTO sqlserver_collection_schedule (collector_name, enabled, frequency_minutes, max_duration_minutes, retention_days, description)
SELECT * FROM (VALUES
    ('wait_stats', TRUE, 1, 2, 30, 'Wait statistics - high frequency for trending'),
    ('query_stats', TRUE, 2, 5, 30, 'Plan cache queries - recent activity focused'),
    ('memory_stats', TRUE, 1, 2, 30, 'Memory pressure monitoring'),
    ('blocking', TRUE, 1, 2, 30, 'Fast blocked process collection'),
    ('deadlocks', TRUE, 1, 3, 30, 'Fast deadlock XML collection'),
    ('cpu_utilization', TRUE, 1, 2, 30, 'CPU utilization from ring buffer'),
    ('perfmon_stats', TRUE, 5, 2, 30, 'Performance counter statistics'),
    ('file_io_stats', TRUE, 1, 2, 30, 'File I/O statistics'),
    ('memory_grant_stats', TRUE, 1, 2, 30, 'Memory grant semaphore pressure'),
    ('cpu_scheduler_stats', TRUE, 1, 2, 30, 'CPU scheduler statistics'),
    ('latch_stats', TRUE, 1, 3, 30, 'Latch contention statistics'),
    ('spinlock_stats', TRUE, 1, 3, 30, 'Spinlock contention statistics'),
    ('tempdb_stats', TRUE, 1, 2, 30, 'TempDB space usage'),
    ('session_stats', TRUE, 1, 2, 30, 'Session and connection statistics'),
    ('waiting_tasks', TRUE, 1, 2, 30, 'Currently waiting tasks'),
    ('running_jobs', TRUE, 1, 2, 7, 'SQL Agent jobs'),
    ('query_store', TRUE, 15, 10, 30, 'Query Store data'),
    ('procedure_stats', TRUE, 2, 10, 30, 'Procedure statistics'),
    ('memory_clerks_stats', TRUE, 5, 3, 30, 'Memory clerk allocation'),
    ('plan_cache_stats', TRUE, 5, 5, 30, 'Plan cache composition'),
    ('query_snapshots', TRUE, 1, 2, 10, 'Currently executing queries'),
    ('server_configuration', TRUE, 1440, 5, 30, 'Server config (daily)'),
    ('database_configuration', TRUE, 1440, 10, 30, 'Database config (daily)'),
    ('server_properties', TRUE, 1440, 5, 365, 'Server properties (daily)'),
    ('database_size_stats', TRUE, 60, 10, 90, 'Database size (hourly)')
) AS v(collector_name, enabled, frequency_minutes, max_duration_minutes, retention_days, description)
ON CONFLICT (collector_name) DO NOTHING;
*/

-- ============================================================================
-- SEED DATA: Collection Frequencies (Unified)
-- ============================================================================
INSERT INTO optima_collector_configs (collector_name, module, frequency_seconds) VALUES
('Postgres Active Queries', 'Postgres', 60),
('Postgres Blocking Locks', 'Postgres', 60),
('Postgres CPU and Memory', 'Postgres', 120),
('Postgres Wait Stats', 'Postgres', 60),
('Postgres Storage I/O', 'Postgres', 300),
('Postgres Long Running Queries', 'Postgres', 60),
('Postgres Query Stats', 'Postgres', 900),
('SQL Server Active Queries', 'SQLSERVER', 60),
('SQL Server Blocking Locks', 'SQLSERVER', 30),
('SQL Server System Metrics', 'SQLSERVER', 60),
('SQL Server Storage', 'SQLSERVER', 120),
('SQL Server Long Running Queries', 'SQLSERVER', 60),
('SQL Server Query Store', 'SQLSERVER', 300),
('SQL Server Database Size', 'SQLSERVER', 3600),
('SQL Server Configuration', 'SQLSERVER', 86400),
('SQL Server Index Usage', 'SQLSERVER', 900),
('SQL Server Table Usage', 'SQLSERVER', 900),
('SQL Server Query Analysis', 'SQLSERVER', 300),
('SQL Server Watched Query Snapshot', 'SQLSERVER', 300),
('SQL Server AG Health', 'SQLSERVER', 60),
('SQL Server Enterprise Metrics', 'SQLSERVER', 300),
('SQL Server Agent Jobs', 'SQLSERVER', 60),
('sqlserver_query_snapshot', 'SQLSERVER', 60),
('sqlserver_session_enrichment', 'SQLSERVER', 30),
('sqlserver_blocking', 'SQLSERVER', 30),
('pg_queries_v2', 'Postgres', 60),
('Performance Debt Collection', 'SQLSERVER', 900),
('Alert Evaluation Loop', 'System', 60),
('Base Collector Ticker', 'System', 300),
('Postgres Session Activity', 'Postgres', 60),
('Postgres Wait Summary', 'Postgres', 60),
('Postgres DB Load', 'Postgres', 60),
('Postgres Query Wait Profile', 'Postgres', 300),
('Postgres Backup Archiver', 'Postgres', 300),
('Postgres WAL Rate', 'Postgres', 300),
('Postgres Base Backup History', 'Postgres', 300),
('Postgres Failed Login Parsing', 'Postgres', 120),
('Postgres Roles Snapshot', 'Postgres', 900),
('Postgres DDL Activity', 'Postgres', 900),
('Postgres Control Center', 'Postgres', 60),
('Postgres Connection Utilization', 'Postgres', 60),
('Postgres Incident Feed',          'Postgres', 60),
('Postgres Checkpoint Health',      'Postgres', 120),
('Postgres Deadlock Rate',          'Postgres', 60),
('Postgres Connections', 'Postgres', 60),
('Postgres Replication', 'Postgres', 60),
('Postgres System Detail', 'Postgres', 60),
('SQL Server Live KPIs', 'SQLSERVER', 60),
('SQL Server Running Queries', 'SQLSERVER', 60),
('SQL Server File IO Latency', 'SQLSERVER', 60),
('SQL Server TempDB Usage', 'SQLSERVER', 60),
('SQL Server Wait Stats Live', 'SQLSERVER', 60),
('SQL Server Connections Live', 'SQLSERVER', 60)
ON CONFLICT (collector_name) DO UPDATE SET frequency_seconds = EXCLUDED.frequency_seconds;

DO $$
BEGIN
    RAISE NOTICE 'Seed data inserted successfully!';
END $$;
