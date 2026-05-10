/*
 * SQL Optima — PostgreSQL Storage & Index Health Dashboard
 */

window.runPgStorageIndexHealthDashboard = async function(opts) {
    const skipLoadingShell = !!(opts && opts.skipLoadingShell);
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
    window.appState.pgSih = window.appState.pgSih || {};
    const state = window.appState.pgSih;
    const dashTitle = 'Index & Table Health';
    
    // Time state
    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    const pad = n => String(n).padStart(2, '0');
    const fmtLocal = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

    state.fromLocal = state.fromLocal || fmtLocal(oneHourAgo);
    state.toLocal = state.toLocal || fmtLocal(now);
    
    if (state.db === undefined || (window.appState.currentDatabase && state.db !== window.appState.currentDatabase)) {
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
        return `instance=${encodeURIComponent(inst.name)}&from=${encodeURIComponent(fromIso)}&to=${encodeURIComponent(toIso)}&db=${encodeURIComponent(db)}&schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}`;
    };

    const fmt = (n, d=0) => {
        const v = Number(n);
        if (n == null || isNaN(v)) return '--';
        return v.toLocaleString(undefined, {minimumFractionDigits:d, maximumFractionDigits:d});
    };

    if (!skipLoadingShell) {
        window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_storage_index_health.html', { inst, dashTitle });
    }

    try {
        const base = `/api/timescale/postgres/storage-index-health`;
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

        const reload = (e) => { 
            if (e) e.preventDefault();
            if ($('sihFrom')) state.fromLocal = $('sihFrom').value;
            if ($('sihTo')) state.toLocal = $('sihTo').value;
            if ($('sihDb')) state.db = $('sihDb').value;
            if ($('sihSchema')) state.schema = $('sihSchema').value;
            if ($('sihTable')) state.table = $('sihTable').value;
            void window.runPgStorageIndexHealthDashboard({ skipLoadingShell: true }); 
        };
        $('sihRefresh').onclick = reload;
        $('sihApply').onclick = reload;
        $('sihDb').onchange = reload;
        $('sihSchema').onchange = reload;
        $('sihTable').onchange = reload;

        // 4. KPIs
        const updateKpi = (id, val, suffix, colorRules, subText) => {
            const el = $('kpi' + id); const card = $('card' + id);
            const delta = $('delta' + id);
            if (!el || !card) return;
            const v = Number(val) || 0;
            el.textContent = fmt(v, (id==='WriteAmp'||id==='Growth7d')?1:0) + (suffix || '');
            
            if (delta && subText) {
                delta.textContent = subText;
                delta.style.display = 'block';
            } else if (delta) {
                delta.style.display = 'none';
            }

            let severity = 'green';
            if (v >= colorRules.red) severity = 'red';
            else if (v >= colorRules.orange) severity = 'orange';
            card.style.borderLeft = `4px solid var(--${severity === 'green' ? 'success' : (severity === 'orange' ? 'warning' : 'danger')})`;
        };
        updateKpi('TotalSize', k.total_db_size_mb, ' MB', { orange: 500000, red: 1000000 });
        let growthSub = k.fastest_growing_table ? 'Top: ' + k.fastest_growing_table.split('.').pop() : '';
        updateKpi('Growth7d', k.growth_7d_pct, '%', { orange: 5, red: 15 }, growthSub);
        updateKpi('Forecast30d', k.forecast_table_mb_90d, ' MB', { orange: k.total_db_size_mb * 1.1, red: k.total_db_size_mb * 1.25 });
        updateKpi('Reclaimable', k.unused_index_mb, ' MB', { orange: 100, red: 1000 });
        updateKpi('Frag', k.avg_fragmentation_pct, '%', { orange: 10, red: 30 });
        updateKpi('WriteAmp', k.index_write_overhead_pct, '', { orange: 3, red: 6 });

        const isTableFiltered = state.table && state.table !== 'all';
        const filteredTableName = isTableFiltered ? state.table : '';

        // Insights
        const insights = $('sihInsightsBody');
        if (insights) {
            const insData = (dash.Insights || []);
            if (!insData.length) {
                insights.innerHTML = '<div class="text-success" style="font-size:0.85rem;"><i class="fa-solid fa-circle-check"></i> No significant risks detected.</div>';
            } else {
                insights.innerHTML = insData.map(ins => `
                    <div class="glass-panel sih-insight-item" style="font-size:0.82rem; display:flex; align-items:center; gap:0.75rem; padding:0.5rem; background:rgba(255,255,255,0.03);">
                        <span class="badge badge-${ins.severity==='critical'?'danger':(ins.severity==='warning'?'warning':'info')}" style="min-width:4.5rem; text-align:center;">${String(ins.severity).toUpperCase()}</span>
                        <span style="flex:1;">${window.escapeHtml(ins.message)}</span>
                        <button class="btn btn-xs btn-outline sih-view-btn" data-db="${window.escapeHtml(ins.db_name || '')}" data-schema="${window.escapeHtml(ins.schema_name || '')}" data-table="${window.escapeHtml(ins.table_name || '')}">View</button>
                    </div>
                `).join('');
                insights.querySelectorAll('.sih-view-btn').forEach(btn => {
                    btn.onclick = () => {
                        if (btn.dataset.table) showPgSihTableDrilldown(inst.name, btn.dataset.db, btn.dataset.schema, btn.dataset.table);
                    };
                });
            }
        }

        // Tables
        const ltBody = $('largestTablesBody');
        const totalSize = Number(k.total_db_size_mb) || 1;
        ltBody.innerHTML = (dash.largest_tables || []).map(t => {
            const totalVal = Number(t.value) || 0;
            const idxVal = Number(t.value2) || 0;
            const dataVal = totalVal - idxVal;
            const ratio = totalVal > 0 ? (idxVal / totalVal) * 100 : 0;
            const pctDb = (totalVal / totalSize) * 100;
            const g = Number(t.growth_pct) || 0;
            const risk = g > 25 ? 'danger' : (g > 10 ? 'warning' : 'success');
            return `
                <tr style="cursor:pointer;" data-action="pg-sih-drilldown" data-db="${window.escapeHtml(t.db_name)}" data-schema="${window.escapeHtml(t.schema_name)}" data-table="${window.escapeHtml(t.table_name)}">
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

        const ieBody = $('indexEfficiencyBody');
        ieBody.innerHTML = (dash.unused_indexes || []).map(idx => {
            const ratio = idx.value / (idx.updates || 1);
            let rec = 'HEALTHY', cls = 'success';
            if (idx.value === 0) { rec = 'DROP'; cls = 'danger'; }
            else if (idx.fragmentation > 30) { rec = 'REBUILD'; cls = 'warning'; }
            else if (ratio < 1) { rec = 'HIGH WRITE COST'; cls = 'warning'; }
            return `<tr><td><span class="text-muted" style="font-size:0.65rem;">${window.escapeHtml(idx.table_name)}.</span><br><strong>${window.escapeHtml(idx.index_name)}</strong></td><td class="text-right">${fmt(idx.value2, 1)}</td><td class="text-right">${fmt(ratio, 2)}</td><td class="text-right">${fmt(idx.fragmentation || 0, 0)}%</td><td><span class="badge badge-${cls}">${rec}</span></td></tr>`;
        }).join('') || '<tr><td colspan="5" class="text-center text-success p-3">No efficiency issues detected.</td></tr>';

        $('btnGrowthAbs').onclick = () => { state.growthMode = 'abs'; reload(); };
        $('btnGrowthPct').onclick = () => { state.growthMode = 'pct'; reload(); };

        setTimeout(() => {
            renderPgSihSparkline('sparkTotalSize', (dash.growth || []).map(p => p.table_size_mb), '#3b82f6');
            renderPgSihSparkline('sparkGrowth7d', (dash.growth || []).map(p => p.table_size_mb), '#10b981');
            renderPgSihGrowthChart(dash.growth || [], filteredTableName);
            renderPgSihTopGrowthChart(dash.growth || [], state.growthMode, filteredTableName);
        }, 100);

        if (!window.pgSihClickListenerAdded) {
            window.routerOutlet.addEventListener('click', (e) => {
                const tr = e.target.closest('tr[data-action="pg-sih-drilldown"]');
                if (tr) showPgSihTableDrilldown(inst.name, tr.dataset.db, tr.dataset.schema, tr.dataset.table);
            });
            window.pgSihClickListenerAdded = true;
        }

    } catch (e) {
        console.error('PG SIH dashboard failed', e);
        if ($('sihDashboardBody')) $('sihDashboardBody').innerHTML = `<div class="alert alert-danger">Analytical load failed: ${e.message}</div>`;
    }
};

function renderPgSihSparkline(canvasId, data, color) {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();
    if (!data || !data.length) return;
    new Chart(canvas, {
        type: 'line',
        data: { labels: data.map((_, i) => i), datasets: [{ data: data, borderColor: color, borderWidth: 1.5, pointRadius: 0, fill: false, tension: 0.4 }] },
        options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false }, tooltip: { enabled: false } }, scales: { x: { display: false }, y: { display: false } } }
    });
}

function renderPgSihGrowthChart(growth, tableName) {
    const canvas = document.getElementById('chartGrowth');
    if (!canvas) return;
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();
    const labelSuffix = tableName ? ` (${tableName})` : '';
    new Chart(canvas, {
        type: 'line',
        data: {
            labels: growth.map(p => new Date(p.bucket).toLocaleDateString()),
            datasets: [
                { label: 'Table Data' + labelSuffix, data: growth.map(p => p.table_size_mb), borderColor: '#3b82f6', backgroundColor: '#3b82f622', fill: true, tension: 0.3 },
                { label: 'Indexes' + labelSuffix, data: growth.map(p => p.index_size_mb), borderColor: '#10b981', backgroundColor: '#10b98122', fill: true, tension: 0.3 }
            ]
        },
        options: { responsive: true, maintainAspectRatio: false, scales: { y: { ticks: { color: '#94a3b8', font: { size: 10 } } }, x: { ticks: { color: '#94a3b8', font: { size: 10 } } } }, plugins: { legend: { labels: { color: '#94a3b8' } } } }
    });
}

function renderPgSihTopGrowthChart(growth, mode, tableName) {
    const canvas = document.getElementById('chartTopGrowth');
    if (!canvas) return;
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();
    const data = growth.map((p,i) => { if (i===0) return 0; const diff = p.table_size_mb - growth[i-1].table_size_mb; return mode==='pct' ? (diff/(growth[i-1].table_size_mb||1))*100 : diff; });
    new Chart(canvas, {
        type: 'bar', data: { labels: growth.map(p => new Date(p.bucket).toLocaleDateString()), datasets: [{ label: (mode==='pct'?'Growth %':'Growth MB') + (tableName?` (${tableName})`:''), data, backgroundColor: '#f43f5e' }] },
        options: { responsive: true, maintainAspectRatio: false }
    });
}

async function showPgSihTableDrilldown(instance, db, schema, table) {
    const fromIso = new Date(Date.now() - 30 * 24 * 3600 * 1000).toISOString();
    const toIso = new Date().toISOString();
    const url = `/api/sqlserver/storage-index/table-drilldown?engine=postgres&instance=${encodeURIComponent(instance)}&db=${encodeURIComponent(db)}&schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}&from=${fromIso}&to=${toIso}`;
    
    // Create modal
    const existing = document.getElementById('sihTableDrilldownModal');
    if (existing) existing.remove();
    
    const modal = document.createElement('div');
    modal.id = 'sihTableDrilldownModal';
    modal.className = 'modal-overlay';
    modal.style.cssText = 'display:flex; position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.8); align-items:center; justify-content:center;';
    
    modal.innerHTML = `
        <div class="glass-panel" style="background:var(--bg-surface); width:95%; max-width:1000px; max-height:90vh; overflow-y:auto; border-radius:12px; border:1px solid var(--border-color); display:flex; flex-direction:column;">
            <div class="modal-header flex-between" style="padding:1rem; border-bottom:1px solid var(--border-color);">
                <h3 style="margin:0;"><i class="fa-solid fa-magnifying-glass-chart text-accent"></i> ${schema}.${table} <span class="text-muted" style="font-size:0.8rem;">(30-day History)</span></h3>
                <button class="btn btn-icon" id="closeSihModal" style="background:transparent; border:none; color:var(--text); font-size:1.5rem; cursor:pointer;">&times;</button>
            </div>
            <div class="modal-body" style="padding:1.5rem; flex:1;">
                <div id="sihModalLoading" class="text-center p-5"><i class="fa-solid fa-spinner fa-spin fa-2xl text-accent"></i><p class="mt-3">Analyzing storage telemetry...</p></div>
                <div id="sihModalContent" style="display:none;">
                    <div class="charts-grid" style="display:grid; grid-template-columns:1.2fr 0.8fr; gap:1rem; margin-bottom:1.5rem;">
                        <div class="glass-panel" style="height:300px; padding:1rem;">
                            <h4 style="font-size:0.8rem; text-transform:uppercase; margin-bottom:0.5rem;" class="text-muted">Size History (MB)</h4>
                            <div style="height:240px;"><canvas id="sihModalGrowthChart"></canvas></div>
                        </div>
                        <div class="glass-panel" style="padding:1rem;">
                            <h4 style="font-size:0.8rem; text-transform:uppercase; margin-bottom:0.5rem;" class="text-muted">Index Efficiency</h4>
                            <div class="table-responsive">
                                <table class="data-table" style="font-size:0.75rem;">
                                    <thead><tr><th>Index</th><th>Scans</th><th>Updates</th></tr></thead>
                                    <tbody id="sihModalIndexBody"></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                    <div class="glass-panel" style="padding:1rem;">
                        <h4 style="font-size:0.8rem; text-transform:uppercase; margin-bottom:0.5rem;" class="text-muted">Index Fragmentation Trend</h4>
                        <div style="height:200px;"><canvas id="sihModalFragChart"></canvas></div>
                    </div>
                </div>
            </div>
        </div>
    `;
    
    document.body.appendChild(modal);
    const closeModal = () => modal.remove();
    document.getElementById('closeSihModal').onclick = closeModal;
    modal.onclick = (e) => { if (e.target === modal) closeModal(); };
    
    try {
        const res = await window.apiClient.authenticatedFetch(url);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        
        document.getElementById('sihModalLoading').style.display = 'none';
        document.getElementById('sihModalContent').style.display = 'block';
        
        // Render Growth Chart
        const growthCtx = document.getElementById('sihModalGrowthChart').getContext('2d');
        new Chart(growthCtx, {
            type: 'line',
            data: {
                labels: (data.growth_series || []).map(p => new Date(p.bucket).toLocaleDateString()),
                datasets: [
                    { label: 'Data', data: (data.growth_series || []).map(p => p.table_size_mb), borderColor: '#3b82f6', backgroundColor: '#3b82f622', fill: true, tension: 0.3 },
                    { label: 'Indexes', data: (data.growth_series || []).map(p => p.index_size_mb), borderColor: '#10b981', backgroundColor: '#10b98122', fill: true, tension: 0.3 }
                ]
            },
            options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'bottom', labels: { boxWidth: 12, font: { size: 10 } } } } }
        });
        
        // Render Index Body
        const idxBody = document.getElementById('sihModalIndexBody');
        idxBody.innerHTML = (data.index_usage || []).map(i => `
            <tr>
                <td><strong>${window.escapeHtml(i.index_name)}</strong></td>
                <td>${Number(i.value).toLocaleString()}</td>
                <td>${Number(i.updates || 0).toLocaleString()}</td>
            </tr>
        `).join('') || '<tr><td colspan="3" class="text-center p-3">No index usage data found.</td></tr>';
        
        // Render Frag Chart
        const fragCtx = document.getElementById('sihModalFragChart').getContext('2d');
        new Chart(fragCtx, {
            type: 'bar',
            data: {
                labels: (data.fragmentation || []).map(p => new Date(p.bucket).toLocaleDateString()),
                datasets: [{ label: 'Avg Fragmentation %', data: (data.fragmentation || []).map(p => p.fragmentation), backgroundColor: '#f59e0b' }]
            },
            options: { responsive: true, maintainAspectRatio: false, scales: { y: { beginAtZero: true, max: 100 } } }
        });
        
    } catch (e) {
        document.getElementById('sihModalLoading').innerHTML = `<div class="alert alert-danger">Drilldown failed: ${e.message}</div>`;
    }
}
