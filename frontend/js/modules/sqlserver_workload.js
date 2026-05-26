// SQL Optima — UI logic for the SQL Server Workload Observability Dashboard.
import { api } from '../api/api.js';
import { formatters } from '../utils/formatters.js';
import { charts } from '../utils/charts.js';

/** Same exclusions as Query Analysis (distribution replica noise). */
const WL_EXCLUDED_DATABASES = new Set(['distribution']);
/** Empty value = all databases (must match API database=all). */
const WL_ALL_DATABASES = '';

function ensureWlAllDatabasesOption(dbSelect) {
    if (!dbSelect) return;
    let allOpt = [...dbSelect.options].find(o => o.value === WL_ALL_DATABASES);
    if (!allOpt) {
        allOpt = document.createElement('option');
        allOpt.value = WL_ALL_DATABASES;
        allOpt.textContent = 'All databases';
        dbSelect.insertBefore(allOpt, dbSelect.firstChild);
    }
}

function isWlExcludedDatabase(name) {
    return WL_EXCLUDED_DATABASES.has(String(name || '').trim().toLowerCase());
}

/** Normalize API bucket timestamps for consistent chart lookup keys. */
function bucketKey(bucket) {
    const d = bucket instanceof Date ? bucket : new Date(bucket);
    return Number.isNaN(d.getTime()) ? String(bucket) : d.toISOString();
}

