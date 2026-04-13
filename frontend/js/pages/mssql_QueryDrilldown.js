window.escapeHtml = function(unsafe) {
    if (unsafe === null || unsafe === undefined) return '';
    return String(unsafe).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
};

window.mssql_QueryDrilldown = async function() {
    window.scrollTo(0, 0);
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    const payload = window.appState.queryDrill || {text: 'No Query Provided', login: 'Unknown', program: 'Unknown', wait: 'None', db: 'N/A'};
    
    // Formatting the SQL string lightly natively
    const formattedSQL = window.escapeHtml(payload.text).replace(/SELECT |FROM |WHERE |AND |OR |JOIN |ON |GROUP BY |ORDER BY /g, function(match){
        return '<br/><strong style="color:var(--accent-blue)">'+match+'</strong> ';
    }).replace(/INSERT |UPDATE |DELETE |EXEC |SET /g, function(match){
        return '<br/><strong style="color:var(--danger)">'+match+'</strong> ';
    });

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <button class="btn btn-sm btn-outline mb-2" onclick="window.appNavigate(window.appState.queryDrillBackRoute || 'drilldown-top-queries')"><i class="fa-solid fa-arrow-left"></i> Back</button>
                    <h1>Distributed Query Drill-Down Diagnostics</h1>
                    <p class="subtitle">Extracting live thread analytics mapped on [${window.escapeHtml(inst.name)}]</p>
                </div>
                <div class="custom-select-group"></div>
            </div>

            <div class="metrics-grid mt-4">
                <div class="metric-card glass-panel status-healthy">
                    <div class="metric-header"><span class="metric-title">Target Database Route</span><i class="fa-solid fa-database card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.5rem">${window.escapeHtml(payload.db)}</div>
                </div>
                <div class="metric-card glass-panel ${payload.wait === 'ONLINE' ? 'status-success' : 'status-warning'}">
                    <div class="metric-header"><span class="metric-title">Live Execution Thread</span><i class="fa-solid fa-network-wired card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.5rem">${window.escapeHtml(payload.wait)}</div>
                </div>
            </div>

            <div class="tables-grid mt-4">
                <div class="table-card glass-panel" style="grid-column: span 2;">
                    <div class="card-header">
                        <h3>Raw Extracted Client Pointer Trace</h3>
                    </div>
                    <div class="table-responsive p-3" style="background: rgba(0,0,0,0.2); border-radius: 8px;">
                        <ul style="list-style: none; margin: 0; padding: 0; line-height: 2;">
                            <li><strong>Agent Bound:</strong> <span class="text-accent">${window.escapeHtml(payload.program)}</span></li>
                            <li><strong>Runtime User:</strong> <span class="text-warning">${window.escapeHtml(payload.login)}</span></li>
                            <li><strong>Length Size:</strong> <span>${payload.text.length} Characters Sent</span></li>
                        </ul>
                    </div>
                </div>
            </div>

            <div class="metric-card glass-panel mt-4" style="text-align: left; padding: 2rem;">
                <div class="card-header flex-between" style="border-bottom: 1px solid rgba(255,255,255,0.1); padding-bottom: 1rem;">
                    <h3>Execution Query Trace Map</h3>
                    <button class="btn btn-sm btn-outline" onclick="navigator.clipboard.writeText(decodeURIComponent('${encodeURIComponent(payload.text)}'))"><i class="fa-solid fa-copy"></i> Copy Trace</button>
                </div>
                <div style="font-family: 'JetBrains Mono', monospace; font-size: 0.95rem; white-space: pre-wrap; background: rgba(0,0,0,0.3); padding: 1.5rem; border-radius: 8px; margin-top: 1rem; color: #e2e8f0;">
                    ${formattedSQL}
                </div>
            </div>
        </div>
    `;
}
