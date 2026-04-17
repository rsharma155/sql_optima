/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: HTTP client implementation for communicating with backend API endpoints. Handles authentication, token management, and authenticated fetch requests.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

// js/api/client.js - Application context and asynchronous fetch handler
window.appState = {
    config: null,
    currentInstanceIdx: 0,
    currentDatabase: 'all',
    activeViewId: 'global',
    isAuthenticated: false,
    queryCache: {}
};

window.apiClient = {
    /** Read the csrf_token cookie (not HttpOnly). */
    _csrfToken() {
        const m = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
        return m ? decodeURIComponent(m[1]) : '';
    },

    // Token management — JWT is now in an HttpOnly cookie.
    getToken() {
        return null; // legacy compat — cookie handles transport
    },

    setToken(_token) {
        window.appState.isAuthenticated = true;
    },

    clearToken() {
        window.appState.isAuthenticated = false;
    },

    // Authenticated fetch wrapper — cookie sent automatically; add CSRF header.
    async authenticatedFetch(url, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers
        };

        const csrf = this._csrfToken();
        if (csrf) {
            headers['X-CSRF-Token'] = csrf;
        }

        const response = await fetch(url, { ...options, headers, credentials: 'same-origin' });

        if (response.status === 401) {
            // Token expired or invalid
            this.clearToken();
            if (window._auth) {
                window._auth.token = null;
                window._auth.user = null;
                localStorage.removeItem('auth_user');
            }
            this.showLoginModal();
            throw new Error('Authentication required');
        }

        return response;
    },

    async fetchConfig() {
        try {
            const response = await fetch('/api/config');
            if (response.ok) {
                const cfg = await response.json();
                window.appState.config = cfg;
                return true;
            } else {
                console.error("Config fetch rejected with status: ", response.status);
            }
        } catch (e) {
            console.error("API Server unreached or config loading failed.", e);
        }

        // Fatal application halt, mock arrays permanently deleted.
        window.appState.config = { instances: [] };
        return false;
    },

    async login(username, password) {
        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                credentials: 'same-origin',
                body: JSON.stringify({ username, password })
            });

            if (response.ok) {
                const data = await response.json();
                // JWT is stored in HttpOnly cookie by the server.
                this.setToken(null);
                return { success: true };
            } else {
                const error = await response.json();
                return { success: false, error: error.error || 'Login failed' };
            }
        } catch (e) {
            console.error("Login request failed:", e);
            return { success: false, error: 'Network error' };
        }
    },

    showLoginModal() {
        const modal = document.getElementById('login-modal');
        const form = document.getElementById('login-form');
        const errorDiv = document.getElementById('login-error');

        modal.style.display = 'flex';
        errorDiv.style.display = 'none';

        form.addEventListener('submit', async (e) => {
            e.preventDefault();

            const submitBtn = form.querySelector('button[type="submit"]');
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;

            submitBtn.disabled = true;
            submitBtn.textContent = 'Logging in...';

            const result = await this.login(username, password);

            if (result.success) {
                modal.style.display = 'none';
                // Retry fetching config now that we're authenticated
                window.boot();
            } else {
                errorDiv.textContent = result.error;
                errorDiv.style.display = 'block';
                submitBtn.disabled = false;
                submitBtn.textContent = 'Login';
            }
        });
    }
};

window.setDashboardRefresh = function(val) {
    const rate = parseInt(val, 10);
    window.appState.dashboardRate = rate;
    if (window.dashboardRefreshInterval) {
        clearInterval(window.dashboardRefreshInterval);
        window.dashboardRefreshInterval = null;
    }
    if (rate > 0) {
        window.dashboardRefreshInterval = setInterval(() => {
            if (window.appState.activeViewId === 'dashboard' && window.refreshDashboardData) {
                window.refreshDashboardData();
            }
        }, rate);
    }
}

window.setJobsRefresh = function(val) {
    const rate = parseInt(val, 10);
    window.appState.jobsRate = rate;
    if (window.jobsRefreshInterval) {
        clearInterval(window.jobsRefreshInterval);
    }
    if (rate > 0) {
        window.jobsRefreshInterval = setInterval(() => {
            if (window.appState.activeViewId === 'jobs' && window.JobsView) {
                window.JobsView();
            }
        }, rate);
    }
}

