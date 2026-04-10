-- Metric: mssql_failed_jobs_24h
-- Source: backend/internal/repository/mssql_jobs.go:51
-- Target Table: N/A (job monitoring)
-- Description: Counts jobs that failed in the last 24 hours

SELECT COUNT(*) FROM msdb.dbo.sysjobhistory h WITH (NOLOCK)
JOIN msdb.dbo.sysjobs j WITH (NOLOCK) ON h.job_id = j.job_id
WHERE h.run_status = 0 AND h.run_date >= CAST(CONVERT(VARCHAR(8), GETDATE()-1, 112) AS INT) AND h.step_id = 0;
