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

    async fetchAuthStatus() {
        try {
            const response = await fetch('/api/auth/status');
            if (!response.ok) return;
            const data = await response.json();
            appState.authRequired = !!data.auth_required;
            appState.authMode = data.auth_mode || 'local';
        } catch (e) {
            console.warn('[auth] /api/auth/status failed', e);
            appState.authRequired = false;
        }
    },

    async authenticatedFetch(url, options = {}) {
        const token = this.getToken();
        if (appState.authRequired && !token) {
            this.showLoginModal();
            throw new Error('No authentication token available');
        }

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
            if (appState.authRequired) {
                this.showLoginModal();
            }
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
                body: JSON.stringify({ username, password })
            });

            if (response.ok) {
                const data = await response.json();
                this.setToken(data.token);
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

    console.log('[BOOT] Config loaded with', appState.config.instances?.length || 0, 'instances');

    appState.config.instances.forEach((inst) => {
        if (!inst.user) inst.user = 'dbsqlmonitor';
        if (!inst.databases || inst.databases.length === 0) inst.databases = [];
    });

    if (!window.router) {
        console.error('[BOOT] window.router not available');
        return;
    }

    window.router.populateInstanceDropdown();
    window.appNavigate('global');
    console.log('[BOOT] Boot sequence complete');
}
