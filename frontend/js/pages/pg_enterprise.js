/*
 * SQL Optima — PostgreSQL enterprise metrics dashboard.
 */

window.PgEnterpriseDashboardView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) { alert('Select an instance first.'); return; }
    if (inst.type !== 'postgres') { alert('Enterprise monitoring is for PostgreSQL only.'); return; }
    const dbName = window.appState.currentDatabase || 'all';

    window.appState.activeViewId = 'drilldown-pg-enterprise';

    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_enterprise.html', { inst, dbName });
    window.initPageTimePicker();

    if (window.appState.fetchingPgEnterpriseMetrics) return;
    window.appState.fetchingPgEnterpriseMetrics = true;

    try {
        const from = window.appState.fromTs;
        const to = window.appState.toTs;
        await Promise.all([
            loadBGWriterData(inst.name, from, to),
            loadArchiverData(inst.name, from, to),
            loadWaitEvents(inst.name, from, to),
            loadDbIO(inst.name, from, to),
            loadConfigDrift(inst.name),
            loadQueryInternals(inst.name, from, to),
            updateEnterpriseHeader(inst.name)
        ]);
    } finally {
        window.appState.fetchingPgEnterpriseMetrics = false;
    }

    if (window.pgEnterpriseInterval) clearInterval(window.pgEnterpriseInterval);
    window.pgEnterpriseInterval = window.registerInterval(() => {
        if (window.appState.activeViewId === 'drilldown-pg-enterprise') {
            window.PgEnterpriseDashboardView();
        } else {
            clearInterval(window.pgEnterpriseInterval);
        }
    }, 30000);
};

async function updateEnterpriseHeader(instName) {
    try {
        const r = await window.apiClient.authenticatedFetch(`/api/postgres/server-info?instance=${encodeURIComponent(instName)}`);
        if (!r.ok) return;
        const s = await r.json() || {};
        const hs = s.health_score || 0;
        const healthColor = hs > 80 ? 'success' : hs > 60 ? 'warning' : 'danger';
        const hBadge = document.getElementById('pgHealthScoreBadge');
        if (hBadge) {
            hBadge.textContent = hs;
            hBadge.style.color = `var(--${healthColor})`;
        }
    } catch (e) { console.error('PG enterprise header fetch failed:', e); }
}

// --- BGWriter / Checkpoint ---

