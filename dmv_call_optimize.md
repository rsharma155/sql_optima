# DMV Call Consolidation Plan

**Project:** SQL Optima — SQL Server Collector Optimization
**Problem:** Multiple Go files independently query the same SQL Server DMVs, creating redundant network round-trips to the monitored server on every collection cycle. This inflates load on production servers, increases monitoring overhead, and produces duplicate data paths.
**Goal:** One DMV → one unified collector → one TimescaleDB table → all Go files read from TimescaleDB, not from the monitored server.

---

## How to Read This Document

Each section covers one DMV (or tightly coupled DMV group). Structure:

- **Current state**: which files hit it and what columns they each need
- **Union of all required columns**: the superset one unified query must capture
- **Target TimescaleDB table**: DDL for the unified storage table (new or extended)
- **Unified collector**: one Go function that runs once per cycle and writes all columns
- **Files to refactor**: every file that currently hits the DMV directly must be changed to read from TimescaleDB instead

---

## Tier 1 — Critical (Highest Redundancy, Most Server Load)

---

### DMV-01: `sys.dm_os_performance_counters`

**Impact: 14 Go files, 15 separate SQL queries to the monitored server per collection cycle.**

Each file issues its own `SELECT ... WHERE counter_name = '...'` query. This means up to 15 individual SQL round-trips per collection cycle just for performance counters.

#### Current State

| File | Counters Queried | Line | Stored To |
|---|---|---|---|
| `sqlserver_memory_analyzer.go` | Total Server Memory (KB), Target Server Memory (KB) | 32-45 | `sqlserver_memory_metrics` |
| `sqlserver_memory_analyzer.go` | Memory Grants Pending | 77-86 | `sqlserver_memory_metrics` |
| `sqlserver_memory_analyzer.go` | Page life expectancy | 123-132 | `sqlserver_memory_metrics` |
| `sqlserver_memory_analyzer.go` | Sort Warnings, Hash Warnings | 147-167 | `sqlserver_memory_metrics` |
| `sqlserver_memory.go` | Page life expectancy | 19 | Live only |
| `sqlserver_memory.go` | Buffer Pool Size (KB) | 28 | Live only |
| `sqlserver_cpu.go` | Batch Requests/sec | 32 | Live only |
| `unified_pulses.go` | Batch Requests/sec, SQL Compilations/sec, Page life expectancy, User Connections | 130-133 | `SQLServerTier2Row` |
| `sqlserver_phase2.go` | Dynamic (caller-specified) | 114-117 | Live only |
| `sqlserver_database_throughput.go` | Transactions/sec | 98-99 | Live only |
| `live/sqlserver_repository.go` | Batch Requests/sec | 37 | Live only |
| `sqlserver_health_worker.go` | Page Reads/sec, Batch Requests/sec, SQL Compilations/sec, Logins/sec | 48-53 | Health metrics |
| `sqlserver_health_worker.go` | Target Server Memory (KB), Total Server Memory (KB) | 168-169 | Health KPIs |
| `sqlserver_health_worker.go` | Page life expectancy, Buffer cache hit ratio, Buffer cache hit ratio base | 255-256 | `sqlserver_risk_health` |

#### Union of All Required Columns

```sql
-- One unified query fetches ALL 15 counters in a single round-trip:
SELECT
    counter_name,
    instance_name,
    cntr_value,
    cntr_type,
    object_name
FROM sys.dm_os_performance_counters WITH (NOLOCK)
WHERE counter_name IN (
    N'Page life expectancy',
    N'Buffer Pool Size (KB)',
    N'Total Server Memory (KB)',
    N'Target Server Memory (KB)',
    N'Memory Grants Pending',
    N'Batch Requests/sec',
    N'Page Reads/sec',
    N'SQL Compilations/sec',
    N'Logins/sec',
    N'User Connections',
    N'Transactions/sec',
    N'Buffer cache hit ratio',
    N'Buffer cache hit ratio base',
    N'Sort Warnings/sec',
    N'Hash Warnings/sec'
)
AND object_name NOT LIKE '%$%';  -- exclude mirrored/availability group duplicates
```

#### Target TimescaleDB Table: `sqlserver_perf_counters`

```sql
CREATE TABLE IF NOT EXISTS sqlserver_perf_counters (
    capture_timestamp  TIMESTAMPTZ    NOT NULL,
    server_id          UUID           NOT NULL REFERENCES monitored_servers(id),
    counter_name       VARCHAR(128)   NOT NULL,
    instance_name      VARCHAR(128)   NOT NULL DEFAULT '',
    cntr_value         BIGINT         NOT NULL DEFAULT 0,
    cntr_type          INT            NOT NULL DEFAULT 0,
    -- Derived rate (computed by collector using delta against previous snapshot)
    rate_per_sec       DOUBLE PRECISION
);
SELECT create_hypertable('sqlserver_perf_counters', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_perf_counters', INTERVAL '90 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_pc_server_counter
    ON sqlserver_perf_counters (server_id, counter_name, capture_timestamp DESC);
```

#### Unified Collector

New file: `backend/internal/collectors/infrastructure/sqlserver/sqlserver_perf_counters_collector.go`

```go
// Runs once per collection cycle. Fetches all 15 counters in one query.
// Computes delta-based rates (Batch Requests/sec etc.) against the previous snapshot.
// Writes one row per counter to sqlserver_perf_counters.
func (c *PerfCountersCollector) Collect(ctx context.Context, db *sql.DB, serverID uuid.UUID) error
```

