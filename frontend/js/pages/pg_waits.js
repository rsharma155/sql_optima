/*
 * SQL Optima — Waits, Bottlenecks & Sessions
 */
(function() {
    function trendTimeMs(t) {
        return window.tsMs ? window.tsMs(t) : null;
    }

    window.PgWaitsView = function() {
        const instance = window.appState.config?.instances?.[window.appState.currentInstanceIdx]?.name;
        if (!instance) return;

        window.appState.activeViewId = 'pg-waits';
        
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <!-- ROW 0: HEADER -->
                <div class="page-title flex-between dashboard-page-title-compact">
                    <div class="dashboard-title-line">
                        <h1><i class="fa-solid fa-clock-rotate-left text-accent"></i> Waits & Sessions</h1>
                        <span class="subtitle">Real-time AAS load, wait forensics, and deep session analysis</span>
                    </div>
                    <div class="flex-center dashboard-page-title-actions" style="gap: 0.75rem;">
                        <div id="time-picker-insertion-point"></div>
                    </div>
                </div>

                <!-- ROW 1: KPI STRIP -->
                <div class="kpi-row">
                    <div class="glass-panel metric-card-compact h-kpi" title="Average Active Sessions: The number of sessions either using CPU or waiting for a resource at this moment.">
                        <div class="metric-label">Avg Active</div>
                        <div class="metric-value" id="kpi-avg-active">--</div>
                        <span style="font-size:0.6rem; color:var(--text-muted);">AAS (Current)</span>
                    </div>
                    <div class="glass-panel metric-card-compact h-kpi" title="Average Waiting Sessions: Number of sessions blocked or waiting for I/O, locks, or other resources.">
                        <div class="metric-label">Avg Waiting</div>
                        <div class="metric-value" id="kpi-avg-waiting">--</div>
                        <span style="font-size:0.6rem; color:var(--text-muted);">Queued Tasks</span>
                    </div>
                    <div class="glass-panel metric-card-compact h-kpi" title="CPU Load: Average number of sessions actively using CPU. Value above CPU core count indicates saturation.">
                        <div class="metric-label">CPU Load</div>
                        <div class="metric-value text-accent" id="kpi-cpu-load">--</div>
                        <span style="font-size:0.6rem; color:var(--text-muted);">Normalized Load</span>
                    </div>
                    <div class="glass-panel metric-card-compact h-kpi" title="IO Wait: Average sessions waiting for disk or network I/O operations.">
                        <div class="metric-label">IO Wait</div>
                        <div class="metric-value text-warning" id="kpi-io-wait">--</div>
                        <span style="font-size:0.6rem; color:var(--text-muted);">Disk Latency Impact</span>
                    </div>
                    <div class="glass-panel metric-card-compact h-kpi" title="Lock Wait: Average sessions waiting for row-level or table-level locks. High value indicates application contention.">
                        <div class="metric-label">Lock Wait</div>
                        <div class="metric-value text-danger" id="kpi-lock-wait">--</div>
                        <span style="font-size:0.6rem; color:var(--text-muted);">Contention Load</span>
                    </div>
                    <div class="glass-panel metric-card-compact h-kpi" title="Idle In Txn: Number of sessions that have an open transaction but are not currently executing a query.">
                        <div class="metric-label">Idle In Txn</div>
                        <div class="metric-value text-muted" id="kpi-avg-idle">--</div>
                        <span style="font-size:0.6rem; color:var(--text-muted);">Zombie Sessions</span>
                    </div>
                </div>

                <!-- ROW 2: DATABASE LOAD ANALYSIS -->
                <div class="grid-container mt-3">
                    <div class="col-8 col-laptop-8 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header flex-between">
                                <h3 style="font-size:0.8rem; margin:0;">Database Load (AAS Timeline)</h3>
                                <span class="badge badge-outline">Active vs Waiting</span>
                            </div>
                            <div class="chart-container" style="height: 230px;">
                                <canvas id="pg-load-chart"></canvas>
                            </div>
                        </div>
                    </div>
                    <div class="col-4 col-laptop-4 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Wait Category Trend</h3></div>
                            <div class="chart-container" style="height: 230px;">
                                <canvas id="pg-wait-trend-chart"></canvas>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- ROW 3: DEEP ANALYSIS -->
                <div class="grid-container mt-3">
                    <div class="col-4 col-laptop-4 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Wait Distribution</h3></div>
                            <div class="chart-container" style="height: 210px;"><canvas id="pg-wait-dist-chart"></canvas></div>
                        </div>
                    </div>
                    <div class="col-4 col-laptop-4 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Connections by App</h3></div>
                            <div class="chart-container" style="height: 210px;"><canvas id="pg-app-conn-chart"></canvas></div>
                        </div>
                    </div>
                    <div class="col-4 col-laptop-8 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Session State Trend</h3></div>
                            <div class="chart-container" style="height: 210px;"><canvas id="pg-session-state-chart"></canvas></div>
                        </div>
                    </div>
                </div>

                <!-- ROW 4: INCIDENT INVESTIGATION -->
                <div class="grid-container mt-3">
                    <div class="col-12">
                        <div class="card glass-panel">
                            <div class="card-header flex-between">
                                <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-triangle-exclamation text-danger"></i> Long Running Sessions</h3>
                                <span class="text-muted" style="font-size:0.65rem;">Active sessions sorted by duration</span>
                            </div>
                            <div class="table-container-compact h-table-md">
                                <table class="modern-table modern-table-compact" id="pg-long-sessions-table">
                                    <thead><tr><th>PID</th><th>Captured At</th><th>App</th><th>User</th><th>State/Wait</th><th>Query Snippet</th></tr></thead>
                                    <tbody></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                    <div class="col-12 mt-3">
                        <div class="card glass-panel">
                            <div class="card-header flex-between">
                                <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-bolt text-warning"></i> Top Resource-Consuming Queries</h3>
                                <span class="text-muted" style="font-size:0.65rem;">Aggregated by total execution time in range</span>
                            </div>
                            <div class="table-container-compact h-table-md">
                                <table class="modern-table modern-table-compact" id="pg-top-queries-waits-table">
                                    <thead><tr><th>Timestamp</th><th>User</th><th>Calls</th><th>Total MS</th><th>Mean MS</th><th>Query Snippet</th></tr></thead>
                                    <tbody></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Modal remained unchanged logic-wise -->
                <div id="pg-query-modal" class="modal" style="display:none; position:fixed; z-index:9999; inset:0; background:rgba(0,0,0,0.7); align-items:center; justify-content:center;">
                    <div class="modal-content glass-panel" style="width: 85%; max-width: 900px; padding: 1.5rem; border-radius: 12px;">
                        <div class="flex-between mb-3">
                            <h3 style="margin: 0; font-size: 1rem;">Query Trace Detail</h3>
                            <button id="pg-query-modal-close" class="btn btn-icon btn-sm">&times;</button>
                        </div>
                        <pre id="pg-query-modal-text" style="background: rgba(0,0,0,0.3); padding: 1rem; border-radius: 8px; white-space: pre-wrap; word-wrap: break-word; max-height: 50vh; overflow-y: auto; color: var(--text-primary); font-family: 'JetBrains Mono', monospace; font-size: 0.75rem; border: 1px solid var(--border-color);"></pre>
                    </div>
                </div>
            </div>
        `;

        window.initPageTimePicker();
        document.getElementById('pg-query-modal-close').addEventListener('click', () => {
            document.getElementById('pg-query-modal').style.display = 'none';
        });
        window.showQueryModal = (q) => {
            const sql = decodePgWaitsQuery(q);
            document.getElementById('pg-query-modal-text').textContent = sql;
            document.getElementById('pg-query-modal').style.display = 'flex';
        };
        fetchData(instance);
    };

    async function fetchData(instance) {
        let from = window.appState.fromTs;
        let to = window.appState.toTs;
        if (from && from.includes('T') && !from.endsWith('Z')) from = new Date(from).toISOString();
        if (to && to.includes('T') && !to.endsWith('Z')) to = new Date(to).toISOString();

        try {
            let url = `/api/pg/observability/dashboard?instance=${encodeURIComponent(instance)}`;
            if (from) url += `&from=${encodeURIComponent(from)}`;
            if (to) url += `&to=${encodeURIComponent(to)}`;
            const resp = await window.apiClient.authenticatedFetch(url);
            if (!resp.ok) return;
            const data = await resp.json();
            renderKPIs(data.kpis, data.wait_distribution);
            renderLoadChart(data.load_trend);
            renderWaitTrend(data.wait_trend);
            renderWaitDist(data.wait_distribution);
            renderAppConn(data.connections_by_app);
            renderSessionStateTrend(data.session_state_trend);
            renderLongSessions(data.long_running_sessions);
            renderTopQueries(data.top_queries);
        } catch (err) { console.error(err); }
    }

    function renderKPIs(kpis, waits) {
        if (!kpis) return;
        const setT = (id, v) => { const el = document.getElementById(id); if (el) el.textContent = v; };
        setT('kpi-avg-active', (kpis.avg_active || 0).toFixed(1));
        setT('kpi-avg-waiting', (kpis.avg_waiting || 0).toFixed(1));
        setT('kpi-avg-idle', (kpis.avg_idle || 0).toFixed(1));
        setT('kpi-max-conn', kpis.max_conn || '0');
        if (waits) {
            setT('kpi-cpu-load', (waits.find(w => w.wait_event_type === 'CPU')?.avg_sessions || 0).toFixed(2));
            setT('kpi-io-wait', (waits.find(w => w.wait_event_type === 'IO')?.avg_sessions || 0).toFixed(2));
            setT('kpi-lock-wait', (waits.find(w => w.wait_event_type === 'Lock')?.avg_sessions || 0).toFixed(2));
        }
    }

    function renderLoadChart(trend) {
        const canvas = document.getElementById('pg-load-chart');
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        if (!trend) return;
        if (window.pgWaitsCharts?.load) window.pgWaitsCharts.load.destroy();
        window.pgWaitsCharts = window.pgWaitsCharts || {};
        window.pgWaitsCharts.load = new Chart(ctx, {
            type: 'line',
            data: {
                labels: trend.map(t => window.parseChartTimestamp ? window.parseChartTimestamp(t) : new Date(t.timestamp || t.ts)),
                datasets: [
                    { label: 'Active', data: trend.map(t => t.active_sessions), backgroundColor: 'rgba(16, 185, 129, 0.15)', borderColor: '#10b981', fill: true, pointRadius: 0, tension: 0.3 },
                    { label: 'Waiting', data: trend.map(t => t.waiting_sessions), backgroundColor: 'rgba(245, 158, 11, 0.15)', borderColor: '#f59e0b', fill: true, pointRadius: 0, tension: 0.3 }
                ]
            },
            options: { 
                responsive: true, 
                maintainAspectRatio: false, 
                plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 }, color: '#94a3b8' } } }, 
                scales: { 
                    x: { type: 'time', display: true, title: { display: true, text: 'Time', color: '#64748b', font: { size: 10 } }, ticks: { color: '#64748b', font: { size: 8 } } }, 
                    y: { beginAtZero: true, title: { display: true, text: 'Sessions (AAS)', color: '#64748b', font: { size: 10 } }, ticks: { color: '#64748b', font: { size: 8 } } } 
                } 
            }
        });
    }

    function renderWaitTrend(trend) {
        const canvas = document.getElementById('pg-wait-trend-chart');
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        if (!trend || trend.length === 0) return;

        if (window.pgWaitsCharts?.waitTrend) window.pgWaitsCharts.waitTrend.destroy();
        window.pgWaitsCharts = window.pgWaitsCharts || {};

        const types = [...new Set(trend.map(t => t.wait_event_type))];
        const labelMs = [...new Set(trend.map(trendTimeMs).filter(m => m != null))].sort((a, b) => a - b);
        const colors = ['#ef4444', '#f59e0b', '#10b981', '#3b82f6', '#8b5cf6', '#ec4899'];
        const datasets = types.map((type, i) => ({
            label: type,
            data: labelMs.map(ms => {
                const row = trend.find(t => trendTimeMs(t) === ms && t.wait_event_type === type);
                return row?.sessions || 0;
            }),
            backgroundColor: colors[i % colors.length] + '44', borderColor: colors[i % colors.length], fill: true, pointRadius: 0, tension: 0.3
        }));
        window.pgWaitsCharts.waitTrend = new Chart(ctx, {
            type: 'line',
            data: { labels: labelMs.map(ms => new Date(ms)), datasets },
            options: { 
                responsive: true, 
                maintainAspectRatio: false, 
                plugins: { legend: { display: false } }, 
                scales: { 
                    x: { type: 'time', display: true, title: { display: true, text: 'Time', color: '#64748b', font: { size: 10 } }, ticks: { color: '#64748b', font: { size: 8 } } }, 
                    y: { stacked: true, beginAtZero: true, title: { display: true, text: 'Sessions', color: '#64748b', font: { size: 10 } }, ticks: { color: '#64748b', font: { size: 8 } } } 
                } 
            }
        });
    }

    function renderWaitDist(dist) {
        const canvas = document.getElementById('pg-wait-dist-chart');
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        if (!dist || dist.length === 0) return;

        if (window.pgWaitsCharts?.waitDist) window.pgWaitsCharts.waitDist.destroy();
        window.pgWaitsCharts = window.pgWaitsCharts || {};

        window.pgWaitsCharts.waitDist = new Chart(ctx, {
            type: 'doughnut',
            data: { labels: dist.map(d => d.wait_event_type), datasets: [{ data: dist.map(d => d.avg_sessions), backgroundColor: ['#ef4444', '#f59e0b', '#10b981', '#3b82f6', '#8b5cf6', '#ec4899'], borderWidth:0 }] },
            options: { responsive: true, maintainAspectRatio: false, cutout:'70%', plugins:{legend:{position:'right', labels:{boxWidth:10, font:{size:9}}}} }
        });
    }

    function renderAppConn(apps) {
        const canvas = document.getElementById('pg-app-conn-chart');
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        if (!apps || apps.length === 0) return;

        if (window.pgWaitsCharts?.appConn) window.pgWaitsCharts.appConn.destroy();
        window.pgWaitsCharts = window.pgWaitsCharts || {};

        window.pgWaitsCharts.appConn = new Chart(ctx, {
            type: 'bar',
            data: { labels: apps.map(a => a.application_name || 'unknown'), datasets: [{ data: apps.map(a => a.count), backgroundColor: '#3b82f6', borderRadius:4 }] },
            options: { indexAxis: 'y', responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { x: { beginAtZero: true }, y: { ticks: { font: { size: 9 } } } } }
        });
    }

    function renderSessionStateTrend(trend) {
        const canvas = document.getElementById('pg-session-state-chart');
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        if (!trend || trend.length === 0) return;

        if (window.pgWaitsCharts?.stateTrend) window.pgWaitsCharts.stateTrend.destroy();
        window.pgWaitsCharts = window.pgWaitsCharts || {};

        // Human-readable labels for pg session states
        const STATE_LABELS = {
            'active':                  'Active',
            'idle':                    'Idle',
            'idle in transaction':     'Idle in Txn',
            'idle in transaction (aborted)': 'Idle in Txn (Abort)',
            'fastpath function call':  'Fast-Path',
            'disabled':                'Disabled',
        };
        const STATE_COLORS = ['#10b981', '#3b82f6', '#f59e0b', '#ef4444', '#8b5cf6', '#64748b'];

        const states = [...new Set(trend.map(t => t.state))];
        const labelMs = [...new Set(trend.map(trendTimeMs).filter(m => m != null))].sort((a, b) => a - b);
        const datasets = states.map((state, i) => ({
            label: STATE_LABELS[state] || state || 'Unknown',
            data: labelMs.map(ms => {
                const row = trend.find(t => trendTimeMs(t) === ms && t.state === state);
                return row?.count || 0;
            }),
            borderColor: STATE_COLORS[i % STATE_COLORS.length],
            backgroundColor: STATE_COLORS[i % STATE_COLORS.length] + '22',
            fill: true,
            pointRadius: 0,
            tension: 0.3,
        }));
        window.pgWaitsCharts.stateTrend = new Chart(ctx, {
            type: 'line',
            data: { labels: labelMs.map(ms => new Date(ms)), datasets },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        display: true,
                        position: 'bottom',
                        labels: { boxWidth: 10, font: { size: 9 }, color: '#94a3b8' }
                    }
                },
                scales: {
                    x: { type: 'time', display: true, ticks: { color: '#64748b', font: { size: 8 }, maxTicksLimit: 6 }, grid: { display: false } },
                    y: { beginAtZero: true, ticks: { color: '#64748b', font: { size: 8 } } }
                }
            }
        });
    }

    function renderLongSessions(sessions) {
        const tbody = document.querySelector('#pg-long-sessions-table tbody');
        if (!tbody) return;
        if (!sessions || sessions.length === 0) { tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No data</td></tr>'; return; }
        tbody.innerHTML = sessions.slice(0, 10).map(s => {
            const decodedQuery = decodePgWaitsQuery(s.query || '');
            const capturedAt = window.fmtChartTick
                ? (window.fmtChartTick(s, { hour: '2-digit', minute: '2-digit', second: '2-digit' }) || '—')
                : '—';
            return `
            <tr style="cursor:pointer;" data-action="call" data-fn="showQueryModal" data-arg="${encodeURIComponent(decodedQuery)}">
                <td><span class="badge badge-outline">${s.pid}</span></td>
                <td class="text-muted" style="font-size:0.78rem;">${capturedAt}</td>
                <td>${window.escapeHtml(s.application_name || 'unknown')}</td>
                <td>${window.escapeHtml(s.usename || '')}</td>
                <td class="text-warning">${window.escapeHtml(s.wait_event || s.state || 'CPU')}</td>
                <td class="small text-muted"><code>${window.escapeHtml(decodedQuery.substring(0, 40))}...</code></td>
            </tr>
        `; }).join('');
    }

    function renderTopQueries(queries) {
        const tbody = document.querySelector('#pg-top-queries-waits-table tbody');
        if (!tbody) return;
        if (!queries || queries.length === 0) { tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No data</td></tr>'; return; }
        const grouped = {};
        queries.forEach(q => {
            const qMs = window.tsMs(q);
            const prevMs = grouped[q.query] ? window.tsMs(grouped[q.query]) : null;
            if (!grouped[q.query] || (qMs != null && (prevMs == null || qMs > prevMs))) grouped[q.query] = q;
        });
        const top = Object.values(grouped).sort((a,b) => b.total_exec_time - a.total_exec_time).slice(0, 10);
        tbody.innerHTML = top.map(q => {
            const decodedQuery = decodePgWaitsQuery(q.query || '');
            return `
            <tr style="cursor:pointer;" data-action="call" data-fn="showQueryModal" data-arg="${encodeURIComponent(decodedQuery)}">
                <td class="small text-muted">${window.fmtChartTick ? window.fmtChartTick(q) : ''}</td>
                <td>${window.escapeHtml(q.usename || 'unknown')}</td>
                <td class="text-center">${Number(q.calls || 0).toLocaleString()}</td>
                <td class="text-right font-bold">${(q.total_exec_time || 0).toFixed(1)}</td>
                <td class="text-right">${(q.mean_exec_time || 0).toFixed(1)}</td>
                <td class="small"><code>${window.escapeHtml(decodedQuery.substring(0, 50))}...</code></td>
            </tr>
        `; }).join('');
    }

    function decodePgWaitsQuery(s) {
        if (window.decodeQueryText) return window.decodeQueryText(s);
        if (!s) return '';
        try {
            if (/%[0-9A-Fa-f]{2}/.test(s)) return decodeURIComponent(s).replace(/\+/g, ' ');
        } catch (e) {}
        return s;
    }
})();
