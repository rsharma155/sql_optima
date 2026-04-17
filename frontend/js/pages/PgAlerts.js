/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL alert configuration and viewing.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgAlertsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...'};
    const instName = inst.name;
    const engine = (inst.engine || inst.type || 'postgres').toLowerCase().includes('sql') ? 'sqlserver' : 'postgres';

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-bell text-danger"></i> Alerts &amp; Event Timeline</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(instName)}</p>
                </div>
                <div class="flex-between" style="align-items:center; gap: 1rem;">
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="PgAlertsView">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>
            <div style="display:flex; justify-content:center; align-items:center; height:200px;">
                <div class="spinner"></div><span style="margin-left: 1rem;">Loading alerts...</span>
            </div>
        </div>
    `;

    // Fetch alerts and open count in parallel
    let alerts = [];
    let openCount = 0;
    try {
        const qs = `instance=${encodeURIComponent(instName)}&engine=${encodeURIComponent(engine)}&status=open`;
        const [alertsResp, countResp] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/alerts?${qs}`),
            window.apiClient.authenticatedFetch(`/api/alerts/count?instance=${encodeURIComponent(instName)}&engine=${encodeURIComponent(engine)}`)
        ]);
        if (alertsResp.ok) {
            const body = await alertsResp.json();
            alerts = (body.data && body.data.alerts) || body.alerts || [];
        }
        if (countResp.ok) {
            const body = await countResp.json();
            openCount = (body.data && body.data.count) != null ? body.data.count : 0;
        }
    } catch (e) {
        console.error("Alert engine fetch failed:", e);
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

    const fmtTime = (iso) => iso ? new Date(iso).toLocaleString() : '--';

    const alertRows = alerts.length === 0
        ? '<tr><td colspan="7" class="text-center text-muted">No active alerts</td></tr>'
        : alerts.map(a => `
            <tr>
                <td>${severityBadge(a.severity)}</td>
                <td>${window.escapeHtml(a.category || '')}</td>
                <td><strong>${window.escapeHtml(a.title)}</strong></td>
                <td title="${window.escapeHtml(a.description || '')}">${window.escapeHtml((a.description || '').substring(0, 80))}</td>
                <td>${a.hit_count || 1}</td>
                <td>${fmtTime(a.last_seen_at)}</td>
                <td style="white-space:nowrap;">
                    ${statusIcon(a.status)}
                    ${a.status === 'open' ? `
                        <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="ack">Ack</button>
                        <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="resolve">Resolve</button>
                    ` : ''}
                    ${a.status === 'acknowledged' ? `
                        <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="resolve">Resolve</button>
                    ` : ''}
                </td>
            </tr>
        `).join('');

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-bell text-danger"></i> Alerts &amp; Event Timeline</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(instName)}</p>
                </div>
                <div class="flex-between" style="align-items:center; gap: 1rem;">
                    <span class="badge badge-${openCount > 0 ? 'danger' : 'info'}" style="font-size:0.85rem;">${openCount} Open</span>
                    <span class="text-muted" style="font-size:0.75rem;">Updated: ${new Date().toLocaleTimeString()}</span>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="PgAlertsView">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>

            <div class="charts-grid mt-3">
                <div class="table-card glass-panel" style="grid-column: span 2;">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0;">Active Incidents</h3>
                        <span class="badge badge-${alerts.length > 0 ? 'danger' : 'info'}">${alerts.length}</span>
                    </div>
                    <div class="table-responsive" style="max-height: 400px; overflow-y: auto;">
                        <table class="data-table" style="font-size: 0.75rem;">
                            <thead>
                                <tr>
                                    <th style="min-width:80px;">Severity</th>
                                    <th style="min-width:100px;">Category</th>
                                    <th style="min-width:150px;">Title</th>
                                    <th style="min-width:200px;">Description</th>
                                    <th style="min-width:50px;">Hits</th>
                                    <th style="min-width:140px;">Last Seen</th>
                                    <th style="min-width:140px;">Status / Actions</th>
                                </tr>
                            </thead>
                            <tbody>${alertRows}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-clock text-accent"></i> Event Timeline</h3>
                    </div>
                    <div style="padding: 0.75rem; background: rgba(0,0,0,0.2); border-radius: 8px; max-height: 400px; overflow-y: auto;">
                        <ul style="list-style:none; padding-left:0; font-size:0.8rem; margin:0;">
                            <li style="margin-bottom:0.75rem; padding-bottom:0.5rem; border-bottom:1px solid var(--border-color);">
                                <div><i class="fa-solid fa-check-circle text-success"></i> <strong>System Status</strong></div>
                                <div class="text-muted" style="font-size:0.7rem">${new Date().toLocaleString()}</div>
                                <div>Monitoring active — ${openCount} open alert${openCount !== 1 ? 's' : ''}</div>
                            </li>
                            ${alerts.slice(0, 8).map(a => `
                                <li style="margin-bottom:0.75rem; padding-bottom:0.5rem; border-bottom:1px solid var(--border-color);">
                                    <div><i class="fa-solid fa-${a.severity === 'critical' ? 'exclamation-triangle' : 'exclamation-circle'} text-${a.severity === 'critical' ? 'danger' : 'warning'}"></i> <strong>${window.escapeHtml(a.title)}</strong></div>
                                    <div class="text-muted" style="font-size:0.7rem">${fmtTime(a.first_seen_at)} — seen ${a.hit_count || 1}x</div>
                                    <div>${window.escapeHtml((a.description || '').substring(0, 120))}</div>
                                </li>
                            `).join('')}
                        </ul>
                    </div>
                </div>
            </div>
        </div>
    `;

    // Wire Ack/Resolve buttons without inline handlers (CSP-safe).
    // Remove any prior handler before registering so re-renders don't stack listeners.
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

// ── Alert action helpers ──────────────────────────────────────
window._alertAck = async function(id) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/alerts/${encodeURIComponent(id)}/acknowledge`, { method: 'POST' });
        if (!resp.ok) console.error('Acknowledge failed', resp.status);
    } catch (e) { console.error('Acknowledge error', e); }
    window.PgAlertsView();
};

window._alertResolve = async function(id) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/alerts/${encodeURIComponent(id)}/resolve`, { method: 'POST' });
        if (!resp.ok) console.error('Resolve failed', resp.status);
    } catch (e) { console.error('Resolve error', e); }
    window.PgAlertsView();
};
