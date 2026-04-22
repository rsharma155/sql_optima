Selected 5 Rules (Converted)

We’ll implement:

Max Server Memory (SQL Server)
MAXDOP (SQL Server)
Cost Threshold for Parallelism (SQL Server)
shared_buffers (PostgreSQL)
Autovacuum Health (PostgreSQL)
📁 Project Structure (Reference)
internal/
  domain/
    evaluation/
      context.go
      result.go
    rules/
      max_memory.go
      maxdop.go
      cost_threshold.go
      pg_shared_buffers.go
      pg_autovacuum.go

infrastructure/
  sql_scripts/
    sqlserver/
      memory.sql
      parallelism.sql
    postgres/
      shared_buffers.sql
      autovacuum.sql

tests/
  unit/
    maxdop_test.go
    memory_test.go
    pg_shared_buffers_test.go
🧠 1. Core Domain Models (Go)
📄 context.go
package evaluation

type Context map[string]float64
📄 result.go
package evaluation

type Result struct {
    RuleID       string
    Severity     string  // HIGH, MEDIUM, LOW, OK
    Confidence   float64
    Message      string
    Recommendation string
    Context      map[string]float64
}
⚙️ 2. RULE IMPLEMENTATIONS (GO)
✅ Rule 1: Max Server Memory
📄 max_memory.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateMaxMemory(ctx evaluation.Context) evaluation.Result {
    total := ctx["total_memory_gb"]
    maxMem := ctx["max_server_memory_gb"]

    osReserve := total * 0.2
    recommended := total - osReserve

    if maxMem > recommended {
        return evaluation.Result{
            RuleID:     "max_memory",
            Severity:   "HIGH",
            Confidence: 0.9,
            Message:    "Max server memory exceeds safe limit",
            Recommendation: "Reduce max_server_memory",
            Context: map[string]float64{
                "recommended": recommended,
            },
        }
    }

    return evaluation.Result{RuleID: "max_memory", Severity: "OK", Confidence: 1.0}
}
✅ Rule 2: MAXDOP
📄 maxdop.go
package rules

import (
    "math"
    "yourapp/internal/domain/evaluation"
)

func EvaluateMaxDOP(ctx evaluation.Context) evaluation.Result {
    cpu := ctx["cpu_count"]
    maxdop := ctx["maxdop"]
    waits := ctx["parallel_wait_pct"]

    recommended := math.Min(8, cpu/2)

    if maxdop > recommended {
        if waits > 20 {
            return evaluation.Result{
                RuleID:     "maxdop",
                Severity:   "HIGH",
                Confidence: 0.92,
                Message:    "High parallel waits due to high MAXDOP",
            }
        }
        return evaluation.Result{
            RuleID:     "maxdop",
            Severity:   "MEDIUM",
            Confidence: 0.75,
        }
    }

    return evaluation.Result{RuleID: "maxdop", Severity: "OK", Confidence: 1.0}
}
✅ Rule 3: Cost Threshold
📄 cost_threshold.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateCostThreshold(ctx evaluation.Context) evaluation.Result {
    cost := ctx["cost_threshold"]
    parallelUsage := ctx["parallel_usage_pct"]

    if cost < 20 && parallelUsage > 30 {
        return evaluation.Result{
            RuleID:     "cost_threshold",
            Severity:   "HIGH",
            Confidence: 0.85,
            Message:    "Low cost threshold causing excessive parallelism",
        }
    }

    if cost < 20 {
        return evaluation.Result{
            RuleID:     "cost_threshold",
            Severity:   "MEDIUM",
        }
    }

    return evaluation.Result{RuleID: "cost_threshold", Severity: "OK"}
}
✅ Rule 4: PostgreSQL shared_buffers
📄 pg_shared_buffers.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateSharedBuffers(ctx evaluation.Context) evaluation.Result {
    shared := ctx["shared_buffers_mb"]
    total := ctx["total_memory_mb"]

    recommended := total * 0.25

    if shared < recommended {
        return evaluation.Result{
            RuleID:     "pg_shared_buffers",
            Severity:   "MEDIUM",
            Confidence: 0.8,
            Message:    "shared_buffers too low",
            Context: map[string]float64{
                "recommended": recommended,
            },
        }
    }

    return evaluation.Result{RuleID: "pg_shared_buffers", Severity: "OK"}
}
✅ Rule 5: PostgreSQL Autovacuum
📄 pg_autovacuum.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateAutovacuum(ctx evaluation.Context) evaluation.Result {
    lag := ctx["autovacuum_lag"]

    if lag > 100000 {
        return evaluation.Result{
            RuleID:     "pg_autovacuum",
            Severity:   "HIGH",
            Confidence: 0.9,
            Message:    "Autovacuum lag is high → risk of bloat",
        }
    }

    return evaluation.Result{RuleID: "pg_autovacuum", Severity: "OK"}
}
🧪 3. TDD TESTS
📄 maxdop_test.go
package unit

