window.escapeHtml = function(unsafe) { return (!unsafe) ? '' : unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;"); };

window.mssql_MetricDetailDrilldown = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    const metricData = window.appState.metricDetail || {};

    let parsedData = metricData.parsedData || {};
    const sqlText = parsedData.sql_text || parsedData.statement || 'No SQL Text Available';

    // Format SQL for better readability
    const formattedSQL = window.escapeHtml(sqlText).replace(/SELECT |FROM |WHERE |AND |OR |JOIN |ON |GROUP BY |ORDER BY |INSERT |UPDATE |DELETE |EXEC |SET /g, function(match){
        return '<br/><strong style="color:var(--accent-blue)">'+match+'</strong> ';
    });

    // Get performance insights based on metric type
    const getPerformanceInsight = (metricType, value) => {
        const numValue = parseFloat(value) || 0;
        switch(metricType) {
            case 'cpu_time':
                if (numValue > 1000000) return { level: 'danger', message: 'Very High CPU Usage - Consider query optimization' };
                if (numValue > 500000) return { level: 'warning', message: 'High CPU Usage - Monitor performance' };
                return { level: 'success', message: 'Acceptable CPU Usage' };
            case 'duration':
                if (numValue > 5000000) return { level: 'danger', message: 'Very Slow Query - Immediate optimization needed' };
                if (numValue > 1000000) return { level: 'warning', message: 'Slow Query - Consider optimization' };
                return { level: 'success', message: 'Good Query Performance' };
            case 'logical_reads':
                if (numValue > 100000) return { level: 'danger', message: 'High I/O Operations - Check indexing' };
                if (numValue > 50000) return { level: 'warning', message: 'Moderate I/O - Review query efficiency' };
                return { level: 'success', message: 'Efficient I/O Usage' };
            default:
                return { level: 'info', message: 'Performance metric analyzed' };
        }
    };

    const insight = getPerformanceInsight(metricData.metricType, metricData.value);

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <button class="btn btn-sm btn-outline mb-2" onclick="window.appNavigate('dashboard')"><i class="fa-solid fa-arrow-left"></i> Back to Dashboard</button>
                    <h1><i class="fa-solid fa-chart-line text-accent"></i> ${metricData.label} Analysis</h1>
                    <p class="subtitle">Performance Metric Details [${window.escapeHtml(inst.name)}]</p>
                </div>
                <div class="custom-select-group">
                    <button class="btn btn-accent btn-sm" onclick="navigator.clipboard.writeText('${sqlText.replace(/'/g, "\\'")}')"><i class="fa-solid fa-copy"></i> Copy SQL</button>
                </div>
            </div>

            <!-- Performance Insight Banner -->
            <div class="alert alert-${insight.level} mt-4" style="border-radius: 8px; padding: 15px;">
                <div class="flex-between">
                    <div>
                        <h4 style="margin: 0 0 5px 0;"><i class="fa-solid fa-exclamation-triangle"></i> Performance Insight</h4>
                        <p style="margin: 0;">${insight.message}</p>
                    </div>
                    <div style="font-size: 2rem;">
                        ${insight.level === 'danger' ? '🔴' : insight.level === 'warning' ? '🟡' : '🟢'}
                    </div>
                </div>
            </div>

            <div class="metrics-grid mt-4">
                <div class="metric-card glass-panel status-${insight.level === 'danger' ? 'danger' : insight.level === 'warning' ? 'warning' : 'healthy'}">
                    <div class="metric-header"><span class="metric-title">${metricData.label}</span><i class="fa-solid fa-tachometer-alt card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.8rem">${metricData.value}</div>
                    <div class="metric-trend">${metricData.unit}</div>
                </div>
                <div class="metric-card glass-panel status-healthy">
                    <div class="metric-header"><span class="metric-title">Database</span><i class="fa-solid fa-database card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${window.escapeHtml(parsedData.database_name || 'N/A')}</div>
                </div>
                <div class="metric-card glass-panel status-info">
                    <div class="metric-header"><span class="metric-title">Username</span><i class="fa-solid fa-user card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${window.escapeHtml(parsedData.username || 'N/A')}</div>
                </div>
                <div class="metric-card glass-panel status-accent">
                    <div class="metric-header"><span class="metric-title">Event Type</span><i class="fa-solid fa-bolt card-icon"></i></div>
                    <div class="metric-value" style="font-size: 1.2rem">${window.escapeHtml(metricData.eventData?.event_type || 'N/A')}</div>
                </div>
            </div>

            <div class="tables-grid mt-4">
                <div class="table-card glass-panel" style="grid-column: span 2;">
                    <div class="card-header">
                        <h3><i class="fa-solid fa-network-wired text-accent"></i> Client Connection Context</h3>
                    </div>
                    <div class="table-responsive p-3" style="background: rgba(0,0,0,0.2); border-radius: 8px;">
                        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px;">
                            <div>
                                <strong>Application:</strong><br/>
                                <span class="text-accent">${window.escapeHtml(parsedData.client_app_name || 'N/A')}</span>
                            </div>
                            <div>
                                <strong>Hostname:</strong><br/>
                                <span class="text-warning">${window.escapeHtml(parsedData.client_hostname || 'N/A')}</span>
                            </div>
                            <div>
                                <strong>Event Timestamp:</strong><br/>
                                <span class="text-info">${metricData.eventData?.event_timestamp || 'N/A'}</span>
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
                        <h3><i class="fa-solid fa-code text-accent"></i> Associated SQL Statement</h3>
                    </div>
                    <div class="p-3" style="background: rgba(0,0,0,0.3); border-radius: 8px; font-family: 'Courier New', monospace; font-size: 0.85rem; white-space: pre-wrap; word-wrap: break-word; max-height: 400px; overflow-y: auto;">
                        ${formattedSQL}
                    </div>
                </div>

                <!-- Performance Recommendations -->
                <div class="table-card glass-panel" style="grid-column: span 2; margin-top: 1.5rem;">
                    <div class="card-header">
                        <h3><i class="fa-solid fa-lightbulb text-accent"></i> Performance Recommendations</h3>
                    </div>
                    <div class="p-3">
                        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 15px;">
                            ${getRecommendations(metricData.metricType, metricData.value, parsedData)}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;
};

