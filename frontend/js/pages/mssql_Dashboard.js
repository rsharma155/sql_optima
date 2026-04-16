/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Main SQL Server dashboard view displaying key performance metrics (CPU, memory, PLE, waits, locks).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.escapeHtml = function(unsafe) {
    if (unsafe === null || unsafe === undefined) return '';
    return String(unsafe).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
};

window.DashboardView = async function() {
    appDebug('[Dashboard] Starting - config:', window.appState.config ? 'loaded' : 'not loaded');
    
    // Prevent multiple simultaneous loads
    if (window.appState.dashboardLoading) {
        appDebug('[Dashboard] Already loading, skipping...');
        return;
    }
    window.appState.dashboardLoading = true;
    
    // Check if config is loaded
    if (!window.appState.config || !window.appState.config.instances || window.appState.config.instances.length === 0) {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select an instance first</h3></div>`;
        window.appState.dashboardLoading = false;
        return;
    }
    
    appDebug('[Dashboard] instances:', window.appState.config.instances.length, 'currentInstanceIdx:', window.appState.currentInstanceIdx);
    
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    appDebug('[Dashboard] Selected instance:', inst);
    if (!inst || typeof inst !== 'object' || !inst.name) {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select an instance first</h3></div>`;
        window.appState.dashboardLoading = false;
        return;
    }
    if (inst.type === 'postgres') {
        window.appState.dashboardLoading = false;
        window.appNavigate('pg-dashboard');
        return;
    }
    window.appState.currentInstanceName = inst.name;
    window.appState.timescaleMetrics = { sqlserver: [], postgres: [], topQueries: [] };
    window.appState.lastUpdate = null;
    if (window.appState.dashboardPollingInterval) clearInterval(window.appState.dashboardPollingInterval);
    window.appState.dashboardPollingInterval = null;

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1>SQL Server Dashboard</h1>
                    <span class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Database: <span class="text-accent">all</span></span>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:1rem;">
                    <span id="dataSourceBadge" class="badge badge-info" style="display:none; font-size:0.65rem;">Source</span>
                    <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="lastRefreshTime">Loading...</span></span>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshDashboardData"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            <div style="display:flex; justify-content:center; align-items:center; height:50vh;">
                <div class="spinner"></div><span style="margin-left:1rem;">Loading metrics...</span>
            </div>
        </div>
    `;

    // Top Offenders grid: column sort (dashboard snapshot is always last 1h; change range in drilldown)
    window.appState.topOffendersGridSort = window.appState.topOffendersGridSort || { key: 'total_cpu_ms', dir: 'desc' };

    try {
        appDebug('[Dashboard] About to fetch dashboard data');
        
        // Phase 2: consume DBA-homepage v2 payload (Timescale-backed strip + compat legacy data).
        const res = await window.apiClient.authenticatedFetch(`/api/mssql/dashboard/v2?instance=${encodeURIComponent(inst.name)}`);
        updateDataSourceBadge(res.headers.get('X-Data-Source'));
        
        appDebug('[Dashboard] Response status:', res.status);
        
        const contentType = res.headers.get('content-type') || '';
        const bodyText = await res.text();
        if (!contentType.includes('application/json')) {
            appDebug('[Dashboard] Non-JSON response:', bodyText.substring(0, 200));
            console.error('[Dashboard] Non-JSON response from API');
            throw new Error(`Expected JSON but got ${contentType || 'unknown'} (${res.status})`);
        }

        let v2;
        try {
            v2 = JSON.parse(bodyText);
        } catch (parseErr) {
            throw new Error(`Invalid JSON from server (${res.status})`);
        }
        if (!res.ok) {
            const errMsg = (v2 && (v2.error || v2.message)) ? String(v2.error || v2.message) : `HTTP ${res.status}`;
            throw new Error(errMsg);
        }
        appDebug('[Dashboard] V2 data response:', Object.keys(v2));
        window.appState.dashboardV2 = v2;
        
        window.appState.dashboardLoading = false;

        // Use legacy compat payload for the existing renderer (incremental migration).
        const liveData = (v2 && v2.compat && v2.compat.dashboard) ? v2.compat.dashboard : v2;

        // Phase 3 charts: prefer Timescale trend series when present.
        // Disk latency trend: adapt v2.root_cause.disk_latency_trend_1h into the existing fileHistory shape
        // expected by updateIoChart() (snapshots with files[]).
        try {
            const ioTrend = v2?.root_cause?.disk_latency_trend_1h;
            if (Array.isArray(ioTrend) && ioTrend.length > 0) {
                window.appState.fileHistory = ioTrend.map(p => ({
                    timestamp: (p.timestamp ? new Date(p.timestamp).toISOString() : ''),
                    files: [{
                        read_latency_ms: Number(p.read_latency_ms || 0),
                        write_latency_ms: Number(p.write_latency_ms || 0)
                    }]
                }));
            }
        } catch (e) {}
        
        const metrics = {
            avg_cpu_load: liveData.avg_cpu_load ?? liveData.AvgCPULoad ?? liveData.cpu_usage ?? -1,
            memory_usage: liveData.memory_usage ?? liveData.MemoryUsage ?? liveData.memory_pct ?? -1,
            active_users: liveData.active_users != null ? liveData.active_users : -1,
            total_locks: liveData.total_locks ?? liveData.TotalLocks ?? 0,
            deadlocks: liveData.deadlocks ?? liveData.Deadlocks ?? 0,
            disk_usage: { data_mb: liveData.disk_usage?.data_mb ?? liveData.disk_usage?.DataMB ?? liveData.DiskUsage?.DataMB ?? 0, log_mb: liveData.disk_usage?.log_mb ?? liveData.disk_usage?.LogMB ?? liveData.DiskUsage?.LogMB ?? 0 },
            cpu_history: Array.isArray(liveData.cpu_history) ? liveData.cpu_history : Array.isArray(liveData.CPUHistory) ? liveData.CPUHistory : [],
            connection_stats: Array.isArray(liveData.connection_stats) ? liveData.connection_stats : Array.isArray(liveData.ConnectionStats) ? liveData.ConnectionStats : [],
            active_blocks: Array.isArray(liveData.active_blocks) ? liveData.active_blocks : Array.isArray(liveData.ActiveBlocks) ? liveData.ActiveBlocks : [],
            ple_history: Array.isArray(liveData.ple_history) ? liveData.ple_history : Array.isArray(liveData.PLEHistory) ? liveData.PLEHistory : [],
            file_history: Array.isArray(liveData.file_history) ? liveData.file_history : Array.isArray(liveData.FileHistory) ? liveData.FileHistory : [],
            locks_by_db: liveData.locks_by_db ?? liveData.LocksByDB ?? {},
            disk_by_db: liveData.disk_by_db ?? liveData.DiskByDB ?? {},
            top_queries: Array.isArray(liveData.top_queries) ? liveData.top_queries : Array.isArray(liveData.TopQueries) ? liveData.TopQueries : [],
            memory_clerks: Array.isArray(liveData.memory_clerks) ? liveData.memory_clerks : Array.isArray(liveData.MemoryClerks) ? liveData.MemoryClerks : [],
            wait_history: Array.isArray(liveData.wait_history) ? liveData.wait_history : Array.isArray(liveData.WaitHistory) ? liveData.WaitHistory : [],
            mem_history: Array.isArray(liveData.mem_history) ? liveData.mem_history : Array.isArray(liveData.MemHistory) ? liveData.MemHistory : []
        };
        
        appDebug('[Dashboard] Parsed metrics - cpu:', metrics.avg_cpu_load, 'mem:', metrics.memory_usage, 'cpu_hist:', metrics.cpu_history ? metrics.cpu_history.length : 'N/A');
        
        window.appState.liveMetrics = liveData;
        window.appState.pleHistory = metrics.ple_history;
        // fileHistory is set from v2 trend above when available; otherwise fall back to legacy metrics.
        if (!window.appState.fileHistory || window.appState.fileHistory.length === 0) {
            window.appState.fileHistory = metrics.file_history;
        }
        window.appState.waitHistory = metrics.wait_history;
        window.appState.lastUpdate = new Date();
        renderDashboard(inst, metrics);
        // keep badge updated after full render (renderDashboard re-creates header)
        updateDataSourceBadge(res.headers.get('X-Data-Source'));
        bindTopOffendersControls();
        if (window.updateAlertsBadge) window.updateAlertsBadge();
        if (window.updateSqlDashboardAlertBanner) window.updateSqlDashboardAlertBanner();
        startTimescalePolling(inst.name);
    } catch(error) {
        console.error("Dashboard fetch error:", error);
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-danger">Error loading dashboard</h3><p>${window.escapeHtml(String(error && error.message ? error.message : error))}</p></div>`;
    }
};

function updateDataSourceBadge(source) {
    const el = document.getElementById('dataSourceBadge');
    if (!el) return;
    if (!source) {
        el.style.display = 'none';
        return;
    }
    let label = 'Source';
    let cls = 'badge badge-info';
    if (source === 'timescale') {
        label = 'Source: Timescale snapshot';
        cls = 'badge badge-success';
    } else if (source === 'live_cache') {
        label = 'Source: Live (collector cache)';
        cls = 'badge badge-info';
    } else if (source === 'live_cache_fallback') {
        label = 'Source: Live cache fallback';
        cls = 'badge badge-warning';
    } else if (source === 'live_dmv_fallback') {
        label = 'Source: Live DMV fallback';
        cls = 'badge badge-warning';
    } else if (source === 'live_dmv_error') {
        label = 'Source: Live DMV error';
        cls = 'badge badge-danger';
    } else {
        label = 'Source: ' + source;
        cls = 'badge badge-info';
    }
    el.className = cls;
    el.textContent = label;
    el.style.display = 'inline-flex';
}

function computeOverallStatus(metrics) {
    const cpu = (typeof metrics.avg_cpu_load === 'number') ? metrics.avg_cpu_load : -1;
    const mem = (typeof metrics.memory_usage === 'number') ? metrics.memory_usage : -1;
    const blocked = (window.appState.liveMetrics?.active_blocks || []).some(s => (s.blocking_session_id || 0) !== 0);
    const hasLongRunning = (window.appState.timescaleMetrics?.longRunningQueries || []).length > 0;

    // very simple scoring for header chip only
    let level = 'OK';
    if (blocked || hasLongRunning) level = 'WARNING';
    if (cpu > 90 || mem > 95) level = 'WARNING';
    if ((cpu > 97 && cpu <= 100) || (mem > 98 && mem <= 100)) level = 'CRITICAL';

    return level;
}

// NOTE: Do not declare helpers with the same name as shared `template.js` helpers
// (e.g. `window.renderStatusStrip`) to avoid clobbering global UI utilities.

window.refreshDashboardData = function() { 
    window.appState.dashboardLoading = false;
    window.DashboardView(); 
};

function dashboardDatabaseQueryParam() {
    const db = window.appState.currentDatabase;
    if (!db || db === 'all') return '';
    return '&database=' + encodeURIComponent(db);
}
window.dashboardDatabaseQueryParam = dashboardDatabaseQueryParam;

/** Execution count for Query Store / bottleneck rows (field names differ by source). */
function topOffenderExecCount(q) {
    if (!q) return 0;
    const v = q.execution_count != null ? q.execution_count : (q.total_executions != null ? q.total_executions : q.executions);
    return Number(v || 0);
}

