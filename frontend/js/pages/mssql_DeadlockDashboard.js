/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Deadlock dashboard with deadlock graph and history.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.mssql_DeadlockDashboard = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};

    // Show loading state
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme" style="display:flex; justify-content:center; align-items:center; height:100%;">
            <div class="spinner"></div><span style="margin-left: 1rem;">Loading deadlock and blocking information...</span>
        </div>
    `;

    try {
        // Fetch dashboard data for blocking information
        const response = await window.apiClient.authenticatedFetch(`/api/mssql/dashboard?instance=${encodeURIComponent(inst.name)}`);
        if (!response.ok) throw new Error("Failed to fetch dashboard data");
        const metrics = await response.json();

        // Get blocking sessions data
        let dbFilter = window.appState.currentDatabase;
        let allSessions = metrics.active_blocks || [];
        if (dbFilter !== 'all') {
            allSessions = allSessions.filter(s => s.database_name === dbFilter);
        }
        allSessions.forEach(s => s.children = []);
        let blockRoots = [];
        allSessions.forEach(s => {
            if (s.blocking_session_id === 0) blockRoots.push(s);
            else {
                let parent = allSessions.find(p => p.blocked_session_id === s.blocking_session_id);
                if (parent) parent.children.push(s);
                else blockRoots.push(s);
            }
        });

        // Build blocking hierarchy HTML
        let bHtml = "";
        function renderNode(n, depth) {
            let isBlocked = n.blocking_session_id !== 0;
            let isBlocker = n.children.length > 0;
            let pad = '&nbsp;'.repeat(depth * 5) + (depth > 0 ? '↳ ' : '');
            let escQ = encodeURIComponent(n.query_text);
            bHtml += `
                <tr class="${isBlocked ? 'bg-danger-subtle' : ''}">
                    <td>${pad}<span class="badge ${isBlocked ? 'badge-danger' : (isBlocker ? 'badge-warning' : 'badge-primary')}">SPID ${n.blocked_session_id}</span></td>
                    <td><span class="badge badge-outline">${window.escapeHtml(n.status)}</span></td>
                    <td>${isBlocked ? n.blocking_session_id : '-'}</td>
                    <td>${window.escapeHtml(n.wait_type)}</td>
                    <td>${n.wait_time_ms} ms</td>
                    <td>${window.escapeHtml(n.database_name)}</td>
                    <td><span class="text-warning">${window.escapeHtml(n.host_name)}</span><br/><span style="font-size:0.7rem">${window.escapeHtml(n.program_name)}</span></td>
                    <td><span class="code-snippet" style="cursor:pointer;" title="View Query" data-action="show-query-modal-direct" data-encoded-query="${escQ}">${window.escapeHtml(n.query_text).substring(0, 30)}...</span></td>
                </tr>`;
            n.children.forEach(c => renderNode(c, depth + 1));
        }
        blockRoots.forEach(r => renderNode(r, 0));

        // Get deadlock information from extended events
        let deadlockEvents = [];
        if (metrics.xevent_metrics && metrics.xevent_metrics.recent_events) {
            deadlockEvents = metrics.xevent_metrics.recent_events.filter(e => {
                try {
                    const parsed = JSON.parse(e.parsed_payload_json || '{}');
                    return parsed.event_type && parsed.event_type.toLowerCase().includes('deadlock');
                } catch (err) {
                    return false;
                }
            });
        }

        // Build deadlock events HTML
        let deadlockHtml = deadlockEvents.slice(0, 20).map(e => {
            let parsedData = {};
            try {
                parsedData = JSON.parse(e.parsed_payload_json || '{}');
            } catch (err) {
                appDebug('Failed to parse deadlock event data:', err);
            }

            return `
                <tr>
                    <td><span class="badge badge-outline">${e.event_timestamp ? e.event_timestamp.substring(11, 19) : 'N/A'}</span></td>
                    <td><span class="badge badge-danger">Deadlock</span></td>
                    <td>${window.escapeHtml(parsedData.database_name || 'N/A')}</td>
                    <td><span class="text-warning">${window.escapeHtml(parsedData.client_hostname || 'N/A')}</span></td>
                    <td><span class="text-accent">${window.escapeHtml(parsedData.client_app_name || 'N/A')}</span></td>
                    <td><span class="text-success">${window.escapeHtml(parsedData.username || 'N/A')}</span></td>
                    <td>
                        <button class="btn btn-xs btn-accent" data-action="call" data-fn="showDeadlockDetails" data-arg="${encodeURIComponent(JSON.stringify(e))}" title="View Deadlock Details">
                            <i class="fa-solid fa-eye"></i>
                        </button>
                    </td>
                </tr>
            `;
        }).join('');

        // Calculate summary stats
        const totalBlocks = allSessions.filter(s => s.blocking_session_id !== 0).length;
        const totalBlockers = allSessions.filter(s => s.children && s.children.length > 0).length;
        const totalDeadlocks = deadlockEvents.length;

        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title flex-between">
                    <div>
                        <button class="btn btn-sm btn-outline mb-2" data-action="navigate" data-route="dashboard"><i class="fa-solid fa-arrow-left"></i> Back to Live Dashboard</button>
                        <h1><i class="fa-solid fa-lock text-danger"></i> Blocking & Deadlock Analysis</h1>
                        <p class="subtitle">Real-time blocking chains and deadlock history [${window.escapeHtml(inst.name)}]</p>
                    </div>
                    <div class="custom-select-group">
                        <button class="btn btn-accent btn-sm" data-action="call" data-fn="refreshDeadlockData">
                            <i class="fa-solid fa-refresh"></i> Refresh Data
                        </button>
                    </div>
                </div>

                <div class="metrics-grid mt-4">
                    <div class="metric-card glass-panel ${totalBlocks > 0 ? 'status-warning' : 'status-healthy'}">
                        <div class="metric-header"><span class="metric-title">Active Blocks</span><i class="fa-solid fa-link-slash card-icon"></i></div>
                        <div class="metric-value" style="font-size: 1.5rem">${totalBlocks}</div>
                        <div class="metric-subtitle">Sessions currently blocked</div>
                    </div>
                    <div class="metric-card glass-panel ${totalBlockers > 0 ? 'status-warning' : 'status-healthy'}">
                        <div class="metric-header"><span class="metric-title">Blocking Sessions</span><i class="fa-solid fa-hand-paper card-icon"></i></div>
                        <div class="metric-value" style="font-size: 1.5rem">${totalBlockers}</div>
                        <div class="metric-subtitle">Sessions causing blocks</div>
                    </div>
                    <div class="metric-card glass-panel ${totalDeadlocks > 0 ? 'status-danger' : 'status-healthy'}">
                        <div class="metric-header"><span class="metric-title">Recent Deadlocks</span><i class="fa-solid fa-exclamation-triangle card-icon"></i></div>
                        <div class="metric-value" style="font-size: 1.5rem">${totalDeadlocks}</div>
                        <div class="metric-subtitle">Deadlocks in last hour</div>
                    </div>
                    <div class="metric-card glass-panel status-info">
                        <div class="metric-header"><span class="metric-title">Database Filter</span><i class="fa-solid fa-database card-icon"></i></div>
                        <div class="metric-value" style="font-size: 1.2rem">${window.escapeHtml(window.appState.currentDatabase)}</div>
                        <div class="metric-subtitle">Current filter applied</div>
                    </div>
                </div>

                <div class="tables-grid mt-4">
                    <div class="table-card glass-panel" style="grid-column: span 2;">
                        <div class="card-header">
                            <h3><i class="fa-solid fa-sitemap text-warning"></i> Current Blocking Hierarchy (DMV Data)</h3>
                        </div>
                        <div class="table-responsive" style="max-height: 500px; overflow-y: auto;">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Session ID Chain</th>
                                        <th>Status</th>
                                        <th>Blocked By</th>
                                        <th>Wait Type</th>
                                        <th>Wait Time</th>
                                        <th>Database</th>
                                        <th>Host/Application</th>
                                        <th>Query Preview</th>
                                    </tr>
                                </thead>
                                <tbody>${bHtml || '<tr><td colspan="8" class="text-center text-success"><i class="fa-solid fa-check"></i> No active blocking chains detected.</td></tr>'}</tbody>
                            </table>
                        </div>
                    </div>

                    ${totalDeadlocks > 0 ? `
                    <div class="table-card glass-panel" style="grid-column: span 2; margin-top: 1.5rem;">
                        <div class="card-header">
                            <h3><i class="fa-solid fa-skull-crossbones text-danger"></i> Recent Deadlock Events (Extended Events)</h3>
                        </div>
                        <div class="table-responsive" style="max-height: 400px; overflow-y: auto;">
                            <table class="data-table">
                                <thead>
                                    <tr>
                                        <th>Timestamp</th>
                                        <th>Event Type</th>
                                        <th>Database</th>
                                        <th>Client Host</th>
                                        <th>Application</th>
                                        <th>Username</th>
                                        <th>Actions</th>
                                    </tr>
                                </thead>
                                <tbody>${deadlockHtml}</tbody>
                            </table>
                        </div>
                    </div>
                    ` : ''}
                </div>
            </div>
        `;

    } catch (error) {
        appDebug("Deadlock dashboard error:", error);
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title">
                    <button class="btn btn-sm btn-outline mb-2" data-action="navigate" data-route="dashboard"><i class="fa-solid fa-arrow-left"></i> Back to Dashboard</button>
                    <h1>Error Loading Deadlock Data</h1>
                </div>
                <div class="alert alert-danger mt-4">
                    <i class="fa-solid fa-exclamation-triangle"></i>
                    Failed to load blocking and deadlock information: ${error.message}
                </div>
            </div>
        `;
    }
};

// Refresh function for deadlock dashboard
window.refreshDeadlockData = async function() {
    const refreshBtn = document.querySelector('button[data-action="call" data-fn="refreshDeadlockData"]');
    if (refreshBtn) {
        refreshBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Refreshing...';
        refreshBtn.disabled = true;
    }

    try {
        await window.mssql_DeadlockDashboard();

        if (refreshBtn) {
            refreshBtn.innerHTML = '<i class="fa-solid fa-check"></i> Refreshed';
            setTimeout(() => {
                refreshBtn.innerHTML = '<i class="fa-solid fa-refresh"></i> Refresh Data';
                refreshBtn.disabled = false;
            }, 2000);
        }
    } catch (error) {
        appDebug("Deadlock refresh failed:", error);
        if (refreshBtn) {
            refreshBtn.innerHTML = '<i class="fa-solid fa-exclamation-triangle"></i> Failed';
            setTimeout(() => {
                refreshBtn.innerHTML = '<i class="fa-solid fa-refresh"></i> Refresh Data';
                refreshBtn.disabled = false;
            }, 3000);
        }
    }
};

// Show deadlock details modal
window.showDeadlockDetails = function(encodedEventData) {
    try {
        const eventData = JSON.parse(decodeURIComponent(encodedEventData));
        let parsedData = {};
        try {
            parsedData = JSON.parse(eventData.parsed_payload_json || '{}');
        } catch (err) {
            appDebug('Failed to parse deadlock event data:', err);
        }

        // Create modal for deadlock details
        const modal = document.createElement('div');
        modal.className = 'modal-overlay';
        modal.innerHTML = `
            <div class="modal-content" style="max-width: 800px;">
                <div class="modal-header">
                    <h3><i class="fa-solid fa-skull-crossbones text-danger"></i> Deadlock Event Details</h3>
                    <button class="modal-close" data-action="remove-closest" data-closest=".modal-overlay">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="alert alert-danger">
                        <strong>Deadlock Detected:</strong> A deadlock occurred involving database operations.
                    </div>

                    <div class="table-responsive mt-3">
                        <table class="data-table">
                            <tbody>
                                <tr>
                                    <td><strong>Timestamp:</strong></td>
                                    <td>${eventData.event_timestamp || 'N/A'}</td>
                                </tr>
                                <tr>
                                    <td><strong>Database:</strong></td>
                                    <td><span class="badge badge-info">${window.escapeHtml(parsedData.database_name || 'N/A')}</span></td>
                                </tr>
                                <tr>
                                    <td><strong>Client Host:</strong></td>
                                    <td><span class="text-warning">${window.escapeHtml(parsedData.client_hostname || 'N/A')}</span></td>
                                </tr>
                                <tr>
                                    <td><strong>Application:</strong></td>
                                    <td><span class="text-accent">${window.escapeHtml(parsedData.client_app_name || 'N/A')}</span></td>
                                </tr>
                                <tr>
                                    <td><strong>Username:</strong></td>
                                    <td><span class="text-success">${window.escapeHtml(parsedData.username || 'N/A')}</span></td>
                                </tr>
                                <tr>
                                    <td><strong>Event Type:</strong></td>
                                    <td><span class="badge badge-danger">${window.escapeHtml(parsedData.event_type || 'DEADLOCK')}</span></td>
                                </tr>
                            </tbody>
                        </table>
                    </div>

                    ${parsedData.xml_report ? `
                    <div class="mt-3">
                        <h4>Deadlock XML Report:</h4>
                        <div style="background: rgba(0,0,0,0.3); border-radius: 8px; padding: 15px; font-family: monospace; font-size: 0.8rem; white-space: pre-wrap; max-height: 300px; overflow-y: auto;">
                            ${window.escapeHtml(parsedData.xml_report)}
                        </div>
                    </div>
                    ` : ''}

                    <div class="mt-3">
                        <h4>Raw Event Data:</h4>
                        <div style="background: rgba(0,0,0,0.2); border-radius: 8px; padding: 10px; font-family: monospace; font-size: 0.7rem; max-height: 200px; overflow-y: auto;">
                            <pre>${JSON.stringify(eventData, null, 2)}</pre>
                        </div>
                    </div>
                </div>
                <div class="modal-footer">
                    <button class="btn btn-secondary" data-action="remove-closest" data-closest=".modal-overlay">Close</button>
                </div>
            </div>
        `;

        document.body.appendChild(modal);
    } catch (err) {
        console.error('Failed to show deadlock details:', err);
    }
};