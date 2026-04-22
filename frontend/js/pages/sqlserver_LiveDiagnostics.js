/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Real-time diagnostics page for live query monitoring, blocking, and waits.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.escapeHtml = function(unsafe) { return (!unsafe) ? '' : unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;"); };

window.LiveDiagnosticsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1>Real-Time Diagnostics (RTD)</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Live DMV Queries</p>
                </div>
                <div class="flex-between" style="align-items:center; gap:1rem;">
                    <select id="liveAutoRefresh" class="custom-select" style="padding:0.3rem; font-size:0.8rem;">
                        <option value="0">Auto-Refresh: Off</option>
                        <option value="10000">10s</option>
                        <option value="30000">30s</option>
                    </select>
                    <button class="btn btn-sm btn-accent" data-action="call" data-fn="refreshLiveDiagnostics"><i class="fa-solid fa-refresh"></i> Refresh Now</button>
                </div>
            </div>

            <div class="alert alert-info" style="background:rgba(59,130,246,0.1); border:1px solid var(--accent); margin-bottom:1rem; padding:0.5rem 1rem; font-size:0.8rem;">
                <i class="fa-solid fa-info-circle"></i> <strong>Live Mode:</strong> Direct DMV queries (10s timeout). Session-oriented panels filter to <strong>user workloads</strong>: <code>is_user_process = 1</code>, current database <code>database_id &gt; 4</code>, and exclude the <strong>distribution</strong> database. Instance-level KPIs (memory, batch requests/sec) are unchanged.
            </div>

            <div class="metrics-grid" style="display:grid; grid-template-columns:repeat(4,1fr); gap:0.75rem; margin-bottom:1rem;">
                <div class="metric-card glass-panel" style="padding:0.75rem; text-align:center;">
                    <div id="kpi-sessions-loader" class="spinner" style="display:none;"></div>
                    <div id="kpi-sessions-content">
                        <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted);">Active Executing Sessions</div>
                        <div id="kpi-active-sessions" class="metric-value" style="font-size:1.75rem; color:var(--accent);">--</div>
                    </div>
                </div>
                <div class="metric-card glass-panel" style="padding:0.75rem; text-align:center;">
                    <div id="kpi-memory-loader" class="spinner" style="display:none;"></div>
                    <div id="kpi-memory-content">
                        <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted);">System Memory (Total vs Available)</div>
                        <div id="kpi-memory-value" class="metric-value" style="font-size:1.25rem; color:var(--accent);">--</div>
                    </div>
                </div>
                <div class="metric-card glass-panel" style="padding:0.75rem; text-align:center;">
                    <div id="kpi-throughput-loader" class="spinner" style="display:none;"></div>
                    <div id="kpi-throughput-content">
                        <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted);">Throughput (Batch Requests/sec)</div>
                        <div id="kpi-batch-requests" class="metric-value" style="font-size:1.75rem; color:var(--accent);">--</div>
                    </div>
                </div>
                <div class="metric-card glass-panel" style="padding:0.75rem; text-align:center;">
                    <div id="tempdb-loader" class="spinner" style="display:none;"></div>
                    <div id="tempdb-content">
                        <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted);">TempDB Space (MB)</div>
                        <div id="tempdb-total" class="metric-value" style="font-size:1.75rem; color:var(--accent);">--</div>
                    </div>
                </div>
            </div>

            <div class="table-card glass-panel mt-3" style="margin-bottom:1rem;">
                <div class="card-header flex-between">
                    <h3 style="font-size:0.9rem; margin:0;">Live Executing Queries (Top 50 by CPU)</h3>
                    <span id="running-queries-count" class="badge badge-info">0</span>
                </div>
                <div id="running-queries-loader" style="display:flex; justify-content:center; padding:2rem;"><div class="spinner"></div></div>
                <div id="running-queries-error" class="alert alert-danger" style="display:none; margin:0.5rem;"></div>
                <div class="table-responsive" style="max-height:400px; overflow-y:auto;">
                    <table class="data-table" style="font-size:0.75rem;" id="runningQueriesTable">
                        <thead style="position:sticky; top:0; background:var(--bg-surface,#1a1a2e); z-index:10;">
                            <tr>
                                <th>SPID</th>
                                <th>Login</th>
                                <th>Program</th>
                                <th>Status</th>
                                <th>CPU (ms)</th>
                                <th>Elapsed (ms)</th>
                                <th>Reads</th>
                                <th>Wait</th>
                                <th>Blocked</th>
                                <th>Query Text</th>
                            </tr>
                        </thead>
                        <tbody id="runningQueriesBody">
                            <tr><td colspan="10" class="text-center text-muted">Loading...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div class="tables-grid" style="display:grid; grid-template-columns:1fr 1fr; gap:0.75rem; margin-bottom:1rem;">
                <div id="blocking-panel" class="table-card glass-panel">
                    <div class="card-header">
                        <h3 style="font-size:0.9rem; margin:0; color:var(--danger);"><i class="fa-solid fa-ban"></i> Active Blocking Chain Analysis</h3>
                    </div>
                    <div id="blocking-loader" style="display:flex; justify-content:center; padding:2rem;"><div class="spinner"></div></div>
                    <div id="blocking-error" class="alert alert-danger" style="display:none; margin:0.5rem;"></div>
                    <div id="blocking-content">
                        <div class="table-responsive" style="max-height:200px; overflow-y:auto;">
                            <table class="data-table" style="font-size:0.7rem;" id="blockingTable">
                                <thead style="position:sticky; top:0; background:var(--bg-surface,#1a1a2e); z-index:10;">
                                    <tr>
                                        <th>Blocker SPID</th>
                                        <th>Login</th>
                                        <th>App</th>
                                        <th>Victims</th>
                                        <th>Max Wait (ms)</th>
                                    </tr>
                                </thead>
                                <tbody id="blockingBody">
                                    <tr><td colspan="5" class="text-center text-muted">Loading...</td></tr>
                                </tbody>
                            </table>
                        </div>
                        <div id="blocking-empty" class="text-center" style="padding:1rem; color:#22c55e; display:none;">
                            <i class="fa-solid fa-check-circle"></i> No Active Blocking Detected
                        </div>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header">
                        <h3 style="font-size:0.9rem; margin:0;">Real-Time File I/O Latency</h3>
                    </div>
                    <div id="io-latency-loader" style="display:flex; justify-content:center; padding:2rem;"><div class="spinner"></div></div>
                    <div id="io-latency-error" class="alert alert-danger" style="display:none; margin:0.5rem;"></div>
                    <div class="table-responsive" style="max-height:200px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;" id="ioLatencyTable">
                            <thead style="position:sticky; top:0; background:var(--bg-surface,#1a1a2e); z-index:10;">
                                <tr>
                                    <th>Database</th>
                                    <th>File</th>
                                    <th>Read (ms)</th>
                                    <th>Write (ms)</th>
                                </tr>
                            </thead>
                            <tbody id="ioLatencyBody">
                                <tr><td colspan="4" class="text-center text-muted">Loading...</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <div class="tables-grid" style="display:grid; grid-template-columns:1fr 1fr; gap:0.75rem; margin-bottom:1rem;">
                <div class="table-card glass-panel">
                    <div class="card-header">
                        <h3 style="font-size:0.9rem; margin:0;">Current Active Wait States</h3>
                    </div>
                    <div id="waits-loader" style="display:flex; justify-content:center; padding:2rem;"><div class="spinner"></div></div>
                    <div id="waits-error" class="alert alert-danger" style="display:none; margin:0.5rem;"></div>
                    <div class="table-responsive" style="max-height:200px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;" id="waitsTable">
                            <thead style="position:sticky; top:0; background:var(--bg-surface,#1a1a2e); z-index:10;">
                                <tr>
                                    <th>Wait Type</th>
                                    <th>Tasks</th>
                                    <th>Wait Time (ms)</th>
                                </tr>
                            </thead>
                            <tbody id="waitsBody">
                                <tr><td colspan="3" class="text-center text-muted">Loading...</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header">
                        <h3 style="font-size:0.9rem; margin:0;">Active Connections by Application</h3>
                    </div>
                    <div id="connections-loader" style="display:flex; justify-content:center; padding:2rem;"><div class="spinner"></div></div>
                    <div id="connections-error" class="alert alert-danger" style="display:none; margin:0.5rem;"></div>
                    <div class="table-responsive" style="max-height:200px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;" id="connectionsTable">
                            <thead style="position:sticky; top:0; background:var(--bg-surface,#1a1a2e); z-index:10;">
                                <tr>
                                    <th>Application</th>
                                    <th>Connections</th>
                                    <th>Logins</th>
                                </tr>
                            </thead>
                            <tbody id="connectionsBody">
                                <tr><td colspan="3" class="text-center text-muted">Loading...</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    `;

    document.getElementById('liveAutoRefresh').addEventListener('change', function() {
        const interval = parseInt(this.value);
        if (window.liveDiagnosticsInterval) {
            clearInterval(window.liveDiagnosticsInterval);
            window.liveDiagnosticsInterval = null;
        }
        if (interval > 0) {
            window.liveDiagnosticsInterval = setInterval(() => {
                window.refreshLiveDiagnostics();
            }, interval);
        }
    });

    window.appState.liveDiagnostics = {
        instanceName: inst.name,
        queryTexts: {}
    };

    await window.refreshLiveDiagnostics();
};

window.refreshLiveDiagnostics = async function() {
    const instanceName = window.appState.liveDiagnostics?.instanceName;
    if (!instanceName) return;

    await Promise.all([
        window.loadLiveKPIs(instanceName),
        window.loadLiveTempDB(instanceName),
        window.loadLiveRunningQueries(instanceName),
        window.loadLiveBlocking(instanceName),
        window.loadLiveIOLatency(instanceName),
        window.loadLiveWaits(instanceName),
        window.loadLiveConnections(instanceName)
    ]);
};

window.loadLiveKPIs = async function(instanceName) {
    const loader = document.getElementById('kpi-sessions-loader');
    const content = document.getElementById('kpi-sessions-content');
    const sessionsEl = document.getElementById('kpi-active-sessions');
    const memoryEl = document.getElementById('kpi-memory-value');
    const throughputEl = document.getElementById('kpi-batch-requests');

    try {
        const res = await window.apiClient.authenticatedFetch(`/api/live/kpis?instance=${encodeURIComponent(instanceName)}`);
        const data = await res.json();

        loader.style.display = 'none';
        content.style.display = 'block';

        if (!data.success) {
            sessionsEl.textContent = 'Error';
            memoryEl.textContent = 'Error';
            throughputEl.textContent = 'Error';
            return;
        }

        const kpis = data.data;
        sessionsEl.textContent = kpis.active_sessions || 0;
        memoryEl.innerHTML = `<span style="color:var(--success);">${kpis.available_memory_mb || 0}</span> / ${kpis.total_memory_mb || 0} MB`;
        throughputEl.textContent = (kpis.batch_requests_sec || 0).toLocaleString();
    } catch (e) {
        loader.style.display = 'none';
        content.style.display = 'block';
        sessionsEl.textContent = 'Error';
        memoryEl.textContent = 'Error';
        throughputEl.textContent = 'Error';
    }
};

window.loadLiveTempDB = async function(instanceName) {
    const loader = document.getElementById('tempdb-loader');
    const content = document.getElementById('tempdb-content');
    const totalEl = document.getElementById('tempdb-total');

    try {
        const res = await window.apiClient.authenticatedFetch(`/api/live/tempdb?instance=${encodeURIComponent(instanceName)}`);
        const data = await res.json();

        loader.style.display = 'none';
        content.style.display = 'block';

        if (!data.success) {
            totalEl.textContent = 'Error';
            return;
        }

        const tempdb = data.data;
        totalEl.textContent = `${tempdb.total_mb || 0} MB`;
    } catch (e) {
        loader.style.display = 'none';
        content.style.display = 'block';
        totalEl.textContent = 'Error';
    }
};

window.loadLiveRunningQueries = async function(instanceName) {
    const loader = document.getElementById('running-queries-loader');
    const errorDiv = document.getElementById('running-queries-error');
    const tbody = document.getElementById('runningQueriesBody');
    const countBadge = document.getElementById('running-queries-count');

    loader.style.display = 'flex';
    errorDiv.style.display = 'none';

    try {
        const db = (window.appState.currentDatabase && window.appState.currentDatabase !== 'all') ? window.appState.currentDatabase : '';
        const qs = db ? `&database=${encodeURIComponent(db)}` : '';
        const res = await window.apiClient.authenticatedFetch(`/api/live/running-queries?instance=${encodeURIComponent(instanceName)}${qs}`);
        const data = await res.json();

        loader.style.display = 'none';

        if (!data.success) {
            errorDiv.textContent = `Error: ${data.error}${data.timeout ? ' (10s timeout)' : ''}`;
            errorDiv.style.display = 'block';
            return;
        }

        const queries = data.data || [];
        window.appState.liveDiagnostics.queryTexts = {};
        countBadge.textContent = queries.length;

        if (queries.length === 0) {
            tbody.innerHTML = '<tr><td colspan="10" class="text-center text-success"><i class="fa-solid fa-check"></i> No active queries</td></tr>';
            return;
        }

        tbody.innerHTML = queries.map((q, idx) => {
            const queryText = q.query_text || 'Unknown';
            const truncated = queryText.length > 80 ? queryText.substring(0, 80) + '...' : queryText;
            window.appState.liveDiagnostics.queryTexts['q' + idx] = queryText;
            
            return `
                <tr>
                    <td><span class="badge badge-info">${q.session_id || '-'}</span></td>
                    <td style="max-width:80px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${window.escapeHtml(q.login_name || 'Unknown')}</td>
                    <td style="max-width:80px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(q.program_name || '')}">${window.escapeHtml(q.program_name || 'Unknown')}</td>
                    <td><span class="badge ${q.status === 'running' ? 'badge-success' : 'badge-warning'}">${window.escapeHtml(q.status || '-')}</span></td>
                    <td class="text-warning">${(q.cpu_time || 0).toLocaleString()}</td>
                    <td>${((q.total_elapsed_time || 0) / 1000).toFixed(1)}s</td>
                    <td>${(q.logical_reads || 0).toLocaleString()}</td>
                    <td><span class="badge badge-outline">${window.escapeHtml(q.wait_type || '-')}</span></td>
                    <td>${q.blocking_session_id ? `<span class="badge badge-danger">${q.blocking_session_id}</span>` : '-'}</td>
                    <td style="max-width:200px; font-family:monospace; font-size:0.65rem; cursor:pointer; color:var(--accent);" 
                        data-action="call" data-fn="showFullQueryText" data-arg="q${idx}" 
                        title="Click to see full query">${window.escapeHtml(truncated)}</td>
                </tr>
            `;
        }).join('');
    } catch (e) {
        loader.style.display = 'none';
        errorDiv.textContent = `Error: ${e.message}`;
        errorDiv.style.display = 'block';
    }
};

window.showFullQueryText = function(key) {
    const queryText = window.appState.liveDiagnostics?.queryTexts?.[key];
    if (!queryText) return;

    const existing = document.getElementById('query-text-modal');
    if (existing) existing.remove();

    const modal = document.createElement('div');
    modal.id = 'query-text-modal';
    modal.style.cssText = 'position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background:rgba(0,0,0,0.8); display:flex; align-items:center; justify-content:center;';
    modal.innerHTML = `
        <div style="background:var(--bg-surface); margin:2%; padding:20px; border-radius:12px; width:95%; max-width:900px; max-height:80vh; overflow:auto; color:var(--text-primary);">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:1rem; border-bottom:1px solid var(--border-color); padding-bottom:0.75rem;">
                <h3 style="margin:0; color:var(--accent);"><i class="fa-solid fa-code"></i> Full Query Text</h3>
                <button data-action="close-id" data-target="query-text-modal" style="background:transparent; border:1px solid var(--border-color); color:var(--text-primary); font-size:1.25rem; cursor:pointer; padding:0.25rem 0.6rem; border-radius:4px;">&times;</button>
            </div>
            <pre style="background:var(--bg-base); padding:1rem; border-radius:8px; white-space:pre-wrap; word-wrap:break-word; font-family:'Courier New',monospace; font-size:0.8rem; line-height:1.5; max-height:60vh; overflow:auto;">${window.escapeHtml(queryText)}</pre>
            <div style="text-align:center; margin-top:1rem;">
                <button id="copyQueryBtn" style="background:var(--accent); color:#fff; border:none; padding:0.5rem 1.5rem; border-radius:6px; cursor:pointer;">
                    <i class="fa-solid fa-copy"></i> Copy SQL
                </button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);

    document.getElementById('copyQueryBtn').addEventListener('click', function() {
        navigator.clipboard.writeText(queryText).then(() => {
            this.innerHTML = '<i class="fa-solid fa-check"></i> Copied!';
            setTimeout(() => { this.innerHTML = '<i class="fa-solid fa-copy"></i> Copy SQL'; }, 1500);
        });
    });

    modal.addEventListener('click', (e) => { if (e.target === modal) modal.remove(); });
};

