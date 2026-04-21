/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Global Estate view module for multi-instance management.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

import { appState } from './app-state.js';
import { escapeHtml } from './ui-manager.js';

export const GlobalEstate = {
    async init() {
        this.render();
        this.bindEvents();
        return this;
    },

    render() {
        const container = document.getElementById('estateBody');
        if (!container) return;

        const instances = appState.config?.instances || [];
        if (instances.length === 0) {
            container.innerHTML = `
                <div class="glass-panel p-5 text-center">
                    <p class="text-muted">No monitoring instances are configured yet.</p>
                    <button class="btn btn-primary mt-3" data-action="navigate" data-route="admin-servers">Add Your First Server</button>
                </div>`;
            return;
        }

        const pgInstances = instances.filter(i => i.type === 'postgres');
        const sqlInstances = instances.filter(i => i.type === 'sqlserver');

        let html = '';
        if (sqlInstances.length > 0) {
            html += `<div class="estate-section-title"><i class="fa-brands fa-microsoft"></i> SQL Server</div>`;
            html += `<div class="estate-grid">${sqlInstances.map(i => this.renderCard(i)).join('')}</div>`;
        }
        if (pgInstances.length > 0) {
            html += `<div class="estate-section-title"><i class="fa-solid fa-server"></i> PostgreSQL</div>`;
            html += `<div class="estate-grid">${pgInstances.map(i => this.renderCard(i)).join('')}</div>`;
        }

        container.innerHTML = html;
        this.attachCardClicks();
    },

    renderCard(inst) {
        const isUp = inst.available !== false;
        const typeIcon = inst.type === 'postgres' ? 'fa-database' : 'fa-microsoft';
        
        return `
            <div class="estate-card glass-panel ${isUp ? 'up' : 'down'}" data-instance-name="${escapeHtml(inst.name)}">
                <div class="estate-card-header">
                    <div style="flex:1; min-width:0;">
                        <h4 class="estate-card-title">${escapeHtml(inst.name)}</h4>
                        <div class="estate-card-type"><i class="fa-solid ${typeIcon}"></i> ${inst.type}</div>
                    </div>
                    <div class="status-badge" style="font-size:0.6rem; font-weight:800; text-transform:uppercase; color:${isUp ? 'var(--success)' : 'var(--danger)'}">
                        ${isUp ? 'Online' : 'Offline'}
                    </div>
                </div>
                <div class="estate-card-body">
                    <div class="estate-card-stat">
                        <span class="estate-card-stat-label">Endpoint</span>
                        <span class="estate-card-stat-value text-mono" style="font-size:0.65rem;">${escapeHtml(inst.host)}:${inst.port}</span>
                    </div>
                    <div class="estate-card-stat">
                        <span class="estate-card-stat-label">Default DB</span>
                        <span class="estate-card-stat-value">${escapeHtml(inst.database || 'postgres')}</span>
                    </div>
                </div>
            </div>`;
    },

    bindEvents() {
        const searchInput = document.getElementById('estateSearch');
        if (searchInput) {
            searchInput.addEventListener('input', (e) => this.handleSearch(e.target.value));
        }
        document.getElementById('estateRefresh')?.addEventListener('click', () => this.render());
    },

    handleSearch(term) {
        const q = term.toLowerCase();
        document.querySelectorAll('.estate-card').forEach(card => {
            const name = card.dataset.instanceName.toLowerCase();
            card.style.display = name.includes(q) ? '' : 'none';
        });
    },

    attachCardClicks() {
        document.querySelectorAll('.estate-card').forEach(card => {
            card.onclick = () => {
                const name = card.dataset.instanceName;
                const idx = appState.config.instances.findIndex(i => i.name === name);
                if (idx !== -1) {
                    window.selectInstanceFromEstate(idx);
                }
            };
        });
    }
};
