/**
 * PostgreSQL Best Practices: prefers Rule Engine results (Timescale) when available;
 * otherwise pg_settings DBA audit (/api/postgres/best-practices), with optional Timescale snapshot overlay.
 */
window.PgBestPracticesView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'postgres') {
        window.routerOutlet.innerHTML = `<div class="page-view active dashboard-sky-theme"><h3 class="text-warning">Best practices tracking is for PostgreSQL instances only.</h3></div>`;
        return;
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-shield-halved text-accent"></i> Best Practices Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)}</p>
                </div>
                <button class="btn btn-sm btn-outline text-accent" onclick="window.PgBestPracticesView()"><i class="fa-solid fa-refresh"></i> Refresh</button>
            </div>
            <div style="display:flex; justify-content:center; align-items:center; height:50vh;">
                <div class="spinner"></div><span style="margin-left:1rem;">Loading best practices…</span>
            </div>
        </div>
    `;

    const serverId = inst.id && inst.id !== 0 ? inst.id : window.appState.currentInstanceIdx + 1;

    if (typeof window.renderBestPracticesDashboard === 'function') {
        try {
            const rulesResp = await window.apiClient.authenticatedFetch(
                `/api/rules/best-practices?server_id=${encodeURIComponent(serverId)}`
            );
            if (rulesResp.ok) {
                const ct = rulesResp.headers.get('content-type') || '';
                if (ct.includes('application/json')) {
                    const rulesData = await rulesResp.json();
                    const list = rulesData.best_practices || [];
                    if (list.length > 0) {
                        window.renderBestPracticesDashboard(inst, rulesData);
                        return;
                    }
                }
            }
        } catch (e) {
            console.warn('[PgBestPractices] Rule engine path skipped:', e);
        }
    }

    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/postgres/best-practices?instance=${encodeURIComponent(inst.name)}`
        );
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const contentType = response.headers.get('content-type') || '';
        if (!contentType.includes('application/json')) {
            const text = await response.text();
            console.error('Best Practices API returned non-JSON:', text.substring(0, 200));
            throw new Error('Server returned non-JSON response');
        }

        const data = await response.json();
        const srcHeader = response.headers.get('X-Data-Source') || data.data_source || '';
        renderPgSettingsBestPracticesAudit(inst, data, srcHeader);
    } catch (error) {
        console.error('[PgBestPractices] Error:', error);
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title">
                    <h1><i class="fa-solid fa-shield-halved text-accent"></i> Best Practices Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)}</p>
                </div>
                <div class="alert alert-danger mt-3">
                    <i class="fa-solid fa-exclamation-triangle"></i> Failed to load best practices data: ${window.escapeHtml(error.message)}
                </div>
            </div>
        `;
    }
};

function pgBpMapStatusForDrawer(status) {
    const u = (status || '').toUpperCase();
    if (u === 'RED') return 'CRITICAL';
    if (u === 'YELLOW') return 'WARNING';
    return 'OK';
}

function pgBpEffectiveValueCell(check) {
    const cur = (check.current_value || '').trim();
    const def = (check.default_value || '').trim();
    const same = !def || def === 'N/A' || cur === def;
    const sub = same
        ? '<div class="text-muted" style="font-size:0.62rem;margin-top:4px;line-height:1.35">Same as <code>reset_val</code> (on-disk default): running value matches what a reload would keep.</div>'
        : `<div class="text-muted" style="font-size:0.62rem;margin-top:4px;line-height:1.35"><strong>Drift:</strong> on-disk reset target is <code style="font-size:0.65rem;">${window.escapeHtml(def)}</code> — live <code>setting</code> differs until reload/restart.</div>`;
    return `<td><code style="background:var(--bg-tertiary);padding:2px 6px;border-radius:4px;font-size:0.7rem;">${window.escapeHtml(check.current_value)}</code>${sub}</td>`;
}

function renderPgSettingsBestPracticesAudit(inst, data, sourceHeader) {
    const checks = data.server_config || [];

    if (checks.length === 0) {
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title">
                    <h1><i class="fa-solid fa-shield-halved text-accent"></i> Best Practices Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)}</p>
                </div>
                <div class="alert alert-warning mt-3">
                    <i class="fa-solid fa-info-circle"></i> No configuration checks available. Ensure the PostgreSQL instance is reachable and pg_settings can be read.
                </div>
            </div>
        `;
        return;
    }

    const redCount = checks.filter(c => c.status === 'RED').length;
    const yellowCount = checks.filter(c => c.status === 'YELLOW').length;
    const greenCount = checks.filter(c => c.status === 'GREEN').length;

    const snap = data.snapshot_captured_at
        ? `Timescale snapshot: ${window.escapeHtml(data.snapshot_captured_at)}`
        : '';
    const subParts = [
        `Instance: ${window.escapeHtml(inst.name)}`,
        'pg_settings audit (built-in DBA rules)',
        snap
    ].filter(Boolean);

    const categories = {};
    checks.forEach(check => {
        const cat = check.category || 'Other';
        if (!categories[cat]) categories[cat] = [];
        categories[cat].push(check);
    });

    let html = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-list-check text-accent"></i> Best Practices Dashboard</h1>
                    <p class="subtitle">${subParts.join(' · ')}</p>
                </div>
                <div style="display:flex; align-items:center; gap:1rem;">
                    ${typeof window.renderStatusStrip === 'function' ? window.renderStatusStrip({ lastUpdateId: 'pgBpLastRefreshTime', sourceBadgeId: 'pgBpDataSourceBadge', includeHealth: false, includeFreshness: false, autoRefreshText: '' }) : ''}
                    <button class="btn btn-sm btn-outline text-accent" onclick="window.PgBestPracticesView()"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>

            <div class="metrics-row" style="display:grid; grid-template-columns:repeat(3, minmax(0, 1fr)); gap:0.75rem; margin-top:0.75rem;">
                <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);">
                    <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#991b1b;">Critical</span><i class="fa-solid fa-circle-xclamation card-icon" style="color:#991b1b;"></i></div>
                    <div class="metric-value" style="font-size:1.25rem !important; font-weight:bold !important; color:#991b1b !important;">${redCount}</div>
                    <div class="metric-trend" style="font-size:0.65rem; color:#991b1b;"><i class="fa-solid fa-triangle-exclamation"></i> Require immediate attention</div>
                </div>
                <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);">
                    <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#92400e;">Warnings</span><i class="fa-solid fa-triangle-exclamation card-icon" style="color:#92400e;"></i></div>
                    <div class="metric-value" style="font-size:1.25rem !important; font-weight:bold !important; color:#92400e !important;">${yellowCount}</div>
                    <div class="metric-trend" style="font-size:0.65rem; color:#92400e;"><i class="fa-solid fa-exclamation-circle"></i> Suboptimal configuration</div>
                </div>
                <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);">
                    <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#166534;">Passed</span><i class="fa-solid fa-circle-check card-icon" style="color:#166534;"></i></div>
                    <div class="metric-value" style="font-size:1.25rem !important; font-weight:bold !important; color:#166534 !important;">${greenCount}</div>
                    <div class="metric-trend" style="font-size:0.65rem; color:#166534;"><i class="fa-solid fa-check"></i> OK for these rules</div>
                </div>
            </div>

            <p class="text-muted" style="font-size:0.72rem; margin:0.75rem 0 0 0; line-height:1.4;">
                <strong>Effective value</strong> is live <code>pg_settings.setting</code>. The note under it compares to <code>reset_val</code> (file/on-disk default).
                Severity uses fixed PostgreSQL built-in baselines (e.g. 128MB for <code>shared_buffers</code>), not “current ≤ reset”.
            </p>
    `;

    Object.keys(categories).sort().forEach(category => {
        const catChecks = categories[category];
        const catRed = catChecks.filter(c => c.status === 'RED').length;
        const catYel = catChecks.filter(c => c.status === 'YELLOW').length;
        const catBadge = catRed > 0 ? 'danger' : catYel > 0 ? 'warning' : 'success';

        html += `
            <div class="table-card glass-panel mt-3" style="padding:0.75rem;">
                <div class="card-header" onclick="this.nextElementSibling.classList.toggle('hidden'); this.querySelector('i.fa-chevron').classList.toggle('fa-chevron-down'); this.querySelector('i.fa-chevron').classList.toggle('fa-chevron-up');" style="cursor:pointer;">
                    <h3 style="font-size:0.85rem; margin:0; display:flex; align-items:center; gap:0.5rem;">
                        <i class="fa-solid fa-chevron-up fa-chevron" style="transition:transform 0.2s;"></i>
                        <span class="text-accent">${window.escapeHtml(category)}</span>
                        <span class="badge badge-${catBadge}" style="font-size:0.65rem;">${catChecks.length}</span>
                    </h3>
                </div>
                <div class="table-responsive" style="max-height:420px; overflow-y:auto;">
                    <table class="data-table" style="font-size:0.75rem; width:100%; table-layout:fixed;">
                        <thead>
                            <tr>
                                <th style="width:44px; text-align:center;">Status</th>
                                <th style="width:22%;">Parameter</th>
                                <th style="width:30%;">Effective value</th>
                                <th style="width:28%;">Guidance</th>
                                <th style="width:80px; text-align:center;">Action</th>
                            </tr>
                        </thead>
                        <tbody>
        `;

        catChecks.forEach(check => {
            let statusIcon = '';
            let statusClass = '';
            if (check.status === 'RED') {
                statusIcon = '<i class="fa-solid fa-circle-xmark" style="color:#dc2626;"></i>';
                statusClass = 'style="background:rgba(239,68,68,0.05);"';
            } else if (check.status === 'YELLOW') {
                statusIcon = '<i class="fa-solid fa-triangle-exclamation" style="color:#d97706;"></i>';
                statusClass = 'style="background:rgba(245,158,11,0.05);"';
            } else {
                statusIcon = '<i class="fa-solid fa-circle-check" style="color:#16a34a;"></i>';
            }

            const msgColor = check.status === 'RED' ? '#b91c1c' : check.status === 'YELLOW' ? '#b45309' : 'var(--text-muted)';
            const drawerId = 'pgbp-' + Math.random().toString(36).slice(2, 11);
            window._drawerData = window._drawerData || {};
            window._drawerData[drawerId] = {
                ruleName: check.configuration_name || '',
                description: check.message || '',
                fixScript: check.remediation_sql || '',
                currentValue: check.current_value || '',
                recommendedValue: (check.default_value && check.default_value !== check.current_value) ? ('reset_val target: ' + check.default_value) : 'Same as effective (no drift vs reset_val)',
                status: pgBpMapStatusForDrawer(check.status)
            };

            html += `
                <tr ${statusClass}>
                    <td style="text-align:center; font-size:1rem;">${statusIcon}</td>
                    <td style="word-wrap:break-word;"><strong style="font-family:monospace; font-size:0.72rem;">${window.escapeHtml(check.configuration_name)}</strong></td>
                    ${pgBpEffectiveValueCell(check)}
                    <td style="color:${msgColor}; font-size:0.72rem; word-wrap:break-word;">${window.escapeHtml(check.message)}</td>
                    <td style="text-align:center;">
                        <button type="button" class="btn btn-xs btn-outline" onclick="window.showRuleDrawerById('${drawerId}')">
                            <i class="fa-solid fa-info-circle"></i> Details
                        </button>
                    </td>
                </tr>
            `;
        });

        html += `
                        </tbody>
                    </table>
                </div>
            </div>
        `;
    });

    html += `
            <div class="table-footer glass-panel mt-2" style="padding:0.5rem 0.75rem;">
                <small class="text-muted" style="line-height:1.4;">
                    ${redCount > 0 ? '<span style="color:#b91c1c;"><i class="fa-solid fa-circle-xmark"></i> ' + redCount + ' critical</span> · ' : ''}
                    ${yellowCount > 0 ? '<span style="color:#b45309;"><i class="fa-solid fa-triangle-exclamation"></i> ' + yellowCount + ' warnings</span> · ' : ''}
                    <span style="color:#166534;"><i class="fa-solid fa-circle-check"></i> ${greenCount} passed</span>
                    &nbsp;|&nbsp;Total: ${checks.length} checks
                </small>
            </div>
        </div>
    `;

    window.routerOutlet.innerHTML = html;

    setTimeout(() => {
        const tEl = document.getElementById('pgBpLastRefreshTime');
        if (tEl) tEl.textContent = new Date().toLocaleTimeString();
        if (window.updateSourceBadge) {
            const raw = (sourceHeader || data.data_source || '').toString().trim().toLowerCase();
            window.updateSourceBadge('pgBpDataSourceBadge', raw || 'live');
        }
    }, 0);
}
