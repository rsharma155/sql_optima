/*
===================================================================
RULE ENGINE: TABLES, FUNCTIONS, AND TIMESCALEDB ROUTINES
===================================================================
Supports both SQL Server and PostgreSQL targets
*/

-- 1. ENABLE TIMESCALE EXTENSION
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Ensure schema exists (from previous setup)
CREATE SCHEMA IF NOT EXISTS ruleengine;

CREATE TABLE IF NOT EXISTS ruleengine.servers (
    server_id    SERIAL PRIMARY KEY,
    server_name  TEXT NOT NULL UNIQUE,
    environment  TEXT,   -- prod / dev / test
    sql_version  TEXT,
    cpu_cores    INT,
    total_ram_gb INT,
    db_type     VARCHAR(20) DEFAULT 'sqlserver',  -- 'sqlserver' | 'postgres'
    created_at   TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- 4. RULE EXECUTION RUN TABLE (Tracks each polling cycle)
CREATE TABLE IF NOT EXISTS ruleengine.rule_runs (
    run_id       BIGSERIAL PRIMARY KEY,
    server_id    INT REFERENCES ruleengine.servers(server_id) ON DELETE CASCADE,
    db_type     VARCHAR(20) DEFAULT 'sqlserver',
    run_time     TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- 5. RAW RULE RESULTS TABLE (Timeseries - What the Go Agent sends)
CREATE TABLE IF NOT EXISTS ruleengine.rule_results_raw (
    run_id       BIGINT REFERENCES ruleengine.rule_runs(run_id) ON DELETE CASCADE,
    rule_id      VARCHAR(50) REFERENCES ruleengine.rules(rule_id),
    server_id    INT REFERENCES ruleengine.servers(server_id) ON DELETE CASCADE,
    raw_payload  JSONB,
    collected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Convert raw results to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.rule_results_raw',
    'collected_at',
    if_not_exists => TRUE
);

-- 6. EVALUATED RESULTS TABLE (Timeseries - What the UI reads)
CREATE TABLE IF NOT EXISTS ruleengine.rule_results_evaluated (
    run_id        BIGINT REFERENCES ruleengine.rule_runs(run_id) ON DELETE CASCADE,
    server_id     INT REFERENCES ruleengine.servers(server_id) ON DELETE CASCADE,
    rule_id       VARCHAR(50) REFERENCES ruleengine.rules(rule_id),
    target_db_type VARCHAR(20) DEFAULT 'sqlserver',
    status        TEXT,      -- Critical | Warning | OK
    current_value TEXT,
    recommended   TEXT,
    evaluated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Convert evaluated results to TimescaleDB hypertable
SELECT create_hypertable(
    'ruleengine.rule_results_evaluated',
    'evaluated_at',
    if_not_exists => TRUE
);


/*
===================================================================
FUNCTIONS (CALLED BY THE GO COLLECTOR)
===================================================================
*/

-- FUNCTION: START NEW RULE RUN
CREATE OR REPLACE FUNCTION ruleengine.start_rule_run(p_server_id INT)
RETURNS BIGINT AS $$
DECLARE 
    v_run_id BIGINT;
BEGIN
    INSERT INTO ruleengine.rule_runs(server_id)
    VALUES (p_server_id)
    RETURNING run_id INTO v_run_id;

    RETURN v_run_id;
END;
$$ LANGUAGE plpgsql;

-- FUNCTION: STORE RAW RESULT FROM AGENT
CREATE OR REPLACE FUNCTION ruleengine.store_raw_result(
    p_run_id     BIGINT,
    p_server_id  INT,
    p_rule_id    VARCHAR(50),
    p_payload    JSONB
)
RETURNS VOID AS $$
BEGIN
    INSERT INTO ruleengine.rule_results_raw (run_id, rule_id, server_id, raw_payload, collected_at)
    VALUES (p_run_id, p_rule_id, p_server_id, p_payload, CURRENT_TIMESTAMP);
END;
$$ LANGUAGE plpgsql;

-- FUNCTION: EVALUATE RULE RESULTS (Supports dynamic evaluation from Go)
CREATE OR REPLACE FUNCTION ruleengine.evaluate_run(p_run_id BIGINT)
RETURNS VOID AS $$
DECLARE 
    r RECORD;
    v_current_value TEXT;
    v_recommended TEXT;
    v_status TEXT;
BEGIN
    FOR r IN
        SELECT rr.run_id, rr.server_id, rr.rule_id, rr.raw_payload, rl.target_db_type
        FROM ruleengine.rule_results_raw rr
        JOIN ruleengine.rules rl USING(rule_id)
        WHERE rr.run_id = p_run_id
    LOOP
        -- Extract values from JSON payload (dynamically calculated by Go)
        v_current_value := r.raw_payload->>'CurrentValue';
        v_recommended := r.raw_payload->>'recommended_value';
        v_status := r.raw_payload->>'EvaluatedStatus';
        
        -- Fallback: if not set by Go, try to extract from raw_payload
        IF v_current_value IS NULL THEN
            BEGIN
                v_current_value := r.raw_payload::text;
            EXCEPTION WHEN OTHERS THEN
                v_current_value := NULL;
            END;
        END IF;
        
        -- Default status if not set
        IF v_status IS NULL THEN
            v_status := 'OK';
        END IF;

        INSERT INTO ruleengine.rule_results_evaluated
        (run_id, server_id, rule_id, target_db_type, status, current_value, recommended, evaluated_at)
        VALUES
        (r.run_id, r.server_id, r.rule_id, r.target_db_type, v_status, v_current_value, v_recommended, CURRENT_TIMESTAMP);

    END LOOP;
END;
$$ LANGUAGE plpgsql;


/*
===================================================================
VIEWS (UI / DASHBOARD QUERIES)
===================================================================
*/

-- MAIN DASHBOARD VIEW
-- Uses DISTINCT ON to guarantee only the absolute latest result per server/rule is shown,
-- regardless of how long ago the polling agent ran.

CREATE OR REPLACE VIEW ruleengine.v_main_dashboard AS
SELECT DISTINCT ON (s.server_id, r.rule_id)
    s.server_name,
    r.rule_name,
    e.status,
    r.severity,
    r.dashboard_placement,
    e.evaluated_at
FROM ruleengine.rule_results_evaluated e
JOIN ruleengine.rules r ON e.rule_id = r.rule_id
JOIN ruleengine.servers s ON e.server_id = s.server_id
WHERE r.dashboard_placement = 'Main'
ORDER BY s.server_id, r.rule_id, e.evaluated_at DESC;

-- BEST PRACTICES DASHBOARD VIEW
-- Includes target_db_type for filtering
CREATE OR REPLACE VIEW ruleengine.v_best_practices_dashboard AS
SELECT DISTINCT ON (s.server_id, r.rule_id)
    s.server_name,
    s.db_type AS target_db_type,
    r.category,
    r.rule_name,
    e.status,
    e.current_value,
    e.recommended AS recommended_value,
    COALESCE(r.fix_script, r.fix_script_pg) AS fix_script,
    r.description,
    e.evaluated_at
FROM ruleengine.rule_results_evaluated e
JOIN ruleengine.rules r ON e.rule_id = r.rule_id
JOIN ruleengine.servers s ON e.server_id = s.server_id
WHERE r.dashboard_placement = 'BestPractice'
ORDER BY s.server_id, r.rule_id, e.evaluated_at DESC;

-- BEST PRACTICES BY DB TYPE
-- Filter by SQL Server or PostgreSQL
CREATE OR REPLACE VIEW ruleengine.v_best_practices_by_type AS
SELECT DISTINCT ON (s.server_id, r.rule_id)
    s.server_id,
    s.server_name,
    s.db_type AS target_db_type,
    r.rule_id,
    r.rule_name,
    r.category,
    e.status,
    e.current_value,
    e.recommended AS recommended_value,
    COALESCE(r.fix_script, r.fix_script_pg) AS fix_script,
    r.description,
    e.evaluated_at
FROM ruleengine.rule_results_evaluated e
JOIN ruleengine.rules r ON e.rule_id = r.rule_id
JOIN ruleengine.servers s ON e.server_id = s.server_id
WHERE r.is_enabled = true
ORDER BY s.server_id, r.rule_id, e.evaluated_at DESC;


-- Add target_db_type column if not exists
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'ruleengine' AND column_name = 'target_db_type') THEN
        ALTER TABLE ruleengine.rule_results_evaluated ADD COLUMN target_db_type VARCHAR(20) DEFAULT 'sqlserver';
    END IF;
END $$;

-- 3. SERVERS TABLE (Inventory) - Add db_type
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'ruleengine' AND table_name = 'servers' AND column_name = 'db_type') THEN
        ALTER TABLE ruleengine.servers ADD COLUMN db_type VARCHAR(20) DEFAULT 'sqlserver';
    END IF;
END $$;