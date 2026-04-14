/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL CPU utilization dashboard.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgCpuView = async function() {
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_cpu.html');
    setTimeout(initPgCpu, 50);
}

async function initPgCpu() {
    window.currentCharts = window.currentCharts || {};
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: ''};

    document.getElementById('last-updated').textContent = new Date().toLocaleString();

    let systemStats = {};
    let overviewData = {};
    let sysHistory = [];

    try {
        const systemResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/system-stats?instance=${encodeURIComponent(inst.name)}`
        );
        if (systemResponse.ok) {
            const contentType = systemResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                systemStats = await systemResponse.json();
            }
        }

        const overviewResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/overview?instance=${encodeURIComponent(inst.name)}`
        );
        if (overviewResponse.ok) {
            const contentType = overviewResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                overviewData = await overviewResponse.json();
            }
        }

        const historyResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/system-stats/history?instance=${encodeURIComponent(inst.name)}&limit=60`
        );
        if (historyResponse.ok) {
            const contentType = historyResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const payload = await historyResponse.json();
                sysHistory = payload.stats || [];
            }
        } else console.error("Failed to load PG system history:", historyResponse.status);
    } catch (e) {
        console.error("PG CPU data fetch failed:", e);
    }

    updateCpuMetrics(systemStats, overviewData);
    createCpuCharts(systemStats, overviewData, sysHistory);
    updateCpuDetailsTable(systemStats, overviewData, sysHistory);
}

function updateCpuMetrics(systemStats, overviewData) {
    const osCpuEl = document.getElementById('os-cpu-current');
    const osCpuTrendEl = document.getElementById('os-cpu-trend');
    if (systemStats.cpu_usage !== undefined) {
        osCpuEl.textContent = systemStats.cpu_usage.toFixed(1) + '%';
        osCpuTrendEl.innerHTML = '<i class="fa-solid fa-chart-line"></i> Current OS CPU usage';
    } else {
        osCpuEl.textContent = 'N/A';
        osCpuTrendEl.innerHTML = '<i class="fa-solid fa-exclamation-triangle"></i> Data unavailable';
    }

    const connectionsEl = document.getElementById('active-connections');
    const connectionsTrendEl = document.getElementById('connections-trend');
    if (overviewData.active_connections !== undefined) {
        connectionsEl.textContent = overviewData.active_connections;
        connectionsTrendEl.innerHTML = '<i class="fa-solid fa-users"></i> Active database connections';
    } else {
        connectionsEl.textContent = '0';
        connectionsTrendEl.innerHTML = '<i class="fa-solid fa-users"></i> No active connections';
    }

    const osCpuActiveEl = document.getElementById('os-cpu-active');
    if (osCpuActiveEl) {
        osCpuActiveEl.textContent = systemStats.cpu_usage !== undefined ? systemStats.cpu_usage.toFixed(1) + '%' : 'N/A';
    }
}

function createCpuCharts(systemStats, overviewData, sysHistory) {
    const rowsAsc = (sysHistory || []).slice().reverse(); // API returns DESC
    const timeLabels = rowsAsc.map(r => new Date(r.capture_timestamp).toLocaleTimeString());

    const cpuTimeSeriesCtx = document.getElementById('cpuTimeSeriesChart');
    if (cpuTimeSeriesCtx) {
        const osCpuHistory = rowsAsc.map(r => Number(r.cpu_usage || 0));

        window.currentCharts.cpuTimeSeries = new Chart(cpuTimeSeriesCtx.getContext('2d'), {
            type: 'line',
            data: {
                labels: timeLabels,
                datasets: [
                    {
                        label: 'OS CPU %',
                        data: osCpuHistory,
                        borderColor: window.getCSSVar('--accent-blue'),
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        fill: true,
                        tension: 0.4,
                        pointRadius: 0,
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: { mode: 'index', intersect: false },
                plugins: { legend: { position: 'top' } },
                scales: {
                    y: {
                        beginAtZero: true,
                        max: 100,
                        title: { display: true, text: 'CPU Usage %' },
                        ticks: { callback: v => v + '%' }
                    },
                    x: {
                        title: { display: true, text: 'Time' }
                    }
                }
            }
        });
    }
}

function updateCpuDetailsTable(systemStats, overviewData, sysHistory) {
    const tbody = document.getElementById('cpu-details-table');
    if (!tbody) return;

    const rowsAsc = (sysHistory || []).slice().reverse().slice(-10);
    const activeConns = overviewData.active_connections || 0;
    const rows = rowsAsc.map((r) => {
        const cpuUsage = Number(r.cpu_usage || 0);
        const time = r.capture_timestamp ? new Date(r.capture_timestamp).toLocaleString() : new Date().toLocaleString();
        return `
            <tr>
                <td>${time}</td>
                <td>${cpuUsage.toFixed(1)}%</td>
                <td>${activeConns}</td>
            </tr>
        `;
    });

    tbody.innerHTML = rows.join('');
}
