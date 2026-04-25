/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Authentication & Authorization bridge. Wires AuthManager module to window for legacy scripts.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

// This file is now a lightweight bridge for the AuthManager ES module.
// The actual logic lives in js/modules/auth-manager.js

window.refreshHeaderAuthUI = function() {
    const btn = document.getElementById('header-logout-btn');
    if (!btn) return;

    if (window._auth && typeof window._auth.isLoggedIn === 'function' && window._auth.isLoggedIn()) {
        btn.style.display = 'inline-flex';
        if (!btn.dataset.bound) {
            btn.addEventListener('click', function() {
                window._auth.logout();
            });
            btn.dataset.bound = '1';
        }
        return;
    }

    btn.style.display = 'none';
};

// LoginView is used by router.js when a protected route is accessed without session.
window.LoginView = function() {
    window.routerOutlet.innerHTML = `
        <div style="display:flex; align-items:center; justify-content:center; min-height:100vh; background:var(--bg-primary);">
            <div class="glass-panel" style="padding:2rem; width:100%; max-width:400px; border-radius:12px;">
                <div style="text-align:center; margin-bottom:2rem;">
                    <i class="fa-solid fa-shield-halved" style="font-size:3rem; color:var(--accent);"></i>
                    <h1 style="margin:1rem 0 0.25rem 0; font-size:1.5rem;">Access Portal</h1>
                    <p class="text-muted" style="font-size:0.85rem;">Sign in to manage database targets</p>
                </div>

                <form id="loginForm">
                    <div style="margin-bottom:1rem;">
                        <label style="display:block; font-size:0.8rem; font-weight:600; margin-bottom:0.25rem;">Username</label>
                        <input type="text" id="loginUsername" class="custom-input" style="width:100%;" placeholder="Enter username" required>
                    </div>
                    <div style="margin-bottom:1.5rem;">
                        <label style="display:block; font-size:0.8rem; font-weight:600; margin-bottom:0.25rem;">Password</label>
                        <input type="password" id="loginPassword" class="custom-input" style="width:100%;" placeholder="Enter password" required>
                    </div>
                    <div id="loginError" style="display:none; color:var(--danger); font-size:0.8rem; margin-bottom:1rem; padding:0.5rem; background:rgba(239,68,68,0.1); border-radius:4px;"></div>
                    <button type="submit" class="btn btn-accent" style="width:100%;" id="loginBtn">
                        <i class="fa-solid fa-right-to-bracket"></i> Sign In
                    </button>
                </form>

                <div style="text-align:center; margin-top:1.5rem;">
                    <a href="/" class="text-muted" style="font-size:0.75rem; text-decoration:none;"><i class="fa-solid fa-arrow-left"></i> Back to Global Overview</a>
                </div>
            </div>
        </div>
    `;

    document.getElementById('loginForm').addEventListener('submit', window.handleLogin);
};

window.handleLogin = async function(event) {
    event.preventDefault();

    const username = document.getElementById('loginUsername').value.trim();
    const password = document.getElementById('loginPassword').value;
    const errorEl = document.getElementById('loginError');
    const btn = document.getElementById('loginBtn');

    btn.disabled = true;
    btn.innerHTML = 'Signing in...';

    try {
        await window._auth.login(username, password);
        window.refreshHeaderAuthUI();
        if (typeof window.boot === 'function') {
            await window.boot();
        } else {
            window.appNavigate('global');
        }
    } catch (error) {
        errorEl.textContent = error.message;
        errorEl.style.display = 'block';
        btn.disabled = false;
        btn.innerHTML = '<i class="fa-solid fa-right-to-bracket"></i> Sign In';
    }
};

document.addEventListener('DOMContentLoaded', () => window.refreshHeaderAuthUI());
