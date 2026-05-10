/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Redesigned PostgreSQL Backup & DR operational cockpit JS logic.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgBackupsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || { name: 'Loading...', type: 'postgres' };
    const dbName = window.appState.currentDatabase || 'all';

    window.appState.activeViewId = 'pg-backups';

    // 1. Initial Shell
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_backups.html', { inst, dbName });
    
    // Initialize state if not present
    window.appState.pgBackups = window.appState.pgBackups || {};
    const state = window.appState.pgBackups;
    
    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
    const pad = n => String(n).padStart(2, '0');
    const fmtLocal = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;

    state.fromLocal = state.fromLocal || fmtLocal(oneHourAgo);
    state.toLocal = state.toLocal || fmtLocal(now);

    const fromInput = document.getElementById('pgBackupFrom');
    const toInput = document.getElementById('pgBackupTo');
    if (fromInput) fromInput.value = state.fromLocal;
    if (toInput) toInput.value = state.toLocal;

    document.getElementById('pgBackupApply')?.addEventListener('click', () => {
        state.fromLocal = fromInput.value;
        state.toLocal = toInput.value;
        refreshPgBackupData(inst.name);
    });

    // Initialize charts (placeholders)
    initPgBackupCharts();

    // 2. Initial Fetch
    await refreshPgBackupData(inst.name);

    // 3. Set Refresh Interval
    if (window.pgBackupsInterval) clearInterval(window.pgBackupsInterval);
    window.pgBackupsInterval = setInterval(() => {
        if (window.appState.activeViewId === 'pg-backups') {
            refreshPgBackupData(inst.name);
        } else {
            clearInterval(window.pgBackupsInterval);
        }
    }, 30000); // 30s refresh for operational cockpit
};

let pgBackupCharts = {};

function initPgBackupCharts() {
    const commonOptions = {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: {
            x: { 
                display: true,
                grid: { display: false },
                ticks: { color: '#94a3b8', font: { size: 8 }, maxTicksLimit: 8 },
                title: { display: true, text: 'Time', color: '#94a3b8', font: { size: 10, weight: 'bold' } }
            },
            y: { 
                beginAtZero: true,
                grid: { color: 'rgba(148,163,184,0.1)' },
                ticks: { color: '#94a3b8', font: { size: 9 } }
            }
        }
    };

    const walCtx = document.getElementById('walTrendChart')?.getContext('2d');
    if (walCtx) {
        pgBackupCharts.wal = new Chart(walCtx, {
            type: 'line',
            data: { labels: [], datasets: [{ label: 'WAL MB', data: [], borderColor: '#4cc9f0', backgroundColor: 'rgba(76, 201, 240, 0.1)', fill: true, tension: 0.3 }] },
            options: {
                ...commonOptions,
                scales: {
                    ...commonOptions.scales,
                    y: { ...commonOptions.scales.y, title: { display: true, text: 'MB / min', color: '#4cc9f0', font: { size: 10, weight: 'bold' } } }
                }
            }
        });
    }

    const lagCtx = document.getElementById('replicaLagChart')?.getContext('2d');
    if (lagCtx) {
        pgBackupCharts.lag = new Chart(lagCtx, {
            type: 'line',
            data: { labels: [], datasets: [] },
            options: { 
                ...commonOptions, 
                plugins: { 
                    legend: { display: true, position: 'bottom', labels: { boxWidth: 10, font: { size: 8 }, color: '#ccc' } },
                    noDataMessage: { display: false, text: 'No replicas detected' }
                },
                scales: {
                    ...commonOptions.scales,
                    y: { ...commonOptions.scales.y, title: { display: true, text: 'Seconds', color: 'rgba(255,255,255,0.4)', font: { size: 9 } } }
                }
            },
            plugins: [{
                id: 'noDataMessage',
                afterDraw: (chart) => {
                    if (chart.data.datasets.length === 0) {
                        const { ctx, width, height } = chart;
                        ctx.save();
                        ctx.textAlign = 'center';
                        ctx.textBaseline = 'middle';
                        ctx.font = '12px sans-serif';
                        ctx.fillStyle = 'rgba(255,255,255,0.5)';
                        ctx.fillText('No replicas detected', width / 2, height / 2);
                        ctx.restore();
                    }
                }
            }]
        });
    }

    const archCtx = document.getElementById('archiveHealthChart')?.getContext('2d');
    if (archCtx) {
        pgBackupCharts.archive = new Chart(archCtx, {
            type: 'bar',
            data: { labels: [], datasets: [
                { label: 'Success', data: [], backgroundColor: '#4ade80' },
                { label: 'Failed', data: [], backgroundColor: '#f87171' }
            ]},
            options: { 
                ...commonOptions, 
                scales: { 
                    ...commonOptions.scales, 
                    x: { ...commonOptions.scales.x, stacked: true }, 
                    y: { ...commonOptions.scales.y, stacked: true, title: { display: true, text: 'Count', color: 'rgba(255,255,255,0.4)', font: { size: 9 } } } 
                } 
            }
        });
    }

    const ckptCtx = document.getElementById('checkpointTrendChart')?.getContext('2d');
    if (ckptCtx) {
        pgBackupCharts.checkpoint = new Chart(ckptCtx, {
            type: 'line',
            data: { labels: [], datasets: [
                { label: 'Write', data: [], borderColor: '#f472b6', tension: 0.3 },
                { label: 'Sync', data: [], borderColor: '#818cf8', tension: 0.3 }
            ]},
            options: { 
                ...commonOptions, 
                plugins: { legend: { display: true, position: 'bottom', labels: { boxWidth: 10, font: { size: 8 }, color: '#ccc' } } },
                scales: {
                    ...commonOptions.scales,
                    y: { ...commonOptions.scales.y, title: { display: true, text: 'MS Latency', color: '#94a3b8', font: { size: 10, weight: 'bold' } } }
                }
            }
        });
    }
}

