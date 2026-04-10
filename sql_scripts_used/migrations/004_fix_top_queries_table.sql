-- Fix sqlserver_top_queries table - add missing columns
ALTER TABLE sqlserver_top_queries ADD COLUMN IF NOT EXISTS logical_reads BIGINT DEFAULT 0;
ALTER TABLE sqlserver_top_queries ADD COLUMN IF NOT EXISTS execution_count BIGINT DEFAULT 0;
