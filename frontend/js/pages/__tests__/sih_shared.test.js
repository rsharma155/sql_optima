/**
 * @jest-environment jsdom
 */
const { sihShared } = require('../sih_shared.js');

// Mock Chart.js as it's used in sih_shared.js
global.Chart = class {
    constructor() {}
    static getChart() { return null; }
    destroy() {}
};

describe('sihShared.fmt', () => {
    test('formats null as --',         () => expect(sihShared.fmt(null)).toBe('--'));
    test('formats undefined as --',    () => expect(sihShared.fmt(undefined)).toBe('--'));
    test('formats 0 as 0',             () => expect(sihShared.fmt(0)).toBe('0'));
    test('formats 1234 with d=0',      () => expect(sihShared.fmt(1234)).toBe('1,234'));
    test('formats 3.14159 with d=2',   () => expect(sihShared.fmt(3.14159, 2)).toBe('3.14'));
});

describe('sihShared.buildHealthScore', () => {
    test('returns 100 for clean instance', () => {
        const kpis = { unused_index_count: 0, high_scan_table_count: 0, index_write_overhead_pct: 0, growth_7d_pct: 0 };
        expect(sihShared.buildHealthScore(kpis)).toBe(100);
    });
    test('penalises unused indexes up to 40 points', () => {
        const kpis = { unused_index_count: 30, high_scan_table_count: 0, index_write_overhead_pct: 0, growth_7d_pct: 0 };
        expect(sihShared.buildHealthScore(kpis)).toBe(60);
    });
    test('score never goes below 0', () => {
        const kpis = { unused_index_count: 100, high_scan_table_count: 50, index_write_overhead_pct: 90, growth_7d_pct: 50 };
        expect(sihShared.buildHealthScore(kpis)).toBeGreaterThanOrEqual(0);
    });
});

describe('sihShared.buildBannerMessages', () => {
    test('returns healthy message when all metrics are normal', () => {
        const msgs = sihShared.buildBannerMessages(
            { unused_index_count: 0, unused_index_mb: 0, growth_7d_pct: 1, index_write_overhead_pct: 5, high_scan_table_count: 0 },
            {}
        );
        expect(msgs).toHaveLength(1);
        expect(msgs[0].sev).toBe('healthy');
    });

    test('returns critical when unused_index_mb > 1024', () => {
        const msgs = sihShared.buildBannerMessages(
            { unused_index_count: 25, unused_index_mb: 1500, growth_7d_pct: 1, index_write_overhead_pct: 5, high_scan_table_count: 0 },
            {}
        );
        const crit = msgs.filter(m => m.sev === 'critical');
        expect(crit.length).toBeGreaterThan(0);
    });

    test('returns multiple messages for multiple conditions', () => {
        const msgs = sihShared.buildBannerMessages(
            { unused_index_count: 30, unused_index_mb: 2000, growth_7d_pct: 20, index_write_overhead_pct: 70, high_scan_table_count: 15 },
            {}
        );
        expect(msgs.length).toBeGreaterThanOrEqual(3);
    });
});

describe('sihShared.renderLargestTables', () => {
    beforeEach(() => {
        document.body.innerHTML = '<table><tbody id="testBody"></tbody></table>';
    });

    test('renders empty row when rows is empty', () => {
        sihShared.renderLargestTables('testBody', [], 'sqlserver', 'drilldown');
        expect(document.getElementById('testBody').innerHTML).toContain('No table data');
    });

    test('renders correct data-action attribute', () => {
        const rows = [{ value: 100, value2: 30, db_name: 'mydb', schema_name: 'dbo', table_name: 'orders' }];
        sihShared.renderLargestTables('testBody', rows, 'sqlserver', 'ms-sih-drilldown');
        const tr = document.querySelector('tr[data-action="ms-sih-drilldown"]');
        expect(tr).not.toBeNull();
        expect(tr.dataset.table).toBe('orders');
    });

    test('value2 <= value (index <= total)', () => {
        const rows = [{ value: 200, value2: 50, db_name: 'db', schema_name: 'dbo', table_name: 't1' }];
        sihShared.renderLargestTables('testBody', rows, 'sqlserver', 'drilldown');
        const cells = document.querySelectorAll('td');
        // cell[1]=totalMB=200, cell[2]=dataMB=150, cell[3]=idxMB=50
        expect(cells[1].textContent).toBe('200.0');
        expect(cells[2].textContent).toBe('150.0');
        expect(cells[3].textContent).toBe('50.0');
    });
});

