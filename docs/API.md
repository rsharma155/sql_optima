# HTTP API surface

All URLs below are relative to the Go server (default `http://localhost:8080`).  
Except where noted, endpoints require `Authorization: Bearer <JWT>` from `POST /api/login`.

## Public

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/login` | Returns `{ "token": "<jwt>", "user_id", "username", "role" }`. Rate-limited per client IP (`LOGIN_RATE_LIMIT_PER_MIN`, default 20/min). |
| POST | `/api/auth/login` | Same handler and limits as `/api/login` (REST-style alias; both paths share the same rate-limit counter per IP). |
| POST | `/api/postgres/explain/analyze` | Body `{ "query"?, "plan": object \| array \| JSON string }`. **JSON plan only** (`EXPLAIN … FORMAT JSON`). Returns `result`, `sql_context` (findings + `heuristic_insights` from the plan tree). Max body ~512 KiB. |
| POST | `/api/postgres/explain/optimize` | Same body (JSON plan only); returns `report` and `sql_context` with `heuristic_insights`. |
| POST | `/api/postgres/explain/index-advisor` | Body `{ "query", "plan": object, "instance_name"?, "database_dsn"?, "query_params"?, "options"? }`. JSON plan only; embeds **pg_missing_index** for index DDL + query rewrites. Resolves DSN from `instance_name` or the sole configured Postgres instance. Returns `recommendation_status`, `top_recommendation`, `query_rewrites`, `diagnostics`, etc. |
| GET | `/api/health` | Liveness: process is up (JSON `{ "status": "ok", "timestamp": "..." }`). |
| GET | `/api/health/ready` | Readiness: config has instances; if `HEALTH_STRICT=1`, `queries.yml` must have loaded. |

## Authenticated

| Method | Path | Query / body | Notes |
|--------|------|----------------|-------|
| GET | `/api/config` | — | Instance list with passwords redacted. Set `CONFIG_REDACT_TOPOLOGY=1` to also redact host/port. |
| GET | `/api/global` | — | Per-instance rollup metrics for the estate view. |
| GET | `/api/postgres/overview` | `instance` | Cached Postgres summary (TPS, cache hit, DB count). |
| GET | `/api/postgres/dashboard` | `instance`, `database` (optional, default `all`) | Throughput series for charts. |
| GET | `/api/mssql/overview` | `instance` | Cached SQL Server summary (CPU, memory, locks, top query count). |
| GET | `/api/mssql/dashboard` | `instance` | Full cached dashboard payload. |
| GET | `/api/mssql/jobs` | `instance` | Agent job metrics. |
| GET | `/api/mssql/xevents` | `instance` | Recent extended events from local SQLite buffer. |
| GET | `/api/mssql/best-practices` | `instance` | Configuration audit / best-practices result. |

## Frontend usage (grep-aligned)

| Source file | Calls |
|-------------|--------|
| `frontend/js/modules/app-client.js` | `/api/config`, `/api/login` |
| `frontend/js/pages/global.js` | `/api/global` |
| `frontend/js/pages/overview.js` | `/api/postgres/dashboard` |
| `frontend/js/pages/mssql_*.js` | `/api/mssql/dashboard`, `/api/mssql/jobs`, `/api/mssql/best-practices` |

## Operational environment

| Variable | Purpose |
|----------|---------|
| `CORS_ALLOWED_ORIGINS` | Comma-separated origins for browser cross-origin API access. |
| `CONFIG_REDACT_TOPOLOGY` | `1` to redact host/port in `/api/config`. |
| `HEALTH_STRICT` | `1` to fail readiness when `queries.yml` did not load. |
| `LOGIN_RATE_LIMIT_PER_MIN` | Override default login rate limit (per IP per minute). |
