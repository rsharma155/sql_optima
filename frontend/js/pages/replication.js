/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Replication monitoring for PostgreSQL and SQL Server.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgReplicationView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...'};
    const database = window.appState.currentDatabase || 'all';

    let replPayload = null;
    let replData = {
        is_primary: false,
        cluster_state: 'unknown',
        max_lag_mb: 0,
        wal_gen_rate_mbps: 0,
        bg_writer_eff_pct: 0,
        standbys: []
    };
    let ccHistory = null;
    let replLagSeries = null;
    let slotPayload = null;
    let slots = [];
    try {
        const [replResp, histResp, lagResp, slotsResp] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/postgres/replication?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/control-center/history?instance=${encodeURIComponent(inst.name)}&limit=180`),
            window.apiClient.authenticatedFetch(`/api/postgres/replication-lag/history?instance=${encodeURIComponent(inst.name)}&limit=180`),
            window.apiClient.authenticatedFetch(`/api/postgres/replication-slots?instance=${encodeURIComponent(inst.name)}`),
        ]);
        if (replResp.ok) {
            const contentType = replResp.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                replPayload = await replResp.json();
                replData = replPayload?.stats || replData;
            }
        } else console.error("Failed to load PG replication:", replResp.status);

        if (histResp.ok) {
            const ct = histResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await histResp.json();
                ccHistory = payload && payload.history ? payload.history : null;
            }
        }
        if (lagResp.ok) {
            const ct = lagResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await lagResp.json();
                replLagSeries = payload && payload.series ? payload.series : null;
            }
        }
        if (slotsResp.ok) {
            const ct = slotsResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                slotPayload = await slotsResp.json();
                slots = (slotPayload && slotPayload.slots) ? slotPayload.slots : [];
            }
        }
    } catch (e) {
        console.error("PG replication fetch failed:", e);
    }

    const standbys = replData.standbys || [];
    const isPrimary = replData.is_primary !== false;
    const maxLag = replData.max_lag_mb || 0;
    const walRate = replData.wal_gen_rate_mbps || 0;
    const bgEff = replData.bg_writer_eff_pct || 0;
    const haProvider = (replPayload?.ha_provider || 'auto').toString();
    const haLabel = haProvider === 'cnpg' ? 'CNPG' : haProvider === 'patroni' ? 'Patroni' : haProvider === 'streaming' ? 'Streaming Replication' : 'Auto';
    const hasSlots = Array.isArray(slots) && slots.length > 0;
    const maxRetained = hasSlots ? Math.max(...slots.map(s => Number(s.retained_wal_mb || 0))) : 0;

            window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme pg-replication-page">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line" style="flex:1; min-width:0;">
                    <h1>Replication, HA &amp; Cluster Health</h1>
                    <span class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Database: <span class="text-accent">${window.escapeHtml(database)}</span></span>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:0.75rem; flex-wrap:wrap; justify-content:flex-end;">
                    <div id="pgReplStatusStrip"></div>
                    <span class="badge badge-outline" style="font-size:0.65rem;">Mode: ${window.escapeHtml(haLabel)}</span>
                    <button class="btn btn-sm btn-outline" data-action="navigate-back"><i class="fa-solid fa-arrow-left"></i> Back</button>
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="PgReplicationView"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>

            <div class="top-strips dashboard-top-strips" style="margin-top:0.5rem;">
                <div class="glass-panel dashboard-strip-panel">
                    <div class="dashboard-strip-header">
                        <h4>
                            <span class="dashboard-strip-header-icons" aria-hidden="true">
                                <i class="fa-solid fa-clone" title="Replication"></i>
                                <i class="fa-solid fa-file-pen" title="WAL"></i>
                                <i class="fa-solid fa-brush" title="BGWriter"></i>
                            </span>
                            Replication &amp; WAL snapshot
                        </h4>
                    </div>
                    <div class="dashboard-strip-metrics-row--6">
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">Role</div>
                            <div class="strip-metric-value">${isPrimary ? 'Primary' : 'Standby'}</div>
                            <div class="text-muted sub">${window.escapeHtml(replData.cluster_state || 'unknown')}</div>
                        </div>
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">Standbys</div>
                            <div class="strip-metric-value">${standbys.length}</div>
                            <div class="text-muted sub">connected</div>
                        </div>
                        <div class="strip-metric-cell ${maxLag > 50 ? 'strip-metric-cell--accent-bad' : (maxLag > 10 ? 'strip-metric-cell--accent-warn' : '')}">
                            <div class="strip-metric-label">Max lag</div>
                            <div class="strip-metric-value">${maxLag.toFixed(1)} <span class="text-muted" style="font-size:0.75em;">MB</span></div>
                            <div class="text-muted sub">${maxLag > 10 ? 'Lag detected' : 'In sync'}</div>
                        </div>
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">WAL rate</div>
                            <div class="strip-metric-value">${walRate.toFixed(1)} <span class="text-muted" style="font-size:0.75em;">MB/s</span></div>
                            <div class="text-muted sub">generation</div>
                        </div>
                        <div class="strip-metric-cell ${bgEff < 90 ? 'strip-metric-cell--accent-warn' : ''}">
                            <div class="strip-metric-label">BGWriter eff</div>
                            <div class="strip-metric-value">${bgEff.toFixed(0)}%</div>
                            <div class="text-muted sub">${bgEff > 95 ? 'Efficient' : 'Review'}</div>
                        </div>
                        <div class="strip-metric-cell">
                            <div class="strip-metric-label">Topology</div>
                            <div class="strip-metric-value">${standbys.length + (isPrimary ? 1 : 0)}</div>
                            <div class="text-muted sub">nodes</div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- WAL Archiver Risk Strip (populated async) -->
            <div id="pgWALArchiverRiskStrip" style="margin-top:0.5rem;"></div>

            <div class="charts-grid mt-3" style="display:grid; grid-template-columns:1fr 2fr; gap:0.75rem;">
                <div class="chart-card glass-panel" style="padding:0.75rem;">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Replication Lag Trend</h3></div>
                    <div class="chart-container" style="height:140px;"><canvas id="pgReplCtx"></canvas></div>
                </div>
                <div class="chart-card glass-panel" style="padding:0.75rem;">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Checkpoint Pressure (Req/Timed)</h3></div>
                    <div class="chart-container" style="height:140px;"><canvas id="pgCheckChart"></canvas></div>
                </div>
            </div>

            <div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Connected Standby Nodes (${window.escapeHtml(haLabel)})</h3></div>
                    <div class="pg-repl-table-scroll" role="region" aria-label="Standby nodes">
                        <div class="pg-repl-row pg-repl-row--head pg-repl-standby-cols">
                            <div>${haProvider === 'cnpg' ? 'Pod / Application' : 'Application'}</div>
                            <div>Client Addr</div>
                            <div>State</div>
                            <div>Sync State</div>
                            <div>Lag (MB)</div>
                        </div>
                        ${standbys.length > 0 ? standbys.map(standby => `
                            <div class="pg-repl-row pg-repl-standby-cols">
                                <div><strong>${window.escapeHtml(standby.replica_pod_name || standby.app_name || '')}</strong></div>
                                <div>${window.escapeHtml(standby.pod_ip || standby.client_addr || 'N/A')}</div>
                                <div><span class="badge ${standby.state === 'streaming' ? 'badge-success' : 'badge-warning'}">${window.escapeHtml(standby.state)}</span></div>
                                <div><span class="text-accent">${window.escapeHtml(standby.sync_state)}</span></div>
                                <div>${(Number(standby.replay_lag_mb || 0)).toFixed(2)}</div>
                            </div>
                        `).join('') : `
                            <div class="pg-repl-row pg-repl-standby-cols">
                                <div class="text-muted text-center" style="grid-column: 1 / -1; padding: 1rem;">No standby connections found</div>
                            </div>
                        `}
                    </div>
                </div>

                <div class="table-card glass-panel" id="pgSlotsCard" style="${hasSlots ? '' : 'display:none;'}">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Replication Slots (Risk)</h3></div>
                    <div class="p-2 text-muted" style="font-size:0.75rem;">
                        Worst retained WAL: <strong class="${maxRetained >= 1024 ? 'text-danger' : (maxRetained >= 256 ? 'text-warning' : 'text-success')}">${maxRetained.toFixed(1)} MB</strong>
                        <span class="text-muted">| Source: ${window.escapeHtml((slotPayload?.source || 'timescale').toString())}</span>
                    </div>
                    <div class="chart-container" id="pgSlotTrendCard" style="height:120px; margin:0 0.5rem 0.5rem 0.5rem; display:none;">
                        <canvas id="pgSlotTrendChart"></canvas>
                    </div>
                    <div class="pg-repl-table-scroll" role="region" aria-label="Replication slots">
                        <div class="pg-repl-row pg-repl-row--head pg-repl-slots-cols">
                            <div>Slot</div>
                            <div>Type</div>
                            <div>Active</div>
                            <div>Retained WAL</div>
                            <div>Restart LSN</div>
                            <div>Confirmed Flush LSN</div>
                        </div>
                        ${(hasSlots ? [...slots].sort((a,b)=>Number(b.retained_wal_mb||0)-Number(a.retained_wal_mb||0)).slice(0,50) : []).map(s => `
                            <div class="pg-repl-row pg-repl-slots-cols">
                                <div><strong>${window.escapeHtml(String(s.slot_name || '-'))}</strong></div>
                                <div>${window.escapeHtml(String(s.slot_type || '-'))}</div>
                                <div>${s.active ? '<span class="badge badge-success">true</span>' : '<span class="badge badge-secondary">false</span>'}</div>
                                <div class="${Number(s.retained_wal_mb||0) >= 1024 ? 'text-danger font-bold' : (Number(s.retained_wal_mb||0) >= 256 ? 'text-warning font-bold' : '')}">
                                    ${Number(s.retained_wal_mb || 0).toFixed(1)} MB
                                </div>
                                <div class="text-muted"><code>${window.escapeHtml(String(s.restart_lsn || ''))}</code></div>
                                <div class="text-muted"><code>${window.escapeHtml(String(s.confirmed_flush_lsn || ''))}</code></div>
                            </div>
                        `).join('') || `
                            <div class="pg-repl-row pg-repl-slots-cols">
                                <div class="text-muted text-center" style="grid-column: 1 / -1; padding: 1rem;">No replication slots found</div>
                            </div>
                        `}
                    </div>
                </div>
            </div>
        </div>
    `;

    const replChartOpts = {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        transitions: { active: { animation: { duration: 0 } } },
        layout: { padding: { top: 2, right: 2, bottom: 2, left: 2 } }
    };

    setTimeout(() => {
        requestAnimationFrame(() => {
            requestAnimationFrame(() => {
        try {
            const strip = document.getElementById('pgReplStatusStrip');
            if (strip && typeof window.renderStatusStrip === 'function') {
                strip.innerHTML = window.renderStatusStrip({
                    lastUpdateId: 'pgReplLastRefreshTime',
                    sourceBadgeId: 'pgReplSourceBadge',
                    includeHealth: false,
                    includeFreshness: false,
                    autoRefreshText: ''
                });
            }
            const t = document.getElementById('pgReplLastRefreshTime');
            if (t) t.textContent = new Date().toLocaleTimeString();
        } catch (e) {
            // non-fatal
        }

        window.currentCharts = window.currentCharts || {};
        try {
            if (window.currentCharts.pgRepl) { window.currentCharts.pgRepl.destroy(); delete window.currentCharts.pgRepl; }
            if (window.currentCharts.pgChk) { window.currentCharts.pgChk.destroy(); delete window.currentCharts.pgChk; }
            if (window.currentCharts.pgSlotTrend) { window.currentCharts.pgSlotTrend.destroy(); delete window.currentCharts.pgSlotTrend; }
        } catch (e) { /* ignore */ }

        const toLocalTime = (iso) => { const d = new Date(iso); return isNaN(d.getTime()) ? iso : d.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}); };

        const replCtx = document.getElementById('pgReplCtx');
        if (replCtx) {
            // Prefer per-replica lag series when available, otherwise fall back to CC max lag seconds series.
            if (replLagSeries && Object.keys(replLagSeries).length) {
                const seriesArr = Object.values(replLagSeries);
                const labels = (seriesArr[0]?.labels || []).map(toLocalTime);
                const palette = ['#3b82f6','#10b981','#f59e0b','#ef4444','#a855f7'];
                const datasets = seriesArr.map((s, idx) => ({
                    label: s.replica_name,
                    data: s.lag_mb || [],
                    borderColor: palette[idx % palette.length],
                    backgroundColor: palette[idx % palette.length],
                    tension: 0.25,
                    pointRadius: 0
                }));
                window.currentCharts.pgRepl = new Chart(replCtx.getContext('2d'), {
                    type: 'line',
                    data: { labels, datasets },
                    options: replChartOpts
                });
            } else if (ccHistory?.labels?.length) {
                window.currentCharts.pgRepl = new Chart(replCtx.getContext('2d'), {
                    type: 'line',
                    data: {
                        labels: ccHistory.labels.map(toLocalTime),
                        datasets: [{
                            label: 'Max replication lag (sec)',
                            data: ccHistory.replication_lag_seconds || [],
                            borderColor: window.getCSSVar('--warning'),
                            backgroundColor: 'rgba(245,158,11,0.15)',
                            tension: 0.25,
                            pointRadius: 0,
                            fill: true
                        }]
                    },
                    options: replChartOpts
                });
            }
        }

        const checkCtx = document.getElementById('pgCheckChart');
        if (checkCtx) {
            if (ccHistory?.labels?.length) {
                window.currentCharts.pgChk = new Chart(checkCtx.getContext('2d'), {
                    type: 'line',
                    data: {
                        labels: ccHistory.labels.map(toLocalTime),
                        datasets: [{
                            label: 'Checkpoint pressure (req/timed)',
                            data: (ccHistory.checkpoint_req_ratio || []),
                            borderColor: window.getCSSVar('--danger'),
                            backgroundColor: 'rgba(239,68,68,0.12)',
                            tension: 0.25,
                            pointRadius: 0,
                            fill: true
                        }]
                    },
                    options: replChartOpts
                });
            }
        }

        // Replication slot retention trend (worst retained MB over time)
        try {
            const slotTrendCanvas = document.getElementById('pgSlotTrendChart');
            const slotTrendCard = document.getElementById('pgSlotTrendCard');
            if (slotTrendCanvas && slotTrendCard && hasSlots) {
                // Fetch a larger set (history) from the same endpoint (Timescale ordered by time desc).
                window.apiClient.authenticatedFetch(`/api/postgres/replication-slots?instance=${encodeURIComponent(inst.name)}&limit=800`)
                    .then(r => r.ok ? r.json() : null)
                    .then(p => {
                        const rows = (p && p.slots) ? p.slots : [];
                        if (!rows.length) return;
                        // We only get latest-per-slot now; if Timescale history is desired, we'd need a dedicated history endpoint.
                        // So here we show a simple "current risk bar" chart across slots.
                        const top = rows.slice().sort((a,b)=>Number(b.retained_wal_mb||0)-Number(a.retained_wal_mb||0)).slice(0, 8);
                        const labels = top.map(s => String(s.slot_name || '').slice(0, 18));
                        const data = top.map(s => Number(s.retained_wal_mb || 0));
                        slotTrendCard.style.display = '';
                        if (window.currentCharts.pgSlotTrend) window.currentCharts.pgSlotTrend.destroy();
                        window.currentCharts.pgSlotTrend = new Chart(slotTrendCanvas.getContext('2d'), {
                            type: 'bar',
                            data: { labels, datasets: [{ label: 'Retained WAL (MB)', data, backgroundColor: window.getCSSVar('--danger') }] },
                            options: { responsive:true, maintainAspectRatio:false, animation:false, plugins:{ legend:{ display:false } }, scales:{ y:{ beginAtZero:true } } }
                        });
                    })
                    .catch(() => {});
            }
        } catch (e) {
            // non-fatal
        }
            });
        });
    }, 50);

    // --- WAL Archiver Risk Strip (Epic 3.2) ---
    window.apiClient.authenticatedFetch(`/api/postgres/wal/archiver-risk?instance=${encodeURIComponent(inst.name)}`)
        .then(r => r.ok ? r.json() : null)
        .then(payload => {
            const strip = document.getElementById('pgWALArchiverRiskStrip');
            if (!strip || !payload?.risk) return;
            const risk = payload.risk;
            const lvl = risk.risk_level || 'low';
            const cls = lvl === 'critical' ? 'alert-danger' : lvl === 'high' ? 'alert-warning' : lvl === 'medium' ? 'alert-warning' : 'alert-success';
            const icon = lvl === 'critical' || lvl === 'high' ? 'fa-triangle-exclamation' : lvl === 'medium' ? 'fa-circle-info' : 'fa-circle-check';
            const fmtAge = (s) => {
                if (!isFinite(s) || s < 0) return '—';
                if (s < 60) return `${Math.round(s)}s`;
                if (s < 3600) return `${(s/60).toFixed(1)}m`;
                return `${(s/3600).toFixed(2)}h`;
            };
            const failRate = (risk.failure_rate_pct || 0).toFixed(1);
            const retained = (risk.max_retained_slot_mb || 0).toFixed(1);
            const details = [];
            if (risk.archived_count !== undefined)
                details.push(`Archived: <strong>${Number(risk.archived_count).toLocaleString()}</strong>`);
            if (risk.failed_count !== undefined)
                details.push(`Failed: <strong class="${risk.failed_count > 0 ? 'text-danger' : ''}">${Number(risk.failed_count).toLocaleString()}</strong> (${failRate}%)`);
            if (risk.last_archived_wal)
                details.push(`Last WAL: <code style="font-size:0.75rem;">${window.escapeHtml ? window.escapeHtml(risk.last_archived_wal.slice(-20)) : risk.last_archived_wal.slice(-20)}</code>`);
            if (risk.last_archived_age_seconds >= 0)
                details.push(`Archive lag: <strong>${fmtAge(risk.last_archived_age_seconds)}</strong>`);
            if (Number(retained) > 0)
                details.push(`Slot retention: <strong class="${Number(retained) >= 1024 ? 'text-danger' : (Number(retained) >= 256 ? 'text-warning' : '')}">${retained} MB</strong> (${window.escapeHtml ? window.escapeHtml(risk.high_retention_slot || '') : (risk.high_retention_slot || '')})`);
            strip.innerHTML = `
                <div class="alert ${cls}" style="padding:.5rem .75rem;border-radius:6px;font-size:0.82rem;display:flex;align-items:center;gap:.75rem;flex-wrap:wrap;">
                    <i class="fa-solid ${icon}"></i>
                    <strong>WAL Archiver Risk: ${lvl.toUpperCase()}</strong>
                    <span class="text-muted" style="flex:1;">${details.join(' &nbsp;|&nbsp; ')}</span>
                    ${risk.last_failed_wal ? `<span class="text-danger small">Last failed WAL: <code>${window.escapeHtml ? window.escapeHtml(risk.last_failed_wal.slice(-24)) : risk.last_failed_wal.slice(-24)}</code></span>` : ''}
                </div>`;
        })
        .catch(() => {});
}
