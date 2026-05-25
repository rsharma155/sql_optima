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
    console.warn("[Dashboard] DashboardView is deprecated. Redirecting to SqlServerHealthV2View.");
    if (typeof window.SqlServerHealthV2View === 'function') {
        return window.SqlServerHealthV2View();
    }
    // Fallback if V2 is not yet loaded
    setTimeout(() => window.DashboardView(), 200);
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

/**
 * Common zoom/pan options for Chart.js
 */
function getChartZoomOptions() {
    return {
        zoom: {
            wheel: { enabled: true },
            pinch: { enabled: true },
            drag: { 
                enabled: true, 
                backgroundColor: 'rgba(59, 130, 246, 0.1)',
                borderColor: 'rgba(59, 130, 246, 0.4)',
                borderWidth: 1
            },
            mode: 'x',
            onZoomComplete: ({chart}) => {
                const {min, max} = chart.scales.x;
                if (min && max && isFinite(min) && isFinite(max)) {
                    window.applyTimeRangeFromChart(min, max);
                }
            }
        },
        pan: {
            enabled: true,
            mode: 'x'
        }
    };
}

/**
 * Synchronize global time picker and refresh dashboard when zooming on a chart
 */
window.applyTimeRangeFromChart = function(min, max) {
    if (!min || !max || !isFinite(min) || !isFinite(max)) return;
    
    const fromDate = new Date(min);
    const toDate = new Date(max);
    
    // Check if the range is significant enough (e.g. > 5 seconds)
    if (toDate.getTime() - fromDate.getTime() < 5000) return;

    const pad = (n) => n.toString().padStart(2, '0');
    const formatForInput = (date) => {
        return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
    };
    
    window.appState.fromTs = formatForInput(fromDate);
    window.appState.toTs = formatForInput(toDate);
    
    // Update the actual input fields if they exist in the DOM
    const fromInput = document.getElementById('from-ts');
    const toInput = document.getElementById('to-ts');
    if (fromInput) fromInput.value = window.appState.fromTs;
    if (toInput) toInput.value = window.appState.toTs;

    console.log(`[Dashboard] Zoomed to: ${window.appState.fromTs} - ${window.appState.toTs}. Refreshing dashboard data...`);

    // Use refreshDashboardData if it's the newer V2 dashboard (DashboardView redirects to V2)
    if (typeof window.refreshDashboardData === 'function') {
        window.refreshDashboardData();
    } else if (window.appNavigate && window.appState.activeViewId) {
        window.appNavigate(window.appState.activeViewId);
    }
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
        let fromISO = new Date(Date.now() - 3600000).toISOString();
        let toISO = new Date().toISOString();

        if (window.appState.fromTs && window.appState.toTs) {
            fromISO = new Date(window.appState.fromTs).toISOString();
            toISO = new Date(window.appState.toTs).toISOString();
        }

        const [sqlserverRes, dbRes, topQueriesRes, longRunningRes, bottlenecksRes, liveDashRes, cpuHistRes] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/timescale/sqlserver/metrics?instance=${encodeURIComponent(instanceName)}&from=${fromISO}&to=${toISO}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/db-throughput?instance=${encodeURIComponent(instanceName)}&from=${fromISO}&to=${toISO}`),
            window.apiClient.authenticatedFetch(`/api/timescale/sqlserver/top-queries?instance=${encodeURIComponent(instanceName)}&from=${fromISO}&to=${toISO}`),
            window.apiClient.authenticatedFetch(`/api/timescale/sqlserver/long-running-queries?instance=${encodeURIComponent(instanceName)}&from=${fromISO}&to=${toISO}${dbQ}`),
            window.apiClient.authenticatedFetch(`/api/queries/bottlenecks?instance=${encodeURIComponent(instanceName)}&from=${fromISO}&to=${toISO}&time_range=${encodeURIComponent(topOffendersSnapshotRange)}&limit=20${dbQ}`),
            window.apiClient.authenticatedFetch(`/api/sqlserver/dashboard/v2?instance=${encodeURIComponent(instanceName)}&from=${fromISO}&to=${toISO}`),
            window.apiClient.authenticatedFetch(`/api/timescale/sqlserver/cpu-history?instance=${encodeURIComponent(instanceName)}&from=${fromISO}&to=${toISO}`)
        ]).finally(() => {
            window.appState.fetchingMetrics = false;
        });

        if (sqlserverRes.ok) {
            const data = await sqlserverRes.json();
            window.appState.timescaleMetrics.sqlserver = data.metrics || [];
        }
        if (cpuHistRes && cpuHistRes.ok) {
            const data = await cpuHistRes.json();
            window.appState.timescaleMetrics.cpuHistory = data.points || [];
        }        if (dbRes.ok) {
            const data = await dbRes.json();
            window.appState.timescaleMetrics.throughput = data.db_stats || [];
        }
        if (topQueriesRes.ok) {
            const data = await topQueriesRes.json();
            const raw = data.top_queries || [];
            window.appState.timescaleMetrics.topQueries = typeof window.dedupeSqlServerTopQueries === 'function'
                ? window.dedupeSqlServerTopQueries(raw)
                : raw;
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

    window.appState.dashboardPollingInterval = window.registerInterval(async () => {
        if (window.appState.activeViewId === 'dashboard') {
            await fetchAndUpdate();
        }
    }, 30000); // Increased to 30 seconds to reduce API calls
}

function setChartMessage(canvasId, message, isRefreshing = false) {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;
    const parent = canvas.parentElement;
    if (!parent) return;

    let overlay = parent.querySelector('.chart-no-data-overlay');
    if (!overlay) {
        overlay = document.createElement('div');
        overlay.className = 'chart-no-data-overlay';
        overlay.style.cssText = 'position:absolute; top:0; left:0; width:100%; height:100%; display:flex; flex-direction:column; align-items:center; justify-content:center; background:rgba(20,20,35,0.7); z-index:10; color:var(--text-muted); font-size:0.8rem; pointer-events:none; border-radius:8px;';
        parent.style.position = 'relative';
        parent.appendChild(overlay);
    }

    if (message) {
        overlay.style.display = 'flex';
        overlay.innerHTML = isRefreshing 
            ? '<div class="spinner" style="width:20px;height:20px;margin-bottom:8px;"></div><span>Refreshing...</span>'
            : `<i class="fa-solid fa-info-circle" style="font-size:1.2rem;margin-bottom:8px;"></i><span>${window.escapeHtml(message)}</span>`;
        canvas.style.opacity = '0.2';
    } else {
        overlay.style.display = 'none';
        canvas.style.opacity = '1';
    }
}

async function updateDashboardCharts() {
    if (!window.appState.currentInstanceName) return;
    
    const v2 = window.appState.dashboardV2 || {};
    const liveData = window.appState.liveMetrics || {};
    const risk = v2.health_risk || {};
    const workload = v2.workload_capacity || {};

    // 1. Update KPI Strips (Surgical)
    const updateKpi = (id, val, suffix = '', dangerThreshold = null, warningThreshold = null) => {
        const el = document.getElementById(id);
        if (!el) return;
        
        const numericVal = parseFloat(val);
        const displayVal = (val === null || val === undefined || isNaN(numericVal) || numericVal < 0) ? '--' : numericVal.toFixed(id.includes('cpu') || id.includes('memory') ? 1 : 0) + suffix;
        el.textContent = displayVal;

        const cell = el.closest('.strip-metric-cell');
        if (cell) {
            cell.classList.remove('strip-metric-cell--accent-bad', 'strip-metric-cell--accent-warn');
            if (dangerThreshold !== null && numericVal > dangerThreshold) cell.classList.add('strip-metric-cell--accent-bad');
            else if (warningThreshold !== null && numericVal > warningThreshold) cell.classList.add('strip-metric-cell--accent-warn');
        }
    };

    const cpu = liveData.avg_cpu_load ?? -1;
    const mem = liveData.memory_usage ?? -1;
    updateKpi('metric-cpu', cpu, '%', 90, 60);
    updateKpi('metric-memory', mem, '%', 95, 85);
    
    const dbFilter = window.appState.currentDatabase || 'all';
    let diskD = 0, diskL = 0;
    if(dbFilter === 'all') { 
        diskD = ((liveData.disk_usage?.data_mb||0)/1024).toFixed(1); 
        diskL = ((liveData.disk_usage?.log_mb||0)/1024).toFixed(1); 
    } else if(liveData.disk_by_db && liveData.disk_by_db[dbFilter]) { 
        diskD = ((liveData.disk_by_db[dbFilter].data_mb||0)/1024).toFixed(1); 
        diskL = ((liveData.disk_by_db[dbFilter].log_mb||0)/1024).toFixed(1); 
    }
    const diskEl = document.getElementById('metric-disk');
    if (diskEl) diskEl.textContent = `${diskD}/${diskL}`;

    const throughputStats = window.appState.timescaleMetrics.throughput || [];
    let tps = 0;
    if(dbFilter === 'all') tps = throughputStats.reduce((s,i) => s + (i.avg_tps || i.tps || 0), 0);
    else { const dbStat = throughputStats.find(s => s.database_name === dbFilter); if(dbStat) tps = (dbStat.avg_tps || dbStat.tps || 0); }
    updateKpi('metric-tps', tps);

    updateKpi('metric-active', liveData.active_users);
    updateKpi('metric-blocked', risk.blocking_sessions ?? 0, '', 0);

    // Health Score
    const hsScore = risk.health_score?.score;
    const hsBadge = document.getElementById('healthScoreBadge');
    if (hsBadge) {
        hsBadge.textContent = hsScore != null ? hsScore : '--';
        hsBadge.className = `badge ml-2 ${hsScore > 80 ? 'badge-success' : (hsScore > 60 ? 'badge-warning' : 'badge-danger')}`;
    }

    updateKpi('metric-grants', risk.memory_grants_pending, '', 0);
    updateKpi('metric-logins', risk.failed_logins_5m, '', 0);
    updateKpi('metric-tempdb', risk.tempdb_used_percent, '%', 80, 60);
    updateKpi('metric-logused', risk.max_log_used_percent, '%', 80, 60);
    const logDbEl = document.getElementById('metric-logused-db');
    if (logDbEl) logDbEl.textContent = risk.max_log_db_name || '';
    updateKpi('metric-ple', risk.ple);
    updateKpi('metric-comp-ratio', (workload.compilation_ratio || 0) * 100, '%', 50, 25);

    // 2. Update Charts
    updateCpuChart();
    updateBatchChart();
    updateIoChart();
    updatePleChart();
    updateBchrChart();
    updateWaitCategoriesDonut();
    
    // 3. Update Tables (Incremental rows only if needed)
    updateActiveSessionsTable();
    updateTopOffendersTable();
    updateLongRunningQueriesTable();
    
    updateLastRefreshTime();
}

function updateCpuChart() {
    const canvasId = 'dashResourcesChart';
    const cpuHist = window.appState.timescaleMetrics?.cpuHistory || window.appState.liveMetrics?.cpu_history || [];

    if (cpuHist.length === 0) {
        setChartMessage(canvasId, "No CPU history available");
        return;
    }

    const sorted = (window.sortByChartTime ? window.sortByChartTime(cpuHist) : [...cpuHist]).slice(-60);

    const sqlData = sorted.map(t => {
        const x = window.parseChartTimestamp ? window.parseChartTimestamp(t) : new Date();
        return { x, y: typeof t.sql_process === 'number' ? t.sql_process : (t.avg_cpu_load || 0) };
    }).filter(p => p.x && !isNaN(p.x.getTime()));
    const systemData = sorted.map(t => {
        const x = window.parseChartTimestamp ? window.parseChartTimestamp(t) : new Date();
        const val = t.system_idle != null ? (100 - t.system_idle - (typeof t.sql_process === 'number' ? t.sql_process : (t.avg_cpu_load || 0))) : 0;
        return { x, y: val };
    }).filter(p => p.x && !isNaN(p.x.getTime()));

    if (window.currentCharts.dashRes) {
        window.currentCharts.dashRes.destroy();
    }
    
    const ctx = document.getElementById(canvasId)?.getContext('2d');
    if (!ctx) return;
    window.currentCharts.dashRes = new Chart(ctx, {
        type: 'line',
        data: {
            datasets: [
                { label: 'SQL Server', data: sqlData, borderColor: window.getCSSVar('--accent-blue'), backgroundColor: 'rgba(59, 130, 246, 0.1)', fill: true, tension: 0.3, pointRadius: 0 },
                { label: 'Other/System', data: systemData, borderColor: 'rgba(255,255,255,0.2)', fill: false, tension: 0.3, pointRadius: 0 }
            ]
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            plugins: { 
                legend: { display: false },
                zoom: getChartZoomOptions()
            },
            scales: { 
                y: { beginAtZero: true, max: 100 }, 
                x: { 
                    type: 'time',
                    time: { displayFormats: { minute: 'HH:mm', second: 'HH:mm:ss' } },
                    ticks: { maxTicksLimit: 8 } 
                } 
            }
        }
    });
    setChartMessage(canvasId, null);
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
        tbody.innerHTML = '<tr><td colspan="4" class="text-center"><div class="spinner"></div> Loading long running queries...</td></tr>';
        return;
    }

    window.appState.queryCache = window.appState.queryCache || {};
    const longRunningData = window.appState.timescaleMetrics.longRunningQueries || [];
    const dbFilter = window.appState.currentDatabase || 'all';
    const filteredData = groupLongRunningQueriesForDisplay(longRunningData, dbFilter);
    
    if (filteredData.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">No long running queries for this database and time range.</td></tr>';
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
            return `<tr><td><span class="badge badge-outline">${tsStr}</span><span class="text-muted" style="font-size:0.65rem; margin-left:4px;">${ageStr}</span></td><td><span class="code-snippet" style="cursor:pointer" data-action="show-query-modal-direct" data-key="lrq${idx}">${window.escapeHtml(qt)}</span></td><td>${(q.cpuTimeMs || 0)}ms</td><td>${(q.totalElapsedMs || 0)}ms</td></tr>`;
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
        ${session.blocking_session_id ? `<div style="margin-top:1rem;"><button class="btn btn-sm btn-outline" style="color:var(--accent, #3b82f6);" data-action="navigate" data-route="sqlserver-locks" data-also-call="closeSessionDetail"><i class="fa-solid fa-link"></i> Drill Down to Locks</button></div>` : ''}
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
        <div style="text-align: center; margin-top: 1rem;">
            <button id="copySqlBtnDashboard" style="background: var(--accent, #3b82f6); color: #fff; border: none; padding: 0.5rem 1.5rem; border-radius: 6px; cursor: pointer; font-size: 0.9rem;">
                <i class="fa-solid fa-copy"></i> copy SQL
            </button>
        </div>
    </div>`;
    document.body.appendChild(modal);
    
    document.getElementById('copySqlBtnDashboard').addEventListener('click', function() {
        const pre = modal.querySelector('pre');
        const textToCopy = pre ? pre.textContent : queryText;
        navigator.clipboard.writeText(textToCopy).then(() => {
            this.innerHTML = '<i class="fa-solid fa-check"></i> copied!';
            setTimeout(() => {
                this.innerHTML = '<i class="fa-solid fa-copy"></i> copy SQL';
            }, 1500);
        });
    });

    modal.addEventListener('click', e => { if(e.target === modal) modal.remove(); });
};

