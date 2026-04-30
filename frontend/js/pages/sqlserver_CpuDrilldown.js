/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: CPU drilldown page for detailed processor utilization analysis.
 *          Updated to match SQL Server Workload layout for consistent experience.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.escapeHtml = function(unsafe) { return (!unsafe) ? '' : unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;"); };

/** Convert datetime-local value to RFC3339 for Timescale APIs */
window.cpuDrilldownLocalToRFC3339 = function(localDt) {
    if (!localDt) return '';
    const d = new Date(localDt);
    if (isNaN(d.getTime())) return '';
    return d.toISOString();
};

window.CpuDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme workload-page-modern">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line" style="flex:1; min-width:0;">
                    <div style="display:flex; align-items:center; gap:0.75rem;">
                        <button class="btn btn-sm btn-outline" style="padding:0.3rem 0.6rem; font-size:1.1rem;" data-action="navigate" data-route="dashboard" title="Back to Dashboard"><i class="fa-solid fa-arrow-left"></i></button>
                        <h1 style="font-size: 1.5rem;">CPU Drilldown <span class="subtitle">- Instance: ${window.escapeHtml(inst.name)}</span></h1>
                    </div>
                </div>
                <div style="display: flex; align-items: center; gap: 1rem;">
                    <div class="date-picker-compact">
                        <input type="datetime-local" id="cpuDrillFrom">
                        <span style="color:var(--text-muted);">→</span>
                        <input type="datetime-local" id="cpuDrillTo">
                    </div>
                    <button class="btn btn-sm btn-accent" data-action="call" data-fn="applyCpuDrilldownRange"><i class="fa-solid fa-filter"></i> Apply</button>
                    <button class="btn btn-sm btn-outline" data-action="call" data-fn="refreshCpuDrilldown"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            
            <!-- CPU History Chart -->
            <div class="chart-card glass-panel mt-4" style="height: 250px;">
                <div class="card-header flex-between">
                    <h3><i class="fa-solid fa-chart-area text-rose"></i> CPU Usage Trend (%)</h3>
                    <span id="cpuDrilldownLastUpdate" class="text-muted" style="font-size:0.8rem;">Loading...</span>
                </div>
                <div class="chart-container" style="height: 190px;"><canvas id="cpuHistoryChart"></canvas></div>
            </div>

            <!-- Top Queries Table (Workload Layout) -->
            <div class="table-card glass-panel mt-4" style="padding:0;">
                <div class="card-header flex-between" style="padding:1rem;">
                    <div style="display: flex; align-items: center; gap: 0.5rem;">
                        <h3><i class="fa-solid fa-ranking-star text-accent"></i> Top Query Offenders (by CPU)</h3>
                    </div>
                    <span id="queryCount" class="badge badge-info">0 queries</span>
                </div>
                
                <div id="cpuTopQueriesContainer" class="table-responsive" style="min-height:200px; position:relative; padding:0 1rem 1rem 1rem;">
                    <div id="cpuLoadingOverlay" class="loading-overlay" style="display:none;">
                        <div class="spinner-border text-primary" role="status"></div>
                        <span class="mt-2">Fetching workload analytics...</span>
                    </div>
                    
                    <table class="data-table wl-table" id="cpuQueriesTable">
                        <thead>
                            <tr>
                                <th class="sortable" data-sort="last_seen" title="Last time this query was seen in the collection">Last Seen</th>
                                <th title="The SQL statement text" class="query-column">Query Statement</th>
                                <th class="sortable text-end" data-sort="total_executions" title="Number of times executed in range">Execs</th>
                                <th class="sortable text-end" data-sort="avg_cpu_ms" title="Average CPU per execution">Avg CPU (ms)</th>
                                <th class="sortable text-end" data-sort="total_cpu_ms" title="Total CPU time in range">Total CPU (ms)</th>
                                <th class="sortable text-end" data-sort="total_reads" title="Total logical reads in range">Reads</th>
                                <th title="Database where the query originated">Database</th>
                                <th title="Application attribution">Application</th>
                                <th title="Login attribution">Login</th>
                            </tr>
                        </thead>
                        <tbody id="cpuQueriesBody">
                            <tr><td colspan="9" class="text-center py-5 text-muted">Loading queries...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>

        <style>
            .date-picker-compact { background: rgba(0,0,0,0.2); border: 1px solid var(--border-color); border-radius: 6px; padding: 0.25rem 0.5rem; display: flex; align-items: center; gap: 0.4rem; }
            .date-picker-compact input { background: transparent; border: none; color: var(--text-primary); font-size: 0.7rem; outline: none; }
            .wl-table { width: 100%; border-collapse: collapse; font-size: 0.75rem; }
            .wl-table th { text-align: left; padding: 0.75rem 0.5rem; color: var(--text-muted); font-weight: 600; border-bottom: 2px solid var(--border-color); white-space: nowrap; }
            .wl-table td { padding: 0.75rem 0.5rem; border-bottom: 1px solid var(--border-color); vertical-align: middle; }
            .wl-table tr:hover { background: rgba(255,255,255,0.03); }
            .query-column { max-width: 350px; min-width: 200px; }
            .query-text-truncated { 
                max-width: 100%; 
                overflow: hidden; 
                text-overflow: ellipsis; 
                white-space: nowrap; 
                display: block;
                font-family: var(--font-mono, monospace); 
                font-size: 0.75rem;
                cursor: pointer;
            }
            .text-end { text-align: right !important; }
            .loading-overlay {
                position: absolute; top: 0; left: 0; right: 0; bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex; flex-direction: column; align-items: center; justify-content: center;
                z-index: 10; border-radius: 8px;
            }
        </style>
    `;

    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 3600000);
    const pad = n => String(n).padStart(2, '0');
    const fmtLocal = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

    document.getElementById('cpuDrillFrom').value = fmtLocal(oneHourAgo);
    document.getElementById('cpuDrillTo').value = fmtLocal(now);
    
    window.appState.cpuOffenders = [];
    window.appState.cpuSortKey = 'total_cpu_ms';
    window.appState.cpuSortDir = 'desc';

    await window.refreshCpuDrilldown();
};

window.refreshCpuDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    
    const fromVal = document.getElementById('cpuDrillFrom').value;
    const toVal = document.getElementById('cpuDrillTo').value;
    
    const overlay = document.getElementById('cpuLoadingOverlay');
    if (overlay) overlay.style.display = 'flex';

    try {
        await Promise.all([
            window.loadCpuDrilldownChartOnly(inst.name, fromVal, toVal),
            window.loadCpuTopOffenders(inst.name, fromVal, toVal)
        ]);
    } catch (e) {
        console.error('Refresh failed', e);
    } finally {
        if (overlay) overlay.style.display = 'none';
    }
};

window.applyCpuDrilldownRange = async function() {
    await window.refreshCpuDrilldown();
};

window.loadCpuTopOffenders = async function(instanceName, fromLocal, toLocal) {
    const fromISO = window.cpuDrilldownLocalToRFC3339(fromLocal);
    const toISO = window.cpuDrilldownLocalToRFC3339(toLocal);
    
    try {
        const url = `/api/sqlserver/workload/top-queries?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}&limit=50`;
        const res = await window.apiClient.authenticatedFetch(url);
        if (!res.ok) throw new Error('Top queries fetch failed');
        
        const data = await res.json();
        window.appState.cpuOffenders = data.top_offenders || [];
        
        window.sortCpuOffenders(window.appState.cpuSortKey, window.appState.cpuSortDir);
        window.renderCpuTopOffenders();
        window.attachCpuSortListeners();
    } catch (e) {
        console.error('Failed to load top queries', e);
        const tbody = document.getElementById('cpuQueriesBody');
        if (tbody) tbody.innerHTML = `<tr><td colspan="9" class="text-center text-danger">Error: ${e.message}</td></tr>`;
    }
};

window.renderCpuTopOffenders = function() {
    const body = document.getElementById('cpuQueriesBody');
    if (!body) return;
    const offenders = window.appState.cpuOffenders || [];
    
    if (offenders.length === 0) {
        body.innerHTML = '<tr><td colspan="9" class="text-center text-muted py-5">No query data found for this range.</td></tr>';
        document.getElementById('queryCount').textContent = '0 queries';
        return;
    }

    document.getElementById('queryCount').textContent = offenders.length + ' queries';
    
    body.innerHTML = offenders.map((q, idx) => {
        const cacheKey = `cpu_offender_${idx}`;
        if (!window.appState.queryCache) window.appState.queryCache = {};
        window.appState.queryCache[cacheKey] = q.query_text || '';

        return `
            <tr>
                <td class="text-muted" style="font-size:0.7rem;">${q.last_seen ? new Date(q.last_seen).toLocaleString() : '--'}</td>
                <td class="query-column">
                    <span class="query-text-truncated" title="Click to view full SQL statement" 
                          data-action="show-query-modal" data-key="${cacheKey}">
                        ${window.escapeHtml(q.query_text)}
                    </span>
                </td>
                <td class="text-end">${(q.total_executions || 0).toLocaleString()}</td>
                <td class="text-end"><b class="text-rose">${(q.avg_cpu_ms || 0).toFixed(1)}</b></td>
                <td class="text-end"><b>${(q.total_cpu_ms || 0).toLocaleString()}</b></td>
                <td class="text-end">${(q.total_reads || 0).toLocaleString()}</td>
                <td><span class="badge badge-outline" style="font-size:0.6rem;">${window.escapeHtml(q.database_name || '—')}</span></td>
                <td><span class="text-info" style="font-size:0.7rem;">${window.escapeHtml(q.program_name || 'unknown')}</span></td>
                <td><span class="text-warning" style="font-size:0.7rem;">${window.escapeHtml(q.login_name || 'unknown')}</span></td>
            </tr>
        `;
    }).join('');

    // Add event listeners for the clickable queries
    body.querySelectorAll('.query-text-truncated').forEach(el => {
        el.onclick = (e) => {
            const key = el.getAttribute('data-key');
            const query = window.appState.queryCache[key];
            if (window.showQueryModal) {
                window.showQueryModal(query);
            } else {
                alert(query);
            }
        };
    });
};

window.attachCpuSortListeners = function() {
    const headers = document.querySelectorAll('#cpuQueriesTable th.sortable');
    headers.forEach(h => {
        h.onclick = () => {
            const col = h.dataset.sort;
            const currentDir = (window.appState.cpuSortKey === col) ? window.appState.cpuSortDir : '';
            const newDir = currentDir === 'desc' ? 'asc' : 'desc';
            
            window.appState.cpuSortKey = col;
            window.appState.cpuSortDir = newDir;

            window.sortCpuOffenders(col, newDir);
            window.renderCpuTopOffenders();
        };
    });
};

window.sortCpuOffenders = function(col, dir) {
    const offenders = window.appState.cpuOffenders || [];
    offenders.sort((a, b) => {
        let valA = a[col];
        let valB = b[col];
        if (col === 'last_seen') {
            valA = new Date(valA).getTime();
            valB = new Date(valB).getTime();
        }
        if (valA < valB) return dir === 'desc' ? 1 : -1;
        if (valA > valB) return dir === 'desc' ? -1 : 1;
        return 0;
    });
};

window.loadCpuDrilldownChartOnly = async function(instanceName, fromLocal, toLocal) {
    const fromISO = window.cpuDrilldownLocalToRFC3339(fromLocal);
    const toISO = window.cpuDrilldownLocalToRFC3339(toLocal);
    
    try {
        const url = `/api/timescale/sqlserver/cpu-history?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`;
        const res = await window.apiClient.authenticatedFetch(url);
        if (!res.ok) throw new Error('CPU history fetch failed');
        
        const data = await res.json();
        const points = data.points || [];
        window.renderCpuDrilldownCharts(points);
        document.getElementById('cpuDrilldownLastUpdate').textContent = 'Last update: ' + new Date().toLocaleTimeString();
    } catch (e) {
        console.error('Failed to load CPU chart', e);
    }
};

window.renderCpuDrilldownCharts = function(cpuHistory) {
    if (window.cpuDrilldownChart) {
        window.cpuDrilldownChart.destroy();
        window.cpuDrilldownChart = null;
    }

    if (!cpuHistory || cpuHistory.length === 0) return;

    const sorted = cpuHistory.sort((a, b) => {
        const ta = new Date(a.capture_timestamp).getTime();
        const tb = new Date(b.capture_timestamp).getTime();
        return ta - tb;
    });

    const labels = sorted.map(t => new Date(t.capture_timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
    const sqlArr = sorted.map(t => t.sql_process || 0);
    const idleArr = sorted.map(t => t.system_idle || 0);
    const otherArr = sorted.map(t => t.other_process || 0);

    const ctx = document.getElementById('cpuHistoryChart').getContext('2d');
    window.cpuDrilldownChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                { label: 'SQL Server CPU', data: sqlArr, borderColor: '#3b82f6', backgroundColor: 'rgba(59, 130, 246, 0.1)', fill: true, tension: 0.4, pointRadius: 0 },
                { label: 'System Idle', data: idleArr, borderColor: '#22c55e', fill: false, tension: 0.4, pointRadius: 0, borderDash: [2, 2] },
                { label: 'Other Processes', data: otherArr, borderColor: '#f59e0b', fill: false, tension: 0.4, pointRadius: 0 }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } }
            },
            scales: {
                y: { max: 100, min: 0, ticks: { callback: v => v + '%' } },
                x: { grid: { display: false }, ticks: { maxTicksLimit: 15 } }
            }
        }
    });
};
