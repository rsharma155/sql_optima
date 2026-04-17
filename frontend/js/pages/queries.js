/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Query analysis page with performance metrics.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgQueriesView = async function() {
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/queries.html');
    setTimeout(initPgQueries, 50);
};

function pgQueriesFormatLocal(dt) {
    const pad = n => String(n).padStart(2, '0');
    return dt.getFullYear() + '-' + pad(dt.getMonth() + 1) + '-' + pad(dt.getDate()) + 'T' + pad(dt.getHours()) + ':' + pad(dt.getMinutes());
}

async function initPgQueries() {
    window.currentCharts = window.currentCharts || {};

    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: '' };
    const database = window.appState.currentDatabase || 'all';

    const fromEl = document.getElementById('pgQueriesFrom');
    const toEl = document.getElementById('pgQueriesTo');
    const now = new Date();
    const hourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    if (fromEl && !fromEl.dataset.touched) {
        fromEl.value = pgQueriesFormatLocal(hourAgo);
    }
    if (toEl && !toEl.dataset.touched) {
        toEl.value = pgQueriesFormatLocal(now);
    }

    const applyBtn = document.getElementById('pgQueriesApplyRange');
    if (applyBtn) {
        applyBtn.onclick = () => {
            if (fromEl) fromEl.dataset.touched = '1';
            if (toEl) toEl.dataset.touched = '1';
            loadPgQueriesPageData();
        };
    }

    function updatePgQueriesRangeHint() {
        const hint = document.getElementById('pgQueriesRangeHint');
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
        fromEl.addEventListener('change', updatePgQueriesRangeHint);
        toEl.addEventListener('change', updatePgQueriesRangeHint);
    }

    try {
        const instEl = document.getElementById('pgQueriesInstance');
        const dbEl = document.getElementById('pgQueriesDatabase');
        if (instEl) instEl.textContent = inst.name || '--';
        if (dbEl) dbEl.textContent = database;

        const strip = document.getElementById('pgQueriesStatusStrip');
        if (strip && typeof window.renderStatusStrip === 'function') {
            strip.innerHTML = window.renderStatusStrip({
                lastUpdateId: 'pgQueriesLastRefreshTime',
                sourceBadgeId: 'pgQueriesSourceBadge',
                includeHealth: false,
                includeFreshness: false,
                autoRefreshText: ''
            });
        }
        const t = document.getElementById('pgQueriesLastRefreshTime');
        if (t) t.textContent = new Date().toLocaleTimeString();
    } catch (e) {
        // non-fatal
    }

    await loadPgQueriesPageData();
}

