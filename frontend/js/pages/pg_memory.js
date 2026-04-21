/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Enhanced PostgreSQL Memory Intelligence Dashboard (Mission Control Cockpit Edition).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgMemoryView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: '', type: 'postgres'};
    if (!inst.name || inst.type !== 'postgres') {
        window.routerOutlet.innerHTML = '<div class="p-4 text-warning">Please select a PostgreSQL instance first.</div>';
        return;
    }

    // Default time window (last 6 hours)
    if (!window.appState.pgMemFrom) {
        const now = new Date();
        const past = new Date(now.getTime() - (6 * 3600000));
        window.appState.pgMemFrom = toLocalISOString(past);
        window.appState.pgMemTo = toLocalISOString(now);
    }

    window.routerOutlet.innerHTML = await window.loadTemplate('pages/pg_memory.html');
    
    // Set initial values
    const fromEl = document.getElementById('pg-mem-from');
    const toEl = document.getElementById('pg-mem-to');
    const labelEl = document.getElementById('pgss-instance-label');

    if (fromEl) fromEl.value = window.appState.pgMemFrom;
    if (toEl) toEl.value = window.appState.pgMemTo;
    if (labelEl) labelEl.textContent = inst.name;

    // Bind Controls
    document.getElementById('pg-mem-apply')?.addEventListener('click', () => {
        window.appState.pgMemFrom = document.getElementById('pg-mem-from')?.value || '';
        window.appState.pgMemTo = document.getElementById('pg-mem-to')?.value || '';
        initPgMemoryCockpit(inst.name);
    });

    // Bind Tabs
    document.querySelectorAll('.cockpit-tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.cockpit-tab-btn').forEach(b => b.classList.remove('active'));
            document.querySelectorAll('.tab-pane').forEach(p => p.style.display = 'none');
            btn.classList.add('active');
            const target = document.getElementById(`cockpit-tab-${btn.dataset.tab}`);
            if (target) {
                target.style.display = (btn.dataset.tab === 'forecast' || btn.dataset.tab === 'raw') ? 'block' : 'grid';
            }
        });
    });

    initPgMemoryCockpit(inst.name);
}

async function initPgMemoryCockpit(instanceName) {
    window.currentCharts = window.currentCharts || {};
    const from = new Date(window.appState.pgMemFrom).toISOString();
    const to = new Date(window.appState.pgMemTo).toISOString();

    try {
        const url = `/api/postgres/memory/intelligence?instance=${encodeURIComponent(instanceName)}&from=${from}&to=${to}`;
        const response = await window.apiClient.authenticatedFetch(url);
        if (!response.ok) throw new Error("API failed");
        
        const data = await response.json();
        renderPgMemoryCockpit(data);
    } catch (e) {
        console.error("PG Memory Cockpit fetch failed:", e);
        const tbody = document.getElementById('pg-memory-raw-tbody');
        if (tbody) tbody.innerHTML = `<tr><td colspan="6" class="text-center text-danger">Error: ${e.message}</td></tr>`;
    }
}

function renderPgMemoryCockpit(data) {
    const series = data.time_series || [];
    const components = data.components || {};
    const osConfigured = data.os_collector_configured;
    
    // Add OS Collector Status Note below Date Picker
    const statusContainer = document.getElementById('os-collector-status-container');
    if (statusContainer) {
        if (!osConfigured) {
            statusContainer.innerHTML = '<div style="font-size: 0.7rem; color: var(--warning); margin-bottom: 0.2rem;"><i class="fa-solid fa-circle-exclamation"></i> OS Collector not configured. OS level metrics unavailable.</div>';
        } else {
            statusContainer.innerHTML = '<div style="font-size: 0.7rem; color: var(--success); margin-bottom: 0.2rem;"><i class="fa-solid fa-circle-check"></i> OS Collector Active</div>';
        }
    }

    if (series.length === 0) {
        const tbody = document.getElementById('pg-memory-raw-tbody');
        if (tbody) tbody.innerHTML = `<tr><td colspan="6" class="text-center text-muted">No memory data found for selected range.</td></tr>`;
        return;
    }

    const latest = series[series.length - 1];
    
    // 1. Status Row
    updateCockpitStatus(latest, osConfigured);

    // 2. Core Triad Charts
    renderHostPgTrendChart(series, osConfigured);
    renderCacheEfficiencyChart(series);
    renderPressureSwapChart(series, osConfigured);

    // 3. Tabbed Charts & Data
    renderPgComponentsChart(components);
    renderTempSpillChart(series);
    renderConnMemoryChart(series);
    updatePgMemoryTable(series);
    updateAdvisorContent(latest, components);
    updateForecast(series);

    // 4. Implement Cross-Highlighting
    setupChartSync();

    window._lastPgMemSeries = series;
}

