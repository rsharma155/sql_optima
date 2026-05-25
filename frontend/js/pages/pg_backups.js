/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Unified PostgreSQL Backup & DR dashboard (WAL archiving, replication/HA,
 *          replication slots, base backups, and DR readiness vs per-instance policy).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */
(function () {
    'use strict';

    let pgBackupCharts = {};

    function tsPoint(d) {
        const t = d.timestamp || d.capture_timestamp || d.collected_at;
        if (!t) return null;
        const ms = Date.parse(String(t));
        return isNaN(ms) ? null : ms;
    }

    function fmtTick(d) {
        const ms = tsPoint(d);
        return ms ? new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '';
    }

    function chartOpts(extra) {
        return {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                x: {
                    display: true,
                    grid: { display: false },
                    ticks: { color: '#94a3b8', font: { size: 8 }, maxTicksLimit: 8 }
                },
                y: {
                    beginAtZero: true,
                    grid: { color: 'rgba(148,163,184,0.1)' },
                    ticks: { color: '#94a3b8', font: { size: 9 } }
                }
            },
            ...extra
        };
    }

    window.PgBackupsView = async function () {
        const inst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
        if (!inst || String(inst.type || '').toLowerCase() !== 'postgres') {
            window.routerOutlet.innerHTML = '<div class="page-view active"><div class="alert alert-warning">Select a PostgreSQL instance.</div></div>';
            return;
        }

        window.appState.activeViewId = 'pg-backups';
        const dbName = window.appState.currentDatabase || 'all';
        window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_backups.html', { inst, dbName });

        window.appState.pgBackups = window.appState.pgBackups || {};
        const state = window.appState.pgBackups;
        const now = new Date();
        const oneHourAgo = new Date(now.getTime() - 3600000);
        const pad = n => String(n).padStart(2, '0');
        const fmtLocal = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

        state.fromLocal = state.fromLocal || fmtLocal(oneHourAgo);
        state.toLocal = state.toLocal || fmtLocal(now);

        const fromInput = document.getElementById('pgBackupFrom');
        const toInput = document.getElementById('pgBackupTo');
        if (fromInput) fromInput.value = state.fromLocal;
        if (toInput) toInput.value = state.toLocal;

        document.getElementById('pgBackupApply')?.addEventListener('click', () => {
            state.fromLocal = fromInput.value;
            state.toLocal = toInput.value;
            refreshPgBackupDR(inst.name);
        });

        document.getElementById('pg-dr-alerts-link')?.addEventListener('click', (e) => {
            e.preventDefault();
            if (typeof window.navigateTo === 'function') window.navigateTo('pg-alerts');
        });

        initPgBackupCharts();
        await refreshPgBackupDR(inst.name);

        if (window.pgBackupsInterval) clearInterval(window.pgBackupsInterval);
        window.pgBackupsInterval = window.registerInterval(() => {
            if (window.appState.activeViewId === 'pg-backups') {
                refreshPgBackupDR(inst.name);
            } else {
                clearInterval(window.pgBackupsInterval);
            }
        }, 30000);
    };

    // Backward-compatible alias
    window.refreshPgBackupData = function (instName) {
        return refreshPgBackupDR(instName);
    };

    async function refreshPgBackupDR(instName) {
        const el = document.getElementById('pgLastRefreshTime');
        if (el) el.textContent = new Date().toLocaleTimeString();

        const state = window.appState.pgBackups || {};
        let from = state.fromLocal;
        let to = state.toLocal;
        if (from && from.includes('T')) from = new Date(from).toISOString();
        if (to && to.includes('T')) to = new Date(to).toISOString();

        const q = `instance=${encodeURIComponent(instName)}`;
        const range = (from && to) ? `&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}` : '';

        try {
            const [dashResp, replResp, infoResp] = await Promise.all([
                window.apiClient.authenticatedFetch(`/api/pg/backup/dashboard?${q}${range}`),
                window.apiClient.authenticatedFetch(`/api/postgres/replication?${q}${range}&limit=500`),
                window.apiClient.authenticatedFetch(`/api/postgres/server-info?${q}`),
            ]);

            let dash = {};
            if (dashResp.ok) dash = await dashResp.json();

            let liveRepl = { stats: { standbys: [] } };
            if (replResp.ok) liveRepl = await replResp.json();

            if (infoResp.ok) {
                const info = await infoResp.json();
                updateHeaderMeta(info, dash.kpis, liveRepl.stats);
            }

            if (dash.readiness) renderReadiness(dash.readiness);
            if (dash.kpis) updateKPIs(dash.kpis, liveRepl.stats);
            updateWALChart(dash.wal_trend);
            updateLagChart(dash.replication_lag_trend);
            updateArchiveChart(dash.archive_health);
            updateCheckpointChart(dash.checkpoint_trend);
            updateReplicationTable(dash.replication_details, liveRepl.stats);
            updateArchiverFailureTable(dash.archiver_failures);
            updateSlotsTable(dash.replication_slots);
            updateBackupSections(dash.backup_latest, dash.backup_history);

            let ccHist = null;
            let lagMbSeries = null;
            try {
                const [ccResp, lagMbResp] = await Promise.all([
                    window.apiClient.authenticatedFetch(`/api/postgres/checkpoint-health/history?${q}${range}`),
                    window.apiClient.authenticatedFetch(`/api/postgres/replication-lag/history?${q}${range}&limit=500`),
                ]);
                if (ccResp.ok) ccHist = await ccResp.json();
                if (lagMbResp.ok) {
                    const p = await lagMbResp.json();
                    lagMbSeries = p.series;
                }
            } catch (_) { /* optional charts */ }

            updateCheckpointActivityChart(ccHist);
            updateLagMbChart(lagMbSeries);
        } catch (e) {
            console.error('Failed to refresh PG Backup & DR:', e);
        }
    }

    function updateHeaderMeta(info, kpis, stats) {
        const hs = info.health_score ?? 0;
        const badge = document.getElementById('pg-dr-health-score');
        if (badge) {
            badge.textContent = hs;
            badge.className = 'badge badge-' + (hs > 80 ? 'success' : hs > 60 ? 'warning' : 'danger');
        }
        const ver = document.getElementById('pg-dr-version');
        if (ver) ver.textContent = (info.version || '').split(',')[0] || '--';
        const roleEl = document.getElementById('pg-dr-role');
        if (roleEl) {
            const role = (kpis?.node_role || (stats?.is_primary === false ? 'replica' : stats?.standbys?.length ? 'primary' : 'standalone') || '').toString().toUpperCase();
            roleEl.textContent = role || '--';
            const notice = document.getElementById('ha-replica-notice');
            if (notice) notice.style.display = role === 'REPLICA' ? 'block' : 'none';
        }
    }

    function renderReadiness(readiness) {
        const overall = document.getElementById('pg-dr-overall');
        const row = document.getElementById('pg-dr-readiness-row');
        if (!row) return;
        const o = readiness.overall || 'green';
        if (overall) {
            overall.textContent = 'Overall: ' + o.toUpperCase();
            overall.className = 'pg-dr-overall text-' + (o === 'green' ? 'success' : o === 'red' ? 'danger' : 'warning');
        }
        const pillars = readiness.pillars || [];
        row.innerHTML = pillars.map(p => `
            <div class="pg-dr-pillar status-${p.status}">
                <h4>${window.escapeHtml(p.title)}</h4>
                <span class="pillar-status text-${p.status === 'green' ? 'success' : p.status === 'red' ? 'danger' : 'warning'}">${p.status}</span>
                <p>${window.escapeHtml(p.summary)}</p>
                <div class="text-muted" style="font-size:0.65rem;">${window.escapeHtml(p.target_label || '')}</div>
            </div>
        `).join('');
    }

    function updateKPIs(kpis, liveStats) {
        if (!kpis) return;
        const set = (id, val) => {
            const el = document.getElementById(id);
            if (el) el.textContent = val;
        };
        set('stat-wal-rate', (kpis.wal_generation_rate_mb_min ?? 0).toFixed(2));

        const arcSucc = kpis.archive_success_percent;
        const arcEl = document.getElementById('stat-archive-success');
        if (arcEl) {
            arcEl.textContent = arcSucc != null ? Number(arcSucc).toFixed(1) : '100';
            arcEl.className = 'kpi-value ' + (arcSucc < 99 ? 'text-danger' : 'text-success');
        }

        const maxLag = kpis.replica_max_lag_seconds ?? 0;
        const lagEl = document.getElementById('stat-max-lag');
        if (lagEl) {
            lagEl.textContent = maxLag < 60 ? maxLag.toFixed(0) + 's' : (maxLag / 60).toFixed(1) + 'm';
            lagEl.className = 'kpi-value ' + (maxLag > 60 ? 'text-danger' : maxLag > 10 ? 'text-warning' : 'text-success');
        }
        const mbLag = liveStats?.max_lag_mb ?? 0;
        set('stat-max-lag-mb', mbLag.toFixed(1));

        set('stat-slots-risk', (kpis.replication_slots_risk_gb ?? 0).toFixed(2));
        set('stat-archive-age', kpis.last_archive_age || 'N/A');
        set('stat-avg-checkpoint', (kpis.checkpoint_avg_write_time ?? 0).toFixed(0));
        set('stat-bgwriter', (liveStats?.bg_writer_eff_pct ?? 0).toFixed(1));
    }

    function initPgBackupCharts() {
        const ids = ['walTrendChart', 'replicaLagChart', 'replicaLagMbChart', 'archiveHealthChart', 'checkpointTrendChart', 'checkpointActivityChart'];
        ids.forEach(id => {
            const ctx = document.getElementById(id)?.getContext('2d');
            if (!ctx) return;
            if (pgBackupCharts[id]) {
                try { pgBackupCharts[id].destroy(); } catch (_) {}
            }
            const type = id === 'archiveHealthChart' ? 'bar' : 'line';
            const datasets = id === 'archiveHealthChart'
                ? [
                    { label: 'Success', data: [], backgroundColor: '#4ade80' },
                    { label: 'Failed', data: [], backgroundColor: '#f87171' }
                ]
                : id === 'checkpointTrendChart'
                    ? [
                        { label: 'Write', data: [], borderColor: '#f472b6', tension: 0.3, pointRadius: 0 },
                        { label: 'Sync', data: [], borderColor: '#818cf8', tension: 0.3, pointRadius: 0 }
                    ]
                    : id === 'checkpointActivityChart'
                        ? [
                            { label: 'Timed', data: [], borderColor: '#10b981', tension: 0.3, pointRadius: 0 },
                            { label: 'Requested', data: [], borderColor: '#ef4444', tension: 0.3, pointRadius: 0 }
                        ]
                        : [{ label: 'Value', data: [], borderColor: '#4cc9f0', tension: 0.3, fill: false, pointRadius: 0 }];

            pgBackupCharts[id] = new Chart(ctx, {
                type,
                data: { labels: [], datasets },
                options: chartOpts({
                    plugins: {
                        legend: {
                            display: ['archiveHealthChart', 'checkpointTrendChart', 'checkpointActivityChart', 'replicaLagChart'].includes(id),
                            position: 'bottom',
                            labels: { boxWidth: 10, font: { size: 8 }, color: '#94a3b8' }
                        }
                    },
                    scales: id === 'archiveHealthChart' ? {
                        x: { stacked: true, ticks: { color: '#94a3b8', maxTicksLimit: 8 } },
                        y: { stacked: true, beginAtZero: true, ticks: { color: '#94a3b8' } }
                    } : undefined
                })
            });
        });
    }

    function updateWALChart(trend) {
        const c = pgBackupCharts.walTrendChart;
        if (!c || !trend?.length) return;
        c.data.labels = trend.map(fmtTick);
        c.data.datasets[0].data = trend.map(d => (Number(d.wal_bytes_delta || 0) / 1024 / 1024));
        c.update('none');
    }

    function updateLagChart(trend) {
        const c = pgBackupCharts.replicaLagChart;
        if (!c || !trend?.length) {
            if (c) { c.data.datasets = []; c.update('none'); }
            return;
        }
        const allTs = [...new Set(trend.map(p => p.timestamp || p.capture_timestamp))].sort();
        const labels = allTs.map(t => fmtTick({ timestamp: t }));
        const byReplica = {};
        trend.forEach(p => {
            const k = p.application_name || 'replica';
            if (!byReplica[k]) byReplica[k] = {};
            byReplica[k][p.timestamp || p.capture_timestamp] = p.lag_seconds ?? 0;
        });
        const colors = ['#4cc9f0', '#10b981', '#f59e0b', '#ef4444'];
        c.data.labels = labels;
        c.data.datasets = Object.entries(byReplica).map(([name, pts], i) => ({
            label: name,
            data: allTs.map(ts => pts[ts] ?? null),
            borderColor: colors[i % colors.length],
            tension: 0.3,
            pointRadius: 0,
            spanGaps: true
        }));
        c.update('none');
    }

    function updateLagMbChart(series) {
        const c = pgBackupCharts.replicaLagMbChart;
        if (!c) return;
        if (!series || !Array.isArray(series) || !series.length) {
            c.data.datasets = [];
            c.update('none');
            return;
        }
        const valid = series.filter(p => tsPoint(p) != null);
        c.data.labels = valid.map(p => fmtTick(p));
        c.data.datasets = [{
            label: 'Lag MB',
            data: valid.map(p => p.value),
            borderColor: '#3b82f6',
            backgroundColor: 'rgba(59,130,246,0.1)',
            fill: true,
            tension: 0.3,
            pointRadius: 0
        }];
        c.update('none');
    }

    function updateArchiveChart(health) {
        const c = pgBackupCharts.archiveHealthChart;
        if (!c || !health?.length) return;
        c.data.labels = health.map(fmtTick);
        c.data.datasets[0].data = health.map(d => d.archived_count ?? 0);
        c.data.datasets[1].data = health.map(d => d.archive_failed_count ?? 0);
        c.update('none');
    }

    function updateCheckpointChart(trend) {
        const c = pgBackupCharts.checkpointTrendChart;
        if (!c || !trend?.length) return;
        c.data.labels = trend.map(fmtTick);
        c.data.datasets[0].data = trend.map(d => d.checkpoint_write_time_ms ?? 0);
        c.data.datasets[1].data = trend.map(d => d.checkpoint_sync_time_ms ?? 0);
        c.update('none');
    }

    function updateCheckpointActivityChart(hist) {
        const c = pgBackupCharts.checkpointActivityChart;
        if (!c) return;
        const rows = Array.isArray(hist) ? hist : [];
        if (!rows.length) return;
        c.data.labels = rows.map(p => fmtTick({ timestamp: p.capture_timestamp || p.time }));
        c.data.datasets[0].data = rows.map(p => p.checkpoints_timed ?? 0);
        c.data.datasets[1].data = rows.map(p => p.checkpoints_req ?? 0);
        c.update('none');
    }

    function pgDrEmptyRow(colspan, icon, message) {
        return `<tr><td colspan="${colspan}" class="pg-dr-empty-cell">
            <i class="fa-solid ${icon}" aria-hidden="true"></i>
            <span>${window.escapeHtml(message)}</span>
        </td></tr>`;
    }

    function setRowCount(id, n, label) {
        const el = document.getElementById(id);
        if (el) el.textContent = n === 0 ? 'None' : `${n} ${label}${n === 1 ? '' : 's'}`;
    }

    function lagBadgeClass(sec) {
        const s = Number(sec);
        if (!isFinite(s) || s <= 0) return 'pg-dr-lag-ok';
        if (s > 60) return 'pg-dr-lag-critical';
        if (s > 10) return 'pg-dr-lag-warn';
        return 'pg-dr-lag-ok';
    }

    function updateReplicationTable(details, liveStats) {
        const tbody = document.getElementById('replicationTbody');
        if (!tbody) return;
        const standbys = liveStats?.standbys || [];
        const rows = [];

        if (standbys.length) {
            standbys.filter(Boolean).forEach(s => {
                const name = s.application_name || s.replica_pod_name || '?';
                const w = s.write_lag_sec != null ? `${Number(s.write_lag_sec).toFixed(1)}s` : '—';
                const f = s.flush_lag_sec != null ? `${Number(s.flush_lag_sec).toFixed(1)}s` : '—';
                const r = s.replay_lag_sec != null
                    ? `${Number(s.replay_lag_sec).toFixed(1)}s`
                    : window.escapeHtml(String(s.replay_lag_time || '—'));
                const replayMb = Number(s.replay_lag_mb ?? 0);
                const stateCls = String(s.state || '').toLowerCase() === 'streaming' ? 'badge-success' : 'badge-warning';
                rows.push(`
                    <tr>
                        <td><strong>${window.escapeHtml(name)}</strong></td>
                        <td class="pg-dr-mono">${window.escapeHtml(s.client_addr || s.pod_ip || '—')}</td>
                        <td><span class="badge ${stateCls}">${window.escapeHtml(s.state || '—')}</span></td>
                        <td>${window.escapeHtml(s.sync_state || '—')}</td>
                        <td class="pg-dr-lag-cell">
                            <span class="${lagBadgeClass(s.write_lag_sec)}">${w}</span>
                            <span class="text-muted">/</span>
                            <span class="${lagBadgeClass(s.flush_lag_sec)}">${f}</span>
                            <span class="text-muted">/</span>
                            <span class="${lagBadgeClass(s.replay_lag_sec)}">${r}</span>
                        </td>
                        <td class="${replayMb > 100 ? 'text-danger' : replayMb > 10 ? 'text-warning' : ''}">${replayMb.toFixed(1)} MB</td>
                        <td class="pg-dr-mono"><code>${window.escapeHtml(s.sent_lsn || '—')}</code></td>
                    </tr>`);
            });
        } else if (details?.length) {
            details.forEach(d => {
                const stateCls = String(d.state || '').toLowerCase() === 'streaming' ? 'badge-success' : 'badge-warning';
                rows.push(`
                    <tr>
                        <td><strong>${window.escapeHtml(d.application_name || '—')}</strong></td>
                        <td class="text-muted">—</td>
                        <td><span class="badge ${stateCls}">${window.escapeHtml(d.state || '—')}</span></td>
                        <td>${window.escapeHtml(d.sync_state || '—')}</td>
                        <td class="pg-dr-lag-cell">${window.escapeHtml(d.write_lag || '—')} / ${window.escapeHtml(d.flush_lag || '—')} / ${window.escapeHtml(d.replay_lag || '—')}</td>
                        <td>${window.escapeHtml(d.replay_lag || '—')}</td>
                        <td class="text-muted">—</td>
                    </tr>`);
            });
        }

        setRowCount('pg-repl-row-count', rows.length, 'standby');
        tbody.innerHTML = rows.length
            ? rows.join('')
            : pgDrEmptyRow(7, 'fa-tower-broadcast', 'No standbys detected on this instance');
    }

    function updateArchiverFailureTable(failures) {
        const tbody = document.getElementById('archiverFailureTbody');
        if (!tbody) return;
        if (!failures?.length) {
            tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">No failures recorded</td></tr>';
            return;
        }
        tbody.innerHTML = failures.map(f => `
            <tr>
                <td>${new Date(f.collected_at || f.capture_timestamp).toLocaleTimeString()}</td>
                <td class="text-danger font-bold">${f.archive_failed_count}</td>
                <td>${f.last_failed_time ? new Date(f.last_failed_time).toLocaleString() : '—'}</td>
            </tr>
        `).join('');
    }

    function updateSlotsTable(slots) {
        const tbody = document.getElementById('pg-slots-tbody');
        if (!tbody) return;
        const list = Array.isArray(slots) ? slots : [];
        setRowCount('pg-slots-row-count', list.length, 'slot');
        if (!list.length) {
            tbody.innerHTML = pgDrEmptyRow(5, 'fa-plug', 'No replication slots on this instance');
            return;
        }
        tbody.innerHTML = list.map(s => {
            const retained = Number(s.retained_wal_mb || 0);
            const risky = !s.active || retained > 1024;
            const walCls = retained > 1024 ? 'text-danger' : retained > 256 ? 'text-warning' : '';
            return `<tr class="${risky ? 'pg-dr-row-risk' : ''}">
                <td><strong>${window.escapeHtml(s.slot_name || '—')}</strong></td>
                <td><span class="badge badge-outline">${window.escapeHtml(s.slot_type || '—')}</span></td>
                <td>${s.active ? '<span class="badge badge-success">Active</span>' : '<span class="badge badge-danger">Inactive</span>'}</td>
                <td class="${walCls}"><strong>${retained.toFixed(1)}</strong> MB</td>
                <td class="pg-dr-mono"><code>${window.escapeHtml(s.restart_lsn || '—')}</code></td>
            </tr>`;
        }).join('');
    }

    function updateBackupSections(latest, history) {
        const summary = document.getElementById('pg-backup-latest-summary');
        if (summary) {
            if (!latest) {
                summary.innerHTML = `<div class="pg-dr-empty-inline">
                    <i class="fa-solid fa-database"></i>
                    <span>No backup runs logged yet. Report via API or backup agent.</span>
                </div>`;
            } else {
                const ts = latest.capture_timestamp ? new Date(latest.capture_timestamp) : null;
                const age = ts && !isNaN(ts.getTime()) ? ts.toLocaleString() : '—';
                const sizeMb = latest.size_bytes ? (latest.size_bytes / 1024 / 1024).toFixed(1) + ' MB' : '—';
                const ok = String(latest.status || '').toLowerCase() === 'success';
                summary.innerHTML = `
                    <div class="pg-dr-latest-status">
                        <span class="badge ${ok ? 'badge-success' : 'badge-warning'}">${window.escapeHtml(latest.status || '—')}</span>
                        <span class="pg-dr-latest-type">${window.escapeHtml(latest.backup_type || '—')}</span>
                    </div>
                    <dl class="pg-dr-latest-meta">
                        <div><dt>Tool</dt><dd>${window.escapeHtml(latest.tool || '—')}</dd></div>
                        <div><dt>Captured</dt><dd>${age}</dd></div>
                        <div><dt>Size</dt><dd>${sizeMb}</dd></div>
                    </dl>`;
            }
        }
        const tbody = document.getElementById('pg-backup-history-tbody');
        if (!tbody) return;
        const hist = Array.isArray(history) ? history : [];
        setRowCount('pg-backup-history-count', hist.length, 'run');
        if (!hist.length) {
            tbody.innerHTML = pgDrEmptyRow(5, 'fa-clock-rotate-left', 'No backup history in the selected window');
            return;
        }
        tbody.innerHTML = hist.map(h => {
            const ts = h.capture_timestamp ? new Date(h.capture_timestamp) : null;
            const when = ts && !isNaN(ts.getTime()) ? ts.toLocaleString() : '—';
            const ok = String(h.status || '').toLowerCase() === 'success';
            const size = h.size_bytes ? (h.size_bytes / 1024 / 1024).toFixed(1) + ' MB' : '—';
            return `<tr>
                <td class="pg-dr-mono">${when}</td>
                <td>${window.escapeHtml(h.tool || '—')}</td>
                <td>${window.escapeHtml(h.backup_type || '—')}</td>
                <td><span class="badge ${ok ? 'badge-success' : 'badge-warning'}">${window.escapeHtml(h.status || '—')}</span></td>
                <td class="text-right">${size}</td>
            </tr>`;
        }).join('');
    }

    window.PgReplicationView = window.PgBackupsView;
    window.PgBackupDRView = window.PgBackupsView;
})();
