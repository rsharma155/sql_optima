/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL storage utilization and growth.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgStorageView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...'};

    let storageData = { tables: [], indexes: [] };
    let ccHistory = null;
    let vacProgress = [];
    let latestMaint = [];
    let maintHistory = null;
    let storageError = '';
    try {
        const [storageResp, histResp, vacResp, maintLatestResp] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/postgres/storage?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/control-center/history?instance=${encodeURIComponent(inst.name)}&limit=180`),
            window.apiClient.authenticatedFetch(`/api/postgres/vacuum/progress?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/table-maintenance/latest?instance=${encodeURIComponent(inst.name)}&limit=50`),
        ]);
        if (storageResp.ok) {
            const contentType = storageResp.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const payload = await storageResp.json();
                storageError = payload.error || '';
                storageData = payload;
            }
        } else console.error("Failed to load PG storage:", storageResp.status);

        if (histResp.ok) {
            const contentType = histResp.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const payload = await histResp.json();
                ccHistory = payload && payload.history ? payload.history : null;
            }
        } else console.error("Failed to load PG CC history:", histResp.status);

        if (vacResp.ok) {
            const contentType = vacResp.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const payload = await vacResp.json();
                vacProgress = payload && payload.progress ? payload.progress : [];
            }
        }
        if (maintLatestResp.ok) {
            const contentType = maintLatestResp.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const payload = await maintLatestResp.json();
                latestMaint = payload && payload.latest ? payload.latest : [];
            }
        }
    } catch (e) {
        console.error("PG storage fetch failed:", e);
    }

    const tables = storageData.tables || [];
    const indexes = storageData.indexes || [];
    const hasVac = Array.isArray(vacProgress) && vacProgress.length > 0;
    const fmtBytes = (n) => {
        const v = Number(n || 0);
        if (!isFinite(v) || v <= 0) return '--';
        const units = ['B','KB','MB','GB','TB'];
        let x = v, i = 0;
        while (x >= 1024 && i < units.length - 1) { x /= 1024; i++; }
        return `${x.toFixed(i >= 2 ? 2 : 0)} ${units[i]}`;
    };
    const fmtWhen = (ts) => {
        if (!ts) return 'Never';
        try { return new Date(ts).toLocaleString(); } catch { return String(ts); }
    };
    const bloatCandidates = (Array.isArray(latestMaint) ? latestMaint : [])
        .map(r => {
            const totalBytes = Number(r.total_bytes || 0);
            const deadPct = Number(r.dead_pct || 0);
            const wastedBytes = Math.max(0, Math.floor(totalBytes * (deadPct / 100)));
            return { ...r, totalBytes, deadPct, wastedBytes };
        })
        .filter(r => r.totalBytes > 0 && r.deadPct >= 10)
        .sort((a,b) => (b.wastedBytes - a.wastedBytes) || (b.deadPct - a.deadPct))
        .slice(0, 15);
    const recommendation = (r) => {
        if (r.deadPct >= 30) return 'High bloat risk: VACUUM (ANALYZE) first; if persistent, schedule REINDEX/pg_repack window. Consider per-table autovacuum tuning.';
        if (r.deadPct >= 20) return 'Elevated bloat: tune autovacuum for this table (scale factors / cost limits) and monitor dead% trend.';
        return 'Watchlist: monitor dead% trend; tune autovacuum if rising.';
    };
    const worstDeadPct = (tables || []).reduce((best, t) => {
        const pct = Number(t?.bloat_pct || 0);
        if (!best) return { t, pct };
        return pct > best.pct ? { t, pct } : best;
    }, null);
    const mostDeadTuples = (tables || []).reduce((best, t) => {
        const dead = Number(t?.dead_tuples || 0);
        if (!best) return { t, dead };
        return dead > best.dead ? { t, dead } : best;
    }, null);

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title">
                <h1><i class="fa-solid fa-hard-drive text-accent"></i> Storage, Tuples & Vacuum Status</h1>
                <p class="subtitle">Monitor Table Bloat, Unused Indexes, and Autovacuum effectiveness.</p>
            </div>
            
            <div class="charts-grid mt-3" style="display:grid; grid-template-columns:1fr 2fr; gap:0.75rem;">
                <div class="chart-card glass-panel" style="padding:0.75rem;">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Database Bloat Estimation</h3></div>
                    <div class="chart-container doughnut-container" style="height:140px;"><canvas id="pgBloatChart"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="padding:0.75rem;">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Autovacuum & Autoanalyze Runs</h3></div>
                    <div class="chart-container" style="height:140px;"><canvas id="pgVacChart"></canvas></div>
                </div>
            </div>
            <div class="charts-grid mt-3" id="pgDeadTrendRow" style="display:none; grid-template-columns:1fr; gap:0.75rem;">
                <div class="chart-card glass-panel" style="padding:0.75rem;">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Dead Tuple % Trend (Selected Table)</h3></div>
                    <div class="chart-container" style="height:140px;"><canvas id="pgDeadPctTrendChart"></canvas></div>
                    <div class="text-muted" style="font-size:0.75rem; margin-top:0.35rem;" id="pgDeadPctTrendLabel"></div>
                </div>
            </div>

            <div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                ${storageError ? `
                    <div class="alert alert-warning">
                        <i class="fa-solid fa-triangle-exclamation"></i> Storage stats error: ${window.escapeHtml(storageError)}
                    </div>
                ` : ''}
                <div class="glass-panel dashboard-strip-panel">
                    <div class="dashboard-strip-header">
                        <h4>
                            <span class="dashboard-strip-header-icons" aria-hidden="true">
                                <i class="fa-solid fa-chart-pie" title="Table health"></i>
                                <i class="fa-solid fa-broom" title="Vacuum"></i>
                            </span>
                            Top Table Offenders (Quick View)
                        </h4>
                    </div>
                    <div style="display:grid; grid-template-columns:repeat(3, 1fr); gap:0.75rem;">
                        <div class="strip-metric-cell" style="${worstDeadPct?.t ? 'cursor:pointer;' : ''}" ${worstDeadPct?.t ? `onclick="window.pgLoadTableDeadTrend('${window.escapeHtml(worstDeadPct.t.schema).replace(/'/g,'&#39;')}','${window.escapeHtml(worstDeadPct.t.table).replace(/'/g,'&#39;')}')"` : ''} title="Highest dead tuple ratio (bloat%) in the current table list. Click to load dead% trend.">
                            <div class="strip-metric-label">Worst Dead %</div>
                            <div class="strip-metric-value metric-value">${worstDeadPct?.t ? `${Number(worstDeadPct.pct || 0).toFixed(1)}%` : '--'}</div>
                            <div class="text-muted sub">${worstDeadPct?.t ? `${window.escapeHtml(worstDeadPct.t.schema)}.${window.escapeHtml(worstDeadPct.t.table)}` : 'No data'}</div>
                        </div>
                        <div class="strip-metric-cell" style="${mostDeadTuples?.t ? 'cursor:pointer;' : ''}" ${mostDeadTuples?.t ? `onclick="window.pgLoadTableDeadTrend('${window.escapeHtml(mostDeadTuples.t.schema).replace(/'/g,'&#39;')}','${window.escapeHtml(mostDeadTuples.t.table).replace(/'/g,'&#39;')}')"` : ''} title="Highest dead tuples count in the current table list. Click to load dead% trend.">
                            <div class="strip-metric-label">Most Dead Tuples</div>
                            <div class="strip-metric-value metric-value">${mostDeadTuples?.t ? Number(mostDeadTuples.dead || 0).toLocaleString() : '--'}</div>
                            <div class="text-muted sub">${mostDeadTuples?.t ? `${window.escapeHtml(mostDeadTuples.t.schema)}.${window.escapeHtml(mostDeadTuples.t.table)}` : 'No data'}</div>
                        </div>
                        <div class="strip-metric-cell" title="How many tables are in the current Storage snapshot list.">
                            <div class="strip-metric-label">Tables Tracked (snapshot)</div>
                            <div class="strip-metric-value metric-value">${Number(tables.length || 0).toLocaleString()}</div>
                            <div class="text-muted sub">from pg_stat_user_tables</div>
                        </div>
                    </div>
                </div>
                <div class="table-card glass-panel" id="pgBloatCandidatesCard" style="${bloatCandidates.length ? '' : 'display:none;'}">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Bloat Candidates (Impact-based)</h3></div>
                    <div class="text-muted" style="font-size:0.75rem; margin:0.25rem 0 0.5rem 0;">
                        Ranked by estimated wasted bytes \(total_bytes × dead%\) from Timescale table maintenance snapshots.
                    </div>
                    <div class="table-responsive" style="max-height:260px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead>
                                <tr><th>Table</th><th>Size</th><th>Dead %</th><th>Est. Waste</th><th>Last Autovac</th><th>Recommendation</th></tr>
                            </thead>
                            <tbody>
                                ${bloatCandidates.map(r => `
                                    <tr>
                                        <td style="cursor:pointer;" title="Click to load dead tuple trend" onclick="window.pgLoadTableDeadTrend('${window.escapeHtml(r.schema_name || '').replace(/'/g,'&#39;')}','${window.escapeHtml(r.table_name || '').replace(/'/g,'&#39;')}')">
                                            <strong>${window.escapeHtml(r.schema_name || '')}.${window.escapeHtml(r.table_name || '')}</strong>
                                        </td>
                                        <td>${window.escapeHtml(fmtBytes(r.totalBytes))}</td>
                                        <td><span class="badge ${r.deadPct >= 30 ? 'badge-danger' : (r.deadPct >= 20 ? 'badge-warning' : 'badge-info')}">${r.deadPct.toFixed(1)}%</span></td>
                                        <td>${window.escapeHtml(fmtBytes(r.wastedBytes))}</td>
                                        <td class="text-muted">${window.escapeHtml(fmtWhen(r.last_autovacuum))}</td>
                                        <td>${window.escapeHtml(recommendation(r))}</td>
                                    </tr>
                                `).join('') || '<tr><td colspan="6" class="text-center text-muted">No candidates yet</td></tr>'}
                            </tbody>
                        </table>
                    </div>
                </div>
                <div class="table-card glass-panel" id="pgVacuumProgressCard" style="${hasVac ? '' : 'display:none;'}">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Autovacuum / Vacuum Progress (Live)</h3></div>
                    <div class="table-responsive" style="max-height:220px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead>
                                <tr><th>PID</th><th>DB/User</th><th>Relation</th><th>Phase</th><th>Scanned</th><th>Vacuumed</th><th>Dead tuples</th></tr>
                            </thead>
                            <tbody>
                                ${(hasVac ? vacProgress : []).map(v => `
                                    <tr>
                                        <td>${v.pid || '-'}</td>
                                        <td><strong>${window.escapeHtml(v.database_name || '-')}</strong><br/><span class="text-muted">${window.escapeHtml(v.user_name || '-')}</span></td>
                                        <td style="max-width:360px; overflow:hidden; text-overflow:ellipsis;" title="${window.escapeHtml(v.relation_name || '')}">
                                            ${window.escapeHtml(v.relation_name || '-')}
                                        </td>
                                        <td>${window.escapeHtml(v.phase || '-')}</td>
                                        <td>${Number(v.heap_blks_scanned || 0).toLocaleString()} / ${Number(v.heap_blks_total || 0).toLocaleString()} <span class="text-muted">(${Number(v.progress_pct || 0).toFixed(0)}%)</span></td>
                                        <td>${Number(v.heap_blks_vacuumed || 0).toLocaleString()}</td>
                                        <td>${Number(v.num_dead_tuples || 0).toLocaleString()} <span class="text-muted">/ ${Number(v.max_dead_tuples || 0).toLocaleString()}</span></td>
                                    </tr>
                                `).join('') || '<tr><td colspan="7" class="text-center text-muted">No vacuum progress currently running</td></tr>'}
                            </tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Table Metrics & Vacuum State</h3></div>
                    <div class="table-responsive" style="max-height:280px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead>
                                <tr><th>Table Name</th><th>Total Size</th><th>Dead Tuples</th><th>Bloat %</th><th>Scans (Seq vs Idx)</th><th>Last Vacuum</th><th>Last Analyze</th></tr>
                            </thead>
                            <tbody>
                                ${tables.length > 0 ? tables.map(table => `
                                    <tr>
                                        <td style="cursor:pointer;" title="Click to load dead tuple trend" onclick="window.pgLoadTableDeadTrend('${window.escapeHtml(table.schema).replace(/'/g,'&#39;')}','${window.escapeHtml(table.table).replace(/'/g,'&#39;')}')"><strong>${window.escapeHtml(table.schema)}.${window.escapeHtml(table.table)}</strong></td>
                                        <td>${window.escapeHtml(table.total_size)}</td>
                                        <td><span class="${table.dead_tuples > 1000 ? 'text-danger font-bold' : ''}">${table.dead_tuples.toLocaleString()}</span></td>
                                        <td><span class="badge ${table.bloat_pct > 20 ? 'badge-danger' : 'badge-success'}">${table.bloat_pct.toFixed(1)}%</span></td>
                                        <td>S: ${table.seq_scans} | I: ${table.idx_scans}</td>
                                        <td>${table.last_vacuum || 'Never'}</td>
                                        <td>${table.last_analyze || 'Never'}</td>
                                    </tr>
                                `).join('') : '<tr><td colspan="7" class="text-center text-muted">No table data available</td></tr>'}
                            </tbody>
                        </table>
                    </div>
                </div>
                
                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Index Inefficiency (Zero Scans / Dupes)</h3></div>
                    <div class="table-responsive" style="max-height:200px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead>
                                <tr><th>Index Name</th><th>Table</th><th>Size</th><th>Reason</th></tr>
                            </thead>
                            <tbody>
                                ${indexes.length > 0 ? indexes.map(index => `
                                    <tr>
                                        <td><strong>${window.escapeHtml(index.index_name)}</strong></td>
                                        <td>${window.escapeHtml(index.table_name)}</td>
                                        <td>${window.escapeHtml(index.size)}</td>
                                        <td><span class="${index.reason === 'Unused' ? 'text-danger' : 'text-warning'}">${window.escapeHtml(index.reason)}</span></td>
                                    </tr>
                                `).join('') : '<tr><td colspan="4" class="text-center text-muted">No unused indexes found</td></tr>'}
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    `;

    setTimeout(() => {
        window.currentCharts = window.currentCharts || {};
        
        const totalBloat = tables.length > 0 ? tables.reduce((sum, t) => sum + (t.bloat_pct || 0), 0) / tables.length : 0;
        const livePct = 100 - totalBloat;
        
        const bloatCtx = document.getElementById('pgBloatChart');
        if (bloatCtx) {
            window.currentCharts.pgBloat = new Chart(bloatCtx.getContext('2d'), {
                type: 'doughnut', data: {
                    labels: ['Bloat (Dead)', 'Live Data'], 
                    datasets: [{ 
                        data: [totalBloat, livePct], 
                        backgroundColor: [window.getCSSVar('--danger'), window.getCSSVar('--success')], 
                        borderWidth: 0
                    }]
                }, options: {responsive:true, maintainAspectRatio:false, cutout:'75%', plugins:{legend:{position:'bottom'}}}
            });
        }

        const vacCtx = document.getElementById('pgVacChart');
        if (vacCtx) {
            const labels = ccHistory?.labels || [];
            const autovac = ccHistory?.autovacuum_workers || [];
            window.currentCharts.pgVac = new Chart(vacCtx.getContext('2d'), {
                type: 'line',
                data: {
                    labels,
                    datasets: [
                        { label:'Autovacuum workers', data: autovac, borderColor: window.getCSSVar('--warning'), backgroundColor: 'rgba(245,158,11,0.15)', fill:true, tension:0.25, pointRadius:0 }
                    ]
                },
                options: { responsive:true, maintainAspectRatio:false, plugins:{legend:{position:'top'}}, scales:{ y:{ beginAtZero:true } } }
            });
        }
    }, 50);
}

