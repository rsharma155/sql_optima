# SQL Server Blank Table Analysis — May 20, 2026

This document details the investigation into why several SQL Server tables in the TimescaleDB monitoring schema are blank.

## Collector Mapping & Status

| schema.tablename | Go Repository (SQL Source) | Go Collector (Logic) | Active/Inactive | Data Type Match? | Issue Found? |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **public.sqlserver_query_plan_dim** | `repository/sqlserver_blocking.go` | `PulseService` | Active | Match | Only inserts if `wait_duration_ms > 5000`. |
| **public.sqlserver_blocking_incidents**| `repository/sqlserver_blocking.go` | `MsLocksBlockingCollector` | **Inactive** | Match | Collector disabled in `appserver.go`. |
| **public.sqlserver_blocking_pairs** | `repository/sqlserver_blocking.go` | `MsLocksBlockingCollector` | **Inactive** | Match | Collector disabled in `appserver.go`. |
| **public.sqlserver_log_shipping_health**| `repository/sqlserver_log_shipping.go`| `StartSqlServerHAReplicationCollector`| Active | Match | Requires active Log Shipping configuration. |
| **public.sqlserver_plan_instability** | `repository/sqlserver_query_analysis.go`| `StartQueryAnalysisCollector` | Active | Match | **Bug:** Missing `USE [db]` context in repository. |
| **public.sqlserver_query_regressions** | `repository/sqlserver_query_analysis.go`| `StartQueryAnalysisCollector` | Active | Match | **Bug:** Missing `USE [db]` context in repository. |
| **public.sqlserver_watched_query_events**| `repository/sqlserver_query_analysis.go`| `StartWatchedQueryCollector` | Active | Match | Depends on user marking queries as "watched". |
| **public.sqlserver_watched_query_snapshots**| `repository/sqlserver_query_analysis.go`| `StartWatchedQueryCollector` | Active | Match | Depends on user marking queries as "watched". |
| **public.sqlserver_watched_queries** | `repository/sqlserver_query_analysis.go`| `StartWatchedQueryCollector` | Active | Match | Depends on user marking queries as "watched". |
| **public.sqlserver_tempdb_top_consumers**| `repository/sqlserver_tempdb.go` | `StartSqlServerHealthCollector` | Active | Match | Requires active TempDB consumers. |
| **public.sqlserver_memory_grant_waiters**| `repository/sqlserver_enterprise_additions.go`| `StartSqlServerHealthCollector` | Active | Match | Requires active memory grant waits. |
| **public.sqlserver_procedure_stats** | `repository/sqlserver_phase2.go` | `StartSqlServerHealthCollector` | Active | Match | Slow-changing (5m interval). |
| **public.sqlserver_memory_history** | `repository/sqlserver_health_v2.go` | `StartSqlServerHealthCollector` | Active | Match | No issues. |
| **public.sqlserver_query_dictionary** | N/A | N/A | **Inactive** | N/A | Schema only; no Go implementation. |
| **public.sqlserver_scheduler_wg** | `repository/sqlserver_phase2.go` | `StartSqlServerHealthCollector` | Active | Match | Requires Resource Governor. |
| **public.sqlserver_cpu_scheduler_stats**| `repository/sqlserver_cpu.go` | `StartSqlServerHealthCollector` | Active | Match | No issues. |
| **monitor.sqlserver_ha_failover_events**| `repository/sqlserver_ag_health.go` | `StartSqlServerHAReplicationCollector`| Active | Match | Requires AlwaysOn + recent failover. |
| **public.sqlserver_risk_health** | `repository/sqlserver_health_v2.go` | `PulseService` | Active | Match | No issues. |
| **monitor.sqlserver_query_store_staging**| `repository/sqlserver_query_store.go` | `StartQueryStoreCollector` | Active | Match | Truncated after processing. |
| **monitor.sqlserver_query_store_interval**| `repository/sqlserver_query_store.go` | `StartQueryStoreCollector` | Active | Match | No issues. |
| **public.sqlserver_ag_health** | `repository/sqlserver_ag_health.go` | N/A | **Inactive** | N/A | **Deprecated**; use `monitor` schema. |
| **public.sqlserver_lock_history** | `repository/sqlserver_blocking.go` | `StartSqlServerHealthCollector` | Active | Match | No issues. |
| **staging.sqlserver_io_raw** | `repository/sqlserver_io.go` | `StartFileIOLatencyCollector` | Active | Match | No issues. |
| **public.sqlserver_session_snapshots** | `repository/sqlserver_collectors.go` | `PulseService` | Active | Match | No issues. |
| **public.sqlserver_cpu_history** | `repository/sqlserver_cpu.go` | `StartSqlServerHealthCollector` | Active | Match | No issues. |
| **public.sqlserver_query_identity_dim**| N/A | N/A | **Inactive** | N/A | Schema only; no Go implementation. |
| **public.sqlserver_query_classification_dim**| `repository/sqlserver_query_analysis.go`| `StartQueryAnalysisCollector` | Active | Match | No issues. |
| **public.sqlserver_session_snapshot** | N/A | N/A | **Inactive** | N/A | Unused; use `session_snapshots` (plural). |
| **public.sqlserver_query_stats_staging_v2**| `repository/sqlserver_queries.go` | `StartQueryStoreCollector` | Active | Match | Temporary staging table. |
| **public.sqlserver_blocking_locks** | `repository/sqlserver_blocking.go` | `PulseService` | Active | Match | No issues. |
| **public.sqlserver_deadlock_events** | `repository/sqlserver_blocking.go` | `StartXEFileTargetWorker` | **Inactive** | Match | **Stub:** Worker unimplemented. |
| **public.sqlserver_text_dim** | `repository/sqlserver_blocking.go` | `PulseService` | Active | Match | No issues. |

## Root Cause Analysis

### 1. Explicitly Disabled Collectors
In `backend/internal/appserver/appserver.go`, several specialized collectors are commented out. This includes:
- `MsLocksBlockingCollector` (responsible for `sqlserver_blocking_incidents` and `sqlserver_blocking_pairs`).
- `MsLocksBlockingCollector` was partially replaced by `PulseService`, but incident tracking was not migrated.

### 2. Missing Database Context (Query Store)
The collectors for `sqlserver_plan_instability` and `sqlserver_query_regressions` iterate through user databases but the SQL execution in `sqlserver_query_analysis.go` does not switch context.
- **Problem:** Queries run against `master`, but Query Store data is local to each database.
- **Fix Required:** Add `USE [%s];` to the repository SQL.

### 3. Stub Implementation (Deadlocks)
The `StartXEFileTargetWorker` is currently a skeleton function in `xe_file_target_worker.go`. It does not poll Extended Event files, which is why `sqlserver_deadlock_events` is empty.

### 4. Configuration Status
Collectors rely on the `optima_collector_configs` table. If a collector's `is_active` status is `false` in that table, `GetCollectorInterval` returns `0` and the loop never executes.

## Recommended Next Steps
1. **Enable Incident Tracking:** Re-enable or migrate `MsLocksBlockingCollector` logic.
2. **Fix Query Context:** Update `sqlserver_query_analysis.go` to use the correct database context.
3. **Implement XE Worker:** Complete the logic for `StartXEFileTargetWorker` to read deadlock graphs from `.xel` files.
4. **Clean Schema:** Drop or mark as internal the tables that have no Go implementation (`sqlserver_query_dictionary`, `sqlserver_query_identity_dim`).
