/*
 * SQL Optima — Enhanced PostgreSQL Memory Intelligence Dashboard
 */

window.PgMemoryView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: '', type: 'postgres'};
    if (!inst.name || inst.type !== 'postgres') {
        window.routerOutlet.innerHTML = '<div class="p-4 text-warning">Please select a PostgreSQL instance first.</div>';
        return;
    }
    const dbName = window.appState.currentDatabase || 'all';

    window.appState.activeViewId = 'pg-memory';
    const dashTitle = "Postgres Memory Intelligence";
    window.routerOutlet.innerHTML = await window.loadTemplate('/pages/pg_memory.html', { inst, dbName, dashTitle });
    window.initPageTimePicker();

    // Tab switching logic
    setTimeout(() => {
        document.querySelectorAll('.cockpit-tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.cockpit-tab-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                const tab = btn.getAttribute('data-tab');
                document.querySelectorAll('.tab-pane').forEach(p => p.style.display = 'none');
                const target = document.getElementById(`cockpit-tab-${tab}`);
                if (target) target.style.display = 'block';
            });
        });
    }, 100);

    await initPgMemoryCockpit(inst.name);

    if (window.pgMemoryInterval) clearInterval(window.pgMemoryInterval);
    window.pgMemoryInterval = setInterval(() => {
        if (window.appState.activeViewId === 'pg-memory') {
            initPgMemoryCockpit(inst.name);
        } else {
            clearInterval(window.pgMemoryInterval);
        }
    }, 30000);
};

async function updatePgMemoryHeader(instName) {
    try {
        const snapshotResp = await window.apiClient.authenticatedFetch(`/api/postgres/server-info?instance=${encodeURIComponent(instName)}`);
        if (snapshotResp.ok) {
            const s = await snapshotResp.json();
            const hs = s.health_score || 0;
            const healthColor = hs > 80 ? 'success' : hs > 60 ? 'warning' : 'danger';
            const hBadge = document.getElementById('health-score-value'); // Matches template
            if (hBadge) {
                hBadge.textContent = hs;
                hBadge.style.color = `var(--${healthColor})`;
            }
            const hLabel = document.getElementById('health-status-label');
            if (hLabel) {
                hLabel.textContent = hs > 80 ? 'HEALTHY' : hs > 60 ? 'WARNING' : 'CRITICAL';
                hLabel.style.color = `var(--${healthColor})`;
            }
        }
    } catch (e) { console.error("PG Memory header fetch failed:", e); }
}

async function initPgMemoryCockpit(instanceName) {
    window.currentCharts = window.currentCharts || {};
    let fromTs = window.appState.fromTs;
    let toTs = window.appState.toTs;

    if (!fromTs || !toTs) {
        const now = new Date();
        const oneHourAgo = new Date(now.getTime() - (60 * 60 * 1000));
        fromTs = oneHourAgo.toISOString();
        toTs = now.toISOString();
    }

    // Ensure we send ISO strings (UTC) to the API to avoid timezone skew
    if (fromTs && fromTs.includes('T') && !fromTs.endsWith('Z')) {
        fromTs = new Date(fromTs).toISOString();
    }
    if (toTs && toTs.includes('T') && !toTs.endsWith('Z')) {
        toTs = new Date(toTs).toISOString();
    }

    updatePgMemoryHeader(instanceName);

    try {
        const url = `/api/postgres/memory/intelligence?instance=${encodeURIComponent(instanceName)}&from=${fromTs}&to=${toTs}`;
        const response = await window.apiClient.authenticatedFetch(url);
        if (!response.ok) {
            const errBody = await response.json().catch(() => ({}));
            throw new Error(errBody.error || `HTTP ${response.status}`);
        }
        const data = await response.json();
        renderPgMemoryCockpit(data);
    } catch (e) {
        console.error("PG Memory Cockpit fetch failed:", e);
        _showMemoryError(e.message);
    }
}

function _showMemoryError(msg) {
    const kpiIds = ['os-memory-pct','pg-rss-mb','cache-hit-pct','swap-used-mb','temp-spill-rate','health-score-value'];
    kpiIds.forEach(id => { const el = document.getElementById(id); if (el) { el.textContent = 'N/A'; el.style.color = 'var(--text-muted)'; } });
    const notice = document.getElementById('memory-data-notice');
    if (notice) { notice.textContent = `Unable to load memory data: ${msg}`; notice.style.display = 'block'; }
    const tbody = document.getElementById('pg-memory-raw-tbody');
    if (tbody) tbody.innerHTML = `<tr><td colspan="6" class="text-center text-danger">${msg}</td></tr>`;
}

