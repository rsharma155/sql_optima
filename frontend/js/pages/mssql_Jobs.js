window.escapeHtml = function(unsafe) { return (!unsafe) ? '' : unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;"); };

window.JobsView = function() {
    // Set default auto-refresh to 15s if it hasn't been set yet
    if (window.appState.jobsRate === undefined) {
        if(window.setJobsRefresh) window.setJobsRefresh(15000);
        else window.appState.jobsRate = 15000;
    }

    if (!document.getElementById('jobsAutoRefreshSelector')) {
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme" style="display:flex; justify-content:center; align-items:center; height:100%;">
                <div class="spinner"></div><span style="margin-left: 1rem;">Pulling Complete MSDB Job Executions Natively...</span>
            </div>
        `;
    }

    setTimeout(async () => {
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
        try {
            const response = await window.apiClient.authenticatedFetch(`/api/mssql/jobs?instance=${encodeURIComponent(inst.name)}`);
            if (!response.ok) throw new Error("Jobs API fetch failed natively");
            const metrics = await response.json();
            
            try {
                // Persist source badge (Jobs is live MSDB currently)
                window.appState.jobsDataSource = response.headers.get('X-Data-Source') || 'live_cache';
                renderJobsDashboard(inst, metrics);
            } catch(e) {
                console.error("DOM Rendering crash mapping Job Structs:", e);
                window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-danger">UI Rendering Failed</h3><pre>${window.escapeHtml(String(e))}</pre></div>`;
            }
        } catch(error) {
            console.error("Jobs trace lockout:", error);
            const errorMsg = error.message || "Unknown error occurred";
            window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-danger">Error fetching MSDB Agent logic.</h3><p class="text-muted">${window.escapeHtml(errorMsg)}</p></div>`;
        }
    }, 100);
}

function parseRunTime(runDate, runTime) {
    if(!runDate || runDate === 0) return "Never Run";
    const dStr = runDate.toString(); const tStr = runTime.toString().padStart(6, '0');
    return `${dStr.substring(0,4)}-${dStr.substring(4,6)}-${dStr.substring(6,8)} ${tStr.substring(0,2)}:${tStr.substring(2,4)}:${tStr.substring(4,6)}`;
}

function formatStringColor(status) {
    if(status === 'Failed') return 'badge-danger';
    if(status === 'Succeeded') return 'badge-success';
    if(status === 'Running') return 'badge-warning';
    if(status === 'Retry') return 'badge-primary';
    return 'badge-outline';
}

