/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Auth Manager for handling user authentication, session state, and permissions.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

import { appState } from './app-state.js';

export const AuthManager = {
    user: JSON.parse(localStorage.getItem('auth_user') || 'null'),
    token: localStorage.getItem('auth_token') || null,

    isLoggedIn() {
        return !!this.user;
    },

    isAdmin() {
        return this.user && this.user.role === 'admin';
    },

    getUser() {
        return this.user;
    },

    setUser(userData, token) {
        this.user = userData;
        this.token = token || null;
        if (userData) {
            localStorage.setItem('auth_user', JSON.stringify(userData));
            appState.isAuthenticated = true;
        } else {
            localStorage.removeItem('auth_user');
            appState.isAuthenticated = false;
        }
        if (this.token) {
            localStorage.setItem('auth_token', this.token);
        } else {
            localStorage.removeItem('auth_token');
        }
    },

    clear() {
        this.setUser(null, null);
    },

    async login(username, password) {
        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ username, password })
            });

            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || 'Login failed');
            }

            const data = await response.json();
            const userData = { user_id: data.user_id, username: data.username, role: data.role };
            this.setUser(userData, data.token);
            return userData;
        } catch (error) {
            console.error('AuthManager.login failed', error);
            throw error;
        }
    },

    async logout() {
        try {
            // Tell the server to clear auth + CSRF cookies.
            await fetch('/api/logout', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'X-CSRF-Token': this.getCsrfToken() }
            });
        } catch (e) {
            console.error('AuthManager.logout server call failed', e);
        } finally {
            this.clear();
            if (window.apiClient && typeof window.apiClient.clearToken === 'function') {
                window.apiClient.clearToken();
            }
            if (typeof window.refreshHeaderAuthUI === 'function') {
                window.refreshHeaderAuthUI();
            }
            if (typeof window.appNavigate === 'function') {
                window.appNavigate('login');
            }
        }
    },

    /** Read the csrf_token cookie. */
    getCsrfToken() {
        const m = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
        return m ? decodeURIComponent(m[1]) : '';
    },

    /** Legacy alias for scripts expecting _csrfToken */
    _csrfToken() {
        return this.getCsrfToken();
    },

    getAuthHeader() {
        const csrf = this.getCsrfToken();
        return csrf ? { 'X-CSRF-Token': csrf } : {};
    }
};
