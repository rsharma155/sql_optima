/**
 * SQL Optima - Authentication & Authorization System
 * 
 * Handles JWT-based login/logout, token persistence,
 * role-based UI rendering, and route protection.
 */

window._auth = window._auth || {
    token: null, // no longer persisted in localStorage — HttpOnly cookie handles transport
    user: JSON.parse(localStorage.getItem('auth_user') || 'null'),
    isLoggedIn: function() {
        // With HttpOnly cookies the JS layer cannot inspect the JWT directly.
        // We keep a lightweight `auth_user` record in localStorage so the UI
        // knows *who* is logged in. On 401 or explicit logout this is cleared.
        return !!this.user;
    },
    isAdmin: function() {
        return this.user && this.user.role === 'admin';
    },
    /** Read the csrf_token cookie set by the backend on login. */
    _csrfToken: function() {
        const m = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
        return m ? decodeURIComponent(m[1]) : '';
    },
    login: async function(username, password) {
        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ username: username, password: password })
            });

            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || 'Login failed');
            }

            const data = await response.json();
            // The JWT is now transported via HttpOnly cookie — we do NOT store it in JS.
            this.token = null;
            this.user = { user_id: data.user_id, username: data.username, role: data.role };
            localStorage.setItem('auth_user', JSON.stringify(this.user));
            return this.user;
        } catch (error) {
            throw error;
        }
    },
    logout: function() {
        // Tell the server to clear auth + CSRF cookies.
        fetch('/api/logout', {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'X-CSRF-Token': this._csrfToken() }
        }).catch(() => {});
        this.token = null;
        this.user = null;
        localStorage.removeItem('auth_token'); // clean up legacy key
        localStorage.removeItem('auth_user');
        if (window.apiClient && typeof window.apiClient.clearToken === 'function') {
            window.apiClient.clearToken();
        }
        window.refreshHeaderAuthUI();
        window.appNavigate('login');
    },
    getAuthHeader: function() {
        // Cookie-based auth: no Authorization header needed for same-origin
        // requests — the browser sends the HttpOnly cookie automatically.
        // Return CSRF header for mutating requests.
        const csrf = this._csrfToken();
        return csrf ? { 'X-CSRF-Token': csrf } : {};
    }
};

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

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => window.refreshHeaderAuthUI());
} else {
    window.refreshHeaderAuthUI();
}

/**
 * Shared client-side checks for add / test monitoring server forms.
 * @param {{name:string,db_type:string,host:string,port:number,username:string,password:string,database?:string}} p
 * @returns {string|null} error message, or null if valid
 */
window.validateMonitoringServerPayload = function(p) {
    const name = (p.name || '').trim();
    const dbType = (p.db_type || '').trim();
    const host = (p.host || '').trim();
    const port = p.port;
    const username = (p.username || '').trim();
    const password = p.password != null ? String(p.password) : '';
    const database = (p.database || '').trim();
    if (!name) return 'Name is required.';
    if (name.length > 128) return 'Name must be 128 characters or fewer.';
    if (dbType !== 'postgres' && dbType !== 'sqlserver') return 'Engine must be PostgreSQL or SQL Server.';
    if (!host) return 'Host is required.';
    if (host.length > 253) return 'Host must be 253 characters or fewer.';
    if (/\s/.test(host)) return 'Host cannot contain whitespace.';
    if (!Number.isFinite(port) || port <= 0 || port > 65535) return 'Enter a valid port (1–65535).';
    if (!username) return 'Username is required.';
    if (username.length > 128) return 'Username must be 128 characters or fewer.';
    if (!password) return 'Password is required.';
    if (password.length > 4096) return 'Password must be 4096 characters or fewer.';
    if (database.length > 128) return 'Initial database / catalog must be 128 characters or fewer.';
    if (database && /[\x00-\x1f\x7f]/.test(database)) return 'Database name contains invalid characters.';
    return null;
};

