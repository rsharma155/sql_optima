-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Migration: 00011_collector_configs.sql
-- Purpose: Store dynamic configuration for metric collector frequencies.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

CREATE TABLE IF NOT EXISTS optima_collector_configs (
    id SERIAL PRIMARY KEY,
    collector_name VARCHAR(100) UNIQUE NOT NULL,
    module VARCHAR(100) NOT NULL,
    frequency_seconds INTEGER NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by VARCHAR(100)
);

-- Seed with initial data from CSV
INSERT INTO optima_collector_configs (collector_name, module, frequency_seconds) VALUES
('Postgres Active Queries', 'Postgres', 15),
('Postgres Blocking Locks', 'Postgres', 15),
('Postgres CPU and Memory', 'Postgres', 60),
('Postgres Wait Stats', 'Postgres', 60),
('Postgres Storage I/O', 'Postgres', 60),
('Postgres Long Running Queries', 'Postgres', 60),
('Postgres Query Stats', 'Postgres', 60),
('SQL Server Active Queries', 'SQLSERVER', 15),
('SQL Server Blocking Locks', 'SQLSERVER', 15),
('SQL Server CPU and Memory', 'SQLSERVER', 60),
('SQL Server Wait Stats', 'SQLSERVER', 60),
('SQL Server Storage I/O', 'SQLSERVER', 60),
('SQL Server Long Running Queries', 'SQLSERVER', 60),
('SQL Server Query Store', 'SQLSERVER', 900),
('SQL Server Procedure Stats', 'SQLSERVER', 120),
('SQL Server Memory Clerks', 'SQLSERVER', 300),
('SQL Server Plan Cache', 'SQLSERVER', 300),
('SQL Server Database Size', 'SQLSERVER', 3600),
('SQL Server Configuration', 'SQLSERVER', 86400)
ON CONFLICT (collector_name) DO UPDATE SET frequency_seconds = EXCLUDED.frequency_seconds;

COMMENT ON TABLE optima_collector_configs IS 'Persistent storage for metric collector execution frequencies controlled by administrators.';
