// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Admin tab — SQL Server collector / TimescaleDB diagnostics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

function esc(s) {
    return window.escapeHtml ? window.escapeHtml(String(s ?? '')) : String(s ?? '');
}

function fmtTs(iso) {
    if (!iso) return '—';
    try {
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return '—';
        return d.toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'medium' });
    } catch {
        return '—';
    }
}

function fmtNum(n) {
    if (n == null || n === '') return '—';
    const x = Number(n);
    if (Number.isNaN(x)) return '—';
    return x.toLocaleString();
}

function fmtCompact(n) {
    const x = Number(n);
    if (Number.isNaN(x)) return '—';
    if (x >= 1e9) return (x / 1e9).toFixed(1) + 'B';
    if (x >= 1e6) return (x / 1e6).toFixed(1) + 'M';
    if (x >= 1e3) return (x / 1e3).toFixed(1) + 'K';
    return String(x);
}

function statusBadge(label, tone) {
    const cls = tone === 'ok' ? 'badge-success' : tone === 'warn' ? 'badge-warning' : tone === 'bad' ? 'badge-danger' : 'badge-outline';
    return `<span class="badge ${cls}" style="font-size:0.7rem;">${esc(label)}</span>`;
}

function healthPill(level, label) {
    const cls = level === 'ok' ? 'admin-diag-health--ok' : level === 'warn' ? 'admin-diag-health--warn' : 'admin-diag-health--bad';
    const icon = level === 'ok' ? 'circle-check' : level === 'warn' ? 'circle-exclamation' : 'circle-xmark';
    return `<span class="admin-diag-health ${cls}"><i class="fa-solid fa-${icon}"></i> ${esc(label)}</span>`;
}

function deriveHealth(d) {
    const hints = Array.isArray(d.hints) ? d.hints : [];
    const conn = d.connection_status;
    const hist = (d.hypertables || []).find((t) => t.qualified_name === 'public.sqlserver_query_stats_history');
    const histWindow = hist?.rows_in_window ?? 0;

    if (!d.registry_active || conn === 'offline') return { level: 'bad', label: 'Blocked' };
    if (hints.length >= 2 || (histWindow === 0 && hints.length > 0)) return { level: 'warn', label: 'Degraded' };
    if (histWindow > 0 && hints.length === 0) return { level: 'ok', label: 'Collecting' };
    return { level: 'warn', label: 'Warming up' };
}

function barPct(rowsInWindow, rowsTotal) {
    const w = Number(rowsInWindow) || 0;
    const t = Number(rowsTotal) || 0;
    if (t <= 0) return w > 0 ? 100 : 0;
    return Math.min(100, Math.round((w / t) * 100));
}

const CATEGORY_ORDER = ['query', 'performance', 'memory', 'storage', 'governance', 'live'];
const CATEGORY_LABELS = {
    query: 'Query pipeline',
    performance: 'Performance & waits',
    memory: 'Memory',
    storage: 'Storage & index',
    governance: 'Governance',
    live: 'Live / sessions',
};

/**
 * Open diagnostics tab with a pre-selected instance (from Monitoring servers row).
 */
export function openSqlServerDiagnosticsForInstance(instanceName) {
    if (instanceName) {
        window._adminDiagnosticsPreset = { instance: instanceName };
    }
    if (typeof window.showAdminTab === 'function') {
        window.showAdminTab('diagnostics');
    }
}

