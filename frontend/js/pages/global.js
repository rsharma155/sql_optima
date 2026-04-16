/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Global multi-instance overview dashboard.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.GlobalEstateView = async function() {
    let sqlHtml = '';
    let pgHtml = '';

    // Build cards from config instances directly (no API call needed)
    const instances = window.appState.config.instances || [];
    
    instances.forEach((m, i) => {
        const isUp = m.available !== false;
        const stateClass = isUp ? 'status-healthy' : 'status-danger';
        const stateTitle = isUp ? 'Connected' : 'Unreachable';
        const stateHtml = '<span class="status-indicator ' + stateClass + '" title="' + stateTitle + '"></span>';
        const card = `
            <div class="estate-card glass-panel clickable-chart" data-instance-idx="${i}" style="padding: 10px; border-radius: 6px; cursor: pointer; border: 1px solid rgba(255,255,255,0.05); transition: 0.2s;">
                <div class="estate-header" style="display:flex; justify-content:space-between; align-items:center; margin-bottom: 6px;">
                    <h4 style="margin:0; font-size: 0.85rem; font-weight:600; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">
                        <i class="fa-solid ${m.type==='postgres' ? 'fa-database' : 'fa-microsoft'} text-accent" style="margin-right:4px;"></i> ${m.name}
                    </h4>
                    ${stateHtml}
                </div>
                <div class="estate-body" style="font-size: 0.75rem;">
                    <div style="display:flex; justify-content:space-between; margin-bottom: 2px;">
                        <span class="text-muted">Host</span><span>${m.host}:${m.port}</span>
                    </div>
                    <div style="display:flex; justify-content:space-between; margin-bottom: 2px;">
                        <span class="text-muted">Type</span><span>${m.type}</span>
                    </div>
                </div>
            </div>
        `;

        if (m.type === 'postgres') {
            pgHtml += card;
        } else {
            sqlHtml += card;
        }
    });

    const hasSql = sqlHtml.length > 0;
    const hasPg = pgHtml.length > 0;
    const hasAny = hasSql || hasPg;

    let bodyBlocks = '';
    if (!hasAny) {
        bodyBlocks = `
            <div class="glass-panel" style="padding:1.5rem;margin-top:1rem;">
                <p class="text-muted" style="margin:0;">No monitoring instances are configured yet. Administrators can add SQL Server or PostgreSQL targets under <strong>Admin</strong> (or the onboarding flow).</p>
            </div>`;
    } else {
        if (hasSql) {
            bodyBlocks += `
            <h3 class="mt-4 mb-3 pb-2" style="border-bottom: 1px solid rgba(255,255,255,0.1);"><i class="fa-brands fa-microsoft text-accent"></i> SQL Server instances</h3>
            <div class="estate-grid" style="display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 15px;">
                ${sqlHtml}
            </div>`;
        }
        if (hasPg) {
            bodyBlocks += `
            <h3 class="mt-5 mb-3 pb-2" style="border-bottom: 1px solid rgba(255,255,255,0.1);"><i class="fa-solid fa-server" style="color:var(--success)"></i> PostgreSQL clusters</h3>
            <div class="estate-grid pb-5" style="display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 15px;">
                ${pgHtml}
            </div>`;
        }
        if (hasSql && !hasPg) {
            bodyBlocks += `<p class="text-muted mt-3" style="font-size:0.8rem;">No PostgreSQL instances are registered.</p>`;
        } else if (!hasSql && hasPg) {
            bodyBlocks += `<p class="text-muted mt-3" style="font-size:0.8rem;">No SQL Server instances are registered.</p>`;
        }
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active">
            <div class="page-title mb-4">
                <h1><i class="fa-solid fa-earth-americas text-accent"></i> Global Estate Overview</h1>
                <p class="subtitle mt-1">Select an instance to populate the diagnostic tools menu.</p>
            </div>
            ${bodyBlocks}
        </div>
    `;

    if (!document.getElementById('estate-card-styles')) {
        const style = document.createElement('style');
        style.id = 'estate-card-styles';
        style.innerHTML = `
            .estate-card:hover { transform: translateY(-3px); box-shadow: 0 4px 15px rgba(0,0,0,0.5); border-color: var(--accent-blue) !important; }
        `;
        document.head.appendChild(style);
    }

    // Bind click via event delegation (CSP-safe, no inline handlers).
    document.querySelectorAll('.estate-card[data-instance-idx]').forEach(function(card) {
        card.addEventListener('click', function() {
            var idx = parseInt(card.getAttribute('data-instance-idx'), 10);
            if (!isNaN(idx)) window.selectInstanceFromEstate(idx);
        });
    });
}

window.selectInstanceFromEstate = function(idx) {
    const sel = document.getElementById('instance-select');
    sel.value = idx;
    sel.dispatchEvent(new Event('change'));
}

// Global search functionality
window.initGlobalSearch = function() {
    const searchInput = document.getElementById('global-search');
    if (!searchInput) return;

    searchInput.addEventListener('input', function(e) {
        const searchTerm = e.target.value.toLowerCase().trim();
        const estateCards = document.querySelectorAll('.estate-card');

        if (!searchTerm) {
            // Show all cards
            estateCards.forEach(card => {
                card.style.display = 'block';
            });
            return;
        }

        estateCards.forEach(card => {
            const instanceName = card.querySelector('h4').textContent.toLowerCase();
            const instanceType = card.querySelector('i').classList.contains('fa-database') ? 'postgres' : 'sqlserver';

            if (instanceName.includes(searchTerm) || instanceType.includes(searchTerm)) {
                card.style.display = 'block';
            } else {
                card.style.display = 'none';
            }
        });
    });

    // Clear search on escape
    searchInput.addEventListener('keydown', function(e) {
        if (e.key === 'Escape') {
            searchInput.value = '';
            searchInput.dispatchEvent(new Event('input'));
        }
    });
}

// Initialize search when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    window.initGlobalSearch();
});
