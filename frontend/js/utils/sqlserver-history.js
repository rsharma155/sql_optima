/*
 * SQL Optima — shared fetch helpers for federated SQL Server history series
 * (hot Timescale + optional cold Trino merge).
 */

/**
 * @typedef {'cpu'|'memory'|'wait'|'connection'} SqlServerHistoryKind
 */

const HISTORY_PATHS = {
    cpu: '/api/timescale/sqlserver/cpu-history',
    memory: '/api/timescale/sqlserver/memory-history',
    wait: '/api/timescale/sqlserver/wait-history',
    connection: '/api/timescale/sqlserver/connection-history',
};

/**
 * Normalize API payload to { points, source }.
 * Accepts legacy bare arrays for backward compatibility.
 */
export function normalizeHistoryPayload(data, sourceHeader) {
    let points = [];
    let source = (sourceHeader || '').toString().trim() || 'hot';
    if (Array.isArray(data)) {
        points = data;
    } else if (data && typeof data === 'object') {
        if (Array.isArray(data.points)) points = data.points;
        else if (Array.isArray(data.series)) points = data.series;
        if (data.source) source = String(data.source);
    }
    return { points, source };
}

/**
 * Fetch a federated history series for the selected instance and time range.
 * @param {SqlServerHistoryKind} kind
 * @param {string} instanceName
 * @param {string} fromISO
 * @param {string} toISO
 * @returns {Promise<{ points: object[], source: string, ok: boolean, status: number }>}
 */
export async function fetchSqlServerHistory(kind, instanceName, fromISO, toISO) {
    const path = HISTORY_PATHS[kind];
    if (!path) {
        return { points: [], source: '', ok: false, status: 0 };
    }
    if (!instanceName || !fromISO || !toISO) {
        return { points: [], source: '', ok: false, status: 0 };
    }
    const url =
        `${path}?instance=${encodeURIComponent(instanceName)}` +
        `&from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}`;
    try {
        const res = await window.apiClient.authenticatedFetch(url);
        if (!res.ok) {
            return { points: [], source: '', ok: false, status: res.status };
        }
        const data = await res.json();
        const headerSrc = res.headers.get('X-Data-Source');
        const normalized = normalizeHistoryPayload(data, headerSrc);
        return { ...normalized, ok: true, status: res.status };
    } catch (e) {
        console.error(`[history] ${kind} fetch failed`, e);
        return { points: [], source: '', ok: false, status: 0 };
    }
}

/** Apply source badge for hot / hot+cold / timescale labels. */
export function applyHistorySourceBadge(badgeId, source) {
    if (typeof window.updateSourceBadge === 'function') {
        window.updateSourceBadge(badgeId, source);
        return;
    }
    const el = document.getElementById(badgeId);
    if (!el || !source) return;
    const raw = String(source).toLowerCase();
    let label = source;
    if (raw === 'hot') label = 'Timescale (hot)';
    if (raw === 'hot+cold' || raw === 'hot + cold') label = 'Hot + cold storage';
    if (raw.includes('cold')) label = 'Hot + cold storage';
    el.textContent = `Source: ${label}`;
    el.style.display = 'inline-block';
    el.className = raw.includes('cold') ? 'badge badge-success' : 'badge badge-info';
}

// Legacy window bridge for non-module page scripts
window.fetchSqlServerHistory = fetchSqlServerHistory;
window.normalizeHistoryPayload = normalizeHistoryPayload;
window.applyHistorySourceBadge = applyHistorySourceBadge;
window.SQLSERVER_HISTORY_PATHS = HISTORY_PATHS;