export async function loadSqlServerDiagnostics() {
    const container = document.getElementById('admin-content');
    if (!container) return;

    const preset = window._adminDiagnosticsPreset || {};
    delete window._adminDiagnosticsPreset;

    container.innerHTML = `
        <div class="admin-diag-shell">
            <div class="admin-diag-hero">
                <div>
                    <h2><i class="fa-solid fa-stethoscope text-accent"></i> SQL Server collector diagnostics</h2>
                    <p>
                        Inspect Timescale hypertable row counts, on-disk size, and collector watermarks for a monitored instance.
                        Use this when dashboards are blank to tell <em>no workload</em> from <em>collector not writing</em>.
                    </p>
                </div>
            </div>

            <div class="admin-diag-toolbar">
                <label>
                    <span class="text-muted">SQL Server instance</span>
                    <select id="admin-diag-instance" class="form-control" style="min-width:15rem;">
                        <option value="">Loading…</option>
                    </select>
                </label>
                <label>
                    <span class="text-muted">Lookback window</span>
                    <select id="admin-diag-hours" class="form-control" style="width:7rem;">
                        <option value="1">1 hour</option>
                        <option value="6">6 hours</option>
                        <option value="24" selected>24 hours</option>
                        <option value="48">48 hours</option>
                        <option value="168">7 days</option>
                    </select>
                </label>
                <button type="button" class="btn btn-sm btn-accent" id="admin-diag-run">
                    <i class="fa-solid fa-magnifying-glass-chart"></i> Run diagnostics
                </button>
                <button type="button" class="btn btn-sm btn-outline" id="admin-diag-copy" style="display:none;" title="Copy JSON for support">
                    <i class="fa-solid fa-copy"></i> Copy JSON
                </button>
            </div>

            <div id="admin-diag-msg"></div>
            <div id="admin-diag-results"></div>
        </div>
    `;

    const sel = document.getElementById('admin-diag-instance');
    const runBtn = document.getElementById('admin-diag-run');
    const copyBtn = document.getElementById('admin-diag-copy');

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/servers');
        if (!response.ok) throw new Error('Failed to load servers');
        const data = await response.json();
        const sqlServers = (data.servers || []).filter(
            (s) => String(s.db_type || '').toLowerCase() === 'sqlserver'
        );
        if (!sqlServers.length) {
            sel.innerHTML = '<option value="">No SQL Server targets registered</option>';
            sel.disabled = true;
            runBtn.disabled = true;
            return;
        }
        sel.innerHTML = '';
        sqlServers.forEach((s) => {
            const opt = document.createElement('option');
            opt.value = s.name || '';
            opt.textContent = s.is_active ? (s.name || '') : `${s.name || ''} (paused)`;
            sel.appendChild(opt);
        });
        if (preset.instance) {
            const match = sqlServers.find(
                (x) => String(x.name).toUpperCase() === String(preset.instance).toUpperCase()
            );
            if (match) sel.value = match.name;
        }
    } catch (err) {
        sel.innerHTML = '<option value="">Could not load servers</option>';
        document.getElementById('admin-diag-msg').innerHTML =
            `<div class="alert alert-danger">${esc(err.message)}</div>`;
        return;
    }

    runBtn.addEventListener('click', () => runDiagnostics());
    copyBtn.addEventListener('click', () => {
        if (!window._adminDiagnosticsLast) return;
        const text = JSON.stringify(window._adminDiagnosticsLast, null, 2);
        navigator.clipboard?.writeText(text).then(
            () => notifyCopy('Diagnostics JSON copied'),
            () => alert(text)
        );
    });

    if (preset.instance && sel.value) {
        runDiagnostics();
    }
}

function notifyCopy(msg) {
    if (typeof window.showNotification === 'function') {
        window.showNotification(msg, 'success');
    }
}

async function runDiagnostics() {
    const msg = document.getElementById('admin-diag-msg');
    const results = document.getElementById('admin-diag-results');
    const copyBtn = document.getElementById('admin-diag-copy');
    const instance = document.getElementById('admin-diag-instance')?.value;
    const hours = document.getElementById('admin-diag-hours')?.value || '24';

    if (!instance) {
        if (msg) msg.innerHTML = '<div class="alert alert-warning">Select a SQL Server instance.</div>';
        return;
    }

    if (msg) msg.innerHTML = '';
    if (copyBtn) copyBtn.style.display = 'none';
    if (results) {
        results.innerHTML =
            '<div class="admin-diag-empty"><div class="spinner" style="margin:0 auto 1rem;"></div>Running diagnostics…</div>';
    }

    const url = `/api/admin/diagnostics/sqlserver/${encodeURIComponent(instance)}?hours=${encodeURIComponent(hours)}`;

    try {
        const response = await window.apiClient.authenticatedFetch(url);
        const body = await response.json().catch(() => ({}));
        if (!response.ok) {
            throw new Error(body.error || `HTTP ${response.status}`);
        }
        window._adminDiagnosticsLast = body;
        if (copyBtn) copyBtn.style.display = 'inline-flex';
        if (results) results.innerHTML = renderDiagnostics(body);
    } catch (err) {
        window._adminDiagnosticsLast = null;
        if (results) results.innerHTML = '';
        if (msg) {
            msg.innerHTML = `<div class="alert alert-danger"><i class="fa-solid fa-triangle-exclamation"></i> ${esc(err.message)}</div>`;
        }
    }
}

