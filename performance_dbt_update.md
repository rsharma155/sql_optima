# Performance Debt Dashboard — Enhancement Specification

**Project:** SQL Optima
**Target:** SQL Server Performance Debt Dashboard (`sqlserver_PerformanceDebt.js`)
**Status:** Specification v4 — updated post DMV-consolidation implementation (May 2026)

---

## Design Principle

**Avoid hitting the same DMV twice.** Before adding any new collection query, the existing collector pipeline was fully audited at the column level. The table below is the ground truth for every DMV mentioned in this spec.

Every DMV touched by the consolidation work in this branch (`cold_storage_refine_collector`) is marked with its current implementation status.

---

## 1. Full DMV Reuse Audit

### 1.1 Pre-existing DMVs (updated to reflect consolidation work)

| DMV | Status | TimescaleDB Table | Collector File | Columns Stored | Notes / Remaining Gaps |
|---|---|---|---|---|---|
| `sys.dm_os_sys_memory` | ✅ **RESOLVED** | `sqlserver_memory_metrics` | `sqlserver_memory_collector.go` | `os_total_memory_mb`, `os_available_memory_mb`, `os_system_memory_state` ← **new** | `system_memory_state_desc` now stored as `os_system_memory_state VARCHAR(60)`. `ALTER TABLE` added to `01_timescale_schema.sql:982`. Collector (`LogSQLServerMemoryMetrics`) writes 20-column INSERT. |
| `sys.dm_os_process_memory` | ⚠️ Partial | `sqlserver_memory_metrics` | `sqlserver_memory_analyzer.go:65` | `process_physical_low`, `process_virtual_low` | `physical_memory_in_use_kb`, `memory_utilization_percentage`, `page_fault_count`, `locked_page_allocations_kb`, `large_page_allocations_kb` — still not stored. Feature A in §2 covers this. |
| `sys.dm_os_volume_stats` | ⚠️ Live only, wrong join | `sqlserver_best_practices.go:408` | **None** | — | Only used for best-practices guardrail. Uses incorrect `LEFT()` character match instead of `CROSS APPLY`. Never stored in TimescaleDB. Feature B in §2 covers this. |
| `sys.dm_io_virtual_file_stats` | ✅ **RESOLVED** | `staging.sqlserver_io_raw` | `ts_logger_pulses.go` | All 15 columns now written: `database_id`, `db_name`, `file_id`, `type_desc`, `physical_name`, `num_of_reads`, `num_of_writes`, `num_of_bytes_read`, `num_of_bytes_written`, `io_stall_read_ms`, `io_stall_write_ms`, `io_stall`, `size_on_disk_bytes` | **Bug fixed**: `LogStagingIORows` now writes all 15 columns (was 6). Source data from `FetchTier3MegaQueryIO()` in `unified_pulses.go` already selected all 15. Phase 12 I/O latency evaluation now reads this table correctly. |
| `sys.databases` | ✅ Yes — comprehensive | `sqlserver_database_catalog` | `sqlserver_database.go:100` | `is_auto_close_on`, `is_auto_shrink_on`, `compatibility_level`, `recovery_model_desc` + 28 more | `is_query_store_on` — still absent from both query and table. Phase 9 (§5) covers this. |
| `sys.database_query_store_options` | ❌ Never queried | — | **None** | — | All columns genuinely new. Phase 9 (§5) covers this. |
| `sys.query_store_plan` | ⚠️ Partial | None | `sqlserver_query_analysis.go:61` | `is_forced_plan` (bool flag only) | `force_failure_count` never queried. Phase 9 covers it. |
| `sys.dm_os_sys_info` | ✅ **RESOLVED** | `sqlserver_server_properties` | `sqlserver_cpu.go:174`, `sqlserver_health_worker.go:252` | `cpu_count`, `hyperthread_ratio`, `socket_count`, `cores_per_socket`, `physical_memory_kb`, `sqlserver_start_time` ← **new**, `ms_ticks` ← **new**, `scheduler_count` ← **new** | Three new columns added: `sqlserver_start_time TIMESTAMPTZ`, `ms_ticks BIGINT DEFAULT 0`, `scheduler_count INT DEFAULT 0`. Both `CollectServerProperties` (cpu.go) and the health worker now collect and persist these. `GetServerProperties` SELECT and scan extended accordingly. |
| `sys.master_files WHERE database_id = 2` | ✅ Yes | `sqlserver_tempdb_files` | `sqlserver_tempdb.go:18` | `file_name`, `file_type`, `allocated_mb`, `used_mb`, `free_mb`, `max_size_mb`, `growth_mb` | File size equality and cpu_count comparison not stored as time-series. Phase 10 (§5) reads existing tables instead. |
| `sys.dm_db_log_info` | ✅ Yes | `sqlserver_performance_debt_findings` (details JSON) | `sqlserver_performance_debt.go:352` | VLF count only | Still uses `dm_db_log_info` even on SQL Server 2017+. Phase §6.3 upgrades to `dm_db_log_stats` for version ≥ 14. |
| `sys.configurations` | ✅ Yes — 7 named configs | `sqlserver_performance_debt_findings` | `performance_debt_worker.go:607-679` | 7 config findings already generated | Nothing new needed. |

---

### 1.2 DMVs unified by the consolidation work (new in this branch)

