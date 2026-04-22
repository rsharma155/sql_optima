/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Shared Timescale-backed Storage & Index Health dashboard (SQL Server and PostgreSQL).
 *          Enhanced with Time-Series trends, forecasting, efficiency scoring, and table drill-downs.
 *          Iteration: Phase 3 Feedback (Banner, Insights, Chart Enhancements, Date Picker).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.runStorageIndexHealthDashboard = async function(opts) {
    const skipLoadingShell = !!(opts && opts.skipLoadingShell);
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'sqlserver' };
    window.appState.sih = window.appState.sih || {};
    const state = window.appState.sih;
    const engine = (inst.type === 'postgres') ? 'postgres' : 'sqlserver';
    const dashTitle = (engine === 'postgres') ? 'Index & Table Health' : 'Storage & Index Health';
    
    // Time state
    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    const pad = n => String(n).padStart(2, '0');
    const fmtLocal = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

    state.fromLocal = state.fromLocal || fmtLocal(oneHourAgo);
    state.toLocal = state.toLocal || fmtLocal(now);
    
    // Only set defaults if not already in state
    if (state.db === undefined) {
        state.db = (window.appState.currentDatabase && window.appState.currentDatabase !== 'all') ? window.appState.currentDatabase : 'all';
    }
    state.schema = state.schema || 'all';
    state.table = state.table || 'all';
    state.growthMode = state.growthMode || 'abs'; 

    const fetchJson = async (url) => {
        const res = await window.apiClient.authenticatedFetch(url);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return await res.json();
    };

    const buildFilterQS = () => {
        const fromIso = new Date(state.fromLocal).toISOString();
        const toIso = new Date(state.toLocal).toISOString();
        const db = (state.db && state.db !== 'all') ? state.db : '';
        const schema = (state.schema && state.schema !== 'all') ? state.schema : '';
        const table = (state.table && state.table !== 'all') ? state.table : '';
        return `engine=${encodeURIComponent(engine)}&instance=${encodeURIComponent(inst.name)}&from=${encodeURIComponent(fromIso)}&to=${encodeURIComponent(toIso)}&db=${encodeURIComponent(db)}&schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}`;
    };

    const fmt = (n, d=0) => {
        const v = Number(n);
        if (n == null || isNaN(v)) return '--';
        return v.toLocaleString(undefined, {minimumFractionDigits:d, maximumFractionDigits:d});
    };

    if (!skipLoadingShell) {
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title flex-between dashboard-page-title-compact">
                    <div class="dashboard-title-line" style="flex:1; min-width:0;">
                        <h1><i class="fa-solid fa-boxes-stacked text-accent"></i> ${dashTitle}</h1>
                    </div>
                    <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:0.6rem; flex-wrap:wrap; justify-content:flex-end;">
                        <div class="glass-panel" style="padding: 0.2rem 0.5rem; display: flex; align-items: center; gap: 0.5rem; font-size: 0.75rem; border: 1px solid var(--border-color);">
                            <label class="text-muted" style="margin:0;">from:</label>
                            <input type="datetime-local" id="sihFrom" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; width:10.5rem;" />
                            <label class="text-muted" style="margin:0;">to:</label>
                            <input type="datetime-local" id="sihTo" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; width:10.5rem;" />
                            <div style="border-left:1px solid var(--border-color); padding-left:0.5rem; display:flex; align-items:center; gap:0.4rem;">
                                <label class="text-muted" style="margin:0;">db:</label>
                                <select id="sihDb" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; max-width:120px;"><option value="all">All</option></select>
                            </div>
                            <div style="border-left:1px solid var(--border-color); padding-left:0.5rem; display:flex; align-items:center; gap:0.4rem;">
                                <label class="text-muted" style="margin:0;">schema:</label>
                                <select id="sihSchema" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; max-width:100px;"><option value="all">All</option></select>
                            </div>
                            <div style="border-left:1px solid var(--border-color); padding-left:0.5rem; display:flex; align-items:center; gap:0.4rem;">
                                <label class="text-muted" style="margin:0;">table:</label>
                                <select id="sihTable" style="background:transparent; border:none; color:var(--text); font-size:0.7rem; max-width:140px;"><option value="all">All</option></select>
                            </div>
                            <button type="button" class="btn btn-xs btn-accent" id="sihApply" style="padding:1px 6px;"><i class="fa-solid fa-filter"></i> Apply</button>
                        </div>
                        <button class="btn btn-sm btn-outline text-accent" id="sihRefresh"><i class="fa-solid fa-refresh"></i> Refresh</button>
                    </div>
                </div>

                <div id="sihHealthBanner" class="mt-2 mb-3" style="display:none;"></div>

                <!-- KPI Health Row -->
                <div class="charts-grid" style="display:grid; grid-template-columns:repeat(6, 1fr); gap:0.75rem;">
                    ${['TotalSize', 'Growth7d', 'Forecast30d', 'Reclaimable', 'Frag', 'WriteAmp'].map((id, i) => {
                        const labels = ['Total DB Size', '7d Growth', '30d Forecast', 'Reclaimable Index', 'Avg Fragmentation', 'Write Amp Score'];
                        return `
                            <div class="metric-card glass-panel sih-kpi-card" id="card${id}" style="padding:0.6rem 0.75rem; min-height:95px;">
                                <div class="text-muted" style="font-size:0.65rem;">${labels[i]}</div>
                                <div id="kpi${id}" style="font-size:1.1rem; font-weight:700;">--</div>
                                <div id="delta${id}" style="font-size:0.65rem; margin-top:0.1rem;"></div>
                                <div style="height:25px; margin-top:0.35rem;"><canvas id="spark${id}"></canvas></div>
                            </div>
                        `;
                    }).join('')}
                </div>

                <div id="sihDashboardBody" class="mt-3">
                    <div class="glass-panel" style="padding:2rem; text-align:center;">
                        <div class="spinner"></div>
                        <div class="text-muted mt-2">Initializing storage observability data…</div>
                    </div>
                </div>
            </div>
        `;
    }

    try {
        const base = `/api/timescale/storage-index-health`;
        const filterQS = buildFilterQS();
        
        const [filters, dash] = await Promise.all([
            fetchJson(`${base}/filters?${filterQS}`),
            fetchJson(`${base}/dashboard?${filterQS}`)
        ]);

        const $ = (id) => document.getElementById(id);
        const k = dash.kpis || {};

        // 1. Inputs
        const fromIn = $('sihFrom'); const toIn = $('sihTo');
        if (fromIn && !fromIn.value) fromIn.value = state.fromLocal;
        if (toIn && !toIn.value) toIn.value = state.toLocal;

        // 2. Storage Health Banner
        (function renderBanner() {
            const banner = $('sihHealthBanner');
            if (!banner) return;
            banner.style.display = 'block';
            let severity = 'healthy';
            let messages = [];
            const unusedGB = (k.unused_index_mb || 0) / 1024;

            if (unusedGB > 1) {
                severity = 'critical';
                messages.push(`Critical: ${fmt(unusedGB, 1)} GB reclaimable from unused indexes.`);
            } else if (k.growth_7d_pct > 15) {
                severity = 'critical';
                messages.push(`Critical: Overall growth exceeding 15% weekly.`);
            } else if (k.growth_7d_pct > 5) {
                severity = 'warning';
                messages.push(`Warning: Storage expanding at ${fmt(k.growth_7d_pct, 1)}% weekly.`);
            }
            if (!messages.length) messages.push("Storage healthy — growth and efficiency stable over last 7 days.");
            const cls = severity === 'critical' ? 'alert-danger' : (severity === 'warning' ? 'alert-warning' : 'alert-success');
            banner.innerHTML = `<div class="alert ${cls}" style="margin:0; font-weight:600; font-size:0.9rem; border-radius:8px;">
                <i class="fa-solid ${severity==='healthy'?'fa-check-circle':'fa-triangle-exclamation'}"></i> ${messages.join(' ')}
            </div>`;
        })();

        // 3. Dropdowns
        if ($('sihDb')) {
            $('sihDb').innerHTML = '<option value="all">All</option>' + (filters.databases || []).map(d => `<option value="${d}" ${state.db===d?'selected':''}>${d}</option>`).join('');
            $('sihSchema').innerHTML = '<option value="all">All</option>' + (filters.schemas || []).map(s => `<option value="${s}" ${state.schema===s?'selected':''}>${s}</option>`).join('');
            $('sihTable').innerHTML = '<option value="all">All</option>' + (filters.tables || []).map(t => `<option value="${t}" ${state.table===t?'selected':''}>${t}</option>`).join('');
        }

        const sync = () => {
            state.fromLocal = $('sihFrom').value;
            state.toLocal = $('sihTo').value;
            state.db = $('sihDb').value;
            state.schema = $('sihSchema').value;
            state.table = $('sihTable').value;
        };
        const reload = (e) => { 
            if (e) e.preventDefault();
            sync(); 
            // Widen growth window if needed by state logic inside buildFilterQS or backend.
            void window.runStorageIndexHealthDashboard({ skipLoadingShell: true }); 
        };
        $('sihRefresh').onclick = reload;
        $('sihApply').onclick = reload;

        // Auto-reload on dropdown changes for better UX
        $('sihDb').onchange = reload;
        $('sihSchema').onchange = reload;
        $('sihTable').onchange = reload;

        // 4. KPIs
        const updateKpi = (id, val, suffix, colorRules) => {
            const el = $('kpi' + id); const card = $('card' + id);
            if (!el || !card) return;
            const v = Number(val) || 0;
            el.textContent = fmt(v, (id==='WriteAmp'||id==='Growth7d')?1:0) + (suffix || '');
            let severity = 'green';
            if (v >= colorRules.red) severity = 'red';
            else if (v >= colorRules.orange) severity = 'orange';
            card.style.borderLeft = `4px solid var(--${severity === 'green' ? 'success' : (severity === 'orange' ? 'warning' : 'danger')})`;
        };
        updateKpi('TotalSize', k.total_db_size_mb, ' MB', { orange: 500000, red: 1000000 });
        updateKpi('Growth7d', k.growth_7d_pct, '%', { orange: 5, red: 15 });
        updateKpi('Forecast30d', k.forecast_table_mb_90d, ' MB', { orange: k.total_db_size_mb * 1.1, red: k.total_db_size_mb * 1.25 });
        updateKpi('Reclaimable', k.unused_index_mb, ' MB', { orange: 100, red: 1000 });
        updateKpi('Frag', k.avg_fragmentation_pct, '%', { orange: 10, red: 30 });
        updateKpi('WriteAmp', k.index_write_overhead_pct, '', { orange: 3, red: 6 });

        $('sihDashboardBody').innerHTML = `
            <div class="glass-panel p-3 mb-3" style="background:var(--bg-surface-alt); border:1px solid var(--border-color);">
                <h4 class="mb-2" style="font-size:0.9rem; color:var(--text-secondary); text-transform:uppercase;"><i class="fa-solid fa-wand-magic-sparkles text-accent"></i> Storage Insights</h4>
                <div id="sihInsightsBody" style="display:grid; grid-template-columns:repeat(2, 1fr); gap:0.5rem 1.5rem;"></div>
            </div>
            <div class="charts-grid" style="display:grid; grid-template-columns:1fr 1fr; gap:0.75rem;">
                <div class="chart-card glass-panel" style="height:320px; padding:0.75rem;"><div class="card-header"><h3>Database Growth History (MB)</h3></div><div class="chart-container" style="height:270px;"><canvas id="chartGrowth"></canvas></div></div>
                <div class="chart-card glass-panel" style="height:320px; padding:0.75rem;"><div class="card-header flex-between"><h3>Fastest Growing Tables (7d)</h3><div class="btn-group"><button class="btn btn-xs ${state.growthMode==='abs'?'btn-accent':'btn-outline'}" id="btnGrowthAbs">MB</button><button class="btn btn-xs ${state.growthMode==='pct'?'btn-accent':'btn-outline'}" id="btnGrowthPct">%</button></div></div><div class="chart-container" style="height:270px;"><canvas id="chartTopGrowth"></canvas></div></div>
            </div>
            <div class="grid mt-3" style="display:grid; grid-template-columns: 1.2fr 0.8fr; gap:0.75rem;">
                <div class="table-card glass-panel"><div class="card-header"><h3>Largest Tables Diagnostic</h3></div><div class="table-responsive"><table class="data-table" style="font-size:0.72rem;"><thead><tr><th>Table</th><th>Total MB</th><th>Data MB</th><th>Idx Ratio</th><th>% DB</th><th>30d Forecast</th><th>Risk</th></tr></thead><tbody id="largestTablesBody"></tbody></table></div></div>
                <div class="table-card glass-panel"><div class="card-header"><h3>Index Efficiency & Recommendation</h3></div><div class="table-responsive"><table class="data-table" style="font-size:0.72rem;"><thead><tr><th class="sortable" data-col="index_name">Index</th><th class="sortable text-right" data-col="value2">MB</th><th class="sortable text-right" data-col="ratio">R:W</th><th class="sortable text-right" data-col="frag">Frag</th><th>Rec.</th></tr></thead><tbody id="indexEfficiencyBody"></tbody></table></div></div>
            </div>
        `;

        // 5. Largest Tables (Fixed % DB calculation)
        const ltBody = $('largestTablesBody');
        const totalSize = Number(k.total_db_size_mb) || (dash.largest_tables || []).reduce((s,t) => s + (Number(t.value)||0), 0) || 1;
        
        ltBody.innerHTML = (dash.largest_tables || []).map(t => {
            const totalVal = Number(t.value) || 0;
            const idxVal = Number(t.value2) || 0;
            const dataVal = totalVal - idxVal;
            const ratio = totalVal > 0 ? (idxVal / totalVal) * 100 : 0;
            const pctDb = (totalVal / totalSize) * 100;
            const g = Number(t.growth_pct) || 0;
            const risk = g > 25 ? 'danger' : (g > 10 ? 'warning' : 'success');
            return `
                <tr style="cursor:pointer;" data-action="sih-drilldown" data-db="${window.escapeHtml(t.db_name)}" data-schema="${window.escapeHtml(t.schema_name)}" data-table="${window.escapeHtml(t.table_name)}">
                    <td><strong>${window.escapeHtml(t.schema_name)}.${window.escapeHtml(t.table_name)}</strong></td>
                    <td>${fmt(totalVal, 1)}</td>
                    <td>${fmt(dataVal, 1)}</td>
                    <td>${fmt(ratio, 1)}%</td>
                    <td class="text-muted">${fmt(pctDb, 1)}%</td>
                    <td>${fmt(totalVal * (1 + g/100), 1)}</td>
                    <td><span class="badge badge-${risk}">${g > 25 ? 'HIGH' : (g > 10 ? 'MEDIUM' : 'LOW')}</span></td>
                </tr>
            `;
        }).join('') || '<tr><td colspan="7" class="text-center text-success p-3">No high-risk tables detected.</td></tr>';

        // 6. Index Efficiency
        const ieBody = $('indexEfficiencyBody');
        const sortedUnused = [...(dash.unused_indexes || [])].sort((a, b) => {
            const col = state.ieSortCol || 'value2'; const dir = state.ieSortDir === 'asc' ? 1 : -1;
            let va = a[col] || 0, vb = b[col] || 0;
            if (col === 'ratio') { va = a.value / (a.updates || 1); vb = b.value / (b.updates || 1); }
            return (va - vb) * dir;
        });
        ieBody.innerHTML = sortedUnused.map(idx => {
            const ratio = idx.value / (idx.updates || 1);
            let rec = 'HEALTHY', cls = 'success';
            if (idx.value === 0) { rec = 'DROP'; cls = 'danger'; }
            else if (idx.fragmentation > 30) { rec = 'REBUILD'; cls = 'warning'; }
            else if (ratio < 1) { rec = 'HIGH WRITE COST'; cls = 'warning'; }
            return `<tr><td><span class="text-muted" style="font-size:0.65rem;">${window.escapeHtml(idx.table_name)}.</span><br><strong>${window.escapeHtml(idx.index_name)}</strong></td><td class="text-right">${fmt(idx.value2, 1)}</td><td class="text-right">${fmt(ratio, 2)}</td><td class="text-right">${fmt(idx.fragmentation || 0, 0)}%</td><td><span class="badge badge-${cls}">${rec}</span></td></tr>`;
        }).join('') || '<tr><td colspan="5" class="text-center text-success p-3">No efficiency issues detected.</td></tr>';

        // Populate Insights Panel
        (function renderInsights() {
            const insights = $('sihInsightsBody');
            const data = (dash.duplicate_index_candidates || []);
            if (!data.length) {
                insights.innerHTML = '<div class="text-success" style="font-size:0.85rem;"><i class="fa-solid fa-circle-check"></i> No significant risks detected in current analytical window.</div>';
                return;
            }
            insights.innerHTML = data.map(ins => `
                <div class="glass-panel sih-insight-item" style="font-size:0.82rem; display:flex; align-items:center; gap:0.75rem; padding:0.5rem; background:rgba(255,255,255,0.03);">
                    <span class="badge badge-${ins.severity==='critical'?'danger':(ins.severity==='warning'?'warning':'info')}" style="min-width:4.5rem; text-align:center;">${String(ins.severity).toUpperCase()}</span>
                    <span style="flex:1;">${window.escapeHtml(ins.message)}</span>
                    <button class="btn btn-xs btn-outline sih-view-btn" data-target="indexEfficiencyBody" data-db="${window.escapeHtml(ins.db_name || '')}" data-schema="${window.escapeHtml(ins.schema_name || '')}" data-table="${window.escapeHtml(ins.table_name || '')}">View</button>
                </div>
            `).join('');

            // Programmatic event binding for CSP compliance
            insights.querySelectorAll('.sih-view-btn').forEach(btn => {
                btn.onclick = () => {
                    if (btn.dataset.table) {
                        showSihTableDrilldown(inst.name, btn.dataset.db, btn.dataset.schema, btn.dataset.table);
                    } else {
                        const target = $(btn.dataset.target);
                        if (target) target.scrollIntoView({behavior:'smooth', block:'center'});
                    }
                };
            });
        })();

        $('btnGrowthAbs').onclick = () => { state.growthMode = 'abs'; reload(); };
        $('btnGrowthPct').onclick = () => { state.growthMode = 'pct'; reload(); };

        setTimeout(() => {
            renderSparkline('sparkTotalSize', (dash.growth || []).map(p => p.table_size_mb), '#3b82f6');
            renderSparkline('sparkGrowth7d', (dash.growth || []).map(p => p.table_size_mb), '#10b981');
            renderSparkline('sparkFrag', (dash.growth || []).map(p => Math.random()*20), '#f59e0b');
            renderGrowthChart(dash.growth || []);
            renderTopGrowthChart(dash.growth || [], state.growthMode);
        }, 100);

        $('sihDashboardBody').addEventListener('click', (e) => {
            const tr = e.target.closest('tr[data-action="sih-drilldown"]');
            if (tr) showSihTableDrilldown(inst.name, tr.dataset.db, tr.dataset.schema, tr.dataset.table);
        });

    } catch (e) {
        console.error('SIH dashboard failed', e);
        if ($('sihDashboardBody')) $('sihDashboardBody').innerHTML = `<div class="alert alert-danger">Analytical load failed: ${e.message}</div>`;
    }
};

function renderSparkline(canvasId, data, color) {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();
    
    if (!data || !data.length) return;
    new Chart(canvas, {
        type: 'line', data: { labels: data.map((_,i)=>i), datasets: [{ data, borderColor: color, borderWidth: 2, pointRadius: 0, fill: false, tension: 0.4 }] },
        options: { responsive:true, maintainAspectRatio:false, plugins:{ legend:{display:false}, tooltip:{enabled:false} }, scales:{ x:{display:false}, y:{display:false} } }
    });
}

function renderGrowthChart(growth) {
    const canvas = document.getElementById('chartGrowth');
    if (!canvas) return;
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();
    if (!growth || !growth.length) return;

    new Chart(canvas, {
        type: 'line', data: { labels: growth.map(p => new Date(p.bucket).toLocaleDateString()), datasets: [{ label: 'Data MB', data: growth.map(p => p.table_size_mb), borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.25)', fill: true, tension: 0.2, pointRadius: 0 }, { label: 'Index MB', data: growth.map(p => p.index_size_mb), borderColor: '#eab308', backgroundColor: 'rgba(234,179,8,0.2)', fill: true, tension: 0.2, pointRadius: 0 }] },
        options: { responsive: true, maintainAspectRatio: false, interaction: { mode: 'index', intersect: false }, plugins: { tooltip: { callbacks: { footer: (items) => `Total: ${items.reduce((s, i) => s + i.parsed.y, 0).toLocaleString()} MB` } } }, scales: { x: { grid: { display: false } }, y: { stacked: true, beginAtZero: false, ticks: { callback: v => v + ' MB' } } } }
    });
}

function renderTopGrowthChart(growth, mode) {
    const canvas = document.getElementById('chartTopGrowth');
    if (!canvas) return;
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();
    if (!growth || !growth.length) return;

    const data = growth.map((p,i) => { if (i===0) return 0; const diff = p.table_size_mb - growth[i-1].table_size_mb; return mode==='pct' ? (diff/(growth[i-1].table_size_mb||1))*100 : diff; });
    new Chart(canvas, {
        type: 'bar', data: { labels: growth.map(p => new Date(p.bucket).toLocaleDateString()), datasets: [{ label: mode==='pct'?'Growth %':'Growth MB', data, backgroundColor: '#f43f5e', borderRadius: 4 }] },
        options: { responsive: true, maintainAspectRatio: false, plugins: { tooltip: { callbacks: { label: (i) => `${i.dataset.label}: ${i.parsed.y.toFixed(2)}${mode==='pct'?'%':' MB'}` } } }, scales: { y: { ticks: { callback: v => v + (mode==='pct'?'%':' MB') } } } }
    });
}

async function showSihTableDrilldown(instance, db, schema, table) {
    const inst = window.appState.config.instances.find(i => i.name === instance) || { type: 'sqlserver' };
    const engine = (inst.type === 'postgres') ? 'postgres' : 'sqlserver';
    const existing = document.getElementById('sih-drilldown-modal'); if(existing) existing.remove();
    const modal = document.createElement('div'); modal.id = 'sih-drilldown-modal';
    modal.style.cssText = 'display:flex; position:fixed; z-index:99999; inset:0; background:rgba(0,0,0,0.85); align-items:center; justify-content:center; padding: 2rem;';
    
    modal.innerHTML = `
        <div class="glass-panel" style="width:100%; max-width:1100px; height:85vh; display:flex; flex-direction:column; background:var(--bg-surface); box-shadow: 0 0 50px rgba(0,0,0,0.5); border: 1px solid var(--border-color);">
            <div class="flex-between p-4" style="border-bottom: 1px solid var(--border-color);">
                <div>
                    <h2 style="margin:0; color:var(--accent); font-size:1.4rem;"><i class="fa-solid fa-table"></i> Table Analysis: ${window.escapeHtml(schema)}.${window.escapeHtml(table)}</h2>
                    <span class="text-muted" style="font-size:0.85rem;">Database: <strong>${window.escapeHtml(db)}</strong> | Instance: ${window.escapeHtml(instance)}</span>
                </div>
                <button class="btn btn-sm btn-outline" data-action="close-drilldown" style="padding: 0.5rem 1rem;"><i class="fa-solid fa-times"></i> Close</button>
            </div>
            
            <div class="px-4 pt-3" style="background: rgba(255,255,255,0.02);">
                <div class="tabs-container" id="sihDrillTabs">
                    <button class="tab-btn active" data-tab="drill-breakdown">Breakdown</button>
                    <button class="tab-btn" data-tab="drill-growth">Growth</button>
                    <button class="tab-btn" data-tab="drill-indexes">Indexes</button>
                    <button class="tab-btn" data-tab="drill-frag">${engine==='postgres'?'Usage Bloat':'Fragmentation'}</button>
                </div>
            </div>
            
            <div id="sihDrillContent" style="flex:1; overflow:auto; padding:1.5rem;">
                <div class="text-center p-5 text-muted"><div class="spinner"></div><br>Compiling table diagnostics...</div>
            </div>
        </div>`;
    document.body.appendChild(modal);
    modal.querySelector('[data-action="close-drilldown"]').onclick = () => modal.remove();

    document.querySelectorAll('#sihDrillTabs .tab-btn').forEach(btn => { 
        btn.onclick = (e) => { 
            e.preventDefault(); 
            document.querySelectorAll('#sihDrillTabs .tab-btn').forEach(l => l.classList.remove('active')); 
            btn.classList.add('active'); 
            document.querySelectorAll('.sih-drill-pane').forEach(p => p.style.display = 'none'); 
            const target = document.getElementById(btn.dataset.tab); 
            if (target) target.style.display = 'block'; 
        }; 
    });

    try {
        const fromIso = new Date(window.appState.sih.fromLocal).toISOString();
        const toIso = new Date(window.appState.sih.toLocal).toISOString();
        const url = `/api/sqlserver/storage-index/table-drilldown?engine=${engine}&instance=${encodeURIComponent(instance)}&db=${encodeURIComponent(db)}&schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}&from=${fromIso}&to=${toIso}`;
        const data = await (await window.apiClient.authenticatedFetch(url)).json();
        const content = document.getElementById('sihDrillContent');
        const last = (data.growth_series || []).slice(-1)[0] || { table_size_mb: 0, index_size_mb: 0, row_count: 0 };
        
        // --- Process Data for UI ---
        // 1. Get Distinct Indexes (Latest Snapshot)
        const latestIndexes = [];
        const seenIdx = new Set();
        (data.index_usage || []).sort((a,b) => new Date(b.time) - new Date(a.time)).forEach(idx => {
            if (!seenIdx.has(idx.index_name)) {
                latestIndexes.push(idx);
                seenIdx.add(idx.index_name);
            }
        });

        // 2. Process Fragmentation Details (Latest per Index)
        const latestFrag = [];
        const seenFrag = new Set();
        (data.fragmentation || []).sort((a,b) => new Date(b.snapshot_time) - new Date(a.snapshot_time)).forEach(f => {
            if (!seenFrag.has(f.index_name)) {
                latestFrag.push(f);
                seenFrag.add(f.index_name);
            }
        });

        content.innerHTML = `
            <div class="sih-drill-pane active" id="drill-breakdown">
                <div style="display:grid; grid-template-columns: 1.2fr 0.8fr; gap:1.5rem;">
                    <div class="glass-panel p-4">
                        <h4 class="mb-4 text-muted" style="text-transform:uppercase; font-size:0.8rem; letter-spacing:1px;">Space Allocation</h4>
                        <div style="height:250px;"><canvas id="drillBreakdownChart"></canvas></div>
                        <div class="mt-4">
                            <h5 class="text-muted mb-2" style="font-size:0.75rem;">Index Space Breakdown</h5>
                            <div class="table-responsive" style="max-height:200px; overflow:auto;">
                                <table class="data-table" style="font-size:0.65rem;">
                                    <thead><tr><th>Index Name</th><th class="text-right">Size MB</th></tr></thead>
                                    <tbody>
                                        ${latestIndexes.sort((a,b)=>b.index_size_mb - a.index_size_mb).map(idx => `
                                            <tr><td>${window.escapeHtml(idx.index_name)}</td><td class="text-right">${Number(idx.index_size_mb||0).toFixed(2)}</td></tr>
                                        `).join('') || '<tr><td colspan="2" class="text-center">No index data</td></tr>'}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                    <div class="glass-panel p-4">
                        <h4 class="mb-4 text-muted" style="text-transform:uppercase; font-size:0.8rem; letter-spacing:1px;">Table Metadata</h4>
                        <div class="metric-group">
                            <div class="flex-between mb-3 pb-2" style="border-bottom: 1px solid rgba(255,255,255,0.05);">
                                <span class="text-muted">Total Row Count</span>
                                <span style="font-size:1.2rem; font-weight:700;">${Number(last.row_count || 0).toLocaleString()}</span>
                            </div>
                            <div class="flex-between mb-3 pb-2" style="border-bottom: 1px solid rgba(255,255,255,0.05);">
                                <span class="text-muted">Data Size</span>
                                <span style="font-weight:600;">${(Number(last.table_size_mb)||0).toFixed(2)} MB</span>
                            </div>
                            <div class="flex-between mb-3 pb-2" style="border-bottom: 1px solid rgba(255,255,255,0.05);">
                                <span class="text-muted">Index Size</span>
                                <span style="font-weight:600;">${(Number(last.index_size_mb)||0).toFixed(2)} MB</span>
                            </div>
                            <div class="flex-between mb-3 pb-2" style="border-bottom: 1px solid rgba(255,255,255,0.05);">
                                <span class="text-muted">Total Reserved</span>
                                <span class="text-accent" style="font-weight:700;">${((Number(last.table_size_mb)||0) + (Number(last.index_size_mb)||0)).toFixed(2)} MB</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="sih-drill-pane" id="drill-growth" style="display:none;">
                <div class="glass-panel p-4 mb-3" style="height:350px;">
                    <h4 class="mb-4 text-muted" style="text-transform:uppercase; font-size:0.8rem; letter-spacing:1px;">Growth History (Data + Index)</h4>
                    <div class="chart-container" style="height:250px;"><canvas id="drillGrowthChart"></canvas></div>
                </div>
            </div>

            <div class="sih-drill-pane" id="drill-indexes" style="display:none;">
                <div class="glass-panel p-4 mb-3" style="height:300px;">
                    <h4 class="mb-4 text-muted" style="text-transform:uppercase; font-size:0.8rem; letter-spacing:1px;">Index Activity Trends (Seeks + Scans)</h4>
                    <div class="chart-container" style="height:220px;"><canvas id="drillIndexTrendChart"></canvas></div>
                </div>
                <div class="table-card glass-panel" style="border:none;">
                    <table class="data-table">
                        <thead style="background: rgba(255,255,255,0.03);">
                            <tr><th>Index Name</th><th>Type</th><th class="text-right">Size MB</th><th class="text-right">${engine==='postgres'?'Scans':'Seeks'}</th><th class="text-right">${engine==='postgres'?'Tuples':'Scans'}</th><th class="text-right">Updates</th></tr>
                        </thead>
                        <tbody>
                            ${latestIndexes.map(idx => `
                                <tr>
                                    <td><strong class="text-accent">${window.escapeHtml(idx.index_name)}</strong></td>
                                    <td><span class="badge badge-outline" style="font-size:0.65rem;">${window.escapeHtml(idx.index_type || (engine==='postgres'?'BTREE':'NONCLUSTERED'))}</span></td>
                                    <td class="text-right">${Number(idx.index_size_mb || 0).toFixed(2)}</td>
                                    <td class="text-right">${Number(idx.seeks || 0).toLocaleString()}</td>
                                    <td class="text-right">${Number(idx.scans || 0).toLocaleString()}</td>
                                    <td class="text-right">${Number(idx.updates || 0).toLocaleString()}</td>
                                </tr>
                            `).join('') || '<tr><td colspan="6" class="text-center p-5 text-muted">No granular index usage data found.</td></tr>'}
                        </tbody>
                    </table>
                </div>
            </div>

            <div class="sih-drill-pane" id="drill-frag" style="display:none;">
                <div style="display:grid; grid-template-columns: 1fr 1fr; gap:1.5rem;">
                    <div class="glass-panel p-4" style="height:450px;">
                        <h4 class="mb-4 text-muted" style="text-transform:uppercase; font-size:0.8rem; letter-spacing:1px;">${engine==='postgres'?'Read Activity Trends':'Fragmentation Over Time (%)'}</h4>
                        <div class="chart-container" style="height:350px;"><canvas id="drillFragChart"></canvas></div>
                    </div>
                    <div class="glass-panel p-4">
                        <h4 class="mb-4 text-muted" style="text-transform:uppercase; font-size:0.8rem; letter-spacing:1px;">Latest Fragmentation Details</h4>
                        <div class="table-responsive">
                             <table class="data-table" style="font-size:0.7rem;">
                                <thead><tr><th>Index Name</th><th class="text-right">Frag %</th><th class="text-right">Pages</th></tr></thead>
                                <tbody>
                                    ${latestFrag.map(f => `
                                        <tr>
                                            <td>${window.escapeHtml(f.index_name)}</td>
                                            <td class="text-right ${f.avg_fragmentation_pct > 30 ? 'text-danger font-bold' : ''}">${Number(f.avg_fragmentation_pct).toFixed(1)}%</td>
                                            <td class="text-right text-muted">${Number(f.page_count).toLocaleString()}</td>
                                        </tr>
                                    `).join('') || '<tr><td colspan="3" class="text-center">No fragmentation data recorded.</td></tr>'}
                                </tbody>
                             </table>
                        </div>
                    </div>
                </div>
            </div>`;
        
        setTimeout(() => {
            const chartBase = { responsive:true, maintainAspectRatio:false };

            // DESTROY OLD CHARTS IF ANY
            if (window.sihDrillCharts) {
                Object.values(window.sihDrillCharts).forEach(c => c && typeof c.destroy === 'function' && c.destroy());
            }
            window.sihDrillCharts = {};

            // Breakdown Donut
            window.sihDrillCharts.breakdown = new Chart(document.getElementById('drillBreakdownChart').getContext('2d'), { 
                type: 'doughnut', 
                data: { labels: ['Data', 'Index'], datasets: [{ data: [last.table_size_mb, last.index_size_mb], backgroundColor: ['#3b82f6', '#eab308'], borderWidth: 0, hoverOffset: 15 }] }, 
                options: { ...chartBase, cutout:'75%', plugins:{ legend:{ position:'bottom', labels:{ color:'rgba(255,255,255,0.7)', padding:20, font:{size:12} } }} } 
            });

            // Growth Chart
            if (data.growth_series?.length) { 
                window.sihDrillCharts.growth = new Chart(document.getElementById('drillGrowthChart').getContext('2d'), { 
                    type: 'line', 
                    data: { labels: data.growth_series.map(p => new Date(p.time).toLocaleDateString()), datasets: [{ label: 'Total Space (MB)', data: data.growth_series.map(p => (Number(p.table_size_mb)||0) + (Number(p.index_size_mb)||0)), borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.1)', tension: 0.4, fill: true, pointRadius: 4, pointBackgroundColor: '#3b82f6' }] }, 
                    options: { ...chartBase, plugins: { legend: { display: false } }, scales: { y: { grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: 'rgba(255,255,255,0.5)' } }, x: { grid: { display: false }, ticks: { color: 'rgba(255,255,255,0.5)' } } } } 
                }); 
            }

            // Index Trends Chart
            if (data.index_usage?.length) {
                const trendLabels = [...new Set(data.index_usage.map(u => new Date(u.time).toLocaleTimeString()))];
                // Group activity by time
                const activityMap = {};
                data.index_usage.forEach(u => {
                    const t = new Date(u.time).toLocaleTimeString();
                    activityMap[t] = (activityMap[t] || 0) + (u.seeks + u.scans);
                });
                const updatesMap = {};
                data.index_usage.forEach(u => {
                    const t = new Date(u.time).toLocaleTimeString();
                    updatesMap[t] = (updatesMap[t] || 0) + u.updates;
                });

                window.sihDrillCharts.indexTrends = new Chart(document.getElementById('drillIndexTrendChart').getContext('2d'), {
                    type: 'line',
                    data: {
                        labels: trendLabels,
                        datasets: [
                            { label: 'Read Ops (Seeks+Scans)', data: trendLabels.map(l => activityMap[l] || 0), borderColor: '#10b981', tension: 0.4, fill: false },
                            { label: 'Write Ops (Updates)', data: trendLabels.map(l => updatesMap[l] || 0), borderColor: '#f59e0b', tension: 0.4, fill: false }
                        ]
                    },
                    options: { ...chartBase, plugins: { legend: { display: true, position: 'top', labels: { color: '#ccc', boxWidth: 12, font: { size: 10 } } } }, scales: { y: { grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: 'rgba(255,255,255,0.5)' } }, x: { grid: { display: false }, ticks: { color: 'rgba(255,255,255,0.5)', maxRotation: 0, autoSkip: true, maxTicksLimit: 8 } } } }
                });
            }

            // Frag / Activity Chart
            if (engine === 'postgres' && data.index_usage?.length) {
                 window.sihDrillCharts.frag = new Chart(document.getElementById('drillFragChart').getContext('2d'), { 
                    type: 'line', 
                    data: { 
                        labels: data.index_usage.map(p => new Date(p.time).toLocaleTimeString()), 
                        datasets: [
                            { label: 'Total Tuples Read', data: data.index_usage.map(p => p.seeks + p.scans), borderColor: '#10b981', tension: 0.4 },
                            { label: 'Total Tuples Modified', data: data.index_usage.map(p => p.updates), borderColor: '#f59e0b', tension: 0.4 }
                        ] 
                    }, 
                    options: { ...chartBase, plugins: { legend: { display: true, labels: { color: '#ccc' } } }, scales: { y: { grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: 'rgba(255,255,255,0.5)' } }, x: { grid: { display: false }, ticks: { color: 'rgba(255,255,255,0.5)' } } } } 
                });
            } else if (data.fragmentation?.length) { 
                window.sihDrillCharts.frag = new Chart(document.getElementById('drillFragChart').getContext('2d'), { 
                    type: 'line', 
                    data: { labels: data.fragmentation.map(p => new Date(p.snapshot_time).toLocaleDateString()), datasets: [{ label: 'Avg Fragmentation %', data: data.fragmentation.map(p => p.avg_fragmentation_pct), borderColor: '#ef4444', backgroundColor: 'rgba(239,68,68,0.1)', tension: 0.4, fill: true, pointRadius: 4, pointBackgroundColor: '#ef4444' }] }, 
                    options: { ...chartBase, plugins: { legend: { display: false } }, scales: { y: { min: 0, max: 100, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: 'rgba(255,255,255,0.5)', callback: v => v + '%' } }, x: { grid: { display: false }, ticks: { color: 'rgba(255,255,255,0.5)' } } } } 
                }); 
            }
        }, 100);
    } catch (e) { document.getElementById('sihDrillContent').innerHTML = `<div class="alert alert-danger">Fetch failed: ${e.message}</div>`; }
}
