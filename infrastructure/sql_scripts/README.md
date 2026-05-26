# SQL Optima — TimescaleDB and SQL scripts

This directory is the **single source** for SQL Optima database scripts (merged from the former `sql_scripts_used/` tree).

## Root-level scripts

| File | Description |
|------|-------------|
| `01_timescale_schema.sql` | Main TimescaleDB schema: hypertables, indexes, compression |
| `07_optima_server_dr_policy.sql` | Idempotent platform config: `optima_server_dr_policy`, `optima_notification_config`; re-run on upgrade |
| `06_seed_data.sql` | Default users, widgets, and collection schedules |
| `03_additional_pg_rules.sql` | Additional PostgreSQL-specific rules for the rule engine |
| `04_alert_engine.sql` | **Canonical** alert engine schema |
| `05_os_metrics_collector.sql` | Schema for host-level OS metrics collector (agent data) |
| `pgsql_init.sql` | PostgreSQL instance bootstrap helper |
| `sqlserver_init.sql` | SQL Server–side helper scripts |
| `SST.sql` | *(Optional)* System setup test — include in repo if you maintain it |
| `02_additional_sqlserver_metrics.sql` | *(Optional)* Extended SQL Server metrics — include if maintained separately |

## Supplementary directories

| Directory | Description |
|-----------|-------------|
| `collection/` | **~70** parameterized queries used by collectors against SQL Server and PostgreSQL (metrics, waits, jobs, PG stats, etc.) |
| `timescaledb_fetch/` | **~48** read/insert templates for TimescaleDB (historical series, summaries, hypertable helpers) |
| `migrations/` | Ordered **incremental** migrations for existing deployments (run after `01_timescale_schema.sql` when upgrading) |
| `postgres/` | Scripts to run **on monitored PostgreSQL** instances (e.g. optional materialized views), not on TimescaleDB |

### Migrations (`migrations/`)

Apply in numeric order when upgrading an existing database. `009_postgres_system_stats_cpu_enhancement.sql` was renumbered from `005_*` when merging with `005_enterprise_metrics_collection.sql` to avoid two `005_` files.

| File | Purpose |
|------|---------|
| `001_create_query_store_stats.sql` | Query store stats table |
| `002_sqlserver_enterprise_monitoring.sql` | SQL Server enterprise objects |
| `003_postgres_enterprise_monitoring.sql` | PostgreSQL enterprise objects |
| `004_fix_top_queries_table.sql` | Top queries fix |
| `005_enterprise_metrics_collection.sql` | Enterprise metrics collection |
| `006_custom_dashboards_alerts.sql` | Dashboards / alerts |
| `007_fix_query_dictionary_constraint.sql` | Query dictionary constraint |
| `008_add_job_error_tracking.sql` | Job error tracking |
| `009_postgres_system_stats_cpu_enhancement.sql` | Extended `postgres_system_stats` CPU columns |
| `010_rule_engine_phase2_1.sql` | SQL Server rule-engine Epic 2.1 first-batch metadata refresh |
| `011_log_shipping_health_epic2_2.sql` | SQL Server log-shipping health Timescale hypertable (Epic 2.2) |

## Usage

### Initial setup (new TimescaleDB)

1. Connect to TimescaleDB (e.g. `dbmonitor_metrics`).
2. Run the main schema:
   ```sql
   \i 01_timescale_schema.sql
   ```
3. Run seed data:
   ```sql
   \i 06_seed_data.sql
   ```
4. Optional: `\i 02_rule_engine.sql` if you use the rule engine.

### Rule engine scripts (historical note)

- The legacy `rule_engine/` folder (with numbered SQL fragments like `001_*`, `006_*`, `007_*`) has been **retired**.
- The **only** supported install path for the rule engine schema is now: `02_rule_engine.sql`.

### Upgrading an existing database

Run only the migration files you have not applied yet, in order (e.g. `\i migrations/010_rule_engine_phase2_1.sql`).

### Collection and fetch scripts

- **`collection/`** — referenced by metric collectors / agent configuration; not executed as a single bundle.
- **`timescaledb_fetch/`** — used by the backend or agents for Timescale inserts/selects; paths are typically configured per metric name.

### Docker Compose