#### Files to Refactor (stop hitting DMV directly)

| File | Change Required |
|---|---|
| `sqlserver_memory_analyzer.go` | Remove 4 separate counter queries; read `Total Server Memory`, `Target Server Memory`, `Memory Grants Pending`, `Page life expectancy`, `Sort Warnings`, `Hash Warnings` from `sqlserver_perf_counters` |
| `sqlserver_memory.go` | Remove PLE and Buffer Pool queries; read from `sqlserver_perf_counters` |
| `sqlserver_cpu.go` | Remove Batch Requests query; read from `sqlserver_perf_counters` |
| `unified_pulses.go` | Remove perf counter sub-query in Tier2 mega-query; source from `sqlserver_perf_counters` |
| `sqlserver_database_throughput.go` | Remove Transactions/sec query; read from `sqlserver_perf_counters WHERE counter_name = 'Transactions/sec'` |
| `live/sqlserver_repository.go` | Remove Batch Requests live query; read latest row from `sqlserver_perf_counters` |
| `sqlserver_health_worker.go` | Remove all 5 separate counter reads; source all health KPIs from `sqlserver_perf_counters` |
| `sqlserver_phase2.go` | Deprecate `FetchPerfCounters`; consumers read from `sqlserver_perf_counters` |

---

### DMV-02: `sys.dm_io_virtual_file_stats` (+ `sys.master_files` join)

**Impact: 6 Go files, 5 separate queries to the monitored server per collection cycle.**

#### Current State

| File | Function | Columns from VFS | Columns from master_files | Stored To |
|---|---|---|---|---|
| `sqlserver_io.go` | `CollectFileIOLatencyForRTD` | database_id, file_id, num_of_reads, num_of_writes, io_stall_read_ms, io_stall_write_ms | name, type_desc, size | Collector (TimescaleDB) |
| `sqlserver_io.go` | `CollectFileIOLatency` | Same 6 columns | Same | Collector |
| `sqlserver_database_throughput.go` | `FetchDatabaseThroughput` | num_of_reads, num_of_writes, num_of_bytes_read, num_of_bytes_written, io_stall_read_ms, io_stall_write_ms | database_id (join via sys.databases) | `sqlserver_database_throughput` |
| `unified_pulses.go` | `FetchTier3MegaQueryIO` | ALL 10 columns | type_desc, physical_name | `staging.sqlserver_io_raw` (bug: 6/15 written) |
| `sqlserver_wait_stats_service.go` | `collectFileIOLatency` | num_of_reads, num_of_bytes_read, io_stall_read_ms, num_of_writes, num_of_bytes_written, io_stall_write_ms | name, type_desc | Live delta calculation |
| `live/sqlserver_repository.go` | `GetIOLatency` | io_stall_read_ms, num_of_reads, io_stall_write_ms, num_of_writes | — | Live only |

#### Union of All Required Columns (from VFS)

`database_id`, `file_id`, `num_of_reads`, `num_of_writes`, `num_of_bytes_read`, `num_of_bytes_written`, `io_stall_read_ms`, `io_stall_write_ms`, `io_stall`, `size_on_disk_bytes`

#### Union of All Required Columns (from master_files in these joins)

`database_id`, `file_id`, `name` (logical file name), `type_desc`, `physical_name`, `size`

#### Target TimescaleDB Table: Fix `staging.sqlserver_io_raw`

The table already has all 15 columns defined but the INSERT only writes 6. **This is a bug, not a design gap.**

```sql
-- Existing table schema is correct. Only the INSERT needs to be fixed.
-- staging.sqlserver_io_raw columns:
-- capture_timestamp, server_id, database_id, db_name, file_id,
-- type_desc, physical_name, num_of_reads, num_of_writes,
-- num_of_bytes_read, num_of_bytes_written,
-- io_stall_read_ms, io_stall_write_ms, io_stall, size_on_disk_bytes
```

Fix `ts_logger_pulses.go:108` — change INSERT from 6 to all 15 columns (the source query already selects them).

#### Additional: `sqlserver_file_catalog` (static file metadata — replaces repeated master_files hits)

```sql
-- Stores file metadata that changes rarely (physical paths, sizes, type).
-- Refreshed every 6 hours. Avoids joining master_files in every IO query.
CREATE TABLE IF NOT EXISTS sqlserver_file_catalog (
    capture_timestamp  TIMESTAMPTZ    NOT NULL,
    server_id          UUID           NOT NULL REFERENCES monitored_servers(id),
    database_id        INT            NOT NULL,
    file_id            INT            NOT NULL,
    database_name      VARCHAR(128),
    logical_file_name  VARCHAR(128),
    physical_name      TEXT,
    type_desc          VARCHAR(10),   -- 'ROWS' or 'LOG'
    size_mb            FLOAT,
    max_size_mb        FLOAT,
    growth             INT,
    is_percent_growth  BOOLEAN,
    state_desc         VARCHAR(20)
);
SELECT create_hypertable('sqlserver_file_catalog', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_file_catalog', INTERVAL '30 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_fc_server_file
    ON sqlserver_file_catalog (server_id, database_id, file_id, capture_timestamp DESC);
```

#### Unified Collector

New file: `backend/internal/collectors/infrastructure/sqlserver/sqlserver_io_collector.go`

```go
// Runs once per collection cycle.
// 1. Queries sys.dm_io_virtual_file_stats JOIN sys.master_files — ONE query, all 15 columns.
// 2. Writes to staging.sqlserver_io_raw (all 15 columns — fixes the existing bug).
// 3. Writes file metadata to sqlserver_file_catalog if stale (> 6h since last write).
func (c *IOCollector) Collect(ctx context.Context, db *sql.DB, serverID uuid.UUID) error
```