describe('sihShared.renderIndexEfficiency', () => {
    beforeEach(() => {
        document.body.innerHTML = '<table><tbody id="testBody"></tbody></table>';
    });

    test('shows DROP badge when seeks=0', () => {
        const rows = [{ value: 0, value2: 150, index_name: 'idx_unused', schema_name: 'dbo', table_name: 'orders', last_user_seek: null }];
        sihShared.renderIndexEfficiency('testBody', rows, 'sqlserver');
        expect(document.querySelector('.badge-danger').textContent).toBe('DROP');
    });

    test('shows HEALTHY badge when seeks > 5', () => {
        const rows = [{ value: 100, value2: 50, index_name: 'idx_used', schema_name: 'dbo', table_name: 'orders', last_user_seek: '2026-05-14T10:00:00Z' }];
        sihShared.renderIndexEfficiency('testBody', rows, 'sqlserver');
        expect(document.querySelector('.badge-success').textContent).toBe('HEALTHY');
    });

    test('shows copy button for DROP recommendation', () => {
        const rows = [{ value: 0, value2: 150, index_name: 'idx_unused', schema_name: 'dbo', table_name: 'orders', last_user_seek: null }];
        sihShared.renderIndexEfficiency('testBody', rows, 'sqlserver');
        const btn = document.querySelector('.sih-copy-drop');
        expect(btn).not.toBeNull();
        expect(btn.dataset.sql).toContain('DROP INDEX');
        expect(btn.dataset.sql).toContain('idx_unused');
    });

    test('generates correct SQL Server DROP syntax', () => {
        const rows = [{ value: 0, value2: 50, index_name: 'idx_x', schema_name: 'dbo', table_name: 'users', last_user_seek: null }];
        sihShared.renderIndexEfficiency('testBody', rows, 'sqlserver');
        const sql = document.querySelector('.sih-copy-drop').dataset.sql;
        expect(sql).toBe('DROP INDEX [idx_x] ON [dbo].[users];');
    });

    test('generates correct PostgreSQL DROP syntax', () => {
        const rows = [{ value: 0, value2: 50, index_name: 'idx_x', schema_name: 'public', table_name: 'users', last_user_seek: null }];
        sihShared.renderIndexEfficiency('testBody', rows, 'postgres');
        const sql = document.querySelector('.sih-copy-drop').dataset.sql;
        expect(sql).toBe('DROP INDEX IF EXISTS public.idx_x;');
    });
});

describe('sihShared.renderProjectionStrip', () => {
    beforeEach(() => {
        document.body.innerHTML = `
            <span id="projCurrent"></span>
            <span id="projDailyRate"></span>
            <span id="proj30d"></span>
            <span id="proj90d"></span>
        `;
    });

    test('populates all 4 projection cells', () => {
        sihShared.renderProjectionStrip({
            growth_summary: {
                current_table_mb: 1024,
                current_index_mb: 200,
                daily_growth_mb: 15,
                growth_30d_pct: 3.5,
                projected_table_mb_90d: 1434
            }
        });
        expect(document.getElementById('projCurrent').textContent).toContain('1.0 GB');
        expect(document.getElementById('projDailyRate').textContent).toContain('15.0 MB/day');
        expect(document.getElementById('proj30d').textContent).toContain('3.5%');
        expect(document.getElementById('proj90d').textContent).toContain('1.4 GB');
    });
});
