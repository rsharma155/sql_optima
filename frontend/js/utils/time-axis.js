/*
 * SQL Optima — shared chart timestamp parsing and series alignment.
 * APIs use capture_timestamp, timestamp, time, ts, etc.; charts should use these helpers.
 */

const TS_FIELD_ORDER = [
    'capture_timestamp',
    'timestamp',
    'bucket',
    'time',
    'ts',
    'event_time',
    'snapshot_time',
    'event_timestamp',
];

/** Raw value from a row or a primitive timestamp. */
function rawTimestampValue(rowOrValue) {
    if (rowOrValue == null) return null;
    if (typeof rowOrValue !== 'object') return rowOrValue;
    if (rowOrValue instanceof Date) return rowOrValue;
    for (const key of TS_FIELD_ORDER) {
        const v = rowOrValue[key];
        if (v != null && v !== '') return v;
    }
    return null;
}

/** Milliseconds since epoch, or null if unparseable. */
export function tsMs(rowOrValue) {
    const raw = rawTimestampValue(rowOrValue);
    if (raw == null) return null;
    if (typeof raw === 'number' && isFinite(raw)) return raw;
    if (raw instanceof Date) {
        const t = raw.getTime();
        return isNaN(t) ? null : t;
    }
    let s = String(raw).trim();
    if (!s) return null;
    if (s.includes(' ') && !s.includes('T')) s = s.replace(' ', 'T');
    const parsed = Date.parse(s);
    return isNaN(parsed) ? null : parsed;
}

/** Date for Chart.js { x } or sorting; null if invalid. */
export function parseChartTimestamp(rowOrValue) {
    const ms = tsMs(rowOrValue);
    return ms == null ? null : new Date(ms);
}

/** Safe axis tick label (time of day). */
export function fmtChartTick(rowOrValue, opts = {}) {
    const d = parseChartTimestamp(rowOrValue);
    if (!d) return '';
    const { hour = '2-digit', minute = '2-digit', second, dateStyle } = opts;
    if (dateStyle === 'short') {
        return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
    }
    const timeOpts = { hour, minute };
    if (second != null) timeOpts.second = second;
    return d.toLocaleTimeString([], timeOpts);
}

/** Copy sorted by chart time ascending. */
export function sortByChartTime(arr) {
    if (!Array.isArray(arr)) return [];
    return [...arr].sort((a, b) => {
        const ta = tsMs(a);
        const tb = tsMs(b);
        if (ta == null && tb == null) return 0;
        if (ta == null) return 1;
        if (tb == null) return -1;
        return ta - tb;
    });
}

/**
 * Align multiple series to a unified time axis.
 * @param {Array<{ points: Array, getValue?: (row) => number }>} seriesDefs
 * @returns {{ labels: string[], times: number[], series: number[][] }}
 */
export function alignSeriesByTime(seriesDefs) {
    const timeSet = new Set();
    const normalized = (seriesDefs || []).map(def => {
        const points = Array.isArray(def.points) ? def.points : [];
        const getValue = def.getValue || ((row) => row.value);
        const byTime = new Map();
        points.forEach(row => {
            const ms = tsMs(row);
            if (ms == null) return;
            byTime.set(ms, getValue(row));
            timeSet.add(ms);
        });
        return { label: def.label, byTime, getValue };
    });

    const times = [...timeSet].sort((a, b) => a - b);
    const labels = times.map(ms => fmtChartTick(ms));
    const series = normalized.map(({ byTime }) =>
        times.map(ms => (byTime.has(ms) ? byTime.get(ms) : null))
    );

    return { labels, times, series, seriesMeta: normalized.map(n => n.label) };
}

/** Same as parseChartTimestamp but never returns null (falls back to now). */
export function parseChartTimestampOrNow(rowOrValue) {
    return parseChartTimestamp(rowOrValue) || new Date();
}