#### Files to Refactor

| File | Change Required |
|---|---|
| `sqlserver_io.go` | Remove `CollectFileIOLatency` and `CollectFileIOLatencyForRTD`; read from `staging.sqlserver_io_raw` |
| `sqlserver_database_throughput.go` | Remove VFS sub-query; compute throughput deltas from `staging.sqlserver_io_raw` |
| `unified_pulses.go` | Remove VFS Tier3 query; `staging.sqlserver_io_raw` is already the target |
| `sqlserver_wait_stats_service.go` | Remove `collectFileIOLatency`; read deltas from `staging.sqlserver_io_raw` |
| `live/sqlserver_repository.go` | `GetIOLatency` reads latest 2 rows from `staging.sqlserver_io_raw` and computes live latency |

---

### DMV-03: `sys.dm_os_sys_memory`

**Impact: 6 Go files hitting the same 3-column DMV independently.**

#### Current State

| File | Columns | Stored To |
|---|---|---|
| `sqlserver_memory_analyzer.go:53` | total_physical_memory_kb, available_physical_memory_kb | `sqlserver_memory_metrics` |
| `sqlserver_memory.go:47` | total_physical_memory_kb, available_physical_memory_kb | Live (percentage calc) |
| `sqlserver_cpu.go:30` | total_physical_memory_kb, available_physical_memory_kb | Live KPI |
| `sqlserver_cpu.go:174` | total_physical_memory_kb, available_physical_memory_kb, system_memory_state_desc | `sqlserver_cpu_scheduler_stats` |
| `unified_pulses.go:136` | total_physical_memory_kb, available_physical_memory_kb | `SQLServerTier2Row` |
| `live/sqlserver_repository.go:35` | total_physical_memory_kb, available_physical_memory_kb | Live only |

#### Union of All Required Columns

`total_physical_memory_kb`, `available_physical_memory_kb`, `system_memory_state_desc`

#### Target: Extend `sqlserver_memory_metrics` (already stores 2 of 3 columns)

```sql
-- Add the missing column to the existing table:
ALTER TABLE sqlserver_memory_metrics
    ADD COLUMN IF NOT EXISTS os_system_memory_state VARCHAR(60) DEFAULT '';
```

The `sqlserver_memory_analyzer.go` collector already queries this DMV and writes to `sqlserver_memory_metrics`. Add `system_memory_state_desc` to that one query. All other files stop querying the DMV and read from `sqlserver_memory_metrics` instead.

#### Files to Refactor

| File | Change Required |
|---|---|
| `sqlserver_memory.go` | Remove sys.dm_os_sys_memory query; read `os_total_memory_mb` / `os_available_memory_mb` from `sqlserver_memory_metrics` latest row |
| `sqlserver_cpu.go:30` | Remove inline subquery; read from `sqlserver_memory_metrics` |
| `sqlserver_cpu.go:174` | Remove inline subquery for memory; read from `sqlserver_memory_metrics` for memory columns |
| `unified_pulses.go` | Remove SYS_MEM sub-query from Tier2; source from `sqlserver_memory_metrics` |
| `live/sqlserver_repository.go` | Read latest memory from `sqlserver_memory_metrics` (< 2 min old) |

---

### DMV-04: `sys.dm_os_sys_info`

**Impact: 7 Go files. Most columns (CPU count, memory, start time) are queried redundantly across service, repository, and collector layers.**

#### Current State

| File | Columns | Stored To |
|---|---|---|
| `sqlserver_performance_debt.go:59` | sqlserver_start_time | Live only |
| `sqlserver_cpu.go:59` | ms_ticks | Live (timestamp calc) |
| `sqlserver_cpu.go:149-174` | cpu_count, scheduler_count, max_workers_count | `sqlserver_cpu_scheduler_stats` |
| `sqlserver_cpu.go:228` | cpu_count, hyperthread_ratio, socket_count, cores_per_socket, physical_memory_kb, virtual_memory_kb, max_workers_count | `sqlserver_server_properties` |
| `sqlserver_query_snapshot.go:77` | sqlserver_start_time | Live (int64 return) |
| `sqlserver_health_worker.go:172` | sqlserver_start_time | `sqlserver_health_v2_kpis` (uptime_seconds) |
| `sqlserver_health_worker.go:232` | cpu_count, hyperthread_ratio, physical_memory_kb, virtual_memory_kb, socket_count, cores_per_socket, max_workers_count | `sqlserver_server_properties` |

#### Union of All Required Columns

`sqlserver_start_time`, `ms_ticks`, `cpu_ticks`, `cpu_count`, `hyperthread_ratio`, `socket_count`, `cores_per_socket`, `scheduler_count`, `physical_memory_kb`, `virtual_memory_kb`, `max_workers_count`

#### Target: Extend `sqlserver_server_properties` (already stores most hardware columns)

```sql
-- Add missing columns to existing table:
ALTER TABLE sqlserver_server_properties
    ADD COLUMN IF NOT EXISTS sqlserver_start_time  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS ms_ticks              BIGINT,
    ADD COLUMN IF NOT EXISTS scheduler_count       INT;
```

The existing `CollectServerProperties` in `sqlserver_cpu.go:228` already writes to `sqlserver_server_properties`. Extend that single query to also capture `sqlserver_start_time`, `ms_ticks`, and `scheduler_count`.

