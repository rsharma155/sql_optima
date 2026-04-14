/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Index usage drilldown for missing index recommendations.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.IndexDrilldown = function() {
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <button class="btn btn-sm btn-outline mb-2" onclick="window.appNavigate('dashboard')"><i class="fa-solid fa-arrow-left"></i> Back to Dashboard</button>
                    <h1>Index Usage Drilldown</h1>
                    <p class="subtitle">Analyze Index Seeks, Scans, and identify Missing Index impact</p>
                </div>
                <div class="filter-group glass-panel p-3 rounded" style="display:flex; gap:1rem;">
                    <select><option>Worst Performing Indexes</option><option>Unused Indexes</option></select>
                    <button class="btn btn-accent btn-sm">Analyze</button>
                </div>
            </div>
            
            <div class="chart-card glass-panel mt-4" style="height: 400px;">
                <div class="card-header">
                    <h3>Historic Seeks vs Scans (7 Days)</h3>
                </div>
                <div class="chart-container" style="height: 300px;"><canvas id="drillIndexChart"></canvas></div>
            </div>

            <div class="tables-grid mt-4">
                <div class="table-card glass-panel">
                    <div class="card-header"><h3>High Impact Missing Indexes</h3></div>
                    <div class="table-responsive">
                        <table class="data-table">
                            <thead>
                                <tr><th>Target Table</th><th>Suggested Columns</th><th>Est. Impact</th><th>Avg Cost</th></tr>
                            </thead>
                            <tbody>
                                <tr>
                                    <td><strong>dbo.AuditLogs</strong></td>
                                    <td>[ActionDate], [UserID]</td>
                                    <td><span class="badge badge-success">98% Info</span></td>
                                    <td>84.5</td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>

                <div class="table-card glass-panel border-warning">
                    <div class="card-header"><h3>Unused/Duplicate Indexes</h3></div>
                    <div class="table-responsive">
                        <table class="data-table">
                            <thead>
                                <tr><th>Index Name</th><th>Updates (Cost)</th><th>Seeks</th></tr>
                            </thead>
                            <tbody>
                                <tr>
                                    <td><strong>idx_Old_Legacy</strong></td>
                                    <td><span class="text-danger">1,245,000</span></td>
                                    <td>0</td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    `;

    setTimeout(() => {
        window.currentCharts = window.currentCharts || {};
        window.currentCharts.drillIdx = new Chart(document.getElementById('drillIndexChart').getContext('2d'), {
            type: 'bar', data: {
                labels: Array.from({length:7}, (_,i)=>`Day ${i+1}`),
                datasets: [
                    { label:'Total Seeks', data:[2400, 2200, 3100, 2900, 4200, 1500, 800], backgroundColor:window.getCSSVar('--success') },
                    { label:'Total Scans', data:[800, 950, 400, 1200, 200, 50, 45], backgroundColor:window.getCSSVar('--danger') }
                ]
            }, options: {responsive:true, maintainAspectRatio:false, scales:{x:{stacked:true}, y:{stacked:true}}}
        });
    }, 50);
}
