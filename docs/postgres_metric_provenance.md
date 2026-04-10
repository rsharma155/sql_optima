## PostgreSQL Control Center — Metric provenance

This document explains where each metric on **PostgreSQL Control Center** comes from, how often it updates, and what to do when it looks bad.

### Collection cadence

- **Collector loop**: controlled by `ENTERPRISE_METRICS_INTERVAL_SEC` (default 120s).
- **UI refresh**: page refresh button or browser reload (UI reads latest + recent history from Timescale).

### Workload & Activity

- **Database Throughput (TPS)**
  - **Source**: Timescale `postgres_control_center_stats.tps` (derived from `pg_stat_database` transaction counters).
  - **Meaning**: Total commit+rollback rate (transactions per second).
  - **When bad**: Sudden drops can indicate stalled workload; spikes can correlate with CPU/IO pressure.
  - **Actions**: correlate with active/waiting sessions and slow queries; check CPU, IO, lock waits.

- **Active Sessions**
  - **Source**: `pg_stat_activity` (snapshot), stored in Timescale (`postgres_control_center_stats.active_sessions`).
  - **Meaning**: sessions actively running on CPU (not idle).
  - **When bad**: sustained growth indicates workload surge or queries not completing.
  - **Actions**: open Sessions page; identify longest running / highest impact queries; check waits.

- **Waiting Sessions**
  - **Source**: `pg_stat_activity` (snapshot), stored in Timescale (`postgres_control_center_stats.waiting_sessions`).
  - **Meaning**: sessions waiting on some event (locks, IO, LWLocks, etc).
  - **When bad**: high waiting with low CPU often means contention (locks) or IO stall.
  - **Actions**: open Locks/Blocking; check top waits; check storage latency.

- **Blocking Sessions**
  - **Source**: `pg_blocking_pids()` against `pg_stat_activity` (snapshot), stored in Timescale (`postgres_control_center_stats.blocking_sessions`).
  - **Meaning**: count of sessions currently blocked by other sessions.
  - **When bad**: any persistent non-zero value is worth investigation.
  - **Actions**: open Locks/Blocking; find top blocker, longest blocked duration; consider killing blocker only after validation.

- **Slow Queries**
  - **Source**: `pg_stat_statements` (if enabled), stored in Timescale (`postgres_control_center_stats.slow_queries`).
  - **Meaning**: count of queries exceeding a latency threshold (collector-defined) in the current snapshot window.
  - **When bad**: rising slow-query count usually correlates with regressions, missing indexes, or resource contention.
  - **Actions**: open Query Performance; review top time/mean time; validate `pg_stat_statements` is enabled.

### Durability & Risk Signals

- **WAL Rate (MB/min)**
  - **Source**: `pg_stat_wal` (preferred) or WAL bytes counters (collector fallback); stored in Timescale (`postgres_control_center_stats.wal_rate_mb_per_min`).
  - **Meaning**: WAL generation rate (higher means more write churn).
  - **When bad**: high WAL rate can exhaust disk (especially on dedicated WAL volume) or increase replica lag.
  - **Actions**: correlate with checkpoints, vacuum, bulk loads; check disk time-to-full and replica lag.

- **Checkpoint Pressure**
  - **Source**: `pg_stat_bgwriter` / checkpoint counters, stored in Timescale (`postgres_control_center_stats.checkpoint_pressure_pct`).
  - **Meaning**: heuristic indicator that checkpoints are happening too frequently / too aggressively.
  - **When bad**: sustained high pressure can cause IO spikes and latency.
  - **Actions**: review `checkpoint_timeout`, `max_wal_size`, `shared_buffers`; validate storage throughput.

- **XID Age / Wraparound Risk**
  - **Source**: `pg_database.datfrozenxid` age calculation; stored in Timescale (`postgres_control_center_stats.xid_age`, `xid_wrap_pct`).
  - **Meaning**: how close the cluster is to transaction ID wraparound.
  - **When bad**: high wrap% is urgent; risk of forced shutdown if not vacuumed.
  - **Actions**: ensure autovacuum is running; investigate tables with high freeze age; run `VACUUM (FREEZE)` where appropriate.

- **Dead Tuple Ratio**
  - **Source**: `pg_stat_user_tables` dead tuples vs live tuples (aggregate); stored in Timescale (`postgres_control_center_stats.dead_tuple_ratio_pct`).
  - **Meaning**: bloat pressure / vacuum health signal.
  - **When bad**: high dead% means vacuums are lagging or workload churn is high.
  - **Actions**: open Storage/Vacuum; check vacuum progress and bloat candidates; tune autovacuum thresholds.

- **Autovacuum Workers**
  - **Source**: `pg_stat_activity` filtered to autovacuum processes; stored in Timescale (`postgres_control_center_stats.autovacuum_workers`).
  - **Meaning**: how many autovacuum workers are currently busy.
  - **When bad**: 0 is fine if workload is light; sustained maxed-out workers can mean vacuum can’t keep up.
  - **Actions**: check `autovacuum_max_workers`, IO limits, and top bloat candidates.

### System tiles

- **Disk Free**
  - **Source**: local `statfs` on configured `pg_disk_paths`; stored in Timescale (`postgres_disk_stats.free_bytes`, `used_pct`).
  - **Meaning**: free space on relevant mount(s), usually `wal` and/or `data`.
  - **When bad**: < 20% is warning; < 10% is critical depending on workload.
  - **Actions**: increase disk, clear WAL archives if misconfigured, reduce WAL rate drivers, validate retention jobs.

- **Disk “~time to full” (rough)**
  - **Source**: derived in UI using Disk Free on `wal` mount + WAL Rate (MB/min).
  - **Meaning**: coarse ETA until WAL disk fills if current WAL rate persists.
  - **Actions**: treat as a directional indicator; confirm with actual ingestion patterns and disk growth trends.

- **Backup**
  - **Source**: external job reports to `POST /api/postgres/backups/report`; stored in Timescale `postgres_backup_runs`.
  - **Meaning**: latest reported backup status.
  - **When bad**: failed or stale success is a recovery risk.
  - **Actions**: fix backup pipeline; validate restore procedure; enforce RPO/RTO.

- **Critical Errors**
  - **Source**: external log shipper reports to `POST /api/postgres/logs/report`; stored in Timescale `postgres_log_events`.
  - **Meaning**: count of ERROR/FATAL/PANIC and selected patterns (auth fails, OOM) in the last window.
  - **When bad**: any PANIC/FATAL deserves immediate investigation.
  - **Actions**: open Logs drilldown; correlate time with deploys, resource saturation, query spikes.

- **Pooler (PgBouncer)**
  - **Source**: PgBouncer admin `SHOW POOLS` (via `pgbouncer_admin_dsn`), stored in Timescale `postgres_pooler_stats`.
  - **Meaning**: waiting clients, used server connections, max wait time.
  - **When bad**: waiting clients or high max wait indicates pool saturation or backend contention.
  - **Actions**: check server max connections, pool size, query latency, lock waits; consider pool tuning.