function renderDiagnostics(d) {
    const health = deriveHealth(d);
    const summary = d.summary || {};
    const st = d.collector_state || {};
    const collectors = d.collectors || {};
    const hints = Array.isArray(d.hints) ? d.hints : [];
    const tables = Array.isArray(d.hypertables) ? d.hypertables : [];

    const conn = d.connection_status || 'unknown';
    const connTone = conn === 'online' ? 'ok' : conn === 'offline' ? 'bad' : '';

    const maxWindowRows = Math.max(1, ...tables.map((t) => Number(t.rows_in_window) || 0));

    const byCategory = {};
    tables.forEach((t) => {
        const cat = t.category || 'other';
        if (!byCategory[cat]) byCategory[cat] = [];
        byCategory[cat].push(t);
    });

    const categorySections = CATEGORY_ORDER.filter((c) => byCategory[c]?.length)
        .map((cat) => renderTableSection(CATEGORY_LABELS[cat] || cat, byCategory[cat], maxWindowRows))
        .join('');

    const collectorRows = Object.entries(collectors)
        .map(([name, c]) => `<tr>
            <td><code>${esc(name)}</code></td>
            <td>${c?.enabled ? statusBadge('enabled', 'ok') : statusBadge('disabled', 'bad')}</td>
            <td style="text-align:right;font-variant-numeric:tabular-nums;">${esc(c?.frequency_seconds ?? '—')}s</td>
        </tr>`)
        .join('');

    const hintsHtml = hints.length
        ? `<ul>${hints.map((h) => `<li>${esc(h)}</li>`).join('')}</ul>`
        : '<p class="text-muted" style="margin:0;">No issues detected for the selected window.</p>';

    return `
        <div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:0.75rem;margin-bottom:1rem;">
            <div>
                <div style="font-size:1.05rem;font-weight:600;">${esc(d.server_name)}</div>
                <div class="text-muted" style="font-size:0.72rem;font-family:monospace;margin-top:0.15rem;">${esc(d.server_id)}</div>
            </div>
            ${healthPill(health.level, health.label)}
        </div>

        <div class="admin-diag-kpi-grid">
            <div class="admin-diag-kpi">
                <div class="kpi-label">Connection</div>
                <div class="kpi-value">${statusBadge(conn, connTone)}</div>
            </div>
            <div class="admin-diag-kpi">
                <div class="kpi-label">Registry</div>
                <div class="kpi-value">${statusBadge(d.registry_active ? 'active' : 'inactive', d.registry_active ? 'ok' : 'warn')}</div>
            </div>
            <div class="admin-diag-kpi">
                <div class="kpi-label">Rows in window</div>
                <div class="kpi-value">${fmtNum(summary.total_rows_in_window)}</div>
                <div class="kpi-sub">${summary.tables_with_rows_in_window ?? 0} / ${summary.tables_checked ?? 0} tables</div>
            </div>
            <div class="admin-diag-kpi">
                <div class="kpi-label">Rows (all time)</div>
                <div class="kpi-value">${fmtCompact(summary.total_rows_all_time)}</div>
                <div class="kpi-sub">this instance</div>
            </div>
            <div class="admin-diag-kpi">
                <div class="kpi-label">Hypertable storage</div>
                <div class="kpi-value">${esc(formatStorageSummary(summary.total_storage_bytes))}</div>
                <div class="kpi-sub">catalog tables (shared disk)</div>
            </div>
            <div class="admin-diag-kpi">
                <div class="kpi-label">Query V2</div>
                <div class="kpi-value">${statusBadge(d.query_v2_pipeline_always_on ? 'on' : 'off', d.query_v2_pipeline_always_on ? 'ok' : 'bad')}</div>
            </div>
        </div>

        <div class="admin-diag-split">
            <div class="admin-diag-section">
                <div class="admin-diag-section__head">
                    <h3><i class="fa-solid fa-water text-accent"></i> Query snapshot state</h3>
                </div>
                <dl class="admin-diag-state-dl">
                    <dt>Last poll</dt><dd>${fmtTs(st.last_poll_time_utc)}</dd>
                    <dt>Last success</dt><dd>${fmtTs(st.last_successful_run)}</dd>
                    <dt>SQL Server start</dt><dd>${fmtTs(st.sqlserver_start_time)}</dd>
                    <dt>State row</dt><dd>${st.has_collector_state_row ? statusBadge('present', 'ok') : statusBadge('missing', 'warn')}</dd>
                </dl>
                ${st.last_error ? `<div class="alert alert-danger" style="margin:0 1.1rem 1rem;font-size:0.8rem;"><strong>Last error:</strong> ${esc(st.last_error)}</div>` : ''}
            </div>
            <div class="admin-diag-section">
                <div class="admin-diag-section__head">
                    <h3><i class="fa-solid fa-clock text-accent"></i> Analysis window</h3>
                </div>
                <dl class="admin-diag-state-dl">
                    <dt>From</dt><dd>${fmtTs(d.window?.from)}</dd>
                    <dt>To</dt><dd>${fmtTs(d.window?.to)}</dd>
                    <dt>Hours</dt><dd>${esc(d.window?.hours ?? '')}</dd>
                </dl>
            </div>
        </div>

        <div class="admin-diag-section" style="margin-bottom:1.25rem;">
            <div class="admin-diag-section__head">
                <h3><i class="fa-solid fa-database text-accent"></i> Timescale hypertables &amp; row counts</h3>
                <span class="text-muted" style="font-size:0.75rem;">Per-instance rows · shared table size</span>
            </div>
            ${categorySections || '<div class="admin-diag-empty">No table artifacts returned.</div>'}
        </div>

        <div class="admin-diag-section" style="margin-bottom:1.25rem;">
            <div class="admin-diag-section__head"><h3><i class="fa-solid fa-gears text-accent"></i> Collector jobs</h3></div>
            <div class="admin-diag-section__body">
                <table class="admin-diag-table">
                    <thead><tr>
                        <th>Name</th><th>Status</th><th style="text-align:right;">Interval</th>
                    </tr></thead>
                    <tbody>${collectorRows || '<tr><td colspan="3" class="text-muted" style="padding:1rem;">No config rows.</td></tr>'}</tbody>
                </table>
            </div>
        </div>

        <div class="admin-diag-section">
            <div class="admin-diag-section__head"><h3><i class="fa-solid fa-lightbulb text-accent"></i> Operator hints</h3></div>
            <div class="admin-diag-hints">${hintsHtml}</div>
        </div>
    `;
}

