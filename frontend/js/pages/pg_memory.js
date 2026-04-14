/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL memory usage dashboard.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgMemoryView = async function() {
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_memory.html');
    setTimeout(initPgMemory, 50);
}

async function initPgMemory() {
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
        console.error("PG Memory data fetch failed:", e);
    }

    updateMemoryMetrics(systemStats, overviewData);
    createMemoryCharts(systemStats, overviewData, sysHistory);
    updateMemoryDetailsTable(systemStats, overviewData, sysHistory);
}

function updateMemoryMetrics(systemStats, overviewData) {
    const fmtBytes = (b) => {
        const n = Number(b || 0);
        if (!isFinite(n) || n <= 0) return 'N/A';
        const gb = n / 1024 / 1024 / 1024;
        if (gb >= 1) return gb.toFixed(1) + ' GB';
        const mb = n / 1024 / 1024;
        return mb.toFixed(0) + ' MB';
    };

    const osMemoryEl = document.getElementById('os-memory-current');
    const osMemoryTrendEl = document.getElementById('os-memory-trend');
    if (systemStats.memory_usage !== undefined) {
        const usedPct = Number(systemStats.memory_usage || 0);
        const total = Number(systemStats.total_memory_bytes || 0);
        const avail = Number(systemStats.available_memory_bytes || 0);
        const usedBytes = total > 0 ? Math.max(0, total - avail) : 0;
        osMemoryEl.textContent = total > 0 ? `${fmtBytes(usedBytes)} / ${fmtBytes(total)} (${usedPct.toFixed(1)}%)` : usedPct.toFixed(1) + '%';
        osMemoryTrendEl.innerHTML = '<i class="fa-solid fa-chart-line"></i> Host memory used';
    } else {
        osMemoryEl.textContent = 'N/A';
        osMemoryTrendEl.innerHTML = '<i class="fa-solid fa-exclamation-triangle"></i> Data unavailable';
    }

    const pgMemoryEl = document.getElementById('pg-memory-current');
    const pgMemoryTrendEl = document.getElementById('pg-memory-trend');
    const sharedBytes = Number(systemStats.shared_buffers_bytes || 0);
    const totalBytes = Number(systemStats.total_memory_bytes || 0);
    const pct = totalBytes > 0 ? (sharedBytes / totalBytes) * 100 : 0;
    pgMemoryEl.textContent = sharedBytes > 0 ? `${fmtBytes(sharedBytes)} (${pct.toFixed(1)}%)` : 'N/A';
    pgMemoryTrendEl.innerHTML = '<i class="fa-solid fa-database"></i> shared_buffers configured';

    const totalMemoryEl = document.getElementById('total-memory');
    const totalMemoryTrendEl = document.getElementById('total-memory-trend');
    const totalMemCard = totalMemoryEl ? totalMemoryEl.closest('.metric-card') : null;
    if (totalMemoryEl) {
        totalMemoryEl.textContent = totalBytes > 0 ? fmtBytes(totalBytes) : 'N/A';
    }
    if (totalMemoryTrendEl) {
        totalMemoryTrendEl.innerHTML = totalBytes > 0 ? '<i class="fa-solid fa-info"></i> Host total memory' : '';
    }
    if (totalMemCard) {
        totalMemCard.style.display = totalBytes > 0 ? '' : 'none';
    }

    const cacheHitEl = document.getElementById('cache-hit-ratio');
    const cacheHitTrendEl = document.getElementById('cache-hit-trend');
    if (overviewData.last_cache_hit_pct !== undefined) {
        cacheHitEl.textContent = overviewData.last_cache_hit_pct.toFixed(1) + '%';
        cacheHitTrendEl.innerHTML = '<i class="fa-solid fa-chart-line"></i> PostgreSQL cache hit ratio';
    } else {
        cacheHitEl.textContent = 'N/A';
        cacheHitTrendEl.innerHTML = '<i class="fa-solid fa-exclamation-triangle"></i> Data unavailable';
    }

    const osMemActiveEl = document.getElementById('os-mem-active');
    if (osMemActiveEl) {
        osMemActiveEl.textContent = systemStats.memory_usage !== undefined ? Number(systemStats.memory_usage || 0).toFixed(1) + '%' : 'N/A';
    }
    const pgMemShareEl = document.getElementById('pg-mem-share');
    if (pgMemShareEl) {
        pgMemShareEl.textContent = sharedBytes > 0 && totalBytes > 0 ? pct.toFixed(1) + '%' : 'N/A';
    }
}

