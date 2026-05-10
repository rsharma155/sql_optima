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
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
    const dbName = window.appState.currentDatabase || 'all';

    window.appState.activeViewId = 'pg-queries';

    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/queries.html', { inst, dbName });
    
    // Tab switching logic for Query Monitor
    setTimeout(() => {
        const tabs = document.querySelectorAll('.qa-tab-btn');
        tabs.forEach(btn => {
            btn.addEventListener('click', () => {
                const container = btn.closest('.card');
                if (!container) return;
                
                // Toggle active state on buttons
                container.querySelectorAll('.qa-tab-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                
                // Toggle active state on panes
                const target = btn.dataset.tab;
                container.querySelectorAll('.tab-pane').forEach(p => p.classList.remove('active'));
                const targetPane = container.querySelector('#tab-' + target);
                if (targetPane) {
                    targetPane.classList.add('active');
                    // Force resize charts in the pane
                    targetPane.querySelectorAll('canvas').forEach(canvas => {
                        if (window.currentCharts && window.currentCharts[canvas.id]) {
                            window.currentCharts[canvas.id].resize();
                        }
                    });
                }
            });
        });
    }, 100);

    window.initPageTimePicker();
    await initPgQueries();

    // Set Refresh Interval
    if (window.pgQueriesInterval) clearInterval(window.pgQueriesInterval);
    window.pgQueriesInterval = setInterval(() => {
        if (window.appState.activeViewId === 'pg-queries' || window.appState.activeViewId === 'pg-stat-statements') {
            initPgQueries();
        } else {
            clearInterval(window.pgQueriesInterval);
        }
    }, 30000); // 30s refresh for queries
};

async function updatePgQueriesHeader(instName) {
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
    } catch (e) { console.error("PG Queries header fetch failed:", e); }
}

function pgQueriesFormatLocal(dt) {
    const pad = n => String(n).padStart(2, '0');
    return dt.getFullYear() + '-' + pad(dt.getMonth() + 1) + '-' + pad(dt.getDate()) + 'T' + pad(dt.getHours()) + ':' + pad(dt.getMinutes()) + ':' + pad(dt.getSeconds());
}

