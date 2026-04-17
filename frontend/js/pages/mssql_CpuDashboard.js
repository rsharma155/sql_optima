/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: CPU utilization dashboard with scheduler statistics and CPU history.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.MssqlCpuDashboardView = async function() {
    appDebug('[CPU Dashboard] Starting');
    if (!window.appState.config || !window.appState.config.instances || window.appState.config.instances.length === 0) {
        window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">Please select an instance first</h3></div>`;
        return;
    }
    
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || typeof inst !== 'object' || !inst.name || inst.type !== 'sqlserver') {
        window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">Please select a SQL Server instance</h3></div>`;
        return;
    }
    
    window.appState.currentInstanceName = inst.name;
    appDebug('[CPU Dashboard] Loading metrics for instance:', inst.name);

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1>CPU Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)} | CPU Scheduler & Memory</p>
                </div>
                <div class="flex-between" style="align-items:center; gap:1rem;">
                    <div style="text-align:right;">
                        <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="cpuDashboardLastUpdate">--:--:--</span></span>
                        <span class="text-muted" style="font-size:0.65rem; display:block;">Auto-refresh: every 60s</span>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshMssqlCpuDashboard"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            <div style="display:flex; justify-content:center; align-items:center; height:50vh;">
                <div class="spinner"></div><span style="margin-left:1rem;">Loading CPU metrics...</span>
            </div>
        </div>
    `;

    await window.loadMssqlCpuDashboard();
};

window.loadMssqlCpuDashboard = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'sqlserver') {
        return;
    }

    appDebug('[CPU Dashboard] Instance:', inst.name);

    try {
        const limit = 100;
        
        const [propertiesRes, schedulerRes] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/mssql/server-properties?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/mssql/cpu-scheduler-stats?instance=${encodeURIComponent(inst.name)}&limit=${limit}`)
        ]);

        appDebug('[CPU Dashboard] Properties response status:', propertiesRes.status);
        appDebug('[CPU Dashboard] Scheduler response status:', schedulerRes.status);

        let serverProps = null;
        let schedulerStats = [];

        if (propertiesRes.ok) {
            const propsData = await propertiesRes.json();
            appDebug('[CPU Dashboard] Full propsData:', propsData);
            serverProps = propsData.server_properties || null;
            appDebug('[CPU Dashboard] Server props:', serverProps);
        } else {
            const errText = await propertiesRes.text();
            appDebug('[CPU Dashboard] Properties fetch failed:', propertiesRes.status, errText);
        }

        if (schedulerRes.ok) {
            const schedData = await schedulerRes.json();
            schedulerStats = schedData.cpu_scheduler_stats || [];
            appDebug('[CPU Dashboard] Scheduler stats:', schedulerStats.length, 'records');
        }

        window.renderCpuDashboard(inst, serverProps, schedulerStats);
        document.getElementById('cpuDashboardLastUpdate').textContent = new Date().toLocaleTimeString();

    } catch (err) {
        appDebug('[CPU Dashboard] Error loading data:', err);
    }
};