function createMemoryCharts(systemStats, overviewData, sysHistory) {
    const rowsAsc = (sysHistory || []).slice().reverse(); // API returns DESC
    const timeLabels = rowsAsc.map(r => new Date(r.capture_timestamp).toLocaleTimeString());

    const memoryTimeSeriesCtx = document.getElementById('memoryTimeSeriesChart');
    if (memoryTimeSeriesCtx) {
        const osMemoryHistory = rowsAsc.map(r => Number(r.memory_usage || 0));
        const pgMemoryHistory = osMemoryHistory.map(v => v * 0.6);

        window.currentCharts.memoryTimeSeries = new Chart(memoryTimeSeriesCtx.getContext('2d'), {
            type: 'line',
            data: {
                labels: timeLabels,
                datasets: [
                    {
                        label: 'OS Memory %',
                        data: osMemoryHistory,
                        borderColor: window.getCSSVar('--accent-blue'),
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        fill: true,
                        tension: 0.4,
                        pointRadius: 0,
                    },
                    {
                        label: 'PostgreSQL Memory %',
                        data: pgMemoryHistory,
                        borderColor: window.getCSSVar('--success'),
                        backgroundColor: 'rgba(16, 185, 129, 0.1)',
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
                        title: { display: true, text: 'Memory Usage %' },
                        ticks: { callback: v => v + '%' }
                    },
                    x: {
                        title: { display: true, text: 'Time' }
                    }
                }
            }
        });
    }

    const osMemoryBreakdownCtx = document.getElementById('osMemoryBreakdownChart');
    if (osMemoryBreakdownCtx) {
        const osMemoryUsage = systemStats.memory_usage || 0;
        const freeMemory = Math.max(0, 100 - osMemoryUsage);

        window.currentCharts.osMemoryBreakdown = new Chart(osMemoryBreakdownCtx.getContext('2d'), {
            type: 'doughnut',
            data: {
                labels: ['Used', 'Free'],
                datasets: [{
                    data: [osMemoryUsage, freeMemory],
                    backgroundColor: [window.getCSSVar('--accent-blue'), window.getCSSVar('--text-muted')],
                    borderWidth: 0
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                cutout: '70%',
                plugins: {
                    legend: { position: 'bottom' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.label + ': ' + context.parsed.toFixed(1) + '%';
                            }
                        }
                    }
                }
            }
        });
    }

    const pgMemoryBreakdownCtx = document.getElementById('pgMemoryBreakdownChart');
    if (pgMemoryBreakdownCtx) {
        const pgMemoryEstimate = (systemStats.memory_usage || 0) * 0.6;
        const otherMemory = Math.max(0, (systemStats.memory_usage || 0) - pgMemoryEstimate);

        window.currentCharts.pgMemoryBreakdown = new Chart(pgMemoryBreakdownCtx.getContext('2d'), {
            type: 'doughnut',
            data: {
                labels: ['PostgreSQL', 'Other Processes'],
                datasets: [{
                    data: [pgMemoryEstimate, otherMemory],
                    backgroundColor: [window.getCSSVar('--success'), window.getCSSVar('--warning')],
                    borderWidth: 0
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                cutout: '70%',
                plugins: {
                    legend: { position: 'bottom' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.parsed.toFixed(1) + '%';
                            }
                        }
                    }
                }
            }
        });
    }
}

function updateMemoryDetailsTable(systemStats, overviewData, sysHistory) {
    const tbody = document.getElementById('memory-details-table');
    if (!tbody) return;

    const cacheHit = overviewData.last_cache_hit_pct || 0;
    const sharedBytes = Number(systemStats.shared_buffers_bytes || 0);
    const sharedStr = sharedBytes > 0 ? (sharedBytes / 1024 / 1024).toFixed(0) + ' MB' : 'N/A';

    const rowsAsc = (sysHistory || []).slice().reverse().slice(-10);
    const rows = rowsAsc.map((r) => {
        const memUsage = Number(r.memory_usage || 0);
        const time = r.capture_timestamp ? new Date(r.capture_timestamp).toLocaleString() : new Date().toLocaleString();
        return `
            <tr>
                <td>${time}</td>
                <td>${memUsage.toFixed(1)}%</td>
                <td>${(memUsage * 0.6).toFixed(1)}%</td>
                <td>${cacheHit.toFixed(1)}%</td>
                <td>${sharedStr}</td>
            </tr>
        `;
    });

    tbody.innerHTML = rows.join('');
}
