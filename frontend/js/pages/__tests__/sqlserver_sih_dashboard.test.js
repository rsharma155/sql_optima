/**
 * @jest-environment jsdom
 */
const { runSqlServerStorageIndexHealthDashboard } = require('../sqlserver_storage_index_health_dashboard.js');

// Mock sihShared as it's a global in the browser but we need it here
global.sihShared = {
    buildFilterQS: jest.fn(() => 'instance=test'),
    buildHealthScore: jest.fn(() => 95),
    fmt: jest.fn(v => v),
    renderBanner: jest.fn(),
    renderInsights: jest.fn(),
    renderProjectionStrip: jest.fn(),
    renderHighScanTables: jest.fn(),
    renderLargestTables: jest.fn(),
    renderLargestIndexes: jest.fn(),
    renderIndexEfficiency: jest.fn(),
    renderDuplicateIndexes: jest.fn(),
    wireCopyButtons: jest.fn(),
    renderSparkline: jest.fn(),
    renderGrowthChart: jest.fn(),
    renderTopGrowthChart: jest.fn(),
    renderSeekScanLookupChart: jest.fn()
};

// Mock apiClient
global.apiClient = {
    authenticatedFetch: jest.fn(() => Promise.resolve({
        ok: true,
        json: () => Promise.resolve({
            kpis: { total_db_size_mb: 1024, growth_7d_pct: 5, forecast_table_mb_90d: 1200, unused_index_count: 2, unused_index_mb: 150, high_scan_table_count: 1, index_write_overhead_pct: 10 },
            growth_summary: { daily_growth_mb: 5 },
            growth: [],
            insights: []
        })
    }))
};

// Mock appState
global.appState = {
    config: { instances: [{ name: 'test-inst', type: 'sqlserver' }] },
    currentInstanceIdx: 0,
    msSih: {}
};

// Mock loadTemplate
global.loadTemplate = jest.fn(() => Promise.resolve('<div id="sihDashboardBody"></div><div id="sihHealthScoreContainer"></div><div id="kpiTotalSize"></div><div id="cardTotalSize"></div><div id="kpiGrowth7d"></div><div id="cardGrowth7d"></div><div id="kpiForecast90d"></div><div id="cardForecast90d"></div><div id="kpiWriteOverhead"></div><div id="cardWriteOverhead"></div><div id="kpiUnusedCount"></div><div id="cardUnusedCount"></div><div id="kpiUnusedMB"></div><div id="cardUnusedMB"></div><div id="kpiHighScan"></div><div id="cardHighScan"></div><div id="kpiDailyGrowth"></div><div id="cardDailyGrowth"></div><div id="sihInsightsBody"></div><div id="highScanTablesBody"></div><div id="largestTablesBody"></div><div id="largestIndexesBody"></div><div id="indexEfficiencyBody"></div><div id="dupIdxBody"></div><div id="dupIdxCount"></div><button id="sihRefresh"></button><button id="sihApply"></button><select id="sihDb"></select><select id="sihSchema"></select><select id="sihTable"></select>'));

// Mock routerOutlet
document.body.innerHTML = '<div id="router-outlet"></div>';
global.routerOutlet = document.getElementById('router-outlet');

describe('SQL Server SIH Dashboard Integration', () => {
    beforeEach(() => {
        jest.clearAllMocks();
    });

    test('orchestrator calls all 8+ render functions on success', async () => {
        await runSqlServerStorageIndexHealthDashboard();
        
        expect(global.sihShared.renderBanner).toHaveBeenCalled();
        expect(global.sihShared.renderInsights).toHaveBeenCalled();
        expect(global.sihShared.renderProjectionStrip).toHaveBeenCalled();
        expect(global.sihShared.renderHighScanTables).toHaveBeenCalled();
        expect(global.sihShared.renderLargestTables).toHaveBeenCalled();
        expect(global.sihShared.renderLargestIndexes).toHaveBeenCalled();
        expect(global.sihShared.renderIndexEfficiency).toHaveBeenCalled();
        expect(global.sihShared.renderDuplicateIndexes).toHaveBeenCalled();
    });

    test('kpiTotalSize is populated from total_db_size_mb', async () => {
        await runSqlServerStorageIndexHealthDashboard();
        // 1024 MB / 1024 = 1.0 GB
        expect(document.getElementById('kpiTotalSize').textContent).toContain('1');
    });

    test('error path removes loading class', async () => {
        global.apiClient.authenticatedFetch.mockImplementationOnce(() => Promise.reject('API Down'));
        
        await runSqlServerStorageIndexHealthDashboard();
        
        const container = document.getElementById('sihDashboardBody');
        expect(container.classList.contains('loading')).toBe(false);
        expect(container.innerHTML).toContain('Analytical load failed');
    });
});
