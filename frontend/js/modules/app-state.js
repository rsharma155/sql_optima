/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Global application state manager for configuration, instance selection, authentication status, and navigation.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

/** Shared application state (ES module). Legacy views still access `window.appState` from entry.js. */
export const appState = {
    config: null,
    currentInstanceIdx: -1,
    currentDatabase: 'all',
    activeViewId: 'global',
    isAuthenticated: false,
    /** When true, API expects JWT on /api/config and dashboard routes (AUTH_REQUIRED=1). */
    authRequired: false,
    authMode: 'local',
    /** "docker" | "dedicated" — from API (SQL_OPTIMA_DEPLOYMENT). */
    deployment: 'dedicated',
    navigationHistory: [],
    fromTs: null,
    toTs: null,
    timescaleMetrics: {},
    liveMetrics: {}
};
