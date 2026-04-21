/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL Overview module for detailed instance analytics.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

import { appState } from './app-state.js';
import { apiClient } from './app-client.js';
import { escapeHtml } from './ui-manager.js';

export const PgOverview = {
    async init() {
        const inst = appState.config?.instances?.[appState.currentInstanceIdx];
        if (!inst || inst.type === 'sqlserver') {
            window.appNavigate('global');
            return;
        }

        // Logic from initPgDashboard in overview.js
        // For now, we delegate to the existing global function until fully migrated
        if (window.PgDashboardView) {
            await window.PgDashboardView();
        }
        return this;
    },

    destroy() {
        // Cleanup if needed
    }
};