function renderDashboardStructure(inst, v2) {
    const dbFilter = window.appState.currentDatabase || 'all';
    const contentArea = document.getElementById('dashboard-content-area');
    if (!contentArea) return;

    contentArea.innerHTML = `
        <div id="sql-dashboard-alert-banner" style="display:none;"></div>
        
        <!-- ROW 1: MISSION CRITICAL KPIs -->
        <div class="kpi-row">
            <div class="glass-panel metric-card-compact h-kpi" title="Percentage of CPU used by the SQL Server process. High values (>90%) may indicate CPU pressure.">
                <div class="metric-label">CPU Load <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="CPU Load"></i></div>
                <div class="metric-value" id="metric-cpu">--%</div>
            </div>
            <div class="glass-panel metric-card-compact h-kpi" style="cursor:pointer;" data-action="navigate" data-route="drilldown-memory" title="Percentage of physical memory used on the host. Click for detailed memory analysis.">
                <div class="metric-label">Memory Usage <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="Memory Usage"></i></div>
                <div class="metric-value" id="metric-memory">--%</div>
            </div>
            <div class="glass-panel metric-card-compact h-kpi" title="Transactions Per Second across all user databases. A key measure of database throughput.">
                <div class="metric-label">TPS <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="TPS"></i></div>
                <div class="metric-value text-success" id="metric-tps">--</div>
            </div>
            <div class="glass-panel metric-card-compact h-kpi" title="Number of active concurrent user sessions currently executing requests.">
                <div class="metric-label">Active Users <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="Active Users"></i></div>
                <div class="metric-value text-accent" id="metric-active">--</div>
            </div>
            <div class="glass-panel metric-card-compact h-kpi" style="cursor:pointer;" data-action="navigate" data-route="sqlserver-locks" title="Number of sessions currently blocked by other sessions. Click to view blocking chains.">
                <div class="metric-label">Blocked <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="Blocked Sessions"></i></div>
                <div class="metric-value text-critical" id="metric-blocked">--</div>
            </div>
            <div class="glass-panel metric-card-compact h-kpi" title="Aggregate health score (0-100) based on CPU, memory, blocking, and waits.">
                <div class="metric-label">Health Score <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="Health Score"></i></div>
                <div class="metric-value" id="healthScoreBadge">--</div>
            </div>
        </div>

        <!-- ROW 2: CORE PERFORMANCE TRENDS -->
        <div class="grid-container">
            <div class="col-4 col-laptop-4 col-tablet-6">
                <div class="card glass-panel h-chart-md">
                    <div class="card-header" title="Trend of SQL Server CPU usage vs total system CPU usage over the last hour."><h3 style="font-size:0.8rem; margin:0;">System Resources (CPU) <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="CPU Load"></i></h3></div>
                    <div class="chart-container" style="height:210px;"><canvas id="dashResourcesChart"></canvas></div>
                </div>
            </div>
            <div class="col-4 col-laptop-4 col-tablet-6">
                <div class="card glass-panel h-chart-md">
                    <div class="card-header" title="Number of SQL batches received per second. A core indicator of server workload intensity."><h3 style="font-size:0.8rem; margin:0;">Batch Requests/sec <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Real-Time Diagnostics" data-metric="Batch Requests/sec"></i></h3></div>
                    <div class="chart-container" style="height:210px;"><canvas id="dashBatchChart"></canvas></div>
                </div>
            </div>
            <div class="col-4 col-laptop-8 col-tablet-6">
                <div class="card glass-panel h-chart-md">
                    <div class="card-header" title="Distribution of wait times by category (CPU, IO, Lock, etc.) over the last 15 minutes."><h3 style="font-size:0.8rem; margin:0;">Wait Categories (15m) <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Enterprise Metrics" data-metric="Runnable Tasks"></i></h3></div>
                    <div class="chart-container" style="height:210px;"><canvas id="dashWaitCategoriesDonut"></canvas></div>
                </div>
            </div>
        </div>

        <!-- ROW 3: STORAGE & MEMORY -->
        <div class="grid-container mt-3">
            <div class="col-4 col-laptop-4 col-tablet-3">
                <div class="card glass-panel" style="height: 180px; padding: 0.75rem;" title="Average time taken for disk read and write operations. High values (>20ms) indicate storage bottlenecks.">
                    <div class="card-header" style="padding: 0 0 0.5rem 0;"><h4 style="font-size:0.7rem; margin:0;">Disk I/O Latency (ms) <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="TPS"></i></h4></div>
                    <div class="chart-container" style="height:120px;"><canvas id="dashIoChart"></canvas></div>
                </div>
            </div>
            <div class="col-4 col-laptop-4 col-tablet-3">
                <div class="card glass-panel" style="height: 180px; padding: 0.75rem;" title="Average time a data page stays in memory. Lower values indicate memory pressure and frequent disk reads.">
                    <div class="card-header" style="padding: 0 0 0.5rem 0;"><h4 style="font-size:0.7rem; margin:0;">Page Life Expectancy <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="PLE"></i></h4></div>
                    <div class="chart-container" style="height:120px;"><canvas id="dashPleChart"></canvas></div>
                </div>
            </div>
            <div class="col-4 col-laptop-8 col-tablet-6">
                <div class="card glass-panel" style="height: 180px; padding: 0.75rem;" title="Percentage of page requests satisfied from memory rather than disk. Should ideally be >95%.">
                    <div class="card-header" style="padding: 0 0 0.5rem 0;"><h4 style="font-size:0.7rem; margin:0;">Buffer Cache Hit % <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="Memory Usage"></i></h4></div>
                    <div class="chart-container" style="height:120px;"><canvas id="dashBchrChart"></canvas></div>
                </div>
            </div>
        </div>

        <!-- ROW 4: TOP OFFENDERS & SESSIONS -->
        <div class="grid-container mt-3">
            <div class="col-8 col-laptop-8 col-tablet-6">
                <div class="card glass-panel h-table-md">
                    <div class="card-header flex-between">
                        <h3 style="font-size:0.85rem; margin:0;" title="Historical query performance bottlenecks identified by Query Store."><i class="fa-solid fa-bolt text-accent"></i> Top Offenders (Query Store) <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Query Analysis" data-metric="Regressions"></i></h3>
                        <a data-action="navigate" data-route="drilldown-bottlenecks" class="btn btn-xs btn-outline text-accent">Full Analysis</a>
                    </div>
                    <div class="table-responsive">
                        <table class="data-table" id="topOffendersGrid" style="font-size:0.7rem;">
                            <thead><tr>
                                <th title="Rank">#</th>
                                <th title="Database Name">DB</th>
                                <th title="SQL Query Text (truncated)">SQL Text</th>
                                <th title="Total number of executions in the period">Exec</th>
                                <th title="Average CPU time per execution (ms)">Avg CPU</th>
                                <th title="Average elapsed time per execution (ms)">Avg Dur</th>
                                <th title="Total cumulative CPU time (ms)">Total CPU</th>
                            </tr></thead>
                            <tbody id="top-offenders-body"></tbody>
                        </table>
                    </div>
                </div>
            </div>
            <div class="col-4 col-laptop-4 col-tablet-6">
                <div class="card glass-panel h-table-md">
                    <div class="card-header flex-between">
                        <h3 style="font-size:0.85rem; margin:0;" title="Queries currently executing that have exceeded typical execution time thresholds."><i class="fa-solid fa-clock text-accent"></i> Long Running <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Real-Time Diagnostics" data-metric="Batch Requests/sec"></i></h3>
                        <span class="badge badge-warning" id="long-running-badge">0</span>
                    </div>
                    <div class="table-responsive">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr>
                                <th title="Last seen timestamp">Time</th>
                                <th title="SQL Query Text">SQL</th>
                                <th title="CPU time used (ms)">CPU</th>
                                <th title="Total elapsed time (ms)">Dur</th>
                            </tr></thead>
                            <tbody id="long-running-body"></tbody>
                        </table>
                    </div>
                </div>
            </div>
            <div class="col-12">
                <div class="card glass-panel h-table-md">
                    <div class="card-header flex-between">
                        <h3 style="font-size:0.85rem; margin:0;" title="Active concurrent sessions with significant resource usage or blocking."><i class="fa-solid fa-users text-accent"></i> Significant Active Sessions <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-sqlserver-info" data-section="Instance Dashboard" data-metric="Active Users"></i></h3>
                        <span class="badge badge-info" id="active-sessions-badge">0</span>
                    </div>
                    <div class="table-responsive">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr>
                                <th title="Session ID (SPID)">SPID</th>
                                <th title="Current session state">State</th>
                                <th title="Blocking session ID (if any)">Blocker</th>
                                <th title="Current execution duration">Duration</th>
                                <th title="Current wait type">Wait Type</th>
                                <th title="Database Name">Database</th>
                                <th title="Login Name">Login</th>
                                <th title="SQL Query Text">SQL Text</th>
                            </tr></thead>
                            <tbody id="active-sessions-body"></tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    `;

    bindTopOffendersControls();
    if (window.updateSqlDashboardAlertBanner) window.updateSqlDashboardAlertBanner();
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
    cpuHist = window.sortByChartTime ? window.sortByChartTime(cpuHist) : cpuHist;
    if(cpuHist.length > 60) cpuHist = cpuHist.slice(-60);
    const sqlArr = cpuHist.map(t => t.sql_process);
    const idleArr = cpuHist.map(t => t.system_idle);
    const cAxis = cpuHist.map(t => window.fmtChartTick ? window.fmtChartTick(t) || '-' : '-');

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
            interaction: { mode: 'index', intersect: false },
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
                backgroundColor: 'rgba(16, 185, 129, 0.1)',
                fill: true,
                tension: 0.35,
                pointRadius: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
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
                backgroundColor: 'rgba(59, 130, 246, 0.1)',
                fill: true,
                tension: 0.35,
                pointRadius: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
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
            interaction: { mode: 'index', intersect: false },
            plugins: {
                legend: {
                    display: true,
                    position: 'top',
                    labels: { boxWidth: 12, font: { size: 11 } }
                }
            },
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
    
    // Source from window.appState.pleHistory, fallback to dashboardV2?.memory_storage_internals?.ple_history
    let pleHist = window.appState.pleHistory || [];
    if (!pleHist || pleHist.length === 0) {
        pleHist = window.appState.dashboardV2?.memory_storage_internals?.ple_history || [];
    }
    
    let hasData = false;
    let pleData = [];
    let labels = [];
    
    if (pleHist.length > 0) {
        if (typeof pleHist[0] === 'number') {
            hasData = pleHist.some(v => v > 0);
            if (hasData) {
                pleData = pleHist.slice(-60);
                labels = pleData.map((_, i) => '-' + (pleData.length - 1 - i) + 'm');
            }
        } else if (typeof pleHist[0] === 'object') {
            // Support keys: ple, avg_ple, value, ple_seconds
            hasData = pleHist.some(v => (v.ple ?? v.avg_ple ?? v.value ?? v.ple_seconds ?? 0) > 0);
            if (hasData) {
                const sorted = [...pleHist].sort((a, b) => {
                    const taStr = String(a.timestamp || a.capture_time || a.event_time || '').replace(' ', 'T');
                    const tbStr = String(b.timestamp || b.capture_time || b.event_time || '').replace(' ', 'T');
                    const ta = new Date(taStr).getTime();
                    const tb = new Date(tbStr).getTime();
                    return (isNaN(ta) || isNaN(tb)) ? 0 : ta - tb;
                }).slice(-60);
                pleData = sorted.map(t => t.ple ?? t.avg_ple ?? t.value ?? t.ple_seconds ?? 0);
                labels = sorted.map(t => {
                    const ts = t.timestamp || t.capture_time || t.event_time;
                    if(!ts) return '';
                    const tsStr = String(ts).replace(' ', 'T');
                    const date = new Date(tsStr);
                    if (isNaN(date.getTime())) return '-';
                    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
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
            interaction: { mode: 'index', intersect: false },
            plugins: { 
                legend: { display: false }
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

// Phase 2: Wait Categories donut (last 15 minutes) from /api/sqlserver/dashboard/v2 payload.
window.updateWaitCategoriesDonut = function() {
    const canvasId = 'dashWaitCategoriesDonut';
    const agg = window.appState.dashboardV2?.root_cause?.wait_categories_15m || [];

    if (agg.length === 0) {
        setChartMessage(canvasId, "No recent wait activity");
        return;
    }

    const labels = agg.map(r => r.wait_category || 'Other');
    const values = agg.map(r => Number(r.wait_time_ms || 0));
    const total = values.reduce((s, v) => s + (isFinite(v) ? v : 0), 0);

    if (!isFinite(total) || total <= 0) {
        setChartMessage(canvasId, "No wait activity detected");
        return;
    }

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

    if (window.currentCharts && window.currentCharts.waitCategoriesDonut) {
        window.currentCharts.waitCategoriesDonut.data.labels = labels;
        window.currentCharts.waitCategoriesDonut.data.datasets[0].data = values;
        window.currentCharts.waitCategoriesDonut.data.datasets[0].backgroundColor = bg;
        window.currentCharts.waitCategoriesDonut.update('none');
    } else {
        const ctx = document.getElementById(canvasId)?.getContext('2d');
        if (!ctx) return;
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
    }
    setChartMessage(canvasId, null);
};
