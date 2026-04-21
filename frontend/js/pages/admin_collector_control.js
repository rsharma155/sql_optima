// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Logic for Collector Control Center (Admin) with Edit/Update flow.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

/**
 * Helper to show notifications or fallback to alert
 */
function notify(message, type = 'success') {
    if (typeof window.showNotification === 'function') {
        window.showNotification(message, type);
    } else {
        alert(message);
    }
}

export async function loadCollectorConfigs() {
    const container = document.getElementById('admin-content');
    if (!container) return;

    container.innerHTML = `
        <div class="card">
            <div class="card-header" style="display:flex; justify-content:space-between; align-items:center;">
                <h3 style="margin:0;"><i class="fa-solid fa-clock-rotate-left"></i> Collector Frequencies</h3>
                <span class="text-muted" style="font-size:0.8rem;">Changes take effect on next cycle</span>
            </div>
            <div id="collector-loading" class="text-center" style="padding:3rem;">
                <div class="spinner" style="margin:0 auto 1rem;"></div>
                <div class="text-muted">Fetching collector configurations…</div>
            </div>
            <div id="collector-table-container" style="display:none;">
                <table class="table" style="width:100%;">
                    <thead>
                        <tr>
                            <th>Collector Name</th>
                            <th>Module</th>
                            <th>Frequency (sec)</th>
                            <th>Last Updated</th>
                            <th>By</th>
                            <th style="text-align:right;">Action</th>
                        </tr>
                    </thead>
                    <tbody id="collector-config-body"></tbody>
                </table>
            </div>
        </div>
    `;

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/collector-configs');
        if (!response.ok) throw new Error('Failed to fetch configs');
        const configs = await response.json();
        
        const body = document.getElementById('collector-config-body');
        body.innerHTML = '';
        
        configs.forEach(c => {
            const tr = document.createElement('tr');
            tr.id = `row-${c.id}`;
            const moduleClass = c.module.toLowerCase() === 'postgres' ? 'badge-info' : 'badge-warning';
            
            tr.innerHTML = `
                <td style="font-weight:500;">${window.escapeHtml(c.collector_name)}</td>
                <td><span class="badge ${moduleClass}">${window.escapeHtml(c.module)}</span></td>
                <td>
                    <span class="view-mode">${c.frequency_seconds}</span>
                    <input type="number" class="input-sm edit-mode" style="width:90px; display:none;" value="${c.frequency_seconds}" id="freq-${c.id}" min="15" max="604800">
                </td>
                <td style="font-size:0.85rem;" class="text-muted">${new Date(c.updated_at).toLocaleString()}</td>
                <td style="font-size:0.85rem;" class="text-muted">${window.escapeHtml(c.updated_by || 'system')}</td>
                <td style="text-align:right;">
                    <div class="view-mode">
                        <button class="btn btn-xs btn-outline btn-edit" data-id="${c.id}">Edit</button>
                    </div>
                    <div class="edit-mode" style="display:none;">
                        <button class="btn btn-xs btn-primary btn-save" data-id="${c.id}">Save</button>
                        <button class="btn btn-xs btn-outline btn-cancel" data-id="${c.id}">Cancel</button>
                    </div>
                </td>
            `;
            body.appendChild(tr);
        });
        
        // Bind events
        body.addEventListener('click', async (e) => {
            const btn = e.target.closest('button');
            if (!btn) return;
            
            const id = btn.dataset.id;
            const row = document.getElementById(`row-${id}`);
            if (!row) return;

            if (btn.classList.contains('btn-edit')) {
                toggleRowMode(row, true);
            } else if (btn.classList.contains('btn-cancel')) {
                toggleRowMode(row, false);
                // Reset input value
                const viewVal = row.querySelector('.view-mode').innerText;
                row.querySelector('input').value = viewVal;
            } else if (btn.classList.contains('btn-save')) {
                const input = row.querySelector('input');
                const freq = parseInt(input.value);
                
                // Client-side validation
                if (isNaN(freq) || freq < 15 || freq > 604800) {
                    notify('Frequency must be between 15 and 604800 seconds (7 days)', 'error');
                    return;
                }
                
                await updateCollectorConfig(id, freq);
            }
        });
        
        document.getElementById('collector-loading').style.display = 'none';
        document.getElementById('collector-table-container').style.display = 'block';
    } catch (err) {
        container.innerHTML = `<div class="alert alert-danger">Error: ${err.message}</div>`;
    }
}

function toggleRowMode(row, isEdit) {
    row.querySelectorAll('.view-mode').forEach(el => el.style.display = isEdit ? 'none' : 'block');
    row.querySelectorAll('.edit-mode').forEach(el => el.style.display = isEdit ? 'inline-block' : 'none');
}

async function updateCollectorConfig(id, frequency) {
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/collector-configs/${id}`, {
            method: 'PUT',
            body: JSON.stringify({ frequency_seconds: frequency })
        });
        
        if (response.ok) {
            notify('Configuration updated successfully');
            loadCollectorConfigs();
        } else {
            const err = await response.json();
            notify('Update failed: ' + (err.error || 'Unknown error'), 'error');
        }
    } catch (err) {
        notify('Update failed: ' + err.message, 'error');
    }
}
