/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Admin Control Panel views and management functions for users, servers, and collectors.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

/**
 * AdminPanelView - Renders the main admin interface
 */
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
                        Manage users and monitored database targets.
                    </p>
                </div>
                <div class="glass-panel" style="padding:0.65rem 1rem;font-size:0.82rem;border-radius:10px;">
                    <span class="text-muted">Signed in as</span><br/><strong>${window.escapeHtml(window._auth.getUser()?.username || '')}</strong>
                </div>
                <button type="button" class="btn btn-sm btn-outline" id="admin-logout-btn" title="Log out">
                    <i class="fa-solid fa-right-from-bracket"></i> Log out
                </button>
            </div>
            <div id="admin-nav-bar" style="display:flex;flex-wrap:wrap;gap:0.5rem;margin-bottom:1.25rem;">
                <button type="button" class="btn btn-sm btn-accent admin-nav-btn" id="admin-tab-users"><i class="fa-solid fa-users"></i> User management</button>
                <button type="button" class="btn btn-sm btn-outline admin-nav-btn" id="admin-tab-servers"><i class="fa-solid fa-server"></i> Monitoring servers</button>
                <button type="button" class="btn btn-sm btn-outline admin-nav-btn" id="admin-tab-collectors"><i class="fa-solid fa-clock-rotate-left"></i> Collector frequencies</button>
                <button type="button" class="btn btn-sm btn-outline admin-nav-btn" id="admin-tab-notifications"><i class="fa-solid fa-bell"></i> Notifications</button>
                <button type="button" class="btn btn-sm btn-outline admin-nav-btn" id="admin-tab-diagnostics"><i class="fa-solid fa-stethoscope"></i> SQL diagnostics</button>
            </div>
            <div id="admin-content">
                <div style="display:flex; justify-content:center; align-items:center; min-height:12rem;">
                    <div class="spinner"></div><span class="text-muted" style="margin-left:1rem;">Loading…</span>
                </div>
            </div>
        </div>
    `;

    // Bind events
    document.getElementById('admin-logout-btn').addEventListener('click', () => window._auth.logout());
    document.getElementById('admin-tab-users').addEventListener('click', () => window.showAdminTab('users'));
    document.getElementById('admin-tab-servers').addEventListener('click', () => window.showAdminTab('servers'));
    document.getElementById('admin-tab-collectors').addEventListener('click', () => window.showAdminTab('collectors'));
    document.getElementById('admin-tab-notifications').addEventListener('click', () => window.showAdminTab('notifications'));
    document.getElementById('admin-tab-diagnostics').addEventListener('click', () => window.showAdminTab('diagnostics'));

    const initialTab = new URLSearchParams(window.location.search).get('tab');
    if (initialTab === 'diagnostics' || initialTab === 'collectors' || initialTab === 'notifications' || initialTab === 'servers' || initialTab === 'users') {
        window.showAdminTab(initialTab);
    } else {
        window.showAdminTab('users');
    }
};

/**
 * showAdminTab - Switches between admin tabs
 */
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
                </div>
                <div id="admin-user-list"><div class="text-center text-muted" style="padding:2rem;">Loading users…</div></div>
            </div>
        `;
        window.loadAdminUsers();
    } else if (tab === 'servers') {
        content.innerHTML = `
            <div style="display:grid; grid-template-columns: 1fr 300px; gap:1.25rem; align-items:start;">
                <div class="glass-panel" style="padding:1.25rem;border-radius:12px;">
                    <div class="flex-between" style="margin-bottom:1rem;flex-wrap:wrap;gap:0.75rem;">
                        <div>
                            <h2 style="font-size:1rem;margin:0;font-weight:600;"><i class="fa-solid fa-server text-accent"></i> Monitoring servers</h2>
                            <p class="text-muted" style="margin:0.35rem 0 0;font-size:0.8rem;max-width:42rem;">Targets are stored in TimescaleDB with envelope encryption.</p>
                        </div>
                        <button type="button" class="btn btn-sm btn-accent" id="btn-show-add-server"><i class="fa-solid fa-plus"></i> Add server</button>
                    </div>
                    <div id="admin-server-msg"></div>
                    <div id="admin-server-add-slot"></div>
                    <div id="admin-server-list"><div class="text-center text-muted" style="padding:2rem;">Loading servers…</div></div>
                </div>

                <div class="glass-panel" style="padding:1rem; border-radius:12px; font-size:0.8rem;">
                    <h3 style="font-size:0.9rem; margin:0 0 0.75rem; font-weight:600;"><i class="fa-solid fa-key text-accent"></i> Permission Setup</h3>
                    <p class="text-muted" style="line-height:1.4; margin-bottom:0.75rem;">
                        Monitoring requires read-only access. Run these scripts to set up the user:
                    </p>
                    <div style="display:flex; flex-direction:column; gap:0.5rem;">
                        <button class="btn btn-xs btn-outline" onclick="window.onbShowPermissionScript('sqlserver')" style="justify-content:flex-start; padding:0.4rem 0.6rem;">
                            <i class="fa-brands fa-microsoft"></i> SQL Server Script
                        </button>
                        <button class="btn btn-xs btn-outline" onclick="window.onbShowPermissionScript('postgres')" style="justify-content:flex-start; padding:0.4rem 0.6rem;">
                            <i class="fa-solid fa-database"></i> PostgreSQL Script
                        </button>
                    </div>

                    <div id="admin-dist-note" style="margin-top:1rem; padding:0.75rem; border-radius:6px; background:rgba(var(--accent-rgb), 0.05); border:1px solid rgba(var(--accent-rgb), 0.1); display:none;">
                        <h4 style="font-size:0.75rem; margin:0 0 0.4rem; color:var(--accent); font-weight:700;">
                            <i class="fa-solid fa-circle-info"></i> Distributor Note
                        </h4>
                        <p style="font-size:0.7rem; line-height:1.3; margin:0;">
                            Remote Distributor? Run script on the <strong>Distributor instance</strong>.
                        </p>
                    </div>
                </div>
            </div>
        `;
        document.getElementById('btn-show-add-server').addEventListener('click', () => window.showAddServerForm());
        window.loadAdminServers();
    } else if (tab === 'collectors') {
        try {
            // Using a relative path for the dynamic import to be more robust across different environments
            const { loadCollectorConfigs } = await import('./admin_collector_control.js');
            if (typeof loadCollectorConfigs === 'function') {
                await loadCollectorConfigs();
            } else {
                throw new Error('loadCollectorConfigs is not a function in the imported module');
            }
        } catch (err) {
            console.error('Failed to load collector configs module:', err);
            content.innerHTML = `
                <div class="glass-panel" style="padding:2rem; border-radius:12px; text-align:center;">
                    <i class="fa-solid fa-triangle-exclamation text-danger" style="font-size:2rem; margin-bottom:1rem;"></i>
                    <h3 style="margin:0 0 0.5rem;">Failed to load Collector Control</h3>
                    <p class="text-muted">${window.escapeHtml(err.message)}</p>
                    <button class="btn btn-sm btn-accent" id="admin-collector-retry-btn">
                        <i class="fa-solid fa-rotate"></i> Retry
                    </button>
                </div>
            `;
            document.getElementById('admin-collector-retry-btn')?.addEventListener('click', () => window.showAdminTab('collectors'));
        }
    } else if (tab === 'notifications') {
        try {
            const { loadNotificationConfig } = await import('./admin_notifications.js');
            if (typeof loadNotificationConfig === 'function') {
                await loadNotificationConfig();
            } else {
                throw new Error('loadNotificationConfig is not a function in the imported module');
            }
        } catch (err) {
            console.error('Failed to load notifications module:', err);
            content.innerHTML = `
                <div class="glass-panel" style="padding:2rem; border-radius:12px; text-align:center;">
                    <i class="fa-solid fa-triangle-exclamation text-danger" style="font-size:2rem; margin-bottom:1rem;"></i>
                    <h3 style="margin:0 0 0.5rem;">Failed to load Notifications</h3>
                    <p class="text-muted">${window.escapeHtml(err.message)}</p>
                    <button class="btn btn-sm btn-accent" id="admin-notif-retry-btn">
                        <i class="fa-solid fa-rotate"></i> Retry
                    </button>
                </div>
            `;
            document.getElementById('admin-notif-retry-btn')?.addEventListener('click', () => window.showAdminTab('notifications'));
        }
    } else if (tab === 'diagnostics') {
        try {
            const { loadSqlServerDiagnostics } = await import('./admin_sqlserver_diagnostics.js');
            if (typeof loadSqlServerDiagnostics === 'function') {
                await loadSqlServerDiagnostics();
            } else {
                throw new Error('loadSqlServerDiagnostics is not a function');
            }
        } catch (err) {
            console.error('Failed to load SQL Server diagnostics module:', err);
            content.innerHTML = `
                <div class="glass-panel" style="padding:2rem; border-radius:12px; text-align:center;">
                    <i class="fa-solid fa-triangle-exclamation text-danger" style="font-size:2rem; margin-bottom:1rem;"></i>
                    <h3 style="margin:0 0 0.5rem;">Failed to load SQL diagnostics</h3>
                    <p class="text-muted">${window.escapeHtml(err.message)}</p>
                    <button class="btn btn-sm btn-accent" id="admin-diag-retry-btn">
                        <i class="fa-solid fa-rotate"></i> Retry
                    </button>
                </div>
            `;
            document.getElementById('admin-diag-retry-btn')?.addEventListener('click', () => window.showAdminTab('diagnostics'));
        }
    }
};

