/*
================================================================================
EXPRESSION ENGINE MIGRATION SCRIPT
Updates rules with standard aliases, expected_calc, and evaluation_logic.
================================================================================
*/

-- 1. MAXDOP Rule (Dynamic based on CPU Count)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT cpu_count, (SELECT CAST(value_in_use AS INT) FROM sys.configurations WHERE name=''max degree of parallelism'') AS MAXDOP FROM sys.dm_os_sys_info;',
    expected_calc = 'cpu_count <= 8 ? cpu_count : 8',
    evaluation_logic = 'MAXDOP == Recommended ? ''OK'' : ''Critical'''
WHERE rule_id = 'INST_CPU_MAXDOP_003';

-- 2. Cost Threshold for Parallelism (Static Threshold)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT CAST(value_in_use AS INT) AS CTFP FROM sys.configurations WHERE name=''cost threshold for parallelism'';',
    expected_calc = '50',
    evaluation_logic = 'CTFP < 40 ? ''Critical'' : (CTFP > 70 ? ''Warning'' : ''OK'')'
WHERE rule_id = 'INST_CPU_CTFP_004';

-- 3. Optimize for Ad Hoc Workloads
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT CAST(value_in_use AS INT) AS AdHocOn FROM sys.configurations WHERE name=''optimize for ad hoc workloads'';',
    expected_calc = '1',
    evaluation_logic = 'AdHocOn == Recommended ? ''OK'' : ''Critical'''
WHERE rule_id = 'INST_PLAN_CACHE_005';

-- 4. Instant File Initialization (Returns 'Y' or 'N')
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT instant_file_initialization_enabled AS IFI FROM sys.dm_server_services WHERE servicename LIKE ''SQL Server (%'';',
    expected_calc = '''Y''',
    evaluation_logic = 'IFI == ''Y'' ? ''OK'' : ''Critical'''
WHERE rule_id = 'INST_IFI_006';

-- 5. Auto Shrink Enabled (Count DBs violating the rule)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT COUNT(*) AS AutoShrinkDBs FROM sys.databases WHERE is_auto_shrink_on=1 AND database_id>4;',
    expected_calc = '0',
    evaluation_logic = 'AutoShrinkDBs == Recommended ? ''OK'' : ''Critical'''
WHERE rule_id = 'DB_AUTOSHRINK_007';

-- 6. Auto Close Enabled (Count DBs violating the rule)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT COUNT(*) AS AutoCloseDBs FROM sys.databases WHERE is_auto_close_on=1 AND database_id>4;',
    expected_calc = '0',
    evaluation_logic = 'AutoCloseDBs == Recommended ? ''OK'' : ''Critical'''
WHERE rule_id = 'DB_AUTOCLOSE_008';

-- 7. TempDB File Count (Dynamic based on CPU)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT cpu_count, (SELECT COUNT(*) FROM sys.master_files WHERE database_id=2 AND type_desc=''ROWS'') AS TempDBFiles FROM sys.dm_os_sys_info;',
    expected_calc = 'cpu_count <= 8 ? cpu_count : 8',
    evaluation_logic = 'TempDBFiles == Recommended ? ''OK'' : ''Warning'''
WHERE rule_id = 'TEMPDB_FILECOUNT_009';

-- 8. Missing Query Store (Count DBs violating the rule)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT COUNT(*) AS QueryStoreOffDBs FROM sys.databases WHERE is_query_store_on=0 AND database_id>4;',
    expected_calc = '0',
    evaluation_logic = 'QueryStoreOffDBs == Recommended ? ''OK'' : ''Critical'''
WHERE rule_id = 'QUERY_STORE_011';

/*
================================================================================
POSTGRESQL SPECIFIC RULES
================================================================================
*/

-- 9. PostgreSQL Max Connections
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT setting::int AS max_conn FROM pg_settings WHERE name = ''max_connections'';',
    expected_calc = '500',
    evaluation_logic = 'max_conn > Recommended ? ''Warning'' : ''OK'''
WHERE rule_id = 'PG_MAX_CONNECTIONS_001';

-- 10. PostgreSQL Random Page Cost (For SSDs)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT setting::float AS rpc FROM pg_settings WHERE name = ''random_page_cost'';',
    expected_calc = '1.1',
    evaluation_logic = 'rpc > 1.5 ? ''Warning'' : ''OK'''
WHERE rule_id = 'PG_RANDOM_PAGE_COST_001';

-- 11. PostgreSQL Dead Tuples (Autovacuum Health)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT COALESCE(MAX((n_dead_tup::float / NULLIF(n_live_tup, 0)) * 100), 0) AS max_dead_tuple_pct FROM pg_stat_user_tables WHERE n_dead_tup > 10000;',
    expected_calc = '20.0',
    evaluation_logic = 'max_dead_tuple_pct > Recommended ? ''Critical'' : (max_dead_tuple_pct > 10.0 ? ''Warning'' : ''OK'')'
WHERE rule_id = 'PG_DEAD_TUPLES_001';

-- 12. PostgreSQL Idle in Transaction (Stuck sessions)
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT count(*) AS idle_tx_count FROM pg_stat_activity WHERE state = ''idle in transaction'' AND state_change < current_timestamp - interval ''15 minutes'';',
    expected_calc = '0',
    evaluation_logic = 'idle_tx_count > Recommended ? ''Critical'' : ''OK'''
WHERE rule_id = 'PG_IDLE_TX_001';

-- 13. PostgreSQL Checkpoint Completion Target
UPDATE ruleengine.rules SET
    detection_sql = 'SELECT setting::float AS chkpt_target FROM pg_settings WHERE name = ''checkpoint_completion_target'';',
    expected_calc = '0.9',
    evaluation_logic = 'chkpt_target < Recommended ? ''Warning'' : ''OK'''
WHERE rule_id = 'PG_CHKPT_COMPLETE_TARGET_001';