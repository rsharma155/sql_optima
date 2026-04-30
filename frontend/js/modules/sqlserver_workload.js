// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: UI logic for the SQL Server Workload Observability Dashboard.
//          Handles time-series visualization using Chart.js and true interval deltas.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

import { api } from '../api/api.js';
import { formatters } from '../utils/formatters.js';
import { charts } from '../utils/charts.js';

export const sqlserverWorkload = {
    charts: {},
    instance: '',
    refreshInterval: null,
    isManualRange: false,
    offenders: [],
    sortKey: 'total_cpu_ms',
    sortDir: 'desc',

    async init() {
        console.log('[Workload] Initializing...');
        this.instance = window.state?.currentInstance || window.appState?.currentInstanceName || '';
        
        const nameEl = document.getElementById('wlInstanceName');
        if (nameEl) nameEl.textContent = this.instance;

        this.setDefaultRange();

        // Attach listeners
        document.getElementById('wlApplyRange')?.addEventListener('click', () => {
            this.isManualRange = true;
            this.refreshAll();
        });

        // Manual Tab Switcher (since Bootstrap JS is not loaded)
        const tabs = document.querySelectorAll('.qa-tab-btn');
        tabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                e.preventDefault();
                
                // Remove active class from all tabs and panes
                tabs.forEach(t => t.classList.remove('active'));
                document.querySelectorAll('.tab-pane').forEach(p => {
                    p.classList.remove('show', 'active');
                });
                
                // Add active class to clicked tab
                tab.classList.add('active');
                
                // Show corresponding pane
                const targetId = tab.getAttribute('data-tab');
                const targetPane = document.getElementById(targetId);
                if (targetPane) {
                    targetPane.classList.add('show', 'active');
                    
                    // Trigger Chart.js resize/update for hidden containers
                    const chartCanvas = targetPane.querySelector('canvas');
                    if (chartCanvas && this.charts[chartCanvas.id]) {
                        this.charts[chartCanvas.id].resize();
                    }
                }
                
                // Trigger data load if needed
                const targetTabId = tab.id;
                const loadNeeded = [
                    'scheduler-history-tab', 'scheduler-stats-tab',
                    'app-lt-tab', 'login-lt-tab', 'app-cpu-tab', 'login-cpu-tab'
                ];
                if (loadNeeded.includes(targetTabId)) {
                    this.refreshAll();
                }
            });
        });

        // Set up auto-refresh (every 30s)
        if (this.refreshInterval) clearInterval(this.refreshInterval);
        this.refreshInterval = setInterval(() => {
            if (!this.isManualRange) {
                this.setDefaultRange();
            }
            this.refreshAll();
        }, 30000);
        
        await this.refreshAll();
    },

    setDefaultRange() {
        const now = new Date();
        const oneHourAgo = new Date(now.getTime() - (60 * 60 * 1000));
        
        const toStr = this.formatToLocalDatetime(now);
        const fromStr = this.formatToLocalDatetime(oneHourAgo);
        
        const fromInput = document.getElementById('wlFrom');
        const toInput = document.getElementById('wlTo');
        if (fromInput) fromInput.value = fromStr;
        if (toInput) toInput.value = toStr;
    },

    formatToLocalDatetime(date) {
        const pad = n => String(n).padStart(2, '0');
        return `${date.getFullYear()}-${pad(date.getMonth()+1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
    },

    destroy() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
        Object.values(this.charts).forEach(c => c.destroy());
        this.charts = {};
    },

    async refreshAll() {
        if (!this.instance) return;

        const fromEl = document.getElementById('wlFrom');
        const toEl = document.getElementById('wlTo');
        if (!fromEl || !toEl) return;

        const from = fromEl.value;
        const to = toEl.value;

        // Show loading
        this.setLoading(true);

        try {
            await Promise.all([
                this.loadSummary(from, to),
                this.loadTrends(from, to),
                this.loadTopOffenders(from, to),
                this.loadServerProperties(),
                this.loadSchedulerStats(),
                this.loadQaSummary(from, to),
                this.loadAppLoadTimeline(from, to),
                this.loadLoginLoadTimeline(from, to),
                this.loadTopApps(from, to),
                this.loadTopLogins(from, to)
            ]);
        } catch (err) {
            console.error('[Workload] Refresh failed:', err);
            if (err.message && err.message.includes('401')) {
                 window.notifications?.error('Session expired. Please log in again.');
            }
        } finally {
            this.setLoading(false);
        }
    },

    setLoading(isLoading) {
        const overlay = document.getElementById('wlLoadingOverlay');
        const table = document.getElementById('wlTopQueriesTable');
        const noData = document.getElementById('wlNoData');

        if (isLoading) {
            if (overlay) overlay.style.display = 'flex';
            if (table) table.style.display = 'none';
            if (noData) noData.style.display = 'none';
        } else {
            if (overlay) overlay.style.display = 'none';
        }
    },

    async loadSummary(from, to) {
        const data = await api.get(`/api/sqlserver/workload/summary?instance=${this.instance}&from=${this.formatDateForApi(from)}&to=${this.formatDateForApi(to)}`);
        
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('wl-kpi-cpu', formatters.number(data.total_cpu_ms / 1000, 1));
        set('wl-kpi-execs', formatters.compactNumber(data.total_executions));
        set('wl-kpi-reads', formatters.compactNumber(data.total_logical_reads));
        set('wl-kpi-mem', formatters.bytes(data.max_memory_grant_kb * 1024));
    },

    async loadServerProperties() {
        try {
            const data = await api.get(`/api/sqlserver/server-properties?instance=${encodeURIComponent(this.instance)}`);
            const props = data.server_properties || {};
            
            const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
            set('wl-prop-cpu', props.cpu_count || '--');
            set('wl-prop-cps', props.cores_per_socket || '--');
            set('wl-prop-sockets', props.socket_count || '--');
            
            const htEl = document.getElementById('wl-prop-ht');
            if (htEl) htEl.innerHTML = props.hyperthread_enabled ? '<span class="badge-success">Enabled</span>' : '<span class="badge-warning">Disabled</span>';
            
            set('wl-prop-numa', props.numa_nodes || '--');
            set('wl-prop-workers', props.max_workers_count || '--');
            set('wl-prop-mem', props.physical_memory_gb ? props.physical_memory_gb.toFixed(1) : '--');
        } catch (e) { console.error('props failed', e); }
    },

    async loadSchedulerStats() {
        try {
            // Fetch more samples for history and table (last 50 polls)
            const data = await api.get(`/api/sqlserver/cpu-scheduler-stats?instance=${encodeURIComponent(this.instance)}&limit=50`);
            const allStats = data.cpu_scheduler_stats || [];
            
            if (allStats.length === 0) return;
            
            const stats = allStats[0]; // Latest for KPIs

            const workerPct = stats.max_workers_count > 0 ? ((stats.total_current_workers_count / stats.max_workers_count) * 100).toFixed(1) : 0;
            const memUsedPct = stats.total_physical_memory_kb > 0 
                ? (((stats.total_physical_memory_kb - stats.available_physical_memory_kb) / stats.total_physical_memory_kb) * 100).toFixed(1) 
                : 0;

            const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
            set('wl-sched-count', stats.scheduler_count || '--');
            set('wl-sched-workers', `${stats.total_current_workers_count || 0}/${stats.max_workers_count || 0} (${workerPct}%)`);
            set('wl-sched-runnable', stats.total_runnable_tasks_count || 0);
            set('wl-sched-queue', stats.total_work_queue_count || 0);
            set('wl-sched-mem', `${memUsedPct}%`);

            const warnings = [];
            if (stats.worker_thread_exhaustion_warning) warnings.push({ label: 'Worker', class: 'badge-danger' });
            if (stats.runnable_tasks_warning) warnings.push({ label: 'Runnable', class: 'badge-warning' });
            if (stats.physical_memory_pressure_warning) warnings.push({ label: 'Memory', class: 'badge-danger' });
            
            const warnEl = document.getElementById('wl-sched-warnings');
            if (warnings.length > 0) {
                warnEl.innerHTML = warnings.map(w => `<span class="${w.class}">${w.label}</span>`).join(' ');
            } else {
                warnEl.innerHTML = '<span class="badge-success">Healthy</span>';
            }

            // Render History Chart
            this.renderSchedulerHistory(allStats);
            
            // Render Stats Table
            this.renderSchedulerTable(allStats);

        } catch (e) { console.error('scheduler failed', e); }
    },

    renderSchedulerHistory(allStats) {
        const labels = allStats.slice().reverse().map(s => new Date(s.capture_timestamp));
        const workerData = allStats.slice().reverse().map(s => s.total_current_workers_count || 0);
        const runnableData = allStats.slice().reverse().map(s => s.total_runnable_tasks_count || 0);
        const queueData = allStats.slice().reverse().map(s => s.total_work_queue_count || 0);

        this.renderChart('wlCpuSchedulerHistoryChart', {
            type: 'line',
            labels: labels,
            datasets: [
                {
                    label: 'Current Workers',
                    data: workerData,
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59, 130, 246, 0.1)',
                    fill: false,
                    tension: 0.3
                },
                {
                    label: 'Runnable Tasks',
                    data: runnableData,
                    borderColor: '#f59e0b',
                    backgroundColor: 'rgba(245, 158, 11, 0.1)',
                    fill: false,
                    tension: 0.3
                },
                {
                    label: 'Work Queue',
                    data: queueData,
                    borderColor: '#ef4444',
                    backgroundColor: 'rgba(239, 68, 68, 0.1)',
                    fill: false,
                    tension: 0.3
                }
            ]
        }, 'CPU Scheduler state over time');
    },

    renderSchedulerTable(allStats) {
        const body = document.getElementById('wlSchedulerStatsBody');
        if (!body) return;

        body.innerHTML = '';
        allStats.forEach(s => {
            const tr = document.createElement('tr');
            const ts = new Date(s.capture_timestamp).toLocaleTimeString();
            tr.innerHTML = `
                <td><span class="text-muted">${ts}</span></td>
                <td class="text-end">${s.max_workers_count}</td>
                <td class="text-end">${s.scheduler_count}</td>
                <td class="text-end"><b class="text-info">${s.total_current_workers_count}</b></td>
                <td class="text-end"><b class="text-warning">${s.total_runnable_tasks_count}</b></td>
                <td class="text-end"><b class="text-rose">${s.total_work_queue_count}</b></td>
                <td class="text-end">${(s.runnable_percent * 100).toFixed(1)}%</td>
                <td><span class="badge ${s.physical_memory_pressure_warning ? 'badge-danger' : 'badge-success'}" style="font-size:0.6rem;">${s.system_memory_state_desc || 'Healthy'}</span></td>
            `;
            body.appendChild(tr);
        });
    },

    async loadQaSummary(from, to) {
        try {
            const excludeSystem = true;
            const dbName = 'all';
            const data = await api.get(`/api/sqlserver/query-analysis/summary?instance=${encodeURIComponent(this.instance)}&from=${this.formatDateForApi(from)}&to=${this.formatDateForApi(to)}&exclude_system=${excludeSystem}&database=${encodeURIComponent(dbName)}`);
            
            const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
            set('kpi-cpu-share', data.top_10_cpu_share_pct != null ? data.top_10_cpu_share_pct.toFixed(1) + '%' : '--');
            set('kpi-duration', formatters.number(data.avg_duration_ms, 1) + ' ms');
            set('kpi-cpu', formatters.number(data.avg_cpu_ms, 1) + ' ms');
            set('kpi-regressions', data.regressions_24h || 0);
            set('kpi-plan-changes', data.plan_changes_24h || 0);
        } catch (e) { console.error('qa summary failed', e); }
    },

    async loadTrends(from, to) {
        const resp = await api.get(`/api/sqlserver/workload/trends?instance=${this.instance}&from=${this.formatDateForApi(from)}&to=${this.formatDateForApi(to)}`);
        const data = resp.trends || [];

        const labels = data.map(p => new Date(p.timestamp));
        
        // 1. CPU Trend (Stacked/Area)
        this.renderChart('wlCpuTrendChart', {
            type: 'line',
            labels: labels,
            datasets: [{
                label: 'CPU seconds',
                data: data.map(p => p.cpu_ms / 1000),
                borderColor: '#ef4444',
                backgroundColor: 'rgba(239, 68, 68, 0.1)',
                fill: true,
                tension: 0.3
            }]
        }, 'CPU consumed per bucket (delta)');

        // 2. Throughput
        this.renderChart('wlExecTrendChart', {
            type: 'line',
            labels: labels,
            datasets: [{
                label: 'Executions',
                data: data.map(p => p.executions),
                borderColor: '#3b82f6',
                backgroundColor: 'rgba(59, 130, 246, 0.1)',
                fill: true,
                tension: 0.3
            }]
        }, 'Executions per bucket');

        // 3. IO Rate
        this.renderChart('wlReadTrendChart', {
            type: 'bar',
            labels: labels,
            datasets: [{
                label: 'Logical Reads',
                data: data.map(p => p.logical_reads),
                backgroundColor: '#f59e0b',
                borderRadius: 4
            }]
        }, 'Logical reads per bucket');

        // 4. Efficiency
        this.renderChart('wlEfficiencyChart', {
            type: 'line',
            labels: labels,
            datasets: [
                {
                    label: 'Avg CPU/Exec (ms)',
                    data: data.map(p => p.avg_cpu_ms),
                    borderColor: '#8b5cf6',
                    yAxisID: 'y'
                },
                {
                    label: 'Avg Rows/Exec',
                    data: data.map(p => p.avg_rows),
                    borderColor: '#10b981',
                    yAxisID: 'y1'
                }
            ]
        }, 'Avg resources per execution over time', true);

        // 5. Pressure Spikes
        this.renderChart('wlPressureChart', {
            type: 'line',
            labels: labels,
            datasets: [
                {
                    label: 'Peak Memory (MB)',
                    data: data.map(p => p.max_grant_kb / 1024),
                    borderColor: '#10b981',
                    borderDash: [5, 5]
                },
                {
                    label: 'Peak Individual Query CPU (ms)',
                    data: data.map(p => p.worst_query_ms),
                    borderColor: '#ef4444',
                    borderWidth: 1
                }
            ]
        }, 'Max resource impact seen in any single poll within the bucket');
    },

    async loadTopOffenders(from, to) {
        const resp = await api.get(`/api/sqlserver/workload/top-queries?instance=${this.instance}&from=${this.formatDateForApi(from)}&to=${this.formatDateForApi(to)}&limit=20`);
        this.offenders = resp.top_offenders || [];
        
        const table = document.getElementById('wlTopQueriesTable');
        const noData = document.getElementById('wlNoData');

        if (this.offenders.length === 0) {
            if (table) table.style.display = 'none';
            if (noData) noData.style.display = 'block';
            return;
        }

        if (table) table.style.display = 'table';
        if (noData) noData.style.display = 'none';

        this.sortOffenders(this.sortKey, this.sortDir);
        this.renderTopOffenders();
        this.attachSortListeners();
    },

    async loadAppLoadTimeline(from, to) {
        try {
            const data = await api.get(`/api/sqlserver/workload/app-load?instance=${this.instance}&from=${this.formatDateForApi(from)}&to=${this.formatDateForApi(to)}`);
            const raw = data.app_load || [];
            
            // Transform for Chart.js (multi-series line chart)
            const appGroups = {};
            const labels = [...new Set(raw.map(r => r.bucket))].sort().map(b => new Date(b));
            
            raw.forEach(r => {
                if (!appGroups[r.app_name]) appGroups[r.app_name] = {};
                appGroups[r.app_name][r.bucket] = r.cpu_ms / 1000;
            });

            const colorPalette = charts.colors && charts.colors.length > 0 ? charts.colors : ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#f97316'];

            const datasets = Object.keys(appGroups).slice(0, 8).map((app, idx) => ({
                label: app,
                data: labels.map(l => appGroups[app][l.toISOString()] || 0),
                borderColor: colorPalette[idx % colorPalette.length],
                fill: false,
                tension: 0.3
            }));

            this.renderChart('wlAppLoadChart', {
                type: 'line',
                labels: labels,
                datasets: datasets
            }, 'App Load (sec)');
        } catch (e) { console.error('app load timeline failed', e); }
    },

    async loadLoginLoadTimeline(from, to) {
        try {
            const data = await api.get(`/api/sqlserver/workload/login-load?instance=${this.instance}&from=${this.formatDateForApi(from)}&to=${this.formatDateForApi(to)}`);
            const raw = data.login_load || [];
            
            const loginGroups = {};
            const labels = [...new Set(raw.map(r => r.bucket))].sort().map(b => new Date(b));
            
            raw.forEach(r => {
                if (!loginGroups[r.login_name]) loginGroups[r.login_name] = {};
                loginGroups[r.login_name][r.bucket] = r.cpu_ms / 1000;
            });

            const colorPalette = charts.colors && charts.colors.length > 0 ? charts.colors : ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#f97316'];

            const datasets = Object.keys(loginGroups).slice(0, 8).map((login, idx) => ({
                label: login,
                data: labels.map(l => loginGroups[login][l.toISOString()] || 0),
                borderColor: colorPalette[(idx + 4) % colorPalette.length],
                fill: false,
                tension: 0.3
            }));

            this.renderChart('wlLoginLoadChart', {
                type: 'line',
                labels: labels,
                datasets: datasets
            }, 'Login Load (sec)');
        } catch (e) { console.error('login load timeline failed', e); }
    },

    async loadTopApps(from, to) {
        try {
            const data = await api.get(`/api/sqlserver/workload/top-apps?instance=${this.instance}&from=${this.formatDateForApi(from)}&to=${this.formatDateForApi(to)}&limit=10`);
            const raw = data.top_apps || [];

            this.renderChart('wlTopAppsChart', {
                type: 'bar',
                labels: raw.map(r => r.app_name),
                datasets: [{
                    label: 'Total CPU (sec)',
                    data: raw.map(r => r.total_cpu_ms / 1000),
                    backgroundColor: 'rgba(16, 185, 129, 0.6)',
                    borderRadius: 4
                }]
            }, 'Top Apps CPU');
        } catch (e) { console.error('top apps failed', e); }
    },

    async loadTopLogins(from, to) {
        try {
            const data = await api.get(`/api/sqlserver/workload/top-logins?instance=${this.instance}&from=${this.formatDateForApi(from)}&to=${this.formatDateForApi(to)}&limit=10`);
            const raw = data.top_logins || [];

            this.renderChart('wlTopLoginsChart', {
                type: 'bar',
                labels: raw.map(r => r.login_name),
                datasets: [{
                    label: 'Total CPU (sec)',
                    data: raw.map(r => r.total_cpu_ms / 1000),
                    backgroundColor: 'rgba(245, 158, 11, 0.6)',
                    borderRadius: 4
                }]
            }, 'Top Logins CPU');
        } catch (e) { console.error('top logins failed', e); }
    },

    attachSortListeners() {
        const headers = document.querySelectorAll('#wlTopQueriesTable th.sortable');
        headers.forEach(h => {
            const col = h.dataset.sort;
            
            // Set initial indicator if this is the active sort
            if (col === this.sortKey) {
                h.dataset.dir = this.sortDir;
                h.innerHTML = h.innerHTML.split(' <i')[0] + ` <i class="fa-solid fa-sort-${this.sortDir === 'desc' ? 'down' : 'up'} ms-1" style="font-size:0.7rem; opacity:0.7;"></i>`;
            }

            h.onclick = () => {
                const currentDir = (this.sortKey === col) ? this.sortDir : '';
                const newDir = currentDir === 'desc' ? 'asc' : 'desc';
                
                this.sortKey = col;
                this.sortDir = newDir;

                // Clear other headers
                headers.forEach(other => {
                    other.dataset.dir = '';
                    other.innerHTML = other.innerHTML.split(' <i')[0];
                });

                h.dataset.dir = newDir;
                h.innerHTML = h.innerHTML.split(' <i')[0] + ` <i class="fa-solid fa-sort-${newDir === 'desc' ? 'down' : 'up'} ms-1" style="font-size:0.7rem; opacity:0.7;"></i>`;

                this.sortOffenders(this.sortKey, this.sortDir);
                this.renderTopOffenders();
            };
        });
    },

    sortOffenders(col, dir) {
        this.offenders.sort((a, b) => {
            let valA = a[col];
            let valB = b[col];

            if (col === 'last_seen') {
                valA = new Date(valA).getTime();
                valB = new Date(valB).getTime();
            }

            if (valA < valB) return dir === 'desc' ? 1 : -1;
            if (valA > valB) return dir === 'desc' ? -1 : 1;
            return 0;
        });
    },

    renderTopOffenders() {
        const body = document.getElementById('wlTopQueriesBody');
        if (!body) return;
        body.innerHTML = '';

        this.offenders.forEach((q, idx) => {
            const tr = document.createElement('tr');
            // Cache query for modal details
            const cacheKey = `wl_offender_${idx}`;
            if (!window.appState.queryCache) window.appState.queryCache = {};
            window.appState.queryCache[cacheKey] = { 
                text: q.query_text, 
                query_hash: q.query_hash, 
                database_name: q.database_name 
            };

            tr.innerHTML = `
                <td class="text-muted" style="font-size:0.7rem;">${q.last_seen ? new Date(q.last_seen).toLocaleString() : '--'}</td>
                <td class="query-column" style="cursor:pointer;" data-action="show-query-modal-direct" data-key="${cacheKey}" data-fn="showQueryStoreQueryModal">
                    <span class="query-text-truncated" title="Click to view full SQL statement">${this.escapeHtml(q.query_text)}</span>
                </td>
                <td class="text-end">${formatters.number(q.total_executions)}</td>
                <td class="text-end"><b class="text-rose">${formatters.number(q.avg_cpu_ms, 1)}</b></td>
                <td class="text-end">${formatters.compactNumber(q.total_reads)}</td>
                <td><span class="badge badge-outline" style="font-size:0.6rem;">${q.database_name}</span></td>
                <td><span class="text-info" style="font-size:0.7rem;">${this.escapeHtml(q.program_name || 'unknown')}</span></td>
                <td><span class="text-warning" style="font-size:0.7rem;">${this.escapeHtml(q.login_name || 'unknown')}</span></td>
            `;
            body.appendChild(tr);
        });
    },

    renderChart(id, config, tooltipTitle, doubleY = false) {
        const ctx = document.getElementById(id);
        if (!ctx) return;

        if (this.charts[id]) {
            this.charts[id].destroy();
        }

        const options = {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 10 } } },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    callbacks: {
                        title: (items) => {
                            const date = new Date(items[0].parsed.x);
                            return tooltipTitle + ' | ' + date.toLocaleTimeString();
                        }
                    }
                }
            },
            scales: {
                x: { type: 'time', time: { unit: 'minute' }, grid: { display: false }, ticks: { font: { size: 9 } } },
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { font: { size: 9 } } }
            }
        };

        if (doubleY) {
            options.scales.y1 = {
                type: 'linear',
                display: true,
                position: 'right',
                grid: { drawOnChartArea: false },
                ticks: { font: { size: 9 } }
            };
        }

        this.charts[id] = new Chart(ctx, {
            type: config.type,
            data: {
                labels: config.labels,
                datasets: config.datasets
            },
            options: options
        });
    },

    formatDateForApi(localDateTime) {
        if (!localDateTime) return '';
        const d = new Date(localDateTime);
        return d.toISOString();
    },

    escapeHtml(unsafe) {
        return unsafe
             .replace(/&/g, "&amp;")
             .replace(/</g, "&lt;")
             .replace(/>/g, "&gt;")
             .replace(/"/g, "&quot;")
             .replace(/'/g, "&#039;");
    }
};

window.wlRefreshAll = () => sqlserverWorkload.refreshAll();
