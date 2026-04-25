-- +goose Up
-- +goose StatementBegin
-- Create schema if not exists (though it usually should exist by now)
CREATE SCHEMA IF NOT EXISTS ruleengine;

-- SQL Server Ignore Rules
CREATE TABLE IF NOT EXISTS ruleengine.sqlserver_ignore_rules(
 rule_type text,
 rule_value text,
 PRIMARY KEY(rule_type,rule_value)
);

INSERT INTO ruleengine.sqlserver_ignore_rules VALUES
('database','master'),
('database','model'),
('database','msdb'),
('database','tempdb'),
('program','SQLAgent%'),
('program','MonitoringCollector%'),
('login','NT AUTHORITY\SYSTEM'),
('login','sa')
ON CONFLICT DO NOTHING;

-- PostgreSQL Ignore Rules
CREATE TABLE IF NOT EXISTS ruleengine.pg_ignore_rules(
 rule_type text,
 rule_value text,
 PRIMARY KEY(rule_type,rule_value)
);

INSERT INTO ruleengine.pg_ignore_rules VALUES
('database','postgres'),
('database','template0'),
('database','template1'),
('application','monitoring_collector')
ON CONFLICT DO NOTHING;

-- SQL Server Query Metrics V2
CREATE TABLE IF NOT EXISTS sqlserver_query_metrics_v2(
 ts timestamptz NOT NULL,
 instance_id text,
 database_name text,
 login_name text,
 application_name text,
 query_hash bigint,
 plan_hash bigint,
 total_executions bigint,
 total_cpu_ms bigint,
 total_elapsed_ms bigint,
 total_logical_reads bigint,
 total_physical_reads bigint,
 total_rows bigint,
 statement_text text
);
SELECT create_hypertable('sqlserver_query_metrics_v2','ts',if_not_exists=>TRUE);
CREATE INDEX IF NOT EXISTS idx_sqlserver_query_metrics_v2_instance_ts ON sqlserver_query_metrics_v2 (instance_id, ts DESC);

-- PostgreSQL Query Metrics V2
CREATE TABLE IF NOT EXISTS pg_query_metrics_v2(
 ts timestamptz NOT NULL,
 instance_id text,
 datname text,
 usename text,
 application_name text,
 queryid bigint,
 calls bigint,
 total_exec_time double precision,
 rows bigint,
 shared_blks_hit bigint,
 shared_blks_read bigint,
 temp_blks_written bigint
);
SELECT create_hypertable('pg_query_metrics_v2','ts',if_not_exists=>TRUE);
CREATE INDEX IF NOT EXISTS idx_pg_query_metrics_v2_instance_ts ON pg_query_metrics_v2 (instance_id, ts DESC);

-- Seed new collectors
INSERT INTO optima_collector_configs (collector_name, module, frequency_seconds) VALUES
('sqlserver_queries_v2', 'SQLSERVER', 15),
('pg_queries_v2', 'Postgres', 15)
ON CONFLICT (collector_name) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS pg_query_metrics_v2;
DROP TABLE IF EXISTS sqlserver_query_metrics_v2;
DROP TABLE IF EXISTS ruleengine.pg_ignore_rules;
DROP TABLE IF EXISTS ruleengine.sqlserver_ignore_rules;
-- +goose StatementEnd
