-- +goose Up
-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Add database_name to mssql_watched_queries and update unique constraint.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

ALTER TABLE mssql_watched_queries ADD COLUMN IF NOT EXISTS database_name TEXT;

-- Update existing rows to have a default database if they were added before this column
UPDATE mssql_watched_queries SET database_name = 'master' WHERE database_name IS NULL;

-- Make database_name NOT NULL for the new constraint
ALTER TABLE mssql_watched_queries ALTER COLUMN database_name SET DEFAULT 'master';
ALTER TABLE mssql_watched_queries ALTER COLUMN database_name SET NOT NULL;

-- Drop old constraint and add new one that includes database_name
ALTER TABLE mssql_watched_queries DROP CONSTRAINT IF EXISTS mssql_watched_queries_server_instance_name_query_hash_key;
ALTER TABLE mssql_watched_queries ADD CONSTRAINT mssql_watched_queries_scoped_unique UNIQUE (server_instance_name, database_name, query_hash);

-- +goose Down
ALTER TABLE mssql_watched_queries DROP CONSTRAINT IF EXISTS mssql_watched_queries_scoped_unique;
ALTER TABLE mssql_watched_queries ADD CONSTRAINT mssql_watched_queries_server_instance_name_query_hash_key UNIQUE (server_instance_name, query_hash);
ALTER TABLE mssql_watched_queries DROP COLUMN IF EXISTS database_name;
