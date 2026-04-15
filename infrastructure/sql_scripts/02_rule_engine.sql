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
    is_enabled           BOOLEAN DEFAULT TRUE,
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
    current_value TEXT,
    recommended   TEXT,
    evaluated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Convert evaluated results to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.rule_results_evaluated',
    'evaluated_at',
    if_not_exists => TRUE
);

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
        SELECT rr.run_id, rr.server_id, rr.rule_id, rr.raw_payload, rl.target_db_type
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
        (run_id, server_id, rule_id, target_db_type, status, current_value, recommended, evaluated_at)
        VALUES
        (r.run_id, r.server_id, r.rule_id, r.target_db_type, v_status, v_current_value, v_recommended, CURRENT_TIMESTAMP);

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
    e.evaluated_at
FROM ruleengine.rule_results_evaluated e
JOIN ruleengine.rules r ON e.rule_id = r.rule_id
JOIN ruleengine.servers s ON e.server_id = s.server_id
WHERE r.is_enabled = true
ORDER BY s.server_id, r.rule_id, e.evaluated_at DESC;

-- ================================================================================
-- SECTION 4: COMPREHENSIVE RULES (from 006_complete_rules_fix.sql)
-- ================================================================================

-- Clear existing rules for fresh start
DELETE FROM ruleengine.rule_results_evaluated;
DELETE FROM ruleengine.rule_results_raw;
DELETE FROM ruleengine.rule_runs;
DELETE FROM ruleengine.rules;

-- Insert SQL Server Rules
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_MEM_MAX_001','Max Server Memory','Instance Config','Instance','Critical','Main','SQL Server consuming all RAM causes OS starvation.','SELECT total_physical_memory_kb/1024/1024 AS TotalRAM_GB, CAST(value_in_use AS BIGINT) AS MaxServerMemoryMB FROM sys.configurations CROSS JOIN sys.dm_os_sys_memory WHERE name=''max server memory (MB)'';',NULL,'MaxServerMemoryMB >= 2147483647 ? "Critical" : (MaxServerMemoryMB < Recommended ? "Warning" : "OK")','(TotalRAM_GB <= 16 ? TotalRAM_GB - 4 : (TotalRAM_GB <= 32 ? TotalRAM_GB - 6 : (TotalRAM_GB <= 64 ? TotalRAM_GB - 8 : (TotalRAM_GB <= 128 ? TotalRAM_GB - 12 : (TotalRAM_GB <= 256 ? TotalRAM_GB - 16 : TotalRAM_GB - 32))))) * 1024','TotalRAM - OS Reserve','EXEC sp_configure ''max server memory (MB)'', <RecommendedMB>; RECONFIGURE;',NULL,'threshold','{"unit":"MB"}',1,'sqlserver',TRUE),

('INST_CPU_MAXDOP_003','MAXDOP Setting','Parallelism','Instance','Critical','Main','Wrong MAXDOP causes CXPACKET waits.','SELECT cpu_count, (SELECT CAST(value_in_use AS INT) FROM sys.configurations WHERE name=''max degree of parallelism'') AS MAXDOP FROM sys.dm_os_sys_info;',NULL,'MAXDOP <= 0 ? "Critical" : (MAXDOP > 8 ? "Warning" : "OK")','cpu_count > 8 ? 8 : cpu_count','<=8 CPUs else 8','EXEC sp_configure ''max degree of parallelism'', <Value>; RECONFIGURE;',NULL,'range','{"min":1,"max":8}',2,'sqlserver',TRUE),

('INST_CPU_CTFP_004','Cost Threshold for Parallelism','Parallelism','Instance','Critical','Main','Default value 5 is too low for modern servers.','SELECT CAST(value_in_use AS INT) AS value_in_use FROM sys.configurations WHERE name=''cost threshold for parallelism'';',NULL,'value_in_use < 50 ? "Warning" : "OK"','50','50','EXEC sp_configure ''cost threshold for parallelism'',50; RECONFIGURE;',NULL,'threshold','{"min":50}',3,'sqlserver',TRUE),

('INST_PLAN_CACHE_005','Optimize for AdHoc Workloads','Plan Cache','Instance','Critical','Main','Prevents plan cache bloat.','SELECT CAST(value_in_use AS INT) AS value_in_use FROM sys.configurations WHERE name=''optimize for ad hoc workloads'';',NULL,'value_in_use == 1 ? "OK" : "Warning"','1','1 (enabled)','EXEC sp_configure ''optimize for ad hoc workloads'',1; RECONFIGURE;',NULL,'exact','{"value":1}',4,'sqlserver',TRUE),

