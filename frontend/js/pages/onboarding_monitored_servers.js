/**
 * Dedicated onboarding: add SQL Server / PostgreSQL monitoring targets (admin-only).
 * Uses /api/admin/servers — same as Admin panel but full-page flow after Timescale + admin setup.
 */

window.OnboardingMonitoredServersView = async function() {
    const outlet = window.routerOutlet;
    if (!outlet) return;

    outlet.innerHTML = `
        <div class="page-view active" style="padding:2rem; max-width:1080px; margin:0 auto;">
            <div class="flex-between" style="margin-bottom:2rem; flex-wrap:wrap; gap:1.5rem; border-bottom:1px solid var(--border-color); padding-bottom:1.5rem;">
                <div>
                    <h1 style="font-size:1.75rem; font-weight:700; margin-bottom:0.5rem; letter-spacing:-0.02em;">
                        <i class="fa-solid fa-server text-accent"></i> Add monitored databases
                    </h1>
                    <p class="text-muted" style="font-size:0.95rem; max-width:56rem; line-height:1.6;">
                        Register each SQL Server or PostgreSQL instance you want to monitor. Credentials are encrypted in TimescaleDB. 
                        You can return to this flow anytime from <strong>Admin → Monitoring Servers</strong>.
                    </p>
                </div>
                <div style="display:flex; gap:0.75rem; flex-wrap:wrap;">
                    <button type="button" class="btn btn-outline" data-action="navigate" data-route="global"><i class="fa-solid fa-globe"></i> Global estate</button>
                    <button type="button" class="btn btn-outline" data-action="navigate" data-route="admin"><i class="fa-solid fa-shield-halved"></i> Admin</button>
                </div>
            </div>

            <div class="glass-panel" style="padding:1.5rem; border-radius:16px; box-shadow: var(--shadow-md);">
                <div class="flex-between" style="margin-bottom:1.5rem; flex-wrap:wrap; gap:1rem;">
                    <h3 style="font-size:1.1rem; margin:0; font-weight:600;"><i class="fa-solid fa-database text-accent"></i> Registered Targets</h3>
                    <button class="btn btn-accent" type="button" data-action="call" data-fn="onbShowAddForm" style="padding:0.6rem 1.25rem;">
                        <i class="fa-solid fa-plus"></i> Add New Server
                    </button>
                </div>
                
                <div id="onb-msg"></div>
                <div id="onb-add-slot"></div>
                
                <div id="onb-list" style="margin-top:1rem;">
                    <div class="text-center" style="padding:4rem;">
                        <div class="spinner" style="margin:0 auto 1.5rem;"></div>
                        <div class="text-muted">Loading your servers…</div>
                    </div>
                </div>
            </div>
        </div>`;

    await window.onbLoadServers();
};

