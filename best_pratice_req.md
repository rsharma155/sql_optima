🎯 Objective

Refactor the existing rule engine into a production-grade, context-aware advisory system by:

Enhancing (not replacing) existing tables
Moving rule logic from SQL into Go
Introducing a signal-based evaluation model
Maintaining backward compatibility with current JS UI
Following DDD + TDD rigorously

Organizing SQL scripts under:

infrastructure/sql_scripts
🧱 1. DOMAIN DESIGN (DDD)
🔹 Core Domains
1. Rule Domain

Represents metadata and evaluation logic

2. Signal Domain

Represents collected metrics (normalized across engines)

3. Evaluation Domain

Executes rules against signals

4. Recommendation Domain

Produces actionable insights

🔹 Bounded Contexts
Context	Responsibility
RuleDefinition	Rule metadata
SignalCollection	Collect + normalize metrics
RuleEvaluation	Apply logic
ResultStorage	Persist outputs
API	Serve frontend
🗄️ 2. DATABASE REFACTOR (ENHANCE EXISTING TABLES)

You already have:

rules
rule_runs
rule_results_raw
rule_results_evaluated

👉 DO NOT DELETE — extend them

🔹 Modify rules table

Add:

ALTER TABLE ruleengine.rules
ADD COLUMN required_signals TEXT[],
ADD COLUMN eval_engine VARCHAR(20) DEFAULT 'go',
ADD COLUMN eval_expression JSONB,
ADD COLUMN recommendation_template JSONB,
ADD COLUMN rule_version INT DEFAULT 1;
🔹 Modify rule_results_evaluated
ALTER TABLE ruleengine.rule_results_evaluated
ADD COLUMN severity VARCHAR(20),
ADD COLUMN confidence NUMERIC,
ADD COLUMN context JSONB;
🔹 Create NEW: signals table
CREATE TABLE ruleengine.signals (
    server_id INT,
    signal_key TEXT,
    signal_value NUMERIC,
    collected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (server_id, signal_key, collected_at)
);
🔹 Create NEW: normalized_signal_snapshot (optional but recommended)
CREATE TABLE ruleengine.signal_snapshots (
    snapshot_id BIGSERIAL PRIMARY KEY,
    server_id INT,
    db_type VARCHAR(20),
    snapshot JSONB,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
⚙️ 3. CODE STRUCTURE (DDD + CLEAN ARCHITECTURE)
📁 Go Project Structure
internal/
  domain/
    rule/
      entity.go
      repository.go
    signal/
      entity.go
    evaluation/
      evaluator.go
    result/
      entity.go

  application/
    rule_engine/
      service.go

  infrastructure/
    db/
      postgres.go
    sql_scripts/

  interfaces/
    api/
      handler.go

tests/
  unit/
  integration/
🧪 4. TDD STRATEGY (MANDATORY)
🔴 Rule: WRITE TESTS FIRST
🔹 Example: MAXDOP Rule Test
func TestMaxDOP_HighParallelWaits(t *testing.T) {
    ctx := Context{
        "cpu_count": 32,
        "maxdop": 16,
        "parallel_wait_pct": 35,
    }

    result := EvaluateMaxDOP(ctx)

    assert.Equal(t, "HIGH", result.Severity)
    assert.True(t, result.Confidence > 0.8)
}
🔹 Coverage Requirements
Rule logic
Signal normalization
API response format
Edge cases
⚙️ 5. RULE ENGINE (GO IMPLEMENTATION)
🔹 Replace SQL-based logic

❌ Old:

evaluation_logic TEXT

✅ New:

type RuleEvaluator interface {
    Evaluate(ctx Context) Result
}
🔹 Context Object
type Context map[string]float64
🔹 Example Rule Implementation
func EvaluateMaxDOP(ctx Context) Result {
    recommended := math.Min(8, ctx["cpu_count"]/2)

    if ctx["maxdop"] > recommended {
        if ctx["parallel_wait_pct"] > 20 {
            return Result{
                Severity: "HIGH",
                Confidence: 0.9,
                Message: "Excessive parallel waits due to high MAXDOP",
            }
        }
        return Result{
            Severity: "MEDIUM",
            Confidence: 0.7,
        }
    }

    return Result{Severity: "OK", Confidence: 1.0}
}
📊 6. SIGNAL COLLECTION LAYER
🔹 Move ALL detection SQL to:
infrastructure/sql_scripts/
Example:
infrastructure/sql_scripts/postgres/
  memory.sql
  parallelism.sql
  vacuum.sql
🔹 Go Loader
func LoadSQLScript(name string) string {
    // read from infrastructure/sql_scripts
}
🔹 Output Example
{
  "cpu_count": 32,
  "shared_buffers": 128,
  "cache_hit_ratio": 0.92
}
🔄 7. RULE EXECUTION FLOW
1. Start rule_run
2. Collect signals (store in rule_results_raw)
3. Normalize into context
4. Evaluate rules in Go
5. Store in rule_results_evaluated
6. Return API response
🔌 8. API CONTRACT (DO NOT BREAK JS)
🔹 Backend Response
{
  "best_practices": [
    {
      "rule_id": "maxdop",
      "severity": "HIGH",
      "confidence": 0.92,
      "message": "...",
      "recommended": "...",
      "context": {...}
    }
  ]
}
🔹 JS Adapter Layer (IMPORTANT)

Update mapping:

function mapSeverity(sev) {
    if (sev === 'HIGH') return 'RED';
    if (sev === 'MEDIUM') return 'YELLOW';
    return 'GREEN';
}

👉 This ensures:
✅ No UI break
✅ No JS errors

🧠 9. BACKWARD COMPATIBILITY

Keep:

status
current_value
recommended

But derive from new model

🛡️ 10. SAFETY RULES
NEVER auto-run fix scripts
Always include:
risk level
rollback hint
🧪 11. TESTING JS (CRITICAL)
Ensure:
No undefined fields
Defensive checks:
const severity = check.severity || 'OK';
📦 12. MIGRATION PLAN
Phase 1
Add new columns
Keep old logic working
Phase 2
Move logic to Go
Keep SQL only for data collection
Phase 3
Introduce signals
Phase 4
Replace evaluation_logic fully
🏁 FINAL EXPECTED OUTCOME

After implementation:

BEFORE ❌
Static rules
SQL-driven logic
False positives
AFTER ✅
Context-aware engine
Go-based evaluation
PostgreSQL + SQL Server parity
Extensible system
🚀 BONUS (Highly Recommended)

Add later:

Rule confidence decay over time
Anomaly detection
Trend-based alerts
💬 Final Instruction

Implement this incrementally with:

Strict TDD
Clear domain boundaries
Zero frontend breakage