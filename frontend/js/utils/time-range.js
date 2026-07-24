/*
 * SQL Optima — shared datetime-local picker ↔ API time range helpers.
 */

/** Format a Date for `<input type="datetime-local" step="1">` (local time, includes seconds). */
export function formatDateTimeLocalInput(date) {
    const d = date instanceof Date ? date : new Date(date);
    if (isNaN(d.getTime())) return '';
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/** Read global #from-ts / #to-ts into appState. Returns { fromTs, toTs } (local strings). */
export function syncTimeRangeFromPicker() {
    const fromInput = document.getElementById('from-ts');
    const toInput = document.getElementById('to-ts');
    if (fromInput?.value) window.appState.fromTs = fromInput.value;
    if (toInput?.value) window.appState.toTs = toInput.value;
    return { fromTs: window.appState.fromTs || '', toTs: window.appState.toTs || '' };
}

/** Parse datetime-local value (local wall time) to UTC ISO for APIs. */
export function localDateTimeToISO(localValue) {
    if (!localValue) return '';
    let s = String(localValue).trim();
    if (!s) return '';
    let d = new Date(s);
    if (isNaN(d.getTime()) && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(s)) {
        d = new Date(s + ':00');
    }
    return isNaN(d.getTime()) ? '' : d.toISOString();
}

/**
 * ISO range from appState (optionally sync global picker first).
 * @param {{ sync?: boolean, fallbackHours?: number }} [opts]
 */
export function getAppTimeRangeISO(opts = {}) {
    if (opts.sync !== false) syncTimeRangeFromPicker();

    const hours = opts.fallbackHours ?? 1;
    const now = Date.now();
    const fromLocal = window.appState.fromTs;
    const toLocal = window.appState.toTs;

    if (fromLocal && toLocal) {
        const from = localDateTimeToISO(fromLocal);
        const to = localDateTimeToISO(toLocal);
        if (from && to) return { from, to, fromLocal, toLocal };
    }

    const to = new Date(now).toISOString();
    const from = new Date(now - hours * 3600000).toISOString();
    return {
        from,
        to,
        fromLocal: formatDateTimeLocalInput(new Date(now - hours * 3600000)),
        toLocal: formatDateTimeLocalInput(new Date(now)),
    };
}

/** Apply a quick range ending now (hours back) and sync picker + appState. */
export function setQuickRangeHours(hours) {
    const h = Number(hours);
    if (!Number.isFinite(h) || h <= 0) return;
    const now = new Date();
    const from = new Date(now.getTime() - h * 3600 * 1000);
    const fromLocal = formatDateTimeLocalInput(from);
    const toLocal = formatDateTimeLocalInput(now);
    window.appState.fromTs = fromLocal;
    window.appState.toTs = toLocal;
    const fromInput = document.getElementById('from-ts');
    const toInput = document.getElementById('to-ts');
    if (fromInput) fromInput.value = fromLocal;
    if (toInput) toInput.value = toLocal;
}

/** Apply global time range: sync picker, refresh current view without full re-navigation when possible. */
export function applyGlobalTimeRangeRefresh() {
    syncTimeRangeFromPicker();
    const route = window.appState.activeViewId;

    switch (route) {
        case 'sqlserver-health-v2':
        case 'dashboard':
            if (typeof window.refreshDashboardData === 'function') {
                window.refreshDashboardData();
                return;
            }
            break;
        case 'sqlserver-workload':
            if (window.sqlserverWorkload?.refreshAll) {
                window.sqlserverWorkload.refreshAll();
                return;
            }
            break;
        case 'sqlserver-locks':
            if (typeof window.refreshMsLocksDashboard === 'function') {
                window.refreshMsLocksDashboard();
                return;
            }
            break;
        case 'drilldown-memory':
            if (typeof window.refreshMemoryDrilldown === 'function') {
                window.refreshMemoryDrilldown();
                return;
            }
            break;
        case 'enterprise-metrics':
            if (typeof window._enterpriseMetricsLoadData === 'function') {
                window._enterpriseMetricsLoadData();
                return;
            }
            break;
        case 'storage-index-health': {
            const inst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
            if (inst?.type === 'sqlserver' && typeof window.runSqlServerStorageIndexHealthDashboard === 'function') {
                const msSih = window.appState.msSih || (window.appState.msSih = {});
                const pad = (n) => String(n).padStart(2, '0');
                const fmt = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
                if (window.appState.fromTs) {
                    msSih.fromLocal = window.formatDateTimeLocalInput
                        ? window.formatDateTimeLocalInput(window.appState.fromTs).slice(0, 16)
                        : String(window.appState.fromTs).slice(0, 16);
                }
                if (window.appState.toTs) {
                    msSih.toLocal = window.formatDateTimeLocalInput
                        ? window.formatDateTimeLocalInput(window.appState.toTs).slice(0, 16)
                        : String(window.appState.toTs).slice(0, 16);
                }
                void window.runSqlServerStorageIndexHealthDashboard({ skipLoadingShell: true });
                return;
            }
            break;
        }
        case 'query-analysis':
            if (typeof window.qaRefreshAll === 'function') {
                window.qaRefreshAll();
                return;
            }
            break;
        case 'sqlserver-waits':
            if (typeof window._waitStatsV2LoadData === 'function') {
                window._waitStatsV2LoadData();
                return;
            }
            break;
        case 'pg-cpu':
            if (typeof window.initPgCpu === 'function') {
                void window.initPgCpu();
                return;
            }
            break;
        case 'pg-memory': {
            const inst = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
            if (inst?.name && typeof window.initPgMemoryCockpit === 'function') {
                void window.initPgMemoryCockpit(inst.name);
                return;
            }
            break;
        }
        default:
            break;
    }

    if (window.appNavigate && route) {
        window.appNavigate(route);
    }
}
