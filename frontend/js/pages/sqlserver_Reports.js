/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Reporting page for database performance reports.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

// Global function to update alerts badge (alert engine + live blocking KPIs)
window.updateAlertsBadge = async function() {
    try {
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
        if (!inst || (inst.type !== 'sqlserver' && !(inst.engine || '').toLowerCase().includes('sql'))) return;

        let totalAlerts = 0;
        const alertData = await window.fetchAlertsPageData(inst);
        totalAlerts += alertData.openCount || 0;

        const blocked = alertData.blockingKpis?.active_blocked_sessions ?? 0;
        if (blocked > 0) totalAlerts += 1;

        const badgeElement = document.getElementById('alerts-badge');
        if (badgeElement) {
            badgeElement.textContent = totalAlerts;
            badgeElement.style.display = totalAlerts > 0 ? 'inline' : 'none';
        }
    } catch (error) {
        appDebug('Failed to update alerts badge:', error);
    }
};

window.AlertsView = async function() {
    if (window.alertsViewInterval) {
        clearInterval(window.alertsViewInterval);
    }
    
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};

    async function loadAlerts() {
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="dashboard-page-title-compact flex-between">
                    <div>
                        <h1 style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:8px;"><i class="fa-solid fa-triangle-exclamation text-accent"></i> Alerts</h1>
                        <p class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Anomaly Detection</p>
                    </div>
                    <div style="display:flex; align-items:center; gap:0.5rem; flex-wrap:wrap;">
                        ${window.renderStatusStrip({ lastUpdateId: 'alertsLastRefreshTime', sourceBadgeId: 'alertsDataSourceBadge', includeHealth: false, includeFreshness: false, autoRefreshText: 'Auto-refresh: every 30s' })}
                        <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="AlertsView"><i class="fa-solid fa-refresh"></i> Refresh</button>
                    </div>
                </div>
                <div style="display:flex; justify-content:center; align-items:center; height:50vh;">
                    <div class="spinner"></div><span style="margin-left: 1rem;">Analyzing system for anomalies...</span>
                </div>
            </div>
        `;

        try {
            const alertPageData = await window.fetchAlertsPageData(inst);

            // Fetch v2 so we can display source badge consistently
            const response = await window.apiClient.authenticatedFetch(`/api/sqlserver/dashboard/v2?instance=${encodeURIComponent(inst.name)}`);
            if (!response.ok) throw new Error(`Failed to fetch dashboard data (HTTP ${response.status})`);
            window.updateSourceBadge('alertsDataSourceBadge', response.headers.get('X-Data-Source'));
            
            // Validate response is JSON before parsing
            const contentType = response.headers.get('content-type') || '';
            if (!contentType.includes('application/json')) {
                const text = await response.text();
                appDebug('Dashboard API returned non-JSON response:', text.substring(0, 200));
                throw new Error('Dashboard API returned an invalid response');
            }
            
            const v2 = await response.json();
            const metrics = (v2 && v2.compat && v2.compat.dashboard) ? v2.compat.dashboard : v2;
            appDebug('[Alerts] Dashboard metrics received:', JSON.stringify(metrics).substring(0, 500));

            // Fetch historical incidents from TimescaleDB
            let historicalIncidents = [];
            try {
                const incidentsRes = await window.apiClient.authenticatedFetch(`/api/incidents/timeline?server=${encodeURIComponent(inst.name)}&hours=24`);
                if (incidentsRes.ok) {
                    const incidentsData = await incidentsRes.json();
                    historicalIncidents = incidentsData.incidents || [];
                }
            } catch (incErr) {
                appDebug('Failed to fetch incidents:', incErr);
            }

            // Analyze metrics for anomalies
            const anomalies = analyzeAnomalies(metrics, inst, historicalIncidents);

            // Update alerts badge in sidebar
            await window.updateAlertsBadge();

            // Generate alerts HTML
            let alertsHtml = '';

            if (anomalies.critical.length > 0) {
                alertsHtml += `
                    <div class="alert-group mt-3">
                        <h3 style="display:flex; align-items:center; gap: 6px; color: var(--danger); font-size: 0.9rem; margin-bottom: 0.75rem;">
                            <span style="position:relative; display:flex; width:10px; height:10px;">
                                <span style="position:absolute; width:100%; height:100%; border-radius:50%; background:var(--danger); opacity:0.8; animation: ping 1.5s cubic-bezier(0, 0, 0.2, 1) infinite;"></span>
                                <span style="position:relative; width:10px; height:10px; border-radius:50%; background:var(--danger);"></span>
                            </span>
                            Critical Anomalies (${anomalies.critical.length})
                        </h3>
                        <div style="display:grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 0.75rem;">
                            ${anomalies.critical.map(alert => `
                                <div class="glass-panel" style="border-left: 3px solid var(--danger); border-radius: 6px; padding: 0.75rem; display:flex; justify-content:space-between; align-items:flex-start; gap:0.5rem; background: rgba(220, 53, 69, 0.08);">
                                    <div style="flex:1; min-width:0;">
                                        <h4 style="margin:0 0 0.3rem 0; color:var(--danger); font-size:0.85rem;"><i class="fa-solid fa-triangle-exclamation"></i> ${alert.title}</h4>
                                        <p style="margin:0; color:var(--text); font-size:0.78rem;">${alert.description}</p>
                                        <small style="color:var(--text-muted); display:block; margin-top:0.3rem; font-size:0.7rem;"><i class="fa-regular fa-clock"></i> ${alert.timestamp}</small>
                                    </div>
                                    ${alert.route ? `<button class="btn btn-xs btn-outline" style="border-color:var(--danger); color:var(--danger); flex-shrink:0;" data-action="navigate" data-route="${alert.route}">${alert.actionText || 'Investigate'}</button>` : ''}
                                </div>
                            `).join('')}
                        </div>
                    </div>
                `;
            }

            if (anomalies.warning.length > 0) {
                alertsHtml += `
                    <div class="alert-group mt-3">
                        <h3 style="color: var(--warning); font-size: 0.9rem; margin-bottom: 0.75rem;"><i class="fa-solid fa-circle-exclamation"></i> Early Warnings (${anomalies.warning.length})</h3>
                        <div style="display:grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 0.6rem;">
                            ${anomalies.warning.map(alert => `
                                <div class="glass-panel" style="border-left: 3px solid var(--warning); border-radius: 6px; padding: 0.6rem 0.75rem; background: rgba(255, 193, 7, 0.05);">
                                    <h4 style="margin:0 0 0.25rem 0; color:var(--warning); font-size:0.82rem;">${alert.title}</h4>
                                    <p style="margin:0; color:var(--text); font-size:0.75rem;">${alert.description}</p>
                                    <div style="display:flex; justify-content:space-between; align-items:center; margin-top:0.4rem;">
                                        <small style="color:var(--text-muted); font-size:0.7rem;"><i class="fa-regular fa-clock"></i> ${alert.timestamp}</small>
                                        ${alert.route ? `<button class="btn btn-xs" style="background:transparent; border:1px solid var(--warning); color:var(--warning); padding:1px 6px;" data-action="navigate" data-route="${alert.route}">${alert.actionText || 'View'}</button>` : ''}
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                `;
            }

            if (anomalies.info.length > 0) {
                alertsHtml += `
                    <div class="alert-group mt-3">
                        <h3 style="color: var(--info); font-size: 0.9rem; margin-bottom: 0.6rem;"><i class="fa-solid fa-info-circle"></i> Correlation Insights (${anomalies.info.length})</h3>
                        <div style="display:grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 0.5rem;">
                            ${anomalies.info.map(alert => `
                                <div class="glass-panel" style="border-left: 2px solid var(--info); border-radius: 5px; padding: 0.5rem 0.7rem; opacity:0.9;">
                                    <h4 style="margin:0 0 0.2rem 0; color:var(--info); font-size:0.78rem;">${alert.title}</h4>
                                    <p style="margin:0; color:var(--text-muted); font-size:0.72rem;">${alert.description}</p>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                `;
            }

            if (anomalies.critical.length === 0 && anomalies.warning.length === 0 && anomalies.info.length === 0) {
                alertsHtml = `
                    <div class="alert alert-success" style="background: rgba(25, 135, 84, 0.1); border: 1px solid var(--success); border-radius: 8px; padding: 1.5rem;">
                        <div style="text-align:center;">
                            <i class="fa-solid fa-shield-heart" style="font-size: 3rem; color: var(--success); margin-bottom: 1rem;"></i>
                            <h4 style="color: var(--success); margin: 0 0 0.5rem 0;">All Systems Operational</h4>
                            <p style="margin:0; color: var(--text-muted);">No anomalies detected in the system. All metrics are within normal ranges.</p>
                        </div>
                    </div>
                `;
            }

            const engineAlertsSection = alertPageData.disconnected ? `
                <div class="glass-panel mt-2" style="padding:1rem; border-left:3px solid var(--warning);">
                    <strong>Alert engine offline</strong>
                    <p class="text-muted small mb-0">TimescaleDB is required for persisted alerts. Live anomaly cards below still use dashboard metrics.</p>
                </div>
            ` : `
                ${window.renderAlertsKpiStrip(alertPageData.openCount, alertPageData.blockingKpis, 'sqlserver')}
                <div class="card glass-panel mt-2">
                    <div class="card-header flex-between">
                        <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-bell text-danger"></i> Alert Engine (TimescaleDB)</h3>
                        <span class="text-muted" style="font-size:0.65rem;">Blocking, jobs, disk · ~60s evaluation</span>
                    </div>
                    <div style="padding:0.5rem;">
                        ${window.renderEngineAlertsTable(alertPageData.engineAlerts, { showAllStatuses: true })}
                    </div>
                </div>
            `;

            window.routerOutlet.innerHTML = `
                <div class="page-view active dashboard-sky-theme">
                    <div class="page-title flex-between dashboard-page-title-compact">
                        <div class="dashboard-title-line">
                            <h1 style="font-size:1.1rem; margin:0;"><i class="fa-solid fa-brain text-accent"></i> Alerts &amp; Anomaly Detection</h1>
                            <p class="subtitle">${window.escapeHtml(inst.name)} · Engine alerts + live dashboard rules</p>
                        </div>
                        <div class="dashboard-page-title-actions" style="display:flex; align-items:center; gap:0.75rem; flex-wrap:wrap;">
                            ${window.renderStatusStrip({ lastUpdateId: 'alertsLastUpdate', sourceBadgeId: 'alertsDataSourceBadge2', includeHealth: false, includeFreshness: false, autoRefreshText: 'Auto-refresh: every 15s' })}
                            <label style="display:flex; align-items:center; gap:0.4rem; font-size:0.75rem; cursor:pointer;">
                                <input type="checkbox" id="alertsAutoRefresh" checked style="width:14px; height:14px;"> Auto-refresh
                            </label>
                            <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshAlerts"><i class="fa-solid fa-refresh"></i> Refresh</button>
                        </div>
                    </div>
                    ${engineAlertsSection}
                    <div class="mt-2" style="display:grid; gap:0.75rem;">
                        <h3 style="font-size:0.8rem; color:var(--text-muted); margin:0.25rem 0 0;"><i class="fa-solid fa-chart-line"></i> Live dashboard anomalies</h3>
                        ${alertsHtml}
                    </div>

                    <!-- System Health Overview -->
                    <div class="tables-grid mt-3" style="display:grid; grid-template-columns:repeat(auto-fill, minmax(300px, 1fr)); gap:0.75rem;">
                        <div class="table-card glass-panel" style="text-align:center; padding:0.75rem;">
                            <div style="margin-bottom:0.5rem;">
                                <i class="fa-solid fa-heart-pulse" style="font-size:1.5rem; color:${getHealthColor(anomalies.healthScore)};"></i>
                            </div>
                            <h3 style="font-size:0.78rem; margin:0 0 0.3rem 0; color:var(--text-muted);">System Health Score</h3>
                            <div class="metric-value" style="font-size:2rem; color:${getHealthColor(anomalies.healthScore)}; line-height:1;">${anomalies.healthScore}%</div>
                            <div style="color:var(--text-muted); margin-top:0.3rem; font-size:0.78rem;">${getHealthStatus(anomalies.healthScore)}</div>
                            <div style="margin-top:0.6rem; background:var(--bg-tertiary); border-radius:6px; padding:0.5rem;">
                                <div style="display:flex; justify-content:space-around;">
                                    <div style="text-align:center;">
                                        <div style="font-size:1rem; font-weight:bold; color:var(--danger);">${anomalies.critical.length}</div>
                                        <div style="font-size:0.62rem; color:var(--text-muted);">Critical</div>
                                    </div>
                                    <div style="text-align:center;">
                                        <div style="font-size:1rem; font-weight:bold; color:var(--warning);">${anomalies.warning.length}</div>
                                        <div style="font-size:0.62rem; color:var(--text-muted);">Warnings</div>
                                    </div>
                                    <div style="text-align:center;">
                                        <div style="font-size:1rem; font-weight:bold; color:var(--info);">${anomalies.info.length}</div>
                                        <div style="font-size:0.62rem; color:var(--text-muted);">Insights</div>
                                    </div>
                                </div>
                            </div>
                        </div>

                        <div class="table-card glass-panel" style="padding:0.75rem;">
                            <h3 style="font-size:0.78rem; margin:0 0 0.6rem 0; color:var(--text-muted);"><i class="fa-solid fa-chart-line text-accent"></i> Metrics Summary</h3>
                            <div style="display:grid; grid-template-columns:repeat(3, 1fr); gap:0.5rem;">
                                <div class="glass-panel" style="padding:0.5rem; text-align:center; cursor:pointer;" role="button" tabindex="0" title="Open CPU drilldown" data-action="navigate" data-route="drilldown-cpu">
                                    <i class="fa-solid fa-microchip text-accent" style="font-size:1.1rem;"></i>
                                    <div style="font-size:1.1rem; font-weight:bold; line-height:1.2;">${(metrics.avg_cpu_load || 0).toFixed(1)}%</div>
                                    <div style="font-size:0.62rem; color:var(--text-muted);">CPU <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:0.55rem; opacity:0.7;"></i></div>
                                    <div style="height:3px; background:var(--bg-tertiary); border-radius:2px; margin-top:0.3rem;">
                                        <div style="height:100%; width:${Math.min(metrics.avg_cpu_load || 0, 100)}%; background:${metrics.avg_cpu_load > 90 ? 'var(--danger)' : (metrics.avg_cpu_load > 75 ? 'var(--warning)' : 'var(--success)')}; border-radius:2px;"></div>
                                    </div>
                                </div>
                                <div class="glass-panel" style="padding:0.5rem; text-align:center;">
                                    <i class="fa-solid fa-users text-success" style="font-size:1.1rem;"></i>
                                    <div style="font-size:1.1rem; font-weight:bold; line-height:1.2;">${metrics.active_users || 0}</div>
                                    <div style="font-size:0.62rem; color:var(--text-muted);">Users</div>
                                </div>
                                <div class="glass-panel" style="padding:0.5rem; text-align:center;">
                                    <i class="fa-solid fa-lock text-warning" style="font-size:1.1rem;"></i>
                                    <div style="font-size:1.1rem; font-weight:bold; line-height:1.2;">${metrics.total_locks || 0}</div>
                                    <div style="font-size:0.62rem; color:var(--text-muted);">Locks</div>
                                </div>
                                <div class="glass-panel" style="padding:0.5rem; text-align:center;">
                                    <i class="fa-solid fa-skull text-danger" style="font-size:1.1rem;"></i>
                                    <div style="font-size:1.1rem; font-weight:bold; line-height:1.2; color:${metrics.deadlocks > 0 ? 'var(--danger)' : 'inherit'};">${metrics.deadlocks || 0}</div>
                                    <div style="font-size:0.62rem; color:var(--text-muted);">Deadlocks</div>
                                </div>
                                <div class="glass-panel" style="padding:0.5rem; text-align:center; cursor:pointer;" role="button" tabindex="0" title="Open Memory drilldown" data-action="navigate" data-route="drilldown-memory">
                                    <i class="fa-solid fa-memory text-info" style="font-size:1.1rem;"></i>
                                    <div style="font-size:1.1rem; font-weight:bold; line-height:1.2;">${(metrics.memory_usage || 0).toFixed(1)}%</div>
                                    <div style="font-size:0.62rem; color:var(--text-muted);">Memory <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:0.55rem; opacity:0.7;"></i></div>
                                </div>
                                <div class="glass-panel" style="padding:0.5rem; text-align:center;">
                                    <i class="fa-solid fa-database text-accent" style="font-size:1.1rem;"></i>
                                    <div style="font-size:1.1rem; font-weight:bold; line-height:1.2;">${metrics.total_db_count || 0}</div>
                                    <div style="font-size:0.62rem; color:var(--text-muted);">Databases</div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            `;
            
            const lastUpdateEl = document.getElementById('alertsLastUpdate');
            if (lastUpdateEl) {
                lastUpdateEl.textContent = new Date().toLocaleTimeString();
            }
            window.updateSourceBadge('alertsDataSourceBadge2', response.headers.get('X-Data-Source'));
            
            const autoRefreshCheckbox = document.getElementById('alertsAutoRefresh');
            const enableAutoRefresh = () => {
                if (window.alertsViewInterval) {
                    clearInterval(window.alertsViewInterval);
                    window.alertsViewInterval = null;
                }
                const checked = autoRefreshCheckbox ? autoRefreshCheckbox.checked : true;
                if (checked) {
                    window.alertsViewInterval = window.registerInterval(loadAlerts, 15000);
                }
            };
            if (autoRefreshCheckbox && !autoRefreshCheckbox.__bound) {
                autoRefreshCheckbox.addEventListener('change', enableAutoRefresh);
                autoRefreshCheckbox.__bound = true;
            }
            enableAutoRefresh();
            window.bindAlertsPageActions(loadAlerts);

        } catch (error) {
            appDebug("Alerts analysis failed:", error);
            window.routerOutlet.innerHTML = `
                <div class="page-view active dashboard-sky-theme">
                    <div class="dashboard-page-title-compact">
                        <h1 style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:8px;"><i class="fa-solid fa-triangle-exclamation text-accent"></i> Anomaly Detection & Alerts</h1>
                    </div>
                    <div class="alert alert-danger mt-4">
                        <i class="fa-solid fa-exclamation-triangle"></i>
                        Failed to analyze system for anomalies: ${error.message}
                    </div>
                </div>
            `;
        }
    }
    
    await loadAlerts();
};

// Anomaly detection logic
function analyzeAnomalies(metrics, instance, historicalIncidents = []) {
    const anomalies = {
        critical: [],
        warning: [],
        info: [],
        healthScore: 100
    };

    const now = new Date().toLocaleTimeString();

    // Add historical incidents as alerts
    if (historicalIncidents.length > 0) {
        historicalIncidents.forEach(inc => {
            const alert = {
                title: inc.category || 'Incident',
                description: inc.description || '',
                timestamp: inc.time ? new Date(inc.time).toLocaleString() : now,
                route: 'incidents',
                actionText: 'View Details'
            };
            if (inc.severity === 'CRITICAL' || inc.severity === 'critical') {
                anomalies.critical.push(alert);
                anomalies.healthScore -= 15;
            } else if (inc.severity === 'WARNING' || inc.severity === 'warning') {
                anomalies.warning.push(alert);
                anomalies.healthScore -= 10;
            } else {
                anomalies.info.push(alert);
                anomalies.healthScore -= 5;
            }
        });
    }

    // CPU anomalies
    const cpuLoad = metrics.avg_cpu_load || 0;
    if (cpuLoad > 90) {
        anomalies.critical.push({
            title: 'Critical CPU Usage',
            description: `CPU utilization at ${cpuLoad.toFixed(1)}% - system may become unresponsive`,
            timestamp: now,
            route: 'drilldown-cpu',
            actionText: 'View CPU Details'
        });
        anomalies.healthScore -= 30;
    } else if (cpuLoad > 75) {
        anomalies.warning.push({
            title: 'High CPU Usage',
            description: `CPU utilization at ${cpuLoad.toFixed(1)}% - monitor for performance impact`,
            timestamp: now,
            route: 'drilldown-cpu',
            actionText: 'View CPU Details'
        });
        anomalies.healthScore -= 15;
    }

    // Deadlock detection
    const deadlocks = metrics.deadlocks || 0;
    if (deadlocks > 0) {
        anomalies.critical.push({
            title: 'Active Deadlocks Detected',
            description: `${deadlocks} deadlock(s) currently active - immediate attention required`,
            timestamp: now,
            route: 'drilldown-deadlocks',
            actionText: 'View Deadlock Analysis'
        });
        anomalies.healthScore -= 25;
    }

    // Extended events deadlock detection
    if (metrics.xevent_metrics && metrics.xevent_metrics.recent_events) {
        const recentDeadlocks = metrics.xevent_metrics.recent_events.filter(e => {
            try {
                const parsed = JSON.parse(e.parsed_payload_json || '{}');
                return parsed.event_type && parsed.event_type.toLowerCase().includes('deadlock');
            } catch (err) {
                return false;
            }
        });

        if (recentDeadlocks.length > 0) {
            anomalies.warning.push({
                title: 'Recent Deadlock Events',
                description: `${recentDeadlocks.length} deadlock(s) detected in the last hour`,
                timestamp: now,
                route: 'drilldown-deadlocks',
                actionText: 'View Deadlock History'
            });
            anomalies.healthScore -= 10;
        }
    }

    // Blocking detection (live DMV snapshot on dashboard)
    const activeBlocks = metrics.active_blocks ? metrics.active_blocks.filter(s => s.blocking_session_id !== 0).length : 0;
    if (activeBlocks >= 5) {
        anomalies.critical.push({
            title: 'Severe Blocking Chain',
            description: `${activeBlocks} sessions currently blocked - major performance impact`,
            timestamp: now,
            route: 'sqlserver-locks',
            actionText: 'View Blocking & Waits'
        });
        anomalies.healthScore -= 20;
    } else if (activeBlocks >= 1) {
        anomalies.warning.push({
            title: 'Active Blocking Detected',
            description: `${activeBlocks} session(s) currently blocked`,
            timestamp: now,
            route: 'sqlserver-locks',
            actionText: 'View Blocking & Waits'
        });
        anomalies.healthScore -= 10;
    }

    // Lock analysis
    const totalLocks = metrics.total_locks || 0;
    appDebug('[Anomaly] Total locks:', totalLocks);
    if (totalLocks > 1000) {
        anomalies.warning.push({
            title: 'High Lock Count',
            description: `${totalLocks} active locks - potential performance bottleneck`,
            timestamp: now,
            route: 'sqlserver-locks',
            actionText: 'View Lock Analysis'
        });
        anomalies.healthScore -= 5;
    }

    // Memory analysis
    const memoryUsage = metrics.memory_usage || 0;
    if (memoryUsage > 90) {
        anomalies.critical.push({
            title: 'Critical Memory Usage',
            description: `Memory utilization at ${memoryUsage.toFixed(1)}% - risk of out-of-memory errors`,
            timestamp: now,
            route: 'drilldown-memory',
            actionText: 'Investigate Memory'
        });
        anomalies.healthScore -= 25;
    } else if (memoryUsage > 80) {
        anomalies.warning.push({
            title: 'High Memory Usage',
            description: `Memory utilization at ${memoryUsage.toFixed(1)}% - monitor closely`,
            timestamp: now,
            route: 'drilldown-memory',
            actionText: 'Investigate Memory'
        });
        anomalies.healthScore -= 10;
    }

    // PLE analysis
    if (metrics.ple_history && metrics.ple_history.length > 0) {
        const avgPLE = metrics.ple_history.reduce((a, b) => a + b, 0) / metrics.ple_history.length;
        if (avgPLE < 100) {
            anomalies.warning.push({
                title: 'Low Page Life Expectancy',
                description: `Average PLE is ${avgPLE.toFixed(0)} seconds - buffer pool under pressure`,
                timestamp: now,
                route: 'drilldown-memory',
                actionText: 'View PLE Chart'
            });
            anomalies.healthScore -= 8;
        }
    }

    // Connection analysis
    const activeUsers = metrics.active_users || 0;
    if (activeUsers > 1000) {
        anomalies.info.push({
            title: 'High Connection Count',
            description: `${activeUsers} active connections - monitor for connection pool exhaustion`,
            timestamp: now
        });
    }

    // Ensure health score doesn't go below 0
    if (anomalies.healthScore < 0) {
        anomalies.healthScore = 0;
    }

    return anomalies;
}

window.analyzeSqlServerAnomalies = analyzeAnomalies;

window.dismissSqlDashboardAlertBanner = function(hours) {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || !inst.name) return;
    const h = Number(hours) > 0 ? Number(hours) : 1;
    localStorage.setItem('sql_dashboard_alert_snooze_' + inst.name, String(Date.now() + h * 3600000));
    const el = document.getElementById('sql-dashboard-alert-banner');
    if (el) {
        el.style.display = 'none';
        el.innerHTML = '';
    }
};

/** In-page banner on SQL dashboard when analyzeSqlServerAnomalies finds issues (sidebar badge alone is easy to miss). */
window.updateSqlDashboardAlertBanner = function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    const el = document.getElementById('sql-dashboard-alert-banner');
    if (!el || !inst || inst.type !== 'sqlserver') return;
    if (typeof window.analyzeSqlServerAnomalies !== 'function') return;

    const lm = window.appState.liveMetrics || {};
    const anomalies = window.analyzeSqlServerAnomalies(lm, inst, []);
    const nCrit = anomalies.critical.length;
    const nWarn = anomalies.warning.length;
    const nInfo = anomalies.info.length;
    const total = nCrit + nWarn + nInfo;

    if (total === 0) {
        el.style.display = 'none';
        el.innerHTML = '';
        return;
    }

    const snoozeUntil = parseInt(localStorage.getItem('sql_dashboard_alert_snooze_' + inst.name) || '0', 10);
    if (Date.now() < snoozeUntil) {
        el.style.display = 'none';
        return;
    }

    const border = nCrit > 0 ? 'var(--danger)' : (nWarn > 0 ? 'var(--warning)' : 'var(--info)');
    
    const messages = [];
    anomalies.critical.forEach(a => messages.push(`<span class="text-danger"><i class="fa-solid fa-circle-exclamation"></i> ${a.title}</span>`));
    anomalies.warning.forEach(a => messages.push(`<span class="text-warning"><i class="fa-solid fa-triangle-exclamation"></i> ${a.title}</span>`));
    
    // Determine the best target route. If only 1 anomaly exists and it has a route, go there.
    const allAnomalies = [...anomalies.critical, ...anomalies.warning, ...anomalies.info];
    let targetRoute = 'alerts';
    let actionText = 'View Incident Details';
    
    if (total === 1 && allAnomalies[0].route) {
        targetRoute = allAnomalies[0].route;
        actionText = allAnomalies[0].actionText || 'Investigate';
    }

    el.style.display = 'block';
    el.innerHTML = `
        <div class="glass-panel" style="border-left:4px solid ${border}; padding:0.75rem 1rem; margin:0 0 0.75rem 0; display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:1rem;">
            <div style="display:flex; align-items:center; gap:1rem; flex:1; min-width:300px;">
                <i class="fa-solid fa-bell" style="font-size:1.25rem; color:${border};"></i>
                <div style="display:flex; flex-direction:column; gap:0.25rem;">
                    <strong style="font-size:0.9rem;">${total} Performance Anomaly Detected</strong>
                    <div style="display:flex; flex-wrap:wrap; gap:0.75rem; font-size:0.75rem;">
                        ${messages.slice(0, 3).join(' · ')}${messages.length > 3 ? ' · ...' : ''}
                    </div>
                </div>
            </div>
            <div style="display:flex; gap:0.5rem; align-items:center;">
                <button type="button" class="btn btn-sm btn-accent" data-action="navigate" data-route="${targetRoute}" style="padding:0.4rem 0.8rem;"><i class="fa-solid fa-eye"></i> ${actionText}</button>
                <button type="button" class="btn btn-sm btn-outline" data-action="call" data-fn="dismissSqlDashboardAlertBanner" data-arg="1" title="Hide this banner for 1 hour">Dismiss</button>
            </div>
        </div>`;
};

function getHealthColor(score) {
    if (score <= 0) return '#94a3b8'; // Grey for no data
    if (score >= 80) return 'var(--success)';
    if (score >= 60) return 'var(--warning)';
    return 'var(--danger)';
}

function getHealthStatus(score) {
    if (score <= 0) return 'No Data';
    if (score >= 80) return 'Excellent';
    if (score >= 60) return 'Good';
    if (score >= 40) return 'Fair';
    return 'Poor';
}

// Refresh alerts function
window.refreshAlerts = async function() {
    const refreshBtn = document.querySelector('button[data-action="call" data-fn="refreshAlerts"]');
    if (refreshBtn) {
        refreshBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Analyzing...';
        refreshBtn.disabled = true;
    }

    try {
        await window.AlertsView();

        if (refreshBtn) {
            refreshBtn.innerHTML = '<i class="fa-solid fa-check"></i> Analysis Complete';
            setTimeout(() => {
                refreshBtn.innerHTML = '<i class="fa-solid fa-refresh"></i> Refresh Analysis';
                refreshBtn.disabled = false;
            }, 2000);
        }
    } catch (error) {
        appDebug("Alerts refresh failed:", error);
        if (refreshBtn) {
            refreshBtn.innerHTML = '<i class="fa-solid fa-exclamation-triangle"></i> Failed';
            setTimeout(() => {
                refreshBtn.innerHTML = '<i class="fa-solid fa-refresh"></i> Refresh Analysis';
                refreshBtn.disabled = false;
            }, 3000);
        }
    }
};

window.BestPracticesView = async function() {
    // Show loading state
    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="dashboard-page-title-compact flex-between">
                <div>
                    <h1 style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:8px;"><i class="fa-solid fa-shield-halved text-accent"></i> Best Practices & Guardrails</h1>
                    <p class="subtitle">Configuration audit + Workload Guardrails health assessment</p>
                </div>
                <div class="flex-between" style="align-items:center; gap: 0.5rem;">
                    <span class="text-muted" style="font-size:0.72rem;">Last Update: <span id="bpLastRefreshTime">Loading...</span></span>
                </div>
            </div>

            <div style="display:flex; justify-content:center; align-items:center; height:50vh;">
                <div class="spinner"></div><span style="margin-left: 1rem;">Auditing configuration against best practices...</span>
            </div>
        </div>
    `;

    try {
        const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};

        // Fetch best practices data
        const response = await window.apiClient.authenticatedFetch(`/api/sqlserver/best-practices?instance=${encodeURIComponent(inst.name)}`);
        if (!response.ok) {
            const contentType = response.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const errData = await response.json();
                throw new Error(errData.error || `Failed to fetch best practices data (HTTP ${response.status})`);
            }
            throw new Error(`Best practices API returned an invalid response (HTTP ${response.status})`);
        }
        
        const contentType = response.headers.get('content-type') || '';
        if (!contentType.includes('application/json')) {
            const text = await response.text();
            appDebug('Best Practices API returned non-JSON response:', text.substring(0, 200));
            throw new Error('Best practices API returned an invalid response');
        }
        
        const data = await response.json();

        // Fetch guardrails data
        let guardrailsData = null;
        try {
            const grResponse = await window.apiClient.authenticatedFetch(`/api/sqlserver/guardrails?instance=${encodeURIComponent(inst.name)}`);
            if (grResponse.ok) {
                guardrailsData = await grResponse.json();
                window.currentGuardrailsData = guardrailsData;
            }
        } catch (grErr) {
            appDebug('Guardrails data unavailable:', grErr);
        }

        // Combine and sort all checks by severity (RED first, then YELLOW, then GREEN)
        const allChecks = [];

        // Add server configuration checks
        data.server_config.forEach(check => {
            allChecks.push({
                type: 'server',
                name: check.configuration_name,
                value: check.current_value,
                status: check.status,
                message: check.message,
                category: 'Server Configuration'
            });
        });

        // Add database configuration checks
        data.database_config.forEach(check => {
            allChecks.push({
                type: 'database',
                name: check.database_name,
                value: `${check.page_verify} | Auto Shrink: ${check.auto_shrink ? 'ON' : 'OFF'} | Auto Close: ${check.auto_close ? 'ON' : 'OFF'} | Recovery Time: ${check.target_recovery_time}s`,
                status: check.status,
                message: check.message,
                category: 'Database Configuration'
            });
        });

        // Sort by severity: RED -> YELLOW -> GREEN
        const severityOrder = { 'RED': 0, 'YELLOW': 1, 'GREEN': 2 };
        allChecks.sort((a, b) => severityOrder[a.status] - severityOrder[b.status]);

        // Group by status for display
        const redChecks = allChecks.filter(c => c.status === 'RED');
        const yellowChecks = allChecks.filter(c => c.status === 'YELLOW');
        const greenChecks = allChecks.filter(c => c.status === 'GREEN');

        // Generate HTML
        let practicesHtml = '';

        if (redChecks.length > 0) {
            practicesHtml += `
                <div class="glass-panel" style="padding: 0.75rem; margin-bottom: 0.75rem; border-left: 4px solid var(--danger);">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0; color: var(--danger);"><i class="fa-solid fa-exclamation-triangle"></i> Critical Issues (${redChecks.length})</h3>
                    </div>
                    <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 0.5rem;">
                        ${redChecks.map(check => `
                            <div class="glass-panel" style="padding: 0.5rem; border: 1px solid var(--danger);">
                                <div style="display: flex; gap: 0.5rem; align-items: flex-start;">
                                    <i class="fa-solid fa-xmark-circle text-danger" style="font-size: 1rem; margin-top: 2px;"></i>
                                    <div>
                                        <div style="font-size: 0.85rem; font-weight: 600;">${window.escapeHtml(check.name)}</div>
                                        <div style="font-size: 0.75rem; color: var(--text-muted);"><strong>Current:</strong> ${window.escapeHtml(check.value)}</div>
                                        <div style="font-size: 0.75rem; color: var(--danger);">${window.escapeHtml(check.message)}</div>
                                        <span class="badge badge-outline" style="font-size: 0.65rem; margin-top: 0.25rem;">${check.category}</span>
                                    </div>
                                </div>
                            </div>
                        `).join('')}
                    </div>
                </div>
            `;
        }

        if (yellowChecks.length > 0) {
            practicesHtml += `
                <div class="glass-panel" style="padding: 0.75rem; margin-bottom: 0.75rem; border-left: 4px solid var(--warning);">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0; color: var(--warning);"><i class="fa-solid fa-exclamation-circle"></i> Recommendations (${yellowChecks.length})</h3>
                    </div>
                    <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 0.5rem;">
                        ${yellowChecks.map(check => `
                            <div class="glass-panel" style="padding: 0.5rem; border: 1px solid var(--warning);">
                                <div style="display: flex; gap: 0.5rem; align-items: flex-start;">
                                    <i class="fa-solid fa-exclamation-triangle text-warning" style="font-size: 1rem; margin-top: 2px;"></i>
                                    <div>
                                        <div style="font-size: 0.85rem; font-weight: 600;">${window.escapeHtml(check.name)}</div>
                                        <div style="font-size: 0.75rem; color: var(--text-muted);"><strong>Current:</strong> ${window.escapeHtml(check.value)}</div>
                                        <div style="font-size: 0.75rem; color: var(--warning);">${window.escapeHtml(check.message)}</div>
                                        <span class="badge badge-outline" style="font-size: 0.65rem; margin-top: 0.25rem;">${check.category}</span>
                                    </div>
                                </div>
                            </div>
                        `).join('')}
                    </div>
                </div>
            `;
        }

        if (greenChecks.length > 0) {
            practicesHtml += `
                <div class="glass-panel" style="padding: 0.75rem; margin-bottom: 0.75rem; border-left: 4px solid var(--success);">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size:0.85rem; margin:0; color: var(--success);"><i class="fa-solid fa-check-circle"></i> Compliant Settings (${greenChecks.length})</h3>
                    </div>
                    <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 0.5rem;">
                        ${greenChecks.map(check => `
                            <div class="glass-panel" style="padding: 0.5rem; border: 1px solid var(--success);">
                                <div style="display: flex; gap: 0.5rem; align-items: flex-start;">
                                    <i class="fa-solid fa-check-circle text-success" style="font-size: 1rem; margin-top: 2px;"></i>
                                    <div>
                                        <div style="font-size: 0.85rem; font-weight: 600;">${window.escapeHtml(check.name)}</div>
                                        <div style="font-size: 0.75rem; color: var(--text-muted);"><strong>Current:</strong> ${window.escapeHtml(check.value)}</div>
                                        <span class="badge badge-outline" style="font-size: 0.65rem; margin-top: 0.25rem;">${check.category}</span>
                                    </div>
                                </div>
                            </div>
                        `).join('')}
                    </div>
                </div>
            `;
        }

        if (allChecks.length === 0) {
            practicesHtml = `
                <div class="glass-panel" style="padding: 1rem; text-align: center;">
                    <i class="fa-solid fa-info-circle text-info" style="font-size: 2rem; margin-bottom: 0.5rem;"></i>
                    <div class="text-muted">No configuration data available. Unable to perform best practices audit.</div>
                </div>
            `;
        }

        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="dashboard-page-title-compact flex-between">
                    <div>
                        <h1 style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:8px;"><i class="fa-solid fa-shield-halved text-accent"></i> Best Practices & Guardrails</h1>
                        <p class="subtitle">Instance: ${window.escapeHtml(inst.name)}</p>
                    </div>
                    <div class="flex-between" style="align-items:center; gap: 0.5rem; flex-wrap:wrap;">
                        <span class="text-muted" style="font-size:0.72rem;">Last Update: <span id="bpLastRefreshTime">${new Date().toLocaleTimeString()}</span></span>
                        <button class="btn btn-sm btn-outline" style="color:#22c55e; border-color:#22c55e;" data-action="call" data-fn="exportSqlServerBestPracticesCSV">
                            <i class="fa-solid fa-file-csv"></i> Export
                        </button>
                        <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="refreshBestPractices">
                            <i class="fa-solid fa-refresh"></i> Refresh
                        </button>
                    </div>
                </div>

                <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 0.75rem; margin-bottom: 0.75rem;">
                    ${(() => {
                        let totalRed = redChecks.length;
                        let totalYellow = yellowChecks.length;
                        let totalGreen = greenChecks.length;
                        
                        if (guardrailsData && guardrailsData.summary) {
                            guardrailsData.summary.forEach(s => {
                                totalRed += s.critical;
                                totalYellow += s.warning;
                            });
                        }
                        
                        return `
                    <div class="glass-panel" style="padding: 0.75rem; text-align: center; border: 1px solid var(--danger);">
                        <div style="font-size: 1.5rem; font-weight: bold; color: var(--danger);">${totalRed}</div>
                        <div style="font-size: 0.75rem; color: var(--text-muted);">Critical</div>
                    </div>
                    <div class="glass-panel" style="padding: 0.75rem; text-align: center; border: 1px solid var(--warning);">
                        <div style="font-size: 1.5rem; font-weight: bold; color: var(--warning);">${totalYellow}</div>
                        <div style="font-size: 0.75rem; color: var(--text-muted);">Warnings</div>
                    </div>
                    <div class="glass-panel" style="padding: 0.75rem; text-align: center; border: 1px solid var(--success);">
                        <div style="font-size: 1.5rem; font-weight: bold; color: var(--success);">${totalGreen}</div>
                        <div style="font-size: 0.75rem; color: var(--text-muted);">Compliant</div>
                    </div>
                        `;
                    })()}
                </div>

                ${guardrailsData ? `
                <div class="glass-panel" style="padding: 0.75rem; margin-bottom: 0.75rem;">
                    <div class="card-header flex-between" style="margin-bottom: 0.5rem;">
                        <h3 style="font-size: 0.9rem; margin: 0;"><i class="fa-solid fa-heart-pulse text-accent"></i> Workload Guardrails Health Score</h3>
                        <span class="badge ${guardrailsData.health_status === 'CRITICAL' ? 'badge-danger' : guardrailsData.health_status === 'WARNING' ? 'badge-warning' : 'badge-success'}">${guardrailsData.health_status}</span>
                    </div>
                    <div style="display: flex; align-items: center; gap: 1rem;">
                        <div style="flex: 1; height: 8px; background: #333; border-radius: 4px; overflow: hidden;">
                            <div style="width: ${guardrailsData.health_score}%; height: 100%; background: ${guardrailsData.health_score < 50 ? 'var(--danger)' : guardrailsData.health_score < 75 ? 'var(--warning)' : 'var(--success)'};"></div>
                        </div>
                        <div style="font-size: 1.25rem; font-weight: bold; min-width: 50px;">${guardrailsData.health_score}</div>
                    </div>
                </div>

                <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 0.75rem; margin-bottom: 0.75rem;">
                    ${guardrailsData.summary && guardrailsData.summary.map((s) => {
                        const safeCatId = s.category.replace(/\s+/g, '-');
                        return `
                        <div class="glass-panel" style="padding: 0.75rem; cursor: pointer; border-left: 3px solid ${s.severity === 'CRITICAL' ? 'var(--danger)' : s.severity === 'WARNING' ? 'var(--warning)' : 'var(--success)'}; transition: background 0.2s;" data-action="guardrails-modal" data-category="${s.category}" data-catid="${safeCatId}">
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <div style="font-weight: 600; font-size: 0.85rem;">${s.category}</div>
                                <i class="fa-solid fa-eye" style="font-size: 0.75rem;"></i>
                            </div>
                            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">
                                ${s.critical > 0 ? `<span class="text-danger">${s.critical} Critical</span>` : ''}
                                ${s.critical > 0 && s.warning > 0 ? ' | ' : ''}
                                ${s.warning > 0 ? `<span class="text-warning">${s.warning} Warning</span>` : ''}
                                ${s.count === 0 ? '<span class="text-success">OK</span>' : ''}
                            </div>
                        </div>
                        `;
                    }).join('')}
                </div>
                ` : ''}

                <div class="mt-3">
                    ${practicesHtml}
                </div>
            </div>
        `;

    } catch (error) {
        appDebug("Best practices audit failed:", error);
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="dashboard-page-title-compact flex-between">
                    <div>
                        <h1 style="font-size:1.1rem; margin:0; display:flex; align-items:center; gap:8px;"><i class="fa-solid fa-shield-halved text-accent"></i> Best Practices & Guardrails</h1>
                        <p class="subtitle">Instance: ${window.escapeHtml(window.appState.currentInstanceName)}</p>
                    </div>
                </div>
                <div class="glass-panel" style="padding: 1rem; border: 1px solid var(--danger);">
                    <i class="fa-solid fa-exclamation-triangle text-danger"></i> Failed to perform configuration audit: ${error.message}
                </div>
            </div>
        `;
    }
};

