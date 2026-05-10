/*
================================================================================
RULE ENGINE CONSOLIDATED SCRIPT
================================================================================
Purpose: Complete rule engine schema for SQL Optima monitoring system
Supports both SQL Server and PostgreSQL targets
Order: Schema -> Tables -> Functions -> Views -> Rules

Script Order:
  1. Schema and base tables (from 001 + 002)
  2. Functions and routines (from 002)
  3. Views (from 002)
  4. Final comprehensive rules (from 006)
  5. Additional rules (from 007)
================================================================================
*/

-- ================================================================================
-- SECTION 1: SCHEMA AND BASE TABLES
-- ================================================================================

-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Create ruleengine schema
CREATE SCHEMA IF NOT EXISTS ruleengine;

-- 1.1: Rules Configuration Table
CREATE TABLE IF NOT EXISTS ruleengine.rules (
    rule_id              VARCHAR(50) PRIMARY KEY,
    rule_name            VARCHAR(200) NOT NULL,
    category             VARCHAR(100),
    applies_to           VARCHAR(50),
    severity             VARCHAR(20),
    dashboard_placement  VARCHAR(50),
    description          TEXT,
    detection_sql        TEXT,
    detection_sql_pg     TEXT,
    evaluation_logic     TEXT,
    expected_calc        TEXT,
    recommended_value    TEXT,
    fix_script           TEXT,
    fix_script_pg        TEXT,
    comparison_type      VARCHAR(20) DEFAULT 'exact',
    threshold_value      JSONB,
    priority             INTEGER DEFAULT 0,
    target_db_type       VARCHAR(20) DEFAULT 'sqlserver',
    required_signals     TEXT[],
    eval_engine          VARCHAR(20) DEFAULT 'go',
    eval_expression      JSONB,
    recommendation_template JSONB,
    rule_version         INT DEFAULT 1,
    is_enabled           BOOLEAN DEFAULT TRUE,
    applicability_sql    TEXT,
    context_tags         JSONB,
    confidence           VARCHAR(20),
    created_date         TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    modified_date        TIMESTAMPTZ NULL
);

-- 1.2: Servers Inventory Table
CREATE TABLE IF NOT EXISTS ruleengine.servers (
    server_id    SERIAL PRIMARY KEY,
    server_name  TEXT NOT NULL UNIQUE,
    environment  TEXT,
    sql_version  TEXT,
    cpu_cores    INT,
    total_ram_gb INT,
    db_type     VARCHAR(20) DEFAULT 'sqlserver',
    created_at   TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- 1.3: Rule Execution Run Table (Tracks each polling cycle)
CREATE TABLE IF NOT EXISTS ruleengine.rule_runs (
    run_id       BIGSERIAL PRIMARY KEY,
    server_id    INT REFERENCES ruleengine.servers(server_id) ON DELETE CASCADE,
    db_type     VARCHAR(20) DEFAULT 'sqlserver',
    run_time     TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- 1.4: Raw Rule Results Table (Timeseries - What the Go Agent sends)
CREATE TABLE IF NOT EXISTS ruleengine.rule_results_raw (
    run_id       BIGINT REFERENCES ruleengine.rule_runs(run_id) ON DELETE CASCADE,
    rule_id      VARCHAR(50) REFERENCES ruleengine.rules(rule_id),
    server_id    INT REFERENCES ruleengine.servers(server_id) ON DELETE CASCADE,
    raw_payload  JSONB,
    collected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Convert raw results to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.rule_results_raw',
    'collected_at',
    if_not_exists => TRUE
);

-- 1.5: Evaluated Results Table (Timeseries - What the UI reads)
CREATE TABLE IF NOT EXISTS ruleengine.rule_results_evaluated (
    run_id        BIGINT REFERENCES ruleengine.rule_runs(run_id) ON DELETE CASCADE,
    server_id     INT REFERENCES ruleengine.servers(server_id) ON DELETE CASCADE,
    rule_id       VARCHAR(50) REFERENCES ruleengine.rules(rule_id),
    target_db_type VARCHAR(20) DEFAULT 'sqlserver',
    status        TEXT,
    severity      VARCHAR(20),
    confidence    VARCHAR(20),
    current_value TEXT,
    recommended   TEXT,
    context       JSONB,
    evaluated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Convert evaluated results to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.rule_results_evaluated',
    'evaluated_at',
    if_not_exists => TRUE
);

/*
-- 1.6: Signals Table (NEW)
CREATE TABLE IF NOT EXISTS ruleengine.signals (
    server_id INT,
    signal_key TEXT,
    signal_value NUMERIC,
    collected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (server_id, signal_key, collected_at)
);

-- Convert signals to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.signals',
    'collected_at',
    if_not_exists => TRUE
);

-- 1.7: Signal Snapshots Table (NEW)
CREATE TABLE IF NOT EXISTS ruleengine.signal_snapshots (
    snapshot_id BIGSERIAL,
    server_id INT,
    db_type VARCHAR(20),
    snapshot JSONB,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (snapshot_id, created_at)
);

-- Convert signal_snapshots to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.signal_snapshots',
    'created_at',
    if_not_exists => TRUE
);
*/

-- 1.8: SQL Server Ignore Rules (NEW)
CREATE TABLE IF NOT EXISTS ruleengine.sqlserver_ignore_rules(
 rule_type text,
 rule_value text,
 PRIMARY KEY(rule_type,rule_value)
);

INSERT INTO ruleengine.sqlserver_ignore_rules VALUES
('database','master'),
('database','model'),
('database','msdb'),
('database','tempdb'),
('program','SQLAgent%'),
('program','sql-optima'),
('login','NT AUTHORITY\SYSTEM'),
('login','sa')
ON CONFLICT DO NOTHING;

-- 1.9: PostgreSQL Ignore Rules (NEW)
CREATE TABLE IF NOT EXISTS ruleengine.pg_ignore_rules(
 rule_type text,
 rule_value text,
 PRIMARY KEY(rule_type,rule_value)
);

INSERT INTO ruleengine.pg_ignore_rules VALUES
('database','postgres'),
('database','template0'),
('database','template1'),
('application','monitoring_collector'),
('application','sql-optima')
ON CONFLICT DO NOTHING;

-- Add target_db_type column if not exists
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'ruleengine' AND column_name = 'target_db_type') THEN
        ALTER TABLE ruleengine.rule_results_evaluated ADD COLUMN target_db_type VARCHAR(20) DEFAULT 'sqlserver';
    END IF;
END $$;

-- Ensure servers table has db_type column
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'ruleengine' AND table_name = 'servers' AND column_name = 'db_type') THEN
        ALTER TABLE ruleengine.servers ADD COLUMN db_type VARCHAR(20) DEFAULT 'sqlserver';
    END IF;
