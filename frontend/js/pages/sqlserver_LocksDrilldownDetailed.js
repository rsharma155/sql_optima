// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: sqlserver_LocksDrilldownDetailed.js
// Purpose: Read-only Blocking Incident Diagnostic Console.
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

let msDrilldownState = {
    selectedSpid: null,
    snapshotData: [],
    recurrenceData: [],
    blockedObjects: [],
    blockedLocks: [],
    charts: {}
};

// Lock mode sort order: most severe first
const LOCK_SEVERITY = { 'X': 0, 'U': 1, 'BU': 2, 'SCH-M': 3, 'S': 4, 'IX': 5, 'IS': 6, 'SCH-S': 7 };
function lockSev(mode) { const m = (mode || '').toUpperCase(); return LOCK_SEVERITY[m] !== undefined ? LOCK_SEVERITY[m] : 99; }

function ddFormatMB(bytes) {
    if (!bytes) return '0.00 MB';
    return (bytes / 1048576).toFixed(2) + ' MB';
}

function severityFromCount(n) {
    if (n >= 10) return { label: 'HIGH',   cls: 'badge-danger',  hdr: 'sev-high' };
    if (n >= 5)  return { label: 'MEDIUM', cls: 'badge-warning', hdr: 'sev-medium' };
    return               { label: 'LOW',   cls: 'badge-warning', hdr: 'sev-low' };
}

// ── Entry point ──────────────────────────────────────────────────────────────
window.sqlserver_LocksDrilldownDetailed = async function(params) {
    if (!params || !params.spid) {
        window.appNavigate('sqlserver-locks');
        return;
    }

    msDrilldownState.selectedSpid  = parseInt(params.spid, 10);
    msDrilldownState.snapshotData  = [];
    msDrilldownState.recurrenceData = [];
    msDrilldownState.blockedObjects = [];
    msDrilldownState.blockedLocks  = [];
    destroyCharts();

    const inst   = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Unknown', type: 'sqlserver' };
    const dbName = window.appState.currentDatabase || 'all';

    window.routerOutlet.innerHTML = await window.loadTemplate('pages/sqlserver_locks_drilldown_detailed.html', { inst, dbName });

    setupListeners();
    await loadData();
};

// ── Listeners ────────────────────────────────────────────────────────────────
function setupListeners() {
    const get = id => document.getElementById(id);

    get('btnBackToLocks')?.addEventListener('click', () => window.appNavigate('sqlserver-locks'));
    get('btnRefreshDrilldown')?.addEventListener('click', () => loadData());

    // Tab switching — use inline style, most reliable
    const tabNav = document.getElementById('rootBlockerTabNav');
    if (tabNav) {
        tabNav.addEventListener('click', e => {
            const btn = e.target.closest('.ddv2-tab-btn');
            if (!btn) return;
            const paneId = btn.getAttribute('data-pane');
            // Deactivate all
            tabNav.querySelectorAll('.ddv2-tab-btn').forEach(b => b.classList.remove('active'));
            document.querySelectorAll('#rootBlockerPanel .ddv2-pane').forEach(p => { p.style.display = 'none'; });
            // Activate target
            btn.classList.add('active');
            const pane = document.getElementById(paneId);
            if (pane) pane.style.display = 'flex';
        });
    }

    // SQL click to expand
    get('ddSessionQueryBox')?.addEventListener('click', function() {
        const sql = this.textContent || '';
        if (sql && !sql.startsWith('--')) window.showQueryModal(sql);
    });
    get('ddQueryText')?.addEventListener('click', function() {
        const sql = this.textContent || '';
        if (sql && !sql.startsWith('--')) window.showQueryModal(sql);
    });

    // Copy SQL
    get('btnCopySql')?.addEventListener('click', () => {
        const sql = get('ddQueryText')?.textContent || '';
        if (!sql) return;
        navigator.clipboard.writeText(sql).then(() => {
            const btn = get('btnCopySql');
            if (btn) { const orig = btn.innerHTML; btn.innerHTML = '<i class="fa-solid fa-check"></i> Copied'; setTimeout(() => btn.innerHTML = orig, 2000); }
        }).catch(() => {});
    });
}