function sortQueryStoreOffenderRows(rows, state) {
    if (!rows || !rows.length) return [];
    const key = (state && state.key) || 'total_cpu_ms';
    const dir = (state && state.dir) === 'asc' ? 1 : -1;
    const textKeys = { database_name: true, query_text: true };
    return [...rows].sort((a, b) => {
        if (textKeys[key]) {
            const va = String((a && a[key]) || '').toLowerCase();
            const vb = String((b && b[key]) || '').toLowerCase();
            return dir * va.localeCompare(vb);
        }
        let va;
        let vb;
        if (key === 'execution_count') {
            va = topOffenderExecCount(a);
            vb = topOffenderExecCount(b);
        } else {
            va = Number(a && a[key] != null ? a[key] : 0);
            vb = Number(b && b[key] != null ? b[key] : 0);
        }
        if (va === vb) return 0;
        return dir * (va < vb ? -1 : 1);
    });
}

function renderTopOffendersRowsHtml(sortedRows) {
    window.appState.queryCache = window.appState.queryCache || {};
    return sortedRows.map((q, idx) => {
        const rawText = String(q.query_text || q.Query_Text || q.queryText || q.QueryText || '').trim();
        const fallbackHash = q.query_hash || q.queryHash || '';
        const qt = rawText || (fallbackHash ? `Query hash: ${fallbackHash}` : 'No query text available');
        const dbn = (q.database_name != null && q.database_name !== '') ? String(q.database_name) : '—';
        window.appState.queryCache['qs' + idx] = {
            text: qt,
            query_hash: fallbackHash,
            database_name: dbn
        };
        const short = rawText
            ? (rawText.length > 60 ? rawText.substring(0, 60) + '…' : rawText)
            : (fallbackHash ? `Hash: ${fallbackHash.substring(0, 12)}` : 'No query text');
        const avgDur = Number(q.avg_duration_ms || 0);
        const avgCpu = Number(q.avg_cpu_ms || 0);
        const avgReads = Number(q.avg_logical_reads || 0);
        const totalCpu = Number(q.total_cpu_ms || 0);
        const execs = topOffenderExecCount(q);
        return `<tr>
            <td><strong>${idx + 1}</strong></td>
            <td title="${window.escapeHtml(dbn)}">${window.escapeHtml(dbn.length > 24 ? dbn.substring(0, 24) + '…' : dbn)}</td>
            <td style="max-width:480px;">
                <span class="code-snippet" style="cursor:pointer" data-action="show-query-modal-direct" data-key="qs${idx}" data-fn="showQueryStoreQueryModal" title="${window.escapeHtml(qt)}">${window.escapeHtml(short)}</span>
            </td>
            <td><span class="badge badge-outline">${execs.toLocaleString()}</span></td>
            <td>${avgCpu.toFixed(1)}</td>
            <td>${avgDur.toFixed(1)}</td>
            <td>${avgReads.toFixed(0)}</td>
            <td>${totalCpu.toFixed(1)}</td>
        </tr>`;
    }).join('');
}

// Drill-down for Query Store rows: fetch full SQL text on demand.
window.showQueryStoreQueryModal = async function(qs) {
    const safe = (qs && typeof qs === 'object') ? qs : { text: String(qs || '') };
    const instance = window.appState.currentInstanceName || '';
    const database = String(safe.database_name || '').trim();
    const queryHash = String(safe.query_hash || '').trim();
    const previewText = String(safe.text || 'No query available');

    // Show preview immediately.
    window.showQueryModalDirect(previewText);

    // Only attempt fetch when we have enough identity to look it up.
    if (!instance || !database || database === '—' || !queryHash) return;

    // Fetch full text and swap into the existing modal.
    try {
        const res = await window.apiClient.authenticatedFetch(
            `/api/queries/query-store/sql-text?instance=${encodeURIComponent(instance)}&database=${encodeURIComponent(database)}&query_hash=${encodeURIComponent(queryHash)}`
        );
        if (!res.ok) return;
        const data = await res.json().catch(() => null);
        const full = data && data.query_text ? String(data.query_text) : '';
        if (!full || full.trim() === '' || full === previewText) return;
        const pre = document.querySelector('#query-modal pre');
        if (pre) pre.textContent = full;
    } catch (e) {}
};

function updateTopOffendersHeaderSortIndicators() {
    const table = document.getElementById('topOffendersGrid');
    if (!table) return;
    const state = window.appState.topOffendersGridSort || { key: 'total_cpu_ms', dir: 'desc' };
    table.querySelectorAll('thead th[data-sort-key]').forEach((th) => {
        const k = th.getAttribute('data-sort-key');
        const icon = th.querySelector('.sort-icon');
        if (!icon) return;
        if (k === state.key) {
            th.classList.add('th-sort-active');
            th.style.color = 'var(--accent, #3b82f6)';
            icon.className = 'fa-solid sort-icon ' + (state.dir === 'asc' ? 'fa-sort-up' : 'fa-sort-down');
        } else {
            th.classList.remove('th-sort-active');
            th.style.color = '';
            icon.className = 'fa-solid fa-sort sort-icon';
        }
    });
}

window.onTopOffendersGridSortClick = function(ev) {
    const th = ev.target.closest('th[data-sort-key]');
    if (!th || !document.getElementById('topOffendersGrid') || !th.closest('#topOffendersGrid')) return;
    ev.preventDefault();
    const key = th.getAttribute('data-sort-key');
    const state = Object.assign({}, window.appState.topOffendersGridSort || { key: 'total_cpu_ms', dir: 'desc' });
    if (state.key === key) {
        state.dir = state.dir === 'asc' ? 'desc' : 'asc';
    } else {
        state.key = key;
        state.dir = (key === 'database_name' || key === 'query_text') ? 'asc' : 'desc';
    }
    window.appState.topOffendersGridSort = state;
    updateTopOffendersTable();
};

function bindTopOffendersGridSort() {
    const table = document.getElementById('topOffendersGrid');
    if (!table || table.dataset.sortDelegateBound === '1') return;
    table.dataset.sortDelegateBound = '1';
    const thead = table.querySelector('thead');
    if (thead) thead.addEventListener('click', window.onTopOffendersGridSortClick);
}