window.loadLiveBlocking = async function(instanceName) {
    const loader = document.getElementById('blocking-loader');
    const errorDiv = document.getElementById('blocking-error');
    const content = document.getElementById('blocking-content');
    const tbody = document.getElementById('blockingBody');
    const emptyDiv = document.getElementById('blocking-empty');
    const panel = document.getElementById('blocking-panel');

    loader.style.display = 'flex';
    errorDiv.style.display = 'none';
    content.style.display = 'block';
    emptyDiv.style.display = 'none';
    panel.style.borderColor = 'var(--border-color)';
    panel.style.borderWidth = '1px';

    try {
        const db = (window.appState.currentDatabase && window.appState.currentDatabase !== 'all') ? window.appState.currentDatabase : '';
        const qs = db ? `&database=${encodeURIComponent(db)}` : '';
        const res = await window.apiClient.authenticatedFetch(`/api/live/blocking?instance=${encodeURIComponent(instanceName)}${qs}`);
        const data = await res.json();

        loader.style.display = 'none';

        if (!data.success) {
            errorDiv.textContent = `Error: ${data.error}${data.timeout ? ' (10s timeout)' : ''}`;
            errorDiv.style.display = 'block';
            return;
        }

        const blocking = data.data || [];

        if (blocking.length === 0) {
            emptyDiv.style.display = 'block';
            tbody.innerHTML = '';
        } else {
            panel.style.borderColor = 'var(--danger)';
            panel.style.borderWidth = '2px';
            tbody.innerHTML = blocking.map(b => `
                <tr>
                    <td><span class="badge badge-danger">${b.Lead_Blocker || '-'}</span></td>
                    <td>${window.escapeHtml(b.Blocker_Login || 'Unknown')}</td>
                    <td style="max-width:80px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${window.escapeHtml(b.Blocker_App || 'Unknown')}</td>
                    <td><span class="badge badge-warning">${b.Total_Victims || 0}</span></td>
                    <td class="text-danger">${(b.Max_Wait_Time_ms || 0).toLocaleString()}</td>
                </tr>
            `).join('');
        }
    } catch (e) {
        loader.style.display = 'none';
        errorDiv.textContent = `Error: ${e.message}`;
        errorDiv.style.display = 'block';
    }
};

