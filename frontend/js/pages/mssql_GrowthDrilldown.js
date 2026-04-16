/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Database growth analysis page with size trend tracking.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.GrowthDrilldown = function() {
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <button class="btn btn-sm btn-outline mb-2" data-action="navigate" data-route="dashboard"><i class="fa-solid fa-arrow-left"></i> Back to Dashboard</button>
                    <h1>Table Growth Drilldown</h1>
                    <p class="subtitle">Analyze Data vs Index size inflation across specific objects</p>
                </div>
                <div class="filter-group glass-panel p-3 rounded" style="display:flex; gap:1rem;">
                    <select><option>Top 5 Growing Tables</option><option>Custom Select...</option></select>
                    <select><option>All FileGroups</option><option>PRIMARY</option><option>USER_DATA</option></select>
                    <button class="btn btn-accent btn-sm">Refresh Trend</button>
                </div>
            </div>
            
            <div class="chart-card glass-panel mt-4" style="height: 400px;">
                <div class="card-header">
                    <h3>30-Day Storage Growth Path (GB)</h3>
                </div>
                <div class="chart-container" style="height: 300px;"><canvas id="drillGrowthChart"></canvas></div>
            </div>

            <div class="table-card glass-panel mt-4">
                <div class="card-header"><h3>Top Space Consumers (Detailed)</h3></div>
                <div class="table-responsive">
                    <table class="data-table">
                        <thead>
                            <tr><th>Schema.Table</th><th>Total Size (GB)</th><th>Data Size</th><th>Index Size</th><th>Rows</th><th>30d Growth</th></tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><strong>dbo.AuditLogs</strong></td>
                                <td><span class="text-danger">75.2</span></td>
                                <td>40.1 GB</td>
                                <td>35.1 GB <span class="badge badge-warning">High</span></td>
                                <td>450,230,120</td>
                                <td><i class="fa-solid fa-arrow-trend-up text-danger"></i> +12 GB</td>
                            </tr>
                            <tr>
                                <td><strong>sales.Orders</strong></td>
                                <td>24.5</td>
                                <td>18.0 GB</td>
                                <td>6.5 GB</td>
                                <td>15,000,000</td>
                                <td><i class="fa-solid fa-arrow-trend-up text-warning"></i> +2.1 GB</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    `;

    setTimeout(() => {
        window.currentCharts = window.currentCharts || {};
        window.currentCharts.drillGrw = new Chart(document.getElementById('drillGrowthChart').getContext('2d'), {
            type: 'line', data: {
                labels: Array.from({length:30}, (_,i)=>`Day ${i+1}`),
                datasets: [
                    { label:'dbo.AuditLogs', data:Array.from({length:30}, (_,i)=> 60 + (i*0.5)), borderColor:window.getCSSVar('--danger'), tension:0.1, fill:true, backgroundColor:'rgba(239, 68, 68, 0.1)' },
                    { label:'sales.Orders', data:Array.from({length:30}, (_,i)=> 20 + (i*0.15)), borderColor:window.getCSSVar('--accent-blue'), tension:0.1 }
                ]
            }, options: {responsive:true, maintainAspectRatio:false}
        });
    }, 50);
}
