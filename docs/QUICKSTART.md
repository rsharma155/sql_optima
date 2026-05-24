# Quickstart Guide

Get SQL Optima up and running in under 5 minutes using Docker Compose.

## 1. Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) or Docker Engine with Compose V2.
- Git (to clone the repo).

## 2. Launch the Stack

Clone the repository and run the following commands:

```bash
git clone https://github.com/your-repo/sql_monitoring_UI.git
cd sql_monitoring_UI/docker
cp .env.example .env
docker compose up --build -d
```

## 3. Access the UI

Open your browser and navigate to:
**[http://localhost:8080](http://localhost:8080)**

The **Global Estate Overview** will load. Since you haven't added any servers yet, it will be empty.

## 4. Add Your First Monitored Server

1.  Navigate to the **Admin** panel (sidebar).
2.  Click **Add New Server**.
3.  Enter your database connection details (PostgreSQL or SQL Server).
    - *Note:* Credentials are encrypted at rest using Vault Transit.
4.  Click **Save**.

SQL Optima will immediately begin collecting real-time telemetry. Historical metrics (dashboards) will begin appearing after ~15 minutes of collection.

## 5. Next Steps

- **Initialize Target DBs:** Run the setup scripts found in `infrastructure/sql_scripts/` against your target databases to ensure the monitoring role has the correct permissions.
- **Enable Auth:** For production, set `AUTH_REQUIRED=1` in your `.env` file and use `go run reset_password.go` in the `backend/` directory to create an admin password.
- **Read the Architecture:** Learn how the system works in [ARCHITECTURE.md](../ARCHITECTURE.md).

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

**dbmonitor_user is created with least privilege, which is the recommended user for both postgres and sqlserver
user: dbmonitor_user
password: Hello@123**

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
| Containers not starting | Run `docker logs patroni1` or `docker logs sql1` to see initialisation errors |
| Port already in use | Edit the host port mappings in the relevant `docker-compose.yml` |
| SQL Server ODBC driver missing | Follow the manual install steps in the [repo README](https://github.com/rsharma155/sqlserver_postgres_ha_cluster/blob/main/README.md#odbc-driver-not-found--install-failed) |
| Web app not starting | `cd web_app && pip install -r requirements.txt && python app.py` |
| SQL Optima shows no data | Wait ~15 minutes after adding the servers — the collector needs a few cycles to populate historical dashboards |
