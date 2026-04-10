window.escapeHtml = function(unsafe) { return (!unsafe) ? '' : unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;"); };

window.mssql_TopQueriesDrilldown = async function() {
    window.scrollTo(0, 0);
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    
    // Fetch extended events data
    let xeventMetrics = null;
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/mssql/dashboard?instance=${encodeURIComponent(inst.name)}`);
        if (response.ok) {
            const metrics = await response.json();
            xeventMetrics = metrics.xevent_metrics;
        }
    } catch (err) {
        console.warn('Failed to fetch extended events data:', err);
    }

    let parsedData = {};
    let queryData = window.appState.topQueryDrill || {};
    try {
        const eventData = JSON.parse(decodeURIComponent(queryData.eventData || '{}'));
        parsedData = JSON.parse(eventData.parsed_payload_json || '{}');
    } catch (err) {
        console.warn('Failed to parse query data:', err);
    }

    const sqlText = parsedData.sql_text || parsedData.statement || 'No SQL Text Available';

    // Format SQL for better readability
    const formattedSQL = window.escapeHtml(sqlText).replace(/SELECT |FROM |WHERE |AND |OR |JOIN |ON |GROUP BY |ORDER BY |INSERT |UPDATE |DELETE |EXEC |SET /g, function(match){
        return '<br/><strong style="color:var(--accent-blue)">'+match+'</strong> ';
    });

    // Aggregate similar SQL text into distinct groups for report
    let groupedQueryData = [];
    if (xeventMetrics && Array.isArray(xeventMetrics.recent_events)) {
        const aggregate = {};

        xeventMetrics.recent_events.forEach(event => {
            let evParsed = {};
            try {
                evParsed = JSON.parse(event.parsed_payload_json || '{}');
            } catch (err) {
                console.warn('Failed to parse event data for aggregation:', err);
            }

            const rawSql = (evParsed.sql_text || evParsed.statement || 'N/A').trim();
            const normalizedSql = rawSql.replace(/\s+/g, ' ').trim() || 'N/A';

            if (!aggregate[normalizedSql]) {
                aggregate[normalizedSql] = {
                    sqlText: rawSql || 'N/A',
                    executionCount: 0,
                    totalCpuTime: 0,
                    totalDuration: 0,
                    totalLogicalReads: 0,
                    latestTimestamp: event.event_timestamp || '',
                    clientAppName: evParsed.client_app_name || 'N/A',
                    clientHostname: evParsed.client_hostname || 'N/A',
                    databaseName: evParsed.database_name || 'N/A',
                    username: evParsed.username || 'N/A',
                    sampleEvent: event
                };
            }

            const bucket = aggregate[normalizedSql];
            bucket.executionCount += 1;
            bucket.totalCpuTime += Number(evParsed.cpu_time) || 0;
            bucket.totalDuration += Number(evParsed.duration) || 0;
            bucket.totalLogicalReads += Number(evParsed.logical_reads) || 0;

            if (event.event_timestamp && event.event_timestamp > bucket.latestTimestamp) {
                bucket.latestTimestamp = event.event_timestamp;
                bucket.sampleEvent = event;
            }

            bucket.clientAppName = evParsed.client_app_name || bucket.clientAppName;
            bucket.clientHostname = evParsed.client_hostname || bucket.clientHostname;
            bucket.databaseName = evParsed.database_name || bucket.databaseName;
            bucket.username = evParsed.username || bucket.username;
        });

        groupedQueryData = Object.values(aggregate).map(bucket => ({
            ...bucket,
            avgCpuTime: bucket.executionCount ? Math.round(bucket.totalCpuTime / bucket.executionCount) : 0,
            avgDuration: bucket.executionCount ? Math.round(bucket.totalDuration / bucket.executionCount) : 0,
            avgLogicalReads: bucket.executionCount ? Math.round(bucket.totalLogicalReads / bucket.executionCount) : 0
        })).sort((a, b) => b.executionCount - a.executionCount).slice(0, 10);
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <button class="btn btn-sm btn-outline mb-2" onclick="window.appNavigate('dashboard')"><i class="fa-solid fa-arrow-left"></i> Back to Live Dashboard</button>
                    <h1><i class="fa-solid fa-code text-accent"></i> Top Query Details</h1>
                    <p class="subtitle">Extended Events Query Analysis [${window.escapeHtml(inst.name)}]</p>
                </div>
                <div class="custom-select-group">
                    <button class="btn btn-accent btn-sm" onclick="navigator.clipboard.writeText('${sqlText.replace(/'/g, "\\'")}')"><i class="fa-solid fa-copy"></i> Copy SQL</button>
                </div>
            </div>

            <div class="metrics-grid mt-4">
                <div class="metric-card glass-panel status-healthy">
                    <div class="metric-header"><span class="metric-title">Database</span><i class="fa-solid fa-database card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${window.escapeHtml(parsedData.database_name || 'N/A')}</div>
                </div>
                <div class="metric-card glass-panel status-info">
                    <div class="metric-header"><span class="metric-title">Username</span><i class="fa-solid fa-user card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${window.escapeHtml(parsedData.username || 'N/A')}</div>
                </div>
                <div class="metric-card glass-panel status-warning">
                    <div class="metric-header"><span class="metric-title">CPU Time</span><i class="fa-solid fa-microchip card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${parsedData.cpu_time ? parsedData.cpu_time + ' μs' : 'N/A'}</div>
                </div>
                <div class="metric-card glass-panel status-success">
                    <div class="metric-header"><span class="metric-title">Duration</span><i class="fa-solid fa-clock card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${parsedData.duration ? parsedData.duration + ' μs' : 'N/A'}</div>
                </div>
                <div class="metric-card glass-panel status-accent">
                    <div class="metric-header"><span class="metric-title">Logical Reads</span><i class="fa-solid fa-hdd card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${parsedData.logical_reads || 'N/A'}</div>
                </div>
                <div class="metric-card glass-panel status-primary">
                    <div class="metric-header"><span class="metric-title">Event Type</span><i class="fa-solid fa-bolt card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${window.escapeHtml(queryData.eventType || 'N/A')}</div>
                </div>
            </div>

            <div class="tables-grid mt-4">
                <div class="table-card glass-panel" style="grid-column: span 2;">
                    <div class="card-header">
                        <h3><i class="fa-solid fa-terminal text-accent"></i> Client Connection Details</h3>
                    </div>
                    <div class="table-responsive p-3" style="background: rgba(0,0,0,0.2); border-radius: 8px;">
                        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 15px;">
                            <div>
                                <strong>Client Application:</strong><br/>
                                <span class="text-accent">${window.escapeHtml(parsedData.client_app_name || 'N/A')}</span>
                            </div>
                            <div>
                                <strong>Client Hostname:</strong><br/>
                                <span class="text-warning">${window.escapeHtml(parsedData.client_hostname || 'N/A')}</span>
                            </div>
                            <div>
                                <strong>Event Timestamp:</strong><br/>
                                <span class="text-info">${queryData.timestamp || 'N/A'}</span>
                            </div>
                            <div>
                                <strong>SQL Length:</strong><br/>
                                <span>${sqlText.length} characters</span>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="table-card glass-panel" style="grid-column: span 2; margin-top: 1.5rem;">
                    <div class="card-header">
                        <h3><i class="fa-solid fa-code text-accent"></i> Complete SQL Statement</h3>
                    </div>
                    <div class="p-3" style="background: rgba(0,0,0,0.3); border-radius: 8px; font-family: 'Courier New', monospace; font-size: 0.85rem; white-space: pre-wrap; word-wrap: break-word; max-height: 400px; overflow-y: auto;">
                        ${formattedSQL}
                    </div>
                </div>
            </div>

            <!-- Extended Events Metrics Section -->
            ${xeventMetrics ? `
            <div class="tables-grid mt-4">
                <div class="table-card glass-panel" style="grid-column: span 2;">
                    <div class="card-header flex-between">
                        <h3><i class="fa-solid fa-sparkles text-accent"></i> Extended Events Telemetry (Last 1 Hour)</h3>
                        <span class="badge badge-accent">${xeventMetrics.total_events_last_hour} Events</span>
                    </div>
                    <div class="chart-container" style="height: 220px;"><canvas id="xeventChart"></canvas></div>
                </div>

                <div class="table-card glass-panel" style="grid-column: span 2; margin-top: 1.5rem;">
                    <div class="card-header flex-between">
                        <h3><i class="fa-solid fa-clock text-accent"></i> Top 10 Long Running Queries History</h3>
                        <div class="flex-between" style="gap: 10px;">
                            <button class="btn btn-sm btn-outline text-accent" onclick="window.refreshTopQueriesData()">
                                <i class="fa-solid fa-refresh"></i> Refresh Data
                            </button>
                            <span class="badge badge-info">${(xeventMetrics.recent_events || []).length} Queries</span>
                        </div>
                    </div>
                    <div class="table-responsive" style="max-height: 600px; overflow-y: auto;">
                        <table class="data-table modern-table" style="font-size: 0.8rem;">
                            <thead>
                                <tr>
                                    <th style="min-width: 120px;">Timestamp</th>
                                    <th style="min-width: 200px;">SQL Text</th>
                                    <th style="min-width: 150px;">Client App Name</th>
                                    <th style="min-width: 150px;">Client Hostname</th>
                                    <th style="min-width: 90px;">Executions</th>
                                    <th style="min-width: 120px;">Avg CPU Time<br/><small class="text-muted">(μs)</small></th>
                                    <th style="min-width: 120px;">Database</th>
                                    <th style="min-width: 120px;">Duration<br/><small class="text-muted">(μs)</small></th>
                                    <th style="min-width: 120px;">Logical Reads</th>
                                    <th style="min-width: 120px;">Username</th>
                                    <th style="min-width: 100px;">Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${groupedQueryData.length > 0 ? groupedQueryData.map(group => {
                                    const truncatedSql = group.sqlText.length > 60 ? group.sqlText.substring(0, 60) + '...' : group.sqlText;
                                    const sample = group.sampleEvent || {};

                                    return `
                                        <tr class="query-row">
                                            <td><span class="badge badge-outline">${group.latestTimestamp ? group.latestTimestamp.substring(11, 19) : 'N/A'}</span></td>
                                            <td title="${window.escapeHtml(group.sqlText)}">
                                                <div class="sql-preview">${window.escapeHtml(truncatedSql)}</div>
                                            </td>
                                            <td><span class="text-accent">${window.escapeHtml(group.clientAppName)}</span></td>
                                            <td><span class="text-warning">${window.escapeHtml(group.clientHostname)}</span></td>
                                            <td>${group.executionCount}</td>
                                            <td class="metric-cell" onclick="event.stopPropagation(); window.showMetricDetail('cpu_time', '${group.avgCpuTime}', '${encodeURIComponent(JSON.stringify(sample))}')">
                                                <span class="metric-value" style="font-size: 0.85rem;">${group.avgCpuTime}</span>
                                            </td>
                                            <td><span class="badge badge-info">${window.escapeHtml(group.databaseName)}</span></td>
                                            <td class="metric-cell" onclick="event.stopPropagation(); window.showMetricDetail('duration', '${group.avgDuration}', '${encodeURIComponent(JSON.stringify(sample))}')">
                                                <span class="metric-value" style="font-size: 0.85rem;">${group.avgDuration}</span>
                                            </td>
                                            <td class="metric-cell" onclick="event.stopPropagation(); window.showMetricDetail('logical_reads', '${group.avgLogicalReads}', '${encodeURIComponent(JSON.stringify(sample))}')">
                                                <span class="metric-value" style="font-size: 0.85rem;">${group.avgLogicalReads}</span>
                                            </td>
                                            <td><span class="text-success">${window.escapeHtml(group.username)}</span></td>
                                            <td>
                                                <button class="btn btn-xs btn-accent" onclick="event.stopPropagation(); window.showQueryDetails('${encodeURIComponent(JSON.stringify(group.sampleEvent))}')" title="View Details">
                                                    <i class="fa-solid fa-eye"></i>
                                                </button>
                                            </td>
                                        </tr>
                                    `;
                                }).join('') : `<tr><td colspan="11" class="text-muted">No extended events found to aggregate.</td></tr>`}
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
            ` : '<div class="alert alert-warning mt-4"><i class="fa-solid fa-exclamation-triangle"></i> Extended Events data not available. Please check your SQL Server configuration.</div>'}

        </div>
    `;

    // Initialize the extended events chart if data is available
    if (xeventMetrics && xeventMetrics.event_counts) {
        setTimeout(() => {
            const eventCounts = xeventMetrics.event_counts || {};
            const eventTypes = Object.keys(eventCounts).slice(0, 10);
            const counts = eventTypes.map(et => eventCounts[et]);
            
            const colors = [
                window.getCSSVar('--accent-blue'),
                window.getCSSVar('--success'),
                window.getCSSVar('--warning'),
                window.getCSSVar('--danger'),
                window.getCSSVar('--info'),
                '#a855f7',
                '#ec4899',
                '#06b6d4',
                '#f59e0b',
                '#10b981'
            ];
            
            const ctx = document.getElementById('xeventChart');
            if (ctx) {
                const chart = new Chart(ctx.getContext('2d'), {
                    type: 'bar',
                    data: {
                        labels: eventTypes,
                        datasets: [{
                            label: 'Event Count (1h)',
                            data: counts,
                            backgroundColor: colors.slice(0, eventTypes.length),
                            borderRadius: 4,
                            borderSkipped: false
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        indexAxis: 'y',
                        plugins: {
                            legend: { display: false }
                        },
                        scales: {
                            x: { beginAtZero: true, title: { display: true, text: 'Event Count' } },
                            y: { grid: { display: false } }
                        }
                    }
                });
            }
        }, 100);
    }
}

// Refresh function for the top queries drilldown page
window.refreshTopQueriesData = async function() {
    const refreshBtn = document.querySelector('button[onclick="window.refreshTopQueriesData()"]');
    if (refreshBtn) {
        refreshBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Refreshing...';
        refreshBtn.disabled = true;
    }

    try {
        // Re-run the drilldown function to refresh data
        await window.mssql_TopQueriesDrilldown();
        
        if (refreshBtn) {
            refreshBtn.innerHTML = '<i class="fa-solid fa-check"></i> Refreshed';
            setTimeout(() => {
                refreshBtn.innerHTML = '<i class="fa-solid fa-refresh"></i> Refresh Data';
                refreshBtn.disabled = false;
            }, 2000);
        }
    } catch (error) {
        console.error("Top queries refresh failed:", error);
        if (refreshBtn) {
            refreshBtn.innerHTML = '<i class="fa-solid fa-exclamation-triangle"></i> Failed';
            setTimeout(() => {
                refreshBtn.innerHTML = '<i class="fa-solid fa-refresh"></i> Refresh Data';
                refreshBtn.disabled = false;
            }, 3000);
        }
    }
};;

// Helper function to show query details from dashboard
window.showQueryDetails = function(encodedEventData) {
    try {
        const eventData = JSON.parse(decodeURIComponent(encodedEventData));
        let parsedData = {};
        try {
            parsedData = JSON.parse(eventData.parsed_payload_json || '{}');
        } catch (err) {
            console.warn('Failed to parse event data:', err);
        }

        window.appState.queryDrill = {
            text: parsedData.sql_text || parsedData.statement || 'No SQL Text Available',
            login: parsedData.username || parsedData.login || 'N/A',
            program: parsedData.client_app_name || parsedData.program_name || parsedData.client_hostname || 'N/A',
            wait: parsedData.wait_type || 'N/A',
            db: parsedData.database_name || 'N/A'
        };

        window.appNavigate('drilldown-query');
        window.scrollTo(0, 0);
    } catch (err) {
        console.error('Failed to process query details:', err);
    }
};