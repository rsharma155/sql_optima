-- Metric: mssql_job_schedules
-- Source: backend/internal/repository/mssql_jobs.go:104
-- Target Table: N/A (job scheduling)
-- Description: Returns next run schedules for all jobs

SELECT 
    ISNULL(j.name, 'Unknown'),
    ISNULL(s.next_run_date, 0),
    ISNULL(s.next_run_time, 0)
FROM msdb.dbo.sysjobs j WITH (NOLOCK)
JOIN msdb.dbo.sysjobschedules s WITH (NOLOCK) ON j.job_id = s.job_id;