window.loadLiveIOLatency = async function(instanceName) {
    const loader = document.getElementById('io-latency-loader');
    const errorDiv = document.getElementById('io-latency-error');
    const tbody = document.getElementById('ioLatencyBody');

    loader.style.display = 'flex';
    errorDiv.style.display = 'none';

    try {
        const res = await window.apiClient.authenticatedFetch(`/api/live/io-latency?instance=${encodeURIComponent(instanceName)}`);
        const data = await res.json();

        loader.style.display = 'none';

        if (!data.success) {
            errorDiv.textContent = `Error: ${data.error}${data.timeout ? ' (10s timeout)' : ''}`;
            errorDiv.style.display = 'block';
            return;
        }

        const ioLatency = data.data || [];

        if (ioLatency.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">No I/O data</td></tr>';
            return;
        }

        tbody.innerHTML = ioLatency.map(io => {
            const readMs = parseFloat(io.read_latency_ms) || 0;
            const writeMs = parseFloat(io.write_latency_ms) || 0;
            const readClass = readMs > 20 ? 'text-danger' : readMs > 10 ? 'text-warning' : 'text-success';
            const writeClass = writeMs > 20 ? 'text-danger' : writeMs > 10 ? 'text-warning' : 'text-success';
            return `
                <tr>
                    <td>${window.escapeHtml(io.database_name || 'Unknown')}</td>
                    <td>${io.file_id || '-'}</td>
                    <td class="${readClass}">${readMs.toFixed(2)}</td>
                    <td class="${writeClass}">${writeMs.toFixed(2)}</td>
                </tr>
            `;
        }).join('');
    } catch (e) {
        loader.style.display = 'none';
        errorDiv.textContent = `Error: ${e.message}`;
        errorDiv.style.display = 'block';
    }
};

