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
import { loadTemplate, escapeHtml, truncate } from './modules/ui-manager.js';
import { AuthManager } from './modules/auth-manager.js';
import { ViewLoader } from './modules/view-loader.js';
import { GlobalEstate } from './modules/global-estate.js';
import { PgOverview } from './modules/pg-overview.js';
import { AdminManager } from './modules/admin-manager.js';
import {
    tsMs,
    parseChartTimestamp,
    parseChartTimestampOrNow,
    fmtChartTick,
    sortByChartTime,
    alignSeriesByTime
} from './utils/time-axis.js';
import {
    applyChartJsDefaults,
    chartInteractionOptions,
    chartTooltipPlugin,
    chartTimeScaleX,
    chartTrendOptions,
    mergeChartOptions,
    formatChartTooltipValue,
    getInstanceMonitoringUser,
    filterOutMonitoringPgQueries
} from './utils/chart-options.js';
import {
    formatDateTimeLocalInput,
    syncTimeRangeFromPicker,
    localDateTimeToISO,
    getAppTimeRangeISO,
    applyGlobalTimeRangeRefresh
} from './utils/time-range.js';
import { decodeQueryText } from './utils/query-text.js';

window.appState = appState;
window.tsMs = tsMs;
window.parseChartTimestamp = parseChartTimestamp;
window.parseChartTimestampOrNow = parseChartTimestampOrNow;
window.fmtChartTick = fmtChartTick;
window.sortByChartTime = sortByChartTime;
window.alignSeriesByTime = alignSeriesByTime;
applyChartJsDefaults();
window.chartInteractionOptions = chartInteractionOptions;
window.chartTooltipPlugin = chartTooltipPlugin;
window.chartTimeScaleX = chartTimeScaleX;
window.chartTrendOptions = chartTrendOptions;
window.mergeChartOptions = mergeChartOptions;
window.formatChartTooltipValue = formatChartTooltipValue;
window.getInstanceMonitoringUser = getInstanceMonitoringUser;
window.filterOutMonitoringPgQueries = filterOutMonitoringPgQueries;
window.apiClient = apiClient;
window.downloadAuthenticatedCSV = (url, fallbackFilename) =>
    apiClient.downloadAuthenticatedCSV(url, fallbackFilename);
window.downloadAuthenticatedBlob = (url, fallbackFilename) =>
    apiClient.downloadAuthenticatedBlob(url, fallbackFilename);
window.AuthManager = AuthManager;
window._auth = AuthManager;
window.loadTemplate = loadTemplate;
window.escapeHtml = escapeHtml;
window.truncate = truncate;
window.ViewLoader = ViewLoader;
window.GlobalEstate = GlobalEstate;
window.PgOverview = PgOverview;
window.setDashboardRefresh = setDashboardRefresh;
window.setJobsRefresh = setJobsRefresh;
window.showQueryModal = showQueryModal;
window.decodeQueryText = decodeQueryText;
window.boot = boot;
window.deleteAdminServer = AdminManager.deleteServer;
window.patchServerActive = AdminManager.patchServerActive;

window.formatDateTimeLocalInput = formatDateTimeLocalInput;
window.syncTimeRangeFromPicker = syncTimeRangeFromPicker;
window.localDateTimeToISO = localDateTimeToISO;
window.getAppTimeRangeISO = getAppTimeRangeISO;
window.applyGlobalTimeRangeRefresh = applyGlobalTimeRangeRefresh;

window.initPageTimePicker = function() {
    const template = document.getElementById('global-time-picker-template');
    const target = document.getElementById('time-picker-insertion-point');
    if (!template || !target) return;

    target.innerHTML = '';
    const clone = template.content.cloneNode(true);
    target.appendChild(clone);

    const fromInput = document.getElementById('from-ts');
    const toInput = document.getElementById('to-ts');
    const refreshBtn = document.getElementById('global-refresh-btn');

    if (!fromInput || !toInput) return;

    // Use current state if available, else default
    if (window.appState.fromTs) fromInput.value = window.appState.fromTs;
    if (window.appState.toTs) toInput.value = window.appState.toTs;

    if (!fromInput.value || !toInput.value) {
        const now = new Date();
        const oneHourAgo = new Date(now.getTime() - (60 * 60 * 1000));
        if (!fromInput.value) fromInput.value = formatDateTimeLocalInput(oneHourAgo);
        if (!toInput.value) toInput.value = formatDateTimeLocalInput(now);
        window.appState.fromTs = fromInput.value;
        window.appState.toTs = toInput.value;
    }

    const onInputChange = () => syncTimeRangeFromPicker();
    fromInput.addEventListener('change', onInputChange);
    toInput.addEventListener('change', onInputChange);
    fromInput.addEventListener('input', onInputChange);
    toInput.addEventListener('input', onInputChange);

    if (refreshBtn) {
        refreshBtn.addEventListener('click', () => applyGlobalTimeRangeRefresh());
    }

    const reloadBtn = document.getElementById('global-reload-btn');
    if (reloadBtn) {
        reloadBtn.addEventListener('click', () => applyGlobalTimeRangeRefresh());
    }
};

// Wait for DOM and classic scripts (router.js, etc.) to be ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        setTimeout(() => boot(), 200);
    });
} else {
    setTimeout(() => boot(), 200);
}
