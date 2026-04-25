/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL Storage, Tuples & Vacuum Status.
 * Merged from Storage Growth and Autovacuum/Bloat dashboards.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
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
                <div class="dashboard-title-line" style="flex:1; min-width:0;">
                    <h1><i class="fa-solid fa-hard-drive text-accent"></i> Storage, Tuples &amp; Vacuum Status</h1>
                    <p class="subtitle">Monitor Table Bloat, Unused Indexes, and Autovacuum effectiveness for ${esc(instanceName)}</p>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:0.6rem; flex-wrap:wrap; justify-content:flex-end;">
                    <div class="glass-panel" style="padding: 0.2rem 0.5rem; display: flex; align-items: center; gap: 0.5rem; font-size: 0.75rem; border: 1px solid var(--border-color);">
                        <label class="text-muted" style="margin:0;">from:</label>
                        <input type="datetime-local" id="pgStorageFrom" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; width:10.5rem;" value="${state.fromLocal}" />
                        <label class="text-muted" style="margin:0;">to:</label>
                        <input type="datetime-local" id="pgStorageTo" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; width:10.5rem;" value="${state.toLocal}" />
                        <div style="border-left:1px solid var(--border-color); padding-left:0.5rem; display:flex; align-items:center; gap:0.4rem;">
                            <label class="text-muted" style="margin:0;">db:</label>
                            <select id="pgStorageDb" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; max-width:120px;"><option value="all">All</option></select>
                        </div>
                        <button type="button" class="btn btn-xs btn-accent" id="pgStorageApply" style="padding:1px 6px;"><i class="fa-solid fa-filter"></i> Apply</button>
                    </div>
                    <button id="pgStorageRefreshBtn" class="btn btn-sm btn-outline text-accent"><i class="fa-solid fa-rotate-right"></i> Refresh</button>
                </div>
            </div>

            <!-- Tab Nav -->
            <div class="tabs-container">
                <button class="tab-btn active" data-tab="overview"><i class="fa-solid fa-chart-line"></i> Overview &amp; Growth</button>
                <button class="tab-btn" data-tab="bloat"><i class="fa-solid fa-broom"></i> Maintenance &amp; Bloat<span class="tab-badge" id="tabBadge-bloat" style="display:none;"></span></button>
                <button class="tab-btn" data-tab="risks"><i class="fa-solid fa-triangle-exclamation"></i> Sessions &amp; Risks<span class="tab-badge" id="tabBadge-risks" style="display:none;"></span></button>
                <button class="tab-btn" data-tab="indices"><i class="fa-solid fa-sitemap"></i> Index Efficiency</button>
            </div>

            <!-- OVERVIEW TAB -->
            <div id="pgStorageTab-overview" class="tab-panel">
                <div class="charts-grid mt-3" style="display:grid; grid-template-columns:1fr 2fr; gap:0.75rem;">
                    <div class="chart-card glass-panel" style="padding:0.75rem;">
                        <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Database Bloat Estimation</h3></div>
                        <div class="chart-container doughnut-container" style="height:140px;"><canvas id="pgBloatChart"></canvas></div>
                    </div>
                    <div class="chart-card glass-panel" style="padding:0.75rem;">
                        <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Autovacuum &amp; Autoanalyze Runs</h3></div>
                        <div class="chart-container" style="height:140px;"><canvas id="pgVacChart"></canvas></div>
                    </div>
                </div>
                
                <div class="glass-panel dashboard-strip-panel mt-3">
                    <div class="dashboard-strip-header">
                        <h4>Storage &amp; Growth KPIs (Time-Series)</h4>
                    </div>
                    <div style="display:grid; grid-template-columns:repeat(4, 1fr); gap:0.75rem;">
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">Total DB Size</div>
                            <div class="strip-metric-value metric-value" id="val-total-db-size">--</div>
                            <div class="text-muted sub">Current Snapshot</div>
                        </div>
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">Growth (7d)</div>
                            <div class="strip-metric-value metric-value" id="val-growth-7d">--</div>
                            <div class="text-muted sub">Weekly Trend</div>
                        </div>
                        <div class="strip-metric-cell" id="metric-worst-dead-pct">
                            <div class="strip-metric-label">Worst Dead %</div>
                            <div class="strip-metric-value metric-value" id="val-worst-dead-pct">--</div>
                            <div class="text-muted sub" id="sub-worst-dead-pct">Loading…</div>
                        </div>
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">Unused Indexes</div>
                            <div class="strip-metric-value metric-value" id="val-unused-idx-count">--</div>
                            <div class="text-muted sub" id="sub-unused-idx-size">Loading…</div>
                        </div>
                    </div>
                </div>

                <div class="table-card glass-panel mt-3">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Database Growth Trend</h3></div>
                    <div class="chart-container" style="height:200px; padding:0.5rem;"><canvas id="pgGrowthTrendChart"></canvas></div>
                </div>

                <div class="table-card glass-panel mt-3">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Vacuum Progress (Live)</h3></div>
                    <div class="table-responsive" style="max-height:220px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead>
                                <tr><th>PID</th><th>Relation</th><th>Phase</th><th>Progress</th><th>Vacuumed</th></tr>
                            </thead>
                            <tbody id="pgVacProgressTbody">
                                <tr><td colspan="5" class="text-center text-muted">No vacuum progress currently running</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- BLOAT & MAINTENANCE TAB -->
            <div id="pgStorageTab-bloat" class="tab-panel" style="display:none;">
                <div class="table-card glass-panel mt-3">
                    <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
                        <h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-database text-warning"></i> Table Bloat &amp; Vacuum History</h3>
                        <span id="pgBloatMeta" class="text-muted small">Loading…</span>
                    </div>
                    <div class="table-responsive" style="overflow-x:auto; max-height:500px;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead>
                                <tr>
                                    <th>Table</th><th>Total Size</th><th>Dead %</th>
                                    <th>Dead Tuples</th><th>Est. Waste</th>
                                    <th>Last Autovacuum</th><th>Vacuum Lag</th><th>Recommendation</th>
                                </tr>
                            </thead>
                            <tbody id="pgBloatTbody">
                                <tr><td colspan="8" class="text-center text-muted">Loading…</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- RISKS & SESSIONS TAB -->
            <div id="pgStorageTab-risks" class="tab-panel" style="display:none;">
                <div id="pgRisksAlerts"></div>
                
                <div class="table-card glass-panel mt-3">
                    <div class="card-header"><h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-hourglass-half text-danger"></i> Idle In Transaction Sessions</h3></div>
                    <div class="table-responsive" style="overflow-x:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead>
                                <tr><th>PID</th><th>User</th><th>Idle Dur</th><th>Wait Event</th><th>Query</th></tr>
                            </thead>
                            <tbody id="pgIdleTbody">
                                <tr><td colspan="5" class="text-center text-muted">Loading…</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel mt-3">
                    <div class="card-header"><h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-clock text-warning"></i> Long-Running Active Transactions (>1 min)</h3></div>
                    <div class="table-responsive" style="overflow-x:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead>
                                <tr><th>PID</th><th>User</th><th>Txn Dur</th><th>Wait Event</th><th>Query</th></tr>
                            </thead>
                            <tbody id="pgLongTxnTbody">
                                <tr><td colspan="5" class="text-center text-muted">No long transactions</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel mt-3">
                    <div class="card-header"><h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-triangle-exclamation text-warning"></i> XID Wraparound Risk by Database</h3></div>
                    <div id="pgXIDBody" style="padding:1rem;">
                        <p class="text-muted text-center">Loading…</p>
                    </div>
                </div>
            </div>

            <!-- INDICES TAB -->
            <div id="pgStorageTab-indices" class="tab-panel" style="display:none;">
                <div class="chart-card glass-panel mt-3" style="padding:0.75rem;">
                    <div class="card-header"><h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-chart-line text-info"></i> Index Usage &amp; Efficiency Trend</h3></div>
                    <div class="chart-container" style="height:250px;"><canvas id="pgIndexEfficiencyTrendChart"></canvas></div>
                </div>
                <div class="table-card glass-panel mt-3">
                    <div class="card-header"><h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-sitemap text-info"></i> Index Bloat &amp; Inefficiency (Top Candidates)</h3></div>
                    <div class="table-responsive" style="overflow-x:auto;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead>
                                <tr>
                                    <th class="sortable" data-col="database_name">Database</th>
                                    <th class="sortable" data-col="table">Table</th>
                                    <th class="sortable" data-col="index_name">Index</th>
                                    <th class="sortable" data-col="index_bytes">Size</th>
                                    <th class="sortable" data-col="idx_scans">Scans</th>
                                    <th>Recommendation</th>
                                </tr>
                            </thead>
                            <tbody id="pgIdxBloatTbody">
                                <tr><td colspan="6" class="text-center text-muted">Loading…</td></tr>
                            </tbody>
                        </table>
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
        const fromIso = new Date(document.getElementById('pgStorageFrom').value).toISOString();
        const toIso = new Date(document.getElementById('pgStorageTo').value).toISOString();
        const selectedDb = document.getElementById('pgStorageDb').value;
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

        // --- 1. SIH DASHBOARD (TIME-SERIES) ---
        if (sihResp.status === 'fulfilled' && sihResp.value.ok) {
            const sih = await sihResp.value.json();
            const k = sih.kpis || {};
            document.getElementById('val-total-db-size').textContent = k.total_db_size_mb ? `${k.total_db_size_mb.toFixed(1)} MB` : '--';
            document.getElementById('val-growth-7d').textContent = k.growth_7d_pct ? `${k.growth_7d_pct.toFixed(2)}%` : '0.0%';
            document.getElementById('val-unused-idx-count').textContent = k.unused_index_count || '0';
            document.getElementById('sub-unused-idx-size').textContent = k.unused_index_mb ? `${k.unused_index_mb.toFixed(1)} MB waste` : '0 MB';

            // Growth Trend Chart
            if (sih.growth && sih.growth.length > 0) {
                setTimeout(() => {
                    const ctx = document.getElementById('pgGrowthTrendChart');
                    if (ctx) {
                        if (window.currentCharts?.pgGrowth) window.currentCharts.pgGrowth.destroy();
                        window.currentCharts = window.currentCharts || {};
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
                }, 100);
            }
        }

        // --- 2. INDEX EFFICIENCY TREND ---
        if (idxUsageResp.status === 'fulfilled' && idxUsageResp.value.ok) {
            const data = await idxUsageResp.value.json();
            const points = data.points || [];
            if (points.length > 0) {
                // Group by time to get total usage trend
                const timeMap = {};
                points.forEach(p => {
                    const t = new Date(p.time).toISOString().slice(0, 16); // group by minute
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
                        if (window.currentCharts?.pgIdxEff) window.currentCharts.pgIdxEff.destroy();
                        window.currentCharts = window.currentCharts || {};
                        window.currentCharts.pgIdxEff = new Chart(ctx.getContext('2d'), {
                            type: 'line',
                            data: {
                                labels: sortedLabels.map(l => new Date(l).toLocaleString()),
                                datasets: [
                                    { label: 'Total Index Scans (Usage)', data: sortedLabels.map(l => timeMap[l].scans), borderColor: window.getCSSVar('--success'), tension: 0.3, yAxisID: 'y' },
                                    { label: 'Total Index Size (MB)', data: sortedLabels.map(l => timeMap[l].size), borderColor: window.getCSSVar('--warning'), backgroundColor: 'rgba(245,158,11,0.1)', fill: true, tension: 0.3, yAxisID: 'y1' }
                                ]
                            },
                            options: {
                                responsive: true, maintainAspectRatio: false,
                                plugins: { legend: { position: 'top', labels: { color: '#94a3b8' } } },
                                scales: {
                                    x: { grid: { display: false }, ticks: { color: '#64748b', maxRotation: 45, minRotation: 45 } },
                                    y: { title: { display: true, text: 'Scans / Seeks', color: '#64748b' }, position: 'left', ticks: { color: '#64748b' } },
                                    y1: { title: { display: true, text: 'Size (MB)', color: '#64748b' }, position: 'right', grid: { display: false }, ticks: { color: '#64748b' } }
                                }
                            }
                        });
                    }
                }, 100);
            }
        }

        // --- 3. OVERVIEW & CHARTS ---
        if (storageResp.status === 'fulfilled' && storageResp.value.ok) {
            const data = await storageResp.value.json();
            const tables = data.tables || [];
            
            const worstDead = tables.reduce((best, t) => (!best || t.bloat_pct > best.bloat_pct) ? t : best, null);
            if (worstDead) {
                document.getElementById('val-worst-dead-pct').textContent = `${worstDead.bloat_pct.toFixed(1)}%`;
                document.getElementById('sub-worst-dead-pct').textContent = `${worstDead.schema}.${worstDead.table}`;
            }

            // Charts
            setTimeout(() => {
                window.currentCharts = window.currentCharts || {};
                const totalBloat = tables.length > 0 ? tables.reduce((sum, t) => sum + (t.bloat_pct || 0), 0) / tables.length : 0;
                const bloatCtx = document.getElementById('pgBloatChart');
                if (bloatCtx) {
                    if (window.currentCharts.pgBloat) window.currentCharts.pgBloat.destroy();
                    window.currentCharts.pgBloat = new Chart(bloatCtx.getContext('2d'), {
                        type: 'doughnut', data: {
                            labels: ['Bloat', 'Live'], 
                            datasets: [{ data: [totalBloat, 100-totalBloat], backgroundColor: [window.getCSSVar('--danger'), window.getCSSVar('--success')], borderWidth: 0 }]
                        }, options: {responsive:true, maintainAspectRatio:false, cutout:'75%', plugins:{legend:{position:'bottom'}}}
                    });
                }
            }, 50);
        }

        if (histResp.status === 'fulfilled' && histResp.value.ok) {
            const data = await histResp.value.json();
            const history = data.history || {};
            setTimeout(() => {
                const vacCtx = document.getElementById('pgVacChart');
                if (vacCtx) {
                    if (window.currentCharts.pgVac) window.currentCharts.pgVac.destroy();
                    window.currentCharts.pgVac = new Chart(vacCtx.getContext('2d'), {
                        type: 'line', data: {
                            labels: history.labels || [],
                            datasets: [{ label:'Autovacuum workers', data: history.autovacuum_workers || [], borderColor: window.getCSSVar('--warning'), backgroundColor: 'rgba(245,158,11,0.15)', fill:true, tension:0.25, pointRadius:0 }]
                        }, options: { responsive:true, maintainAspectRatio:false, scales:{ y:{ beginAtZero:true } } }
                    });
                }
            }, 50);
        }

        if (vacResp.status === 'fulfilled' && vacResp.value.ok) {
            const data = await vacResp.value.json();
            const prog = data.progress || [];
            const tbody = document.getElementById('pgVacProgressTbody');
            const filteredProg = (selectedDb && selectedDb !== 'all') ? prog.filter(v => v.database_name === selectedDb) : prog;
            
            if (filteredProg.length > 0) {
                tbody.innerHTML = filteredProg.map(v => `
                    <tr>
                        <td>${v.pid}</td>
                        <td title="${esc(v.relation_name)}">${esc(v.relation_name)}</td>
                        <td>${esc(v.phase)}</td>
                        <td>${Number(v.progress_pct || 0).toFixed(1)}%</td>
                        <td>${Number(v.heap_blks_vacuumed || 0).toLocaleString()} blks</td>
                    </tr>
                `).join('');
            } else {
                tbody.innerHTML = '<tr><td colspan="5" class="text-center text-muted">No vacuum progress currently running</td></tr>';
            }
        }

        // --- 4. BLOAT TAB ---
        if (bloatResp.status === 'fulfilled' && bloatResp.value.ok) {
            const data = await bloatResp.value.json();
            let rows = data.tables || [];
            if (selectedDb && selectedDb !== 'all') {
                rows = rows.filter(r => r.database_name === selectedDb);
            }
            const tbody = document.getElementById('pgBloatTbody');
            document.getElementById('pgBloatMeta').textContent = `${rows.length} tables analyzed`;
            document.getElementById('tabBadge-bloat').textContent = rows.filter(r => r.dead_pct > 20).length;
            document.getElementById('tabBadge-bloat').style.display = rows.filter(r => r.dead_pct > 20).length > 0 ? '' : 'none';

            tbody.innerHTML = rows.map(r => {
                const vacLagSec = Number(r.vacuum_lag_seconds || 0);
                const vacLagStr = vacLagSec < 0 ? 'Never' : vacLagSec < 86400 ? `${(vacLagSec/3600).toFixed(1)}h` : `${(vacLagSec/86400).toFixed(1)}d`;
                return `<tr>
                    <td><strong>${esc(r.schema)}.${esc(r.table)}</strong></td>
                    <td>${esc(r.total_size || fmtBytes(r.total_bytes))}</td>
                    <td><span class="badge ${r.dead_pct > 20 ? 'badge-danger' : 'badge-info'}">${r.dead_pct.toFixed(1)}%</span></td>
                    <td>${Number(r.dead_tuples).toLocaleString()}</td>
                    <td>${Number(r.estimated_waste_mb || 0).toFixed(1)} MB</td>
                    <td class="text-muted">${fmtTs(r.last_autovacuum)}</td>
                    <td class="${vacLagSec > 86400 ? 'text-warning' : ''}">${vacLagStr}</td>
                    <td class="small">${esc(r.recommendation)}</td>
                </tr>`;
            }).join('');
        }

        // --- 5. SESSIONS & RISKS ---
        let riskCount = 0;
        if (idleResp.status === 'fulfilled' && idleResp.value.ok) {
            const data = await idleResp.value.json();
            let s = data.sessions || [];
            if (selectedDb && selectedDb !== 'all') {
                s = s.filter(sess => sess.database_name === selectedDb);
            }
            riskCount += s.length;
            document.getElementById('pgIdleTbody').innerHTML = s.map(sess => `
                <tr>
                    <td>${sess.pid}</td><td>${esc(sess.user_name)}</td>
                    <td class="${sess.idle_seconds > 300 ? 'text-danger' : ''}">${fmtDur(sess.idle_seconds)}</td>
                    <td>${esc(sess.wait_event)}</td>
                    <td class="small" title="${esc(sess.query)}">${esc(sess.query.slice(0,60))}…</td>
                </tr>
            `).join('') || '<tr><td colspan="5" class="text-center text-muted">No idle sessions</td></tr>';
        }

        if (longTxnResp.status === 'fulfilled' && longTxnResp.value.ok) {
            const data = await longTxnResp.value.json();
            let t = data.transactions || [];
            if (selectedDb && selectedDb !== 'all') {
                t = t.filter(txn => txn.database_name === selectedDb);
            }
            riskCount += t.length;
            document.getElementById('pgLongTxnTbody').innerHTML = t.map(txn => `
                <tr>
                    <td>${txn.pid}</td><td>${esc(txn.user_name)}</td>
                    <td class="text-warning">${fmtDur(txn.txn_duration_seconds)}</td>
                    <td>${esc(txn.wait_event)}</td>
                    <td class="small" title="${esc(txn.query)}">${esc(txn.query.slice(0,60))}…</td>
                </tr>
            `).join('') || '<tr><td colspan="5" class="text-center text-muted">No long transactions</td></tr>';
        }
        
        const risksBadge = document.getElementById('tabBadge-risks');
        risksBadge.textContent = riskCount;
        risksBadge.style.display = riskCount > 0 ? '' : 'none';

        if (xidResp.status === 'fulfilled' && xidResp.value.ok) {
            const data = await xidResp.value.json();
            let dbs = data.databases || [];
            if (selectedDb && selectedDb !== 'all') {
                dbs = dbs.filter(d => d.database_name === selectedDb);
            }
            document.getElementById('pgXIDBody').innerHTML = dbs.map(d => `
                <div class="mb-2">
                    <div class="flex-between small mb-1">
                        <strong>${esc(d.database_name)}</strong>
                        <span>${riskBadge(d.risk_level)} — ${d.used_pct.toFixed(1)}% of 2.1B XIDs</span>
                    </div>
                    <div class="progress-bar-container" style="height:8px;"><div class="progress-bar" style="width:${d.used_pct}%; background:${d.risk_level==='critical'?'var(--danger)':'var(--warning)'}"></div></div>
                </div>
            `).join('');
        }

        // --- 6. INDICES ---
        if (idxBloatResp.status === 'fulfilled' && idxBloatResp.value.ok) {
            const data = await idxBloatResp.value.json();
            state.allIndexes = data.indexes || [];
            renderIndexTable();
        }
    };

    const renderIndexTable = () => {
        const selectedDb = document.getElementById('pgStorageDb').value;
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

        document.getElementById('pgIdxBloatTbody').innerHTML = idxs.map(ix => `
            <tr>
                <td>${esc(ix.database_name)}</td>
                <td>${esc(ix.table)}</td>
                <td><strong>${esc(ix.index_name)}</strong></td>
                <td>${esc(ix.index_size)}</td>
                <td>${Number(ix.idx_scans).toLocaleString()}</td>
                <td class="small">${esc(ix.recommendation)}</td>
            </tr>
        `).join('') || '<tr><td colspan="6" class="text-center text-muted">No data</td></tr>';
    };

    // Load DB filters
    const loadFilters = async () => {
        try {
            const resp = await window.apiClient.authenticatedFetch(`/api/timescale/storage-index-health/filters?engine=postgres&instance=${encodeURIComponent(instanceName)}`);
            if (resp.ok) {
                const filters = await resp.json();
                const dbSelect = document.getElementById('pgStorageDb');
                if (dbSelect) {
                    dbSelect.innerHTML = '<option value="all">All</option>' + (filters.databases || []).map(d => `<option value="${d}" ${state.db === d ? 'selected' : ''}>${d}</option>`).join('');
                }
            }
        } catch (e) { console.error('Failed to load filters', e); }
    };

    const syncState = () => {
        state.fromLocal = document.getElementById('pgStorageFrom').value;
        state.toLocal = document.getElementById('pgStorageTo').value;
        state.db = document.getElementById('pgStorageDb').value;
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

