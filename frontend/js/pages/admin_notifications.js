/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Admin panel module for configuring outbound alert notification channels.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

const CHANNELS = [
    { key: 'webhook', label: 'Generic Webhook', icon: 'fa-solid fa-link', desc: 'POST JSON alert payload to any HTTP endpoint (custom receiver). Do not use Slack or PagerDuty Events API URLs here.', placeholder: 'https://…', inputType: 'url', fieldLabel: 'Webhook URL' },
    { key: 'slack',   label: 'Slack',           icon: 'fa-brands fa-slack', desc: 'Slack Incoming Webhook URL (hooks.slack.com/…). Required for Slack — the generic webhook channel will not work with Slack.', placeholder: 'https://hooks.slack.com/services/…', inputType: 'url', fieldLabel: 'Webhook URL' },
    { key: 'pagerduty', label: 'PagerDuty',     icon: 'fa-solid fa-pager', desc: 'PagerDuty Events API v2 routing key (Integration Key). Opens/resolves incidents via dedup_key = alert fingerprint.', placeholder: 'Events API routing key', inputType: 'text', fieldLabel: 'Routing key' },
    { key: 'email', label: 'Email (SMTP)', icon: 'fa-solid fa-envelope', desc: 'Native SMTP delivery. Password is never shown after save; leave blank to keep the existing password.', placeholder: '', inputType: 'email-form', fieldLabel: 'SMTP' },
];

function emailFormHTML(ch, es, enabled) {
    const e = es || {};
    const toVal = Array.isArray(e.to) ? e.to.join(', ') : '';
    return `
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.5rem;">
            <div>
                <label style="font-size:0.78rem;color:var(--text-muted);display:block;margin-bottom:0.25rem;">Host</label>
                <input type="text" id="notif-email-host" class="form-control" style="font-size:0.82rem;" value="${window.escapeHtml(e.host || '')}" autocomplete="off" />
            </div>
            <div>
                <label style="font-size:0.78rem;color:var(--text-muted);display:block;margin-bottom:0.25rem;">Port</label>
                <input type="number" id="notif-email-port" class="form-control" style="font-size:0.82rem;" value="${e.port || 587}" min="1" max="65535" />
            </div>
            <div>
                <label style="font-size:0.78rem;color:var(--text-muted);display:block;margin-bottom:0.25rem;">Username</label>
                <input type="text" id="notif-email-user" class="form-control" style="font-size:0.82rem;" value="${window.escapeHtml(e.username || '')}" autocomplete="off" />
            </div>
            <div>
                <label style="font-size:0.78rem;color:var(--text-muted);display:block;margin-bottom:0.25rem;">Password ${e.has_password ? '(saved)' : ''}</label>
                <input type="password" id="notif-email-pass" class="form-control" style="font-size:0.82rem;" placeholder="${e.has_password ? 'Leave blank to keep' : ''}" autocomplete="new-password" />
            </div>
            <div style="grid-column:1/-1;">
                <label style="font-size:0.78rem;color:var(--text-muted);display:block;margin-bottom:0.25rem;">From</label>
                <input type="email" id="notif-email-from" class="form-control" style="font-size:0.82rem;" value="${window.escapeHtml(e.from || '')}" />
            </div>
            <div style="grid-column:1/-1;">
                <label style="font-size:0.78rem;color:var(--text-muted);display:block;margin-bottom:0.25rem;">To (comma-separated)</label>
                <input type="text" id="notif-email-to" class="form-control" style="font-size:0.82rem;" value="${window.escapeHtml(toVal)}" />
            </div>
            <label style="display:flex;align-items:center;gap:0.4rem;font-size:0.82rem;cursor:pointer;">
                <input type="checkbox" id="notif-email-starttls" ${e.starttls !== false ? 'checked' : ''} style="accent-color:var(--accent-color);">
                STARTTLS
            </label>
            <label style="display:flex;align-items:center;gap:0.4rem;font-size:0.82rem;cursor:pointer;">
                <input type="checkbox" id="notif-enabled-email" ${enabled ? 'checked' : ''} style="accent-color:var(--accent-color);">
                Enabled
            </label>
        </div>`;
}

