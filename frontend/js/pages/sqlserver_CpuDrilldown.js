/*
 * SQL Optima — CPU Intelligence
 */

window.CpuDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <!-- ROW 0: HEADER -->
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <div style="display:flex; align-items:center; gap:0.75rem;">
                        <button class="btn btn-sm btn-outline" style="padding:0.3rem 0.6rem; font-size:1.1rem;" data-action="navigate" data-route="dashboard" title="Back to Dashboard"><i class="fa-solid fa-arrow-left"></i></button>
                        <h1><i class="fa-solid fa-microchip"></i> CPU Intelligence</h1>
                        <span class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Detailed Processor Utilization & Top Offenders</span>
                    </div>
                </div>
                <div class="flex-center dashboard-page-title-actions" style="gap: 0.75rem;">
                    <div class="glass-panel" style="padding: 0.2rem 0.5rem; display: flex; align-items: center; gap: 0.5rem; font-size: 0.75rem;">
                        <input type="datetime-local" id="cpuDrillFrom" class="bg-transparent border-none text-primary" style="width:12rem;" />
                        <span class="text-muted">→</span>
                        <input type="datetime-local" id="cpuDrillTo" class="bg-transparent border-none text-primary" style="width:12rem;" />
                        <button class="btn btn-xs btn-accent" id="cpuDrillApply">Apply</button>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshCpuDrilldown"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            
            <!-- ROW 1: CPU HISTORY CHART -->
            <div class="grid-container mt-3">
                <div class="col-12">
                    <div class="card glass-panel h-chart-md">
                        <div class="card-header flex-between">
                            <h3 style="font-size:0.8rem; margin:0;"><i class="fa-solid fa-chart-area text-rose"></i> CPU Usage Trend (SQL Server vs System)</h3>
                            <div style="display:flex; align-items:center; gap:0.5rem;">
                                <span id="cpuHistorySourceBadge" class="badge badge-info" style="display:none; font-size:0.65rem;"></span>
                                <span id="cpuDrilldownLastUpdate" class="text-muted" style="font-size:0.65rem;">Loading...</span>
                            </div>
                        </div>
                        <div class="chart-container" style="height: 230px;"><canvas id="cpuHistoryChart"></canvas></div>
                    </div>
                </div>
            </div>

            <!-- ROW 2: TOP OFFENDERS TABLE -->
            <div class="grid-container mt-3">
                <div class="col-12">
                    <div class="card glass-panel">
                        <div class="card-header flex-between">
                            <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-ranking-star text-accent"></i> Top Query Offenders (by CPU)</h3>
                            <span id="queryCount" class="badge badge-info">0 queries</span>
                        </div>
                        
                        <div id="cpuTopQueriesContainer" class="table-container-compact h-table-md" style="height: 500px; position:relative;">
                            <div id="cpuLoadingOverlay" class="loading-overlay" style="display:none; position:absolute; inset:0; background:rgba(0,0,0,0.3); z-index:10; display:flex; align-items:center; justify-content:center; flex-direction:column;">
                                <div class="spinner"></div>
                                <span class="mt-2 small">Loading...</span>
                            </div>
                            
                            <table class="modern-table modern-table-compact" id="cpuQueriesTable">
                                <thead>
                                    <tr>
                                        <th class="sortable" data-sort="last_seen">Last Seen</th>
                                        <th class="query-column">Query Statement</th>
                                        <th class="sortable text-end" data-sort="total_executions">Execs</th>
                                        <th class="sortable text-end" data-sort="avg_cpu_ms">Avg CPU</th>
                                        <th class="sortable text-end" data-sort="total_cpu_ms">Total CPU</th>
                                        <th class="sortable text-end" data-sort="total_reads">Reads</th>
                                        <th>Database</th>
                                        <th>Application</th>
                                        <th>Login</th>
                                    </tr>
                                </thead>
                                <tbody id="cpuQueriesBody">
                                    <tr><td colspan="9" class="text-center py-5 text-muted">Loading analytics...</td></tr>
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;

    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 3600000);
    const pad = n => String(n).padStart(2, '0');
    const fmtL = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

    document.getElementById('cpuDrillFrom').value = fmtL(oneHourAgo);
    document.getElementById('cpuDrillTo').value = fmtL(now);
    document.getElementById('cpuDrillApply').onclick = () => window.refreshCpuDrilldown();
    
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
    } catch (e) { console.error(e); }
    finally { if (overlay) overlay.style.display = 'none'; }
};

window.loadCpuTopOffenders = async function(instanceName, fromLocal, toLocal) {
    const fromISO = new Date(fromLocal).toISOString();
    const toISO = new Date(toLocal).toISOString();
    try {
        const url = `/api/sqlserver/workload/top-queries?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}&limit=50`;
        const res = await window.apiClient.authenticatedFetch(url);
        const data = await res.json();
        window.appState.cpuOffenders = data.top_offenders || [];
        window.sortCpuOffenders(window.appState.cpuSortKey, window.appState.cpuSortDir);
        window.renderCpuTopOffenders();
        window.attachCpuSortListeners();
    } catch (e) { console.error(e); }
};

