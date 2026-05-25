/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Backup & Recovery dashboard.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */
(function () {
    'use strict';

    let mssqlBackupCharts = {};

    function fmtTime(t) {
        if (!t) return '—';
        const d = new Date(t);
        return isNaN(d.getTime()) ? '—' : d.toLocaleString();
    }

    function fmtAgeMinutes(m) {
        if (m == null || m >= 999999) return 'Never';
        if (m < 60) return `${m}m`;
        if (m < 1440) return `${Math.floor(m / 60)}h`;
        return `${Math.floor(m / 1440)}d`;
    }

    function badge(ok, labelOk, labelBad) {
        if (ok) return `<span class="mssql-badge ok"><i class="fa-solid fa-circle-check"></i>${labelOk || 'OK'}</span>`;
        return `<span class="mssql-badge bad"><i class="fa-solid fa-circle-xmark"></i>${labelBad || 'Stale'}</span>`;
    }

    function emptyTable(msg, icon) {
        return `<p class="mssql-empty-cell"><i class="fa-solid ${icon || 'fa-inbox'}"></i>${window.escapeHtml(msg)}</p>`;
    }

    function rowClass(r) {
        if (!r.full_fresh_ok) return 'mssql-row-stale';
        const rm = String(r.recovery_model || '').toUpperCase();
        if ((rm === 'FULL' || rm === 'BULK_LOGGED') && !r.log_fresh_ok) return 'mssql-row-warn';
        return '';
    }

    function timeCell(when, ageMinutes, freshOk, isSimple) {
        if (isSimple) {
            return '<span class="text-muted">N/A</span>';
        }
        const isNever = !when || ageMinutes == null || ageMinutes >= 999999;
        if (isNever) {
            return `<div class="mssql-time-cell mssql-time-cell-single">${badge(false, 'Fresh', 'Never')}</div>`;
        }
        const ageLabel = freshOk ? 'Fresh' : fmtAgeMinutes(ageMinutes);
        return `<div class="mssql-time-cell">
  <span>${fmtTime(when)}</span>
  ${badge(freshOk, 'Fresh', ageLabel)}
</div>`;
    }

    function chartOpts(extra) {
        const base = window.ChartOptions?.lineChart?.() || {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: true, labels: { color: '#94a3b8', font: { size: 9 } } } },
            scales: {
                x: { ticks: { color: '#94a3b8', font: { size: 8 }, maxTicksLimit: 8 }, grid: { display: false } },
                y: { beginAtZero: true, ticks: { color: '#94a3b8', font: { size: 9 } }, grid: { color: 'rgba(148,163,184,0.1)' } },
            },
        };
        return { ...base, ...extra };
    }

    window.SqlServerBackupsView = async function () {
        const inst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
        if (!inst || String(inst.type || '').toLowerCase() !== 'sqlserver') {
            window.routerOutlet.innerHTML =
                '<div class="page-view active"><div class="alert alert-warning">Select a SQL Server instance.</div></div>';
            return;
        }

        window.appState.activeViewId = 'sqlserver-backups';
        window.appState.mssqlBackups = window.appState.mssqlBackups || {};
        const state = window.appState.mssqlBackups;
        const now = new Date();
        const oneDayAgo = new Date(now.getTime() - 86400000);
        const pad = n => String(n).padStart(2, '0');
        const fmtLocal = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
        state.fromLocal = state.fromLocal || fmtLocal(oneDayAgo);
        state.toLocal = state.toLocal || fmtLocal(now);

        window.routerOutlet.innerHTML = await window.loadTemplate('/pages/sqlserver_backups.html', { inst });

        const fromInput = document.getElementById('mssql-backup-from');
        const toInput = document.getElementById('mssql-backup-to');
        if (fromInput) fromInput.value = state.fromLocal;
        if (toInput) toInput.value = state.toLocal;

        document.getElementById('mssql-backup-apply')?.addEventListener('click', () => {
            state.fromLocal = fromInput?.value || state.fromLocal;
            state.toLocal = toInput?.value || state.toLocal;
            refreshSqlServerBackups(inst.name);
        });

        document.getElementById('mssql-goto-ha')?.addEventListener('click', () => {
            if (typeof window.navigateTo === 'function') window.navigateTo('drilldown-ha');
        });

        document.querySelectorAll('.mssql-backup-tab').forEach(btn => {
            btn.addEventListener('click', () => switchBackupTab(btn.dataset.tab));
        });

        document.getElementById('mssql-backup-policy-form')?.addEventListener('submit', async (e) => {
            e.preventDefault();
            await saveBackupPolicy(inst.name);
        });

        initMssqlBackupCharts();
        await refreshSqlServerBackups(inst.name);
        await loadBackupPolicy(inst.name);

        if (window.mssqlBackupsInterval) clearInterval(window.mssqlBackupsInterval);
        window.mssqlBackupsInterval = window.registerInterval(() => {
            if (window.appState.activeViewId === 'sqlserver-backups') {
                refreshSqlServerBackups(inst.name);
            }
        }, 60000);
    };

    function switchBackupTab(tab) {
        document.querySelectorAll('.mssql-backup-tab').forEach(b => {
            b.classList.toggle('active', b.dataset.tab === tab);
        });
        ['history', 'jobs', 'logship', 'policy'].forEach(id => {
            const el = document.getElementById(`mssql-tab-${id}`);
            if (el) el.style.display = id === tab ? '' : 'none';
        });
    }

    async function refreshSqlServerBackups(instanceName) {
        const refreshEl = document.getElementById('mssql-backup-refresh-time');
        if (refreshEl) refreshEl.textContent = new Date().toLocaleTimeString();

        const state = window.appState.mssqlBackups || {};
        let from = state.fromLocal;
        let to = state.toLocal;
        if (from && from.includes('T')) from = new Date(from).toISOString();
        if (to && to.includes('T')) to = new Date(to).toISOString();

        const q = `instance=${encodeURIComponent(instanceName)}`;
        const range = (from && to) ? `&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}` : '';

        try {
            const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/backup/dashboard?${q}${range}`);
            if (!resp.ok) {
                console.error('[MssqlBackups] dashboard', resp.status);
                return;
            }
            const data = await resp.json();
            renderDashboard(data);
        } catch (e) {
            console.error('[MssqlBackups] refresh failed', e);
        }
    }

    function renderDashboard(data) {
        renderReadiness(data.readiness, data.ha_context);
        renderKPIs(data.kpis || {});
        renderPostureTable(data.posture || []);
        renderHistoryTable(data.history || []);
        renderJobsPanel(data.backup_jobs || {});
        renderLogShippingTable(data.log_shipping || []);
        updateVolumeChart(data.history_trend || []);
        updateCountChart(data.history_trend || []);

        const cfg = data.instance_config || {};
        const comp = document.getElementById('mssql-kpi-compress');
        if (comp) comp.textContent = cfg.backup_compression_default ? 'On' : 'Off';
    }

    function renderReadiness(readiness, haCtx) {
        const row = document.getElementById('mssql-backup-readiness-row');
        const overall = document.getElementById('mssql-backup-overall');
        const gotoHa = document.getElementById('mssql-goto-ha');
        if (!row) return;

        const chips = readiness?.chips || [];
        row.innerHTML = chips.map(c =>
            `<span class="mssql-backup-readiness-chip ${c.class}">${window.escapeHtml(c.label)}</span>`
        ).join('');

        if (overall) {
            const o = readiness?.overall || 'collecting';
            overall.textContent = o.replace(/_/g, ' ');
            overall.className = 'mssql-dr-overall small ' + (
                o.includes('attention') ? 'text-danger' :
                    o.includes('collecting') ? 'text-warning' : 'text-success'
            );
        }

        const ha = haCtx || {};
        if (gotoHa) {
            gotoHa.style.display = (ha.ha_enabled || ha.ag_enabled || ha.log_shipping_enabled) ? '' : 'none';
        }
    }

    function renderKPIs(kpis) {
        const set = (id, v) => { const el = document.getElementById(id); if (el) el.textContent = v; };
        set('mssql-kpi-databases', kpis.databases_tracked ?? '—');
        set('mssql-kpi-stale', kpis.stale_full ?? '—');
        set('mssql-kpi-log', kpis.overdue_log ?? '—');
        set('mssql-kpi-jobs', kpis.failed_backup_jobs_24h ?? '—');
        set('mssql-kpi-gb', kpis.backup_gb_24h != null ? String(kpis.backup_gb_24h) : '—');
        set('mssql-kpi-duration', kpis.avg_backup_duration_sec ?? '—');
    }

    function renderPostureTable(rows) {
        const host = document.getElementById('mssql-posture-table');
        const countEl = document.getElementById('mssql-posture-count');
        if (!host) return;
        if (countEl) countEl.textContent = rows.length ? `${rows.length} database(s)` : '—';
        if (!rows.length) {
            host.innerHTML = emptyTable('No database posture yet. Collector runs every 5 minutes (or use Refresh).', 'fa-database');
            return;
        }
        const sorted = [...rows].sort((a, b) => (b.minutes_since_full || 0) - (a.minutes_since_full || 0));
        const tr = sorted.map(r => {
            const rm = (r.recovery_model || '').replace(/_/g, ' ');
            const isSimple = rm.toUpperCase().includes('SIMPLE');
            const prot = r.protection_level
                ? `<span class="mssql-badge tier">${window.escapeHtml(r.protection_level)}</span>`
                : '<span class="text-muted">—</span>';
            const sizeFull = r.last_full_size_mb > 0 ? `${r.last_full_size_mb.toFixed(0)} MB` : '—';
            return `<tr class="${rowClass(r)}">
  <td class="col-db">${window.escapeHtml(r.database_name || '')}</td>
  <td>${window.escapeHtml(rm)}</td>
  <td class="col-status">${timeCell(r.last_full_finish, r.minutes_since_full, r.full_fresh_ok, false)}</td>
  <td class="col-status">${fmtTime(r.last_diff_finish)}</td>
  <td class="col-status">${timeCell(r.last_log_finish, r.minutes_since_log, r.log_fresh_ok, isSimple)}</td>
  <td class="col-num">${sizeFull}</td>
  <td class="col-status">${prot}</td>
</tr>`;
        }).join('');
        host.innerHTML = `<table class="modern-table modern-table-compact mssql-grid-table"><thead><tr>
  <th>Database</th><th>Recovery</th><th>Last full</th><th>Last diff</th><th>Last log</th><th>Size</th><th>Protection</th>
</tr></thead><tbody>${tr}</tbody></table>`;
    }

    function renderHistoryTable(rows) {
        const host = document.getElementById('mssql-backup-history-table');
        if (!host) return;
        if (!rows.length) {
            host.innerHTML = emptyTable('No backup history in the selected time range.', 'fa-clock-rotate-left');
            return;
        }
        const typeLabel = t => ({ D: 'Full', I: 'Diff', L: 'Log' }[t] || t);
        const tr = rows.slice(0, 100).map(r => `<tr>
  <td class="col-status">${fmtTime(r.backup_finish_date)}</td>
  <td class="col-db">${window.escapeHtml(r.database_name || '')}</td>
  <td><span class="mssql-badge type-${r.backup_type}">${typeLabel(r.backup_type)}</span></td>
  <td class="col-num">${(r.backup_size_mb || 0).toFixed(1)}</td>
  <td class="col-num">${(r.compressed_backup_size_mb || 0).toFixed(1)}</td>
  <td class="col-num">${r.duration_seconds ?? '—'}s</td>
  <td class="col-status">${r.is_compressed ? '<i class="fa-solid fa-check text-success"></i>' : '—'}</td>
</tr>`).join('');
        host.innerHTML = `<table class="modern-table modern-table-compact mssql-grid-table"><thead><tr>
  <th>Finished</th><th>Database</th><th>Type</th><th>Size (MB)</th><th>Compressed (MB)</th><th>Duration</th><th>Compressed</th>
</tr></thead><tbody>${tr}</tbody></table>`;
    }

    function renderJobsPanel(jobsData) {
        const host = document.getElementById('mssql-backup-jobs-table');
        if (!host) return;
        const jobs = jobsData.jobs || [];
        const failures = jobsData.failures || [];
        if (!jobs.length && !failures.length) {
            host.innerHTML = emptyTable('No backup or maintenance Agent jobs in the latest snapshot.', 'fa-briefcase');
            return;
        }
        let html = '<div class="mssql-jobs-stack">';
        if (failures.length) {
            html += '<p class="text-muted small" style="margin:0.35rem 0.5rem 0.25rem;font-weight:600;">Recent failures (24h)</p>';
            html += '<table class="modern-table modern-table-compact mssql-grid-table"><thead><tr><th>Job</th><th>When</th></tr></thead><tbody>';
            html += failures.map(f => `<tr class="mssql-row-stale">
  <td class="col-db">${window.escapeHtml(f.job_name || '')}</td>
  <td class="col-status">${fmtTime(f.run_dt)}</td>
</tr>`).join('');
            html += '</tbody></table>';
        }
        if (jobs.length) {
            html += '<p class="text-muted small" style="margin:0.75rem 0.5rem 0.25rem;font-weight:600;">Backup / maintenance jobs</p>';
            html += '<table class="modern-table modern-table-compact mssql-grid-table"><thead><tr><th>Job</th><th>Category</th><th>Enabled</th><th>Last status</th></tr></thead><tbody>';
            html += jobs.map(j => {
                const st = String(j.last_run_status || '').toLowerCase();
                const rowCls = st === 'failed' ? 'mssql-row-stale' : '';
                return `<tr class="${rowCls}">
  <td class="col-db">${window.escapeHtml(j.job_name || '')}</td>
  <td>${window.escapeHtml(j.job_category || '')}</td>
  <td class="col-status">${j.job_enabled ? '<span class="mssql-badge ok">Yes</span>' : '<span class="mssql-badge warn">No</span>'}</td>
  <td>${window.escapeHtml(j.last_run_status || '—')}</td>
</tr>`;
            }).join('');
            html += '</tbody></table>';
        }
        html += '</div>';
        host.innerHTML = html;
    }

    function renderLogShippingTable(rows) {
        const host = document.getElementById('mssql-logshipping-table');
        if (!host) return;
        if (!rows.length) {
            host.innerHTML = emptyTable('Log shipping not configured or no recent health data.', 'fa-truck-fast');
            return;
        }
        const tr = rows.map(r => {
            const delay = +(r.restore_delay_minutes ?? 0);
            const thresh = +(r.restore_threshold_minutes ?? 0);
            const behind = thresh > 0 && delay > thresh;
            return `<tr class="${behind ? 'mssql-row-stale' : ''}">
  <td>${r.is_primary ? '<span class="mssql-badge tier">Primary</span>' : '<span class="mssql-badge tier">Secondary</span>'}</td>
  <td class="col-db">${window.escapeHtml(r.primary_database || '')}${r.secondary_database ? ' <span class="text-muted">→</span> ' + window.escapeHtml(r.secondary_database) : ''}</td>
  <td class="col-status">${fmtTime(r.last_backup_date)}</td>
  <td class="col-status">${fmtTime(r.last_restore_date)}</td>
  <td class="col-num ${behind ? 'text-danger' : ''}">${delay}m <span class="text-muted">/ ${thresh || '—'}m</span></td>
</tr>`;
        }).join('');
        host.innerHTML = `<table class="modern-table modern-table-compact mssql-grid-table"><thead><tr>
  <th>Role</th><th>Database</th><th>Last backup</th><th>Last restore</th><th>Delay / threshold</th>
</tr></thead><tbody>${tr}</tbody></table>`;
    }

    function initMssqlBackupCharts() {
        if (typeof window.ChartBootstrap === 'undefined') return;
        const vol = document.getElementById('mssql-backup-volume-chart');
        const cnt = document.getElementById('mssql-backup-count-chart');
        if (vol) mssqlBackupCharts.volume = window.ChartBootstrap.create(vol, 'line', chartOpts({}));
        if (cnt) mssqlBackupCharts.count = window.ChartBootstrap.create(cnt, 'bar', chartOpts({ plugins: { legend: { display: false } } }));
    }

    function trendLabels(trend) {
        return trend.map(p => {
            const t = p.timestamp || p.bucket;
            if (!t) return '';
            return new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        });
    }

    function updateVolumeChart(trend) {
        const ch = mssqlBackupCharts.volume;
        if (!ch || !trend?.length) return;
        ch.data.labels = trendLabels(trend);
        ch.data.datasets = [
            { label: 'Full', data: trend.map(p => +(p.full_mb ?? 0)), borderColor: '#4ade80', backgroundColor: 'rgba(74,222,128,0.2)', fill: true, tension: 0.3 },
            { label: 'Diff', data: trend.map(p => +(p.diff_mb ?? 0)), borderColor: '#60a5fa', backgroundColor: 'rgba(96,165,250,0.15)', fill: true, tension: 0.3 },
            { label: 'Log', data: trend.map(p => +(p.log_mb ?? 0)), borderColor: '#fbbf24', backgroundColor: 'rgba(251,191,36,0.15)', fill: true, tension: 0.3 },
        ];
        ch.update();
    }

    function updateCountChart(trend) {
        const ch = mssqlBackupCharts.count;
        if (!ch || !trend?.length) return;
        ch.data.labels = trendLabels(trend);
        ch.data.datasets = [{
            label: 'Count',
            data: trend.map(p => +(p.backup_count ?? 0)),
            backgroundColor: 'rgba(96,165,250,0.5)',
        }];
        ch.update();
    }

    async function loadBackupPolicy(instanceName) {
        const full = document.getElementById('mssql-policy-full-hours');
        const log = document.getElementById('mssql-policy-log-minutes');
        if (full && !full.value) full.value = 24;
        if (log && !log.value) log.value = 15;

        const q = `instance=${encodeURIComponent(instanceName)}`;
        try {
            const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/backup/policy?${q}`);
            if (!resp.ok) {
                return;
            }
            const p = await resp.json();
            if (full) full.value = p.rpo_full_backup_hours ?? 24;
            if (log) log.value = p.rpo_log_backup_minutes ?? 15;
        } catch (_) {
            /* defaults already set */
        }
    }

    async function saveBackupPolicy(instanceName) {
        const status = document.getElementById('mssql-policy-status');
        const body = {
            rpo_full_backup_hours: parseInt(document.getElementById('mssql-policy-full-hours')?.value, 10) || 24,
            rpo_log_backup_minutes: parseInt(document.getElementById('mssql-policy-log-minutes')?.value, 10) || 15,
        };
        const q = `instance=${encodeURIComponent(instanceName)}`;
        try {
            const resp = await window.apiClient.authenticatedFetch(`/api/sqlserver/backup/policy?${q}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });
            if (status) status.textContent = resp.ok ? 'Saved.' : 'Save failed.';
            if (resp.ok) await refreshSqlServerBackups(instanceName);
        } catch (e) {
            if (status) status.textContent = 'Error saving policy.';
        }
    }
})();
