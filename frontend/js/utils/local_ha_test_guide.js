/**
 * Local HA test cluster guide for onboarding (no live PostgreSQL / SQL Server).
 * Keep in sync with docs/QUICKSTART.md § Local test environment: HA clusters for development
 */
(function() {
    const HA_REPO = 'https://github.com/rsharma155/sqlserver_postgres_ha_cluster';
    const QUICKSTART_ANCHOR = 'https://github.com/rsharma155/sql_optima/blob/main/docs/QUICKSTART.md#local-test-environment-ha-clusters-for-development';

    function guideBodyHtml() {
        return `
            <p class="text-muted" style="font-size:0.9rem; line-height:1.6; margin:0 0 1.25rem;">
                Spin up production-like <strong>PostgreSQL Patroni</strong> and <strong>SQL Server 3-node</strong> clusters on your laptop,
                plus a CRUD load generator — then register them here in SQL Optima.
            </p>
            <p style="margin:0 0 1.25rem;">
                <a href="${HA_REPO}" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-accent">
                    <i class="fa-brands fa-github"></i> sqlserver_postgres_ha_cluster
                </a>
            </p>

            <h4 style="font-size:0.95rem; margin:1.5rem 0 0.75rem; font-weight:700;">Prerequisites</h4>
            <ul class="text-muted" style="font-size:0.85rem; line-height:1.6; margin:0 0 1rem; padding-left:1.25rem;">
                <li>Docker 24+ and Docker Compose v2+</li>
                <li>Python 3.8+ (for the CRUD web app)</li>
                <li>SQL Server CRUD: launcher may install Microsoft ODBC 18 (<code>sudo</code> on Linux, <code>brew</code> on macOS)</li>
            </ul>

            <h4 style="font-size:0.95rem; margin:1.5rem 0 0.75rem; font-weight:700;">Step 1 — Clone and start HA clusters</h4>
            <pre style="margin:0 0 0.75rem; padding:1rem; background:#1e1e1e; color:#d4d4d4; border-radius:8px; font-size:0.78rem; overflow-x:auto; line-height:1.5; font-family:var(--font-mono); border:1px solid #333;"><code>git clone https://github.com/rsharma155/sqlserver_postgres_ha_cluster.git
cd sqlserver_postgres_ha_cluster/Postgres_SQLServer_Test_Servers

# Linux / macOS (interactive — choose PG, SQL Server, or both):
./start_servers.sh

# Windows PowerShell:
PowerShell -ExecutionPolicy Bypass -File .\\start_all.ps1</code></pre>
            <p class="text-muted" style="font-size:0.8rem; margin:0 0 1rem;">Allow ~60 seconds for containers to initialise. RAM limits scale with your system.</p>
            <pre style="margin:0 0 1rem; padding:0.75rem 1rem; background:rgba(0,0,0,0.04); border-radius:8px; font-size:0.75rem; overflow-x:auto; font-family:var(--font-mono);"><code># PostgreSQL only:  ./start_servers.sh --skip-sql-server
# SQL Server only:  ./start_servers.sh --skip-postgres</code></pre>

            <h4 style="font-size:0.95rem; margin:1.5rem 0 0.75rem; font-weight:700;">Step 2 — Verify clusters</h4>
            <pre style="margin:0 0 1rem; padding:1rem; background:#1e1e1e; color:#d4d4d4; border-radius:8px; font-size:0.78rem; overflow-x:auto; line-height:1.5; font-family:var(--font-mono); border:1px solid #333;"><code>./start_servers.sh --status          # Linux / macOS
.\\start_all.ps1 -Status              # Windows

psql -h localhost -p 5000 -U postgres -c "SELECT pg_is_in_recovery();"
# Expected: f (primary via HAProxy write port)

sqlcmd -S localhost,14331 -U sa -P 'S@L_2024_HADr_D0ck3r!' -Q "SELECT @@SERVERNAME"</code></pre>

            <h4 style="font-size:0.95rem; margin:1.5rem 0 0.75rem; font-weight:700;">Step 3 — Register in SQL Optima (this form)</h4>
            <p class="text-muted" style="font-size:0.85rem; margin:0 0 0.75rem;">Use the values below in <strong>Add New Server</strong> above.</p>

            <p style="font-size:0.8rem; font-weight:600; margin:0 0 0.5rem;">PostgreSQL — HAProxy write (primary)</p>
            <table class="data-table" style="font-size:0.8rem; margin-bottom:1rem; width:100%;">
                <tbody>
                    <tr><td style="padding:0.4rem 0.75rem;">Host</td><td><code>localhost</code></td></tr>
                    <tr><td style="padding:0.4rem 0.75rem;">Port</td><td><code>5000</code></td></tr>
                    <tr><td style="padding:0.4rem 0.75rem;">User / Password</td><td><code>postgres</code> / <code>postgres123</code></td></tr>
                    <tr><td style="padding:0.4rem 0.75rem;">Database</td><td><code>hotel_booking</code> (or any demo DB)</td></tr>
                </tbody>
            </table>
            <p class="text-muted" style="font-size:0.75rem; margin:0 0 1rem;">Read replica: port <code>5001</code>. Direct Patroni nodes: <code>5043</code>, <code>5044</code>, <code>5045</code>.</p>

            <p style="font-size:0.8rem; font-weight:600; margin:0 0 0.5rem;">SQL Server — Always On nodes (add each separately)</p>
            <table class="data-table" style="font-size:0.8rem; margin-bottom:1rem; width:100%;">
                <thead><tr><th>Node</th><th>Host</th><th>Port</th></tr></thead>
                <tbody>
                    <tr><td>sql1 (primary)</td><td><code>localhost</code></td><td><code>14331</code></td></tr>
                    <tr><td>sql2</td><td><code>localhost</code></td><td><code>14332</code></td></tr>
                    <tr><td>sql3</td><td><code>localhost</code></td><td><code>14333</code></td></tr>
                </tbody>
            </table>
            <p class="text-muted" style="font-size:0.75rem; margin:0 0 1rem;">Credentials: <code>sa</code> / <code>S@L_2024_HADr_D0ck3r!</code>. Recommended monitor login: <code>dbmonitor_user</code> / <code>Hello@123</code>.</p>

            <h4 style="font-size:0.95rem; margin:1.5rem 0 0.75rem; font-weight:700;">Step 4 — Generate load (optional)</h4>
            <p class="text-muted" style="font-size:0.85rem; margin:0 0 0.5rem;">
                Open <a href="http://localhost:5002" target="_blank" rel="noopener noreferrer"><strong>http://localhost:5002</strong></a> —
                select engine, set threads/duration, click <strong>Start CRUD Load</strong> (default 60% read / 40% write).
            </p>

            <h4 style="font-size:0.95rem; margin:1.5rem 0 0.75rem; font-weight:700;">Step 5 — Stop clusters</h4>
            <pre style="margin:0 0 1rem; padding:1rem; background:#1e1e1e; color:#d4d4d4; border-radius:8px; font-size:0.78rem; overflow-x:auto; font-family:var(--font-mono); border:1px solid #333;"><code>./stop_servers.sh
# Windows: PowerShell -ExecutionPolicy Bypass -File .\\stop_all.ps1</code></pre>

            <h4 style="font-size:0.95rem; margin:1.5rem 0 0.75rem; font-weight:700;">Connection reference</h4>
            <div class="table-responsive" style="max-height:220px; overflow:auto; border:1px solid var(--border-color); border-radius:8px;">
                <table class="data-table" style="font-size:0.72rem; width:100%; margin:0;">
                    <thead><tr><th>Engine</th><th>Port</th><th>Purpose</th><th>Credentials</th></tr></thead>
                    <tbody>
                        <tr><td>PostgreSQL</td><td>5000</td><td>HAProxy writes</td><td>postgres / postgres123</td></tr>
                        <tr><td>PostgreSQL</td><td>5001</td><td>HAProxy reads</td><td>postgres / postgres123</td></tr>
                        <tr><td>SQL Server</td><td>14331–14333</td><td>sql1–sql3</td><td>sa / S@L_2024_HADr_D0ck3r!</td></tr>
                        <tr><td>CRUD app</td><td>5002</td><td>Load generator UI</td><td>—</td></tr>
                    </tbody>
                </table>
            </div>

            <h4 style="font-size:0.95rem; margin:1.25rem 0 0.5rem; font-weight:700;">Troubleshooting</h4>
            <ul class="text-muted" style="font-size:0.8rem; line-height:1.5; margin:0; padding-left:1.25rem;">
                <li>Containers not starting → <code>docker logs patroni1</code> or <code>docker logs sql1</code></li>
                <li>Port in use → edit host ports in the HA repo <code>docker-compose.yml</code></li>
                <li>No historical data in SQL Optima → wait ~15 minutes after registering a server</li>
            </ul>
        `;
    }

    window.showLocalHaTestGuide = function(opts) {
        opts = opts || {};
        const existing = document.getElementById('local-ha-test-guide-modal');
        if (existing) existing.remove();

        const modal = document.createElement('div');
        modal.id = 'local-ha-test-guide-modal';
        modal.className = 'app-modal-overlay';
        modal.style.cssText = 'position:fixed; top:0; left:0; width:100%; height:100%; background:rgba(0,0,0,0.65); z-index:10050; display:flex; align-items:center; justify-content:center; padding:1.5rem; backdrop-filter:blur(4px);';
        modal.innerHTML = `
            <div class="glass-panel" role="dialog" aria-labelledby="local-ha-guide-title" style="width:100%; max-width:920px; max-height:92vh; display:flex; flex-direction:column; padding:0; border-radius:16px; overflow:hidden; box-shadow:var(--shadow-lg);${opts.highlight ? ' border:2px solid rgba(var(--accent-rgb),0.45);' : ''}">
                <div class="flex-between" style="padding:1.25rem 1.5rem; border-bottom:1px solid var(--border-color); background:rgba(255,255,255,0.05); flex-shrink:0;">
                    <h3 id="local-ha-guide-title" style="margin:0; font-size:1.15rem; font-weight:700;">
                        <i class="fa-solid fa-flask text-accent"></i> Local test environment: HA clusters
                    </h3>
                    <button type="button" class="btn btn-xs btn-outline local-ha-guide-close" style="border:none; font-size:1.5rem; padding:0;" aria-label="Close">&times;</button>
                </div>
                <div class="local-ha-guide-body" style="padding:1.5rem; flex:1; overflow-y:auto; background:var(--bg-subtle);">
                    ${guideBodyHtml()}
                </div>
                <div class="flex-between" style="padding:1rem 1.5rem; border-top:1px solid var(--border-color); background:rgba(255,255,255,0.05); flex-shrink:0; flex-wrap:wrap; gap:0.75rem;">
                    <a href="${QUICKSTART_ANCHOR}" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-outline">
                        <i class="fa-solid fa-book"></i> Full documentation
                    </a>
                    <button type="button" class="btn btn-sm btn-accent local-ha-guide-close">Got it</button>
                </div>
            </div>
        `;

        function closeGuide(persistDismiss) {
            if (persistDismiss) {
                try { localStorage.setItem('sql_optima_ha_guide_dismissed', '1'); } catch (e) { /* ignore */ }
            }
            modal.remove();
            document.removeEventListener('keydown', onKey);
        }

        function onKey(e) {
            if (e.key === 'Escape') closeGuide(true);
        }

        modal.querySelectorAll('.local-ha-guide-close').forEach(btn => {
            btn.addEventListener('click', () => closeGuide(true));
        });
        modal.addEventListener('click', (e) => {
            if (e.target === modal) closeGuide(true);
        });
        document.addEventListener('keydown', onKey);
        document.body.appendChild(modal);
    };

    window.isLocalHaGuideDismissed = function() {
        try { return localStorage.getItem('sql_optima_ha_guide_dismissed') === '1'; } catch (e) { return false; }
    };
})();
