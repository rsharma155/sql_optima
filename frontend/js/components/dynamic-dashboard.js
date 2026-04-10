/**
 * SQL Optima - Metadata-Driven Dashboard Engine
 * 
 * Dynamic widget rendering system (Grafana-style).
 * Fetches widget metadata from /api/dashboard/widgets,
 * executes SQL via /api/dashboard/query/execute,
 * and renders charts/grids based on chart_type.
 * 
 * Admin Mode: Toggle to show edit (pen) icons on every widget.
 * WidgetEditorModal: View/edit SQL, test query, save, or revert to default.
 */

// ========== STATE ==========
window._widgetRegistry = window._widgetRegistry || {
    widgets: [],
    adminMode: false,
    results: {},
    charts: {},
    loading: false
};

// ========== DYNAMIC DASHBOARD RENDERER ==========

window.DynamicDashboardView = async function(section) {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) {
        window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">No instance selected.</h3></div>`;
        return;
    }

    window.appState.currentInstanceName = inst.name;

    // Fetch widget metadata
    try {
        const response = await window.apiClient.authenticatedFetch('/api/dashboard/widgets');
        if (response.ok) {
            const contentType = response.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const data = await response.json();
                window._widgetRegistry.widgets = data.widgets || [];
            }
        }
    } catch (e) {
        console.error('[DynamicDashboard] Failed to fetch widgets:', e);
    }

    // Filter by section if specified
    let widgets = window._widgetRegistry.widgets;
    if (section) {
        widgets = widgets.filter(w => w.dashboard_section === section);
    }

    // Group widgets by section
    const grouped = {};
    widgets.forEach(w => {
        if (!grouped[w.dashboard_section]) grouped[w.dashboard_section] = [];
        grouped[w.dashboard_section].push(w);
    });

    // Build HTML
    let html = `
        <div class="page-view active">
            <div class="page-title flex-between">
                <div>
                    <h1><i class="fa-solid fa-chart-line text-accent"></i> Dynamic Dashboard</h1>
                    <p class="subtitle">Instance: ${window.escapeHtml(inst.name)} | Metadata-Driven Widgets</p>
                </div>
                <div style="display:flex; align-items:center; gap:1rem;">
                    ${window._auth && window._auth.isAdmin() ? `
                        <label style="display:flex; align-items:center; gap:0.5rem; font-size:0.85rem; cursor:pointer;">
                            <input type="checkbox" id="adminModeToggle" ${window._widgetRegistry.adminMode ? 'checked' : ''} style="width:16px; height:16px;">
                            <strong>Admin Mode</strong>
                        </label>
                    ` : ''}
                    <button class="btn btn-sm btn-outline text-accent" onclick="window.refreshDynamicDashboard('${section || ''}')">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>
    `;

    if (widgets.length === 0) {
        html += `<div class="glass-panel mt-3" style="padding:2rem; text-align:center;">
            <i class="fa-solid fa-chart-line" style="font-size:3rem; color:var(--text-muted);"></i>
            <h3 style="margin-top:1rem; color:var(--text-muted);">No widgets configured</h3>
            <p class="text-muted">Add widgets to the optima_ui_widgets table to populate this dashboard.</p>
        </div>`;
    } else {
        for (const [sectionName, sectionWidgets] of Object.entries(grouped)) {
            html += `<h3 style="margin:1.5rem 0 0.75rem 0; color:var(--text-secondary); text-transform:uppercase; font-size:0.75rem; letter-spacing:1px;">
                <i class="fa-solid fa-layer-group"></i> ${window.escapeHtml(sectionName.replace(/_/g, ' '))}
            </h3>`;
            html += `<div class="charts-grid" style="display:grid; grid-template-columns:repeat(auto-fill, minmax(400px, 1fr)); gap:0.75rem;">`;
            sectionWidgets.forEach(w => {
                html += window.renderWidgetCard(w);
            });
            html += `</div>`;
        }
    }

    html += `</div>`;
    window.routerOutlet.innerHTML = html;

    // Bind admin mode toggle
    const toggle = document.getElementById('adminModeToggle');
    if (toggle) {
        toggle.addEventListener('change', function() {
            window._widgetRegistry.adminMode = this.checked;
            window.refreshDynamicDashboard(section || '');
        });
    }

    // Execute queries for all widgets
    await window.executeAllWidgets(section || '');
};