// Generate performance recommendations based on metrics
function getRecommendations(metricType, value, parsedData) {
    const numValue = parseFloat(value) || 0;
    const recommendations = [];

    switch(metricType) {
        case 'cpu_time':
            if (numValue > 1000000) {
                recommendations.push({
                    title: 'High CPU Usage',
                    items: [
                        'Review query execution plan for expensive operations',
                        'Check for missing indexes on joined tables',
                        'Consider query rewriting to reduce computational complexity',
                        'Evaluate if query can be cached or optimized'
                    ]
                });
            }
            break;
        case 'duration':
            if (numValue > 5000000) {
                recommendations.push({
                    title: 'Slow Query Performance',
                    items: [
                        'Analyze execution plan for bottlenecks',
                        'Check for table scans vs index seeks',
                        'Review join operations and their efficiency',
                        'Consider breaking complex query into smaller parts'
                    ]
                });
            }
            break;
        case 'logical_reads':
            if (numValue > 100000) {
                recommendations.push({
                    title: 'High I/O Operations',
                    items: [
                        'Review indexing strategy for queried tables',
                        'Check for unnecessary data access patterns',
                        'Consider query optimization or denormalization',
                        'Evaluate if result set can be reduced'
                    ]
                });
            }
            break;
    }

    if (recommendations.length === 0) {
        return `
            <div class="recommendation-card" style="background: rgba(16, 185, 129, 0.1); border: 1px solid rgba(16, 185, 129, 0.3); border-radius: 8px; padding: 15px;">
                <h4 style="margin: 0 0 10px 0; color: var(--success);">✅ Good Performance</h4>
                <p style="margin: 0; color: var(--success);">This metric shows acceptable performance. Continue monitoring for any changes.</p>
            </div>
        `;
    }

    return recommendations.map(rec => `
        <div class="recommendation-card" style="background: rgba(245, 158, 11, 0.1); border: 1px solid rgba(245, 158, 11, 0.3); border-radius: 8px; padding: 15px;">
            <h4 style="margin: 0 0 10px 0; color: var(--warning);"><i class="fa-solid fa-exclamation-triangle"></i> ${rec.title}</h4>
            <ul style="margin: 0; padding-left: 20px; color: var(--text);">
                ${rec.items.map(item => `<li style="margin-bottom: 5px;">${item}</li>`).join('')}
            </ul>
        </div>
    `).join('');
}