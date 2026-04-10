(function () {
    function sevKey(sev) { return String(sev || 'INFO').toUpperCase(); }
    function bySeverityRank(sev) {
        const s = (sev || '').toUpperCase();
        if (s === 'CRITICAL') return 0;
        if (s === 'WARNING') return 1;
        return 2;
    }

    function safeJson(obj) {
        try { return JSON.stringify(obj, null, 2); } catch (e) { return ''; }
    }

    function ensurePerfDebtModal() {
        if (document.getElementById('perfDebtModalOverlay')) return;
        const el = document.createElement('div');
        el.id = 'perfDebtModalOverlay';
        el.className = 'modal-overlay';
        el.style.display = 'none';
        el.innerHTML = `
            <div class="modal-content" style="max-width:900px; width:92%; max-height:86vh; overflow:auto;">
                <div class="modal-header">
                    <i class="fa-solid fa-circle-info text-accent"></i>
                    <h3 id="perfDebtModalTitle">Finding</h3>
                    <button class="btn btn-icon" id="perfDebtModalClose" style="margin-left:auto;"><i class="fa-solid fa-xmark"></i></button>
                </div>
                <div class="modal-body" style="padding:1rem 1.25rem;">
                    <div id="perfDebtModalMeta" class="text-muted" style="font-size:0.85rem;"></div>
                    <div style="display:flex; gap:0.5rem; margin-top:0.75rem; flex-wrap:wrap;">
                        <button class="btn btn-sm btn-outline text-accent" id="perfDebtCopyFixBtn" style="display:none;"><i class="fa-regular fa-copy"></i> Copy fix script</button>
                    </div>
                    <div style="margin-top:0.9rem;">
                        <div class="text-muted" style="font-size:0.8rem; margin-bottom:0.25rem;">Recommendation</div>
                        <div id="perfDebtModalRec" style="white-space:pre-wrap;"></div>
                    </div>
                    <div style="margin-top:0.9rem;">
                        <div class="text-muted" style="font-size:0.8rem; margin-bottom:0.25rem;">Details</div>
                        <pre id="perfDebtModalDetails" style="white-space:pre-wrap; margin:0; font-size:0.8rem;"></pre>
                    </div>
                    <div id="perfDebtFixBlock" style="margin-top:0.9rem; display:none;">
                        <div class="text-muted" style="font-size:0.8rem; margin-bottom:0.25rem;">Fix script</div>
                        <pre id="perfDebtModalFix" style="white-space:pre-wrap; margin:0; font-size:0.8rem;"></pre>
                    </div>
                </div>
            </div>
        `;
        document.body.appendChild(el);

        const close = () => { el.style.display = 'none'; };
        el.addEventListener('click', (e) => { if (e.target === el) close(); });
        el.querySelector('#perfDebtModalClose').addEventListener('click', close);
        document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && el.style.display !== 'none') close(); });
    }

    function openPerfDebtFindingModal(finding) {
        ensurePerfDebtModal();
        const overlay = document.getElementById('perfDebtModalOverlay');
        const titleEl = document.getElementById('perfDebtModalTitle');
        const metaEl = document.getElementById('perfDebtModalMeta');
        const recEl = document.getElementById('perfDebtModalRec');
        const detailsEl = document.getElementById('perfDebtModalDetails');
        const fixBlock = document.getElementById('perfDebtFixBlock');
        const fixEl = document.getElementById('perfDebtModalFix');
        const copyBtn = document.getElementById('perfDebtCopyFixBtn');

        const ts = finding?.capture_timestamp ? new Date(finding.capture_timestamp).toLocaleString() : '--';
        const sev = sevKey(finding?.severity);
        const obj = finding?.object_name || '';
        const key = finding?.finding_key || '';

        titleEl.textContent = finding?.title || 'Finding';
        metaEl.textContent = `Severity: ${sev} • Captured: ${ts}${obj ? ` • Object: ${obj}` : ''}${key ? ` • Key: ${key}` : ''}`;
        recEl.textContent = finding?.recommendation || '';
        const details = (typeof finding?.details === 'object') ? safeJson(finding.details) : String(finding?.details || '');
        detailsEl.textContent = details;

        const fix = String(finding?.fix_script || '').trim();
        if (fix) {
            fixBlock.style.display = 'block';
            fixEl.textContent = fix;
            copyBtn.style.display = 'inline-block';
            copyBtn.onclick = async () => {
                try {
                    await navigator.clipboard.writeText(fix);
                    copyBtn.innerHTML = '<i class="fa-solid fa-check"></i> Copied';
                    setTimeout(() => { copyBtn.innerHTML = '<i class="fa-regular fa-copy"></i> Copy fix script'; }, 1200);
                } catch (e) {
                    appDebug('copy fix failed', e);
                }
            };
        } else {
            fixBlock.style.display = 'none';
            fixEl.textContent = '';
            copyBtn.style.display = 'none';
            copyBtn.onclick = null;
        }

        overlay.style.display = 'flex';
    }

    function groupBySection(findings) {
        const out = {};
        (findings || []).forEach(f => {
            const sec = f.section || 'Other';
            out[sec] = out[sec] || [];
            out[sec].push(f);
        });
        Object.values(out).forEach(arr => sortPerfDebtItems(arr, 'severity_asc'));
        return out;
    }

    function sortPerfDebtItems(items, mode) {
        if (!items || !items.length) return items;
        const m = mode || 'severity_asc';
        const cmpTs = (a, b) => String(a.capture_timestamp || '').localeCompare(String(b.capture_timestamp || ''));
        const titleOf = (x) => String(x.title || '');
        const objOf = (x) => String(x.object_name || '');
        const recOf = (x) => String(x.recommendation || '');
        items.sort((a, b) => {
            if (m === 'captured_desc' || m === 'captured_asc') {
                const c = cmpTs(a, b);
                return m === 'captured_desc' ? -c : c;
            }
            if (m === 'finding_asc' || m === 'finding_desc') {
                const c = titleOf(a).localeCompare(titleOf(b), undefined, { sensitivity: 'base' });
                return m === 'finding_asc' ? c : -c;
            }
            if (m === 'object_asc' || m === 'object_desc') {
                const c = objOf(a).localeCompare(objOf(b), undefined, { sensitivity: 'base' });
                return m === 'object_asc' ? c : -c;
            }
            if (m === 'recommendation_asc' || m === 'recommendation_desc') {
                const c = recOf(a).localeCompare(recOf(b), undefined, { sensitivity: 'base' });
                return m === 'recommendation_asc' ? c : -c;
            }
            const ra = bySeverityRank(a.severity);
            const rb = bySeverityRank(b.severity);
            if (ra !== rb) return m === 'severity_desc' ? rb - ra : ra - rb;
            return String(b.capture_timestamp || '').localeCompare(String(a.capture_timestamp || ''));
        });
        return items;
    }

    function renderTable(items, sectionName) {
        if (!items || items.length === 0) {
            return `<div class="text-muted" style="padding:0.5rem 0;">No findings.</div>`;
        }
        const sec = sectionName || 'Other';
        window._perfDebtSort = window._perfDebtSort || {};
        const sortVal = window._perfDebtSort[sec] || 'severity_asc';
        const sortOptions = [
            ['severity_asc', 'Severity (critical first)'],
            ['severity_desc', 'Severity (info first)'],
            ['captured_desc', 'Captured (newest)'],
            ['captured_asc', 'Captured (oldest)'],
            ['finding_asc', 'Finding (A–Z)'],
            ['finding_desc', 'Finding (Z–A)'],
            ['object_asc', 'Object (A–Z)'],
            ['object_desc', 'Object (Z–A)'],
            ['recommendation_asc', 'Recommendation (A–Z)'],
            ['recommendation_desc', 'Recommendation (Z–A)'],
        ];
        const sortSelect = `
            <div class="perfdebt-sort-bar" style="display:flex; align-items:center; gap:0.5rem; margin-bottom:0.5rem; flex-wrap:wrap;">
                <span class="text-muted" style="font-size:0.75rem;">Sort</span>
                <select class="custom-select perfdebt-sort" data-section="${window.escapeHtml(sec)}" style="font-size:0.75rem; padding:0.25rem 0.5rem; max-width:min(100%, 22rem);">
                    ${sortOptions.map(([v, label]) => `<option value="${v}" ${v === sortVal ? 'selected' : ''}>${window.escapeHtml(label)}</option>`).join('')}
                </select>
            </div>`;

        window._perfDebtFindings = window._perfDebtFindings || {};
        const rows = items.slice(0, 200).map((f) => {
            const ts = f.capture_timestamp ? new Date(f.capture_timestamp).toLocaleString() : '--';
            const sev = sevKey(f.severity);
            const titleRaw = f.title || '';
            const title = window.escapeHtml(titleRaw);
            const obj = window.escapeHtml(f.object_name || '');
            const rec = window.escapeHtml(f.recommendation || '');
            const key = window.escapeHtml(f.finding_key || '');
            const fix = f.fix_script || '';
            const hasFix = fix && String(fix).trim().length > 0;
            const fid = 'fd-' + Math.random().toString(36).slice(2);
            window._perfDebtFindings[fid] = f;

            let statusIcon = '<i class="fa-solid fa-circle-check" style="color:var(--success);"></i>';
            let rowStyle = '';
            if (sev === 'CRITICAL') {
                statusIcon = '<i class="fa-solid fa-circle-xmark" style="color:var(--danger);"></i>';
                rowStyle = 'style="background:rgba(239,68,68,0.04);"';
            } else if (sev === 'WARNING') {
                statusIcon = '<i class="fa-solid fa-triangle-exclamation" style="color:var(--warning);"></i>';
                rowStyle = 'style="background:rgba(245,158,11,0.04);"';
            }

            return `
                <tr ${rowStyle}>
                    <td class="perfdebt-col-status">${statusIcon}</td>
                    <td class="perfdebt-col-finding">
                        <button class="perfdebt-finding-link" data-open-finding="1" data-fid="${fid}" title="Click to view details">
                            ${title}
                        </button>
                    </td>
                    <td class="perfdebt-col-object">
                        <div class="perfdebt-cell-ellipsis" title="${obj}">${obj}</div>
                    </td>
                    <td class="perfdebt-col-captured">${ts}</td>
                    <td class="perfdebt-col-rec">
                        <div class="perfdebt-cell-ellipsis" title="${rec}">${rec}</div>
                    </td>
                    <td class="perfdebt-col-fix">
                        ${hasFix ? `<button class="btn btn-sm btn-outline text-accent" data-copy-fix="1" data-fix="${encodeURIComponent(fix)}" title="Copy fix script"><i class="fa-regular fa-copy"></i></button>` : `<span class="text-muted" title="No fix script available for this finding">N/A</span>`}
                    </td>
                </tr>
            `;
        }).join('');
        return `
            ${sortSelect}
            <div class="table-responsive perfdebt-table-wrap">
                <table class="data-table perfdebt-table" style="font-size:0.78rem; width:100%; table-layout:fixed;">
                    <colgroup>
                        <col class="perfdebt-col-status" />
                        <col class="perfdebt-col-finding" />
                        <col class="perfdebt-col-object" />
                        <col class="perfdebt-col-captured" />
                        <col class="perfdebt-col-rec" />
                        <col class="perfdebt-col-fix" />
                    </colgroup>
                    <thead>
                        <tr>
                            <th class="perfdebt-col-status" style="text-align:center;">Status</th>
                            <th class="perfdebt-col-finding">Finding</th>
                            <th class="perfdebt-col-object">Object</th>
                            <th class="perfdebt-col-captured">Captured</th>
                            <th class="perfdebt-col-rec">Recommendation</th>
                            <th class="perfdebt-col-fix" style="text-align:right;">Fix</th>
                        </tr>
                    </thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>
        `;
    }

    function bindPerfDebtSortHandler(root) {
        if (!root || root._perfDebtSortBound) return;
        root._perfDebtSortBound = true;
        root.addEventListener('change', (e) => {
            const sel = e.target && e.target.closest ? e.target.closest('select.perfdebt-sort') : null;
            if (!sel || !window._perfDebtGrouped) return;
            const section = sel.getAttribute('data-section');
            if (!section) return;
            const mode = sel.value;
            window._perfDebtSort = window._perfDebtSort || {};
            window._perfDebtSort[section] = mode;
            const items = [...(window._perfDebtGrouped[section] || [])];
            sortPerfDebtItems(items, mode);
            const panel = sel.closest('[data-perfdebt-panel]');
            const content = panel && panel.querySelector('[data-perfdebt-content]');
            if (content) content.innerHTML = renderTable(items, section);
        });
    }

    /** Per-section expand/collapse (delegated). Expand/Collapse all buttons use setAllSectionsCollapsed. */
    function bindPerfDebtPanelToggle(root) {
        if (!root || root._perfDebtToggleBound) return;
        root._perfDebtToggleBound = true;
        root.addEventListener('click', (e) => {
            const header = e.target && e.target.closest ? e.target.closest('[data-perfdebt-toggle]') : null;
            if (!header) return;
            const targetId = header.getAttribute('data-perfdebt-target');
            if (!targetId) return;
            const content = document.getElementById(targetId);
            if (!content) return;
            content.classList.toggle('hidden');
            const nowHidden = content.classList.contains('hidden');
            const icon = header.querySelector('[data-perfdebt-chevron]');
            if (icon) {
                icon.classList.remove('fa-chevron-down', 'fa-chevron-up');
                icon.classList.add(nowHidden ? 'fa-chevron-down' : 'fa-chevron-up');
            }
        });
    }

    function bindCopyHandlers(root) {
        if (!root) return;
        root.addEventListener('click', async (e) => {
            const openBtn = e.target && e.target.closest ? e.target.closest('button[data-open-finding]') : null;
            if (openBtn) {
                const fid = openBtn.getAttribute('data-fid');
                const finding = (window._perfDebtFindings && fid) ? window._perfDebtFindings[fid] : null;
                if (finding) openPerfDebtFindingModal(finding);
                return;
            }

            const btn = e.target && e.target.closest ? e.target.closest('button[data-copy-fix]') : null;
            if (!btn) return;
            const fix = decodeURIComponent(btn.getAttribute('data-fix') || '');
            try {
                await navigator.clipboard.writeText(fix);
                btn.innerHTML = '<i class="fa-solid fa-check"></i> Copied';
                setTimeout(() => { btn.innerHTML = '<i class="fa-regular fa-copy"></i> Copy'; }, 1200);
            } catch (err) {
                appDebug('clipboard write failed', err);
            }
        });
    }

    async function loadPerformanceDebt() {
        const outlet = window.routerOutlet;
        const instance = window.appState.config?.instances?.[window.appState.currentInstanceIdx];
        if (!instance || instance.type !== 'sqlserver') {
            outlet.innerHTML = `<div class="page-view active dashboard-sky-theme">
                <h3 class="text-warning">Please select a SQL Server instance first</h3>
            </div>`;
            return;
        }

        outlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <style>
                    /* Performance Debt table polish (scoped to this view) */
                    .perfdebt-table-wrap { max-height: 560px; overflow: auto; border-radius: 10px; border: 1px solid var(--border-color); }
                    .perfdebt-table thead th { position: sticky; top: 0; z-index: 2; background: var(--bg-surface); backdrop-filter: blur(6px); }
                    .perfdebt-table td, .perfdebt-table th { padding: 0.55rem 0.6rem; }
                    .perfdebt-table tbody tr:hover { background: rgba(59, 130, 246, 0.06); }
                    .perfdebt-cell-ellipsis { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
                    .perfdebt-finding-link {
                        background: transparent;
                        border: 1px solid transparent;
                        color: var(--accent-blue);
                        padding: 0;
                        font: inherit;
                        font-weight: 600;
                        cursor: pointer;
                        text-align: left;
                    }
                    .perfdebt-finding-link:hover { text-decoration: underline; }
                    .perfdebt-finding-link:focus { outline: none; border-color: var(--accent-blue); border-radius: 6px; padding: 0.1rem 0.25rem; }
                    .perfdebt-table col.perfdebt-col-status { width: 48px; max-width: 48px; }
                    .perfdebt-table col.perfdebt-col-finding { width: 11%; }
                    .perfdebt-table col.perfdebt-col-object { width: 21%; min-width: 150px; }
                    .perfdebt-table col.perfdebt-col-captured { width: 116px; }
                    .perfdebt-table col.perfdebt-col-rec { width: 38%; min-width: 260px; }
                    .perfdebt-table col.perfdebt-col-fix { width: 72px; max-width: 72px; }
                    .perfdebt-table .perfdebt-col-captured { white-space: nowrap; color: var(--text-secondary); font-size: 0.75rem; }
                    .perfdebt-table .perfdebt-col-status { text-align: center; width: 48px; }
                    .perfdebt-table .perfdebt-col-fix { text-align: right; }
                </style>
                <div class="page-title flex-between">
                    <div>
                        <h1><i class="fa-solid fa-screwdriver-wrench text-accent"></i> Performance Debt</h1>
                        <p class="subtitle">Instance: ${window.escapeHtml(instance.name)} | Maintenance & Risk (hourly snapshots)</p>
                    </div>
                    <div style="display:flex; align-items:center; gap:1rem; flex-wrap:wrap; justify-content:flex-end;">
                        ${window.renderStatusStrip({ lastUpdateId: 'perfDebtLastRefresh', sourceBadgeId: 'perfDebtSourceBadge', includeHealth: false, includeFreshness: false, autoRefreshText: '' })}
                        <div class="flex-between" style="align-items:center; gap:0.5rem;">
                            <span class="text-muted" style="font-size:0.8rem;">Lookback</span>
                            <select id="perfDebtLookback" class="custom-select" style="width:auto; padding:0.4rem 0.6rem;">
                                <option value="2">2h</option>
                                <option value="6">6h</option>
                                <option value="24">24h</option>
                                <option value="168">7d</option>
                            </select>
                            <button id="perfDebtRefreshBtn" class="btn btn-sm btn-outline text-accent"><i class="fa-solid fa-rotate"></i> Refresh</button>
                            <button id="perfDebtExpandAll" class="btn btn-sm btn-outline text-accent"><i class="fa-solid fa-up-right-and-down-left-from-center"></i> Expand all</button>
                            <button id="perfDebtCollapseAll" class="btn btn-sm btn-outline text-accent"><i class="fa-solid fa-down-left-and-up-right-to-center"></i> Collapse all</button>
                        </div>
                    </div>
                </div>

                <div class="metrics-row" style="display:grid; grid-template-columns:repeat(4, minmax(0, 1fr)); gap:0.75rem; margin-top:0.75rem;">
                    <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, rgba(254,226,226,0.85) 0%, rgba(254,202,202,0.85) 100%);">
                        <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#991b1b;">Critical</span><i class="fa-solid fa-circle-xmark card-icon" style="color:#991b1b;"></i></div>
                        <div id="perfDebtCrit" class="metric-value" style="font-size:1.25rem; font-weight:bold; color:#991b1b;">--</div>
                        <div class="metric-trend" style="font-size:0.65rem; color:#991b1b;">Immediate action</div>
                    </div>
                    <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, rgba(254,243,199,0.85) 0%, rgba(253,230,138,0.85) 100%);">
                        <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#92400e;">Warning</span><i class="fa-solid fa-triangle-exclamation card-icon" style="color:#92400e;"></i></div>
                        <div id="perfDebtWarn" class="metric-value" style="font-size:1.25rem; font-weight:bold; color:#92400e;">--</div>
                        <div class="metric-trend" style="font-size:0.65rem; color:#92400e;">Should be addressed</div>
                    </div>
                    <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem; background:linear-gradient(135deg, rgba(220,252,231,0.85) 0%, rgba(187,247,208,0.85) 100%);">
                        <div class="metric-header"><span class="metric-title" style="font-size:0.7rem; color:#166534;">Info</span><i class="fa-solid fa-circle-info card-icon" style="color:#166534;"></i></div>
                        <div id="perfDebtInfo" class="metric-value" style="font-size:1.25rem; font-weight:bold; color:#166534;">--</div>
                        <div class="metric-trend" style="font-size:0.65rem; color:#166534;">Review/track</div>
                    </div>
                    <div class="metric-card glass-panel" style="padding:0.4rem 0.6rem;">
                        <div class="metric-header"><span class="metric-title" style="font-size:0.7rem;">Total Findings</span><i class="fa-solid fa-list-check card-icon text-accent"></i></div>
                        <div id="perfDebtTotal" class="metric-value" style="font-size:1.25rem; font-weight:bold;">--</div>
                        <div id="perfDebtMeta" class="metric-trend" style="font-size:0.65rem;" class="text-muted">--</div>
                    </div>
                </div>

                <div id="perfDebtBody" style="margin-top:0.75rem;">
                    <div class="glass-panel" style="padding:1rem;">
                        <div class="text-muted">Loading findings…</div>
                    </div>
                </div>
            </div>
        `;

        const body = document.getElementById('perfDebtBody');
        bindCopyHandlers(body);
        bindPerfDebtSortHandler(body);
        bindPerfDebtPanelToggle(body);

        function setAllSectionsCollapsed(collapsed) {
            const panels = body.querySelectorAll('[data-perfdebt-panel]');
            panels.forEach(p => {
                const content = p.querySelector('[data-perfdebt-content]');
                const icon = p.querySelector('[data-perfdebt-chevron]');
                if (!content || !icon) return;
                const isHidden = content.classList.contains('hidden');
                if (collapsed && !isHidden) {
                    content.classList.add('hidden');
                    icon.classList.remove('fa-chevron-up'); icon.classList.add('fa-chevron-down');
                }
                if (!collapsed && isHidden) {
                    content.classList.remove('hidden');
                    icon.classList.remove('fa-chevron-down'); icon.classList.add('fa-chevron-up');
                }
            });
        }

        document.getElementById('perfDebtExpandAll').addEventListener('click', () => setAllSectionsCollapsed(false));
        document.getElementById('perfDebtCollapseAll').addEventListener('click', () => setAllSectionsCollapsed(true));

        async function fetchAndRender() {
            const lookback = Number(document.getElementById('perfDebtLookback')?.value || 2);
            const url = `/api/mssql/performance-debt?instance=${encodeURIComponent(instance.name)}&lookback_hours=${encodeURIComponent(String(lookback))}`;
            const t0 = Date.now();
            const resp = await fetch(url);
            window.updateSourceBadge('perfDebtSourceBadge', resp.headers.get('X-Data-Source'));
            let data;
            try {
                data = await resp.json();
            } catch (e) {
                const txt = await resp.text().catch(() => '');
                throw new Error(`Bad JSON response (status ${resp.status}): ${txt.slice(0, 200)}`);
            }
            if (!resp.ok) {
                const msg = (data && (data.error || data.message)) ? (data.error || data.message) : `HTTP ${resp.status}`;
                throw new Error(msg);
            }
            const findings = data?.findings || [];
            const grouped = groupBySection(findings);
            window._perfDebtGrouped = grouped;

            const total = findings.length;
            const crit = findings.filter(f => String(f.severity || '').toUpperCase() === 'CRITICAL').length;
            const warn = findings.filter(f => String(f.severity || '').toUpperCase() === 'WARNING').length;
            const info = total - crit - warn;

            const lastRefreshEl = document.getElementById('perfDebtLastRefresh');
            if (lastRefreshEl) lastRefreshEl.textContent = new Date().toLocaleTimeString();

            const critEl = document.getElementById('perfDebtCrit');
            const warnEl = document.getElementById('perfDebtWarn');
            const infoEl = document.getElementById('perfDebtInfo');
            const totalEl = document.getElementById('perfDebtTotal');
            const metaEl = document.getElementById('perfDebtMeta');
            if (critEl) critEl.textContent = String(crit);
            if (warnEl) warnEl.textContent = String(warn);
            if (infoEl) infoEl.textContent = String(info);
            if (totalEl) totalEl.textContent = String(total);

            let newestTs = null;
            if (findings.length > 0) {
                newestTs = findings.map(f => f.capture_timestamp).filter(Boolean).sort().slice(-1)[0] || null;
            }
            const newestTxt = newestTs ? new Date(newestTs).toLocaleString() : '--';
            if (metaEl) metaEl.textContent = `Latest snapshot: ${newestTxt} • ${(Date.now() - t0)}ms`;

            const sections = [
                'Index Health',
                'Statistics Health',
                'Storage & Growth',
                'Backup & Recovery',
                'SQL Agent',
                'Engine Config',
            ];

            const html = sections.map((sec, idx) => {
                const items = grouped[sec] || [];
                const catCritical = items.filter(x => sevKey(x.severity) === 'CRITICAL').length;
                const catWarning = items.filter(x => sevKey(x.severity) === 'WARNING').length;
                const catSeverity = catCritical > 0 ? 'CRITICAL' : (catWarning > 0 ? 'WARNING' : 'OK');
                const badgeClass = catSeverity === 'CRITICAL' ? 'badge-danger' : (catSeverity === 'WARNING' ? 'badge-warning' : 'badge-success');
                const sectionId = 'perfdebt-sec-' + idx + '-' + Math.random().toString(36).slice(2, 11);
                const collapsed = (sec !== 'Index Health'); // keep Index Health open by default
                return `
                    <div class="table-card glass-panel mt-3" style="padding:0.75rem;" data-perfdebt-panel="1">
                        <div class="card-header" data-perfdebt-toggle="1" data-perfdebt-target="${sectionId}" style="cursor:pointer;">
                            <h3 style="font-size:0.85rem; margin:0; display:flex; align-items:center; gap:0.5rem;">
                                <i class="fa-solid ${collapsed ? 'fa-chevron-down' : 'fa-chevron-up'}" data-perfdebt-chevron="1" style="transition:transform 0.2s;"></i>
                                <span class="text-accent">${window.escapeHtml(sec)}</span>
                                <span class="badge ${badgeClass}" style="font-size:0.65rem;">${items.length}</span>
                            </h3>
                        </div>
                        <div id="${sectionId}" data-perfdebt-content="1" class="table-responsive ${collapsed ? 'hidden' : ''}" style="margin-top:0.5rem;">
                            ${renderTable(items, sec)}
                        </div>
                    </div>
                `;
            }).join('');

            body.innerHTML = html || `<div class="glass-panel" style="padding:1rem;"><div class="text-muted">No findings available.</div></div>`;
        }

        document.getElementById('perfDebtRefreshBtn').addEventListener('click', () => {
            body.innerHTML = `<div class="glass-panel" style="padding:1rem;"><div class="text-muted">Loading findings…</div></div>`;
            fetchAndRender().catch(err => {
                appDebug('perf debt fetch failed', err);
                body.innerHTML = `<div class="glass-panel" style="padding:1rem;"><div class="text-danger">Failed to load findings.</div></div>`;
            });
        });

        document.getElementById('perfDebtLookback').addEventListener('change', () => {
            body.innerHTML = `<div class="glass-panel" style="padding:1rem;"><div class="text-muted">Loading findings…</div></div>`;
            fetchAndRender().catch(() => { body.innerHTML = `<div class="glass-panel" style="padding:1rem;"><div class="text-danger">Failed to load findings.</div></div>`; });
        });

        fetchAndRender().catch(err => {
            appDebug('perf debt fetch failed', err);
            body.innerHTML = `<div class="text-danger">Failed to load findings.</div>
                <div class="text-muted" style="margin-top:0.35rem; font-size:0.85rem;">${window.escapeHtml(err?.message || String(err))}</div>`;
        });
    }

    window.mssql_PerformanceDebtDashboard = loadPerformanceDebt;
})();

