/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Dashboard v2 (Instant Triage) controller.
 * Metadata:
 *   - Version: 1.5 (Standardized UI & Rigid Layout)
 *   - Features: Sparklines, Wait Trends, IO Latency, Throughput, Bar TempDB, Modern Problems Table.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.SqlServerHealthV2View = async function() {
    const outlet = window.routerOutlet;
    if (!outlet) return;

    const instIdx = window.appState.currentInstanceIdx;
    const inst = window.appState.config?.instances?.[instIdx];
    if (!inst) {
        outlet.innerHTML = '<div class="alert alert-warning">Please select a SQL Server instance first.</div>';
        return;
    }

    // Cleanup previous charts
    if (window.v2Charts) {
        Object.values(window.v2Charts).forEach(c => c && c.destroy());
    }
    window.v2Charts = {};

    // 1. Initial Shell with standard heading and NEW RIGID GRID CSS
    outlet.innerHTML = `
        <style>
            .dash-v2-container {
                display: flex;
                flex-direction: column;
                height: calc(100vh - 110px);
                padding: 12px;
                gap: 12px;
                background: var(--bg-primary);
                overflow: hidden;
            }
            .dash-v2-row {
                display: grid;
                grid-template-columns: repeat(12, 1fr);
                gap: 12px;
            }
            @media (max-width: 1400px) {
                #dash-v2-page-view { overflow-y: auto !important; height: auto !important; }
                .dash-v2-container {
                    height: auto;
                    min-height: unset;
                    overflow: visible;
                }
                .dash-v2-row .v2-panel { grid-column: span 12 !important; }
                .elastic-row-2, .elastic-row-3 { min-height: unset; flex-grow: unset; }
            }
            
            /* ROW 1: KPI STRIP */
            .kpi-row {
                flex-shrink: 0;
            }
            .kpi-grid {
                grid-column: span 12;
                display: flex;
                flex-wrap: nowrap;
                gap: 8px;
                overflow-x: auto;
                padding-bottom: 5px;
            }
            .kpi-card-v2 {
                background: var(--bg-surface);
                border: 1px solid var(--border-color);
                border-radius: 6px;
                height: 70px;
                padding: 8px 10px;
                display: flex;
                flex-direction: column;
                justify-content: space-between;
                position: relative;
                transition: transform 0.2s;
                flex: 1 0 0%;
                min-width: 90px;
            }
            @media (max-width: 1400px) {
                .kpi-grid { flex-wrap: wrap; overflow-x: hidden; }
                .kpi-card-v2 { flex: 1 1 calc(20% - 8px); min-width: 110px; }
            }
            @media (max-width: 1000px) { .kpi-card-v2 { flex: 1 1 calc(33% - 8px); } }
            @media (max-width: 600px) { .kpi-card-v2 { flex: 1 1 calc(50% - 8px); } }
            .kpi-card-v2:hover { transform: translateY(-2px); border-color: var(--accent); }
            
            .kpi-header-v2 {
                display: flex;
                justify-content: space-between;
                align-items: center;
                font-size: 0.65rem;
                font-weight: 700;
                color: var(--text-secondary);
                text-transform: uppercase;
                letter-spacing: 0.02em;
            }
            .kpi-value-v2 {
                font-size: 1.1rem;
                font-weight: 700;
                color: var(--text-primary);
                line-height: 1;
            }
            .kpi-spark-wrap {
                height: 18px;
                width: 100%;
            }
            .status-dot {
                width: 6px;
                height: 6px;
                border-radius: 50%;
                display: inline-block;
                margin-right: 4px;
            }
            .status-dot.healthy { background: var(--success); box-shadow: 0 0 5px var(--success); }
            .status-dot.warning { background: var(--warning); box-shadow: 0 0 5px var(--warning); }
            .status-dot.critical { background: var(--danger); box-shadow: 0 0 5px var(--danger); }

            /* ROW 2 & 3: ELASTIC GROWTH */
            .elastic-row-2 { flex-grow: 2; min-height: 380px; }
            .elastic-row-3 { flex-grow: 2; min-height: 300px; }

            .v2-panel {
                background: var(--bg-surface);
                border: 1px solid var(--border-color);
                border-radius: 6px;
                display: flex;
                flex-direction: column;
                overflow: hidden;
            }
            .v2-panel-header {
                padding: 8px 12px;
                border-bottom: 1px solid var(--border-color);
                display: flex;
                justify-content: space-between;
                align-items: center;
                background: rgba(255,255,255,0.02);
            }
            .v2-panel-header h3 {
                margin: 0;
                font-size: 0.75rem;
                font-weight: 700;
                color: var(--text-secondary);
                text-transform: uppercase;
            }
            .v2-panel-content {
                flex-grow: 1;
                position: relative;
                padding: 8px;
            }

            /* ACTIVE PROBLEMS REDESIGN */
            .problems-panel {
                display: flex;
                flex-direction: column;
                height: 100%;
            }
            .blocking-summary {
                height: 30%;
                display: grid;
                grid-template-columns: repeat(3, 1fr);
                gap: 8px;
                padding: 10px;
                background: rgba(239, 68, 68, 0.05);
                border-bottom: 1px solid var(--border-color);
            }
            .summary-tile {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                text-align: center;
            }
            .summary-tile .val { font-size: 1rem; font-weight: 700; color: var(--danger); }
            .summary-tile .lbl { font-size: 0.55rem; color: var(--text-muted); text-transform: uppercase; }

            .problems-tabs-wrap {
                height: 70%;
                display: flex;
                flex-direction: column;
            }
            .v2-tabs-mini {
                display: flex;
                gap: 2px;
                background: var(--bg-primary);
                padding: 4px 8px 0 8px;
            }
            .v2-tab-mini-btn {
                padding: 4px 10px;
                font-size: 0.65rem;
                border: none;
                background: transparent;
                color: var(--text-muted);
                cursor: pointer;
                border-radius: 4px 4px 0 0;
            }
            .v2-tab-mini-btn.active {
                background: var(--bg-surface);
                color: var(--accent);
                font-weight: 700;
            }

            /* TOOLTIP STYLES */
            .dba-tooltip {
                position: absolute;
                z-index: 1000;
                background: var(--bg-surface);
                border: 1px solid var(--accent);
                border-radius: 6px;
                padding: 12px;
                width: 280px;
                font-size: 0.75rem;
                box-shadow: 0 10px 25px rgba(0, 0, 0, 0.3);
                pointer-events: none;
                display: none;
                color: var(--text-primary);
            }
            .dba-tooltip h4 { margin: 0 0 8px 0; color: var(--accent); font-size: 0.85rem; border-bottom: 1px solid var(--border-color); padding-bottom: 5px; }
            .dba-tooltip .section { margin-top: 8px; line-height: 1.4; }
            .dba-tooltip .label { font-weight: 700; color: var(--text-secondary); margin-right: 4px; display: block; font-size: 0.65rem; text-transform: uppercase; }
            .dba-tooltip .content { display: block; }

            /* TempDB Mini KPIs */
            .tempdb-mini-kpis {
                display: grid;
                grid-template-columns: repeat(3, 1fr);
                gap: 5px;
                margin-bottom: 8px;
            }
            .mini-kpi {
                background: rgba(0,0,0,0.2);
                padding: 4px;
                border-radius: 3px;
                text-align: center;
            }
            .mini-kpi .v { font-size: 0.7rem; font-weight: 700; display: block; }
            .mini-kpi .l { font-size: 0.5rem; color: var(--text-muted); text-transform: uppercase; }

            .info-icon-clickable { cursor: help; color: var(--text-muted); font-size: 0.7rem; }
            .info-icon-clickable:hover { color: var(--accent); }
        </style>

        <div class="page-view active dashboard-sky-theme" style="padding:0; overflow:hidden; height:100%;" id="dash-v2-page-view">
            <div class="page-title flex-between" style="padding: 10px 20px; background: var(--bg-primary); border-bottom: 1px solid var(--border-color); height: 65px;">
                <div>
                    <h1 style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:10px;">
                        <i class="fa-solid fa-heart-pulse text-accent"></i>
                        SQL Server Dashboard
                        <i class="fa-solid fa-circle-info text-accent info-icon-clickable" style="font-size: 0.9rem;" data-metric="dashboard-info"></i>
                    </h1>
                    <div style="display:flex; gap:12px; align-items:center; margin-top:2px;">
                        <span style="font-size:0.7rem; color:var(--text-secondary); font-weight:600;"><i class="fa-solid fa-server" style="font-size:0.6rem;"></i> ${window.escapeHtml(inst.name)}</span>
                        <span style="font-size:0.7rem; color:var(--text-muted);"><i class="fa-solid fa-database" style="font-size:0.6rem;"></i> ${window.escapeHtml(window.appState.currentDatabase || inst.database || 'master')}</span>
                        <span id="v2-header-edition" style="font-size:0.65rem; background:rgba(0,0,0,0.1); padding:2px 6px; border-radius:4px; color:var(--text-muted);">--</span>
                        <span id="v2-header-uptime" style="font-size:0.65rem; color:var(--success); font-weight:600;"><i class="fa-solid fa-clock" style="font-size:0.6rem;"></i> --</span>
                    </div>
                </div>
                <div class="flex-between" style="align-items:center; gap:1rem;">
                    <div id="time-picker-insertion-point"></div>
                    <div class="text-muted" style="font-size:0.65rem; background: rgba(0,0,0,0.2); padding: 4px 8px; border-radius: 4px;">
                        Update: <span id="v2-last-update" class="text-accent">--:--:--</span>
                    </div>
                    <button class="btn btn-xs btn-accent" id="v2-refresh-btn"><i class="fa-solid fa-sync"></i></button>
                </div>
            </div>

            <div class="dash-v2-container">
                <!-- ROW 1: KPIs -->
                <div class="dash-v2-row kpi-row">
                    <div class="kpi-grid" id="v2-kpi-strip">
                        ${['cpu', 'runnable', 'memory', 'page-reads', 'log-wait', 'blocking', 'connections', 'batch-req', 'compilations', 'status'].map(id => `
                            <div class="kpi-card-v2" id="kpi-${id}">
                                <div class="kpi-header-v2">
                                    <span><span class="status-dot healthy" id="dot-${id}"></span><span id="label-${id}">---</span></span>
                                    <i class="fa-solid fa-info-circle info-icon-clickable" data-metric="${id}"></i>
                                </div>
                                <div class="kpi-value-v2" id="val-${id}">--</div>
                                <div class="kpi-spark-wrap"><canvas id="spark-${id}"></canvas></div>
                            </div>
                        `).join('')}
                    </div>
                </div>

                <!-- ROW 2: PRIMARY BOTTLENECKS -->
                <div class="dash-v2-row elastic-row-2">
                    <div class="v2-panel" style="grid-column: span 8;">
                        <div class="v2-panel-header">
                            <div style="display:flex; align-items:center; gap:8px;">
                                <h3><i class="fa-solid fa-hourglass-half text-warning"></i> Wait Stats Trend</h3>
                                <div id="summary-waits" style="font-size:0.6rem; color:var(--text-muted); font-weight:600; text-transform:uppercase;"></div>
                            </div>
                            <i class="fa-solid fa-info-circle info-icon-clickable" data-metric="waits"></i>
                        </div>
                        <div class="v2-panel-content">
                            <canvas id="v2-wait-chart"></canvas>
                        </div>
                    </div>
                    <div class="v2-panel" style="grid-column: span 4;">
                        <div class="v2-panel-header">
                            <div style="display:flex; align-items:center; gap:8px;">
                                <h3><i class="fa-solid fa-microchip text-accent"></i> IO Latency & IOPS</h3>
                                <div id="summary-io" style="font-size:0.6rem; color:var(--text-muted); font-weight:600; text-transform:uppercase;"></div>
                            </div>
                            <i class="fa-solid fa-info-circle info-icon-clickable" data-metric="io"></i>
                        </div>
                        <div class="v2-panel-content">
                            <canvas id="v2-io-chart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- ROW 3: SECONDARY TRIAGE -->
                <div class="dash-v2-row elastic-row-3">
                    <div class="v2-panel" style="grid-column: span 4;">
                        <div class="v2-panel-header">
                            <h3><i class="fa-solid fa-chart-line text-success"></i> Throughput vs Logins</h3>
                            <i class="fa-solid fa-info-circle info-icon-clickable" data-metric="throughput"></i>
                        </div>
                        <div class="v2-panel-content">
                            <canvas id="v2-throughput-chart"></canvas>
                        </div>
                    </div>
                    <div class="v2-panel" style="grid-column: span 4;">
                        <div class="v2-panel-header">
                            <h3><i class="fa-solid fa-database text-warning"></i> TempDB Breakdown</h3>
                            <i class="fa-solid fa-info-circle info-icon-clickable" data-metric="tempdb"></i>
                        </div>
                        <div class="v2-panel-content">
                            <div class="tempdb-mini-kpis">
                                <div class="mini-kpi"><span class="v" id="td-size">--</span><span class="l">Total Size</span></div>
                                <div class="mini-kpi"><span class="v" id="td-version">--</span><span class="l">Version Store</span></div>
                                <div class="mini-kpi"><span class="v" id="td-log">--</span><span class="l">Log Status</span></div>
                            </div>
                            <div style="position:relative; height:160px;"><canvas id="v2-tempdb-chart" style="position:absolute;inset:0;width:100%;height:100%;"></canvas></div>
                        </div>
                    </div>
                    <div class="v2-panel" style="grid-column: span 4;" id="panel-active-problems">
                        <div class="v2-panel-header">
                            <h3><i class="fa-solid fa-list-check text-accent"></i> Active Problems</h3>
                            <button class="btn btn-xs btn-outline" style="font-size:0.5rem;" id="v2-blocking-nav-btn">Blocking Monitor <i class="fa-solid fa-external-link"></i></button>
                        </div>
                        <div class="problems-panel">
                            <div class="blocking-summary">
                                <div class="summary-tile"><span class="val" id="pb-blocked">0</span><span class="lbl">Blocked</span></div>
                                <div class="summary-tile"><span class="val" id="pb-head" style="font-size:0.75rem;">None</span><span class="lbl">Head Blocker</span></div>
                                <div class="summary-tile"><span class="val" id="pb-duration">0s</span><span class="lbl">Max Wait</span></div>
                            </div>
                            <div class="problems-tabs-wrap">
                                <div class="v2-tabs-mini">
                                    <button class="v2-tab-mini-btn active" data-tab="long-running">LONG RUNNING</button>
                                    <button class="v2-tab-mini-btn" data-tab="blocking">BLOCKING TREE</button>
                                    <button class="v2-tab-mini-btn" data-tab="metrics">METRIC SNAPSHOT</button>
                                </div>
                                <div class="v2-panel-content" id="v2-problems-table" style="padding:0; overflow-y:auto;"></div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            <div id="dba-tooltip-container" class="dba-tooltip"></div>
        </div>
    `;

    // Wire buttons that were rendered into innerHTML (inline onclick removed for CSP compliance)
    document.getElementById('v2-refresh-btn')?.addEventListener('click', () => window.SqlServerHealthV2View());
    document.getElementById('v2-blocking-nav-btn')?.addEventListener('click', () => window.appNavigate('sqlserver-locks'));

    // Initialize DBA Tooltips
    initDBATooltips();

    // Setup Refresh Logic
    window.refreshDashboardData = async () => {
        try {
            const instIdx = window.appState.currentInstanceIdx;
            const inst = window.appState.config?.instances?.[instIdx];
            if (!inst) return;

            // Convert picker values (local) to ISO strings (UTC) for backend
            const from = window.appState.fromTs ? new Date(window.appState.fromTs).toISOString() : "";
            const to = window.appState.toTs ? new Date(window.appState.toTs).toISOString() : "";
            
            const url = `/api/sqlserver/health-v2?instance=${encodeURIComponent(inst.name)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;
            const response = await window.apiClient.authenticatedFetch(url);
            if (response.ok) {
                const data = await response.json();
                renderV2Dashboard(data);
                
                // Dynamically adjust refresh rate based on backend configuration
                if (data.refresh_interval_ms && window.setDashboardRefresh) {
                    window.setDashboardRefresh(data.refresh_interval_ms);
                }
            }
        } catch (e) { console.warn("[V2] Auto-refresh failed", e); }
    };

    window.cleanupDashboard = () => {
        if (window.dashboardRefreshInterval) {
            clearInterval(window.dashboardRefreshInterval);
            window.dashboardRefreshInterval = null;
        }
    };

    window.setDashboardRefresh = (ms) => {
        if (ms < 10000) ms = 10000; // Floor at 10s
        if (window.dashboardRefreshInterval) {
            // Only reset if interval changed significantly
            return; 
        }
        window.dashboardRefreshInterval = window.registerInterval(window.refreshDashboardData, ms);
    };

    // Set initial default refresh rate
    const dynamicInterval = window.collectorConfig ? window.collectorConfig.getInterval("SQL Server Health KPIs", 30000) : 30000;
    window.setDashboardRefresh(dynamicInterval);

    if (window.initPageTimePicker) window.initPageTimePicker();

    try {
        const from = window.appState.fromTs ? new Date(window.appState.fromTs).toISOString() : "";
        const to = window.appState.toTs ? new Date(window.appState.toTs).toISOString() : "";
        const url = `/api/sqlserver/health-v2?instance=${encodeURIComponent(inst.name)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;
        const response = await window.apiClient.authenticatedFetch(url);
        if (!response.ok) throw new Error("API error: " + response.status);
        const data = await response.json();

        renderV2Dashboard(data);
    } catch (err) {
        console.error("[V2] Load failed", err);
        // Ensure UI is populated with empty state
        renderV2Dashboard({ kpis: {}, problems: { long_running: [], blocking: [] } });
    }
};

function renderV2Dashboard(data) {
    document.getElementById('v2-last-update').textContent = new Date().toLocaleTimeString();
    
    if (data.kpis) {
        if (document.getElementById('v2-header-edition')) document.getElementById('v2-header-edition').textContent = data.kpis.edition || 'Unknown Edition';
        if (document.getElementById('v2-header-uptime')) document.getElementById('v2-header-uptime').innerHTML = `<i class="fa-solid fa-clock" style="font-size:0.6rem;"></i> ${data.kpis.uptime || '--'}`;
    }

    // Fallbacks
    const k = {
        sql_cpu_pct: 0, runnable_tasks: 0, mem_grants_pending: 0, 
        page_reads_per_sec: 0, log_write_wait_ms: 0, blocked_sessions: 0, 
        user_connections: 0, batch_requests: 0, compilations: 0, 
        instance_status: 'Healthy',
        ...(data.kpis || {})
    };
    const waitData = data.wait_trends || [];
    const ioData = data.io_latency || [];
    const tpData = data.throughput || [];
    const tdData = data.tempdb || { user_obj_mb: 0, internal_obj_mb: 0, version_store_mb: 0, free_mb: 0, contention_found: false };

    renderKPIs(k, waitData, ioData, tpData);
    initWaitChart(waitData);
    initIOChart(ioData);
    initThroughputChart(tpData);
    initTempDBChart(tdData);
    renderProblems(data.problems, 'long-running', k);

    document.querySelectorAll('.v2-tab-mini-btn').forEach(btn => {
        btn.onclick = (e) => {
            document.querySelectorAll('.v2-tab-mini-btn').forEach(b => b.classList.remove('active'));
            e.target.classList.add('active');
            renderProblems(data.problems, e.target.dataset.tab, k);
        };
    });
}

function initDBATooltips() {
    if (window.v2TooltipsInitialized) return;
    window.v2TooltipsInitialized = true;

    const tooltipData = {
        'cpu': { 
            title: 'SQL CPU Utilization', 
            def: 'The percentage of total processor capacity used by the sqlservr.exe process across all logical cores.', 
            why: 'High CPU leads to request queuing, increased latencies, and transaction timeouts. It can be caused by inefficient execution plans or insufficient hardware.', 
            healthy: 'Below 70%', worry: 'Sustained > 90%' 
        },
        'runnable': { 
            title: 'Runnable Tasks', 
            def: 'The count of worker threads that are ready to execute but are waiting in the runnable queue for an available scheduler (CPU core).', 
            why: 'Indicates CPU starvation (signal wait). High values mean your CPU is saturated and cannot keep up with the task load.', 
            healthy: '< 10', worry: 'Sustained > 20' 
        },
        'memory': { 
            title: 'Memory Grants Pending', 
            def: 'The number of processes waiting for a workspace memory grant to start execution (RESOURCE_SEMAPHORE).', 
            why: 'Direct evidence of severe memory pressure. Queries cannot even start, causing massive application stalls.', 
            healthy: '0', worry: '> 1' 
        },
        'page-reads': { 
            title: 'Physical Page Reads', 
            def: 'Number of 8KB pages read from disk per second because they were not found in the Buffer Pool (data cache).', 
            why: 'High values indicate "cold cache" or buffer pool churn. Usually suggests missing indexes or insufficient RAM for the working set.', 
            healthy: 'Low baseline', worry: 'Sustained high values' 
        },
        'log-wait': { 
            title: 'Log Write Wait', 
            def: 'Average duration in milliseconds that threads spent waiting for transaction log flushes to reach disk (WRITELOG).', 
            why: 'Transaction throughput is limited by the speed of the log disk. High latency here slows down every INSERT/UPDATE/DELETE.', 
            healthy: '< 2ms', worry: '> 5ms' 
        },
        'blocking': { 
            title: 'Blocked Sessions', 
            def: 'The count of active sessions waiting for a lock held by another session (exclusive or shared).', 
            why: 'Blocking creates a waterfall effect where one slow query stops hundreds of others. Leads to application "freezing".', 
            healthy: '0', worry: '> 5' 
        },
        'connections': { 
            title: 'Active Connections', 
            def: 'Total number of client processes currently connected to the SQL Server instance.', 
            why: 'Excessive connections waste memory and increase context switching. Spikes may indicate connection leaks in the application.', 
            healthy: 'Stable', worry: 'Rapid growth or > 80% capacity' 
        },
        'batch-req': { 
            title: 'Batch Requests/sec', 
            def: 'The number of T-SQL batches received by the server per second. The primary metric for throughput.', 
            why: 'Used to measure workload volume. A sudden drop while CPU is high indicates internal contention or blocking.', 
            healthy: 'Baseline dependent', worry: 'Unexpected deviation' 
        },
        'compilations': { 
            title: 'SQL Compilations/sec', 
            def: 'How many times per second SQL Server is forced to create a new execution plan.', 
            why: 'Compilations are CPU-intensive. High rates suggest plan cache instability or excessive use of ad-hoc, non-parameterized queries.', 
            healthy: '< 10% of Batch Req', worry: '> 25% of Batch Req' 
        },
        'status': { 
            title: 'Instance Health', 
            def: 'An aggregate health score based on CPU, Memory, IO Latency, and Concurrency metrics.', 
            why: 'Provides a single "pulse" for the server to determine if immediate DBA intervention is required.', 
            healthy: 'Healthy', worry: 'Critical or Warning' 
        },
        'waits': { 
            title: 'Wait Stats Analysis', 
            def: 'Breakdown of where SQL Server threads are spending time (CPU, IO, Locks, Parallelism).', 
            why: 'Tells you exactly WHY the server is slow. It eliminates guesswork by pointing to the specific bottleneck category.', 
            healthy: 'Balanced distribution', worry: 'One category dominating > 60%' 
        },
        'io': { 
            title: 'IO Latency Analysis', 
            def: 'Measure of disk responsiveness for Data (MDF) and Log (LDF) files in milliseconds.', 
            why: 'Disk is the slowest component. If latency is high, everything else will be slow regardless of how much you tune the SQL.', 
            healthy: 'Data < 20ms, Log < 5ms', worry: 'Data > 50ms, Log > 10ms' 
        },
        'throughput': { 
            title: 'Throughput vs Concurrency', 
            def: 'Correlation between workload (Batch Requests) and resource usage (Connections/Logins).', 
            why: 'Helps identify if performance issues are caused by "more users" or "less efficiency".', 
            healthy: 'Linear correlation', worry: 'Requests drop as connections rise' 
        },
        'tempdb': { 
            title: 'TempDB Breakdown Definitions', 
            def: '<strong>USER:</strong> Temporary tables, table variables, and cursors.<br><strong>INT:</strong> Internal objects like hash joins and sorts.<br><strong>VERS:</strong> Version store for snapshot isolation/RCSI.<br><strong>FREE:</strong> Unallocated space available.', 
            why: 'TempDB is shared by ALL databases. Running out of space or high contention here crashes the entire instance.', 
            healthy: 'Free > 20%', worry: 'Version store growth' 
        },
        'dashboard-info': {
            title: 'SQL Server Dashboard',
            def: 'An advanced real-time observability cockpit designed for DBAs to perform instant triage of performance bottlenecks.',
            why: 'Consolidates Wait Stats, IO Latency, Throughput, and Active Problems into a single high-density view for rapid root-cause analysis.',
            healthy: 'Standardized UI', worry: 'Elastic Grid'
        }
    };

    document.addEventListener('mouseover', (e) => {
        const icon = e.target.closest('.info-icon-clickable');
        if (!icon) return;

        const container = document.getElementById('dba-tooltip-container');
        if (!container) return;

        const metric = icon.dataset.metric;
        const data = tooltipData[metric];
        if (!data) return;

        container.innerHTML = `
            <h4>${data.title}</h4>
            <div class="section"><span class="label">Definition</span><span class="content">${data.def}</span></div>
            <div class="section"><span class="label">Impact & Why it matters</span><span class="content">${data.why}</span></div>
            <div class="section" style="display:flex; gap:15px; border-top:1px solid var(--border-color); padding-top:8px;">
                <div><span class="label">Healthy</span><span class="content text-success">${data.healthy}</span></div>
                <div><span class="label">Worry</span><span class="content text-danger">${data.worry}</span></div>
            </div>
        `;
        
        const rect = icon.getBoundingClientRect();
        
        // Ensure display is block before measuring offsetParent
        container.style.display = 'block';
        const offsetParent = container.offsetParent || document.body;
        const containerRect = offsetParent.getBoundingClientRect();
        
        // Smarter Positioning relative to offsetParent
        const tooltipWidth = 280;
        const padding = 15;
        
        // Calculate horizontal position relative to offsetParent
        let left = (rect.left - containerRect.left) + (rect.width / 2) - (tooltipWidth / 2);
        
        // Right bound check (viewport based)
        if (rect.left + (rect.width / 2) + (tooltipWidth / 2) > window.innerWidth - padding) {
            left = (window.innerWidth - containerRect.left) - tooltipWidth - padding;
        }
        
        // Left bound check (viewport based)
        if (rect.left + (rect.width / 2) - (tooltipWidth / 2) < padding) {
            left = padding - containerRect.left;
        }

        container.style.top = (rect.bottom - containerRect.top + 8) + 'px';
        container.style.left = left + 'px';
    });

    document.addEventListener('mouseout', (e) => {
        const container = document.getElementById('dba-tooltip-container');
        if (container && e.target.closest('.info-icon-clickable')) {
            container.style.display = 'none';
        }
    });
}

function renderKPIs(k, waits, ios, tps) {
    const items = [
        { id: 'cpu', label: 'SQL CPU', value: k.sql_cpu_pct.toFixed(1) + '%', status: k.sql_cpu_pct > 80 ? 'critical' : k.sql_cpu_pct > 60 ? 'warning' : 'healthy', series: waits.map(w => w.cpu) },
        { id: 'runnable', label: 'Runnable', value: k.runnable_tasks, status: k.runnable_tasks > 15 ? 'critical' : k.runnable_tasks > 5 ? 'warning' : 'healthy', series: waits.map(w => w.cpu * 0.7) },
        { id: 'memory', label: 'Grants', value: k.mem_grants_pending, status: k.mem_grants_pending > 0 ? 'warning' : 'healthy', series: waits.map(w => w.memory) },
        { id: 'page-reads', label: 'Page Reads', value: k.page_reads_per_sec.toFixed(0), status: 'healthy', series: ios.map(i => i.read_iops) },
        { id: 'log-wait', label: 'Log Wait', value: k.log_write_wait_ms.toFixed(1) + 'ms', status: k.log_write_wait_ms > 5 ? 'warning' : 'healthy', series: ios.map(i => i.log_write_ms) },
        { id: 'blocking', label: 'Blocked', value: k.blocked_sessions, status: k.blocked_sessions > 0 ? 'warning' : 'healthy', series: waits.map(w => w.locking) },
        { id: 'connections', label: 'Conns', value: k.user_connections, status: 'healthy', series: tps.map(t => t.connections) },
        { id: 'batch-req', label: 'Batch Req', value: k.batch_requests.toFixed(0), status: 'healthy', series: tps.map(t => t.batch_requests) },
        { id: 'compilations', label: 'Compiles', value: k.compilations.toFixed(0), status: 'healthy', series: tps.map(t => t.batch_requests * 0.05) },
        { id: 'status', label: 'Health', value: k.instance_status, status: k.instance_status.toLowerCase(), series: waits.map(w => 100 - w.cpu) }
    ];

    items.forEach(item => {
        const valEl = document.getElementById('val-' + item.id);
        const lblEl = document.getElementById('label-' + item.id);
        const dotEl = document.getElementById('dot-' + item.id);
        
        if (valEl) valEl.textContent = item.value;
        if (lblEl) lblEl.textContent = item.label;
        if (dotEl) dotEl.className = 'status-dot ' + item.status;
        
        initSparkline('spark-' + item.id, item.series, item.status === 'critical' ? '#ef4444' : item.status === 'warning' ? '#f59e0b' : '#3b82f6');
    });
}

function initSparkline(id, data, color) {
    const canvas = document.getElementById(id);
    if (!canvas) return;
    
    // Destroy existing chart if it exists in window.v2Charts
    if (window.v2Charts[id]) {
        window.v2Charts[id].destroy();
    }

    const ctx = canvas.getContext('2d');
    window.v2Charts[id] = new Chart(ctx, {
        type: 'line',
        data: { labels: data.map((_, i) => i), datasets: [{ data: data, borderColor: color, borderWidth: 1.5, fill: false, pointRadius: 0, tension: 0.4 }] },
        options: { responsive: true, maintainAspectRatio: false, scales: { x: { display: false }, y: { display: false } }, plugins: { legend: { display: false }, tooltip: { enabled: false } } }
    });
}

function initWaitChart(trends) {
    // Summary
    const last = trends[trends.length - 1] || {};
    const total = (last.cpu || 0) + (last.io || 0) + (last.memory || 0) + (last.locking || 0) + (last.parallel || 0);
    const cats = { 'CPU': last.cpu, 'IO': last.io, 'MEM': last.memory, 'LCK': last.locking, 'PX': last.parallel };
    const top = Object.keys(cats).reduce((a, b) => cats[a] > cats[b] ? a : b);
    document.getElementById('summary-waits').textContent = `Total: ${total.toFixed(0)} ms/s | Top: ${top}`;

    const canvas = document.getElementById('v2-wait-chart');
    if (!canvas) return;

    if (window.v2Charts['wait']) {
        window.v2Charts['wait'].destroy();
    }

    const ctx = canvas.getContext('2d');
    window.v2Charts['wait'] = new Chart(ctx, {
        type: 'line',
        data: {
            labels: trends.map(t => new Date(t.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })),
            datasets: [
                { label: 'CPU', data: trends.map(t => t.cpu), backgroundColor: 'rgba(59, 130, 246, 0.2)', fill: true, borderColor: '#3b82f6', tension: 0.3, pointRadius: 0, borderWidth: 2 },
                { label: 'IO', data: trends.map(t => t.io), backgroundColor: 'rgba(16, 185, 129, 0.2)', fill: true, borderColor: '#10b981', tension: 0.3, pointRadius: 0, borderWidth: 2 },
                { label: 'Memory', data: trends.map(t => t.memory), backgroundColor: 'rgba(245, 158, 11, 0.2)', fill: true, borderColor: '#f59e0b', tension: 0.3, pointRadius: 0 },
                { label: 'Locking', data: trends.map(t => t.locking), backgroundColor: 'rgba(239, 68, 68, 0.2)', fill: true, borderColor: '#ef4444', tension: 0.3, pointRadius: 0 },
                { label: 'Parallel', data: trends.map(t => t.parallel), backgroundColor: 'rgba(139, 92, 246, 0.15)', fill: true, borderColor: '#8b5cf6', tension: 0.3, pointRadius: 0 }
            ]
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            layout: { padding: { bottom: 12 } },
            interaction: { mode: 'index', intersect: false },
            scales: {
                x: { stacked: true, grid: { display: false }, ticks: { font: { size: 9 }, maxRotation: 0, color: '#6c757d', autoSkip: true, maxTicksLimit: 8 } },
                y: { stacked: true, beginAtZero: true, grid: { color: 'rgba(255,255,255,0.03)' }, ticks: { font: { size: 9 }, color: '#6c757d' } }
            },
            plugins: { 
                legend: { 
                    display: true, position: 'top', align: 'end',
                    labels: { boxWidth: 8, font: { size: 8 }, padding: 4, color: '#6c757d' } 
                },
                tooltip: { backgroundColor: 'rgba(0,0,0,0.8)', titleFont: { size: 10 }, bodyFont: { size: 10 } }
            }
        }
    });
}

function initIOChart(data) {
    // Summary
    const last = data[data.length - 1] || {};
    document.getElementById('summary-io').textContent = `D: ${(last.data_read_ms || 0).toFixed(1)}ms | L: ${(last.log_write_ms || 0).toFixed(1)}ms | IOPS: ${(last.read_iops || 0) + (last.write_iops || 0)}`;

    const canvas = document.getElementById('v2-io-chart');
    if (!canvas) return;

    if (window.v2Charts['io']) {
        window.v2Charts['io'].destroy();
    }

    const ctx = canvas.getContext('2d');
    window.v2Charts['io'] = new Chart(ctx, {
        type: 'line',
        data: {
            labels: data.map(d => new Date(d.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })),
            datasets: [
                { label: 'Read Lat', data: data.map(d => d.data_read_ms), borderColor: '#3b82f6', tension: 0.2, pointRadius: 0, borderWidth: 1.5 },
                { label: 'Write Lat', data: data.map(d => d.data_write_ms), borderColor: '#10b981', tension: 0.2, pointRadius: 0, borderWidth: 1.5 },
                { label: 'Log Lat', data: data.map(d => d.log_write_ms), borderColor: '#f59e0b', tension: 0.2, pointRadius: 0, borderWidth: 1.5 },
                { label: 'IOPS', data: data.map(d => d.read_iops + d.write_iops), borderColor: '#6c757d', borderDash: [2, 2], yAxisID: 'y1', tension: 0.2, pointRadius: 0, borderWidth: 1 }
            ]
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            layout: { padding: { bottom: 12 } },
            interaction: { mode: 'index', intersect: false },
            scales: {
                x: { grid: { display: false }, ticks: { font: { size: 9 }, color: '#6c757d', autoSkip: true, maxTicksLimit: 6 } },
                y: { type: 'linear', display: true, position: 'left', grid: { color: 'rgba(255,255,255,0.03)' }, ticks: { font: { size: 9 }, color: '#6c757d' }, title: { display: false } },
                y1: { type: 'linear', display: true, position: 'right', grid: { display: false }, ticks: { font: { size: 9 }, color: '#6c757d' }, title: { display: false } }
            },
            plugins: { 
                legend: { position: 'top', align: 'end', labels: { boxWidth: 6, font: { size: 8 }, padding: 4 } },
                tooltip: { backgroundColor: 'rgba(0,0,0,0.8)', titleFont: { size: 10 }, bodyFont: { size: 10 } }
            }
        }
    });
}

