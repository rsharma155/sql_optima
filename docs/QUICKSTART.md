# Quickstart Guide

Get SQL Optima running in minutes. This guide has two paths: **development / quick test** first (no secret editing), then **production**.

---

## Development — download and run (recommended first)

Use this path to evaluate SQL Optima on your laptop. You do **not** need to set `JWT_SECRET`, database passwords, or run CLI tools before opening the UI.

### What you need

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) or Docker Engine with Compose V2
- Git (to clone the repo), **or** a release archive from [GitHub Releases](https://github.com/rsharma155/sql_optima/releases)

### One command

From **any directory** (install script clones the repo if needed, starts Docker, waits for the API, and opens your browser):

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/rsharma155/sql_optima/main/install.sh | bash
```

Or from a cloned repo:

```bash
./install.sh
./install.sh --no-browser    # print URL only, do not open a browser
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/rsharma155/sql_optima/main/install.ps1 | iex
```

Or from a cloned repo:

```powershell
PowerShell -ExecutionPolicy Bypass -File .\install.ps1
.\install.ps1 -NoBrowser
```

Environment: `SQL_OPTIMA_DIR` (target clone path), `SQL_OPTIMA_NO_BROWSER=1`, `SQL_OPTIMA_WAIT_SEC` (API wait timeout, default 900).

The install script delegates to `docker/start-dev.sh` or `docker/start-dev.ps1`, which copy `docker/.env.dev` → `docker/.env` on first run (safe dev defaults only), then `docker compose up --build -d`.

**Alternate** (manual clone + compose):

```bash
git clone https://github.com/rsharma155/sql_optima.git
cd sql_optima/docker
./start-dev.sh
```

(`start-dev` also waits for `/api/health` and opens the browser unless `--no-browser` / `-NoBrowser`.)

### First visit in the browser

1. The install script opens **[http://localhost:8080](http://localhost:8080)** when ready (or the port in `API_PORT`).
2. First build can take several minutes; the script polls until the API and TimescaleDB are up.
3. The **Docker quick start** wizard appears. TimescaleDB and schema are already provisioned by Compose — you only create your **admin username and password** in the browser.
4. You are routed to **Add monitored databases**. If you have no PostgreSQL or SQL Server yet, wait a few seconds on the empty form (or click **Local HA setup guide** in the sidebar) for step-by-step instructions to run the [companion HA cluster project](#local-test-environment-ha-clusters-for-development).
5. Register PostgreSQL or SQL Server when ready.

Historical dashboards usually need **~15 minutes** (two collector cycles) after you add a server.

### Stop the stack

```bash
cd sql_optima/docker
docker compose down          # stop, keep data
docker compose down -v       # stop and remove volumes (fresh install next time)
```

### No database to monitor yet?

Use the companion [sqlserver_postgres_ha_cluster](https://github.com/rsharma155/sqlserver_postgres_ha_cluster) project for local PostgreSQL and SQL Server 3-node cluster. After creating your admin account, the onboarding page shows an in-app **Local HA setup guide** (also in the sidebar). Full details: [Local test environment](#local-test-environment-ha-clusters-for-development) below.

### Development troubleshooting

| Symptom | What to do |
|---------|------------|
| Setup wizard says metrics DB unreachable | Run `docker compose ps` — wait for `timescaledb` and `schema-setup` to finish; check `docker compose logs api` |
| “Setup unavailable” / public setup disabled | You are using production `.env` values. For dev, use `./start-dev.sh` / `.\start-dev.ps1` or `cp .env.dev .env` and restart |
| Forgot admin password (dev) | `docker compose down -v` then `./start-dev.sh` or `.\start-dev.ps1` for a fresh DB and setup wizard |
| PowerShell blocks the script | Run `PowerShell -ExecutionPolicy Bypass -File .\start-dev.ps1` |
| Port 8080 in use | Set `API_PORT=8081` in `docker/.env` and restart |
| `docker-vault-1 is unhealthy` / `Vault API did not become ready within 90s` | Update `docker/scripts/vault-entrypoint.sh` (fixed wait for uninitialized Vault), then `docker compose down -v` and restart; see `docker compose logs vault` |
| Save server: `invalid token` / Transit 403 | Use the **full repo** (not only `docker/`). Remove `VAULT_TOKEN=root` from `.env`. Rebuild API: `docker compose up -d --build --force-recreate api`. Logs should show `[api] Using Vault token from /vault/token/.root_token` |
| `/api/os-collector/status` 404 while adding a server | Harmless while typing a name before save; status returns `registered: false` after API rebuild. Hard-refresh the browser for updated JS |
| Build: `parent snapshot ... does not exist: not found` | Corrupted BuildKit cache (Windows). Run `docker builder prune -af`, then `docker compose build --no-cache api`, then `docker compose up -d`. Restart Docker Desktop if needed |

---

## Production — hardened deployment

Use this when SQL Optima is shared on a network, exposed beyond localhost, or kept running long term.

### 1. Configure secrets

```bash
cd sql_optima/docker
cp .env.example .env
```

Edit `.env` and set at minimum:

| Variable | Action |
|----------|--------|
| `JWT_SECRET` | Strong random value, e.g. `openssl rand -base64 32` |
| `DB_PASSWORD` | Strong password for the TimescaleDB role |
| `AUTH_REQUIRED` | Keep `1` |
| `DISABLE_PUBLIC_SETUP` | Keep `1` (locks public setup API after bootstrap) |

See inline comments in [`docker/.env.example`](../docker/.env.example) for webhooks, cold storage, and Vault.

### 2. Start the stack

```bash
docker compose up --build -d
```

### 3. Create the first admin

With `DISABLE_PUBLIC_SETUP=1`, the browser setup wizard is off. Create the admin once:

```bash
cd ../backend
NEW_ADMIN_PASSWORD='YourStrongPassword8+' go run reset_password.go
```

Sign in at the UI as user **`admin`** with that password. Add further users from **Admin** if needed.

### 4. Hardening checklist

- [ ] Replace all default passwords and `JWT_SECRET`
- [ ] Keep `AUTH_REQUIRED=1` and `DISABLE_PUBLIC_SETUP=1`
- [ ] Register monitored servers only via **Admin** (credentials encrypted with Vault Transit)
- [ ] Follow [`docs/vault_production.md`](vault_production.md) for Vault (not dev root tokens)
- [ ] Read [`SECURITY.md`](../SECURITY.md) and [`docs/operations.md`](operations.md)
- [ ] Pin container images by version tag, e.g. `ghcr.io/rsharma155/sql-optima:0.5.0`

### Platform profile (Redis worker, Prometheus, Grafana)

```bash
export JWT_SECRET=$(openssl rand -base64 32)
docker compose -f docker-compose.platform.yml up -d
```

---

## After install — next steps

- **Target database grants:** `infrastructure/sql_scripts/pgsql_init.sql` and `sqlserver_init.sql` on **monitored** databases; validate in **Admin → Check permissions**
- **PostgreSQL host RAM (optional):** [OS collector](../os_collector/README.md) — download bundle from Admin or **PostgreSQL → Memory / CPU**
- **Architecture:** [ARCHITECTURE.md](../ARCHITECTURE.md)
- **Upgrading from 0.4.x:** [RELEASES.md § 0.5.0](../RELEASES.md)

---

## Local test environment: HA clusters for development

If you don't have a live PostgreSQL or SQL Server instance to monitor, you can spin up a fully-featured local HA environment using the companion project:

**[rsharma155/sqlserver_postgres_ha_cluster](https://github.com/rsharma155/sqlserver_postgres_ha_cluster)**

This project launches two production-like HA database clusters on your laptop — a **PostgreSQL Patroni cluster** (3 nodes + etcd + HAProxy) and a **SQL Server Always On Availability Group** (3-node AG) — plus a **Python/Flask web app** to generate realistic CRUD traffic across 5 pre-seeded demo databases.

### What you get

| Component | Details |
|-----------|---------|
| PostgreSQL HA | 3 Patroni nodes, etcd leader election, HAProxy read/write split |
| SQL Server HA | 3-node Always On AG with automated bootstrap scripts |
| 5 demo databases | `hotel_booking`, `e_commerce`, `erp_system`, `hrm_tool`, `department_store` (50K–200K seed rows each) |
| CRUD load generator | Flask web UI at `http://localhost:5002` — configurable threads, duration, concurrent users |
| Backup manager | Scheduled PG WAL archiving and SQL Server TLOG backups |

### Prerequisites

- Docker 24+ and Docker Compose v2+
- Python 3.8+
- For SQL Server CRUD: the launcher auto-installs the Microsoft ODBC 18 driver (requires `sudo` on Linux or `brew` on macOS)

### Step 1 — Clone and start the HA clusters

```bash
git clone https://github.com/rsharma155/sqlserver_postgres_ha_cluster.git
cd sqlserver_postgres_ha_cluster/Postgres_SQLServer_Test_Servers

# Linux / macOS — interactive launcher (choose PostgreSQL, SQL Server, or both):
./start_servers.sh

# Windows PowerShell:
PowerShell -ExecutionPolicy Bypass -File .\start_all.ps1
```

The launcher detects your system RAM and scales container memory limits automatically, then starts the selected clusters and the Flask web app. Allow ~60 seconds for all containers to initialise.

To start only one engine (no interactive prompt):

```bash
# PostgreSQL only
./start_servers.sh --skip-sql-server

# SQL Server only
./start_servers.sh --skip-postgres
```

### Step 2 — Verify the clusters are up

```bash
# Check container status
./start_servers.sh --status          # Linux / macOS
.\start_all.ps1 -Status              # Windows

# Spot-check PostgreSQL primary (HAProxy write port)
psql -h localhost -p 5000 -U postgres -c "SELECT pg_is_in_recovery();"
# Expected: f  (false = this is the primary)

# Spot-check SQL Server node 1
sqlcmd -S localhost,14331 -U sa -P 'S@L_2024_HADr_D0ck3r!' -Q "SELECT @@SERVERNAME"
```

### Step 3 — Register the servers in SQL Optima

With both stacks running, open the SQL Optima **Admin → Add New Server** panel and add each endpoint:

#### PostgreSQL — HAProxy write (primary)

| Field | Value |
|-------|-------|
| Host | `localhost` |
| Port | `5000` |
| Username | `postgres` |
| Password | `postgres123` |
| Database | `hotel_booking` (or any of the 5 demo DBs) |

> Alternatively use port **5001** for the HAProxy read-replica endpoint, or **5043 / 5044 / 5045** for direct Patroni node connections.

#### SQL Server — Always On AG nodes

Add each node separately to observe per-replica metrics and HA replication dashboards:

| Node | Host | Port |
|------|------|------|
| sql1 (primary) | `localhost` | `14331` |
| sql2 (secondary) | `localhost` | `14332` |
| sql3 (secondary) | `localhost` | `14333` |

Credentials for all nodes: username `sa`, password `S@L_2024_HADr_D0ck3r!`

**Recommended monitoring login (least privilege):**

| Field | Value |
|-------|-------|
| User | `dbmonitor_user` |
| Password | `Hello@123` |

### Step 4 — Generate load with the CRUD web app

Open **[http://localhost:5002]** in your browser:

1. Select **PostgreSQL** or **SQL Server**.
2. Set duration (seconds), worker threads, and concurrent users.
3. Click **Start CRUD Load** — live per-operation logs stream in real time.
4. When complete, download the detailed text report or review the failed-task analysis.

The default CRUD mix is **60% reads / 40% writes**, which produces realistic wait events, lock contention, and query-store data for SQL Optima's dashboards and intelligence reports.

### Step 5 — Stop the clusters

```bash
# Linux / macOS
./stop_servers.sh

# Windows
PowerShell -ExecutionPolicy Bypass -File .\stop_all.ps1
```

This stops all containers and cleans up any generated `docker-compose.override.yml` files.

### Connection reference

| Engine | Endpoint | Port | Purpose | Credentials |
|--------|----------|------|---------|-------------|
| PostgreSQL | localhost | 5000 | HAProxy — writes (primary) | postgres / postgres123 |
| PostgreSQL | localhost | 5001 | HAProxy — reads (replicas) | postgres / postgres123 |
| PostgreSQL | localhost | 5043 | Direct — patroni1 | postgres / postgres123 |
| PostgreSQL | localhost | 5044 | Direct — patroni2 | postgres / postgres123 |
| PostgreSQL | localhost | 5045 | Direct — patroni3 | postgres / postgres123 |
| SQL Server | localhost | 14331 | sql1 node | sa / S@L_2024_HADr_D0ck3r! |
| SQL Server | localhost | 14332 | sql2 node | sa / S@L_2024_HADr_D0ck3r! |
| SQL Server | localhost | 14333 | sql3 node | sa / S@L_2024_HADr_D0ck3r! |
| CRUD web app | localhost | 5002 | Flask load-generator UI | — |

### HA cluster troubleshooting

| Symptom | Fix |
|---------|-----|
| Containers not starting | Run `docker logs patroni1` or `docker logs sql1` |
| Port already in use | Edit host port mappings in the HA repo `docker-compose.yml` |
| SQL Server ODBC driver missing | See [HA cluster README](https://github.com/rsharma155/sqlserver_postgres_ha_cluster/blob/main/README.md#odbc-driver-not-found--install-failed) |
| SQL Optima shows no historical data | Wait ~15 minutes; check API logs; use **Admin → SQL Server diagnostics** |
| OS Collector badge missing | Enable ingest in UI; see [`docs/os_collector.md`](os_collector.md) |
