# PostgreSQL Blank Table Analysis - May 20, 2026

## Overview
A comprehensive audit of the PostgreSQL monitoring schema was performed to identify why certain tables remain empty despite the application service being active. The analysis categorized 41 tables based on their collection source, activation status, and dependencies.

---

## 1. Core Metrics (Active but Empty)
These tables are managed by the main `MetricsService` collectors. They remain empty either because no events have occurred or because specific extensions are missing.

| Table Name | Collector / Go Script | Reason for Empty |
| :--- | :--- | :--- |
| `public.pg_ts_instance_snapshot` | `PgSnapshotCollector` | Stores the *latest* snapshot. If empty, check server connectivity. |
| `public.pg_ts_metrics` | `PgHealthRepo.LogMetric` | Replaced by `postgres_snapshot_metrics` (Wide-row optimization). |
| `monitor.pg_incident_feed_ts` | `CollectPgIncidents` | Only populated during active incidents (long queries, blocking). |
| `monitor.pg_session_snapshot` | `StartPgLocksBlockingCollector` | Populated during active lock/session scrapes. |
| `monitor.pg_blocking_pairs` | `StartPgLocksBlockingCollector` | Only has data when blocking is occurring. |
| `monitor.pg_blocking_incident` | `StartPgLocksBlockingCollector` | Only has data when a root blocker is identified. |
| `public.pg_ts_locks` | `PgTimescaleColl` | Stores historical lock events; empty if no contention detected. |

---

## 2. Extension-Dependent Tables
These tables require specific PostgreSQL extensions to be installed and loaded in `shared_preload_libraries`.

| Table Name | Required Extension | Status |
| :--- | :--- | :--- |
| `public.postgres_query_stats` | `pg_stat_statements` | Required for query-level telemetry. |
| `public.pgss_query_dim` | `pg_stat_statements` | Maps query IDs to SQL text. |
| `public.pgss_delta_1m` | `pg_stat_statements` | Stores 1-minute deltas of query execution. |
| `monitor.pg_query_wait_profile_ts` | `pg_stat_statements` | Requires wait-event sampling from the extension. |
| `public.pg_query_bucket_metrics` | `pg_stat_monitor` | Advanced alternative to `pg_stat_statements`. |
| `public.pg_collector_bucket_state`| `pg_stat_monitor` | Tracks collection offsets for `pg_stat_monitor`. |
| `public.postgres_vacuum_progress` | (Native PG 12+) | Pulls from `pg_stat_progress_vacuum`. |

---

## 3. External Agent Dependency (OS Metrics)
The following tables are **not** populated by the main Backend/API process. They require the `os_collector` binary to be running on the database host.

- `monitor.pg_os_host_instance`
- `monitor.pg_os_memory_samples`
- `monitor.pg_os_memory_pressure`
- `monitor.pg_os_process_memory`
- `monitor.pg_os_cpu_samples`

**Status:** INACTIVE unless the `os_collector` agent is deployed and configured with a valid API key.

---

## 4. Legacy or Redundant Tables
These tables exist in the schema but have been superseded by newer architectural designs.

- `public.pg_ts_stat_statements_delta`: Replaced by `pgss_delta_1m`.
- `public.pg_query_metrics`: Replaced by `postgres_snapshot_metrics` or `pgss_delta_1m`.
- `public.pg_instance`: Placeholder; `pg_instance_snapshot` is the active entity.

---

## 5. Architectural Placeholders (Future Features)
These tables are defined in `01_timescale_schema.sql` but currently have **no corresponding write logic** in the Go backend. They are reserved for future release phases.

- `public.postgres_throughput_metrics`
- `public.postgres_connection_stats`
- `public.postgres_replication_stats`
- `public.postgres_system_stats`
- `monitor.pg_failed_login_events` (Placeholder for future log parser)

---

## 6. Derived Views & Materialized Aggregates
These are not standard tables; they depend on data being present in their underlying base tables.

- `dashboard.postgres_active_incidents`: View on `staging.postgres_activity_locks_raw`.
- `public.postgres_checkpoint_summary`: Materialized View on `postgres_snapshot_metrics`.
- `public.postgres_archive_summary`: Materialized View on `postgres_snapshot_metrics`.

---

## Conclusion & Recommendations
1. **Enable Extensions:** Ensure `pg_stat_statements` is added to `shared_preload_libraries` in `postgresql.conf` and `CREATE EXTENSION pg_stat_statements` is run on each monitored DB.
2. **Deploy OS Collector:** Run the `os_collector` binary to populate the `monitor.pg_os_*` tables.
3. **Ignore Legacy:** Tables like `pg_ts_stat_statements_delta` should be considered deprecated.
4. **Baseline Activity:** If tables like `pg_blocking_pairs` are empty, it confirms a healthy, contention-free environment.