async function refreshPgBackupData(instName) {
    if (document.getElementById('pgLastRefreshTime')) 
        document.getElementById('pgLastRefreshTime').textContent = new Date().toLocaleTimeString();

    const state = window.appState.pgBackups || {};
    let from = state.fromLocal;
    let to = state.toLocal;

    // Ensure ISO format for API
    if (from && from.includes('T')) from = new Date(from).toISOString();
    if (to && to.includes('T')) to = new Date(to).toISOString();

    try {
        let url = `/api/pg/backup/dashboard?instance=${encodeURIComponent(instName)}`;
        if (from) url += `&from=${encodeURIComponent(from)}`;
        if (to) url += `&to=${encodeURIComponent(to)}`;

        const resp = await window.apiClient.authenticatedFetch(url);
        if (!resp.ok) return;
        const data = await resp.json();

        updateKPIs(data.kpis);
        updateWALChart(data.wal_trend);
        updateLagChart(data.replication_lag_trend);
        updateArchiveChart(data.archive_health);
        updateCheckpointChart(data.checkpoint_trend);
        updateReplicationTable(data.replication_details);
        updateArchiverFailureTable(data.archiver_failures);

    } catch (e) {
        console.error("Failed to refresh PG Backup data:", e);
    }
}

function updateKPIs(kpis) {
    if (!kpis) return;
    const set = (id, val) => {
        const el = document.getElementById(id);
        if (el) el.textContent = val;
    };

    set('stat-wal-rate', kpis.wal_generation_rate_mb_min?.toFixed(2) || '0.00');
    
    const arcSucc = kpis.archive_success_percent;
    const arcEl = document.getElementById('stat-archive-success');
    if (arcEl) {
        arcEl.textContent = arcSucc?.toFixed(1) || '100';
        arcEl.className = 'kpi-value ' + (arcSucc < 99 ? 'text-danger' : 'text-success');
    }

    const maxLag = kpis.replica_max_lag_seconds;
    const lagEl = document.getElementById('stat-max-lag');
    if (lagEl) {
        lagEl.textContent = maxLag?.toFixed(0) || '0';
        lagEl.className = 'kpi-value ' + (maxLag > 60 ? 'text-danger' : 'text-success');
    }

    set('stat-slots-risk', kpis.replication_slots_risk_gb?.toFixed(2) || '0.00');
    set('stat-archive-age', kpis.last_archive_age || 'N/A');
    set('stat-avg-checkpoint', kpis.checkpoint_avg_write_time?.toFixed(0) || '0');
}

