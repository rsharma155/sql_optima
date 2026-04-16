-- +goose Up
-- +goose StatementBegin

-- ==========================================================================
-- Baseline migration: core application tables.
--
-- For NEW databases: run `goose up` to create the application schema.
-- For EXISTING databases: these tables already exist; `IF NOT EXISTS` makes
-- the migration safe to apply without data loss.
--
-- Time-series / metric hypertables remain in infrastructure/sql_scripts/
-- and should be provisioned separately via the setup API or manually.
-- ==========================================================================

-- 1. Server Registry
CREATE TABLE IF NOT EXISTS optima_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    db_type TEXT NOT NULL CHECK (db_type IN ('postgres','sqlserver')),
    host TEXT NOT NULL,
    port INT NOT NULL,
    username TEXT NOT NULL,
    auth_type TEXT NOT NULL DEFAULT 'static',
    encrypted_secret BYTEA NOT NULL,
    encrypted_dek BYTEA NOT NULL,
    ssl_mode TEXT DEFAULT 'require',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    created_by TEXT,
    last_test_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_optima_servers_active ON optima_servers (is_active);
CREATE INDEX IF NOT EXISTS idx_optima_servers_name   ON optima_servers (name);
CREATE INDEX IF NOT EXISTS idx_optima_servers_type   ON optima_servers (db_type);

-- 2. Audit Log
CREATE TABLE IF NOT EXISTS optima_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    server_id UUID NULL,
    actor TEXT,
    ip_address TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_optima_audit_logs_time        ON optima_audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_optima_audit_logs_server_time ON optima_audit_logs (server_id, created_at DESC);

-- 3. User Management
CREATE TABLE IF NOT EXISTS optima_users (
    user_id       SERIAL PRIMARY KEY,
    username      VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(50)  NOT NULL DEFAULT 'viewer',
    created_at    TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_optima_users_username ON optima_users (username);

-- 4. Widget Registry
CREATE TABLE IF NOT EXISTS optima_ui_widgets (
    widget_id         VARCHAR(100) PRIMARY KEY,
    dashboard_section VARCHAR(100) NOT NULL,
    title             VARCHAR(200) NOT NULL,
    chart_type        VARCHAR(50)  NOT NULL,
    current_sql       TEXT         NOT NULL,
    default_sql       TEXT         NOT NULL,
    updated_at        TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_optima_widgets_section ON optima_ui_widgets (dashboard_section);

-- 5. EXPLAIN Plan Cache
CREATE TABLE IF NOT EXISTS plan_analysis_cache (
    plan_hash              TEXT PRIMARY KEY,
    schema_version         INTEGER NOT NULL DEFAULT 1,
    query_text             TEXT NULL,
    raw_plan_json          JSONB NOT NULL,
    report_json            JSONB NOT NULL,
    total_execution_time_ms DOUBLE PRECISION NULL,
    created_at             TIMESTAMPTZ DEFAULT NOW(),
    updated_at             TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plan_analysis_cache_updated_at ON plan_analysis_cache (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_plan_analysis_cache_exec_time  ON plan_analysis_cache (total_execution_time_ms DESC);

-- 6. Custom Dashboards
CREATE TABLE IF NOT EXISTS user_dashboards (
    id SERIAL,
    user_id INTEGER NOT NULL,
    dashboard_name TEXT NOT NULL,
    dashboard_type TEXT NOT NULL DEFAULT 'custom',
    layout_config JSONB NOT NULL DEFAULT '{}',
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
);
CREATE INDEX IF NOT EXISTS idx_user_dashboards_user ON user_dashboards (user_id);
CREATE INDEX IF NOT EXISTS idx_user_dashboards_name ON user_dashboards (dashboard_name);

DO $$
BEGIN
    ALTER TABLE user_dashboards ADD CONSTRAINT user_dashboards_id_unique UNIQUE (id);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Unique constraint may already exist: %', SQLERRM;
END $$;

-- 7. Dashboard Widgets
CREATE TABLE IF NOT EXISTS dashboard_widgets (
    id SERIAL PRIMARY KEY,
    dashboard_id INTEGER REFERENCES user_dashboards(id) ON DELETE CASCADE,
    widget_type TEXT NOT NULL,
    widget_title TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    chart_type TEXT DEFAULT 'line',
    position_x INTEGER DEFAULT 0,
    position_y INTEGER DEFAULT 0,
    width INTEGER DEFAULT 4,
    height INTEGER DEFAULT 3,
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dashboard_widgets_dashboard ON dashboard_widgets (dashboard_id);

-- 8. Alert Thresholds
CREATE TABLE IF NOT EXISTS alert_thresholds (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    metric_name TEXT NOT NULL,
    threshold_name TEXT NOT NULL,
    threshold_type TEXT NOT NULL CHECK (threshold_type IN ('cpu', 'memory', 'disk', 'connections', 'tps', 'wait', 'custom')),
    condition_type TEXT NOT NULL CHECK (condition_type IN ('above', 'below', 'equals', 'between')),
    warning_threshold FLOAT NOT NULL,
    critical_threshold FLOAT,
    evaluation_interval TEXT DEFAULT '5m',
    evaluation_window TEXT DEFAULT '5m',
    is_enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_alert_thresholds_user    ON alert_thresholds (user_id);
CREATE INDEX IF NOT EXISTS idx_alert_thresholds_metric  ON alert_thresholds (metric_name);
CREATE INDEX IF NOT EXISTS idx_alert_thresholds_enabled ON alert_thresholds (is_enabled) WHERE is_enabled = TRUE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Reverse order of creation (respecting FK dependencies).
DROP TABLE IF EXISTS dashboard_widgets;
DROP TABLE IF EXISTS user_dashboards;
DROP TABLE IF EXISTS alert_thresholds;
DROP TABLE IF EXISTS plan_analysis_cache;
DROP TABLE IF EXISTS optima_ui_widgets;
DROP TABLE IF EXISTS optima_users;
DROP TABLE IF EXISTS optima_audit_logs;
DROP TABLE IF EXISTS optima_servers;

-- +goose StatementEnd
