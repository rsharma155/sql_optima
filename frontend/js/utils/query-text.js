/*
 * SQL Optima — decode URL-encoded query text from APIs or data attributes.
 */

/** True when the string still looks percent-encoded (e.g. %20, %2C). */
function looksPercentEncoded(s) {
    return /%(?:[0-9A-Fa-f]{2}|$)/.test(s);
}

/**
 * Decode query text that may be URL-encoded once or multiple times.
 * Also normalizes application/x-www-form-urlencoded '+' spaces.
 */
export function decodeQueryText(raw) {
    if (raw == null || raw === '') return '';
    let s = String(raw).replace(/\+/g, ' ');
    if (!looksPercentEncoded(s)) return s;

    for (let i = 0; i < 5 && looksPercentEncoded(s); i++) {
        try {
            const next = decodeURIComponent(s);
            if (next === s) break;
            s = next.replace(/\+/g, ' ');
        } catch {
            break;
        }
    }
    return s;
}
