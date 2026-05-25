/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL Locks & Blocking dashboard — hierarchical tree, incident drilldown,
 *          session control (cancel/terminate), chronic bottleneck profiling.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgLocksView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
    const dbName = window.appState.currentDatabase || 'all';

    window.appState.activeViewId = 'pg-locks';

    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_locks.html', { inst, dbName });

    window.initPageTimePicker();
    await initPgLocks();

    if (window.pgLocksInterval) clearInterval(window.pgLocksInterval);
    window.pgLocksInterval = window.registerInterval(() => {
        if (window.appState.activeViewId === 'pg-locks') {
            initPgLocks();
        } else {
            clearInterval(window.pgLocksInterval);
        }
    }, 30000);
};

// ── Utilities ─────────────────────────────────────────────────────────────────

function pgLocksFormatLocal(dt) {
    const pad = n => String(n).padStart(2, '0');
    return `${dt.getFullYear()}-${pad(dt.getMonth()+1)}-${pad(dt.getDate())}T${pad(dt.getHours())}:${pad(dt.getMinutes())}:${pad(dt.getSeconds())}`;
}

function pgLocksToRFC3339(v) {
    const ms = Date.parse(v);
    if (!isFinite(ms)) return '';
    return new Date(ms).toISOString();
}

function formatDuration(totalSeconds) {
    const s = Number(totalSeconds || 0);
    if (!isFinite(s) || s <= 0) return '0s';
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = Math.floor(s % 60);
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${sec}s`;
    return `${sec}s`;
}

function pgShowToast(msg, type = 'info') {
    const existing = document.getElementById('pgToastBanner');
    if (existing) existing.remove();
    const div = document.createElement('div');
    div.id = 'pgToastBanner';
    const colors = { success: 'var(--success)', danger: 'var(--danger)', info: 'var(--accent-blue)', warning: 'var(--warning)' };
    div.style.cssText = `position:fixed; bottom:1.5rem; right:1.5rem; z-index:9999; background:var(--bg-secondary);
        border-left:4px solid ${colors[type]||colors.info}; border-radius:6px; padding:0.65rem 1rem;
        box-shadow:0 4px 16px rgba(0,0,0,0.4); font-size:0.82rem; max-width:380px;`;
    div.textContent = msg;
    document.body.appendChild(div);
    setTimeout(() => div.remove(), 4000);
}

// ── Header ────────────────────────────────────────────────────────────────────

async function updatePgLocksHeader(instName) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/server-info?instance=${encodeURIComponent(instName)}`);
        if (!resp.ok) return;
        const s = await resp.json();
        if (document.getElementById('pg-uptime')) document.getElementById('pg-uptime').textContent = 'Uptime: ' + (s.uptime || 'N/A');
        if (document.getElementById('pg-version')) document.getElementById('pg-version').textContent = (s.version || '').split(',')[0];
        if (document.getElementById('pgLastRefreshTime')) document.getElementById('pgLastRefreshTime').textContent = new Date().toLocaleTimeString();
        const hs = s.health_score || 0;
        const healthColor = hs > 80 ? 'success' : hs > 60 ? 'warning' : 'danger';
        const hBadge = document.getElementById('pgHealthScoreBadge');
        if (hBadge) { hBadge.textContent = hs; hBadge.className = `badge badge-${healthColor}`; }
    } catch (e) { console.error('[PgLocks] header fetch failed:', e); }
}

// ── Main init ─────────────────────────────────────────────────────────────────