async function fetchTimescaleMetrics(instanceName) {
    try {
        // Add a guard to prevent duplicate fetches
        if (window.appState.fetchingMetrics) {
            appDebug('[Dashboard] Already fetching metrics, skipping...');
            return;
        }
        window.appState.fetchingMetrics = true;
        window.appState.metricsRefreshInProgress = true;
        if (window.appState.activeViewId === 'dashboard') {
            updateDashboardCharts();
        }

        const dbQ = dashboardDatabaseQueryParam();
        const topOffendersSnapshotRange = '1h';
        const [mssqlRes, dbRes, topQueriesRes, longRunningRes, bottlenecksRes, liveDashRes] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/timescale/mssql/metrics?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/mssql/db-throughput?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/timescale/mssql/top-queries?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/timescale/mssql/long-running-queries?instance=${encodeURIComponent(instanceName)}${dbQ}`),
            window.apiClient.authenticatedFetch(`/api/queries/bottlenecks?instance=${encodeURIComponent(instanceName)}&time_range=${encodeURIComponent(topOffendersSnapshotRange)}&limit=20${dbQ}`),
            window.apiClient.authenticatedFetch(`/api/mssql/dashboard/v2?instance=${encodeURIComponent(instanceName)}`)
        ]).finally(() => {
            window.appState.fetchingMetrics = false;
        });
        
        if (mssqlRes.ok) {
            const data = await mssqlRes.json();
            window.appState.timescaleMetrics.sqlserver = data.metrics || [];
        }
        if (dbRes.ok) {
            const data = await dbRes.json();
            window.appState.timescaleMetrics.throughput = data.db_stats || [];
        }
        if (topQueriesRes.ok) {
            const data = await topQueriesRes.json();
            window.appState.timescaleMetrics.topQueries = data.top_queries || [];
        }
        if (longRunningRes.ok) {
            const data = await longRunningRes.json();
            appDebug("Long running queries API response:", data);
            window.appState.timescaleMetrics.longRunningQueries = data.long_running_queries || [];
        } else {
            appDebug("Failed to fetch long-running queries:", longRunningRes.status);
        }
        if (bottlenecksRes.ok) {
            const data = await bottlenecksRes.json();
            window.appState.queryStoreBottlenecks = data.bottlenecks || [];
        } else {
            window.appState.queryStoreBottlenecks = [];
        }
        if (liveDashRes.ok) {
            const v2 = await liveDashRes.json();
            window.appState.dashboardV2 = v2;
            const ld = (v2 && v2.compat && v2.compat.dashboard) ? v2.compat.dashboard : v2;
            window.appState.liveMetrics = ld;
            window.appState.pleHistory = ld.ple_history || ld.PLEHistory || window.appState.pleHistory || [];
            // Prefer v2 disk latency trend (Timescale-backed) when present.
            const ioTrend = (v2 && v2.root_cause && Array.isArray(v2.root_cause.disk_latency_trend_1h)) ? v2.root_cause.disk_latency_trend_1h : null;
            if (ioTrend && ioTrend.length > 0) {
                // Must match DashboardView + updateIoChart: timestamp + files[] (not flat capture_time / p.ts).
                window.appState.fileHistory = ioTrend.map(p => ({
                    timestamp: (p.timestamp ? new Date(p.timestamp).toISOString() : ''),
                    files: [{
                        read_latency_ms: Number(p.read_latency_ms ?? 0),
                        write_latency_ms: Number(p.write_latency_ms ?? 0)
                    }]
                }));
            } else {
                window.appState.fileHistory = ld.file_history || ld.FileHistory || window.appState.fileHistory || [];
            }
            window.appState.waitHistory = ld.wait_history || ld.WaitHistory || window.appState.waitHistory || [];
        }
        window.appState.lastUpdate = new Date();
        updateLastRefreshTime();
        if (window.appState.activeViewId === 'dashboard' && window.updateSqlDashboardAlertBanner) {
            window.updateSqlDashboardAlertBanner();
        }
    } catch (e) { 
        appDebug("Failed to fetch metrics:", e); 
        // Still try to update time even on error
        window.appState.lastUpdate = new Date();
        updateLastRefreshTime();
    } finally {
        window.appState.metricsRefreshInProgress = false;
    }
}

function startTimescalePolling(instanceName) {
    if (window.appState.dashboardPollingInterval) clearInterval(window.appState.dashboardPollingInterval);

    const fetchAndUpdate = async () => {
        await fetchTimescaleMetrics(instanceName);
        if (window.appState.activeViewId === 'dashboard') {
            updateDashboardCharts();
        }
    };

    // Fetch immediately and then poll on interval.
    fetchAndUpdate().catch((err) => appDebug('[Dashboard] Timescale polling error:', err));

    window.appState.dashboardPollingInterval = setInterval(async () => {
        if (window.appState.activeViewId === 'dashboard') {
            await fetchAndUpdate();
        }
    }, 30000); // Increased to 30 seconds to reduce API calls
}

async function updateDashboardCharts() {
    if (!window.appState.currentInstanceName) return;
    
    // Use cached data - don't refetch on every call
    const liveData = window.appState.liveMetrics || {};
    const tsMetrics = window.appState.timescaleMetrics.sqlserver || [];
    
    // Get latest from TimescaleDB or use live data
    const latestMetric = tsMetrics.length > 0 ? tsMetrics[tsMetrics.length - 1] : {};
    
    // CPU: prefer live data, fallback to TimescaleDB (handle undefined/null)
    const cpu = (liveData.avg_cpu_load !== undefined && liveData.avg_cpu_load !== null && liveData.avg_cpu_load >= 0 && liveData.avg_cpu_load <= 100) ? liveData.avg_cpu_load : 
                (latestMetric.avg_cpu_load !== undefined && latestMetric.avg_cpu_load !== null && latestMetric.avg_cpu_load >= 0 && latestMetric.avg_cpu_load <= 100) ? latestMetric.avg_cpu_load : -1;
    
    // Memory: prefer live data, fallback to TimescaleDB  
    const memory = (liveData.memory_usage !== undefined && liveData.memory_usage !== null && liveData.memory_usage >= 0 && liveData.memory_usage <= 100) ? liveData.memory_usage :
                   (latestMetric.memory_usage !== undefined && latestMetric.memory_usage !== null && latestMetric.memory_usage >= 0 && latestMetric.memory_usage <= 100) ? latestMetric.memory_usage : -1;
    
    // TPS: calculate from throughput or live data
    const throughput = window.appState.timescaleMetrics.throughput || [];
    let tps = 0;
    if (throughput.length > 0) {
        tps = throughput.reduce((s,i) => s + (i.avg_tps || i.tps || 0), 0);
    }
    
    appDebug('[Dashboard] Updating charts - cpu:', cpu, 'mem:', memory, 'tps:', tps, 'liveData keys:', Object.keys(liveData));

    const metricsRefreshing = !!window.appState.metricsRefreshInProgress;
    updateChartLoadingOverlays(metricsRefreshing && (!window.appState.timescaleMetrics.sqlserver || window.appState.timescaleMetrics.sqlserver.length === 0));
    
    const cpuEl = document.getElementById('metric-cpu');
    const memEl = document.getElementById('metric-memory');
    
    // Always update CPU display (handle -1 as "no data")
    if (cpuEl) {
        const parent = cpuEl.closest('.metric-value');
        if (parent) {
            const loading = parent.querySelector('.metric-loading');
            if (loading) loading.remove();
        }
        if (metricsRefreshing && cpu < 0) {
            cpuEl.innerHTML = '<span style="font-size:0.75rem;color:var(--text-muted);"><i class="fa-solid fa-spinner fa-spin"></i> Loading</span>';
        } else {
            cpuEl.textContent = cpu >= 0 ? cpu.toFixed(1) + '%' : 'N/A';
        }
        const card = cpuEl.closest('.metric-card');
        if (card) card.className = 'metric-card glass-panel ' + (cpu > 85 ? 'status-danger' : (cpu > 60 ? 'status-warning' : 'status-healthy'));
    }
    
    // Always update Memory display
    if (memEl) {
        const parent = memEl.closest('.metric-value');
        if (parent) {
            const loading = parent.querySelector('.metric-loading');
            if (loading) loading.remove();
        }
        if (metricsRefreshing && memory < 0) {
            memEl.innerHTML = '<span style="font-size:0.75rem;color:var(--text-muted);"><i class="fa-solid fa-spinner fa-spin"></i> Loading</span>';
        } else {
            memEl.textContent = memory >= 0 ? memory.toFixed(1) + '%' : 'N/A';
        }
        const card = memEl.closest('.metric-card');
        if (card) card.className = 'metric-card glass-panel ' + (memory > 95 ? 'status-danger' : (memory > 85 ? 'status-warning' : 'status-healthy'));
    }
    
    // Update active users (handle -1 as no data)
    const activeEl = document.getElementById('metric-active');
    if (activeEl) {
        const parent = activeEl.closest('.metric-value');
        if (parent) {
            const loading = parent.querySelector('.metric-loading');
            if (loading) loading.remove();
        }
        const active = (liveData.active_users !== undefined && liveData.active_users !== null) ? liveData.active_users : 
                       (liveData.ActiveUsers !== undefined && liveData.ActiveUsers !== null) ? liveData.ActiveUsers : -1;
        if (metricsRefreshing && active < 0) {
            activeEl.innerHTML = '<span style="font-size:0.75rem;color:var(--text-muted);"><i class="fa-solid fa-spinner fa-spin"></i></span>';
        } else {
            activeEl.textContent = active >= 0 ? active : 'N/A';
        }
    }
    
    // Update blocked count using active block sessions from DMV live data.
    const blockedEl = document.getElementById('metric-blocked');
    if (blockedEl) {
        const blocked = Array.isArray(liveData.active_blocks) ? liveData.active_blocks.length :
            (Array.isArray(liveData.ActiveBlocks) ? liveData.ActiveBlocks.length : 0);
        blockedEl.textContent = blocked;
    }
    
    // Update TPS display
    const tpsCard = Array.from(document.querySelectorAll('.metric-card')).find(c => c.textContent.includes('TPS'));
    if (tpsCard) {
        const tpsEl = tpsCard.querySelector('.metric-value');
        if (tpsEl) {
            const loading = tpsEl.querySelector('.metric-loading');
            if (loading) loading.remove();
            if (metricsRefreshing && tps <= 0) {
                tpsEl.innerHTML = '<span style="font-size:0.75rem;color:var(--text-muted);"><i class="fa-solid fa-spinner fa-spin"></i> Loading</span>';
            } else {
                tpsEl.textContent = tps > 0 ? tps.toFixed(1) : 'N/A';
            }
        }
    }
    
    // Update CPU chart from live data
    if (window.currentCharts && window.currentCharts.dashRes) {
        const cpuHist = liveData.cpu_history || liveData.CPUHistory || [];
        if (cpuHist.length > 0) {
            const sorted = [...cpuHist].sort((a, b) => {
                const ta = a.event_time ? new Date(a.event_time.replace(' ','T')).getTime() : 0;
                const tb = b.event_time ? new Date(b.event_time.replace(' ','T')).getTime() : 0;
                return ta - tb;
            }).slice(-60);
            window.currentCharts.dashRes.data.labels = sorted.map(t => { if(!t.event_time) return ''; const parts = t.event_time.split(' '); if(parts.length < 2) return ''; return parts[1].substring(0,5); });
            window.currentCharts.dashRes.data.datasets[0].data = sorted.map(t => t.sql_process);
            window.currentCharts.dashRes.data.datasets[1].data = sorted.map(t => t.system_idle);
            window.currentCharts.dashRes.update('none');
        }
    }
    
    // Update PLE chart
    if (window.updatePleChart) window.updatePleChart();
    // Update IO chart
    if (window.updateIoChart) window.updateIoChart();
    // Update Wait Categories donut (Phase 2)
    if (window.updateWaitCategoriesDonut) window.updateWaitCategoriesDonut();
    // Update Batch Requests trend
    if (window.updateBatchChart) window.updateBatchChart();
    // Update Buffer Cache Hit ratio
    if (window.updateBchrChart) window.updateBchrChart();
    
    // Update Active Sessions table
    updateActiveSessionsTable();
    
    // Update Query Store offenders table (best-effort)
    updateTopOffendersTable();

    // Update Long Running Queries table
    updateLongRunningQueriesTable();
    
    updateLastRefreshTime();
    if (window.updateSqlDashboardAlertBanner) window.updateSqlDashboardAlertBanner();
}

function updateChartLoadingOverlays(show) {
    const chartIds = ['dashResourcesChart', 'dashBatchChart', 'dashIoChart', 'dashPleChart', 'dashBchrChart', 'dashWaitCategoriesDonut'];
    chartIds.forEach((chartId) => {
        const canvas = document.getElementById(chartId);
        if (!canvas) return;
        const parent = canvas.parentElement;
        if (!parent) return;
        parent.style.position = parent.style.position || 'relative';
        let overlay = parent.querySelector('.dashboard-chart-loading-overlay');
        if (show) {
            if (!overlay) {
                overlay = document.createElement('div');
                overlay.className = 'dashboard-chart-loading-overlay';
                overlay.style.cssText = 'position:absolute;top:0;left:0;width:100%;height:100%;display:flex;align-items:center;justify-content:center;background:rgba(255,255,255,0.75);color:var(--text-muted, #6b7280);font-size:0.85rem;z-index:2;padding:0.5rem;';
                overlay.innerHTML = '<div style="text-align:center;"><div class="spinner"></div><div style="margin-top:0.5rem;">Loading chart data...</div></div>';
                parent.appendChild(overlay);
            }
        } else if (overlay) {
            overlay.remove();
        }
    });
}

function updateTopOffendersTable() {
    const tbody = document.getElementById('top-offenders-body');
    if (!tbody) return;

    let qsOffenders = window.appState.queryStoreBottlenecks || [];
    if (window.appState.fetchingMetrics) {
        tbody.innerHTML = '<tr><td colspan="8" class="text-center"><div class="spinner"></div> Loading Query Store offenders...</td></tr>';
        return;
    }
    if (!qsOffenders || qsOffenders.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">No Query Store offenders for this scope. Enable Query Store per database, ensure the collector runs, or try &quot;All databases&quot;.</td></tr>';
        updateTopOffendersHeaderSortIndicators();
        return;
    }

    const state = window.appState.topOffendersGridSort || { key: 'total_cpu_ms', dir: 'desc' };
    const sorted = sortQueryStoreOffenderRows(qsOffenders, state);
    tbody.innerHTML = renderTopOffendersRowsHtml(sorted);
    updateTopOffendersHeaderSortIndicators();
    bindTopOffendersGridSort();
}

async function refreshTopOffenders(instanceName) {
    const tbody = document.getElementById('top-offenders-body');
    if (tbody) {
        tbody.innerHTML = '<tr><td colspan="8" class="text-center"><div class="spinner"></div> Loading Query Store offenders...</td></tr>';
    }

    const dbQ = dashboardDatabaseQueryParam();
    try {
        const res = await window.apiClient.authenticatedFetch(
            `/api/queries/bottlenecks?instance=${encodeURIComponent(instanceName)}&time_range=1h&limit=20${dbQ}`
        );
        if (res.ok) {
            const data = await res.json();
            window.appState.queryStoreBottlenecks = data.bottlenecks || [];
        } else {
            window.appState.queryStoreBottlenecks = [];
        }
    } catch (e) {
        window.appState.queryStoreBottlenecks = [];
    }
    updateTopOffendersTable();
}

function bindTopOffendersControls() {
    bindTopOffendersGridSort();
    updateTopOffendersHeaderSortIndicators();
}

function updateActiveSessionsTable() {
    const tbody = document.querySelector('#active-sessions-body');
    if (!tbody) return;
    
    const liveData = window.appState.liveMetrics || {};
    const allSessions = liveData.active_blocks || [];
    const dbFilter = window.appState.currentDatabase || 'all';
    
    let sessList = allSessions;
    if (dbFilter !== 'all') sessList = sessList.filter(s => s.database_name === dbFilter);
    const significantSessions = sessList.filter(s => { const d = s.wait_time_ms||0; return ((s.status||'').toLowerCase()==='running' || s.blocking_session_id!==0) && d > 5000; });
    const sortedSessions = [...significantSessions].sort((a,b) => { if(a.blocking_session_id!==0 && b.blocking_session_id===0) return -1; if(a.blocking_session_id===0 && b.blocking_session_id!==0) return 1; return (b.wait_time_ms||0)-(a.wait_time_ms||0); });
    
    if (window.appState.fetchingMetrics) {
        tbody.innerHTML = '<tr><td colspan="9" class="text-center"><div class="spinner"></div> Loading sessions...</td></tr>';
    } else if (sortedSessions.length === 0) {
        tbody.innerHTML = '<tr><td colspan="9" class="text-center text-muted">No significant sessions.</td></tr>';
    } else {
        tbody.innerHTML = sortedSessions.slice(0,25).map((s, idx) => renderSessionRow(s, idx)).join('');
    }
    
    // Update the badge count
    const badge = document.querySelector('#active-sessions-badge');
    if (badge) badge.textContent = sortedSessions.length;
}

/** Groups long-running rows (same logic as first paint + refresh) for consistent counts and columns. */
function groupLongRunningQueriesForDisplay(longRunningData, dbFilter) {
    const excludedDbs = ['master', 'model', 'msdb', 'distribution'];
    const filter = dbFilter || 'all';
    let filteredByDb = (longRunningData || []).filter(q => !excludedDbs.includes((q.database_name || '').toLowerCase()));
    if (filter !== 'all') {
        filteredByDb = filteredByDb.filter(q => (q.database_name || '') === filter);
    }
    const groups = new Map();
    filteredByDb.forEach(q => {
        const rawText = q.query_text || q.Query_Text || q['query_text'] || '';
        const norm = rawText.replace(/'[^']*'/g, "'?'").replace(/\b\d+(\.\d+)?\b/g, '?').replace(/\s+/g, ' ').trim().substring(0, 100);
        const dbName = q.database_name || 'Unknown';
        const sessionId = q.session_id || 0;
        const key = dbName + '|||' + norm + '|||' + sessionId;
        if (!groups.has(key)) {
            groups.set(key, {
                queryText: rawText,
                dbName: dbName,
                sessionId: sessionId,
                loginName: q.login_name || 'N/A',
                programName: q.program_name || q.programName || '',
                waitType: q.wait_type || 'N/A',
                status: q.status || 'N/A',
                cpuTimeMs: parseFloat(q.cpu_time_ms || q.CPUTimeMs || 0),
                totalElapsedMs: parseFloat(q.total_elapsed_time_ms || q.TotalElapsedTimeMs || 0),
                captureTimestamp: q.capture_timestamp
            });
        } else {
            const g = groups.get(key);
            if (q.total_elapsed_time_ms > g.totalElapsedMs) {
                g.totalElapsedMs = q.total_elapsed_time_ms;
                g.cpuTimeMs = q.cpu_time_ms;
                g.queryText = rawText;
                g.captureTimestamp = q.capture_timestamp;
                g.programName = q.program_name || q.programName || g.programName;
            }
        }
    });
    return Array.from(groups.values()).sort((a, b) => b.totalElapsedMs - a.totalElapsedMs);
}

function updateLongRunningQueriesTable() {
    const tbody = document.querySelector('#long-running-body');
    if (!tbody) return;
    
    if (window.appState.fetchingMetrics) {
        tbody.innerHTML = '<tr><td colspan="8" class="text-center"><div class="spinner"></div> Loading long running queries...</td></tr>';
        return;
    }

    window.appState.queryCache = window.appState.queryCache || {};
    const longRunningData = window.appState.timescaleMetrics.longRunningQueries || [];
    const dbFilter = window.appState.currentDatabase || 'all';
    const filteredData = groupLongRunningQueriesForDisplay(longRunningData, dbFilter);
    
    if (filteredData.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">No long running queries for this database and time range.</td></tr>';
    } else {
        const sortedData = filteredData.sort((a, b) => b.totalElapsedMs - a.totalElapsedMs);
        const displayQueries = sortedData.slice(0, 10);
        displayQueries.forEach((q, idx) => {
            window.appState.queryCache['lrq' + idx] = q.queryText || 'No query';
        });
        
        tbody.innerHTML = displayQueries.map((q, idx) => {
            const ts = q.captureTimestamp ? new Date(q.captureTimestamp) : null;
            const tsStr = ts ? ts.toLocaleTimeString('en-US', {hour: '2-digit', minute: '2-digit'}) : '';
            const ageMinutes = ts ? Math.floor((Date.now() - ts.getTime()) / 60000) : -1;
            const ageStr = ageMinutes >= 0 && ageMinutes < 60 ? `(${ageMinutes}m)` : (tsStr || '');
            const qt = (q.queryText || 'Unknown').substring(0, 40);
            const app = q.programName != null ? String(q.programName) : '';
            return `<tr><td><span class="badge badge-outline">${tsStr}</span><span class="text-muted" style="font-size:0.65rem; margin-left:4px;">${ageStr}</span></td><td><span class="code-snippet" style="cursor:pointer" data-action="show-query-modal-direct" data-key="lrq${idx}">${window.escapeHtml(qt)}</span></td><td>${window.escapeHtml(q.dbName)}</td><td>${window.escapeHtml(q.loginName)}</td><td title="${window.escapeHtml(app)}">${window.escapeHtml(app.length > 28 ? app.substring(0, 28) + '…' : app)}</td><td>${(q.cpuTimeMs || 0)}ms</td><td>${(q.totalElapsedMs || 0)}ms</td><td>${window.escapeHtml(q.waitType)}</td></tr>`;
        }).join('');
    }
    
    // Update the badge count
    const badge = document.querySelector('#long-running-badge');
    if (badge) badge.textContent = filteredData.length;
}

function updateLastRefreshTime() {
    const timeEl = document.getElementById('lastRefreshTime');
    if (timeEl && window.appState.lastUpdate) timeEl.textContent = window.appState.lastUpdate.toLocaleTimeString();
}

function renderSessionRow(session, index) {
    const isBlocked = session.blocking_session_id !== 0;
    const duration = session.wait_time_ms || 0;
    let rowClass = isBlocked ? 'bg-danger-subtle' : '';
    let durationClass = duration > 30000 ? 'text-danger fw-bold' : (duration > 10000 ? 'text-warning fw-bold' : '');
    const spid = session.blocked_session_id || session.session_id || 0;
    const sqlText = window.escapeHtml(session.query_text || 'N/A');
    return `<tr class="${rowClass}" data-spid="${spid}" data-index="${index}" data-session='${encodeURIComponent(JSON.stringify(session))}' style="cursor:pointer;">
        <td><strong>${spid}</strong></td>
        <td>${isBlocked ? '<span class="badge badge-danger"> BLOCKING</span>' : '<span class="badge badge-success">RUNNING</span>'}</td>
        <td>${isBlocked ? session.blocking_session_id : '-'}</td>
        <td><span class="${durationClass}">${(duration/1000).toFixed(1)}s</span></td>
        <td><span class="badge badge-outline">${window.escapeHtml(session.status || 'N/A')}</span></td>
        <td><span class="badge badge-info">${window.escapeHtml(session.wait_type || 'N/A')}</span></td>
        <td style="font-size:0.75rem;">${window.escapeHtml(session.database_name || 'N/A')}</td>
        <td style="font-size:0.75rem;">${window.escapeHtml(session.login_name || 'N/A')}</td>
        <td style="font-size:0.7rem; max-width:150px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; cursor:pointer; color:var(--accent);" data-action="call" data-fn="showSessionDetailFromRow" data-pass-el="1">${sqlText.substring(0,50)}...</td>
    </tr>`;
}

window.showSessionDetailFromRow = function(cell) {
    const row = cell.closest('tr');
    if (!row) return;
    const spid = row.dataset.spid;
    window.showSessionDetail(spid);
};

window.showSessionDetail = function(spid) {
    const row = document.querySelector(`tr[data-spid="${spid}"]`);
    if (!row) return;
    let session = {}; try { session = JSON.parse(decodeURIComponent(row.dataset.session)); } catch(e) { return; }
    const modal = document.getElementById('session-detail-modal'); if(modal) modal.remove();
    const modalEl = document.createElement('div'); modalEl.id = 'session-detail-modal';
    // Use CSS variables for theme-aware styling
    modalEl.style.cssText = 'display:flex; position:fixed; z-index:9999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.5); align-items:center; justify-content:center;';
    // Use theme colors from CSS variables
    modalEl.innerHTML = `<div style="background:var(--bg-secondary, #ffffff); margin:2%; padding:20px; border:1px solid var(--border-color, #e5e7eb); border-radius:12px; width:95%; max-width:1000px; max-height:90vh; overflow-y:auto; color:var(--text-primary, #1f2937);">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:1rem; border-bottom:1px solid var(--border-color, #e5e7eb); padding-bottom:0.75rem;">
            <h3 style="margin:0; color:var(--accent, #3b82f6);"><i class="fa-solid fa-id-card"></i> Session Detail - SPID ${spid}</h3>
            <button data-action="call" data-fn="closeSessionDetail" style="background:transparent; border:1px solid var(--border-color, #d1d5db); color:var(--text-primary, #1f2937); font-size:1.25rem; cursor:pointer; padding:0.25rem 0.6rem; border-radius:4px;">&times;</button>
        </div>
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:1rem; margin-bottom:1rem;">
            <div class="glass-panel" style="background:var(--bg-tertiary, #f9fafb); padding:1rem;"><h4 style="margin:0 0 0.75rem 0; color:var(--accent, #3b82f6);">Identity</h4>
                <div style="display:grid; gap:0.5rem; font-size:0.85rem;">
                    <div><span style="color:var(--text-muted, #6b7280);">SPID:</span> <strong>${spid}</strong></div>
                    <div><span style="color:var(--text-muted, #6b7280);">Login:</span> ${window.escapeHtml(session.login_name || 'N/A')}</div>
                    <div><span style="color:var(--text-muted, #6b7280);">Host:</span> ${window.escapeHtml(session.host_name || 'N/A')}</div>
                    <div><span style="color:var(--text-muted, #6b7280);">Database:</span> ${window.escapeHtml(session.database_name || 'N/A')}</div>
                </div>
            </div>
            <div class="glass-panel" style="background:var(--bg-tertiary, #f9fafb); padding:1rem;"><h4 style="margin:0 0 0.75rem 0; color:var(--accent, #3b82f6);">State</h4>
                <div style="display:grid; gap:0.5rem; font-size:0.85rem;">
                    <div><span style="color:var(--text-muted, #6b7280);">Status:</span> <span style="background:${session.status==='suspended'?'#eab308':'#22c55e'}; color:#000; padding:0.1rem 0.4rem; border-radius:3px;">${window.escapeHtml(session.status || 'N/A')}</span></div>
                    <div><span style="color:var(--text-muted, #6b7280);">Duration:</span> ${((session.wait_time_ms||0)/1000).toFixed(2)}s</div>
                    <div><span style="color:var(--text-muted, #6b7280);">Blocking:</span> ${session.blocking_session_id?`<span style="background:#ef4444; color:#fff; padding:0.1rem 0.4rem; border-radius:3px;">${session.blocking_session_id}</span>`:'None'}</div>
                    <div><span style="color:var(--text-muted, #6b7280);">Wait:</span> ${window.escapeHtml(session.wait_type || 'N/A')}</div>
                </div>
            </div>
        </div>
        <div style="margin-bottom:1rem;"><h4 style="margin:0 0 0.75rem 0; color:var(--accent, #3b82f6);">SQL Text</h4>
            <div class="glass-panel" style="background:var(--bg-tertiary, #f9fafb); padding:1rem; max-height:200px; overflow:auto;"><pre style="margin:0; white-space:pre-wrap; color:var(--text-primary, #1f2937); font-family:monospace; font-size:0.85rem;">${window.escapeHtml(session.query_text || 'No query available')}</pre></div>
        </div>
        ${session.blocking_session_id ? `<div style="margin-top:1rem;"><button class="btn btn-sm btn-outline" style="color:var(--accent, #3b82f6);" data-action="navigate" data-route="drilldown-locks" data-also-call="closeSessionDetail"><i class="fa-solid fa-link"></i> Drill Down to Locks</button></div>` : ''}
    </div>`;
    document.body.appendChild(modalEl);
    modalEl.addEventListener('click', e => { if(e.target === modalEl) window.closeSessionDetail(); });
};
window.closeSessionDetail = function() { const m = document.getElementById('session-detail-modal'); if(m) m.remove(); };

function aggregateQueries(queries) {
    if (!queries || queries.length === 0) return [];
    const queryMap = new Map();
    queries.forEach(q => {
        const qt = q.query_text || q.Query_Text || '';
        if (!qt || qt.trim() === '' || qt === 'Unknown') return;
        const norm = qt.replace(/'[^']*'/g,"'?'").replace(/\d+(\.\d+)?/g,"?").replace(/\s+/g,' ').trim().substring(0,200);
        if (queryMap.has(norm)) { const e = queryMap.get(norm); e.cpu_time_ms = (e.cpu_time_ms||0)+(q.cpu_time_ms||0); e.exec_time_ms = Math.max(e.exec_time_ms||0,q.exec_time_ms||0); } 
        else queryMap.set(norm, {...q, query_text:qt, cpu_time_ms:q.cpu_time_ms||0, exec_time_ms:q.exec_time_ms||0});
    });
    return Array.from(queryMap.values()).sort((a,b) => (b.cpu_time_ms||0)-(a.cpu_time_ms||0));
}

window.showQueryModalDirect = function(queryText) {
    if (!queryText) queryText = 'No query available';
    const existing = document.getElementById('query-modal'); if(existing) existing.remove();
    const modal = document.createElement('div'); modal.id = 'query-modal';
    modal.style.cssText = 'display:flex; position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.8); align-items:center; justify-content:center;';
    modal.innerHTML = `<div style="background:var(--bg-surface); margin:2%; padding:20px; border:1px solid var(--border-color,#333); border-radius:12px; width:95%; max-width:1000px; max-height:90vh; overflow-y:auto; color:var(--text-primary,#e0e0e0);">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:1rem;">
            <h3 style="margin:0; color:var(--accent,#3b82f6);">Query Details</h3>
            <button data-action="close-id" data-target="query-modal" style="background:transparent; border:1px solid #555; color:#e0e0e0; font-size:1.25rem; cursor:pointer; padding:0.25rem 0.6rem; border-radius:4px;">&times;</button>
        </div>
        <div style="background:var(--bg-base); padding:1rem; border-radius:8px; max-height:60vh; overflow:auto; border:1px solid var(--border-color,#333);">
            <pre style="margin:0; white-space:pre-wrap; font-family:monospace; font-size:0.85rem;">${window.escapeHtml(queryText)}</pre>
        </div>
    </div>`;
    document.body.appendChild(modal);
    modal.addEventListener('click', e => { if(e.target === modal) modal.remove(); });
};

function renderDashboard(inst, metrics) {
    const dbFilter = window.appState.currentDatabase || 'all';
    const liveData = window.appState.liveMetrics || {};
    const v2 = window.appState.dashboardV2 || {};
    const risk = v2.health_risk || {};
    const workload = v2.workload_capacity || {};
    const blocking = (risk.blocking_sessions != null) ? Number(risk.blocking_sessions) : null;
    const allSessions = liveData.active_blocks || [];
    // Blocking_sessions from v2 health strip is the canonical value now.
    const connStats = { active: metrics.active_users || 0, blocked: (blocking != null ? blocking : 0) };
    
    let totalLocks = 0, totalDeadlocks = 0;
    if(dbFilter === 'all') { totalLocks = metrics.total_locks || 0; totalDeadlocks = metrics.deadlocks || 0; }
    else if(metrics.locks_by_db && metrics.locks_by_db[dbFilter]) { totalLocks = metrics.locks_by_db[dbFilter].total_locks; totalDeadlocks = metrics.locks_by_db[dbFilter].deadlocks; }

    let diskD = 0, diskL = 0;
    if(dbFilter === 'all') { diskD = ((metrics.disk_usage?.data_mb||0)/1024).toFixed(1); diskL = ((metrics.disk_usage?.log_mb||0)/1024).toFixed(1); }
    else if(metrics.disk_by_db && metrics.disk_by_db[dbFilter]) { diskD = ((metrics.disk_by_db[dbFilter].data_mb||0)/1024).toFixed(1); diskL = ((metrics.disk_by_db[dbFilter].log_mb||0)/1024).toFixed(1); }

    let tps = 0;
    const throughputStats = window.appState.timescaleMetrics.throughput || [];
    if(dbFilter === 'all') tps = throughputStats.reduce((s,i) => s + (i.avg_tps || i.tps || 0), 0);
    else { const dbStat = throughputStats.find(s => s.database_name === dbFilter); if(dbStat) tps = (dbStat.avg_tps || dbStat.tps || 0); }

    // Filter out unrealistic CPU values (>100 means it's raw tick count, not percentage)
    // Handle undefined/null properly - treat as "no data" (show loading)
    const rawCpu = metrics.avg_cpu_load;
    const avgCpuLoad = (rawCpu !== undefined && rawCpu !== null && rawCpu >= 0 && rawCpu <= 100) ? rawCpu : -1;
    const rawMemory = metrics.memory_usage;
    const memoryUsage = (rawMemory !== undefined && rawMemory !== null && rawMemory >= 0 && rawMemory <= 100) ? rawMemory : -1;
    const activeUsers = metrics.active_users !== undefined ? metrics.active_users : -1;
    const cpuIsDanger = avgCpuLoad > 90;
    const cpuIsWarning = avgCpuLoad > 60 && avgCpuLoad <= 90;
    const memIsDanger = memoryUsage > 95;
    const memIsWarning = memoryUsage > 85 && memoryUsage <= 95;
    const blockedIsDanger = (blocking != null ? blocking : 0) > 0;

    let sessList = allSessions;
    if(dbFilter !== 'all') sessList = sessList.filter(s => s.database_name === dbFilter);
    const significantSessions = sessList.filter(s => { const d = s.wait_time_ms||0; return ((s.status||'').toLowerCase()==='running' || s.blocking_session_id!==0) && d > 5000; });
    const sortedSessions = [...significantSessions].sort((a,b) => { if(a.blocking_session_id!==0 && b.blocking_session_id===0) return -1; if(a.blocking_session_id===0 && b.blocking_session_id!==0) return 1; return (b.wait_time_ms||0)-(a.wait_time_ms||0); });
    const sessionsHtml = sortedSessions.slice(0,25).map((s, idx) => renderSessionRow(s, idx)).join('');

    window.appState.queryCache = {};
    const longRunningRaw = window.appState.timescaleMetrics.longRunningQueries || [];
    const longRunningGrouped = groupLongRunningQueriesForDisplay(longRunningRaw, dbFilter);
    appDebug("Initial render: long running groups (scoped) =", longRunningGrouped.length);
    const displayLongRunning = longRunningGrouped.slice(0, 10).map((q, idx) => {
        window.appState.queryCache['lrq' + idx] = q.queryText || 'No query';
        const ts = q.captureTimestamp ? new Date(q.captureTimestamp) : null;
        const tsStr = ts ? ts.toLocaleTimeString('en-US', {hour: '2-digit', minute: '2-digit'}) : '';
        const ageMinutes = ts ? Math.floor((Date.now() - ts.getTime()) / 60000) : -1;
        const ageStr = ageMinutes >= 0 && ageMinutes < 60 ? `(${ageMinutes}m)` : (tsStr || '');
        const qt = (q.queryText || 'Unknown').substring(0, 40);
        const app = q.programName != null ? String(q.programName) : '';
        return `<tr><td><span class="badge badge-outline">${tsStr}</span><span class="text-muted" style="font-size:0.65rem; margin-left:4px;">${ageStr}</span></td><td><span class="code-snippet" style="cursor:pointer" data-action="show-query-modal-direct" data-key="lrq${idx}">${window.escapeHtml(qt)}</span></td><td>${window.escapeHtml(q.dbName)}</td><td>${window.escapeHtml(q.loginName)}</td><td title="${window.escapeHtml(app)}">${window.escapeHtml(app.length > 28 ? app.substring(0, 28) + '…' : app)}</td><td>${(q.cpuTimeMs || 0)}ms</td><td>${(q.totalElapsedMs || 0)}ms</td><td>${window.escapeHtml(q.waitType)}</td></tr>`;
    }).join('');
    const longRunningHtml = displayLongRunning;

    // Batch Requests/sec trend (1h) from v2
    const batchTrend = (window.appState.dashboardV2?.root_cause?.batch_requests_trend_1h) || [];

    // Query Store Top Offenders — dashboard uses fixed 1h snapshot; change range in drilldown
    const qsOffenders = window.appState.queryStoreBottlenecks || [];
    const toGridSort = window.appState.topOffendersGridSort || { key: 'total_cpu_ms', dir: 'desc' };
    const qsSortedForGrid = sortQueryStoreOffenderRows(qsOffenders, toGridSort);
    const offendersHtml = (qsSortedForGrid && qsSortedForGrid.length > 0)
        ? renderTopOffendersRowsHtml(qsSortedForGrid)
        : '';

    const overall = computeOverallStatus(metrics);
    const overallCls = overall === 'CRITICAL' ? 'badge badge-danger' : (overall === 'WARNING' ? 'badge badge-warning' : 'badge badge-success');

    // Phase 2: strip tiles (DBA homepage top rows)
    const hs = risk.health_score || {};
    const hsScore = Number.isFinite(hs.score) ? hs.score : null;
    const hsSeverity = (hs.severity || '').toString().toUpperCase();
    const hsBadgeCls = hsSeverity === 'CRITICAL' ? 'badge badge-danger' : (hsSeverity === 'WARNING' ? 'badge badge-warning' : 'badge badge-success');
    const hsLabel = hs.label || (hsSeverity === 'CRITICAL' ? 'Critical' : (hsSeverity === 'WARNING' ? 'Warning' : 'Healthy'));

    const memGrants = (risk.memory_grants_pending != null) ? Number(risk.memory_grants_pending) : null;
    const failedLogins5m = (risk.failed_logins_5m != null) ? Number(risk.failed_logins_5m) : null;
    const tempdbPct = (risk.tempdb_used_percent != null) ? Number(risk.tempdb_used_percent) : null;
    const logPct = (risk.max_log_used_percent != null) ? Number(risk.max_log_used_percent) : null;
    const logDb = (risk.max_log_db_name != null) ? String(risk.max_log_db_name) : '';

    const batch = (workload.batch_requests_per_sec != null) ? Number(workload.batch_requests_per_sec) : null;
    const comp = (workload.compilations_per_sec != null) ? Number(workload.compilations_per_sec) : null;
    const compRatio = (workload.compilation_ratio != null) ? Number(workload.compilation_ratio) : null;
    const compSev = (workload.compilation_severity || '').toString().toUpperCase();

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1>SQL Server Dashboard</h1>
                    <span class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Database: <span class="text-accent">${window.escapeHtml(dbFilter)}</span></span>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:1rem;">
                    <div style="display:flex; gap:0.4rem; align-items:center; flex-wrap:wrap; justify-content:flex-end;">
                        <span class="${overallCls}" style="font-size:0.65rem;">Health: ${overall}</span>
                        <span id="dataSourceBadge" class="badge badge-info" style="font-size:0.65rem; display:none;">Source</span>
                    </div>
                    <div style="text-align:right;">
                        <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="lastRefreshTime">--:--:--</span></span>
                        <span class="text-muted" style="font-size:0.65rem; display:block;">Auto-refresh: every 10s</span>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshDashboardData"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            <!--
            <div class="glass-panel mt-2" style="padding:0.55rem 0.75rem; border-left:3px solid var(--accent-blue,#3b82f6);">
                <div style="font-size:0.78rem; color:var(--text-muted); line-height:1.45;">
                    <strong style="color:var(--text);">Related collectors:</strong>
                    <strong>Storage &amp; Index Health</strong> (separate page) — index samples ~every <strong>15 min</strong>; logs <code style="font-size:0.72rem;">[Collector][SIH]</code>; Timescale required; <code style="font-size:0.72rem;">Instances[].databases</code> optional (auto-discovers user DBs if empty).
                    <strong>Disk I/O Latency</strong> below — Timescale trend from file stats; this dashboard load triggers a snapshot write when Timescale is connected (plus the Enterprise metrics collector, ~2 min, <code style="font-size:0.72rem;">ENTERPRISE_METRICS_INTERVAL_SEC</code>). See chart <i class="fa-solid fa-circle-info"></i> tooltip.
                </div>
            </div>
            -->
            <div id="sql-dashboard-alert-banner" style="display:none;"></div>
            <div class="top-strips dashboard-top-strips" style="grid-template-columns: 1fr 1fr;">
                <div class="glass-panel dashboard-strip-panel">
                    <div class="dashboard-strip-header">
                        <h4>
                            <span class="dashboard-strip-header-icons" aria-hidden="true">
                                <i class="fa-solid fa-microchip" title="CPU / compute"></i>
                                <i class="fa-solid fa-server" title="Host / hardware"></i>
                                <i class="fa-solid fa-database" title="SQL Engine"></i>
                            </span>
                            Host, Hardware, Engine, &amp; Workload
                            <span class="dashboard-strip-header-tooltip" title="Shows CPU, memory, disk, TPS, active sessions and blocking activity sourced from the monitored SQL Server instance."><i class="fa-solid fa-circle-info"></i></span>
                        </h4>
                    </div>
                    <div class="dashboard-strip-metrics-row--6">
                        <div class="strip-metric-cell ${cpuIsDanger ? 'strip-metric-cell--accent-bad' : (cpuIsWarning ? 'strip-metric-cell--accent-warn' : '')}">
                            <div class="strip-metric-label">CPU</div>
                            <div class="strip-metric-value"><span id="metric-cpu" class="${cpuIsDanger ? 'text-danger fw-bold' : (cpuIsWarning ? 'text-warning fw-bold' : '')}">${avgCpuLoad >= 0 ? avgCpuLoad.toFixed(1) + '%' : ''}</span>${avgCpuLoad < 0 ? '<div class="metric-loading" style="font-size:0.75rem;"><div class="spinner"></div><span>Loading...</span></div>' : ''}</div>
                        </div>
                        <div class="strip-metric-cell ${memIsDanger ? 'strip-metric-cell--accent-bad' : (memIsWarning ? 'strip-metric-cell--accent-warn' : '')}" style="cursor:pointer;" data-action="navigate" data-route="drilldown-memory" title="Open Memory Drilldown (Timescale range)">
                            <div class="strip-metric-label">Memory</div>
                            <div class="strip-metric-value"><span id="metric-memory" class="${memIsDanger ? 'text-danger fw-bold' : (memIsWarning ? 'text-warning fw-bold' : '')}">${memoryUsage >= 0 ? memoryUsage.toFixed(1) + '%' : ''}</span>${memoryUsage < 0 ? '<div class="metric-loading" style="font-size:0.75rem;"><div class="spinner"></div><span>Loading...</span></div>' : ''}</div>
                        </div>
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">Disk GB D/L</div>
                            <div class="strip-metric-value">${diskD > 0 || diskL > 0 ? diskD + '/' + diskL : '<div class="metric-loading" style="font-size:0.75rem;"><div class="spinner"></div><span>Loading...</span></div>'}</div>
                        </div>
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">TPS</div>
                            <div class="strip-metric-value">${tps > 0 ? tps.toFixed(1) : 'N/A'}</div>
                        </div>
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">Active</div>
                            <div class="strip-metric-value" style="color:var(--success,#22c55e);"><span id="metric-active">${activeUsers >= 0 ? activeUsers : '<div class="metric-loading" style="font-size:0.75rem;"><div class="spinner"></div><span>Loading...</span></div>'}</span></div>
                        </div>
                        <div class="strip-metric-cell ${blockedIsDanger ? 'strip-metric-cell--accent-warn' : ''}">
                            <div class="strip-metric-label" title="Count of active blocked sessions from live blocking activity, not total lock count." style="display:flex; justify-content:space-between; align-items:center; gap:0.35rem;">Blocked${(blocking != null && blocking > 0) ? '<a data-action="navigate" data-route="drilldown-locks" style="font-size:0.65rem; color:var(--accent);" title="Drill down to Locks"><i class="fa-solid fa-arrow-right"></i></a>' : ''}</div>
                            <div class="strip-metric-value"><span id="metric-blocked">${blocking != null ? blocking : 0}</span></div>
                        </div>
                    </div>
                </div>

                <div class="glass-panel dashboard-strip-panel">
                    <div class="dashboard-strip-header">
                        <h4>
                            <i class="fa-solid fa-heart-pulse text-accent"></i>
                            Health & Risk (Snapshot)
                            <span class="dashboard-strip-header-tooltip" title="Snapshot risk signals for SQL Server: blocking sessions, PLE, memory grants, login failures, TempDB and log pressure."><i class="fa-solid fa-circle-info"></i></span>
                            <span class="dashboard-strip-header-meta ${hsBadgeCls}">Health Score: ${hsScore != null ? hsScore : '--'} • ${window.escapeHtml(hsLabel)}</span>
                        </h4>
                    </div>
                    <div class="dashboard-strip-metrics-row--6">
                        <div class="strip-metric-cell ${memGrants != null && memGrants > 0 ? 'strip-metric-cell--accent-warn' : ''}">
                            <div class="strip-metric-label">Mem Grants Pending</div>
                            <div class="strip-metric-value">${memGrants != null ? memGrants : '--'}</div>
                        </div>
                        <div class="strip-metric-cell ${failedLogins5m != null && failedLogins5m > 0 ? 'strip-metric-cell--accent-warn' : ''}">
                            <div class="strip-metric-label">Failed Logins (5m)</div>
                            <div class="strip-metric-value">${failedLogins5m != null ? failedLogins5m : '--'}</div>
                        </div>
                        <div class="strip-metric-cell ${tempdbPct != null && tempdbPct > 80 ? 'strip-metric-cell--accent-bad' : (tempdbPct != null && tempdbPct > 60 ? 'strip-metric-cell--accent-warn' : '')}">
                            <div class="strip-metric-label">TempDB Used</div>
                            <div class="strip-metric-value">${tempdbPct != null ? tempdbPct.toFixed(0) + '%' : '--'}</div>
                        </div>
                        <div class="strip-metric-cell ${logPct != null && logPct > 80 ? 'strip-metric-cell--accent-bad' : (logPct != null && logPct > 60 ? 'strip-metric-cell--accent-warn' : '')}">
                            <div class="strip-metric-label">Max Log Used</div>
                            <div class="strip-metric-value">${logPct != null ? logPct.toFixed(0) + '%' : '--'}</div>
                            <div class="text-muted sub" title="${window.escapeHtml(logDb)}">${window.escapeHtml(logDb || '')}</div>
                        </div>
                        <div class="strip-metric-cell" style="cursor:pointer;" data-action="navigate" data-route="drilldown-memory" title="Open Memory Drilldown (PLE history)">
                            <div class="strip-metric-label">PLE</div>
                            <div class="strip-metric-value">${risk.ple != null ? Number(risk.ple).toFixed(0) : '--'}</div>
                        </div>
                        <div class="strip-metric-cell ${compSev === 'WARNING' ? 'strip-metric-cell--accent-warn' : ''}">
                            <div class="strip-metric-label">Compilations Ratio</div>
                            <div class="strip-metric-value">${compRatio != null ? (compRatio * 100).toFixed(1) + '%' : '--'}</div>
                            <div class="text-muted sub">${batch != null ? batch.toFixed(0) : '--'} batch/s · ${comp != null ? comp.toFixed(0) : '--'} comp/s</div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="charts-grid dashboard-charts-row mt-3" data-cols="3">
                <div class="chart-card glass-panel" style="padding:0.6rem; cursor:pointer;" data-action="navigate" data-route="drilldown-cpu" title="Drilldown into System Resources"><div class="card-header"><h3 style="font-size:0.82rem; margin:0;">System Resources</h3></div><div class="chart-container"><canvas id="dashResourcesChart"></canvas></div></div>
                <div class="chart-card glass-panel" style="padding:0.6rem;"><div class="card-header"><h3 style="font-size:0.82rem; margin:0;">Batch Requests/sec (1h)</h3></div><div class="chart-container"><canvas id="dashBatchChart"></canvas></div></div>
                <div class="chart-card glass-panel" style="padding:0.6rem;"><div class="card-header"><h3 style="font-size:0.82rem; margin:0;">Disk I/O Latency <span class="dashboard-strip-header-tooltip" title="Trend uses Timescale (sqlserver_file_io_latency). Each dashboard load records one snapshot when Timescale is connected; the Enterprise metrics collector also writes on a timer (default ~2 min, ENTERPRISE_METRICS_INTERVAL_SEC). Check logs for WarmFileIOLatency / EnterpriseMetricsCollector / insert errors. Latency can read as 0 ms with no I/O or very fast storage."><i class="fa-solid fa-circle-info"></i></span></h3></div><div class="chart-container"><canvas id="dashIoChart"></canvas></div></div>
            </div>
            <div class="charts-grid dashboard-charts-row mt-2" data-cols="3">
                <div class="chart-card glass-panel" style="padding:0.6rem; cursor:pointer;" data-action="navigate" data-route="drilldown-memory" title="Memory drilldown (PLE &amp; Timescale)"><div class="card-header"><h3 style="font-size:0.82rem; margin:0;">Page Life Expectancy</h3></div><div class="chart-container"><canvas id="dashPleChart"></canvas></div></div>
                <div class="chart-card glass-panel" style="padding:0.6rem;"><div class="card-header"><h3 style="font-size:0.82rem; margin:0;">Buffer Cache Hit % (1h)</h3></div><div class="chart-container"><canvas id="dashBchrChart"></canvas></div></div>
                <div class="chart-card glass-panel" style="padding:0.6rem;"><div class="card-header"><h3 style="font-size:0.82rem; margin:0;">Wait Categories (15m)</h3></div><div class="chart-container"><canvas id="dashWaitCategoriesDonut"></canvas></div></div>
            </div>
            <div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                <div class="table-card glass-panel">
                    <div class="card-header">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-bolt text-accent"></i> Top Offenders (Query Store) <span style="font-size:0.6rem; font-weight:normal; color:var(--text-muted);">${dbFilter === 'all' ? '· all user DBs' : '· ' + window.escapeHtml(dbFilter)}</span></h3>
                        <div style="display:flex; gap:0.5rem; align-items:center; flex-wrap:wrap; justify-content:flex-end;">
                            <span class="text-muted" style="font-size:0.65rem;">1h snapshot · click column headers to sort</span>
                            <span class="badge badge-info">${qsOffenders.length}</span>
                            <a data-action="navigate" data-route="drilldown-bottlenecks" class="btn btn-xs btn-outline text-accent" style="font-size:0.7rem;" title="Open full Query Store view (time range &amp; details)"><i class="fa-solid fa-arrow-right"></i> Drill down</a>
                        </div>
                    </div>
                    <div class="table-responsive" style="max-height:220px; overflow-y:auto;">
                        <table class="data-table" id="topOffendersGrid" style="font-size:0.7rem;">
                            <thead><tr>
                                <th>#</th>
                                <th data-sort-key="database_name" title="Sort" style="cursor:pointer;user-select:none;white-space:nowrap;">database <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.6rem;"></i></th>
                                <th data-sort-key="query_text" title="Sort" style="cursor:pointer;user-select:none;white-space:nowrap;">sql_text <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.6rem;"></i></th>
                                <th data-sort-key="execution_count" title="Sort" style="cursor:pointer;user-select:none;white-space:nowrap;">exec <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.6rem;"></i></th>
                                <th data-sort-key="avg_cpu_ms" title="Sort" style="cursor:pointer;user-select:none;white-space:nowrap;">avg_cpu(ms) <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.6rem;"></i></th>
                                <th data-sort-key="avg_duration_ms" title="Sort" style="cursor:pointer;user-select:none;white-space:nowrap;">avg_dur(ms) <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.6rem;"></i></th>
                                <th data-sort-key="avg_logical_reads" title="Sort" style="cursor:pointer;user-select:none;white-space:nowrap;">avg_reads <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.6rem;"></i></th>
                                <th data-sort-key="total_cpu_ms" title="Sort" style="cursor:pointer;user-select:none;white-space:nowrap;">total_cpu(ms) <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.6rem;"></i></th>
                            </tr></thead>
                            <tbody id="top-offenders-body">${offendersHtml || '<tr><td colspan="8" class="text-center text-muted">No Query Store offenders yet (Query Store may be disabled or not collected).</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>
            </div>
            <div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                <div class="table-card glass-panel">
                    <div class="card-header">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-clock text-accent"></i> Long Running Queries <span style="font-size:0.65rem; font-weight:normal; color:var(--text-muted);">(last 15 min)</span> <span style="font-size:0.6rem; font-weight:normal; color:var(--text-muted);">${dbFilter === 'all' ? '· all user DBs' : '· ' + window.escapeHtml(dbFilter)}</span></h3>
                        <span class="badge badge-warning" id="long-running-badge">${longRunningGrouped.length}</span>
                    </div>
                    <div class="table-responsive" style="max-height:220px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;"><thead><tr><th>time</th><th>sql_text</th><th>database</th><th>login</th><th>app</th><th>cpu(ms)</th><th>duration(ms)</th><th>wait</th></tr></thead>
                        <tbody id="long-running-body">${longRunningHtml || '<tr><td colspan="8" class="text-center text-muted">No long running queries in the current time range.</td></tr>'}</tbody></table>
                    </div>
                </div>
            </div>
            <div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Active Sessions (&gt;5s or Blocking)</h3><div style="display:flex; gap:0.5rem; align-items:center;"><span class="badge badge-${connStats.blocked>0?'danger':'info'}" id="active-sessions-badge">${significantSessions.length}</span><a data-action="navigate" data-route="drilldown-locks" style="font-size:0.7rem; color:var(--accent);" title="Drill down to Locks"><i class="fa-solid fa-arrow-right"></i></a></div></div>
                    <div class="table-responsive" style="max-height:280px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;"><thead><tr><th>spid</th><th>state</th><th>blocker</th><th>duration</th><th>status</th><th>wait_type</th><th>database</th><th>login</th><th>sql_text</th></tr></thead>
                        <tbody id="active-sessions-body">${sessionsHtml || '<tr><td colspan="9" class="text-center text-muted">No significant sessions.</td></tr>'}</tbody></table>
                    </div>
                </div>
            </div>
        </div>`;
    updateLastRefreshTime();
    bindTopOffendersControls();
    setTimeout(() => initDashboardCharts(metrics), 25);
}

function initDashboardCharts(metrics) {
    if(window.currentCharts && window.currentCharts.dashRes) window.currentCharts.dashRes.destroy();
    if(window.currentCharts && window.currentCharts.dashWait) window.currentCharts.dashWait.destroy();
    if(window.currentCharts && window.currentCharts.dashPle) window.currentCharts.dashPle.destroy();
    if(window.currentCharts && window.currentCharts.dashIo) window.currentCharts.dashIo.destroy();
    if(window.currentCharts && window.currentCharts.dashBatch) window.currentCharts.dashBatch.destroy();
    if(window.currentCharts && window.currentCharts.dashBchr) window.currentCharts.dashBchr.destroy();
    window.currentCharts = window.currentCharts || {};
    
    let cpuHist = metrics.cpu_history || [];
    cpuHist = cpuHist.filter(t => t.sql_process >= 0 && t.sql_process <= 100);
    cpuHist.sort((a, b) => {
        const ta = a.event_time ? new Date(a.event_time.replace(' ','T')).getTime() : 0;
        const tb = b.event_time ? new Date(b.event_time.replace(' ','T')).getTime() : 0;
        return ta - tb;
    });
    if(cpuHist.length > 60) cpuHist = cpuHist.slice(-60);
    const sqlArr = cpuHist.map(t => t.sql_process);
    const idleArr = cpuHist.map(t => t.system_idle);
    const cAxis = cpuHist.map(t => { if(!t.event_time) return ''; const parts = t.event_time.split(' '); if(parts.length < 2) return ''; return parts[1].substring(0,5); });

    const hasCpuData = cpuHist.length > 0 && sqlArr.some(v => v >= 0);
    const resChartContainer = document.getElementById('dashResourcesChart')?.parentElement;
    if (resChartContainer) {
        const existingMsg = resChartContainer.querySelector('.chart-message');
        if (existingMsg) existingMsg.remove();
        if (!hasCpuData) {
            resChartContainer.innerHTML += '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;"><div class="spinner" style="width:20px;height:20px;margin:0 auto 8px;"></div>Loading...</div>';
        }
    }
    const ctx = document.getElementById('dashResourcesChart')?.getContext('2d');
    if(ctx && hasCpuData) {
        const gradCPU = ctx.createLinearGradient(0,0,0,300);
        gradCPU.addColorStop(0,'rgba(59,130,246,0.4)');
        gradCPU.addColorStop(1,'rgba(59,130,246,0)');
        
        const chartData = {
            labels: cAxis,
            datasets: [
                {
                    label: 'SQL CPU',
                    data: sqlArr,
                    borderColor: window.getCSSVar('--accent-blue'),
                    backgroundColor: gradCPU,
                    fill: true,
                    tension: 0.4,
                    pointRadius: 0
                },
                {
                    label: 'System Idle',
                    data: idleArr,
                    borderColor: window.getCSSVar('--success'),
                    fill: false,
                    tension: 0.4,
                    pointRadius: 0
                }
            ]
        };
        
        const chartOptions = {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    position: 'top',
                    labels: { boxWidth: 8, font: { size: 10 } }
                }
            },
            scales: {
                y: {
                    max: 100,
                    min: 0,
                    ticks: { callback: function(v) { return v + '%'; } }
                },
                x: {
                    grid: { display: true, color: 'rgba(255,255,255,0.05)' },
                    ticks: { maxTicksLimit: 12 }
                }
            }
        };
        
        window.currentCharts.dashRes = new Chart(ctx, { type: 'line', data: chartData, options: chartOptions });
    }
    window.updatePleChart();
    window.updateIoChart();
    window.updateWaitChart();
    window.updateBatchChart();
    window.updateBchrChart();
}