async function initPgQueries() {
    window.currentCharts = window.currentCharts || {};

    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: '' };
    const database = window.appState.currentDatabase || 'all';

    const fromEl = document.getElementById('pgQueriesFrom');
    const toEl = document.getElementById('pgQueriesTo');
    const now = new Date();
    const hourAgo = new Date(now.getTime() - 3600000); // 1 hour
    if (fromEl) {
        fromEl.value = pgQueriesFormatLocal(hourAgo);
    }
    if (toEl) {
        toEl.value = pgQueriesFormatLocal(now);
    }

    const applyBtn = document.getElementById('pgQueriesApplyRange');
    if (applyBtn) {
        applyBtn.onclick = () => {
            if (fromEl) fromEl.dataset.touched = '1';
            if (toEl) toEl.dataset.touched = '1';
            loadPgQueriesPageData();
            loadPgssAll();
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

    updatePgQueriesHeader(inst.name);

    // Lazy-load snapshot data when Deep Diagnostics is expanded
    const diagPanel = document.querySelector('.pgss-diagnostics-panel');
    if (diagPanel) {
        diagPanel.addEventListener('toggle', function onDiagToggle() {
            if (diagPanel.open && !diagPanel.dataset.loaded) {
                diagPanel.dataset.loaded = '1';
                loadPgQueriesPageData();
            }
        });
    }

    // Initialize pgss time-series section (merged from pg_stat_statements page)
    initPgssSection(inst.name);

    // Trigger initial data load
    loadPgQueriesPageData();
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
                if (data.collected_at) {
                    collectedAt = data.collected_at;
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

    // Render diagnostic charts only (snapshot table removed — merged into Top Queries)

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
        const fullSql = pgssSmartDecode(query.query || '');
        const sqlPreview = fullSql.substring(0, 60) + (fullSql.length > 60 ? '...' : '');
        const escPreview = window.escapeHtml(sqlPreview);
        const escUser = window.escapeHtml(user || '-');

        return `
            <tr>
                <td>${escUser || '<span class="text-muted">-</span>'}</td>
                <td style="max-width:350px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
                    <span class="pg-sql-preview" style="cursor:pointer; text-decoration:underline; text-underline-offset:2px; color:var(--accent);" title="View full SQL and capture time" data-action="call" data-fn="pgOpenQuerySql" data-arg="${fingerprint}">
                        ${escPreview}
                    </span>
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

// ============================================================
// pg_stat_statements Time-Series Section (merged)
// ============================================================
window._pgssCurrentSort = 'total_time';
window._pgssCurrentSortDir = 'desc';
window._pgssRefreshTimer = null;
window._pgssTopQueriesCache = [];

window.pgssSort = function (el) {
    const sort = el?.dataset?.sort || 'total_time';
    document.querySelectorAll('#pgss-sort-tabs li').forEach(t => t.classList.remove('active'));
    if (el) el.classList.add('active');
    window._pgssCurrentSort = sort;
    loadPgssTopQueries();
};

window.pgssHeaderSort = function(key) {
    const keyMap = {
        total_time: 'total_time_ms', pct_db: 'pct_db_time', calls: 'calls',
        avg_ms: 'avg_ms', rows_per_call: 'rows_per_call', plan_ratio: 'plan_ratio',
        reads_per_call: 'reads_per_call', hit_pct: 'hit_pct', temp_mb: 'temp_mb', wal_mb: 'wal_mb'
    };
    if (window._pgssCurrentSort === key) {
        window._pgssCurrentSortDir = window._pgssCurrentSortDir === 'desc' ? 'asc' : 'desc';
    } else {
        window._pgssCurrentSort = key;
        window._pgssCurrentSortDir = 'desc';
    }
    // Update header icons
    document.querySelectorAll('#pgss-top-table .pgss-sortable-th').forEach(th => {
        const icon = th.querySelector('i');
        if (!icon) return;
        if (th.dataset.sortKey === key) {
            th.dataset.sortDir = window._pgssCurrentSortDir;
            icon.className = window._pgssCurrentSortDir === 'desc' ? 'fa-solid fa-sort-down' : 'fa-solid fa-sort-up';
        } else {
            delete th.dataset.sortDir;
            icon.className = 'fa-solid fa-sort';
        }
    });
    // Client-side re-sort from cache
    const field = keyMap[key] || key;
    const dir = window._pgssCurrentSortDir === 'desc' ? -1 : 1;
    const sorted = [...window._pgssTopQueriesCache].sort((a, b) => {
        const va = a[field] ?? 0, vb = b[field] ?? 0;
        return (va - vb) * dir;
    });
    pgssRenderTopRows(sorted);
};

window.pgssApplyTimeRange = function () {
    loadPgssAll();
};

function initPgssSection(instanceName) {
    // Clear any previous auto-refresh timer
    if (window._pgssRefreshTimer) {
        clearInterval(window._pgssRefreshTimer);
        window._pgssRefreshTimer = null;
    }

    window.currentCharts = window.currentCharts || {};
    const pgssChartIds = ['pgss-chart-load','pgss-chart-qps','pgss-chart-cache','pgss-chart-latency','pgss-chart-wal','pgss-chart-execplan','pgss-chart-io-pressure'];
    pgssChartIds.forEach(id => pgssDestroyChart(id));

    const label = document.getElementById('pgss-instance-label');
    if (label) label.textContent = instanceName;

    checkPgssStatus(instanceName);
    loadPgssAll();

    // Wire up sortable column headers
    document.querySelectorAll('#pgss-top-table .pgss-sortable-th').forEach(th => {
        th.addEventListener('click', () => window.pgssHeaderSort(th.dataset.sortKey));
    });

    // Auto-refresh every 60 seconds
    window._pgssRefreshTimer = setInterval(() => {
        if (!document.getElementById('pgss-instance-label')) {
            clearInterval(window._pgssRefreshTimer);
            window._pgssRefreshTimer = null;
            return;
        }
        loadPgssAll();
    }, 60000);
}

function getPgssTimeRange() {
    const fromEl = document.getElementById('pgQueriesFrom');
    const toEl = document.getElementById('pgQueriesTo');
    const from = fromEl?.value ? new Date(fromEl.value).toISOString() : new Date(Date.now() - 3600000).toISOString();
    const to = toEl?.value ? new Date(toEl.value).toISOString() : new Date().toISOString();
    return { from, to };
}

async function checkPgssStatus(instance) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/pgss/status?instance=${encodeURIComponent(instance)}`);
        if (resp.ok) {
            const data = await resp.json();
            const banner = document.getElementById('pgss-status-banner');
            const msg = document.getElementById('pgss-status-msg');
            if (!data.ready && banner && msg) {
                banner.style.display = '';
                msg.textContent = data.message || 'pg_stat_statements is not available on this instance.';
            }
        }
    } catch (_) { /* non-fatal */ }
}

