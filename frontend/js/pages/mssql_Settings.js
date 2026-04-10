window.SettingsView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'N/A'};
    const userId = window.appState.user?.id || 1;

    // Ensure appState has all required arrays
    if (!window.appState.config.dashboards) window.appState.config.dashboards = [];
    if (!window.appState.config.alertThresholds) window.appState.config.alertThresholds = [];
    if (!window.appState.config.notificationChannels) window.appState.config.notificationChannels = [];
    if (!window.appState.recentAlerts) window.appState.recentAlerts = [];

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title">
                <h1><i class="fa-solid fa-gear text-accent"></i> Settings & Configuration</h1>
                <p class="subtitle">Manage dashboards, alerts, and server connections</p>
            </div>
            
            <div class="settings-tabs" style="display:flex; gap:0.5rem; margin-bottom:1rem; border-bottom:1px solid var(--border-color); padding-bottom:0.5rem;">
                <button class="btn btn-sm btn-accent" onclick="window.showSettingsTab('dashboards')" id="tab-dashboards">Dashboards</button>
                <button class="btn btn-sm btn-outline" onclick="window.showSettingsTab('alerts')" id="tab-alerts">Alerts</button>
                <button class="btn btn-sm btn-outline" onclick="window.showSettingsTab('servers')" id="tab-servers">Servers</button>
                <button class="btn btn-sm btn-outline" onclick="window.showSettingsTab('collection')" id="tab-collection">Metrics Collection</button>
                <button class="btn btn-sm btn-outline" onclick="window.showSettingsTab('import-export')" id="tab-import-export">Import/Export</button>
            </div>
            
            <div id="settings-content">
                <div style="display:flex; justify-content:center; align-items:center; height:200px;">
                    <div class="spinner"></div><span style="margin-left:1rem;">Loading settings...</span>
                </div>
            </div>
        </div>
    `;

    window.showSettingsTab('dashboards');
};

window.showSettingsTab = async function(tab) {
    document.querySelectorAll('.settings-tabs .btn').forEach(btn => {
        btn.classList.remove('btn-accent');
        btn.classList.add('btn-outline');
    });
    const activeBtn = document.getElementById('tab-' + tab);
    if (activeBtn) {
        activeBtn.classList.remove('btn-outline');
        activeBtn.classList.add('btn-accent');
    }

    const content = document.getElementById('settings-content');
    
    switch(tab) {
        case 'dashboards':
            content.innerHTML = await window.renderDashboardSettings();
            window.loadDashboardList();
            break;
        case 'alerts':
            content.innerHTML = await window.renderAlertSettings();
            window.loadThresholdList();
            window.loadChannelList();
            window.loadAlertHistory();
            break;
        case 'servers':
            content.innerHTML = await window.renderServerSettings();
            window.loadServerList();
            break;
        case 'collection':
            content.innerHTML = await window.renderCollectionSettings();
            break;
        case 'import-export':
            content.innerHTML = await window.renderImportExportSettings();
            break;
    }
};

// ========== CUSTOM DIALOG (replaces browser prompt/alert/confirm) ==========

window.showOptimaDialog = function(options) {
    return new Promise((resolve) => {
        const overlay = document.createElement('div');
        overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.6);z-index:100000;display:flex;align-items:center;justify-content:center;';
        
        let inputHtml = '';
        if (options.input) {
            const inputType = options.inputType || 'text';
            const inputVal = options.value || '';
            inputHtml = `<input type="${inputType}" id="optima-dialog-input" value="${window.escapeHtml(inputVal)}" style="width:100%;padding:0.5rem;margin-top:0.75rem;background:var(--bg-secondary);color:var(--text);border:1px solid var(--border-color);border-radius:4px;font-size:0.85rem;">`;
        }

        overlay.innerHTML = `
            <div style="background:var(--bg-primary);border:1px solid var(--border-color);border-radius:8px;padding:1.5rem;max-width:400px;width:90%;box-shadow:0 8px 32px rgba(0,0,0,0.4);">
                <div style="display:flex;align-items:center;gap:0.5rem;margin-bottom:1rem;">
                    <i class="fa-solid fa-database text-accent"></i>
                    <strong style="font-size:1rem;">SQL Optima</strong>
                </div>
                <p style="margin:0 0 0.5rem 0;color:var(--text);font-size:0.85rem;">${options.message || ''}</p>
                ${inputHtml}
                <div id="optima-dialog-error" style="color:var(--danger);font-size:0.75rem;margin-top:0.25rem;display:none;"></div>
                <div style="display:flex;gap:0.5rem;justify-content:flex-end;margin-top:1rem;">
                    ${options.cancel !== false ? `<button id="optima-dialog-cancel" class="btn btn-sm btn-outline">Cancel</button>` : ''}
                    <button id="optima-dialog-ok" class="btn btn-sm btn-accent">${options.okText || 'OK'}</button>
                </div>
            </div>
        `;

        document.body.appendChild(overlay);

        const inputEl = document.getElementById('optima-dialog-input');
        const errorEl = document.getElementById('optima-dialog-error');
        const okBtn = document.getElementById('optima-dialog-ok');
        const cancelBtn = document.getElementById('optima-dialog-cancel');

        if (inputEl) {
            inputEl.focus();
            inputEl.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') okBtn.click();
                if (e.key === 'Escape') cancelBtn?.click();
            });
        }

        okBtn.addEventListener('click', () => {
            if (options.validate) {
                const val = inputEl ? inputEl.value : true;
                const err = options.validate(val);
                if (err) {
                    errorEl.textContent = err;
                    errorEl.style.display = 'block';
                    inputEl.focus();
                    return;
                }
            }
            errorEl.style.display = 'none';
            document.body.removeChild(overlay);
            resolve(inputEl ? inputEl.value : true);
        });

        if (cancelBtn) {
            cancelBtn.addEventListener('click', () => {
                document.body.removeChild(overlay);
                resolve(null);
            });
        }
    });
};

window.optimaAlert = async function(message) {
    await window.showOptimaDialog({ message: message, cancel: false, okText: 'OK' });
};

window.optimaConfirm = async function(message) {
    const result = await window.showOptimaDialog({ message: message, okText: 'Confirm' });
    return result !== null;
};

window.optimaPrompt = async function(message, defaultValue, options) {
    return await window.showOptimaDialog({
        message: message,
        input: true,
        value: defaultValue || '',
        inputType: options?.inputType || 'text',
        validate: options?.validate,
        okText: options?.okText || 'OK'
    });
};

// ========== RENDER FUNCTIONS ==========

window.renderDashboardSettings = async function() {
    return `
        <div class="glass-panel" style="padding:1rem;">
            <div class="flex-between" style="margin-bottom:1rem;">
                <h3><i class="fa-solid fa-chart-line text-accent"></i> Custom Dashboards</h3>
                <button class="btn btn-sm btn-accent" onclick="window.createNewDashboard()">
                    <i class="fa-solid fa-plus"></i> New Dashboard
                </button>
            </div>
            <div id="dashboard-list" style="display:grid; gap:0.75rem;">
                <div class="text-center text-muted">Loading dashboards...</div>
            </div>
        </div>
        <div class="glass-panel mt-3" style="padding:1rem;">
            <h3><i class="fa-solid fa-puzzle-piece text-accent"></i> Available Metrics</h3>
            <p class="text-muted" style="font-size:0.85rem;">Select metrics to add to your custom dashboards</p>
            <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(200px, 1fr)); gap:0.5rem; margin-top:0.5rem;">
                <div class="metric-option" style="padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; cursor:pointer; display:flex; align-items:center; gap:0.5rem;" onclick="window.toggleMetricSelection(this)">
                    <input type="checkbox" value="cpu_usage" style="pointer-events:none;"> <span>CPU Usage</span>
                </div>
                <div class="metric-option" style="padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; cursor:pointer; display:flex; align-items:center; gap:0.5rem;" onclick="window.toggleMetricSelection(this)">
                    <input type="checkbox" value="memory_usage" style="pointer-events:none;"> <span>Memory Usage</span>
                </div>
                <div class="metric-option" style="padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; cursor:pointer; display:flex; align-items:center; gap:0.5rem;" onclick="window.toggleMetricSelection(this)">
                    <input type="checkbox" value="disk_io" style="pointer-events:none;"> <span>Disk I/O</span>
                </div>
                <div class="metric-option" style="padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; cursor:pointer; display:flex; align-items:center; gap:0.5rem;" onclick="window.toggleMetricSelection(this)">
                    <input type="checkbox" value="active_connections" style="pointer-events:none;"> <span>Active Connections</span>
                </div>
                <div class="metric-option" style="padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; cursor:pointer; display:flex; align-items:center; gap:0.5rem;" onclick="window.toggleMetricSelection(this)">
                    <input type="checkbox" value="tps" style="pointer-events:none;"> <span>Transactions/sec</span>
                </div>
                <div class="metric-option" style="padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; cursor:pointer; display:flex; align-items:center; gap:0.5rem;" onclick="window.toggleMetricSelection(this)">
                    <input type="checkbox" value="wait_stats" style="pointer-events:none;"> <span>Wait Statistics</span>
                </div>
                <div class="metric-option" style="padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; cursor:pointer; display:flex; align-items:center; gap:0.5rem;" onclick="window.toggleMetricSelection(this)">
                    <input type="checkbox" value="query_performance" style="pointer-events:none;"> <span>Query Performance</span>
                </div>
                <div class="metric-option" style="padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; cursor:pointer; display:flex; align-items:center; gap:0.5rem;" onclick="window.toggleMetricSelection(this)">
                    <input type="checkbox" value="locks" style="pointer-events:none;"> <span>Lock Statistics</span>
                </div>
            </div>
        </div>
    `;
};

window.renderAlertSettings = async function() {
    return `
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:0.75rem;">
            <div class="glass-panel" style="padding:0.75rem;">
                <div class="flex-between" style="margin-bottom:0.75rem;">
                    <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-bell text-accent"></i> Alert Thresholds</h3>
                    <button class="btn btn-xs btn-accent" onclick="window.createNewThreshold()"><i class="fa-solid fa-plus"></i></button>
                </div>
                <div id="threshold-list" style="display:flex; flex-direction:column; gap:0.5rem; max-height:350px; overflow-y:auto;">
                    <div class="text-center text-muted" style="font-size:0.8rem;">Loading...</div>
                </div>
            </div>
            <div class="glass-panel" style="padding:0.75rem;">
                <div class="flex-between" style="margin-bottom:0.75rem;">
                    <h3 style="font-size:0.85rem; margin:0;"><i class="fa-solid fa-paper-plane text-accent"></i> Notification Channels</h3>
                    <button class="btn btn-xs btn-accent" onclick="window.createNewChannel()"><i class="fa-solid fa-plus"></i></button>
                </div>
                <div id="channel-list" style="display:flex; flex-direction:column; gap:0.5rem; max-height:350px; overflow-y:auto;">
                    <div class="text-center text-muted" style="font-size:0.8rem;">Loading...</div>
                </div>
            </div>
        </div>
        <div class="glass-panel mt-3" style="padding:0.75rem;">
            <h3 style="font-size:0.85rem; margin:0 0 0.5rem 0;"><i class="fa-solid fa-history text-accent"></i> Alert History</h3>
            <div class="table-responsive" style="max-height:200px; overflow-y:auto;">
                <table class="data-table" style="font-size:0.7rem;">
                    <thead><tr><th>Time</th><th>Instance</th><th>Metric</th><th>Value</th><th>Severity</th><th>Message</th><th>Status</th></tr></thead>
                    <tbody id="alert-history-body"><tr><td colspan="7" class="text-center text-muted">No alerts recorded.</td></tr></tbody>
                </table>
            </div>
        </div>
    `;
};

window.renderServerSettings = async function() {
    return `
        <div class="glass-panel" style="padding:1rem;">
            <div class="flex-between" style="margin-bottom:1rem;">
                <h3><i class="fa-solid fa-server text-accent"></i> Monitored Servers</h3>
                <button class="btn btn-sm btn-accent" onclick="window.addNewServer()">
                    <i class="fa-solid fa-plus"></i> Add Server
                </button>
            </div>
            <div id="server-list" style="display:grid; gap:0.75rem;">
                <div class="text-center text-muted">Loading servers...</div>
            </div>
            <div class="mt-4" style="border-top:1px solid var(--border-color); padding-top:1rem;">
                <h4><i class="fa-solid fa-file-code text-accent"></i> Add via Configuration File</h4>
                <p class="text-muted" style="font-size:0.85rem;">Add servers by importing a JSON configuration file</p>
                <div style="display:flex; gap:0.5rem; align-items:center;">
                    <input type="file" id="server-config-file" accept=".json" style="font-size:0.85rem;">
                    <button class="btn btn-sm btn-outline" onclick="window.importServerConfig()">Import</button>
                </div>
            </div>
        </div>
    `;
};

window.renderCollectionSettings = async function() {
    return `
        <div class="glass-panel" style="padding:1rem;">
            <h3><i class="fa-solid fa-clock text-accent"></i> Metric Collection Settings</h3>
            <p class="text-muted" style="font-size:0.85rem;">Configure which metrics to collect and their intervals</p>
            <div style="display:grid; gap:1rem; margin-top:1rem;">
                <div class="glass-panel" style="padding:0.75rem;">
                    <div class="flex-between">
                        <div><strong>CPU Metrics</strong><div class="text-muted" style="font-size:0.75rem;">Collect CPU utilization and per-core metrics</div></div>
                        <label class="switch"><input type="checkbox" checked onchange="window.toggleCollection('cpu', this.checked)"><span class="slider"></span></label>
                    </div>
                    <div style="margin-top:0.5rem;"><label style="font-size:0.8rem;">Collection Interval:</label>
                        <select class="custom-select" style="width:auto;" onchange="window.updateCollectionInterval('cpu', this.value)">
                            <option value="5s">5 seconds</option><option value="15s" selected>15 seconds</option><option value="30s">30 seconds</option><option value="1m">1 minute</option>
                        </select>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <div class="flex-between">
                        <div><strong>Memory Metrics</strong><div class="text-muted" style="font-size:0.75rem;">Collect memory usage and buffer pool metrics</div></div>
                        <label class="switch"><input type="checkbox" checked onchange="window.toggleCollection('memory', this.checked)"><span class="slider"></span></label>
                    </div>
                    <div style="margin-top:0.5rem;"><label style="font-size:0.8rem;">Collection Interval:</label>
                        <select class="custom-select" style="width:auto;" onchange="window.updateCollectionInterval('memory', this.value)">
                            <option value="5s">5 seconds</option><option value="15s" selected>15 seconds</option><option value="30s">30 seconds</option><option value="1m">1 minute</option>
                        </select>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <div class="flex-between">
                        <div><strong>Query Performance</strong><div class="text-muted" style="font-size:0.75rem;">Collect top queries and execution stats</div></div>
                        <label class="switch"><input type="checkbox" checked onchange="window.toggleCollection('queries', this.checked)"><span class="slider"></span></label>
                    </div>
                    <div style="margin-top:0.5rem; display:flex; align-items:center; gap:1rem;">
                        <div><label style="font-size:0.8rem;">Interval:</label>
                            <select class="custom-select" style="width:auto;" onchange="window.updateCollectionInterval('queries', this.value)">
                                <option value="15s">15 seconds</option><option value="30s">30 seconds</option><option value="1m" selected>1 minute</option><option value="5m">5 minutes</option>
                            </select>
                        </div>
                        <div><label style="font-size:0.8rem;">Query Threshold (ms):</label>
                            <input type="number" id="query-threshold-input" class="custom-input" style="width:100px;" value="1000" min="100" onchange="window.updateQueryThreshold(this.value)">
                        </div>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <div class="flex-between">
                        <div><strong>Wait Statistics</strong><div class="text-muted" style="font-size:0.75rem;">Collect wait type and duration metrics</div></div>
                        <label class="switch"><input type="checkbox" checked onchange="window.toggleCollection('waits', this.checked)"><span class="slider"></span></label>
                    </div>
                    <div style="margin-top:0.5rem;"><label style="font-size:0.8rem;">Collection Interval:</label>
                        <select class="custom-select" style="width:auto;" onchange="window.updateCollectionInterval('waits', this.value)">
                            <option value="15s">15 seconds</option><option value="30s" selected>30 seconds</option><option value="1m">1 minute</option><option value="5m">5 minutes</option>
                        </select>
                    </div>
                </div>
            </div>
            <div class="mt-3"><button class="btn btn-accent" onclick="window.saveCollectionSettings()"><i class="fa-solid fa-save"></i> Save Settings</button></div>
        </div>
    `;
};

window.renderImportExportSettings = async function() {
    return `
        <div class="glass-panel" style="padding:1rem;">
            <h3><i class="fa-solid fa-download text-accent"></i> Export Configuration</h3>
            <p class="text-muted" style="font-size:0.85rem;">Export your dashboards, alerts, and server configurations</p>
            <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(180px, 1fr)); gap:0.75rem; margin-top:1rem;">
                <div class="glass-panel" style="padding:0.75rem; text-align:center;">
                    <i class="fa-solid fa-chart-line" style="font-size:1.5rem; color:var(--accent);"></i>
                    <h4 style="margin:0.5rem 0; font-size:0.85rem;">Dashboards</h4>
                    <button class="btn btn-xs btn-outline" onclick="window.exportConfig('dashboard')">Export</button>
                </div>
                <div class="glass-panel" style="padding:0.75rem; text-align:center;">
                    <i class="fa-solid fa-bell" style="font-size:1.5rem; color:var(--warning);"></i>
                    <h4 style="margin:0.5rem 0; font-size:0.85rem;">Alerts</h4>
                    <button class="btn btn-xs btn-outline" onclick="window.exportConfig('alerts')">Export</button>
                </div>
                <div class="glass-panel" style="padding:0.75rem; text-align:center;">
                    <i class="fa-solid fa-server" style="font-size:1.5rem; color:var(--success);"></i>
                    <h4 style="margin:0.5rem 0; font-size:0.85rem;">Servers</h4>
                    <button class="btn btn-xs btn-outline" onclick="window.exportConfig('servers')">Export</button>
                </div>
                <div class="glass-panel" style="padding:0.75rem; text-align:center;">
                    <i class="fa-solid fa-box-archive" style="font-size:1.5rem; color:var(--info);"></i>
                    <h4 style="margin:0.5rem 0; font-size:0.85rem;">Full Export</h4>
                    <button class="btn btn-xs btn-accent" onclick="window.exportConfig('full')">Export All</button>
                </div>
            </div>
        </div>
        <div class="glass-panel mt-3" style="padding:1rem;">
            <h3><i class="fa-solid fa-upload text-accent"></i> Import Configuration</h3>
            <p class="text-muted" style="font-size:0.85rem;">Import configuration from a previously exported file</p>
            <div style="display:flex; gap:0.5rem; align-items:center; margin-top:1rem;">
                <input type="file" id="import-config-file" accept=".json" style="font-size:0.85rem;">
                <button class="btn btn-accent" onclick="window.importConfig()"><i class="fa-solid fa-upload"></i> Import</button>
            </div>
        </div>
    `;
};

// ========== DATA LOADING ==========

window.loadDashboardList = function() {
    const container = document.getElementById('dashboard-list');
    if (!container) return;
    const dashboards = window.appState.config.dashboards || [];
    if (dashboards.length === 0) {
        container.innerHTML = '<div class="text-center text-muted" style="padding:2rem;"><i class="fa-solid fa-chart-line" style="font-size:2rem; display:block; margin-bottom:0.5rem;"></i>No custom dashboards yet. Click "New Dashboard" to create one.</div>';
        return;
    }
    container.innerHTML = dashboards.map((d, i) => `
        <div class="glass-panel" style="padding:0.75rem; display:flex; justify-content:space-between; align-items:center;">
            <div><strong>${window.escapeHtml(d.name || 'Dashboard ' + (i + 1))}</strong><div class="text-muted" style="font-size:0.75rem;">${(d.metrics || []).length} metrics | ${d.type || 'custom'}</div></div>
            <div style="display:flex; gap:0.5rem;">
                <button class="btn btn-sm btn-outline" onclick="window.editDashboard(${i})"><i class="fa-solid fa-edit"></i></button>
                <button class="btn btn-sm btn-outline" style="border-color:var(--danger); color:var(--danger);" onclick="window.deleteDashboard(${i})"><i class="fa-solid fa-trash"></i></button>
            </div>
        </div>
    `).join('');
};

window.loadThresholdList = function() {
    const container = document.getElementById('threshold-list');
    if (!container) return;
    const thresholds = window.appState.config.alertThresholds || [];
    if (thresholds.length === 0) {
        container.innerHTML = '<div class="text-center text-muted" style="font-size:0.8rem; padding:1rem;">No thresholds configured.</div>';
        return;
    }
    container.innerHTML = thresholds.map((t, i) => `
        <div style="display:flex; justify-content:space-between; align-items:center; padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; font-size:0.8rem;">
            <div><strong>${window.escapeHtml(t.metric || t.name)}</strong> <span class="text-muted">${t.condition || '>'} ${t.value || 0} | ${t.severity || 'warning'}</span></div>
            <div style="display:flex; gap:0.25rem;">
                <button class="btn btn-xs btn-outline" onclick="window.editThreshold(${i})"><i class="fa-solid fa-edit"></i></button>
                <button class="btn btn-xs btn-outline" style="border-color:var(--danger); color:var(--danger);" onclick="window.deleteThreshold(${i})"><i class="fa-solid fa-trash"></i></button>
            </div>
        </div>
    `).join('');
};

window.loadChannelList = function() {
    const container = document.getElementById('channel-list');
    if (!container) return;
    const channels = window.appState.config.notificationChannels || [];
    if (channels.length === 0) {
        container.innerHTML = '<div class="text-center text-muted" style="font-size:0.8rem; padding:1rem;">No channels configured.</div>';
        return;
    }
    container.innerHTML = channels.map((c, i) => `
        <div style="display:flex; justify-content:space-between; align-items:center; padding:0.5rem; background:var(--bg-tertiary); border-radius:4px; font-size:0.8rem;">
            <div><strong><i class="fa-solid fa-${c.type === 'email' ? 'envelope' : c.type === 'slack' ? 'slack' : 'webhook'}"></i> ${window.escapeHtml(c.name || c.type)}</strong><div class="text-muted" style="font-size:0.7rem;">${window.escapeHtml(c.endpoint || c.email || '')}</div></div>
            <div style="display:flex; gap:0.25rem;">
                <button class="btn btn-xs btn-outline" onclick="window.editChannel(${i})"><i class="fa-solid fa-edit"></i></button>
                <button class="btn btn-xs btn-outline" style="border-color:var(--danger); color:var(--danger);" onclick="window.deleteChannel(${i})"><i class="fa-solid fa-trash"></i></button>
            </div>
        </div>
    `).join('');
};

window.loadAlertHistory = function() {
    const tbody = document.getElementById('alert-history-body');
    if (!tbody) return;
    const alerts = window.appState.recentAlerts || [];
    if (alerts.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">No alerts recorded.</td></tr>';
        return;
    }
    tbody.innerHTML = alerts.slice(0, 20).map(a => {
        const sevBadge = a.severity === 'critical' ? 'badge-danger' : a.severity === 'warning' ? 'badge-warning' : 'badge-info';
        const statusBadge = a.acknowledged ? 'badge-success' : 'badge-warning';
        return `<tr><td>${window.escapeHtml(a.timestamp || '-')}</td><td>${window.escapeHtml(a.instance || '-')}</td><td>${window.escapeHtml(a.metric || '-')}</td><td>${a.value || '-'}</td><td><span class="badge ${sevBadge}">${window.escapeHtml(a.severity || '-')}</span></td><td>${window.escapeHtml(a.message || '-')}</td><td><span class="badge ${statusBadge}">${a.acknowledged ? 'Ack' : 'Active'}</span></td></tr>`;
    }).join('');
};

window.loadServerList = function() {
    const container = document.getElementById('server-list');
    if (!container) return;
    const servers = window.appState.config.instances || [];
    if (servers.length === 0) {
        container.innerHTML = '<div class="text-center text-muted" style="padding:2rem;"><i class="fa-solid fa-server" style="font-size:2rem; display:block; margin-bottom:0.5rem;"></i>No servers configured. Click "Add Server" to add one.</div>';
        return;
    }
    container.innerHTML = servers.map((s, i) => {
        const typeBadge = s.type === 'postgres' ? 'badge-success' : 'badge-info';
        const typeLabel = s.type === 'postgres' ? 'PostgreSQL' : 'SQL Server';
        return `
            <div class="glass-panel" style="padding:0.75rem; display:flex; justify-content:space-between; align-items:center;">
                <div><strong>${window.escapeHtml(s.name)}</strong><span class="badge ${typeBadge}" style="margin-left:0.5rem;">${typeLabel}</span><div class="text-muted" style="font-size:0.75rem;">${window.escapeHtml(s.host || '')}:${s.port || (s.type === 'postgres' ? 5432 : 1433)} | ${window.escapeHtml(s.user || '')}</div></div>
                <div style="display:flex; gap:0.5rem;">
                    <button class="btn btn-sm btn-outline" onclick="window.editServer(${i})"><i class="fa-solid fa-edit"></i></button>
                    <button class="btn btn-sm btn-outline" style="border-color:var(--danger); color:var(--danger);" onclick="window.deleteServer(${i})"><i class="fa-solid fa-trash"></i></button>
                </div>
            </div>
        `;
    }).join('');
};

// ========== CRUD ==========

window.toggleMetricSelection = function(el) {
    const checkbox = el.querySelector('input[type="checkbox"]');
    checkbox.checked = !checkbox.checked;
    if (checkbox.checked) {
        el.style.background = 'var(--accent)';
        el.style.color = '#fff';
    } else {
        el.style.background = 'var(--bg-tertiary)';
        el.style.color = 'var(--text)';
    }
};

window.createNewDashboard = async function() {
    const name = await window.optimaPrompt('Enter dashboard name:', '', {
        validate: (val) => {
            if (!val || !val.trim()) return 'Dashboard name is required.';
            if (val.length > 50) return 'Name must be 50 characters or less.';
            return null;
        }
    });
    if (!name) return;

    if (!window.appState.config.dashboards) window.appState.config.dashboards = [];
    window.appState.config.dashboards.push({
        name: name.trim(),
        type: 'custom',
        metrics: [],
        created: new Date().toISOString()
    });

    await window.optimaAlert('Dashboard "' + name.trim() + '" created successfully!');
    window.loadDashboardList();
};

window.editDashboard = async function(index) {
    const dashboards = window.appState.config.dashboards || [];
    const d = dashboards[index];
    if (!d) return;

    const name = await window.optimaPrompt('Edit dashboard name:', d.name, {
        validate: (val) => {
            if (!val || !val.trim()) return 'Dashboard name is required.';
            return null;
        }
    });
    if (name) {
        d.name = name.trim();
        await window.optimaAlert('Dashboard updated!');
        window.loadDashboardList();
    }
};

window.deleteDashboard = async function(index) {
    const dashboards = window.appState.config.dashboards || [];
    const d = dashboards[index];
    if (!d) return;

    const ok = await window.optimaConfirm('Delete dashboard "' + window.escapeHtml(d.name) + '"?');
    if (!ok) return;
    window.appState.config.dashboards.splice(index, 1);
    window.loadDashboardList();
};

window.createNewThreshold = async function() {
    const metric = await window.optimaPrompt('Enter metric name (e.g., cpu_usage):', '', {
        validate: (val) => (!val || !val.trim()) ? 'Metric name is required.' : null
    });
    if (!metric) return;

    const value = await window.optimaPrompt('Enter threshold value:', '90', {
        inputType: 'number',
        validate: (val) => {
            if (!val || isNaN(parseFloat(val))) return 'Enter a valid number.';
            return null;
        }
    });
    if (value === null) return;

    const condition = await window.optimaPrompt('Enter condition (>, <, >=, <=):', '>');
    if (!condition) return;

    const severity = await window.optimaPrompt('Enter severity (info, warning, critical):', 'warning');
    if (!severity) return;

    if (!window.appState.config.alertThresholds) window.appState.config.alertThresholds = [];
    window.appState.config.alertThresholds.push({
        metric: metric.trim(),
        value: parseFloat(value),
        condition: condition.trim() || '>',
        severity: severity.trim() || 'warning'
    });

    await window.optimaAlert('Threshold created!');
    window.loadThresholdList();
};

window.editThreshold = async function(index) {
    const thresholds = window.appState.config.alertThresholds || [];
    const t = thresholds[index];
    if (!t) return;

    const value = await window.optimaPrompt('Edit threshold value:', String(t.value), {
        inputType: 'number',
        validate: (val) => (!val || isNaN(parseFloat(val))) ? 'Enter a valid number.' : null
    });
    if (value !== null) {
        t.value = parseFloat(value);
        await window.optimaAlert('Threshold updated!');
        window.loadThresholdList();
    }
};

window.deleteThreshold = async function(index) {
    const ok = await window.optimaConfirm('Delete this threshold?');
    if (!ok) return;
    window.appState.config.alertThresholds.splice(index, 1);
    window.loadThresholdList();
};

window.createNewChannel = async function() {
    const name = await window.optimaPrompt('Enter channel name:', '', {
        validate: (val) => (!val || !val.trim()) ? 'Channel name is required.' : null
    });
    if (!name) return;

    const type = await window.optimaPrompt('Enter channel type (email, slack, webhook):', 'webhook');
    if (!type) return;

    const endpoint = await window.optimaPrompt('Enter endpoint URL or email address:', '', {
        validate: (val) => (!val || !val.trim()) ? 'Endpoint is required.' : null
    });
    if (!endpoint) return;

    if (!window.appState.config.notificationChannels) window.appState.config.notificationChannels = [];
    window.appState.config.notificationChannels.push({
        name: name.trim(),
        type: type.trim() || 'webhook',
        endpoint: endpoint.trim()
    });

    await window.optimaAlert('Channel "' + name.trim() + '" created!');
    window.loadChannelList();
};

window.editChannel = async function(index) {
    const channels = window.appState.config.notificationChannels || [];
    const c = channels[index];
    if (!c) return;

    const endpoint = await window.optimaPrompt('Edit endpoint:', c.endpoint);
    if (endpoint !== null) {
        c.endpoint = endpoint.trim();
        await window.optimaAlert('Channel updated!');
        window.loadChannelList();
    }
};

window.deleteChannel = async function(index) {
    const ok = await window.optimaConfirm('Delete this channel?');
    if (!ok) return;
    window.appState.config.notificationChannels.splice(index, 1);
    window.loadChannelList();
};

window.addNewServer = async function() {
    const name = await window.optimaPrompt('Enter server name:', '', {
        validate: (val) => {
            if (!val || !val.trim()) return 'Server name is required.';
            if (val.length > 50) return 'Name must be 50 characters or less.';
            if (window.appState.config.instances && window.appState.config.instances.some(s => s.name === val.trim())) return 'A server with this name already exists.';
            return null;
        }
    });
    if (!name) return;

    const host = await window.optimaPrompt('Enter server host/IP:', '', {
        validate: (val) => {
            if (!val || !val.trim()) return 'Host/IP address is required.';
            return null;
        }
    });
    if (!host) return;

    const port = await window.optimaPrompt('Enter port (5432 for PG, 1433 for SQL Server):', '5432', {
        inputType: 'number',
        validate: (val) => {
            const p = parseInt(val);
            if (!val || isNaN(p) || p < 1 || p > 65535) return 'Enter a valid port number (1-65535).';
            return null;
        }
    });
    if (port === null) return;

    const type = await window.optimaPrompt('Enter type (postgres, sqlserver):', 'postgres', {
        validate: (val) => {
            const t = (val || '').toLowerCase().trim();
            if (t !== 'postgres' && t !== 'sqlserver') return 'Type must be "postgres" or "sqlserver".';
            return null;
        }
    });
    if (!type) return;

    const user = await window.optimaPrompt('Enter username:', 'postgres', {
        validate: (val) => (!val || !val.trim()) ? 'Username is required.' : null
    });
    if (!user) return;

    const password = await window.optimaPrompt('Enter password:', '', {
        inputType: 'password',
        validate: (val) => (!val || !val.trim()) ? 'Password is required.' : null
    });
    if (!password) return;

    if (!window.appState.config.instances) window.appState.config.instances = [];
    window.appState.config.instances.push({
        name: name.trim(),
        host: host.trim(),
        port: parseInt(port),
        type: type.toLowerCase().trim(),
        user: user.trim(),
        password: password,
        databases: []
    });

    await window.optimaAlert('Server "' + name.trim() + '" added! Please restart the application to apply changes.');
    window.loadServerList();
};

window.editServer = async function(index) {
    const servers = window.appState.config.instances || [];
    const s = servers[index];
    if (!s) return;

    const host = await window.optimaPrompt('Edit host/IP:', s.host, {
        validate: (val) => (!val || !val.trim()) ? 'Host/IP is required.' : null
    });
    if (host) {
        s.host = host.trim();
        await window.optimaAlert('Server updated! Please restart to apply changes.');
        window.loadServerList();
    }
};

window.deleteServer = async function(index) {
    const servers = window.appState.config.instances || [];
    const s = servers[index];
    if (!s) return;

    const ok = await window.optimaConfirm('Delete server "' + window.escapeHtml(s.name) + '"? This cannot be undone.');
    if (!ok) return;
    window.appState.config.instances.splice(index, 1);
    await window.optimaAlert('Server removed! Please restart the application to apply changes.');
    window.loadServerList();
};

window.importServerConfig = function() {
    const fileInput = document.getElementById('server-config-file');
    if (!fileInput || fileInput.files.length === 0) {
        window.optimaAlert('Please select a JSON configuration file first.');
        return;
    }

    const reader = new FileReader();
    reader.onload = function(e) {
        try {
            const data = JSON.parse(e.target.result);
            if (!data.instances || !Array.isArray(data.instances)) {
                window.optimaAlert('Invalid configuration file: missing "instances" array.');
                return;
            }

            let added = 0;
            let skipped = 0;
            if (!window.appState.config.instances) window.appState.config.instances = [];

            data.instances.forEach(newInst => {
                if (!newInst.name || !newInst.host) { skipped++; return; }
                if (window.appState.config.instances.some(s => s.name === newInst.name)) { skipped++; return; }
                window.appState.config.instances.push(newInst);
                added++;
            });

            window.optimaAlert(`Import complete.\nAdded: ${added}\nSkipped (duplicate/invalid): ${skipped}\n\nPlease restart the application to apply changes.`);
            window.loadServerList();
        } catch (err) {
            window.optimaAlert('Invalid JSON file: ' + err.message);
        }
    };
    reader.readAsText(fileInput.files[0]);
};

// ========== COLLECTION SETTINGS ==========

window._collectionSettings = window._collectionSettings || {
    cpu: { enabled: true, interval: '15s' },
    memory: { enabled: true, interval: '15s' },
    queries: { enabled: true, interval: '1m', threshold: 1000 },
    waits: { enabled: true, interval: '30s' }
};

window.toggleCollection = function(category, checked) {
    if (window._collectionSettings[category]) {
        window._collectionSettings[category].enabled = checked;
    }
};

window.updateCollectionInterval = function(category, value) {
    if (window._collectionSettings[category]) {
        window._collectionSettings[category].interval = value;
    }
};

window.updateQueryThreshold = function(value) {
    if (window._collectionSettings.queries) {
        window._collectionSettings.queries.threshold = parseInt(value) || 1000;
    }
};

window.saveCollectionSettings = async function() {
    try {
        localStorage.setItem('sqlOptima_collectionSettings', JSON.stringify(window._collectionSettings));
        await window.optimaAlert('Collection settings saved successfully!');
    } catch (err) {
        await window.optimaAlert('Failed to save settings: ' + err.message);
    }
};

// ========== EXPORT / IMPORT ==========

window.exportConfig = async function(type) {
    let exportData = {};
    let sectionCount = 0;

    switch(type) {
        case 'dashboard':
            exportData = {
                type: 'dashboards',
                timestamp: new Date().toISOString(),
                version: '1.0',
                dashboards: window.appState.config.dashboards || []
            };
            sectionCount = (exportData.dashboards || []).length;
            break;
        case 'alerts':
            exportData = {
                type: 'alerts',
                timestamp: new Date().toISOString(),
                version: '1.0',
                thresholds: window.appState.config.alertThresholds || [],
                channels: window.appState.config.notificationChannels || []
            };
            sectionCount = (exportData.thresholds || []).length + (exportData.channels || []).length;
            break;
        case 'servers':
            exportData = {
                type: 'servers',
                timestamp: new Date().toISOString(),
                version: '1.0',
                instances: window.appState.config.instances || []
            };
            sectionCount = (exportData.instances || []).length;
            break;
        case 'full':
            exportData = {
                type: 'full_export',
                timestamp: new Date().toISOString(),
                version: '1.0',
                instances: window.appState.config.instances || [],
                dashboards: window.appState.config.dashboards || [],
                alertThresholds: window.appState.config.alertThresholds || [],
                notificationChannels: window.appState.config.notificationChannels || [],
                collectionSettings: window._collectionSettings || {}
            };
            sectionCount = (exportData.instances || []).length + (exportData.dashboards || []).length + (exportData.alertThresholds || []).length + (exportData.notificationChannels || []).length;
            break;
    }

    if (sectionCount === 0) {
        await window.optimaAlert('No data to export for "' + type + '". Please configure some items first.');
        return;
    }

    const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'sql-optima-' + type + '-' + Date.now() + '.json';
    a.click();
    URL.revokeObjectURL(url);

    await window.optimaAlert(`Exported ${sectionCount} item(s) to sql-optima-${type}-${Date.now()}.json`);
};

window.importConfig = function() {
    const fileInput = document.getElementById('import-config-file');
    if (!fileInput || fileInput.files.length === 0) {
        window.optimaAlert('Please select a configuration file first.');
        return;
    }

    const reader = new FileReader();
    reader.onload = function(e) {
        try {
            const data = JSON.parse(e.target.result);

            if (!data.type || !data.timestamp) {
                window.optimaAlert('Invalid configuration file: missing required fields (type, timestamp).');
                return;
            }

            let imported = 0;
            let skipped = 0;

            if (data.instances && Array.isArray(data.instances)) {
                if (!window.appState.config.instances) window.appState.config.instances = [];
                data.instances.forEach(inst => {
                    if (!inst.name || !inst.host) { skipped++; return; }
                    if (window.appState.config.instances.some(s => s.name === inst.name)) { skipped++; return; }
                    window.appState.config.instances.push(inst);
                    imported++;
                });
            }
            if (data.dashboards && Array.isArray(data.dashboards)) {
                window.appState.config.dashboards = data.dashboards;
                imported++;
            }
            if (data.alertThresholds && Array.isArray(data.alertThresholds)) {
                window.appState.config.alertThresholds = data.alertThresholds;
                imported++;
            }
            if (data.notificationChannels && Array.isArray(data.notificationChannels)) {
                window.appState.config.notificationChannels = data.notificationChannels;
                imported++;
            }
            if (data.collectionSettings && typeof data.collectionSettings === 'object') {
                window._collectionSettings = data.collectionSettings;
                imported++;
            }

            if (imported === 0) {
                window.optimaAlert('No valid configuration data found in the file.');
                return;
            }

            window.optimaAlert(`Configuration imported successfully!\nImported: ${imported} section(s)\nSkipped (duplicates/invalid): ${skipped}\n\nPlease restart the application to apply server changes.`);
            window.showSettingsTab('servers');
        } catch (err) {
            window.optimaAlert('Invalid configuration file: ' + err.message);
        }
    };
    reader.readAsText(fileInput.files[0]);
};