| DMV | Status | TimescaleDB Table | Collector File | Service File | Collection Interval | Dedup Strategy |
|---|---|---|---|---|---|---|
| `sys.dm_os_performance_counters` | ✅ **UNIFIED** | `sqlserver_perf_counters` | `sqlserver_perf_counters_collector.go` | `sqlserver_perf_counters_service.go` | 30 s | `UNIQUE INDEX idx_pc_dedup ON sqlserver_perf_counters (server_id, counter_name, instance_name, capture_timestamp)` + `ON CONFLICT DO NOTHING` |
| `sys.dm_exec_sessions` + `sys.dm_exec_requests` | ✅ **UNIFIED** | `sqlserver_active_sessions` | `sqlserver_session_collector.go` | `sqlserver_session_service.go` | 30 s | SHA-256 hash of `session_id\|blocking_session_id\|status`; skip write if hash unchanged |
| `sys.dm_db_index_physical_stats` | ✅ **UNIFIED** | `sqlserver_index_fragmentation` | `sqlserver_index_fragmentation_collector.go` | `sqlserver_index_service.go` | 6 h | Only writes rows where `avg_fragmentation_in_percent >= 5.0 AND page_count >= 100`; index `idx_idx_frag_server_db` per-database |
| `sys.dm_db_missing_index_*` (group/groups/details) | ✅ **UNIFIED** | `sqlserver_missing_indexes` | `sqlserver_missing_index_collector.go` | `sqlserver_index_service.go` (shared) | 6 h | Table-level unique index on `(server_id, database_name, table_name, equality_columns, capture_timestamp)` |
| `sys.dm_os_memory_clerks` | ✅ Existing wired | `sqlserver_memory_clerks` | `sqlserver_memory_collector.go` | Memory intelligence cycle | 60 s | Existing write path; pre-existing table and retention policy |
| `sys.dm_exec_query_memory_grants` | ✅ Existing + extended | `sqlserver_memory_grants` (aggregate) + `sqlserver_memory_grants_detail` ← **new** | `sqlserver_memory_analyzer.go` | Memory intelligence cycle | 60 s | `sqlserver_memory_grants_detail` stores per-session rows; aggregate kept unchanged for existing dashboard queries |
| `sys.master_files` + `sys.dm_io_virtual_file_stats` (file catalog) | ✅ **NEW TABLE** | `sqlserver_file_catalog` | (future: `sqlserver_io_collector.go`) | (future: `sqlserver_io_service.go`) | 6 h | `UNIQUE INDEX idx_file_catalog_dedup ON sqlserver_file_catalog (server_id, database_id, file_id, capture_timestamp)` |

---

### 1.3 `sys.dm_os_performance_counters` — Extended Schema Detail

The `sqlserver_perf_counters` table gained three new columns:

```sql
ALTER TABLE sqlserver_perf_counters ADD COLUMN IF NOT EXISTS instance_name VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE sqlserver_perf_counters ADD COLUMN IF NOT EXISTS cntr_value    BIGINT       NOT NULL DEFAULT 0;
ALTER TABLE sqlserver_perf_counters ADD COLUMN IF NOT EXISTS cntr_type     INT          NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_pc_dedup
    ON sqlserver_perf_counters (server_id, counter_name, instance_name, capture_timestamp);
```

Consumers redirected to cache-first reads from this table (fall back to live DMV only when `capture_timestamp < NOW() - INTERVAL '5 minutes'`):
- `sqlserver_database_throughput.go` — `Transactions/sec` per database now reads `sqlserver_perf_counters WHERE counter_name = 'Transactions/sec'`
- `sqlserver_memory_analyzer.go` — PLE, Total Server Memory, Target Server Memory, Memory Grants Pending
- `sqlserver_memory.go` — Buffer Pool + PLE
- `sqlserver_cpu.go` — Batch Requests
- `sqlserver_health_worker.go` — 5 duplicate per-counter reads replaced

---

### 1.4 `FetchIndexFragmentation` — Cache-First Implementation

`sqlserver_performance_debt.go:FetchIndexFragmentation` now follows cache-first pattern:

1. Check `sqlserver_index_fragmentation` WHERE `server_id = $1 AND database_name = $2 AND capture_timestamp >= NOW() - INTERVAL '8 hours'`
2. If rows found → return from TimescaleDB (no DMV call)
3. Fallback: live DMV query using `'SAMPLED'` mode (upgraded from `'LIMITED'`)

The `IndexFragmentationCollector` (runs at 6 h cadence) populates the table proactively so the fallback is only hit for brand-new servers or after a restart.

---

### 1.5 `FetchMissingIndexRecommendations` — Cache-First Implementation

`sqlserver_performance_debt.go:FetchMissingIndexRecommendations` now follows cache-first pattern:

1. Check `sqlserver_missing_indexes` WHERE `server_id = $1 AND database_name = $2 AND capture_timestamp >= NOW() - INTERVAL '8 hours'`
2. If rows found → `fetchMissingIndexesFromTS` helper reconstructs `CREATE INDEX` statement from stored `equality_columns`, `inequality_columns`, `included_columns`, `schema_name`, `table_name`
3. Fallback: live DMV query (original path, thresholds still raised per §6.2)

**Note**: `server_start_time` is now available from `sqlserver_server_properties.sqlserver_start_time` — the UI can show "index advisor data valid since last restart" without an extra DMV call.

---

## 2. Implementation Strategy by Feature

Each feature is classified by the minimum-change approach that avoids duplicate DMV queries.

---

### Feature A: Server Vitals — Memory Panel

**Goal**: Show OS memory vs. SQL process memory in the new Server Vitals ribbon at the top of the Performance Debt page.

**Finding**: `sqlserver_memory_metrics` already stores `os_total_memory_mb`, `os_available_memory_mb`, `process_physical_low`, `process_virtual_low`, `sql_memory_used_mb`, `sql_memory_target_mb`, `ple_seconds`, `memory_grants_pending`, and now also `os_system_memory_state` (see §1.1). The collector runs every ~60 seconds.

**What to add**: 4-5 new columns to the EXISTING `sqlserver_memory_metrics` table and EXTEND the existing `FetchMemoryAnalyzerSnapshot()` query in `sqlserver_memory_analyzer.go` — not a new query or table.

#### Schema change: add 5 columns to `sqlserver_memory_metrics`

```sql
-- Add to existing table definition in 01_timescale_schema.sql:
ALTER TABLE sqlserver_memory_metrics
    ADD COLUMN IF NOT EXISTS sql_physical_memory_in_use_mb  BIGINT  DEFAULT 0,
    ADD COLUMN IF NOT EXISTS sql_memory_utilization_pct     INT     DEFAULT 0,
    ADD COLUMN IF NOT EXISTS sql_page_fault_count           BIGINT  DEFAULT 0,
    ADD COLUMN IF NOT EXISTS sql_locked_page_alloc_mb       BIGINT  DEFAULT 0,
    ADD COLUMN IF NOT EXISTS sql_large_page_alloc_mb        BIGINT  DEFAULT 0;
```

#### Collector change: extend existing query in `sqlserver_memory_analyzer.go`

