/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: View Loader for standardizing component lifecycle (init, refresh, cleanup).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

import { loadTemplate } from './ui-manager.js';

export const ViewLoader = {
    currentView: null,

    /**
     * Loads a view into the router outlet.
     * @param {string} templatePath - Path to HTML template.
     * @param {Function} initFn - Initialization function for the view.
     */
    async load(templatePath, initFn) {
        this.cleanup();
        
        const outlet = window.routerOutlet;
        if (!outlet) return;

        outlet.innerHTML = await loadTemplate(templatePath);
        
        if (typeof initFn === 'function') {
            this.currentView = await initFn();
        }
    },

    cleanup() {
        if (this.currentView && typeof this.currentView.destroy === 'function') {
            this.currentView.destroy();
        }
        this.currentView = null;
        
        // Cleanup charts
        if (window.currentCharts) {
            Object.values(window.currentCharts).forEach(chart => {
                if (chart && typeof chart.destroy === 'function') chart.destroy();
            });
            window.currentCharts = {};
        }
    }
};
