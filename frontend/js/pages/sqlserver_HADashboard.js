/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Modernized High Availability dashboard for Always On AG and Replication.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.HADashboardView = function() {
    const instance = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!instance) { alert('Select an instance first.'); return; }
    
    // Helper for local ISO string (YYYY-MM-DDTHH:mm)
    const toLocalISO = (date) => {
        const offset = date.getTimezoneOffset() * 60000;
        const local = new Date(date.getTime() - offset);
        return local.toISOString().slice(0, 16);
    };

    const now = new Date();
    const sixHoursAgo = new Date(now.getTime() - (6 * 60 * 60 * 1000));
    const defaultFrom = toLocalISO(sixHoursAgo);
    const defaultTo = toLocalISO(now);

    if (!window.appState.haFrom) window.appState.haFrom = defaultFrom;
    if (!window.appState.haTo) window.appState.haTo = defaultTo;

    // Refresh if more than 10 minutes old
    const lastTo = new Date(window.appState.haTo);
    if (now.getTime() - lastTo.getTime() > 10 * 60 * 1000) {
        window.appState.haFrom = defaultFrom;
        window.appState.haTo = defaultTo;
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1><i class="fa-solid fa-shield-halved text-accent"></i> HA & Replication</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(instance.name)} | AlwaysOn, Log Shipping & Pub/Sub</p>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="gap: 0.75rem; align-items: center;">
                    <div class="date-picker-group glass-panel" style="display:flex; align-items:center; gap:0.5rem; padding:0.25rem 0.75rem; border-radius:8px;">
                        <span class="text-muted" style="font-size:0.75rem;">Range:</span>
                        <input type="datetime-local" id="haFrom" class="custom-date-input" value="${window.appState.haFrom}">
                        <span class="text-muted">to</span>
                        <input type="datetime-local" id="haTo" class="custom-date-input" value="${window.appState.haTo}">
                        <button id="haApplyRange" class="btn btn-xs btn-accent">Apply</button>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="HADashboardView"><i class="fa-solid fa-rotate"></i></button>
                </div>
            </div>

            <div class="tabs-container mt-3">
                <button class="tab-btn active" data-tab="ag">Availability Groups</button>
                <button class="tab-btn" data-tab="repl">Replication (Pub/Sub)</button>
                <button class="tab-btn" data-tab="ls">Log Shipping</button>
            </div>

            <div id="haContent" class="mt-3">
                <div id="haTab-ag" class="tab-panel">
                    <div class="grid-2 mt-2">
                        <div class="glass-panel" style="padding:1rem;">
                            <div class="card-header"><h3 style="font-size:0.85rem;margin:0;">AG Queue Trend (Avg KB)</h3></div>
                            <div class="chart-container" style="height:200px;"><canvas id="agQueueChart"></canvas></div>
                        </div>
                        <div class="glass-panel" style="padding:1rem;">
                            <div class="card-header"><h3 style="font-size:0.85rem;margin:0;">Secondary Lag Trend (Max Sec)</h3></div>
                            <div class="chart-container" style="height:200px;"><canvas id="agLagChart"></canvas></div>
                        </div>
                    </div>
                    <div class="glass-panel mt-3" style="padding:1rem;">
                        <div class="card-header"><h3 style="font-size:0.85rem;margin:0;">Current AG Replica Status</h3></div>
                        <div id="agStatusGrid" class="mt-2"></div>
                    </div>
                </div>

                <div id="haTab-repl" class="tab-panel" style="display:none;">
                    <div class="glass-panel" style="padding:1rem;">
                        <div class="card-header"><h3 style="font-size:0.85rem;margin:0;">Replication Publications & Subscriptions</h3></div>
                        <div id="replStatusGrid" class="mt-2">
                             <div class="text-center p-4 text-muted">Loading replication data...</div>
                        </div>
                    </div>
                </div>

                <div id="haTab-ls" class="tab-panel" style="display:none;">
                    <div class="glass-panel" style="padding:1rem;">
                        <div class="card-header"><h3 style="font-size:0.85rem;margin:0;">Log Shipping Pairs</h3></div>
                        <div id="lsStatusGrid" class="mt-2"></div>
                    </div>
                </div>
            </div>
        </div>
    `;

    document.getElementById('haApplyRange').addEventListener('click', () => {
        window.appState.haFrom = document.getElementById('haFrom').value;
        window.appState.haTo = document.getElementById('haTo').value;
        loadHAData(instance.name);
    });

    // Tab switching
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            document.querySelectorAll('.tab-panel').forEach(p => p.style.display = 'none');
            btn.classList.add('active');
            document.getElementById(`haTab-${btn.dataset.tab}`).style.display = '';
        });
    });

    loadHAData(instance.name);
};

async function loadHAData(instanceName) {
    const from = window.appState.haFrom || '';
    const to = window.appState.haTo || '';
    
    // AG Health & History
    fetchAGData(instanceName, from, to);
    // Replication
    fetchReplData(instanceName);
    // Log Shipping
    fetchLSData(instanceName);
}

async function fetchAGData(instanceName, from, to) {
    const grid = document.getElementById('agStatusGrid');
    try {
        const [respHealth, respHist] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/sqlserver/ag-health?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/ag-health/history?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`)
        ]);

        const dataHealth = await respHealth.json();
        const dataHist = await respHist.json();

        renderAGGrid(dataHealth.ag_stats || []);
        renderAGCharts(dataHist.history || []);
    } catch(err) {
        grid.innerHTML = `<div class="alert alert-danger">AG Load Failed: ${err.message}</div>`;
    }
}

