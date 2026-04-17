/**
 * Dedicated onboarding: add SQL Server / PostgreSQL monitoring targets (admin-only).
 * Uses /api/admin/servers — same as Admin panel but full-page flow after Timescale + admin setup.
 */
window.OnboardingMonitoredServersView = async function() {
    const outlet = window.routerOutlet;
    if (!outlet) return;

    outlet.innerHTML = `
        <div class="page-view active" style="padding:1.5rem;max-width:960px;margin:0 auto;">
            <div class="page-title flex-between" style="flex-wrap:wrap;gap:0.75rem;">
                <div>
                    <h1 style="font-size:1.35rem;margin-bottom:0.25rem;"><i class="fa-solid fa-server text-accent"></i> Add monitored databases</h1>
                    <p class="text-muted" style="font-size:0.85rem;max-width:52rem;">Register each SQL Server or PostgreSQL instance you want to monitor. Credentials are encrypted in TimescaleDB; only administrators can add or change them. You can return to this flow anytime from <strong>Admin → Monitoring Servers</strong>.</p>
                </div>
                <div style="display:flex;gap:0.5rem;flex-wrap:wrap;">
                    <button type="button" class="btn btn-sm btn-outline" data-action="navigate" data-route="global"><i class="fa-solid fa-globe"></i> Global estate</button>
                    <button type="button" class="btn btn-sm btn-outline" data-action="navigate" data-route="admin"><i class="fa-solid fa-shield-halved"></i> Admin</button>
                </div>
            </div>
            <div class="glass-panel" style="padding:1rem;margin-bottom:1rem;">
                <div class="flex-between" style="margin-bottom:1rem;flex-wrap:wrap;gap:0.5rem;">
                    <h3 style="font-size:0.9rem;margin:0;"><i class="fa-solid fa-database text-accent"></i> Monitoring targets</h3>
                    <button class="btn btn-sm btn-accent" type="button" data-action="call" data-fn="onbShowAddForm"><i class="fa-solid fa-plus"></i> Add server</button>
                </div>
                <div id="onb-msg"></div>
                <div id="onb-add-slot"></div>
                <div id="onb-list"><div class="text-center text-muted" style="padding:2rem;">Loading…</div></div>
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
        <div class="glass-panel" id="onb-add-form" style="padding:0;margin-bottom:1rem;border-radius:12px;overflow:hidden;border:1px solid var(--border-color);">
            <div class="flex-between" style="padding:0.85rem 1rem;border-bottom:1px solid var(--border-color);background:linear-gradient(135deg,rgba(59,130,246,0.1),rgba(139,92,246,0.06));">
                <h4 style="margin:0;font-size:0.9rem;font-weight:600;"><i class="fa-solid fa-plus text-accent"></i> New monitoring server</h4>
                <button type="button" class="btn btn-xs btn-outline" data-action="close-id" data-target="onb-add-form"><i class="fa-solid fa-xmark"></i></button>
            </div>
            <div style="padding:1rem 1rem 1.1rem;">
                <p class="text-muted" style="font-size:0.75rem;margin:0 0 0.85rem;line-height:1.45;"><i class="fa-solid fa-vault"></i> Keys: configure <code>VAULT_ADDR</code> / Transit on the API for production KMS; credentials here are encrypted before storage.</p>
                <div id="onb-add-grid" style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem 1.2rem;align-items:start;">
                    <div class="onb-fld"><label for="onb-name">Name</label><input class="custom-input" id="onb-name" placeholder="Production Postgres" /></div>
                    <div class="onb-fld"><label for="onb-type">Engine</label>
                        <select class="custom-select" id="onb-type" style="width:100%;min-height:2.4rem;"><option value="postgres">PostgreSQL</option><option value="sqlserver">SQL Server</option></select></div>
                    <div class="onb-fld"><label for="onb-host">Host</label><input class="custom-input" id="onb-host" placeholder="endpoint.region.rds.amazonaws.com" /></div>
                    <div class="onb-fld"><label for="onb-port">Port</label><input class="custom-input" id="onb-port" placeholder="5432" inputmode="numeric" /></div>
                    <div class="onb-fld"><label for="onb-user">Username</label><input class="custom-input" id="onb-user" /></div>
                    <div class="onb-fld"><label for="onb-pass">Password</label><input class="custom-input" id="onb-pass" type="password" autocomplete="new-password" /></div>
                    <div class="onb-fld" style="grid-column:1/-1;"><label for="onb-ssl">SSL mode (PostgreSQL / RDS)</label>
                        <select class="custom-select" id="onb-ssl" style="width:100%;min-height:2.4rem;"><option value="require">require</option><option value="disable">disable</option><option value="verify-full">verify-full</option></select></div>
                    <div class="onb-fld" style="grid-column:1/-1;"><label for="onb-database">Initial database / catalog <span class="text-muted" style="font-weight:400;">(optional)</span></label>
                        <input class="custom-input" id="onb-database" placeholder="postgres or master" />
                        <span class="text-muted" style="font-size:0.68rem;display:block;margin-top:0.3rem;">RDS/Aurora: <code>postgres</code>. Azure SQL / MI: <code>master</code>; public MI often <code>3342</code>.</span></div>
                    <div id="onb-trust-wrap" class="onb-fld" style="grid-column:1/-1;display:none;"><label style="display:flex;align-items:center;gap:0.45rem;cursor:pointer;margin:0;font-weight:500;">
                        <input type="checkbox" id="onb-trust-cert" style="width:1rem;height:1rem;" /> Trust server certificate (Azure SQL / MI)
                    </label></div>
                </div>
                <style>
                    #onb-add-form .onb-fld label { display:block; font-size:0.7rem; font-weight:600; text-transform:uppercase; letter-spacing:0.04em; color:var(--text-muted); margin-bottom:0.35rem; }
                    #onb-add-form .onb-fld .custom-input, #onb-add-form .onb-fld .custom-select { width:100%; min-height:2.4rem; box-sizing:border-box; }
                </style>
                <div style="display:flex;flex-wrap:wrap;gap:0.5rem;margin-top:1rem;">
                    <button type="button" class="btn btn-sm btn-outline" data-action="call" data-fn="onbTestAddDraft"><i class="fa-solid fa-plug-circle-check"></i> Test connection</button>
                    <button type="button" class="btn btn-sm btn-accent" data-action="call" data-fn="onbSubmitAdd"><i class="fa-solid fa-floppy-disk"></i> Save</button>
                </div>
            </div>
            <div id="onb-add-err" style="display:none;margin:0 1rem 1rem;" class="alert alert-danger"></div>
        </div>`;
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
    const msg = document.getElementById('onb-msg');
    const errEl = document.getElementById('onb-add-err');
    if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
    const name = document.getElementById('onb-name')?.value?.trim() || '';
    const dbType = document.getElementById('onb-type')?.value?.trim() || '';
    const host = document.getElementById('onb-host')?.value?.trim() || '';
    const port = parseInt(document.getElementById('onb-port')?.value?.trim() || '', 10);
    const username = document.getElementById('onb-user')?.value?.trim() || '';
    const password = document.getElementById('onb-pass')?.value || '';
    const ssl_mode = document.getElementById('onb-ssl')?.value?.trim() || 'require';
    const database = document.getElementById('onb-database')?.value?.trim() || '';
    const trust_server_certificate = !!document.getElementById('onb-trust-cert')?.checked;
    const draftPayload = { name, db_type: dbType, host, port, username, password, database };
    const vErr = window.validateMonitoringServerPayload ? window.validateMonitoringServerPayload(draftPayload) : null;
    if (vErr) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">${window.escapeHtml(vErr)}</div>`;
        return;
    }
    if (msg) msg.innerHTML = '<div class="alert alert-info">Testing connection…</div>';
    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/servers/test-draft', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, db_type: dbType, host, port, username, password, ssl_mode, database, trust_server_certificate })
        });
        const j = await response.json().catch(() => ({}));
        if (!response.ok || !j.success) throw new Error(j.error || `HTTP ${response.status}`);
        if (msg) msg.innerHTML = '<div class="alert alert-success">Connection test succeeded.</div>';
    } catch (e) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">${window.escapeHtml(e.message || String(e))}</div>`;
    }
};