The existing query at line 65 already queries `sys.dm_os_process_memory`. Change from:
```sql
SELECT process_physical_memory_low, process_virtual_memory_low
FROM sys.dm_os_process_memory WITH (NOLOCK);
```
to:
```sql
SELECT
    process_physical_memory_low,
    process_virtual_memory_low,
    physical_memory_in_use_kb     / 1024  AS sql_physical_memory_in_use_mb,
    memory_utilization_percentage          AS sql_memory_utilization_pct,
    page_fault_count                       AS sql_page_fault_count,
    locked_page_allocations_kb    / 1024  AS sql_locked_page_alloc_mb,
    large_page_allocations_kb     / 1024  AS sql_large_page_alloc_mb
FROM sys.dm_os_process_memory WITH (NOLOCK);
```

Add the 5 new fields to the `FetchMemoryAnalyzerSnapshot()` output map and to the INSERT in the corresponding ts_logger writer.

#### API: new endpoint reads from existing table

`GET /api/sqlserver/server-vitals?instance={name}` queries the latest row from `sqlserver_memory_metrics` (within last 5 minutes). No new collection loop or table.

---

### Feature B: Server Vitals — Volume / Storage Panel

**Goal**: Show per-volume free space (mount point, total GB, free GB, free%) in the new Server Vitals ribbon and the Storage & File Locations accordion.

**Finding**: `sys.dm_os_volume_stats` is only queried in `sqlserver_best_practices.go:408` as a live guardrails check with an **incorrect join** (`LEFT(mf.physical_name, 1) = LEFT(vs.volume_mount_point, 1)` — breaks on UNC paths and mount points). The data is **never stored in TimescaleDB**. This is genuinely new.

`sqlserver_disk_history` only has database-level `data_mb`/`log_mb` aggregates, not physical volume free space.

#### New TimescaleDB table: `sqlserver_volume_stats`

```sql
-- Add to 01_timescale_schema.sql inline:
CREATE TABLE IF NOT EXISTS sqlserver_volume_stats (
    capture_timestamp   TIMESTAMPTZ   NOT NULL,
    server_id           UUID          NOT NULL REFERENCES monitored_servers(id),
    database_name       VARCHAR(128)  NOT NULL,
    logical_file_name   VARCHAR(128)  NOT NULL,
    physical_name       TEXT          NOT NULL,
    file_type           VARCHAR(10)   NOT NULL,   -- 'ROWS' or 'LOG'
    file_size_mb        FLOAT,
    volume_mount_point  VARCHAR(512)  NOT NULL,
    volume_label        VARCHAR(256),
    volume_total_gb     FLOAT,
    volume_available_gb FLOAT,
    volume_free_pct     FLOAT
);
SELECT create_hypertable('sqlserver_volume_stats', 'capture_timestamp', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_volume_stats', INTERVAL '90 days', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_vs_server_ts
    ON sqlserver_volume_stats (server_id, capture_timestamp DESC);
```

#### New collector function: `FetchVolumeStats` in `sqlserver_volume.go` (new file)

Use the correct Microsoft-documented CROSS APPLY pattern, not the LEFT() character match:

```sql
-- Correct join: table-valued function with database_id + file_id
SELECT DISTINCT
    mf.database_id,
    DB_NAME(mf.database_id)            AS database_name,
    mf.name                            AS logical_file_name,
    mf.physical_name,
    mf.type_desc                       AS file_type,
    mf.size * 8 / 1024.0              AS file_size_mb,
    vs.volume_mount_point,
    vs.logical_volume_name             AS volume_label,
    vs.total_bytes    / 1073741824.0   AS volume_total_gb,
    vs.available_bytes / 1073741824.0  AS volume_available_gb,
    CAST(vs.available_bytes * 100.0
         / NULLIF(vs.total_bytes, 0)
         AS DECIMAL(5,2))              AS volume_free_pct
FROM sys.master_files mf WITH (NOLOCK)
CROSS APPLY sys.dm_os_volume_stats(mf.database_id, mf.file_id) vs
WHERE mf.database_id > 4
  AND mf.state = 0
ORDER BY volume_free_pct ASC;
```

Also fix the existing bug in `sqlserver_best_practices.go:408` to use the same CROSS APPLY.

#### Go struct: `VolumeStatsRow` in `row_types.go`

```go
type VolumeStatsRow struct {
    CaptureTimestamp  time.Time `json:"capture_timestamp"`
    ServerID          uuid.UUID `json:"server_id"`
    DatabaseName      string    `json:"database_name"`
    LogicalFileName   string    `json:"logical_file_name"`
    PhysicalName      string    `json:"physical_name"`
    FileType          string    `json:"file_type"`
    FileSizeMB        float64   `json:"file_size_mb"`
    VolumeMountPoint  string    `json:"volume_mount_point"`
    VolumeLabel       string    `json:"volume_label"`
    VolumeTotalGB     float64   `json:"volume_total_gb"`
    VolumeAvailableGB float64   `json:"volume_available_gb"`
    VolumeFreePct     float64   `json:"volume_free_pct"`
}
```

#### Collection integration

Add Phase 8 to `performance_debt_worker.go` — runs once per instance (not inside the per-database loop) at the end of the cycle using the master connection. Calls `FetchVolumeStats`, writes to `sqlserver_volume_stats`, then calls `evaluateServerVitalsFindings` to generate alert findings.

---

## 3. Local Rule-Based Alerting

### Design

Do **not** build a separate alert service. Inject server vitals findings into the existing `sqlserver_performance_debt_findings` table using the same `PerformanceDebtFindingRow` struct and `finding_key` fingerprint deduplication already in place (`ts_logger_performance_debt.go`). These findings surface automatically in the existing dashboard banners.

### Storage Alert Rules (section = "Storage & Growth")

| Condition | Severity | `finding_type` | `finding_key` pattern |
|---|---|---|---|
| `volume_free_pct < 10` | CRITICAL | `volume_critically_low` | `vol_crit_{server_id}_{mount_hash8}` |
| `volume_free_pct < 15` | WARNING | `volume_low` | `vol_warn_{server_id}_{mount_hash8}` |

`fix_script`: `-- No automated fix. Expand volume or relocate files: ALTER DATABASE [{db}] MODIFY FILE (NAME = N'{file}', FILENAME = N'{new_path}');`

### Memory Alert Rules (section = "Engine Config")