import (
    "testing"
    "yourapp/internal/domain/evaluation"
    "yourapp/internal/domain/rules"
)

func TestMaxDOP_HighWaits(t *testing.T) {
    ctx := evaluation.Context{
        "cpu_count": 32,
        "maxdop": 16,
        "parallel_wait_pct": 40,
    }

    res := rules.EvaluateMaxDOP(ctx)

    if res.Severity != "HIGH" {
        t.Errorf("Expected HIGH, got %s", res.Severity)
    }
}
📄 pg_shared_buffers_test.go
func TestSharedBuffersLow(t *testing.T) {
    ctx := evaluation.Context{
        "shared_buffers_mb": 128,
        "total_memory_mb": 8192,
    }

    res := rules.EvaluateSharedBuffers(ctx)

    if res.Severity != "MEDIUM" {
        t.Errorf("Expected MEDIUM")
    }
}


🧠 🔥 Strategy (Do THIS, not naive conversion)
❌ What NOT to do
func Rule_123() { ... }
func Rule_124() { ... }

👉 This does NOT scale.

✅ What TO do

Build a hybrid evaluator system:

rules table (metadata)
        ↓
Go registry (rule handlers)
        ↓
shared evaluation framework
🧱 1. Rule Classification (CRITICAL STEP)

From your table (rules_tables.txt), your rules fall into 4 types:

Type	Columns Used	Example
CONFIG	detection_sql	MAXDOP
THRESHOLD	threshold_value	memory
EXPRESSION	evaluation_logic	custom
SCRIPT	fix_script	remediation
✅ Map them to Go:
DB Type	Go Strategy
simple threshold	generic evaluator
complex logic	custom function
SQL-based	signal collector
fix_script	metadata only
🧱 2. Build Rule Registry (Core Engine)
📄 rule_registry.go
package rules

import "yourapp/internal/domain/evaluation"

type RuleHandler func(ctx evaluation.Context, meta RuleMeta) evaluation.Result

var Registry = map[string]RuleHandler{
    "max_server_memory": EvaluateMaxMemory,
    "maxdop": EvaluateMaxDOP,
    "cost_threshold": EvaluateCostThreshold,
    "optimize_adhoc": EvaluateAdhoc,
    "instant_file_init": EvaluateIFI,
}
🧱 3. RuleMeta (from DB)
📄 rule_meta.go
package rules

type RuleMeta struct {
    RuleID          string
    Threshold       map[string]float64
    ComparisonType  string
    Recommended     string
}
⚙️ 4. Generic Evaluator (AUTO-CONVERT 70% RULES)

Most of your rules can be handled here.

📄 generic_evaluator.go
package rules

import "yourapp/internal/domain/evaluation"