window.loadLiveWaits = async function(instanceName) {
    const loader = document.getElementById('waits-loader');
    const errorDiv = document.getElementById('waits-error');
    const tbody = document.getElementById('waitsBody');

    loader.style.display = 'flex';
    errorDiv.style.display = 'none';

    try {
        const db = (window.appState.currentDatabase && window.appState.currentDatabase !== 'all') ? window.appState.currentDatabase : '';
        const qs = db ? `&database=${encodeURIComponent(db)}` : '';
        const res = await window.apiClient.authenticatedFetch(`/api/live/waits?instance=${encodeURIComponent(instanceName)}${qs}`);
        const data = await res.json();

        loader.style.display = 'none';

        if (!data.success) {
            errorDiv.textContent = `Error: ${data.error}${data.timeout ? ' (10s timeout)' : ''}`;
            errorDiv.style.display = 'block';
            return;
        }

        const waits = data.data || [];

        if (waits.length === 0) {
            tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">No wait data</td></tr>';
            return;
        }

        tbody.innerHTML = waits.map(w => `
            <tr>
                <td><span class="badge badge-outline">${window.escapeHtml(w.wait_type || '-')}</span></td>
                <td>${(w.waiting_tasks_count || 0).toLocaleString()}</td>
                <td class="text-warning">${(w.wait_time_ms || 0).toLocaleString()}</td>
            </tr>
        `).join('');
    } catch (e) {
        loader.style.display = 'none';
        errorDiv.textContent = `Error: ${e.message}`;
        errorDiv.style.display = 'block';
    }
};

