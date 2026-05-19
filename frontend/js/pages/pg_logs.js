/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL log viewer and analysis.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgLogsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'postgres'};
    const dbName = window.appState.currentDatabase || 'all';

    // 1. Initial Shell
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_logs.html', { inst, dbName });
    
    // 2. Initial Fetch
    await initPgLogs(inst.name);

    // 3. Set Refresh Interval
    if (window.pgLogsInterval) clearInterval(window.pgLogsInterval);
    window.pgLogsInterval = window.registerInterval(() => {
        if (window.appState.activeViewId === 'pg-logs') {
            initPgLogs(inst.name);
        } else {
            clearInterval(window.pgLogsInterval);
        }
    }, 60000); // 60s refresh
};

async function updatePgLogsHeader(instName) {
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
    } catch (e) { console.error("PG Logs header fetch failed:", e); }
}

async function initPgLogs(instName) {
    updatePgLogsHeader(instName);

    const els = {
        sev: document.getElementById('pgLogsSeverity'),
        limit: document.getElementById('pgLogsLimit'),
        search: document.getElementById('pgLogsSearch'),
        meta: document.getElementById('pgLogsMeta'),
        tbody: document.getElementById('pgLogsTbody'),
    };

    const fmtTs = (s) => {
        try { return new Date(s).toLocaleString(); } catch { return s || ''; }
    };
    const sevClass = (s) => {
        const v = (s || '').toLowerCase();
        if (v === 'panic' || v === 'fatal') return 'text-danger font-bold';
        if (v === 'error') return 'text-warning';
        if (v === 'warning') return 'text-muted';
        return 'text-muted';
    };
    const esc = (v) => window.escapeHtml(v || '');

    let cached = [];
    const applyFilter = () => {
        const q = (els.search?.value || '').trim().toLowerCase();
        const rows = q
            ? cached.filter((r) => {
                  const hay = [
                      r.severity, r.sqlstate, r.message,
                      r.database_name, r.user_name, r.application_name, r.client_addr,
                  ].join(' ').toLowerCase();
                  return hay.includes(q);
              })
            : cached;

        if (!rows.length) {
            if (els.tbody) els.tbody.innerHTML = `<tr><td colspan="6" class="text-center text-muted">No matching events</td></tr>`;
            return;
        }

        if (els.tbody) {
            els.tbody.innerHTML = rows.map((r) => {
                const dbUser = [r.database_name, r.user_name].filter(Boolean).join(' / ') || '-';
                const appClient = [r.application_name, r.client_addr].filter(Boolean).join(' / ') || '-';
                return `
                    <tr>
                        <td>${esc(fmtTs(r.capture_timestamp))}</td>
                        <td class="${sevClass(r.severity)}">${esc((r.severity || '').toUpperCase())}</td>
                        <td><code>${esc(r.sqlstate || '')}</code></td>
                        <td title="${esc(r.message || '')}">${esc(window.truncate ? window.truncate(r.message, 120) : r.message)}</td>
                        <td>${esc(dbUser)}</td>
                        <td>${esc(appClient)}</td>
                    </tr>
                `;
            }).join('');
        }
    };

    if (els.sev) els.sev.onchange = () => initPgLogs(instName);
    if (els.limit) els.limit.onchange = () => initPgLogs(instName);
    if (els.search) els.search.oninput = applyFilter;

    const severity = els.sev?.value || 'error';
    const limit = parseInt(els.limit?.value || '200', 10) || 200;

    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/logs/recent?instance=${encodeURIComponent(instName)}&severity=${encodeURIComponent(severity)}&limit=${encodeURIComponent(String(limit))}`
        );
        if (resp.ok) {
            const payload = await resp.json();
            cached = Array.isArray(payload?.events) ? payload.events : [];
            if (els.meta) els.meta.textContent = `${cached.length} event(s) • source: ${payload?.source || 'unknown'}`;
            applyFilter();
            
            // Update KPIs (mock/placeholder logic if specific stats API not yet available)
            const fatalCount = cached.filter(e => ['fatal','panic'].includes((e.severity||'').toLowerCase())).length;
            const errorCount = cached.filter(e => (e.severity||'').toLowerCase() === 'error').length;
            const warnCount = cached.filter(e => (e.severity||'').toLowerCase() === 'warning').length;
            
            if (document.getElementById('stat-logs-fatal')) document.getElementById('stat-logs-fatal').textContent = fatalCount;
            if (document.getElementById('stat-logs-errors')) document.getElementById('stat-logs-errors').textContent = errorCount;
            if (document.getElementById('stat-logs-warning')) document.getElementById('stat-logs-warning').textContent = warnCount;
        }
    } catch (e) {
        console.error("Failed to load logs:", e);
    }
}