function loadBGWriterData(instanceName, from, to) {
    const section = document.getElementById('bgwriter-section');
    if (!section) return;

    let url = `/api/postgres/bgwriter?instance=${encodeURIComponent(instanceName)}`;
    if (from && to) url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;

    return window.apiClient.authenticatedFetch(url)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            const stats = (data && data.stats) ? data.stats : [];
            if (stats.length === 0) {
                section.innerHTML = '<div class="alert alert-info">No BGWriter data available yet. Collector may still be warming up.</div>';
                return;
            }

            const latest = stats[0];
            const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
            set('stat-ckpt-timed', fmtNum(latest.checkpoints_timed || 0));
            set('stat-ckpt-req', fmtNum(latest.checkpoints_req || 0));
            set('stat-bgw-halts', fmtNum(latest.maxwritten_clean || 0));

            if ((latest.checkpoints_req || 0) > (latest.checkpoints_timed || 0)) {
                document.getElementById('card-ckpt-req')?.classList.add('border-warning');
            }
            if ((latest.maxwritten_clean || 0) > 0) {
                document.getElementById('pgBgwHaltsCard')?.classList.add('border-warning');
            }

            // Compute per-interval deltas on cumulative counters (ascending order)
            const asc = stats.slice().reverse();
            const labels = asc.map(s => new Date(s.time).toLocaleTimeString());
            const deltaOrZero = (arr, key) => arr.map((r, i) => i === 0 ? 0 : Math.max(0, (r[key] || 0) - (arr[i-1][key] || 0)));
            const timedDelta = deltaOrZero(asc, 'checkpoints_timed');
            const reqDelta = deltaOrZero(asc, 'checkpoints_req');
            const writeTimes = asc.map(s => Number(s.checkpoint_write_time || 0).toFixed(0));
            const syncTimes = asc.map(s => Number(s.checkpoint_sync_time || 0).toFixed(0));

            // Cache hit ratio from IO data if available
            const cacheHitEl = document.getElementById('stat-cache-hit');
            if (cacheHitEl && window._pgLatestIOStats) {
                const total = (window._pgLatestIOStats.blks_hit || 0) + (window._pgLatestIOStats.blks_read || 0);
                const ratio = total > 0 ? ((window._pgLatestIOStats.blks_hit / total) * 100).toFixed(1) : '--';
                cacheHitEl.textContent = ratio + '%';
            }

            section.innerHTML = `
                <div class="grid-container">
                    <div class="col-8 col-tablet-12">
                        <div class="flex-between mb-1" style="font-size:0.7rem; color:var(--text-muted);">
                            <span><strong>Checkpoint Frequency</strong> — deltas per collection interval</span>
                            <span id="bgw-ckpt-label"></span>
                        </div>
                        <div style="font-size:0.65rem; color:var(--text-muted); margin-bottom:4px; line-height:1.4;">
                            <span style="color:#3b82f6;">&#9632;</span> <strong>Timed</strong> — scheduled by <code>checkpoint_timeout</code> (default 5 min). Normal, expected activity — steady rate is healthy.
                            &nbsp;&nbsp;
                            <span style="color:#f59e0b;">&#9632;</span> <strong>Req</strong> — forced by WAL pressure (<code>max_wal_size</code> reached). High or rising values indicate heavy write load; consider increasing <code>max_wal_size</code>.
                        </div>
                        <div style="height:160px;"><canvas id="pgBgwriterChart"></canvas></div>
                        <div class="flex-between mt-2 mb-1" style="font-size:0.7rem; color:var(--text-muted);">
                            <span><strong>Checkpoint I/O Duration</strong></span>
                        </div>
                        <div style="font-size:0.65rem; color:var(--text-muted); margin-bottom:4px; line-height:1.4;">
                            <span style="color:#8b5cf6;">&#9632;</span> <strong>Write ms</strong> — time to write dirty pages from the buffer pool to disk. High values = I/O-bound storage.
                            &nbsp;&nbsp;
                            <span style="color:#10b981;">&#9632;</span> <strong>Sync ms</strong> — time spent on <code>fsync</code> to make data durable. Long sync = slow disk or large checkpoint; may cause I/O stalls for application queries.
                        </div>
                        <div style="height:110px;"><canvas id="pgCkptTimeChart"></canvas></div>
                    </div>
                    <div class="col-4 col-tablet-12 table-container-compact" style="max-height:330px; overflow-y:auto;">
                        <table class="modern-table modern-table-compact">
                            <thead><tr>
                                <th>Time</th>
                                <th title="Timed: scheduled at checkpoint_timeout interval">Timed</th>
                                <th title="Req: forced by WAL fill (max_wal_size) — investigate if nonzero">Req ⚠</th>
                                <th title="Time spent writing dirty pages to disk (ms)">Write ms</th>
                                <th title="Time spent on fsync to ensure durability (ms)">Sync ms</th>
                            </tr></thead>
                            <tbody>${asc.slice().reverse().slice(0, 15).map(s => `
                                <tr>
                                    <td>${new Date(s.time).toLocaleTimeString()}</td>
                                    <td>${s.checkpoints_timed}</td>
                                    <td class="${(s.checkpoints_req||0)>0?'text-warning':''}">${s.checkpoints_req}</td>
                                    <td>${Number(s.checkpoint_write_time||0).toFixed(0)}</td>
                                    <td>${Number(s.checkpoint_sync_time||0).toFixed(0)}</td>
                                </tr>`).join('')}
                            </tbody>
                        </table>
                    </div>
                </div>
            `;

            const chartDefaults = { responsive: true, maintainAspectRatio: false, plugins: { legend: { labels: { color: 'var(--text)', font: { size: 10 } } } }, scales: { x: { ticks: { color: 'var(--text-muted)', font: { size: 9 }, maxTicksLimit: 8 }, grid: { color: 'rgba(255,255,255,0.05)' } }, y: { ticks: { color: 'var(--text-muted)', font: { size: 9 } }, grid: { color: 'rgba(255,255,255,0.05)' }, beginAtZero: true } } };

            const c1 = document.getElementById('pgBgwriterChart');
            Chart.getChart(c1)?.destroy();
            new Chart(c1.getContext('2d'), {
                type: 'bar',
                data: { labels, datasets: [
                    { label: 'Timed Δ', data: timedDelta, backgroundColor: 'rgba(59,130,246,0.6)', borderColor: '#3b82f6', borderWidth: 1 },
                    { label: 'Req Δ', data: reqDelta, backgroundColor: 'rgba(245,158,11,0.6)', borderColor: '#f59e0b', borderWidth: 1 }
                ]},
                options: { ...chartDefaults, scales: { ...chartDefaults.scales, x: { ...chartDefaults.scales.x, stacked: false }, y: { ...chartDefaults.scales.y, stacked: false } } }
            });

            const c2 = document.getElementById('pgCkptTimeChart');
            Chart.getChart(c2)?.destroy();
            new Chart(c2.getContext('2d'), {
                type: 'line',
                data: { labels, datasets: [
                    { label: 'Write ms', data: writeTimes, borderColor: '#8b5cf6', backgroundColor: 'rgba(139,92,246,0.1)', fill: true, tension: 0.3, pointRadius: 0 },
                    { label: 'Sync ms', data: syncTimes, borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.1)', fill: true, tension: 0.3, pointRadius: 0 }
                ]},
                options: chartDefaults
            });
        })
        .catch(e => {
            if (section) section.innerHTML = `<div class="alert alert-warning">BGWriter data unavailable: ${e.message}</div>`;
        });
}