window.updateBchrChart = function() {
    if(window.currentCharts && window.currentCharts.dashBchr) window.currentCharts.dashBchr.destroy();
    const trend = window.appState.dashboardV2?.memory_storage_internals?.buffer_cache_hit_trend_1h || [];

    const container = document.getElementById('dashBchrChart')?.parentElement;
    if (!container) return;
    const existingMsg = container.querySelector('.chart-message');
    if (existingMsg) existingMsg.remove();

    const bchrRefreshing = !!window.appState.metricsRefreshInProgress;
    if (!Array.isArray(trend) || trend.length === 0) {
        container.innerHTML += bchrRefreshing
            ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
            : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">Waiting for data collection...</div>';
        return;
    }

    const pts = trend.map(p => ({
        ts: p.timestamp ? new Date(p.timestamp) : null,
        v: Number(p.buffer_cache_hit_ratio || 0)
    })).filter(p => p.ts && isFinite(p.v));

    if (pts.length === 0) {
        container.innerHTML += bchrRefreshing
            ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
            : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">No cache data detected</div>';
        return;
    }

    const labels = pts.map(p => `${p.ts.getHours().toString().padStart(2,'0')}:${p.ts.getMinutes().toString().padStart(2,'0')}`);
    const values = pts.map(p => p.v);

    const ctx = document.getElementById('dashBchrChart')?.getContext('2d');
    if (!ctx) return;

    window.currentCharts = window.currentCharts || {};
    window.currentCharts.dashBchr = new Chart(ctx, {
        type: 'line',
        data: {
            labels,
            datasets: [{
                label: 'BCHR %',
                data: values,
                borderColor: window.getCSSVar('--success'),
                tension: 0.35,
                pointRadius: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: false, suggestedMin: 0, suggestedMax: 100, ticks: { callback: (v) => v + '%' } },
                x: { grid: { display: false }, ticks: { maxTicksLimit: 8 } }
            }
        }
    });
};

