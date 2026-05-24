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
    $$SELECT capture_timestamp AS time, cpu_usage AS value FROM postgres_system_stats WHERE server_id = '{{server_name}}' ORDER BY capture_timestamp DESC LIMIT 30$$,
    $$SELECT capture_timestamp AS time, cpu_usage AS value FROM postgres_system_stats WHERE server_id = '{{server_name}}' ORDER BY capture_timestamp DESC LIMIT 30$$
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
    $$SELECT capture_timestamp AS time, tps AS value FROM postgres_throughput_metrics WHERE server_id = '{{server_name}}' AND database_name = '{{database}}' ORDER BY capture_timestamp DESC LIMIT 30$$,
    $$SELECT capture_timestamp AS time, tps AS value FROM postgres_throughput_metrics WHERE server_id = '{{server_name}}' AND database_name = '{{database}}' ORDER BY capture_timestamp DESC LIMIT 30$$
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
    $$SELECT capture_timestamp AS time, max_lag_mb AS value FROM postgres_replication_stats WHERE server_id = '{{server_name}}' ORDER BY capture_timestamp DESC LIMIT 30$$,
    $$SELECT capture_timestamp AS time, max_lag_mb AS value FROM postgres_replication_stats WHERE server_id = '{{server_name}}' ORDER BY capture_timestamp DESC LIMIT 30$$
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
-- run_order reflects the goroutine launch sequence from appserver.go:
--   10  = PulseService (first to start; real-time ≤30 s pulses)
--   20  = BackgroundCollector (table-driven; most Postgres + legacy SS collectors)
--   30  = PerformanceDebtCollector
--   40  = QueryAnalysisCollector  (SS Query Analysis + Watched Query Snapshot)
--   50  = SqlServerStorageHistoryCollector
--   60  = PostgresNewDashboardsCollectors
--   70  = PostgresStorageIndexHealthCollector
--   80  = QueryStoreCollector
--   100 = SqlServerHealthCollector  (Live KPIs, Health KPIs, Running Queries)
--   110 = SqlServerHAReplicationCollector
--   120 = SqlServerDatabaseCatalogCollector  (immediate first-run at startup)
--   130 = XEFileTargetWorker
--   140 = SharedWaitStatsCollector  (Wait Stats Cumulative + Live)
--   150 = ActiveWaitSessionsCollector
--   160 = FileIOLatencyCollector
--   170 = MemoryIntelligenceCollector
--   180 = QueryV2Collector
--   NULL = System/framework (Alert Loop, Base Ticker, Asynq, metadata pulse)
INSERT INTO optima_collector_configs (collector_name, module, frequency_seconds, is_active, run_order) VALUES
-- ── Postgres: BackgroundCollector (20) ─────────────────────────────────────
('Postgres Active Queries',          'Postgres',  60,    true, 20),
('Postgres Blocking Locks',          'Postgres',  60,    true, 20),
('Postgres CPU and Memory',          'Postgres',  120,   true, 20),
('Postgres Wait Stats',              'Postgres',  60,    true, 20),
('Postgres Storage I/O',             'Postgres',  300,   true, 20),
('Postgres Long Running Queries',    'Postgres',  60,    true, 20),
('Postgres Query Stats',             'Postgres',  60,   true, 20),
('Postgres Session Activity',        'Postgres',  60,    true, 20),
('Postgres Wait Summary',            'Postgres',  60,    true, 20),
('Postgres DB Load',                 'Postgres',  60,    true, 20),
('Postgres Query Wait Profile',      'Postgres',  300,   true, 20),
('Postgres Backup Archiver',         'Postgres',  300,   true, 20),
('Postgres WAL Rate',                'Postgres',  300,   true, 20),
('Postgres Base Backup History',     'Postgres',  300,   true, 20),
('Postgres Failed Login Parsing',    'Postgres',  120,   true, 20),
('Postgres Roles Snapshot',          'Postgres',  900,   true, 20),
('Postgres DDL Activity',            'Postgres',  900,   true, 20),
('Postgres Control Center',          'Postgres',  60,    true, 20),
('Postgres Connection Utilization',  'Postgres',  60,    true, 20),
('Postgres Incident Feed',           'Postgres',  60,    true, 20),
('Postgres Checkpoint Health',       'Postgres',  120,   true, 20),
('Postgres Deadlock Rate',           'Postgres',  60,    true, 20),
('Postgres Connections',             'Postgres',  60,    true, 20),
('Postgres Replication',             'Postgres',  60,    true, 20),
('Postgres System Detail',           'Postgres',  60,    true, 20),
('Postgres PGSS',                    'Postgres',  60,    true, 20),
('pg_queries_v2',                    'Postgres',  60,    true, 20),
-- ── SQL Server: BackgroundCollector (20) ───────────────────────────────────
('SQL Server Active Queries',        'SQLSERVER', 60,    true, 20),
('SQL Server Blocking Locks',        'SQLSERVER', 30,    true, 20),
('SQL Server System Metrics',        'SQLSERVER', 60,    true, 20),
('SQL Server Storage',               'SQLSERVER', 86400, true, 20),
('SQL Server Long Running Queries',  'SQLSERVER', 60,    true, 20),
('SQL Server Database Size',         'SQLSERVER', 3600,  true, 20),
('SQL Server Configuration',         'SQLSERVER', 86400, true, 20),
('SQL Server Index Usage',           'SQLSERVER', 900,   true, 20),
('SQL Server Table Usage',           'SQLSERVER', 900,   true, 20),
('SQL Server AG Health',             'SQLSERVER', 60,    true, 20),
('SQL Server Health KPIs',           'SQLSERVER', 30,    true, 20),
('SQL Server Enterprise Metrics',    'SQLSERVER', 3600,  true, 20),
('SQL Server Job Details',           'SQLSERVER', 300,   true, 20),
('SQL Server Spinlock Stats',        'SQLSERVER', 300,   true, 20),
('SQL Server Agent Jobs',            'SQLSERVER', 60,    true, 20),
('sqlserver_query_snapshot',         'SQLSERVER', 60,    true, 20),
('sqlserver_session_enrichment',     'SQLSERVER', 30,    true, 20),
('sqlserver_blocking',               'SQLSERVER', 30,    true, 20),
('SQL Server TempDB Usage',          'SQLSERVER', 60,    true, 20),
-- ── PulseService managed (10) ──────────────────────────────────────────────
('sqlserver_sessions_activity_pulse','SQLSERVER', 30,    true, 10),
('sqlserver_performance_kpi_pulse',  'SQLSERVER', 60,    true, 10),
('sqlserver_io_memory_grants_pulse', 'SQLSERVER', 120,   true, 10),
('sqlserver_locks_pulse',            'SQLSERVER', 30,    true, 10),
('SQL Server Perf Counters Delta',   'SQLSERVER', 30,    true, 10),
('postgres_activity_locks_pulse',    'Postgres',  30,    true, 10),
('postgres_global_stats_pulse',      'Postgres',  60,    true, 10),
('pg_locks_blocking',                'Postgres',  30,    true, 10),
('sqlserver_locks_blocking',         'SQLSERVER', 30,    true, 10),
('pg_new_dashboards_worker',         'Postgres',  30,    true, 10),
-- ── Dedicated goroutines ───────────────────────────────────────────────────
('Performance Debt Collection',      'SQLSERVER', 900,   true, 30),
('SQL Server Query Analysis',        'SQLSERVER', 300,   true, 40),
('SQL Server Watched Query Snapshot','SQLSERVER', 300,   true, 40),
('SQL Server Storage History',       'SQLSERVER', 21600, true, 50),
('Postgres Dashboard Snapshot',      'Postgres',  60,    true, 60),
('pg_storage_index_health',          'Postgres',  300,   true, 70),
('pg_storage_index_health_index15m', 'Postgres',  900,   true, 70),
('pg_storage_index_health_growth6h', 'Postgres',  21600, true, 70),
('pg_storage_index_health_defs_daily','Postgres', 86400, true, 70),
('SQL Server Query Store',           'SQLSERVER', 300,   true, 80),
('SQL Server Live KPIs',             'SQLSERVER', 60,    true, 100),
('SQL Server Running Queries',       'SQLSERVER', 60,    true, 100),
('sqlserver_ha_replication',         'SQLSERVER', 30,    true, 110),
('sqlserver_ha_discovery',           'SQLSERVER', 900,   true, 110),
('sqlserver_ha_health',              'SQLSERVER', 30,    true, 110),
('sqlserver_replication_performance','SQLSERVER', 60,    true, 110),
('sqlserver_replication_topology',   'SQLSERVER', 900,   true, 110),
('SQL Server Database Catalog',      'SQLSERVER', 3600,  true, 120),
('SQL Server Extended Events',       'SQLSERVER', 60,    true, 130),
('SQL Server Wait Stats Cumulative', 'SQLSERVER', 300,   true, 140),
('SQL Server Wait Stats Live',       'SQLSERVER', 60,    true, 140),
('SQL Server Active Wait Sessions',  'SQLSERVER', 30,    true, 150),
('SQL Server File IO Latency',       'SQLSERVER', 3600,  true, 160),
('SQL Server Memory Intelligence',   'SQLSERVER', 3600,  true, 170),
('SQL Server Volume Stats',          'SQLSERVER', 3600,  true, 175),
('Query V2 Pipeline',                'System',    5,     true, 180),
-- ── System / framework (NULL = no startup sequence) ────────────────────────
('Alert Evaluation Loop',            'System',    60,    true, NULL),
('Base Collector Ticker',            'System',    300,   true, NULL),
('Asynq Historical',                 'System',    60,    true, NULL),
('metadata_configuration_pulse',     'System',    900,   true, NULL)
ON CONFLICT (collector_name) DO UPDATE SET
    frequency_seconds = EXCLUDED.frequency_seconds,
    module            = EXCLUDED.module,
    run_order         = EXCLUDED.run_order;


