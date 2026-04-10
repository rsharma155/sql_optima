// DBA War Room - Instance Health Dashboard
appDebug('[InstanceHealthDashboard] Script is loading...');
window.InstanceHealthDashboardView = async function() {
    appDebug('[InstanceHealthDashboard] Starting');
    
    // Ensure config is loaded
    if (!window.appState.config || !window.appState.config.instances) {
        appDebug('[InstanceHealthDashboard] Waiting for config...');
        setTimeout(() => window.InstanceHealthDashboardView(), 500);
        return;
    }
    
    if (window.appState.config.instances.length === 0) {
        window.routerOutlet.innerHTML = '<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">No instances configured</h3></div>';
        return;
    }
    
    // Get current instance - prefer stored index, otherwise use first
    const idx = window.appState.currentInstanceIdx ?? 0;
    const inst = window.appState.config.instances[idx];
    appDebug('[InstanceHealthDashboard] Using instance index:', idx, 'instance:', inst);
    
    if (!inst) {
        window.routerOutlet.innerHTML = '<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">Please select an instance first</h3></div>';
        return;
    }
    
    window.appState.currentInstanceName = inst.name;
    appDebug('[InstanceHealthDashboard] Instance name:', inst.name);
    
    window.routerOutlet.innerHTML = '<div class="page-view active dashboard-sky-theme"><div class="page-title flex-between"><div><h1>Instance Health Dashboard</h1><p class="subtitle">Instance: ' + window.escapeHtml(inst.name) + ' | Health & Anomaly Detection</p></div><div class="flex-between" style="align-items:center; gap:1rem;"><span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="healthDashboardLastUpdate">--:--:--</span></span><button class="btn btn-sm btn-outline text-accent" onclick="window.InstanceHealthDashboardView()"><i class="fa-solid fa-refresh"></i> Refresh</button></div></div><div style="display:flex; justify-content:center; align-items:center; height:50vh;"><div class="spinner"></div><span style="margin-left:1rem;">Loading health metrics...</span></div></div>';

    await window.loadHealthDashboard().catch(e => {
        appDebug('[HealthDashboard] Load failed:', e);
    });
};