window.renderWidgetCard = function(widget) {
    const isAdmin = window._widgetRegistry.adminMode && window._auth && window._auth.isAdmin();
    const chartIcon = {
        'line': 'fa-chart-line',
        'bar': 'fa-chart-bar',
        'doughnut': 'fa-chart-pie',
        'gauge': 'fa-gauge-high',
        'grid': 'fa-table'
    }[widget.chart_type] || 'fa-chart-line';

    return `
        <div class="chart-card glass-panel" style="padding:0.75rem; position:relative;" id="widget-card-${widget.widget_id}">
            <div class="card-header flex-between">
                <h3 style="font-size:0.85rem; margin:0;">
                    <i class="fa-solid ${chartIcon} text-accent"></i> ${window.escapeHtml(widget.title)}
                </h3>
                <div style="display:flex; align-items:center; gap:0.5rem;">
                    <span class="text-muted" style="font-size:0.65rem;" id="widget-status-${widget.widget_id}">Loading...</span>
                    ${isAdmin ? `
                        <button class="btn btn-xs btn-outline" onclick="window.openWidgetEditor('${widget.widget_id}')" title="Edit Widget SQL" style="padding:2px 6px; font-size:0.7rem;">
                            <i class="fa-solid fa-pen"></i> Edit
                        </button>
                    ` : ''}
                </div>
            </div>
            <div id="widget-content-${widget.widget_id}" style="min-height:150px; display:flex; align-items:center; justify-content:center;">
                <div class="spinner"></div>
            </div>
        </div>
    `;
};

window.executeAllWidgets = async function(section) {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) return;

    let widgets = window._widgetRegistry.widgets;
    if (section) {
        widgets = widgets.filter(w => w.dashboard_section === section);
    }

    for (const widget of widgets) {
        await window.executeWidget(widget, inst);
    }
};

window.executeWidget = async function(widget, inst) {
    const statusEl = document.getElementById(`widget-status-${widget.widget_id}`);
    const contentEl = document.getElementById(`widget-content-${widget.widget_id}`);
    if (!contentEl) return;

    try {
        const response = await window.apiClient.authenticatedFetch('/api/dashboard/query/execute', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                widget_id: widget.widget_id,
                parameters: {
                    server_name: inst.name,
                    database: window.appState.currentDatabase || 'all'
                }
            })
        });

        if (!response.ok) {
            const errData = await response.json().catch(() => ({}));
            throw new Error(errData.error || `HTTP ${response.status}`);
        }

        const data = await response.json();
        window._widgetRegistry.results[widget.widget_id] = data.rows || [];

        if (statusEl) {
            statusEl.textContent = `${data.count || 0} rows`;
            statusEl.style.color = 'var(--success)';
        }

        window.renderWidgetContent(widget, data.rows || []);
    } catch (error) {
        console.error(`[Widget ${widget.widget_id}] Error:`, error);
        if (statusEl) {
            statusEl.textContent = 'Error';
            statusEl.style.color = 'var(--danger)';
        }
        contentEl.innerHTML = `<div style="text-align:center; padding:1rem; color:var(--danger);">
            <i class="fa-solid fa-exclamation-triangle"></i> ${window.escapeHtml(error.message)}
        </div>`;
    }
};

window.renderWidgetContent = function(widget, rows) {
    const contentEl = document.getElementById(`widget-content-${widget.widget_id}`);
    if (!contentEl) return;

    if (!rows || rows.length === 0) {
        contentEl.innerHTML = '<div class="text-center text-muted" style="padding:2rem;">No data returned</div>';
        return;
    }

    switch (widget.chart_type) {
        case 'line':
            window.renderLineWidget(widget, rows, contentEl);
            break;
        case 'bar':
            window.renderBarWidget(widget, rows, contentEl);
            break;
        case 'doughnut':
            window.renderDoughnutWidget(widget, rows, contentEl);
            break;
        case 'gauge':
            window.renderGaugeWidget(widget, rows, contentEl);
            break;
        case 'grid':
            window.renderGridWidget(widget, rows, contentEl);
            break;
        default:
            contentEl.innerHTML = `<div class="text-center text-muted">Unknown chart type: ${window.escapeHtml(widget.chart_type)}</div>`;
    }
};

