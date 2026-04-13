window.loadTemplate = async function(url) {
    // Templates change frequently during development; avoid stale in-memory cache.
    // We still keep a best-effort cache, but always re-fetch and overwrite it.
    const response = await fetch(url, { cache: 'no-store' });
    if (!response.ok) {
        console.error(`Template ${url} missing. Generating Empty Block.`);
        return `<div class="p-3 text-warning">Template ${url} Failed to Load</div>`;
    }
    const text = await response.text();
    window.templateCache = window.templateCache || {};
    window.templateCache[url] = text;
    return text;
};

/**
 * Shared dashboard header helpers (keeps UI consistent across pages).
 */
window.renderStatusStrip = function renderStatusStrip(opts) {
    const lastUpdateId = opts?.lastUpdateId || 'lastRefreshTime';
    const sourceBadgeId = opts?.sourceBadgeId || 'dataSourceBadge';
    const freshnessId = opts?.freshnessId || 'freshnessChip';
    const autoRefreshText = opts?.autoRefreshText || '';
    const includeHealth = !!opts?.includeHealth;
    const healthText = opts?.healthText || 'OK';
    const healthClass = opts?.healthClass || 'badge badge-success';
    const includeFreshness = !!opts?.includeFreshness;
    const includeSource = opts?.includeSource !== false;

    return `
        <div class="flex-between" style="align-items:center; gap: 1rem;">
            <div style="display:flex; gap:0.4rem; align-items:center; flex-wrap:wrap; justify-content:flex-end;">
                ${includeHealth ? `<span class="${healthClass}" style="font-size:0.65rem;">Health: ${window.escapeHtml(healthText)}</span>` : ''}
                ${includeSource ? `<span id="${sourceBadgeId}" class="badge badge-info" style="font-size:0.65rem; display:none;">Source</span>` : ''}
                ${includeFreshness ? `<span class="badge badge-outline" style="font-size:0.65rem;">Freshness: <span id="${freshnessId}">--</span></span>` : ''}
            </div>
            <div style="text-align:right;">
                <span class="text-muted" style="font-size:0.75rem;">Last Update: <span id="${lastUpdateId}">--:--:--</span></span>
                ${autoRefreshText ? `<span class="text-muted" style="font-size:0.65rem; display:block;">${window.escapeHtml(autoRefreshText)}</span>` : ''}
            </div>
        </div>
    `;
};

window.updateSourceBadge = function updateSourceBadge(badgeId, sourceHeaderValue) {
    const el = document.getElementById(badgeId);
    if (!el) return;

    const raw = (sourceHeaderValue || '').toString().trim().toLowerCase();
    if (!raw) {
        el.style.display = 'none';
        return;
    }

    // Normalize common values
    let label = raw;
    if (raw === 'timescale') label = 'Timescale snapshot';
    if (raw === 'live_cache') label = 'Live DMV (cached)';
    if (raw === 'live_cache_fallback') label = 'Live DMV fallback';
    if (raw === 'live') label = 'Live DMV';

    el.textContent = `Source: ${label}`;
    el.className = 'badge badge-info';
    if (raw.includes('fallback')) el.className = 'badge badge-warning';
    if (raw.includes('timescale')) el.className = 'badge badge-success';

    el.style.display = 'inline-block';
};

function _escOverlay(s) {
    const t = String(s ?? '');
    if (typeof window.escapeHtml === 'function') return window.escapeHtml(t);
    return t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

/** True if both values are set and From is strictly before To (datetime-local strings). */
window.isDatetimeLocalRangeValid = function(fromVal, toVal) {
    if (fromVal == null || toVal == null || fromVal === '' || toVal === '') return true;
    const a = new Date(fromVal).getTime();
    const b = new Date(toVal).getTime();
    if (isNaN(a) || isNaN(b)) return false;
    return a < b;
};

/** Empty string if OK; otherwise a short validation message. */
window.getDatetimeLocalRangeError = function(fromVal, toVal) {
    if (!fromVal || !toVal) return '';
    if (!window.isDatetimeLocalRangeValid(fromVal, toVal)) return 'From must be earlier than To.';
    return '';
};

window.showDateRangeValidationError = function(message) {
    const m = message || 'Invalid date range.';
    if (typeof window.optimaAlert === 'function') {
        window.optimaAlert(m);
        return;
    }
    alert(m);
};

window.clearChartOverlay = function(canvasId) {
    const canvas = typeof canvasId === 'string' ? document.getElementById(canvasId) : canvasId;
    if (!canvas) return;
    const host = canvas.closest && canvas.closest('.chart-container');
    const parent = host || canvas.parentElement;
    if (!parent) return;
    parent.querySelectorAll('.chart-overlay-state').forEach(function(el) { el.remove(); });
};

/**
 * @param {string} canvasId
 * @param {'loading'|'empty'|'clear'} kind
 * @param {string} [message]
 */
window.setChartOverlayState = function(canvasId, kind, message) {
    if (kind === 'clear') {
        window.clearChartOverlay(canvasId);
        return;
    }
    window.clearChartOverlay(canvasId);
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;
    const host = canvas.closest('.chart-container') || canvas.parentElement;
    if (!host) return;
    const pos = window.getComputedStyle(host).position;
    if (!pos || pos === 'static') host.style.position = 'relative';
    const div = document.createElement('div');
    div.className = 'chart-overlay-state';
    div.setAttribute('role', 'status');
    div.style.cssText = 'position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:0.5rem;background:rgba(15,23,42,0.55);z-index:3;font-size:0.82rem;color:var(--text-muted);text-align:center;padding:0.5rem;';
    if (kind === 'loading') {
        div.innerHTML = '<div class="spinner"></div><span>' + _escOverlay(message || 'Loading…') + '</span>';
    } else {
        div.innerHTML = '<span>' + _escOverlay(message || 'No data') + '</span>';
    }
    host.appendChild(div);
};
