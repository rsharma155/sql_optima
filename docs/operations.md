# Operations Guide

<!-- Author: Ravi Sharma -->
<!-- Copyright (c) 2026 Ravi Sharma -->
<!-- SPDX-License-Identifier: MIT -->

Operator notes for production deployments, including **P2-7** in-memory state behavior.

---

## API restarts and metric deltas (P2-7)

Several subsystems keep **in-process state** to compute deltas between collection cycles. An API process restart clears that memory. TimescaleDB historical data is unaffected; **the first collection cycle after restart** may show skewed or zero deltas until a second baseline snapshot exists.

| Subsystem | In-memory state | Effect after restart |
|-----------|-----------------|----------------------|
| **Query V2 pipeline** (always enabled) | Per-query hash snapshots for SQL Server / PG | First cycle may emit no or partial query deltas |
| **PostgreSQL pg_stat_statements** | `PgRepository` previous snapshot per `(dbid, userid, queryid)` | First PGSS delta row may equal full counters |
| **SQL Server wait stats** | Enterprise batch dedup hashes in `TimescaleLogger` | Duplicate scrape suppression resets; one extra write possible |
| **Live dashboard cache** | `dashboardCache` / `pgDashboardCache` in `MetricsService` | Live API may refresh from DMV on next request |
| **SQL Server Query Store live helpers** | `prevQueryStoreStats` in hot storage logger | Short-lived skew on rate-style metrics |

### Recommended operator response

1. **Expect a warm-up window** of 1–2 collector intervals (typically 1–5 minutes) after rolling restart or deploy.
2. For **incident analysis**, prefer Timescale-backed dashboards (historical path) once collectors have run twice post-restart.
3. Avoid interpreting **single-point spikes** in wait-stats or query-delta charts immediately after deploy.
4. For **zero-downtime deploys**, run multiple API replicas behind a load balancer; note each replica maintains its own delta memory (Query V2 is per-process).

### SQL Server restart vs API restart

DMV counters reset on **SQL Server restart** (`restart_detected` on wait-stats deltas). That is separate from API restart and is already filtered in intelligence/wait-trend queries where noted in code.

---

## Collector and TimescaleDB

- Ensure `schema-setup` completed (`05_os_metrics_collector.sql` for OS tables).
- Tune retention/compression via `01_timescale_schema.sql` policies.
- Optional upgrade scripts:
  - `migrations/014_timescale_retention_downsampling.sql` — explicit 90d retention floors + hourly CPU continuous aggregate.
  - `migrations/011_cold_storage_reduce_hot_retention_60d.sql` — opt-in 60d hot retention after cold validation.
- Federated long lookbacks (when `COLD_STORAGE_TRINO_URL` is set): CPU / memory / wait / connection history APIs return `X-Data-Source: hot+cold`.
- See [`docs/os_collector.md`](os_collector.md) for host RAM telemetry.

## OIDC group → role mapping

When `AUTH_MODE=oidc`, set:

| Variable | Purpose |
|----------|---------|
| `OIDC_GROUP_CLAIM` | Claim name (`groups` or `roles`) |
| `OIDC_GROUP_ROLE_MAP` | `groupName:admin,other:dba,viewers:viewer` |

Precedence: explicit `optima_role` claim → group map (admin > dba > viewer) → Keycloak `resource_access` roles → viewer.

## Kubernetes (Helm)

Starter chart: [`deploy/helm/sql-optima`](../deploy/helm/sql-optima). TimescaleDB remains external.

### SQL Server collector diagnostics (admin)

When workload, query-analysis, or SIH charts are empty, use the admin-only diagnostic endpoint to separate **no SQL activity**, **collector not writing**, and **frontend/API issues**. No credentials are returned.

**Endpoint**

- `GET /api/admin/diagnostics/sqlserver/{instance}`
- `GET /api/admin/diagnostics/sqlserver?instance={name-or-uuid}` (also accepts `server_id` / `server`)

**Auth:** `admin` role, CSRF cookie (same as other `/api/admin` routes).

**Query parameters**

| Param | Default | Description |
|-------|---------|-------------|
| `hours` | `24` | Lookback window for row counts (1–168) |

**Example**

```bash
curl -s -b cookies.txt \
  'http://localhost:8080/api/admin/diagnostics/sqlserver/MSSQL-01?hours=6' | jq .
```

**Response fields (summary)**

| Field | Meaning |
|-------|---------|
| `connection_status` | Live DMV path: `online` / `offline` (from in-process ping cache) |
| `collector_state` | `sqlserver_collector_instance_state`: last poll, last successful query snapshot, `last_error` |
| `collectors` | `optima_collector_configs` for `sqlserver_query_snapshot`, `sqlserver_perf_counters`, `sqlserver_session_enrichment` |
| `query_v2_pipeline_always_on` | Query V2 orchestrator is always enabled at API startup |
| `latest_capture` | `MAX(capture_timestamp)` per hypertable (legacy map; mirrors `hypertables`) |
| `row_counts_in_window` | Rows in the selected `hours` window (legacy map) |
| `hypertables[]` | Per-table: `rows_in_window`, `rows_total` (instance), `relation_size_bytes` / `relation_size_pretty`, `num_chunks`, `compression_enabled`, `dashboards` |
| `summary` | `tables_checked`, `tables_with_rows_in_window`, `total_rows_in_window`, `total_rows_all_time`, `total_storage_bytes` |
| `hints` | Operator-oriented interpretation (empty history + perf counters present, disabled snapshot job, etc.) |

**Admin UI:** Admin control panel → **SQL diagnostics** tab (or stethoscope icon on a SQL Server row under Monitoring servers). Deep link: `/admin?tab=diagnostics`.

**Typical interpretations**

1. **`row_counts_in_window.sqlserver_query_stats_history` = 0** but perf counters > 0 → allow 1–2 `sqlserver_query_snapshot` intervals after deploy; workload may show perf-counter fallback until history exists.
2. **`collector_state.last_error` set** → fix DMV permissions or connectivity on the monitored instance.
3. **`collectors.sqlserver_query_snapshot.enabled` = false** → re-enable in Admin → Collector Control.
4. **`connection_status` = offline** → registry target unreachable; historical collectors will not advance watermarks.

---

## Security baseline

- `AUTH_REQUIRED=1`, `DISABLE_PUBLIC_SETUP=1` after bootstrap.
- Vault production: [`docs/vault_production.md`](vault_production.md).
- API errors: [`docs/api_errors.md`](api_errors.md).

---

## Audit and compliance (P2-5)

- **Monitored server CRUD** → `optima_audit_logs` (`add_server`, `update_server`, `delete_server`, …).
- **Admin users** → `optima_audit_logs` (`create_user`, `delete_user`, `update_user_role`) + structured slog via `middleware.AuditAction`.
- **Alerts** → `optima_alert_history` append-only status transitions.
- **Collector frequency** → `optima_audit_logs` (`update_collector_frequency`) + slog `AuditAction` (metadata: id, frequency_seconds; no secrets).
- **Notification channels** → `optima_audit_logs` (`update_notification_channel`) + slog `AuditAction` (metadata: channel, is_enabled, url_changed — **URL never stored**).

Query audit table:

```sql
SELECT created_at, event_type, actor, server_id, metadata
FROM optima_audit_logs
ORDER BY created_at DESC
LIMIT 50;
```
