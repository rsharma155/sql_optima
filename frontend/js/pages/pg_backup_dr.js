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
                        <div class="metric-label">Primary</div>
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

                <!-- ROW 3: Failed Events & Replication -->
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
        ['pg-wal-rate-chart', 'pg-archive-status-chart', 'pg-checkpoint-trend-chart'].forEach(id => {
            window.setChartOverlayState(id, 'loading');
        });

        try {
            let url = `/api/pg/backup/dashboard?instance=${encodeURIComponent(instance)}`;
            if (from) url += `&from=${encodeURIComponent(from)}`;
            if (to) url += `&to=${encodeURIComponent(to)}`;

            const resp = await window.apiClient.authenticatedFetch(url);

            if (!resp.ok) {
                const msg = resp.status === 503 ? "TimescaleDB Disconnected" : "Failed to fetch backup data";
                ['pg-wal-rate-chart', 'pg-archive-status-chart', 'pg-checkpoint-trend-chart'].forEach(id => {
                    window.setChartOverlayState(id, 'empty', msg);
                });
                return;
            }
            const data = await resp.json();

            ['pg-wal-rate-chart', 'pg-archive-status-chart', 'pg-checkpoint-trend-chart'].forEach(id => {
                window.clearChartOverlay(id);
            });

            renderKPIs(data.kpis);
            renderWALTrend(data.wal_trend);
            renderArchiveHealth(data.archive_health);
            renderCheckpointTrend(data.checkpoint_trend);
            renderFailedEvents(data.failed_events);
        } catch (err) { 
            console.error(err); 
            ['pg-wal-rate-chart', 'pg-archive-status-chart', 'pg-checkpoint-trend-chart'].forEach(id => {
                window.setChartOverlayState(id, 'empty', "Error loading data");
            });
        }
    }

    function renderKPIs(kpis) {
        if (!kpis) return;
        const setVal = (id, val) => {
            const el = document.getElementById(id);
            if (el) el.textContent = val;
        };
        setVal('kpi-last-archive', kpis.last_archive_age || 'N/A');
        setVal('kpi-wal-rate', (kpis.wal_mb || 0).toFixed(2));
        setVal('kpi-archive-fails', kpis.failed_count || '0');
        setVal('kpi-avg-checkpoint', (kpis.avg_checkpoint_time || 0).toFixed(1) + 's');
        setVal('kpi-is-primary', 'YES');
        setVal('kpi-sync-count', '1');
    }

    function renderWALTrend(trend) {
        const el = document.getElementById('pg-wal-rate-chart');
        if (!el || !trend) return;
        const ctx = el.getContext('2d');
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: trend.map(t => new Date(t.bucket)),
                datasets: [{ label: 'WAL Bytes', data: trend.map(t => t.wal_bytes), borderColor: '#3b82f6', tension: 0.1, fill: false }]
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
                labels: health.map(h => new Date(h.bucket)),
                datasets: [
                    { label: 'Archived', data: health.map(h => h.archived), backgroundColor: '#10b981' },
                    { label: 'Failed', data: health.map(h => h.failed), backgroundColor: '#ef4444' }
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
                labels: trend.map(t => new Date(t.bucket)),
                datasets: [{ label: 'Checkpoint Write Time', data: trend.map(t => t.write_time), borderColor: '#f59e0b', tension: 0.1, fill: true, backgroundColor: 'rgba(245, 158, 11, 0.2)' }]
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
})();
