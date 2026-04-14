/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL configuration parameter management.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgConfigView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...'};

    let configData = { settings: [] };
    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/postgres/config?instance=${encodeURIComponent(inst.name)}`
        );
        if (response.ok) {
            const contentType = response.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                configData = await response.json();
            }
        } else {
            console.error("Failed to load PG config:", response.status);
        }
    } catch (e) {
        console.error("PG config fetch failed:", e);
    }

    const settings = configData.settings || [];
    const getSetting = (name) => settings.find(s => s.name === name);
    const sharedBuffers = getSetting('shared_buffers');
    const workMem = getSetting('work_mem');
    const maintWorkMem = getSetting('maintenance_work_mem');
    const maxConn = getSetting('max_connections');

    const assessSetting = (setting) => {
        if (!setting) return { status: 'unknown', message: 'Not found' };

        const val = parseFloat(setting.value);
        switch (setting.name) {
            case 'shared_buffers':
                const gb = val / (1024 * 1024 * 1024);
                return gb > 1 ? { status: 'healthy', message: `~${gb.toFixed(1)}GB RAM` } : { status: 'warning', message: 'Low memory' };
            case 'work_mem':
                const mb = val / (1024 * 1024);
                return mb > 64 ? { status: 'warning', message: 'High per-query memory' } : { status: 'healthy', message: 'Reasonable' };
            case 'maintenance_work_mem':
                const maintMb = val / (1024 * 1024 * 1024);
                return maintMb > 0.5 ? { status: 'healthy', message: 'Good for maintenance' } : { status: 'warning', message: 'Low maintenance memory' };
            case 'max_connections':
                return val > 200 ? { status: 'danger', message: 'High connection count' } : { status: 'healthy', message: 'Reasonable connections' };
            default:
                return { status: 'info', message: setting.value };
        }
    };

    const sharedBuffersAssessment = assessSetting(sharedBuffers);
    const workMemAssessment = assessSetting(workMem);
    const maintWorkMemAssessment = assessSetting(maintWorkMem);
    const maxConnAssessment = assessSetting(maxConn);

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title">
                <h1><i class="fa-solid fa-sliders text-accent"></i> System Configuration Tracker</h1>
                <p class="subtitle">Review pg_settings parameters with highlights on risky or non-default values.</p>
            </div>

            <div class="metrics-row" style="display:grid; grid-template-columns:1fr 1fr; gap:0.75rem; margin-top:0.75rem;">
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-memory"></i> Memory Configuration</h4>
                    <div style="display:flex; gap:0.5rem; flex-wrap:wrap;">
                        <div class="metric-card glass-panel status-${sharedBuffersAssessment.status}" style="padding:0.4rem 0.75rem; flex:1; min-width:120px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">shared_buffers</span><i class="fa-solid fa-memory card-icon"></i></div>
                            <div class="metric-value" style="font-size:1rem">${sharedBuffers ? sharedBuffers.value : 'N/A'}</div>
                            <div class="metric-trend ${sharedBuffersAssessment.status === 'healthy' ? 'positive' : sharedBuffersAssessment.status === 'warning' ? 'warning' : 'danger'}" style="font-size:0.6rem">
                                <i class="fa-solid fa-${sharedBuffersAssessment.status === 'healthy' ? 'check' : sharedBuffersAssessment.status === 'warning' ? 'triangle-exclamation' : 'skull'}"></i>
                                ${sharedBuffersAssessment.message}
                            </div>
                        </div>
                        <div class="metric-card glass-panel status-${workMemAssessment.status}" style="padding:0.4rem 0.75rem; flex:1; min-width:120px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">work_mem</span><i class="fa-solid fa-microchip card-icon"></i></div>
                            <div class="metric-value" style="font-size:1rem">${workMem ? workMem.value : 'N/A'}</div>
                            <div class="metric-trend ${workMemAssessment.status === 'healthy' ? 'positive' : workMemAssessment.status === 'warning' ? 'warning' : 'danger'}" style="font-size:0.6rem">
                                <i class="fa-solid fa-${workMemAssessment.status === 'healthy' ? 'check' : workMemAssessment.status === 'warning' ? 'triangle-exclamation' : 'skull'}"></i>
                                ${workMemAssessment.message}
                            </div>
                        </div>
                        <div class="metric-card glass-panel status-${maintWorkMemAssessment.status}" style="padding:0.4rem 0.75rem; flex:1; min-width:120px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">maintenance_work_mem</span><i class="fa-solid fa-hammer card-icon"></i></div>
                            <div class="metric-value" style="font-size:0.9rem">${maintWorkMem ? maintWorkMem.value : 'N/A'}</div>
                            <div class="metric-trend ${maintWorkMemAssessment.status === 'healthy' ? 'positive' : maintWorkMemAssessment.status === 'warning' ? 'warning' : 'danger'}" style="font-size:0.6rem">
                                <i class="fa-solid fa-${maintWorkMemAssessment.status === 'healthy' ? 'check' : maintWorkMemAssessment.status === 'warning' ? 'triangle-exclamation' : 'skull'}"></i>
                                ${maintWorkMemAssessment.message}
                            </div>
                        </div>
                    </div>
                </div>
                <div class="glass-panel" style="padding:0.75rem;">
                    <h4 style="margin:0 0 0.5rem 0; color:var(--text-secondary,#888); font-size:0.75rem; text-transform:uppercase;"><i class="fa-solid fa-network-wired"></i> Connection & Safety</h4>
                    <div style="display:flex; gap:0.5rem; flex-wrap:wrap;">
                        <div class="metric-card glass-panel status-${maxConnAssessment.status}" style="padding:0.4rem 0.75rem; flex:1; min-width:120px;">
                            <div class="metric-header"><span class="metric-title" style="font-size:0.65rem">max_connections</span><i class="fa-solid fa-network-wired card-icon"></i></div>
                            <div class="metric-value" style="font-size:1rem">${maxConn ? maxConn.value : 'N/A'}</div>
                            <div class="metric-trend ${maxConnAssessment.status === 'healthy' ? 'positive' : maxConnAssessment.status === 'warning' ? 'warning' : 'danger'}" style="font-size:0.6rem">
                                <i class="fa-solid fa-${maxConnAssessment.status === 'healthy' ? 'check' : maxConnAssessment.status === 'warning' ? 'triangle-exclamation' : 'skull'}"></i>
                                ${maxConnAssessment.message}
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="tables-grid mt-3" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                <div class="table-card glass-panel">
                    <div class="card-header"><h3 style="font-size:0.85rem; margin:0;">All Configuration Parameters</h3></div>
                    <div class="table-responsive" style="max-height:500px; overflow-y:auto;">
                        <table class="data-table" style="font-size:0.7rem;">
                            <thead>
                                <tr><th>Parameter</th><th>Value</th><th>Unit</th><th>Category</th><th>Source</th><th>Assessment</th></tr>
                            </thead>
                            <tbody>
                                ${settings.length > 0 ? settings.map(setting => {
                                    const isDefault = setting.source === 'default';
                                    const isRisky = (setting.name === 'synchronous_commit' && setting.value === 'off') ||
                                                   (setting.name === 'fsync' && setting.value === 'off');
                                    return `
                                        <tr>
                                            <td><strong>${window.escapeHtml(setting.name)}</strong></td>
                                            <td>${window.escapeHtml(setting.value)}</td>
                                            <td>${window.escapeHtml(setting.unit || '-')}</td>
                                            <td><span class="badge ${setting.category && setting.category.includes('Autovacuum') ? 'badge-success' : setting.category && setting.category.includes('WAL') ? 'badge-danger' : 'badge-info'}">${window.escapeHtml(setting.category || 'Other')}</span></td>
                                            <td><span class="${setting.source === 'configuration file' ? 'text-accent' : 'text-muted'}">${window.escapeHtml(setting.source)}</span></td>
                                            <td>
                                                ${isRisky ?
                                                    '<span class="text-danger font-bold"><i class="fa-solid fa-triangle-exclamation"></i> RISKY</span>' :
                                                    isDefault ?
                                                        '<span class="text-muted">Default</span>' :
                                                        '<span class="text-success"><i class="fa-solid fa-check"></i> Configured</span>'
                                                }
                                            </td>
                                        </tr>
                                    `;
                                }).join('') : '<tr><td colspan="6" class="text-center text-muted">No configuration data available</td></tr>'}
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    `;
}