function renderCpuDashboard(inst, serverProps, schedulerStats) {
    const currentStats = schedulerStats.length > 0 ? schedulerStats[0] : null;
    const hasData = currentStats !== null;
    
    appDebug('[CPU Dashboard] Data received - schedulerStats:', schedulerStats.length, 'hasData:', hasData);
    
    const workerPct = currentStats?.max_workers_count > 0 ? ((currentStats.total_current_workers_count / currentStats.max_workers_count) * 100).toFixed(1) : 0;
    const memUsedPct = currentStats?.total_physical_memory_kb > 0 
        ? (((currentStats.total_physical_memory_kb - currentStats.available_physical_memory_kb) / currentStats.total_physical_memory_kb) * 100).toFixed(1) 
        : 0;
    
    const memIsDanger = memUsedPct > 95;
    const memIsWarning = memUsedPct > 85 && memUsedPct <= 95;
    const workerIsDanger = workerPct > 90;
    const workerIsWarning = workerPct > 70 && workerPct <= 90;

    const warnings = [];
    if (currentStats?.worker_thread_exhaustion_warning) warnings.push({ label: 'Worker Exhaustion', class: 'badge-danger' });
    if (currentStats?.runnable_tasks_warning) warnings.push({ label: 'High Runnable', class: 'badge-warning' });
    if (currentStats?.physical_memory_pressure_warning) warnings.push({ label: 'Memory Pressure', class: 'badge-danger' });
    if (currentStats?.offline_cpu_warning) warnings.push({ label: 'Offline CPU', class: 'badge-danger' });

    const memGB = serverProps?.physical_memory_gb ? serverProps.physical_memory_gb.toFixed(1) : 'N/A';
    const htEnabled = serverProps?.hyperthread_enabled ? '<span class="badge badge-success">Enabled</span>' : '<span class="badge badge-warning">Disabled</span>';

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1>CPU Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Database: <span class="text-accent">CPU Metrics</span></p>
                </div>
                <div class="flex-between" style="align-items:center; gap:1rem;">
                    <div style="text-align:right;">
                        <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="cpuDashboardLastUpdate">${new Date().toLocaleTimeString()}</span></span>
                        <span class="text-muted" style="font-size:0.65rem; display:block;">Auto-refresh: every 60s</span>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshMssqlCpuDashboard"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>

            <div class="metrics-row" style="display:grid; grid-template-columns:1fr 1fr; gap:0.75rem; margin-top:0.75rem;">
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-server"></i> Server Hardware Properties</h4>
                    <div style="display:flex; gap:0.5rem; flex-wrap:wrap;">
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">CPU Cores</span></div>
                            <div class="metric-value" style="font-size:1rem">${serverProps?.cpu_count || 'N/A'}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Cores/Socket</span></div>
                            <div class="metric-value" style="font-size:1rem">${serverProps?.cores_per_socket || 'N/A'}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Sockets</span></div>
                            <div class="metric-value" style="font-size:1rem">${serverProps?.socket_count || 'N/A'}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Hyperthread</span></div>
                            <div class="metric-value" style="font-size:0.9rem">${htEnabled}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">NUMA Nodes</span></div>
                            <div class="metric-value" style="font-size:1rem">${serverProps?.numa_nodes || 'N/A'}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Max Workers</span></div>
                            <div class="metric-value" style="font-size:1rem">${serverProps?.max_workers_count || 'N/A'}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Memory GB</span></div>
                            <div class="metric-value" style="font-size:1rem">${memGB}</div>
                        </div>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-microchip"></i> CPU Scheduler Status</h4>
                    <div style="display:flex; gap:0.5rem; flex-wrap:wrap;">
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Schedulers</span></div>
                            <div class="metric-value" style="font-size:1rem">${currentStats?.scheduler_count || 'N/A'}</div>
                        </div>
                        <div class="metric-card glass-panel ${workerIsDanger?'status-danger':(workerIsWarning?'status-warning':'status-healthy')}" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Workers</span></div>
                            <div class="metric-value" style="font-size:1rem">${currentStats?.total_current_workers_count || 0}/${currentStats?.max_workers_count || 0} (${workerPct}%)</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Runnable Tasks</span></div>
                            <div class="metric-value" style="font-size:1rem">${currentStats?.total_runnable_tasks_count || 0}</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Work Queue</span></div>
                            <div class="metric-value" style="font-size:1rem">${currentStats?.total_work_queue_count || 0}</div>
                        </div>
                        <div class="metric-card glass-panel ${memIsDanger?'status-danger':(memIsWarning?'status-warning':'status-healthy')}" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Memory Used</span></div>
                            <div class="metric-value" style="font-size:1rem">${memUsedPct}%</div>
                        </div>
                        <div class="metric-card glass-panel status-healthy" style="padding:0.4rem 0.75rem; flex:1; min-width:100px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">Warnings</span></div>
                            <div class="metric-value" style="font-size:0.9rem">${warnings.length > 0 ? warnings.map(w => `<span class="${w.class}">${w.label}</span>`).join(' ') : '<span class="badge badge-success">None</span>'}</div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="charts-grid mt-3" data-cols="4" style="display:grid; grid-template-columns:repeat(4,1fr); gap:0.75rem;">
                <div class="chart-card glass-panel" style="padding:0.75rem; grid-column: span 2;">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-chart-line"></i> CPU Scheduler History</h3><span class="badge badge-info">${schedulerStats.length} samples</span></div>
                    <div class="chart-container" style="height:180px;"><canvas id="cpuSchedulerHistoryChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="padding:0.75rem; grid-column: span 2;">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-memory"></i> Memory Pressure</h3></div>
                    <div class="chart-container" style="height:180px;"><canvas id="cpuMemoryChart"></canvas></div>
                </div>
            </div>

            <div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                <div class="table-card glass-panel">
                    <div class="card-header" style="flex-wrap:wrap; gap:0.35rem;">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-list text-accent"></i> Recent CPU Scheduler Stats</h3>
                        <span class="badge badge-info">${schedulerStats.length} records</span>
                    </div>
                    <p class="text-muted" style="font-size:0.7rem; margin:0 0 0.5rem 0; line-height:1.4;">
                        Each row is a <strong>point-in-time snapshot</strong> of SQL Server scheduler pressure (CPU schedulers, worker usage, runnable tasks, work queues, and OS memory signals). Use it to spot sustained runnable backlog or worker exhaustion. Identical snapshots across polls are not stored again, so the list emphasizes <strong>changes</strong> in engine state.
                    </p>
                    <div class="table-responsive" style="max-height:300px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead>
                                <tr>
                                    <th>Timestamp</th>
                                    <th>CPUs</th>
                                    <th>Schedulers</th>
                                    <th>Max Workers</th>
                                    <th>Workers</th>
                                    <th>Runnable Tasks</th>
                                    <th>Work Queue</th>
                                    <th>Memory %</th>
                                    <th>Warnings</th>
                                </tr>
                            </thead>
                            <tbody id="cpuSchedulerTableBody">
                                <tr><td colspan="9" class="text-center text-muted">Loading...</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    `;

    initCpuDashboardCharts(schedulerStats);
    renderCpuSchedulerTable(schedulerStats);
}

function initCpuDashboardCharts(schedulerStats) {
    if (window.currentCharts && window.currentCharts.cpuSchedulerHistory) window.currentCharts.cpuSchedulerHistory.destroy();
    if (window.currentCharts && window.currentCharts.cpuMemory) window.currentCharts.cpuMemory.destroy();
    window.currentCharts = window.currentCharts || {};

    const ctxHistory = document.getElementById('cpuSchedulerHistoryChart')?.getContext('2d');
    if (ctxHistory && schedulerStats.length > 0) {
        const labels = schedulerStats.slice().reverse().map(s => {
            const d = new Date(s.capture_timestamp);
            return d.toLocaleTimeString();
        });
        const workerData = schedulerStats.slice().reverse().map(s => s.total_current_workers_count || 0);
        const runnableData = schedulerStats.slice().reverse().map(s => s.total_runnable_tasks_count || 0);
        const queueData = schedulerStats.slice().reverse().map(s => s.total_work_queue_count || 0);

        window.currentCharts.cpuSchedulerHistory = new Chart(ctxHistory, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [
                    {
                        label: 'Current Workers',
                        data: workerData,
                        borderColor: window.getCSSVar('--primary'),
                        backgroundColor: 'rgba(99, 102, 241, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0
                    },
                    {
                        label: 'Runnable Tasks',
                        data: runnableData,
                        borderColor: window.getCSSVar('--warning'),
                        backgroundColor: 'rgba(245, 158, 11, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0
                    },
                    {
                        label: 'Work Queue',
                        data: queueData,
                        borderColor: window.getCSSVar('--danger'),
                        backgroundColor: 'rgba(239, 68, 68, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'top', labels: { boxWidth: 8, font: { size: 10 } } } },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'Count' } },
                    x: { title: { display: true, text: 'Time' }, ticks: { maxTicksLimit: 10 } }
                }
            }
        });
    }

    const ctxMemory = document.getElementById('cpuMemoryChart')?.getContext('2d');
    if (ctxMemory && schedulerStats.length > 0) {
        const labels = schedulerStats.slice().reverse().map(s => {
            const d = new Date(s.capture_timestamp);
            return d.toLocaleTimeString();
        });
        const memData = schedulerStats.slice().reverse().map(s => {
            const total = s.total_physical_memory_kb || 1;
            const avail = s.available_physical_memory_kb || 0;
            return ((total - avail) / total * 100);
        });

        window.currentCharts.cpuMemory = new Chart(ctxMemory, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [{
                    label: 'Memory Used %',
                    data: memData,
                    borderColor: window.getCSSVar('--accent-blue'),
                    backgroundColor: 'rgba(59, 130, 246, 0.2)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'top', labels: { boxWidth: 8, font: { size: 10 } } } },
                scales: {
                    y: { min: 0, max: 100, title: { display: true, text: 'Memory %' } },
                    x: { title: { display: true, text: 'Time' }, ticks: { maxTicksLimit: 10 } }
                }
            }
        });
    }
}

function renderCpuSchedulerTable(stats) {
    const tbody = document.getElementById('cpuSchedulerTableBody');
    if (!tbody) return;

    if (!stats || stats.length === 0) {
        tbody.innerHTML = '<tr><td colspan="9" class="text-center text-muted">No data available</td></tr>';
        return;
    }

    tbody.innerHTML = stats.slice(0, 50).map(s => {
        const warnings = [];
        if (s.worker_thread_exhaustion_warning) warnings.push('W');
        if (s.runnable_tasks_warning) warnings.push('R');
        if (s.physical_memory_pressure_warning) warnings.push('M');
        if (s.offline_cpu_warning) warnings.push('O');

        const warningClass = warnings.length > 0 ? 'badge-warning' : 'badge-success';
        const warningText = warnings.length > 0 ? warnings.join(', ') : 'None';

        const totalMem = s.total_physical_memory_kb || 1;
        const availMem = s.available_physical_memory_kb || 0;
        const memPct = ((totalMem - availMem) / totalMem * 100).toFixed(1);

        return `
            <tr>
                <td>${new Date(s.capture_timestamp).toLocaleString()}</td>
                <td>${s.cpu_count || '-'}</td>
                <td>${s.scheduler_count || '-'}</td>
                <td>${s.max_workers_count || '-'}</td>
                <td>${s.total_current_workers_count || 0}</td>
                <td>${s.total_runnable_tasks_count || 0}</td>
                <td>${s.total_work_queue_count || 0}</td>
                <td>${memPct}%</td>
                <td><span class="badge ${warningClass}">${warningText}</span></td>
            </tr>
        `;
    }).join('');
}

window.refreshMssqlCpuDashboard = function() {
    window.MssqlCpuDashboardView();
};