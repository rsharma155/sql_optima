-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Enhanced schema for SQL Server Watched Query Analyzer — 
--          stores historical wait stats and un-truncated execution plans.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

ALTER TABLE mssql_watched_query_snapshots ADD COLUMN IF NOT EXISTS wait_stats JSONB;
ALTER TABLE mssql_watched_query_snapshots ALTER COLUMN query_plan TYPE TEXT;