// --- WAL Archiver ---

function loadArchiverData(instanceName, from, to) {
    const section = document.getElementById('archiver-section');
    if (!section) return;

    let url = `/api/postgres/archiver?instance=${encodeURIComponent(instanceName)}`;
    if (from && to) url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;

    return window.apiClient.authenticatedFetch(url)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            const stats = (data && data.stats) ? data.stats : [];
            if (stats.length === 0) {
                section.innerHTML = '<div class="alert alert-info">No Archiver data. WAL archiving may not be enabled on this instance.</div>';
                return;
            }
            const latest = stats[0];
            const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
            set('stat-archived-wals', fmtNum(latest.archived_count || 0));
            set('stat-archiving-failures', fmtNum(latest.failed_count || 0));

            if ((latest.failed_count || 0) > 0) {
                document.getElementById('card-archiving-failures')?.classList.add('border-danger');
            }

            // Compute per-interval archive deltas
            const asc = stats.slice().reverse();
            const labels = asc.map(s => new Date(s.time || s.timestamp).toLocaleTimeString());
            const archivedDelta = asc.map((s, i) => i === 0 ? 0 : Math.max(0, (s.archived_count || 0) - (asc[i-1].archived_count || 0)));

            section.innerHTML = `
                <div style="height:130px; margin-bottom:0.5rem;"><canvas id="pgArchiverChart"></canvas></div>
                <div class="table-container-compact" style="max-height:160px; overflow-y:auto;">
                    <table class="modern-table modern-table-compact">
                        <thead><tr><th>Time</th><th>Archived</th><th>Failed</th><th>Last Fail WAL</th></tr></thead>
                        <tbody>${stats.slice(0, 10).map(s => `
                            <tr>
                                <td>${new Date(s.time || s.timestamp).toLocaleTimeString()}</td>
                                <td>${s.archived_count}</td>
                                <td class="${(s.failed_count||0)>0?'text-danger':''}">${s.failed_count}</td>
                                <td style="font-size:0.65rem;">${s.last_failed_wal || '-'}</td>
                            </tr>`).join('')}
                        </tbody>
                    </table>
                </div>
            `;

            const c = document.getElementById('pgArchiverChart');
            Chart.getChart(c)?.destroy();
            new Chart(c.getContext('2d'), {
                type: 'bar',
                data: { labels, datasets: [
                    { label: 'WALs Archived (Δ)', data: archivedDelta, backgroundColor: 'rgba(16,185,129,0.6)', borderColor: '#10b981', borderWidth: 1 }
                ]},
                options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { labels: { color: 'var(--text)', font: { size: 10 } } } }, scales: { x: { ticks: { color: 'var(--text-muted)', font: { size: 9 }, maxTicksLimit: 8 }, grid: { color: 'rgba(255,255,255,0.05)' } }, y: { ticks: { color: 'var(--text-muted)', font: { size: 9 } }, grid: { color: 'rgba(255,255,255,0.05)' }, beginAtZero: true } } }
            });
        })
        .catch(e => {
            if (section) section.innerHTML = `<div class="alert alert-warning">Archiver data unavailable: ${e.message}</div>`;
        });
}