function renderAGGrid(stats) {
    const grid = document.getElementById('agStatusGrid');
    if (stats.length === 0) {
        grid.innerHTML = `<div class="text-center p-4 text-muted">No Availability Groups detected or enabled.</div>`;
        return;
    }

    grid.innerHTML = `
        <table class="data-table">
            <thead>
                <tr>
                    <th>AG Name</th><th>Database</th><th>Role</th><th>Sync State</th>
                    <th>Log Send (KB)</th><th>Redo Queue (KB)</th><th>Lag (s)</th>
                </tr>
            </thead>
            <tbody>
                ${stats.map(s => `
                    <tr>
                        <td><strong>${window.escapeHtml(s.ag_name)}</strong></td>
                        <td>${window.escapeHtml(s.database_name)}</td>
                        <td><span class="badge ${s.is_primary_replica ? 'badge-primary' : 'badge-info'}">${s.replica_role || (s.is_primary_replica ? 'PRIMARY' : 'SECONDARY')}</span></td>
                        <td><span class="text-${s.synchronization_state === 'SYNCHRONIZED' ? 'success' : 'warning'}">${window.escapeHtml(s.synchronization_state)}</span></td>
                        <td>${Number(s.avg_log_send_queue_kb || s.log_send_queue_kb || 0).toLocaleString()}</td>
                        <td>${Number(s.avg_redo_queue_kb || s.redo_queue_kb || 0).toLocaleString()}</td>
                        <td class="${(s.secondary_lag_secs || 0) > 30 ? 'text-danger' : ''}">${s.secondary_lag_secs || 0}</td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;
}

