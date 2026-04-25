-- Migration: 00014_fix_missing_relations.sql
-- Description: Create legacy compatibility views and missing relations.
-- Author: Ravi Sharma

CREATE SCHEMA IF NOT EXISTS monitor;

-- system_stats_detail view for PgDiskSpaceEvaluator
-- Maps to sqlserver_metrics for SQL Server and returns zeros for Postgres (until host agent disk metrics are unified)
CREATE OR REPLACE VIEW system_stats_detail AS
SELECT 
    capture_timestamp,
    server_instance_name,
    data_disk_mb / 1024.0 AS disk_total_gb,
    (data_disk_mb - free_disk_mb) / 1024.0 AS disk_used_gb
FROM sqlserver_metrics
UNION ALL
SELECT
    now() as capture_timestamp,
    name as server_instance_name,
    0.0 as disk_total_gb,
    0.0 as disk_used_gb
FROM optima_servers
WHERE db_type = 'postgres';

-- postgres_lock_stats compatibility view
-- Points to the modern monitor.pg_lock_snapshot
CREATE OR REPLACE VIEW postgres_lock_stats AS
SELECT 
    collected_at AS capture_timestamp,
    (SELECT name FROM optima_servers WHERE id::text = server_id LIMIT 1) AS server_instance_name,
    pid,
    locktype,
    mode,
    granted,
    relation_name,
    waiting_seconds
FROM monitor.pg_lock_snapshot;
