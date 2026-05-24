// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: pg_stat_statements dashboard — workload charts, KPIs, filters, breakdown tabs.
//
// Metadata:
//   Type: Frontend Module
//   Page: pg_stat_statements
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

window.pgssOpenQueryDetail = function(sql) {
    if (window.showQueryModal) {
        window.showQueryModal(sql);
    } else {
        alert(sql);
    }
};

window.pgssApplyTimeRange = function () {
    loadPgssAll();
};

// ---- shared state ----
window._pgssState = {
    sort:           'total_time',
    page:           0,
    pageSize:       25,
    search:         '',
    dbName:         '',
    userName:       '',
    appName:        '',
    qtype:          '',
    hideSystem:     false,
    clientSortCol:  null,   // null = use server sort
    clientSortDir:  'desc',
    allRows:        [],     // cached full result for client-side search/pagination
};
window._pgssRefreshTimer = null;

// ============================================================
// Entry point
// ============================================================
window.PgStatStatementsView = async function () {
    const inst = (window.appState.config.instances || [])[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'postgres') {
        window.routerOutlet.innerHTML = '<div class="p-4 text-warning">Please select a PostgreSQL instance first.</div>';
        return;
    }

    if (!window.appState.pgssFrom) {
        const now = new Date();
        window.appState.pgssFrom = toLocalISOString(new Date(now.getTime() - 3600000));
        window.appState.pgssTo   = toLocalISOString(now);
    }

    window.routerOutlet.innerHTML = await window.loadTemplate('pages/pg_stat_statements.html');

    const fromEl = document.getElementById('pgss-from');
    const toEl   = document.getElementById('pgss-to');
    if (fromEl) fromEl.value = window.appState.pgssFrom;
    if (toEl)   toEl.value   = window.appState.pgssTo;

    const label = document.getElementById('pgss-instance-label');
    if (label) label.textContent = `(${inst.name})`;

    window.currentCharts = window.currentCharts || {};

    await checkPgssStatus(inst.name);
    pgssBindEvents();

    // Load filter dropdowns, then all data
    await loadPgssFilters();
    await loadPgssAll();

    if (window._pgssRefreshTimer) clearInterval(window._pgssRefreshTimer);
    window._pgssRefreshTimer = window.registerInterval(() => {
        if (window.location.hash.includes('pg-stat-statements')) loadPgssAll();
    }, 60000);
};

// ============================================================
// Shared filter apply logic (used by both header and filter bar)
// ============================================================
function applyPgssFilters() {
    window._pgssState.dbName   = document.getElementById('pgss-filter-db')?.value   || '';
    window._pgssState.userName = document.getElementById('pgss-filter-user')?.value  || '';
    window._pgssState.appName  = document.getElementById('pgss-filter-app')?.value   || '';
    window._pgssState.qtype    = document.getElementById('pgss-filter-qtype')?.value || '';
    window._pgssState.page     = 0;
    loadPgssAll();
}

// Populate DB/User/App dropdowns from already-loaded row data when the
// API returns nothing (no snapshot data collected yet).
function refreshFilterDropdownsFromRows() {
    const rows = window._pgssState.allRows || [];
    if (!rows.length) return;
    const fill = (id, vals, placeholder) => {
        const el = document.getElementById(id);
        if (!el || el.options.length > 1) return; // already populated by API
        populateSelect(id, [...new Set(vals)].filter(Boolean).sort(), placeholder);
    };
    fill('pgss-filter-db',   rows.map(r => r.db_name),  'All databases');
    fill('pgss-filter-user', rows.map(r => r.username),  'All users');
    fill('pgss-filter-app',  rows.map(r => r.app_name),  'All apps');
}

