/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server entry for Storage & Index Health (Timescale SIH). Delegates to the shared dashboard renderer.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.MssqlStorageIndexHealthView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (inst && String(inst.type || '').toLowerCase() === 'postgres') {
        window.appNavigate('pg-dashboard');
        return;
    }
    if (typeof window.runStorageIndexHealthDashboard !== 'function') {
        window.routerOutlet.innerHTML = '<div class="page-view active"><div class="alert alert-warning">Loading Storage & Index Health scripts…</div></div>';
        setTimeout(() => window.appNavigate('storage-index-health'), 200);
        return;
    }
    return window.runStorageIndexHealthDashboard();
};