// Refresh best practices function
window.refreshBestPractices = function() {
    window.BestPracticesView();
};

// Toggle guardrails detail section
window.toggleGuardrailsDetail = function(category, element) {
    const detailEl = document.getElementById('detail-' + category);
    const iconEl = element.querySelector('.fa-chevron-down, .fa-chevron-up');
    
    if (detailEl) {
        if (detailEl.style.display === 'none') {
            detailEl.style.display = 'block';
            if (iconEl) iconEl.classList.replace('fa-chevron-down', 'fa-chevron-up');
        } else {
            detailEl.style.display = 'none';
            if (iconEl) iconEl.classList.replace('fa-chevron-up', 'fa-chevron-down');
        }
    }
};

// Show guardrails detail in modal
window.showGuardrailsModal = function(category, safeCatId) {
    const existing = document.getElementById('guardrails-modal');
    if (existing) existing.remove();
    
    const categoryKey = category.replace(/-/g, ' ');
    const data = window.currentGuardrailsData;
    if (!data) return;
    
    const catMap = {
        'Storage & Files': data.storage_risks,
        'Disk Space': data.disk_space,
        'Transaction Logs': data.log_health,
        'Log Backups': data.log_backups,
        'Long Transactions': data.long_transactions,
        'Autogrowth': data.autogrowth_risks,
        'TempDB': data.tempdb_config ? [data.tempdb_config] : [],
        'Resource Governor': data.resource_governor
    };
    
    const items = catMap[categoryKey];
    const summary = data.summary ? data.summary.find(s => s.category === categoryKey) : null;
    
    let itemsHtml = '';
    if (!items || (Array.isArray(items) && items.length === 0)) {
        itemsHtml = '<div class="text-muted" style="padding: 1rem; text-align: center;">No issues found - all clear!</div>';
    } else if (categoryKey === 'TempDB' && items[0]) {
        const it = items[0];
        itemsHtml = `
            <div style="margin-bottom: 1rem;">
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; margin-bottom: 0.5rem;">
                    <div><strong>File Count:</strong> ${it.file_count || 0}</div>
                    <div><strong>Total Size:</strong> ${it.total_size_mb || 0} MB</div>
                </div>
                <div style="margin-bottom: 0.5rem;"><strong>Severity:</strong> <span class="${it.severity === 'CRITICAL' ? 'text-danger' : it.severity === 'WARNING' ? 'text-warning' : 'text-success'}">${it.severity || 'GREEN'}</span></div>
                <div style="margin-bottom: 0.5rem;">${it.message || ''}</div>
                ${it.drill_down ? `<div style="background: var(--bg-tertiary); padding: 0.75rem; border-radius: 6px; margin-bottom: 0.5rem;"><strong>Why it matters:</strong><br/>${it.drill_down}</div>` : ''}
                ${it.mitigation_sql ? `<div style="background: #1e3a5f; padding: 0.75rem; border-radius: 6px;"><strong>Mitigation SQL:</strong><pre style="margin: 0.5rem 0 0 0; white-space: pre-wrap; font-size: 0.75rem;">${it.mitigation_sql}</pre></div>` : ''}
                ${it.files && it.files.length > 0 ? `
                    <div style="margin-top: 0.75rem;"><strong>Files:</strong></div>
                    ${it.files.map(f => `<div style="color: var(--text-muted);">- ${f.logical_name || 'N/A'}: ${f.size_mb || 0} MB</div>`).join('')}
                ` : ''}
            </div>
        `;
    } else if (categoryKey === 'Resource Governor' && items) {
        itemsHtml = `
            <div style="margin-bottom: 1rem;">
                <div style="margin-bottom: 0.5rem;"><strong>Enabled:</strong> <span class="${items.is_enabled ? 'text-success' : 'text-danger'}">${items.is_enabled ? 'Yes' : 'No'}</span></div>
                <div style="margin-bottom: 0.5rem;"><strong>Severity:</strong> <span class="${items.severity === 'CRITICAL' ? 'text-danger' : items.severity === 'WARNING' ? 'text-warning' : 'text-success'}">${items.severity || 'GREEN'}</span></div>
                <div style="margin-bottom: 0.5rem;">${items.message || ''}</div>
                ${items.drill_down ? `<div style="background: var(--bg-tertiary); padding: 0.75rem; border-radius: 6px; margin-bottom: 0.5rem;"><strong>Why it matters:</strong><br/>${items.drill_down}</div>` : ''}
                ${items.mitigation_sql ? `<div style="background: #1e3a5f; padding: 0.75rem; border-radius: 6px;"><strong>Mitigation SQL:</strong><pre style="margin: 0.5rem 0 0 0; white-space: pre-wrap; font-size: 0.75rem;">${items.mitigation_sql}</pre></div>` : ''}
            </div>
        `;
    } else if (Array.isArray(items)) {
        itemsHtml = renderGuardrailsItems(items, categoryKey);
    }
    
    const modal = document.createElement('div');
    modal.id = 'guardrails-modal';
    modal.style.cssText = 'display:flex; position:fixed; z-index:99999; left:0; top:0; width:100%; height:100%; background-color:rgba(0,0,0,0.8); align-items:center; justify-content:center;';
    modal.innerHTML = `<div style="background:var(--bg-surface); margin:2%; padding:20px; border:1px solid var(--border-color,#333); border-radius:12px; width:95%; max-width:900px; max-height:90vh; overflow-y:auto; color:var(--text-primary,#e0e0e0);">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:1rem; border-bottom: 1px solid #333; padding-bottom: 0.5rem;">
            <div>
                <h3 style="margin:0; color:var(--accent,#3b82f6);">${category}</h3>
                <div style="font-size: 0.85rem; color: var(--text-muted); margin-top: 0.25rem;">
                    ${summary ? `<span class="${summary.severity === 'CRITICAL' ? 'text-danger' : summary.severity === 'WARNING' ? 'text-warning' : 'text-success'}">${summary.critical} Critical | ${summary.warning} Warnings</span>` : ''}
                </div>
            </div>
            <button data-action="close-id" data-target="guardrails-modal" style="background:transparent; border:1px solid #555; color:#e0e0e0; font-size:1.25rem; cursor:pointer; padding:0.25rem 0.6rem; border-radius:4px;">&times;</button>
        </div>
        <div style="max-height: 70vh; overflow-y: auto;">
            ${itemsHtml}
        </div>
    </div>`;
    document.body.appendChild(modal);
    modal.addEventListener('click', e => {
        if (e.target === modal) { modal.remove(); return; }
        const btn = e.target?.closest?.('[data-action="toggle-detail"]');
        if (btn) {
            const d = document.getElementById(btn.dataset.rowId);
            if (d) d.style.display = d.style.display === 'none' ? 'table-row' : 'none';
        }
    });
};