/**
 * loadAdminServers - Fetches and renders the list of monitored servers
 */
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
            <div class="table-responsive">
                <table class="data-table" style="font-size:0.85rem; border-collapse: separate; border-spacing: 0 0.5rem;">
                    <thead>
                        <tr style="text-align:left; font-size:0.75rem; text-transform:uppercase; color:var(--text-muted);">
                            <th style="padding:0.75rem 1rem;">Server Name</th>
                            <th style="padding:0.75rem 1rem;">Engine</th>
                            <th style="padding:0.75rem 1rem;">Endpoint</th>
                            <th style="padding:0.75rem 1rem;">Status</th>
                            <th style="padding:0.75rem 1rem; text-align:right;">Actions</th>
                        </tr>
                    </thead>
                    <tbody id="admin-server-body">
                        ${servers.map(s => `
                            <tr class="hover-row" style="background:rgba(255,255,255,0.02);">
                                <td style="padding:1rem; border-top-left-radius:8px; border-bottom-left-radius:8px;">
                                    <div style="font-weight:600;">${window.escapeHtml(s.name || '')}</div>
                                    <div style="font-size:0.7rem; color:var(--text-muted);">${window.escapeHtml(s.username || '')}</div>
                                </td>
                                <td style="padding:1rem;">
                                    <span class="badge badge-outline" style="text-transform:capitalize;">${window.escapeHtml(String(s.db_type || ''))}</span>
                                </td>
                                <td style="padding:1rem;">
                                    <code>${window.escapeHtml(s.host || '')}:${window.escapeHtml(String(s.port ?? ''))}</code>
                                </td>
                                <td style="padding:1rem;">
                                    ${s.is_active ? 
                                        '<span class="badge badge-success" style="font-size:0.65rem;"><i class="fa-solid fa-circle-check"></i> ACTIVE</span>' : 
                                        '<span class="badge badge-warning" style="font-size:0.65rem;"><i class="fa-solid fa-circle-pause"></i> PAUSED</span>'}
                                </td>
                                <td style="padding:1rem; text-align:right; border-top-right-radius:8px; border-bottom-right-radius:8px;">
                                    <div style="display:flex; justify-content:flex-end; gap:0.4rem;">
                                        ${String(s.db_type || '').toLowerCase() === 'postgres' ? `
                                        <button class="btn btn-xs btn-outline" data-action="dr-policy" data-id="${s.id}" data-name="${window.escapeHtml(s.name || '')}" title="DR policy (RPO/RTO)">
                                            <i class="fa-solid fa-shield-heart"></i>
                                        </button>` : ''}
                                        ${String(s.db_type || '').toLowerCase() === 'sqlserver' ? `
                                        <button class="btn btn-xs btn-outline" data-action="sqlserver-diagnostics" data-name="${window.escapeHtml(s.name || '')}" title="Collector diagnostics">
                                            <i class="fa-solid fa-stethoscope"></i>
                                        </button>` : ''}
                                        <button class="btn btn-xs btn-outline" data-action="test-server" data-id="${s.id}" title="Test Connection">
                                            <i class="fa-solid fa-plug"></i>
                                        </button>
                                        <button class="btn btn-xs btn-outline" data-action="patch-server-active" data-id="${s.id}" data-active="${s.is_active ? 'false' : 'true'}" title="${s.is_active ? 'Pause' : 'Resume'} Collection">
                                            <i class="fa-solid fa-${s.is_active ? 'pause' : 'play'}"></i>
                                        </button>
                                        <button class="btn btn-xs btn-outline" style="color:var(--danger); border-color:rgba(var(--danger-rgb), 0.1);" data-action="delete-server" data-id="${s.id}" title="Delete Server">
                                            <i class="fa-solid fa-trash-can"></i>
                                        </button>
                                    </div>
                                </td>
                            </tr>`).join('')}
                    </tbody>
                </table>
            </div>
        `;

        container.addEventListener('click', async (e) => {
            const btn = e.target.closest('button');
            if (!btn) return;
            const { action, id, active, name } = btn.dataset;
            if (!action) return;

            if (action === 'sqlserver-diagnostics') {
                const mod = await import('./admin_sqlserver_diagnostics.js');
                if (typeof mod.openSqlServerDiagnosticsForInstance === 'function') {
                    mod.openSqlServerDiagnosticsForInstance(name || '');
                }
                return;
            }
            if (!id) return;

            if (action === 'test-server') {
                await window.testAdminServer(id);
            } else if (action === 'dr-policy') {
                await window.showAdminDRPolicy(id, btn.dataset.name || '');
            } else if (action === 'patch-server-active') {
                try {
                    const resp = await window.apiClient.authenticatedFetch(`/api/admin/servers/${id}`, {
                        method: 'PATCH',
                        body: JSON.stringify({ is_active: active === 'true' })
                    });
                    if (!resp.ok) throw new Error('Failed to toggle active state');
                    window.loadAdminServers();
                } catch (err) { alert(err.message); }
            } else if (action === 'delete-server') {
                if (!confirm('Are you sure you want to delete this server? Historical metrics will be preserved but collection will stop.')) return;
                try {
                    const resp = await window.apiClient.authenticatedFetch(`/api/admin/servers/${id}`, { method: 'DELETE' });
                    if (!resp.ok) throw new Error('Failed to delete server');
                    window.loadAdminServers();
                } catch (err) { alert(err.message); }
            }
        });

    } catch (error) {
        container.innerHTML = `<div class="alert alert-danger">Failed to load servers: ${window.escapeHtml(error.message)}</div>`;
    }
};

/**
 * showAddServerForm - Shows the form to add a new monitoring target
 */
window.showAddServerForm = function() {
    const slot = document.getElementById('admin-server-add-slot');
    const msg = document.getElementById('admin-server-msg');
    if (msg) msg.innerHTML = '';
    if (!slot || document.getElementById('admin-add-server-form')) return;

    slot.innerHTML = `
        <style>
            #admin-add-server-grid { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:0.5rem 0.9rem; align-items:start; }
            #admin-add-server-form .srv-fld label { display:block; font-size:0.68rem; font-weight:600; text-transform:uppercase; letter-spacing:0.04em; color:var(--text-muted); margin-bottom:0.25rem; }
            #admin-add-server-form .srv-fld .custom-input,
            #admin-add-server-form .srv-fld .custom-select { width:100%; min-height:2rem; box-sizing:border-box; font-size:0.8rem; padding:0.3rem 0.55rem; }
            @media (max-width: 900px) {
                #admin-add-server-grid { grid-template-columns:repeat(2,minmax(0,1fr)); }
                .srv-fld-ssl, .srv-fld-db, #srv-trust-wrap, #pg-extension-guide { grid-column:1/-1 !important; }
            }
            @media (max-width: 560px) {
                #admin-add-server-grid { grid-template-columns:1fr; }
                #admin-add-server-grid .srv-fld { grid-column:1/-1 !important; }
            }
        </style>
        <div class="glass-panel" id="admin-add-server-form" style="padding:0;margin-bottom:1rem;border-radius:10px;overflow:hidden;border:1px solid var(--border-color);box-shadow:var(--shadow-sm);">
            <div class="flex-between" style="padding:0.6rem 0.9rem;border-bottom:1px solid var(--border-color);background:linear-gradient(135deg,rgba(59,130,246,0.1),rgba(139,92,246,0.06));">
                <h4 style="margin:0;font-size:0.9rem;font-weight:600;"><i class="fa-solid fa-plus text-accent"></i> New monitoring server</h4>
                <button type="button" class="btn btn-xs btn-outline" id="btn-close-add-server"><i class="fa-solid fa-xmark"></i></button>
            </div>
            <div style="padding:0.75rem 0.9rem 0.85rem;">
                <p class="text-muted" style="font-size:0.7rem;margin:0 0 0.6rem;line-height:1.4;"><i class="fa-solid fa-vault"></i> Credentials are encrypted before storage. Ensure <code>VAULT_ADDR</code> is set for production.</p>
                <div id="admin-add-server-grid">
                    <div class="srv-fld"><label for="srv-name">Name</label><input class="custom-input" id="srv-name" placeholder="Production Postgres" /></div>
                    <div class="srv-fld"><label for="srv-type">Engine</label>
                        <select class="custom-select" id="srv-type"><option value="postgres">PostgreSQL</option><option value="sqlserver">SQL Server</option></select></div>
                    <div class="srv-fld"><label for="srv-host">Host</label><input class="custom-input" id="srv-host" placeholder="localhost" /></div>
                    <div class="srv-fld"><label for="srv-port">Port</label><input class="custom-input" id="srv-port" placeholder="5432" inputmode="numeric" /></div>
                    <div class="srv-fld"><label for="srv-user">Username</label><input class="custom-input" id="srv-user" /></div>
                    <div class="srv-fld"><label for="srv-pass">Password</label><input class="custom-input" id="srv-pass" type="password" autocomplete="new-password" /></div>
                    <div class="srv-fld srv-fld-ssl" style="grid-column:span 2;"><label for="srv-ssl">SSL mode (PostgreSQL / RDS)</label>
                        <select class="custom-select" id="srv-ssl"><option value="disable" selected>disable</option><option value="require">require</option><option value="verify-full">verify-full</option></select></div>
                    <div class="srv-fld srv-fld-db"><label for="srv-db">Initial database <span class="text-muted" style="font-weight:400;">(optional)</span></label>
                        <input class="custom-input" id="srv-db" placeholder="postgres or master" /></div>
                    <div id="srv-trust-wrap" class="srv-fld" style="grid-column:1/-1;display:none;"><label style="display:flex;align-items:center;gap:0.45rem;cursor:pointer;margin:0;font-weight:500;font-size:0.8rem;">
                        <input type="checkbox" id="srv-trust-cert" style="width:1rem;height:1rem;" /> Trust server certificate (Azure SQL / MI)
                    </label></div>
                    <div id="pg-extension-guide" style="grid-column:1/-1; font-size:0.7rem; line-height:1.4; padding:0.5rem 0.65rem; border-radius:6px; margin-top:0.1rem; display:none; border:1px solid rgba(var(--accent-rgb),0.2); background:rgba(var(--accent-rgb),0.03); color:var(--text-muted);">
                        <i class="fa-solid fa-circle-info text-accent"></i> Enable <code>pg_stat_statements</code> (required) and optionally <code>pg_stat_monitor</code> via <code>shared_preload_libraries</code> for advanced query metrics.
                    </div>
                    <div id="pg-os-collector-add-slot" style="grid-column:1/-1; display:none; margin-top:0.25rem;"></div>
                </div>
                <div id="admin-add-server-err" class="alert alert-danger" style="display:none;margin-top:0.6rem;font-size:0.78rem;padding:0.4rem 0.75rem;"></div>
                <div style="margin-top:0.75rem;display:flex;align-items:center;gap:0.6rem;">
                    <button type="button" class="btn btn-sm btn-outline" id="srv-test-btn"><i class="fa-solid fa-plug-circle-check"></i> Test connection</button>
                    <span id="srv-test-msg" style="font-size:0.78rem;font-weight:500;"></span>
                    <button type="button" class="btn btn-sm btn-accent" id="srv-save-btn" disabled title="Test connection first"><i class="fa-solid fa-floppy-disk"></i> Save server</button>
                </div>
            </div>
        </div>
    `;

    document.getElementById('btn-close-add-server').addEventListener('click', () => slot.innerHTML = '');
    
    const saveBtn = document.getElementById('srv-save-btn');
    const testMsg = document.getElementById('srv-test-msg');
    const typeSel = document.getElementById('srv-type');
    const trustWrap = document.getElementById('srv-trust-wrap');
    const pgGuide = document.getElementById('pg-extension-guide');
    const distNote = document.getElementById('admin-dist-note');

    const pgOsSlot = document.getElementById('pg-os-collector-add-slot');
    const syncTrustUI = () => {
        if (!typeSel) return;
        const isSqlServer = typeSel.value === 'sqlserver';
        const isPostgres = typeSel.value === 'postgres';
        if (trustWrap) trustWrap.style.display = isSqlServer ? 'block' : 'none';
        if (pgGuide) pgGuide.style.display = isPostgres ? 'block' : 'none';
        if (distNote) distNote.style.display = isSqlServer ? 'block' : 'none';
        if (pgOsSlot) {
            if (isPostgres && window.mountOsCollectorSetupPanel) {
                pgOsSlot.style.display = 'block';
                const name = document.getElementById('srv-name')?.value.trim() || '';
                window.mountOsCollectorSetupPanel(pgOsSlot, {
                    instanceName: name,
                    statusId: 'os-collector-admin-add-status',
                    compact: true
                });
            } else {
                pgOsSlot.style.display = 'none';
                pgOsSlot.innerHTML = '';
            }
        }
    };
    typeSel?.addEventListener('change', syncTrustUI);
    document.getElementById('srv-name')?.addEventListener('input', syncTrustUI);
    syncTrustUI();

    // Disable save if any input changes
    ['srv-name', 'srv-type', 'srv-host', 'srv-port', 'srv-user', 'srv-pass', 'srv-db', 'srv-ssl', 'srv-trust-cert'].forEach(id => {
        document.getElementById(id).addEventListener('input', () => {
            saveBtn.disabled = true;
            saveBtn.title = 'Test connection first';
            testMsg.textContent = '';
        });
    });

    document.getElementById('srv-test-btn').addEventListener('click', async () => {
        const errEl = document.getElementById('admin-add-server-err');
        errEl.style.display = 'none';
        testMsg.textContent = 'Testing...';
        testMsg.style.color = 'var(--text-muted)';
        
        const dbType = document.getElementById('srv-type').value;
        let portStr = document.getElementById('srv-port').value.trim();
        if (!portStr) portStr = (dbType === 'postgres' ? '5432' : '1433');

        const payload = {
            name: document.getElementById('srv-name').value.trim(),
            db_type: dbType,
            host: document.getElementById('srv-host').value.trim(),
            port: parseInt(portStr, 10) || 0,
            username: document.getElementById('srv-user').value.trim(),
            password: document.getElementById('srv-pass').value,
            database: document.getElementById('srv-db').value.trim(),
            ssl_mode: document.getElementById('srv-ssl').value,
            trust_server_certificate: !!document.getElementById('srv-trust-cert').checked
        };

        const vErr = window.validateMonitoringServerPayload(payload);
        if (vErr) {
            testMsg.textContent = '✘ Validation failed';
            testMsg.style.color = 'var(--danger)';
            errEl.textContent = vErr;
            errEl.style.display = 'block';
            return;
        }

        try {
            const resp = await window.apiClient.authenticatedFetch('/api/admin/servers/test-draft', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            const j = await resp.json().catch(() => ({}));
            if (!resp.ok || !j.success) throw new Error(j.error || 'Connection failed');
            
            testMsg.textContent = '✔ Connection OK';
            testMsg.style.color = 'var(--success)';
            saveBtn.disabled = false;
            saveBtn.title = 'Connection verified';
        } catch (e) {
            testMsg.textContent = '✘ Failed';
            testMsg.style.color = 'var(--danger)';
            errEl.textContent = e.message;
            errEl.style.display = 'block';
        }
    });

    saveBtn.addEventListener('click', async () => {
        const errEl = document.getElementById('admin-add-server-err');
        errEl.style.display = 'none';
        saveBtn.disabled = true;
        saveBtn.textContent = 'Saving...';

        const dbType = document.getElementById('srv-type').value;
        let portStr = document.getElementById('srv-port').value.trim();
        if (!portStr) portStr = (dbType === 'postgres' ? '5432' : '1433');

        const payload = {
            name: document.getElementById('srv-name').value.trim(),
            db_type: dbType,
            host: document.getElementById('srv-host').value.trim(),
            port: parseInt(portStr, 10) || 0,
            username: document.getElementById('srv-user').value.trim(),
            password: document.getElementById('srv-pass').value,
            database: document.getElementById('srv-db').value.trim(),
            ssl_mode: document.getElementById('srv-ssl').value,
            trust_server_certificate: !!document.getElementById('srv-trust-cert').checked
        };

        const vErr = window.validateMonitoringServerPayload(payload);
        if (vErr) {
            errEl.textContent = vErr;
            errEl.style.display = 'block';
            saveBtn.disabled = false;
            saveBtn.innerHTML = '<i class="fa-solid fa-floppy-disk"></i> Save server';
            return;
        }

        try {
            const resp = await window.apiClient.authenticatedFetch('/api/admin/servers', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            const j = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(j.error || 'Save failed');
            
            slot.innerHTML = '';
            const mainMsg = document.getElementById('admin-server-msg');
            if (mainMsg) mainMsg.innerHTML = `<div class="alert alert-success">Server saved successfully.</div>`;
            window.loadAdminServers();
            
            // Refresh config so sidebar dropdown and estate view reflect the new server immediately.
            if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
                await window.apiClient.fetchConfig();
                if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
                    window.router.populateInstanceDropdown();
                }
                
                // Full boot ensures Global Estate and all components reflect new inventory.
                if (typeof window.boot === 'function') {
                    window.boot();
                } else if (window.GlobalEstate && typeof window.GlobalEstate.render === 'function') {
                    window.GlobalEstate.render();
                }
            }
        } catch (e) {
            errEl.textContent = e.message;
            errEl.style.display = 'block';
            saveBtn.disabled = false;
            saveBtn.innerHTML = '<i class="fa-solid fa-floppy-disk"></i> Save server';
        }
    });
};

/**
 * loadAdminUsers - Fetches and renders the list of users
 */
window.loadAdminUsers = async function() {
    const container = document.getElementById('admin-user-list');
    if (!container) return;

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/users');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        const users = data.users || [];

        container.innerHTML = `
            <div class="table-responsive">
                <table class="data-table" style="font-size:0.85rem; border-collapse: separate; border-spacing: 0 0.5rem;">
                    <thead>
                        <tr style="text-align:left; font-size:0.75rem; text-transform:uppercase; color:var(--text-muted);">
                            <th style="padding:0.75rem 1rem;">ID</th>
                            <th style="padding:0.75rem 1rem;">User Profile</th>
                            <th style="padding:0.75rem 1rem;">Role</th>
                            <th style="padding:0.75rem 1rem; text-align:right;">Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${users.map(u => `
                            <tr class="hover-row" style="background:rgba(255,255,255,0.02);">
                                <td style="padding:1rem; border-top-left-radius:8px; border-bottom-left-radius:8px; color:var(--text-muted); font-family:monospace;">${u.user_id}</td>
                                <td style="padding:1rem;">
                                    <div style="display:flex; align-items:center; gap:0.75rem;">
                                        <div style="width:2rem; height:2rem; border-radius:50%; background:var(--accent); display:flex; align-items:center; justify-content:center; color:white; font-weight:bold;">
                                            ${(u.username||'U').charAt(0).toUpperCase()}
                                        </div>
                                        <div style="font-weight:600;">${window.escapeHtml(u.username)}</div>
                                    </div>
                                </td>
                                <td style="padding:1rem;">
                                    <span class="badge ${u.role === 'admin' ? 'badge-danger' : 'badge-info'}" style="text-transform:uppercase; font-size:0.65rem;">
                                        ${window.escapeHtml(u.role)}
                                    </span>
                                </td>
                                <td style="padding:1rem; text-align:right; border-top-right-radius:8px; border-bottom-right-radius:8px;">
                                    ${u.user_id !== 1 ? `
                                        <button class="btn btn-xs btn-outline btn-delete-user" data-id="${u.user_id}" style="color:var(--danger); border-color:rgba(var(--danger-rgb), 0.2);">
                                            <i class="fa-solid fa-user-minus"></i> Remove
                                        </button>
                                    ` : '<span class="text-muted" style="font-size:0.7rem; font-style:italic;">Protected</span>'}
                                </td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;

        container.querySelectorAll('.btn-delete-user').forEach(btn => {
            btn.addEventListener('click', (e) => window.deleteUser(e.currentTarget.dataset.id));
        });
    } catch (error) {
        container.innerHTML = `<div class="alert alert-danger">Failed to load users: ${window.escapeHtml(error.message)}</div>`;
    }
};

/**
 * Global Admin helpers (test, delete, etc.)
 */
window.testAdminServer = async function(id) {
    const msg = document.getElementById('admin-server-msg');
    if (msg) msg.innerHTML = `<div class="alert alert-info">Testing connection...</div>`;
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${id}/test`, { method: 'POST' });
        const j = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(j.error || 'Connection failed');
        if (msg) msg.innerHTML = `<div class="alert alert-success">Connection OK.</div>`;
    } catch (e) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">Test failed: ${window.escapeHtml(e.message)}</div>`;
    }
};

