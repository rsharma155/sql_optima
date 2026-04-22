-- Migration: 00010_sqlserver_watched_query_waits.sql
-- Description: Add wait stats and improve plan storage for watched queries.
-- Author: Ravi Sharma

ALTER TABLE sqlserver_watched_query_snapshots ADD COLUMN IF NOT EXISTS wait_stats JSONB;
ALTER TABLE sqlserver_watched_query_snapshots ALTER COLUMN query_plan TYPE TEXT; -- Ensure it can hold large XML