// ── Data loading ─────────────────────────────────────────────────────────────
async function loadData() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;

    const from = window.appState.fromTs ? new Date(window.appState.fromTs).toISOString() : '';
    const to   = window.appState.toTs   ? new Date(window.appState.toTs).toISOString()   : '';
    const base = `instance=${encodeURIComponent(inst.name)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;

    try {
        const [detRes, lockRes, objRes] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/details?${base}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/locks?${base}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/most-blocked-objects?${base}`)
        ]);

        msDrilldownState.snapshotData  = detRes.ok  ? (await detRes.json()  || []) : [];
        msDrilldownState.blockedLocks  = lockRes.ok ? (await lockRes.json() || []) : [];
        msDrilldownState.blockedObjects = objRes.ok ? (await objRes.json()  || []) : [];

        // Recurrence (needs root blocker identity)
        const root = getRootBlocker();
        if (root) {
            const sqlHash = encodeURIComponent(root.sql_hash  || '');
            const login   = encodeURIComponent(root.login_name || '');
            const recRes  = await window.apiClient.authenticatedFetch(
                `/api/sqlserver/blocking/recurrence?instance=${encodeURIComponent(inst.name)}&sql_hash=${sqlHash}&login_name=${login}`
            );
            msDrilldownState.recurrenceData = recRes.ok ? (await recRes.json() || []) : [];
        }

        render();
    } catch (err) {
        appDebug('[LocksDrilldownDetailed] load failed:', err);
        const el = document.getElementById('ddVictimsTbody');
        if (el) el.innerHTML = `<tr><td colspan="8" class="text-center text-danger" style="padding:1rem;">Failed to load data: ${window.escapeHtml(String(err?.message || err))}</td></tr>`;
    }
}

// ── Data helpers ──────────────────────────────────────────────────────────────
function getSelectedVictim() {
    const spid = msDrilldownState.selectedSpid;
    return msDrilldownState.snapshotData.find(d => d.session_id === spid) || null;
}

function getRootBlocker() {
    const victim = getSelectedVictim();
    if (!victim) return null;
    // If this session isn't blocked by anyone — it IS the root
    if (!victim.blocking_session_id || victim.blocking_session_id === 0) return victim;

    const data = msDrilldownState.snapshotData;
    let current = victim;
    const visited = new Set([victim.session_id]);

    for (let i = 0; i < 50; i++) {
        const parent = data.find(d => d.session_id === current.blocking_session_id);
        if (!parent || visited.has(parent.session_id)) break;
        visited.add(parent.session_id);
        current = parent;
        if (!current.blocking_session_id || current.blocking_session_id === 0) break;
    }
    return current;
}

