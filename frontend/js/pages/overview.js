/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL Control Center (Overview) — Mission Control Revamp v2.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgDashboardView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
    const instName = inst.name;
    const dbName = window.appState.currentDatabase || 'all';

    window.appState.activeViewId = 'pg-dashboard';

    const shellId = `pg-dashboard-shell-${instName}-${dbName}`;
    const needsShell = !document.getElementById(shellId);

    if (needsShell) {
        window.routerOutlet.innerHTML = await window.loadTemplate('/pages/overview.html', { inst, dbName });
        const shellContainer = window.routerOutlet.querySelector('.page-view');
        if (shellContainer) shellContainer.id = shellId;

        const actions = document.querySelector('.dashboard-page-title-actions');
        if (actions) {
            const insertion = document.createElement('div');
            insertion.id = 'time-picker-insertion-point';
            actions.prepend(insertion);
            window.initPageTimePicker();
        }

        // Attach BG worker toggle listener
        const bgToggle = document.getElementById('pgLoadHideBgWorkers');
        if (bgToggle) {
            bgToggle.onchange = () => window.PgDashboardView();
        }
    }

    const fromVal = window.appState.fromTs;
    const toVal = window.appState.toTs;

    let dashboardData = {};
    try {
        if (window.appState.fetchingPgMetrics) return;
        window.appState.fetchingPgMetrics = true;

        let statsUrl = `/api/postgres/control-center?instance=${encodeURIComponent(instName)}`;
        let histUrl = `/api/postgres/control-center/history?instance=${encodeURIComponent(instName)}&limit=120`;
        if (fromVal && toVal) {
            const fromISO = encodeURIComponent(new Date(fromVal).toISOString());
            const toISO = encodeURIComponent(new Date(toVal).toISOString());
            statsUrl += `&from=${fromISO}&to=${toISO}`;
            histUrl += `&from=${fromISO}&to=${toISO}`;
        }

        const [statsResp, histResp] = await Promise.all([
            window.apiClient.authenticatedFetch(statsUrl),
            window.apiClient.authenticatedFetch(histUrl)
        ]);

        if (statsResp.ok) dashboardData = await statsResp.json() || {};
        if (histResp.ok) {
            const h = await histResp.json();
            if (h && h.labels && h.labels.length > 0) dashboardData.history = h;
        }
    } catch (e) {
        console.error("PG Control Center fetch failed:", e);
    } finally {
        window.appState.fetchingPgMetrics = false;
    }

    await updateDashboardV2(instName, dashboardData, fromVal, toVal);

    const dynInterval = window.collectorConfig ? window.collectorConfig.getInterval("Postgres System Metrics", 30000) : 30000;
    if (window.pgDashboardInterval) clearInterval(window.pgDashboardInterval);
    window.pgDashboardInterval = window.registerInterval(() => {
        if (window.appState.activeViewId === 'pg-dashboard') {
            window.PgDashboardView();
        } else {
            clearInterval(window.pgDashboardInterval);
        }
    }, dynInterval);
};