function updateCockpitStatus(latest, osConfigured) {
    const setT = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
    
    if (osConfigured) {
        setT('os-memory-pct', (latest.memory_pressure_percent || 0).toFixed(1) + '%');
        const totalGB = (latest.total_mem_mb || 0) / 1024;
        const usedGB = (latest.used_mb || 0) / 1024;
        setT('os-memory-raw', `${usedGB.toFixed(1)} / ${totalGB > 0 ? totalGB.toFixed(1) : '--'} GB`);
        setT('swap-used-mb', (latest.swap_used_mb || 0).toLocaleString() + ' MB');
    } else {
        setT('os-memory-pct', '-- %');
        setT('os-memory-raw', 'N/A');
        setT('swap-used-mb', 'N/A');
    }
    
    setT('pg-rss-mb', (latest.postgres_rss_mb || 0).toLocaleString() + ' MB');
    if (osConfigured) {
        setT('pg-rss-pct', (latest.pg_memory_percent || 0).toFixed(1) + '% of Host');
    } else {
        setT('pg-rss-pct', 'Host RAM unknown');
    }
    
    setT('cache-hit-pct', (latest.cache_hit_ratio || 0).toFixed(1) + '%');
    
    const swapStatus = document.getElementById('swap-status');
    if (swapStatus) {
        if (!osConfigured) {
            swapStatus.textContent = 'N/A';
            swapStatus.className = 'text-muted';
        } else if (latest.swap_used_mb > 512) { swapStatus.textContent = 'Critical'; swapStatus.className = 'text-danger'; }
        else if (latest.swap_used_mb > 0) { swapStatus.textContent = 'Active'; swapStatus.className = 'text-warning'; }
        else { swapStatus.textContent = 'Healthy'; swapStatus.className = 'text-muted'; }
    }

    setT('temp-spill-rate', (latest.temp_spill_rate_mb_s || 0).toFixed(2) + ' MB/s');
    
    const pressure = document.getElementById('pressure-level');
    if (pressure) {
        if (!osConfigured) {
            pressure.textContent = 'N/A';
            pressure.style.color = 'var(--text-muted)';
        } else {
            const pVal = latest.memory_pressure_percent || 0;
            if (pVal > 90) { pressure.textContent = 'CRITICAL'; pressure.style.color = 'var(--danger)'; }
            else if (pVal > 75) { pressure.textContent = 'HIGH'; pressure.style.color = 'var(--warning)'; }
            else { pressure.textContent = 'LOW'; pressure.style.color = 'var(--success)'; }
        }
    }

    // Health Score
    const score = latest.health_score || 100;
    const valueEl = document.getElementById('health-score-value');
    const labelEl = document.getElementById('health-status-label');
    const bannerEl = document.getElementById('pg-memory-health-banner');

    if (valueEl) {
        valueEl.textContent = score;
        const color = score < 60 ? 'var(--danger)' : score < 90 ? 'var(--warning)' : 'var(--success)';
        valueEl.style.color = color;
        if (labelEl) {
            labelEl.textContent = score < 60 ? 'CRITICAL' : score < 90 ? 'WARNING' : 'HEALTHY';
            labelEl.style.color = color;
        }
        if (bannerEl) bannerEl.style.borderLeftColor = color;
    }

    setT('last-updated', new Date(latest.ts).toLocaleTimeString());
}

