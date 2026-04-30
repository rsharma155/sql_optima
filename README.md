# SQL Optima

A dual-engine (PostgreSQL & SQL Server) database monitoring platform with a single-page application UI. Built with Go and vanilla JavaScript, architected around Domain-Driven Design for cross-platform support (Windows, macOS, Linux).

Features include PostgreSQL EXPLAIN plan analysis with optimization and index advisor workflows, live SQL Server diagnostics, TimescaleDB-backed historical dashboards, an **enhanced rules engine** for context-aware best-practice checks, **push-based OS metrics** for host-level telemetry, and a **cross-engine alert engine** with fingerprint-based deduplication, maintenance windows, and audit history.

---

## UI Preview

![SQL Server dashboard](docs/screenshots/sqlserver-dashboard.png)

![PostgreSQL dashboard](docs/screenshots/postgres-dashboard.png)

---

## Quick Start — Docker Compose (recommended)

This brings up **TimescaleDB + Vault (Transit KMS) + Go API + static UI** with automatic schema bootstrap. You can then add monitored SQL Server / PostgreSQL targets from the web UI — no `config.yaml` editing required.

### Prerequisites
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) or Docker Engine with Compose V2.

### Steps

```bash
cd docker
cp .env.example .env          # optional — edit to change ports, passwords, or enable auth
docker compose up --build
```

Open **http://localhost:8080** — the Global Estate Overview loads immediately.

### What starts automatically

| Service | Purpose |
|---------|---------|
| **api** | Go backend serving the API + SPA UI on port 8080 |
| **timescaledb** | TimescaleDB (pg16) for metric / time-series storage |
| **vault** | HashiCorp Vault dev-mode (Transit KMS for credential encryption) |
| **vault-setup** | One-shot: enables Transit engine and creates the encryption key |
| **schema-setup** | One-shot: applies schema, rule engine, alert engine, seed data to TimescaleDB |

### Adding monitored servers

After the stack is running, add targets from the **Admin** panel in the UI or via the API:

```bash
# API example
curl -X POST http://localhost:8080/api/admin/servers \
  -H 'Content-Type: application/json' \
  -d '{"name":"PG-Prod","db_type":"postgres","host":"10.0.5.21","port":5432,"username":"monitor","password":"secret"}'
```

Credentials are encrypted at rest using Vault Transit.

### Stopping

```bash
docker compose down            # data persists in the timescaledb_data volume
docker compose down -v         # also removes stored data
```

### Create / reset admin user (local auth)

```bash
cd backend
NEW_ADMIN_PASSWORD="Admin123!ChangeMe" go run reset_password.go
```

---

## Option 2: TimescaleDB via Docker + Manual Go Server (dev workflow)

Use this when you want to develop the Go backend locally but still need TimescaleDB.

### Phase 1 — Start TimescaleDB

```bash
cd infrastructure/docker
cp ../../docker/.env.example .env    # adjust DB_PASSWORD
docker compose up -d
docker logs dbmonitor_timescaledb    # verify healthy
```

### Phase 2 — Run the Go backend