// ============================================================
// Event Binding
// ============================================================
function pgssBindEvents() {
    // Row click → show query detail modal
    const topTbody = document.getElementById('pgss-top-tbody');
    if (topTbody) {
        topTbody.addEventListener('click', e => {
            const row = e.target.closest('tr[data-query]');
            if (row) window.pgssOpenQueryDetail(row.getAttribute('data-query'));
        });
    }

    // Sortable column headers
    document.querySelectorAll('#pgss-top-table .pgss-sortable-th').forEach(th => {
        th.addEventListener('click', () => {
            const col = th.dataset.sortCol;
            const s = window._pgssState;
            if (s.clientSortCol === col) {
                s.clientSortDir = s.clientSortDir === 'desc' ? 'asc' : 'desc';
            } else {
                s.clientSortCol = col;
                s.clientSortDir = 'desc';
            }
            s.page = 0;
            renderTopQueriesPage();
            // Update header icons
            document.querySelectorAll('#pgss-top-table .pgss-sortable-th').forEach(h => {
                const icon = h.querySelector('i');
                if (!icon) return;
                if (h.dataset.sortCol === col) {
                    icon.className = s.clientSortDir === 'desc'
                        ? 'fa-solid fa-sort-down'
                        : 'fa-solid fa-sort-up';
                    h.style.color = 'var(--accent)';
                } else {
                    icon.className = 'fa-solid fa-sort';
                    h.style.color = '';
                }
            });
        });
    });

    // Hide system queries toggle
    document.getElementById('pgss-hide-system')?.addEventListener('change', e => {
        window._pgssState.hideSystem = e.target.checked;
        window._pgssState.page = 0;
        renderTopQueriesPage();
    });

    // Filter apply/reset
    document.getElementById('pgss-filter-apply')?.addEventListener('click', applyPgssFilters);
    document.getElementById('pgss-header-apply')?.addEventListener('click', applyPgssFilters);

    document.getElementById('pgss-filter-reset')?.addEventListener('click', () => {
        ['pgss-filter-db','pgss-filter-user','pgss-filter-app','pgss-filter-qtype'].forEach(id => {
            const el = document.getElementById(id);
            if (el) el.value = '';
        });
        const sysEl = document.getElementById('pgss-hide-system');
        if (sysEl) sysEl.checked = false;
        window._pgssState.dbName = window._pgssState.userName =
        window._pgssState.appName = window._pgssState.qtype = '';
        window._pgssState.hideSystem = false;
        window._pgssState.page = 0;
        loadPgssAll();
    });

    // Inline search (client-side, no re-fetch)
    const searchEl = document.getElementById('pgss-inline-search');
    if (searchEl) {
        searchEl.addEventListener('input', () => {
            window._pgssState.search = searchEl.value.toLowerCase();
            window._pgssState.page = 0;
            renderTopQueriesPage();
        });
    }

    // Tab switching
    document.querySelectorAll('[data-pgss-tab]').forEach(btn => {
        btn.addEventListener('click', () => {
            const tab = btn.getAttribute('data-pgss-tab');
            document.querySelectorAll('[data-pgss-tab]').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            document.querySelectorAll('.pgss-tab-panel').forEach(p => p.classList.remove('active'));
            const panel = document.getElementById(`pgss-panel-${tab}`);
            if (panel) panel.classList.add('active');

            if (tab === 'by-database') loadByDatabase();
            else if (tab === 'by-user') loadByUser();
        });
    });
}

// ============================================================
// Filter Options
// ============================================================
async function loadPgssFilters() {
    const inst = currentInst();
    const { from, to } = getTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/filters?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) return;
        const data = await resp.json();
        populateSelect('pgss-filter-db',   data.databases || [], 'All databases');
        populateSelect('pgss-filter-user', data.users     || [], 'All users');
        populateSelect('pgss-filter-app',  data.app_names || [], 'All apps');
    } catch (e) {
        console.error('[PGSS] Failed to load filter options', e);
    }
}

