-- ============================================================================
-- SQL Optima: Postgres Control Center Dashboards (Aggregations)
-- ============================================================================
-- Purpose: Provides materialized views and real-time aggregations for the 
--          v2 PostgreSQL Control Center observability dashboard.
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE SCHEMA IF NOT EXISTS monitor;
SET search_path TO monitor, public;

-- --------------------------------------------------------------------------
-- 1. Database Load (AAS Approximation View)
-- Supports the Hero "Database Pressure" Stacked Area Chart.
-- --------------------------------------------------------------------------
CREATE OR REPLACE VIEW monitor.pg_db_load_summary AS
SELECT 
    time_bucket('1 minute', ts) AS bucket,
    instance_id,
    ROUND(AVG(active_sessions), 2) AS avg_active_sessions,
    ROUND(AVG(cpu_sessions), 2) AS avg_cpu_sessions,
    ROUND(AVG(waiting_sessions), 2) AS avg_waiting_sessions,
    ROUND(AVG(idle_in_txn), 2) AS avg_idle_in_txn
FROM 
    monitor.pg_db_load_ts
WHERE 
    ts > NOW() - INTERVAL '24 hours'
GROUP BY 
    bucket, instance_id
ORDER BY 
    bucket DESC;

-- --------------------------------------------------------------------------
-- 2. Top Wait Categories Summary
-- Supports the "Wait Categories" Pie Chart
-- --------------------------------------------------------------------------
CREATE OR REPLACE VIEW monitor.pg_wait_categories_summary AS
SELECT 
    instance_id,
    CASE 
        WHEN wait_event_type = 'LWLock' THEN 'LWLock'
        WHEN wait_event_type = 'Lock' THEN 'Lock'
        WHEN wait_event_type LIKE 'IO%' THEN 'IO'
        WHEN wait_event_type IS NULL AND state = 'active' THEN 'CPU'
        ELSE 'Other'
    END AS wait_category,
    SUM(sessions) as total_wait_time_approx
FROM 
    monitor.pg_wait_event_summary_ts
WHERE 
    ts > NOW() - INTERVAL '1 hour'
GROUP BY 
    instance_id, wait_category;

-- --------------------------------------------------------------------------
-- 3. Top Wait Events Summary
-- Supports the "Top Wait Events" Bar Chart
-- --------------------------------------------------------------------------
CREATE OR REPLACE VIEW monitor.pg_top_wait_events AS
SELECT 
    instance_id,
    wait_event,
    SUM(sessions) as wait_occurrences
FROM 
    monitor.pg_wait_event_summary_ts
WHERE 
    ts > NOW() - INTERVAL '1 hour'
    AND wait_event IS NOT NULL
GROUP BY 
    instance_id, wait_event
ORDER BY 
    wait_occurrences DESC
LIMIT 10;

-- --------------------------------------------------------------------------
-- 4. Session State Trends
-- Supports the "Session States" Stacked Area Chart
-- --------------------------------------------------------------------------
CREATE OR REPLACE VIEW monitor.pg_session_states_trend AS
SELECT 
    time_bucket('1 minute', ts) AS bucket,
    instance_id,
    COUNT(*) FILTER (WHERE state = 'active') AS active_count,
    COUNT(*) FILTER (WHERE state = 'idle') AS idle_count,
    COUNT(*) FILTER (WHERE state = 'idle in transaction') AS idle_in_txn_count
FROM 
    monitor.pg_session_activity_ts
WHERE 
    ts > NOW() - INTERVAL '24 hours'
GROUP BY 
    bucket, instance_id
ORDER BY 
    bucket DESC;

-- Grant access
GRANT SELECT ON monitor.pg_db_load_summary TO PUBLIC;
GRANT SELECT ON monitor.pg_wait_categories_summary TO PUBLIC;
GRANT SELECT ON monitor.pg_top_wait_events TO PUBLIC;
GRANT SELECT ON monitor.pg_session_states_trend TO PUBLIC;
