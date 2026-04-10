/**
 * SQL Optima - Authentication & Authorization System
 * 
 * Handles JWT-based login/logout, token persistence,
 * role-based UI rendering, and route protection.
 */

window._auth = window._auth || {
    token: localStorage.getItem('auth_token') || null,
    user: JSON.parse(localStorage.getItem('auth_user') || 'null'),
    isLoggedIn: function() {
        return !!this.token && !!this.user;
    },
    isAdmin: function() {
        return this.user && this.user.role === 'admin';
    },
    login: async function(username, password) {
        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username: username, password: password })
            });

            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || 'Login failed');
            }

            const data = await response.json();
            this.token = data.token;
            this.user = { user_id: data.user_id, username: data.username, role: data.role };
            localStorage.setItem('auth_token', data.token);
            localStorage.setItem('auth_user', JSON.stringify(this.user));
            return this.user;
        } catch (error) {
            throw error;
        }
    },
    logout: function() {
        this.token = null;
        this.user = null;
        localStorage.removeItem('auth_token');
        localStorage.removeItem('auth_user');
        window.appNavigate('login');
    },
    getAuthHeader: function() {
        return this.token ? { 'Authorization': 'Bearer ' + this.token } : {};
    }
};

// Override authenticatedFetch to include JWT token (deferred until apiClient is ready)
(function setupAuthFetch() {
    function doOverride() {
        if (window.apiClient && window.apiClient.authenticatedFetch) {
            const originalFetch = window.apiClient.authenticatedFetch.bind(window.apiClient);
            window.apiClient.authenticatedFetch = async function(url, options) {
                options = options || {};
                options.headers = options.headers || {};
                if (window._auth.token) {
                    options.headers['Authorization'] = 'Bearer ' + window._auth.token;
                }
                return originalFetch(url, options);
            };
            console.log('[Auth] authenticatedFetch overridden with token support');
        } else {
            setTimeout(doOverride, 100);
        }
    }
    doOverride();
})();

// ========== LOGIN PAGE ==========

window.LoginView = function() {
    window.routerOutlet.innerHTML = `
        <div style="display:flex; align-items:center; justify-content:center; min-height:100vh; background:var(--bg-primary);">
            <div class="glass-panel" style="padding:2rem; width:100%; max-width:400px; border-radius:12px;">
                <div style="text-align:center; margin-bottom:2rem;">
                    <i class="fa-solid fa-shield-halved" style="font-size:3rem; color:var(--accent);"></i>
                    <h1 style="margin:1rem 0 0.25rem 0; font-size:1.5rem;">Admin Access</h1>
                    <p class="text-muted" style="font-size:0.85rem;">Login required for administration</p>
                </div>

                <form id="loginForm" onsubmit="window.handleLogin(event)">
                    <div style="margin-bottom:1rem;">
                        <label style="display:block; font-size:0.8rem; font-weight:600; margin-bottom:0.25rem;">Username</label>
                        <input type="text" id="loginUsername" class="custom-input" style="width:100%;" placeholder="Enter username" required autocomplete="username">
                    </div>
                    <div style="margin-bottom:1.5rem;">
                        <label style="display:block; font-size:0.8rem; font-weight:600; margin-bottom:0.25rem;">Password</label>
                        <input type="password" id="loginPassword" class="custom-input" style="width:100%;" placeholder="Enter password" required autocomplete="current-password">
                    </div>
                    <div id="loginError" style="display:none; color:var(--danger); font-size:0.8rem; margin-bottom:1rem; padding:0.5rem; background:rgba(239,68,68,0.1); border-radius:4px;"></div>
                    <button type="submit" class="btn btn-accent" style="width:100%;" id="loginBtn">
                        <i class="fa-solid fa-right-to-bracket"></i> Sign In
                    </button>
                </form>

                <div style="text-align:center; margin-top:1.5rem;">
                    <a href="/" class="text-muted" style="font-size:0.75rem; text-decoration:none;"><i class="fa-solid fa-arrow-left"></i> Back to Dashboard</a>
                </div>
            </div>
        </div>
    `;
};