function populateSelect(id, items, placeholder) {
    const el = document.getElementById(id);
    if (!el) return;
    const current = el.value;
    el.innerHTML = `<option value="">${escapeHtml(placeholder)}</option>` +
        items.map(v => `<option value="${escapeHtml(v)}"${v === current ? ' selected' : ''}>${escapeHtml(v)}</option>`).join('');
}

// ============================================================
// Load All
// ============================================================
async function loadPgssAll() {
    await Promise.all([
        loadPgssWorkload(),
        loadPgssLatency(),
        loadPgssTopQueries(),
        loadPgssRegressions(),
        loadPgssSummaryKPIs(),
    ]);
}

// ============================================================
// KPI Strip
// ============================================================
async function loadPgssSummaryKPIs() {
    const inst = currentInst();
    const { from, to } = getTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/summary?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) return;
        const d = await resp.json();

        setKpi('pgss-kpi-qps-val',   fmtNum2(d.avg_qps ?? d.qps), d.avg_qps > 500 ? 'kpi-warn' : '');
        setKpi('pgss-kpi-p99-val',   fmtMs(d.p99_ms), d.p99_ms > 500 ? 'kpi-critical' : d.p99_ms > 100 ? 'kpi-warn' : 'kpi-good');
        setKpi('pgss-kpi-cache-val', pct(d.cache_hit_pct), d.cache_hit_pct < 90 ? 'kpi-warn' : 'kpi-good');
        setKpi('pgss-kpi-load-val',  fmtMs(d.total_exec_ms_sec ?? d.query_load_ms_sec), '');
        setKpi('pgss-kpi-wal-val',   fmtNum2(d.wal_mb_sec ?? 0), '');
        setKpi('pgss-kpi-temp-val',  fmtNum2(d.temp_mb_sec ?? 0), d.temp_mb_sec > 10 ? 'kpi-warn' : '');
        setKpi('pgss-kpi-uq-val',    fmtNum(d.unique_query_count), '');
    } catch (e) {
        console.error('[PGSS] Failed to load summary KPIs', e);
    }
}

function setKpi(id, value, cls) {
    const el = document.getElementById(id);
    if (!el) return;
    el.textContent = value;
    const tile = el.closest('.glass-panel');
    if (tile) {
        tile.classList.remove('kpi-warn', 'kpi-critical', 'kpi-good');
        if (cls) tile.classList.add(cls);
    }
}

// ============================================================
// Workload Charts
// ============================================================
async function loadPgssWorkload() {
    const inst = currentInst();
    const { from, to } = getTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/workload?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) return;
        const data = await resp.json();
        const pts = data.points || [];
        const labels = pts.map(p => new Date(p.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));

        renderLineChart('pgss-chart-load', labels, [
            { label: 'Query Load (ms/s)', data: pts.map(p => p.query_load_ms_sec), borderColor: '#38bdf8', backgroundColor: 'rgba(56,189,248,0.1)', fill: true }
        ]);
        renderLineChart('pgss-chart-qps', labels, [
            { label: 'QPS',    data: pts.map(p => p.qps),      borderColor: '#10b981', yAxisID: 'y' },
            { label: 'Rows/s', data: pts.map(p => p.rows_sec), borderColor: '#f59e0b', yAxisID: 'y1' }
        ], true);
        renderLineChart('pgss-chart-cache', labels, [
            { label: 'Cache Hit %', data: pts.map(p => p.cache_hit_ratio), borderColor: '#8b5cf6', tension: 0.4 }
        ]);
        renderLineChart('pgss-chart-wal', labels, [
            { label: 'WAL MB/s', data: pts.map(p => p.wal_bytes_sec / (1024*1024)), borderColor: '#f43f5e' }
        ]);
        renderStackedArea('pgss-chart-execplan', labels, [
            { label: 'Exec %', data: pts.map(p => p.exec_pct), backgroundColor: 'rgba(16,185,129,0.4)', borderColor: '#10b981' },
            { label: 'Plan %', data: pts.map(p => p.plan_pct), backgroundColor: 'rgba(56,189,248,0.4)', borderColor: '#38bdf8' }
        ]);
    } catch (e) { console.error('PgssWorkload failed', e); }
}

