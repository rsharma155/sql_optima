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
