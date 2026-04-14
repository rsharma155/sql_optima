/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Lock analysis page for lock contention and blocking sessions.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.LocksDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'sqlserver') {
        window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">Select a SQL Server instance first</h3></div>`;
        return;
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <button class="btn btn-sm btn-outline mb-2" onclick="window.appNavigate('dashboard')"><i class="fa-solid fa-arrow-left"></i> Back to Dashboard</button>
                    <h1>Blocking & Wait Types Drilldown</h1>
                    <p class="subtitle">Live blocking chains, blocked sessions, and current wait pressure (user workloads)</p>
                </div>
                <div class="filter-group glass-panel p-3 rounded flex-between" style="flex-wrap:wrap; gap:0.75rem; max-width:420px;">
                    <span style="font-size:0.85rem;">Wait category (chart):</span>
                    <select class="custom-select" style="min-width:11rem;" id="waitCategoryFilter" onchange="window.applyLocksWaitCategoryFilter()">
                        <option value="all">All waits</option>
                        <option value="LCK">LCK (locks)</option>
                        <option value="PAGEIOLATCH">PAGEIOLATCH</option>
                        <option value="CXPACKET">CXPACKET / CXCONSUMER</option>
                        <option value="ASYNC_NETWORK_IO">ASYNC_NETWORK_IO</option>
                    </select>
                    <button type="button" class="btn btn-sm btn-outline" onclick="window.loadBlockingData(true)" title="Refresh all sections"><i class="fa-solid fa-refresh"></i></button>
                </div>
            </div>

            <div id="locks-quick-links" class="flex-between glass-panel p-2 mt-2" style="flex-wrap:wrap; gap:0.5rem; font-size:0.78rem;">
                <span class="text-muted">Jump to:</span>
                <button type="button" class="btn btn-sm btn-outline" onclick="window.appNavigate('live-diagnostics')"><i class="fa-solid fa-bolt text-warning"></i> Real-Time Diagnostics</button>
                <button type="button" class="btn btn-sm btn-outline" onclick="window.appNavigate('drilldown-deadlocks')"><i class="fa-solid fa-skull text-danger"></i> Deadlock history</button>
                <button type="button" class="btn btn-sm btn-outline" onclick="window.appNavigate('drilldown-cpu')"><i class="fa-solid fa-microchip"></i> CPU drilldown</button>
                <button type="button" class="btn btn-sm btn-outline" onclick="window.appNavigate('drilldown-memory')"><i class="fa-solid fa-memory"></i> Memory drilldown</button>
            </div>

            <div id="locks-summary-strip" class="tables-grid mt-3" style="display:grid; grid-template-columns:repeat(auto-fit,minmax(140px,1fr)); gap:0.75rem;"></div>

            <div class="chart-card glass-panel mt-4" style="height:350px; position:relative;">
                <div class="card-header flex-between" style="flex-wrap:wrap; gap:0.5rem;">
                    <h3 style="margin:0;">Current wait time by type <span class="text-muted" style="font-size:0.72rem; font-weight:normal;">(sys.dm_os_waiting_tasks, user DBs)</span></h3>
                </div>
                <div class="chart-container" style="height:250px; position:relative;"><canvas id="drillLockChart"></canvas></div>
            </div>

            <div class="table-card glass-panel mt-4 border-danger">
                <div class="card-header flex-between" style="flex-wrap:wrap; gap:0.5rem;">
                    <h3 style="margin:0;">Live blocking chains</h3>
                    <span class="text-muted" style="font-size:0.72rem;">Lead blockers from <code>sys.dm_exec_requests</code> (same as Real-Time Diagnostics)</span>
                </div>
                <div class="table-responsive" id="live-blocking-table-container">
                    <div style="display:flex; justify-content:center; align-items:center; height:100px;">
                        <div class="spinner"></div><span style="margin-left:1rem;">Loading…</span>
                    </div>
                </div>
            </div>

            <div class="table-card glass-panel mt-4 border-warning">
                <div class="card-header flex-between" style="flex-wrap:wrap; gap:0.5rem;">
                    <h3 style="margin:0;">Blocked sessions (dashboard snapshot)</h3>
                    <span class="text-muted" style="font-size:0.72rem;">Victims with a non-zero blocking SPID — detail from cached dashboard payload</span>
                </div>
                <div class="table-responsive" id="blocked-sessions-table-container">
                    <div style="display:flex; justify-content:center; align-items:center; height:100px;">
                        <div class="spinner"></div><span style="margin-left:1rem;">Loading…</span>
                    </div>
                </div>
            </div>

            <div class="table-card glass-panel mt-4 border-danger">
                <div class="card-header flex-between" style="flex-wrap:wrap; gap:0.5rem;">
                    <h3 style="margin:0;">Head blockers (aggregated from snapshot)</h3>
                    <button type="button" class="btn btn-sm btn-outline" onclick="window.appNavigate('drilldown-deadlocks')"><i class="fa-solid fa-skull text-danger"></i> Deadlocks</button>
                </div>
                <div class="table-responsive" id="blocking-table-container">
                    <div style="display:flex; justify-content:center; align-items:center; height:100px;">
                        <div class="spinner"></div><span style="margin-left:1rem;">Loading…</span>
                    </div>
                </div>
            </div>
        </div>
    `;

    await window.loadBlockingData(true);
};

function locksWaitCategoryMatches(waitType, cat) {
    const w = (waitType || '').toUpperCase();
    if (!cat || cat === 'all') return true;
    if (cat === 'LCK') return w.indexOf('LCK') !== -1 || w.indexOf('LOCK') !== -1;
    if (cat === 'PAGEIOLATCH') return w.indexOf('PAGEIOLATCH') !== -1;
    if (cat === 'CXPACKET') return w.indexOf('CXPACKET') !== -1 || w.indexOf('CXCONSUMER') !== -1;
    if (cat === 'ASYNC_NETWORK_IO') return w.indexOf('ASYNC_NETWORK_IO') !== -1;
    return true;
}

window.applyLocksWaitCategoryFilter = function() {
    const c = window._locksDrilldownCache;
    if (!c) return;
    const cat = document.getElementById('waitCategoryFilter')?.value || 'all';
    initWaitChartFromLiveWaits(c.liveWaits, cat, c.dashboardWaitStats);
};

window.loadBlockingData = async function(forceRefresh) {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;

    const liveBlockingEl = document.getElementById('live-blocking-table-container');
    const blockedEl = document.getElementById('blocked-sessions-table-container');
    const headEl = document.getElementById('blocking-table-container');
    const summaryEl = document.getElementById('locks-summary-strip');

    if (!liveBlockingEl || !blockedEl || !headEl) return;

    if (forceRefresh) {
        window._locksDrilldownCache = null;
        document.querySelectorAll('.locks-waits-fallback-note').forEach(n => n.remove());
        const spin = '<div style="display:flex; justify-content:center; align-items:center; min-height:100px;"><div class="spinner"></div><span style="margin-left:1rem;">Loading…</span></div>';
        liveBlockingEl.innerHTML = spin;
        blockedEl.innerHTML = spin;
        headEl.innerHTML = spin;
        if (summaryEl) summaryEl.innerHTML = '';
    }

    try {
        const enc = encodeURIComponent(inst.name);
        const [dashRes, blockRes, waitsRes] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/mssql/dashboard?instance=${enc}`),
            window.apiClient.authenticatedFetch(`/api/live/blocking?instance=${enc}`),
            window.apiClient.authenticatedFetch(`/api/live/waits?instance=${enc}`)
        ]);

        let dashboard = {};
        if (dashRes.ok) {
            const ct = dashRes.headers.get('content-type') || '';
            if (ct.includes('application/json')) dashboard = await dashRes.json();
        }

        let liveBlocking = [];
        let blockErr = null;
        if (blockRes.ok) {
            const bj = await blockRes.json();
            if (bj.success) liveBlocking = bj.data || [];
            else blockErr = bj.error || 'Blocking API error';
        } else {
            blockErr = `HTTP ${blockRes.status}`;
        }

        let liveWaits = [];
        let waitsErr = null;
        if (waitsRes.ok) {
            const wj = await waitsRes.json();
            if (wj.success) liveWaits = wj.data || [];
            else waitsErr = wj.error || 'Waits API error';
        } else {
            waitsErr = `HTTP ${waitsRes.status}`;
        }

        const activeBlocks = dashboard.active_blocks || [];
        const victims = activeBlocks.filter(s => (s.blocking_session_id || 0) !== 0);
        const waitStats = dashboard.wait_stats || [];

        window._locksDrilldownCache = {
            liveWaits,
            dashboardWaitStats: waitStats,
            liveBlocking,
            activeBlocks,
            blockErr,
            waitsErr
        };

        if (summaryEl) {
            const topWait = (liveWaits && liveWaits.length) ? liveWaits[0] : null;
            const topLabel = topWait ? (topWait.wait_type || '—') : '—';
            const topMs = topWait ? Number(topWait.wait_time_ms || 0) : 0;
            summaryEl.innerHTML = `
                <div class="glass-panel" style="padding:0.75rem; text-align:center;">
                    <div class="text-muted" style="font-size:0.65rem;">Lead blockers (live)</div>
                    <div class="metric-value" style="font-size:1.5rem;">${liveBlocking.length}</div>
                </div>
                <div class="glass-panel" style="padding:0.75rem; text-align:center;">
                    <div class="text-muted" style="font-size:0.65rem;">Blocked sessions (snapshot)</div>
                    <div class="metric-value" style="font-size:1.5rem; color:${victims.length ? 'var(--danger)' : 'inherit'};">${victims.length}</div>
                </div>
                <div class="glass-panel" style="padding:0.75rem; text-align:center;">
                    <div class="text-muted" style="font-size:0.65rem;">Wait types (live sample)</div>
                    <div class="metric-value" style="font-size:1.5rem;">${liveWaits.length}</div>
                </div>
                <div class="glass-panel" style="padding:0.75rem; text-align:center;">
                    <div class="text-muted" style="font-size:0.65rem;">Top wait (filtered chart uses category)</div>
                    <div style="font-size:0.75rem; font-weight:600;" title="${window.escapeHtml(topLabel)}">${window.escapeHtml(topLabel.length > 22 ? topLabel.substring(0, 22) + '…' : topLabel)}</div>
                    <div class="text-muted" style="font-size:0.65rem;">${topMs.toLocaleString()} ms total</div>
                </div>`;
        }

        // Live blocking chains table
        if (blockErr) {
            liveBlockingEl.innerHTML = `<div class="alert alert-warning m-3"><i class="fa-solid fa-triangle-exclamation"></i> Live blocking unavailable: ${window.escapeHtml(blockErr)}</div>`;
        } else if (liveBlocking.length === 0) {
            liveBlockingEl.innerHTML = `
                <table class="data-table">
                    <thead>
                        <tr><th>Lead SPID</th><th>Blocker login</th><th>Blocker app</th><th>Victims</th><th>Max wait (ms)</th></tr>
                    </thead>
                    <tbody>
                        <tr><td colspan="5" class="text-center text-muted">No blocking chains detected right now</td></tr>
                    </tbody>
                </table>`;
        } else {
            liveBlockingEl.innerHTML = `
                <table class="data-table">
                    <thead>
                        <tr><th>Lead SPID</th><th>Blocker login</th><th>Blocker app</th><th>Victims</th><th>Max wait (ms)</th></tr>
                    </thead>
                    <tbody>
                        ${liveBlocking.map(b => `
                            <tr>
                                <td><span class="badge badge-danger">${b.Lead_Blocker != null ? b.Lead_Blocker : '—'}</span></td>
                                <td>${window.escapeHtml(b.Blocker_Login || 'Unknown')}</td>
                                <td style="max-width:140px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(b.Blocker_App || '')}">${window.escapeHtml(b.Blocker_App || 'Unknown')}</td>
                                <td><span class="badge badge-warning">${b.Total_Victims != null ? b.Total_Victims : 0}</span></td>
                                <td class="text-danger">${(b.Max_Wait_Time_ms != null ? Number(b.Max_Wait_Time_ms) : 0).toLocaleString()}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>`;
        }

        // Blocked sessions (victims) from snapshot
        const sortedVictims = [...victims].sort((a, b) => (b.wait_time_ms || 0) - (a.wait_time_ms || 0));
        if (sortedVictims.length === 0) {
            blockedEl.innerHTML = `
                <table class="data-table">
                    <thead>
                        <tr><th>Session</th><th>Blocked by</th><th>Wait</th><th>Wait (ms)</th><th>Status</th><th>DB / Program</th><th>Query preview</th></tr>
                    </thead>
                    <tbody>
                        <tr><td colspan="7" class="text-center text-muted">No blocked sessions in the latest dashboard snapshot</td></tr>
                    </tbody>
                </table>`;
        } else {
            window.appState.queryCache = window.appState.queryCache || {};
            blockedEl.innerHTML = `
                <table class="data-table">
                    <thead>
                        <tr><th>Session</th><th>Blocked by</th><th>Wait</th><th>Wait (ms)</th><th>Status</th><th>DB / Program</th><th>Query preview</th></tr>
                    </thead>
                    <tbody>
                        ${sortedVictims.map((s, i) => {
                            const sid = s.session_id != null ? s.session_id : i;
                            const qt = s.query_text || '';
                            window.appState.queryCache['lock_victim_' + i] = qt;
                            const short = qt ? (qt.length > 90 ? qt.substring(0, 90) + '…' : qt) : 'N/A';
                            const db = s.database_name || '—';
                            const prog = s.program_name || '—';
                            return `<tr>
                                <td><strong>${sid}</strong></td>
                                <td><span class="badge badge-warning">${s.blocking_session_id != null ? s.blocking_session_id : '—'}</span></td>
                                <td><span class="badge badge-outline">${window.escapeHtml(s.wait_type || '—')}</span></td>
                                <td>${(s.wait_time_ms != null ? Number(s.wait_time_ms) : 0).toLocaleString()}</td>
                                <td>${window.escapeHtml(s.status || '—')}</td>
                                <td style="max-width:160px; font-size:0.75rem;" title="${window.escapeHtml(db + ' / ' + prog)}">${window.escapeHtml(db)}<br/><span class="text-muted">${window.escapeHtml(prog.length > 40 ? prog.substring(0, 40) + '…' : prog)}</span></td>
                                <td style="max-width:260px; font-size:0.7rem;">
                                    <span class="code-snippet" style="cursor:pointer; display:inline-block; max-width:240px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;"
                                          title="${window.escapeHtml(qt)}"
                                          onclick="window.showQueryModalDirect(window.appState.queryCache['lock_victim_${i}'])">${window.escapeHtml(short)}</span>
                                </td>
                            </tr>`;
                        }).join('')}
                    </tbody>
                </table>`;
        }

        // Head blockers aggregated (original drilldown logic)
        const blockers = activeBlocks.filter(s => (s.blocking_session_id || 0) !== 0);
        const leadBlockers = [];
        const seenSpids = new Set();
        blockers.forEach(b => {
            const bsid = b.blocking_session_id;
            if (bsid == null || seenSpids.has(bsid)) return;
            seenSpids.add(bsid);
            const blockedCount = blockers.filter(bb => bb.blocking_session_id === bsid).length;
            leadBlockers.push({
                spid: bsid,
                blocked_count: blockedCount,
                wait_type: b.wait_type,
                login: b.login_name,
                host: b.host_name,
                program: b.program_name,
                query_text: b.query_text,
                resource: b.resource
            });
        });

        if (leadBlockers.length === 0) {
            headEl.innerHTML = `
                <table class="data-table">
                    <thead>
                        <tr><th>Head SPID</th><th>Program / User</th><th>Wait</th><th>Blocked</th><th>Resource</th><th>Query preview</th></tr>
                    </thead>
                    <tbody>
                        <tr><td colspan="6" class="text-center text-muted">No head blockers in snapshot (see live chains above if blocking exists)</td></tr>
                    </tbody>
                </table>`;
        } else {
            window.appState.queryCache = window.appState.queryCache || {};
            headEl.innerHTML = `
                <table class="data-table">
                    <thead>
                        <tr><th>Head SPID</th><th>Program / User</th><th>Wait</th><th>Blocked</th><th>Resource</th><th>Query preview</th></tr>
                    </thead>
                    <tbody>
                        ${leadBlockers.map(b => `
                            <tr>
                                <td><strong><i class="fa-solid fa-link text-warning"></i> ${b.spid}</strong></td>
                                <td>${window.escapeHtml(b.program || 'N/A')} <br/><span class="text-muted">${window.escapeHtml(b.login || 'N/A')}</span></td>
                                <td><span class="badge badge-danger">${window.escapeHtml(b.wait_type || 'N/A')}</span></td>
                                <td><span class="text-danger font-bold">${b.blocked_count}</span></td>
                                <td style="font-size:0.75rem;">${window.escapeHtml(b.resource || 'N/A')}</td>
                                <td style="font-size:0.7rem; max-width:260px;">
                                    <span class="code-snippet" style="cursor:pointer; display:inline-block; max-width:240px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;"
                                          title="${window.escapeHtml(b.query_text || '')}"
                                          onclick="window.showQueryModalDirect(window.appState.queryCache['lock_${b.spid}'])">
                                        ${window.escapeHtml((b.query_text || 'N/A').substring(0, 80))}${(b.query_text && b.query_text.length > 80) ? '…' : ''}
                                    </span>
                                </td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>`;
            leadBlockers.forEach(b => { window.appState.queryCache['lock_' + b.spid] = b.query_text || ''; });
        }

        const cat = document.getElementById('waitCategoryFilter')?.value || 'all';
        initWaitChartFromLiveWaits(liveWaits, cat, waitStats);
        const chartCard = document.getElementById('drillLockChart')?.closest('.chart-card');
        if (waitsErr && (!liveWaits || liveWaits.length === 0) && chartCard) {
            if (!chartCard.querySelector('.locks-waits-fallback-note')) {
                const note = document.createElement('div');
                note.className = 'alert alert-warning m-2 locks-waits-fallback-note';
                note.style.fontSize = '0.78rem';
                note.innerHTML = `<i class="fa-solid fa-circle-info"></i> Live waits unavailable (${window.escapeHtml(waitsErr)}). Chart uses cumulative <code>wait_stats</code> from the dashboard if present.`;
                chartCard.appendChild(note);
            }
        } else if (chartCard) {
            chartCard.querySelectorAll('.locks-waits-fallback-note').forEach(n => n.remove());
        }
    } catch (error) {
        appDebug('Failed to load blocking data:', error);
        const msg = window.escapeHtml(String(error && error.message ? error.message : error));
        const errHtml = `<div class="alert alert-danger m-3">Failed to load: ${msg}</div>`;
        liveBlockingEl.innerHTML = errHtml;
        blockedEl.innerHTML = errHtml;
        headEl.innerHTML = errHtml;
    }
};

