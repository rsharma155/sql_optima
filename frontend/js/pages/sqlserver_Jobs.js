/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Agent job monitoring page with job status and history.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.escapeHtml = function(unsafe) { return (!unsafe) ? '' : unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;"); };

window.JobsView = function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between" style="padding: 10px 20px; background: var(--bg-primary); border-bottom: 1px solid var(--border-color); height: 65px;">
                <div>
                    <h1 style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:10px;">
                        <i class="fa-solid fa-briefcase text-accent"></i>
                        SQL Agent Jobs
                    </h1>
                    <div style="display:flex; gap:12px; align-items:center; margin-top:2px;">
                        <span style="font-size:0.7rem; color:var(--text-secondary); font-weight:600;"><i class="fa-solid fa-server" style="font-size:0.6rem;"></i> ${window.escapeHtml(inst.name)}</span>
                        <span style="font-size:0.7rem; color:var(--text-muted);"><i class="fa-solid fa-clock-rotate-left" style="font-size:0.6rem;"></i> Time-series status &amp; history</span>
                    </div>
                </div>
                <div class="flex-between" style="align-items:center; gap:1rem;">
                    <div id="time-picker-insertion-point"></div>
                    <div class="text-muted" style="font-size:0.65rem; background: rgba(0,0,0,0.2); padding: 4px 8px; border-radius: 4px;">
                        Update: <span id="jobs-last-update" class="text-accent">--:--:--</span>
                    </div>
                </div>
            </div>

            <div id="jobsDashboardContent">
                <div style="display:flex; justify-content:center; align-items:center; height:300px;">
                    <div class="spinner"></div><span style="margin-left: 1rem;">Analyzing job states for selected period...</span>
                </div>
            </div>
        </div>
    `;

    if (window.initPageTimePicker) window.initPageTimePicker();

    refreshJobsData(inst);
}

async function refreshJobsData(inst) {
    const fromLocal = window.appState.fromTs;
    const toLocal = window.appState.toTs;
    const content = document.getElementById('jobsDashboardContent');
    if (!content) return;

    let from = fromLocal || '';
    let to = toLocal || '';

    // Convert local datetime-local strings to UTC ISO for the backend
    if (from && !from.includes('Z')) {
        try { from = new Date(from).toISOString(); } catch(e) {}
    }
    if (to && !to.includes('Z')) {
        try { to = new Date(to).toISOString(); } catch(e) {}
    }
    
    try {
        const url = `/api/sqlserver/jobs?instance=${encodeURIComponent(inst.name)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;
        const response = await window.apiClient.authenticatedFetch(url);
        if (!response.ok) {
            const errBody = await response.json().catch(() => ({}));
            throw new Error(errBody.error || "Jobs API failed");
        }
        const data = await response.json();
        
        renderJobsContent(inst, data);
        const updateEl = document.getElementById('jobs-last-update');
        if (updateEl) updateEl.textContent = new Date().toLocaleTimeString();
    } catch(err) {
        content.innerHTML = `<div class="alert alert-danger" style="margin-top:1rem;">
            <i class="fa-solid fa-triangle-exclamation"></i> <strong>Historical Data Unavailable:</strong> ${window.escapeHtml(err.message)}
            <p class="small mt-2">Ensure the TimescaleDB collector is running and has processed at least one tick for this instance.</p>
        </div>`;
    }
}

function parseRunTime(runDate, runTime) {
    if(!runDate || runDate === 0) return "Never Run";
    const dStr = runDate.toString(); const tStr = runTime.toString().padStart(6, '0');
    if (dStr.length < 8) return "Invalid Date";
    return `${dStr.substring(0,4)}-${dStr.substring(4,6)}-${dStr.substring(6,8)} ${tStr.substring(0,2)}:${tStr.substring(2,4)}:${tStr.substring(4,6)}`;
}

function formatStringColor(status) {
    if(status === 'Failed') return 'badge-danger';
    if(status === 'Succeeded') return 'badge-success';
    if(status === 'Running') return 'badge-warning';
    if(status === 'Retry') return 'badge-primary';
    return 'badge-outline';
}

