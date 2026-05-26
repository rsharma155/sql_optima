/*
 * SQL Optima — PostgreSQL alert configuration and viewing.
 */

window.PgAlertsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...'};
    const instName = inst.name;

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
                <span class="text-muted">Loading alert engine and blocking telemetry...</span>
            </div>
        </div>
    `;

    const data = await window.fetchAlertsPageData(inst);

    if (data.disconnected) {
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

    const openAlerts = data.engineAlerts.filter(a => a.status === 'open' || a.status === 'acknowledged');
    const timelineItems = openAlerts.length > 0 ? openAlerts : data.engineAlerts;

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1><i class="fa-solid fa-bell text-danger"></i> Alerts &amp; Event Timeline</h1>
                    <span class="subtitle">Instance: ${window.escapeHtml(instName)} · PostgreSQL</span>
                </div>
                <div class="flex-center dashboard-page-title-actions" style="gap:1rem;">
                    <div class="glass-panel" style="padding:0.4rem 0.8rem; border-radius:8px;">
                        <span class="badge badge-${data.openCount > 0 ? 'danger' : 'success'}">${data.openCount} Open</span>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="PgAlertsView">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>

            ${window.renderAlertsKpiStrip(data.openCount, data.blockingKpis, data.engine)}

            <div class="grid-container mt-2">
                <div class="col-8 col-laptop-8 col-tablet-6">
                    <div class="card glass-panel">
                        <div class="card-header flex-between">
                            <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-database text-accent"></i> Alert Engine</h3>
                            <span class="text-muted" style="font-size:0.65rem;">Evaluated every ~60s · Updated ${new Date().toLocaleTimeString()}</span>
                        </div>
                        <div style="padding:0.5rem;">
                            ${window.renderEngineAlertsTable(data.engineAlerts, { showAllStatuses: true })}
                        </div>
                    </div>
                </div>
                <div class="col-4 col-laptop-4 col-tablet-6">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-clock text-accent"></i> Recent Activity</h3></div>
                        <div style="padding: 0.75rem; height: 360px; overflow-y: auto;">
                            <ul style="list-style:none; padding-left:0; font-size:0.75rem; margin:0;">
                                ${timelineItems.length === 0 ? `
                                    <li class="text-muted">No engine alerts yet. Blocking and replication rules run in the background.</li>
                                ` : timelineItems.slice(0, 20).map(a => `
                                    <li style="margin-bottom:0.75rem; padding-bottom:0.75rem; border-bottom:1px solid var(--border-color);">
                                        <div class="flex-between">
                                            <span class="text-${a.severity === 'critical' ? 'danger' : 'warning'}"><strong>${window.escapeHtml(a.title)}</strong></span>
                                            <span class="text-muted" style="font-size:0.65rem">${a.last_seen_at ? new Date(a.last_seen_at).toLocaleTimeString() : '--'}</span>
                                        </div>
                                        <div class="mt-1 text-muted">${window.escapeHtml(String(a.description || '').substring(0, 100))}</div>
                                    </li>
                                `).join('')}
                            </ul>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;

    window.bindAlertsPageActions(window.PgAlertsView);
};