async function updateDashboardV2(instName, dashboardData, from, to) {
    if (!dashboardData) return;
    const s = dashboardData.stats || {};
    const h = dashboardData.history || {};

    // 1. Update KPI Strip (10 cards)
    const updateVal = (id, val, parseFn = (v)=>v) => {
        const el = document.getElementById(id);
        if (el) el.textContent = val !== undefined && val !== null ? parseFn(val) : '--';
    };

    updateVal('stat-health-score', s.health_score);
    const healthValEl = document.getElementById('stat-health-score');
    if (healthValEl) {
        const hs = s.health_score || 0;
        healthValEl.className = 'metric-value ' + (hs >= 80 ? 'text-success' : hs >= 60 ? 'text-warning' : (hs > 0 ? 'text-danger' : 'text-muted'));
    }

    updateVal('stat-active-sessions', s.active_sessions);
    
    updateVal('stat-waiting-sessions', s.waiting_sessions);
    const waitEl = document.getElementById('stat-waiting-sessions');
    if (waitEl) {
        const v = s.waiting_sessions || 0;
        waitEl.className = 'metric-value ' + (v > 5 ? 'text-danger' : v > 0 ? 'text-warning' : 'text-accent');
    }

    updateVal('stat-blocked-sessions', s.blocking_sessions);
    const blockEl = document.getElementById('stat-blocked-sessions');
    if (blockEl) {
        const v = s.blocking_sessions || 0;
        blockEl.className = 'metric-value ' + (v > 0 ? 'text-critical' : 'text-success');
    }

    updateVal('stat-tps', s.tps, v => Number(v || 0).toFixed(0));

    updateVal('stat-conn-pct', s.connections_usage_pct, v => Number(v || 0).toFixed(1) + '%');
    const connPctEl = document.getElementById('stat-conn-pct');
    if (connPctEl) {
        const v = s.connections_usage_pct || 0;
        connPctEl.className = 'metric-value ' + (v > 85 ? 'text-danger' : v > 70 ? 'text-warning' : 'text-accent');
    }

    updateVal('stat-cache-hit', s.cache_hit_ratio_pct, v => Number(v || 0).toFixed(1) + '%');
    const cacheEl = document.getElementById('stat-cache-hit');
    if (cacheEl) {
        const v = s.cache_hit_ratio_pct || 0;
        cacheEl.className = 'metric-value ' + (v >= 98 ? 'text-success' : v >= 95 ? 'text-warning' : 'text-danger');
    }

    updateVal('stat-repl-lag', s.replica_lag_sec, v => Number(v || 0).toFixed(0) + 's');
    const replEl = document.getElementById('stat-repl-lag');
    if (replEl) {
        const v = s.replica_lag_sec || 0;
        replEl.className = 'metric-value ' + (v > 30 ? 'text-danger' : v > 5 ? 'text-warning' : 'text-success');
    }

    updateVal('stat-wal-rate', s.wal_mb_per_min, v => Number(v || 0).toFixed(1));
    
    updateVal('stat-deadlocks', s.deadlocks_per_min, v => Number(v || 0).toFixed(2));
    const dlEl = document.getElementById('stat-deadlocks');
    if (dlEl) {
        const v = s.deadlocks_per_min || 0;
        dlEl.className = 'metric-value ' + (v > 0 ? 'text-danger' : 'text-success');
    }

    updateVal('stat-xid-pct', s.xid_wraparound_pct, v => Number(v || 0).toFixed(1) + '%');
    const xidEl = document.getElementById('stat-xid-pct');
    const xidCard = document.getElementById('kpi-card-xid');
    if (xidEl) {
        const v = s.xid_wraparound_pct || 0;
        xidEl.className = 'metric-value ' + (v > 70 ? 'text-danger' : v > 25 ? 'text-warning' : 'text-success');
        if (xidCard) {
            xidCard.classList.toggle('border-danger', v > 70);
            xidCard.classList.toggle('border-warning', v > 25 && v <= 70);
        }
    }

    updateVal('stat-dead-tuple-pct', s.dead_tuple_pct, v => Number(v || 0).toFixed(1) + '%');
    const dtEl = document.getElementById('stat-dead-tuple-pct');
    if (dtEl) {
        const v = s.dead_tuple_pct || 0;
        dtEl.className = 'metric-value ' + (v > 20 ? 'text-danger' : v > 10 ? 'text-warning' : 'text-success');
    }

    // 2. Fetch Multi-layer Metrics
    const hideBg = document.getElementById('pgLoadHideBgWorkers')?.checked !== false;
    const loadMetrics = ['cpu_load', 'io_wait_load', 'lock_wait_load', 'idle_in_txn_load'];
    
    const metricPromises = [
        ...loadMetrics.map(m => fetchMetric(instName, m, from, to, hideBg)),
        fetchMetric(instName, 'cpu_usage_pct', from, to),
        fetchMetric(instName, 'memory_usage_pct', from, to)
    ];

    const results = await Promise.all(metricPromises);
    const mData = {
        cpu_load: results[0],
        io_wait_load: results[1],
        lock_wait_load: results[2],
        idle_in_txn_load: results[3],
        cpu_usage_pct: results[4],
        memory_usage_pct: results[5]
    };

    // 3. Render 4-Layer AAS Load Chart
    renderAASLoadChart('pgDbLoadChart', mData, from, to);

    // 4. Render Host Resources
    renderHostResourceChart(mData.cpu_usage_pct, mData.memory_usage_pct, from, to);

    // 5. Session State Doughnut
    renderDoughnutWithTotal('pgSessionStatesChart', 
        ['Active', 'Idle', 'Idle in Txn', 'Waiting'],
        [s.active_sessions || 0, s.idle_sessions || 0, s.idle_in_txn_sessions || 0, s.waiting_sessions || 0],
        ['#3b82f6', '#94a3b8', '#f59e0b', '#ef4444']
    );

    // 6. Wait Categories (Horizontal Bar)
    await updateWaitCategories(instName);

    // 7. Operational Trends
    if (h.labels && h.labels.length > 0) {
        const labels = h.labels.map(l => new Date(l).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}));
        renderOverviewLineChart('pgCacheHitTrendChart', labels, h.cache_hit_ratio_pct, window.getCSSVar('--success') || '#10b981', '%', false, 'Cache Hit %');
        renderOverviewLineChart('pgWalRateTrendChart', labels, h.wal_mb_per_min, '#a855f7', 'MB/min', false, 'WAL Rate');
        renderOverviewLineChart('pgConnSaturationChart', labels, h.connections_usage_pct, '#3b82f6', '%', false, 'Usage %');
        
        // Update summary text in headers
        const connSummary = document.getElementById('stat-conn-summary');
        if (connSummary) connSummary.textContent = `${s.connections_used || 0} / ${s.connections_max || 0} (${(s.connections_usage_pct || 0).toFixed(1)}%)`;
    }

    // 8. Checkpoint Health
    await updateCheckpointHealth(instName, from, to);

    // 9. Incident Feed
    await updateIncidentFeed(instName);

    // 10. Replica Detail Grid
    await updateReplicaDetail(instName);
}

