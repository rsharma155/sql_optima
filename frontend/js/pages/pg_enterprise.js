/*
 * SQL Optima — PostgreSQL enterprise metrics dashboard.
 */

window.PgEnterpriseDashboardView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) { alert('Select an instance first.'); return; }
    if (inst.type !== 'postgres') { alert('Enterprise monitoring is for PostgreSQL only.'); return; }
    const dbName = window.appState.currentDatabase || 'all';

    window.appState.activeViewId = 'drilldown-pg-enterprise';
    
    // We always reload to ensure fresh bindings
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
    window.pgEnterpriseInterval = setInterval(() => {
        if (window.appState.activeViewId === 'drilldown-pg-enterprise') {
            window.PgEnterpriseDashboardView();
        } else {
            clearInterval(window.pgEnterpriseInterval);
        }
    }, 30000);
};

async function updateEnterpriseHeader(instName) {
    try {
        const snapshotResp = await window.apiClient.authenticatedFetch(`/api/postgres/server-info?instance=${encodeURIComponent(instName)}`);
        if (snapshotResp.ok) {
            const s = await snapshotResp.json();
            if (document.getElementById('pg-uptime')) document.getElementById('pg-uptime').textContent = 'Uptime: ' + (s.uptime || 'N/A');
            if (document.getElementById('pg-version')) document.getElementById('pg-version').textContent = (s.version || '').split(',')[0];
            if (document.getElementById('pgLastRefreshTime')) document.getElementById('pgLastRefreshTime').textContent = new Date().toLocaleTimeString();

            // Health Score
            const hs = s.health_score || 0;
            const healthColor = hs > 80 ? 'success' : hs > 60 ? 'warning' : 'danger';
            const hBadge = document.getElementById('pgHealthScoreBadge');
            if (hBadge) {
                hBadge.textContent = hs;
                hBadge.style.color = `var(--${healthColor})`;
            }
        }
    } catch (e) { console.error("PG enterprise header fetch failed:", e); }
}

function loadBGWriterData(instanceName) {
    const section = document.getElementById('bgwriter-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/postgres/bgwriter?instance=${encodeURIComponent(instanceName)}`)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            if (!data.stats || data.stats.length === 0) {
                section.innerHTML = '<div class="alert alert-info">No BGWriter data available yet.</div>';
                return;
            }

            const latest = data.stats[0];
            const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
            set('stat-ckpt-timed', formatNumber(latest.checkpoints_timed || 0));
            set('stat-ckpt-req', formatNumber(latest.checkpoints_req || 0));
            set('stat-bgw-halts', formatNumber(latest.maxwritten_clean || 0));
            
            if (latest.checkpoints_req > latest.checkpoints_timed) {
                document.getElementById('card-ckpt-req')?.classList.add('border-warning');
            }

            const stats = data.stats.slice().reverse();
            const labels = stats.map(s => new Date(s.time || s.timestamp).toLocaleTimeString());
            const timed = stats.map(s => Number(s.checkpoints_timed || 0));
            const req = stats.map(s => Number(s.checkpoints_req || 0));

            section.innerHTML = `
                <div class="grid-container">
                    <div class="col-8 col-tablet-6"><div style="height:200px;"><canvas id="pgBgwriterChart"></canvas></div></div>
                    <div class="col-4 col-tablet-6 table-container-compact" style="height:200px;">
                        <table class="modern-table modern-table-compact">
                            <thead><tr><th>Time</th><th>Timed</th><th>Req</th><th>Write ms</th></tr></thead>
                            <tbody>${stats.slice(-10).reverse().map(s => `<tr><td>${new Date(s.time || s.timestamp).toLocaleTimeString()}</td><td>${s.checkpoints_timed}</td><td>${s.checkpoints_req}</td><td>${s.checkpoint_write_time}</td></tr>`).join('')}</tbody>
                        </table>
                    </div>
                </div>
            `;

            new Chart(document.getElementById('pgBgwriterChart').getContext('2d'), {
                type: 'line',
                data: { labels, datasets: [
                    { label: 'Timed', data: timed, borderColor: '#3b82f6', pointRadius: 0, tension: 0.3 },
                    { label: 'Req', data: req, borderColor: '#f59e0b', pointRadius: 0, tension: 0.3 }
                ]},
                options: { responsive: true, maintainAspectRatio: false }
            });
        });
}

