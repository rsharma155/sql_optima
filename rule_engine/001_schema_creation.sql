/*
RULE ENGINE SCHEMA DESIGN FOR POSTGRESQL
Supports both SQL Server and PostgreSQL targets
*/

-- 1. Create Schema with existence check
CREATE SCHEMA IF NOT EXISTS ruleengine;

-- 2. Create Master Rule Table
CREATE TABLE IF NOT EXISTS ruleengine.rules
(
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

-- 3. Insert SQL Server Rules
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_MEM_MAX_001','Max Server Memory Configuration','Instance Config','Instance','Critical','Main','SQL Server consuming all RAM causes OS starvation.','SELECT total_physical_memory_kb/1024/1024 AS TotalRAM_GB, value_in_use AS MaxServerMemoryMB FROM sys.configurations CROSS JOIN sys.dm_os_sys_memory WHERE name=''max server memory (MB)'';',NULL,'MaxServerMemoryMB == 2147483647 ? ''Critical'' : (MaxServerMemoryMB < Recommended ? ''Warning'' : ''OK'')','(TotalRAM_GB <= 16 ? TotalRAM_GB - 4 : (TotalRAM_GB <= 32 ? TotalRAM_GB - 6 : (TotalRAM_GB <= 64 ? TotalRAM_GB - 8 : (TotalRAM_GB <= 128 ? TotalRAM_GB - 12 : (TotalRAM_GB <= 256 ? TotalRAM_GB - 16 : TotalRAM_GB - 32))))) * 1024','TotalRAM - OS Reserve','EXEC sp_configure ''max server memory (MB)'', <RecommendedMB>; RECONFIGURE;',NULL,'threshold','{"unit":"MB"}',1,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_CPU_MAXDOP_003','MAXDOP Setting','Parallelism','Instance','Critical','Main','Wrong MAXDOP causes CXPACKET storms.','SELECT cpu_count,(SELECT value_in_use FROM sys.configurations WHERE name=''max degree of parallelism'') AS MAXDOP FROM sys.dm_os_sys_info;',NULL,'MAXDOP = MIN(8, CPUs per NUMA)',NULL,'<=8 CPUs else 8','EXEC sp_configure ''max degree of parallelism'', <Value>; RECONFIGURE;',NULL,'range','{"min":1,"max":8}',2,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_CPU_CTFP_004','Cost Threshold','Parallelism','Instance','Critical','Main','Default value 5 is obsolete.','SELECT value_in_use FROM sys.configurations WHERE name=''cost threshold for parallelism'';',NULL,'Critical if < 40',NULL,'40-70','EXEC sp_configure ''cost threshold for parallelism'',50; RECONFIGURE;',NULL,'threshold','{"min":40}',3,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_PLAN_CACHE_005','Optimize for AdHoc Off','Plan Cache','Instance','Critical','Main','Prevents plan cache bloat.','SELECT value_in_use FROM sys.configurations WHERE name=''optimize for ad hoc workloads'';',NULL,'Must be ON',NULL,'1','EXEC sp_configure ''optimize for ad hoc workloads'',1; RECONFIGURE;',NULL,'exact','{"value":1}',4,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('INST_IFI_006','Instant File Initialization','Storage','OS','Critical','Main','Database growth freezes server without IFI.','SELECT instant_file_initialization_enabled FROM sys.dm_server_services;',NULL,'Must be enabled',NULL,'Windows privilege required','Grant Perform Volume Maintenance Tasks to SQL Service Account',NULL,'exact','{"value":"enabled"}',5,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('DB_AUTOSHRINK_007','Auto Shrink Enabled','Database Config','Database','Critical','Main','Causes fragmentation.','SELECT name FROM sys.databases WHERE is_auto_shrink_on=1 AND database_id>4;',NULL,'Must be OFF',NULL,'OFF','ALTER DATABASE <DB> SET AUTO_SHRINK OFF;',NULL,'exact','{"value":0}',6,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('DB_AUTOCLOSE_008','Auto Close Enabled','Database Config','Database','Critical','Main','Causes connection latency.','SELECT name FROM sys.databases WHERE is_auto_close_on=1 AND database_id>4;',NULL,'Must be OFF',NULL,'OFF','ALTER DATABASE <DB> SET AUTO_CLOSE OFF;',NULL,'exact','{"value":0}',7,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('TEMPDB_FILECOUNT_009','TempDB File Count Incorrect','TempDB','TempDB','Critical','Main','Improper TempDB layout causes PAGELATCH waits.','SELECT COUNT(*) FROM sys.master_files WHERE database_id=2 AND type_desc=''ROWS'';',NULL,'1 file per 4 CPU cores (max 8)',NULL,'CPU/4','Manual TempDB reconfiguration required',NULL,'range','{"max":8}',8,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('BACKUP_LOG_010','Missing Log Backup','Backup','Database','Critical','Main','Breaks point-in-time recovery.','SELECT name FROM msdb.dbo.backupset WHERE type=''L'' AND backup_finish_date > DATEADD(HOUR, -24, GETDATE());',NULL,'Must exist last 24h',NULL,'15 min RPO','Create SQL Agent log backup job',NULL,'threshold','{"hours":24}',9,'sqlserver',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('QUERY_STORE_011','Query Store Disabled','Monitoring','Database','Critical','Main','No performance regression tracking.','SELECT name FROM sys.databases WHERE is_query_store_on=0 AND database_id>4;',NULL,'Must be ON',NULL,'ON','ALTER DATABASE <DB> SET QUERY_STORE ON;',NULL,'exact','{"value":1}',10,'sqlserver',TRUE);

-- 4. Insert PostgreSQL Rules
INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_SHARED_BUFFERS_001','Shared Buffers Not Tuned','Memory','Instance','Critical','Main','PostgreSQL shared_buffers at default 128MB is too low.','SELECT name, setting, boot_val FROM pg_settings WHERE name = ''shared_buffers'';','SELECT name, setting, boot_val FROM pg_settings WHERE name = ''shared_buffers'';','Set to 25% of RAM',NULL,'25% of RAM','ALTER SYSTEM SET shared_buffers = ''<value>'';','ALTER SYSTEM SET shared_buffers = ''<value>'';','threshold','{"unit":"GB","percent":25}',1,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_MAX_CONNECTIONS_001','High Max Connections','Connection','Instance','Warning','BestPractice','PostgreSQL process-per-connection model is heavy.','SELECT name, setting FROM pg_settings WHERE name = ''max_connections'';','SELECT name, setting FROM pg_settings WHERE name = ''max_connections'';','Lower if > 500, use PgBouncer',NULL,'100-500','ALTER SYSTEM SET max_connections = ''100'';','ALTER SYSTEM SET max_connections = ''100'';','threshold','{"max":500}',2,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_WORK_MEM_001','Work Mem Not Tuned','Memory','Instance','Warning','BestPractice','Complex sorts spill to disk at default 4MB.','SELECT name, setting FROM pg_settings WHERE name = ''work_mem'';','SELECT name, setting FROM pg_settings WHERE name = ''work_mem'';','Set based on RAM/max_connections',NULL,'4MB per 256MB RAM','ALTER SYSTEM SET work_mem = ''256MB'';','ALTER SYSTEM SET work_mem = ''256MB'';','threshold','{"unit":"MB","min":256}',3,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_MAINT_WORK_MEM_001','Maintenance Work Mem','Maintenance','Instance','Warning','BestPractice','VACUUM and CREATE INDEX are slow at default 64MB.','SELECT name, setting FROM pg_settings WHERE name = ''maintenance_work_mem'';','SELECT name, setting FROM pg_settings WHERE name = ''maintenance_work_mem'';','Set to 512MB-1GB',NULL,'512MB','ALTER SYSTEM SET maintenance_work_mem = ''512MB'';','ALTER SYSTEM SET maintenance_work_mem = ''512MB'';','threshold','{"unit":"MB","min":512}',4,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_RANDOM_PAGE_COST_001','Random Page Cost High','Performance','Instance','Warning','BestPractice','Default 4.0 is for HDD, not SSD.','SELECT name, setting FROM pg_settings WHERE name = ''random_page_cost'';','SELECT name, setting FROM pg_settings WHERE name = ''random_page_cost'';','Set to 1.1 for SSD',NULL,'1.1','ALTER SYSTEM SET random_page_cost = ''1.1'';','ALTER SYSTEM SET random_page_cost = ''1.1'';','threshold','{"max":1.1}',5,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_EFFECTIVE_CACHE_SIZE_001','Effective Cache Size Default','Performance','Instance','Warning','BestPractice','Default 4GB is too low for modern servers.','SELECT name, setting FROM pg_settings WHERE name = ''effective_cache_size'';','SELECT name, setting FROM pg_settings WHERE name = ''effective_cache_size'';','Set to 75% of RAM',NULL,'75% of RAM','ALTER SYSTEM SET effective_cache_size = ''8GB'';','ALTER SYSTEM SET effective_cache_size = ''8GB'';','threshold','{"unit":"GB","percent":75}',6,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_CHKPT_COMPLETE_TARGET_001','Checkpoint Target Low','Performance','Instance','Warning','BestPractice','Low target causes I/O spikes.','SELECT name, setting FROM pg_settings WHERE name = ''checkpoint_completion_target'';','SELECT name, setting FROM pg_settings WHERE name = ''checkpoint_completion_target'';','Set to 0.9',NULL,'0.9','ALTER SYSTEM SET checkpoint_completion_target = ''0.9'';','ALTER SYSTEM SET checkpoint_completion_target = ''0.9'';','threshold','{"min":0.9}',7,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_WAL_BUFFERS_001','WAL Buffers Setting','Performance','Instance','Warning','BestPractice','Low WAL buffers causes frequent flushes.','SELECT name, setting FROM pg_settings WHERE name = ''wal_buffers'';','SELECT name, setting FROM pg_settings WHERE name = ''wal_buffers'';','Set to 16MB',NULL,'16MB','ALTER SYSTEM SET wal_buffers = ''16MB'';','ALTER SYSTEM SET wal_buffers = ''16MB'';','threshold','{"unit":"MB","min":16}',8,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_DEAD_TUPLES_001','High Dead Tuple Ratio','Maintenance','Database','Critical','Main','Autovacuum is failing to keep up, causing table bloat and slow sequential scans.','SELECT relname, n_dead_tup, n_live_tup, (n_dead_tup::float / NULLIF(n_live_tup, 0)) * 100 AS dead_tuple_pct FROM pg_stat_user_tables WHERE n_dead_tup > 10000 AND (n_dead_tup::float / NULLIF(n_live_tup, 0)) > 0.2;','SELECT relname, n_dead_tup, n_live_tup, (n_dead_tup::float / NULLIF(n_live_tup, 0)) * 100 AS dead_tuple_pct FROM pg_stat_user_tables WHERE n_dead_tup > 10000 AND (n_dead_tup::float / NULLIF(n_live_tup, 0)) > 0.2;','Critical if dead tuples > 20%',NULL,'< 20%','VACUUM ANALYZE <table_name>; (And tune autovacuum parameters)','VACUUM ANALYZE <table_name>; (And tune autovacuum parameters)','threshold','{"max":20}',9,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_REPL_LAG_001','Replication Lag','High Availability','Instance','Critical','Main','Standby nodes are falling behind the primary, risking data loss on failover.','SELECT application_name, state, sync_state, pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes FROM pg_stat_replication;','SELECT application_name, state, sync_state, pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes FROM pg_stat_replication;','Critical if lag > 50MB (varies by workload)',NULL,'0 lag (or minimal)','Investigate network bandwidth or standby disk I/O','Investigate network bandwidth or standby disk I/O','threshold','{"unit":"MB","max":50}',10,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_STAT_STMTS_001','pg_stat_statements Extension','Monitoring','Instance','Critical','BestPractice','Without pg_stat_statements, historical query performance and bottlenecks cannot be tracked.','SELECT extname FROM pg_extension WHERE extname = ''pg_stat_statements'';','SELECT extname FROM pg_extension WHERE extname = ''pg_stat_statements'';','Must be installed',NULL,'Installed','CREATE EXTENSION pg_stat_statements; (Ensure it is in shared_preload_libraries)','CREATE EXTENSION pg_stat_statements; (Ensure it is in shared_preload_libraries)','exact','{"value":"installed"}',11,'postgres',TRUE);

INSERT INTO ruleengine.rules (rule_id, rule_name, category, applies_to, severity, dashboard_placement, description, detection_sql, detection_sql_pg, evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg, comparison_type, threshold_value, priority, target_db_type, is_enabled) VALUES
('PG_IDLE_TX_001','Idle in Transaction','Connection','Instance','Critical','Main','Sessions left open mid-transaction block VACUUM, causing severe database bloat.','SELECT count(*) FROM pg_stat_activity WHERE state = ''idle in transaction'' AND state_change < current_timestamp - interval ''15 minutes'';','SELECT count(*) FROM pg_stat_activity WHERE state = ''idle in transaction'' AND state_change < current_timestamp - interval ''15 minutes'';','Must be 0 for transactions > 15m',NULL,'0','SELECT pg_terminate_backend(<pid>); (And configure idle_in_transaction_session_timeout)','SELECT pg_terminate_backend(<pid>); (And configure idle_in_transaction_session_timeout)','threshold','{"max":0}',12,'postgres',TRUE);