// Override authenticatedFetch to include CSRF token (deferred until apiClient is ready).
// The JWT is now in an HttpOnly cookie — the browser sends it automatically.
(function setupAuthFetch() {
    function doOverride() {
        if (window.apiClient && window.apiClient.authenticatedFetch) {
            const originalFetch = window.apiClient.authenticatedFetch.bind(window.apiClient);
            window.apiClient.authenticatedFetch = async function(url, options) {
                options = options || {};
                options.headers = options.headers || {};
                options.credentials = 'same-origin';
                // Attach CSRF token for mutating requests.
                const csrf = window._auth._csrfToken();
                if (csrf) {
                    options.headers['X-CSRF-Token'] = csrf;
                }
                return originalFetch(url, options);
            };
            console.log('[Auth] authenticatedFetch overridden with cookie + CSRF support');
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
                    <p class="text-muted" style="font-size:0.85rem;">Login required for admin</p>
                </div>

                <form id="loginForm" data-submit-action="handleLogin">
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
        window.refreshHeaderAuthUI();
        if (typeof window.boot === 'function') {
            await window.boot();
        } else {
            window.appNavigate('admin');
        }
    } catch (error) {
        errorEl.textContent = error.message;
        errorEl.style.display = 'block';
        btn.disabled = false;
        btn.innerHTML = '<i class="fa-solid fa-right-to-bracket"></i> Sign In';
    }
};

// ========== ADMIN CONTROL PANEL ==========

window.AdminPanelView = async function() {
    if (!window._auth.isLoggedIn()) {
        if (typeof window.LoginView === 'function') {
            window.LoginView();
        } else {
            window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Login required</h3></div>`;
        }
        return;
    }
    if (!window._auth.isAdmin()) {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-danger"><i class="fa-solid fa-ban"></i> Access Denied. Admin role required.</h3></div>`;
        return;
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active admin-shell" style="max-width:1180px;margin:0 auto;padding:0 1rem 2.5rem;">
            <div style="display:flex;flex-wrap:wrap;align-items:flex-start;justify-content:space-between;gap:1.25rem;margin-bottom:1.5rem;padding-bottom:1.25rem;border-bottom:1px solid var(--border-color);">
                <div>
                    <h1 style="margin:0;font-size:1.5rem;font-weight:600;letter-spacing:-0.02em;">
                        <i class="fa-solid fa-sliders text-accent"></i> Admin control panel
                    </h1>
                    <p class="text-muted" style="margin:0.4rem 0 0;font-size:0.9rem;line-height:1.5;max-width:40rem;">
                        Manage users and monitored database targets. Dashboard widget SQL is edited in context from each dashboard (pencil icon), not from a separate list here.
                    </p>
                </div>
                <div class="glass-panel" style="padding:0.65rem 1rem;font-size:0.82rem;border-radius:10px;">
                    <span class="text-muted">Signed in as</span><br/><strong>${window.escapeHtml(window._auth.user?.username || '')}</strong>
                </div>
                <button type="button" class="btn btn-sm btn-outline" data-action="logout" title="Log out">
                    <i class="fa-solid fa-right-from-bracket"></i> Log out
                </button>
            </div>
            <div id="admin-nav-bar" style="display:flex;flex-wrap:wrap;gap:0.5rem;margin-bottom:1.25rem;">
                <button type="button" class="btn btn-sm btn-accent admin-nav-btn" data-action="show-admin-tab" data-tab="users" id="admin-tab-users"><i class="fa-solid fa-users"></i> User management</button>
                <button type="button" class="btn btn-sm btn-outline admin-nav-btn" data-action="show-admin-tab" data-tab="servers" id="admin-tab-servers"><i class="fa-solid fa-server"></i> Monitoring servers</button>
            </div>
            <div id="admin-content">
                <div style="display:flex; justify-content:center; align-items:center; min-height:12rem;">
                    <div class="spinner"></div><span class="text-muted" style="margin-left:1rem;">Loading…</span>
                </div>
            </div>
        </div>
    `;

    window.showAdminTab('users');
};

window.showAdminTab = async function(tab) {
    document.querySelectorAll('#admin-nav-bar .admin-nav-btn').forEach(btn => {
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
            <div class="glass-panel" style="padding:1.25rem;border-radius:12px;">
                <div class="flex-between" style="margin-bottom:1rem;flex-wrap:wrap;gap:0.75rem;">
                    <h2 style="font-size:1rem;margin:0;font-weight:600;"><i class="fa-solid fa-users text-accent"></i> Users</h2>
                    <button type="button" class="btn btn-sm btn-accent" data-action="show-create-user"><i class="fa-solid fa-user-plus"></i> New user</button>
                </div>
                <div id="admin-user-list"><div class="text-center text-muted" style="padding:2rem;">Loading users…</div></div>
            </div>
        `;
        window.loadAdminUsers();
    } else if (tab === 'servers') {
        content.innerHTML = `
            <div class="glass-panel" style="padding:1.25rem;border-radius:12px;">
                <div class="flex-between" style="margin-bottom:1rem;flex-wrap:wrap;gap:0.75rem;">
                    <div>
                        <h2 style="font-size:1rem;margin:0;font-weight:600;"><i class="fa-solid fa-server text-accent"></i> Monitoring servers</h2>
                        <p class="text-muted" style="margin:0.35rem 0 0;font-size:0.8rem;max-width:42rem;">Targets are stored in TimescaleDB with envelope encryption. <strong>Delete</strong> removes the row permanently (audit log may still record the event). Use <strong>Deactivate</strong> to pause collection without removing config.</p>
                    </div>
                    <button type="button" class="btn btn-sm btn-accent" data-action="show-add-server"><i class="fa-solid fa-plus"></i> Add server</button>
                </div>
                <details style="margin-bottom:1rem;font-size:0.8rem;" class="text-muted">
                    <summary style="cursor:pointer;color:var(--accent);font-weight:500;">Reset TimescaleDB connection &amp; run setup again</summary>
                    <div style="margin-top:0.6rem;line-height:1.55;max-width:44rem;">
                        <ol style="margin:0;padding-left:1.2rem;">
                            <li>Stop the API process.</li>
                            <li>Delete the encrypted file <code style="font-size:0.75rem;">data/timescale_connection.enc.json</code> (under the same directory as your <code>config.yaml</code>).</li>
                            <li>Set <code>ALLOW_TIMESCALE_RECONFIG=1</code> if the server refuses to replace an existing persisted connection.</li>
                            <li>Optionally truncate <code>optima_servers</code> in Timescale if you want an empty registry.</li>
                            <li>Restart the API and open <strong>/setup</strong> again.</li>
                        </ol>
                    </div>
                </details>
                <div id="admin-server-msg"></div>
                <div id="admin-server-list"><div class="text-center text-muted" style="padding:2rem;">Loading servers…</div></div>
            </div>
        `;
        window.loadAdminServers();
    }
};

window.loadAdminServers = async function() {
    const container = document.getElementById('admin-server-list');
    if (!container) return;
    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/servers');
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        const data = await response.json();
        const servers = data.servers || [];

        if (!Array.isArray(servers) || servers.length === 0) {
            container.innerHTML = '<div class="text-center text-muted" style="padding:2rem;">No servers registered yet.</div>';
            return;
        }

        container.innerHTML = `
            <div class="table-responsive" style="max-height:500px; overflow-y:auto;">
                <table class="data-table" style="font-size:0.75rem;">
                    <thead><tr>
                        <th>Name</th><th>Type</th><th>Host</th><th>Port</th><th>Username</th><th>Active</th><th>Last tested</th><th>Actions</th>
                    </tr></thead>
                    <tbody>
                        ${servers.map(s => {
                            const sid = String(s.id || '').replace(/'/g, '');
                            const last = s.last_tested ? new Date(s.last_tested).toLocaleString() : '—';
                            return `
                            <tr>
                                <td><strong>${window.escapeHtml(s.name || '')}</strong></td>
                                <td><span class="badge badge-outline">${window.escapeHtml(String(s.db_type || ''))}</span></td>
                                <td><code>${window.escapeHtml(s.host || '')}</code></td>
                                <td>${window.escapeHtml(String(s.port ?? ''))}</td>
                                <td>${window.escapeHtml(s.username || '')}</td>
                                <td>${s.is_active ? '<span class="badge badge-success">Yes</span>' : '<span class="badge badge-warning">No</span>'}</td>
                                <td class="text-muted" style="white-space:nowrap;font-size:0.7rem;">${window.escapeHtml(last)}</td>
                                <td style="white-space:normal;">
                                    <button class="btn btn-xs btn-outline" data-action="test-server" data-id="${window.escapeHtml(sid)}"><i class="fa-solid fa-plug-circle-check"></i> Test</button>
                                    <button class="btn btn-xs btn-outline" data-action="patch-server-active" data-id="${window.escapeHtml(sid)}" data-active="${s.is_active ? 'false' : 'true'}"><i class="fa-solid fa-toggle-on"></i> ${s.is_active ? 'Deactivate' : 'Activate'}</button>
                                    <button class="btn btn-xs btn-outline" data-action="show-edit-server" data-id="${window.escapeHtml(sid)}"><i class="fa-solid fa-pen"></i> Edit</button>
                                    <button class="btn btn-xs btn-outline" data-action="rotate-server" data-id="${window.escapeHtml(sid)}"><i class="fa-solid fa-key"></i> Rotate</button>
                                    <button class="btn btn-xs btn-outline" style="border-color:var(--danger);color:var(--danger);" data-action="delete-server" data-id="${window.escapeHtml(sid)}"><i class="fa-solid fa-trash"></i></button>
                                </td>
                            </tr>`;
                        }).join('')}
                    </tbody>
                </table>
            </div>
        `;
    } catch (error) {
        container.innerHTML = `<div class="alert alert-danger">Failed to load servers: ${window.escapeHtml(error.message)}</div>`;
    }
};

window.showAddServerForm = function() {
    const content = document.getElementById('admin-content');
    if (!content) return;

    const formId = 'admin-add-server-form';
    const existing = document.getElementById(formId);
    if (existing) {
        existing.scrollIntoView({ behavior: 'smooth', block: 'start' });
        return;
    }

    const msg = document.getElementById('admin-server-msg');
    if (msg) msg.innerHTML = '';

    const html = `
        <div class="glass-panel" id="${formId}" style="padding:0;margin-bottom:1.25rem;border-radius:12px;overflow:hidden;border:1px solid var(--border-color);">
            <div style="padding:0.9rem 1.1rem;background:linear-gradient(135deg, rgba(59,130,246,0.12), rgba(139,92,246,0.08));border-bottom:1px solid var(--border-color);" class="flex-between">
                <h3 style="margin:0;font-size:0.95rem;font-weight:600;"><i class="fa-solid fa-plus text-accent"></i> Add monitoring server</h3>
                <button type="button" class="btn btn-xs btn-outline" data-action="close-id" data-target="${formId}"><i class="fa-solid fa-xmark"></i></button>
            </div>
            <div style="padding:1.1rem 1.15rem 1.25rem;">
                <div class="glass-panel" style="padding:0.75rem 0.9rem;margin-bottom:1rem;font-size:0.78rem;line-height:1.5;border-left:3px solid var(--accent);border-radius:8px;">
                    <strong style="display:block;margin-bottom:0.25rem;"><i class="fa-solid fa-vault"></i> Vault &amp; encryption</strong>
                    <span class="text-muted">Credentials are encrypted in TimescaleDB. Data encryption keys come from <strong>HashiCorp Vault Transit</strong> when the API has <code>VAULT_ADDR</code> (and typically <code>VAULT_TOKEN</code>, <code>VAULT_TRANSIT_MOUNT</code>, <code>VAULT_TRANSIT_KEY</code>, optional <code>VAULT_NAMESPACE</code>). Otherwise a local envelope KMS is used. Configure those on the API process—nothing Vault-specific is entered in this form.</span>
                </div>
                <div class="admin-add-grid" style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem 1.25rem;align-items:start;">
                    <div class="admin-add-field">
                        <label for="srv-name">Display name</label>
                        <input class="custom-input" id="srv-name" placeholder="Production Postgres" autocomplete="off" />
                    </div>
                    <div class="admin-add-field">
                        <label for="srv-type">Engine</label>
                        <select class="custom-select" id="srv-type" style="width:100%;min-height:2.4rem;">
                            <option value="postgres">PostgreSQL</option>
                            <option value="sqlserver">SQL Server</option>
                        </select>
                    </div>
                    <div class="admin-add-field">
                        <label for="srv-host">Host</label>
                        <input class="custom-input" id="srv-host" placeholder="db.example.com" autocomplete="off" />
                    </div>
                    <div class="admin-add-field">
                        <label for="srv-port">Port</label>
                        <input class="custom-input" id="srv-port" placeholder="5432 or 1433" inputmode="numeric" autocomplete="off" />
                    </div>
                    <div class="admin-add-field">
                        <label for="srv-user">Username</label>
                        <input class="custom-input" id="srv-user" placeholder="monitoring login" autocomplete="off" />
                    </div>
                    <div class="admin-add-field">
                        <label for="srv-pass">Password</label>
                        <input class="custom-input" id="srv-pass" type="password" autocomplete="new-password" />
                    </div>
                    <div class="admin-add-field" style="grid-column:1/-1;">
                        <label for="srv-ssl">SSL mode (PostgreSQL / RDS)</label>
                        <select class="custom-select" id="srv-ssl" style="width:100%;min-height:2.4rem;">
                            <option value="require">require</option>
                            <option value="disable">disable</option>
                            <option value="verify-full">verify-full</option>
                        </select>
                    </div>
                    <div class="admin-add-field" style="grid-column:1/-1;">
                        <label for="srv-database">Initial database / catalog <span class="text-muted" style="font-weight:400;">(optional)</span></label>
                        <input class="custom-input" id="srv-database" placeholder="postgres or master (Azure SQL / MI)" autocomplete="off" />
                        <p class="text-muted" style="font-size:0.72rem;margin:0.35rem 0 0;line-height:1.4;">RDS/Aurora: often <code>postgres</code>. Azure SQL / Managed Instance: <code>master</code>; public MI commonly port <code>3342</code>.</p>
                    </div>
                    <div id="srv-trust-wrap" class="admin-add-field" style="grid-column:1/-1;display:none;">
                        <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-weight:500;margin:0;">
                            <input type="checkbox" id="srv-trust-cert" style="width:1rem;height:1rem;" />
                            Trust server certificate (Azure SQL / MI when strict TLS validation fails)
                        </label>
                    </div>
                </div>
                <style>
                    #${formId} .admin-add-field label { display:block; font-size:0.72rem; font-weight:600; text-transform:uppercase; letter-spacing:0.04em; color:var(--text-muted); margin-bottom:0.35rem; }
                    #${formId} .admin-add-field .custom-input, #${formId} .admin-add-field .custom-select { width:100%; min-height:2.4rem; box-sizing:border-box; }
                </style>
                <div style="display:flex;flex-wrap:wrap;gap:0.5rem;margin-top:1.1rem;align-items:center;">
                    <button type="button" class="btn btn-sm btn-outline" data-action="test-server-add-draft"><i class="fa-solid fa-plug-circle-check"></i> Test connection</button>
                    <button type="button" class="btn btn-sm btn-accent" data-action="submit-add-server"><i class="fa-solid fa-floppy-disk"></i> Save server</button>
                    <button type="button" class="btn btn-sm btn-outline" data-action="load-admin-servers"><i class="fa-solid fa-rotate"></i> Refresh list</button>
                </div>
                <div id="srv-add-error" style="display:none;margin-top:0.85rem;" class="alert alert-danger"></div>
            </div>
        </div>
    `;

    const serversPanel = content.querySelector('.glass-panel');
    if (serversPanel) {
        serversPanel.insertAdjacentHTML('beforebegin', html);
    } else {
        content.insertAdjacentHTML('afterbegin', html);
    }
    const typeSel = document.getElementById('srv-type');
    const trustWrap = document.getElementById('srv-trust-wrap');
    const syncTrust = () => {
        if (!trustWrap || !typeSel) return;
        trustWrap.style.display = typeSel.value === 'sqlserver' ? 'block' : 'none';
    };
    typeSel?.addEventListener('change', syncTrust);
    syncTrust();
};

window.testServerAddDraft = async function() {
    const errEl = document.getElementById('srv-add-error');
    const msg = document.getElementById('admin-server-msg');
    if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
    const name = document.getElementById('srv-name')?.value?.trim() || '';
    const dbType = document.getElementById('srv-type')?.value?.trim() || '';
    const host = document.getElementById('srv-host')?.value?.trim() || '';
    const portRaw = document.getElementById('srv-port')?.value?.trim() || '';
    const username = document.getElementById('srv-user')?.value?.trim() || '';
    const password = document.getElementById('srv-pass')?.value || '';
    const sslMode = document.getElementById('srv-ssl')?.value?.trim() || '';
    const database = document.getElementById('srv-database')?.value?.trim() || '';
    const trust_server_certificate = !!document.getElementById('srv-trust-cert')?.checked;
    const port = parseInt(portRaw, 10);
    const payload = { name, db_type: dbType, host, port, username, password, ssl_mode: sslMode, database, trust_server_certificate };
    const vErr = window.validateMonitoringServerPayload ? window.validateMonitoringServerPayload(payload) : null;
    if (vErr) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">${window.escapeHtml(vErr)}</div>`;
        return;
    }
    if (msg) msg.innerHTML = '<div class="alert alert-info">Testing connection…</div>';
    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/servers/test-draft', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
        const j = await response.json().catch(() => ({}));
        if (!response.ok || !j.success) {
            throw new Error(j.error || `HTTP ${response.status}`);
        }
        if (msg) msg.innerHTML = '<div class="alert alert-success">Connection test succeeded. You can save the server.</div>';
    } catch (e) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">Connection test failed: ${window.escapeHtml(e.message || String(e))}</div>`;
    }
};

