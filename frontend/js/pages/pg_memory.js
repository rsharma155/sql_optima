/*
 * SQL Optima — PostgreSQL Memory Intelligence Dashboard
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

    // Tab switching
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
    window.pgMemoryInterval = window.registerInterval(() => {
        if (window.appState.activeViewId === 'pg-memory') {
            initPgMemoryCockpit(inst.name);
        } else {
            clearInterval(window.pgMemoryInterval);
        }
    }, 30000);
};

async function updatePgMemoryHeader(instName) {
    try {
        const resp = await window.apiClient.authenticatedFetch(`/api/postgres/server-info?instance=${encodeURIComponent(instName)}`);
        if (!resp.ok) return;
        const s = await resp.json();
        const hs = s.health_score || 0;
        const healthColor = hs > 80 ? 'success' : hs > 60 ? 'warning' : 'danger';
        const hBadge = document.getElementById('health-score-value');
        if (hBadge) { hBadge.textContent = hs; hBadge.style.color = `var(--${healthColor})`; }
        const hLabel = document.getElementById('health-status-label');
        if (hLabel) { hLabel.textContent = hs > 80 ? 'HEALTHY' : hs > 60 ? 'WARNING' : 'CRITICAL'; hLabel.style.color = `var(--${healthColor})`; }
    } catch (e) { console.error("PG Memory header fetch failed:", e); }
}

async function initPgMemoryCockpit(instanceName) {
    window.currentCharts = window.currentCharts || {};
    let fromTs = window.appState.fromTs;
    let toTs = window.appState.toTs;

    if (!fromTs || !toTs) {
        const now = new Date();
        fromTs = new Date(now.getTime() - 3600000).toISOString();
        toTs = now.toISOString();
    }
    if (fromTs && !fromTs.endsWith('Z')) fromTs = new Date(fromTs).toISOString();
    if (toTs && !toTs.endsWith('Z')) toTs = new Date(toTs).toISOString();

    updatePgMemoryHeader(instanceName);

    try {
        const url = `/api/postgres/memory/intelligence?instance=${encodeURIComponent(instanceName)}&from=${fromTs}&to=${toTs}`;
        const response = await window.apiClient.authenticatedFetch(url);
        if (!response.ok) {
            const errBody = await response.json().catch(() => ({}));
            throw new Error(errBody.error || `HTTP ${response.status}`);
        }
        const data = await response.json();
        renderPgMemoryCockpit(data, instanceName);
    } catch (e) {
        console.error("PG Memory Cockpit fetch failed:", e);
        _showMemoryError(e.message);
    }
}

function _showMemoryError(msg) {
    const notice = document.getElementById('memory-data-notice');
    if (notice) { notice.textContent = `Unable to load memory data: ${msg}`; notice.style.display = 'block'; }
    const tbody = document.getElementById('pg-memory-raw-tbody');
    if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="text-center text-danger">${window.escapeHtml(msg)}</td></tr>`;
}

function updateOsCollectorSetupMount(instanceName, osConfigured) {
    const mount = document.getElementById('os-collector-setup-mount');
    if (!mount || !window.mountOsCollectorSetupPanel) return;
    if (osConfigured || (window.isOsCollectorPromptDismissed && window.isOsCollectorPromptDismissed())) {
        mount.style.display = 'none';
        mount.innerHTML = '';
        return;
    }
    mount.style.display = 'block';
    if (!mount.querySelector('.os-collector-setup-panel')) {
        window.mountOsCollectorSetupPanel(mount, {
            instanceName,
            statusId: 'os-collector-memory-status',
            compact: true
        });
    } else if (window.refreshOsCollectorSetupStatus) {
        const slot = document.getElementById('os-collector-memory-status');
        window.refreshOsCollectorSetupStatus(slot, instanceName);
    }
}

function renderPgMemoryCockpit(data, instanceName) {
    const series = data.time_series || [];
    const components = data.components || {};
    const osConfigured = !!data.os_collector_configured;

    const notice = document.getElementById('memory-data-notice');
    if (notice) notice.style.display = 'none';

    updateOsCollectorSetupMount(instanceName, osConfigured);

    // Show/hide OS collector badge
    const osBadge = document.getElementById('os-collector-badge');
    if (osBadge) osBadge.style.display = osConfigured ? 'inline-block' : 'none';

    if (series.length === 0) {
        const hasGucs = (components.shared_buffers_mb || components.work_mem_mb || components.max_connections);
        const noDataMsg = hasGucs
            ? 'No time-series samples in the selected range. Try widening the time window or wait for the next collector cycle (~1 minute).'
            : 'No memory data collected yet. Ensure the PostgreSQL instance is online and the Postgres Dashboard Snapshot collector is enabled (Settings → Collectors).';
        if (notice) { notice.textContent = noDataMsg; notice.style.display = 'block'; }
        const tbody = document.getElementById('pg-memory-raw-tbody');
        if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="text-center text-muted">${window.escapeHtml(noDataMsg)}</td></tr>`;
        renderPgComponentsChart(components);
        updateGucTable(components);
        return;
    }

    const latest = series[series.length - 1];

    // ── KPI: OS Memory (only meaningful with OS collector)
    if (osConfigured && (latest.total_mem_mb || 0) > 0) {
        _setKpi('os-memory-pct', (latest.memory_pressure_percent || 0).toFixed(1) + '%',
            _memPressureColor(latest.memory_pressure_percent));
        _setEl('os-memory-raw', `${((latest.used_mb || 0)/1024).toFixed(1)} / ${((latest.total_mem_mb || 0)/1024).toFixed(1)} GB`);
        _setKpi('swap-used-mb', (latest.swap_used_mb || 0).toLocaleString() + ' MB',
            (latest.swap_used_mb || 0) > 0 ? 'var(--danger)' : 'var(--success)');
        _setEl('swap-status', (latest.swap_used_mb || 0) > 0 ? 'Swap in use!' : 'No swap activity');
    } else {
        _setEl('os-memory-pct', 'N/A');
        _setEl('os-memory-raw', 'OS Collector Required');
        _setEl('swap-used-mb', 'N/A');
        _setEl('swap-status', 'OS Collector Required');
    }

    // ── KPI: Postgres RSS
    _setEl('pg-rss-mb', osConfigured && latest.postgres_rss_mb > 0
        ? (latest.postgres_rss_mb).toLocaleString() + ' MB'
        : 'N/A');
    _setEl('pg-rss-pct', osConfigured && latest.postgres_rss_mb > 0
        ? (latest.pg_memory_percent || 0).toFixed(1) + '% of Host RAM'
        : 'OS Collector Required');

    // ── KPI: Cache Hit Ratio
    const cacheHit = latest.cache_hit_ratio || 0;
    _setKpi('cache-hit-pct', cacheHit.toFixed(2) + '%', _cacheHitColor(cacheHit));

    // ── KPI: Active Connections
    const activeConn = latest.active_connections || 0;
    _setEl('active-conn-kpi', activeConn.toString());
    _setEl('conn-overhead-kpi', `~${(activeConn * 10).toLocaleString()} MB est. overhead`);

    // ── KPI: Temp Spill Rate
    const spill = latest.temp_spill_rate_mb_s || 0;
    _setKpi('temp-spill-rate', spill.toFixed(3) + ' MB/s', spill > 0.1 ? 'var(--danger)' : spill > 0 ? 'var(--warning)' : 'var(--success)');

    // ── Charts
    renderHostPgTrendChart(series, osConfigured);
    renderCacheEfficiencyChart(series);
    renderConnectionTrendChart(series);
    renderBgwriterChart(series);
    renderForecastChart(series, osConfigured);
    renderPgComponentsChart(components);
    updateGucTable(components);
    updatePgMemoryTable(series);
    updateAdvisorContent(latest, components, osConfigured);
}

// ── Color helpers ──────────────────────────────────────────────────────────

function _memPressureColor(pct) {
    if (pct >= 90) return 'var(--danger)';
    if (pct >= 80) return 'var(--warning)';
    return 'var(--success)';
}

function _cacheHitColor(pct) {
    if (pct < 95) return 'var(--danger)';
    if (pct < 99) return 'var(--warning)';
    return 'var(--success)';
}

function _setKpi(id, text, color) {
    const el = document.getElementById(id);
    if (!el) return;
    el.textContent = text;
    if (color) el.style.color = color;
}

function _setEl(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
}

// ── Shared chart options ───────────────────────────────────────────────────

const _baseOpts = {
    responsive: true, maintainAspectRatio: false,
    animation: false,
    plugins: { legend: { display: false } },
    scales: {
        x: { grid: { display: false }, ticks: { font: { size: 9 }, color: '#64748b', maxTicksLimit: 8 } },
        y: { grid: { color: 'rgba(255,255,255,0.04)' }, ticks: { font: { size: 9 }, color: '#64748b' } }
    }
};

function _labels(series) {
    return series.map(s => window.fmtChartTick ? window.fmtChartTick(s, { hour: '2-digit', minute: '2-digit' }) : '');
}

function _destroyAndCreate(key, ctx, config) {
    if (window.currentCharts[key]) window.currentCharts[key].destroy();
    window.currentCharts[key] = new Chart(ctx, config);
}

// ── Chart 1: Memory Load History (RSS + OS pressure) ─────────────────────

function renderHostPgTrendChart(series, osConfigured) {
    const ctx = document.getElementById('hostPgTrendChart')?.getContext('2d');
    if (!ctx) return;

    const datasets = [];

    if (osConfigured) {
        datasets.push({
            label: 'OS Memory Used %', yAxisID: 'yPct',
            data: series.map(s => (s.memory_pressure_percent || 0).toFixed(1)),
            borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.08)',
            fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1.5
        });
        datasets.push({
            label: 'PG RSS MB', yAxisID: 'yMB',
            data: series.map(s => s.postgres_rss_mb || 0),
            borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.08)',
            fill: false, tension: 0.3, pointRadius: 0, borderWidth: 1.5
        });
    } else {
        // Without OS collector, show PG-available metrics: cache hits and connections as a proxy
        datasets.push({
            label: 'Cache Hit %', yAxisID: 'yPct',
            data: series.map(s => (s.cache_hit_ratio || 0).toFixed(2)),
            borderColor: '#8b5cf6', backgroundColor: 'rgba(139,92,246,0.08)',
            fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1.5
        });
        document.getElementById('main-memory-trend-title') &&
            (document.getElementById('main-memory-trend-title').textContent = 'Cache Hit Ratio History (install OS Collector for host memory)');
    }

    _destroyAndCreate('hostPgTrend', ctx, {
        type: 'line',
        data: { labels: _labels(series), datasets },
        options: {
            ..._baseOpts,
            plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 } } } },
            scales: {
                x: _baseOpts.scales.x,
                yPct: { position: 'left', min: 0, max: 100, grid: { color: 'rgba(255,255,255,0.04)' }, ticks: { font: { size: 9 }, color: '#64748b', callback: v => v + '%' } },
                yMB: { display: osConfigured, position: 'right', grid: { display: false }, ticks: { font: { size: 9 }, color: '#64748b', callback: v => v + ' MB' } }
            }
        }
    });
}

// ── Chart 2: Cache Efficiency ─────────────────────────────────────────────

function renderCacheEfficiencyChart(series) {
    const ctx = document.getElementById('cacheEfficiencyChart')?.getContext('2d');
    if (!ctx) return;

    // Dynamically set Y axis min based on actual data (floor to nearest 0.5% below min)
    const values = series.map(s => s.cache_hit_ratio || 0).filter(v => v > 0);
    const minVal = values.length ? Math.max(85, Math.floor((Math.min(...values) - 0.5) * 2) / 2) : 90;

    _destroyAndCreate('cacheEfficiency', ctx, {
        type: 'line',
        data: {
            labels: _labels(series),
            datasets: [{
                label: 'Hit Ratio %',
                data: series.map(s => (s.cache_hit_ratio || 0).toFixed(3)),
                borderColor: '#8b5cf6', backgroundColor: 'rgba(139,92,246,0.07)',
                fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1.5
            }]
        },
        options: {
            ..._baseOpts,
            scales: {
                x: _baseOpts.scales.x,
                y: { ..._baseOpts.scales.y, min: minVal, max: 100,
                    ticks: { ..._baseOpts.scales.y.ticks, callback: v => v.toFixed(1) + '%' } }
            }
        }
    });
}

// ── Chart 3: Connection Trend ─────────────────────────────────────────────

function renderConnectionTrendChart(series) {
    const ctx = document.getElementById('connectionTrendChart')?.getContext('2d');
    if (!ctx) return;

    _destroyAndCreate('connTrend', ctx, {
        type: 'line',
        data: {
            labels: _labels(series),
            datasets: [
                {
                    label: 'Active', data: series.map(s => s.active_connections || 0),
                    borderColor: '#f59e0b', backgroundColor: 'rgba(245,158,11,0.1)',
                    fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1.5
                },
                {
                    label: 'Idle', data: series.map(s => s.idle_connections || 0),
                    borderColor: '#64748b', backgroundColor: 'rgba(100,116,139,0.06)',
                    fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1
                },
                {
                    label: 'Total', data: series.map(s => s.total_connections || 0),
                    borderColor: '#3b82f6', borderDash: [4, 3],
                    fill: false, tension: 0.3, pointRadius: 0, borderWidth: 1
                }
            ]
        },
        options: {
            ..._baseOpts,
            plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 } } } },
            scales: { x: _baseOpts.scales.x, y: { ..._baseOpts.scales.y, min: 0, ticks: { ..._baseOpts.scales.y.ticks, stepSize: 1 } } }
        }
    });
}

// ── Chart 4: BGWriter Buffer Allocation ──────────────────────────────────

function renderBgwriterChart(series) {
    const ctx = document.getElementById('bgwriterChart')?.getContext('2d');
    if (!ctx) return;

    // Compute deltas between adjacent points for each buffer type.
    // Positive delta = buffers written in that interval; reset to 0 on server restart.
    function deltas(key) {
        return series.map((s, i) => {
            if (i === 0) return 0;
            const d = (s[key] || 0) - (series[i - 1][key] || 0);
            return d < 0 ? 0 : d; // guard against counter reset
        });
    }

    const dCkpt = deltas('buffers_checkpoint');
    const dBackend = deltas('buffers_backend');
    const dClean = deltas('buffers_clean');
    const hasData = [...dCkpt, ...dBackend, ...dClean].some(v => v > 0);

    if (!hasData) {
        // Show a placeholder message inside the canvas area
        const parent = ctx.canvas.parentElement;
        if (parent && !parent.querySelector('.bgwriter-no-data')) {
            const msg = document.createElement('div');
            msg.className = 'bgwriter-no-data';
            msg.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;font-size:0.72rem;color:var(--text-muted);pointer-events:none;';
            msg.textContent = 'Accumulating BGWriter data…';
            parent.style.position = 'relative';
            parent.appendChild(msg);
        }
    }

    _destroyAndCreate('bgwriter', ctx, {
        type: 'bar',
        data: {
            labels: _labels(series),
            datasets: [
                { label: 'Checkpoint', data: dCkpt, backgroundColor: 'rgba(16,185,129,0.7)', stack: 'buf' },
                { label: 'BGWriter', data: dClean, backgroundColor: 'rgba(59,130,246,0.7)', stack: 'buf' },
                { label: 'Backend', data: dBackend, backgroundColor: 'rgba(239,68,68,0.7)', stack: 'buf' }
            ]
        },
        options: {
            ..._baseOpts,
            plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 } } },
                tooltip: { callbacks: { footer: items => items[0]?.dataset.label === 'Backend' && items[0].raw > 0 ? '⚠ Backend writes indicate checkpoint pressure' : '' } }
            },
            scales: { x: { ..._baseOpts.scales.x, stacked: true }, y: { ..._baseOpts.scales.y, stacked: true, ticks: { ..._baseOpts.scales.y.ticks, stepSize: 1 } } }
        }
    });
}

// ── Chart 5: Saturation Forecast ─────────────────────────────────────────

function renderForecastChart(series, osConfigured) {
    const ctx = document.getElementById('forecastChart')?.getContext('2d');
    if (!ctx) return;

    // Use cache_hit_ratio as the primary metric for the forecast (always available).
    // Invert it so rising = worsening: "cache miss %" = 100 - hit%.
    // Also project RSS trend if OS collector is active.
    const metricKey = 'cache_hit_ratio';
    const metricLabel = 'Cache Miss % (100 - hit ratio)';

    if (series.length < 3) {
        _setEl('forecast-summary', 'Need at least 3 data points to compute a trend. Refresh after a few collection cycles.');
        return;
    }

    const xs = series.map((s, i) => i); // use index as X
    const ys = series.map(s => parseFloat((100 - (s[metricKey] || 100)).toFixed(4)));

    const reg = _linearRegression(xs, ys);

    // Extend 2 hours forward (assuming ~1 min interval → 120 more points)
    const intervalMs = series.length > 1
        ? (new Date(series[series.length-1].ts) - new Date(series[0].ts)) / (series.length - 1)
        : 60000;
    const forecastSteps = Math.round(7200000 / intervalMs);
    const extendCount = Math.min(forecastSteps, 60); // cap at 60 points

    const allLabels = _labels(series);
    const allData = ys.map((v, i) => ({ x: i, y: v }));

    // Add projected points
    const projectedData = [];
    let lastTime = new Date(series[series.length - 1].ts);
    for (let i = 1; i <= extendCount; i++) {
        lastTime = new Date(lastTime.getTime() + intervalMs);
        const xi = series.length - 1 + i;
        projectedData.push({ x: xi, y: Math.max(0, reg.slope * xi + reg.intercept) });
        allLabels.push(lastTime.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
    }

    // Trend summary
    const currentMiss = ys[ys.length - 1];
    const projectedMiss = projectedData.length ? projectedData[projectedData.length - 1].y : currentMiss;
    const projectedHit = (100 - projectedMiss).toFixed(2);
    const trend = reg.slope > 0.001 ? '↑ worsening' : reg.slope < -0.001 ? '↓ improving' : '→ stable';
    const summaryEl = document.getElementById('forecast-summary');
    if (summaryEl) {
        const color = projectedHit < 95 ? 'var(--danger)' : projectedHit < 99 ? 'var(--warning)' : 'var(--success)';
        summaryEl.innerHTML = `
            <div class="mb-2"><strong>Cache Hit Ratio Trend:</strong> <span style="color:${color};">${trend}</span></div>
            <div class="mb-1">Current miss rate: <strong>${currentMiss.toFixed(3)}%</strong></div>
            <div class="mb-1">Projected miss rate (2h): <strong style="color:${color};">${projectedMiss.toFixed(3)}%</strong></div>
            <div class="mb-1">Projected cache hit (2h): <strong style="color:${color};">${projectedHit}%</strong></div>
            <div class="mt-2 text-muted" style="font-size:0.65rem;">Linear projection based on ${series.length} data points. Dashed line shows forecast.</div>
            ${!osConfigured ? '<div class="mt-2 text-warning" style="font-size:0.65rem;">OS Collector not detected — host RAM metrics unavailable.</div>' : ''}
        `;
    }

    _destroyAndCreate('forecast', ctx, {
        type: 'line',
        data: {
            labels: allLabels,
            datasets: [
                {
                    label: metricLabel,
                    data: ys,
                    borderColor: '#8b5cf6', backgroundColor: 'rgba(139,92,246,0.07)',
                    fill: true, tension: 0.3, pointRadius: 0, borderWidth: 1.5
                },
                {
                    label: 'Projected',
                    data: [...Array(series.length - 1).fill(null), ys[ys.length - 1], ...projectedData.map(p => p.y)],
                    borderColor: '#f59e0b', borderDash: [6, 4],
                    fill: false, tension: 0.3, pointRadius: 0, borderWidth: 1.5
                }
            ]
        },
        options: {
            ..._baseOpts,
            plugins: { legend: { display: true, position: 'top', labels: { boxWidth: 10, font: { size: 9 } } } },
            scales: {
                x: _baseOpts.scales.x,
                y: { ..._baseOpts.scales.y, min: 0, ticks: { ..._baseOpts.scales.y.ticks, callback: v => v.toFixed(2) + '%' } }
            }
        }
    });
}

function _linearRegression(xs, ys) {
    const n = xs.length;
    const sx = xs.reduce((a, v) => a + v, 0);
    const sy = ys.reduce((a, v) => a + v, 0);
    const sxy = xs.reduce((a, v, i) => a + v * ys[i], 0);
    const sx2 = xs.reduce((a, v) => a + v * v, 0);
    const denom = (n * sx2 - sx * sx) || 1;
    const slope = (n * sxy - sx * sy) / denom;
    const intercept = (sy - slope * sx) / n;
    return { slope, intercept };
}

// ── Chart 6: Memory Components Doughnut ──────────────────────────────────

function renderPgComponentsChart(comp) {
    const ctx = document.getElementById('pgComponentsChart')?.getContext('2d');
    if (!ctx) return;

    const data = [
        comp.shared_buffers_mb || 0,
        comp.work_mem_mb || 0,
        comp.maintenance_work_mem_mb || 0,
        comp.wal_buffers_mb || 0,
        comp.temp_buffers_mb || 0
    ];
    const total = data.reduce((a, v) => a + v, 0);

    _destroyAndCreate('pgComp', ctx, {
        type: 'doughnut',
        data: {
            labels: ['shared_buffers', 'work_mem', 'maint_work_mem', 'wal_buffers', 'temp_buffers'],
            datasets: [{
                data,
                backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#64748b'],
                borderWidth: 0
            }]
        },
        options: {
            responsive: true, maintainAspectRatio: false, cutout: '70%',
            plugins: {
                legend: { position: 'right', labels: { boxWidth: 10, font: { size: 9 } } },
                tooltip: { callbacks: { label: item => `${item.label}: ${item.raw} MB (${total > 0 ? ((item.raw/total)*100).toFixed(1) : 0}%)` } }
            }
        }
    });
}

function updateGucTable(comp) {
    function mb(v) { return v ? `${v.toLocaleString()} MB` : '--'; }
    _setEl('guc-shared-buffers', mb(comp.shared_buffers_mb));
    _setEl('guc-work-mem', mb(comp.work_mem_mb));
    _setEl('guc-maint-mem', mb(comp.maintenance_work_mem_mb));
    _setEl('guc-eff-cache', mb(comp.effective_cache_size_mb));
    _setEl('guc-wal-buffers', mb(comp.wal_buffers_mb));
    _setEl('guc-temp-buffers', mb(comp.temp_buffers_mb));
    _setEl('guc-max-conn', comp.max_connections ? comp.max_connections.toString() : '--');
}

// ── Raw telemetry table ───────────────────────────────────────────────────

function updatePgMemoryTable(series) {
    const tbody = document.getElementById('pg-memory-raw-tbody');
    if (!tbody) return;
    tbody.innerHTML = series.slice().reverse().slice(0, 50).map(s => `
        <tr>
            <td>${new Date(s.ts).toLocaleTimeString()}</td>
            <td>${(s.cache_hit_ratio||0).toFixed(3)}%</td>
            <td>${(s.temp_spill_rate_mb_s||0).toFixed(4)}</td>
            <td>${s.active_connections||0}</td>
            <td>${s.total_connections||0}</td>
            <td>${s.temp_files||0}</td>
            <td>${(s.memory_pressure_percent||0) > 0 ? (s.memory_pressure_percent||0).toFixed(1)+'%' : 'N/A'}</td>
            <td>${(s.swap_used_mb||0) > 0 ? (s.swap_used_mb||0).toLocaleString() : 'N/A'}</td>
        </tr>
    `).join('');
}

// ── Advisor panels ────────────────────────────────────────────────────────

function updateAdvisorContent(latest, components, osConfigured) {
    const totalRamMB = latest.total_mem_mb || 0;
    const hostPressure = latest.memory_pressure_percent || 0;
    const swapMB = latest.swap_used_mb || 0;
    const pgRssMB = latest.postgres_rss_mb || 0;
    const recommendedSB = totalRamMB > 0 ? Math.round(totalRamMB * 0.25) : 0;
    const recommendedEffLow = totalRamMB > 0 ? Math.round(totalRamMB * 0.5) : 0;
    const recommendedEffHigh = totalRamMB > 0 ? Math.round(totalRamMB * 0.75) : 0;

    // ── Shared Buffers / Cache Hit Advisor ───────────────────────────────
    const memAdvisor = document.getElementById('mem-advisor-content');
    if (memAdvisor) {
        const hit = latest.cache_hit_ratio || 0;
        const sb  = components.shared_buffers_mb || 0;
        const eff = components.effective_cache_size_mb || 0;
        let html = '<ul class="p-0 m-0" style="list-style:none; font-size:0.78rem;">';

        if (osConfigured && totalRamMB > 0) {
            if (hostPressure >= 90 || swapMB > 0) {
                html += `<li class="text-danger"><i class="fa-solid fa-server"></i> <strong>Host memory pressure:</strong> OS RAM use is ${hostPressure.toFixed(1)}%` +
                    (swapMB > 0 ? ` with ${swapMB.toLocaleString()} MB swap in use` : '') +
                    `. Reduce connection count or <code>work_mem</code> before raising PostgreSQL caches.</li>`;
            } else if (hostPressure >= 80) {
                html += `<li class="text-warning"><i class="fa-solid fa-server"></i> Host RAM use is ${hostPressure.toFixed(1)}% (${(totalRamMB / 1024).toFixed(1)} GB total). Headroom is limited for larger <code>shared_buffers</code> / sort memory.</li>`;
            } else {
                html += `<li class="text-success"><i class="fa-solid fa-server"></i> Host RAM ${(totalRamMB / 1024).toFixed(1)} GB — OS pressure ${hostPressure.toFixed(1)}%, no swap. Safe to tune PostgreSQL memory GUCs against physical RAM.</li>`;
            }
            if (pgRssMB > 0) {
                const rssPct = (pgRssMB / totalRamMB) * 100;
                if (rssPct > 85) {
                    html += `<li class="text-danger mt-1"><i class="fa-solid fa-database"></i> PostgreSQL RSS ${pgRssMB.toLocaleString()} MB is ${rssPct.toFixed(1)}% of host RAM — instance is memory-bound at the OS level.</li>`;
                } else if (rssPct > 60) {
                    html += `<li class="text-warning mt-1"><i class="fa-solid fa-database"></i> PostgreSQL RSS ${pgRssMB.toLocaleString()} MB (${rssPct.toFixed(1)}% of ${totalRamMB.toLocaleString()} MB host RAM).</li>`;
                }
            }
        } else if (!osConfigured) {
            html += `<li class="text-muted"><i class="fa-solid fa-circle-info"></i> Install the OS Collector to compare GUC settings against actual host RAM (25% / 50–75% sizing rules).</li>`;
        }

        // Cache hit ratio assessment
        if (hit < 90) {
            html += `<li class="text-danger mt-1"><i class="fa-solid fa-circle-exclamation"></i> <strong>Critical:</strong> Cache hit ratio is ${hit.toFixed(2)}% — far below target. Substantial disk I/O on every query. Increase <code>shared_buffers</code> significantly.</li>`;
        } else if (hit < 95) {
            html += `<li class="text-danger mt-1"><i class="fa-solid fa-circle-exclamation"></i> Cache hit ratio ${hit.toFixed(2)}% is below 95%. Each miss costs a disk read. Check for large sequential scans bypassing the buffer pool.</li>`;
        } else if (hit < 99) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-triangle-exclamation"></i> Cache hit ratio ${hit.toFixed(2)}% — below the 99% baseline. Monitor <code>pg_statio_user_tables.heap_blks_read</code> to identify cold tables.</li>`;
        } else {
            html += `<li class="text-success mt-1"><i class="fa-solid fa-circle-check"></i> Cache hit ratio ${hit.toFixed(2)}% — excellent. Buffer pool is absorbing the workload efficiently.</li>`;
        }

        // shared_buffers sizing (OS-aware when collector is present)
        if (sb > 0 && recommendedSB > 0) {
            if (sb < recommendedSB * 0.5) {
                html += `<li class="text-danger mt-1"><i class="fa-solid fa-memory"></i> <code>shared_buffers</code> = ${sb} MB is well below the 25% host-RAM guideline (~${recommendedSB.toLocaleString()} MB on this server). Raise gradually and watch cache hit ratio.</li>`;
            } else if (sb < recommendedSB * 0.85) {
                html += `<li class="text-warning mt-1"><i class="fa-solid fa-memory"></i> <code>shared_buffers</code> = ${sb} MB is below the recommended ~${recommendedSB.toLocaleString()} MB (25% of ${totalRamMB.toLocaleString()} MB RAM).</li>`;
            } else if (sb > recommendedSB * 1.35 && hostPressure >= 80) {
                html += `<li class="text-warning mt-1"><i class="fa-solid fa-memory"></i> <code>shared_buffers</code> = ${sb} MB exceeds the usual 25% RAM target while host pressure is ${hostPressure.toFixed(1)}%. Consider lowering before adding more backends.</li>`;
            } else {
                html += `<li class="text-success mt-1"><i class="fa-solid fa-circle-check"></i> <code>shared_buffers</code> = ${sb} MB aligns with ~25% of host RAM (${recommendedSB.toLocaleString()} MB target).</li>`;
            }
        } else if (sb > 0 && sb < 128) {
            html += `<li class="text-danger mt-1"><i class="fa-solid fa-memory"></i> <code>shared_buffers</code> = ${sb} MB is very low. Set to at least 25% of server RAM (typically 256 MB–8 GB).</li>`;
        } else if (sb > 0 && sb < 256) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-memory"></i> <code>shared_buffers</code> = ${sb} MB may be undersized for a production workload.</li>`;
        } else if (sb > 0) {
            html += `<li class="text-success mt-1"><i class="fa-solid fa-circle-check"></i> <code>shared_buffers</code> = ${sb} MB appears adequately sized.</li>`;
        }

        // effective_cache_size vs host RAM and shared_buffers
        if (eff > 0 && recommendedEffLow > 0) {
            if (eff < recommendedEffLow) {
                html += `<li class="text-warning mt-1"><i class="fa-solid fa-lightbulb"></i> <code>effective_cache_size</code> = ${eff} MB understates OS cache — set between ${recommendedEffLow.toLocaleString()}–${recommendedEffHigh.toLocaleString()} MB (50–75% of ${totalRamMB.toLocaleString()} MB RAM) so the planner favors index scans.</li>`;
            } else if (eff > recommendedEffHigh * 1.2) {
                html += `<li class="text-warning mt-1"><i class="fa-solid fa-lightbulb"></i> <code>effective_cache_size</code> = ${eff} MB exceeds realistic OS cache on this host (${totalRamMB.toLocaleString()} MB RAM). Planner may overestimate cache availability.</li>`;
            } else if (sb > 0 && eff < sb * 2) {
                html += `<li class="text-warning mt-1"><i class="fa-solid fa-lightbulb"></i> <code>effective_cache_size</code> = ${eff} MB is less than 2× <code>shared_buffers</code> (${sb} MB). Raise toward ${recommendedEffLow.toLocaleString()}–${recommendedEffHigh.toLocaleString()} MB.</li>`;
            } else {
                html += `<li class="text-success mt-1"><i class="fa-solid fa-circle-check"></i> <code>effective_cache_size</code> = ${eff} MB is within the 50–75% host-RAM planner hint range.</li>`;
            }
        } else if (eff > 0 && sb > 0 && eff < sb * 2) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-lightbulb"></i> <code>effective_cache_size</code> = ${eff} MB is less than 2× <code>shared_buffers</code>. Set it to 50–75% of total server RAM when OS metrics are available.</li>`;
        }

        html += '</ul>';
        memAdvisor.innerHTML = html;
    }

    // ── work_mem / Temp Spill Advisor ────────────────────────────────────
    const workMemAdvisor = document.getElementById('workmem-advisor-content');
    if (workMemAdvisor) {
        const spill = latest.temp_spill_rate_mb_s || 0;
        const wm    = components.work_mem_mb || 4;
        const tmpFiles = latest.temp_files || 0;
        const active = latest.active_connections || 0;
        const sortNodes = 4;
        const estWorkMemMB = active > 0 ? active * wm * sortNodes : 0;
        const freeRamMB = totalRamMB > 0 ? Math.max(0, totalRamMB - (latest.used_mb || 0)) : 0;
        let html = '<ul class="p-0 m-0" style="list-style:none; font-size:0.78rem;">';

        if (osConfigured && totalRamMB > 0 && estWorkMemMB > 0) {
            if (estWorkMemMB > freeRamMB * 0.6 && hostPressure >= 75) {
                html += `<li class="text-danger"><i class="fa-solid fa-calculator"></i> Worst-case <code>work_mem</code> footprint ~${estWorkMemMB.toLocaleString()} MB (${active} active × ${wm} MB × ~${sortNodes} sort nodes) vs ~${freeRamMB.toLocaleString()} MB free host RAM — raising <code>work_mem</code> is risky until connections drop.</li>`;
            } else if (estWorkMemMB > totalRamMB * 0.4) {
                html += `<li class="text-warning"><i class="fa-solid fa-calculator"></i> Estimated peak sort memory ~${estWorkMemMB.toLocaleString()} MB could exceed 40% of host RAM if many queries sort concurrently. Prefer query tuning or PgBouncer before large <code>work_mem</code> increases.</li>`;
            }
        }

        if (spill > 1.0) {
            html += `<li class="text-danger mt-1"><i class="fa-solid fa-bolt"></i> <strong>High spill rate:</strong> ${spill.toFixed(2)} MB/s being written to temp files. Queries are heavily overflowing <code>work_mem</code> (${wm} MB).</li>`;
            let suggested = Math.min(Math.max(wm * 2, 16), 256);
            if (osConfigured && totalRamMB > 0 && active > 0) {
                const budgetPerConn = Math.max(4, Math.floor((totalRamMB * 0.15) / (active * sortNodes)));
                suggested = Math.min(suggested, budgetPerConn, 256);
            }
            html += `<li class="text-accent mt-1"><i class="fa-solid fa-lightbulb"></i> Consider raising <code>work_mem</code> toward ${suggested} MB (per sort/hash node × active backends). Test under load; use session-level overrides for heavy reports.</li>`;
        } else if (spill > 0.1) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-circle-exclamation"></i> Disk spill rate ${spill.toFixed(3)} MB/s. Some queries exceed <code>work_mem</code> = ${wm} MB. Identify offenders via <code>pg_stat_statements.temp_blks_written</code>.</li>`;
        } else if (spill > 0) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-circle-exclamation"></i> Minor spill (${spill.toFixed(4)} MB/s). Monitor complex sorts and hash joins; may be intermittent.</li>`;
        } else {
            html += `<li class="text-success mt-1"><i class="fa-solid fa-circle-check"></i> No disk spills. <code>work_mem</code> = ${wm} MB is sufficient for the current workload.</li>`;
        }

        if (tmpFiles > 10) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-file-alt"></i> ${tmpFiles} temp files created in this window — review plans for <code>Sort Method: external merge</code>.</li>`;
        }

        html += '</ul>';
        workMemAdvisor.innerHTML = html;
    }

    // ── Connection Overhead Advisor ──────────────────────────────────────
    const connAdvisor = document.getElementById('conn-advisor-content');
    if (connAdvisor) {
        const active = latest.active_connections || 0;
        const total  = latest.total_connections || 0;
        const maxConn = components.max_connections || 100;
        const usagePct = maxConn > 0 ? (total / maxConn) * 100 : 0;
        // PostgreSQL forks ~5-10 MB per backend (stack + shared mem overhead)
        const estOverheadMB = total * 7;
        let html = '<ul class="p-0 m-0" style="list-style:none; font-size:0.78rem;">';

        html += `<li><i class="fa-solid fa-info-circle text-info"></i> ${active} active / ${total} total out of ${maxConn} max connections (${usagePct.toFixed(1)}% used). Estimated backend overhead: ~${estOverheadMB.toLocaleString()} MB.</li>`;

        if (osConfigured && totalRamMB > 0) {
            const wm = components.work_mem_mb || 4;
            const sb = components.shared_buffers_mb || 0;
            const connBudget = total * wm * 4;
            const totalEst = sb + connBudget + estOverheadMB;
            if (totalEst > totalRamMB * 0.9) {
                html += `<li class="text-danger mt-1"><i class="fa-solid fa-scale-balanced"></i> Rough memory budget (shared_buffers + connections×work_mem×4 + overhead) ≈ ${totalEst.toLocaleString()} MB exceeds ~90% of ${totalRamMB.toLocaleString()} MB host RAM — lower <code>max_connections</code> or add PgBouncer.</li>`;
            } else if (totalEst > totalRamMB * 0.7) {
                html += `<li class="text-warning mt-1"><i class="fa-solid fa-scale-balanced"></i> Estimated PostgreSQL footprint ~${totalEst.toLocaleString()} MB vs ${totalRamMB.toLocaleString()} MB host RAM leaves limited headroom for OS page cache.</li>`;
            }
        }

        if (usagePct > 90) {
            html += `<li class="text-danger mt-1"><i class="fa-solid fa-users-slash"></i> <strong>Connection saturation critical:</strong> ${usagePct.toFixed(1)}% of <code>max_connections</code> consumed. Remaining headroom: ${maxConn - total}. Deploy PgBouncer immediately.</li>`;
        } else if (usagePct > 75) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-users"></i> Connection usage at ${usagePct.toFixed(1)}%. At this rate, saturation is likely under peak load. Consider PgBouncer connection pooling.</li>`;
        } else if (total > 100) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-users"></i> ${total} connections is high. Each backend fork uses ~5–10 MB OS overhead beyond shared memory. PgBouncer can reduce this to a handful of server-side connections.</li>`;
        } else {
            html += `<li class="text-success mt-1"><i class="fa-solid fa-users"></i> Connection count is healthy (${usagePct.toFixed(1)}% of max). No action needed.</li>`;
        }

        // Idle connection warning
        const idle = (total - active);
        if (idle > 50 && idle > total * 0.6) {
            html += `<li class="text-warning mt-1"><i class="fa-solid fa-plug"></i> ~${idle} connections appear idle. Idle server backends still consume memory. PgBouncer's <em>session pooling</em> or <em>transaction pooling</em> can reclaim this.</li>`;
        }

        html += '</ul>';
        connAdvisor.innerHTML = html;
    }
}

window.initPgMemoryCockpit = initPgMemoryCockpit;
