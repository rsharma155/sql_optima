-- Update SQL Server Rules with proper evaluation_logic expressions
-- These need to be valid expr-lang expressions

-- INST_MEM_MAX_001 - Already has valid expression
-- INST_CPU_MAXDOP_003 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'MAXDOP <= 0 ? "Critical" : (MAXDOP <= 8 ? "OK" : "Warning")',
    expected_calc = 'MIN(cpu_count, 8)'
WHERE rule_id = 'INST_CPU_MAXDOP_003';

-- INST_CPU_CTFP_004 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'value_in_use < 40 ? "Critical" : (value_in_use < 50 ? "Warning" : "OK")',
    expected_calc = '50'
WHERE rule_id = 'INST_CPU_CTFP_004';

-- INST_PLAN_CACHE_005 - Fix evaluation_logic  
UPDATE ruleengine.rules SET 
    evaluation_logic = 'value_in_use == 1 ? "OK" : "Critical"',
    expected_calc = '1'
WHERE rule_id = 'INST_PLAN_CACHE_005';

-- INST_IFI_006 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'instant_file_initialization_enabled == true ? "OK" : "Critical"',
    expected_calc = '1'
WHERE rule_id = 'INST_IFI_006';

-- DB_AUTOSHRINK_007 - Fix - query returns list, need different approach
UPDATE ruleengine.rules SET 
    detection_sql = 'SELECT TOP 1 is_auto_shrink_on FROM sys.databases WHERE database_id = DB_ID();',
    evaluation_logic = 'is_auto_shrink_on == 0 ? "OK" : "Critical"',
    expected_calc = '0'
WHERE rule_id = 'DB_AUTOSHRINK_007';

-- DB_AUTOCLOSE_008 - Fix - query returns list
UPDATE ruleengine.rules SET 
    detection_sql = 'SELECT TOP 1 is_auto_close_on FROM sys.databases WHERE database_id = DB_ID();',
    evaluation_logic = 'is_auto_close_on == 0 ? "OK" : "Critical"',
    expected_calc = '0'
WHERE rule_id = 'DB_AUTOCLOSE_008';

-- TEMPDB_FILECOUNT_009 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'COUNT <= 8 ? "OK" : (COUNT <= 16 ? "Warning" : "Critical")',
    expected_calc = 'MIN(8, cpu_count / 4)'
WHERE rule_id = 'TEMPDB_FILECOUNT_009';

-- BACKUP_LOG_010 - Fix - query returns list, need different approach
UPDATE ruleengine.rules SET 
    detection_sql = 'SELECT ISNULL(MAX(DATEDIFF(MINUTE, backup_finish_date, GETDATE())), 9999) AS minutes_since_last_log FROM msdb.dbo.backupset WHERE type = ''L'' AND database_name = DB_NAME();',
    evaluation_logic = 'minutes_since_last_log < 60 ? "OK" : (minutes_since_last_log < 1440 ? "Warning" : "Critical")',
    expected_calc = '15'
WHERE rule_id = 'BACKUP_LOG_010';

-- QUERY_STORE_011 - Fix - query returns list
UPDATE ruleengine.rules SET 
    detection_sql = 'SELECT TOP 1 is_query_store_on FROM sys.databases WHERE database_id = DB_ID();',
    evaluation_logic = 'is_query_store_on == 1 ? "OK" : "Critical"',
    expected_calc = '1'
WHERE rule_id = 'QUERY_STORE_011';

-- PostgreSQL Rules

-- PG_SHARED_BUFFERS_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'setting_value < 131072 ? "Critical" : (setting_value < 262144 ? "Warning" : "OK")',
    expected_calc = 'totalRAM_GB * 256'
WHERE rule_id = 'PG_SHARED_BUFFERS_001';

-- PG_MAX_CONNECTIONS_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'setting > 500 ? "Warning" : (setting > 200 ? "OK" : "Warning")',
    expected_calc = '100'
WHERE rule_id = 'PG_MAX_CONNECTIONS_001';

-- PG_WORK_MEM_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'setting < 256 ? "Warning" : "OK"',
    expected_calc = '256'
WHERE rule_id = 'PG_WORK_MEM_001';

-- PG_MAINT_WORK_MEM_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'setting < 512 ? "Warning" : "OK"',
    expected_calc = '512'
WHERE rule_id = 'PG_MAINT_WORK_MEM_001';

-- PG_RANDOM_PAGE_COST_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'setting > 1.5 ? "Warning" : (setting > 1.1 ? "OK" : "Warning")',
    expected_calc = '1.1'
WHERE rule_id = 'PG_RANDOM_PAGE_COST_001';

-- PG_EFFECTIVE_CACHE_SIZE_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'setting < 8 ? "Warning" : "OK"',
    expected_calc = 'totalRAM_GB * 0.75'
WHERE rule_id = 'PG_EFFECTIVE_CACHE_SIZE_001';

-- PG_CHKPT_COMPLETE_TARGET_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'setting < 0.8 ? "Warning" : "OK"',
    expected_calc = '0.9'
WHERE rule_id = 'PG_CHKPT_COMPLETE_TARGET_001';

-- PG_WAL_BUFFERS_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'setting < 16 ? "Warning" : "OK"',
    expected_calc = '16'
WHERE rule_id = 'PG_WAL_BUFFERS_001';

-- PG_DEAD_TUPLES_001 - Fix evaluation_logic (this needs special handling)
UPDATE ruleengine.rules SET 
    evaluation_logic = 'dead_pct > 20 ? "Critical" : (dead_pct > 10 ? "Warning" : "OK")',
    expected_calc = '10'
WHERE rule_id = 'PG_DEAD_TUPLES_001';

-- PG_REPL_LAG_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'lag_bytes > 52428800 ? "Critical" : (lag_bytes > 10485760 ? "Warning" : "OK")',
    expected_calc = '0'
WHERE rule_id = 'PG_REPL_LAG_001';

-- PG_STAT_STMTS_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'extname == ''pg_stat_statements'' ? "OK" : "Critical"',
    expected_calc = '1'
WHERE rule_id = 'PG_STAT_STMTS_001';

-- PG_IDLE_TX_001 - Fix evaluation_logic
UPDATE ruleengine.rules SET 
    evaluation_logic = 'count > 0 ? "Critical" : "OK"',
    expected_calc = '0'
WHERE rule_id = 'PG_IDLE_TX_001';

-- Verify updates
SELECT rule_id, rule_name, evaluation_logic, expected_calc FROM ruleengine.rules;