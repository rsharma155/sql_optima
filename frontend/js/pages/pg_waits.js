/*
 * SQL Optima — Waits, Bottlenecks & Sessions (Merged)
 */
(function() {
    window.PgWaitsView = function() {
        const instance = window.appState.config?.instances?.[window.appState.currentInstanceIdx]?.name;
        if (!instance) return;

        window.appState.activeViewId = 'pg-waits';
        
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title flex-between dashboard-page-title-compact">
                    <div class="dashboard-title-line">
                        <h1><i class="fa-solid fa-clock-rotate-left"></i> Waits & Sessions</h1>
                        <span class="subtitle">Real-time bottleneck detection and session analysis</span>
                    </div>
                    <div class="flex-center">
                        <div id="time-picker-insertion-point"></div>
                    </div>
                </div>

                <!-- ROW 1: Compact Metric Strip -->
                <div class="metrics-row-compact">
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Avg Active</div>
                        <div class="metric-value text-success" id="kpi-avg-active">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Avg Waiting</div>
                        <div class="metric-value text-warning" id="kpi-avg-waiting">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Avg Idle In Txn</div>
                        <div class="metric-value" id="kpi-avg-idle">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Max Conn</div>
                        <div class="metric-value" id="kpi-max-conn">--</div>
                    </div>
                    <!-- Placeholders to fulfill 8-column layout if needed, or just use 4 -->
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">CPU Load</div>
                        <div class="metric-value" id="kpi-cpu-load">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">IO Wait</div>
                        <div class="metric-value" id="kpi-io-wait">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Lock Wait</div>
                        <div class="metric-value" id="kpi-lock-wait">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Temp Files</div>
                        <div class="metric-value" id="kpi-temp-files">--</div>
                    </div>
                </div>

                <!-- ROW 2: Database Load & Wait Category Trend -->
                <div class="chart-row-compact" style="grid-template-columns: 1fr 1fr;">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Database Load (AAS)</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-load-chart"></canvas>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Wait Category Trend</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-wait-trend-chart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- ROW 3: Wait Distribution & Connections -->
                <div class="chart-row-compact" style="grid-template-columns: 1fr 1fr 1fr;">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Wait Distribution</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-wait-dist-chart"></canvas>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Connections by App</h3></div>
                        <div class="chart-container chart-container-compact" style="height:150px !important;">
                            <canvas id="pg-app-conn-chart"></canvas>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Session State Trend</h3></div>
                        <div class="chart-container chart-container-compact" style="height:150px !important;">
                            <canvas id="pg-session-state-chart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- ROW 4: Long Running Sessions -->
                <div class="card glass-panel" style="margin-bottom: 1rem;">
                    <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Long Running Sessions</h3></div>
                    <div class="table-container-compact" style="height:150px !important;">
                        <table class="modern-table modern-table-compact" id="pg-long-sessions-table">
                            <thead>
                                <tr><th>PID</th><th>Time</th><th>App</th><th>User</th><th>State/Wait</th><th>Query</th></tr>
                            </thead>
                            <tbody></tbody>
                        </table>
                    </div>
                </div>

                <!-- ROW 5: Top Queries -->
                <div class="card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Top Queries by Total Time</h3></div>
                    <div class="table-container-compact" style="height:150px !important;">
                        <table class="modern-table modern-table-compact" id="pg-top-queries-waits-table">
                            <thead>
                                <tr><th>Query ID</th><th>User</th><th>Calls</th><th>Total(ms)</th><th>Mean(ms)</th><th>Query</th></tr>
                            </thead>
                            <tbody></tbody>
                        </table>
                    </div>
                </div>

                <!-- Query Details Modal -->
                <div id="pg-query-modal" class="modal" style="display:none; position:fixed; z-index:9999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.7); display:none; align-items:center; justify-content:center;">
                    <div class="modal-content glass-panel" style="width: 80%; max-width: 800px; padding: 20px; border-radius: 8px;">
                        <div class="flex-between" style="margin-bottom: 15px;">
                            <h3 style="margin: 0;">Query Details</h3>
                            <button id="pg-query-modal-close" style="background: none; border: none; color: #fff; font-size: 1.5rem; cursor: pointer;">&times;</button>
                        </div>
                        <pre id="pg-query-modal-text" style="background: rgba(0,0,0,0.3); padding: 15px; border-radius: 5px; white-space: pre-wrap; word-wrap: break-word; max-height: 400px; overflow-y: auto; color: #e5e7eb; font-family: monospace;"></pre>
                    </div>
                </div>
            </div>
        `;

        window.initPageTimePicker();
        document.getElementById('pg-query-modal-close').addEventListener('click', () => {
            document.getElementById('pg-query-modal').style.display = 'none';
        });
        fetchData(instance);
    };

    async function fetchData(instance) {
        let from = window.appState.fromTs;
        let to = window.appState.toTs;
        
        // Convert to ISO if it's a datetime-local string
        if (from && from.includes('T') && !from.endsWith('Z')) from = new Date(from).toISOString();
        if (to && to.includes('T') && !to.endsWith('Z')) to = new Date(to).toISOString();

        // Show loading state
        ['pg-load-chart', 'pg-wait-trend-chart', 'pg-wait-dist-chart', 'pg-app-conn-chart', 'pg-session-state-chart'].forEach(id => {
            window.setChartOverlayState(id, 'loading');
        });

        try {
            let url = `/api/pg/observability/dashboard?instance=${encodeURIComponent(instance)}`;
            if (from) url += `&from=${encodeURIComponent(from)}`;
            if (to) url += `&to=${encodeURIComponent(to)}`;
            
            const resp = await window.apiClient.authenticatedFetch(url);
            if (!resp.ok) {
                const msg = resp.status === 503 ? "TimescaleDB Disconnected" : "Failed to fetch dashboard data";
                ['pg-load-chart', 'pg-wait-trend-chart', 'pg-wait-dist-chart', 'pg-app-conn-chart', 'pg-session-state-chart'].forEach(id => {
                    window.setChartOverlayState(id, 'empty', msg);
                });
                return;
            }
            const data = await resp.json();
            
            ['pg-load-chart', 'pg-wait-trend-chart', 'pg-wait-dist-chart', 'pg-app-conn-chart', 'pg-session-state-chart'].forEach(id => {
                window.clearChartOverlay(id);
            });

            renderKPIs(data.kpis, data.wait_distribution);
            renderLoadChart(data.load_trend);
            renderWaitTrend(data.wait_trend);
            renderWaitDist(data.wait_distribution);
            renderAppConn(data.connections_by_app);
            renderSessionStateTrend(data.session_state_trend);
            renderLongSessions(data.long_running_sessions);
            renderTopQueries(data.top_queries);
        } catch (err) { 
            console.error(err); 
            ['pg-load-chart', 'pg-wait-trend-chart', 'pg-wait-dist-chart', 'pg-app-conn-chart', 'pg-session-state-chart'].forEach(id => {
                window.setChartOverlayState(id, 'empty', "Error loading data");
            });
        }
    }

    function renderKPIs(kpis, waits) {
        if (!kpis) return;
        const avgActive = document.getElementById('kpi-avg-active');
        avgActive.textContent = kpis.avg_active || '0';
        avgActive.className = 'metric-value ' + (kpis.avg_active > 50 ? 'text-danger' : (kpis.avg_active > 20 ? 'text-warning' : 'text-success'));
        
        const avgWait = document.getElementById('kpi-avg-waiting');
        avgWait.textContent = kpis.avg_waiting || '0';
        avgWait.className = 'metric-value ' + (kpis.avg_waiting > 10 ? 'text-danger' : (kpis.avg_waiting > 5 ? 'text-warning' : 'text-success'));
        
        document.getElementById('kpi-avg-idle').textContent = kpis.avg_idle || '0';
        document.getElementById('kpi-max-conn').textContent = kpis.max_conn || '0';
        
        if (waits) {
            const cpu = waits.find(w => w.wait_event_type === 'CPU')?.avg_sessions || 0;
            const io = waits.find(w => w.wait_event_type === 'IO')?.avg_sessions || 0;
            const lock = waits.find(w => w.wait_event_type === 'Lock')?.avg_sessions || 0;
            
            const cpuEl = document.getElementById('kpi-cpu-load');
            cpuEl.textContent = cpu.toFixed(2);
            cpuEl.className = 'metric-value ' + (cpu > 10 ? 'text-danger' : (cpu > 5 ? 'text-warning' : ''));
            
            const ioEl = document.getElementById('kpi-io-wait');
            ioEl.textContent = io.toFixed(2);
            ioEl.className = 'metric-value ' + (io > 5 ? 'text-danger' : (io > 2 ? 'text-warning' : ''));
            
            const lockEl = document.getElementById('kpi-lock-wait');
            lockEl.textContent = lock.toFixed(2);
            lockEl.className = 'metric-value ' + (lock > 5 ? 'text-danger' : (lock > 1 ? 'text-warning' : ''));
        }
    }

    function renderLoadChart(trend) {
        const ctx = document.getElementById('pg-load-chart').getContext('2d');
        if (!trend) return;
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: trend.map(t => new Date(t.ts)),
                datasets: [
                    { label: 'Active', data: trend.map(t => t.active_sessions), backgroundColor: 'rgba(16, 185, 129, 0.2)', borderColor: '#10b981', fill: true, stack: 'stack1' },
                    { label: 'Waiting', data: trend.map(t => t.waiting_sessions), backgroundColor: 'rgba(245, 158, 11, 0.2)', borderColor: '#f59e0b', fill: true, stack: 'stack1' },
                    { label: 'Idle in Txn', data: trend.map(t => t.idle_in_txn), backgroundColor: 'rgba(59, 130, 246, 0.2)', borderColor: '#3b82f6', fill: true, stack: 'stack1' }
                ]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { 
                    x: { type: 'time', time: { unit: 'minute' }, display: true, title: { display: true, text: 'Time' } }, 
                    y: { stacked: true, beginAtZero: true, title: { display: true, text: 'Sessions (AAS)' } } 
                }
            }
        });
    }

    function renderWaitTrend(trend) {
        const ctx = document.getElementById('pg-wait-trend-chart').getContext('2d');
        if (!trend || trend.length === 0) return;
        const types = [...new Set(trend.map(t => t.wait_event_type))];
        const labels = [...new Set(trend.map(t => t.bucket))].sort();
        const datasets = types.map(type => ({
            label: type,
            data: labels.map(l => {
                const item = trend.find(t => t.bucket === l && t.wait_event_type === type);
                return item ? item.sessions : 0;
            }),
            fill: true, stack: 'stack1'
        }));
        new Chart(ctx, {
            type: 'line',
            data: { labels: labels.map(l => new Date(l)), datasets: datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { x: { type: 'time', time: { unit: 'minute' }, display: false }, y: { stacked: true, beginAtZero: true } }
            }
        });
    }

    function renderWaitDist(dist) {
        const ctx = document.getElementById('pg-wait-dist-chart').getContext('2d');
        if (!dist || dist.length === 0) return;
        new Chart(ctx, {
            type: 'pie',
            data: {
                labels: dist.map(d => d.wait_event_type),
                datasets: [{ data: dist.map(d => d.avg_sessions), backgroundColor: ['#ef4444', '#f59e0b', '#10b981', '#3b82f6', '#8b5cf6', '#ec4899'] }]
            },
            options: { responsive: true, maintainAspectRatio: false }
        });
    }

    function renderAppConn(apps) {
        const ctx = document.getElementById('pg-app-conn-chart').getContext('2d');
        if (!apps || apps.length === 0) return;
        new Chart(ctx, {
            type: 'bar',
            data: {
                labels: apps.map(a => a.application_name || 'unknown'),
                datasets: [{ data: apps.map(a => a.count), backgroundColor: '#3b82f6' }]
            },
            options: { indexAxis: 'y', responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } } }
        });
    }

    function renderSessionStateTrend(trend) {
        const ctx = document.getElementById('pg-session-state-chart').getContext('2d');
        if (!trend || trend.length === 0) return;
        const states = [...new Set(trend.map(t => t.state))];
        const labels = [...new Set(trend.map(t => t.bucket))].sort();
        const datasets = states.map(state => ({
            label: state || 'unknown',
            data: labels.map(l => {
                const item = trend.find(t => t.bucket === l && t.state === state);
                return item ? item.count : 0;
            }),
            fill: true, stack: 'stack1'
        }));
        new Chart(ctx, {
            type: 'line',
            data: { labels: labels.map(l => new Date(l)), datasets: datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { x: { type: 'time', time: { unit: 'minute' }, display: false }, y: { stacked: true, beginAtZero: true } }
            }
        });
    }

    function renderLongSessions(sessions) {
        const tbody = document.querySelector('#pg-long-sessions-table tbody');
        if (!sessions || sessions.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">None</td></tr>';
            return;
        }
        window.showQueryModal = function(q) {
            document.getElementById('pg-query-modal-text').textContent = decodeURIComponent(q);
            document.getElementById('pg-query-modal').style.display = 'flex';
        };
        tbody.innerHTML = sessions.slice(0, 5).map(s => `
            <tr style="cursor:pointer;" class="clickable-query-row" data-query="${encodeURIComponent(s.query || '')}">
                <td>${s.pid}</td>
                <td>${s.duration || ''}</td>
                <td>${window.escapeHtml(s.application_name || 'unknown')}</td>
                <td>${window.escapeHtml(s.usename || '')}</td>
                <td>${window.escapeHtml(s.wait_event || s.state || 'CPU')}</td>
                <td><code title="${window.escapeHtml(s.query || '')}">${window.truncate(s.query || '', 20)}</code></td>
            </tr>
        `).join('');

        tbody.onclick = function(e) {
            const tr = e.target.closest('.clickable-query-row');
            if (tr) {
                window.showQueryModal(tr.getAttribute('data-query'));
            }
        };
    }

    function renderTopQueries(queries) {
        const tbody = document.querySelector('#pg-top-queries-waits-table tbody');
        if (!queries || queries.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">None</td></tr>';
            return;
        }
        tbody.innerHTML = queries.slice(0, 5).map(q => `
            <tr style="cursor:pointer;" class="clickable-query-row" data-query="${encodeURIComponent(q.query || '')}">
                <td>${q.queryid}</td>
                <td>${window.escapeHtml(q.usename || 'unknown')}</td>
                <td>${q.calls}</td>
                <td>${q.total_exec_time ? q.total_exec_time.toFixed(1) : '0'}</td>
                <td>${q.mean_exec_time ? q.mean_exec_time.toFixed(1) : '0'}</td>
                <td><code title="${window.escapeHtml(q.query || '')}">${window.truncate(q.query || '', 30)}</code></td>
            </tr>
        `).join('');

        tbody.onclick = function(e) {
            const tr = e.target.closest('.clickable-query-row');
            if (tr && typeof window.showQueryModal === 'function') {
                window.showQueryModal(tr.getAttribute('data-query'));
            }
        };
    }
})();
