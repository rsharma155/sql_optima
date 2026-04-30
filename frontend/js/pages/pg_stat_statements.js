// pg_stat_statements Dashboard — frontend JS module
// Pattern: window.PgStatStatementsView + Chart.js + apiClient.authenticatedFetch

window.pgssOpenQueryDetail = function(sql) {
    if (window.showQueryModal) {
        window.showQueryModal(sql);
    } else {
        alert(sql);
    }
};

window.pgssApplyTimeRange = function () {
    loadPgssAll();
};

window.pgssSort = function (sort, el) {
    document.querySelectorAll('#pgss-sort-tabs li').forEach(t => t.classList.remove('active'));
    if (el) el.classList.add('active');
    window._pgssCurrentSort = sort;
    loadPgssControlCenterTopQueries();
};

// ---- internal state ----
window._pgssCurrentSort = 'total_time';
window._pgssRefreshTimer = null;

window.PgStatStatementsView = async function () {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'postgres') {
        window.routerOutlet.innerHTML = '<div class="p-4 text-warning">Please select a PostgreSQL instance first.</div>';
        return;
    }

    // Set default time window (last 1 hour) if not set
    if (!window.appState.pgssFrom) {
        const now = new Date();
        const oneHourAgo = new Date(now.getTime() - 3600000);
        window.appState.pgssFrom = toLocalISOString(oneHourAgo);
        window.appState.pgssTo = toLocalISOString(now);
    }

    window.routerOutlet.innerHTML = await window.loadTemplate('pages/pg_stat_statements.html');
    
    const fromEl = document.getElementById('pgss-from');
    const toEl = document.getElementById('pgss-to');
    if (fromEl) fromEl.value = window.appState.pgssFrom;
    if (toEl) toEl.value = window.appState.pgssTo;

    const label = document.getElementById('pgss-instance-label');
    if (label) label.textContent = `(${inst.name})`;

    // Check if PGSS is ready
    await checkPgssStatus(inst.name);

    window.currentCharts = window.currentCharts || {};
    
    // Bind events
    pgssBindEvents();

    await loadPgssAll();

    // Auto-refresh every 60s
    if (window._pgssRefreshTimer) clearInterval(window._pgssRefreshTimer);
    window._pgssRefreshTimer = setInterval(() => {
        if (window.location.hash.includes('pg-stat-statements')) {
            loadPgssAll();
        }
    }, 60000);
};

function toLocalISOString(date) {
    const tzo = -date.getTimezoneOffset(),
        dif = tzo >= 0 ? '+' : '-',
        pad = function(num) {
            return (num < 10 ? '0' : '') + num;
        };
    return date.getFullYear() +
        '-' + pad(date.getMonth() + 1) +
        '-' + pad(date.getDate()) +
        'T' + pad(date.getHours()) +
        ':' + pad(date.getMinutes()) +
        ':' + pad(date.getSeconds());
}

function getTimeRange() {
    const fromEl = document.getElementById('pgss-from');
    const toEl = document.getElementById('pgss-to');
    
    // Store in appState for persistence
    if (fromEl?.value) window.appState.pgssFrom = fromEl.value;
    if (toEl?.value) window.appState.pgssTo = toEl.value;

    // Convert local datetime-local to ISO UTC for backend
    const from = fromEl?.value ? new Date(fromEl.value).toISOString() : new Date(Date.now() - 3600000).toISOString();
    const to = toEl?.value ? new Date(toEl.value).toISOString() : new Date().toISOString();
    return { from, to };
}

function pgssBindEvents() {
    const topTbody = document.getElementById('pgss-top-tbody');
    if (topTbody) {
        topTbody.addEventListener('click', (e) => {
            const target = e.target.closest('[data-action="view-query"]');
            if (target) {
                const query = target.getAttribute('data-query');
                if (query) window.pgssOpenQueryDetail(query);
            }
        });
    }
}