function initWaitChartFromLiveWaits(liveWaits, category, fallbackWaitStats) {
    if (window.currentCharts && window.currentCharts.drillLck) {
        window.currentCharts.drillLck.destroy();
        window.currentCharts.drillLck = null;
    }

    const canvas = document.getElementById('drillLockChart');
    const chartHost = canvas?.parentElement;
    if (chartHost) {
        chartHost.querySelectorAll('.locks-chart-overlay').forEach(n => n.remove());
    }

    let rows = (liveWaits || []).filter(w => locksWaitCategoryMatches(w.wait_type, category));
    let labelKey = 'wait_type';
    let valueKey = 'wait_time_ms';

    if (rows.length === 0 && fallbackWaitStats && fallbackWaitStats.length) {
        rows = fallbackWaitStats.filter(w => locksWaitCategoryMatches(w.wait_type, category));
        labelKey = 'wait_type';
        valueKey = 'wait_time_ms';
    }

    rows = [...rows].sort((a, b) => Number(b[valueKey] || 0) - Number(a[valueKey] || 0)).slice(0, 12);

    if (rows.length === 0) {
        if (chartHost) {
            const d = document.createElement('div');
            d.className = 'locks-chart-overlay';
            d.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;color:var(--text-muted);font-size:0.85rem;padding:1rem;text-align:center;';
            d.textContent = 'No wait data for this filter. Try “All waits” or refresh.';
            chartHost.appendChild(d);
        }
        return;
    }

    const labels = rows.map(w => w[labelKey] || 'Unknown');
    const values = rows.map(w => Number(w[valueKey] || 0));
    const ctx = canvas?.getContext('2d');
    if (!ctx) return;

    window.currentCharts = window.currentCharts || {};
    window.currentCharts.drillLck = new Chart(ctx, {
        type: 'bar',
        data: {
            labels,
            datasets: [{
                label: 'Wait time (ms)',
                data: values,
                backgroundColor: 'rgba(239, 68, 68, 0.7)',
                borderColor: 'rgba(239, 68, 68, 1)',
                borderWidth: 1
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false },
                tooltip: {
                    callbacks: {
                        afterLabel: (items) => {
                            const i = items.dataIndex;
                            const r = rows[i];
                            if (r && r.waiting_tasks_count != null) {
                                return 'Waiting tasks: ' + r.waiting_tasks_count;
                            }
                            return '';
                        }
                    }
                }
            },
            scales: {
                y: { beginAtZero: true, title: { display: true, text: 'ms' } },
                x: { ticks: { maxRotation: 45, minRotation: 45 } }
            }
        }
    });
}
