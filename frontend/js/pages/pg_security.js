/*
 * SQL Optima — Security Monitoring
 * Redesigned for Security Operations Flow - COMPACT VERSION
 */
(function() {
    let lastFetchedData = null;
    const chartKeys = { login: 'pg-login-trend-chart', audit: 'pg-audit-trend-chart', role: 'pg-role-trend-chart' };

    function sortedTrend(trend) {
        return window.sortByChartTime ? window.sortByChartTime(trend || []) : [...(trend || [])];
    }

    function trendLabels(trend) {
        const rows = sortedTrend(trend);
        if (!rows.length) return [];
        const spanMs = rows.length >= 2
            ? (window.tsMs(rows[rows.length - 1]) ?? 0) - (window.tsMs(rows[0]) ?? 0)
            : 0;
        const multiDay = spanMs > 36 * 3600 * 1000;
        return rows.map((row) => {
            const d = window.parseChartTimestamp ? window.parseChartTimestamp(row) : null;
            if (!d) return '';
            if (multiDay) {
                return d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
            }
            return window.fmtChartTick
                ? window.fmtChartTick(row, { hour: '2-digit', minute: '2-digit' })
                : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        });
    }

    function destroySecurityChart(key) {
        const id = chartKeys[key];
        window.pgSecurityCharts = window.pgSecurityCharts || {};
        if (window.pgSecurityCharts[id]) {
            window.pgSecurityCharts[id].destroy();
            delete window.pgSecurityCharts[id];
        }
    }

    function baseChartOptions(extra = {}) {
        const tooltip = window.chartTooltipPlugin ? window.chartTooltipPlugin() : { enabled: true };
        const interaction = window.chartInteractionOptions
            ? window.chartInteractionOptions()
            : { mode: 'index', intersect: false };
        const base = {
            responsive: true,
            maintainAspectRatio: false,
            interaction,
            plugins: { legend: { display: false }, tooltip },
        };
        return window.mergeChartOptions ? window.mergeChartOptions(base, extra) : { ...base, ...extra };
    }

    window.PgSecurityView = function() {
        const instance = window.appState.config?.instances?.[window.appState.currentInstanceIdx]?.name;
        if (!instance) return;

        window.appState.activeViewId = 'pg-security';
        
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme pg-security-page">
                <!-- HEADER BAR (ROW 0) -->
                <div class="page-title flex-between dashboard-page-title-compact pg-security-page-title">
                    <div class="dashboard-title-line">
                        <h1><i class="fa-solid fa-user-lock"></i> Security Monitor <i class="fa-solid fa-circle-info text-accent cursor-pointer" style="font-size: 0.9rem; margin-left: 0.5rem;" data-action="show-pg-dashboard-detail" data-dashboard="Security Monitor" title="Learn more about this dashboard"></i></h1>
                        <span class="subtitle pg-security-subtitle">Failed logins • Privilege escalation • DDL monitoring</span>
                    </div>
                    <div class="flex-center dashboard-page-title-actions pg-security-title-actions">
                        <div id="time-picker-insertion-point"></div>
                        <button id="export-security-btn" class="btn btn-xs btn-outline" title="Export Security Report as CSV"><i class="fa-solid fa-file-csv"></i> CSV</button>
                    </div>
                </div>

                <!-- ROW 1: KPI STRIP (single row on laptop/small) -->
                <div class="kpi-row pg-security-kpi-row">
                    <div class="pg-security-kpi-risk">
                        <div class="glass-panel risk-card risk-info" id="risk-level-card">
                            <div class="risk-main">
                                <div class="risk-label">Overall Risk Level <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-pg-info" data-section="Security Monitor" data-metric="Failed Logins"></i></div>
                                <div class="risk-level-value" id="kpi-risk-level">LOW</div>
                            </div>
                            <div class="risk-indicators" id="risk-indicators-box">
                                <span id="risk-failed-indicator">Threat: --</span>
                                <span id="risk-privilege-indicator">Priv: --</span>
                            </div>
                        </div>
                    </div>

                    <div class="pg-security-kpi-item">
                        <div class="glass-panel security-kpi-card row-height-kpi">
                            <div class="kpi-label">Failed Logins <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-pg-info" data-section="Security Monitor" data-metric="Failed Logins"></i></div>
                            <div class="kpi-value text-danger" id="kpi-failed-logins">--</div>
                            <div class="kpi-trend" id="trend-failed-logins">
                                <span class="text-muted"><i class="fa-solid fa-minus"></i> Stable</span>
                            </div>
                        </div>
                    </div>

                    <div class="pg-security-kpi-item">
                        <div class="glass-panel security-kpi-card row-height-kpi">
                            <div class="kpi-label">New Roles 7d <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-pg-info" data-section="Security Monitor" data-metric="Superuser Count"></i></div>
                            <div class="kpi-value text-warning" id="kpi-new-roles">--</div>
                            <div class="kpi-trend" id="trend-new-roles"><span class="text-muted">Stable</span></div>
                        </div>
                    </div>

                    <div class="pg-security-kpi-item">
                        <div class="glass-panel security-kpi-card row-height-kpi">
                            <div class="kpi-label">Superusers <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-pg-info" data-section="Security Monitor" data-metric="Superuser Count"></i></div>
                            <div class="kpi-value text-critical" id="kpi-superusers">--</div>
                            <div class="kpi-trend" id="trend-superusers"><span class="text-muted">No change</span></div>
                        </div>
                    </div>

                    <div class="pg-security-kpi-item">
                        <div class="glass-panel security-kpi-card row-height-kpi">
                            <div class="kpi-label">Replica Privs <i class="fa-solid fa-info-circle info-icon-sm" data-action="show-pg-info" data-section="Backup & DR" data-metric="Replication Slots Risk"></i></div>
                            <div class="kpi-value text-info" id="kpi-repl-privs">--</div>
                            <div class="kpi-trend" id="trend-repl-privs"><span class="text-muted">Stable</span></div>
                        </div>
                    </div>
                </div>

                <!-- MAIN SECURITY GRID -->
                <div class="security-grid">

                    <!-- ROW 2: DETAILED ACCESS GRID -->
                    <div class="col-span-12 col-mobile-12">
                        <div class="card glass-panel pg-security-access-card">
                            <div class="card-header flex-between pg-security-card-header">
                                <h3 class="pg-security-card-title">Roles, Login & Access Privileges</h3>
                                <span class="text-muted pg-security-card-hint">Active session-level and object-level permissions summary for all users and roles</span>
                            </div>
                            <div class="table-container-compact pg-security-table-scroll pg-security-access-table-wrap">
                                <table class="compact-security-table" id="pg-access-privs-grid">
                                    <thead>
                                        <tr>
                                            <th>Role Name</th>
                                            <th>Can Login</th>
                                            <th>Superuser</th>
                                            <th>Create DB</th>
                                            <th>Create Role</th>
                                            <th>Replication</th>
                                            <th>Risk Impact</th>
                                        </tr>
                                    </thead>
                                    <tbody></tbody>
                                </table>
                            </div>
                        </div>
                    </div>

                    <!-- ROW 3: TREND CHARTS -->
                    <div class="col-span-6 col-laptop-8 col-tablet-6 col-mobile-12">
                        <div class="card glass-panel pg-security-chart-card">
                            <div class="card-header flex-between pg-security-card-header">
                                <h3 class="pg-security-card-title">Failed Login Trend</h3>
                            </div>
                            <div class="security-chart-container pg-security-chart-wrap">
                                <canvas id="pg-login-trend-chart"></canvas>
                                <div id="login-no-data" class="no-data-placeholder" style="display:none;">No data</div>
                            </div>
                        </div>
                    </div>

                    <div class="col-span-6 col-laptop-8 col-tablet-6 col-mobile-12">
                        <div class="card glass-panel pg-security-chart-card">
                            <div class="card-header flex-between pg-security-card-header">
                                <h3 class="pg-security-card-title">DML / Audit Activity</h3>
                            </div>
                            <div class="security-chart-container pg-security-chart-wrap">
                                <canvas id="pg-audit-trend-chart"></canvas>
                                <div id="audit-no-data" class="no-data-placeholder" style="display:none;">No data</div>
                            </div>
                        </div>
                    </div>

                    <!-- ROW 4: INVESTIGATION ZONE -->
                    <div class="col-span-5 col-laptop-8 col-tablet-6 col-mobile-12">
                        <div class="card glass-panel pg-security-origins-card">
                            <div class="card-header"><h3 class="pg-security-card-title">Failed Login Origins</h3></div>
                            <div class="table-container-compact row-height-table pg-security-table-scroll">
                                <table class="compact-security-table" id="pg-origins-table">
                                    <thead><tr><th>IP Address</th><th>Fails</th><th>Action</th></tr></thead>
                                    <tbody></tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                    <div class="col-span-7 col-laptop-8 col-tablet-6 col-mobile-12">
                        <div class="card glass-panel pg-security-role-chart-card">
                            <div class="card-header"><h3 class="pg-security-card-title">Role History Trend</h3></div>
                            <div class="security-chart-container row-height-table pg-security-chart-wrap pg-security-role-chart-wrap">
                                <canvas id="pg-role-trend-chart"></canvas>
                                <div id="role-no-data" class="no-data-placeholder" style="display:none;">No data</div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `;

        window.initPageTimePicker();
        
        document.getElementById('export-security-btn')?.addEventListener('click', () => {
            exportToCSV(instance);
        });

        fetchData(instance);
    };

    async function fetchData(instance) {
        let from = window.appState.fromTs;
        let to = window.appState.toTs;
        if (from && from.includes('T') && !from.endsWith('Z')) from = new Date(from).toISOString();
        if (to && to.includes('T') && !to.endsWith('Z')) to = new Date(to).toISOString();

        try {
            let url = `/api/pg/security/dashboard?instance=${encodeURIComponent(instance)}`;
            if (from) url += `&from=${encodeURIComponent(from)}`;
            if (to) url += `&to=${encodeURIComponent(to)}`;

            const resp = await window.apiClient.authenticatedFetch(url);

            if (!resp.ok) return;
            const data = await resp.json();
            lastFetchedData = data;

            renderKPIs(data.kpis, data.dml_trend);
            renderLoginTrend(data.login_trend);
            renderAccessGrid(data.all_roles || data.elevated_roles);
            renderAuditTrend(data.dml_trend);
            renderOrigins(data.connection_origins);
            renderRoleTrend(data.role_trend);
            calculateRisk(data.kpis, data.dml_trend, data.login_trend);
        } catch (err) { console.error(err); }
    }

    function calculateRisk(kpis, dml, loginTrend) {
        if (!kpis) return;
        let maxFailures = 0;
        if (loginTrend && loginTrend.length > 0) {
            maxFailures = Math.max(...loginTrend.map(t => t.count));
        }
        const failed_login_rate = Math.min(maxFailures / 10, 1.0);
        const privilege_risk_score = Math.min(((kpis.superusers || 0) * 5 + (kpis.repl_privileges || 0) * 3 + (kpis.new_roles || 0)) / 25, 1.0);
        
        let dml_activity = 0;
        if (dml && dml.length > 0) {
            const last = dml[dml.length - 1];
            dml_activity = (last.ins || 0) + (last.upd || 0) + (last.del || 0);
        }
        const ddl_risk_score = Math.min(dml_activity / 5000, 1.0);
        const score = (failed_login_rate * 4 + privilege_risk_score * 3 + ddl_risk_score * 2);
        
        let level = 'LOW', cls = 'risk-safe';
        if (score > 6) { level = 'CRITICAL'; cls = 'risk-critical'; }
        else if (score > 4) { level = 'HIGH'; cls = 'risk-critical'; }
        else if (score > 2) { level = 'MEDIUM'; cls = 'risk-warning'; }
        
        const card = document.getElementById('risk-level-card');
        if (card) {
            card.className = `glass-panel risk-card ${cls}`;
            document.getElementById('kpi-risk-level').textContent = level;
            document.getElementById('risk-indicators-box').innerHTML = `
                <span>Threat Index: ${Math.round(failed_login_rate * 100)}%</span>
                <span>Priv Risk: ${Math.round(privilege_risk_score * 100)}%</span>
            `;
        }
    }

    function renderKPIs(kpis, dml) {
        if (!kpis) return;
        document.getElementById('kpi-failed-logins').textContent = kpis.failed_logins || '0';
        document.getElementById('kpi-superusers').textContent = kpis.superusers || '0';
        document.getElementById('kpi-repl-privs').textContent = kpis.repl_privileges || '0';
        document.getElementById('kpi-new-roles').textContent = kpis.new_roles || '0';
    }

    function renderLoginTrend(trend) {
        const canvas = document.getElementById('pg-login-trend-chart');
        const noData = document.getElementById('login-no-data');
        destroySecurityChart('login');
        if (!trend || trend.length === 0) {
            if (canvas) canvas.style.display = 'none';
            if (noData) noData.style.display = 'flex';
            return;
        }
        const rows = sortedTrend(trend);
        canvas.style.display = 'block';
        noData.style.display = 'none';
        window.pgSecurityCharts = window.pgSecurityCharts || {};
        window.pgSecurityCharts[chartKeys.login] = new Chart(canvas.getContext('2d'), {
            type: 'line',
            data: {
                labels: trendLabels(rows),
                datasets: [{
                    label: 'Failed Logins',
                    data: rows.map(t => t.count),
                    borderColor: '#ef4444',
                    backgroundColor: 'rgba(239, 68, 68, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 1,
                }],
            },
            options: baseChartOptions({
                plugins: { legend: { display: false } },
                scales: {
                    x: {
                        display: true,
                        ticks: {
                            color: '#64748b',
                            font: { size: 9 },
                            maxRotation: 45,
                            autoSkip: true,
                            maxTicksLimit: window.innerWidth < 768 ? 5 : 8,
                        },
                        grid: { display: false },
                    },
                    y: { beginAtZero: true, ticks: { color: '#64748b', font: { size: 9 } } },
                },
            }),
        });
    }

    function renderAccessGrid(roles) {
        const tbody = document.querySelector('#pg-access-privs-grid tbody');
        if (!roles || roles.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">No security role data available</td></tr>';
            return;
        }
        
        const sorted = [...roles].sort((a,b) => {
            if (a.rolsuper !== b.rolsuper) return b.rolsuper ? 1 : -1;
            return a.rolname.localeCompare(b.rolname);
        });

        tbody.innerHTML = sorted.map(u => {
            let risk = '<span class="badge badge-info">Low</span>';
            if (u.rolsuper) risk = '<span class="badge badge-danger" title="Superusers bypass all permission checks and can execute OS commands. Minimum of 1-2 required.">Critical</span>';
            else if (u.rolcreaterole || u.rolreplication) risk = '<span class="badge badge-warning">High</span>';

            return `
                <tr class="${u.rolsuper ? 'row-superuser' : ''}">
                    <td><strong>${window.escapeHtml(u.rolname)}</strong></td>
                    <td>${u.rolcanlogin !== false ? '<i class="fa-solid fa-check text-safe"></i>' : '<i class="fa-solid fa-xmark text-muted"></i>'}</td>
                    <td>${u.rolsuper ? '<span class="text-danger">YES</span>' : 'No'}</td>
                    <td>${u.rolcreatedb ? 'Yes' : 'No'}</td>
                    <td>${u.rolcreaterole ? 'Yes' : 'No'}</td>
                    <td>${u.rolreplication ? 'Yes' : 'No'}</td>
                    <td>${risk}</td>
                </tr>
            `;
        }).join('');
    }

    function renderAuditTrend(trend) {
        const canvas = document.getElementById('pg-audit-trend-chart');
        const noData = document.getElementById('audit-no-data');
        destroySecurityChart('audit');
        if (!trend || trend.length === 0) {
            if (canvas) canvas.style.display = 'none';
            if (noData) noData.style.display = 'flex';
            return;
        }
        const rows = sortedTrend(trend);
        canvas.style.display = 'block';
        noData.style.display = 'none';
        window.pgSecurityCharts = window.pgSecurityCharts || {};
        window.pgSecurityCharts[chartKeys.audit] = new Chart(canvas.getContext('2d'), {
            type: 'bar',
            data: {
                labels: trendLabels(rows),
                datasets: [
                    { label: 'Inserts', data: rows.map(t => t.ins), backgroundColor: '#22c55e' },
                    { label: 'Updates', data: rows.map(t => t.upd), backgroundColor: '#f59e0b' },
                    { label: 'Deletes', data: rows.map(t => t.del), backgroundColor: '#ef4444' },
                ],
            },
            options: baseChartOptions({
                plugins: {
                    legend: {
                        display: true,
                        position: 'top',
                        labels: { boxWidth: 10, font: { size: 9 }, color: '#94a3b8' },
                    },
                },
                scales: {
                    x: {
                        display: true,
                        stacked: true,
                        ticks: {
                            color: '#64748b',
                            font: { size: 9 },
                            maxRotation: 45,
                            autoSkip: true,
                            maxTicksLimit: window.innerWidth < 768 ? 5 : 8,
                        },
                        grid: { display: false },
                    },
                    y: { stacked: true, beginAtZero: true, ticks: { color: '#64748b', font: { size: 9 } } },
                },
            }),
        });
    }

    function renderOrigins(origins) {
        const tbody = document.querySelector('#pg-origins-table tbody');
        if (!origins || origins.length === 0) {
            tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">No threat origins detected</td></tr>';
            return;
        }
        tbody.innerHTML = origins.map(o => `
            <tr>
                <td><strong>${window.escapeHtml(o.client_addr)}</strong></td>
                <td><span class="text-danger font-bold">${o.fails}</span></td>
                <td><button class="btn btn-xs btn-outline-danger">Audit Logs</button></td>
            </tr>
        `).join('');
    }

    function renderRoleTrend(trend) {
        const canvas = document.getElementById('pg-role-trend-chart');
        const noData = document.getElementById('role-no-data');
        destroySecurityChart('role');
        if (!trend || trend.length === 0) {
            if (canvas) canvas.style.display = 'none';
            if (noData) noData.style.display = 'flex';
            return;
        }
        const rows = sortedTrend(trend);
        canvas.style.display = 'block';
        noData.style.display = 'none';
        window.pgSecurityCharts = window.pgSecurityCharts || {};
        window.pgSecurityCharts[chartKeys.role] = new Chart(canvas.getContext('2d'), {
            type: 'line',
            data: {
                labels: trendLabels(rows),
                datasets: [{
                    label: 'Total Roles',
                    data: rows.map(t => t.total),
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59, 130, 246, 0.05)',
                    fill: true,
                    tension: 0.4,
                    pointRadius: 0,
                }],
            },
            options: baseChartOptions({
                plugins: { legend: { display: false } },
                scales: {
                    x: {
                        display: true,
                        ticks: {
                            color: '#64748b',
                            font: { size: 9 },
                            maxRotation: 45,
                            autoSkip: true,
                            maxTicksLimit: window.innerWidth < 768 ? 5 : 8,
                        },
                        grid: { display: false },
                    },
                    y: { beginAtZero: false, ticks: { color: '#64748b', font: { size: 9 } } },
                },
            }),
        });
    }

    function exportToCSV(instance) {
        if (!lastFetchedData) {
            alert("No data available to export. Please refresh first.");
            return;
        }

        const rows = [];
        // Header
        rows.push(["Security Report", instance, new Date().toLocaleString()]);
        rows.push([]);

        // Roles & Privileges
        rows.push(["ROLES, LOGIN & ACCESS PRIVILEGES"]);
        rows.push(["Role Name", "Can Login", "Superuser", "Create DB", "Create Role", "Replication"]);
        const allRoles = lastFetchedData.all_roles || lastFetchedData.elevated_roles || [];
        
        allRoles.forEach(u => {
            rows.push([
                u.rolname,
                u.rolcanlogin !== false ? "YES" : "NO",
                u.rolsuper ? "YES" : "NO",
                u.rolcreatedb ? "YES" : "NO",
                u.rolcreaterole ? "YES" : "NO",
                u.rolreplication ? "YES" : "NO"
            ]);
        });
        rows.push([]);

        // Connection Origins
        rows.push(["FAILED LOGIN ORIGINS"]);
        rows.push(["IP Address", "Failures Count"]);
        (lastFetchedData.connection_origins || []).forEach(o => {
            rows.push([o.client_addr, o.fails]);
        });

        // Convert to CSV string
        let csvContent = "data:text/csv;charset=utf-8," 
            + rows.map(e => e.map(cell => `"${String(cell).replace(/"/g, '""')}"`).join(",")).join("\n");

        const encodedUri = encodeURI(csvContent);
        const link = document.createElement("a");
        link.setAttribute("href", encodedUri);
        link.setAttribute("download", `security_report_${instance}_${new Date().toISOString().slice(0,10)}.csv`);
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
    }
})();
