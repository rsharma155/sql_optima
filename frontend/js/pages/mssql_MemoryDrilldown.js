/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Memory drilldown with PLE, memory clerks, and buffer pool analysis.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

/**
 * Memory drilldown: Timescale-backed series with RFC3339 from/to (same pattern as CPU drilldown).
 * API: GET /api/timescale/mssql/memory-drilldown?instance=&from=&to=&limit=
 */
window.memoryDrilldownLocalToRFC3339 = function(localDt) {
    if (window.cpuDrilldownLocalToRFC3339) {
        return window.cpuDrilldownLocalToRFC3339(localDt);
    }
    if (!localDt) return '';
    const d = new Date(localDt);
    if (isNaN(d.getTime())) return '';
    return d.toISOString();
};

window._destroyMemoryDrillCharts = function() {
    [
        'memoryDrillChartMetrics',
        'memoryDrillChartPle',
        'memoryDrillChartSched',
        'memoryDrillChartWorkspace',
        'memoryDrillChartSpill',
        'memoryDrillChartPlanCache',
        'memoryDrillChartBufferPool',
        'memoryDrillChartClerks',
        'memoryDrillChartCorrelation'
    ].forEach(function(key) {
        if (window[key] && typeof window[key].destroy === 'function') {
            window[key].destroy();
            window[key] = null;
        }
    });
};

window._memoryDrilldownPointTime = function(p) {
    if (!p) return 0;
    const raw = p.event_time || p.capture_timestamp;
    if (!raw) return 0;
    const t = new Date(String(raw).replace(' ', 'T')).getTime();
    return isNaN(t) ? 0 : t;
};

window._memoryDrilldownSortPoints = function(arr) {
    if (!arr || !arr.length) return [];
    return arr.slice().sort(function(a, b) {
        return window._memoryDrilldownPointTime(a) - window._memoryDrilldownPointTime(b);
    });
};

window._memoryDrilldownLabels = function(sorted) {
    return sorted.map(function(t) {
        const ts = t.event_time || t.capture_timestamp;
        if (!ts) return '';
        const d = new Date(String(ts).replace(' ', 'T'));
        if (isNaN(d.getTime())) return '';
        return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
    });
};

