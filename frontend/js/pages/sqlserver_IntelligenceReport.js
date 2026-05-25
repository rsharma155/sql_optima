/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Intelligence Report (rule-based analysis).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function() {
    const IFRAME_LOAD_TIMEOUT_MS = 90000;

    function formatGeneratedAt(iso) {
        if (!iso) return '';
        try {
            const d = new Date(iso);
            return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
        } catch (e) {
            return iso;
        }
    }

    window.SqlServerIntelligenceReportView = async function() {
        const instIdx = window.appState.currentInstanceIdx;
        const inst = window.appState.config?.instances?.[instIdx];
        const serverID = inst?.server_id || inst?.name;

        if (!inst || inst.type !== 'sqlserver') {
            window.routerOutlet.innerHTML = '<div class="page-view active"><h3 class="text-warning">Please select a SQL Server instance first</h3></div>';
            return;
        }

        const template = await window.loadTemplate('pages/sqlserver_intelligence_report.html');
        window.routerOutlet.innerHTML = template;

        if (window.appState.config && window.appState.config.intelligence_active === false) {
            const warning = document.createElement('div');
            warning.className = 'alert alert-warning m-3';
            warning.innerHTML = '<i class="fa-solid fa-triangle-exclamation"></i> <strong>Intelligence Engine Offline:</strong> The backend analysis engine is currently unavailable. This usually means the primary TimescaleDB metrics store is not connected or initialized.';
            window.routerOutlet.prepend(warning);
        }

        const nameEl = document.getElementById('ir-instance-name');
        if (nameEl && inst.name) nameEl.textContent = inst.name;

        const btn = document.getElementById('generate-report-btn');
        const refreshBtn = document.getElementById('refresh-report-btn');
        const status = document.getElementById('report-status');
        const loading = document.getElementById('report-loading');
        const iframe = document.getElementById('report-iframe');
        const generatedAtEl = document.getElementById('report-generated-at');

        if (!btn || !iframe) return;

        let navListener = null;
        let iframeTimer = null;

        const onNavigateMessage = (e) => {
            if (!e.data || e.data.type !== 'sql-optima-navigate' || !e.data.route) return;
            if (typeof window.appNavigate === 'function') {
                window.appNavigate(e.data.route);
            }
        };
        window.addEventListener('message', onNavigateMessage);
        navListener = onNavigateMessage;

        const cleanup = () => {
            if (navListener) {
                window.removeEventListener('message', navListener);
                navListener = null;
            }
            if (iframeTimer) {
                clearTimeout(iframeTimer);
                iframeTimer = null;
            }
        };

        const resetSteps = () => {
            ['step-metrics', 'step-rules', 'step-forecast', 'step-render'].forEach(id => {
                const el = document.getElementById(id);
                if (el) {
                    el.className = 'text-muted';
                    const icon = el.querySelector('i');
                    if (icon) icon.className = 'fa-regular fa-circle';
                }
            });
        };

        const updateStep = (id, stepStatus) => {
            const el = document.getElementById(id);
            if (!el) return;
            const icon = el.querySelector('i');
            if (stepStatus === 'active') {
                el.className = 'text-accent fw-bold';
                if (icon) icon.className = 'fa-solid fa-circle-notch fa-spin';
            } else if (stepStatus === 'done') {
                el.className = 'text-success';
                if (icon) icon.className = 'fa-solid fa-circle-check';
            }
        };

        const loadReportIframe = (runId, { fromSnapshot = false, refresh = false } = {}) => {
            const theme = document.documentElement.getAttribute('data-theme') || 'light';
            const instName = encodeURIComponent(inst.name || serverID);
            let url = `/api/sqlserver/intelligence-report/report/${runId}?format=html&server_id=${serverID}&instance_name=${instName}&theme=${theme}`;
            if (fromSnapshot) url += '&from_snapshot=1';
            if (refresh) url += '&refresh=1';

            updateStep('step-render', 'active');

            return new Promise((resolve, reject) => {
                if (iframeTimer) clearTimeout(iframeTimer);

                const onLoad = () => {
                    if (iframeTimer) clearTimeout(iframeTimer);
                    iframe.removeEventListener('load', onLoad);
                    iframe.removeEventListener('error', onError);
                    updateStep('step-render', 'done');
                    resolve();
                };
                const onError = () => {
                    if (iframeTimer) clearTimeout(iframeTimer);
                    iframe.removeEventListener('load', onLoad);
                    iframe.removeEventListener('error', onError);
                    reject(new Error('Report iframe failed to load'));
                };

                iframe.addEventListener('load', onLoad);
                iframe.addEventListener('error', onError);
                iframeTimer = setTimeout(() => {
                    iframe.removeEventListener('load', onLoad);
                    iframe.removeEventListener('error', onError);
                    reject(new Error('Report render timed out — try Refresh or check server logs'));
                }, IFRAME_LOAD_TIMEOUT_MS);

                iframe.src = url;
            });
        };

        const runAnalysis = async ({ refresh = false, preferSnapshot = false } = {}) => {
            const signal = typeof window.getPageSignal === 'function' ? window.getPageSignal() : undefined;
            btn.disabled = true;
            if (refreshBtn) refreshBtn.disabled = true;
            loading.style.display = 'flex';
            resetSteps();
            status.textContent = 'Gathering metrics and computing thresholds...';
            updateStep('step-metrics', 'active');

            try {
                let useSnapshotOnly = false;
                let latestMeta = null;

                if (!refresh && preferSnapshot) {
                    const latestResp = await window.apiClient.authenticatedFetch(
                        `/api/sqlserver/intelligence-report/latest?server_id=${serverID}`,
                        { signal }
                    );
                    if (latestResp.ok) {
                        latestMeta = await latestResp.json();
                        if (latestMeta?.has_snapshot && latestMeta.age_hours < 24) {
                            useSnapshotOnly = true;
                            if (generatedAtEl) {
                                generatedAtEl.textContent = `Last report: ${formatGeneratedAt(latestMeta.generated_at)} (${latestMeta.category || '—'}, score ${Number(latestMeta.overall_score || 0).toFixed(1)})`;
                            }
                        }
                    }
                }

                if (useSnapshotOnly && latestMeta) {
                    updateStep('step-metrics', 'done');
                    updateStep('step-rules', 'done');
                    updateStep('step-forecast', 'done');
                    status.innerHTML = `<span class="text-info"><i class="fa-solid fa-clock-rotate-left"></i> Loading cached report from ${window.escapeHtml(formatGeneratedAt(latestMeta.generated_at))}.</span>`;
                    await loadReportIframe(serverID, { fromSnapshot: true });
                    loading.style.display = 'none';
                    return;
                }

                if (preferSnapshot && !refresh) {
                    status.textContent = 'No recent cached report. Click Run Analysis to generate a new intelligence report.';
                    loading.style.display = 'none';
                    return;
                }

                const response = await window.apiClient.authenticatedFetch(
                    `/api/sqlserver/intelligence-report/analyze?server_id=${serverID}`,
                    { method: 'POST', signal }
                );

                updateStep('step-metrics', 'done');
                updateStep('step-rules', 'active');

                if (!response.ok) {
                    let errorMessage = `Server returned ${response.status}`;
                    try {
                        const errorData = await response.json();
                        if (errorData?.error) errorMessage = errorData.error;
                    } catch (e) {
                        const text = await response.text();
                        if (text) errorMessage = text;
                    }
                    throw new Error(errorMessage);
                }

                const data = await response.json();
                updateStep('step-rules', 'done');
                updateStep('step-forecast', 'active');

                if (data.generated_at && generatedAtEl) {
                    generatedAtEl.textContent = `Generated: ${formatGeneratedAt(data.generated_at)}`;
                }

                if (data.data_status === 'Insufficient') {
                    status.innerHTML = `<span class="text-warning"><i class="fa-solid fa-hourglass-start"></i> ${window.escapeHtml(data.data_note)}</span>`;
                    iframe.src = 'about:blank';
                    loading.style.display = 'none';
                } else {
                    const statusClass = data.data_status === 'Partial' ? 'text-info' : 'text-success';
                    const icon = data.data_status === 'Partial' ? 'fa-circle-info' : 'fa-check-circle';
                    status.innerHTML = `<span class="${statusClass}"><i class="fa-solid ${icon}"></i> Analysis complete (${data.data_status} data).</span>
                        Overall Risk: <strong>${window.escapeHtml(data.overall_risk?.category || '—')}</strong> (${Number(data.overall_risk?.overall_score || 0).toFixed(1)})`;
                    if (data.data_status === 'Partial' && data.data_note) {
                        status.innerHTML += `<div style="font-size:0.8rem;margin-top:2px;" class="text-muted">${window.escapeHtml(data.data_note)}</div>`;
                    }

                    updateStep('step-forecast', 'done');
                    await loadReportIframe(data.run_id || serverID, { refresh });
                    loading.style.display = 'none';
                }
            } catch (err) {
                if (err.name === 'AbortError') return;
                console.error('[IntelligenceReport] Analysis failed:', err);
                status.innerHTML = `<span class="text-danger"><i class="fa-solid fa-circle-exclamation"></i> Error: ${window.escapeHtml(err.message)}</span>`;
                loading.style.display = 'none';
            } finally {
                btn.disabled = false;
                if (refreshBtn) refreshBtn.disabled = false;
            }
        };

        btn.addEventListener('click', () => runAnalysis({ refresh: true }));
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => runAnalysis({ refresh: true }));
        }

        // Auto-load cached report when fresh snapshot exists (< 24h)
        runAnalysis({ preferSnapshot: true });

        window._intelReportCleanup = cleanup;
    };
})();
