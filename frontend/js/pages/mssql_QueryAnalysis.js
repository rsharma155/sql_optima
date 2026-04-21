/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Query Analysis Dashboard — regressions, plan instability,
 *          top queries, live queries, trend charts and summary KPIs powered by Query Store.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function () {
    /* ── helpers ─────────────────────────────────────────────── */
    const esc = (s) => window.escapeHtml ? window.escapeHtml(String(s ?? '')) : String(s ?? '');
    const fmtNum = (n) => (n == null || isNaN(n)) ? '--' : Number(n).toLocaleString(undefined, { maximumFractionDigits: 2 });
    const fmtMs = (n) => (n == null || isNaN(n)) ? '--' : `${Number(n).toLocaleString(undefined, { maximumFractionDigits: 1 })} ms`;
    const fmtPct = (n) => (n == null || isNaN(n)) ? '--' : `${Number(n).toFixed(1)}%`;
    const fmtK = (n) => {
        if (n == null || isNaN(n)) return '--';
        n = Number(n);
        if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
        if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
        return n.toLocaleString(undefined, { maximumFractionDigits: 0 });
    };

    const setSafeText = (id, val) => {
        const el = document.getElementById(id);
        if (el) el.textContent = val;
    };

    /* ── State ── */
    let _charts = {};
    let _topRows = [];
    let _regRows = [];
    let _instabRows = [];
    let _liveRows = [];
    let _schedulerRows = [];
    let _watchedHashes = new Set();
    let _searchDebounce = null;
    window._qaSort = window._qaSort || {
        'top-queries': { col: 'total_cpu_ms', dir: 'desc' },
        'regressions': { col: 'percent_change', dir: 'desc' },
        'instability': { col: 'plan_count', dir: 'desc' },
        'live': { col: 'cpu_time_ms', dir: 'desc' }
    };

    function destroyCharts() {
        Object.values(_charts).forEach(c => { try { c.destroy(); } catch (_) {} });
        _charts = {};
    }

    /* ── main view ──────────────────────────────────────────── */
    async function loadQueryAnalysis() {
        destroyCharts();
        _topRows = []; _regRows = []; _instabRows = []; _liveRows = []; _schedulerRows = []; _watchedHashes = new Set();
        window.appState.queryCache = window.appState.queryCache || {};
        
        const outlet = window.routerOutlet;
        if (!outlet) return;
        
        const inst = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
        if (!inst || String(inst.type || '').toLowerCase() !== 'sqlserver') {
            outlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select a SQL Server instance.</h3></div>`;
            return;
        }
        const instName = inst.name;

        // Load modern template
        outlet.innerHTML = await window.loadTemplate('/pages/sqlserver_query_analysis.html');

        // UI Setup
        document.getElementById('qaInstanceName').textContent = instName;
        const fromEl = document.getElementById('qaFrom');
        const toEl = document.getElementById('qaTo');
        const excludeSystemEl = document.getElementById('qaExcludeSystem');
        
        const now = new Date();
        const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
        
        const pad = (n) => String(n).padStart(2, '0');
        const fmtLocal = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
        
        fromEl.value = fmtLocal(oneHourAgo);
        toEl.value = fmtLocal(now);

        // Bind Actions
        window.qaRefreshAll = fetchActiveTab;
        document.getElementById('qaApplyRange').onclick = fetchActiveTab;
        
        const tabs = document.querySelectorAll('.qa-tab-btn');
        tabs.forEach(btn => {
            btn.onclick = () => {
                tabs.forEach(t => t.classList.remove('active'));
                btn.classList.add('active');
                document.querySelectorAll('.qa-tab-panel').forEach(p => p.style.display = 'none');
                document.getElementById(btn.dataset.tab).style.display = 'block';
                fetchActiveTab();
            };
        });

        const searchInput = document.getElementById('qaSearch');
        if (searchInput) {
            searchInput.addEventListener('input', (e) => {
                clearTimeout(_searchDebounce);
                _searchDebounce = setTimeout(() => {
                    renderActiveTab();
                }, 300);
            });
        }

        async function fetchActiveTab() {
            const activeTab = document.querySelector('.qa-tab-btn.active').dataset.tab;
            const from = new Date(fromEl.value).toISOString();
            const to = new Date(toEl.value).toISOString();
            const excludeSystem = excludeSystemEl.checked;
            const dbName = window.appState.currentDatabase || 'all';
            
            // Always refresh summary for KPIs
            fetchSummary(from, to, excludeSystem, dbName);
            // And fetch current watch list so we show filled stars
            await fetchWatchedHashes();

            if (activeTab === 'top-queries') {
                await fetchTopQueries(from, to, excludeSystem, dbName);
            } else if (activeTab === 'regressions') {
                await fetchRegressions(dbName);
            } else if (activeTab === 'instability') {
                await fetchPlanInstability(dbName);
            } else if (activeTab === 'live') {
                await Promise.all([
                    fetchLiveQueries(dbName),
                    fetchSchedulerStats()
                ]);
            }
            renderActiveTab();
            bindActiveTabEvents(activeTab);
        }

        async function fetchSchedulerStats() {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/mssql/cpu-scheduler-stats?instance=${encodeURIComponent(instName)}&limit=100`);
                if (!resp.ok) return;
                const data = await resp.json();
                _schedulerRows = data.cpu_scheduler_stats || [];
            } catch (e) { console.error('scheduler stats failed', e); }
        }

        async function fetchWatchedHashes() {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/mssql/watched-queries?instance=${encodeURIComponent(instName)}`, { cache: 'no-store' });
                if (resp.ok) {
                    const data = await resp.json();
                    _watchedHashes = new Set((data.watched_queries || []).map(q => q.query_hash));
                }
            } catch (e) { console.error('failed to fetch watched hashes', e); }
        }

        function renderActiveTab() {
            const activeTab = document.querySelector('.qa-tab-btn.active').dataset.tab;
            if (activeTab === 'top-queries') renderTopQueries();
            else if (activeTab === 'regressions') renderRegressions();
            else if (activeTab === 'instability') renderInstability();
            else if (activeTab === 'live') renderLive();
        }

        async function fetchSummary(from, to, excludeSystem, dbName) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/mssql/query-analysis/summary?instance=${encodeURIComponent(instName)}&from=${from}&to=${to}&exclude_system=${excludeSystem}&database=${encodeURIComponent(dbName)}`);
                if (!resp.ok) return;
                const data = await resp.json();
                updateKpis(data);
                updateSummaryStrip(data);
            } catch (e) { console.error('summary fetch failed', e); }
        }

        function updateKpis(data) {
            setSafeText('kpi-executions', fmtK(data.total_executions));
            setSafeText('kpi-duration', fmtMs(data.avg_duration_ms));
            setSafeText('kpi-cpu', fmtMs(data.avg_cpu_ms));
            setSafeText('kpi-regressions', data.regressions_24h || 0);
            setSafeText('kpi-plan-changes', data.plan_changes_24h || 0);
            const share = data.top_10_cpu_share_pct != null ? data.top_10_cpu_share_pct : '--';
            setSafeText('kpi-cpu-share', (share !== '--') ? share.toFixed(1) + '%' : '--');
        }

        function updateSummaryStrip(data) {
            setSafeText('summary-total-queries', fmtK(data.total_queries_in_qs || 0));
            setSafeText('summary-executed-range', fmtK(data.queries_executed_in_range || 0));
            setSafeText('summary-multi-plan', data.queries_with_multi_plans || 0);
            setSafeText('summary-single-exec', data.queries_single_execution || 0);
        }

        async function fetchTrendCharts(from, to, dbName) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/timescale/mssql/query-stats-timeseries?instance=${encodeURIComponent(instName)}&from=${from}&to=${to}&database=${encodeURIComponent(dbName)}`);
                if (!resp.ok) return;
                const data = await resp.json();
                renderWorkloadChart(data.series || []);
                renderLatencyChart(data.series || []);
            } catch (e) { console.error('trend charts failed', e); }
        }

        async function fetchTopQueries(from, to, excludeSystem, dbName) {
            const body = document.getElementById('qaTopQueriesBody');
            body.innerHTML = '<div class="text-muted p-3"><i class="fa-solid fa-spinner fa-spin"></i> Loading top queries...</div>';
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/mssql/query-analysis/top-queries?instance=${encodeURIComponent(instName)}&from=${from}&to=${to}&exclude_system=${excludeSystem}&limit=50&database=${encodeURIComponent(dbName)}`);
                if (!resp.ok) throw new Error('HTTP ' + resp.status);
                const data = await resp.json();
                _topRows = data.queries || data || [];
                fetchTrendCharts(from, to, dbName);
            } catch (e) {
                body.innerHTML = `<div class="text-danger p-3">Failed to load top queries: ${e.message}</div>`;
            }
        }

        function renderTopQueries() {
            const container = document.getElementById('qaTopQueriesBody');
            if (!_topRows.length) {
                container.innerHTML = `<div class="text-muted p-4 text-center">No queries found for this period.</div>`;
                return;
            }
            const q = document.getElementById('qaSearch')?.value.toLowerCase() || '';
            const filtered = _topRows.filter(r => (r.query_text || '').toLowerCase().includes(q) || (r.query_hash || '').toLowerCase().includes(q));
            container.innerHTML = renderTopQueriesTable(filtered);
            renderContributionChart(_topRows.slice(0, 10));
        }

        function renderRegressions() {
            const container = document.getElementById('qaRegressionsBody');
            if (!_regRows.length) {
                container.innerHTML = `<div class="text-muted p-4 text-center">No regressions detected.</div>`;
                return;
            }
            container.innerHTML = renderRegressionsTable(_regRows);
        }

        function renderInstability() {
            const container = document.getElementById('qaInstabilityBody');
            if (!_instabRows.length) {
                container.innerHTML = `<div class="text-muted p-4 text-center">No plan instability detected.</div>`;
                return;
            }
            container.innerHTML = renderPlanInstabilityTable(_instabRows);
        }

        function renderLive() {
            const container = document.getElementById('qaLiveBody');
            if (!_liveRows.length) {
                container.innerHTML = `<div class="text-muted p-4 text-center">No live sessions found.</div>`;
                return;
            }
            container.innerHTML = `
                <div class="chart-card glass-panel mb-3" style="height:200px;">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-chart-line"></i> CPU Scheduler History (Last 100 Samples)</h3></div>
                    <div class="chart-container" style="height:160px;"><canvas id="qaSchedulerHistoryChart"></canvas></div>
                </div>
                ${renderLiveQueriesTable(_liveRows)}
            `;
            setTimeout(() => renderSchedulerChart(_schedulerRows), 50);
        }

        function renderSchedulerChart(schedulerStats) {
            const ctx = document.getElementById('qaSchedulerHistoryChart')?.getContext('2d');
            if (!ctx || !schedulerStats.length) return;
            if (_charts.scheduler) _charts.scheduler.destroy();

            const labels = schedulerStats.slice().reverse().map(s => new Date(s.capture_timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
            const workerData = schedulerStats.slice().reverse().map(s => s.total_current_workers_count || 0);
            const runnableData = schedulerStats.slice().reverse().map(s => s.total_runnable_tasks_count || 0);
            const queueData = schedulerStats.slice().reverse().map(s => s.total_work_queue_count || 0);

            _charts.scheduler = new Chart(ctx, {
                type: 'line',
                data: {
                    labels,
                    datasets: [
                        { label: 'Workers', data: workerData, borderColor: '#3b82f6', tension: 0.3, fill: false, pointRadius: 0 },
                        { label: 'Runnable', data: runnableData, borderColor: '#f59e0b', tension: 0.3, fill: false, pointRadius: 0 },
                        { label: 'Queue', data: queueData, borderColor: '#ef4444', tension: 0.3, fill: false, pointRadius: 0 }
                    ]
                },
                options: {
                    responsive: true, maintainAspectRatio: false,
                    plugins: { legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } },
                    scales: {
                        x: { grid: { display: false }, ticks: { font: { size: 9 }, maxTicksLimit: 10 } },
                        y: { beginAtZero: true, ticks: { font: { size: 9 } } }
                    }
                }
            });
        }

        function bindActiveTabEvents(tabId) {
            const container = document.getElementById(tabId);
            if (!container || container._bound) return; 
            
            container.addEventListener('click', async (e) => {
                const target = e.target.closest('[data-action]');
                if (!target) return;
                const action = target.dataset.action;

                if (action === 'sort-table') {
                    const col = target.dataset.col;
                    const tab = target.dataset.tab;
                    const current = window._qaSort[tab] || { col: '', dir: 'desc' };
                    window._qaSort[tab] = {
                        col: col,
                        dir: (current.col === col && current.dir === 'desc') ? 'asc' : 'desc'
                    };
                    renderActiveTab();
                } else if (action === 'show-query-modal-direct') {
                    const obj = window.appState.queryCache[target.dataset.key];
                    if (window[target.dataset.fn]) window[target.dataset.fn](obj);
                    else if (window.showQueryModal) window.showQueryModal(obj.text);
                } else if (action === 'watch-query') {
                    e.stopPropagation();
                    watchQuery(target);
                }
            });
            container._bound = true;
        }

        async function watchQuery(btn) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/mssql/watched-queries?instance=${encodeURIComponent(instName)}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ 
                        query_hash: btn.dataset.hash, 
                        name: btn.dataset.label, 
                        database_name: btn.dataset.db,
                        query_text: btn.dataset.queryText 
                    })
                });
                if (resp.ok) {
                    btn.innerHTML = '<i class="fa-solid fa-star"></i>';
                    btn.classList.add('watched');
                    btn.disabled = true;
                    alert('Query added to watch list.');
                } else if (resp.status === 409) {
                    btn.innerHTML = '<i class="fa-solid fa-star"></i>';
                    btn.classList.add('watched');
                    btn.disabled = true;
                    alert('This query is already in your watch list.');
                } else if (resp.status === 403) {
                    const err = await resp.json();
                    alert(err.error || 'Maximum of 10 watched queries per instance reached.');
                } else {
                    const err = await resp.json();
                    alert('Failed to watch: ' + (err.error || 'Unknown error'));
                }
            } catch (e) { console.error('watch failed', e); }
        }

        function sortTh(activeTab, col, label, extraClass) {
            const currentSort = window._qaSort?.[activeTab] || { col: '', dir: 'desc' };
            const active = currentSort.col === col;
            const icon = active ? (currentSort.dir === 'desc' ? 'fa-arrow-down' : 'fa-arrow-up') : 'fa-sort';
            return `<th class="sortable${active ? ' sort-active' : ''}${extraClass ? ' ' + extraClass : ''}" 
                        style="cursor:pointer" data-action="sort-table" data-tab="${activeTab}" data-col="${esc(col)}">
                        ${label} <i class="fa-solid ${icon} sort-icon" style="font-size:0.6rem; opacity:0.5;"></i>
                    </th>`;
        }

        function applySort(rows, activeTab) {
            const sort = window._qaSort?.[activeTab];
            if (!sort || !sort.col) return rows;
            return [...rows].sort((a, b) => {
                let va = a[sort.col], vb = b[sort.col];
                if (typeof va === 'string') va = va.toLowerCase();
                if (typeof vb === 'string') vb = vb.toLowerCase();
                if (va < vb) return sort.dir === 'desc' ? 1 : -1;
                if (va > vb) return sort.dir === 'desc' ? -1 : 1;
                return 0;
            });
        }

        /* ── Tables ── */
        function renderTopQueriesTable(rows) {
            const sorted = applySort(rows, 'top-queries');
            return `<div class="table-responsive"><table class="qa-table">
                <thead><tr>
                    <th style="width:2.5rem; text-align:center;"></th>
                    <th>Query</th>
                    ${sortTh('top-queries', 'executions_per_sec', 'Exec/sec', 'text-right')}
                    ${sortTh('top-queries', 'executions', 'Executions', 'text-right')}
                    ${sortTh('top-queries', 'avg_cpu_ms', 'Avg CPU', 'text-right')}
                    ${sortTh('top-queries', 'total_cpu_ms', 'Total CPU', 'text-right')}
                    ${sortTh('top-queries', 'plan_count', 'Plans', 'text-center')}
                    ${sortTh('top-queries', 'last_execution_time', 'Last Execution', 'text-right')}
                    <th>Database</th>
                </tr></thead>
                <tbody>${sorted.map((r, i) => {
                    const cacheKey = 'qa_top_' + i;
                    window.appState.queryCache[cacheKey] = { text: r.query_text, query_hash: r.query_hash, database_name: r.database_name };
                    const isWatched = _watchedHashes.has(r.query_hash);
                    return `<tr>
                    <td><button class="qa-watch-btn ${isWatched ? 'watched' : ''}" data-action="watch-query" data-hash="${esc(r.query_hash)}" data-db="${esc(r.database_name)}" data-label="${esc((r.query_text || '').substring(0, 80))}" data-query-text="${esc(r.query_text || '')}" ${isWatched ? 'disabled' : ''}>
                        <i class="fa-${isWatched ? 'solid' : 'regular'} fa-star"></i>
                    </button></td>
                    <td style="max-width:300px; cursor:pointer;" data-action="show-query-modal-direct" data-key="${cacheKey}" data-fn="showQueryStoreQueryModal">
                        <span class="qa-query-text" title="Click to view full SQL">${esc(r.query_text)}</span>
                    </td>
                    <td class="text-right font-mono">${fmtNum(r.executions_per_sec || 0)}</td>
                    <td class="text-right font-mono">${fmtK(r.executions)}</td>
                    <td class="text-right font-mono">${fmtMs(r.avg_cpu_ms)}</td>
                    <td class="text-right font-mono" style="color:var(--accent); font-weight:700;">${fmtMs(r.total_cpu_ms)}</td>
                    <td class="text-center"><span class="qa-badge ${r.plan_count > 1 ? 'qa-badge--warn' : 'qa-badge--default'}">${r.plan_count}</span></td>
                    <td class="text-right text-muted" style="font-size:0.65rem;">${r.last_execution_time ? new Date(r.last_execution_time).toLocaleString() : '--'}</td>
                    <td class="text-muted" style="font-size:0.7rem;">${esc(r.database_name)}</td>
                </tr>`}).join('')}</tbody></table></div>`;
        }

        function renderRegressionsTable(rows) {
            const sorted = applySort(rows, 'regressions');
            return `<div class="table-responsive"><table class="qa-table">
                <thead><tr>
                    <th>Query</th>
                    ${sortTh('regressions', 'regression_type', 'Type', '')}
                    ${sortTh('regressions', 'previous_avg', 'Prev Avg', 'text-right')}
                    ${sortTh('regressions', 'current_avg', 'Curr Avg', 'text-right')}
                    ${sortTh('regressions', 'percent_change', '% Change', 'text-right')}
                    <th class="text-center">Plan Changed</th>
                    <th></th>
                </tr></thead>
                <tbody>${sorted.map((r, i) => {
                    const cacheKey = 'qa_reg_' + i;
                    window.appState.queryCache[cacheKey] = { text: r.query_text, query_hash: r.query_hash, database_name: r.database_name };
                    const isWatched = _watchedHashes.has(r.query_hash);
                    return `<tr>
                    <td style="max-width:300px; cursor:pointer;" data-action="show-query-modal-direct" data-key="${cacheKey}" data-fn="showQueryStoreQueryModal">
                        <span class="qa-query-text" title="Click to view full SQL">${esc(r.query_text)}</span>
                    </td>
                    <td><span class="qa-badge qa-badge--default">${esc(r.regression_type)}</span></td>
                    <td class="text-right font-mono">${fmtMs(r.previous_avg)}</td>
                    <td class="text-right font-mono" style="font-weight:700;">${fmtMs(r.current_avg)}</td>
                    <td class="text-right"><i class="fa-solid fa-arrow-up text-danger"></i> ${fmtPct(r.percent_change)}</td>
                    <td class="text-center">${r.plan_changed ? '<i class="fa-solid fa-circle-check text-success"></i>' : '—'}</td>
                    <td><button class="qa-watch-btn ${isWatched ? 'watched' : ''}" data-action="watch-query" data-hash="${esc(r.query_hash)}" data-db="${esc(r.database_name)}" data-label="${esc((r.query_text || '').substring(0, 80))}" data-query-text="${esc(r.query_text || '')}" ${isWatched ? 'disabled' : ''}>
                        <i class="fa-${isWatched ? 'solid' : 'regular'} fa-star"></i>
                    </button></td>
                </tr>`}).join('')}</tbody></table></div>`;
        }

        function renderPlanInstabilityTable(rows) {
            const sorted = applySort(rows, 'instability');
            return `<div class="table-responsive"><table class="qa-table">
                <thead><tr>
                    <th>Query</th>
                    ${sortTh('instability', 'plan_count', 'Plan Count', 'text-center')}
                    ${sortTh('instability', 'last_execution_time', 'Last Execution', 'text-right')}
                    <th>Database</th>
                    <th></th>
                </tr></thead>
                <tbody>${sorted.map((r, i) => {
                    const cacheKey = 'qa_inst_' + i;
                    window.appState.queryCache[cacheKey] = { text: r.query_text, query_hash: r.query_hash, database_name: r.database_name };
                    const isWatched = _watchedHashes.has(r.query_hash);
                    return `<tr>
                    <td style="max-width:350px; cursor:pointer;" data-action="show-query-modal-direct" data-key="${cacheKey}" data-fn="showQueryStoreQueryModal">
                        <span class="qa-query-text" title="Click to view full SQL">${esc(r.query_text)}</span>
                    </td>
                    <td class="text-center"><span class="qa-badge qa-badge--warn" style="font-size:0.8rem;">${r.plan_count}</span></td>
                    <td class="text-right text-muted" style="font-size:0.65rem;">${r.last_execution_time ? new Date(r.last_execution_time).toLocaleString() : '--'}</td>
                    <td class="text-muted" style="font-size:0.7rem;">${esc(r.database_name)}</td>
                    <td><button class="qa-watch-btn ${isWatched ? 'watched' : ''}" data-action="watch-query" data-hash="${esc(r.query_hash)}" data-db="${esc(r.database_name)}" data-label="${esc((r.query_text || '').substring(0, 80))}" data-query-text="${esc(r.query_text || '')}" ${isWatched ? 'disabled' : ''}>
                        <i class="fa-${isWatched ? 'solid' : 'regular'} fa-star"></i>
                    </button></td>
                </tr>`}).join('')}</tbody></table></div>`;
        }

        function renderLiveQueriesTable(rows) {
            const sorted = applySort(rows, 'live');
            return `<div class="table-responsive"><table class="qa-table">
                <thead><tr>
                    ${sortTh('live', 'session_id', 'SPID', '')}
                    ${sortTh('live', 'status', 'Status', '')}
                    ${sortTh('live', 'total_elapsed_time_ms', 'Duration', 'text-right')}
                    ${sortTh('live', 'cpu_time_ms', 'CPU', 'text-right')}
                    <th>Wait Type</th>
                    <th>Blocking</th>
                    <th>User / App</th>
                    <th>Query Text</th>
                </tr></thead>
                <tbody>${sorted.map((r, i) => {
                    const cacheKey = 'qa_live_' + i;
                    window.appState.queryCache[cacheKey] = { text: r.query_text, query_hash: r.query_hash, database_name: r.database_name };
                    return `<tr>
                    <td style="font-weight:700; color:var(--accent);">${esc(r.session_id)}</td>
                    <td><span class="qa-badge qa-badge--default">${esc(r.status)}</span></td>
                    <td class="text-right font-mono">${fmtMs(r.total_elapsed_time_ms)}</td>
                    <td class="text-right font-mono">${fmtMs(r.cpu_time_ms)}</td>
                    <td><span class="qa-badge ${r.wait_type?.startsWith('LCK') ? 'qa-badge--danger' : 'qa-badge--warn'}">${esc(r.wait_type)}</span></td>
                    <td class="text-danger" style="font-weight:700;">${r.blocking_session_id || ''}</td>
                    <td class="text-muted" style="font-size:0.65rem;">
                        <div style="font-weight:600; color:var(--text);">${esc(r.login_name)}</div>
                        <div>${esc(r.program_name)}</div>
                    </td>
                    <td style="max-width:250px; cursor:pointer;" data-action="show-query-modal-direct" data-key="${cacheKey}" data-fn="showQueryStoreQueryModal">
                        <span class="qa-query-text" title="${esc(r.query_text)}">${esc(r.query_text)}</span>
                    </td>
                </tr>`}).join('')}</tbody></table></div>`;
        }

        /* ── Trend Charts ── */
        function renderWorkloadChart(series) {
            const ctx = document.getElementById('qaWorkloadChart')?.getContext('2d');
            if (!ctx) return;
            if (_charts.workload) _charts.workload.destroy();
            const labels = series.map(p => new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
            _charts.workload = new Chart(ctx, {
                type: 'line',
                data: {
                    labels,
                    datasets: [
                        { label: 'CPU (ms)', data: series.map(p => p.cpu_ms), borderColor: '#f43f5e', tension: 0.3, fill: false, pointRadius: 0 },
                        { label: 'Reads', data: series.map(p => p.logical_reads), borderColor: '#10b981', tension: 0.3, fill: false, pointRadius: 0, yAxisID: 'y1' }
                    ]
                },
                options: {
                    responsive: true, maintainAspectRatio: false,
                    interaction: { mode: 'index', intersect: false },
                    plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } },
                    scales: {
                        x: { display: true, grid: { display: false }, ticks: { font: { size: 9 }, maxTicksLimit: 8 } },
                        y: { beginAtZero: true, ticks: { font: { size: 9 } } },
                        y1: { position: 'right', beginAtZero: true, grid: { display: false }, ticks: { font: { size: 9 } } }
                    }
                }
            });
        }

        function renderLatencyChart(series) {
            const ctx = document.getElementById('qaLatencyChart')?.getContext('2d');
            if (!ctx) return;
            if (_charts.latency) _charts.latency.destroy();
            const labels = series.map(p => new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
            _charts.latency = new Chart(ctx, {
                type: 'line',
                data: {
                    labels,
                    datasets: [
                        { label: 'Avg Duration', data: series.map(p => p.avg_duration_ms), borderColor: '#38bdf8', tension: 0.3, fill: false, pointRadius: 0 },
                        { label: 'Exec/sec', data: series.map(p => p.executions_per_sec), borderColor: '#f59e0b', tension: 0.3, fill: false, pointRadius: 0, yAxisID: 'y1' }
                    ]
                },
                options: {
                    responsive: true, maintainAspectRatio: false,
                    interaction: { mode: 'index', intersect: false },
                    plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } },
                    scales: {
                        x: { display: true, grid: { display: false }, ticks: { font: { size: 9 }, maxTicksLimit: 8 } },
                        y: { beginAtZero: true, ticks: { font: { size: 9 } } },
                        y1: { position: 'right', beginAtZero: true, grid: { display: false }, ticks: { font: { size: 9 } } }
                    }
                }
            });
        }

        function renderContributionChart(top10) {
            const ctx = document.getElementById('qaContributionChart')?.getContext('2d');
            if (!ctx) return;
            if (_charts.contrib) _charts.contrib.destroy();
            const totalCpu = top10.reduce((s, r) => s + (r.total_cpu_ms || 0), 1);
            const data = top10.map(r => ({ label: (r.query_text || '').substring(0, 40) + '...', value: (r.total_cpu_ms / totalCpu) * 100 }));
            _charts.contrib = new Chart(ctx, {
                type: 'bar',
                data: { labels: data.map(d => d.label), datasets: [{ data: data.map(d => d.value), backgroundColor: '#3b82f6', borderRadius: 4 }] },
                options: { indexAxis: 'y', responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } },
                    scales: { x: { beginAtZero: true, max: 100, ticks: { callback: v => v + '%', font: { size: 9 } } }, y: { ticks: { font: { size: 9 } } } }
                }
            });
        }

        /* ── Fetchers ── */
        async function fetchRegressions(dbName) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/mssql/query-analysis/regressions?instance=${encodeURIComponent(instName)}&database=${encodeURIComponent(dbName)}`);
                const data = await resp.json();
                _regRows = data.regressions || data || [];
            } catch (err) { console.error('regressions failed', err); }
        }

        async function fetchPlanInstability(dbName) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/mssql/query-analysis/plan-instability?instance=${encodeURIComponent(instName)}&database=${encodeURIComponent(dbName)}`);
                const data = await resp.json();
                _instabRows = data.plan_instability || data || [];
            } catch (err) { console.error('instability failed', err); }
        }

        async function fetchLiveQueries(dbName) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/live/running-queries?instance=${encodeURIComponent(instName)}&database=${encodeURIComponent(dbName)}`);
                const data = await resp.json();
                _liveRows = data.data || data.queries || (Array.isArray(data) ? data : []);
            } catch (err) { console.error('live-queries failed', err); }
        }

        /* ── initial load ── */
        await fetchActiveTab();
    }

    window.mssql_QueryAnalysisDashboard = loadQueryAnalysis;
})();
