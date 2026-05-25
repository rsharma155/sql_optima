/*
 * SQL Optima — shared Chart.js defaults and helpers for time-series / bar charts.
 * Tooltip injection is handled by chart-bootstrap.js (loaded with Chart.js).
 */

/** Format a numeric chart value for tooltips. */
export function formatChartTooltipValue(value, dataset = {}) {
    const n = Number(value);
    if (value == null || !Number.isFinite(n)) return '—';
    const unit = dataset.tooltipUnit || dataset.yAxisUnit || '';
    const decimals = dataset.tooltipDecimals ?? (Math.abs(n) >= 100 ? 0 : Math.abs(n) >= 10 ? 1 : 2);
    const formatted = n.toLocaleString(undefined, {
        minimumFractionDigits: decimals,
        maximumFractionDigits: decimals,
    });
    return unit ? `${formatted} ${unit}` : formatted;
}

/** Read Y (or X for horizontal bars) from a tooltip context. */
export function tooltipNumericValue(ctx) {
    const p = ctx.parsed;
    if (p != null) {
        if (typeof p === 'number' && Number.isFinite(p)) return p;
        if (p.y != null && Number.isFinite(Number(p.y))) return Number(p.y);
        if (p.x != null && Number.isFinite(Number(p.x))) return Number(p.x);
    }
    const raw = ctx.raw;
    if (typeof raw === 'number' && Number.isFinite(raw)) return raw;
    if (raw && typeof raw === 'object' && raw.y != null && Number.isFinite(Number(raw.y))) {
        return Number(raw.y);
    }
    return null;
}

/** Standard hover interaction for trend / bar charts. */
export function chartInteractionOptions() {
    return { mode: 'index', intersect: false };
}

/** Tooltip plugin block merged into chart options. */
export function chartTooltipPlugin(overrides = {}) {
    const baseCallbacks = {
        title: (items) => {
            if (!items?.length) return '';
            const item = items[0];
            if (item.label != null && String(item.label) !== '') return String(item.label);
            const x = item.parsed?.x;
            if (x != null && Number.isFinite(Number(x))) {
                const d = new Date(x);
                if (!isNaN(d.getTime())) return d.toLocaleString();
            }
            return '';
        },
        label: (ctx) => {
            const label = ctx.dataset?.label || '';
            const value = tooltipNumericValue(ctx);
            if (value == null) return label || '';
            const formatted = formatChartTooltipValue(value, ctx.dataset);
            return label ? `${label}: ${formatted}` : formatted;
        },
    };
    const { callbacks: overrideCallbacks, ...restOverrides } = overrides;
    return {
        enabled: true,
        mode: 'index',
        intersect: false,
        backgroundColor: 'rgba(15, 23, 42, 0.92)',
        titleColor: '#e2e8f0',
        bodyColor: '#cbd5e1',
        borderColor: 'rgba(148, 163, 184, 0.35)',
        borderWidth: 1,
        padding: 10,
        titleFont: { size: 11, weight: 'bold' },
        bodyFont: { size: 11 },
        ...restOverrides,
        callbacks: {
            ...baseCallbacks,
            ...(overrideCallbacks || {}),
        },
    };
}

/** X-axis config with visible time tick labels and optional title. */
export function chartTimeScaleX(titleText = 'Time') {
    return {
        display: true,
        title: {
            display: Boolean(titleText),
            text: titleText,
            color: '#64748b',
            font: { size: 10, weight: '500' },
        },
        ticks: {
            color: '#64748b',
            maxTicksLimit: 10,
            maxRotation: 0,
            autoSkip: true,
            font: { size: 9 },
        },
        grid: { color: 'rgba(100, 116, 139, 0.12)', drawOnChartArea: true },
    };
}

/** Deep-merge chart options (shallow for plugins/scales). */
export function mergeChartOptions(base, extra) {
    const out = { ...base, ...extra };
    if (base?.plugins || extra?.plugins) {
        out.plugins = { ...(base?.plugins || {}), ...(extra?.plugins || {}) };
        const mergedTip = { ...(base?.plugins?.tooltip || {}), ...(extra?.plugins?.tooltip || {}) };
        if (Object.keys(mergedTip).length) {
            out.plugins.tooltip = chartTooltipPlugin(mergedTip);
        }
    }
    if (base?.scales || extra?.scales) {
        out.scales = { ...(base?.scales || {}), ...(extra?.scales || {}) };
    }
    if (base?.interaction || extra?.interaction) {
        out.interaction = { ...(base?.interaction || {}), ...(extra?.interaction || {}) };
    }
    return out;
}

/** Base options for line / bar trend charts. */
export function chartTrendOptions({ xTitle = 'Time', yTitle = '', tooltipOverrides } = {}) {
    const scales = { x: chartTimeScaleX(xTitle) };
    if (yTitle) {
        scales.y = {
            ticks: { color: '#64748b', font: { size: 9 } },
            grid: { color: 'rgba(100, 116, 139, 0.12)' },
            title: { display: true, text: yTitle, color: '#64748b', font: { size: 10 } },
        };
    } else {
        scales.y = {
            ticks: { color: '#64748b', font: { size: 9 } },
            grid: { color: 'rgba(100, 116, 139, 0.12)' },
        };
    }
    return {
        responsive: true,
        maintainAspectRatio: false,
        interaction: chartInteractionOptions(),
        plugins: {
            legend: { labels: { color: '#94a3b8', font: { size: 11 }, boxWidth: 12 } },
            tooltip: chartTooltipPlugin(tooltipOverrides),
        },
        scales,
    };
}

/** Monitoring role from optima_servers.username (exposed as monitoring_user on /api/config). */
export function getInstanceMonitoringUser(inst) {
    if (!inst) return '';
    const raw = (inst.monitoring_user || inst.user || '').trim();
    return raw.toLowerCase();
}

/** Drop rows executed by the instance monitoring user (and obvious collector SQL). */
export function filterOutMonitoringPgQueries(rows, inst) {
    if (!Array.isArray(rows) || rows.length === 0) return rows;
    const monitoringUser = getInstanceMonitoringUser(inst);
    return rows.filter((q) => {
        const text = (q.query || '').trim();
        if (!text || text === '<insufficient privilege>') return false;
        if (text.includes('/* SQL_OPTIMA')) return false;
        if (monitoringUser && String(q.username || q.user_name || '').toLowerCase() === monitoringUser) {
            return false;
        }
        return true;
    });
}

/** No-op if chart-bootstrap.js already registered the plugin. */
export function applyChartJsDefaults() {
    if (typeof Chart === 'undefined') return;
    if (window.__sqlOptimaChartTooltipsRegistered) return;
    // Fallback when entry.js loads before chart-bootstrap (should not happen).
    if (typeof window.createOptimaChart === 'undefined') {
        window.createOptimaChart = (ctx, config) => new Chart(ctx, config);
    }
}