window.updateBatchChart = function() {
    if(window.currentCharts && window.currentCharts.dashBatch) window.currentCharts.dashBatch.destroy();
    const trend = window.appState.dashboardV2?.root_cause?.batch_requests_trend_1h || [];

    const container = document.getElementById('dashBatchChart')?.parentElement;
    if (!container) return;
    const existingMsg = container.querySelector('.chart-message');
    if (existingMsg) existingMsg.remove();

    const batchRefreshing = !!window.appState.metricsRefreshInProgress;
    if (!Array.isArray(trend) || trend.length === 0) {
        container.innerHTML += batchRefreshing
            ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
            : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">Waiting for data collection...</div>';
        return;
    }

    const pts = trend.map(p => ({
        ts: p.timestamp ? new Date(p.timestamp) : null,
        v: Number(p.batch_requests_per_sec || 0)
    })).filter(p => p.ts && isFinite(p.v));

    if (pts.length === 0) {
        container.innerHTML += batchRefreshing
            ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
            : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">No workload activity detected</div>';
        return;
    }

    const labels = pts.map(p => `${p.ts.getHours().toString().padStart(2,'0')}:${p.ts.getMinutes().toString().padStart(2,'0')}`);
    const values = pts.map(p => p.v);

    const ctx = document.getElementById('dashBatchChart')?.getContext('2d');
    if (!ctx) return;

    window.currentCharts = window.currentCharts || {};
    window.currentCharts.dashBatch = new Chart(ctx, {
        type: 'line',
        data: {
            labels,
            datasets: [{
                label: 'Batch Requests/sec',
                data: values,
                borderColor: window.getCSSVar('--accent-blue'),
                tension: 0.35,
                pointRadius: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, title: { display: true, text: '/sec' } },
                x: { grid: { display: false }, ticks: { maxTicksLimit: 8 } }
            }
        }
    });
};

