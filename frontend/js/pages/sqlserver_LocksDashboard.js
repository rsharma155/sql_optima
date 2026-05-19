/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * File: sqlserver_LocksDashboard.js
 * Purpose: SQL Server Blocking & Deadlock Analysis Dashboard Logic (Compact & High-Density).
 * Metadata:
 *   - Version: 1.7
 *   - Screen Target: 1080p (No Scroll)
 *   - Features: In-grid Filtering, De-duplicated Active Victims, Node Drilldown Action
 *   - Fix: Restored missing chart functions (updateMsTopQueriesChart, updateMsBlockedDbsChart)
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

let msDashboardState = {
    timelineMetric: 'incidents',
    sortColumn: 'wait_duration_ms',
    sortDir: 'desc',
    gridFilters: {}, // Dynamic filters from table row
    rawDetailsData: [],
    timelineData: []
};

// Global navigation helper for drilldown
window.navigate = function(route, params) {
    window.appState.routeParams = params;
    window.appNavigate(route);
};

window.sqlserver_LocksDashboard = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    const dbName = window.appState.currentDatabase || 'all';

    // 1. Initial Render
    window.routerOutlet.innerHTML = await window.loadTemplate('pages/sqlserver_locks.html', { inst, dbName });

    // 2. Initialize Components
    if (typeof window.initPageTimePicker === 'function') {
        window.initPageTimePicker();
    }

    // 3. Setup UI Event Listeners
    setupMsDashboardListeners();

    // 4. Load Data
    await refreshMsLocksDashboard();

    // 5. Auto-refresh based on dynamic collector interval
    const dynInterval = window.collectorConfig ? window.collectorConfig.getInterval("sqlserver_blocking", 15000) : 15000;
    window.registerInterval(() => {
        if (document.querySelector('.sqlserver-locks-page')) {
            refreshMsLocksDashboard();
        }
    }, dynInterval);
};

function setupMsDashboardListeners() {
    // Universal Tab Switching
    document.querySelectorAll('.qa-tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const container = btn.closest('.glass-panel, .table-card');
            if (!container) return;
            container.querySelectorAll('.qa-tab-btn').forEach(b => b.classList.remove('active'));
            container.querySelectorAll('.qa-tab-panel').forEach(p => p.style.display = 'none');
            
            btn.classList.add('active');
            const target = document.getElementById(btn.dataset.tab);
            if (target) target.style.display = 'block';
        });
    });

    // Timeline metric toggle
    document.querySelectorAll('.js-timeline-toggle').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.js-timeline-toggle').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            msDashboardState.timelineMetric = btn.dataset.metric;
            updateMsLocksTimeline(msDashboardState.timelineData);
        });
    });

    // In-grid Filtering Logic
    window.routerOutlet.addEventListener('input', (e) => {
        const filterInput = e.target.closest('.filter-row input');
        if (filterInput) {
            const field = filterInput.dataset.filter;
            const val = filterInput.value.toLowerCase();
            if (!val) delete msDashboardState.gridFilters[field];
            else msDashboardState.gridFilters[field] = val;
            updateMsLocksDetails(msDashboardState.rawDetailsData);
        }
    });

    // Row 2 sorting
    document.querySelectorAll('#msActiveBlockedSessionsTableShort th.sortable').forEach(th => {
        th.addEventListener('click', () => {
            const col = th.dataset.sort;
            msDashboardState.sortDir = (msDashboardState.sortColumn === col && msDashboardState.sortDir === 'desc') ? 'asc' : 'desc';
            msDashboardState.sortColumn = col;
            updateMsLocksDetails(msDashboardState.rawDetailsData);
        });
    });

    // Event Delegation
    document.addEventListener('click', (e) => {
        // Only handle clicks within our dashboard
        if (!document.querySelector('.sqlserver-locks-page')) return;

        const qBtn = e.target.closest('.js-view-query');
        if (qBtn && qBtn.dataset.query) {
            e.stopPropagation();
            window.viewMsQueryDetail(qBtn.dataset.query);
            return;
        }
        
        const row = e.target.closest('#msActiveBlockedSessionsTableShort tbody tr');
        if (row && row.dataset.spid) {
            highlightTreeNode(row.dataset.spid);
            return;
        }

        const ddBtn = e.target.closest('.js-dd-spid');
        if (ddBtn && ddBtn.dataset.spid) {
            e.preventDefault();
            e.stopPropagation();
            const spid = ddBtn.dataset.spid;
            console.log('[LocksDashboard] Drilling down to SPID:', spid);
            if (typeof window.navigate === 'function') {
                window.navigate('sqlserver-locks-drilldown', { spid: spid });
            } else {
                window.appState.routeParams = { spid: spid };
                window.appNavigate('sqlserver-locks-drilldown');
            }
        }
    }, true); // Use capture phase to intercept before other listeners
}

