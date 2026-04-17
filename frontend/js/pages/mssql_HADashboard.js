/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: High Availability dashboard for Always On Availability Groups.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.HADashboardView = function() {
    const instance = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!instance) { alert('Select an instance first.'); return; }
    if (instance.type !== 'sqlserver') { alert('HA/AG monitoring is for SQL Server only.'); return; }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-server text-accent"></i> High Availability / AlwaysOn Monitoring</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(instance.name)}</p>
                </div>
                <div style="display:flex; align-items:center; gap: 1rem;">
                    ${window.renderStatusStrip({ lastUpdateId: 'haLastRefreshTime', sourceBadgeId: 'haDataSourceBadge', includeHealth: false, includeFreshness: false, autoRefreshText: '' })}
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="HADashboardView">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>

            <div style="display: grid; gap: 0.75rem; margin-top: 0.75rem;">
                <div class="glass-panel" style="padding: 0.75rem;">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-clone text-accent"></i> Availability Groups Health</h3>
                    </div>
                    <div id="ag-health-section">
                        <div style="display:flex; justify-content:center; align-items:center; height:100px;">
                            <div class="spinner"></div><span style="margin-left: 1rem;">Loading AG Health data...</span>
                        </div>
                    </div>
                </div>

                <div class="glass-panel" style="padding: 0.75rem;">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-chart-line text-accent"></i> Database Throughput (TPS)</h3>
                    </div>
                    <div id="db-throughput-section">
                        <div style="display:flex; justify-content:center; align-items:center; height:100px;">
                            <div class="spinner"></div><span style="margin-left: 1rem;">Loading DB Throughput data...</span>
                        </div>
                    </div>
                </div>

                <div class="glass-panel" style="padding: 0.75rem;">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-ship text-accent"></i> Log Shipping Health</h3>
                    </div>
                    <div id="log-shipping-section">
                        <div style="display:flex; justify-content:center; align-items:center; height:60px;">
                            <div class="spinner"></div><span style="margin-left: 1rem;">Loading Log Shipping data...</span>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;

    loadAGHealthData(instance.name);
    loadDBThroughputData(instance.name);
    loadLogShippingData(instance.name);
};