export const sqlserverWorkload = {
    charts: {},
    instance: '',
    refreshInterval: null,
    _sortBound: false,
    _tabClickBound: false,
    loadedTabs: new Set(['queries-content']),
    offenders: [],
    topQueriesPage: 0,
    topQueriesPageSize: 10,
    topQueriesFetchLimit: 100,
    sortKey: 'total_cpu_ms',
    sortDir: 'desc',
    sectionErrors: {},
    _refreshInFlight: false,
    _pendingRefresh: false,
    _lastFrom: null,
    _lastTo: null,
    databaseFilter: '',

    destroy() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
        Object.keys(this.charts).forEach(id => {
            try { this.charts[id]?.destroy(); } catch (_) { /* ignore */ }
            delete this.charts[id];
        });
        this.loadedTabs = new Set(['queries-content']);
        this._sortBound = false;
        this._tabClickBound = false;
        this._qaLinkBound = false;
        this._topQueriesPageBound = false;
    },

    async init() {
        this.instance = window.appState?.config?.instances?.[window.appState?.currentInstanceIdx]?.name || '';
        const nameEl = document.getElementById('wlInstanceName');
        if (nameEl) nameEl.textContent = this.instance || '--';

        if (!this.instance) {
            this.setBanner('Select a SQL Server instance to view workload data.', 'warning');
            return;
        }

        if (window.initPageTimePicker) window.initPageTimePicker();
        this.bindTimeRangeRefresh();
        this.bindFilters();
        this.bindTabs();
        this.bindSortHandlers();

        const range = window.getAppTimeRangeISO ? window.getAppTimeRangeISO() : {};
        const initFrom = range.from || new Date(Date.now() - 3600000).toISOString();
        const initTo = range.to || new Date().toISOString();
        await this.ensureDatabaseSelected(initFrom, initTo);

        const dynInterval = window.collectorConfig
            ? window.collectorConfig.getInterval('sqlserver_query_snapshot', 30000)
            : 30000;
        if (this.refreshInterval) clearInterval(this.refreshInterval);
        this.refreshInterval = setInterval(() => this.refreshAll(), dynInterval);

        await this.refreshAll();
    },

    getFilters() {
        const dbEl = document.getElementById('wl-database-filter');
        const database = (dbEl?.value || this.databaseFilter || '').trim();
        this.databaseFilter = database;
        return { database };
    },

    /** Query Analysis default: exclude_system=true (user workload + monitoring login exclusion on API). */
    filterQueryString() {
        const { database } = this.getFilters();
        let q = '&exclude_system=true';
        q += database
            ? `&database=${encodeURIComponent(database)}`
            : '&database=all';
        return q;
    },

    workloadDatabasesUrl(from, to) {
        return `/api/sqlserver/workload/databases?instance=${encodeURIComponent(this.instance)}` +
            `&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&exclude_system=true`;
    },

    applyDatabaseListToSelect(dbSelect, list) {
        ensureWlAllDatabasesOption(dbSelect);
        const counts = new Map();
        list.forEach(d => {
            const name = (d.database_name || '').trim();
            if (name) counts.set(name, d.row_count || 0);
        });
        const seen = new Set([WL_ALL_DATABASES]);
        list.forEach(d => {
            const name = (d.database_name || '').trim();
            if (!name || seen.has(name)) return;
            seen.add(name);
            let opt = [...dbSelect.options].find(o => o.value === name);
            if (!opt) {
                opt = document.createElement('option');
                opt.value = name;
                dbSelect.appendChild(opt);
            }
            opt.textContent = `${name} (${counts.get(name) || 0})`;
        });
        [...dbSelect.options].forEach(opt => {
            if (!opt.value) return;
            if (counts.has(opt.value)) {
                opt.textContent = `${opt.value} (${counts.get(opt.value)})`;
            }
        });
    },

    pickDatabaseForRange(list, preferName) {
        const trimmed = (preferName || '').trim();
        if (trimmed) {
            const current = list.find(d => (d.database_name || '').trim() === trimmed && (d.row_count || 0) > 0);
            if (current?.database_name) return current.database_name.trim();
        }
        const active = list.find(d => (d.row_count || 0) > 0);
        if (active?.database_name) return active.database_name.trim();
        const first = list[0]?.database_name;
        return first ? first.trim() : '';
    },

    /** Refresh database dropdown options; keep the user's selection when possible. */
    async syncDatabaseForRange(from, to, { pickIfInvalid = false } = {}) {
        const dbSelect = document.getElementById('wl-database-filter');
        if (!dbSelect) return '';
        const previous = (dbSelect.value ?? this.databaseFilter ?? '').trim();
        try {
            const data = await api.get(this.workloadDatabasesUrl(from, to)) || {};
            const list = (data.databases || []).filter(d => !isWlExcludedDatabase(d.database_name));
            this.applyDatabaseListToSelect(dbSelect, list);

            if (!previous) {
                dbSelect.value = WL_ALL_DATABASES;
                this.databaseFilter = '';
                return '';
            }

            const hasOption = [...dbSelect.options].some(o => o.value === previous);
            if (hasOption) {
                dbSelect.value = previous;
                this.databaseFilter = previous;
                return previous;
            }

            if (pickIfInvalid && list.length > 0) {
                const name = this.pickDatabaseForRange(list, '');
                if (name) {
                    dbSelect.value = name;
                    this.databaseFilter = name;
                    return name;
                }
            }

            dbSelect.value = WL_ALL_DATABASES;
            this.databaseFilter = '';
            return '';
        } catch (e) {
            console.warn('[Workload] databases in range', e);
        }
        dbSelect.value = WL_ALL_DATABASES;
        this.databaseFilter = '';
        return '';
    },

    async ensureDatabaseSelected(from, to) {
        // Default to all databases; user can narrow to one DB from the dropdown.
        const dbSelect = document.getElementById('wl-database-filter');
        if (dbSelect) {
            ensureWlAllDatabasesOption(dbSelect);
            dbSelect.value = WL_ALL_DATABASES;
            this.databaseFilter = '';
        }
        await this.syncDatabaseForRange(from, to, { pickIfInvalid: false });
        return this.getFilters().database;
    },

    bindTimeRangeRefresh() {
        const fromInput = document.getElementById('from-ts');
        const toInput = document.getElementById('to-ts');
        const onRangeApply = () => { void this.refreshAll(); };
        [fromInput, toInput].forEach(el => {
            if (!el || el._wlRangeBound) return;
            el._wlRangeBound = true;
            el.addEventListener('change', onRangeApply);
        });
    },

    bindFilters() {
        const dbSelect = document.getElementById('wl-database-filter');
        if (dbSelect) {
            ensureWlAllDatabasesOption(dbSelect);
            if (dbSelect.options.length <= 1) {
                const dbs = window.appState?.databases || window.appState?.config?.instances?.[window.appState?.currentInstanceIdx]?.databases || [];
                const seen = new Set([WL_ALL_DATABASES]);
                dbs.forEach(db => {
                    const name = typeof db === 'string' ? db : db?.name;
                    if (!name || seen.has(name) || isWlExcludedDatabase(name)) return;
                    seen.add(name);
                    const opt = document.createElement('option');
                    opt.value = name;
                    opt.textContent = name;
                    dbSelect.appendChild(opt);
                });
            }
            if (!dbSelect.value) dbSelect.value = WL_ALL_DATABASES;
        }
        const applyBtn = document.getElementById('wl-apply-filters');
        if (applyBtn && !applyBtn._wlBound) {
            applyBtn._wlBound = true;
            applyBtn.addEventListener('click', () => this.refreshAll());
        }
        if (dbSelect && !dbSelect._wlBound) {
            dbSelect._wlBound = true;
            dbSelect.addEventListener('change', () => {
                this.databaseFilter = dbSelect.value;
                this.refreshAll();
            });
        }
    },

    bindTabs() {
        if (this._tabClickBound) return;
        const tabs = document.querySelectorAll('.workload-page-modern .qa-tab-btn');
        tabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                e.preventDefault();
                tabs.forEach(t => t.classList.remove('active'));
                document.querySelectorAll('#workloadTabsContent .tab-pane').forEach(p => {
                    p.classList.remove('show', 'active');
                });
                tab.classList.add('active');
                const targetId = tab.getAttribute('data-tab');
                const targetPane = document.getElementById(targetId);
                if (targetPane) {
                    targetPane.classList.add('show', 'active');
                    const canvas = targetPane.querySelector('canvas');
                    if (canvas && this.charts[canvas.id]) this.charts[canvas.id].resize();
                }
                if (targetId) {
                    this.loadedTabs.add(targetId);
                    if (this._lastFrom && this._lastTo) {
                        if (targetId === 'app-cpu-content') this.loadAppAnalysis(this._lastFrom, this._lastTo);
                        if (targetId === 'login-cpu-content') this.loadUserAnalysis(this._lastFrom, this._lastTo);
                        if (targetId === 'scheduler-history-content') this.loadSchedulerStats();
                    }
                }
            });
        });
        this._tabClickBound = true;
    },

    bindSortHandlers() {
        if (this._sortBound) return;
        const table = document.getElementById('wlTopQueriesTable');
        if (!table) return;
        table.querySelectorAll('th.sortable[data-sort]').forEach(th => {
            th.style.cursor = 'pointer';
            th.addEventListener('click', () => {
                const key = th.getAttribute('data-sort');
                if (this.sortKey === key) {
                    this.sortDir = this.sortDir === 'desc' ? 'asc' : 'desc';
                } else {
                    this.sortKey = key;
                    this.sortDir = 'desc';
                }
                this.topQueriesPage = 0;
                this.renderTopQueriesTable();
            });
        });
        this._sortBound = true;
    },

    setBanner(message, type = 'danger') {
        const el = document.getElementById('wl-error-banner');
        if (!el) return;
        if (!message) {
            el.style.display = 'none';
            el.textContent = '';
            return;
        }
        el.className = `alert alert-${type} wl-error-banner`;
        el.style.display = 'block';
        el.textContent = message;
    },

    setSectionError(section, message) {
        if (message) this.sectionErrors[section] = message;
        else delete this.sectionErrors[section];
        const parts = Object.values(this.sectionErrors);
        this.setBanner(parts.length ? parts.join(' · ') : null);
    },

    async refreshAll() {
        if (!this.instance) return;
        if (this._refreshInFlight) {
            this._pendingRefresh = true;
            return;
        }
        this._refreshInFlight = true;
        const range = window.getAppTimeRangeISO ? window.getAppTimeRangeISO() : {};
        const from = range.from || new Date(Date.now() - 3600000).toISOString();
        const to = range.to || new Date().toISOString();
        const rangeChanged = this._lastFrom !== from || this._lastTo !== to;
        this._lastFrom = from;
        this._lastTo = to;
        this.sectionErrors = {};

        try {
            await this.syncDatabaseForRange(from, to, { pickIfInvalid: rangeChanged });

            this.setLoading(true);
            const tasks = [
                this.loadSection('summary', () => this.loadSummary(from, to)),
                this.loadSection('trends', () => this.loadTrends(from, to)),
                this.loadSection('top-queries', () => this.loadTopOffenders(from, to)),
                this.loadSection('server-properties', () => this.loadServerProperties()),
                this.loadSection('scheduler', () => this.loadSchedulerStats()),
            ];
            if (this.loadedTabs.has('app-cpu-content')) {
                tasks.push(this.loadSection('app-analysis', () => this.loadAppAnalysis(from, to)));
            }
            if (this.loadedTabs.has('login-cpu-content')) {
                tasks.push(this.loadSection('user-analysis', () => this.loadUserAnalysis(from, to)));
            }
            await Promise.all(tasks);
            const updateEl = document.getElementById('wl-last-update');
            if (updateEl) updateEl.textContent = new Date().toLocaleTimeString();
        } catch (err) {
            console.error('[Workload] Refresh failed:', err);
            this.setBanner('Workload refresh failed. Check console for details.');
        } finally {
            this.setLoading(false);
            this._refreshInFlight = false;
            if (this._pendingRefresh) {
                this._pendingRefresh = false;
                void this.refreshAll();
            }
        }
        // QA summary uses heavy history + lateral joins; do not block the workload overlay.
        void this.loadSection('qa-summary', () => this.loadQaSummary(from, to));
    },

    async loadSection(section, fn) {
        try {
            await fn();
            this.setSectionError(section, null);
        } catch (e) {
            console.warn(`[Workload] ${section} failed:`, e);
            this.setSectionError(section, `${section}: unavailable`);
        }
    },

    setLoading(isLoading) {
        const overlay = document.getElementById('wlLoadingOverlay');
        if (overlay) overlay.style.display = isLoading ? 'flex' : 'none';
    },

    async loadSummary(from, to) {
        const data = await api.get(this.workloadUrl('summary', from, to)) || {};
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('wl-kpi-cpu', formatters.number((data.total_cpu_ms || 0) / 1000, 1));
        set('wl-kpi-execs', formatters.compactNumber(data.total_executions || 0));
        set('wl-kpi-reads', formatters.compactNumber(data.total_logical_reads || 0));
        set('wl-kpi-mem', formatters.bytes((data.max_memory_grant_kb || 0) * 1024));
        this.showDataCoverageHint(data, from, to);
    },

    showDataCoverageHint(summary, from, to) {
        const d = summary.diagnostics || {};
        const hasKpis = (summary.total_cpu_ms || 0) > 0 || (summary.total_executions || 0) > 0;
        const hasRows = (d.history_rows_in_range || 0) > 0 || (d.metrics_rows_in_range || 0) > 0;
        if (hasKpis || hasRows) {
            this.setBanner(null);
            return;
        }
        const { database } = this.getFilters();
        const parts = [];
        const fmt = (ts) => ts ? new Date(ts).toLocaleString() : 'never';
        parts.push(`No user workload rows in selected range (${new Date(from).toLocaleString()} – ${new Date(to).toLocaleString()}). System and monitoring SQL are excluded (same as Query Analysis).`);
        if ((d.metrics_rows_unfiltered || 0) > 0 && (d.metrics_rows_in_range || 0) === 0) {
            parts.push(`TimescaleDB has ${d.metrics_rows_unfiltered} raw row(s) in range but 0 after user-workload filters.`);
            const dbList = (d.databases_in_range || []).filter(x => (x.row_count || 0) > 0);
            if (dbList.length > 0) {
                parts.push(`Databases with user workload: ${dbList.map(x => `${x.database_name} (${x.row_count})`).join(', ')}.`);
            }
        }
        if (d.latest_history_capture) {
            parts.push(`Latest TimescaleDB snapshot: ${fmt(d.latest_history_capture)}.`);
        } else {
            parts.push('No data in sqlserver_query_stats_history — confirm sqlserver_query_snapshot collector is running.');
        }
        if (d.collector_last_poll) {
            parts.push(`Collector last poll: ${fmt(d.collector_last_poll)}.`);
        }
        if (database) {
            parts.push(`No user workload for database "${database}" in this range — pick another database from the dropdown.`);
        }
        this.setBanner(parts.join(' '), 'warning');
    },

    async loadServerProperties() {
        const data = await api.get(`/api/sqlserver/server-properties?instance=${encodeURIComponent(this.instance)}`) || {};
        const p = data.server_properties || {};
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('wl-prop-cpu', p.cpu_count || '--');
        set('wl-prop-mem', p.physical_memory_gb ? p.physical_memory_gb.toFixed(0) : '--');
    },

    async loadSchedulerStats() {
        const data = await api.get(`/api/sqlserver/cpu-scheduler-stats?instance=${encodeURIComponent(this.instance)}&limit=50`) || {};
        const all = data.cpu_scheduler_stats || [];
        if (all.length === 0) {
            const body = document.getElementById('wlSchedulerStatsBody');
            if (body) body.innerHTML = '<tr><td colspan="5" class="text-center text-muted">No scheduler samples</td></tr>';
            return;
        }
        const s = all[0];
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('wl-sched-workers', `${s.total_current_workers_count || 0}/${s.max_workers_count || 0}`);
        set('wl-sched-queue', s.total_work_queue_count || 0);

        this.renderSchedulerHistory(all);
        const body = document.getElementById('wlSchedulerStatsBody');
        if (body) {
            body.innerHTML = all.slice(0, 20).map(x => {
                let status = x.system_memory_state_desc || 'Healthy';
                if (status === 'Available physical memory is high') status = 'Healthy';
                if (status === 'Physical memory usage is steady') status = 'Stable';
                return `
                    <tr>
                        <td>${new Date(x.capture_timestamp).toLocaleTimeString()}</td>
                        <td>${x.total_current_workers_count}</td>
                        <td>${x.total_runnable_tasks_count}</td>
                        <td>${x.total_work_queue_count}</td>
                        <td><span class="badge badge-${x.physical_memory_pressure_warning ? 'danger' : 'success'}">${status}</span></td>
                    </tr>
                `;
            }).join('');
        }
    },

    renderSchedulerHistory(all) {
        const labels = all.slice().reverse().map(s => new Date(s.capture_timestamp));
        this.renderChart('wlCpuSchedulerHistoryChart', {
            type: 'line',
            labels,
            datasets: [
                { label: 'Workers', data: all.slice().reverse().map(s => s.total_current_workers_count), borderColor: '#3b82f6', tension: 0.3, pointRadius: 0 },
                { label: 'Runnable', data: all.slice().reverse().map(s => s.total_runnable_tasks_count), borderColor: '#f59e0b', tension: 0.3, pointRadius: 0 },
                { label: 'Queue', data: all.slice().reverse().map(s => s.total_work_queue_count), borderColor: '#ef4444', tension: 0.3, pointRadius: 0 },
            ],
        });
    },

    async loadQaSummary(from, to) {
        const { database } = this.getFilters();
        const dbParam = encodeURIComponent(database || 'all');
        const data = await api.get(
            `/api/sqlserver/query-analysis/summary?instance=${encodeURIComponent(this.instance)}` +
            `&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}` +
            `&exclude_system=true&database=${dbParam}`
        ) || {};
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('kpi-cpu-share', data.top_10_cpu_share_pct ? data.top_10_cpu_share_pct.toFixed(1) + '%' : '--');
        set('kpi-regressions', data.regressions_24h ?? '--');
    },

    async loadTrends(from, to) {
        const resp = await api.get(this.workloadUrl('trends', from, to)) || {};
        const data = resp.trends || [];

        const trendChartIds = ['wlCpuTrendChart', 'wlExecTrendChart', 'wlReadTrendChart'];
        if (data.length === 0) {
            trendChartIds.forEach(id => {
                if (this.charts[id]) { this.charts[id].destroy(); delete this.charts[id]; }
                if (window.setChartOverlayState) window.setChartOverlayState(id, 'empty', 'No data in selected range');
            });
            return;
        }
        trendChartIds.forEach(id => { if (window.clearChartOverlay) window.clearChartOverlay(id); });

        const labels = data.map(p => {
            const d = window.parseChartTimestamp ? window.parseChartTimestamp(p) : new Date(p.timestamp);
            return d && !isNaN(d.getTime()) ? d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '';
        });
        const cpuVals = data.map(p => (p.cpu_ms || 0) / 1000);
        const execVals = data.map(p => Number(p.executions) || 0);
        const readVals = data.map(p => Number(p.logical_reads) || 0);

        this.renderChart('wlCpuTrendChart', {
            type: 'line', labels,
            datasets: [{ label: 'CPU (sec per bucket)', data: cpuVals, borderColor: '#ef4444', fill: true, backgroundColor: 'rgba(239, 68, 68, 0.1)', tension: 0.3, pointRadius: 0 }],
        });
        if (cpuVals.every(v => v === 0) && window.setChartOverlayState) {
            window.setChartOverlayState('wlCpuTrendChart', 'empty', 'No CPU activity recorded in this range');
        }

        this.renderChart('wlExecTrendChart', {
            type: 'line', labels,
            datasets: [{ label: 'Executions (per bucket)', data: execVals, borderColor: '#3b82f6', fill: true, backgroundColor: 'rgba(59, 130, 246, 0.1)', tension: 0.3, pointRadius: 0 }],
        });
        if (execVals.every(v => v === 0) && window.setChartOverlayState) {
            window.setChartOverlayState('wlExecTrendChart', 'empty', 'No executions recorded in this range');
        }

        this.renderChart('wlReadTrendChart', {
            type: 'bar', labels,
            datasets: [{ label: 'Logical reads (per bucket)', data: readVals, backgroundColor: '#f59e0b', borderRadius: 4 }],
        });
        if (readVals.every(v => v === 0) && window.setChartOverlayState) {
            window.setChartOverlayState('wlReadTrendChart', 'empty', 'No logical reads recorded in this range');
        }
    },

    async loadTopOffenders(from, to) {
        const limit = this.topQueriesFetchLimit;
        const resp = await api.get(this.workloadUrl('top-queries', from, to) + `&limit=${limit}`) || {};
        const raw = (resp.top_offenders || []).filter(q =>
            String(q.database_name || '').trim().toLowerCase() !== 'distribution'
        );
        this.offenders = typeof window.dedupeSqlServerTopQueries === 'function'
            ? window.dedupeSqlServerTopQueries(raw)
            : raw;
        this.topQueriesPage = 0;
        this.renderTopQueriesTable();
    },

    sortOffenders(rows) {
        const key = this.sortKey;
        const dir = this.sortDir === 'asc' ? 1 : -1;
        return [...rows].sort((a, b) => {
            let va = a[key];
            let vb = b[key];
            if (key === 'last_seen') {
                va = va ? new Date(va).getTime() : 0;
                vb = vb ? new Date(vb).getTime() : 0;
            }
            if (key === 'total_reads') {
                va = a.total_reads ?? 0;
                vb = b.total_reads ?? 0;
            }
            if (key === 'avg_elapsed_ms' || key === 'total_elapsed_ms') {
                va = a.avg_elapsed_ms ?? 0;
                vb = b.avg_elapsed_ms ?? 0;
            }
            if (typeof va === 'string') va = va.toLowerCase();
            if (typeof vb === 'string') vb = vb.toLowerCase();
            if (va < vb) return -dir;
            if (va > vb) return dir;
            return 0;
        });
    },

    formatElapsedCell(q) {
        const avg = q.avg_elapsed_ms;
        if (avg == null || avg <= 0) return '<span class="text-muted">--</span>';
        return `<span class="text-end font-mono">${avg.toFixed(1)}</span>`;
    },

    renderTopQueriesPagination(totalRows, page, pageSize) {
        const el = document.getElementById('wlTopQueriesPagination');
        if (!el) return;
        const totalPages = Math.max(1, Math.ceil(totalRows / pageSize));
        const safePage = Math.min(page, totalPages - 1);
        const start = totalRows === 0 ? 0 : safePage * pageSize + 1;
        const end = Math.min((safePage + 1) * pageSize, totalRows);
        el.innerHTML = `
            <span>Showing ${start}–${end} of ${totalRows} queries</span>
            <div class="wl-page-btns">
                <button type="button" data-action="wl-top-queries-first" ${safePage <= 0 ? 'disabled' : ''} title="First page"><i class="fa-solid fa-angles-left"></i></button>
                <button type="button" data-action="wl-top-queries-prev" ${safePage <= 0 ? 'disabled' : ''} title="Previous"><i class="fa-solid fa-chevron-left"></i> Prev</button>
                <span>Page ${safePage + 1} / ${totalPages}</span>
                <button type="button" data-action="wl-top-queries-next" ${safePage >= totalPages - 1 ? 'disabled' : ''} title="Next">Next <i class="fa-solid fa-chevron-right"></i></button>
                <button type="button" data-action="wl-top-queries-last" ${safePage >= totalPages - 1 ? 'disabled' : ''} title="Last page"><i class="fa-solid fa-angles-right"></i></button>
            </div>`;
    },

    renderTopQueriesTable() {
        const body = document.getElementById('wlTopQueriesBody');
        if (!body) return;
        window.appState = window.appState || {};
        window.appState.queryCache = window.appState.queryCache || {};

        const sorted = this.sortOffenders(this.offenders);
        const pageSize = this.topQueriesPageSize;
        const totalPages = Math.max(1, Math.ceil(sorted.length / pageSize));
        if (this.topQueriesPage >= totalPages) this.topQueriesPage = totalPages - 1;
        if (this.topQueriesPage < 0) this.topQueriesPage = 0;

        this.renderTopQueriesPagination(sorted.length, this.topQueriesPage, pageSize);

        if (sorted.length === 0) {
            body.innerHTML = '<tr><td colspan="10" class="text-center text-muted">No query data found for this range</td></tr>';
            return;
        }

        const pageStart = this.topQueriesPage * pageSize;
        const pageRows = sorted.slice(pageStart, pageStart + pageSize);

        body.innerHTML = pageRows.map((q, i) => {
            const cacheKey = `wl_top_${pageStart + i}`;
            window.appState.queryCache[cacheKey] = {
                text: q.query_text,
                query_hash: q.query_hash,
                database_name: q.database_name,
            };
            const cpuSec = ((q.total_cpu_ms || 0) / 1000).toFixed(1);
            const variantBadge = (q.hash_variant_count > 1)
                ? `<span class="badge badge-muted ml-1" title="${q.hash_variant_count} query hashes rolled into this statement">${q.hash_variant_count} hashes</span>`
                : '';
            return `
            <tr>
                <td class="text-muted small" title="Server local time">${q.last_seen ? new Date(q.last_seen).toLocaleString() : '--'}</td>
                <td class="query-column small" style="cursor:pointer;" data-action="show-query-modal-direct" data-key="${cacheKey}" data-fn="showQueryStoreQueryModal">
                    <span class="query-text-truncated">${this.escapeHtml((q.query_text || '').substring(0, 100))}${(q.query_text || '').length > 100 ? '...' : ''}</span>${variantBadge}
                </td>
                <td class="text-end font-mono small">${formatters.compactNumber(q.total_executions || 0)}</td>
                <td class="text-end text-rose font-bold">${(q.avg_cpu_ms || 0).toFixed(1)}</td>
                <td class="text-end">${this.formatElapsedCell(q)}</td>
                <td class="text-end">${formatters.compactNumber(q.total_reads)}</td>
                <td class="text-end text-accent">${cpuSec}</td>
                <td><span class="badge badge-outline">${this.escapeHtml(q.database_name || '')}</span></td>
                <td class="small text-info">${this.escapeHtml(q.program_name || 'unknown')}</td>
                <td class="small">
                    <span class="text-warning">${this.escapeHtml(q.login_name || 'unknown')}</span>
                    ${q.query_hash ? `<a href="#" class="wl-qa-link ml-1" title="Open in Query Analysis" data-action="wl-open-qa" data-hash="${this.escapeHtml(q.query_hash)}"><i class="fa-solid fa-arrow-up-right-from-square"></i></a>` : ''}
                </td>
            </tr>`;
        }).join('');

        if (!this._topQueriesPageBound) {
            document.getElementById('wlTopQueriesPagination')?.addEventListener('click', (e) => {
                const btn = e.target.closest('button[data-action^="wl-top-queries-"]');
                if (!btn || btn.disabled) return;
                const action = btn.getAttribute('data-action');
                const total = this.offenders.length;
                const pages = Math.max(1, Math.ceil(total / this.topQueriesPageSize));
                if (action === 'wl-top-queries-first') this.topQueriesPage = 0;
                else if (action === 'wl-top-queries-prev') this.topQueriesPage = Math.max(0, this.topQueriesPage - 1);
                else if (action === 'wl-top-queries-next') this.topQueriesPage = Math.min(pages - 1, this.topQueriesPage + 1);
                else if (action === 'wl-top-queries-last') this.topQueriesPage = pages - 1;
                this.renderTopQueriesTable();
            });
            this._topQueriesPageBound = true;
        }

        if (!this._qaLinkBound) {
            document.getElementById('wlTopQueriesTable')?.addEventListener('click', (e) => {
                const link = e.target.closest('[data-action="wl-open-qa"]');
                if (!link) return;
                e.preventDefault();
                window.appState = window.appState || {};
                window.appState.wlQaSearchPrefill = link.getAttribute('data-hash') || '';
                if (typeof window.appNavigate === 'function') window.appNavigate('query-analysis');
                else document.querySelector('[data-route="query-analysis"]')?.click();
            });
            this._qaLinkBound = true;
        }
    },

    buildTimelineChart(raw, nameField) {
        const appGroups = {};
        const bucketSet = new Set();
        raw.forEach(r => {
            const key = bucketKey(r.bucket);
            bucketSet.add(key);
            const name = r[nameField];
            if (!appGroups[name]) appGroups[name] = {};
            appGroups[name][key] = (r.cpu_ms || 0) / 1000;
        });
        const labelKeys = [...bucketSet].sort();
        const labels = labelKeys.map(k => new Date(k));
        const datasets = Object.keys(appGroups).slice(0, 5).map((name, idx) => ({
            label: name,
            data: labelKeys.map(k => appGroups[name][k] || 0),
            borderColor: charts.colors[idx % charts.colors.length],
            pointRadius: 0,
            tension: 0.3,
        }));
        return { labels, datasets };
    },

    async loadAppAnalysis(from, to) {
        const [timelineResp, topResp] = await Promise.all([
            api.get(this.workloadUrl('app-load', from, to)),
            api.get(this.workloadUrl('top-apps', from, to) + '&limit=10'),
        ]);
        const raw = (timelineResp && timelineResp.app_load) || [];
        const { labels, datasets } = this.buildTimelineChart(raw, 'app_name');
        this.renderChart('wlAppLoadChart', { type: 'line', labels, datasets });

        const top = (topResp && topResp.top_apps) || [];
        this.renderChart('wlTopAppsChart', {
            type: 'bar',
            labels: top.map(x => x.app_name),
            datasets: [{ label: 'CPU sec', data: top.map(x => (x.total_cpu_ms || 0) / 1000), backgroundColor: 'rgba(59, 130, 246, 0.6)' }],
        });

        const totalCpu = top.reduce((a, b) => a + (b.total_cpu_ms || 0), 0);
        const body = document.getElementById('wlAppTableBody');
        if (body) {
            body.innerHTML = top.length
                ? top.map(x => `
                <tr><td>${this.escapeHtml(x.app_name)}</td><td>${((x.total_cpu_ms || 0) / 1000).toFixed(1)}</td><td>${totalCpu > 0 ? (((x.total_cpu_ms || 0) / totalCpu) * 100).toFixed(1) : 0}%</td></tr>
            `).join('')
                : '<tr><td colspan="3" class="text-center text-muted">No application data</td></tr>';
        }
    },

    async loadUserAnalysis(from, to) {
        const [timelineResp, topResp] = await Promise.all([
            api.get(this.workloadUrl('login-load', from, to)),
            api.get(this.workloadUrl('top-logins', from, to) + '&limit=10'),
        ]);
        const raw = (timelineResp && timelineResp.login_load) || [];
        const { labels, datasets } = this.buildTimelineChart(raw, 'login_name');
        this.renderChart('wlLoginLoadChart', { type: 'line', labels, datasets });

        const top = (topResp && topResp.top_logins) || [];
        this.renderChart('wlTopLoginsChart', {
            type: 'bar',
            labels: top.map(x => x.login_name),
            datasets: [{ label: 'CPU sec', data: top.map(x => (x.total_cpu_ms || 0) / 1000), backgroundColor: 'rgba(16, 185, 129, 0.6)' }],
        });

        const totalCpu = top.reduce((a, b) => a + (b.total_cpu_ms || 0), 0);
        const body = document.getElementById('wlLoginTableBody');
        if (body) {
            body.innerHTML = top.length
                ? top.map(x => `
                <tr><td>${this.escapeHtml(x.login_name)}</td><td>${((x.total_cpu_ms || 0) / 1000).toFixed(1)}</td><td>${totalCpu > 0 ? (((x.total_cpu_ms || 0) / totalCpu) * 100).toFixed(1) : 0}%</td></tr>
            `).join('')
                : '<tr><td colspan="3" class="text-center text-muted">No login data</td></tr>';
        }
    },

    workloadUrl(endpoint, from, to) {
        return `/api/sqlserver/workload/${endpoint}?instance=${encodeURIComponent(this.instance)}` +
            `&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}` +
            this.filterQueryString();
    },

    renderChart(id, config) {
        const ctx = document.getElementById(id);
        if (!ctx) return;
        if (typeof Chart !== 'undefined') {
            const existing = Chart.getChart(ctx);
            if (existing) {
                try { existing.destroy(); } catch (_) { /* ignore */ }
            }
        }
        if (this.charts[id]) {
            try { this.charts[id].destroy(); } catch (_) { /* ignore */ }
        }
        if (window.destroyChartOnCanvas) window.destroyChartOnCanvas(ctx);
        const xScale = { display: true, grid: { display: false }, ticks: { font: { size: 8 }, color: '#64748b', maxRotation: 30 } };
        const chartConfig = {
            type: config.type,
            data: { labels: config.labels, datasets: config.datasets },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 }, color: '#94a3b8' } } },
                scales: {
                    x: xScale,
                    y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.03)' }, ticks: { font: { size: 8 }, color: '#64748b' } },
                },
            },
        };
        this.charts[id] = (typeof window.createOptimaChart === 'function')
            ? window.createOptimaChart(ctx, chartConfig)
            : new Chart(ctx, chartConfig);
    },

    escapeHtml(unsafe) {
        if (unsafe === null || unsafe === undefined) return '';
        return String(unsafe)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    },
};

window.wlRefreshAll = () => sqlserverWorkload.refreshAll();
