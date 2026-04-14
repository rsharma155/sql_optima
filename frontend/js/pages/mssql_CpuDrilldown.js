/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: CPU drilldown page for detailed processor utilization analysis.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.escapeHtml = function(unsafe) { return (!unsafe) ? '' : unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;"); };

/** Convert datetime-local value to RFC3339 for Timescale APIs */
window.cpuDrilldownLocalToRFC3339 = function(localDt) {
    if (!localDt) return '';
    const d = new Date(localDt);
    if (isNaN(d.getTime())) return '';
    return d.toISOString();
};

window.CpuDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div class="flex-between" style="align-items:center; gap:1rem;">
                    <button class="btn btn-sm btn-outline" style="padding:0.3rem 0.6rem; font-size:1.1rem;" onclick="window.appNavigate('dashboard')" title="Back to Dashboard"><i class="fa-solid fa-arrow-left"></i></button>
                    <h1 style="font-size: 1.5rem;">CPU Drilldown <span class="subtitle">- Instance: ${window.escapeHtml(inst.name)}</span></h1>
                </div>
                <div style="display: flex; align-items: center; gap: 1rem;">
                    <div class="glass-panel" style="padding: 0.5rem 1rem; display: flex; align-items: center; gap: 0.5rem;">
                        <label style="font-size: 0.8rem; color: var(--text-muted);">from:</label>
                        <input type="datetime-local" id="cpuDrillFrom" class="custom-select" style="padding: 0.25rem; font-size: 0.8rem;">
                    </div>
                    <div class="glass-panel" style="padding: 0.5rem 1rem; display: flex; align-items: center; gap: 0.5rem;">
                        <label style="font-size: 0.8rem; color: var(--text-muted);">to:</label>
                        <input type="datetime-local" id="cpuDrillTo" class="custom-select" style="padding: 0.25rem; font-size: 0.8rem;">
                    </div>
                    <button class="btn btn-sm btn-accent" onclick="window.applyCpuDrilldownRange()"><i class="fa-solid fa-filter"></i> Apply</button>
                    <button class="btn btn-sm btn-outline" onclick="window.refreshCpuDrilldown()"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            
            <div class="chart-card glass-panel mt-4" style="height: 200px;">
                <div class="card-header flex-between">
                    <h3>CPU Usage (Historical)</h3>
                    <span id="cpuDrilldownLastUpdate" class="text-muted" style="font-size:0.8rem;">Loading...</span>
                </div>
                <div class="chart-container" style="height: 140px;"><canvas id="cpuHistoryChart"></canvas></div>
            </div>

            <div class="table-card glass-panel mt-4">
                <div class="card-header flex-between" style="flex-wrap: wrap; gap: 0.5rem;">
                    <div style="display: flex; align-items: center; gap: 0.5rem;">
                        <h3>Top Queries</h3>
                        <select id="queryMetricSelect" class="custom-select" style="padding: 0.25rem 0.5rem; font-size: 0.8rem;" onchange="window.changeQueryMetric()">
                            <option value="cpu">CPU Time</option>
                            <option value="duration">Duration</option>
                            <option value="reads">Logical Reads</option>
                            <option value="executions">Executions</option>
                        </select>
                    </div>
                    <span id="queryCount" class="badge badge-info">0 queries</span>
                </div>
                <div style="font-size: 0.75rem; color: var(--text-muted); margin: 0.5rem 0; padding: 0.5rem; background: rgba(0,0,0,0.2); border-radius: 4px;">
                    <i class="fa-solid fa-info-circle"></i> Note: Login and Client App columns show the most recent user/application that executed each query. If a background application connects, runs a query, and instantly disconnects, the connection context may be lost and will display as "Unknown/Disconnected".
                </div>
                <div class="table-responsive" style="max-height: 500px; overflow-y: auto; position: relative;">
                    <table class="data-table" style="border-collapse: separate; border-spacing: 0;" id="cpuQueriesTable">
                        <thead style="position: sticky; top: 0; z-index: 10; background: var(--bg-surface, #1a1a2e);">
                            <tr>
                                <th style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e); cursor:pointer;" onclick="window.sortCpuTable(0)">#</th>
                                <th style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e); cursor:pointer;" onclick="window.sortCpuTable(1)">Time</th>
                                <th style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e); cursor:pointer;" onclick="window.sortCpuTable(2)">Database <i class="fa-solid fa-sort"></i></th>
                                <th style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e);">Query Text</th>
                                <th id="cpuDrillMetricTotalTh" style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e); cursor:pointer;" onclick="window.sortCpuTable(4)">Total CPU (ms) <i class="fa-solid fa-sort"></i></th>
                                <th id="cpuDrillMetricAvgTh" style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e); cursor:pointer;" onclick="window.sortCpuTable(5)">Avg CPU (ms) <i class="fa-solid fa-sort"></i></th>
                                <th style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e); cursor:pointer;" onclick="window.sortCpuTable(6)">Execs <i class="fa-solid fa-sort"></i></th>
                                <th style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e); cursor:pointer;" onclick="window.sortCpuTable(7)">Login <i class="fa-solid fa-sort"></i></th>
                                <th style="position: sticky; top: 0; background: var(--bg-surface, #1a1a2e); cursor:pointer;" onclick="window.sortCpuTable(8)">Client App <i class="fa-solid fa-sort"></i></th>
                            </tr>
                        </thead>
                        <tbody id="cpuQueriesBody">
                            <tr><td colspan="9" class="text-center"><div class="spinner"></div> Loading queries...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    `;

    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    const dd = String(now.getDate()).padStart(2, '0');
    const hh = String(now.getHours()).padStart(2, '0');
    const min = String(now.getMinutes()).padStart(2, '0');
    
    document.getElementById('cpuDrillFrom').value = `${yyyy}-${mm}-${dd}T${String(oneHourAgo.getHours()).padStart(2,'0')}:${String(oneHourAgo.getMinutes()).padStart(2,'0')}`;
    document.getElementById('cpuDrillTo').value = `${yyyy}-${mm}-${dd}T${hh}:${min}`;

    window.appState.cpuQueriesTableData = [];
    window.appState.currentQueryMetric = 'cpu';
    window.updateCpuDrillMetricHeaders();
    const fromVal = document.getElementById('cpuDrillFrom').value;
    const toVal = document.getElementById('cpuDrillTo').value;
    await Promise.all([
        window.loadCpuDrilldownChartOnly(inst.name, fromVal, toVal),
        window.loadQueryStatsWithMetric(inst.name, 'cpu', '1h', fromVal, toVal)
    ]);
};

window.updateCpuDrillMetricHeaders = function() {
    const m = window.appState.currentQueryMetric || 'cpu';
    const totalTh = document.getElementById('cpuDrillMetricTotalTh');
    const avgTh = document.getElementById('cpuDrillMetricAvgTh');
    const labels = {
        cpu: ['Total CPU (ms)', 'Avg CPU (ms)'],
        duration: ['Total Duration (ms)', 'Avg Duration (ms)'],
        reads: ['Logical Reads', 'Avg Reads / exec'],
        executions: ['Executions', 'Avg CPU (ms)']
    };
    const pair = labels[m] || labels.cpu;
    if (totalTh) {
        totalTh.innerHTML = pair[0] + ' <i class="fa-solid fa-sort"></i>';
        totalTh.onclick = function() { window.sortCpuTable(4); };
    }
    if (avgTh) {
        avgTh.innerHTML = pair[1] + ' <i class="fa-solid fa-sort"></i>';
        avgTh.onclick = function() { window.sortCpuTable(5); };
    }
};

window.changeQueryMetric = async function() {
    const metricSelect = document.getElementById('queryMetricSelect');
    if (!metricSelect) return;
    window.appState.currentQueryMetric = metricSelect.value;
    window.updateCpuDrillMetricHeaders();
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    const fromInput = document.getElementById('cpuDrillFrom');
    const toInput = document.getElementById('cpuDrillTo');
    await window.loadQueryStatsWithMetric(inst.name, metricSelect.value, '1h',
        fromInput ? fromInput.value : '', toInput ? toInput.value : '');
};

window.loadQueryStatsWithMetric = async function(instanceName, metric, timeRange, fromLocal, toLocal) {
    if (!metric) metric = window.appState.currentQueryMetric || 'cpu';
    if (!timeRange) timeRange = '1h';
    const rangeErr = window.getDatetimeLocalRangeError && window.getDatetimeLocalRangeError(fromLocal, toLocal);
    if (rangeErr) {
        window.showDateRangeValidationError(rangeErr);
        return;
    }
    const fromISO = window.cpuDrilldownLocalToRFC3339(fromLocal);
    const toISO = window.cpuDrilldownLocalToRFC3339(toLocal);
    const tbodyLoading = document.getElementById('cpuQueriesBody');
    if (tbodyLoading) {
        tbodyLoading.innerHTML = '<tr><td colspan="9" class="text-center"><div class="spinner"></div> Loading queries…</td></tr>';
    }
    try {
        let apiUrl = `/api/timescale/mssql/query-stats-dashboard?instance=${encodeURIComponent(instanceName)}&metric=${encodeURIComponent(metric)}&time_range=${encodeURIComponent(timeRange)}&dimension=query&limit=50`;
        if (fromISO && toISO) {
            apiUrl += `&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`;
        }
        const res = await window.apiClient.authenticatedFetch(apiUrl);
        if (!res.ok) throw new Error('query-stats-dashboard failed');
        const data = await res.json();
        const queries = data.results || [];
        // query-stats-dashboard reads sqlserver_query_stats_interval; collectors often only populate
        // sqlserver_top_queries first. Empty results here is normal — use Timescale range / latest / DMV.
        if (queries.length === 0) {
            appDebug('query-stats-dashboard returned 0 rows; loading sqlserver_top_queries via cpu-drilldown');
            await window.loadCpuDrilldownDataLegacy(instanceName, fromLocal, toLocal);
            return;
        }
        const countEl = document.getElementById('queryCount');
        if (countEl) countEl.textContent = queries.length + ' queries';
        window.appState.cpuQueriesTableData = queries;
        window.renderCpuDrilldownTable(queries, fromLocal, toLocal);
    } catch (e) {
        appDebug('Failed to load query stats, using Timescale top queries / DMV fallback:', e);
        await window.loadCpuDrilldownDataLegacy(instanceName, fromLocal, toLocal);
    }
};

window.loadCpuDrilldownDataLegacy = async function(instanceName, fromLocal, toLocal) {
    const rangeErr = window.getDatetimeLocalRangeError && window.getDatetimeLocalRangeError(fromLocal, toLocal);
    if (rangeErr) {
        window.showDateRangeValidationError(rangeErr);
        return;
    }
    try {
        let queriesUrl = `/api/mssql/cpu-drilldown?instance=${encodeURIComponent(instanceName)}&limit=200`;
        const fromISO = window.cpuDrilldownLocalToRFC3339(fromLocal);
        const toISO = window.cpuDrilldownLocalToRFC3339(toLocal);
        if (fromISO && toISO) {
            queriesUrl += `&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`;
        }
        const res = await window.apiClient.authenticatedFetch(queriesUrl);
        if (!res.ok) throw new Error('cpu-drilldown failed');
        const topData = await res.json();
        const queries = topData.queries || topData.top_queries || [];
        const countEl = document.getElementById('queryCount');
        if (countEl) countEl.textContent = queries.length + ' queries';
        window.appState.cpuQueriesTableData = queries;
        window.renderCpuDrilldownTable(queries, fromLocal, toLocal);
    } catch (e) {
        appDebug('Failed to load legacy CPU drilldown queries:', e);
        const tbody = document.getElementById('cpuQueriesBody');
        if (tbody) {
            tbody.innerHTML = `<tr><td colspan="9" class="text-center text-danger">Failed to load queries: ${window.escapeHtml(e.message)}</td></tr>`;
        }
    }
};

window.loadCpuDrilldownChartOnly = async function(instanceName, fromLocal, toLocal) {
    const rangeErr = window.getDatetimeLocalRangeError && window.getDatetimeLocalRangeError(fromLocal, toLocal);
    if (rangeErr) {
        window.showDateRangeValidationError(rangeErr);
        return;
    }
    if (window.setChartOverlayState) window.setChartOverlayState('cpuHistoryChart', 'loading', 'Loading CPU history…');
    try {
        const fromISO = window.cpuDrilldownLocalToRFC3339(fromLocal);
        const toISO = window.cpuDrilldownLocalToRFC3339(toLocal);
        let cpuHistory = [];
        let strictRange = false;

        if (fromISO && toISO) {
            const tsRes = await window.apiClient.authenticatedFetch(
                `/api/timescale/mssql/cpu-history?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`
            );
            if (tsRes.ok) {
                const tsData = await tsRes.json();
                cpuHistory = tsData.points || [];
                strictRange = true;
            } else {
                cpuHistory = [];
                strictRange = true;
            }
        } else {
            const dashRes = await window.apiClient.authenticatedFetch(`/api/mssql/dashboard?instance=${encodeURIComponent(instanceName)}`);
            if (dashRes.ok) {
                const dashData = await dashRes.json();
                if (dashData.cpu_history) {
                    cpuHistory = dashData.cpu_history;
                } else if (dashData.metrics) {
                    cpuHistory = dashData.metrics.map(m => ({
                        event_time: m.capture_timestamp,
                        capture_timestamp: m.capture_timestamp,
                        sql_process: m.avg_cpu_load || 0,
                        system_idle: 100 - (m.avg_cpu_load || 0),
                        other_process: 0
                    }));
                }
            }
            strictRange = false;
        }

        window.appState.cpuDrilldownHistory = cpuHistory;
        window.renderCpuDrilldownCharts(cpuHistory, fromLocal, toLocal, { strictRange });
        window.updateCpuDrilldownTimestamp();
    } catch (e) {
        appDebug('CPU chart load failed:', e);
        if (window.setChartOverlayState) window.setChartOverlayState('cpuHistoryChart', 'empty', 'Could not load chart.');
    }
};

window.loadCpuDrilldownData = async function(instanceName, fromTime, toTime) {
    try {
        let cpuUrl = `/api/timescale/mssql/cpu-history?instance=${encodeURIComponent(instanceName)}`;
        // Top queries: API prefers TimescaleDB (/api/mssql/cpu-drilldown); use Real-Time Diagnostics for live DMV.
        let queriesUrl = `/api/mssql/cpu-drilldown?instance=${encodeURIComponent(instanceName)}&limit=200`;
        
        if (fromTime && toTime) {
            cpuUrl += `&from=${encodeURIComponent(fromTime)}&to=${encodeURIComponent(toTime)}`;
            queriesUrl += `&from=${encodeURIComponent(fromTime)}&to=${encodeURIComponent(toTime)}`;
        }
        
        const [dashRes, topQueriesRes] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/mssql/dashboard?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(queriesUrl)
        ]);

        let cpuHistory = [];
        let cpuQueries = [];

        if (dashRes.ok) {
            const dashData = await dashRes.json();
            if (dashData.cpu_history) {
                cpuHistory = dashData.cpu_history;
            } else if (dashData.metrics) {
                cpuHistory = dashData.metrics.map(m => ({
                    event_time: m.capture_timestamp,
                    sql_process: m.avg_cpu_load || 0,
                    system_idle: 100 - (m.avg_cpu_load || 0),
                    other_process: 0
                }));
            }
        }

        if (topQueriesRes.ok) {
            const topData = await topQueriesRes.json();
            cpuQueries = topData.queries || topData.top_queries || [];
            appDebug('[CPU Drilldown] Top queries received:', cpuQueries.length);
        }

        window.appState.cpuQueriesTableData = cpuQueries;
        window.appState.cpuDrilldownHistory = cpuHistory;

        window.renderCpuDrilldownCharts(cpuHistory, fromTime, toTime);
        window.renderCpuDrilldownTable(cpuQueries, fromTime, toTime);
        window.updateCpuDrilldownTimestamp();
    } catch(e) {
        appDebug("CPU Drilldown data load failed:", e);
        const tbody = document.getElementById('cpuQueriesBody');
        if (tbody) {
            tbody.innerHTML = `<tr><td colspan="9" class="text-center text-danger">Failed to load: ${window.escapeHtml(e.message)}</td></tr>`;
        }
    }
};

window.renderCpuDrilldownCharts = function(cpuHistory, fromTime, toTime, opts) {
    const strictRange = opts && opts.strictRange;

    if (!cpuHistory || cpuHistory.length === 0) {
        if (window.cpuDrilldownChart) {
            window.cpuDrilldownChart.destroy();
            window.cpuDrilldownChart = null;
        }
        if (window.setChartOverlayState) {
            window.setChartOverlayState('cpuHistoryChart', 'empty', strictRange ? 'No CPU samples in this range.' : 'No CPU history yet.');
        }
        return;
    }

    let filtered = cpuHistory;

    if (fromTime && toTime && !strictRange) {
        const fromMs = new Date(fromTime).getTime();
        const toMs = new Date(toTime).getTime();
        filtered = cpuHistory.filter(t => {
            if (!t.event_time && !t.capture_timestamp) return false;
            const ts = t.event_time || t.capture_timestamp;
            const d = new Date(String(ts).replace(' ', 'T')).getTime();
            return d >= fromMs && d <= toMs;
        });
    }

    if (filtered.length === 0 && !strictRange) {
        filtered = cpuHistory;
    }

    if (filtered.length === 0) {
        if (window.cpuDrilldownChart) {
            window.cpuDrilldownChart.destroy();
            window.cpuDrilldownChart = null;
        }
        if (window.setChartOverlayState) {
            window.setChartOverlayState('cpuHistoryChart', 'empty', strictRange ? 'No CPU samples in this range.' : 'No data for the selected window.');
        }
        return;
    }

    if (window.clearChartOverlay) window.clearChartOverlay('cpuHistoryChart');
    
    const sorted = filtered.slice(-500).sort((a, b) => {
        const ta = a.event_time ? new Date(String(a.event_time).replace(' ', 'T')).getTime() : (a.capture_timestamp ? new Date(a.capture_timestamp).getTime() : 0);
        const tb = b.event_time ? new Date(String(b.event_time).replace(' ', 'T')).getTime() : (b.capture_timestamp ? new Date(b.capture_timestamp).getTime() : 0);
        return ta - tb;
    });
    const labels = sorted.map(t => {
        const ts = t.event_time || t.capture_timestamp;
        if (!ts) return '';
        const d = new Date(String(ts).replace(' ', 'T'));
        if (isNaN(d.getTime())) return '';
        return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
    });
    const sqlArr = sorted.map(t => t.sql_process || t.avg_cpu_load || 0);
    const idleArr = sorted.map(t => t.system_idle || (100 - (t.avg_cpu_load || 0)));
    const otherArr = sorted.map(t => t.other_process || 0);

    if (window.cpuDrilldownChart) window.cpuDrilldownChart.destroy();

    const ctx = document.getElementById('cpuHistoryChart').getContext('2d');
    window.cpuDrilldownChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                { label: 'SQL Server CPU', data: sqlArr, borderColor: '#3b82f6', backgroundColor: 'rgba(59, 130, 246, 0.1)', fill: true, tension: 0.4, pointRadius: 0 },
                { label: 'System Idle', data: idleArr, borderColor: '#22c55e', fill: false, tension: 0.4, pointRadius: 0, borderDash: [2, 2] },
                { label: 'Other Processes', data: otherArr, borderColor: '#f59e0b', fill: false, tension: 0.4, pointRadius: 0 }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } }
            },
            scales: {
                y: { max: 100, min: 0, ticks: { callback: v => v + '%' } },
                x: { grid: { display: true, color: 'rgba(255,255,255,0.05)' }, ticks: { maxTicksLimit: 15 } }
            }
        }
    });
};

window.sortCpuTable = function(colIdx) {
    const table = document.getElementById('cpuQueriesTable');
    if (!table) return;
    const tbody = table.querySelector('tbody');
    const rows = Array.from(tbody.querySelectorAll('tr'));
    const dir = table.dataset.sortDir === 'asc' ? 'desc' : 'asc';
    table.dataset.sortDir = dir;
    table.dataset.sortCol = colIdx;

    table.querySelectorAll('thead th i.fa-sort').forEach(i => { i.className = 'fa-solid fa-sort'; });
    const activeTh = table.querySelectorAll('thead th')[colIdx];
    if (activeTh) {
        const icon = activeTh.querySelector('i.fa-sort');
        if (icon) icon.className = dir === 'asc' ? 'fa-solid fa-sort-up' : 'fa-solid fa-sort-down';
    }

    rows.sort((a, b) => {
        const aCell = a.children[colIdx];
        const bCell = b.children[colIdx];
        if (!aCell || !bCell) return 0;
        const aText = aCell.textContent.trim();
        const bText = bCell.textContent.trim();
        const aNum = parseFloat(aText.replace(/[^0-9.\-]/g, ''));
        const bNum = parseFloat(bText.replace(/[^0-9.\-]/g, ''));
        if (!isNaN(aNum) && !isNaN(bNum)) {
            return dir === 'asc' ? aNum - bNum : bNum - aNum;
        }
        return dir === 'asc' ? aText.localeCompare(bText) : bText.localeCompare(aText);
    });

    rows.forEach(r => tbody.appendChild(r));
};

window.renderCpuDrilldownTable = function(queries, fromTime, toTime) {
    const tbody = document.getElementById('cpuQueriesBody');
    if (!tbody) return;
    
    if (!queries || queries.length === 0) {
        document.getElementById('queryCount').textContent = '0 queries';
        tbody.innerHTML = '<tr><td colspan="9" class="text-center text-muted"><i class="fa-solid fa-info-circle"></i> No queries captured yet. Queries will appear here as they execute.</td></tr>';
        return;
    }

    if (queries[0] && Object.prototype.hasOwnProperty.call(queries[0], 'metric_value')) {
        window.renderCpuDrilldownDashboardTable(queries, fromTime, toTime);
        return;
    }

    let filtered = queries;
    if (fromTime && toTime) {
        const fromMs = new Date(fromTime).getTime();
        const toMs = new Date(toTime).getTime();
        filtered = queries.filter(q => {
            const ts = q.capture_timestamp || q.Capture_Timestamp || '';
            if (!ts) return true;
            const d = new Date(ts).getTime();
            return d >= fromMs && d <= toMs;
        });
    }

    const excludedDbs = ['master', 'model', 'msdb', 'distribution'];
    filtered = filtered.filter(q => {
        const dbName = (q.database_name || q.Database_Name || q['database_name'] || '').toLowerCase();
        return !excludedDbs.includes(dbName);
    });

    function normalizeQuery(qt) {
        if (!qt) return '';
        return qt.replace(/'[^']*'/g, "'?'").replace(/\b\d+(\.\d+)?\b/g, '?').replace(/\s+/g, ' ').trim().substring(0, 300);
    }

    const groups = new Map();
    filtered.forEach(q => {
        const rawText = q.query_text || q.Query_Text || 'Unknown';
        const norm = normalizeQuery(rawText);
        const dbName = q.database_name || q.Database_Name || 'Unknown';
        // TimescaleDB returns: login_name, program_name, total_cpu_time_ms, total_execution_count, total_exec_time_ms
        // Live endpoint returns: Login_Name, Client_App, Total_CPU_ms, Executions
        const loginName   = q.login_name   || q.Login_Name   || 'Unknown/Disconnected';
        const programName = q.program_name || q.Client_App   || q.Program_Name || 'Unknown/Disconnected';
        const totalCpu    = parseFloat(q.total_cpu_time_ms  || q.cpu_time_ms    || q.Total_CPU_ms  || q.total_cpu_ms  || 0);
        const execTime    = parseFloat(q.total_exec_time_ms || q.exec_time_ms   || q.Total_Elapsed_ms || 0);
        const logicalReads = parseInt(q.total_logical_reads || q.logical_reads   || q.Total_Logical_Reads || 0);
        const executionCount = parseInt(q.total_execution_count || q.execution_count || q.Executions || 1);
        // Avg CPU: TimescaleDB pre-aggregates totals, so derive avg here
        const avgCpu      = executionCount > 0 ? (totalCpu / executionCount) : parseFloat(q.avg_cpu_ms || q.Avg_CPU_ms || 0);
        const captureTs   = q.capture_timestamp || q.last_capture || null;

        const key = dbName + '|||' + norm;
        if (!groups.has(key)) {
            groups.set(key, {
                queryText: rawText,
                dbName: dbName,
                loginName: loginName,
                programName: programName,
                totalCpu: totalCpu,
                execTime: execTime,
                logicalReads: logicalReads,
                maxExecs: executionCount,
                avgCpu: avgCpu,
                capture_timestamp: captureTs
            });
        } else {
            const g = groups.get(key);
            g.totalCpu     += totalCpu;
            g.execTime     += execTime;
            g.logicalReads += logicalReads;
            g.maxExecs     += executionCount;
            if (captureTs && !g.capture_timestamp) g.capture_timestamp = captureTs;
            // Always keep the latest query text sample
            if (totalCpu > 0) g.queryText = rawText;
        }
    });

    const sorted = Array.from(groups.values()).sort((a, b) => b.totalCpu - a.totalCpu);
    document.getElementById('queryCount').textContent = sorted.length + ' unique queries';

    window.appState.queryCache = {};
    tbody.innerHTML = sorted.map((g, idx) => {
        const avgCpu = g.maxExecs > 0 ? (g.totalCpu / g.maxExecs) : 0;
        const cpuClass = g.totalCpu > 5000 ? 'text-danger' : g.totalCpu > 1000 ? 'text-warning' : '';
        const truncatedText = g.queryText.length > 80 ? g.queryText.substring(0, 80) + '...' : g.queryText;
        window.appState.queryCache['q' + idx] = g.queryText;
        
        const ts = g.capture_timestamp ? new Date(g.capture_timestamp) : null;
        const tsStr = ts ? ts.toLocaleTimeString('en-US', {hour: '2-digit', minute: '2-digit'}) : '';

        return `
            <tr>
                <td>${idx + 1}</td>
                <td><span class="badge badge-outline">${tsStr}</span></td>
                <td><span class="badge badge-info">${window.escapeHtml(g.dbName)}</span></td>
                <td style="max-width: 350px;">
                    <span class="code-snippet" style="cursor: pointer; display: inline-block; max-width: 330px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 2px 6px; border-radius: 4px; font-size: 0.75rem;" 
                          title="${window.escapeHtml(g.queryText)}"
                          onclick="window.showQueryModalDirect(window.appState.queryCache['q${idx}'])">
                        ${window.escapeHtml(truncatedText)}
                    </span>
                </td>
                <td class="${cpuClass}"><strong>${g.totalCpu.toFixed(2)}</strong></td>
                <td>${avgCpu.toFixed(2)}</td>
                <td>${g.maxExecs.toLocaleString()}</td>
                <td style="font-size:0.7rem; max-width:120px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(g.loginName)}">${window.escapeHtml(g.loginName)}</td>
                <td style="font-size:0.7rem; max-width:120px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(g.programName)}">${window.escapeHtml(g.programName)}</td>
            </tr>
        `;
    }).join('');
};

window.renderCpuDrilldownDashboardTable = function(queries, fromTime, toTime) {
    const tbody = document.getElementById('cpuQueriesBody');
    if (!tbody) return;
    const metric = window.appState.currentQueryMetric || 'cpu';

    let filtered = queries;
    if (fromTime && toTime) {
        const fromMs = new Date(fromTime).getTime();
        const toMs = new Date(toTime).getTime();
        filtered = queries.filter(q => {
            const ts = q.capture_timestamp || q.last_capture || '';
            if (!ts) return true;
            const d = new Date(ts).getTime();
            return d >= fromMs && d <= toMs;
        });
    }

    const excludedDbs = ['master', 'model', 'msdb', 'distribution'];
    filtered = filtered.filter(q => {
        const dbName = (q.database_name || q.Database_Name || '').toLowerCase();
        return !excludedDbs.includes(dbName);
    });

    const sorted = [...filtered].sort((a, b) => (parseFloat(b.metric_value) || 0) - (parseFloat(a.metric_value) || 0));
    document.getElementById('queryCount').textContent = sorted.length + ' queries';

    window.appState.queryCache = {};
    tbody.innerHTML = sorted.map((q, idx) => {
        const rawText = q.query_text || '';
        const dbName = q.database_name || '—';
        const loginName = q.login_name || 'Unknown/Disconnected';
        const programName = q.client_app || 'Unknown/Disconnected';
        const totalMetric = parseFloat(q.metric_value) || 0;
        const execs = parseInt(q.total_executions, 10) || 0;
        let avgCol = 0;
        if (metric === 'cpu') avgCol = parseFloat(q.avg_cpu_ms) || 0;
        else if (metric === 'duration') avgCol = parseFloat(q.avg_duration_ms) || 0;
        else if (metric === 'reads') avgCol = parseFloat(q.avg_reads) || 0;
        else if (metric === 'executions') avgCol = parseFloat(q.avg_cpu_ms) || 0;

        const truncatedText = rawText.length > 80 ? rawText.substring(0, 80) + '...' : rawText;
        window.appState.queryCache['q' + idx] = rawText;

        return `
            <tr>
                <td>${idx + 1}</td>
                <td><span class="badge badge-outline">—</span></td>
                <td><span class="badge badge-info">${window.escapeHtml(dbName)}</span></td>
                <td style="max-width: 350px;">
                    <span class="code-snippet" style="cursor: pointer; display: inline-block; max-width: 330px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 2px 6px; border-radius: 4px; font-size: 0.75rem;"
                          title="${window.escapeHtml(rawText)}"
                          onclick="window.showQueryModalDirect(window.appState.queryCache['q${idx}'])">
                        ${window.escapeHtml(truncatedText)}
                    </span>
                </td>
                <td><strong>${totalMetric.toLocaleString(undefined, { maximumFractionDigits: 2 })}</strong></td>
                <td>${avgCol.toLocaleString(undefined, { maximumFractionDigits: 2 })}</td>
                <td>${execs.toLocaleString()}</td>
                <td style="font-size:0.7rem; max-width:120px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(loginName)}">${window.escapeHtml(loginName)}</td>
                <td style="font-size:0.7rem; max-width:120px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(programName)}">${window.escapeHtml(programName)}</td>
            </tr>
        `;
    }).join('');
};

window.updateCpuDrilldownTimestamp = function() {
    const el = document.getElementById('cpuDrilldownLastUpdate');
    if (el) {
        el.textContent = 'Updated: ' + new Date().toLocaleTimeString();
    }
};

window.refreshCpuDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    const fromInput = document.getElementById('cpuDrillFrom');
    const toInput = document.getElementById('cpuDrillTo');
    const fromVal = fromInput ? fromInput.value : '';
    const toVal = toInput ? toInput.value : '';
    await Promise.all([
        window.loadCpuDrilldownChartOnly(inst.name, fromVal, toVal),
        window.loadQueryStatsWithMetric(inst.name, window.appState.currentQueryMetric || 'cpu', '1h', fromVal, toVal)
    ]);
};

window.applyCpuDrilldownRange = async function() {
    const fromInput = document.getElementById('cpuDrillFrom');
    const toInput = document.getElementById('cpuDrillTo');
    
    if (!fromInput || !toInput) return;
    
    const fromTime = fromInput.value;
    const toTime = toInput.value;
    const err = window.getDatetimeLocalRangeError && window.getDatetimeLocalRangeError(fromTime, toTime);
    if (err) {
        window.showDateRangeValidationError(err);
        return;
    }

    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (inst) {
        await Promise.all([
            window.loadCpuDrilldownChartOnly(inst.name, fromTime, toTime),
            window.loadQueryStatsWithMetric(inst.name, window.appState.currentQueryMetric || 'cpu', '1h', fromTime, toTime)
        ]);
    }
};

window.showQueryModalDirect = function(queryText) {
    if (!queryText) {
        queryText = 'No query text available';
    }
    
    const existingModal = document.getElementById('query-modal');
    if (existingModal) existingModal.remove();

    const modal = document.createElement('div');
    modal.id = 'query-modal';
    modal.style.cssText = 'display: flex; position: fixed; z-index: 99999; left: 0; top: 0; width: 100%; height: 100%; background-color: rgba(0,0,0,0.8); align-items: center; justify-content: center;';
    
    const safeText = window.escapeHtml(queryText);
    
    modal.innerHTML = `
        <div style="background: var(--bg-surface); margin: 2%; padding: 20px; border: 1px solid var(--border-color, #333); border-radius: 12px; width: 95%; max-width: 1000px; max-height: 90vh; overflow-y: auto; color: var(--text-primary, #e0e0e0); font-family: inherit; box-shadow: 0 4px 20px rgba(0,0,0,0.5);">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; border-bottom: 1px solid var(--border-color, #333); padding-bottom: 0.75rem;">
                <h3 style="margin: 0; color: var(--accent, #3b82f6); font-size: 1.1rem;"><i class="fa-solid fa-code"></i> Query Details</h3>
                <button onclick="document.getElementById('query-modal').remove()" style="background: transparent; border: 1px solid var(--border-color, #555); color: var(--text-primary, #e0e0e0); font-size: 1.25rem; cursor: pointer; padding: 0.25rem 0.6rem; border-radius: 4px; line-height: 1;">&times;</button>
            </div>
            <div style="background: var(--bg-base); padding: 1rem; border-radius: 8px; max-height: 60vh; overflow: auto; border: 1px solid var(--border-color, #333);">
                <pre id="queryModalText" style="margin: 0; white-space: pre-wrap; word-wrap: break-word; color: var(--text-primary, #e0e0e0); font-family: 'Courier New', monospace; font-size: 0.85rem; line-height: 1.5;"></pre>
            </div>
            <div style="text-align: center; margin-top: 1rem;">
                <button id="copySqlBtnDirect" style="background: var(--accent, #3b82f6); color: #fff; border: none; padding: 0.5rem 1.5rem; border-radius: 6px; cursor: pointer; font-size: 0.9rem;">
                    <i class="fa-solid fa-copy"></i> copy SQL
                </button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
    
    document.getElementById('queryModalText').textContent = queryText;
    
    document.getElementById('copySqlBtnDirect').addEventListener('click', function() {
        navigator.clipboard.writeText(queryText).then(() => {
            this.innerHTML = '<i class="fa-solid fa-check"></i> copied!';
            setTimeout(() => {
                this.innerHTML = '<i class="fa-solid fa-copy"></i> copy SQL';
            }, 1500);
        });
    });

    modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.remove();
    });
};
