-- Metric: sqlserver_job_summary
-- Source: backend/internal/repository/sqlserver_jobs.go:28
-- Target Table: sqlserver_job_metrics (TimescaleDB)
-- Description: Gets SQL Agent job summary counts (total, enabled, disabled)

SELECT /* SQL_OPTIMA */   
    COUNT(*) AS TotalJobs,
    SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END) AS EnabledJobs,
    SUM(CASE WHEN enabled = 0 THEN 1 ELSE 0 END) AS DisabledJobs
FROM msdb.dbo.sysjobs WITH (NOLOCK);