window.loadHealthDashboard = async function() {
    const idx = window.appState.currentInstanceIdx ?? 0;
    const inst = window.appState.config?.instances?.[idx];
    appDebug('[HealthDashboard] Loading for instance idx:', idx, 'instance:', inst);
    
    if (!inst || !inst.name) {
        document.querySelector('.spinner')?.closest('div')?.parentElement?.querySelector('.page-view') && (document.querySelector('.spinner').closest('div').innerHTML = '<p class="text-warning">No instance selected</p>');
        window.renderHealthDashboard(inst, { score: 100, cpu_deviation: 1.0, blocked_sessions: 0 }, [], [], [], [], { cpu_history: [], batch_history: [], disk_history: [], baselines: {} });
        return;
    }

    try {
        appDebug('[HealthDashboard] Fetching health score...');
        
        let healthScore = { score: 100, cpu_deviation: 1.0, blocked_sessions: 0 };
        let anomalies = [];
        let regressedQueries = [];
        let waitSpikes = [];
        let incidents = [];
        let metricsHistory = { cpu_history: [], batch_history: [], disk_history: [], baselines: {} };
        let timescaleAvailable = true;

        const serverQ = encodeURIComponent(inst.name);
        const safeJson = async (res, fallback) => {
            try { return await res.json(); } catch (e) { return fallback; }
        };

        const [
            healthScoreRes,
            anomaliesRes,
            regressedRes,
            waitSpikesRes,
            timelineRes,
            metricsHistoryRes
        ] = await Promise.all([
            window.apiClient.authenticatedFetch('/api/health/score?server=' + serverQ),
            window.apiClient.authenticatedFetch('/api/health/anomalies?server=' + serverQ),
            window.apiClient.authenticatedFetch('/api/health/regressed-queries?server=' + serverQ),
            window.apiClient.authenticatedFetch('/api/health/wait-spikes?server=' + serverQ),
            window.apiClient.authenticatedFetch('/api/incidents/timeline?server=' + serverQ + '&hours=24'),
            window.apiClient.authenticatedFetch('/api/health/metrics-history?server=' + serverQ),
        ]);

        // Timescale availability signal: Score returns 503 when TimescaleDB pool is nil.
        // Other endpoints may return 200 with an "error" field in JSON.
        if (healthScoreRes && healthScoreRes.status === 503) {
            timescaleAvailable = false;
        }

        if (healthScoreRes.ok) {
            healthScore = await safeJson(healthScoreRes, healthScore);
            appDebug('[HealthDashboard] Health score:', healthScore);
        } else {
            const hs = await safeJson(healthScoreRes, {});
            if (hs && typeof hs.error === 'string' && hs.error.toLowerCase().includes('timescaledb')) {
                timescaleAvailable = false;
            }
        }
        if (anomaliesRes.ok) {
            const a = await safeJson(anomaliesRes, {});
            anomalies = a.anomalies || [];
            if (a && typeof a.error === 'string' && a.error.toLowerCase().includes('timescaledb')) {
                timescaleAvailable = false;
            }
        }
        if (regressedRes.ok) {
            const rq = await safeJson(regressedRes, {});
            regressedQueries = rq.regressed_queries || [];
            if (rq && typeof rq.error === 'string' && rq.error.toLowerCase().includes('timescaledb')) {
                timescaleAvailable = false;
            }
        }
        if (waitSpikesRes.ok) {
            const ws = await safeJson(waitSpikesRes, {});
            waitSpikes = ws.wait_spikes || [];
            if (ws && typeof ws.error === 'string' && ws.error.toLowerCase().includes('timescaledb')) {
                timescaleAvailable = false;
            }
        }
        if (timelineRes.ok) {
            const tl = await safeJson(timelineRes, {});
            incidents = tl.incidents || [];
            if (tl && typeof tl.error === 'string' && tl.error.toLowerCase().includes('timescaledb')) {
                timescaleAvailable = false;
            }
        }
        if (metricsHistoryRes.ok) {
            metricsHistory = await safeJson(metricsHistoryRes, metricsHistory);
            appDebug('[HealthDashboard] Metrics history:', metricsHistory);
        } else {
            const mh = await safeJson(metricsHistoryRes, {});
            if (mh && typeof mh.error === 'string' && mh.error.toLowerCase().includes('timescaledb')) {
                timescaleAvailable = false;
            }
        }

        appDebug('[HealthDashboard] Rendering with data');
        window.renderHealthDashboard(inst, healthScore, anomalies, regressedQueries, waitSpikes, incidents, metricsHistory, timescaleAvailable);
        
        const lastUpdateEl = document.getElementById('healthDashboardLastUpdate');
        if (lastUpdateEl) lastUpdateEl.textContent = new Date().toLocaleString();

    } catch (err) {
        console.error('[Health Dashboard] Error loading data:', err);
        window.renderHealthDashboard(inst, { score: 100, cpu_deviation: 1.0, blocked_sessions: 0 }, [], [], [], [], { cpu_history: [], batch_history: [], disk_history: [], baselines: {} }, false);
    }
};