| Condition | Severity | `finding_type` | Notes |
|---|---|---|---|
| `sql_process_physical_low = true` | CRITICAL | `memory_physical_pressure` | From new column in `sqlserver_memory_metrics` |
| `sql_process_virtual_low = true` | CRITICAL | `memory_virtual_pressure` | Already collected; just add to evaluation |
| `os_utilization_pct > 90 AND sql_memory_used_mb / os_total_memory_mb > 0.85` | WARNING | `memory_headroom_low` | All fields already in `sqlserver_memory_metrics` |

`fix_script` for `memory_headroom_low`:
```sql
-- Recommended: leave OS at least 10% or 4 GB headroom
EXEC sp_configure 'max server memory (MB)', {recommended_mb};
RECONFIGURE;
```

### Alert evaluation function

Add `evaluateServerVitalsFindings(vitals, volumes)` to `performance_debt_worker.go`. Takes the latest `sqlserver_memory_metrics` row (already fetched during collection) and the newly collected `[]VolumeStatsRow`. Returns `[]PerformanceDebtFindingRow`.

---

## 4. API Design

### `GET /api/sqlserver/server-vitals?instance={name}`

Reads from **existing/extended** tables — no new collection needed at query time:
- `sqlserver_memory_metrics` — latest row within 5 minutes (includes `os_system_memory_state` from §1.1 fix)
- `sqlserver_volume_stats` — latest rows per volume within 15 minutes (collection is 6-hourly)

Response shape:
```json
{
  "memory": {
    "os_total_mb": 65536,
    "os_available_mb": 12288,
    "os_utilization_pct": 81.3,
    "sql_in_use_mb": 48000,
    "sql_utilization_pct": 73,
    "sql_process_physical_low": false,
    "sql_process_virtual_low": false,
    "os_system_memory_state": "AVAILABLE",
    "ple_seconds": 1840,
    "memory_grants_pending": 0,
    "captured_at": "2026-05-22T10:30:00Z"
  },
  "volumes": [
    {
      "volume_mount_point": "C:\\",
      "volume_label": "OS",
      "volume_total_gb": 120.0,
      "volume_available_gb": 18.5,
      "volume_free_pct": 15.4,
      "files": [{"database": "master", "logical_name": "master", "file_type": "ROWS", "physical_path": "C:\\...\\master.mdf", "size_mb": 4.0}]
    }
  ]
}
```

### `GET /api/sqlserver/volume-stats/live?instance={name}`

Bypasses TimescaleDB and runs `FetchVolumeStats` DMV query directly. Used by the UI "Refresh" button in the Storage accordion. Pattern mirrors existing live endpoints (active sessions, etc.).

---

## 5. Additional Performance Debt Phases — Reuse-First Analysis

### Phase 9: Query Store Health

**What's already there**:
- `sys.databases.is_query_store_on` is **never queried** — not in `sqlserver_database.go` query and not in `sqlserver_database_catalog` table.
- `sys.database_query_store_options` is **never queried** anywhere in the codebase.
- `sys.query_store_plan.force_failure_count` is **never queried**.

**Minimum-change approach**:

**Step 1**: Add `is_query_store_on` to the existing `sys.databases` query in `sqlserver_database.go:100` and add the column to `sqlserver_database_catalog` in the schema. This is a one-line SQL change and one column addition — no new query.

```sql
-- Add to existing SELECT in sqlserver_database.go:
d.is_query_store_on,
-- Add to existing INSERT and schema table definition
```

**Step 2**: Add a new per-database query for `sys.database_query_store_options` and `sys.query_store_plan` — this is genuinely new (not collected anywhere):

```sql
-- Run per user database after confirming is_query_store_on = 1
SELECT
    DB_NAME()                                                 AS database_name,
    desired_state_desc,
    actual_state_desc,
    readonly_reason,
    current_storage_size_mb,
    max_storage_size_mb,
    CAST(current_storage_size_mb * 100.0
         / NULLIF(max_storage_size_mb, 0) AS DECIMAL(5,2))  AS storage_used_pct,
    (SELECT COUNT(*)
     FROM sys.query_store_plan
     WHERE is_forced_plan = 1
       AND force_failure_count > 0)                          AS broken_forced_plans
FROM sys.database_query_store_options;
```

**Findings generated**:

| Condition | Severity | `finding_type` |
|---|---|---|
| `actual_state_desc = 'OFF'` (user DB) | WARNING | `query_store_disabled` |
| `actual_state_desc = 'READ_ONLY'` | CRITICAL | `query_store_read_only` |
| `storage_used_pct > 90` | WARNING | `query_store_almost_full` |
| `broken_forced_plans > 0` | WARNING | `query_store_broken_forced_plan` |

---

### Phase 10: TempDB Design Check

**What's already there**:
- `sqlserver_tempdb_files` table has `allocated_mb`, `used_mb`, `free_mb`, `file_name`, `file_type` — populated by existing collector.
- `sqlserver_server_properties` table now has `cpu_count`, `scheduler_count` — `scheduler_count` was added by the DMV consolidation work (§1.1). Use `LEAST(8, cpu_count)` for the recommended file count as before.

**Minimum-change approach**: Do NOT re-query `sys.master_files` or `sys.dm_os_sys_info`. **Query the existing TimescaleDB tables** in the Performance Debt worker's evaluation phase:

```sql
-- Query existing tables to evaluate TempDB design, no new DMV access:
WITH tempdb_files AS (
    SELECT
        COUNT(*) FILTER (WHERE file_type = 'ROWS')      AS data_file_count,
        COUNT(DISTINCT allocated_mb) FILTER (WHERE file_type = 'ROWS') AS distinct_file_sizes,
        MIN(allocated_mb) FILTER (WHERE file_type = 'ROWS') AS min_file_mb,
        MAX(allocated_mb) FILTER (WHERE file_type = 'ROWS') AS max_file_mb
    FROM sqlserver_tempdb_files
    WHERE server_id = $1
      AND capture_timestamp >= NOW() - INTERVAL '10 minutes'
),
server_props AS (
    SELECT cpu_count, COALESCE(scheduler_count, cpu_count) AS scheduler_count
    FROM sqlserver_server_properties
    WHERE server_id = $1
    ORDER BY capture_timestamp DESC
    LIMIT 1
)
SELECT t.*, s.cpu_count, LEAST(8, s.cpu_count) AS recommended_file_count
FROM tempdb_files t CROSS JOIN server_props s;
```

