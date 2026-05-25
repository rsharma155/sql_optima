/*
 * SQL Optima — Chart.js helpers (classic script, runs immediately after Chart.js).
 * Do NOT mutate chart.options inside Chart.register hooks — Chart.js 4 proxies recurse on Object.set.
 */
(function () {
    if (typeof Chart === 'undefined') return;
    if (window.__sqlOptimaChartBootstrap) return;
    window.__sqlOptimaChartBootstrap = true;

    // Remove legacy plugin if a previous bundle registered it (avoids sticky recursion after deploy).
    try {
        const legacy = Chart.registry && Chart.registry.getPlugin('sqlOptimaTooltips');
        if (legacy) Chart.registry.remove(legacy);
    } catch (_) { /* ignore */ }

    function formatTooltipValue(value, dataset) {
        const n = Number(value);
        if (value == null || !Number.isFinite(n)) return '—';
        const unit = (dataset && (dataset.tooltipUnit || dataset.yAxisUnit)) || '';
        const decimals = (dataset && dataset.tooltipDecimals) != null
            ? dataset.tooltipDecimals
            : (Math.abs(n) >= 100 ? 0 : Math.abs(n) >= 10 ? 1 : 2);
        const formatted = n.toLocaleString(undefined, {
            minimumFractionDigits: decimals,
            maximumFractionDigits: decimals,
        });
        return unit ? formatted + ' ' + unit : formatted;
    }

    function tooltipNumericValue(ctx) {
        const chartType = ctx.chart && ctx.chart.config && ctx.chart.config.type;
        if (chartType === 'pie' || chartType === 'doughnut') {
            if (typeof ctx.raw === 'number' && Number.isFinite(ctx.raw)) return ctx.raw;
            if (typeof ctx.parsed === 'number' && Number.isFinite(ctx.parsed)) return ctx.parsed;
        }
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

    function tooltipTitle(items) {
        if (!items || !items.length) return '';
        const item = items[0];
        if (item.label != null && String(item.label) !== '') return String(item.label);
        const x = item.parsed && item.parsed.x;
        if (x != null && Number.isFinite(Number(x))) {
            const d = new Date(x);
            if (!isNaN(d.getTime())) return d.toLocaleString();
        }
        return '';
    }

    function tooltipLabel(ctx) {
        const label = (ctx.dataset && ctx.dataset.label) || '';
        const value = tooltipNumericValue(ctx);
        if (value == null) return label || '';
        const formatted = formatTooltipValue(value, ctx.dataset);
        return label ? label + ': ' + formatted : formatted;
    }

    /** Merge tooltip defaults into a plain options object (never chart.options proxy). */
    function applyTooltipToPlainOptions(opts) {
        if (!opts || opts.plugins?.tooltip?.enabled === false) return opts;

        opts.interaction = Object.assign({ mode: 'index', intersect: false }, opts.interaction || {});
        opts.plugins = opts.plugins || {};
        const tip = opts.plugins.tooltip || {};

        if (tip.enabled === false) return opts;

        opts.plugins.tooltip = Object.assign({
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
            callbacks: {
                title: tooltipTitle,
                label: tooltipLabel,
            },
        }, tip, {
            callbacks: Object.assign(
                { title: tooltipTitle, label: tooltipLabel },
                tip.callbacks || {}
            ),
        });

        return opts;
    }

    try {
        if (Chart.defaults.interaction) {
            Chart.defaults.interaction.mode = 'index';
            Chart.defaults.interaction.intersect = false;
        }
        const defTip = Chart.defaults.plugins && Chart.defaults.plugins.tooltip;
        if (defTip) {
            defTip.enabled = true;
            if (!defTip.mode) defTip.mode = 'index';
            defTip.intersect = false;
            defTip.callbacks = defTip.callbacks || {};
            if (!defTip.callbacks.label) defTip.callbacks.label = tooltipLabel;
            if (!defTip.callbacks.title) defTip.callbacks.title = tooltipTitle;
        }
        if (Chart.defaults.elements && Chart.defaults.elements.point) {
            Chart.defaults.elements.point.hitRadius = 12;
            Chart.defaults.elements.point.hoverRadius = 6;
        }
    } catch (_) { /* non-fatal */ }

    window.createOptimaChart = function (ctx, config) {
        if (!config) return new Chart(ctx, config);
        const cfg = {
            ...config,
            options: config.options
                ? applyTooltipToPlainOptions({ ...config.options, plugins: { ...config.options.plugins } })
                : config.options,
        };
        return new Chart(ctx, cfg);
    };

    window.destroyChartOnCanvas = function (canvasOrId) {
        const canvas = typeof canvasOrId === 'string' ? document.getElementById(canvasOrId) : canvasOrId;
        if (!canvas) return;
        const existing = Chart.getChart(canvas);
        if (existing) {
            try { existing.destroy(); } catch (_) { /* ignore */ }
        }
        const id = canvas.id;
        if (id) {
            if (!window.currentCharts) window.currentCharts = {};
            if (window.currentCharts[id]) {
                try { window.currentCharts[id].destroy(); } catch (_) { /* ignore */ }
                window.currentCharts[id] = null;
            }
        }
    };
})();