function renderJobsDashboard(inst, metrics) {
    const sums = metrics.summary || {total_jobs:0, enabled_jobs:0, disabled_jobs:0, failed_jobs:0, running_jobs:0};
    const jList = metrics.jobs || [];
    const sched = metrics.schedules || [];
    const fails = metrics.failures || [];

    let jHtml = jList.map(j => `
        <tr>
            <td><strong>${window.escapeHtml(j.job_name)}</strong></td>
            <td><span class="${j.enabled ? 'badge badge-success' : 'badge badge-outline'}">${j.enabled ? 'Yes' : 'No'}</span></td>
            <td><span class="${j.current_status === 'Running' ? 'badge badge-warning' : 'badge badge-outline'}"><i class="${j.current_status === 'Running' ? 'fa-solid fa-spinner fa-spin' : 'fa-solid fa-bed'}"></i> ${window.escapeHtml(j.current_status)}</span></td>
            <td>${j.created_date ? j.created_date.substring(0, 10) : 'N/A'}</td>
            <td>${parseRunTime(j.last_run_date, j.last_run_time)}</td>
            <td><span class="badge ${formatStringColor(j.last_run_status)}">${window.escapeHtml(j.last_run_status)}</span></td>
            <td><span class="text-warning">${window.escapeHtml(j.owner)}</span></td>
        </tr>
    `).join('');

    let sHtml = sched.map(s => {
        const timeDisplay = s.next_run_datetime ? s.next_run_datetime.substring(0, 19).replace('T', ' ') : 'N/A';
        const isEnabled = s.job_enabled ? '<span class="badge badge-success">Yes</span>' : '<span class="badge badge-outline">No</span>';
        const isSchedActive = s.status === 'Active' ? 'text-success' : 'text-warning';
        return `
            <tr>
                <td><strong>${window.escapeHtml(s.job_name)}</strong></td>
                <td>${window.escapeHtml(s.schedule_name)}</td>
                <td>${isEnabled}</td>
                <td><span class="${isSchedActive}">${window.escapeHtml(s.status)}</span></td>
                <td><i class="fa-regular fa-clock text-accent"></i> ${timeDisplay}</td>
            </tr>
        `;
    }).join('');

    let fHtml = fails.map((f, idx) => {
        const message = f.message || '';
        const truncatedMsg = message.length > 100 ? message.substring(0, 100) + '...' : message;
        const escapedMsg = window.escapeHtml(message);
        return `
        <tr>
            <td><strong>${window.escapeHtml(f.job_name)}</strong></td>
            <td><span class="badge badge-outline">${window.escapeHtml(f.step_name)}</span></td>
            <td style="max-width:400px;">
                <span style="font-size:0.75rem; color:var(--text-primary); cursor:pointer;" 
                      title="Click to see full message"
                      onclick="window.showJobFailureDetail(${idx}, '${escapedMsg.replace(/'/g, "\\'").replace(/"/g, '&quot;')}')">
                    ${window.escapeHtml(truncatedMsg)}
                    ${message.length > 100 ? '<span style="color:var(--accent);"> [more]</span>' : ''}
                </span>
            </td>
            <td style="white-space:nowrap; font-size:0.75rem;">${parseRunTime(f.run_date, f.run_time)}</td>
        </tr>
    `}).join('');
    
    window.appState.jobFailureMessages = fails.map(f => f.message || '');

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1>SQL Agent Jobs</h1>
                    <span class="subtitle">Instance: ${window.escapeHtml(inst.name)} | MSDB jobs &amp; schedules</span>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="gap: 0.5rem; align-items: center; flex-wrap: wrap; justify-content: flex-end;">
                    <span id="jobsDataSourceBadge" class="badge badge-info" style="display:none; font-size:0.65rem;">Source</span>
                    <span class="text-muted" style="font-size:0.75rem;">Auto refresh:</span>
                    <select id="jobsAutoRefreshSelector" class="custom-select" onchange="window.setJobsRefresh(this.value)" style="padding:0.25rem; font-size:0.8rem; min-width:100px;">
                        <option value="0">Off</option>
                        <option value="10000" ${window.appState.jobsRate === 10000 ? 'selected' : ''}>10s</option>
                        <option value="15000" ${window.appState.jobsRate === 15000 ? 'selected' : ''}>15s</option>
                        <option value="30000" ${window.appState.jobsRate === 30000 ? 'selected' : ''}>30s</option>
                    </select>
                    <button class="btn btn-sm btn-outline text-accent" onclick="window.appNavigate('dashboard')"><i class="fa-solid fa-arrow-left"></i> Dashboard</button>
                </div>
            </div>

            <div class="glass-panel dashboard-strip-panel mt-3">
                <div class="dashboard-strip-header">
                    <h4><i class="fa-solid fa-briefcase text-accent"></i> Job summary</h4>
                </div>
                <div style="padding:0.65rem;">
            <div class="metrics-grid" style="display: grid; grid-template-columns: repeat(5, 1fr); gap: 0.5rem;">
                <div class="metric-card glass-panel" style="padding: 0.5rem; text-align:center;">
                    <div class="metric-header" style="font-size:0.65rem; margin-bottom:0.25rem; color:var(--text-muted);">Total Jobs</div>
                    <div class="metric-value" style="font-size:1.5rem; color:var(--text-primary);">${sums.total_jobs}</div>
                </div>

                <div class="metric-card glass-panel status-healthy" style="padding: 0.5rem; text-align:center;">
                    <div class="metric-header" style="font-size:0.65rem; margin-bottom:0.25rem; color:var(--text-muted);">Enabled</div>
                    <div class="metric-value" style="font-size:1.5rem; color:#22c55e; font-weight:600;">${sums.enabled_jobs}</div>
                </div>

                <div class="metric-card glass-panel" style="padding: 0.5rem; text-align:center;">
                    <div class="metric-header" style="font-size:0.65rem; margin-bottom:0.25rem; color:var(--text-muted);">Disabled</div>
                    <div class="metric-value" style="font-size:1.5rem; color:var(--text-secondary, #9ca3af);">${sums.disabled_jobs}</div>
                </div>

                <div class="metric-card glass-panel ${sums.running_jobs > 0 ? 'status-warning' : 'status-healthy'}" style="padding: 0.5rem; text-align:center;">
                    <div class="metric-header" style="font-size:0.65rem; margin-bottom:0.25rem; color:var(--text-muted);">Running</div>
                    <div class="metric-value" style="font-size:1.5rem; color:${sums.running_jobs > 0 ? '#f59e0b' : 'var(--text-primary)'}; font-weight:600;">${sums.running_jobs}</div>
                </div>

                <div class="metric-card glass-panel ${sums.failed_jobs > 0 ? 'status-danger' : 'status-healthy'}" style="padding: 0.5rem; text-align:center; cursor:pointer;" onclick="document.getElementById('JobFailAnchor').scrollIntoView({behavior: 'smooth'})">
                    <div class="metric-header" style="font-size:0.65rem; margin-bottom:0.25rem; color:var(--text-muted);">Failed (24h)</div>
                    <div class="metric-value" style="font-size:1.5rem; color:${sums.failed_jobs > 0 ? '#ef4444' : 'var(--text-primary)'}; font-weight:600;">${sums.failed_jobs}</div>
                </div>
            </div>
                </div>
            </div>

            <!-- Chart -->
            <div class="chart-card glass-panel mt-3" style="padding: 0.75rem;">
                <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">Failures Frequency (Last 24h)</h3></div>
                <div class="chart-container" style="height: 140px;"><canvas id="jobsFailuresChart"></canvas></div>
            </div>

            <div class="tables-grid mt-3" style="grid-template-columns: 1fr 1fr; gap: 0.5rem;">
                <div class="table-card glass-panel">
                    <div class="card-header flex-between">
                        <h3 style="font-size:0.85rem; margin:0;">Job List</h3>
                    </div>
                    <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr><th>Job Name</th><th>Enabled</th><th>Status</th><th>Date Created</th><th>Last Run</th><th>Result</th><th>Owner</th></tr></thead>
                            <tbody>${jHtml || '<tr><td colspan="7" class="text-center text-muted">No jobs found</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel">
                    <div class="card-header">
                        <h3 style="font-size:0.85rem; margin:0;">Schedules</h3>
                    </div>
                    <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr><th>Job</th><th>Schedule</th><th>Enabled</th><th>Status</th><th>Next Run</th></tr></thead>
                            <tbody>${sHtml || '<tr><td colspan="5" class="text-center text-muted">No schedules</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>
            </div>

            <div class="tables-grid mt-3" id="JobFailAnchor">
                <div class="table-card glass-panel" style="grid-column: span 2;">
                    <div class="card-header">
                        <h3 style="font-size:0.85rem; margin:0; color:var(--danger);">Recent Failures</h3>
                    </div>
                    <div class="table-responsive" style="max-height:200px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead><tr><th>Job</th><th>Step</th><th>Message</th><th>Run Time</th></tr></thead>
                            <tbody>${fHtml || '<tr><td colspan="4" class="text-center text-success"><i class="fa-solid fa-check"></i> No failures</td></tr>'}</tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    `;

    // Use the shared source badge helper if available.
    if (window.updateSourceBadge) {
        window.updateSourceBadge('jobsDataSourceBadge', window.appState.jobsDataSource || 'live_cache');
    }

    setTimeout(() => initJobsCharts(metrics), 50);
}

function initJobsCharts(metrics) {
    if(window.currentCharts && window.currentCharts.jobsFail) window.currentCharts.jobsFail.destroy();
    window.currentCharts = window.currentCharts || {};
    
    // Generate a quick frequency map of the last 100 historical failures by Date
    let fails = metrics.failures || [];
    let freq = {};
    fails.forEach(f => {
        let d = f.run_date.toString();
        let formatted = d ? `${d.substring(0,4)}-${d.substring(4,6)}-${d.substring(6,8)}` : "Unknown";
        freq[formatted] = (freq[formatted] || 0) + 1;
    });

    let keys = Object.keys(freq).sort();
    let vals = keys.map(k => freq[k]);

    if(keys.length === 0) { keys = ["No Target Data"]; vals = [0]; }

    const ctx = document.getElementById('jobsFailuresChart').getContext('2d');
    const gradFail = ctx.createLinearGradient(0,0,0,200); gradFail.addColorStop(0, 'rgba(239, 68, 68, 0.4)'); gradFail.addColorStop(1, 'rgba(239, 68, 68, 0.0)');

    window.currentCharts.jobsFail = new Chart(ctx, {
        type: 'line', data: {
            labels: keys,
            datasets: [{ label:'Failed Target Phases', data: vals, borderColor:window.getCSSVar('--danger'), backgroundColor:gradFail, fill:true, tension:0.3, pointRadius:4 }]
        }, options: { responsive:true, maintainAspectRatio:false, plugins:{legend:{display: false}}, scales:{y:{min:0, ticks:{stepSize: 1}}, x:{grid:{display:true, color:'rgba(255,255,255,0.05)'}}} }
    });
}

window.showJobFailureDetail = function(idx, message) {
    const existingModal = document.getElementById('job-failure-modal');
    if (existingModal) existingModal.remove();

    const modal = document.createElement('div');
    modal.id = 'job-failure-modal';
    modal.style.cssText = 'display:flex; position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.8); align-items:center; justify-content:center;';
    
    modal.innerHTML = `
        <div style="background:var(--bg-surface); margin:2%; padding:20px; border:1px solid var(--border-color,#333); border-radius:12px; width:95%; max-width:800px; max-height:80vh; overflow-y:auto; color:var(--text-primary,#e0e0e0); box-shadow:0 4px 20px rgba(0,0,0,0.5);">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:1rem; border-bottom:1px solid var(--border-color,#333); padding-bottom:0.75rem;">
                <h3 style="margin:0; color:var(--danger,#ef4444); font-size:1.1rem;"><i class="fa-solid fa-circle-exclamation"></i> Job Failure Details</h3>
                <button onclick="document.getElementById('job-failure-modal').remove()" style="background:transparent; border:1px solid var(--border-color,#555); color:var(--text-primary,#e0e0e0); font-size:1.25rem; cursor:pointer; padding:0.25rem 0.6rem; border-radius:4px; line-height:1;">&times;</button>
            </div>
            <div style="background:var(--bg-base); padding:1rem; border-radius:8px; border:1px solid var(--border-color,#333);">
                <pre style="margin:0; white-space:pre-wrap; word-wrap:break-word; color:var(--text-primary,#e0e0e0); font-family:'Courier New',monospace; font-size:0.85rem; line-height:1.5;">${window.escapeHtml(message)}</pre>
            </div>
            <div style="text-align:center; margin-top:1rem;">
                <button id="copyJobFailMsgBtn" style="background:var(--accent,#3b82f6); color:#fff; border:none; padding:0.5rem 1.5rem; border-radius:6px; cursor:pointer; font-size:0.9rem;">
                    <i class="fa-solid fa-copy"></i> Copy Message
                </button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
    
    document.getElementById('copyJobFailMsgBtn').addEventListener('click', function() {
        navigator.clipboard.writeText(message).then(() => {
            this.innerHTML = '<i class="fa-solid fa-check"></i> Copied!';
            setTimeout(() => {
                this.innerHTML = '<i class="fa-solid fa-copy"></i> Copy Message';
            }, 1500);
        });
    });

    modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.remove();
    });
};