END $$;

-- ================================================================================
-- SECTION 2: FUNCTIONS AND ROUTINES
-- ================================================================================

-- FUNCTION: START NEW RULE RUN
CREATE OR REPLACE FUNCTION ruleengine.start_rule_run(p_server_id INT)
RETURNS BIGINT AS $$
DECLARE 
    v_run_id BIGINT;
BEGIN
    INSERT INTO ruleengine.rule_runs(server_id)
    VALUES (p_server_id)
    RETURNING run_id INTO v_run_id;

    RETURN v_run_id;
END;
$$ LANGUAGE plpgsql;

-- FUNCTION: STORE RAW RESULT FROM AGENT
CREATE OR REPLACE FUNCTION ruleengine.store_raw_result(
    p_run_id     BIGINT,
    p_server_id  INT,
    p_rule_id    VARCHAR(50),
    p_payload    JSONB
)
RETURNS VOID AS $$
BEGIN
    INSERT INTO ruleengine.rule_results_raw (run_id, rule_id, server_id, raw_payload, collected_at)
    VALUES (p_run_id, p_rule_id, p_server_id, p_payload, CURRENT_TIMESTAMP);
END;
$$ LANGUAGE plpgsql;

-- FUNCTION: EVALUATE RULE RESULTS (Supports dynamic evaluation from Go)
CREATE OR REPLACE FUNCTION ruleengine.evaluate_run(p_run_id BIGINT)
RETURNS VOID AS $$
DECLARE 
    r RECORD;
    v_current_value TEXT;
    v_recommended TEXT;
    v_status TEXT;
BEGIN
    FOR r IN
        SELECT rr.run_id, rr.server_id, rr.rule_id, rr.raw_payload, rl.target_db_type, rl.context_tags, rl.confidence
        FROM ruleengine.rule_results_raw rr
        JOIN ruleengine.rules rl USING(rule_id)
        WHERE rr.run_id = p_run_id
    LOOP
        v_current_value := r.raw_payload->>'CurrentValue';
        v_recommended := r.raw_payload->>'recommended_value';
        v_status := r.raw_payload->>'EvaluatedStatus';
        
        IF v_current_value IS NULL THEN
            BEGIN
                v_current_value := r.raw_payload::text;
            EXCEPTION WHEN OTHERS THEN
                v_current_value := NULL;
            END;
        END IF;
        
        IF v_status IS NULL THEN
            v_status := 'OK';
        END IF;

        INSERT INTO ruleengine.rule_results_evaluated
        (run_id, server_id, rule_id, target_db_type, status, current_value, recommended, context, confidence, evaluated_at)
        VALUES
        (r.run_id, r.server_id, r.rule_id, r.target_db_type, v_status, v_current_value, v_recommended, r.context_tags, r.confidence, CURRENT_TIMESTAMP);
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- ================================================================================
-- SECTION 3: VIEWS (UI / DASHBOARD QUERIES)
-- ================================================================================