function renderPgMemoryCockpit(data) {
    const series = data.time_series || [];
    const components = data.components || {};
    const osConfigured = data.os_collector_configured;
    
    const notice = document.getElementById('memory-data-notice');
    if (notice) notice.style.display = 'none';

    // Update KPIs with latest available point
    if (series.length > 0) {
        const latest = series[series.length - 1];
        const setT = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
        const hasHostData = latest.total_mem_mb > 0;
        
        if (osConfigured || hasHostData) {
            setT('os-memory-pct', (latest.memory_pressure_percent || 0).toFixed(1) + '%');
            setT('os-memory-raw', `${((latest.used_mb || 0)/1024).toFixed(1)} / ${((latest.total_mem_mb || 0)/1024).toFixed(1)} GB`);
            setT('swap-used-mb', (latest.swap_used_mb || 0).toLocaleString() + ' MB');
        } else {
            setT('os-memory-pct', '-- %');
            setT('os-memory-raw', 'N/A');
            setT('swap-used-mb', 'N/A');
        }
        
        setT('pg-rss-mb', (latest.postgres_rss_mb || 0).toLocaleString() + ' MB');
        setT('pg-rss-pct', (osConfigured || hasHostData) ? (latest.pg_memory_percent || 0).toFixed(1) + '% of Host' : 'Host RAM unknown');
        setT('cache-hit-pct', (latest.cache_hit_ratio || 0).toFixed(1) + '%');
        setT('temp-spill-rate', (latest.temp_spill_rate_mb_s || 0).toFixed(2) + ' MB/s');

        // Charts
        renderHostPgTrendChart(series, osConfigured || hasHostData);
        renderCacheEfficiencyChart(series);
        updatePgMemoryTable(series);
        updateAdvisorContent(latest, components);
    } else {
        const noDataMsg = 'No memory data collected yet for this instance. Collection runs on the next cycle (typically within 1 minute).';
        if (notice) { notice.textContent = noDataMsg; notice.style.display = 'block'; }
        const tbody = document.getElementById('pg-memory-raw-tbody');
        if (tbody) tbody.innerHTML = `<tr><td colspan="6" class="text-center text-muted">${noDataMsg}</td></tr>`;
    }

    // Always render components if available
    renderPgComponentsChart(components);
}

const sharedOpts = {
    responsive: true, maintainAspectRatio: false,
    plugins: { legend: { display: false } },
    scales: { x: { grid: { display: false }, ticks: { font: { size: 9 }, color: '#64748b' } }, y: { grid: { color: 'rgba(255,255,255,0.03)' }, ticks: { font: { size: 9 }, color: '#64748b' } } }
};

function renderHostPgTrendChart(series, osConfigured) {
    const ctx = document.getElementById('hostPgTrendChart')?.getContext('2d');
    if (!ctx) return;
    if (window.currentCharts.hostPgTrend) window.currentCharts.hostPgTrend.destroy();
    
    const datasets = [
        { label: 'Postgres RSS (MB)', data: series.map(s => s.postgres_rss_mb), borderColor: '#10b981', backgroundColor: 'rgba(16, 185, 129, 0.1)', fill: true, tension: 0.3, pointRadius: 0 }
    ];
    if (osConfigured) {
        datasets.push({ label: 'OS Used %', data: series.map(s => s.memory_pressure_percent), borderColor: '#3b82f6', tension: 0.3, pointRadius: 0, yAxisID: 'y1' });
    }

    window.currentCharts.hostPgTrend = new Chart(ctx, {
        type: 'line',
        data: { labels: series.map(s => new Date(s.ts).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'})), datasets },
        options: { ...sharedOpts, plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 10 } } } }, scales: { ...sharedOpts.scales, y1: { display: osConfigured, position: 'right', max: 100, min: 0, grid: { display: false } } } }
    });
}