function renderItemDetails(item, category) {
    let details = '';
    let basicInfo = '';
    
    if (category === 'Storage & Files') {
        basicInfo = `<div><strong>${item.database_name || 'N/A'}</strong> - ${item.file_type || 'N/A'}</div>
            <div style="color: var(--text-muted);">Path: ${item.physical_name || 'N/A'}</div>
            <div style="color: var(--text-muted);">Size: ${item.size_mb || 0} MB</div>`;
    } else if (category === 'Disk Space') {
        basicInfo = `<div><strong>Drive ${item.drive_letter || 'N/A'}:</strong> ${item.free_space_mb || 0} MB free of ${item.total_size_mb || 0} MB (${(item.free_percent || 0).toFixed(1)}%)</div>
            <div style="color: var(--text-muted);">Log Size: ${item.log_size_mb || 0} MB</div>`;
    } else if (category === 'Transaction Logs') {
        basicInfo = `<div><strong>${item.database_name || 'N/A'}</strong> (${item.recovery_model || 'N/A'})</div>
            <div style="color: var(--text-muted);">Log Reuse Wait: ${item.log_reuse_wait || 'NOTHING'}</div>
            <div style="color: var(--text-muted);">VLF Count: ${item.vlf_count || 0}</div>`;
    } else if (category === 'Log Backups') {
        basicInfo = `<div><strong>${item.database_name || 'N/A'}</strong></div>
            <div style="color: var(--text-muted);">Last Backup: ${item.last_backup || 'Never'}</div>
            <div style="color: var(--text-muted);">${(item.minutes_ago || 0) > 0 ? item.minutes_ago + ' minutes ago' : 'N/A'}</div>`;
    } else if (category === 'Long Transactions') {
        basicInfo = `<div><strong>Session ${item.session_id || 'N/A'}</strong> - ${item.status || 'N/A'}</div>
            <div style="color: var(--text-muted);">Login: ${item.login_name || 'N/A'} | DB: ${item.database_name || 'N/A'}</div>
            <div style="color: var(--text-muted);">Duration: ${item.elapsed_seconds || 0}s | CPU: ${item.cpu_time || 0}ms | Reads: ${item.logical_reads || 0}</div>
            ${item.is_orphaned ? '<div class="text-danger" style="font-weight: bold;">⚠️ ORPHANED TRANSACTION</div>' : ''}
            ${item.blocking_session_id > 0 ? `<div class="text-danger">Blocking Session: ${item.blocking_session_id}</div>` : ''}`;
    } else if (category === 'Autogrowth') {
        basicInfo = `<div><strong>${item.database_name || 'N/A'}</strong> - ${item.logical_name || 'N/A'}</div>
            <div style="color: var(--text-muted);">${item.is_percent_growth ? 'Percentage: ' + item.growth + '%' : 'Growth: ' + item.growth + ' pages'}</div>`;
    }
    
    details = basicInfo;
    if (item.drill_down) {
        details += `<div style="background: var(--bg-base); padding: 0.5rem; border-radius: 4px; margin-top: 0.5rem;"><strong>Why it matters:</strong><br/>${item.drill_down}</div>`;
    }
    if (item.mitigation_sql) {
        details += `<div style="background: #1e3a5f; padding: 0.5rem; border-radius: 4px; margin-top: 0.5rem;"><strong>Mitigation SQL:</strong><pre style="margin: 0.5rem 0 0 0; white-space: pre-wrap; font-size: 0.7rem;">${item.mitigation_sql}</pre></div>`;
    }
    
    return details;
}