window.handleLogin = async function(event) {
    event.preventDefault();

    const username = document.getElementById('loginUsername').value.trim();
    const password = document.getElementById('loginPassword').value;
    const errorEl = document.getElementById('loginError');
    const btn = document.getElementById('loginBtn');

    if (!username || !password) {
        errorEl.textContent = 'Please enter both username and password.';
        errorEl.style.display = 'block';
        return;
    }

    btn.disabled = true;
    btn.innerHTML = '<div class="spinner" style="display:inline-block; width:16px; height:16px; border-width:2px;"></div> Signing in...';
    errorEl.style.display = 'none';

    try {
        await window._auth.login(username, password);
        window.appNavigate('admin');
    } catch (error) {
        errorEl.textContent = error.message;
        errorEl.style.display = 'block';
        btn.disabled = false;
        btn.innerHTML = '<i class="fa-solid fa-right-to-bracket"></i> Sign In';
    }
};

// ========== ADMIN CONTROL PANEL ==========

window.AdminPanelView = async function() {
    if (!window._auth.isAdmin()) {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-danger"><i class="fa-solid fa-ban"></i> Access Denied. Admin role required.</h3></div>`;
        return;
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-shield-halved text-accent"></i> Admin Control Panel</h1>
                    <p class="subtitle">Manage users, widgets, and system settings</p>
                </div>
                <div class="text-muted" style="font-size:0.8rem;">Logged in as: <strong>${window.escapeHtml(window._auth.user?.username || '')}</strong></div>
            </div>

            <div class="settings-tabs" style="display:flex; gap:0.5rem; margin-bottom:1rem; border-bottom:1px solid var(--border-color); padding-bottom:0.5rem;">
                <button class="btn btn-sm btn-accent" onclick="window.showAdminTab('users')" id="admin-tab-users">User Management</button>
                <button class="btn btn-sm btn-outline" onclick="window.showAdminTab('widgets')" id="admin-tab-widgets">Widget Master List</button>
            </div>

            <div id="admin-content">
                <div style="display:flex; justify-content:center; align-items:center; height:200px;">
                    <div class="spinner"></div><span style="margin-left:1rem;">Loading...</span>
                </div>
            </div>
        </div>
    `;

    window.showAdminTab('users');
};

window.showAdminTab = async function(tab) {
    document.querySelectorAll('.settings-tabs .btn').forEach(btn => {
        btn.classList.remove('btn-accent');
        btn.classList.add('btn-outline');
    });
    const activeBtn = document.getElementById('admin-tab-' + tab);
    if (activeBtn) {
        activeBtn.classList.remove('btn-outline');
        activeBtn.classList.add('btn-accent');
    }

    const content = document.getElementById('admin-content');
    if (!content) return;

    if (tab === 'users') {
        content.innerHTML = `
            <div class="glass-panel" style="padding:1rem;">
                <div class="flex-between" style="margin-bottom:1rem;">
                    <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-users text-accent"></i> Users</h3>
                    <button class="btn btn-sm btn-accent" onclick="window.showCreateUserForm()"><i class="fa-solid fa-plus"></i> New User</button>
                </div>
                <div id="admin-user-list"><div class="text-center text-muted">Loading users...</div></div>
            </div>
        `;
        window.loadAdminUsers();
    } else if (tab === 'widgets') {
        content.innerHTML = `
            <div class="glass-panel" style="padding:1rem;">
                <h3 style="font-size:0.85rem; margin:0 0 1rem 0;"><i class="fa-solid fa-chart-line text-accent"></i> Widget Master List</h3>
                <div id="admin-widget-list"><div class="text-center text-muted">Loading widgets...</div></div>
            </div>
        `;
        window.loadAdminWidgets();
    }
};

window.loadAdminUsers = async function() {
    const container = document.getElementById('admin-user-list');
    if (!container) return;

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/users');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        const users = data.users || [];

        if (users.length === 0) {
            container.innerHTML = '<div class="text-center text-muted" style="padding:2rem;">No users found.</div>';
            return;
        }

        container.innerHTML = `
            <div class="table-responsive" style="max-height:400px; overflow-y:auto;">
                <table class="data-table" style="font-size:0.75rem;">
                    <thead><tr><th>ID</th><th>Username</th><th>Role</th><th>Created</th><th>Actions</th></tr></thead>
                    <tbody>
                        ${users.map(u => `
                            <tr>
                                <td>${u.user_id}</td>
                                <td><strong>${window.escapeHtml(u.username)}</strong></td>
                                <td><span class="badge ${u.role === 'admin' ? 'badge-danger' : 'badge-info'}">${window.escapeHtml(u.role)}</span></td>
                                <td>${u.created_at ? new Date(u.created_at).toLocaleDateString() : '-'}</td>
                                <td>
                                    <button class="btn btn-xs btn-outline" onclick="window.changeUserRole(${u.user_id}, '${window.escapeHtml(u.role)}')"><i class="fa-solid fa-edit"></i></button>
                                    ${u.user_id !== 1 ? `<button class="btn btn-xs btn-outline" style="border-color:var(--danger); color:var(--danger);" onclick="window.deleteUser(${u.user_id})"><i class="fa-solid fa-trash"></i></button>` : ''}
                                </td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;
    } catch (error) {
        container.innerHTML = `<div class="alert alert-danger">Failed to load users: ${window.escapeHtml(error.message)}</div>`;
    }
};

window.showCreateUserForm = async function() {
    const username = prompt('Enter username:');
    if (!username || !username.trim()) return;

    const password = prompt('Enter password:');
    if (!password) return;

    const role = prompt('Enter role (admin, viewer):', 'viewer');
    if (role !== 'admin' && role !== 'viewer') {
        alert('Role must be "admin" or "viewer".');
        return;
    }

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/users', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username: username.trim(), password: password, role: role })
        });

        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }

        alert('User created successfully!');
        window.loadAdminUsers();
    } catch (error) {
        alert('Failed to create user: ' + error.message);
    }
};

window.changeUserRole = async function(userId, currentRole) {
    const newRole = currentRole === 'admin' ? 'viewer' : 'admin';
    if (!confirm(`Change user role from "${currentRole}" to "${newRole}"?`)) return;

    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/users/${userId}/role`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ role: newRole })
        });

        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }

        alert('User role updated!');
        window.loadAdminUsers();
    } catch (error) {
        alert('Failed to update role: ' + error.message);
    }
};

