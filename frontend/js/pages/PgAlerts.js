/*
 * SQL Optima — PostgreSQL alert configuration and viewing.
 */

window.PgAlertsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...'};
    const instName = inst.name;
    const engine = (inst.engine || inst.type || 'postgres').toLowerCase().includes('sql') ? 'sqlserver' : 'postgres';

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1><i class="fa-solid fa-bell text-danger"></i> Alerts &amp; Event Timeline</h1>
                    <span class="subtitle">Active incidents and historical event logs for ${window.escapeHtml(instName)}</span>
                </div>
                <div class="flex-center dashboard-page-title-actions">
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="PgAlertsView">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>
            <div class="flex-center" style="height:200px; flex-direction:column; gap:1rem;">
                <div class="spinner"></div>
                <span class="text-muted">Analyzing incident telemetry...</span>
            </div>
        </div>
    `;

    // Fetch alerts and open count in parallel
    let alerts = [];
    let openCount = 0;
    let disconnected = false;
    try {
        const qs = `instance=${encodeURIComponent(instName)}&engine=${encodeURIComponent(engine)}&status=open`;
        const [alertsResp, countResp] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/alerts?${qs}`),
            window.apiClient.authenticatedFetch(`/api/alerts/count?instance=${encodeURIComponent(instName)}&engine=${encodeURIComponent(engine)}`)
        ]);
        if (alertsResp.status === 503 || countResp.status === 503) disconnected = true;
        if (alertsResp.ok) {
            const body = await alertsResp.json();
            alerts = (body && (body.data && body.data.alerts || body.alerts)) ? (body.data && body.data.alerts || body.alerts) : [];
        }
        if (countResp.ok) {
            const body = await countResp.json();
            openCount = (body && body.data && body.data.count != null) ? body.data.count : (body && body.count != null ? body.count : 0);
        }
    } catch (e) { console.error("Alert engine fetch failed:", e); }

    if (disconnected) {
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title flex-between">
                    <div><h1><i class="fa-solid fa-bell text-muted"></i> Alerts</h1></div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="PgAlertsView"><i class="fa-solid fa-refresh"></i> Retry</button>
                </div>
                <div class="glass-panel" style="margin-top:2rem; padding:3rem; text-align:center;">
                    <i class="fa-solid fa-plug-circle-xmark" style="font-size:3rem; color:var(--text-muted); margin-bottom:1.5rem; display:block;"></i>
                    <h2>Alert Engine Disconnected</h2>
                    <p class="text-muted">The alert engine requires a connection to TimescaleDB.</p>
                </div>
            </div>
        `;
        return;
    }

    const severityBadge = (s) => {
        const cls = s === 'critical' ? 'danger' : s === 'warning' ? 'warning' : 'info';
        return `<span class="badge badge-${cls}">${window.escapeHtml(s.toUpperCase())}</span>`;
    };

    const statusIcon = (s) => {
        if (s === 'acknowledged') return '<i class="fa-solid fa-eye text-warning"></i> ACK';
        if (s === 'resolved') return '<i class="fa-solid fa-check-circle text-success"></i> RESOLVED';
        return '<i class="fa-solid fa-triangle-exclamation text-danger"></i> OPEN';
    };

    const alertRows = alerts.length === 0
        ? '<tr><td colspan="7" class="text-center text-muted">No active alerts</td></tr>'
        : alerts.map(a => `
            <tr>
                <td>${severityBadge(a.severity)}</td>
                <td>${window.escapeHtml(a.category || '')}</td>
                <td><strong>${window.escapeHtml(a.title)}</strong></td>
                <td class="small" title="${window.escapeHtml(a.description || '')}">${window.escapeHtml((a.description || '').substring(0, 80))}</td>
                <td class="text-center">${a.hit_count || 1}</td>
                <td class="small text-muted">${a.last_seen_at ? new Date(a.last_seen_at).toLocaleString() : '--'}</td>
                <td style="white-space:nowrap;">
                    ${a.status === 'open' ? `
                        <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="ack">Ack</button>
                        <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="resolve">Resolve</button>
                    ` : a.status === 'acknowledged' ? `
                        <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="resolve">Resolve</button>
                    ` : statusIcon(a.status)}
                </td>
            </tr>
        `).join('');

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1><i class="fa-solid fa-bell text-danger"></i> Alerts &amp; Event Timeline</h1>
                    <span class="subtitle">Instance: ${window.escapeHtml(instName)}</span>
                </div>
                <div class="flex-center dashboard-page-title-actions" style="gap:1rem;">
                    <div class="glass-panel" style="padding:0.4rem 0.8rem; border-radius:8px;">
                        <span class="badge badge-${openCount > 0 ? 'danger' : 'success'}">${openCount} Open Incidents</span>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="PgAlertsView">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>

            <div class="grid-container mt-3">
                <!-- Row 1: Main Alerts Table -->
                <div class="col-8 col-laptop-8 col-tablet-6">
                    <div class="card glass-panel">
                        <div class="card-header flex-between">
                            <h3 style="font-size:0.85rem; margin:0;">Active Incidents</h3>
                            <span class="text-muted" style="font-size:0.65rem;">Updated: ${new Date().toLocaleTimeString()}</span>
                        </div>
                        <div class="table-container-compact" style="height: 500px; overflow-y: auto;">
                            <table class="modern-table modern-table-compact">
                                <thead>
                                    <tr>
                                        <th>Severity</th><th>Category</th><th>Title</th><th>Description</th><th>Hits</th><th>Last Seen</th><th>Actions</th>
                                    </tr>
                                </thead>
                                <tbody>${alertRows}</tbody>
                            </table>
                        </div>
                    </div>
                </div>

                <!-- Row 1, Col 2: Event Timeline -->
                <div class="col-4 col-laptop-4 col-tablet-6">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-clock text-accent"></i> Recent Activity</h3></div>
                        <div style="padding: 0.75rem; height: 500px; overflow-y: auto;">
                            <ul style="list-style:none; padding-left:0; font-size:0.75rem; margin:0;">
                                <li style="margin-bottom:0.75rem; padding-bottom:0.75rem; border-bottom:1px solid var(--border-color);">
                                    <div class="flex-between">
                                        <span class="text-success"><i class="fa-solid fa-check-circle"></i> Monitoring Active</span>
                                        <span class="text-muted" style="font-size:0.65rem">${new Date().toLocaleTimeString()}</span>
                                    </div>
                                    <div class="mt-1">System health checks running normally.</div>
                                </li>
                                ${alerts.slice(0, 15).map(a => `
                                    <li style="margin-bottom:0.75rem; padding-bottom:0.75rem; border-bottom:1px solid var(--border-color);">
                                        <div class="flex-between">
                                            <span class="text-${a.severity === 'critical' ? 'danger' : 'warning'}"><strong>${window.escapeHtml(a.title)}</strong></span>
                                            <span class="text-muted" style="font-size:0.65rem">${a.last_seen_at ? new Date(a.last_seen_at).toLocaleTimeString() : '--'}</span>
                                        </div>
                                        <div class="mt-1 text-muted">${window.escapeHtml((a.description || '').substring(0, 100))}</div>
                                    </li>
                                `).join('')}
                            </ul>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;

    if (window._pgAlertsActionHandler) {
        window.routerOutlet.removeEventListener('click', window._pgAlertsActionHandler);
    }
    window._pgAlertsActionHandler = function onAlertAction(e) {
        const btn = e.target.closest('[data-alert-action]');
        if (!btn) return;
        const id = btn.dataset.alertId;
        const action = btn.dataset.alertAction;
        if (action === 'ack') window._alertAck(id);
        else if (action === 'resolve') window._alertResolve(id);
    };
    window.routerOutlet.addEventListener('click', window._pgAlertsActionHandler);
};

window._alertAck = async function(id) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/alerts/${encodeURIComponent(id)}/acknowledge`, { method: 'POST' });
        if (resp.ok) window.PgAlertsView();
    } catch (e) { console.error('Ack failed', e); }
};

window._alertResolve = async function(id) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/alerts/${encodeURIComponent(id)}/resolve`, { method: 'POST' });
        if (resp.ok) window.PgAlertsView();
    } catch (e) { console.error('Resolve failed', e); }
};
