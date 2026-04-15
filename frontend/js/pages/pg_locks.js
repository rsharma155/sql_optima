/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL Locks & Blocking dashboard (stateful incident monitoring).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgLocksView = async function() {
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_locks.html');
    setTimeout(initPgLocks, 50);
}

function pgLocksFormatLocal(dt) {
    const pad = n => String(n).padStart(2, '0');
    return dt.getFullYear() + '-' + pad(dt.getMonth() + 1) + '-' + pad(dt.getDate()) + 'T' + pad(dt.getHours()) + ':' + pad(dt.getMinutes());
}

function pgLocksToRFC3339(datetimeLocalValue) {
    // datetime-local is interpreted as local time. Convert to RFC3339 UTC for API.
    const ms = Date.parse(datetimeLocalValue);
    if (!isFinite(ms)) return '';
    return new Date(ms).toISOString();
}

async function initPgLocks() {
    window.currentCharts = window.currentCharts || {};
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: ''};
    const instQ = encodeURIComponent(inst.name);

    // Range UI (default last 1 hour).
    const fromEl = document.getElementById('pgLocksFrom');
    const toEl = document.getElementById('pgLocksTo');
    const now = new Date();
    const hourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    if (fromEl && !fromEl.dataset.touched) fromEl.value = pgLocksFormatLocal(hourAgo);
    if (toEl && !toEl.dataset.touched) toEl.value = pgLocksFormatLocal(now);

    const applyBtn = document.getElementById('pgLocksApplyRange');
    if (applyBtn) {
        applyBtn.onclick = () => {
            if (fromEl) fromEl.dataset.touched = '1';
            if (toEl) toEl.dataset.touched = '1';
            loadPgLocksRangeData();
        };
    }

    function updateRangeHint() {
        const hint = document.getElementById('pgLocksRangeHint');
        if (!hint || !fromEl || !toEl) return;
        if (!fromEl.value || !toEl.value) {
            hint.style.display = 'none';
            hint.textContent = '';
            return;
        }
        const err = typeof window.getDatetimeLocalRangeError === 'function'
            ? window.getDatetimeLocalRangeError(fromEl.value, toEl.value) : '';
        if (err) {
            hint.textContent = err;
            hint.style.display = 'block';
        } else {
            hint.style.display = 'none';
            hint.textContent = '';
        }
    }
    if (fromEl && toEl && !fromEl.dataset.pgRangeListeners) {
        fromEl.dataset.pgRangeListeners = '1';
        fromEl.addEventListener('change', updateRangeHint);
        toEl.addEventListener('change', updateRangeHint);
    }

    // Fetch locks data
    let locks = [];
    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/postgres/locks?instance=${instQ}`
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

    // Always load KPIs (current), then load range-driven time series.
    let kpis = null;
    try {
        const kRes = await window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/kpis?instance=${instQ}`);
        if (kRes.ok && (kRes.headers.get('content-type') || '').includes('application/json')) {
            const data = await kRes.json();
            kpis = data && data.kpis ? data.kpis : null;
        }
    } catch (e) {
        console.error("PG locks-blocking KPI fetch failed:", e);
    }
    updatePgLocksKpis(kpis);

    async function loadPgLocksRangeData() {
        if (fromEl && toEl && fromEl.value && toEl.value) {
            const rangeErr = typeof window.getDatetimeLocalRangeError === 'function'
                ? window.getDatetimeLocalRangeError(fromEl.value, toEl.value) : '';
            if (rangeErr) {
                if (typeof window.showDateRangeValidationError === 'function') {
                    window.showDateRangeValidationError(rangeErr);
                }
                const hint = document.getElementById('pgLocksRangeHint');
                if (hint) {
                    hint.textContent = rangeErr;
                    hint.style.display = 'block';
                }
                return;
            }
        }

        const fromISO = fromEl && fromEl.value ? pgLocksToRFC3339(fromEl.value) : '';
        const toISO = toEl && toEl.value ? pgLocksToRFC3339(toEl.value) : '';
        const meta = document.getElementById('pgLocksRangeMeta');
        if (meta && fromISO && toISO) {
            meta.textContent = `${new Date(fromISO).toLocaleString()} → ${new Date(toISO).toLocaleString()}`;
        }

        let timelinePoints = [];
        let incidentWindows = [];
        let topLockedTables = [];
        let detailsTree = [];
        let detailsCapturedAt = '';
        try {
            const [tRes, topRes, dRes] = await Promise.all([
                window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/timeline?instance=${instQ}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`),
                window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/top-locked-tables?instance=${instQ}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}&limit=10`),
                window.apiClient.authenticatedFetch(`/api/postgres/locks-blocking/details?instance=${instQ}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`),
            ]);
            if (tRes.ok && (tRes.headers.get('content-type') || '').includes('application/json')) {
                const data = await tRes.json();
                timelinePoints = Array.isArray(data.timeline) ? data.timeline : [];
                incidentWindows = Array.isArray(data.incidents) ? data.incidents : [];
            }
            if (topRes.ok && (topRes.headers.get('content-type') || '').includes('application/json')) {
                const data = await topRes.json();
                topLockedTables = Array.isArray(data.tables) ? data.tables : [];
            }
            if (dRes.ok && (dRes.headers.get('content-type') || '').includes('application/json')) {
                const data = await dRes.json();
                detailsTree = Array.isArray(data.blocking_tree) ? data.blocking_tree : [];
                detailsCapturedAt = data && data.collected_at ? String(data.collected_at) : '';
            }
        } catch (e) {
            console.error("PG locks-blocking range fetch failed:", e);
        }

        renderPgBlockingIncidentTimeline(timelinePoints, incidentWindows);
        renderPgTopLockedTables(topLockedTables);
        renderBlockingTree(detailsTree);
        const capEl = document.getElementById('pgBlockingDetailsCapturedAt');
        if (capEl) {
            capEl.textContent = detailsCapturedAt ? new Date(detailsCapturedAt).toLocaleString() : '—';
        }
        renderBlockingDetails(detailsTree, detailsCapturedAt);
        renderBlockingSummary(detailsTree);
    }

    window.__loadPgLocksRangeData = loadPgLocksRangeData;
    await loadPgLocksRangeData();

    // NOTE: Blocking Tree + Details are now range-driven and Timescale-backed via
    // /api/postgres/locks-blocking/details (see loadPgLocksRangeData()).

    // Deadlock delta history (Timescale-backed). No dummy charts.
    let deadlockHist = null;
    try {
        const dlResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/deadlocks/history?instance=${instQ}&window_minutes=180&limit=400`
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
            `/api/postgres/locks/wait-history?instance=${instQ}&window_minutes=180&limit=400`
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

function updatePgLocksKpis(kpis) {
    const elVictims = document.getElementById('pgKpiActiveVictims');
    const elMeta = document.getElementById('pgKpiIncidentMeta');
    const elRootPid = document.getElementById('pgKpiRootPid');
    const elRootQuery = document.getElementById('pgKpiRootQuery');
    const elIdle = document.getElementById('pgKpiIdleRisk');
    const elSev = document.getElementById('pgKpiSeverity');
    const elBand = document.getElementById('pgKpiSeverityBand');
    if (!elVictims || !elMeta || !elRootPid || !elRootQuery || !elIdle || !elSev || !elBand) return;

    const victims = Number(kpis?.active_blocking_sessions ?? 0) || 0;
    const depth = Number(kpis?.chain_depth ?? 0) || 0;
    const idleRisk = Number(kpis?.idle_in_txn_risk_count ?? 0) || 0;
    const durMins = Number(kpis?.incident_duration_mins ?? 0) || 0;
    const rootPid = kpis?.root_blocker_pid != null ? String(kpis.root_blocker_pid) : '—';
    const rootQ = String(kpis?.root_blocker_query ?? '').trim();

    elVictims.textContent = String(victims);
    elVictims.classList.toggle('text-danger', victims > 0);
    elVictims.classList.toggle('text-success', victims === 0);

    const incId = kpis?.incident_id != null ? String(kpis.incident_id) : '';
    elMeta.textContent = incId ? `incident #${incId} • ${durMins}m • depth ${depth}` : 'no active incident';

    elRootPid.textContent = rootPid;
    elRootQuery.textContent = rootQ ? (rootQ.length > 110 ? (rootQ.slice(0, 110) + '…') : rootQ) : '—';

    elIdle.textContent = String(idleRisk);
    elIdle.classList.toggle('text-warning', idleRisk > 0);

    const score = (victims * 10) + (depth * 5) + (idleRisk * 30) + (durMins * 2);
    elSev.textContent = String(score);
    const band = score >= 80 ? 'RED' : score >= 50 ? 'ORANGE' : score >= 20 ? 'YELLOW' : 'GREEN';
    elBand.textContent = band;
    elBand.classList.remove('text-success', 'text-warning', 'text-danger');
    if (band === 'GREEN') elBand.classList.add('text-success');
    else if (band === 'YELLOW') elBand.classList.add('text-warning');
    else elBand.classList.add('text-danger');

    // On-page alert banner for active blocking / alarming severity.
    const banner = document.getElementById('pgLocksAlertBanner');
    const title = document.getElementById('pgLocksAlertTitle');
    const msg = document.getElementById('pgLocksAlertMsg');
    if (banner && title && msg) {
        const alarming = victims > 0 || score >= 50;
        if (!alarming) {
            banner.style.display = 'none';
        } else {
            banner.style.display = 'block';
            const incId = kpis?.incident_id != null ? String(kpis.incident_id) : '';
            title.textContent = victims > 0 ? 'Blocking detected' : 'Incident severity elevated';
            const root = kpis?.root_blocker_pid != null ? `root PID ${kpis.root_blocker_pid}` : 'root unknown';
            msg.textContent = `${victims} blocked • score ${score} (${band})${incId ? ` • incident #${incId}` : ''} • ${root}`;
            banner.classList.remove('alert-warning', 'alert-danger');
            banner.classList.add(score >= 80 || victims > 0 ? 'alert-danger' : 'alert-warning');
        }
    }
}

