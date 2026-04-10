-- Metric: mssql_job_summary
-- Source: backend/internal/repository/mssql_jobs.go:28
-- Target Table: sqlserver_job_metrics (TimescaleDB)
-- Description: Gets SQL Agent job summary counts (total, enabled, disabled)

SELECT 
    COUNT(*) AS TotalJobs,
    SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END) AS EnabledJobs,
    SUM(CASE WHEN enabled = 0 THEN 1 ELSE 0 END) AS DisabledJobs
FROM msdb.dbo.sysjobs WITH (NOLOCK);