// Get guardrails detail HTML based on category
window.getGuardrailsDetailHtml = function(data, category) {
    // Convert dashes back to spaces for lookup
    const categoryKey = category.replace(/-/g, ' ');
    
    const catMap = {
        'Storage & Files': data.storage_risks,
        'Disk Space': data.disk_space,
        'Transaction Logs': data.log_health,
        'Log Backups': data.log_backups,
        'Long Transactions': data.long_transactions,
        'Autogrowth': data.autogrowth_risks,
        'TempDB': data.tempdb_config ? [data.tempdb_config] : [],
        'Resource Governor': data.resource_governor
    };
    
    const items = catMap[categoryKey];
    if (!items || !Array.isArray(items) || items.length === 0) {
        // Handle single object case (Resource Governor)
        if (categoryKey === 'Resource Governor' && items) {
            return renderResourceGovernorDetail(items);
        }
        return '<div style="font-size: 0.75rem; color: var(--text-muted);">No issues found.</div>';
    }
    
    if (categoryKey === 'TempDB') {
        return renderTempDBDetail(items[0]);
    }
    
    return renderGuardrailsItems(items, categoryKey);
};

function renderTempDBDetail(item) {
    if (!item) {
        return '<div style="font-size: 0.75rem; color: var(--text-muted);">No data available.</div>';
    }
    return `
        <div style="font-size: 0.75rem;">
            <div><strong>File Count:</strong> ${item.file_count || 0}</div>
            <div><strong>Total Size:</strong> ${item.total_size_mb || 0} MB</div>
            <div><strong>Severity:</strong> <span class="${item.severity === 'WARNING' ? 'text-warning' : 'text-success'}">${item.severity || 'GREEN'}</span></div>
            <div style="margin-top: 0.25rem;">${item.message || ''}</div>
            ${item.files && item.files.length > 0 ? `
                <div style="margin-top: 0.5rem;"><strong>Files:</strong></div>
                ${item.files.map(f => `<div style="color: var(--text-muted);">- ${f.logical_name || 'N/A'}: ${f.size_mb || 0} MB</div>`).join('')}
            ` : ''}
        </div>
    `;
}

