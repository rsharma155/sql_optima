/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Implements the sessions dashboard view.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgSessionsView = async function() {
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/sessions.html');
    setTimeout(initPgSessions, 50);
}

async function initPgSessions() {
    window.currentCharts = window.currentCharts || {};
    
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: ''};
    const database = window.appState.currentDatabase || 'all';

    // SQL Server-style header widgets (status strip + instance/database labels)
    try {
        const instEl = document.getElementById('pgSessionsInstance');
        const dbEl = document.getElementById('pgSessionsDatabase');
        if (instEl) instEl.textContent = inst.name || '--';
        if (dbEl) dbEl.textContent = database;

        const strip = document.getElementById('pgSessionsStatusStrip');
        if (strip && typeof window.renderStatusStrip === 'function') {
            strip.innerHTML = window.renderStatusStrip({
                lastUpdateId: 'pgSessionsLastRefreshTime',
                sourceBadgeId: 'pgSessionsSourceBadge',
                includeHealth: false,
                includeFreshness: false,
                autoRefreshText: ''
            });
        }
        const t = document.getElementById('pgSessionsLastRefreshTime');
        if (t) t.textContent = new Date().toLocaleTimeString();
    } catch (e) {
        // non-fatal
    }

    let sessions = [];
    let stateHist = null;
    try {
        const [sessResp, histResp] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/postgres/sessions?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/sessions/state/history?instance=${encodeURIComponent(inst.name)}&limit=180`)
        ]);

        if (sessResp.ok) {
            const contentType = sessResp.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const data = await sessResp.json();
                sessions = data.sessions || [];
            }
        } else console.error("Failed to load PG sessions:", sessResp.status);

        if (histResp.ok) {
            const contentType = histResp.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const data = await histResp.json();
                stateHist = data && data.history ? data.history : null;
            }
        }
    } catch (e) {
        console.error("PG sessions fetch failed:", e);
    }

    try {
        const t = document.getElementById('pgSessionsLastRefreshTime');
        if (t) t.textContent = new Date().toLocaleTimeString();
    } catch (e) {
        // non-fatal
    }

    window._pgSessionsData = sessions;

    // Respect the currently selected filter when re-rendering after refresh/reload.
    const filterEl = document.getElementById('sessionStateFilter');
    const _applySessionFilter = (allSessions) => {
        const filter = filterEl ? filterEl.value : 'all';
        if (filter === 'active') {
            return allSessions.filter(s => (s.state || '').toLowerCase() === 'active');
        } else if (filter === 'idle in transaction') {
            return allSessions.filter(s => (s.state || '').toLowerCase() === 'idle in transaction');
        }
        return allSessions;
    };

    renderSessionsTable(_applySessionFilter(sessions));
    renderSessionsSpotlight(sessions);
    renderSessionStateCharts(stateHist);

    // Guard against duplicate listener attachment on auto-refresh re-runs.
    if (filterEl && !filterEl._pgFilterBound) {
        filterEl._pgFilterBound = true;
        filterEl.addEventListener('change', function() {
            renderSessionsTable(_applySessionFilter(window._pgSessionsData || []));
        });
    }

}

function renderSessionsSpotlight(sessions) {
    const byState = (s, st) => (String(s?.state || '').toLowerCase() === st);
    const hasWait = (s) => !!(s?.wait_event && String(s.wait_event).trim() !== '');
    const isBlocked = (s) => s?.blocked_by !== undefined && s?.blocked_by !== null && String(s.blocked_by) !== '';

    const parseDurSec = (dur) => {
        const v = String(dur || '');
        const m = v.match(/^(\d+):([0-5]\d):([0-5]\d)$/);
        if (!m) return 0;
        return (parseInt(m[1], 10) * 3600) + (parseInt(m[2], 10) * 60) + parseInt(m[3], 10);
    };
    const fmtDur = (sec) => {
        const s = Number(sec || 0);
        if (!isFinite(s) || s <= 0) return '—';
        const h = Math.floor(s / 3600);
        const m = Math.floor((s % 3600) / 60);
        const ss = Math.floor(s % 60);
        if (h > 0) return `${h}h ${m}m`;
        if (m > 0) return `${m}m ${ss}s`;
        return `${ss}s`;
    };

    const activeNow = (sessions || []).filter(s => byState(s, 'active')).length;
    const waitingNow = (sessions || []).filter(hasWait).length;
    const idleInTxnNow = (sessions || []).filter(s => byState(s, 'idle in transaction')).length;
    const blockedNow = (sessions || []).filter(isBlocked).length;
    const totalNow = (sessions || []).length;

    const setText = (id, txt) => { const el = document.getElementById(id); if (el) el.textContent = txt; };
    setText('pgSessActiveNow', String(activeNow));
    setText('pgSessWaitingNow', String(waitingNow));
    setText('pgSessIdleInTxnNow', String(idleInTxnNow));
    setText('pgSessBlockedNow', String(blockedNow));
    setText('pgSessTotalNow', String(totalNow));

    const longestActive = (sessions || [])
        .filter(s => byState(s, 'active'))
        .map(s => ({ s, sec: parseDurSec(s.duration) }))
        .sort((a, b) => b.sec - a.sec)[0];
    setText('pgSessLongestActiveDur', longestActive?.sec ? fmtDur(longestActive.sec) : '—');
    setText('pgSessLongestActivePid', longestActive?.s?.pid ? `PID ${longestActive.s.pid}` : '—');

    const longestIit = (sessions || [])
        .filter(s => byState(s, 'idle in transaction'))
        .map(s => ({ s, sec: parseDurSec(s.duration) }))
        .sort((a, b) => b.sec - a.sec)[0];
    setText('pgSessLongestIdleInTxnDur', longestIit?.sec ? fmtDur(longestIit.sec) : '—');
    setText('pgSessLongestIdleInTxnPid', longestIit?.s?.pid ? `PID ${longestIit.s.pid}` : '—');

    const elIit = document.getElementById('pgSessIdleInTxnNow');
    if (elIit) elIit.className = `strip-metric-value metric-value ${idleInTxnNow > 0 ? 'text-warning' : ''}`;
    const elBlocked = document.getElementById('pgSessBlockedNow');
    if (elBlocked) elBlocked.className = `strip-metric-value metric-value ${blockedNow > 0 ? 'text-danger' : ''}`;
}

function renderSessionStateCharts(hist) {
    try {
        const c1 = document.getElementById('pgSessionStateTrendChart');
        const c2 = document.getElementById('pgIdleInTxnTrendChart');
        if (!c1 || !c2) return;

        const labels = Array.isArray(hist?.labels) ? hist.labels : [];
        const active = Array.isArray(hist?.active) ? hist.active : [];
        const idle = Array.isArray(hist?.idle) ? hist.idle : [];
        const idleInTxn = Array.isArray(hist?.idle_in_txn) ? hist.idle_in_txn : [];
        const waiting = Array.isArray(hist?.waiting) ? hist.waiting : [];

        if (!labels.length) {
            const card = c1.closest('.chart-card') || c1.parentElement;
            if (card) card.style.display = 'none';
            const card2 = c2.closest('.chart-card') || c2.parentElement;
            if (card2) card2.style.display = 'none';
            return;
        }

        const xLabels = labels.map(s => {
            try { return new Date(s).toLocaleTimeString(); } catch { return ''; }
        });

        window.currentCharts = window.currentCharts || {};
        if (window.currentCharts.pgSessTrend) window.currentCharts.pgSessTrend.destroy();
        if (window.currentCharts.pgIitTrend) window.currentCharts.pgIitTrend.destroy();

        window.currentCharts.pgSessTrend = new Chart(c1.getContext('2d'), {
            type: 'line',
            data: {
                labels: xLabels,
                datasets: [
                    { label: 'Active', data: active, borderColor: window.getCSSVar('--success'), backgroundColor: 'rgba(34,197,94,0.10)', fill: true, tension: 0.25, pointRadius: 0 },
                    { label: 'Waiting', data: waiting, borderColor: window.getCSSVar('--danger'), backgroundColor: 'rgba(239,68,68,0.08)', fill: true, tension: 0.25, pointRadius: 0 },
                    { label: 'Idle', data: idle, borderColor: window.getCSSVar('--accent-blue'), backgroundColor: 'rgba(59,130,246,0.07)', fill: true, tension: 0.25, pointRadius: 0 },
                ]
            },
            options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'top' } }, scales: { y: { beginAtZero: true } } }
        });

        window.currentCharts.pgIitTrend = new Chart(c2.getContext('2d'), {
            type: 'line',
            data: {
                labels: xLabels,
                datasets: [
                    { label: 'Idle in Txn', data: idleInTxn, borderColor: window.getCSSVar('--warning'), backgroundColor: 'rgba(245,158,11,0.15)', fill: true, tension: 0.25, pointRadius: 0 }
                ]
            },
            options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'top' } }, scales: { y: { beginAtZero: true } } }
        });
    } catch (e) {
        // non-fatal
    }
}

function renderSessionsTable(sessions) {
    const tbody = document.getElementById('sessionsTbody');
    if (!tbody) return;

    if (!sessions || sessions.length === 0) {
        tbody.innerHTML = '<tr><td colspan="9" class="text-center text-muted">No sessions found for the selected filter.</td></tr>';
        return;
    }

    tbody.innerHTML = sessions.map(session => {
        const blockedBy = session.blocked_by || '-';
        const stateBadge = getStateBadge(session.state);
        const waitBadge = session.wait_event ? `<span class="badge badge-warning">${window.escapeHtml(session.wait_event)}</span>` : '<span class="text-muted">-</span>';
        const querySnippet = session.query ? `<span class="code-snippet" title="${window.escapeHtml(session.query)}">${window.escapeHtml(session.query.substring(0, 50))}${session.query.length > 50 ? '...' : ''}</span>` : '<span class="text-muted">-</span>';

        return `
            <tr>
                <td>${session.pid || '-'}</td>
                <td><strong>${window.escapeHtml(session.user || '-')}</strong><br/><span class="text-muted">${window.escapeHtml(session.database || '-')}</span></td>
                <td>${window.escapeHtml(session.app_name || 'Unknown')}</td>
                <td>${stateBadge}</td>
                <td>${session.duration || '-'}</td>
                <td>${waitBadge}</td>
                <td>${blockedBy}</td>
                <td>${querySnippet}</td>
                <td>
                    <button class="btn btn-sm btn-outline" style="border-color:var(--danger); color:var(--danger)" data-action="call" data-fn="killPgSession" data-arg="${session.pid}">Kill</button>
                    <button class="btn btn-sm btn-outline" data-action="call" data-fn="showQueryModal" data-arg="${window.escapeHtml(session.query || '')}">Explain</button>
                </td>
            </tr>
        `;
    }).join('');
}

window.killPgSession = function(pid) {
    if (confirm(`Terminate PostgreSQL backend process PID ${pid}?`)) {
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
        if (!inst) return;
        window.apiClient.authenticatedFetch(`/api/postgres/kill-session?instance=${encodeURIComponent(inst.name)}&pid=${pid}`, { method: 'POST' })
            .then(res => res.json())
            .then(data => {
                if (data.success) {
                    alert(`Session PID ${pid} terminated successfully.`);
                    window.PgSessionsView();
                } else {
                    alert(`Failed to terminate session: ${data.error || 'Unknown error'}`);
                }
            })
            .catch(err => alert(`Error: ${err.message}`));
    }
};

function getStateBadge(state) {
    const stateLower = (state || '').toLowerCase();
    if (stateLower === 'active') {
        return '<span class="badge badge-success">active</span>';
    } else if (stateLower === 'idle') {
        return '<span class="badge badge-secondary">idle</span>';
    } else if (stateLower === 'idle in transaction') {
        return '<span class="badge badge-warning">idle in transaction</span>';
    } else {
        return `<span class="badge badge-info">${window.escapeHtml(state)}</span>`;
    }
}
