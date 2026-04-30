/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL backup status and history page.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgBackupsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'postgres'};
    const dbName = window.appState.currentDatabase || 'all';

    // 1. Initial Shell
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_backups.html', { inst, dbName });
    
    // 2. Initial Fetch
    await initPgBackups(inst.name);

    // 3. Set Refresh Interval
    if (window.pgBackupsInterval) clearInterval(window.pgBackupsInterval);
    window.pgBackupsInterval = setInterval(() => {
        if (window.appState.activeViewId === 'pg-backups') {
            initPgBackups(inst.name);
        } else {
            clearInterval(window.pgBackupsInterval);
        }
    }, 300000); // 5m refresh
};

async function updatePgBackupsHeader(instName) {
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
    } catch (e) { console.error("PG Backups header fetch failed:", e); }
}

async function initPgBackups(instName) {
    updatePgBackupsHeader(instName);

    const esc = (v) => window.escapeHtml(v || '');
    const fmtTs = (s) => {
        try { return new Date(s).toLocaleString(); } catch { return s || ''; }
    };
    const fmtBytes = (n) => {
        const v = Number(n || 0);
        if (!isFinite(v) || v <= 0) return '-';
        const units = ['B','KB','MB','GB','TB'];
        let x = v, i = 0;
        while (x >= 1024 && i < units.length - 1) { x /= 1024; i++; }
        return `${x.toFixed(i >= 2 ? 2 : 0)} ${units[i]}`;
    };
    const statusClass = (s) => {
        const v = (s || '').toLowerCase();
        if (v === 'success') return 'text-success';
        if (v === 'failed') return 'text-danger';
        if (v === 'partial') return 'text-warning';
        return 'text-muted';
    };

    const els = {
        limit: document.getElementById('pgBackupLimit'),
        search: document.getElementById('pgBackupSearch'),
        meta: document.getElementById('pgBackupMeta'),
        tbody: document.getElementById('pgBackupTbody'),
    };

    let cached = [];
    const applyFilter = () => {
        const q = (els.search.value || '').trim().toLowerCase();
        const rows = q
            ? cached.filter((r) => {
                  const hay = [r.tool, r.backup_type, r.status, r.error_message].join(' ').toLowerCase();
                  return hay.includes(q);
              })
            : cached;
        if (!rows.length) {
            els.tbody.innerHTML = `<tr><td colspan="7" class="text-center text-muted">No matching rows</td></tr>`;
            return;
        }
        els.tbody.innerHTML = rows.map((r) => {
            return `
                <tr>
                    <td>${esc(fmtTs(r.started_at))}</td>
                    <td>${esc(fmtTs(r.finished_at))}</td>
                    <td class="${statusClass(r.status)}">${esc((r.status || '').toUpperCase())}</td>
                    <td>${esc(r.tool || '-')}</td>
                    <td>${esc(r.backup_type || '-')}</td>
                    <td>${esc(fmtBytes(r.size_bytes))}</td>
                    <td>${esc(r.error_message || '')}</td>
                </tr>
            `;
        }).join('');
    };

    if (els.limit) els.limit.onchange = () => initPgBackups(instName);
    if (els.search) els.search.oninput = applyFilter;

    const limit = parseInt(els.limit?.value || '100', 10) || 100;
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/backups/history?instance=${encodeURIComponent(instName)}&limit=${encodeURIComponent(String(limit))}`
        );
        if (resp.ok) {
            const payload = await resp.json();
            cached = Array.isArray(payload?.runs) ? payload.runs : (Array.isArray(payload) ? payload : []);
            if (els.meta) els.meta.textContent = `${cached.length} run(s)`;
            applyFilter();
            
            // Update KPIs
            if (cached.length > 0) {
                const latest = cached[0];
                const lastSuccess = cached.find(r => (r.status || '').toLowerCase() === 'success');
                
                if (document.getElementById('stat-last-backup')) document.getElementById('stat-last-backup').textContent = lastSuccess ? new Date(lastSuccess.finished_at).toLocaleDateString() : 'Never';
                if (document.getElementById('stat-backup-status')) {
                    const status = (latest.status || 'unknown').toUpperCase();
                    const el = document.getElementById('stat-backup-status');
                    el.textContent = status;
                    el.className = 'strip-metric-value ' + statusClass(latest.status);
                }
                if (document.getElementById('stat-backup-size')) document.getElementById('stat-backup-size').textContent = fmtBytes(latest.size_bytes);
                
                const dayAgo = new Date(Date.now() - 86400000);
                const failures = cached.filter(r => new Date(r.started_at) > dayAgo && (r.status || '').toLowerCase() === 'failed').length;
                if (document.getElementById('stat-backup-failures')) {
                    const el = document.getElementById('stat-backup-failures');
                    el.textContent = failures;
                    el.className = 'strip-metric-value ' + (failures > 0 ? 'text-danger font-bold' : 'text-success');
                }
            }
        }
    } catch (e) {
        console.error("Failed to load backups:", e);
    }
}
