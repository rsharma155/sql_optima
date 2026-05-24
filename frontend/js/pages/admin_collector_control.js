// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Logic for Collector Control Center (Admin) with Modern UI and Validation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

/**
 * Helper to show notifications or fallback to alert
 */
function notify(message, type = 'success') {
    if (typeof window.showNotification === 'function') {
        window.showNotification(message, type);
    } else {
        alert(message);
    }
}

export async function loadCollectorConfigs() {
    const container = document.getElementById('admin-content');
    if (!container) return;

    container.innerHTML = `
        <div class="glass-panel" style="padding:1.5rem; border-radius:12px; box-shadow: var(--shadow-sm);">
            <div class="flex-between" style="margin-bottom:1.5rem; border-bottom:1px solid var(--border-color); padding-bottom:1rem;">
                <div>
                    <h2 style="font-size:1.1rem; margin:0; font-weight:600;">
                        <i class="fa-solid fa-clock-rotate-left text-accent"></i> Collector Frequencies
                    </h2>
                    <p class="text-muted" style="margin:0.25rem 0 0; font-size:0.85rem;">
                        Tune telemetry collection intervals. Changes take effect on the next collection cycle.
                    </p>
                </div>
                <div class="badge badge-outline" style="font-size:0.75rem;">Global Configuration</div>
            </div>
            
            <div id="collector-loading" class="text-center" style="padding:4rem;">
                <div class="spinner" style="margin:0 auto 1.5rem; width:2.5rem; height:2.5rem; border-width:3px;"></div>
                <div class="text-muted" style="font-weight:500;">Retrieving collector definitions…</div>
            </div>

            <div id="collector-table-container" style="display:none;">
                <div class="table-responsive">
                    <table class="data-table" style="width:100%; border-collapse: separate; border-spacing: 0 0.5rem;">
                        <thead>
                            <tr style="text-align:left; font-size:0.75rem; text-transform:uppercase; letter-spacing:0.05em; color:var(--text-muted);">
                                <th style="padding:0.75rem 1rem;">Collector Name</th>
                                <th style="padding:0.75rem 1rem;">Module</th>
                                <th style="padding:0.75rem 0.5rem; text-align:center;" title="Startup launch sequence (lower = earlier goroutine). 10=Pulse, 20=Background, 30-180=dedicated goroutines, —=system.">Order</th>
                                <th style="padding:0.75rem 1rem;">Interval</th>
                                <th style="padding:0.75rem 1rem;">Metadata</th>
                                <th style="padding:0.75rem 1rem; text-align:right;">Actions</th>
                            </tr>
                        </thead>
                        <tbody id="collector-config-body"></tbody>
                    </table>
                </div>
            </div>
        </div>
    `;

    try {
        const response = await window.apiClient.authenticatedFetch('/api/admin/collector-configs');
        if (!response.ok) throw new Error('Failed to fetch configs');
        const configs = await response.json();
        
        const body = document.getElementById('collector-config-body');
        body.innerHTML = '';
        
        const collectorMapping = {
            'Postgres Active Queries': 'Dashboard: Sessions | Metric: Active Queries',
            'Postgres Blocking Locks': 'Dashboard: Locks | Metric: Blocking Locks',
            'Postgres CPU and Memory': 'Dashboards: CPU, Memory | Metrics: CPU, Memory stats',
            'Postgres Wait Stats': 'Dashboard: Wait Stats | Metric: Wait Events',
            'Postgres Storage I/O': 'Dashboard: Storage | Metric: I/O throughput/latency',
            'Postgres Long Running Queries': 'Dashboard: Queries | Metric: Long running queries',
            'Postgres Query Stats': 'Dashboard: pg_stat_statements | Metric: Query performance',
            'SQL Server Active Queries': 'Dashboard: SQL Server Overview | Metric: Active Queries',
            'SQL Server Blocking Locks': 'Dashboard: SQL Server Locks | Metric: Blocking Locks',
            'SQL Server System Metrics': 'Dashboard: SQL Server Overview | Metric: CPU/Memory',
            'SQL Server Storage': 'Dashboard: SQL Server Storage | Metric: File I/O',
            'SQL Server Long Running Queries': 'Dashboard: SQL Server Queries | Metric: Long running queries',
            'SQL Server Query Store': 'Dashboard: SQL Server Query Analysis | Metric: Query Store stats',
            'SQL Server Database Size': 'Dashboard: Global Estate | Metric: Database size',
            'SQL Server Configuration': 'Dashboard: SQL Server Settings | Metric: Config params',
            'SQL Server Index Usage': 'Dashboard: SQL Server Index | Metric: Index usage stats',
            'SQL Server Table Usage': 'Dashboard: SQL Server Table | Metric: Table usage stats',
            'SQL Server Query Analysis': 'Dashboard: SQL Server Query Analysis | Metric: Query performance',
            'SQL Server Watched Query Snapshot': 'Dashboard: SQL Server Watched Queries | Metric: Query snapshots',
            'SQL Server AG Health': 'Dashboard: SQL Server HA | Metric: AG replication health',
            'SQL Server Enterprise Metrics': 'Dashboard: SQL Server Enterprise | Metric: Enterprise KPIs',
            'SQL Server Agent Jobs': 'Dashboard: SQL Server Jobs | Metric: Job status',
            'Query V2 Pipeline': 'System: V2 pipeline orchestrator | Governs: sqlserver_query_snapshot (60 s), sqlserver_session_enrichment (30 s), pg_queries_v2 (60 s) | Toggle: ENABLE_QUERY_V2_PIPELINE env var',
            'sqlserver_query_snapshot': 'Dashboards: SQL Server Workload, Query Analysis, Health | Metric: Watermark-based DMV snapshot with restart detection → sqlserver_query_metrics_v2',
            'sqlserver_session_enrichment': 'Dashboard: SQL Server Workload (app/login timelines) | Metric: plan_handle → login_name/application_name enrichment, correlated by query_snapshot',
            'sqlserver_blocking': 'Dashboard: SQL Server Locks | Metric: Blocking chain',
            'pg_queries_v2': 'Dashboard: Postgres Query Analysis (staged — no dashboard reader yet) | Metric: Delta-tracked pg_stat_statements per query_id → pg_query_metrics_v2',
            'Performance Debt Collection': 'Dashboard: Performance Debt | Metric: Best practices score',
            'Alert Evaluation Loop': 'System: Alerting Engine | Metric: Alert triggers',
            'Base Collector Ticker': 'System: Core Collector | Metric: Heartbeat',
            'Postgres Session Activity': 'Dashboard: Sessions | Metric: Session state',
            'Postgres Wait Summary': 'Dashboard: Wait Stats | Metric: Wait summary',
            'Postgres DB Load': 'Dashboard: Overview | Metric: Database load',
            'Postgres Query Wait Profile': 'Dashboard: Query Analysis | Metric: Wait profile',
            'Postgres Backup Archiver': 'Dashboard: Backups | Metric: Archiver status',
            'Postgres WAL Rate': 'Dashboard: Enterprise | Metric: WAL generation rate',
            'Postgres Base Backup History': 'Dashboard: Backups | Metric: Backup history',
            'Postgres Failed Login Parsing': 'Dashboard: Logs | Metric: Failed logins',
            'Postgres Roles Snapshot': 'Dashboard: Security | Metric: Role configuration',
            'Postgres DDL Activity': 'Dashboard: Logs | Metric: DDL changes',
            'Postgres Control Center': 'Dashboard: Enterprise | Metric: Checkpointer/Archiver',
            'Postgres Connections': 'Dashboard: Sessions | Metric: Connection count',
            'Postgres Replication': 'Dashboard: Replication | Metric: Replication lag',
            'Postgres System Detail': 'Dashboard: Overview | Metric: OS metrics',
            'SQL Server Live KPIs': 'Dashboard: SQL Server Live | Metric: Real-time KPIs',
            'SQL Server Running Queries': 'Dashboard: SQL Server Live | Metric: Currently executing',
            'SQL Server File IO Latency': 'Dashboard: SQL Server Live | Metric: I/O latency',
            'SQL Server TempDB Usage': 'Dashboard: SQL Server Live | Metric: TempDB pressure',
            'SQL Server Wait Stats Live': 'Dashboard: SQL Server Live | Metric: Live waits',
            'SQL Server Connections Live': 'Dashboard: SQL Server Live | Metric: Live connections'
        };

        const v2PipelineJobs = new Set(['Query V2 Pipeline', 'sqlserver_query_snapshot', 'sqlserver_session_enrichment', 'pg_queries_v2']);

        if (!configs || configs.length === 0) {
            body.innerHTML = '<tr><td colspan="6" class="text-center text-muted" style="padding:2rem;">No collectors configured.</td></tr>';
        } else {
            configs.forEach(c => {
                const tr = document.createElement('tr');
                tr.id = `row-${c.id}`;
                tr.className = 'hover-row';
                tr.style.background = 'rgba(255,255,255,0.02)';
                
                const moduleClass = c.module.toLowerCase() === 'postgres' ? 'badge-info' : 'badge-warning';
                const mappingInfo = collectorMapping[c.collector_name] || 'System Internal / Background Task';

                const orderDisplay = c.run_order != null
                    ? `<span style="font-family:monospace; font-weight:600;">${c.run_order}</span>`
                    : `<span class="text-muted">—</span>`;

                const v2Badge = v2PipelineJobs.has(c.collector_name)
                    ? `<span style="background:rgba(99,102,241,0.15);color:#6366f1;font-size:0.6rem;font-weight:700;padding:0.1rem 0.4rem;border-radius:4px;margin-left:0.4rem;vertical-align:middle;letter-spacing:0.03em;">V2</span>`
                    : '';

                tr.innerHTML = `
                    <td style="padding:1rem; border-top-left-radius:8px; border-bottom-left-radius:8px;">
                        <div style="font-weight:600; font-size:0.9rem;">${window.escapeHtml(c.collector_name)}${v2Badge}</div>
                        <div style="font-size:0.7rem; color:var(--text-muted); margin-top:0.2rem;">ID: ${c.id}</div>
                    </td>
                    <td style="padding:1rem;">
                        <span class="badge ${moduleClass}" style="text-transform:capitalize; padding:0.25rem 0.6rem;">${window.escapeHtml(c.module)}</span>
                    </td>
                    <td style="padding:1rem 0.5rem; text-align:center;">${orderDisplay}</td>
                    <td style="padding:1rem;">
                        <div class="view-mode">
                            <span style="font-family:monospace; font-weight:600; font-size:1rem;">${c.frequency_seconds}</span>
                            <span class="text-muted" style="font-size:0.75rem; margin-left:0.25rem;">sec</span>
                        </div>
                        <div class="edit-mode" style="display:none; align-items:center; gap:0.5rem;">
                            <input type="number" 
                                   class="custom-input" 
                                   style="width:100px; padding:0.35rem; font-size:0.9rem;" 
                                   value="${c.frequency_seconds}" 
                                   id="freq-${c.id}" 
                                   min="5" max="604800">
                            <span class="text-muted" style="font-size:0.75rem;">sec</span>
                        </div>
                    </td>
                    <td style="padding:1rem; font-size:0.8rem;">
                        <div style="color:var(--accent-blue); font-weight:500; margin-bottom:0.35rem;"><i class="fa-solid fa-circle-info" style="font-size:0.7rem; width:1rem;"></i> ${window.escapeHtml(mappingInfo)}</div>
                        <div class="text-muted"><i class="fa-solid fa-user" style="font-size:0.7rem; width:1rem;"></i> ${window.escapeHtml(c.updated_by || 'system')}</div>
                        <div class="text-muted" style="margin-top:0.2rem;"><i class="fa-solid fa-clock" style="font-size:0.7rem; width:1rem;"></i> ${new Date(c.updated_at).toLocaleDateString()} ${new Date(c.updated_at).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'})}</div>
                    </td>
                    <td style="padding:1rem; text-align:right; border-top-right-radius:8px; border-bottom-right-radius:8px;">
                        <div class="view-mode">
                            <button class="btn btn-xs btn-outline btn-edit" data-id="${c.id}" style="padding:0.3rem 0.75rem;">
                                <i class="fa-solid fa-pen-to-square"></i> Edit
                            </button>
                        </div>
                        <div class="edit-mode" style="display:none; gap:0.4rem; justify-content:flex-end;">
                            <button class="btn btn-xs btn-accent btn-save" data-id="${c.id}" style="padding:0.3rem 0.75rem;">
                                <i class="fa-solid fa-check"></i> Save
                            </button>
                            <button class="btn btn-xs btn-outline btn-cancel" data-id="${c.id}" style="padding:0.3rem 0.75rem;">
                                <i class="fa-solid fa-xmark"></i>
                            </button>
                        </div>
                    </td>
                `;
                body.appendChild(tr);
            });
        }
        
        // Event Delegation
        body.addEventListener('click', async (e) => {
            const btn = e.target.closest('button');
            if (!btn) return;
            
            const id = btn.dataset.id;
            const row = document.getElementById(`row-${id}`);
            if (!row) return;

            if (btn.classList.contains('btn-edit')) {
                toggleRowMode(row, true);
            } else if (btn.classList.contains('btn-cancel')) {
                toggleRowMode(row, false);
                const viewVal = row.querySelector('.view-mode span').innerText;
                row.querySelector('input').value = viewVal;
            } else if (btn.classList.contains('btn-save')) {
                const input = row.querySelector('input');
                const freq = parseInt(input.value);
                
                // Advanced Validation
                if (isNaN(freq)) {
                    notify('Frequency must be a valid number', 'error');
                    return;
                }
                if (freq < 5) {
                    notify('Minimum allowed frequency is 5 seconds for critical metrics', 'error');
                    return;
                }
                if (freq > 604800) {
                    notify('Maximum allowed frequency is 604800 seconds (1 week)', 'error');
                    return;
                }
                
                btn.disabled = true;
                btn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i>';
                
                await updateCollectorConfig(id, freq);
            }
        });
        
        document.getElementById('collector-loading').style.display = 'none';
        document.getElementById('collector-table-container').style.display = 'block';
    } catch (err) {
        container.innerHTML = `<div class="alert alert-danger" style="margin:2rem;">
            <i class="fa-solid fa-triangle-exclamation"></i> <strong>Initialization Error:</strong> ${window.escapeHtml(err.message)}
        </div>`;
    }
}

function toggleRowMode(row, isEdit) {
    row.querySelectorAll('.view-mode').forEach(el => el.style.display = isEdit ? 'none' : 'block');
    row.querySelectorAll('.edit-mode').forEach(el => el.style.display = isEdit ? 'flex' : 'none');
    if (isEdit) {
        row.style.background = 'rgba(var(--accent-rgb), 0.05)';
        row.querySelector('input').focus();
    } else {
        row.style.background = 'rgba(255,255,255,0.02)';
    }
}

async function updateCollectorConfig(id, frequency) {
    try {
        const response = await window.apiClient.authenticatedFetch(`/api/admin/collector-configs/${id}`, {
            method: 'PUT',
            body: JSON.stringify({ frequency_seconds: frequency })
        });
        
        if (response.ok) {
            notify('Frequency updated successfully. Changes will propagate in the next cycle.');
            loadCollectorConfigs();
        } else {
            const err = await response.json();
            notify('Update failed: ' + (err.error || 'Server rejected the change'), 'error');
            loadCollectorConfigs(); // reset state
        }
    } catch (err) {
        notify('Network error: ' + err.message, 'error');
        loadCollectorConfigs(); // reset state
    }
}
