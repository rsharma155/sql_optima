/*
 * SQL Optima — Security Monitoring
 */
(function() {
    window.PgSecurityView = function() {
        const instance = window.appState.config?.instances?.[window.appState.currentInstanceIdx]?.name;
        if (!instance) return;

        window.appState.activeViewId = 'pg-security';
        
        window.routerOutlet.innerHTML = `
            <div class="page-view active dashboard-sky-theme">
                <div class="page-title flex-between dashboard-page-title-compact">
                    <div class="dashboard-title-line">
                        <h1><i class="fa-solid fa-user-lock"></i> Security Monitor</h1>
                        <span class="subtitle">Failed logins, elevated privileges, and DDL activity tracking</span>
                    </div>
                    <div class="flex-center">
                        <div id="time-picker-insertion-point"></div>
                    </div>
                </div>

                <!-- ROW 1: Compact Metric Strip -->
                <div class="metrics-row-compact">
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Failed Logins</div>
                        <div class="metric-value text-danger" id="kpi-failed-logins">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Superusers</div>
                        <div class="metric-value text-warning" id="kpi-superusers">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Replica Privs</div>
                        <div class="metric-value" id="kpi-repl-privs">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">New Roles (7d)</div>
                        <div class="metric-value text-success" id="kpi-new-roles">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Inserts (24h)</div>
                        <div class="metric-value" id="kpi-inserts">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Updates (24h)</div>
                        <div class="metric-value" id="kpi-updates">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Deletes (24h)</div>
                        <div class="metric-value" id="kpi-deletes">--</div>
                    </div>
                    <div class="glass-panel metric-card-compact">
                        <div class="metric-label">Risk Level</div>
                        <div class="metric-value" id="kpi-risk-level">Low</div>
                    </div>
                </div>

                <!-- ROW 2: Failed Logins & Audit Activity -->
                <div class="chart-row-compact">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Failed Login Trend</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-login-trend-chart"></canvas>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">DML/DDL Activity (Audit)</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-audit-trend-chart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- ROW 3: Superusers & Elevated Roles Tables -->
                <div class="chart-row-compact">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Superusers</h3></div>
                        <div class="table-container-compact">
                            <table class="modern-table modern-table-compact" id="pg-superusers-table">
                                <thead><tr><th>Role</th><th>DB</th><th>Role</th><th>Repl</th></tr></thead>
                                <tbody></tbody>
                            </table>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Elevated Privileges</h3></div>
                        <div class="table-container-compact">
                            <table class="modern-table modern-table-compact" id="pg-elevated-roles-table">
                                <thead><tr><th>Role</th><th>Super</th><th>Create</th><th>Repl</th></tr></thead>
                                <tbody></tbody>
                            </table>
                        </div>
                    </div>
                <!-- ROW 4: Additional Security Metrics -->
                <div class="chart-row-compact">
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Connection Origins (Failed Logins)</h3></div>
                        <div class="table-container-compact">
                            <table class="modern-table modern-table-compact" id="pg-origins-table">
                                <thead><tr><th>IP Address</th><th>Failures</th></tr></thead>
                                <tbody></tbody>
                            </table>
                        </div>
                    </div>
                    <div class="card glass-panel">
                        <div class="card-header"><h3 style="font-size:0.75rem; margin:0;">Total Roles Over Time</h3></div>
                        <div class="chart-container chart-container-compact">
                            <canvas id="pg-role-trend-chart"></canvas>
                        </div>
                    </div>
                </div>
            </div>
        `;

        window.initPageTimePicker();
        fetchData(instance);
    };

    async function fetchData(instance) {
        let from = window.appState.fromTs;
        let to = window.appState.toTs;
        if (from && from.includes('T') && !from.endsWith('Z')) from = new Date(from).toISOString();
        if (to && to.includes('T') && !to.endsWith('Z')) to = new Date(to).toISOString();

        // Show loading state
        ['pg-login-trend-chart', 'pg-audit-trend-chart', 'pg-role-trend-chart'].forEach(id => {
            window.setChartOverlayState(id, 'loading');
        });

        try {
            let url = `/api/pg/security/dashboard?instance=${encodeURIComponent(instance)}`;
            if (from) url += `&from=${encodeURIComponent(from)}`;
            if (to) url += `&to=${encodeURIComponent(to)}`;

            const resp = await window.apiClient.authenticatedFetch(url);

            if (!resp.ok) {
                const msg = resp.status === 503 ? "TimescaleDB Disconnected" : "Failed to fetch security data";
                ['pg-login-trend-chart', 'pg-audit-trend-chart', 'pg-role-trend-chart'].forEach(id => {
                    window.setChartOverlayState(id, 'empty', msg);
                });
                return;
            }
            const data = await resp.json();

            ['pg-login-trend-chart', 'pg-audit-trend-chart', 'pg-role-trend-chart'].forEach(id => {
                window.clearChartOverlay(id);
            });

            renderKPIs(data.kpis, data.dml_trend);
            renderLoginTrend(data.login_trend);
            renderSuperusers(data.superusers);
            renderElevatedRoles(data.elevated_roles);
            renderAuditTrend(data.dml_trend);
            renderOrigins(data.connection_origins);
            renderRoleTrend(data.role_trend);
        } catch (err) { 
            console.error(err); 
            ['pg-login-trend-chart', 'pg-audit-trend-chart', 'pg-role-trend-chart'].forEach(id => {
                window.setChartOverlayState(id, 'empty', "Error loading data");
            });
        }
    }

    function renderKPIs(kpis, dml) {
        if (!kpis) return;
        document.getElementById('kpi-failed-logins').textContent = kpis.failed_logins || '0';
        document.getElementById('kpi-superusers').textContent = kpis.superusers || '0';
        document.getElementById('kpi-repl-privs').textContent = kpis.repl_privileges || '0';
        document.getElementById('kpi-new-roles').textContent = kpis.new_roles || '0';
        
        if (dml && dml.length > 0) {
            const last = dml[dml.length - 1];
            document.getElementById('kpi-inserts').textContent = Math.round(last.ins || 0);
            document.getElementById('kpi-updates').textContent = Math.round(last.upd || 0);
            document.getElementById('kpi-deletes').textContent = Math.round(last.del || 0);
        }
    }

    function renderLoginTrend(trend) {
        const ctx = document.getElementById('pg-login-trend-chart').getContext('2d');
        if (!trend) return;
        new Chart(ctx, {
            type: 'bar',
            data: {
                labels: trend.map(t => new Date(t.bucket)),
                datasets: [{ label: 'Failed Logins', data: trend.map(t => t.count), backgroundColor: '#ef4444' }]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { x: { type: 'time', time: { unit: 'hour' }, display: false } }
            }
        });
    }

    function renderSuperusers(users) {
        const tbody = document.querySelector('#pg-superusers-table tbody');
        if (!users || users.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">None</td></tr>';
            return;
        }
        tbody.innerHTML = users.slice(0, 4).map(u => `
            <tr>
                <td><strong>${window.escapeHtml(u.rolname)}</strong></td>
                <td>${u.rolcreatedb ? '✓' : ''}</td>
                <td>${u.rolcreaterole ? '✓' : ''}</td>
                <td>${u.rolreplication ? '✓' : ''}</td>
            </tr>
        `).join('');
    }

    function renderElevatedRoles(roles) {
        const tbody = document.querySelector('#pg-elevated-roles-table tbody');
        if (!roles || roles.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">None</td></tr>';
            return;
        }
        tbody.innerHTML = roles.slice(0, 4).map(r => `
            <tr>
                <td>${window.escapeHtml(r.rolname)}</td>
                <td>${r.rolsuper ? 'Yes' : ''}</td>
                <td>${r.rolcreaterole ? 'Yes' : ''}</td>
                <td>${r.rolreplication ? 'Yes' : ''}</td>
            </tr>
        `).join('');
    }

    function renderAuditTrend(trend) {
        const ctx = document.getElementById('pg-audit-trend-chart').getContext('2d');
        if (!trend) return;
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: trend.map(t => new Date(t.bucket)),
                datasets: [
                    { label: 'Inserts', data: trend.map(t => t.ins), borderColor: '#10b981', backgroundColor: 'rgba(16, 185, 129, 0.2)', tension: 0.1, fill: true, stack: 'stack1' },
                    { label: 'Updates', data: trend.map(t => t.upd), borderColor: '#f59e0b', backgroundColor: 'rgba(245, 158, 11, 0.2)', tension: 0.1, fill: true, stack: 'stack1' },
                    { label: 'Deletes', data: trend.map(t => t.del), borderColor: '#ef4444', backgroundColor: 'rgba(239, 68, 68, 0.2)', tension: 0.1, fill: true, stack: 'stack1' }
                ]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { x: { type: 'time', time: { unit: 'hour' }, display: false }, y: { stacked: true, beginAtZero: true } }
            }
        });
    }

    function renderOrigins(origins) {
        const tbody = document.querySelector('#pg-origins-table tbody');
        if (!origins || origins.length === 0) {
            tbody.innerHTML = '<tr><td colspan="2" class="text-center text-muted">No failed logins</td></tr>';
            return;
        }
        tbody.innerHTML = origins.slice(0, 5).map(o => `
            <tr>
                <td><strong>${window.escapeHtml(o.client_addr)}</strong></td>
                <td><span class="text-danger">${o.fails}</span></td>
            </tr>
        `).join('');
    }

    function renderRoleTrend(trend) {
        const ctx = document.getElementById('pg-role-trend-chart');
        if (!ctx) return;
        if (!trend || trend.length === 0) return;
        new Chart(ctx.getContext('2d'), {
            type: 'line',
            data: {
                labels: trend.map(t => new Date(t.bucket)),
                datasets: [{ label: 'Total Roles', data: trend.map(t => t.total), borderColor: '#3b82f6', backgroundColor: 'rgba(59, 130, 246, 0.2)', fill: true, tension: 0.2 }]
            },
            options: {
                responsive: true, maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { x: { type: 'time', time: { unit: 'day' }, display: true }, y: { beginAtZero: false } }
            }
        });
    }
})();