// --- Contention: Wait Events ---

function loadWaitEvents(instanceName, from, to) {
    const section = document.getElementById('waits-section');
    if (!section) return;

    let url = `/api/postgres/waits/history?instance=${encodeURIComponent(instanceName)}`;
    if (from && to) url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;

    return window.apiClient.authenticatedFetch(url)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            const rows = (data && data.rows) ? data.rows : (Array.isArray(data) ? data : []);
            if (!rows.length) {
                section.innerHTML = '<div class="text-muted p-4">No wait event data in selected range.</div>';
                return;
            }

            // Aggregate by timestamp bucket and wait_event_type
            const byTs = new Map();
            rows.forEach(r => {
                const ts = r.capture_timestamp || r.timestamp;
                if (!byTs.has(ts)) byTs.set(ts, {});
                const evtType = r.wait_event_type || 'Other';
                byTs.get(ts)[evtType] = (byTs.get(ts)[evtType] || 0) + (r.sessions_count || 0);
            });

            const sortedTs = Array.from(byTs.keys()).sort();
            const labels = sortedTs.map(t => new Date(t).toLocaleTimeString());
            const waitTypes = ['Client', 'CPU', 'IO', 'Lock', 'LWLock', 'BufferPin', 'IPC'];
            const colors = { Client: '#6b7280', CPU: '#10b981', IO: '#3b82f6', Lock: '#ef4444', LWLock: '#f59e0b', BufferPin: '#8b5cf6', IPC: '#ec4899' };

            // Only include types that actually have data
            const activeTypes = waitTypes.filter(t => rows.some(r => (r.wait_event_type || 'Other') === t));

            const datasets = activeTypes.map(t => ({
                label: t,
                data: sortedTs.map(ts => byTs.get(ts)[t] || 0),
                backgroundColor: (colors[t] || '#94a3b8') + 'CC',
                borderColor: colors[t] || '#94a3b8',
                borderWidth: 1,
                fill: true,
                tension: 0.3,
                pointRadius: 0
            }));

            // Summary table: top wait types in range
            const typeSums = {};
            rows.forEach(r => {
                const t = r.wait_event_type || 'Other';
                typeSums[t] = (typeSums[t] || 0) + (r.sessions_count || 0);
            });
            const topTypes = Object.entries(typeSums).sort((a,b) => b[1]-a[1]).slice(0, 6);

            section.innerHTML = `
                <div class="grid-container">
                    <div class="col-8 col-tablet-12"><div style="height:200px;"><canvas id="pgWaitsChartAdv"></canvas></div></div>
                    <div class="col-4 col-tablet-12">
                        <table class="modern-table modern-table-compact" style="font-size:0.7rem;">
                            <thead><tr><th>Wait Type</th><th>Sessions</th></tr></thead>
                            <tbody>${topTypes.map(([t, n]) => `<tr><td>${t}</td><td>${fmtNum(n)}</td></tr>`).join('')}</tbody>
                        </table>
                    </div>
                </div>
            `;

            const c = document.getElementById('pgWaitsChartAdv');
            Chart.getChart(c)?.destroy();
            new Chart(c.getContext('2d'), {
                type: 'line',
                data: { labels, datasets },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: { legend: { labels: { color: 'var(--text)', font: { size: 9 } } } },
                    scales: {
                        x: { ticks: { color: 'var(--text-muted)', font: { size: 9 }, maxTicksLimit: 8 }, grid: { color: 'rgba(255,255,255,0.05)' }, stacked: true },
                        y: { ticks: { color: 'var(--text-muted)', font: { size: 9 } }, grid: { color: 'rgba(255,255,255,0.05)' }, beginAtZero: true, stacked: true }
                    }
                }
            });
        })
        .catch(e => {
            if (section) section.innerHTML = `<div class="alert alert-warning">Wait event data unavailable: ${e.message}</div>`;
        });
}

// --- Database I/O & Temp Spill ---

