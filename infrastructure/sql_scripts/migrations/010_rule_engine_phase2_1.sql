-- SQL Optima
--
-- Purpose: Roll forward SQL Server rule-engine metadata for Epic 2.1 first-batch coverage.
-- Safe for existing deployments that already use ruleengine.rules.

UPDATE ruleengine.rules
SET
    detection_sql = 'SELECT COUNT(*) AS affected_databases, MIN(page_verify_option) AS page_verify, STRING_AGG(name, '', '') AS sample_databases, STRING_AGG(page_verify_option_desc, '', '') AS page_verify_mode FROM sys.databases WHERE database_id > 4 AND source_database_id IS NULL AND page_verify_option <> 2;',
    evaluation_logic = 'affected_databases == 0 ? "OK" : "Critical"',
    expected_calc = '0',
    recommended_value = 'CHECKSUM',
    fix_script = 'ALTER DATABASE [db_name] SET PAGE_VERIFY CHECKSUM;'
WHERE rule_id = 'DB_PAGE_VERIFY_006';

UPDATE ruleengine.rules
SET
    detection_sql = 'SELECT COUNT(*) AS affected_databases, MAX(CAST(is_trustworthy_on AS INT)) AS is_trustworthy, STRING_AGG(name, '', '') AS sample_databases FROM sys.databases WHERE database_id > 4 AND source_database_id IS NULL AND is_trustworthy_on = 1;',
    evaluation_logic = 'affected_databases == 0 ? "OK" : "Critical"',
    expected_calc = '0',
    recommended_value = 'OFF',
    fix_script = 'ALTER DATABASE [db_name] SET TRUSTWORTHY OFF;'
WHERE rule_id = 'DB_TRUSTWORTHY_007';

UPDATE ruleengine.rules
SET
    detection_sql = 'SELECT COUNT(*) AS affected_databases, MAX(CAST(is_db_chaining_on AS INT)) AS is_chaining, STRING_AGG(name, '', '') AS sample_databases FROM sys.databases WHERE database_id > 4 AND source_database_id IS NULL AND is_db_chaining_on = 1;',
    evaluation_logic = 'affected_databases == 0 ? "OK" : "Critical"',
    expected_calc = '0',
    recommended_value = 'Disabled',
    fix_script = 'ALTER DATABASE [db_name] SET DB_CHAINING OFF;'
WHERE rule_id = 'DB_CHAINING_008';

UPDATE ruleengine.rules
SET
    detection_sql = 'SELECT COUNT(*) AS failing_jobs_24h, STRING_AGG(j.name, '', '') AS sample_jobs FROM msdb.dbo.sysjobhistory h WITH (NOLOCK) JOIN msdb.dbo.sysjobs j WITH (NOLOCK) ON h.job_id = j.job_id WHERE h.run_status = 0 AND h.run_date >= CAST(CONVERT(VARCHAR(8), DATEADD(DAY, -1, GETDATE()), 112) AS INT) AND h.step_id = 0;',
    evaluation_logic = 'failing_jobs_24h == 0 ? "OK" : (failing_jobs_24h < 3 ? "Warning" : "Critical")',
    expected_calc = '0',
    recommended_value = 'No failed jobs',
    fix_script = 'Investigate failing SQL Agent jobs and confirm recent schedule/owner changes.'
WHERE rule_id = 'AGENT_FAILED_JOBS_009';

UPDATE ruleengine.rules
SET
    detection_sql = 'SELECT COUNT(*) AS disabled_count, STRING_AGG(name, '', '') AS sample_jobs FROM msdb.dbo.sysjobs WITH (NOLOCK) WHERE enabled = 0;',
    evaluation_logic = 'disabled_count == 0 ? "OK" : "Warning"',
    expected_calc = '0',
    recommended_value = 'No disabled jobs',
    fix_script = 'Review disabled jobs and re-enable any maintenance or backup jobs that should still run.'
WHERE rule_id = 'AGENT_DISABLED_JOBS_010';

UPDATE ruleengine.rules
SET
    detection_sql = 'SELECT CAST(COALESCE(SUM(CASE WHEN usecounts = 1 AND cacheobjtype = ''Compiled Plan'' THEN 1 ELSE 0 END) * 100.0 / NULLIF(SUM(CASE WHEN cacheobjtype = ''Compiled Plan'' THEN 1 ELSE 0 END), 0), 0) AS FLOAT) AS single_use_pct, SUM(CASE WHEN usecounts = 1 AND cacheobjtype = ''Compiled Plan'' THEN 1 ELSE 0 END) AS single_use_plans, SUM(CASE WHEN cacheobjtype = ''Compiled Plan'' THEN 1 ELSE 0 END) AS total_compiled_plans FROM sys.dm_exec_cached_plans WITH (NOLOCK);',
    fix_script = 'Enable optimize for ad hoc workloads and review plan cache churn from one-off queries.'
WHERE rule_id = 'PERF_SINGLE_USE_PLANS_016';

-- HA_AG_REPLICA_017: new rule — AG Replica Health
-- Evaluates sys.dm_hadr_database_replica_states across all AGs on the instance.
-- Returns 0 unhealthy_replicas (OK) on instances where AGs are not configured.
INSERT INTO ruleengine.rules (
    rule_id, rule_name, category, applies_to, severity, dashboard_placement,
    description, detection_sql, detection_sql_pg,
    evaluation_logic, expected_calc, recommended_value, fix_script, fix_script_pg,
    comparison_type, threshold_value, priority, target_db_type, is_enabled
)
SELECT
    'HA_AG_REPLICA_017',
    'AG Replica Health',
    'High Availability',
    'Instance',
    'Critical',
    'Main',
    'Unhealthy Always On availability group replicas risk data loss and unplanned failover.',
    'SELECT ISNULL(COUNT(*), 0) AS unhealthy_replicas, ISNULL(STRING_AGG(ar.replica_server_name, '', ''), '''') AS sample_replicas, ISNULL(STRING_AGG(drs.synchronization_health_desc, '', ''), '''') AS health_states FROM sys.dm_hadr_database_replica_states drs WITH (NOLOCK) JOIN sys.availability_replicas ar WITH (NOLOCK) ON drs.replica_id = ar.replica_id WHERE drs.synchronization_health <> 2;',
    NULL,
    'unhealthy_replicas == 0 ? "OK" : (unhealthy_replicas < 2 ? "Warning" : "Critical")',
    '0',
    'All replicas HEALTHY',
    'Investigate replica synchronisation: check network, disk I/O, and SQL Server error log on each replica.',
    NULL,
    'threshold',
    '{"max":0}',
    26,
    'sqlserver',
    TRUE
WHERE NOT EXISTS (SELECT 1 FROM ruleengine.rules WHERE rule_id = 'HA_AG_REPLICA_017');