window.updateIoChart = function() {
    if(window.currentCharts && window.currentCharts.dashIo) window.currentCharts.dashIo.destroy();
    let fHist = window.appState.fileHistory || [];

    const ioSnapFiles = (snap) => {
        if (!snap || typeof snap !== 'object') return [];
        if (Array.isArray(snap.files) && snap.files.length > 0) return snap.files;
        if (snap.read_latency_ms != null || snap.write_latency_ms != null) {
            return [{ read_latency_ms: snap.read_latency_ms, write_latency_ms: snap.write_latency_ms }];
        }
        return [];
    };
    const ioSnapTime = (snap) => (snap && (snap.timestamp || snap.capture_time)) || '';

    appDebug('[Dashboard] IO Chart - fileHistory:', JSON.stringify(fHist).substring(0, 500));
    appDebug('[Dashboard] IO Chart - fileHistory length:', fHist.length, 'type:', fHist.length > 0 ? typeof fHist[0] : 'empty', 'first:', fHist.length > 0 ? JSON.stringify(fHist[0]).substring(0, 200) : 'empty');

    let hasData = false;
    let fData = [];
    let labels = [];

    if (fHist.length > 0) {
        if (typeof fHist[0] === 'number') {
            hasData = fHist.some(v => v > 0);
            if (hasData) {
                fData = fHist.slice(-60);
                labels = fData.map((_, i) => '-' + (fData.length - 1 - i) + 'm');
            }
        } else {
            for (let i = 0; i < fHist.length; i++) {
                const snap = fHist[i];
                const files = ioSnapFiles(snap);
                for (let j = 0; j < files.length; j++) {
                    const f = files[j];
                    const r = Number(f.read_latency_ms);
                    const w = Number(f.write_latency_ms);
                    if (Number.isFinite(r) || Number.isFinite(w)) {
                        hasData = true;
                        break;
                    }
                }
                if (hasData) break;
            }
            if (hasData) {
                if (fHist.length > 60) fHist = fHist.slice(-60);
                fData = fHist;
                labels = fHist.map(snap => {
                    const t = ioSnapTime(snap);
                    if (t) {
                        const d = new Date(String(t).replace(' ', 'T'));
                        if (!isNaN(d.getTime())) {
                            return `${d.getHours().toString().padStart(2,'0')}:${d.getMinutes().toString().padStart(2,'0')}`;
                        }
                    }
                    return '-';
                });
            }
        }
    }
    
    const ioChartContainer = document.getElementById('dashIoChart')?.parentElement;
    const ioRefreshing = !!window.appState.metricsRefreshInProgress;
    if (ioChartContainer) {
        const existingMsg = ioChartContainer.querySelector('.chart-message');
        if (existingMsg) existingMsg.remove();
        // Show message based on data state
        if (fHist.length === 0) {
            ioChartContainer.innerHTML += ioRefreshing
                ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
                : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">Waiting for data collection...</div>';
        } else if (!hasData) {
            ioChartContainer.innerHTML += ioRefreshing
                ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
                : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">No I/O activity detected</div>';
        }
    }
    
    if (!hasData) return;
    
    const rLat = fData.map(snap => {
        if (typeof snap === 'number') return snap;
        const files = ioSnapFiles(snap);
        let r = 0, c = 0;
        files.forEach(f => { r += Number(f.read_latency_ms) || 0; c++; });
        return c > 0 ? r / c : 0;
    });
    const wLat = fData.map(snap => {
        if (typeof snap === 'number') return 0;
        const files = ioSnapFiles(snap);
        let w = 0, c = 0;
        files.forEach(f => { w += Number(f.write_latency_ms) || 0; c++; });
        return c > 0 ? w / c : 0;
    });
    const ctxIo = document.getElementById('dashIoChart')?.getContext('2d');
    if(ctxIo) {
        const ioChartData = {
            labels: labels,
            datasets: [
                { label: 'Read Latency', data: rLat, borderColor: window.getCSSVar('--accent-blue'), tension: 0.3, pointRadius: 0 },
                { label: 'Write Latency', data: wLat, borderColor: window.getCSSVar('--warning'), tension: 0.3, pointRadius: 0 }
            ]
        };
        const ioChartOptions = {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, title: { display: true, text: 'ms' } },
                x: { grid: { display: false }, ticks: { maxTicksLimit: 8 } }
            }
        };
        window.currentCharts.dashIo = new Chart(ctxIo, { type: 'line', data: ioChartData, options: ioChartOptions });
    }
};

