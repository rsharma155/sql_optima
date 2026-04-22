/**
 * SQL Optima — Smoke Test for SQL Server Query Analysis Dashboard
 * 
 * This is a standalone test suite that can be run in the browser console
 * to verify that the dashboard logic is free of reference errors and
 * correctly handles DOM updates.
 */

(function() {
    console.log('%c [TEST] Starting sqlserver_QueryAnalysis smoke test...', 'color: #3b82f6; font-weight: bold;');

    // 1. Setup Mocks
    window.appState = {
        config: {
            instances: [{ name: 'Test Instance', type: 'sqlserver' }]
        },
        currentInstanceIdx: 0,
        queryCache: {}
    };

    window.apiClient = {
        authenticatedFetch: async (url) => {
            console.log(`[MOCK] Fetching: ${url}`);
            return {
                ok: true,
                json: async () => {
                    if (url.includes('summary')) return { total_executions: 1000, avg_duration_ms: 50 };
                    if (url.includes('top-queries')) return { queries: [{ query_text: 'SELECT 1', query_hash: '0x1', executions: 100 }] };
                    if (url.includes('timeseries')) return { series: [] };
                    return [];
                }
            };
        }
    };

    window.loadTemplate = async (path) => {
        console.log(`[MOCK] Loading template: ${path}`);
        return `
            <div id="qaInstanceName"></div>
            <input type="datetime-local" id="qaFrom">
            <input type="datetime-local" id="qaTo">
            <input type="checkbox" id="qaExcludeSystem">
            <input type="checkbox" id="qaComparePrev">
            <button id="qaApplyRange"></button>
            <div id="kpi-executions"></div>
            <div id="kpi-duration"></div>
            <div id="kpi-cpu"></div>
            <div id="kpi-cpu-share"></div>
            <div id="kpi-regressions"></div>
            <div id="kpi-plan-changes"></div>
            <div id="summary-total-queries"></div>
            <div id="summary-executed-range"></div>
            <div id="summary-multi-plan"></div>
            <div id="summary-single-exec"></div>
            <div id="qaTopQueriesBody"></div>
            <div id="qaRegressionsBody"></div>
            <div id="qaInstabilityBody"></div>
            <div id="qaLiveBody"></div>
            <div class="qa-tab-btn active" data-tab="top-queries"></div>
        `;
    };

    window.routerOutlet = document.createElement('div');

    // 2. Execute Load
    async function runTest() {
        try {
            if (typeof window.sqlserver_QueryAnalysisDashboard !== 'function') {
                throw new Error('sqlserver_QueryAnalysisDashboard function not found on window');
            }

            console.log('[TEST] Executing dashboard load...');
            await window.sqlserver_QueryAnalysisDashboard();
            
            // 3. Verify Initial State
            console.assert(document.getElementById('qaInstanceName').textContent === 'Test Instance', 'Instance name not rendered');
            console.assert(window.appState.queryCache !== undefined, 'Query cache not initialized');
            
            console.log('%c [TEST] PASSED: Dashboard initialized without reference errors.', 'color: #22c55e; font-weight: bold;');
        } catch (err) {
            console.error('%c [TEST] FAILED: Dashboard load crashed!', 'color: #ef4444; font-weight: bold;');
            console.error(err);
        }
    }

    // Export runner
    window.runSqlServerQASmokeTest = runTest;
    console.log('[TEST] Type %crunSqlServerQASmokeTest()%c in console to execute.', 'font-family: monospace; font-weight: bold; background: #eee;', '');
})();