async function loadPgssAll() {
    await Promise.all([
        loadPgssSummary(),
        loadPgssWorkload(),
        loadPgssLatency(),
        loadPgssTopQueries(),
        loadPgssRegressions()
    ]);
}

async function loadPgssSummary() {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
    const { from, to } = getPgssTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/summary?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) return;
        const d = await resp.json();

        pgssSetKpi('pgss-kpi-load', d.query_load_ms_sec, 1, [500, 2000]);
        pgssSetKpi('pgss-kpi-qps', d.qps, 0, null);
        pgssSetKpi('pgss-kpi-p99', d.p99_ms, 2, [50, 200]);
        pgssSetKpi('pgss-kpi-cache', d.cache_hit_pct, 1, null, true);
        pgssSetKpi('pgss-kpi-temp', d.temp_mb_sec, 2, [1, 10]);
        pgssSetKpi('pgss-kpi-wal', d.wal_mb_sec, 2, [5, 50]);
    } catch (_) { /* non-fatal */ }
}

function pgssSetKpi(id, value, decimals, thresholds, invertThresholds) {
    const el = document.getElementById(id);
    if (!el) return;
    if (value == null || isNaN(value)) { el.textContent = '--'; return; }
    el.textContent = typeof value === 'number' ? value.toFixed(decimals) : value;

    const cell = document.getElementById(id + '-cell');
    if (!cell || !thresholds) return;
    cell.classList.remove('pgss-kpi-warn', 'pgss-kpi-critical');

    if (invertThresholds) {
        // lower is worse (e.g. cache hit %)
        if (value < thresholds[0]) cell.classList.add('pgss-kpi-critical');
        else if (value < thresholds[1]) cell.classList.add('pgss-kpi-warn');
    } else {
        // higher is worse
        if (value >= thresholds[1]) cell.classList.add('pgss-kpi-critical');
        else if (value >= thresholds[0]) cell.classList.add('pgss-kpi-warn');
    }
}

