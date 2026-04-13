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
    ['memoryDrillChartMetrics', 'memoryDrillChartPle', 'memoryDrillChartSched'].forEach(function(key) {
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
                    <h1 style="font-size: 1.5rem;">Memory Drilldown <span class="subtitle">- Instance: ${window.escapeHtml(inst.name)}</span></h1>
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
            <p class="text-muted" style="font-size:0.78rem; margin:0.75rem 0 0 0;">
                <i class="fa-solid fa-database"></i> Sources: <code>sqlserver_metrics.memory_usage</code>, <code>sqlserver_memory_history</code> (PLE),
                and scheduler OS memory fields from <code>sqlserver_cpu_scheduler_stats</code>. Requires Timescale collection for the selected window.
            </p>
            <div class="chart-card glass-panel mt-3" style="height: 200px;">
                <div class="card-header flex-between">
                    <h3>SQL Server memory % (metrics snapshot)</h3>
                    <span id="memDrilldownLastUpdate" class="text-muted" style="font-size:0.8rem;">Loading...</span>
                </div>
                <div class="chart-container" style="height: 140px;"><canvas id="memMetricsChart"></canvas></div>
            </div>
            <div class="chart-card glass-panel mt-3" style="height: 200px;">
                <div class="card-header"><h3>Page Life Expectancy (seconds)</h3></div>
                <div class="chart-container" style="height: 140px;"><canvas id="memPleChart"></canvas></div>
            </div>
            <div class="chart-card glass-panel mt-3" style="height: 200px;">
                <div class="card-header"><h3>Host physical memory free % (scheduler / sys.dm_os_sys_memory)</h3></div>
                <div class="chart-container" style="height: 140px;"><canvas id="memSchedulerChart"></canvas></div>
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
        ['memMetricsChart', 'memPleChart', 'memSchedulerChart'].forEach(function(id) {
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
        ['memMetricsChart', 'memPleChart', 'memSchedulerChart'].forEach(function(id) {
            if (window.setChartOverlayState) window.setChartOverlayState(id, 'empty', 'Could not load.');
        });
        if (el) el.textContent = 'Error';
    }
};

window.renderMemoryDrilldownCharts = function(data) {
    window._destroyMemoryDrillCharts();
    ['memMetricsChart', 'memPleChart', 'memSchedulerChart'].forEach(function(id) {
        if (window.clearChartOverlay) window.clearChartOverlay(id);
    });

    const metrics = window._memoryDrilldownSortPoints(data.sqlserver_metrics || []);
    const ple = window._memoryDrilldownSortPoints(data.memory_history || []);
    const sched = window._memoryDrilldownSortPoints(data.scheduler_memory || []);

    const baseOpts = {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } },
        scales: {
            x: { grid: { display: true, color: 'rgba(255,255,255,0.05)' }, ticks: { maxTicksLimit: 15 } }
        }
    };

    if (metrics.length && document.getElementById('memMetricsChart')) {
        const labels = window._memoryDrilldownLabels(metrics);
        const memPct = metrics.map(function(m) { return Number(m.memory_usage) || 0; });
        const ctx = document.getElementById('memMetricsChart').getContext('2d');
        window.memoryDrillChartMetrics = new Chart(ctx, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [{
                    label: 'Memory % (engine snapshot)',
                    data: memPct,
                    borderColor: '#a855f7',
                    backgroundColor: 'rgba(168, 85, 247, 0.12)',
                    fill: true,
                    tension: 0.35,
                    pointRadius: 0
                }]
            },
            options: Object.assign({}, baseOpts, {
                scales: Object.assign({}, baseOpts.scales, {
                    y: { min: 0, max: 100, ticks: { callback: function(v) { return v + '%'; } } }
                })
            })
        });
    } else if (document.getElementById('memMetricsChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memMetricsChart', 'empty', 'No memory % data in range.');
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
                datasets: [{
                    label: 'PLE (s)',
                    data: pleVals,
                    borderColor: '#22c55e',
                    backgroundColor: 'rgba(34, 197, 94, 0.1)',
                    fill: true,
                    tension: 0.35,
                    pointRadius: 0
                }]
            },
            options: baseOpts
        });
    } else if (document.getElementById('memPleChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memPleChart', 'empty', 'No PLE samples in range.');
    }

    if (sched.length && document.getElementById('memSchedulerChart')) {
        const labels = window._memoryDrilldownLabels(sched);
        const freePct = sched.map(function(s) { return Number(s.physical_memory_free_percent) || 0; });
        const ctx3 = document.getElementById('memSchedulerChart').getContext('2d');
        window.memoryDrillChartSched = new Chart(ctx3, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [{
                    label: 'Physical memory free %',
                    data: freePct,
                    borderColor: '#38bdf8',
                    backgroundColor: 'rgba(56, 189, 248, 0.1)',
                    fill: true,
                    tension: 0.35,
                    pointRadius: 0
                }]
            },
            options: Object.assign({}, baseOpts, {
                scales: Object.assign({}, baseOpts.scales, {
                    y: { min: 0, max: 100, ticks: { callback: function(v) { return v + '%'; } } }
                })
            })
        });
    } else if (document.getElementById('memSchedulerChart') && window.setChartOverlayState) {
        window.setChartOverlayState('memSchedulerChart', 'empty', 'No host memory samples in range.');
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