/*
-- Mark collectors with no active implementation as inactive.
-- Only deactivates entries that haven't been explicitly managed by an admin
-- (updated_by IS NULL means the row was only ever touched by seed/system).
UPDATE optima_collector_configs
SET is_active = false
WHERE collector_name IN (
    'Postgres Active Queries',
    'Postgres Blocking Locks',
    'Postgres CPU and Memory',
    'Postgres Wait Stats',
    'Postgres Storage I/O',
    'Postgres Long Running Queries',
    'Postgres Query Stats',
    'SQL Server Active Queries',
    'SQL Server Blocking Locks',
    'SQL Server System Metrics',
    'SQL Server Storage',
    'SQL Server Long Running Queries',
    'SQL Server Database Size',
    'SQL Server Configuration',
    'SQL Server Index Usage',
    'SQL Server Table Usage',
    'SQL Server AG Health',
    'SQL Server Enterprise Metrics',
    'SQL Server Job Details',
    'SQL Server Spinlock Stats',
    'SQL Server Agent Jobs',
    'sqlserver_blocking',
    'Query V2 Pipeline',
    'Postgres Session Activity',
    'Postgres Wait Summary',
    'Postgres DB Load',
    'Postgres Query Wait Profile',
    'Postgres Backup Archiver',
    'Postgres WAL Rate',
    'Postgres Base Backup History',
    'Postgres Failed Login Parsing',
    'Postgres Roles Snapshot',
    'Postgres DDL Activity',
    'Postgres Control Center',
    'Postgres Connection Utilization',
    'Postgres Incident Feed',
    'Postgres Checkpoint Health',
    'Postgres Deadlock Rate',
    'Postgres Connections',
    'Postgres Replication',
    'Postgres System Detail',
    'SQL Server Live KPIs',
    'SQL Server Running Queries',
    'SQL Server TempDB Usage',
    'SQL Server Wait Stats Live',
    'postgres_activity_locks_pulse',
    'sqlserver_performance_kpi_pulse',
    'postgres_global_stats_pulse',
    'sqlserver_io_memory_grants_pulse',
    'sqlserver_locks_pulse',
    'metadata_configuration_pulse',
    'SQL Server Perf Counters Delta'
)
AND (updated_by IS NULL OR updated_by = '' OR updated_by = 'system');
*/