async function loadPgssWorkload() {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
    const { from, to } = getPgssTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/workload?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) return;
        const data = await resp.json();
        const pts = data.points || [];
        const labels = pts.map(p => new Date(p.ts).toLocaleTimeString());

        pgssRenderLineChart('pgss-chart-load', labels, [
            { label: 'Query Load ms/s', data: pts.map(p => p.query_load_ms_sec), borderColor: '#38bdf8', fill: true, backgroundColor: 'rgba(56,189,248,0.15)' },
            { label: 'CPU Saturation', data: pts.map(p => p.cpu_saturation_ms_sec || 4000), borderColor: '#ef4444', borderDash: [5, 3], borderWidth: 1, pointRadius: 0, fill: false }
        ]);
        pgssRenderLineChart('pgss-chart-qps', labels, [
            { label: 'QPS', data: pts.map(p => p.qps), borderColor: '#a78bfa', yAxisID: 'y' },
            { label: 'Rows/s', data: pts.map(p => p.rows_sec), borderColor: '#34d399', yAxisID: 'y1' }
        ], true);
        pgssRenderLineChart('pgss-chart-cache', labels, [
            { label: 'Cache Hit %', data: pts.map(p => p.cache_hit_ratio), borderColor: '#facc15', fill: true, backgroundColor: 'rgba(250,204,21,0.1)' }
        ]);
        // I/O Pressure (stacked: shared reads + temp writes)
        pgssRenderStackedArea('pgss-chart-io-pressure', labels, [
            { label: 'Blk Read/s', data: pts.map(p => p.blks_read_sec || 0), backgroundColor: 'rgba(248,113,113,0.45)' },
            { label: 'Temp Write/s', data: pts.map(p => p.temp_blks_written_sec || 0), backgroundColor: 'rgba(245,158,11,0.4)' }
        ]);
        pgssRenderLineChart('pgss-chart-wal', labels, [
            { label: 'WAL bytes/s', data: pts.map(p => p.wal_bytes_sec), borderColor: '#f87171', fill: true, backgroundColor: 'rgba(248,113,113,0.12)' }
        ]);
        pgssRenderStackedArea('pgss-chart-execplan', labels, [
            { label: 'Exec %', data: pts.map(p => p.exec_pct), backgroundColor: 'rgba(56,189,248,0.5)' },
            { label: 'Plan %', data: pts.map(p => p.plan_pct), backgroundColor: 'rgba(250,204,21,0.5)' }
        ]);
        // Show planning warning if any point has plan_pct > 10
        const planWarn = document.getElementById('pgss-planning-warning');
        if (planWarn) {
            const hasHighPlan = pts.some(p => (p.plan_pct || 0) > 10);
            planWarn.style.display = hasHighPlan ? '' : 'none';
        }
    } catch (_) { /* non-fatal */ }
}

async function loadPgssLatency() {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
    const { from, to } = getPgssTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/latency?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) return;
        const data = await resp.json();
        const pts = data.points || [];
        const labels = pts.map(p => new Date(p.ts).toLocaleTimeString());
        pgssRenderLineChart('pgss-chart-latency', labels, [
            { label: 'p50', data: pts.map(p => p.p50), borderColor: '#34d399' },
            { label: 'p95', data: pts.map(p => p.p95), borderColor: '#facc15' },
            { label: 'p99', data: pts.map(p => p.p99), borderColor: '#f87171' }
        ]);
    } catch (_) { /* non-fatal */ }
}

