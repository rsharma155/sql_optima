/**
 * SQL Optima — OS collector setup helpers (download bundle, status panel).
 */
(function () {
    const PANEL_STYLE = 'padding:0.85rem 1rem;border-radius:8px;border:1px solid rgba(59,130,246,0.25);background:rgba(59,130,246,0.06);font-size:0.8rem;line-height:1.45;';
    const DISMISS_STORAGE_KEY = 'sql_optima_os_collector_prompt_dismissed';
    const COLLAPSE_STORAGE_KEY = 'sql_optima_os_collector_prompt_collapsed';

    function isOsCollectorPromptDismissed() {
        try {
            return localStorage.getItem(DISMISS_STORAGE_KEY) === '1';
        } catch (_) {
            return false;
        }
    }

    function dismissOsCollectorPrompt() {
        try {
            localStorage.setItem(DISMISS_STORAGE_KEY, '1');
        } catch (_) { /* private mode */ }
    }

    function isOsCollectorPromptCollapsed() {
        try {
            return sessionStorage.getItem(COLLAPSE_STORAGE_KEY) === '1';
        } catch (_) {
            return false;
        }
    }

    function setOsCollectorPromptCollapsed(collapsed) {
        try {
            if (collapsed) {
                sessionStorage.setItem(COLLAPSE_STORAGE_KEY, '1');
            } else {
                sessionStorage.removeItem(COLLAPSE_STORAGE_KEY);
            }
        } catch (_) { /* ignore */ }
    }

    function findOsCollectorMount(el) {
        if (!el) return null;
        return el.closest(
            '#os-collector-setup-mount, #os-collector-setup-mount-cpu, #pg-os-collector-add-slot, #onb-pg-os-collector-slot'
        );
    }

    function hideOsCollectorMount(container) {
        if (!container) return;
        container.style.display = 'none';
        container.innerHTML = '';
    }

    function hideOsCollectorCpuFallback() {
        const fallback = document.getElementById('pg-cpu-os-notice-fallback');
        if (fallback) fallback.style.display = 'none';
    }

    async function fetchOsCollectorStatus(instanceName) {
        const base = encodeURIComponent(window.location.origin);
        const url = `/api/os-collector/status?instance=${encodeURIComponent(instanceName)}&metrics_base_url=${base}`;
        const resp = await window.apiClient.authenticatedFetch(url);
        const body = await resp.json().catch(() => ({}));
        if (!resp.ok) {
            throw new Error(body.error || `Status failed (${resp.status})`);
        }
        return body;
    }

    async function enableOsMetricsIngest() {
        const paths = ['/api/os-collector/ingest', '/api/admin/os-collector/ingest'];
        let lastErr = 'Enable ingest failed';
        for (const path of paths) {
            const resp = await window.apiClient.authenticatedFetch(path, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ enabled: true })
            });
            const body = await resp.json().catch(() => ({}));
            if (resp.ok) {
                return body;
            }
            lastErr = body.error || body.details || `Enable ingest failed (${resp.status})`;
            if (resp.status !== 404) {
                throw new Error(lastErr);
            }
        }
        throw new Error(
            lastErr + '. Restart the API after pulling the latest code (go run cmd/server/main.go).'
        );
    }

    async function downloadOsCollectorBundle(instanceName) {
        const name = (instanceName || '').trim();
        if (!name) {
            throw new Error('Instance name is required to build the OS collector bundle.');
        }
        const base = encodeURIComponent(window.location.origin);
        const url = `/api/os-collector/bundle?instance=${encodeURIComponent(name)}&metrics_base_url=${base}`;
        if (window.apiClient.downloadAuthenticatedBlob) {
            await window.apiClient.downloadAuthenticatedBlob(
                url,
                `sql-optima-os-collector-${name.replace(/[^a-zA-Z0-9_-]+/g, '_')}.zip`
            );
            try {
                await enableOsMetricsIngest();
            } catch (e) {
                console.warn('OS metrics ingest auto-enable:', e);
            }
            return;
        }
        const resp = await window.apiClient.authenticatedFetch(url, { method: 'GET', headers: {} });
        if (!resp.ok) {
            const text = await resp.text().catch(() => '');
            throw new Error(text || `Download failed (${resp.status})`);
        }
        const blob = await resp.blob();
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = `sql-optima-os-collector-${name}.zip`;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(a.href);
        try {
            await enableOsMetricsIngest();
        } catch (e) {
            console.warn('OS metrics ingest auto-enable:', e);
        }
    }

    function renderSetupPanelHtml(opts) {
        const instance = opts.instanceName || '';
        const compact = !!opts.compact;
        const title = compact
            ? '<strong><i class="fa-solid fa-server"></i> Host telemetry (OS Collector)</strong>'
            : '<strong><i class="fa-solid fa-download"></i> OS Collector — host RAM &amp; CPU on the database server</strong>';
        const collapsed = isOsCollectorPromptCollapsed();

        return `
            <div class="os-collector-setup-panel${collapsed ? ' os-collector-setup-collapsed' : ''}" style="${PANEL_STYLE}">
                <div class="os-collector-setup-header" style="display:flex;justify-content:space-between;align-items:flex-start;gap:0.5rem;flex-wrap:wrap;">
                    <div style="flex:1;min-width:12rem;">${title}</div>
                    <div style="display:flex;gap:0.35rem;flex-shrink:0;align-items:center;">
                        <button type="button" class="btn btn-sm btn-outline os-collector-collapse-btn" title="${collapsed ? 'Expand' : 'Collapse'}">
                            <i class="fa-solid fa-chevron-${collapsed ? 'down' : 'up'}"></i>
                            <span class="os-collector-collapse-label">${collapsed ? 'Expand' : 'Collapse'}</span>
                        </button>
                        <button type="button" class="btn btn-sm btn-outline os-collector-dismiss-btn" title="Hide on all pages until you clear browser storage for this site">
                            <i class="fa-solid fa-xmark"></i> Dismiss
                        </button>
                    </div>
                </div>
                <div class="os-collector-setup-panel-body" style="${collapsed ? 'display:none;' : ''}">
                    <p class="text-muted" style="margin:0.5rem 0 0;font-size:0.75rem;">
                        Download the zip (pre-filled with <strong>instance name</strong>, <strong>server ID</strong>, and <strong>app URL</strong>).
                        On the PostgreSQL host: unzip, then run <code>./quick-install.sh</code> (installs a <strong>cron</strong> job every 5 minutes; prompts for admin JWT once).
                    </p>
                    <div id="${opts.statusId || 'os-collector-status-slot'}" style="margin-top:0.5rem;font-size:0.72rem;"></div>
                    <div style="margin-top:0.65rem;display:flex;flex-wrap:wrap;gap:0.5rem;align-items:center;">
                        <button type="button" class="btn btn-sm btn-accent os-collector-download-btn" data-instance="${window.escapeHtml(instance)}">
                            <i class="fa-solid fa-file-zipper"></i> Download bundle (.zip)
                        </button>
                        <button type="button" class="btn btn-sm btn-outline os-collector-refresh-btn" data-instance="${window.escapeHtml(instance)}">
                            <i class="fa-solid fa-rotate"></i> Refresh status
                        </button>
                        <span class="text-muted" style="font-size:0.72rem;">See <code>INSTALL.txt</code> inside the zip</span>
                    </div>
                </div>
            </div>`;
    }

    function applyCollapseState(panel, collapsed) {
        if (!panel) return;
        const body = panel.querySelector('.os-collector-setup-panel-body');
        const btn = panel.querySelector('.os-collector-collapse-btn');
        const icon = btn?.querySelector('i');
        const label = btn?.querySelector('.os-collector-collapse-label');
        panel.classList.toggle('os-collector-setup-collapsed', collapsed);
        if (body) body.style.display = collapsed ? 'none' : '';
        if (icon) icon.className = `fa-solid fa-chevron-${collapsed ? 'down' : 'up'}`;
        if (label) label.textContent = collapsed ? 'Expand' : 'Collapse';
        if (btn) btn.title = collapsed ? 'Expand' : 'Collapse';
    }

    async function refreshStatusSlot(slotEl, instanceName) {
        if (!slotEl || !instanceName) return;
        if (isOsCollectorPromptDismissed()) {
            hideOsCollectorMount(findOsCollectorMount(slotEl));
            hideOsCollectorCpuFallback();
            return;
        }
        slotEl.innerHTML = '<span class="text-muted"><i class="fa-solid fa-spinner fa-spin"></i> Checking…</span>';
        try {
            const st = await fetchOsCollectorStatus(instanceName);
            const parts = [];
            if (st.ingest_enabled) {
                parts.push('<span class="text-success"><i class="fa-solid fa-circle-check"></i> API ingest enabled</span>');
                if (st.ingest_source === 'env') {
                    parts.push('<span class="text-muted">(controlled by server environment variable)</span>');
                }
            } else if (st.ingest_configurable !== false) {
                parts.push(`<span class="text-warning"><i class="fa-solid fa-triangle-exclamation"></i> API ingest is off</span>
                    <button type="button" class="btn btn-sm btn-accent os-collector-enable-ingest-btn" style="margin-left:0.35rem;vertical-align:baseline;">
                        <i class="fa-solid fa-power-off"></i> Enable ingest
                    </button>`);
            } else {
                parts.push('<span class="text-warning"><i class="fa-solid fa-triangle-exclamation"></i> Set <code>OS_METRICS_INGEST_ENABLED=1</code> on the SQL Optima API and restart (env lock)</span>');
            }
            if (st.os_collector_configured) {
                parts.push('<span class="text-success"><i class="fa-solid fa-circle-check"></i> Host metrics received</span>');
                hideOsCollectorMount(findOsCollectorMount(slotEl));
                hideOsCollectorCpuFallback();
                return;
            }
            parts.push('<span class="text-muted"><i class="fa-solid fa-circle"></i> No host metrics yet for this instance</span>');
            if (st.server_id) {
                parts.push(`<span class="text-muted">Server ID: <code>${window.escapeHtml(st.server_id)}</code></span>`);
            }
            if (st.app_url) {
                parts.push(`<span class="text-muted">App URL: <code>${window.escapeHtml(st.app_url)}</code></span>`);
            }
            if (st.metrics_url) {
                parts.push(`<span class="text-muted">Metrics: <code>${window.escapeHtml(st.metrics_url)}</code></span>`);
            }
            slotEl.innerHTML = parts.join('<br>');
            slotEl.querySelector('.os-collector-enable-ingest-btn')?.addEventListener('click', async (ev) => {
                const btn = ev.currentTarget;
                btn.disabled = true;
                try {
                    await enableOsMetricsIngest();
                    await refreshStatusSlot(slotEl, instanceName);
                } catch (e) {
                    alert(e.message || 'Failed to enable ingest');
                } finally {
                    btn.disabled = false;
                }
            });
        } catch (e) {
            slotEl.innerHTML = `<span class="text-danger">${window.escapeHtml(e.message)}</span>`;
        }
    }

    function wireSetupPanel(container, instanceName) {
        if (!container) return;
        const panel = container.querySelector('.os-collector-setup-panel');
        const slot = container.querySelector('[id^="os-collector-status"]') || panel?.querySelector('[id^="os-collector-"]');
        const getInstance = () => {
            const inp = document.getElementById('srv-name') || document.getElementById('onb-name');
            return (inp && inp.value.trim()) || instanceName || '';
        };

        panel?.querySelector('.os-collector-collapse-btn')?.addEventListener('click', () => {
            const collapsed = !panel.classList.contains('os-collector-setup-collapsed');
            setOsCollectorPromptCollapsed(collapsed);
            applyCollapseState(panel, collapsed);
        });

        panel?.querySelector('.os-collector-dismiss-btn')?.addEventListener('click', () => {
            dismissOsCollectorPrompt();
            hideOsCollectorMount(container);
            hideOsCollectorCpuFallback();
        });

        container.querySelectorAll('.os-collector-download-btn').forEach(btn => {
            btn.addEventListener('click', async () => {
                const inst = btn.getAttribute('data-instance') || getInstance();
                btn.disabled = true;
                try {
                    await downloadOsCollectorBundle(inst);
                } catch (e) {
                    alert(e.message || 'Download failed');
                } finally {
                    btn.disabled = false;
                }
            });
        });
        container.querySelectorAll('.os-collector-refresh-btn').forEach(btn => {
            btn.addEventListener('click', () => refreshStatusSlot(slot, btn.getAttribute('data-instance') || getInstance()));
        });

        const inst = instanceName || getInstance();
        if (inst) refreshStatusSlot(slot, inst);
    }

    /**
     * Mount setup panel into a container element.
     * @param {HTMLElement} container
     * @param {{ instanceName?: string, compact?: boolean, statusId?: string }} opts
     */
    window.mountOsCollectorSetupPanel = function (container, opts = {}) {
        if (!container) return;
        if (isOsCollectorPromptDismissed()) {
            hideOsCollectorMount(container);
            hideOsCollectorCpuFallback();
            return;
        }
        container.innerHTML = renderSetupPanelHtml(opts);
        container.style.display = 'block';
        wireSetupPanel(container, opts.instanceName || '');
    };

    function clearOsCollectorPromptDismiss() {
        try {
            localStorage.removeItem(DISMISS_STORAGE_KEY);
            sessionStorage.removeItem(COLLAPSE_STORAGE_KEY);
        } catch (_) { /* ignore */ }
    }

    window.isOsCollectorPromptDismissed = isOsCollectorPromptDismissed;
    window.dismissOsCollectorPrompt = dismissOsCollectorPrompt;
    window.clearOsCollectorPromptDismiss = clearOsCollectorPromptDismiss;
    window.fetchOsCollectorStatus = fetchOsCollectorStatus;
    window.downloadOsCollectorBundle = downloadOsCollectorBundle;
    window.enableOsMetricsIngest = enableOsMetricsIngest;
    window.refreshOsCollectorSetupStatus = refreshStatusSlot;
})();