-- MAIN DASHBOARD VIEW
CREATE OR REPLACE VIEW ruleengine.v_main_dashboard AS
SELECT DISTINCT ON (s.server_id, r.rule_id)
    s.server_name,
    r.rule_name,
    e.status,
    r.severity,
    r.dashboard_placement,
    e.evaluated_at
FROM ruleengine.rule_results_evaluated e
JOIN ruleengine.rules r ON e.rule_id = r.rule_id
JOIN ruleengine.servers s ON e.server_id = s.server_id
WHERE r.dashboard_placement = 'Main'
ORDER BY s.server_id, r.rule_id, e.evaluated_at DESC;

-- BEST PRACTICES DASHBOARD VIEW
CREATE OR REPLACE VIEW ruleengine.v_best_practices_dashboard AS
SELECT DISTINCT ON (s.server_id, r.rule_id)
    s.server_name,
    s.db_type AS target_db_type,
    r.category,
    r.rule_name,
    e.status,
    e.current_value,
    e.recommended AS recommended_value,
    COALESCE(r.fix_script, r.fix_script_pg) AS fix_script,
    r.description,
    e.context AS context_tags,
    e.confidence,
    e.evaluated_at
FROM ruleengine.rule_results_evaluated e
JOIN ruleengine.rules r ON e.rule_id = r.rule_id
JOIN ruleengine.servers s ON e.server_id = s.server_id
WHERE r.dashboard_placement = 'BestPractice'
ORDER BY s.server_id, r.rule_id, e.evaluated_at DESC;

-- BEST PRACTICES BY DB TYPE VIEW
CREATE OR REPLACE VIEW ruleengine.v_best_practices_by_type AS
SELECT DISTINCT ON (s.server_id, r.rule_id)
    s.server_id,
    s.server_name,
    s.db_type AS target_db_type,
    r.rule_id,
    r.rule_name,
    r.category,
    e.status,
    e.current_value,
    e.recommended AS recommended_value,
    COALESCE(r.fix_script, r.fix_script_pg) AS fix_script,
    r.description,
    e.context AS context_tags,
    e.confidence,
    e.evaluated_at
FROM ruleengine.rule_results_evaluated e
JOIN ruleengine.rules r ON e.rule_id = r.rule_id
JOIN ruleengine.servers s ON e.server_id = s.server_id
WHERE r.is_enabled = true
ORDER BY s.server_id, r.rule_id, e.evaluated_at DESC;

-- ================================================================================
-- SECTION 4: COMPREHENSIVE RULES (Refined & Updated)
-- ================================================================================

-- Clear existing rules for fresh start
DELETE FROM ruleengine.rule_results_evaluated;
DELETE FROM ruleengine.rule_results_raw;
DELETE FROM ruleengine.rule_runs;
DELETE FROM ruleengine.rules;