export async function loadNotificationConfig() {
    const content = document.getElementById('admin-content');
    if (!content) return;

    content.innerHTML = `
        <div class="glass-panel" style="padding:1.25rem;border-radius:12px;">
            <div style="margin-bottom:1.1rem;">
                <h2 style="font-size:1rem;margin:0;font-weight:600;">
                    <i class="fa-solid fa-bell text-accent"></i> Alert notification channels
                </h2>
                <p class="text-muted" style="margin:0.35rem 0 0;font-size:0.82rem;max-width:48rem;">
                    Secrets are never shown in full after save. Changes take effect immediately without a server restart.
                </p>
            </div>
            <div id="notif-channel-cards" style="display:flex;flex-direction:column;gap:1rem;">
                <div class="text-center text-muted" style="padding:2rem;"><span class="spinner"></span> Loading…</div>
            </div>
        </div>
    `;

    let existing = [];
    try {
        const resp = await window.apiClient.authenticatedFetch('/api/admin/notifications/config');
        if (resp.ok) existing = await resp.json();
    } catch (_) { /* show empty cards on failure */ }

    const byChannel = {};
    (existing || []).forEach(r => { byChannel[r.channel] = r; });

    const cards = content.querySelector('#notif-channel-cards');
    cards.innerHTML = CHANNELS.map(ch => {
        const row = byChannel[ch.key] || {};
        const enabled = row.is_enabled || false;
        const urlMasked = row.url_masked || '';
        const hasURL = row.has_url || false;
        const fields = ch.inputType === 'email-form'
            ? emailFormHTML(ch, row.email_settings, enabled)
            : `
                    <div style="display:flex;gap:0.5rem;flex-wrap:wrap;align-items:flex-end;">
                        <div style="flex:1;min-width:200px;">
                            <label style="font-size:0.78rem;color:var(--text-muted);display:block;margin-bottom:0.25rem;">${window.escapeHtml(ch.fieldLabel || 'Webhook URL')}</label>
                            <input type="${ch.inputType || 'url'}" id="notif-url-${ch.key}" class="form-control" style="font-size:0.82rem;"
                                placeholder="${hasURL ? 'Enter new value to replace (current: ' + window.escapeHtml(urlMasked) + ')' : window.escapeHtml(ch.placeholder || 'https://…')}"
                                autocomplete="off" spellcheck="false" />
                            ${hasURL ? `<div style="font-size:0.75rem;color:var(--text-muted);margin-top:0.2rem;">Current: <code>${window.escapeHtml(urlMasked)}</code></div>` : ''}
                        </div>
                        <label style="display:flex;align-items:center;gap:0.4rem;font-size:0.82rem;white-space:nowrap;cursor:pointer;padding-bottom:0.1rem;">
                            <input type="checkbox" id="notif-enabled-${ch.key}" ${enabled ? 'checked' : ''} style="accent-color:var(--accent-color);width:1rem;height:1rem;">
                            Enabled
                        </label>
                    </div>`;

        return `
        <div class="glass-panel" id="notif-card-${ch.key}" style="padding:1.1rem 1.25rem;border-radius:10px;border:1px solid var(--border-color);">
            <div style="display:flex;align-items:flex-start;gap:1rem;flex-wrap:wrap;">
                <div style="flex:1;min-width:220px;">
                    <div style="display:flex;align-items:center;gap:0.6rem;margin-bottom:0.4rem;">
                        <i class="${ch.icon} text-accent" style="font-size:1.15rem;width:1.3rem;text-align:center;"></i>
                        <span style="font-weight:600;font-size:0.95rem;">${window.escapeHtml(ch.label)}</span>
                        <span id="notif-badge-${ch.key}" class="badge ${enabled ? 'badge-success' : 'badge-muted'}" style="font-size:0.7rem;padding:0.2rem 0.55rem;border-radius:99px;">
                            ${enabled ? 'Enabled' : 'Disabled'}
                        </span>
                    </div>
                    <p class="text-muted" style="margin:0;font-size:0.8rem;">${ch.desc}</p>
                </div>
                <div style="flex:2;min-width:260px;">
                    <div id="notif-msg-${ch.key}" style="min-height:1.4rem;margin-bottom:0.5rem;font-size:0.82rem;"></div>
                    ${fields}
                    <div style="display:flex;gap:0.5rem;margin-top:0.65rem;flex-wrap:wrap;">
                        <button class="btn btn-sm btn-accent" id="notif-save-${ch.key}">
                            <i class="fa-solid fa-floppy-disk"></i> Save
                        </button>
                        <button class="btn btn-sm btn-outline" id="notif-test-${ch.key}" ${!hasURL || !enabled ? 'disabled title="Enable and save first"' : ''}>
                            <i class="fa-solid fa-paper-plane"></i> Test
                        </button>
                    </div>
                </div>
            </div>
        </div>`;
    }).join('');

    CHANNELS.forEach(ch => {
        document.getElementById(`notif-save-${ch.key}`)?.addEventListener('click', () => saveChannel(ch.key));
        document.getElementById(`notif-test-${ch.key}`)?.addEventListener('click', () => testChannel(ch.key));
    });
}

