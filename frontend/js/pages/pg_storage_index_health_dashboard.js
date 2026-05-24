/*
 * SQL Optima — PostgreSQL Storage & Index Health Dashboard
 */

window.runPgStorageIndexHealthDashboard = async function(opts) {
    const skipLoadingShell = !!(opts && opts.skipLoadingShell);
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
    window.appState.pgSih = window.appState.pgSih || {};
    const state = window.appState.pgSih;
    
    // Time state - Default to 24h as per redesign spec
    const now = new Date();
    const defaultRange = 24 * 60 * 60 * 1000;
    const pad = n => String(n).padStart(2, '0');
    const fmtLocal = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

    state.fromLocal = state.fromLocal || fmtLocal(new Date(now.getTime() - defaultRange));
    state.toLocal = state.toLocal || fmtLocal(now);
    
    let isInitialLoad = false;
    if (state.db === undefined || (window.appState.currentDatabase && state.db !== window.appState.currentDatabase)) {
        isInitialLoad = true;
        state.db = (window.appState.currentDatabase && window.appState.currentDatabase !== 'all') ? window.appState.currentDatabase : 'all';
    }
    state.schema = state.schema || 'all';
    state.table = state.table || 'all';
    state.growthMode = state.growthMode || 'abs'; 

    if (!skipLoadingShell) {
        window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_storage_index_health.html', { inst });
    }

    const $ = (id) => document.getElementById(id);
    const container = $('sihDashboardBody');
    if (container) container.classList.add('loading');

    try {
        const base = `/api/timescale/postgres/storage-index-health`;
        const filterQS = window.sihShared.buildFilterQS(state, inst.name);
        
        const [filters, dash] = await Promise.all([
            window.apiClient.authenticatedFetch(`${base}/filters?${filterQS}`).then(r => r.json()),
            window.apiClient.authenticatedFetch(`${base}/dashboard?${filterQS}`).then(r => r.json())
        ]);

        if (container) container.classList.remove('loading');

        const k = dash.kpis || {};
        const gs = dash.growth_summary || {};
        const growth = dash.growth || [];

        // 1. Inputs & Presets
        const fromIn = $('sihFrom'); const toIn = $('sihTo');
        if (fromIn) fromIn.value = state.fromLocal;
        if (toIn) toIn.value = state.toLocal;

        document.querySelectorAll('.sih-preset').forEach(btn => {
            btn.onclick = () => {
                const h = parseInt(btn.dataset.h);
                const end = new Date();
                const start = new Date(end.getTime() - h * 3600 * 1000);
                state.fromLocal = fmtLocal(start);
                state.toLocal = fmtLocal(end);
                reload();
            };
        });

        // 2. Health Score
        (function renderHealthScore() {
            const score = window.sihShared.buildHealthScore(k);
            const container = $('sihHealthScoreContainer');
            if (!container) return;
            const color = score >= 80 ? '#10b981' : (score >= 60 ? '#f59e0b' : '#f43f5e');
            container.innerHTML = `
                <div style="width:50px; height:50px; border-radius:50%; border:4px solid ${color}; display:flex; align-items:center; justify-content:center; font-weight:700; color:${color}; font-size:1.1rem; background:rgba(255,255,255,0.03);">
                    ${score}
                </div>
            `;
        })();

        // 3. Dropdowns — auto-select first database on initial landing so data is
        //    scoped by default instead of showing an aggregated cross-db view.
        if (isInitialLoad && state.db === 'all' && filters.databases && filters.databases.length > 0) {
            state.db = filters.databases[0];
            // Re-fetch with the auto-selected database so schema/table dropdowns
            // are correctly scoped from the start.
            void window.runPgStorageIndexHealthDashboard({ skipLoadingShell: true });
            return;
        }

        const syncSelect = (id, options, current) => {
            const el = $(id);
            if (!el) return;
            el.innerHTML = '<option value="all">All</option>' + (options || []).map(o => `<option value="${o}" ${current===o?'selected':''}>${o}</option>`).join('');
        };
        syncSelect('sihDb', filters.databases, state.db);
        syncSelect('sihSchema', filters.schemas, state.schema);
        syncSelect('sihTable', filters.tables, state.table);

        const reload = (e) => { 
            if (e) e.preventDefault();
            if ($('sihFrom')) state.fromLocal = $('sihFrom').value;
            if ($('sihTo')) state.toLocal = $('sihTo').value;
            if ($('sihDb')) state.db = $('sihDb').value;
            if ($('sihSchema')) state.schema = $('sihSchema').value;
            if ($('sihTable')) state.table = $('sihTable').value;
            void window.runPgStorageIndexHealthDashboard({ skipLoadingShell: true }); 
        };
        const sihRefreshEl = $('sihRefresh'); if (sihRefreshEl) sihRefreshEl.onclick = reload;
        const sihApplyEl = $('sihApply'); if (sihApplyEl) sihApplyEl.onclick = reload;
        const sihDbEl = $('sihDb'); if (sihDbEl) sihDbEl.onchange = reload;
        const sihSchemaEl = $('sihSchema'); if (sihSchemaEl) sihSchemaEl.onchange = reload;
        const sihTableEl = $('sihTable'); if (sihTableEl) sihTableEl.onchange = reload;

        // 4. KPIs
        const sihKpi = (cardId, valId, val, suffix, rawVal, rules) => {
            const card = $(cardId);
            const el = $(valId);
            if (!el || !card) return;
            el.textContent = val + (suffix || '');
            let sev = 'success';
            if (rawVal >= rules.red) sev = 'danger';
            else if (rawVal >= rules.orange) sev = 'warning';
            card.style.borderLeft = `4px solid var(--${sev})`;
        };

        const totalGB = k.total_db_size_mb / 1024;
        sihKpi('cardTotalSize',    'kpiTotalSize',    window.sihShared.fmt(totalGB, 1), ' GB', totalGB, {orange: 512, red: 2048});
        sihKpi('cardGrowth7d',     'kpiGrowth7d',     window.sihShared.fmt(k.growth_7d_pct, 1), '%', k.growth_7d_pct, {orange: 5, red: 15});
        sihKpi('cardForecast90d',  'kpiForecast90d',  window.sihShared.fmt(k.forecast_table_mb_90d / 1024, 1), ' GB', k.forecast_table_mb_90d / (k.total_db_size_mb || 1) * 100, {orange: 110, red: 125});
        sihKpi('cardWriteOverhead','kpiWriteOverhead', window.sihShared.fmt(k.index_write_overhead_pct, 1), '%', k.index_write_overhead_pct, {orange: 30, red: 60});

        sihKpi('cardUnusedCount',  'kpiUnusedCount',  window.sihShared.fmt(k.unused_index_count, 0), '', k.unused_index_count, {orange: 5, red: 20});
        sihKpi('cardUnusedMB',     'kpiUnusedMB',     window.sihShared.fmt(k.unused_index_mb, 0), ' MB', k.unused_index_mb, {orange: 100, red: 1000});
        sihKpi('cardHighScan',     'kpiHighScan',     window.sihShared.fmt(k.high_scan_table_count, 0), '', k.high_scan_table_count, {orange: 3, red: 10});
        sihKpi('cardDailyGrowth',  'kpiDailyGrowth',  window.sihShared.fmt(gs.daily_growth_mb, 1), ' MB/d', gs.daily_growth_mb, {orange: 50, red: 200});

        // 5. Shared Renders
        window.sihShared.renderBanner('sihHealthBanner', k, gs);
        window.sihShared.renderInsights('sihInsightsBody', dash.insights || dash.Insights || []);
        window.sihShared.renderProjectionStrip(dash);
        window.sihShared.renderHighScanTables('highScanTablesBody', dash.high_scan_tables, 'postgres');
        window.sihShared.renderLargestTables('largestTablesBody', dash.largest_tables, 'postgres', 'pg-sih-drilldown');
        window.sihShared.renderLargestIndexes('largestIndexesBody', dash.largest_indexes);
        window.sihShared.renderIndexEfficiency('indexEfficiencyBody', dash.unused_indexes, 'postgres');
        window.sihShared.renderDuplicateIndexes('dupIdxBody', 'dupIdxCount', dash.raw_duplicate_indexes, 'postgres');
        window.sihShared.wireCopyButtons(container);

        const copyAllBtn = $('btnCopyAllRecs');
        if (copyAllBtn) {
            copyAllBtn.onclick = () => {
                const scripts = (dash.unused_indexes || [])
                    .filter(idx => Number(idx.value) === 0)
                    .map(idx => `DROP INDEX IF EXISTS ${idx.schema_name}.${idx.index_name};`)
                    .join('\n');
                if (!scripts) {
                    alert('No DROP candidates found.');
                    return;
                }
                navigator.clipboard.writeText(`-- Generated by SQL Optima\n${scripts}`)
                    .then(() => {
                        copyAllBtn.textContent = '✓ Copied!';
                        setTimeout(() => { copyAllBtn.innerHTML = '<i class="fa-solid fa-copy"></i> Copy All'; }, 2000);
                    });
            };
        }

        // 6. Charts
        setTimeout(() => {
            window.sihShared.renderSparkline('sparkTotalSize',   growth.map(p => p.table_size_mb), '#3b82f6');
            window.sihShared.renderSparkline('sparkGrowth7d',    growth.map(p => p.table_size_mb), '#10b981');
            
            window.sihShared.renderGrowthChart('chartGrowth', growth, 'postgres');
            window.sihShared.renderTopGrowthChart('chartTopGrowth', dash.top_growth_tables || [], state.growthMode);
            window.sihShared.renderSeekScanLookupChart('chartSeekScanLookup', dash.seek_scan_lookup || [], 'postgres');
        }, 0);

        // 7. Event Listeners (AbortController to prevent leaks)
        if (state._clickController) state._clickController.abort();
        state._clickController = new AbortController();
        
        window.routerOutlet.addEventListener('click', (e) => {
            const tr = e.target.closest('tr[data-action="pg-sih-drilldown"]');
            if (tr) showPgSihTableDrilldown(inst.name, tr.dataset.db, tr.dataset.schema, tr.dataset.table);
        }, { signal: state._clickController.signal });

        // Duplicate panel toggle
        const dupToggle = $('dupIdxToggle');
        if (dupToggle) {
            dupToggle.onclick = () => {
                const body = $('dupIdxBody');
                const chevron = $('dupIdxChevron');
                const isOpen = body.style.display !== 'none';
                body.style.display = isOpen ? 'none' : 'block';
                chevron.className = isOpen ? 'fa-solid fa-chevron-down' : 'fa-solid fa-chevron-up';
            };
        }

        const btnAbs = $('btnGrowthAbs'); if (btnAbs) btnAbs.onclick = () => { state.growthMode = 'abs'; reload(); };
        const btnPct = $('btnGrowthPct'); if (btnPct) btnPct.onclick = () => { state.growthMode = 'pct'; reload(); };

    } catch (e) {
        console.error('PostgreSQL SIH dashboard failed', e);
        if (container) {
            container.classList.remove('loading');
            container.innerHTML = `<div class="alert alert-danger">Analytical load failed: ${e.message}</div>`;
        }
    }
};

