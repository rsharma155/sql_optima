// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Standalone logic for Admin Collector Control (for direct page access).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

import { checkAuth, logout } from '/js/api/auth.js';

async function loadConfigs() {
    try {
        const response = await fetch('/api/admin/collector-configs');
        if (!response.ok) throw new Error('Failed to fetch configs');
        const configs = await response.json();
        
        const body = document.getElementById('config-body');
        if (!body) return;
        body.innerHTML = '';
        
        configs.forEach(c => {
            const tr = document.createElement('tr');
            const moduleClass = c.module.toLowerCase() === 'postgres' ? 'module-postgres' : 'module-mssql';
            
            tr.innerHTML = `
                <td><strong>${window.escapeHtml(c.collector_name)}</strong></td>
                <td><span class="module-badge ${moduleClass}">${window.escapeHtml(c.module)}</span></td>
                <td>
                    <input type="number" class="frequency-input" value="${c.frequency_seconds}" id="freq-${c.id}">
                </td>
                <td>${new Date(c.updated_at).toLocaleString()}</td>
                <td>${window.escapeHtml(c.updated_by || 'system')}</td>
                <td>
                    <button class="btn-save" data-id="${c.id}">Update</button>
                </td>
            `;
            body.appendChild(tr);
        });
        
        document.querySelectorAll('.btn-save').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                const id = e.target.dataset.id;
                const freq = document.getElementById(`freq-${id}`).value;
                await updateConfig(id, freq);
            });
        });
        
        document.getElementById('loading').style.display = 'none';
        document.getElementById('config-container').style.display = 'block';
    } catch (err) {
        console.error(err);
        const loading = document.getElementById('loading');
        if (loading) loading.innerText = 'Error loading configurations. Ensure you have admin privileges.';
    }
}

async function updateConfig(id, frequency) {
    try {
        const response = await fetch(`/api/admin/collector-configs/${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ frequency_seconds: parseInt(frequency) })
        });
        
        if (response.ok) {
            alert('Configuration updated successfully.');
            loadConfigs();
        } else {
            const err = await response.json();
            alert('Update failed: ' + (err.error || 'Unknown error'));
        }
    } catch (err) {
        alert('Update failed: ' + err.message);
    }
}

window.addEventListener('DOMContentLoaded', async () => {
    if (typeof window.escapeHtml !== 'function') {
        window.escapeHtml = (str) => {
            const div = document.createElement('div');
            div.textContent = str;
            return div.innerHTML;
        };
    }
    const user = await checkAuth();
    if (!user || user.role !== 'admin') {
        window.location.href = '/index.html';
        return;
    }
    const display = document.getElementById('username-display');
    if (display) display.innerText = user.username;
    loadConfigs();
});

const logoutBtn = document.getElementById('logout-btn');
if (logoutBtn) logoutBtn.addEventListener('click', logout);