function renderHealthDashboard(inst, healthScore, anomalies, regressedQueries, waitSpikes, incidents, metricsHistory, timescaleAvailable) {
    appDebug('[renderHealthDashboard] Rendering with inst:', inst, 'healthScore:', healthScore);
    
    const instanceName = inst?.name || window.appState.currentInstanceName || 'Unknown';
    const instance = inst || { name: instanceName };
    
    const score = healthScore?.score ?? 100;
    let scoreClass = 'status-healthy';
    let scoreColor = 'var(--success)';
    if (score < 70) { scoreClass = 'status-danger'; scoreColor = 'var(--danger)'; }
    else if (score < 90) { scoreClass = 'status-warning'; scoreColor = 'var(--warning)'; }

    const activeAlerts = (anomalies || []).length;
    const blockedSessions = healthScore?.blocked_sessions ?? 0;
    const blockingStatus = blockedSessions > 0 
        ? '<span class="badge badge-danger">' + blockedSessions + ' Blocked</span>'
        : '<span class="badge badge-success">No Blocking</span>';

    const baselines = metricsHistory?.baselines || {};
    const cpuBaseline = baselines.cpu || 50;
    const batchBaseline = baselines.batch || 100;
    const diskBaseline = baselines.disk || 10;

    const waitSpikesList = waitSpikes || [];
    const regressedQueriesList = regressedQueries || [];
    const incidentsList = incidents || [];

    var html = '<div class="page-view active dashboard-sky-theme">';
    html += '<div class="page-title flex-between">';
    html += '<div><h1>Instance Health Dashboard</h1><p class="subtitle">Instance: ' + window.escapeHtml(instanceName) + ' | Health & Anomaly Detection</p></div>';
    html += '<div class="flex-between" style="align-items:center; gap:1rem;">';
    html += '<span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="healthDashboardLastUpdate">' + new Date().toLocaleString() + '</span></span>';
    html += '<button class="btn btn-sm btn-outline text-accent" onclick="window.InstanceHealthDashboardView()"><i class="fa-solid fa-refresh"></i> Refresh</button>';
    html += '</div></div>';

    if (timescaleAvailable === false) {
        html += `
            <div class="alert alert-warning mt-3">
                <i class="fa-solid fa-triangle-exclamation"></i>
                <strong>TimescaleDB unavailable.</strong>
                <span class="text-muted">This dashboard reads historical health/anomaly data from TimescaleDB; charts and incident data may be empty until it reconnects.</span>
            </div>
        `;
    }

    // Row 1: 3 KPI Cards
    html += '<div class="metrics-row" style="display:grid; grid-template-columns:repeat(auto-fit, minmax(220px, 1fr)); gap:0.75rem; margin-top:0.75rem;">';
    html += '<div class="metric-card glass-panel ' + scoreClass + '" style="padding:0.6rem 0.75rem; text-align:center;">';
    html += '<div class="metric-header"><span class="metric-title" style="font-size:0.7rem; text-transform:uppercase;">Health Score</span></div>';
    html += '<div class="metric-value" style="font-size:1.6rem; font-weight:700; color:' + scoreColor + ';">' + score + '</div>';
    html += '<div style="font-size:0.65rem; color:var(--text-muted);">CPU Deviation: ' + ((healthScore?.cpu_deviation || 1).toFixed(2)) + 'x</div></div>';
    html += '<div class="metric-card glass-panel" style="padding:0.6rem 0.75rem; text-align:center;">';
    html += '<div class="metric-header"><span class="metric-title" style="font-size:0.7rem; text-transform:uppercase;">Active Alerts</span></div>';
    html += '<div class="metric-value" style="font-size:1.6rem; font-weight:700; color:' + (activeAlerts > 0 ? 'var(--danger)' : 'var(--success)') + ';">' + activeAlerts + '</div>';
    html += '<div style="font-size:0.65rem; color:var(--text-muted);">Last 24 hours</div></div>';
    html += '<div class="metric-card glass-panel" style="padding:0.6rem 0.75rem; text-align:center;">';
    html += '<div class="metric-header"><span class="metric-title" style="font-size:0.7rem; text-transform:uppercase;">Blocking Status</span></div>';
    html += '<div class="metric-value" style="font-size:0.95rem; padding-top:0.35rem;">' + blockingStatus + '</div>';
    html += '<div style="font-size:0.65rem; color:var(--text-muted);">Current sessions</div></div></div>';

    // Row 2: Charts
    html += '<div class="charts-grid mt-3" data-cols="3" style="display:grid; grid-template-columns:repeat(3, 1fr); gap:0.75rem;">';
    html += '<div class="chart-card glass-panel" style="padding:0.75rem;"><div class="card-header"><h3 style="font-size:0.85rem; margin:0;">CPU vs Baseline</h3></div><div class="chart-container" style="height:150px;"><canvas id="cpuBaselineChart"></canvas></div></div>';
    html += '<div class="chart-card glass-panel" style="padding:0.75rem;"><div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Batch Requests vs Baseline</h3></div><div class="chart-container" style="height:150px;"><canvas id="batchReqChart"></canvas></div></div>';
    html += '<div class="chart-card glass-panel" style="padding:0.75rem;"><div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Disk Latency vs Baseline</h3></div><div class="chart-container" style="height:150px;"><canvas id="diskLatencyChart"></canvas></div></div></div>';

    // Row 3: Wait Spikes
    html += '<div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">';
    html += '<div class="table-card glass-panel">';
    html += '<div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-bolt text-warning"></i> Wait Spikes</h3><span class="badge badge-info">' + waitSpikesList.length + '</span></div>';
    html += '<div class="table-responsive" style="max-height:200px; overflow-y:auto;">';
    html += '<table class="data-table" style="font-size:0.75rem;"><thead><tr><th>Wait Type</th><th>Current (ms)</th><th>Baseline (ms)</th><th>Spike Ratio</th><th>Recommendation</th></tr></thead><tbody>';
    
    if (waitSpikesList.length === 0) {
        html += '<tr><td colspan="5" class="text-center text-muted">No wait spikes detected</td></tr>';
    } else {
        for (var i = 0; i < waitSpikesList.length; i++) {
            var w = waitSpikesList[i];
            html += '<tr><td><span class="badge badge-warning">' + window.escapeHtml(w.wait_type) + '</span></td>';
            html += '<td>' + (w.current_wait_ms?.toFixed(2) || 0) + '</td>';
            html += '<td>' + (w.baseline_ms?.toFixed(2) || 0) + '</td>';
            html += '<td><span class="badge ' + (w.spike_ratio > 3 ? 'badge-danger' : 'badge-warning') + '">' + (w.spike_ratio?.toFixed(1) || 0) + 'x</span></td>';
            html += '<td style="max-width:300px; font-size:0.7rem;">' + window.escapeHtml(w.recommendation || '') + '</td></tr>';
        }
    }
    html += '</tbody></table></div></div></div>';

    // Row 4: Regressed Queries
    html += '<div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">';
    html += '<div class="table-card glass-panel">';
    html += '<div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-arrow-trend-down text-danger"></i> Regressed Queries</h3><span class="badge badge-danger">' + regressedQueriesList.length + '</span></div>';
    html += '<div class="table-responsive" style="max-height:250px; overflow-y:auto;">';
    html += '<table class="data-table" style="font-size:0.75rem; cursor:pointer;"><thead><tr><th>Query</th><th>Exec Count</th><th>Avg Duration (ms)</th><th>Regression %</th></tr></thead><tbody>';
    
    if (regressedQueriesList.length === 0) {
        html += '<tr><td colspan="4" class="text-center text-muted">No regressed queries</td></tr>';
    } else {
        for (var j = 0; j < regressedQueriesList.length; j++) {
            var q = regressedQueriesList[j];
            html += '<tr onclick="window.showQueryAnalysisModal(\'' + encodeURIComponent(q.query_text || q.query_hash) + '\')">';
            html += '<td style="max-width:400px; overflow:hidden; text-overflow:ellipsis;">' + window.escapeHtml((q.query_text || q.query_hash || 'N/A').substring(0, 50)) + '</td>';
            html += '<td>' + q.exec_count + '</td>';
            html += '<td>' + (q.avg_duration_ms?.toFixed(0) || 0) + '</td>';
            html += '<td><span class="badge badge-danger">+' + (q.regression_pct?.toFixed(0) || 0) + '%</span></td></tr>';
        }
    }
    html += '</tbody></table></div></div></div>';

    // Row 5: Incident Timeline
    html += '<div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">';
    html += '<div class="table-card glass-panel">';
    html += '<div class="card-header"><h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-clock-rotate-left text-accent"></i> Incident Timeline (24h)</h3><span class="badge badge-info">' + incidentsList.length + '</span></div>';
    html += '<div class="table-responsive" style="max-height:300px; overflow-y:auto;">';
    html += '<table class="data-table" style="font-size:0.75rem;"><thead><tr><th>Time</th><th>Severity</th><th>Category</th><th>Description</th><th>Recommendations</th></tr></thead><tbody>';
    
    if (incidentsList.length === 0) {
        html += '<tr><td colspan="5" class="text-center text-muted">No incidents in the last 24 hours</td></tr>';
    } else {
        for (var k = 0; k < incidentsList.length; k++) {
            var inc = incidentsList[k];
            html += '<tr><td>' + new Date(inc.timestamp).toLocaleString() + '</td>';
            html += '<td><span class="badge ' + (inc.severity === 'CRITICAL' ? 'badge-danger' : (inc.severity === 'WARNING' ? 'badge-warning' : 'badge-info')) + '">' + inc.severity + '</span></td>';
            html += '<td>' + window.escapeHtml(inc.category) + '</td>';
            html += '<td style="max-width:300px;">' + window.escapeHtml(inc.description || '') + '</td>';
            html += '<td style="max-width:250px; font-size:0.7rem;">' + window.escapeHtml(inc.recommendations || '') + '</td></tr>';
        }
    }
    html += '</tbody></table></div></div></div></div>';

    window.routerOutlet.innerHTML = html;
    initHealthCharts(metricsHistory);
}