function loadDbIO(instanceName, from, to) {
    const section = document.getElementById('io-section');
    if (!section) return;

    let url = `/api/postgres/io/history?instance=${encodeURIComponent(instanceName)}`;
    if (from && to) url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;

    return window.apiClient.authenticatedFetch(url)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            // Backend returns {"rows": [...]} now
            const rows = (data && data.rows) ? data.rows : (data && data.stats) ? data.stats : [];
            if (!rows.length) {
                section.innerHTML = '<div class="text-muted p-4">No I/O data in selected range.</div>';
                return;
            }

            // Compute aggregate cache hit ratio from latest snapshot
            const latestByDb = {};
            rows.forEach(r => {
                const db = r.database_name || 'unknown';
                if (!latestByDb[db] || new Date(r.capture_timestamp) > new Date(latestByDb[db].capture_timestamp)) {
                    latestByDb[db] = r;
                }
            });
            const totHit = Object.values(latestByDb).reduce((s, r) => s + (r.blks_hit || 0), 0);
            const totRead = Object.values(latestByDb).reduce((s, r) => s + (r.blks_read || 0), 0);
            const cacheHitPct = (totHit + totRead) > 0 ? ((totHit / (totHit + totRead)) * 100).toFixed(1) : null;

            // Update cache hit KPI card
            const cacheEl = document.getElementById('stat-cache-hit');
            if (cacheEl && cacheHitPct !== null) {
                cacheEl.textContent = cacheHitPct + '%';
                const color = Number(cacheHitPct) > 99 ? 'success' : Number(cacheHitPct) > 95 ? 'warning' : 'danger';
                cacheEl.style.color = `var(--${color})`;
                const card = document.getElementById('card-cache-hit');
                if (card && Number(cacheHitPct) < 95) card.classList.add('border-warning');
            }
            window._pgLatestIOStats = { blks_hit: totHit, blks_read: totRead };

            const dbs = Array.from(new Set(rows.map(r => r.database_name))).sort();
            const latestDb = dbs[0];

            section.innerHTML = `
                <select id="pgIoDbSel" class="form-select mb-2" style="font-size:0.7rem; padding:2px 6px;">${dbs.map(d => `<option value="${d}">${d}</option>`).join('')}</select>
                <div style="height:155px;"><canvas id="pgIoChartAdv"></canvas></div>
            `;

            const render = (db) => {
                const series = rows.filter(r => r.database_name === db).sort((a,b) => new Date(a.capture_timestamp) - new Date(b.capture_timestamp));
                const lbls = series.map(s => new Date(s.capture_timestamp).toLocaleTimeString());
                const deltaPos = (arr, k) => arr.map((r, i) => i === 0 ? 0 : Math.max(0, (r[k] || 0) - (arr[i-1][k] || 0)));
                const readsDelta = deltaPos(series, 'blks_read');
                const tempMB = deltaPos(series, 'temp_bytes').map(v => +(v / 1048576).toFixed(2));

                const c = document.getElementById('pgIoChartAdv');
                Chart.getChart(c)?.destroy();
                new Chart(c.getContext('2d'), {
                    type: 'line',
                    data: { labels: lbls, datasets: [
                        { label: 'Disk Reads (Δ)', data: readsDelta, borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.1)', fill: true, tension: 0.3, pointRadius: 0, yAxisID: 'y' },
                        { label: 'Temp (Δ MB)', data: tempMB, borderColor: '#ef4444', backgroundColor: 'rgba(239,68,68,0.1)', fill: true, tension: 0.3, pointRadius: 0, yAxisID: 'y2' }
                    ]},
                    options: {
                        responsive: true, maintainAspectRatio: false,
                        plugins: { legend: { labels: { color: 'var(--text)', font: { size: 9 } } } },
                        scales: {
                            x: { ticks: { color: 'var(--text-muted)', font: { size: 9 }, maxTicksLimit: 8 }, grid: { color: 'rgba(255,255,255,0.05)' } },
                            y: { position: 'left', ticks: { color: '#3b82f6', font: { size: 9 } }, grid: { color: 'rgba(255,255,255,0.05)' }, beginAtZero: true, title: { display: true, text: 'Reads', color: '#3b82f6', font: { size: 9 } } },
                            y2: { position: 'right', ticks: { color: '#ef4444', font: { size: 9 } }, grid: { drawOnChartArea: false }, beginAtZero: true, title: { display: true, text: 'Temp MB', color: '#ef4444', font: { size: 9 } } }
                        }
                    }
                });
            };

            document.getElementById('pgIoDbSel').addEventListener('change', e => render(e.target.value));
            render(latestDb);
        })
        .catch(e => {
            if (section) section.innerHTML = `<div class="alert alert-warning">I/O data unavailable: ${e.message}</div>`;
        });
}

