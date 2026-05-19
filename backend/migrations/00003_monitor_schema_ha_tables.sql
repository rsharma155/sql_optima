-- +goose Up
-- +goose StatementBegin
-- Creates the monitor schema and the canonical HA/Replication tables that
-- replace the legacy public.sqlserver_ag_health table.
CREATE SCHEMA IF NOT EXISTS monitor;

-- HA Feature Detection
CREATE TABLE IF NOT EXISTS monitor.sqlserver_feature_detection (
    server_id             UUID        NOT NULL,
    capture_timestamp     TIMESTAMPTZ NOT NULL,
    ha_enabled            BOOLEAN     NOT NULL DEFAULT false,
    replication_enabled   BOOLEAN     NOT NULL DEFAULT false,
    ag_enabled            BOOLEAN     NOT NULL DEFAULT false,
    fci_enabled           BOOLEAN     NOT NULL DEFAULT false,
    log_shipping_enabled  BOOLEAN     NOT NULL DEFAULT false,
    mirroring_enabled     BOOLEAN     NOT NULL DEFAULT false,
    replication_types     TEXT[]      NOT NULL DEFAULT '{}',
    PRIMARY KEY (server_id, capture_timestamp)
);

-- HA Replica State (per-replica, from dm_hadr_availability_replica_states)
CREATE TABLE IF NOT EXISTS monitor.sqlserver_ha_replica_state (
    capture_timestamp           TIMESTAMPTZ NOT NULL,
    server_id                   UUID        NOT NULL,
    ag_name                     TEXT        NOT NULL DEFAULT '',
    replica_server_name         TEXT        NOT NULL DEFAULT '',
    role_desc                   TEXT        NOT NULL DEFAULT 'UNKNOWN',
    synchronization_state_desc  TEXT        NOT NULL DEFAULT 'UNKNOWN',
    synchronization_health_desc TEXT        NOT NULL DEFAULT 'UNKNOWN',
    availability_mode_desc      TEXT        NOT NULL DEFAULT 'UNKNOWN',
    log_send_queue_kb           BIGINT      NOT NULL DEFAULT 0,
    redo_queue_kb               BIGINT      NOT NULL DEFAULT 0,
    log_send_rate_kbps          BIGINT      NOT NULL DEFAULT 0,
    redo_rate_kbps              BIGINT      NOT NULL DEFAULT 0,
    last_commit_time            TIMESTAMPTZ,
    secondary_lag_seconds       BIGINT      NOT NULL DEFAULT 0,
    connected_state_desc        TEXT        NOT NULL DEFAULT 'UNKNOWN',
    is_failover_ready           BOOLEAN     NOT NULL DEFAULT false,
    long_running_tx_count       INT         NOT NULL DEFAULT 0,
    quorum_state_desc           TEXT        NOT NULL DEFAULT 'UNKNOWN',
    PRIMARY KEY (server_id, capture_timestamp, ag_name, replica_server_name)
);

SELECT create_hypertable('monitor.sqlserver_ha_replica_state', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.sqlserver_ha_replica_state', INTERVAL '30 days', if_not_exists => TRUE);

ALTER TABLE monitor.sqlserver_ha_replica_state SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,ag_name,replica_server_name',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_ha_replica_state', INTERVAL '7 days', if_not_exists => TRUE);

-- HA Database State (per-database, from dm_hadr_database_replica_states)
CREATE TABLE IF NOT EXISTS monitor.sqlserver_ha_database_state (
    capture_timestamp           TIMESTAMPTZ NOT NULL,
    server_id                   UUID        NOT NULL,
    ag_name                     TEXT        NOT NULL DEFAULT '',
    database_name               TEXT        NOT NULL DEFAULT '',
    replica_server_name         TEXT        NOT NULL DEFAULT '',
    synchronization_state_desc  TEXT        NOT NULL DEFAULT 'UNKNOWN',
    is_suspended                BOOLEAN     NOT NULL DEFAULT false,
    log_send_queue_kb           BIGINT      NOT NULL DEFAULT 0,
    redo_queue_kb               BIGINT      NOT NULL DEFAULT 0,
    last_commit_time            TIMESTAMPTZ,
    backup_fresh_ok             BOOLEAN     NOT NULL DEFAULT false,
    PRIMARY KEY (server_id, capture_timestamp, ag_name, database_name, replica_server_name)
);