#### Files to Refactor

| File | Change Required |
|---|---|
| `sqlserver_performance_debt.go:59` | Read `sqlserver_start_time` from `sqlserver_server_properties` latest row for this server |
| `sqlserver_cpu.go:59` | Read `ms_ticks` from `sqlserver_server_properties`; remove standalone sys_info query |
| `sqlserver_cpu.go:149` | Read cpu_count, scheduler_count from `sqlserver_server_properties`; remove duplicate sys_info query |
| `sqlserver_query_snapshot.go:77` | Read `sqlserver_start_time` from `sqlserver_server_properties` |
| `sqlserver_health_worker.go:172` | Read `sqlserver_start_time` from `sqlserver_server_properties` |
| `sqlserver_health_worker.go:232` | Remove duplicate server property collection; already written by `sqlserver_cpu.go` collector |

---

## Tier 2 — High Duplication, Moderate Load

---

### DMV-05: `sys.dm_exec_sessions` + `sys.dm_exec_requests`

**Impact: 11 files (sessions) + 9 files (requests). Often queried together in the same round-trip.**

These DMVs are heavily used for both live dashboards (current state) and historical analysis (connection trends, blocking history). The approach is two-tiered:

**Live consumers** (API handlers that need the current snapshot): all route through a single shared `FetchActiveSessionsSnapshot()` function — not duplicate SQL in each handler.

**Historical consumers** (collectors that store trends): one background collector writes to a unified short-retention table.

#### Union of All Required Columns (sessions)

`session_id`, `login_name`, `host_name`, `program_name`, `client_interface_name`, `status`, `database_id`, `open_transaction_count`, `is_user_process`, `memory_usage`, `transaction_isolation_level`

#### Union of All Required Columns (requests)

`session_id`, `request_id`, `database_id`, `status`, `command`, `start_time`, `wait_type`, `wait_time`, `last_wait_type`, `blocking_session_id`, `cpu_time`, `total_elapsed_time`, `logical_reads`, `reads`, `writes`, `row_count`, `granted_query_memory`, `dop`, `percent_complete`, `sql_handle`, `plan_handle`, `query_hash`, `query_plan_hash`, `statement_start_offset`, `statement_end_offset`

#### Target TimescaleDB Tables

These already partially exist. Extend and unify:

**`sqlserver_active_sessions`** (30-day retention — point-in-time snapshot):
```sql
CREATE TABLE IF NOT EXISTS sqlserver_active_sessions (
    capture_timestamp         TIMESTAMPTZ  NOT NULL,
    server_id                 UUID         NOT NULL REFERENCES monitored_servers(id),
    session_id                INT          NOT NULL,
    login_name                VARCHAR(128),
    host_name                 VARCHAR(128),
    program_name              VARCHAR(256),
    client_interface_name     VARCHAR(32),
    status                    VARCHAR(30),
    database_id               INT,
    database_name             VARCHAR(128),
    open_transaction_count    INT,
    is_user_process           BOOLEAN,
    memory_usage              INT,
    transaction_isolation_level SMALLINT,
    -- From dm_exec_requests (NULL if session is not executing):
    request_status            VARCHAR(30),
    command                   VARCHAR(32),
    wait_type                 VARCHAR(128),
    wait_time_ms              INT,
    blocking_session_id       INT,
    cpu_time_ms               BIGINT,
    total_elapsed_ms          BIGINT,
    logical_reads             BIGINT,
    reads                     BIGINT,
    writes                    BIGINT,
    granted_query_memory_kb   BIGINT,
    dop                       INT,
    sql_handle                BYTEA,
    query_hash                BYTEA
);
SELECT create_hypertable('sqlserver_active_sessions', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_active_sessions', INTERVAL '30 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_as_server_ts
    ON sqlserver_active_sessions (server_id, capture_timestamp DESC);
```

#### Unified Collector

```go
// Runs every 30 seconds. One query joins dm_exec_sessions + dm_exec_requests.
// Writes to sqlserver_active_sessions.
// All blocking detection, connection history, wait analysis reads from this table.
func (c *SessionCollector) Collect(ctx context.Context, db *sql.DB, serverID uuid.UUID) error
```

#### Files to Refactor

| File | Change |
|---|---|
| `sqlserver_blocking.go` `FetchBlockingSnapshots` | Read from `sqlserver_active_sessions WHERE blocking_session_id > 0` |
| `sqlserver_connection_stats.go` | Read from `sqlserver_active_sessions` for connection counts |
| `sqlserver_long_running_queries.go` | Read from `sqlserver_active_sessions WHERE total_elapsed_ms > threshold` |
| `sqlserver_queries.go` `CollectLongRunningQueries` | Same as above |
| `sqlserver_health_worker.go` session/connection queries | Source from `sqlserver_active_sessions` |
| `sqlserver_session_enrichment.go` | Read from `sqlserver_active_sessions` |
| Live handlers (`sqlserver_live.go`) | Keep live query but consolidate into one shared `FetchActiveSessionsSnapshot()` |
| `sqlserver_wait_stats_collector.go` `FetchActiveWaitSessions` | Source session context from `sqlserver_active_sessions`; waiting tasks from `sqlserver_waiting_tasks` |

---

### DMV-06: `sys.dm_os_wait_stats`

**Impact: 4 Go files. Partially unified — one collector already writes to `sqlserver_wait_stats_delta`. Two files bypass it.**

#### Current State

