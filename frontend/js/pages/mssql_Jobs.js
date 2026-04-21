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
    
    // Helper for local ISO string (YYYY-MM-DDTHH:mm)
    const toLocalISO = (date) => {
        const offset = date.getTimezoneOffset() * 60000;
        const local = new Date(date.getTime() - offset);
        return local.toISOString().slice(0, 16);
    };

    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - (1 * 60 * 60 * 1000));
    
    // Always refresh 'To' to now and 'From' to 1h ago when entering, 
    // unless we want to keep the user's last selection.
    // For now, let's always default to the latest hour to avoid "stuck" times.
    const defaultFrom = toLocalISO(oneHourAgo);
    const defaultTo = toLocalISO(now);

    if (!window.appState.jobsFrom) window.appState.jobsFrom = defaultFrom;
    if (!window.appState.jobsTo) window.appState.jobsTo = defaultTo;

    // If the 'To' time is more than 5 minutes old, refresh it to now
    const lastTo = new Date(window.appState.jobsTo);
    if (now.getTime() - lastTo.getTime() > 5 * 60 * 1000) {
        window.appState.jobsFrom = defaultFrom;
        window.appState.jobsTo = defaultTo;
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line">
                    <h1><i class="fa-solid fa-briefcase text-accent"></i> SQL Agent Jobs</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Time-series status & history</p>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="gap: 0.75rem; align-items: center;">
                    <div class="date-picker-group glass-panel" style="display:flex; align-items:center; gap:0.5rem; padding:0.25rem 0.75rem; border-radius:8px;">
                        <span class="text-muted" style="font-size:0.75rem;">Window:</span>
                        <input type="datetime-local" id="jobsFrom" class="custom-date-input" value="${window.appState.jobsFrom}">
                        <span class="text-muted">to</span>
                        <input type="datetime-local" id="jobsTo" class="custom-date-input" value="${window.appState.jobsTo}">
                        <button id="jobsApplyRange" class="btn btn-xs btn-accent">Update Status</button>
                    </div>
                    <button class="btn btn-sm btn-outline text-accent" data-action="navigate" data-route="dashboard"><i class="fa-solid fa-arrow-left"></i></button>
                </div>
            </div>

            <div id="jobsDashboardContent">
                <div style="display:flex; justify-content:center; align-items:center; height:300px;">
                    <div class="spinner"></div><span style="margin-left: 1rem;">Analyzing job states for selected period...</span>
                </div>
            </div>
        </div>
    `;

    document.getElementById('jobsApplyRange')?.addEventListener('click', () => {
        window.appState.jobsFrom = document.getElementById('jobsFrom').value;
        window.appState.jobsTo = document.getElementById('jobsTo').value;
        refreshJobsData(inst);
    });

    refreshJobsData(inst);
}

async function refreshJobsData(inst) {
    const from = window.appState.jobsFrom || '';
    const to = window.appState.jobsTo || '';
    const content = document.getElementById('jobsDashboardContent');
    if (!content) return;
    
    try {
        const url = `/api/mssql/jobs?instance=${encodeURIComponent(inst.name)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;
        const response = await window.apiClient.authenticatedFetch(url);
        if (!response.ok) throw new Error("Jobs API failed");
        const data = await response.json();
        
        renderJobsContent(inst, data);
    } catch(err) {
        content.innerHTML = `<div class="alert alert-danger">Error: ${window.escapeHtml(err.message)}</div>`;
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

    content.innerHTML = `
        <div class="metrics-grid mt-3" style="display: grid; grid-template-columns: repeat(5, 1fr); gap: 0.75rem;">
            <div class="metric-card glass-panel" style="padding: 1rem; text-align:center;">
                <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted); text-transform:uppercase;">Total Jobs</div>
                <div class="metric-value" style="font-size:1.75rem; font-weight:700;">${sums.total_jobs}</div>
            </div>
            <div class="metric-card glass-panel" style="padding: 1rem; text-align:center; border-bottom: 3px solid var(--success);">
                <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted); text-transform:uppercase;">Enabled</div>
                <div class="metric-value" style="font-size:1.75rem; font-weight:700; color:var(--success);">${sums.enabled_jobs}</div>
            </div>
            <div class="metric-card glass-panel" style="padding: 1rem; text-align:center;">
                <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted); text-transform:uppercase;">Disabled</div>
                <div class="metric-value" style="font-size:1.75rem; font-weight:700; color:var(--text-muted);">${sums.disabled_jobs}</div>
            </div>
            <div class="metric-card glass-panel" style="padding: 1rem; text-align:center; border-bottom: 3px solid var(--warning);">
                <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted); text-transform:uppercase;">Running</div>
                <div class="metric-value" style="font-size:1.75rem; font-weight:700; color:var(--warning);">${sums.running_jobs}</div>
            </div>
            <div class="metric-card glass-panel" style="padding: 1rem; text-align:center; border-bottom: 3px solid var(--danger);">
                <div class="metric-header" style="font-size:0.7rem; color:var(--text-muted); text-transform:uppercase;">Failed (Period)</div>
                <div class="metric-value" style="font-size:1.75rem; font-weight:700; color:var(--danger);">${sums.failed_jobs}</div>
            </div>
        </div>

        <div class="chart-card glass-panel mt-3" style="padding: 1rem;">
            <div class="card-header"><h3 style="font-size:0.9rem; margin:0;"><i class="fa-solid fa-chart-area text-accent"></i> Job Failure Trend</h3></div>
            <div class="chart-container" style="height: 180px;"><canvas id="jobsFailuresChart"></canvas></div>
        </div>

        <div class="tabs-container mt-3">
            <button class="tab-btn active" data-tab="list">Job Inventory</button>
            <button class="tab-btn" data-tab="schedules">Next Runs</button>
            <button class="tab-btn" data-tab="failures">Failure History</button>
        </div>

        <div id="jobTab-list" class="tab-panel mt-2">
            <div class="table-card glass-panel">
                <div class="table-responsive" style="max-height:400px;">
                    <table class="data-table">
                        <thead><tr><th>Job Name</th><th>Category</th><th>Enabled</th><th>Status</th><th>Last Run</th><th>Last Result</th><th>Owner</th></tr></thead>
                        <tbody>
                            ${jList.map(j => `
                                <tr>
                                    <td><strong title="${window.escapeHtml(j.description)}">${window.escapeHtml(j.job_name)}</strong></td>
                                    <td><span class="small text-muted">${window.escapeHtml(j.category || 'Uncategorized')}</span></td>
                                    <td><span class="badge ${j.enabled ? 'badge-success' : 'badge-outline'}">${j.enabled ? 'Yes' : 'No'}</span></td>
                                    <td><span class="badge ${j.current_status === 'Running' ? 'badge-warning' : 'badge-outline'}">${window.escapeHtml(j.current_status)}</span></td>
                                    <td>${parseRunTime(j.last_run_date, j.last_run_time)}</td>
                                    <td><span class="badge ${formatStringColor(j.last_run_status)}">${window.escapeHtml(j.last_run_status)}</span></td>
                                    <td class="text-muted small">${window.escapeHtml(j.owner)}</td>
                                </tr>
                            `).join('') || '<tr><td colspan="7" class="text-center text-muted">No jobs found</td></tr>'}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>

        <div id="jobTab-schedules" class="tab-panel mt-2" style="display:none;">
            <div class="table-card glass-panel">
                <table class="data-table">
                    <thead><tr><th>Job</th><th>Schedule</th><th>Active</th><th>Next Expected Run</th></tr></thead>
                    <tbody>
                        ${sched.map(s => `
                            <tr>
                                <td><strong>${window.escapeHtml(s.job_name)}</strong></td>
                                <td>${window.escapeHtml(s.schedule_name)}</td>
                                <td><span class="badge ${s.status === 'Active' ? 'badge-success' : 'badge-outline'}">${s.status}</span></td>
                                <td><i class="fa-regular fa-clock text-accent"></i> ${s.next_run_datetime ? s.next_run_datetime.substring(0, 19).replace('T', ' ') : 'N/A'}</td>
                            </tr>
                        `).join('') || '<tr><td colspan="4" class="text-center text-muted">No upcoming schedules</td></tr>'}
                    </tbody>
                </table>
            </div>
        </div>

        <div id="jobTab-failures" class="tab-panel mt-2" style="display:none;">
            <div class="table-card glass-panel">
                <table class="data-table">
                    <thead><tr><th>Job</th><th>Step</th><th>Failed At</th><th>Message</th></tr></thead>
                    <tbody>
                        ${fails.map((f, idx) => `
                            <tr>
                                <td><strong>${window.escapeHtml(f.job_name)}</strong></td>
                                <td><span class="badge badge-outline">${window.escapeHtml(f.step_name)}</span></td>
                                <td style="white-space:nowrap;">${parseRunTime(f.run_date, f.run_time)}</td>
                                <td class="small" style="max-width:500px; cursor:pointer;" onclick="window.showJobFailureDetail(${idx})">
                                    ${window.escapeHtml(f.message ? (f.message.slice(0, 120) + '...') : 'No message')}
                                </td>
                            </tr>
                        `).join('') || '<tr><td colspan="4" class="text-center text-success">No failures in selected range</td></tr>'}
                    </tbody>
                </table>
            </div>
        </div>
    `;

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
                <button style="background:transparent; border:none; color:var(--text-primary,#e0e0e0); font-size:1.25rem; cursor:pointer;" onclick="this.closest('#job-failure-modal').remove()">&times;</button>
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
