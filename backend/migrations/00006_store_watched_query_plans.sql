-- +goose Up
-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Store execution plans historically for watched queries to avoid live fetching.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

ALTER TABLE sqlserver_watched_query_snapshots ADD COLUMN IF NOT EXISTS query_plan TEXT;

-- +goose Down
ALTER TABLE sqlserver_watched_query_snapshots DROP COLUMN IF EXISTS query_plan;