window.loadLiveConnections = async function(instanceName) {
    const loader = document.getElementById('connections-loader');
    const errorDiv = document.getElementById('connections-error');
    const tbody = document.getElementById('connectionsBody');

    loader.style.display = 'flex';
    errorDiv.style.display = 'none';

    try {
        const db = (window.appState.currentDatabase && window.appState.currentDatabase !== 'all') ? window.appState.currentDatabase : '';
        const qs = db ? `&database=${encodeURIComponent(db)}` : '';
        const res = await window.apiClient.authenticatedFetch(`/api/live/connections?instance=${encodeURIComponent(instanceName)}${qs}`);
        const data = await res.json();

        loader.style.display = 'none';

        if (!data.success) {
            errorDiv.textContent = `Error: ${data.error}${data.timeout ? ' (10s timeout)' : ''}`;
            errorDiv.style.display = 'block';
            return;
        }

        const connections = data.data || [];

        if (connections.length === 0) {
            tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">No connection data</td></tr>';
            return;
        }

        tbody.innerHTML = connections.map(c => `
            <tr>
                <td style="max-width:120px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${window.escapeHtml(c.program_name || 'Unknown')}">${window.escapeHtml(c.program_name || 'Unknown')}</td>
                <td><span class="badge badge-info">${c.connection_count || 0}</span></td>
                <td>${c.unique_logins || 0}</td>
            </tr>
        `).join('');
    } catch (e) {
        loader.style.display = 'none';
        errorDiv.textContent = `Error: ${e.message}`;
        errorDiv.style.display = 'block';
    }
};