| File | Function | Columns | Stored To |
|---|---|---|---|
| `sqlserver_wait_stats_collector.go` | `FetchCumulativeSnapshot` | wait_type, waiting_tasks_count, wait_time_ms, signal_wait_time_ms, resource_wait_time_ms | `sqlserver_wait_stats_delta` ✅ |
| `sqlserver_phase2.go` | `FetchWaitStatsCumulative` | wait_type, wait_time_ms | Live only ❌ duplicate |
| `live/sqlserver_repository.go` | `GetWaitStats` | wait_type, waiting_tasks_count, wait_time_ms | Live only ❌ duplicate |
| `sqlserver_health_worker.go` | Anonymous inline | wait_type, wait_time_ms (aggregated by category) | `sqlserver_wait_history` |

#### Fix

- `sqlserver_phase2.go:FetchWaitStatsCumulative` → read from `sqlserver_wait_stats_delta` (latest snapshot)
- `live/sqlserver_repository.go:GetWaitStats` → read latest 2 snapshots from `sqlserver_wait_stats_delta`, compute live delta
- `sqlserver_health_worker.go` inline wait query → read from `sqlserver_wait_stats_delta`, apply category grouping in Go code

---

### DMV-07: `sys.dm_db_index_usage_stats`

**Impact: 3 Go files. One already writes to `monitor.index_usage_stats` — the other two bypass it.**

#### Current State

| File | Function | Columns | Stored To |
|---|---|---|---|
| `index_usage_collector.go` | `CollectSQLServerIndexUsage` | user_seeks, user_scans, user_lookups, user_updates, last_user_seek, last_user_scan, last_user_lookup | `monitor.index_usage_stats` ✅ |
| `sqlserver_index.go` | `CollectSQLServerIndexUsageMetrics` | user_seeks, user_scans, user_lookups, user_updates, last_user_seek | Live only ❌ duplicate |
| `sqlserver_performance_debt.go` | `FetchUnusedIndexes` | user_seeks, user_scans, user_lookups, user_updates | Live only ❌ — should read from TimescaleDB |

#### Fix

- `sqlserver_index.go:CollectSQLServerIndexUsageMetrics` → read latest row from `monitor.index_usage_stats` for the requested index; delete the DMV query
- `sqlserver_performance_debt.go:FetchUnusedIndexes` → read from `monitor.index_usage_stats` (latest snapshot per index); this eliminates the expensive per-database DMV scan from the performance debt worker

---

### DMV-08: `sys.dm_exec_query_memory_grants`

**Impact: 8 Go files. Only 1 writes to TimescaleDB; 7 issue live queries for the same data.**

#### Current State

| File | Columns | Stored To |
|---|---|---|
| `sqlserver_memory.go:CollectMemoryGrants` | session_id, request_id, database_id, login_name, granted_memory_kb, used_memory_kb, max_used_memory_kb, dop, query_duration_sec | Live only |
| `sqlserver_phase2.go:FetchMemoryGrantsPending` | grant_time (IS NULL count) | Live only |
| `sqlserver_phase2.go:FetchMemoryGrantsSummary` | grant_time, granted_memory_kb | Live only |
| `sqlserver_enterprise_additions.go:CollectMemoryGrantWaiters` | session_id, request_id, database_id, login_name, requested_memory_kb, granted_memory_kb, required_memory_kb, wait_time_ms, dop | Live only |
| `sqlserver_tempdb.go` (inline) | session_id, requested_memory_kb, granted_memory_kb, used_memory_kb, query_cost | Live only |
| `sqlserver_memory_analyzer.go` (inline) | granted_memory_kb, requested_memory_kb, grant_time | Live only |
| `sqlserver_health_worker.go:162` | grant_time (count WHERE IS NULL) | `sqlserver_health_v2_kpis` |
| `unified_pulses.go:244` | session_id, requested_memory_kb, granted_memory_kb, used_memory_kb, max_used_memory_kb, request_time, grant_time, query_cost | `SQLServerMemoryGrantRow` (pulse table) |

#### Union of All Required Columns

`session_id`, `request_id`, `database_id`, `login_name`, `granted_memory_kb`, `requested_memory_kb`, `used_memory_kb`, `max_used_memory_kb`, `required_memory_kb`, `dop`, `grant_time`, `request_time`, `wait_time_ms`, `query_cost`

#### Target TimescaleDB Table: `sqlserver_memory_grants`

```sql
CREATE TABLE IF NOT EXISTS sqlserver_memory_grants (
    capture_timestamp    TIMESTAMPTZ   NOT NULL,
    server_id            UUID          NOT NULL REFERENCES monitored_servers(id),
    session_id           INT           NOT NULL,
    request_id           INT,
    database_name        VARCHAR(128),
    login_name           VARCHAR(128),
    granted_memory_kb    BIGINT,
    requested_memory_kb  BIGINT,
    used_memory_kb       BIGINT,
    max_used_memory_kb   BIGINT,
    required_memory_kb   BIGINT,
    dop                  INT,
    grant_time           TIMESTAMPTZ,    -- NULL = waiting for grant
    request_time         TIMESTAMPTZ,
    wait_time_ms         BIGINT,
    query_cost           FLOAT,
    is_waiting           BOOLEAN GENERATED ALWAYS AS (grant_time IS NULL) STORED
);
SELECT create_hypertable('sqlserver_memory_grants', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_memory_grants', INTERVAL '30 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_mg_server_ts
    ON sqlserver_memory_grants (server_id, capture_timestamp DESC);
```

#### Unified Collector

Single collector queries the DMV once, writes all 14 columns to `sqlserver_memory_grants`. Add to the existing `sqlserver_memory_analyzer.go` collector or as Phase N of the health worker.

