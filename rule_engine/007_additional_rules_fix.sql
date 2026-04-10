/*
NEW RULES - Compatible with current rule engine schema
Run this after 006_complete_rules_fix.sql to add additional rules
*/

-- First add missing columns to existing table if not present
ALTER TABLE ruleengine.rules ADD COLUMN IF NOT EXISTS comparison_type VARCHAR(20) DEFAULT 'exact';
ALTER TABLE ruleengine.rules ADD COLUMN IF NOT EXISTS threshold_value JSONB;
ALTER TABLE ruleengine.rules ADD COLUMN IF NOT EXISTS priority INTEGER DEFAULT 0;

-- SQL SERVER RULES

-- INST_CPU_PRIORITYBOOST_002 - Priority Boost
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_CPU_PRIORITYBOOST_002','Priority Boost Enabled','Instance Config','Instance','Critical','Main','Priority Boost can destabilize Windows scheduler.','SELECT CAST(ISNULL((SELECT value_in_use FROM sys.configurations WHERE name = ''priority boost''), 0) AS INT) AS priority_boost;',NULL,'priority_boost == 0 ? "OK" : "Critical"','0','Disabled (0)','EXEC sp_configure ''priority boost'',0; RECONFIGURE;',NULL,'exact','{"value":0}',11,'sqlserver',TRUE);

-- INST_LIGHTWEIGHT_POOLING_003 - Lightweight Pooling
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_LIGHTWEIGHT_POOLING_003','Lightweight Pooling Enabled','Instance Config','Instance','Critical','Main','Fiber mode breaks modern schedulers and parallelism.','SELECT CAST(ISNULL((SELECT value_in_use FROM sys.configurations WHERE name = ''lightweight pooling''), 0) AS INT) AS lightweight_pooling;',NULL,'lightweight_pooling == 0 ? "OK" : "Critical"','0','Disabled (0)','EXEC sp_configure ''lightweight pooling'',0; RECONFIGURE;',NULL,'exact','{"value":0}',12,'sqlserver',TRUE);

-- INST_LPIM_004 - Lock Pages in Memory
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_LPIM_004','Lock Pages in Memory','Memory','Instance','Warning','BestPractice','Prevents OS from paging SQL memory.','SELECT sql_memory_model_desc FROM sys.dm_os_sys_info;',NULL,'sql_memory_model_desc == ''LOCK_PAGES'' ? "OK" : "Warning"','LOCK_PAGES','LPIM enabled','Grant LPIM privilege to SQL Service Account',NULL,'exact','{"value":"LOCK_PAGES"}',13,'sqlserver',TRUE);

-- INST_TRACEFLAGS_005 - Trace Flags
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_TRACEFLAGS_005','Important Trace Flags','Instance Config','Instance','Warning','BestPractice','Recommended trace flags should be enabled.','SELECT CAST(value AS INT) AS trace_flag FROM (SELECT 1117 AS value UNION SELECT 1118 UNION SELECT 3226 UNION SELECT 4199) t;',NULL,'COUNT >= 4 ? "OK" : "Warning"','4','1117,1118,3226,4199','Enable trace flags via startup parameters',NULL,'threshold','{"min":4}',14,'sqlserver',TRUE);

-- DB_PAGE_VERIFY_006 - Page Verify CHECKSUM
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('DB_PAGE_VERIFY_006','Page Verify CHECKSUM','Database','Database','Critical','Main','Checksum detects corruption early.','SELECT CAST(ISNULL((SELECT TOP 1 page_verify_option FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS page_verify;',NULL,'page_verify == 2 ? "OK" : "Critical"','2','CHECKSUM','ALTER DATABASE SET PAGE_VERIFY CHECKSUM;',NULL,'exact','{"value":2}',15,'sqlserver',TRUE);

-- DB_TRUSTWORTHY_007 - Trustworthy
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('DB_TRUSTWORTHY_007','Trustworthy Enabled','Security','Database','Critical','Main','Security vulnerability allowing privilege escalation.','SELECT CAST(ISNULL((SELECT TOP 1 is_trustworthy_on FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS is_trustworthy;',NULL,'is_trustworthy == 0 ? "OK" : "Critical"','0','OFF','ALTER DATABASE SET TRUSTWORTHY OFF;',NULL,'exact','{"value":0}',16,'sqlserver',TRUE);