async function initPgLocks() {
    console.log('[PgLocks] Initializing...');
    window.currentCharts = window.currentCharts || {};
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: '' };
    const instQ = encodeURIComponent(inst.name);

    updatePgLocksHeader(inst.name);

    // Range UI defaults
    const fromEl = document.getElementById('pgLocksFrom');
    const toEl   = document.getElementById('pgLocksTo');
    const now = new Date();
    const hourAgo = new Date(now.getTime() - 3600000);
    if (fromEl && !fromEl.value) fromEl.value = pgLocksFormatLocal(hourAgo);
    if (toEl   && !toEl.value)   toEl.value   = pgLocksFormatLocal(now);

    const applyBtn = document.getElementById('pgLocksApplyRange');
    if (applyBtn && !applyBtn.dataset.pgBound) {
        applyBtn.dataset.pgBound = '1';
        applyBtn.onclick = () => loadPgLocksRangeData().catch(e => console.error('[PgLocks] range load failed:', e));
    }
    function updateRangeHint() {
        const hint = document.getElementById('pgLocksRangeHint');
        if (!hint || !fromEl || !toEl) return;
        const err = typeof window.getDatetimeLocalRangeError === 'function'
            ? window.getDatetimeLocalRangeError(fromEl.value, toEl.value) : '';
        hint.textContent = err;
        hint.style.display = err ? 'block' : 'none';
    }
    if (fromEl && toEl && !fromEl.dataset.pgRangeListeners) {
        fromEl.dataset.pgRangeListeners = '1';
        fromEl.addEventListener('change', updateRangeHint);
        toEl.addEventListener('change', updateRangeHint);
    }

    // Fetch current locks
    let locks = [];
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/locks?instance=${instQ}`);
        if (resp.ok && (resp.headers.get('content-type')||'').includes('application/json')) {
            const d = await resp.json();
            locks = (d && d.locks) ? d.locks : [];
        }
    } catch (e) { console.error('[PgLocks] locks fetch failed:', e); }

    const tbody = document.getElementById('locksTbody');
    if (tbody) {
        if (!locks.length) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No locks detected. All clear!</td></tr>';
        } else {
            tbody.innerHTML = locks.map(lock => `
                <tr>
                    <td>${lock.pid||'-'}</td>
                    <td>${window.escapeHtml(lock.lock_type||'-')}</td>
                    <td>${window.escapeHtml(lock.relation||'-')}</td>
                    <td>${getLockModeBadge(lock.mode)}</td>
                    <td>${lock.granted ? '<span class="text-success font-bold">true</span>' : '<span class="text-danger font-bold">false</span>'}</td>
                    <td>${lock.waiting_for||'-'}</td>
                </tr>
            `).join('');
        }
    }

    // KPIs
    let kpis = null;
    try {
        const kRes = await window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/kpis?instance=${instQ}`);
        if (kRes.ok && (kRes.headers.get('content-type')||'').includes('application/json')) {
            const d = await kRes.json();
            kpis = d?.kpis ?? null;
        }
    } catch (e) { console.error('[PgLocks] KPI fetch failed:', e); }
    updatePgLocksKpis(kpis);

    // Range-driven data load
    async function loadPgLocksRangeData() {
        if (fromEl && toEl && fromEl.value && toEl.value) {
            const err = typeof window.getDatetimeLocalRangeError === 'function'
                ? window.getDatetimeLocalRangeError(fromEl.value, toEl.value) : '';
            if (err) {
                const hint = document.getElementById('pgLocksRangeHint');
                if (hint) { hint.textContent = err; hint.style.display = 'block'; }
                return;
            }
        }
        let fromISO = fromEl?.value ? pgLocksToRFC3339(fromEl.value) : '';
        let toISO   = toEl?.value   ? pgLocksToRFC3339(toEl.value)   : '';
        if (!fromISO || !toISO) {
            const n = new Date();
            if (!toISO)   { toISO   = n.toISOString(); if (toEl)   toEl.value   = pgLocksFormatLocal(n); }
            if (!fromISO) { const f = new Date(n - 3600000); fromISO = f.toISOString(); if (fromEl) fromEl.value = pgLocksFormatLocal(f); }
        }
        const meta = document.getElementById('pgLocksRangeMeta');
        if (meta) meta.textContent = `${new Date(fromISO).toLocaleString()} → ${new Date(toISO).toLocaleString()}`;

        let timelinePoints=[], incidentWindows=[], topLockedTables=[], detailsTree=[], detailsCapturedAt='';
        try {
            const [tRes, topRes, dRes] = await Promise.all([
                window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/timeline?instance=${instQ}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`),
                window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/top-locked-tables?instance=${instQ}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}&limit=10`),
                window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/details?instance=${instQ}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`),
            ]);
            if (tRes.ok && (tRes.headers.get('content-type')||'').includes('application/json')) {
                const d = await tRes.json();
                timelinePoints  = (d && Array.isArray(d.timeline))   ? d.timeline   : [];
                incidentWindows = (d && Array.isArray(d.incidents))  ? d.incidents  : [];
            }
            if (topRes.ok && (topRes.headers.get('content-type')||'').includes('application/json')) {
                const d = await topRes.json();
                topLockedTables = (d && Array.isArray(d.tables)) ? d.tables : [];
            }
            if (dRes.ok && (dRes.headers.get('content-type')||'').includes('application/json')) {
                const d = await dRes.json();
                detailsTree      = (d && Array.isArray(d.blocking_tree)) ? d.blocking_tree : [];
                detailsCapturedAt = (d && d.collected_at) ? String(d.collected_at) : '';
            }
        } catch (e) { console.error('[PgLocks] range fetch failed:', e); }

        renderPgBlockingIncidentTimeline(timelinePoints, incidentWindows);
        renderPgTopLockedTables(topLockedTables);
        renderBlockingTree(detailsTree);
        const capEl = document.getElementById('pgBlockingDetailsCapturedAt');
        if (capEl) capEl.textContent = detailsCapturedAt ? 'Snapshot: ' + new Date(detailsCapturedAt).toLocaleString() : 'Snapshot: —';
        renderBlockingDetails(detailsTree, detailsCapturedAt);
        renderBlockingSummary(detailsTree);

        // Load incident history (pass from/to for exact window matching)
        loadIncidentHistory(instQ, fromISO, toISO);
        // Load chronic blockers
        loadChronicBlockers(instQ, fromISO, toISO);
    }

    window.__loadPgLocksRangeData = loadPgLocksRangeData;
    await loadPgLocksRangeData();

    // Deadlock history chart
    let deadlockHist = null;
    try {
        const dlRes = await window.apiClient.authenticatedFetch(`/api/postgres/deadlocks/history?instance=${instQ}&window_minutes=180&limit=400`);
        if (dlRes.ok && (dlRes.headers.get('content-type')||'').includes('application/json')) {
            const d = await dlRes.json();
            deadlockHist = d?.history ?? null;
        }
    } catch (e) { console.error('[PgLocks] deadlock history failed:', e); }

    let lockWaitHist = null;
    try {
        const lwRes = await window.apiClient.authenticatedFetch(`/api/postgres/locks/wait-history?instance=${instQ}&window_minutes=180&limit=400`);
        if (lwRes.ok && (lwRes.headers.get('content-type')||'').includes('application/json')) {
            const d = await lwRes.json();
            lockWaitHist = d?.history ?? null;
        }
    } catch (e) { console.error('[PgLocks] lock-wait history failed:', e); }

    // Lock wait trend chart
    const lwCanvas = document.getElementById('pgLockWaitTrendChart');
    if (lwCanvas) {
        const lwLabels = Array.isArray(lockWaitHist?.labels) ? lockWaitHist.labels : [];
        const lwCounts = Array.isArray(lockWaitHist?.lock_waiting_sessions) ? lockWaitHist.lock_waiting_sessions : [];
        if (!lwLabels.length) {
            const card = lwCanvas.closest('.chart-card') || lwCanvas.parentElement;
            if (card) card.style.display = 'none';
        } else {
            const n = Math.min(60, lwLabels.length);
            if (window.currentCharts.pgLockWaitTrend) window.currentCharts.pgLockWaitTrend.destroy();
            window.currentCharts.pgLockWaitTrend = new Chart(lwCanvas.getContext('2d'), {
                type: 'line',
                data: { labels: lwLabels.slice(-n).map(s => { try { return new Date(s).toLocaleTimeString(); } catch { return ''; } }),
                        datasets: [{ label:'Sessions in Lock wait', data: lwCounts.slice(-n).map(v => Number(v||0)),
                            borderColor: window.getCSSVar('--warning'), backgroundColor:'rgba(234,179,8,0.12)', fill:true, tension:0.2 }] },
                options: { responsive:true, maintainAspectRatio:false, scales:{ y:{ beginAtZero:true } } }
            });
        }
    }

    // Deadlocks bar chart
    const dlc = document.getElementById('pgDeadlocksChart');
    if (dlc) {
        const labels = Array.isArray(deadlockHist?.labels) ? deadlockHist.labels : [];
        const deltas = Array.isArray(deadlockHist?.deadlocks_delta) ? deadlockHist.deadlocks_delta : [];
        if (!labels.length) {
            const card = dlc.closest('.chart-card') || dlc.parentElement;
            if (card) card.style.display = 'none';
        } else {
            if (window.currentCharts.pgDlck) window.currentCharts.pgDlck.destroy();
            window.currentCharts.pgDlck = new Chart(dlc.getContext('2d'), {
                type: 'bar',
                data: { labels: labels.slice(-30).map(s => { try { return new Date(s).toLocaleTimeString(); } catch { return ''; } }),
                        datasets: [{ label:'Deadlocks', data: deltas.slice(-30).map(v => Number(v||0)), backgroundColor: window.getCSSVar('--danger') }] },
                options: { responsive:true, maintainAspectRatio:false, scales:{ y:{ beginAtZero:true } } }
            });
        }
    }

    // Lock distribution doughnut
    const ldistCtx = document.getElementById('pgLockDistChart');
    if (ldistCtx) {
        const lockModes = {};
        locks.forEach(l => { const m = l.mode || 'unknown'; lockModes[m] = (lockModes[m]||0)+1; });
        const modeLabels = Object.keys(lockModes).length > 0 ? Object.keys(lockModes) : ['None'];
        const modeData   = modeLabels.length > 1 ? Object.values(lockModes) : [0];
        if (window.currentCharts.pgLockDist) window.currentCharts.pgLockDist.destroy();
        window.currentCharts.pgLockDist = new Chart(ldistCtx.getContext('2d'), {
            type: 'doughnut',
            data: { labels: modeLabels, datasets: [{ data: modeData,
                backgroundColor:[window.getCSSVar('--danger'), window.getCSSVar('--warning'), window.getCSSVar('--success'), window.getCSSVar('--accent-blue')], borderWidth:0 }] },
            options: { responsive:true, maintainAspectRatio:false, cutout:'60%', plugins:{ legend:{ position:'bottom' } } }
        });
    }
}

// ── KPI panel ─────────────────────────────────────────────────────────────────

function updatePgLocksKpis(kpis) {
    const elVictims  = document.getElementById('pgKpiActiveVictims');
    const elMeta     = document.getElementById('pgKpiIncidentMeta');
    const elRootPid  = document.getElementById('pgKpiRootPid');
    const elRootQ    = document.getElementById('pgKpiRootQuery');
    const elIdle     = document.getElementById('pgKpiIdleRisk');
    const elSev      = document.getElementById('pgKpiSeverity');
    const elBand     = document.getElementById('pgKpiSeverityBand');
    if (!elVictims || !elMeta || !elRootPid || !elRootQ || !elIdle || !elSev || !elBand) return;

    const victims  = Number(kpis?.active_blocking_sessions ?? 0) || 0;
    const depth    = Number(kpis?.chain_depth ?? 0) || 0;
    const idleRisk = Number(kpis?.idle_in_txn_risk_count ?? 0) || 0;
    const durMins  = Number(kpis?.incident_duration_mins ?? 0) || 0;
    const rootPid  = kpis?.root_blocker_pid != null ? String(kpis.root_blocker_pid) : '—';
    const rootQ    = String(kpis?.root_blocker_query ?? '').trim();

    elVictims.textContent = String(victims);
    elVictims.classList.toggle('text-danger', victims > 0);
    elVictims.classList.toggle('text-success', victims === 0);

    const incId = kpis?.incident_id != null ? String(kpis.incident_id) : '';
    elMeta.textContent = incId ? `incident #${incId} · ${durMins}m · depth ${depth}` : 'no active incident';

    elRootPid.textContent = rootPid;
    elRootQ.textContent   = rootQ ? (rootQ.length > 110 ? rootQ.slice(0,110)+'…' : rootQ) : '—';

    elIdle.textContent = String(idleRisk);
    elIdle.classList.toggle('text-warning', idleRisk > 0);

    const score = (victims*10) + (depth*5) + (idleRisk*30) + (durMins*2);
    elSev.textContent = String(score);
    const band = score >= 80 ? 'RED' : score >= 50 ? 'ORANGE' : score >= 20 ? 'YELLOW' : 'GREEN';
    elBand.textContent = band;
    elBand.classList.remove('text-success','text-warning','text-danger');
    if (band === 'GREEN') elBand.classList.add('text-success');
    else if (band === 'YELLOW') elBand.classList.add('text-warning');
    else elBand.classList.add('text-danger');

    const banner = document.getElementById('pgLocksAlertBanner');
    const title  = document.getElementById('pgLocksAlertTitle');
    const msg    = document.getElementById('pgLocksAlertMsg');
    if (banner && title && msg) {
        const alarming = victims > 0 || score >= 50;
        banner.style.display = alarming ? 'block' : 'none';
        if (alarming) {
            title.textContent = victims > 0 ? 'Blocking detected' : 'Incident severity elevated';
            const root = kpis?.root_blocker_pid != null ? `root PID ${kpis.root_blocker_pid}` : 'root unknown';
            msg.textContent = `${victims} blocked · score ${score} (${band})${incId ? ` · incident #${incId}` : ''} · ${root}`;
            banner.classList.remove('alert-warning','alert-danger');
            banner.classList.add(score >= 80 || victims > 0 ? 'alert-danger' : 'alert-warning');
        }
    }
}

// ── Blocking Tree (visual redesign) ───────────────────────────────────────────

function renderBlockingTree(tree) {
    const container = document.getElementById('blockingTreeList');
    if (!container) return;
    if (!tree || !tree.length) {
        container.innerHTML = '<div class="text-success" style="font-size:0.85rem;"><i class="fa-solid fa-check-circle"></i> No blocking sessions detected</div>';
        return;
    }

    // Clear incident breadcrumb
    const bc = document.getElementById('pgBlockingBreadcrumb');
    if (bc) bc.style.display = 'none';

    container.innerHTML = tree.map(root => renderTreeNode(root, 0, true)).join('');

    // Delegate all button clicks on the tree (avoids inline onclick for CSP compliance)
    if (!container.dataset.pgSqlBound) {
        container.dataset.pgSqlBound = '1';
        container.addEventListener('click', (e) => {
            const btn = e.target?.closest?.('[data-action], .pg-sql-btn');
            if (!btn) return;
            const action = btn.dataset.action;
            if (!action && btn.classList.contains('pg-sql-btn')) {
                window.pgShowSqlModal(btn.dataset.pid || '', btn.dataset.user || '', btn.dataset.sql || '');
            } else if (action === 'cancel-backend') {
                window.pgCancelBackend(btn.dataset.pid);
            } else if (action === 'terminate-backend') {
                window.pgTerminateBackend(btn.dataset.pid);
            }
        });
    }
}

function renderTreeNode(node, depth, isRoot) {
    const pid        = node.pid ?? '?';
    const state      = window.escapeHtml(node.state || 'unknown');
    const dur        = window.escapeHtml(node.duration || '—');
    const wet        = node.wait_event_type || '';
    const we         = node.wait_event     || '';
    const wDisplay   = window.escapeHtml(we || wet || '—');
    const app        = window.escapeHtml(node.application_name || '');
    const addr       = window.escapeHtml(node.client_addr      || '');
    const user       = window.escapeHtml(node.user             || '');
    const db         = window.escapeHtml(node.database         || '');
    const idleInTxn  = node.is_idle_in_txn === true || (node.state||'').toLowerCase().includes('idle in transaction');
    const relations  = Array.isArray(node.affected_relations) ? node.affected_relations.filter(r => r && r !== 'virtual') : [];
    const modes      = Array.isArray(node.lock_modes) ? node.lock_modes : [];
    const queryFull  = node.query || '';
    const children   = Array.isArray(node.blocked_by) ? node.blocked_by : [];

    let badge, borderCssVar;
    if (isRoot && children.length > 0) {
        badge = '<span class="badge badge-danger" style="font-size:0.6rem;">ROOT BLOCKER</span>';
        borderCssVar = '--danger';
    } else if (!isRoot && children.length > 0) {
        badge = '<span class="badge badge-warning" style="font-size:0.6rem;">BLOCKER + VICTIM</span>';
        borderCssVar = '--warning';
    } else if (isRoot && children.length === 0) {
        badge = '';
        borderCssVar = '--warning';
    } else {
        badge = '<span class="badge badge-info" style="font-size:0.6rem;">WAITING</span>';
        borderCssVar = '--accent-blue';
    }

    const idlePill     = idleInTxn ? '<span class="badge badge-danger" style="font-size:0.6rem; margin-left:0.3rem;">IDLE IN TXN</span>' : '';
    const idleStyle    = idleInTxn ? 'border-left:3px solid var(--danger); padding-left:0.4rem;' : '';
    const indentRem    = depth * 1.6;
    const safeQuery    = window.escapeHtml(queryFull);
    const safeQueryAttr= queryFull.replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/</g,'&lt;').replace(/>/g,'&gt;');

    const relHtml = relations.length
        ? `<div style="font-size:0.72rem; color:var(--text-muted); margin-top:0.15rem;">Relation(s): ${relations.map(r=>`<code>${window.escapeHtml(r)}</code>`).join(', ')}</div>`
        : '';
    const modeHtml = modes.length
        ? `<div style="font-size:0.72rem; color:var(--text-muted); margin-top:0.15rem;">Lock mode(s): ${modes.map(m=>`<code>${window.escapeHtml(m)}</code>`).join(', ')}</div>`
        : '';

    const metaLine = [app ? `App: <code>${app}</code>` : '', addr ? `IP: <code>${addr}</code>` : '', db ? `DB: <code>${db}</code>` : '']
        .filter(Boolean).join(' · ');

    const safeUser = user; // already html-escaped via const user = window.escapeHtml(...)
    let html = `
    <div style="margin-left:${indentRem}rem; margin-bottom:0.55rem;">
      <div class="glass-panel" style="padding:0.5rem 0.7rem; border-left:3px solid var(${borderCssVar}); ${idleStyle}">
        <div style="display:flex; justify-content:space-between; align-items:flex-start; gap:0.5rem; flex-wrap:wrap;">
          <div>
            <span style="font-weight:700; font-size:0.85rem;">PID ${pid}</span>
            ${badge}${idlePill}
            <span class="text-muted" style="font-size:0.75rem; margin-left:0.4rem;">${state} · ${dur}</span>
          </div>
          <div style="display:flex; gap:0.3rem; flex-shrink:0; flex-wrap:wrap;">
            <button class="btn btn-xs btn-outline pg-sql-btn" style="font-size:0.68rem; padding:0.1rem 0.4rem;"
                    data-pid="${pid}" data-user="${safeUser}" data-sql="${safeQueryAttr}">
              SQL
            </button>
            <button class="btn btn-xs btn-outline text-warning" style="font-size:0.68rem; padding:0.1rem 0.4rem;"
                    data-action="cancel-backend" data-pid="${pid}">Cancel</button>
            <button class="btn btn-xs btn-outline text-danger" style="font-size:0.68rem; padding:0.1rem 0.4rem;"
                    data-action="terminate-backend" data-pid="${pid}">Kill</button>
          </div>
        </div>
        ${metaLine ? `<div style="font-size:0.74rem; margin-top:0.2rem;">${metaLine}</div>` : ''}
        <div style="font-size:0.72rem; color:var(--text-muted); margin-top:0.15rem;">Wait: <code>${wDisplay}</code></div>
        ${relHtml}${modeHtml}
      </div>
      ${children.length ? `
        <div style="margin-left:1.2rem; padding-left:0.6rem; border-left:2px dashed var(--border-color); margin-top:0.3rem;">
          ${children.map(ch => renderTreeNode(ch, depth+1, false)).join('')}
        </div>` : ''}
    </div>`;
    return html;
}

// ── Session control ───────────────────────────────────────────────────────────

window.pgCancelBackend = async function(pid) {
    if (!confirm(`Cancel backend PID ${pid}?\nThe session's current query will be interrupted but the connection remains.`)) return;
    await _pgSessionControl(pid, 'cancel');
};

window.pgTerminateBackend = async function(pid) {
    if (!confirm(`Terminate backend PID ${pid}?\nThe session will be fully disconnected.`)) return;
    await _pgSessionControl(pid, 'terminate');
};

async function _pgSessionControl(pid, action) {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx]?.name;
    if (!inst) { pgShowToast('No instance selected.', 'danger'); return; }
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/session/${action}`,
            { method: 'POST', headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ instance: inst, pid }) }
        );
        const data = await resp.json();
        if (!resp.ok || !data.ok) {
            pgShowToast(`${action} failed: ${data.error || 'unknown error'}`, 'danger');
            return;
        }
        const done = data[`${action}d`];
        pgShowToast(done ? `PID ${pid} ${action}d successfully.` : `PID ${pid} was already gone.`, done ? 'success' : 'info');
        setTimeout(() => window.__loadPgLocksRangeData?.(), 1200);
    } catch (e) {
        pgShowToast(`${action} request failed: ${e.message}`, 'danger');
    }
}

// ── Incident history panel ────────────────────────────────────────────────────

// Module-level store for incidents (avoids JSON-in-onclick attribute escaping issues)
window._pgIncidentStore = [];

async function loadIncidentHistory(instQ, fromISO, toISO) {
    const tbody = document.getElementById('pgIncidentTbody');
    const cntEl = document.getElementById('pgIncidentCount');
    if (!tbody) return;
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/locks-blocking/incidents?instance=${instQ}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}&limit=50`
        );
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">Error loading incidents</td></tr>'; return; }
        const d = await resp.json();
        const incidents = Array.isArray(d.incidents) ? d.incidents : [];
        window._pgIncidentStore = incidents;
        if (cntEl) cntEl.textContent = `${incidents.length} incident(s)`;
        if (!incidents.length) {
            tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">No blocking incidents in this window</td></tr>';
            return;
        }
        tbody.innerHTML = incidents.map((inc, idx) => {
            const started    = inc.started_at ? new Date(inc.started_at).toLocaleString() : '—';
            const dur        = formatDuration(Math.round(inc.duration_seconds || 0));
            const victims    = inc.peak_blocked_sessions || 0;
            const rootPid    = inc.root_blocker_pid != null ? inc.root_blocker_pid : '—';
            const status     = (inc.status || '').toLowerCase();
            const statusBadge = status === 'active'
                ? '<span class="badge badge-danger" style="font-size:0.6rem;">active</span>'
                : '<span class="badge badge-info" style="font-size:0.6rem;">resolved</span>';
            const incId      = inc.incident_id || idx + 1;
            return `<tr>
                <td>${incId}</td>
                <td class="text-muted">${started}</td>
                <td>${dur}</td>
                <td><strong>${victims}</strong></td>
                <td>${rootPid}</td>
                <td>${statusBadge}</td>
                <td>
                    <button class="btn btn-xs btn-outline text-accent pg-drilldown-btn"
                            style="font-size:0.65rem; padding:0.1rem 0.35rem;"
                            data-inc-idx="${idx}">View Tree</button>
                </td>
            </tr>`;
        }).join('');

        // Delegate incident drilldown clicks (avoids JSON-in-onclick escaping)
        tbody.removeEventListener('click', _pgIncidentClickHandler);
        tbody.addEventListener('click', _pgIncidentClickHandler);
    } catch (e) {
        console.error('[PgLocks] incident list failed:', e);
        if (tbody) tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">Error loading incidents</td></tr>';
    }
}