function renderPgTopLockedTables(rows) {
    const tbody = document.getElementById('pgTopLockedTablesTbody');
    if (!tbody) return;
    const list = Array.isArray(rows) ? rows : [];
    if (list.length === 0) {
        tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">No waiting locks in lookback window</td></tr>';
        return;
    }
    tbody.innerHTML = list.map(r => {
        const rel = window.escapeHtml(String(r?.relation_name ?? 'virtual'));
        const cnt = Number(r?.waiting_count ?? 0) || 0;
        const maxw = Number(r?.max_wait_seconds ?? 0) || 0;
        return `<tr><td>${rel}</td><td><strong>${cnt}</strong></td><td>${maxw.toFixed(0)}</td></tr>`;
    }).join('');
}

function renderPgBlockingIncidentTimeline(points, incidents) {
    const canvas = document.getElementById('pgBlockingIncidentTimelineChart');
    if (!canvas) return;

    const rows = Array.isArray(points) ? points : [];
    const labels = rows.map(p => {
        const ts = p?.bucket;
        try { return ts ? new Date(ts).toLocaleTimeString() : ''; } catch { return ''; }
    });
    const data = rows.map(p => Number(p?.blocked_sessions ?? 0) || 0);

    const windows = (Array.isArray(incidents) ? incidents : []).map(w => {
        const s = w?.started_at ? Date.parse(w.started_at) : NaN;
        const e = w?.ended_at ? Date.parse(w.ended_at) : NaN;
        return { startMs: s, endMs: isFinite(e) ? e : Date.now() };
    }).filter(w => isFinite(w.startMs));

    const shadePlugin = {
        id: 'incidentShading',
        beforeDatasetsDraw(chart) {
            if (!windows.length) return;
            const ctx = chart.ctx;
            const xScale = chart.scales.x;
            const area = chart.chartArea;
            if (!xScale || !area) return;

            const xs = rows.map(p => {
                const ts = p?.bucket;
                const ms = ts ? Date.parse(ts) : NaN;
                return isFinite(ms) ? ms : NaN;
            });

            ctx.save();
            ctx.fillStyle = 'rgba(239, 68, 68, 0.10)';
            windows.forEach(w => {
                let first = -1, last = -1;
                for (let i = 0; i < xs.length; i++) {
                    const ms = xs[i];
                    if (!isFinite(ms)) continue;
                    if (ms >= w.startMs && ms <= w.endMs) {
                        if (first === -1) first = i;
                        last = i;
                    }
                }
                if (first === -1 || last === -1) return;
                const x0 = xScale.getPixelForValue(first);
                const x1 = xScale.getPixelForValue(last);
                ctx.fillRect(x0, area.top, Math.max(1, x1 - x0), area.bottom - area.top);
            });
            ctx.restore();
        }
    };

    if (window.currentCharts.pgBlockingTimeline) {
        window.currentCharts.pgBlockingTimeline.destroy();
    }

    window.currentCharts.pgBlockingTimeline = new Chart(canvas.getContext('2d'), {
        type: 'line',
        data: { labels, datasets: [{
            label: 'Blocked sessions / min',
            data,
            borderColor: window.getCSSVar('--danger'),
            backgroundColor: 'rgba(239, 68, 68, 0.08)',
            fill: true,
            tension: 0.25,
            pointRadius: 0,
        }]},
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            scales: { y: { beginAtZero: true } }
        },
        plugins: [shadePlugin],
    });
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

