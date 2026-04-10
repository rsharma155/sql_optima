-- Metric: ts_insert_sqlserver_job_metrics
-- Source: backend/internal/storage/hot/ts_logger.go:529
-- Target Table: sqlserver_job_metrics
-- Description: Inserts SQL Agent job summary metrics (total, enabled, disabled, running, failed, error_message)

INSERT INTO sqlserver_job_metrics (capture_timestamp, server_instance_name, total_jobs, enabled_jobs, disabled_jobs, running_jobs, failed_jobs_24h, error_message)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