window.refreshMsLocksDashboard = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;

    const from = window.appState.fromTs ? new Date(window.appState.fromTs).toISOString() : '';
    const to = window.appState.toTs ? new Date(window.appState.toTs).toISOString() : '';
    const queryParams = `?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`;

    try {
        const [kpiRes, timelineRes, detailsRes, deadlockRes, topQueriesRes, blockedDbsRes, blockedObjsRes] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/kpis?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/timeline${queryParams}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/details${queryParams}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/deadlocks/history${queryParams}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/top-queries${queryParams}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/most-blocked-databases${queryParams}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/blocking/most-blocked-objects${queryParams}`)
        ]);

        if (kpiRes.ok) updateMsLocksKPIs(await kpiRes.json());
        if (timelineRes.ok) {
            msDashboardState.timelineData = await timelineRes.json();
            updateMsLocksTimeline(msDashboardState.timelineData);
        }
        if (detailsRes.ok) {
            msDashboardState.rawDetailsData = await detailsRes.json();
            updateMsLocksDetails(msDashboardState.rawDetailsData);
        }
        if (deadlockRes.ok) {
            const deadlockData = await deadlockRes.json();
            const events = deadlockData.events || [];
            updateMsDeadlockHistory(events, deadlockData.enabled, deadlockData.message);
            updateMsDeadlockTimeline(events);
        }
        if (topQueriesRes.ok) {
            const topQueriesData = await topQueriesRes.json();
            updateMsTopQueriesChart(topQueriesData);
            updateMsTopQueriesTable(topQueriesData);
        }
        if (blockedDbsRes.ok) updateMsBlockedDbsChart(await blockedDbsRes.json());
        if (blockedObjsRes.ok) {
            const blockedObjsData = await blockedObjsRes.json();
            updateMsBlockedObjsTable(blockedObjsData);
            updateMsMiniBlockedObjsTable(blockedObjsData);
        }
        const updEl = document.getElementById('msBlockingLastUpdated');
        if (updEl) updEl.textContent = 'Updated ' + new Date().toLocaleTimeString();
    } catch (err) {
        console.error("Refresh failed:", err);
    }
};

function getDurationColor(ms) {
    if (ms >= 30000) return '#ef4444';
    if (ms >= 10000) return '#f59e0b';
    return 'inherit';
}

function updateMsLocksKPIs(kpis) {
    if (!kpis) return;
    const activeBlocked = kpis.active_blocked_sessions || 0;
    document.getElementById('msKpiActiveBlocked').textContent = activeBlocked;
    document.getElementById('msKpiRootBlockers').textContent = kpis.root_blockers || 0;
    const lwMs = kpis.max_wait_duration_ms || 0;
    const lwEl = document.getElementById('msKpiLongestWait');
    lwEl.textContent = `${Math.floor(lwMs/60000).toString().padStart(2,'0')}:${Math.floor((lwMs%60000)/1000).toString().padStart(2,'0')}`;
    lwEl.style.color = getDurationColor(lwMs);
    document.getElementById('msKpiOpenTransactions').textContent = kpis.open_tran_over_1min || 0;
    document.getElementById('msKpiDeadlocks').textContent = kpis.deadlocks_24h || 0;
    document.getElementById('msKpiIncidents24h').textContent = kpis.blocking_incidents_24h || 0;

    // Live blocking badge in page header
    const liveBadge = document.getElementById('msLiveBlockingBadge');
    if (liveBadge) liveBadge.style.display = activeBlocked > 0 ? 'inline-flex' : 'none';
}

let msBlockingChart = null;
function updateMsLocksTimeline(data) {
    if (!Array.isArray(data)) data = [];
    const ctx = document.getElementById('msBlockingTimelineChart').getContext('2d');
    if (msBlockingChart) msBlockingChart.destroy();
    
    const labels = data.map(d => new Date(d.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
    let values, color, label;
    if (msDashboardState.timelineMetric === 'incidents') {
        values = data.map(d => d.blocked_sessions || 0);
        color  = '#ef4444'; label = 'Blocked Sessions';
    } else {
        values = data.map(d => ((d.total_wait_ms || 0) / 1000).toFixed(1));
        color  = '#3b82f6'; label = 'Total Wait (s)';
    }

    msBlockingChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels,
            datasets: [{ label, data: values, borderColor: color, backgroundColor: color+'1A', fill: true, tension: 0.4, pointRadius: 0 }]
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: '#94a3b8', font: { size: 8 } } },
                x: { grid: { display: false }, ticks: { color: '#94a3b8', font: { size: 8 }, maxTicksLimit: 6 } }
            }
        }
    });
}

function updateMsLocksDetails(data) {
    const detailTbody = document.querySelector('#msActiveBlockingTable tbody');
    const shortTbody = document.querySelector('#msActiveBlockedSessionsTableShort tbody');

    if (!Array.isArray(data) || data.length === 0) {
        if (detailTbody) detailTbody.innerHTML = '<tr><td colspan="10" class="text-center text-muted">No blocking sessions in selected window.</td></tr>';
        if (shortTbody) shortTbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted" style="font-size:0.6rem;">All Clear</td></tr>';
        renderMsBlockingTree([]);
        const badge = document.getElementById('msChainCountBadge');
        if (badge) badge.style.display = 'none';
        return;
    }

    // 1. DEDUPLICATION: Take the latest state for each SPID within a 70s window of the most recent activity
    
    // Sort by TS descending
    const sorted = [...data].sort((a, b) => new Date(b.ts) - new Date(a.ts));
    const maxTs = new Date(sorted[0].ts).getTime();
    const windowMs = 70000; // 70s window to catch sessions not re-logged due to 60s delta interval

    const currentSnapsMap = new Map();
    sorted.forEach(s => {
        const sTs = new Date(s.ts).getTime();
        // If we already have a later entry for this SPID, skip
        if (currentSnapsMap.has(s.session_id)) return;
        
        // Only include if it's part of the "current" incident window
        if (maxTs - sTs <= windowMs) {
            currentSnapsMap.set(s.session_id, s);
        }
    });
    
    const currentSnaps = Array.from(currentSnapsMap.values());
    renderMsBlockingTree(currentSnaps);

    // 2. Apply Grid Filters
    const applyFilters = (list) => {
        return list.filter(r => {
            for (const field in msDashboardState.gridFilters) {
                const filterVal = msDashboardState.gridFilters[field];
                const itemVal = String(r[field] || '').toLowerCase();
                
                // Numeric "greater than" filter for certain fields
                if (field === 'wait_duration_ms' || field === 'total_wait_duration_ms' || field === 'total_wait') {
                    if (parseFloat(r[field]) < parseFloat(filterVal)) return false;
                } else {
                    if (!itemVal.includes(filterVal)) return false;
                }
            }
            return true;
        });
    };

    const filtered = applyFilters(data);

    // 3. Sort Row 2 Table
    const victims = currentSnaps.filter(s => s.blocking_session_id > 0);
    victims.sort((a, b) => {
        const valA = a[msDashboardState.sortColumn], valB = b[msDashboardState.sortColumn];
        return msDashboardState.sortDir === 'asc' ? (valA > valB ? 1 : -1) : (valA < valB ? 1 : -1);
    });

    // Update chain count badge (header of right panel)
    const badge = document.getElementById('msChainCountBadge');
    if (badge) {
        badge.textContent = currentSnaps.length;
        badge.style.display = currentSnaps.length > 0 ? 'inline-block' : 'none';
    }

    // Short table: 4 cols — SPID, Blocker, Wait, Wait Type
    if (shortTbody) shortTbody.innerHTML = victims.map(s => `
        <tr data-spid="${s.session_id}" style="cursor:pointer;">
            <td><span class="badge badge-danger">#${s.session_id}</span></td>
            <td><strong style="color:var(--accent);">${s.blocking_session_id}</strong></td>
            <td style="font-weight:700; color:${getDurationColor(s.wait_duration_ms)}">${(s.wait_duration_ms/1000).toFixed(1)}s</td>
            <td style="color:var(--text-muted); font-size:0.5rem;">${(s.wait_type||'').substring(0,12)}</td>
        </tr>
    `).join('') || '<tr><td colspan="4" class="text-center text-muted" style="font-size:0.6rem;">All Clear</td></tr>';

    // Detail table: 10 cols — Time, SPID, Blocker, Wait Type, Wait, DB, Login, Isolation, Query, Actions
    if (detailTbody) detailTbody.innerHTML = filtered.slice(0, 100).map(r => {
        const isVictim = r.blocking_session_id > 0;
        const sevClass = r.wait_duration_ms >= 30000 ? 'row-sev-high' : r.wait_duration_ms >= 10000 ? 'row-sev-med' : '';
        const isoClass = getIsoClass(r.transaction_isolation_level);
        return `
        <tr class="${sevClass}">
            <td class="text-muted" style="white-space:nowrap;">${new Date(r.ts).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit', second:'2-digit'})}</td>
            <td><span class="badge ${isVictim ? 'badge-danger' : 'badge-warning'}">#${r.session_id}</span></td>
            <td>${isVictim ? `<strong style="color:var(--accent);">${r.blocking_session_id}</strong>` : '<span style="color:var(--text-muted);">—</span>'}</td>
            <td style="color:var(--accent); font-size:0.55rem; white-space:nowrap;">${r.wait_type || '—'}</td>
            <td style="font-weight:700; color:${getDurationColor(r.wait_duration_ms)}; white-space:nowrap;">${(r.wait_duration_ms/1000).toFixed(1)}s</td>
            <td>${r.database_name || '—'}</td>
            <td title="${r.login_name}">${(r.login_name||'').substring(0,14)}</td>
            <td><span class="iso-badge ${isoClass}">${r.transaction_isolation_level || '—'}</span></td>
            <td><span class="code-snippet js-view-query" data-query="${window.escapeHtml(r.query_text)}" title="${window.escapeHtml(r.query_text)}">${window.escapeHtml((r.query_text||'—').substring(0, 28))}</span></td>
            <td class="text-center" style="white-space:nowrap;">
                <button class="btn btn-xs btn-outline-accent js-dd-spid" data-spid="${r.session_id}" title="Deep Investigation">
                    <i class="fa-solid fa-magnifying-glass-chart"></i>
                </button>
            </td>
        </tr>`;
    }).join('') || '<tr><td colspan="10" class="text-center text-muted">No matches.</td></tr>';
}

function getIsoClass(level) {
    if (!level) return '';
    const l = level.toLowerCase();
    if (l.includes('readcommitted') || l === 'readcommitted') return 'iso-rc';
    if (l.includes('readuncommitted') || l === 'readuncommitted') return 'iso-ru';
    if (l.includes('serializable')) return 'iso-ser';
    if (l.includes('snapshot')) return 'iso-snap';
    if (l.includes('repeatable')) return 'iso-rep';
    return '';
}

function renderMsBlockingTree(snaps) {
    const treeList = document.getElementById('msBlockingTree');
    if (!treeList) return;
    treeList.innerHTML = '';

    const depthBadge = document.getElementById('msTreeDepthBadge');
    const depthVal   = document.getElementById('msTreeDepthVal');

    if (!snaps.length) {
        treeList.innerHTML = '<li style="color:var(--text-muted); font-style:italic; font-size:0.58rem; padding:1.2rem;">No active blocking detected.</li>';
        const panel = document.getElementById('msTreeDetailPanel');
        if (panel) panel.innerHTML = `<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;height:100%;color:var(--text-muted);"><i class="fa-solid fa-arrow-pointer" style="font-size:1.2rem;margin-bottom:0.35rem;opacity:0.2;"></i><p style="font-size:0.58rem;">Click a node to view details.</p></div>`;
        if (depthBadge) depthBadge.style.display = 'none';
        return;
    }

    // Build session map + tree
    const sessions = {};
    snaps.forEach(s => sessions[s.session_id] = { ...s, children: [] });
    const roots = [];
    snaps.forEach(s => {
        if (s.blocking_session_id === 0 || !sessions[s.blocking_session_id]) roots.push(sessions[s.session_id]);
        else sessions[s.blocking_session_id].children.push(sessions[s.session_id]);
    });

    // Max wait for proportional progress bars
    const maxWait = Math.max(...snaps.map(s => s.wait_duration_ms || 0), 1);

    const buildNode = (node) => {
        const isRoot    = node.blocking_session_id === 0 || !sessions[node.blocking_session_id];
        const waitMs    = node.wait_duration_ms || 0;
        const waitSec   = isRoot ? '—' : (waitMs / 1000).toFixed(1) + 's';
        const waitColor = waitMs >= 30000 ? '#ef4444' : waitMs >= 10000 ? '#f97316' : waitMs >= 3000 ? '#f59e0b' : '#22c55e';
        const barPct    = isRoot ? 0 : Math.min(100, (waitMs / maxWait) * 100);
        const waitType  = (node.wait_type || '').substring(0, 13);
        const login     = (node.login_name || 'N/A').substring(0, 15);

        const li  = document.createElement('li');
        const div = document.createElement('div');
        div.className = `ms-tree-node ${isRoot ? 'root-blocker' : 'waiting'}`;
        div.id = `tree-node-${node.session_id}`;

        div.innerHTML = `
            <div class="ms-node-hdr ${isRoot ? 'hdr-blocker' : 'hdr-victim'}">
                <span class="ms-node-spid">#${node.session_id}</span>
                <span class="ms-node-role ${isRoot ? 'role-blocker' : 'role-victim'}">${isRoot ? 'BLKR' : 'VIC'}</span>
            </div>
            <div class="ms-node-body">
                <div class="ms-node-login" title="${window.escapeHtml(node.login_name || '')}">${window.escapeHtml(login)}</div>
                <div class="ms-node-wait-row">
                    <span class="ms-node-wait-val" style="color:${waitColor};">${waitSec}</span>
                    <span class="ms-node-wait-type" title="${window.escapeHtml(node.wait_type || '')}">${window.escapeHtml(waitType)}</span>
                </div>
                <div class="ms-node-bar-wrap">
                    <div class="ms-node-bar" style="width:${barPct.toFixed(1)}%; background:${waitColor};"></div>
                </div>
            </div>
            <div class="ms-node-action-overlay">
                <button class="ms-drill-btn js-dd-spid" data-spid="${node.session_id}" title="Deep Investigation">
                    <i class="fa-solid fa-magnifying-glass-plus"></i>
                </button>
            </div>
        `;

        div.onclick = (e) => {
            if (e.target.closest('.ms-drill-btn')) return;
            e.stopPropagation();
            document.querySelectorAll('.ms-tree-node').forEach(n => n.classList.remove('active'));
            div.classList.add('active');
            showMsTreeDetails(node);
        };

        li.appendChild(div);
        if (node.children.length) {
            const ul = document.createElement('ul');
            node.children.forEach(c => ul.appendChild(buildNode(c)));
            li.appendChild(ul);
        }
        return li;
    };

    roots.forEach(r => treeList.appendChild(buildNode(r)));

    // Chain depth badge
    const calcDepth = (node, d = 0) => node.children.length ? Math.max(...node.children.map(c => calcDepth(c, d + 1))) : d;
    const maxDepth  = roots.reduce((mx, r) => Math.max(mx, calcDepth(r)), 0);
    if (depthBadge && depthVal) {
        depthVal.textContent    = maxDepth + 1;
        depthBadge.style.display = maxDepth > 0 ? 'inline-block' : 'none';
    }

    // Auto-click first root node to populate SPID details panel
    const firstNode = document.querySelector('.ms-tree-node');
    if (firstNode) firstNode.click();
}