async function checkPgssStatus(instance) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/pgss/status?instance=${encodeURIComponent(instance)}`);
        const status = await resp.json();
        const banner = document.getElementById('pgss-status-banner');
        const msg = document.getElementById('pgss-status-msg');
        if (banner && !status.ready) {
            banner.style.display = 'block';
            msg.textContent = status.message || 'pg_stat_statements is not fully enabled.';
        } else if (banner) {
            banner.style.display = 'none';
        }
    } catch (_) { /* non-fatal */ }
}

async function loadPgssAll() {
    const rangeLabel = document.getElementById('pgss-range-label');
    const { from, to } = getTimeRange();
    if (rangeLabel) {
        rangeLabel.textContent = `${new Date(from).toLocaleString()} - ${new Date(to).toLocaleString()}`;
    }

    await Promise.all([
        loadPgssWorkload(),
        loadPgssLatency(),
        loadPgssControlCenterTopQueries(),
        loadPgssRegressions()
    ]);
}

async function loadPgssWorkload() {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
    const { from, to } = getTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/pgss/workload?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`);
        const data = await resp.json();
        const pts = data.points || [];
        const labels = pts.map(p => new Date(p.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));

        renderLineChart('pgss-chart-load', labels, [
            { label: 'Query Load (ms/s)', data: pts.map(p => p.query_load_ms_sec), borderColor: '#38bdf8', backgroundColor: 'rgba(56,189,248,0.1)', fill: true }
        ]);

        renderLineChart('pgss-chart-qps', labels, [
            { label: 'QPS', data: pts.map(p => p.qps), borderColor: '#10b981', yAxisID: 'y' },
            { label: 'Rows/s', data: pts.map(p => p.rows_sec), borderColor: '#f59e0b', yAxisID: 'y1' }
        ], true);

        renderLineChart('pgss-chart-cache', labels, [
            { label: 'Cache Hit %', data: pts.map(p => p.cache_hit_ratio), borderColor: '#8b5cf6', tension: 0.4 }
        ]);

        renderLineChart('pgss-chart-wal', labels, [
            { label: 'WAL MB/s', data: pts.map(p => p.wal_bytes_sec / (1024*1024)), borderColor: '#f43f5e' }
        ]);

        renderStackedArea('pgss-chart-execplan', labels, [
            { label: 'Exec %', data: pts.map(p => p.exec_pct), backgroundColor: 'rgba(16,185,129,0.4)', borderColor: '#10b981' },
            { label: 'Plan %', data: pts.map(p => p.plan_pct), backgroundColor: 'rgba(56,189,248,0.4)', borderColor: '#38bdf8' }
        ]);

    } catch (e) { console.error('PgssWorkload failed', e); }
}

async function loadPgssLatency() {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
    const { from, to } = getTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/pgss/latency?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`);
        const data = await resp.json();
        const pts = data.points || [];
        const labels = pts.map(p => new Date(p.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));

        renderLineChart('pgss-chart-latency', labels, [
            { label: 'P99', data: pts.map(p => p.p99), borderColor: '#ef4444' },
            { label: 'P95', data: pts.map(p => p.p95), borderColor: '#f59e0b' },
            { label: 'P50', data: pts.map(p => p.p50), borderColor: '#10b981' }
        ]);
    } catch (e) { console.error('PgssLatency failed', e); }
}

async function loadPgssControlCenterTopQueries(customFrom, customTo) {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
    let { from, to } = getTimeRange();
    if (customFrom) from = customFrom;
    if (customTo) to = customTo;

    const sort = window._pgssCurrentSort || 'total_time';
    const tbody = document.getElementById('pgss-top-tbody');
    if (!tbody) return;
    
    tbody.innerHTML = '<tr><td colspan="11" class="text-center p-4"><div class="spinner"></div></td></tr>';

    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/top?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}&sort=${sort}`
        );
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="11">Error loading data</td></tr>'; return; }
        const data = await resp.json();
        const queries = data.queries || [];
        if (queries.length === 0) {
            tbody.innerHTML = '<tr><td colspan="11" class="text-center text-muted p-4">No data for selected time range</td></tr>';
            return;
        }
        tbody.innerHTML = queries.map((q, i) => {
            const flags = (q.flags || []).map(f => `<span class="badge badge-outline-warning" style="font-size:0.6rem;padding:1px 4px;margin:0 1px;">${f}</span>`).join('');
            
            return `
            <tr>
                <td class="pgss-col-id text-muted">${i + 1}</td>
                <td class="pgss-col-flags text-center">${flags}</td>
                <td class="pgss-col-query" data-action="view-query" data-query="${escapeHtml(q.query)}">
                    <div class="pgss-query-text" title="Click to view full SQL">${escapeHtml(truncate(q.query, 200))}</div>
                </td>
                <td class="pgss-col-stat">${fmtMs(q.total_time_ms)}</td>
                <td class="pgss-col-stat">${q.pct_db_time.toFixed(1)}%</td>
                <td class="pgss-col-stat">${fmtNum(q.calls)}</td>
                <td class="pgss-col-stat" style="font-weight:600;color:var(--accent);">${fmtMs(q.avg_ms)}</td>
                <td class="pgss-col-stat">${q.rows_per_call?.toFixed(1) ?? '-'}</td>
                <td class="pgss-col-stat">${q.hit_pct?.toFixed(1) ?? '-'}%</td>
                <td class="pgss-col-stat">${q.temp_mb?.toFixed(2) ?? '0'}</td>
                <td class="pgss-col-stat">${q.wal_mb?.toFixed(2) ?? '0'}</td>
            </tr>
        `}).join('');
    } catch (_) { tbody.innerHTML = '<tr><td colspan="11">Error</td></tr>'; }
}

