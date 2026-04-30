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
        'memoryDrillChartCorrelation',
        'memoryDrillChartPressure'
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
    
    // Check if we came from sidebar
    const fromSidebar = window.appState.navigationHistory && window.appState.navigationHistory.length === 0;

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line" style="flex:1; min-width:0;">
                    <div style="display:flex; align-items:center; gap:0.75rem;">
                        ${fromSidebar ? '' : '<button class="btn btn-sm btn-icon" data-action="navigate" data-route="dashboard" title="Back"><i class="fa-solid fa-arrow-left"></i></button>'}
                        <h1>Memory Performance Analyzer</h1>
                    </div>
                    <span class="subtitle">Instance: ${window.escapeHtml(inst.name)}</span>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:0.6rem; flex-wrap:wrap; justify-content:flex-end;">
                    <div class="glass-panel" style="padding: 0.2rem 0.5rem; display: flex; align-items: center; gap: 0.4rem; font-size: 0.75rem; border: 1px solid var(--border-color);">
                        <label class="text-muted" style="margin:0;">from:</label>
                        <input type="datetime-local" id="memDrillFrom" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; width:10.5rem;" />
                        <label class="text-muted" style="margin:0;">to:</label>
                        <input type="datetime-local" id="memDrillTo" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; width:10.5rem;" />
                        <button type="button" class="btn btn-xs btn-accent" id="memApplyRange" style="padding:1px 6px;"><i class="fa-solid fa-filter"></i> Apply</button>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshMemoryDrilldown"><i class="fa-solid fa-refresh"></i> Refresh</button>
                    <span id="memDataSourceBadge" class="badge badge-info" style="font-size:0.65rem; display:none;">Source</span>
                </div>
            </div>

            <!-- Consolidated KPI Strip -->
            <div class="glass-panel dashboard-strip-panel mt-2">
                <div class="dashboard-strip-metrics-row--7">
                    <div class="strip-metric-cell">
                        <div class="strip-metric-label">Health</div>
                        <div class="strip-metric-value metric-value" id="kpiMemHealth">--</div>
                        <div class="text-muted sub" id="kpiMemHealthText">Evaluating…</div>
                    </div>
                    <div class="strip-metric-cell">
                        <div class="strip-metric-label">Grants Pending</div>
                        <div class="strip-metric-value metric-value" id="kpiMemGrantsPending">--</div>
                        <div class="text-muted sub">Queue Count</div>
                    </div>
                    <div class="strip-metric-cell">
                        <div class="strip-metric-label">Waiting Grants</div>
                        <div class="strip-metric-value metric-value" id="kpiWaitingGrants">--</div>
                        <div class="text-muted sub">Active waits</div>
                    </div>
                    <div class="strip-metric-cell">
                        <div class="strip-metric-label">Active Grants</div>
                        <div class="strip-metric-value metric-value" id="kpiActiveGrants">--</div>
                        <div class="text-muted sub">Running</div>
                    </div>
                    <div class="strip-metric-cell">
                        <div class="strip-metric-label">Headroom</div>
                        <div class="strip-metric-value metric-value" id="kpiMemHeadroom">--</div>
                        <div class="text-muted sub">MB Free</div>
                    </div>
                    <div class="strip-metric-cell">
                        <div class="strip-metric-label">Process Low</div>
                        <div class="strip-metric-value metric-value" id="kpiProcLow">--</div>
                        <div class="text-muted sub">Memory State</div>
                    </div>
                    <div class="strip-metric-cell">
                        <div class="strip-metric-label">Last Update</div>
                        <div class="strip-metric-value metric-value" id="memDrilldownLastUpdate" style="font-size:0.8rem;">--</div>
                        <div class="text-muted sub">Snap time</div>
                    </div>
                </div>
            </div>

            <div class="chart-card glass-panel mt-2" style="height: 180px;">
                <div class="card-header" style="padding: 0.25rem 0.75rem;"><h3 style="font-size:0.85rem;">Correlation: PLE vs Grants Pending vs Spills/sec</h3></div>
                <div class="chart-container" style="height: 140px;"><canvas id="memCorrelationChart"></canvas></div>
            </div>

            <div class="charts-grid mt-2" style="display:grid; grid-template-columns: repeat(3, 1fr); gap:0.75rem;">
                <div class="chart-card glass-panel" style="height: 170px;">
                    <div class="card-header" style="padding: 0.25rem 0.75rem;"><h3 style="font-size:0.85rem;">Memory Pressure Trend (%)</h3></div>
                    <div class="chart-container" style="height: 130px;"><canvas id="memPressureChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 170px;">
                    <div class="card-header" style="padding: 0.25rem 0.75rem;"><h3 style="font-size:0.85rem;">OS Free Memory %</h3></div>
                    <div class="chart-container" style="height: 130px;"><canvas id="memOsFreePctChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 170px;">
                    <div class="card-header" style="padding: 0.25rem 0.75rem;"><h3 style="font-size:0.85rem;">Page Life Expectancy (s)</h3></div>
                    <div class="chart-container" style="height: 130px;"><canvas id="memPleChart"></canvas></div>
                </div>
            </div>

            <div class="charts-grid mt-2" style="display:grid; grid-template-columns: repeat(3, 1fr); gap:0.75rem;">
                <div class="chart-card glass-panel" style="height: 170px;">
                    <div class="card-header" style="padding: 0.25rem 0.75rem;"><h3 style="font-size:0.85rem;">Workspace (Granted vs Req MB)</h3></div>
                    <div class="chart-container" style="height: 130px;"><canvas id="memWorkspaceChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 170px;">
                    <div class="card-header" style="padding: 0.25rem 0.75rem;"><h3 style="font-size:0.85rem;">TempDB Spills (warnings delta)</h3></div>
                    <div class="chart-container" style="height: 130px;"><canvas id="memSpillChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 170px;">
                    <div class="card-header" style="padding: 0.25rem 0.75rem;"><h3 style="font-size:0.85rem;">Plan Cache Size Trend (MB)</h3></div>
                    <div class="chart-container" style="height: 130px;"><canvas id="memPlanCacheChart"></canvas></div>
                </div>
            </div>

            <div class="grid mt-2" style="display:grid; grid-template-columns: 1.4fr 0.6fr; gap:0.75rem;">
                <div class="chart-card glass-panel" style="height: 200px;">
                    <div class="card-header" style="padding: 0.25rem 0.75rem;"><h3 style="font-size:0.85rem;">Buffer Pool by Database (MB) <span class="text-muted" style="font-size:0.65rem; font-weight:400;">User DBs only</span></h3></div>
                    <div class="chart-container" style="height: 160px;"><canvas id="memBufferPoolDbChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="height: 200px;">
                    <div class="card-header flex-between" style="padding: 0.25rem 0.75rem; gap:0.5rem;">
                        <h3 style="margin:0; font-size:0.85rem;">Top Memory Clerks (MB)</h3>
                    </div>
                    <div class="chart-container" style="height: 135px;"><canvas id="memClerksChart"></canvas></div>
                    <div id="memClerksLegend" class="text-muted" style="font-size:0.65rem; padding:0.1rem 0.5rem;"></div>
                </div>
            </div>
        </div>
    `;

    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    const pad = n => String(n).padStart(2, '0');
    const fmtL = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

    document.getElementById('memDrillFrom').value = fmtL(oneHourAgo);
    document.getElementById('memDrillTo').value = fmtL(now);

    document.getElementById('memApplyRange').onclick = () => window.applyMemoryDrilldownRange();

    await window.loadMemoryDrilldownData(inst.name, document.getElementById('memDrillFrom').value, document.getElementById('memDrillTo').value);
};

window.loadMemoryDrilldownData = async function(instanceName, fromLocal, toLocal) {
    const fromISO = window.memoryDrilldownLocalToRFC3339(fromLocal);
    const toISO = window.memoryDrilldownLocalToRFC3339(toLocal);
    const el = document.getElementById('memDrilldownLastUpdate');
    try {
        const url = `/api/timescale/sqlserver/memory-drilldown?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`;
        const res = await window.apiClient.authenticatedFetch(url);
        const data = await res.json();
        
        if (window.updateDataSourceBadge) window.updateDataSourceBadge('memDataSourceBadge', res.headers.get('X-Data-Source'));
        
        window.renderMemoryDrilldownCharts(data);
        if (el) el.textContent = new Date().toLocaleTimeString();
    } catch (e) {
        console.error('Memory drilldown load failed:', e);
        if (el) el.textContent = 'Error';
    }
};

window.renderMemoryDrilldownCharts = function(data) {
    window._destroyMemoryDrillCharts();
    
    const mem = window._memoryDrilldownSortPoints(data.memory_metrics || []);
    const sched = window._memoryDrilldownSortPoints(data.scheduler_memory || []);
    const ple = window._memoryDrilldownSortPoints(data.memory_history || []);
    const plan = window._memoryDrilldownSortPoints(data.plan_cache_health || []);
    const clerks = window._memoryDrilldownSortPoints(data.memory_clerks || []);
    const bpdbRaw = window._memoryDrilldownSortPoints(data.buffer_pool_by_db || []);
    const bpdb = bpdbRaw.filter(r => !['master','model','msdb','tempdb','resource'].includes(String(r.database_name || r.database || '').toLowerCase()));

    const baseOpts = {
        responsive: true, maintainAspectRatio: false,
        plugins: { legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } },
        scales: { x: { grid: { display: false }, ticks: { maxTicksLimit: 12 } } }
    };

    // 1. Grouped KPIs
    (function renderKpis() {
        const last = mem.length ? mem[mem.length - 1] : {};
        const pending = Number(last.memory_grants_pending) || 0;
        const waiting = Number(last.waiting_memory_grants) || 0;
        const active = Number(last.active_memory_grants) || 0;
        const used = Number(last.sql_memory_used_mb) || 0;
        const target = Number(last.sql_memory_target_mb) || 0;
        const headroom = Math.max(0, target - used);
        const procLow = (last.process_physical_low === true || last.process_physical_low === 1 || String(last.process_physical_low).toLowerCase() === 'true');

        const elPending = document.getElementById('kpiMemGrantsPending');
        if (elPending) elPending.textContent = pending;
        
        const elWaiting = document.getElementById('kpiWaitingGrants');
        if (elWaiting) elWaiting.textContent = waiting;
        
        const elActive = document.getElementById('kpiActiveGrants');
        if (elActive) elActive.textContent = active;
        
        const elHeadroom = document.getElementById('kpiMemHeadroom');
        if (elHeadroom) elHeadroom.textContent = headroom.toFixed(0);
        
        const elProcLow = document.getElementById('kpiProcLow');
        if (elProcLow) {
            elProcLow.textContent = procLow ? 'LOW' : 'Healthy';
            elProcLow.style.color = procLow ? 'var(--danger)' : 'var(--success)';
        }

        let health = 100;
        if (pending > 0) health -= 20;
        if (procLow) health -= 40;
        if (headroom < 100) health -= 10;
        
        const hEl = document.getElementById('kpiMemHealth');
        if (hEl) {
            hEl.textContent = health + '%';
            hEl.style.color = health > 80 ? 'var(--success)' : (health > 60 ? 'var(--warning)' : 'var(--danger)');
        }
        
        const hTextEl = document.getElementById('kpiMemHealthText');
        if (hTextEl) hTextEl.textContent = health > 80 ? 'Optimal' : 'Pressure';
    })();

    // 2. Correlation Chart
    if (mem.length && document.getElementById('memCorrelationChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        const ctx = document.getElementById('memCorrelationChart').getContext('2d');
        window.memoryDrillChartCorrelation = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets: [
                { label: 'PLE (s)', data: mem.map(m => Number(m.ple_seconds) || 0), borderColor: '#22c55e', yAxisID: 'y' },
                { label: 'Pending', data: mem.map(m => Number(m.memory_grants_pending) || 0), borderColor: '#eab308', yAxisID: 'y1' },
                { label: 'Spills/sec', data: mem.map(m => (Number(m.sort_warnings_per_sec)||0) + (Number(m.hash_warnings_per_sec)||0)), borderColor: '#fb7185', yAxisID: 'y1' }
            ]},
            options: { ...baseOpts, scales: { x: baseOpts.scales.x, y: { position: 'left' }, y1: { position: 'right', grid: { display: false } } } }
        });
    }

    // 3. Pressure & OS Free
    if (sched.length && document.getElementById('memPressureChart')) {
        const labels = window._memoryDrilldownLabels(sched);
        const ctx = document.getElementById('memPressureChart').getContext('2d');
        window.memoryDrillChartPressure = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets: [{ label: 'Memory Used %', data: sched.map(s => { const t = s.total_physical_memory_kb || 1; return ((t - (s.available_physical_memory_kb||0)) / t * 100); }), borderColor: '#3b82f6', fill: true, backgroundColor: 'rgba(59, 130, 246, 0.1)', tension: 0.3, pointRadius: 0 }] },
            options: baseOpts
        });
    }
    if (mem.length && document.getElementById('memOsFreePctChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        const ctx = document.getElementById('memOsFreePctChart').getContext('2d');
        window.memoryDrillChartSched = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets: [{ label: 'OS free %', data: mem.map(m => { const t = Number(m.os_total_memory_mb) || 1; return (Number(m.os_available_memory_mb) / t * 100); }), borderColor: '#38bdf8', fill: true, backgroundColor: 'rgba(56, 189, 248, 0.1)', pointRadius: 0 }] },
            options: { ...baseOpts, scales: { y: { min: 0, max: 100 } } }
        });
    }

    // 4. PLE & Workspace
    const pleSource = ple.length ? ple : mem;
    const pleField = ple.length ? 'page_life_expectancy_seconds' : 'ple_seconds';

    if (pleSource.length && document.getElementById('memPleChart')) {
        const labels = window._memoryDrilldownLabels(pleSource);
        const ctx = document.getElementById('memPleChart').getContext('2d');
        window.memoryDrillChartPle = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets: [{ label: 'PLE (s)', data: pleSource.map(p => Number(p[pleField]) || 0), borderColor: '#22c55e', fill: true, backgroundColor: 'rgba(34, 197, 94, 0.1)', pointRadius: 0 }] },
            options: baseOpts
        });
    }
    if (mem.length && document.getElementById('memWorkspaceChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        const ctx = document.getElementById('memWorkspaceChart').getContext('2d');
        window.memoryDrillChartWorkspace = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets: [
                { label: 'Granted (MB)', data: mem.map(m => Number(m.granted_workspace_mb) || 0), borderColor: '#22c55e' },
                { label: 'Requested (MB)', data: mem.map(m => Number(m.requested_workspace_mb) || 0), borderColor: '#ef4444' }
            ]},
            options: baseOpts
        });
    }

    // 5. Spills & Plan Cache
    if (mem.length && document.getElementById('memSpillChart')) {
        const labels = window._memoryDrilldownLabels(mem);
        const ctx = document.getElementById('memSpillChart').getContext('2d');
        window.memoryDrillChartSpill = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets: [
                { label: 'Sort warnings/s', data: mem.map(m => Number(m.sort_warnings_per_sec) || 0), borderColor: '#f97316' },
                { label: 'Hash warnings/s', data: mem.map(m => Number(m.hash_warnings_per_sec) || 0), borderColor: '#fb7185' }
            ]},
            options: baseOpts
        });
    }
    if (plan.length && document.getElementById('memPlanCacheChart')) {
        const labels = window._memoryDrilldownLabels(plan);
        const ctx = document.getElementById('memPlanCacheChart').getContext('2d');
        window.memoryDrillChartPlanCache = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets: [{ label: 'Plan cache (MB)', data: plan.map(p => Number(p.total_cache_mb) || 0), borderColor: '#eab308', fill: true, backgroundColor: 'rgba(234, 179, 8, 0.1)', pointRadius: 0 }] },
            options: baseOpts
        });
    }

    // 6. Buffer Pool & Clerks
    if (bpdb.length && document.getElementById('memBufferPoolDbChart')) {
        const labels = window._memoryDrilldownLabels(bpdb);
        const topNames = [...new Set(bpdb.map(r => String(r.database_name || r.database || '')))].slice(0, 8);
        const byDb = {}; topNames.forEach(n => byDb[n] = new Array(labels.length).fill(0));
        bpdb.forEach(r => {
            const n = String(r.database_name || r.database || '');
            if (byDb[n]) {
                const l = new Date(window._memoryDrilldownPointTime(r)).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
                const idx = labels.indexOf(l); if (idx >= 0) byDb[n][idx] = Number(r.buffer_mb) || 0;
            }
        });
        const ctx = document.getElementById('memBufferPoolDbChart').getContext('2d');
        const palette = ['#60a5fa','#22c55e','#a855f7','#f97316','#eab308','#fb7185','#38bdf8','#34d399'];
        window.memoryDrillChartBufferPool = new Chart(ctx, {
            type: 'line', data: { labels, datasets: topNames.map((n, i) => ({ label: n, data: byDb[n], borderColor: palette[i % palette.length], fill: true, backgroundColor: palette[i % palette.length] + '11', pointRadius: 0 })) },
            options: { ...baseOpts, plugins: { legend: { position: 'bottom' } } }
        });
    }
    if (clerks.length && document.getElementById('memClerksChart')) {
        const last = clerks.slice().sort((a,b) => (Number(b.pages_mb)||0) - (Number(a.pages_mb)||0)).slice(0, 10);
        const ctx = document.getElementById('memClerksChart').getContext('2d');
        
        // Local truncation helper
        const trunc = (s, n) => {
            const t = String(s ?? '');
            return t.length <= n ? t : t.slice(0, n) + '…';
        };

        window.memoryDrillChartClerks = new Chart(ctx, {
            type: 'bar',
            data: { labels: last.map(c => trunc(c.clerk_name || c.clerk_type || 'Unknown', 20)), datasets: [{ label: 'MB', data: last.map(c => Number(c.pages_mb) || 0), backgroundColor: '#3b82f6' }] },
            options: { ...baseOpts, indexAxis: 'y', plugins: { legend: { display: false } } }
        });
    }
};

window.applyMemoryDrilldownRange = async function() {
    const fromInput = document.getElementById('memDrillFrom');
    const toInput = document.getElementById('memDrillTo');
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    await window.loadMemoryDrilldownData(inst.name, fromInput.value, toInput.value);
};

window.refreshMemoryDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    await window.loadMemoryDrilldownData(inst.name, document.getElementById('memDrillFrom').value, document.getElementById('memDrillTo').value);
};