async function showPgSihTableDrilldown(instance, db, schema, table) {
    const fromIso = new Date(Date.now() - 30 * 24 * 3600 * 1000).toISOString();
    const toIso = new Date().toISOString();
    // For Postgres, we use the unified timescale storage endpoint
    const url = `/api/timescale/postgres/storage-index-health/index-usage?instance=${encodeURIComponent(instance)}&db=${encodeURIComponent(db)}&schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}&from=${fromIso}&to=${toIso}`;
    
    const existing = document.getElementById('sihTableDrilldownModal');
    if (existing) existing.remove();
    
    const modal = document.createElement('div');
    modal.id = 'sihTableDrilldownModal';
    modal.className = 'modal-overlay';
    modal.style.cssText = 'display:flex; position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.8); align-items:center; justify-content:center;';
    
    modal.innerHTML = `
        <div class="glass-panel" style="background:var(--bg-surface); width:95%; max-width:1000px; max-height:90vh; overflow-y:auto; border-radius:12px; border:1px solid var(--border-color); display:flex; flex-direction:column;">
            <div class="modal-header flex-between" style="padding:1rem; border-bottom:1px solid var(--border-color);">
                <h3 style="margin:0;"><i class="fa-solid fa-magnifying-glass-chart text-accent"></i> ${schema}.${table} <span class="text-muted" style="font-size:0.8rem;">(Index Usage Analysis)</span></h3>
                <button class="btn btn-icon" id="closeSihModal" style="background:transparent; border:none; color:var(--text); font-size:1.5rem; cursor:pointer;">&times;</button>
            </div>
            <div class="modal-body" style="padding:1.5rem; flex:1;">
                <div id="sihModalLoading" class="text-center p-5"><i class="fa-solid fa-spinner fa-spin fa-2xl text-accent"></i><p class="mt-3">Fetching index telemetry...</p></div>
                <div id="sihModalContent" style="display:none;">
                    <div class="table-responsive">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead><tr><th>Index Name</th><th>Reads (Idx Scans)</th><th>Updates</th><th>Size MB</th><th>Last Used</th></tr></thead>
                            <tbody id="sihModalIndexBody"></tbody>
                        </table>
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
        
        const points = data.points || (Array.isArray(data) ? data : []);
        const idxBody = document.getElementById('sihModalIndexBody');
        idxBody.innerHTML = points.map(i => `
            <tr>
                <td><strong>${window.sihShared.escH(i.index_name)}</strong></td>
                <td>${Number(i.scans || 0).toLocaleString()}</td>
                <td>${Number(i.updates || 0).toLocaleString()}</td>
                <td>${window.sihShared.fmt(i.index_size_mb, 1)}</td>
                <td>${i.last_user_seek ? new Date(i.last_user_seek).toLocaleDateString() : 'Never'}</td>
            </tr>
        `).join('') || '<tr><td colspan="5" class="text-center p-3">No index usage data found.</td></tr>';
        
    } catch (e) {
        document.getElementById('sihModalLoading').innerHTML = `<div class="alert alert-danger">Drilldown failed: ${e.message}</div>`;
    }
}
