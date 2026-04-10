-- Metric: mssql_running_jobs_count
-- Source: backend/internal/repository/mssql_jobs.go:43
-- Target Table: N/A (job monitoring)
-- Description: Counts currently running SQL Agent jobs

SELECT COUNT(*) FROM msdb.dbo.sysjobactivity WITH (NOLOCK) WHERE start_execution_date IS NOT NULL AND stop_execution_date IS NULL;