('INST_IFI_006','Instant File Initialization','Storage','OS','Critical','Main','Database growth freezes server without IFI.','SELECT CAST(instant_file_initialization_enabled AS INT) AS instant_file_initialization_enabled FROM sys.dm_server_services;',NULL,'instant_file_initialization_enabled == 1 ? "OK" : "Critical"','1','1 (enabled)','Grant Perform Volume Maintenance Tasks to SQL Service Account',NULL,'exact','{"value":1}',5,'sqlserver',TRUE),

('DB_AUTOSHRINK_007','Auto Shrink Enabled','Database Config','Database','Critical','Main','Causes fragmentation and performance issues.','SELECT CAST(ISNULL((SELECT TOP 1 is_auto_shrink_on FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS is_auto_shrink_on;',NULL,'is_auto_shrink_on == 1 ? "Critical" : "OK"','0','OFF','ALTER DATABASE SET AUTO_SHRINK OFF;',NULL,'exact','{"value":0}',6,'sqlserver',TRUE),

('DB_AUTOCLOSE_008','Auto Close Enabled','Database Config','Database','Critical','Main','Causes connection latency.','SELECT CAST(ISNULL((SELECT TOP 1 is_auto_close_on FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS is_auto_close_on;',NULL,'is_auto_close_on == 1 ? "Critical" : "OK"','0','OFF','ALTER DATABASE SET AUTO_CLOSE OFF;',NULL,'exact','{"value":0}',7,'sqlserver',TRUE),

('TEMPDB_FILECOUNT_009','TempDB File Count','TempDB','TempDB','Critical','Main','Improper TempDB layout causes PAGELATCH waits.','SELECT CAST(COUNT(*) AS INT) AS file_count FROM sys.master_files WHERE database_id=2 AND type_desc=''ROWS'';',NULL,'file_count == 0 ? "Critical" : (file_count > 8 ? "Warning" : "OK")','4','4 or CPU/4','Manual TempDB reconfiguration required',NULL,'range','{"min":4,"max":8}',8,'sqlserver',TRUE),

('BACKUP_LOG_010','Log Backup Recent','Backup','Database','Critical','Main','Missing log backups break point-in-time recovery.','SELECT CAST(ISNULL(DATEDIFF(MINUTE, (SELECT MAX(backup_finish_date) FROM msdb.dbo.backupset WHERE type = ''L''), GETDATE()), 9999) AS INT) AS minutes_since_log_backup;',NULL,'minutes_since_log_backup > 60 ? "Critical" : (minutes_since_log_backup > 15 ? "Warning" : "OK")','15','< 15 minutes','Create SQL Agent log backup job',NULL,'threshold','{"max":15}',9,'sqlserver',TRUE),

('QUERY_STORE_011','Query Store Enabled','Monitoring','Database','Critical','Main','Query Store provides performance insights.','SELECT CAST(ISNULL((SELECT TOP 1 is_query_store_on FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS is_query_store_on;',NULL,'is_query_store_on == 1 ? "OK" : "Warning"','1','ON','ALTER DATABASE SET QUERY_STORE ON;',NULL,'exact','{"value":1}',10,'sqlserver',TRUE);

-- Insert PostgreSQL Rules
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_SHARED_BUFFERS_001','Shared Buffers','Memory','Instance','Critical','Main','PostgreSQL shared_buffers at default 128MB is too low.','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''shared_buffers'';','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''shared_buffers'';','setting < 131072 ? "Critical" : (setting < 262144 ? "Warning" : "OK")','262144','256MB','ALTER SYSTEM SET shared_buffers = ''256MB'';','ALTER SYSTEM SET shared_buffers = ''256MB'';','threshold','{"min":262144}',1,'postgres',TRUE),

('PG_MAX_CONNECTIONS_001','Max Connections','Connection','Instance','Warning','BestPractice','High connections consume memory.','SELECT setting::int AS setting FROM pg_settings WHERE name = ''max_connections'';','SELECT setting::int AS setting FROM pg_settings WHERE name = ''max_connections'';','setting > 500 ? "Warning" : "OK"','100','100','ALTER SYSTEM SET max_connections = ''100'';','ALTER SYSTEM SET max_connections = ''100'';','threshold','{"max":500}',2,'postgres',TRUE),