function loadArchiverData(instanceName) {
    const section = document.getElementById('archiver-section');
    if (!section) return;

    window.apiClient.authenticatedFetch(`/api/postgres/archiver?instance=${encodeURIComponent(instanceName)}`)
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
        .then(data => {
            if (!data.stats || data.stats.length === 0) {
                section.innerHTML = '<div class="alert alert-info">No Archiver data available.</div>';
                return;
            }
            const latest = data.stats[0];
            const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
            set('stat-archived-wals', formatNumber(latest.archived_count || 0));
            set('stat-archiving-failures', formatNumber(latest.failed_count || 0));
            
            if (latest.failed_count > 0) {
                document.getElementById('card-archiving-failures')?.classList.add('border-danger');
            }

            section.innerHTML = `
                <div class="table-container-compact" style="height:200px;">
                    <table class="modern-table modern-table-compact">
                        <thead><tr><th>Time</th><th>Archived</th><th>Failed</th><th>Last Fail WAL</th></tr></thead>
                        <tbody>${data.stats.slice(0, 10).map(s => `<tr><td>${new Date(s.timestamp).toLocaleTimeString()}</td><td>${s.archived_count}</td><td class="${s.failed_count>0?'text-danger':''}">${s.failed_count}</td><td>${s.last_failed_wal || '-'}</td></tr>`).join('')}</tbody>
                    </table>
                </div>
            `;
        });
}

function loadWaitEvents(instanceName, from, to) {
    const section = document.getElementById('waits-section');
    if (!section) return;
    let url = `/api/postgres/waits/history?instance=${encodeURIComponent(instanceName)}`;
    if (from && to) url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;
    
    window.apiClient.authenticatedFetch(url).then(r => r.json()).then(data => {
        const rows = data.rows || [];
        if (!rows.length) { section.innerHTML = '<div class="text-muted p-4">No wait data.</div>'; return; }
        const byTs = new Map();
        rows.forEach(r => {
            const ts = r.capture_timestamp || r.timestamp;
            if (!byTs.has(ts)) byTs.set(ts, {});
            byTs.get(ts)[r.wait_event_type || 'Other'] = r.sessions_count;
        });
        const labels = Array.from(byTs.keys()).sort();
        const types = ['CPU', 'IO', 'Lock', 'LWLock', 'BufferPin'];
        const datasets = types.map((t, idx) => ({
            label: t, data: labels.map(ts => byTs.get(ts)[t] || 0), borderColor: ['#10b981', '#3b82f6', '#f59e0b', '#ef4444', '#8b5cf6'][idx], tension: 0.3, pointRadius: 0
        }));

        section.innerHTML = '<div style="height:200px;"><canvas id="pgWaitsChartAdv"></canvas></div>';
        new Chart(document.getElementById('pgWaitsChartAdv').getContext('2d'), { type: 'line', data: { labels: labels.map(l => new Date(l).toLocaleTimeString()), datasets }, options: { responsive: true, maintainAspectRatio: false } });
    });
}

