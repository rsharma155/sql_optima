// SQL Optima — UI logic for the SQL Server Workload Observability Dashboard.
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
        this.instance = window.appState?.config?.instances?.[window.appState?.currentInstanceIdx]?.name || '';
        const nameEl = document.getElementById('wlInstanceName');
        if (nameEl) nameEl.textContent = this.instance;

        this.setDefaultRange();

        document.getElementById('wlApplyRange')?.addEventListener('click', () => {
            this.isManualRange = true;
            this.refreshAll();
        });

        // Tab Switcher
        const tabs = document.querySelectorAll('.qa-tab-btn');
        tabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                e.preventDefault();
                tabs.forEach(t => t.classList.remove('active'));
                document.querySelectorAll('.tab-pane').forEach(p => p.classList.remove('show', 'active'));
                tab.classList.add('active');
                const targetId = tab.getAttribute('data-tab');
                const targetPane = document.getElementById(targetId);
                if (targetPane) {
                    targetPane.classList.add('show', 'active');
                    const canvas = targetPane.querySelector('canvas');
                    if (canvas && this.charts[canvas.id]) this.charts[canvas.id].resize();
                }
            });
        });

        const dynInterval = window.collectorConfig ? window.collectorConfig.getInterval("SQL Server Query Analysis", 30000) : 30000;
        if (this.refreshInterval) clearInterval(this.refreshInterval);
        this.refreshInterval = setInterval(() => {
            if (!this.isManualRange) this.setDefaultRange();
            this.refreshAll();
        }, dynInterval);
        
        await this.refreshAll();
    },

    setDefaultRange() {
        const now = new Date();
        const oneHourAgo = new Date(now.getTime() - (60 * 60 * 1000));
        const fromInput = document.getElementById('wlFrom');
        const toInput = document.getElementById('wlTo');
        if (fromInput) fromInput.value = this.formatToLocalDatetime(oneHourAgo);
        if (toInput) toInput.value = this.formatToLocalDatetime(now);
    },

    formatToLocalDatetime(d) {
        const pad = n => String(n).padStart(2, '0');
        return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    },

    async refreshAll() {
        if (!this.instance) return;
        const from = document.getElementById('wlFrom')?.value;
        const to = document.getElementById('wlTo')?.value;
        if (!from || !to) return;

        this.setLoading(true);
        try {
            await Promise.all([
                this.loadSummary(from, to),
                this.loadTrends(from, to),
                this.loadTopOffenders(from, to),
                this.loadServerProperties(),
                this.loadSchedulerStats(),
                this.loadQaSummary(from, to),
                this.loadAppAnalysis(from, to),
                this.loadUserAnalysis(from, to)
            ]);
        } catch (err) {
            console.error('[Workload] Refresh failed:', err);
        } finally {
            this.setLoading(false);
        }
    },

    setLoading(isLoading) {
        const overlay = document.getElementById('wlLoadingOverlay');
        if (overlay) overlay.style.display = isLoading ? 'flex' : 'none';
    },

    async loadSummary(from, to) {
        const data = await api.get(`/api/sqlserver/workload/summary?instance=${this.instance}&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}`) || {};
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('wl-kpi-cpu', formatters.number((data.total_cpu_ms || 0) / 1000, 1));
        set('wl-kpi-execs', formatters.compactNumber(data.total_executions || 0));
        set('wl-kpi-reads', formatters.compactNumber(data.total_logical_reads || 0));
        set('wl-kpi-mem', formatters.bytes((data.max_memory_grant_kb || 0) * 1024));
    },

    async loadServerProperties() {
        const data = await api.get(`/api/sqlserver/server-properties?instance=${encodeURIComponent(this.instance)}`) || {};
        const p = data.server_properties || {};
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('wl-prop-cpu', p.cpu_count || '--');
        set('wl-prop-sockets', p.socket_count || '--');
        const htEl = document.getElementById('wl-prop-ht');
        if (htEl) htEl.textContent = p.hyperthread_enabled ? 'YES' : 'NO';
        set('wl-prop-numa', p.numa_nodes || '--');
        set('wl-prop-mem', p.physical_memory_gb ? p.physical_memory_gb.toFixed(0) : '--');
    },

    async loadSchedulerStats() {
        const data = await api.get(`/api/sqlserver/cpu-scheduler-stats?instance=${encodeURIComponent(this.instance)}&limit=50`) || {};
        const all = data.cpu_scheduler_stats || [];
        if (all.length === 0) return;
        const s = all[0];
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('wl-sched-workers', `${s.total_current_workers_count || 0}/${s.max_workers_count || 0}`);
        set('wl-sched-runnable', s.total_runnable_tasks_count || 0);
        set('wl-sched-queue', s.total_work_queue_count || 0);
        set('wl-sched-mem', (s.total_physical_memory_kb > 0 ? (((s.total_physical_memory_kb - s.available_physical_memory_kb) / s.total_physical_memory_kb) * 100).toFixed(1) : 0) + '%');
        const warnEl = document.getElementById('wl-sched-warnings');
        if (warnEl) warnEl.textContent = s.physical_memory_pressure_warning ? 'PRESSURE' : 'Healthy';

        this.renderSchedulerHistory(all);
        const body = document.getElementById('wlSchedulerStatsBody');
        if (body) {
            body.innerHTML = all.slice(0, 20).map(x => {
                let status = x.system_memory_state_desc || 'Healthy';
                if (status === 'Available physical memory is high') status = 'Healthy';
                if (status === 'Physical memory usage is steady') status = 'Stable';
                
                return `
                    <tr>
                        <td>${new Date(x.capture_timestamp).toLocaleTimeString()}</td>
                        <td>${x.total_current_workers_count}</td>
                        <td>${x.total_runnable_tasks_count}</td>
                        <td>${x.total_work_queue_count}</td>
                        <td><span class="badge badge-${x.physical_memory_pressure_warning ? 'danger' : 'success'}">${status}</span></td>
                    </tr>
                `;
            }).join('');
        }
    },

    renderSchedulerHistory(all) {
        const labels = all.slice().reverse().map(s => new Date(s.capture_timestamp));
        this.renderChart('wlCpuSchedulerHistoryChart', {
            type: 'line',
            labels: labels,
            datasets: [
                { label: 'Workers', data: all.slice().reverse().map(s => s.total_current_workers_count), borderColor: '#3b82f6', tension: 0.3, pointRadius: 0 },
                { label: 'Runnable', data: all.slice().reverse().map(s => s.total_runnable_tasks_count), borderColor: '#f59e0b', tension: 0.3, pointRadius: 0 },
                { label: 'Queue', data: all.slice().reverse().map(s => s.total_work_queue_count), borderColor: '#ef4444', tension: 0.3, pointRadius: 0 }
            ]
        });
    },

    async loadQaSummary(from, to) {
        const data = await api.get(`/api/sqlserver/query-analysis/summary?instance=${encodeURIComponent(this.instance)}&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}&exclude_system=true&database=all`) || {};
        const set = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        set('kpi-cpu-share', data.top_10_cpu_share_pct ? data.top_10_cpu_share_pct.toFixed(1) + '%' : '--');
        set('kpi-regressions', data.regressions_24h || 0);
    },

    async loadTrends(from, to) {
        const resp = await api.get(`/api/sqlserver/workload/trends?instance=${this.instance}&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}`) || {};
        const data = resp.trends || [];
        const labels = data.map(p => new Date(p.timestamp));
        this.renderChart('wlCpuTrendChart', { type: 'line', labels, datasets: [{ label: 'CPU', data: data.map(p => p.cpu_ms / 1000), borderColor: '#ef4444', fill: true, backgroundColor: 'rgba(239, 68, 68, 0.1)', tension: 0.3, pointRadius: 0 }] });
        this.renderChart('wlExecTrendChart', { type: 'line', labels, datasets: [{ label: 'Execs', data: data.map(p => p.executions), borderColor: '#3b82f6', fill: true, backgroundColor: 'rgba(59, 130, 246, 0.1)', tension: 0.3, pointRadius: 0 }] });
        this.renderChart('wlReadTrendChart', { type: 'bar', labels, datasets: [{ label: 'Reads', data: data.map(p => p.logical_reads), backgroundColor: '#f59e0b', borderRadius: 4 }] });
        this.renderChart('wlEfficiencyChart', { type: 'line', labels, datasets: [{ label: 'CPU/Exec (ms)', data: data.map(p => p.avg_cpu_ms), borderColor: '#8b5cf6', tension: 0.3, pointRadius: 0 }] });
        this.renderChart('wlPressureChart', { type: 'line', labels, datasets: [{ label: 'Peak Mem (MB)', data: data.map(p => p.max_grant_kb / 1024), borderColor: '#10b981', tension: 0.3, pointRadius: 0 }] });
    },

    async loadTopOffenders(from, to) {
        const resp = await api.get(`/api/sqlserver/workload/top-queries?instance=${this.instance}&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}&limit=20`) || {};
        const body = document.getElementById('wlTopQueriesBody');
        if (!body) return;
        body.innerHTML = (resp.top_offenders || []).map(q => `
            <tr>
                <td class="text-muted small">${q.last_seen ? new Date(q.last_seen).toLocaleString() : '--'}</td>
                <td class="query-column small" style="cursor:pointer;" data-action="call" data-fn="showQueryModal" data-arg="${window.escapeHtml(q.query_text.replace(/'/g, "\\'"))}">
                    <span class="query-text-truncated">${window.escapeHtml(q.query_text.substring(0, 100))}...</span>
                </td>
                <td class="text-end">${q.total_executions.toLocaleString()}</td>
                <td class="text-end text-rose font-bold">${q.avg_cpu_ms.toFixed(1)}</td>
                <td class="text-end">${formatters.compactNumber(q.total_reads)}</td>
                <td><span class="badge badge-outline">${q.database_name}</span></td>
                <td class="small text-info">${this.escapeHtml(q.program_name || 'unknown')}</td>
                <td class="small text-warning">${this.escapeHtml(q.login_name || 'unknown')}</td>
            </tr>
        `).join('') || '<tr><td colspan="8" class="text-center text-muted">No query data found</td></tr>';
    },

    async loadAppAnalysis(from, to) {
        try {
            const [timelineResp, topResp] = await Promise.all([
                api.get(`/api/sqlserver/workload/app-load?instance=${this.instance}&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}`),
                api.get(`/api/sqlserver/workload/top-apps?instance=${this.instance}&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}&limit=10`)
            ]);
            
            const raw = (timelineResp && timelineResp.app_load) || [];
            const appGroups = {};
            const labels = [...new Set(raw.map(r => r.bucket))].sort().map(b => new Date(b));
            raw.forEach(r => {
                if (!appGroups[r.app_name]) appGroups[r.app_name] = {};
                appGroups[r.app_name][r.bucket] = r.cpu_ms / 1000;
            });
            const datasets = Object.keys(appGroups).slice(0, 5).map((app, idx) => ({
                label: app, data: labels.map(l => appGroups[app][l.toISOString()] || 0), borderColor: charts.colors[idx % charts.colors.length], pointRadius: 0, tension: 0.3
            }));
            this.renderChart('wlAppLoadChart', { type: 'line', labels, datasets });

            const top = (topResp && topResp.top_apps) || [];
            this.renderChart('wlTopAppsChart', { type: 'bar', labels: top.map(x => x.app_name), datasets: [{ label: 'CPU sec', data: top.map(x => x.total_cpu_ms / 1000), backgroundColor: 'rgba(59, 130, 246, 0.6)' }] });

            const totalCpu = top.reduce((a, b) => a + (b.total_cpu_ms || 0), 0);
            const body = document.getElementById('wlAppTableBody');
            if (body) {
                body.innerHTML = top.map(x => `
                    <tr><td>${this.escapeHtml(x.app_name)}</td><td>${((x.total_cpu_ms || 0)/1000).toFixed(1)}</td><td>${totalCpu > 0 ? (((x.total_cpu_ms || 0)/totalCpu)*100).toFixed(1) : 0}%</td></tr>
                `).join('');
            }
        } catch (e) { console.error(e); }
    },

    async loadUserAnalysis(from, to) {
        try {
            const [timelineResp, topResp] = await Promise.all([
                api.get(`/api/sqlserver/workload/login-load?instance=${this.instance}&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}`),
                api.get(`/api/sqlserver/workload/top-logins?instance=${this.instance}&from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}&limit=10`)
            ]);
            
            const raw = (timelineResp && timelineResp.login_load) || [];
            const groups = {};
            const labels = [...new Set(raw.map(r => r.bucket))].sort().map(b => new Date(b));
            raw.forEach(r => {
                if (!groups[r.login_name]) groups[r.login_name] = {};
                groups[r.login_name][r.bucket] = r.cpu_ms / 1000;
            });
            const datasets = Object.keys(groups).slice(0, 5).map((u, idx) => ({
                label: u, data: labels.map(l => groups[u][l.toISOString()] || 0), borderColor: charts.colors[(idx+3) % charts.colors.length], pointRadius: 0, tension: 0.3
            }));
            this.renderChart('wlLoginLoadChart', { type: 'line', labels, datasets });

            const top = (topResp && topResp.top_logins) || [];
            this.renderChart('wlTopLoginsChart', { type: 'bar', labels: top.map(x => x.login_name), datasets: [{ label: 'CPU sec', data: top.map(x => x.total_cpu_ms / 1000), backgroundColor: 'rgba(16, 185, 129, 0.6)' }] });

            const totalCpu = top.reduce((a, b) => a + (b.total_cpu_ms || 0), 0);
            const body = document.getElementById('wlLoginTableBody');
            if (body) {
                body.innerHTML = top.map(x => `
                    <tr><td>${this.escapeHtml(x.login_name)}</td><td>${((x.total_cpu_ms || 0)/1000).toFixed(1)}</td><td>${totalCpu > 0 ? (((x.total_cpu_ms || 0)/totalCpu)*100).toFixed(1) : 0}%</td></tr>
                `).join('');
            }
        } catch (e) { console.error(e); }
    },

    renderChart(id, config) {
        const ctx = document.getElementById(id);
        if (!ctx) return;
        if (this.charts[id]) this.charts[id].destroy();
        // Use time axis only when labels are Date objects; use category axis otherwise.
        const hasDateLabels = Array.isArray(config.labels) && config.labels.length > 0 && config.labels[0] instanceof Date;
        const xScale = hasDateLabels
            ? { type: 'time', display: true, grid: { display: false }, ticks: { font: { size: 8 }, color: '#64748b' } }
            : { display: true, grid: { display: false }, ticks: { font: { size: 8 }, color: '#64748b', maxRotation: 30 } };
        this.charts[id] = new Chart(ctx, {
            type: config.type,
            data: { labels: config.labels, datasets: config.datasets },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 }, color: '#94a3b8' } } },
                scales: {
                    x: xScale,
                    y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.03)' }, ticks: { font: { size: 8 }, color: '#64748b' } }
                }
            }
        });
    },

    escapeHtml(unsafe) {
        if (unsafe === null || unsafe === undefined) return "";
        return String(unsafe).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
    }
};

window.wlRefreshAll = () => sqlserverWorkload.refreshAll();
