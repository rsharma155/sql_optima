-- +goose Up
-- +goose StatementBegin
-- Wide-row replacement for the legacy EAV pg_ts_metrics table.
-- Captures per-cycle PostgreSQL instance KPIs as typed columns for
-- efficient time-series queries without EAV aggregation overhead.
CREATE TABLE IF NOT EXISTS postgres_snapshot_metrics (
    capture_timestamp     TIMESTAMPTZ    NOT NULL,
    server_id             UUID           NOT NULL,
    tps                   DOUBLE PRECISION DEFAULT 0,
    wal_mb_per_min        DOUBLE PRECISION DEFAULT 0,
    dead_tuple_pct        DOUBLE PRECISION DEFAULT 0,
    replica_lag_sec       DOUBLE PRECISION DEFAULT 0,
    cache_hit_ratio       DOUBLE PRECISION DEFAULT 0,
    checkpoint_req_ratio  DOUBLE PRECISION DEFAULT 0,
    database_size_gb      DOUBLE PRECISION DEFAULT 0,
    temp_bytes_mb         DOUBLE PRECISION DEFAULT 0,
    cpu_usage_pct         DOUBLE PRECISION DEFAULT 0,
    memory_usage_pct      DOUBLE PRECISION DEFAULT 0,
    active_sessions       INTEGER          DEFAULT 0,
    idle_sessions         INTEGER          DEFAULT 0,
    idle_in_txn_sessions  INTEGER          DEFAULT 0,
    waiting_sessions      INTEGER          DEFAULT 0,
    health_score          INTEGER          DEFAULT 0
);

SELECT create_hypertable('postgres_snapshot_metrics', 'capture_timestamp', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_pg_snap_metrics_server_time
    ON postgres_snapshot_metrics (server_id, capture_timestamp DESC);

ALTER TABLE postgres_snapshot_metrics SET (
    timescaledb.compress = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);

SELECT add_compression_policy('postgres_snapshot_metrics', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('postgres_snapshot_metrics', INTERVAL '90 days', if_not_exists => TRUE);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS postgres_snapshot_metrics;
-- +goose StatementEnd