function renderResourceGovernorDetail(item) {
    if (!item) {
        return '<div style="font-size: 0.75rem; color: var(--text-muted);">No data available.</div>';
    }
    return `
        <div style="font-size: 0.75rem;">
            <div><strong>Enabled:</strong> <span class="${item.is_enabled ? 'text-success' : 'text-warning'}">${item.is_enabled ? 'Yes' : 'No'}</span></div>
            <div><strong>Severity:</strong> <span class="${item.severity === 'WARNING' ? 'text-warning' : 'text-success'}">${item.severity || 'GREEN'}</span></div>
            <div style="margin-top: 0.25rem;">${item.message || ''}</div>
        </div>
    `;
}

function renderGuardrailsItems(items, category) {
    const sev = s => s === 'CRITICAL'
        ? '<span class="text-danger" style="font-size:0.7rem; white-space:nowrap;">● Critical</span>'
        : s === 'WARNING'
            ? '<span class="text-warning" style="font-size:0.7rem; white-space:nowrap;">● Warning</span>'
            : '<span class="text-success" style="font-size:0.7rem; white-space:nowrap;">● OK</span>';

    let headers, getRow;
    if (category === 'Log Backups') {
        headers = ['Database', 'Last Backup', 'Time Ago', 'Status'];
        getRow = i => [
            `<strong>${i.database_name || 'N/A'}</strong>`,
            i.last_backup || 'Never',
            (i.minutes_ago || 0) > 0 ? `${i.minutes_ago} min ago` : '—',
            sev(i.severity)
        ];
    } else if (category === 'Transaction Logs') {
        headers = ['Database', 'Recovery Model', 'Log Reuse Wait', 'VLF Count', 'Status'];
        getRow = i => [
            `<strong>${i.database_name || 'N/A'}</strong>`,
            i.recovery_model || 'N/A',
            i.log_reuse_wait || 'NOTHING',
            i.vlf_count || 0,
            sev(i.severity)
        ];
    } else if (category === 'Storage & Files') {
        headers = ['Database', 'File Type', 'Path', 'Size MB', 'Status'];
        getRow = i => [
            `<strong>${i.database_name || 'N/A'}</strong>`,
            i.file_type || 'N/A',
            `<span style="font-size:0.68rem; color:var(--text-muted); word-break:break-all;">${i.physical_name || 'N/A'}</span>`,
            i.size_mb || 0,
            sev(i.severity)
        ];
    } else if (category === 'Disk Space') {
        headers = ['Drive', 'Free MB', 'Total MB', 'Free %', 'Log MB', 'Status'];
        getRow = i => [
            `<strong>${i.drive_letter || 'N/A'}:</strong>`,
            i.free_space_mb || 0,
            i.total_size_mb || 0,
            `${(i.free_percent || 0).toFixed(1)}%`,
            i.log_size_mb || 0,
            sev(i.severity)
        ];
    } else if (category === 'Long Transactions') {
        headers = ['Session', 'Status', 'Login', 'Database', 'Duration', 'Blocking', 'Status'];
        getRow = i => [
            `<strong>${i.session_id || 'N/A'}</strong>`,
            i.status || 'N/A',
            i.login_name || 'N/A',
            i.database_name || 'N/A',
            `${i.elapsed_seconds || 0}s`,
            (i.blocking_session_id || 0) > 0 ? `<span class="text-danger">${i.blocking_session_id}</span>` : '—',
            sev(i.severity)
        ];
    } else if (category === 'Autogrowth') {
        headers = ['Database', 'File', 'Growth Type', 'Amount', 'Status'];
        getRow = i => [
            `<strong>${i.database_name || 'N/A'}</strong>`,
            i.logical_name || 'N/A',
            i.is_percent_growth ? 'Percentage' : 'Fixed',
            i.is_percent_growth ? `${i.growth}%` : `${i.growth} pages`,
            sev(i.severity)
        ];
    } else {
        headers = ['Item', 'Message', 'Status'];
        getRow = i => [i.database_name || i.name || 'N/A', i.message || '', sev(i.severity)];
    }

    const rows = items.map((item, idx) => {
        const cells = getRow(item);
        const remediation = window.getRemediationSql ? window.getRemediationSql(category, item) : '';
        const drillHtml = [
            item.message ? `<div style="color:var(--text-muted);margin-bottom:0.25rem;">${item.message}</div>` : '',
            item.drill_down ? `<div style="background:var(--bg-base);padding:0.4rem;border-radius:4px;margin-bottom:0.25rem;"><strong>Why:</strong> ${item.drill_down}</div>` : '',
            remediation ? `<div style="background:#1e3a5f;padding:0.4rem;border-radius:4px;"><strong>SQL Fix:</strong><pre style="margin:0.3rem 0 0;white-space:pre-wrap;font-size:0.65rem;">${remediation}</pre></div>` : ''
        ].join('');
        const rowBorder = item.severity === 'CRITICAL' ? 'var(--danger)' : item.severity === 'WARNING' ? 'var(--warning)' : 'var(--success)';
        return `
            <tr style="border-bottom:1px solid #2a2a2a; border-left:2px solid ${rowBorder};">
                ${cells.map(c => `<td style="padding:0.35rem 0.5rem; vertical-align:middle;">${c}</td>`).join('')}
                ${drillHtml ? `<td style="padding:0.35rem 0.4rem; white-space:nowrap;"><button style="background:transparent;border:1px solid #555;color:#bbb;font-size:0.62rem;cursor:pointer;padding:0.1rem 0.3rem;border-radius:3px;" data-action="toggle-detail" data-row-id="gr-d-${idx}">Details</button></td>` : '<td></td>'}
            </tr>
            ${drillHtml ? `<tr id="gr-d-${idx}" style="display:none; background:var(--bg-tertiary);"><td colspan="${cells.length + 1}" style="padding:0.5rem 0.75rem; font-size:0.75rem;">${drillHtml}</td></tr>` : ''}
        `;
    }).join('');

    return `
        <div style="overflow-x:auto;">
            <table style="width:100%; border-collapse:collapse; font-size:0.75rem;">
                <thead>
                    <tr style="border-bottom:1px solid #444;">
                        ${headers.map(h => `<th style="text-align:left; padding:0.35rem 0.5rem; color:var(--text-muted); font-weight:600; white-space:nowrap; font-size:0.7rem;">${h}</th>`).join('')}
                        <th style="padding:0.35rem 0.4rem;"></th>
                    </tr>
                </thead>
                <tbody>${rows}</tbody>
            </table>
        </div>
    `;
}

