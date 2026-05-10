-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Enhancements for SQL Server AG Health monitoring.
--          Adds operational and connected state tracking for better diagnostics.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

DO $$
BEGIN
    -- Add operational_state to sqlserver_ag_health
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_ag_health' AND column_name='operational_state') THEN
        ALTER TABLE sqlserver_ag_health ADD COLUMN operational_state TEXT;
    END IF;

    -- Add connected_state to sqlserver_ag_health
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sqlserver_ag_health' AND column_name='connected_state') THEN
        ALTER TABLE sqlserver_ag_health ADD COLUMN connected_state TEXT;
    END IF;
END $$;
