-- ============================================================
-- Rule Engine Schema for PostgreSQL (TimescaleDB)
-- Run this script to set up the rule engine backend
-- ============================================================

-- Create ruleengine schema
CREATE SCHEMA IF NOT EXISTS ruleengine;

-- Create rules configuration table
CREATE TABLE IF NOT EXISTS ruleengine.rules (
    rule_id SERIAL PRIMARY KEY,
    rule_name VARCHAR(255) NOT NULL,
    category VARCHAR(100) NOT NULL,
    description TEXT,
    detection_sql TEXT NOT NULL,
    fix_script TEXT,
    comparison_type VARCHAR(50) DEFAULT 'exact',
    threshold_value JSONB,
    is_enabled BOOLEAN DEFAULT true,
    priority INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create rule_results table to store raw detection results
CREATE TABLE IF NOT EXISTS ruleengine.rule_results (
    result_id BIGSERIAL PRIMARY KEY,
    run_id INTEGER NOT NULL,
    rule_id INTEGER NOT NULL,
    server_id INTEGER NOT NULL,
    raw_results JSONB,
    detected_at TIMESTAMP DEFAULT NOW(),
    error_message TEXT
);

-- Create rule_runs table to track execution runs
CREATE TABLE IF NOT EXISTS ruleengine.rule_runs (
    run_id SERIAL PRIMARY KEY,
    server_id INTEGER NOT NULL,
    started_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,
    status VARCHAR(50) DEFAULT 'running'
);

-- Create evaluated_results table with status
CREATE TABLE IF NOT EXISTS ruleengine.evaluated_results (
    eval_id BIGSERIAL PRIMARY KEY,
    run_id INTEGER NOT NULL,
    rule_id INTEGER NOT NULL,
    server_id INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL,
    current_value TEXT,
    recommended_value TEXT,
    json_payload JSONB,
    evaluated_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================
-- Dashboard View (reads from latest evaluated results)
-- ============================================================
CREATE OR REPLACE VIEW ruleengine.v_best_practices_dashboard AS
SELECT 
    r.rule_id,
    r.rule_name,
    r.category,
    COALESCE(er.status, 'OK') AS status,
    er.current_value,
    r.threshold_value::text AS recommended_value,
    r.description,
    r.fix_script,
    er.evaluated_at AS last_check,
    COALESCE(er.server_id, 1) AS server_id
FROM ruleengine.rules r
LEFT JOIN LATERAL (
    SELECT rule_id, server_id, status, current_value, evaluated_at
    FROM ruleengine.evaluated_results
    WHERE rule_id = r.rule_id
    ORDER BY evaluated_at DESC
    LIMIT 1
) er ON true
WHERE r.is_enabled = true
ORDER BY 
    CASE COALESCE(er.status, 'OK')
        WHEN 'CRITICAL' THEN 1
        WHEN 'WARNING' THEN 2
        ELSE 3
    END,
    r.category,
    r.rule_name;

-- ============================================================
-- Stored Functions
-- ============================================================

-- Start a rule run
CREATE OR REPLACE FUNCTION ruleengine.start_rule_run(p_server_id INTEGER)
RETURNS INTEGER AS $$
DECLARE v_run_id INTEGER;
BEGIN
    INSERT INTO ruleengine.rule_runs (server_id, status)
    VALUES (p_server_id, 'running')
    RETURNING run_id INTO v_run_id;
    RETURN v_run_id;
END;
$$ LANGUAGE plpgsql;

-- Store raw detection result
CREATE OR REPLACE FUNCTION ruleengine.store_raw_result(
    p_run_id INTEGER,
    p_rule_id INTEGER,
    p_server_id INTEGER,
    p_payload JSONB
) RETURNS VOID AS $$
BEGIN
    INSERT INTO ruleengine.rule_results (run_id, rule_id, server_id, raw_results)
    VALUES (p_run_id, p_rule_id, p_server_id, p_payload);
END;
$$ LANGUAGE plpgsql;

-- Evaluate run results
CREATE OR REPLACE FUNCTION ruleengine.evaluate_run(p_run_id INTEGER) RETURNS VOID AS $$
DECLARE
    v_result RECORD;
    v_rule RECORD;
    v_status VARCHAR(50);
BEGIN
    FOR v_result IN
        SELECT rr.rule_id, rr.server_id, rr.raw_results
        FROM ruleengine.rule_results rr
        WHERE rr.run_id = p_run_id
    LOOP
        SELECT r.rule_id, r.comparison_type, r.threshold_value
        INTO v_rule
        FROM ruleengine.rules r
        WHERE r.rule_id = v_result.rule_id;

        v_status := 'OK';
        
        IF v_rule.threshold_value IS NOT NULL THEN
            DECLARE
                v_current_val TEXT := v_result.raw_results->>'value';
                v_threshold TEXT := v_rule.threshold_value->>'value';
                v_operator TEXT := v_rule.threshold_value->>'operator';
            BEGIN
                IF v_current_val IS NOT NULL AND v_threshold IS NOT NULL THEN
                    IF v_operator = '>' AND (v_current_val::NUMERIC) > (v_threshold::NUMERIC) THEN
                        v_status := 'CRITICAL';
                    ELSIF v_operator = '<' AND (v_current_val::NUMERIC) < (v_threshold::NUMERIC) THEN
                        v_status := 'WARNING';
                    ELSIF v_operator = '=' AND v_current_val != v_threshold THEN
                        v_status := 'WARNING';
                    ELSIF v_operator = '>=' AND (v_current_val::NUMERIC) < (v_threshold::NUMERIC) THEN
                        v_status := 'WARNING';
                    END IF;
                END IF;
            EXCEPTION WHEN OTHERS THEN
                v_status := 'ERROR';
            END;
        END IF;

        INSERT INTO ruleengine.evaluated_results (run_id, rule_id, server_id, status, current_value, json_payload)
        VALUES (p_run_id, v_result.rule_id, v_result.server_id, v_status, 
                v_result.raw_results::text, v_result.raw_results);
    END LOOP;

    UPDATE ruleengine.rule_runs 
    SET completed_at = NOW(), status = 'completed' 
    WHERE run_id = p_run_id;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- Sample Rules (MSSQL examples)
-- ============================================================
INSERT INTO ruleengine.rules (rule_name, category, description, detection_sql, fix_script, comparison_type, threshold_value, priority) VALUES 
('Max Server Memory', 'Memory', 'SQL Server memory should be capped - default unlimited will starve the OS',
'SELECT CAST(value_in_use AS VARCHAR(50)) AS value FROM sys.configurations WITH (NOLOCK) WHERE name = ''max server memory (MB)''',
'EXEC sp_configure ''max server memory (MB)'', 8192; RECONFIGURE;',
'threshold', '{"value": "2147483647", "operator": "="}', 10),

('Cost Threshold for Parallelism', 'Performance', 'Default 5 is too low for modern workloads',
'SELECT CAST(value_in_use AS VARCHAR(50)) AS value FROM sys.configurations WITH (NOLOCK) WHERE name = ''cost threshold for parallelism''',
'EXEC sp_configure ''cost threshold for parallelism'', 50; RECONFIGURE;',
'threshold', '{"value": "50", "operator": ">="}', 20),

('Optimize for Ad Hoc Workloads', 'Performance', 'Prevents plan cache bloat from single-use queries',
'SELECT CAST(value_in_use AS VARCHAR(50)) AS value FROM sys.configurations WITH (NOLOCK) WHERE name = ''optimize for ad hoc workloads''',
'EXEC sp_configure ''optimize for ad hoc workloads'', 1; RECONFIGURE;',
'threshold', '{"value": "1", "operator": "="}', 30),

('Backup Compression Default', 'Storage', 'Enable backup compression to save disk space',
'SELECT CAST(value_in_use AS VARCHAR(50)) AS value FROM sys.configurations WITH (NOLOCK) WHERE name = ''backup compression default''',
'EXEC sp_configure ''backup compression default'', 1; RECONFIGURE;',
'threshold', '{"value": "1", "operator": "="}', 40)
ON CONFLICT DO NOTHING;

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_rule_results_run ON ruleengine.rule_results(run_id);
CREATE INDEX IF NOT EXISTS idx_rule_results_rule_server ON ruleengine.rule_results(rule_id, server_id);
CREATE INDEX IF NOT EXISTS idx_evaluated_results_rule ON ruleengine.evaluated_results(rule_id);

SELECT 'Rule Engine Schema Created' AS status;