// ============================================================
// Latency Chart
// ============================================================
async function loadPgssLatency() {
    const inst = currentInst();
    const { from, to } = getTimeRange();
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/latency?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) return;
        const data = await resp.json();
        const pts = data.points || [];
        const labels = pts.map(p => new Date(p.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
        renderLineChart('pgss-chart-latency', labels, [
            { label: 'P99', data: pts.map(p => p.p99), borderColor: '#ef4444' },
            { label: 'P95', data: pts.map(p => p.p95), borderColor: '#f59e0b' },
            { label: 'P50', data: pts.map(p => p.p50), borderColor: '#10b981' }
        ]);
    } catch (e) { console.error('PgssLatency failed', e); }
}

// ============================================================
// Top Queries (server-side filters, client-side search+pagination)
// ============================================================
async function loadPgssTopQueries() {
    const inst = currentInst();
    const { from, to } = getTimeRange();
    const s = window._pgssState;
    const tbody = document.getElementById('pgss-top-tbody');
    if (!tbody) return;

    tbody.innerHTML = '<tr><td colspan="14" class="text-center p-4"><div class="spinner"></div></td></tr>';

    const params = new URLSearchParams({
        instance: inst.name, from, to,
        sort:        s.sort,
        limit:       '200',
        hide_system: s.hideSystem ? 'true' : 'false',
    });
    if (s.dbName)   params.set('db_name',    s.dbName);
    if (s.userName) params.set('username',   s.userName);
    if (s.appName)  params.set('app_name',   s.appName);
    if (s.qtype)    params.set('query_type', s.qtype);

    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/pgss/top?${params}`);
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="14">Error loading data</td></tr>'; return; }
        const data = await resp.json();
        window._pgssState.allRows = data.queries || [];
        window._pgssState.page    = 0;
        refreshFilterDropdownsFromRows();
        renderTopQueriesPage();
    } catch (e) {
        console.error('[PGSS] Failed to load top queries', e);
        tbody.innerHTML = '<tr><td colspan="14">Error</td></tr>';
    }
}

const SYSTEM_QUERY_RE = /^(VACUUM\b|ANALYZE\b|autovacuum:|CHECKPOINT\b|SELECT pg_sleep\b|SET\s|RESET\s|DEALLOCATE\s|DISCARD\s|UNLISTEN\b|LISTEN\s|SHOW\s|BEGIN\b|COMMIT\b|ROLLBACK\b|SAVEPOINT\b|RELEASE\sSAVEPOINT\b|SELECT\s+1\b|SELECT\s+pg_is_in_recovery\(\)|SELECT\s+pg_catalog\.|SELECT\s+.*\bFROM\s+pg_catalog\.|SELECT\s+.*\bFROM\s+information_schema\.|DECLARE\b|FETCH\b|MOVE\b|CLOSE\b)/i;

function isSystemQuery(q) {
    const text = (q.query || '').trim();
    if (!text || text === '<insufficient privilege>') return true;
    if (q.username === 'postgres' && (text.includes('pg_stat_') || text.includes('pg_catalog.'))) return true;
    return SYSTEM_QUERY_RE.test(text);
}

function renderTopQueriesPage() {
    const s      = window._pgssState;
    const search = s.search.trim();
    const tbody  = document.getElementById('pgss-top-tbody');
    if (!tbody) return;

    const inst = currentInst();
    const monitoringUser = (inst.user || 'dbsqlmonitor').toLowerCase();

    let rows = s.allRows;

    // Permanent always-on filters
    rows = rows.filter(q => {
        const text = (q.query || '').trim();
        if (!text || text === '<insufficient privilege>') return false;
        if (text.includes('/* SQL_OPTIMA')) return false;
        if ((q.username || '').toLowerCase() === 'postgres') return false;
        if ((q.db_name  || '').toLowerCase() === 'postgres') return false;
        if ((q.username || '').toLowerCase() === monitoringUser) return false;
        return true;
    });

    // Optional system query filter (hide-system toggle)
    if (s.hideSystem) rows = rows.filter(q => !isSystemQuery(q));

    // Inline search filter
    if (search) rows = rows.filter(q => (q.query || '').toLowerCase().includes(search));

    // Client-side sort (overrides server sort for already-fetched rows)
    if (s.clientSortCol) {
        const col = s.clientSortCol;
        const dir = s.clientSortDir === 'asc' ? 1 : -1;
        rows = [...rows].sort((a, b) => ((a[col] ?? 0) - (b[col] ?? 0)) * dir);
    }

    const total = rows.length;
    const start = s.page * s.pageSize;
    const page  = rows.slice(start, start + s.pageSize);

    const rowCount = document.getElementById('pgss-row-count');
    if (rowCount) rowCount.textContent = total ? `Showing ${start + 1}–${Math.min(start + page.length, total)} of ${total}` : '';

    if (page.length === 0) {
        tbody.innerHTML = '<tr><td colspan="14" class="text-center text-muted p-4">No data for selected time range</td></tr>';
        renderPagination(0, 0);
        return;
    }

    tbody.innerHTML = page.map((q, i) => {
        const capturedTs  = fmtCapturedAt(q.captured_at);
        const qtBadge = `<span class="qtype-badge qtype-${escapeHtml(q.query_type || 'O')}">${qtLabel(q.query_type)}</span>`;

        return `<tr data-query="${escapeHtml(q.query)}" title="Click to view full SQL">
            <td class="c-rownum">${start + i + 1}</td>
            <td class="c-captured-ts" title="${escapeHtml(q.captured_at || '')}">${capturedTs}</td>
            <td class="ctr">${qtBadge}</td>
            <td class="c-query">${escapeHtml(truncate(q.query, 160))}</td>
            <td class="c-dim">${escapeHtml(q.db_name   || '—')}</td>
            <td class="c-dim">${escapeHtml(q.username  || '—')}</td>
            <td class="num">${fmtMs(q.total_time_ms)}</td>
            <td class="num">${q.pct_db_time?.toFixed(1) ?? '—'}%</td>
            <td class="num">${fmtNum(q.calls)}</td>
            <td class="num c-hi">${fmtMs(q.avg_ms)}</td>
            <td class="num">${q.rows_per_call?.toFixed(1) ?? '—'}</td>
            <td class="num">${q.hit_pct?.toFixed(1) ?? '—'}%</td>
            <td class="num">${q.temp_mb?.toFixed(2) ?? '0'}</td>
            <td class="num">${q.wal_mb?.toFixed(2) ?? '0'}</td>
        </tr>`;
    }).join('');

    renderPagination(total, s.pageSize);
}

function renderPagination(total, pageSize) {
    const s = window._pgssState;
    const container = document.getElementById('pgss-pagination');
    if (!container) return;
    const pages = Math.ceil(total / pageSize);
    if (pages <= 1) { container.innerHTML = ''; return; }

    const cur = s.page;
    const isFirst = cur === 0;
    const isLast  = cur === pages - 1;

    // Build page window: show up to 5 page numbers centred on current page
    const windowSize = 5;
    let winStart = Math.max(0, cur - Math.floor(windowSize / 2));
    const winEnd = Math.min(pages, winStart + windowSize);
    winStart = Math.max(0, winEnd - windowSize);

    const pageNums = Array.from({ length: winEnd - winStart }, (_, i) => winStart + i)
        .map(i => `<button class="pgss-pg-btn${i === cur ? ' active' : ''}" data-action="pgss-page" data-page="${i}" aria-label="Page ${i + 1}">${i + 1}</button>`)
        .join('');

    container.innerHTML = `
        <button class="pgss-pg-nav" data-action="pgss-page" data-page="0" ${isFirst ? 'disabled' : ''} title="First page" aria-label="First page">
            <i class="fa-solid fa-angles-left"></i>
        </button>
        <button class="pgss-pg-nav" data-action="pgss-page" data-page="${cur - 1}" ${isFirst ? 'disabled' : ''} title="Previous page" aria-label="Previous page">
            <i class="fa-solid fa-angle-left"></i>
        </button>
        <span class="pgss-pg-nums">${pageNums}</span>
        <button class="pgss-pg-nav" data-action="pgss-page" data-page="${cur + 1}" ${isLast ? 'disabled' : ''} title="Next page" aria-label="Next page">
            <i class="fa-solid fa-angle-right"></i>
        </button>
        <button class="pgss-pg-nav" data-action="pgss-page" data-page="${pages - 1}" ${isLast ? 'disabled' : ''} title="Last page" aria-label="Last page">
            <i class="fa-solid fa-angles-right"></i>
        </button>`;

    if (!container.dataset.pgssBound) {
        container.dataset.pgssBound = '1';
        container.addEventListener('click', (e) => {
            const btn = e.target?.closest?.('[data-action="pgss-page"]');
            if (btn && !btn.disabled) {
                const p = Number(btn.dataset.page);
                if (!isNaN(p) && p >= 0 && p < Math.ceil(window._pgssState.allRows.length / window._pgssState.pageSize)) {
                    window._pgssGoPage(p);
                }
            }
        });
    }
}

window._pgssGoPage = function(page) {
    window._pgssState.page = page;
    renderTopQueriesPage();
};

// ============================================================
// By Database Breakdown
// ============================================================
async function loadByDatabase() {
    const inst  = currentInst();
    const { from, to } = getTimeRange();
    const tbody = document.getElementById('pgss-db-tbody');
    if (!tbody) return;
    tbody.innerHTML = '<tr><td colspan="7" class="text-center p-4"><div class="spinner"></div></td></tr>';

    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/by-database?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="7">Error loading</td></tr>'; return; }
        const data = await resp.json();
        const rows = data.rows || [];
        if (!rows.length) {
            tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted p-3">No data</td></tr>';
            return;
        }
        tbody.innerHTML = rows.map(r => {
            const barW = Math.round(Math.max(2, Math.min(80, (r.pct_of_server || 0) * 0.8)));
            return `<tr>
                <td class="c-name">${escapeHtml(r.db_name)}</td>
                <td class="c-load">
                  <span class="load-bar" style="width:${barW}px;"></span>${r.pct_of_server?.toFixed(1) ?? '0'}%
                </td>
                <td class="num">${fmtMs(r.total_exec_ms)}</td>
                <td class="num">${fmtNum(r.total_calls)}</td>
                <td class="num c-hi">${fmtMs(r.avg_ms)}</td>
                <td class="num">${r.cache_hit_pct?.toFixed(1) ?? '—'}%</td>
                <td class="num">${fmtNum(r.unique_query_ids)}</td>
            </tr>`;
        }).join('');
    } catch (e) { tbody.innerHTML = '<tr><td colspan="7">Error</td></tr>'; }
}

// ============================================================
// By User Breakdown
// ============================================================
async function loadByUser() {
    const inst  = currentInst();
    const { from, to } = getTimeRange();
    const tbody = document.getElementById('pgss-user-tbody');
    if (!tbody) return;
    tbody.innerHTML = '<tr><td colspan="6" class="text-center p-4"><div class="spinner"></div></td></tr>';

    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/by-user?instance=${encodeURIComponent(inst.name)}&from=${from}&to=${to}`
        );
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="6">Error loading</td></tr>'; return; }
        const data = await resp.json();
        const rows = data.rows || [];
        if (!rows.length) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted p-3">No data</td></tr>';
            return;
        }
        tbody.innerHTML = rows.map(r => {
            const barW = Math.round(Math.max(2, Math.min(80, (r.pct_of_server || 0) * 0.8)));
            return `<tr>
                <td class="c-name">${escapeHtml(r.username)}</td>
                <td class="c-load">
                  <span class="load-bar" style="width:${barW}px;"></span>${r.pct_of_server?.toFixed(1) ?? '0'}%
                </td>
                <td class="num">${fmtMs(r.total_exec_ms)}</td>
                <td class="num">${fmtNum(r.total_calls)}</td>
                <td class="num c-hi">${fmtMs(r.avg_ms)}</td>
                <td class="num">${fmtNum(r.unique_query_ids)}</td>
            </tr>`;
        }).join('');
    } catch (e) { tbody.innerHTML = '<tr><td colspan="6">Error</td></tr>'; }
}

