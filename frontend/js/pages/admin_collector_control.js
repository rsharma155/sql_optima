// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Logic for Collector Control Center (Admin) with Modern UI and Validation.
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
        <div class="glass-panel" style="padding:1.5rem; border-radius:12px; box-shadow: var(--shadow-sm);">
            <div class="flex-between" style="margin-bottom:1.5rem; border-bottom:1px solid var(--border-color); padding-bottom:1rem;">
                <div>
                    <h2 style="font-size:1.1rem; margin:0; font-weight:600;">
                        <i class="fa-solid fa-clock-rotate-left text-accent"></i> Collector Frequencies
                    </h2>
                    <p class="text-muted" style="margin:0.25rem 0 0; font-size:0.85rem;">
                        Tune telemetry collection intervals. Changes take effect on the next collection cycle.
                    </p>
                </div>
                <div class="badge badge-outline" style="font-size:0.75rem;">Global Configuration</div>
            </div>
            
            <div id="collector-loading" class="text-center" style="padding:4rem;">
                <div class="spinner" style="margin:0 auto 1.5rem; width:2.5rem; height:2.5rem; border-width:3px;"></div>
                <div class="text-muted" style="font-weight:500;">Retrieving collector definitions…</div>
            </div>

            <div id="collector-table-container" style="display:none;">
                <div class="table-responsive">
                    <table class="data-table" style="width:100%; border-collapse: separate; border-spacing: 0 0.5rem;">
                        <thead>
                            <tr style="text-align:left; font-size:0.75rem; text-transform:uppercase; letter-spacing:0.05em; color:var(--text-muted);">
                                <th style="padding:0.75rem 1rem;">Collector Name</th>
                                <th style="padding:0.75rem 1rem;">Module</th>
                                <th style="padding:0.75rem 1rem;">Interval</th>
                                <th style="padding:0.75rem 1rem;">Metadata</th>
                                <th style="padding:0.75rem 1rem; text-align:right;">Actions</th>
                            </tr>
                        </thead>
                        <tbody id="collector-config-body"></tbody>
                    </table>
                </div>
            </div>
        </div>
    `;

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/collector-configs');
        if (!response.ok) throw new Error('Failed to fetch configs');
        const configs = await response.json();
        
        const body = document.getElementById('collector-config-body');
        body.innerHTML = '';
        
        if (!configs || configs.length === 0) {
            body.innerHTML = '<tr><td colspan="5" class="text-center text-muted" style="padding:2rem;">No collectors configured.</td></tr>';
        } else {
            configs.forEach(c => {
                const tr = document.createElement('tr');
                tr.id = `row-${c.id}`;
                tr.className = 'hover-row';
                tr.style.background = 'rgba(255,255,255,0.02)';
                
                const moduleClass = c.module.toLowerCase() === 'postgres' ? 'badge-info' : 'badge-warning';
                
                tr.innerHTML = `
                    <td style="padding:1rem; border-top-left-radius:8px; border-bottom-left-radius:8px;">
                        <div style="font-weight:600; font-size:0.9rem;">${window.escapeHtml(c.collector_name)}</div>
                        <div style="font-size:0.7rem; color:var(--text-muted); margin-top:0.2rem;">ID: ${c.id}</div>
                    </td>
                    <td style="padding:1rem;">
                        <span class="badge ${moduleClass}" style="text-transform:capitalize; padding:0.25rem 0.6rem;">${window.escapeHtml(c.module)}</span>
                    </td>
                    <td style="padding:1rem;">
                        <div class="view-mode">
                            <span style="font-family:monospace; font-weight:600; font-size:1rem;">${c.frequency_seconds}</span>
                            <span class="text-muted" style="font-size:0.75rem; margin-left:0.25rem;">sec</span>
                        </div>
                        <div class="edit-mode" style="display:none; align-items:center; gap:0.5rem;">
                            <input type="number" 
                                   class="custom-input" 
                                   style="width:100px; padding:0.35rem; font-size:0.9rem;" 
                                   value="${c.frequency_seconds}" 
                                   id="freq-${c.id}" 
                                   min="5" max="604800">
                            <span class="text-muted" style="font-size:0.75rem;">sec</span>
                        </div>
                    </td>
                    <td style="padding:1rem; font-size:0.8rem;">
                        <div class="text-muted"><i class="fa-solid fa-user" style="font-size:0.7rem; width:1rem;"></i> ${window.escapeHtml(c.updated_by || 'system')}</div>
                        <div class="text-muted" style="margin-top:0.25rem;"><i class="fa-solid fa-clock" style="font-size:0.7rem; width:1rem;"></i> ${new Date(c.updated_at).toLocaleDateString()} ${new Date(c.updated_at).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'})}</div>
                    </td>
                    <td style="padding:1rem; text-align:right; border-top-right-radius:8px; border-bottom-right-radius:8px;">
                        <div class="view-mode">
                            <button class="btn btn-xs btn-outline btn-edit" data-id="${c.id}" style="padding:0.3rem 0.75rem;">
                                <i class="fa-solid fa-pen-to-square"></i> Edit
                            </button>
                        </div>
                        <div class="edit-mode" style="display:none; gap:0.4rem; justify-content:flex-end;">
                            <button class="btn btn-xs btn-accent btn-save" data-id="${c.id}" style="padding:0.3rem 0.75rem;">
                                <i class="fa-solid fa-check"></i> Save
                            </button>
                            <button class="btn btn-xs btn-outline btn-cancel" data-id="${c.id}" style="padding:0.3rem 0.75rem;">
                                <i class="fa-solid fa-xmark"></i>
                            </button>
                        </div>
                    </td>
                `;
                body.appendChild(tr);
            });
        }
        
        // Event Delegation
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
                const viewVal = row.querySelector('.view-mode span').innerText;
                row.querySelector('input').value = viewVal;
            } else if (btn.classList.contains('btn-save')) {
                const input = row.querySelector('input');
                const freq = parseInt(input.value);
                
                // Advanced Validation
                if (isNaN(freq)) {
                    notify('Frequency must be a valid number', 'error');
                    return;
                }
                if (freq < 5) {
                    notify('Minimum allowed frequency is 5 seconds for critical metrics', 'error');
                    return;
                }
                if (freq > 604800) {
                    notify('Maximum allowed frequency is 604800 seconds (1 week)', 'error');
                    return;
                }
                
                btn.disabled = true;
                btn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i>';
                
                await updateCollectorConfig(id, freq);
            }
        });
        
        document.getElementById('collector-loading').style.display = 'none';
        document.getElementById('collector-table-container').style.display = 'block';
    } catch (err) {
        container.innerHTML = `<div class="alert alert-danger" style="margin:2rem;">
            <i class="fa-solid fa-triangle-exclamation"></i> <strong>Initialization Error:</strong> ${window.escapeHtml(err.message)}
        </div>`;
    }
}

function toggleRowMode(row, isEdit) {
    row.querySelectorAll('.view-mode').forEach(el => el.style.display = isEdit ? 'none' : 'block');
    row.querySelectorAll('.edit-mode').forEach(el => el.style.display = isEdit ? 'flex' : 'none');
    if (isEdit) {
        row.style.background = 'rgba(var(--accent-rgb), 0.05)';
        row.querySelector('input').focus();
    } else {
        row.style.background = 'rgba(255,255,255,0.02)';
    }
}

async function updateCollectorConfig(id, frequency) {
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/collector-configs/${id}`, {
            method: 'PUT',
            body: JSON.stringify({ frequency_seconds: frequency })
        });
        
        if (response.ok) {
            notify('Frequency updated successfully. Changes will propagate in the next cycle.');
            loadCollectorConfigs();
        } else {
            const err = await response.json();
            notify('Update failed: ' + (err.error || 'Server rejected the change'), 'error');
            loadCollectorConfigs(); // reset state
        }
    } catch (err) {
        notify('Network error: ' + err.message, 'error');
        loadCollectorConfigs(); // reset state
    }
}
