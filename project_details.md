# SQL Optima — Application flow and route map

This document describes how the **Go backend**, **SPA frontend**, and **navigation** fit together. Use it alongside `README.md` for setup and `docs/API.md` for endpoint details.

---

## High-level architecture

1. **Backend (`backend/`)** — HTTP API built with Gorilla `mux`. It loads `config.yaml`, resolves database passwords from environment variables, and exposes JSON under `/api/...`. A **collector service** polls SQL Server and PostgreSQL and writes snapshots to **TimescaleDB** (optional) via the hot storage layer.
2. **Frontend (`frontend/`)** — Static HTML/CSS/JS SPA. `index.html` loads `js/entry.js` (module bootstrap), `auth.js`, `router.js`, and many page scripts that attach view functions to `window`.
3. **Routing** — There is **no path-based** router in the URL bar for most views. Navigation is **`window.appNavigate(routeId)`**, which swaps content inside `#router-outlet` and updates the sidebar’s `data-route` items. Browser history is tracked in `appState.navigationHistory` for back navigation where used.
4. **Authentication** — `POST /api/login` and **`POST /api/auth/login`** are the same rate-limited handler (shared per-IP budget, implemented in `internal/api/router.go` via `AuthHandlers.Login`). The UI uses `/api/login` by default. `GET /api/auth/me` requires a valid JWT. Tokens are stored in `localStorage` and `apiClient.authenticatedFetch` sends `Authorization: Bearer …`. Many read-only dashboard APIs are intentionally **public**; mutating endpoints (e.g. kill session, admin) require auth. (A previous bug registered `POST /api/auth/login` behind JWT middleware, which made login impossible on that path; that registration was removed.)

---

## Backend request flow (simplified)

```
Browser → SecurityHeadersMiddleware → CORS (if enabled) → mux routes
    → Public /api/* handlers (dashboards, postgres, mssql, timescale, health, rules)
    → OR Protected /api/* (JWT) for admin, widgets, xevents, postgres POST actions
```

- **`/api/config`** — Returns instance list from config (host/port/names/types). Does not include passwords.
- **`POST /api/postgres/explain/analyze`** — Body `{ "query"?, "plan": string | object }`; returns `result`, `plan_graph`, `plan_mermaid`, and optional `sql_context` (heuristic SQL line excerpts + `CREATE INDEX` drafts when `query` is set). Max body ~512 KiB.
- **`POST /api/postgres/explain/optimize`** — Same body; returns `report`, `plan_root`, `plan_graph`, `plan_mermaid`, optional `sql_context`.
- **Instance parameter** — Most handlers take `?instance=<name>`. Names are validated with `^[a-zA-Z0-9_-]+$` (max length capped) to reduce injection-style abuse in query construction.
- **Rules / best practices** — `GET /api/rules/best-practices` drives the generic rules engine UI; PostgreSQL-specific checks also use `GET /api/postgres/best-practices` where wired in the PG best-practices view.

---

## Frontend boot flow

1. `js/entry.js` loads configuration via `apiClient.fetchConfig()` → `GET /api/config`.
2. `window.router.populateInstanceDropdown()` fills `#instance-select`.
3. `window.appNavigate('global')` shows the global estate view (`GlobalEstateView`).
4. When the user picks an instance, `router.js` rebuilds `#sidebar-nav` for **PostgreSQL** or **SQL Server** and may switch to `pg-dashboard` or `dashboard`.

---

## SPA route map (`window.appNavigate('<route>')`)

Routes must match `^[a-zA-Z0-9-]+$` (length ≤ 96). Unknown routes show a **Page not found** view instead of silently loading the wrong dashboard.