function highlightTreeNode(spid) {
    const node = document.getElementById(`tree-node-${spid}`);
    if (node) { node.scrollIntoView({ behavior: 'smooth', block: 'center' }); node.click(); }
}

function showMsTreeDetails(node) {
    const panel = document.getElementById('msTreeDetailPanel');
    if (!panel) return;
    const isRoot = node.blocking_session_id === 0;
    const isoClass = getIsoClass(node.transaction_isolation_level);
    panel.innerHTML = `
        <div style="display:flex; flex-direction:column; gap:0.4rem;">
            <div style="display:flex; justify-content:space-between; align-items:flex-start;">
                <div>
                    <div style="font-size:1rem; font-weight:900; color:var(--text-primary); line-height:1;">#${node.session_id}</div>
                    <span class="badge ${isRoot ? 'badge-danger' : 'badge-warning'}" style="font-size:0.42rem;">${isRoot ? '⬛ ROOT BLOCKER' : '▶ ' + (node.wait_type||'WAITING')}</span>
                </div>
                <span class="iso-badge ${isoClass}" style="margin-top:2px;">${node.transaction_isolation_level || '—'}</span>
            </div>
            <div style="display:grid; grid-template-columns:1fr 1fr; gap:3px;">
                <div style="background:var(--bg-tertiary); border:1px solid var(--border-color); border-radius:3px; padding:3px 5px;">
                    <div style="font-size:0.4rem; color:var(--text-muted); text-transform:uppercase;">Wait</div>
                    <div style="font-size:0.62rem; font-weight:800; color:${getDurationColor(node.wait_duration_ms)}">${(node.wait_duration_ms/1000).toFixed(1)}s</div>
                </div>
                <div style="background:var(--bg-tertiary); border:1px solid var(--border-color); border-radius:3px; padding:3px 5px;">
                    <div style="font-size:0.4rem; color:var(--text-muted); text-transform:uppercase;">DB</div>
                    <div style="font-size:0.58rem; font-weight:700; color:var(--accent); overflow:hidden; text-overflow:ellipsis;">${node.database_name || '—'}</div>
                </div>
            </div>
            <div style="display:grid; grid-template-columns:1fr 1fr; gap:3px;">
                <div style="background:var(--bg-tertiary); border:1px solid var(--border-color); border-radius:3px; padding:3px 5px;">
                    <div style="font-size:0.4rem; color:var(--text-muted); text-transform:uppercase;">Login</div>
                    <div style="font-size:0.58rem; font-weight:700; overflow:hidden; text-overflow:ellipsis;" title="${node.login_name}">${node.login_name || '—'}</div>
                </div>
                <div style="background:var(--bg-tertiary); border:1px solid var(--border-color); border-radius:3px; padding:3px 5px;">
                    <div style="font-size:0.4rem; color:var(--text-muted); text-transform:uppercase;">Host</div>
                    <div style="font-size:0.58rem; font-weight:700; overflow:hidden; text-overflow:ellipsis;" title="${node.host_name}">${node.host_name || '—'}</div>
                </div>
            </div>
            <div>
                <div style="font-size:0.42rem; font-weight:800; text-transform:uppercase; color:var(--text-muted); margin-bottom:3px;">Active Query</div>
                <div style="background:rgba(15,23,42,0.25); border:1px solid var(--border-color); border-radius:3px; padding:4px 6px; max-height:70px; overflow-y:auto;">
                    <pre style="margin:0; font-family:'JetBrains Mono',monospace; font-size:0.5rem; color:var(--text-primary); white-space:pre-wrap; word-break:break-all; line-height:1.4;">${window.escapeHtml(node.query_text || '-- no query captured')}</pre>
                </div>
            </div>
            <div style="display:flex; gap:3px;">
                <button class="btn btn-xs btn-outline-accent js-view-query" data-query="${window.escapeHtml(node.query_text || '')}" style="flex:1; font-size:0.52rem;">
                    <i class="fa-solid fa-eye"></i> View SQL
                </button>
                <button class="btn btn-xs btn-danger js-dd-spid" data-spid="${node.session_id}" style="flex:1; font-size:0.52rem;">
                    <i class="fa-solid fa-magnifying-glass-chart"></i> Investigate
                </button>
            </div>
        </div>
    `;
    // Show/hide the header "Investigate" button
    const headerBtn = document.getElementById('btnInvestigateFromDetail');
    if (headerBtn) {
        headerBtn.style.display = 'inline-flex';
        headerBtn.onclick = () => window.navigate('sqlserver-locks-drilldown', { spid: node.session_id });
    }
}