function renderJobsContent(inst, metrics) {
    const sums = metrics.summary || {total_jobs:0, enabled_jobs:0, disabled_jobs:0, failed_jobs:0, running_jobs:0};
    const jList = metrics.jobs || [];
    const sched = metrics.schedules || [];
    const fails = metrics.failures || [];

    const content = document.getElementById('jobsDashboardContent');
    if (!content) return;

    // Store state for pagination
    window.appState.allJobs = jList;
    window.appState.jobsPage = 1;
    const pageSize = 15;

    content.innerHTML = `
        <div class="metrics-grid mt-2" style="display: grid; grid-template-columns: repeat(5, 1fr); gap: 0.5rem;">
            <div class="metric-card glass-panel" style="padding: 0.5rem 0.75rem; text-align:center;">
                <div class="metric-header" style="font-size:0.65rem; color:var(--text-muted); text-transform:uppercase;">Total Jobs</div>
                <div class="metric-value" style="font-size:1.25rem; font-weight:700;">${sums.total_jobs}</div>
            </div>
            <div class="metric-card glass-panel" style="padding: 0.5rem 0.75rem; text-align:center; border-bottom: 3px solid var(--success);">
                <div class="metric-header" style="font-size:0.65rem; color:var(--text-muted); text-transform:uppercase;">Enabled</div>
                <div class="metric-value" style="font-size:1.25rem; font-weight:700; color:var(--success);">${sums.enabled_jobs}</div>
            </div>
            <div class="metric-card glass-panel" style="padding: 0.5rem 0.75rem; text-align:center;">
                <div class="metric-header" style="font-size:0.65rem; color:var(--text-muted); text-transform:uppercase;">Disabled</div>
                <div class="metric-value" style="font-size:1.25rem; font-weight:700; color:var(--text-muted);">${sums.disabled_jobs}</div>
            </div>
            <div class="metric-card glass-panel" style="padding: 0.5rem 0.75rem; text-align:center; border-bottom: 3px solid var(--warning);">
                <div class="metric-header" style="font-size:0.65rem; color:var(--text-muted); text-transform:uppercase;">Running</div>
                <div class="metric-value" style="font-size:1.25rem; font-weight:700; color:var(--warning);">${sums.running_jobs}</div>
            </div>
            <div class="metric-card glass-panel" style="padding: 0.5rem 0.75rem; text-align:center; border-bottom: 3px solid var(--danger);">
                <div class="metric-header" style="font-size:0.65rem; color:var(--text-muted); text-transform:uppercase;">Failed</div>
                <div class="metric-value" style="font-size:1.25rem; font-weight:700; color:var(--danger);">${sums.failed_jobs}</div>
            </div>
        </div>

        <div class="chart-card glass-panel mt-2" style="padding: 0.5rem 0.75rem; max-height: 200px; overflow:hidden;">
            <div class="card-header" style="padding:0 0 0.25rem 0;"><h3 style="font-size:0.78rem; margin:0;"><i class="fa-solid fa-chart-area text-accent"></i> Job Failure Trend</h3></div>
            <div class="chart-container" style="height: 150px;"><canvas id="jobsFailuresChart"></canvas></div>
        </div>

        <div class="tabs-container mt-3">
            <button class="tab-btn active" data-tab="list">Job Inventory</button>
            <button class="tab-btn" data-tab="schedules">Next Runs</button>
            <button class="tab-btn" data-tab="failures">Failure History</button>
        </div>

        <div id="jobTab-list" class="tab-panel mt-2">
            <div class="table-card glass-panel" style="padding:0; min-height:400px; background: rgba(15, 23, 42, 0.4);">
                <div class="table-responsive" id="jobInventoryTableWrap">
                    <!-- Table content will be injected by pagination -->
                </div>
                <div id="jobInventoryPagination" class="pagination-footer mt-2 p-2 flex-between" style="border-top: 1px solid var(--border-color); display: flex;">
                    <!-- Pagination controls -->
                </div>
            </div>
        </div>

        <div id="jobTab-schedules" class="tab-panel mt-2" style="display:none;">
            <div class="table-card glass-panel" style="padding:0; background: rgba(15, 23, 42, 0.4);">
                <table class="modern-table modern-table-compact">
                    <thead><tr><th>Job</th><th>Schedule</th><th>Active</th><th>Next Expected Run</th></tr></thead>
                    <tbody>
                        ${sched.map(s => `
                            <tr>
                                <td><strong>${window.escapeHtml(s.job_name)}</strong></td>
                                <td><span class="small">${window.escapeHtml(s.schedule_name)}</span></td>
                                <td>
                                    <span class="badge ${s.status === 'Active' ? 'badge-success' : 'badge-outline'}" style="font-size:0.6rem;">
                                        ${s.status}
                                    </span>
                                </td>
                                <td class="small"><i class="fa-regular fa-clock text-accent"></i> ${s.next_run_datetime ? s.next_run_datetime.substring(0, 19).replace('T', ' ') : 'N/A'}</td>
                            </tr>
                        `).join('') || '<tr><td colspan="4" class="text-center text-muted">No upcoming schedules</td></tr>'}
                    </tbody>
                </table>
            </div>
        </div>

        <div id="jobTab-failures" class="tab-panel mt-2" style="display:none;">
            <div class="table-card glass-panel" style="padding:0; background: rgba(15, 23, 42, 0.4);">
                <table class="modern-table modern-table-compact">
                    <thead><tr><th>Job</th><th>Step</th><th>Failed At</th><th>Message</th></tr></thead>
                    <tbody>
                        ${fails.map((f, idx) => `
                            <tr>
                                <td><strong class="text-danger">${window.escapeHtml(f.job_name)}</strong></td>
                                <td><span class="badge badge-outline" style="font-size:0.6rem;">${window.escapeHtml(f.step_name)}</span></td>
                                <td style="white-space:nowrap;" class="small text-muted">${parseRunTime(f.run_date, f.run_time)}</td>
                                <td class="small" style="max-width:500px; cursor:pointer;" data-action="call" data-fn="showJobFailureDetail" data-idx="${idx}">
                                    <div class="text-truncate-2">${window.escapeHtml(f.message ? f.message : 'No message')}</div>
                                </td>
                            </tr>
                        `).join('') || '<tr><td colspan="4" class="text-center text-success">No failures in selected range</td></tr>'}
                    </tbody>
                </table>
            </div>
        </div>
    `;

    const renderPage = (page) => {
        const start = (page - 1) * pageSize;
        const end = start + pageSize;
        const pageJobs = window.appState.allJobs.slice(start, end);
        const totalPages = Math.ceil(window.appState.allJobs.length / pageSize);

        const tableWrap = document.getElementById('jobInventoryTableWrap');
        if (!tableWrap) return;

        tableWrap.innerHTML = `
            <table class="modern-table modern-table-compact">
                <thead><tr><th>Job Name</th><th>Category</th><th>Enabled</th><th>Status</th><th>Last Run</th><th>Last Result</th><th>Owner</th></tr></thead>
                <tbody>
                    ${pageJobs.map(j => `
                        <tr>
                            <td>
                                <div style="display:flex; align-items:center; gap:0.5rem;">
                                    <i class="fa-solid fa-gear ${j.current_status === 'Running' ? 'fa-spin text-warning' : 'text-muted'}"></i>
                                    <strong title="${window.escapeHtml(j.description)}">${window.escapeHtml(j.job_name)}</strong>
                                </div>
                            </td>
                            <td><span style="font-size:0.75rem;">${window.escapeHtml(j.category || 'Uncategorized')}</span></td>
                            <td>
                                <span class="badge ${j.enabled ? 'badge-success' : 'badge-outline'}" style="font-size:0.6rem;">
                                    <i class="fa-solid ${j.enabled ? 'fa-check-circle' : 'fa-times-circle'}"></i> ${j.enabled ? 'Enabled' : 'Disabled'}
                                </span>
                            </td>
                            <td>
                                <span class="badge ${j.current_status === 'Running' ? 'badge-warning' : 'badge-outline'}" style="font-size:0.6rem;">
                                    ${window.escapeHtml(j.current_status)}
                                </span>
                            </td>
                            <td class="small">${parseRunTime(j.last_run_date, j.last_run_time)}</td>
                            <td>
                                <div style="display:flex; align-items:center; gap:0.4rem;">
                                    <i class="fa-solid ${j.last_run_status === 'Succeeded' ? 'fa-circle-check text-success' : (j.last_run_status === 'Failed' ? 'fa-circle-xmark text-danger' : 'fa-circle-question text-muted')}"></i>
                                    <span class="badge ${formatStringColor(j.last_run_status)}" style="font-size:0.6rem;">${window.escapeHtml(j.last_run_status)}</span>
                                </div>
                            </td>
                            <td style="font-size:0.75rem;">${window.escapeHtml(j.owner)}</td>
                        </tr>
                    `).join('') || '<tr><td colspan="7" class="text-center text-muted">No jobs found</td></tr>'}
                </tbody>
            </table>
        `;

        const pag = document.getElementById('jobInventoryPagination');
        if (pag) {
            if (totalPages <= 1) {
                pag.style.display = 'none';
            } else {
                pag.style.display = 'flex';
                pag.innerHTML = `
                    <div class="small text-muted">Showing ${start+1}-${Math.min(end, window.appState.allJobs.length)} of ${window.appState.allJobs.length} jobs</div>
                    <div style="display:flex; gap:0.5rem;">
                        <button class="btn btn-xs btn-outline" ${page === 1 ? 'disabled' : ''} id="jobsPrevPage"><i class="fa-solid fa-chevron-left"></i></button>
                        <span class="small text-muted" style="align-self:center;">Page ${page} of ${totalPages}</span>
                        <button class="btn btn-xs btn-outline" ${page === totalPages ? 'disabled' : ''} id="jobsNextPage"><i class="fa-solid fa-chevron-right"></i></button>
                    </div>
                `;
                document.getElementById('jobsPrevPage')?.addEventListener('click', () => { window.appState.jobsPage--; renderPage(window.appState.jobsPage); });
                document.getElementById('jobsNextPage')?.addEventListener('click', () => { window.appState.jobsPage++; renderPage(window.appState.jobsPage); });
            }
        }
    };

    renderPage(1);

    window.appState.jobFailureMessages = fails.map(f => f.message || '');

    // Wire up tabs
    content.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            content.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            content.querySelectorAll('.tab-panel').forEach(p => p.style.display = 'none');
            btn.classList.add('active');
            const panel = document.getElementById(`jobTab-${btn.dataset.tab}`);
            if (panel) panel.style.display = '';
        });
    });

    initJobsCharts(metrics);
}