async function loadPgssTopQueries() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: '' };
    const { from, to } = getPgssTimeRange();
    const sort = window._pgssCurrentSort || 'total_time';
    const grid = document.getElementById('pgss-top-grid');
    if (!grid) return;
    
    // Remove existing rows (keep header which is first child)
    const header = grid.querySelector('.pgss-grid-header');
    grid.innerHTML = '';
    if (header) grid.appendChild(header);

    const loadingDiv = document.createElement('div');
    loadingDiv.className = 'text-center p-4';
    loadingDiv.style.gridColumn = '1 / -1';
    loadingDiv.innerHTML = '<div class="spinner"></div>';
    grid.appendChild(loadingDiv);

    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/top?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}&sort=${sort}`
        );
        if (!resp.ok) { 
            loadingDiv.innerHTML = 'Error loading data'; 
            return; 
        }
        const data = await resp.json();
        const queries = data.queries || [];

        // Show time range on the table header
        const rangeEl = document.getElementById('pgss-top-time-range');
        if (rangeEl) {
            const fmtFrom = new Date(from).toLocaleString();
            const fmtTo = new Date(to).toLocaleString();
            rangeEl.textContent = fmtFrom + ' → ' + fmtTo;
        }

        grid.removeChild(loadingDiv);

        if (queries.length === 0) {
            window._pgssTopQueriesCache = [];
            const empty = document.createElement('div');
            empty.className = 'text-center text-muted p-4';
            empty.style.gridColumn = '1 / -1';
            empty.textContent = 'No data for selected time range';
            grid.appendChild(empty);
            return;
        }
        // Store query texts for detail modal
        window._pgssQueryTexts = {};
        queries.forEach(q => { if (q.query_id) window._pgssQueryTexts[String(q.query_id)] = q.query || ''; });
        window._pgssTopQueriesCache = queries;

        pgssRenderTopRows(queries);
    } catch (e) { 
        console.error('loadPgssTopQueries failed', e);
        loadingDiv.innerHTML = 'Error'; 
    }
}

function pgssRenderTopRows(queries) {
    const grid = document.getElementById('pgss-top-grid');
    if (!grid || !queries) return;
    
    // Remove existing rows (keep header which is first child)
    const header = grid.querySelector('.pgss-grid-header');
    grid.innerHTML = '';
    if (header) grid.appendChild(header);

    queries.forEach((q, i) => {
        const flags = (q.flags || []).map(f => {
            const lower = f.toLowerCase();
            return `<span class="pgss-badge pgss-badge-${pgssEscapeHtml(lower)}">${pgssEscapeHtml(f)}</span>`;
        }).join('');
        const qid = q.query_id != null ? String(q.query_id) : '';
        
        const row = document.createElement('div');
        row.className = 'pgss-grid-row';
        row.innerHTML = `
            <div>${i + 1}</div>
            <div style="justify-content:center;">${flags || '<span class="text-muted">—</span>'}</div>
            <div class="pgss-grid-cell-query" data-action="call" data-fn="pgssOpenQuery" data-arg="${qid}" title="Click to view full SQL">
                ${pgssEscapeHtml(pgssTrancate(pgssSmartDecode(q.query), 80))}
            </div>
            <div class="pgss-grid-cell-stat">${pgssFmtMs(q.total_time_ms)}</div>
            <div class="pgss-grid-cell-stat">${q.pct_db_time != null ? q.pct_db_time.toFixed(1) + '%' : '-'}</div>
            <div class="pgss-grid-cell-stat">${pgssFmtNum(q.calls)}</div>
            <div class="pgss-grid-cell-stat" style="font-weight:600;color:var(--accent);">${pgssFmtMs(q.avg_ms)}</div>
            <div class="pgss-grid-cell-stat">${q.rows_per_call?.toFixed(1) ?? '-'}</div>
            <div class="pgss-grid-cell-stat">${q.plan_ratio != null ? (q.plan_ratio * 100).toFixed(1) + '%' : '-'}</div>
            <div class="pgss-grid-cell-stat">${q.reads_per_call?.toFixed(1) ?? '-'}</div>
            <div class="pgss-grid-cell-stat">${q.hit_pct?.toFixed(1) ?? '-'}%</div>
            <div class="pgss-grid-cell-stat">${Number(q.temp_mb || 0).toFixed(2)}</div>
            <div class="pgss-grid-cell-stat">${Number(q.wal_mb || 0).toFixed(2)}</div>
        `;
        grid.appendChild(row);
    });
}

window.pgssOpenQuery = function(queryId) {
    const qid = String(queryId || '');
    const sql = (window._pgssQueryTexts && window._pgssQueryTexts[qid]) || '';
    const { from, to } = getPgssTimeRange();
    window.showPgQueryDetails('', sql, { window_from: from, window_to: to });
};

async function loadPgssRegressions() {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
    const tbody = document.getElementById('pgss-regression-tbody');
    if (!tbody) return;
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/regressions?instance=${encodeURIComponent(inst.name)}`
        );
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="5">Error loading</td></tr>'; return; }
        const data = await resp.json();
        const regs = data.regressions || [];
        if (regs.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" class="text-muted">No regressions detected</td></tr>';
            return;
        }
        tbody.innerHTML = regs.map(r => {
            const decodedSql = pgssSmartDecode(r.query || '');
            return `
            <tr>
                <td style="max-width:420px; cursor:pointer; text-decoration:underline; text-underline-offset:2px;" title="Click to view full query" data-action="call" data-fn="pgssOpenRegressionQuery" data-arg="${pgssEscapeHtml(decodedSql)}">${pgssEscapeHtml(pgssTrancate(decodedSql, 80))}</td>
                <td>${pgssFmtMs(r.prev_avg_ms)}</td>
                <td>${pgssFmtMs(r.curr_avg_ms)}</td>
                <td class="${r.change_pct > 50 ? 'text-danger' : r.change_pct > 20 ? 'text-warning' : ''}">+${r.change_pct.toFixed(0)}%</td>
                <td><span class="badge ${r.status === 'Degraded' ? 'badge-danger' : 'badge-warning'}">${r.status}</span></td>
                <td>${r.detected_at ? new Date(r.detected_at).toLocaleTimeString() : '-'}</td>
            </tr>
        `; }).join('');
    } catch (_) { tbody.innerHTML = '<tr><td colspan="5">Error</td></tr>'; }
}

