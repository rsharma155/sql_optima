/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Intelligence Report (Autonomous Analysis).
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function() {
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

        const btn = document.getElementById('generate-report-btn');
        const status = document.getElementById('report-status');
        const loading = document.getElementById('report-loading');
        const iframe = document.getElementById('report-iframe');

        if (!btn) return;

        btn.addEventListener('click', async () => {
            btn.disabled = true;
            loading.style.display = 'flex';
            
            const updateStep = (id, status) => {
                const el = document.getElementById(id);
                if (!el) return;
                if (status === 'active') {
                    el.className = 'text-accent fw-bold';
                    el.querySelector('i').className = 'fa-solid fa-circle-notch fa-spin';
                } else if (status === 'done') {
                    el.className = 'text-success';
                    el.querySelector('i').className = 'fa-solid fa-circle-check';
                }
            };

            // Reset steps
            ['step-metrics', 'step-rules', 'step-forecast', 'step-render'].forEach(id => {
                const el = document.getElementById(id);
                if (el) {
                    el.className = 'text-muted';
                    el.querySelector('i').className = 'fa-regular fa-circle';
                }
            });

            status.textContent = 'Gathering metrics and computing thresholds...';
            updateStep('step-metrics', 'active');

            try {
                const response = await window.apiClient.authenticatedFetch(`/api/sqlserver/intelligence-report/analyze?server_id=${serverID}`, {
                    method: 'POST'
                });

                updateStep('step-metrics', 'done');
                updateStep('step-rules', 'active');

                if (!response.ok) {
                    const errText = await response.text();
                    throw new Error(errText || `Server returned ${response.status}`);
                }

                const data = await response.json();
                
                updateStep('step-rules', 'done');
                updateStep('step-forecast', 'active');
                
                if (data.data_status === 'Insufficient') {
                    status.innerHTML = `<span class="text-warning"><i class="fa-solid fa-hourglass-start"></i> ${window.escapeHtml(data.data_note)}</span>`;
                    iframe.src = 'about:blank';
                    loading.style.display = 'none';
                } else {
                    const statusClass = data.data_status === 'Partial' ? 'text-info' : 'text-success';
                    const icon = data.data_status === 'Partial' ? 'fa-circle-info' : 'fa-check-circle';
                    
                    status.innerHTML = `<span class="${statusClass}"><i class="fa-solid ${icon}"></i> Analysis complete (${data.data_status} data).</span> 
                                       Overall Risk: <strong>${data.overall_risk.category}</strong> (${data.overall_risk.overall_score.toFixed(1)})`;
                    
                    if (data.data_status === 'Partial') {
                        status.innerHTML += `<div style="font-size:0.8rem; margin-top:2px;" class="text-muted">${window.escapeHtml(data.data_note)}</div>`;
                    }
                    
                    updateStep('step-forecast', 'done');
                    updateStep('step-render', 'active');
                    
                    // Fetch the HTML report with theme parity (UI-6)
                    const theme = document.documentElement.getAttribute('data-theme') || 'dark';
                    iframe.src = `/api/sqlserver/intelligence-report/report/${data.run_id}?format=html&server_id=${serverID}&theme=${theme}`;
                    
                    iframe.onload = () => {
                        updateStep('step-render', 'done');
                        loading.style.display = 'none';
                    };
                }
                
            } catch (err) {
                console.error('[IntelligenceReport] Analysis failed:', err);
                status.innerHTML = `<span class="text-danger"><i class="fa-solid fa-circle-exclamation"></i> Error: ${window.escapeHtml(err.message)}</span>`;
                loading.style.display = 'none';
            } finally {
                btn.disabled = false;
            }
        });
    };
})();
