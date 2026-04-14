/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Shared Timescale-backed Storage & Index Health dashboard (SQL Server and PostgreSQL; engine from instance type).
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
    const backRoute = (inst.type === 'postgres') ? 'pg-dashboard' : 'dashboard';
    const dashTitle = (engine === 'postgres') ? 'Index & Table Health' : 'Storage & Index Health';
    state.timeRange = state.timeRange || '24h';
    state.db = state.db || 'all';
    state.schema = state.schema || 'all';
    state.table = state.table || 'all';

    const fetchJson = async (url) => {
        const res = await window.apiClient.authenticatedFetch(url);
        const contentType = res.headers.get('content-type') || '';
        if (!contentType.includes('application/json')) {
            const text = await res.text().catch(() => '');
            const snippet = String(text || '').slice(0, 180).replace(/\s+/g, ' ').trim();
            throw new Error(`Non-JSON response (${res.status}) ct=${contentType || 'n/a'} body="${snippet}"`);
        }
        const body = await res.json();
        if (!res.ok) {
            throw new Error(body?.error || `HTTP ${res.status}`);
        }
        return body;
    };

    const buildFilterQS = () => {
        const db = (state.db && state.db !== 'all') ? state.db : '';
        const schema = (state.schema && state.schema !== 'all') ? state.schema : '';
        const table = (state.table && state.table !== 'all') ? state.table : '';
        return `engine=${encodeURIComponent(engine)}&instance=${encodeURIComponent(inst.name)}&time_range=${encodeURIComponent(state.timeRange)}&db=${encodeURIComponent(db)}&schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}`;
    };

    if (!skipLoadingShell) {
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-boxes-stacked text-accent"></i> ${dashTitle}</h1>
                    <p class="subtitle">Index usage, scan hotspots, and growth trends (Timescale-backed).</p>
                </div>
                <div style="display:flex; gap:0.5rem; align-items:center; flex-wrap:wrap;">
                    <div class="glass-panel" style="padding:0.5rem 0.75rem;">
                        <div class="text-muted" style="font-size:0.75rem;">Instance</div>
                        <div style="font-weight:600;">${window.escapeHtml(inst.name)}</div>
                    </div>
                    <button class="btn btn-sm btn-outline" onclick="window.appNavigate('${backRoute}')"><i class="fa-solid fa-arrow-left"></i> Back</button>
                </div>
            </div>
            <div class="glass-panel mt-2" style="padding:0.75rem;">
                <div style="font-size:0.78rem; color:var(--text-muted); line-height:1.45; margin-bottom:0.65rem; padding:0.5rem 0.65rem; background:rgba(59,130,246,0.08); border-radius:6px;">
                    <strong style="color:var(--text);">While this page loads:</strong> index usage is collected on about a <strong>15 min</strong> tick (see server logs <code style="font-size:0.72rem;">[Collector][SIH]</code>). You need Timescale connected and historical collection running (or a Redis worker consuming <code style="font-size:0.72rem;">historical</code> tasks).
                    ${engine === 'postgres'
                        ? ` For PostgreSQL, the collector reads <code style="font-size:0.72rem;">pg_stat_user_*</code> on the connected database.`
                        : ` If <code style="font-size:0.72rem;">Instances[].databases</code> is omitted, the collector auto-discovers user databases (<code>database_id &gt; 4</code>) your login can access; otherwise list DBs explicitly to limit scope.`}
                </div>
                <div style="display:flex; gap:0.75rem; flex-wrap:wrap; align-items:center;">
                    <div>
                        <div class="text-muted" style="font-size:0.75rem;">Time range</div>
                        <select id="sihRange" class="custom-select">
                            <option value="1h" ${state.timeRange==='1h'?'selected':''}>1h</option>
                            <option value="24h" ${state.timeRange==='24h'?'selected':''}>24h</option>
                            <option value="7d" ${state.timeRange==='7d'?'selected':''}>7d</option>
                            <option value="30d" ${state.timeRange==='30d'?'selected':''}>30d</option>
                        </select>
                    </div>
                    <div style="min-width:220px;">
                        <div class="text-muted" style="font-size:0.75rem;">Database</div>
                        <select id="sihDb" class="custom-select"><option value="all">All</option></select>
                    </div>
                    <div style="min-width:220px;">
                        <div class="text-muted" style="font-size:0.75rem;">Schema</div>
                        <select id="sihSchema" class="custom-select"><option value="all">All</option></select>
                    </div>
                    <div style="min-width:260px;">
                        <div class="text-muted" style="font-size:0.75rem;">Table</div>
                        <select id="sihTable" class="custom-select"><option value="all">All</option></select>
                    </div>
                    <button class="btn btn-sm btn-accent" id="sihApply"><i class="fa-solid fa-filter"></i> Apply</button>
                    <button class="btn btn-sm btn-outline" id="sihRefresh"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>
            <div class="glass-panel mt-2" style="padding:0.75rem;">
                <div class="text-muted" style="font-size:0.8rem;">Loading metrics…</div>
                <div class="spinner" style="margin-top:0.5rem;"></div>
            </div>
        </div>
    `;
    }

    try {
        const applyHandlers = () => {
            const $ = (id) => document.getElementById(id);
            const sync = () => {
                state.timeRange = ($('sihRange')?.value || '24h');
                state.db = ($('sihDb')?.value || 'all');
                state.schema = ($('sihSchema')?.value || 'all');
                state.table = ($('sihTable')?.value || 'all');
            };
            const reload = () => { sync(); void window.runStorageIndexHealthDashboard({ skipLoadingShell: true }); };
            $('sihApply')?.addEventListener('click', reload);
            $('sihRefresh')?.addEventListener('click', reload);
            $('sihRange')?.addEventListener('change', reload);

            // Cascading filters: refetch in-place (no route navigation) so selects stay usable.
            $('sihDb')?.addEventListener('change', () => {
                sync();
                state.schema = 'all';
                state.table = 'all';
                void window.runStorageIndexHealthDashboard({ skipLoadingShell: true });
            });
            $('sihSchema')?.addEventListener('change', () => {
                sync();
                state.table = 'all';
                void window.runStorageIndexHealthDashboard({ skipLoadingShell: true });
            });
        };

        const base = `/api/timescale/storage-index-health`;
        // Load filter options first (drives the dropdowns)
        const filterQS = `engine=${encodeURIComponent(engine)}&instance=${encodeURIComponent(inst.name)}&time_range=${encodeURIComponent(state.timeRange)}&db=${encodeURIComponent((state.db && state.db !== 'all') ? state.db : '')}&schema=${encodeURIComponent((state.schema && state.schema !== 'all') ? state.schema : '')}`;
        const filters = await fetchJson(`${base}/filters?${filterQS}`);
        const dbs = Array.isArray(filters.databases) ? filters.databases : [];
        const schemas = Array.isArray(filters.schemas) ? filters.schemas : [];
        const tables = Array.isArray(filters.tables) ? filters.tables : [];

        // Load dashboard after filter options so the page can render consistent selects.
        const dash = await fetchJson(`${base}/dashboard?${buildFilterQS()}`);

        const fmt = (n, digits=0) => {
            const v = Number(n||0);
            if (!isFinite(v)) return '--';
            return v.toFixed(digits);
        };

        const k = dash.kpis || {};
        const topScans = Array.isArray(dash.top_scans) ? dash.top_scans : [];
        const seekScanLookup = Array.isArray(dash.seek_scan_lookup) ? dash.seek_scan_lookup : [];
        const largestTables = Array.isArray(dash.largest_tables) ? dash.largest_tables : [];
        const largestIndexes = Array.isArray(dash.largest_indexes) ? dash.largest_indexes : [];
        const growth = Array.isArray(dash.growth) ? dash.growth : [];
        const growthSummary = (dash && typeof dash.growth_summary === 'object' && dash.growth_summary) ? dash.growth_summary : {};
        const unusedIndexes = Array.isArray(dash.unused_indexes) ? dash.unused_indexes : [];
        const highScanTables = Array.isArray(dash.high_scan_tables) ? dash.high_scan_tables : [];
        const dupCandidates = Array.isArray(dash.duplicate_index_candidates) ? dash.duplicate_index_candidates : [];

        const rowCounts = filters.source_row_counts || {};
        const rcSum = Number(rowCounts.table_size_history || 0) + Number(rowCounts.table_usage_stats || 0) + Number(rowCounts.index_usage_stats || 0);
        const dataHealthBanner = rcSum === 0
            ? `<div class="alert alert-warning mt-2" style="font-size:0.8rem; margin:0;">
                <strong>No SIH rows in Timescale</strong> for <code>${window.escapeHtml(inst.name)}</code> in this time window — all of
                <code>monitor.index_usage_stats</code>, <code>monitor.table_usage_stats</code>, and <code>monitor.table_size_history</code> are empty.
                Verify Timescale connectivity, the background collector is running, and the instance name matches <code>server_id</code> written by the collector.
               </div>`
            : `<div class="glass-panel mt-2" style="padding:0.45rem 0.65rem; font-size:0.72rem; color:var(--text-muted);">
                Timescale row counts (${window.escapeHtml(state.timeRange)}): table_size_history=${rowCounts.table_size_history ?? 0}, table_usage_stats=${rowCounts.table_usage_stats ?? 0}, index_usage_stats=${rowCounts.index_usage_stats ?? 0}.
               </div>`;
        const emptyDiagCharts = (topScans.length === 0 && seekScanLookup.length === 0)
            ? `<div class="glass-panel mt-2" style="padding:0.5rem 0.65rem; font-size:0.78rem; color:var(--text-muted); border-left:3px solid var(--border-color);">No diagnostic chart series returned for this window (aggregates from <code>table_usage_stats</code> / <code>index_usage_stats</code>).</div>`
            : '';
        const emptyGrowthNote = growth.length === 0
            ? `<p class="text-muted" style="font-size:0.75rem; margin-top:0.35rem;">No daily buckets in <code>monitor.table_size_history</code> for this scope (growth snapshots run about every 6 hours).</p>`
            : '';
        const emptyLargestTbl = largestTables.length === 0
            ? `<tr><td colspan="2" class="text-muted">No latest size rows in Timescale for largest tables.</td></tr>`
            : '';
        const emptyLargestIdx = largestIndexes.length === 0
            ? `<tr><td colspan="2" class="text-muted">No latest size rows in Timescale for largest indexes.</td></tr>`
            : '';

        const ku = Number(k.unused_index_count || 0);
        const kh = Number(k.high_scan_table_count || 0);
        const kfg = Number(k.fastest_growing_table_growth_7d_pct || 0);
        const ko = Number(k.index_write_overhead_pct || 0);
        const nUnused = unusedIndexes.length;
        const nHigh = highScanTables.length;
        const nDup = dupCandidates.length;

        const kpiStyleUnused = ku > 0 ? 'border-left:4px solid var(--danger,#dc2626);' : 'border-left:4px solid transparent;';
        const kpiStyleHigh = kh > 0 ? 'border-left:4px solid var(--warning,#f59e0b);' : 'border-left:4px solid transparent;';
        const kpiStyleGrow = kfg > 10 ? 'border-left:4px solid var(--warning,#f59e0b);' : 'border-left:4px solid transparent;';
        const kpiStyleOH = ko > 30 ? 'border-left:4px solid var(--warning,#f59e0b);' : 'border-left:4px solid transparent;';

        const badgeUnused = ku > 0
            ? '<span class="badge badge-danger" style="font-size:0.62rem;">Alert</span>'
            : '<span class="badge badge-outline" style="font-size:0.62rem;opacity:0.55;">OK</span>';
        const badgeHigh = kh > 0
            ? '<span class="badge badge-warning" style="font-size:0.62rem;">Review</span>'
            : '<span class="badge badge-outline" style="font-size:0.62rem;opacity:0.55;">OK</span>';
        const badgeGrow = kfg > 10
            ? '<span class="badge badge-warning" style="font-size:0.62rem;">10%+</span>'
            : '<span class="badge badge-outline" style="font-size:0.62rem;opacity:0.55;">OK</span>';
        const badgeOH = ko > 30
            ? '<span class="badge badge-warning" style="font-size:0.62rem;">30%+</span>'
            : '<span class="badge badge-outline" style="font-size:0.62rem;opacity:0.55;">OK</span>';

        try {
            window.currentCharts = window.currentCharts || {};
            if (window.currentCharts.sihTopScans) { window.currentCharts.sihTopScans.destroy(); delete window.currentCharts.sihTopScans; }
            if (window.currentCharts.sihSeekScanLookup) { window.currentCharts.sihSeekScanLookup.destroy(); delete window.currentCharts.sihSeekScanLookup; }
            if (window.currentCharts.sihGrowth) { window.currentCharts.sihGrowth.destroy(); delete window.currentCharts.sihGrowth; }
        } catch (e) { /* ignore */ }

        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title flex-between">
                    <div>
                        <h1><i class="fa-solid fa-boxes-stacked text-accent"></i> ${dashTitle}</h1>
                        <p class="subtitle">Engine: <span class="text-accent">${window.escapeHtml(engine)}</span> | Instance: <span class="text-accent">${window.escapeHtml(inst.name)}</span></p>
                    </div>
                    <div style="display:flex; gap:0.5rem; align-items:center; flex-wrap:wrap;">
                        <div class="glass-panel" style="padding:0.5rem 0.75rem;">
                            <div class="text-muted" style="font-size:0.75rem;">Instance</div>
                            <div style="font-weight:600;">${window.escapeHtml(inst.name)}</div>
                        </div>
                        <button class="btn btn-sm btn-outline" onclick="window.appNavigate('${backRoute}')"><i class="fa-solid fa-arrow-left"></i> Back</button>
                        <button class="btn btn-sm btn-outline text-accent" onclick="window.appNavigate(window.appState.activeViewId)"><i class="fa-solid fa-refresh"></i> Refresh</button>
                    </div>
                </div>
                ${dataHealthBanner}

                <div id="sihFilterBar" class="glass-panel mt-2" style="padding:0.75rem;">
                    <div class="text-muted" style="font-size:0.7rem; margin-bottom:0.5rem; line-height:1.35;">
                        Filters use distinct <strong>db_name</strong>, <strong>schema_name</strong>, and <strong>table_name</strong> from Timescale
                        <code>monitor.index_usage_stats</code>, <code>monitor.table_usage_stats</code>, and <code>monitor.table_size_history</code> (union) for this instance and time range.
                    </div>
                    <div style="display:flex; gap:0.75rem; flex-wrap:wrap; align-items:center;">
                        <div>
                            <div class="text-muted" style="font-size:0.75rem;">Time range</div>
                            <select id="sihRange" class="custom-select">
                                <option value="1h" ${state.timeRange==='1h'?'selected':''}>1h</option>
                                <option value="24h" ${state.timeRange==='24h'?'selected':''}>24h</option>
                                <option value="7d" ${state.timeRange==='7d'?'selected':''}>7d</option>
                                <option value="30d" ${state.timeRange==='30d'?'selected':''}>30d</option>
                            </select>
                        </div>
                        <div style="min-width:220px;">
                            <div class="text-muted" style="font-size:0.75rem;">Database</div>
                            <select id="sihDb" class="custom-select">
                                <option value="all">All</option>
                                ${dbs.map(d => `<option value="${window.escapeHtml(d)}" ${state.db===d?'selected':''}>${window.escapeHtml(d)}</option>`).join('')}
                            </select>
                        </div>
                        <div style="min-width:220px;">
                            <div class="text-muted" style="font-size:0.75rem;">Schema</div>
                            <select id="sihSchema" class="custom-select">
                                <option value="all">All</option>
                                ${schemas.map(s => `<option value="${window.escapeHtml(s)}" ${state.schema===s?'selected':''}>${window.escapeHtml(s)}</option>`).join('')}
                            </select>
                        </div>
                        <div style="min-width:260px;">
                            <div class="text-muted" style="font-size:0.75rem;">Table</div>
                            <select id="sihTable" class="custom-select">
                                <option value="all">All</option>
                                ${tables.map(t => `<option value="${window.escapeHtml(t)}" ${state.table===t?'selected':''}>${window.escapeHtml(t)}</option>`).join('')}
                            </select>
                        </div>
                        <button class="btn btn-sm btn-accent" id="sihApply"><i class="fa-solid fa-filter"></i> Apply</button>
                        <button class="btn btn-sm btn-outline" id="sihRefresh"><i class="fa-solid fa-refresh"></i> Refresh</button>
                    </div>
                </div>
                <div class="glass-panel mt-2" style="padding:0.65rem 0.85rem; border-left:3px solid var(--accent-blue, #3b82f6);">
                    <div style="font-size:0.8rem; color:var(--text-muted); line-height:1.45;">
                        <strong style="color:var(--text);">When does data show up?</strong>
                        Index usage deltas are written on about a <strong>15 minute</strong> collector tick (table usage on the same cadence; growth snapshots about every <strong>6 hours</strong>; index-definition / daily unused analysis about once per <strong>24 hours</strong>).
                        ${engine === 'postgres'
                            ? ` Right after deploy, empty charts are normal until the first tick. If it stays empty after 15+ minutes, verify Timescale is connected, the historical collector is running, and the Postgres connection can read <code style="font-size:0.72rem;">pg_stat_user_*</code> catalogs. Check logs for <code style="font-size:0.72rem;">[Collector][SIH]</code>.`
                            : ` Right after deploy, empty charts are normal until the first tick. If it stays empty after 15+ minutes, verify Timescale is connected, the historical collector is running, and the SQL login can access at least one user database (or set <code style="font-size:0.72rem;">Instances[].databases</code> explicitly). Check logs for <code style="font-size:0.72rem;">[Collector][SIH]</code> (auto-discover, USE failures, or persist errors).`}
                    </div>
                </div>

                <div class="charts-grid mt-3" style="display:grid; grid-template-columns:repeat(4, 1fr); gap:0.75rem;">
                    <div class="glass-panel" style="padding:0.75rem; ${kpiStyleUnused}">
                        <div class="flex-between" style="align-items:flex-start; gap:0.35rem; margin-bottom:0.25rem;">
                            <div class="text-muted" style="font-size:0.75rem;">Unused indexes (writes but 0 reads)</div>
                            ${badgeUnused}
                        </div>
                        <div style="font-size:1.6rem; font-weight:700;" class="${ku > 0 ? 'text-danger' : ''}">${fmt(k.unused_index_count)}</div>
                    </div>
                    <div class="glass-panel" style="padding:0.75rem; ${kpiStyleHigh}">
                        <div class="flex-between" style="align-items:flex-start; gap:0.35rem; margin-bottom:0.25rem;">
                            <div class="text-muted" style="font-size:0.75rem;">High scan tables</div>
                            ${badgeHigh}
                        </div>
                        <div style="font-size:1.6rem; font-weight:700;" class="${kh > 0 ? 'text-warning' : ''}">${fmt(k.high_scan_table_count)}</div>
                    </div>
                    <div class="glass-panel" style="padding:0.75rem; ${kpiStyleGrow}">
                        <div class="flex-between" style="align-items:flex-start; gap:0.35rem; margin-bottom:0.25rem;">
                            <div class="text-muted" style="font-size:0.75rem;">Fastest growing table (7d)</div>
                            ${badgeGrow}
                        </div>
                        <div style="font-size:0.95rem; font-weight:700; word-break:break-word;" class="${kfg > 10 ? 'text-warning' : ''}">${window.escapeHtml(k.fastest_growing_table || '--')}</div>
                        ${(k.fastest_growing_table && String(k.fastest_growing_table).trim() !== '') ? `<div class="text-muted" style="font-size:0.68rem; margin-top:0.2rem;">${fmt(kfg, 1)}% vs 7d start</div>` : ''}
                    </div>
                    <div class="glass-panel" style="padding:0.75rem; ${kpiStyleOH}">
                        <div class="flex-between" style="align-items:flex-start; gap:0.35rem; margin-bottom:0.25rem;">
                            <div class="text-muted" style="font-size:0.75rem;">Index write overhead %</div>
                            ${badgeOH}
                        </div>
                        <div style="font-size:1.6rem; font-weight:700;" class="${ko > 30 ? 'text-warning' : ''}">${fmt(k.index_write_overhead_pct, 1)}%</div>
                    </div>
                </div>

                <div class="mt-3">
                    <h2 style="font-size:1.05rem; font-weight:700; margin:0 0 0.65rem 0; color:var(--text); letter-spacing:-0.02em;">Immediate Optimization Opportunities</h2>
                    <div class="sih-action-grids" style="display:grid; grid-template-columns:repeat(3, minmax(0, 1fr)); gap:0.75rem; align-items:stretch;">
                        <div class="glass-panel sih-action-card" style="padding:0.65rem 0.75rem; display:flex; flex-direction:column; min-height:280px; max-height:280px;">
                            <h3 style="margin:0 0 0.4rem 0; font-size:0.88rem;">${engine === 'sqlserver' ? 'Unused Nonclustered Indexes' : 'Unused Indexes'} (${nUnused})</h3>
                            <div style="flex:1; min-height:0; overflow:auto; border-radius:6px;">
                                <table class="data-table" style="font-size:0.68rem;">
                                    <thead>
                                        <tr>
                                            <th>DB</th><th>Schema</th><th>Table</th><th>Index</th>
                                            <th class="text-right">${engine==='postgres'?'Wr Δ':'Upd Δ'}</th>
                                            <th class="text-right">MB</th><th>${engine === 'postgres' ? 'Last seek/scan' : 'Last seek'}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        ${unusedIndexes.map(r => `
                                            <tr>
                                                <td>${window.escapeHtml(r.db_name)}</td>
                                                <td>${window.escapeHtml(r.schema_name)}</td>
                                                <td>${window.escapeHtml(r.table_name)}</td>
                                                <td>${window.escapeHtml(r.index_name || '')}</td>
                                                <td class="text-right">${fmt(r.value, 0)}</td>
                                                <td class="text-right">${fmt(r.value2, 1)}</td>
                                                <td>${r.last_user_seek ? window.escapeHtml(new Date(r.last_user_seek).toLocaleDateString()) : '—'}</td>
                                            </tr>
                                        `).join('')}
                                        ${unusedIndexes.length===0 ? `<tr><td colspan="7" class="text-muted">No rows in window.</td></tr>` : ``}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                        <div class="glass-panel sih-action-card" style="padding:0.65rem 0.75rem; display:flex; flex-direction:column; min-height:280px; max-height:280px;">
                            <h3 style="margin:0 0 0.4rem 0; font-size:0.88rem;">High Scan Tables (${nHigh})</h3>
                            <div style="flex:1; min-height:0; overflow:auto; border-radius:6px;">
                                <table class="data-table" style="font-size:0.68rem;">
                                    <thead>
                                        <tr>
                                            <th>DB</th><th>Schema</th><th>Table</th>
                                            <th class="text-right">${engine==='postgres'?'Seq':'Scan'}</th>
                                            <th class="text-right">${engine==='postgres'?'Idx':'Seek'}</th>
                                            <th class="text-right">Ratio</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        ${highScanTables.map(r => {
                                            const scans = Number(r.value||0);
                                            const seeks = Number(r.value2||0);
                                            const ratio = seeks > 0 ? (scans / seeks) : (scans > 0 ? Infinity : 0);
                                            return `
                                                <tr>
                                                    <td>${window.escapeHtml(r.db_name)}</td>
                                                    <td>${window.escapeHtml(r.schema_name)}</td>
                                                    <td>${window.escapeHtml(r.table_name)}</td>
                                                    <td class="text-right">${fmt(scans, 0)}</td>
                                                    <td class="text-right">${fmt(seeks, 0)}</td>
                                                    <td class="text-right">${(ratio === Infinity) ? '∞' : fmt(ratio, 2)}</td>
                                                </tr>
                                            `;
                                        }).join('')}
                                        ${highScanTables.length===0 ? `<tr><td colspan="6" class="text-muted">No rows in window.</td></tr>` : ``}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                        <div class="glass-panel sih-action-card" style="padding:0.65rem 0.75rem; display:flex; flex-direction:column; min-height:280px; max-height:280px;">
                            <h3 style="margin:0 0 0.35rem 0; font-size:0.88rem;">Duplicate Index Candidates (${nDup})</h3>
                            <p class="text-muted" style="font-size:0.65rem; margin:0 0 0.35rem 0; line-height:1.35;">Validate with workload before dropping.</p>
                            <div style="flex:1; min-height:0; overflow:auto; border-radius:6px;">
                                <table class="data-table" style="font-size:0.68rem;">
                                    <thead>
                                        <tr>
                                            <th>DB</th><th>Schema</th><th>Table</th>
                                            <th>Idx A</th><th>Idx B</th>
                                            <th class="text-right">%</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        ${dupCandidates.map(r => `
                                            <tr>
                                                <td>${window.escapeHtml(r.db_name || '')}</td>
                                                <td>${window.escapeHtml(r.schema_name || '')}</td>
                                                <td>${window.escapeHtml(r.table_name || '')}</td>
                                                <td>${window.escapeHtml(r.index1 || '')}</td>
                                                <td>${window.escapeHtml(r.index2 || '')}</td>
                                                <td class="text-right">${fmt(r.key_match_pct, 1)}</td>
                                            </tr>
                                        `).join('')}
                                        ${dupCandidates.length===0 ? `<tr><td colspan="6" class="text-muted">No candidates in scope (requires daily index-definition snapshot).</td></tr>` : ``}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                </div>
                <style>
                    @media (max-width: 1200px) {
                        .sih-action-grids { grid-template-columns: 1fr !important; }
                        .sih-action-card { max-height: none !important; min-height: 240px !important; }
                    }
                </style>
                ${emptyDiagCharts}

                <h2 style="font-size:1.05rem; font-weight:700; margin:1rem 0 0.5rem 0; color:var(--text); letter-spacing:-0.02em;">Diagnostic Charts</h2>
                <div class="charts-grid mt-1" style="display:grid; grid-template-columns:2fr 1fr; gap:0.75rem;">
                    <div class="glass-panel" style="padding:0.75rem;">
                        <div class="flex-between" style="align-items:center;">
                            <h3 style="margin:0; font-size:0.95rem;">${engine === 'sqlserver' ? 'Top Tables by Range Scans' : 'Top Tables by Scans'}</h3>
                            <span class="text-muted" style="font-size:0.75rem;">${window.escapeHtml(state.timeRange)}</span>
                        </div>
                        <div class="chart-container" style="height:220px;"><canvas id="sihTopScansChart"></canvas></div>
                    </div>
                    <div class="glass-panel" style="padding:0.75rem;">
                        <div class="flex-between" style="align-items:center;">
                            <h3 style="margin:0; font-size:0.95rem;">${engine === 'postgres' ? 'Idx vs sequential scans (tables)' : 'Seek vs Scan vs Lookup'}</h3>
                            <span class="text-muted" style="font-size:0.75rem;">stacked</span>
                        </div>
                        <div class="chart-container" style="height:220px;"><canvas id="sihSeekScanLookupChart"></canvas></div>
                    </div>
                </div>

                <h2 style="font-size:1.05rem; font-weight:700; margin:1rem 0 0.5rem 0; color:var(--text); letter-spacing:-0.02em;">Storage distribution</h2>
                <div class="charts-grid mt-1" style="display:grid; grid-template-columns:1fr 1fr; gap:0.75rem;">
                    <div class="glass-panel" style="padding:0.75rem;">
                        <h3 style="margin:0 0 0.5rem 0; font-size:0.95rem;">Largest Tables (MB)</h3>
                        <div style="overflow:auto; max-height:280px;">
                            <table class="data-table">
                                <thead><tr><th>Table</th><th class="text-right">MB</th></tr></thead>
                                <tbody>
                                    ${largestTables.length ? largestTables.map(r => `
                                        <tr>
                                            <td>${window.escapeHtml(`${r.db_name}.${r.schema_name}.${r.table_name}`)}</td>
                                            <td class="text-right">${fmt(r.value, 1)}</td>
                                        </tr>
                                    `).join('') : emptyLargestTbl}
                                </tbody>
                            </table>
                        </div>
                    </div>
                    <div class="glass-panel" style="padding:0.75rem;">
                        <h3 style="margin:0 0 0.5rem 0; font-size:0.95rem;">Largest Indexes (MB)</h3>
                        <div style="overflow:auto; max-height:280px;">
                            <table class="data-table">
                                <thead><tr><th>Index</th><th class="text-right">MB</th></tr></thead>
                                <tbody>
                                    ${largestIndexes.length ? largestIndexes.map(r => `
                                        <tr>
                                            <td>${window.escapeHtml(`${r.db_name}.${r.schema_name}.${r.table_name}.${r.index_name}`)}</td>
                                            <td class="text-right">${fmt(r.value, 1)}</td>
                                        </tr>
                                    `).join('') : emptyLargestIdx}
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>

                <h2 style="font-size:1.05rem; font-weight:700; margin:1rem 0 0.5rem 0; color:var(--text); letter-spacing:-0.02em;">Growth trend</h2>
                <div class="charts-grid mt-1" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                    <div class="glass-panel" style="padding:0.75rem;">
                        <div class="flex-between" style="align-items:center;">
                            <h3 style="margin:0; font-size:0.95rem;">Growth Trend (MB)</h3>
                            <span class="text-muted" style="font-size:0.75rem;">daily buckets</span>
                        </div>
                        <div class="chart-container" style="height:220px;"><canvas id="sihGrowthChart"></canvas></div>
                        ${emptyGrowthNote}
                        <div class="text-muted" style="font-size:0.8rem; margin-top:0.5rem; display:flex; gap:1rem; flex-wrap:wrap;">
                            <div><strong>Daily growth</strong>: ${fmt(growthSummary.daily_growth_mb, 1)} MB</div>
                            <div><strong>7d</strong>: ${fmt(growthSummary.growth_7d_pct, 1)}%</div>
                            <div><strong>30d</strong>: ${fmt(growthSummary.growth_30d_pct, 1)}%</div>
                            <div><strong>90d projected</strong>: ${fmt(growthSummary.projected_table_mb_90d, 1)} MB table, ${fmt(growthSummary.projected_index_mb_90d, 1)} MB index</div>
                        </div>
                    </div>
                </div>
            </div>
        `;

        applyHandlers();

        // Charts
        setTimeout(() => {
            window.currentCharts = window.currentCharts || {};
            try {
                const labels = topScans.map(r => `${r.schema_name}.${r.table_name}`);
                const scans = topScans.map(r => Number(r.value || 0));
                const seeks = topScans.map(r => Number(r.value2 || 0));
                const ctx1 = document.getElementById('sihTopScansChart')?.getContext('2d');
                if (ctx1) {
                    if (window.currentCharts.sihTopScans) window.currentCharts.sihTopScans.destroy();
                    window.currentCharts.sihTopScans = new Chart(ctx1, {
                        type: 'bar',
                        data: {
                            labels,
                            datasets: [
                                { label: (engine === 'postgres' ? 'Seq scans (delta)' : (engine === 'sqlserver' ? 'Range scans (delta)' : 'Scans (delta)')), data: scans, backgroundColor: window.getCSSVar('--danger') || 'rgba(239,68,68,0.8)' },
                                ...((engine === 'sqlserver' || engine === 'postgres') ? [{ label: (engine === 'postgres' ? 'Index scans (delta)' : 'Index seeks (delta)'), data: seeks, backgroundColor: window.getCSSVar('--success') || 'rgba(34,197,94,0.8)' }] : [])
                            ]
                        },
                        options: { responsive: true, maintainAspectRatio: false, scales: { x: { stacked: true }, y: { stacked: true } } }
                    });
                }

                const ctxMix = document.getElementById('sihSeekScanLookupChart')?.getContext('2d');
                if (ctxMix) {
                    if (window.currentCharts.sihSeekScanLookup) window.currentCharts.sihSeekScanLookup.destroy();
                    const mLabels = seekScanLookup.map(r => `${r.schema_name}.${r.table_name}`);
                    const mSeeks = seekScanLookup.map(r => Number(r.seeks || 0));
                    const mScans = seekScanLookup.map(r => Number(r.scans || 0));
                    const mLookups = seekScanLookup.map(r => Number(r.lookups || 0));
                    const mixDatasets = engine === 'postgres'
                        ? [
                            { label: 'Index scans (delta)', data: mSeeks, backgroundColor: window.getCSSVar('--success') || 'rgba(34,197,94,0.8)' },
                            { label: 'Seq scans (delta)', data: mScans, backgroundColor: window.getCSSVar('--danger') || 'rgba(239,68,68,0.8)' }
                        ]
                        : [
                            { label: 'Seeks (delta)', data: mSeeks, backgroundColor: window.getCSSVar('--success') || 'rgba(34,197,94,0.8)' },
                            { label: 'Scans (delta)', data: mScans, backgroundColor: window.getCSSVar('--danger') || 'rgba(239,68,68,0.8)' },
                            { label: 'Lookups (delta)', data: mLookups, backgroundColor: window.getCSSVar('--warning') || 'rgba(245,158,11,0.8)' }
                        ];
                    window.currentCharts.sihSeekScanLookup = new Chart(ctxMix, {
                        type: 'bar',
                        data: {
                            labels: mLabels,
                            datasets: mixDatasets
                        },
                        options: { responsive: true, maintainAspectRatio: false, scales: { x: { stacked: true }, y: { stacked: true } } }
                    });
                }

                const gLabels = growth.map(p => {
                    const d = new Date(p.bucket);
                    return isNaN(d.getTime()) ? '' : d.toLocaleDateString();
                });
                const gTable = growth.map(p => Number(p.table_size_mb || 0));
                const gIndex = growth.map(p => Number(p.index_size_mb || 0));
                const ctx2 = document.getElementById('sihGrowthChart')?.getContext('2d');
                if (ctx2) {
                    if (window.currentCharts.sihGrowth) window.currentCharts.sihGrowth.destroy();
                    window.currentCharts.sihGrowth = new Chart(ctx2, {
                        type: 'line',
                        data: {
                            labels: gLabels,
                            datasets: [
                                { label: 'Table MB', data: gTable, borderColor: window.getCSSVar('--accent-blue') || '#60a5fa', tension: 0.2 },
                                { label: 'Index MB', data: gIndex, borderColor: window.getCSSVar('--warning') || '#f59e0b', tension: 0.2 }
                            ]
                        },
                        options: { responsive: true, maintainAspectRatio: false }
                    });
                }
            } catch (e) {}
        }, 60);
    } catch (e) {
        const msg = (e && e.message) ? String(e.message) : String(e);
        window.routerOutlet.innerHTML = `
            <div class="page-view active">
                <div class="alert alert-warning">
                    <h3>${dashTitle} unavailable</h3>
                    <div class="text-muted">Error: ${window.escapeHtml(msg)}</div>
                    <div style="margin-top:0.75rem;">
                        <button class="btn btn-primary" onclick="window.appNavigate('global')"><i class="fa-solid fa-home"></i> Home</button>
                        <button class="btn btn-outline" onclick="window.appNavigate(window.appState.activeViewId)"><i class="fa-solid fa-refresh"></i> Retry</button>
                    </div>
                </div>
            </div>
        `;
    }
};