function renderAGCharts(history) {
    const canvasQ = document.getElementById('agQueueChart');
    const canvasL = document.getElementById('agLagChart');
    const ctxQ = canvasQ.getContext('2d');
    const ctxL = canvasL.getContext('2d');

    if (window.currentCharts?.agQueue) window.currentCharts.agQueue.destroy();
    if (window.currentCharts?.agLag) window.currentCharts.agLag.destroy();
    window.currentCharts = window.currentCharts || {};

    if (!history || history.length === 0) {
        // Clear canvases and show message
        [canvasQ, canvasL].forEach(c => {
            const ctx = c.getContext('2d');
            ctx.clearRect(0, 0, c.width, c.height);
            ctx.font = '12px Inter, sans-serif';
            ctx.fillStyle = '#94a3b8';
            ctx.textAlign = 'center';
            ctx.fillText('No Availability Groups detected or enabled.', c.width / 2, c.height / 2);
        });
        return;
    }

    const labels = history.map(h => new Date(h.timestamp).toLocaleTimeString());
    const logData = history.map(h => h.avg_log_send_queue_kb);
    const redoData = history.map(h => h.avg_redo_queue_kb);
    const lagData = history.map(h => h.max_lag_sec);

    window.currentCharts.agQueue = new Chart(ctxQ, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                { label: 'Log Send', data: logData, borderColor: '#38bdf8', tension: 0.3, fill: false },
                { label: 'Redo Queue', data: redoData, borderColor: '#fbbf24', tension: 0.3, fill: false }
            ]
        },
        options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } } }
    });

    window.currentCharts.agLag = new Chart(ctxL, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{ label: 'Secondary Lag (s)', data: lagData, borderColor: '#f87171', backgroundColor: 'rgba(248,113,113,0.1)', fill: true, tension: 0.3 }]
        },
        options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } } }
    });
}

async function fetchReplData(instanceName) {
    const grid = document.getElementById('replStatusGrid');
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/replication-status?instance=${encodeURIComponent(instanceName)}`);
        const data = await resp.json();
        const repl = data.replication || [];
        
        if (repl.length === 0) {
            grid.innerHTML = `<div class="text-center p-4 text-muted">No Publication or Subscription data found.</div>`;
            return;
        }

        grid.innerHTML = `
            <table class="data-table">
                <thead><tr><th>Type</th><th>Name</th><th>Database</th><th>Details</th><th>Articles</th><th>Status</th></tr></thead>
                <tbody>
                    ${repl.map(r => `
                        <tr>
                            <td><span class="badge ${r.type === 'Publication' ? 'badge-primary' : 'badge-info'}">${r.type}</span></td>
                            <td><strong>${window.escapeHtml(r.name)}</strong></td>
                            <td>${window.escapeHtml(r.database)}</td>
                            <td class="text-muted small">${window.escapeHtml(r.details)}</td>
                            <td>${r.article_count}</td>
                            <td><span class="badge ${r.status === 'Active' || r.status === 'Subscribed' ? 'badge-success' : 'badge-outline'}">${r.status}</span></td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    } catch(e) { grid.innerHTML = `<div class="alert alert-danger">Repl Failed: ${e.message}</div>`; }
}

async function fetchLSData(instanceName) {
    const grid = document.getElementById('lsStatusGrid');
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/log-shipping?instance=${encodeURIComponent(instanceName)}`);
        const data = await resp.json();
        const rows = data.log_shipping || [];

        if (rows.length === 0) {
            grid.innerHTML = `<div class="text-center p-4 text-muted">No Log Shipping monitoring found.</div>`;
            return;
        }

        grid.innerHTML = `
            <table class="data-table">
                <thead><tr><th>Primary DB</th><th>Secondary Server</th><th>Last Backup</th><th>Last Restored</th><th>Delay</th><th>Status</th></tr></thead>
                <tbody>
                    ${rows.map(r => `
                        <tr>
                            <td><strong>${window.escapeHtml(r.primary_database)}</strong></td>
                            <td>${window.escapeHtml(r.secondary_server)}</td>
                            <td class="small text-muted">${r.last_backup_date ? new Date(r.last_backup_date).toLocaleString() : 'N/A'}</td>
                            <td class="small text-muted">${r.last_restore_date ? new Date(r.last_restore_date).toLocaleString() : 'N/A'}</td>
                            <td>${r.restore_delay_minutes}m</td>
                            <td><span class="badge ${r.status === 1 ? 'badge-success' : 'badge-danger'}">${r.status === 1 ? 'OK' : 'ERROR'}</span></td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    } catch(e) { grid.innerHTML = `<div class="alert alert-danger">LS Failed: ${e.message}</div>`; }
}
