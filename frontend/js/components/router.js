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

// ── Interval & Fetch Registry ──────────────────────────────────────────────
// All page-level polling intervals must be registered here so they are
// automatically cleared on every navigation.
window._registeredIntervals = new Set();
window._currentPageAbortController = new AbortController();

/**
 * Register a polling interval that will be automatically cleared on navigation.
 * Drop-in replacement for setInterval().
 */
window.registerInterval = function(fn, ms) {
    const id = setInterval(fn, ms);
    window._registeredIntervals.add(id);
    return id;
};

/**
 * Get an AbortSignal that will be triggered on next navigation.
 * Use this for all fetch/api calls in page modules.
 */
window.getPageSignal = function() {
    return window._currentPageAbortController.signal;
};

/**
 * Clear all registered intervals and abort pending fetches.
 * Called automatically on every navigation.
 */
window.clearAllIntervals = function() {
    // Abort any in-flight fetch calls from the previous page
    if (window._currentPageAbortController) {
        window._currentPageAbortController.abort();
    }
    window._currentPageAbortController = new AbortController();

    // Clear all intervals
    window._registeredIntervals.forEach(id => clearInterval(id));
    window._registeredIntervals.clear();
};
// ─────────────────────────────────────────────────────────────────────────────

// ── Dashboard Guide Back Button ───────────────────────────────────────────────
// A floating "Back to Dashboard Guide" button injected only when the user
// navigated to a dashboard by clicking a link inside a Dashboard Guide page.
// Automatically removed at the start of every subsequent navigation.
window._pendingGuideReturn = null;

function _removeGuideBackBtn() {
    const el = document.getElementById('_sql-optima-guide-back');
    if (el) el.remove();
}

function _injectGuideBackBtn(guideRoute) {
    _removeGuideBackBtn();
    const isPostgres = guideRoute === 'pg-dashboard-guide';
    const label = isPostgres ? 'PostgreSQL Dashboard Guide' : 'SQL Server Dashboard Guide';
    const accentColor = isPostgres ? '#2563eb' : '#0078d4';
    const wrap = document.createElement('div');
    wrap.id = '_sql-optima-guide-back';
    wrap.innerHTML = `<style>@keyframes _gbIn{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:translateY(0)}}</style>`;
    const btn = document.createElement('button');
    btn.setAttribute('aria-label', 'Back to Dashboard Guide');
    btn.style.cssText = [
        'position:fixed', 'bottom:28px', 'right:28px', 'z-index:9998',
        'background:var(--bg-surface)', 'border:1px solid var(--border-color)',
        'border-radius:24px', 'padding:9px 20px', 'cursor:pointer',
        'display:flex', 'align-items:center', 'gap:8px',
        'box-shadow:0 4px 20px rgba(0,0,0,0.22)',
        'font-size:0.8rem', 'font-weight:600', 'color:var(--text-primary)',
        'font-family:inherit', 'animation:_gbIn 0.22s ease',
        'transition:background 0.15s,color 0.15s,transform 0.15s,box-shadow 0.15s',
    ].join(';');
    btn.innerHTML = `<i class="fa-solid fa-arrow-left" style="font-size:0.72rem;opacity:0.8;"></i><span>Back to ${label}</span>`;
    btn.addEventListener('mouseenter', () => {
        btn.style.background = accentColor;
        btn.style.color = '#fff';
        btn.style.transform = 'translateY(-2px)';
        btn.style.boxShadow = `0 8px 24px rgba(0,0,0,0.28)`;
    });
    btn.addEventListener('mouseleave', () => {
        btn.style.background = '';
        btn.style.color = '';
        btn.style.transform = '';
        btn.style.boxShadow = '0 4px 20px rgba(0,0,0,0.22)';
    });
    btn.addEventListener('click', () => {
        _removeGuideBackBtn();
        window.appNavigate(guideRoute);
    });
    wrap.appendChild(btn);
    document.body.appendChild(wrap);
}
// ─────────────────────────────────────────────────────────────────────────────

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
            if (route) window.appNavigate(route, true);
        });
        window.__sidebarNavDelegateBound = true;
    }
}