-- Wait type → category mapping (comprehensive)
INSERT INTO sqlserver_wait_type_category (wait_type, category, sort_order) VALUES
    ('SOS_SCHEDULER_YIELD',              'CPU',         1),
    ('THREADPOOL',                       'CPU',         1),
    ('CMEMTHREAD',                       'CPU',         1),
    ('PAGEIOLATCH_SH',                   'IO_DATA',     2),
    ('PAGEIOLATCH_EX',                   'IO_DATA',     2),
    ('PAGEIOLATCH_UP',                   'IO_DATA',     2),
    ('PAGEIOLATCH_KP',                   'IO_DATA',     2),
    ('IO_COMPLETION',                    'IO_DATA',     2),
    ('ASYNC_IO_COMPLETION',              'IO_DATA',     2),
    ('WRITELOG',                         'IO_LOG',      3),
    ('LOGBUFFER',                        'IO_LOG',      3),
    ('LOGMGR',                           'IO_LOG',      3),
    ('LOGMGR_FLUSH',                     'IO_LOG',      3),
    ('LOGMGR_RESERVE_APPEND',            'IO_LOG',      3),
    ('LCK_M_S',                          'LOCKING',     4),
    ('LCK_M_X',                          'LOCKING',     4),
    ('LCK_M_U',                          'LOCKING',     4),
    ('LCK_M_IS',                         'LOCKING',     4),
    ('LCK_M_IX',                         'LOCKING',     4),
    ('LCK_M_SIX',                        'LOCKING',     4),
    ('LCK_M_UIX',                        'LOCKING',     4),
    ('RESOURCE_SEMAPHORE',               'MEMORY',      5),
    ('RESOURCE_SEMAPHORE_QUERY_COMPILE', 'MEMORY',      5),
    ('CMEMPARTITIONED',                  'MEMORY',      5),
    ('RESERVED_MEMORY_ALLOCATION_EXT',   'MEMORY',      5),
    ('CXPACKET',                         'PARALLELISM', 6),
    ('CXCONSUMER',                       'PARALLELISM', 6),
    ('EXECSYNC',                         'PARALLELISM', 6),
    ('ASYNC_NETWORK_IO',                 'NETWORK',     7),
    ('NET_WAITFOR_PACKET',               'NETWORK',     7)