window.renderLineWidget = function(widget, rows, container) {
    const canvasId = `widget-canvas-${widget.widget_id}`;
    container.innerHTML = `<div class="chart-container" style="height:200px;"><canvas id="${canvasId}"></canvas></div>`;

    const ctx = document.getElementById(canvasId);
    if (!ctx) return;

    const labels = rows.map(r => {
        const t = r.time || r.timestamp || r.capture_timestamp || '';
        return t ? new Date(t).toLocaleTimeString() : '';
    });
    const values = rows.map(r => parseFloat(r.value || r.metric_value || 0));

    if (window._widgetRegistry.charts[widget.widget_id]) {
        window._widgetRegistry.charts[widget.widget_id].destroy();
    }

    window._widgetRegistry.charts[widget.widget_id] = new Chart(ctx.getContext('2d'), {
        type: 'line',
        data: {
            labels,
            datasets: [{
                label: widget.title,
                data: values,
                borderColor: window.getCSSVar('--accent-blue'),
                backgroundColor: 'rgba(59, 130, 246, 0.1)',
                fill: true,
                tension: 0.3,
                pointRadius: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true },
                x: { ticks: { maxTicksLimit: 10 } }
            }
        }
    });
};

window.renderBarWidget = function(widget, rows, container) {
    const canvasId = `widget-canvas-${widget.widget_id}`;
    container.innerHTML = `<div class="chart-container" style="height:200px;"><canvas id="${canvasId}"></canvas></div>`;

    const ctx = document.getElementById(canvasId);
    if (!ctx) return;

    const labels = rows.map(r => r.label || r.state || r.name || '');
    const values = rows.map(r => parseFloat(r.value || r.count || 0));

    if (window._widgetRegistry.charts[widget.widget_id]) {
        window._widgetRegistry.charts[widget.widget_id].destroy();
    }

    window._widgetRegistry.charts[widget.widget_id] = new Chart(ctx.getContext('2d'), {
        type: 'bar',
        data: {
            labels,
            datasets: [{
                label: widget.title,
                data: values,
                backgroundColor: [window.getCSSVar('--accent-blue'), window.getCSSVar('--success'), window.getCSSVar('--warning'), window.getCSSVar('--danger')]
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: false } },
            scales: { y: { beginAtZero: true } }
        }
    });
};