function _pgIncidentClickHandler(e) {
    const btn = e.target?.closest?.('.pg-drilldown-btn');
    if (!btn) return;
    const idx = parseInt(btn.dataset.incIdx, 10);
    const inc = window._pgIncidentStore[idx];
    if (inc) window.pgDrilldownIncident(inc);
}

window.pgDrilldownIncident = async function(inc) {
    const startDate = inc.started_at ? new Date(inc.started_at) : null;
    const endDate   = inc.ended_at   ? new Date(inc.ended_at)   : null;
    if (!startDate || isNaN(startDate.getTime())) return;

    const fromT = new Date(startDate.getTime() - 30000).toISOString();
    const toT   = (endDate && !isNaN(endDate.getTime()))
                    ? new Date(endDate.getTime() + 30000).toISOString()
                    : new Date(startDate.getTime() + 600000).toISOString();

    const inst = window.appState.config.instances[window.appState.currentInstanceIdx]?.name;
    if (!inst) return;
    const instQ = encodeURIComponent(inst);

    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/locks-blocking/details?instance=${instQ}&from=${encodeURIComponent(fromT)}&to=${encodeURIComponent(toT)}`
        );
        if (!resp.ok) return;
        const d = await resp.json();
        const tree = Array.isArray(d.blocking_tree) ? d.blocking_tree : [];

        renderBlockingTree(tree);

        const bc = document.getElementById('pgBlockingBreadcrumb');
        if (bc) {
            const started = inc.started_at ? new Date(inc.started_at).toLocaleString() : '?';
            const dur     = formatDuration(inc.duration_seconds||0);
            bc.textContent = `Incident #${inc.incident_id} · started ${started} · duration ${dur} · ${inc.peak_blocked_sessions||0} peak victims`;
            bc.style.display = 'block';
        }
        const capEl = document.getElementById('pgBlockingDetailsCapturedAt');
        if (capEl && d.collected_at) capEl.textContent = 'Snapshot: ' + new Date(d.collected_at).toLocaleString();
    } catch (e) {
        console.error('[PgLocks] incident drilldown failed:', e);
    }
};