// ============================================================
// Regressions
// ============================================================
async function loadPgssRegressions() {
    const inst  = currentInst();
    const tbody = document.getElementById('pgss-regression-tbody');
    if (!tbody) return;
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/regressions?instance=${encodeURIComponent(inst.name)}`
        );
        if (!resp.ok) { tbody.innerHTML = '<tr><td colspan="6">Error loading</td></tr>'; return; }
        const data = await resp.json();
        const regs = data.regressions || [];
        if (!regs.length) {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted p-3">No regressions detected</td></tr>';
            return;
        }
        tbody.innerHTML = regs.map(r => `
            <tr>
                <td title="${escapeHtml(r.query)}" style="cursor:pointer;text-decoration:underline;"
                    data-action="view-query" data-query="${escapeHtml(r.query)}">${escapeHtml(truncate(r.query, 80))}</td>
                <td>${fmtMs(r.prev_avg_ms)}</td>
                <td>${fmtMs(r.curr_avg_ms)}</td>
                <td class="${r.change_pct > 100 ? 'text-danger' : 'text-warning'}">+${r.change_pct?.toFixed(0)}%</td>
                <td><span class="badge ${r.status === 'Degraded' ? 'badge-danger' : 'badge-warning'}">${r.status}</span></td>
                <td>${r.detected_at ? new Date(r.detected_at).toLocaleTimeString() : '—'}</td>
            </tr>`).join('');
    } catch (_) { tbody.innerHTML = '<tr><td colspan="6">Error</td></tr>'; }
}

// ============================================================
// Status Check
// ============================================================
async function checkPgssStatus(instance) {
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/pgss/status?instance=${encodeURIComponent(instance)}`
        );
        if (!resp.ok) return;
        const status = await resp.json();
        const banner = document.getElementById('pgss-status-banner');
        const msg    = document.getElementById('pgss-status-msg');
        if (!banner) return;

        if (!status.enabled) {
            banner.style.display = 'inline-flex';
            banner.className = 'pgss-title-status pgss-title-status-warn';
            if (msg) msg.textContent = status.message || 'pg_stat_statements is not enabled — add it to shared_preload_libraries and restart PostgreSQL.';
        } else if (!status.has_data) {
            banner.style.display = 'inline-flex';
            banner.className = 'pgss-title-status pgss-title-status-info';
            if (msg) msg.textContent = status.message || 'pg_stat_statements is enabled. Query data collection is in progress — charts will appear within 2–3 minutes.';
        } else {
            banner.style.display = 'none';
        }
    } catch (_) { /* non-fatal */ }
}