func GenericThresholdEvaluator(ctx evaluation.Context, meta RuleMeta) evaluation.Result {
    value := ctx[meta.RuleID]
    threshold := meta.Threshold["value"]

    switch meta.ComparisonType {
    case "greater_than":
        if value > threshold {
            return evaluation.Result{
                RuleID: meta.RuleID,
                Severity: "HIGH",
                Message: "Value exceeds threshold",
            }
        }
    case "less_than":
        if value < threshold {
            return evaluation.Result{
                RuleID: meta.RuleID,
                Severity: "HIGH",
            }
        }
    }

    return evaluation.Result{RuleID: meta.RuleID, Severity: "OK"}
}
🔄 5. Convert YOUR RULES (Actual Mapping)

Now let’s convert your actual rule categories.

✅ Rule 1: max server memory
DB row → Go
func EvaluateMaxMemory(ctx evaluation.Context, meta RuleMeta) evaluation.Result {
    total := ctx["total_memory_gb"]
    maxMem := ctx["max_server_memory_gb"]

    reserve := total * 0.2
    recommended := total - reserve

    if maxMem > recommended {
        return evaluation.Result{
            RuleID: meta.RuleID,
            Severity: "HIGH",
            Confidence: 0.9,
            Message: "Max memory too high",
            Recommendation: "Reduce to safe level",
        }
    }

    return evaluation.Result{RuleID: meta.RuleID, Severity: "OK"}
}
✅ Rule 2: MAXDOP
func EvaluateMaxDOP(ctx evaluation.Context, meta RuleMeta) evaluation.Result {
    cpu := ctx["cpu_count"]
    maxdop := ctx["maxdop"]

    recommended := min(8, cpu/2)

    if maxdop > recommended {
        return evaluation.Result{
            RuleID: meta.RuleID,
            Severity: "MEDIUM",
            Message: "MAXDOP too high",
        }
    }

    return evaluation.Result{RuleID: meta.RuleID, Severity: "OK"}
}
✅ Rule 3: Cost Threshold
func EvaluateCostThreshold(ctx evaluation.Context, meta RuleMeta) evaluation.Result {
    cost := ctx["cost_threshold"]

    if cost < 20 {
        return evaluation.Result{
            RuleID: meta.RuleID,
            Severity: "MEDIUM",
            Message: "Cost threshold too low",
        }
    }

    return evaluation.Result{RuleID: meta.RuleID, Severity: "OK"}
}
✅ Rule 4: Optimize for AdHoc
func EvaluateAdhoc(ctx evaluation.Context, meta RuleMeta) evaluation.Result {
    val := ctx["optimize_for_adhoc"]

    if val == 0 {
        return evaluation.Result{
            RuleID: meta.RuleID,
            Severity: "LOW",
            Message: "Adhoc optimization disabled",
        }
    }

    return evaluation.Result{RuleID: meta.RuleID, Severity: "OK"}
}
✅ Rule 5: Instant File Init
func EvaluateIFI(ctx evaluation.Context, meta RuleMeta) evaluation.Result {
    val := ctx["instant_file_init"]

    if val == 0 {
        return evaluation.Result{
            RuleID: meta.RuleID,
            Severity: "MEDIUM",
            Message: "Instant File Initialization disabled",
        }
    }

    return evaluation.Result{RuleID: meta.RuleID, Severity: "OK"}
}
🔁 6. Dynamic Rule Execution (KEY)
📄 engine.go
func EvaluateAll(ctx evaluation.Context, metas []RuleMeta) []evaluation.Result {
    results := []evaluation.Result{}

    for _, meta := range metas {
        handler, ok := Registry[meta.RuleID]

        if ok {
            results = append(results, handler(ctx, meta))
        } else {
            results = append(results, GenericThresholdEvaluator(ctx, meta))
        }
    }

    return results
}
🧪 7. AUTO TEST GENERATION PATTERN
📄 rule_test_template.go
func TestRule_Generic(t *testing.T) {
    ctx := evaluation.Context{
        "maxdop": 16,
        "cpu_count": 8,
    }

    meta := RuleMeta{
        RuleID: "maxdop",
    }

    res := EvaluateMaxDOP(ctx, meta)

    if res.Severity != "MEDIUM" {
        t.Fail()
    }
}
📦 8. SQL → SIGNAL MAPPING (CRITICAL)