function initJobsCharts(metrics) {
    if(window.currentCharts && window.currentCharts.jobsFail) window.currentCharts.jobsFail.destroy();
    window.currentCharts = window.currentCharts || {};
    
    let fails = metrics.failures || [];
    let freq = {};
    fails.forEach(f => {
        let d = f.run_date.toString();
        if (d.length >= 8) {
           let formatted = `${d.substring(0,4)}-${d.substring(4,6)}-${d.substring(6,8)}`;
           freq[formatted] = (freq[formatted] || 0) + 1;
        }
    });

    let keys = Object.keys(freq).sort();
    let vals = keys.map(k => freq[k]);

    if(keys.length === 0) { keys = ["No Failure Data"]; vals = [0]; }

    const canvas = document.getElementById('jobsFailuresChart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const gradFail = ctx.createLinearGradient(0,0,0,200); 
    gradFail.addColorStop(0, 'rgba(239, 68, 68, 0.4)'); 
    gradFail.addColorStop(1, 'rgba(239, 68, 68, 0.0)');

    window.currentCharts.jobsFail = new Chart(ctx, {
        type: 'line', 
        data: {
            labels: keys,
            datasets: [{ label:'Job Failures', data: vals, borderColor:'#ef4444', backgroundColor:gradFail, fill:true, tension:0.3, pointRadius:4 }]
        }, 
        options: { 
            responsive:true, 
            maintainAspectRatio:false, 
            plugins:{legend:{display: false}}, 
            scales:{
                y:{min:0, ticks:{stepSize: 1}, grid:{color:'rgba(255,255,255,0.05)'}}, 
                x:{grid:{display:false}}
            } 
        }
    });
}

window.showJobFailureDetail = function(idx) {
    const message = window.appState.jobFailureMessages[idx];
    const existingModal = document.getElementById('job-failure-modal');
    if (existingModal) existingModal.remove();

    const modal = document.createElement('div');
    modal.id = 'job-failure-modal';
    modal.style.cssText = 'display:flex; position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.8); align-items:center; justify-content:center;';
    
    modal.innerHTML = `
        <div style="background:var(--bg-surface); margin:2%; padding:20px; border:1px solid var(--border-color,#333); border-radius:12px; width:95%; max-width:800px; max-height:80vh; overflow-y:auto; color:var(--text-primary,#e0e0e0); box-shadow:0 4px 20px rgba(0,0,0,0.5);">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:1rem; border-bottom:1px solid var(--border-color,#333); padding-bottom:0.75rem;">
                <h3 style="margin:0; color:var(--danger,#ef4444); font-size:1.1rem;"><i class="fa-solid fa-circle-exclamation"></i> Job Failure Details</h3>
                <button style="background:transparent; border:none; color:var(--text-primary,#e0e0e0); font-size:1.25rem; cursor:pointer;" data-action="remove-closest" data-selector="#job-failure-modal">&times;</button>
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