async function loadPgQueriesPageData() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: '' };
    const fromEl = document.getElementById('pgQueriesFrom');
    const toEl = document.getElementById('pgQueriesTo');

    if (fromEl && toEl && fromEl.value && toEl.value) {
        const rangeErr = typeof window.getDatetimeLocalRangeError === 'function'
            ? window.getDatetimeLocalRangeError(fromEl.value, toEl.value) : '';
        if (rangeErr) {
            if (typeof window.showDateRangeValidationError === 'function') {
                window.showDateRangeValidationError(rangeErr);
            }
            const hint = document.getElementById('pgQueriesRangeHint');
            if (hint) {
                hint.textContent = rangeErr;
                hint.style.display = 'block';
            }
            return;
        }
    }

    const tbody = document.getElementById('pg-queries-tbody');
    if (tbody) {
        tbody.innerHTML = '<tr><td colspan="7" class="text-center"><div class="spinner"></div> Loading query stats…</td></tr>';
    }
    if (typeof window.setChartOverlayState === 'function') {
        window.setChartOverlayState('pgQryDistChart', 'loading', 'Loading chart…');
        window.setChartOverlayState('pgQrySlowChart', 'loading', 'Loading chart…');
    }

    let url = `/api/postgres/queries?instance=${encodeURIComponent(inst.name)}`;
    if (fromEl && toEl && fromEl.value && toEl.value) {
        const fromIso = new Date(fromEl.value).toISOString();
        const toIso = new Date(toEl.value).toISOString();
        url += `&from=${encodeURIComponent(fromIso)}&to=${encodeURIComponent(toIso)}`;
    }

    let queries = [];
    let pgStatStatementsEnabled = true;
    let collectedAt = null;
    const noteEl = document.getElementById('pgQueriesStatsNote');
    if (noteEl) noteEl.textContent = '';

    try {
        const response = await window.apiClient.authenticatedFetch(url);
        if (response.ok) {
            const contentType = response.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const data = await response.json();
                queries = data.queries || [];
                window._pgQueriesMeta = {
                    end_capture: data.end_capture || '',
                    window_from: data.window_from || '',
                    window_to: data.window_to || '',
                    stats_note: data.stats_note || '',
                    stats_source: data.stats_source || '',
                    window_note: data.window_note || '',
                    baseline_capture: data.baseline_capture || ''
                };
                if (data.error && noteEl) {
                    noteEl.textContent = data.error;
                } else if (data.stats_note && noteEl) {
                    let t = data.stats_note;
                    if (data.stats_source === 'timescale_delta' && data.window_note) {
                        t += ' (' + data.window_note.replace(/_/g, ' ') + ')';
                    }
                    noteEl.textContent = t;
                }
                const cAt = document.getElementById('pgQueriesCollectedAt');
                if (data.collected_at) {
                    collectedAt = data.collected_at;
                }
                if (cAt && collectedAt) {
                    cAt.textContent = new Date(collectedAt).toLocaleString();
                }
                if (data.pg_stat_statements_enabled === false) {
                    pgStatStatementsEnabled = false;
                }
            }
        } else {
            console.error('Failed to load PG queries:', response.status);
            pgStatStatementsEnabled = false;
            if (noteEl) noteEl.textContent = 'Failed to load query stats (HTTP ' + response.status + ').';
        }
    } catch (e) {
        console.error('PG queries fetch failed:', e);
        pgStatStatementsEnabled = false;
        if (noteEl) noteEl.textContent = 'Failed to load query stats: ' + (e.message || String(e));
    }

    try {
        const t = document.getElementById('pgQueriesLastRefreshTime');
        if (t) t.textContent = new Date().toLocaleTimeString();
    } catch (e) {
        // non-fatal
    }

    const warningEl = document.getElementById('pg-statements-warning');
    if (warningEl && !pgStatStatementsEnabled) {
        warningEl.style.display = 'block';
    } else if (warningEl) {
        warningEl.style.display = 'none';
    }

    if (typeof window.clearChartOverlay === 'function') {
        window.clearChartOverlay('pgQryDistChart');
        window.clearChartOverlay('pgQrySlowChart');
    }

    window._pgQueriesData = queries;
    window._pgQueriesCollectedAt = collectedAt;
    window._pgQuerySqlById = {};
    window._pgQueryUserById = {};
    (queries || []).forEach(q => {
        const id = q && q.query_id !== undefined ? String(q.query_id) : '';
        if (!id) return;
        window._pgQuerySqlById[id] = q.query || '';
        window._pgQueryUserById[id] = q.user || '';
    });

    renderQueriesTable(queries, 'total');

    const distCtx = document.getElementById('pgQryDistChart');
    if (distCtx && queries.length > 0) {
        const buckets = [0, 0, 0, 0, 0];
        queries.forEach(q => {
            const meanMs = q.mean_time || 0;
            if (meanMs < 1) buckets[0]++;
            else if (meanMs < 5) buckets[1]++;
            else if (meanMs < 20) buckets[2]++;
            else if (meanMs < 100) buckets[3]++;
            else buckets[4]++;
        });

        if (window.currentCharts.pgQryDist) {
            window.currentCharts.pgQryDist.destroy();
        }
        window.currentCharts.pgQryDist = new Chart(distCtx.getContext('2d'), {
            type: 'bar',
            data: {
                labels: ['<1ms', '1-5ms', '5-20ms', '20-100ms', '100+ms'],
                datasets: [{ label: 'Queries', data: buckets, backgroundColor: [window.getCSSVar('--success'), window.getCSSVar('--accent-blue'), window.getCSSVar('--warning'), window.getCSSVar('--danger'), '#991b1b'] }]
            },
            options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } } }
        });
    } else if (distCtx) {
        if (window.currentCharts.pgQryDist) {
            window.currentCharts.pgQryDist.destroy();
        }
        window.currentCharts.pgQryDist = new Chart(distCtx.getContext('2d'), {
            type: 'bar',
            data: {
                labels: ['<1ms', '1-5ms', '5-20ms', '20-100ms', '100+ms'],
                datasets: [{ label: 'Queries', data: [0, 0, 0, 0, 0], backgroundColor: [window.getCSSVar('--success'), window.getCSSVar('--accent-blue'), window.getCSSVar('--warning'), window.getCSSVar('--danger'), '#991b1b'] }]
            },
            options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } } }
        });
        if (typeof window.setChartOverlayState === 'function') {
            window.setChartOverlayState('pgQryDistChart', 'empty', 'No query rows in this range.');
        }
    }

    const slowCtx = document.getElementById('pgQrySlowChart');
    if (slowCtx && queries.length > 0) {
        const sorted = [...queries].sort((a, b) => (b.mean_time || 0) - (a.mean_time || 0));
        const top5 = sorted.slice(0, 5);
        const p99 = top5.map(q => (q.mean_time || 0) * 2);
        const p95 = top5.map(q => (q.mean_time || 0) * 1.5);
        const labels = top5.map(q => {
            const sql = q.query || '';
            return sql.substring(0, 30) + (sql.length > 30 ? '...' : '');
        });

        if (window.currentCharts.pgQrySlow) {
            window.currentCharts.pgQrySlow.destroy();
        }
        window.currentCharts.pgQrySlow = new Chart(slowCtx.getContext('2d'), {
            type: 'bar',
            data: {
                labels,
                datasets: [
                    { label: 'p99 (ms)', data: p99, backgroundColor: window.getCSSVar('--danger') },
                    { label: 'p95 (ms)', data: p95, backgroundColor: window.getCSSVar('--warning') }
                ]
            },
            options: { responsive: true, maintainAspectRatio: false, indexAxis: 'y' }
        });
    } else if (slowCtx) {
        if (window.currentCharts.pgQrySlow) {
            window.currentCharts.pgQrySlow.destroy();
        }
        window.currentCharts.pgQrySlow = new Chart(slowCtx.getContext('2d'), {
            type: 'line',
            data: {
                labels: ['—'],
                datasets: [{ label: 'p99 (ms)', data: [0], borderColor: window.getCSSVar('--danger') }, { label: 'p95 (ms)', data: [0], borderColor: window.getCSSVar('--warning') }]
            },
            options: { responsive: true, maintainAspectRatio: false }
        });
        if (typeof window.setChartOverlayState === 'function') {
            window.setChartOverlayState('pgQrySlowChart', 'empty', 'No query rows in this range.');
        }
    }
}