// ============================================================
// Chart Helpers
// ============================================================
function renderLineChart(canvasId, labels, datasets, dualAxis) {
    destroyChart(canvasId);
    const ctx = document.getElementById(canvasId);
    if (!ctx) return;
    const cfg = {
        type: 'line',
        data: { labels, datasets: datasets.map(ds => ({ tension: 0.3, pointRadius: 0, borderWidth: 1.5, fill: ds.fill || false, ...ds })) },
        options: {
            responsive: true, maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            plugins: { legend: { labels: { color: '#94a3b8', font: { size: 11 } } } },
            scales: {
                x: { ticks: { color: '#64748b', maxTicksLimit: 12 }, grid: { color: 'rgba(100,116,139,0.15)' } },
                y: { ticks: { color: '#64748b' }, grid: { color: 'rgba(100,116,139,0.15)' } }
            }
        }
    };
    if (dualAxis) {
        cfg.options.scales.y1 = { position: 'right', ticks: { color: '#64748b' }, grid: { drawOnChartArea: false } };
    }
    window.currentCharts[canvasId] = new Chart(ctx, cfg);
}

function renderStackedArea(canvasId, labels, datasets) {
    destroyChart(canvasId);
    const ctx = document.getElementById(canvasId);
    if (!ctx) return;
    window.currentCharts[canvasId] = new Chart(ctx, {
        type: 'line',
        data: { labels, datasets: datasets.map(ds => ({ fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1, ...ds })) },
        options: {
            responsive: true, maintainAspectRatio: false,
            plugins: { legend: { labels: { color: '#94a3b8', font: { size: 11 } } } },
            scales: {
                x: { stacked: true, ticks: { color: '#64748b', maxTicksLimit: 12 }, grid: { color: 'rgba(100,116,139,0.15)' } },
                y: { stacked: true, max: 100, ticks: { color: '#64748b' }, grid: { color: 'rgba(100,116,139,0.15)' } }
            }
        }
    });
}

