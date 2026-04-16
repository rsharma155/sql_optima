/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Application client boot loader and initialization with API client setup and polling management.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

// Authenticated API client and boot (ES module).
import { appState } from './app-state.js';

export const apiClient = {
    /** Read the csrf_token cookie (not HttpOnly). */
    _csrfToken() {
        const m = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
        return m ? decodeURIComponent(m[1]) : '';
    },

    getToken() {
        // Legacy compatibility — returns null. JWT is now in HttpOnly cookie.
        return null;
    },

    setToken(_token) {
        // JWT is stored in an HttpOnly cookie by the backend.
        appState.isAuthenticated = true;
    },

    clearToken() {
        appState.isAuthenticated = false;
    },

    async fetchAuthStatus() {
        try {
            const response = await fetch('/api/auth/status');
            if (!response.ok) return;
            const data = await response.json();
            appState.authRequired = !!data.auth_required;
            appState.authMode = data.auth_mode || 'local';
            if (data.deployment === 'docker' || data.deployment === 'dedicated') {
                appState.deployment = data.deployment;
            }
        } catch (e) {
            console.warn('[auth] /api/auth/status failed', e);
            appState.authRequired = false;
        }
    },

    async authenticatedFetch(url, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers
        };

        // Attach CSRF token for all requests (server only checks on mutating methods).
        const csrf = this._csrfToken();
        if (csrf) {
            headers['X-CSRF-Token'] = csrf;
        }

        const response = await fetch(url, { ...options, headers, credentials: 'same-origin' });

        if (response.status === 401) {
            this.clearToken();
            // Clear in-memory auth state so stale "Signed in as" displays are removed.
            if (window._auth) {
                window._auth.token = null;
                window._auth.user = null;
                localStorage.removeItem('auth_user');
            }
            // Always prompt login on 401 — admin routes require auth regardless of AUTH_REQUIRED setting.
            this.showLoginModal();
            throw new Error('Authentication required');
        }

        return response;
    },

    async fetchConfig() {
        try {
            const url = '/api/config';
            const response = appState.authRequired
                ? await this.authenticatedFetch(url)
                : await fetch(url);
            if (response.ok) {
                const cfg = await response.json();
                appState.config = cfg;
                return true;
            }
            console.error('Config fetch rejected with status:', response.status);
        } catch (e) {
            if (e.message !== 'No authentication token available') {
                console.error('API Server unreached or config loading failed.', e);
            }
        }

        appState.config = { instances: [] };
        return false;
    },

    async login(username, password) {
        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ username, password })
            });

            if (response.ok) {
                const data = await response.json();
                // JWT is in HttpOnly cookie — just mark state as authenticated.
                this.setToken(null);
                return { success: true };
            }
            const error = await response.json().catch(() => ({}));
            return { success: false, error: error.error || 'Login failed' };
        } catch (e) {
            console.error('Login request failed:', e);
            return { success: false, error: 'Network error' };
        }
    },

    showLoginModal() {
        const modal = document.getElementById('login-modal');
        const form = document.getElementById('login-form');
        const errorDiv = document.getElementById('login-error');
        if (!modal || !form) return;

        modal.style.display = 'flex';
        if (errorDiv) errorDiv.style.display = 'none';

        const submitHandler = async (e) => {
            e.preventDefault();
            const submitBtn = form.querySelector('button[type="submit"]');
            const username = document.getElementById('username')?.value;
            const password = document.getElementById('password')?.value;
            if (submitBtn) {
                submitBtn.disabled = true;
                submitBtn.textContent = 'Logging in...';
            }

            const result = await this.login(username, password);

            if (result.success) {
                modal.style.display = 'none';
                form.removeEventListener('submit', submitHandler);
                window.boot();
            } else {
                if (errorDiv) {
                    errorDiv.textContent = result.error;
                    errorDiv.style.display = 'block';
                }
                if (submitBtn) {
                    submitBtn.disabled = false;
                    submitBtn.textContent = 'Login';
                }
            }
        };

        form.addEventListener('submit', submitHandler);
    },

    async apiFetch(url, options = {}) {
        if (appState.authRequired) {
            return this.authenticatedFetch(url, options);
        }
        return fetch(url, options);
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
            if (appState.activeViewId === 'jobs' && window.refreshJobsData) {
                window.refreshJobsData();
            } else if (appState.activeViewId === 'jobs' && window.JobsView) {
                window.JobsView();
            }
        }, rate);
    }
}