('PG_WORK_MEM_001','Work Mem','Memory','Instance','Warning','BestPractice','Complex sorts spill to disk at default 4MB.','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''work_mem'';','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''work_mem'';','setting < 256 ? "Warning" : "OK"','256','256MB','ALTER SYSTEM SET work_mem = ''256MB'';','ALTER SYSTEM SET work_mem = ''256MB'';','threshold','{"min":256}',3,'postgres',TRUE),

('PG_MAINT_WORK_MEM_001','Maintenance Work Mem','Maintenance','Instance','Warning','BestPractice','VACUUM and CREATE INDEX are slow at default 64MB.','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''maintenance_work_mem'';','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''maintenance_work_mem'';','setting < 512 ? "Warning" : "OK"','512','512MB','ALTER SYSTEM SET maintenance_work_mem = ''512MB'';','ALTER SYSTEM SET maintenance_work_mem = ''512MB'';','threshold','{"min":512}',4,'postgres',TRUE),

('PG_RANDOM_PAGE_COST_001','Random Page Cost','Performance','Instance','Warning','BestPractice','Default 4.0 is for HDD, not SSD.','SELECT setting::float8 AS setting FROM pg_settings WHERE name = ''random_page_cost'';','SELECT setting::float8 AS setting FROM pg_settings WHERE name = ''random_page_cost'';','setting > 1.5 ? "Warning" : "OK"','1.1','1.1 (SSD)','ALTER SYSTEM SET random_page_cost = ''1.1'';','ALTER SYSTEM SET random_page_cost = ''1.1'';','threshold','{"max":1.1}',5,'postgres',TRUE),

('PG_EFFECTIVE_CACHE_SIZE_001','Effective Cache Size','Performance','Instance','Warning','BestPractice','Default 4GB is too low for modern servers.','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''effective_cache_size'';','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''effective_cache_size'';','setting < 8388608 ? "Warning" : "OK"','8388608','8GB','ALTER SYSTEM SET effective_cache_size = ''8GB'';','ALTER SYSTEM SET effective_cache_size = ''8GB'';','threshold','{"min":8388608}',6,'postgres',TRUE),

('PG_CHKPT_COMPLETE_TARGET_001','Checkpoint Completion Target','Performance','Instance','Warning','BestPractice','Low target causes I/O spikes.','SELECT setting::float8 AS setting FROM pg_settings WHERE name = ''checkpoint_completion_target'';','SELECT setting::float8 AS setting FROM pg_settings WHERE name = ''checkpoint_completion_target'';','setting < 0.9 ? "Warning" : "OK"','0.9','0.9','ALTER SYSTEM SET checkpoint_completion_target = ''0.9'';','ALTER SYSTEM SET checkpoint_completion_target = ''0.9'';','threshold','{"min":0.9}',7,'postgres',TRUE),

('PG_WAL_BUFFERS_001','WAL Buffers','Performance','Instance','Warning','BestPractice','Low WAL buffers causes frequent flushes.','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''wal_buffers'';','SELECT setting::bigint AS setting FROM pg_settings WHERE name = ''wal_buffers'';','setting < 16 ? "Warning" : "OK"','16','16MB','ALTER SYSTEM SET wal_buffers = ''16MB'';','ALTER SYSTEM SET wal_buffers = ''16MB'';','threshold','{"min":16}',8,'postgres',TRUE),

('PG_DEAD_TUPLES_001','Dead Tuple Ratio','Maintenance','Database','Critical','Main','High dead tuples indicate autovacuum issues.','SELECT n_dead_tup, n_live_tup, CASE WHEN n_live_tup > 0 THEN (n_dead_tup::float / n_live_tup) * 100 ELSE 0 END AS dead_pct FROM pg_stat_user_tables WHERE n_dead_tup > 1000 ORDER BY n_dead_tup DESC LIMIT 1;','SELECT n_dead_tup, n_live_tup, CASE WHEN n_live_tup > 0 THEN (n_dead_tup::float / n_live_tup) * 100 ELSE 0 END AS dead_pct FROM pg_stat_user_tables WHERE n_dead_tup > 1000 ORDER BY n_dead_tup DESC LIMIT 1;','dead_pct > 20 ? "Critical" : (dead_pct > 10 ? "Warning" : "OK")','10','< 10%','VACUUM ANALYZE;',NULL,'threshold','{"max":10}',9,'postgres',TRUE),