window.updatePleChart = function() {
    if(window.currentCharts && window.currentCharts.dashPle) window.currentCharts.dashPle.destroy();
    const pleHist = window.appState.pleHistory || [];
    
    // Handle both array of numbers and array of objects
    let hasData = false;
    let pleData = [];
    let labels = [];
    
    if (pleHist.length > 0) {
        // Check if it's an array of numbers or objects
        if (typeof pleHist[0] === 'number') {
            // Format: [123, 124, 125, ...] - just numbers
            hasData = pleHist.some(v => v > 0);
            if (hasData) {
                pleData = pleHist.slice(-60);
                labels = pleData.map((_, i) => '-' + (pleData.length - 1 - i) + 'm');
            }
        } else if (typeof pleHist[0] === 'object') {
            // Format: [{ple: 123, timestamp: ...}, ...]
            hasData = pleHist.some(v => (v.ple || v.avg_ple || 0) > 0);
            if (hasData) {
                const sorted = [...pleHist].sort((a, b) => {
                    const ta = a.timestamp ? new Date(a.timestamp).getTime() : 0;
                    const tb = b.timestamp ? new Date(b.timestamp).getTime() : 0;
                    return ta - tb;
                }).slice(-60);
                pleData = sorted.map(t => t.ple || t.avg_ple || 0);
                labels = sorted.map(t => {
                    if(!t.timestamp) return '';
                    const parts = t.timestamp.split(' ');
                    if(parts.length < 2) return '';
                    return parts[1].substring(0,5);
                });
            }
        }
    }
    
    const pleChartContainer = document.getElementById('dashPleChart')?.parentElement;
    const pleRefreshing = !!window.appState.metricsRefreshInProgress;
    if (pleChartContainer) {
        const existingMsg = pleChartContainer.querySelector('.chart-message');
        if (existingMsg) existingMsg.remove();
        if (pleHist.length === 0) {
            pleChartContainer.innerHTML += pleRefreshing
                ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
                : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">Waiting for data collection...</div>';
        } else if (!hasData) {
            pleChartContainer.innerHTML += pleRefreshing
                ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
                : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">Collecting data...</div>';
        }
    }
    
    if (!hasData) return;
    const maxServerMemoryGB = window.appState.liveMetrics?.max_server_memory_gb || 8192;
    const pleThreshold = (maxServerMemoryGB / 4) * 300;
    const ctxPle = document.getElementById('dashPleChart')?.getContext('2d');
    if(ctxPle) {
        const pleChartData = {
            labels: labels,
            datasets: [{
                label: 'PLE (seconds)',
                data: pleData,
                borderColor: window.getCSSVar('--success'),
                fill: true,
                backgroundColor: 'rgba(16,185,129,0.2)',
                tension: 0.3,
                pointRadius: 0
            }]
        };
        const pleChartOptions = {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { 
                legend: { display: false },
                annotation: {
                    annotations: {
                        threshold: {
                            type: 'line',
                            yMin: pleThreshold,
                            yMax: pleThreshold,
                            borderColor: 'rgba(239, 68, 68, 0.7)',
                            borderWidth: 2,
                            borderDash: [6, 6],
                            label: {
                                display: true,
                                content: 'Threshold: ' + pleThreshold.toFixed(0) + 's',
                                position: 'end',
                                backgroundColor: 'rgba(239, 68, 68, 0.8)',
                                color: 'white',
                                font: { size: 10 }
                            }
                        }
                    }
                }
            },
            scales: {
                y: { 
                    beginAtZero: true,
                    title: { display: true, text: 'Seconds', color: 'var(--text-muted)', font: { size: 10 } }
                },
                x: { grid: { display: false }, ticks: { maxTicksLimit: 8 } }
            }
        };
        window.currentCharts.dashPle = new Chart(ctxPle, { type: 'line', data: pleChartData, options: pleChartOptions });
    }
};

