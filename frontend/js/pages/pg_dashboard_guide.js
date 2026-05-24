/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 * Purpose: PostgreSQL Dashboard Guide — visual reference for all PG dashboards, metrics, and DBA thresholds
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgDashboardGuideView = function () {

    // Navigation wrapper: sets the guide return flag so the router injects
    // a "Back to PostgreSQL Dashboard Guide" floating button on the target page.
    window._pgGuideNav = function(route) {
        window._pendingGuideReturn = 'pg-dashboard-guide';
        window.appNavigate(route);
    };

    const DASHBOARDS = [
        {
            id: 'control-center', route: 'pg-dashboard',
            icon: 'fa-gauge-high', color: '#2563eb',
            name: 'Control Center', tagline: 'Primary real-time health triage',
            description: 'The "first look" dashboard. A DBA should assess instance health within 30 seconds. Aggregates session concurrency, cache efficiency, wait event distribution, replica lag, and bloat risk into a single pane of glass sourced from pg_stat_activity, pg_stat_bgwriter, and pg_stat_database.',
            metrics: [
                { name: 'Health Score',               h: '90–100%',        w: '70–89%',        c: '<70%' },
                { name: 'Cache Hit Ratio',            h: '>99%',           w: '95–99%',        c: '<95%' },
                { name: 'Blocked Sessions',           h: '0',              w: '1–3 (>15s)',    c: '>3 or any >60s' },
                { name: 'Connections vs max',         h: '<70%',           w: '70–85%',        c: '>85%' },
                { name: 'Replica Lag',                h: '<10s',           w: '10–60s',        c: '>60s' },
            ],
            tips: [
                '"Idle in Transaction" sessions hold all acquired locks indefinitely — set <code>idle_in_transaction_session_timeout = \'5min\'</code>',
                'Every PostgreSQL connection is a separate OS process (~5–10 MB overhead). Use PgBouncer when connection count exceeds 200.',
                'Dropping TPS + rising waits = blocked concurrency. Dropping TPS + rising CPU = query regression.',
            ]
        },
        {
            id: 'enterprise-monitor', route: 'drilldown-pg-enterprise',
            icon: 'fa-chart-line', color: '#7c3aed',
            name: 'Enterprise Monitor', tagline: 'BGWriter, Checkpointer & WAL Archiver engine room',
            description: 'Monitors the background processes that maintain durability and I/O efficiency — BGWriter, Checkpointer, and WAL Archiver. These are invisible to applications but directly determine overall write performance. Also tracks temp file spill and configuration drift.',
            metrics: [
                { name: 'Timed vs Requested Checkpoints', h: '>90% timed',       w: 'Requested >10%', c: 'Requested >25%' },
                { name: 'BGWriter Halts (maxwritten_clean)', h: '0',             w: 'Occasional',     c: 'Frequent' },
                { name: 'WAL Archive Failures',           h: '0',                w: 'Any failure',    c: 'Persisting >5 min' },
                { name: 'buffers_backend vs buffers_clean', h: 'Clean >> Backend', w: 'Backend rising', c: 'Backend dominates' },
                { name: 'Temp Files / OLTP query',        h: '0',                w: 'Occasional',     c: 'Continuous' },
            ],
            tips: [
                'A single unattended WAL archive failure grows pg_wal/ without bound — when the filesystem fills, PostgreSQL crashes with PANIC.',
                'Increase <code>max_wal_size</code> to 4–16 GB if requested checkpoints exceed 10% of total; the default 1 GB is usually too small.',
                '<code>work_mem</code> is allocated per sort/hash operation per connection — setting it > 64 MB risks OOM under high concurrency.',
            ]
        },
        {
            id: 'cpu-health', route: 'pg-cpu',
            icon: 'fa-microchip', color: '#0891b2',
            name: 'CPU Health Monitor', tagline: 'Host OS and per-query CPU attribution',
            description: 'Tracks CPU utilization from both the host OS and PostgreSQL process perspective. Identifies which databases and queries drive consumption. CPU saturation in PostgreSQL is almost always caused by query-level issues — missing indexes, sort spills, or large hash joins.',
            metrics: [
                { name: 'PostgreSQL CPU %',    h: '<60% sustained',  w: '60–80%',           c: '>80% sustained' },
                { name: 'Dead Tuple %',        h: '<5%',             w: '5–20%',            c: '>20%' },
                { name: 'Cache Hit Ratio',     h: '>99%',            w: '95–99%',           c: '<95%' },
                { name: 'CPU per Connection',  h: '<1%',             w: '5–15%',            c: '>20% (runaway query)' },
            ],
            tips: [
                'Tables with >20% dead tuples force sequential scans to read and skip dead rows — bloat is both a storage and a CPU problem.',
                'Use <code>pg_stat_statements.total_exec_time</code> to find the top 10 queries — they typically account for 60–80% of total CPU.',
                'A single query can fan out to <code>max_parallel_workers_per_gather</code> parallel workers, each consuming a full CPU core.',
            ]
        },
        {
            id: 'memory', route: 'pg-memory',
            icon: 'fa-memory', color: '#d97706',
            name: 'Memory Intelligence', tagline: 'RAM, shared_buffers, work_mem, and spill analysis',
            description: 'Deep visibility into RAM usage, shared_buffers cache efficiency, work_mem spill behavior, and predictive memory analytics. Memory misconfiguration is the most common reason a well-designed PostgreSQL database performs poorly.',
            metrics: [
                { name: 'OS RAM Used %',       h: '<80%',    w: '80–90%',    c: '>90% or any swap' },
                { name: 'Swap Activity',       h: '0',       w: 'Any',       c: 'Sustained any rate' },
                { name: 'Cache Hit Ratio',     h: '>99%',    w: '95–99%',    c: '<95%' },
                { name: 'Temp Spill Rate',     h: '0/min',   w: 'Occasional',c: 'Regular OLTP spills' },
            ],
            tips: [
                'Memory budget: shared_buffers + (max_connections × 5–10 MB) + (active × work_mem × avg_sort_nodes). Plan carefully.',
                'Swap is 1,000,000× slower than RAM. Any sustained swap I/O on a production PostgreSQL server requires immediate action.',
                'Set <code>shared_buffers</code> to 25–40% of total RAM. On a 64 GB server: <code>shared_buffers = 16GB</code>.',
            ]
        },
        {
            id: 'waits-sessions', route: 'pg-waits',
            icon: 'fa-clock-rotate-left', color: '#059669',
            name: 'Waits & Sessions', tagline: 'Live session triage and wait event analysis',
            description: 'A real-time, comprehensive view of all database connections, their state, and the events they are waiting on. The "operating table" for live session diagnosis — shows exactly what every connection is doing right now.',
            metrics: [
                { name: 'Idle-in-Transaction Sessions', h: '0',           w: '1–3',           c: '>3 or any >5 min' },
                { name: 'Longest Active Session',       h: '<30 min',     w: '30–60 min',     c: '>1 hour' },
                { name: 'Wait Event: DataFileRead',     h: 'Minimal',     w: 'Trending up',   c: 'Dominant wait' },
                { name: 'Wait Event: relation lock',    h: '0',           w: 'Occasional',    c: 'Persistent >15s' },
            ],
            tips: [
                'An "idle in transaction" session blocks autovacuum on every table it touched — bloat accumulates for the entire duration.',
                'Set <code>statement_timeout = \'30min\'</code> as a safety net. For OLTP APIs: <code>ALTER ROLE api_user SET statement_timeout = \'5s\'</code>.',
                'The <code>wait_event_type + wait_event</code> column pair is the most actionable diagnostic: it precisely names what each session is waiting for.',
            ]
        },
        {
            id: 'locks', route: 'pg-locks',
            icon: 'fa-link-slash', color: '#dc2626',
            name: 'Locks & Blocking', tagline: 'Blocking hierarchy, deadlocks, and lock forensics',
            description: 'Specialized tool for troubleshooting database contention. PostgreSQL\'s MVCC means readers never block writers — so any blocking event is significant and typically indicates an application design problem or idle-in-transaction session.',
            metrics: [
                { name: 'Active Blocking Sessions', h: '0',         w: '1–3',          c: '>3' },
                { name: 'Deadlock Rate',            h: '0/day',     w: '1–5/day',      c: '>10/day' },
                { name: 'Idle-in-Txn Blocking',     h: '0',         w: 'Any',          c: 'Locking high-write table' },
                { name: 'Lock type: relation',      h: 'None',      w: 'Occasional DDL', c: 'Persistent' },
            ],
            tips: [
                'In a multi-level blocking chain (A→B→C), killing B is futile — only killing the root A releases all downstream sessions.',
                '<code>ALTER TABLE</code> requires AccessExclusiveLock — even a brief DDL can stall every subsequent query if a long SELECT is ahead of it.',
                'Use <code>SET lock_timeout = \'2s\'</code> before DDL in production to fail fast rather than blocking a queue of readers.',
            ]
        },
        {
            id: 'query-perf', route: 'pg-queries',
            icon: 'fa-bolt', color: '#ea580c',
            name: 'Query Monitor', tagline: 'pg_stat_statements — workload analysis & regressions',
            description: 'Aggregated performance statistics for all queries, powered by pg_stat_statements. The primary tool for query-level performance analysis and regression detection. Prerequisite: pg_stat_statements must be installed and in shared_preload_libraries.',
            metrics: [
                { name: 'p95 Latency trend',      h: 'Stable',        w: 'Gradual rise',   c: 'Sudden spike' },
                { name: 'Queries with temp spill', h: '0',            w: 'Occasional',     c: 'Regular OLTP spills' },
                { name: 'WAL bytes/exec (high ratio)', h: 'Stable',   w: 'Rising',         c: 'Sudden 3–5× spike' },
                { name: 'Regression: mean_exec_time', h: 'Stable',    w: '1.5× baseline', c: '>2× baseline' },
            ],
            tips: [
                'Sort by Total Time first — the top 5–10 queries account for 60–80% of total CPU. Fix these for maximum impact.',
                'High <code>stddev_exec_time</code> relative to <code>mean_exec_time</code> signals plan instability — the query is unpredictably fast or slow.',
                'After a regression: use <code>EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)</code> and compare plans to identify what changed.',
            ]
        },
        {
            id: 'explain', route: 'pg-explain',
            icon: 'fa-diagram-project', color: '#0d9488',
            name: 'EXPLAIN Plan Analyzer', tagline: 'Visual execution plan trees and index advisor',
            description: 'Interactive tool for visualizing PostgreSQL execution plans and receiving automated tuning advice. Accepts JSON output from EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) and renders a node-cost tree, metrics table, and concrete index suggestions.',
            metrics: [
                { name: 'Row Estimation Error (est vs actual)', h: '<10×', w: '10–100×',  c: '>100×' },
                { name: 'Sort Method',       h: 'quicksort (in-mem)', w: 'top-N heapsort', c: 'external merge (disk)' },
                { name: 'Hash Batches',      h: '1 (fits in work_mem)', w: '2–4',         c: '>4' },
                { name: 'Seq Scan on large table', h: 'None',           w: 'With filter',  c: 'Full table scan' },
            ],
            tips: [
                'Row estimation error >100× means the optimizer had wrong cardinality estimates — create extended statistics: <code>CREATE STATISTICS s ON (col_a, col_b) FROM t</code>.',
                'Always use <code>CREATE INDEX CONCURRENTLY</code> in production — standard CREATE INDEX takes an AccessExclusiveLock that blocks all reads and writes.',
                '"External merge" in a Sort node means sort spilled to disk — either add an index to eliminate the sort or increase <code>work_mem</code> session-locally.',
            ]
        },
        {
            id: 'storage', route: 'pg-storage',
            icon: 'fa-hard-drive', color: '#65a30d',
            name: 'Storage & Maintenance', tagline: 'Bloat, autovacuum health, and XID wraparound',
            description: 'Monitors physical database footprint, table and index bloat from PostgreSQL\'s MVCC model, and autovacuum efficiency. Every UPDATE and DELETE creates a dead tuple — autovacuum must reclaim them to prevent bloat and the catastrophic XID wraparound shutdown.',
            metrics: [
                { name: 'Dead Tuple %',          h: '<5%',           w: '5–20%',         c: '>20%' },
                { name: 'XID Age (datfrozenxid)', h: '<200M',        w: '200M–500M',     c: '>1.5 billion' },
                { name: 'Unused Index Count',     h: '0',            w: '1–5',           c: '>10 on busy tables' },
                { name: '7-Day Growth (bloat)',   h: '<2%',          w: '2–5%',          c: '>5% without TPS growth' },
            ],
            tips: [
                'XID wraparound at >1.9 billion causes a complete database shutdown with no connections allowed — monitor daily.',
                'For large tables, lower <code>autovacuum_vacuum_scale_factor</code> to 0.01 — the default 0.2 means 20M dead tuples on a 100M row table before vacuum triggers.',
                'Use <code>pg_repack</code> (not VACUUM FULL) for online table defragmentation — VACUUM FULL locks the table for the entire operation.',
            ]
        },
        {
            id: 'index-table', route: 'storage-index-health',
            icon: 'fa-boxes-stacked', color: '#7c3aed',
            name: 'Index & Table Health', tagline: 'Historical growth trends and index efficiency',
            description: 'Long-term historical analysis of index and table efficiency using TimescaleDB-backed metrics. Distinguishes organic growth (healthy) from bloat accumulation (unhealthy). Tracks sequential vs. index scan ratios and index read/write efficiency.',
            metrics: [
                { name: 'Index vs Seq Scan ratio', h: 'idx_scan dominant',  w: 'Mixed',           c: 'seq_scan dominant on large table' },
                { name: 'Index read:write ratio',  h: '>10:1',              w: '1:1 to 10:1',     c: '<1:1 (write cost > benefit)' },
                { name: 'Index Bloat %',           h: '<20%',               w: '20–40%',          c: '>40%' },
                { name: 'Growth trend',            h: 'Linear',             w: 'Accelerating',    c: 'Sudden step-up' },
            ],
            tips: [
                'Index bloat cannot be cleaned by VACUUM — only REINDEX CONCURRENTLY or pg_repack removes it.',
                'Verify server uptime before dropping unused indexes: <code>SELECT now() - pg_postmaster_start_time()</code> — stats reset on restart.',
                'TOAST tables can be larger than the main table for JSONB/text-heavy schemas — check pg_toast_* sizes when diagnosing growth.',
            ]
        },
        {
            id: 'backup-dr', route: 'pg-backups',
            icon: 'fa-shield-heart', color: '#0891b2',
            name: 'Backup & DR', tagline: 'WAL archiving, replica lag, and replication slot risk',
            description: 'Monitors WAL archiving health, streaming replication status, and replication slot risk. The archive is the foundation of PITR — a single missing WAL file makes all subsequent archives useless for recovery up to that point.',
            metrics: [
                { name: 'Archive Success %',       h: '100%',          w: 'Any failure',    c: '<99% or >5 min sustained' },
                { name: 'Max Replica Replay Lag',  h: '<10s',          w: '10–60s',         c: '>60s' },
                { name: 'Slot WAL Retention',      h: '<1 GB',         w: '1–10 GB',        c: '>10 GB' },
                { name: 'Inactive Replication Slots', h: '0',          w: 'Any inactive',   c: 'Inactive + growing WAL' },
            ],
            tips: [
                'A replication slot whose consumer disconnects will retain ALL WAL since disconnection — this fills the disk and crashes the primary.',
                'Set <code>max_slot_wal_keep_size = \'10GB\'</code> (PG 13+) to invalidate dangerous slots before they exhaust disk space.',
                'For failover readiness, watch <strong>replay lag</strong>, not write or flush lag — replay lag determines actual data currency on the standby.',
            ]
        },
        {
            id: 'security', route: 'pg-security',
            icon: 'fa-user-lock', color: '#dc2626',
            name: 'Security Monitor', tagline: 'Auth failures, privilege audit, and access patterns',
            description: 'Audits user access, failed authentication attempts, privilege assignments, and data activity patterns. Essential for SOC2, PCI-DSS, and HIPAA compliance. PostgreSQL has no built-in account lockout — brute-force protection must be at the network layer.',
            metrics: [
                { name: 'Failed Logins / hour',   h: '<5',           w: '5–50',            c: '>100 or unexpected IPs' },
                { name: 'Superuser Count',         h: '1–2',          w: '3–5',             c: '>5 or any app account' },
                { name: 'Roles with REPLICATION',  h: '1 (repl user)', w: '2–3',            c: '>3 or unknown roles' },
                { name: 'Roles with BYPASSRLS',    h: '0',            w: '1',               c: '>1 or app accounts' },
            ],
            tips: [
                'PostgreSQL superusers can read any data, drop any object, and access the filesystem via COPY TO/FROM PROGRAM — treat each as a full-compromise surface.',
                'Restrict network access in pg_hba.conf to known IP ranges and use <code>scram-sha-256</code> (not md5) for all user authentication.',
                'Enable <code>pgaudit</code> extension for production-grade audit logging: <code>pgaudit.log = \'ddl, write\'</code>.',
            ]
        },
        {
            id: 'best-practices', route: 'pg-best-practices',
            icon: 'fa-shield-halved', color: '#059669',
            name: 'Best Practices', tagline: 'Automated configuration audit with remediation SQL',
            description: 'Automated auditor that compares the current PostgreSQL configuration against best-practice guardrails, producing a RED/YELLOW/GREEN report with remediation SQL for each finding. A production-ready instance should have zero Critical findings.',
            metrics: [
                { name: 'shared_buffers',                    h: '25–40% RAM',    w: '<1 GB on >8 GB RAM', c: '128 MB (default)' },
                { name: 'autovacuum_vacuum_scale_factor',     h: '0.01–0.05',     w: '>0.1 on large tables', c: '0.2 (default) on >5M rows' },
                { name: 'idle_in_transaction_session_timeout', h: '<5 min',       w: '5–30 min',           c: '0 (disabled)' },
                { name: 'ssl',                                h: 'on',            w: 'pending',            c: 'off' },
                { name: 'wal_level',                          h: 'replica/logical', w: 'replica (no logical)', c: 'minimal with replicas' },
            ],
            tips: [
                '<code>effective_cache_size</code> does not allocate memory — it only influences the optimizer. Set it to 75% of total RAM for accurate plan choices.',
                'Default <code>autovacuum_max_workers = 3</code> is insufficient for high-write multi-database instances — increase to 5–8.',
                'Enabling <code>log_autovacuum_min_duration = 250</code> provides invaluable visibility into vacuum behavior at near-zero cost.',
            ]
        },
        {
            id: 'alerts', route: 'pg-alerts',
            icon: 'fa-bell', color: '#b45309',
            name: 'Alerts & Events', tagline: 'Active incidents and event timeline',
            description: 'Central hub for all active incidents and a chronological log of significant database events. Uses a two-column layout: active incidents table on the left and a real-time activity feed on the right. Incidents follow an Open → Acknowledged → Resolved lifecycle.',
            metrics: [
                { name: 'Open Critical Incidents',   h: '0',          w: '1–2 (acknowledged)', c: '>2 open/unacknowledged' },
                { name: 'Incident Response (Critical)', h: '<5 min',  w: '5–30 min',           c: '>30 min unacknowledged' },
                { name: 'Repeat incident rate',       h: '0',         w: 'Recurring weekly',   c: 'Daily recurring (unfixed)' },
            ],
            tips: [
                'Acknowledge Critical alerts within 5 minutes — it signals to teammates that someone is handling it and suppresses re-notifications.',
                'A single alert with a very high hit count and old first-seen time indicates a persistent condition that has been firing through multiple collection cycles.',
                'Resolve alerts only after confirming the underlying condition is cleared — not just the threshold crossed back.',
            ]
        },
    ];

    const thresholdRef = [
        ['Health Score', '90–100%', '70–89%', '<70%'],
        ['Cache Hit Ratio', '>99%', '95–99%', '<95%'],
        ['Active Sessions', '<baseline', '1.5× baseline', '>2× baseline'],
        ['Blocked Sessions', '0', '1–3', '>3'],
        ['Connections vs max', '<70%', '70–85%', '>85%'],
        ['Dead Tuple %', '<5%', '5–20%', '>20%'],
        ['XID Age', '<200M', '200M–500M', '>500M'],
        ['Replica Lag', '<10s', '10–60s', '>60s'],
        ['Archive Success %', '100%', 'Any failure', 'Sustained failure'],
        ['Slot WAL Retention', '<1 GB', '1–10 GB', '>10 GB'],
        ['Temp Files Rate', '0 (OLTP)', 'Occasional', 'Regular OLTP spills'],
        ['OS RAM Used', '<80%', '80–90%', '>90%'],
        ['Swap Activity', '0', 'Any', 'Sustained'],
        ['Timed Checkpoint %', '>90%', '75–90%', '<75%'],
        ['BGWriter Halts/s', '0', 'Occasional', 'Frequent'],
        ['Superuser Count', '1–2', '3–5', '>5'],
        ['Failed Logins/hr', '<5', '5–50', '>50'],
        ['Idle-in-Txn Sessions', '0', '1–3', '>3'],
        ['WAL Generation', 'Stable', 'Sudden 2× spike', 'Continuous growth'],
    ];

    function renderThresholdBadges(m) {
        return `
            <div style="display:flex;flex-direction:column;gap:4px;margin-top:8px;">
                ${m.map(metric => `
                <div style="display:grid;grid-template-columns:1fr auto auto auto;align-items:center;gap:6px;font-size:0.75rem;padding:4px 0;border-bottom:1px solid var(--border-color);">
                    <span style="color:var(--text-primary);font-weight:500;">${metric.name}</span>
                    <span style="background:rgba(16,185,129,0.15);color:#10b981;padding:1px 7px;border-radius:20px;white-space:nowrap;">${metric.h}</span>
                    <span style="background:rgba(245,158,11,0.15);color:#f59e0b;padding:1px 7px;border-radius:20px;white-space:nowrap;">${metric.w}</span>
                    <span style="background:rgba(239,68,68,0.15);color:#ef4444;padding:1px 7px;border-radius:20px;white-space:nowrap;">${metric.c}</span>
                </div>`).join('')}
            </div>`;
    }

    function renderDashboardCard(d) {
        return `
        <div class="guide-dash-card" data-route-target="${d.route}" style="
            background:var(--bg-surface);border:1px solid var(--border-color);
            border-radius:12px;padding:0;overflow:hidden;cursor:pointer;
            transition:transform 0.15s,box-shadow 0.15s;
        " onmouseenter="this.style.transform='translateY(-2px)';this.style.boxShadow='0 8px 24px rgba(0,0,0,0.15)'"
           onmouseleave="this.style.transform='';this.style.boxShadow=''">
            <div style="background:${d.color};padding:16px 20px;display:flex;align-items:center;gap:12px;">
                <div style="background:rgba(255,255,255,0.2);border-radius:8px;padding:10px;flex-shrink:0;">
                    <i class="fa-solid ${d.icon}" style="font-size:1.4rem;color:#fff;"></i>
                </div>
                <div>
                    <div style="font-weight:700;font-size:0.95rem;color:#fff;">${d.name}</div>
                    <div style="font-size:0.72rem;color:rgba(255,255,255,0.8);margin-top:2px;">${d.tagline}</div>
                </div>
            </div>
            <div style="padding:14px 16px;">
                <p style="font-size:0.78rem;color:var(--text-muted);margin:0 0 10px;line-height:1.5;display:-webkit-box;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden;">${d.description}</p>
                <button class="btn btn-xs btn-outline" onclick="event.stopPropagation();_pgGuideNav('${d.route}')" style="font-size:0.72rem;">
                    <i class="fa-solid fa-arrow-up-right-from-square" style="margin-right:4px;"></i>Open Dashboard
                </button>
            </div>
        </div>`;
    }

    function renderDetailSection(d) {
        return `
        <div id="guide-section-${d.id}" style="background:var(--bg-surface);border:1px solid var(--border-color);border-radius:12px;overflow:hidden;margin-bottom:16px;">
            <div style="background:${d.color};padding:14px 20px;display:flex;align-items:center;justify-content:space-between;">
                <div style="display:flex;align-items:center;gap:12px;">
                    <i class="fa-solid ${d.icon}" style="font-size:1.3rem;color:#fff;"></i>
                    <div>
                        <span style="font-weight:700;font-size:1rem;color:#fff;">${d.name}</span>
                        <span style="font-size:0.72rem;color:rgba(255,255,255,0.8);margin-left:10px;">${d.tagline}</span>
                    </div>
                </div>
                <button class="btn btn-xs" onclick="_pgGuideNav('${d.route}')" style="background:rgba(255,255,255,0.2);border:1px solid rgba(255,255,255,0.4);color:#fff;font-size:0.72rem;">
                    <i class="fa-solid fa-arrow-up-right-from-square" style="margin-right:4px;"></i>Open
                </button>
            </div>
            <div style="padding:18px 20px;display:grid;grid-template-columns:1fr 1fr;gap:20px;">
                <div>
                    <p style="font-size:0.82rem;color:var(--text-muted);line-height:1.6;margin:0 0 12px;">${d.description}</p>
                    <div style="display:flex;gap:8px;margin-bottom:10px;font-size:0.7rem;font-weight:600;color:var(--text-muted);">
                        <span style="background:rgba(16,185,129,0.15);color:#10b981;padding:2px 8px;border-radius:4px;">✓ HEALTHY</span>
                        <span style="background:rgba(245,158,11,0.15);color:#f59e0b;padding:2px 8px;border-radius:4px;">⚠ WARNING</span>
                        <span style="background:rgba(239,68,68,0.15);color:#ef4444;padding:2px 8px;border-radius:4px;">✕ CRITICAL</span>
                    </div>
                    ${renderThresholdBadges(d.metrics)}
                </div>
                <div>
                    <div style="font-size:0.75rem;font-weight:700;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.05em;margin-bottom:10px;">
                        <i class="fa-solid fa-lightbulb" style="color:#f59e0b;margin-right:6px;"></i>DBA Insights
                    </div>
                    <div style="display:flex;flex-direction:column;gap:8px;">
                        ${d.tips.map(tip => `
                        <div style="background:var(--bg-base);border-left:3px solid ${d.color};border-radius:0 6px 6px 0;padding:8px 12px;font-size:0.78rem;color:var(--text-secondary);line-height:1.5;">
                            ${tip}
                        </div>`).join('')}
                    </div>
                </div>
            </div>
        </div>`;
    }

    const cardGrid = DASHBOARDS.map(renderDashboardCard).join('');
    const detailSections = DASHBOARDS.map(renderDetailSection).join('');

    const refRows = thresholdRef.map(([m, h, w, c]) => `
        <tr>
            <td style="font-weight:500;font-size:0.78rem;">${m}</td>
            <td style="color:#10b981;font-size:0.75rem;">${h}</td>
            <td style="color:#f59e0b;font-size:0.75rem;">${w}</td>
            <td style="color:#ef4444;font-size:0.75rem;">${c}</td>
        </tr>`).join('');

    const jumpNav = DASHBOARDS.map(d => `
        <button onclick="document.getElementById('guide-section-${d.id}')?.scrollIntoView({behavior:'smooth',block:'start'})"
            style="flex-shrink:0;background:var(--bg-surface);border:1px solid var(--border-color);border-radius:20px;padding:5px 14px;font-size:0.72rem;color:var(--text-secondary);cursor:pointer;display:flex;align-items:center;gap:5px;white-space:nowrap;transition:all 0.15s;"
            onmouseenter="this.style.background='${d.color}';this.style.color='#fff';this.style.borderColor='${d.color}'"
            onmouseleave="this.style.background='';this.style.color='';this.style.borderColor=''">
            <i class="fa-solid ${d.icon}" style="font-size:0.7rem;"></i>${d.name}
        </button>`).join('');

    window.routerOutlet.innerHTML = `
    <div class="page-view active" style="padding:0;background:var(--bg-base);">

        <!-- Hero Header -->
        <div style="background:linear-gradient(135deg,#1e3a5f 0%,#1a2e50 50%,#162040 100%);padding:28px 32px 24px;position:relative;overflow:hidden;">
            <div style="position:absolute;top:-20px;right:-20px;width:200px;height:200px;background:rgba(59,130,246,0.08);border-radius:50%;pointer-events:none;"></div>
            <div style="position:absolute;bottom:-40px;left:40%;width:300px;height:300px;background:rgba(99,102,241,0.05);border-radius:50%;pointer-events:none;"></div>
            <div style="display:flex;align-items:flex-start;gap:16px;position:relative;">
                <div style="background:rgba(59,130,246,0.2);border:1px solid rgba(59,130,246,0.4);border-radius:12px;padding:14px;flex-shrink:0;">
                    <i class="fa-solid fa-database" style="font-size:2rem;color:#60a5fa;"></i>
                </div>
                <div>
                    <div style="display:flex;align-items:center;gap:10px;margin-bottom:4px;">
                        <h1 style="margin:0;font-size:1.5rem;font-weight:700;color:#f0f4ff;">PostgreSQL Dashboard Guide</h1>
                        <span style="background:rgba(59,130,246,0.3);border:1px solid rgba(59,130,246,0.5);color:#93c5fd;font-size:0.7rem;font-weight:600;padding:2px 10px;border-radius:12px;text-transform:uppercase;letter-spacing:0.05em;">Reference</span>
                    </div>
                    <p style="margin:0 0 14px;color:rgba(255,255,255,0.6);font-size:0.85rem;">Complete metric reference, DBA thresholds, and action guidance for all dashboards</p>
                    <div style="display:flex;gap:20px;flex-wrap:wrap;">
                        ${[['fa-table-columns','14 Dashboards'],['fa-chart-bar','100+ Metrics'],['fa-circle-check','Threshold Reference'],['fa-lightbulb','DBA Insights']].map(([ico,lbl]) => `
                        <div style="display:flex;align-items:center;gap:6px;color:rgba(255,255,255,0.7);font-size:0.78rem;">
                            <i class="fa-solid ${ico}" style="color:#60a5fa;font-size:0.9rem;"></i>${lbl}
                        </div>`).join('')}
                    </div>
                </div>
            </div>
        </div>

        <!-- Jump Navigation -->
        <div style="background:var(--bg-surface);border-bottom:1px solid var(--border-color);padding:10px 24px;display:flex;align-items:center;gap:8px;overflow-x:auto;position:sticky;top:0;z-index:20;">
            <span style="font-size:0.7rem;font-weight:600;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.05em;white-space:nowrap;margin-right:4px;">Jump to:</span>
            ${jumpNav}
        </div>

        <div style="padding:24px 28px;max-width:1400px;">

            <!-- Overview Cards Grid -->
            <div style="margin-bottom:28px;">
                <h2 style="font-size:0.85rem;font-weight:700;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.06em;margin:0 0 14px;display:flex;align-items:center;gap:8px;">
                    <i class="fa-solid fa-grid-2" style="color:#3b82f6;"></i>Dashboard Overview
                </h2>
                <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:14px;">
                    ${cardGrid}
                </div>
            </div>

            <!-- Detailed Reference Sections -->
            <div style="margin-bottom:28px;">
                <h2 style="font-size:0.85rem;font-weight:700;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.06em;margin:0 0 14px;display:flex;align-items:center;gap:8px;">
                    <i class="fa-solid fa-book-open" style="color:#3b82f6;"></i>Detailed Dashboard Reference
                </h2>
                ${detailSections}
            </div>

            <!-- Quick Reference Table -->
            <div style="background:var(--bg-surface);border:1px solid var(--border-color);border-radius:12px;overflow:hidden;">
                <div style="background:linear-gradient(90deg,#1e3a5f,#1a2e50);padding:14px 20px;display:flex;align-items:center;gap:10px;">
                    <i class="fa-solid fa-table" style="color:#60a5fa;font-size:1.1rem;"></i>
                    <span style="font-weight:700;color:#f0f4ff;font-size:0.95rem;">Quick Reference: PostgreSQL Metric Thresholds</span>
                </div>
                <div style="overflow-x:auto;">
                    <table style="width:100%;border-collapse:collapse;font-size:0.8rem;">
                        <thead>
                            <tr style="background:var(--bg-base);">
                                <th style="padding:10px 16px;text-align:left;font-weight:600;color:var(--text-muted);border-bottom:2px solid var(--border-color);">Metric</th>
                                <th style="padding:10px 16px;text-align:left;font-weight:600;color:#10b981;border-bottom:2px solid var(--border-color);">✓ Healthy</th>
                                <th style="padding:10px 16px;text-align:left;font-weight:600;color:#f59e0b;border-bottom:2px solid var(--border-color);">⚠ Warning</th>
                                <th style="padding:10px 16px;text-align:left;font-weight:600;color:#ef4444;border-bottom:2px solid var(--border-color);">✕ Critical</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${refRows}
                        </tbody>
                    </table>
                </div>
            </div>

        </div>
    </div>`;

    // Card click → open dashboard (sets guide return flag)
    document.querySelectorAll('.guide-dash-card').forEach(card => {
        card.addEventListener('click', () => {
            const route = card.dataset.routeTarget;
            if (route) _pgGuideNav(route);
        });
    });
};
