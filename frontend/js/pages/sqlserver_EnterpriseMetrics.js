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

    // Standard Time picker setup
    window.initPageTimePicker();
    const fromEl = document.getElementById('from-ts');
    const toEl = document.getElementById('to-ts');
    const refreshBtn = document.getElementById('global-refresh-btn');

    const pad = (n) => String(n).padStart(2, '0');
    const fmtLocal = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    
    window.emSetRange = (hours) => {
        const t = new Date();
        const f = new Date(t.getTime() - hours * 60 * 60 * 1000);
        if (fromEl) fromEl.value = fmtLocal(f);
        if (toEl) toEl.value = fmtLocal(t);
        window.appState.fromTs = fromEl ? fromEl.value : window.appState.fromTs;
        window.appState.toTs = toEl ? toEl.value : window.appState.toTs;
        loadData();
    };

    window._enterpriseMetricsLoadData = loadData;

    // Chart state
    window._emCharts = window._emCharts || {};
    function destroyCharts() {
        Object.values(window._emCharts).forEach(c => { try { c.destroy(); } catch (_) {} });
        window._emCharts = {};
        ['chart-wait-stats', 'chart-throughput', 'chart-file-io', 'chart-plan-cache', 'chart-memory'].forEach(id => {
            if (window.destroyChartOnCanvas) window.destroyChartOnCanvas(id);
        });
    }

    function makeChart(canvasId, config) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return null;
        if (window.destroyChartOnCanvas) window.destroyChartOnCanvas(canvas);
        const chart = (typeof window.createOptimaChart === 'function')
            ? window.createOptimaChart(canvas, config)
            : new Chart(canvas, config);
        return chart;
    }

    async function loadData() {
        const range = window.getAppTimeRangeISO
            ? window.getAppTimeRangeISO({ sync: true, fallbackHours: 1 })
            : null;
        let from = range?.from || (window.localDateTimeToISO ? window.localDateTimeToISO(fromEl?.value) : '');
        let to = range?.to || (window.localDateTimeToISO ? window.localDateTimeToISO(toEl?.value) : '');
        if (!from || !to) {
            console.warn('[EnterpriseMetrics] Invalid time range; using last 1 hour');
            const now = Date.now();
            from = new Date(now - 3600000).toISOString();
            to = new Date(now).toISOString();
        }
        
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
        
        // Helper to set text content safely
        const setVal = (id, val) => {
            const el = document.getElementById(id);
            if (el) el.textContent = val;
        };
        
        // Runnable tasks (latest)
        const runnable = snapshot.runnable_tasks ?? '--';
        setVal('val-runnable', runnable);
        
        // Pending grants
        const pendingGrants = snapshot.mem_grants_pending ?? '--';
        setVal('val-pending-grants', pendingGrants);
        
        // Logins/sec (latest in time range)
        const logins = data.throughput?.length > 0 ? data.throughput[data.throughput.length-1].logins_per_sec : 0;
        setVal('val-logins', logins.toFixed(1));

        // Server Memory
        const memTotal = snapshot.total_server_memory_mb ? (snapshot.total_server_memory_mb / 1024).toFixed(1) : '--';
        const memTarget = snapshot.target_server_memory_mb ? (snapshot.target_server_memory_mb / 1024).toFixed(1) : '--';
        setVal('val-memory', memTotal === '--' ? '--' : (memTotal + ' GB'));
        setVal('val-memory-total', memTotal);
        setVal('val-memory-target', memTarget);
        
        // Plan waste (latest in time range)
        const cache = data.plan_cache || [];
        const latestCache = cache.filter(c => c.timestamp === cache[cache.length-1]?.timestamp);
        const totalCache = latestCache.reduce((sum, c) => sum + (c.size_mb || 0), 0);
        const adhoc = latestCache.find(c => c.cache_type === 'Adhoc')?.size_mb || 0;
        const wastePct = totalCache > 0 ? (adhoc / totalCache * 100) : 0;
        setVal('val-plan-waste', wastePct.toFixed(1) + '%');

        // Log Write Stall
        const logStall = data.file_io?.filter(f => f.file_type === 'LOG').pop()?.write_latency_ms ?? 0;
        setVal('val-log-waits', logStall.toFixed(1));

        // Set card colors
        const setStatus = (id, val, warn, crit) => {
            const el = document.getElementById(id);
            if (!el) return;
            el.className = 'enterprise-kpi-card glass-panel ' + (val >= crit ? 'status-danger' : (val >= warn ? 'status-warning' : 'status-healthy'));
        };
        setStatus('card-runnable', Number(runnable), 5, 20);
        setStatus('card-pending-grants', Number(pendingGrants), 1, 5);
        setStatus('card-logins', logins, 5, 20);
        setStatus('card-plan-waste', wastePct, 20, 40);
        setStatus('card-log-waits', logStall, 20, 100);

        // Memory Pressure Logic: If Total < 90% of Target
        if (memTotal !== '--' && memTarget !== '--' && Number(memTotal) < (Number(memTarget) * 0.9)) {
            const memCard = document.getElementById('card-memory');
            if (memCard) memCard.className = 'enterprise-kpi-card glass-panel status-warning';
        }

        // 2. Wait Stats Chart (Stacked Area)
        renderWaitStatsChart(data.wait_trends);

        // 3. Throughput Chart
        renderThroughputChart(data.throughput);

        // 4. File IO Chart
        renderFileIOChart(data.file_io);

        // 5. Plan Cache Composition
        renderPlanCacheChart(data.plan_cache);

        // 6. Memory & Grants
        renderMemoryChart(data.memory_clerks, data.memory_grants);

        // 7. TempDB Consumers Table
        renderTempdbTable(data.tempdb_consumers);
    }

    function renderWaitStatsChart(trends) {
        const ctx = document.getElementById('chart-wait-stats');
        if (!ctx || !trends || trends.length === 0) return;

        const labels = trends.map(t => new Date(t.timestamp).toLocaleTimeString());
        const categories = ['cpu', 'io', 'memory', 'locking', 'parallel', 'other'];
        const colors = {
            cpu: '#3b82f6',
            io: '#10b981',
            memory: '#8b5cf6',
            locking: '#f59e0b',
            parallel: '#06b6d4',
            other: '#6b7280'
        };

        const datasets = categories.map(cat => ({
            label: cat.toUpperCase(),
            data: trends.map(t => t[cat] || 0),
            backgroundColor: colors[cat] + '80', // 50% alpha
            borderColor: colors[cat],
            fill: true,
            pointRadius: 0,
            tension: 0.2
        }));

        window._emCharts.waits = makeChart('chart-wait-stats', {
            type: 'line',
            data: { labels, datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                scales: { x: { display: true }, y: { stacked: true, beginAtZero: true, title: { display: true, text: 'Wait Time (ms)' } } },
                plugins: { legend: { position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } }
            }
        });
    }

    function renderThroughputChart(throughput) {
        const ctx = document.getElementById('chart-throughput');
        if (!ctx || !throughput || throughput.length === 0) return;

        const labels = throughput.map(p => new Date(p.timestamp).toLocaleTimeString());

        window._emCharts.throughput = makeChart('chart-throughput', {
            type: 'line',
            data: {
                labels,
                datasets: [
                    { label: 'Batch Requests', data: throughput.map(p => p.batch_requests), borderColor: '#3b82f6', pointRadius: 0, tension: 0.2 },
                    { label: 'Logins/sec', data: throughput.map(p => p.logins_per_sec), borderColor: '#f59e0b', pointRadius: 0, tension: 0.2 },
                    { label: 'Connections', data: throughput.map(p => p.connections), borderColor: '#10b981', pointRadius: 0, tension: 0.2, yAxisID: 'y1' }
                ]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                scales: { 
                    y: { beginAtZero: true, title: { display: true, text: 'Req/Sec' } },
                    y1: { position: 'right', beginAtZero: true, title: { display: true, text: 'Count' }, grid: { drawOnChartArea: false } }
                },
                plugins: { legend: { position: 'top' } }
            }
        });
    }

    function renderFileIOChart(io) {
        const ctx = document.getElementById('chart-file-io');
        if (!ctx || !io || io.length === 0) return;

        // Group by database, excluding noise system DBs but keeping tempdb
        const dbGroups = {};
        const systemDbsToIgnore = ['master', 'model', 'msdb'];
        
        // 1. Filter and group
        io.forEach(f => {
            const dbName = f.database_name;
            if (!dbName) return;
            const db = String(dbName).toLowerCase();
            if (systemDbsToIgnore.includes(db)) return;
            
            dbGroups[f.database_name] = dbGroups[f.database_name] || [];
            dbGroups[f.database_name].push(f);
        });

        // 2. Sort by latest average latency to pick top 5
        const sortedDbs = Object.keys(dbGroups).sort((a, b) => {
            const lastA = dbGroups[a][dbGroups[a].length - 1];
            const lastB = dbGroups[b][dbGroups[b].length - 1];
            const latA = (lastA.read_latency_ms + lastA.write_latency_ms) / 2;
            const latB = (lastB.read_latency_ms + lastB.write_latency_ms) / 2;
            return latB - latA;
        }).slice(0, 5);

        if (sortedDbs.length === 0) return;

        const labels = dbGroups[sortedDbs[0]].map(f => window.fmtChartTick ? window.fmtChartTick(f) : '');
        const datasets = sortedDbs.map((db, idx) => ({
            label: db,
            data: dbGroups[db].map(f => (f.read_latency_ms + f.write_latency_ms) / 2),
            borderColor: getPaletteColor(idx, 1),
            pointRadius: 0,
            tension: 0.2
        }));

        window._emCharts.fileio = makeChart('chart-file-io', {
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
            const ms = window.tsMs(c);
            if (ms == null) return;
            groups[ms] = groups[ms] || {};
            groups[ms][c.cache_type] = c.size_mb;
        });

        const sortedTimes = Object.keys(groups).map(Number).sort((a, b) => a - b);
        const datasets = types.map((type, idx) => ({
            label: type,
            data: sortedTimes.map(ms => groups[ms][type] || 0),
            backgroundColor: getPaletteColor(idx + 5, 0.5),
            borderColor: getPaletteColor(idx + 5, 1),
            fill: true,
            pointRadius: 0
        }));

        window._emCharts.plancache = makeChart('chart-plan-cache', {
            type: 'line',
            data: { labels: sortedTimes.map(ms => window.fmtChartTick(ms)), datasets },
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

        const firstClerk = window.sortByChartTime(clerkGroups[topClerks[0]] || []);
        const labelTimes = firstClerk.map(c => window.tsMs(c)).filter(t => t != null);
        const labels = labelTimes.map(ms => window.fmtChartTick(ms));

        const datasets = topClerks.map((name, idx) => {
            const byTime = new Map();
            (clerkGroups[name] || []).forEach(c => {
                const ms = window.tsMs(c);
                if (ms != null) byTime.set(ms, c.pages_mb);
            });
            return {
                label: name,
                data: labelTimes.map(ms => byTime.get(ms) ?? null),
                backgroundColor: getPaletteColor(idx + 10, 0.5),
                borderColor: getPaletteColor(idx + 10, 1),
                fill: true,
                pointRadius: 0,
                yAxisID: 'y'
            };
        });

        if (grants && grants.length > 0) {
            const grantByTime = new Map();
            grants.forEach(g => {
                const ms = window.tsMs(g);
                if (ms != null) grantByTime.set(ms, g.pending_grants);
            });
            datasets.push({
                label: 'Pending Grants',
                data: labelTimes.map(ms => grantByTime.get(ms) ?? null),
                borderColor: '#ef4444',
                borderWidth: 2,
                pointRadius: 0,
                yAxisID: 'y1',
                type: 'line',
                fill: false
            });
        }

        window._emCharts.memory = makeChart('chart-memory', {
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

        const sortedConsumers = window.sortByChartTime(consumers);
        const latestMs = window.tsMs(sortedConsumers[sortedConsumers.length - 1]);
        const latest = sortedConsumers.filter(c => {
            const ms = window.tsMs(c);
            return latestMs != null && ms != null && Math.abs(ms - latestMs) < 1000;
        }).sort((a, b) => (b.tempdb_mb || b.allocated_mb || 0) - (a.tempdb_mb || a.allocated_mb || 0))
            .slice(0, 15);

        tbody.innerHTML = latest.map((c, idx) => {
            const cacheKey = `tempdb_q_${idx}`;
            if (!window.appState.queryCache) window.appState.queryCache = {};
            window.appState.queryCache[cacheKey] = c.query_text || '';

            return `
                <tr>
                    <td>${c.session_id}</td>
                    <td>${(c.tempdb_mb || c.allocated_mb || 0).toFixed(1)} MB</td>
                    <td>${(c.user_objects_mb || 0).toFixed(1)} MB</td>
                    <td>${(c.internal_objects_mb || 0).toFixed(1)} MB</td>
                    <td style="max-width: 500px;">
                        <span class="code-snippet clickable-query" style="cursor: pointer; display: inline-block; max-width: 480px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 2px 6px; border-radius: 4px; font-size: 0.75rem;" 
                              title="Click to view full query"
                              data-action="show-query-modal-direct" data-key="${cacheKey}">
                            ${window.escapeHtml((c.query_text || '').substring(0, 100))}${c.query_text && c.query_text.length > 100 ? '...' : ''}
                        </span>
                    </td>
                </tr>
            `;
        }).join('');

        // Add event listeners for the clickable queries
        tbody.querySelectorAll('.clickable-query').forEach(el => {
            el.onclick = (e) => {
                const key = el.getAttribute('data-key');
                const query = window.appState.queryCache[key];
                if (window.showQueryModal) {
                    window.showQueryModal(query);
                } else {
                    alert(query);
                }
            };
        });
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