function updateAdvisorContent(latest, components) {
    const memAdvisor = document.getElementById('mem-advisor-content');
    const workmemAdvisor = document.getElementById('workmem-advisor-content');
    const connAdvisor = document.getElementById('conn-advisor-content');
    if (!memAdvisor || !workmemAdvisor) return;

    // Rule 1 & 2: Shared Buffers
    let memHtml = '<ul style="padding-left: 1rem; margin: 0;">';
    const totalMem = latest.total_mem_mb || 0;
    const sbPct = totalMem > 0 ? (components.shared_buffers_mb / totalMem) * 100 : 0;

    if (totalMem === 0) {
        memHtml += '<li class="text-muted">OS level metrics unavailable. Host-aware memory rules skipped.</li>';
    } else if (latest.cache_hit_ratio < 95 && sbPct < 25) {
        memHtml += `<li class="text-warning"><strong>Rule 1:</strong> Cache hit ratio is suboptimal (${(latest.cache_hit_ratio || 0).toFixed(1)}%). Increase <code>shared_buffers</code> to 25–40% of RAM.</li>`;
    } else if (latest.memory_pressure_percent > 85 && latest.swap_used_mb > 0) {
        memHtml += '<li class="text-danger"><strong>Rule 2:</strong> <code>shared_buffers</code> may be too large for host RAM. Risk of swapping detected.</li>';
    } else {
        memHtml += `<li class="text-success">Buffer cache efficiency is optimal (${(latest.cache_hit_ratio || 0).toFixed(1)}%).</li>`;
    }
    
    if (totalMem > 0 && latest.memory_pressure_percent > 85) {
        memHtml += `<li class="text-warning">High OS memory pressure (${latest.memory_pressure_percent.toFixed(1)}%). Check for non-PG RAM consumers.</li>`;
    }
    memHtml += '</ul>';
    memAdvisor.innerHTML = memHtml;

    // Rule 3 & 4: work_mem
    const possibleWorkMem = (components.max_connections || 100) * (components.work_mem_mb || 4);
    const oomRiskPct = totalMem > 0 ? ((components.shared_buffers_mb + possibleWorkMem) / totalMem) * 100 : 0;

    let workHtml = '';
    if (latest.temp_spill_rate_mb_s > 0.1) {
        workHtml += `<p class="text-warning"><strong>Rule 3:</strong> Queries spilling to disk (${latest.temp_spill_rate_mb_s.toFixed(2)} MB/s). Increase <code>work_mem</code> or tune heavy queries.</p>`;
    }
    if (totalMem > 0 && oomRiskPct > 80) {
        workHtml += `<p class="text-danger"><strong>Rule 4:</strong> High OOM Risk! <code>work_mem</code> × <code>max_connections</code> could exhaust RAM (${oomRiskPct.toFixed(1)}% potential usage).</p>`;
    }
    if (!workHtml) {
        workHtml = '<p class="text-success"><i class="fa-solid fa-circle-check"></i> <code>work_mem</code> is sufficient for existing sort/hash operations.</p>';
    }
    workmemAdvisor.innerHTML = workHtml;

    // Rule 5: Connection Memory
    if (connAdvisor) {
        const active = latest.active_connections || 0;
        const total = latest.total_connections || 0;
        const connMemEst = latest.connection_memory_est_mb || (active * 10.0);
        
        connAdvisor.innerHTML = `
            <p>Connections: ${active} active / ${total} total.</p>
            <p>Est. Connection RAM: <strong>${connMemEst.toFixed(1)} MB</strong></p>
            ${latest.pg_memory_percent > 70 ? '<p class="text-warning"><strong>Rule 5:</strong> High per-connection memory. Connection pool recommended.</p>' : '<p class="text-success">Connection memory overhead is stable.</p>'}
        `;
    }
}