window.submitAddServer = async function() {
    const errEl = document.getElementById('srv-add-error');
    if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }

    const name = document.getElementById('srv-name')?.value?.trim() || '';
    const dbType = document.getElementById('srv-type')?.value?.trim() || '';
    const host = document.getElementById('srv-host')?.value?.trim() || '';
    const portRaw = document.getElementById('srv-port')?.value?.trim() || '';
    const username = document.getElementById('srv-user')?.value?.trim() || '';
    const password = document.getElementById('srv-pass')?.value || '';
    const sslMode = document.getElementById('srv-ssl')?.value?.trim() || '';
    const database = document.getElementById('srv-database')?.value?.trim() || '';
    const trustServerCertificate = !!document.getElementById('srv-trust-cert')?.checked;

    const port = parseInt(portRaw, 10);
    const payload = { name, db_type: dbType, host, port, username, password, ssl_mode: sslMode, database, trust_server_certificate: trustServerCertificate };
    const vErr = window.validateMonitoringServerPayload ? window.validateMonitoringServerPayload(payload) : null;
    if (vErr) {
        if (errEl) {
            errEl.textContent = vErr;
            errEl.style.display = 'block';
        }
        return;
    }

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/servers', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        await response.json().catch(() => ({}));
        window.loadAdminServers();
        if (document.getElementById('onb-list') && typeof window.onbLoadServers === 'function') {
            window.onbLoadServers();
        }
        // Refresh the config to include the new server in the instance dropdown
        if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
            await window.apiClient.fetchConfig();
            if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
                window.router.populateInstanceDropdown();
            }
        }
        const form = document.getElementById('admin-add-server-form');
        if (form) form.remove();
        const msg = document.getElementById('admin-server-msg');
        if (msg) {
            msg.innerHTML = `<div class="alert alert-success">Server added successfully. Open <strong>Global Estate Overview</strong> in the sidebar to see all monitored targets, then pick an instance.</div>`;
        }
    } catch (e) {
        if (errEl) {
            errEl.textContent = e.message || String(e);
            errEl.style.display = 'block';
        } else {
            alert('Failed to add server: ' + (e.message || String(e)));
        }
    }
};