function initThroughputChart(data) {
    const canvas = document.getElementById('v2-throughput-chart');
    if (!canvas) return;

    if (window.v2Charts['tp']) {
        window.v2Charts['tp'].destroy();
    }

    const ctx = canvas.getContext('2d');
    window.v2Charts['tp'] = new Chart(ctx, {
        type: 'line',
        data: {
            labels: data.map(d => new Date(d.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })),
            datasets: [
                { label: 'Batch Req', data: data.map(d => d.batch_requests), borderColor: '#3b82f6', tension: 0.3, pointRadius: 0, borderWidth: 1.5 },
                { label: 'Conns', data: data.map(d => d.connections), borderColor: '#10b981', tension: 0.3, pointRadius: 0, borderWidth: 1.5 },
                { label: 'Logins/s', data: data.map(d => d.logins_per_sec), borderColor: '#f59e0b', borderDash: [2,2], tension: 0.3, pointRadius: 0, borderWidth: 1 }
            ]
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            layout: { padding: { bottom: 12 } },
            interaction: { mode: 'index', intersect: false },
            scales: {
                x: { grid: { display: false }, ticks: { font: { size: 9 }, maxRotation: 0, autoSkip: true, maxTicksLimit: 6 } },
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { font: { size: 9 } } }
            },
            plugins: { 
                legend: { position: 'top', align: 'end', labels: { boxWidth: 6, font: { size: 8 }, padding: 4 } } 
            }
        }
    });
}

