/*
 * SQL Optima — shared helpers for the Alerts & Events pages.
 */

window.fetchAlertsPageData = async function(inst) {
    const engine = (inst.engine || inst.type || 'postgres').toLowerCase().includes('sql') ? 'sqlserver' : 'postgres';
    const instName = inst.name || '';
    const out = {
        engine,
        engineAlerts: [],
        openCount: 0,
        blockingKpis: null,
        disconnected: false,
    };

    const qs = `instance=${encodeURIComponent(instName)}&engine=${encodeURIComponent(engine)}`;
    try {
        const [alertsResp, countResp, kpisResp] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/alerts?${qs}&limit=50`),
            window.apiClient.authenticatedFetch(`/api/alerts/count?instance=${encodeURIComponent(instName)}`),
            engine === 'sqlserver'
                ? window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/kpis?instance=${encodeURIComponent(instName)}`)
                : window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/kpis?instance=${encodeURIComponent(instName)}`),
        ]);

        if (alertsResp.status === 503 || countResp.status === 503) {
            out.disconnected = true;
            return out;
        }

        if (alertsResp.ok) {
            const body = await alertsResp.json();
            const list = body?.data?.list ?? body?.data?.alerts ?? body?.alerts ?? body?.list;
            out.engineAlerts = Array.isArray(list) ? list : [];
        }
        if (countResp.ok) {
            const body = await countResp.json();
            out.openCount = body?.data?.count ?? body?.count ?? 0;
        }
        if (out.engineAlerts.length > 0) {
            out.openCount = Math.max(out.openCount, out.engineAlerts.filter(a => a.status === 'open' || a.status === 'acknowledged').length);
        }
        if (kpisResp.ok) {
            const raw = await kpisResp.json();
            out.blockingKpis = raw.kpis || raw.data || raw;
        }
    } catch (e) {
        console.error('Alerts page fetch failed:', e);
    }
    return out;
};

window.renderAlertsKpiStrip = function(openCount, kpis, engine) {
    const blocked = kpis?.active_blocked_sessions ?? kpis?.blocked_sessions ?? 0;
    const incidents24h = kpis?.blocking_incidents_24h ?? kpis?.incidents_24h ?? 0;
    const deadlocks = kpis?.deadlocks_24h ?? 0;
    const locksRoute = engine === 'sqlserver' ? 'sqlserver-locks' : 'pg-locks';

    return `
        <div class="alerts-kpi-strip" style="display:grid; grid-template-columns:repeat(auto-fit, minmax(160px, 1fr)); gap:0.75rem; margin-bottom:1rem;">
            <div class="glass-panel alerts-kpi-card" style="padding:0.85rem 1rem; border-left:3px solid var(--danger);">
                <div class="text-muted" style="font-size:0.65rem; text-transform:uppercase; letter-spacing:0.04em;">Open Alerts</div>
                <div style="font-size:1.6rem; font-weight:700; color:${openCount > 0 ? 'var(--danger)' : 'var(--success)'};">${openCount}</div>
                <div style="font-size:0.7rem; color:var(--text-muted);">Alert engine (TimescaleDB)</div>
            </div>
            <div class="glass-panel alerts-kpi-card" style="padding:0.85rem 1rem; border-left:3px solid var(--warning);">
                <div class="text-muted" style="font-size:0.65rem; text-transform:uppercase;">Blocked Sessions</div>
                <div style="font-size:1.6rem; font-weight:700; color:${blocked > 0 ? 'var(--warning)' : 'var(--text)'};">${blocked}</div>
                <button class="btn btn-xs btn-outline mt-1" data-action="navigate" data-route="${locksRoute}" style="font-size:0.65rem;">Locks dashboard</button>
            </div>
            <div class="glass-panel alerts-kpi-card" style="padding:0.85rem 1rem; border-left:3px solid var(--accent);">
                <div class="text-muted" style="font-size:0.65rem; text-transform:uppercase;">Blocking Episodes (24h)</div>
                <div style="font-size:1.6rem; font-weight:700;">${incidents24h}</div>
                <div style="font-size:0.7rem; color:var(--text-muted);">Distinct snapshot windows</div>
            </div>
            <div class="glass-panel alerts-kpi-card" style="padding:0.85rem 1rem; border-left:3px solid #a855f7;">
                <div class="text-muted" style="font-size:0.65rem; text-transform:uppercase;">Deadlocks (24h)</div>
                <div style="font-size:1.6rem; font-weight:700;">${deadlocks}</div>
            </div>
        </div>
    `;
};

window.renderEngineAlertsTable = function(alerts, { showAllStatuses } = {}) {
    const severityBadge = (s) => {
        const cls = s === 'critical' ? 'danger' : s === 'warning' ? 'warning' : 'info';
        return `<span class="badge badge-${cls}">${window.escapeHtml(String(s || '').toUpperCase())}</span>`;
    };
    const statusBadge = (s) => {
        if (s === 'acknowledged') return '<span class="badge badge-warning">ACK</span>';
        if (s === 'resolved') return '<span class="badge badge-success">RESOLVED</span>';
        return '<span class="badge badge-danger">OPEN</span>';
    };

    const rows = (alerts || []).filter(a => showAllStatuses || a.status === 'open' || a.status === 'acknowledged');
    if (rows.length === 0) {
        return `<p class="text-muted" style="padding:1rem; text-align:center;">No alerts from the alert engine yet. Blocking is evaluated every minute from TimescaleDB incidents and live snapshots.</p>`;
    }

    return `
        <div class="table-container-compact" style="max-height:320px; overflow-y:auto;">
            <table class="modern-table modern-table-compact">
                <thead>
                    <tr>
                        <th>Status</th><th>Severity</th><th>Category</th><th>Title</th><th>Hits</th><th>Last Seen</th><th></th>
                    </tr>
                </thead>
                <tbody>
                    ${rows.map(a => `
                        <tr>
                            <td>${statusBadge(a.status)}</td>
                            <td>${severityBadge(a.severity)}</td>
                            <td>${window.escapeHtml(a.category || '')}</td>
                            <td>
                                <strong>${window.escapeHtml(a.title || '')}</strong>
                                <div class="text-muted small">${window.escapeHtml(String(a.description || '').substring(0, 120))}</div>
                            </td>
                            <td class="text-center">${a.hit_count || 1}</td>
                            <td class="small text-muted">${a.last_seen_at ? new Date(a.last_seen_at).toLocaleString() : '--'}</td>
                            <td style="white-space:nowrap;">
                                ${a.status === 'open' ? `
                                    <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="ack">Ack</button>
                                    <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="resolve">Resolve</button>
                                ` : a.status === 'acknowledged' ? `
                                    <button class="btn btn-xs btn-outline" data-alert-id="${window.escapeHtml(String(a.id))}" data-alert-action="resolve">Resolve</button>
                                ` : ''}
                            </td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        </div>
    `;
};

window.bindAlertsPageActions = function(refreshFn) {
    if (window._alertsPageActionHandler) {
        window.routerOutlet.removeEventListener('click', window._alertsPageActionHandler);
    }
    window._alertsPageActionHandler = function onAlertAction(e) {
        const btn = e.target.closest('[data-alert-action]');
        if (!btn) return;
        const id = btn.dataset.alertId;
        const action = btn.dataset.alertAction;
        if (action === 'ack') window._alertAck(id, refreshFn);
        else if (action === 'resolve') window._alertResolve(id, refreshFn);
    };
    window.routerOutlet.addEventListener('click', window._alertsPageActionHandler);
};

window._alertAck = async function(id, refreshFn) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/alerts/${encodeURIComponent(id)}/acknowledge`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ reason: '' }),
        });
        if (resp.ok && typeof refreshFn === 'function') refreshFn();
    } catch (e) { console.error('Ack failed', e); }
};

window._alertResolve = async function(id, refreshFn) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/alerts/${encodeURIComponent(id)}/resolve`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ reason: '' }),
        });
        if (resp.ok && typeof refreshFn === 'function') refreshFn();
    } catch (e) { console.error('Resolve failed', e); }
};