function updateMsDeadlockHistory(events, enabled, statusMsg) {
    const tbody = document.querySelector('#msDeadlockHistoryTable tbody');
    if (!tbody) return;

    if (enabled === false) {
        tbody.innerHTML = `<tr><td colspan="5" class="text-center text-warning" style="font-size:0.7rem; padding:1rem;">${statusMsg}</td></tr>`;
        return;
    }

    tbody.innerHTML = (events || []).slice(0, 10).map(e => `
        <tr>
            <td class="text-muted" style="white-space:nowrap;">${new Date(e.ts).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'})}</td>
            <td title="${e.database_name}">${(e.database_name||'').substring(0,12)}</td>
            <td><span class="badge badge-danger">#${e.victim_session_id}</span></td>
            <td><span class="code-snippet js-view-query" data-query="${window.escapeHtml(e.victim_sql_text || e.victim_sql_hash || 'N/A')}" title="Click to view full SQL">${window.escapeHtml((e.victim_sql_text || e.victim_sql_hash || 'N/A').substring(0,24))}</span></td>
            <td class="text-center"><button class="btn btn-xs btn-outline-accent js-view-query" data-query="${window.escapeHtml(e.deadlock_graph)}" title="View XML Graph" style="font-size:0.5rem; padding:1px 4px;">XML</button></td>
        </tr>
    `).join('') || '<tr><td colspan="5" class="text-center text-muted">No deadlocks.</td></tr>';
}

