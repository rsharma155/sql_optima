window.PgLocksView = async function() {
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/locks.html');
    setTimeout(initPgLocks, 50);
}

async function initPgLocks() {
    window.currentCharts = window.currentCharts || {};
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: ''};

    // Fetch locks data
    let locks = [];
    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/postgres/locks?instance=${encodeURIComponent(inst.name)}`
        );
        if (response.ok) {
            const contentType = response.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const data = await response.json();
                locks = data.locks || [];
            }
        } else {
            console.error("Failed to load PG locks:", response.status);
        }
    } catch (e) {
        console.error("PG locks fetch failed:", e);
    }

    // Populate the locks table
    const tbody = document.getElementById('locksTbody');
    if (tbody) {
        if (locks.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No locks detected. All clear!</td></tr>';
        } else {
            tbody.innerHTML = locks.map(lock => `
                <tr>
                    <td>${lock.pid || '-'}</td>
                    <td>${window.escapeHtml(lock.lock_type || '-')}</td>
                    <td>${window.escapeHtml(lock.relation || '-')}</td>
                    <td>${getLockModeBadge(lock.mode)}</td>
                    <td>${lock.granted ? '<span class="text-success font-bold">true</span>' : '<span class="text-danger font-bold">false</span>'}</td>
                    <td>${lock.waiting_for || '-'}</td>
                </tr>
            `).join('');
        }
    }

    // Fetch blocking tree data
    let blockingTree = [];
    try {
        const blockingResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/blocking-tree?instance=${encodeURIComponent(inst.name)}`
        );
        if (blockingResponse.ok) {
            const contentType = blockingResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const data = await blockingResponse.json();
                blockingTree = data.blocking_tree || [];
            }
        } else {
            console.error("Failed to load PG blocking tree:", blockingResponse.status);
        }
    } catch (e) {
        console.error("PG blocking tree fetch failed:", e);
    }

    renderBlockingTree(blockingTree);
    renderBlockingDetails(blockingTree);
    renderBlockingSummary(blockingTree);

    // Deadlock delta history (Timescale-backed). No dummy charts.
    let deadlockHist = null;
    try {
        const dlResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/deadlocks/history?instance=${encodeURIComponent(inst.name)}&window_minutes=180&limit=400`
        );
        if (dlResponse.ok) {
            const contentType = dlResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const dlData = await dlResponse.json();
                deadlockHist = dlData && dlData.history ? dlData.history : null;
            }
        }
    } catch (e) {
        console.error("PG deadlock history fetch failed:", e);
    }

    let lockWaitHist = null;
    try {
        const lwResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/locks/wait-history?instance=${encodeURIComponent(inst.name)}&window_minutes=180&limit=400`
        );
        if (lwResponse.ok) {
            const contentType = lwResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const lwData = await lwResponse.json();
                lockWaitHist = lwData && lwData.history ? lwData.history : null;
            }
        }
    } catch (e) {
        console.error("PG lock-wait history fetch failed:", e);
    }

    const lwCanvas = document.getElementById('pgLockWaitTrendChart');
    if (lwCanvas) {
        const lwLabels = Array.isArray(lockWaitHist?.labels) ? lockWaitHist.labels : [];
        const lwCounts = Array.isArray(lockWaitHist?.lock_waiting_sessions) ? lockWaitHist.lock_waiting_sessions : [];
        if (!lwLabels.length || !lwCounts.length) {
            const card = lwCanvas.closest('.chart-card') || lwCanvas.parentElement;
            if (card) card.style.display = 'none';
        } else {
            const n = Math.min(60, lwLabels.length);
            const sl = lwLabels.slice(-n).map(s => {
                try { return new Date(s).toLocaleTimeString(); } catch { return ''; }
            });
            const sv = lwCounts.slice(-n).map(v => Number(v || 0));
            if (window.currentCharts.pgLockWaitTrend) {
                window.currentCharts.pgLockWaitTrend.destroy();
            }
            window.currentCharts.pgLockWaitTrend = new Chart(lwCanvas.getContext('2d'), {
                type: 'line',
                data: {
                    labels: sl,
                    datasets: [{
                        label: 'Sessions in Lock wait',
                        data: sv,
                        borderColor: window.getCSSVar('--warning'),
                        backgroundColor: 'rgba(234, 179, 8, 0.12)',
                        fill: true,
                        tension: 0.2
                    }]
                },
                options: { responsive: true, maintainAspectRatio: false, scales: { y: { beginAtZero: true } } }
            });
        }
    }

    const dlc = document.getElementById('pgDeadlocksChart');
    if (dlc) {
        const labels = Array.isArray(deadlockHist?.labels) ? deadlockHist.labels : [];
        const deltas = Array.isArray(deadlockHist?.deadlocks_delta) ? deadlockHist.deadlocks_delta : [];
        if (!labels.length || !deltas.length) {
            const card = dlc.closest('.chart-card') || dlc.parentElement;
            if (card) card.style.display = 'none';
        } else {
            const dlLabels = labels.slice(-30).map(s => {
                try { return new Date(s).toLocaleTimeString(); } catch { return ''; }
            });
            const dlValues = deltas.slice(-30).map(v => Number(v || 0));

            if (window.currentCharts.pgDlck) {
                window.currentCharts.pgDlck.destroy();
            }
            window.currentCharts.pgDlck = new Chart(dlc.getContext('2d'), {
                type: 'bar', data: {
                    labels: dlLabels, datasets: [{ label:'Deadlocks', data:dlValues, backgroundColor:window.getCSSVar('--danger') }]
                }, options: {responsive:true, maintainAspectRatio:false, scales:{y:{beginAtZero:true}}}
            });
        }
    }

    const ldistCtx = document.getElementById('pgLockDistChart');
    if (ldistCtx) {
        const lockModes = {};
        locks.forEach(l => {
            const mode = l.mode || 'unknown';
            lockModes[mode] = (lockModes[mode] || 0) + 1;
        });
        const modeLabels = Object.keys(lockModes).length > 0 ? Object.keys(lockModes) : ['None'];
        const modeData = modeLabels.length > 1 ? Object.values(lockModes) : [0];

        window.currentCharts.pgLockDist = new Chart(ldistCtx.getContext('2d'), {
            type: 'doughnut', data: {
                labels: modeLabels, datasets: [{ data:modeData, backgroundColor:[window.getCSSVar('--danger'), window.getCSSVar('--warning'), window.getCSSVar('--success'), window.getCSSVar('--accent-blue')], borderWidth:0 }]
            }, options: {responsive:true, maintainAspectRatio:false, cutout:'60%', plugins:{legend:{position:'bottom'}}}
        });
    }
}