window.pgLoadTableDeadTrend = async function(schema, table) {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: ''};
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/table-maintenance/history?instance=${encodeURIComponent(inst.name)}&schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}&limit=180`);
        if (!resp.ok) return;
        const ct = resp.headers.get('content-type') || '';
        if (!ct.includes('application/json')) return;
        const payload = await resp.json();
        const rows = payload.history || [];
        if (!rows.length) return;

        const rowEl = document.getElementById('pgDeadTrendRow');
        const labelEl = document.getElementById('pgDeadPctTrendLabel');
        if (rowEl) rowEl.style.display = 'grid';
        if (labelEl) labelEl.textContent = `${schema}.${table}`;

        const ctx = document.getElementById('pgDeadPctTrendChart');
        if (!ctx) return;

        const asc = rows.slice().reverse();
        const labels = asc.map(r => {
            const ts = r.capture_timestamp ? new Date(r.capture_timestamp) : null;
            return ts ? ts.toLocaleTimeString() : '';
        });
        const deadPct = asc.map(r => Number(r.dead_pct || 0));

        window.currentCharts = window.currentCharts || {};
        if (window.currentCharts.pgDeadPctTrend) {
            window.currentCharts.pgDeadPctTrend.destroy();
        }
        window.currentCharts.pgDeadPctTrend = new Chart(ctx.getContext('2d'), {
            type: 'line',
            data: { labels, datasets: [{ label: 'Dead %', data: deadPct, borderColor: window.getCSSVar('--danger'), backgroundColor: 'rgba(239,68,68,0.12)', fill:true, tension:0.25, pointRadius:0 }]},
            options: { responsive:true, maintainAspectRatio:false, scales:{ y:{ beginAtZero:true, max:100 } } }
        });
    } catch (e) {
        // non-fatal
    }
};