-- Insert Refined Rules (SQL Server & PostgreSQL)
INSERT INTO ruleengine.rules (
    rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, 
    detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, 
    fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, 
    is_enabled, applicability_sql, context_tags, confidence
) VALUES
-- SQL Server Fixes
('INST_MEM_MAX_001', 'Max Server Memory', 'Memory', 'Instance', 'Critical', 'Main', 'SQL Server consuming all RAM causes OS starvation. Tiered reservation recommended.', 'SELECT TOP 1 total_physical_memory_kb / 1024 / 1024 AS TotalRAM_GB, CAST(c.value_in_use AS BIGINT) AS MaxServerMemoryMB FROM sys.configurations c CROSS JOIN (SELECT TOP 1 total_physical_memory_kb FROM sys.dm_os_sys_memory) m WHERE c.name = ''max server memory (MB)'';', NULL, 'MaxServerMemoryMB >= 2147483647 ? "Critical" : (MaxServerMemoryMB < Expected ? "Warning" : "OK")', '(TotalRAM_GB <= 16 ? TotalRAM_GB - 4 : (TotalRAM_GB <= 32 ? TotalRAM_GB - 6 : (TotalRAM_GB <= 64 ? TotalRAM_GB - 8 : (TotalRAM_GB <= 128 ? TotalRAM_GB - 12 : (TotalRAM_GB <= 256 ? TotalRAM_GB - 16 : TotalRAM_GB - 32))))) * 1024', 'TotalRAM - OS Reserve', 'EXEC sp_configure ''max server memory (MB)'', <RecommendedMB>; RECONFIGURE;', NULL, 'threshold', '{"unit": "MB"}', 1, 'sqlserver', TRUE, NULL, '{"category": "instance_config"}', 'definitive'),
('INST_CPU_MAXDOP_003', 'MAXDOP Setting', 'Parallelism', 'Instance', 'Critical', 'Main', 'Wrong MAXDOP causes CXPACKET waits. Checks instance and database scoped configs.', 'SELECT (SELECT cpu_count FROM sys.dm_os_sys_info) AS cpu_count, (SELECT CAST(value_in_use AS INT) FROM sys.configurations WHERE name=''max degree of parallelism'') AS MAXDOP, (SELECT COUNT(DISTINCT parent_node_id) FROM sys.dm_os_schedulers WHERE status = ''VISIBLE ONLINE'' AND parent_node_id < 64) AS numa_node_count, ISNULL(STUFF((SELECT '', '' + d.name + '' ('' + CAST(v.value AS VARCHAR) + '')'' FROM sys.databases d CROSS APPLY (SELECT value FROM sys.database_scoped_configurations WHERE name = ''MAXDOP'') v WHERE v.value <> 0 FOR XML PATH('''')), 1, 2, ''''), ''None Scoped'') AS db_scoped_configs;', NULL, 'MAXDOP == 0 && cpu_count > 8 ? "Critical" : (MAXDOP == 0 && cpu_count <= 8 ? "Warning" : (MAXDOP > 8 ? "Warning" : "OK"))', 'cpu_count > 8 ? 8 : cpu_count', '<= 8 CPUs else 8 or cores per NUMA node', 'EXEC sp_configure ''max degree of parallelism'', <Value>; RECONFIGURE;', NULL, 'range', '{"min": 1, "max": 8}', 3, 'sqlserver', TRUE, NULL, '{"workload": "mixed"}', 'context_dependent'),
('TEMPDB_FILECOUNT_009', 'TempDB File Count & Size', 'TempDB', 'TempDB', 'Critical', 'Main', 'Improper TempDB layout causes PAGELATCH waits. Files must be equal sized.', 'SELECT COUNT(*) AS file_count, MIN(size) AS min_size_pages, MAX(size) AS max_size_pages, CASE WHEN MAX(size) > MIN(size) THEN 1 ELSE 0 END AS unequal_sizes FROM sys.master_files WHERE database_id = 2 AND type_desc = ''ROWS'';', NULL, 'unequal_sizes == 1 ? "Critical" : (file_count < 2 ? "Warning" : "OK")', '4', '4 or CPU/4', 'Manual TempDB reconfiguration required to match file sizes', NULL, 'range', '{"min": 4, "max": 8}', 9, 'sqlserver', TRUE, NULL, '{"category": "performance"}', 'definitive'),
('BACKUP_LOG_010', 'Log Backup Recent', 'Backup', 'Database', 'Critical', 'Main', 'Missing log backups break point-in-time recovery for FULL/BULK_LOGGED databases.', 'WITH DBs AS (SELECT name, recovery_model_desc FROM sys.databases WHERE database_id > 4 AND state = 0 AND recovery_model_desc IN (''FULL'', ''BULK_LOGGED'')), Backups AS (SELECT database_name, MAX(backup_finish_date) AS last_backup FROM msdb.dbo.backupset WHERE type = ''L'' GROUP BY database_name), Stats AS (SELECT d.name, DATEDIFF(MINUTE, b.last_backup, GETUTCDATE()) AS diff_min FROM DBs d LEFT JOIN Backups b ON d.name = b.database_name) SELECT (SELECT COUNT(*) FROM DBs) AS total_eligible, ISNULL(STUFF((SELECT '', '' + name + '' ('' + CAST(ISNULL(diff_min, 0) AS VARCHAR) + ''m)'' FROM Stats WHERE diff_min > 60 OR diff_min IS NULL FOR XML PATH('''')), 1, 2, ''''), ''All Healthy'') AS current_value, ISNULL(MAX(diff_min), 0) AS minutes_since_log_backup FROM Stats;', NULL, 'minutes_since_log_backup > 60 ? "Critical" : (minutes_since_log_backup > 15 ? "Warning" : "OK")', '15', '< 15 minutes', 'Create SQL Agent log backup job', NULL, 'threshold', '{"max": 15}', 10, 'sqlserver', TRUE, 'SELECT 1 FROM sys.databases WHERE recovery_model_desc IN (''FULL'', ''BULK_LOGGED'')', '{"recovery_model": "non-simple"}', 'definitive'),
('QUERY_STORE_011', 'Query Store Enabled', 'Monitoring', 'Database', 'Critical', 'Main', 'Query Store provides essential performance insights for regressions.', 'SELECT (SELECT COUNT(*) FROM sys.databases WHERE database_id > 4 AND state = 0 AND compatibility_level >= 130) AS total_eligible, (SELECT COUNT(*) FROM sys.databases WHERE database_id > 4 AND state = 0 AND compatibility_level >= 130 AND is_query_store_on = 1) AS enabled_count, ISNULL(STUFF((SELECT '', '' + name FROM sys.databases WHERE database_id > 4 AND state = 0 AND compatibility_level >= 130 AND is_query_store_on = 0 FOR XML PATH('''')), 1, 2, ''''), ''All Enabled'') AS current_value;', NULL, 'enabled_count == total_eligible ? "OK" : "Warning"', 'total_eligible', 'All Enabled', 'ALTER DATABASE [DatabaseName] SET QUERY_STORE = ON (OPERATION_MODE = READ_WRITE, CLEANUP_POLICY = (STALE_QUERY_THRESHOLD_DAYS = 30), DATA_FLUSH_INTERVAL_SECONDS = 900, MAX_STORAGE_SIZE_MB = 1000, QUERY_CAPTURE_MODE = AUTO);', NULL, 'exact', '{"value": 1}', 11, 'sqlserver', TRUE, 'SELECT 1 FROM sys.databases WHERE compatibility_level >= 130', '{"version": "2016+"}', 'definitive'),
('PERF_MEMORY_GRANTS_014', 'Memory Grants Pending', 'Performance', 'Instance', 'Critical', 'Main', 'Waiters for memory grants indicate severe memory pressure.', 'SELECT CAST(ISNULL((SELECT TOP 1 cntr_value FROM sys.dm_os_performance_counters WHERE counter_name = ''Memory Grants Pending'' AND object_name LIKE ''%Memory Manager%''), 0) AS BIGINT) AS memory_grants;', NULL, 'memory_grants == 0 ? "OK" : (memory_grants < 10 ? "Warning" : "Critical")', '0', '0', 'Investigate memory-intensive queries or increase server RAM.', NULL, 'threshold', '{"max": 0}', 14, 'sqlserver', TRUE, NULL, '{"category": "memory"}', 'definitive'),
('PERF_BLOCKING_015', 'Long Blocking', 'Performance', 'Instance', 'Critical', 'Main', 'Blocking chains detected lasting more than 5 seconds.', 'SELECT CAST(ISNULL((SELECT COUNT(*) FROM sys.dm_exec_requests WHERE blocking_session_id <> 0 AND wait_time > 5000), 0) AS INT) AS blocking_count;', NULL, 'blocking_count == 0 ? "OK" : (blocking_count < 3 ? "Warning" : "Critical")', '0', 'No long blocking', 'Identify the lead blocker and investigate query plan or lock contention.', NULL, 'threshold', '{"max": 0}', 15, 'sqlserver', TRUE, NULL, '{"category": "performance"}', 'definitive'),
('HA_AG_REPLICA_017', 'AG Replica Health', 'High Availability', 'Instance', 'Critical', 'Main', 'Unhealthy Always On replicas risk data loss.', 'SELECT CASE WHEN (SELECT COUNT(*) FROM sys.availability_groups) = 0 THEN -1 ELSE (SELECT COUNT(*) FROM sys.dm_hadr_database_replica_states drs JOIN sys.availability_replicas ar ON drs.replica_id = ar.replica_id WHERE drs.synchronization_health <> 2) END AS unhealthy_replicas;', NULL, 'unhealthy_replicas == -1 ? "N/A" : (unhealthy_replicas == 0 ? "OK" : "Critical")', '0', 'All replicas HEALTHY', 'Investigate replica synchronization status in dashboard.', NULL, 'threshold', '{"max": 0}', 17, 'sqlserver', TRUE, NULL, '{"feature": "alwayson"}', 'definitive'),
('BACKUP_RETENTION_013', 'Full Backup Recency', 'Backup', 'Instance', 'Critical', 'Main', 'Verifies that a full backup has been taken recently.', 'WITH DBs AS (SELECT name FROM sys.databases WHERE database_id > 4 AND state = 0 AND source_database_id IS NULL), Backups AS (SELECT database_name, MAX(backup_finish_date) AS last_backup FROM msdb.dbo.backupset WHERE type = ''D'' GROUP BY database_name), Stats AS (SELECT d.name, DATEDIFF(DAY, b.last_backup, GETUTCDATE()) AS diff_days FROM DBs d LEFT JOIN Backups b ON d.name = b.database_name) SELECT (SELECT COUNT(*) FROM DBs) AS total_eligible, ISNULL(STUFF((SELECT '', '' + name + '' ('' + CAST(ISNULL(diff_days, 0) AS VARCHAR) + ''d)'' FROM Stats WHERE diff_days > 7 OR diff_days IS NULL FOR XML PATH('''')), 1, 2, ''''), ''All Recent'') AS current_value, ISNULL(MAX(diff_days), 0) AS days_since_full_backup FROM Stats;', NULL, 'days_since_full_backup > 7 ? "Critical" : (days_since_full_backup > 1 ? "Warning" : "OK")', '1', '< 7 days', 'Execute a full database backup immediately.', NULL, 'threshold', '{"max": 7}', 13, 'sqlserver', TRUE, NULL, '{"category": "backup"}', 'definitive'),

