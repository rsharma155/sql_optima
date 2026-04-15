-- Metric: mssql_job_list
-- Source: backend/internal/repository/mssql_jobs.go:63
-- Target Table: N/A (job monitoring)
-- Description: Returns full job list with current status, last run info, and owner

SELECT 
    ISNULL(j.name, 'Unknown') AS JobName,
    CAST(j.enabled AS BIT),
    ISNULL(SUSER_SNAME(j.owner_sid), 'Unknown') AS Owner,
    ISNULL(CONVERT(VARCHAR, j.date_created, 120), '') AS date_created,
    CASE 
        WHEN ja.start_execution_date IS NOT NULL AND ja.stop_execution_date IS NULL THEN 'Running'
        ELSE 'Idle'
    END AS CurrentStatus,
    ISNULL(h.run_date, 0),
    ISNULL(h.run_time, 0),
    CASE ISNULL(h.run_status, -1)
        WHEN 0 THEN 'Failed'
        WHEN 1 THEN 'Succeeded'
        WHEN 2 THEN 'Retry'
        WHEN 3 THEN 'Canceled'
        ELSE 'Unknown'
    END AS LastRunStatus
FROM msdb.dbo.sysjobs j WITH (NOLOCK)
LEFT JOIN (
    SELECT job_id, MAX(session_id) as session_id FROM msdb.dbo.sysjobactivity WITH (NOLOCK) GROUP BY job_id
) max_ja ON j.job_id = max_ja.job_id
LEFT JOIN msdb.dbo.sysjobactivity ja WITH (NOLOCK) ON max_ja.job_id = ja.job_id AND max_ja.session_id = ja.session_id
LEFT JOIN (
    SELECT job_id, MAX(instance_id) AS instance_id FROM msdb.dbo.sysjobhistory WITH (NOLOCK) WHERE step_id = 0 GROUP BY job_id
) max_h ON j.job_id = max_h.job_id
LEFT JOIN msdb.dbo.sysjobhistory h WITH (NOLOCK) ON max_h.instance_id = h.instance_id;