// --- Forecast Logic ---
function updateForecast(series) {
    const forecastPane = document.getElementById('cockpit-tab-forecast');
    if (!forecastPane) return;

    // Check if we have OS data in the series
    const hasOSData = series.some(s => (s.memory_pressure_percent || 0) > 0);
    if (!hasOSData) {
        forecastPane.innerHTML = `
            <div style="display: flex; align-items:center; justify-content:center; height: 100%; flex-direction: column; color: var(--text-muted); background: rgba(0,0,0,0.1); border-radius:8px; padding: 2rem;">
                <i class="fa-solid fa-triangle-exclamation" style="font-size: 2.5rem; margin-bottom: 1.5rem; color: var(--warning);"></i>
                <h3 style="font-size: 1rem; margin-bottom: 0.5rem; color: var(--text-primary);">Forecast Unavailable</h3>
                <p style="font-size: 0.8rem; max-width: 400px; text-align: center;">OS-level metrics are not being collected. Please install and configure the OS Collector agent on the monitored host to enable predictive memory analytics.</p>
            </div>
        `;
        return;
    }

    if (series.length < 5) {
        forecastPane.innerHTML = '<div class="text-center p-4 text-muted">Awaiting more data points to generate forecast...</div>';
        return;
    }

    // Simple Linear Regression: y = mx + b
    const calcUsageSlope = (data, key) => {
        const n = data.length;
        let sumX = 0, sumY = 0, sumXY = 0, sumX2 = 0;
        data.forEach((p, i) => {
            sumX += i;
            sumY += (p[key] || 0);
            sumXY += i * (p[key] || 0);
            sumX2 += i * i;
        });
        return (n * sumXY - sumX * sumY) / (n * sumX2 - sumX * sumX);
    };

    const slopeOS = calcUsageSlope(series, 'memory_pressure_percent');
    const slopeRSS = calcUsageSlope(series, 'postgres_rss_mb');
    
    const currentOS = series[series.length - 1].memory_pressure_percent;
    let forecastHtml = '<div style="display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; gap: 1rem; padding: 1.5rem; background: rgba(0,0,0,0.1); border-radius: 12px;">';
    
    if (slopeOS > 0.0001) {
        const minsTo95 = (95 - currentOS) / slopeOS;
        const daysTo95 = (minsTo95 / 1440).toFixed(1);
        
        forecastHtml += `
            <i class="fa-solid fa-chart-line-up text-danger" style="font-size: 2.5rem;"></i>
            <h3 style="margin:0; font-size:1rem;">Memory Saturation Forecast</h3>
            <div class="glass-panel" style="padding: 1.25rem; text-align: center; border-bottom: 3px solid var(--danger);">
                <div class="text-muted small uppercase">Estimated Critical Memory In</div>
                <div style="font-size: 2.2rem; font-weight: 800; color: var(--danger);">${daysTo95} Days</div>
            </div>
            <div style="max-width: 400px; text-align: center; font-size: 0.8rem;">
                <p><strong>Cause:</strong> Host memory pressure trending upward (+${(slopeOS * 60).toFixed(2)}% / hr).</p>
                <p class="text-muted small">PostgreSQL RSS growth: +${(slopeRSS * 60).toFixed(0)} MB/hr</p>
            </div>
        `;
    } else {
        forecastHtml += `
            <i class="fa-solid fa-square-check text-success" style="font-size: 2.5rem;"></i>
            <h3 style="margin:0; font-size:1rem;">Stable Trajectory</h3>
            <p class="text-muted small">No significant memory growth trend detected in the selected window.</p>
            <p class="text-accent" style="font-size: 0.75rem; font-weight:700;">Rate of change: ${(slopeOS * 60).toFixed(3)}% / hr</p>
        `;
    }
    forecastHtml += '</div>';
    forecastPane.innerHTML = forecastHtml;
}

// Optimized Chart Renderers
const sharedOptions = {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    interaction: { mode: 'index', intersect: false },
    plugins: { legend: { display: false } },
    scales: {
        x: { ticks: { display: false }, grid: { display: false } },
        y: { ticks: { font: { size: 9 }, color: '#64748b' }, grid: { color: 'rgba(255,255,255,0.03)' } }
    }
};

function renderHostPgTrendChart(series, osConfigured) {
    const canvas = document.getElementById('hostPgTrendChart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    
    // Update Chart Title
    const chartHeader = document.getElementById('main-memory-trend-title');
    if (chartHeader) {
        chartHeader.innerHTML = osConfigured 
            ? '<i class="fa-solid fa-chart-area text-accent"></i> Host vs PostgreSQL Memory Trend' 
            : '<i class="fa-solid fa-chart-area text-accent"></i> PostgreSQL Memory Trend';
    }

    if (window.currentCharts.hostPgTrend) window.currentCharts.hostPgTrend.destroy();
    
    const datasets = [
        { label: 'RSS MB', data: series.map(s => s.postgres_rss_mb), borderColor: '#10b981', backgroundColor: 'rgba(16, 185, 129, 0.1)', fill: true, yAxisID: 'y', pointRadius: 0, tension: 0.2 }
    ];

    if (osConfigured) {
        datasets.unshift({ label: 'OS %', data: series.map(s => s.memory_pressure_percent), borderColor: '#3b82f6', yAxisID: 'y1', pointRadius: 0, tension: 0.2 });
    }

    window.currentCharts.hostPgTrend = new Chart(ctx, {
        type: 'line',
        data: {
            labels: series.map(s => new Date(s.ts).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'})),
            datasets: datasets
        },
        options: {
            ...sharedOptions,
            plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 }, color: '#94a3b8' } } },
            scales: {
                x: { 
                    display: true,
                    ticks: { display: true, maxTicksLimit: 8, font: { size: 9 }, color: '#64748b' }, 
                    grid: { color: 'rgba(255,255,255,0.03)' },
                    title: { display: true, text: 'Timeline', font: { size: 9 } }
                },
                y: { ...sharedOptions.scales.y, title: { display: true, text: 'Physical Memory (MB)', font: { size: 9 } } },
                y1: { display: osConfigured, position: 'right', max: 100, min: 0, ticks: { font: { size: 9 }, color: '#64748b' }, grid: { display: false }, title: { display: true, text: 'OS %', font: { size: 9 } } }
            }
        }
    });
}

