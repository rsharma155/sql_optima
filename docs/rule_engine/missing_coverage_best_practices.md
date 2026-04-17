# SQL Server Best-Practice Coverage Gaps

This document tracks the Epic 2.1 SQL Server rule-engine expansion work and the first batch we are actively wiring into the product.

## First Batch

Status: **complete**

| Rule ID | Rule Name | Status | Notes |
|---|---|---|---|
| `AGENT_FAILED_JOBS_009` | Failed Jobs Last 24h | ✅ Done | Uses `STRING_AGG` for sample evidence; tiered Warning/Critical at 3+ failures |
| `AGENT_DISABLED_JOBS_010` | Disabled Jobs Found | ✅ Done | Lists disabled job names as evidence |
| `DB_PAGE_VERIFY_006` | Page Verify CHECKSUM | ✅ Done | Evaluates all user databases, not just the connection database |
| `DB_TRUSTWORTHY_007` | Trustworthy Enabled | ✅ Done | Evaluates all user databases; returns affected DB evidence |
| `DB_CHAINING_008` | Cross DB Ownership Chaining | ✅ Done | Evaluates all user databases; returns affected DB evidence |
| `HA_AG_REPLICA_017` | AG Replica Health | ✅ Done | Queries `sys.dm_hadr_database_replica_states`; safe on non-AG instances (returns 0) |
| `PERF_MEMORY_GRANTS_014` | Memory Grants Pending | ✅ Done | Already seeded; included in first batch |
| `PERF_SINGLE_USE_PLANS_016` | Plan Cache Bloat | ✅ Done | Uses `sys.dm_exec_cached_plans`; evidence reflects real single-use compiled plans |

## Infrastructure Delivered With This Batch

- `ruleengine.rule_results_raw` table — stores raw per-rule evaluation payloads as JSONB
- `enrichDashboardEntries` path in `RulesHandler` — enriches dashboard entries with structured evidence and 8-point history
- `buildRawRulePayload` / `buildRuleEvidence` / `summariseEvidenceRows` helpers — normalise evidence from heterogeneous rule result shapes
- `renderRuleRemediation` — resolves placeholder tokens (`<RecommendedMB>`, `<Recommended>`, `<Value>`) in fix scripts
- `DashboardEntry` model extended with `Severity`, `Impact`, `Evidence`, `Remediation`, `History`
- Rules-engine UI drawer updated to show severity, impact, evidence, history, and remediation sections
- Migration `010_rule_engine_phase2_1.sql` — idempotent roll-forward for existing deployments

## Product Gaps This Batch Addresses

- Rule output now carries structured evidence, severity, impact, and remediation text.
- The rules UI shows recent status history so recurring SQL Server problems are visible.
- SQL Server database-scoped rules now evaluate across the instance, not only the login database.
- AG replica health is visible in the rules engine dashboard for Always On environments.

