/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Shared validation logic for frontend forms.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

/**
 * validateMonitoringServerPayload - Shared validation for monitoring target registration.
 * @param {Object} p - The server payload object.
 * @returns {string|null} - Error message if invalid, otherwise null.
 */
window.validateMonitoringServerPayload = function(p) {
    if (!p.name || p.name.trim().length === 0) return 'Display name is required.';
    if (p.name.length > 100) return 'Name too long (max 100 characters).';
    if (!p.host || p.host.trim().length === 0) return 'Hostname or IP address is required.';
    if (!p.username || p.username.trim().length === 0) return 'Database username is required.';
    if (isNaN(p.port) || p.port < 1 || p.port > 65535) return 'Invalid port number (1-65535).';
    
    // Engine specific basic checks
    if (p.db_type === 'postgres' && !p.ssl_mode) return 'SSL mode is required for PostgreSQL.';
    
    return null;
};
