# Cold Storage Pipeline — End-to-End Testing Plan

> **Date:** May 2026  
> **Scope:** Validate TimescaleDB compression → cold storage export → S3/MinIO upload → DuckDB query  
> **Target branch:** `post_release_cold_storage_refinement`

---

## 1. Objectives

| # | Objective | Pass Criteria |
|---|-----------|---------------|
| 1 | Seed all 35 registered tables with realistic historical data spanning 30, 60, and 90 days | Row counts match expected volumes per table |
| 2 | Verify TimescaleDB compression activates on aged chunks | `is_compressed = TRUE` for chunks older than 7 days |
| 3 | Verify retention policies will drop data beyond hot-tier limits | `drop_chunks` removes records outside retention window |
| 4 | Trigger cold storage export cycle and confirm watermarks advance | `coldstorage.watermarks.last_exported_at` moves forward |
| 5 | Confirm Parquet files land in MinIO with correct Hive partition layout | `mc ls` shows files under `engine=*/table=*/server_id=*/year=*/month=*/day=*/` |
| 6 | Query cold Parquet files with DuckDB and validate row counts match export | `SELECT COUNT(*)` in DuckDB matches exported rows |
| 7 | Validate federated Trino query (`/api/cold-storage/query`) returns data | HTTP 200 with non-empty result set |
| 8 | Verify cold storage admin UI tab shows correct status and run history | Browser shows run status, row/byte counts, watermarks per table |

---

## 2. Test Environment Setup

### 2.1 Prerequisites

```bash
# Start full stack with cold-storage profile (MinIO + Nessie + Trino)
cd docker
COMPOSE_PROFILES=cold-storage docker compose up -d

# Verify MinIO is healthy
curl -s http://localhost:9000/minio/health/live && echo "MinIO OK"

# Verify bucket was created
docker exec -it sql_monitoring_UI-minio-1 mc ls local/sql-optima-cold/ || echo "Bucket not yet created"

# Enable cold storage in .env (or override inline)
# COLD_STORAGE_ENABLED=true
# COLD_STORAGE_ENDPOINT=http://minio:9000
# COLD_STORAGE_ACCESS_KEY_ID=sqloptima
# COLD_STORAGE_SECRET_ACCESS_KEY=change_me_in_production
# COLD_STORAGE_BUCKET=sql-optima-cold
# COLD_STORAGE_LAG_DAYS=2
```

### 2.2 Test Server UUID

All test scripts use a single fixed server UUID so results are predictable and cleanup is precise:

```
Test Server UUID : aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa  (SQL Server engine)
Test PG UUID     : bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb  (PostgreSQL engine)
```

Both are registered as `is_active = TRUE` so the exporter picks them up.

---

## 3. Data Seeding Strategy

### 3.1 Time Window Design

```
NOW()
  │
  │←──── 2 days ─────────┐  ExportCutoff (data newer than this is NOT exported)
  │                       │
  ├── 8 days ago ─────────┼── Compression boundary (TimescaleDB compresses chunks >7d old)
  │                       │
  ├── 30 days ago ────────┼── 30-day Group B hot-tier upper bound
  │   (recent archive)    │
  │                       │
  ├── 60 days ago ────────┼── Mid-archive window
  │   (mid archive)       │
  │                       │
  ├── 90 days ago ────────┼── 90-day Group A hot-tier upper bound
  │   (deep archive)      │
  │                       │
  └── 92 days ago ─────── oldest seeded record (2 days inside 90d retention boundary)
```

### 3.2 Seeding Layers

| Layer | Time Range | Tables Covered | Interval | Purpose |
|-------|-----------|----------------|----------|---------|
| **90-day tier** | 92d → 32d ago | Group A (SQL Server + PG core) | 30 min | Deep archive, should be fully compressed and exported |
| **60-day tier** | 62d → 32d ago | Group A + Group B | 15 min | Mid archive, overlaps 90d layer |
| **30-day tier** | 32d → 3d ago | Group A + Group B | 5 min | Recent archive, still within hot retention |

### 3.3 Row Volume Estimates

| Table | Interval | 90d rows | 60d rows | 30d rows | Total |
|-------|---------|---------|---------|---------|-------|
| `sqlserver_cpu_history` | 30 min | 4,320 | 2,880 | 8,640 | 15,840 |
| `sqlserver_wait_history` (×5 types) | 30 min | 21,600 | 14,400 | 43,200 | 79,200 |
| `sqlserver_metrics` | 30 min | 4,320 | 2,880 | 8,640 | 15,840 |
| `sqlserver_disk_history` (×3 DBs) | 30 min | 12,960 | 8,640 | 25,920 | 47,520 |
| `postgres_wait_event_stats` (×6 types) | 30 min | 25,920 | 17,280 | 51,840 | 95,040 |
| (all other tables) | 30 min | ~4,000–8,000 each | — | — | ~500K total |

> **Note:** These volumes are sufficient to exercise compression (chunks have data), export watermark advancement, and Parquet file creation, without taking hours to insert.

---

## 4. Test Execution Steps

### Step 1 — Seed Test Servers

