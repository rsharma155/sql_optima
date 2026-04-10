window.PgAlertsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...'};

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-bell text-danger"></i> PostgreSQL Alerts & Event Timeline</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)}</p>
                </div>
                <div class="flex-between" style="align-items:center; gap: 1rem;">
                    <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="alertsLastRefreshTime">--:--:--</span></span>
                    <button class="btn btn-sm btn-outline text-accent" onclick="window.PgAlertsView()">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>

            <div style="display:flex; justify-content:center; align-items:center; height:200px;">
                <div class="spinner"></div><span style="margin-left: 1rem;">Loading alerts...</span>
            </div>
        </div>
    `;

    let alertsData = { alerts: [] };
    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/postgres/alerts?instance=${encodeURIComponent(inst.name)}`
        );
        if (response.ok) {
            alertsData = await response.json();
        } else {
            console.error("Failed to load PG alerts:", response.status);
        }
    } catch (e) {
        console.error("PG alerts fetch failed:", e);
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-bell text-danger"></i> PostgreSQL Alerts & Event Timeline</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)}</p>
                </div>
                <div class="flex-between" style="align-items:center; gap: 1rem;">
                    <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="alertsLastRefreshTime">${new Date().toLocaleTimeString()}</span></span>
                    <button class="btn btn-sm btn-outline text-accent" onclick="window.PgAlertsView()">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>

            <div class="charts-grid mt-3">
                <div class="table-card glass-panel" style="grid-column: span 2;">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0;">Active Incidents</h3>
                        <span class="badge badge-${alertsData.alerts.length > 0 ? 'danger' : 'info'}">${alertsData.alerts.length}</span>
                    </div>
                    <div class="table-responsive" style="max-height: 300px; overflow-y: auto;">
                        <table class="data-table" style="font-size: 0.75rem;">
                            <thead>
                                <tr><th style="min-width: 80px;">Severity</th><th style="min-width: 150px;">Metric / Event</th><th style="min-width: 100px;">Threshold</th><th style="min-width: 100px;">Current Value</th><th style="min-width: 150px;">Timestamp</th><th style="min-width: 80px;">Status</th></tr>
                            </thead>
                            <tbody>
                                ${alertsData.alerts.map(alert => `
                                    <tr>
                                        <td><span class="badge badge-${alert.severity === 'CRITICAL' ? 'danger' : 'warning'}">${alert.severity}</span></td>
                                        <td><strong>${window.escapeHtml(alert.metric)}</strong></td>
                                        <td><span class="text-muted">${window.escapeHtml(alert.threshold)}</span></td>
                                        <td><span class="text-${alert.severity === 'CRITICAL' ? 'danger' : 'warning'} fw-bold">${window.escapeHtml(alert.current_val)}</span></td>
                                        <td>${window.escapeHtml(alert.timestamp)}</td>
                                        <td><span class="text-${alert.status === 'ACTIVE' ? 'danger' : 'warning'}"><i class="fa-solid fa-${alert.status === 'ACTIVE' ? 'triangle-exclamation' : 'eye'}"></i> ${alert.status}</span></td>
                                    </tr>
                                `).join('')}
                                ${alertsData.alerts.length === 0 ? '<tr><td colspan="6" class="text-center text-muted">No active alerts</td></tr>' : ''}
                            </tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-clock text-accent"></i> Event Timeline</h3>
                    </div>
                    <div style="padding: 0.75rem; background: rgba(0,0,0,0.2); border-radius: 8px; max-height: 300px; overflow-y: auto;">
                        <ul style="list-style:none; padding-left:0; font-size:0.8rem; margin:0;">
                            <li style="margin-bottom:0.75rem; padding-bottom:0.5rem; border-bottom:1px solid var(--border-color);">
                                <div><i class="fa-solid fa-check-circle text-success"></i> <strong>System Status</strong></div>
                                <div class="text-muted" style="font-size:0.7rem">${new Date().toLocaleString()}</div>
                                <div>Monitoring active - ${alertsData.alerts.length} alert${alertsData.alerts.length !== 1 ? 's' : ''} detected</div>
                            </li>
                            ${alertsData.alerts.slice(0, 5).map((alert, i) => `
                                <li style="margin-bottom:0.75rem; padding-bottom:0.5rem; border-bottom:1px solid var(--border-color);">
                                    <div><i class="fa-solid fa-${alert.severity === 'CRITICAL' ? 'exclamation-triangle' : 'exclamation-circle'} text-${alert.severity === 'CRITICAL' ? 'danger' : 'warning'}"></i> <strong>${window.escapeHtml(alert.metric)}</strong></div>
                                    <div class="text-muted" style="font-size:0.7rem">${window.escapeHtml(alert.timestamp)}</div>
                                    <div>Current: <span class="text-${alert.severity === 'CRITICAL' ? 'danger' : 'warning'} fw-bold">${window.escapeHtml(alert.current_val)}</span></div>
                                </li>
                            `).join('')}
                        </ul>
                    </div>
                </div>
            </div>
        </div>
    `;
}
