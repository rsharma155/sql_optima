-- SQL Optima — PostgreSQL Control Center Revamp
-- Purpose: Schema for high-speed dashboard snapshot and trend tracking.

-- 1. Snapshot Table (Single Row Per Instance)
CREATE TABLE IF NOT EXISTS pg_instance_snapshot (
    instance_id text,
    collected_at timestamptz DEFAULT now(),
    -- workload
    tps numeric,
    active_sessions int,
    idle_sessions int,
    idle_in_tx_sessions int,
    blocked_sessions int,
    -- cpu & memory
    cpu_usage numeric,
    shared_buffers_used_pct numeric,
    cache_hit_ratio numeric,
    -- durability
    wal_mb_per_min numeric,
    checkpoints_timed int,
    checkpoints_req int,
    checkpoint_write_time numeric,
    -- risk
    max_xid_age bigint,
    oldest_tx_age_sec bigint,
    -- storage
    database_size_gb numeric,
    temp_bytes_mb numeric,
    -- vacuum
    autovacuum_workers int,
    dead_tuple_pct numeric,
    -- replication
    replica_lag_sec numeric,
    replication_slots int,
    -- health
    health_score int,
    version text,
    uptime text,
    checkpoint_req_ratio numeric DEFAULT 0,
    PRIMARY KEY(instance_id)
);

-- 2. Time-Series Hypertables

-- WAL Metrics
CREATE TABLE IF NOT EXISTS pg_ts_wal_metrics(
    time timestamptz NOT NULL,
    instance_id text NOT NULL,
    wal_mb numeric
);
SELECT create_hypertable('pg_ts_wal_metrics','time', if_not_exists=>TRUE);

-- TPS
CREATE TABLE IF NOT EXISTS pg_ts_tps(
    time timestamptz NOT NULL,
    instance_id text NOT NULL,
    tps numeric
);
SELECT create_hypertable('pg_ts_tps','time', if_not_exists=>TRUE);

-- Dead Tuple Trend
CREATE TABLE IF NOT EXISTS pg_ts_bloat(
    time timestamptz NOT NULL,
    instance_id text NOT NULL,
    dead_tuple_pct numeric
);
SELECT create_hypertable('pg_ts_bloat','time', if_not_exists=>TRUE);

-- Replication Lag
CREATE TABLE IF NOT EXISTS pg_ts_replication_lag(
    time timestamptz NOT NULL,
    instance_id text NOT NULL,
    replica text,
    lag_sec numeric
);
SELECT create_hypertable('pg_ts_replication_lag','time', if_not_exists=>TRUE);
