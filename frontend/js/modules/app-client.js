// Authenticated API client and boot (ES module).
import { appState } from './app-state.js';

export const apiClient = {
    getToken() {
        return localStorage.getItem('auth_token');
    },

    setToken(token) {
        localStorage.setItem('auth_token', token);
        appState.isAuthenticated = true;
    },

    clearToken() {
        localStorage.removeItem('auth_token');
        appState.isAuthenticated = false;
    },

    async authenticatedFetch(url, options = {}) {
        const token = this.getToken();
        
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers
        };
        
        if (token) {
            headers['Authorization'] = `Bearer ${token}`;
        }

        const response = await fetch(url, { ...options, headers });

        if (response.status === 401) {
            this.clearToken();
            throw new Error('Authentication required');
        }

        return response;
    },

    async fetchConfig() {
        try {
            const response = await fetch('/api/config');
            if (response.ok) {
                const cfg = await response.json();
                appState.config = cfg;
                return true;
            } else {
                console.error("Config fetch rejected with status: ", response.status);
            }
        } catch (e) {
            console.error("API Server unreached or config loading failed.", e);
        }

        appState.config = { instances: [] };
        return false;
    }
};

export function setDashboardRefresh(val) {
    const rate = parseInt(val, 10);
    appState.dashboardRate = rate;
    if (window.dashboardRefreshInterval) {
        clearInterval(window.dashboardRefreshInterval);
    }
    if (rate > 0) {
        window.dashboardRefreshInterval = setInterval(() => {
            // Don't re-render the whole view; just refresh data.
            if (appState.activeViewId === 'dashboard' && window.refreshDashboardData) {
                window.refreshDashboardData();
            }
        }, rate);
    }
}

export function setJobsRefresh(val) {
    const rate = parseInt(val, 10);
    appState.jobsRate = rate;
    if (window.jobsRefreshInterval) {
        clearInterval(window.jobsRefreshInterval);
    }
    if (rate > 0) {
        window.jobsRefreshInterval = setInterval(() => {
            // Prefer a lightweight refresh hook if present.
            if (appState.activeViewId === 'jobs' && window.refreshJobsData) {
                window.refreshJobsData();
            } else if (appState.activeViewId === 'jobs' && window.JobsView) {
                window.JobsView();
            }
        }, rate);
    }
}

export function showQueryModal(queryText) {
    alert("Full Query Target Trace:\n\n" + queryText);
}

export async function boot() {
    console.log("[BOOT] Starting application boot sequence...");

    console.log("[BOOT] Fetching config from /api/config...");
    const configLoaded = await apiClient.fetchConfig();

    if (!configLoaded) {
        console.error("[BOOT] Config loading failed");
        return;
    }

    console.log("[BOOT] Config loaded with", appState.config.instances?.length || 0, "instances");

    appState.config.instances.forEach(inst => {
        if (!inst.user) inst.user = "dbsqlmonitor";
        if (!inst.databases || inst.databases.length === 0) inst.databases = [];
    });

    if (!window.router) {
        console.error("[BOOT] window.router not available");
        return;
    }

    console.log("[BOOT] Populating instance dropdown");
    window.router.populateInstanceDropdown();
    console.log("[BOOT] Navigating to global view");
    window.appNavigate('global');
    console.log("[BOOT] Boot sequence complete");
}