-- DB_CHAINING_008 - Cross DB Ownership Chaining
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('DB_CHAINING_008','Cross DB Ownership Chaining','Security','Database','Critical','Main','Security risk allowing cross-db privilege escalation.','SELECT CAST(ISNULL((SELECT TOP 1 is_db_chaining_on FROM sys.databases WHERE database_id = DB_ID()), 0) AS INT) AS is_chaining;',NULL,'is_chaining == 0 ? "OK" : "Critical"','0','Disabled','ALTER DATABASE SET DB_CHAINING OFF;',NULL,'exact','{"value":0}',17,'sqlserver',TRUE);

-- AGENT_FAILED_JOBS_009 - Failed Jobs
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('AGENT_FAILED_JOBS_009','Failed Jobs Last 24h','SQL Agent','Instance','Critical','Main','Recent job failures detected.','SELECT CAST(ISNULL((SELECT TOP 1 run_status FROM msdb.dbo.sysjobhistory WHERE step_id = 0 AND run_date >= CONVERT(int,FORMAT(GETDATE()-1,''yyyyMMdd'')) ORDER BY run_date DESC), 1) AS INT) AS failed_status;',NULL,'failed_status == 0 ? "Warning" : "OK"','0','No failed jobs','Investigate failing SQL Agent jobs',NULL,'threshold','{"max":0}',18,'sqlserver',TRUE);

-- AGENT_DISABLED_JOBS_010 - Disabled Jobs
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('AGENT_DISABLED_JOBS_010','Disabled Jobs Found','SQL Agent','Instance','Warning','BestPractice','Disabled jobs may indicate broken maintenance.','SELECT CAST(ISNULL((SELECT COUNT(*) FROM msdb.dbo.sysjobs WHERE enabled = 0), 0) AS INT) AS disabled_count;',NULL,'disabled_count == 0 ? "OK" : "Warning"','0','No disabled jobs','Review and enable required jobs',NULL,'threshold','{"max":0}',19,'sqlserver',TRUE);

-- AGENT_JOB_OWNER_011 - Jobs Not Owned by SA
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('AGENT_JOB_OWNER_011','Jobs Not Owned by SA','Security','Instance','Warning','BestPractice','Job ownership by non-SA accounts can cause failures.','SELECT CAST(ISNULL((SELECT COUNT(*) FROM msdb.dbo.sysjobs WHERE owner_sid <> 0x01), 0) AS INT) AS non_sa_owner_count;',NULL,'non_sa_owner_count == 0 ? "OK" : "Warning"','0','All jobs owned by SA','Change job owner to sa',NULL,'threshold','{"max":0}',20,'sqlserver',TRUE);

-- BACKUP_ENCRYPTION_012 - Backup Encryption
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('BACKUP_ENCRYPTION_012','Backup Encryption','Backup','Instance','Warning','BestPractice','Backups should be encrypted for compliance.','SELECT CAST(ISNULL((SELECT COUNT(*) FROM msdb.dbo.backupset WHERE backup_finish_date > GETDATE()-30 AND encryptor_type IS NOT NULL), 0) AS INT) AS encrypted_count;',NULL,'encrypted_count > 0 ? "OK" : "Warning"','1','Encrypted backups','Enable backup encryption',NULL,'threshold','{"min":1}',21,'sqlserver',TRUE);

-- BACKUP_RETENTION_013 - Backup Retention
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('BACKUP_RETENTION_013','Backup Retention','Backup','Instance','Warning','BestPractice','Backups older than retention should exist.','SELECT CAST(ISNULL(DATEDIFF(DAY, (SELECT TOP 1 backup_finish_date FROM msdb.dbo.backupset ORDER BY backup_finish_date DESC), GETDATE()), 999) AS INT) AS days_since_backup;',NULL,'days_since_backup < 30 ? "OK" : "Warning"','30','14-30 days','Adjust backup retention policy',NULL,'threshold','{"max":30}',22,'sqlserver',TRUE);

-- PERF_MEMORY_GRANTS_014 - Memory Grants Pending
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PERF_MEMORY_GRANTS_014','Memory Grants Pending','Performance','Instance','Critical','Main','Memory pressure detected.','SELECT CAST(ISNULL((SELECT TOP 1 cntr_value FROM sys.dm_os_performance_counters WHERE counter_name = ''Memory Grants Pending'' AND object_name LIKE ''%Buffer Manager%''), 0) AS BIGINT) AS memory_grants;',NULL,'memory_grants == 0 ? "OK" : (memory_grants < 10 ? "Warning" : "Critical")','0','0','Investigate memory pressure',NULL,'threshold','{"max":0}',23,'sqlserver',TRUE);

-- PERF_BLOCKING_015 - Blocking
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PERF_BLOCKING_015','Long Blocking','Performance','Instance','Critical','Main','Blocking chains detected.','SELECT CAST(ISNULL((SELECT COUNT(*) FROM sys.dm_exec_requests WHERE blocking_session_id <> 0), 0) AS INT) AS blocking_count;',NULL,'blocking_count == 0 ? "OK" : "Critical"','0','No blocking','Investigate blocking queries',NULL,'threshold','{"max":0}',24,'sqlserver',TRUE);