function renderCacheEfficiencyChart(series) {
    const canvas = document.getElementById('cacheEfficiencyChart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (window.currentCharts.cacheEfficiency) window.currentCharts.cacheEfficiency.destroy();
    
    // Determine dynamic min/max for Y axis to avoid "blank" look if hit ratio is low
    const hits = series.map(s => s.cache_hit_ratio || 0);
    const minHit = Math.min(...hits);
    const yMin = minHit < 90 ? Math.max(0, Math.floor(minHit - 5)) : 90;

    window.currentCharts.cacheEfficiency = new Chart(ctx, {
        type: 'line',
        data: {
            labels: series.map(s => new Date(s.ts).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'})),
            datasets: [{ label: 'Cache %', data: hits, borderColor: '#8b5cf6', backgroundColor: 'rgba(139, 92, 246, 0.05)', fill: true, pointRadius: 0, tension: 0.3 }]
        },
        options: { 
            ...sharedOptions, 
            scales: { 
                ...sharedOptions.scales, 
                x: { 
                    display: true, 
                    ticks: { display: true, maxTicksLimit: 8, font: { size: 9 }, color: '#64748b' },
                    grid: { color: 'rgba(255,255,255,0.03)' },
                    title: { display: true, text: 'Timeline', font: { size: 9 } } 
                },
                y: { ...sharedOptions.scales.y, min: yMin, max: 100, title: { display: true, text: 'Hit Ratio %', font: { size: 9 } } } 
            } 
        }
    });
}

function renderPressureSwapChart(series, osConfigured) {
    const canvas = document.getElementById('pressureSwapChart');
    if (!canvas) return;
    
    const card = canvas.closest('.chart-card');
    if (!osConfigured) {
        if (card) card.style.display = 'none';
        return;
    }
    if (card) card.style.display = 'flex';

    const ctx = canvas.getContext('2d');
    if (window.currentCharts.pressureSwap) window.currentCharts.pressureSwap.destroy();
    window.currentCharts.pressureSwap = new Chart(ctx, {
        type: 'line',
        data: {
            labels: series.map(s => new Date(s.ts).toLocaleTimeString()),
            datasets: [{ label: 'Swap MB', data: series.map(s => s.swap_used_mb), borderColor: '#ef4444', backgroundColor: 'rgba(239, 68, 68, 0.05)', fill: true, pointRadius: 0, tension: 0.2 }]
        },
        options: {
            ...sharedOptions,
            scales: { ...sharedOptions.scales, y: { ...sharedOptions.scales.y, ticks: { ...sharedOptions.scales.y.ticks, stepSize: 10 } } }
        }
    });
}

function renderPgComponentsChart(comp) {
    const canvas = document.getElementById('pgComponentsChart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (window.currentCharts.pgComp) window.currentCharts.pgComp.destroy();

    // Ensure we have values and handle both camelCase and snake_case just in case
    const dataPoints = [
        comp.shared_buffers_mb || comp.SharedBuffersMB || 0,
        comp.work_mem_mb || comp.WorkMemMB || 0,
        comp.maintenance_work_mem_mb || comp.MaintenanceWorkMemMB || 0,
        comp.wal_buffers_mb || comp.WalBuffersMB || 0,
        comp.temp_buffers_mb || comp.TempBuffersMB || 0
    ];

    const allZero = dataPoints.every(v => v === 0);
    const labels = ['Shared Buffers', 'work_mem', 'maint_work_mem', 'WAL Buffers', 'temp_buffers'];

    window.currentCharts.pgComp = new Chart(ctx, {
        type: 'doughnut',
        data: {
            labels: allZero ? ['No configuration data found'] : labels,
            datasets: [{ 
                data: allZero ? [1] : dataPoints, 
                backgroundColor: allZero ? ['rgba(148,163,184,0.1)'] : ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#f43f5e'], 
                borderWidth: 0 
            }]
        },
        options: { 
            responsive: true, 
            maintainAspectRatio: false, 
            plugins: { 
                legend: { 
                    position: 'right', 
                    labels: { color: '#94a3b8', font: { size: 9 }, boxWidth: 10 } 
                },
                tooltip: { 
                    enabled: !allZero,
                    callbacks: {
                        label: function(context) {
                            return `${context.label}: ${context.raw} MB`;
                        }
                    }
                }
            }, 
            cutout: '75%' 
        }
    });
}

function renderTempSpillChart(series) {
    const canvas = document.getElementById('tempSpillChart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (window.currentCharts.tempSpill) window.currentCharts.tempSpill.destroy();
    window.currentCharts.tempSpill = new Chart(ctx, {
        type: 'bar',
        data: { labels: series.map(s => new Date(s.ts).toLocaleTimeString()), datasets: [{ label: 'MB/s', data: series.map(s => s.temp_spill_rate_mb_s), backgroundColor: '#f59e0b' }] },
        options: sharedOptions
    });
}

function renderConnMemoryChart(series) {
    const canvas = document.getElementById('connMemoryChart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (window.currentCharts.connMem) window.currentCharts.connMem.destroy();
    window.currentCharts.connMem = new Chart(ctx, {
        type: 'line',
        data: { labels: series.map(s => new Date(s.ts).toLocaleTimeString()), datasets: [{ label: 'Est MB', data: series.map(s => s.connection_memory_est_mb), borderColor: '#3b82f6', pointRadius: 0, tension: 0.3 }] },
        options: sharedOptions
    });
}

// Chart Synchronization (Cross-Highlighting)
function setupChartSync() {
    const charts = Object.values(window.currentCharts).filter(c => c && c.config.type !== 'doughnut');
    charts.forEach(chart => {
        chart.options.onHover = (event, elements) => {
            if (elements.length > 0) {
                const index = elements[0].index;
                charts.forEach(otherChart => {
                    if (otherChart && otherChart !== chart) {
                        otherChart.setActiveElements([{ datasetIndex: 0, index }]);
                        otherChart.tooltip.setActiveElements([{ datasetIndex: 0, index }], { x: 0, y: 0 });
                        otherChart.update();
                    }
                });
            }
        };
    });
}

function updatePgMemoryTable(series) {
    const tbody = document.getElementById('pg-memory-raw-tbody');
    if (!tbody) return;
    tbody.innerHTML = series.slice().reverse().slice(0, 100).map(s => `
        <tr>
            <td>${new Date(s.ts).toLocaleTimeString()}</td>
            <td>${(s.memory_pressure_percent || 0).toFixed(1)}%</td>
            <td>${(s.postgres_rss_mb || 0).toLocaleString()}</td>
            <td class="${s.swap_used_mb > 0 ? 'text-warning' : ''}">${(s.swap_used_mb || 0).toLocaleString()}</td>
            <td>${(s.temp_spill_rate_mb_s || 0).toFixed(3)}</td>
            <td>${(s.cache_hit_ratio || 0).toFixed(2)}%</td>
        </tr>
    `).join('');
}

// Helpers
function toLocalISOString(date) {
    const tzo = -date.getTimezoneOffset(), pad = (num) => (num < 10 ? '0' : '') + num;
    return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + 'T' + pad(date.getHours()) + ':' + pad(date.getMinutes());
}

window.exportPgMemoryCSV = function() {
    if (!window._lastPgMemSeries || window._lastPgMemSeries.length === 0) return;
    const headers = ["Timestamp", "OS Used %", "PG RSS MB", "Swap Used MB", "Temp Rate MB/s", "Cache Hit %"];
    const csv = [headers.join(",")].concat(window._lastPgMemSeries.map(s => [s.ts, s.memory_pressure_percent, s.postgres_rss_mb, s.swap_used_mb, s.temp_spill_rate_mb_s, s.cache_hit_ratio].join(","))).join("\n");
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a'); a.href = url; a.download = `pg_memory_cockpit_${new Date().toISOString()}.csv`; a.click();
}
