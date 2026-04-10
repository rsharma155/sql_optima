/**
 * PostgreSQL Enterprise Dashboard View
 * 
 * This page displays PostgreSQL enterprise metrics collected from TimescaleDB:
 * - BGWriter/Checkpoint statistics
 * - WAL Archiver statistics
 * 
 * Data is collected every 15 minutes by the background collector and stored
 * in TimescaleDB for historical analysis.
 */
window.PgEnterpriseDashboardView = function() {
    const instance = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!instance) { alert('Select an instance first.'); return; }
    if (instance.type !== 'postgres') { alert('Enterprise monitoring is for PostgreSQL only.'); return; }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
        <div class="page-title flex-between dashboard-page-title-compact">
            <div class="dashboard-title-line" style="flex:1; min-width:0;">
                <h1>Advanced Enterprise Monitor</h1>
                <span class="subtitle">Raw/enterprise Timescale-backed metrics (drilldown)</span>
            </div>
            <div class="flex-between dashboard-page-title-actions" style="align-items:center; gap:0.75rem; flex-wrap:wrap; justify-content:flex-end;">
                <button class="btn btn-sm btn-outline" onclick="window.appNavigate('pg-dashboard')"><i class="fa-solid fa-arrow-left"></i> Back</button>
                <button class="btn btn-sm btn-outline text-accent" onclick="window.PgEnterpriseDashboardView()"><i class="fa-solid fa-refresh"></i> Refresh</button>
            </div>
        </div>

        <div class="dashboard-grid" style="display: grid; gap: var(--spacing-md); margin-top:0.75rem;">
            <!-- BGWriter/Checkpoint Statistics Card -->
            <div class="card">
                <div class="card-header">
                    <h3><i class="fa-solid fa-database"></i> BGWriter / Checkpoint Statistics</h3>
                </div>
                <div class="card-body">
                    <div id="bgwriter-section">
                        <div class="loading-spinner"><i class="fa-solid fa-spinner fa-spin"></i> Loading BGWriter data...</div>
                    </div>
                </div>
            </div>

            <!-- WAL Archiver Statistics Card -->
            <div class="card">
                <div class="card-header">
                    <h3><i class="fa-solid fa-archive"></i> WAL Archiver Statistics</h3>
                </div>
                <div class="card-body">
                    <div id="archiver-section">
                        <div class="loading-spinner"><i class="fa-solid fa-spinner fa-spin"></i> Loading Archiver data...</div>
                    </div>
                </div>
            </div>

            <!-- Contention: Wait Events -->
            <div class="card">
                <div class="card-header">
                    <h3><i class="fa-solid fa-road-barrier"></i> Contention: Wait Events (history)</h3>
                </div>
                <div class="card-body">
                    <div id="waits-section">
                        <div class="loading-spinner"><i class="fa-solid fa-spinner fa-spin"></i> Loading wait events...</div>
                    </div>
                </div>
            </div>

            <!-- IO: per DB IO/temp -->
            <div class="card">
                <div class="card-header">
                    <h3><i class="fa-solid fa-hard-drive"></i> IO: Per-DB Reads / Temp Spill (history)</h3>
                </div>
                <div class="card-body">
                    <div id="io-section">
                        <div class="loading-spinner"><i class="fa-solid fa-spinner fa-spin"></i> Loading IO stats...</div>
                    </div>
                </div>
            </div>

            <!-- Config drift -->
            <div class="card">
                <div class="card-header">
                    <h3><i class="fa-solid fa-sliders"></i> Config Drift (latest vs previous snapshot)</h3>
                </div>
                <div class="card-body">
                    <div id="drift-section">
                        <div class="loading-spinner"><i class="fa-solid fa-spinner fa-spin"></i> Loading drift...</div>
                    </div>
                </div>
            </div>

            <!-- Query internals (pg_stat_statements) -->
            <div class="card">
                <div class="card-header">
                    <h3><i class="fa-solid fa-magnifying-glass-chart"></i> Query Internals (pg_stat_statements)</h3>
                </div>
                <div class="card-body">
                    <div id="qint-section">
                        <div class="loading-spinner"><i class="fa-solid fa-spinner fa-spin"></i> Loading query internals...</div>
                    </div>
                </div>
            </div>
        </div>
        </div>
    `;

    // Load data from TimescaleDB API endpoints
    loadBGWriterData(instance.name);
    loadArchiverData(instance.name);
    loadWaitEvents(instance.name);
    loadDbIO(instance.name);
    loadConfigDrift(instance.name);
    loadQueryInternals(instance.name);
};

function loadWaitEvents(instanceName) {
    const section = document.getElementById('waits-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/postgres/waits/history?instance=${encodeURIComponent(instanceName)}&limit=1200`)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            const rows = data.rows || [];
            if (!Array.isArray(rows) || rows.length === 0) {
                section.innerHTML = `<div class="alert alert-info"><i class="fa-solid fa-info-circle"></i> No wait-event history yet (collector will populate after next cycle).</div>`;
                return;
            }

            // Aggregate by wait_event_type for a simple “contention taxonomy” view.
            const byTs = new Map(); // ts -> {type -> count}
            rows.forEach(r => {
                const ts = r.capture_timestamp || r.timestamp;
                const t = (r.wait_event_type || 'Other') || 'Other';
                const c = Number(r.sessions_count || 0);
                if (!ts) return;
                if (!byTs.has(ts)) byTs.set(ts, {});
                byTs.get(ts)[t] = (byTs.get(ts)[t] || 0) + c;
            });
            const labels = Array.from(byTs.keys()).sort();
            const types = new Set();
            labels.forEach(ts => Object.keys(byTs.get(ts) || {}).forEach(k => types.add(k)));
            const typeArr = Array.from(types).slice(0, 6); // keep chart readable
            const palette = ['#3b82f6','#10b981','#f59e0b','#ef4444','#a855f7','#22c55e'];
            const datasets = typeArr.map((t, idx) => ({
                label: t,
                data: labels.map(ts => (byTs.get(ts)?.[t] || 0)),
                borderColor: palette[idx % palette.length],
                backgroundColor: palette[idx % palette.length],
                tension: 0.25,
                pointRadius: 0
            }));

            section.innerHTML = `
                <div class="chart-container" style="height:180px;">
                    <canvas id="pgWaitsChartAdvanced"></canvas>
                </div>
                <div class="table-footer"><small class="text-muted">Aggregated by wait_event_type from pg_stat_activity snapshots.</small></div>
            `;
            const ctx = document.getElementById('pgWaitsChartAdvanced');
            if (ctx && window.Chart) {
                window.currentCharts = window.currentCharts || {};
                if (window.currentCharts.pgWaitsAdv) window.currentCharts.pgWaitsAdv.destroy();
                window.currentCharts.pgWaitsAdv = new Chart(ctx.getContext('2d'), {
                    type: 'line',
                    data: { labels: labels.map(l => new Date(l).toLocaleTimeString()), datasets },
                    options: { responsive:true, maintainAspectRatio:false }
                });
            }
        })
        .catch(err => {
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load wait events: ${window.escapeHtml(err.message)}</div>`;
        });
}

function loadDbIO(instanceName) {
    const section = document.getElementById('io-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/postgres/io/history?instance=${encodeURIComponent(instanceName)}&limit=2000`)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            const rows = data.rows || [];
            if (!Array.isArray(rows) || rows.length === 0) {
                section.innerHTML = `<div class="alert alert-info"><i class="fa-solid fa-info-circle"></i> No IO history yet (collector will populate after next cycle).</div>`;
                return;
            }

            // Database selector is built from the latest timestamp snapshot.
            const latestTs = rows[0]?.capture_timestamp;
            const latestRows = rows.filter(r => r.capture_timestamp === latestTs);
            const dbs = Array.from(new Set(latestRows.map(r => String(r.database_name || '')).filter(Boolean))).sort();
            latestRows.sort((a, b) => Number(b.temp_bytes || 0) - Number(a.temp_bytes || 0));
            const defaultDb = (latestRows[0]?.database_name) || (dbs[0] || rows[0]?.database_name || '');

            window._pgEnterpriseIoRows = rows; // cached for dropdown re-render
            window._pgEnterpriseIoDefaultDb = defaultDb;

            const renderDb = (dbName) => {
                const safeDb = String(dbName || '');
                const all = window._pgEnterpriseIoRows || [];
                const series = all.filter(r => String(r.database_name || '') === safeDb).slice().reverse();
                if (!series.length) return;

                const labels = series.map(r => new Date(r.capture_timestamp).toLocaleTimeString());
                const d = (arr, k) => arr.map((r, i) => i === 0 ? 0 : Math.max(0, Number(r[k] || 0) - Number(arr[i - 1][k] || 0)));
                const blksReadD = d(series, 'blks_read');
                const tempBytesD = d(series, 'temp_bytes').map(v => v / 1024 / 1024);

                const ctx = document.getElementById('pgIoChartAdvanced');
                if (ctx && window.Chart) {
                    window.currentCharts = window.currentCharts || {};
                    if (window.currentCharts.pgIoAdv) window.currentCharts.pgIoAdv.destroy();
                    window.currentCharts.pgIoAdv = new Chart(ctx.getContext('2d'), {
                        type: 'line',
                        data: {
                            labels,
                            datasets: [
                                { label: 'blks_read Δ', data: blksReadD, borderColor: '#3b82f6', backgroundColor: '#3b82f6', tension: 0.25, pointRadius: 0 },
                                { label: 'temp_bytes Δ (MB)', data: tempBytesD, borderColor: '#ef4444', backgroundColor: '#ef4444', tension: 0.25, pointRadius: 0 }
                            ]
                        },
                        options: { responsive: true, maintainAspectRatio: false }
                    });
                }
            };

            section.innerHTML = `
                <div style="display:flex; gap:0.5rem; align-items:center; flex-wrap:wrap; margin-bottom:0.35rem;">
                    <div class="text-muted" style="font-size:0.75rem;">Database</div>
                    <select id="pgIoDbSelectAdvanced" class="form-select" style="padding:0.25rem 0.5rem; font-size:0.75rem; max-width:360px;">
                        ${(dbs.length ? dbs : [defaultDb]).map(db => `
                            <option value="${window.escapeHtml(db)}" ${db === defaultDb ? 'selected' : ''}>${window.escapeHtml(db)}</option>
                        `).join('')}
                    </select>
                    <div class="text-muted" style="font-size:0.7rem;">(deltas per snapshot)</div>
                </div>
                <div class="chart-container" style="height:180px;">
                    <canvas id="pgIoChartAdvanced"></canvas>
                </div>
                <div class="table-footer"><small class="text-muted">blks_read and temp_bytes are computed as deltas between stored pg_stat_database counters. Temp is MB per interval.</small></div>
            `;

            const sel = document.getElementById('pgIoDbSelectAdvanced');
            if (sel) {
                sel.addEventListener('change', () => renderDb(sel.value));
            }
            renderDb(defaultDb);
        })
        .catch(err => {
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load IO stats: ${window.escapeHtml(err.message)}</div>`;
        });
}

function loadConfigDrift(instanceName) {
    const section = document.getElementById('drift-section');
    if (!section) return;
    window.apiClient.authenticatedFetch(`/api/postgres/settings/drift?instance=${encodeURIComponent(instanceName)}`)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            const changes = data.changes || [];
            if (!Array.isArray(changes) || changes.length === 0) {
                section.innerHTML = `<div class="alert alert-info"><i class="fa-solid fa-info-circle"></i> No config drift detected (or only one snapshot exists so far).</div>`;
                return;
            }
            section.innerHTML = `
                <table class="data-table" style="font-size:0.75rem;">
                    <thead><tr><th>Setting</th><th>Old</th><th>New</th><th>Unit</th><th>Source</th></tr></thead>
                    <tbody>
                        ${changes.map(c => `
                            <tr>
                                <td><strong>${window.escapeHtml(c.name)}</strong></td>
                                <td class="text-muted">${window.escapeHtml(c.old_value || '')}</td>
                                <td class="text-accent font-bold">${window.escapeHtml(c.new_value || '')}</td>
                                <td class="text-muted">${window.escapeHtml(c.unit || '')}</td>
                                <td class="text-muted">${window.escapeHtml((c.old_source || '') + ' → ' + (c.new_source || ''))}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            `;
        })
        .catch(err => {
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load drift: ${window.escapeHtml(err.message)}</div>`;
        });
}

function loadQueryInternals(instanceName) {
    const section = document.getElementById('qint-section');
    if (!section) return;
    const to = new Date();
    const from = new Date(to.getTime() - 60 * 60 * 1000);
    const qurl = `/api/postgres/queries?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`;
    window.apiClient.authenticatedFetch(qurl)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            window._pgEnterpriseQueriesMeta = {
                end_capture: data.end_capture || '',
                window_from: data.window_from || '',
                window_to: data.window_to || '',
                stats_note: data.stats_note || '',
                baseline_capture: data.baseline_capture || ''
            };
            if (data.pg_stat_statements_enabled === false) {
                section.innerHTML = `<div class="alert alert-warning"><i class="fa-solid fa-triangle-exclamation"></i> pg_stat_statements is not enabled on this instance.</div>`;
                return;
            }
            const qs = data.queries || [];
            if (!Array.isArray(qs) || qs.length === 0) {
                section.innerHTML = `<div class="alert alert-info"><i class="fa-solid fa-info-circle"></i> No query stats returned.</div>`;
                return;
            }
            // Avoid embedding SQL directly into onclick (can break on special chars).
            window._pgEnterpriseSqlById = window._pgEnterpriseSqlById || {};
            window._pgEnterpriseUserById = window._pgEnterpriseUserById || {};
            qs.forEach(q => {
                const id = String(q.query_id || '');
                if (!id) return;
                window._pgEnterpriseSqlById[id] = String(q.query || '');
                window._pgEnterpriseUserById[id] = String(q.user || '');
            });
            window.pgEnterpriseOpenSql = window.pgEnterpriseOpenSql || function(queryId) {
                const id = String(queryId || '');
                const sql = window._pgEnterpriseSqlById?.[id] || '';
                const user = window._pgEnterpriseUserById?.[id] || '';
                if (typeof window.showPgQueryFingerprintDetails === 'function') {
                    window.showPgQueryFingerprintDetails(id, user, sql);
                }
            };

            // Show “internal” columns (IO-ish) that aren’t in the main dashboard.
            const top = qs.slice().sort((a,b)=>Number(b.temp_blks_written||0)-Number(a.temp_blks_written||0)).slice(0, 20);
            section.innerHTML = `
                <table class="data-table" style="font-size:0.72rem;">
                    <thead>
                        <tr>
                            <th>User</th><th>Calls</th><th>Total ms</th><th>Temp wr</th><th>Shared rd</th><th>WAL bytes</th><th>SQL</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${top.map(q => `
                            <tr>
                                <td>${window.escapeHtml(q.user || '')}</td>
                                <td class="text-right">${Number(q.calls||0).toLocaleString()}</td>
                                <td class="text-right">${Number(q.total_time||0).toFixed(0)}</td>
                                <td class="text-right ${Number(q.temp_blks_written||0)>0?'text-warning font-bold':''}">${Number(q.temp_blks_written||0).toLocaleString()}</td>
                                <td class="text-right">${Number(q.shared_blks_read||0).toLocaleString()}</td>
                                <td class="text-right">${Number(q.wal_bytes||0).toLocaleString()}</td>
                                <td style="max-width:520px;">
                                    <div class="code-snippet pg-sql-preview" style="cursor:pointer; text-decoration:underline;"
                                         onclick="window.pgEnterpriseOpenSql('${window.escapeHtml(String(q.query_id||''))}')">
                                        ${window.escapeHtml(q.query || '').slice(0, 220)}
                                    </div>
                                </td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
                <div class="table-footer"><small class="text-muted">Sorted by temp_blks_written (spill risk). Click SQL to view.</small></div>
            `;
        })
        .catch(err => {
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load query internals: ${window.escapeHtml(err.message)}</div>`;
        });
}

function loadBGWriterData(instanceName) {
    const section = document.getElementById('bgwriter-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/postgres/bgwriter?instance=${encodeURIComponent(instanceName)}`)
        .then(response => {
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            const contentType = response.headers.get('content-type') || '';
            if (!contentType.includes('application/json')) {
                throw new Error('Server returned non-JSON response');
            }
            return response.json();
        })
        .then(data => {
            if (data.error) {
                section.innerHTML = `<div class="alert alert-warning"><i class="fa-solid fa-exclamation-triangle"></i> ${window.escapeHtml(data.error)}</div>`;
                return;
            }

            if (!data.stats || data.stats.length === 0) {
                section.innerHTML = `
                    <div class="alert alert-info">
                        <i class="fa-solid fa-info-circle"></i> No BGWriter data available yet.
                        <p class="text-sm mt-1">Data will appear after the first collection cycle (15 min).</p>
                    </div>
                `;
                return;
            }

            // Delta/dedupe on client as a safety net: keep only the first row for each timestamp.
            const seenTs = new Set();
            const stats = (data.stats || []).filter(s => {
                const t = String(s.timestamp || '');
                if (!t || seenTs.has(t)) return false;
                seenTs.add(t);
                return true;
            });

            // Build a compact trend chart + smaller table.
            const labels = stats.slice().reverse().map(s => {
                const d = s.timestamp ? new Date(s.timestamp) : null;
                return d ? d.toLocaleTimeString() : '—';
            });
            const timed = stats.slice().reverse().map(s => Number(s.checkpoints_timed || 0));
            const req = stats.slice().reverse().map(s => Number(s.checkpoints_req || 0));
            const wms = stats.slice().reverse().map(s => Number(s.checkpoint_write_time || 0));

            let html = `
                <div style="display:grid; grid-template-columns: 1.1fr 0.9fr; gap:0.75rem; align-items:start;">
                    <div class="glass-panel" style="padding:0.6rem;">
                        <div class="text-muted" style="font-size:0.75rem; margin-bottom:0.25rem;">Checkpoint trend</div>
                        <div style="height:170px;"><canvas id="pgBgwriterChart"></canvas></div>
                    </div>
                    <div class="table-responsive" style="max-height:220px; overflow:auto;">
                        <table class="data-table" style="font-size:0.78rem;">
                            <thead>
                                <tr>
                                    <th>Time</th>
                                    <th class="text-right">Timed</th>
                                    <th class="text-right">Req</th>
                                    <th class="text-right">Write ms</th>
                                    <th class="text-right">Bufs ckpt</th>
                                </tr>
                            </thead>
                            <tbody>
            `;

            stats.forEach(stat => {
                const timestamp = stat.timestamp ? new Date(stat.timestamp).toLocaleString() : 'N/A';
                html += `
                    <tr>
                        <td>${window.escapeHtml(timestamp)}</td>
                        <td class="text-right">${formatNumber(stat.checkpoints_timed || 0, 0)}</td>
                        <td class="text-right">${formatNumber(stat.checkpoints_req || 0, 0)}</td>
                        <td class="text-right">${formatNumber(stat.checkpoint_write_time || 0, 0)}</td>
                        <td class="text-right">${formatNumber(stat.buffers_checkpoint || 0, 0)}</td>
                    </tr>
                `;
            });

            html += `
                            </tbody>
                        </table>
                        <div class="table-footer"><small class="text-muted">Showing ${stats.length} most recent captures (deduped).</small></div>
                    </div>
                </div>
            `;

            section.innerHTML = html;

            // Render chart
            try {
                window.currentCharts = window.currentCharts || {};
                if (window.currentCharts.pgBgwriterChart) {
                    window.currentCharts.pgBgwriterChart.destroy();
                }
                const c = document.getElementById('pgBgwriterChart');
                if (c && window.Chart) {
                    window.currentCharts.pgBgwriterChart = new Chart(c.getContext('2d'), {
                        type: 'line',
                        data: {
                            labels,
                            datasets: [
                                { label: 'checkpoints_timed', data: timed, borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.12)', tension: 0.25, pointRadius: 0 },
                                { label: 'checkpoints_req', data: req, borderColor: '#f59e0b', backgroundColor: 'rgba(245,158,11,0.12)', tension: 0.25, pointRadius: 0 },
                                { label: 'write_time_ms', data: wms, borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.12)', tension: 0.25, pointRadius: 0, yAxisID: 'y2' },
                            ]
                        },
                        options: {
                            responsive: true,
                            maintainAspectRatio: false,
                            plugins: { legend: { display: true, labels: { boxWidth: 10, font: { size: 10 } } } },
                            scales: {
                                y: { ticks: { font: { size: 10 } }, grid: { color: 'rgba(148,163,184,0.15)' } },
                                y2: { position: 'right', ticks: { font: { size: 10 } }, grid: { display: false } },
                                x: { ticks: { font: { size: 10 }, maxTicksLimit: 8 }, grid: { display: false } }
                            }
                        }
                    });
                }
            } catch (e) {
                // non-fatal
            }
        })
        .catch(error => {
            console.error('Error loading BGWriter data:', error);
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load BGWriter data: ${window.escapeHtml(error.message)}</div>`;
        });
}

/**
 * Loads WAL Archiver statistics from TimescaleDB
 * @param {string} instanceName - The PostgreSQL instance name
 */
function loadArchiverData(instanceName) {
    const section = document.getElementById('archiver-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/postgres/archiver?instance=${encodeURIComponent(instanceName)}`)
        .then(response => {
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            const contentType = response.headers.get('content-type') || '';
            if (!contentType.includes('application/json')) {
                throw new Error('Server returned non-JSON response');
            }
            return response.json();
        })
        .then(data => {
            if (data.error) {
                section.innerHTML = `<div class="alert alert-warning"><i class="fa-solid fa-exclamation-triangle"></i> ${window.escapeHtml(data.error)}</div>`;
                return;
            }

            if (!data.stats || data.stats.length === 0) {
                section.innerHTML = `
                    <div class="alert alert-info">
                        <i class="fa-solid fa-info-circle"></i> No Archiver data available yet.
                        <p class="text-sm mt-1">Data will appear after the first collection cycle (15 min).</p>
                    </div>
                `;
                return;
            }

            let html = `
                <table class="data-table">
                    <thead>
                        <tr>
                            <th>Time</th>
                            <th>Total Archived</th>
                            <th>Total Failed</th>
                            <th>Max Failed in Period</th>
                            <th>Has Failures</th>
                        </tr>
                    </thead>
                    <tbody>
            `;

            data.stats.forEach(stat => {
                const timestamp = stat.timestamp ? new Date(stat.timestamp).toLocaleString() : 'N/A';
                const hasFailures = stat.has_failures ? 
                    '<span class="badge badge-danger">Yes</span>' : 
                    '<span class="badge badge-success">No</span>';
                
                html += `
                    <tr>
                        <td>${window.escapeHtml(timestamp)}</td>
                        <td class="text-right">${formatNumber(stat.total_archived || 0)}</td>
                        <td class="text-right ${(stat.total_failed || 0) > 0 ? 'text-danger' : ''}">${formatNumber(stat.total_failed || 0)}</td>
                        <td class="text-right ${(stat.max_failed || 0) > 0 ? 'text-danger' : ''}">${formatNumber(stat.max_failed || 0)}</td>
                        <td>${hasFailures}</td>
                    </tr>
                `;
            });

            html += `
                    </tbody>
                </table>
                <div class="table-footer">
                    <small class="text-muted">Showing ${data.stats.length} time buckets | Last 24 hours aggregated data</small>
                </div>
            `;

            section.innerHTML = html;
        })
        .catch(error => {
            console.error('Error loading Archiver data:', error);
            section.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-exclamation-circle"></i> Failed to load Archiver data: ${window.escapeHtml(error.message)}</div>`;
        });
}

/**
 * Formats a number with optional decimal places
 * @param {number} num - The number to format
 * @param {number} decimals - Number of decimal places (default: 0)
 * @returns {string} Formatted number string
 */
function formatNumber(num, decimals = 0) {
    if (num === null || num === undefined) return '0';
    if (decimals > 0) {
        return Number(num).toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
    }
    return Number(num).toLocaleString('en-US');
}
