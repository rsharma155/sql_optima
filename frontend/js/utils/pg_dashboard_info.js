/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Comprehensive information data for PostgreSQL dashboards.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function () {
    'use strict';

    window.pgDashboardInfo = {
        "Control Center": {
            description: "The primary command center providing a high-level, real-time overview of database health, throughput, concurrency, and operational diagnostics. This is the 'first look' dashboard — a DBA should be able to determine within 30 seconds whether the instance is healthy or in crisis. It consolidates signals from pg_stat_activity, pg_stat_bgwriter, and pg_stat_database to provide a 360-degree view of instance performance.",
            metrics: {
                "Health Score": {
                    title: "Health Score (0–100%)",
                    text: "**What it is:** A composite index calculated from weighted sub-scores across session health, cache efficiency, wait event distribution, replication lag, and bloat risk.\n\n**Score interpretation:**\n- ✅ 90–100%: Healthy\n- ⚠️ 70–89%: Degraded — investigate trending metrics\n- 🔴 50–69%: Significant issues — active intervention likely needed\n- 🚨 < 50%: Emergency — multiple critical conditions active simultaneously\n\n**Why it matters:** Provides a single actionable number for quick triage during on-call rotations."
                },
                "Active Sessions": {
                    title: "Active Sessions",
                    text: "**What it is:** Count of connections with `state = 'active'` in `pg_stat_activity`. These are sessions currently executing a query.\n\n**Why it matters:** Active session count is the real-time concurrency gauge. A sudden spike without a corresponding TPS increase means queries are running slower (possible contention or regression).\n\n**PostgreSQL nuance:** Every connection is a separate OS process consuming ~5–10MB of memory. High active counts have both a concurrency and memory impact."
                },
                "Waiting Sessions": {
                    title: "Waiting Sessions",
                    text: "**What it is:** Count of connections with a non-null `wait_event` in `pg_stat_activity`. These sessions are stalled on a resource — a lock, I/O, a buffer pin, or network.\n\n**Why it matters:** A high waiting-to-active ratio is the clearest signal of a systemic bottleneck.\n\n**Key categories:** Lock (relation, transactionid, tuple), LWLock (BufferMapping, WALWrite), IO (DataFileRead), Client (ClientRead, ClientWrite)."
                },
                "Blocked Sessions": {
                    title: "Blocked Sessions",
                    text: "**What it is:** Count of sessions with a non-null result from `pg_blocking_pids(pid)` — sessions waiting specifically on a PostgreSQL lock held by another session.\n\n**Thresholds:**\n- ✅ Healthy: 0\n- ⚠️ Warning: 1–3 blocked sessions persisting > 15 seconds\n- 🔴 Critical: > 3 blocked, or any session blocked > 60 seconds\n\n**Critical pattern:** A session in `idle in transaction` holds all locks indefinitely, preventing autovacuum and blocking DDL."
                },
                "TPS": {
                    title: "TPS (Transactions Per Second)",
                    text: "**What it is:** The rate of committed transactions per second, derived from `xact_commit` delta in `pg_stat_database`.\n\n**Why it matters:** TPS is the heartbeat of the database. A sudden drop signals a bottleneck.\n\n**DBA action:** Dropping TPS + rising waits = blocked concurrency. Dropping TPS + rising CPU = query regression. Stable TPS + rising CPU = individual queries getting more expensive."
                },
                "Total Connections": {
                    title: "Total Connections",
                    text: "**What it is:** Total count of all connections regardless of state, compared against `max_connections`.\n\n**Thresholds:**\n- ✅ Healthy: < 70% of max_connections\n- ⚠️ Warning: 70–85%\n- 🔴 Critical: > 85% — exhaustion imminent\n\n**Critical note:** When `max_connections` is reached, new attempts fail immediately with `FATAL: sorry, too many clients already`. Use PgBouncer for high-concurrency workloads."
                },
                "Replica Lag": {
                    title: "Replica Lag",
                    text: "**What it is:** The maximum replication lag across all connected standby servers in bytes or seconds, from `pg_stat_replication`.\n\n**Thresholds:**\n- ✅ Healthy: < 10 seconds\n- ⚠️ Warning: 10–60 seconds\n- 🔴 Critical: > 60 seconds, or standby in `catchup` for > 5 minutes\n\n**Why it matters:** Lag determines data currency on standbys and the data loss window if the primary fails."
                },
                "Cache Hit Ratio": {
                    title: "Cache Hit Ratio (%)",
                    text: "**What it is:** Percentage of data block requests satisfied from PostgreSQL's `shared_buffers` cache without disk I/O.\n\n**Thresholds:**\n- ✅ Healthy: > 99%\n- ⚠️ Warning: 95–99%\n- 🔴 Critical: < 95%\n\n**Note:** PostgreSQL has a two-level cache — `shared_buffers` (managed by PostgreSQL) and the OS page cache. A 1% drop on a busy system can mean thousands of additional disk reads per second."
                },
                "XID Wraparound": {
                    title: "XID Wraparound Risk",
                    text: "**What it is:** Distance toward PostgreSQL's transaction ID (XID) hard limit of ~2.1 billion. Shown as a percentage toward the shutdown threshold.\n\n**Why it is uniquely catastrophic:** When a database's oldest unfrozen XID approaches 2 billion, PostgreSQL will **shut down the entire instance** to prevent data corruption. No users can connect — not even reads.\n\n**Thresholds:**\n- ✅ Healthy: < 200M age (autovacuum_freeze_max_age)\n- ⚠️ Warning: 200M–500M age\n- 🔴 Critical: > 1.5 billion age — run `VACUUM FREEZE` immediately"
                },
                "Dead Tuples": {
                    title: "Dead Tuple % (Bloat Indicator)",
                    text: "**What it is:** The highest percentage of dead tuples across all user tables, from `pg_stat_user_tables`.\n\n**Why it matters:** Dead tuples from PostgreSQL's MVCC model waste disk space and force sequential scans to read more data than necessary, burning CPU and I/O.\n\n**Thresholds:**\n- ✅ Healthy: < 5%\n- ⚠️ Warning: 5–20%\n- 🔴 Critical: > 20% on frequently scanned tables\n\n**Fix:** Manual `VACUUM table_name` or tune autovacuum thresholds."
                },
                "WAL Rate": {
                    title: "WAL Generation Rate (MB/min)",
                    text: "**What it is:** Rate at which Write-Ahead Log records are being generated, measured from `pg_current_wal_lsn()` deltas.\n\n**Why it matters:** WAL rate directly measures write workload intensity. High rates increase archiving pressure, replication bandwidth, and checkpoint I/O.\n\n**Signal:** A sudden 3–5× spike without a corresponding TPS increase often indicates a runaway UPDATE/DELETE loop or uncontrolled bulk load."
                },
                "Deadlocks": {
                    title: "Deadlocks / min",
                    text: "**What it is:** Rate of deadlocks detected and resolved by PostgreSQL, from `pg_stat_database.deadlocks`.\n\n**Why it matters:** When PostgreSQL detects a deadlock cycle, it aborts one transaction (returning an error to the application). A non-zero deadlock rate indicates application-level concurrency design problems.\n\n**Common patterns:** UPDATE order reversal, FK + parent update conflicts, concurrent UPSERT on the same key.\n\n**Fix:** Enable `log_lock_waits = on` and set `deadlock_timeout` low for diagnosis."
                },
                "Database Load": {
                    title: "Database Load (AAS 4-Way Chart)",
                    text: "**What it is:** An area chart showing active session states over time — Active, Waiting, Idle in Transaction, and Background Workers.\n\n**Why it matters:** The ratio between active (running) and waiting (stalled) sessions tells the story: a healthy server has a thin 'waiting' band. As a bottleneck develops, the waiting band grows — this widening gap pattern is the visual signature of an emerging crisis.\n\n**'Hide BG Workers' toggle:** Background workers (autovacuum, WAL sender, etc.) inflate the active count; hide them to see only client query activity."
                },
                "Incident Feed": {
                    title: "Unified Incident Feed",
                    text: "**What it is:** A real-time table of active incidents: blocking chains, long-running queries (elapsed > threshold), idle-in-transaction sessions, replication lag spikes, autovacuum blockers, and lock queue depth.\n\n**Why it matters:** Consolidates signals from multiple dashboards into one actionable list. A DBA can review this table and know immediately what requires attention without navigating across dashboards.\n\n**Source:** TimescaleDB-backed alert history covering the last 24 hours."
                },
                "Replication Detail": {
                    title: "Streaming Replication Detail",
                    text: "**What it is:** Per-standby status from `pg_stat_replication` showing write, flush, and replay lag separately for each connected standby.\n\n**Key lag types:**\n- **Write lag:** WAL generation → standby receipt (network latency)\n- **Flush lag:** WAL receipt → durably written to standby disk\n- **Replay lag:** WAL flushed → applied to standby database (determines actual data currency)\n\n**Sync state `sync`:** The primary waits for this standby before committing. Losing it may pause all writes."
                },
                "Session Distribution": {
                    title: "Session State Distribution",
                    text: "**What it is:** A snapshot breakdown of all connections by `state` in `pg_stat_activity`: Active, Idle, Idle in Transaction, and Idle in Transaction (Aborted).\n\n**Why it matters:** The distribution reveals connection pool hygiene. A dangerous pattern: many connections 'Idle in Transaction' — these have opened a transaction and forgotten to close it.\n\n**Fix:** Set `idle_in_transaction_session_timeout` (e.g., `'5min'`) to automatically terminate stale idle-in-transaction sessions."
                },
                "Wait Categories": {
                    title: "Wait Categories Chart",
                    text: "**What it is:** Distribution of current wait events grouped by `wait_event_type` in `pg_stat_activity`.\n\n**Why it matters:** Wait event analysis is the PostgreSQL equivalent of SQL Server wait statistics — the most reliable way to identify *what* the database is waiting for.\n\n**Most common:**\n- `ClientRead`: Waiting for application to send query\n- `DataFileRead`: Cache miss — reading from disk\n- `relation` lock: DDL or explicit LOCK TABLE\n- `transactionid`: Row-level write-write conflict\n- `WALWrite`: Waiting for WAL I/O"
                }
            }
        },
        "Enterprise Monitor": {
            description: "Monitors the internal 'engine room' of PostgreSQL — the background processes that maintain durability, consistency, and I/O efficiency. These processes (BGWriter, Checkpointer, WAL Archiver) are invisible to applications but directly determine overall database stability and write performance.",
            metrics: {
                "Cache Hit Ratio": {
                    title: "Shared Buffer Cache Hit Ratio (%)",
                    text: "**What it is:** Percentage of block reads satisfied from PostgreSQL's shared buffer cache vs. those requiring physical I/O.\n\n**Thresholds:**\n- ✅ Healthy: > 99%\n- ⚠️ Warning: 95–99%\n- 🔴 Critical: < 95%\n\n**Why it matters:** Values below 95% indicate `shared_buffers` is too small for the working set, or full-table scans are evicting useful pages."
                },
                "Checkpoint Ratio": {
                    title: "Timed vs. Requested Checkpoints",
                    text: "**What it is:** From `pg_stat_bgwriter`: timed checkpoints occur when `checkpoint_timeout` expires (normal). Requested checkpoints occur when dirty WAL exceeds `max_wal_size` (emergency — I/O spike risk).\n\n**Thresholds:**\n- ✅ Healthy: > 90% timed\n- ⚠️ Warning: Requested > 10% of total\n- 🔴 Critical: Requested > 25% — `max_wal_size` is too small\n\n**Fix:** Increase `max_wal_size` or `checkpoint_completion_target`."
                },
                "BGWriter Halts": {
                    title: "BGWriter Halts (maxwritten_clean)",
                    text: "**What it is:** Count of times the BGWriter stopped its background cleaning sweep because it hit the `bgwriter_lru_maxpages` limit in a single round.\n\n**Why it matters:** When the BGWriter is 'maxed out', backend processes must write their own dirty pages to disk — adding latency to individual queries.\n\n**Fix:** Increase `bgwriter_lru_maxpages` (default: 100) to allow more pages per round, or adjust `bgwriter_delay`."
                },
                "Archive Success": {
                    title: "WAL Archive Success / Failures",
                    text: "**What it is:** From `pg_stat_archiver`: count of WAL files successfully archived vs. those that failed.\n\n**Why archive failures are critical:** When `archive_mode = on` and `archive_command` fails, PostgreSQL keeps retrying — the `pg_wal/` directory grows without bound. When that filesystem fills, PostgreSQL **crashes with a PANIC**.\n\n**Thresholds:** Any failure is ⚠️ Warning. Failures persisting > 5 minutes, or `pg_wal/` growing continuously, is 🔴 Critical."
                },
                "BGWriter Stats": {
                    title: "BGWriter / Checkpoint Statistics",
                    text: "**`buffers_clean` vs. `buffers_backend`:** `buffers_clean` = pages written proactively by the BGWriter. `buffers_backend` = pages written directly by a backend because no clean buffer was available. A healthy system has `buffers_clean` >> `buffers_backend`.\n\n**`checkpoint_write_time` vs. `checkpoint_sync_time`:** Write time = writing dirty pages to the OS (async). Sync time = issuing `fsync()` calls to force durability. Long sync time indicates slow storage or virtualization write-back caching issues."
                },
                "WAL Archiver": {
                    title: "WAL Archiver Statistics",
                    text: "**`last_archived_wal` and `last_archived_time`:** The name and timestamp of the most recently successfully archived WAL file.\n\n**Why it matters:** The gap between `now()` and `last_archived_time` is the actual archiving lag — how far behind the archive is relative to the current WAL position. For point-in-time recovery (PITR), the archive must be current.\n\n**Example:** If `last_archived_time` is 30 minutes old on a busy server, your PITR capability is 30 minutes stale."
                },
                "Config Drift": {
                    title: "Config Drift (Audit)",
                    text: "**What it is:** A comparison between currently effective configuration values (`pg_settings.setting`) and on-disk values. Highlights settings where `pending_restart = true` or where `ALTER SYSTEM SET` was used without updating documentation.\n\n**Why it matters:** Configuration drift means a temporary change (made for a specific incident) may be silently reverted on the next service restart, or an unreviewed change may have taken effect. In production environments, all config changes should be deliberate and tracked."
                },
                "Contention Waits": {
                    title: "Contention Wait Analysis",
                    text: "**What it is:** Time-series chart of session counts by wait event type over the collection period.\n\n**Why it matters:** Persistent wait event patterns reveal systemic bottlenecks. A growing `Lock` band indicates lock contention. A growing `IO` band means storage can't keep up. A growing `LWLock` band may indicate `shared_buffers` contention.\n\n**Key events:** `ClientRead` (network round-trips), `DataFileRead` (disk reads), `relation` (table locks), `WALWrite` (WAL I/O bottleneck)."
                },
                "I/O & Temp Spill": {
                    title: "Database I/O & Temp Spill Monitoring",
                    text: "**What it is:** From `pg_stat_database`: `temp_files` (count of temporary files created) and `temp_bytes` (total size) for operations that could not fit in `work_mem`.\n\n**Why it matters:** Temp file creation is the PostgreSQL equivalent of memory grant spills in SQL Server. Disk-based sorts are 10–100× slower than in-memory sorts.\n\n**Thresholds:**\n- ✅ Healthy: 0 temp files for OLTP queries\n- ⚠️ Warning: Regular temp files for OLTP\n- 🔴 Critical: `temp_bytes` growing continuously\n\n**Note:** `work_mem` is allocated per sort/hash operation, per query, per connection — not per connection."
                },
                "High-Impact Queries": {
                    title: "High-Impact Internal Queries",
                    text: "**What it is:** Top queries from `pg_stat_statements` ranked by total execution time, showing normalized query text, call count, mean execution time, and resource usage.\n\n**Key columns:**\n- **Total Time:** Cumulative wall-clock time — the true total cost across all executions\n- **Calls:** Execution frequency — a cheap query called 10M times may outrank an expensive one called 100 times\n- **Avg ms:** Average per-execution latency — high mean signals slow individual executions\n- **Cache %:** Buffer hit ratio per query — low values identify cache-miss culprits"
                }
            }
        },
        "CPU Health": {
            description: "Tracks CPU utilization from both the host OS and the PostgreSQL process perspective, identifying which databases and queries are driving CPU consumption. CPU saturation in PostgreSQL is almost always caused by query-level issues — missing indexes, sort spills, or high-cardinality hash joins.",
            metrics: {
                "Postgres CPU %": {
                    title: "Postgres CPU %",
                    text: "**What it is:** CPU utilization attributed to PostgreSQL backend processes, from OS-level process stats.\n\n**Thresholds:**\n- ✅ Healthy: < 60% sustained\n- ⚠️ Warning: 60–80%\n- 🔴 Critical: > 80% sustained\n\n**Why it matters:** High CPU is almost always caused by query-level issues — sequential scans from missing indexes, sort operations spilling to disk, or expensive hash joins. A single query can fan out to `max_parallel_workers_per_gather` parallel workers."
                },
                "CPU per Connection": {
                    title: "CPU per Connection",
                    text: "**What it is:** Total CPU% divided by active session count — average CPU consumption per executing query.\n\n**Why it matters:** A server at 80% CPU with 80 active connections (1%/connection) handles 80 lightweight queries. A server at 80% CPU with 2 connections (40%/connection) has one or two massively expensive queries destroying the server. The correct intervention (kill one query vs. scale out) depends on this ratio."
                },
                "Avg Query Latency": {
                    title: "Average Query Latency (ms)",
                    text: "**What it is:** Mean execution time across all queries from `pg_stat_statements.mean_exec_time`, representing the average time a query takes to complete.\n\n**Why it matters:** Average latency is a leading indicator of query regression. A sudden increase means either a specific query got much slower (plan change, bloat, missing index) or the overall load is degrading response times.\n\n**Tip:** Use p95/p99 latency on the Query Performance dashboard for a more accurate picture of user-facing latency."
                },
                "Cache Hit %": {
                    title: "Buffer Cache Hit Ratio (%)",
                    text: "**What it is:** Percentage of data block requests served from `shared_buffers` without disk I/O, from `pg_statio_user_tables`.\n\n**Thresholds:**\n- ✅ Healthy: > 99%\n- ⚠️ Warning: 95–99%\n- 🔴 Critical: < 95%\n\n**On the CPU dashboard:** Low cache hit ratio means the CPU is waiting on disk I/O, not just computing — both CPU and I/O are being wasted. Increasing `shared_buffers` (to 25–40% of RAM) is the primary fix."
                },
                "Execution Load": {
                    title: "PostgreSQL Execution Load Chart",
                    text: "**What it is:** Total query execution time per time bucket from `pg_stat_statements` — a proxy for PostgreSQL CPU load. The right axis shows average query latency.\n\n**Why it matters:** Unlike raw CPU%, this chart shows *database work done*. Spikes in execution load without corresponding active session increases mean individual queries are getting more expensive (plan regression, bloat, lock waits adding to elapsed time).\n\n**Bucket size:** Determined by the selected time range (e.g., 5-minute buckets for a 6-hour view)."
                },
                "Database Share": {
                    title: "Database CPU Load Share (Donut)",
                    text: "**What it is:** CPU consumption broken down per database, based on query-level metrics from `pg_stat_statements` attributed to each `dbid`.\n\n**Why it matters:** On a multi-database PostgreSQL instance (common in SaaS environments), one tenant database can monopolize CPU, impacting all others. This chart immediately identifies the offending database, enabling targeted workload isolation decisions."
                },
                "Query Type": {
                    title: "Query Type Breakdown",
                    text: "**What it is:** Breakdown of execution time by query type (SELECT, INSERT, UPDATE, DELETE, DDL, Other) from `pg_stat_statements`.\n\n**Why it matters:** Identifies the write/read balance driving CPU load. Unexpected dominance by UPDATE/DELETE often indicates a background job or a runaway retry loop. Heavy DDL presence in a production database is a red flag."
                },
                "Top CPU Queries": {
                    title: "Top CPU-Consuming User Queries",
                    text: "**What it is:** Ranked list of user queries from `pg_stat_statements` sorted by total execution time, with monitoring/system queries excluded.\n\n**Key columns:**\n- **Total Exec (ms):** Cumulative time — the true total CPU cost\n- **Calls:** Execution frequency\n- **Avg (ms):** Per-execution latency\n\n**Optimization principle:** The top 5–10 queries typically account for 60–80% of total CPU. Use `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` on the query text to get the actual execution plan."
                }
            }
        },
        "Memory Intelligence": {
            description: "Deep visibility into RAM usage, shared_buffers cache efficiency, work_mem spill behavior, and predictive memory analytics. Memory misconfiguration is the most common reason a well-designed PostgreSQL database performs poorly. PostgreSQL's process-per-connection model means memory management is a balance between shared_buffers and the sum of work_mem used by active connections.",
            metrics: {
                "OS Used %": {
                    title: "OS Used % (Host Memory)",
                    text: "**What it is:** Percentage of total physical RAM in use by all processes on the host.\n\n**Thresholds:**\n- ✅ Healthy: < 80%\n- ⚠️ Warning: 80–90%\n- 🔴 Critical: > 90%, or any swap activity\n\n**Why it matters:** PostgreSQL relies on the OS page cache as a second-tier buffer. If OS memory is exhausted, the kernel evicts page cache entries — effectively shrinking PostgreSQL's total available cache — and may start swapping."
                },
                "Postgres RSS": {
                    title: "Postgres RSS (Resident Set Size)",
                    text: "**What it is:** The actual physical RAM currently allocated to all PostgreSQL processes combined (postmaster + backends + background workers).\n\n**Memory budget formula:**\n```\nTotal ≈ shared_buffers\n      + (max_connections × 5–10 MB)\n      + (active_connections × work_mem × avg_sort_nodes)\n      + autovacuum workers × maintenance_work_mem\n```\n\n**Why it matters:** Unexpectedly large RSS indicates too many connections, `work_mem` too high, or a memory leak in an extension."
                },
                "Cache Hit Ratio": {
                    title: "Cache Hit Ratio (%)",
                    text: "**What it is:** Percentage of data block requests satisfied from PostgreSQL's `shared_buffers` without disk I/O. On the Memory dashboard, the trend is shown over time.\n\n**Critical insight:** A cache hit ratio declining by 0.5% per week is easy to miss in daily monitoring but represents a serious trend. A drop from 99.5% to 97% means disk I/O for data access has increased by **5×**.\n\n**Fix:** Increase `shared_buffers` (25–40% of RAM). Example: on a 64GB server, set `shared_buffers = 16GB`."
                },
                "Swap Activity": {
                    title: "Swap Activity",
                    text: "**What it is:** Rate of memory pages being moved to or from disk swap space at the OS level.\n\n**Thresholds:**\n- ✅ Healthy: 0 swap I/O\n- 🔴 Critical: Any sustained swap activity — immediate action required\n\n**Why it matters:** Swap is virtual memory backed by disk. When PostgreSQL's memory is swapped out and then accessed, read latency spikes from nanoseconds (RAM) to milliseconds (disk) — a factor of **1,000,000×**. Even 1MB/s of sustained swap I/O causes severe query latency."
                },
                "Temp Spill Rate": {
                    title: "Temp Spill Rate",
                    text: "**What it is:** Rate of temporary file creation per minute, from `pg_stat_database.temp_files` delta. Temp files are created when sort operations, hash joins, or hash aggregates exceed their `work_mem` allocation.\n\n**Why it matters:** Disk-based sorts are 10–100× slower than in-memory sorts. High spill rates signal `work_mem` is undersized for the query patterns.\n\n**Note:** `work_mem` is allocated *per sort/hash operation, per query, per connection*. Setting it too high risks OOM kills. Raise it session-locally for heavy queries: `SET work_mem = '256MB';`"
                },
                "Memory Load History": {
                    title: "Memory Load History Chart",
                    text: "**What it is:** Dual-line chart showing Host RAM usage (total OS) and PostgreSQL RSS over time.\n\n**Why it matters:** The gap between total host RAM and PostgreSQL RSS is the 'available OS page cache budget.' When PostgreSQL RSS + other processes consume > 90% of host RAM, the OS page cache shrinks, increasing physical I/O even for data that was previously cached at the OS level.\n\n**Pattern to watch:** Steadily growing PostgreSQL RSS without a corresponding TPS increase may indicate a connection pool growing over time or per-query `work_mem` grants accumulating."
                },
                "Cache Efficiency Trend": {
                    title: "Cache Hit Ratio Trend",
                    text: "**What it is:** Historical trend of `shared_buffers` hit ratio over time, enabling detection of gradual working-set growth.\n\n**Why it matters:** Applications rarely shrink — they grow. A cache hit ratio that was 99.8% three months ago and is now 98.5% tells a clear story: the working dataset has grown beyond `shared_buffers` capacity. This chart provides the evidence needed to justify a `shared_buffers` increase or a hardware upgrade."
                },
                "Connection Memory": {
                    title: "Connection Trend",
                    text: "**What it is:** Time-series chart of active, idle, and total connection counts over the collection period.\n\n**Why it matters:** Each PostgreSQL connection is a separate OS process consuming ~5–10MB of memory for process overhead alone, regardless of whether it's doing any work. A growing idle connection count without a corresponding increase in active sessions indicates connection pool over-provisioning.\n\n**Recommendation:** Use PgBouncer. Set `max_connections = 100–200` and accept thousands of application connections through the pooler."
                },
                "BGWriter Allocation": {
                    title: "BGWriter Buffer Allocation",
                    text: "**What it is:** Time-series chart of buffer writes split by source: `buffers_clean` (BGWriter proactive writes) vs. `buffers_backend` (backend forced writes) vs. `buffers_checkpoint` (checkpoint writes), from `pg_stat_bgwriter`.\n\n**Why it matters:** A healthy system has `buffers_clean` >> `buffers_backend`. If `buffers_backend` is growing rapidly, the BGWriter is not keeping pace with dirty page creation, and query latency is being directly impacted by synchronous I/O embedded within query execution."
                },
                "Memory Advisor": {
                    title: "Memory Advisor (Rule-Based Insights)",
                    text: "**Buffer Management:** Reviews `shared_buffers` size relative to working dataset and cache hit ratio trends. Recommends increases when the hit ratio is declining.\n\n**Work Memory (Sorting):** Reviews temp file creation rates and mean query duration to assess whether `work_mem` is causing sort/hash spills.\n\n**Connection Overhead:** Estimates total memory consumed by connection process overhead. High connection overhead with low active session counts is a sign to reduce `max_connections` and deploy PgBouncer.\n\n**Guideline:** Start `work_mem` conservatively at 4–8MB and raise session-locally for known heavy analytical queries."
                },
                "Saturation Forecast": {
                    title: "Saturation Forecast",
                    text: "**What it is:** A predictive model projecting when RAM will be exhausted based on current RSS growth trend.\n\n**Why it matters:** Memory exhaustion in PostgreSQL leads to OOM kills — the OS kills processes to free memory, which can crash the postmaster itself, requiring a full instance restart.\n\n**How to use:** If the forecast shows exhaustion in < 30 days, plan a `shared_buffers` reduction, lower `max_connections`, reduce `work_mem`, or add physical RAM. A forecast gives lead time to scale before a crisis."
                }
            }
        },
        "Waits & Sessions": {
            description: "A real-time, comprehensive view of all database connections, their current state, and the events they are waiting on. The 'operating table' for live session triage. Uses pg_stat_activity data to show exactly what every connection is doing right now.",
            metrics: {
                "Longest Active": {
                    title: "Longest Active Session",
                    text: "**What it is:** Elapsed time and PID of the longest-running currently executing query, from `pg_stat_activity` where `state = 'active'`.\n\n**Why it matters:** A single long-running query can cascade into a crisis — it holds locks blocking others, prevents autovacuum from cleaning tables it has open, and consumes a worker process and potentially `work_mem` for the duration.\n\n**Recommended config:** `statement_timeout = '30min'` as a safety net. For OLTP APIs: `ALTER ROLE api_user SET statement_timeout = '5s'`."
                },
                "Longest Idle Txn": {
                    title: "Longest Idle-in-Transaction",
                    text: "**What it is:** Elapsed time since the last query for the session with the oldest open transaction in `idle in transaction` state.\n\n**Why this is the most dangerous metric:** An idle-in-transaction session holding row locks prevents autovacuum from cleaning dead tuples on any touched tables, and blocks DDL that needs exclusive locks. In high-write environments, 10 minutes of blocked autovacuum can cause significant table bloat.\n\n**Fix:** `idle_in_transaction_session_timeout = '5min'` — automatically terminates these sessions."
                },
                "Session State Trend": {
                    title: "Session State Trend Chart",
                    text: "**What it is:** A stacked time-series chart showing the count of sessions per state over time — Active, Idle, Idle in Transaction, and others.\n\n**Why it matters:** Trends reveal patterns: idle-in-transaction sessions spiking at the same time daily (a batch job that doesn't commit promptly), active sessions growing slowly over weeks (approaching connection limits), or blocked sessions appearing suddenly (a lock contention incident beginning)."
                },
                "Idle-in-Txn Trend": {
                    title: "Idle-in-Transaction Trend",
                    text: "**What it is:** A dedicated time-series chart tracking only the count and duration of idle-in-transaction sessions.\n\n**Why it matters:** Because idle-in-transaction is such a uniquely harmful state in PostgreSQL (lock retention + autovacuum blocking), it deserves its own trend. A sustained non-zero count is a sign that the application connection handling code has a bug — e.g., a `BEGIN` without a corresponding `COMMIT/ROLLBACK` in an error handling path."
                },
                "Live Sessions": {
                    title: "Live Sessions Table",
                    text: "**What it is:** Real-time grid from `pg_stat_activity` showing for each connection: PID, database, user, application name, client address, state, duration, wait event, blocking PID, and query text.\n\n**Key diagnostic columns:**\n- **wait_event_type + wait_event:** Precisely identifies what the session is waiting for\n- **pg_blocking_pids(pid):** If populated, this session is a lock victim — follow the chain to the root blocker\n- **state_change:** When the current state began — a session in `idle in transaction` with state_change 20 minutes ago is a high-priority investigation target\n- **application_name:** Attributes sessions to specific application services"
                }
            }
        },
        "Locks & Blocking": {
            description: "Specialized tool for troubleshooting database contention, identifying blocking hierarchies, and analyzing deadlock patterns. PostgreSQL's MVCC means blocking is less common than in SQL Server, which makes any blocking event more significant. This dashboard helps identify the 'Idle in Transaction' pattern where a session holds locks indefinitely.",
            metrics: {
                "Active Blocking Sessions": {
                    title: "Active Blocking Sessions (Victims)",
                    text: "**What it is:** Count of sessions currently blocking at least one other session by holding a lock that another backend is waiting for.\n\n**Why it matters:** Blocking in PostgreSQL often signals a design issue. Persistent blocking leads to application timeouts and connection pool exhaustion.\n\n**PostgreSQL DDL blocking pattern:** `ALTER TABLE` requires an `AccessExclusiveLock`, which conflicts with every other lock type including SELECT. A long-running SELECT can block an `ALTER TABLE`, which in turn blocks every subsequent query on that table."
                },
                "Root Blocker PID": {
                    title: "Root Blocker PID",
                    text: "**What it is:** The PID at the root of the deepest or largest blocking chain — the single session that, if terminated, would free the most blocked sessions.\n\n**Why it matters:** In a multi-level blocking chain (A blocks B, B blocks C), killing B is futile — A immediately blocks whatever B's role was. Only terminating A releases the entire chain.\n\n**To terminate:** `SELECT pg_terminate_backend(pid);` — use with caution in production. Identify the root cause (long transaction, idle-in-transaction) before killing."
                },
                "Idle-in-Transaction Risk": {
                    title: "Idle-in-Transaction Risk",
                    text: "**What it is:** Count and severity of idle-in-transaction sessions that are currently holding locks, weighted by the activity level of the locked objects.\n\n**Why it matters:** An idle-in-transaction session locking a high-write table (e.g., an `orders` table) is far more dangerous than one locking a low-activity reference table.\n\n**Prevention:** Set `idle_in_transaction_session_timeout = '5min'` in `postgresql.conf` to automatically terminate these sessions."
                },
                "Blocking Tree": {
                    title: "Blocking Tree Visualizer",
                    text: "**What it is:** A hierarchical diagram showing the lock dependency graph: root blockers at the top, with victim sessions branching below them.\n\n**Why it matters:** Makes the 'head of the snake' immediately obvious in a complex blocking scenario. In PostgreSQL, the tree is typically shallow (1–2 levels) due to MVCC's write-write isolation, but DDL operations requiring `AccessExclusiveLock` can create deep chains.\n\n**To read the tree:** Start from the top. The session with no blockers above it is the root — killing it releases all sessions below."
                },
                "Incident Timeline": {
                    title: "Incident Timeline / History",
                    text: "**What it is:** A bar chart and history of blocking events detected over the last hour, showing start time, end time, duration, root blocker PID, victim count, and the SQL of the blocking query.\n\n**Why it matters:** Many blocking incidents resolve themselves (the blocking transaction commits), so by the time a DBA investigates, the active blocking is gone. The incident timeline preserves the forensic evidence — what happened, when, for how long, and which query caused it."
                },
                "Lock Wait Trend": {
                    title: "Lock Wait Trend",
                    text: "**What it is:** Time-series chart of lock wait counts over the observation period, showing whether contention is increasing, decreasing, or episodic.\n\n**Why it matters:** A growing lock wait trend over hours indicates a systemic worsening (a long-running transaction accumulating more victims). A flat or declining trend indicates a transient incident that has self-resolved."
                },
                "Top Locked Tables": {
                    title: "Top Locked Tables",
                    text: "**What it is:** Relations (tables and indexes) that are the most frequent subjects of lock waits, ranked by lock wait count and total wait duration.\n\n**Why it matters:** Structural identification of problem tables. If 80% of lock waits are on the `orders` table, its access patterns need redesign.\n\n**DBA action for hot tables:**\n1. Identify the dominant lock type (row-level vs. table-level)\n2. For row-level: Can writes be spread across more rows? Can UPDATE be replaced with INSERT?\n3. For table-level: Is there unnecessary DDL? Use `CREATE INDEX CONCURRENTLY`"
                },
                "Lock Wait Distribution": {
                    title: "Lock Type Distribution",
                    text: "**Key lock types:**\n- **`relation`:** Table-level lock from DDL or `LOCK TABLE` — most disruptive\n- **`transactionid`:** Waiting for a transaction to commit/rollback — normal in high-concurrency writes\n- **`tuple`:** Multiple transactions updating the same row simultaneously — hot-row contention\n- **`extend`:** Waiting to add a new page to a table — common for high-insert workloads\n- **`advisory`:** Application-managed advisory locks — contention indicates distributed locking bottleneck"
                }
            }
        },
        "Query Performance": {
            description: "Aggregated performance statistics for all queries executed on the instance, powered by pg_stat_statements. The primary tool for query-level performance analysis and regression detection. Prerequisite: pg_stat_statements extension must be installed and shared_preload_libraries includes 'pg_stat_statements'.",
            metrics: {
                "QPS": {
                    title: "QPS (Queries Per Second)",
                    text: "**What it is:** Total number of query executions per second from `pg_stat_statements` total `calls` delta. Includes SELECT, INSERT, UPDATE, DELETE, and utility commands.\n\n**Why it matters:** QPS is the throughput metric. Compare QPS trends to query load trends: if QPS is stable but load is rising, individual queries are getting more expensive. If both are rising proportionally, it's an organic traffic increase."
                },
                "Latency Percentiles": {
                    title: "Latency Percentiles (p50, p95, p99)",
                    text: "**What it is:** Per-query latency distribution percentiles across all statements.\n\n**Why p95 and p99 matter:** The mean latency is heavily influenced by fast queries and hides tail latency. If p99 is 10× p50, 1% of queries are experiencing severe slowdowns.\n\n**PostgreSQL note (PG 14+):** `pg_stat_statements` captures `mean_exec_time` and `stddev_exec_time` separately. High stddev relative to mean indicates high variance — unpredictable performance."
                },
                "Query Load": {
                    title: "Query Load (ms/sec)",
                    text: "**What it is:** Total query execution time per second — the area under the 'how busy is the database' curve. Calculated as sum of `pg_stat_statements.total_exec_time` delta divided by collection interval.\n\n**Why it matters:** Unlike QPS (which counts queries regardless of cost), query load captures the actual work being done. A server running 100 QPS of 1ms queries (100ms/s load) is very differently loaded than one running 100 QPS of 50ms queries (5,000ms/s load)."
                },
                "Top Queries Table": {
                    title: "Top Queries Table",
                    text: "**What it is:** Sortable grid of queries from `pg_stat_statements`, showing normalized query text and per-query metrics.\n\n**Sort by:**\n- **Total Time:** Greatest cumulative impact — fix these for maximum overall improvement\n- **Mean Time:** Slowest individual executions — fix these for worst user-experience impact\n- **Calls:** Most frequent — a 1ms optimization on 10M calls/day saves 10,000 CPU-seconds\n- **Temp MB:** Most sort spills — candidates for `work_mem` increase or query optimization\n- **WAL MB:** Most write-intensive — driving replication lag and checkpoint pressure"
                },
                "Query Regressions": {
                    title: "Query Regressions Table",
                    text: "**What it is:** Comparison of query performance in the last 30 minutes against the preceding period, flagging queries where `mean_exec_time` has increased beyond a threshold (e.g., > 2× slower).\n\n**Common PostgreSQL regression causes:**\n- **Plan change after ANALYZE:** Updated statistics changed the optimizer's join order or scan method\n- **Bloat crossing a threshold:** Dead tuple % crossed a point where sequential scans became dramatically more expensive\n- **Statistics target too low:** Default (100) insufficient for highly skewed columns — increase with `ALTER TABLE t ALTER COLUMN c SET STATISTICS 500`"
                },
                "WAL Generation": {
                    title: "WAL Bytes/sec",
                    text: "**What it is:** Rate of Write-Ahead Log generation attributed to query workload, from `pg_stat_statements.wal_bytes` delta.\n\n**Why it matters:** High WAL generation from specific queries increases replication lag and checkpoint pressure. A query generating disproportionate WAL is often doing large batch UPDATEs or running HOT updates inefficiently.\n\n**Tip:** Queries with very high WAL/call ratio relative to rows affected may benefit from `wal_compression = on` or batching smaller transactions."
                }
            }
        },
        "EXPLAIN Plan Analyzer": {
            description: "Visualizes and analyzes PostgreSQL execution plans to identify inefficient operators, missing indexes, and cardinality estimation errors. By pasting a JSON-formatted plan (generated via `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`), users can see a Sankey-style tree showing the flow of data and a heuristic-driven optimization report that flags 'plan smells' like sequential scans on large tables or expensive nested loops.",
            metrics: {
                "Plan Map": {
                    title: "Plan Map (Sankey-Style Tree)",
                    text: "**What it is:** A visual flow diagram mapping the execution plan's node tree, showing rows flowing from leaf nodes (table scans) through intermediate operators (joins, sorts, aggregates) to the final result.\n\n**Key plan node types:**\n- **Seq Scan:** No usable index — expensive on large tables with selective filtering\n- **Index Only Scan:** All needed columns in the index — best for read-heavy queries\n- **Hash Join:** Inner table fits in `work_mem` — check `Batches > 1` for disk spill\n- **Nested Loop:** Catastrophic if outer set is large and index lookup is expensive\n- **Sort with 'external merge':** Sort is spilling to disk — needs more `work_mem` or index-based ordering"
                },
                "Optimization Report": {
                    title: "Optimization Report",
                    text: "**What it is:** A rule-based analysis that flags common plan inefficiencies such as sequential scans on large tables, excessive disk spilling, or large row-count discrepancies (estimate vs. actual).\n\n**Interpreting row estimation errors:**\n- Error < 10×: Acceptable\n- Error 10–100×: Investigate — statistics may be stale\n- Error > 100×: Critical — likely missing extended statistics for correlated column predicates: `CREATE STATISTICS s ON (col_a, col_b) FROM table`"
                }
            }
        },
        "Storage & Maintenance": {
            description: "Monitors physical database footprint, table and index bloat caused by PostgreSQL's MVCC model, and the efficiency of autovacuum. PostgreSQL creates a new row version for every UPDATE and DELETE — old versions (dead tuples) remain physically in the table until autovacuum reclaims them. This is fundamentally different from SQL Server's in-place update model.",
            metrics: {
                "7-Day Growth (%)": {
                    title: "7-Day Growth (%)",
                    text: "**What it is:** Percentage size change over 7 days from historical snapshots.\n\n**Key distinction:** If 7-day growth is > 5% but TPS and row insertion rates are normal, suspect bloat accumulation rather than real data growth.\n\n**Growth patterns:**\n- Linear: Healthy, predictable data accumulation\n- Accelerating (convex curve): Possible bloat accumulation — check dead tuple % alongside size growth\n- Sudden step increase: Large data import or restored from backup"
                },
                "Worst Dead Tuple %": {
                    title: "Worst Dead Tuple %",
                    text: "**What it is:** The highest `n_dead_tup / (n_live_tup + n_dead_tup)` ratio across all user tables.\n\n**Thresholds:**\n- ✅ Healthy: < 5%\n- ⚠️ Warning: 5–20%\n- 🔴 Critical: > 20%, or any table with > 1M dead tuples that autovacuum hasn't cleaned in > 24 hours\n\n**Fix:** `VACUUM table_name` (concurrent), or `VACUUM ANALYZE table_name` (vacuum + update statistics). In production, prefer `pg_repack` over `VACUUM FULL` to avoid table locks."
                },
                "XID Wraparound Risk": {
                    title: "XID Wraparound Risk Visualizer",
                    text: "**This is one of the most critical monitoring elements in PostgreSQL.**\n\n**What it is:** How many transactions remain before each database reaches the XID wraparound limit. Source: `age(datfrozenxid)` from `pg_database`.\n\n**Why it is uniquely catastrophic:** PostgreSQL will **shut down the entire instance** when the oldest unfrozen XID age approaches 2 billion — no reads, no writes, no connections until `VACUUM FREEZE` is run.\n\n**Thresholds:**\n- ✅ < 200M (autovacuum_freeze_max_age)\n- ⚠️ > 500M — autovacuum freezing may be falling behind\n- 🔴 > 1.5 billion — emergency `VACUUM FREEZE` needed immediately"
                },
                "Unused Indexes": {
                    title: "Unused Index Count",
                    text: "**What it is:** Count of indexes with `idx_scan = 0` since the last statistics reset in `pg_stat_user_indexes`.\n\n**Why it matters:** In PostgreSQL, indexes must be maintained on every INSERT, UPDATE, and DELETE. An unused index provides zero query benefit while burning write I/O and storage.\n\n**Important caveat:** `pg_stat_user_indexes` resets on service restart. Verify server uptime before dropping: `SELECT now() - pg_postmaster_start_time();`. Use `DROP INDEX CONCURRENTLY` in production."
                },
                "DB Size": {
                    title: "Total Database Size",
                    text: "**What it is:** Physical size of all database files including tables, indexes, TOAST data, and free space map files. From `pg_database_size(current_database())`.\n\n**TOAST (The Oversized-Attribute Storage Technique):** When a row value exceeds ~2KB, PostgreSQL automatically moves it to a separate TOAST table. Large text fields, JSONB columns, and bytea columns commonly use TOAST. Tables with very large `pg_toast_*` sizes indicate TOAST-heavy schemas."
                },
                "90d Forecast": {
                    title: "90-Day Storage Forecast",
                    text: "**What it is:** Projected database size in 90 days based on the current daily growth rate, calculated from historical snapshots in TimescaleDB.\n\n**Why it matters:** Unlike SQL Server, PostgreSQL does not have auto-grow on separate volumes — it simply runs out of disk space, causing an immediate hard crash. A forecast gives DBAs weeks of lead time to plan storage expansion or data archival."
                },
                "Write Overhead": {
                    title: "Index Write Overhead %",
                    text: "**What it is:** Estimated percentage of write I/O consumed by index maintenance, calculated from the ratio of index writes to total writes.\n\n**Why it matters:** Every index on a table adds overhead to INSERT, UPDATE, and DELETE operations. A table with 10 indexes doing 100,000 writes/minute may be spending 50%+ of its write I/O on index maintenance. Dropping unused indexes directly reduces write overhead."
                },
                "Storage Growth": {
                    title: "Storage Growth History Chart",
                    text: "**What it is:** Historical line chart showing total database size and individual table sizes over time, stored as time-series snapshots in TimescaleDB.\n\n**Why it matters:** Point-in-time size measurements don't reveal trajectory. A 50GB table is not concerning if it has been stable at 50GB for 6 months. The same 50GB table growing at 1GB/day will exhaust storage in 50 days."
                },
                "Access Pattern": {
                    title: "Access Pattern — Index vs. Sequential Scans",
                    text: "**What it is:** For each table, the ratio of `idx_scan` (index-driven reads) to `seq_scan` (full table reads) from `pg_stat_user_tables`.\n\n**Why it matters:** A high `seq_scan` count on a large table is a strong signal of missing indexes. Every sequential scan reads the entire table — for a 100GB table, each scan reads 100GB from disk (or cache).\n\n**Nuance:** Not all sequential scans are bad. For small tables (< 1000 rows) or queries retrieving > 20% of rows, sequential scans are often faster."
                },
                "Largest Tables": {
                    title: "Largest Tables Diagnostic",
                    text: "**What it is:** Ranked list of tables by total size, showing breakdown between data (heap), indexes, and TOAST.\n\n**Key insight — Index %:** If a table's index size is > 100% of its data size, it is over-indexed. Each index adds write overhead. Review index usage on the Index Efficiency panel before any schema changes."
                },
                "Seq Scan Tables": {
                    title: "Seq Scan Dominant Tables",
                    text: "**What it is:** Tables where sequential scans significantly outnumber index scans, from `pg_stat_user_tables`.\n\n**DBA action:** For each high-seq-scan table:\n1. Run `EXPLAIN (ANALYZE, BUFFERS)` on the most common queries against this table\n2. Identify missing index opportunities\n3. Use `CREATE INDEX CONCURRENTLY` in production\n4. Consider partial indexes for common WHERE predicates to minimize index size"
                },
                "Index Efficiency": {
                    title: "Index Efficiency Table",
                    text: "**What it is:** For each index, `idx_scan` (reads) compared to write operations that must maintain the index, from `pg_stat_user_indexes`.\n\n**Why it matters:** An index with zero reads but thousands of writes per minute provides no benefit while consuming maintenance overhead.\n\n**Safe index removal:** Use `DROP INDEX CONCURRENTLY index_name` in production to avoid `AccessExclusiveLock`. Monitor query performance for 24–48 hours after dropping."
                }
            }
        },
        "Backup & DR": {
            description: "Monitors the safety, durability, and high-availability status of the instance — specifically WAL archiving health, streaming replication status, and replication slot risk. The archive is the foundation of Point-In-Time Recovery (PITR); a single missing WAL file makes the entire subsequent archive useless for recovery.",
            metrics: {
                "DR Readiness": {
                    title: "DR Readiness pillars",
                    text: "**What it is:** Four executive cards (recoverability, point-in-time, availability, WAL safety) scored green/amber/red against per-instance RPO/RTO targets configured in Admin → DR policy.\n\n**Why it matters:** Gives DBAs and business stakeholders a single glance at whether the instance can meet recovery and failover commitments."
                },
                "Base Backup": {
                    title: "Base backup runs",
                    text: "**What it is:** History of logical/physical backup jobs reported to SQL Optima (`postgres_backup_runs`), including tool, type, status, and size.\n\n**Why it matters:** WAL archiving alone does not replace base backups — both are required for full restore. Stale or missing runs trigger `PGBackupStale` alerts using your configured RPO backup hours."
                },
                "WAL Gen Rate": {
                    title: "WAL Generation Rate (MB/min)",
                    text: "**What it is:** Rate at which Write-Ahead Log records are being generated. On this dashboard, WAL rate is contextualized against archiving capacity.\n\n**Why it matters:** If WAL is being generated faster than the archive can consume it, the `pg_wal/` directory grows without bound. On a busy server, this can fill an entire disk in hours.\n\n**Signal:** A sudden 3–5× spike in WAL rate without a corresponding TPS increase often indicates a runaway UPDATE/DELETE loop or uncontrolled bulk load."
                },
                "Archive Success": {
                    title: "Archive Success %",
                    text: "**What it is:** `archived_count / (archived_count + failed_count) × 100` from `pg_stat_archiver`.\n\n**Thresholds:**\n- ✅ Healthy: 100%\n- ⚠️ Warning: Any value below 100% — investigate immediately\n- 🔴 Critical: < 99% or any sustained failure > 5 minutes\n\n**Why it matters:** A 95% success rate means 5% of WAL files failed. In a high-write environment generating 100 WAL files/hour, that's 5 unarchived files/hour — gaps in the recovery timeline that PITR cannot bridge."
                },
                "Max Repl Lag": {
                    title: "Max Replica Lag",
                    text: "**What it is:** The highest lag (in seconds or bytes) among all connected standby servers.\n\n**Understanding lag types:**\n- **Write lag:** WAL generation on primary → receipt by standby (network latency)\n- **Flush lag:** WAL receipt → durable write to standby disk\n- **Replay lag:** WAL flushed → applied to standby database\n\nFor failover readiness, **replay lag** is the metric that determines actual data currency on the standby."
                },
                "Replication Slots Risk": {
                    title: "Replication Slots Risk",
                    text: "**This is one of the most operationally dangerous metrics in PostgreSQL monitoring.**\n\n**What it is:** A risk assessment of all replication slots (`pg_replication_slots`), monitoring slots where the consumer has fallen behind.\n\n**Why it can crash your server:** A replication slot prevents WAL deletion. If a consumer disconnects and never reconnects, the `pg_wal/` directory grows without bound until the disk fills, crashing the entire database.\n\n**Thresholds:**\n- ✅ All slots active and retaining < 1GB\n- ⚠️ Any inactive slot or retaining > 1GB\n- 🔴 Any slot retaining > 10GB\n\n**Prevention:** Set `max_slot_wal_keep_size = '10GB'` (PostgreSQL 13+)."
                },
                "Last Archive Age": {
                    title: "Last Archive Age",
                    text: "**What it is:** Time elapsed since the most recently successfully archived WAL file, from `pg_stat_archiver.last_archived_time`.\n\n**Why it matters:** This is the practical measure of your PITR gap. If the last archive was 30 minutes ago and the primary fails now, you can only recover to a point 30 minutes in the past.\n\n**Healthy target:** < 5 minutes on a busy server. A high archive age with low archive failure count suggests a slow or overloaded archive target (NFS, S3)."
                },
                "Avg Checkpoint": {
                    title: "Average Checkpoint Time",
                    text: "**What it is:** Average total time for checkpoint operations (write + sync) from `pg_stat_bgwriter`.\n\n**Why it matters:** Long checkpoints indicate storage I/O saturation. During a checkpoint, the database is writing all dirty pages to disk — slow checkpoints extend the window during which write operations experience elevated latency.\n\n**Fix:** Increase `checkpoint_completion_target` (default 0.9) to spread checkpoint I/O over a longer portion of the checkpoint interval. Move WAL to a faster storage device."
                },
                "WAL Generation Trend": {
                    title: "WAL Generation Trend Chart",
                    text: "**What it is:** Time-series chart of WAL generation rate over the observation period.\n\n**DBA use patterns:**\n- **Constant low rate:** Healthy write workload\n- **Periodic spikes:** Batch writes or large autovacuum runs — normal if controlled\n- **Continuously growing:** The archive cannot keep pace — storage or network bottleneck\n- **Sudden step up:** A schema migration, bulk load, or new high-write application feature was deployed"
                },
                "Replica Lag Chart": {
                    title: "Replica Lag per Standby Chart",
                    text: "**What it is:** Real-time lag visualization for each connected standby, showing write, flush, and replay lag separately over time.\n\n**Patterns:**\n- **Constant and small:** Healthy — normal replication latency\n- **Pulses periodically:** Batch writes on primary — normal if controlled\n- **Continuously growing:** Standby cannot keep pace — I/O bottleneck or network saturation\n- **Suddenly jumps and stabilizes:** A one-time large write that has now been applied"
                },
                "Archive Health": {
                    title: "Archive Success vs. Failure Chart",
                    text: "**What it is:** Time-series chart showing archived WAL file count vs. failure count over the observation window.\n\n**Why it matters:** Even a small sustained failure rate will eventually create gaps in the recovery timeline. Monitor this chart for any non-zero failure bars — each one represents a WAL file that cannot be used for PITR."
                },
                "Replication Node": {
                    title: "Replication Node Detail Table",
                    text: "**What it is:** Per-standby status from `pg_stat_replication` showing: application name, state, sync state, sent/write/flush/replay LSN, and computed lag.\n\n**Key status values:**\n- **`streaming`:** Normal, healthy replication\n- **`catchup`:** Standby is behind and consuming WAL faster than generated to catch up — concerning if sustained\n- **Sync state `sync`:** Primary waits for this standby before committing. Losing it may pause all writes if it's the only sync replica."
                }
            }
        },
        "Security Monitor": {
            description: "Audits user access, failed authentication attempts, privilege assignments, and data activity patterns. Essential for compliance (SOC2, PCI-DSS, HIPAA) and detecting security threats. Unlike some other systems, PostgreSQL brute-force protection must be implemented at the network or log-analysis layer.",
            metrics: {
                "Failed Logins": {
                    title: "Failed Logins (Count)",
                    text: "**What it is:** Count of authentication failures from log analysis.\n\n**Thresholds:**\n- ✅ Healthy: Near 0\n- ⚠️ Warning: > 10 failures/hour from any single IP\n- 🔴 Critical: > 100 failures/hour or unexpected geographic IP ranges\n\n**Why it matters:** Failed login patterns indicate brute-force attacks, application misconfiguration (wrong password after rotation), or credential theft attempts. PostgreSQL has no built-in account lockout — protect at the firewall or use `auth_delay` extension."
                },
                "Superuser Count": {
                    title: "Superuser Count",
                    text: "**What it is:** Count of roles with `rolsuper = true` in `pg_roles`.\n\n**Thresholds:**\n- ✅ Healthy: 1–2 superusers\n- ⚠️ Warning: 3–5\n- 🔴 Critical: > 5, or any application service account with superuser\n\n**Why it matters:** Superusers bypass all access controls — they can read any data, drop any object, and access the file system via `COPY TO/FROM PROGRAM`. Each superuser account is a potential full-compromise attack surface."
                },
                "Replication Privileges": {
                    title: "Replication Privileges",
                    text: "**What it is:** Count of roles with `rolreplication = true` in `pg_roles`.\n\n**Why it matters:** A compromised account with replication privilege can create a rogue replication slot (causing disk exhaustion), stream all database changes externally (data exfiltration), or consume the entire replication bandwidth.\n\n**Best practice:** Only the dedicated replication user should have this privilege. Revoke from all others: `ALTER ROLE user_name NOREPLICATION;`"
                }
            }
        },
        "Best Practices": {
            description: "Automated auditor that compares the current configuration against PostgreSQL-specific best practices, producing a clear RED/YELLOW/GREEN report with remediation SQL. Surfaces accumulated technical risk from sub-optimal configurations, such as inadequate shared_buffers or aggressive autovacuum settings.",
            metrics: {
                "shared_buffers": {
                    title: "shared_buffers",
                    text: "**What it controls:** PostgreSQL's own memory cache for data pages.\n\n**Recommendation:**\n- Minimum: 25% of total RAM\n- Practical: 25–40% of total RAM\n- Maximum effective: Diminishing returns above 40%\n- Example: On a 64GB server → `shared_buffers = 16GB`\n\n**Why it matters:** The default of 128MB is grossly inadequate for any production workload. Inadequate settings lead to frequent disk reads and poor query performance."
                },
                "Autovacuum Config": {
                    title: "Autovacuum Configuration",
                    text: "**`autovacuum_vacuum_scale_factor`:** Default 0.2 (20% of table must be dead before vacuum triggers). On a 100M row table, that's 20M dead tuples before any cleanup. Fix: `ALTER TABLE large_orders SET (autovacuum_vacuum_scale_factor = 0.01);`\n\n**`autovacuum_max_workers`:** Default 3 is insufficient for high-write multi-database instances. Recommend 5–8.\n\n**`log_autovacuum_min_duration`:** Set to `250ms` — logs slow autovacuum operations, invaluable for diagnosing contention."
                }
            }
        }
    };

    /**
     * Shows a modal with information about a specific metric or section.
     */
    window.pgShowInfo = function(section, metric) {
        const data = window.pgDashboardInfo[section];
        if (!data) return;

        let title, text;
        if (metric && data.metrics[metric]) {
            title = data.metrics[metric].title;
            text = data.metrics[metric].text;
        } else {
            title = section + " Overview";
            text = data.description;
        }

        if (window.showAppInfoModal) {
            window.showAppInfoModal(title, text);
        }
    };

    /**
     * Shows a complete detailed info modal for the entire dashboard with scroll.
     */
    window.pgShowDashboardDetail = function(dashboardName) {
        const data = window.pgDashboardInfo[dashboardName];
        if (!data) return;

        let fullText = `# ${dashboardName}\n\n${data.description}\n\n---\n\n`;
        fullText += `## Dashboard Metrics & Components\n\n`;

        for (const m in data.metrics) {
            const metric = data.metrics[m];
            fullText += `### ${metric.title}\n${metric.text}\n\n`;
        }

        if (window.showAppInfoModal) {
            window.showAppInfoModal(dashboardName + " - Complete Detailed Info", fullText, { width: '850px', maxHeight: '90vh' });
        }
    };
})();