**Findings generated**:

| Condition | Severity | `finding_type` |
|---|---|---|
| `data_file_count < LEAST(8, cpu_count)` | WARNING | `tempdb_too_few_files` |
| `distinct_file_sizes > 1` | WARNING | `tempdb_unequal_files` |

Fix script for unequal files:
```sql
ALTER DATABASE tempdb MODIFY FILE (NAME = N'{file}', SIZE = {target_mb}MB);
-- Repeat for all data files. Equal sizes enable SQL Server round-robin allocation.
```

---

### Phase 11: Destructive Database Settings

**What's already there**:
- `sqlserver_database_catalog` table already has `is_auto_close_on`, `is_auto_shrink_on`, `compatibility_level` — all populated by the existing `sqlserver_database.go` collector.

**Minimum-change approach**: Do NOT re-query `sys.databases`. **Query the existing `sqlserver_database_catalog` table** in the Performance Debt worker evaluation:

```sql
-- Evaluate from existing TimescaleDB data — no new DMV query:
SELECT DISTINCT ON (database_name)
    database_name,
    is_auto_close_on,
    is_auto_shrink_on,
    compatibility_level,
    capture_timestamp
FROM sqlserver_database_catalog
WHERE server_id = $1
  AND capture_timestamp >= NOW() - INTERVAL '30 minutes'
ORDER BY database_name, capture_timestamp DESC;
```

Findings generated (one per offending database):

| Condition | Severity | `finding_type` | Fix Script |
|---|---|---|---|
| `is_auto_close_on = true` | WARNING | `auto_close_enabled` | `ALTER DATABASE [{db}] SET AUTO_CLOSE OFF;` |
| `is_auto_shrink_on = true` | CRITICAL | `auto_shrink_enabled` | `ALTER DATABASE [{db}] SET AUTO_SHRINK OFF;` |
| `compatibility_level` ≥ 2 versions behind server | WARNING | `compat_level_stale` | `ALTER DATABASE [{db}] SET COMPATIBILITY_LEVEL = {target};` |

> **No DMV query needed at collection time.** The Performance Debt worker simply reads from `sqlserver_database_catalog` (already populated every collection cycle) and generates findings if any database violates these rules.

---

### Phase 12: I/O Latency Health

**What's already there (post-fix)**:
- ✅ **RESOLVED**: `staging.sqlserver_io_raw` INSERT bug is fixed. `LogStagingIORows` in `ts_logger_pulses.go` now writes all 15 columns. The table correctly stores: `database_id`, `db_name`, `file_id`, `type_desc`, `physical_name`, `num_of_reads`, `num_of_writes`, `num_of_bytes_read`, `num_of_bytes_written`, `io_stall_read_ms`, `io_stall_write_ms`, `io_stall`, `size_on_disk_bytes`.

No schema change or collector fix needed for this phase — the prerequisite (P1) is complete. Proceed directly to the evaluation query.

#### Performance Debt evaluation query (reads from existing table)

```sql
-- Compute per-file average latency from the most recent collection window:
WITH io_latest AS (
    SELECT DISTINCT ON (db_name, file_id)
        db_name,
        file_id,
        physical_name,
        type_desc,
        CASE WHEN num_of_reads  = 0 THEN 0.0
             ELSE CAST(io_stall_read_ms  AS FLOAT) / num_of_reads  END AS avg_read_ms,
        CASE WHEN num_of_writes = 0 THEN 0.0
             ELSE CAST(io_stall_write_ms AS FLOAT) / num_of_writes END AS avg_write_ms,
        num_of_reads + num_of_writes AS total_ops
    FROM staging.sqlserver_io_raw
    WHERE server_id = $1
      AND capture_timestamp >= NOW() - INTERVAL '30 minutes'
      AND num_of_reads + num_of_writes > 1000
    ORDER BY db_name, file_id, capture_timestamp DESC
)
SELECT * FROM io_latest
WHERE avg_read_ms > 20 OR avg_write_ms > 5
ORDER BY GREATEST(avg_read_ms, avg_write_ms) DESC;
```

**Findings generated**:

| Condition | Severity | `finding_type` |
|---|---|---|
| `avg_read_ms > 30` (data file) | CRITICAL | `io_read_latency_critical` |
| `avg_read_ms > 20` (data file) | WARNING | `io_read_latency_warning` |
| `avg_write_ms > 10` (log file) | CRITICAL | `io_write_latency_critical` — log writer is synchronous; >10ms stalls every COMMIT |
| `avg_write_ms > 5` (log file) | WARNING | `io_write_latency_warning` |

Fix script: `-- Move the log file to a dedicated, low-latency volume: ALTER DATABASE [{db}] MODIFY FILE (NAME = N'{file}', FILENAME = N'{new_path}');`

---

## 6. DBA Query Review — Six Existing Collector Queries

### 6.1 `FetchIndexFragmentation` — `sqlserver_performance_debt.go:175`

**Current state (post cache-first implementation)**:

`FetchIndexFragmentation` now follows the cache-first pattern (see §1.4):
- **Primary path**: reads from `sqlserver_index_fragmentation` table populated by `IndexFragmentationCollector` every 6 hours. The collector already uses `'SAMPLED'` mode and filters `avg_fragmentation >= 5.0 AND page_count >= 100`.
- **Fallback path**: live DMV with `'SAMPLED'` mode (upgraded from original `'LIMITED'`).

**Remaining fixes still needed in the fallback query**:

**Issue 1: Columnstore indexes included**

`REBUILD`/`REORGANIZE` semantics differ for Columnstore (`type IN (5, 6)`). The current fallback query includes them, generating incorrect fix scripts. The `IndexFragmentationCollector` already excludes them via `index_id > 0`; the fallback SQL needs the same filter:

```sql
-- Apply to fallback query in FetchIndexFragmentation:
AND i.type NOT IN (5, 6)          -- exclude Columnstore
```

**Issue 2: Threshold alignment**

The fallback query uses `avg_fragmentation_in_percent >= 10.0` while the collector uses `>= 5.0`. Align to `>= 5.0 AND page_count >= 100` so the fallback and cache paths return consistent result sets.