window.testAdminServer = async function(id) {
    id = String(id || '').trim();
    if (!id) return;
    const msg = document.getElementById('admin-server-msg');
    if (msg) msg.innerHTML = `<div class="alert alert-info">Testing connection...</div>`;

    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}/test`, { method: 'POST' });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || 'connection failed');
        }
        await response.json().catch(() => ({}));
        if (msg) msg.innerHTML = `<div class="alert alert-success">Connection OK.</div>`;
        if (typeof window.loadAdminServers === 'function') window.loadAdminServers();
        if (document.getElementById('onb-list') && typeof window.onbLoadServers === 'function') {
            window.onbLoadServers();
        }
    } catch (e) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">Connection failed: ${window.escapeHtml(e.message || String(e))}</div>`;
    }
};

window.patchServerActive = async function(id, nextActive) {
    id = String(id || '').trim();
    if (!id) return;
    const msg = document.getElementById('admin-server-msg');
    try {
        const on = (nextActive === true || nextActive === 'true');
        const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ is_active: on })
        });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        if (msg) msg.innerHTML = `<div class="alert alert-success">Server updated.</div>`;
        window.loadAdminServers();
        if (document.getElementById('onb-list') && typeof window.onbLoadServers === 'function') {
            window.onbLoadServers();
        }
        if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
            await window.apiClient.fetchConfig();
            if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
                window.router.populateInstanceDropdown();
            }
        }
    } catch (e) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">${window.escapeHtml(e.message || String(e))}</div>`;
    }
};