window.renderCpuTopOffenders = function() {
    const body = document.getElementById('cpuQueriesBody');
    if (!body) return;
    const offenders = window.appState.cpuOffenders || [];
    if (offenders.length === 0) {
        body.innerHTML = '<tr><td colspan="9" class="text-center text-muted py-5">No data found.</td></tr>';
        document.getElementById('queryCount').textContent = '0 queries';
        return;
    }
    document.getElementById('queryCount').textContent = offenders.length + ' queries';
    body.innerHTML = offenders.map(q => `
        <tr>
            <td class="text-muted small">${q.last_seen ? new Date(q.last_seen).toLocaleString() : '--'}</td>
            <td class="query-column">
                <span class="code-snippet" style="cursor:pointer; color:var(--accent);" data-action="call" data-fn="showQueryModal" data-arg="${window.escapeHtml(q.query_text.replace(/'/g, "\\'"))}">
                    ${window.escapeHtml(q.query_text.substring(0, 100))}...
                </span>
            </td>
            <td class="text-end">${q.total_executions.toLocaleString()}</td>
            <td class="text-end text-danger">${q.avg_cpu_ms.toFixed(1)}</td>
            <td class="text-end font-bold">${q.total_cpu_ms.toLocaleString()}</td>
            <td class="text-end">${q.total_reads.toLocaleString()}</td>
            <td><span class="badge badge-outline">${q.database_name}</span></td>
            <td class="small text-muted">${window.escapeHtml(q.program_name || 'unknown')}</td>
            <td class="small text-muted">${window.escapeHtml(q.login_name || 'unknown')}</td>
        </tr>
    `).join('');
};

window.attachCpuSortListeners = function() {
    document.querySelectorAll('#cpuQueriesTable th.sortable').forEach(h => {
        h.onclick = () => {
            const col = h.dataset.sort;
            const dir = (window.appState.cpuSortKey === col && window.appState.cpuSortDir === 'desc') ? 'asc' : 'desc';
            window.appState.cpuSortKey = col;
            window.appState.cpuSortDir = dir;
            window.sortCpuOffenders(col, dir);
            window.renderCpuTopOffenders();
        };
    });
};

window.sortCpuOffenders = function(col, dir) {
    const offenders = window.appState.cpuOffenders || [];
    offenders.sort((a, b) => {
        let valA = a[col], valB = b[col];
        if (col === 'last_seen') { valA = new Date(valA).getTime(); valB = new Date(valB).getTime(); }
        if (valA < valB) return dir === 'desc' ? 1 : -1;
        if (valA > valB) return dir === 'desc' ? -1 : 1;
        return 0;
    });
};

window.loadCpuDrilldownChartOnly = async function(instanceName, fromLocal, toLocal) {
    const fromISO = new Date(fromLocal).toISOString();
    const toISO = new Date(toLocal).toISOString();
    try {
        const hist = typeof window.fetchSqlServerHistory === 'function'
            ? await window.fetchSqlServerHistory('cpu', instanceName, fromISO, toISO)
            : { points: [], source: '', ok: false };
        const points = hist.points || [];
        window.renderCpuDrilldownCharts(points);
        if (window.applyHistorySourceBadge) {
            window.applyHistorySourceBadge('cpuHistorySourceBadge', hist.source || (hist.ok ? 'hot' : ''));
        }
        document.getElementById('cpuDrilldownLastUpdate').textContent = 'Last update: ' + new Date().toLocaleTimeString();
    } catch (e) { console.error(e); }
};

window.renderCpuDrilldownCharts = function(cpuHistory) {
    if (window.cpuDrilldownChart) window.cpuDrilldownChart.destroy();
    if (!cpuHistory || cpuHistory.length === 0) return;
    const sorted = (window.sortByChartTime ? window.sortByChartTime(cpuHistory) : [...cpuHistory].sort((a, b) => {
        const ta = new Date(a.capture_timestamp || a.timestamp).getTime();
        const tb = new Date(b.capture_timestamp || b.timestamp).getTime();
        return ta - tb;
    }));
    const labels = sorted.map(t => {
        const d = new Date(t.capture_timestamp || t.timestamp);
        return isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour:'2-digit', minute:'2-digit' });
    });
    const ctx = document.getElementById('cpuHistoryChart').getContext('2d');
    window.cpuDrilldownChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                { label: 'SQL CPU', data: sorted.map(t => t.sql_process || 0), borderColor: '#3b82f6', backgroundColor: 'rgba(59, 130, 246, 0.1)', fill: true, tension: 0.4, pointRadius: 0 },
                { label: 'System Idle', data: sorted.map(t => t.system_idle || 0), borderColor: '#22c55e', fill: false, tension: 0.4, pointRadius: 0, borderDash: [2, 2] },
                { label: 'Other', data: sorted.map(t => t.other_process || 0), borderColor: '#f59e0b', fill: false, tension: 0.4, pointRadius: 0 }
            ]
        },
        options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } }, scales: { y: { max: 100, min: 0, ticks: { callback: v => v + '%' } }, x: { grid: { display: false }, ticks: { maxTicksLimit: 12 } } } }
    });
};
