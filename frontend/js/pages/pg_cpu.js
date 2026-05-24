/*
 * SQL Optima — PostgreSQL CPU Health Monitor dashboard.
 * Sources: pgss_delta_1m (exec time proxy, query types, cache efficiency),
 *          postgres/control-center (sessions, TPS, dead tuples),
 *          postgres/server-info (health score, uptime).
 * OS-level CPU metrics are intentionally absent — requires OS Collector agent.
 */
(function() {
window.PgCpuView = async function() {
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
        const dbName = window.appState.currentDatabase || 'all';

        window.appState.activeViewId = 'pg-cpu';

        window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_cpu.html', { inst, dbName });

        // Attach listener for system query toggle
        const showSystemToggle = document.getElementById('pg-cpu-show-system');
        if (showSystemToggle) {
            showSystemToggle.onchange = () => {
                const subtitle = document.getElementById('pg-cpu-queries-subtitle');
                if (subtitle) {
                    subtitle.textContent = showSystemToggle.checked 
                        ? 'Ranked by total execution time · including system/internal queries'
                        : 'Ranked by total execution time · monitoring users excluded';
                }
                initPgCpu();
            };
        }

        window.initPageTimePicker();
        initPgCpu();
    };

    function esc(s) {
        return window.escapeHtml ? window.escapeHtml(String(s ?? '')) : String(s ?? '');
    }

    function trunc(s, n) {
        const t = String(s ?? '');
        return t.length <= n ? t : t.slice(0, n) + '…';
    }

    function num(v) {
        const x = Number(v);
        return Number.isFinite(x) ? x : 0;
    }

    // Query type code → human label + color
    const QUERY_TYPE_META = {
        S: { label: 'SELECT', color: '#3b82f6' },
        I: { label: 'INSERT', color: '#10b981' },
        U: { label: 'UPDATE', color: '#f59e0b' },
        D: { label: 'DELETE', color: '#ef4444' },
        E: { label: 'DDL',    color: '#8b5cf6' },
        O: { label: 'Other',  color: '#64748b' },
    };

    async function updateHeader(instName) {
        try {
            const res = await window.apiClient.authenticatedFetch(`/api/postgres/server-info?instance=${encodeURIComponent(instName)}`);
            if (!res.ok) return;
            const s = await res.json() || {};
            const el = document.getElementById('pg-uptime');
            if (el) el.textContent = s.uptime || 'N/A';
            const hs = s.health_score || 0;
            const badge = document.getElementById('pgHealthScoreBadge');
            if (badge) {
                badge.textContent = hs;
                badge.className = 'metric-value ' + (hs > 80 ? 'text-success' : hs > 60 ? 'text-warning' : 'text-danger');
            }
        } catch (e) { console.error('PG CPU header fetch:', e); }
    }

    async function checkPgCpuStatus(instName) {
        try {
            const resp = await window.apiClient.authenticatedFetch(`/api/postgres/pgss/status?instance=${encodeURIComponent(instName)}`);
            if (!resp.ok) return;
            const data = await resp.json();
            const banner = document.getElementById('pgss-status-banner');
            const msg    = document.getElementById('pgss-status-msg');
            if (!banner || !msg) return;
            if (!data.ready) {
                banner.style.display = '';
                msg.textContent = data.message || 'pg_stat_statements is not available — chart data will appear once the collector has 2+ snapshots.';
            } else {
                banner.style.display = 'none';
            }
        } catch (_) { /* non-fatal */ }
    }

    async function initPgCpu() {
        window.currentCharts = window.currentCharts || {};
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: '' };
        const q = encodeURIComponent(inst.name);

        let from = window.appState.fromTs;
        let to   = window.appState.toTs;
        if (from && from.includes('T') && !from.endsWith('Z')) from = new Date(from).toISOString();
        if (to   && to.includes('T')   && !to.endsWith('Z'))   to   = new Date(to).toISOString();

        const timeParams = (from && to) ? `&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}` : '&limit=60';
        
        const showSystem = document.getElementById('pg-cpu-show-system')?.checked === true;
        const systemParam = showSystem ? '&include_system=true' : '';

        updateHeader(inst.name);
        checkPgCpuStatus(inst.name);

        let pgTs = []; let dbRows = []; let topQueries = []; let qtypes = []; let cc = {};

        try {
            const [tsRes, dbRes, qRes, qtRes, ccRes] = await Promise.all([
                window.apiClient.authenticatedFetch(`/api/cpu/pg-timeseries?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/cpu/database?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/cpu/top-queries?instance=${q}${timeParams}&limit=20${systemParam}`),
                window.apiClient.authenticatedFetch(`/api/cpu/query-types?instance=${q}${timeParams}`),
                window.apiClient.authenticatedFetch(`/api/postgres/control-center?instance=${q}`),
            ]);
            if (tsRes.ok)  { const r = await tsRes.json();  pgTs      = r.points  || []; }
            if (dbRes.ok)  { const r = await dbRes.json();  dbRows    = r.rows    || []; }
            if (qRes.ok)   { const r = await qRes.json();   topQueries = r.queries || []; }
            if (qtRes.ok)  { const r = await qtRes.json();  qtypes    = r.types   || []; }
            if (ccRes.ok)  { const r = await ccRes.json();  cc        = r.stats   || r    || {}; }
        } catch (e) { console.error('PG CPU dashboard fetch:', e); }

        updateKpis(pgTs, cc);
        renderExecLoadChart(pgTs);
        renderDbShareDonut(dbRows);
        renderQueryTypeDonut(qtypes);
        renderCacheHitTrend(pgTs);
        renderTopQueries(topQueries);
    }

    function updateKpis(pgTs, cc) {
        // Health score comes from updateHeader; skip here.

        // Active sessions + TPS + waiting + dead tuples from control-center
        const activeEl = document.getElementById('kpi-active-conn');
        const tpsEl    = document.getElementById('kpi-tps');
        const waitEl   = document.getElementById('kpi-waiting');
        const deadEl   = document.getElementById('kpi-dead-tuple');
        if (activeEl) activeEl.textContent = String(cc.active_sessions ?? '--');
        if (tpsEl)    tpsEl.textContent    = num(cc.tps).toFixed(0);
        if (waitEl)   waitEl.textContent   = String(cc.waiting_sessions ?? '--');
        if (deadEl)   deadEl.textContent   = num(cc.dead_tuple_pct).toFixed(1) + '%';

        // Avg latency + cache hit from most recent pg-timeseries bucket
        const latEl   = document.getElementById('kpi-avg-latency');
        const cacheEl = document.getElementById('kpi-cache-hit');
        const cacheBadge = document.getElementById('pg-cache-badge');

        if (pgTs.length > 0) {
            const latest = pgTs[pgTs.length - 1];
            const avgMs  = num(latest.avg_exec_time_ms);
            const hitPct = num(latest.cache_hit_pct);

            if (latEl)   latEl.textContent  = avgMs.toFixed(1) + ' ms';
            if (cacheEl) {
                cacheEl.textContent = hitPct.toFixed(1) + '%';
                cacheEl.className   = 'metric-value ' + (hitPct >= 99 ? 'text-success' : hitPct >= 95 ? 'text-warning' : 'text-danger');
            }
            if (cacheBadge) {
                cacheBadge.textContent  = hitPct.toFixed(1) + '%';
                cacheBadge.className    = 'pg-cpu-cache-badge pg-cpu-cache-badge--' + (hitPct >= 99 ? 'ok' : hitPct >= 95 ? 'warn' : 'danger');
            }
        } else {
            // Fall back to control-center cache hit if available
            const ccHit = num(cc.cache_hit_ratio_pct);
            if (cacheEl && ccHit > 0) {
                cacheEl.textContent = ccHit.toFixed(1) + '%';
                cacheEl.className   = 'metric-value ' + (ccHit >= 99 ? 'text-success' : ccHit >= 95 ? 'text-warning' : 'text-danger');
            }
        }
    }

    function renderExecLoadChart(points) {
        const canvas = document.getElementById('pgExecLoadChart');
        if (!canvas) return;
        if (window.currentCharts.pgExecLoad) window.currentCharts.pgExecLoad.destroy();

        if (!points || points.length === 0) {
            renderNoData(canvas, 'No execution history for selected range — wait for the collector to populate data');
            return;
        }

        const labels    = points.map(p => p.bucket ? new Date(p.bucket).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}) : '');
        const execSec   = points.map(p => +(num(p.total_exec_time_ms) / 1000).toFixed(2));
        const avgMs     = points.map(p => +num(p.avg_exec_time_ms).toFixed(2));

        // Update bucket-size badge
        const bucketBadge = document.getElementById('pg-ts-bucket-label');
        if (bucketBadge && points.length >= 2) {
            const diffMs = new Date(points[1].bucket) - new Date(points[0].bucket);
            const mins   = Math.round(diffMs / 60000);
            bucketBadge.textContent = mins === 1 ? '1 min buckets' : `${mins} min buckets`;
        }

        window.currentCharts.pgExecLoad = new Chart(canvas.getContext('2d'), {
            type: 'bar',
            data: {
                labels,
                datasets: [
                    {
                        label: 'Total Exec (s)',
                        data: execSec,
                        backgroundColor: 'rgba(20, 184, 166, 0.45)',
                        borderColor: '#14b8a6',
                        borderWidth: 1,
                        yAxisID: 'y',
                        order: 2,
                    },
                    {
                        label: 'Avg Latency (ms)',
                        data: avgMs,
                        type: 'line',
                        borderColor: '#f59e0b',
                        borderWidth: 1.5,
                        pointRadius: 0,
                        fill: false,
                        tension: 0.3,
                        yAxisID: 'y2',
                        order: 1,
                    },
                ],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: { mode: 'index', intersect: false },
                plugins: {
                    legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 } } },
                    tooltip: {
                        callbacks: {
                            label: ctx => ctx.dataset.label + ': ' + ctx.parsed.y,
                        },
                    },
                },
                scales: {
                    x: { display: true, ticks: { maxTicksLimit: 8, font: { size: 9 } }, grid: { display: false } },
                    y: {
                        beginAtZero: true,
                        position: 'left',
                        title: { display: true, text: 'Exec Time (s)', font: { size: 9 } },
                        ticks: { font: { size: 9 } },
                    },
                    y2: {
                        beginAtZero: true,
                        position: 'right',
                        title: { display: true, text: 'Avg Latency (ms)', font: { size: 9 } },
                        ticks: { font: { size: 9 } },
                        grid: { drawOnChartArea: false },
                    },
                },
            },
        });
    }

    function renderDbShareDonut(dbRows) {
        const canvas = document.getElementById('cpuDbDonutChart');
        if (!canvas) return;
        if (window.currentCharts.cpuDbDonut) window.currentCharts.cpuDbDonut.destroy();
        const rows   = (dbRows || []).filter(r => num(r.total_exec_time_ms) > 0);
        const labels = rows.map(r => r.datname || 'unknown');
        const data   = rows.map(r => num(r.total_exec_time_ms));
        window.currentCharts.cpuDbDonut = new Chart(canvas.getContext('2d'), {
            type: 'doughnut',
            data: {
                labels: labels.length ? labels : ['No data'],
                datasets: [{ data: data.length ? data : [1], backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ef4444', '#14b8a6'] }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'right', labels: { boxWidth: 10, font: { size: 9 } } } },
                cutout: '68%',
            },
        });
    }

    function renderQueryTypeDonut(types) {
        const canvas = document.getElementById('queryTypeDonutChart');
        if (!canvas) return;
        if (window.currentCharts.queryTypeDonut) window.currentCharts.queryTypeDonut.destroy();

        const rows   = (types || []).filter(r => num(r.total_exec_time_ms) > 0);
        const labels = rows.map(r => (QUERY_TYPE_META[r.query_type] || { label: r.query_type }).label);
        const data   = rows.map(r => num(r.total_exec_time_ms));
        const colors = rows.map(r => (QUERY_TYPE_META[r.query_type] || { color: '#64748b' }).color);

        window.currentCharts.queryTypeDonut = new Chart(canvas.getContext('2d'), {
            type: 'doughnut',
            data: {
                labels: labels.length ? labels : ['No data'],
                datasets: [{ data: data.length ? data : [1], backgroundColor: colors.length ? colors : ['#64748b'] }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { position: 'bottom', labels: { boxWidth: 10, font: { size: 9 } } },
                    tooltip: {
                        callbacks: {
                            label: ctx => {
                                const total = ctx.dataset.data.reduce((a, b) => a + b, 0);
                                const pct   = total > 0 ? ((ctx.parsed / total) * 100).toFixed(1) : 0;
                                return `${ctx.label}: ${pct}%`;
                            },
                        },
                    },
                },
                cutout: '62%',
            },
        });
    }

    function renderCacheHitTrend(points) {
        const canvas = document.getElementById('cacheHitTrendChart');
        if (!canvas) return;
        if (window.currentCharts.cacheHitTrend) window.currentCharts.cacheHitTrend.destroy();

        if (!points || points.length === 0) {
            renderNoData(canvas, 'No cache hit data for selected range — wait for the collector to populate data');
            return;
        }

        const labels  = points.map(p => p.bucket ? new Date(p.bucket).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}) : '');
        const hitPcts = points.map(p => +num(p.cache_hit_pct).toFixed(2));

        // Color each point based on health threshold
        const pointColors = hitPcts.map(v => v >= 99 ? '#10b981' : v >= 95 ? '#f59e0b' : '#ef4444');

        window.currentCharts.cacheHitTrend = new Chart(canvas.getContext('2d'), {
            type: 'line',
            data: {
                labels,
                datasets: [
                    {
                        label: 'Cache Hit %',
                        data: hitPcts,
                        borderColor: '#14b8a6',
                        borderWidth: 1.5,
                        pointRadius: hitPcts.length <= 60 ? 2 : 0,
                        pointBackgroundColor: pointColors,
                        fill: true,
                        backgroundColor: 'rgba(20,184,166,0.08)',
                        tension: 0.3,
                    },
                ],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    annotation: {
                        // Requires chartjs-plugin-annotation if available; safe to omit
                    },
                },
                scales: {
                    x: { display: true, ticks: { maxTicksLimit: 8, font: { size: 9 } }, grid: { display: false } },
                    y: {
                        min: Math.max(0, Math.min(...hitPcts.filter(v => v > 0)) - 5),
                        max: 100,
                        ticks: { font: { size: 9 }, callback: v => v + '%' },
                    },
                },
            },
        });
    }

    function renderTopQueries(queries) {
        const tbody = document.getElementById('cpu-top-queries-body');
        if (!tbody) return;
        const list = queries || [];
        if (list.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted" style="padding:1.5rem;">No user query data for selected time range</td></tr>';
            return;
        }

        // Accumulate by query text (normalise duplicates across capture windows)
        const map = new Map();
        list.forEach(q => {
            const key = q.query || 'empty';
            if (!map.has(key)) {
                map.set(key, {
                    captured_at:      q.captured_at,
                    user_name:        q.user_name,
                    query:            q.query,
                    total_exec_time:  num(q.total_exec_time),
                    calls:            parseInt(q.calls || 0, 10),
                });
            } else {
                const ex = map.get(key);
                ex.total_exec_time += num(q.total_exec_time);
                ex.calls           += parseInt(q.calls || 0, 10);
                if (new Date(q.captured_at) > new Date(ex.captured_at)) {
                    ex.captured_at = q.captured_at;
                    ex.user_name   = q.user_name;
                }
            }
        });

        const sorted = Array.from(map.values()).sort((a, b) => b.total_exec_time - a.total_exec_time);

        tbody.innerHTML = sorted.slice(0, 15).map(q => {
            const ts    = q.captured_at ? new Date(q.captured_at).toLocaleTimeString() : '—';
            const avgMs = q.calls > 0 ? (q.total_exec_time / q.calls).toFixed(2) : '0.00';
            const avgClass = parseFloat(avgMs) > 1000 ? 'text-danger' : parseFloat(avgMs) > 100 ? 'text-warning' : 'text-success';
            return `
            <tr>
                <td><span class="badge badge-outline">${ts}</span></td>
                <td>${esc(q.user_name || '—')}</td>
                <td><code class="pg-cpu-query-link" title="${esc(q.query)}">${trunc(esc(q.query), 80)}</code></td>
                <td class="text-right">${q.total_exec_time.toFixed(1)}</td>
                <td class="text-right">${q.calls || 0}</td>
                <td class="text-right ${avgClass}">${avgMs}</td>
            </tr>`;
        }).join('');
    }

    function renderNoData(canvas, msg) {
        // Use the CSS-rendered size of the parent container so the text is
        // always visible even before Chart.js has had a chance to size the canvas.
        const parent = canvas.parentElement;
        const w = parent ? parent.offsetWidth || 300 : (canvas.offsetWidth || 300);
        const h = parent ? parent.offsetHeight || 150 : (canvas.offsetHeight || 150);
        canvas.width = w;
        canvas.height = h;
        const ctx = canvas.getContext('2d');
        ctx.clearRect(0, 0, w, h);
        ctx.fillStyle = 'rgba(100,116,139,0.35)';
        ctx.font = '11px sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText(msg, w / 2, h / 2);
    }
})();