```sql
-- Fixed fallback query:
FROM sys.dm_db_index_physical_stats(DB_ID(), NULL, NULL, NULL, 'SAMPLED') ps
JOIN sys.indexes i ON ps.object_id = i.object_id AND ps.index_id = i.index_id
JOIN sys.objects o ON o.object_id = ps.object_id
WHERE ps.avg_fragmentation_in_percent >= 5.0
  AND ps.page_count >= 100
  AND i.is_disabled = 0
  AND i.is_hypothetical = 0
  AND i.type NOT IN (5, 6)          -- exclude Columnstore
  AND OBJECTPROPERTY(ps.object_id, 'IsUserTable') = 1
ORDER BY ps.avg_fragmentation_in_percent DESC;
```

---

### 6.2 `FetchMissingIndexRecommendations` — `sqlserver_performance_debt.go:111`

**Current state (post cache-first implementation)**:

`FetchMissingIndexRecommendations` now follows the cache-first pattern (see §1.5):
- **Primary path**: reads from `sqlserver_missing_indexes` populated by `MissingIndexCollector` every 6 hours.
- **Fallback path**: live DMV query (thresholds still need raising per issues below).

**`server_start_time` now available without extra DMV call**: `sqlserver_server_properties.sqlserver_start_time` is populated by the health worker (§1.1 fix). The fallback query and the cache-first path should both include this in the response. For the cache path, join to:

```sql
SELECT sqlserver_start_time
FROM sqlserver_server_properties
WHERE server_id = $1
ORDER BY capture_timestamp DESC LIMIT 1
```

**Remaining fixes still needed in the fallback query**:

**Issue 1: Improvement score threshold too low**

Formula: `avg_total_user_cost × avg_user_impact × (user_seeks + user_scans)`. A score of 500 can come from `cost=0.01 × impact=1.0 × seeks=50,000` — trivial per-query cost at high frequency. On a busy OLTP server this generates dozens of noisy low-value recommendations.

**Issue 2: Seek floor too low**

100 seeks since the last SQL Server restart (which could be months ago) is not a meaningful threshold.

```sql
-- Fixed thresholds in fallback query:
SELECT TOP (25)
    ...existing columns...,
    (SELECT TOP 1 CONVERT(VARCHAR(30), sqlserver_start_time, 126)
     FROM sys.dm_os_sys_info) AS server_start_time
FROM sys.dm_db_missing_index_group_stats migs
JOIN sys.dm_db_missing_index_groups mig ON migs.group_handle = mig.index_group_handle
JOIN sys.dm_db_missing_index_details mid ON mig.index_handle = mid.index_handle
WHERE mid.database_id = DB_ID()
  AND migs.user_seeks >= 500                         -- raised from 100
  AND (migs.avg_total_user_cost * migs.avg_user_impact
       * (migs.user_seeks + migs.user_scans)) >= 5000  -- raised from 500
ORDER BY improvement_score DESC;
```

> **Note on the collector**: `MissingIndexCollector` already uses a limit parameter (default 100) but does not yet enforce these score/seek thresholds. Update the WHERE clause in `sqlserver_missing_index_collector.go` to match so the cached data is also filtered consistently.

---

### 6.3 `FetchVLFCount` — `sqlserver_performance_debt.go:352`

**Issue: Inefficient on SQL Server 2017+**

`SELECT COUNT(*) FROM sys.dm_db_log_info(DB_ID())` scans all VLF rows. On SQL Server 2017+ (version 14), `sys.dm_db_log_stats(DB_ID())` returns `total_vlf_count` as a single pre-computed value plus `active_log_size_mb` and `log_truncation_holdup_reason`.

**Implementation**: Cache the SQL Server major version once in the worker struct at startup:

```go
// In PerformanceDebtWorker struct, set once at startup:
var sqlMajorVersion int
db.QueryRowContext(ctx,
    "SELECT CAST(SERVERPROPERTY('ProductMajorVersion') AS INT)").Scan(&sqlMajorVersion)

// In FetchVLFCount — branch on version:
if sqlMajorVersion >= 14 {
    // SELECT total_vlf_count, active_log_size_mb, log_truncation_holdup_reason
    // FROM sys.dm_db_log_stats(DB_ID())
} else {
    // SELECT COUNT(*) AS vlf_count, NULL, NULL
    // FROM sys.dm_db_log_info(DB_ID())
}
```

> **Shortcut available**: `sqlserver_server_properties` now stores `sqlserver_start_time` and could store `version_major` if added. For now, a one-time `SERVERPROPERTY` call at worker startup is acceptable since it is not a hot path.

Store `active_log_size_mb` and `log_truncation_holdup_reason` in the finding's `details` JSON blob.

---

### 6.4 `FetchLastFullBackupAgeHours` — `sqlserver_performance_debt.go:373`

**Issue: Missing `WITH (NOLOCK)` on msdb**

`msdb.dbo.backupset` can be large and is written during backup operations. A shared lock can cause brief stalls. Add `WITH (NOLOCK)`:

```sql
FROM msdb.dbo.backupset WITH (NOLOCK)
WHERE database_name = @p1
  AND type = 'D'
ORDER BY backup_finish_date DESC;
```

**Note on differential backups**: The query correctly reports last FULL backup age. Sites using full + differential strategy will see a "stale full" warning on day 2+. This is correct policy behavior. Update the `recommendation` text to clarify: "Full backup is {age} old. Ensure differential backups are covering the RPO gap."

---

### 6.5 `FetchStaleStatistics` — `sqlserver_performance_debt.go:243`

**Quality: Well-constructed.** The `CASE WHEN rows <= 25000 THEN rows * 0.20 ELSE (0.20 * rows) + SQRT(1000.0 * rows)` dynamic threshold matches SQL Server's internal auto-update formula exactly. No logic change needed.

**Minor enrichment**: Add two context columns to the finding details JSON:

```sql
-- Add to SELECT:
s.auto_created,    -- TRUE = optimizer-created; treat differently from user-managed
s.is_incremental   -- TRUE = incremental stats on partitioned table
```

If `is_incremental = 1`, change fix script to:
```sql
UPDATE STATISTICS [{schema}].[{table}] ([{stats_name}]) WITH RESAMPLE ON PARTITIONS(...);
-- FULLSCAN on a partitioned table scans the entire table; RESAMPLE is more efficient
```

---

### 6.6 `FetchAutogrowthRisks` — `sqlserver_performance_debt.go:310`

**Issue: Growth settings checked without current fill level**

