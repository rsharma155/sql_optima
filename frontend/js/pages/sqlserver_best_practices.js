/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 * Purpose: SQL Server Best Practices Dashboard Controller
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.SqlserverBestPracticesView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'sqlserver') {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Best practices tracking is for SQL Server instances only.</h3></div>`;
        return;
    }

    // Load Template
    if (typeof window.loadTemplate === 'function') {
        window.routerOutlet.innerHTML = await window.loadTemplate('/pages/sqlserver_best_practices.html');
    }

    const subtitleEl = document.getElementById('sqlserver-bp-subtitle');
    if (subtitleEl) subtitleEl.textContent = `Instance: ${inst.name} | SQL Server Refined Audit`;

    const serverId = inst.id && inst.id !== 0 ? inst.id : window.appState.currentInstanceIdx + 1;

    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/rules/best-practices?server_id=${serverId}&db_type=sqlserver`
        );
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const data = await response.json();
        window._sqlserverBpData = data.best_practices || [];
        renderSqlserverRefinedBestPractices(inst, window._sqlserverBpData);
        initSqlBpFilters();
    } catch (error) {
        console.error('[SqlserverBestPractices] Error:', error);
        const container = document.getElementById('sqlserver-bp-sections');
        if (container) {
            container.innerHTML = `
                <div class="alert alert-danger">
                    <i class="fa-solid fa-exclamation-triangle"></i> Failed to load best practices: ${window.escapeHtml(error.message)}
                </div>
            `;
        }
    }
};

function initSqlBpFilters() {
    const searchInput = document.getElementById('sqlserver-bp-search');
    const categorySelect = document.getElementById('sqlserver-bp-filter-category');
    const statusButtons = document.querySelectorAll('[data-filter="status"]');

    if (!searchInput || !categorySelect) return;

    // Populate categories
    const categories = [...new Set(window._sqlserverBpData.map(r => r.category || 'General'))].sort();
    categorySelect.innerHTML = '<option value="all">All Categories</option>' + 
        categories.map(c => `<option value="${window.escapeHtml(c)}">${window.escapeHtml(c)}</option>`).join('');

    const filterFn = () => {
        const searchTerm = searchInput.value.toLowerCase();
        const category = categorySelect.value;
        const activeStatusBtn = document.querySelector('[data-filter="status"].active');
        const status = activeStatusBtn ? activeStatusBtn.getAttribute('data-value') : 'all';

        const filtered = window._sqlserverBpData.filter(r => {
            const matchesSearch = !searchTerm || 
                (r.rule_name && r.rule_name.toLowerCase().includes(searchTerm)) || 
                (r.description && r.description.toLowerCase().includes(searchTerm)) ||
                (r.category && r.category.toLowerCase().includes(searchTerm));
            
            const matchesCategory = category === 'all' || r.category === category;
            
            const matchesStatus = status === 'all' || (r.status || 'OK').toUpperCase() === status.toUpperCase();

            return matchesSearch && matchesCategory && matchesStatus;
        });

        const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
        renderSqlserverRefinedBestPractices(inst, filtered, true); // true = partial render
    };

    searchInput.addEventListener('input', filterFn);
    categorySelect.addEventListener('change', filterFn);
    statusButtons.forEach(btn => {
        btn.addEventListener('click', () => {
            statusButtons.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            filterFn();
        });
    });
}