('PG_REPL_LAG_001','Replication Lag','High Availability','Instance','Critical','Main','Standby falling behind risks data loss.','SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) AS lag_bytes FROM pg_stat_replication LIMIT 1;','SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) AS lag_bytes FROM pg_stat_replication LIMIT 1;','lag_bytes > 52428800 ? "Critical" : (lag_bytes > 10485760 ? "Warning" : "OK")','0','0 bytes','Check network/IO on standby',NULL,'threshold','{"max":0}',10,'postgres',TRUE),

('PG_STAT_STMTS_001','pg_stat_statements','Monitoring','Instance','Critical','BestPractice','Extension needed for query analysis.','SELECT COUNT(*) AS cnt FROM pg_extension WHERE extname = ''pg_stat_statements'';','SELECT COUNT(*) AS cnt FROM pg_extension WHERE extname = ''pg_stat_statements'';','cnt > 0 ? "OK" : "Critical"','1','Installed','CREATE EXTENSION pg_stat_statements;',NULL,'exact','{"value":1}',11,'postgres',TRUE),

('PG_IDLE_TX_001','Idle in Transaction','Connection','Instance','Critical','Main','Long idle transactions block VACUUM.','SELECT COUNT(*) AS cnt FROM pg_stat_activity WHERE state = ''idle in transaction'' AND state_change < current_timestamp - interval ''5 minutes'';','SELECT COUNT(*) AS cnt FROM pg_stat_activity WHERE state = ''idle in transaction'' AND state_change < current_timestamp - interval ''5 minutes'';','cnt > 0 ? "Warning" : "OK"','0','0','Set idle_in_transaction_session_timeout',NULL,'threshold','{"max":0}',12,'postgres',TRUE);

-- ================================================================================
-- SECTION 5: ADDITIONAL RULES (from 007_additional_rules_fix.sql)
-- ================================================================================

-- SQL Server Additional Rules
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_CPU_PRIORITYBOOST_002','Priority Boost Enabled','Instance Config','Instance','Critical','Main','Priority Boost can destabilize Windows scheduler.','SELECT CAST(ISNULL((SELECT value_in_use FROM sys.configurations WHERE name = ''priority boost''), 0) AS INT) AS priority_boost;',NULL,'priority_boost == 0 ? "OK" : "Critical"','0','Disabled (0)','EXEC sp_configure ''priority boost'',0; RECONFIGURE;',NULL,'exact','{"value":0}',11,'sqlserver',TRUE),

('INST_LIGHTWEIGHT_POOLING_003','Lightweight Pooling Enabled','Instance Config','Instance','Critical','Main','Fiber mode breaks modern schedulers and parallelism.','SELECT CAST(ISNULL((SELECT value_in_use FROM sys.configurations WHERE name = ''lightweight pooling''), 0) AS INT) AS lightweight_pooling;',NULL,'lightweight_pooling == 0 ? "OK" : "Critical"','0','Disabled (0)','EXEC sp_configure ''lightweight pooling'',0; RECONFIGURE;',NULL,'exact','{"value":0}',12,'sqlserver',TRUE),

('INST_LPIM_004','Lock Pages in Memory','Memory','Instance','Warning','BestPractice','Prevents OS from paging SQL memory.','SELECT sql_memory_model_desc FROM sys.dm_os_sys_info;',NULL,'sql_memory_model_desc == ''LOCK_PAGES'' ? "OK" : "Warning"','LOCK_PAGES','LPIM enabled','Grant LPIM privilege to SQL Service Account',NULL,'exact','{"value":"LOCK_PAGES"}',13,'sqlserver',TRUE),

('INST_TRACEFLAGS_005','Important Trace Flags','Instance Config','Instance','Warning','BestPractice','Recommended trace flags should be enabled.','SELECT CAST(value AS INT) AS trace_flag FROM (SELECT 1117 AS value UNION SELECT 1118 UNION SELECT 3226 UNION SELECT 4199) t;',NULL,'COUNT >= 4 ? "OK" : "Warning"','4','1117,1118,3226,4199','Enable trace flags via startup parameters',NULL,'threshold','{"min":4}',14,'sqlserver',TRUE),