function renderCacheEfficiencyChart(series) {
    const ctx = document.getElementById('cacheEfficiencyChart')?.getContext('2d');
    if (!ctx) return;
    if (window.currentCharts.cacheEfficiency) window.currentCharts.cacheEfficiency.destroy();
    window.currentCharts.cacheEfficiency = new Chart(ctx, {
        type: 'line',
        data: { labels: series.map(s => new Date(s.ts).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'})), datasets: [{ label: 'Hit Ratio %', data: series.map(s => s.cache_hit_ratio), borderColor: '#8b5cf6', backgroundColor: 'rgba(139, 92, 246, 0.05)', fill: true, tension: 0.3, pointRadius: 0 }] },
        options: { ...sharedOpts, scales: { ...sharedOpts.scales, y: { ...sharedOpts.scales.y, min: 90, max: 100 } } }
    });
}

function renderPgComponentsChart(comp) {
    const ctx = document.getElementById('pgComponentsChart')?.getContext('2d');
    if (!ctx) return;
    if (window.currentCharts.pgComp) window.currentCharts.pgComp.destroy();
    const data = [comp.shared_buffers_mb||0, comp.work_mem_mb||0, comp.maintenance_work_mem_mb||0, comp.wal_buffers_mb||0];
    window.currentCharts.pgComp = new Chart(ctx, {
        type: 'doughnut',
        data: { labels: ['Shared Buffers', 'work_mem', 'maint_work_mem', 'WAL'], datasets: [{ data, backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6'], borderWidth: 0 }] },
        options: { responsive: true, maintainAspectRatio: false, cutout: '75%', plugins: { legend: { position: 'right', labels: { boxWidth: 10, font: { size: 10 } } } } }
    });
    setT('guc-shared-buffers', (comp.shared_buffers_mb || 0) + ' MB');
    setT('guc-work-mem', (comp.work_mem_mb || 0) + ' MB');
    setT('guc-maint-mem', (comp.maintenance_work_mem_mb || 0) + ' MB');
    setT('guc-eff-cache', (comp.effective_cache_size_mb || 0) + ' MB');
}

function updatePgMemoryTable(series) {
    const tbody = document.getElementById('pg-memory-raw-tbody');
    if (!tbody) return;
    tbody.innerHTML = series.slice().reverse().slice(0, 50).map(s => `
        <tr><td>${new Date(s.ts).toLocaleTimeString()}</td><td>${(s.memory_pressure_percent||0).toFixed(1)}%</td><td>${(s.postgres_rss_mb||0).toLocaleString()}</td><td>${(s.swap_used_mb||0).toLocaleString()}</td><td>${(s.temp_spill_rate_mb_s||0).toFixed(3)}</td><td>${(s.cache_hit_ratio||0).toFixed(2)}%</td></tr>
    `).join('');
}

function updateAdvisorContent(latest, components) {
    const memAdvisor = document.getElementById('mem-advisor-content');
    const workMemAdvisor = document.getElementById('workmem-advisor-content');
    const connAdvisor = document.getElementById('conn-advisor-content');

    if (memAdvisor) {
        let html = '<ul class="p-0 m-0" style="list-style:none;">';
        if (latest.cache_hit_ratio < 95) html += '<li class="text-warning"><i class="fa-solid fa-circle-exclamation"></i> Cache hit ratio low. Consider increasing shared_buffers.</li>';
        else html += '<li class="text-success"><i class="fa-solid fa-circle-check"></i> Shared buffer efficiency is optimal.</li>';
        if ((components.shared_buffers_mb || 0) < 128) html += '<li class="text-warning mt-1"><i class="fa-solid fa-triangle-exclamation"></i> shared_buffers is very small.</li>';
        html += '</ul>';
        memAdvisor.innerHTML = html;
    }

    if (workMemAdvisor) {
        let html = '<ul class="p-0 m-0" style="list-style:none;">';
        if (latest.temp_spill_rate_mb_s > 0.1) {
            html += '<li class="text-danger"><i class="fa-solid fa-bolt"></i> High disk spill rate detected. Queries are struggling with current work_mem.</li>';
            html += `<li class="text-accent mt-1"><i class="fa-solid fa-lightbulb"></i> Try increasing work_mem to ${((components.work_mem_mb || 4)*2)} MB.</li>`;
        } else if (latest.temp_spill_rate_mb_s > 0) {
            html += '<li class="text-warning"><i class="fa-solid fa-circle-exclamation"></i> Minor spills detected. Monitor complex sort/join queries.</li>';
        } else {
            html += '<li class="text-success"><i class="fa-solid fa-circle-check"></i> No disk spills detected. work_mem is sufficient for current workload.</li>';
        }
        html += '</ul>';
        workMemAdvisor.innerHTML = html;
    }

    if (connAdvisor) {
        // Estimate based on typical 10MB per connection if not available
        const estConnMem = (latest.active_connections || 20) * 10;
        let html = '<ul class="p-0 m-0" style="list-style:none;">';
        html += `<li><i class="fa-solid fa-info-circle text-info"></i> Est. footprint: ~${estConnMem} MB for ${latest.active_connections || 'N/A'} conns.</li>`;
        if ((latest.active_connections || 0) > 200) {
            html += '<li class="text-warning mt-1"><i class="fa-solid fa-users-slash"></i> High connection count. Consider a connection pooler like PgBouncer.</li>';
        } else {
            html += '<li class="text-success mt-1"><i class="fa-solid fa-users"></i> Connection overhead is within healthy limits.</li>';
        }
        html += '</ul>';
        connAdvisor.innerHTML = html;
    }
}

function setT(id, val) { const el = document.getElementById(id); if (el) el.textContent = val; }
