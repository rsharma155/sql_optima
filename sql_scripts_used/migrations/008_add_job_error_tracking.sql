-- Migration: 008_add_job_error_tracking.sql
-- Description: Adds error_message column to sqlserver_job_metrics table for tracking permission/collection errors
-- Date: 2026-04-05

-- Add error_message column to sqlserver_job_metrics if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'sqlserver_job_metrics' 
        AND column_name = 'error_message'
    ) THEN
        ALTER TABLE sqlserver_job_metrics ADD COLUMN error_message TEXT;
        RAISE NOTICE 'Added error_message column to sqlserver_job_metrics';
    ELSE
        RAISE NOTICE 'error_message column already exists in sqlserver_job_metrics';
    END IF;
END $$;

-- Add unique constraint to prevent duplicate entries
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'sqlserver_job_metrics_unique'
    ) THEN
        ALTER TABLE sqlserver_job_metrics 
        ADD CONSTRAINT sqlserver_job_metrics_unique 
        UNIQUE (capture_timestamp, server_instance_name);
        RAISE NOTICE 'Added unique constraint to sqlserver_job_metrics';
    END IF;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Constraint may already exist: %', SQLERRM;
END $$;

COMMENT ON COLUMN sqlserver_job_metrics.error_message IS 'Stores error message if job collection failed (e.g., permission denied on msdb tables)';