function renderQueriesTable(queries, sortBy) {
    const tbody = document.getElementById('pg-queries-tbody');
    if (!tbody) return;

    if (!queries || queries.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">No query data for this range (load finished). pg_stat_statements may be off, or there may be no snapshots yet (Timescale + enterprise collector).</td></tr>';
        return;
    }

    let sorted = [...queries];
    if (sortBy === 'total') {
        sorted.sort((a, b) => (b.total_time || 0) - (a.total_time || 0));
    } else if (sortBy === 'mean') {
        sorted.sort((a, b) => (b.mean_time || 0) - (a.mean_time || 0));
    } else if (sortBy === 'calls') {
        sorted.sort((a, b) => (b.calls || 0) - (a.calls || 0));
    } else if (sortBy === 'io') {
        sorted.sort((a, b) => ((b.shared_blks_read || 0) + (b.temp_blks_read || 0)) - ((a.shared_blks_read || 0) + (a.temp_blks_read || 0)));
    }

    tbody.innerHTML = sorted.slice(0, 20).map(query => {
        const totalTime = query.total_time || 0;
        const avgTime = query.mean_time || 0;
        const totalH = Math.floor(totalTime / 3600000);
        const totalM = Math.floor((totalTime % 3600000) / 60000);
        const totalS = ((totalTime % 60000) / 1000).toFixed(1);
        const timeStr = totalH > 0 ? `${totalH}h ${totalM}m` : totalM > 0 ? `${totalM}m ${totalS}s` : `${totalS}s`;

        const hitRatio = query.shared_blks_hit > 0 ? ((query.shared_blks_hit / (query.shared_blks_hit + query.shared_blks_read)) * 100).toFixed(0) : 0;
        const readRatio = 100 - hitRatio;
        const hitClass = hitRatio >= 99 ? 'text-success' : hitRatio >= 90 ? 'text-warning' : 'text-danger';

        const avgClass = avgTime > 100 ? 'text-danger font-bold' : avgTime > 10 ? 'text-warning' : '';

        const fingerprint = query.query_id !== undefined ? String(query.query_id) : '-';
        const user = query.user || '';
        const fullSql = query.query || '';
        const sqlPreview = fullSql.substring(0, 80) + (fullSql.length > 80 ? '...' : '');
        const escPreview = window.escapeHtml(sqlPreview);
        const escUser = window.escapeHtml(user || '-');

        return `
            <tr>
                <td>${escUser || '<span class="text-muted">-</span>'}</td>
                <td style="max-width:520px">
                    <div class="code-snippet w-100 pg-sql-preview" style="display:block; cursor:pointer; text-decoration:underline; text-underline-offset:2px;" title="View full SQL and capture time" data-action="call" data-fn="pgOpenQuerySql" data-arg="${window.escapeHtml(fingerprint)}">
                        ${escPreview}
                    </div>
                </td>
                <td><span class="badge badge-outline">${(query.calls || 0).toLocaleString()}</span></td>
                <td>${timeStr}</td>
                <td class="${avgClass}">${avgTime.toFixed(2)} ms</td>
                <td>${(query.rows || 0).toLocaleString()}</td>
                <td>H: <span class="${hitClass}">${hitRatio}%</span> / R: <span class="${readRatio > 10 ? 'text-danger' : ''}">${readRatio}%</span></td>
            </tr>
        `;
    }).join('');
}

