/**
 * Runs 00_, 01_, 02_ Timescale SQL scripts sequentially via /api/setup/timescale/migrate-step,
 * then persists connection with POST /api/setup/timescale. Draft lives in sessionStorage (SETUP_TS_DRAFT).
 */
(function() {
    var STORAGE_KEY = 'sql_optima_setup_ts_draft';

    function readDraft() {
        try {
            var raw = sessionStorage.getItem(STORAGE_KEY);
            if (!raw) return null;
            return JSON.parse(raw);
        } catch (e) {
            return null;
        }
    }

    function clearDraft() {
        try {
            sessionStorage.removeItem(STORAGE_KEY);
        } catch (e) { /* ignore */ }
    }

    function esc(s) {
        if (window.escapeHtml) return window.escapeHtml(String(s));
        return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    window.SetupSchemaProgressView = async function() {
        var outlet = window.routerOutlet;
        if (!outlet) return;

        var draft = readDraft();
        if (!draft || typeof draft !== 'object') {
            outlet.innerHTML = '<div class="page-view active setup-ref"><div class="setup-ref__inner"><div class="setup-ref-card"><p class="setup-ref-muted">No setup draft found. Return to Initial setup.</p><button type="button" class="btn btn-accent" data-action="navigate" data-route="setup">Back to setup</button></div></div></div>';
            return;
        }

        var steps = [
            { n: 0, label: '00 · Core schema', file: '00_timescale_schema.sql' },
            { n: 1, label: '01 · Seed data', file: '01_seed_data.sql' },
            { n: 2, label: '02 · Rule engine', file: '02_rule_engine.sql' }
        ];

        var panels = steps.map(function(s, i) {
            return (
                '<div class="setup-ref-migrate-panel" data-migrate-step="' + s.n + '" id="mig-panel-' + s.n + '">' +
                '<div class="setup-ref-migrate-panel__head">' +
                '<span class="setup-ref-migrate-status" id="mig-st-' + s.n + '">Pending</span>' +
                '<h4>' + esc(s.label) + '</h4>' +
                '<p class="setup-ref-migrate-panel__file">' + esc(s.file) + '</p>' +
                '</div>' +
                '<pre class="setup-ref-migrate-out" id="mig-out-' + s.n + '"></pre>' +
                '</div>'
            );
        }).join('');

        outlet.innerHTML =
            '<div class="page-view active setup-ref">' +
            '<div class="setup-ref__inner">' +
            '<header class="setup-ref-hero">' +
            '<div class="setup-ref-hero__icon"><i class="fa-solid fa-database"></i></div>' +
            '<div class="setup-ref-hero__main">' +
            '<h1 class="setup-ref-hero__title">Applying schema</h1>' +
            '<p class="setup-ref-hero__sub"><strong>' + esc(draft.host || '') + '</strong> · <strong>' + esc(draft.database || '') + '</strong> — scripts 00 → 02 (first may run several minutes).</p>' +
            '</div></header>' +
            '<div class="setup-ref-card setup-ref-card--migrate">' +
            '<div class="setup-ref-migrate-grid">' + panels + '</div></div>' +
            '<div id="mig-final-msg"></div>' +
            '<div class="setup-ref-actions">' +
            '<button type="button" class="btn btn-outline" id="mig-cancel-btn">Cancel</button>' +
            '</div></div></div>';

        document.getElementById('mig-cancel-btn')?.addEventListener('click', function() {
            clearDraft();
            window.appNavigate('setup');
        });

        var payloadBase = {
            host: draft.host,
            port: draft.port,
            database: draft.database,
            username: draft.username,
            password: draft.password,
            ssl_mode: draft.ssl_mode || 'require',
            use_vault: !!draft.use_vault,
            vault_secret_path: draft.vault_secret_path || ''
        };

        function setStatus(step, text, cls) {
            var el = document.getElementById('mig-st-' + step);
            if (!el) return;
            el.textContent = text;
            el.className = 'setup-ref-migrate-status' + (cls ? ' ' + cls : '');
        }

        function appendOut(step, text) {
            var el = document.getElementById('mig-out-' + step);
            if (!el) return;
            el.textContent = (el.textContent ? el.textContent + '\n' : '') + text;
        }

        for (var si = 0; si < steps.length; si++) {
            var st = steps[si].n;
            setStatus(st, 'Running…', 'setup-ref-migrate-status--run');
            var panel = document.getElementById('mig-panel-' + st);
            if (panel) panel.classList.add('setup-ref-migrate-panel--active');

            var body = Object.assign({}, payloadBase, { step: st });
            try {
                var r = await fetch('/api/setup/timescale/migrate-step', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body)
                });
                var j = await r.json().catch(function() { return {}; });
                if (!r.ok || !j.success) {
                    setStatus(st, 'Failed', 'setup-ref-migrate-status--fail');
                    appendOut(st, (j.error || 'Request failed') + '');
                    document.getElementById('mig-final-msg').innerHTML =
                        '<div class="setup-ref-alert setup-ref-alert--err">Schema run stopped at step ' + st + '. Fix the error, then return to Initial setup to retry.</div>';
                    return;
                }
                setStatus(st, 'Done', 'setup-ref-migrate-status--ok');
                appendOut(st, (j.summary || 'OK') + '');
                if (panel) {
                    panel.classList.remove('setup-ref-migrate-panel--active');
                    panel.classList.add('setup-ref-migrate-panel--done');
                }
            } catch (e) {
                setStatus(st, 'Failed', 'setup-ref-migrate-status--fail');
                appendOut(st, String(e.message || e));
                document.getElementById('mig-final-msg').innerHTML =
                    '<div class="setup-ref-alert setup-ref-alert--err">Network or server error.</div>';
                return;
            }
        }

        document.getElementById('mig-final-msg').innerHTML =
            '<div class="setup-ref-alert setup-ref-alert--info">Saving TimescaleDB connection to the application…</div>';

        try {
            var r2 = await fetch('/api/setup/timescale', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payloadBase)
            });
            var j2 = await r2.json().catch(function() { return {}; });
            if (!r2.ok) {
                document.getElementById('mig-final-msg').innerHTML =
                    '<div class="setup-ref-alert setup-ref-alert--err">Scripts completed but saving configuration failed: ' + esc(j2.error || r2.status) + '</div>';
                return;
            }
            clearDraft();
            document.getElementById('mig-final-msg').innerHTML =
                '<div class="setup-ref-alert setup-ref-alert--ok">Connection saved. Continue with the administrator step.</div>';
            setTimeout(function() {
                window.appNavigate('setup');
            }, 900);
        } catch (e) {
            document.getElementById('mig-final-msg').innerHTML =
                '<div class="setup-ref-alert setup-ref-alert--err">' + esc(String(e.message || e)) + '</div>';
        }
    };
})();