('DB_PAGE_VERIFY_006','Page Verify CHECKSUM','Database','Database','Critical','Main','Checksum detects corruption early.','SELECT CAST(ISNULL((SELECT TOP 1 page_verify_option FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS page_verify;',NULL,'page_verify == 2 ? "OK" : "Critical"','2','CHECKSUM','ALTER DATABASE SET PAGE_VERIFY CHECKSUM;',NULL,'exact','{"value":2}',15,'sqlserver',TRUE),

('DB_TRUSTWORTHY_007','Trustworthy Enabled','Security','Database','Critical','Main','Security vulnerability allowing privilege escalation.','SELECT CAST(ISNULL((SELECT TOP 1 is_trustworthy_on FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS is_trustworthy;',NULL,'is_trustworthy == 0 ? "OK" : "Critical"','0','OFF','ALTER DATABASE SET TRUSTWORTHY OFF;',NULL,'exact','{"value":0}',16,'sqlserver',TRUE),

('DB_CHAINING_008','Cross DB Ownership Chaining','Security','Database','Critical','Main','Security risk allowing cross-db privilege escalation.','SELECT CAST(ISNULL((SELECT TOP 1 is_db_chaining_on FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS is_chaining;',NULL,'is_chaining == 0 ? "OK" : "Critical"','0','Disabled','ALTER DATABASE SET DB_CHAINING OFF;',NULL,'exact','{"value":0}',17,'sqlserver',TRUE),

('AGENT_FAILED_JOBS_009','Failed Jobs Last 24h','SQL Agent','Instance','Critical','Main','Recent job failures detected.','SELECT CAST(ISNULL((SELECT TOP 1 run_status FROM msdb.dbo.sysjobhistory WHERE step_id = 0 AND run_date >= CONVERT(int,FORMAT(GETDATE()-1,''yyyyMMdd'')) ORDER BY run_date DESC), 1) AS INT) AS failed_status;',NULL,'failed_status == 0 ? "Warning" : "OK"','0','No failed jobs','Investigate failing SQL Agent jobs',NULL,'threshold','{"max":0}',18,'sqlserver',TRUE),

('AGENT_DISABLED_JOBS_010','Disabled Jobs Found','SQL Agent','Instance','Warning','BestPractice','Disabled jobs may indicate broken maintenance.','SELECT CAST(ISNULL((SELECT COUNT(*) FROM msdb.dbo.sysjobs WHERE enabled = 0), 0) AS INT) AS disabled_count;',NULL,'disabled_count == 0 ? "OK" : "Warning"','0','No disabled jobs','Review and enable required jobs',NULL,'threshold','{"max":0}',19,'sqlserver',TRUE),

('AGENT_JOB_OWNER_011','Jobs Not Owned by SA','Security','Instance','Warning','BestPractice','Job ownership by non-SA accounts can cause failures.','SELECT CAST(ISNULL((SELECT COUNT(*) FROM msdb.dbo.sysjobs WHERE owner_sid <> 0x01), 0) AS INT) AS non_sa_owner_count;',NULL,'non_sa_owner_count == 0 ? "OK" : "Warning"','0','All jobs owned by SA','Change job owner to sa',NULL,'threshold','{"max":0}',20,'sqlserver',TRUE),

('BACKUP_ENCRYPTION_012','Backup Encryption','Backup','Instance','Warning','BestPractice','Backups should be encrypted for compliance.','SELECT CAST(ISNULL((SELECT COUNT(*) FROM msdb.dbo.backupset WHERE backup_finish_date > GETDATE()-30 AND encryptor_type IS NOT NULL), 0) AS INT) AS encrypted_count;',NULL,'encrypted_count > 0 ? "OK" : "Warning"','1','Encrypted backups','Enable backup encryption',NULL,'threshold','{"min":1}',21,'sqlserver',TRUE),

('BACKUP_RETENTION_013','Backup Retention','Backup','Instance','Warning','BestPractice','Backups older than retention should exist.','SELECT CAST(ISNULL(DATEDIFF(DAY, (SELECT TOP 1 backup_finish_date FROM msdb.dbo.backupset ORDER BY backup_finish_date DESC), GETDATE()), 999) AS INT) AS days_since_backup;',NULL,'days_since_backup < 30 ? "OK" : "Warning"','30','14-30 days','Adjust backup retention policy',NULL,'threshold','{"max":30}',22,'sqlserver',TRUE),