window.renderDoughnutWidget = function(widget, rows, container) {
    const canvasId = `widget-canvas-${widget.widget_id}`;
    container.innerHTML = `<div class="chart-container doughnut-container" style="height:200px;"><canvas id="${canvasId}"></canvas></div>`;

    const ctx = document.getElementById(canvasId);
    if (!ctx) return;

    const labels = rows.map(r => r.label || r.name || '');
    const values = rows.map(r => parseFloat(r.value || 0));

    if (window._widgetRegistry.charts[widget.widget_id]) {
        window._widgetRegistry.charts[widget.widget_id].destroy();
    }

    window._widgetRegistry.charts[widget.widget_id] = new Chart(ctx.getContext('2d'), {
        type: 'doughnut',
        data: {
            labels,
            datasets: [{
                data: values,
                backgroundColor: [window.getCSSVar('--accent-blue'), window.getCSSVar('--success'), window.getCSSVar('--warning'), window.getCSSVar('--danger')],
                borderWidth: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            cutout: '65%',
            plugins: { legend: { position: 'bottom' } }
        }
    });
};

window.renderGaugeWidget = function(widget, rows, container) {
    if (!rows || rows.length === 0) {
        container.innerHTML = '<div class="text-center text-muted">No data</div>';
        return;
    }

    const total = rows.reduce((sum, r) => sum + parseFloat(r.value || r.size_mb || 0), 0);

    container.innerHTML = `
        <div style="text-align:center; padding:1rem;">
            <div style="font-size:2rem; font-weight:700; color:var(--accent-blue);">${total.toFixed(1)}</div>
            <div class="text-muted" style="font-size:0.8rem;">${widget.title}</div>
            <table class="data-table" style="font-size:0.7rem; margin-top:0.5rem; max-height:120px; overflow-y:auto;">
                <thead><tr><th>Database</th><th>Size (MB)</th></tr></thead>
                <tbody>
                    ${rows.map(r => `<tr><td>${window.escapeHtml(r.database || r.label || '')}</td><td class="text-right">${parseFloat(r.value || r.size_mb || 0).toFixed(1)}</td></tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
};

window.renderGridWidget = function(widget, rows, container) {
    if (!rows || rows.length === 0) {
        container.innerHTML = '<div class="text-center text-muted">No data</div>';
        return;
    }

    const columns = Object.keys(rows[0]);

    container.innerHTML = `
        <div class="table-responsive" style="max-height:250px; overflow-y:auto;">
            <table class="data-table" style="font-size:0.7rem;">
                <thead><tr>${columns.map(c => `<th>${window.escapeHtml(c)}</th>`).join('')}</tr></thead>
                <tbody>
                    ${rows.slice(0, 50).map(row => `<tr>${columns.map(c => `<td>${window.escapeHtml(String(row[c] ?? ''))}</td>`).join('')}</tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
};

window.refreshDynamicDashboard = function(section) {
    window.DynamicDashboardView(section || '');
};

// ========== WIDGET EDITOR MODAL (Admin Mode) ==========

window.openWidgetEditor = async function(widgetId) {
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/widgets/${encodeURIComponent(widgetId)}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const contentType = response.headers.get('content-type') || '';
        if (!contentType.includes('application/json')) {
            throw new Error('Server returned non-JSON response');
        }

        const widget = await response.json();
        window.showWidgetEditorModal(widget);
    } catch (error) {
        console.error('[WidgetEditor] Failed to fetch widget:', error);
        alert('Failed to load widget: ' + error.message);
    }
};

window.showWidgetEditorModal = function(widget) {
    const overlay = document.createElement('div');
    overlay.id = 'widget-editor-overlay';
    overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.7);z-index:100000;display:flex;align-items:center;justify-content:center;';

    overlay.innerHTML = `
        <div style="background:var(--bg-primary);border:1px solid var(--border-color);border-radius:8px;padding:1.5rem;max-width:900px;width:95%;max-height:90vh;overflow-y:auto;box-shadow:0 8px 32px rgba(0,0,0,0.5);">
            <div class="flex-between" style="margin-bottom:1rem;">
                <div>
                    <h3 style="margin:0;"><i class="fa-solid fa-pen-to-square text-accent"></i> Widget Editor</h3>
                    <p class="text-muted" style="font-size:0.8rem; margin:0.25rem 0 0 0;">${window.escapeHtml(widget.title)} (${window.escapeHtml(widget.chart_type)})</p>
                </div>
                <button onclick="window.closeWidgetEditor()" style="background:none;border:none;color:var(--text-muted);font-size:1.2rem;cursor:pointer;"><i class="fa-solid fa-xmark"></i></button>
            </div>

            <div style="margin-bottom:1rem;">
                <label style="font-size:0.8rem; font-weight:600;">Widget ID:</label>
                <div style="font-family:monospace; font-size:0.8rem; color:var(--text-muted);">${window.escapeHtml(widget.widget_id)}</div>
            </div>

            <div style="margin-bottom:1rem;">
                <label style="font-size:0.8rem; font-weight:600;">SQL Query:</label>
                <div style="position:relative;">
                    <textarea id="widget-sql-editor" style="width:100%;height:200px;background:var(--bg-secondary);color:var(--text);border:1px solid var(--border-color);border-radius:4px;padding:0.75rem;font-family:'JetBrains Mono',monospace;font-size:0.8rem;resize:vertical;" spellcheck="false">${window.escapeHtml(widget.current_sql)}</textarea>
                </div>
                <div style="font-size:0.7rem; color:var(--text-muted); margin-top:0.25rem;">
                    Parameters: <code>{{server_name}}</code>, <code>{{database}}</code>
                </div>
            </div>

            <div style="display:flex; gap:0.5rem; margin-bottom:1rem; flex-wrap:wrap;">
                <button class="btn btn-sm btn-accent" onclick="window.testWidgetQuery('${widget.widget_id}')">
                    <i class="fa-solid fa-play"></i> Run Query
                </button>
                <button class="btn btn-sm btn-outline" onclick="window.saveWidgetSql('${widget.widget_id}')">
                    <i class="fa-solid fa-save"></i> Save to Dashboard
                </button>
                <button class="btn btn-sm btn-outline" style="border-color:var(--warning); color:var(--warning);" onclick="window.restoreWidgetDefault('${widget.widget_id}')">
                    <i class="fa-solid fa-rotate-left"></i> Revert to Default
                </button>
            </div>

            <div id="widget-query-results" style="display:none;">
                <h4 style="font-size:0.85rem; margin:0 0 0.5rem 0;"><i class="fa-solid fa-table text-accent"></i> Query Results Preview</h4>
                <div id="widget-results-content" style="max-height:200px; overflow-y:auto;"></div>
            </div>
        </div>
    `;

    document.body.appendChild(overlay);
};

