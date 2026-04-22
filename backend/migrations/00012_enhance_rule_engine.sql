-- 00012_enhance_rule_engine.sql
-- Refactor Rule Engine to context-aware advisory system

-- Modify rules table
ALTER TABLE ruleengine.rules
ADD COLUMN IF NOT EXISTS required_signals TEXT[],
ADD COLUMN IF NOT EXISTS eval_engine VARCHAR(20) DEFAULT 'go',
ADD COLUMN IF NOT EXISTS eval_expression JSONB,
ADD COLUMN IF NOT EXISTS recommendation_template JSONB,
ADD COLUMN IF NOT EXISTS rule_version INT DEFAULT 1;

-- Modify rule_results_evaluated
ALTER TABLE ruleengine.rule_results_evaluated
ADD COLUMN IF NOT EXISTS severity VARCHAR(20),
ADD COLUMN IF NOT EXISTS confidence NUMERIC,
ADD COLUMN IF NOT EXISTS context JSONB;

-- Create NEW: signals table
CREATE TABLE IF NOT EXISTS ruleengine.signals (
    server_id INT,
    signal_key TEXT,
    signal_value NUMERIC,
    collected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (server_id, signal_key, collected_at)
);

-- Convert signals to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.signals',
    'collected_at',
    if_not_exists => TRUE
);

-- Create NEW: signal_snapshots table
CREATE TABLE IF NOT EXISTS ruleengine.signal_snapshots (
    snapshot_id BIGSERIAL PRIMARY KEY,
    server_id INT,
    db_type VARCHAR(20),
    snapshot JSONB,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Convert signal_snapshots to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.signal_snapshots',
    'created_at',
    if_not_exists => TRUE
);
