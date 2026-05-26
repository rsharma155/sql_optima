-- SQL Optima — idempotent platform config tables (upgrade patch).
-- Safe to re-run on every deploy (Docker schema-patches / manual upgrade).
-- Requires optima_servers (from 01_timescale_schema.sql) for optima_server_dr_policy.
--
-- These are dimension/config tables (not time-series). Do not hypertable or compress;
-- historical backup/DR metrics live in snapshot.* / postgres_backup_runs hypertables.

SET search_path TO public, pg_catalog;

-- ---------------------------------------------------------------------------
-- optima_server_dr_policy — per-server RPO/RTO thresholds (UI + alert engine)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS optima_server_dr_policy (
    server_id UUID PRIMARY KEY REFERENCES optima_servers(id) ON DELETE CASCADE,
    rpo_backup_hours      INT NOT NULL DEFAULT 24,
    rpo_archive_minutes   INT NOT NULL DEFAULT 5,
    rpo_replay_seconds    INT NOT NULL DEFAULT 60,
    max_slot_retention_gb NUMERIC(10, 2) NOT NULL DEFAULT 10,
    rto_failover_minutes  INT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by TEXT
);

ALTER TABLE optima_server_dr_policy ADD COLUMN IF NOT EXISTS rpo_log_backup_minutes INT NOT NULL DEFAULT 15;

COMMENT ON COLUMN optima_server_dr_policy.rpo_log_backup_minutes IS
    'SQL Server: max minutes since last log backup for FULL/BULK_LOGGED databases (Backup & Recovery dashboard).';

COMMENT ON TABLE optima_server_dr_policy IS
    'Current RPO/RTO thresholds per monitored server for Backup & DR readiness and alerts. '
    'Dimension table (not time-series); use postgres_backup_runs and snapshot.pg_backup_dr_timeseries for history.';

CREATE INDEX IF NOT EXISTS idx_optima_server_dr_policy_updated
    ON optima_server_dr_policy (updated_at DESC);

-- ---------------------------------------------------------------------------
-- optima_notification_config — outbound webhook/Slack (admin UI)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS optima_notification_config (
    id              SERIAL PRIMARY KEY,
    channel         VARCHAR(50) UNIQUE NOT NULL,  -- 'webhook' | 'slack'
    url             TEXT NOT NULL DEFAULT '',
    is_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      VARCHAR(100)
);

INSERT INTO optima_notification_config (channel, url, is_enabled)
VALUES ('webhook', '', false), ('slack', '', false)
ON CONFLICT (channel) DO NOTHING;

GRANT SELECT, INSERT, UPDATE ON optima_notification_config TO sql_optima_app;
GRANT USAGE, SELECT ON SEQUENCE optima_notification_config_id_seq TO sql_optima_app;

COMMENT ON TABLE optima_notification_config IS
    'Admin-managed webhook/Slack URLs for alert notifications (loaded by service.Notifier at startup).';
