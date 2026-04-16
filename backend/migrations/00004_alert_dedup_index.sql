-- ============================================================================
-- SQL Optima: Alert Engine – fingerprint dedup index
-- Purpose: Adds a partial unique index on fingerprint for non-resolved alerts
--          so that INSERT … ON CONFLICT can deduplicate by fingerprint.
-- Version: 1.0.0
-- Last Updated: 2026-04-16
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS idx_alerts_open_fingerprint
    ON optima_alerts (fingerprint)
    WHERE status IN ('open', 'acknowledged');

-- +goose Down
DROP INDEX IF EXISTS idx_alerts_open_fingerprint;