| Route | View function | Notes |
|--------|----------------|--------|
| `global` | `GlobalEstateView` | Default after boot; no instance required |
| `dashboard` | `DashboardView` | MSSQL instance dashboard; requires instance |
| `drilldown-cpu` | `CpuDrilldown` | |
| `mssql-cpu-dashboard` | `MssqlCpuDashboardView` | |
| `instance-health` | `InstanceHealthDashboardView` | |
| `drilldown-query` | `mssql_QueryDrilldown` | |
| `drilldown-top-queries` | `mssql_TopQueriesDrilldown` | |
| `drilldown-metric-detail` | `mssql_MetricDetailDrilldown` | |
| `drilldown-deadlocks` | `mssql_DeadlockDashboard` | |
| `drilldown-deadlock` | `mssql_DeadlockDashboard` | Alias for deadlock UI |
| `drilldown-growth` | `GrowthDrilldown` | |
| `drilldown-index` | `IndexDrilldown` | |
| `drilldown-locks` | `LocksDrilldown` | |
| `drilldown-bottlenecks` | `HistoricalBottlenecksView` | |
| `drilldown-ha` | `HADashboardView` | |
| `drilldown-pg-enterprise` | `PgEnterpriseDashboardView` | |
| `enterprise-metrics` | `EnterpriseMetricsView` | Requires instance |
| `performance-debt` | `mssql_PerformanceDebtDashboard` | |
| `jobs` | `JobsView` | |
| `alerts` | `AlertsView` | |
| `incidents` | `AlertsView` | **Alias** (e.g. from Reports actions) |
| `login` | `LoginView` | Used after logout |
| `settings` | `SettingsView` | |
| `best-practices` | `RulesEngineView` | MSSQL-oriented rules dashboard |
| `live-diagnostics` | `LiveDiagnosticsView` | |
| `pg-dashboard` | `PgDashboardView` | Control Center |
| `pg-sessions` | `PgSessionsView` | |
| `pg-locks` | `PgLocksView` | |
| `pg-queries` | `PgQueriesView` | |
| `pg-explain` | `PgExplainView` | Paste EXPLAIN text/JSON; analyze + optimization report (no live query execution) |
| `pg-storage` | `PgStorageView` | |
| `pg-replication` | `PgReplicationView` | |
| `pg-logs` | `PgLogsView` | |
| `pg-backups` | `PgBackupsView` | |
| `pg-alerts` | `PgAlertsView` | |
| `pg-config` | `PgConfigView` | Not in default PG sidebar; deep-link only |
| `pg-cpu` | `PgCpuView` | |
| `pg-memory` | `PgMemoryView` | |
| `pg-cnpg` | `CNPGClusterTopologyView` | Legacy / deep-link |
| `pg-best-practices` | `PgBestPracticesView` | |
| `dynamic-dashboard` | `DynamicDashboardView` | Not in default sidebar; deep-link |
| `sentinel-mock` | `SentinelMockView` | Optional mock UI; see below |
| `admin` | `AdminPanelView` | Requires logged-in **admin** |

---

## Sidebar vs static HTML

- **`index.html`** initially contains only **Global Estate** in `#sidebar-nav` so users are not offered MSSQL-only links before config and instance selection run.
- After an instance is selected, `router.populateDatabaseDropdown()` injects the full **Postgres** or **SQL Server** menu (see `router.js`).

---

## Optional Sentinel mock (not bundled by default)

- **Why omit from `index.html`:** Fewer scripts in production, smaller surface area, no demo-only UI in routine operator workflows.
- **Route:** `window.appNavigate('sentinel-mock')` is registered in `router.js`. If the script is not loaded, the app shows short instructions instead of a blank page.
- **Enable locally:** After `router.js`, add:
  `<script src="js/pages/ui_SentinelMock.js"></script>`
  Then run `appNavigate('sentinel-mock')` from the browser console or a temporary button.

---

## Security notes (frontend + API)

- **XSS**: User-facing and API error strings assigned to `innerHTML` should go through `window.escapeHtml`. Recent hardening includes router error surfaces, locks drilldown, jobs view, and MSSQL dashboard errors; navigation uses `data-route` equality instead of embedding the route in a CSS selector.
- **Open dashboards**: Many `GET /api/mssql/*`, `GET /api/postgres/*`, and `GET /api/timescale/*` routes are **unauthenticated** by design. Treat network access as a trust boundary; place the UI/API behind SSO/VPN or enable auth at the reverse proxy if needed.
- **JWT**: Set a strong `JWT_SECRET` in production (`main.go` warns when unset).

---

## Related files

| Area | Path |
|------|------|
| HTTP routes | `backend/internal/api/router.go` |
| JWT / security headers | `backend/internal/middleware/` |
| SPA router | `frontend/js/components/router.js` |
| Auth UI | `frontend/js/components/auth.js` |
| API client | `frontend/js/api/client.js` |
| App boot | `frontend/js/modules/app-client.js`, `frontend/js/entry.js` |