#### Files to Refactor

All 8 files above replace their direct DMV queries with reads from `sqlserver_memory_grants` latest snapshot (within 2 minutes).

---

## Tier 3 — Lower Duplication, Implement After Tier 1+2

---

### DMV-09: `sys.dm_db_index_physical_stats`

**Impact: 2 Go files. Both query it live; neither stores results.**

| File | Function | Columns | Stored To |
|---|---|---|---|
| `sqlserver_index.go` | `CollectSQLServerIndexFragmentationMetrics` | avg_fragmentation_in_percent, page_count | Live only |
| `sqlserver_performance_debt.go` | `FetchIndexFragmentation` | avg_fragmentation_in_percent, page_count | Live (findings generated) |

#### Target TimescaleDB Table: `sqlserver_index_fragmentation`

```sql
CREATE TABLE IF NOT EXISTS sqlserver_index_fragmentation (
    capture_timestamp            TIMESTAMPTZ   NOT NULL,
    server_id                    UUID          NOT NULL REFERENCES monitored_servers(id),
    database_name                VARCHAR(128)  NOT NULL,
    schema_name                  VARCHAR(128),
    table_name                   VARCHAR(256)  NOT NULL,
    index_name                   VARCHAR(256),
    index_id                     INT           NOT NULL,
    index_type                   INT,
    avg_fragmentation_in_percent FLOAT         NOT NULL,
    page_count                   BIGINT        NOT NULL
);
SELECT create_hypertable('sqlserver_index_fragmentation', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_index_fragmentation', INTERVAL '14 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_if_server_ts
    ON sqlserver_index_fragmentation (server_id, capture_timestamp DESC);
```

Unified query (with `'SAMPLED'` mode per DBA review recommendation):
```sql
SELECT
    DB_NAME()                              AS database_name,
    SCHEMA_NAME(o.schema_id)              AS schema_name,
    OBJECT_NAME(ps.object_id)             AS table_name,
    COALESCE(i.name,'')                   AS index_name,
    ps.index_id,
    i.type                                AS index_type,
    ps.avg_fragmentation_in_percent,
    ps.page_count
FROM sys.dm_db_index_physical_stats(DB_ID(), NULL, NULL, NULL, 'SAMPLED') ps
JOIN sys.indexes i ON ps.object_id = i.object_id AND ps.index_id = i.index_id
JOIN sys.objects o ON o.object_id = ps.object_id
WHERE ps.avg_fragmentation_in_percent >= 5.0   -- capture more, threshold in Go
  AND ps.page_count >= 100
  AND i.is_disabled = 0
  AND i.is_hypothetical = 0
  AND i.type NOT IN (5, 6)                      -- exclude Columnstore
  AND OBJECTPROPERTY(ps.object_id, 'IsUserTable') = 1;
```

**Both files** (`sqlserver_index.go` and `sqlserver_performance_debt.go`) read from `sqlserver_index_fragmentation` (latest snapshot per database).

---

### DMV-10: `sys.dm_db_missing_index_group_stats` + `sys.dm_db_missing_index_groups` + `sys.dm_db_missing_index_details`

**Impact: Currently 1 file (no duplication), but storing results enables the performance debt worker to read from TimescaleDB rather than hitting the DMV live.**

#### Target TimescaleDB Table: `sqlserver_missing_indexes`

```sql
CREATE TABLE IF NOT EXISTS sqlserver_missing_indexes (
    capture_timestamp    TIMESTAMPTZ   NOT NULL,
    server_id            UUID          NOT NULL REFERENCES monitored_servers(id),
    database_name        VARCHAR(128)  NOT NULL,
    improvement_score    FLOAT         NOT NULL,
    avg_user_impact      FLOAT,
    user_seeks           BIGINT,
    user_scans           BIGINT,
    table_name           TEXT          NOT NULL,
    equality_columns     TEXT,
    inequality_columns   TEXT,
    included_columns     TEXT,
    create_statement     TEXT,
    server_start_time    TIMESTAMPTZ
);
SELECT create_hypertable('sqlserver_missing_indexes', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_missing_indexes', INTERVAL '14 days', if_not_exists => TRUE);
```

Performance debt worker reads from `sqlserver_missing_indexes` (latest snapshot) instead of querying the DMV each time.

---

### DMV-11: `sys.dm_os_memory_clerks`

**Impact: 2 Go files. Neither stores results. Call it once, store results.**

| File | Columns |
|---|---|
| `sqlserver_memory.go:CollectMemoryClerks` | type, memory_node_id, pages_kb, virtual_memory_reserved_kb, virtual_memory_committed_kb, awe_allocated_kb |
| `sqlserver_memory.go` (COUNT DISTINCT) | memory_clerk_address |

#### Target TimescaleDB Table: `sqlserver_memory_clerks`

```sql
CREATE TABLE IF NOT EXISTS sqlserver_memory_clerks (
    capture_timestamp           TIMESTAMPTZ   NOT NULL,
    server_id                   UUID          NOT NULL REFERENCES monitored_servers(id),
    clerk_type                  VARCHAR(60)   NOT NULL,
    memory_node_id              SMALLINT,
    pages_mb                    FLOAT,
    virtual_memory_reserved_mb  FLOAT,
    virtual_memory_committed_mb FLOAT,
    awe_allocated_mb            FLOAT
);
SELECT create_hypertable('sqlserver_memory_clerks', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_memory_clerks', INTERVAL '90 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_mc_server_ts
    ON sqlserver_memory_clerks (server_id, capture_timestamp DESC);
```

