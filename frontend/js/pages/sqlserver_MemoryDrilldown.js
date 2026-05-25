/*
 * SQL Optima — Memory drilldown with PLE, memory clerks, and buffer pool analysis.
 */

window._memoryDrilldownSortPoints = function(arr) {
    return window.sortByChartTime ? window.sortByChartTime(arr) : (Array.isArray(arr) ? [...arr] : []);
};

window._memoryDrilldownLabels = function(arr) {
    return (arr || []).map(r => window.fmtChartTick ? window.fmtChartTick(r) : '');
};

window.CpuDrilldownLocalToRFC3339 = function(localDt) {
    if (!localDt) return '';
    const d = new Date(localDt);
    if (isNaN(d.getTime())) return '';
    return d.toISOString();
};

window.MemoryDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'sqlserver' };
    
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dash-header-stack-laptop dash-v2-page-header dashboard-page-title-compact" style="padding: 10px 20px; background: var(--bg-primary); border-bottom: 1px solid var(--border-color);">
                <div class="dash-v2-header-body">
                    <div class="dash-v2-header-row-top dash-header-title-inline">
                        <h1 class="dash-v2-h1" style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:10px;">
                            <i class="fa-solid fa-memory text-accent"></i>
                            SQL Server Memory Intelligence
                            <i class="fa-solid fa-circle-info text-accent cursor-pointer" style="font-size: 0.9rem;" data-action="show-sqlserver-dashboard-detail" data-dashboard="Memory Analyzer" title="Learn more about this dashboard"></i>
                        </h1>
                    </div>
                    <div class="dash-v2-header-row-bottom">
                        <span class="dash-v2-instance-name" style="font-size:0.7rem; color:var(--text-secondary); font-weight:600;"><i class="fa-solid fa-server" style="font-size:0.6rem;"></i> ${window.escapeHtml(inst.name)}</span>
                        <div class="dash-v2-header-badges dash-header-meta">
                            <span style="font-size:0.7rem; color:var(--text-muted);"><i class="fa-solid fa-database" style="font-size:0.6rem;"></i> Detailed Memory Pressure &amp; Buffer Pool Analytics</span>
                        </div>
                        <div class="dash-v2-header-actions dash-header-actions dashboard-page-title-actions" style="align-items:center; gap:1rem;">
                            <div id="time-picker-insertion-point"></div>
                            <div class="text-muted" style="font-size:0.65rem; background: rgba(0,0,0,0.2); padding: 4px 8px; border-radius: 4px;">
                                Update: <span id="mem-last-update" class="text-accent">--:--:--</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- ROW 1: MISSION CONTROL KPIs -->
            <div class="kpi-row">
                <div class="glass-panel metric-card-compact h-kpi">
                    <div class="metric-label">Health Score <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Memory Analyzer" data-metric="Memory Health Score"></i></div>
                    <div class="metric-value" id="kpiMemHealth">--%</div>
                    <span style="font-size:0.6rem; color:var(--text-muted);" id="kpiMemHealthText">Evaluating...</span>
                </div>
                <div class="glass-panel metric-card-compact h-kpi">
                    <div class="metric-label">Grants Pending <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Memory Analyzer" data-metric="Grants Pending"></i></div>
                    <div class="metric-value text-danger" id="kpiMemGrantsPending">--</div>
                    <span style="font-size:0.6rem; color:var(--text-muted);">Queue Length</span>
                </div>
                <div class="glass-panel metric-card-compact h-kpi">
                    <div class="metric-label">Waiting Grants <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Memory Analyzer" data-metric="Grants Pending"></i></div>
                    <div class="metric-value text-warning" id="kpiWaitingGrants">--</div>
                    <span style="font-size:0.6rem; color:var(--text-muted);">Active Waits</span>
                </div>
                <div class="glass-panel metric-card-compact h-kpi">
                    <div class="metric-label">Active Grants <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Memory Analyzer" data-metric="Grants Pending"></i></div>
                    <div class="metric-value text-success" id="kpiActiveGrants">--</div>
                    <span style="font-size:0.6rem; color:var(--text-muted);">Running</span>
                </div>
                <div class="glass-panel metric-card-compact h-kpi">
                    <div class="metric-label">Headroom <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Memory Analyzer" data-metric="Available Headroom"></i></div>
                    <div class="metric-value text-accent" id="kpiMemHeadroom">--</div>
                    <span style="font-size:0.6rem; color:var(--text-muted);">MB Available</span>
                </div>
                <div class="glass-panel metric-card-compact h-kpi">
                    <div class="metric-label">PLE <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="PLE"></i></div>
                    <div class="metric-value" id="kpiProcLow">--</div>
                    <span style="font-size:0.6rem; color:var(--text-muted);">Page Life Exp</span>
                </div>
            </div>

            <!-- ROW 2: CORE CORRELATION ANALYSIS -->
            <div class="grid-container mt-3">
                <div class="col-12">
                    <div class="card glass-panel h-chart-md">
                        <div class="card-header flex-between">
                            <h3 style="font-size:0.8rem; margin:0;">Correlation: PLE vs Grants Pending vs Spills/sec</h3>
                        </div>
                        <div class="chart-container" style="height: 230px;"><canvas id="memCorrelationChart"></canvas></div>
                    </div>
                </div>
            </div>

            <!-- ROW 3: PRESSURE & OS TELEMETRY -->
            <div class="grid-container mt-3">
                <div class="col-6 col-laptop-8 col-tablet-6">
                    <div class="card glass-panel h-chart-md">
                        <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Memory Pressure Trend (%)</h3></div>
                        <div class="chart-container" style="height: 220px;"><canvas id="memPressureChart"></canvas></div>
                    </div>
                </div>
                <div class="col-6 col-laptop-8 col-tablet-6">
                    <div class="card glass-panel h-chart-md">
                        <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">OS Free Memory %</h3></div>
                        <div class="chart-container" style="height: 220px;"><canvas id="memOsFreePctChart"></canvas></div>
                    </div>
                </div>
            </div>

            <!-- ROW 4: WORKSPACE -->
            <div class="grid-container mt-3">
                <div class="col-12">
                    <div class="card glass-panel h-chart-md">
                        <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Workspace Memory (Granted vs Requested MB)</h3></div>
                        <div class="chart-container" style="height: 220px;"><canvas id="memWorkspaceChart"></canvas></div>
                    </div>
                </div>
            </div>

            <!-- ROW 5: BUFFER POOL & CLERKS -->
            <div class="grid-container mt-3">
                <div class="col-8 col-laptop-8 col-tablet-6">
                    <div class="card glass-panel h-chart-md">
                        <div class="card-header flex-between">
                            <h3 style="font-size:0.8rem; margin:0;">Buffer Pool by Database (MB)</h3>
                            <span class="badge badge-outline">User DBs Only</span>
                        </div>
                        <div class="chart-container" style="height: 220px;"><canvas id="memBufferPoolDbChart"></canvas></div>
                    </div>
                </div>
                <div class="col-4 col-laptop-4 col-tablet-6">
                    <div class="card glass-panel h-chart-md">
                        <div class="card-header flex-between">
                            <h3 style="font-size:0.8rem; margin:0;">Top Memory Clerks (MB)</h3>
                        </div>
                        <div class="chart-container" style="height: 200px;"><canvas id="memClerksChart"></canvas></div>
                        <div id="memClerksLegend" class="text-muted text-center" style="font-size:0.6rem; padding:0.25rem;"></div>
                    </div>
                </div>
            </div>
        </div>
    `;

    if (window.initPageTimePicker) window.initPageTimePicker();

    const range = window.getAppTimeRangeISO
        ? window.getAppTimeRangeISO({ sync: true, fallbackHours: 1 })
        : { fromLocal: window.appState.fromTs, toLocal: window.appState.toTs };
    await window.loadMemoryDrilldownData(inst.name, range.fromLocal, range.toLocal);
};

window.loadMemoryDrilldownData = async function(instanceName, fromLocal, toLocal) {
    if (!fromLocal || !toLocal) {
        const range = window.getAppTimeRangeISO ? window.getAppTimeRangeISO({ fallbackHours: 1 }) : null;
        if (!range) return;
        fromLocal = range.fromLocal;
        toLocal = range.toLocal;
    }
    const fromISO = new Date(fromLocal).toISOString();
    const toISO = new Date(toLocal).toISOString();
    try {
        const url = `/api/timescale/sqlserver/memory-drilldown?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`;
        const res = await window.apiClient.authenticatedFetch(url);
        const data = await res.json();
        window.renderMemoryDrilldownCharts(data);
        const updateEl = document.getElementById('mem-last-update');
        if (updateEl) updateEl.textContent = new Date().toLocaleTimeString();
    } catch (e) { console.error('Memory drilldown load failed:', e); }
};

const _memDrillChartIds = ['memCorrelationChart','memPressureChart','memOsFreePctChart','memWorkspaceChart','memBufferPoolDbChart','memClerksChart'];
window._clearMemoryDrillNoDataOverlays = function() {
    _memDrillChartIds.forEach(id => {
        const el = document.getElementById(id);
        if (!el) return;
        el.style.display = '';
        const p = el.parentElement;
        if (p) p.querySelectorAll('.mem-nodata').forEach(n => n.remove());
    });
};

window._destroyMemoryDrillCharts = function() {
    _memDrillChartIds.forEach(id => {
        if (window.destroyChartOnCanvas) window.destroyChartOnCanvas(id);
        else {
            const el = document.getElementById(id);
            if (el) {
                const existing = Chart.getChart(el);
                if (existing) existing.destroy();
            }
        }
    });
};

window._memoryDrillNewChart = function(canvas, config) {
    if (!canvas) return null;
    if (window.destroyChartOnCanvas) window.destroyChartOnCanvas(canvas);
    return (typeof window.createOptimaChart === 'function')
        ? window.createOptimaChart(canvas, config)
        : new Chart(canvas, config);
};

window.renderMemoryDrilldownCharts = function(data) {
    window._destroyMemoryDrillCharts();
    window._clearMemoryDrillNoDataOverlays();
    const mem = window._memoryDrilldownSortPoints(data.memory_metrics || []);
    const sched = window._memoryDrilldownSortPoints(data.scheduler_memory || []);
    const bpdb = window._memoryDrilldownSortPoints(data.buffer_pool_by_db || []).filter(r => !['master','model','msdb','tempdb','resource'].includes(String(r.database_name || r.database || '').toLowerCase()));
    const axisOpts = (xLabel, yLabel, extraScales) => ({
        responsive: true, maintainAspectRatio: false,
        plugins: { legend: { position: 'top', labels: { boxWidth: 10, font: { size: 9 } } } },
        scales: {
            x: { grid: { display: false }, ticks: { maxTicksLimit: 12, font: { size: 8 } }, title: { display: !!xLabel, text: xLabel, font: { size: 9 }, color: 'var(--text-muted)' } },
            y: { beginAtZero: true, ticks: { font: { size: 8 } }, grid: { color: 'rgba(255,255,255,0.05)' }, title: { display: !!yLabel, text: yLabel, font: { size: 9 }, color: 'var(--text-muted)' } },
            ...(extraScales || {})
        }
    });
    const baseOpts = axisOpts('Time', '');

    // KPI update logic...
    (function renderKpis() {
        const last = mem.length ? mem[mem.length - 1] : {};
        const pending = Number(last.memory_grants_pending) || 0;
        const used = Number(last.sql_memory_used_mb) || 0;
        const target = Number(last.sql_memory_target_mb) || 0;
        const procLow = (last.process_physical_low === true || last.process_physical_low === 1);
        
        const elPending = document.getElementById('kpiMemGrantsPending');
        if (elPending) elPending.textContent = pending;
        
        const elWaiting = document.getElementById('kpiWaitingGrants');
        if (elWaiting) elWaiting.textContent = Number(last.waiting_memory_grants) || 0;
        
        const elActive = document.getElementById('kpiActiveGrants');
        if (elActive) elActive.textContent = Number(last.active_memory_grants) || 0;
        
        const elHeadroom = document.getElementById('kpiMemHeadroom');
        if (elHeadroom) elHeadroom.textContent = Math.max(0, target - used).toFixed(0);
        
        const elProcLow = document.getElementById('kpiProcLow');
        if (elProcLow) {
            elProcLow.textContent = procLow ? 'LOW' : 'Healthy';
            elProcLow.className = 'metric-value ' + (procLow ? 'text-danger' : 'text-success');
        }
        
        let health = 100; if (pending > 0) health -= 20; if (procLow) health -= 40;
        const hEl = document.getElementById('kpiMemHealth');
        if (hEl) {
            hEl.textContent = health + '%';
            hEl.className = 'metric-value ' + (health > 80 ? 'text-success' : (health > 60 ? 'text-warning' : 'text-danger'));
        }
    })();

    const labels = window._memoryDrilldownLabels(mem);

    if (!mem.length && !data.memory_clerks?.length && !bpdb.length) {
        const noDataMsg = `<div style="display:flex; align-items:center; justify-content:center; height:100%; min-height:60px; color:var(--text-muted); font-size:0.72rem;"><i class="fa-solid fa-database-slash" style="margin-right:6px; opacity:0.4;"></i>No collector data yet — metrics will appear after the first collection cycle.</div>`;
        ['memCorrelationChart','memPressureChart','memOsFreePctChart','memWorkspaceChart','memBufferPoolDbChart'].forEach(id => {
            const el = document.getElementById(id);
            if (el) { const p = el.parentElement; if (p) { el.style.display = 'none'; if (!p.querySelector('.mem-nodata')) { const d = document.createElement('div'); d.className = 'mem-nodata'; d.style.cssText = 'height:100%;'; d.innerHTML = noDataMsg; p.appendChild(d); } } }
        });
        const elLegend = document.getElementById('memClerksLegend');
        if (elLegend) elLegend.innerHTML = '';
        return;
    }

    if (mem.length) {
        const ctxCorrelation = document.getElementById('memCorrelationChart')?.getContext('2d');
        if (ctxCorrelation) {
            window._memoryDrillNewChart(ctxCorrelation.canvas, {
                type: 'line', data: { labels, datasets: [
                    { label: 'PLE (s)', data: mem.map(m => Number(m.ple_seconds) || 0), borderColor: '#22c55e', yAxisID: 'y', tension: 0.3, pointRadius: 0 },
                    { label: 'Pending Grants', data: mem.map(m => Number(m.memory_grants_pending) || 0), borderColor: '#eab308', yAxisID: 'y1', tension: 0.3, pointRadius: 0 }
                ]}, options: axisOpts('Time', 'PLE (seconds)', {
                    y:  { position: 'left',  beginAtZero: true, ticks: { font: { size: 8 } }, title: { display: true, text: 'PLE (s)', font: { size: 9 }, color: 'var(--text-muted)' } },
                    y1: { position: 'right', beginAtZero: true, ticks: { font: { size: 8 } }, grid: { display: false }, title: { display: true, text: 'Pending Grants', font: { size: 9 }, color: 'var(--text-muted)' } }
                })
            });
        }

        const ctxPressure = document.getElementById('memPressureChart')?.getContext('2d');
        if (ctxPressure) {
            window._memoryDrillNewChart(ctxPressure.canvas, {
                type: 'line', data: { labels, datasets: [{ label: 'Pressure %', data: mem.map(m => 100 - (Number(m.os_available_memory_mb)/Number(m.os_total_memory_mb)*100)), borderColor: '#3b82f6', fill: true, backgroundColor: 'rgba(59, 130, 246, 0.1)', tension: 0.3, pointRadius: 0 }] },
                options: axisOpts('Time', '% Used')
            });
        }

        const ctxOsFree = document.getElementById('memOsFreePctChart')?.getContext('2d');
        if (ctxOsFree) {
            window._memoryDrillNewChart(ctxOsFree.canvas, {
                type: 'line', data: { labels, datasets: [{ label: 'OS Free %', data: mem.map(m => Number(m.os_total_memory_mb) ? (Number(m.os_available_memory_mb)/Number(m.os_total_memory_mb)*100) : 0), borderColor: '#22c55e', fill: true, backgroundColor: 'rgba(34,197,94,0.1)', tension: 0.3, pointRadius: 0 }] },
                options: axisOpts('Time', '% Free')
            });
        }

        const ctxWorkspace = document.getElementById('memWorkspaceChart')?.getContext('2d');
        if (ctxWorkspace) {
            window._memoryDrillNewChart(ctxWorkspace.canvas, {
                type: 'line', data: { labels, datasets: [
                    { label: 'Granted MB', data: mem.map(m => Number(m.granted_workspace_mb) || 0), borderColor: '#22c55e', pointRadius: 0 },
                    { label: 'Requested MB', data: mem.map(m => Number(m.requested_workspace_mb) || 0), borderColor: '#ef4444', pointRadius: 0 }
                ]}, options: axisOpts('Time', 'MB')
            });
        }
    }

    if (data.memory_clerks && data.memory_clerks.length) {
        // Find the most-recent snapshot timestamp and filter to it
        const latestTs = data.memory_clerks.reduce((max, c) => {
            const t = window.tsMs(c);
            return t != null && t > max ? t : max;
        }, 0);
        const latestClerks = data.memory_clerks
            .filter(c => {
                const t = window.tsMs(c);
                return t != null && Math.abs(t - latestTs) < 1000;
            })
            .sort((a, b) => (Number(b.pages_mb)||0) - (Number(a.pages_mb)||0))
            .slice(0, 8);
        
        const ctxClerks = document.getElementById('memClerksChart')?.getContext('2d');
        if (ctxClerks) {
            window._memoryDrillNewChart(ctxClerks.canvas, {
                type: 'doughnut',
                data: {
                    labels: latestClerks.map(c => c.clerk_name.replace('MEMORYCLERK_','')),
                    datasets: [{
                        data: latestClerks.map(c => Number(c.pages_mb) || 0),
                        backgroundColor: ['#3b82f6','#10b981','#f59e0b','#ef4444','#8b5cf6','#06b6d4','#f43f5e','#14b8a6']
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: { legend: { display: false } }
                }
            });
        }

        const legendHtml = latestClerks.map((c, i) => `<span style="display:inline-block; margin-right:8px;"><i class="fa-solid fa-circle" style="color:${['#3b82f6','#10b981','#f59e0b','#ef4444','#8b5cf6','#06b6d4','#f43f5e','#14b8a6'][i]}; font-size:0.5rem;"></i> ${c.clerk_name.replace('MEMORYCLERK_','')} (${(Number(c.pages_mb)||0).toFixed(0)}MB)</span>`).join('');
        const elLegend = document.getElementById('memClerksLegend');
        if (elLegend) elLegend.innerHTML = legendHtml;
    } else {
        const elLegend = document.getElementById('memClerksLegend');
        if (elLegend) elLegend.innerHTML = '<div class="text-muted p-4">No clerk data available.</div>';
    }

    if (bpdb.length) {
        const validBpdb = bpdb.filter(r => {
            const n = r.database_name || r.database;
            return n && n !== 'undefined' && n !== 'null' && String(n).trim() !== '';
        });
        const topNames = [...new Set(validBpdb.map(r => r.database_name || r.database))].slice(0, 5);
        const bpTimes = [...new Set(validBpdb.map(r => window.tsMs(r)).filter(t => t != null))].sort((a, b) => a - b);
        const bpLabels = bpTimes.map(ms => window.fmtChartTick(ms));
        const ctxBp = document.getElementById('memBufferPoolDbChart')?.getContext('2d');
        if (ctxBp && topNames.length > 0 && bpLabels.length > 0) {
            window._memoryDrillNewChart(ctxBp.canvas, {
                type: 'line',
                data: {
                    labels: bpLabels,
                    datasets: topNames.map((n, i) => {
                        const byTime = new Map();
                        validBpdb.filter(r => (r.database_name || r.database) === n).forEach(r => {
                            const ms = window.tsMs(r);
                            if (ms != null) byTime.set(ms, Number(r.buffer_mb) || 0);
                        });
                        return {
                            label: n,
                            data: bpTimes.map(ms => byTime.get(ms) ?? null),
                            borderColor: ['#3b82f6','#22c55e','#f59e0b','#ef4444','#a855f7'][i],
                            pointRadius: 0,
                            tension: 0.3
                        };
                    })
                },
                options: axisOpts('Time', 'Buffer Pool (MB)')
            });
        } else if (ctxBp) {
            const p = ctxBp.canvas.parentElement;
            if (p) { ctxBp.canvas.style.display = 'none'; const d = document.createElement('div'); d.style.cssText = 'height:100%;display:flex;align-items:center;justify-content:center;font-size:0.72rem;color:var(--text-muted);'; d.textContent = 'No user database buffer pool data available.'; p.appendChild(d); }
        }
    } else {
        const ctxBp = document.getElementById('memBufferPoolDbChart')?.getContext('2d');
        if (ctxBp) {
            const p = ctxBp.canvas.parentElement;
            if (p) {
                ctxBp.canvas.style.display = 'none';
                if (!p.querySelector('.mem-nodata')) {
                    const d = document.createElement('div');
                    d.className = 'mem-nodata';
                    d.style.cssText = 'height:100%;display:flex;align-items:center;justify-content:center;font-size:0.72rem;color:var(--text-muted);';
                    d.textContent = 'Buffer pool by database appears after the Memory Intelligence collector runs (default ~60s).';
                    p.appendChild(d);
                }
            }
        }
    }
};

window.applyMemoryDrilldownRange = async function() { await window.refreshMemoryDrilldown(); };
window.refreshMemoryDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    const range = window.getAppTimeRangeISO
        ? window.getAppTimeRangeISO({ sync: true, fallbackHours: 1 })
        : { fromLocal: window.appState.fromTs, toLocal: window.appState.toTs };
    await window.loadMemoryDrilldownData(inst.name, range.fromLocal, range.toLocal);
};
