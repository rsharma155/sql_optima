/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL entry for Storage & Index Health (Timescale SIH). Delegates to the shared dashboard renderer.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgStorageIndexHealthView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    const dashTitle = 'Index & Table Health';
    if (!inst || String(inst.type || '').toLowerCase() !== 'postgres') {
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="alert alert-warning">
                    <h3>${dashTitle}</h3>
                    <p class="text-muted">Select a PostgreSQL instance to view index and table efficiency metrics.</p>
                    <button class="btn btn-primary mt-2" data-action="navigate" data-route="global"><i class="fa-solid fa-home"></i> Global Estate</button>
                </div>
            </div>`;
        return;
    }
    if (typeof window.runStorageIndexHealthDashboard !== 'function') {
        window.routerOutlet.innerHTML = `<div class="page-view active"><div class="alert alert-warning">Loading ${dashTitle} scripts…</div></div>`;
        setTimeout(() => window.appNavigate('storage-index-health'), 200);
        return;
    }
    return window.runStorageIndexHealthDashboard();
};
