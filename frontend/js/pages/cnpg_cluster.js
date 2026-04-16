/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Cloud Native PostgreSQL cluster monitoring.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

/**
 * CNPG Cluster Topology View
 * 
 * Polls /api/postgres/replication every 15 seconds and displays:
 * - Primary: Data grid of connected replicas with pod name, IP, state, sync mode, lag
 * - Standby: Large KPI gauge showing local replay lag
 * 
 * Formatting rules:
 * - sync_state == 'sync' or 'quorum' -> green text
 * - state != 'streaming' -> yellow row
 * - replay_lag_mb > 50 -> bold red lag number
 */
window.CNPGClusterTopologyView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'postgres') {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">CNPG topology is for PostgreSQL instances only.</h3></div>`;
        return;
    }
    const database = window.appState.currentDatabase || 'all';

    if (window.cnpgTopologyInterval) {
        clearInterval(window.cnpgTopologyInterval);
        window.cnpgTopologyInterval = null;
    }

    window.appState.currentInstanceName = inst.name;

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme" id="cnpgPage">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line" style="flex:1; min-width:0;">
                    <h1>CNPG Cluster Health</h1>
                    <span class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Database: <span class="text-accent">${window.escapeHtml(database)}</span></span>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:0.75rem; flex-wrap:wrap; justify-content:flex-end;">
                    <div id="cnpgStatusStrip"></div>
                    <span class="badge badge-outline" style="font-size:0.65rem;">Last Poll: <span id="cnpgLastPoll">--:--:--</span></span>
                    <label class="flex-between" style="align-items:center; gap:0.5rem; font-size:0.8rem; cursor:pointer;">
                        <input type="checkbox" id="cnpgAutoRefresh" checked style="width:16px; height:16px;"> Auto-refresh (15s)
                    </label>
                    <button class="btn btn-sm btn-outline" data-action="navigate-back"><i class="fa-solid fa-arrow-left"></i> Back</button>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshCNPGTopology"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            <div id="cnpgContent" style="display:flex; justify-content:center; align-items:center; height:50vh;">
                <div class="spinner"></div><span style="margin-left:1rem;">Polling CNPG cluster topology...</span>
            </div>
        </div>
    `;

    try {
        const strip = document.getElementById('cnpgStatusStrip');
        if (strip && typeof window.renderStatusStrip === 'function') {
            strip.innerHTML = window.renderStatusStrip({
                lastUpdateId: 'cnpgLastUpdate',
                sourceBadgeId: 'cnpgSourceBadge',
                includeHealth: false,
                includeFreshness: false,
                autoRefreshText: ''
            });
        }
        const t = document.getElementById('cnpgLastUpdate');
        if (t) t.textContent = new Date().toLocaleTimeString();
    } catch (e) {
        // non-fatal
    }

    const checkbox = document.getElementById('cnpgAutoRefresh');
    if (checkbox) {
        checkbox.addEventListener('change', function() {
            if (this.checked) {
                if (window.cnpgTopologyInterval) clearInterval(window.cnpgTopologyInterval);
                window.cnpgTopologyInterval = setInterval(window.loadCNPGTopology, 15000);
            } else {
                clearInterval(window.cnpgTopologyInterval);
                window.cnpgTopologyInterval = null;
            }
        });
    }

    await window.loadCNPGTopology();
    window.cnpgTopologyInterval = setInterval(window.loadCNPGTopology, 15000);
};

window.loadCNPGTopology = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'postgres') return;

    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/postgres/replication?instance=${encodeURIComponent(inst.name)}`
        );
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const contentType = response.headers.get('content-type') || '';
        if (!contentType.includes('application/json')) {
            const text = await response.text();
            console.error('Replication API returned non-JSON:', text.substring(0, 200));
            throw new Error('Invalid response from replication API');
        }

        const data = await response.json();
        window.updateCNPGContent(inst, data);

        const pollEl = document.getElementById('cnpgLastPoll');
        if (pollEl) pollEl.textContent = new Date().toLocaleTimeString();
    } catch (error) {
        console.error('[CNPGClusterTopology] Error:', error);
        const contentEl = document.getElementById('cnpgContent');
        if (contentEl) {
            contentEl.innerHTML = `
                <div class="alert alert-danger mt-3">
                    <i class="fa-solid fa-exclamation-triangle"></i> Failed to load CNPG topology: ${window.escapeHtml(error.message)}
                </div>
            `;
        }
    }
};