function loadAGHealthData(instanceName) {
    const section = document.getElementById('ag-health-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/mssql/ag-health?instance=${encodeURIComponent(instanceName)}`)
        .then(response => {
            window.updateSourceBadge('haDataSourceBadge', response.headers.get('X-Data-Source'));
            return response.json();
        })
        .then(data => {
            if (data.error) {
                section.innerHTML = `<div class="alert alert-warning"><i class="fa-solid fa-exclamation-triangle"></i> ${escapeHtml(data.error)}</div>`;
                return;
            }

            const agStats = data.ag_stats || data.ag_health || [];
            const hadrEnabled = (typeof data.hadr_enabled === 'boolean') ? data.hadr_enabled : (agStats.length > 0);

            if (!hadrEnabled) {
                section.innerHTML = `
                    <div class="alert alert-info">
                        <i class="fa-solid fa-circle-info"></i> <strong>No HADR configured for this server.</strong>
                        <p class="text-sm mt-1">
                            AlwaysOn Availability Groups are not enabled or no Availability Groups exist on this instance.
                        </p>
                    </div>
                `;
                return;
            }

            // HADR enabled banner
            let html = `
                <div class="alert" style="background: rgba(34,197,94,0.08); border: 1px solid rgba(34,197,94,0.25); color: var(--text);">
                    <i class="fa-solid fa-circle-check" style="color: var(--success);"></i>
                    <strong style="margin-left: 0.25rem; color: var(--success);">HADR enabled</strong>
                    <span class="text-muted" style="margin-left: 0.5rem;">Availability Groups detected on this instance.</span>
                </div>
            `;

            if (!agStats || agStats.length === 0) {
                html += `
                    <div class="alert alert-warning">
                        <i class="fa-solid fa-triangle-exclamation"></i> HADR is enabled, but no AG database replica rows were returned.
                        <p class="text-sm mt-1">This can happen with limited permissions, transient DMV issues, or if AG metadata is unavailable.</p>
                    </div>
                `;
                section.innerHTML = html;
                return;
            }

            html += '<div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 1rem; margin-top: 1rem;">';

            agStats.forEach(stat => {
                const isHealthy = stat.synchronization_state === 'SYNCHRONIZED' || stat.synchronization_state_desc === 'SYNCHRONIZED';
                const isSyncing = stat.synchronization_state === 'SYNCHRONIZING' || stat.synchronization_state_desc === 'SYNCHRONIZING';
                const healthColor = isHealthy ? 'var(--success)' : (isSyncing ? 'var(--warning)' : 'var(--danger)');
                const healthClass = isHealthy ? 'text-success' : (isSyncing ? 'text-warning' : 'text-danger');
                
                const isPrimary = stat.is_primary_replica;
                const roleBadge = isPrimary 
                    ? '<span class="badge badge-primary" style="font-size: 0.7rem; padding: 2px 6px;">PRIMARY</span>' 
                    : '<span class="badge badge-info" style="font-size: 0.7rem; padding: 2px 6px;">SECONDARY</span>';

                html += `
                    <div class="glass-panel" style="padding: 1.25rem; border-left: 4px solid ${healthColor}; border-radius: 8px; position: relative;">
                        <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 0.75rem;">
                            <div>
                                <h4 style="margin: 0; font-size: 1.1rem; color: var(--text);">${escapeHtml(stat.ag_name || 'N/A')}</h4>
                                <div style="font-size: 0.8rem; color: var(--text-muted); margin-top: 0.25rem;">
                                    <i class="fa-solid fa-server"></i> ${escapeHtml(stat.replica_server_name || 'N/A')}
                                </div>
                            </div>
                            <div style="text-align: right;">
                                ${roleBadge}
                            </div>
                        </div>
                        
                        <div style="background: rgba(0,0,0,0.2); border-radius: 6px; padding: 0.75rem; margin-bottom: 1rem;">
                            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
                                <span style="font-size: 0.8rem; color: var(--text-muted);">Database</span>
                                <strong style="font-size: 0.9rem; color: var(--accent-light);">${escapeHtml(stat.database_name || 'N/A')}</strong>
                            </div>
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <span style="font-size: 0.8rem; color: var(--text-muted);">Sync State</span>
                                <strong class="${healthClass}" style="font-size: 0.9rem; display: flex; align-items: center; gap: 6px;">
                                    <div style="width: 8px; height: 8px; border-radius: 50%; background: ${healthColor}; box-shadow: 0 0 8px ${healthColor};"></div>
                                    ${escapeHtml(stat.synchronization_state || 'UNKNOWN')}
                                </strong>
                            </div>
                        </div>

                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; text-align: center;">
                            <div style="background: rgba(255,255,255,0.03); padding: 0.5rem; border-radius: 4px;">
                                <div style="font-size: 0.7rem; color: var(--text-muted); text-transform: uppercase;">Avg Send Queue</div>
                                <div style="font-size: 1.1rem; font-family: 'JetBrains Mono', monospace; color: var(--text); margin-top: 0.25rem;">
                                    ${formatNumber(stat.avg_log_send_queue_kb || 0)} <span style="font-size: 0.65rem;">KB</span>
                                </div>
                                <div style="font-size: 0.65rem; color: var(--text-muted); margin-top: 0.15rem;">Max: ${formatNumber(stat.max_log_send_queue_kb || 0)} KB</div>
                            </div>
                            <div style="background: rgba(255,255,255,0.03); padding: 0.5rem; border-radius: 4px;">
                                <div style="font-size: 0.7rem; color: var(--text-muted); text-transform: uppercase;">Avg Redo Queue</div>
                                <div style="font-size: 1.1rem; font-family: 'JetBrains Mono', monospace; color: var(--text); margin-top: 0.25rem;">
                                    ${formatNumber(stat.avg_redo_queue_kb || 0)} <span style="font-size: 0.65rem;">KB</span>
                                </div>
                                <div style="font-size: 0.65rem; color: var(--text-muted); margin-top: 0.15rem;">Max: ${formatNumber(stat.max_redo_queue_kb || 0)} KB</div>
                            </div>
                        </div>
                        ${!isPrimary ? `
                        <div style="margin-top: 0.5rem; background: rgba(255,255,255,0.03); padding: 0.5rem; border-radius: 4px; text-align: center;">
                            <div style="font-size: 0.7rem; color: var(--text-muted); text-transform: uppercase;">Secondary Lag</div>
                            <div style="font-size: 1.1rem; font-family: 'JetBrains Mono', monospace; color: ${(stat.secondary_lag_secs || 0) > 30 ? 'var(--warning)' : 'var(--text)'}; margin-top: 0.25rem;">
                                ${formatNumber(stat.secondary_lag_secs || 0)} <span style="font-size: 0.65rem;">sec</span>
                            </div>
                        </div>` : ''}
                    </div>
                `;
            });

            html += `
                </div>
                <div class="table-footer" style="margin-top: 1.5rem;">
                    <small class="text-muted"><i class="fa-solid fa-clock-rotate-left"></i> Replica health from Timescale (last hour) when available, else live DMVs | Showing ${agStats.length} Replica Database(s)</small>
                </div>
            `;

            section.innerHTML = html;
        })
        .catch(error => {
            console.error('Error loading AG Health data:', error);
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load AG Health data: ${escapeHtml(error.message)}</div>`;
        });
}

function loadDBThroughputData(instanceName) {
    const section = document.getElementById('db-throughput-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/mssql/db-throughput?instance=${encodeURIComponent(instanceName)}`)
        .then(response => {
            // If AG call didn't set it (or differs), this will update it.
            window.updateSourceBadge('haDataSourceBadge', response.headers.get('X-Data-Source'));
            return response.json();
        })
        .then(data => {
            if (data.error) {
                section.innerHTML = `<div class="alert alert-warning"><i class="fa-solid fa-exclamation-triangle"></i> ${escapeHtml(data.error)}</div>`;
                return;
            }

            const dbStats = data.db_stats || data.db_throughput || [];
            if (!dbStats || dbStats.length === 0) {
                section.innerHTML = `
                    <div class="alert alert-info">
                        <i class="fa-solid fa-info-circle"></i> No DB Throughput data available yet.
                        <p class="text-sm mt-1">Data will appear after the first collection cycle (15 min).</p>
                    </div>
                `;
                return;
            }

            let html = `
                <table class="data-table">
                    <thead>
                        <tr>
                            <th>Database</th>
                            <th>Avg TPS</th>
                            <th>Max TPS</th>
                            <th>Avg Batch Requests/s</th>
                            <th>Total Reads</th>
                            <th>Total Writes</th>
                            <th>Samples</th>
                        </tr>
                    </thead>
                    <tbody>
            `;

            dbStats.forEach(stat => {
                html += `
                    <tr>
                        <td><strong>${escapeHtml(stat.database_name || 'N/A')}</strong></td>
                        <td class="text-right">${formatNumber(stat.avg_tps || 0, 2)}</td>
                        <td class="text-right">${formatNumber(stat.max_tps || 0, 2)}</td>
                        <td class="text-right">${formatNumber(stat.avg_batch_requests || 0, 2)}</td>
                        <td class="text-right">${formatNumber(stat.total_reads || 0)}</td>
                        <td class="text-right">${formatNumber(stat.total_writes || 0)}</td>
                        <td class="text-right">${stat.sample_count || 0}</td>
                    </tr>
                `;
            });

            html += `
                    </tbody>
                </table>
                <div class="table-footer">
                    <small class="text-muted">Showing ${dbStats.length} database(s) | Throughput from Timescale (last hour) when available, else live snapshot</small>
                </div>
            `;

            section.innerHTML = html;
        })
        .catch(error => {
            console.error('Error loading DB Throughput data:', error);
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load DB Throughput data: ${escapeHtml(error.message)}</div>`;
        });
}

function loadLogShippingData(instanceName) {
    const section = document.getElementById('log-shipping-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/mssql/log-shipping?instance=${encodeURIComponent(instanceName)}`)
        .then(response => response.json())
        .then(data => {
            if (data.error) {
                section.innerHTML = `<div class="alert alert-warning"><i class="fa-solid fa-exclamation-triangle"></i> ${window.escapeHtml(data.error)}</div>`;
                return;
            }

            const rows = data.log_shipping || [];
            if (!data.log_shipping_enabled || rows.length === 0) {
                section.innerHTML = `
                    <div class="alert alert-info">
                        <i class="fa-solid fa-circle-info"></i> <strong>No Log Shipping configured for this instance.</strong>
                        <p class="text-sm mt-1">Log Shipping is not set up or no monitor rows were found in msdb.</p>
                    </div>
                `;
                return;
            }

            const statusLabel = s => ({ 1: '<span class="badge badge-success">OK</span>', 2: '<span class="badge badge-warning">WARNING</span>', 3: '<span class="badge badge-danger">ERROR</span>' }[s] || '<span class="badge">UNKNOWN</span>');
            const fmtDate = d => d ? new Date(d).toLocaleString() : '<span class="text-muted">—</span>';

            let html = `
                <table class="data-table">
                    <thead>
                        <tr>
                            <th>Primary DB</th>
                            <th>Secondary Server</th>
                            <th>Secondary DB</th>
                            <th>Last Backup</th>
                            <th>Last Restored</th>
                            <th>Restore Delay</th>
                            <th>Threshold</th>
                            <th>Status</th>
                        </tr>
                    </thead>
                    <tbody>
            `;
            rows.forEach(r => {
                html += `<tr>
                    <td><strong>${window.escapeHtml(r.primary_database || '—')}</strong></td>
                    <td>${window.escapeHtml(r.secondary_server || '—')}</td>
                    <td>${window.escapeHtml(r.secondary_database || '—')}</td>
                    <td>${fmtDate(r.last_backup_date)}</td>
                    <td>${fmtDate(r.last_restore_date)}</td>
                    <td class="text-right">${r.restore_delay_minutes ?? '—'} min</td>
                    <td class="text-right">${r.restore_threshold_minutes ?? '—'} min</td>
                    <td>${statusLabel(r.status)}</td>
                </tr>`;
            });
            html += `</tbody></table>
                <div class="table-footer"><small class="text-muted">Showing ${rows.length} log shipping pair(s) | Data from Timescale (1-hour lookback) when available, else live msdb</small></div>`;

            section.innerHTML = html;
        })
        .catch(err => {
            console.error('Error loading log shipping data:', err);
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load log shipping data: ${window.escapeHtml(err.message)}</div>`;
        });
}

function formatNumber(num, decimals = 0) {
    if (num === null || num === undefined) return '0';
    if (decimals > 0) {
        return Number(num).toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
    }
    return Number(num).toLocaleString('en-US');
}