// ── Main render ───────────────────────────────────────────────────────────────
function render() {
    const victim = getSelectedVictim();
    const root   = getRootBlocker();

    if (!victim && !root) {
        setHtml('ddVictimsTbody', `<tr><td colspan="8" class="text-center text-muted" style="padding:1.5rem;">
            Selected SPID #${msDrilldownState.selectedSpid} not found in the current time window.
            <br><small>Try expanding the time range, or check if blocking has already resolved.</small>
        </td></tr>`);
        return;
    }

    const data    = msDrilldownState.snapshotData;
    const rootSpid = root ? root.session_id : (victim?.blocking_session_id ?? 0);
    // Victims are sessions being blocked BY root
    const victims = data.filter(d => d.blocking_session_id === rootSpid);

    const totalWaitMs = victims.reduce((s, d) => s + (d.wait_duration_ms || 0), 0);
    const maxWaitMs   = victims.length ? Math.max(...victims.map(d => d.wait_duration_ms || 0)) : 0;
    const refTs       = victim ? (window.parseChartTimestampOrNow ? window.parseChartTimestampOrNow(victim) : new Date()) : new Date();

    let txnAgeSec = 0;
    if (root?.transaction_start_time) {
        txnAgeSec = Math.max(0, (refTs - new Date(root.transaction_start_time)) / 1000);
    }

    // ── Header severity ──
    const sev = severityFromCount(victims.length);
    const hdr = document.getElementById('incidentHeader');
    if (hdr) { hdr.classList.remove('sev-low', 'sev-medium', 'sev-high'); hdr.classList.add(sev.hdr); }
    const pill = document.getElementById('ddSeverityPill');
    if (pill) { pill.textContent = sev.label; pill.className = `badge ${sev.cls}`; pill.style.cssText = 'font-size:0.7rem; padding:4px 10px;'; }

    // ── Header KPIs ──
    const r = root || victim;
    setText('ddSummarySpid',   `SPID ${r?.session_id ?? '--'}`);
    setText('ddHeaderSub',
        `DB: ${r?.database_name || '--'}  •  Host: ${r?.host_name || '--'}  •  Login: ${r?.login_name || '--'}  •  Iso: ${r?.transaction_isolation_level || '--'}  •  OpenTran: ${r?.open_transaction_count ?? '--'}`
    );
    setText('ddKpiWaitImpact',  `${(totalWaitMs / 1000).toFixed(1)}s`);
    setText('ddKpiVictims',     victims.length);
    setText('ddKpiLongestWait', `${(maxWaitMs / 1000).toFixed(1)}s`);
    setText('ddKpiDuration',    `${txnAgeSec.toFixed(0)}s`);

    if (!root) {
        setText('ddVictimCountBadge', '0 Sessions');
        renderVictimsTable([], refTs);
        return;
    }

    // ── Root Blocker SPID badge ──
    setText('ddBlockerSpidTag', `#${root.session_id}`);

    // ── SESSION TAB ──
    setText('ddStatus',       root.open_transaction_count > 0 ? 'BLOCKING (OPEN TRAN)' : 'BLOCKING');
    setTextTitle('ddProgram', root.program_name || 'N/A');
    setTextTitle('ddHost',    root.host_name    || 'N/A');
    setTextTitle('ddLogin',   root.login_name   || 'N/A');
    setText('ddCpu',          (root.cpu_time    || 0).toLocaleString() + ' ms');
    setText('ddMemory',       (root.memory_usage || 0).toLocaleString() + ' pages');
    setText('ddReads',        (root.reads        || 0).toLocaleString());
    setText('ddWrites',       (root.writes       || 0).toLocaleString());
    setText('ddLastRequest',  window.fmtChartTick ? window.fmtChartTick(root) || '--' : '--');
    setText('ddOpenTran',     root.open_transaction_count ?? 0);
    setText('ddIso',          root.transaction_isolation_level || 'N/A');
    setText('ddWaitType',     root.wait_type || 'N/A');

    const sqlText = root.query_text || '';
    setText('ddSessionQueryBox', sqlText || '-- No SQL text captured for this session');
    setText('ddQueryText',       sqlText || '-- No SQL text captured for this session');

    // ── QUERY TAB ──
    setText('ddSqlHash',  root.sql_hash  || '--');
    setText('ddPlanHash', root.plan_hash || '--');
    const sameHashRows = data.filter(d => d.sql_hash && d.sql_hash === root.sql_hash);
    const incidentCount = sameHashRows.length || data.filter(d => d.session_id === root.session_id).length;
    const blockerRows   = data.filter(d => d.session_id === root.session_id);
    const avgWait = blockerRows.length
        ? Math.round(blockerRows.reduce((s, d) => s + (d.wait_duration_ms || 0), 0) / blockerRows.length)
        : null;
    setText('ddExecCount', incidentCount || '--');
    setText('ddAvgWait',   avgWait != null ? `${avgWait.toLocaleString()} ms` : '--');

    // ── TRANSACTION TAB ──
    const txnBegin = root.transaction_start_time ? new Date(root.transaction_start_time) : null;
    setText('ddTxnStart',    txnBegin ? txnBegin.toLocaleString() : '--');
    setText('ddTxnAge',      txnBegin ? `${txnAgeSec.toFixed(1)}s` : '--');
    setText('ddLogUsed',     ddFormatMB(root.transaction_log_bytes_used    || 0));
    setText('ddLogReserved', ddFormatMB(root.transaction_log_bytes_reserved || 0));
    setText('ddTxnState',    root.open_transaction_count > 0 ? 'Active (Open Transaction)' : 'Idle / No Active Transaction');

    // ── LOCKS TAB ──
    renderLocksTable(root.session_id);

    // ── VICTIMS TABLE ──
    renderVictimsTable(victims, refTs);

    // ── CHARTS ──
    destroyCharts();
    renderWaitContributionChart(victims);
    renderBlockingObjects();
    renderWaitAccumulationChart(data.filter(d => d.session_id === root.session_id));
    renderRecurrenceChart();
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function setText(id, val) {
    const el = document.getElementById(id);
    if (el) el.textContent = (val === null || val === undefined) ? '--' : String(val);
}
function setTextTitle(id, val) {
    const el = document.getElementById(id);
    if (el) { const s = (val === null || val === undefined) ? '--' : String(val); el.textContent = s; el.title = s; }
}
function setHtml(id, html) {
    const el = document.getElementById(id);
    if (el) el.innerHTML = html;
}

// ── Locks table ───────────────────────────────────────────────────────────────
function renderLocksTable(spid) {
    const locks = (msDrilldownState.blockedLocks || [])
        .filter(l => l.session_id === spid)
        .sort((a, b) => lockSev(a.request_mode) - lockSev(b.request_mode));

    if (!locks.length) {
        setHtml('ddBlockerLocksTbody', `<tr><td colspan="4" class="text-center text-muted" style="padding:1rem;">No lock records for SPID ${spid} in this window.</td></tr>`);
        return;
    }
    setHtml('ddBlockerLocksTbody', locks.map(l => {
        const statusCls = l.request_status === 'GRANT' ? 'text-accent' : 'text-warning';
        const desc = (l.resource_description || '--');
        return `<tr>
            <td style="font-size:0.7rem;">${window.escapeHtml(l.resource_type || '--')}</td>
            <td style="font-size:0.7rem; max-width:180px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(desc)}">${window.escapeHtml(desc.length > 40 ? desc.substring(0,40) + '…' : desc)}</td>
            <td><span class="badge badge-outline" style="font-size:0.62rem;">${window.escapeHtml(l.request_mode || '--')}</span></td>
            <td class="${statusCls}" style="font-size:0.7rem; font-weight:700;">${window.escapeHtml(l.request_status || '--')}</td>
        </tr>`;
    }).join(''));
}

// ── Victims table ─────────────────────────────────────────────────────────────
function renderVictimsTable(victims, refTs) {
    const badge = document.getElementById('ddVictimCountBadge');
    if (badge) badge.textContent = `${victims.length} Session${victims.length !== 1 ? 's' : ''}`;

    if (!victims.length) {
        setHtml('ddVictimsTbody', `<tr><td colspan="8" class="text-center text-muted" style="padding:1rem;">No victims found — the selected session may be the only node in the chain.</td></tr>`);
        return;
    }

    const sorted = [...victims].sort((a, b) => (b.wait_duration_ms || 0) - (a.wait_duration_ms || 0));
    setHtml('ddVictimsTbody', sorted.map(d => {
        const waitSec  = (d.wait_duration_ms || 0) / 1000;
        const rowCls   = waitSec >= 30 ? 'ddv2-victim-row-red' : waitSec >= 10 ? 'ddv2-victim-row-orange' : '';
        const waitColor = waitSec >= 30 ? '#ef4444' : waitSec >= 10 ? '#f97316' : '#10b981';
        const tranAge  = d.transaction_start_time
            ? `${((refTs - new Date(d.transaction_start_time)) / 1000).toFixed(1)}s`
            : '--';
        const hashShort = (d.sql_hash || '').replace('0x','').substring(0,8) || '--';
        const sqlFull   = d.query_text || '';
        const sqlShort  = sqlFull ? (sqlFull.length > 22 ? sqlFull.substring(0,22) + '…' : sqlFull) : null;
        const qCell = sqlShort
            ? `<span class="ddv2-sql-link" title="${window.escapeHtml(sqlFull)}"
                 data-action="show-query-modal-direct"
                 data-encoded-query="${encodeURIComponent(sqlFull)}"
                 data-fn="showQueryModal">${window.escapeHtml(sqlShort)}</span>`
            : `<span style="color:var(--text-muted); font-size:0.65rem; font-style:italic;">N/A</span>`;

        return `<tr class="${rowCls}">
            <td><strong style="font-size:0.75rem;">${d.session_id}</strong></td>
            <td style="font-size:0.65rem; color:var(--accent,#3b82f6); font-weight:600; white-space:nowrap;">${window.escapeHtml(d.wait_type || '--')}</td>
            <td style="font-weight:800; color:${waitColor}; white-space:nowrap;">${waitSec.toFixed(1)}</td>
            <td style="font-size:0.65rem; color:var(--text-muted); max-width:90px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(d.wait_resource || '')}">${window.escapeHtml((d.wait_resource || '--').substring(0,16))}</td>
            <td style="font-size:0.68rem; white-space:nowrap;">${window.escapeHtml(d.database_name || '--')}</td>
            <td class="font-mono" style="font-size:0.6rem; color:var(--accent,#3b82f6);">${hashShort}</td>
            <td style="font-size:0.68rem; white-space:nowrap;">${tranAge}</td>
            <td>${qCell}</td>
        </tr>`;
    }).join(''));
}

// ── Charts ────────────────────────────────────────────────────────────────────
function destroyCharts() {
    Object.keys(msDrilldownState.charts).forEach(k => {
        if (msDrilldownState.charts[k]) { msDrilldownState.charts[k].destroy(); msDrilldownState.charts[k] = null; }
    });
}

function renderWaitContributionChart(victims) {
    const canvas = document.getElementById('ddWaitContributionChart');
    if (!canvas) return;

    const waits = {};
    victims.forEach(v => {
        const t = v.wait_type || 'OTHER';
        waits[t] = (waits[t] || 0) + ((v.wait_duration_ms || 0) / 1000);
    });
    const sorted = Object.entries(waits).sort((a,b) => b[1]-a[1]).slice(0,5);
    if (!sorted.length) return;

    msDrilldownState.charts.contribution = new Chart(canvas.getContext('2d'), {
        type: 'bar',
        data: {
            labels: sorted.map(s => s[0]),
            datasets: [{ data: sorted.map(s => parseFloat(s[1].toFixed(2))), backgroundColor: '#3b82f6', borderRadius: 2, barThickness: 14 }]
        },
        options: {
            indexAxis: 'y',
            responsive: true, maintainAspectRatio: false,
            plugins: { legend: { display: false }, tooltip: { callbacks: { label: c => `${c.parsed.x.toFixed(2)}s` } } },
            scales: {
                x: { beginAtZero: true, grid: { color: 'rgba(100,116,139,0.1)' }, ticks: { color: '#94a3b8', font: { size: 8 } } },
                y: { grid: { display: false }, ticks: { color: '#e2e8f0', font: { size: 9 } } }
            }
        }
    });
}

function renderBlockingObjects() {
    const objects = msDrilldownState.blockedObjects || [];
    if (!objects.length) return;

    setHtml('ddObjectsTbody', objects.slice(0,10).map(o => {
        const waitSec  = ((o.total_wait || 0) / 1000).toFixed(1);
        const objName  = (o.object_name || 'Unknown');
        const db       = (o.database_name || '--');
        return `<tr>
            <td style="font-size:0.65rem; color:var(--text-muted);">${window.escapeHtml(db)}</td>
            <td class="font-mono" style="font-size:0.65rem; font-weight:700;" title="${window.escapeHtml(objName)}">${window.escapeHtml(objName.length > 28 ? objName.substring(0,28)+'…' : objName)}</td>
            <td style="font-size:0.65rem;"><span class="badge badge-outline">${window.escapeHtml(o.lock_type || '--')}</span></td>
            <td style="font-weight:800; color:#f59e0b; font-size:0.72rem;">${waitSec}</td>
            <td style="font-size:0.72rem;">${o.incident_count || 0}</td>
        </tr>`;
    }).join(''));
}

function renderWaitAccumulationChart(occurrences) {
    const canvas = document.getElementById('ddWaitAccumulationChart');
    if (!canvas || !occurrences.length) return;

    const sorted = window.sortByChartTime ? window.sortByChartTime(occurrences) : [...occurrences];
    let sum = 0;
    const labels = sorted.map(d => window.fmtChartTick(d, { hour: '2-digit', minute: '2-digit' }));
    const values = sorted.map(d => { sum += (d.wait_duration_ms||0)/1000; return parseFloat(sum.toFixed(2)); });

    msDrilldownState.charts.accumulation = new Chart(canvas.getContext('2d'), {
        type: 'line',
        data: {
            labels,
            datasets: [{ data: values, borderColor: '#ef4444', backgroundColor: 'rgba(239,68,68,0.06)', fill: true, tension: 0.1, pointRadius: 0, borderWidth: 2 }]
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(100,116,139,0.1)' }, ticks: { color: '#94a3b8', font: { size: 8 } } },
                x: { grid: { display: false }, ticks: { color: '#94a3b8', font: { size: 8 }, maxTicksLimit: 10 } }
            }
        }
    });
}

function renderRecurrenceChart() {
    const canvas = document.getElementById('ddRecurrenceChart');
    const data   = msDrilldownState.recurrenceData || [];
    if (!canvas || !data.length) return;

    const sorted = window.sortByChartTime ? window.sortByChartTime(data) : [...data];
    msDrilldownState.charts.recurrence = new Chart(canvas.getContext('2d'), {
        type: 'bar',
        data: {
            labels: sorted.map(d => window.fmtChartTick(d, { dateStyle: 'short' })),
            datasets: [{ data: sorted.map(d => d.incident_count||0), backgroundColor: '#10b981', borderRadius: 2, barPercentage: 0.6 }]
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(100,116,139,0.1)' }, ticks: { color: '#94a3b8', font: { size: 8 }, stepSize: 1 } },
                x: { grid: { display: false }, ticks: { color: '#94a3b8', font: { size: 8 }, maxTicksLimit: 12 } }
            }
        }
    });
}
