/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: UI Manager for handling template loading, navigation events, and common UI interactions.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

/**
 * Loads an HTML template from the specified path and returns its content as a string.
 * @param {string} path - The path to the HTML template file.
 * @returns {Promise<string>} - The template HTML content.
 */
export async function loadTemplate(path) {
    try {
        const response = await fetch(path);
        if (!response.ok) {
            throw new Error(`Failed to load template: ${path} (HTTP ${response.status})`);
        }
        return await response.text();
    } catch (error) {
        console.error('[UIManager] Template load error:', error);
        return `<div class="alert alert-danger">Error loading view: ${error.message}</div>`;
    }
}

/**
 * Utility to escape HTML entities in strings.
 */
export function escapeHtml(unsafe) {
    if (unsafe === null || unsafe === undefined) return '';
    return String(unsafe)
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#039;");
}

/**
 * Utility to truncate long strings.
 */
export function truncate(str, n = 100) {
    if (!str) return '';
    return (str.length > n) ? str.substr(0, n - 1) + '...' : str;
}
