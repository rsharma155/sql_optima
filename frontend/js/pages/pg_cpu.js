/*
 * SQL Optima — PostgreSQL CPU utilization dashboard (host vs Postgres, saturation, DB share, top queries).
 */
(function() {
    window.PgCpuView = async function() {
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
        const dbName = window.appState.currentDatabase || 'all';
        
        window.appState.activeViewId = 'pg-cpu';
        
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme pg-cpu-page">
                <div class="page-title flex-between dashboard-page-title-compact">
                    <div class="dashboard-title-line">
                        <h1><i class="fa-solid fa-microchip"></i> CPU Usage</h1>
                        <span class="subtitle">Instance: ${inst.name} | Database: <span class="text-accent">${dbName}</span></span>
                    </div>
                    <div class="flex-center">
                        <div id="time-picker-insertion-point"></div>
                    </div>
                </div>

                <!-- ROW 1: Compact Metric Strip -->
                <div class="metrics-row-compact">
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Host CPU</div>
                        <div class="metric-value" id="kpi-host-cpu">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Postgres CPU</div>
                        <div class="metric-value text-accent" id="kpi-pg-cpu">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Active Conns</div>
                        <div class="metric-value" id="kpi-active-conn">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Saturation</div>
                        <div class="metric-value" id="cpu-saturation-badge">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">CPU/Conn</div>
                        <div class="metric-value" id="kpi-cpu-per-conn">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Load/Cores</div>
                        <div class="metric-value" id="kpi-load-cores">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Uptime</div>
                        <div class="metric-value" id="pg-uptime" style="font-size:0.8rem;">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Health</div>
                        <div class="metric-value" id="pgHealthScoreBadge">--</div>
                    </div>
                </div>

                <!-- ROW 2: Charts -->
                <div class="chart-row-compact">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Host vs. Postgres CPU %</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="cpuTimeSeriesChart"></canvas>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">CPU Time by DB</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="cpuDbDonutChart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- ROW 3: Top CPU Queries -->
                <div class="card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Top CPU Intensive Queries</h3></div>
                    <div class="table-container-compact" style="height:180px !important;">
                        <table class="modern-table modern-table-compact">
                            <thead>
                                <tr><th>User</th><th>QueryID</th><th>Query</th><th>Total ms</th><th>Calls</th><th>Avg ms</th></tr>
                            </thead>
                            <tbody id="cpu-top-queries-body"></tbody>
                        </table>
                    </div>
                </div>
            </div>
        `;

        window.initPageTimePicker();
        initPgCpu();
    };

    async function updatePgCpuHeader(instName) {
        try {
            const snapshotResp = await window.apiClient.authenticatedFetch(`/api/postgres/server-info?instance=${encodeURIComponent(instName)}`);
            if (snapshotResp.ok) {
                const s = await snapshotResp.json();
                if (document.getElementById('pg-uptime')) document.getElementById('pg-uptime').textContent = s.uptime || 'N/A';
                const hs = s.health_score || 0;
                const healthColor = hs > 80 ? 'text-success' : hs > 60 ? 'text-warning' : 'text-danger';
                const hBadge = document.getElementById('pgHealthScoreBadge');
                if (hBadge) {
                    hBadge.textContent = hs;
                    hBadge.className = `metric-value ${healthColor}`;
                }
            }
        } catch (e) { console.error("PG CPU header fetch failed:", e); }
    }

    function pgCpuEsc(s) {
        return window.escapeHtml ? window.escapeHtml(String(s ?? '')) : String(s ?? '');
    }

    function pgCpuTrunc(s, n) {
        const t = String(s ?? '');
        return t.length <= n ? t : t.slice(0, n) + '…';
    }

    function pgCpuNum(v) {
        const x = Number(v);
        return Number.isFinite(x) ? x : 0;
    }

    async function initPgCpu() {
        window.currentCharts = window.currentCharts || {};
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: '' };
        const q = encodeURIComponent(inst.name);

        let from = window.appState.fromTs;
        let to = window.appState.toTs;

        if (from && from.includes('T') && !from.endsWith('Z')) from = new Date(from).toISOString();
        if (to && to.includes('T') && !to.endsWith('Z')) to = new Date(to).toISOString();

        const timeParams = (from && to) ? `&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}` : '&limit=60';
        updatePgCpuHeader(inst.name);
        let sat = {}; let points = []; let dbRows = []; let topQueries = [];

        try {
            const [histRes, satRes, dbRes, qRes] = await Promise.all([
                window.apiClient.authenticatedFetch(`/api/cpu/history?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/cpu/saturation?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/cpu/database?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/cpu/top-queries?instance=${q}${timeParams}&limit=20`),
            ]);
            if (histRes.ok) points = (await histRes.json()).points || [];
            if (satRes.ok) sat = await satRes.json();
            if (dbRes.ok) dbRows = (await dbRes.json()).rows || [];
            if (qRes.ok) topQueries = (await qRes.json()).queries || [];
        } catch (e) { console.error('PG CPU dashboard fetch failed:', e); }

        updatePgCpuKpis(sat);
        renderPgCpuLineChart(points);
        renderPgCpuDonut(dbRows);
        renderPgCpuTopQueries(topQueries);
    }

    function updatePgCpuKpis(sat) {
        const osConfigured = sat.os_collector_configured;
        const host = pgCpuNum(sat.host_cpu_percent);
        const pg = pgCpuNum(sat.postgres_cpu_percent);
        const satPct = pgCpuNum(sat.cpu_saturation_pct);
        const perConn = pgCpuNum(sat.cpu_per_connection);
        const load1 = pgCpuNum(sat.load_1m);
        const cores = parseInt(String(sat.cpu_cores ?? 0), 10) || 0;
        const active = parseInt(String(sat.active_connections ?? 0), 10) || 0;

        const hostEl = document.getElementById('kpi-host-cpu');
        const pgEl = document.getElementById('kpi-pg-cpu');
        const connEl = document.getElementById('kpi-active-conn');
        const perEl = document.getElementById('kpi-cpu-per-conn');
        const loadEl = document.getElementById('kpi-load-cores');
        const badge = document.getElementById('cpu-saturation-badge');

        if (hostEl) hostEl.textContent = osConfigured ? host.toFixed(1) + '%' : 'N/A';
        if (pgEl) pgEl.textContent = pg.toFixed(1) + '%';
        if (connEl) connEl.textContent = String(active);
        if (perEl) perEl.textContent = perConn > 0 ? perConn.toFixed(2) + '%' : (active > 0 ? '0%' : 'N/A');
        if (loadEl) loadEl.textContent = osConfigured ? `${load1.toFixed(2)} / ${cores}` : 'N/A';

        if (badge) {
            badge.textContent = osConfigured ? satPct.toFixed(0) + '%' : 'N/A';
            badge.style.color = satPct > 90 ? 'var(--danger)' : satPct > 70 ? 'var(--warning)' : 'var(--success)';
        }
    }

    function renderPgCpuLineChart(points) {
        const rowsAsc = (points || []).slice().reverse();
        const timeLabels = rowsAsc.map((r) => r.capture_timestamp ? new Date(r.capture_timestamp).toLocaleTimeString() : '');
        const hostSeries = rowsAsc.map((r) => pgCpuNum(r.host_cpu_percent || r.cpu_usage));
        const pgSeries = rowsAsc.map((r) => pgCpuNum(r.postgres_cpu_percent));
        const ctx = document.getElementById('cpuTimeSeriesChart');
        if (!ctx) return;
        if (window.currentCharts.cpuTimeSeries) window.currentCharts.cpuTimeSeries.destroy();
        window.currentCharts.cpuTimeSeries = new Chart(ctx.getContext('2d'), {
            type: 'line',
            data: {
                labels: timeLabels,
                datasets: [
                    { label: 'Host %', data: hostSeries, borderColor: '#3b82f6', fill: false, tension: 0.3, pointRadius: 0 },
                    { label: 'PG %', data: pgSeries, borderColor: '#14b8a6', fill: false, tension: 0.3, pointRadius: 0 }
                ]
            },
            options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { x: { display: false }, y: { beginAtZero: true, max: 100 } } }
        });
    }

    function renderPgCpuDonut(dbRows) {
        const canvas = document.getElementById('cpuDbDonutChart');
        if (!canvas) return;
        if (window.currentCharts.cpuDbDonut) window.currentCharts.cpuDbDonut.destroy();
        const rows = (dbRows || []).filter((r) => pgCpuNum(r.total_exec_time_ms) > 0);
        const labels = rows.map((r) => r.datname || 'unknown');
        const data = rows.map((r) => pgCpuNum(r.total_exec_time_ms));
        window.currentCharts.cpuDbDonut = new Chart(canvas.getContext('2d'), {
            type: 'doughnut',
            data: {
                labels: labels.length ? labels : ['No data'],
                datasets: [{ data: data.length ? data : [1], backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ef4444'] }]
            },
            options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'right', labels: { boxWidth: 10, font: { size: 9 } } } }, cutout: '70%' }
        });
    }

    function renderPgCpuTopQueries(queries) {
        const tbody = document.getElementById('cpu-top-queries-body');
        if (!tbody) return;
        const list = queries || [];
        if (list.length === 0) { tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">None</td></tr>'; return; }
        tbody.innerHTML = list.slice(0, 5).map((q) => `
            <tr>
                <td>${pgCpuEsc(q.user_name || '—')}</td>
                <td><code>${pgCpuEsc(q.queryid || '—')}</code></td>
                <td><code title="${pgCpuEsc(q.query)}">${pgCpuTrunc(q.query, 30)}</code></td>
                <td>${pgCpuNum(q.total_exec_time).toFixed(1)}</td>
                <td>${q.calls || '0'}</td>
                <td>${pgCpuNum(q.avg_ms).toFixed(2)}</td>
            </tr>`).join('');
    }
})();