Single collector in `sqlserver_memory_analyzer.go`. Both callers read from `sqlserver_memory_clerks`.

---

## Summary Table: DMV Consolidation Impact

| DMV | Files Before | SQL Queries / Cycle | Files After | Queries / Cycle | Reduction |
|---|---|---|---|---|---|
| `sys.dm_os_performance_counters` | 14 | 15 separate queries | 1 collector | 1 query (all counters) | **−93%** |
| `sys.dm_io_virtual_file_stats` | 6 | 5 separate queries | 1 collector | 1 query | **−80%** |
| `sys.dm_os_sys_memory` | 6 | 6 separate queries | 1 collector | 1 query | **−83%** |
| `sys.dm_os_sys_info` | 7 | 5 separate queries | 1 collector | 1 query (extended) | **−80%** |
| `sys.dm_exec_sessions` + `dm_exec_requests` | 11 + 9 = 20 | 20 queries | 1 collector | 1 combined query | **−95%** |
| `sys.dm_os_wait_stats` | 4 | 4 queries | 1 collector (existing) | 1 query | **−75%** |
| `sys.dm_db_index_usage_stats` | 3 | 3 queries/DB | 1 collector (existing) | 1 query/DB | **−67%** |
| `sys.dm_exec_query_memory_grants` | 8 | 8 queries | 1 collector | 1 query | **−88%** |
| `sys.dm_db_index_physical_stats` | 2 | 2 queries/DB | 1 collector | 1 query/DB | **−50%** |
| `sys.dm_db_missing_index_*` | 1 | 1 query/DB | 1 collector | 1 query/DB | 0% (now stored) |
| `sys.dm_os_memory_clerks` | 2 | 2 queries | 1 collector | 1 query | **−50%** |
| **Total** | **73 files** | **~70 queries/cycle** | **11 collectors** | **~11 queries/cycle** | **~84%** |

---

## New Collector File Inventory

| New/Updated File | DMV(s) Consolidated | Interval | Target Table |
|---|---|---|---|
| `sqlserver_perf_counters_collector.go` (new) | `sys.dm_os_performance_counters` (all 15 counters) | 30s | `sqlserver_perf_counters` |
| `sqlserver_io_collector.go` (new, replaces sqlserver_io.go) | `sys.dm_io_virtual_file_stats` + `sys.master_files` | 30s | `staging.sqlserver_io_raw`, `sqlserver_file_catalog` |
| `sqlserver_memory_analyzer.go` (extend) | `sys.dm_os_sys_memory` + `sys.dm_os_process_memory` + `sys.dm_exec_query_memory_grants` + `sys.dm_os_memory_clerks` | 60s | `sqlserver_memory_metrics` (extended), `sqlserver_memory_grants`, `sqlserver_memory_clerks` |
| `sqlserver_cpu.go` (extend) | `sys.dm_os_sys_info` (extend existing query) | 60s | `sqlserver_server_properties` (extended) |
| `sqlserver_session_collector.go` (new) | `sys.dm_exec_sessions` + `sys.dm_exec_requests` + `sys.dm_os_waiting_tasks` | 30s | `sqlserver_active_sessions` |
| `sqlserver_wait_stats_collector.go` (already exists) | `sys.dm_os_wait_stats` | 60s | `sqlserver_wait_stats_delta` ✅ |
| `index_usage_collector.go` (already exists) | `sys.dm_db_index_usage_stats` | 6h | `monitor.index_usage_stats` ✅ |
| `sqlserver_index_fragmentation_collector.go` (new) | `sys.dm_db_index_physical_stats` | 6h | `sqlserver_index_fragmentation` |
| `sqlserver_missing_index_collector.go` (new) | `sys.dm_db_missing_index_*` | 6h | `sqlserver_missing_indexes` |

---

## New TimescaleDB Tables Required

| Table | Purpose | Retention | Replaces |
|---|---|---|---|
| `sqlserver_perf_counters` | All 15 perf counters, one row per counter per cycle | 90d | 15 separate queries across 14 files |
| `sqlserver_file_catalog` | File metadata (paths, sizes, types) — rarely changes | 30d | Repeated `sys.master_files` joins |
| `sqlserver_active_sessions` | Session + request combined snapshot | 30d | 20 files hitting `dm_exec_sessions` + `dm_exec_requests` |
| `sqlserver_memory_grants` | Query memory grants snapshot | 30d | 8 files hitting `dm_exec_query_memory_grants` |
| `sqlserver_memory_clerks` | Memory clerk breakdown | 90d | 2 files with no storage |
| `sqlserver_index_fragmentation` | Per-database fragmentation snapshot | 14d | 2 files with no storage |
| `sqlserver_missing_indexes` | Missing index recommendations | 14d | Performance debt worker live query |

Existing tables to **extend** (add columns, no new table):
- `sqlserver_memory_metrics` — add `os_system_memory_state`, 5 process memory columns
- `sqlserver_server_properties` — add `sqlserver_start_time`, `ms_ticks`, `scheduler_count`
- `staging.sqlserver_io_raw` — fix INSERT to write all 15 defined columns (schema is already correct)

---

## Implementation Order

### Phase 1 — Quick Wins (no new tables, just bug fixes and redirections)