let msDeadlockChart = null;
function updateMsDeadlockTimeline(data) {
    if (!Array.isArray(data)) data = [];
    const canvasEl = document.getElementById('msDeadlockTimelineChart');
    if (!canvasEl) return;
    const ctx = canvasEl.getContext('2d');
    if (msDeadlockChart) msDeadlockChart.destroy();
    const minuteMap = {};
    data.forEach(d => { const min = new Date(d.ts).setSeconds(0, 0); minuteMap[min] = (minuteMap[min] || 0) + 1; });
    const sortedMins = Object.keys(minuteMap).sort();
    msDeadlockChart = new Chart(ctx, {
        type: 'scatter',
        data: { datasets: [{ label: 'Deadlocks', data: sortedMins.map(m => ({ x: parseInt(m), y: minuteMap[m] })), backgroundColor: '#a855f7', pointRadius: 4 }] },
        options: {
            responsive: true, maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { stepSize: 1, color: '#94a3b8', font: { size: 8 } } },
                x: { type: 'linear', grid: { display: false }, ticks: { color: '#94a3b8', font: { size: 8 }, callback: (v) => new Date(v).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'}) } }
            }
        }
    });
}

function updateMsTopQueriesTable(data) {
    const tbody = document.querySelector('#msTopBlockingQueriesTable tbody');
    if (!tbody) return;
    tbody.innerHTML = (data || []).slice(0, 10).map(q => `
        <tr>
            <td class="font-mono">${(q.sql_hash || '').substring(0,10)}</td>
            <td><strong>${(q.total_wait_duration_ms/1000).toFixed(0)}s</strong></td>
            <td>${q.incident_count}</td>
            <td>${(q.avg_wait_duration_ms/1000).toFixed(1)}s</td>
            <td><span class="code-snippet js-view-query" data-query="${window.escapeHtml(q.query_text)}">${window.escapeHtml((q.query_text||'').substring(0, 20))}...</span></td>
        </tr>
    `).join('') || '<tr><td colspan="5" class="text-center text-muted">No data.</td></tr>';
}

