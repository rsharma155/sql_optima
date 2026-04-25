/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Shared logic for server management actions (delete, test, patch).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

export const AdminManager = {
    async deleteServer(id) {
        if (!confirm('Delete this server?')) return;
        
        // Find which view we are in to show message and refresh correctly
        const onbMsg = document.getElementById('onb-msg');
        const adminMsg = document.getElementById('admin-server-msg');
        const targetMsg = onbMsg || adminMsg;
        
        try {
            const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}`, { method: 'DELETE' });
            if (!response.ok) {
                const j = await response.json().catch(() => ({}));
                throw new Error(j.error || `HTTP ${response.status}`);
            }
            
            if (targetMsg) targetMsg.innerHTML = '<div class="alert alert-success">Server deleted.</div>';
            
            // Refresh based on active view
            if (typeof window.onbLoadServers === 'function' && document.getElementById('onb-list')) {
                await window.onbLoadServers();
            } else if (typeof window.loadAdminServers === 'function' && document.getElementById('admin-server-list')) {
                await window.loadAdminServers();
            }
            
            // Sync config after deletion
            if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
                await window.apiClient.fetchConfig();
                if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
                    window.router.populateInstanceDropdown();
                }
            }
        } catch (e) {
            if (targetMsg) targetMsg.innerHTML = `<div class="alert alert-danger">Delete failed: ${window.escapeHtml(e.message)}</div>`;
            else alert('Delete failed: ' + e.message);
        }
    },

    async patchServerActive(id, active) {
        const onbMsg = document.getElementById('onb-msg');
        const adminMsg = document.getElementById('admin-server-msg');
        const targetMsg = onbMsg || adminMsg;

        try {
            const response = await window.apiClient.authenticatedFetch(`/api/admin/servers/${encodeURIComponent(id)}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ is_active: active })
            });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || `HTTP ${response.status}`);
            }
            
            // Refresh based on active view
            if (typeof window.onbLoadServers === 'function' && document.getElementById('onb-list')) {
                await window.onbLoadServers();
            } else if (typeof window.loadAdminServers === 'function' && document.getElementById('admin-server-list')) {
                await window.loadAdminServers();
            }

            // Sync config after update
            if (window.apiClient && typeof window.apiClient.fetchConfig === 'function') {
                await window.apiClient.fetchConfig();
                if (window.router && typeof window.router.populateInstanceDropdown === 'function') {
                    window.router.populateInstanceDropdown();
                }
            }
        } catch (e) {
            if (targetMsg) targetMsg.innerHTML = `<div class="alert alert-danger">Update failed: ${window.escapeHtml(e.message)}</div>`;
        }
    }
};
