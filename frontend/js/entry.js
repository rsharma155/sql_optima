/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Main entry point and bootstrap for the SQL Optima frontend SPA. Wires shared state and API client onto window for legacy page scripts.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

/**
 * ES module entry: wires shared state and API client onto window for legacy page scripts.
 */
import { appState } from './modules/app-state.js';
import {
    apiClient,
    setDashboardRefresh,
    setJobsRefresh,
    showQueryModal,
    boot
} from './modules/app-client.js';

window.appState = appState;
window.apiClient = apiClient;
window.setDashboardRefresh = setDashboardRefresh;
window.setJobsRefresh = setJobsRefresh;
window.showQueryModal = showQueryModal;
window.boot = boot;

// Wait for DOM and classic scripts (router.js, etc.) to be ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => setTimeout(() => boot(), 200));
} else {
    setTimeout(() => boot(), 200);
}
