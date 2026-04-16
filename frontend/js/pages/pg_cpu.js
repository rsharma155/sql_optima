/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL CPU utilization dashboard (host vs Postgres, saturation, DB share, top queries).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgCpuView = async function() {
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_cpu.html');
    setTimeout(initPgCpu, 50);
};

function pgCpuEsc(s) {
    if (typeof window.escapeHtml === 'function') {
        return window.escapeHtml(String(s ?? ''));
    }
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
}

function pgCpuTrunc(s, n) {
    const t = String(s ?? '');
    if (t.length <= n) return t;
    return t.slice(0, n) + '…';
}

function pgCpuNum(v) {
    const x = Number(v);
    return Number.isFinite(x) ? x : 0;
}

async function initPgCpu() {
    window.currentCharts = window.currentCharts || {};
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: '' };
    const q = encodeURIComponent(inst.name);

    document.getElementById('last-updated').textContent = new Date().toLocaleString();

    let sat = {};
    let points = [];
    let dbRows = [];
    let topQueries = [];

    try {
        const [histRes, satRes, dbRes, qRes] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/cpu/history?instance=${q}&limit=60`),
            window.apiClient.authenticatedFetch(`/api/cpu/saturation?instance=${q}`),
            window.apiClient.authenticatedFetch(`/api/cpu/database?instance=${q}`),
            window.apiClient.authenticatedFetch(`/api/cpu/top-queries?instance=${q}&limit=20`),
        ]);

        if (histRes.ok && (histRes.headers.get('content-type') || '').includes('application/json')) {
            const p = await histRes.json();
            points = Array.isArray(p.points) ? p.points : [];
        }
        if (satRes.ok && (satRes.headers.get('content-type') || '').includes('application/json')) {
            sat = await satRes.json();
        }
        if (dbRes.ok && (dbRes.headers.get('content-type') || '').includes('application/json')) {
            const d = await dbRes.json();
            dbRows = Array.isArray(d.rows) ? d.rows : [];
        }
        if (qRes.ok && (qRes.headers.get('content-type') || '').includes('application/json')) {
            const t = await qRes.json();
            topQueries = Array.isArray(t.queries) ? t.queries : [];
        }
    } catch (e) {
        console.error('PG CPU dashboard fetch failed:', e);
    }

    updatePgCpuKpis(sat);
    renderPgCpuLineChart(points);
    renderPgCpuDonut(dbRows);
    renderPgCpuTopQueries(topQueries);
}

function pgCpuShowQueryModal(row) {
    const existing = document.getElementById('pg-cpu-query-modal');
    if (existing) existing.remove();

    const qtext = String(row?.query || '');
    const qid = row?.queryid != null ? String(row.queryid) : '—';
    const user = row?.user_name != null ? String(row.user_name) : '';
    const ts = row?.captured_at ? new Date(row.captured_at).toLocaleString() : '';
    const total = pgCpuNum(row?.total_exec_time).toFixed(1);
    const calls = row?.calls != null ? String(row.calls) : '—';
    const avg = pgCpuNum(row?.avg_ms).toFixed(3);

    const modal = document.createElement('div');
    modal.id = 'pg-cpu-query-modal';
    modal.style.cssText = 'position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background:rgba(0,0,0,0.8); display:flex; align-items:center; justify-content:center;';
    modal.innerHTML = `
        <div style="background:var(--bg-surface); margin:2%; padding:16px; border:1px solid var(--border-color,#333); border-radius:12px; width:95%; max-width:980px; max-height:90vh; overflow-y:auto; color:var(--text-primary,#e0e0e0);">
            <div style="display:flex; justify-content:space-between; align-items:center; gap:12px;">
                <div>
                    <h3 style="margin:0; color:var(--accent-blue, #3b82f6); font-size:1.05rem;"><i class="fa-solid fa-code"></i> Query Details</h3>
                    <div class="text-muted" style="font-size:0.75rem; margin-top:0.2rem;">
                        ${pgCpuEsc(ts)} ${user ? `• user: <strong>${pgCpuEsc(user)}</strong>` : ''} • queryid: <code>${pgCpuEsc(qid)}</code>
                        • total: <strong>${pgCpuEsc(total)} ms</strong> • calls: <strong>${pgCpuEsc(calls)}</strong> • avg: <strong>${pgCpuEsc(avg)} ms</strong>
                    </div>
                </div>
                <button data-action="close-id" data-target="pg-cpu-query-modal" style="background:transparent; border:1px solid var(--border-color); color:var(--text-primary); font-size:1.25rem; cursor:pointer; padding:0.25rem 0.6rem; border-radius:4px;">&times;</button>
            </div>
            <hr style="border:0; border-top:1px solid var(--border-color,#333); margin:12px 0;">
            <pre style="white-space:pre-wrap; word-break:break-word; font-size:0.8rem; line-height:1.35; margin:0;">${pgCpuEsc(qtext)}</pre>
        </div>
    `;
    document.body.appendChild(modal);
    modal.addEventListener('click', (e) => { if (e.target === modal) modal.remove(); });
}

function updatePgCpuKpis(sat) {
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

    if (hostEl) hostEl.textContent = host > 0 ? host.toFixed(1) + '%' : 'N/A';
    if (pgEl) pgEl.textContent = pg > 0 ? pg.toFixed(1) + '%' : 'N/A';
    if (connEl) connEl.textContent = String(active);
    if (perEl) perEl.textContent = perConn > 0 ? perConn.toFixed(2) + '%' : (active > 0 ? '0%' : 'N/A');

    if (loadEl) {
        loadEl.textContent = (load1 > 0 || cores > 0) ? `${load1.toFixed(2)} / ${cores}` : 'N/A';
    }

    if (badge) {
        badge.textContent = satPct > 0 ? satPct.toFixed(0) + '%' : 'N/A';
        badge.classList.remove('saturation-badge--danger', 'saturation-badge--warn', 'saturation-badge--ok', 'saturation-badge--muted');
        if (satPct <= 0 && cores === 0) {
            badge.classList.add('saturation-badge--muted');
        } else if (satPct > 100) {
            badge.classList.add('saturation-badge--danger');
        } else if (satPct > 80) {
            badge.classList.add('saturation-badge--warn');
        } else {
            badge.classList.add('saturation-badge--ok');
        }
    }
}

function renderPgCpuLineChart(points) {
    const rowsAsc = (points || []).slice().reverse();
    const timeLabels = rowsAsc.map((r) => {
        const ts = r.capture_timestamp;
        return ts ? new Date(ts).toLocaleTimeString() : '';
    });

    const hostSeries = rowsAsc.map((r) => {
        const h = pgCpuNum(r.host_cpu_percent);
        if (h > 0) return h;
        return pgCpuNum(r.cpu_usage);
    });
    const pgSeries = rowsAsc.map((r) => pgCpuNum(r.postgres_cpu_percent));

    const ctx = document.getElementById('cpuTimeSeriesChart');
    if (!ctx) return;

    if (window.currentCharts.cpuTimeSeries) {
        window.currentCharts.cpuTimeSeries.destroy();
    }

    const accent = window.getCSSVar ? window.getCSSVar('--accent-blue') : '#3b82f6';
    const accent2 = window.getCSSVar ? window.getCSSVar('--accent-teal') : '#14b8a6';

    window.currentCharts.cpuTimeSeries = new Chart(ctx.getContext('2d'), {
        type: 'line',
        data: {
            labels: timeLabels,
            datasets: [
                {
                    label: 'Host CPU %',
                    data: hostSeries,
                    borderColor: accent,
                    backgroundColor: 'rgba(59, 130, 246, 0.08)',
                    fill: true,
                    tension: 0.35,
                    pointRadius: 0,
                },
                {
                    label: 'Postgres CPU %',
                    data: pgSeries,
                    borderColor: accent2,
                    backgroundColor: 'rgba(20, 184, 166, 0.06)',
                    fill: true,
                    tension: 0.35,
                    pointRadius: 0,
                },
            ],
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            plugins: { legend: { position: 'top' } },
            scales: {
                y: {
                    beginAtZero: true,
                    max: 100,
                    title: { display: true, text: 'CPU %' },
                    ticks: { callback: (v) => v + '%' },
                },
                x: {
                    title: { display: true, text: 'Time' },
                },
            },
        },
    });
}

function renderPgCpuDonut(dbRows) {
    const canvas = document.getElementById('cpuDbDonutChart');
    if (!canvas) return;

    if (window.currentCharts.cpuDbDonut) {
        window.currentCharts.cpuDbDonut.destroy();
    }

    const rows = (dbRows || []).filter((r) => pgCpuNum(r.total_exec_time_ms) > 0);
    if (rows.length === 0) {
        window.currentCharts.cpuDbDonut = new Chart(canvas.getContext('2d'), {
            type: 'doughnut',
            data: {
                labels: ['No data'],
                datasets: [{ data: [1], backgroundColor: ['rgba(148,163,184,0.35)'] }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'right' } },
            },
        });
        return;
    }

    const labels = rows.map((r) => r.datname || 'unknown');
    const data = rows.map((r) => pgCpuNum(r.total_exec_time_ms));
    const palette = [
        'rgba(59, 130, 246, 0.85)',
        'rgba(20, 184, 166, 0.85)',
        'rgba(245, 158, 11, 0.85)',
        'rgba(168, 85, 247, 0.85)',
        'rgba(239, 68, 68, 0.85)',
        'rgba(34, 197, 94, 0.85)',
        'rgba(236, 72, 153, 0.85)',
        'rgba(99, 102, 241, 0.85)',
    ];
    const bg = labels.map((_, i) => palette[i % palette.length]);

    window.currentCharts.cpuDbDonut = new Chart(canvas.getContext('2d'), {
        type: 'doughnut',
        data: {
            labels,
            datasets: [{ data, backgroundColor: bg, borderWidth: 1 }],
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { position: 'right' },
                tooltip: {
                    callbacks: {
                        label(ctx) {
                            const v = ctx.parsed;
                            return `${ctx.label}: ${v.toFixed(0)} ms`;
                        },
                    },
                },
            },
        },
    });
}

function renderPgCpuTopQueries(queries) {
    const tbody = document.getElementById('cpu-top-queries-body');
    if (!tbody) return;

    const list = queries || [];
    if (list.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">No pg_stat_statements data</td></tr>';
        return;
    }

    window.pgCpuTopQueriesCache = list;
    tbody.innerHTML = list
        .map((q, idx) => {
            const ts = q.captured_at ? new Date(q.captured_at).toLocaleString() : '—';
            const user = q.user_name != null && String(q.user_name).trim() !== '' ? String(q.user_name) : '—';
            const qid = q.queryid != null ? String(q.queryid) : '—';
            const qt = pgCpuTrunc(q.query || '', 120);
            const tot = pgCpuNum(q.total_exec_time);
            const calls = q.calls != null ? String(q.calls) : '—';
            const avg = pgCpuNum(q.avg_ms);
            return `
            <tr>
                <td>${pgCpuEsc(ts)}</td>
                <td>${pgCpuEsc(user)}</td>
                <td><code>${pgCpuEsc(qid)}</code></td>
                <td style="max-width:360px;word-break:break-word;">
                    <span class="pg-cpu-query-link" role="button" tabindex="0"
                          data-action="pg-cpu-query-details" data-idx="${idx}"
                          onkeydown="if(event.key==='Enter'){window.pgCpuShowQueryDetails(${idx});}">
                        ${pgCpuEsc(qt)}
                    </span>
                </td>
                <td>${tot.toFixed(1)}</td>
                <td>${pgCpuEsc(calls)}</td>
                <td>${avg.toFixed(3)}</td>
            </tr>`;
        })
        .join('');
}

window.pgCpuShowQueryDetails = function(idx) {
    const list = window.pgCpuTopQueriesCache || [];
    const row = list[idx];
    if (!row) return;
    pgCpuShowQueryModal(row);
};