function updateWALChart(trend) {
    if (!trend || !pgBackupCharts.wal) return;
    pgBackupCharts.wal.data.labels = trend.map(d => new Date(d.collected_at).toLocaleTimeString());
    pgBackupCharts.wal.data.datasets[0].data = trend.map(d => (d.wal_bytes_delta / 1024 / 1024).toFixed(2));
    pgBackupCharts.wal.update('none');
}

function updateLagChart(trend) {
    if (!trend || !pgBackupCharts.lag) return;
    
    // Group by application_name
    const groups = {};
    const labels = [...new Set(trend.map(d => new Date(d.collected_at).toLocaleTimeString()))];
    
    trend.forEach(d => {
        if (!groups[d.application_name]) groups[d.application_name] = [];
        groups[d.application_name].push({ x: new Date(d.collected_at).toLocaleTimeString(), y: d.lag_seconds });
    });

    const colors = ['#4cc9f0', '#4361ee', '#3a0ca3', '#7209b7', '#f72585'];
    pgBackupCharts.lag.data.labels = labels;
    pgBackupCharts.lag.data.datasets = Object.keys(groups).map((name, i) => ({
        label: name,
        data: groups[name].map(pt => pt.y),
        borderColor: colors[i % colors.length],
        tension: 0.3,
        pointRadius: 0
    }));
    pgBackupCharts.lag.update('none');
}

function updateArchiveChart(health) {
    if (!health || !pgBackupCharts.archive) return;
    pgBackupCharts.archive.data.labels = health.map(d => new Date(d.collected_at).toLocaleTimeString());
    pgBackupCharts.archive.data.datasets[0].data = health.map(d => d.archived_count);
    pgBackupCharts.archive.data.datasets[1].data = health.map(d => d.archive_failed_count);
    pgBackupCharts.archive.update('none');
}

function updateCheckpointChart(trend) {
    if (!trend || !pgBackupCharts.checkpoint) return;
    pgBackupCharts.checkpoint.data.labels = trend.map(d => new Date(d.collected_at).toLocaleTimeString());
    pgBackupCharts.checkpoint.data.datasets[0].data = trend.map(d => d.checkpoint_write_time_ms);
    pgBackupCharts.checkpoint.data.datasets[1].data = trend.map(d => d.checkpoint_sync_time_ms);
    pgBackupCharts.checkpoint.update('none');
}

function updateReplicationTable(details) {
    const tbody = document.getElementById('replicationTbody');
    if (!tbody) return;
    if (!details || details.length === 0) {
        tbody.innerHTML = '<tr><td colspan="5" class="text-center text-muted">No replicas detected</td></tr>';
        return;
    }

    tbody.innerHTML = details.map(d => `
        <tr>
            <td><span class="text-accent font-bold">${window.escapeHtml(d.application_name)}</span></td>
            <td><span class="badge ${d.state === 'streaming' ? 'badge-success' : 'badge-warning'}">${window.escapeHtml(d.state)}</span></td>
            <td>${window.escapeHtml(d.sync_state)}</td>
            <td title="Write / Flush / Replay">${window.escapeHtml(d.write_lag)} / ${window.escapeHtml(d.flush_lag)} / ${window.escapeHtml(d.replay_lag)}</td>
            <td>${Number(d.retained_mb).toLocaleString()} MB</td>
        </tr>
    `).join('');
}

function updateArchiverFailureTable(failures) {
    const tbody = document.getElementById('archiverFailureTbody');
    if (!tbody) return;
    if (!failures || failures.length === 0) {
        tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">No failures recorded</td></tr>';
        return;
    }

    tbody.innerHTML = failures.map(f => `
        <tr>
            <td>${new Date(f.collected_at).toLocaleTimeString()}</td>
            <td class="text-danger font-bold">${f.archive_failed_count}</td>
            <td>${new Date(f.last_failed_time).toLocaleString()}</td>
        </tr>
    `).join('');
}