async function renderAASLoadChart(id, mData, from, to) {
    const el = document.getElementById(id);
    if (!el) return;

    const labels = (mData.cpu_load || []).map(d => new Date(d.time).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}));
    if (labels.length === 0) {
        // Show a "no data" placeholder so the chart area isn't silently empty.
        const parent = el.parentElement;
        const w = parent ? parent.offsetWidth || 300 : 300;
        const h = parent ? parent.offsetHeight || 230 : 230;
        el.width = w; el.height = h;
        const ctx = el.getContext('2d');
        ctx.clearRect(0, 0, w, h);
        ctx.fillStyle = 'rgba(100,116,139,0.3)';
        ctx.font = '11px sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText('No session-load data collected yet. Waiting for the next Control Center snapshot.', w / 2, h / 2);
        return;
    }

    const datasets = [
        { label: 'CPU Runnable', data: mData.cpu_load.map(d=>d.value), color: '#3b82f6' },
        { label: 'I/O Wait', data: mData.io_wait_load.map(d=>d.value), color: '#f59e0b' },
        { label: 'Lock Wait', data: mData.lock_wait_load.map(d=>d.value), color: '#ef4444' },
        { label: 'Idle in Txn', data: mData.idle_in_txn_load.map(d=>d.value), color: '#94a3b8' }
    ];

    renderStackedAreaChart(id, labels, datasets, 'Avg Active Sessions');
}

