-- ============================================================================
-- SQL Optima: Alert Engine Schema
-- Purpose: Tables for alert lifecycle, audit history, maintenance windows,
--          and built-in alert rules with seed data.
-- Version: 1.0.0
-- Last Updated: 2026-04-16
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- +goose Up
-- +goose StatementBegin

-- ──────────────────────────────────────────────
-- Core alert records
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS optima_alerts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fingerprint     TEXT        NOT NULL,
    server_id       UUID        REFERENCES optima_servers(id) ON DELETE CASCADE,
    instance_name   TEXT        NOT NULL,
    engine          TEXT        NOT NULL CHECK (engine IN ('postgres', 'sqlserver')),
    severity        TEXT        NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    status          TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'resolved')),
    category        TEXT        NOT NULL,
    title           TEXT        NOT NULL,
    description     TEXT,
    evidence        JSONB,
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    hit_count       INT         NOT NULL DEFAULT 1,
    acknowledged_by TEXT,
    acknowledged_at TIMESTAMPTZ,
    resolved_by     TEXT,
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alerts_fingerprint_status
    ON optima_alerts (fingerprint, status) WHERE status != 'resolved';
CREATE INDEX IF NOT EXISTS idx_alerts_instance_status
    ON optima_alerts (instance_name, status);
CREATE INDEX IF NOT EXISTS idx_alerts_engine
    ON optima_alerts (engine);
CREATE INDEX IF NOT EXISTS idx_alerts_severity
    ON optima_alerts (severity, status);
CREATE INDEX IF NOT EXISTS idx_alerts_last_seen
    ON optima_alerts (last_seen_at DESC);

-- ──────────────────────────────────────────────
-- Alert state-change history (audit trail)
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS optima_alert_history (
    id          BIGSERIAL   PRIMARY KEY,
    alert_id    UUID        NOT NULL REFERENCES optima_alerts(id) ON DELETE CASCADE,
    old_status  TEXT,
    new_status  TEXT        NOT NULL,
    changed_by  TEXT,
    reason      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_history_alert
    ON optima_alert_history (alert_id, created_at DESC);

-- ──────────────────────────────────────────────
-- Maintenance windows
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS optima_maintenance_windows (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_name   TEXT        NOT NULL,
    engine          TEXT        NOT NULL CHECK (engine IN ('postgres', 'sqlserver')),
    reason          TEXT,
    starts_at       TIMESTAMPTZ NOT NULL,
    ends_at         TIMESTAMPTZ NOT NULL,
    created_by      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_window_range CHECK (ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS idx_maint_window_active
    ON optima_maintenance_windows (instance_name, starts_at, ends_at);

-- ──────────────────────────────────────────────
-- Alert rule configuration
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS optima_alert_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT        NOT NULL UNIQUE,
    engine          TEXT        NOT NULL CHECK (engine IN ('postgres', 'sqlserver', 'all')),
    category        TEXT        NOT NULL,
    default_severity TEXT       NOT NULL DEFAULT 'warning' CHECK (default_severity IN ('info', 'warning', 'critical')),
    description     TEXT,
    is_enabled      BOOLEAN     NOT NULL DEFAULT true,
    config          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed built-in alert rules
INSERT INTO optima_alert_rules (name, engine, category, default_severity, description, config) VALUES
    ('mssql_blocking',       'sqlserver', 'blocking',        'critical', 'Detects active blocking chains on SQL Server',                 '{"threshold_seconds": 30}'::jsonb),
    ('mssql_failed_jobs',    'sqlserver', 'jobs',            'warning',  'Detects SQL Agent jobs that failed in the last 24 hours',       '{"lookback_hours": 24}'::jsonb),
    ('mssql_disk_space',     'sqlserver', 'disk',            'warning',  'Detects low free disk space on SQL Server drives',              '{"warning_pct": 20, "critical_pct": 10}'::jsonb),
    ('pg_replication_lag',   'postgres',  'replication',     'warning',  'Detects replication lag exceeding threshold',                   '{"warning_bytes": 104857600, "critical_bytes": 524288000}'::jsonb),
    ('pg_blocking',          'postgres',  'blocking',        'critical', 'Detects active blocking chains on PostgreSQL',                  '{"threshold_seconds": 30}'::jsonb),
    ('pg_backup_freshness',  'postgres',  'backup',          'warning',  'Detects stale backups exceeding age threshold',                 '{"max_age_hours": 24}'::jsonb),
    ('pg_disk_space',        'postgres',  'disk',            'warning',  'Detects low free disk space on PostgreSQL data directory',       '{"warning_pct": 20, "critical_pct": 10}'::jsonb)
ON CONFLICT (name) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS optima_alert_history;
DROP TABLE IF EXISTS optima_alerts;
DROP TABLE IF EXISTS optima_maintenance_windows;
DROP TABLE IF EXISTS optima_alert_rules;
-- +goose StatementEnd
