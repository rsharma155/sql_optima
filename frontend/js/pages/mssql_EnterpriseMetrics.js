window.EnterpriseMetricsView = async function() {
    appDebug('[EnterpriseMetrics] Starting');
    if (!window.appState.config || !window.appState.config.instances || window.appState.config.instances.length === 0) {
        window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">Please select an instance first</h3></div>`;
        return;
    }
    
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    appDebug('[EnterpriseMetrics] Selected instance:', inst);
    if (!inst || typeof inst !== 'object' || !inst.name) {
        window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">Please select an instance first</h3></div>`;
        return;
    }
    
    window.appState.currentInstanceName = inst.name;
    appDebug('[EnterpriseMetrics] Loading metrics for instance:', inst.name);
    
    if (window.enterpriseMetricsInterval) {
        clearInterval(window.enterpriseMetricsInterval);
    }
    
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1>Enterprise Metrics</h1>
                    <span class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Advanced Performance Monitoring</span>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:0.75rem; flex-wrap:wrap; justify-content:flex-end;">
                    <span id="enterpriseDataSourceBadge" class="badge badge-info" style="display:none; font-size:0.65rem;">Source</span>
                    <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="enterpriseMetricsLastUpdate">--:--:--</span></span>
                    <label class="flex-between" style="align-items:center; gap:0.5rem; font-size:0.8rem; cursor:pointer;">
                        <input type="checkbox" id="enterpriseMetricsAutoRefresh" checked style="width:16px; height:16px;"> Auto-refresh (10s)
                    </label>
                    <button class="btn btn-sm btn-outline text-accent" onclick="window.EnterpriseMetricsView()"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            <div style="display:flex; justify-content:center; align-items:center; height:50vh;">
                <div class="spinner"></div><span style="margin-left:1rem;">Loading metrics...</span>
            </div>
        </div>
    `;

    async function loadMetrics() {
        try {
            const fetchWithError = async (url) => {
                try {
                    const res = await window.apiClient.authenticatedFetch(url);
                    if (!res.ok) {
                        appDebug('[EnterpriseMetrics] Fetch failed for', url, 'status:', res.status);
                        return [];
                    }
                    // Use any one endpoint to determine the server-side data source behavior.
                    const ds = res.headers.get('X-Data-Source');
                    if (ds) updateEnterpriseDataSourceBadge(ds);
                    const data = await res.json();
                    if (data == null) return [];
                    // Most endpoints return {key: []}. Normalize to the array payload.
                    if (Array.isArray(data)) return data;
                    if (typeof data === 'object') {
                        const keys = [
                            'latch_stats',
                            'waiting_tasks',
                            'memory_grants',
                            'scheduler_wg',
                            'scheduler_workers',
                            'procedure_stats',
                            'file_io_latency',
                            'spinlock_stats',
                            'memory_clerks',
                            'tempdb_stats'
                        ];
                        for (const k of keys) {
                            if (Array.isArray(data[k])) return data[k];
                        }
                    }
                    return [];
                } catch(e) {
                    appDebug('[EnterpriseMetrics] Exception for', url, ':', e);
                    return [];
                }
            };
            
            const [latchStats, waitingTasks, memoryGrants, schedulerWG, procedureStats, fileIoLatency, spinlockStats, memoryClerks, tempdbStats, planCacheHealth, memoryGrantWaiters, tempdbTopConsumers, waitCategories] = await Promise.all([
                fetchWithError(`/api/mssql/latch-stats?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/waiting-tasks?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/memory-grants?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/scheduler-wg?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/procedure-stats?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/file-io-latency?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/spinlock-stats?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/memory-clerks?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/tempdb-stats?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/plan-cache-health?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/memory-grant-waiters?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/tempdb-top-consumers?instance=${encodeURIComponent(inst.name)}`),
                fetchWithError(`/api/mssql/wait-categories?instance=${encodeURIComponent(inst.name)}`)
            ]);
            
            const metrics = {
                latchStats: latchStats || [],
                waitingTasks: waitingTasks || [],
                memoryGrants: memoryGrants || [],
                schedulerWG: schedulerWG || [],
                procedureStats: procedureStats || [],
                fileIoLatency: fileIoLatency || [],
                spinlockStats: spinlockStats || [],
                memoryClerks: memoryClerks || [],
                tempdbStats: tempdbStats || [],
                planCacheHealth: planCacheHealth || [],
                memoryGrantWaiters: memoryGrantWaiters || [],
                tempdbTopConsumers: tempdbTopConsumers || [],
                waitCategories: waitCategories || []
            };
            
            appDebug('[EnterpriseMetrics] Loaded metrics:', metrics);
            renderEnterpriseMetrics(inst, metrics);
            
            const lastUpdateEl = document.getElementById('enterpriseMetricsLastUpdate');
            if (lastUpdateEl) {
                lastUpdateEl.textContent = new Date().toLocaleTimeString();
            }
        } catch(error) {
            appDebug("[EnterpriseMetrics] Error:", error);
        }
    }
    
    await loadMetrics();

    // Auto-refresh: ensure we only have one interval.
    const autoRefreshCheckbox = document.getElementById('enterpriseMetricsAutoRefresh');
    const enableAutoRefresh = () => {
        if (window.enterpriseMetricsInterval) {
            clearInterval(window.enterpriseMetricsInterval);
            window.enterpriseMetricsInterval = null;
        }
        const checked = autoRefreshCheckbox ? autoRefreshCheckbox.checked : true;
        if (checked) {
            window.enterpriseMetricsInterval = setInterval(loadMetrics, 10000);
        }
    };
    if (autoRefreshCheckbox && !autoRefreshCheckbox.__bound) {
        autoRefreshCheckbox.addEventListener('change', enableAutoRefresh);
        autoRefreshCheckbox.__bound = true;
    }
    enableAutoRefresh();
};

function updateEnterpriseDataSourceBadge(source) {
    const el = document.getElementById('enterpriseDataSourceBadge');
    if (!el) return;
    if (!source) {
        el.style.display = 'none';
        return;
    }
    let label = 'Source: ' + source;
    let cls = 'badge badge-info';
    if (source === 'timescale') {
        label = 'Source: Timescale snapshot';
        cls = 'badge badge-success';
    } else if (source === 'live_dmv_fallback') {
        label = 'Source: Live DMV fallback';
        cls = 'badge badge-warning';
    } else if (source === 'live_dmv_error') {
        label = 'Source: Live DMV error';
        cls = 'badge badge-danger';
    }
    el.className = cls;
    el.textContent = label;
    el.style.display = 'inline-flex';
}

function renderEnterpriseMetrics(inst, metrics) {
    // Defensive: ensure metrics is defined
    if (!metrics || typeof metrics !== 'object') {
        appDebug('[EnterpriseMetrics] Invalid metrics object:', metrics);
        window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-danger">Error: No metrics data available</h3></div>`;
        return;
    }
    
    const latchData = (metrics?.latchStats != null) ? metrics.latchStats : [];
    const waitingData = (metrics?.waitingTasks != null) ? metrics.waitingTasks : [];
    const memoryGrantsData = (metrics?.memoryGrants != null) ? metrics.memoryGrants : [];
    const schedulerWGData = (metrics?.schedulerWG != null) ? metrics.schedulerWG : [];
    const procedureStatsData = (metrics?.procedureStats != null) ? metrics.procedureStats : [];
    const fileIoLatencyData = (metrics?.fileIoLatency != null) ? metrics.fileIoLatency : [];
    const spinlockStatsData = (metrics?.spinlockStats != null) ? metrics.spinlockStats : [];
    const memoryClerksData = (metrics?.memoryClerks != null) ? metrics.memoryClerks : [];
    const tempdbStatsData = (metrics?.tempdbStats != null) ? metrics.tempdbStats : [];
    const planCacheHealthData = (metrics?.planCacheHealth != null) ? metrics.planCacheHealth : [];
    const memoryGrantWaitersData = (metrics?.memoryGrantWaiters != null) ? metrics.memoryGrantWaiters : [];
    const tempdbTopConsumersData = (metrics?.tempdbTopConsumers != null) ? metrics.tempdbTopConsumers : [];
    const waitCategoriesData = (metrics?.waitCategories != null) ? metrics.waitCategories : [];
    
    appDebug('[EnterpriseMetrics] renderEnterpriseMetrics - latchData len:', latchData.length);
    
    const latchRows = latchData.slice(0, 15).map(l => `
        <tr>
            <td><span class="badge badge-outline">${window.escapeHtml(l.wait_type || 'N/A')}</span></td>
            <td>${l.waiting_tasks_count || 0}</td>
            <td>${((l.wait_time_ms || 0) / 1000).toFixed(1)}s</td>
            <td>${((l.signal_wait_time_ms || 0) / 1000).toFixed(1)}s</td>
        </tr>
    `).join('');

    const waitingRows = waitingData.slice(0, 15).map(w => `
        <tr>
            <td><span class="badge badge-info">${window.escapeHtml(w.wait_type || 'N/A')}</span></td>
            <td>${window.escapeHtml((w.resource_description || '').substring(0, 50))}</td>
            <td>${w.waiting_tasks_count || 0}</td>
            <td>${((w.wait_duration_ms || 0) / 1000).toFixed(1)}s</td>
        </tr>
    `).join('');

    const memoryGrantRows = memoryGrantsData.slice(0, 15).map(m => `
        <tr>
            <td>${m.session_id || 'N/A'}</td>
            <td>${window.escapeHtml(m.database_name || 'N/A')}</td>
            <td>${window.escapeHtml(m.login_name || 'N/A')}</td>
            <td>${((m.granted_memory_kb || 0) / 1024).toFixed(1)} MB</td>
            <td>${((m.used_memory_kb || 0) / 1024).toFixed(1)} MB</td>
            <td>${m.dop || 1}</td>
        </tr>
    `).join('');

    const schedulerRows = schedulerWGData.slice(0, 10).map(s => `
        <tr>
            <td><span class="badge badge-outline">${window.escapeHtml(s.pool_name || 'N/A')}</span></td>
            <td><span class="badge badge-info">${window.escapeHtml(s.group_name || 'N/A')}</span></td>
            <td>${s.active_requests || 0}</td>
            <td>${s.queued_requests || 0}</td>
            <td>${(s.cpu_usage_percent || 0).toFixed(1)}%</td>
        </tr>
    `).join('');

    const procedureRows = procedureStatsData.slice(0, 15).map(p => `
        <tr>
            <td>${window.escapeHtml(p.database_name || 'N/A')}</td>
            <td style="max-width:200px; overflow:hidden; text-overflow:ellipsis;">${window.escapeHtml(p.schema_name || 'dbo')}.${window.escapeHtml(p.object_name || 'N/A')}</td>
            <td>${p.execution_count || 0}</td>
            <td>${(p.total_worker_time_ms || 0).toFixed(0)}ms</td>
            <td>${(p.total_elapsed_time_ms || 0).toFixed(0)}ms</td>
            <td>${(p.total_logical_reads || 0).toLocaleString()}</td>
        </tr>
    `).join('');

    const fileIoRows = fileIoLatencyData.slice(0, 15).map(f => `
        <tr>
            <td>${window.escapeHtml(f.database_name || 'N/A')}</td>
            <td style="font-size:0.7rem;">${window.escapeHtml((f.file_name || '').split('\\').pop())}</td>
            <td>${window.escapeHtml(f.file_type || 'N/A')}</td>
            <td class="${f.read_latency_ms > 10 ? 'text-warning' : ''}">${(f.read_latency_ms || 0).toFixed(1)} ms</td>
            <td class="${f.write_latency_ms > 10 ? 'text-warning' : ''}">${(f.write_latency_ms || 0).toFixed(1)} ms</td>
        </tr>
    `).join('');

    const spinlockRows = spinlockStatsData.slice(0, 10).map(s => `
        <tr>
            <td><span class="badge badge-outline">${window.escapeHtml(s.spinlock_type || 'N/A')}</span></td>
            <td>${(s.collisions || 0).toLocaleString()}</td>
            <td>${(s.spins || 0).toLocaleString()}</td>
            <td>${(s.sleep_time_ms || 0).toLocaleString()}</td>
        </tr>
    `).join('');

    const memoryClerkRows = memoryClerksData.slice(0, 15).map(m => `
        <tr>
            <td style="max-width:150px; overflow:hidden; text-overflow:ellipsis;"><span class="badge badge-outline">${window.escapeHtml(m.clerk_type || 'N/A')}</span></td>
            <td>${m.memory_node || 0}</td>
            <td class="text-accent">${(m.pages_mb || 0).toFixed(1)} MB</td>
            <td>${(m.virtual_memory_reserved_mb || 0).toFixed(1)} MB</td>
            <td>${(m.virtual_memory_committed_mb || 0).toFixed(1)} MB</td>
        </tr>
    `).join('');

    const tempdbRows = tempdbStatsData.map(t => `
        <tr>
            <td><span class="badge badge-${t.file_type === 'DATA' ? 'info' : 'warning'}">${t.file_type || 'N/A'}</span></td>
            <td>${(t.size_mb || 0).toFixed(1)} MB</td>
            <td>${(t.used_mb || 0).toFixed(1)} MB</td>
            <td>${(t.free_mb || 0).toFixed(1)} MB</td>
            <td class="${(t.used_percent || 0) > 80 ? 'text-danger fw-bold' : (t.used_percent > 60 ? 'text-warning' : '')}">${(t.used_percent || 0).toFixed(1)}%</td>
        </tr>
    `).join('');

    const planCacheLatest = Array.isArray(planCacheHealthData) && planCacheHealthData.length > 0 ? planCacheHealthData[0] : null;
    const planCachePct = planCacheLatest ? (planCacheLatest.single_use_cache_pct || 0) : 0;
    const planCacheRows = planCacheLatest ? `
        <tr><td>Total Cache</td><td class="text-accent">${(planCacheLatest.total_cache_mb || 0).toFixed(1)} MB</td></tr>
        <tr><td>Single-use Cache</td><td class="${planCachePct > 40 ? 'text-danger fw-bold' : (planCachePct > 20 ? 'text-warning' : '')}">${(planCacheLatest.single_use_cache_mb || 0).toFixed(1)} MB (${planCachePct.toFixed(1)}%)</td></tr>
        <tr><td>Adhoc</td><td>${(planCacheLatest.adhoc_cache_mb || 0).toFixed(1)} MB</td></tr>
        <tr><td>Prepared</td><td>${(planCacheLatest.prepared_cache_mb || 0).toFixed(1)} MB</td></tr>
        <tr><td>Proc</td><td>${(planCacheLatest.proc_cache_mb || 0).toFixed(1)} MB</td></tr>
    ` : '';

    const memGrantWaiterRows = memoryGrantWaitersData.slice(0, 15).map(m => `
        <tr>
            <td>${m.session_id || 'N/A'}</td>
            <td>${window.escapeHtml(m.database_name || 'N/A')}</td>
            <td>${window.escapeHtml(m.login_name || 'N/A')}</td>
            <td>${((m.requested_memory_kb || 0) / 1024).toFixed(1)} MB</td>
            <td>${(m.wait_time_ms || 0).toLocaleString()}</td>
            <td style="max-width:320px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${window.escapeHtml(m.query_text || '')}</td>
        </tr>
    `).join('');

    const tempdbConsumerRows = tempdbTopConsumersData.slice(0, 15).map(t => `
        <tr>
            <td>${t.session_id || 'N/A'}</td>
            <td>${window.escapeHtml(t.database_name || 'N/A')}</td>
            <td>${(t.tempdb_mb || 0).toFixed(1)} MB</td>
            <td>${(t.user_objects_mb || 0).toFixed(1)} MB</td>
            <td>${(t.internal_objects_mb || 0).toFixed(1)} MB</td>
            <td style="max-width:320px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${window.escapeHtml(t.query_text || '')}</td>
        </tr>
    `).join('');

    const waitCatRows = waitCategoriesData.slice(0, 12).map(w => `
        <tr>
            <td><span class="badge badge-outline">${window.escapeHtml(w.category || w.wait_category || 'N/A')}</span></td>
            <td>${((w.wait_time_ms || w.wait_time_ms_delta || 0) / 1000).toFixed(1)}s</td>
        </tr>
    `).join('');

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div style="flex:1; min-width:0;">
                    <div class="dashboard-title-line">
                        <h1>Enterprise Metrics</h1>
                        <span class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Advanced Performance Monitoring</span>
                    </div>
                    <p class="text-muted enterprise-metrics-intro" style="margin:0.4rem 0 0 0; font-size:0.8rem; line-height:1.45; white-space:nowrap; overflow-x:auto; overflow-y:hidden; max-width:100%; -webkit-overflow-scrolling:touch;">
                        This diagnostic view provides real-time visibility into the SQL Server database engine, highlighting active bottlenecks, memory pressure, storage performance, and query execution efficiency.
                    </p>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:0.75rem; flex-wrap:wrap; justify-content:flex-end;">
                    <span id="enterpriseDataSourceBadge" class="badge badge-info" style="display:none; font-size:0.65rem;">Source</span>
                    <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="enterpriseMetricsLastUpdate">--:--:--</span></span>
                    <label class="flex-between" style="align-items:center; gap:0.5rem; font-size:0.8rem; cursor:pointer;">
                        <input type="checkbox" id="enterpriseMetricsAutoRefresh" checked style="width:16px; height:16px;"> Auto-refresh (10s)
                    </label>
                    <button class="btn btn-sm btn-outline text-accent" onclick="window.EnterpriseMetricsView()"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            <style>
                .info-tooltip { position: relative; display:inline-flex; align-items:center; justify-content:center; width:16px; height:16px; margin-left:6px; cursor:help; background:var(--bg-tertiary,#e5e7eb); color:var(--text-muted,#6b7280); border-radius:50%; font-size:0.65rem; font-weight:600; z-index:10000; }
                .info-tooltip:hover { background:var(--accent,#3b82f6); color:#fff; }
                .info-tooltip[title]:hover::after,
                .info-tooltip[data-tooltip]:hover::after {
                    content: attr(title);
                    position: fixed;
                    top: auto;
                    left: 50%;
                    transform: translateX(-50%);
                    background: #1f2937;
                    color: #f3f4f6;
                    padding: 8px 12px;
                    border-radius: 6px;
                    font-size: 0.7rem;
                    white-space: normal;
                    max-width: 250px;
                    z-index: 99999;
                    box-shadow: 0 4px 12px rgba(0,0,0,0.3);
                    pointer-events: none;
                }
                .info-tooltip[data-tooltip]:hover::after {
                    content: attr(data-tooltip);
                }
                .metric-card { position: relative; z-index: auto; overflow: visible; }
                .metrics-grid { overflow: visible; }
            </style>

            <div class="glass-panel dashboard-strip-panel" style="margin-top:0.5rem;">
                <div class="dashboard-strip-header">
                    <h4><i class="fa-solid fa-gauge-high text-accent"></i> Engine snapshot</h4>
                </div>
                <div style="padding:0.65rem;">
            <div class="metrics-grid" style="display:grid; grid-template-columns:repeat(4,1fr); gap:0.5rem;">
                <div class="metric-card glass-panel status-healthy" style="padding:0.5rem;">
                    <div class="metric-header" style="font-size:0.7rem; display:flex; align-items:center;"><span class="metric-title">Latch Waits</span><span class="info-tooltip" title="Tracks internal engine synchronization delays. Spikes often point to tempdb contention or buffer pool memory constraints.">?</span></div>
                    <div class="metric-value" style="font-size:1.1rem;">${latchData.length}</div>
                </div>
                <div class="metric-card glass-panel status-healthy" style="padding:0.5rem;">
                    <div class="metric-header" style="font-size:0.7rem; display:flex; align-items:center;"><span class="metric-title">Waiting Tasks</span><span class="info-tooltip" title="Shows active sessions currently waiting on resources to proceed. High counts indicate blocking, locking, or resource starvation.">?</span></div>
                    <div class="metric-value" style="font-size:1.1rem;">${waitingData.length}</div>
                </div>
                <div class="metric-card glass-panel status-healthy" style="padding:0.5rem;">
                    <div class="metric-header" style="font-size:0.7rem; display:flex; align-items:center;"><span class="metric-title">Memory Grants</span><span class="info-tooltip" title="Displays queries requesting or actively using workspace memory for sorting and hashing. Crucial for diagnosing RESOURCE_SEMAPHORE waits.">?</span></div>
                    <div class="metric-value" style="font-size:1.1rem;">${memoryGrantsData.length}</div>
                </div>
                <div class="metric-card glass-panel status-healthy" style="padding:0.5rem;">
                    <div class="metric-header" style="font-size:0.7rem; display:flex; align-items:center;"><span class="metric-title">Single-use Plans</span><span class="info-tooltip" title="Single-use plan cache pressure is a common sign of ad hoc workload thrash. High % suggests 'Optimize for ad hoc workloads' / forced parameterization review.">?</span></div>
                    <div class="metric-value" style="font-size:1.1rem;">${planCacheLatest ? `${planCachePct.toFixed(1)}%` : '--'}</div>
                </div>
            </div>
                </div>
            </div>

            <div class="charts-grid mt-3" style="display:grid; grid-template-columns:1fr 1fr; gap:0.75rem;">
                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-hourglass-half text-accent"></i> Wait Categories (15m) <span class="info-tooltip" title="Aggregated wait time deltas by category over the last 15 minutes. Use this to quickly identify the dominant bottleneck class (IO, locks, CPU, log, etc.).">?</span></h3></div>
                    <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead><tr><th>Category</th><th>Wait time</th></tr></thead>
                            <tbody>${waitCatRows || '<tr><td colspan="2" class="text-center text-muted">No wait category data</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-layer-group text-accent"></i> Plan Cache Health <span class="info-tooltip" title="Tracks cache size breakdown and single-use plan pressure. High single-use % can cause CPU overhead and memory waste.">?</span></h3></div>
                    <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead><tr><th>Metric</th><th>Value</th></tr></thead>
                            <tbody>${planCacheRows || '<tr><td colspan="2" class="text-center text-muted">No plan cache data yet</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-database text-accent"></i> File I/O Latency <span class="info-tooltip" title="Measures the physical read and write delays (in milliseconds) at the database file level. Sustained spikes above 15ms indicate storage bottlenecks.">?</span></h3></div>
                    <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr><th>Database</th><th>File</th><th>Type</th><th>Read Latency</th><th>Write Latency</th></tr></thead>
                            <tbody>${fileIoRows || '<tr><td colspan="5" class="text-center text-muted">No user database file I/O data</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-procedures text-accent"></i> Procedure Stats <span class="info-tooltip" title="Execution metrics for cached stored procedures. Identifies the most frequently run or resource-intensive procedures over the collection window.">?</span></h3></div>
                    <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr><th>Database</th><th>Procedure</th><th>Exec Count</th><th>CPU ms</th><th>Duration</th><th>Logical Reads</th></tr></thead>
                            <tbody>${procedureRows || '<tr><td colspan="6" class="text-center text-muted">No procedure stats</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-spinner text-accent"></i> Spinlock Stats <span class="info-tooltip" data-tooltip="Advanced engine metrics tracking ultra-low-level thread synchronization. Used primarily for troubleshooting extreme CPU concurrency issues.">?</span></h3></div>
                    <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr><th>Spinlock Type</th><th>Collisions</th><th>Spins</th><th>Sleep Time</th></tr></thead>
                            <tbody>${spinlockRows || '<tr><td colspan="4" class="text-center text-muted">No spinlock contention</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-memory text-accent"></i> Memory Clerks <span class="info-tooltip" data-tooltip="Shows how SQL Server is internally allocating its memory (e.g., Buffer Pool vs. Plan Cache) to help identify memory bloat or pressure.">?</span></h3></div>
                    <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr><th>Clerk Type</th><th>Node</th><th>Pages MB</th><th>Reserved</th><th>Committed</th></tr></thead>
                            <tbody>${memoryClerkRows || '<tr><td colspan="5" class="text-center text-muted">No memory clerk data</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>
            </div>

            <div class="charts-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-hand text-accent"></i> Memory Grant Waiters <span class="info-tooltip" title="Queries waiting on workspace memory (RESOURCE_SEMAPHORE). These are often the root cause of throughput collapse under concurrency.">?</span></h3></div>
                    <div class="table-responsive" style="max-height:220px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead><tr><th>SPID</th><th>DB</th><th>Login</th><th>Requested</th><th>Wait ms</th><th>Query</th></tr></thead>
                            <tbody>${memGrantWaiterRows || '<tr><td colspan="6" class="text-center text-muted">No memory grant waiters</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-fire text-accent"></i> TempDB Top Consumers <span class="info-tooltip" title="Sessions currently consuming tempdb (spills, hashes/sorts, version store patterns). Useful for diagnosing tempdb pressure quickly.">?</span></h3></div>
                    <div class="table-responsive" style="max-height:220px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead><tr><th>SPID</th><th>DB</th><th>Total</th><th>User obj</th><th>Internal</th><th>Query</th></tr></thead>
                            <tbody>${tempdbConsumerRows || '<tr><td colspan="6" class="text-center text-muted">No tempdb consumers detected</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-temp text-accent"></i> TempDB Usage</h3></div>
                    <div class="table-responsive" style="max-height:200px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead><tr><th>File Type</th><th>Size</th><th>Used</th><th>Free</th><th>Used %</th></tr></thead>
                            <tbody>${tempdbRows || '<tr><td colspan="5" class="text-center text-muted">No TempDB stats</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>`;
}