function updateMsBlockedObjsTable(data) {
    const tbody = document.querySelector('#msBlockedObjectsTable tbody');
    if (!tbody) return;
    tbody.innerHTML = (data || []).slice(0, 10).map(o => `
        <tr>
            <td><strong>${o.object_name}</strong></td>
            <td><span class="badge badge-outline" style="font-size:0.5rem;">${o.lock_type}</span></td>
            <td>${(o.total_wait || 0).toLocaleString()}ms</td>
            <td>${o.incident_count}</td>
        </tr>
    `).join('') || '<tr><td colspan="4" class="text-center text-muted">No data.</td></tr>';
}

function updateMsMiniBlockedObjsTable(data) {
    const tbody = document.querySelector('#msMiniBlockedObjectsTable tbody');
    if (!tbody) return;
    tbody.innerHTML = (data || []).slice(0, 3).map(o => `
        <tr>
            <td title="${o.object_name}">${o.object_name.split('.').pop()}</td>
            <td>${o.lock_type}</td>
            <td>${(o.total_wait || 0).toLocaleString()}ms</td>
        </tr>
    `).join('') || '<tr><td colspan="3" class="text-center text-muted">No data</td></tr>';
}

let msTopQueriesChart = null;
function updateMsTopQueriesChart(data) {
    const topQEl = document.getElementById('msTopBlockingQueriesChart');
    if (!topQEl) return;
    const ctx = topQEl.getContext('2d');
    if (msTopQueriesChart) msTopQueriesChart.destroy();
    const labels = (data || []).slice(0, 5).map(q => (q.sql_hash || 'Unknown').substring(0, 8));
    const values = (data || []).slice(0, 5).map(q => q.total_wait_duration_ms / 1000);
    msTopQueriesChart = new Chart(ctx, {
        type: 'bar', data: { labels, datasets: [{ data: values, backgroundColor: '#3b82f6', borderRadius: 4 }] },
        options: { indexAxis: 'y', responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { x: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: '#94a3b8', font: { size: 8 } } }, y: { grid: { display: false }, ticks: { color: '#94a3b8', font: { size: 8 } } } } }
    });
}

let msBlockedDbsChart = null;
function updateMsBlockedDbsChart(data) {
    const dbsEl = document.getElementById('msMostBlockedDatabasesChart');
    if (!dbsEl) return;
    const ctx = dbsEl.getContext('2d');
    if (msBlockedDbsChart) msBlockedDbsChart.destroy();
    const labels = (data || []).slice(0, 5).map(d => d.database_name);
    const values = (data || []).slice(0, 5).map(d => d.incident_count);
    msBlockedDbsChart = new Chart(ctx, {
        type: 'bar', data: { labels, datasets: [{ data: values, backgroundColor: '#f59e0b', borderRadius: 4 }] },
        options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: '#94a3b8', font: { size: 8 } } }, x: { grid: { display: false }, ticks: { color: '#94a3b8', font: { size: 8 } } } } }
    });
}

window.viewMsQueryDetail = function(sql) {
    if (typeof window.showQueryModalDirect === 'function') window.showQueryModalDirect(sql);
    else alert(sql);
};
