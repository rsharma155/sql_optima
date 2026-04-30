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
                    <div style="display:flex; align-items:center; gap:1rem;">
                        <button class="btn btn-secondary btn-sm" data-action="navigate-back" title="Back to Control Center">
                            <i class="fa-solid fa-arrow-left"></i> Back
                        </button>
                        <div class="dashboard-title-line" style="flex:1; min-width:0;">
                            <h1><i class="fa-solid fa-boxes-stacked text-accent"></i> ${dashTitle}</h1>
                        </div>
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
                <div class="charts-grid" style="display:grid; grid-template-columns:repeat(6, minmax(0, 1fr)); gap:0.5rem;">
                    ${['TotalSize', 'Growth7d', 'Forecast30d', 'Reclaimable', 'Frag', 'WriteAmp'].map((id, i) => {
                        const labels = ['DB Size', '7d Growth', '30d Forecast', 'Reclaimable', 'Avg Frag', 'Write Amp'];
                        return `
                            <div class="metric-card glass-panel sih-kpi-card" id="card${id}" style="padding:0.4rem 0.6rem; min-height:75px; display:flex; flex-direction:column; justify-content:center;">
                                <div class="text-muted" style="font-size:0.6rem; text-transform:uppercase;">${labels[i]}</div>
                                <div id="kpi${id}" style="font-size:1rem; font-weight:700; line-height:1.2;">--</div>
                                <div id="delta${id}" style="font-size:0.6rem;"></div>
                                <div style="height:20px; margin-top:0.25rem;"><canvas id="spark${id}"></canvas></div>
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
            if ($('sihFrom')) state.fromLocal = $('sihFrom').value;
            if ($('sihTo')) state.toLocal = $('sihTo').value;
            if ($('sihDb')) state.db = $('sihDb').value;
            if ($('sihSchema')) state.schema = $('sihSchema').value;
            if ($('sihTable')) state.table = $('sihTable').value;
        };
        const reload = (e) => { 
            if (e) e.preventDefault();
            sync(); 
            void window.runStorageIndexHealthDashboard({ skipLoadingShell: true }); 
        };
        $('sihRefresh').onclick = reload;
        $('sihApply').onclick = reload;

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

        const isTableFiltered = state.table && state.table !== 'all';

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
                <div class="table-card glass-panel">
                    <div class="card-header flex-between">
                        <h3 style="font-size:0.85rem; margin:0;">Largest Tables Diagnostic ${isTableFiltered ? '<span class="text-accent ml-2">(Filtered)</span>' : ''}</h3>
                        ${isTableFiltered ? '<span class="text-muted" style="font-size:0.7rem;"><i class="fa-solid fa-circle-info"></i> Click row to see table drill-down details</span>' : ''}
                    </div>
                    <div class="table-responsive"><table class="data-table" style="font-size:0.72rem;"><thead><tr><th>Table</th><th>Total MB</th><th>Data MB</th><th>Idx Ratio</th><th>% DB</th><th>30d Forecast</th><th>Risk</th></tr></thead><tbody id="largestTablesBody"></tbody></table></div>
                </div>
                <div class="table-card glass-panel"><div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Index Efficiency & Recommendation</h3></div><div class="table-responsive"><table class="data-table" style="font-size:0.72rem;"><thead><tr><th class="sortable" data-col="index_name">Index</th><th class="sortable text-right" data-col="value2">MB</th><th class="sortable text-right" data-col="ratio">R:W</th><th class="sortable text-right" data-col="frag">Frag</th><th>Rec.</th></tr></thead><tbody id="indexEfficiencyBody"></tbody></table></div></div>
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
            const data = (dash.Insights || []);
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

        // Add event listener only ONCE on router outlet for better lifecycle management
        if (!window.sihClickListenerAdded) {
            window.routerOutlet.addEventListener('click', (e) => {
                const tr = e.target.closest('tr[data-action="sih-drilldown"]');
                if (tr) showSihTableDrilldown(inst.name, tr.dataset.db, tr.dataset.schema, tr.dataset.table);
                
                const closeBtn = e.target.closest('[data-action="close-drilldown"]');
                if (closeBtn) {
                    const m = document.getElementById('sih-drilldown-modal');
                    if (m) m.remove();
                }
                
                const tabBtn = e.target.closest('.sih-tab-btn');
                if (tabBtn) {
                    const target = tabBtn.dataset.tab;
                    document.querySelectorAll('.sih-tab-btn').forEach(b => b.classList.toggle('active', b === tabBtn));
                    document.querySelectorAll('.sih-drill-pane').forEach(p => p.style.display = p.id === 'drill-' + target ? 'block' : 'none');
                    
                    // Trigger chart resize/re-render if needed
                    if (target === 'growth' && window._sihDrillGrowthChart) {
                        window._sihDrillGrowthChart.update();
                    } else if (target === 'indexes' && window._sihDrillIndexTrendChart) {
                        window._sihDrillIndexTrendChart.update();
                    }
                }
            });
            window.sihClickListenerAdded = true;
        }

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
        type: 'line',
        data: {
            labels: data.map((_, i) => i),
            datasets: [{
                data: data,
                borderColor: color,
                borderWidth: 1.5,
                pointRadius: 0,
                fill: false,
                tension: 0.4
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: false }, tooltip: { enabled: false } },
            scales: { x: { display: false }, y: { display: false } }
        }
    });
}

function renderGrowthChart(growth) {
    const canvas = document.getElementById('chartGrowth');
    if (!canvas) return;
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();

    new Chart(canvas, {
        type: 'line',
        data: {
            labels: growth.map(p => new Date(p.bucket).toLocaleDateString()),
            datasets: [
                { label: 'Table Data', data: growth.map(p => p.table_size_mb), borderColor: '#3b82f6', backgroundColor: '#3b82f622', fill: true, tension: 0.3 },
                { label: 'Indexes', data: growth.map(p => p.index_size_mb), borderColor: '#10b981', backgroundColor: '#10b98122', fill: true, tension: 0.3 }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            scales: { y: { beginAtZero: false, ticks: { color: '#94a3b8', font: { size: 10 } }, grid: { color: 'rgba(255,255,255,0.05)' } }, x: { ticks: { color: '#94a3b8', font: { size: 10 }, maxTicksLimit: 8 }, grid: { display: false } } },
            plugins: { legend: { position: 'top', align: 'end', labels: { boxWidth: 12, color: '#94a3b8', font: { size: 10 } } } }
        }
    });
}

function renderTopGrowthChart(growth, mode) {
    const canvas = document.getElementById('chartTopGrowth');
    if (!canvas) return;
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();

    // Mode 'abs' = delta MB, 'pct' = %
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
    modal.style.cssText = 'position:fixed; z-index:99999; inset:0; background:rgba(0,0,0,0.85); display:flex; align-items:center; justify-content:center; padding: 2rem;';

    modal.innerHTML = `
        <div class="glass-panel" style="width:100%; max-width:1100px; height:85vh; display:flex; flex-direction:column; background:var(--bg-surface); box-shadow: 0 0 50px rgba(0,0,0,0.5); border: 1px solid var(--border-color);">
            <div class="flex-between p-4" style="border-bottom: 1px solid var(--border-color);">
                <div>
                    <h2 style="margin:0; color:var(--accent); font-size:1.4rem;"><i class="fa-solid fa-table"></i> Table Analysis: ${window.escapeHtml(schema)}.${window.escapeHtml(table)}</h2>
                    <span class="text-muted" style="font-size:0.85rem;">Database: <strong>${window.escapeHtml(db)}</strong> | Instance: ${window.escapeHtml(instance)}</span>
                </div>
                <button class="btn btn-sm btn-outline" data-action="close-drilldown" style="padding: 0.5rem 1rem;"><i class="fa-solid fa-times"></i> Close</button>
            </div>
            
            <div class="flex p-2" style="background: rgba(0,0,0,0.2); border-bottom: 1px solid var(--border-color); gap: 0.5rem;">
                <button class="btn btn-xs sih-tab-btn active" data-tab="breakdown">Breakdown</button>
                <button class="btn btn-xs sih-tab-btn" data-tab="growth">Growth History</button>
                <button class="btn btn-xs sih-tab-btn" data-tab="indexes">Indexes</button>
            </div>

            <div id="sihDrillContent" style="flex:1; overflow:auto; padding:1.5rem;">
                <div style="display:flex; justify-content:center; align-items:center; height:100%;">
                    <div class="spinner"></div><span class="ml-3">Loading table analytics...</span>
                </div>
            </div>
        </div>
    `;

    document.body.appendChild(modal);

    // Add Event Listeners for the modal (Fix for non-working tabs and close button)
    modal.addEventListener('click', (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;

        if (btn.dataset.action === 'close-drilldown') {
            modal.remove();
            return;
        }

        if (btn.classList.contains('sih-tab-btn')) {
            const tab = btn.dataset.tab;
            // Update buttons
            modal.querySelectorAll('.sih-tab-btn').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));
            // Update panes
            modal.querySelectorAll('.sih-drill-pane').forEach(p => {
                const isTarget = p.id === `drill-${tab}`;
                p.style.display = isTarget ? 'block' : 'none';
                p.classList.toggle('active', isTarget);
            });
        }
    });

    try {
        const fromIso = new Date(Date.now() - 30 * 24 * 3600 * 1000).toISOString();
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
        `;

        // Render Charts
        if (window._sihDrillBreakdownChart) window._sihDrillBreakdownChart.destroy();
        window._sihDrillBreakdownChart = new Chart(document.getElementById('drillBreakdownChart'), {
            type: 'doughnut',
            data: { labels: ['Data', 'Indexes'], datasets: [{ data: [last.table_size_mb, last.index_size_mb], backgroundColor: ['#3b82f6', '#10b981'], borderWeight: 0 }] },
            options: { responsive: true, maintainAspectRatio: false, cutout: '70%', plugins: { legend: { position: 'bottom', labels: { color: '#94a3b8' } } } }
        });

        if (window._sihDrillGrowthChart) window._sihDrillGrowthChart.destroy();
        window._sihDrillGrowthChart = new Chart(document.getElementById('drillGrowthChart'), {
            type: 'line',
            data: {
                labels: (data.growth_series || []).map(p => new Date(p.time).toLocaleDateString()),
                datasets: [
                    { label: 'Table MB', data: (data.growth_series || []).map(p => p.table_size_mb), borderColor: '#3b82f6', fill: false },
                    { label: 'Index MB', data: (data.growth_series || []).map(p => p.index_size_mb), borderColor: '#10b981', fill: false }
                ]
            },
            options: { responsive: true, maintainAspectRatio: false, scales: { y: { beginAtZero: false, grid: { color: 'rgba(255,255,255,0.05)' } } } }
        });

        if (window._sihDrillIndexTrendChart) window._sihDrillIndexTrendChart.destroy();
        window._sihDrillIndexTrendChart = new Chart(document.getElementById('drillIndexTrendChart'), {
            type: 'line',
            data: {
                labels: (data.index_usage || []).filter(ix => ix.index_name === latestIndexes[0]?.index_name).map(p => new Date(p.time).toLocaleTimeString()),
                datasets: [
                    { label: 'Seeks/Scans', data: (data.index_usage || []).filter(ix => ix.index_name === latestIndexes[0]?.index_name).map(p => p.seeks), borderColor: '#3b82f6', tension: 0.4 },
                    { label: 'Updates', data: (data.index_usage || []).filter(ix => ix.index_name === latestIndexes[0]?.index_name).map(p => p.updates), borderColor: '#f43f5e', tension: 0.4 }
                ]
            },
            options: { responsive: true, maintainAspectRatio: false, scales: { y: { beginAtZero: true } } }
        });

    } catch (e) {
        console.error('Table drilldown failed', e);
        document.getElementById('sihDrillContent').innerHTML = `<div class="alert alert-danger">Load failed: ${e.message}</div>`;
    }
}
