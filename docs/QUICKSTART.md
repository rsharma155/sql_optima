# Quickstart Guide

Get SQL Optima up and running in under 5 minutes using Docker Compose.

## 1. Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) or Docker Engine with Compose V2.
- Git (to clone the repo).

## 2. Launch the Stack

Clone the repository and run the following commands:

```bash
git clone https://github.com/rsharma155/sql_optima.git
cd sql_optima/docker
cp .env.example .env
```

**Before first start (recommended):** edit `.env` and set a strong `JWT_SECRET` and `DB_PASSWORD`. Compose defaults to `AUTH_REQUIRED=1` (login required).

```bash
docker compose up --build -d
```

Create a local admin password (first login):

```bash
cd ../backend
NEW_ADMIN_PASSWORD="Admin123!ChangeMe" go run reset_password.go
```

Then sign in at the UI with that password.

**Local dev only (open API, no login):** in `.env` set `AUTH_REQUIRED=0` and `DISABLE_PUBLIC_SETUP=0` — not for production.

## 3. Access the UI

Open your browser and navigate to:
**[http://localhost:8080](http://localhost:8080)**

The **Global Estate Overview** will load. Since you haven't added any servers yet, it will be empty.

## 4. Add Your First Monitored Server

1. Navigate to the **Admin** panel (sidebar).
2. Click **Add New Server**.
3. Enter your database connection details (PostgreSQL or SQL Server).
   - *Note:* Credentials are encrypted at rest using Vault Transit (dev Compose includes a Vault container).
4. Click **Save**.

SQL Optima will immediately begin collecting live telemetry. Historical metrics (dashboards) typically appear after **~15 minutes** (two or more collector cycles at the default cadence).

### Optional: PostgreSQL host RAM (OS collector)

Host memory and CPU are **not** available through a normal PostgreSQL monitoring connection. For **PostgreSQL on Linux**:

1. Save the server in Admin first (so the bundle includes **server ID**).
2. In the UI, use **Download bundle (.zip)** (Admin → Add server, or **PostgreSQL → Memory / CPU**).
3. On the DB host: unzip and run `./quick-install.sh` (prompts once for your **admin JWT**).
4. Enable ingest from the UI (**Enable ingest**) or set `OS_METRICS_INGEST_ENABLED=1` in `.env` and restart the API.

Details: [`os_collector/README.md`](../os_collector/README.md), [`docs/os_collector.md`](os_collector.md).

## 5. Next Steps

- **Initialize target DBs:** Run the setup scripts in `infrastructure/sql_scripts/` against your **monitored** databases (`pgsql_init.sql`, `sqlserver_init.sql`). Use **Admin → Check permissions** to validate grants.
- **Production hardening:** Keep `AUTH_REQUIRED=1`, set a strong `JWT_SECRET`, follow [`docs/vault_production.md`](vault_production.md), and read [`SECURITY.md`](../SECURITY.md).
- **Upgrading from 0.4.x:** Re-apply `01_timescale_schema.sql` (idempotent) or pending files under `infrastructure/sql_scripts/migrations/`. See [RELEASES.md § 0.5.0](../RELEASES.md).
- **Read the architecture:** [ARCHITECTURE.md](../ARCHITECTURE.md).

---

## Local Test Environment: HA Clusters for Development

If you don't have a live PostgreSQL or SQL Server instance to monitor, you can spin up a fully-featured local HA environment using the companion project:

**[rsharma155/sqlserver_postgres_ha_cluster](https://github.com/rsharma155/sqlserver_postgres_ha_cluster)**

This project launches two production-like HA database clusters on your laptop — a **PostgreSQL Patroni cluster** (3 nodes + etcd + HAProxy) and a **SQL Server Always On Availability Group** (3-node AG) — plus a **Python/Flask web app** to generate realistic CRUD traffic across 5 pre-seeded demo databases.

### What You Get

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

### Step 1 — Clone and Start the HA Clusters

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

### Step 2 — Verify the Clusters Are Up

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

### Step 3 — Register the Servers in SQL Optima

With both stacks running, open the SQL Optima **Admin → Add New Server** panel and add each endpoint:

#### PostgreSQL — HAProxy Write (primary)

| Field | Value |
|-------|-------|
| Host | `localhost` |
| Port | `5000` |
| Username | `postgres` |
| Password | `postgres123` |
| Database | `hotel_booking` (or any of the 5 demo DBs) |

> Alternatively use port **5001** for the HAProxy read-replica endpoint, or **5043 / 5044 / 5045** for direct Patroni node connections.

#### SQL Server — Always On AG Nodes

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

### Step 4 — Generate Load with the CRUD Web App

Open **[http://localhost:5002]** in your browser:

1. Select **PostgreSQL** or **SQL Server**.
2. Set duration (seconds), worker threads, and concurrent users.
3. Click **Start CRUD Load** — live per-operation logs stream in real time.
4. When complete, download the detailed text report or review the failed-task analysis.

The default CRUD mix is **60% reads / 40% writes**, which produces realistic wait events, lock contention, and query-store data for SQL Optima's dashboards and intelligence reports.

### Step 5 — Stop the Clusters

```bash
# Linux / macOS
./stop_servers.sh

# Windows
PowerShell -ExecutionPolicy Bypass -File .\stop_all.ps1
```

This stops all containers and cleans up any generated `docker-compose.override.yml` files.

### Connection Reference

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

### Troubleshooting

| Symptom | Fix |
|---------|-----|
| Cannot log in after compose up | Run `reset_password.go` in `backend/` (see §2) |
| Containers not starting (HA test repo) | Run `docker logs patroni1` or `docker logs sql1` |
| Port already in use | Edit host port mappings in the HA repo `docker-compose.yml` |
| SQL Server ODBC driver missing | See [HA cluster README](https://github.com/rsharma155/sqlserver_postgres_ha_cluster/blob/main/README.md#odbc-driver-not-found--install-failed) |
| SQL Optima shows no historical data | Wait ~15 minutes; check API logs for collector errors; use **Admin → SQL Server diagnostics** for empty charts |
| OS Collector badge missing | Enable ingest in UI; verify cron/systemd on DB host; see [`docs/os_collector.md`](os_collector.md) |
