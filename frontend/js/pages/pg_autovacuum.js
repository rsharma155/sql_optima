/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL autovacuum, bloat, idle-in-transaction, and XID wraparound risk dashboard.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgAutovacuumView = async function PgAutovacuumView() {
    const inst = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
    const instanceName = inst?.name;
    if (!instanceName || inst?.type !== 'postgres') {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select a PostgreSQL instance first</h3></div>`;
        return;
    }

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
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-broom text-accent"></i> Autovacuum &amp; Bloat Risk</h1>
                    <p class="subtitle">Instance: ${esc(instanceName)}</p>
                </div>
                <div>
                    <button id="pgAvRefreshBtn" class="btn btn-sm btn-outline text-accent"><i class="fa-solid fa-rotate-right"></i> Refresh</button>
                </div>
            </div>

            <!-- Tab Nav -->
            <div class="tabs-container">
                <button class="tab-btn active" data-tab="bloat"><i class="fa-solid fa-database" style="margin-right:.35rem;opacity:.75;"></i>Table Bloat<span class="tab-badge" id="tabBadge-bloat">…</span></button>
                <button class="tab-btn" data-tab="idle"><i class="fa-solid fa-hourglass-half" style="margin-right:.35rem;opacity:.75;"></i>Idle In Transaction<span class="tab-badge" id="tabBadge-idle">…</span></button>
                <button class="tab-btn" data-tab="xid"><i class="fa-solid fa-rotate" style="margin-right:.35rem;opacity:.75;"></i>XID Wraparound Risk</button>
                <button class="tab-btn" data-tab="longtxn"><i class="fa-solid fa-clock" style="margin-right:.35rem;opacity:.75;"></i>Long-Running Transactions<span class="tab-badge" id="tabBadge-longtxn">…</span></button>
                <button class="tab-btn" data-tab="idxbloat"><i class="fa-solid fa-layer-group" style="margin-right:.35rem;opacity:.75;"></i>Index Bloat<span class="tab-badge" id="tabBadge-idxbloat">…</span></button>
            </div>

            <!-- Bloat Tab -->
            <div id="pgAvTab-bloat" class="tab-panel">
                <div class="table-card glass-panel" style="margin-bottom:1rem;">
                    <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
                        <h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-database text-warning"></i> Table Bloat Estimates</h3>
                        <span id="pgBloatMeta" class="text-muted small">Loading…</span>
                    </div>
                    <div class="table-responsive" style="overflow-x:auto;">
                        <table class="data-table" style="font-size:0.82rem;">
                            <thead>
                                <tr>
                                    <th>Schema</th><th>Table</th><th>Total Size</th>
                                    <th>Live Tuples</th><th>Dead Tuples</th><th>Dead %</th>
                                    <th>Est. Waste</th><th>Seq Scans</th>
                                    <th>Last Autovacuum</th><th>Vacuum Lag</th><th>Last Vacuum</th>
                                    <th>Freeze Age (M XIDs)</th><th>Recommendation</th>
                                </tr>
                            </thead>
                            <tbody id="pgBloatTbody">
                                <tr><td colspan="13" class="text-center text-muted">Loading…</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- Idle In Transaction Tab -->
            <div id="pgAvTab-idle" class="tab-panel" style="display:none;">
                <div id="pgIdleAlert" style="margin-bottom:0.75rem;"></div>
                <div class="table-card glass-panel">
                    <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
                        <h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-hourglass-half text-danger"></i> Idle In Transaction Sessions</h3>
                        <span id="pgIdleMeta" class="text-muted small">Loading…</span>
                    </div>
                    <div class="table-responsive" style="overflow-x:auto;">
                        <table class="data-table" style="font-size:0.82rem;">
                            <thead>
                                <tr>
                                    <th>PID</th><th>User</th><th>Database</th><th>State</th>
                                    <th>Idle Duration</th><th>Wait Event</th>
                                    <th>Client</th><th>Query Start</th><th>Last Query (truncated)</th>
                                </tr>
                            </thead>
                            <tbody id="pgIdleTbody">
                                <tr><td colspan="9" class="text-center text-muted">Loading…</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- XID Wraparound Tab -->
            <div id="pgAvTab-xid" class="tab-panel" style="display:none;">
                <div class="table-card glass-panel">
                    <div class="card-header">
                        <h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-triangle-exclamation text-warning"></i> XID Wraparound Risk by Database
                        <span class="text-muted" style="font-size:0.75rem;font-weight:normal;margin-left:.5rem;">(PostgreSQL wraparound at 2.1B transactions)</span></h3>
                    </div>
                    <div id="pgXIDBody" style="padding:0.5rem 0;">
                        <p class="text-muted text-center">Loading…</p>
                    </div>
                </div>
            </div>

            <!-- Long-Running Transactions Tab -->
            <div id="pgAvTab-longtxn" class="tab-panel" style="display:none;">
                <div id="pgLongTxnAlert" style="margin-bottom:0.75rem;"></div>
                <div class="table-card glass-panel">
                    <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
                        <h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-clock text-warning"></i> Long-Running Active Transactions (&gt;1 min)</h3>
                        <span id="pgLongTxnMeta" class="text-muted small">Loading…</span>
                    </div>
                    <div class="table-responsive" style="overflow-x:auto;">
                        <table class="data-table" style="font-size:0.82rem;">
                            <thead>
                                <tr>
                                    <th>PID</th><th>User</th><th>Database</th><th>State</th>
                                    <th>Txn Duration</th><th>Wait Event</th>
                                    <th>Client</th><th>Txn Start</th><th>Query (truncated)</th>
                                </tr>
                            </thead>
                            <tbody id="pgLongTxnTbody">
                                <tr><td colspan="9" class="text-center text-muted">Loading…</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- Index Bloat Tab -->
            <div id="pgAvTab-idxbloat" class="tab-panel" style="display:none;">
                <div class="table-card glass-panel">
                    <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
                        <h3 style="font-size:0.85rem;margin:0;"><i class="fa-solid fa-sitemap text-info"></i> Index Bloat &amp; Usage</h3>
                        <span id="pgIdxBloatMeta" class="text-muted small">Loading…</span>
                    </div>
                    <div class="table-responsive" style="overflow-x:auto;">
                        <table class="data-table" style="font-size:0.82rem;">
                            <thead>
                                <tr>
                                    <th>Schema</th><th>Table</th><th>Index</th>
                                    <th>Index Size</th><th>Scans</th>
                                    <th>Unique</th><th>Primary</th><th>Recommendation</th>
                                </tr>
                            </thead>
                            <tbody id="pgIdxBloatTbody">
                                <tr><td colspan="8" class="text-center text-muted">Loading…</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>`;

    // Tab switching
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            document.querySelectorAll('.tab-panel').forEach(p => p.style.display = 'none');
            btn.classList.add('active');
            const panel = document.getElementById(`pgAvTab-${btn.dataset.tab}`);
            if (panel) panel.style.display = '';
        });
    });

    const loadAll = async () => {
        const [bloatResp, idleResp, xidResp, longTxnResp, idxBloatResp] = await Promise.allSettled([
            window.apiClient.authenticatedFetch(`/api/postgres/bloat?instance=${encodeURIComponent(instanceName)}&limit=200`),
            window.apiClient.authenticatedFetch(`/api/postgres/idle-in-transaction?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/xid-wraparound?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/long-running-transactions?instance=${encodeURIComponent(instanceName)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/index-bloat?instance=${encodeURIComponent(instanceName)}&limit=200`),
        ]);

        // ---- Bloat ----
        try {
            const meta = document.getElementById('pgBloatMeta');
            const tbody = document.getElementById('pgBloatTbody');
            if (bloatResp.status === 'fulfilled' && bloatResp.value.ok) {
                const data = await bloatResp.value.json();
                const rows = data.tables || [];
                meta.textContent = `${rows.length} tables with dead tuples`;
                if (rows.length === 0) {
                    tbody.innerHTML = `<tr><td colspan="13" class="text-center text-muted">No bloat detected — all tables healthy</td></tr>`;
                } else {
                    tbody.innerHTML = rows.map(r => {
                        const deadPctCls = r.dead_pct >= 30 ? 'text-danger' : r.dead_pct >= 10 ? 'text-warning' : '';
                        const recCls = r.recommendation && r.recommendation.startsWith('VACUUM FREEZE') ? 'text-danger' : r.dead_pct >= 30 ? 'text-danger' : r.dead_pct >= 10 ? 'text-warning' : 'text-muted';
                        const freezeM = (Number(r.table_freeze_age || 0) / 1e6).toFixed(1);
                        const freezeCls = Number(r.table_freeze_age || 0) > 1575000000 ? 'text-danger' : Number(r.table_freeze_age || 0) > 1050000000 ? 'text-warning' : '';
                        const vacLagSec = Number(r.vacuum_lag_seconds || 0);
                        const vacLagStr = vacLagSec < 0 ? 'Never' : vacLagSec < 3600 ? `${(vacLagSec/60).toFixed(0)}m` : vacLagSec < 86400 ? `${(vacLagSec/3600).toFixed(1)}h` : `${(vacLagSec/86400).toFixed(1)}d`;
                        const vacLagCls = vacLagSec < 0 ? 'text-danger' : vacLagSec > 86400 ? 'text-warning' : '';
                        return `<tr>
                            <td>${esc(r.schema)}</td>
                            <td><strong>${esc(r.table)}</strong></td>
                            <td>${esc(r.total_size || fmtBytes(r.total_bytes))}</td>
                            <td>${Number(r.live_tuples||0).toLocaleString()}</td>
                            <td>${Number(r.dead_tuples||0).toLocaleString()}</td>
                            <td class="${deadPctCls}">${Number(r.dead_pct||0).toFixed(1)}%</td>
                            <td>${Number(r.estimated_waste_mb||0).toFixed(2)} MB</td>
                            <td>${Number(r.seq_scans||0).toLocaleString()}</td>
                            <td class="text-muted">${fmtTs(r.last_autovacuum)}</td>
                            <td class="${vacLagCls}">${vacLagStr}</td>
                            <td class="text-muted">${fmtTs(r.last_vacuum)}</td>
                            <td class="${freezeCls}">${freezeM}M</td>
                            <td class="${recCls}">${esc(r.recommendation)}</td>
                        </tr>`;
                    }).join('');
                }
            } else {
                if (meta) meta.textContent = 'Load error';
                if (tbody) tbody.innerHTML = `<tr><td colspan="13" class="text-center text-danger">Failed to load bloat data</td></tr>`;
            }
        } catch (e) { console.error('Bloat render error:', e); }

        // ---- Idle In Transaction ----
        try {
            const meta = document.getElementById('pgIdleMeta');
            const tbody = document.getElementById('pgIdleTbody');
            const alertEl = document.getElementById('pgIdleAlert');
            if (idleResp.status === 'fulfilled' && idleResp.value.ok) {
                const data = await idleResp.value.json();
                const sessions = data.sessions || [];
                const count = sessions.length;
                meta.textContent = `${count} session${count !== 1 ? 's' : ''}`;

                if (count > 0) {
                    alertEl.innerHTML = `
                        <div class="alert alert-warning" style="padding:.6rem 1rem;border-radius:6px;display:flex;align-items:center;gap:.5rem;">
                            <i class="fa-solid fa-triangle-exclamation"></i>
                            <strong>${count} idle-in-transaction session${count !== 1 ? 's' : ''} detected.</strong>
                            These sessions hold locks and prevent vacuum from reclaiming dead rows. Terminate long-running sessions if safe to do so.
                        </div>`;
                } else {
                    alertEl.innerHTML = `<div class="alert alert-success" style="padding:.6rem 1rem;border-radius:6px;">
                        <i class="fa-solid fa-circle-check"></i> No idle-in-transaction sessions detected.</div>`;
                }

                if (sessions.length === 0) {
                    tbody.innerHTML = `<tr><td colspan="9" class="text-center text-muted">No idle-in-transaction sessions</td></tr>`;
                } else {
                    tbody.innerHTML = sessions.map(s => {
                        const durCls = s.idle_seconds > 300 ? 'text-danger' : s.idle_seconds > 60 ? 'text-warning' : '';
                        return `<tr>
                            <td>${esc(s.pid)}</td>
                            <td>${esc(s.user_name)}</td>
                            <td>${esc(s.database)}</td>
                            <td class="text-warning">${esc(s.state)}</td>
                            <td class="${durCls}"><strong>${fmtDur(s.idle_seconds)}</strong></td>
                            <td>${esc(s.wait_event_type)} / ${esc(s.wait_event)}</td>
                            <td>${esc(s.client_addr || '—')}</td>
                            <td class="text-muted">${fmtTs(s.query_start)}</td>
                            <td style="max-width:320px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${esc(s.query)}">${esc((s.query||'').slice(0,120))}</td>
                        </tr>`;
                    }).join('');
                }
            } else {
                if (meta) meta.textContent = 'Load error';
            }
        } catch (e) { console.error('Idle-in-txn render error:', e); }

        // ---- XID Wraparound ----
        try {
            const body = document.getElementById('pgXIDBody');
            if (xidResp.status === 'fulfilled' && xidResp.value.ok) {
                const data = await xidResp.value.json();
                const dbs = data.databases || [];
                if (dbs.length === 0) {
                    body.innerHTML = `<p class="text-muted text-center">No database freeze risk data available</p>`;
                } else {
                    const maxAge = Math.max(...dbs.map(d => Number(d.freeze_age || 0)));
                    body.innerHTML = `
                        <div style="max-width:800px;">
                        ${dbs.map(d => {
                            const pct = Math.min(100, Number(d.used_pct || 0));
                            const barCls = d.risk_level === 'critical' || d.risk_level === 'high'
                                ? 'background:var(--danger)'
                                : d.risk_level === 'medium'
                                ? 'background:var(--warning)'
                                : 'background:var(--success)';
                            const ageM = (Number(d.freeze_age||0)/1e6).toFixed(1);
                            const limitM = (Number(d.wraparound_limit||200000000)/1e6).toFixed(0);
                            return `
                            <div style="margin-bottom:1rem;">
                                <div style="display:flex;justify-content:space-between;align-items:baseline;margin-bottom:.2rem;">
                                    <strong>${esc(d.database_name)}</strong>
                                    <span style="font-size:0.8rem;">${riskBadge(d.risk_level)} — Age: ${ageM}M / Limit: ${limitM}M XID — ${pct.toFixed(1)}% used</span>
                                </div>
                                <div style="background:var(--bg-card-2,#2a2a3a);border-radius:4px;height:16px;overflow:hidden;">
                                    <div style="height:100%;width:${pct}%;${barCls};border-radius:4px;transition:width .3s;"></div>
                                </div>
                            </div>`;
                        }).join('')}
                        <p class="text-muted small" style="margin-top:.5rem;">
                            <i class="fa-solid fa-circle-info"></i>
                            Thresholds: low &lt;25%, medium 25–50%, high 50–75%, critical &gt;75% of 2.1B XID limit.
                            Consider running <code>VACUUM FREEZE</code> on high-risk databases.
                        </p>
                        </div>`;
                }
            } else {
                body.innerHTML = `<p class="text-danger text-center">Failed to load XID risk data</p>`;
            }
        } catch (e) { console.error('XID render error:', e); }

        // ---- Long-Running Transactions ----
        try {
            const meta = document.getElementById('pgLongTxnMeta');
            const tbody = document.getElementById('pgLongTxnTbody');
            const alertEl = document.getElementById('pgLongTxnAlert');
            if (longTxnResp.status === 'fulfilled' && longTxnResp.value.ok) {
                const data = await longTxnResp.value.json();
                const txns = data.transactions || [];
                const count = txns.length;
                if (meta) meta.textContent = `${count} transaction${count !== 1 ? 's' : ''}`;

                if (alertEl) {
                    if (count > 0) {
                        alertEl.innerHTML = `
                            <div class="alert alert-warning" style="padding:.6rem 1rem;border-radius:6px;display:flex;align-items:center;gap:.5rem;">
                                <i class="fa-solid fa-triangle-exclamation"></i>
                                <strong>${count} long-running transaction${count !== 1 ? 's' : ''} detected (&gt;1 minute).</strong>
                                Long transactions hold locks, bloat WAL, and prevent vacuum from reclaiming space. Investigate and terminate if safe.
                            </div>`;
                    } else {
                        alertEl.innerHTML = `<div class="alert alert-success" style="padding:.6rem 1rem;border-radius:6px;">
                            <i class="fa-solid fa-circle-check"></i> No long-running active transactions detected.</div>`;
                    }
                }

                if (tbody) {
                    if (txns.length === 0) {
                        tbody.innerHTML = `<tr><td colspan="9" class="text-center text-muted">No long-running transactions</td></tr>`;
                    } else {
                        tbody.innerHTML = txns.map(t => {
                            const durCls = t.txn_duration_seconds > 600 ? 'text-danger' : t.txn_duration_seconds > 120 ? 'text-warning' : '';
                            return `<tr>
                                <td>${esc(t.pid)}</td>
                                <td>${esc(t.user_name)}</td>
                                <td>${esc(t.database)}</td>
                                <td>${esc(t.state)}</td>
                                <td class="${durCls}"><strong>${fmtDur(t.txn_duration_seconds)}</strong></td>
                                <td>${esc(t.wait_event_type)} / ${esc(t.wait_event)}</td>
                                <td>${esc(t.client_addr || '—')}</td>
                                <td class="text-muted">${fmtTs(t.xact_start)}</td>
                                <td style="max-width:320px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${esc(t.query)}">${esc((t.query||'').slice(0,120))}</td>
                            </tr>`;
                        }).join('');
                    }
                }
            } else {
                if (meta) meta.textContent = 'Load error';
            }
        } catch (e) { console.error('Long-txn render error:', e); }

        // ---- Index Bloat ----
        try {
            const meta = document.getElementById('pgIdxBloatMeta');
            const tbody = document.getElementById('pgIdxBloatTbody');
            if (idxBloatResp.status === 'fulfilled' && idxBloatResp.value.ok) {
                const data = await idxBloatResp.value.json();
                const indexes = data.indexes || [];
                if (meta) meta.textContent = `${indexes.length} indexes`;
                if (indexes.length === 0) {
                    if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="text-center text-muted">No index data available</td></tr>`;
                } else if (tbody) {
                    tbody.innerHTML = indexes.map(ix => {
                        const recCls = (ix.recommendation || '').startsWith('Unused') ? 'text-danger'
                            : (ix.recommendation || '').startsWith('Large') ? 'text-warning' : 'text-muted';
                        const scansCls = ix.idx_scans === 0 && !ix.is_primary && !ix.is_unique ? 'text-danger' : '';
                        return `<tr>
                            <td>${esc(ix.schema)}</td>
                            <td>${esc(ix.table)}</td>
                            <td><strong>${esc(ix.index_name)}</strong></td>
                            <td>${esc(ix.index_size)}</td>
                            <td class="${scansCls}">${Number(ix.idx_scans||0).toLocaleString()}</td>
                            <td>${ix.is_unique ? '<span class="badge badge-info">yes</span>' : '<span class="badge badge-secondary">no</span>'}</td>
                            <td>${ix.is_primary ? '<span class="badge badge-success">yes</span>' : '<span class="badge badge-secondary">no</span>'}</td>
                            <td class="${recCls}">${esc(ix.recommendation)}</td>
                        </tr>`;
                    }).join('');
                }
            } else {
                if (meta) meta.textContent = 'Load error';
            }
        } catch (e) { console.error('Index bloat render error:', e); }

        // ---- Update tab badges ----
        try {
            const bloatCount = document.getElementById('pgBloatTbody')?.querySelectorAll('tr:not(.text-muted)').length || 0;
            const idleCount  = document.getElementById('pgIdleTbody')?.querySelectorAll('tr:not(.text-muted)').length || 0;
            const longCount  = document.getElementById('pgLongTxnTbody')?.querySelectorAll('tr:not(.text-muted)').length || 0;
            const idxCount   = document.getElementById('pgIdxBloatTbody')?.querySelectorAll('tr:not(.text-muted)').length || 0;
            const bb = document.getElementById('tabBadge-bloat');
            const bi = document.getElementById('tabBadge-idle');
            const bl = document.getElementById('tabBadge-longtxn');
            const bx = document.getElementById('tabBadge-idxbloat');
            if (bb) bb.textContent = bloatCount;
            if (bi) bi.textContent = idleCount;
            if (bl) bl.textContent = longCount;
            if (bx) bx.textContent = idxCount;
        } catch (_) {}
    };

    document.getElementById('pgAvRefreshBtn')?.addEventListener('click', loadAll);
    await loadAll();
};
