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

    /** Matches checkbox default (checked) and API default exclude_system=true. */
    function getQaExcludeSystem() {
        const el = document.getElementById('qaIgnoreSystem');
        return el ? el.checked : true;
    }

    function populateQaDatabaseFilter() {
        const dbSelect = document.getElementById('qa-database-filter');
        if (!dbSelect) return;
        const inst = window.appState?.config?.instances?.[window.appState?.currentInstanceIdx];
        const dbs = window.appState?.databases || inst?.databases || [];
        const seen = new Set();
        dbSelect.innerHTML = '';
        ensureQaAllDatabasesOption(dbSelect);
        seen.add(QA_ALL_DATABASES);
        dbs.forEach(db => {
            const name = typeof db === 'string' ? db : db?.name;
            if (!name || seen.has(name) || isQaExcludedDatabase(name)) return;
            seen.add(name);
            const opt = document.createElement('option');
            opt.value = name;
            opt.textContent = name;
            dbSelect.appendChild(opt);
        });
        dbSelect.value = QA_ALL_DATABASES;
    }

    function mergeQaDatabaseOptionsFromRows(rows) {
        const dbSelect = document.getElementById('qa-database-filter');
        if (!dbSelect || !Array.isArray(rows)) return;
        const seen = new Set([...dbSelect.options].map(o => o.value));
        rows.forEach(r => {
            const name = r?.database_name;
            if (!name || seen.has(name) || isQaExcludedDatabase(name)) return;
            seen.add(name);
            const opt = document.createElement('option');
            opt.value = name;
            opt.textContent = name;
            dbSelect.appendChild(opt);
        });
    }

    const QA_EXCLUDED_DATABASES = new Set(['distribution']);
    const QA_ALL_DATABASES = '';

    function isQaExcludedDatabase(name) {
        return QA_EXCLUDED_DATABASES.has(String(name || '').trim().toLowerCase());
    }

    function ensureQaAllDatabasesOption(dbSelect) {
        if (!dbSelect) return;
        let allOpt = [...dbSelect.options].find(o => o.value === QA_ALL_DATABASES);
        if (!allOpt) {
            allOpt = document.createElement('option');
            allOpt.value = QA_ALL_DATABASES;
            allOpt.textContent = 'All databases';
            dbSelect.insertBefore(allOpt, dbSelect.firstChild);
        }
    }

    function getQaDatabaseFilter() {
        const dbEl = document.getElementById('qa-database-filter');
        return (dbEl?.value || '').trim();
    }

    function qaDatabaseQueryParam(dbName) {
        return dbName ? encodeURIComponent(dbName) : 'all';
    }

    async function ensureQaDatabaseSelected(instName, from, to) {
        const dbSelect = document.getElementById('qa-database-filter');
        if (!dbSelect) return '';
        ensureQaAllDatabasesOption(dbSelect);
        const excludeSystem = getQaExcludeSystem();
        const exParam = excludeSystem ? '' : '&exclude_system=false';
        try {
            const resp = await window.apiClient.authenticatedFetch(
                `/api/sqlserver/workload/databases?instance=${encodeURIComponent(instName)}&from=${from}&to=${to}${exParam}`
            );
            if (resp.ok) {
                const data = await resp.json();
                const list = data.databases || [];
                const seen = new Set([...dbSelect.options].map(o => o.value));
                list.forEach(d => {
                    const name = (d.database_name || '').trim();
                    if (!name || seen.has(name) || isQaExcludedDatabase(name)) return;
                    seen.add(name);
                    const opt = document.createElement('option');
                    opt.value = name;
                    opt.textContent = `${name} (${d.row_count || 0})`;
                    dbSelect.appendChild(opt);
                });
                const active = list.find(d => (d.row_count || 0) > 0);
                if (active?.database_name) {
                    const name = active.database_name.trim();
                    dbSelect.value = name;
                    return name;
                }
            }
        } catch (e) { console.warn('[QueryAnalysis] databases in range', e); }
        dbSelect.value = QA_ALL_DATABASES;
        return '';
    }

    function filterQaByDatabase(rows, dbName) {
        if (!Array.isArray(rows)) return [];
        let out = rows.filter(r => !isQaExcludedDatabase(r?.database_name));
        if (!dbName) return out;
        const want = String(dbName).toLowerCase();
        return out.filter(r => String(r.database_name || '').toLowerCase() === want);
    }

    function filterQaExcludedRows(rows) {
        if (!Array.isArray(rows)) return [];
        return rows.filter(r => !isQaExcludedDatabase(r?.database_name));
    }

    /* ── State ── */
    let _charts = {};
    let _topRows = [];
    let _regRows = [];
    let _instabRows = [];
    let _schedulerRows = [];
    let _watchedHashes = new Set();
    let _searchDebounce = null;
    let _pageSize = 10;
    let _currentPage = 1;
    let _trendSpanMs = 0;

    window._qaSort = window._qaSort || {
        'top-queries': { col: 'total_cpu_ms', dir: 'desc' },
        'regressions': { col: 'percent_change', dir: 'desc' },
        'instability': { col: 'plan_count', dir: 'desc' },
    };

    function destroyCharts() {
        Object.values(_charts).forEach(c => { try { c.destroy(); } catch (_) {} });
        _charts = {};
    }

    /* ── main view ──────────────────────────────────────────── */
    async function loadQueryAnalysis() {
        destroyCharts();
        _topRows = []; _regRows = []; _instabRows = []; _schedulerRows = []; _watchedHashes = new Set();
        _currentPage = 1;
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
        
        const fmtLocal = window.formatDateTimeLocalInput || ((d) => {
            const pad = (n) => String(n).padStart(2, '0');
            const x = d instanceof Date ? d : new Date(d);
            return `${x.getFullYear()}-${pad(x.getMonth() + 1)}-${pad(x.getDate())}T${pad(x.getHours())}:${pad(x.getMinutes())}:${pad(x.getSeconds())}`;
        });
        if (window.appState.fromTs && window.appState.toTs) {
            fromEl.value = window.appState.fromTs;
            toEl.value = window.appState.toTs;
        } else {
            const now = new Date();
            fromEl.value = fmtLocal(new Date(now.getTime() - 60 * 60 * 1000));
            toEl.value = fmtLocal(now);
            window.appState.fromTs = fromEl.value;
            window.appState.toTs = toEl.value;
        }

        const syncQaRangeToState = () => {
            window.appState.fromTs = fromEl.value;
            window.appState.toTs = toEl.value;
        };
        fromEl.addEventListener('change', syncQaRangeToState);
        toEl.addEventListener('change', syncQaRangeToState);
        fromEl.addEventListener('input', syncQaRangeToState);
        toEl.addEventListener('input', syncQaRangeToState);

        // Bind Actions
        window.qaRefreshAll = () => {
            syncQaRangeToState();
            return fetchActiveTab();
        };
        document.getElementById('qaApplyRange').onclick = () => window.qaRefreshAll();

        populateQaDatabaseFilter();
        const qaDbFilter = document.getElementById('qa-database-filter');
        if (qaDbFilter && !qaDbFilter._qaBound) {
            qaDbFilter._qaBound = true;
            qaDbFilter.addEventListener('change', () => {
                _currentPage = 1;
                window.qaRefreshAll();
            });
        }

        const initialRange = window.getAppTimeRangeISO
            ? window.getAppTimeRangeISO({ sync: false, fallbackHours: 1 })
            : null;
        let initFrom = initialRange?.from || (window.localDateTimeToISO ? window.localDateTimeToISO(fromEl.value) : new Date(fromEl.value).toISOString());
        let initTo = initialRange?.to || (window.localDateTimeToISO ? window.localDateTimeToISO(toEl.value) : new Date(toEl.value).toISOString());
        await ensureQaDatabaseSelected(instName, initFrom, initTo);

        const tabs = document.querySelectorAll('.qa-tab-btn');
        tabs.forEach(btn => {
            btn.onclick = () => {
                tabs.forEach(t => t.classList.remove('active'));
                btn.classList.add('active');
                document.querySelectorAll('.qa-tab-panel').forEach(p => p.style.display = 'none');
                document.getElementById(btn.dataset.tab).style.display = 'block';
                _currentPage = 1;
                fetchActiveTab();
            };
        });

        const searchInput = document.getElementById('qaSearch');
        if (searchInput) {
            if (window.appState?.wlQaSearchPrefill) {
                searchInput.value = window.appState.wlQaSearchPrefill;
                delete window.appState.wlQaSearchPrefill;
            }
            searchInput.addEventListener('input', (e) => {
                clearTimeout(_searchDebounce);
                _searchDebounce = setTimeout(() => {
                    _currentPage = 1;
                    renderActiveTab();
                }, 300);
            });
        }
        
        const ignoreSystemCheck = document.getElementById('qaIgnoreSystem');
        if (ignoreSystemCheck) {
            ignoreSystemCheck.addEventListener('change', () => {
                _currentPage = 1;
                fetchActiveTab();
            });
        }

        async function fetchActiveTab() {
            const activeTab = document.querySelector('.qa-tab-btn.active').dataset.tab;
            const range = window.getAppTimeRangeISO
                ? window.getAppTimeRangeISO({ sync: false, fallbackHours: 1 })
                : null;
            let from = range?.from || (window.localDateTimeToISO ? window.localDateTimeToISO(fromEl.value) : new Date(fromEl.value).toISOString());
            let to = range?.to || (window.localDateTimeToISO ? window.localDateTimeToISO(toEl.value) : new Date(toEl.value).toISOString());
            if (!from || !to) {
                console.warn('[QueryAnalysis] Invalid time range; using last 1 hour');
                const now = Date.now();
                from = new Date(now - 3600000).toISOString();
                to = new Date(now).toISOString();
            }
            const excludeSystem = getQaExcludeSystem();
            let dbName = getQaDatabaseFilter();

            // Always refresh summary for KPIs and trend charts
            fetchSummary(from, to, excludeSystem, dbName);
            fetchTrendCharts(from, to, dbName);

            // And fetch current watch list so we show filled stars
            await fetchWatchedHashes();

            if (activeTab === 'top-queries') {
                await fetchTopQueries(from, to, dbName);
            } else if (activeTab === 'regressions') {
                await fetchRegressions(dbName);
            } else if (activeTab === 'instability') {
                await fetchPlanInstability(dbName);
            }
            renderActiveTab();
            bindActiveTabEvents(activeTab);
        }

        async function fetchSchedulerStats() {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/cpu-scheduler-stats?instance=${encodeURIComponent(instName)}&limit=100`);
                if (!resp.ok) return;
                const data = await resp.json();
                _schedulerRows = data.cpu_scheduler_stats || [];
            } catch (e) { console.error('scheduler stats failed', e); }
        }

        async function fetchWatchedHashes() {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/watched-queries?instance=${encodeURIComponent(instName)}`, { cache: 'no-store' });
                if (resp.ok) {
                    const data = await resp.json();
                    _watchedHashes = new Set(((data && data.watched_queries) || []).map(q => q.query_hash));
                }
            } catch (e) { console.error('failed to fetch watched hashes', e); }
        }

        function renderActiveTab() {
            const activeTab = document.querySelector('.qa-tab-btn.active').dataset.tab;
            if (activeTab === 'top-queries') renderTopQueries();
            else if (activeTab === 'regressions') renderRegressions();
            else if (activeTab === 'instability') renderInstability();
        }

        async function fetchSummary(from, to, excludeSystem, dbName) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/query-analysis/summary?instance=${encodeURIComponent(instName)}&from=${from}&to=${to}&exclude_system=${excludeSystem}&database=${qaDatabaseQueryParam(dbName)}`);
                if (!resp.ok) return;
                const data = await resp.json();
                updateKpis(data);
                updateSummaryStrip(data);
            } catch (e) { console.error('summary fetch failed', e); }
        }

        function updateKpis(data) {
            const hasData = (data.total_executions || 0) > 0 || (data.total_queries_in_qs || 0) > 0 || (_topRows && _topRows.length > 0);
            const warning = document.getElementById('qaDataWarning');
            if (warning) {
                if (!hasData) {
                    const dbFilter = getQaDatabaseFilter();
                    let diag = data.message
                        || (data.latest_history_capture
                            ? `No activity in range. Latest collector data: ${new Date(data.latest_history_capture).toLocaleString()} (${data.history_rows_in_range || 0} history rows).`
                            : 'No query metrics in TimescaleDB for this range. Ensure the SQL Server collector is running.');
                    if (dbFilter) {
                        diag += ` Database "${dbFilter}" has no collector rows in this range.`;
                    }
                    warning.innerHTML = `<i class="fa-solid fa-circle-exclamation"></i> ${esc(diag)}`;
                    warning.style.display = 'inline';
                } else {
                    warning.style.display = 'none';
                }
            }

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
                const excludeSystem = getQaExcludeSystem();
                const exParam = `&exclude_system=${excludeSystem}`;
                const dbParam = `&database=${qaDatabaseQueryParam(dbName)}`;
                const base = `/api/timescale/sqlserver/query-stats-timeseries?instance=${encodeURIComponent(instName)}&from=${from}&to=${to}`;

                const cpuResp = await window.apiClient.authenticatedFetch(`${base}&metric=cpu${dbParam}${exParam}`);
                if (cpuResp.ok) {
                    const cpuData = await cpuResp.json();
                    renderWorkloadChart(cpuData.series || []);
                }

                const durResp = await window.apiClient.authenticatedFetch(`${base}&metric=duration${dbParam}${exParam}`);
                if (durResp.ok) {
                    const durData = await durResp.json();
                    renderLatencyChart(durData.series || []);
                }

            } catch (e) { console.error('trend charts failed', e); }
        }

        function top10RowsForTrend(rows) {
            return [...(rows || [])]
                .sort((a, b) => (b.total_cpu_ms || 0) - (a.total_cpu_ms || 0))
                .slice(0, 10);
        }

        async function refreshTopQueryTrendChart(from, to, dbName, excludeSystem) {
            const top = top10RowsForTrend(_topRows);
            const fingerprints = top.map(r => r.statement_fingerprint).filter(Boolean).join(',');
            if (!fingerprints) {
                renderContributionTrendChart({ series: [] });
                return;
            }
            try {
                const resp = await window.apiClient.authenticatedFetch(
                    `/api/sqlserver/query-analysis/top-queries-trends?instance=${encodeURIComponent(instName)}&from=${from}&to=${to}&exclude_system=${excludeSystem}&limit=10&database=${qaDatabaseQueryParam(dbName)}&fingerprints=${encodeURIComponent(fingerprints)}`
                );
                if (!resp.ok) return;
                const data = await resp.json();
                const byFp = new Map((data.series || []).map(s => [s.statement_fingerprint, s]));
                data.series = top.map(r => byFp.get(r.statement_fingerprint)).filter(Boolean);
                renderContributionTrendChart(data);
            } catch (e) {
                console.warn('[QueryAnalysis] top query trends', e);
            }
        }

        async function fetchTopQueries(from, to, dbName) {
            const body = document.getElementById('qaTopQueriesBody');
            const excludeSystem = getQaExcludeSystem();
            body.innerHTML = '<div class="text-muted p-3"><i class="fa-solid fa-spinner fa-spin"></i> Loading top queries...</div>';
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/query-analysis/top-queries?instance=${encodeURIComponent(instName)}&from=${from}&to=${to}&exclude_system=${excludeSystem}&limit=100&database=${qaDatabaseQueryParam(dbName)}`);
                if (!resp.ok) throw new Error('HTTP ' + resp.status);
                const data = await resp.json();
                const raw = Array.isArray(data) ? data : (data && data.queries) || [];
                const deduped = typeof window.dedupeSqlServerTopQueries === 'function'
                    ? window.dedupeSqlServerTopQueries(raw)
                    : raw;
                _topRows = filterQaExcludedRows(deduped);
                mergeQaDatabaseOptionsFromRows(_topRows);

                // Hide warning if we have data
                const warning = document.getElementById('qaDataWarning');
                if (warning && _topRows.length > 0) warning.style.display = 'none';

                const range = window.getAppTimeRangeISO
                    ? window.getAppTimeRangeISO({ sync: false, fallbackHours: 1 })
                    : null;
                let fromR = range?.from || from;
                let toR = range?.to || to;
                await refreshTopQueryTrendChart(fromR, toR, dbName, excludeSystem);
            } catch (e) {
                body.innerHTML = `<div class="text-danger p-3">Failed to load top queries: ${esc(e.message)}</div>`;
            }
        }

        function renderTopQueries() {
            const container = document.getElementById('qaTopQueriesBody');
            if (!_topRows.length) {
                container.innerHTML = `<div class="text-muted p-4 text-center">No queries found for this period.</div>`;
                return;
            }
            const q = document.getElementById('qaSearch')?.value.toLowerCase() || '';
            const filtered = _topRows.filter(r =>
                (r.query_text || '').toLowerCase().includes(q) || (r.query_hash || '').toLowerCase().includes(q)
            );
            container.innerHTML = renderTopQueriesTable(filtered);
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
                } else if (action === 'change-page') {
                    _currentPage = parseInt(target.dataset.page);
                    renderActiveTab();
                }
            });
            container._bound = true;
        }

        async function watchQuery(btn) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/watched-queries?instance=${encodeURIComponent(instName)}`, {
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
            
            // Pagination logic
            const total = sorted.length;
            const totalPages = Math.ceil(total / _pageSize);
            if (_currentPage > totalPages) _currentPage = Math.max(1, totalPages);
            const start = (_currentPage - 1) * _pageSize;
            const paged = sorted.slice(start, start + _pageSize);

            let paginationHtml = '';
            if (totalPages > 1) {
                paginationHtml = `
                    <div class="flex-between mt-3 px-2">
                        <div class="small text-muted">Showing ${start + 1} to ${Math.min(start + _pageSize, total)} of ${total}</div>
                        <div class="flex-center gap-2">
                            <button class="btn btn-xs btn-outline" ${_currentPage === 1 ? 'disabled' : ''} data-action="change-page" data-page="${_currentPage - 1}">Prev</button>
                            <span class="small">Page ${_currentPage} of ${totalPages}</span>
                            <button class="btn btn-xs btn-outline" ${_currentPage === totalPages ? 'disabled' : ''} data-action="change-page" data-page="${_currentPage + 1}">Next</button>
                        </div>
                    </div>`;
            }

            return `<div class="table-responsive"><table class="qa-table">
                <thead><tr>
                    <th style="width:2.5rem; text-align:center;"></th>
                    <th>Query</th>
                    ${sortTh('top-queries', 'executions', 'Executions', 'text-right')}
                    ${sortTh('top-queries', 'avg_cpu_ms', 'Avg CPU', 'text-right')}
                    ${sortTh('top-queries', 'avg_duration_ms', 'Avg Duration', 'text-right')}
                    ${sortTh('top-queries', 'avg_reads', 'Avg Reads', 'text-right')}
                    ${sortTh('top-queries', 'total_cpu_ms', 'Total CPU', 'text-right')}
                    ${sortTh('top-queries', 'last_execution_time', 'Last Execution', 'text-right')}
                    <th>Database</th>
                </tr></thead>
                <tbody>${paged.map((r, i) => {
                    const cacheKey = 'qa_top_' + i;
                    window.appState.queryCache[cacheKey] = { text: r.query_text, query_hash: r.query_hash, database_name: r.database_name };
                    const isWatched = _watchedHashes.has(r.query_hash);
                    const lastExec = r.last_execution_time && r.last_execution_time !== '0001-01-01T00:00:00Z' 
                                    ? new Date(r.last_execution_time).toLocaleString() 
                                    : '--';
                    const variantBadge = (r.hash_variant_count > 1)
                        ? `<span class="badge badge-muted ml-1" title="${r.hash_variant_count} query hashes rolled into this statement">${r.hash_variant_count} hashes</span>`
                        : '';
                    return `<tr>
                    <td><button class="qa-watch-btn ${isWatched ? 'watched' : ''}" data-action="watch-query" data-hash="${esc(r.query_hash)}" data-db="${esc(r.database_name)}" data-label="${esc((r.query_text || '').substring(0, 80))}" data-query-text="${esc(r.query_text || '')}" ${isWatched ? 'disabled' : ''}>
                        <i class="fa-${isWatched ? 'solid' : 'regular'} fa-star"></i>
                    </button></td>
                    <td style="max-width:300px; cursor:pointer;" data-action="show-query-modal-direct" data-key="${cacheKey}" data-fn="showQueryStoreQueryModal">
                        <span class="qa-query-text" title="Click to view full SQL">${esc(r.query_text)}</span>${variantBadge}
                    </td>
                    <td class="text-right font-mono">${fmtK(r.executions)}</td>
                    <td class="text-right font-mono">${fmtMs(r.avg_cpu_ms)}</td>
                    <td class="text-right font-mono">${fmtMs(r.avg_duration_ms)}</td>
                    <td class="text-right font-mono">${fmtNum(r.avg_reads)}</td>
                    <td class="text-right font-mono" style="color:var(--accent); font-weight:700;">${fmtMs(r.total_cpu_ms)}</td>
                    <td class="text-right text-muted" style="font-size:0.65rem;">${lastExec}</td>
                    <td class="text-muted" style="font-size:0.7rem;">${esc(r.database_name)}</td>
                </tr>`}).join('')}</tbody></table></div>${paginationHtml}`;
        }

        function renderRegressionsTable(rows) {
            const sorted = applySort(rows, 'regressions');
            return `<div class="table-responsive"><table class="qa-table">
                <thead><tr>
                    <th>Query</th>
                    <th>Database</th>
                    ${sortTh('regressions', 'regression_type', 'Type', '')}
                    ${sortTh('regressions', 'previous_avg', 'Prev Avg', 'text-right')}
                    ${sortTh('regressions', 'current_avg', 'Curr Avg', 'text-right')}
                    ${sortTh('regressions', 'percent_change', '% Change', 'text-right')}
                    <th class="text-center">Plan Changed</th>
                    <th>Login</th>
                    <th>App</th>
                    <th></th>
                </tr></thead>
                <tbody>${sorted.map((r, i) => {
                    const cacheKey = 'qa_reg_' + i;
                    window.appState.queryCache[cacheKey] = { text: r.query_text, query_hash: r.query_hash, database_name: r.database_name };
                    const isWatched = _watchedHashes.has(r.query_hash);
                    return `<tr>
                    <td style="max-width:280px; cursor:pointer;" data-action="show-query-modal-direct" data-key="${cacheKey}" data-fn="showQueryStoreQueryModal">
                        <span class="qa-query-text" title="Click to view full SQL">${esc(r.query_text)}</span>
                    </td>
                    <td class="text-muted" style="font-size:0.7rem;">${esc(r.database_name)}</td>
                    <td><span class="qa-badge qa-badge--default">${esc(r.regression_type)}</span></td>
                    <td class="text-right font-mono">${fmtMs(r.previous_avg)}</td>
                    <td class="text-right font-mono" style="font-weight:700;">${fmtMs(r.current_avg)}</td>
                    <td class="text-right"><i class="fa-solid fa-arrow-up text-danger"></i> ${fmtPct(r.percent_change)}</td>
                    <td class="text-center">${r.plan_changed ? '<i class="fa-solid fa-circle-check text-success"></i>' : '—'}</td>
                    <td class="text-muted" style="font-size:0.7rem;" title="${esc(r.login_name)}">${esc(r.login_name || '—')}</td>
                    <td class="text-muted" style="font-size:0.7rem;" title="${esc(r.application_name)}">${esc(r.application_name || '—')}</td>
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

        /* ── Trend Charts ── */
        function renderWorkloadChart(series) {
            const canvas = document.getElementById('qaWorkloadChart');
            if (!canvas) return;
            if (window.destroyChartOnCanvas) window.destroyChartOnCanvas(canvas);
            else if (_charts.workload) { try { _charts.workload.destroy(); } catch (_) {} }
            const labels = (series || []).map(p => new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
            const workloadCfg = {
                type: 'line',
                data: {
                    labels,
                    datasets: [
                        { label: 'CPU (ms)', data: (series || []).map(p => p.cpu_ms), borderColor: '#f43f5e', tension: 0.3, fill: false, pointRadius: 0 },
                        { label: 'Reads/sec', data: (series || []).map(p => p.logical_reads), borderColor: '#10b981', tension: 0.3, fill: false, pointRadius: 0, yAxisID: 'y1' },
                    ],
                },
                options: {
                    responsive: true, maintainAspectRatio: false,
                    interaction: { mode: 'index', intersect: false },
                    plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } },
                    scales: {
                        x: { display: true, grid: { display: false }, ticks: { font: { size: 9 }, maxTicksLimit: 8 } },
                        y: { beginAtZero: true, ticks: { font: { size: 9 } } },
                        y1: { position: 'right', beginAtZero: true, grid: { display: false }, ticks: { font: { size: 9 } } },
                    },
                },
            };
            _charts.workload = (typeof window.createOptimaChart === 'function')
                ? window.createOptimaChart(canvas, workloadCfg)
                : new Chart(canvas, workloadCfg);
        }

        function renderLatencyChart(series) {
            const canvas = document.getElementById('qaLatencyChart');
            if (!canvas) return;
            if (window.destroyChartOnCanvas) window.destroyChartOnCanvas(canvas);
            else if (_charts.latency) { try { _charts.latency.destroy(); } catch (_) {} }
            const labels = (series || []).map(p => new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
            const latencyCfg = {
                type: 'line',
                data: {
                    labels,
                    datasets: [
                        { label: 'Avg Duration', data: (series || []).map(p => p.avg_duration_ms), borderColor: '#38bdf8', tension: 0.3, fill: false, pointRadius: 0 },
                        { label: 'Executions/bucket', data: (series || []).map(p => p.executions_per_sec), borderColor: '#f59e0b', tension: 0.3, fill: false, pointRadius: 0, yAxisID: 'y1' },
                    ],
                },
                options: {
                    responsive: true, maintainAspectRatio: false,
                    interaction: { mode: 'index', intersect: false },
                    plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } },
                    scales: {
                        x: { display: true, grid: { display: false }, ticks: { font: { size: 9 }, maxTicksLimit: 8 } },
                        y: { beginAtZero: true, ticks: { font: { size: 9 } } },
                        y1: { position: 'right', beginAtZero: true, grid: { display: false }, ticks: { font: { size: 9 } } },
                    },
                },
            };
            _charts.latency = (typeof window.createOptimaChart === 'function')
                ? window.createOptimaChart(canvas, latencyCfg)
                : new Chart(canvas, latencyCfg);
        }

        const QA_TREND_COLORS = ['#3b82f6', '#f59e0b', '#10b981', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899', '#84cc16', '#f97316', '#6366f1'];

        function truncateTrendLabel(text, maxLen = 36) {
            const raw = (text || '').trim();
            if (!raw) return '—';
            return raw.length > maxLen ? raw.substring(0, maxLen) + '…' : raw;
        }

        function formatTrendBucketLabel(ts) {
            const d = new Date(ts);
            if (isNaN(d.getTime())) return '';
            const spanH = (_trendSpanMs || 0) / 3600000;
            if (spanH > 24) {
                return d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
            }
            return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        }

        function renderContributionTrendChart(payload) {
            const canvas = document.getElementById('qaContributionChart');
            if (!canvas) return;
            if (window.destroyChartOnCanvas) window.destroyChartOnCanvas(canvas);
            else if (_charts.contrib) { try { _charts.contrib.destroy(); } catch (_) {} }

            const series = (payload && payload.series) || [];
            if (!series.length) {
                if (window.setChartOverlayState) {
                    window.setChartOverlayState('qaContributionChart', 'empty', 'No execution trend data in range');
                }
                return;
            }
            if (window.setChartOverlayState) window.setChartOverlayState('qaContributionChart', 'clear');

            const bucketKeys = new Set();
            series.forEach(s => (s.points || []).forEach(p => {
                if (p.timestamp) bucketKeys.add(new Date(p.timestamp).toISOString());
            }));
            const sortedKeys = [...bucketKeys].sort();
            _trendSpanMs = sortedKeys.length >= 2
                ? new Date(sortedKeys[sortedKeys.length - 1]) - new Date(sortedKeys[0])
                : 0;
            const labels = sortedKeys.map(formatTrendBucketLabel);
            const labelIndex = new Map(sortedKeys.map((k, i) => [k, i]));

            const datasets = series.map((s, idx) => {
                const data = new Array(labels.length).fill(0);
                const cpuMs = new Array(labels.length).fill(0);
                (s.points || []).forEach(p => {
                    const key = new Date(p.timestamp).toISOString();
                    const i = labelIndex.get(key);
                    if (i == null) return;
                    data[i] = Number(p.executions) || 0;
                    cpuMs[i] = Number(p.cpu_ms) || 0;
                });
                return {
                    label: truncateTrendLabel(s.label || s.query_text),
                    data,
                    cpuMs,
                    borderColor: QA_TREND_COLORS[idx % QA_TREND_COLORS.length],
                    backgroundColor: QA_TREND_COLORS[idx % QA_TREND_COLORS.length] + '33',
                    tension: 0.25,
                    fill: false,
                    pointRadius: 0,
                    borderWidth: 1.5,
                };
            });

            const trendCfg = {
                type: 'line',
                data: { labels, datasets },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    interaction: { mode: 'index', intersect: false },
                    plugins: {
                        legend: { display: true, position: 'bottom', labels: { boxWidth: 8, font: { size: 8 }, maxWidth: 120 } },
                        tooltip: {
                            callbacks: {
                                label: (ctx) => {
                                    const bucketExec = ctx.parsed?.y ?? 0;
                                    const bucketCpu = ctx.dataset?.cpuMs?.[ctx.dataIndex] ?? 0;
                                    const ser = series[ctx.datasetIndex];
                                    const windowExec = ser?.total_executions ?? 0;
                                    let line = `${ctx.dataset.label}: ${Number(bucketExec).toLocaleString()} exec (bucket)`;
                                    if (bucketCpu > 0) line += `, ${Number(bucketCpu).toLocaleString()} ms CPU`;
                                    if (windowExec > 0) line += ` — ${Number(windowExec).toLocaleString()} total in range`;
                                    return line;
                                },
                            },
                        },
                    },
                    scales: {
                        x: { display: true, grid: { display: false }, ticks: { font: { size: 8 }, maxTicksLimit: 8 } },
                        y: { beginAtZero: true, ticks: { font: { size: 8 } }, title: { display: true, text: 'Executions / bucket', font: { size: 9 } } },
                    },
                },
            };

            _charts.contrib = (typeof window.createOptimaChart === 'function')
                ? window.createOptimaChart(canvas, trendCfg)
                : new Chart(canvas.getContext('2d'), trendCfg);
        }

        /* ── Fetchers ── */
        async function fetchRegressions(dbName) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/query-analysis/regressions?instance=${encodeURIComponent(instName)}&database=${qaDatabaseQueryParam(dbName)}`);
                if (!resp.ok) { console.error('regressions HTTP', resp.status); return; }
                const data = await resp.json();
                const raw = data.regressions || data || [];
                _regRows = filterQaByDatabase(raw, dbName);
                mergeQaDatabaseOptionsFromRows(raw);
            } catch (err) { console.error('regressions failed', err); }
        }

        async function fetchPlanInstability(dbName) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/query-analysis/plan-instability?instance=${encodeURIComponent(instName)}&database=${qaDatabaseQueryParam(dbName)}`);
                if (!resp.ok) { console.error('instability HTTP', resp.status); return; }
                const data = await resp.json();
                const raw = data.plan_instability || data || [];
                _instabRows = filterQaByDatabase(raw, dbName);
                mergeQaDatabaseOptionsFromRows(raw);
            } catch (err) { console.error('instability failed', err); }
        }

        /* ── initial load ── */
        await fetchActiveTab();
    }

    window.sqlserver_QueryAnalysisDashboard = loadQueryAnalysis;
})();