window.pgOpenQuerySql = function(queryId) {
    const qid = String(queryId || '');
    const sql = (window._pgQuerySqlById && window._pgQuerySqlById[qid]) ? window._pgQuerySqlById[qid] : '';
    const user = (window._pgQueryUserById && window._pgQueryUserById[qid]) ? window._pgQueryUserById[qid] : '';
    window.showPgQueryDetails(user, sql, window._pgQueriesMeta || {});
};

/** Detail modal: shows capture/window timestamps (pg_stat_statements has no per-execution run time). */
window.showPgQueryDetails = function(user, sql, meta) {
    const existing = document.getElementById('pgQueryDetailModal');
    if (existing) existing.remove();

    const m = meta || {};
    const endCap = m.end_capture ? new Date(m.end_capture).toLocaleString() : '';
    const winFrom = m.window_from ? new Date(m.window_from).toLocaleString() : '';
    const winTo = m.window_to ? new Date(m.window_to).toLocaleString() : '';
    const baseline = m.baseline_capture ? new Date(m.baseline_capture).toLocaleString() : '';

    const div = document.createElement('div');
    div.id = 'pgQueryDetailModal';
    div.style.position = 'fixed';
    div.style.inset = '0';
    div.style.background = 'rgba(0,0,0,0.55)';
    div.style.zIndex = '9999';

    const panel = document.createElement('div');
    panel.className = 'glass-panel';
    panel.style.maxWidth = '900px';
    panel.style.margin = '6vh auto';
    panel.style.padding = '0.9rem';

    const header = document.createElement('div');
    header.className = 'flex-between';
    header.style.alignItems = 'center';
    header.style.gap = '1rem';

    const titleBlock = document.createElement('div');
    const h = document.createElement('div');
    h.style.fontWeight = '700';
    h.textContent = 'Query detail';
    titleBlock.appendChild(h);

    const sub = document.createElement('div');
    sub.className = 'text-muted';
    sub.style.fontSize = '0.8rem';
    const lines = [];
    if (winFrom && winTo) {
        lines.push('Selected window: ' + winFrom + ' → ' + winTo);
    }
    if (baseline) {
        lines.push('Baseline snapshot: ' + baseline);
    }
    if (endCap) {
        lines.push('End snapshot (stats as of): ' + endCap);
    }
    if (!lines.length) {
        lines.push('Timestamps unavailable for this view.');
    }
    lines.push('Note: pg_stat_statements does not record the wall-clock time of individual executions—only aggregated counters.');
    sub.textContent = lines.join('\n');
    titleBlock.appendChild(sub);

    const userRow = document.createElement('div');
    userRow.className = 'text-muted mt-2';
    userRow.style.fontSize = '0.8rem';
    userRow.textContent = 'User: ' + (user || '—');

    const closeBtn = document.createElement('button');
    closeBtn.className = 'btn btn-sm btn-outline';
    closeBtn.id = 'pgQueryDetailClose';
    closeBtn.textContent = 'Close';

    header.appendChild(titleBlock);
    header.appendChild(closeBtn);

    const preWrap = document.createElement('div');
    preWrap.className = 'mt-2';
    preWrap.style.maxHeight = '55vh';
    preWrap.style.overflow = 'auto';
    const pre = document.createElement('pre');
    pre.style.whiteSpace = 'pre-wrap';
    pre.style.margin = '0';
    pre.style.fontSize = '0.75rem';
    pre.style.background = 'var(--bg-tertiary)';
    pre.style.padding = '0.75rem';
    pre.style.borderRadius = '8px';
    pre.textContent = sql || '';

    preWrap.appendChild(pre);
    panel.appendChild(header);
    panel.appendChild(userRow);
    panel.appendChild(preWrap);
    div.appendChild(panel);
    document.body.appendChild(div);

    closeBtn.onclick = () => div.remove();
    div.onclick = e => {
        if (e.target === div) div.remove();
    };
};

/** @deprecated use showPgQueryDetails; kept for pg_enterprise and other callers */
window.showPgQueryFingerprintDetails = function(queryId, user, sql) {
    window.showPgQueryDetails(user || '', sql || '', window._pgEnterpriseQueriesMeta || window._pgQueriesMeta || {});
};

window.sortPgQueries = function(tabElement, sortKey) {
    const tabs = document.querySelectorAll('#pg-queries-tabs li');
    tabs.forEach(t => {
        t.className = 'text-muted';
        t.style.borderBottom = 'none';
    });
    tabElement.className = 'text-accent active-tab';
    tabElement.style.borderBottom = '2px solid var(--accent-blue)';

    renderQueriesTable(window._pgQueriesData || [], sortKey);
};

window.resetPgQueries = function() {
    if (confirm('Reset pg_stat_statements? This will clear all accumulated query statistics.')) {
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
        if (!inst) return;
        window.apiClient.authenticatedFetch(`/api/postgres/reset-queries?instance=${encodeURIComponent(inst.name)}`, { method: 'POST' })
            .then(() => {
                alert('Query statistics reset successfully.');
                window.PgQueriesView();
            })
            .catch(err => alert(`Error: ${err.message}`));
    }
};