-- SQL Server Disables
('INST_TRACEFLAGS_005', 'Important Trace Flags', 'Instance Config', 'Instance', 'Warning', 'BestPractice', 'Rule disabled: original detection logic was flawed and flags are deprecated in 2022.', NULL, NULL, '"OK"', NULL, 'Review TF 3226', NULL, NULL, 'exact', NULL, 5, 'sqlserver', FALSE, NULL, '{"reason": "deprecated"}', 'informational'),
('INST_LPIM_004', 'Lock Pages in Memory', 'Memory', 'Instance', 'Warning', 'BestPractice', 'Rule disabled: context-dependent; prefer proper Max Server Memory configuration.', NULL, NULL, '"OK"', NULL, 'Review OS memory model', NULL, NULL, 'exact', NULL, 4, 'sqlserver', FALSE, NULL, '{"reason": "context_dependent"}', 'informational'),
('AGENT_DISABLED_JOBS_010', 'Disabled Jobs Found', 'SQL Agent', 'Instance', 'Warning', 'BestPractice', 'Rule disabled: disabled jobs are often intentional.', NULL, NULL, '"OK"', NULL, 'Check maintenance jobs only', NULL, NULL, 'exact', NULL, 10, 'sqlserver', FALSE, NULL, '{"reason": "noise_reduction"}', 'informational'),
('AGENT_JOB_OWNER_011', 'Jobs Not Owned by SA', 'Security', 'Instance', 'Warning', 'BestPractice', 'Rule disabled: SA ownership is no longer the absolute recommendation.', NULL, NULL, '"OK"', NULL, 'Use least-privilege accounts', NULL, NULL, 'exact', NULL, 11, 'sqlserver', FALSE, NULL, '{"reason": "security_policy"}', 'informational'),
-- SQL Server New Rules
('AG_QUORUM_018', 'AG Cluster Quorum Status', 'High Availability', 'Instance', 'Critical', 'Main', 'Checks if the Always On cluster has a normal quorum state.', 'SELECT quorum_state_desc FROM sys.dm_hadr_cluster;', NULL, 'upper(quorum_state_desc) == "NORMAL_QUORUM" ? "OK" : "Critical"', 'NORMAL_QUORUM', 'NORMAL_QUORUM', 'Investigate cluster quorum health and witness status.', NULL, 'exact', '{"value": "NORMAL_QUORUM"}', 18, 'sqlserver', TRUE, NULL, '{"feature": "cluster"}', 'definitive'),
('TEMPDB_VLF_019', 'TempDB High VLF Count', 'TempDB', 'TempDB', 'Warning', 'BestPractice', 'High VLF count in TempDB log can slow down recovery and growth.', 'SELECT COUNT(*) AS vlf_count FROM tempdb.sys.database_files f CROSS APPLY sys.dm_db_log_info(2) WHERE f.type_desc = ''LOG'';', NULL, 'vlf_count > 500 ? "Critical" : (vlf_count > 200 ? "Warning" : "OK")', '100', '< 200', 'Shrink TempDB log and regrow in larger increments.', NULL, 'threshold', '{"max": 200}', 19, 'sqlserver', TRUE, NULL, '{"category": "maintenance"}', 'definitive'),
('STATISTICS_NORECOMPUTE_020', 'Indexes with NoRecompute Stats', 'Maintenance', 'Instance', 'Warning', 'BestPractice', 'NORECOMPUTE prevents automatic statistics updates, leading to stale plans.', 'SELECT COUNT(*) AS norecompute_count FROM sys.stats s JOIN sys.objects o ON s.object_id = o.object_id WHERE s.no_recompute = 1 AND o.is_ms_shipped = 0;', NULL, 'norecompute_count > 0 ? "Warning" : "OK"', '0', '0', 'Update statistics with RECOMPUTE enabled.', NULL, 'threshold', '{"max": 0}', 20, 'sqlserver', TRUE, NULL, '{"category": "performance"}', 'informational'),
-- PG Fixes
('PG_SHARED_BUFFERS_001', 'Shared Buffers', 'Memory', 'Instance', 'Critical', 'Main', 'PostgreSQL shared_buffers should be ~25% of total RAM.', 'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''shared_buffers'';', NULL, 'setting < 131072 ? "Critical" : (setting < 262144 ? "Warning" : "OK")', '262144', '2GB (for ≥8GB RAM servers)', NULL, 'ALTER SYSTEM SET shared_buffers = ''2GB'';', 'threshold', '{"min": 262144}', 1, 'postgres', TRUE, NULL, '{"unit": "blocks_8k"}', 'definitive'),
('PG_MAX_CONNECTIONS_001', 'Max Connections', 'Connection', 'Instance', 'Warning', 'BestPractice', 'High connections consume memory; use a pooler like PgBouncer.', 'SELECT setting::int AS setting FROM pg_settings WHERE name = ''max_connections'';', NULL, 'setting > 300 ? "Warning" : "OK"', '100', 'Use PgBouncer/pgpool; < 200', NULL, 'ALTER SYSTEM SET max_connections = ''100'';', 'threshold', '{"max": 300}', 2, 'postgres', TRUE, NULL, '{"category": "scalability"}', 'context_dependent'),
('PG_WORK_MEM_001', 'Work Mem', 'Memory', 'Instance', 'Warning', 'BestPractice', 'Complex sorts spill to disk. Avoid setting too high globally.', 'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''work_mem'';', NULL, 'setting < 1024 ? "Critical" : (setting < 4096 ? "Warning" : (setting > 65536 ? "Warning" : "OK"))', '4096', '4-16 MB (4096-16384 kB)', NULL, 'ALTER SYSTEM SET work_mem = ''8MB'';', 'threshold', '{"min": 4096}', 3, 'postgres', TRUE, NULL, '{"unit": "kB"}', 'context_dependent'),
('PG_MAINT_WORK_MEM_001', 'Maintenance Work Mem', 'Maintenance', 'Instance', 'Warning', 'BestPractice', 'Controls memory for VACUUM and CREATE INDEX.', 'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''maintenance_work_mem'';', NULL, 'setting < 32768 ? "Warning" : (setting > 2097152 ? "Warning" : "OK")', '262144', '256 MB-1 GB', NULL, 'ALTER SYSTEM SET maintenance_work_mem = ''256MB'';', 'threshold', '{"min": 262144}', 4, 'postgres', TRUE, NULL, '{"unit": "kB"}', 'context_dependent'),
('PG_WAL_BUFFERS_001', 'WAL Buffers', 'Performance', 'Instance', 'Warning', 'BestPractice', 'Low WAL buffers causes frequent flushes. Handles -1 (auto) case.', 'SELECT setting::bigint AS setting, CASE WHEN setting::bigint = -1 THEN (SELECT setting::bigint / 32 FROM pg_settings WHERE name = ''shared_buffers'') ELSE setting::bigint END AS effective_wal_buffers_pages FROM pg_settings WHERE name = ''wal_buffers'';', NULL, 'effective_wal_buffers_pages < 256 ? "Warning" : "OK"', '256', '16MB', NULL, 'ALTER SYSTEM SET wal_buffers = ''16MB'';', 'threshold', '{"min": 256}', 8, 'postgres', TRUE, NULL, '{"unit": "pages_8k"}', 'definitive'),
('PG_DEAD_TUPLES_001', 'Dead Tuple Ratio', 'Maintenance', 'Database', 'Critical', 'Main', 'High dead tuples indicate autovacuum issues.', 'SELECT schemaname || ''.'' || relname AS worst_table, n_dead_tup, n_live_tup, ROUND((n_dead_tup::numeric / NULLIF(n_live_tup, 0)) * 100, 1) AS dead_pct FROM pg_stat_user_tables WHERE n_dead_tup > 1000 AND n_live_tup + n_dead_tup > 10000 ORDER BY dead_pct DESC LIMIT 1;', NULL, 'dead_pct > 20 ? "Critical" : (dead_pct > 10 ? "Warning" : "OK")', '10', '< 10%', NULL, 'VACUUM ANALYZE;', 'threshold', '{"max": 10}', 9, 'postgres', TRUE, NULL, '{"category": "bloat"}', 'definitive'),
('PG_LONG_TX_020', 'Long Running Transactions', 'Performance', 'Instance', 'Warning', 'BestPractice', 'Excludes autovacuum and background workers.', 'SELECT COUNT(*) AS long_tx_count FROM pg_stat_activity WHERE state != ''idle'' AND now() - xact_start > interval ''5 minutes'' AND backend_type = ''client backend'' AND xact_start IS NOT NULL;', NULL, 'long_tx_count == 0 ? "OK" : "Warning"', '0', 'No long transactions', NULL, 'SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE ...', 'threshold', '{"max": 0}', 20, 'postgres', TRUE, NULL, '{"category": "concurrency"}', 'definitive'),
('PG_EFFECTIVE_CACHE_SIZE_001', 'Effective Cache Size', 'Performance', 'Instance', 'Warning', 'BestPractice', 'Planner hint for OS cache availability.', 'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''effective_cache_size'';', NULL, 'setting < 131072 ? "Warning" : "OK"', '1048576', '50-75% of total RAM', NULL, 'ALTER SYSTEM SET effective_cache_size = ''8GB'';', 'threshold', '{"min": 131072}', 6, 'postgres', TRUE, NULL, '{"unit": "blocks_8k"}', 'context_dependent'),
-- PG Disables
('PG_IDLE_TX_018', 'Idle in Transaction (Aggressive)', 'Connection', 'Instance', 'Critical', 'Main', 'Rule disabled: superseded by PG_IDLE_TX_001 with 5-minute threshold.', NULL, NULL, '"OK"', NULL, 'See PG_IDLE_TX_001', NULL, NULL, 'exact', NULL, 18, 'postgres', FALSE, NULL, '{"reason": "duplicate"}', 'informational'),
('PG_REPLICATION_LAG_019', 'Replication Lag (Aggressive)', 'High Availability', 'Instance', 'Critical', 'Main', 'Rule disabled: superseded by PG_REPL_LAG_001 with tiered thresholds.', NULL, NULL, '"OK"', NULL, 'See PG_REPL_LAG_001', NULL, NULL, 'exact', NULL, 19, 'postgres', FALSE, NULL, '{"reason": "duplicate"}', 'informational'),
-- PG New Rules
('PG_UNUSED_INDEXES_031', 'Unused Indexes', 'Performance', 'Database', 'Warning', 'BestPractice', 'Indexes with zero scans after 7 days uptime.', 'SELECT COUNT(*) AS cnt, EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time())) / 86400 AS server_uptime_days FROM pg_stat_user_indexes WHERE idx_scan = 0 AND schemaname NOT IN (''pg_catalog'', ''information_schema'');', NULL, 'server_uptime_days < 7 ? "INFO" : (cnt == 0 ? "OK" : "Warning")', '0', '0', NULL, 'DROP INDEX <index_name>;', 'threshold', '{"max": 0}', 31, 'postgres', TRUE, NULL, '{"requires": "7d_uptime"}', 'context_dependent'),
('PG_AUTOVAC_FREEZE_032', 'Autovacuum Freeze Max Age', 'Maintenance', 'Instance', 'Warning', 'BestPractice', 'Prevents transaction ID wraparound issues.', 'SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''autovacuum_freeze_max_age'';', NULL, 'setting > 800000000 ? "Critical" : (setting > 200000000 ? "Warning" : "OK")', '200000000', '200,000,000', NULL, 'ALTER SYSTEM SET autovacuum_freeze_max_age = 200000000;', 'threshold', '{"max": 200000000}', 32, 'postgres', TRUE, NULL, '{"category": "safety"}', 'context_dependent'),
('PG_SEQUENCE_EXHAUSTION_033', 'Sequence Exhaustion Risk', 'Schema', 'Database', 'Critical', 'BestPractice', 'Checks sequences approaching their maximum value (80% threshold).', 'SELECT COUNT(*) AS cnt FROM pg_sequences WHERE maximum_value > 0 AND last_value IS NOT NULL AND (last_value::numeric - minimum_value::numeric + 1) / NULLIF((maximum_value::numeric - minimum_value::numeric + 1), 0) * 100 >= 80;', NULL, 'cnt == 0 ? "OK" : "Critical"', '0', '0', NULL, 'ALTER SEQUENCE <seq> AS BIGINT;', 'threshold', '{"max": 0}', 33, 'postgres', TRUE, NULL, '{"category": "availability"}', 'definitive');

-- ================================================================================
-- SECTION 6: VERIFICATION
-- ================================================================================

-- Verify all rules
DO $$
DECLARE
    v_sqlserver_count INTEGER;
    v_postgres_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_sqlserver_count FROM ruleengine.rules WHERE target_db_type = 'sqlserver';
    SELECT COUNT(*) INTO v_postgres_count FROM ruleengine.rules WHERE target_db_type = 'postgres';
    
    RAISE NOTICE '============================================';
    RAISE NOTICE 'Rule Engine Schema Created Successfully!';
    RAISE NOTICE 'SQL Server Rules: %', v_sqlserver_count;
    RAISE NOTICE 'PostgreSQL Rules: %', v_postgres_count;
    RAISE NOTICE 'Total Rules: %', v_sqlserver_count + v_postgres_count;
    RAISE NOTICE '============================================';
END $$;

-- Show all rules
SELECT rule_id, rule_name, category, severity, dashboard_placement, target_db_type, is_enabled 
FROM ruleengine.rules 
ORDER BY target_db_type, priority, rule_id;
