/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Client-side routing and navigation manager between dashboard views. Handles URL routing and view rendering.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

// js/components/router.js - DOM Routing Abstraction Layer
window.routerOutlet = document.getElementById('router-outlet');
if (!window.routerOutlet) {
    console.error("router-outlet element not found!");
}
window.getCSSVar = function(name) { return getComputedStyle(document.documentElement).getPropertyValue(name).trim(); };
window.escapeHtml = function(unsafe) {
    if (unsafe === null || unsafe === undefined) return '';
    return String(unsafe).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
};

// Sidebar navigation is re-rendered dynamically; use event delegation so new `li`s keep working.
if (!window.__sidebarNavDelegateBound) {
    const sidebarNavEl = document.getElementById('sidebar-nav');
    if (sidebarNavEl) {
        sidebarNavEl.addEventListener('click', (e) => {
            const target = e.target;
            const li = target && target.closest ? target.closest('li[data-route]') : null;
            if (!li) return;
            const route = li.dataset.route;
            appDebug('Sidebar clicked:', route);
            if (route) window.appNavigate(route);
        });
        window.__sidebarNavDelegateBound = true;
    }
}

window.appNavigate = function(route, skipHistory = false) {
    appDebug('Navigating to:', route, 'instance idx:', window.appState.currentInstanceIdx);

    if (typeof route !== 'string' || route.length === 0 || route.length > 96 || !/^[a-z0-9-]+$/i.test(route)) {
        console.error('[Router] Invalid or unsafe route id:', route);
        return;
    }
    
    if (!window.routerOutlet) {
        console.error("Cannot navigate - router-outlet not found");
        return;
    }
    
    // Track navigation history for back button functionality
    if (!skipHistory && window.appState.activeViewId && window.appState.activeViewId !== route) {
        window.appState.navigationHistory.push(window.appState.activeViewId);
        if (window.appState.navigationHistory.length > 10) {
            window.appState.navigationHistory.shift();
        }
    }
    
    // Cleanup dashboard polling when navigating away from dashboard
    const previousRoute = window.appState.activeViewId;
    if (previousRoute === 'dashboard' && route !== 'dashboard') {
        if (window.cleanupDashboard) window.cleanupDashboard();
    }
    
    if (window.appState.dashboardPollingInterval && route !== 'dashboard') {
        clearInterval(window.appState.dashboardPollingInterval);
        window.appState.dashboardPollingInterval = null;
    }
    if (window.dashboardRefreshInterval && route !== 'dashboard') {
        clearInterval(window.dashboardRefreshInterval);
        window.dashboardRefreshInterval = null;
    }
    
    if (window.enterpriseMetricsInterval && route !== 'enterprise-metrics') {
        clearInterval(window.enterpriseMetricsInterval);
        window.enterpriseMetricsInterval = null;
    }
    
    if (window.alertsViewInterval && route !== 'alerts') {
        clearInterval(window.alertsViewInterval);
        window.alertsViewInterval = null;
    }
    if (window.jobsRefreshInterval && route !== 'jobs') {
        clearInterval(window.jobsRefreshInterval);
        window.jobsRefreshInterval = null;
    }
    
    if (window.cnpgTopologyInterval && route !== 'pg-cnpg') {
        clearInterval(window.cnpgTopologyInterval);
        window.cnpgTopologyInterval = null;
    }
    
    window.appState.activeViewId = route;

    if (route === 'setup-schema') {
        if (typeof window.SetupSchemaProgressView === 'function') {
            window.SetupSchemaProgressView();
        } else {
            window.routerOutlet.innerHTML = '<div class="page-view active"><h3 class="text-warning">Setup schema view not loaded</h3></div>';
        }
        return;
    }

    if (route === 'setup') {
        if (typeof window.SetupWizardView === 'function') {
            window.SetupWizardView();
        } else {
            window.routerOutlet.innerHTML = '<div class="page-view active"><h3 class="text-warning">Setup wizard not loaded</h3></div>';
        }
        return;
    }

    if (route === 'onboarding-servers') {
        const isLoggedIn = !!(window._auth && typeof window._auth.isLoggedIn === 'function' && window._auth.isLoggedIn());
        const isAdmin = !!(window._auth && typeof window._auth.isAdmin === 'function' && window._auth.isAdmin());
        if (!isLoggedIn) {
            if (typeof window.LoginView === 'function') window.LoginView();
            else window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Login required</h3></div>`;
            return;
        }
        if (!isAdmin) {
            window.routerOutlet.innerHTML = `<div class="page-view active" style="display:flex;align-items:center;justify-content:center;min-height:60vh;">
                <div style="text-align:center;">
                    <h2 class="text-muted">Access denied</h2>
                    <p class="text-muted">Only administrators can register monitoring servers.</p>
                    <a href="/" class="btn btn-accent" style="margin-top:1rem;">Home</a>
                </div></div>`;
            return;
        }
        if (typeof window.OnboardingMonitoredServersView === 'function') {
            window.OnboardingMonitoredServersView();
        } else {
            window.routerOutlet.innerHTML = '<div class="page-view active"><h3 class="text-warning">Onboarding view not loaded</h3></div>';
        }
        return;
    }
    
    // Admin panel only accessible via /admin URL and requires admin role
    if (route === 'admin') {
        const isLoggedIn = !!(window._auth && typeof window._auth.isLoggedIn === 'function' && window._auth.isLoggedIn());
        const isAdmin = !!(window._auth && typeof window._auth.isAdmin === 'function' && window._auth.isAdmin());

        if (!isLoggedIn) {
            // Better UX: prompt login instead of a dead-end "denied" message.
            if (typeof window.LoginView === 'function') {
                window.LoginView();
            } else {
                window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Login required</h3></div>`;
            }
            return;
        }
        if (!isAdmin) {
            window.routerOutlet.innerHTML = `<div class="page-view active" style="display:flex; align-items:center; justify-content:center; min-height:60vh;">
                <div style="text-align:center;">
                    <i class="fa-solid fa-ban" style="font-size:4rem; color:var(--danger); opacity:0.5;"></i>
                    <h2 style="margin-top:1rem; color:var(--text-muted);">Access Denied</h2>
                    <p class="text-muted">Admin privileges required.</p>
                    <a href="/" class="btn btn-accent" style="margin-top:1rem;"><i class="fa-solid fa-home"></i> Go to Dashboard</a>
                </div>
            </div>`;
            return;
        }

        if (window.AdminPanelView) { window.AdminPanelView(); }
        return;
    }

    document.querySelectorAll('.nav-links li').forEach(li => li.classList.remove('active'));
    const navLinks = document.querySelectorAll('.nav-links li[data-route]');
    for (let i = 0; i < navLinks.length; i++) {
        const li = navLinks[i];
        if (li.getAttribute('data-route') === route) {
            li.classList.add('active');
            break;
        }
    }

    if(window.currentCharts) {
        Object.values(window.currentCharts).forEach(c => c.destroy());
    }
    window.currentCharts = {};

    switch(route) {
        case 'global': 
            if (window.GlobalEstateView) window.GlobalEstateView(); 
            else window.routerOutlet.innerHTML = '<div class="page-view active"><h3>Loading Global...</h3></div>';
            break;
        case 'dashboard': 
            // Check if instance is selected
            const instance = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
            appDebug('Dashboard check - instance:', instance);
            if (!instance || typeof instance !== 'object' || Object.keys(instance).length === 0) {
                window.routerOutlet.innerHTML = `<div class="page-view active">
                    <h3 class="text-warning">Please select an instance first</h3>
                    <p>Go to the instance dropdown above and select a SQL Server instance.</p>
                </div>`;
                return;
            }
            if (instance.type === 'postgres') {
                window.appNavigate('pg-dashboard');
                return;
            }
            // Wait for DashboardView to be defined (in case scripts haven't loaded yet)
            const waitForDashboard = () => {
                if (window.DashboardView) {
                    appDebug('Calling DashboardView...');
                    window.DashboardView(); 
                } else {
                    appDebug('Waiting for DashboardView to load...');
                    setTimeout(waitForDashboard, 100);
                }
            };
            waitForDashboard();
            break;
        case 'drilldown-cpu': window.CpuDrilldown(); break;
        case 'drilldown-memory': if (window.MemoryDrilldown) window.MemoryDrilldown(); break;
        case 'mssql-cpu-dashboard': window.MssqlCpuDashboardView(); break;
        case 'instance-health': 
            appDebug('[Router] instance-health route triggered');
            appDebug('[Router] Checking for InstanceHealthDashboardView...');
            
            let attempts = 0;
            function tryLoadHealthDashboard() {
                attempts++;
                appDebug('[Router] Attempt', attempts, '- InstanceHealthDashboardView type:', typeof window.InstanceHealthDashboardView);
                
                if (typeof window.InstanceHealthDashboardView === 'function') {
                    appDebug('[Router] Found function, calling it');
                    try {
                        window.InstanceHealthDashboardView();
                    } catch(e) {
                        appDebug('[Router] Error:', e);
                        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-danger">Error: ${window.escapeHtml(String(e.message || e))}</h3></div>`;
                    }
                } else if (attempts < 20) {
                    appDebug('[Router] Not found, waiting 100ms...');
                    setTimeout(tryLoadHealthDashboard, 100);
                } else {
                    appDebug('[Router] Giving up, checking window keys...');
                    const keys = Object.keys(window).filter(k => k.toLowerCase().includes('instance') || k.toLowerCase().includes('health'));
                    appDebug('[Router] Related keys:', keys);
                    window.routerOutlet.innerHTML = `<div class="page-view active">
                        <h3 class="text-warning">DBA War Room unavailable</h3>
                        <p>Script failed to load. Please check console.</p>
                        <button data-action="reload" class="btn btn-primary">Refresh Page</button>
                    </div>`;
                }
            }
            tryLoadHealthDashboard();
            break;
        case 'drilldown-query': if(window.mssql_QueryDrilldown) window.mssql_QueryDrilldown(); break;
        case 'drilldown-top-queries': if(window.mssql_TopQueriesDrilldown) window.mssql_TopQueriesDrilldown(); break;
        case 'drilldown-metric-detail': if(window.mssql_MetricDetailDrilldown) window.mssql_MetricDetailDrilldown(); break;
        case 'drilldown-deadlocks': if(window.mssql_DeadlockDashboard) window.mssql_DeadlockDashboard(); break;
        case 'drilldown-growth': window.GrowthDrilldown(); break;
        case 'drilldown-index': window.IndexDrilldown(); break;
        case 'drilldown-locks': window.LocksDrilldown(); break;
        // "Deadlock graph" page: route to the functional dashboard implementation.
        case 'drilldown-deadlock': if(window.mssql_DeadlockDashboard) window.mssql_DeadlockDashboard(); break;
        case 'drilldown-bottlenecks': window.HistoricalBottlenecksView(); break;
        case 'drilldown-ha': window.HADashboardView(); break;
        case 'drilldown-pg-enterprise': window.PgEnterpriseDashboardView(); break;
        case 'enterprise-metrics': 
            const emInstance = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
            if (!emInstance || typeof emInstance !== 'object' || Object.keys(emInstance).length === 0) {
                window.routerOutlet.innerHTML = `<div class="page-view active">
                    <h3 class="text-warning">Please select an instance first</h3>
                    <p>Go to the instance dropdown above and select a SQL Server instance.</p>
                </div>`;
                return;
            }
            const waitForEM = () => {
                if (window.EnterpriseMetricsView) {
                    window.EnterpriseMetricsView(); 
                } else {
                    setTimeout(waitForEM, 100);
                }
            };
            waitForEM();
            break;
        case 'performance-debt':
            if (window.mssql_PerformanceDebtDashboard) {
                window.mssql_PerformanceDebtDashboard();
            } else {
                window.routerOutlet.innerHTML = '<div class="page-view active"><h3>Loading Performance Debt…</h3></div>';
                setTimeout(() => window.appNavigate('performance-debt'), 200);
            }
            break;
        case 'storage-index-health': {
            const sihInst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
            const runPg = () => {
                if (typeof window.PgStorageIndexHealthView === 'function') window.PgStorageIndexHealthView();
                else {
                    window.routerOutlet.innerHTML = '<div class="page-view active"><h3>Loading Index & Table Health…</h3></div>';
                    setTimeout(() => window.appNavigate('storage-index-health'), 200);
                }
            };
            const runMs = () => {
                if (typeof window.MssqlStorageIndexHealthView === 'function') window.MssqlStorageIndexHealthView();
                else {
                    window.routerOutlet.innerHTML = '<div class="page-view active"><h3>Loading Storage & Index Health…</h3></div>';
                    setTimeout(() => window.appNavigate('storage-index-health'), 200);
                }
            };
            if (sihInst && String(sihInst.type || '').toLowerCase() === 'postgres') runPg();
            else runMs();
            break;
        }
        case 'jobs': window.JobsView(); break;
        case 'alerts': window.AlertsView(); break;
        case 'incidents':
            if (typeof window.AlertsView === 'function') window.AlertsView();
            break;
        case 'login':
            if (typeof window.LoginView === 'function') window.LoginView();
            else window.routerOutlet.innerHTML = '<div class="page-view active"><h3>Login unavailable</h3></div>';
            break;
        case 'settings': window.SettingsView(); break;
        case 'best-practices': 
            if (typeof window.RulesEngineView === 'function') {
                window.RulesEngineView(); 
            } else {
                appDebug('RulesEngineView not loaded yet');
                window.routerOutlet.innerHTML = '<div class="alert alert-warning">Loading Best Practices...</div>';
                setTimeout(() => window.appNavigate('best-practices'), 500);
            }
            break;
        case 'live-diagnostics': window.LiveDiagnosticsView(); break;
        
        // Postgres
        case 'pg-dashboard': {
            const pgInst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
            if (pgInst && pgInst.type === 'sqlserver') {
                window.appNavigate('dashboard');
                return;
            }
            window.PgDashboardView();
            break;
        }
        case 'pg-sessions': window.PgSessionsView(); break;
        case 'pg-locks': window.PgLocksView(); break;
        case 'pg-queries': window.PgQueriesView(); break;
        case 'pg-explain': window.PgExplainView(); break;
        case 'pg-storage': window.PgStorageView(); break;
        case 'pg-replication': window.PgReplicationView(); break;
        case 'pg-logs': window.PgLogsView(); break;
        case 'pg-backups': window.PgBackupsView(); break;
        case 'pg-alerts': window.PgAlertsView(); break;
        // pg-config removed from sidebar (still callable if needed)
        case 'pg-config': window.PgConfigView(); break;
        case 'pg-cpu': window.PgCpuView(); break;
        case 'pg-memory': window.PgMemoryView(); break;
        // CNPG is being merged into replication engine; keep route for backward links.
        case 'pg-cnpg': window.CNPGClusterTopologyView(); break;
        case 'pg-best-practices':
            if (typeof window.PgBestPracticesView === 'function') {
                window.PgBestPracticesView();
            } else {
                window.routerOutlet.innerHTML = '<div class="alert alert-warning">Loading PostgreSQL Best Practices…</div>';
                setTimeout(() => window.appNavigate('pg-best-practices'), 500);
            }
            break;
        // Dynamic dashboard removed from sidebar; keep route for backward links.
        case 'dynamic-dashboard': window.DynamicDashboardView(); break;
        case 'sentinel-mock':
            if (typeof window.SentinelMockView === 'function') {
                window.SentinelMockView();
            } else {
                window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-muted">Sentinel mock not loaded</h3><p class="text-muted">This optional demo view is not bundled by default. To enable: add <code>&lt;script src="js/pages/ui_SentinelMock.js"&gt;&lt;/script&gt;</code> to <code>index.html</code> (after <code>router.js</code>), then open <code>sentinel-mock</code> again.</p><p><button type="button" class="btn btn-primary" data-action="navigate" data-route="global">Global Estate</button></p></div>`;
            }
            break;
        default:
            console.warn('[Router] Unknown route:', route);
            window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Page not found</h3><p class="text-muted">No view is registered for <code>${window.escapeHtml(route)}</code>.</p><p><button type="button" class="btn btn-primary" data-action="navigate" data-route="global">Go to Global Estate</button></p></div>`;
    }
};

window.appNavigateBack = function() {
    if (window.appState.navigationHistory.length > 0) {
        const previousRoute = window.appState.navigationHistory.pop();
        window.appNavigate(previousRoute, true); // Skip adding to history when going back
    }
};

window.router = {
    populateInstanceDropdown() {
        const sel = document.getElementById('instance-select');
        if (!sel) return;
        
        sel.innerHTML = '<option value="-1" disabled selected>-- Select Target Instance --</option>';
        
        if (!window.appState.config?.instances) {
            window.appState.currentInstanceIdx = -1;
            window.router.populateDatabaseDropdown();
            return;
        }
        
        const sorted = [...window.appState.config.instances].map((inst, i) => ({inst, i})).sort((a,b) => a.inst.name.localeCompare(b.inst.name));
        
        sorted.forEach(({inst, i}) => {
            const opt = document.createElement('option');
            opt.value = i; opt.textContent = `${inst.name} (${inst.type})`;
            sel.appendChild(opt);
        });
        window.appState.currentInstanceIdx = -1;
        window.router.populateDatabaseDropdown();
    },

    populateDatabaseDropdown() {
        const dbSel = document.getElementById('database-select');
        const brand = document.getElementById('brand-icon');
        const sidebarNav = document.getElementById('sidebar-nav');
        const adminLi = (window._auth && typeof window._auth.isAdmin === 'function' && window._auth.isAdmin())
            ? '<li data-route="admin" id="nav-admin"><i class="fa-solid fa-user-shield"></i> Admin</li>'
            : '';

        if (window.appState.currentInstanceIdx === -1) {
            dbSel.innerHTML = '<option value="all">-- N/A --</option>';
            brand.className = 'fa-solid fa-earth-americas xl-icon logo-icon text-accent';
            sidebarNav.innerHTML = '<li data-route="global" class="active"><i class="fa-solid fa-globe"></i> Global Estate Overview</li>' + adminLi;
            // Click handling is delegated globally.
            return;
        }

        const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
        dbSel.innerHTML = '<option value="all">-- Loading Databases --</option>';

        // Fetch databases from server if postgres
        if (inst.type === 'postgres') {
            window.apiClient.authenticatedFetch(`/api/postgres/databases?instance=${encodeURIComponent(inst.name)}`)
                .then(response => {
                    if (!response.ok) {
                        throw new Error(`HTTP ${response.status}`);
                    }
                    const contentType = response.headers.get('content-type') || '';
                    if (!contentType.includes('application/json')) {
                        throw new Error('Server returned non-JSON response');
                    }
                    return response.json();
                })
                .then(data => {
                    dbSel.innerHTML = '<option value="all">-- All Databases --</option>';
                    if (data.databases && data.databases.length > 0) {
                        data.databases.forEach(db => {
                            const opt = document.createElement('option');
                            opt.value = db;
                            opt.textContent = db;
                            dbSel.appendChild(opt);
                        });
                        window.appState.currentDatabase = data.databases[0];
                        dbSel.value = data.databases[0];
                        appDebug(`[Router] Loaded ${data.databases.length} databases for ${inst.name}`);
                    } else {
                        window.appState.currentDatabase = 'all';
                        console.warn(`[Router] No databases returned for ${inst.name}`);
                    }
                })
                .catch(error => {
                    console.error('[Router] Error fetching databases:', error);
                    const msg = (error && error.message) ? String(error.message) : 'Unknown error';
                    dbSel.innerHTML = `<option value="all">-- Error: ${window.escapeHtml(msg)} --</option>`;
                    window.appState.currentDatabase = 'all';
                });
        } else {
            // For MSSQL, use static list
            dbSel.innerHTML = '<option value="all">-- All Databases --</option>';
            inst.databases.forEach(db => {
                const opt = document.createElement('option'); opt.value=db; opt.textContent=db; dbSel.appendChild(opt);
            });
            if (inst.databases && inst.databases.length > 0) {
                window.appState.currentDatabase = inst.databases[0];
                dbSel.value = inst.databases[0];
            } else {
                window.appState.currentDatabase = 'all';
            }
        }
        
        if(inst.type==='postgres') { 
            brand.className='fa-solid fa-database xl-icon logo-icon text-accent'; 
            sidebarNav.innerHTML = `
                <li data-route="pg-dashboard" id="nav-pg-dashboard"><i class="fa-solid fa-gauge-high"></i> Control Center</li>
                <li data-route="pg-sessions"><i class="fa-solid fa-network-wired"></i> Sessions & Activity</li>
                <li data-route="pg-locks"><i class="fa-solid fa-link-slash"></i> Locks & Blocking</li>
                <li data-route="pg-queries"><i class="fa-solid fa-bolt"></i> Query Performance</li>
                <li data-route="pg-explain"><i class="fa-solid fa-diagram-project"></i> EXPLAIN Analyzer</li>
                <li data-route="pg-storage"><i class="fa-solid fa-hard-drive"></i> Storage & Vacuum</li>
                <li data-route="storage-index-health"><i class="fa-solid fa-boxes-stacked"></i> Index & Table Health</li>
                <li data-route="pg-replication"><i class="fa-solid fa-clone"></i> Replication, HA & Cluster</li>
                <li data-route="pg-best-practices"><i class="fa-solid fa-shield-halved"></i> Best Practices</li>
                <li data-route="pg-cpu"><i class="fa-solid fa-microchip"></i> CPU Usage</li>
                <li data-route="pg-memory"><i class="fa-solid fa-memory"></i> Memory Usage</li>
                <li data-route="pg-alerts"><i class="fa-solid fa-bell text-danger"></i> Alerts & Events</li>
                ${adminLi}
            `;
        } else { 
            brand.className='fa-brands fa-microsoft xl-icon logo-icon text-accent'; 
            sidebarNav.innerHTML = `
                <li data-route="dashboard" id="nav-dashboard"><i class="fa-solid fa-gauge-high"></i> Instance Dashboard</li>
                <li data-route="mssql-cpu-dashboard"><i class="fa-solid fa-microchip"></i> CPU Dashboard</li>
                <li data-route="drilldown-memory"><i class="fa-solid fa-memory"></i> Memory Analyzer</li>
                <li data-route="live-diagnostics"><i class="fa-solid fa-bolt text-warning"></i> Real-Time Diagnostics</li>
                <!-- Query Bottlenecks is now treated as a drilldown from Top Offenders -->
                <li data-route="drilldown-ha"><i class="fa-solid fa-server"></i> HA/AG Monitor</li>
                <li data-route="enterprise-metrics"><i class="fa-solid fa-chart-line"></i> Enterprise Metrics</li>
                <li data-route="storage-index-health"><i class="fa-solid fa-boxes-stacked"></i> Storage & Index Health</li>
                <li data-route="performance-debt"><i class="fa-solid fa-screwdriver-wrench"></i> Performance Debt</li>
                <li data-route="jobs" id="nav-agent-jobs"><i class="fa-solid fa-briefcase"></i> SQL Agent Jobs</li>
                <li data-route="alerts"><i class="fa-solid fa-triangle-exclamation"></i> Alerts <span id="alerts-badge" class="badge badge-danger">0</span></li>
                <li data-route="best-practices"><i class="fa-solid fa-shield-halved"></i> Best Practices</li>
                ${adminLi}
            `;
        }
    }
};

// Event Listeners strictly mounting globally mapped nodes
document.getElementById('instance-select').addEventListener('change', (e) => {
    window.appState.currentInstanceIdx = parseInt(e.target.value);
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    
    if (!inst) {
        window.appNavigate('global');
        return;
    }
    
    window.router.populateDatabaseDropdown();
    
    if(window.appState.activeViewId === 'global') {
        window.appNavigate(inst.type === 'postgres' ? 'pg-dashboard' : 'dashboard');
    } else {
        const isCurrentRoutePG = window.appState.activeViewId.startsWith('pg-');
        const isSharedEngineRoute = window.appState.activeViewId === 'storage-index-health';
        const root = inst.type === 'postgres' ? 'pg-dashboard' : 'dashboard';
        
        if(inst.type === 'postgres' && !isCurrentRoutePG && !isSharedEngineRoute) {
            window.appNavigate(root);
        } else if(inst.type === 'sqlserver' && isCurrentRoutePG) {
            window.appNavigate(root);
        } else {
            window.appNavigate(window.appState.activeViewId);
        }
    }
});

document.getElementById('database-select').addEventListener('change', (e) => {
    window.appState.currentDatabase = e.target.value;
    if(window.appState.activeViewId !== 'global') window.appNavigate(window.appState.activeViewId);
});

document.getElementById('theme-toggle').addEventListener('change', (e) => {
    document.documentElement.setAttribute('data-theme', e.target.checked ? 'dark' : 'light');
    localStorage.setItem('theme', e.target.checked ? 'dark' : 'light');
    window.appNavigate(window.appState.activeViewId);
});

// Sidebar collapse toggle (for smaller screens)
(function initSidebarCollapse() {
    if (window.__sidebarCollapseBound) return;
    window.__sidebarCollapseBound = true;

    const syncInlineToggle = (collapsed) => {
        const inlineBtn = document.getElementById('sidebar-toggle-inline');
        if (!inlineBtn) return;
        if (collapsed) {
            inlineBtn.title = 'Expand sidebar';
            inlineBtn.setAttribute('aria-label', 'Expand sidebar');
            inlineBtn.innerHTML = '<span class="sidebar-toggle-glyph" aria-hidden="true">&gt;&gt;</span>';
        } else {
            inlineBtn.title = 'Collapse sidebar';
            inlineBtn.setAttribute('aria-label', 'Collapse sidebar');
            inlineBtn.innerHTML = '<span class="sidebar-toggle-glyph" aria-hidden="true">&lt;&lt;</span>';
        }
    };

    const apply = (collapsed) => {
        document.body.classList.toggle('sidebar-collapsed', !!collapsed);
        localStorage.setItem('sidebar_collapsed', collapsed ? '1' : '0');
        syncInlineToggle(!!collapsed);
    };

    // Default: if user never chose, auto-collapse on laptop widths.
    const saved = localStorage.getItem('sidebar_collapsed');
    if (saved === null) {
        apply(window.innerWidth <= 1200);
    } else {
        apply(saved === '1');
    }

    const inlineBtn = document.getElementById('sidebar-toggle-inline');
    if (inlineBtn) {
        inlineBtn.addEventListener('click', () => {
            const next = !document.body.classList.contains('sidebar-collapsed');
            apply(next);
        });
    }
})();