- The infra stack under `infrastructure/docker/` mounts this directory and applies core scripts on startup.
- The root-level `docker/docker-compose.yml` also mounts this directory via `../infrastructure/sql_scripts:/sql_scripts:ro` for schema setup.
- Subfolders are available for manual or scripted use.

## Schema overview

### SQL Server metrics

Core and enterprise hypertables: metrics, CPU/memory/wait history, connections, locks, disks, throughput, query store, AG health, jobs, and extended performance objects as defined in `01_timescale_schema.sql` and migrations.

### PostgreSQL metrics

Throughput, connections, replication, system stats, database stats, sessions, locks, tables, indexes, queries, config, long-running queries, bgwriter, archiver, etc.

### Cross-engine storage and index health

`monitor.index_usage_stats`, `monitor.table_usage_stats`, `monitor.table_size_history` (see main schema).

### Application tables

Users, widgets, dashboards, alerts, monitored servers, `postgres_query_dictionary`, and related tables.

## Default credentials

| Username | Password | Role |
|----------|----------|------|
| admin | admin123 | admin |
| viewer | admin123 | viewer |

Change these in production.

## Compression

Hypertables typically use compression for chunks older than 7 days. See `01_timescale_schema.sql` for policies.

## PostgreSQL Recommended Configuration

For optimal monitoring of PostgreSQL instances, it is highly recommended to enable and configure the `pg_stat_statements` and `pg_stat_monitor` extensions.

### Step 1: Enable Extensions
Search for `shared_preload_libraries` in your `postgresql.conf` and add the extensions:
```conf
shared_preload_libraries = 'pg_stat_statements,pg_stat_monitor'
```

### Step 2: Configure pg_stat_statements
Add the following baseline configuration to the end of `postgresql.conf`:
```conf
pg_stat_statements.max = 10000
pg_stat_statements.track = all
pg_stat_statements.save = on
pg_stat_statements.track_utility = on
```

### Step 3: Configure pg_stat_monitor
Add the following recommended starting configuration to `postgresql.conf`:
```conf
pg_stat_monitor.pgsm_max = 10000
pg_stat_monitor.pgsm_bucket_time = 60     -- 1 min buckets (perfect for dashboards)
pg_stat_monitor.pgsm_max_buckets = 1440   -- 24 hours of buckets
pg_stat_monitor.pgsm_track = all
pg_stat_monitor.pgsm_track_utility = on
pg_stat_monitor.pgsm_normalized_query = on
pg_stat_monitor.pgsm_enable_query_plan = on
pg_stat_monitor.pgsm_track_application_names = on
```

### Step 4: Create Extensions
Connect to your monitored PostgreSQL instance and run the following SQL:
```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
CREATE EXTENSION IF NOT EXISTS pg_stat_monitor;
```
**Important:** A restart of the PostgreSQL service is required after modifying `shared_preload_libraries` in `postgresql.conf` for the changes to take effect.

### Benefits
- **1-minute granularity**
- **24 hours rolling history** inside Postgres
- **Rich metadata** for dashboards
- **Efficient collection**: The collector can safely poll every 30–60 seconds.

### Architecture
Postgres (`pg_stat_monitor` → primary, `pg_stat_statements` → fallback) → Collector → TimescaleDB → Dashboards

## Troubleshooting

1. TimescaleDB extension: `SELECT * FROM pg_extension WHERE extname = 'timescaledb';`
2. Clean Docker volumes if reinitializing from scratch.
3. Migrations require a user with sufficient privileges.
4. **`relation "optima_server_dr_policy" does not exist`**: The table is created early in `01_timescale_schema.sql` and by `07_optima_server_dr_policy.sql`. Older volumes may have stopped applying `01` before the end of the file (e.g. `ruleengine.rules` not present yet). Run:
   ```bash
   docker compose -f infrastructure/docker/docker-compose.yml run --rm schema-patches
   ```
   Or apply manually: `psql ... -f infrastructure/sql_scripts/07_optima_server_dr_policy.sql`
5. **`relation "optima_notification_config" does not exist`** (startup log: `notifier: could not load config from DB`): Re-run `07_optima_server_dr_policy.sql` via `schema-patches` (same command as item 4 above).
