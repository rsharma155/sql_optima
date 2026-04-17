/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Rules engine page for best practices evaluation.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.RulesEngineView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">No instance selected.</h3></div>`;
        return;
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-list-check text-accent"></i> Best Practices Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Rule Engine Results</p>
                </div>
                <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="RulesEngineView"><i class="fa-solid fa-refresh"></i> Refresh</button>
            </div>
            <div style="display:flex; justify-content:center; align-items:center; height:50vh;">
                <div class="spinner"></div><span style="margin-left:1rem;">Loading best practices...</span>
            </div>
        </div>
    `;

    try {
        let serverId = inst.id;
        if (!serverId || serverId === 0) {
            serverId = window.appState.currentInstanceIdx + 1;
        }
        const response = await window.apiClient.authenticatedFetch(
            `/api/rules/best-practices?server_id=${serverId}`
        );
        if (!response.ok) {
            if (response.status === 400) {
                renderBestPracticesDashboard(inst, { best_practices: [], count: 0 });
                return;
            }
            throw new Error(`HTTP ${response.status}`);
        }

        const data = await response.json();
        renderBestPracticesDashboard(inst, data);
    } catch (error) {
        console.error('[RulesEngine] Error:', error);
        window.routerOutlet.innerHTML = `
            <div class="page-view active">
                <div class="page-title">
                    <h1><i class="fa-solid fa-list-check text-accent"></i> Best Practices Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)}</p>
                </div>
                <div class="alert alert-danger mt-3">
                    <i class="fa-solid fa-exclamation-triangle"></i> Failed to load best practices: ${window.escapeHtml(error.message)}
                </div>
            </div>
        `;
    }
};

function renderBestPracticesDashboard(inst, data) {
    const checks = data.best_practices || [];

    if (checks.length === 0) {
        window.routerOutlet.innerHTML = `
            <div class="page-view active">
                <div class="page-title">
                    <h1><i class="fa-solid fa-list-check text-accent"></i> Best Practices Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)}</p>
                </div>
                <div class="alert alert-warning mt-3">
                    <i class="fa-solid fa-info-circle"></i> No best practice rules configured. Please add rules to the Rule Engine.
                </div>
            </div>
        `;
        return;
    }

    const criticalCount = checks.filter(c => c.status === 'CRITICAL').length;
    const warningCount = checks.filter(c => c.status === 'WARNING').length;
    const okCount = checks.filter(c => c.status === 'OK').length;

    const categories = {};
    checks.forEach(check => {
        const cat = check.category || 'Uncategorized';
        if (!categories[cat]) categories[cat] = [];
        categories[cat].push(check);
    });

    const bpTheme = inst && inst.type === 'postgres' ? 'dashboard-sky-theme' : '';
    let html = `
        <div class="page-view active ${bpTheme}">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-list-check text-accent"></i> Best Practices Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Dynamic Rule Engine</p>
                </div>
                <div style="display:flex; align-items:center; gap:1rem;">
                    ${window.renderStatusStrip({ lastUpdateId: 'bpLastRefreshTime', sourceBadgeId: 'bpDataSourceBadge', includeHealth: false, includeFreshness: false, autoRefreshText: '' })}
                    <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="${inst && inst.type === 'postgres' && typeof window.PgBestPracticesView === 'function' ? 'PgBestPracticesView' : 'RulesEngineView'}"><i class="fa-solid fa-refresh"></i> Refresh</button>
                </div>
            </div>

            <div class="metrics-row" style="display:grid; grid-template-columns:repeat(3, minmax(0, 1fr)); gap:0.75rem; margin-top:0.75rem;">
                <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);">
                    <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#991b1b;">Critical</span><i class="fa-solid fa-circle-xclamation card-icon" style="color:#991b1b;"></i></div>
                    <div class="metric-value" style="font-size:1.25rem; font-weight:bold; color:#991b1b;">${criticalCount}</div>
                    <div class="metric-trend" style="font-size:0.65rem; color:#991b1b;"><i class="fa-solid fa-triangle-exclamation"></i> Require immediate action</div>
                </div>
                <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);">
                    <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#92400e;">Warnings</span><i class="fa-solid fa-triangle-exclamation card-icon" style="color:#92400e;"></i></div>
                    <div class="metric-value" style="font-size:1.25rem; font-weight:bold; color:#92400e;">${warningCount}</div>
                    <div class="metric-trend" style="font-size:0.65rem; color:#92400e;"><i class="fa-solid fa-exclamation-circle"></i> Should be addressed</div>
                </div>
                <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);">
                    <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#166534;">Passed</span><i class="fa-solid fa-circle-check card-icon" style="color:#166534;"></i></div>
                    <div class="metric-value" style="font-size:1.25rem; font-weight:bold; color:#166534;">${okCount}</div>
                    <div class="metric-trend positive" style="font-size:0.65rem; color:#166534;"><i class="fa-solid fa-check"></i> Configured correctly</div>
                </div>
            </div>
    `;

    // Best Practices are sourced from Rule Engine (Timescale/Postgres). Show a stable badge.
    setTimeout(() => {
        if (window.updateSourceBadge) {
            window.updateSourceBadge('bpDataSourceBadge', 'timescale');
        }
        const tEl = document.getElementById('bpLastRefreshTime');
        if (tEl) tEl.textContent = new Date().toLocaleTimeString();
    }, 0);

    Object.keys(categories).sort().forEach(category => {
        const catChecks = categories[category];
        const catCritical = catChecks.filter(c => c.status === 'CRITICAL').length;
        const catWarning = catChecks.filter(c => c.status === 'WARNING').length;

        const catSeverity = catCritical > 0 ? 'CRITICAL' : catWarning > 0 ? 'WARNING' : 'OK';

        html += `
            <div class="table-card glass-panel mt-3" style="padding:0.75rem;">
                <div class="card-header" data-action="toggle-section" style="cursor:pointer;">
                    <h3 style="font-size:0.85rem; margin:0; display:flex; align-items:center; gap:0.5rem;">
                        <i class="fa-solid fa-chevron-up" style="transition:transform 0.2s;"></i>
                        <span class="text-accent">${window.escapeHtml(category)}</span>
                        <span class="badge badge-${catSeverity.toLowerCase()}" style="font-size:0.65rem;">${catChecks.length}</span>
                    </h3>
                </div>
                <div class="table-responsive" style="max-height:400px; overflow-y:auto;">
                    <table class="data-table" style="font-size:0.75rem; width:100%; table-layout:fixed;">
                        <thead>
                            <tr>
                                <th style="width:50px; text-align:center;">Status</th>
                                <th style="width:200px;">Rule Name</th>
                                <th style="width:120px; text-align:center;">Current Value</th>
                                <th style="width:120px; text-align:center;">Recommended</th>
                                <th style="width:80px; text-align:center;">Action</th>
                            </tr>
                        </thead>
                        <tbody>
        `;

        catChecks.forEach((check, idx) => {
            const statusKey = (check.status || 'OK').toUpperCase();
            let statusIcon = '';
            let statusClass = '';
            if (statusKey === 'CRITICAL') {
                statusIcon = '<i class="fa-solid fa-circle-xmark" style="color:var(--danger);"></i>';
                statusClass = 'style="background:rgba(239,68,68,0.04);"';
            } else if (statusKey === 'WARNING') {
                statusIcon = '<i class="fa-solid fa-triangle-exclamation" style="color:var(--warning);"></i>';
                statusClass = 'style="background:rgba(245,158,11,0.04);"';
            } else {
                statusIcon = '<i class="fa-solid fa-circle-check" style="color:var(--success);"></i>';
            }

            const safeDesc = window.escapeHtml(check.description || '');
            const safeCurr = window.escapeHtml(check.current_value || '-');
            const safeRec = window.escapeHtml(check.recommended_value || '-');
            
            // Store raw values for drawer - will escape on display
            const rawFixScript = check.fix_script || '';
            const drawerId = 'drawer-' + Math.random().toString(36).substr(2, 9);
            
            // Store drawer data in a global object to avoid JSON parsing issues
            window._drawerData = window._drawerData || {};
            window._drawerData[drawerId] = {
                ruleName: check.rule_name || '',
                description: check.description || '',
                fixScript: rawFixScript,
                currentValue: check.current_value || '',
                recommendedValue: check.recommended_value || '',
                status: statusKey
            };

            html += `
                <tr ${statusClass}>
                    <td style="text-align:center; font-size:1rem;">${statusIcon}</td>
                    <td style="word-wrap:break-word;"><strong style="font-family:monospace; font-size:0.7rem;">${window.escapeHtml(check.rule_name)}</strong></td>
                    <td style="text-align:center;"><code style="background:var(--bg-tertiary); padding:2px 4px; border-radius:4px; font-size:0.65rem; display:inline-block; width:100%;">${safeCurr}</code></td>
                    <td style="text-align:center;"><code style="background:var(--bg-tertiary); padding:2px 4px; border-radius:4px; font-size:0.65rem; display:inline-block; width:100%;">${safeRec}</code></td>
                    <td style="text-align:center;">
                        <button class="btn btn-xs btn-outline" data-action="call" data-fn="showRuleDrawerById" data-arg="${drawerId}">
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
            <div class="table-footer" style="padding:0.5rem 0 0 0;">
                <small class="text-muted">
                    ${criticalCount > 0 ? '<span style="color:var(--danger);"><i class="fa-solid fa-circle-xmark"></i> ' + criticalCount + ' critical</span> | ' : ''}
                    ${warningCount > 0 ? '<span style="color:var(--warning);"><i class="fa-solid fa-triangle-warning"></i> ' + warningCount + ' warnings</span> | ' : ''}
                    <span style="color:var(--success);"><i class="fa-solid fa-circle-check"></i> ' + okCount + ' passed</span>
                    &nbsp;&nbsp;|&nbsp;&nbsp;Total: ${checks.length} rules
                </small>
            </div>
        </div>
    `;

    window.routerOutlet.innerHTML = html;
}

// Used by PostgreSQL sidebar "Best Practices" when the rule engine returns data.
window.renderBestPracticesDashboard = renderBestPracticesDashboard;

window.showRuleDrawerFromData = function(btn) {
    try {
        const dataStr = btn.getAttribute('data-drawer');
        if (!dataStr) {
            console.error('[showRuleDrawerFromData] No data attribute found');
            return;
        }
        const data = JSON.parse(dataStr.replace(/&quot;/g, '"'));
        window.showRuleDrawer(data.ruleName, data.description, data.fixScript, data.status);
    } catch (err) {
        console.error('[showRuleDrawerFromData] Error:', err);
        alert('Error opening details: ' + err.message);
    }
};

window.showRuleDrawerById = function(drawerId) {
    const data = window._drawerData ? window._drawerData[drawerId] : null;
    if (!data) {
        console.error('[showRuleDrawerById] No data found for id:', drawerId);
        alert('Error: drawer data not found');
        return;
    }
    let fixScript = data.fixScript || '';
    if (data.recommendedValue && data.recommendedValue !== '-') {
        fixScript = fixScript.replace(/<RecommendedMB>|<Recommended>|<Value>/gi, data.recommendedValue);
    }
    window.showRuleDrawer(data.ruleName, data.description, fixScript, data.status, data.currentValue, data.recommendedValue);
};

window.showRuleDrawer = function(ruleName, description, fixScript, status, currentValue, recommendedValue) {
    try {
        const existingDrawer = document.getElementById('rule-drawer-overlay');
        if (existingDrawer) existingDrawer.remove();

        const statusKey = (status || 'OK').toUpperCase();
        const statusStyles = {
            'CRITICAL': { bg: '#fee2e2', color: '#991b1b', badge: 'danger' },
            'WARNING': { bg: '#fef3c7', color: '#92400e', badge: 'warning' },
            'OK': { bg: '#dcfce7', color: '#166534', badge: 'success' }
        };
        const style = statusStyles[statusKey] || { bg: '#f3f4f6', color: '#6b7280', badge: 'secondary' };

        const overlay = document.createElement('div');
        overlay.id = 'rule-drawer-overlay';
        overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.5);z-index:9999;display:flex;justify-content:flex-end;';
        overlay.onclick = function(e) {
            if (e.target === overlay) overlay.remove();
        };

        const drawer = document.createElement('div');
        drawer.style.cssText = `width:450px;max-width:90vw;background:#ffffff;height:100%;padding:1.5rem;overflow-y:auto;animation:slideIn 0.2s ease-out;box-shadow:-4px 0 20px rgba(0,0,0,0.3);border-left:4px solid ${style.color};`;

        const safeDesc = description ? window.escapeHtml(description) : 'No description available.';
        const safeFix = fixScript ? window.escapeHtml(fixScript) : '';
        const safeRuleName = ruleName ? window.escapeHtml(ruleName) : 'Unknown Rule';
        const safeCurr = currentValue ? window.escapeHtml(currentValue) : '-';
        const safeRec = recommendedValue ? window.escapeHtml(recommendedValue) : '-';

        // Hide fix script for OK status
        const showFixScript = statusKey !== 'OK' && safeFix;

        const copyBtn = showFixScript ? `<button class="btn btn-sm btn-outline" data-action="copy-closest-fix"><i class="fa-solid fa-copy"></i> Copy</button>` : '';

        drawer.innerHTML = `
            <div class="drawer-content" data-fix="${safeFix}" style="font-size:0.85rem; color:#1f2937;">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem; border-bottom:1px solid #e5e7eb; padding-bottom:1rem;">
                    <h2 style="margin:0;font-size:1.1rem;color:#111827; font-weight:600;">${safeRuleName}</h2>
                    <button class="btn btn-sm btn-outline" data-action="close-id" data-target="rule-drawer-overlay" style="border:1px solid #d1d5db; border-radius:4px; padding:4px 8px; cursor:pointer;"><i class="fa-solid fa-times"></i></button>
                </div>
                <div style="margin-bottom:1rem;">
                    <span class="badge badge-${style.badge}" style="background:${style.bg}; color:${style.color}; padding:4px 12px; border-radius:12px; font-size:0.75rem; font-weight:600;">${statusKey}</span>
                </div>
                <div style="margin-bottom:1rem; padding:1rem; background:#f9fafb; border-radius:8px;">
                    <div style="display:grid; grid-template-columns:1fr 1fr; gap:0.5rem; margin-bottom:0.5rem;">
                        <div><span style="color:#6b7280; font-size:0.7rem;">Current Value:</span><br><strong>${safeCurr}</strong></div>
                        <div><span style="color:#6b7280; font-size:0.7rem;">Recommended:</span><br><strong>${safeRec}</strong></div>
                    </div>
                </div>
                <div style="margin-bottom:1.5rem; padding:1rem; background:#f9fafb; border-radius:8px;">
                    <h4 style="color:#6b7280;font-size:0.7rem;text-transform:uppercase;margin:0 0 0.5rem 0; font-weight:600;">Why This Matters</h4>
                    <p style="color:#374151;margin:0;line-height:1.6;">${safeDesc}</p>
                </div>
                ${showFixScript ? `
                <div style="margin-bottom:1.5rem;">
                    <h4 style="color:#6b7280;font-size:0.7rem;text-transform:uppercase;margin:0 0 0.5rem 0; font-weight:600;">Fix Script</h4>
                    <pre style="background:#1f2937; color:#f9fafb; padding:1rem;border-radius:8px;overflow-x:auto;font-size:0.75rem;margin:0; font-family:monospace;"><code>${safeFix}</code></pre>
                </div>
                ` : ''}
                <div style="margin-top:1rem; padding-top:1rem; border-top:1px solid #e5e7eb;">
                    ${copyBtn}
                </div>
            </div>
        `;

        overlay.appendChild(drawer);
        document.body.appendChild(overlay);
    } catch (err) {
        console.error('[showRuleDrawer] error:', err);
        alert('Error opening details: ' + err.message);
    }
};