window.onbShowAddForm = function() {
    const slot = document.getElementById('onb-add-slot');
    const msg = document.getElementById('onb-msg');
    if (msg) msg.innerHTML = '';
    if (!slot || document.getElementById('onb-add-form')) {
        document.getElementById('onb-add-form')?.scrollIntoView({ behavior: 'smooth' });
        return;
    }

    slot.innerHTML = `
        <div class="glass-panel" id="onb-add-form" style="padding:0; margin-bottom:2rem; border-radius:12px; overflow:hidden; border:1px solid var(--border-color); box-shadow: var(--shadow-lg);">
            <div class="flex-between" style="padding:1rem 1.25rem; border-bottom:1px solid var(--border-color); background:linear-gradient(135deg,rgba(59,130,246,0.1),rgba(139,92,246,0.06));">
                <h4 style="margin:0; font-size:1rem; font-weight:600;"><i class="fa-solid fa-plus text-accent"></i> New monitoring server</h4>
                <button type="button" class="btn btn-xs btn-outline" data-action="close-id" data-target="onb-add-form" style="border:none; font-size:1.2rem;">&times;</button>
            </div>
            <div style="padding:1.5rem;">
                <p class="text-muted" style="font-size:0.8rem; margin:0 0 1.25rem; line-height:1.5;">
                    <i class="fa-solid fa-vault text-accent"></i> Credentials are encrypted before storage. Ensure <code>VAULT_ADDR</code> is configured on the API for KMS integration.
                </p>
                <div id="onb-add-grid" style="display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:1.25rem 1.5rem; align-items:start;">
                    <div class="onb-fld"><label for="onb-name">Display Name</label><input class="custom-input" id="onb-name" placeholder="Production Cluster" /></div>
                    <div class="onb-fld"><label for="onb-type">Engine Type</label>
                        <select class="custom-select" id="onb-type" style="width:100%; min-height:2.6rem;"><option value="postgres">PostgreSQL</option><option value="sqlserver">SQL Server</option></select></div>
                    <div class="onb-fld"><label for="onb-host">Hostname / IP</label><input class="custom-input" id="onb-host" placeholder="db.example.com" /></div>
                    <div class="onb-fld"><label for="onb-port">Port</label><input class="custom-input" id="onb-port" placeholder="5432" inputmode="numeric" /></div>
                    <div class="onb-fld"><label for="onb-user">Username</label><input class="custom-input" id="onb-user" placeholder="monitor_user" /></div>
                    <div class="onb-fld"><label for="onb-pass">Password</label><input class="custom-input" id="onb-pass" type="password" autocomplete="new-password" placeholder="••••••••" /></div>
                    <div class="onb-fld" style="grid-column:1/-1;"><label for="onb-ssl">SSL Connectivity</label>
                        <select class="custom-select" id="onb-ssl" style="width:100%; min-height:2.6rem;"><option value="require">require (default)</option><option value="disable">disable</option><option value="verify-full">verify-full</option></select></div>
                    <div class="onb-fld" style="grid-column:1/-1;"><label for="onb-database">Initial Database / Catalog <span class="text-muted" style="font-weight:400;">(optional)</span></label>
                        <input class="custom-input" id="onb-database" placeholder="postgres or master" />
                        <span class="text-muted" style="font-size:0.7rem; display:block; margin-top:0.4rem;">Typically <code>postgres</code> for PG, <code>master</code> for SQL Server.</span></div>
                    <div id="onb-trust-wrap" class="onb-fld" style="grid-column:1/-1; display:none;">
                        <label style="display:flex; align-items:center; gap:0.6rem; cursor:pointer; margin:0; font-weight:500; text-transform:none; letter-spacing:normal; color:inherit;">
                            <input type="checkbox" id="onb-trust-cert" style="width:1.1rem; height:1.1rem;" /> Trust server certificate (Required for some Azure/AWS instances)
                        </label>
                    </div>
                </div>
                <style>
                    #onb-add-form .onb-fld label { display:block; font-size:0.75rem; font-weight:600; text-transform:uppercase; letter-spacing:0.05em; color:var(--text-muted); margin-bottom:0.4rem; }
                    #onb-add-form .onb-fld .custom-input, #onb-add-form .onb-fld .custom-select { width:100%; min-height:2.6rem; box-sizing:border-box; }
                </style>
                <div id="onb-add-err" style="display:none; margin-top:1.25rem;" class="alert alert-danger"></div>
                <div style="display:flex; flex-wrap:wrap; align-items:center; gap:1rem; margin-top:1.5rem; padding-top:1.25rem; border-top:1px solid var(--border-color);">
                    <button type="button" class="btn btn-outline" data-action="call" data-fn="onbTestAddDraft" id="onb-test-btn" style="padding:0.6rem 1.25rem;"><i class="fa-solid fa-plug-circle-check"></i> Test connection</button>
                    <span id="onb-test-inline-msg" style="font-size:0.85rem; font-weight:600;"></span>
                    <button type="button" class="btn btn-accent" data-action="call" data-fn="onbSubmitAdd" id="onb-save-btn" disabled title="Test connection first" style="padding:0.6rem 2rem; margin-left:auto;"><i class="fa-solid fa-floppy-disk"></i> Save & Register</button>
                </div>
            </div>
        </div>`;

    // Wire up change listeners
    ['onb-name', 'onb-type', 'onb-host', 'onb-port', 'onb-user', 'onb-pass', 'onb-ssl', 'onb-database', 'onb-trust-cert'].forEach(id => {
        document.getElementById(id)?.addEventListener('input', () => {
            const saveBtn = document.getElementById('onb-save-btn');
            if (saveBtn) {
                saveBtn.disabled = true;
                saveBtn.title = 'Test connection first';
            }
            const inlineMsg = document.getElementById('onb-test-inline-msg');
            if (inlineMsg) inlineMsg.textContent = '';
        });
    });

    const typeSel = document.getElementById('onb-type');
    const trustWrap = document.getElementById('onb-trust-wrap');
    const syncOnbTrust = () => {
        if (!trustWrap || !typeSel) return;
        trustWrap.style.display = typeSel.value === 'sqlserver' ? 'block' : 'none';
    };
    typeSel?.addEventListener('change', syncOnbTrust);
    syncOnbTrust();
};