function renderSqlserverRefinedBestPractices(inst, rules, isPartial = false) {
    const container = document.getElementById('sqlserver-bp-sections');
    if (!container) return;

    // Only update KPIs and Health on initial full load
    if (!isPartial) {
        // 1. Calculate Counts (Global)
        const counts = {
            CRITICAL: rules.filter(r => r.status === 'CRITICAL').length,
            WARNING: rules.filter(r => r.status === 'WARNING').length,
            OK: rules.filter(r => r.status === 'OK').length,
            INFO: rules.filter(r => r.status === 'INFO').length,
            NA: rules.filter(r => (r.status === 'N/A' || r.status === 'NA' || r.current_value === '-1')).length
        };

        document.getElementById('sqlserver-bp-count-critical').textContent = counts.CRITICAL;
        document.getElementById('sqlserver-bp-count-warning').textContent = counts.WARNING;
        document.getElementById('sqlserver-bp-count-passed').textContent = counts.OK;
        document.getElementById('sqlserver-bp-count-na').textContent = counts.NA;

        // 2. Calculate Health Score
        const totalRelevant = rules.length - counts.NA - counts.INFO;
        const score = totalRelevant > 0 
            ? Math.round(((counts.OK + (counts.WARNING * 0.5)) / totalRelevant) * 100)
            : 100;

        const scoreEl = document.getElementById('sqlserver-health-score');
        if (scoreEl) scoreEl.textContent = score + '%';
        
        const labelEl = document.getElementById('sqlserver-health-label');
        if (labelEl) {
            labelEl.className = 'badge ' + (score > 85 ? 'badge-success' : score > 65 ? 'badge-warning' : 'badge-danger');
            labelEl.textContent = score > 85 ? 'Excellent' : score > 65 ? 'Fair' : 'Needs Attention';
        }

        // 3. Render Health Ring
        initSqlHealthRing('sqlserver-health-ring', score);
    }

    if (rules.length === 0) {
        container.innerHTML = `<div class="alert alert-warning">No matching best practice rules found.</div>`;
        return;
    }

    // 4. Group by Category and filter out hidden rules
    const categories = {};
    rules.forEach(r => {
        // User hint 3: If HA is not configured (unhealthy_replicas == -1) then do not show it
        if (r.rule_id === 'HA_AG_REPLICA_017' && (r.status === 'N/A' || r.current_value === '-1')) {
            return;
        }

        const cat = r.category || 'General';
        if (!categories[cat]) categories[cat] = [];
        categories[cat].push(r);
    });

    // 5. Build HTML
    let html = '';
    const sortedCats = Object.keys(categories).sort();
    
    if (sortedCats.length === 0) {
        container.innerHTML = `<div class="alert alert-warning">No visible best practice rules found for this instance.</div>`;
        return;
    }

    sortedCats.forEach(cat => {
        const catRules = categories[cat];
        if (catRules.length === 0) return;

        const catPassed = catRules.filter(r => r.status === 'OK').length;
        const catPct = Math.round((catPassed / catRules.length) * 100);

        html += `
            <div class="table-card glass-panel mb-3" style="padding:0.75rem; border-top: 2px solid var(--accent);">
                <div class="card-header flex-between mb-2" style="border-bottom:1px solid var(--border-color); padding-bottom:0.4rem;">
                    <h3 style="font-size:0.85rem; margin:0; display:flex; align-items:center; gap:0.4rem;">
                        <span class="text-accent" style="text-transform:uppercase; letter-spacing:0.5px;">${window.escapeHtml(cat)}</span>
                        <span class="text-muted" style="font-size:0.65rem;">(${catRules.length})</span>
                    </h3>
                    <div style="width:100px;">
                        <div class="progress" style="height:4px; background:var(--bg-tertiary);">
                            <div class="progress-bar ${catPct === 100 ? 'bg-success' : 'bg-accent'}" style="width:${catPct}%"></div>
                        </div>
                    </div>
                </div>
                <div style="display:grid; grid-template-columns: 1fr; gap:0.5rem;">
        `;

        catRules.forEach(rule => {
            html += renderSqlRuleRow(rule);
        });

        html += `
                </div>
            </div>
        `;
    });

    container.innerHTML = html || `<div class="alert alert-warning">No matching rules found.</div>`;
}


