-- SQL Optima
-- Epic 2.2: SQL Server HA, Jobs, and Operational Reliability
--
-- Purpose: Add sqlserver_log_shipping_health TimescaleDB hypertable.
--          Captures primary/secondary log shipping monitor rows so latency
--          and restore-delay trends are visible historically.
--
-- Idempotent: safe to re-apply on existing deployments.

CREATE TABLE IF NOT EXISTS sqlserver_log_shipping_health (
    capture_timestamp        TIMESTAMPTZ     NOT NULL,
    server_instance_name     VARCHAR(255)    NOT NULL,
    primary_server           VARCHAR(255)    NOT NULL DEFAULT '',
    primary_database         VARCHAR(255)    NOT NULL DEFAULT '',
    secondary_server         VARCHAR(255)    NOT NULL DEFAULT '',
    secondary_database       VARCHAR(255)    NOT NULL DEFAULT '',
    last_backup_date         TIMESTAMPTZ,
    last_backup_file         VARCHAR(512)    NOT NULL DEFAULT '',
    last_restore_date        TIMESTAMPTZ,
    last_copied_date         TIMESTAMPTZ,
    restore_delay_minutes    INT             NOT NULL DEFAULT 0,
    restore_threshold_minutes INT            NOT NULL DEFAULT 0,
    status                   SMALLINT        NOT NULL DEFAULT 0,  -- 0=unknown, 1=ok, 2=warning, 3=error
    is_primary               BOOLEAN         NOT NULL DEFAULT FALSE
);

SELECT create_hypertable(
    'sqlserver_log_shipping_health',
    'capture_timestamp',
    if_not_exists => TRUE,
    migrate_data  => FALSE
);

CREATE INDEX IF NOT EXISTS idx_sqlserver_logship_server
    ON sqlserver_log_shipping_health (server_instance_name, capture_timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_sqlserver_logship_primary_db
    ON sqlserver_log_shipping_health (primary_database, capture_timestamp DESC);

ALTER TABLE sqlserver_log_shipping_health
    SET (timescaledb.compress, timescaledb.compress_segmentby = 'server_instance_name');

SELECT add_compression_policy(
    'sqlserver_log_shipping_health',
    INTERVAL '7 days',
    if_not_exists => TRUE
);