window.onbTestAddDraft = async function() {
    const errEl = document.getElementById('onb-add-err');
    if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
    
    const name = document.getElementById('onb-name')?.value?.trim() || '';
    const dbType = document.getElementById('onb-type')?.value?.trim() || 'postgres';
    const host = document.getElementById('onb-host')?.value?.trim() || '';
    
    let portStr = document.getElementById('onb-port')?.value?.trim() || '';
    if (portStr === '') {
        portStr = dbType === 'postgres' ? '5432' : '1433';
    }
    const port = parseInt(portStr, 10) || 0;
    
    const username = document.getElementById('onb-user')?.value?.trim() || '';
    const password = document.getElementById('onb-pass')?.value || '';
    const ssl_mode = document.getElementById('onb-ssl')?.value?.trim() || 'require';
    const database = document.getElementById('onb-database')?.value?.trim() || '';
    const trust_server_certificate = !!document.getElementById('onb-trust-cert')?.checked;
    
    const payload = { name, db_type: dbType, host, port, username, password, database, ssl_mode, trust_server_certificate };
    
    const vErr = window.validateMonitoringServerPayload(payload);
    if (vErr) {
        if (errEl) {
            errEl.textContent = vErr;
            errEl.style.display = 'block';
        }
        return;
    }
    
    var inlineMsg = document.getElementById('onb-test-inline-msg');
    if (inlineMsg) { inlineMsg.textContent = 'Testing connection…'; inlineMsg.style.color = 'var(--text-muted)'; }
    
    const testBtn = document.getElementById('onb-test-btn');
    const saveBtn = document.getElementById('onb-save-btn');
    if (testBtn) {
        testBtn.disabled = true;
        testBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Testing…';
    }

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/servers/test-draft', {
            method: 'POST',
            body: JSON.stringify(payload)
        });
        const j = await response.json().catch(() => ({}));
        if (!response.ok || !j.success) throw new Error(j.error || `HTTP ${response.status}`);
        
        if (inlineMsg) { 
            inlineMsg.textContent = '\u2714 Connection verified'; 
            inlineMsg.style.color = 'var(--success)'; 
        }
        if (saveBtn) {
            saveBtn.disabled = false;
            saveBtn.title = 'Verified successfully';
        }
    } catch (e) {
        if (errEl) {
            errEl.textContent = 'Connection failed: ' + e.message;
            errEl.style.display = 'block';
        }
        if (inlineMsg) { 
            inlineMsg.textContent = '\u2716 Failed'; 
            inlineMsg.style.color = 'var(--danger)'; 
        }
    } finally {
        if (testBtn) {
            testBtn.disabled = false;
            testBtn.innerHTML = '<i class="fa-solid fa-plug-circle-check"></i> Test connection';
        }
    }
};

window.onbSubmitAdd = async function() {
    const errEl = document.getElementById('onb-add-err');
    if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
    
    const saveBtn = document.getElementById('onb-save-btn');
    if (saveBtn) {
        saveBtn.disabled = true;
        saveBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Saving…';
    }

    const name = document.getElementById('onb-name')?.value?.trim() || '';
    const dbType = document.getElementById('onb-type')?.value?.trim() || 'postgres';
    const host = document.getElementById('onb-host')?.value?.trim() || '';
    
    let portStr = document.getElementById('onb-port')?.value?.trim() || '';
    if (portStr === '') {
        portStr = dbType === 'postgres' ? '5432' : '1433';
    }
    const port = parseInt(portStr, 10) || 0;
    
    const username = document.getElementById('onb-user')?.value?.trim() || '';
    const password = document.getElementById('onb-pass')?.value || '';
    const ssl_mode = document.getElementById('onb-ssl')?.value?.trim() || 'require';
    const database = document.getElementById('onb-database')?.value?.trim() || '';
    const trust_server_certificate = !!document.getElementById('onb-trust-cert')?.checked;
    
    const payload = { name, db_type: dbType, host, port, username, password, ssl_mode, database, trust_server_certificate };
    
    const vErr = window.validateMonitoringServerPayload(payload);
    if (vErr) {
        if (errEl) {
            errEl.textContent = vErr;
            errEl.style.display = 'block';
        }
        if (saveBtn) {
            saveBtn.disabled = false;
            saveBtn.innerHTML = '<i class="fa-solid fa-floppy-disk"></i> Save & Register';
        }
        return;
    }

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/servers', {
            method: 'POST',
            body: JSON.stringify(payload)
        });
        if (!response.ok) {
            const j = await response.json().catch(() => ({}));
            throw new Error(j.error || `HTTP ${response.status}`);
        }
        
        document.getElementById('onb-add-form')?.remove();
        const msg = document.getElementById('onb-msg');
        if (msg) {
            msg.innerHTML = `<div class="alert alert-success" style="padding:1rem;">
                <i class="fa-solid fa-circle-check"></i> <strong>Server registered!</strong> Collection will start immediately. 
                View it in the <a href="#" data-action="navigate" data-route="global" style="color:inherit; text-decoration:underline;">Global Estate Overview</a>.
            </div>`;
        }
        await window.onbLoadServers();
        if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
            await window.apiClient.fetchConfig();
        }
    } catch (e) {
        if (errEl) {
            errEl.textContent = e.message;
            errEl.style.display = 'block';
        }
        if (saveBtn) {
            saveBtn.disabled = false;
            saveBtn.innerHTML = '<i class="fa-solid fa-floppy-disk"></i> Save & Register';
        }
    }
};