('PERF_MEMORY_GRANTS_014','Memory Grants Pending','Performance','Instance','Critical','Main','Memory pressure detected.','SELECT CAST(ISNULL((SELECT TOP 1 cntr_value FROM sys.dm_os_performance_counters WHERE counter_name = ''Memory Grants Pending'' AND object_name LIKE ''%Buffer Manager%''), 0) AS BIGINT) AS memory_grants;',NULL,'memory_grants == 0 ? "OK" : (memory_grants < 10 ? "Warning" : "Critical")','0','0','Investigate memory pressure',NULL,'threshold','{"max":0}',23,'sqlserver',TRUE),

('PERF_BLOCKING_015','Long Blocking','Performance','Instance','Critical','Main','Blocking chains detected.','SELECT CAST(ISNULL((SELECT COUNT(*) FROM sys.dm_exec_requests WHERE blocking_session_id <> 0), 0) AS INT) AS blocking_count;',NULL,'blocking_count == 0 ? "OK" : "Critical"','0','No blocking','Investigate blocking queries',NULL,'threshold','{"max":0}',24,'sqlserver',TRUE),

('PERF_SINGLE_USE_PLANS_016','Plan Cache Bloat','Performance','Instance','Warning','BestPractice','Too many single-use plans.','SELECT CAST(ISNULL((SELECT TOP 1 (a.cntr_value * 1.0 / NULLIF(b.cntr_value, 0)) * 100 FROM sys.dm_os_performance_counters a JOIN sys.dm_os_performance_counters b ON a.object_name = b.object_name WHERE a.counter_name = ''Cached Plans'' AND a.instance_name = ''SQL Plans'' AND b.counter_name = ''Total Plans''), 0) AS FLOAT) AS single_use_pct;',NULL,'single_use_pct < 30 ? "OK" : "Warning"','30','<30%','Enable optimize for adhoc workloads',NULL,'threshold','{"max":30}',25,'sqlserver',TRUE);

-- PostgreSQL Additional Rules
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_AUTOVACUUM_017','Autovacuum Enabled','Maintenance','Instance','Critical','Main','Autovacuum prevents table bloat.','SELECT setting AS autovacuum FROM pg_settings WHERE name = ''autovacuum'';','SELECT setting AS autovacuum FROM pg_settings WHERE name = ''autovacuum'';','autovacuum == ''on'' ? "OK" : "Critical"','on','ON','ALTER SYSTEM SET autovacuum = on;','ALTER SYSTEM SET autovacuum = on;','exact','{"value":"on"}',13,'postgres',TRUE),

('PG_IDLE_TX_018','Idle in Transaction','Connection','Instance','Critical','Main','Idle transactions cause bloat and lock retention.','SELECT COUNT(*) AS idle_tx_count FROM pg_stat_activity WHERE state = ''idle in transaction'';','SELECT COUNT(*) AS idle_tx_count FROM pg_stat_activity WHERE state = ''idle in transaction'';','idle_tx_count == 0 ? "OK" : "Critical"','0','0 sessions','Set idle_in_transaction_session_timeout',NULL,'threshold','{"max":0}',14,'postgres',TRUE),

('PG_REPLICATION_LAG_019','Replication Lag','High Availability','Instance','Critical','Main','Replica lag detected.','SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) AS lag_bytes FROM pg_stat_replication LIMIT 1;','SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) AS lag_bytes FROM pg_stat_replication LIMIT 1;','lag_bytes == 0 ? "OK" : (lag_bytes < 10485760 ? "Warning" : "Critical")','0','<30 seconds','Investigate replication',NULL,'threshold','{"max":0}',15,'postgres',TRUE),

('PG_LONG_TX_020','Long Running Transactions','Performance','Instance','Warning','BestPractice','Long transactions cause bloat.','SELECT COUNT(*) AS long_tx_count FROM pg_stat_activity WHERE state != ''idle'' AND now() - xact_start > interval ''5 minutes'';','SELECT COUNT(*) AS long_tx_count FROM pg_stat_activity WHERE state != ''idle'' AND now() - xact_start > interval ''5 minutes'';','long_tx_count == 0 ? "OK" : "Warning"','0','No long transactions','Investigate long running transactions',NULL,'threshold','{"max":0}',16,'postgres',TRUE);

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