/**
 * Visual-only mock (no API). Not loaded in index.html by default — see project_details.md ("Optional Sentinel mock").
 * Route: window.appNavigate('sentinel-mock') after adding this script to index.html (after router.js).
 */
window.SentinelMockView = function SentinelMockView() {
    const inst = window.appState.config?.instances?.[window.appState.currentInstanceIdx] || { name: 'Demo Instance', type: 'sqlserver' };
    const dbType = (inst.type === 'postgres') ? 'PostgreSQL' : 'SQL Server';

    // This is a visual-only mock inspired by /dba-sentinel-dashboard. No API calls.
    window.routerOutlet.innerHTML = `
        <div class="page-view active">
            <div class="glass-panel" style="padding:0.75rem; border-radius:16px; background:rgba(255,255,255,0.06);">
                <div style="display:flex; justify-content:space-between; align-items:center; gap:1rem; flex-wrap:wrap;">
                    <div style="display:flex; align-items:center; gap:0.75rem;">
                        <div style="width:36px; height:36px; border-radius:10px; display:flex; align-items:center; justify-content:center; background:rgba(59,130,246,0.15); border:1px solid rgba(59,130,246,0.25);">
                            <i class="fa-solid fa-shield-halved text-accent"></i>
                        </div>
                        <div>
                            <div style="font-weight:700; letter-spacing:0.04em; font-size:0.95rem;">DBA SENTINEL (Mock Preview)</div>
                            <div class="text-muted" style="font-size:0.75rem;">${window.escapeHtml(dbType)} • ${window.escapeHtml(inst.name)} • visual-only preview</div>
                        </div>
                    </div>
                    <div style="display:flex; align-items:center; gap:0.5rem; flex-wrap:wrap; justify-content:flex-end;">
                        <span class="badge badge-outline" style="font-size:0.65rem; letter-spacing:0.12em; text-transform:uppercase;">Live Monitoring</span>
                        <span class="badge badge-info" style="font-size:0.65rem;">Source: Timescale snapshot</span>
                        <button class="btn btn-sm btn-outline text-accent" onclick="window.appNavigate('dashboard')"><i class="fa-solid fa-arrow-left"></i> Back</button>
                    </div>
                </div>
            </div>

            <div class="mt-4" style="display:grid; grid-template-columns:1fr; gap:0.75rem;">
                <div class="glass-panel" style="padding:0.75rem; border-radius:16px;">
                    <div style="display:grid; grid-template-columns:1fr 2fr; gap:0.75rem; align-items:stretch;">
                        <div class="glass-panel" style="padding:0.75rem; border-radius:16px;">
                            <div class="text-muted" style="font-size:0.7rem; letter-spacing:0.12em; text-transform:uppercase;">Health Score</div>
                            <div style="display:flex; align-items:flex-end; gap:0.5rem; margin-top:0.5rem;">
                                <div style="font-size:2.25rem; font-weight:800; font-family:'JetBrains Mono', monospace;">92</div>
                                <span class="badge badge-success" style="font-size:0.65rem;">Healthy</span>
                            </div>
                            <div class="text-muted" style="font-size:0.75rem; margin-top:0.25rem;">High DBA confidence signals</div>
                            <div style="margin-top:0.75rem; display:flex; gap:0.4rem; flex-wrap:wrap;">
                                <span class="badge badge-outline" style="font-size:0.65rem;">Blocking: 0</span>
                                <span class="badge badge-outline" style="font-size:0.65rem;">TempDB: 12%</span>
                                <span class="badge badge-outline" style="font-size:0.65rem;">Max Log: 18%</span>
                            </div>
                        </div>

                        <div class="glass-panel" style="padding:0.75rem; border-radius:16px;">
                            <div style="display:flex; justify-content:space-between; align-items:center;">
                                <div class="text-muted" style="font-size:0.7rem; letter-spacing:0.12em; text-transform:uppercase;">Key Signals</div>
                                <span class="text-muted" style="font-size:0.7rem;">Last update: --:--:--</span>
                            </div>
                            <div style="margin-top:0.5rem; display:grid; grid-template-columns:repeat(4, minmax(0,1fr)); gap:0.5rem;">
                                <div class="glass-panel" style="padding:0.6rem; border-radius:14px;">
                                    <div class="text-muted" style="font-size:0.65rem; text-transform:uppercase; letter-spacing:0.1em;">CPU</div>
                                    <div style="font-family:'JetBrains Mono', monospace; font-size:1.15rem; font-weight:700;">34%</div>
                                </div>
                                <div class="glass-panel" style="padding:0.6rem; border-radius:14px;">
                                    <div class="text-muted" style="font-size:0.65rem; text-transform:uppercase; letter-spacing:0.1em;">Memory</div>
                                    <div style="font-family:'JetBrains Mono', monospace; font-size:1.15rem; font-weight:700;">62%</div>
                                </div>
                                <div class="glass-panel" style="padding:0.6rem; border-radius:14px;">
                                    <div class="text-muted" style="font-size:0.65rem; text-transform:uppercase; letter-spacing:0.1em;">Throughput</div>
                                    <div style="font-family:'JetBrains Mono', monospace; font-size:1.15rem; font-weight:700;">145 TPS</div>
                                </div>
                                <div class="glass-panel" style="padding:0.6rem; border-radius:14px;">
                                    <div class="text-muted" style="font-size:0.65rem; text-transform:uppercase; letter-spacing:0.1em;">Waits</div>
                                    <div style="font-family:'JetBrains Mono', monospace; font-size:1.15rem; font-weight:700;">CPU / LCK</div>
                                </div>
                            </div>
                            <div class="text-muted" style="font-size:0.75rem; margin-top:0.6rem;">
                                This is a preview of a cleaner “Sentinel-style” layout using calmer cards, typography hierarchy, and consistent spacing.
                            </div>
                        </div>
                    </div>
                </div>

                <div class="glass-panel" style="padding:0.75rem; border-radius:16px;">
                    <div style="display:flex; justify-content:space-between; align-items:center;">
                        <div class="text-muted" style="font-size:0.7rem; letter-spacing:0.12em; text-transform:uppercase;">Top Offenders (Mock)</div>
                        <span class="badge badge-outline" style="font-size:0.65rem;">Drilldown-ready</span>
                    </div>
                    <div class="table-responsive" style="margin-top:0.5rem;">
                        <table class="data-table" style="font-size:0.75rem;">
                            <thead>
                                <tr>
                                    <th>#</th>
                                    <th>Query</th>
                                    <th>Total CPU</th>
                                    <th>Avg Dur</th>
                                    <th>Execs</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr class="data-row"><td>1</td><td><span class="code-snippet">SELECT ...</span></td><td>2213ms</td><td>18ms</td><td>12,404</td></tr>
                                <tr class="data-row"><td>2</td><td><span class="code-snippet">UPDATE ...</span></td><td>1410ms</td><td>32ms</td><td>2,104</td></tr>
                                <tr class="data-row"><td>3</td><td><span class="code-snippet">EXEC dbo.proc ...</span></td><td>903ms</td><td>44ms</td><td>401</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    `;
};