function renderSqlRuleRow(rule) {
    const status = (rule.status || 'OK').toUpperCase();
    let statusConfig = {
        icon: 'fa-circle-check',
        color: 'var(--success)',
        bg: 'rgba(34,197,94,0.05)',
        label: 'PASSED'
    };

    if (status === 'CRITICAL') {
        statusConfig = { icon: 'fa-circle-xmark', color: 'var(--danger)', bg: 'rgba(239,68,68,0.05)', label: 'CRITICAL' };
    } else if (status === 'WARNING') {
        statusConfig = { icon: 'fa-triangle-exclamation', color: 'var(--warning)', bg: 'rgba(245,158,11,0.05)', label: 'WARNING' };
    } else if (status === 'INFO') {
        statusConfig = { icon: 'fa-circle-info', color: 'var(--accent)', bg: 'rgba(59,130,246,0.05)', label: 'INFO' };
    } else if (status === 'N/A') {
        statusConfig = { icon: 'fa-circle-minus', color: 'var(--text-muted)', bg: 'rgba(107,114,128,0.05)', label: 'N/A' };
    }

    const confidence = (rule.confidence || 'context_dependent').toLowerCase();
    const confLabel = confidence.replace('_', '-').charAt(0).toUpperCase() + confidence.replace('_', '-').slice(1);
    const confClass = confidence === 'definitive' ? 'badge-success' : confidence === 'informational' ? 'badge-info' : 'badge-warning';

    let tagsHtml = '';
    if (rule.context_tags) {
        try {
            const tags = typeof rule.context_tags === 'string' ? JSON.parse(rule.context_tags) : rule.context_tags;
            Object.entries(tags).forEach(([k, v]) => {
                tagsHtml += `<span class="badge" style="font-size:0.6rem; background:var(--bg-tertiary); color:var(--text-muted); border:1px solid var(--border-color);">${k}:${v}</span> `;
            });
        } catch (e) { }
    }

    const drawerId = 'sqlrule-' + Math.random().toString(36).substr(2, 9);
    window._drawerData = window._drawerData || {};
    window._drawerData[drawerId] = {
        ruleName: rule.rule_name,
        description: rule.description,
        fixScript: rule.fix_script,
        currentValue: rule.current_value,
        recommendedValue: rule.recommended_value,
        status: status
    };

    return `
        <div class="rule-row glass-panel" style="display:grid; grid-template-columns: 48px 1fr 120px 80px; align-items:center; padding:0.75rem; background:${statusConfig.bg}; border:1px solid var(--border-color);">
            <div style="text-align:center; font-size:1.2rem; color:${statusConfig.color};">
                <i class="fa-solid ${statusConfig.icon}"></i>
            </div>
            <div>
                <div style="display:flex; align-items:center; gap:0.5rem; flex-wrap:wrap;">
                    <strong style="font-size:0.8rem; color:var(--text-main);">${window.escapeHtml(rule.rule_name)}</strong>
                    <span class="badge ${confClass}" style="font-size:0.6rem; text-transform:uppercase;">${confLabel}</span>
                    ${tagsHtml}
                </div>
                <div class="text-muted" style="font-size:0.7rem; margin-top:0.25rem;">${window.escapeHtml(rule.description)}</div>
            </div>
            <div style="text-align:center;">
                <div style="font-size:0.6rem; color:var(--text-muted); text-transform:uppercase;">Current</div>
                <code style="font-size:0.75rem; color:var(--text-main);">${window.escapeHtml(rule.current_value || '-')}</code>
            </div>
            <div style="text-align:right;">
                <button class="btn btn-xs btn-outline" data-action="call" data-fn="showRuleDrawerById" data-arg="${drawerId}">
                    <i class="fa-solid fa-circle-info"></i> Details
                </button>
            </div>
        </div>
    `;
}

function initSqlHealthRing(canvasId, score) {
    const el = document.getElementById(canvasId);
    if (!el) return;
    const ctx = el.getContext('2d');
    const color = score > 85 ? '#22c55e' : score > 65 ? '#f59e0b' : '#ef4444';
    
    if (window.sqlHealthChart) window.sqlHealthChart.destroy();
    
    window.sqlHealthChart = new Chart(ctx, {
        type: 'doughnut',
        data: {
            datasets: [{
                data: [score, 100 - score],
                backgroundColor: [color, 'rgba(0,0,0,0.05)'],
                borderWidth: 0
            }]
        },
        options: {
            cutout: '80%',
            responsive: true,
            maintainAspectRatio: false,
            plugins: { tooltip: { enabled: false }, legend: { display: false } }
        }
    });
}

window.exportSqlServerBestPracticesCSV = function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;
    window.location.href = `/api/sqlserver/best-practices/export?instance=${encodeURIComponent(inst.name)}`;
};