// Deprecated: old wait-history line chart. Kept to avoid breaking any external calls,
// but the UI now renders "Wait Categories (15m)" donut from /v2 aggregation.
window.updateWaitChart = function() {
    if (window.updateWaitCategoriesDonut) {
        window.updateWaitCategoriesDonut();
    }
};

// Phase 2: Wait Categories donut (last 15 minutes) from /api/mssql/dashboard/v2 payload.
window.updateWaitCategoriesDonut = function() {
    if (window.currentCharts && window.currentCharts.waitCategoriesDonut) {
        window.currentCharts.waitCategoriesDonut.destroy();
    }

    const v2 = window.appState.dashboardV2 || {};
    const agg = v2?.root_cause?.wait_categories_15m || [];

    const container = document.getElementById('dashWaitCategoriesDonut')?.parentElement;
    if (!container) return;

    // Remove any existing message
    const existingMsg = container.querySelector('.chart-message');
    if (existingMsg) existingMsg.remove();

    const waitRefreshing = !!window.appState.metricsRefreshInProgress;
    if (!Array.isArray(agg) || agg.length === 0) {
        container.innerHTML += waitRefreshing
            ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
            : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">Waiting for data collection...</div>';
        return;
    }

    const labels = agg.map(r => r.wait_category || 'Other');
    const values = agg.map(r => Number(r.wait_time_ms || 0));
    const total = values.reduce((s, v) => s + (isFinite(v) ? v : 0), 0);
    if (!isFinite(total) || total <= 0) {
        container.innerHTML += waitRefreshing
            ? '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;text-align:center;"><div class="spinner" style="width:18px;height:18px;margin:0 auto 6px;"></div>Loading metrics…</div>'
            : '<div class="chart-message" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);font-size:0.8rem;">No wait activity detected</div>';
        return;
    }

    const ctx = document.getElementById('dashWaitCategoriesDonut')?.getContext('2d');
    if (!ctx) return;

    const palette = {
        CPU: window.getCSSVar('--warning') || '#f59e0b',
        IO: window.getCSSVar('--accent-blue') || '#3b82f6',
        Log: window.getCSSVar('--danger') || '#ef4444',
        Locking: window.getCSSVar('--danger') || '#ef4444',
        Memory: window.getCSSVar('--warning') || '#f59e0b',
        Network: window.getCSSVar('--text-muted') || '#94a3b8',
        Other: window.getCSSVar('--text-muted') || '#94a3b8'
    };
    const bg = labels.map(l => palette[l] || palette.Other);

    window.currentCharts = window.currentCharts || {};
    window.currentCharts.waitCategoriesDonut = new Chart(ctx, {
        type: 'doughnut',
        data: {
            labels: labels,
            datasets: [{
                data: values,
                backgroundColor: bg,
                borderColor: 'rgba(255,255,255,0.08)',
                borderWidth: 1
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { position: 'right', labels: { boxWidth: 10, font: { size: 9 } } },
                tooltip: {
                    callbacks: {
                        label: function(context) {
                            const v = context.parsed || 0;
                            const pct = total > 0 ? ((v / total) * 100).toFixed(1) : '0.0';
                            return `${context.label}: ${Math.round(v)} ms (${pct}%)`;
                        }
                    }
                }
            },
            cutout: '55%'
        }
    });
};