// --- Config Drift ---

function loadConfigDrift(instanceName) {
    const section = document.getElementById('drift-section');
    if (!section) return;

    return window.apiClient.authenticatedFetch(`/api/postgres/settings/drift?instance=${encodeURIComponent(instanceName)}`)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            const changes = (data && data.changes) ? data.changes : [];
            if (!changes.length) {
                section.innerHTML = '<div class="text-muted p-4 text-center"><i class="fa-solid fa-check-circle text-success"></i> No configuration changes detected in last 7 days.</div>';
                return;
            }
            section.innerHTML = `
                <div class="table-container-compact" style="max-height:200px; overflow-y:auto;">
                    <table class="modern-table modern-table-compact" style="font-size:0.7rem;">
                        <thead><tr><th>Setting</th><th>Previous</th><th>Current</th></tr></thead>
                        <tbody>${changes.map(c => `
                            <tr>
                                <td><strong>${window.escapeHtml(c.name)}</strong></td>
                                <td class="text-muted">${window.escapeHtml(String(c.old_value))}</td>
                                <td class="text-accent">${window.escapeHtml(String(c.new_value))}</td>
                            </tr>`).join('')}
                        </tbody>
                    </table>
                </div>
            `;
        })
        .catch(e => {
            if (section) section.innerHTML = `<div class="alert alert-warning">Config drift unavailable: ${e.message}</div>`;
        });
}

// --- High-Impact Internal Queries (pg_stat_statements via pgss_delta_1m) ---

function loadQueryInternals(instanceName, from, to) {
    const section = document.getElementById('qint-section');
    if (!section) return;

    let url = `/api/postgres/queries?instance=${encodeURIComponent(instanceName)}`;
    if (from && to) url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;

    return window.apiClient.authenticatedFetch(url)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            const qs = (data && data.queries) ? data.queries : [];
            if (!qs.length) {
                section.innerHTML = '<div class="text-muted p-4">No query data. Ensure <code>pg_stat_statements</code> is enabled and the PGSS pipeline is active.</div>';
                return;
            }

            // Save to state for sorting
            window.appState.pgInternalQueriesRaw = qs;
            window.appState.pgIntSortCol = window.appState.pgIntSortCol || 'total_time';
            window.appState.pgIntSortDir = window.appState.pgIntSortDir || 'desc';

            section.innerHTML = `
                <div class="table-container-compact" style="max-height:380px; overflow-y:auto;">
                    <table class="modern-table modern-table-compact" style="font-size:0.7rem;" id="pgInternalQueriesTable">
                        <thead>
                            <tr>
                                <th class="sortable" data-col="username">User</th>
                                <th class="sortable" data-col="calls" title="Number of executions in window">Calls</th>
                                <th class="sortable" data-col="total_time" title="Total execution time (ms)">Total ms</th>
                                <th class="sortable" data-col="mean_time" title="Average execution time per call">Avg ms</th>
                                <th class="sortable" data-col="temp_blks_written" title="Temporary disk blocks written — high values indicate work_mem pressure">Temp Writes</th>
                                <th>SQL</th>
                            </tr>
                        </thead>
                        <tbody id="pgInternalQueriesTbody"></tbody>
                    </table>
                </div>
            `;

            renderInternalQueriesTable();

            // Wire up sorting
            section.querySelectorAll('th.sortable').forEach(th => {
                th.style.cursor = 'pointer';
                th.addEventListener('click', () => {
                    const col = th.dataset.col;
                    if (window.appState.pgIntSortCol === col) {
                        window.appState.pgIntSortDir = window.appState.pgIntSortDir === 'asc' ? 'desc' : 'asc';
                    } else {
                        window.appState.pgIntSortCol = col;
                        window.appState.pgIntSortDir = 'desc';
                    }
                    renderInternalQueriesTable();
                });
            });
        })
        .catch(e => {
            if (section) section.innerHTML = `<div class="alert alert-warning">Query data unavailable: ${e.message}</div>`;
        });
}

