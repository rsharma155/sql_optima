/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Replication monitoring for PostgreSQL.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgReplicationView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'postgres'};
    const dbName = window.appState.currentDatabase || 'all';

    window.appState.activeViewId = 'pg-replication';

    // 1. Initial Shell
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/replication.html', { inst, dbName });
    
    window.initPageTimePicker();

    // 2. Initial Fetch
    await initPgReplication(inst.name);

    // 3. Set Refresh Interval
    if (window.pgReplicationInterval) clearInterval(window.pgReplicationInterval);
    window.pgReplicationInterval = setInterval(() => {
        if (window.appState.activeViewId === 'pg-replication') {
            initPgReplication(inst.name);
        } else {
            clearInterval(window.pgReplicationInterval);
        }
    }, 60000); // 60s refresh
};

async function updatePgReplicationHeader(instName) {
    try {
        const snapshotResp = await window.apiClient.authenticatedFetch(`/api/postgres/server-info?instance=${encodeURIComponent(instName)}`);
        if (snapshotResp.ok) {
            const s = await snapshotResp.json();
            if (document.getElementById('pg-uptime')) document.getElementById('pg-uptime').textContent = 'Uptime: ' + (s.uptime || 'N/A');
            if (document.getElementById('pg-version')) document.getElementById('pg-version').textContent = (s.version || '').split(',')[0];
            if (document.getElementById('pgLastRefreshTime')) document.getElementById('pgLastRefreshTime').textContent = new Date().toLocaleTimeString();
            
            const hs = s.health_score || 0;
            const healthColor = hs > 80 ? 'success' : hs > 60 ? 'warning' : 'danger';
            const hBadge = document.getElementById('pgHealthScoreBadge');
            if (hBadge) {
                hBadge.textContent = hs;
                hBadge.className = `badge badge-${healthColor}`;
            }
        }
    } catch (e) { console.error("PG Replication header fetch failed:", e); }
}

async function initPgReplication(instName) {
    updatePgReplicationHeader(instName);

    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 3600000);
    const buildUrl = (base) => {
        return `${base}?instance=${encodeURIComponent(instName)}&from=${oneHourAgo.toISOString()}&to=${now.toISOString()}&limit=500`;
    };

    let replData = {
        is_primary: false,
        cluster_state: 'unknown',
        max_lag_mb: 0,
        wal_gen_rate_mbps: 0,
        bg_writer_eff_pct: 0,
        standbys: []
    };
    let ccHistory = null;
    let replLagSeries = null;
    
    try {
        const [replResp, histResp, lagResp] = await Promise.all([
            window.apiClient.authenticatedFetch(buildUrl('/api/postgres/replication')),
            window.apiClient.authenticatedFetch(buildUrl('/api/postgres/control-center/history')),
            window.apiClient.authenticatedFetch(buildUrl('/api/postgres/replication-lag/history')),
        ]);
        
        if (replResp.ok) {
            const payload = await replResp.json();
            replData = payload?.stats || replData;
        }
        if (histResp.ok) {
            const payload = await histResp.json();
            ccHistory = payload && payload.history ? payload.history : null;
        }
        if (lagResp.ok) {
            const payload = await lagResp.json();
            replLagSeries = payload && payload.series ? payload.series : null;
        }
    } catch (e) {
        console.error("PG replication fetch failed:", e);
    }

    // Update KPIs
    const standbys = replData.standbys || [];
    const isPrimary = replData.is_primary !== false;
    
    const topoEl = document.getElementById('stat-repl-topology');
    if (topoEl) topoEl.textContent = isPrimary ? `Primary (${standbys.length} Standbys)` : 'Standby / Replica';
    
    const lagEl = document.getElementById('stat-repl-max-lag');
    if (lagEl) lagEl.textContent = (replData.max_lag_mb || 0).toFixed(1) + ' MB';
    
    const walEl = document.getElementById('stat-repl-wal-gen');
    if (walEl) walEl.textContent = (replData.wal_gen_rate_mbps || 0).toFixed(1) + ' MB/s';
    
    const bgEl = document.getElementById('stat-repl-bgwriter');
    if (bgEl) bgEl.textContent = (replData.bg_writer_eff_pct || 0).toFixed(1) + '%';

    // Populate Standby Table
    const tbody = document.getElementById('pgReplicationTbody');
    if (tbody) {
        if (standbys.length === 0) {
            tbody.innerHTML = `<tr><td colspan="7" class="text-center text-muted">${isPrimary ? 'No standbys connected' : 'Instance is not a primary'}</td></tr>`;
        } else {
            tbody.innerHTML = standbys.map(s => `
                <tr>
                    <td><strong>${window.escapeHtml(s.application_name || 'unknown')}</strong></td>
                    <td>${window.escapeHtml(s.client_addr || '-')}</td>
                    <td><span class="badge badge-success">${window.escapeHtml(s.state || '-')}</span></td>
                    <td><span class="text-accent">${window.escapeHtml(s.sync_state || '-')}</span></td>
                    <td>${window.escapeHtml(s.sent_lsn || '-')}</td>
                    <td class="text-right ${Number(s.replay_lag_mb) > 10 ? 'text-danger font-bold' : ''}">${(s.replay_lag_mb || 0).toFixed(1)} MB</td>
                    <td>${window.escapeHtml(s.replay_lag_time || '-')}</td>
                </tr>
            `).join('');
        }
    }

    // Render Charts
    renderReplicationCharts(replLagSeries, ccHistory);
}

function renderReplicationCharts(lagSeries, ccHistory) {
    window.currentCharts = window.currentCharts || {};
    
    // Lag Chart
    const lagCtx = document.getElementById('pgReplCtx');
    if (lagCtx && lagSeries && Array.isArray(lagSeries)) {
        if (window.currentCharts['pgReplCtx']) window.currentCharts['pgReplCtx'].destroy();
        const labels = lagSeries.map(p => new Date(p.time).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}));
        const data = lagSeries.map(p => p.value);
        window.currentCharts['pgReplCtx'] = new Chart(lagCtx, {
            type: 'line',
            data: {
                labels,
                datasets: [{
                    label: 'Replication Lag (MB)',
                    data,
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59, 130, 246, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: {
                    x: { ticks: { maxTicksLimit: 8, color: '#94a3b8' } },
                    y: { beginAtZero: true, ticks: { color: '#94a3b8' } }
                }
            }
        });
    }

    // Checkpointer Chart
    const checkCtx = document.getElementById('pgCheckChart');
    if (checkCtx && ccHistory) {
        if (window.currentCharts['pgCheckChart']) window.currentCharts['pgCheckChart'].destroy();
        const labels = ccHistory.map(p => new Date(p.time).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}));
        const timed = ccHistory.map(p => p.checkpoints_timed || 0);
        const req = ccHistory.map(p => p.checkpoints_req || 0);
        window.currentCharts['pgCheckChart'] = new Chart(checkCtx, {
            type: 'line',
            data: {
                labels,
                datasets: [
                    { label: 'Timed', data: timed, borderColor: '#10b981', tension: 0.3, pointRadius: 0 },
                    { label: 'Requested', data: req, borderColor: '#ef4444', tension: 0.3, pointRadius: 0 }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { display: true, position: 'top', align: 'end', labels: { boxWidth: 12, color: '#94a3b8' } } },
                scales: {
                    x: { ticks: { maxTicksLimit: 8, color: '#94a3b8' } },
                    y: { beginAtZero: true, ticks: { color: '#94a3b8' } }
                }
            }
        });
    }
}