From your table:

detection_sql
detection_sql_pg

👉 Move ALL to:

infrastructure/sql_scripts/
Example mapping
Rule	SQL Output → Context
maxdop	maxdop
memory	total_memory, max_memory
adhoc	optimize_for_adhoc
🔌 9. API OUTPUT (NO JS BREAK)

Convert:

func ToAPI(res evaluation.Result) map[string]interface{} {
    return map[string]interface{}{
        "rule_id": res.RuleID,
        "severity": res.Severity,
        "status": mapSeverity(res.Severity), // JS safe
        "message": res.Message,
        "recommended": res.Recommendation,
    }
}
🏁 FINAL RESULT

After conversion:

✅ You now have:
Rule registry (extensible)
Generic evaluator (covers most rules)
Custom evaluators (only where needed)
Clean Go logic (no SQL hacks)
Metadata-driven behavior




I’ll give you production-ready PostgreSQL advanced rules for:

VACUUM health
Table / index bloat
Index efficiency & missing indexes

All aligned with your new architecture:

✅ Go evaluators
✅ SQL in infrastructure/sql_scripts
✅ Signal-based model
✅ TDD-ready
🧠 ⚠️ First: Reality Check

These rules are not simple thresholds.

They require:

Aggregation
Heuristics
Imperfect signals

👉 So we design them as:

“confidence-based advisory rules”, not absolute truths

🧱 1. SIGNAL COLLECTION (PostgreSQL)
📁 infrastructure/sql_scripts/postgres/vacuum_stats.sql
SELECT
    relname,
    n_dead_tup,
    n_live_tup,
    last_autovacuum,
    last_vacuum
FROM pg_stat_user_tables;
📁 postgres/table_bloat_estimate.sql
SELECT
    schemaname,
    relname,
    pg_total_relation_size(relid) AS total_size,
    pg_relation_size(relid) AS table_size
FROM pg_catalog.pg_statio_user_tables;

👉 (Simple version; real bloat estimation can be added later)

📁 postgres/index_usage.sql
SELECT
    relname AS table_name,
    indexrelname AS index_name,
    idx_scan,
    pg_relation_size(indexrelid) AS index_size
FROM pg_stat_user_indexes;
📁 postgres/cache_hit.sql
SELECT
    sum(heap_blks_hit) / nullif(sum(heap_blks_hit + heap_blks_read),0) AS cache_hit_ratio