window.closeWidgetEditor = function() {
    const overlay = document.getElementById('widget-editor-overlay');
    if (overlay) document.body.removeChild(overlay);
};

window.testWidgetQuery = async function(widgetId) {
    const sqlEditor = document.getElementById('widget-sql-editor');
    if (!sqlEditor) return;

    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst) {
        alert('No instance selected.');
        return;
    }

    const resultsDiv = document.getElementById('widget-query-results');
    const contentDiv = document.getElementById('widget-results-content');
    if (!resultsDiv || !contentDiv) return;

    resultsDiv.style.display = 'block';
    contentDiv.innerHTML = '<div class="text-center text-muted" style="padding:1rem;"><div class="spinner" style="display:inline-block;"></div> Executing...</div>';

    try {
        const response = await window.apiClient.authenticatedFetch('/api/dashboard/query/execute', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                widget_id: widgetId,
                parameters: {
                    server_name: inst.name,
                    database: window.appState.currentDatabase || 'all'
                }
            })
        });

        if (!response.ok) {
            const errData = await response.json().catch(() => ({}));
            throw new Error(errData.error || `HTTP ${response.status}`);
        }

        const data = await response.json();
        const rows = data.rows || [];

        if (rows.length === 0) {
            contentDiv.innerHTML = '<div class="text-center text-muted" style="padding:1rem;">Query returned 0 rows.</div>';
            return;
        }

        const columns = Object.keys(rows[0]);
        contentDiv.innerHTML = `
            <div style="font-size:0.75rem; color:var(--success); margin-bottom:0.5rem;"><i class="fa-solid fa-check-circle"></i> Success: ${rows.length} rows returned</div>
            <div class="table-responsive" style="max-height:180px; overflow-y:auto;">
                <table class="data-table" style="font-size:0.7rem;">
                    <thead><tr>${columns.map(c => `<th>${window.escapeHtml(c)}</th>`).join('')}</tr></thead>
                    <tbody>
                        ${rows.slice(0, 20).map(row => `<tr>${columns.map(c => `<td>${window.escapeHtml(String(row[c] ?? ''))}</td>`).join('')}</tr>`).join('')}
                    </tbody>
                </table>
            </div>
            ${rows.length > 20 ? `<div class="text-muted" style="font-size:0.7rem; margin-top:0.25rem;">Showing first 20 of ${rows.length} rows</div>` : ''}
        `;
    } catch (error) {
        contentDiv.innerHTML = `<div style="color:var(--danger); font-size:0.8rem; padding:1rem; background:rgba(239,68,68,0.1); border-radius:4px;">
            <i class="fa-solid fa-exclamation-triangle"></i> <strong>Error:</strong> ${window.escapeHtml(error.message)}
        </div>`;
    }
};

window.saveWidgetSql = async function(widgetId) {
    const sqlEditor = document.getElementById('widget-sql-editor');
    if (!sqlEditor) return;

    const sql = sqlEditor.value.trim();
    if (!sql) {
        alert('SQL cannot be empty.');
        return;
    }

    if (!confirm('Save this SQL query to the dashboard? This will affect all users.')) return;

    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/widgets/${encodeURIComponent(widgetId)}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ current_sql: sql })
        });

        if (!response.ok) {
            const errData = await response.json().catch(() => ({}));
            throw new Error(errData.error || `HTTP ${response.status}`);
        }

        alert('Widget SQL saved successfully!');
        window.closeWidgetEditor();
        window.refreshDynamicDashboard('');
    } catch (error) {
        alert('Failed to save widget SQL: ' + error.message);
    }
};

window.restoreWidgetDefault = async function(widgetId) {
    if (!confirm('Revert this widget\'s SQL to the default? This cannot be undone.')) return;

    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/widgets/${encodeURIComponent(widgetId)}/restore`, {
            method: 'POST'
        });

        if (!response.ok) {
            const errData = await response.json().catch(() => ({}));
            throw new Error(errData.error || `HTTP ${response.status}`);
        }

        // Refresh the editor with the restored SQL
        window.openWidgetEditor(widgetId);
    } catch (error) {
        alert('Failed to restore default SQL: ' + error.message);
    }
};