function formatStorageSummary(bytes) {
    const tables = window._adminDiagnosticsLast?.hypertables;
    if (Array.isArray(tables) && tables[0]?.relation_size_pretty) {
        const n = Number(bytes) || 0;
        if (n >= 1024 * 1024 * 1024) return (n / (1024 ** 3)).toFixed(1) + ' GB';
        if (n >= 1024 * 1024) return (n / (1024 ** 2)).toFixed(1) + ' MB';
        if (n >= 1024) return (n / 1024).toFixed(1) + ' KB';
        return n + ' B';
    }
    return fmtNum(bytes);
}

function renderTableSection(title, rows, maxWindowRows) {
    const body = rows
        .sort((a, b) => (Number(b.rows_in_window) || 0) - (Number(a.rows_in_window) || 0))
        .map((t) => renderTableRow(t, maxWindowRows))
        .join('');

    return `
        <div style="padding:0.35rem 0 0.75rem;">
            <div style="padding:0.4rem 1.1rem 0.5rem;font-size:0.78rem;font-weight:600;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.04em;">
                ${esc(title)}
            </div>
            <table class="admin-diag-table">
                <thead><tr>
                    <th>Table</th>
                    <th>Category</th>
                    <th>Latest capture</th>
                    <th style="text-align:right;">Rows (window)</th>
                    <th style="text-align:right;">Rows (total)</th>
                    <th>Window activity</th>
                    <th style="text-align:right;">Disk size</th>
                    <th style="text-align:right;">Chunks</th>
                </tr></thead>
                <tbody>${body}</tbody>
            </table>
        </div>
    `;
}

function renderTableRow(t, maxWindowRows) {
    const windowRows = Number(t.rows_in_window) || 0;
    const totalRows = Number(t.rows_total) || 0;
    const pct = barPct(windowRows, totalRows);
    const relPct = maxWindowRows > 0 ? Math.round((windowRows / maxWindowRows) * 100) : 0;
    const barClass = windowRows === 0 ? 'admin-diag-bar--empty' : 'admin-diag-bar--ok';
    const emptyFlag = windowRows === 0 && (t.qualified_name || '').includes('query_stats_history')
        ? statusBadge('no history', 'warn')
        : windowRows === 0
          ? statusBadge('empty', '')
          : '';

    return `<tr>
        <td>
            <code>${esc(t.qualified_name || t.table)}</code>
            ${t.dashboards ? `<div class="text-muted" style="font-size:0.68rem;margin-top:0.2rem;">${esc(t.dashboards)}</div>` : ''}
        </td>
        <td><span class="admin-diag-cat">${esc(t.category || '')}</span></td>
        <td style="white-space:nowrap;">${fmtTs(t.latest_capture)}</td>
        <td style="text-align:right;font-variant-numeric:tabular-nums;font-weight:600;">${fmtNum(windowRows)} ${emptyFlag}</td>
        <td style="text-align:right;font-variant-numeric:tabular-nums;">${fmtNum(totalRows)}</td>
        <td>
            <div class="admin-diag-bar-wrap">
                <div class="admin-diag-bar ${barClass}" title="${pct}% of instance total in table"><span style="width:${relPct}%"></span></div>
                <div class="text-muted" style="font-size:0.65rem;margin-top:0.2rem;">${pct}% of instance rows</div>
            </div>
        </td>
        <td style="text-align:right;white-space:nowrap;">${esc(t.relation_size_pretty || '—')}</td>
        <td style="text-align:right;font-variant-numeric:tabular-nums;">${t.num_chunks != null ? fmtNum(t.num_chunks) : '—'}</td>
    </tr>`;
}