async function updateWaitCategories(instName) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/waits/history?instance=${encodeURIComponent(instName)}&limit=100`);
        if (resp.ok) {
            const payload = await resp.json();
            const rows = (payload && payload.rows) ? payload.rows : [];
            if (rows.length === 0) {
                renderBarChart('pgWaitCategoryChart', ['None'], [0], '#94a3b8');
                return;
            }

            // Group by type (latest snapshot logic)
            const latestTs = rows[0].capture_timestamp;
            const latestRows = rows.filter(r => r.capture_timestamp === latestTs);
            
            const categories = {};
            latestRows.forEach(r => {
                const type = r.wait_event_type || 'Unknown';
                if (type === 'Activity' || type === 'Extension' || type === 'Client') return; // Filter background noise
                categories[type] = (categories[type] || 0) + r.sessions_count;
            });

            const labels = Object.keys(categories).sort((a,b) => categories[b] - categories[a]).slice(0, 7);
            const data = labels.map(l => categories[l]);

            if (labels.length === 0) {
                renderBarChart('pgWaitCategoryChart', ['Idle/Activity'], [1], '#94a3b8');
            } else {
                renderBarChart('pgWaitCategoryChart', labels, data, '#3b82f6');
            }
        }
    } catch (e) { console.error("Wait category update failed:", e); }
}

async function updateCheckpointHealth(instName, from, to) {
    try {
        let url = `/api/postgres/checkpoint-health/history?instance=${encodeURIComponent(instName)}`;
        const resp = await window.apiClient.authenticatedFetch(url);
        if (resp.ok) {
            const data = await resp.json() || [];
            if (!data || data.length === 0) {
                // Show a soft placeholder so the chart is not silently blank.
                const canvas = document.getElementById('pgCheckpointHealthChart');
                if (canvas) {
                    const parent = canvas.parentElement;
                    const w = parent ? parent.offsetWidth || 300 : 300;
                    const h = parent ? parent.offsetHeight || 160 : 160;
                    canvas.width = w; canvas.height = h;
                    const ctx = canvas.getContext('2d');
                    ctx.clearRect(0, 0, w, h);
                    ctx.fillStyle = 'rgba(100,116,139,0.3)';
                    ctx.font = '11px sans-serif';
                    ctx.textAlign = 'center';
                    ctx.fillText('No checkpoint data yet — bgwriter stats are collected each cycle.', w / 2, h / 2);
                }
                return;
            }

            const labels = data.map(d => new Date(d.time).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'})).reverse();
            const writeTime = data.map(d => d.checkpoint_write_time).reverse();
            const syncTime = data.map(d => d.checkpoint_sync_time).reverse();
            const latestRatio = data[0].checkpoint_req_ratio || 0;

            const ratioEl = document.getElementById('stat-checkpoint-ratio');
            if (ratioEl) {
                ratioEl.textContent = `Req Ratio: ${latestRatio.toFixed(2)}`;
                ratioEl.className = latestRatio > 1.0 ? 'text-danger' : latestRatio > 0.5 ? 'text-warning' : '';
            }

            renderMultiLineChart('pgCheckpointHealthChart', labels, [
                { label: 'Write Time', data: writeTime, color: '#3b82f6' },
                { label: 'Sync Time', data: syncTime, color: '#10b981' }
            ], 'ms');
        }
    } catch (e) { console.error("Checkpoint health update failed:", e); }
}

async function updateIncidentFeed(instName) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/incident-feed?instance=${encodeURIComponent(instName)}&limit=30`);
        if (resp.ok) {
            const payload = await resp.json();
            const incidents = (payload && payload.incidents) ? payload.incidents : [];
            const tbody = document.getElementById('pgIncidentsFeedTbody');
            if (!tbody) return;

            if (incidents.length === 0) {
                tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No recent incidents detected.</td></tr>';
                return;
            }

            tbody.innerHTML = incidents.map(inc => {
                const sevClass = inc.severity === 'critical' ? 'badge-danger' : inc.severity === 'warning' ? 'badge-warning' : 'badge-outline';
                const typeLabel = inc.incident_type.charAt(0).toUpperCase() + inc.incident_type.slice(1).replace('_', ' ');
                
                let snippet = inc.query_snippet || '--';
                if (snippet !== '--') {
                    try {
                        snippet = decodeURIComponent(snippet).replace(/\+/g, ' ');
                    } catch (e) { /* ignore if not encoded */ }
                }

                return `
                    <tr>
                        <td><span class="badge ${sevClass}">${typeLabel}</span></td>
                        <td>${new Date(inc.ts).toLocaleTimeString()}</td>
                        <td>${inc.duration_ms > 0 ? (inc.duration_ms / 1000).toFixed(1) + 's' : '--'}</td>
                        <td>${inc.usename || '--'}</td>
                        <td>${inc.datname || '--'}</td>
                        <td class="text-truncate" style="max-width:300px; font-family:var(--font-mono); font-size:0.75rem;" title="${window.escapeHtml(snippet)}">${window.escapeHtml(snippet)}</td>
                    </tr>
                `;
            }).join('');
        }
    } catch (e) { console.error("Incident feed update failed:", e); }
}

