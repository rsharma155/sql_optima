/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Wait Statistics Dashboard (V2) - Interactive Analytics.
 *          Inline HTML, HealthDashboardV2 CSS patterns, loading states, live DMV fallback.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.WaitStatsV2View = async function () {
    appDebug('[WaitStatsV2] Starting Initialization');

    const outlet = window.routerOutlet;
    if (!outlet) return;

    const inst = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'sqlserver') {
        outlet.innerHTML = '<div class="alert alert-warning m-3">Please select a SQL Server instance first.</div>';
        return;
    }

    // ── Destroy any previously created charts ──────────────────────────────────
    if (window._wsV2Charts) {
        Object.values(window._wsV2Charts).forEach(c => { try { c.destroy(); } catch (_) {} });
    }
    window._wsV2Charts = {};

    // ── Inline HTML shell ──────────────────────────────────────────────────────
    outlet.innerHTML = `
<style>
    /* ── WaitStats V2 ─────────────────────────────────────────────────── */
    .ws-kpi-strip {
        display: flex;
        flex-wrap: nowrap;
        gap: 8px;
        overflow-x: auto;
        padding-bottom: 4px;
        margin-bottom: 0.5rem;
    }
    .ws-kpi-strip::-webkit-scrollbar { height: 3px; }
    .ws-kpi-strip::-webkit-scrollbar-thumb { background: var(--border-color); border-radius: 2px; }
    .kpi-card-v2 {
        background: var(--bg-surface);
        border: 1px solid var(--border-color);
        border-radius: 6px;
        height: 70px;
        padding: 8px 10px;
        display: flex;
        flex-direction: column;
        justify-content: space-between;
        flex: 1 0 0%;
        min-width: 130px;
        transition: transform .2s;
    }
    .kpi-card-v2:hover { transform: translateY(-2px); border-color: var(--accent); }
    .kpi-header-v2 { display:flex; justify-content:space-between; align-items:center; font-size:0.62rem; font-weight:700; color:var(--text-secondary); text-transform:uppercase; letter-spacing:.04em; }
    .kpi-value-v2 { font-size: 1.1rem; font-weight: 700; color: var(--text-primary); line-height: 1; }
    .kpi-sub-v2 { font-size: 0.58rem; color: var(--text-muted); margin-top: 2px; }
    /* Chart panels */
    .ws-chart-panel {
        background: var(--bg-surface);
        border: 1px solid var(--border-color);
        border-radius: 6px;
        overflow: hidden;
    }
    .ws-panel-header {
        padding: 6px 12px;
        border-bottom: 1px solid var(--border-color);
        display: flex;
        justify-content: space-between;
        align-items: center;
        background: rgba(255,255,255,.02);
    }
    .ws-panel-header h3 { margin:0; font-size:0.7rem; font-weight:700; color:var(--text-secondary); text-transform:uppercase; letter-spacing:.04em; }
    .ws-chart-body { position: relative; }
    /* Progress bar */
    .ws-progress-wrap { display: flex; align-items: center; gap: 6px; margin-top: 3px; }
    .ws-progress { flex: 1; height: 3px; border-radius: 2px; background: var(--border-color); overflow: hidden; }
    .ws-progress-bar { height: 100%; border-radius: 2px; transition: width .4s; }
    /* Tables */
    .ws-table { width: 100%; border-collapse: collapse; font-size: 0.72rem; }
    .ws-table th { font-size: 0.62rem; text-transform: uppercase; color: var(--text-muted); padding: 4px 8px; border-bottom: 1px solid var(--border-color); text-align: left; font-weight: 600; position: sticky; top: 0; z-index: 2; background: var(--bg-surface); }
    .ws-table td { padding: 4px 8px; border-bottom: 1px solid rgba(255,255,255,.04); vertical-align: middle; }
    .ws-table tbody tr:hover { background: rgba(255,255,255,.03); }
    /* Top-wait-type KPI: smaller value font to show long wait type names */
    .kpi-card-top-type .kpi-value-v2 { font-size: 0.68rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .kpi-card-top-type .kpi-header-v2 > span:first-child { white-space: normal; line-height: 1.3; }
    /* Badges */
    .ws-badge { display: inline-block; padding: 1px 6px; border-radius: 3px; font-size: 0.6rem; font-weight: 700; }
    /* Placeholder overlays */
    .ws-placeholder {
        position: absolute; inset: 0;
        display: flex; flex-direction: column;
        align-items: center; justify-content: center;
        gap: 6px; font-size: 0.72rem; color: var(--text-muted);
        background: var(--bg-surface);
    }
    .ws-spinner {
        width: 18px; height: 18px;
        border: 2px solid var(--border-color);
        border-top-color: var(--accent);
        border-radius: 50%;
        animation: ws-spin .8s linear infinite;
    }
    @keyframes ws-spin { to { transform: rotate(360deg); } }
    @media (max-width: 900px) {
        .ws-grid-2, .ws-grid-3-2 { grid-template-columns: 1fr !important; }
    }
</style>

<div class="page-view active dashboard-sky-theme" style="opacity:1 !important; pointer-events:auto !important; min-height:100%;">

    <!-- ── PAGE HEADER ────────────────────────────────────────────────────── -->
    <div class="dashboard-page-title-compact flex-between">
        <div>
            <h1 style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:8px;">
                <i class="fa-solid fa-hourglass-half text-accent"></i>
                Wait Statistics Analyzer
                <i class="fa-solid fa-circle-info text-accent" style="font-size:0.85rem; cursor:help;" title="Wait Statistics show where SQL Server threads spend time waiting. High signal waits indicate CPU pressure; high resource waits indicate IO, locking or memory pressure. Use the trend chart to spot regressions and the Top Wait Types table to identify the dominant bottleneck."></i>
            </h1>
            <span id="ws-inst-name" class="subtitle"></span>
        </div>
        <div style="display:flex;align-items:center;gap:6px;flex-wrap:wrap;">
            <div class="glass-panel" style="padding:0.2rem 0.5rem;display:flex;align-items:center;gap:0.4rem;font-size:0.7rem;border:1px solid var(--border-color);flex-wrap:nowrap;">
                <label class="text-muted" style="margin:0;font-weight:600;text-transform:uppercase;font-size:0.6rem;">From</label>
                <input type="datetime-local" id="from-ts" step="1" style="background:transparent;border:none;color:inherit;font-size:0.7rem;width:9.5rem;padding:0;" />
                <label class="text-muted" style="margin:0;font-weight:600;text-transform:uppercase;font-size:0.6rem;">To</label>
                <input type="datetime-local" id="to-ts" step="1" style="background:transparent;border:none;color:inherit;font-size:0.7rem;width:9.5rem;padding:0;" />
                <button id="apply-time-btn" class="btn btn-xs btn-accent" style="padding:2px 8px;min-width:auto;" title="Apply Range">Apply</button>
            </div>
        </div>
    </div>

    <!-- ── KPI STRIP ──────────────────────────────────────────────────────── -->
    <div class="ws-kpi-strip" id="ws-kpi-strip">
        ${[
            { id: 'ws-kpi-signal',   tip: 'Signal Wait % = time threads spend waiting for a CPU scheduler slot. > 20% indicates CPU pressure; > 40% is critical.' },
            { id: 'ws-kpi-total',    tip: 'Total wait time accumulated by all threads in the selected time window across all wait categories.' },
            { id: 'ws-kpi-resource', tip: 'Resource Wait = total wait minus signal wait. Represents time spent waiting for IO, locks, memory or network — not CPU.' },
            { id: 'ws-kpi-tasks',    tip: 'Average number of tasks waiting at any given snapshot. High values alongside long waits indicate saturation.' },
            { id: 'ws-kpi-top-type', tip: 'The single wait type accumulating the most total wait time in the selected window. This is your primary bottleneck.' },
        ].map(({ id, tip }, idx) => `
        <div class="kpi-card-v2${idx === 4 ? ' kpi-card-top-type' : ''}" title="${tip}">
            <div class="kpi-header-v2"><span id="${id}-label">...</span><span id="${id}-icon" style="font-size:.75rem;"></span></div>
            <div class="kpi-value-v2" id="${id}-val">—</div>
            <div class="kpi-sub-v2" id="${id}-sub"></div>
        </div>`).join('')}
    </div>

    <!-- ── ROW 1: Trend Chart + Distribution Donut ────────────────────────── -->
    <div class="ws-grid-3-2" style="display:grid; grid-template-columns:3fr 2fr; gap:10px; margin-bottom:10px;">
        <div class="ws-chart-panel" title="Stacked area chart showing wait time per category over the selected window. Spikes in a single category reveal the root bottleneck (e.g. IO spike = disk saturation).">
            <div class="ws-panel-header">
                <h3>Wait Time Trend</h3>
                <span id="ws-trend-range" style="font-size:0.62rem;color:var(--text-muted);"></span>
            </div>
            <div class="ws-chart-body" style="height:230px;">
                <canvas id="chart-ws-trend" style="position:absolute;inset:0;width:100%;height:100%;"></canvas>
                <div class="ws-placeholder" id="ph-trend"><div class="ws-spinner"></div><span>Loading…</span></div>
            </div>
        </div>
        <div class="ws-chart-panel" title="Donut chart showing proportional wait time across CPU, IO, Memory, Locking, Parallelism and Other categories. Largest slice is the primary bottleneck type.">
            <div class="ws-panel-header"><h3>Wait Distribution</h3></div>
            <div class="ws-chart-body" style="height:230px;">
                <canvas id="chart-ws-pie" style="position:absolute;inset:0;width:100%;height:100%;"></canvas>
                <div class="ws-placeholder" id="ph-pie"><div class="ws-spinner"></div><span>Loading…</span></div>
            </div>
        </div>
    </div>

    <!-- ── ROW 2: CPU Pressure + Database Impact ────────────────────────── -->
    <div class="ws-grid-2" style="display:grid; grid-template-columns:1fr 1fr; gap:10px; margin-bottom:10px;">
        <div class="ws-chart-panel" title="Signal Wait (time waiting for CPU) vs. SOS_SCHEDULER_YIELD (time yielding CPU). High values in both indicate heavy CPU saturation.">
            <div class="ws-panel-header"><h3>CPU Pressure Trend</h3></div>
            <div class="ws-chart-body" style="height:220px;">
                <canvas id="chart-ws-cpu-pressure" style="position:absolute;inset:0;width:100%;height:100%;"></canvas>
                <div class="ws-placeholder" id="ph-cpu-pressure"><div class="ws-spinner"></div><span>Loading…</span></div>
            </div>
        </div>
        <div class="ws-chart-panel" title="Wait time impact per database. Identifies which database is responsible for the majority of the wait time.">
            <div class="ws-panel-header"><h3>Database Wait Impact</h3></div>
            <div class="ws-chart-body" style="height:220px;">
                <canvas id="chart-ws-db-impact" style="position:absolute;inset:0;width:100%;height:100%;"></canvas>
                <div class="ws-placeholder" id="ph-db-impact"><div class="ws-spinner"></div><span>Loading…</span></div>
            </div>
        </div>
    </div>

    <!-- ── ROW 3: Heatmap + Top Wait Types table ──────────────────────────── -->
    <div class="ws-grid-2" style="display:grid; grid-template-columns:1fr 1fr; gap:10px; margin-bottom:10px;">
        <div class="ws-chart-panel" title="Bar heatmap of total wait time aggregated by day over the last 7 days. Darker bars indicate heavier wait pressure. Useful for spotting recurring busy periods.">
            <div class="ws-panel-header"><h3>Daily Wait Heatmap</h3><span style="font-size:0.62rem;color:var(--text-muted);">7 days</span></div>
            <div class="ws-chart-body" style="height:250px;">
                <canvas id="chart-ws-heatmap" style="position:absolute;inset:0;width:100%;height:100%;"></canvas>
                <div class="ws-placeholder" id="ph-heatmap"><div class="ws-spinner"></div><span>Loading…</span></div>
            </div>
        </div>
        <div class="ws-chart-panel" title="Ranked list of individual wait types ordered by total wait time. The ℹ icon on each row shows a description of the wait type and the recommended action to address it.">
            <div class="ws-panel-header"><h3>Top Wait Types</h3><span id="ws-top-count" style="font-size:0.62rem;color:var(--accent);"></span></div>
            <div style="height:250px; overflow-y:auto;">
                <table class="ws-table">
                    <thead><tr>
                        <th>Wait Type</th><th>Category</th><th>Total</th><th>Avg</th><th>% Share</th><th></th>
                    </tr></thead>
                    <tbody id="tbody-top-waits">
                        <tr><td colspan="6" class="text-center" style="padding:20px;color:var(--text-muted);font-size:.7rem;">
                            <div class="ws-spinner" style="margin:0 auto 6px;"></div>Loading…</td></tr>
                    </tbody>
                </table>
            </div>
        </div>
    </div>

    <!-- ── ROW 4: Blocking Tree ────────────────────────────────────────────── -->
    <div class="ws-chart-panel" style="margin-bottom:10px;" title="Visual representation of the blocking hierarchy. Root nodes are the head blockers causing the chain.">
        <div class="ws-panel-header">
            <h3>Blocking Hierarchy</h3>
            <span style="font-size:0.62rem;color:var(--text-muted);">Most-recent snapshot in selected window</span>
        </div>
        <div id="ws-blocking-tree-container" style="min-height:150px; padding:10px; overflow-x:auto;">
            <div class="ws-placeholder" id="ph-blocking-tree"><div class="ws-spinner"></div><span>Loading…</span></div>
        </div>
    </div>

    <!-- ── ROW 5: Active Waits ─────────────────────────────────────────────── -->
    <div class="ws-chart-panel" style="margin-bottom:10px;" title="Sessions waiting at the most-recent collector snapshot within the selected time window. Blocker column shows the blocking SPID if any. Use the time picker to navigate historical windows.">
        <div class="ws-panel-header">
            <h3>Active Wait Sessions <span id="ws-active-count" style="color:var(--accent);margin-left:4px;font-size:0.65rem;"></span></h3>
            <span style="font-size:0.62rem;color:var(--text-muted);">Latest stored snapshot in selected window</span>
        </div>
        <div style="max-height:280px; overflow-y:auto;">
            <table class="ws-table">
                <thead><tr>
                    <th>Time</th><th>Session</th><th>Wait Type</th><th>Duration</th><th>Blocker</th><th>Database</th><th>Login</th><th>Query</th>
                </tr></thead>
                <tbody id="tbody-active-waits">
                    <tr><td colspan="8" class="text-center" style="padding:20px;color:var(--text-muted);font-size:.7rem;">
                        <div class="ws-spinner" style="margin:0 auto 6px;"></div>Loading…</td></tr>
                </tbody>
            </table>
        </div>
    </div>

</div>`;

    // ── Wire instance name ─────────────────────────────────────────────────────
    document.getElementById('ws-inst-name').textContent = `· ${inst.name}`;

    // ── Pre-fill time inputs from shared appState or default to last 1h ────────
    const fromEl = document.getElementById('from-ts');
    const toEl   = document.getElementById('to-ts');
    {
        const _fmt = window.formatDateTimeLocalInput || ((d) => {
            const _pad = n => n.toString().padStart(2, '0');
            const x = d instanceof Date ? d : new Date(d);
            return `${x.getFullYear()}-${_pad(x.getMonth()+1)}-${_pad(x.getDate())}T${_pad(x.getHours())}:${_pad(x.getMinutes())}:${_pad(x.getSeconds())}`;
        });
        const _now = new Date();
        const _ago = new Date(_now.getTime() - 3600000);
        if (fromEl) fromEl.value = window.appState.fromTs || _fmt(_ago);
        if (toEl)   toEl.value   = window.appState.toTs   || _fmt(_now);
        window.appState.fromTs = fromEl?.value || window.appState.fromTs;
        window.appState.toTs = toEl?.value || window.appState.toTs;
    }

    // ── Helper: format milliseconds ────────────────────────────────────────────
    function fmtMs(ms) {
        if (!ms || ms <= 0) return '0ms';
        if (ms < 1000)  return Math.round(ms) + 'ms';
        if (ms < 60000) return (ms / 1000).toFixed(1) + 's';
        if (ms < 3600000) return (ms / 60000).toFixed(1) + 'm';
        return (ms / 3600000).toFixed(2) + 'h';
    }

    // ── Category colour palette ────────────────────────────────────────────────
    const CAT_COLORS = {
        CPU:         '#3b82f6',
        IO_DATA:     '#10b981',
        IO_LOG:      '#f59e0b',
        LOCKING:     '#ef4444',
        MEMORY:      '#8b5cf6',
        PARALLELISM: '#06b6d4',
        NETWORK:     '#14b8a6',
        OTHER:       '#6b7280',
        // lowercase keys from trend data
        cpu:         '#3b82f6',
        io:          '#10b981',
        memory:      '#8b5cf6',
        locking:     '#ef4444',
        parallel:    '#06b6d4',
        other:       '#6b7280',
    };
    function catColor(k) { return CAT_COLORS[k] || '#6b7280'; }

    // ── Show / hide placeholder ────────────────────────────────────────────────
    function showPlaceholder(id, msg, isSpinner = false) {
        const el = document.getElementById(id);
        if (!el) return;
        el.style.display = 'flex';
        el.innerHTML = isSpinner
            ? `<div class="ws-spinner"></div><span>${msg}</span>`
            : `<i class="fa-solid fa-chart-column" style="font-size:1.2rem;opacity:.3;"></i><span>${msg}</span>`;
    }
    function hidePlaceholder(id) {
        const el = document.getElementById(id);
        if (el) el.style.display = 'none';
    }

    // ── Render KPI strip ──────────────────────────────────────────────────────
    function renderKPIs(kpis) {
        const k = kpis || {};
        const signalPct = k.signal_wait_pct || 0;
        const totalMs   = k.total_wait_time_ms || 0;
        const resMs     = k.resource_wait_time_ms || (totalMs - (k.signal_wait_time_ms || 0));
        const tasks     = k.total_waiting_tasks || 0;
        const restart   = k.restart_detected || false;

        // Signal Wait %
        setKPI('ws-kpi-signal',
            'Signal Wait %',
            signalPct.toFixed(1) + '%',
            signalPct > 40 ? 'CPU pressure ⚠' : signalPct > 20 ? 'Moderate' : 'Healthy',
            signalPct > 40 ? '⚠' : '✓',
            signalPct > 40 ? 'var(--danger)' : signalPct > 20 ? 'var(--warning)' : 'var(--success)'
        );
        // Total Wait
        setKPI('ws-kpi-total',   'Total Wait',     fmtMs(totalMs),  'in selected window', '⏱');
        // Resource Wait
        setKPI('ws-kpi-resource','Resource Wait',  fmtMs(resMs),    'non-CPU wait time',  '🔒');
        // Waiting Tasks
        setKPI('ws-kpi-tasks',   'Waiting Tasks',  tasks.toLocaleString(), 'avg tasks waiting', '📊');
        
        // Top Wait Type / Restart
        if (restart) {
            setKPI('ws-kpi-top-type', 'Restart Detected', 'YES', 'SQL Server restarted', '⚠', 'var(--danger)');
        } else {
            setKPI('ws-kpi-top-type', 'Top Wait Category', k.top_wait_category || '—', 'primary bottleneck', '🔍');
        }
    }

    // ── Render CPU Pressure chart ─────────────────────────────────────────────
    function renderCPUPressure(cpu) {
        hidePlaceholder('ph-cpu-pressure');
        if (!cpu || cpu.length === 0) {
            showPlaceholder('ph-cpu-pressure', 'No CPU pressure data');
            return;
        }

        const ctx = document.getElementById('chart-ws-cpu-pressure');
        if (!ctx) return;

        const labels = cpu.map(p => {
            const d = new Date(p.timestamp);
            return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        });

        window._wsV2Charts.cpuPressure = new Chart(ctx, {
            type: 'line',
            data: {
                labels,
                datasets: [
                    {
                        label: 'Signal Wait (ms)',
                        data: cpu.map(p => p.signal_wait_ms),
                        borderColor: '#ef4444',
                        backgroundColor: '#ef444422',
                        borderWidth: 2,
                        fill: true,
                        tension: 0.3
                    },
                    {
                        label: 'Scheduler Yields (ms)',
                        data: cpu.map(p => p.scheduler_yield_ms),
                        borderColor: '#3b82f6',
                        backgroundColor: 'transparent',
                        borderWidth: 2,
                        fill: false,
                        tension: 0.3
                    }
                ]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                animation: false,
                scales: {
                    x: { ticks: { font: { size: 9 }, maxTicksLimit: 10, color: 'var(--text-muted)' }, grid: { display: false } },
                    y: { beginAtZero: true, ticks: { font: { size: 9 }, color: 'var(--text-muted)' }, grid: { color: 'rgba(255,255,255,.06)' } }
                },
                plugins: {
                    legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 }, color: 'var(--text-secondary)' } }
                }
            }
        });
    }

    // ── Render Database Impact chart ──────────────────────────────────────────
    function renderDatabaseImpact(impact) {
        hidePlaceholder('ph-db-impact');
        if (!impact || impact.length === 0) {
            showPlaceholder('ph-db-impact', 'No database impact data');
            return;
        }

        const ctx = document.getElementById('chart-ws-db-impact');
        if (!ctx) return;

        window._wsV2Charts.dbImpact = new Chart(ctx, {
            type: 'bar',
            data: {
                labels: impact.map(i => i.database_name),
                datasets: [{
                    label: 'Wait Time (ms)',
                    data: impact.map(i => i.total_wait_ms),
                    backgroundColor: '#8b5cf6',
                    borderRadius: 4
                }]
            },
            options: {
                indexAxis: 'y',
                responsive: true, maintainAspectRatio: false,
                animation: false,
                scales: {
                    x: { beginAtZero: true, ticks: { font: { size: 9 }, color: 'var(--text-muted)' }, grid: { color: 'rgba(255,255,255,.06)' } },
                    y: { ticks: { font: { size: 9 }, color: 'var(--text-muted)' }, grid: { display: false } }
                },
                plugins: {
                    legend: { display: false }
                }
            }
        });
    }

    // ── Render Blocking Tree ──────────────────────────────────────────────────
    function renderBlockingTree(tree) {
        const container = document.getElementById('ws-blocking-tree-container');
        if (!container) return;
        hidePlaceholder('ph-blocking-tree');

        if (!tree || tree.length === 0) {
            container.innerHTML = `<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;height:80px;gap:6px;color:var(--success);font-size:0.76rem;"><i class="fa-solid fa-circle-check" style="font-size:1.2rem;opacity:0.7;"></i><span>No blocking chains detected — instance is healthy</span></div>`;
            return;
        }

        const escape = window.escapeHtml || (s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'));

        function buildNodeHTML(node, level = 0) {
            const indent = level * 20;
            const hasChildren = node.children && node.children.length > 0;
            const color = node.blocking_session_id === 0 ? '#ef4444' : 'var(--text-primary)';
            const qText = escape(node.query_text || '').slice(0, 100);
            
            let html = `
            <div style="margin-left:${indent}px; border-left:1px solid var(--border-color); padding:4px 10px; margin-bottom:4px; background:rgba(255,255,255,0.02); border-radius:4px;">
                <div style="display:flex; align-items:center; gap:8px;">
                    <span style="font-weight:700; color:${color}; font-size:0.75rem;">SPID ${node.session_id}</span>
                    <span class="badge" style="background:var(--bg-surface); border:1px solid var(--border-color); font-size:0.6rem;">${escape(node.wait_type)}</span>
                    <span style="font-size:0.7rem; color:var(--text-muted);">${node.wait_duration_ms.toLocaleString()} ms</span>
                    <span style="font-size:0.7rem; color:var(--accent); font-weight:600;">[${escape(node.database_name)}]</span>
                </div>
                <div style="font-size:0.65rem; color:var(--text-muted); margin-top:2px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;" title="${escape(node.query_text)}">
                    <code>${qText}${node.query_text && node.query_text.length > 100 ? '...' : ''}</code>
                </div>
            </div>`;

            if (hasChildren) {
                node.children.forEach(child => {
                    html += buildNodeHTML(child, level + 1);
                });
            }
            return html;
        }

        container.innerHTML = tree.map(root => buildNodeHTML(root)).join('');
    }

    function setKPI(prefix, label, val, sub, icon, iconColor) {
        const lEl  = document.getElementById(prefix + '-label');
        const vEl  = document.getElementById(prefix + '-val');
        const sEl  = document.getElementById(prefix + '-sub');
        const iEl  = document.getElementById(prefix + '-icon');
        if (lEl) lEl.textContent = label;
        if (vEl) vEl.textContent = val;
        if (sEl) sEl.textContent = sub || '';
        if (iEl) { iEl.textContent = icon || ''; if (iconColor) iEl.style.color = iconColor; }
    }

    // ── Render stacked-area trend chart ───────────────────────────────────────
    function renderTrend(trends) {
        hidePlaceholder('ph-trend');
        if (!trends || trends.length === 0) {
            showPlaceholder('ph-trend', 'No trend data in selected window');
            return;
        }

        const ctx = document.getElementById('chart-ws-trend');
        if (!ctx) return;

        const labels = trends.map(t => {
            const d = new Date(t.timestamp);
            return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        });

        const cats = [
            { key: 'cpu',      label: 'CPU' },
            { key: 'io',       label: 'IO' },
            { key: 'memory',   label: 'Memory' },
            { key: 'locking',  label: 'Locking' },
            { key: 'parallel', label: 'Parallelism' },
            { key: 'other',    label: 'Other' },
        ];

        const datasets = cats.map(c => ({
            label: c.label,
            data: trends.map(t => Math.round(t[c.key] || 0)),
            backgroundColor: catColor(c.key) + '55',
            borderColor: catColor(c.key),
            borderWidth: 1.5,
            fill: true,
            pointRadius: trends.length > 60 ? 0 : 2,
            tension: 0.3,
        }));

        // Range label
        const rangeEl = document.getElementById('ws-trend-range');
        if (rangeEl && trends.length) {
            const d0 = new Date(trends[0].timestamp);
            const d1 = new Date(trends[trends.length - 1].timestamp);
            const mins = Math.round((d1 - d0) / 60000);
            rangeEl.textContent = mins < 120 ? `${mins}m window` : `${(mins/60).toFixed(1)}h window`;
        }

        window._wsV2Charts.trend = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                animation: false,
                interaction: { mode: 'index', intersect: false },
                scales: {
                    x: { ticks: { font: { size: 9 }, maxTicksLimit: 10, color: 'var(--text-muted)' }, grid: { display: false } },
                    y: { stacked: true, beginAtZero: true,
                         ticks: { font: { size: 9 }, color: 'var(--text-muted)', callback: v => fmtMs(v) },
                         grid: { color: 'rgba(255,255,255,.06)' } }
                },
                plugins: {
                    legend: { display: true, position: 'top',
                              labels: { boxWidth: 8, font: { size: 9 }, color: 'var(--text-secondary)' } },
                    tooltip: {
                        callbacks: { label: ctx2 => ` ${ctx2.dataset.label}: ${fmtMs(ctx2.raw)}` }
                    }
                }
            }
        });
    }

    // ── Render donut — shows top individual wait types ────────────────────────
    const PIE_COLORS = [
        '#3b82f6','#10b981','#f59e0b','#ef4444','#8b5cf6',
        '#06b6d4','#f43f5e','#14b8a6','#a855f7','#84cc16',
        '#fb923c','#e879f9','#38bdf8','#4ade80','#fbbf24',
    ];

    function renderPie(topWaits) {
        hidePlaceholder('ph-pie');
        if (!topWaits || topWaits.length === 0) {
            showPlaceholder('ph-pie', 'No data yet');
            return;
        }

        const ctx = document.getElementById('chart-ws-pie');
        if (!ctx) return;

        // Show top 14 individual wait types + aggregate the rest as "Other"
        const sorted = [...topWaits].sort((a, b) => (b.wait_time_ms || 0) - (a.wait_time_ms || 0));
        const TOP_N = 14;
        const top = sorted.slice(0, TOP_N);
        const rest = sorted.slice(TOP_N);
        const otherMs = rest.reduce((s, w) => s + (w.wait_time_ms || 0), 0);

        const labels = top.map(w => w.wait_type || 'UNKNOWN');
        const values = top.map(w => w.wait_time_ms || 0);
        const colors = top.map((_, i) => PIE_COLORS[i % PIE_COLORS.length]);

        if (otherMs > 0) {
            labels.push('Other');
            values.push(otherMs);
            colors.push('#6b7280');
        }

        window._wsV2Charts.pie = new Chart(ctx, {
            type: 'doughnut',
            data: {
                labels,
                datasets: [{ data: values, backgroundColor: colors, borderWidth: 0, hoverOffset: 4 }]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                cutout: '55%',
                animation: false,
                plugins: {
                    legend: {
                        position: 'right',
                        labels: { boxWidth: 8, font: { size: 8 }, color: 'var(--text-secondary)', padding: 5 }
                    },
                    tooltip: {
                        callbacks: { label: ctx2 => ` ${ctx2.label}: ${fmtMs(ctx2.raw)}` }
                    }
                }
            }
        });
    }

    // ── Render heatmap bar chart ──────────────────────────────────────────────
    function renderHeatmap(daily) {
        hidePlaceholder('ph-heatmap');
        if (!daily || daily.length === 0) {
            showPlaceholder('ph-heatmap', 'No daily data yet');
            return;
        }

        const ctx = document.getElementById('chart-ws-heatmap');
        if (!ctx) return;

        const labels = daily.map(t => {
            const d = new Date(t.timestamp);
            return d.toLocaleDateString([], { month: 'short', day: 'numeric', hour: '2-digit' });
        });
        const totals = daily.map(t =>
            (t.cpu || 0) + (t.io || 0) + (t.memory || 0) + (t.locking || 0) + (t.parallel || 0) + (t.other || 0)
        );
        const max = Math.max(...totals, 1);

        window._wsV2Charts.heatmap = new Chart(ctx, {
            type: 'bar',
            data: {
                labels,
                datasets: [{
                    label: 'Total Wait',
                    data: totals,
                    backgroundColor: totals.map(v => `rgba(59, 130, 246, ${0.18 + (v / max) * 0.82})`),
                    borderRadius: 3,
                }]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                animation: false,
                scales: {
                    x: { ticks: { font: { size: 8 }, color: 'var(--text-muted)', maxTicksLimit: 14 }, grid: { display: false } },
                    y: { beginAtZero: true, display: false }
                },
                plugins: {
                    legend: { display: false },
                    tooltip: { callbacks: { label: ctx2 => ` ${fmtMs(ctx2.raw)}` } }
                }
            }
        });
    }

    // ── Top Wait Types sort state ─────────────────────────────────────────────
    let _topWaitSortCol = 'wait_time_ms';
    let _topWaitSortDir = 'desc';
    let _topWaitData    = [];

    function sortTopWaits(col) {
        if (_topWaitSortCol === col) {
            _topWaitSortDir = _topWaitSortDir === 'desc' ? 'asc' : 'desc';
        } else {
            _topWaitSortCol = col;
            _topWaitSortDir = 'desc';
        }
        renderTopWaitsTable();
    }

    function renderTopWaitsTable() {
        const tbody = document.getElementById('tbody-top-waits');
        if (!tbody || !_topWaitData.length) return;

        const sorted = [..._topWaitData].sort((a, b) => {
            const av = a[_topWaitSortCol] || 0;
            const bv = b[_topWaitSortCol] || 0;
            return _topWaitSortDir === 'desc' ? bv - av : av - bv;
        });

        function thIcon(col) {
            if (_topWaitSortCol !== col) return '<i class="fa-solid fa-sort" style="font-size:.55rem;opacity:.35;margin-left:3px;"></i>';
            return _topWaitSortDir === 'desc'
                ? '<i class="fa-solid fa-sort-down" style="font-size:.55rem;color:var(--accent);margin-left:3px;"></i>'
                : '<i class="fa-solid fa-sort-up" style="font-size:.55rem;color:var(--accent);margin-left:3px;"></i>';
        }

        // Re-render headers with sort indicators
        const thead = tbody.closest('table')?.querySelector('thead tr');
        if (thead) {
            thead.innerHTML = `
                <th style="cursor:default;">Wait Type</th>
                <th style="cursor:default;">Category</th>
                <th style="cursor:pointer;" data-sort-col="wait_time_ms">Total${thIcon('wait_time_ms')}</th>
                <th style="cursor:pointer;" data-sort-col="avg_wait_ms">Avg${thIcon('avg_wait_ms')}</th>
                <th style="cursor:pointer;" data-sort-col="percent_of_total">% Share${thIcon('percent_of_total')}</th>
                <th></th>`;
            thead.querySelectorAll('th[data-sort-col]').forEach(th => {
                th.addEventListener('click', () => sortTopWaits(th.dataset.sortCol));
            });
        }

        tbody.innerHTML = sorted.map(w => {
            const color  = catColor(w.category || 'OTHER');
            const pct    = (w.percent_of_total || 0).toFixed(1);
            const avg    = (w.avg_wait_ms || 0).toFixed(1);
            const desc   = window.escapeHtml ? window.escapeHtml(w.description || '') : (w.description || '');
            const action = window.escapeHtml ? window.escapeHtml(w.recommended_action || '') : (w.recommended_action || '');
            return `
            <tr>
                <td style="font-weight:600;font-size:.7rem;">${w.wait_type || ''}</td>
                <td><span class="ws-badge" style="background:${color}22;color:${color};">${w.category || 'OTHER'}</span></td>
                <td style="font-size:.7rem;">${fmtMs(w.wait_time_ms || 0)}</td>
                <td style="font-size:.7rem;">${avg}ms</td>
                <td>
                    <div class="ws-progress-wrap">
                        <span style="font-size:.65rem;min-width:30px;">${pct}%</span>
                        <div class="ws-progress"><div class="ws-progress-bar" style="width:${pct}%;background:${color};"></div></div>
                    </div>
                </td>
                <td>${desc ? `<span title="${desc}&#10;&#10;Action: ${action}" style="cursor:help;font-size:.75rem;color:var(--accent);">ℹ</span>` : ''}</td>
            </tr>`;
        }).join('');
    }

    window._wsV2SortTopWaits = sortTopWaits;

    // ── Render Top Wait Types table ────────────────────────────────────────────
    function renderTopWaits(top) {
        const tbody = document.getElementById('tbody-top-waits');
        const countEl = document.getElementById('ws-top-count');
        if (!tbody) return;

        if (!top || top.length === 0) {
            tbody.innerHTML = `<tr><td colspan="6" style="text-align:center;padding:20px;color:var(--text-muted);font-size:.7rem;">
                No wait data in selected window. Collector runs every 5 min.</td></tr>`;
            if (countEl) countEl.textContent = '';
            return;
        }

        if (countEl) countEl.textContent = `Top ${top.length}`;

        // Update "Top Wait Type" KPI
        const topRow = top[0];
        setKPI('ws-kpi-top-type', 'Top Wait Type', topRow.wait_type || '—',
            topRow.category || '', '🔍');

        _topWaitData = top;
        renderTopWaitsTable();
    }

    // ── Render Active Waits table ─────────────────────────────────────────────
    function renderActiveWaits(active) {
        const tbody   = document.getElementById('tbody-active-waits');
        const countEl = document.getElementById('ws-active-count');
        if (!tbody) return;

        const count = active ? active.length : 0;
        if (countEl) countEl.textContent = count > 0 ? `(${count})` : '';

        if (!active || active.length === 0) {
            tbody.innerHTML = `<tr><td colspan="7" style="text-align:center;padding:18px;color:var(--text-muted);font-size:.7rem;">
                No active waits detected — server is healthy or data not yet collected.</td></tr>`;
            return;
        }

        const escape = window.escapeHtml || (s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'));

        tbody.innerHTML = active.map((s, idx) => {
            const color = catColor(s.wait_category || s.wait_type || 'OTHER');
            const cacheKey = `ws_active_${idx}`;
            if (!window.appState.queryCache) window.appState.queryCache = {};
            window.appState.queryCache[cacheKey] = s.query_text || '';
            const hasQuery = !!(s.query_text && s.query_text.trim());
            const ts = s.capture_timestamp ? new Date(s.capture_timestamp).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit', second:'2-digit'}) : '—';
            return `
            <tr style="cursor:${hasQuery ? 'pointer' : 'default'};" title="${hasQuery ? 'Click to view full query' : ''}" data-ws-query-key="${hasQuery ? cacheKey : ''}">
                <td style="color:var(--text-muted); font-size:0.65rem;">${ts}</td>
                <td style="font-weight:600;">${s.session_id}</td>
                <td><span style="font-size:.68rem;font-weight:700;color:${color};">${escape(s.wait_type || '')}</span></td>
                <td style="font-size:.7rem;">${fmtMs(s.wait_duration_ms || 0)}</td>
                <td style="font-size:.7rem;">${s.blocking_session_id != null ? `<span style="color:var(--danger);font-weight:700;">${s.blocking_session_id}</span>` : '<span style="color:var(--text-muted);">—</span>'}</td>
                <td style="font-size:.7rem;">${escape(s.database_name || '')}</td>
                <td style="font-size:.7rem;">${escape(s.login_name || '')}</td>
                <td style="max-width:240px;">
                    ${hasQuery ? `<code style="font-size:.62rem;display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--accent);" title="${escape(s.query_text)}">${escape(s.query_text)}</code>` : '<span style="color:var(--text-muted);font-size:.62rem;">—</span>'}
                </td>
            </tr>`;
        }).join('');

        // Wire click handler for query detail modal
        tbody.querySelectorAll('tr[data-ws-query-key]').forEach(row => {
            const key = row.getAttribute('data-ws-query-key');
            if (!key) return;
            row.addEventListener('click', () => {
                const qt = window.appState.queryCache?.[key];
                if (!qt) return;
                if (typeof window.showQueryModal === 'function') {
                    window.showQueryModal(qt);
                } else {
                    const esc2 = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
                    const id = 'wsQueryModal_' + Date.now();
                    const el = document.createElement('div');
                    el.id = id;
                    el.style.cssText = 'position:fixed;z-index:99999;inset:0;background:rgba(0,0,0,0.75);display:flex;align-items:center;justify-content:center;';
                    el.innerHTML = `<div style="background:var(--bg-surface);border:1px solid var(--border-color);border-radius:10px;padding:1.25rem;max-width:800px;width:92%;max-height:80vh;overflow:auto;">
                        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.75rem;">
                            <h3 style="margin:0;font-size:0.9rem;">Query Text — SPID ${escape(String(row.children[1]?.textContent || ''))}</h3>
                            <button onclick="document.getElementById('${id}').remove()" style="background:none;border:none;font-size:1.3rem;cursor:pointer;color:var(--text-muted);">&times;</button>
                        </div>
                        <pre style="white-space:pre-wrap;font-size:0.78rem;margin:0;overflow-x:auto;">${esc2(qt)}</pre>
                    </div>`;
                    document.body.appendChild(el);
                    el.addEventListener('click', e => { if (e.target === el) el.remove(); });
                }
            });
            row.addEventListener('mouseenter', () => { row.style.background = 'rgba(255,255,255,0.05)'; });
            row.addEventListener('mouseleave', () => { row.style.background = ''; });
        });
    }

    // ── Main data-load function ────────────────────────────────────────────────
    async function loadData() {
        // Show loading spinners in placeholders
        showPlaceholder('ph-trend',   'Loading…', true);
        showPlaceholder('ph-pie',     'Loading…', true);
        showPlaceholder('ph-cpu-pressure', 'Loading…', true);
        showPlaceholder('ph-db-impact', 'Loading…', true);
        showPlaceholder('ph-blocking-tree', 'Loading…', true);
        showPlaceholder('ph-heatmap', 'Loading…', true);

        // Destroy any existing charts before re-drawing
        Object.values(window._wsV2Charts).forEach(c => { try { c.destroy(); } catch (_) {} });
        window._wsV2Charts = {};

        const _safeIso = (val, fallback) => {
            if (!val) return fallback;
            const iso = window.localDateTimeToISO ? window.localDateTimeToISO(val) : '';
            if (iso) return iso;
            const d = new Date(val);
            return isNaN(d.getTime()) ? fallback : d.toISOString();
        };
        if (fromEl?.value) window.appState.fromTs = fromEl.value;
        if (toEl?.value) window.appState.toTs = toEl.value;
        const from = _safeIso(fromEl?.value, new Date(Date.now() - 3600000).toISOString());
        const to   = _safeIso(toEl?.value,   new Date().toISOString());

        let data = null;
        try {
            const resp = await window.apiClient.authenticatedFetch(
                `/api/sqlserver/wait-stats/dashboard?instance=${encodeURIComponent(inst.name)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
            );

            if (!resp.ok) {
                throw new Error(`HTTP ${resp.status}`);
            }
            data = await resp.json();
        } catch (e) {
            appError('[WaitStatsV2] fetch error:', e);
            showPlaceholder('ph-trend',   'Failed to load data');
            showPlaceholder('ph-pie',     'Failed to load data');
            showPlaceholder('ph-cpu-pressure', 'Failed to load data');
            showPlaceholder('ph-db-impact', 'Failed to load data');
            showPlaceholder('ph-blocking-tree', 'Failed to load data');
            showPlaceholder('ph-heatmap', 'Failed to load data');
            const errRow = `<tr><td colspan="7" style="text-align:center;padding:12px;color:var(--danger);font-size:.7rem;">Error: ${escape(e.message)}</td></tr>`;
            const t1 = document.getElementById('tbody-top-waits');
            const t2 = document.getElementById('tbody-active-waits');
            if (t1) t1.innerHTML = errRow;
            if (t2) t2.innerHTML = errRow;
            return;
        }

        const timescaleReady = data.timescale_ready !== false; // undefined → true (normal response)
        const noData = !timescaleReady;

        // KPIs
        renderKPIs(data.kpis);

        // Charts
        const hourly      = data.wait_trends_hourly || [];
        const daily       = data.wait_trends_daily  || [];
        const top         = data.top_wait_types      || [];
        const active      = data.active_waits        || [];
        const cpuPressure = data.cpu_pressure        || [];
        const dbImpact    = data.database_impact     || [];
        const blocking    = data.blocking_tree       || [];

        renderTrend(hourly.length ? hourly : null);
        renderPie(top.length   ? top   : null);
        renderCPUPressure(cpuPressure.length ? cpuPressure : null);
        renderDatabaseImpact(dbImpact.length ? dbImpact : null);
        renderBlockingTree(blocking.length ? blocking : null);
        renderHeatmap(daily.length ? daily : null);

        if (noData) {
            const collectingMsg = '<tr><td colspan="7" style="text-align:center;padding:18px;color:var(--text-muted);font-size:.7rem;">TimescaleDB not connected — collector data unavailable.</td></tr>';
            const t1 = document.getElementById('tbody-top-waits');
            const t2 = document.getElementById('tbody-active-waits');
            if (t1) t1.innerHTML = collectingMsg;
            if (t2) t2.innerHTML = collectingMsg;
            return;
        }

        renderTopWaits(top);
        renderActiveWaits(active);
    }

    // ── Wire time-picker ───────────────────────────────────────────────────────
    const syncWsRange = () => {
        if (fromEl?.value) window.appState.fromTs = fromEl.value;
        if (toEl?.value) window.appState.toTs = toEl.value;
    };
    if (fromEl) {
        fromEl.addEventListener('change', syncWsRange);
        fromEl.addEventListener('input', syncWsRange);
    }
    if (toEl) {
        toEl.addEventListener('change', syncWsRange);
        toEl.addEventListener('input', syncWsRange);
    }
    const applyBtn = document.getElementById('apply-time-btn');
    if (applyBtn) applyBtn.addEventListener('click', () => { syncWsRange(); loadData(); });

    window._waitStatsV2LoadData = () => { syncWsRange(); return loadData(); };

    // ── Initial load ───────────────────────────────────────────────────────────
    await loadData();
};