function renderBlockingDetails(blockingTree, capturedAtIso) {
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

    // IMPORTANT: avoid inline onclick with raw SQL (quotes/newlines break JS parsing).
    const capTxt = capturedAtIso ? (() => { try { return new Date(capturedAtIso).toLocaleString(); } catch { return capturedAtIso; } })() : '—';
    tbody.innerHTML = top.map((n, idx) => {
        const qs = n.query_start ? new Date(n.query_start).toLocaleString() : '-';
        const sql = String(n.query || '');
        const preview = sql.substring(0, 90) + (sql.length > 90 ? '...' : '');
        const pid = n.pid != null ? String(n.pid) : '';
        const user = String(n.user || '');
        const db = String(n.database || '');
        const dur = String(n.duration || '');
        const wait = String(n.wait_event || '');
        return `
            <tr class="pg-blocking-row" style="cursor:pointer;"
                data-pid="${window.escapeHtml(pid)}"
                data-user="${window.escapeHtml(user)}"
                data-sql="${window.escapeHtml(sql)}">
                <td class="text-muted">${window.escapeHtml(capTxt)}</td>
                <td>${window.escapeHtml(pid || '-')}</td>
                <td>${window.escapeHtml(user || '-')}</td>
                <td>${window.escapeHtml(db || '-')}</td>
                <td class="text-muted">${window.escapeHtml(qs)}</td>
                <td>${window.escapeHtml(dur || '-')}</td>
                <td class="text-muted">${window.escapeHtml(wait || '-')}</td>
                <td><span class="code-snippet" title="${window.escapeHtml(sql)}">${window.escapeHtml(preview)}</span></td>
            </tr>
        `;
    }).join('');

    // Delegate click handling to the tbody.
    if (!tbody.dataset.pgBlockingClickBound) {
        tbody.dataset.pgBlockingClickBound = '1';
        tbody.addEventListener('click', (e) => {
            const tr = e.target && e.target.closest ? e.target.closest('tr.pg-blocking-row') : null;
            if (!tr) return;
            const pid = tr.dataset.pid || '';
            const user = tr.dataset.user || '';
            const sql = tr.dataset.sql || '';
            window.pgShowSqlModal(pid, user, sql);
        });
    }
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