async function updateReplicaDetail(instName) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/replication?instance=${encodeURIComponent(instName)}&limit=1`);
        const container = document.getElementById('pg-replication-grid-container');
        if (resp.ok) {
            const payload = await resp.json();
            const tbody = document.getElementById('pgReplicaDetailTbody');
            if (!tbody) return;

            // payload is { instance: "...", series: { "replica_name": { ... } } }
            const series = (payload && payload.series) ? payload.series : {};
            const replicas = Object.values(series);
            if (replicas.length === 0) {
                if (container) container.style.display = 'none';
                tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">No streaming replicas configured.</td></tr>';
                return;
            }

            if (container) container.style.display = 'block';
            tbody.innerHTML = replicas.map(r => {
                const latestIdx = r.lag_mb && r.lag_mb.length > 0 ? r.lag_mb.length - 1 : -1;
                const lagMB = latestIdx >= 0 ? r.lag_mb[latestIdx] : 0;
                const writeSec = (r.write_lag_sec && latestIdx >= 0) ? r.write_lag_sec[latestIdx] : null;
                const flushSec = (r.flush_lag_sec && latestIdx >= 0) ? r.flush_lag_sec[latestIdx] : null;
                const replaySec = (r.replay_lag_sec && latestIdx >= 0) ? r.replay_lag_sec[latestIdx] : null;
                const lagClass = lagMB > 100 ? 'text-danger' : lagMB > 10 ? 'text-warning' : 'text-success';
                const syncBadge = r.sync_state === 'sync' ? 'badge-success' : r.sync_state === 'quorum' ? 'badge-warning' : 'badge-outline';
                const syncLabel = r.sync_state ? r.sync_state.charAt(0).toUpperCase() + r.sync_state.slice(1) : 'Async';
                const fmtLag = (v) => v !== null && v >= 0 ? (v < 1 ? (v * 1000).toFixed(0) + 'ms' : v.toFixed(2) + 's') : '--';
                const stateIcon = (r.state === 'streaming')
                    ? '<span class="text-success"><i class="fa-solid fa-circle-check"></i> Streaming</span>'
                    : `<span class="text-warning">${r.state || 'Unknown'}</span>`;
                return `
                    <tr>
                        <td><i class="fa-solid fa-server text-muted small"></i> ${r.replica_name || 'Unknown'}</td>
                        <td class="${lagClass}">${fmtLag(writeSec)}</td>
                        <td class="${lagClass}">${fmtLag(flushSec)}</td>
                        <td class="${lagClass}">${fmtLag(replaySec)}</td>
                        <td><span class="badge ${syncBadge}">${syncLabel}</span></td>
                        <td class="${lagClass}">${lagMB.toFixed(1)} MB</td>
                        <td>${stateIcon}</td>
                    </tr>
                `;
            }).join('');
        }
    } catch (e) { console.error("Replica detail update failed:", e); }
}

// Helper: Doughnut with total in center
function renderDoughnutWithTotal(id, labels, data, colors) {
    const el = document.getElementById(id);
    if (!el) return;
    if (window.currentCharts[id]) window.currentCharts[id].destroy();

    const total = data.reduce((a, b) => a + b, 0);

    window.currentCharts[id] = new Chart(el.getContext('2d'), {
        type: 'doughnut',
        data: {
            labels: labels,
            datasets: [{ data: data, backgroundColor: colors, borderWidth: 0 }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            cutout: '75%',
            plugins: {
                legend: { position: 'right', labels: { boxWidth: 10, font: { size: 10 }, color: '#94a3b8' } },
                centerText: { display: true, text: `Total: ${total}` }
            }
        },
        plugins: [{
            id: 'centerText',
            afterDraw: (chart) => {
                const { width, height, ctx } = chart;
                ctx.restore();
                const fontSize = (height / 160).toFixed(2);
                ctx.font = fontSize + "em sans-serif";
                ctx.textBaseline = "middle";
                ctx.fillStyle = "#94a3b8";
                const text = chart.options.plugins.centerText.text;
                const textX = Math.round((width - ctx.measureText(text).width) / 2);
                const textY = height / 2;
                ctx.fillText(text, textX, textY);
                ctx.save();
            }
        }]
    });
}

function renderMultiLineChart(id, labels, datasets, unit) {
    const el = document.getElementById(id);
    if (!el) return;
    if (window.currentCharts[id]) window.currentCharts[id].destroy();

    window.currentCharts[id] = new Chart(el.getContext('2d'), {
        type: 'line',
        data: {
            labels: labels,
            datasets: datasets.map(ds => ({
                label: ds.label,
                data: ds.data,
                borderColor: ds.color,
                borderWidth: 2,
                pointRadius: 0,
                tension: 0.3,
                fill: false
            }))
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: true, position: 'top', align: 'end', labels: { boxWidth: 10, font: { size: 9 }, color: '#94a3b8' } } },
            scales: {
                x: { ticks: { color: '#64748b', maxTicksLimit: 6 }, grid: { display: false } },
                y: { ticks: { color: '#64748b', font: { size: 9 } }, grid: { color: 'rgba(148,163,184,0.1)' } }
            }
        }
    });
}

// Re-use standard helper functions from previous overview.js or globally available ones
// (fetchMetric, renderStackedAreaChart, renderOverviewLineChart, renderBarChart, renderDoughnutChart, renderHostResourceChart)
// were mostly kept or slightly enhanced.
async function fetchMetric(instance, metric, from, to, filterBg = false) {
    try {
        let url = `/api/postgres/metrics/timeseries?instance=${encodeURIComponent(instance)}&metric=${encodeURIComponent(metric)}`;
        if (from && to) {
            url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;
        } else {
            url += `&limit=60`;
        }
        if (filterBg) {
            url += `&filter_bg_workers=true`;
        }
        const resp = await window.apiClient.authenticatedFetch(url);
        if (resp.ok) {
            const payload = await resp.json();
            return (payload && payload.data) ? payload.data : [];
        }
    } catch (e) { console.error(`Failed to fetch metric ${metric}:`, e); }
    return [];
}

function renderStackedAreaChart(id, labels, datasets, yLabel) {
    const el = document.getElementById(id);
    if (!el) return;
    if (window.currentCharts[id]) window.currentCharts[id].destroy();

    window.currentCharts[id] = new Chart(el.getContext('2d'), {
        type: 'line',
        data: {
            labels: labels,
            datasets: datasets.map(ds => ({
                label: ds.label,
                data: ds.data,
                borderColor: ds.color,
                backgroundColor: ds.color + '44',
                fill: true,
                tension: 0.4,
                pointRadius: 0,
                borderWidth: 1
            }))
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            plugins: {
                legend: { position: 'top', align: 'end', labels: { boxWidth: 10, font: { size: 10 }, color: '#94a3b8' } },
                tooltip: { enabled: true }
            },
            scales: {
                y: { stacked: true, beginAtZero: true, ticks: { font: { size: 10 }, color: '#64748b' }, grid: { color: 'rgba(148,163,184,0.1)' } },
                x: { ticks: { font: { size: 10 }, color: '#64748b', maxTicksLimit: 6 }, grid: { display: false } }
            }
        }
    });
}

function renderOverviewLineChart(id, labels, data, color, unit, showLegend, datasetLabel) {
    const el = document.getElementById(id);
    if (!el) return;
    if (window.currentCharts[id]) window.currentCharts[id].destroy();

    window.currentCharts[id] = new Chart(el.getContext('2d'), {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: datasetLabel || 'Value',
                data: data,
                borderColor: color,
                backgroundColor: color + '22',
                fill: true,
                tension: 0.3,
                pointRadius: 0,
                borderWidth: 2
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: showLegend, labels: { color: '#94a3b8' } } },
            scales: {
                x: { ticks: { maxTicksLimit: 6, color: '#94a3b8', font: { size: 9 } }, grid: { display: false } },
                y: { beginAtZero: true, ticks: { maxTicksLimit: 5, color: '#94a3b8', font: { size: 9 } }, grid: { color: 'rgba(148,163,184,0.1)' } }
            }
        }
    });
}

function renderBarChart(id, labels, data, color) {
    const el = document.getElementById(id);
    if (!el) return;
    if (window.currentCharts[id]) window.currentCharts[id].destroy();

    window.currentCharts[id] = new Chart(el.getContext('2d'), {
        type: 'bar',
        data: {
            labels: labels,
            datasets: [{
                data: data,
                backgroundColor: color,
                borderWidth: 0,
                borderRadius: 2
            }]
        },
        options: {
            indexAxis: 'y',
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                x: { beginAtZero: true, ticks: { color: '#64748b', maxTicksLimit: 5 }, grid: { color: 'rgba(148,163,184,0.1)' } },
                y: { ticks: { color: '#94a3b8', font: { size: 10 } }, grid: { display: false } }
            }
        }
    });
}

function renderHostResourceChart(cpuRaw, memRaw, from, to) {
    const canvasId = 'pgHostResourceChart';
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;

    if (window.currentCharts && window.currentCharts[canvasId]) {
        window.currentCharts[canvasId].destroy();
    }

    // When the OS Collector is not deployed, both arrays will be empty.
    // Replace the chart area with an informative notice rather than
    // leaving a blank, unlabeled chart container.
    if ((!cpuRaw || cpuRaw.length === 0) && (!memRaw || memRaw.length === 0)) {
        const container = canvas.closest('.card');
        if (container) {
            container.style.display = 'flex';
            container.style.alignItems = 'center';
            container.style.justifyContent = 'center';
            const noticeEl = container.querySelector('.host-res-notice') || document.createElement('div');
            noticeEl.className = 'host-res-notice';
            noticeEl.style.cssText = 'text-align:center; padding:0.75rem 1.25rem; font-size:0.72rem; color:var(--text-muted); line-height:1.6;';
            noticeEl.innerHTML = '<i class="fa-solid fa-microchip" style="font-size:1.4rem; color:#475569; display:block; margin-bottom:0.4rem;"></i>'
                + '<strong style="color:var(--text-secondary);">Host Resources (CPU/Mem)</strong><br>'
                + 'OS-level metrics require the <strong>OS Collector agent</strong>.<br>'
                + 'Deploy the agent on this host to see CPU%, memory%, and process metrics.';
            canvas.style.display = 'none';
            const header = container.querySelector('.card-header');
            if (header) header.style.display = 'none';
            if (!container.querySelector('.host-res-notice')) container.appendChild(noticeEl);
        }
        return;
    }

    const combined = [];
    const cpuMap = new Map();
    const memMap = new Map();
    cpuRaw.forEach(d => cpuMap.set(d.time, d.value));
    memRaw.forEach(d => memMap.set(d.time, d.value));
    const allTimes = new Set([...cpuRaw.map(d => d.time), ...memRaw.map(d => d.time)]);
    allTimes.forEach(t => {
        combined.push({ time: t, cpu: cpuMap.get(t) || 0, mem: memMap.get(t) || 0 });
    });
    combined.sort((a,b) => new Date(a.time) - new Date(b.time));

    const labels = combined.map(d => new Date(d.time).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}));
    const cpuData = combined.map(d => d.cpu);
    const memData = combined.map(d => d.mem);

    const curCpu = cpuData[cpuData.length-1] || 0;
    const curMem = memData[memData.length-1] || 0;
    const cpuEl = document.getElementById('current-cpu-val');
    const memEl = document.getElementById('current-mem-val');
    if (cpuEl) {
        cpuEl.textContent = `CPU: ${curCpu.toFixed(1)}%`;
        cpuEl.className = curCpu > 90 ? 'text-critical' : curCpu > 70 ? 'text-warning' : 'text-success';
    }
    if (memEl) {
        memEl.textContent = `Mem: ${curMem.toFixed(1)}%`;
        memEl.className = curMem > 90 ? 'text-critical' : curMem > 70 ? 'text-warning' : 'text-accent';
    }

    window.currentCharts[canvasId] = new Chart(canvas.getContext('2d'), {
        type: 'line',
        data: {
            labels,
            datasets: [
                { label: 'CPU %', data: cpuData, borderColor: '#ef4444', tension: 0.4, pointRadius: 0 },
                { label: 'Mem %', data: memData, borderColor: '#3b82f6', tension: 0.4, pointRadius: 0 }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { position: 'top', align: 'end', labels: { boxWidth: 10, font: { size: 10 }, color: '#94a3b8' } } },
            scales: {
                x: { ticks: { color: '#64748b', maxTicksLimit: 8 }, grid: { display: false } },
                y: { min: 0, max: 100, ticks: { color: '#64748b' }, grid: { color: 'rgba(148,163,184,0.1)' } }
            }
        }
    });
}
