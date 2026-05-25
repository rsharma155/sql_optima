// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: frontend/js/pages/admin_cold_storage.js
// Purpose: UI logic for monitoring cold storage archival status and recent execution runs.
//
// Author: Gemini CLI (AI Agent)
// Date: May 2026
// SPDX-License-Identifier: MIT

document.addEventListener('DOMContentLoaded', () => {
    init();
});

function init() {
    const tabStatus = document.getElementById('tab-status');
    const tabRuns = document.getElementById('tab-runs');
    const statusContent = document.getElementById('status-content');
    const runsContent = document.getElementById('runs-content');

    tabStatus.addEventListener('click', () => {
        tabStatus.classList.add('active');
        tabRuns.classList.remove('active');
        statusContent.style.display = 'block';
        runsContent.style.display = 'none';
        loadStatus();
    });

    tabRuns.addEventListener('click', () => {
        tabRuns.classList.add('active');
        tabStatus.classList.remove('active');
        runsContent.style.display = 'block';
        statusContent.style.display = 'none';
        loadRuns();
    });

    loadStatus();
}

async function loadStatus() {
    const container = document.getElementById('status-container');
    const body = document.getElementById('status-body');
    const loading = document.getElementById('status-loading');

    loading.style.display = 'block';
    container.style.display = 'none';

    try {
        const response = await window.apiClient.authenticatedFetch('/api/cold-storage/status');
        if (!response.ok) throw new Error('Failed to fetch status');
        const data = await response.json();

        body.innerHTML = '';
        if (!data || data.length === 0) {
            body.innerHTML = '<tr><td colspan="5" style="text-align:center;">No archival data found.</td></tr>';
        } else {
            data.forEach(item => {
                const tr = document.createElement('tr');
                tr.innerHTML = `
                    <td><strong>${escapeHtml(item.table_name)}</strong></td>
                    <td>${escapeHtml(item.server_name || item.server_id)}</td>
                    <td>${formatDate(item.last_exported_at)}</td>
                    <td><span class="status-badge ${getLagClass(item.age)}">${formatAge(item.age)}</span></td>
                    <td>${formatDate(item.watermark_updated_at)}</td>
                `;
                body.appendChild(tr);
            });
        }

        loading.style.display = 'none';
        container.style.display = 'table';
    } catch (err) {
        body.innerHTML = `<tr><td colspan="5" style="color:red; text-align:center;">Error: ${window.escapeHtml(err.message)}</td></tr>`;
        loading.style.display = 'none';
        container.style.display = 'table';
    }
}

async function loadRuns() {
    const container = document.getElementById('runs-container');
    const body = document.getElementById('runs-body');
    const loading = document.getElementById('runs-loading');

    loading.style.display = 'block';
    container.style.display = 'none';

    try {
        const response = await window.apiClient.authenticatedFetch('/api/cold-storage/runs');
        if (!response.ok) throw new Error('Failed to fetch runs');
        const data = await response.json();

        body.innerHTML = '';
        if (!data || data.length === 0) {
            body.innerHTML = '<tr><td colspan="7" style="text-align:center;">No archival runs found.</td></tr>';
        } else {
            data.forEach(run => {
                const tr = document.createElement('tr');
                const volumeMB = (run.total_bytes / (1024 * 1024)).toFixed(2);
                tr.innerHTML = `
                    <td class="metric-value">#${run.run_id}</td>
                    <td>${formatDate(run.run_started)}</td>
                    <td>${formatDate(run.run_finished)}</td>
                    <td><span class="status-badge status-${run.status}">${run.status.toUpperCase()}</span></td>
                    <td>${run.tables_ok} / ${run.tables_failed}</td>
                    <td class="metric-value">${run.total_rows.toLocaleString()}</td>
                    <td class="metric-value">${volumeMB} MB</td>
                `;
                body.appendChild(tr);
            });
        }

        loading.style.display = 'none';
        container.style.display = 'table';
    } catch (err) {
        body.innerHTML = `<tr><td colspan="7" style="color:red; text-align:center;">Error: ${window.escapeHtml(err.message)}</td></tr>`;
        loading.style.display = 'none';
        container.style.display = 'table';
    }
}

function formatDate(isoStr) {
    if (!isoStr || isoStr === '0001-01-01T00:00:00Z') return 'Never';
    const d = new Date(isoStr);
    return d.toLocaleString();
}

function formatAge(ageStr) {
    if (!ageStr) return 'N/A';
    // PostgreSQL interval string like "2 days 04:05:06"
    return ageStr;
}

function getLagClass(ageStr) {
    if (!ageStr) return 'status-running';
    if (ageStr.includes('days') && parseInt(ageStr) > 3) return 'status-failed';
    if (ageStr.includes('days')) return 'status-partial';
    return 'status-success';
}

function escapeHtml(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}