function renderInternalQueriesTable() {
    const tbody = document.getElementById('pgInternalQueriesTbody');
    if (!tbody) return;

    let qs = [...(window.appState.pgInternalQueriesRaw || [])];
    const col = window.appState.pgIntSortCol;
    const dir = window.appState.pgIntSortDir === 'asc' ? 1 : -1;

    qs.sort((a, b) => {
        const va = a[col] ?? 0;
        const vb = b[col] ?? 0;
        if (typeof va === 'string') return va.localeCompare(vb) * dir;
        return (va - vb) * dir;
    });

    const top = qs.slice(0, 20);
    window.appState.pgInternalQueries = top.map(q => q.query);

    // Update header icons
    const table = document.getElementById('pgInternalQueriesTable');
    if (table) {
        table.querySelectorAll('th.sortable').forEach(th => {
            th.innerHTML = th.innerHTML.split(' <i')[0]; // strip old icon
            if (th.dataset.col === col) {
                th.innerHTML += ` <i class="fa-solid fa-sort-${window.appState.pgIntSortDir === 'asc' ? 'up' : 'down'} small ml-1"></i>`;
            }
        });
    }

    tbody.innerHTML = top.map((q, idx) => `
        <tr>
            <td>${window.escapeHtml(q.username || '-')}</td>
            <td>${fmtNum(q.calls)}</td>
            <td class="${col==='total_time'?'text-accent font-bold':''}">${Number(q.total_time || 0).toFixed(0)}</td>
            <td>${Number(q.mean_time || 0).toFixed(1)}</td>
            <td class="${(q.temp_blks_written||0)>0?'text-warning':''}">${fmtNum(q.temp_blks_written)}</td>
            <td class="small text-muted" style="cursor:pointer; text-decoration:underline; max-width:280px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;"
                data-action="call" data-fn="showPgInternalQueryDetail" data-idx="${idx}"
                title="${window.escapeHtml((q.query || '').substring(0, 200))}">
                ${window.escapeHtml((q.query || '').substring(0, 70))}${(q.query||'').length > 70 ? '…' : ''}
            </td>
        </tr>`).join('');
}

// --- Query Detail Modal ---

window.showPgInternalQueryDetail = function(idx) {
    const message = (window.appState.pgInternalQueries || [])[idx];
    if (!message) return;
    const existingModal = document.getElementById('pg-query-modal');
    if (existingModal) existingModal.remove();

    const modal = document.createElement('div');
    modal.id = 'pg-query-modal';
    modal.style.cssText = 'display:flex; position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.8); align-items:center; justify-content:center;';

    modal.innerHTML = `
        <div class="glass-panel" style="background:var(--bg-surface); margin:2%; padding:20px; border:1px solid var(--border-color); border-radius:12px; width:95%; max-width:900px; max-height:85vh; display:flex; flex-direction:column; box-shadow:0 4px 50px rgba(0,0,0,0.5);">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:1rem; border-bottom:1px solid var(--border-color); padding-bottom:0.75rem;">
                <h3 style="margin:0; color:var(--accent); font-size:1.1rem;"><i class="fa-solid fa-code"></i> Internal Query Detail</h3>
                <button style="background:transparent; border:none; color:var(--text); font-size:1.5rem; cursor:pointer;" data-action="close-id" data-target="pg-query-modal">&times;</button>
            </div>
            <div style="flex:1; overflow:auto; background:rgba(0,0,0,0.3); padding:1.25rem; border-radius:8px; border:1px solid var(--border-color);">
                <pre style="margin:0; white-space:pre-wrap; word-wrap:break-word; color:var(--text); font-family:'Fira Code', 'Courier New', monospace; font-size:0.85rem; line-height:1.6;">${window.escapeHtml(message)}</pre>
            </div>
            <div style="text-align:right; margin-top:1.25rem;">
                <button id="copyPgQueryBtn" class="btn btn-sm btn-accent" style="padding:0.5rem 1.5rem;">
                    <i class="fa-solid fa-copy"></i> Copy SQL
                </button>
                <button class="btn btn-sm btn-outline ml-2" data-action="close-id" data-target="pg-query-modal">Close</button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);

    document.getElementById('copyPgQueryBtn').addEventListener('click', function() {
        navigator.clipboard.writeText(message).then(() => {
            this.innerHTML = '<i class="fa-solid fa-check"></i> Copied!';
            setTimeout(() => { this.innerHTML = '<i class="fa-solid fa-copy"></i> Copy SQL'; }, 1500);
        });
    });

    modal.addEventListener('click', e => { if (e.target === modal) modal.remove(); });
};

function fmtNum(num) { return Number(num || 0).toLocaleString(); }
