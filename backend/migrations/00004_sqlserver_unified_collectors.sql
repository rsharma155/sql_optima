-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- Phase 4: Unified Session Snapshot and Memory Grants Detail
-- ============================================================================

-- sqlserver_active_sessions — unified session snapshot
-- One row per session per collection cycle.
CREATE TABLE IF NOT EXISTS sqlserver_active_sessions (
    capture_timestamp       TIMESTAMPTZ     NOT NULL,
    server_id               UUID            NOT NULL,
    session_id              INT             NOT NULL,
    login_name              TEXT            NOT NULL DEFAULT '',
    host_name               TEXT            NOT NULL DEFAULT '',
    program_name            TEXT            NOT NULL DEFAULT '',
    database_name           TEXT            NOT NULL DEFAULT '',
    request_status          TEXT            NOT NULL DEFAULT '',
    wait_type               TEXT            NOT NULL DEFAULT '',
    wait_time_ms            BIGINT          NOT NULL DEFAULT 0,
    blocking_session_id     INT             NOT NULL DEFAULT 0,
    cpu_time_ms             BIGINT          NOT NULL DEFAULT 0,
    total_elapsed_ms        BIGINT          NOT NULL DEFAULT 0,
    logical_reads           BIGINT          NOT NULL DEFAULT 0,
    reads                   BIGINT          NOT NULL DEFAULT 0,
    writes                  BIGINT          NOT NULL DEFAULT 0,
    granted_query_memory_kb BIGINT          NOT NULL DEFAULT 0,
    dop                     INT             NOT NULL DEFAULT 0,
    query_hash              TEXT            NOT NULL DEFAULT '',
    query_text              TEXT            NOT NULL DEFAULT ''
);

SELECT create_hypertable('sqlserver_active_sessions', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_sessions_server_time
    ON sqlserver_active_sessions (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_sessions_blocking
    ON sqlserver_active_sessions (server_id, capture_timestamp DESC, blocking_session_id)
    WHERE blocking_session_id > 0;

SELECT add_retention_policy('sqlserver_active_sessions',
    INTERVAL '30 days', if_not_exists => TRUE);

-- sqlserver_memory_grants_detail — per-session memory grant detail
CREATE TABLE IF NOT EXISTS sqlserver_memory_grants_detail (
    capture_timestamp       TIMESTAMPTZ     NOT NULL,
    server_id               UUID            NOT NULL,
    session_id              INT             NOT NULL,
    login_name              TEXT            NOT NULL DEFAULT '',
    database_name           TEXT            NOT NULL DEFAULT '',
    query_cost              NUMERIC(19,4)   NOT NULL DEFAULT 0,
    requested_memory_kb     BIGINT          NOT NULL DEFAULT 0,
    granted_memory_kb       BIGINT          NOT NULL DEFAULT 0,
    used_memory_kb          BIGINT          NOT NULL DEFAULT 0,
    max_used_memory_kb      BIGINT          NOT NULL DEFAULT 0,
    dop                     INT             NOT NULL DEFAULT 0,
    grant_time              TIMESTAMPTZ,
    queue_time              TIMESTAMPTZ
);

SELECT create_hypertable('sqlserver_memory_grants_detail', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_grants_detail_server_time
    ON sqlserver_memory_grants_detail (server_id, capture_timestamp DESC);

SELECT add_retention_policy('sqlserver_memory_grants_detail',
    INTERVAL '30 days', if_not_exists => TRUE);

-- ============================================================================
-- Phase 5: Index Health (Fragmentation and Missing Indexes)
-- ============================================================================

-- sqlserver_index_fragmentation — physical index health
CREATE TABLE IF NOT EXISTS sqlserver_index_fragmentation (
    capture_timestamp           TIMESTAMPTZ     NOT NULL,
    server_id                   UUID            NOT NULL,
    database_name               TEXT            NOT NULL DEFAULT '',
    schema_name                 TEXT            NOT NULL DEFAULT '',
    table_name                  TEXT            NOT NULL DEFAULT '',
    index_name                  TEXT            NOT NULL DEFAULT '',
    index_id                    INT             NOT NULL DEFAULT 0,
    index_type_desc             TEXT            NOT NULL DEFAULT '',
    avg_fragmentation_pct       NUMERIC(8,2)    NOT NULL DEFAULT 0,
    page_count                  BIGINT          NOT NULL DEFAULT 0,
    avg_page_space_used_pct     NUMERIC(8,2)    NOT NULL DEFAULT 0,
    record_count                BIGINT          NOT NULL DEFAULT 0,
    fragment_count              BIGINT          NOT NULL DEFAULT 0,
    avg_fragment_size_pages     NUMERIC(8,2)    NOT NULL DEFAULT 0
);

SELECT create_hypertable('sqlserver_index_fragmentation', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_idx_frag_server_time
    ON sqlserver_index_fragmentation (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_idx_frag_db
    ON sqlserver_index_fragmentation (server_id, database_name, capture_timestamp DESC);

SELECT add_retention_policy('sqlserver_index_fragmentation',
    INTERVAL '14 days', if_not_exists => TRUE);

-- sqlserver_missing_indexes — advisor output from dm_db_missing_index_* DMVs
CREATE TABLE IF NOT EXISTS sqlserver_missing_indexes (
    capture_timestamp           TIMESTAMPTZ     NOT NULL,
    server_id                   UUID            NOT NULL,
    database_name               TEXT            NOT NULL DEFAULT '',
    schema_name                 TEXT            NOT NULL DEFAULT '',
    table_name                  TEXT            NOT NULL DEFAULT '',
    equality_columns            TEXT            NOT NULL DEFAULT '',
    inequality_columns          TEXT            NOT NULL DEFAULT '',
    included_columns            TEXT            NOT NULL DEFAULT '',
    user_seeks                  BIGINT          NOT NULL DEFAULT 0,
    user_scans                  BIGINT          NOT NULL DEFAULT 0,
    avg_total_user_cost         NUMERIC(19,4)   NOT NULL DEFAULT 0,
    avg_user_impact             NUMERIC(8,2)    NOT NULL DEFAULT 0,
    improvement_score           NUMERIC(19,4)   NOT NULL DEFAULT 0,
    last_user_seek              TIMESTAMPTZ,
    last_user_scan              TIMESTAMPTZ
);

SELECT create_hypertable('sqlserver_missing_indexes', 'capture_timestamp',
    if_not_exists => TRUE, migrate_data => TRUE);

CREATE INDEX IF NOT EXISTS idx_sqlserver_missing_idx_server_time
    ON sqlserver_missing_indexes (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sqlserver_missing_idx_db
    ON sqlserver_missing_indexes (server_id, database_name, capture_timestamp DESC);

SELECT add_retention_policy('sqlserver_missing_indexes',
    INTERVAL '14 days', if_not_exists => TRUE);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS sqlserver_missing_indexes;
DROP TABLE IF EXISTS sqlserver_index_fragmentation;
DROP TABLE IF EXISTS sqlserver_memory_grants_detail;
DROP TABLE IF EXISTS sqlserver_active_sessions;
-- +goose StatementEnd