window.appNavigate = function(route, skipHistory = false) {
    appDebug('Navigating to:', route, 'instance idx:', window.appState.currentInstanceIdx);

    // Remove any floating guide back button from the previous page
    _removeGuideBackBtn();

    // Clear all registered polling intervals and abort pending fetches from the previous page
    window.clearAllIntervals();

    if (typeof route !== 'string' || route.length === 0 || route.length > 96 || !/^[a-z0-9-]+$/i.test(route)) {
        console.error('[Router] Invalid or unsafe route id:', route);
        return;
    }
    
    if (!window.routerOutlet) {
        console.error("Cannot navigate - router-outlet not found");
        return;
    }
    
    if (skipHistory) {
        window.appState.navigationHistory = [];
    }

    // Track navigation history for back button functionality
    if (!skipHistory && window.appState.activeViewId && window.appState.activeViewId !== route) {
        window.appState.navigationHistory.push(window.appState.activeViewId);
        if (window.appState.navigationHistory.length > 10) {
            window.appState.navigationHistory.shift();
        }
    }
    
    // Global back button visibility handler
    setTimeout(() => {
        const backBtns = document.querySelectorAll('[data-action="navigate-back"]');
        const hasHistory = window.appState.navigationHistory && window.appState.navigationHistory.length > 0;
        backBtns.forEach(btn => {
            btn.style.display = hasHistory ? 'inline-flex' : 'none';
        });
    }, 100);
    
    // Cleanup dashboard polling when navigating away from dashboard
    const previousRoute = window.appState.activeViewId;
    if ((previousRoute === 'dashboard' || previousRoute === 'sqlserver-health-v2') && route !== previousRoute) {
        if (window.cleanupDashboard) window.cleanupDashboard();
    }
    
    if (window.appState.dashboardPollingInterval && route !== 'dashboard' && route !== 'sqlserver-health-v2') {
        clearInterval(window.appState.dashboardPollingInterval);
        window.appState.dashboardPollingInterval = null;
    }
    if (window.dashboardRefreshInterval && route !== 'dashboard' && route !== 'sqlserver-health-v2') {
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

    // Refresh sidebar dynamically on navigation
    if (window.router && typeof window.router.populateDatabaseDropdown === 'function') {
        window.router.populateDatabaseDropdown();
    }

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

    // Helper for lazy-loaded views to prevent infinite loops and provide unified error handling
    function loadLazyView(viewFnName, routeId, displayName) {
        let attempts = 0;
        const tryLoad = () => {
            attempts++;
            if (typeof window[viewFnName] === 'function') {
                try {
                    window[viewFnName]();
                } catch(e) {
                    console.error(`[Router] Error in ${viewFnName}:`, e);
                    window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-danger">Error loading ${displayName}</h3><p>${window.escapeHtml(String(e.message || e))}</p></div>`;
                }
            } else if (attempts < 15) {
                window.routerOutlet.innerHTML = `<div class="page-view active"><h3>Loading ${displayName}…</h3></div>`;
                setTimeout(tryLoad, 200);
            } else {
                window.routerOutlet.innerHTML = `
                    <div class="page-view active">
                        <h3 class="text-warning">${displayName} unavailable</h3>
                        <p>The dashboard script failed to load. Please try refreshing the page.</p>
                        <button class="btn btn-primary" onclick="window.location.reload()">Refresh Page</button>
                    </div>`;
            }
        };
        tryLoad();
    }

    // If this navigation was initiated from a Dashboard Guide page, schedule a
    // floating back button to appear once the target page has rendered.
    if (window._pendingGuideReturn) {
        const _gr = window._pendingGuideReturn;
        window._pendingGuideReturn = null;
        setTimeout(() => _injectGuideBackBtn(_gr), 180);
    }

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

            // Always use the new optimized dashboard for SQL Server
            window.appNavigate('sqlserver-health-v2');
            break;
        case 'sqlserver-health-v2':
            loadLazyView('SqlServerHealthV2View', 'sqlserver-health-v2', 'Health Dashboard');
            break;
        case 'sqlserver-waits':
            loadLazyView('WaitStatsV2View', 'sqlserver-waits', 'Wait Stats');
            break;
        case 'drilldown-cpu': window.CpuDrilldown(); break;
        case 'drilldown-memory': if (window.MemoryDrilldown) window.MemoryDrilldown(); break;
        case 'sqlserver-cpu-dashboard': window.SqlServerCpuDashboardView(); break;
        case 'sqlserver-workload':
            loadLazyView('SqlServerWorkloadDashboardView', 'sqlserver-workload', 'Workload Analytics');
            break;
        case 'instance-health': 
            loadLazyView('InstanceHealthDashboardView', 'instance-health', 'DBA War Room');
            break;
        case 'drilldown-query': if(window.sqlserver_QueryDrilldown) window.sqlserver_QueryDrilldown(); break;
        case 'drilldown-top-queries': if(window.sqlserver_TopQueriesDrilldown) window.sqlserver_TopQueriesDrilldown(); break;
        case 'drilldown-metric-detail': if(window.sqlserver_MetricDetailDrilldown) window.sqlserver_MetricDetailDrilldown(); break;
        case 'drilldown-deadlocks': if(window.sqlserver_DeadlockDashboard) window.sqlserver_DeadlockDashboard(); break;
        case 'drilldown-growth': window.GrowthDrilldown(); break;
        case 'drilldown-index': window.IndexDrilldown(); break;
        case 'drilldown-locks': window.LocksDrilldown(); break;
        // "Deadlock graph" page: route to the functional dashboard implementation.
        case 'drilldown-deadlock': if(window.sqlserver_DeadlockDashboard) window.sqlserver_DeadlockDashboard(); break;
        case 'drilldown-bottlenecks': window.HistoricalBottlenecksView(); break;
        case 'drilldown-ha':
            loadLazyView('HADashboardView', 'drilldown-ha', 'HA & Replication');
            break;
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
            loadLazyView('EnterpriseMetricsView', 'enterprise-metrics', 'Enterprise Metrics');
            break;
        case 'performance-debt':
            loadLazyView('sqlserver_PerformanceDebtDashboard', 'performance-debt', 'Performance Debt');
            break;
        case 'sqlserver-intelligence-report':
            loadLazyView('SqlServerIntelligenceReportView', 'sqlserver-intelligence-report', 'Intelligence Report');
            break;
        case 'query-analysis':
            loadLazyView('sqlserver_QueryAnalysisDashboard', 'query-analysis', 'Query Analysis');
            break;
        case 'sqlserver-plan-analyzer':
            loadLazyView('SqlServerPlanAnalyzerView', 'sqlserver-plan-analyzer', 'Plan Analyzer');
            break;
        case 'watched-queries':
            loadLazyView('sqlserver_WatchedQueryAnalyzer', 'watched-queries', 'Watched Queries');
            break;
        case 'storage-index-health': {
            const sihInst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
            const runPg = () => {
                loadLazyView('PgStorageIndexHealthView', 'storage-index-health', 'Index & Table Health');
            };
            const runMs = () => {
                loadLazyView('SqlServerStorageIndexHealthView', 'storage-index-health', 'Storage & Index Health');
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
            loadLazyView('SqlserverBestPracticesView', 'best-practices', 'Best Practices');
            break;
        case 'sqlserver-locks':
            loadLazyView('sqlserver_LocksDashboard', 'sqlserver-locks', 'Locks & Blocking');
            break;
        case 'sqlserver-locks-drilldown':
            if (window.sqlserver_LocksDrilldownDetailed) {
                window.sqlserver_LocksDrilldownDetailed(window.appState.routeParams);
            } else {
                loadLazyView('sqlserver_LocksDrilldownDetailed', 'sqlserver-locks-drilldown', 'Deep Dive');
            }
            break;
        // Postgres
        case 'pg-dashboard': {
            const pgInst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
            if (pgInst && pgInst.type === 'sqlserver') {
                window.appNavigate('dashboard');
                return;
            }
            if (typeof window.PgDashboardView === 'function') {
                window.PgDashboardView();
            } else {
                console.error('PgDashboardView not found');
            }
            break;
        }
        case 'pg-sessions': if (typeof window.PgSessionsView === 'function') window.PgSessionsView(); break;
        case 'pg-locks': if (typeof window.PgLocksView === 'function') window.PgLocksView(); break;
        case 'pg-queries': if (typeof window.PgStatStatementsView === 'function') window.PgStatStatementsView(); break;
        case 'pg-explain': if (typeof window.PgExplainView === 'function') window.PgExplainView(); break;
        case 'pg-storage': if (typeof window.PgStorageView === 'function') window.PgStorageView(); break;
        case 'pg-replication': if (typeof window.PgReplicationView === 'function') window.PgReplicationView(); break;
        case 'pg-logs': if (typeof window.PgLogsView === 'function') window.PgLogsView(); break;
        case 'pg-backups': if (typeof window.PgBackupsView === 'function') window.PgBackupsView(); break;
        case 'pg-waits': if (window.PgWaitsView) window.PgWaitsView(); break;
        case 'pg-backup-dr': if (window.PgBackupDRView) window.PgBackupDRView(); break;
        case 'pg-security': if (window.PgSecurityView) window.PgSecurityView(); break;
        case 'pg-alerts': window.PgAlertsView(); break;
        case 'pg-autovacuum': window.PgStorageView(); break;
        // pg-config removed from sidebar (still callable if needed)        case 'pg-config': window.PgConfigView(); break;
        case 'pg-cpu': window.PgCpuView(); break;
        case 'pg-memory': window.PgMemoryView(); break;
        // CNPG is being merged into replication engine; keep route for backward links.
        case 'pg-cnpg': window.CNPGClusterTopologyView(); break;
        case 'pg-stat-statements': if (typeof window.PgStatStatementsView === 'function') window.PgStatStatementsView(); break;
        case 'pg-best-practices':
            loadLazyView('PgBestPracticesView', 'pg-best-practices', 'PostgreSQL Best Practices');
            break;
        case 'pg-dashboard-guide':
            loadLazyView('PgDashboardGuideView', 'pg-dashboard-guide', 'PostgreSQL Dashboard Guide');
            break;
        case 'sqlserver-dashboard-guide':
            loadLazyView('SqlServerDashboardGuideView', 'sqlserver-dashboard-guide', 'SQL Server Dashboard Guide');
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
            if (inst && inst.name && String(inst.name) !== 'undefined') {
                const opt = document.createElement('option');
                opt.value = i;
                opt.textContent = inst.name;
                sel.appendChild(opt);
            }
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

        if (window.appState.currentInstanceIdx === -1 || !window.appState.config || !window.appState.config.instances || !window.appState.config.instances[window.appState.currentInstanceIdx]) {
            dbSel.innerHTML = '<option value="all">-- N/A --</option>';
            brand.className = 'fa-solid fa-earth-americas xl-icon logo-icon text-accent';
            sidebarNav.innerHTML = '<li data-route="global" class="active"><i class="fa-solid fa-globe"></i> Global Estate Overview</li>' + adminLi;
            // Click handling is delegated globally.
            return;
        }

        const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
        const instType = String(inst.type || '').toLowerCase();
        dbSel.innerHTML = '<option value="all">-- Loading Databases --</option>';

        // Fetch databases from server if postgres
        if (instType === 'postgres') {
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
                    const currentDb = window.appState.currentDatabase;
                    dbSel.innerHTML = '<option value="all">-- All Databases --</option>';
                    let dbExists = false;
                    if (data.databases && data.databases.length > 0) {
                        data.databases.forEach(db => {
                            if (db && String(db) !== 'undefined' && String(db) !== 'null') {
                                const opt = document.createElement('option');
                                opt.value = db;
                                opt.textContent = db;
                                dbSel.appendChild(opt);
                                if (db === currentDb) dbExists = true;
                            }
                        });
                        if (dbExists) {
                            dbSel.value = currentDb;
                        } else {
                            window.appState.currentDatabase = data.databases[0];
                            dbSel.value = data.databases[0];
                        }
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
            // For SQL Server / Others, use static list from config
            const currentDb = window.appState.currentDatabase;
            dbSel.innerHTML = '<option value="all">-- All Databases --</option>';
            let dbExists = false;
            if (Array.isArray(inst.databases)) {
                inst.databases.forEach(db => {
                    if (db && String(db) !== 'undefined' && String(db) !== 'null') {
                        const opt = document.createElement('option');
                        opt.value = db;
                        opt.textContent = db;
                        dbSel.appendChild(opt);
                        if (db === currentDb) dbExists = true;
                    }
                });
            }
            if (dbExists) {
                dbSel.value = currentDb;
            } else if (inst.databases && inst.databases.length > 0) {
                window.appState.currentDatabase = inst.databases[0];
                dbSel.value = inst.databases[0];
            } else {
                window.appState.currentDatabase = 'all';
                dbSel.value = 'all';
            }
        }
        
        if (instType === 'postgres') { 
            brand.className='fa-solid fa-database xl-icon logo-icon text-accent'; 
            sidebarNav.innerHTML = `
                <li data-route="pg-dashboard" id="nav-pg-dashboard"><i class="fa-solid fa-gauge-high"></i> Control Center</li>
                <li data-route="drilldown-pg-enterprise"><i class="fa-solid fa-chart-line"></i> Enterprise Monitor</li>
                <li data-route="pg-cpu" class="sub-nav"><i class="fa-solid fa-microchip"></i> CPU Usage</li>
                <li data-route="pg-memory" class="sub-nav"><i class="fa-solid fa-memory"></i> Memory Usage</li>
                <li data-route="pg-waits"><i class="fa-solid fa-clock-rotate-left"></i> Waits & Sessions</li>
                <li data-route="pg-locks"><i class="fa-solid fa-link-slash"></i> Locks & Blocking</li>
                <li data-route="pg-queries"><i class="fa-solid fa-bolt"></i> Query Performance</li>
                <li data-route="pg-explain"><i class="fa-solid fa-diagram-project"></i> EXPLAIN Analyzer</li>
                <li data-route="pg-storage"><i class="fa-solid fa-hard-drive"></i> Storage & Vacuum</li>
                <li data-route="storage-index-health"><i class="fa-solid fa-boxes-stacked"></i> Index and Table</li>
                <li data-route="pg-replication" id="nav-pg-replication" style="display:none;"><i class="fa-solid fa-clone"></i> Replication & HA</li>
                <li data-route="pg-backups"><i class="fa-solid fa-shield-heart"></i> Backup & DR</li>
                <li data-route="pg-security"><i class="fa-solid fa-user-lock"></i> Security Monitor</li>
                <li data-route="pg-best-practices"><i class="fa-solid fa-shield-halved"></i> Best Practices</li>
                <li data-route="pg-alerts"><i class="fa-solid fa-bell text-danger"></i> Alerts & Events</li>
                <li data-route="pg-dashboard-guide" style="border-top:1px solid var(--border-color);margin-top:6px;padding-top:6px;"><i class="fa-solid fa-book-open text-accent"></i> Dashboard Info</li>
                ${adminLi}
            `;
        } else { 
            // Default to SQL Server sidebar for anything else
            brand.className='fa-brands fa-microsoft xl-icon logo-icon text-accent'; 
            sidebarNav.innerHTML = `
                <li data-route="sqlserver-health-v2" id="nav-dashboard"><i class="fa-solid fa-gauge-high"></i> Instance Dashboard</li>
                <li data-route="sqlserver-workload"><i class="fa-solid fa-chart-area"></i> Workload Analytics</li>
                <li data-route="sqlserver-waits"><i class="fa-solid fa-clock-rotate-left"></i> Wait Statistics</li>
                <li data-route="drilldown-memory"><i class="fa-solid fa-memory"></i> Memory Analyzer</li>
                <li data-route="sqlserver-locks"><i class="fa-solid fa-link-slash"></i> Locks & Blocking</li>
                <li data-route="drilldown-ha" style="display:none;"><i class="fa-solid fa-server"></i> HA & Replication</li>
                <li data-route="enterprise-metrics"><i class="fa-solid fa-chart-line"></i> Enterprise Metrics</li>
                <li data-route="storage-index-health"><i class="fa-solid fa-boxes-stacked"></i> Index and Table</li>
                <li data-route="performance-debt"><i class="fa-solid fa-screwdriver-wrench"></i> Performance Debt</li>
                <li data-route="sqlserver-intelligence-report"><i class="fa-solid fa-brain"></i> Intelligence Report</li>
                <li data-route="query-analysis"><i class="fa-solid fa-magnifying-glass-chart"></i> Query Analysis</li>
                <li data-route="sqlserver-plan-analyzer"><i class="fa-solid fa-diagram-project"></i> Plan Analyzer</li>
                <li data-route="watched-queries"><i class="fa-solid fa-binoculars"></i> Watched Queries</li>
                <li data-route="jobs" id="nav-agent-jobs"><i class="fa-solid fa-briefcase"></i> SQL Agent Jobs</li>
                <li data-route="alerts"><i class="fa-solid fa-triangle-exclamation"></i> Alerts <span id="alerts-badge" class="badge badge-danger">0</span></li>
                <li data-route="best-practices"><i class="fa-solid fa-shield-halved"></i> Best Practices</li>
                <li data-route="sqlserver-dashboard-guide" style="border-top:1px solid var(--border-color);margin-top:6px;padding-top:6px;"><i class="fa-solid fa-book-open text-accent"></i> Dashboard Info</li>
                ${adminLi}
            `;
        }

        // Apply active class to sidebar item matching current route
        const activeRoute = window.appState.activeViewId;
        const allLis = sidebarNav.querySelectorAll('li[data-route]');
        allLis.forEach(li => {
            if (li.getAttribute('data-route') === activeRoute) {
                li.classList.add('active');
            } else {
                li.classList.remove('active');
            }
        });

        // Dynamic sidebar link visibility (Replication/HA).
        // On instance selection always use instance-level detection (no database param)
        // so the link appears if the instance has any HA or replication regardless of
        // which database happens to be selected first.
        if (window.appState.currentInstanceIdx !== -1 && window.appState.config) {
            const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
            _refreshHALinkVisibility(inst, '');
        }
    }
};

// _refreshHALinkVisibility shows or hides the HA/Replication sidebar link based on feature detection.
// For SQL Server, uses instance-level detection so the link remains visible even if 
// a non-HA database (like master) is currently selected.
// Safe to call any time the instance or database selection changes.
async function _refreshHALinkVisibility(inst, database) {
    if (!inst) return;
    try {
        const type = String(inst.type || '').toLowerCase();
        if (type === 'postgres') {
            const r = await window.apiClient.authenticatedFetch(
                `/api/postgres/replication?instance=${encodeURIComponent(inst.name)}`
            );
            if (r.ok) {
                const d = await r.json();
                const s = d.stats || {};
                const hasRepl = s.is_primary === false || (Array.isArray(s.standbys) && s.standbys.length > 0);
                const li = document.querySelector('li[data-route="pg-replication"]');
                if (li) li.style.display = hasRepl ? 'block' : 'none';
            }
        } else if (type === 'sqlserver') {
            // Sidebar visibility is instance-wide. We don't pass the database param here
            // because the HA/Replication dashboard covers the entire instance. This
            // prevents the link from disappearing when a non-HA database (like master) is selected.
            const r = await window.apiClient.authenticatedFetch(
                `/api/sqlserver/ha/feature-status?instance=${encodeURIComponent(inst.name)}`
            );
            if (r.ok) {
                const d = await r.json();
                const hasHA = d.ha_enabled === true || d.replication_enabled === true;
                const li = document.querySelector('li[data-route="drilldown-ha"]');
                if (li) li.style.display = hasHA ? 'block' : 'none';
            }
        }
    } catch (e) {
        console.warn('[Router] HA/Repl link visibility check failed', e);
    }
}

// Event Listeners strictly mounting globally mapped nodes
document.getElementById('instance-select').addEventListener('change', (e) => {
    let idx = parseInt(e.target.value);
    if (isNaN(idx)) idx = -1;
    window.appState.currentInstanceIdx = idx;

    const inst = window.appState.config && window.appState.config.instances ? window.appState.config.instances[idx] : null;
    if (!inst) {
        window.appState.currentInstanceIdx = -1;
        window.appNavigate('global');
        return;
    }

    // Set current instance name for modules that rely on window.state or similar
    if (window.state) window.state.currentInstance = inst.name;

    window.router.populateDatabaseDropdown();

    // Always navigate to the primary dashboard for the selected engine
    const root = inst.type === 'postgres' ? 'pg-dashboard' : 'sqlserver-health-v2';
    window.appNavigate(root);
});

document.getElementById('database-select').addEventListener('change', (e) => {
    const selectedDb = e.target.value;
    window.appState.currentDatabase = selectedDb;

    const inst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
    if (!inst) return;

    // Re-check HA/Replication link visibility with the new database selection.
    _refreshHALinkVisibility(inst, selectedDb);

    // Always navigate to the engine's default dashboard so the user sees
    // instance-level data for the newly selected database context.
    const defaultRoute = inst.type === 'postgres' ? 'pg-dashboard' : 'sqlserver-health-v2';
    window.appNavigate(defaultRoute);
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