Requires [Go 1.25+](https://go.dev/dl/).

```bash
cd backend
go mod tidy
go run cmd/server/main.go
```

Open **http://localhost:8080** — If this is the first run, the **Setup Wizard** will appear to help you connect to the TimescaleDB instance. Ensure you have run the schema initialization scripts (from `infrastructure/sql_scripts/`) against TimescaleDB prior to this.

### Phase 3 — Stop TimescaleDB

```bash
cd infrastructure/docker
docker compose down
```

---

## Option 3: Dedicated PostgreSQL / TimescaleDB (no Docker)

> Your PostgreSQL server **must** have the [TimescaleDB extension](https://docs.timescale.com/install/latest/) installed.

### Run the backend

```bash
cd backend
go mod tidy
go run cmd/server/main.go
```

Open **http://localhost:8080** and follow the **Setup Wizard**. The UI will:
1. Test your connection to the dedicated TimescaleDB.
2. Allow you to verify that the **schema is initialized** (Ensure you have applied the scripts in `infrastructure/sql_scripts/` beforehand).
3. Create your initial admin user.

---

## Build standalone binaries

```bash
# Build API server
cd backend
go build -o ../dist/sql-optima ./cmd/server

# Build OS collector (optional, for remote hosts)
cd ../os_collector
go build -o ../dist/os-collector .
```

Run the API from anywhere:

```bash
./dist/sql-optima
```

---

## Configuration

### Adding monitored targets

**Recommended:** Use the **Admin** panel in the web UI. Credentials are encrypted with Vault Transit (or a local KMS) and stored in the TimescaleDB server registry. This is the primary way to manage monitoring targets.

> Targets added via the UI are persisted in the metrics database and automatically reloaded when the server restarts.

---

## Platform Compose (production profile)

For production-like deployments with Redis (Asynq worker queue), Prometheus, and Grafana:

```bash
# Requires JWT_SECRET to be set
export JWT_SECRET=$(openssl rand -base64 32)
docker compose -f docker-compose.platform.yml up -d
```

This starts: API, worker, TimescaleDB, Redis, Prometheus, and Grafana. Auth is enabled by default (`AUTH_REQUIRED=1`).

---

## Target database setup scripts

Provision monitoring roles on your target databases so SQL Optima can collect telemetry.

### SQL Server

```powershell
sqlcmd -S <server> -i infrastructure/sql_scripts/sqlserver_init.sql
```

### PostgreSQL

```bash
psql -U postgres -f infrastructure/sql_scripts/pgsql_init.sql
psql -U postgres -d <target_db> -c "SELECT grant_db_permissions();"
```

---

## Repository layout

| Path | Purpose |
|------|---------|
| `docker/docker-compose.yml` | **Primary** compose — API + TimescaleDB + Vault + schema bootstrap |
| `docker-compose.platform.yml` | Production profile — adds worker, Redis, Prometheus, Grafana |
| `Dockerfile` | Multi-stage build for the API server (distroless non-root image) |
| `Dockerfile.worker` | Multi-stage build for the background worker |
| `infrastructure/docker/` | Standalone TimescaleDB compose for local dev (Option 2) |
| `infrastructure/sql_scripts/` | Schema, seed data, rule engine, alert engine, and target DB setup scripts |
| `backend/` | Go API, collector, service layer, repository, middleware |
| `os_collector/` | Lightweight agent for push-based host telemetry |
| `frontend/` | Static SPA (HTML/CSS/JS) served by the Go backend |
| `docs/` | API reference, threat model, architecture docs |
| `config.yaml` | (Optional) Legacy instance definitions (not needed when using server registry) |

---

## Alert Engine

The built-in alert engine continuously evaluates rules against every monitored instance and creates deduplicated, auditable alerts.

### Key design

- **Fingerprint-based dedup** — each rule+instance combination produces a stable SHA-256 fingerprint. A partial unique index (`fingerprint WHERE status IN ('open', 'acknowledged')`) ensures only one active alert per fingerprint; subsequent evaluations bump `hit_count` and `last_seen_at` instead of creating duplicates.
- **Singleton evaluation** — the background loop uses `pg_try_advisory_xact_lock` so that in multi-replica deployments only one process evaluates per tick.
- **Maintenance windows** — suppress alert creation for a given instance + engine during scheduled maintenance.
- **Audit trail** — every status transition (open → acknowledged → resolved) is recorded in `optima_alert_history` with actor, reason, and timestamp.
- **Auth-derived actor** — mutation endpoints (`acknowledge`, `resolve`, `create maintenance window`) extract the actor identity from JWT claims; no client-supplied `actor` field is trusted.

### Built-in evaluators

| Evaluator | Engine | Category |
|-----------|--------|----------|
| Blocking sessions | SQL Server | blocking |
| Failed agent jobs | SQL Server | jobs |
| Disk space | SQL Server | storage |
| Replication lag | PostgreSQL | replication |
| Blocking queries | PostgreSQL | blocking |
| Backup freshness | PostgreSQL | backup |
| Disk space | PostgreSQL | storage |

### API endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/alerts` | GET | List alerts (filter by status, instance, severity, engine) |
| `/api/alerts/{id}` | GET | Get alert detail |
| `/api/alerts/{id}/acknowledge` | POST | Acknowledge — body `{"reason": "..."}` is optional |
| `/api/alerts/{id}/resolve` | POST | Resolve — body `{"reason": "..."}` is optional |
| `/api/alerts/count` | GET | Count open alerts for an instance |
| `/api/alerts/maintenance` | POST | Create maintenance window |
| `/api/alerts/maintenance` | GET | List active maintenance windows |
| `/api/alerts/maintenance/{id}` | DELETE | Delete maintenance window |

### Admin — Permission check

Probe whether the monitoring role has the required grants and generate ready-to-run SQL scripts:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/admin/servers/check-permissions-draft` | POST | Check permissions for a server definition not yet saved; returns `grant_script` and `create_user_script` |
| `/api/admin/servers/{id}/check-permissions` | POST | Check permissions for a registered server by ID |

---

## Storage & Index Health (Timescale-backed)

The **Storage & Index Health** dashboard is a cross-engine page (`storage-index-health`) that reads historical snapshots from TimescaleDB and surfaces:

- **Index usage deltas** (seeks/scans/lookups/updates) and "unused index" candidates with index definition detail
- **High-scan tables** (scan-to-seek ratios) and scan hotspots
- **Largest tables / indexes** by size
- **Growth trends** (table + index) with simple projections
- **Duplicate index candidates** (requires index-definition snapshots)

### Backend endpoints

All endpoints are **Timescale reads** and require `engine` and `instance` query parameters.

| Endpoint | Description |
|----------|-------------|
| `GET /api/timescale/storage-index-health/filters` | Distinct db/schema/table options |
| `GET /api/timescale/storage-index-health/dashboard` | Pre-aggregated KPIs, hotspots, candidates |
| `GET /api/timescale/storage-index-health/index-usage` | Index usage point series |
| `GET /api/timescale/storage-index-health/table-usage` | Table usage point series |
| `GET /api/timescale/storage-index-health/growth` | Table/index growth trends |
| `GET /api/timescale/storage-index-health/index-definition` | Index CREATE statement details |

> After first deploy, dashboards will be empty until the historical collector has run a few ticks (~15 min cadence for index/table usage; growth snapshots are coarser).

---

## Security operations checklist

1. **JWT**: Set `JWT_SECRET` to a long random value in any shared or production environment. The server logs a warning if it falls back to the development default.
2. **API exposure**: All monitoring API endpoints now strictly enforce JWT authentication. The API is no longer public. Ensure `AUTH_REQUIRED=1` is set in your environment variables.
3. **Secrets**: Use the Admin UI to register monitored servers. Credentials added via the UI are encrypted at rest (using Vault Transit or a local KMS) and never stored in plain text.
4. **Auth**: Set `AUTH_REQUIRED=1` in production.
5. **Vault**: For production, use external Vault with AppRole/policies — do not use dev-mode root tokens.

---

## Documentation

| Document | Description |
|----------|-------------|
| [project_details.md](./project_details.md) | Application flow, SPA route map, sidebar behavior |
| [docs/API.md](./docs/API.md) | API endpoint reference |
| [docs/threat_model.md](./docs/threat_model.md) | Security threat model and mitigations |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System architecture and trust boundaries |
| [CONTRIBUTING.md](./CONTRIBUTING.md) | Contribution guidelines |
| [SECURITY.md](./SECURITY.md) | Security disclosure policy |

---

## PostgreSQL and SQL Server dashboards

- **PostgreSQL**: Control Center, sessions, locks, queries, EXPLAIN analyzer, storage, replication/HA, autovacuum & bloat risk, enterprise monitor, best-practices, CPU/memory, alerts.
- **SQL Server**: Instance dashboard (4-column KPI layout), CPU dashboard, live diagnostics, HA/AG, enterprise metrics, performance debt, memory drilldown, agent jobs (with auto-refresh), real-time delta metrics for queries, alerts, best practices; drilldowns for CPU, queries, bottlenecks, growth, indexes, locks, deadlocks.