window.deleteUser = async function(userId) {
    if (!confirm('Delete this user? This cannot be undone.')) return;

    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/users/${userId}`, {
            method: 'DELETE'
        });

        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }

        alert('User deleted!');
        window.loadAdminUsers();
    } catch (error) {
        alert('Failed to delete user: ' + error.message);
    }
};

window.loadAdminWidgets = async function() {
    const container = document.getElementById('admin-widget-list');
    if (!container) return;

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/widgets');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        const widgets = data.widgets || [];

        if (widgets.length === 0) {
            container.innerHTML = '<div class="text-center text-muted" style="padding:2rem;">No widgets configured in the registry.</div>';
            return;
        }

        container.innerHTML = `
            <div class="table-responsive" style="max-height:500px; overflow-y:auto;">
                <table class="data-table" style="font-size:0.7rem;">
                    <thead><tr><th>Widget ID</th><th>Section</th><th>Title</th><th>Type</th><th>Modified?</th><th>Updated</th><th>Actions</th></tr></thead>
                    <tbody>
                        ${widgets.map(w => {
                            const isModified = w.current_sql !== w.default_sql;
                            return `
                                <tr style="${isModified ? 'background:rgba(245,158,11,0.04);' : ''}">
                                    <td><code style="font-size:0.65rem;">${window.escapeHtml(w.widget_id)}</code></td>
                                    <td>${window.escapeHtml(w.dashboard_section)}</td>
                                    <td><strong>${window.escapeHtml(w.title)}</strong></td>
                                    <td><span class="badge badge-outline">${window.escapeHtml(w.chart_type)}</span></td>
                                    <td>${isModified ? '<span class="badge badge-warning">Modified</span>' : '<span class="text-muted">Default</span>'}</td>
                                    <td>${w.updated_at ? new Date(w.updated_at).toLocaleString() : '-'}</td>
                                    <td>
                                        <button class="btn btn-xs btn-outline" onclick="window.openWidgetEditor('${window.escapeHtml(w.widget_id)}')"><i class="fa-solid fa-pen"></i></button>
                                        ${isModified ? `<button class="btn btn-xs btn-outline" style="border-color:var(--warning); color:var(--warning);" onclick="window.restoreWidgetDefault('${window.escapeHtml(w.widget_id)}')"><i class="fa-solid fa-rotate-left"></i></button>` : ''}
                                    </td>
                                </tr>
                            `;
                        }).join('')}
                    </tbody>
                </table>
            </div>
        `;
    } catch (error) {
        container.innerHTML = `<div class="alert alert-danger">Failed to load widgets: ${window.escapeHtml(error.message)}</div>`;
    }
};
