window.LocksDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'sqlserver') {
        window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">Select a SQL Server instance first</h3></div>`;
        return;
    }
    
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <button class="btn btn-sm btn-outline mb-2" onclick="window.appNavigate('dashboard')"><i class="fa-solid fa-arrow-left"></i> Back to Dashboard</button>
                    <h1>Blocking & Wait Types Drilldown</h1>
                    <p class="subtitle">Identify Lead Blockers and critical wait bottlenecks</p>
                </div>
                <div class="filter-group glass-panel p-3 rounded flex-between" style="width: 400px;">
                    <span>Wait Category:</span>
                    <select class="custom-select w-50" id="waitCategoryFilter" onchange="window.loadBlockingData()">
                        <option value="all">All Waits</option>
                        <option value="LCK">LCK (Locks)</option>
                        <option value="PAGEIOLATCH">PAGEIOLATCH</option>
                        <option value="CXPACKET">CXPACKET</option>
                        <option value="ASYNC_NETWORK_IO">ASYNC_NETWORK_IO</option>
                    </select>
                </div>
            </div>
            
            <div class="chart-card glass-panel mt-4" style="height: 350px;">
                <div class="card-header">
                    <h3>Wait Time Distribution (Top Wait Types)</h3>
                </div>
                <div class="chart-container" style="height: 250px;"><canvas id="drillLockChart"></canvas></div>
            </div>

            <div class="table-card glass-panel mt-4 border-danger">
                <div class="card-header">
                    <h3>Active Lead Blockers (Blocking Chain)</h3>
                    <button class="btn btn-sm btn-outline" onclick="window.appNavigate('drilldown-deadlock')"><i class="fa-solid fa-skull text-danger"></i> View Deadlocks</button>
                </div>
                <div class="table-responsive" id="blocking-table-container">
                    <div style="display:flex; justify-content:center; align-items:center; height:100px;">
                        <div class="spinner"></div><span style="margin-left:1rem;">Loading blocking data...</span>
                    </div>
                </div>
            </div>
        </div>
    `;

    await window.loadBlockingData();
};

window.loadBlockingData = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    
    const container = document.getElementById('blocking-table-container');
    if (!container) return;
    
    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/mssql/dashboard?instance=${encodeURIComponent(inst.name)}`
        );
        if (!response.ok) throw new Error('Failed to fetch');
        const data = await response.json();
        
        const activeBlocks = data.active_blocks || [];
        const blockers = activeBlocks.filter(s => s.blocking_session_id !== 0);
        const leadBlockers = [];
        const seenSpids = new Set();
        
        blockers.forEach(b => {
            if (!seenSpids.has(b.blocking_session_id)) {
                seenSpids.add(b.blocking_session_id);
                const blockedCount = blockers.filter(bb => bb.blocking_session_id === b.blocking_session_id).length;
                leadBlockers.push({
                    spid: b.blocking_session_id,
                    blocked_count: blockedCount,
                    wait_type: b.wait_type,
                    login: b.login_name,
                    host: b.host_name,
                    program: b.program_name,
                    query_text: b.query_text,
                    resource: b.resource
                });
            }
        });
        
        if (leadBlockers.length === 0) {
            container.innerHTML = `
                <table class="data-table">
                    <thead>
                        <tr><th>Head Blocker SPID</th><th>Program / User</th><th>Wait Type</th><th>Blocked Sessions</th><th>Resource</th><th>Kill</th></tr>
                    </thead>
                    <tbody>
                        <tr><td colspan="6" class="text-center text-muted">No active blockers detected</td></tr>
                    </tbody>
                </table>`;
        } else {
            window.appState.queryCache = window.appState.queryCache || {};
            container.innerHTML = `
                <table class="data-table">
                    <thead>
                        <tr><th>Head Blocker SPID</th><th>Program / User</th><th>Wait Type</th><th>Blocked Sessions</th><th>Resource</th><th>Query Preview</th></tr>
                    </thead>
                    <tbody>
                        ${leadBlockers.map(b => `
                            <tr>
                                <td><strong><i class="fa-solid fa-link text-warning"></i> ${b.spid}</strong></td>
                                <td>${window.escapeHtml(b.program || 'N/A')} <br/><span class="text-muted">${window.escapeHtml(b.login || 'N/A')}</span></td>
                                <td><span class="badge badge-danger">${window.escapeHtml(b.wait_type || 'N/A')}</span></td>
                                <td><span class="text-danger font-bold">${b.blocked_count}</span></td>
                                <td style="font-size:0.75rem;">${window.escapeHtml(b.resource || 'N/A')}</td>
                                <td style="font-size:0.7rem; max-width:260px;">
                                    <span class="code-snippet" style="cursor:pointer; display:inline-block; max-width:240px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;"
                                          title="${window.escapeHtml(b.query_text || '')}"
                                          onclick="window.showQueryModalDirect(window.appState.queryCache['lock_${b.spid}'])">
                                        ${window.escapeHtml((b.query_text || 'N/A')).substring(0, 80)}${(b.query_text && b.query_text.length > 80) ? '…' : ''}
                                    </span>
                                </td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>`;
            leadBlockers.forEach(b => { window.appState.queryCache['lock_' + b.spid] = b.query_text || ''; });
        }
        
        // Update chart with wait stats
        initWaitChart(data);
        
    } catch (error) {
        appDebug('Failed to load blocking data:', error);
        container.innerHTML = `<div class="alert alert-danger">Failed to load blocking data: ${window.escapeHtml(String(error && error.message ? error.message : error))}</div>`;
    }
};

function initWaitChart(data) {
    if (window.currentCharts && window.currentCharts.drillLck) {
        window.currentCharts.drillLck.destroy();
    }
    
    const waitStats = data.wait_stats || [];
    if (waitStats.length === 0) {
        const chartContainer = document.getElementById('drillLockChart')?.parentElement;
        if (chartContainer) {
            chartContainer.innerHTML += '<div style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:var(--text-muted);">No wait stats available</div>';
        }
        return;
    }
    
    const labels = waitStats.slice(0, 10).map(w => w.wait_type || 'Unknown');
    const waitTimes = waitStats.slice(0, 10).map(w => w.wait_time_ms || 0);
    
    const ctx = document.getElementById('drillLockChart')?.getContext('2d');
    if (ctx) {
        window.currentCharts.drillLck = new Chart(ctx, {
            type: 'bar',
            data: {
                labels: labels,
                datasets: [{
                    label: 'Wait Time (ms)',
                    data: waitTimes,
                    backgroundColor: 'rgba(239, 68, 68, 0.7)',
                    borderColor: 'rgba(239, 68, 68, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false }
                },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'ms' } },
                    x: { ticks: { maxRotation: 45, minRotation: 45 } }
                }
            }
        });
    }
}
