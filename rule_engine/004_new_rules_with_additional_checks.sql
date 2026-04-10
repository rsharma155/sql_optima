--SQL SERVER — INSTANCE & SECURITY
INSERT INTO ruleengine.rules (
rule_id, rule_name, category, applies_to, severity, dashboard_placement,
description, detection_sql, expected_calc, evaluation_logic, recommended_value,
fix_script, target_db_type, is_enabled)
VALUES

-- Priority Boost
('INST_CPU_PRIORITYBOOST_002','Priority Boost Enabled','Instance Config','Instance','Critical','Main',
'Priority Boost can destabilize Windows scheduler.',
'SELECT CAST(value_in_use AS INT) AS priority_boost FROM sys.configurations WHERE name = ''priority boost'';',
'0',
'priority_boost == 0 ? ''OK'' : ''Critical''',
'Disabled (0)',
'EXEC sp_configure ''priority boost'',0; RECONFIGURE;',
'sqlserver',TRUE),

-- Lightweight pooling
('INST_LIGHTWEIGHT_POOLING_003','Lightweight Pooling Enabled','Instance Config','Instance','Critical','Main',
'Fiber mode breaks modern schedulers and parallelism.',
'SELECT CAST(value_in_use AS INT) AS lightweight_pooling FROM sys.configurations WHERE name = ''lightweight pooling'';',
'0',
'lightweight_pooling == 0 ? ''OK'' : ''Critical''',
'Disabled (0)',
'EXEC sp_configure ''lightweight pooling'',0; RECONFIGURE;',
'sqlserver',TRUE),

-- Lock pages in memory
('INST_LPIM_004','Lock Pages in Memory Missing','Memory','Instance','Warning','BestPractice',
'Prevents OS from paging SQL memory.',
'SELECT sql_memory_model_desc FROM sys.dm_os_sys_info;',
'LOCK_PAGES',
'sql_memory_model_desc LIKE ''%LOCK_PAGES%'' ? ''OK'' : ''Warning''',
'LPIM enabled',
'Grant LPIM privilege to SQL Service Account',
'sqlserver',TRUE),

-- Trace flags baseline
('INST_TRACEFLAGS_005','Important Trace Flags Missing','Instance Config','Instance','Warning','BestPractice',
'Recommended optimizer and tempdb trace flags missing.',
'DBCC TRACESTATUS(-1);',
'1117,1118,3226,4199',
'Expected flags present ? ''OK'' : ''Warning''',
'1117,1118,3226,4199 enabled',
'Enable via startup parameters',
'sqlserver',TRUE);

--DATABASE SECURITY & SAFETY
INSERT INTO ruleengine.rules VALUES

('DB_PAGE_VERIFY_006','Page Verify Not CHECKSUM','Database','Database','Critical','Main',
'Checksum detects corruption early.',
'SELECT name,page_verify_option_desc FROM sys.databases WHERE database_id>4;',
'CHECKSUM',
'page_verify_option_desc==''CHECKSUM'' ? ''OK'' : ''Critical''',
'CHECKSUM',
'ALTER DATABASE <db> SET PAGE_VERIFY CHECKSUM;',
'sqlserver',TRUE),

('DB_TRUSTWORTHY_007','Trustworthy Enabled','Security','Database','Critical','Main',
'Security vulnerability allowing privilege escalation.',
'SELECT name FROM sys.databases WHERE is_trustworthy_on=1 AND database_id>4;',
'0 rows',
'rows==0 ? ''OK'' : ''Critical''',
'OFF',
'ALTER DATABASE <db> SET TRUSTWORTHY OFF;',
'sqlserver',TRUE),

('DB_CHAINING_008','Cross DB Ownership Chaining Enabled','Security','Database','Critical','Main',
'Security risk allowing cross-db privilege escalation.',
'SELECT name FROM sys.databases WHERE is_db_chaining_on=1;',
'0 rows',
'rows==0 ? ''OK'' : ''Critical''',
'Disabled',
'ALTER DATABASE <db> SET DB_CHAINING OFF;',
'sqlserver',TRUE);

--SQL AGENT RELIABILITY
INSERT INTO ruleengine.rules VALUES

('AGENT_FAILED_JOBS_009','Failed Jobs Last 24h','SQL Agent','Instance','Critical','Main',
'Recent job failures detected.',
'SELECT COUNT(*) AS failed_jobs FROM msdb.dbo.sysjobhistory WHERE run_status=0 AND run_date>=CONVERT(int,FORMAT(GETDATE()-1,''yyyyMMdd''));',
'0',
'failed_jobs==0 ? ''OK'' : ''Critical''',
'No failed jobs',
'Investigate failing SQL Agent jobs',
'sqlserver',TRUE),

