-- +goose Up
-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Add database_name to postgres_table_maintenance_stats for multi-DB visibility.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

ALTER TABLE postgres_table_maintenance_stats ADD COLUMN IF NOT EXISTS database_name TEXT;

-- Update existing rows to default if unknown
UPDATE postgres_table_maintenance_stats SET database_name = 'postgres' WHERE database_name IS NULL;

-- Update compression and indexes to include database_name for better query performance
-- Note: We don't drop the hypertable, just add the index.
CREATE INDEX IF NOT EXISTS idx_pg_table_maint_db ON postgres_table_maintenance_stats (server_instance_name, database_name, capture_timestamp DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_pg_table_maint_db;
ALTER TABLE postgres_table_maintenance_stats DROP COLUMN IF EXISTS database_name;