// ── Chronic blockers panel ────────────────────────────────────────────────────

async function loadChronicBlockers(instQ, fromISO, toISO) {
    const tbody = document.getElementById('pgChronicBlockersTbody');
    if (!tbody) return;
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/locks-blocking/chronic-blockers?instance=${instQ}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}&limit=15`
        );
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Error loading chronic blockers</td></tr>'; return; }
        const d = await resp.json();
        const blockers = Array.isArray(d.blockers) ? d.blockers : [];
        if (!blockers.length) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No chronic blocking patterns in this window</td></tr>';
            return;
        }
        tbody.innerHTML = blockers.map((b, idx) => {
            const q       = String(b.query_sample || '');
            const preview = window.escapeHtml(q.length > 80 ? q.slice(0,80)+'…' : q);
            return `<tr class="pg-chronic-row" style="cursor:pointer;"
                        data-pid="${idx+1}" data-user="" data-sql="${window.escapeHtml(q)}">
                <td>${idx+1}</td>
                <td><span class="code-snippet" title="${window.escapeHtml(q)}">${preview}</span></td>
                <td><strong>${b.root_blocker_count||0}</strong></td>
                <td>${b.total_victims||0}</td>
                <td>${Number(b.avg_duration_sec||0).toFixed(1)}</td>
                <td>${Number(b.max_duration_sec||0).toFixed(1)}</td>
            </tr>`;
        }).join('');

        if (!tbody.dataset.pgChronicBound) {
            tbody.dataset.pgChronicBound = '1';
            tbody.addEventListener('click', (e) => {
                const tr = e.target?.closest?.('tr.pg-chronic-row');
                if (!tr) return;
                window.pgShowSqlModal(tr.dataset.pid||'', '', tr.dataset.sql||'');
            });
        }
    } catch (e) {
        console.error('[PgLocks] chronic blockers failed:', e);
        if (tbody) tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Error loading chronic blockers</td></tr>';
    }
}

// ── Tables ────────────────────────────────────────────────────────────────────

function renderPgTopLockedTables(rows) {
    const tbody = document.getElementById('pgTopLockedTablesTbody');
    if (!tbody) return;
    const list = Array.isArray(rows) ? rows : [];
    if (!list.length) {
        tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">No waiting locks in lookback window</td></tr>';
        return;
    }
    tbody.innerHTML = list.map(r => {
        const rel  = window.escapeHtml(String(r?.relation_name ?? 'virtual'));
        const cnt  = Number(r?.waiting_count  ?? 0) || 0;
        const maxw = Number(r?.max_wait_seconds ?? 0) || 0;
        return `<tr><td>${rel}</td><td><strong>${cnt}</strong></td><td>${maxw.toFixed(0)}</td></tr>`;
    }).join('');
}

function renderBlockingDetails(blockingTree, capturedAtIso) {
    const tbody = document.getElementById('pgBlockingDetailsTbody');
    if (!tbody) return;

    const flatten = (nodes, out=[]) => {
        (nodes||[]).forEach(n => { out.push(n); flatten(n.blocked_by, out); });
        return out;
    };
    const all = flatten(blockingTree, []);
    if (!all.length) {
        tbody.innerHTML = '<tr><td colspan="5" class="text-center text-muted">No blocking sessions detected</td></tr>';
        return;
    }
    all.sort((a,b) => (b.blocked_by?.length||0) - (a.blocked_by?.length||0));
    const top = all.slice(0, 20);

    tbody.innerHTML = top.map(n => {
        const sql     = String(n.query || '');
        const preview = window.escapeHtml(sql.substring(0, 90) + (sql.length > 90 ? '...' : ''));
        const pid     = n.pid != null ? String(n.pid) : '';
        return `
            <tr class="pg-blocking-row" style="cursor:pointer;"
                data-pid="${window.escapeHtml(pid)}"
                data-user="${window.escapeHtml(String(n.user||''))}"
                data-sql="${window.escapeHtml(sql)}">
                <td>${window.escapeHtml(pid||'-')}</td>
                <td>${window.escapeHtml(String(n.user||'-'))}</td>
                <td>${window.escapeHtml(String(n.duration||'-'))}</td>
                <td class="text-muted">${window.escapeHtml(String(n.wait_event||'-'))}</td>
                <td><span class="code-snippet">${preview}</span></td>
            </tr>
        `;
    }).join('');

    if (!tbody.dataset.pgBlockingClickBound) {
        tbody.dataset.pgBlockingClickBound = '1';
        tbody.addEventListener('click', (e) => {
            const tr = e.target?.closest?.('tr.pg-blocking-row');
            if (!tr) return;
            window.pgShowSqlModal(tr.dataset.pid||'', tr.dataset.user||'', tr.dataset.sql||'');
        });
    }
}

function renderBlockingSummary(blockingTree) {
    const elTotal    = document.getElementById('pgBlockedSessionsTotal');
    const elTopPid   = document.getElementById('pgTopBlockerPid');
    const elTopCnt   = document.getElementById('pgTopBlockerCount');
    const elWorstDur = document.getElementById('pgWorstBlockedDur');
    const elWorstPid = document.getElementById('pgWorstBlockedPid');
    const elIdleInTxn= document.getElementById('pgIdleInTxnInvolved');
    if (!elTotal||!elTopPid||!elTopCnt||!elWorstDur||!elWorstPid||!elIdleInTxn) return;

    if (!Array.isArray(blockingTree)||!blockingTree.length) {
        elTotal.textContent='0'; elTopPid.textContent='—'; elTopCnt.textContent='No blocking';
        elWorstDur.textContent='—'; elWorstPid.textContent='—'; elIdleInTxn.textContent='No';
        return;
    }

    const nowMs = Date.now();
    const blockers = new Map();
    let blockedSessionsTotal = 0;
    let idleInTxnInvolved = false;

    const walk = (parent, children) => {
        (children||[]).forEach(child => {
            blockedSessionsTotal++;
            if ((child.state||'').toLowerCase().includes('idle in transaction') ||
                (parent.state||'').toLowerCase().includes('idle in transaction') ||
                child.is_idle_in_txn === true || parent.is_idle_in_txn === true) {
                idleInTxnInvolved = true;
            }
            const parentPid = Number(parent?.pid||0)||0;
            if (parentPid) {
                const qsMs = child.query_start ? Date.parse(child.query_start) : 0;
                const bsec = qsMs ? Math.max(0, Math.floor((nowMs-qsMs)/1000)) : 0;
                const cur  = blockers.get(parentPid) || { blockedCount:0, maxBlockedSec:0 };
                cur.blockedCount++;
                cur.maxBlockedSec = Math.max(cur.maxBlockedSec, bsec);
                blockers.set(parentPid, cur);
            }
            walk(child, child.blocked_by);
        });
    };
    blockingTree.forEach(root => walk(root, root.blocked_by));

    elTotal.textContent = String(blockedSessionsTotal);
    elIdleInTxn.textContent = idleInTxnInvolved ? 'Yes' : 'No';
    elIdleInTxn.className = `strip-metric-value metric-value ${idleInTxnInvolved ? 'text-danger' : ''}`;

    const entries = Array.from(blockers.entries()).map(([pid,v]) => ({ pid, ...v }));
    if (!entries.length) {
        elTopPid.textContent='—'; elTopCnt.textContent='No blocking';
        elWorstDur.textContent='—'; elWorstPid.textContent='—';
        return;
    }
    entries.sort((a,b) => (b.blockedCount-a.blockedCount)||(b.maxBlockedSec-a.maxBlockedSec));
    const top = entries[0];
    elTopPid.textContent = String(top.pid);
    elTopCnt.textContent = `${top.blockedCount} blocked`;
    const worst = entries.slice().sort((a,b) => b.maxBlockedSec-a.maxBlockedSec)[0];
    elWorstDur.textContent = formatDuration(worst.maxBlockedSec);
    elWorstPid.textContent = `PID ${worst.pid}`;
}

// ── Timeline chart ────────────────────────────────────────────────────────────

function renderPgBlockingIncidentTimeline(points, incidents) {
    const canvas = document.getElementById('pgBlockingIncidentTimelineChart');
    if (!canvas) return;
    const rows   = Array.isArray(points) ? points : [];
    const labels = rows.map(p => (typeof window.fmtChartTick === 'function' ? window.fmtChartTick(p) : ''));
    const data   = rows.map(p => Number(p?.blocked_sessions ?? 0) || 0);

    const windows = (Array.isArray(incidents) ? incidents : []).map(w => ({
        startMs: typeof window.tsMs === 'function' ? (window.tsMs(w.started_at) ?? NaN) : (w?.started_at ? Date.parse(w.started_at) : NaN),
        endMs:   w?.ended_at
            ? (typeof window.tsMs === 'function' ? (window.tsMs(w.ended_at) ?? Date.now()) : Date.parse(w.ended_at))
            : Date.now(),
    })).filter(w => isFinite(w.startMs));

    const shadePlugin = {
        id: 'incidentShading',
        beforeDatasetsDraw(chart) {
            if (!windows.length) return;
            const ctx = chart.ctx, xScale = chart.scales.x, area = chart.chartArea;
            if (!xScale || !area) return;
            const xs = rows.map(p => {
                const ms = typeof window.tsMs === 'function' ? window.tsMs(p) : null;
                return ms != null ? ms : NaN;
            });
            ctx.save();
            ctx.fillStyle = 'rgba(239,68,68,0.10)';
            windows.forEach(w => {
                let first=-1, last=-1;
                for (let i=0; i<xs.length; i++) {
                    if (!isFinite(xs[i])) continue;
                    if (xs[i] >= w.startMs && xs[i] <= w.endMs) { if (first===-1) first=i; last=i; }
                }
                if (first===-1) return;
                const x0 = xScale.getPixelForValue(first);
                const x1 = xScale.getPixelForValue(last);
                ctx.fillRect(x0, area.top, Math.max(1, x1-x0), area.bottom-area.top);
            });
            ctx.restore();
        }
    };

    if (window.currentCharts.pgBlockingTimeline) window.currentCharts.pgBlockingTimeline.destroy();
    window.currentCharts.pgBlockingTimeline = new Chart(canvas.getContext('2d'), {
        type: 'line',
        data: { labels, datasets: [{ label:'Blocked sessions / min', data,
            borderColor: window.getCSSVar('--danger'), backgroundColor:'rgba(239,68,68,0.08)',
            fill:true, tension:0.25, pointRadius:0 }] },
        options: { responsive:true, maintainAspectRatio:false,
            interaction:{ mode:'index', intersect:false }, scales:{ y:{ beginAtZero:true } } },
        plugins: [shadePlugin],
    });
}

// ── SQL modal ─────────────────────────────────────────────────────────────────

window.pgShowSqlModal = function(pid, user, sql) {
    const existing = document.getElementById('pgSqlModal');
    if (existing) existing.remove();
    const div = document.createElement('div');
    div.id = 'pgSqlModal';
    div.style.cssText = 'position:fixed; inset:0; background:rgba(0,0,0,0.55); z-index:9999;';
    div.innerHTML = `
        <div class="glass-panel" style="max-width:900px; margin:6vh auto; padding:0.9rem;">
            <div class="flex-between" style="align-items:center; gap:1rem;">
                <div>
                    <div style="font-weight:700;">Blocking SQL Details</div>
                    <div class="text-muted" style="font-size:0.8rem;">PID: <code>${pid}</code> | User: <code>${user||'-'}</code></div>
                </div>
                <button class="btn btn-sm btn-outline" id="pgSqlClose">Close</button>
            </div>
            <div class="mt-2" style="max-height:55vh; overflow:auto;">
                <pre style="white-space:pre-wrap; margin:0; font-size:0.75rem; background:var(--bg-tertiary); padding:0.75rem; border-radius:8px;">${window.escapeHtml(sql||'(no query text)')}</pre>
            </div>
        </div>`;
    document.body.appendChild(div);
    const closeBtn = document.getElementById('pgSqlClose');
    if (closeBtn) closeBtn.onclick = () => div.remove();
    div.onclick = (e) => { if (e.target === div) div.remove(); };
};

// ── Lock mode badge ───────────────────────────────────────────────────────────

function getLockModeBadge(mode) {
    const ml = (mode||'').toLowerCase();
    if (ml.includes('exclusive')) return `<span class="badge badge-danger">${window.escapeHtml(mode)}</span>`;
    if (ml.includes('share'))     return `<span class="badge badge-warning">${window.escapeHtml(mode)}</span>`;
    return `<span class="badge badge-info">${window.escapeHtml(mode)}</span>`;
}
