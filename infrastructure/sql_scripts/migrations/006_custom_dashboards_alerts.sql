-- ============================================
-- SQL Monitoring Tool - Schema Enhancement
-- Custom Dashboards, Alerts, and Server Management
-- ============================================

-- ============================================
-- User-Defined Dashboards
-- ============================================
CREATE TABLE IF NOT EXISTS user_dashboards (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    dashboard_name TEXT NOT NULL,
    dashboard_type TEXT NOT NULL DEFAULT 'custom',
    layout_config JSONB NOT NULL DEFAULT '{}',
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('user_dashboards', 'created_at', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_user_dashboards_user 
    ON user_dashboards (user_id);
CREATE INDEX IF NOT EXISTS idx_user_dashboards_name 
    ON user_dashboards (dashboard_name);

-- ============================================
-- Dashboard Widgets Configuration
-- ============================================
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

CREATE INDEX IF NOT EXISTS idx_dashboard_widgets_dashboard 
    ON dashboard_widgets (dashboard_id);

-- ============================================
-- Alert Threshold Configurations
-- ============================================
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

CREATE INDEX IF NOT EXISTS idx_alert_thresholds_user 
    ON alert_thresholds (user_id);
CREATE INDEX IF NOT EXISTS idx_alert_thresholds_metric 
    ON alert_thresholds (metric_name);
CREATE INDEX IF NOT EXISTS idx_alert_thresholds_enabled 
    ON alert_thresholds (is_enabled) WHERE is_enabled = TRUE;

-- ============================================
-- Alert Notifications Channels
-- ============================================
CREATE TABLE IF NOT EXISTS notification_channels (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    channel_name TEXT NOT NULL,
    channel_type TEXT NOT NULL CHECK (channel_type IN ('email', 'slack', 'webhook', 'pagerduty')),
    config JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_channels_user 
    ON notification_channels (user_id);

-- ============================================
-- Alert Subscriptions
-- ============================================
CREATE TABLE IF NOT EXISTS alert_subscriptions (
    id SERIAL PRIMARY KEY,
    threshold_id INTEGER REFERENCES alert_thresholds(id) ON DELETE CASCADE,
    channel_id INTEGER REFERENCES notification_channels(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(threshold_id, channel_id)
);

-- ============================================
-- Alert History (Triggered Alerts)
-- ============================================
CREATE TABLE IF NOT EXISTS alert_history (
    id SERIAL PRIMARY KEY,
    threshold_id INTEGER REFERENCES alert_thresholds(id),
    instance_name TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value FLOAT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('warning', 'critical')),
    message TEXT,
    acknowledged BOOLEAN DEFAULT FALSE,
    acknowledged_by INTEGER,
    acknowledged_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('alert_history', 'created_at', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_alert_history_instance 
    ON alert_history (instance_name);
CREATE INDEX IF NOT EXISTS idx_alert_history_created 
    ON alert_history (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_history_threshold 
    ON alert_history (threshold_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_acknowledged 
    ON alert_history (acknowledged) WHERE acknowledged = FALSE;

-- ============================================
-- Server Configurations (User-Managed)
-- ============================================
CREATE TABLE IF NOT EXISTS monitored_servers (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    server_name TEXT NOT NULL,
    server_type TEXT NOT NULL CHECK (server_type IN ('sqlserver', 'postgres')),
    host TEXT NOT NULL,
    port INTEGER DEFAULT 1433,
    database_name TEXT,
    connection_string_encrypted TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    collection_enabled BOOLEAN DEFAULT TRUE,
    collection_interval TEXT DEFAULT '15s',
    tags JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, server_name)
);

CREATE INDEX IF NOT EXISTS idx_monitored_servers_user 
    ON monitored_servers (user_id);
CREATE INDEX IF NOT EXISTS idx_monitored_servers_active 
    ON monitored_servers (is_active) WHERE is_active = TRUE;

-- ============================================
-- Custom Metric Collection Settings
-- ============================================
CREATE TABLE IF NOT EXISTS metric_collection_settings (
    id SERIAL PRIMARY KEY,
    server_id INTEGER REFERENCES monitored_servers(id) ON DELETE CASCADE,
    metric_category TEXT NOT NULL,
    is_enabled BOOLEAN DEFAULT TRUE,
    collection_interval TEXT DEFAULT '30s',
    retention_period TEXT DEFAULT '7 days',
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_metric_collection_server 
    ON metric_collection_settings (server_id);

-- ============================================
-- Dashboard Export/Import Config
-- ============================================
CREATE TABLE IF NOT EXISTS dashboard_exports (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    export_name TEXT NOT NULL,
    export_type TEXT NOT NULL CHECK (export_type IN ('dashboard', 'alerts', 'servers', 'full')),
    export_data JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dashboard_exports_user 
    ON dashboard_exports (user_id);

-- ============================================
-- Comments and Documentation
-- ============================================
COMMENT ON TABLE user_dashboards IS 'Stores user-defined custom dashboard configurations';
COMMENT ON TABLE dashboard_widgets IS 'Individual widget configurations for dashboards';
COMMENT ON TABLE alert_thresholds IS 'User-configured alert thresholds for metrics';
COMMENT ON TABLE notification_channels IS 'Notification delivery channels (email, slack, webhook)';
COMMENT ON TABLE alert_subscriptions IS 'Links thresholds to notification channels';
COMMENT ON TABLE alert_history IS 'History of all triggered alerts';
COMMENT ON TABLE monitored_servers IS 'User-managed server configurations';
COMMENT ON TABLE metric_collection_settings IS 'Custom metric collection settings per server';
COMMENT ON TABLE dashboard_exports IS 'Stored export configurations for backup/sharing';

-- ============================================
-- Add compression policy for alert history
-- ============================================
ALTER TABLE alert_history SET (
    timescaledb.compress,
    timescaledb.compress_orderby = 'created_at DESC',
    timescaledb.compress_segmentby = 'instance_name, metric_name'
);

SELECT add_compression_policy('alert_history', INTERVAL '30 days', if_not_exists => TRUE);