A percent-growth file at 95% utilization is an emergency. A percent-growth file at 5% is low risk. The current query does not compute fill level, so the worker cannot distinguish.

```sql
-- Enhanced: add fill level via FILEPROPERTY
SELECT TOP (50)
    name,
    type,
    type_desc,
    size * 8 / 1024.0                                           AS size_mb,
    CAST(FILEPROPERTY(name, 'SpaceUsed') AS FLOAT) * 8 / 1024  AS used_mb,
    CAST(FILEPROPERTY(name, 'SpaceUsed') AS FLOAT) * 100.0
        / NULLIF(size, 0)                                       AS pct_full,
    CASE WHEN max_size IN (-1, 268435456) THEN -1
         ELSE max_size * 8 / 1024.0 END                        AS max_size_mb,
    growth,
    is_percent_growth,
    physical_name
FROM sys.database_files
ORDER BY pct_full DESC;
```

**Updated severity rules in worker** — escalate `is_percent_growth` findings based on fill:

| Condition | Severity |
|---|---|
| `is_percent_growth = 1 AND pct_full > 70` | CRITICAL — percent growth on nearly-full file causes exponential consumption |
| `is_percent_growth = 1 AND size_mb > 100` | WARNING (existing rule) |
| `growth_mb < 64 AND size_mb > 1024` | WARNING — small growth increment on large file |

---

## 7. Frontend UI Design — Server Vitals Section

### Placement

Insert **above** the existing severity KPI cards (Critical / Warning / Info / Total). Server resource context must be the first thing a DBA sees, before query-level debt details.

### Component 1: Server Vitals Ribbon (always visible)

