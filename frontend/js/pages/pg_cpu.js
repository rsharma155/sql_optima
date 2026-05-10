/*
 * SQL Optima — PostgreSQL CPU utilization dashboard (host vs Postgres, saturation, DB share, top queries).
 */
(function() {
window.PgCpuView = async function() {
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
        const dbName = window.appState.currentDatabase || 'all';
        
        window.appState.activeViewId = 'pg-cpu';
        
        window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_cpu.html', { inst, dbName });

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
        let sat = {}; let points = []; let dbRows = []; let topQueries = []; let cc = {};

        try {
            const [histRes, satRes, dbRes, qRes, ccRes] = await Promise.all([
                window.apiClient.authenticatedFetch(`/api/cpu/history?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/cpu/saturation?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/cpu/database?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/cpu/top-queries?instance=${q}${timeParams}&limit=20`),
                window.apiClient.authenticatedFetch(`/api/postgres/control-center?instance=${q}`),
            ]);
            if (histRes.ok) points = (await histRes.json()).points || [];
            if (satRes.ok) sat = await satRes.json();
            if (dbRes.ok) dbRows = (await dbRes.json()).rows || [];
            if (qRes.ok) topQueries = (await qRes.json()).queries || [];
            if (ccRes.ok) cc = (await ccRes.json()).stats || {};
        } catch (e) { console.error('PG CPU dashboard fetch failed:', e); }

        updatePgCpuKpis(sat, cc);
        renderPgCpuLineChart(points);
        renderPgCpuDonut(dbRows);
        renderPgCpuTopQueries(topQueries);
    }

    function updatePgCpuKpis(sat, cc) {
        const pg = pgCpuNum(sat.postgres_cpu_percent);
        const perConn = pgCpuNum(sat.cpu_per_connection);
        const active = parseInt(String(sat.active_connections ?? 0), 10) || 0;

        const pgEl = document.getElementById('kpi-pg-cpu');
        const connEl = document.getElementById('kpi-active-conn');
        const perEl = document.getElementById('kpi-cpu-per-conn');
        const tpsEl = document.getElementById('kpi-tps');
        const waitEl = document.getElementById('kpi-waiting');
        const deadEl = document.getElementById('kpi-dead-tuple');

        if (pgEl) pgEl.textContent = pg.toFixed(1) + '%';
        if (connEl) connEl.textContent = String(active);
        if (perEl) perEl.textContent = perConn > 0 ? perConn.toFixed(2) + '%' : (active > 0 ? '0%' : 'N/A');

        if (tpsEl) tpsEl.textContent = pgCpuNum(cc.tps).toFixed(0);
        if (waitEl) waitEl.textContent = String(cc.waiting_sessions ?? 0);
        if (deadEl) deadEl.textContent = pgCpuNum(cc.dead_tuple_pct).toFixed(1) + '%';
    }

    function renderPgCpuLineChart(points) {
        const rowsAsc = (points || []).slice().reverse();
        const timeLabels = rowsAsc.map((r) => r.capture_timestamp ? new Date(r.capture_timestamp).toLocaleTimeString() : '');
        const pgSeries = rowsAsc.map((r) => pgCpuNum(r.postgres_cpu_percent));
        const ctx = document.getElementById('cpuTimeSeriesChart');
        if (!ctx) return;
        if (window.currentCharts.cpuTimeSeries) window.currentCharts.cpuTimeSeries.destroy();
        window.currentCharts.cpuTimeSeries = new Chart(ctx.getContext('2d'), {
            type: 'line',
            data: {
                labels: timeLabels,
                datasets: [
                    { label: 'Postgres CPU %', data: pgSeries, borderColor: '#14b8a6', fill: false, tension: 0.3, pointRadius: 0 }
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

        // Accumulate by query text
        const map = new Map();
        list.forEach(q => {
            const key = q.query || 'empty';
            if (!map.has(key)) {
                map.set(key, {
                    captured_at: q.captured_at,
                    user_name: q.user_name,
                    query: q.query,
                    total_exec_time: pgCpuNum(q.total_exec_time),
                    calls: parseInt(q.calls || 0, 10),
                    count: 1
                });
            } else {
                const existing = map.get(key);
                existing.total_exec_time += pgCpuNum(q.total_exec_time);
                existing.calls += parseInt(q.calls || 0, 10);
                existing.count += 1;
                // keep the latest timestamp
                if (new Date(q.captured_at) > new Date(existing.captured_at)) {
                    existing.captured_at = q.captured_at;
                    existing.user_name = q.user_name;
                }
            }
        });

        const sorted = Array.from(map.values()).sort((a, b) => b.total_exec_time - a.total_exec_time);

        tbody.innerHTML = sorted.slice(0, 10).map((q) => {
            const ts = q.captured_at ? new Date(q.captured_at).toLocaleTimeString() : '—';
            const avgMs = q.calls > 0 ? q.total_exec_time / q.calls : 0;
            return `
            <tr>
                <td><span class="badge badge-outline">${ts}</span></td>
                <td>${pgCpuEsc(q.user_name || '—')}</td>
                <td><code title="${pgCpuEsc(q.query)}">${pgCpuTrunc(q.query, 60)}</code></td>
                <td class="text-right">${q.total_exec_time.toFixed(1)}</td>
                <td class="text-right">${q.calls || '0'}</td>
                <td class="text-right">${avgMs.toFixed(2)}</td>
            </tr>`;
        }).join('');
    }
})();
