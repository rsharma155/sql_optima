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
import { AuthManager } from './auth-manager.js';
import { loadTemplate } from './ui-manager.js';
import { decodeQueryText } from '../utils/query-text.js';

export const apiClient = {
    /** Read the csrf_token cookie (not HttpOnly). */
    _csrfToken() {
        return AuthManager.getCsrfToken();
    },

    getToken() {
        return null;
    },

    setToken(token) {
        appState.isAuthenticated = true;
        if (token && window._auth) {
            window._auth.token = token;
        }
    },

    clearToken() {
        AuthManager.clear();
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
            // Sync AuthManager if backend says we are not logged in but we have local state
            if (appState.authRequired && response.status === 401) {
                AuthManager.clear();
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

        // Send Authorization header using stored token (covers the setup/bootstrap flow
        // where the HttpOnly cookie may not yet be visible to the browser).
        const token = (window._auth && window._auth.token) || localStorage.getItem('auth_token');
        if (token && !headers['Authorization']) {
            headers['Authorization'] = 'Bearer ' + token;
        }

        const csrf = this._csrfToken();
        if (csrf) {
            headers['X-CSRF-Token'] = csrf;
        }

        const response = await fetch(url, { ...options, headers, credentials: 'same-origin' });

        if (response.status === 401) {
            this.clearToken();
            this.showLoginModal();
            throw new Error('Authentication required');
        }

        return response;
    },

    /** Download a binary attachment (zip, etc.) with auth headers. */
    async downloadAuthenticatedBlob(url, fallbackFilename = 'download.zip') {
        const response = appState.authRequired
            ? await this.authenticatedFetch(url, { method: 'GET', headers: {} })
            : await fetch(url, { credentials: 'same-origin' });
        if (!response.ok) {
            const text = await response.text().catch(() => '');
            throw new Error(text || `Download failed (${response.status})`);
        }
        const blob = await response.blob();
        let filename = fallbackFilename;
        const disp = response.headers.get('Content-Disposition');
        if (disp) {
            const m = /filename="?([^";\n]+)"?/.exec(disp);
            if (m) filename = m[1];
        }
        const objectUrl = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = objectUrl;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(objectUrl);
    },

    /** Download a CSV (or other attachment) with auth headers; avoids window.location bypassing Bearer token. */
    async downloadAuthenticatedCSV(url, fallbackFilename = 'export.csv') {
        const response = appState.authRequired
            ? await this.authenticatedFetch(url, { method: 'GET', headers: {} })
            : await fetch(url, { credentials: 'same-origin' });
        if (!response.ok) {
            const text = await response.text().catch(() => '');
            throw new Error(text || `Export failed (${response.status})`);
        }
        const blob = await response.blob();
        let filename = fallbackFilename;
        const disp = response.headers.get('Content-Disposition');
        if (disp) {
            const m = /filename="?([^";\n]+)"?/.exec(disp);
            if (m) filename = m[1];
        }
        const objectUrl = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = objectUrl;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(objectUrl);
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
        } catch (e) {
            console.error('Config fetch failed:', e);
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
                AuthManager.setUser({ user_id: data.user_id, username: data.username, role: data.role });
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
        if (!modal || !form) {
            // No modal element in the DOM — fall back to full-page LoginView.
            if (typeof window.LoginView === 'function') {
                window.LoginView();
            }
            return;
        }

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
    if (!queryText) {
        queryText = 'No query text available';
    } else {
        queryText = decodeQueryText(queryText);
    }
    
    const existingModal = document.getElementById('query-modal');
    if (existingModal) existingModal.remove();

    const modal = document.createElement('div');
    modal.id = 'query-modal';
    modal.style.cssText = 'display: flex; position: fixed; z-index: 99999; left: 0; top: 0; width: 100%; height: 100%; background-color: rgba(0,0,0,0.8); align-items: center; justify-content: center;';
    
    const safeText = window.escapeHtml ? window.escapeHtml(queryText) : queryText;
    
    modal.innerHTML = `
        <div style="background: var(--bg-surface, #1a1a2e); margin: 2%; padding: 20px; border: 1px solid var(--border-color, #333); border-radius: 12px; width: 95%; max-width: 1000px; max-height: 90vh; overflow-y: auto; color: var(--text-primary, #e0e0e0); font-family: inherit; box-shadow: 0 4px 20px rgba(0,0,0,0.5);">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; border-bottom: 1px solid var(--border-color, #333); padding-bottom: 0.75rem;">
                <h3 style="margin: 0; color: var(--accent, #3b82f6); font-size: 1.1rem;"><i class="fa-solid fa-code"></i> Query Details</h3>
                <button id="close-query-modal" style="background: transparent; border: 1px solid var(--border-color, #555); color: var(--text-primary, #e0e0e0); font-size: 1.25rem; cursor: pointer; padding: 0.25rem 0.6rem; border-radius: 4px; line-height: 1;">&times;</button>
            </div>
            <div style="background: var(--bg-base, #0f0f1a); padding: 1rem; border-radius: 8px; max-height: 60vh; overflow: auto; border: 1px solid var(--border-color, #333);">
                <pre id="queryModalText" style="margin: 0; white-space: pre-wrap; word-wrap: break-word; color: var(--text-primary, #e0e0e0); font-family: 'Courier New', monospace; font-size: 0.85rem; line-height: 1.5;"></pre>
            </div>
            <div style="text-align: center; margin-top: 1rem;">
                <button id="copySqlBtn" style="background: var(--accent, #3b82f6); color: #fff; border: none; padding: 0.5rem 1.5rem; border-radius: 6px; cursor: pointer; font-size: 0.9rem;">
                    <i class="fa-solid fa-copy"></i> copy SQL
                </button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
    
    document.getElementById('queryModalText').textContent = queryText;
    
    document.getElementById('close-query-modal').addEventListener('click', () => modal.remove());
    
    document.getElementById('copySqlBtn').addEventListener('click', function() {
        const textToCopy = document.getElementById('queryModalText').textContent;
        navigator.clipboard.writeText(textToCopy).then(() => {
            this.innerHTML = '<i class="fa-solid fa-check"></i> copied!';
            setTimeout(() => {
                this.innerHTML = '<i class="fa-solid fa-copy"></i> copy SQL';
            }, 1500);
        });
    });

    modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.remove();
    });
}

export async function boot() {
    console.log('[BOOT] Starting application boot sequence...');

    // Global event delegation for close buttons (CSP compliance)
    document.addEventListener('click', (e) => {
        const btnClose = e.target.closest('.btn-close');
        if (btnClose && btnClose.parentElement) {
            btnClose.parentElement.style.display = 'none';
        }
    });

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

    const isLoggedIn = !!(window._auth && typeof window._auth.isLoggedIn === 'function' && window._auth.isLoggedIn());
    if (appState.authRequired && !isLoggedIn) {
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
        const monitoringUser = (inst.monitoring_user || inst.user || '').trim();
        if (monitoringUser) {
            inst.monitoring_user = monitoringUser;
            inst.user = monitoringUser;
        } else {
            inst.user = 'dbsqlmonitor';
            inst.monitoring_user = inst.user;
        }
        if (!inst.databases || inst.databases.length === 0) inst.databases = [];
    });

    if (!window.router) {
        console.error('[BOOT] window.router not available');
        return;
    }

    // Load dynamic intervals for dashboard control
    if (window.collectorConfig) {
        await window.collectorConfig.loadIntervals();
    }

    window.router.populateInstanceDropdown();

    const isAdmin = !!(window._auth && window._auth.isLoggedIn && window._auth.isLoggedIn() && window._auth.isAdmin && window._auth.isAdmin());

    // Route to onboarding only when there are genuinely no registered servers.
    // Checking both the DB status flag AND instanceCount avoids a race where
    // RegistryReload hasn't propagated yet but instances are already in config.
    if (instanceCount === 0) {
        if (st2.needs_onboarding_servers && isAdmin) {
            window.appNavigate('onboarding-servers');
            console.log('[BOOT] Routed to onboarding (no active monitoring servers in registry)');
        } else if (!isAdmin) {
            window.appNavigate('login');
        } else {
            window.appNavigate('onboarding-servers');
        }
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