export function showQueryModal(queryText) {
    alert('Full Query Target Trace:\n\n' + queryText);
}

export async function boot() {
    console.log('[BOOT] Starting application boot sequence...');

    await apiClient.fetchAuthStatus();
    console.log('[BOOT] auth_required=', appState.authRequired);

    let st = {};
    try {
        const sr = await fetch('/api/setup/status');
        if (sr.ok) {
            st = await sr.json();
            if (st.deployment === 'docker' || st.deployment === 'dedicated') {
                appState.deployment = st.deployment;
            }
        }
    } catch (e) {
        console.warn('[BOOT] /api/setup/status', e);
    }

    const docker = appState.deployment === 'docker' || !!st.docker_mode;
    const pathEarly = (window.location && window.location.pathname) ? window.location.pathname : '/';

    // Dedicated: Timescale + admin wizard (browser) before anything else.
    if (!docker && !st.public_setup_disabled) {
        if (st.needs_timescale || st.needs_bootstrap_admin) {
            if (pathEarly === '/setup-schema') {
                window.appNavigate('setup-schema');
            } else {
                window.appNavigate('setup');
            }
            return;
        }
    }

    // Docker: Timescale from compose — only wizard for admin bootstrap or compose error (handled inside /setup view).
    if (docker && !st.public_setup_disabled) {
        if (!st.timescale_connected || st.needs_bootstrap_admin) {
            if (pathEarly === '/setup-schema') {
                window.appNavigate('setup-schema');
            } else {
                window.appNavigate('setup');
            }
            return;
        }
    }

    if (appState.authRequired && !apiClient.getToken()) {
        console.log('[BOOT] Login required before loading config');
        apiClient.showLoginModal();
        return;
    }

    const configLoaded = await apiClient.fetchConfig();
    if (!configLoaded) {
        console.error('[BOOT] Config loading failed');
        return;
    }

    let st2 = st;
    try {
        const sr2 = await fetch('/api/setup/status');
        if (sr2.ok) st2 = await sr2.json();
    } catch (e) { /* ignore */ }

    const instanceCount = appState.config.instances?.length || 0;
    console.log('[BOOT] Config loaded with', instanceCount, 'instances');

    appState.config.instances.forEach((inst) => {
        if (!inst.user) inst.user = 'dbsqlmonitor';
        if (!inst.databases || inst.databases.length === 0) inst.databases = [];
    });

    if (!window.router) {
        console.error('[BOOT] window.router not available');
        return;
    }

    window.router.populateInstanceDropdown();

    const isAdmin = !!(window._auth && window._auth.isLoggedIn && window._auth.isLoggedIn() && window._auth.isAdmin && window._auth.isAdmin());

    // After Timescale + users exist: full-page onboarding until at least one active monitoring server is registered.
    if (st2.needs_onboarding_servers && isAdmin) {
        window.appNavigate('onboarding-servers');
        console.log('[BOOT] Routed to onboarding (no active monitoring servers in registry)');
        return;
    }

    if (instanceCount === 0) {
        if (!isAdmin) {
            window.appNavigate('login');
        } else {
            window.appNavigate('onboarding-servers');
        }
        console.log('[BOOT] No instances in config; routed to login or onboarding');
        return;
    }

    const path = (window.location && window.location.pathname) ? window.location.pathname : '/';
    if (path === '/admin') {
        window.appNavigate('admin');
    } else if (path === '/login') {
        window.appNavigate('login');
    } else if (path === '/setup') {
        window.appNavigate('setup');
    } else if (path === '/setup-schema') {
        window.appNavigate('setup-schema');
    } else if (path === '/onboarding-servers') {
        window.appNavigate('onboarding-servers');
    } else {
        window.appNavigate('global');
    }
    console.log('[BOOT] Boot sequence complete');
}