function renderBlockingSummary(blockingTree) {
    const elTotal = document.getElementById('pgBlockedSessionsTotal');
    const elTopPid = document.getElementById('pgTopBlockerPid');
    const elTopCnt = document.getElementById('pgTopBlockerCount');
    const elWorstDur = document.getElementById('pgWorstBlockedDur');
    const elWorstPid = document.getElementById('pgWorstBlockedPid');
    const elIdleInTxn = document.getElementById('pgIdleInTxnInvolved');
    if (!elTotal || !elTopPid || !elTopCnt || !elWorstDur || !elWorstPid || !elIdleInTxn) return;

    if (!Array.isArray(blockingTree) || blockingTree.length === 0) {
        elTotal.textContent = '0';
        elTopPid.textContent = '—';
        elTopCnt.textContent = 'No blocking';
        elWorstDur.textContent = '—';
        elWorstPid.textContent = '—';
        elIdleInTxn.textContent = 'No';
        return;
    }

    const nowMs = Date.now();
    const blockers = new Map(); // pid -> { blockedCount, maxBlockedSec }
    let blockedSessionsTotal = 0;
    let idleInTxnInvolved = false;

    const nodePid = (n) => Number(n?.pid || 0) || 0;
    const nodeState = (n) => String(n?.state || '').toLowerCase();
    const nodeQueryStartMs = (n) => {
        if (!n?.query_start) return 0;
        const t = Date.parse(n.query_start);
        return isFinite(t) ? t : 0;
    };

    // Traverse edges parent -> child (parent blocks child).
    const walk = (parent, children) => {
        (children || []).forEach(child => {
            blockedSessionsTotal += 1;
            if (nodeState(child).includes('idle in transaction') || nodeState(parent).includes('idle in transaction')) {
                idleInTxnInvolved = true;
            }

            const parentPid = nodePid(parent);
            if (parentPid) {
                const qsMs = nodeQueryStartMs(child);
                const blockedSec = qsMs ? Math.max(0, Math.floor((nowMs - qsMs) / 1000)) : 0;
                const cur = blockers.get(parentPid) || { blockedCount: 0, maxBlockedSec: 0 };
                cur.blockedCount += 1;
                cur.maxBlockedSec = Math.max(cur.maxBlockedSec, blockedSec);
                blockers.set(parentPid, cur);
            }

            // Also attribute indirect blocking up the chain.
            if (parent?.__ancestors && Array.isArray(parent.__ancestors)) {
                parent.__ancestors.forEach(ancPid => {
                    const qsMs = nodeQueryStartMs(child);
                    const blockedSec = qsMs ? Math.max(0, Math.floor((nowMs - qsMs) / 1000)) : 0;
                    const cur = blockers.get(ancPid) || { blockedCount: 0, maxBlockedSec: 0 };
                    cur.blockedCount += 1;
                    cur.maxBlockedSec = Math.max(cur.maxBlockedSec, blockedSec);
                    blockers.set(ancPid, cur);
                });
            }

            const next = Object.assign({}, child, { __ancestors: [...(parent.__ancestors || []), parentPid].filter(Boolean) });
            walk(next, child.blocked_by);
        });
    };

    blockingTree.forEach(root => {
        const r = Object.assign({}, root, { __ancestors: [] });
        walk(r, root.blocked_by);
    });

    elTotal.textContent = String(blockedSessionsTotal);
    elIdleInTxn.textContent = idleInTxnInvolved ? 'Yes' : 'No';
    elIdleInTxn.className = `strip-metric-value metric-value ${idleInTxnInvolved ? 'text-danger' : ''}`;

    const entries = Array.from(blockers.entries()).map(([pid, v]) => ({ pid, ...v }));
    if (entries.length === 0) {
        elTopPid.textContent = '—';
        elTopCnt.textContent = 'No blocking';
        elWorstDur.textContent = '—';
        elWorstPid.textContent = '—';
        return;
    }

    entries.sort((a, b) => (b.blockedCount - a.blockedCount) || (b.maxBlockedSec - a.maxBlockedSec));
    const top = entries[0];
    elTopPid.textContent = String(top.pid);
    elTopCnt.textContent = `${top.blockedCount} blocked`;

    const worst = entries.slice().sort((a, b) => b.maxBlockedSec - a.maxBlockedSec)[0];
    elWorstDur.textContent = formatDuration(worst.maxBlockedSec);
    elWorstPid.textContent = `PID ${worst.pid}`;
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

function getLockModeBadge(mode) {
    const modeLower = (mode || '').toLowerCase();
    if (modeLower.includes('exclusive')) {
        return `<span class="badge badge-danger">${window.escapeHtml(mode)}</span>`;
    } else if (modeLower.includes('share')) {
        return `<span class="badge badge-warning">${window.escapeHtml(mode)}</span>`;
    } else {
        return `<span class="badge badge-info">${window.escapeHtml(mode)}</span>`;
    }
}

function renderBlockingTree(blockingTree) {
    const container = document.getElementById('blockingTreeList');
    if (!container) return;

    if (!blockingTree || blockingTree.length === 0) {
        container.innerHTML = '<li style="color: var(--success);"><i class="fa-solid fa-check-circle"></i> No blocking sessions detected</li>';
        return;
    }

    function renderNode(node, isRoot = true) {
        const iconClass = node.state === 'idle in transaction' ? 'fa-lock text-danger' : 'fa-lock text-warning';
        const liClass = isRoot ? 'style="margin-bottom:0.75rem;"' : 'style="margin-top:0.5rem;"';
        
        let html = `<li ${liClass}><i class="fa-solid ${iconClass}"></i> <strong>PID ${node.pid}</strong> (${window.escapeHtml(node.state)}) <em>Duration: ${window.escapeHtml(node.duration)}</em>`;
        
        if (node.wait_event) {
            html += ` <em>WaitEvent: ${window.escapeHtml(node.wait_event)}</em>`;
        }
        
        if (node.blocked_by && node.blocked_by.length > 0) {
            html += '<ul style="list-style:none; border-left: 2px dashed var(--warning); margin-left:1rem; padding-left:1rem;">';
            node.blocked_by.forEach(blocked => {
                html += renderNode(blocked, false);
            });
            html += '</ul>';
        }
        
        html += '</li>';
        return html;
    }

    let html = '';
    blockingTree.forEach(node => {
        html += renderNode(node);
    });
    
    container.innerHTML = html;
}

function renderBlockingDetails(blockingTree) {
    const tbody = document.getElementById('pgBlockingDetailsTbody');
    if (!tbody) return;

    const flatten = (nodes, out=[]) => {
        (nodes || []).forEach(n => { out.push(n); flatten(n.blocked_by, out); });
        return out;
    };
    const all = flatten(blockingTree, []);
    if (!all.length) {
        tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">No blocking sessions detected</td></tr>';
        return;
    }
    all.sort((a,b) => (b.blocked_by?.length||0) - (a.blocked_by?.length||0));
    const top = all.slice(0, 20);

    tbody.innerHTML = top.map(n => {
        const qs = n.query_start ? new Date(n.query_start).toLocaleString() : '-';
        const sql = n.query || '';
        const preview = sql.substring(0, 90) + (sql.length > 90 ? '...' : '');
        return `
            <tr style="cursor:pointer;" onclick="window.pgShowSqlModal('${window.escapeHtml(String(n.pid || ''))}','${window.escapeHtml(String(n.user||''))}','${window.escapeHtml(sql).replace(/'/g,'&#39;')}')">
                <td>${n.pid}</td>
                <td>${window.escapeHtml(n.user || '-')}</td>
                <td>${window.escapeHtml(n.database || '-')}</td>
                <td class="text-muted">${window.escapeHtml(qs)}</td>
                <td>${window.escapeHtml(n.duration || '-')}</td>
                <td class="text-muted">${window.escapeHtml(n.wait_event || '-')}</td>
                <td><span class="code-snippet" title="${window.escapeHtml(sql)}">${window.escapeHtml(preview)}</span></td>
            </tr>
        `;
    }).join('');
}

window.pgShowSqlModal = function(pid, user, sql) {
    const existing = document.getElementById('pgSqlModal');
    if (existing) existing.remove();
    const div = document.createElement('div');
    div.id = 'pgSqlModal';
    div.style.position = 'fixed';
    div.style.inset = '0';
    div.style.background = 'rgba(0,0,0,0.55)';
    div.style.zIndex = '9999';
    div.innerHTML = `
        <div class="glass-panel" style="max-width:900px; margin:6vh auto; padding:0.9rem;">
            <div class="flex-between" style="align-items:center; gap:1rem;">
                <div>
                    <div style="font-weight:700;">Blocking SQL Details</div>
                    <div class="text-muted" style="font-size:0.8rem;">PID: <code>${pid}</code> | User: <code>${user || '-'}</code></div>
                </div>
                <button class="btn btn-sm btn-outline" id="pgSqlClose">Close</button>
            </div>
            <div class="mt-2" style="max-height:55vh; overflow:auto;">
                <pre style="white-space:pre-wrap; margin:0; font-size:0.75rem; background:var(--bg-tertiary); padding:0.75rem; border-radius:8px;">${sql || ''}</pre>
            </div>
        </div>
    `;
    document.body.appendChild(div);
    const close = document.getElementById('pgSqlClose');
    if (close) close.onclick = () => div.remove();
    div.onclick = (e) => { if (e.target === div) div.remove(); };
};