window.showAdminDRPolicy = async function (serverId, instanceName) {
    const name = instanceName || serverId;
    let policy = {
        rpo_backup_hours: 24,
        rpo_archive_minutes: 5,
        rpo_replay_seconds: 60,
        max_slot_retention_gb: 10,
        rto_failover_minutes: null
    };
    try {
        const r = await window.apiClient.authenticatedFetch(
            `/api/postgres/dr-policy?instance=${encodeURIComponent(name)}`
        );
        if (r.ok) policy = await r.json();
    } catch (_) { /* defaults */ }

    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay active';
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:9999;display:flex;align-items:center;justify-content:center;padding:1rem;';
    overlay.innerHTML = `
        <div class="glass-panel" style="max-width:420px;width:100%;padding:1rem;">
            <h3 style="margin:0 0 0.75rem;font-size:0.95rem;"><i class="fa-solid fa-shield-heart text-accent"></i> DR policy — ${window.escapeHtml(name)}</h3>
            <form id="admin-dr-policy-form" style="display:grid;gap:0.5rem;font-size:0.8rem;">
                <label>RPO backup (hours)<input type="number" min="1" class="custom-input" name="rpo_backup_hours" value="${policy.rpo_backup_hours ?? 24}" /></label>
                <label>RPO archive (minutes)<input type="number" min="1" class="custom-input" name="rpo_archive_minutes" value="${policy.rpo_archive_minutes ?? 5}" /></label>
                <label>RPO replay lag (seconds)<input type="number" min="1" class="custom-input" name="rpo_replay_seconds" value="${policy.rpo_replay_seconds ?? 60}" /></label>
                <label>Max slot retention (GB)<input type="number" min="0.1" step="0.1" class="custom-input" name="max_slot_retention_gb" value="${policy.max_slot_retention_gb ?? 10}" /></label>
                <label>RTO failover (minutes, optional)<input type="number" min="1" class="custom-input" name="rto_failover_minutes" value="${policy.rto_failover_minutes ?? ''}" placeholder="optional" /></label>
            </form>
            <div id="admin-dr-policy-err" class="alert alert-danger" style="display:none;margin-top:0.5rem;font-size:0.75rem;"></div>
            <div style="display:flex;justify-content:flex-end;gap:0.5rem;margin-top:0.75rem;">
                <button type="button" class="btn btn-sm btn-outline" id="admin-dr-policy-cancel">Cancel</button>
                <button type="button" class="btn btn-sm btn-accent" id="admin-dr-policy-save">Save</button>
            </div>
        </div>`;
    document.body.appendChild(overlay);
    overlay.querySelector('#admin-dr-policy-cancel')?.addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
    overlay.querySelector('#admin-dr-policy-save')?.addEventListener('click', async () => {
        const form = overlay.querySelector('#admin-dr-policy-form');
        const errEl = overlay.querySelector('#admin-dr-policy-err');
        const body = {
            rpo_backup_hours: parseInt(form.rpo_backup_hours.value, 10) || 24,
            rpo_archive_minutes: parseInt(form.rpo_archive_minutes.value, 10) || 5,
            rpo_replay_seconds: parseInt(form.rpo_replay_seconds.value, 10) || 60,
            max_slot_retention_gb: parseFloat(form.max_slot_retention_gb.value) || 10,
        };
        const rto = form.rto_failover_minutes.value.trim();
        if (rto) body.rto_failover_minutes = parseInt(rto, 10);
        try {
            const resp = await window.apiClient.authenticatedFetch(
                `/api/postgres/dr-policy?instance=${encodeURIComponent(name)}`,
                { method: 'PUT', body: JSON.stringify(body) }
            );
            if (!resp.ok) {
                const j = await resp.json().catch(() => ({}));
                throw new Error(j.error || 'Save failed');
            }
            overlay.remove();
            const msg = document.getElementById('admin-server-msg');
            if (msg) msg.innerHTML = '<div class="alert alert-success">DR policy saved.</div>';
        } catch (e) {
            if (errEl) {
                errEl.style.display = 'block';
                errEl.textContent = e.message;
            }
        }
    });
};

window.deleteUser = async function(id) {
    if (!confirm('Delete user?')) return;
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/users/${id}`, { method: 'DELETE' });
        if (!response.ok) {
            const j = await response.json().catch(() => ({}));
            throw new Error(j.error || `HTTP ${response.status}`);
        }
        window.loadAdminUsers();
    } catch (e) { alert('Delete failed: ' + e.message); }
};