FROM pg_statio_user_tables;
⚙️ 2. GO RULE IMPLEMENTATIONS
✅ Rule 1: VACUUM Health (Critical)
📄 pg_vacuum_health.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateVacuumHealth(ctx evaluation.Context) evaluation.Result {
    dead := ctx["dead_tuple_pct"]
    lastVacuumHours := ctx["last_vacuum_hours"]

    if dead > 20 && lastVacuumHours > 24 {
        return evaluation.Result{
            RuleID:     "pg_vacuum_health",
            Severity:   "HIGH",
            Confidence: 0.9,
            Message:    "High dead tuples with stale vacuum → bloat risk",
            Recommendation: "Tune autovacuum or run manual VACUUM",
        }
    }

    if dead > 10 {
        return evaluation.Result{
            RuleID:     "pg_vacuum_health",
            Severity:   "MEDIUM",
            Confidence: 0.7,
        }
    }

    return evaluation.Result{
        RuleID: "pg_vacuum_health",
        Severity: "OK",
    }
}
🧪 Test
func TestVacuumHighDeadTuples(t *testing.T) {
    ctx := evaluation.Context{
        "dead_tuple_pct": 25,
        "last_vacuum_hours": 48,
    }

    res := EvaluateVacuumHealth(ctx)

    if res.Severity != "HIGH" {
        t.Errorf("Expected HIGH")
    }
}
✅ Rule 2: Table Bloat Detection
📄 pg_table_bloat.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateTableBloat(ctx evaluation.Context) evaluation.Result {
    ratio := ctx["bloat_ratio"]

    if ratio > 1.5 {
        return evaluation.Result{
            RuleID:     "pg_table_bloat",
            Severity:   "HIGH",
            Confidence: 0.85,
            Message:    "Significant table bloat detected",
            Recommendation: "Consider VACUUM FULL or pg_repack",
        }
    }

    if ratio > 1.2 {
        return evaluation.Result{
            RuleID:     "pg_table_bloat",
            Severity:   "MEDIUM",
        }
    }

    return evaluation.Result{RuleID: "pg_table_bloat", Severity: "OK"}
}
✅ Rule 3: Index Bloat / Unused Indexes
📄 pg_index_efficiency.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateIndexUsage(ctx evaluation.Context) evaluation.Result {
    scans := ctx["idx_scan"]
    size := ctx["index_size_mb"]

    if scans == 0 && size > 100 {
        return evaluation.Result{
            RuleID:     "pg_unused_index",
            Severity:   "HIGH",
            Confidence: 0.9,
            Message:    "Large index never used",
            Recommendation: "Consider dropping unused index",
        }
    }

    if scans < 10 && size > 50 {
        return evaluation.Result{
            RuleID:     "pg_unused_index",
            Severity:   "MEDIUM",
        }
    }

    return evaluation.Result{RuleID: "pg_unused_index", Severity: "OK"}
}
✅ Rule 4: Cache Hit Ratio
📄 pg_cache_hit.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateCacheHit(ctx evaluation.Context) evaluation.Result {
    ratio := ctx["cache_hit_ratio"]

    if ratio < 0.90 {
        return evaluation.Result{
            RuleID:     "pg_cache_hit",
            Severity:   "HIGH",
            Message:    "Low cache hit ratio → disk I/O pressure",
            Recommendation: "Increase shared_buffers or optimize queries",
        }
    }

    if ratio < 0.95 {
        return evaluation.Result{
            RuleID:     "pg_cache_hit",
            Severity:   "MEDIUM",
        }
    }

    return evaluation.Result{RuleID: "pg_cache_hit", Severity: "OK"}
}
✅ Rule 5: Missing Index Heuristic (Simple)
📄 pg_missing_index.go
package rules

import "yourapp/internal/domain/evaluation"

func EvaluateMissingIndex(ctx evaluation.Context) evaluation.Result {
    seqScan := ctx["seq_scan_pct"]

    if seqScan > 70 {
        return evaluation.Result{
            RuleID:     "pg_missing_index",
            Severity:   "MEDIUM",
            Confidence: 0.6,
            Message:    "High sequential scan ratio → possible missing indexes",
        }
    }

    return evaluation.Result{RuleID: "pg_missing_index", Severity: "OK"}
}
🔄 3. SIGNAL NORMALIZATION (IMPORTANT)

Your Go layer must compute derived metrics:

ctx["dead_tuple_pct"] = dead / (live + dead) * 100
ctx["bloat_ratio"] = total_size / table_size
ctx["index_size_mb"] = size / 1024 / 1024
ctx["seq_scan_pct"] = seq_scan / (seq_scan + idx_scan) * 100
🔌 4. RULE REGISTRY UPDATE
var Registry = map[string]RuleHandler{
    "pg_vacuum_health": EvaluateVacuumHealth,
    "pg_table_bloat": EvaluateTableBloat,
    "pg_unused_index": EvaluateIndexUsage,
    "pg_cache_hit": EvaluateCacheHit,
    "pg_missing_index": EvaluateMissingIndex,
}
🧪 5. TEST COVERAGE YOU SHOULD ADD
High dead tuples + no vacuum
Huge index with zero scans
Cache hit < 90%
Bloat ratio > 1.5
⚠️ 6. Known Limitations (Be Honest in UI)

Add note in UI:

“Some recommendations are heuristic-based and should be validated before applying.”

🏁 FINAL RESULT

With these rules, your system now detects:

✅ Autovacuum problems
✅ Table bloat
✅ Index inefficiency
✅ Cache pressure
✅ Missing index patterns