window.showQueryModal = function(queryText) {
    const existingModal = document.getElementById('query-modal');
    if (existingModal) existingModal.remove();

    try {
        queryText = decodeURIComponent(queryText);
    } catch(e) {}

    const modal = document.createElement('div');
    modal.id = 'query-modal';
    modal.style.cssText = 'display: flex; position: fixed; z-index: 99999; left: 0; top: 0; width: 100%; height: 100%; background-color: rgba(0,0,0,0.8); align-items: center; justify-content: center;';
    
    const safeText = escapeHtml(queryText);
    
    modal.innerHTML = `
        <div style="background: var(--bg-secondary, #1a1a1a); margin: 2%; padding: 20px; border: 1px solid var(--border-color, #333); border-radius: 12px; width: 95%; max-width: 1000px; max-height: 90vh; overflow-y: auto; color: var(--text-primary, #e0e0e0); font-family: inherit; box-shadow: 0 4px 20px rgba(0,0,0,0.5);">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; border-bottom: 1px solid var(--border-color, #333); padding-bottom: 0.75rem;">
                <h3 style="margin: 0; color: var(--accent, #3b82f6); font-size: 1.1rem;"><i class="fa-solid fa-code"></i> query text details</h3>
                <button data-action="close-id" data-target="query-modal" style="background: transparent; border: 1px solid var(--border-color, #555); color: var(--text-primary, #e0e0e0); font-size: 1.25rem; cursor: pointer; padding: 0.25rem 0.6rem; border-radius: 4px; line-height: 1;">&times;</button>
            </div>
            <div style="background: var(--bg-primary, #141414); padding: 1rem; border-radius: 8px; max-height: 60vh; overflow: auto; border: 1px solid var(--border-color, #333);">
                <pre style="margin: 0; white-space: pre-wrap; word-wrap: break-word; color: var(--text-primary, #e0e0e0); font-family: 'Courier New', monospace; font-size: 0.85rem; line-height: 1.5;">${safeText}</pre>
            </div>
            <div style="text-align: center; margin-top: 1rem;">
                <button id="copySqlBtn" style="background: var(--accent, #3b82f6); color: #fff; border: none; padding: 0.5rem 1.5rem; border-radius: 6px; cursor: pointer; font-size: 0.9rem;">
                    <i class="fa-solid fa-copy"></i> copy SQL
                </button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
    
    document.getElementById('copySqlBtn').addEventListener('click', function() {
        navigator.clipboard.writeText(queryText).then(() => {
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

window.showQueryModalFromData = function(queryText) {
    const existingModal = document.getElementById('query-modal');
    if (existingModal) existingModal.remove();

    const modal = document.createElement('div');
    modal.id = 'query-modal';
    modal.style.cssText = 'display: flex; position: fixed; z-index: 99999; left: 0; top: 0; width: 100%; height: 100%; background-color: rgba(0,0,0,0.8); align-items: center; justify-content: center;';
    
    const safeText = escapeHtml(queryText);
    
    modal.innerHTML = `
        <div style="background: var(--bg-secondary, #1a1a1a); margin: 2%; padding: 20px; border: 1px solid var(--border-color, #333); border-radius: 12px; width: 95%; max-width: 1000px; max-height: 90vh; overflow-y: auto; color: var(--text-primary, #e0e0e0); font-family: inherit; box-shadow: 0 4px 20px rgba(0,0,0,0.5);">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; border-bottom: 1px solid var(--border-color, #333); padding-bottom: 0.75rem;">
                <h3 style="margin: 0; color: var(--accent, #3b82f6); font-size: 1.1rem;"><i class="fa-solid fa-code"></i> query text details</h3>
                <button data-action="close-id" data-target="query-modal" style="background: transparent; border: 1px solid var(--border-color, #555); color: var(--text-primary, #e0e0e0); font-size: 1.25rem; cursor: pointer; padding: 0.25rem 0.6rem; border-radius: 4px; line-height: 1;">&times;</button>
            </div>
            <div style="background: var(--bg-primary, #141414); padding: 1rem; border-radius: 8px; max-height: 60vh; overflow: auto; border: 1px solid var(--border-color, #333);">
                <pre style="margin: 0; white-space: pre-wrap; word-wrap: break-word; color: var(--text-primary, #e0e0e0); font-family: 'Courier New', monospace; font-size: 0.85rem; line-height: 1.5;">${safeText}</pre>
            </div>
            <div style="text-align: center; margin-top: 1rem;">
                <button id="copySqlBtn2" style="background: var(--accent, #3b82f6); color: #fff; border: none; padding: 0.5rem 1.5rem; border-radius: 6px; cursor: pointer; font-size: 0.9rem;">
                    <i class="fa-solid fa-copy"></i> copy SQL
                </button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
    
    document.getElementById('copySqlBtn2').addEventListener('click', function() {
        navigator.clipboard.writeText(queryText).then(() => {
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

function escapeHtml(str) {
    if (str === null || str === undefined) return '';
    return String(str).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
}

// Boot is handled by entry.js (ES module) - this file only provides the API client