Three-card flex row:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  SERVER VITALS                                               ⏱ 2 min ago    │
├─────────────────────┬────────────────────────────┬───────────────────────────┤
│  MEMORY             │  STORAGE (WORST VOLUME)    │  ACTIVE ALERTS            │
│  OS:  64 GB total   │  C:\  —  15.4% free        │  ⚠ Volume C:\ low         │
│  SQL: 48 GB in use  │  18.5 GB remaining          │  ✓ Memory OK              │
│  73% utilized       │  ████████░░                 │                           │
│  PLE: 1,840 sec     │  [click to expand table]    │                           │
│  State: AVAILABLE   │                             │                           │
└─────────────────────┴────────────────────────────┴───────────────────────────┘
```

> Added `State: AVAILABLE` row — `os_system_memory_state` is now collected and available in `sqlserver_memory_metrics`.

Color coding:
- **Memory**: green (<70% util), amber (70-85%), red (>85% OR `process_physical_low = true`)
- **Storage**: green (worst volume > 25% free), amber (10-25%), red (< 10%)

### Component 2: Storage & File Locations (collapsible accordion)

Title: `Storage & File Locations` — same accordion style as existing sections.

Table columns: Database | File Type | Logical Name | Physical Path | Volume | Total (GB) | Free (GB) | Free %

Row coloring: red if `free_pct < 10`, amber if `free_pct < 25`.

**Refresh button**: Calls `GET /api/sqlserver/volume-stats/live?instance={name}` for on-demand DMV refresh (collection is every 6 hours).

### Component 3: Alert surfacing

No new mechanism needed. Vitals findings written to `sqlserver_performance_debt_findings` appear automatically in the existing Warning/Critical banner logic.

---

## 8. Implementation Order

### Completed (this branch)

| Priority | Task | Files | Status |
|---|---|---|---|
| **P1** | Fix `staging.sqlserver_io_raw` INSERT to write all 15 columns | `ts_logger_pulses.go` | ✅ **DONE** — `LogStagingIORows` writes all 15 columns |
| **P-sys_info** | Extend `sys.dm_os_sys_info` collection: `sqlserver_start_time`, `ms_ticks`, `scheduler_count` | `sqlserver_cpu.go`, `sqlserver_health_worker.go`, `ts_logger_sqlserver.go`, `01_timescale_schema.sql`, `models/dashboard.go` | ✅ **DONE** |
| **P-sys_mem_state** | Add `os_system_memory_state` to `sqlserver_memory_metrics` | `sqlserver_memory_analyzer.go`, `sqlserver_memory_intelligence.go`, `01_timescale_schema.sql` | ✅ **DONE** |
| **P-perf_counters** | Unify `sys.dm_os_performance_counters` — extend table, new collector, redirect consumers | `sqlserver_perf_counters_collector.go`, `sqlserver_perf_counters_service.go`, `ts_logger_sqlserver_enterprise_v2.go`, `01_timescale_schema.sql` + 5 consumer files | ✅ **DONE** — 30 s collection, `idx_pc_dedup` dedup |
| **P-sessions** | Unify `sys.dm_exec_sessions` + `dm_exec_requests` — new table, collector, service | `sqlserver_session_collector.go`, `sqlserver_session_service.go`, `ts_logger_sqlserver_sessions.go`, `01_timescale_schema.sql` | ✅ **DONE** — hash-delta dedup |
| **P-idx_frag** | Unify `sys.dm_db_index_physical_stats` — new table, collector, service, cache-first redirect | `sqlserver_index_fragmentation_collector.go`, `sqlserver_index_service.go`, `ts_logger_sqlserver_index_frag.go`, `sqlserver_performance_debt.go`, `01_timescale_schema.sql` | ✅ **DONE** — SAMPLED mode, 6 h |
| **P-missing_idx** | Unify `sys.dm_db_missing_index_*` — new table, collector, service, cache-first redirect | `sqlserver_missing_index_collector.go` (same index service), `ts_logger_sqlserver_index_frag.go`, `sqlserver_performance_debt.go`, `01_timescale_schema.sql` | ✅ **DONE** — 6 h, CREATE INDEX reconstruction |
| **P-file_catalog** | New `sqlserver_file_catalog` table (schema only) | `01_timescale_schema.sql` | ✅ **DONE** (table ready; full IO collector future work) |
| **P-memory_grants_detail** | New `sqlserver_memory_grants_detail` table (schema only) | `01_timescale_schema.sql` | ✅ **DONE** (table ready; writer in `ts_logger_sqlserver_sessions.go`) |

### Still Pending

| Priority | Task | Files | DMV Touched? |
|---|---|---|---|
| **P0** | Fix `dm_os_volume_stats` incorrect join in `sqlserver_best_practices.go:408` | `sqlserver_best_practices.go` | Just fix existing query |
| **P2** | Add 5 columns to `sqlserver_memory_metrics` + extend `FetchMemoryAnalyzerSnapshot` | `sqlserver_memory_analyzer.go`, `01_timescale_schema.sql` | Extends existing `dm_os_process_memory` query |
| **P3** | Add `sqlserver_volume_stats` table to schema | `01_timescale_schema.sql` | New table only |
| **P4** | Add `FetchVolumeStats` with correct CROSS APPLY | `sqlserver_volume.go` (new) | First real query of `dm_os_volume_stats` correctly |
| **P5** | Add Phase 8 (`collectServerVitals`) + `evaluateServerVitalsFindings` to worker | `performance_debt_worker.go` | Reads `sqlserver_memory_metrics`; calls FetchVolumeStats |
| **P6** | Add `LogServerVitals` writer (volume stats INSERT) | `ts_logger_performance_debt.go` or new file | — |
| **P7** | Add `GET /api/sqlserver/server-vitals` endpoint | `sqlserver.go`, `monitoring_routes.go` | Reads from existing tables only |
| **P8** | Add `GET /api/sqlserver/volume-stats/live` endpoint | `sqlserver.go`, `monitoring_routes.go` | Calls FetchVolumeStats directly |
| **P9** | Frontend: Server Vitals Ribbon + Storage accordion | `sqlserver_PerformanceDebt.js` | — |
| **P10** | Apply 6 query fixes from §6 (Columnstore filter in frag fallback, thresholds in missing index fallback, VLF version-branch, backup NOLOCK, stats enrichment, autogrowth fill level) | `sqlserver_performance_debt.go`, `sqlserver_missing_index_collector.go` | Minor changes to existing queries |
| **P11** | Add Phase 9: Query Store (`is_query_store_on` to DB catalog + new QS options query) | `sqlserver_database.go`, `01_timescale_schema.sql`, `performance_debt_worker.go` | Extends `sys.databases` query; new `sys.database_query_store_options` query |
| **P12** | Add Phase 10: TempDB design (reads `sqlserver_tempdb_files` + `sqlserver_server_properties`) | `performance_debt_worker.go` | No new DMV — queries existing tables |
| **P13** | Add Phase 11: Destructive settings (reads `sqlserver_database_catalog`) | `performance_debt_worker.go` | No new DMV — queries existing table |
| **P14** | Add Phase 12: I/O latency findings (reads `staging.sqlserver_io_raw` — now fully populated) | `performance_debt_worker.go` | No new DMV — reads fixed staging table |

---

## 9. Verification Checklist

### Completed (verified or derivable from code)

- [x] `cd backend && go build ./...` passes (verified after each phase of consolidation work)
- [x] `cd backend && golangci-lint run` passes
- [x] `SELECT * FROM staging.sqlserver_io_raw LIMIT 1` — all 15 columns populated (not just 6). Fix in `ts_logger_pulses.go:LogStagingIORows`.
- [x] `SELECT os_system_memory_state FROM sqlserver_memory_metrics LIMIT 1` — column exists and is populated by `sqlserver_memory_intelligence.go`
- [x] `SELECT sqlserver_start_time, ms_ticks, scheduler_count FROM sqlserver_server_properties LIMIT 1` — all three new columns exist and populated by health worker
- [x] `SELECT instance_name, cntr_value, cntr_type FROM sqlserver_perf_counters LIMIT 1` — three new columns populated by `PerfCountersCollector` (30 s cadence)
- [x] `SELECT * FROM sqlserver_active_sessions LIMIT 1` — rows appear every 30 s from `StartSessionCollector`; delta hash prevents duplicate snapshots
- [x] `SELECT * FROM sqlserver_index_fragmentation LIMIT 1` — rows appear at 6 h cadence from `StartIndexCollector`; SAMPLED mode; filters `>= 5% fragmentation AND >= 100 pages`
- [x] `SELECT * FROM sqlserver_missing_indexes LIMIT 1` — rows appear at 6 h cadence from `StartIndexCollector`
- [x] `FetchIndexFragmentation` returns from TimescaleDB cache when data is < 8 hours old (no DMV call)
- [x] `FetchMissingIndexRecommendations` returns from TimescaleDB cache when data is < 8 hours old (no DMV call); CREATE INDEX statement correctly reconstructed from stored columns
- [x] `FetchDatabaseThroughput` reads Transactions/sec from `sqlserver_perf_counters` (cache-first) instead of DMV

### Still Pending

- [ ] `SELECT * FROM sqlserver_memory_metrics LIMIT 1` — shows new process-memory columns (`sql_physical_memory_in_use_mb`, `sql_memory_utilization_pct`, etc.) after Feature A work
- [ ] `SELECT * FROM sqlserver_volume_stats WHERE server_id = '{id}' LIMIT 5` — returns rows after first collection cycle (Feature B work)
- [ ] `GET /api/sqlserver/server-vitals?instance={name}` returns HTTP 200 with memory + volumes sections populated
- [ ] `GET /api/sqlserver/volume-stats/live?instance={name}` returns HTTP 200 directly from DMV (uses CROSS APPLY, not LEFT() match)
- [ ] Performance Debt page shows Server Vitals Ribbon above severity KPI cards
- [ ] Storage & File Locations accordion shows all database files with GB and % values
- [ ] Volume below 15% free creates a WARNING finding in `sqlserver_performance_debt_findings`
- [ ] `process_physical_memory_low = true` creates a CRITICAL finding
- [ ] TempDB unequal files creates a WARNING finding (reads from `sqlserver_tempdb_files`, no new DMV)
- [ ] I/O latency > threshold creates findings (reads from `staging.sqlserver_io_raw` — now fully populated)
- [ ] Destructive settings findings read from `sqlserver_database_catalog`, no new DMV query
- [ ] Query Store findings generated for databases where `actual_state_desc = 'READ_ONLY'`
- [ ] `FetchIndexFragmentation` fallback query excludes Columnstore indexes (`type NOT IN (5, 6)`)
- [ ] `MissingIndexCollector` fallback threshold raised to `user_seeks >= 500` and `improvement_score >= 5000`
- [ ] No regression in existing accordion sections (Index Health, Statistics Health, Storage & Growth, etc.)
