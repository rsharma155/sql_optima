/*
 * SQL Optima — PostgreSQL Storage, Tuples & Vacuum Status.
 */

window.PgStorageView = async function() {
    const inst = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
    const instanceName = inst?.name;
    if (!instanceName || inst?.type !== 'postgres') {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select a PostgreSQL instance first</h3></div>`;
        return;
    }

    // Initialize state
    window.appState.pgStorage = window.appState.pgStorage || {};
    const state = window.appState.pgStorage;
    
    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    const pad = n => String(n).padStart(2, '0');
    const fmtLocal = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

    state.fromLocal = state.fromLocal || fmtLocal(oneHourAgo);
    state.toLocal = state.toLocal || fmtLocal(now);
    if (state.db === undefined) state.db = 'all';

    // Helper functions
    const esc = (v) => window.escapeHtml ? window.escapeHtml(String(v ?? '')) : String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
    const fmtBytes = (n) => {
        const v = Number(n ?? 0);
        if (!isFinite(v) || v <= 0) return '-';
        const u = ['B','KB','MB','GB','TB'];
        let x = v, i = 0;
        while (x >= 1024 && i < u.length - 1) { x /= 1024; i++; }
        return `${x.toFixed(i >= 2 ? 2 : 0)} ${u[i]}`;
    };
    const fmtTs = (s) => { try { return s ? new Date(s).toLocaleString() : '—'; } catch { return s || '—'; } };
    const fmtDur = (secs) => {
        const s = Number(secs ?? 0);
        if (!isFinite(s) || s < 0) return '—';
        if (s < 60) return `${s.toFixed(0)}s`;
        if (s < 3600) return `${(s/60).toFixed(1)}m`;
        return `${(s/3600).toFixed(2)}h`;
    };
    const riskBadge = (level) => {
        const map = { low: 'text-success', medium: 'text-warning', high: 'text-danger', critical: 'text-danger' };
        const cls = map[level] || 'text-muted';
        return `<span class="${cls}">${esc(level?.toUpperCase() ?? '—')}</span>`;
    };

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1><i class="fa-solid fa-hard-drive"></i> Storage &amp; Maintenance</h1>
                    <span class="subtitle">Table Bloat, Unused Indexes, and Autovacuum Analysis</span>
                </div>
                <div class="flex-center dashboard-page-title-actions" style="gap: 0.75rem;">
                    <div class="glass-panel" style="padding: 0.2rem 0.5rem; display: flex; align-items: center; gap: 0.5rem; font-size: 0.75rem;">
                        <input type="datetime-local" id="pgStorageFrom" class="bg-transparent border-none text-primary" style="width:12rem;" value="${state.fromLocal}" />
                        <span class="text-muted">→</span>
                        <input type="datetime-local" id="pgStorageTo" class="bg-transparent border-none text-primary" style="width:12rem;" value="${state.toLocal}" />
                        <select id="pgStorageDb" class="bg-transparent border-none text-primary" style="max-width:120px;"><option value="all">All DBs</option></select>
                        <button type="button" class="btn btn-xs btn-accent" id="pgStorageApply">Apply</button>
                    </div>
                    <button id="pgStorageRefreshBtn" class="btn btn-sm btn-outline text-accent"><i class="fa-solid fa-sync"></i> Refresh</button>
                </div>
            </div>

            <!-- Tab Nav -->
            <div class="tabs-container">
                <button class="tab-btn active" data-tab="overview">Overview &amp; Growth</button>
                <button class="tab-btn" data-tab="bloat">Bloat Analysis<span class="tab-badge" id="tabBadge-bloat" style="display:none;"></span></button>
                <button class="tab-btn" data-tab="risks">Maintenance Risks<span class="tab-badge" id="tabBadge-risks" style="display:none;"></span></button>
                <button class="tab-btn" data-tab="indices">Index Efficiency</button>
            </div>

            <!-- OVERVIEW TAB -->
            <div id="pgStorageTab-overview" class="tab-panel">
                <div class="kpi-row mt-3">
                    <div class="glass-panel metric-card-compact h-kpi">
                        <div class="metric-label">Total DB Size</div>
                        <div class="metric-value text-accent" id="val-total-db-size">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact h-kpi">
                        <div class="metric-label">7d Growth</div>
                        <div class="metric-value text-warning" id="val-growth-7d">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact h-kpi">
                        <div class="metric-label">Worst Dead %</div>
                        <div class="metric-value text-danger" id="val-worst-dead-pct">--</div>
                        <div class="text-muted" id="sub-worst-dead-pct" style="font-size:0.6rem;"></div>
                    </div>
                    <div class="glass-panel metric-card-compact h-kpi">
                        <div class="metric-label">Unused Indexes</div>
                        <div class="metric-value" id="val-unused-idx-count">--</div>
                        <div class="text-muted" id="sub-unused-idx-size" style="font-size:0.6rem;"></div>
                    </div>
                </div>

                <div class="grid-container mt-3">
                    <div class="col-4 col-laptop-4 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Bloat Estimation</h3></div>
                            <div class="chart-container" style="height:210px;"><canvas id="pgBloatChart"></canvas></div>
                        </div>
                    </div>
                    <div class="col-8 col-laptop-8 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Autovacuum Activity Trend</h3></div>
                            <div class="chart-container" style="height:210px;"><canvas id="pgVacChart"></canvas></div>
                        </div>
                    </div>
                </div>
                
                <div class="grid-container mt-3">
                    <div class="col-7 col-laptop-8 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Database Growth History</h3></div>
                            <div class="chart-container" style="height:230px;"><canvas id="pgGrowthTrendChart"></canvas></div>
                        </div>
                    </div>
                    <div class="col-5 col-laptop-4 col-tablet-6">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.8rem; margin:0;">Live Vacuum Progress</h3></div>
                            <div class="table-container-compact" style="height:230px; overflow-y:auto;">
                                <table class="modern-table modern-table-compact">
                                    <thead><tr><th>PID</th><th>Relation</th><th>Phase</th><th>%</th></tr></thead>
                                    <tbody id="pgVacProgressTbody"></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- BLOAT TAB -->
            <div id="pgStorageTab-bloat" class="tab-panel" style="display:none;">
                <div class="grid-container mt-3">
                    <div class="col-12">
                        <div class="card glass-panel">
                            <div class="card-header flex-between">
                                <h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-database text-warning"></i> Table Bloat &amp; Vacuum History</h3>
                                <span id="pgBloatMeta" class="text-muted small"></span>
                            </div>
                            <div class="table-container-compact h-table-md" style="height: calc(100vh - 280px); min-height: 500px;">
                                <table class="modern-table modern-table-compact">
                                    <thead>
                                        <tr>
                                            <th>Table</th><th>Total Size</th><th>Dead %</th><th>Dead Tuples</th><th>Est. Waste</th><th>Last Vacuum</th><th>Lag</th><th>Action</th>
                                        </tr>
                                    </thead>
                                    <tbody id="pgBloatTbody"></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- RISKS TAB -->
            <div id="pgStorageTab-risks" class="tab-panel" style="display:none;">
                <div class="grid-container mt-3">
                    <div class="col-6 col-laptop-12">
                        <div class="card glass-panel h-table-md">
                            <div class="card-header"><h3 style="font-size:0.8rem;margin:0;">Idle In Transaction Sessions</h3></div>
                            <div class="table-container-compact">
                                <table class="modern-table modern-table-compact">
                                    <thead><tr><th>PID</th><th>User</th><th>Idle Dur</th><th>Query</th></tr></thead>
                                    <tbody id="pgIdleTbody"></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                    <div class="col-6 col-laptop-12">
                        <div class="card glass-panel h-table-md">
                            <div class="card-header"><h3 style="font-size:0.8rem;margin:0;">Long-Running Active Transactions</h3></div>
                            <div class="table-container-compact">
                                <table class="modern-table modern-table-compact">
                                    <thead><tr><th>PID</th><th>User</th><th>Txn Dur</th><th>Query</th></tr></thead>
                                    <tbody id="pgLongTxnTbody"></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                    <div class="col-12">
                        <div class="card glass-panel">
                            <div class="card-header"><h3 style="font-size:0.8rem;margin:0;">XID Wraparound Risk</h3></div>
                            <div id="pgXIDBody" style="padding:1rem;"></div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- INDICES TAB -->
            <div id="pgStorageTab-indices" class="tab-panel" style="display:none;">
                <div class="grid-container mt-3">
                    <div class="col-12">
                        <div class="card glass-panel h-chart-md">
                            <div class="card-header"><h3 style="font-size:0.85rem;margin:0;">Index Usage &amp; Efficiency Trend</h3></div>
                            <div class="chart-container" style="height:230px;"><canvas id="pgIndexEfficiencyTrendChart"></canvas></div>
                        </div>
                    </div>
                    <div class="col-12 mt-3">
                        <div class="card glass-panel">
                            <div class="card-header"><h3 style="font-size:0.85rem;margin:0;">Index Bloat &amp; Inefficiency</h3></div>
                            <div class="table-container-compact h-table-md">
                                <table class="modern-table modern-table-compact">
                                    <thead>
                                        <tr>
                                            <th class="sortable" data-col="database_name">DB</th>
                                            <th class="sortable" data-col="table">Table</th>
                                            <th class="sortable" data-col="index_name">Index</th>
                                            <th class="sortable" data-col="index_bytes">Size</th>
                                            <th class="sortable" data-col="idx_scans">Scans</th>
                                            <th>Recommendation</th>
                                        </tr>
                                    </thead>
                                    <tbody id="pgIdxBloatTbody"></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;

    // Tab switching
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            document.querySelectorAll('.tab-panel').forEach(p => p.style.display = 'none');
            btn.classList.add('active');
            const panel = document.getElementById(`pgStorageTab-${btn.dataset.tab}`);
            if (panel) panel.style.display = '';
        });
    });

    const loadAllData = async () => {
        const isStillActive = () => window.appState.activeViewId === 'pg-storage' || window.appState.activeViewId === 'pg-autovacuum';
        const setT = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        const setH = (id, val) => { const el = document.getElementById(id); if (el) el.innerHTML = val; };

        const elFrom = document.getElementById('pgStorageFrom');
        const elTo = document.getElementById('pgStorageTo');
        const elDb = document.getElementById('pgStorageDb');
        if (!elFrom || !elTo || !elDb) return;

        const fromLocal = elFrom.value;
        const toLocal = elTo.value;
        const fromIso = new Date(fromLocal).toISOString();
        const toIso = new Date(toLocal).toISOString();
        const selectedDb = elDb.value;
        const dbParam = (selectedDb && selectedDb !== 'all') ? `&db=${encodeURIComponent(selectedDb)}` : '';

        const [storageResp, histResp, vacResp, bloatResp, idleResp, xidResp, longTxnResp, idxBloatResp, sihResp, idxUsageResp] = await Promise.allSettled([
            window.apiClient.authenticatedFetch(`/api/postgres/storage?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/control-center/history?instance=${encodeURIComponent(instanceName)}&limit=180`),
            window.apiClient.authenticatedFetch(`/api/postgres/vacuum/progress?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/bloat?instance=${encodeURIComponent(instanceName)}&limit=200`),
            window.apiClient.authenticatedFetch(`/api/postgres/idle-in-transaction?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/xid-wraparound?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/long-running-transactions?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/index-bloat?instance=${encodeURIComponent(instanceName)}&limit=200`),
            window.apiClient.authenticatedFetch(`/api/timescale/storage-index-health/dashboard?engine=postgres&instance=${encodeURIComponent(instanceName)}&from=${fromIso}&to=${toIso}${dbParam}`),
            window.apiClient.authenticatedFetch(`/api/timescale/storage-index-health/index-usage?engine=postgres&instance=${encodeURIComponent(instanceName)}&from=${fromIso}&to=${toIso}&limit=500${dbParam}`),
        ]);

        if (!isStillActive()) return;

        // --- 1. SIH DASHBOARD (TIME-SERIES) ---
        if (sihResp.status === 'fulfilled' && sihResp.value.ok) {
            const sih = await sihResp.value.json();
            if (!isStillActive()) return;
            const k = sih.kpis || {};
            setT('val-total-db-size', k.total_db_size_mb ? `${k.total_db_size_mb.toFixed(1)} MB` : '--');
            setT('val-growth-7d', k.growth_7d_pct ? `${k.growth_7d_pct.toFixed(2)}%` : '0.0%');
            setT('val-unused-idx-count', k.unused_index_count || '0');
            setT('sub-unused-idx-size', k.unused_index_mb ? `${k.unused_index_mb.toFixed(1)} MB waste` : '0 MB');

            // Growth Trend Chart
            setTimeout(() => {
                const ctx = document.getElementById('pgGrowthTrendChart');
                if (ctx) {
                    if (window.currentCharts?.pgGrowth) window.currentCharts.pgGrowth.destroy();
                    window.currentCharts = window.currentCharts || {};
                    if (!sih.growth || sih.growth.length === 0) {
                        const p = ctx.parentElement;
                        const w = p ? p.offsetWidth || 300 : 300;
                        const h = p ? p.offsetHeight || 150 : 150;
                        ctx.width = w; ctx.height = h;
                        const c = ctx.getContext('2d');
                        c.clearRect(0, 0, w, h);
                        c.fillStyle = 'rgba(100,116,139,0.35)';
                        c.font = '11px sans-serif';
                        c.textAlign = 'center';
                        c.fillText('No growth data yet — collected each cycle', w / 2, h / 2);
                    } else {
                        window.currentCharts.pgGrowth = new Chart(ctx.getContext('2d'), {
                            type: 'line',
                            data: {
                                labels: sih.growth.map(p => new Date(p.bucket).toLocaleDateString()),
                                datasets: [
                                    { label: 'Table Size (MB)', data: sih.growth.map(p => p.table_size_mb), borderColor: window.getCSSVar('--accent'), backgroundColor: 'rgba(56,189,248,0.1)', fill: true, tension: 0.3, pointRadius: 2 },
                                    { label: 'Index Size (MB)', data: sih.growth.map(p => p.index_size_mb), borderColor: window.getCSSVar('--info'), tension: 0.3, pointRadius: 0 }
                                ]
                            },
                            options: { 
                                responsive: true, maintainAspectRatio: false, 
                                plugins: { legend: { position: 'top', labels: { color: '#94a3b8', font: { size: 10 } } } },
                                scales: { 
                                    x: { grid: { display: false }, ticks: { color: '#64748b', font: { size: 9 } } },
                                    y: { title: { display: true, text: 'Size (MB)', color: '#64748b' }, ticks: { color: '#64748b', font: { size: 9 } } }
                                }
                            }
                        });
                    }
                }
            }, 100);
        }

        // --- 2. INDEX EFFICIENCY TREND ---
        if (idxUsageResp.status === 'fulfilled' && idxUsageResp.value.ok) {
            const data = await idxUsageResp.value.json();
            if (!isStillActive()) return;
            const points = data.points || [];
            const timeMap = {};
            points.forEach(p => {
                const t = new Date(p.time).toISOString().slice(0, 16);
                if (!timeMap[t]) timeMap[t] = { scans: 0, updates: 0, size: 0, count: 0 };
                timeMap[t].scans += (p.scans || 0);
                timeMap[t].updates += (p.updates || 0);
                timeMap[t].size += (p.index_size_mb || 0);
                timeMap[t].count++;
            });
            const sortedLabels = Object.keys(timeMap).sort();

            setTimeout(() => {
                const ctx = document.getElementById('pgIndexEfficiencyTrendChart');
                if (ctx) {
                    if (window.currentCharts?.pgIdxTrend) window.currentCharts.pgIdxTrend.destroy();
                    window.currentCharts = window.currentCharts || {};
                    if (sortedLabels.length === 0) {
                        const p = ctx.parentElement;
                        const w = p ? p.offsetWidth || 300 : 300;
                        const h = p ? p.offsetHeight || 150 : 150;
                        ctx.width = w; ctx.height = h;
                        const c = ctx.getContext('2d');
                        c.clearRect(0, 0, w, h);
                        c.fillStyle = 'rgba(100,116,139,0.35)';
                        c.font = '11px sans-serif';
                        c.textAlign = 'center';
                        c.fillText('No index usage data in this time range', w / 2, h / 2);
                    } else {
                        window.currentCharts.pgIdxTrend = new Chart(ctx.getContext('2d'), {
                            type: 'line',
                            data: {
                                labels: sortedLabels.map(l => new Date(l).toLocaleString()),
                                datasets: [
                                    { label: 'Index Scans', data: sortedLabels.map(l => timeMap[l].scans), borderColor: window.getCSSVar('--success'), tension: 0.3, yAxisID: 'y' },
                                    { label: 'Total Index Size (MB)', data: sortedLabels.map(l => timeMap[l].size), borderColor: window.getCSSVar('--warning'), backgroundColor: 'rgba(245,158,11,0.1)', fill: true, tension: 0.3, yAxisID: 'y1' }
                                ]
                            },
                            options: {
                                responsive: true, maintainAspectRatio: false,
                                plugins: { legend: { position: 'top', labels: { color: '#94a3b8', font:{size:10} } } },
                                scales: {
                                    x: { grid: { display: false }, ticks: { color: '#64748b', font:{size:9}, maxRotation: 45 } },
                                    y: { title: { display: true, text: 'Scans', color: '#64748b' }, position: 'left', ticks: { color: '#64748b' } },
                                    y1: { title: { display: true, text: 'Size (MB)', color: '#64748b' }, position: 'right', grid: { display: false }, ticks: { color: '#64748b' } }
                                }
                            }
                        });
                    }
                }
            }, 100);
        }

        // --- 3. OVERVIEW & CHARTS ---
        if (storageResp.status === 'fulfilled' && storageResp.value.ok) {
            const data = await storageResp.value.json();
            if (!isStillActive()) return;
            if (data) {
                const tables = data.tables || [];
                
                const worstDead = tables.reduce((best, t) => (!best || t.dead_pct > best.dead_pct) ? t : best, null);
                if (worstDead) {
                    setT('val-worst-dead-pct', `${worstDead.dead_pct.toFixed(1)}%`);
                    setT('sub-worst-dead-pct', `${worstDead.schema_name}.${worstDead.table_name}`);
                }

                setTimeout(() => {
                    window.currentCharts = window.currentCharts || {};
                    const totalBloat = tables.length > 0 ? tables.reduce((sum, t) => sum + (t.dead_pct || 0), 0) / tables.length : 0;
                    const bloatCtx = document.getElementById('pgBloatChart');
                    if (bloatCtx) {
                        if (window.currentCharts.pgBloat) window.currentCharts.pgBloat.destroy();
                        window.currentCharts.pgBloat = new Chart(bloatCtx.getContext('2d'), {
                            type: 'doughnut', data: {
                                labels: ['Bloat', 'Live'], 
                                datasets: [{ data: [totalBloat, 100-totalBloat], backgroundColor: [window.getCSSVar('--danger'), window.getCSSVar('--success')], borderWidth: 0 }]
                            }, options: {responsive:true, maintainAspectRatio:false, cutout:'75%', plugins:{legend:{position:'bottom', labels:{boxWidth:10, font:{size:10}}}}}
                        });
                    }
                }, 50);
            }
        }

        if (histResp.status === 'fulfilled' && histResp.value.ok) {
            const data = await histResp.value.json();
            if (!isStillActive()) return;
            const history = data.history || {};
            const vacLabels = history.labels || [];
            const vacWorkers = history.autovacuum_workers || [];
            setTimeout(() => {
                const vacCtx = document.getElementById('pgVacChart');
                if (vacCtx) {
                    if (window.currentCharts.pgVac) window.currentCharts.pgVac.destroy();
                    window.currentCharts = window.currentCharts || {};
                    if (vacLabels.length === 0) {
                        const p = vacCtx.parentElement;
                        const w = p ? p.offsetWidth || 300 : 300;
                        const h = p ? p.offsetHeight || 150 : 150;
                        vacCtx.width = w; vacCtx.height = h;
                        const c = vacCtx.getContext('2d');
                        c.clearRect(0, 0, w, h);
                        c.fillStyle = 'rgba(100,116,139,0.35)';
                        c.font = '11px sans-serif';
                        c.textAlign = 'center';
                        c.fillText('No autovacuum history yet — collected each cycle', w / 2, h / 2);
                    } else {
                        window.currentCharts.pgVac = new Chart(vacCtx.getContext('2d'), {
                            type: 'line', data: {
                                labels: vacLabels.map(l => {
                                    try { return new Date(l).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }); }
                                    catch (e) { return l; }
                                }),
                                datasets: [{ label:'Autovacuum workers', data: vacWorkers, borderColor: window.getCSSVar('--warning'), backgroundColor: 'rgba(245,158,11,0.15)', fill:true, tension:0.25, pointRadius:0 }]
                            }, options: { responsive:true, maintainAspectRatio:false, plugins:{legend:{display:false}}, scales:{ y:{ beginAtZero:true } } }
                        });
                    }
                }
            }, 50);
        }

        if (vacResp.status === 'fulfilled' && vacResp.value.ok) {
            const data = await vacResp.value.json();
            if (!isStillActive()) return;
            const prog = data.progress || [];
            const filteredProg = (selectedDb && selectedDb !== 'all') ? prog.filter(v => v.database_name === selectedDb) : prog;
            
            let html = '';
            if (filteredProg.length > 0) {
                html = filteredProg.map(v => {
                    const pct = v.heap_blks_total > 0 ? (v.heap_blks_scanned / v.heap_blks_total) * 100 : 0;
                    return `
                    <tr>
                        <td>${v.pid}</td>
                        <td title="${esc(v.relation_name)}">${esc(v.relation_name)}</td>
                        <td>${esc(v.phase)}</td>
                        <td>${pct.toFixed(1)}%</td>
                    </tr>
                `}).join('');
            } else {
                html = '<tr><td colspan="4" class="text-center text-muted">No vacuum progress running</td></tr>';
            }
            setH('pgVacProgressTbody', html);
        }

        // --- 4. BLOAT TAB ---
        if (bloatResp.status === 'fulfilled' && bloatResp.value.ok) {
            const data = await bloatResp.value.json();
            if (!isStillActive()) return;
            let rows = data.tables || [];
            if (selectedDb && selectedDb !== 'all') {
                rows = rows.filter(r => r.database_name === selectedDb);
            }
            setT('pgBloatMeta', `${rows.length} tables analyzed`);
            const highBloatCount = rows.filter(r => r.dead_pct > 20).length;
            setT('tabBadge-bloat', highBloatCount);
            const badgeEl = document.getElementById('tabBadge-bloat');
            if (badgeEl) badgeEl.style.display = highBloatCount > 0 ? '' : 'none';

            const html = rows.length > 0
                ? rows.map(r => {
                    const vacLagSec = Number(r.vacuum_lag_seconds || 0);
                    const vacLagStr = vacLagSec < 0 ? 'Never' : vacLagSec < 86400 ? `${(vacLagSec/3600).toFixed(1)}h` : `${(vacLagSec/86400).toFixed(1)}d`;
                    return `<tr>
                        <td><strong>${esc(r.schema_name)}.${esc(r.table_name)}</strong></td>
                        <td>${esc(r.total_pretty || fmtBytes(r.total_bytes))}</td>
                        <td><span class="badge ${r.dead_pct > 20 ? 'badge-danger' : 'badge-info'}">${r.dead_pct.toFixed(1)}%</span></td>
                        <td>${Number(r.dead_tuples).toLocaleString()}</td>
                        <td>${Number(r.estimated_waste_mb || 0).toFixed(1)} MB</td>
                        <td class="text-muted small">${fmtTs(r.last_autovacuum)}</td>
                        <td class="${vacLagSec > 86400 ? 'text-warning' : ''}">${vacLagStr}</td>
                        <td class="small">${esc(r.recommendation)}</td>
                    </tr>`;
                }).join('')
                : '<tr><td colspan="8" class="text-center text-muted" style="padding:1.2rem;">No bloat data — run a live VACUUM ANALYZE or wait for autovacuum to populate statistics.</td></tr>';
            setH('pgBloatTbody', html);
        }

        // --- 5. SESSIONS & RISKS ---
        let riskCount = 0;
        if (idleResp.status === 'fulfilled' && idleResp.value.ok) {
            const data = await idleResp.value.json();
            if (!isStillActive()) return;
            let s = data.sessions || [];
            if (selectedDb && selectedDb !== 'all') {
                s = s.filter(sess => (sess.database_name || sess.database) === selectedDb);
            }
            riskCount += s.length;
            setH('pgIdleTbody', s.map(sess => `
                <tr>
                    <td>${sess.pid}</td><td>${esc(sess.user_name)}</td>
                    <td class="${sess.idle_seconds > 300 ? 'text-danger' : ''}">${fmtDur(sess.idle_seconds)}</td>
                    <td class="small">${esc(sess.query.slice(0,60))}…</td>
                </tr>
            `).join('') || '<tr><td colspan="4" class="text-center text-muted">No idle sessions</td></tr>');
        }

        if (longTxnResp.status === 'fulfilled' && longTxnResp.value.ok) {
            const data = await longTxnResp.value.json();
            if (!isStillActive()) return;
            let t = data.transactions || [];
            if (selectedDb && selectedDb !== 'all') {
                t = t.filter(txn => (txn.database_name || txn.database) === selectedDb);
            }
            riskCount += t.length;
            setH('pgLongTxnTbody', t.map(txn => `
                <tr>
                    <td>${txn.pid}</td><td>${esc(txn.user_name)}</td>
                    <td class="text-warning">${fmtDur(txn.txn_duration_seconds)}</td>
                    <td class="small">${esc(txn.query.slice(0,60))}…</td>
                </tr>
            `).join('') || '<tr><td colspan="4" class="text-center text-muted">No long transactions</td></tr>');
        }
        
        const risksBadge = document.getElementById('tabBadge-risks');
        if (risksBadge) {
            risksBadge.textContent = riskCount;
            risksBadge.style.display = riskCount > 0 ? '' : 'none';
        }

        if (xidResp.status === 'fulfilled' && xidResp.value.ok) {
            const data = await xidResp.value.json();
            if (!isStillActive()) return;
            let dbs = data.databases || [];
            if (selectedDb && selectedDb !== 'all') {
                dbs = dbs.filter(d => d.database_name === selectedDb);
            }
            setH('pgXIDBody', dbs.map(d => `
                <div class="mb-2">
                    <div class="flex-between small mb-1">
                        <strong>${esc(d.database_name)}</strong>
                        <span>${riskBadge(d.risk_level)} — ${d.used_pct.toFixed(1)}% of 2.1B XIDs</span>
                    </div>
                    <div class="progress-bar-container" style="height:8px;"><div class="progress-bar" style="width:${d.used_pct}%; background:${d.risk_level==='critical'?'var(--danger)':'var(--warning)'}"></div></div>
                </div>
            `).join(''));
        }

        // --- 6. INDICES ---
        if (idxBloatResp.status === 'fulfilled' && idxBloatResp.value.ok) {
            const data = await idxBloatResp.value.json();
            if (!isStillActive()) return;
            state.allIndexes = data.indexes || [];
            renderIndexTable();
        }

        // Fetch Trend for Indices
        (async () => {
            try {
                const trendUrl = `/api/timescale/storage-index-health/index-usage-trend?engine=postgres&instance=${encodeURIComponent(instanceName)}&from=${fromIso}&to=${toIso}`;
                const tResp = await window.apiClient.authenticatedFetch(trendUrl);
                if (tResp.ok) {
                    const tData = await tResp.json();
                    const points = tData.points || [];
                    setTimeout(() => {
                        const trendCtx = document.getElementById('pgIndexEfficiencyTrendChart');
                        if (trendCtx) {
                            if (window.currentCharts.pgIdxTrend) window.currentCharts.pgIdxTrend.destroy();
                            window.currentCharts.pgIdxTrend = new Chart(trendCtx.getContext('2d'), {
                                type: 'line',
                                data: {
                                    labels: points.map(p => new Date(p.bucket).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'})),
                                    datasets: [
                                        { label: 'Seeks', data: points.map(p => p.seeks), borderColor: '#3b82f6', tension: 0.3, pointRadius: 0, fill: true, backgroundColor: 'rgba(59, 130, 246, 0.1)' },
                                        { label: 'Scans', data: points.map(p => p.scans), borderColor: '#f43f5e', tension: 0.3, pointRadius: 0, fill: true, backgroundColor: 'rgba(244, 63, 94, 0.1)' }
                                    ]
                                },
                                options: { 
                                    responsive: true, maintainAspectRatio: false,
                                    scales: { y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' } }, x: { grid: { display: false } } },
                                    plugins: { legend: { display: true, position: 'top', align: 'end', labels: { boxWidth: 10, font: { size: 10 } } } }
                                }
                            });
                        }
                    }, 50);
                }
            } catch (e) { console.warn("Index trend fetch failed", e); }
        })();
    };

    const renderIndexTable = () => {
        const elDb = document.getElementById('pgStorageDb');
        if (!elDb) return;
        const selectedDb = elDb.value;
        let idxs = state.allIndexes || [];
        if (selectedDb && selectedDb !== 'all') {
            idxs = idxs.filter(ix => ix.database_name === selectedDb);
        }

        if (state.idxSortCol) {
            idxs.sort((a, b) => {
                const va = a[state.idxSortCol];
                const vb = b[state.idxSortCol];
                const dir = state.idxSortDir === 'asc' ? 1 : -1;
                if (typeof va === 'string') return va.localeCompare(vb) * dir;
                return (va - vb) * dir;
            });
        }

        const html = idxs.map(ix => `
            <tr>
                <td>${esc(ix.database_name)}</td>
                <td>${esc(ix.table_name)}</td>
                <td><strong>${esc(ix.index_name)}</strong></td>
                <td>${esc(ix.index_size)}</td>
                <td>${Number(ix.idx_scans).toLocaleString()}</td>
                <td class="small">${esc(ix.recommendation)}</td>
            </tr>
        `).join('') || '<tr><td colspan="6" class="text-center text-muted">No data</td></tr>';
        const tbody = document.getElementById('pgIdxBloatTbody');
        if (tbody) tbody.innerHTML = html;
    };

    // Load DB filters
    const loadFilters = async () => {
        try {
            const resp = await window.apiClient.authenticatedFetch(`/api/timescale/storage-index-health/filters?engine=postgres&instance=${encodeURIComponent(instanceName)}`);
            if (resp.ok) {
                const filters = await resp.json();
                const dbSelect = document.getElementById('pgStorageDb');
                if (dbSelect) {
                    dbSelect.innerHTML = '<option value="all">All DBs</option>' + (filters.databases || []).map(d => `<option value="${d}" ${state.db === d ? 'selected' : ''}>${d}</option>`).join('');
                }
            }
        } catch (e) { console.error('Failed to load filters', e); }
    };

    const syncState = () => {
        const elFrom = document.getElementById('pgStorageFrom');
        const elTo = document.getElementById('pgStorageTo');
        const elDb = document.getElementById('pgStorageDb');
        if (elFrom) state.fromLocal = elFrom.value;
        if (elTo) state.toLocal = elTo.value;
        if (elDb) state.db = elDb.value;
    };

    const handleApply = async (e) => {
        if (e) e.preventDefault();
        syncState();
        await loadAllData();
    };

    document.getElementById('pgStorageApply')?.addEventListener('click', handleApply);
    document.getElementById('pgStorageRefreshBtn')?.addEventListener('click', handleApply);
    document.getElementById('pgStorageDb')?.addEventListener('change', handleApply);

    // Sorting event listeners
    document.querySelectorAll('#pgStorageTab-indices th.sortable').forEach(th => {
        th.style.cursor = 'pointer';
        th.addEventListener('click', () => {
            const col = th.dataset.col;
            if (state.idxSortCol === col) {
                state.idxSortDir = state.idxSortDir === 'asc' ? 'desc' : 'asc';
            } else {
                state.idxSortCol = col;
                state.idxSortDir = 'desc';
            }
            renderIndexTable();
        });
    });

    await loadFilters();
    await loadAllData();
}