| Task | Files | Impact |
|---|---|---|
| Fix `staging.sqlserver_io_raw` INSERT to write all 15 columns | `ts_logger_pulses.go:108` | Unlocks IO latency data already being collected |
| Redirect `sqlserver_phase2.go:FetchWaitStatsCumulative` to read from `sqlserver_wait_stats_delta` | `sqlserver_phase2.go` | Removes 1 live DMV call per cycle |
| Redirect `live/sqlserver_repository.go:GetWaitStats` to read from `sqlserver_wait_stats_delta` | `live/sqlserver_repository.go` | Removes 1 live DMV call per cycle |
| Redirect `sqlserver_index.go:CollectSQLServerIndexUsageMetrics` to read from `monitor.index_usage_stats` | `sqlserver_index.go` | Removes 1 DMV call per DB per cycle |
| Redirect `sqlserver_performance_debt.go:FetchUnusedIndexes` to read from `monitor.index_usage_stats` | `sqlserver_performance_debt.go` | Removes 1 DMV call per DB per 6h |

### Phase 2 — sys.dm_os_performance_counters Unification (highest ROI)

| Task | Files |
|---|---|
| Create `sqlserver_perf_counters` table | `01_timescale_schema.sql` |
| Create `sqlserver_perf_counters_collector.go` | New file |
| Remove per-counter queries from `sqlserver_memory_analyzer.go`, `sqlserver_memory.go`, `sqlserver_cpu.go`, `unified_pulses.go`, `sqlserver_database_throughput.go`, `live/sqlserver_repository.go`, `sqlserver_health_worker.go`, `sqlserver_phase2.go` | 8 files |

### Phase 3 — I/O and Memory Unification

| Task | Files |
|---|---|
| Extend `sqlserver_memory_metrics` (5 new columns) + extend `FetchMemoryAnalyzerSnapshot` | `sqlserver_memory_analyzer.go`, schema |
| Extend `sqlserver_server_properties` (3 new columns) + extend `CollectServerProperties` | `sqlserver_cpu.go`, schema |
| Create `sqlserver_file_catalog` table + `sqlserver_io_collector.go` | New file, schema |
| Redirect all io/storage files away from direct DMV queries | 5 files |
| Remove redundant sys.dm_os_sys_memory queries from 5 files | 5 files |
| Remove redundant sys.dm_os_sys_info queries from 5 files | 5 files |

### Phase 4 — Session, Memory Grants, Memory Clerks

| Task | Files |
|---|---|
| Create `sqlserver_active_sessions` table | Schema |
| Create `sqlserver_session_collector.go` (sessions + requests + waiting tasks combined query) | New file |
| Create `sqlserver_memory_grants` table + extend `sqlserver_memory_analyzer.go` collector | Schema, `sqlserver_memory_analyzer.go` |
| Create `sqlserver_memory_clerks` table + add clerk collection to memory analyzer | Schema, `sqlserver_memory_analyzer.go` |
| Redirect 20 session/request files to read from `sqlserver_active_sessions` | 20 files (phased) |
| Redirect 8 memory grant files to read from `sqlserver_memory_grants` | 8 files |

### Phase 5 — Index Fragmentation and Missing Indexes

| Task | Files |
|---|---|
| Create `sqlserver_index_fragmentation` table + `sqlserver_index_fragmentation_collector.go` | New file, schema |
| Create `sqlserver_missing_indexes` table + `sqlserver_missing_index_collector.go` | New file, schema |
| Redirect `sqlserver_index.go` and `sqlserver_performance_debt.go` to read from new tables | 2 files |

---

## Architecture After Consolidation

```
Monitored SQL Server
        │
        │  ~11 queries per cycle (was ~70)
        ▼
┌───────────────────────────────────────────────────────┐
│              Unified Collector Layer                  │
│  (one file per DMV, runs once per cycle)              │
│                                                       │
│  PerfCountersCollector   → sqlserver_perf_counters    │
│  IOCollector             → staging.sqlserver_io_raw   │
│                            sqlserver_file_catalog     │
│  MemoryAnalyzerCollector → sqlserver_memory_metrics   │
│                            sqlserver_memory_grants    │
│                            sqlserver_memory_clerks    │
│  CPUCollector (extend)   → sqlserver_server_properties│
│  SessionCollector        → sqlserver_active_sessions  │
│  WaitStatsCollector ✅   → sqlserver_wait_stats_delta │
│  IndexUsageCollector ✅  → monitor.index_usage_stats  │
│  FragmentationCollector  → sqlserver_index_frag...    │
│  MissingIndexCollector   → sqlserver_missing_indexes  │
└───────────────────────────────────────────────────────┘
        │
        ▼
   TimescaleDB
        │
        ▼
┌───────────────────────────────────────────────────────┐
│         All Go Consumers (API, Service, Worker)       │
│         read from TimescaleDB — never from DMV        │
└───────────────────────────────────────────────────────┘
```

---

## Verification Checklist

- [ ] `go build ./...` passes after each phase
- [ ] After Phase 1: `SELECT * FROM staging.sqlserver_io_raw LIMIT 1` shows all 15 columns populated
- [ ] After Phase 2: Single row in `sqlserver_perf_counters` per counter per cycle; no more per-counter individual queries in logs
- [ ] After Phase 3: `sqlserver_memory_metrics` has `os_system_memory_state` populated; `sqlserver_server_properties` has `sqlserver_start_time`
- [ ] After Phase 4: `sqlserver_active_sessions` rows populated every 30s; blocking detection still works via TimescaleDB query
- [ ] All existing API responses return identical data (no regression in dashboard values)
- [ ] Monitored server query count per minute drops by ~80% (verify via SQL Server Profiler or Extended Events)