SELECT create_hypertable('monitor.sqlserver_ha_database_state', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.sqlserver_ha_database_state', INTERVAL '30 days', if_not_exists => TRUE);

ALTER TABLE monitor.sqlserver_ha_database_state SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id,ag_name,replica_server_name',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('monitor.sqlserver_ha_database_state', INTERVAL '7 days', if_not_exists => TRUE);

-- HA Failover Events
CREATE TABLE IF NOT EXISTS monitor.sqlserver_ha_failover_events (
    capture_timestamp        TIMESTAMPTZ NOT NULL,
    server_id                UUID        NOT NULL,
    ag_name                  TEXT        NOT NULL DEFAULT '',
    previous_primary         TEXT        NOT NULL DEFAULT '',
    new_primary              TEXT        NOT NULL DEFAULT '',
    failover_type            TEXT        NOT NULL DEFAULT 'UNKNOWN',
    failover_duration_seconds INT         NOT NULL DEFAULT 0,
    PRIMARY KEY (server_id, capture_timestamp, ag_name, previous_primary, new_primary)
);

SELECT create_hypertable('monitor.sqlserver_ha_failover_events', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.sqlserver_ha_failover_events', INTERVAL '90 days', if_not_exists => TRUE);

-- Replication Topology
CREATE TABLE IF NOT EXISTS monitor.sqlserver_replication_topology (
    capture_timestamp  TIMESTAMPTZ NOT NULL,
    server_id          UUID        NOT NULL,
    publisher          TEXT        NOT NULL DEFAULT '',
    subscriber         TEXT        NOT NULL DEFAULT '',
    publication        TEXT        NOT NULL DEFAULT '',
    publication_db     TEXT        NOT NULL DEFAULT '',
    subscriber_db      TEXT        NOT NULL DEFAULT '',
    replication_type   TEXT        NOT NULL DEFAULT '',
    sync_type          TEXT        NOT NULL DEFAULT '',
    agent_status       TEXT        NOT NULL DEFAULT '',
    last_sync_time     TIMESTAMPTZ,
    PRIMARY KEY (server_id, capture_timestamp, publisher, subscriber, publication)
);

SELECT create_hypertable('monitor.sqlserver_replication_topology', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.sqlserver_replication_topology', INTERVAL '30 days', if_not_exists => TRUE);

-- Replication Latency
CREATE TABLE IF NOT EXISTS monitor.sqlserver_replication_latency (
    capture_timestamp       TIMESTAMPTZ NOT NULL,
    server_id               UUID        NOT NULL,
    publisher               TEXT        NOT NULL DEFAULT '',
    subscriber              TEXT        NOT NULL DEFAULT '',
    publication             TEXT        NOT NULL DEFAULT '',
    latency_seconds         BIGINT      NOT NULL DEFAULT 0,
    undistributed_commands  BIGINT      NOT NULL DEFAULT 0,
    delivery_rate_cmds_sec  BIGINT      NOT NULL DEFAULT 0,
    status                  TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (server_id, capture_timestamp, publisher, subscriber, publication)
);

SELECT create_hypertable('monitor.sqlserver_replication_latency', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.sqlserver_replication_latency', INTERVAL '30 days', if_not_exists => TRUE);

-- Replication Articles
CREATE TABLE IF NOT EXISTS monitor.sqlserver_replication_articles (
    capture_timestamp   TIMESTAMPTZ NOT NULL,
    server_id           UUID        NOT NULL,
    publication         TEXT        NOT NULL DEFAULT '',
    database_name       TEXT        NOT NULL DEFAULT '',
    schema_name         TEXT        NOT NULL DEFAULT '',
    table_name          TEXT        NOT NULL DEFAULT '',
    subscriber          TEXT        NOT NULL DEFAULT '',
    rows_per_sec        BIGINT      NOT NULL DEFAULT 0,
    latency_seconds     BIGINT      NOT NULL DEFAULT 0,
    conflicts_detected  BIGINT      NOT NULL DEFAULT 0,
    status              TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (server_id, capture_timestamp, publication, database_name, schema_name, table_name, subscriber)
);

SELECT create_hypertable('monitor.sqlserver_replication_articles', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('monitor.sqlserver_replication_articles', INTERVAL '30 days', if_not_exists => TRUE);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS monitor.sqlserver_replication_articles;
DROP TABLE IF EXISTS monitor.sqlserver_replication_latency;
DROP TABLE IF EXISTS monitor.sqlserver_replication_topology;
DROP TABLE IF EXISTS monitor.sqlserver_ha_failover_events;
DROP TABLE IF EXISTS monitor.sqlserver_ha_database_state;
DROP TABLE IF EXISTS monitor.sqlserver_ha_replica_state;
DROP TABLE IF EXISTS monitor.sqlserver_feature_detection;
-- Note: do not drop the monitor schema itself as it may contain other objects.
-- +goose StatementEnd
