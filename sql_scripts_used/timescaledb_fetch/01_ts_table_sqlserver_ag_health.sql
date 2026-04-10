-- Metric: ts_table_sqlserver_ag_health
-- Source: backend/internal/storage/hot/storage.go:135
-- Target Table: sqlserver_ag_health (CREATE TABLE)
-- Description: Stores AlwaysOn Availability Group health metrics

CREATE TABLE IF NOT EXISTS sqlserver_ag_health (
    capture_timestamp TIMESTAMPTZ NOT NULL,
    server_instance_name TEXT NOT NULL,
    ag_name TEXT,
    replica_server_name TEXT,
    database_name TEXT,
    replica_role TEXT,
    synchronization_state TEXT,
    synchronization_state_desc TEXT,
    is_primary_replica BOOLEAN,
    log_send_queue_kb BIGINT DEFAULT 0,
    redo_queue_kb BIGINT DEFAULT 0,
    log_send_rate_kb BIGINT DEFAULT 0,
    redo_rate_kb BIGINT DEFAULT 0,
    last_sent_time TIMESTAMPTZ,
    last_received_time TIMESTAMPTZ,
    last_hardened_time TIMESTAMPTZ,
    last_redone_time TIMESTAMPTZ,
    secondary_lag_seconds BIGINT DEFAULT 0,
    inserted_at TIMESTAMPTZ DEFAULT NOW()
);
