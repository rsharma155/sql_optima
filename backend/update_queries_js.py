import sys

path = '../frontend/js/pages/queries.js'
with open(path, 'r') as f:
    content = f.read()

# 1. Fix Top Queries sorting selectors
content = content.replace(\"#pgss-top-table .pgss-sortable-th\", \"#pgss-top-grid .pgss-sortable-th\")

# 2. Add Regression sorting logic
regr_sort_code = \"\"\"
window._pgssRegrSort = 'change';
window._pgssRegrSortDir = 'desc';
window._pgssRegrCache = [];

window.pgssRegressionSort = function(key) {
    const keyMap = { query: 'query', prev: 'prev_avg_ms', curr: 'curr_avg_ms', change: 'change_pct' };
    if (window._pgssRegrSort === key) {
        window._pgssRegrSortDir = window._pgssRegrSortDir === 'desc' ? 'asc' : 'desc';
    } else {
        window._pgssRegrSort = key;
        window._pgssRegrSortDir = 'desc';
    }
    
    // Update header icons
    document.querySelectorAll('#pgss-regression-table .pgss-sortable-th').forEach(th => {
        const icon = th.querySelector('i');
        if (!icon) return;
        if (th.dataset.sort === key) {
            icon.className = window._pgssRegrSortDir === 'desc' ? 'fa-solid fa-sort-down' : 'fa-solid fa-sort-up';
        } else {
            icon.className = 'fa-solid fa-sort';
        }
    });

    const field = keyMap[key] || key;
    const dir = window._pgssRegrSortDir === 'desc' ? -1 : 1;
    const sorted = [...window._pgssRegrCache].sort((a, b) => {
        let va = a[field], vb = b[field];
        if (typeof va === 'string') return va.localeCompare(vb) * dir;
        return ((va ?? 0) - (vb ?? 0)) * dir;
    });
    pgssRenderRegressionRows(sorted);
};

function pgssRenderRegressionRows(regs) {
    const tbody = document.getElementById('pgss-regression-tbody');
    if (!tbody) return;
    if (regs.length === 0) {
        tbody.innerHTML = '<tr><td colspan=\"6\" class=\"text-center text-muted\">No regressions detected.</td></tr>';
        return;
    }
    tbody.innerHTML = regs.map(r => {
        const decodedSql = pgssSmartDecode(r.query || '');
        return `
            <tr>
                <td style=\"max-width:420px; cursor:pointer; text-decoration:underline; text-underline-offset:2px;\" title=\"Click to view full query\" data-action=\"call\" data-fn=\"pgssOpenRegressionQuery\" data-arg=\"${pgssEscapeHtml(decodedSql)}\">${pgssEscapeHtml(pgssTrancate(decodedSql, 80))}</td>
                <td class=\"text-right\">${pgssFmtMs(r.prev_avg_ms)}</td>
                <td class=\"text-right\">${pgssFmtMs(r.curr_avg_ms)}</td>
                <td class=\"text-right ${r.change_pct > 50 ? 'text-danger' : r.change_pct > 20 ? 'text-warning' : ''}\">+${r.change_pct.toFixed(0)}%</td>
                <td class=\"text-center\"><span class=\"badge ${r.status === 'Degraded' ? 'badge-danger' : 'badge-warning'}\">${r.status}</span></td>
                <td class=\"text-right\">${r.detected_at ? new Date(r.detected_at).toLocaleTimeString() : '-'}</td>
            </tr>
        `; }).join('');
}
\"\"\"

# Insert regression sort code before loadPgssRegressions
content = content.replace(\"async function loadPgssRegressions()\", regr_sort_code + \"\\nasync function loadPgssRegressions()\")

# Update loadPgssRegressions to use the cache and renderer
old_regr_logic = \"\"\"        const data = await resp.json();
        const regs = data.regressions || [];
        if (regs.length === 0) {
            tbody.innerHTML = '<tr><td colspan=\"5\" class=\"text-muted\">No regressions detected</td></tr>';
            return;
        }
        tbody.innerHTML = regs.map(r => {
            const decodedSql = pgssSmartDecode(r.query || '');
            return `
            <tr>
                <td style=\"max-width:420px; cursor:pointer; text-decoration:underline; text-underline-offset:2px;\" title=\"Click to view full query\" data-action=\"call\" data-fn=\"pgssOpenRegressionQuery\" data-arg=\"${pgssEscapeHtml(decodedSql)}\">${pgssEscapeHtml(pgssTrancate(decodedSql, 80))}</td>
                <td>${pgssFmtMs(r.prev_avg_ms)}</td>
                <td>${pgssFmtMs(r.curr_avg_ms)}</td>
                <td class=\"${r.change_pct > 50 ? 'text-danger' : r.change_pct > 20 ? 'text-warning' : ''}\">+${r.change_pct.toFixed(0)}%</td>
                <td><span class=\"badge ${r.status === 'Degraded' ? 'badge-danger' : 'badge-warning'}\">${r.status}</span></td>
                <td>${r.detected_at ? new Date(r.detected_at).toLocaleTimeString() : '-'}</td>
            </tr>
        `; }).join('');\"\"\"

new_regr_logic = \"\"\"        const data = await resp.json();
        window._pgssRegrCache = data.regressions || [];
        pgssRenderRegressionRows(window._pgssRegrCache);
        
        // Wire up sortable headers if not done
        document.querySelectorAll('#pgss-regression-table .pgss-sortable-th').forEach(th => {
            if (!th.dataset.wired) {
                th.dataset.wired = '1';
                th.addEventListener('click', () => window.pgssRegressionSort(th.dataset.sort));
            }
        });\"\"\"

content = content.replace(old_regr_logic, new_regr_logic)

# 3. Add Wide View logic
wide_view_code = \"\"\"
window.togglePgssWideView = function() {
    const grid = document.getElementById('pgss-top-grid');
    if (!grid) return;
    grid.classList.toggle('wide-view');
    const btn = document.getElementById('btn-wide-view');
    if (btn) {
        const isWide = grid.classList.contains('wide-view');
        btn.innerHTML = isWide ? '<i class=\"fa-solid fa-compress\"></i> Standard View' : '<i class=\"fa-solid fa-expand\"></i> Wide View';
    }
};
\"\"\"

content += wide_view_code

# Add event listener for wide view button in initPgssSection
content = content.replace(
    \"checkPgssStatus(instanceName);\",
    \"checkPgssStatus(instanceName);\\n    const wideBtn = document.getElementById('btn-wide-view');\\n    if (wideBtn) wideBtn.onclick = window.togglePgssWideView;\"
)

with open(path, 'w') as f:
    f.write(content)
