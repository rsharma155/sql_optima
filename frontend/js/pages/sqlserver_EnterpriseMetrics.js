/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Enterprise metrics dashboard (V2) - Time-Series driven observability.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.EnterpriseMetricsView = async function() {
    appDebug('[EnterpriseMetrics] Starting V2');
    if (!window.appState.config || !window.appState.config.instances || window.appState.config.instances.length === 0) {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select an instance first</h3></div>`;
        return;
    }
    
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'sqlserver') {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select a SQL Server instance.</h3></div>`;
        return;
    }

    // Load template
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/sqlserver_enterprise_metrics_v2.html');
    document.getElementById('emInstanceName').textContent = inst.name;

    // Time picker setup
    const fromEl = document.getElementById('emFrom');
    const toEl = document.getElementById('emTo');
    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    
    const pad = (n) => String(n).padStart(2, '0');
    const fmtLocal = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    
    fromEl.value = fmtLocal(oneHourAgo);
    toEl.value = fmtLocal(now);

    window.emSetRange = (hours) => {
        const t = new Date();
        const f = new Date(t.getTime() - hours * 60 * 60 * 1000);
        fromEl.value = fmtLocal(f);
        toEl.value = fmtLocal(t);
        loadData();
    };

    document.getElementById('emApplyRange').onclick = loadData;

    // Chart state
    window._emCharts = window._emCharts || {};
    function destroyCharts() {
        Object.values(window._emCharts).forEach(c => { try { c.destroy(); } catch (_) {} });
        window._emCharts = {};
    }

    async function loadData() {
        const from = new Date(fromEl.value).toISOString();
        const to = new Date(toEl.value).toISOString();
        
        try {
            const resp = await window.apiClient.authenticatedFetch(
                `/api/sqlserver/enterprise-dashboard/v2?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
            );
            if (!resp.ok) throw new Error('Failed to fetch enterprise metrics');
            const data = await resp.json();
            renderDashboard(data);
        } catch (e) {
            console.error('[EnterpriseMetrics] Load failed:', e);
        }
    }

    function renderDashboard(data) {
        destroyCharts();
        
        // 1. Snapshot Cards
        const snapshot = data.snapshot || {};
        const perf = data.perf_counters || [];
        
        // Runnable tasks (latest)
        const runnable = snapshot.runnable_tasks_count ?? '--';
        document.getElementById('val-runnable').textContent = runnable;
        
        // Pending grants
        const pendingGrants = snapshot.memory_grants_pending ?? '--';
        document.getElementById('val-pending-grants').textContent = pendingGrants;
        
        // Latch Waits/sec (latest in time range)
        const latchWaits = perf.filter(p => p.counter_name === 'Latch Waits/sec').pop()?.value ?? 0;
        document.getElementById('val-latch-waits').textContent = latchWaits.toFixed(1);
        
        // Plan waste (latest in time range)
        const cache = data.plan_cache || [];
        const latestCache = cache.filter(c => c.timestamp === cache[cache.length-1]?.timestamp);
        const totalCache = latestCache.reduce((sum, c) => sum + (c.size_mb || 0), 0);
        const adhoc = latestCache.find(c => c.cache_type === 'Adhoc')?.size_mb || 0;
        const wastePct = totalCache > 0 ? (adhoc / totalCache * 100) : 0;
        document.getElementById('val-plan-waste').textContent = wastePct.toFixed(1) + '%';

        // Set card colors
        const setStatus = (id, val, warn, crit) => {
            const el = document.getElementById(id);
            if (!el) return;
            el.className = 'enterprise-kpi-card glass-panel ' + (val >= crit ? 'status-danger' : (val >= warn ? 'status-warning' : 'status-healthy'));
        };
        setStatus('card-runnable', Number(runnable), 5, 20);
        setStatus('card-pending-grants', Number(pendingGrants), 1, 5);
        setStatus('card-latch-waits', latchWaits, 10, 50);
        setStatus('card-plan-waste', wastePct, 20, 40);

        // 2. Wait Stats Chart (Stacked Area)
        renderWaitStatsChart(data.wait_stats);

        // 3. Throughput Chart
        renderThroughputChart(data.perf_counters);

        // 4. File IO Chart
        renderFileIOChart(data.file_io);

        // 5. Plan Cache Composition
        renderPlanCacheChart(data.plan_cache);

        // 6. Memory & Grants
        renderMemoryChart(data.memory_clerks, data.memory_grants);

        // 7. TempDB Consumers Table
        renderTempdbTable(data.tempdb_consumers);
    }

    function renderWaitStatsChart(waits) {
        const ctx = document.getElementById('chart-wait-stats');
        if (!ctx || !waits || waits.length === 0) return;

        // Group by timestamp and category
        const groups = {};
        const categories = new Set();
        waits.forEach(w => {
            const t = w.event_time;
            groups[t] = groups[t] || {};
            groups[t][w.wait_category] = (groups[t][w.wait_category] || 0) + (w.wait_time_ms || 0);
            categories.add(w.wait_category);
        });

        const sortedTimestamps = Object.keys(groups).sort();
        const datasets = Array.from(categories).map((cat, idx) => ({
            label: cat,
            data: sortedTimestamps.map(t => groups[t][cat] || 0),
            backgroundColor: getPaletteColor(idx, 0.5),
            borderColor: getPaletteColor(idx, 1),
            fill: true,
            pointRadius: 0,
            tension: 0.2
        }));

        window._emCharts.waits = new Chart(ctx, {
            type: 'line',
            data: { labels: sortedTimestamps.map(t => new Date(t).toLocaleTimeString()), datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                scales: { x: { display: true }, y: { stacked: true, beginAtZero: true, title: { display: true, text: 'Wait Time (ms)' } } },
                plugins: { legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } }
            }
        });
    }

    function renderThroughputChart(perf) {
        const ctx = document.getElementById('chart-throughput');
        if (!ctx || !perf || perf.length === 0) return;

        const batch = perf.filter(p => p.counter_name === 'Batch Requests/sec');
        const comp = perf.filter(p => p.counter_name === 'SQL Compilations/sec');
        const recomp = perf.filter(p => p.counter_name === 'SQL Re-Compilations/sec');

        const labels = batch.map(p => new Date(p.event_time).toLocaleTimeString());

        window._emCharts.throughput = new Chart(ctx, {
            type: 'line',
            data: {
                labels,
                datasets: [
                    { label: 'Batch Requests', data: batch.map(p => p.value), borderColor: '#3b82f6', pointRadius: 0, tension: 0.2 },
                    { label: 'Compilations', data: comp.map(p => p.value), borderColor: '#10b981', pointRadius: 0, tension: 0.2 },
                    { label: 'Re-Compilations', data: recomp.map(p => p.value), borderColor: '#ef4444', borderDash: [5, 5], pointRadius: 0, tension: 0.2 }
                ]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                scales: { y: { beginAtZero: true } },
                plugins: { legend: { position: 'top' } }
            }
        });
    }

    function renderFileIOChart(io) {
        const ctx = document.getElementById('chart-file-io');
        if (!ctx || !io || io.length === 0) return;

        // Group by database
        const dbGroups = {};
        io.forEach(f => {
            const db = f.database_name;
            dbGroups[db] = dbGroups[db] || [];
            dbGroups[db].push(f);
        });

        const labels = io.filter(f => f.database_name === Object.keys(dbGroups)[0]).map(f => new Date(f.event_time).toLocaleTimeString());
        const datasets = Object.keys(dbGroups).slice(0, 5).map((db, idx) => ({
            label: db + ' Latency',
            data: dbGroups[db].map(f => (f.read_latency_ms + f.write_latency_ms) / 2),
            borderColor: getPaletteColor(idx, 1),
            pointRadius: 0,
            tension: 0.2
        }));

        window._emCharts.fileio = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                scales: { y: { beginAtZero: true, title: { display: true, text: 'ms' } } }
            }
        });
    }

    function renderPlanCacheChart(cache) {
        const ctx = document.getElementById('chart-plan-cache');
        if (!ctx || !cache || cache.length === 0) return;

        const types = ['Adhoc', 'Prepared', 'Proc'];
        const groups = {};
        cache.forEach(c => {
            const t = c.event_time;
            groups[t] = groups[t] || {};
            groups[t][c.cache_type] = c.size_mb;
        });

        const sortedTimestamps = Object.keys(groups).sort();
        const datasets = types.map((type, idx) => ({
            label: type,
            data: sortedTimestamps.map(t => groups[t][type] || 0),
            backgroundColor: getPaletteColor(idx + 5, 0.5),
            borderColor: getPaletteColor(idx + 5, 1),
            fill: true,
            pointRadius: 0
        }));

        window._emCharts.plancache = new Chart(ctx, {
            type: 'line',
            data: { labels: sortedTimestamps.map(t => new Date(t).toLocaleTimeString()), datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                scales: { y: { stacked: true, beginAtZero: true, title: { display: true, text: 'MB' } } }
            }
        });
    }

    function renderMemoryChart(clerks, grants) {
        const ctx = document.getElementById('chart-memory');
        if (!ctx) return;

        // Combine clerks and grants? Or use dual Y axis.
        // Let's show top 5 clerks stacked, and grants pending as a line on secondary axis.
        
        const clerkGroups = {};
        (clerks || []).forEach(c => {
            clerkGroups[c.clerk_name] = clerkGroups[c.clerk_name] || [];
            clerkGroups[c.clerk_name].push(c);
        });

        // Get top 5 clerks by latest value
        const topClerks = Object.keys(clerkGroups)
            .sort((a, b) => {
                const valA = clerkGroups[a][clerkGroups[a].length - 1]?.pages_mb || 0;
                const valB = clerkGroups[b][clerkGroups[b].length - 1]?.pages_mb || 0;
                return valB - valA;
            })
            .slice(0, 5);

        const firstClerk = clerkGroups[topClerks[0]] || [];
        const labels = firstClerk.map(c => new Date(c.event_time).toLocaleTimeString());

        const datasets = topClerks.map((name, idx) => ({
            label: name,
            data: clerkGroups[name].map(c => c.pages_mb),
            backgroundColor: getPaletteColor(idx + 10, 0.5),
            borderColor: getPaletteColor(idx + 10, 1),
            fill: true,
            pointRadius: 0,
            yAxisID: 'y'
        }));

        if (grants && grants.length > 0) {
            datasets.push({
                label: 'Pending Grants',
                data: grants.map(g => g.pending_grants),
                borderColor: '#ef4444',
                borderWidth: 2,
                pointRadius: 0,
                yAxisID: 'y1',
                type: 'line',
                fill: false
            });
        }

        window._emCharts.memory = new Chart(ctx, {
            type: 'line',
            data: { labels, datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                scales: {
                    y: { stacked: true, beginAtZero: true, title: { display: true, text: 'Memory MB' } },
                    y1: { position: 'right', beginAtZero: true, grid: { drawOnChartArea: false }, title: { display: true, text: 'Pending Grants' } }
                }
            }
        });
    }

    function renderTempdbTable(consumers) {
        const tbody = document.getElementById('tbody-tempdb-consumers');
        if (!tbody) return;
        
        if (!consumers || consumers.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" class="text-center text-muted">No TempDB consumers</td></tr>';
            return;
        }

        // Get latest snapshot
        const latestTime = consumers[consumers.length - 1].event_time;
        const latest = consumers.filter(c => c.event_time === latestTime)
            .sort((a, b) => (b.allocated_mb || 0) - (a.allocated_mb || 0))
            .slice(0, 15);

        tbody.innerHTML = latest.map(c => `
            <tr>
                <td>${c.session_id}</td>
                <td>${(c.allocated_mb || 0).toFixed(1)} MB</td>
                <td>${(c.user_objects_mb || 0).toFixed(1)} MB</td>
                <td>${(c.internal_objects_mb || 0).toFixed(1)} MB</td>
                <td title="${window.escapeHtml(c.query_text || '')}">${(c.query_text || '').substring(0, 50)}...</td>
            </tr>
        `).join('');
    }

    function getPaletteColor(idx, alpha) {
        const colors = [
            `rgba(59, 130, 246, ${alpha})`,  // blue
            `rgba(16, 185, 129, ${alpha})`,  // green
            `rgba(245, 158, 11, ${alpha})`,  // amber
            `rgba(239, 68, 68, ${alpha})`,   // red
            `rgba(139, 92, 246, ${alpha})`,  // violet
            `rgba(236, 72, 153, ${alpha})`,  // pink
            `rgba(20, 184, 166, ${alpha})`,  // teal
            `rgba(249, 115, 22, ${alpha})`,  // orange
            `rgba(107, 114, 128, ${alpha})`, // gray
            `rgba(99, 102, 241, ${alpha})`,  // indigo
        ];
        return colors[idx % colors.length];
    }

    await loadData();
};
