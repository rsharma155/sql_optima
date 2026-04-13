window.escapeHtml = function(unsafe) { return (!unsafe) ? '' : unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;"); };

function bottleneckRowHasLogin(q) {
    const v = q && q.login_name;
    return v != null && String(v).trim() !== '';
}
function bottleneckRowHasApp(q) {
    const v = q && (q.program_name || q.application_name);
    return v != null && String(v).trim() !== '';
}
/** Which optional session columns to show (Query Store payloads omit these). */
function bottleneckColumnVisibility(queries) {
    if (!queries || !queries.length) {
        return { showLogin: false, showApp: false, colCount: 10 };
    }
    const showLogin = queries.some(bottleneckRowHasLogin);
    const showApp = queries.some(bottleneckRowHasApp);
    return { showLogin, showApp, colCount: 10 };
}
function syncBottlenecksTableHeaders(showLogin, showApp) {
    const thLogin = document.getElementById('bottlenecks-th-login');
    const thApp = document.getElementById('bottlenecks-th-app');
    if (thLogin) thLogin.style.display = showLogin ? '' : 'none';
    if (thApp) thApp.style.display = showApp ? '' : 'none';
}
/** Keep 10 columns aligned with thead; hide cells when session fields absent. */
function bottleneckSessionTd(show, text) {
    const disp = show ? '' : 'display: none;';
    return `<td style="font-size:0.7rem; ${disp}">${window.escapeHtml(text)}</td>`;
}
function syncBottlenecksAboutHints(showLogin, showApp) {
    const liLogin = document.getElementById('bottlenecks-about-login');
    const liApp = document.getElementById('bottlenecks-about-app');
    if (liLogin) liLogin.style.display = showLogin ? '' : 'none';
    if (liApp) liApp.style.display = showApp ? '' : 'none';
}

function bottleneckExecCount(q) {
    return parseInt(q && (q.execution_count != null ? q.execution_count : (q.total_executions != null ? q.total_executions : q.executions)) || 0, 10) || 0;
}

function sortBottleneckQueries(rows, state) {
    if (!rows || !rows.length) return [];
    const key = (state && state.key) || 'total_cpu_ms';
    const dir = (state && state.dir) === 'asc' ? 1 : -1;
    const textKeys = { query_text: true, login_name: true, database_name: true, program_name: true };
    return [...rows].sort((a, b) => {
        if (textKeys[key]) {
            const va = String((a && a[key]) || '').toLowerCase();
            const vb = String((b && b[key]) || '').toLowerCase();
            return dir * va.localeCompare(vb);
        }
        if (key === 'execution_count') {
            const va = bottleneckExecCount(a);
            const vb = bottleneckExecCount(b);
            if (va === vb) return 0;
            return dir * (va < vb ? -1 : 1);
        }
        const va = Number(a && a[key] != null ? a[key] : 0);
        const vb = Number(b && b[key] != null ? b[key] : 0);
        if (va === vb) return 0;
        return dir * (va < vb ? -1 : 1);
    });
}

function updateBottlenecksHeaderSortIndicators() {
    const table = document.getElementById('bottlenecksTable');
    if (!table) return;
    const state = window.appState.bottlenecksGridSort || { key: 'total_cpu_ms', dir: 'desc' };
    table.querySelectorAll('thead th[data-sort-key]').forEach((th) => {
        const k = th.getAttribute('data-sort-key');
        const icon = th.querySelector('.sort-icon');
        if (!icon) return;
        if (k === state.key) {
            th.style.color = 'var(--accent, #3b82f6)';
            icon.className = 'fa-solid sort-icon ' + (state.dir === 'asc' ? 'fa-sort-up' : 'fa-sort-down');
        } else {
            th.style.color = '';
            icon.className = 'fa-solid fa-sort sort-icon';
        }
    });
}

window.onBottlenecksGridSortClick = function(ev) {
    const th = ev.target.closest('th[data-sort-key]');
    if (!th || !document.getElementById('bottlenecksTable') || !th.closest('#bottlenecksTable')) return;
    ev.preventDefault();
    const key = th.getAttribute('data-sort-key');
    const state = Object.assign({}, window.appState.bottlenecksGridSort || { key: 'total_cpu_ms', dir: 'desc' });
    if (state.key === key) {
        state.dir = state.dir === 'asc' ? 'desc' : 'asc';
    } else {
        state.key = key;
        const textKeys = { query_text: true, login_name: true, database_name: true, program_name: true };
        state.dir = textKeys[key] ? 'asc' : 'desc';
    }
    window.appState.bottlenecksGridSort = state;
    if (window.bottlenecksLastRawQueries && window.bottlenecksLastRawQueries.length >= 0) {
        window.renderBottlenecksTable(window.bottlenecksLastRawQueries);
    }
};

function bindBottlenecksGridSort() {
    const table = document.getElementById('bottlenecksTable');
    if (!table || table.dataset.sortDelegateBound === '1') return;
    table.dataset.sortDelegateBound = '1';
    const thead = table.querySelector('thead');
    if (thead) thead.addEventListener('click', window.onBottlenecksGridSortClick);
}

window.HistoricalBottlenecksView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...', type: 'sqlserver'};
    window.appState.currentInstanceName = inst.name;
    window.appState.bottlenecksGridSort = window.appState.bottlenecksGridSort || { key: 'total_cpu_ms', dir: 'desc' };
    const savedRange = window.appState.bottlenecksTimeRange || '1h';
    const sel15 = savedRange === '15m' ? 'selected' : '';
    const sel1h = savedRange === '1h' ? 'selected' : '';
    const sel6 = savedRange === '6h' ? 'selected' : '';
    const sel24 = savedRange === '24h' ? 'selected' : '';
    const sel7 = savedRange === '7d' ? 'selected' : '';

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme">
            <div class="page-title flex-between">
                <div class="flex-between" style="align-items:center; gap:1rem; flex-wrap:wrap;">
                    <button type="button" class="btn btn-sm btn-outline" style="padding:0.3rem 0.6rem;" onclick="window.appNavigate('dashboard')" title="Back to dashboard"><i class="fa-solid fa-arrow-left"></i> Back</button>
                    <div>
                        <h1 style="font-size:1.35rem; margin:0;">Historical Query Bottlenecks</h1>
                        <p class="subtitle" style="margin:0.25rem 0 0 0;">Instance: ${window.escapeHtml(inst.name)} · Query Store · click column headers to sort</p>
                    </div>
                </div>
                <div style="display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap;">
                    <label class="text-muted" style="font-size:0.75rem; margin:0;">Time range</label>
                    <select id="bottleneckTimeRange" class="custom-select" style="padding: 0.35rem 0.5rem; font-size: 0.8rem; min-width: 160px;">
                        <option value="15m" ${sel15}>Last 15 minutes</option>
                        <option value="1h" ${sel1h}>Last 1 hour</option>
                        <option value="6h" ${sel6}>Last 6 hours</option>
                        <option value="24h" ${sel24}>Last 24 hours</option>
                        <option value="7d" ${sel7}>Last 7 days</option>
                    </select>
                    <button class="btn btn-sm btn-outline text-accent" onclick="window.refreshBottlenecks()">
                        <i class="fa-solid fa-refresh"></i> Refresh
                    </button>
                </div>
            </div>
            
            <div class="glass-panel mt-3" style="padding: 0.75rem;">
                <div class="flex-between" style="margin-bottom: 0.5rem;">
                    <h3 style="font-size: 0.85rem; margin: 0;">
                        <i class="fa-solid fa-fire text-danger"></i> Top Resource-Consuming Queries
                    </h3>
                    <span id="bottleneckCount" class="badge badge-info">Loading...</span>
                </div>
                
                <div class="table-responsive" style="max-height: calc(100vh - 280px); overflow-y: auto;">
                    <table class="data-table" id="bottlenecksTable" style="font-size: 0.75rem;">
                        <thead>
                            <tr>
                                <th style="min-width: 40px;">#</th>
                                <th data-sort-key="query_text" style="min-width: 200px; cursor:pointer; user-select:none;" title="Sort">Query Text <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                                <th id="bottlenecks-th-login" data-sort-key="login_name" style="min-width: 80px; display: none; cursor:pointer; user-select:none;" title="Sort">Login <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                                <th data-sort-key="database_name" style="min-width: 80px; cursor:pointer; user-select:none;" title="Sort">Database <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                                <th id="bottlenecks-th-app" data-sort-key="program_name" style="min-width: 60px; display: none; cursor:pointer; user-select:none;" title="Sort">App <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                                <th data-sort-key="execution_count" style="min-width: 80px; cursor:pointer; user-select:none;" title="Sort">Executions <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                                <th data-sort-key="avg_cpu_ms" style="min-width: 80px; cursor:pointer; user-select:none;" title="Sort">Avg CPU (ms) <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                                <th data-sort-key="avg_duration_ms" style="min-width: 100px; cursor:pointer; user-select:none;" title="Sort">Avg Duration (ms) <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                                <th data-sort-key="avg_logical_reads" style="min-width: 100px; cursor:pointer; user-select:none;" title="Sort">Avg Logical Reads <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                                <th data-sort-key="total_cpu_ms" style="min-width: 80px; cursor:pointer; user-select:none;" title="Sort">Total CPU (ms) <i class="fa-solid fa-sort sort-icon" style="opacity:0.65;font-size:0.65rem;"></i></th>
                            </tr>
                        </thead>
                        <tbody id="bottlenecksBody">
                            <tr><td colspan="10" class="text-center"><div class="spinner"></div> Loading query bottlenecks...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
            
            <div class="glass-panel mt-3" style="padding: 1rem;">
                <h3 style="font-size: 0.9rem; margin: 0 0 0.75rem 0;">
                    <i class="fa-solid fa-info-circle text-accent"></i> About Query Store Bottlenecks
                </h3>
                <div style="font-size: 0.75rem; color: var(--text-secondary, #888); line-height: 1.6;">
                    <p>This view displays aggregated statistics from SQL Server's Query Store, showing the most resource-intensive queries based on total CPU consumption.</p>
                    <ul style="margin: 0; padding-left: 1.25rem;">
                        <li id="bottlenecks-about-login" style="display: none;"><strong>Login</strong>: The SQL Server login (shown when live/session-enriched data includes it)</li>
                        <li><strong>Database</strong>: The database context where the query ran</li>
                        <li id="bottlenecks-about-app" style="display: none;"><strong>App</strong>: Client application name (shown when live/session-enriched data includes it)</li>
                        <li><strong>Executions</strong>: Total number of times the query was executed during the selected time range</li>
                        <li><strong>Avg CPU (ms)</strong>: Average CPU time in milliseconds per execution</li>
                        <li><strong>Avg Duration (ms)</strong>: Average total execution time in milliseconds</li>
                        <li><strong>Avg Logical Reads</strong>: Average number of logical reads per execution</li>
                        <li><strong>Total CPU (ms)</strong>: Total CPU time (Avg CPU × Executions)</li>
                    </ul>
                </div>
            </div>
        </div>
    `;

    document.getElementById('bottleneckTimeRange').addEventListener('change', function() {
        window.appState.bottlenecksTimeRange = document.getElementById('bottleneckTimeRange').value;
        window.refreshBottlenecks();
    });
    
    await window.loadBottlenecks();
};

window.loadBottlenecks = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx];
    if (!inst || inst.type !== 'sqlserver') {
        document.getElementById('bottlenecksBody').innerHTML = '<tr><td colspan="10" class="text-center text-warning">Query Store is only available for SQL Server instances.</td></tr>';
        return;
    }

    const timeRangeEl = document.getElementById('bottleneckTimeRange');
    const timeRange = timeRangeEl ? timeRangeEl.value : '1h';
    window.appState.bottlenecksTimeRange = timeRange;
    const apiTimeRange = timeRange;
    const dbQ = (typeof window.dashboardDatabaseQueryParam === 'function') ? window.dashboardDatabaseQueryParam() : '';
    
    try {
        // First try Query Store API (historical data from TimescaleDB)
        const response = await window.apiClient.authenticatedFetch(
            `/api/queries/bottlenecks?instance=${encodeURIComponent(inst.name)}&time_range=${encodeURIComponent(apiTimeRange)}&limit=50${dbQ}`
        );
        
        if (!response.ok) {
            throw new Error(`API error: ${response.status}`);
        }
        
        const data = await response.json();
        console.log('[Bottlenecks] Query Store data:', JSON.stringify(data).substring(0, 500));
        
        // If Query Store has data, use it (API returns `bottlenecks`)
        let queries = data.bottlenecks || data.queries || [];
        
        // Only fallback to live data if TimescaleDB table is empty
        if (queries.length === 0) {
            console.log('[Bottlenecks] Query Store table is empty, falling back to live top queries');
            // Fallback to live top queries from dashboard
            const liveResponse = await window.apiClient.authenticatedFetch(
                `/api/mssql/dashboard?instance=${encodeURIComponent(inst.name)}`
            );
            if (liveResponse.ok) {
                const liveData = await liveResponse.json();
                console.log('[Bottlenecks] Live dashboard data:', JSON.stringify(liveData).substring(0, 500));
                queries = liveData.top_queries || [];
            }
        }
        
        window.renderBottlenecksTable(queries);
        document.getElementById('bottleneckCount').textContent = `${queries.length} queries`;
    } catch (error) {
        console.error('Failed to load bottlenecks:', error);
        
        // Final fallback - try live dashboard
        try {
            const liveResponse = await window.apiClient.authenticatedFetch(
                `/api/mssql/dashboard?instance=${encodeURIComponent(inst.name)}`
            );
            if (liveResponse.ok) {
                const liveData = await liveResponse.json();
                const queries = liveData.top_queries || [];
                window.renderBottlenecksTable(queries);
                document.getElementById('bottleneckCount').textContent = `${queries.length} queries (live)`;
                return;
            }
        } catch (e) {
            console.error('Live fallback also failed:', e);
        }
        
        document.getElementById('bottlenecksBody').innerHTML = `<tr><td colspan="10" class="text-center text-danger">Failed to load: ${window.escapeHtml(error.message)}</td></tr>`;
        document.getElementById('bottleneckCount').textContent = 'Error';
    }
};

window.renderBottlenecksTable = function(queries) {
    const tbody = document.getElementById('bottlenecksBody');
    window.bottlenecksLastRawQueries = queries && queries.length ? [...queries] : [];
    const vis = bottleneckColumnVisibility(window.bottlenecksLastRawQueries);
    window.bottlenecksSessionColumnVis = vis;
    syncBottlenecksTableHeaders(vis.showLogin, vis.showApp);
    syncBottlenecksAboutHints(vis.showLogin, vis.showApp);

    const sortState = window.appState.bottlenecksGridSort || { key: 'total_cpu_ms', dir: 'desc' };
    const sortedQueries = sortBottleneckQueries(window.bottlenecksLastRawQueries, sortState);
    
    if (!sortedQueries || sortedQueries.length === 0) {
        tbody.innerHTML = `<tr><td colspan="${vis.colCount}" class="text-center text-muted"><i class="fa-solid fa-info-circle"></i> No query bottlenecks found in the selected time range. Query Store may not be enabled or no significant queries were captured.</td></tr>`;
        updateBottlenecksHeaderSortIndicators();
        bindBottlenecksGridSort();
        return;
    }
    
    tbody.innerHTML = sortedQueries.map((q, idx) => {
        const queryText = q.query_text || 'N/A';
        const truncatedText = queryText.length > 80 ? queryText.substring(0, 80) + '...' : queryText;
        const executions = bottleneckExecCount(q);
        const avgCpu = parseFloat(q.avg_cpu_ms || 0);
        const avgDuration = parseFloat(q.avg_duration_ms || 0);
        const avgReads = parseFloat(q.avg_logical_reads || 0);
        const totalCpu = parseFloat(q.total_cpu_ms || 0);
        const loginName = bottleneckRowHasLogin(q) ? String(q.login_name).trim() : '';
        const databaseName = q.database_name || 'N/A';
        const appName = bottleneckRowHasApp(q) ? String(q.program_name || q.application_name).trim() : '';
        
        // Color coding based on severity
        const cpuClass = avgCpu > 1000 ? 'text-danger' : avgCpu > 100 ? 'text-warning' : '';
        const durationClass = avgDuration > 10000 ? 'text-danger' : avgDuration > 1000 ? 'text-warning' : '';
        const readsClass = avgReads > 100000 ? 'text-danger' : avgReads > 10000 ? 'text-warning' : '';
        
        // Store query data in global object for modal access
        window.bottleneckQueryData = window.bottleneckQueryData || {};
        window.bottleneckQueryData[idx] = q;
        
        const loginTd = bottleneckSessionTd(vis.showLogin, loginName || '—');
        const appTd = bottleneckSessionTd(vis.showApp, appName || '—');
        
        return `
            <tr onclick="window.showBottleneckDetail(${idx})" style="cursor: pointer;">
                <td><strong>${idx + 1}</strong></td>
                <td style="max-width: 250px;">
                    <span class="code-snippet" style="cursor: pointer; color: var(--accent);" title="${window.escapeHtml(queryText)}" onclick="event.stopPropagation(); window.showBottleneckModal('${window.escapeHtml(queryText.replace(/'/g, "\\'"))}', ${idx})">
                        ${window.escapeHtml(truncatedText)}
                    </span>
                </td>
                ${loginTd}
                <td style="font-size:0.7rem;">${window.escapeHtml(databaseName)}</td>
                ${appTd}
                <td><span class="badge badge-outline">${executions.toLocaleString()}</span></td>
                <td class="${cpuClass}"><strong>${avgCpu.toFixed(2)}</strong></td>
                <td class="${durationClass}">${avgDuration.toFixed(2)}</td>
                <td class="${readsClass}">${avgReads.toLocaleString(undefined, {maximumFractionDigits: 0})}</td>
                <td class="text-danger"><strong>${totalCpu.toFixed(2)}</strong></td>
            </tr>
        `;
    }).join('');

    updateBottlenecksHeaderSortIndicators();
    bindBottlenecksGridSort();
};

window.showBottleneckDetail = function(idx) {
    const q = window.bottleneckQueryData && window.bottleneckQueryData[idx];
    if (!q) return;
    const qt = q.query_text || '';
    window.showBottleneckModal(qt, idx);
};

window.showBottleneckModal = function(queryText, queryIdx) {
    const existingModal = document.getElementById('bottleneck-modal');
    if (existingModal) existingModal.remove();
    
    // Handle case where queryIdx might be a number (new) or undefined/string (old)
    const queryData = (typeof queryIdx === 'number') ? (window.bottleneckQueryData && window.bottleneckQueryData[queryIdx]) : null;
    
    const hasLogin = queryData && typeof queryData === 'object' && bottleneckRowHasLogin(queryData);
    const hasApp = queryData && typeof queryData === 'object' && bottleneckRowHasApp(queryData);
    const loginName = hasLogin ? String(queryData.login_name).trim() : '';
    const appName = hasApp ? String(queryData.program_name || queryData.application_name || '').trim() : '';
    const databaseName = (queryData && typeof queryData === 'object') ? (queryData.database_name || 'N/A') : 'N/A';
    const executions = (queryData && typeof queryData === 'object') ? (queryData.execution_count || queryData.total_executions || 0) : 0;
    const avgDuration = (queryData && typeof queryData === 'object') ? (queryData.avg_duration_ms || 0) : 0;
    const avgCpu = (queryData && typeof queryData === 'object') ? (queryData.avg_cpu_ms || 0) : 0;
    const avgReads = (queryData && typeof queryData === 'object') ? (queryData.avg_logical_reads || 0) : 0;
    
    const modal = document.createElement('div');
    modal.id = 'bottleneck-modal';
    modal.style.cssText = 'display: flex; position: fixed; z-index: 99999; left: 0; top: 0; width: 100%; height: 100%; background-color: rgba(0,0,0,0.8); align-items: center; justify-content: center;';
    
    modal.innerHTML = `
        <div style="background: var(--bg-surface); margin: 2%; padding: 20px; border: 1px solid var(--border-color, #333); border-radius: 12px; width: 95%; max-width: 1000px; max-height: 90vh; overflow-y: auto; color: var(--text-primary, #e0e0e0); font-family: inherit; box-shadow: 0 4px 20px rgba(0,0,0,0.5);">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; border-bottom: 1px solid var(--border-color, #333); padding-bottom: 0.75rem;">
                <h3 style="margin: 0; color: var(--accent, #3b82f6); font-size: 1.1rem;"><i class="fa-solid fa-code"></i> Query Details</h3>
                <button onclick="document.getElementById('bottleneck-modal').remove()" style="background: transparent; border: 1px solid var(--border-color, #555); color: var(--text-primary, #e0e0e0); font-size: 1.25rem; cursor: pointer; padding: 0.25rem 0.6rem; border-radius: 4px; line-height: 1;">&times;</button>
            </div>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-bottom: 1rem;">
                ${(hasLogin || hasApp) ? `
                <div class="glass-panel" style="padding: 0.75rem; background: var(--bg-tertiary); grid-column: 1 / -1;">
                    <div style="font-size: 0.75rem; color: var(--text-muted); margin-bottom: 0.35rem;">Session context</div>
                    ${hasLogin ? `<div style="font-size: 0.9rem;"><span style="color: var(--text-muted); font-size: 0.75rem;">Login</span><br/>${window.escapeHtml(loginName)}</div>` : ''}
                    ${hasLogin && hasApp ? '<div style="height: 0.5rem;"></div>' : ''}
                    ${hasApp ? `<div style="font-size: 0.9rem;"><span style="color: var(--text-muted); font-size: 0.75rem;">Application</span><br/>${window.escapeHtml(appName)}</div>` : ''}
                </div>` : ''}
                <div class="glass-panel" style="padding: 0.75rem; background: var(--bg-tertiary);">
                    <div style="font-size: 0.75rem; color: var(--text-muted);">Database</div>
                    <div style="font-size: 0.9rem;">${window.escapeHtml(databaseName)}</div>
                </div>
                <div class="glass-panel" style="padding: 0.75rem; background: var(--bg-tertiary);">
                    <div style="font-size: 0.75rem; color: var(--text-muted);">Total Executions</div>
                    <div style="font-size: 0.9rem;">${parseInt(executions).toLocaleString()}</div>
                </div>
                <div class="glass-panel" style="padding: 0.75rem; background: var(--bg-tertiary);">
                    <div style="font-size: 0.75rem; color: var(--text-muted);">Avg CPU (ms)</div>
                    <div style="font-size: 0.9rem;">${parseFloat(avgCpu).toFixed(2)}</div>
                </div>
                <div class="glass-panel" style="padding: 0.75rem; background: var(--bg-tertiary);">
                    <div style="font-size: 0.75rem; color: var(--text-muted);">Avg Duration (ms)</div>
                    <div style="font-size: 0.9rem;">${parseFloat(avgDuration).toFixed(2)}</div>
                </div>
                <div class="glass-panel" style="padding: 0.75rem; background: var(--bg-tertiary);">
                    <div style="font-size: 0.75rem; color: var(--text-muted);">Avg Logical Reads</div>
                    <div style="font-size: 0.9rem;">${parseInt(avgReads).toLocaleString()}</div>
                </div>
            </div>
            <div style="margin-bottom: 1rem;">
                <h4 style="margin: 0 0 0.75rem 0; color: var(--accent, #3b82f6); font-size: 0.9rem;">Query Text</h4>
                <div style="background: var(--bg-base); padding: 1rem; border-radius: 8px; max-height: 40vh; overflow: auto; border: 1px solid var(--border-color, #333);">
                    <pre style="margin: 0; white-space: pre-wrap; word-wrap: break-word; color: var(--text-primary, #e0e0e0); font-family: 'Courier New', monospace; font-size: 0.85rem; line-height: 1.5;">${window.escapeHtml(queryText)}</pre>
                </div>
            </div>
            <div style="text-align: center; margin-top: 1rem;">
                <button id="copyBottleneckSql" style="background: var(--accent, #3b82f6); color: #fff; border: none; padding: 0.5rem 1.5rem; border-radius: 6px; cursor: pointer; font-size: 0.9rem;">
                    <i class="fa-solid fa-copy"></i> Copy SQL
                </button>
            </div>
        </div>
    `;
    
    document.body.appendChild(modal);
    
    document.getElementById('copyBottleneckSql').addEventListener('click', function() {
        navigator.clipboard.writeText(queryText).then(() => {
            this.innerHTML = '<i class="fa-solid fa-check"></i> Copied!';
            setTimeout(() => {
                this.innerHTML = '<i class="fa-solid fa-copy"></i> Copy SQL';
            }, 1500);
        });
    });
    
    modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.remove();
    });
};

window.refreshBottlenecks = async function() {
    const selectEl = document.getElementById('bottleneckTimeRange');
    if (selectEl) {
        window.appState.bottlenecksTimeRange = selectEl.value;
    }
    
    const tbody = document.getElementById('bottlenecksBody');
    if (tbody) {
        tbody.innerHTML = '<tr><td colspan="10" class="text-center"><div class="spinner"></div> Loading query bottlenecks...</td></tr>';
    }
    
    await window.loadBottlenecks();
};