('AGENT_DISABLED_JOBS_010','Disabled Jobs Found','SQL Agent','Instance','Warning','BestPractice',
'Disabled jobs may indicate broken maintenance.',
'SELECT COUNT(*) FROM msdb.dbo.sysjobs WHERE enabled=0;',
'0',
'count==0 ? ''OK'' : ''Warning''',
'No disabled jobs',
'Review and enable required jobs',
'sqlserver',TRUE),

('AGENT_JOB_OWNER_011','Jobs Not Owned by SA','Security','Instance','Warning','BestPractice',
'Job ownership by non-SA accounts can cause failures.',
'SELECT COUNT(*) FROM msdb.dbo.sysjobs WHERE SUSER_SNAME(owner_sid)<>''sa'';',
'0',
'count==0 ? ''OK'' : ''Warning''',
'All jobs owned by SA',
'EXEC msdb.dbo.sp_update_job @job_name=?,@owner_login_name=''sa'';',
'sqlserver',TRUE);

--BACKUP & DR ADVANCED
INSERT INTO ruleengine.rules VALUES

('BACKUP_ENCRYPTION_012','Backup Encryption Missing','Backup','Instance','Warning','BestPractice',
'Backups should be encrypted for compliance.',
'SELECT COUNT(*) FROM msdb.dbo.backupset WHERE encryptor_type IS NOT NULL;',
'>0',
'count>0 ? ''OK'' : ''Warning''',
'Encrypted backups enabled',
'Enable backup encryption',
'sqlserver',TRUE),

('BACKUP_RETENTION_013','Backup Retention Too Low','Backup','Instance','Warning','BestPractice',
'Backups older than retention missing.',
'SELECT MIN(backup_finish_date) FROM msdb.dbo.backupset;',
'>14 days',
'older_than_14days ? ''OK'' : ''Warning''',
'14–30 days retention',
'Adjust backup retention policy',
'sqlserver',TRUE);

--PERFORMANCE BASELINE RULES
INSERT INTO ruleengine.rules VALUES

('PERF_MEMORY_GRANTS_014','Memory Grants Pending','Performance','Instance','Critical','Main',
'Memory pressure detected.',
'SELECT cntr_value FROM sys.dm_os_performance_counters WHERE counter_name=''Memory Grants Pending'';',
'0',
'value==0 ? ''OK'' : ''Critical''',
'0',
'Investigate memory pressure',
'sqlserver',TRUE),

('PERF_BLOCKING_015','Long Blocking Detected','Performance','Instance','Critical','Main',
'Blocking chains detected.',
'SELECT COUNT(*) FROM sys.dm_exec_requests WHERE blocking_session_id<>0;',
'0',
'value==0 ? ''OK'' : ''Critical''',
'No blocking',
'Investigate blocking queries',
'sqlserver',TRUE),

('PERF_SINGLE_USE_PLANS_016','Plan Cache Bloat','Performance','Instance','Warning','BestPractice',
'Too many single-use plans.',
'SELECT COUNT(*) FROM sys.dm_exec_cached_plans WHERE usecounts=1;',
'<30%',
'value<threshold ? ''OK'' : ''Warning''',
'<30%',
'Enable optimize for adhoc workloads',
'sqlserver',TRUE);

/* POSTGRES RULES (CRITICAL ADDITIONS) */


INSERT INTO ruleengine.rules VALUES

('PG_AUTOVACUUM_017','Autovacuum Disabled','Maintenance','Instance','Critical','Main',
'Autovacuum prevents table bloat.',
'SHOW autovacuum;',
'on',
'result==''on'' ? ''OK'' : ''Critical''',
'ON',
'ALTER SYSTEM SET autovacuum = on;',
'postgres',TRUE),

('PG_IDLE_TX_018','Idle in Transaction Sessions','Performance','Instance','Critical','Main',
'Idle transactions cause bloat and lock retention.',
'SELECT COUNT(*) FROM pg_stat_activity WHERE state=''idle in transaction'';',
'0',
'value==0 ? ''OK'' : ''Critical''',
'0 sessions',
'Terminate idle transactions',
'postgres',TRUE),

('PG_REPLICATION_LAG_019','Replication Lag','HA','Instance','Critical','Main',
'Replica lag detected.',
'SELECT EXTRACT(EPOCH FROM now()-pg_last_xact_replay_timestamp());',
'<30 sec',
'value<30 ? ''OK'' : ''Critical''',
'<30 seconds',
'Investigate replication',
'postgres',TRUE),

('PG_LONG_TX_020','Long Running Transactions','Performance','Instance','Warning','BestPractice',
'Long transactions cause bloat.',
'SELECT COUNT(*) FROM pg_stat_activity WHERE now()-xact_start > interval ''15 minutes'';',
'0',
'value==0 ? ''OK'' : ''Warning''',
'No long transactions',
'Investigate long running transactions',
'postgres',TRUE);