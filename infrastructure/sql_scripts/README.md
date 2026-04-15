# SQL Optima — TimescaleDB and SQL scripts

This directory is the **single source** for SQL Optima database scripts (merged from the former `sql_scripts_used/` tree).

## Root-level scripts

| File | Description |
|------|-------------|
| `00_timescale_schema.sql` | Main TimescaleDB schema: hypertables, indexes, compression |
| `01_seed_data.sql` | Default users, widgets, and collection schedules |
| `02_rule_engine.sql` | Rule engine schema (if used) |
| `pgsql_init.sql` | PostgreSQL instance bootstrap helper |
| `sqlserver_init.sql` | SQL Server–side helper scripts |
| `SST.sql` | *(Optional)* System setup test — include in repo if you maintain it |
| `02_additional_sqlserver_metrics.sql` | *(Optional)* Extended SQL Server metrics — include if maintained separately |

## Supplementary directories

| Directory | Description |
|-----------|-------------|
| `collection/` | **~70** parameterized queries used by collectors against SQL Server and PostgreSQL (metrics, waits, jobs, PG stats, etc.) |
| `timescaledb_fetch/` | **~48** read/insert templates for TimescaleDB (historical series, summaries, hypertable helpers) |
| `migrations/` | Ordered **incremental** migrations for existing deployments (run after `00_timescale_schema.sql` when upgrading) |
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

## Usage

### Initial setup (new TimescaleDB)

1. Connect to TimescaleDB (e.g. `dbmonitor_metrics`).
2. Run the main schema:
   ```sql
   \i 00_timescale_schema.sql
   ```
3. Run seed data:
   ```sql
   \i 01_seed_data.sql
   ```
4. Optional: `\i 02_rule_engine.sql` if you use the rule engine.

### Upgrading an existing database

Run only the migration files you have not applied yet, in order (e.g. `\i migrations/009_postgres_system_stats_cpu_enhancement.sql`).

### Collection and fetch scripts

- **`collection/`** — referenced by metric collectors / agent configuration; not executed as a single bundle.
- **`timescaledb_fetch/`** — used by the backend or agents for Timescale inserts/selects; paths are typically configured per metric name.

### Docker Compose

The `infrastructure/docker` stack may mount this folder (e.g. `../sql_scripts:/sql_scripts:ro`) and run `00_timescale_schema.sql` and `01_seed_data.sql` on startup. Subfolders are available for manual or scripted use.

## Schema overview

### SQL Server metrics

Core and enterprise hypertables: metrics, CPU/memory/wait history, connections, locks, disks, throughput, query store, AG health, jobs, and extended performance objects as defined in `00_timescale_schema.sql` and migrations.

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

Hypertables typically use compression for chunks older than 7 days. See `00_timescale_schema.sql` for policies.

## Troubleshooting

1. TimescaleDB extension: `SELECT * FROM pg_extension WHERE extname = 'timescaledb';`
2. Clean Docker volumes if reinitializing from scratch.
3. Migrations require a user with sufficient privileges.