// Get remediation SQL for guardrails issues
window.getRemediationSql = function(category, item) {
    // Convert dashes to spaces
    const cat = (category || '').replace(/-/g, ' ');
    
    if (cat === 'Storage & Files') {
        return `-- Move database files off C: drive
ALTER DATABASE [${item.database_name}] 
MODIFY FILE (NAME = N'${item.logical_name}', FILENAME = 'D:\\Data\\[${item.database_name}_${item.file_type === 'LOG' ? 'log' : 'data'}.mdf');
-- Then restart SQL Server and move physical files`;
    }
    if (cat === 'Disk Space') {
        return `-- Free up disk space or add new drive
EXEC xp_fixeddrives;
-- Consider archiving old data, clearing logs, or adding disk space`;
    }
    if (cat === 'Transaction Logs') {
        return `-- Check log reuse wait and truncate logs
SELECT name, log_reuse_wait_desc FROM sys.databases WHERE name = '${item.database_name}';
-- For ACTIVE_TRANSACTION: COMMIT/ROLLBACK open transactions
-- For LOG_BACKUP: Run log backup
-- For NOTHING: Log is healthy`;
    }
    if (cat === 'Log Backups') {
        return `-- Configure log backup maintenance
BACKUP LOG [${item.database_name}] TO DISK = 'D:\\Backup\\[${item.database_name}_log.bak';
-- Schedule log backups every 15-30 minutes in SQL Agent`;
    }
    if (cat === 'Long Transactions') {
        return `-- Find and kill long running transaction
SELECT * FROM sys.dm_exec_requests WHERE session_id = ${item.session_id};
-- To kill: KILL ${item.session_id};`;
    }
    if (cat === 'Autogrowth') {
        return `-- Change autogrowth to fixed size (not percentage)
ALTER DATABASE [${item.database_name}]
MODIFY FILE (NAME = N'${item.logical_name}', MAXSIZE = UNLIMITED, FILEGROWTH = 64MB);
-- Recommended: 64MB for data files, 16MB for log files`;
    }
    if (cat === 'TempDB') {
        return `-- Add more tempdb data files
ALTER DATABASE tempdb
ADD LOG FILE (NAME = 'tempdev4', FILENAME = 'D:\\Data\\tempdev4.ndf', SIZE = 256MB, MAXSIZE = UNLIMITED, FILEGROWTH = 64MB);
-- Add 3-4 equally sized data files for tempdb`;
    }
    if (cat === 'Resource Governor') {
        return `-- Enable Resource Governor
ALTER RESOURCE GOVERNOR RECONFIGURE;
-- Or create workload groups for resource isolation`;
    }
    return '';
};

window.exportSqlServerBestPracticesCSV = function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    const url = `/api/sqlserver/best-practices/export?instance=${encodeURIComponent(inst.name)}`;
    window.downloadAuthenticatedCSV(url, `sqlserver_best_practices_${inst.name}.csv`).catch(err => {
        alert(`Export failed: ${err.message}`);
    });
};

window.exportSqlServerGuardrailsCSV = function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    const url = `/api/sqlserver/guardrails/export?instance=${encodeURIComponent(inst.name)}`;
    window.downloadAuthenticatedCSV(url, `sqlserver_guardrails_${inst.name}.csv`).catch(err => {
        alert(`Export failed: ${err.message}`);
    });
};