window.onbSubmitAdd = async function() {
    const errEl = document.getElementById('onb-add-err');
    if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
    const name = document.getElementById('onb-name')?.value?.trim() || '';
    const dbType = document.getElementById('onb-type')?.value?.trim() || '';
    const host = document.getElementById('onb-host')?.value?.trim() || '';
    const port = parseInt(document.getElementById('onb-port')?.value?.trim() || '', 10);
    const username = document.getElementById('onb-user')?.value?.trim() || '';
    const password = document.getElementById('onb-pass')?.value || '';
    const ssl_mode = document.getElementById('onb-ssl')?.value?.trim() || 'require';
    const database = document.getElementById('onb-database')?.value?.trim() || '';
    const trust_server_certificate = !!document.getElementById('onb-trust-cert')?.checked;
    const savePayload = { name, db_type: dbType, host, port, username, password, database };
    const vErr = window.validateMonitoringServerPayload ? window.validateMonitoringServerPayload(savePayload) : null;
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
            body: JSON.stringify({ name, db_type: dbType, host, port, username, password, ssl_mode, database, trust_server_certificate })
        });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        document.getElementById('onb-add-form')?.remove();
        const msg = document.getElementById('onb-msg');
        if (msg) {
            msg.innerHTML = `<div class="alert alert-success">Server saved. Click <strong>Global Estate Overview</strong> in the sidebar to see monitored servers, then select an instance for dashboards. You can add more targets here or under <strong>Admin</strong>.</div>`;
        }
        await window.onbLoadServers();
        if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
            await window.apiClient.fetchConfig();
            if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
                window.router.populateInstanceDropdown();
            }
        }
    } catch (e) {
        if (errEl) {
            errEl.textContent = e.message || String(e);
            errEl.style.display = 'block';
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
            container.innerHTML = '<div class="text-center text-muted" style="padding:2rem;">No servers yet — use <strong>Add server</strong> above.</div>';
            return;
        }
        container.innerHTML = `
            <div class="table-responsive" style="max-height:480px;overflow-y:auto;">
                <table class="data-table" style="font-size:0.75rem;">
                    <thead><tr>
                        <th>Name</th><th>Type</th><th>Host</th><th>Port</th><th>Active</th><th>Actions</th>
                    </tr></thead>
                    <tbody>
                        ${servers.map(s => {
                            const sid = String(s.id || '').replace(/'/g, '');
                            return `<tr>
                                <td><strong>${window.escapeHtml(s.name || '')}</strong></td>
                                <td><span class="badge badge-outline">${window.escapeHtml(String(s.db_type || ''))}</span></td>
                                <td><code>${window.escapeHtml(s.host || '')}</code></td>
                                <td>${window.escapeHtml(String(s.port ?? ''))}</td>
                                <td>${s.is_active ? '<span class="badge badge-success">Yes</span>' : '<span class="badge badge-warning">No</span>'}</td>
                                <td>
                                    <button type="button" class="btn btn-xs btn-outline" data-action="call" data-fn="onbTest" data-id="${window.escapeHtml(sid)}">Test</button>
                                    <button type="button" class="btn btn-xs btn-outline" data-action="call" data-fn="patchServerActive" data-id="${window.escapeHtml(sid)}" data-active="${s.is_active ? 'false' : 'true'}">Toggle</button>
                                    <button type="button" class="btn btn-xs btn-outline" style="border-color:var(--danger);color:var(--danger);" data-action="call" data-fn="deleteAdminServer" data-id="${window.escapeHtml(sid)}">Delete</button>
                                </td>
                            </tr>`;
                        }).join('')}
                    </tbody>
                </table>
            </div>
            <p style="margin-top:1rem;font-size:0.85rem;"><button type="button" class="btn btn-accent" data-action="call" data-fn="onbDone"><i class="fa-solid fa-arrow-right"></i> Continue to dashboard</button></p>`;
    } catch (e) {
        container.innerHTML = `<div class="alert alert-danger">${window.escapeHtml(e.message || String(e))}</div>`;
    }
};

window.onbTest = async function(id) {
    const msg = document.getElementById('onb-msg');
    if (msg) msg.innerHTML = '<div class="alert alert-info">Testing…</div>';
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}/test`, { method: 'POST' });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || 'failed');
        }
        if (msg) msg.innerHTML = '<div class="alert alert-success">Connection OK.</div>';
        window.onbLoadServers();
    } catch (e) {
        if (msg) msg.innerHTML = `<div class="alert alert-danger">${window.escapeHtml(e.message || String(e))}</div>`;
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