async function loadPgssRegressions() {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
    const tbody = document.getElementById('pgss-regression-tbody');
    if (!tbody) return;
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/regressions?instance=${encodeURIComponent(inst.name)}`
        );
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="5">Error loading</td></tr>'; return; }
        const data = await resp.json();
        const regs = data.regressions || [];
        if (regs.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" class="text-center text-muted p-3">No regressions detected</td></tr>';
            return;
        }
        tbody.innerHTML = regs.map(r => `
            <tr>
                <td title="${escapeHtml(r.query)}">${escapeHtml(truncate(r.query, 80))}</td>
                <td>${fmtMs(r.prev_avg_ms)}</td>
                <td>${fmtMs(r.curr_avg_ms)}</td>
                <td class="${r.change_pct > 100 ? 'text-danger' : 'text-warning'}">+${r.change_pct.toFixed(0)}%</td>
                <td><span class="badge ${r.status === 'Degraded' ? 'badge-danger' : 'badge-warning'}">${r.status}</span></td>
            </tr>
        `).join('');
    } catch (_) { tbody.innerHTML = '<tr><td colspan="5">Error</td></tr>'; }
}

// ============================================================
// Chart Helpers
// ============================================================
function renderLineChart(canvasId, labels, datasets, dualAxis) {
    destroyChart(canvasId);
    const ctx = document.getElementById(canvasId);
    if (!ctx) return;
    const cfg = {
        type: 'line',
        data: { labels, datasets: datasets.map(ds => ({ tension: 0.3, pointRadius: 0, borderWidth: 1.5, fill: ds.fill || false, ...ds })) },
        options: {
            responsive: true, maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            onClick: (e, elements) => {
                if (elements.length > 0) {
                    const idx = elements[0].index;
                    const timeLabel = labels[idx];
                    // On click, filter top queries to that specific minute
                    const originalTo = new Date(getTimeRange().to);
                    // This is a bit simplified, but demonstrates the time-series navigation
                    console.log('Filtering Top Queries for:', timeLabel);
                    // For a real implementation, we'd need the exact timestamp from pts[idx]
                }
            },
            plugins: { legend: { labels: { color: '#94a3b8', font: { size: 11 } } } },
            scales: {
                x: { ticks: { color: '#64748b', maxTicksLimit: 12 }, grid: { color: 'rgba(100,116,139,0.15)' } },
                y: { ticks: { color: '#64748b' }, grid: { color: 'rgba(100,116,139,0.15)' } }
            }
        }
    };
    if (dualAxis) {
        cfg.options.scales.y1 = { position: 'right', ticks: { color: '#64748b' }, grid: { drawOnChartArea: false } };
    }
    window.currentCharts[canvasId] = new Chart(ctx, cfg);
}

function renderStackedArea(canvasId, labels, datasets) {
    destroyChart(canvasId);
    const ctx = document.getElementById(canvasId);
    if (!ctx) return;
    window.currentCharts[canvasId] = new Chart(ctx, {
        type: 'line',
        data: { labels, datasets: datasets.map(ds => ({ fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1, ...ds })) },
        options: {
            responsive: true, maintainAspectRatio: false,
            plugins: { legend: { labels: { color: '#94a3b8', font: { size: 11 } } } },
            scales: {
                x: { stacked: true, ticks: { color: '#64748b', maxTicksLimit: 12 }, grid: { color: 'rgba(100,116,139,0.15)' } },
                y: { stacked: true, max: 100, ticks: { color: '#64748b' }, grid: { color: 'rgba(100,116,139,0.15)' } }
            }
        }
    });
}

function destroyChart(id) {
    if (window.currentCharts && window.currentCharts[id]) {
        window.currentCharts[id].destroy();
        delete window.currentCharts[id];
    }
}

function escapeHtml(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}
function truncate(s, n) {
    if (!s) return '';
    return s.length > n ? s.slice(0, n) + '…' : s;
}
function fmtMs(ms) {
    if (ms == null) return '-';
    if (ms >= 1000) return (ms / 1000).toFixed(2) + ' s';
    return ms.toFixed(2) + ' ms';
}
function fmtNum(n) {
    if (n == null) return '-';
    return n.toLocaleString();
}
