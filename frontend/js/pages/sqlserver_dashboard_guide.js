/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 * Purpose: SQL Server Dashboard Guide — visual reference for all SQL Server dashboards, metrics, and DBA thresholds
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.SqlServerDashboardGuideView = function () {

    // Navigation wrapper: sets the guide return flag so the router injects
    // a "Back to SQL Server Dashboard Guide" floating button on the target page.
    window._ssGuideNav = function(route) {
        window._pendingGuideReturn = 'sqlserver-dashboard-guide';
        window.appNavigate(route);
    };

    const DASHBOARDS = [
        {
            id: 'instance-dashboard', route: 'sqlserver-health-v2',
            icon: 'fa-gauge-high', color: '#0078d4',
            name: 'Instance Dashboard', tagline: 'Primary real-time health triage',
            description: 'The "first look" dashboard. A DBA should assess instance health within 30 seconds. Consolidates CPU, memory, I/O, blocking, and wait state signals from multiple DMVs. Surfaces top offending queries, long-running sessions, and the most actionable session data on a single page refreshed every 30 seconds.',
            metrics: [
                { name: 'Health Score',         h: '90–100%',       w: '70–89%',          c: '<70%' },
                { name: 'CPU Load',             h: '<60% sustained', w: '60–80%',          c: '>80% or >95% spike' },
                { name: 'PLE (Page Life Expectancy)', h: '>1000s', w: '300–1000s',       c: '<300s or rapid drop' },
                { name: 'Buffer Cache Hit Ratio', h: '>99%',        w: '95–99%',          c: '<95%' },
                { name: 'Blocked Sessions',     h: '0',             w: '1–5 (>10s)',      c: '>5 or any >30s' },
                { name: 'TPS trend',            h: 'Stable/growing', w: 'Declining 10–30%', c: 'Sudden drop >30%' },
            ],
            tips: [
                'PLE and Buffer Cache Hit Ratio must be evaluated together — a high BCHR with low PLE means the cache is turning over rapidly, not that it is healthy.',
                'Always cross-reference the Wait Categories doughnut. Dominant CPU waits → query optimization. Dominant I/O waits → memory or storage issue.',
                'An "idle in transaction" root blocker is the most dangerous blocking pattern — the transaction holds locks indefinitely with no active SQL.',
            ]
        },
        {
            id: 'workload', route: 'sqlserver-workload',
            icon: 'fa-chart-area', color: '#7c3aed',
            name: 'Workload Analytics', tagline: 'CPU/IO trends, app attribution, and scheduler health',
            description: 'Deep-dive analysis of total workload impact. Identifies which applications, users, and query patterns drive resource consumption. Breaks workload intensity into CPU compute time, I/O volume, and throughput to distinguish "more work" from "less efficient work".',
            metrics: [
                { name: 'CPU per Execution trend', h: 'Stable',        w: 'Gradual rise',   c: 'Step-change up (regression)' },
                { name: 'Runnable Tasks / core',   h: '0',             w: '1–5',            c: '>5 (CPU queue)' },
                { name: 'Work Queue Depth',        h: '0',             w: 'Occasional',     c: '>10 (thread pool saturating)' },
                { name: 'Re-compilations/sec vs Batch Requests', h: '<1%', w: '1–5%',      c: '>5%' },
            ],
            tips: [
                'A sustained rise in CPU-per-execution without a deployment is a "slow boil" regression that threshold alerts miss entirely.',
                'If one application suddenly dominates CPU in the App Analysis chart, compare its query patterns in Query Analysis filtered by that app_name.',
                'High re-compilations: check for SET option changes between connections (common ORM pitfall), schema changes, or excessive WITH RECOMPILE hints.',
            ]
        },
        {
            id: 'wait-stats', route: 'sqlserver-waits',
            icon: 'fa-clock-rotate-left', color: '#0891b2',
            name: 'Wait Statistics', tagline: 'Long-term wait category trends and bottleneck identification',
            description: 'Wait statistics are the most reliable diagnostic tool in SQL Server — they tell you what the server is waiting for rather than just what it\'s doing. This dashboard shows long-term wait trends to catch structural degradation that point-in-time views miss entirely.',
            metrics: [
                { name: 'PAGEIOLATCH waits trend',  h: 'Stable low',   w: 'Gradual rise',   c: 'Dominant (I/O saturation)' },
                { name: 'LCK_M_* waits',            h: '0',            w: 'Occasional',     c: 'Dominant (locking crisis)' },
                { name: 'SOS_SCHEDULER_YIELD',      h: 'Minimal',      w: 'Rising',         c: 'Dominant (CPU saturation)' },
                { name: 'WRITELOG / LOG_RATE_GOV',  h: 'Low',          w: 'Moderate',       c: 'Dominant (log I/O bottleneck)' },
                { name: 'ASYNC_NETWORK_IO',         h: 'Low',          w: 'Moderate',       c: 'Dominant (client consuming slowly)' },
            ],
            tips: [
                'A gradual 3-week rise in PAGEIOLATCH waits predicts I/O saturation before it becomes an incident — act on the trend, not just the alert.',
                'WRITELOG dominant: move the transaction log to a dedicated faster disk — log writes are always synchronous and block transaction commit.',
                'ASYNC_NETWORK_IO dominant: the bottleneck is in the application tier, not SQL Server — reduce result set sizes or fix application code.',
            ]
        },
        {
            id: 'memory', route: 'drilldown-memory',
            icon: 'fa-memory', color: '#d97706',
            name: 'Memory Analyzer', tagline: 'Buffer pool, memory clerks, and grant queue analysis',
            description: 'Advanced memory diagnostics to identify pressure points, cache efficiency, and workspace memory grant distribution. Goes beyond memory % to reveal why memory is pressured — whether from buffer pool eviction, grant queue saturation, or oversized plan cache.',
            metrics: [
                { name: 'Grants Pending',           h: '0',            w: '1–3',            c: '>3 or any >30s' },
                { name: 'PLE',                      h: '>1000s',       w: '300–1000s',      c: '<300s' },
                { name: 'Available Headroom (MB)',  h: '>500 MB',      w: '100–500 MB',     c: '<100 MB' },
                { name: 'Plan Cache type: Adhoc',   h: 'Small fraction', w: 'Growing fast', c: '>80% of all plans' },
            ],
            tips: [
                'Grants Pending is a stop-the-world condition — queries in the queue cannot execute at all, not slowly. Escalate immediately.',
                'Plan cache bloat from non-parameterized queries: enable <code>EXEC sp_configure \'optimize for ad hoc workloads\', 1</code> immediately.',
                'When PLE drops while Buffer Cache Hit Ratio stays high: the cache is recycling rapidly — the working dataset is larger than the buffer pool.',
            ]
        },
        {
            id: 'blocking', route: 'sqlserver-locks',
            icon: 'fa-link-slash', color: '#b91c1c',
            name: 'Locks & Blocking', tagline: 'Visual blocking trees, deadlock history, and contention analysis',
            description: 'High-density analysis of locking contention, blocking hierarchies, and deadlock history. Surfaces the root blocker (the session to kill), visualizes the blocking tree, and provides deadlock XML forensics from the System Health extended event session.',
            metrics: [
                { name: 'Active Blocked Sessions',  h: '0',            w: '1–5',            c: '>5' },
                { name: 'Longest Wait (ms)',         h: '<100ms',       w: '100ms–30s',      c: '>30s (timeouts likely)' },
                { name: 'Deadlock Count (24h)',      h: '0–2',          w: '3–10',           c: '>10 or any critical txn' },
                { name: 'Root Blockers count',       h: '0',            w: '1',              c: '>1' },
            ],
            tips: [
                'Kill the root blocker, not the victims — killing a victim frees it temporarily but the root immediately blocks the next arrival.',
                'An idle root blocker with no active SQL is the most dangerous pattern: the application opened a transaction and forgot to commit it.',
                'Enable <code>READ_COMMITTED_SNAPSHOT ISOLATION (RCSI)</code> to eliminate reader-writer conflicts — readers will never block writers.',
            ]
        },
        {
            id: 'enterprise-metrics', route: 'enterprise-metrics',
            icon: 'fa-chart-line', color: '#059669',
            name: 'Enterprise Metrics', tagline: 'Long-term engine telemetry and plan cache composition',
            description: 'Enterprise-wide, time-series observability focused on engine efficiency. Long-term wait trends, throughput vs. compilation rates, plan cache composition, and TempDB consumer analysis. Reveals systemic trends invisible in point-in-time views.',
            metrics: [
                { name: 'Compilations / Batch Req ratio', h: '<5%',     w: '5–15%',         c: '>15%' },
                { name: 'Re-compilations / Batch Req',    h: '<1%',     w: '1–5%',          c: '>5%' },
                { name: 'Single-use (Adhoc) plans %',     h: '<20%',    w: '20–60%',        c: '>80%' },
                { name: 'Latch Waits/sec',                h: 'Stable low', w: 'Rising',      c: 'PAGELATCH_EX dominant' },
            ],
            tips: [
                'Adhoc plans growing faster than Proc plans signals missing query parameterization — each unique literal generates a unique cache entry.',
                'PAGELATCH_EX on TempDB: add TempDB data files equal to CPU count (up to 8) to distribute PFS/GAM allocation bitmap contention.',
                'High compilations + stable Batch Requests: the plan cache is thrashing — queries are not being reused and each execution pays compilation overhead.',
            ]
        },
        {
            id: 'storage-index', route: 'storage-index-health',
            icon: 'fa-boxes-stacked', color: '#65a30d',
            name: 'Storage & Index Health', tagline: 'Growth forecasting, fragmentation, and index efficiency',
            description: 'Advanced observability into storage consumption, index efficiency, and table growth forecasting. Bridges operational monitoring and capacity planning. Identifies unused, duplicate, and highly fragmented indexes — each a source of write overhead without query benefit.',
            metrics: [
                { name: '7-Day Growth %',              h: 'Predictable', w: 'Accelerating',  c: 'Sudden spike' },
                { name: 'Index Fragmentation %',       h: '<10%',         w: '10–30%',       c: '>30%' },
                { name: 'Index Read:Write ratio',      h: '>10:1',        w: '1:1 to 10:1',  c: '<1:1' },
                { name: 'Reclaimable space (unused idx)', h: 'Small',    w: '>10 GB',        c: '>50 GB' },
            ],
            tips: [
                '<code>sys.dm_db_index_usage_stats</code> resets on service restart — verify server uptime before dropping any index with zero reads.',
                'Index-to-data ratio > 100% on a table means it is over-indexed: more storage and write overhead is consumed by indexes than actual data.',
                'Use <code>ALTER INDEX ... REORGANIZE</code> (online, low impact) at 10–30% fragmentation; <code>REBUILD</code> for >30% on large indexes.',
            ]
        },
        {
            id: 'performance-debt', route: 'performance-debt',
            icon: 'fa-screwdriver-wrench', color: '#6366f1',
            name: 'Performance Debt', tagline: 'Configuration audit, statistics health, and backup assessment',
            description: 'Systematic audit of accumulated technical risk from sub-optimal configurations, missed maintenance, and operational gaps. Turns reactive firefighting into proactive hardening. Evaluates MAXDOP, memory, TempDB, index health, statistics freshness, and backup currency.',
            metrics: [
                { name: 'MAXDOP',                        h: '1–8 (OLTP)',  w: '0 (unlimited)', c: '0 with no CTP change' },
                { name: 'Cost Threshold for Parallelism', h: '25–50',      w: '5–25',          c: '5 (default, too low)' },
                { name: 'Max Server Memory',              h: 'Explicitly set', w: 'Near total RAM', c: 'Unset (= total RAM)' },
                { name: 'Backup age vs SLA',              h: 'Within SLA', w: 'Approaching limit', c: 'Missed SLA window' },
                { name: 'AUTO_SHRINK',                    h: 'OFF',        w: 'OFF',           c: 'ON (destructive)' },
            ],
            tips: [
                'AUTO_SHRINK creates a destructive expand-shrink cycle that continuously fragments indexes — flag as Critical and disable immediately.',
                'MAXDOP = 0 allows a single query to consume every CPU core, starving all concurrent users. Set to min(physical cores per NUMA, 8) for OLTP.',
                'Statistics threshold at 20% of rows: fine for small tables, but a 1-billion-row table requires 200M row changes — stats may never auto-update.',
            ]
        },
        {
            id: 'intel-report', route: 'sqlserver-intelligence-report',
            icon: 'fa-brain', color: '#7c3aed',
            name: 'Intelligence Report', tagline: 'AI-driven instance health narrative and anomaly detection',
            description: 'AI-generated natural language analysis of cross-metric correlations and behavioral anomalies. Surfaces insights that single-metric monitoring misses — patterns only visible by reading multiple dashboards simultaneously.',
            metrics: [
                { name: 'Overall instance risk',    h: 'Low',       w: 'Medium',        c: 'High / Critical' },
                { name: 'Anomaly detection',        h: 'No anomalies', w: 'Behavioral deviation', c: 'Multiple correlated anomalies' },
            ],
            tips: [
                '"Connection count increased 40% while CPU increased 80%" — suggests new queries per connection, not more users. Identify what changed in the app.',
                '"PLE declining 15% over 6 hours while buffer pool usage is stable" — possible external OS memory pressure, not internal SQL Server issue.',
                'Use intelligence report observations as leads: they point to which specialized dashboard to investigate next.',
            ]
        },
        {
            id: 'query-analysis', route: 'query-analysis',
            icon: 'fa-magnifying-glass-chart', color: '#ea580c',
            name: 'Query Analysis', tagline: 'Query Store: regressions, plan instability, and top consumers',
            description: 'Deep analysis of Query Store data to identify regressions, plan instability, and high-impact queries. Makes Query Store actionable — surfaces queries that were fast yesterday and slow today, or queries oscillating between multiple execution plans.',
            metrics: [
                { name: 'Query Regressions (24h)',   h: '0',          w: '1–3',           c: '>3 or critical query' },
                { name: 'Plan Changes (24h)',        h: '0–5',        w: '5–20',          c: '>20 (mass invalidation)' },
                { name: 'Plan Instability (multi-plan queries)', h: 'Minimal', w: 'A few', c: 'Many high-frequency queries' },
                { name: 'Top 10 CPU Share %',        h: '<50%',       w: '50–80%',        c: '>80% (concentrated load)' },
            ],
            tips: [
                'Force the previously-good plan using <code>sys.sp_query_store_force_plan</code> while you investigate root cause — don\'t let users suffer while you debug.',
                'Parameter sniffing causes plan instability on high-skew data. Test with <code>OPTION (OPTIMIZE FOR UNKNOWN)</code> to use average statistics.',
                'A sudden spike in plan changes (200 in an hour vs. 5/day) indicates mass plan invalidation — correlate with stats updates or schema changes.',
            ]
        },
        {
            id: 'plan-analyzer', route: 'sqlserver-plan-analyzer',
            icon: 'fa-diagram-project', color: '#0d9488',
            name: 'Plan Analyzer', tagline: 'Visual execution plan tree and optimization guidance',
            description: 'Interactive tool for visualizing SQL Server execution plans. Upload a plan XML to see a node-cost tree, identify the most expensive operators, detect cardinality estimation errors, and receive concrete index recommendations with estimated improvement percentages.',
            metrics: [
                { name: 'Cardinality estimation error', h: '<10×',    w: '10–100×',        c: '>100× (optimizer blind)' },
                { name: 'Parallel plan efficiency',     h: 'Good',    w: 'DOP < MAXDOP',   c: 'DOP 1 but MAXDOP set high' },
                { name: 'Sort/Hash spill to TempDB',    h: 'None',    w: 'Occasional',     c: 'On every execution' },
                { name: 'Key Lookup operations',        h: '0',       w: 'Low volume',     c: 'On large table (cover the index)' },
            ],
            tips: [
                'A Key Lookup on a large table means the non-clustered index is missing columns used in SELECT — add an INCLUDE clause to make it covering.',
                'Hash Match Probe residual predicates indicate a scan was forced due to type mismatch or implicit conversion — check column data types in the join.',
                'When plan forces don\'t stick: verify the query has a stable query_hash in Query Store and that parameterization is consistent.',
            ]
        },
        {
            id: 'watched-queries', route: 'watched-queries',
            icon: 'fa-binoculars', color: '#0891b2',
            name: 'Watched Queries', tagline: 'Persistent per-query monitoring and plan history',
            description: 'Deep, persistent monitoring for a curated set of critical queries. Provides the longitudinal context aggregate dashboards cannot — per-query duration, CPU, and I/O history alongside a complete execution plan timeline for each watched query.',
            metrics: [
                { name: 'Per-query Duration trend',  h: 'Stable',      w: 'Gradual rise',  c: 'Sudden step-change up' },
                { name: 'Plan changes for query',    h: '0–1',         w: '2–5',           c: '>5 (extreme instability)' },
                { name: 'CPU variance (stddev)',      h: 'Low',         w: 'Moderate',      c: 'High (unpredictable latency)' },
            ],
            tips: [
                'For a business-critical query, the plan history is the definitive regression evidence: compare good plan vs. bad plan in XML side-by-side.',
                'A steadily increasing per-execution cost over months without a plan change signals data growth outpacing the index design.',
                'Periodic spikes correlating with specific times: cross-reference with SQL Agent Jobs — nightly maintenance windows often cause plan cache thrashing.',
            ]
        },
        {
            id: 'agent-jobs', route: 'jobs',
            icon: 'fa-briefcase', color: '#374151',
            name: 'SQL Agent Jobs', tagline: 'Scheduled maintenance monitoring and failure forensics',
            description: 'Centralized monitoring of all SQL Server Agent jobs — maintenance, ETL, backups, and reports. A failed backup job means the next hardware failure results in data loss. A failed statistics job means queries degrade silently. These failures are invisible without active monitoring.',
            metrics: [
                { name: 'Failed jobs in period',     h: '0',           w: '1–2',           c: '>2 or any backup job' },
                { name: 'Jobs running during peak',  h: 'None',        w: 'Light jobs only', c: 'Heavy I/O jobs' },
                { name: 'Job overlap risk',          h: 'None',        w: 'Potential',      c: 'Confirmed I/O contention' },
            ],
            tips: [
                'A failed backup job is a silent disaster — the next outage will produce data loss. Treat backup job failures as Critical regardless of timing.',
                'Failures clustering at the same time daily indicate a resource conflict at that hour — check for overlapping jobs or peak-hours TempDB exhaustion.',
                'Use the step-level failure modal for the actual error message — the top-level "job failed" message is useless without step detail.',
            ]
        },
        {
            id: 'alerts', route: 'alerts',
            icon: 'fa-triangle-exclamation', color: '#b45309',
            name: 'Alerts & Events', tagline: 'Active incidents and behavior-based anomaly detection',
            description: 'Central hub for all active incidents and behavior-based anomaly detection. Identifies deviations from established normal patterns — not just static threshold crossings — providing early warnings before traditional alerts fire.',
            metrics: [
                { name: 'Open Critical Alerts',     h: '0',           w: '1–2 acknowledged', c: '>2 unacknowledged' },
                { name: 'Anomaly: CPU rate of change', h: 'Normal',   w: 'Rising trend',     c: 'Sudden spike (2× baseline)' },
                { name: 'Warning → Critical conversion', h: 'Rare',   w: 'Occasional',       c: 'Frequent (systemic issue)' },
            ],
            tips: [
                'A Warning alert for "CPU trending upward over 3 hours" means the system is heading for a crisis — act on trends, not just threshold crossings.',
                'Anomaly detection fires when rate-of-change or deviation from baseline is abnormal — CPU at 60% that was 20% yesterday is a Warning even though 60% is fine in isolation.',
                'Cross-metric AI insights bridge "something is wrong" and "here\'s why" — read them as leads for which specialized dashboard to open next.',
            ]
        },
        {
            id: 'best-practices', route: 'best-practices',
            icon: 'fa-shield-halved', color: '#059669',
            name: 'Best Practices', tagline: 'Automated guardrails with copy-paste remediation SQL',
            description: 'Automated "DBA consultant" evaluating server and database configurations against curated best practices. Produces a RED/YELLOW/GREEN report with verified, copy-paste remediation T-SQL scripts. A production-ready instance should have zero Critical findings.',
            metrics: [
                { name: 'MAXDOP',                    h: '1–8',         w: '>8 on OLTP',     c: '0 (unlimited)' },
                { name: 'Max Server Memory',         h: 'Explicitly set', w: 'Near total',  c: 'Unset (default 2147483647)' },
                { name: 'Auto-Shrink',               h: 'OFF on all DBs', w: 'OFF',         c: 'ON any database' },
                { name: 'Recovery Model vs backups', h: 'FULL + log backups', w: 'BULK_LOGGED', c: 'SIMPLE on critical DB' },
                { name: 'Page Verification',         h: 'CHECKSUM',    w: 'TORN_PAGE',      c: 'NONE' },
            ],
            tips: [
                'Max Server Memory unset allows SQL Server to consume all physical RAM, starving the OS and causing catastrophic swapping.',
                'VLF count > 500 in the transaction log causes slow log backup and restore. Fix: shrink log once, set a large fixed auto-growth increment, let it regrow.',
                'Instant File Initialization enabled reduces data file growth I/O stalls from minutes to milliseconds — verify it is configured at the OS service account level.',
            ]
        },
    ];

    const thresholdRef = [
        ['CPU Load',                '<60%',          '60–80%',        '>80%'],
        ['PLE',                     '>1000s',        '300–1000s',     '<300s'],
        ['Buffer Cache Hit Ratio',  '>99%',          '95–99%',        '<95%'],
        ['Grants Pending',          '0',             '1–3',           '>3'],
        ['Blocked Sessions',        '0',             '1–5',           '>5'],
        ['Disk Read Latency',       '<5ms',          '5–20ms',        '>20ms'],
        ['Disk Write Latency',      '<1ms',          '1–10ms',        '>10ms'],
        ['Deadlocks (24h)',         '0–2',           '3–10',          '>10'],
        ['TempDB Used',             '<50%',          '50–80%',        '>80%'],
        ['Index Fragmentation',     '<10%',          '10–30%',        '>30%'],
        ['Runnable Tasks / Core',   '0',             '1–5',           '>5'],
        ['Health Score',            '90–100%',       '70–89%',        '<70%'],
        ['Compilations / Batch Req','<5%',           '5–15%',         '>15%'],
        ['Query Regressions (24h)', '0',             '1–3',           '>3'],
        ['Plan Changes (24h)',      '0–5',           '5–20',          '>20'],
        ['Auto-Shrink',             'OFF',           'OFF',           'ON (Critical)'],
        ['MAXDOP',                  '1–8 (OLTP)',    '>8',            '0 (unlimited)'],
        ['Backup age vs SLA',       'Within SLA',   'Approaching',   'Missed SLA'],
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
        <div class="guide-ss-card" data-route-target="${d.route}" style="
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
                <button class="btn btn-xs btn-outline" onclick="event.stopPropagation();_ssGuideNav('${d.route}')" style="font-size:0.72rem;">
                    <i class="fa-solid fa-arrow-up-right-from-square" style="margin-right:4px;"></i>Open Dashboard
                </button>
            </div>
        </div>`;
    }

    function renderDetailSection(d) {
        return `
        <div id="guide-ss-section-${d.id}" style="background:var(--bg-surface);border:1px solid var(--border-color);border-radius:12px;overflow:hidden;margin-bottom:16px;">
            <div style="background:${d.color};padding:14px 20px;display:flex;align-items:center;justify-content:space-between;">
                <div style="display:flex;align-items:center;gap:12px;">
                    <i class="fa-solid ${d.icon}" style="font-size:1.3rem;color:#fff;"></i>
                    <div>
                        <span style="font-weight:700;font-size:1rem;color:#fff;">${d.name}</span>
                        <span style="font-size:0.72rem;color:rgba(255,255,255,0.8);margin-left:10px;">${d.tagline}</span>
                    </div>
                </div>
                <button class="btn btn-xs" onclick="_ssGuideNav('${d.route}')" style="background:rgba(255,255,255,0.2);border:1px solid rgba(255,255,255,0.4);color:#fff;font-size:0.72rem;">
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
        <button onclick="document.getElementById('guide-ss-section-${d.id}')?.scrollIntoView({behavior:'smooth',block:'start'})"
            style="flex-shrink:0;background:var(--bg-surface);border:1px solid var(--border-color);border-radius:20px;padding:5px 14px;font-size:0.72rem;color:var(--text-secondary);cursor:pointer;display:flex;align-items:center;gap:5px;white-space:nowrap;transition:all 0.15s;"
            onmouseenter="this.style.background='${d.color}';this.style.color='#fff';this.style.borderColor='${d.color}'"
            onmouseleave="this.style.background='';this.style.color='';this.style.borderColor=''">
            <i class="fa-solid ${d.icon}" style="font-size:0.7rem;"></i>${d.name}
        </button>`).join('');

    window.routerOutlet.innerHTML = `
    <div class="page-view active" style="padding:0;background:var(--bg-base);">

        <!-- Hero Header -->
        <div style="background:linear-gradient(135deg,#001427 0%,#001f3f 50%,#00264d 100%);padding:28px 32px 24px;position:relative;overflow:hidden;">
            <div style="position:absolute;top:-20px;right:-20px;width:200px;height:200px;background:rgba(0,120,212,0.08);border-radius:50%;pointer-events:none;"></div>
            <div style="position:absolute;bottom:-40px;left:40%;width:300px;height:300px;background:rgba(99,102,241,0.05);border-radius:50%;pointer-events:none;"></div>
            <div style="display:flex;align-items:flex-start;gap:16px;position:relative;">
                <div style="background:rgba(0,120,212,0.2);border:1px solid rgba(0,120,212,0.4);border-radius:12px;padding:14px;flex-shrink:0;">
                    <i class="fa-brands fa-microsoft" style="font-size:2rem;color:#60a5fa;"></i>
                </div>
                <div>
                    <div style="display:flex;align-items:center;gap:10px;margin-bottom:4px;">
                        <h1 style="margin:0;font-size:1.5rem;font-weight:700;color:#f0f4ff;">SQL Server Dashboard Guide</h1>
                        <span style="background:rgba(0,120,212,0.3);border:1px solid rgba(0,120,212,0.5);color:#93c5fd;font-size:0.7rem;font-weight:600;padding:2px 10px;border-radius:12px;text-transform:uppercase;letter-spacing:0.05em;">Reference</span>
                    </div>
                    <p style="margin:0 0 14px;color:rgba(255,255,255,0.6);font-size:0.85rem;">Complete metric reference, DBA thresholds, and action guidance for all dashboards</p>
                    <div style="display:flex;gap:20px;flex-wrap:wrap;">
                        ${[['fa-table-columns','15 Dashboards'],['fa-chart-bar','80+ Metrics'],['fa-circle-check','Threshold Reference'],['fa-lightbulb','DBA Insights']].map(([ico,lbl]) => `
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
                    <i class="fa-solid fa-grid-2" style="color:#0078d4;"></i>Dashboard Overview
                </h2>
                <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:14px;">
                    ${cardGrid}
                </div>
            </div>

            <!-- Detailed Reference Sections -->
            <div style="margin-bottom:28px;">
                <h2 style="font-size:0.85rem;font-weight:700;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.06em;margin:0 0 14px;display:flex;align-items:center;gap:8px;">
                    <i class="fa-solid fa-book-open" style="color:#0078d4;"></i>Detailed Dashboard Reference
                </h2>
                ${detailSections}
            </div>

            <!-- Quick Reference Table -->
            <div style="background:var(--bg-surface);border:1px solid var(--border-color);border-radius:12px;overflow:hidden;">
                <div style="background:linear-gradient(90deg,#001427,#001f3f);padding:14px 20px;display:flex;align-items:center;gap:10px;">
                    <i class="fa-solid fa-table" style="color:#60a5fa;font-size:1.1rem;"></i>
                    <span style="font-weight:700;color:#f0f4ff;font-size:0.95rem;">Quick Reference: SQL Server Metric Thresholds</span>
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
    document.querySelectorAll('.guide-ss-card').forEach(card => {
        card.addEventListener('click', () => {
            const route = card.dataset.routeTarget;
            if (route) _ssGuideNav(route);
        });
    });
};
