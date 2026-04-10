window.DeadlockDrilldown = function() {
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title">
                <button class="btn btn-sm btn-outline mb-2" onclick="window.appNavigate('drilldown-locks')"><i class="fa-solid fa-arrow-left"></i> Back to Locks</button>
                <h1><i class="fa-solid fa-skull-crossbones text-danger"></i> Deadlock Analysis</h1>
                <p class="subtitle">Historical Deadlock graphs tracked via Extended Events</p>
            </div>
            
            <div class="table-card glass-panel mt-4 border-danger">
                <div class="card-header">
                    <h3>Recent Deadlock Occurrences (Last 24h)</h3>
                </div>
                <div class="table-responsive">
                    <table class="data-table">
                        <thead>
                            <tr><th>Time</th><th>Victim SPID</th><th>Winning SPID</th><th>Resource</th><th>Action</th></tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td>2 Hours Ago (14:32)</td>
                                <td><strong>SPID 114</strong> (SalesPortal)</td>
                                <td><strong>SPID 92</strong> (SQL Agent)</td>
                                <td>Object: dbo.Orders</td>
                                <td><button class="btn btn-sm btn-outline" onclick="alert('Viewing Deadlock XML Graph...')"><i class="fa-solid fa-sitemap"></i> View Graph</button></td>
                            </tr>
                            <tr>
                                <td>Yesterday (21:15)</td>
                                <td><strong>SPID 65</strong> (Reporting)</td>
                                <td><strong>SPID 81</strong> (Web_App)</td>
                                <td>Page: 1:45210</td>
                                <td><button class="btn btn-sm btn-outline" onclick="alert('Viewing Deadlock XML Graph...')"><i class="fa-solid fa-sitemap"></i> View Graph</button></td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div class="chart-card glass-panel mt-4">
                <div class="card-header text-center w-100" style="justify-content:center; display:flex;">
                    <h3 class="text-muted"><i class="fa-solid fa-diagram-project"></i> Select a deadlock above to view the graphical XML representation</h3>
                </div>
                <div style="height: 300px; display:flex; align-items:center; justify-content:center; border: 2px dashed var(--border-color); border-radius: 8px;">
                    <p class="text-muted">Interactive graph canvas mapping Process Nodes to Resource Nodes</p>
                </div>
            </div>
        </div>
    `;
}