ON CONFLICT (wait_type) DO NOTHING;

-- Wait type help text (used by dashboard tooltips)
INSERT INTO sqlserver_wait_type_help
    (wait_type, description, likely_cause, recommended_action,
     threshold_warning_ms, threshold_critical_ms) VALUES
('PAGEIOLATCH_SH',
 'Waiting for a data page to be read from disk into the buffer pool.',
 'Slow storage, missing indexes causing large scans, insufficient RAM for working set.',
 'Check sys.dm_io_virtual_file_stats for read latency. Review missing indexes. Increase max server memory.',
 500, 2000),
('WRITELOG',
 'Waiting for the transaction log buffer to be flushed to disk.',
 'Slow log disk, high write transaction rate.',
 'Move transaction log to a dedicated fast SSD. Measure latency with sys.dm_io_virtual_file_stats.',
 200, 1000),
('LCK_M_X',
 'Waiting for an exclusive lock held by another session.',
 'Long-running transactions, missing indexes causing table-level locks.',
 'Identify the blocking head session. Review transaction length. Add appropriate indexes.',
 100, 500),
('SOS_SCHEDULER_YIELD',
 'Session voluntarily yielding CPU to allow other tasks to run — signal of CPU saturation.',
 'Too many runnable threads competing for CPU cores.',
 'Check CPU utilisation. Identify top CPU queries in sys.dm_exec_query_stats. Tune MAXDOP.',
 1000, 5000),
('RESOURCE_SEMAPHORE',
 'Query waiting for a memory grant before it can begin execution.',
 'Insufficient memory, large sort/hash operations, high concurrency.',
 'Review max server memory, Resource Governor. Tune queries with large memory grants.',
 200, 1000),
('CXPACKET',
 'Parallel query threads waiting for the slowest thread in the same plan.',
 'Unbalanced parallel plans, data skew.',
 'Review MAXDOP settings. Check Cost Threshold for Parallelism. Inspect actual parallel plans.',
 500, 2000),
('ASYNC_NETWORK_IO',
 'SQL Server has results ready but the client is not consuming them fast enough.',
 'Slow client network, large result sets, client application processing delay.',
 'Reduce result set sizes. Add pagination. Check client network bandwidth.',
 200, 1000)
ON CONFLICT (wait_type) DO NOTHING;



DO $$
BEGIN
    RAISE NOTICE 'Seed data inserted successfully!';
END $$;
