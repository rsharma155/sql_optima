/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Watched Query Analyzer — Detailed performance analysis,
 *          trends, and event markers for specific high-value queries.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function () {
    const esc = (s) => window.escapeHtml ? window.escapeHtml(String(s ?? '')) : String(s ?? '');
    const fmtNum = (n) => (n == null || isNaN(n)) ? '--' : Number(n).toLocaleString(undefined, { maximumFractionDigits: 1 });
    const fmtMs = (n) => (n == null || isNaN(n)) ? '--' : `${Number(n).toLocaleString(undefined, { maximumFractionDigits: 1 })} ms`;

    let _charts = {};
    function destroyCharts() {
        Object.values(_charts).forEach(c => { try { c.destroy(); } catch (_) {} });
        _charts = {};
    }

    /* ── main view ──────────────────────────────────────────── */
    async function loadWatchedQueryAnalyzer() {
        destroyCharts();
        const outlet = window.routerOutlet;
        if (!outlet) return;
        const inst = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
        if (!inst || String(inst.type || '').toLowerCase() !== 'sqlserver') {
            outlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select a SQL Server instance.</h3></div>`;
            return;
        }
        const instName = inst.name;

        // Load modern template
        try {
            const html = await window.loadTemplate('pages/sqlserver_watched_queries.html');
            if (html.includes('Error loading view') || html.includes('Template Failed to Load')) {
                outlet.innerHTML = html;
                return;
            }
            outlet.innerHTML = html;
        } catch (err) {
            outlet.innerHTML = `<div class="alert alert-danger">Critical: Failed to load UI template. ${err.message}</div>`;
            return;
        }

        const refreshBtn = document.getElementById('wqRefresh');
        if (refreshBtn) {
            refreshBtn.onclick = () => fetchList();
        } else {
            console.warn('[WatchedQueries] wqRefresh element not found');
        }

        document.getElementById('wqCloseDetail')?.addEventListener('click', () => {
            document.getElementById('wqDetailPanel').style.display = 'none';
            destroyCharts();
        });

        // Tab Switching
        const tabs = document.querySelectorAll('[data-wq-tab]');
        tabs.forEach(btn => {
            btn.onclick = () => {
                tabs.forEach(t => t.classList.remove('active'));
                btn.classList.add('active');
                document.querySelectorAll('.wq-tab-panel').forEach(p => p.style.display = 'none');
                document.getElementById(`wq-tab-${btn.dataset.wqTab}`).style.display = 'block';
            };
        });

        /* ── list ─────────────────────────────────────────── */
        async function fetchList() {
            const body = document.getElementById('wqListBody');
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/watched-queries?instance=${encodeURIComponent(instName)}`, { cache: 'no-store' });
                if (!resp.ok) throw new Error('HTTP ' + resp.status);
                const data = await resp.json();
                const rows = data.watched_queries || [];
                
                document.getElementById('wqCount').textContent = `(${rows.length} / 10)`;
                if (!rows.length) {
                    body.innerHTML = `<div class="text-muted p-4 text-center">No watched queries. Use the ⭐ button on the Query Analysis page to track a specific query.</div>`;
                    return;
                }
                body.innerHTML = renderList(rows);
                
                body.querySelectorAll('.wq-delete-btn').forEach(btn => {
                    btn.addEventListener('click', async () => {
                        if (!confirm('Remove this watched query and all its history?')) return;
                        await window.apiClient.authenticatedFetch(`/api/sqlserver/watched-queries?instance=${encodeURIComponent(instName)}&id=${btn.dataset.id}`, { method: 'DELETE' });
                        fetchList();
                    });
                });
            } catch (err) {
                body.innerHTML = `<div class="text-danger p-4 text-center">Failed to load watched queries: ${err.message}</div>`;
            }
        }

        function renderList(rows) {
            return `<div class="table-responsive"><table class="qa-table">
                <thead><tr>
                    <th><i class="fa-solid fa-code text-muted mr-1"></i> Query</th>
                    <th><i class="fa-solid fa-database text-muted mr-1"></i> Database</th>
                    <th><i class="fa-solid fa-clock-rotate-left text-muted mr-1"></i> Last Executed</th>
                    <th><i class="fa-solid fa-calendar-plus text-muted mr-1"></i> Added On</th>
                    <th class="text-right">Actions</th>
                </tr></thead>
                <tbody>${rows.map(r => `<tr>
                    <td style="font-weight:600; color:var(--accent); min-width:300px;">${esc(r.name || r.query_hash)}</td>
                    <td><span class="qa-badge qa-badge--accent">${esc(r.database_name || 'master')}</span></td>
                    <td class="text-muted" style="font-size:0.75rem; font-weight:500;">
                        ${r.last_executed_at && r.last_executed_at !== '0001-01-01T00:00:00Z' 
                            ? `<i class="fa-regular fa-clock opacity-50 mr-1"></i> ${new Date(r.last_executed_at).toLocaleString([], {year:'numeric', month:'numeric', day:'numeric', hour:'2-digit', minute:'2-digit'})}` 
                            : '<span class="opacity-50">Never</span>'}
                    </td>
                    <td class="text-muted" style="font-size:0.7rem;">${r.created_at ? new Date(r.created_at).toLocaleString([], {year:'numeric', month:'numeric', day:'numeric', hour:'2-digit', minute:'2-digit'}) : '--'}</td>
                    <td class="text-right">
                        <button class="btn btn-xs btn-outline wq-detail-btn" data-id="${r.id}" data-name="${esc(r.name)}" data-hash="${esc(r.query_hash)}" data-db="${esc(r.database_name)}"><i class="fa-solid fa-chart-line"></i> Analyze</button>
                        <button class="btn btn-xs btn-outline text-danger wq-delete-btn ml-1" data-id="${r.id}" title="Remove from watch list"><i class="fa-solid fa-trash-can"></i></button>
                    </td>
                </tr>`).join('')}</tbody></table></div>`;
        }

        /* ── detail ───────────────────────────────────────── */
        async function openDetail(id, queryHash, dbName, queryName) {
            const panel = document.getElementById('wqDetailPanel');
            panel.style.display = '';
            panel.scrollIntoView({ behavior: 'smooth' });

            const displayName = queryName && queryName !== queryHash ? queryName : `Query #${id}`;
            document.getElementById('wqDetailTitle').innerHTML = `<i class="fa-solid fa-chart-area text-accent"></i> Analytical Drill-down: <span style="font-size:1.1rem; color:var(--text);">${esc(displayName)}</span>`;

            await Promise.all([
                fetchSnapshots(id),
                fetchPlans(queryHash, dbName),
                fetchWaits(queryHash, dbName)
            ]);
        }

        // Re-bind click for openDetail with name
        document.getElementById('wqListBody').onclick = (e) => {
            const btn = e.target.closest('.wq-detail-btn');
            if (btn) openDetail(btn.dataset.id, btn.dataset.hash, btn.dataset.db, btn.dataset.name);
        };

        async function fetchSnapshots(id) {
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/watched-queries/detail?instance=${encodeURIComponent(instName)}&id=${id}`, { cache: 'no-store' });
                const data = await resp.json();
                
                const meta = document.getElementById('wqDetailMeta');
                const wq = data.watched_query;
                if (wq) {
                    meta.innerHTML = `
                        <div class="wq-detail-meta-item"><span class="wq-detail-meta-label"><i class="fa-solid fa-code"></i> Query:</span> ${esc(wq.name || '--')}</div>
                        <div class="wq-detail-meta-item"><span class="wq-detail-meta-label"><i class="fa-solid fa-database"></i> Database:</span> <span class="qa-badge qa-badge--accent" style="border:none; background:rgba(56,189,248,0.1);">${esc(wq.database_name || 'master')}</span></div>
                        <div class="wq-detail-meta-item"><span class="wq-detail-meta-label"><i class="fa-solid fa-hashtag"></i> Hash:</span> <span class="text-mono" style="opacity:0.6; font-size:0.7rem;">${wq.query_hash}</span></div>
                        <div class="wq-detail-meta-item"><span class="wq-detail-meta-label"><i class="fa-solid fa-calendar-check"></i> Tracking Since:</span> ${wq.created_at ? new Date(wq.created_at).toLocaleString() : '--'}</div>
                    `;
                    if (wq.query_text) {
                        document.getElementById('wqSqlText').textContent = wq.query_text;
                    } else {
                        document.getElementById('wqSqlText').innerHTML = '<span class="text-muted italic">No full SQL text available for this query.</span>';
                    }
                }
                
                renderSnapshotChart(data.snapshots || [], data.events || []);
            } catch (err) {
                console.error('wq snapshots failed', err);
            }
        }

        function renderSnapshotChart(snapshots, events) {
            destroyCharts();
            const ctx = document.getElementById('wqSnapshotChart')?.getContext('2d');
            if (!ctx) return;
            
            if (!snapshots.length) {
                ctx.font = '14px Inter'; ctx.fillStyle = '#94a3b8'; ctx.textAlign = 'center';
                ctx.fillText('No performance history collected for this query yet.', ctx.canvas.width/2, ctx.canvas.height/2);
                return;
            }

            const labels = snapshots.map(s => new Date(s.snapshot_time).toLocaleString());
            
            _charts.snapshot = new Chart(ctx, {
                type: 'line',
                data: {
                    labels,
                    datasets: [
                        { label: 'Avg Duration (ms)', data: snapshots.map(s => s.avg_duration_ms), borderColor: '#38bdf8', backgroundColor: 'rgba(56,189,248,0.1)', tension: 0.3, fill: true, pointRadius: 2, borderWidth: 2 },
                        { label: 'Avg CPU (ms)', data: snapshots.map(s => s.avg_cpu_ms), borderColor: '#f43f5e', tension: 0.3, fill: false, pointRadius: 2, borderWidth: 2 },
                        { label: 'Avg Reads', data: snapshots.map(s => s.avg_reads), borderColor: '#10b981', tension: 0.3, fill: false, pointRadius: 2, yAxisID: 'y1', borderWidth: 2 }
                    ]
                },
                options: {
                    responsive: true, maintainAspectRatio: false,
                    interaction: { mode: 'index', intersect: false },
                    plugins: {
                        legend: { position: 'top', align: 'end', labels: { boxWidth: 12, usePointStyle: true, font: { size: 11 }, color: 'rgba(255,255,255,0.7)' } }
                    },
                    scales: {
                        x: { ticks: { maxRotation: 0, autoSkip: true, maxTicksLimit: 10, font: { size: 10 }, color: 'rgba(255,255,255,0.5)' }, grid: { display: false } },
                        y: { beginAtZero: true, ticks: { font: { size: 10 }, color: 'rgba(255,255,255,0.5)' }, grid: { color: 'rgba(255,255,255,0.05)' } },
                        y1: { position: 'right', beginAtZero: true, grid: { display: false }, ticks: { font: { size: 10 }, color: 'rgba(255,255,255,0.5)' } }
                    }
                }
            });
        }

        async function fetchPlans(queryHash, dbName) {
            const body = document.getElementById('wqPlansBody');
            body.innerHTML = '<div class="text-muted p-2">Loading plans...</div>';
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/query-analysis/query-plans?instance=${encodeURIComponent(instName)}&database=${encodeURIComponent(dbName||'master')}&query_hash=${encodeURIComponent(queryHash)}`, { cache: 'no-store' });
                const data = await resp.json();
                const rows = data.plans || [];
                if (!rows || !rows.length) { body.innerHTML = '<div class="text-muted p-2">No active plans in Query Store.</div>'; return; }
                
                // Store plans globally for the modal
                window._wqCurrentPlans = rows;

                body.innerHTML = `<div class="table-responsive"><table class="qa-table wq-snapshot-table">
                    <thead><tr><th>Plan ID</th><th>State</th><th>Created</th><th class="text-right">Avg Duration</th><th class="text-right">Executions</th><th class="text-center">XML</th></tr></thead>
                    <tbody>${rows.map((r, i) => `<tr>
                        <td class="font-bold">${r.plan_id}</td>
                        <td>${r.is_forced_plan ? '<span class="text-warning"><i class="fa-solid fa-lock"></i> Forced</span>' : '<span class="text-muted">Standard</span>'}</td>
                        <td>${r.created_at ? new Date(r.created_at).toLocaleString() : '--'}</td>
                        <td class="text-right font-mono">${fmtMs(r.avg_duration_ms)}</td>
                        <td class="text-right">${fmtNum(r.executions)}</td>
                        <td class="text-center">
                            <button class="btn btn-xs btn-outline" data-action="call" data-fn="_showPlanXml" data-idx="${i}"><i class="fa-solid fa-file-code"></i> View</button>
                        </td>
                    </tr>`).join('')}</tbody></table></div>`;
            } catch (err) { body.innerHTML = '<div class="text-danger p-2">Failed to load plan history.</div>'; }
        }

        window._showPlanXml = (idx) => {
            const plan = window._wqCurrentPlans[idx];
            if (!plan) return;
            
            const existing = document.getElementById('wq-xml-modal'); if(existing) existing.remove();
            const modal = document.createElement('div');
            modal.id = 'wq-xml-modal';
            modal.style.cssText = 'position:fixed; inset:0; z-index:100000; background:rgba(0,0,0,0.8); display:flex; align-items:center; justify-content:center; padding:2rem;';
            modal.innerHTML = `
                <div class="glass-panel" style="width:100%; max-width:900px; height:80vh; display:flex; flex-direction:column; background:var(--bg-surface);">
                    <div class="flex-between p-3" style="border-bottom:1px solid var(--border-color);">
                        <h4 style="margin:0;"><i class="fa-solid fa-file-code text-accent"></i> Execution Plan XML (ID: ${plan.plan_id})</h4>
                        <div class="flex-row gap-2">
                            <button class="btn btn-xs btn-primary" id="wqCopyXml"><i class="fa-solid fa-copy"></i> Copy XML</button>
                            <button class="btn btn-xs btn-outline" data-action="close-id" data-target="wq-xml-modal"><i class="fa-solid fa-times"></i></button>
                        </div>
                    </div>
                    <div style="flex:1; overflow:auto; padding:1rem;">
                        <pre class="code-block" style="font-size:0.7rem; margin:0; height:100%;">${esc(plan.query_plan)}</pre>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);
            document.getElementById('wqCopyXml').onclick = () => {
                navigator.clipboard.writeText(plan.query_plan);
                alert('Plan XML copied to clipboard.');
            };
        };

        async function fetchWaits(queryHash, dbName) {
            const body = document.getElementById('wqWaitsBody');
            body.innerHTML = '<div class="text-muted p-2">Loading wait statistics...</div>';
            try {
                const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/query-analysis/query-wait-stats?instance=${encodeURIComponent(instName)}&database=${encodeURIComponent(dbName||'master')}&query_hash=${encodeURIComponent(queryHash)}`, { cache: 'no-store' });
                const data = await resp.json();
                const rows = data.wait_stats || [];
                if (!rows || !rows.length) { body.innerHTML = '<div class="text-muted p-2">No wait stats recorded in Query Store for this hash.</div>'; return; }
                body.innerHTML = `<div class="table-responsive"><table class="qa-table wq-snapshot-table">
                    <thead><tr><th>Wait Category</th><th class="text-right">Total Wait</th><th class="text-right">Avg Wait</th></tr></thead>
                    <tbody>${rows.map(r => `<tr>
                        <td class="font-bold">${esc(r.wait_category)}</td>
                        <td class="text-right font-mono">${fmtMs(r.total_wait_ms)}</td>
                        <td class="text-right font-mono">${fmtMs(r.avg_wait_ms)}</td>
                    </tr>`).join('')}</tbody></table></div>`;
            } catch (err) { body.innerHTML = '<div class="text-danger p-2">Failed to load wait stats.</div>'; }
        }

        /* initial load */
        await fetchList();
    }

    window.sqlserver_WatchedQueryAnalyzer = loadWatchedQueryAnalyzer;
})();