window.onbLoadServers = async function() {
    const container = document.getElementById('onb-list');
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
            container.innerHTML = '<div class="text-center text-muted" style="padding:3rem; border:2px dashed var(--border-color); border-radius:12px;">No servers registered yet. Use <strong>Add New Server</strong> to begin monitoring.</div>';
            return;
        }

        container.innerHTML = `
            <div class="table-responsive" style="border-radius:12px; border:1px solid var(--border-color); overflow:hidden;">
                <table class="data-table" style="font-size:0.85rem; border-collapse: separate; border-spacing: 0;">
                    <thead>
                        <tr style="background:rgba(0,0,0,0.02); text-align:left; font-size:0.75rem; text-transform:uppercase; color:var(--text-muted);">
                            <th style="padding:1rem;">Instance Name</th>
                            <th style="padding:1rem;">Engine</th>
                            <th style="padding:1rem;">Endpoint</th>
                            <th style="padding:1rem;">Status</th>
                            <th style="padding:1rem; text-align:right;">Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${servers.map(s => {
                            const sid = String(s.id || '').replace(/'/g, '');
                            return `
                            <tr class="hover-row" style="border-top:1px solid var(--border-color);">
                                <td style="padding:1rem;">
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
                                <td style="padding:1rem; text-align:right;">
                                    <div style="display:flex; justify-content:flex-end; gap:0.5rem;">
                                        <button type="button" class="btn btn-xs btn-outline" data-action="call" data-fn="onbTest" data-id="${window.escapeHtml(sid)}">Test</button>
                                        <button type="button" class="btn btn-xs btn-outline" data-action="patch-server-active" data-id="${window.escapeHtml(sid)}" data-active="${s.is_active ? 'false' : 'true'}"><i class="fa-solid fa-${s.is_active ? 'pause' : 'play'}"></i></button>
                                        <button type="button" class="btn btn-xs btn-outline" style="color:var(--danger); border-color:rgba(var(--danger-rgb),0.2);" data-action="delete-server" data-id="${window.escapeHtml(sid)}"><i class="fa-solid fa-trash-can"></i></button>
                                    </div>
                                </td>
                            </tr>`;
                        }).join('')}
                    </tbody>
                </table>
            </div>
            <div style="margin-top:2rem; text-align:center;">
                <button type="button" class="btn btn-lg btn-accent" data-action="call" data-fn="onbDone" style="padding:0.75rem 2.5rem; font-weight:600; border-radius:30px; box-shadow: var(--shadow-sm);">
                    <i class="fa-solid fa-circle-check"></i> Finish Setup & Go to Dashboard
                </button>
            </div>`;
    } catch (e) {
        container.innerHTML = `<div class="alert alert-danger" style="margin-top:1rem;">${window.escapeHtml(e.message || String(e))}</div>`;
    }
};

window.onbTest = async function(id) {
    const msg = document.getElementById('onb-msg');
    if (msg) msg.innerHTML = '<div class="alert alert-info"><i class="fa-solid fa-spinner fa-spin"></i> Testing connection…</div>';
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}/test`, { method: 'POST' });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || 'Connection failed');
        }
        if (msg) msg.innerHTML = '<div class="alert alert-success"><i class="fa-solid fa-check"></i> Connection verified successfully.</div>';
        window.onbLoadServers();
    } catch (e) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-triangle-exclamation"></i> ${window.escapeHtml(e.message || String(e))}</div>`;
    }
};

window.onbDone = async function() {
    if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
        await window.apiClient.fetchConfig();
        if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
            window.router.populateInstanceDropdown();
        }
    }
    window.appNavigate('global');
};