function destroyChart(id) {
    if (window.currentCharts?.[id]) {
        window.currentCharts[id].destroy();
        delete window.currentCharts[id];
    }
}

// ============================================================
// Utilities
// ============================================================
function currentInst() {
    return (window.appState.config.instances || [])[window.appState.currentInstanceIdx] || { name: '' };
}

function getTimeRange() {
    const fromEl = document.getElementById('pgss-from');
    const toEl   = document.getElementById('pgss-to');
    if (fromEl?.value) window.appState.pgssFrom = fromEl.value;
    if (toEl?.value)   window.appState.pgssTo   = toEl.value;
    const from = fromEl?.value ? new Date(fromEl.value).toISOString() : new Date(Date.now() - 3600000).toISOString();
    const to   = toEl?.value   ? new Date(toEl.value).toISOString()   : new Date().toISOString();
    return { from, to };
}

function toLocalISOString(date) {
    const pad = n => (n < 10 ? '0' : '') + n;
    return `${date.getFullYear()}-${pad(date.getMonth()+1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function qtLabel(code) {
    const map = { S: 'SELECT', I: 'INSERT', U: 'UPDATE', D: 'DELETE', E: 'DDL', O: 'OTHER' };
    return map[code] || code || 'OTHER';
}

function fmtCapturedAt(ts) {
    if (!ts) return '—';
    const d = new Date(ts);
    if (isNaN(d)) return '—';
    const mo  = d.toLocaleString('default', { month: 'short' });
    const day = String(d.getDate()).padStart(2, '0');
    const hh  = String(d.getHours()).padStart(2, '0');
    const mm  = String(d.getMinutes()).padStart(2, '0');
    return `${mo} ${day} ${hh}:${mm}`;
}

function escapeHtml(s) {
    if (!s) return '';
    return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}
function truncate(s, n) {
    if (!s) return '';
    return s.length > n ? s.slice(0, n) + '…' : s;
}
function fmtMs(ms) {
    if (ms == null || ms === '') return '—';
    if (ms >= 1000) return (ms / 1000).toFixed(2) + ' s';
    return ms.toFixed(2) + ' ms';
}
function fmtNum(n) {
    if (n == null) return '—';
    return Number(n).toLocaleString();
}
function fmtNum2(n) {
    if (n == null) return '—';
    return Number(n).toFixed(2);
}
function pct(n) {
    if (n == null) return '—';
    return Number(n).toFixed(1) + '%';
}
