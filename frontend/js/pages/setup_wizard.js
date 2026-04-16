/**
 * First-run wizard (reference layout): Timescale → optional Vault → run 00/01/02 on separate progress page → admin.
 */
(function() {
    var DRAFT_KEY = 'sql_optima_setup_ts_draft';

    function showMsg(elId, text, kind) {
        var el = document.getElementById(elId);
        if (!el) return;
        el.style.display = 'block';
        el.innerHTML = '';
        if (!text) {
            el.style.display = 'none';
            return;
        }
        var div = document.createElement('div');
        div.className = 'setup-ref-alert setup-ref-alert--' + (kind || 'info');
        div.textContent = text;
        el.appendChild(div);
    }

    function milestoneClass(state) {
        if (state === 'complete') return 'setup-ref-ms setup-ref-ms--done';
        if (state === 'active') return 'setup-ref-ms setup-ref-ms--active';
        return 'setup-ref-ms setup-ref-ms--pending';
    }

    function applyMilestones(m1, m2, m3) {
        var map = { 1: m1, 2: m2, 3: m3 };
        [1, 2, 3].forEach(function(n) {
            var el = document.querySelector('[data-setup-ms="' + n + '"]');
            if (el) el.className = milestoneClass(map[n]);
        });
    }

    function computeMs(showTS, showAdmin, showDone) {
        if (showDone) return { 1: 'complete', 2: 'complete', 3: 'active' };
        if (showTS) return { 1: 'active', 2: 'pending', 3: 'pending' };
        if (showAdmin) return { 1: 'complete', 2: 'active', 3: 'pending' };
        return { 1: 'complete', 2: 'complete', 3: 'active' };
    }

    function syncVaultUI() {
        var on = document.getElementById('ts-vault-toggle')?.checked;
        var wrap = document.getElementById('ts-vault-fields');
        if (wrap) wrap.style.display = on ? 'block' : 'none';
    }

    function collectDraft() {
        var portRaw = document.getElementById('ts-port')?.value?.trim() || '5432';
        var port = parseInt(portRaw, 10);
        if (!Number.isFinite(port)) port = 5432;
        return {
            host: document.getElementById('ts-host')?.value?.trim() || '',
            port: port,
            database: document.getElementById('ts-db')?.value?.trim() || '',
            username: document.getElementById('ts-user')?.value?.trim() || '',
            password: document.getElementById('ts-pass')?.value || '',
            ssl_mode: document.getElementById('ts-ssl')?.value || 'require',
            use_vault: !!document.getElementById('ts-vault-toggle')?.checked,
            vault_secret_path: document.getElementById('ts-vault-path')?.value?.trim() || ''
        };
    }

    window.SetupWizardView = async function() {
        var outlet = window.routerOutlet;
        if (!outlet) return;

        var st = {};
        try {
            var r = await fetch('/api/setup/status');
            st = r.ok ? await r.json() : {};
        } catch (e) {
            st = {};
        }

        var docker = !!st.docker_mode;
        var dedicated = !docker;

        if (st.public_setup_disabled && (st.needs_timescale || st.needs_bootstrap_admin)) {
            outlet.innerHTML =
                '<div class="page-view active setup-ref">' +
                '<div class="setup-ref-blocked">' +
                '<div class="setup-ref-card">' +
                '<div class="setup-ref-card__body" style="text-align:center;padding:2rem;">' +
                '<div class="setup-ref-hero__icon setup-ref-hero__icon--muted"><i class="fa-solid fa-lock"></i></div>' +
                '<h2 style="margin:1rem 0 0.5rem;">Setup unavailable</h2>' +
                '<p class="setup-ref-muted">Public setup is disabled. Contact your administrator.</p></div></div></div></div>';
            return;
        }

        if (docker && !st.timescale_connected) {
            outlet.innerHTML =
                '<div class="page-view active setup-ref"><div class="setup-ref__inner">' +
                '<div class="setup-ref-card setup-ref-card--warn">' +
                '<div class="setup-ref-card__head"><h3 class="setup-ref-card__title"><i class="fa-solid fa-triangle-exclamation"></i> Metrics database unreachable</h3></div>' +
                '<div class="setup-ref-card__body"><p class="setup-ref-muted">Check Docker Compose: <code>timescaledb</code> service, <code>DB_*</code> variables, and network.</p></div></div></div></div>';
            return;
        }

        var showTS = dedicated && !!st.needs_dedicated_timescale;
        var showAdmin = !!st.needs_bootstrap_admin;
        var showDone = !showTS && !showAdmin;

        var title = docker ? 'Docker quick start' : 'Initial setup';
        var subtitle = docker
            ? 'Compose-backed TimescaleDB → admin → monitored targets.'
            : 'TimescaleDB → automated schema creation → admin → targets.';
        var heroNote = 'Use a reachable <strong>PostgreSQL</strong> instance where the <strong>TimescaleDB extension</strong> can be enabled (for example the official TimescaleDB image or Postgres with <code>timescaledb</code> installed). The setup wizard will connect and create objects in the database you specify below.';

        var ms = computeMs(showTS, showAdmin, showDone);
        var m1 = milestoneClass(ms[1]);
        var m2 = milestoneClass(ms[2]);
        var m3 = milestoneClass(ms[3]);
        var tsStep = showTS ? '1' : '—';
        var admStep = showTS ? '2' : '1';

        var deployLabel = docker ? 'Docker deployment' : 'Dedicated deployment';
        var deployIcon = docker ? 'fa-brands fa-docker' : 'fa-solid fa-cloud';

        outlet.innerHTML =
            '<div class="page-view active setup-ref">' +
            '<div class="setup-ref__inner">' +
            '<header class="setup-ref-hero">' +
            '<div class="setup-ref-hero__icon"><i class="fa-solid fa-wand-magic-sparkles"></i></div>' +
            '<div class="setup-ref-hero__main">' +
            '<h1 class="setup-ref-hero__title">' + title + '</h1>' +
            '<p class="setup-ref-hero__sub">' + subtitle + '</p>' +
            '<p class="setup-ref-hero__note">' + heroNote + '</p>' +
            '<div class="setup-ref-deploy-pill"><i class="' + deployIcon + '"></i> ' + deployLabel + '</div>' +
            '</div></header>' +

            '<nav class="setup-ref-stepper" aria-label="Progress">' +
            '<div class="setup-ref-stepper__line" aria-hidden="true"></div>' +
            '<div class="' + m1 + '" data-setup-ms="1"><div class="setup-ref-ms__icon"><i class="fa-solid fa-database"></i></div><div class="setup-ref-ms__lbl">Metrics store</div></div>' +
            '<div class="' + m2 + '" data-setup-ms="2"><div class="setup-ref-ms__icon"><i class="fa-solid fa-user-shield"></i></div><div class="setup-ref-ms__lbl">Administrator</div></div>' +
            '<div class="' + m3 + '" data-setup-ms="3"><div class="setup-ref-ms__icon"><i class="fa-solid fa-server"></i></div><div class="setup-ref-ms__lbl">Targets</div></div>' +
            '</nav>' +

            '<section id="setup-step-ts" class="setup-ref-card" style="display:' + (showTS ? 'block' : 'none') + ';">' +
            '<div class="setup-ref-card__head setup-ref-card__head--blue">' +
            '<span class="setup-ref-step-chip">Step ' + tsStep + '</span>' +
            '<div><h3 class="setup-ref-card__title">TimescaleDB connection</h3>' +
            '<p class="setup-ref-card__lead">Metrics + encrypted registry. Next step runs <code>00_</code> / <code>01_</code> / <code>02_</code> scripts automatically.</p></div></div>' +
            '<div class="setup-ref-card__body">' +
            '<p class="setup-ref-prereq-line">Reachable host · privileged DB user (CREATE EXTENSION) · dedicated DB (e.g. <code>dbmonitor_metrics</code>).</p>' +
            '<div class="setup-ref-grid">' +
            '<div class="setup-ref-field setup-ref-field--full"><label for="ts-host"><i class="fa-solid fa-network-wired"></i> Host</label><input class="custom-input" id="ts-host" placeholder="timescaledb.internal" autocomplete="off" /></div>' +
            '<div class="setup-ref-field"><label for="ts-port"><i class="fa-solid fa-hashtag"></i> Port</label><input class="custom-input" id="ts-port" value="5432" inputmode="numeric" autocomplete="off" /></div>' +
            '<div class="setup-ref-field"><label for="ts-db"><i class="fa-solid fa-database"></i> Database</label><input class="custom-input" id="ts-db" placeholder="dbmonitor_metrics" autocomplete="off" /></div>' +
            '<div class="setup-ref-field"><label for="ts-ssl"><i class="fa-solid fa-lock"></i> SSL mode</label><select class="custom-select" id="ts-ssl"><option value="require" selected>require</option><option value="verify-full">verify-full</option><option value="disable">disable</option></select></div>' +
            '<div class="setup-ref-field"><label for="ts-user"><i class="fa-solid fa-user"></i> Username</label><input class="custom-input" id="ts-user" placeholder="dbmonitor" autocomplete="off" /></div>' +
            '<div class="setup-ref-field setup-ref-field--pass-span"><label for="ts-pass"><i class="fa-solid fa-key"></i> Password</label><input class="custom-input" id="ts-pass" type="password" autocomplete="new-password" /></div>' +
            '</div>' +

            '<div class="setup-ref-vault-row">' +
            '<label class="setup-ref-vault-label"><span class="setup-ref-vault-label__text"><i class="fa-solid fa-vault"></i> Use HashiCorp Vault for credentials</span>' +
            '<label class="switch"><input type="checkbox" id="ts-vault-toggle" /><span class="slider round"></span></label></label>' +
            '</div>' +
            '<div id="ts-vault-fields" style="display:none;" class="setup-ref-vault-fields">' +
            '<div class="setup-ref-field setup-ref-field--full">' +
            '<label for="ts-vault-path"><i class="fa-solid fa-folder-tree"></i> Vault secret path</label>' +
            '<input class="custom-input setup-ref-input--vault" id="ts-vault-path" placeholder="secret/data/monitoring/timescaledb_creds" autocomplete="off" />' +
            '<p class="setup-ref-vault-help">API needs <code>VAULT_ADDR</code> + <code>VAULT_TOKEN</code>. KV keys: host, port, database, username, password, ssl_mode (optional).</p></div></div>' +

            '<div id="ts-msg" style="display:none;"></div>' +
            '<div class="setup-ref-actions">' +
            '<button type="button" class="btn btn-outline" id="ts-test-btn"><i class="fa-solid fa-plug-circle-check"></i> Test connection</button>' +
            '<button type="button" class="btn btn-accent" id="ts-schema-btn"><i class="fa-solid fa-play"></i> Run schema scripts &amp; continue</button>' +
            '</div></div></section>' +

            '<section id="setup-step-admin" class="setup-ref-card" style="display:' + (showAdmin ? 'block' : 'none') + ';">' +
            '<div class="setup-ref-card__head setup-ref-card__head--green">' +
            '<span class="setup-ref-step-chip">' + (showTS ? 'Step 2' : 'Step 1') + '</span>' +
            '<div><h3 class="setup-ref-card__title">Administrator account</h3>' +
            '<p class="setup-ref-card__lead">Full access to register monitoring targets.</p></div></div>' +
            '<div class="setup-ref-card__body">' +
            '<div class="setup-ref-grid setup-ref-grid--admin">' +
            '<div class="setup-ref-field"><label for="adm-user"><i class="fa-solid fa-user"></i> Username</label><input class="custom-input" id="adm-user" autocomplete="username" /></div>' +
            '<div class="setup-ref-field"><label for="adm-pass"><i class="fa-solid fa-key"></i> Password <span class="setup-ref-muted">(min 8)</span></label><input class="custom-input" id="adm-pass" type="password" autocomplete="new-password" /></div>' +
            '<div class="setup-ref-field"><label for="adm-pass2"><i class="fa-solid fa-key"></i> Confirm password</label><input class="custom-input" id="adm-pass2" type="password" autocomplete="new-password" /></div>' +
            '</div>' +
            '<div id="adm-msg" style="display:none;"></div>' +
            '<button type="button" class="btn btn-accent" id="adm-create-btn"><i class="fa-solid fa-user-shield"></i> Create administrator</button>' +
            '</div></section>' +

            '<section id="setup-done" class="setup-ref-card" style="display:' + (showDone ? 'block' : 'none') + ';">' +
            '<div class="setup-ref-card__head setup-ref-card__head--green">' +
            '<span class="setup-ref-step-chip setup-ref-step-chip--muted"><i class="fa-solid fa-check"></i> Ready</span>' +
            '<div><h3 class="setup-ref-card__title">Setup complete</h3><p class="setup-ref-card__lead">Add monitored databases or open the global overview.</p></div></div>' +
            '<div class="setup-ref-card__body setup-ref-actions">' +
            '<a href="#" id="setup-goto-onb" class="btn btn-accent"><i class="fa-solid fa-server"></i> Add monitored databases</a>' +
            '<a href="#" id="setup-goto-dash" class="btn btn-outline"><i class="fa-solid fa-globe"></i> Global estate overview</a>' +
            '</div></section>' +

            '</div></div>';

        document.getElementById('ts-vault-toggle')?.addEventListener('change', syncVaultUI);
        syncVaultUI();

        document.getElementById('ts-test-btn')?.addEventListener('click', async function() {
            var draft = collectDraft();
            if (!draft.use_vault) {
                if (!draft.host || !draft.database || !draft.username || !draft.password) {
                    showMsg('ts-msg', 'Fill host, database, username, and password (or enable Vault).', 'err');
                    return;
                }
            } else if (!draft.vault_secret_path) {
                showMsg('ts-msg', 'Enter the Vault secret path.', 'err');
                return;
            }
            showMsg('ts-msg', 'Testing connection…', 'info');
            var btn = document.getElementById('ts-test-btn');
            if (btn) btn.disabled = true;
            try {
                var response = await fetch('/api/setup/timescale/test', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(draft)
                });
                var j = await response.json().catch(function() { return {}; });
                if (!response.ok || !j.success) {
                    showMsg('ts-msg', (j.error || 'Connection failed') + '', 'err');
                    return;
                }
                showMsg('ts-msg', 'Connection successful.', 'ok');
            } catch (e) {
                showMsg('ts-msg', String(e.message || e), 'err');
            } finally {
                if (btn) btn.disabled = false;
            }
        });

        document.getElementById('ts-schema-btn')?.addEventListener('click', function() {
            var draft = collectDraft();
            if (!draft.use_vault) {
                if (!draft.host || !draft.database || !draft.username || !draft.password) {
                    showMsg('ts-msg', 'Fill host, database, username, and password (or enable Vault).', 'err');
                    return;
                }
            } else if (!draft.vault_secret_path) {
                showMsg('ts-msg', 'Enter the Vault secret path.', 'err');
                return;
            }
            try {
                sessionStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
            } catch (e) {
                showMsg('ts-msg', 'Could not store draft in session. Check browser storage.', 'err');
                return;
            }
            window.appNavigate('setup-schema');
        });

        document.getElementById('adm-create-btn')?.addEventListener('click', async function() {
            var username = document.getElementById('adm-user')?.value?.trim() || '';
            var password = document.getElementById('adm-pass')?.value || '';
            var passwordVerify = document.getElementById('adm-pass2')?.value || '';
            var createBtn = document.getElementById('adm-create-btn');
            if (!username) {
                showMsg('adm-msg', 'Enter a username.', 'err');
                return;
            }
            if (username.length > 64) {
                showMsg('adm-msg', 'Username must be 64 characters or fewer.', 'err');
                return;
            }
            if (password.length < 8) {
                showMsg('adm-msg', 'Password must be at least 8 characters.', 'err');
                return;
            }
            if (password !== passwordVerify) {
                showMsg('adm-msg', 'Passwords do not match.', 'err');
                return;
            }
            if (createBtn) createBtn.disabled = true;
            try {
                var r = await fetch('/api/setup/bootstrap-admin', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username: username, password: password, password_verify: passwordVerify })
                });
                var j = await r.json().catch(function() { return {}; });
                if (!r.ok) {
                    showMsg('adm-msg', (j.error || 'HTTP ' + r.status) + '', 'err');
                    return;
                }
                if (window._auth) {
                    window._auth.token = j.token;
                    window._auth.user = { user_id: j.user_id, username: j.username, role: j.role };
                    localStorage.setItem('auth_token', j.token);
                    localStorage.setItem('auth_user', JSON.stringify(window._auth.user));
                }
                if (typeof window.apiClient?.setToken === 'function') {
                    window.apiClient.setToken(j.token);
                }
                applyMilestones('complete', 'complete', 'active');
                showMsg('adm-msg', 'Administrator created. Opening monitored servers…', 'ok');
                setTimeout(function() {
                    window.appNavigate('onboarding-servers');
                }, 700);
            } catch (e) {
                showMsg('adm-msg', String(e.message || e), 'err');
            } finally {
                if (createBtn) createBtn.disabled = false;
            }
        });

        document.getElementById('setup-goto-dash')?.addEventListener('click', function(e) {
            e.preventDefault();
            window.appNavigate('global');
        });
        document.getElementById('setup-goto-onb')?.addEventListener('click', function(e) {
            e.preventDefault();
            window.appNavigate('onboarding-servers');
        });
    };
})();
