# Intelligence Report Redesign Plan
**SQL Optima — v2 Health Intelligence Engine**
**Author:** Ravi Sharma  
**Date:** 2026-05-21  
**Branch:** `cold_storage_refine_collector`

---

## Table of Contents

1. [Current Design Audit](#1-current-design-audit)
2. [Defects & Bugs (15 Confirmed)](#2-defects--bugs-15-confirmed)
3. [Structural Shortcomings](#3-structural-shortcomings)
4. [What a DBA Needs in an Intelligence Report](#4-what-a-dba-needs-in-an-intelligence-report)
5. [What a Product Owner Needs](#5-what-a-product-owner-needs)
6. [New Architecture Design](#6-new-architecture-design)
7. [Data Sources & Queries — Mapped to Existing Tables](#7-data-sources--queries--mapped-to-existing-timescaledb-tables)
8. [Prediction & Forecasting Logic (New)](#8-prediction--forecasting-logic-new)
9. [New Report Structure — Section by Section](#9-new-report-structure--section-by-section)
10. [UI/UX Design](#10-uiux-design)
11. [Backend Code File Plan (with Metadata)](#11-backend-code-file-plan-with-metadata)
12. [Implementation Roadmap](#12-implementation-roadmap)

---

## 1. Current Design Audit

### 1.1 What Exists Today

The current system (`backend/internal/intel/`) implements a five-stage analysis pipeline:

```
TimescaleDB (14 queries) → DynamicThresholds → RuleEngine (dual system) 
→ AnomalyDetection → RiskScorer → Forecasting → HTML Report
```

**Files & Responsibilities:**

| File | Lines | Purpose |
|------|-------|---------|
| `analysis/engine.go` | 1,061 | Orchestrates all analysis stages |
| `analysis/thresholds.go` | 338 | Computes adaptive alert thresholds from hardware + history |
| `analysis/sqlserver_features.go` | ~400 | Feature library (25 features, NOT wired to engine) |
| `reports/generator.go` | 444 | Builds HTML template data from analysis output |
| `reports/template_data.go` | ~200 | Capacity tables, confidence scores, incident timeline |
| `templates/report_v3.html` | ~2,000 | Full HTML report with Plotly charts |
| `recommendations/generator.go` | ~300 | Maps rules to fix recommendations |
| `normalization/normalizer.go` | ~200 | Converts raw metrics to typed structs |
| `forecasting/engine.go` | ~230 | Linear regression + exponential smoothing |
| `anomaly/detector.go` | ~100 | Z-score anomaly detection on time series |
| `risk/scorer.go` | ~150 | Weighted 6-dimension risk score |
| `rule_engine/engine.go` | ~200 | YAML rule evaluation (second rule system) |
| `service/intelligence_report_service.go` | 556 | Data aggregation + service orchestration |

### 1.2 What Data Is Being Used

**TimescaleDB tables queried today:**

| Table | Metrics Used | Gap |
|-------|-------------|-----|
| `sqlserver_metrics` | avg_cpu_load, memory_usage, disk | Only latest row |
| `sqlserver_cpu_history` | sql_process, system_idle, other | Only latest row |
| `sqlserver_memory_metrics` | ple_seconds, grants_pending, os_available_mb | Only latest row |
| `sqlserver_risk_health` | blocking, tempdb_pct, ple, batch_requests | Latest + 7-day snapshots |
| `sqlserver_cpu_scheduler_stats` | runnable, worker_exhaustion | Only latest row |
| `sqlserver_server_properties` | cpu_count, ram, numa, sockets | Only latest row |
| `sqlserver_database_throughput` | read_latency_ms, write_latency_ms, tps | Only latest row |
| `sqlserver_wait_history` | disk_ms, blocking_ms, parallelism_ms, other_ms | Only latest row |
| `sqlserver_disk_history` | data_mb, log_mb, free_mb, delta_data_mb | Latest + 60-point series |
| `sqlserver_job_metrics` | failed_jobs_24h | Only latest row |
| `monitor.sqlserver_ha_replica_state` | secondary_lag, log_send_queue_kb | Last 5 min |
| `sqlserver_performance_debt_findings` | findings count by severity | Latest |

**Critical data NOT being used despite availability:**
- Hourly/daily CPU patterns for peak time detection
- Per-database size breakdown for growth projection
- Wait type names (only 4 broad categories used, not individual wait types)
- Query workload patterns (query count, duration trends)
- `sqlserver_cpu_history` system_idle trend for utilization classification

---

## 2. Defects & Bugs (15 Confirmed)

### DEFECT-1 — Risk Dimension Weights Do Not Normalize to 100%
**Location:** `analysis/engine.go:99–105`  
**Severity:** High  
**Issue:** Base risk dimensions use hardcoded multipliers (60 for CPU contribution, 40 for memory). The weighted sum can exceed 100 in high-stress scenarios because `math.Max()` is applied per dimension but the weighted formula does not cap the composite. Example: All 6 dimensions at 60+ → overall score > 100.  
**Fix:** Apply `min(100, weightedSum)` after computing the composite, and ensure per-dimension caps at 100 before weighting.

### DEFECT-2 — Missing Disk Data Silently Returns 0 Risk
**Location:** `analysis/engine.go:431–448` → `computeBaseCapacityRisk()`  
**Severity:** High  
**Issue:** When disk metrics are absent (collector never ran or columns empty), capacity risk returns 0.0, which the caller interprets as "low risk." A server with no disk telemetry appears safer than one with partial data. Missing data ≠ healthy data.  
**Fix:** Return a sentinel value (`-1` or a separate `DataMissing bool` flag) and propagate this through the risk score as a data gap penalty, not as a zero risk signal.

### DEFECT-3 — Risk Trend Uses Different Logic Than Live Risk
**Location:** `analysis/engine.go:484–505` → `calculateRiskTrend()`  
**Severity:** Medium  
**Issue:** Historical risk trend is recomputed from raw metric snapshots WITHOUT running rule evaluations. The live risk score includes rule triggers; the trend score does not. Result: the 7-day trend chart shows a different metric than the headline risk score, making the trend misleading.  
**Fix:** Persist the computed risk score to `intelreport.intel_snapshots` (new `intelreport` schema, see Section 7.0), then query that for trend data instead of recomputing from raw metrics.

### DEFECT-4 — Zero Default for Missing Metrics Hides Data Gaps
**Location:** `analysis/engine.go:555–578`  
**Severity:** Medium  
**Issue:** `getFloat(raw, primary, secondary, 0)` returns `0` when both primary and secondary keys are absent. `blocking_sessions=0` looks healthy even if the collector never ran. This is systematically optimistic under partial data collection.  
**Fix:** Use `getFloatOK(raw, key) (float64, bool)` that returns a `found bool`. Callers that get `found=false` skip the check and mark the relevant dimension as `unknown` rather than `healthy`.

### DEFECT-5 — Risk Scores Are Monotonically Increasing (Can Never Recover)
**Location:** `risk/scorer.go:33–51`  
**Severity:** High  
**Issue:** Every rule maps its severity to a score, then `math.Max(currentRisk, ruleSeverityScore)` ensures risk only ever rises. If blocking cleared up 2 hours ago, QueryRisk stays at 60 indefinitely because the max-assignment never decays. A server that recovered from an incident looks identical to one actively in crisis.  
**Fix:** Replace the `max()` assignment with a decay formula: `newRisk = max(ruleSeverityScore, currentRisk × 0.9)` per evaluation cycle, or compute risk purely from the current snapshot without carrying forward state from prior cycles.

### DEFECT-6 — Recommendations Are Hardcoded, Ignore Actual Hardware
**Location:** `recommendations/generator.go:125–135`  
**Severity:** Medium  
**Issue:** The recommendation "set MAXDOP to 8" is hardcoded. On a 4-core server, this is wrong (MAXDOP should be 4 or less). The recommendation generator does not receive the computed `DynamicThresholds` or `ServerConfig`, so it cannot produce context-aware advice.  
**Fix:** Pass `ServerConfig` and `DynamicThresholds` into `GenerateRecommendations()`. Use `computeOptimalMAXDOP()` (already exists in `engine.go`) to output the correct value.

### DEFECT-7 — Two Parallel Rule Systems With No Conflict Resolution
**Location:** `analysis/engine.go:552` + `rule_engine/engine.go:149`  
**Severity:** High  
**Issue:** Dynamic rules (Go code) and YAML rules (database table) both evaluate `avg_cpu_load`. A YAML rule may fire at `>80%` while the dynamic rule fires at a dynamically computed `>85%`. Both can fire simultaneously and create duplicate `CurrentIssues` entries with different messages and severities for the same root cause.  
**Fix:** Designate dynamic rules as the authoritative source for hardware-sensitive metrics. YAML rules should be limited to binary/configuration checks (e.g., "Query Store enabled?") that don't require threshold adaptation. Add explicit name-based deduplication that keeps the higher-severity result.

### DEFECT-8 — Constant-Zero Series Triggers False Anomaly
**Location:** `anomaly/detector.go:36–38`  
**Severity:** Low  
**Issue:** When σ < 1e-9 (perfectly constant series, e.g., `memory_grants_pending=0` for days), the detector falls back to an absolute difference check (`|value - mean| > 1e-6`). Any floating-point rounding (e.g., `0.000001`) triggers a false anomaly. This creates spurious alerts on stable servers.  
**Fix:** When σ < 1e-9 and the mean is also near-zero, return `isAnomaly=false`. Only flag when the absolute deviation exceeds a meaningful business floor (e.g., 1 grant pending when mean was 0).

### DEFECT-9 — Failure Threshold Inferred From Metric Name, Not Actual SLA
**Location:** `forecasting/engine.go:209–231`  
**Severity:** High  
**Issue:** The forecasting engine infers the "failure threshold" from the metric name string (e.g., `if strings.Contains(name, "disk") → 95% used`). This ignores: (a) actual configured thresholds, (b) the minimum required headroom (e.g., 20% for SQL Server log files), and (c) per-server SLA requirements.  
**Fix:** Accept a `map[string]float64` of `metricName → failureThreshold` computed from `DynamicThresholds`. Fall back to defaults only when the threshold map has no entry for a metric.

### DEFECT-10 — Low-Confidence Forecasts Presented Without Warning
**Location:** `forecasting/engine.go:114`  
**Severity:** Medium  
**Issue:** Forecasts with R² = 0.05 (essentially random noise) are still rendered as "here is the predicted value in 30 days" in the capacity planning section, with no visual indication of unreliability. Users may act on noise.  
**Fix:** Add a `ConfidenceBand bool` (show/hide CI ribbons on charts) and a textual `ForecastReliability` field: "Reliable" (R² > 0.7), "Indicative" (R² 0.3–0.7), "Unreliable — insufficient trend" (R² < 0.3). Only show day-level forecasts in UI when reliability ≥ "Indicative."

### DEFECT-11 — Phantom Default Memory Value (72.0) When Metric Missing
**Location:** `normalization/normalizer.go:38`  
**Severity:** Low  
**Issue:** When `memory_usage` is absent from raw data, a hardcoded default of `72.0` is substituted. The number `72.0` appears to have been chosen arbitrarily. This creates a phantom data point that influences anomaly detection, risk scoring, and the "current memory usage" headline in the report.  
**Fix:** Return `NaN` or a tagged `MissingValue` sentinel. The normalization layer should never invent a metric value; it should mark the metric as unobserved.

### DEFECT-12 — Daily Storage Growth Calculation Is Wrong (Off by 96×)
**Location:** `reports/template_data.go:114`  
**Severity:** High  
**Issue:**
```go
dailyStorageGrowthGB = deltaMB / 1024.0  // uses raw delta per collection cycle
```
`delta_data_mb` is the growth *since the last collection cycle* (~15 minutes). A single-cycle delta of 100 MB does not mean 100 MB/day; it means `100 × 96 = 9,600 MB/day` (96 cycles × 15 min = 24 h). The formula omits the frequency multiplier, making capacity projections 96× too conservative (growth looks tiny). For disk contractions it would be inverted (shrinkage looks larger).  
**Fix:**
```go
// Correct formula: aggregate delta over 24 hours of actual data, not a single cycle
SELECT SUM(delta_data_mb) FROM sqlserver_disk_history
WHERE server_id=$1 AND capture_timestamp > NOW() - INTERVAL '24 hours'
```
Use a true 24-hour aggregated delta as the daily growth rate. Fall back to `delta × cyclesPerDay` only when insufficient history exists.

### DEFECT-13 — Linear Growth Assumption on Spikey Metrics
**Location:** `reports/template_data.go:165–175`  
**Severity:** Medium  
**Issue:** The 30/60/90-day capacity projection is `current + (dailyGrowth × days)`, assuming perfectly linear growth. For log files (which spike on large transactions), this dramatically overstates capacity consumption. For OLTP data files (which have step-function growth from bulk loads), it understates the next growth event.  
**Fix:** Use the 7-day or 14-day average growth rate instead of a single-cycle delta. Add a `GrowthPattern` enum (`Steady | Stepped | Spikey`) derived from the variance of daily deltas, and display it in the UI so the DBA knows the forecast reliability.

### DEFECT-14 — Plotly Receives Null JSON Without Validation
**Location:** `templates/report_v3.html:156–162`, `safeJS()` function  
**Severity:** Low  
**Issue:** `safeJS()` only checks if the string starts with `{` or `[`. When a metric series is empty, `rawSeriesJSON()` returns the string `"null"`, which passes the brace check but causes Plotly to silently fail or render blank charts with no error message.  
**Fix:** Validate that the JSON string parses as a non-null, non-empty array before passing to Plotly. Provide a fallback empty dataset (`[]`) with a "No data available" annotation on the chart.

### DEFECT-15 — SQLServer Feature Library Is Defined But Never Used
**Location:** `analysis/sqlserver_features.go` (entire file)  
**Severity:** High  
**Issue:** The file defines 25 detailed feature specifications (Backup, Always On, Query Store, Resource Governor, etc.) with check descriptions, thresholds, and recommended values. The main `evaluateConfigChecks()` function in `engine.go` hardcodes the same logic independently. The feature library is a dead-code architectural artifact — updating it has no effect on the report.  
**Fix:** Wire `GetFeatureSpecs()` into the engine. Each feature's `Checks[]` should drive the config evaluation, and each feature's `Thresholds{}` should be the authoritative source rather than the hardcoded values in `engine.go:167–317`.

---

## 3. Structural Shortcomings

Beyond individual defects, the system has the following architectural problems:

### 3.1 No Peak Time Detection
The system has no concept of time-of-day or day-of-week patterns. It evaluates each metric in isolation without knowing whether a CPU reading of 75% is a normal Tuesday morning peak or an unusual Sunday midnight spike. This means:
- Alerts fire during known-busy windows (false positives)
- Real anomalies during off-hours are not distinguished from normal peaks

### 3.2 No Utilization Classification
There is no explicit "is this server over-utilized or under-utilized?" determination. A server running at 15% average CPU with 45 GB RAM allocated to SQL Server that has 2 TB free disk is massively under-utilized — an intelligence report should say so. Rightsizing recommendations are entirely absent.

### 3.3 Wait Type Analysis Is Superficial
The current intelligence service only reads `sqlserver_wait_history`, which stores 4 aggregate categories (disk, blocking, parallelism, other). However, the schema already has `sqlserver_wait_stats_delta` (per-type delta with `wait_category` tagging), `sqlserver_wait_stats_cumulative` (per-type cumulative), `hourly_wait_stats_baseline` and `sqlserver_cagg_wait_delta_1h` (continuous aggregates). The analysis engine is not using any of these. The top 10 named wait types (CXPACKET, LCK_M_X, PAGEIOLATCH_SH, SOS_SCHEDULER_YIELD, etc.) are the primary diagnostic signal for DBAs — they are already collected but ignored by the report.

### 3.4 Per-Database Growth Is Collected But Not Used
`sqlserver_disk_history` already has a `database_name` column and captures `data_mb`, `log_mb`, `delta_data_mb`, and `delta_log_mb` per database per collection cycle. The intelligence service does not query this per-database breakdown — it only reads a single aggregate row. A DBA needs to know "Which database grew 40 GB this month?" and this answer already exists in TimescaleDB.

### 3.5 DB Size Estimation Uses Incorrect Formula
The report does attempt disk capacity projections but uses a single-cycle `delta_data_mb` without aggregating 24h of deltas (DEFECT-12, off by 96×). The result is that growth rate appears nearly zero on slow-growing databases, and capacity breach dates are massively overestimated. "The database will be 500 GB in 60 days" is more useful than "growth rate: 0.1 MB/cycle.""

### 3.6 Risk Score Is Stateful in the Wrong Way
The `calculateRiskTrend()` re-derives risk from snapshots rather than using persisted scores. And the live risk can never decrease due to `math.Max()`. A server that cleared all incidents still shows historical risk bars at the same peak level. This destroys trust in the gauge over time.

### 3.7 Two Disconnected Rule Engines
The dual-engine system (Go dynamic rules + YAML rules in the database) has no reconciliation. Same metric → two evaluations → two alerts → user confusion. No single authoritative registry of active rules.

### 3.8 Confidence Score Logic Is Inverted
More dimensions firing → higher confidence. This is backwards. High confidence should come from having *sufficient clean data*, not from having *more problems detected*. A server with 6 critical issues should have a lower confidence score if the data is sparse, not a higher one.

### 3.9 No Historical Baseline Comparison
The report has a "Historical Comparison" tab but it only computes a 7-point delta. There is no baseline period (e.g., "compare this week to the same week last month") or seasonality-aware comparison.

### 3.10 Report Is Generated Fresh Every Time (No Persistence)
Each report generation re-runs all 14 TimescaleDB queries and the full analysis pipeline. There is no caching of results, no ability to view "yesterday's report," and no audit trail of how the server health evolved over the past month.

---

## 4. What a DBA Needs in an Intelligence Report

A DBA opening this report needs to answer these questions in under 5 minutes:

1. **Is this server in trouble right now?** → Headline health score with trend direction
2. **What is the server doing at peak load?** → CPU, memory, and I/O during the busiest hours
3. **When exactly are the busy windows?** → Hour-of-day heatmap showing average load by day/hour
4. **Is the server over-provisioned or under-provisioned?** → Utilization band classification
5. **What is the server waiting on?** → Top 10 named wait types with time-over-threshold
6. **How fast is the database growing?** → Per-database daily growth, projected size in 30/60/90 days
7. **When will we run out of disk?** → Projected breach date with confidence interval
8. **Are any queries regressed?** → Top 5 queries that got slower since last week
9. **Is the AG/replication healthy?** → Lag trend, failover readiness
10. **What should I fix first?** → Prioritized fix list with effort estimate and impact

---

## 5. What a Product Owner Needs

A non-technical product owner reviewing the report needs:

1. **A single number / traffic light** → "Server Health: 72/100 — Elevated"
2. **Plain English explanation** → "The database is approaching capacity and will need more disk within 60 days."
3. **Business impact framing** → "Users may experience slow response times during peak hours (9am–11am weekdays)."
4. **Trend direction** → "Performance has been degrading over the past 7 days."
5. **Top 3 action items** → Simple, non-technical labels: "Add disk capacity", "Schedule maintenance window", "Review query performance"
6. **A forward-looking view** → "At current growth, estimated database size in 90 days: 1.8 TB"
7. **Risk to the business** → "High availability configuration is healthy — no failover risk detected."
8. **Confidence in the data** → "Report based on 14 days of monitoring data (Full Coverage)"

---

## 6. New Architecture Design

### 6.1 Revised Pipeline

```
┌─────────────────────────────────────────────────────────────────┐
│                     DATA COLLECTION LAYER                        │
│  (collectors write to TimescaleDB every 15–30s)                 │
│  All required data already exists — no new collectors needed.   │
│  + NEW: intelreport.intel_snapshots (report persistence only)   │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│                   AGGREGATION LAYER (new)                        │
│  IntelligenceDataAggregator                                      │
│  ├── LoadPeakWindowAnalysis()     — hourly CPU/mem patterns      │
│  ├── LoadUtilizationProfile()     — 14-day percentile bands      │
│  ├── LoadTopWaitTypes()           — top 10 wait types + trend    │
│  ├── LoadDatabaseGrowthSeries()   — per-db size over 90 days     │
│  ├── LoadCapacityEstimates()      — true 24h growth rates        │
│  ├── LoadWaitHistory()            — 60-point time series         │
│  ├── LoadMetricHistory()          — CPU, PLE, blocking series    │
│  └── LoadHardwareProfile()        — server_properties            │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│                   ANALYSIS ENGINE v2 (redesigned)                │
│  Stage 1: UtilizationClassifier                                  │
│     → Computes avg/p50/p95/p99 CPU and memory                   │
│     → Classifies: Under-utilized | Optimal | Elevated | Stressed │
│     → Detects peak windows (top 3 busiest hour blocks)           │
│                                                                  │
│  Stage 2: UnifiedRuleEvaluator (single rule system)             │
│     → Hardware-adaptive dynamic rules (engine.go)               │
│     → Configuration rules (feature library, now wired)          │
│     → YAML rules for binary checks only                          │
│     → Result: []RuleTrigger with deduplication by name           │
│                                                                  │
│  Stage 3: WaitAnalyzer                                           │
│     → Ranks wait types by total wait time                        │
│     → Classifies into categories (CPU pressure, Lock, I/O, etc.) │
│     → Flags wait types that exceed baseline by >30%              │
│                                                                  │
│  Stage 4: RiskScorer v2                                          │
│     → Per-dimension risk with proper decay (no monotonic rise)   │
│     → Missing data → DataGap penalty, not zero risk              │
│     → Confidence from data completeness, not rule count          │
│                                                                  │
│  Stage 5: CapacityForecaster v2                                  │
│     → Uses 24h true daily growth (not single-cycle delta)        │
│     → Per-database growth projection                             │
│     → Growth pattern classification (Steady/Stepped/Spikey)     │
│     → Reliability tier (Reliable/Indicative/Unreliable)          │
│     → Breach date with confidence interval                       │
│                                                                  │
│  Stage 6: AnomalyDetector v2                                     │
│     → Time-of-day normalized anomaly (vs. baseline for same hour)│
│     → Handles constant-zero series correctly                     │
│     → Flags only when deviation is business-meaningful           │
│                                                                  │
│  Stage 7: NarrativeBuilder                                       │
│     → Generates executive summary (plain English)               │
│     → Generates DBA-level diagnostic summary                     │
│     → Business impact framing                                    │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│                   REPORT GENERATOR v2                            │
│  ├── Persists IntelligenceSnapshot to intelreport.intel_snapshots│
│  ├── Renders HTML report from report_v4.html template            │
│  ├── Returns structured JSON for API consumers                   │
│  └── Exposes /reports/history endpoint for timeline view         │
└─────────────────────────────────────────────────────────────────┘
```

### 6.2 New Go Package Structure

```
backend/internal/intel/
├── analysis/
│   ├── engine.go               — orchestrator (redesigned)
│   ├── thresholds.go           — dynamic thresholds (fixed)
│   ├── utilization_classifier.go  — NEW: over/under utilization
│   ├── peak_window_detector.go    — NEW: hour-of-day pattern analysis
│   ├── wait_analyzer.go           — NEW: wait type analysis
│   └── sqlserver_features.go   — wired to engine (fixed)
├── anomaly/
│   └── detector.go             — fixed constant-zero handling
├── config/
│   └── config.go               — engine configuration
├── forecasting/
│   ├── engine.go               — fixed threshold + reliability tier
│   └── capacity_forecaster.go  — NEW: per-database growth
├── normalization/
│   └── normalizer.go           — fixed phantom defaults
├── ontology/
│   └── models.go               — extended data models
├── recommendations/
│   └── generator.go            — fixed: receives ServerConfig
├── reports/
│   ├── generator.go            — redesigned template data prep
│   └── snapshot_store.go       — NEW: persist snapshots to DB
├── risk/
│   └── scorer.go               — fixed decay formula + data gaps
├── rule_engine/
│   └── engine.go               — unified rule system
├── templates/
│   └── report_v4.html          — new UI (see Section 10)
└── tests/
    └── reports_test.go         — updated tests
```

---

## 7. Data Sources & Queries — Mapped to Existing TimescaleDB Tables

> **Key correction from initial draft:** No new collectors are required. All data needed for
> the redesigned Intelligence Report already exists in TimescaleDB, populated by collectors
> already running. The schema uses `capture_timestamp` throughout — not `time`. The only new
> database object is a single persistence table (`intelreport.intel_snapshots`) in a new
> `intelreport` schema.

---

### 7.0 New Schema: `intelreport` (The Only New Database Object)

All Intelligence Report persistence lives in a dedicated schema. Add to `01_timescale_schema.sql`:

```sql
-- ============================================================================
-- INTELLIGENCE REPORT — Persistence Schema
-- Stores computed analysis snapshots for trend charts and historical comparison.
-- Only IntelligenceReportService writes here — no collector writes.
-- ============================================================================
CREATE SCHEMA IF NOT EXISTS intelreport;

CREATE TABLE IF NOT EXISTS intelreport.intel_snapshots (
    run_id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    server_id           UUID        NOT NULL,
    capture_timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    overall_risk        FLOAT8      NOT NULL DEFAULT 0,
    performance_risk    FLOAT8      NOT NULL DEFAULT 0,
    capacity_risk       FLOAT8      NOT NULL DEFAULT 0,
    availability_risk   FLOAT8      NOT NULL DEFAULT 0,
    replication_risk    FLOAT8      NOT NULL DEFAULT 0,
    maintenance_risk    FLOAT8      NOT NULL DEFAULT 0,
    query_risk          FLOAT8      NOT NULL DEFAULT 0,
    utilization_class   TEXT        NOT NULL DEFAULT 'unknown',
    -- Headline metrics captured at report time (avoids re-querying for trend charts)
    cpu_p50             FLOAT8      NOT NULL DEFAULT 0,
    cpu_p95             FLOAT8      NOT NULL DEFAULT 0,
    cpu_avg             FLOAT8      NOT NULL DEFAULT 0,
    mem_p95             FLOAT8      NOT NULL DEFAULT 0,
    ple_current         FLOAT8      NOT NULL DEFAULT 0,
    disk_used_pct       FLOAT8      NOT NULL DEFAULT 0,
    disk_days_remaining INT         NOT NULL DEFAULT 0,
    -- Report quality
    data_completeness   FLOAT8      NOT NULL DEFAULT 0,   -- 0.0–1.0
    data_coverage_days  INT         NOT NULL DEFAULT 0,
    rule_count_critical INT         NOT NULL DEFAULT 0,
    rule_count_high     INT         NOT NULL DEFAULT 0,
    -- Full report output
    report_json         JSONB,
    report_html         TEXT,
    PRIMARY KEY (server_id, capture_timestamp, run_id)
);

SELECT create_hypertable('intelreport.intel_snapshots', 'capture_timestamp',
    if_not_exists => TRUE);

ALTER TABLE intelreport.intel_snapshots SET (
    timescaledb.compress           = true,
    timescaledb.compress_segmentby = 'server_id',
    timescaledb.compress_orderby   = 'capture_timestamp DESC'
);
SELECT add_compression_policy('intelreport.intel_snapshots',
    INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('intelreport.intel_snapshots',
    INTERVAL '90 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_intel_snapshots_server_time
    ON intelreport.intel_snapshots (server_id, capture_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_intel_snapshots_run_id
    ON intelreport.intel_snapshots (run_id);
```

---

### 7.1 Existing Tables — Complete Source Map

Every Intelligence Report section below draws from tables **already populated by existing collectors**.
No new collectors or collection queries are needed.

| Report Section | Source Table(s) | Key Columns Used |
|----------------|----------------|-----------------|
| **Health KPIs** | `sqlserver_health_kpis_v2` | `sql_cpu_pct`, `mem_grants_pending`, `blocked_sessions`, `batch_requests`, `user_connections`, `edition`, `uptime_seconds` |
| **CPU Utilization Profile** | `sqlserver_metrics` | `avg_cpu_load`, `memory_usage`, `data_disk_mb`, `free_disk_mb` |
| **CPU Process Breakdown** | `sqlserver_cpu_history` | `sql_process`, `system_idle`, `other_process` |
| **Peak Window Heatmap** | `sqlserver_metrics` | `capture_timestamp` (group by hour+dow), `avg_cpu_load` |
| **Scheduler Pressure** | `sqlserver_cpu_scheduler_stats` | `avg_runnable_tasks_count`, `runnable_percent`, `worker_thread_exhaustion_warning`, `physical_memory_pressure_warning`, `available_physical_memory_kb` |
| **PLE Trend** | `sqlserver_memory_history` | `page_life_expectancy`, `capture_timestamp` |
| **Memory Metrics** | `sqlserver_memory_metrics` | `ple_seconds`, `memory_grants_pending`, `active_memory_grants`, `os_available_memory_mb`, `sort_warnings_per_sec`, `hash_warnings_per_sec`, `process_physical_low` |
| **Memory Grants** | `sqlserver_memory_grants` | `pending_grants`, `active_grants`, `granted_memory_mb` |
| **Memory Clerks** | `sqlserver_memory_clerks` | `clerk_name`, `pages_mb` |
| **Buffer Pool by DB** | `sqlserver_buffer_pool_db` | `database_name`, `buffer_mb` |
| **Memory Grant Waiters** | `sqlserver_memory_grant_waiters` | `session_id`, `requested_memory_kb`, `wait_time_ms`, `dop` |
| **Wait Analysis — Top Types** | `sqlserver_wait_stats_delta` | `wait_type`, `wait_category`, `delta_wait_ms`, `delta_resource_wait_ms`, `delta_waiting_tasks`, `restart_detected` |
| **Wait Analysis — Categories** | `sqlserver_wait_stats` | `wait_category`, `wait_time_ms`, `waiting_tasks` |
| **Wait Analysis — Hourly** | `sqlserver_cagg_wait_delta_1h` | Continuous aggregate: hourly wait deltas by type (already computed) |
| **Wait Analysis — Daily** | `sqlserver_cagg_wait_delta_1d` | Continuous aggregate: daily wait deltas (already computed) |
| **Wait Baseline (anomaly)** | `hourly_wait_stats_baseline` | `avg_disk_read_ms`, `avg_blocking_ms`, `avg_parallelism_ms` (existing cagg) |
| **Active Wait Sessions** | `sqlserver_active_wait_sessions` | `wait_type`, `wait_duration_ms`, `blocking_session_id` |
| **Latch Waits** | `sqlserver_latch_waits` | `wait_type`, `wait_time_ms`, `signal_wait_time_ms` |
| **I/O Latency — Per File** | `sqlserver_file_io` | `database_name`, `file_type`, `read_latency_ms`, `write_latency_ms`, `read_bytes_per_sec`, `write_bytes_per_sec` |
| **I/O Latency — Per DB** | `sqlserver_database_throughput` | `database_name`, `read_latency_ms`, `write_latency_ms`, `tps`, `batch_requests_per_sec` |
| **Perf Counters** | `sqlserver_perf_counters` | `counter_name`, `value_per_sec` |
| **Per-Database Growth** | `sqlserver_disk_history` | `database_name`, `data_mb`, `log_mb`, `delta_data_mb`, `delta_log_mb` (already per-DB) |
| **Instance Disk Capacity** | `sqlserver_metrics` | `data_disk_mb`, `log_disk_mb`, `free_disk_mb` |
| **TempDB File Usage** | `sqlserver_tempdb_files` | `file_name`, `file_type`, `allocated_mb`, `used_mb`, `used_percent` |
| **TempDB Top Consumers** | `sqlserver_tempdb_top_consumers` | `session_id`, `tempdb_mb`, `user_objects_mb`, `internal_objects_mb` |
| **Plan Cache** | `sqlserver_plan_cache` | `cache_type`, `size_mb`, `single_use_cache_pct`, `adhoc_cache_mb` |
| **Risk Health** | `sqlserver_risk_health` | `blocking_sessions`, `tempdb_used_percent`, `ple`, `batch_requests_per_sec`, `buffer_cache_hit_ratio`, `compilations_per_sec` |
| **Blocking Snapshots** | `sqlserver_blocking_snapshots` | `wait_type`, `wait_duration_ms`, `blocking_session_id`, `login_name`, `open_transaction_count` |
| **Blocking Incidents** | `sqlserver_blocking_incidents` | `started_at`, `ended_at`, `peak_blocked_sessions`, `root_blocker_query`, `status` |
| **Deadlocks** | `sqlserver_deadlock_events` | `database_name`, `victim_session_id`, `capture_timestamp` |
| **Query Regressions** | `sqlserver_query_regressions` | `regression_type`, `previous_avg`, `current_avg`, `percent_change`, `plan_changed` |
| **Plan Instability** | `sqlserver_plan_instability` | `query_hash`, `plan_count`, `last_execution_time` |
| **Top Queries** | `sqlserver_query_metrics_v2` | `database_name`, `total_cpu_ms`, `total_logical_reads`, `total_elapsed_ms`, `statement_text` |
| **Query Stats History** | `sqlserver_query_stats_history` | `cpu_delta_ms`, `reads_delta`, `exec_delta` |
| **Procedure Stats** | `sqlserver_procedure_stats` | `object_name`, `total_worker_time_ms`, `total_logical_reads`, `execution_count` |
| **Performance Debt** | `sqlserver_performance_debt_findings` | `section`, `finding_type`, `severity`, `title`, `impact_score`, `recommendation` |
| **HA Replica State** | `monitor.sqlserver_ha_replica_state` | `secondary_lag_seconds`, `log_send_queue_kb`, `redo_queue_kb`, `is_failover_ready`, `quorum_state_desc` |
| **RPO Trend** | `monitor.sqlserver_rpo_1min` | `rpo_seconds`, `avg_rpo_seconds`, `replica_count` (existing cagg) |
| **RTO Estimate** | `monitor.sqlserver_rto_1min` | `max_redo_queue_kb`, `estimated_rto_seconds` (existing cagg) |
| **Replication Latency** | `monitor.sqlserver_replication_latency` | `latency_seconds`, `undistributed_commands`, `delivery_rate_cmds_sec` |
| **HA Database State** | `monitor.sqlserver_ha_database_state` | `synchronization_state_desc`, `log_send_queue_kb`, `is_suspended` |
| **Job Health** | `sqlserver_job_metrics` | `failed_jobs_24h`, `running_jobs`, `total_jobs` |
| **Job Failures Detail** | `sqlserver_job_failures` | `job_name`, `last_run_outcome`, `step_name` |
| **Log Shipping** | `sqlserver_log_shipping_health` | `last_restore_date`, `restore_threshold_minutes`, `status`, `is_primary` |
| **DB Config / Catalog** | `sqlserver_database_catalog` | `is_auto_shrink_on`, `is_auto_close_on`, `recovery_model_desc`, `page_verify_option_desc`, `is_encrypted` |
| **Hardware Profile** | `sqlserver_server_properties` | `cpu_count`, `physical_memory_gb`, `numa_nodes`, `socket_count`, `max_workers_count`, `hyperthread_enabled` |
| **Report History Trend** | `intelreport.intel_snapshots` | `overall_risk`, `utilization_class`, `cpu_p95`, `disk_used_pct` (NEW — only new table) |

---

### 7.2 Query: CPU Peak Window Detection

**Source:** `sqlserver_metrics` (already collected every ~30s)

```sql
-- Hour-of-day × day-of-week CPU heatmap over 14 days.
-- Uses capture_timestamp — no new table needed.
SELECT
    EXTRACT(isodow FROM capture_timestamp)::INT  AS day_of_week,   -- 1=Mon, 7=Sun
    EXTRACT(hour  FROM capture_timestamp)::INT   AS hour_of_day,
    AVG(avg_cpu_load)                            AS avg_cpu_pct,
    MAX(avg_cpu_load)                            AS peak_cpu_pct,
    PERCENTILE_CONT(0.95) WITHIN GROUP
        (ORDER BY avg_cpu_load)                  AS p95_cpu_pct,
    AVG(memory_usage)                            AS avg_memory_pct,
    COUNT(*)                                     AS sample_count
FROM sqlserver_metrics
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '14 days'
GROUP BY day_of_week, hour_of_day
ORDER BY avg_cpu_pct DESC;
```

---

### 7.3 Query: Utilization Profile (14-Day Percentile Bands)

**Source:** `sqlserver_metrics` (existing)

```sql
SELECT
    PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY avg_cpu_load) AS cpu_p50,
    PERCENTILE_CONT(0.90) WITHIN GROUP (ORDER BY avg_cpu_load) AS cpu_p90,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY avg_cpu_load) AS cpu_p95,
    PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY avg_cpu_load) AS cpu_p99,
    AVG(avg_cpu_load)                                           AS cpu_avg,
    PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY memory_usage) AS mem_p50,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY memory_usage) AS mem_p95,
    AVG(memory_usage)                                           AS mem_avg,
    COUNT(*)                                                    AS sample_count
FROM sqlserver_metrics
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '14 days';
```

---

### 7.4 Query: Top Wait Types (24-Hour Window)

**Source:** `sqlserver_wait_stats_delta` (existing — delta-based per-type with category tagging)

```sql
SELECT
    wait_type,
    wait_category,
    SUM(delta_wait_ms)          AS total_wait_ms_24h,
    SUM(delta_resource_wait_ms) AS total_resource_wait_ms_24h,
    SUM(delta_waiting_tasks)    AS total_waiting_tasks_24h,
    ROUND(
        SUM(delta_wait_ms) * 100.0
        / NULLIF(SUM(SUM(delta_wait_ms)) OVER (), 0),
    2)                          AS pct_of_total_wait,
    RANK() OVER (ORDER BY SUM(delta_wait_ms) DESC) AS wait_rank
FROM sqlserver_wait_stats_delta
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '24 hours'
  AND restart_detected = FALSE
GROUP BY wait_type, wait_category
ORDER BY total_wait_ms_24h DESC
LIMIT 15;
```

---

### 7.5 Query: Wait Trend (7-Day Hourly, by Category)

**Source:** `sqlserver_cagg_wait_delta_1h` continuous aggregate (existing, already computed)

```sql
-- Uses the pre-computed continuous aggregate — no raw table scan.
SELECT
    bucket                    AS hour,
    wait_category,
    SUM(total_wait_ms)        AS wait_ms,
    SUM(total_waiting_tasks)  AS waiting_tasks
FROM sqlserver_cagg_wait_delta_1h
WHERE server_id = $1
  AND bucket > NOW() - INTERVAL '7 days'
GROUP BY hour, wait_category
ORDER BY hour ASC;
```

---

### 7.6 Query: Per-Database Growth (True Daily Rate)

**Source:** `sqlserver_disk_history` (existing — already has `database_name` column)

```sql
-- sqlserver_disk_history already tracks data_mb, log_mb, delta_data_mb per database.
-- Aggregate deltas per calendar day for true daily growth (not single-cycle delta).
SELECT
    database_name,
    DATE_TRUNC('day', capture_timestamp)     AS growth_day,
    SUM(delta_data_mb)                       AS daily_data_growth_mb,
    SUM(delta_log_mb)                        AS daily_log_growth_mb,
    MAX(data_mb)                             AS eod_data_mb,
    MAX(log_mb)                              AS eod_log_mb
FROM sqlserver_disk_history
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '30 days'
  AND delta_data_mb IS NOT NULL
  AND database_name IS NOT NULL
  AND database_name NOT IN ('master','model','msdb','tempdb')
GROUP BY database_name, growth_day
ORDER BY database_name, growth_day DESC;
```

---

### 7.7 Query: Instance-Level Disk Capacity Timeline

**Source:** `sqlserver_metrics` (existing)

```sql
SELECT
    capture_timestamp,
    data_disk_mb,
    log_disk_mb,
    free_disk_mb,
    (data_disk_mb + log_disk_mb)                AS total_used_mb,
    (data_disk_mb + log_disk_mb + free_disk_mb) AS total_disk_mb,
    ROUND(
        (data_disk_mb + log_disk_mb) * 100.0
        / NULLIF(data_disk_mb + log_disk_mb + free_disk_mb, 0),
    2)                                          AS used_pct
FROM sqlserver_metrics
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '30 days'
  AND data_disk_mb > 0
ORDER BY capture_timestamp ASC;
```

---

### 7.8 Query: Memory Pressure Timeline

**Source:** `sqlserver_memory_history` + `sqlserver_memory_metrics` (both existing)

```sql
-- PLE trend — sqlserver_memory_history is the dedicated PLE hypertable (90-day retention)
SELECT capture_timestamp, page_life_expectancy AS ple_seconds
FROM sqlserver_memory_history
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '7 days'
ORDER BY capture_timestamp ASC;

-- Memory grants pressure
SELECT
    capture_timestamp,
    memory_grants_pending,
    active_memory_grants,
    os_available_memory_mb,
    sort_warnings_per_sec,
    hash_warnings_per_sec,
    process_physical_low,
    process_virtual_low
FROM sqlserver_memory_metrics
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '24 hours'
ORDER BY capture_timestamp ASC;
```

---

### 7.9 Query: Blocking Analysis

**Source:** `sqlserver_blocking_incidents` + `sqlserver_blocking_snapshots` (both existing)

```sql
-- Recent blocking incidents (stateful table, not hypertable)
SELECT
    started_at,
    ended_at,
    EXTRACT(EPOCH FROM
        (COALESCE(ended_at, NOW()) - started_at))::INT AS duration_seconds,
    peak_blocked_sessions,
    status,
    root_blocker_query
FROM sqlserver_blocking_incidents
WHERE server_id = $1
  AND started_at > NOW() - INTERVAL '7 days'
ORDER BY started_at DESC
LIMIT 20;

-- Blocking wait type distribution (last 24h)
SELECT
    wait_type,
    COUNT(DISTINCT session_id) AS blocked_sessions,
    AVG(wait_duration_ms)      AS avg_wait_ms,
    MAX(wait_duration_ms)      AS max_wait_ms
FROM sqlserver_blocking_snapshots
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '24 hours'
  AND blocking_session_id > 0
GROUP BY wait_type
ORDER BY avg_wait_ms DESC;
```

---

### 7.10 Query: Query Regressions

**Source:** `sqlserver_query_regressions` + `sqlserver_plan_instability` (both existing)

```sql
SELECT
    database_name,
    query_text,
    regression_type,
    ROUND(previous_avg::NUMERIC, 2)    AS previous_avg_ms,
    ROUND(current_avg::NUMERIC, 2)     AS current_avg_ms,
    ROUND(percent_change::NUMERIC, 1)  AS pct_change,
    plan_changed,
    capture_timestamp
FROM sqlserver_query_regressions
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '7 days'
ORDER BY percent_change DESC
LIMIT 10;
```

---

### 7.11 Query: HA Health Summary

**Source:** `monitor.sqlserver_ha_replica_state` + `monitor.sqlserver_rpo_1min` (both existing)

```sql
-- Latest replica state per replica
SELECT DISTINCT ON (replica_server_name)
    ag_name,
    replica_server_name,
    role_desc,
    synchronization_state_desc,
    synchronization_health_desc,
    secondary_lag_seconds,
    log_send_queue_kb,
    redo_queue_kb,
    is_failover_ready,
    quorum_state_desc,
    capture_timestamp
FROM monitor.sqlserver_ha_replica_state
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '5 minutes'
ORDER BY replica_server_name, capture_timestamp DESC;

-- RPO trend (continuous aggregate — pre-computed, fast)
SELECT bucket, rpo_seconds, avg_rpo_seconds, replica_count
FROM monitor.sqlserver_rpo_1min
WHERE server_id = $1
  AND bucket > NOW() - INTERVAL '24 hours'
ORDER BY bucket ASC;
```

---

### 7.12 Query: Performance Debt Summary

**Source:** `sqlserver_performance_debt_findings` (existing)

```sql
SELECT
    section,
    severity,
    COUNT(*)             AS finding_count,
    AVG(impact_score)    AS avg_impact_score,
    MAX(capture_timestamp) AS last_seen
FROM sqlserver_performance_debt_findings
WHERE server_id = $1
  AND capture_timestamp = (
      SELECT MAX(capture_timestamp)
      FROM sqlserver_performance_debt_findings
      WHERE server_id = $1
  )
GROUP BY section, severity
ORDER BY
    CASE severity WHEN 'CRITICAL' THEN 1 WHEN 'WARNING' THEN 2 ELSE 3 END,
    finding_count DESC;
```

---

### 7.13 Query: Report History Trend

**Source:** `intelreport.intel_snapshots` (new — the only new table)

```sql
SELECT
    capture_timestamp,
    overall_risk,
    performance_risk,
    capacity_risk,
    availability_risk,
    utilization_class,
    cpu_p95,
    ple_current,
    disk_used_pct,
    rule_count_critical,
    rule_count_high
FROM intelreport.intel_snapshots
WHERE server_id = $1
  AND capture_timestamp > NOW() - INTERVAL '30 days'
ORDER BY capture_timestamp ASC;
```

---

## 8. Prediction & Forecasting Logic (New)

### 8.1 Utilization Classification

**Algorithm:**
```
Given: cpu_p50, cpu_p95, mem_p50, mem_p95 (over 14 days)

if cpu_p95 < 30 AND mem_p95 < 50:
    UtilizationClass = "Under-utilized"
    Message = "Server resources are significantly under-used. Consider consolidating workloads."
    
elif cpu_p95 < 60 AND mem_p95 < 75:
    UtilizationClass = "Optimal"
    Message = "Resource utilization is healthy with headroom for growth."
    
elif cpu_p95 < 85 OR mem_p95 < 90:
    UtilizationClass = "Elevated"
    Message = "Resources are under moderate pressure. Monitor for growth."
    
else:
    UtilizationClass = "Over-utilized"
    Message = "Server is under sustained resource pressure. Hardware upgrade or workload reduction recommended."
```

**Under-Utilization Recommendations:**
- If cpu_avg < 10% AND mem_p50 < 40%: Flag as "Candidate for VM/workload consolidation"
- If max_workers_count > `computeFormulaMaxWorkers()` × 2: Flag as "max worker threads over-configured"
- If total_ram_gb > current_peak_usage_gb × 3: Flag as "Memory over-provisioned"

### 8.2 Peak Window Detection

**Algorithm:**
1. Query 14-day hourly CPU matrix (day × hour → avg_cpu)
2. Rank all 168 hourly slots (7 days × 24 hours) by avg_cpu
3. Identify top contiguous cluster: the peak window is the largest block of consecutive hours with avg > p75
4. Output:
   - `PeakDays`: e.g., ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday"]
   - `PeakHours`: e.g., "09:00–11:00, 14:00–16:00"
   - `OffPeakWindow`: e.g., "22:00–06:00 on weekdays, all day Saturday"
   - `PeakCpuAvg`: e.g., 72%
   - `OffPeakCpuAvg`: e.g., 18%

**Anomaly contextualization:**
An anomaly detector MUST compare the observed value against the baseline for *the same hour slot*, not the overall mean. A 70% CPU reading at 10 AM Monday is normal; the same reading at 2 AM Sunday is anomalous.

```go
// Time-aware Z-score
func (d *AnomalyDetector) IsAnomaly(value float64, hourSlot int, daySlot int, baseline PeakMatrix) bool {
    slotMean := baseline[daySlot][hourSlot].Mean
    slotStd  := baseline[daySlot][hourSlot].StdDev
    if slotStd < 1.0 { // minimum meaningful std
        return math.Abs(value - slotMean) > 20.0 // 20 pp deviation
    }
    zScore := math.Abs(value - slotMean) / slotStd
    return zScore > 2.5
}
```

### 8.3 Capacity Forecasting v2

**Three-tier forecast approach:**

**Tier 1 — Short term (30 days):** Use 7-day true daily growth average. Best for planning near-term purchases.
```go
shortTermGrowthMBPerDay = sum(daily_data_growth_mb over 7 days) / 7
projected30d = currentSizeMB + shortTermGrowthMBPerDay * 30
```

**Tier 2 — Medium term (60–90 days):** Use linear regression over 30 days of daily snapshots. Include confidence interval (R² → CI width).
```go
// Linear regression on (day_index, eod_data_mb) pairs
slope, intercept, rSquared = FitLinearRegression(30-day daily snapshots)
projected90d = intercept + slope * (currentDay + 90)
ciWidth = (1 - rSquared) * slope * 90 * 0.5  // wider CI for noisier data
```

**Tier 3 — Estimated breach date:**
```go
requiredHeadroom = max(10% of totalDisk, 10 GB)
effectiveCapacity = totalDiskGB - requiredHeadroomGB
daysUntilBreach = (effectiveCapacity - currentUsedGB) / dailyGrowthGBPerDay
breachDate = today + daysUntilBreach days
```

**Reliability tier classification:**
| R² | Growth Variance | Tier |
|----|----------------|------|
| > 0.80 | Low | "Reliable" — show day-level precision |
| 0.50–0.80 | Medium | "Indicative" — show week-level precision |
| < 0.50 | High | "Unreliable" — show direction only, no date |

**Growth pattern detection:**
```go
// Coefficient of Variation of daily growth rates
cv = stdDev(dailyGrowthRates) / mean(dailyGrowthRates)
if cv < 0.15: GrowthPattern = "Steady"     // predictable, CI tight
if cv < 0.50: GrowthPattern = "Variable"   // moderate noise
else:         GrowthPattern = "Spikey"     // log or batch growth, CI wide
```

### 8.4 Wait Type Categorization

**Wait Category Mapping (SQL Server standard):**
| Wait Type Prefix/Name | Category | Implication |
|----------------------|----------|-------------|
| PAGEIOLATCH_*, IO_COMPLETION | I/O | Disk read/write bottleneck |
| LCK_M_*, LOCK_* | Lock Contention | Blocking/deadlock pressure |
| CXPACKET, CXCONSUMER | Parallelism | Excessive parallelism, MAXDOP too high |
| SOS_SCHEDULER_YIELD | CPU Saturation | CPU under pressure |
| RESOURCE_SEMAPHORE | Memory Grants | Memory grant queue pressure |
| WRITELOG | Log I/O | Transaction log bottleneck |
| ASYNC_NETWORK_IO | Network | Client not consuming results fast enough |
| THREADPOOL | Worker Exhaustion | Max worker threads too low |
| HADR_SYNC_COMMIT | AG Sync | Always On performance overhead |

**Wait score formula:**
```go
// Score each wait type 0–100 based on % of total wait time
totalWaitMs = sum(all wait types)
for each waitType:
    waitShare = waitType.WaitMs / totalWaitMs * 100
    waitScore = min(100, waitShare * 2)  // >50% of total = score 100
```

### 8.5 Risk Scoring v2

**Decay formula (replaces monotonic max):**
```go
// Per evaluation cycle, risk decays unless a rule sustains it
const decayFactor = 0.85  // 15% decay per cycle
for each dimension:
    newRisk = decayFactor * previousRisk
    for each triggeredRule in dimension:
        newRisk = max(newRisk, ruleSeverityScore)
```

**Confidence formula (from data completeness, not rule count):**
```go
// Count how many key metrics have non-missing, non-default values
populated = count of metrics where value != missingsentinel
total     = count of all expected metrics (currently ~25)
rawCoverage = populated / total

// Time coverage: reward having 14+ days of history
timeWeight = min(1.0, daysOfHistory / 14.0)

confidence = rawCoverage * 0.7 + timeWeight * 0.3
```

**Data gap penalty:**
```go
// If a dimension has >50% missing metrics, apply uncertainty penalty
if dimension.missingCount > dimension.totalMetrics * 0.5:
    dimension.risk = max(10, dimension.risk)  // floor of 10 = "unknown"
    dimension.dataGap = true
```

---

## 9. New Report Structure — Section by Section

### Section A — Executive Score Card (always visible, top of page)

| Element | Content |
|---------|---------|
| Health Score Gauge | 0–100, color-coded (0–40=red, 40–70=amber, 70–90=green, 90–100=bright green) |
| Utilization Badge | "UNDER-UTILIZED" / "OPTIMAL" / "ELEVATED" / "OVER-UTILIZED" with color |
| Trend Arrow | ▲ Improving / → Stable / ▼ Degrading (vs. previous report) |
| Peak Window | "Peak: Mon–Fri 09:00–11:00 (avg 72% CPU)" |
| Report Metadata | Server name, data coverage (days), generated at |
| Data Confidence | Bar: 0–100% with label (Full / Partial / Insufficient) |

### Section B — Resource Utilization Summary (tab 1)

**B.1 — CPU Analysis**
- 7-day CPU time series (line chart, moving average overlay)
- Hour-of-day heatmap (7 rows × 24 columns, colored by avg CPU %)
- CPU utilization distribution histogram (buckets: 0–20%, 20–40%, etc.)
- Peak vs. off-peak summary table:
  - Average CPU (peak window) | Average CPU (off-peak) | P95 overall

**B.2 — Memory Analysis**
- PLE trend (7-day line chart with threshold line at `computePLEFloor()`)
- Memory grants pending trend
- OS available memory trend
- Memory pressure events (times PLE dropped below threshold)

**B.3 — I/O Analysis**
- Read latency + write latency trend (dual-axis line chart)
- I/O throughput (MB/s) trend
- Latency distribution (% of time in: <1ms, 1–5ms, 5–10ms, >10ms)

### Section C — Wait Analysis (tab 2)

- **Top 10 Wait Types** — horizontal bar chart (sorted by total wait time)
- **Wait Category Breakdown** — pie/donut chart (CPU, Lock, I/O, Memory, Network, Other)
- **Wait Timeline** — stacked area chart of 4 main categories over 7 days
- **Wait Interpretation Cards** — for each top-3 wait type:
  - Wait name, description, category
  - Current rank vs. 7 days ago (rank change arrow)
  - Plain-English implication: "CXPACKET waits indicate parallel query overhead. Consider reviewing MAXDOP setting."
  - Linked recommendation

### Section D — Capacity Planning (tab 3)

**D.1 — Disk Capacity**
- Gauge: Current used % of total disk (with threshold markers at 80% and 90%)
- Growth trend chart (14-day line, projected 90-day forward)
- Estimated breach date (with reliability tier label)
- Growth pattern badge (Steady / Variable / Spikey)

**D.2 — Database Size Breakdown**
- Table: Per-database current size, 7-day growth, 30-day growth, projected 90-day size
- Sorted by 30-day growth (biggest growers first)
- Color-coded: red for databases on track to exceed 80% of instance capacity

**D.3 — Capacity Projection Summary**

| Metric | Current | +30 Days | +60 Days | +90 Days | Breach? |
|--------|---------|---------|---------|---------|---------|
| Total Data (GB) | 480 | 510 | 545 | 580 | — |
| Total Log (GB) | 85 | 90 | 95 | 102 | — |
| Free Disk (GB) | 435 | 405 | 370 | 335 | 240 days |
| TempDB (GB) | 42 | 44 | 46 | 48 | — |
| Total RAM Committed (GB) | 64 | 64 | 64 | 64 | — |

**D.4 — Rightsizing Assessment**

If under-utilized:
> "At current workload levels, this server could absorb 2–3× additional databases of similar size before reaching optimal utilization (70% CPU P95)."

If over-utilized:
> "At current growth rates, the server will require a CPU upgrade or workload migration within an estimated 45 days to maintain acceptable response times."

### Section E — Active Issues & Root Causes (tab 4)

**E.1 — Current Issues List**
- Filterable by severity (critical / high / medium / low)
- Each issue card shows:
  - Rule name (human-readable), severity badge
  - Current metric value vs. threshold
  - "First observed" timestamp
  - Linked recommendation
  - "Suppressed" toggle for acknowledged items

**E.2 — Root Cause Chains**
- Visual dependency graph: "Why did X happen?"
- Example chain: `High CXPACKET waits → MAXDOP = 0 (unlimited) → 8-core server creating 8 worker threads per query → CPU saturation`
- Text walkthrough below the graph

### Section F — Recommendations (tab 5)

**Three-column Kanban board:**

| Immediate (< 1 week) | Short-term (1–4 weeks) | Strategic (> 1 month) |
|---------------------|----------------------|----------------------|
| Hardware-context recommendations | Configuration recommendations | Architectural recommendations |

Each card shows:
- Action title
- Why it matters (1 sentence)
- Estimated effort (Low / Medium / High)
- Business impact (High / Medium / Low)
- Specific command or configuration value (e.g., "EXEC sp_configure 'max degree of parallelism', 4")

### Section G — HA & Replication (tab 6)
*(Only shown if HA data exists)*

- AG replica health table (primary + all secondaries)
- Log send queue trend + redo queue trend
- Secondary lag timeline
- Failover readiness indicator
- Backup recency table (last full, last log, last diff)

### Section H — Trend History (tab 7)

- 30-day historical health score line chart (from `intelreport.intel_snapshots`)
- Risk dimension breakdown over time (stacked area)
- Incident timeline (when rules triggered/cleared)

---

## 10. UI/UX Design

### 10.1 Layout Principles

- **Mobile-first responsive layout** — single column on phones, tabs on desktop
- **Progressive disclosure** — headlines first, drill-down on click
- **Consistent severity palette:**
  - Critical: `#DC2626` (red-600)
  - High: `#EA580C` (orange-600)
  - Medium: `#D97706` (amber-600)
  - Low: `#2563EB` (blue-600)
  - Healthy: `#16A34A` (green-600)
  - Unknown/Gap: `#6B7280` (gray-500)

### 10.2 Top Navigation

```
┌─────────────────────────────────────────────────────────────────────┐
│  SQL Optima Intelligence Report                                      │
│  PROD-SQL-01  ·  2026-05-21 14:32  ·  14-day coverage  ·  FULL DATA│
│  Health: [████████░░] 72/100  ELEVATED ▼ Degrading                  │
│  Peak: Mon–Fri 09:00–11:00 (avg CPU 74%)                           │
├──────────┬──────────┬──────────┬──────────┬──────────┬─────────────┤
│ Overview │ Waits    │ Capacity │ Issues   │ Actions  │  HA         │
└──────────┴──────────┴──────────┴──────────┴──────────┴─────────────┘
```

### 10.3 Executive Score Card (homepage / overview)

```
┌─────────────────┬─────────────────┬─────────────────┬─────────────┐
│    72 / 100     │  OVER-UTILIZED  │   3 Critical    │  ≈287 days  │
│  Health Score   │   Utilization   │ 2 High  8 Med   │  Disk Left  │
│  ▼ Degrading    │ CPU p95: 87%    │                 │  ~Sep 2026  │
└─────────────────┴─────────────────┴─────────────────┴─────────────┘
│  Performance  ████████░░ 78   Capacity  ████████░░ 72            │
│  Memory       ██████░░░░ 61   Replication ████░░░░░░ 42          │
│  Maintenance  ████░░░░░░ 38   Query      █████░░░░░ 52           │
└─────────────────────────────────────────────────────────────────────┘

NARRATIVE SUMMARY
─────────────────
"PROD-SQL-01 is under sustained CPU pressure, with P95 utilization at 87%
over the past 14 days. Peak load occurs on weekdays between 09:00–11:00
(avg 82% CPU). The top wait type is CXPACKET (38% of total waits), 
suggesting excessive parallelism — MAXDOP is currently set to 0 (unlimited)
on a server with 8 logical cores. Disk capacity is adequate with an estimated
287 days of headroom at the current 1.1 GB/day growth rate. Always On
replica is healthy with lag < 5 seconds."
```

### 10.4 CPU Heatmap (Resource Overview tab)

```
Hour:  00 01 02 03 04 05 06 07 08 09 10 11 12 13 14 15 16 17 18 19 20 21 22 23
Mon   [  ][  ][  ][  ][  ][  ][░░][░░][██][██][██][██][░░][░░][██][██][██][░░][  ][  ][  ][  ][  ][  ]
Tue   [  ][  ][  ][  ][  ][  ][░░][░░][██][██][██][██][░░][░░][██][██][██][░░][  ][  ][  ][  ][  ][  ]
Wed   [  ][  ][  ][  ][  ][  ][░░][░░][██][██][██][██][░░][░░][██][██][██][░░][  ][  ][  ][  ][  ][  ]
Thu   [  ][  ][  ][  ][  ][  ][░░][░░][██][██][██][██][░░][░░][██][██][██][░░][  ][  ][  ][  ][  ][  ]
Fri   [  ][  ][  ][  ][  ][  ][░░][░░][██][██][██][██][░░][░░][██][██][████][░░][  ][  ][  ][  ][  ][  ]
Sat   [  ][  ][  ][  ][  ][  ][  ][  ][░░][░░][░░][░░][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ]
Sun   [  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ][  ]

Legend: [  ] < 20%   [░░] 20–60%   [██] 60–85%   [██] > 85% (critical)
```

### 10.5 Wait Analysis Card

```
TOP WAIT TYPES (last 24 hours)
─────────────────────────────────────────────────────────────────
 1  CXPACKET          ███████████████████████░░░░░  38% │ ↑ rank 1
    Parallelism overhead — review MAXDOP setting
    
 2  PAGEIOLATCH_SH    ████████████░░░░░░░░░░░░░░░░  24% │ ↑ rank 2
    Disk read I/O wait — check disk latency and buffer pool size
    
 3  LCK_M_X           ██████░░░░░░░░░░░░░░░░░░░░░░  12% │ ↓ rank 5
    Exclusive lock contention — investigate blocking chains
    
 4  WRITELOG          ████░░░░░░░░░░░░░░░░░░░░░░░░   8% │ → rank 4
    Transaction log I/O — check log disk speed and VLF count
    
 5  SOS_SCHEDULER_YIELD ██░░░░░░░░░░░░░░░░░░░░░░░░   4% │ new
    CPU scheduler pressure — server may need more CPU capacity
```

### 10.6 Capacity Projection Cards

```
DISK CAPACITY                          DATABASE SIZE (Top Growers)
─────────────────────                  ─────────────────────────────────────────
Used: 480 GB / 960 GB (50%)           Database           Now    +30d   +90d    
                                       ──────────────────────────────────────────
[▓▓▓▓▓░░░░░]                          ORDERS_DB         210GB   225GB  255GB ▲
                                       ANALYTICS_DB      145GB   158GB  184GB ▲
Growth: 1.1 GB/day (STEADY)           FINANCE_DB         82GB    83GB   85GB →
Pattern: Reliable forecast             ARCHIVE_DB         43GB    43GB   44GB →
                                       
Projected:                            Estimated total DB size in 90 days:
  +30 days:  513 GB  (53%)                              ≈ 482 GB (+16%)
  +60 days:  546 GB  (57%)
  +90 days:  579 GB  (60%)           CAUTION: ORDERS_DB growing 500 MB/day —
                                      at this rate will exceed 500 GB in ~180 days
Disk breach (at 90%): ≈ Sep 2026
Confidence: HIGH (R² = 0.89)
```

---

## 11. Backend Code File Plan (with Metadata)

Every file in `backend/internal/intel/` requires a package-level doc comment. Below is the standard format to apply and a description of each file's purpose:

### Standard File Header Format

```go
// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file implements [specific responsibility].
//
// Design context:
//   - [Key design decision or constraint]
//   - [Dependency on other packages or external systems]
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT
```

### File-by-File Metadata Plan

| File | Package Comment Purpose |
|------|------------------------|
| `analysis/engine.go` | Orchestrates the 7-stage health intelligence pipeline. Entry point: `Analyze()`. Coordinates threshold computation, rule evaluation, anomaly detection, risk scoring, forecasting, and narrative generation. |
| `analysis/thresholds.go` | Computes hardware-adaptive alert thresholds from server configuration and historical metric percentiles. All thresholds are recomputed fresh per analysis run. |
| `analysis/utilization_classifier.go` | NEW. Classifies server utilization as Under-utilized / Optimal / Elevated / Over-utilized based on 14-day CPU and memory percentile bands. Also identifies peak hour windows. |
| `analysis/peak_window_detector.go` | NEW. Detects the busiest and quietest periods using a 14-day hour-of-day CPU matrix. Output drives time-aware anomaly detection and the heatmap UI section. |
| `analysis/wait_analyzer.go` | NEW. Ranks and categorizes SQL Server wait types from `sqlserver_wait_stats_delta` (per-type delta) and `sqlserver_waiting_tasks` (active waits). Maps individual wait types to DBA-meaningful categories (CPU, Lock, I/O, Memory, Network). |
| `analysis/sqlserver_features.go` | Defines 25 SQL Server feature specifications (backup, AG, Query Store, etc.) with thresholds, recommended values, and common issues. These specs are consumed by `evaluateConfigChecks()` in `engine.go`. |
| `anomaly/detector.go` | Detects statistical anomalies in metric time series using Z-score analysis. Supports both time-naive (overall mean) and time-aware (per-hour-slot baseline) detection. |
| `config/config.go` | Configuration for the intelligence engine: risk weights, anomaly sensitivity, forecast horizon, and data-gap penalties. Reads from environment variables with safe defaults. |
| `forecasting/engine.go` | Implements linear regression and exponential smoothing forecasting. Returns typed `ForecastResult` with reliability tier (Reliable/Indicative/Unreliable) and confidence intervals. |
| `forecasting/capacity_forecaster.go` | NEW. Per-database and instance-level disk capacity forecasting. Computes true 24h daily growth from aggregated deltas, classifies growth patterns (Steady/Variable/Spikey), and projects breach dates. |
| `normalization/normalizer.go` | Converts the raw `map[string]interface{}` metric snapshot into typed `NormalizedSystem` structs. Returns `MissingValue` sentinels rather than phantom defaults when metrics are absent. |
| `ontology/models.go` | All shared data model types: `IntelligenceReportResponse`, `RuleTriggerResult`, `ForecastResult`, `UtilizationProfile`, `PeakWindow`, `WaitAnalysis`, `DatabaseGrowth`, `ServerConfig`, `DynamicThresholds`. |
| `recommendations/generator.go` | Maps triggered rule names to prioritized, hardware-context-aware action items. Receives `ServerConfig` and `DynamicThresholds` to produce specific values (e.g., correct MAXDOP for the actual CPU count). |
| `reports/generator.go` | Builds the `TemplateData` struct from `IntelligenceReportResponse` and renders the HTML report using `templates/report_v4.html`. Also delegates to `snapshot_store.go` to persist the result. |
| `reports/snapshot_store.go` | NEW. Persists computed risk scores and full report JSON/HTML to `intelreport.intel_snapshots` so historical trend charts do not require recomputing from raw metrics. |
| `risk/scorer.go` | Computes the six risk dimensions (performance, capacity, availability, replication, maintenance, query) using a decay-based formula. Data gaps inflate the uncertainty floor rather than silently scoring as zero. |
| `rule_engine/engine.go` | Evaluates YAML-defined rules from the `ruleengine.rules` table against the raw metric snapshot. Restricted to binary/configuration checks; metric-adaptive rules live in `analysis/engine.go`. |
| `schema_parser/` | Parses YAML rule pack files into typed `RuleDefinition` structs. Used at startup to load rule packs into memory. |
| `templates/report_v4.html` | Jinja2-style Go HTML template for the redesigned intelligence report. Sections: Executive Scorecard, Resource Utilization, Wait Analysis, Capacity Planning, Issues & Root Causes, Recommendations, HA Status, Trend History. |
| `tests/reports_test.go` | Integration tests for the full analysis pipeline. Tests cover: missing data handling, risk decay, utilization classification, peak window detection, capacity forecast reliability tiers, and HTML template rendering. |
| `utils/stats.go` | Statistical utility functions: `FitLinearRegression`, `Percentile`, `Mean`, `StdDev`, `ComputeMovingAverage`. Shared across forecasting, anomaly detection, and threshold computation. |

---

## 12. Implementation Roadmap

### Phase 0 — Foundation Fixes (Week 1) ← Start Here

Fix critical defects before building new features. These are the minimum viable correctness changes.

1. **Fix DEFECT-12** (storage growth calculation): Change the daily growth query to aggregate 24h deltas from `sqlserver_disk_history` instead of using a single cycle delta.
2. **Fix DEFECT-5** (monotonic risk): Replace `math.Max` in `scorer.go` with the decay formula.
3. **Fix DEFECT-2** (missing disk = 0 risk): Add `DataGap bool` to risk dimensions; when data is missing, set risk floor to 10 and set `DataGap=true`.
4. **Fix DEFECT-1** (weight normalization): Apply `min(100, weightedSum)` after composite calculation; ensure per-dimension values are capped at 100 before weighting.
5. **Fix DEFECT-4** (zero default): Introduce `getFloatOK(raw, key) (float64, bool)` and convert all callers.
6. **Fix DEFECT-11** (phantom 72.0 memory): Remove hardcoded default; return `MissingValue` sentinel.
7. **Add file header metadata** to all files in `backend/internal/intel/` using the standard format above.

### Phase 1 — Schema & Data Layer (Week 2)

8. Add `intelreport` schema and `intelreport.intel_snapshots` table to `01_timescale_schema.sql` (see Section 7.0) — the only new schema change required
9. Update `intelligence_report_service.go` with the 5 new aggregation queries mapped in Section 7:
   - Peak window heatmap (from `sqlserver_metrics`)
   - Utilization profile percentiles (from `sqlserver_metrics`)
   - Per-database growth (from `sqlserver_disk_history` — already has `database_name`)
   - Top wait types (from `sqlserver_wait_stats_delta` — already delta-based per type)
   - Wait trend hourly (from `sqlserver_cagg_wait_delta_1h` — already a continuous aggregate)
10. Add `reports/snapshot_store.go` to persist results to `intelreport.intel_snapshots`

### Phase 2 — New Analysis Modules (Week 3)

14. Implement `analysis/utilization_classifier.go` (Section 6.2, 8.1)
15. Implement `analysis/peak_window_detector.go` (Section 8.2)
16. Implement `analysis/wait_analyzer.go` (Section 8.4)
17. Implement `forecasting/capacity_forecaster.go` (Section 8.3)
18. Fix DEFECT-15: Wire `sqlserver_features.go` into `evaluateConfigChecks()`
19. Fix DEFECT-7: Unify rule systems; YAML rules restricted to binary checks only
20. Fix DEFECT-9: Pass `DynamicThresholds` map into `forecasting/engine.go`
21. Fix DEFECT-6: Pass `ServerConfig` into `recommendations/generator.go`
22. Fix DEFECT-10: Add `ForecastReliability` tier to all `ForecastResult` objects
23. Implement `reports/snapshot_store.go` (persist to `intelreport.intel_snapshots`)
24. Fix confidence score formula in `risk/scorer.go` (data completeness, not rule count)

### Phase 3 — Report Redesign (Week 4)

25. Redesign `templates/report_v4.html`:
    - Executive scorecard with health gauge, utilization badge, peak window
    - CPU heatmap (7×24 grid using Plotly heatmap)
    - Wait types horizontal bar chart + category donut
    - Per-database growth table
    - Capacity breach projection with reliability tier and confidence band
    - Recommendations Kanban with hardware-specific values
    - Utilization classification card with rightsizing guidance
    - HA status section (conditional)
    - Historical trend chart (from `intelreport.intel_snapshots`)
26. Fix DEFECT-14: Validate series JSON before passing to Plotly; add empty-array fallback
27. Fix DEFECT-8: Handle constant-zero series in anomaly detector
28. Fix DEFECT-3: Populate trend from `intelreport.intel_snapshots` table
29. Update `reports/generator.go` to populate new `TemplateData` fields
30. Update `reports/template_data.go` to use corrected daily growth calculation

### Phase 4 — Tests & Validation (Week 5)

31. Update `tests/reports_test.go` to cover all new modules
32. Add test cases for:
    - Utilization classification boundary conditions
    - Peak window detection with sparse data
    - Capacity forecast reliability tiers
    - Risk decay formula
    - Wait type categorization
    - Missing data → DataGap, not zero risk
    - Time-aware anomaly detection (same value, different hour slot)
33. Run full end-to-end test with sample data against redesigned report
34. Performance test: ensure all 8 new queries complete within 500ms on TimescaleDB

### Phase 5 — Frontend Integration (Week 6)

35. Update `frontend/pages/sqlserver_intelligence_report.html`:
    - Remove "work in progress" disclaimer
    - Add report history selector (view past reports from `intelreport.intel_snapshots`)
    - Add loading skeleton for each section (not a single full-page spinner)
    - Add "Refresh" button with cooldown (max 1 report per 5 minutes)
    - Link "View in Intelligence Report" from dashboard alert cards

---

## Appendix: Key Formulas Reference

| Formula | Correct Implementation |
|---------|----------------------|
| PLE Floor | `max(300, (bufferPoolGB/4) × 300)` |
| Optimal MAXDOP | See `computeOptimalMAXDOP()` in `analysis/engine.go` |
| OS Memory Headroom | `max(4, min(totalRAMGB × 0.1, 16))` GB |
| Daily Disk Growth | `SUM(delta_data_mb) FROM last 24h` |
| Disk Breach Days | `(totalDiskGB - usedDiskGB - headroomGB) / dailyGrowthGBPerDay` |
| CPU Utilization Band | `if p95 < 30%: under-utilized; < 60%: optimal; < 85%: elevated; > 85%: over-stressed` |
| Wait Score | `waitTypePct × 2` (capped at 100) |
| Forecast Confidence | `dataCompleteness × 0.7 + timeWeightedCoverage × 0.3` |
| Risk Decay | `max(ruleSeverityScore, previousRisk × 0.85)` per cycle |
| Growth Pattern | CV < 0.15 = Steady; < 0.50 = Variable; else Spikey |
