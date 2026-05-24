/*
 * SQL Optima — Shared utilities for Storage & Index Health dashboards.
 * Used by both sqlserver_storage_index_health_dashboard.js and
 * pg_storage_index_health_dashboard.js.
 */

window.sihShared = (function() {

    function fmt(n, d = 0) {
        const v = Number(n);
        if (n == null || isNaN(v)) return '--';
        return v.toLocaleString('en-US', { minimumFractionDigits: d, maximumFractionDigits: d });
    }

    function escH(s) { return window.escapeHtml ? window.escapeHtml(String(s || '')) : String(s || ''); }

    function emptyRow(cols, msg) {
        return `<tr><td colspan="${cols}" class="text-center text-muted p-3">${msg}</td></tr>`;
    }

    function buildFilterQS(state, instName) {
        const fromIso = new Date(state.fromLocal).toISOString();
        const toIso   = new Date(state.toLocal).toISOString();
        const db      = (state.db     && state.db     !== 'all') ? state.db     : '';
        const schema  = (state.schema && state.schema !== 'all') ? state.schema : '';
        const table   = (state.table  && state.table  !== 'all') ? state.table  : '';
        return `instance=${encodeURIComponent(instName)}&from=${encodeURIComponent(fromIso)}&to=${encodeURIComponent(toIso)}&db=${encodeURIComponent(db)}&schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}`;
    }

    function renderSparkline(canvasId, data, color) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return;
        const existing = Chart.getChart(canvas);
        if (existing) existing.destroy();
        if (!data || !data.length) return;
        new Chart(canvas, {
            type: 'line',
            data: {
                labels: data.map((_, i) => i),
                datasets: [{ data, borderColor: color, borderWidth: 1.5, pointRadius: 0, fill: false, tension: 0.4 }]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { 
                    legend: { display: false }, 
                    tooltip: { 
                        enabled: true,
                        callbacks: {
                            label: (context) => fmt(context.raw, 1) + ' MB'
                        }
                    } 
                },
                scales: { x: { display: false }, y: { display: false } }
            }
        });
    }

    function renderGrowthChart(canvasId, growth, engine) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return;
        const existing = Chart.getChart(canvas);
        if (existing) existing.destroy();
        if (!growth || !growth.length) {
            canvas.parentElement.innerHTML = '<p class="text-center text-muted p-5" style="font-size:0.8rem;"><i class="fa-solid fa-chart-line mb-2 d-block" style="font-size:1.5rem; opacity:0.3;"></i>No growth history data available for this window.</p>';
            return;
        }
        const labels = growth.map(p => new Date(p.bucket).toLocaleDateString());
        const datasets = [
            { label: 'Table Data', data: growth.map(p => p.table_size_mb), borderColor: '#3b82f6', backgroundColor: '#3b82f622', fill: true, tension: 0.3, pointRadius: 3, pointHoverRadius: 5 },
            { label: 'Indexes',    data: growth.map(p => p.index_size_mb), borderColor: '#10b981', backgroundColor: '#10b98122', fill: true, tension: 0.3, pointRadius: 3, pointHoverRadius: 5 }
        ];
        new Chart(canvas, {
            type: 'line',
            data: { labels, datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                scales: {
                    y: { 
                        beginAtZero: false, 
                        ticks: { color: '#94a3b8', font: { size: 10 } }, 
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        title: { display: true, text: 'Size (MB)', color: '#94a3b8', font: { size: 10 } }
                    },
                    x: { 
                        ticks: { color: '#94a3b8', font: { size: 10 } }, 
                        grid: { display: false },
                        title: { display: true, text: 'Date', color: '#94a3b8', font: { size: 10 } }
                    }
                },
                plugins: { 
                    legend: { labels: { color: '#94a3b8', font: { size: 10 } } },
                    tooltip: {
                        mode: 'index',
                        intersect: false,
                        callbacks: {
                            label: function(context) {
                                return context.dataset.label + ': ' + fmt(context.raw, 1) + ' MB';
                            }
                        }
                    }
                }
            }
        });
    }

    function renderTopGrowthChart(canvasId, topTables, mode) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return;
        const existing = Chart.getChart(canvas);
        if (existing) existing.destroy();
        if (!topTables || !topTables.length) {
            canvas.parentElement.innerHTML = '<p class="text-center text-muted p-5" style="font-size:0.8rem;"><i class="fa-solid fa-arrow-up-right-dots mb-2 d-block" style="font-size:1.5rem; opacity:0.3;"></i>No significant growth detected in the last 7 days.</p>';
            return;
        }
        const labels = topTables.map(t => `${t.schema_name}.${t.table_name}`);
        const data   = topTables.map(t => {
            const growth = Number(t.value) || 0;
            const base   = Number(t.value2) || 0;
            if (mode === 'pct') {
                return base > 0 ? (growth / base) * 100 : 0;
            }
            return growth;
        });
        const label  = mode === 'pct' ? 'Growth (%)' : 'Growth (MB)';
        new Chart(canvas, {
            type: 'bar',
            data: { labels, datasets: [{ label, data, backgroundColor: '#f43f5e', borderRadius: 3 }] },
            options: {
                indexAxis: 'y', responsive: true, maintainAspectRatio: false,
                scales: {
                    x: { 
                        ticks: { color: '#94a3b8', font: { size: 9 } },
                        title: { display: true, text: mode === 'pct' ? 'Growth Percentage (%)' : 'Growth Size (MB)', color: '#94a3b8', font: { size: 10 } }
                    },
                    y: { 
                        ticks: { color: '#94a3b8', font: { size: 9 }, maxRotation: 0 },
                        title: { display: true, text: 'Table Name', color: '#94a3b8', font: { size: 10 } }
                    }
                },
                plugins: { 
                    legend: { labels: { color: '#94a3b8', font: { size: 10 } } },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return label + ': ' + fmt(context.raw, 1) + (mode === 'pct' ? '%' : ' MB');
                            }
                        }
                    }
                }
            }
        });
    }

    function renderSeekScanLookupChart(canvasId, data, engine) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return;
        const existing = Chart.getChart(canvas);
        if (existing) existing.destroy();
        if (!data || !data.length) {
            canvas.parentElement.innerHTML = '<p class="text-center text-muted p-3" style="font-size:0.8rem;">No access pattern data for selected window.</p>';
            return;
        }
        const labels = data.map(r => `${r.schema_name}.${r.table_name}`);
        const isPg   = engine === 'postgres';
        new Chart(canvas, {
            type: 'bar',
            data: {
                labels,
                datasets: [
                    { label: isPg ? 'Index Scans' : 'Seeks',   data: data.map(r => r.seeks),   backgroundColor: '#3b82f6' },
                    { label: isPg ? 'Seq Scans'   : 'Scans',   data: data.map(r => r.scans),   backgroundColor: '#f59e0b' },
                    { label: isPg ? 'Bitmap Scans': 'Lookups', data: data.map(r => r.lookups), backgroundColor: '#10b981' }
                ]
            },
            options: {
                indexAxis: 'y', responsive: true, maintainAspectRatio: false,
                plugins: { 
                    legend: { position: 'top', labels: { color: '#94a3b8', font: { size: 10 } } }, 
                    tooltip: { 
                        mode: 'index', 
                        intersect: false,
                        callbacks: {
                            footer: (items) => {
                                const total = items.reduce((s, i) => s + i.raw, 0);
                                return 'Total Operations: ' + total.toLocaleString();
                            }
                        }
                    } 
                },
                scales: {
                    x: { 
                        stacked: true, 
                        ticks: { color: '#94a3b8', font: { size: 9 } },
                        title: { display: true, text: 'Operation Count', color: '#94a3b8', font: { size: 10 } }
                    },
                    y: { 
                        stacked: true, 
                        ticks: { color: '#94a3b8', font: { size: 9 }, maxRotation: 0 },
                        title: { display: true, text: 'Table Name', color: '#94a3b8', font: { size: 10 } }
                    }
                }
            }
        });
    }

    function buildHealthScore(kpis) {
        let score = 100;
        score -= Math.min(40, (kpis.unused_index_count || 0) * 2);
        score -= Math.min(20, (kpis.high_scan_table_count || 0) * 1);
        score -= Math.min(20, (kpis.index_write_overhead_pct || 0) / 3);
        const g = kpis.growth_7d_pct || 0;
        if (g > 5) score -= Math.min(20, (g - 5) * 1.5);
        return Math.max(0, Math.round(score));
    }

    function buildBannerMessages(kpis, growthSummary) {
        const msgs = [];
        const unusedGB = (kpis.unused_index_mb || 0) / 1024;
        const unusedIsCritical = kpis.unused_index_count > 20 || unusedGB > 1;

        if (unusedIsCritical)
            msgs.push({ sev: 'critical', text: `${kpis.unused_index_count} unused indexes consuming ${fmt(kpis.unused_index_mb, 0)} MB reclaimable space.` });
        if ((kpis.growth_7d_pct || 0) > 15)
            msgs.push({ sev: 'critical', text: `Storage growing at ${fmt(kpis.growth_7d_pct, 1)}% per week — capacity risk.` });
        if ((kpis.index_write_overhead_pct || 0) > 60)
            msgs.push({ sev: 'critical', text: `Index write overhead at ${fmt(kpis.index_write_overhead_pct, 1)}% — over-indexed for write workload.` });
        if ((kpis.high_scan_table_count || 0) > 10)
            msgs.push({ sev: 'warning', text: `${kpis.high_scan_table_count} tables have dominant scan patterns — missing index candidates.` });
        if ((kpis.growth_7d_pct || 0) > 5 && (kpis.growth_7d_pct || 0) <= 15)
            msgs.push({ sev: 'warning', text: `Storage expanding at ${fmt(kpis.growth_7d_pct, 1)}% per week.` });
        if (!unusedIsCritical && kpis.unused_index_count > 0)
            msgs.push({ sev: 'info', text: `${kpis.unused_index_count} unused indexes (${fmt(kpis.unused_index_mb, 0)} MB) eligible for review.` });
        if (!msgs.length)
            msgs.push({ sev: 'healthy', text: 'Storage and index health stable over the selected period.' });
        return msgs;
    }

    function renderBanner(bannerId, kpis, growthSummary) {
        const el = document.getElementById(bannerId);
        if (!el) return;
        const msgs = buildBannerMessages(kpis, growthSummary);
        const sevCls = { critical: 'alert-danger', warning: 'alert-warning', info: 'alert-info', healthy: 'alert-success' };
        const sevIcon = { critical: 'fa-triangle-exclamation', warning: 'fa-triangle-exclamation', info: 'fa-circle-info', healthy: 'fa-check-circle' };
        el.style.display = 'block';
        el.innerHTML = msgs.map(m => `
            <div class="alert ${sevCls[m.sev]}" style="margin-bottom:0.25rem; font-size:0.85rem; border-radius:6px; padding:0.4rem 0.75rem;">
                <i class="fa-solid ${sevIcon[m.sev]}"></i> ${m.text}
            </div>
        `).join('');
    }

    function renderProjectionStrip(dash) {
        const gs = dash.growth_summary || {};
        const el = id => document.getElementById(id);
        const setT = (id, v) => { const e = el(id); if (e) e.textContent = v; };
        const currentGB = ((gs.current_table_mb || 0) / 1024).toFixed(1);
        setT('projCurrent',  `${currentGB} GB table + ${fmt(gs.current_index_mb, 0)} MB index`);
        setT('projDailyRate', `+${fmt(gs.daily_growth_mb, 1)} MB/day`);
        setT('proj30d',  `${fmt(gs.growth_30d_pct, 1)}%`);
        setT('proj90d',  `${fmt(gs.projected_table_mb_90d / 1024, 1)} GB`);
        const g30 = gs.growth_30d_pct || 0;
        const p30 = el('proj30d');
        if (p30) p30.style.color = g30 > 15 ? 'var(--danger)' : (g30 > 5 ? 'var(--warning)' : 'var(--success)');
    }

    function renderHighScanTables(tbodyId, rows, engine) {
        const tbody = document.getElementById(tbodyId);
        if (!tbody) return;
        tbody.innerHTML = (rows || []).map(r => {
            const scans  = Number(r.value)  || 0;
            const seeks  = Number(r.value2) || 0;
            const ratio  = seeks > 0 ? (scans / seeks).toFixed(1) : '∞';
            const risk   = scans > seeks * 20 ? 'danger' : (scans > seeks * 5 ? 'warning' : 'info');
            const riskLbl= risk === 'danger' ? 'CRITICAL' : (risk === 'warning' ? 'HIGH' : 'MEDIUM');
            const tooltip = `Table: ${r.schema_name}.${r.table_name}\nScans: ${scans.toLocaleString()}\nSeeks: ${seeks.toLocaleString()}\nScan/Seek Ratio: ${ratio}\nRisk: ${riskLbl}`;
            return `<tr style="border-left: 3px solid var(--${risk});" title="${escH(tooltip)}">
                <td><strong>${escH(r.schema_name)}.${escH(r.table_name)}</strong></td>
                <td class="text-right">${fmt(scans, 0)}</td>
                <td class="text-right">${fmt(seeks, 0)}</td>
                <td class="text-right">${ratio}</td>
                <td><span class="badge badge-${risk}">${riskLbl}</span></td>
            </tr>`;
        }).join('') || emptyRow(5, 'No high-scan tables in selected window.');
    }

    function renderLargestIndexes(tbodyId, rows) {
        const tbody = document.getElementById(tbodyId);
        if (!tbody) return;
        tbody.innerHTML = (rows || []).map(r => {
            const idxMB   = Number(r.value)  || 0;
            const tblMB   = Number(r.value2) || 0;
            const pctTbl  = tblMB > 0 ? ((idxMB / tblMB) * 100).toFixed(1) : '--';
            const tooltip = `Index: ${r.index_name}\nTable: ${r.schema_name}.${r.table_name}\nSize: ${idxMB.toFixed(1)} MB\nPercentage of Table: ${pctTbl}%`;
            return `<tr title="${escH(tooltip)}">
                <td><strong>${escH(r.index_name)}</strong><br>
                    <span class="text-muted" style="font-size:0.6rem;">${escH(r.schema_name)}.${escH(r.table_name)}</span></td>
                <td class="text-right">${fmt(idxMB, 1)}</td>
                <td class="text-right">${pctTbl}%</td>
            </tr>`;
        }).join('') || emptyRow(3, 'No index data available.');
    }

    function renderLargestTables(tbodyId, rows, engine, drilldownAction) {
        const tbody = document.getElementById(tbodyId);
        if (!tbody) return;
        tbody.innerHTML = (rows || []).map(t => {
            const totalMB = Number(t.value)  || 0;
            const idxMB   = Number(t.value2) || 0;
            const dataMB  = Math.max(0, totalMB - idxMB);
            const idxPct  = totalMB > 0 ? ((idxMB / totalMB) * 100).toFixed(1) : '0.0';
            const tooltip = `Table: ${t.schema_name}.${t.table_name}\nTotal Size: ${totalMB.toFixed(1)} MB\nData: ${dataMB.toFixed(1)} MB\nIndexes: ${idxMB.toFixed(1)} MB (${idxPct}%)`;
            return `<tr style="cursor:pointer;" data-action="${drilldownAction}" title="${escH(tooltip)}"
                        data-db="${escH(t.db_name)}" data-schema="${escH(t.schema_name)}" data-table="${escH(t.table_name)}">
                <td><strong>${escH(t.schema_name)}.${escH(t.table_name)}</strong></td>
                <td class="text-right">${fmt(totalMB, 1)}</td>
                <td class="text-right">${fmt(dataMB, 1)}</td>
                <td class="text-right">${fmt(idxMB, 1)}</td>
                <td class="text-right">${idxPct}%</td>
                <td><button class="btn btn-xs btn-outline" title="View detailed index usage">▶</button></td>
            </tr>`;
        }).join('') || emptyRow(6, 'No table data for selected window.');
    }

    function renderIndexEfficiency(tbodyId, rows, engine) {
        const tbody = document.getElementById(tbodyId);
        if (!tbody) return;
        tbody.innerHTML = (rows || []).map(idx => {
            const sizeMB   = Number(idx.value2) || 0;
            const seeks    = Number(idx.value)  || 0;
            const lastSeen = idx.last_user_seek
                ? new Date(idx.last_user_seek).toLocaleDateString()
                : 'Never';
            let rec = 'HEALTHY', cls = 'success';
            if (seeks === 0)    { rec = 'DROP';            cls = 'danger';  }
            else if (seeks < 5) { rec = 'REVIEW';          cls = 'warning'; }
            const isMs   = engine === 'sqlserver';
            const dropSql = isMs
                ? `DROP INDEX [${idx.index_name}] ON [${idx.schema_name}].[${idx.table_name}];`
                : `DROP INDEX IF EXISTS ${idx.schema_name}.${idx.index_name};`;
            const tooltip = `Index: ${idx.index_name}\nTable: ${idx.schema_name}.${idx.table_name}\nSize: ${sizeMB.toFixed(1)} MB\nSeeks in Window: ${seeks}\nLast User Seek: ${lastSeen}\nRecommendation: ${rec}`;
            return `<tr title="${escH(tooltip)}">
                <td>
                    <span class="text-muted" style="font-size:0.6rem;">${escH(idx.schema_name)}.${escH(idx.table_name)}</span><br>
                    <strong>${escH(idx.index_name)}</strong>
                </td>
                <td class="text-right">${fmt(sizeMB, 1)}</td>
                <td class="text-right">${fmt(seeks, 0)}</td>
                <td class="text-right" style="font-size:0.65rem;">${lastSeen}</td>
                <td><span class="badge badge-${cls}">${rec}</span></td>
                <td>
                    ${rec !== 'HEALTHY' ? `<button class="btn btn-xs btn-outline sih-copy-drop"
                        data-sql="${escH(dropSql)}" title="Copy DROP statement">
                        <i class="fa-solid fa-copy"></i>
                    </button>` : ''}
                </td>
            </tr>`;
        }).join('') || emptyRow(6, 'No unused/inefficient indexes.');
    }

    function renderDuplicateIndexes(bodyId, countId, duplicates, engine) {
        const body  = document.getElementById(bodyId);
        const count = document.getElementById(countId);
        if (count) count.textContent = (duplicates || []).length;
        if (!body) return;
        if (!duplicates || !duplicates.length) {
            body.innerHTML = '<p class="text-success" style="font-size:0.8rem;"><i class="fa-solid fa-check-circle"></i> No duplicate indexes detected.</p>';
            return;
        }
        const byTable = {};
        duplicates.forEach(d => {
            const key = `${d.db_name || ''}.${d.schema_name || ''}.${d.table_name || ''}`;
            (byTable[key] = byTable[key] || []).push(d);
        });
        body.innerHTML = Object.entries(byTable).map(([tbl, pairs]) => `
            <div style="margin-bottom:0.75rem;">
                <div style="font-size:0.75rem; font-weight:700; color:var(--warning); margin-bottom:0.25rem;">
                    <i class="fa-solid fa-triangle-exclamation"></i> ${escH(tbl)}
                </div>
                ${pairs.map(p => {
                    const ia = String(p.index_a || p.index_name || '');
                    const ib = String(p.index_b || p.index_name_2 || '');
                    const ka = String(p.key_columns_a || p.key_columns || '');
                    const sn = String(p.schema_name  || '');
                    const tn = String(p.table_name   || '');
                    const isMs = engine === 'sqlserver';
                    const dropA = isMs ? `DROP INDEX [${ia}] ON [${sn}].[${tn}];` : `DROP INDEX IF EXISTS ${sn}.${ia};`;
                    const tooltip = `Potential Duplicate Indexes:\n- ${ia}\n- ${ib}\nKeys: ${ka}\nThese indexes share the same leading columns and can often be consolidated.`;
                    return `<div style="display:flex; align-items:center; gap:0.5rem; font-size:0.72rem; padding:0.25rem 0.5rem; background:rgba(255,255,255,0.03); border-radius:4px; margin-top:0.2rem; flex-wrap:wrap;" title="${escH(tooltip)}">
                        <span class="badge badge-warning">DUP</span>
                        <span><strong>${escH(ia)}</strong> vs <strong>${escH(ib)}</strong></span>
                        <span class="text-muted">Keys: ${escH(ka)}</span>
                        <button class="btn btn-xs btn-outline sih-copy-drop ml-auto" data-sql="${escH(dropA)}" title="Copy DROP for ${ia}">
                            <i class="fa-solid fa-copy"></i> Copy DROP
                        </button>
                    </div>`;
                }).join('')}
            </div>
        `).join('');
    }

    function renderInsights(bodyId, insights) {
        const el = document.getElementById(bodyId);
        if (!el) return;
        const sorted = [...(insights || [])].sort((a, b) => {
            const rank = { critical: 0, warning: 1, info: 2 };
            return (rank[a.severity] ?? 3) - (rank[b.severity] ?? 3);
        });
        if (!sorted.length) {
            el.innerHTML = '<div class="text-success" style="font-size:0.85rem;"><i class="fa-solid fa-circle-check"></i> No significant risks detected.</div>';
            return;
        }
        const sevCls  = { critical: 'danger', warning: 'warning', info: 'info' };
        const sevBdr  = { critical: 'var(--danger)', warning: 'var(--warning)', info: 'var(--info, #3b82f6)' };
        el.innerHTML = sorted.map(ins => {
            const tooltip = `Severity: ${ins.severity.toUpperCase()}\n${ins.message}`;
            return `
            <div style="display:flex; align-items:center; gap:0.75rem; padding:0.4rem 0.6rem; margin-bottom:0.3rem;
                        background:rgba(255,255,255,0.03); border-radius:5px; border-left:3px solid ${sevBdr[ins.severity] || 'transparent'};" title="${escH(tooltip)}">
                <span class="badge badge-${sevCls[ins.severity] || 'secondary'}" style="min-width:4.5rem; text-align:center; flex-shrink:0;">
                    ${String(ins.severity || 'info').toUpperCase()}
                </span>
                <span style="flex:1; font-size:0.82rem;">${escH(ins.message)}</span>
                ${ins.table_name ? `<button class="btn btn-xs btn-outline sih-insight-view"
                    data-db="${escH(ins.db_name || '')}" data-schema="${escH(ins.schema_name || '')}" data-table="${escH(ins.table_name || '')}" title="View details for ${ins.table_name}">
                    View
                </button>` : ''}
            </div>
        `; }).join('');
    }

    // Wire copy-to-clipboard on any sih-copy-drop buttons within container
    function wireCopyButtons(containerEl) {
        if (!containerEl) return;
        containerEl.querySelectorAll('.sih-copy-drop').forEach(btn => {
            btn.addEventListener('click', e => {
                e.stopPropagation();
                const sql = btn.dataset.sql;
                navigator.clipboard.writeText(sql).then(() => {
                    btn.textContent = '✓ Copied';
                    setTimeout(() => { btn.innerHTML = '<i class="fa-solid fa-copy"></i>'; }, 2000);
                });
            });
        });
    }

    return {
        fmt, escH, emptyRow, buildFilterQS,
        renderSparkline, renderGrowthChart, renderTopGrowthChart,
        renderSeekScanLookupChart,
        buildHealthScore, buildBannerMessages, renderBanner,
        renderProjectionStrip,
        renderHighScanTables, renderLargestIndexes, renderLargestTables,
        renderIndexEfficiency, renderDuplicateIndexes, wireCopyButtons, renderInsights
    };
})();

// For Jest/Node testing environment compatibility
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { sihShared: window.sihShared };
}