window.deleteAdminServer = async function(id) {
    id = String(id || '').trim();
    if (!id || !confirm('Permanently delete this monitoring server from the registry? This cannot be undone. Use Deactivate if you only want to pause collection.')) return;
    const msg = document.getElementById('admin-server-msg');
    const onbMsg = document.getElementById('onb-msg');
    const setMsg = (html) => {
        if (msg) msg.innerHTML = html;
        if (onbMsg) onbMsg.innerHTML = html;
    };
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}`, { method: 'DELETE' });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        setMsg(`<div class="alert alert-success">Server deleted from the registry.</div>`);
        window.loadAdminServers();
        if (document.getElementById('onb-list') && typeof window.onbLoadServers === 'function') {
            window.onbLoadServers();
        }
        if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
            await window.apiClient.fetchConfig();
            if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
                window.router.populateInstanceDropdown();
            }
        }
    } catch (e) {
        setMsg(`<div class="alert alert-danger">${window.escapeHtml(e.message || String(e))}</div>`);
    }
};

window.rotateAdminServerPrompt = async function(id) {
    id = String(id || '').trim();
    if (!id) return;
    const pw = prompt('Enter new password for this monitoring login:');
    if (!pw) return;
    const ssl = prompt('SSL mode for Postgres (optional, blank = keep):', '') || '';
    const msg = document.getElementById('admin-server-msg');
    try {
        const body = { password: pw };
        if (ssl.trim()) body.ssl_mode = ssl.trim();
        const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}/rotate`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        if (msg) msg.innerHTML = `<div class="alert alert-success">Credentials rotated.</div>`;
    } catch (e) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">${window.escapeHtml(e.message || String(e))}</div>`;
    }
};