window.pgssOpenRegressionQuery = function(sql) {
    window.showPgQueryDetails('', sql || '', window._pgQueriesMeta || {});
};

// ---- pgss chart helpers ----
function pgssRenderLineChart(canvasId, labels, datasets, dualAxis) {
    pgssDestroyChart(canvasId);
    const ctx = document.getElementById(canvasId);
    if (!ctx) return;
    const cfg = {
        type: 'line',
        data: { labels, datasets: datasets.map(ds => ({ tension: 0.3, pointRadius: 0, borderWidth: 1.5, fill: ds.fill || false, ...ds })) },
        options: {
            responsive: true, maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            plugins: { legend: { labels: { color: '#94a3b8', font: { size: 11 } } } },
            scales: {
                x: { 
                    title: { display: true, text: 'Time (Last 60 Minutes)', color: '#64748b', font: { size: 10 } },
                    ticks: { color: '#64748b', maxTicksLimit: 12 }, 
                    grid: { color: 'rgba(100,116,139,0.15)' } 
                },
                y: { ticks: { color: '#64748b' }, grid: { color: 'rgba(100,116,139,0.15)' } }
            }
        }
    };
    if (dualAxis) {
        cfg.options.scales.y1 = { position: 'right', ticks: { color: '#64748b' }, grid: { drawOnChartArea: false } };
    }
    window.currentCharts[canvasId] = new Chart(ctx, cfg);
}

function pgssRenderStackedArea(canvasId, labels, datasets) {
    pgssDestroyChart(canvasId);
    const ctx = document.getElementById(canvasId);
    if (!ctx) return;
    window.currentCharts[canvasId] = new Chart(ctx, {
        type: 'line',
        data: { labels, datasets: datasets.map(ds => ({ fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1, ...ds })) },
        options: {
            responsive: true, maintainAspectRatio: false,
            plugins: { legend: { labels: { color: '#94a3b8', font: { size: 11 } } } },
            scales: {
                x: { 
                    title: { display: true, text: 'Time (Last 60 Minutes)', color: '#64748b', font: { size: 10 } },
                    stacked: true, 
                    ticks: { color: '#64748b', maxTicksLimit: 12 }, 
                    grid: { color: 'rgba(100,116,139,0.15)' } 
                },
                y: { stacked: true, max: 100, ticks: { color: '#64748b' }, grid: { color: 'rgba(100,116,139,0.15)' } }
            }
        }
    });
}

function pgssDestroyChart(id) {
    if (window.currentCharts && window.currentCharts[id]) {
        window.currentCharts[id].destroy();
        delete window.currentCharts[id];
    }
}

// ---- pgss formatting helpers ----
function pgssEscapeHtml(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}
function pgssTrancate(s, n) {
    if (!s) return '';
    return s.length > n ? s.slice(0, n) + '…' : s;
}
function pgssFmtMs(ms) {
    if (ms == null) return '-';
    if (ms >= 1000) return (ms / 1000).toFixed(2) + ' s';
    return ms.toFixed(2) + ' ms';
}
function pgssFmtNum(n) {
    if (n == null) return '-';
    return n.toLocaleString();
}
function pgssSmartDecode(s) {
    if (!s) return '';
    try {
        if (s.indexOf('%20') !== -1 || s.indexOf('%0A') !== -1) {
            return decodeURIComponent(s).replace(/\+/g, ' ');
        }
    } catch (e) {}
    return s;
}