function loadDbIO(instanceName, from, to) {
    const section = document.getElementById('io-section');
    if (!section) return;
    let url = `/api/postgres/io/history?instance=${encodeURIComponent(instanceName)}`;
    if (from && to) url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;
    
    window.apiClient.authenticatedFetch(url).then(r => r.json()).then(data => {
        const rows = data.rows || [];
        if (!rows.length) { section.innerHTML = '<div class="text-muted p-4">No IO data.</div>'; return; }
        const dbs = Array.from(new Set(rows.map(r => r.database_name))).sort();
        const latestDb = dbs[0];

        section.innerHTML = `
            <select id="pgIoDbSel" class="form-select mb-2" style="font-size:0.7rem; padding:2px;">${dbs.map(d => `<option value="${d}">${d}</option>`).join('')}</select>
            <div style="height:170px;"><canvas id="pgIoChartAdv"></canvas></div>
        `;
        
        const render = (db) => {
            const series = rows.filter(r => r.database_name === db).slice().reverse();
            const labels = series.map(s => new Date(s.capture_timestamp).toLocaleTimeString());
            const delta = (arr, k) => arr.map((r, i) => i === 0 ? 0 : Math.max(0, r[k] - arr[i-1][k]));
            new Chart(document.getElementById('pgIoChartAdv').getContext('2d'), {
                type: 'line',
                data: { labels, datasets: [
                    { label: 'Reads', data: delta(series, 'blks_read'), borderColor: '#3b82f6', tension: 0.3, pointRadius: 0 },
                    { label: 'Temp (MB)', data: delta(series, 'temp_bytes').map(v => v/1024/1024), borderColor: '#ef4444', tension: 0.3, pointRadius: 0 }
                ]},
                options: { responsive: true, maintainAspectRatio: false }
            });
        };
        document.getElementById('pgIoDbSel').onchange = (e) => render(e.target.value);
        render(latestDb);
    });
}

function loadConfigDrift(instanceName) {
    const section = document.getElementById('drift-section');
    if (!section) return;
    window.apiClient.authenticatedFetch(`/api/postgres/settings/drift?instance=${encodeURIComponent(instanceName)}`).then(r => r.json()).then(data => {
        const changes = data.changes || [];
        if (!changes.length) { section.innerHTML = '<div class="text-muted p-4 text-center">No recent changes detected.</div>'; return; }
        section.innerHTML = `
            <table class="modern-table modern-table-compact" style="font-size:0.7rem;">
                <thead><tr><th>Setting</th><th>Old</th><th>New</th></tr></thead>
                <tbody>${changes.map(c => `<tr><td>${c.name}</td><td class="text-muted">${c.old_value}</td><td class="text-accent">${c.new_value}</td></tr>`).join('')}</tbody>
            </table>
        `;
    });
}

function loadQueryInternals(instanceName, from, to) {
    const section = document.getElementById('qint-section');
    if (!section) return;
    let url = `/api/postgres/queries?instance=${encodeURIComponent(instanceName)}`;
    if (from && to) url += `&from=${encodeURIComponent(new Date(from).toISOString())}&to=${encodeURIComponent(new Date(to).toISOString())}`;
    
    window.apiClient.authenticatedFetch(url).then(r => r.json()).then(data => {
        const qs = data.queries || [];
        if (!qs.length) { section.innerHTML = '<div class="text-muted p-4">No query internals.</div>'; return; }
        const top = qs.sort((a,b) => b.temp_blks_written - a.temp_blks_written).slice(0, 10);
        
        window.appState.pgInternalQueries = top.map(q => q.query);

        section.innerHTML = `
            <table class="modern-table modern-table-compact" style="font-size:0.7rem;">
                <thead><tr><th>User</th><th>Calls</th><th>Total ms</th><th>Temp Write</th><th>SQL</th></tr></thead>
                <tbody>${top.map((q, idx) => `
                    <tr>
                        <td>${q.username}</td>
                        <td>${q.calls}</td>
                        <td>${q.total_time.toFixed(0)}</td>
                        <td class="${q.temp_blks_written>0?'text-warning':''}">${q.temp_blks_written}</td>
                        <td class="small text-muted" style="cursor:pointer; text-decoration:underline;" data-action="call" data-fn="showPgInternalQueryDetail" data-idx="${idx}">
                            ${window.escapeHtml(q.query.substring(0,60))}...
                        </td>
                    </tr>`).join('')}
                </tbody>
            </table>
        `;
    });
}

window.showPgInternalQueryDetail = function(idx) {
    const message = window.appState.pgInternalQueries[idx];
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
                <button id="copyPgQueryBtn" class="btn btn-sm btn-accent" style="padding: 0.5rem 1.5rem;">
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
            setTimeout(() => {
                this.innerHTML = '<i class="fa-solid fa-copy"></i> Copy SQL';
            }, 1500);
        });
    });

    modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.remove();
    });
};

function formatNumber(num) { return Number(num || 0).toLocaleString(); }
