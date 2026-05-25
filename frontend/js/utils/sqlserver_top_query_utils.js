/**
 * SQL Optima — helpers for SQL Server top-query tables (Query Analysis + Workload).
 */
(function () {
    const HELP =
        'Totals are the sum of per-collector-interval deltas in the selected time range (not live DMV lifetime counters). ' +
        'Averages are weighted: total metric ÷ total executions.';

    function rollupKey(row) {
        const fp = String(row.statement_fingerprint || '').trim().toLowerCase();
        if (fp) return 'fp:' + fp;
        const h = String(row.query_hash || '').trim().toLowerCase();
        return h ? 'hash:' + h : '';
    }

    function dedupeSqlServerTopQueries(rows) {
        const seen = new Set();
        const out = [];
        for (const r of rows || []) {
            const key = rollupKey(r);
            if (!key) {
                out.push(r);
                continue;
            }
            if (seen.has(key)) continue;
            seen.add(key);
            out.push(r);
        }
        return out;
    }

    window.SQLSERVER_TOP_QUERY_METRICS_HELP = HELP;
    window.dedupeSqlServerTopQueries = dedupeSqlServerTopQueries;
})();
