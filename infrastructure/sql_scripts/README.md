# SQL Optima - TimescaleDB Schema

This directory contains the unified SQL scripts for the SQL Optima monitoring tool.

## Files

| File | Description |
|------|-------------|
| `00_timescale_schema.sql` | Main schema - all core hypertable and table definitions |
| `01_seed_data.sql` | Default users, widgets, and collection schedules |
| `02_additional_sqlserver_metrics.sql` | Extended SQL Server metrics (from Eric Darling's Performance Monitor) |
| `SST.sql` | System Setup Test - sanity check script |

## Usage

### Initial Setup

1. **Connect to TimescaleDB:**
   ```bash
   psql -h localhost -p 5432 -U postgres -d dbmonitor_metrics
   ```

2. **Run the main schema:**
   ```sql
   \i 00_timescale_schema.sql
   ```

3. **Run additional SQL Server metrics (optional - for extended monitoring):**
   ```sql
   \i 02_additional_sqlserver_metrics.sql
   ```

4. **Run seed data:**
   ```sql
   \i 01_seed_data.sql
   ```

5. **Run sanity check:**
   ```sql
   \i SST.sql
   ```

### Docker Compose Usage

The schema is automatically applied during container initialization via:
```yaml
volumes:
  - ./sql_scripts:/docker-entrypoint-initdb.d
```

## Schema Overview

### SQL Server Metrics (48 hypertables in core + additional)
- Core: metrics, cpu_history, memory_history, wait_history, file_io_history, connection_history, lock_history, disk_history, top_queries
- Enterprise: database_throughput, query_store_stats, ag_health
- Jobs: job_metrics, job_details, agent_schedules, job_failures
- Advanced: latch_waits, memory_clerks, waiting_tasks, procedure_stats, spinlock_stats, tempdb_stats, database_size, server_config, database_config, memory_grants, scheduler_wg, file_io_latency, qs_runtime
- Extended (from Eric Darling's Performance Monitor): wait_stats, memory_stats, memory_pressure_events, cpu_utilization_stats, deadlock_xml, blocked_process_xml, blocking_deadlock_stats, perfmon_stats, cpu_scheduler_stats, latch_stats, spinlock_stats_v2, plan_cache_stats, session_stats, tempdb_stats_v2, server_properties, critical_issues, database_size_v2, trace_flags_history

### PostgreSQL Metrics (14 hypertables)
- Core: throughput_metrics, connection_stats, replication_stats, system_stats, database_stats, session_stats, lock_stats, table_stats, index_stats, query_stats, config_settings, long_running_queries
- Enterprise: bgwriter_stats, archiver_stats

### Application Tables (13 regular tables)
- optima_users, optima_ui_widgets, user_dashboards, dashboard_widgets
- alert_thresholds, notification_channels, alert_subscriptions, alert_history
- monitored_servers, metric_collection_settings, dashboard_exports
- postgres_query_dictionary, query_store_stats

### Management Tables
- sqlserver_collection_schedule, sqlserver_collection_log

## Default Credentials

| Username | Password | Role |
|----------|----------|------|
| admin | admin123 | admin |
| viewer | admin123 | viewer |

**IMPORTANT:** Change these passwords in production!

## Compression Policy

All hypertables have compression enabled for chunks older than 7 days. Configuration tables use 30-day retention.

## Troubleshooting

If SST fails, check:
1. TimescaleDB extension is installed: `SELECT * FROM pg_extension WHERE extname = 'timescaledb';`
2. Docker volume is clean (delete `timescaledb_data` volume if reinitializing)
3. User has superuser privileges