window.updateCNPGContent = function(inst, data) {
    const contentEl = document.getElementById('cnpgContent');
    if (!contentEl) return;

    const isPrimary = data.is_primary === true;
    const standbys = data.standbys || [];
    const localLag = data.local_lag_mb || 0;
    const maxLag = data.max_lag_mb || 0;
    const bgEff = data.bg_writer_eff_pct || 0;

    const lagColor = (lag) => lag > 50 ? 'var(--danger)' : lag > 10 ? 'var(--warning)' : 'var(--success)';
    const lagWeight = (lag) => lag > 50 ? 'bold' : 'normal';
    const syncColor = (sync) => {
        if (sync === 'sync' || sync === 'quorum') return '#22c55e';
        if (sync === 'potential') return '#f59e0b';
        return 'var(--text-muted)';
    };
    const rowBg = (state) => state !== 'streaming' ? 'rgba(245,158,11,0.06)' : '';

    let html = `
        <!-- Role Badge -->
        <div style="margin-bottom:1.5rem;">
            ${isPrimary ? `
                <span style="display:inline-flex; align-items:center; gap:0.5rem; background:rgba(34,197,94,0.12); border:1px solid #22c55e; border-radius:6px; padding:0.4rem 1rem; font-size:0.85rem; color:#22c55e; font-weight:600;">
                    <i class="fa-solid fa-crown"></i> Role: Primary Node
                </span>
            ` : `
                <span style="display:inline-flex; align-items:center; gap:0.5rem; background:rgba(59,130,246,0.12); border:1px solid #3b82f6; border-radius:6px; padding:0.4rem 1rem; font-size:0.85rem; color:#3b82f6; font-weight:600;">
                    <i class="fa-solid fa-book-open"></i> Role: Standby Node (Read-Only)
                </span>
            `}
        </div>
    `;

    if (isPrimary) {
        // Primary: 3-column layout matching PG Overview pattern
        html += `
            <div class="metrics-row" style="display:grid; grid-template-columns:1fr 1fr 1fr; gap:0.75rem; margin-top:0.75rem;">
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-clone"></i> Replication Status</h4>
                    <div style="display:flex; flex-direction:column; gap:0.5rem;">
                        <div class="metric-card glass-panel status-healthy" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">Max Lag</span><i class="fa-solid fa-clock card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem; color:${lagColor(maxLag)};">${maxLag.toFixed(2)}<span style="font-size:0.5em"> MB</span></div>
                            <div class="metric-trend ${maxLag > 10 ? 'warning' : 'positive'}" style="font-size:0.65rem"><i class="fa-solid fa-${maxLag > 10 ? 'triangle-exclamation' : 'check'}"></i> ${maxLag > 10 ? 'Sync Delay' : 'In Sync'}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">WAL Gen Rate</span><i class="fa-solid fa-file-pen card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem;">${data.wal_gen_rate_mbps ? data.wal_gen_rate_mbps.toFixed(1) : '0.0'}<span style="font-size:0.5em"> MB/s</span></div>
                            <div class="metric-trend positive" style="font-size:0.65rem"><i class="fa-solid fa-arrow-right"></i> Transmission</div>
                        </div>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-cog"></i> Background Engine</h4>
                    <div style="display:flex; flex-direction:column; gap:0.5rem;">
                        <div class="metric-card glass-panel status-healthy" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">BGWriter Eff</span><i class="fa-solid fa-brush card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem; color:var(--success);">${bgEff.toFixed(0)}<span>%</span></div>
                            <div class="metric-trend positive" style="font-size:0.65rem"><i class="fa-solid fa-check"></i> ${bgEff > 95 ? 'Efficient' : 'Moderate'}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">Cluster State</span><i class="fa-solid fa-server card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem; color:var(--accent-blue);">${window.escapeHtml(data.cluster_state || 'primary')}</div>
                            <div class="metric-trend positive" style="font-size:0.65rem"><i class="fa-solid fa-check-double"></i> Healthy</div>
                        </div>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-layer-group"></i> Replica Count</h4>
                    <div style="display:flex; flex-direction:column; gap:0.5rem;">
                        <div class="metric-card glass-panel status-healthy" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">Replicas</span><i class="fa-solid fa-clone card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem; color:var(--accent-blue);">${standbys.length}</div>
                            <div class="metric-trend positive" style="font-size:0.65rem"><i class="fa-solid fa-check"></i> Connected</div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Replicas Table -->
            <div class="table-card glass-panel mt-3" style="padding:0.75rem;">
                <div class="card-header">
                    <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-clone text-accent"></i> Connected CNPG Replicas (${standbys.length})</h3>
                </div>
                ${standbys.length === 0 ? `
                    <div style="text-align:center; padding:2rem; color:var(--text-muted);">
                        <i class="fa-solid fa-info-circle"></i> No connected replicas found in pg_stat_replication
                    </div>
                ` : `
                    <div class="table-responsive" style="max-height:300px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead>
                                <tr>
                                    <th>Pod Name</th>
                                    <th>IP</th>
                                    <th>State</th>
                                    <th>Sync Mode</th>
                                    <th>Lag (MB)</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${standbys.map(r => `
                                    <tr style="background:${rowBg(r.state)};">
                                        <td><strong style="font-family:monospace; font-size:0.8rem;">${window.escapeHtml(r.replica_pod_name)}</strong></td>
                                        <td style="font-family:monospace; font-size:0.75rem;">${window.escapeHtml(r.pod_ip || 'N/A')}</td>
                                        <td><span class="badge ${r.state === 'streaming' ? 'badge-success' : 'badge-warning'}">${window.escapeHtml(r.state)}</span></td>
                                        <td style="color:${syncColor(r.sync_state)}; font-weight:600;">${window.escapeHtml(r.sync_state)}</td>
                                        <td style="color:${lagColor(r.replay_lag_mb)}; font-weight:${lagWeight(r.replay_lag_mb)};">
                                            ${r.replay_lag_mb.toFixed(2)}
                                            ${r.replay_lag_mb > 50 ? '<i class="fa-solid fa-exclamation-triangle" style="margin-left:4px; font-size:0.7rem;"></i>' : ''}
                                        </td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    </div>
                `}
            </div>
        `;
    } else {
        // Standby: 3-column layout matching PG Overview pattern
        html += `
            <div class="metrics-row" style="display:grid; grid-template-columns:1fr 1fr 1fr; gap:0.75rem; margin-top:0.75rem;">
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-clock"></i> Replay Lag</h4>
                    <div style="display:flex; flex-direction:column; gap:0.5rem;">
                        <div class="metric-card glass-panel ${localLag > 50 ? 'status-danger' : localLag > 10 ? 'status-warning' : 'status-healthy'}" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">Local Replay Lag</span><i class="fa-solid fa-clock card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem; color:${lagColor(localLag)};">${localLag.toFixed(2)}<span style="font-size:0.5em"> MB</span></div>
                            <div class="metric-trend ${localLag > 10 ? 'warning' : 'positive'}" style="font-size:0.65rem"><i class="fa-solid fa-${localLag > 10 ? 'triangle-exclamation' : 'check'}"></i> ${localLag > 50 ? 'CRITICAL' : localLag > 10 ? 'Elevated' : 'In Sync'}</div>
                        </div>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-cog"></i> Background Engine</h4>
                    <div style="display:flex; flex-direction:column; gap:0.5rem;">
                        <div class="metric-card glass-panel status-healthy" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">BGWriter Eff</span><i class="fa-solid fa-brush card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem; color:var(--success);">${bgEff.toFixed(0)}<span>%</span></div>
                            <div class="metric-trend positive" style="font-size:0.65rem"><i class="fa-solid fa-check"></i> ${bgEff > 95 ? 'Efficient' : 'Moderate'}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">Cluster State</span><i class="fa-solid fa-server card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem; color:var(--accent-blue);">${window.escapeHtml(data.cluster_state || 'standby')}</div>
                            <div class="metric-trend positive" style="font-size:0.65rem"><i class="fa-solid fa-check-double"></i> Standby</div>
                        </div>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-info"></i> Node Info</h4>
                    <div style="display:flex; flex-direction:column; gap:0.5rem;">
                        <div class="metric-card glass-panel status-info" style="padding:0.5rem 0.75rem;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.7rem">Node Role</span><i class="fa-solid fa-book-open card-icon"></i></div>
                            <div class="metric-value" style="font-size:1.4rem; color:#3b82f6;">Standby</div>
                            <div class="metric-trend" style="font-size:0.65rem; color:#3b82f6;"><i class="fa-solid fa-book-open"></i> Read-Only</div>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    contentEl.innerHTML = html;
};

window.refreshCNPGTopology = async function() {
    const contentEl = document.getElementById('cnpgContent');
    if (contentEl) {
        contentEl.innerHTML = '<div style="display:flex; justify-content:center; align-items:center; height:50vh;"><div class="spinner"></div><span style="margin-left:1rem;">Refreshing...</span></div>';
    }
    await window.loadCNPGTopology();
};