```bash
psql -h localhost -U postgres -d sql_optima \
  -f infrastructure/sql_scripts/Test_scripts/01_cold_storage_test_setup.sql
```

**Verifies:** Both test UUIDs exist in `optima_servers`; `coldstorage.watermarks` is empty for them.

---

### Step 2 — Insert 90-Day Group A Data

```bash
psql -h localhost -U postgres -d sql_optima \
  -f infrastructure/sql_scripts/Test_scripts/02_cold_storage_test_90day_data.sql
```

**What it inserts:** Core SQL Server metrics (CPU, memory, wait, disk, connection, lock, throughput, memory metrics, buffer pool, scheduler, risk health) and PostgreSQL tables (settings snapshot, backup archiver, basebackup history, roles, failed logins) — all with timestamps from 92 to 32 days ago at 30-minute intervals.

**Expected:** ~15,000–25,000 rows per table; `RAISE NOTICE` confirms each table.

---

### Step 3 — Insert 30-Day Group B Data

```bash
psql -h localhost -U postgres -d sql_optima \
  -f infrastructure/sql_scripts/Test_scripts/03_cold_storage_test_30day_data.sql
```

**What it inserts:** Group B tables (latch waits, spinlock stats, procedure stats, long-running queries, AG health, risk health, memory grant waiters, waiting tasks) and PostgreSQL tables (wait event stats, DB I/O stats, session activity, wait event summary, DB load, query wait profile, DDL activity) — with timestamps from 32 to 3 days ago at 15-minute intervals, plus spot data from the 90-day and 60-day windows.

---

### Step 4 — Verify Data Counts Before Export

```bash
psql -h localhost -U postgres -d sql_optima \
  -f infrastructure/sql_scripts/Test_scripts/04_cold_storage_verify_pre_export.sql
```

**Expected output:** A table showing `table_name | row_count | min_ts | max_ts` for each registered table, with non-zero counts across a 90-day range.

---

### Step 5 — Force TimescaleDB Compression

```bash
psql -h localhost -U postgres -d sql_optima \
  -f infrastructure/sql_scripts/Test_scripts/05_cold_storage_force_compress.sql
```

**What it does:** Calls `compress_chunk()` on all chunks older than 7 days for each Group A table. In production, this happens automatically; in testing we force it so the exporter's compression check passes immediately.

**Verify:**
```sql
SELECT hypertable_name, COUNT(*) FILTER (WHERE is_compressed) AS compressed,
       COUNT(*) FILTER (WHERE NOT is_compressed) AS uncompressed
FROM timescaledb_information.chunks
WHERE range_end < NOW() - INTERVAL '7 days'
  AND hypertable_schema = 'public'
GROUP BY 1 ORDER BY 1;
```

---

### Step 6 — Trigger Cold Storage Export (Manual)

The exporter runs at 02:00 UTC nightly. For testing, trigger it manually via the API:

```bash
# Option A: API trigger endpoint (if wired)
curl -X POST http://localhost:8080/api/admin/cold-storage/trigger \
  -H "Authorization: Bearer $ADMIN_JWT"

# Option B: Set COLD_STORAGE_LAG_DAYS=3 and restart the API with COLD_STORAGE_ENABLED=true
# The exporter will run at 02:00 UTC or on next scheduled tick.

# Option C: Temporarily modify COLD_STORAGE_LAG_DAYS to 0 to export all data up to today
```

**What to watch in API logs:**
```
[ColdExporter] Starting export cycle, cutoff=2026-05-28T00:00:00Z
[ColdExporter] Exporting sqlserver_cpu_history / server=aaaaaaaa... from=2026-03-01 to=2026-05-28
[ColdExporter] Uploaded sqlserver_cpu_history (4320 rows) → s3://sql-optima-cold/metrics/engine=sqlserver/...
[ColdExporter] Export cycle complete. Status: success, Rows: 345000, Bytes: 52428800
```

---

### Step 7 — Verify Export Watermarks

```bash
psql -h localhost -U postgres -d sql_optima \
  -f infrastructure/sql_scripts/Test_scripts/06_cold_storage_verify_watermarks.sql
```

**Expected output:** All registered tables show `last_exported_at` close to `ExportCutoff()` (NOW() - 2 days).

---

### Step 8 — Verify MinIO File Layout

```bash
# List all uploaded Parquet files
docker exec sql_monitoring_UI-minio-1 mc ls --recursive \
  local/sql-optima-cold/metrics/ | head -40

# Expected pattern:
# [2026-05-30] ...KB metrics/engine=sqlserver/table=sqlserver_cpu_history/server_id=aaaaa.../year=2026/month=03/day=01/part-000001.parquet
# [2026-05-30] ...KB metrics/engine=sqlserver/table=sqlserver_cpu_history/server_id=aaaaa.../year=2026/month=03/day=02/part-000001.parquet

# Count total Parquet files
docker exec sql_monitoring_UI-minio-1 mc ls --recursive local/sql-optima-cold/ | grep ".parquet" | wc -l
# Expected: ~2000+ files (35 tables × 2 servers × ~30 days = ~2100 files)
```

---

### Step 9 — DuckDB Validation

Install DuckDB if not already available:

```bash
# macOS
brew install duckdb

# Direct binary
curl -LO https://github.com/duckdb/duckdb/releases/latest/download/duckdb_cli-osx-universal.zip
unzip duckdb_cli-osx-universal.zip && chmod +x duckdb
```

Run DuckDB validation queries:

```bash
duckdb -init infrastructure/sql_scripts/Test_scripts/07_cold_storage_duckdb_validation.sql
```

**Expected output:**
```
-- CPU history validation
┌──────────────────────────────┬────────┬────────────────────┬────────────────────┐
│         table_path           │  cnt   │      min_ts        │      max_ts        │
├──────────────────────────────┼────────┼────────────────────┼────────────────────┤
│ sqlserver_cpu_history        │  4320  │ 2026-02-28 00:00   │ 2026-04-29 23:30   │
└──────────────────────────────┴────────┴────────────────────┴────────────────────┘
```

---

### Step 10 — Federated Trino Query Test

```bash
# Verify Trino is running
curl http://localhost:8080/api/cold-storage/status | jq '.trino_url'

# Run a federated query via the SQL Optima API
curl -X POST http://localhost:8080/api/cold-storage/query \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "sql": "SELECT COUNT(*) AS total_rows FROM cold.metrics.\"sqlserver_cpu_history\" WHERE server_id = '\''aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'\''"
  }'
```

---

### Step 11 — Cold Storage Admin UI

1. Open `http://localhost:8080`
2. Navigate to **Admin → Cold Storage**
3. Verify:
   - **Status tab** shows both test servers with `last_exported_at` timestamps
   - **Run History tab** shows the successful export run with row/byte counts
   - **Tables Behind** list is empty (all watermarks current)

---

### Step 12 — Cleanup

```bash
psql -h localhost -U postgres -d sql_optima \
  -f infrastructure/sql_scripts/Test_scripts/08_cold_storage_test_cleanup.sql
```

**What it removes:** All rows for the two test server UUIDs from every registered table, removes their watermarks from `coldstorage.watermarks`, and deletes the test server entries from `optima_servers`.

---

## 5. Pass / Fail Criteria Summary

| Test | Pass | Fail |
|------|------|------|
| Row counts in TimescaleDB | Every table has rows spanning ≥30 days | Any table has 0 rows |
| Chunk compression | Chunks >7 days old show `is_compressed=TRUE` | Uncompressed chunks for old data |
| Watermark advancement | All 35 tables have `last_exported_at` within 24h of cutoff | Any table has NULL or unchanged watermark |
| MinIO file count | ≥500 Parquet files uploaded | 0 files or wrong partition path |
| DuckDB row count | DuckDB COUNT(*) within 1% of TimescaleDB COUNT(*) | >1% discrepancy or DuckDB query error |
| Trino federated query | HTTP 200, non-empty `rows` array | HTTP error or empty result |
| Admin UI | Status tab shows both servers; run shows in history | Empty status or 0 tables |
| Export run status | `coldstorage.runs` shows `status = 'success'` | `status = 'partial'` or `'failed'` |

---

## 6. Known Limitations and Test Notes

| Limitation | Impact | Workaround |
|-----------|--------|------------|
| `monitor.collector_runs` table may not exist as a hypertable in all deployments | Export skips this table silently | Verify manually with `\dt monitor.*` |
| `monitor.sqlserver_query_store_snapshot` and `monitor.sqlserver_query_store_interval` need Query Store enabled on SQL Server | No real data; test data inserted manually | Seed data with realistic-looking JSONB columns |
| TimescaleDB compression requires chunks to be fully populated (no ongoing writes) | Test data uses fixed UUIDs so real collectors won't interfere | Use different UUIDs from production servers |
| Parquet file sizes depend on Snappy compression ratio | Expect ~3–10× compression vs raw row size | Row counts are the reliable metric, not byte counts |
| `COLD_STORAGE_LAG_DAYS=2` means data within the last 2 days is never exported | All test data seeded older than 3 days | Confirmed: all seeded timestamps are ≥3 days old |

---

## 7. Post-Validation Actions (Non-Automated)

After 2+ weeks of successful production exports, perform these manual steps:

1. **Reduce hot-tier retention** for Group A core tables from 90 to 60 days by applying the opt-in migration (never auto-run):

   ```bash
   psql "$DATABASE_URL" -f infrastructure/sql_scripts/migrations/011_cold_storage_reduce_hot_retention_60d.sql
   ```

   Confirm watermarks and S3 Parquet first (`GET /api/cold-storage/status`). Keep a Timescale backup.

2. **Enable Iceberg registration** by setting `COLD_STORAGE_CATALOG_URL=http://nessie:19120/api/v1` in `.env`.

3. **Enable Trino** by starting with `COMPOSE_PROFILES=cold-storage` and setting `COLD_STORAGE_TRINO_URL` in `.env`.

4. **Enable frontend time-range picker** (Phase 3): When `COLD_STORAGE_ENABLED=true`, `/api/config` exposes `cold_storage_enabled` and `max_dashboard_range_days=90`; the global picker shows **30d/90d** presets and `ParseTimeRange` allows up to 90 days.