function initHealthCharts(metricsHistory) {
    var baselines = metricsHistory?.baselines || {};
    var cpuBaseline = baselines.cpu || 50;
    var batchBaseline = baselines.batch || 100;
    var diskBaseline = baselines.disk || 10;

    var ctxCpu = document.getElementById('cpuBaselineChart');
    if (ctxCpu) {
        var cpuData = (metricsHistory?.cpu_history || []).slice(-30);
        var cpuLabels = cpuData.map(function(d) {
            var t = new Date(d.timestamp);
            return t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        });
        var cpuValues = cpuData.map(function(d) { return d.cpu || 0; });

        new Chart(ctxCpu.getContext('2d'), {
            type: 'line',
            data: {
                labels: cpuLabels,
                datasets: [{
                    label: 'CPU %',
                    data: cpuValues,
                    borderColor: 'var(--primary)',
                    backgroundColor: 'rgba(99, 102, 241, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0
                }, {
                    label: 'Baseline',
                    data: Array(cpuLabels.length).fill(cpuBaseline),
                    borderColor: 'var(--text-muted)',
                    borderDash: [5, 5],
                    fill: false,
                    pointRadius: 0
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'top', labels: { boxWidth: 8 } } },
                scales: {
                    y: { min: 0, max: 100, title: { display: true, text: 'CPU %' } },
                    x: { title: { display: true, text: 'Time' }, ticks: { maxTicksLimit: 8 } }
                }
            }
        });
    }

    var ctxBatch = document.getElementById('batchReqChart');
    if (ctxBatch) {
        var batchData = (metricsHistory?.batch_history || []).slice(-30);
        var batchLabels = batchData.map(function(d) {
            var t = new Date(d.timestamp);
            return t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        });
        var tpsValues = batchData.map(function(d) { return d.tps || 0; });

        new Chart(ctxBatch.getContext('2d'), {
            type: 'line',
            data: {
                labels: batchLabels,
                datasets: [{
                    label: 'Batch Requests/sec',
                    data: tpsValues,
                    borderColor: 'var(--success)',
                    backgroundColor: 'rgba(34, 197, 94, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0
                }, {
                    label: 'Baseline',
                    data: Array(batchLabels.length).fill(batchBaseline),
                    borderColor: 'var(--text-muted)',
                    borderDash: [5, 5],
                    fill: false,
                    pointRadius: 0
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'top', labels: { boxWidth: 8 } } },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'TPS' } },
                    x: { title: { display: true, text: 'Time' }, ticks: { maxTicksLimit: 8 } }
                }
            }
        });
    }

    var ctxDisk = document.getElementById('diskLatencyChart');
    if (ctxDisk) {
        var diskData = (metricsHistory?.disk_history || []).slice(-30);
        var diskLabels = diskData.map(function(d) {
            var t = new Date(d.timestamp);
            return t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        });
        var latencyValues = diskData.map(function(d) { return d.latency || 0; });

        new Chart(ctxDisk.getContext('2d'), {
            type: 'line',
            data: {
                labels: diskLabels,
                datasets: [{
                    label: 'Disk Latency (ms)',
                    data: latencyValues,
                    borderColor: 'var(--warning)',
                    backgroundColor: 'rgba(245, 158, 11, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0
                }, {
                    label: 'Baseline',
                    data: Array(diskLabels.length).fill(diskBaseline),
                    borderColor: 'var(--text-muted)',
                    borderDash: [5, 5],
                    fill: false,
                    pointRadius: 0
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'top', labels: { boxWidth: 8 } } },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'Latency (ms)' } },
                    x: { title: { display: true, text: 'Time' }, ticks: { maxTicksLimit: 8 } }
                }
            }
        });
    }
}

window.showQueryAnalysisModal = function(queryData) {
    var queryText = decodeURIComponent(queryData);
    alert('Query Analysis Modal\n\nQuery: ' + queryText + '\n\n(Implement detailed query analysis modal here)');
};

console.log('[InstanceHealthDashboard] Script fully loaded');