async function saveChannel(channel) {
    const msgEl   = document.getElementById(`notif-msg-${channel}`);
    const saveBtn = document.getElementById(`notif-save-${channel}`);
    if (!msgEl || !saveBtn) return;

    let body;
    if (channel === 'email') {
        const toRaw = (document.getElementById('notif-email-to')?.value || '').trim();
        body = {
            channel: 'email',
            is_enabled: !!document.getElementById('notif-enabled-email')?.checked,
            email: {
                host: (document.getElementById('notif-email-host')?.value || '').trim(),
                port: Number(document.getElementById('notif-email-port')?.value || 587),
                username: (document.getElementById('notif-email-user')?.value || '').trim(),
                password: document.getElementById('notif-email-pass')?.value || '',
                from: (document.getElementById('notif-email-from')?.value || '').trim(),
                to: toRaw.split(',').map(s => s.trim()).filter(Boolean),
                starttls: !!document.getElementById('notif-email-starttls')?.checked,
            },
        };
    } else {
        const urlInput = document.getElementById(`notif-url-${channel}`);
        const enabledCb = document.getElementById(`notif-enabled-${channel}`);
        body = {
            channel,
            url: (urlInput?.value || '').trim(),
            is_enabled: enabledCb?.checked || false,
        };
    }

    saveBtn.disabled = true;
    msgEl.innerHTML = `<span class="text-muted"><span class="spinner" style="width:12px;height:12px;"></span> Saving…</span>`;

    try {
        const resp = await window.apiClient.authenticatedFetch('/api/admin/notifications/config', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!resp.ok) {
            const errBody = await resp.text();
            throw new Error(errBody || `HTTP ${resp.status}`);
        }
        msgEl.innerHTML = `<span class="text-success"><i class="fa-solid fa-check"></i> Saved successfully</span>`;
        const enabled = body.is_enabled;
        const badge = document.getElementById(`notif-badge-${channel}`);
        if (badge) {
            badge.className = `badge ${enabled ? 'badge-success' : 'badge-muted'}`;
            badge.textContent = enabled ? 'Enabled' : 'Disabled';
        }
        const testBtn = document.getElementById(`notif-test-${channel}`);
        if (testBtn) {
            testBtn.disabled = !enabled;
            testBtn.title = !enabled ? 'Enable and save first' : '';
        }
        if (channel === 'email') {
            const pass = document.getElementById('notif-email-pass');
            if (pass) pass.value = '';
        } else {
            const urlInput = document.getElementById(`notif-url-${channel}`);
            if (body.url && urlInput) urlInput.value = '';
        }
    } catch (err) {
        msgEl.innerHTML = `<span class="text-danger"><i class="fa-solid fa-triangle-exclamation"></i> ${window.escapeHtml(String(err.message))}</span>`;
    } finally {
        saveBtn.disabled = false;
    }
}

async function testChannel(channel) {
    const msgEl   = document.getElementById(`notif-msg-${channel}`);
    const testBtn = document.getElementById(`notif-test-${channel}`);
    if (!msgEl || !testBtn) return;

    testBtn.disabled = true;
    msgEl.innerHTML = `<span class="text-muted"><span class="spinner" style="width:12px;height:12px;"></span> Sending test…</span>`;

    try {
        const resp = await window.apiClient.authenticatedFetch('/api/admin/notifications/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ channel }),
        });
        const body = await resp.json().catch(() => ({}));
        if (!resp.ok || !body.ok) {
            throw new Error(body.message || `HTTP ${resp.status}`);
        }
        msgEl.innerHTML = `<span class="text-success"><i class="fa-solid fa-check"></i> Test notification sent</span>`;
    } catch (err) {
        msgEl.innerHTML = `<span class="text-danger"><i class="fa-solid fa-triangle-exclamation"></i> ${window.escapeHtml(String(err.message))}</span>`;
    } finally {
        testBtn.disabled = false;
    }
}
