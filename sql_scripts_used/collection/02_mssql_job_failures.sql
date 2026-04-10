-- Metric: mssql_job_failures
-- Source: backend/internal/repository/mssql_jobs.go:124
-- Target Table: N/A (job failure history)
-- Description: Returns last 100 job step failures with error messages

SELECT TOP 100
    ISNULL(j.name, 'Unknown'),
    ISNULL(h.step_name, 'Unknown'),
    ISNULL(SUBSTRING(h.message, 1, 300), 'No Trace'),
    ISNULL(h.run_date, 0),
    ISNULL(h.run_time, 0)
FROM msdb.dbo.sysjobhistory h WITH (NOLOCK)
JOIN msdb.dbo.sysjobs j WITH (NOLOCK) ON h.job_id = j.job_id
WHERE h.run_status = 0 AND h.step_id > 0
ORDER BY h.run_date DESC, h.run_time DESC;