function initTempDBChart(t) {
    // Update mini-KPIs first
    const total = t.user_obj_mb + t.internal_obj_mb + t.version_store_mb + t.free_mb;
    document.getElementById('td-size').textContent = (total / 1024).toFixed(1) + 'GB';
    document.getElementById('td-version').textContent = (total > 0 ? ((t.version_store_mb / total) * 100) : 0).toFixed(1) + '%';
    document.getElementById('td-log').textContent = 'Healthy'; // Simplified for now as log usage isn't in t

    const canvas = document.getElementById('v2-tempdb-chart');
    if (!canvas) return;

    if (window.v2Charts['tempdb']) {
        window.v2Charts['tempdb'].destroy();
    }

    const ctx = canvas.getContext('2d');
    window.v2Charts['tempdb'] = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: ['User', 'Int', 'Vers', 'Free'],
            datasets: [{ label: 'MB', data: [t.user_obj_mb, t.internal_obj_mb, t.version_store_mb, t.free_mb], backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#4a5568'], borderRadius: 2 }]
        },
        options: {
            indexAxis: 'y', responsive: true, maintainAspectRatio: false,
            layout: { padding: { bottom: 12 } },
            scales: { 
                x: { display: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { font: { size: 8 } } }, 
                y: { grid: { display: false }, ticks: { font: { size: 9 }, color: '#adb5bd' } } 
            },
            plugins: { 
                legend: { display: false },
                tooltip: { backgroundColor: 'rgba(0,0,0,0.8)', titleFont: { size: 10 }, bodyFont: { size: 10 } }
            }
        }
    });
}

