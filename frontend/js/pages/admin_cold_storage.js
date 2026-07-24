// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: frontend/js/pages/admin_cold_storage.js
// Purpose: UI logic for monitoring cold storage archival status and recent execution runs.
//          Exported as a module function so it can be loaded as a tab from admin.js,
//          or used standalone from admin_cold_storage.html.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

/**
 * loadColdStorage renders the cold storage admin panel into `container`.
 * Called by admin.js when the user clicks the "Cold Storage" tab.
 * @param {HTMLElement} container - target element (defaults to #admin-content)
 */
export async function loadColdStorage(container) {
    const root = container || document.getElementById('admin-content');
    if (!root) return;

    root.innerHTML = `
        <div class="glass-panel" style="padding:1.25rem;border-radius:12px;">
            <div style="display:flex;align-items:center;gap:0.75rem;margin-bottom:1rem;flex-wrap:wrap;">
                <h2 style="font-size:1rem;margin:0;font-weight:600;">
                    <i class="fa-solid fa-snowflake text-accent"></i> Cold Storage
                </h2>
                <div style="display:flex;gap:0.5rem;margin-left:auto;">
                    <button class="btn btn-sm btn-accent" id="cs-tab-status">Archival Status</button>
                    <button class="btn btn-sm btn-outline" id="cs-tab-runs">Run History</button>
                </div>
            </div>
            <div id="cs-status-panel"></div>
            <div id="cs-runs-panel" style="display:none;"></div>
        </div>
    `;

    document.getElementById('cs-tab-status').addEventListener('click', () => switchTab('status'));
    document.getElementById('cs-tab-runs').addEventListener('click', () => switchTab('runs'));

    await renderStatusPanel();
}

function switchTab(tab) {
    document.getElementById('cs-status-panel').style.display = tab === 'status' ? '' : 'none';
    document.getElementById('cs-runs-panel').style.display = tab === 'runs' ? '' : 'none';
    document.getElementById('cs-tab-status').className = `btn btn-sm ${tab === 'status' ? 'btn-accent' : 'btn-outline'}`;
    document.getElementById('cs-tab-runs').className = `btn btn-sm ${tab === 'runs' ? 'btn-accent' : 'btn-outline'}`;
    if (tab === 'runs') renderRunsPanel();
}

async function renderStatusPanel() {
    const panel = document.getElementById('cs-status-panel');
    panel.innerHTML = '<div class="text-muted" style="padding:1rem;">Loading archival status…</div>';

    try {
        const resp = await window.apiClient.authenticatedFetch('/api/cold-storage/status');
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const data = await resp.json();

        if (!data || data.length === 0) {
            panel.innerHTML = '<p class="text-muted" style="padding:1rem;">No archival watermarks found. Has the first export cycle run?</p>';
            return;
        }

        const rows = data.map(item => `
            <tr>
                <td style="font-family:monospace;font-size:0.85rem;">${window.escapeHtml(item.table_name)}</td>
                <td>${window.escapeHtml(item.server_name || item.server_id || '—')}</td>
                <td>${formatDate(item.last_exported_at)}</td>
                <td><span class="badge ${lagBadgeClass(item.age)}" style="font-size:0.75rem;">${window.escapeHtml(item.age || 'N/A')}</span></td>
                <td>${formatDate(item.watermark_updated_at)}</td>
            </tr>`).join('');

        panel.innerHTML = `
            <div style="overflow-x:auto;">
                <table style="width:100%;border-collapse:collapse;font-size:0.85rem;">
                    <thead>
                        <tr style="border-bottom:2px solid var(--border-color,#e5e7eb);">
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Table</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Server</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Last Exported</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Lag</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Updated</th>
                        </tr>
                    </thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>`;
    } catch (err) {
        panel.innerHTML = `<p style="color:var(--danger,#ef4444);padding:1rem;">
            <i class="fa-solid fa-triangle-exclamation"></i> ${window.escapeHtml(err.message)}
        </p>`;
    }
}

async function renderRunsPanel() {
    const panel = document.getElementById('cs-runs-panel');
    panel.innerHTML = '<div class="text-muted" style="padding:1rem;">Loading run history…</div>';

    try {
        const resp = await window.apiClient.authenticatedFetch('/api/cold-storage/runs');
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const data = await resp.json();

        if (!data || data.length === 0) {
            panel.innerHTML = '<p class="text-muted" style="padding:1rem;">No export runs found yet.</p>';
            return;
        }

        const rows = data.map(run => {
            const mb = run.total_bytes ? (run.total_bytes / (1024 * 1024)).toFixed(2) : '0.00';
            const statusColor = { success: '#22c55e', partial: '#f59e0b', failed: '#ef4444', running: '#3b82f6' };
            const color = statusColor[run.status] || '#6b7280';
            return `
            <tr style="border-bottom:1px solid var(--border-color,#e5e7eb);">
                <td style="padding:0.5rem 0.75rem;font-family:monospace;">#${run.run_id}</td>
                <td style="padding:0.5rem 0.75rem;">${formatDate(run.run_started)}</td>
                <td style="padding:0.5rem 0.75rem;">${formatDate(run.run_finished)}</td>
                <td style="padding:0.5rem 0.75rem;">
                    <span class="badge" style="background:${color}20;color:${color};font-size:0.75rem;">
                        ${window.escapeHtml(run.status ? run.status.toUpperCase() : '—')}
                    </span>
                </td>
                <td style="padding:0.5rem 0.75rem;">${run.tables_ok} / ${run.tables_failed}</td>
                <td style="padding:0.5rem 0.75rem;font-family:monospace;">${(run.total_rows || 0).toLocaleString()}</td>
                <td style="padding:0.5rem 0.75rem;font-family:monospace;">${mb} MB</td>
            </tr>`;
        }).join('');

        panel.innerHTML = `
            <div style="overflow-x:auto;">
                <table style="width:100%;border-collapse:collapse;font-size:0.85rem;">
                    <thead>
                        <tr style="border-bottom:2px solid var(--border-color,#e5e7eb);">
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Run ID</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Started</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Finished</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Status</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Tables OK/Fail</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Rows</th>
                            <th style="padding:0.6rem 0.75rem;text-align:left;">Volume</th>
                        </tr>
                    </thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>`;
    } catch (err) {
        panel.innerHTML = `<p style="color:var(--danger,#ef4444);padding:1rem;">
            <i class="fa-solid fa-triangle-exclamation"></i> ${window.escapeHtml(err.message)}
        </p>`;
    }
}

function formatDate(iso) {
    if (!iso || iso.startsWith('0001-')) return '—';
    return new Date(iso).toLocaleString();
}

function lagBadgeClass(ageStr) {
    if (!ageStr) return '';
    const days = parseInt(ageStr);
    if (!isNaN(days) && ageStr.includes('day') && days > 3) return 'badge-danger';
    if (!isNaN(days) && ageStr.includes('day')) return 'badge-warning';
    return 'badge-success';
}

// ── Standalone page support ───────────────────────────────────────────────────
// When this file is loaded directly (admin_cold_storage.html), DOMContentLoaded
// bootstraps without the admin.js tab system. In that case window.apiClient may
// not be set; show an error rather than crashing.
if (typeof document !== 'undefined' && !document.getElementById('admin-content')) {
    document.addEventListener('DOMContentLoaded', () => {
        const root = document.getElementById('cold-storage-root');
        if (!root) return;
        if (!window.apiClient) {
            root.innerHTML = '<p style="color:red;padding:1rem;">Error: apiClient not available. Access this page through the main admin panel.</p>';
            return;
        }
        loadColdStorage(root);
    });
}
