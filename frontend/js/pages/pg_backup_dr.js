/*
 * SQL Optima — Backup & Disaster Recovery
 */
(function() {
    window.PgBackupDRView = function() {
        const instance = window.appState.config?.instances?.[window.appState.currentInstanceIdx]?.name;
        if (!instance) return;

        window.appState.activeViewId = 'pg-backup-dr';
        
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title flex-between dashboard-page-title-compact">
                    <div class="dashboard-title-line">
                        <h1><i class="fa-solid fa-shield-heart"></i> Backup & DR</h1>
                        <span class="subtitle">WAL archiving, replication health, and RPO monitoring</span>
                    </div>
                    <div class="flex-center">
                        <div id="time-picker-insertion-point"></div>
                    </div>
                </div>

                <!-- ROW 1: Compact Metric Strip -->
                <div class="metrics-row-compact">
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Archive Age</div>
                        <div class="metric-value" id="kpi-last-archive">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">WAL Gen (MB/min)</div>
                        <div class="metric-value text-accent" id="kpi-wal-rate">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Failures</div>
                        <div class="metric-value text-danger" id="kpi-archive-fails">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Avg Checkpoint</div>
                        <div class="metric-value" id="kpi-avg-checkpoint">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Replica Lag</div>
                        <div class="metric-value" id="kpi-repl-lag">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Node Role</div>
                        <div class="metric-value text-success" id="kpi-is-primary">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Sync Standbys</div>
                        <div class="metric-value" id="kpi-sync-count">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">RPO Target</div>
                        <div class="metric-value" id="kpi-rpo-target">5m</div>
                    </div>
                </div>

                <!-- Replica notice (hidden by default, shown when node is a replica) -->
                <div id="ha-replica-notice" class="alert alert-info mt-1" style="display:none; font-size:0.8rem; padding:0.5rem 0.75rem;">
                    <i class="fa-solid fa-info-circle"></i>
                    <strong>This is a replica node.</strong> Replication topology and standby list are only visible from the <strong>primary</strong>. Switch to the primary instance to see the full HA cluster view.
                </div>

                <!-- HA Cluster Members (shown for multi-instance PG setups) -->
                <div class="card glass-panel mt-1" id="ha-cluster-card">
                    <div class="card-header" style="padding:0.4rem 0.75rem;">
                        <h3 style="font-size:0.75rem; margin:0;"><i class="fa-solid fa-network-wired text-accent"></i> HA Cluster Members</h3>
                    </div>
                    <div style="padding:0.35rem 0.5rem;">
                        <table class="modern-table modern-table-compact" style="font-size:0.72rem;">
                            <thead><tr><th>Instance</th><th>Host</th><th>Role</th></tr></thead>
                            <tbody id="ha-cluster-members">
                                <tr><td colspan="3" class="text-center text-muted">Loading...</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>

                <!-- ROW 2: WAL Trend & Archive Health -->
                <div class="chart-row-compact">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">WAL Generation Rate</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-wal-rate-chart"></canvas>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Archive Success vs Failures</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-archive-status-chart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- ROW 3: Failed Events & Checkpoint -->
                <div class="chart-row-compact">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Failed Archive Audit</h3></div>
                        <div class="table-container-compact">
                            <table class="modern-table modern-table-compact" id="pg-failed-archives-table">
                                <thead><tr><th>Time</th><th>Failed</th><th>Last Error</th></tr></thead>
                                <tbody></tbody>
                            </table>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Checkpoint Write Time Trend</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-checkpoint-trend-chart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- ROW 4: Replication Lag Trend & Topology -->
                <div class="chart-row-compact">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Replication Lag Trend</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-replication-lag-chart"></canvas>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Standby Topology</h3></div>
                        <div class="table-container-compact">
                            <table class="modern-table modern-table-compact">
                                <thead><tr><th>Replica</th><th>State</th><th>Sync</th><th>Write Lag</th><th>Replay Lag</th><th>Retained</th></tr></thead>
                                <tbody id="pg-replication-topology-tbody"></tbody>
                            </table>
                        </div>
                    </div>
                </div>
            </div>
        `;

        window.initPageTimePicker();
        fetchData(instance);
    };

    async function fetchData(instance) {
        let from = window.appState.fromTs;
        let to = window.appState.toTs;
        if (from && from.includes('T') && !from.endsWith('Z')) from = new Date(from).toISOString();
        if (to && to.includes('T') && !to.endsWith('Z')) to = new Date(to).toISOString();

        // Show loading state
        ['pg-wal-rate-chart', 'pg-archive-status-chart', 'pg-checkpoint-trend-chart', 'pg-replication-lag-chart'].forEach(id => {
            window.setChartOverlayState(id, 'loading');
        });

        try {
            let url = `/api/pg/backup/dashboard?instance=${encodeURIComponent(instance)}`;
            if (from) url += `&from=${encodeURIComponent(from)}`;
            if (to) url += `&to=${encodeURIComponent(to)}`;

            const resp = await window.apiClient.authenticatedFetch(url);

            if (!resp.ok) {
                const msg = resp.status === 503 ? "TimescaleDB Disconnected" : "Failed to fetch backup data";
                ['pg-wal-rate-chart', 'pg-archive-status-chart', 'pg-checkpoint-trend-chart', 'pg-replication-lag-chart'].forEach(id => {
                    window.setChartOverlayState(id, 'empty', msg);
                });
                return;
            }
            const data = await resp.json();

            ['pg-wal-rate-chart', 'pg-archive-status-chart', 'pg-checkpoint-trend-chart', 'pg-replication-lag-chart'].forEach(id => {
                window.clearChartOverlay(id);
            });

            renderKPIs(data.kpis, data.replication_details);
            renderWALTrend(data.wal_trend);
            renderArchiveHealth(data.archive_health);
            renderCheckpointTrend(data.checkpoint_trend);
            renderReplicationLagTrend(data.lag_trend);
            renderReplicationTopology(data.replication_details);
            renderFailedEvents(data.failed_events);
        } catch (err) { 
            console.error(err); 
            ['pg-wal-rate-chart', 'pg-archive-status-chart', 'pg-checkpoint-trend-chart', 'pg-replication-lag-chart'].forEach(id => {
                window.setChartOverlayState(id, 'empty', "Error loading data");
            });
        }
    }

    function renderKPIs(kpis, replDetails) {
        if (!kpis) return;
        const setVal = (id, val) => {
            const el = document.getElementById(id);
            if (el) el.textContent = val;
        };
        setVal('kpi-last-archive', kpis.last_archive_age || 'N/A');
        setVal('kpi-wal-rate', (kpis.wal_generation_rate_mb_min || 0).toFixed(2));
        setVal('kpi-archive-fails', kpis.failed_count || '0');
        setVal('kpi-avg-checkpoint', (kpis.checkpoint_avg_write_time || 0).toFixed(1) + 'ms');
        const lagSec = kpis.replica_max_lag_seconds || 0;
        setVal('kpi-repl-lag', lagSec > 0 ? (lagSec < 60 ? lagSec.toFixed(1) + 's' : (lagSec / 60).toFixed(1) + 'm') : '0s');

        const replArr = replDetails || [];
        setVal('kpi-sync-count', replArr.filter(r => r.sync_state === 'sync').length || replArr.length || '0');

        // Use node_role from API (primary/replica/standalone)
        const role = (kpis.node_role || '').toUpperCase() ||
            (replArr.length > 0 ? 'PRIMARY' : (kpis.is_in_recovery ? 'REPLICA' : 'STANDALONE'));
        const roleEl = document.getElementById('kpi-is-primary');
        if (roleEl) {
            roleEl.textContent = role;
            roleEl.className = `metric-value ${role === 'PRIMARY' ? 'text-success' : role === 'REPLICA' ? 'text-accent' : 'text-muted'}`;
        }

        // Show/hide replica notice
        const replicaNotice = document.getElementById('ha-replica-notice');
        if (replicaNotice) {
            replicaNotice.style.display = (role === 'REPLICA') ? 'block' : 'none';
        }

        // Render cluster members
        renderClusterMembers(role);
    }

    function renderClusterMembers(currentRole) {
        const container = document.getElementById('ha-cluster-members');
        if (!container) return;
        const instances = (window.appState.config?.instances || []).filter(i => String(i.type || '').toLowerCase() === 'postgres');
        if (instances.length <= 1) {
            container.closest('.card')?.setAttribute('style', 'display:none');
            return;
        }
        const currentName = (window.appState.config?.instances || [])[window.appState.currentInstanceIdx]?.name || '';
        container.innerHTML = instances.map(inst => {
            const isCurrent = inst.name === currentName;
            const role = isCurrent ? currentRole : '–';
            const badge = isCurrent
                ? (currentRole === 'PRIMARY' ? 'badge-success' : currentRole === 'REPLICA' ? 'badge-info' : 'badge-secondary')
                : 'badge-secondary';
            return `<tr>
                <td><strong>${inst.name}</strong>${isCurrent ? ' <span class="badge badge-outline" style="font-size:0.6rem;">viewing</span>' : ''}</td>
                <td>${inst.host || ''}:${inst.port || 5432}</td>
                <td><span class="badge ${badge}">${role}</span></td>
            </tr>`;
        }).join('');
    }

    function renderWALTrend(trend) {
        const el = document.getElementById('pg-wal-rate-chart');
        if (!el || !trend) return;
        const ctx = el.getContext('2d');
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: trend.map(t => new Date(t.collected_at)),
                datasets: [{ label: 'WAL Bytes Delta', data: trend.map(t => t.wal_bytes_delta), borderColor: '#3b82f6', tension: 0.1, fill: false }]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { x: { type: 'time', time: { unit: 'hour' }, display: false } }
            }
        });
    }

    function renderArchiveHealth(health) {
        const el = document.getElementById('pg-archive-status-chart');
        if (!el || !health) return;
        const ctx = el.getContext('2d');
        new Chart(ctx, {
            type: 'bar',
            data: {
                labels: health.map(h => new Date(h.collected_at)),
                datasets: [
                    { label: 'Archived', data: health.map(h => h.archived_count), backgroundColor: '#10b981' },
                    { label: 'Failed', data: health.map(h => h.archive_failed_count), backgroundColor: '#ef4444' }
                ]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { 
                    x: { stacked: true, type: 'time', time: { unit: 'hour' }, display: true, title: { display: true, text: 'Timeline', font: { size: 10 } } }, 
                    y: { stacked: true, title: { display: true, text: 'WAL Files', font: { size: 10 } } } 
                }
            }
        });
    }

    function renderCheckpointTrend(trend) {
        const el = document.getElementById('pg-checkpoint-trend-chart');
        if (!el || !trend) return;
        const ctx = el.getContext('2d');
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: trend.map(t => new Date(t.collected_at)),
                datasets: [{ label: 'Checkpoint Write Time (ms)', data: trend.map(t => t.checkpoint_write_time_ms), borderColor: '#f59e0b', tension: 0.1, fill: true, backgroundColor: 'rgba(245, 158, 11, 0.2)' }]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { 
                    x: { type: 'time', time: { unit: 'hour' }, display: true, title: { display: true, text: 'Timeline', font: { size: 10 } } },
                    y: { title: { display: true, text: 'Write Time (s)', font: { size: 10 } } }
                }
            }
        });
    }

    function renderFailedEvents(events) {
        const tbody = document.querySelector('#pg-failed-archives-table tbody');
        if (!tbody) return;
        if (!events || events.length === 0) {
            tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">None</td></tr>';
            return;
        }
        tbody.innerHTML = events.slice(0, 4).map(e => `
            <tr>
                <td>${new Date(e.ts).toLocaleTimeString()}</td>
                <td class="text-danger">${e.failed_count}</td>
                <td>${new Date(e.last_failed_time).toLocaleTimeString()}</td>
            </tr>
        `).join('');
    }

    function renderReplicationLagTrend(trend) {
        const el = document.getElementById('pg-replication-lag-chart');
        if (!el || !trend || trend.length === 0) {
            if (el) window.setChartOverlayState('pg-replication-lag-chart', 'empty', 'No replication lag data');
            return;
        }
        const byReplica = {};
        trend.forEach(p => {
            const k = p.application_name || 'replica';
            if (!byReplica[k]) byReplica[k] = [];
            byReplica[k].push({ x: new Date(p.capture_timestamp), y: p.lag_seconds || 0 });
        });
        const colors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444'];
        const datasets = Object.entries(byReplica).map(([name, pts], i) => ({
            label: name,
            data: pts,
            borderColor: colors[i % colors.length],
            tension: 0.1,
            fill: false,
            pointRadius: 0,
        }));
        const ctx = el.getContext('2d');
        new Chart(ctx, {
            type: 'line',
            data: { datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: true, labels: { font: { size: 10 }, color: '#94a3b8' } } },
                scales: {
                    x: { type: 'time', time: { unit: 'hour' }, display: true, title: { display: true, text: 'Timeline', font: { size: 10 } } },
                    y: { title: { display: true, text: 'Lag (s)', font: { size: 10 } }, beginAtZero: true }
                }
            }
        });
    }

    function fmtLagSec(v) {
        const n = Number(v);
        if (!v || isNaN(n) || n === 0) return '0s';
        if (n < 1) return n.toFixed(2) + 's';
        if (n < 60) return n.toFixed(1) + 's';
        return (n / 60).toFixed(1) + 'm';
    }

    function renderReplicationTopology(details) {
        const el = document.getElementById('pg-replication-topology-tbody');
        if (!el) return;
        if (!details || details.length === 0) {
            el.innerHTML = '<tr><td colspan="6" class="text-center text-muted">No replication standbys detected. This node may be a replica — view the primary for full topology.</td></tr>';
            return;
        }
        const esc = v => String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
        el.innerHTML = details.map(r => {
            const writeLag = r.write_lag_sec ?? r.write_lag;
            const replayLag = r.replay_lag_sec ?? r.replay_lag;
            const lagHigh = Number(replayLag) > 5;
            return `<tr>
                <td><strong>${esc(r.application_name)}</strong></td>
                <td><span class="badge ${r.state === 'streaming' ? 'badge-success' : 'badge-warning'}">${esc(r.state)}</span></td>
                <td>${esc(r.sync_state)}</td>
                <td class="${Number(writeLag) > 0 ? 'text-warning' : ''}">${fmtLagSec(writeLag)}</td>
                <td class="${lagHigh ? 'text-danger' : Number(replayLag) > 0 ? 'text-warning' : ''}">${fmtLagSec(replayLag)}</td>
                <td>${r.retained_mb ? Number(r.retained_mb).toFixed(1) + ' MB' : '—'}</td>
            </tr>`;
        }).join('');
    }
})();
