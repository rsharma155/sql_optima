/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Execution Plan Analyzer — Client-side logic for generating
 *          and rendering library-native high-fidelity HTML reports.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function() {
    const esc = (s) => window.escapeHtml ? window.escapeHtml(String(s ?? '')) : String(s ?? '');

    window.SqlServerPlanAnalyzerView = async function() {
        try {
            const html = await window.loadTemplate('pages/sqlserver_plan_analyzer.html?v=' + new Date().getTime());
            window.routerOutlet.innerHTML = html;
        } catch (err) {
            window.routerOutlet.innerHTML = `<div class="alert alert-danger">Failed to load template: ${err.message}</div>`;
            return;
        }

        const uploadArea = document.getElementById('plan-upload-area');
        const fileInput = document.getElementById('plan-file-input');
        const xmlInput = document.getElementById('plan-xml-input');
        const analyzeBtn = document.getElementById('analyze-btn');
        const resultsPlaceholder = document.getElementById('results-placeholder');
        const resultsContainer = document.getElementById('results-container');
        const reportHost = document.getElementById('report-host');
        
        let lastReportHtml = '';

        // Check for pending plan from other pages (e.g. Watched Query Analyzer)
        if (window._pendingPlanXml) {
            xmlInput.value = window._pendingPlanXml;
            window._pendingPlanXml = null; // Clear it
            setTimeout(() => {
                analyzeBtn.click();
            }, 100);
        }

        // Drag & Drop
        uploadArea.addEventListener('click', () => fileInput.click());
        uploadArea.addEventListener('dragover', (e) => { 
            e.preventDefault(); 
            uploadArea.style.borderColor = 'var(--accent)'; 
            uploadArea.style.background = 'rgba(56, 189, 248, 0.1)';
        });
        uploadArea.addEventListener('dragleave', () => { 
            uploadArea.style.borderColor = 'var(--border-color)'; 
            uploadArea.style.background = 'transparent';
        });
        uploadArea.addEventListener('drop', (e) => {
            e.preventDefault();
            uploadArea.style.borderColor = 'var(--border-color)';
            uploadArea.style.background = 'transparent';
            if (e.dataTransfer.files.length > 0) handleFileUpload(e.dataTransfer.files[0]);
        });

        fileInput.addEventListener('change', () => {
            if (fileInput.files.length > 0) handleFileUpload(fileInput.files[0]);
        });

        function handleFileUpload(file) {
            const reader = new FileReader();
            reader.onload = (e) => {
                xmlInput.value = e.target.result;
                uploadArea.innerHTML = `<i class="fa-solid fa-file-circle-check text-accent"></i><p><b>${esc(file.name)}</b> loaded</p><span>Ready for analysis</span>`;
            };
            reader.readAsText(file);
        }

        analyzeBtn.addEventListener('click', async () => {
            const xml = xmlInput.value.trim();
            if (!xml) { alert('Please provide an execution plan XML.'); return; }
            
            analyzeBtn.disabled = true;
            analyzeBtn.innerHTML = '<i class="fa-solid fa-circle-notch fa-spin"></i> Generating Report...';

            try {
                const response = await window.apiClient.authenticatedFetch('/api/sqlserver/plan/analyze', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/xml' },
                    body: xml
                });
                
                if (!response.ok) {
                    const errText = await response.text();
                    throw new Error(errText || 'Report generation failed');
                }

                const data = await response.json();
                if (data.html_report) {
                    lastReportHtml = data.html_report;
                    renderReport(data.html_report);
                } else {
                    throw new Error('Server did not return an HTML report.');
                }
            } catch (err) {
                console.error('[PlanAnalyzer] Analysis error:', err);
                alert('Error: ' + err.message);
            } finally {
                analyzeBtn.disabled = false;
                analyzeBtn.innerHTML = '<i class="fa-solid fa-magnifying-glass"></i> Generate Report';
            }
        });

        function renderReport(html) {
            resultsPlaceholder.style.display = 'none';
            resultsContainer.style.display = 'block';
            
            // To guarantee that the report is functional and doesn't cause infinite scroll:
            // We strip <html>/<body> and inject into an iframe WITH NO AUTO-RESIZE loop.
            // Instead, we give it a large fixed height or standard scrolling.
            const reportIframe = document.createElement('iframe');
            reportIframe.style.width = '100%';
            reportIframe.style.height = '1200px'; // High fixed height for stability
            reportIframe.style.border = 'none';
            reportIframe.style.background = '#fff';
            
            reportHost.innerHTML = '';
            reportHost.appendChild(reportIframe);
            
            // This is the most compatible way across Safari/Brave
            const doc = reportIframe.contentWindow.document;
            doc.open();
            doc.write(html);
            doc.close();
            
            // Optional: One-time resize
            reportIframe.onload = () => {
                setTimeout(() => {
                    try {
                        const h = reportIframe.contentWindow.document.documentElement.scrollHeight;
                        reportIframe.style.height = Math.max(h, 800) + 100 + 'px';
                    } catch(e) {}
                }, 500);
            };
        }

        // Action Bar Handlers
        document.getElementById('btn-open-new-tab').onclick = () => {
            if (!lastReportHtml) return;
            const win = window.open();
            win.document.write(lastReportHtml);
            win.document.close();
        };

        document.getElementById('btn-download-report').onclick = () => {
            if (!lastReportHtml) return;
            const blob = new Blob([lastReportHtml], { type: 'text/html' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `plan_analysis_${new Date().getTime()}.html`;
            a.click();
            URL.revokeObjectURL(url);
        };
    };
})();