window.MemoryDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'sqlserver' };

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div class="flex-between" style="align-items:center; gap:1rem;">
                    <button class="btn btn-sm btn-outline" style="padding:0.3rem 0.6rem; font-size:1.1rem;" onclick="window.appNavigate('dashboard')" title="Back to Dashboard"><i class="fa-solid fa-arrow-left"></i></button>
                    <h1 style="font-size: 1.5rem;">Memory Performance Analyzer <span class="subtitle">- Instance: ${window.escapeHtml(inst.name)}</span></h1>
                </div>
                <div style="display: flex; align-items: center; gap: 1rem; flex-wrap: wrap;">
                    <div class="glass-panel" style="padding: 0.5rem 1rem; display: flex; align-items: center; gap: 0.5rem;">
                        <label style="font-size: 0.8rem; color: var(--text-muted);">from:</label>
                        <input type="datetime-local" id="memDrillFrom" class="custom-select" style="padding: 0.25rem; font-size: 0.8rem;">
                    </div>
                    <div class="glass-panel" style="padding: 0.5rem 1rem; display: flex; align-items: center; gap: 0.5rem;">
                        <label style="font-size: 0.8rem; color: var(--text-muted);">to:</label>
                        <input type="datetime-local" id="memDrillTo" class="custom-select" style="padding: 0.25rem; font-size: 0.8rem;">
                    </div>
                    <button class="btn btn-sm btn-accent" onclick="window.applyMemoryDrilldownRange()"><i class="fa-solid fa-filter"></i> Apply</button>
                    <button class="btn btn-sm btn-outline" onclick="window.refreshMemoryDrilldown()"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            <div id="memHealthBanner" class="glass-panel mt-3" style="padding:0.75rem 1rem; display:flex; align-items:center; justify-content:space-between; gap:1rem;">
                <div style="display:flex; align-items:center; gap:0.6rem;">
                    <span id="memHealthPill" class="badge badge-info" style="font-size:0.75rem;">HEALTH</span>
                    <span id="memHealthText" style="font-weight:600;">Evaluating memory health…</span>
                </div>
                <div style="display:flex; align-items:center; gap:0.5rem; flex-wrap:wrap;">
                    <span class="text-muted" style="font-size:0.75rem;">Data:</span>
                    <span id="memDataSourceChip" class="badge badge-secondary" style="font-size:0.75rem;">Loading…</span>
                </div>
            </div>
            <div class="flex-between mt-4" style="align-items:flex-start; gap:1rem; flex-wrap:wrap;">
                <div class="glass-panel" style="padding:0.75rem 1rem; min-width:220px;">
                    <div class="text-muted" style="font-size:0.72rem;">Memory Grants Pending</div>
                    <div id="kpiMemGrantsPending" style="font-size:1.4rem; font-weight:700;">-</div>
                </div>
                <div class="glass-panel" style="padding:0.75rem 1rem; min-width:220px;">
                    <div class="text-muted" style="font-size:0.72rem;">Waiting Grants</div>
                    <div id="kpiWaitingGrants" style="font-size:1.4rem; font-weight:700;">-</div>
                </div>
                <div class="glass-panel" style="padding:0.75rem 1rem; min-width:220px;">
                    <div class="text-muted" style="font-size:0.72rem;">Active Memory Grants</div>
                    <div id="kpiActiveGrants" style="font-size:1.4rem; font-weight:700;">-</div>
                </div>
                <div class="glass-panel" style="padding:0.75rem 1rem; min-width:220px;">
                    <div class="text-muted" style="font-size:0.72rem;">Memory Headroom (MB)</div>
                    <div id="kpiMemHeadroom" style="font-size:1.4rem; font-weight:700;">-</div>
                </div>
                <div class="glass-panel" style="padding:0.75rem 1rem; min-width:220px;">
                    <div class="text-muted" style="font-size:0.72rem;">Process Memory Low</div>
                    <div id="kpiProcLow" style="font-size:1.4rem; font-weight:700;">-</div>
                </div>
                <div class="glass-panel" style="padding:0.75rem 1rem; min-width:260px;">
                    <div class="text-muted" style="font-size:0.72rem;">Last update</div>
                    <div id="memDrilldownLastUpdate" style="font-size:0.95rem;">Loading…</div>
                </div>
            </div>

            <div class="chart-card glass-panel mt-4" style="height: 240px;">
                <div class="card-header"><h3>Correlation: PLE vs Grants Pending vs Spills/sec</h3></div>
                <div class="chart-container" style="height: 180px;"><canvas id="memCorrelationChart"></canvas></div>
            </div>

            <div class="grid mt-4" style="display:grid; grid-template-columns: 1fr 1fr; gap:1rem;">
                <div class="chart-card glass-panel" style="height: 220px;">
                    <div class="card-header"><h3>SQL Memory vs Target (MB)</h3></div>
                    <div class="chart-container" style="height: 160px;"><canvas id="memSqlUsedTargetChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 220px;">
                    <div class="card-header"><h3>OS Free Memory %</h3></div>
                    <div class="chart-container" style="height: 160px;"><canvas id="memOsFreePctChart"></canvas></div>
                </div>
            </div>

            <div class="grid mt-4" style="display:grid; grid-template-columns: 1fr 1fr; gap:1rem;">
                <div class="chart-card glass-panel" style="height: 220px;">
                    <div class="card-header"><h3>Page Life Expectancy (seconds)</h3></div>
                    <div class="chart-container" style="height: 160px;"><canvas id="memPleChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 220px;">
                    <div class="card-header"><h3>Workspace Memory (Granted vs Requested MB)</h3></div>
                    <div class="chart-container" style="height: 160px;"><canvas id="memWorkspaceChart"></canvas></div>
                </div>
            </div>

            <div class="grid mt-4" style="display:grid; grid-template-columns: 1fr 1fr; gap:1rem;">
                <div class="chart-card glass-panel" style="height: 220px;">
                    <div class="card-header"><h3>TempDB Spill Indicators (warnings delta)</h3></div>
                    <div class="chart-container" style="height: 160px;"><canvas id="memSpillChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 220px;">
                    <div class="card-header"><h3>Plan Cache Size Trend (MB)</h3></div>
                    <div class="chart-container" style="height: 160px;"><canvas id="memPlanCacheChart"></canvas></div>
                </div>
            </div>

            <div class="grid mt-4" style="display:grid; grid-template-columns: 1.2fr 0.8fr; gap:1rem;">
                <div class="chart-card glass-panel" style="height: 260px;">
                    <div class="card-header"><h3>Buffer Pool by Database (MB)</h3></div>
                    <div class="chart-container" style="height: 200px;"><canvas id="memBufferPoolDbChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 260px;">
                    <div class="card-header flex-between" style="gap:0.5rem;">
                        <h3 style="margin:0;">Top Memory Clerks (MB)</h3>
                        <span class="text-muted" style="font-size:0.75rem;">latest snapshot</span>
                    </div>
                    <div class="chart-container" style="height: 165px;"><canvas id="memClerksChart"></canvas></div>
                    <div id="memClerksLegend" class="text-muted" style="font-size:0.72rem; padding:0.35rem 0.75rem 0.65rem 0.75rem;"></div>
                </div>
            </div>
        </div>
    `;

    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    const dd = String(now.getDate()).padStart(2, '0');
    const hh = String(now.getHours()).padStart(2, '0');
    const min = String(now.getMinutes()).padStart(2, '0');

    document.getElementById('memDrillFrom').value = `${yyyy}-${mm}-${dd}T${String(oneHourAgo.getHours()).padStart(2, '0')}:${String(oneHourAgo.getMinutes()).padStart(2, '0')}`;
    document.getElementById('memDrillTo').value = `${yyyy}-${mm}-${dd}T${hh}:${min}`;

    const fromVal = document.getElementById('memDrillFrom').value;
    const toVal = document.getElementById('memDrillTo').value;
    await window.loadMemoryDrilldownData(inst.name, fromVal, toVal);
};

window.loadMemoryDrilldownData = async function(instanceName, fromLocal, toLocal) {
    const rangeErr = window.getDatetimeLocalRangeError && window.getDatetimeLocalRangeError(fromLocal, toLocal);
    if (rangeErr) {
        window.showDateRangeValidationError(rangeErr);
        return;
    }
    const fromISO = window.memoryDrilldownLocalToRFC3339(fromLocal);
    const toISO = window.memoryDrilldownLocalToRFC3339(toLocal);
    const el = document.getElementById('memDrilldownLastUpdate');
    try {
        if (!fromISO || !toISO) {
            if (el) el.textContent = 'Set from/to and Apply';
            return;
        }
        if (el) el.textContent = 'Loading…';
        ['memSqlUsedTargetChart','memOsFreePctChart','memPleChart','memWorkspaceChart','memSpillChart','memPlanCacheChart','memBufferPoolDbChart','memClerksChart'].forEach(function(id) {
            if (window.setChartOverlayState) window.setChartOverlayState(id, 'loading', 'Loading…');
        });
        const url = `/api/timescale/mssql/memory-drilldown?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`;
        const res = await window.apiClient.authenticatedFetch(url);
        if (!res.ok) {
            const errBody = await res.json().catch(function() { return {}; });
            throw new Error(errBody.error || ('HTTP ' + res.status));
        }
        const data = await res.json();
        window.renderMemoryDrilldownCharts(data);
        if (el) el.textContent = 'Updated ' + new Date().toLocaleTimeString();
    } catch (e) {
        appDebug('Memory drilldown load failed:', e);
        window._destroyMemoryDrillCharts();
        ['memSqlUsedTargetChart','memOsFreePctChart','memPleChart','memWorkspaceChart','memSpillChart','memPlanCacheChart','memBufferPoolDbChart','memClerksChart'].forEach(function(id) {
            if (window.setChartOverlayState) window.setChartOverlayState(id, 'empty', 'Could not load.');
        });
        if (el) el.textContent = 'Error';
    }
};

window.renderMemoryDrilldownCharts = function(data) {
    window._destroyMemoryDrillCharts();
    ['memCorrelationChart','memSqlUsedTargetChart','memOsFreePctChart','memPleChart','memWorkspaceChart','memSpillChart','memPlanCacheChart','memBufferPoolDbChart','memClerksChart'].forEach(function(id) {
        if (window.clearChartOverlay) window.clearChartOverlay(id);
    });

    const metrics = window._memoryDrilldownSortPoints(data.sqlserver_metrics || []);
    const ple = window._memoryDrilldownSortPoints(data.memory_history || []);
    const sched = window._memoryDrilldownSortPoints(data.scheduler_memory || []);
    const mem = window._memoryDrilldownSortPoints(data.memory_metrics || []);
    const plan = window._memoryDrilldownSortPoints(data.plan_cache_health || []);
    const clerks = window._memoryDrilldownSortPoints(data.memory_clerks || []);
    const bpdb = window._memoryDrilldownSortPoints(data.buffer_pool_by_db || []);
    const src = data.data_source || {};

    const baseOpts = {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } },
        scales: {
            x: { grid: { display: true, color: 'rgba(255,255,255,0.05)' }, ticks: { maxTicksLimit: 15 } }
        }
    };

    // KPI tiles from latest memory_metrics row (fallback to risk_health if present)
    (function renderKpis() {
        const last = mem.length ? mem[mem.length - 1] : null;
        const pending = last ? Number(last.memory_grants_pending) : null;
        const waiting = last ? Number(last.waiting_memory_grants) : null;
        const active = last ? Number(last.active_memory_grants) : null;
        const procLow = last ? (last.process_physical_low === true || last.process_physical_low === 1 || String(last.process_physical_low).toLowerCase() === 'true') : null;
        const headroom = last ? ((Number(last.sql_memory_target_mb) || 0) - (Number(last.sql_memory_used_mb) || 0)) : null;
        const elPending = document.getElementById('kpiMemGrantsPending');
        const elWaiting = document.getElementById('kpiWaitingGrants');
        const elActive = document.getElementById('kpiActiveGrants');
        const elProcLow = document.getElementById('kpiProcLow');
        const elHeadroom = document.getElementById('kpiMemHeadroom');
        if (elPending) elPending.textContent = (pending == null || isNaN(pending)) ? '-' : String(pending);
        if (elWaiting) elWaiting.textContent = (waiting == null || isNaN(waiting)) ? '-' : String(waiting);
        if (elActive) elActive.textContent = (active == null || isNaN(active)) ? '-' : String(active);
        if (elProcLow) elProcLow.textContent = (procLow == null) ? '-' : (procLow ? 'TRUE' : 'FALSE');
        if (elHeadroom) elHeadroom.textContent = (headroom == null || isNaN(headroom)) ? '-' : String(Math.max(0, Math.round(headroom)));
        if (elPending) {
            elPending.style.color = (pending != null && pending > 5) ? 'var(--danger)' : ((pending != null && pending > 0) ? 'var(--warning)' : '');
        }
        if (elWaiting) {
            elWaiting.style.color = (waiting != null && waiting > 0) ? 'var(--warning)' : '';
        }
        if (elHeadroom) {
            elHeadroom.style.color = (headroom != null && headroom <= 0) ? 'var(--danger)' : ((headroom != null && headroom < 512) ? 'var(--warning)' : '');
        }
        if (elProcLow) {
            elProcLow.style.color = procLow ? 'var(--danger)' : '';
        }
    })();

    // Data source chip
    (function renderSourceChip() {
        const el = document.getElementById('memDataSourceChip');
        if (!el) return;
        const mm = String(src.memory_metrics || 'timescale');
        const bp = String(src.buffer_pool_by_db || 'timescale');
        const label = (mm === 'live_fallback' || bp === 'live_fallback') ? 'Live (fallback)' : 'Timescale';
        el.textContent = label;
        el.className = 'badge ' + ((label === 'Timescale') ? 'badge-success' : 'badge-warning');
    })();

    // Health banner evaluation
    (function renderHealthBanner() {
        const pill = document.getElementById('memHealthPill');
        const text = document.getElementById('memHealthText');
        if (!pill || !text) return;
        if (!mem.length) {
            pill.className = 'badge badge-secondary';
            pill.textContent = 'UNKNOWN';
            text.textContent = 'No memory_metrics samples in range.';
            return;
        }
        const last = mem[mem.length - 1];

        const osTotal = Number(last.os_total_memory_mb) || 0;
        const osAvail = Number(last.os_available_memory_mb) || 0;
        const osFreePct = (osTotal > 0) ? (osAvail / osTotal) * 100.0 : 0;

        const procLow = (last.process_physical_low === true || last.process_physical_low === 1 || String(last.process_physical_low).toLowerCase() === 'true');
        const pendingNow = Number(last.memory_grants_pending) || 0;
        const used = Number(last.sql_memory_used_mb) || 0;
        const target = Number(last.sql_memory_target_mb) || 0;
        const req = Number(last.requested_workspace_mb) || 0;
        const gr = Number(last.granted_workspace_mb) || 0;

        const nowT = window._memoryDrilldownPointTime(last);
        const inLastSec = function(sec) {
            const cutoff = nowT - (sec * 1000);
            return mem.filter(function(p){ return window._memoryDrilldownPointTime(p) >= cutoff; });
        };
        const inLastMin = inLastSec(60);
        const inLast10m = inLastSec(600);

        const grantsPendingSustained1m = inLastMin.length >= 2 && inLastMin.every(function(p){ return (Number(p.memory_grants_pending) || 0) > 0; });

        const pleSeries = inLast10m.map(function(p){ return Number(p.ple_seconds) || 0; }).filter(function(v){ return v > 0; });
        const pleLatest = Number(last.ple_seconds) || 0;
        const pleMax10m = pleSeries.length ? Math.max.apply(null, pleSeries) : 0;
        const pleDrop50pct10m = (pleLatest > 0 && pleMax10m > 0) ? (pleLatest < (pleMax10m * 0.5)) : false;

        const usedNearTargetNow = (target > 0) ? (used / target) >= 0.98 : false;
        const usedNearTarget10m = (inLast10m.length >= 2 && target > 0) ? inLast10m.every(function(p){
            const u = Number(p.sql_memory_used_mb) || 0;
            const t = Number(p.sql_memory_target_mb) || 0;
            return t > 0 && (u / t) >= 0.98;
        }) : false;

        const reqGtGranted30pct = (gr > 0) ? (req > (gr * 1.3)) : (req > 0);

        const critReasons = [];
        const warnReasons = [];

        if (grantsPendingSustained1m) critReasons.push('Memory Grants Pending sustained >60s');
        if (osFreePct < 10) critReasons.push('OS Free Memory < 10%');
        if (procLow) critReasons.push('process_physical_memory_low = TRUE');
        if (pleDrop50pct10m) critReasons.push('PLE dropped >50% within 10 min');

        if (usedNearTarget10m) warnReasons.push('SQL Used ≈ Target sustained >10 min');
        if (osFreePct < 15) warnReasons.push('OS Free Memory < 15%');
        if (reqGtGranted30pct) warnReasons.push('Requested workspace > Granted by 30%');

        let status = 'HEALTHY';
        let klass = 'badge-success';
        let reasons = [];
        if (critReasons.length) {
            status = 'CRITICAL';
            klass = 'badge-danger';
            reasons = critReasons;
        } else if (warnReasons.length) {
            status = 'WARNING';
            klass = 'badge-warning';
            reasons = warnReasons;
        }
        pill.className = 'badge ' + klass;
        pill.textContent = status;
        text.textContent = reasons.length ? reasons.join(' • ') : (usedNearTargetNow ? 'SQL memory is at/near target; monitor OS free memory and grants.' : 'No memory pressure indicators detected in selected range.');
    })();

    if (mem.length && document.getElementById('memSqlUsedTargetChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        const used = mem.map(function(m) { return Number(m.sql_memory_used_mb) || 0; });
        const target = mem.map(function(m) { return Number(m.sql_memory_target_mb) || 0; });
        const ctx = document.getElementById('memSqlUsedTargetChart').getContext('2d');
        window.memoryDrillChartMetrics = new Chart(ctx, {
            type: 'line',
            data: { labels: labels, datasets: [
                { label: 'Used (MB)', data: used, borderColor: '#a855f7', backgroundColor: 'rgba(168, 85, 247, 0.08)', fill: true, tension: 0.35, pointRadius: 0 },
                { label: 'Target (MB)', data: target, borderColor: '#eab308', backgroundColor: 'rgba(234, 179, 8, 0.06)', fill: false, tension: 0.35, pointRadius: 0 }
            ]},
            options: baseOpts
        });
    } else if (document.getElementById('memSqlUsedTargetChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memSqlUsedTargetChart', 'empty', 'No SQL memory/target samples in range.');
    }

    if (mem.length && document.getElementById('memOsFreePctChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        const freePct = mem.map(function(m) {
            const total = Number(m.os_total_memory_mb) || 0;
            const avail = Number(m.os_available_memory_mb) || 0;
            if (total <= 0) return 0;
            return Math.max(0, Math.min(100, (avail / total) * 100.0));
        });
        const ctx = document.getElementById('memOsFreePctChart').getContext('2d');
        const warnLine = new Array(labels.length).fill(15);
        const critLine = new Array(labels.length).fill(10);
        window.memoryDrillChartSched = new Chart(ctx, {
            type: 'line',
            data: { labels: labels, datasets: [
                { label: 'OS free %', data: freePct, borderColor: '#38bdf8', backgroundColor: 'rgba(56, 189, 248, 0.1)', fill: true, tension: 0.35, pointRadius: 0 },
                { label: 'Warn 15%', data: warnLine, borderColor: 'rgba(234,179,8,0.9)', borderDash: [6,4], fill: false, pointRadius: 0 },
                { label: 'Crit 10%', data: critLine, borderColor: 'rgba(239,68,68,0.9)', borderDash: [6,4], fill: false, pointRadius: 0 }
            ] },
            options: Object.assign({}, baseOpts, { scales: Object.assign({}, baseOpts.scales, { y: { min: 0, max: 100, ticks: { callback: function(v){ return v + '%'; } } } }) })
        });
    } else if (document.getElementById('memOsFreePctChart') && window.setChartOverlayState) {
        // fallback to scheduler_memory free % if mem table empty
        if (sched.length && document.getElementById('memOsFreePctChart')) {
            const labels = window._memoryDrilldownLabels(sched);
            const freePct = sched.map(function(s) { return Number(s.physical_memory_free_percent) || 0; });
            const ctx = document.getElementById('memOsFreePctChart').getContext('2d');
            window.memoryDrillChartSched = new Chart(ctx, {
                type: 'line',
                data: { labels: labels, datasets: [{ label: 'OS free %', data: freePct, borderColor: '#38bdf8', backgroundColor: 'rgba(56, 189, 248, 0.1)', fill: true, tension: 0.35, pointRadius: 0 }] },
                options: Object.assign({}, baseOpts, { scales: Object.assign({}, baseOpts.scales, { y: { min: 0, max: 100, ticks: { callback: function(v){ return v + '%'; } } } }) })
            });
        } else {
            window.setChartOverlayState('memOsFreePctChart', 'empty', 'No OS memory samples in range.');
        }
    }

    if (ple.length && document.getElementById('memPleChart')) {
        const labels = window._memoryDrilldownLabels(ple);
        const pleVals = ple.map(function(p) {
            return Number(p.page_life_expectancy_seconds != null ? p.page_life_expectancy_seconds : p.page_life_expectancy) || 0;
        });
        const ctx2 = document.getElementById('memPleChart').getContext('2d');
        window.memoryDrillChartPle = new Chart(ctx2, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [
                    { label: 'PLE (s)', data: pleVals, borderColor: '#22c55e', backgroundColor: 'rgba(34, 197, 94, 0.1)', fill: true, tension: 0.35, pointRadius: 0 },
                    { label: 'Warn 1000s', data: new Array(labels.length).fill(1000), borderColor: 'rgba(234,179,8,0.9)', borderDash: [6,4], fill: false, pointRadius: 0 },
                    { label: 'Crit 300s', data: new Array(labels.length).fill(300), borderColor: 'rgba(239,68,68,0.9)', borderDash: [6,4], fill: false, pointRadius: 0 }
                ]
            },
            options: baseOpts
        });
    } else if (mem.length && document.getElementById('memPleChart')) {
        // Fallback: use PLE captured in memory_metrics when memory_history is empty
        const labels = window._memoryDrilldownLabels(mem);
        const pleVals = mem.map(function(m) { return Number(m.ple_seconds) || 0; });
        const ctx2 = document.getElementById('memPleChart').getContext('2d');
        window.memoryDrillChartPle = new Chart(ctx2, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [
                    { label: 'PLE (s)', data: pleVals, borderColor: '#22c55e', backgroundColor: 'rgba(34, 197, 94, 0.1)', fill: true, tension: 0.35, pointRadius: 0 },
                    { label: 'Warn 1000s', data: new Array(labels.length).fill(1000), borderColor: 'rgba(234,179,8,0.9)', borderDash: [6,4], fill: false, pointRadius: 0 },
                    { label: 'Crit 300s', data: new Array(labels.length).fill(300), borderColor: 'rgba(239,68,68,0.9)', borderDash: [6,4], fill: false, pointRadius: 0 }
                ]
            },
            options: baseOpts
        });
    } else if (document.getElementById('memPleChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memPleChart', 'empty', 'No PLE samples in range.');
    }

    if (mem.length && document.getElementById('memWorkspaceChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        const granted = mem.map(function(m) { return Number(m.granted_workspace_mb) || 0; });
        const requested = mem.map(function(m) { return Number(m.requested_workspace_mb) || 0; });
        const ctx = document.getElementById('memWorkspaceChart').getContext('2d');
        window.memoryDrillChartWorkspace = new Chart(ctx, {
            type: 'line',
            data: { labels: labels, datasets: [
                { label: 'Granted (MB)', data: granted, borderColor: '#22c55e', backgroundColor: 'rgba(34, 197, 94, 0.08)', fill: true, tension: 0.35, pointRadius: 0 },
                { label: 'Requested (MB)', data: requested, borderColor: '#ef4444', backgroundColor: 'rgba(239, 68, 68, 0.05)', fill: false, tension: 0.35, pointRadius: 0 }
            ]},
            options: baseOpts
        });
    } else if (document.getElementById('memWorkspaceChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memWorkspaceChart', 'empty', 'No workspace memory samples in range.');
    }

    if (mem.length && document.getElementById('memSpillChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        // Prefer server-computed rates; fallback to local delta/sec.
        const sortRate = mem.map(function(m, i) {
            const v = Number(m.sort_warnings_per_sec);
            if (!isNaN(v) && v > 0) return v;
            if (i === 0) return 0;
            const a = Number(mem[i-1].sort_warnings_total) || 0;
            const b = Number(m.sort_warnings_total) || 0;
            const t0 = window._memoryDrilldownPointTime(mem[i-1]);
            const t1 = window._memoryDrilldownPointTime(m);
            const dt = Math.max(1, (t1 - t0) / 1000.0);
            return Math.max(0, (b - a) / dt);
        });
        const hashRate = mem.map(function(m, i) {
            const v = Number(m.hash_warnings_per_sec);
            if (!isNaN(v) && v > 0) return v;
            if (i === 0) return 0;
            const a = Number(mem[i-1].hash_warnings_total) || 0;
            const b = Number(m.hash_warnings_total) || 0;
            const t0 = window._memoryDrilldownPointTime(mem[i-1]);
            const t1 = window._memoryDrilldownPointTime(m);
            const dt = Math.max(1, (t1 - t0) / 1000.0);
            return Math.max(0, (b - a) / dt);
        });
        const ctx = document.getElementById('memSpillChart').getContext('2d');
        window.memoryDrillChartSpill = new Chart(ctx, {
            type: 'line',
            data: { labels: labels, datasets: [
                { label: 'Sort warnings/sec', data: sortRate, borderColor: '#f97316', backgroundColor: 'rgba(249, 115, 22, 0.08)', fill: true, tension: 0.35, pointRadius: 0 },
                { label: 'Hash warnings/sec', data: hashRate, borderColor: '#fb7185', backgroundColor: 'rgba(251, 113, 133, 0.08)', fill: true, tension: 0.35, pointRadius: 0 },
                { label: 'Warn 5/sec', data: new Array(labels.length).fill(5), borderColor: 'rgba(234,179,8,0.9)', borderDash: [6,4], fill: false, pointRadius: 0 },
                { label: 'Crit 20/sec', data: new Array(labels.length).fill(20), borderColor: 'rgba(239,68,68,0.9)', borderDash: [6,4], fill: false, pointRadius: 0 }
            ]},
            options: baseOpts
        });
    } else if (document.getElementById('memSpillChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memSpillChart', 'empty', 'No spill indicator samples in range.');
    }

    if ((plan.length || mem.length) && document.getElementById('memPlanCacheChart')) {
        // Prefer dedicated plan_cache_mb in memory_metrics; fallback to total_cache_mb from plan_cache_health
        const labels = mem.length ? window._memoryDrilldownLabels(mem) : window._memoryDrilldownLabels(plan);
        const vals = mem.length
            ? mem.map(function(m) { return Number(m.plan_cache_mb) || 0; })
            : plan.map(function(p) { return Number(p.total_cache_mb) || 0; });
        const ctx = document.getElementById('memPlanCacheChart').getContext('2d');
        window.memoryDrillChartPlanCache = new Chart(ctx, {
            type: 'line',
            data: { labels: labels, datasets: [{ label: 'Plan cache (MB)', data: vals, borderColor: '#eab308', backgroundColor: 'rgba(234, 179, 8, 0.08)', fill: true, tension: 0.35, pointRadius: 0 }] },
            options: baseOpts
        });
    } else if (document.getElementById('memPlanCacheChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memPlanCacheChart', 'empty', 'No plan cache samples in range.');
    }

    if (bpdb.length && document.getElementById('memBufferPoolDbChart')) {
        // Stacked area by top DBs (last sample's top 8)
        const lastTs = bpdb[bpdb.length - 1].capture_timestamp || bpdb[bpdb.length - 1].event_time;
        const lastT = new Date(String(lastTs).replace(' ', 'T')).getTime();
        const atLast = bpdb.filter(function(r) {
            const t = window._memoryDrilldownPointTime(r);
            return Math.abs(t - lastT) < 2000;
        });
        const topNames = atLast
            .slice()
            .sort(function(a,b){ return (Number(b.buffer_mb)||0) - (Number(a.buffer_mb)||0); })
            .slice(0, 8)
            .map(function(r){ return String(r.database_name || r.database || '').trim(); })
            .filter(Boolean);
        const labels = window._memoryDrilldownLabels(bpdb);
        const byDb = {};
        topNames.forEach(function(n){ byDb[n] = new Array(labels.length).fill(0); });
        bpdb.forEach(function(r, idx){
            const name = String(r.database_name || '').trim();
            if (!byDb[name]) return;
            // Each row is a single db at a timestamp; align by point time label index.
            const t = window._memoryDrilldownPointTime(r);
            const label = new Date(t).toLocaleTimeString('en-US', { hour:'2-digit', minute:'2-digit', hour12:false });
            const li = labels.indexOf(label);
            if (li >= 0) byDb[name][li] = Number(r.buffer_mb) || 0;
        });
        const palette = ['#60a5fa','#22c55e','#a855f7','#f97316','#eab308','#fb7185','#38bdf8','#34d399'];
        const datasets = topNames.map(function(n, i){
            return { label: n, data: byDb[n], borderColor: palette[i % palette.length], backgroundColor: palette[i % palette.length] + '22', fill: true, tension: 0.25, pointRadius: 0 };
        });
        const ctx = document.getElementById('memBufferPoolDbChart').getContext('2d');
        window.memoryDrillChartBufferPool = new Chart(ctx, {
            type: 'line',
            data: { labels: labels, datasets: datasets },
            options: Object.assign({}, baseOpts, { plugins: { legend: { position: 'bottom', labels: { boxWidth: 10, font: { size: 10 } } } } })
        });
    } else if (document.getElementById('memBufferPoolDbChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memBufferPoolDbChart', 'empty', 'No buffer pool-by-DB samples in range.');
    }

    if (clerks.length && document.getElementById('memClerksChart')) {
        // Chart top clerk types based on last snapshot; show as horizontal bar with legend
        const lastTs = clerks[clerks.length - 1].capture_timestamp || clerks[clerks.length - 1].event_time;
        const lastT = new Date(String(lastTs).replace(' ', 'T')).getTime();
        const atLast = clerks.filter(function(r) {
            const t = window._memoryDrilldownPointTime(r);
            return Math.abs(t - lastT) < 2000;
        }).slice().sort(function(a,b){ return (Number(b.memory_mb)||Number(b.pages_mb)||0) - (Number(a.memory_mb)||Number(a.pages_mb)||0); }).slice(0, 12);
        const labels = atLast.map(function(r){ return String(r.clerk_type || '').trim(); });
        const vals = atLast.map(function(r){ return Number(r.memory_mb != null ? r.memory_mb : r.pages_mb) || 0; });
        const palette = ['#60a5fa','#22c55e','#a855f7','#f97316','#eab308','#fb7185','#38bdf8','#34d399','#c084fc','#f59e0b','#10b981','#f472b6'];
        const colors = labels.map(function(_, i){ return palette[i % palette.length]; });
        const ctx = document.getElementById('memClerksChart').getContext('2d');
        window.memoryDrillChartClerks = new Chart(ctx, {
            type: 'bar',
            data: { labels: labels, datasets: [{ label: 'MB', data: vals, backgroundColor: colors.map(function(c){ return c + '66'; }), borderColor: colors, borderWidth: 1 }] },
            options: Object.assign({}, baseOpts, {
                indexAxis: 'y',
                plugins: { legend: { display: false } },
                scales: Object.assign({}, baseOpts.scales, {
                    x: { grid: { display: true, color: 'rgba(255,255,255,0.05)' }, ticks: { callback: function(v){ return v + ' MB'; } } }
                })
            })
        });

        const legend = document.getElementById('memClerksLegend');
        if (legend) {
            legend.innerHTML = labels.map(function(name, i) {
                const c = colors[i];
                const v = vals[i];
                return `<div style="display:flex; align-items:center; gap:0.5rem; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">
                    <span style="display:inline-block; width:10px; height:10px; border-radius:3px; background:${c}; opacity:0.9;"></span>
                    <span style="min-width:1.2rem; color:var(--text-muted);">#${i+1}</span>
                    <span style="flex:1; overflow:hidden; text-overflow:ellipsis;">${window.escapeHtml(name)}</span>
                    <span style="color:var(--text-muted);">${Number(v).toFixed(0)} MB</span>
                </div>`;
            }).join('');
        }
    } else if (document.getElementById('memClerksChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memClerksChart', 'empty', 'No memory clerk samples in range.');
    }

    // Correlation chart: overlay PLE, grants pending, spills/sec
    if (mem.length && document.getElementById('memCorrelationChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        const pleVals = mem.map(function(m){ return Number(m.ple_seconds) || 0; });
        const pendingVals = mem.map(function(m){ return Number(m.memory_grants_pending) || 0; });
        const spillVals = mem.map(function(m, i){
            const v = Number(m.sort_warnings_per_sec);
            const v2 = Number(m.hash_warnings_per_sec);
            const s = (!isNaN(v) ? v : 0) + (!isNaN(v2) ? v2 : 0);
            if (s > 0) return s;
            if (i === 0) return 0;
            const a = (Number(mem[i-1].sort_warnings_total)||0) + (Number(mem[i-1].hash_warnings_total)||0);
            const b = (Number(m.sort_warnings_total)||0) + (Number(m.hash_warnings_total)||0);
            const t0 = window._memoryDrilldownPointTime(mem[i-1]);
            const t1 = window._memoryDrilldownPointTime(m);
            const dt = Math.max(1, (t1 - t0) / 1000.0);
            return Math.max(0, (b - a) / dt);
        });
        const ctx = document.getElementById('memCorrelationChart').getContext('2d');
        window.memoryDrillChartCorrelation = new Chart(ctx, {
            type: 'line',
            data: { labels: labels, datasets: [
                { label: 'PLE (s)', data: pleVals, borderColor: '#22c55e', backgroundColor: 'rgba(34,197,94,0.06)', fill: true, tension: 0.35, pointRadius: 0, yAxisID: 'yPle' },
                { label: 'Grants Pending', data: pendingVals, borderColor: '#eab308', backgroundColor: 'rgba(234,179,8,0.06)', fill: true, tension: 0.35, pointRadius: 0, yAxisID: 'yCount' },
                { label: 'Spills/sec (sort+hash)', data: spillVals, borderColor: '#fb7185', backgroundColor: 'rgba(251,113,133,0.06)', fill: true, tension: 0.35, pointRadius: 0, yAxisID: 'yRate' }
            ]},
            options: Object.assign({}, baseOpts, {
                scales: {
                    x: baseOpts.scales.x,
                    yPle: { type: 'linear', position: 'left', grid: { color: 'rgba(255,255,255,0.05)' } },
                    yCount: { type: 'linear', position: 'right', grid: { drawOnChartArea: false }, ticks: { precision: 0 } },
                    yRate: { type: 'linear', position: 'right', grid: { drawOnChartArea: false } }
                }
            })
        });
    } else if (document.getElementById('memCorrelationChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memCorrelationChart', 'empty', 'No memory metrics in range.');
    }
};

window.applyMemoryDrilldownRange = async function() {
    const fromInput = document.getElementById('memDrillFrom');
    const toInput = document.getElementById('memDrillTo');
    if (!fromInput || !toInput) return;
    const err = window.getDatetimeLocalRangeError && window.getDatetimeLocalRangeError(fromInput.value, toInput.value);
    if (err) {
        window.showDateRangeValidationError(err);
        return;
    }
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    await window.loadMemoryDrilldownData(inst.name, fromInput.value, toInput.value);
};

window.refreshMemoryDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    const fromInput = document.getElementById('memDrillFrom');
    const toInput = document.getElementById('memDrillTo');
    await window.loadMemoryDrilldownData(inst.name, fromInput ? fromInput.value : '', toInput ? toInput.value : '');
};