-- PERF_SINGLE_USE_PLANS_016 - Plan Cache
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PERF_SINGLE_USE_PLANS_016','Plan Cache Bloat','Performance','Instance','Warning','BestPractice','Too many single-use plans.','SELECT CAST(ISNULL((SELECT TOP 1 (a.cntr_value * 1.0 / NULLIF(b.cntr_value, 0)) * 100 FROM sys.dm_os_performance_counters a JOIN sys.dm_os_performance_counters b ON a.object_name = b.object_name WHERE a.counter_name = ''Cached Plans'' AND a.instance_name = ''SQL Plans'' AND b.counter_name = ''Total Plans''), 0) AS FLOAT) AS single_use_pct;',NULL,'single_use_pct < 30 ? "OK" : "Warning"','30','<30%','Enable optimize for adhoc workloads',NULL,'threshold','{"max":30}',25,'sqlserver',TRUE);

-- POSTGRES RULES

-- PG_AUTOVACUUM_017 - Autovacuum
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_AUTOVACUUM_017','Autovacuum Enabled','Maintenance','Instance','Critical','Main','Autovacuum prevents table bloat.','SELECT setting AS autovacuum FROM pg_settings WHERE name = ''autovacuum'';','SELECT setting AS autovacuum FROM pg_settings WHERE name = ''autovacuum'';','autovacuum == ''on'' ? "OK" : "Critical"','on','ON','ALTER SYSTEM SET autovacuum = on;','ALTER SYSTEM SET autovacuum = on;','exact','{"value":"on"}',13,'postgres',TRUE);

-- PG_IDLE_TX_NEW - Idle in Transaction
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_IDLE_TX_018','Idle in Transaction','Connection','Instance','Critical','Main','Idle transactions cause bloat and lock retention.','SELECT COUNT(*) AS idle_tx_count FROM pg_stat_activity WHERE state = ''idle in transaction'';','SELECT COUNT(*) AS idle_tx_count FROM pg_stat_activity WHERE state = ''idle in transaction'';','idle_tx_count == 0 ? "OK" : "Critical"','0','0 sessions','Set idle_in_transaction_session_timeout',NULL,'threshold','{"max":0}',14,'postgres',TRUE);

-- PG_REPLICATION_LAG_NEW - Replication Lag
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_REPLICATION_LAG_019','Replication Lag','High Availability','Instance','Critical','Main','Replica lag detected.','SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) AS lag_bytes FROM pg_stat_replication LIMIT 1;','SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) AS lag_bytes FROM pg_stat_replication LIMIT 1;','lag_bytes == 0 ? "OK" : (lag_bytes < 10485760 ? "Warning" : "Critical")','0','<30 seconds','Investigate replication',NULL,'threshold','{"max":0}',15,'postgres',TRUE);

-- PG_LONG_TX_020 - Long Running Transactions
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_LONG_TX_020','Long Running Transactions','Performance','Instance','Warning','BestPractice','Long transactions cause bloat.','SELECT COUNT(*) AS long_tx_count FROM pg_stat_activity WHERE state != ''idle'' AND now() - xact_start > interval ''5 minutes'';','SELECT COUNT(*) AS long_tx_count FROM pg_stat_activity WHERE state != ''idle'' AND now() - xact_start > interval ''5 minutes'';','long_tx_count == 0 ? "OK" : "Warning"','0','No long transactions','Investigate long running transactions',NULL,'threshold','{"max":0}',16,'postgres',TRUE);

-- Verify the new rules
SELECT rule_id, rule_name, category, target_db_type, evaluation_logic, expected_calc 
FROM ruleengine.rules 
WHERE rule_id LIKE '%_002' OR rule_id LIKE '%_003' OR rule_id LIKE '%_004' OR rule_id LIKE '%_005' OR rule_id LIKE '%_006' OR rule_id LIKE '%_007' OR rule_id LIKE '%_008' OR rule_id LIKE '%_009' OR rule_id LIKE '%_010' OR rule_id LIKE '%_011' OR rule_id LIKE '%_012' OR rule_id LIKE '%_013' OR rule_id LIKE '%_014' OR rule_id LIKE '%_015' OR rule_id LIKE '%_016' OR rule_id LIKE '%_017' OR rule_id LIKE '%_018' OR rule_id LIKE '%_019' OR rule_id LIKE '%_020'
ORDER BY target_db_type, rule_id;