window.showEditServerForm = async function(id) {
    id = String(id || '').trim();
    if (!id) return;
    let row = null;
    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/servers');
        if (!response.ok) return;
        const data = await response.json();
        row = (data.servers || []).find(x => String(x.id) === id);
    } catch (e) { return; }
    if (!row) return;

    const content = document.getElementById('admin-content');
    if (!content) return;
    const formId = 'admin-edit-server-form';
    document.getElementById(formId)?.remove();

    const html = `
        <div class="glass-panel" id="${formId}" style="padding:1rem;margin-bottom:1rem;border:1px solid var(--border-color);">
            <div class="flex-between" style="margin-bottom:0.75rem;">
                <h4 style="margin:0;font-size:0.85rem;"><i class="fa-solid fa-pen text-accent"></i> Edit server</h4>
                <button type="button" class="btn btn-xs btn-outline" data-action="close-id" data-target="${formId}"><i class="fa-solid fa-xmark"></i></button>
            </div>
            <p class="text-muted" style="font-size:0.75rem;">Leave password empty to keep existing credentials. To change password use <strong>Rotate</strong> or enter a new password here.</p>
            <input type="hidden" id="edit-srv-id" value="${window.escapeHtml(id)}" />
            <div class="grid" style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:0.75rem;">
                <div><label style="font-size:0.75rem;">Name</label><input class="custom-input" id="edit-srv-name" value="${window.escapeHtml(row.name || '')}" /></div>
                <div><label style="font-size:0.75rem;">Host</label><input class="custom-input" id="edit-srv-host" value="${window.escapeHtml(row.host || '')}" /></div>
                <div><label style="font-size:0.75rem;">Port</label><input class="custom-input" id="edit-srv-port" value="${window.escapeHtml(String(row.port ?? ''))}" /></div>
                <div><label style="font-size:0.75rem;">Username</label><input class="custom-input" id="edit-srv-user" value="${window.escapeHtml(row.username || '')}" /></div>
                <div><label style="font-size:0.75rem;">New password (optional)</label><input class="custom-input" id="edit-srv-pass" type="password" autocomplete="new-password" /></div>
                <div><label style="font-size:0.75rem;">SSL mode</label>
                    <select class="custom-select" id="edit-srv-ssl">
                        <option value="require" ${String(row.ssl_mode) === 'require' ? 'selected' : ''}>require</option>
                        <option value="disable" ${String(row.ssl_mode) === 'disable' ? 'selected' : ''}>disable</option>
                        <option value="verify-full" ${String(row.ssl_mode) === 'verify-full' ? 'selected' : ''}>verify-full</option>
                    </select>
                </div>
            </div>
            <div id="edit-srv-err" style="display:none;margin-top:0.75rem;" class="alert alert-danger"></div>
            <button type="button" class="btn btn-sm btn-accent" style="margin-top:0.75rem;" data-action="submit-edit-server"><i class="fa-solid fa-floppy-disk"></i> Save</button>
        </div>`;
    const serversPanel = content.querySelector('.glass-panel');
    if (serversPanel) serversPanel.insertAdjacentHTML('beforebegin', html);
    else content.insertAdjacentHTML('afterbegin', html);
};