function renderProblems(problems, tab, kpis) {
    // 1. Update Blocking Summary (independent of current tab)
    const blockingList = (problems && problems.blocking) || [];
    const blockedCount = blockingList.length;
    let headBlocker = 'None';
    let maxWait = 0;

    if (blockedCount > 0) {
        // Simple logic to find head blocker (session that blocks others but is not blocked)
        const blockedBySet = new Set(blockingList.map(b => b.blocking_session_id));
        const sessionSet = new Set(blockingList.map(b => b.session_id));
        for (let b of blockingList) {
            if (blockedBySet.has(b.session_id) && !sessionSet.has(b.blocking_session_id)) {
                headBlocker = b.session_id;
                break;
            }
        }
        if (headBlocker === 'None') headBlocker = blockingList[0].blocking_session_id;
        maxWait = Math.max(...blockingList.map(b => b.wait_duration_ms || 0)) / 1000;
    }

    document.getElementById('pb-blocked').textContent = blockedCount;
    document.getElementById('pb-head').textContent = headBlocker;
    document.getElementById('pb-duration').textContent = maxWait.toFixed(1) + 's';

    // 3. Dynamic Panel Color
    const panel = document.getElementById('panel-active-problems');
    if (panel) {
        panel.style.transition = 'background 0.3s ease';
        if (blockedCount > 0) {
            panel.style.background = 'rgba(239, 68, 68, 0.08)'; // Light red
        } else if ((problems.long_running || []).length > 2) {
            panel.style.background = 'rgba(245, 158, 11, 0.08)'; // Amber
        } else {
            panel.style.background = 'rgba(16, 185, 129, 0.05)'; // Light green
        }
    }

    // 2. Render Table
    const wrap = document.getElementById('v2-problems-table');
    
    if (tab === 'metrics' && kpis) {
        let html = `<table class="modern-table" style="font-size:0.65rem; width:100%; border-collapse:collapse;">
            <thead style="background:rgba(0,0,0,0.2); position:sticky; top:0;">
                <tr><th style="padding:6px; text-align:left;">Metric</th><th style="padding:6px; text-align:right;">Current Value</th></tr>
            </thead>
            <tbody>`;
        const metrics = [
            { n: 'SQL CPU', v: (kpis.sql_cpu_pct || 0).toFixed(1) + '%' },
            { n: 'Runnable Tasks', v: kpis.runnable_tasks || 0 },
            { n: 'Pending Memory Grants', v: kpis.mem_grants_pending || 0 },
            { n: 'Page Reads/sec', v: (kpis.page_reads_per_sec || 0).toFixed(0) },
            { n: 'Log Write Wait (ms)', v: (kpis.log_write_wait_ms || 0).toFixed(1) },
            { n: 'Blocked Sessions', v: kpis.blocked_sessions || 0 },
            { n: 'User Connections', v: kpis.user_connections || 0 },
            { n: 'Batch Requests/sec', v: (kpis.batch_requests || 0).toFixed(0) },
            { n: 'SQL Compilations/sec', v: (kpis.compilations || 0).toFixed(0) }
        ];
        metrics.forEach(m => {
            html += `<tr style="border-bottom:1px solid var(--border-color);">
                <td style="padding:6px; font-weight:700; color:var(--text-secondary);">${m.n}</td>
                <td style="padding:6px; text-align:right; font-family:monospace; color:var(--accent);">${m.v}</td>
            </tr>`;
        });
        wrap.innerHTML = html + '</tbody></table>';
        return;
    }

    if (!problems) { wrap.innerHTML = '<div class="text-muted p-2">No data.</div>'; return; }
    
    let list = tab === 'long-running' ? (problems.long_running || []) : blockingList;
    
    if (list.length === 0) { 
        wrap.innerHTML = `<div style="display:flex; flex-direction:column; align-items:center; justify-content:center; height:100%; color:var(--text-muted); font-size:0.7rem; padding: 20px;">
            <i class="fa-solid fa-check-circle text-success" style="font-size:1.2rem; margin-bottom:5px;"></i>
            No ${tab === 'long-running' ? 'slow queries' : 'blocking'} detected.
        </div>`;
        return; 
    }
    
    let html = `<table class="modern-table" style="font-size:0.65rem; width:100%; border-collapse:collapse;">
        <thead style="background:rgba(0,0,0,0.2); position:sticky; top:0;">
            <tr>
                <th style="padding:6px; text-align:left;">SPID</th>
                <th style="padding:6px; text-align:left;">${tab === 'long-running' ? 'Duration' : 'Blocked By'}</th>
                <th style="padding:6px; text-align:left;">Database</th>
                <th style="padding:6px; text-align:left;">Query</th>
            </tr>
        </thead>
        <tbody>`;
    
    list.forEach((item, idx) => {
        const secondary = tab === 'long-running' ? ((item.total_elapsed_time_ms / 1000).toFixed(1) + 's') : (item.blocking_session_id || '??');
        html += `<tr style="border-bottom:1px solid var(--border-color);">
            <td style="padding:6px;"><span class="badge ${tab === 'long-running' ? 'badge-accent' : 'badge-danger'}" style="font-size:0.6rem;">${item.session_id}</span></td>
            <td style="padding:6px; font-weight:700;">${secondary}</td>
            <td style="padding:6px; opacity:0.8;">${window.escapeHtml(item.database_name || 'master')}</td>
            <td style="padding:6px;" class="sql-preview v2-clickable-query" style="cursor:pointer;" data-idx="${idx}" title="Click to view full query">
                ${window.truncate(item.query_text, 35)}
            </td>
        </tr>`;
    });
    wrap.innerHTML = html + '</tbody></table>';

    // Bind click events
    wrap.querySelectorAll('.v2-clickable-query').forEach(el => {
        el.onclick = () => {
            const idx = el.dataset.idx;
            const query = list[idx].query_text;
            if (window.showQueryModal) {
                window.showQueryModal(query);
            } else {
                console.log('Query:', query);
            }
        };
    });
}

function generateDummyPoints(count, type) {
    const points = [];
    const now = Date.now();
    for (let i = count; i >= 0; i--) {
        const ts = new Date(now - i * 60000).toISOString();
        if (type === 'wait') points.push({ timestamp: ts, cpu: 20+Math.random()*20, io: 5+Math.random()*10, memory: Math.random()*5, locking: Math.random()*2, parallel: 40+Math.random()*40, other: 5 });
        else if (type === 'io') points.push({ timestamp: ts, data_read_ms: 1+Math.random()*3, data_write_ms: 2+Math.random()*5, log_write_ms: 0.5+Math.random(), read_iops: 200+Math.random()*200, write_iops: 100+Math.random()*100 });
        else if (type === 'throughput') points.push({ timestamp: ts, batch_requests: 600+Math.random()*400, connections: 120, logins_per_sec: Math.random()*2 });
    }
    return points;
}