window.submitEditServer = async function() {
    const errEl = document.getElementById('edit-srv-err');
    if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
    const id = document.getElementById('edit-srv-id')?.value?.trim() || '';
    const name = document.getElementById('edit-srv-name')?.value?.trim() || '';
    const host = document.getElementById('edit-srv-host')?.value?.trim() || '';
    const port = parseInt(document.getElementById('edit-srv-port')?.value?.trim() || '', 10);
    const username = document.getElementById('edit-srv-user')?.value?.trim() || '';
    const password = document.getElementById('edit-srv-pass')?.value || '';
    const ssl_mode = document.getElementById('edit-srv-ssl')?.value || 'require';
    const payload = { name, host, port, username, ssl_mode };
    if (password) payload.password = password;
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        document.getElementById('admin-edit-server-form')?.remove();
        window.loadAdminServers();
        if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
            await window.apiClient.fetchConfig();
            if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
                window.router.populateInstanceDropdown();
            }
        }
        const msg = document.getElementById('admin-server-msg');
        if (msg) msg.innerHTML = `<div class="alert alert-success">Server updated.</div>`;
    } catch (e) {
        if (errEl) {
            errEl.textContent = e.message || String(e);
            errEl.style.display = 'block';
        }
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
                                    <button class="btn btn-xs btn-outline" data-action="change-user-role" data-id="${u.user_id}" data-role="${window.escapeHtml(u.role)}"><i class="fa-solid fa-edit"></i></button>
                                    ${u.user_id !== 1 ? `<button class="btn btn-xs btn-outline" style="border-color:var(--danger); color:var(--danger);" data-action="delete-user" data-id="${u.user_id}"><i class="fa-solid fa-trash"></i></button>` : ''}
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
                                        <button class="btn btn-xs btn-outline" data-action="open-widget-editor" data-id="${window.escapeHtml(w.widget_id)}"><i class="fa-solid fa-pen"></i></button>
                                        ${isModified ? `<button class="btn btn-xs btn-outline" style="border-color:var(--warning); color:var(--warning);" data-action="restore-widget-default" data-id="${window.escapeHtml(w.widget_id)}"><i class="fa-solid fa-rotate-left"></i></button>` : ''}
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
