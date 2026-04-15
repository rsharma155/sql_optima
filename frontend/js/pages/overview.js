/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Instance overview page with key metrics summary.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgDashboardView = async function() {
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: 'Loading...'};
    const dbName = window.appState.currentDatabase !== 'all' ? window.appState.currentDatabase : 'Cluster / System Check';

    let html = await window.loadTemplate('/pages/overview.html');
    
    // Evaluate literal variables exactly as previously compiled ES6 strings
    html = html.replace('${inst.name}', inst.name).replace('${dbName}', dbName);

    window.routerOutlet.innerHTML = html;

    setTimeout(() => initPgDashboard().catch(e => console.error("PG dashboard init failed:", e)), 50);
}

async function initPgDashboard() {
    window.currentCharts = window.currentCharts || {};
    
    const inst = window.appState.config.instances[window.appState.currentInstanceIdx] || {name: ''};
    const database = window.appState.currentDatabase || 'all';

    let labels = Array.from({length:30}, (_,i)=> `-${30-i}m`);
    labels[labels.length - 1] = "Now";
    let tps = Array.from({length:30}, ()=>0);
    let cacheHitPct = Array.from({length:30}, ()=>0);

    // Fetch overview data for metric cards
    let overviewData = {};
    try {
        const overviewResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/overview?instance=${encodeURIComponent(inst.name)}`
        );
        if (overviewResponse.ok) {
            const contentType = overviewResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                overviewData = await overviewResponse.json();
            }
        } else {
            console.error("Failed to load PG overview:", overviewResponse.status);
        }
    } catch (e) {
        console.error("PG overview fetch failed:", e);
    }

    // Fetch database size + growth
    let dbSize = null;
    try {
        const resp = await window.apiClient.authenticatedFetch(
            `/api/postgres/database-size?instance=${encodeURIComponent(inst.name)}`
        );
        if (resp.ok) {
            const ct = resp.headers.get('content-type') || '';
            if (ct.includes('application/json')) dbSize = await resp.json();
        }
    } catch (e) {
        console.error("PG database-size fetch failed:", e);
    }

    // Fetch server info (version, uptime)
    let serverInfo = {};
    try {
        const serverResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/server-info?instance=${encodeURIComponent(inst.name)}`
        );
        if (serverResponse.ok) {
            const contentType = serverResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                serverInfo = await serverResponse.json();
            }
        } else {
            console.error("Failed to load PG server info:", serverResponse.status);
        }
    } catch (e) {
        console.error("PG server info fetch failed:", e);
    }

    // Fetch system stats (CPU, memory)
    let systemStats = {};
    try {
        const systemResponse = await window.apiClient.authenticatedFetch(
            `/api/postgres/system-stats?instance=${encodeURIComponent(inst.name)}`
        );
        if (systemResponse.ok) {
            const contentType = systemResponse.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                systemStats = await systemResponse.json();
            }
        } else {
            console.error("Failed to load PG system stats:", systemResponse.status);
        }
    } catch (e) {
        console.error("PG system stats fetch failed:", e);
    }

    // Fetch DB Observation Metrics (DBA Health) and Control Center derived metrics (Timescale snapshot)
    let dbObsMetrics = {};
    let ccStats = null;
    let ccHistory = null;
    let replLagSeries = null;
    let blockingTree = [];
    let diskStats = [];
    let backupLatest = null;
    let logSummary = null;
    let poolerLatest = null;
    try {
        const [dbObsResponse, ccResp, ccHistResp, replLagResp, blockingResp, diskResp, backupResp, logsResp, poolerResp] = await Promise.all([
            window.apiClient.authenticatedFetch(`/api/postgres/db-observation?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/control-center?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/control-center/history?instance=${encodeURIComponent(inst.name)}&limit=90`),
            window.apiClient.authenticatedFetch(`/api/postgres/replication-lag/history?instance=${encodeURIComponent(inst.name)}&limit=180`),
            window.apiClient.authenticatedFetch(`/api/postgres/blocking-tree?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/disk?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/backups/latest?instance=${encodeURIComponent(inst.name)}`),
            window.apiClient.authenticatedFetch(`/api/postgres/logs/summary?instance=${encodeURIComponent(inst.name)}&window_minutes=60`),
            window.apiClient.authenticatedFetch(`/api/postgres/pooler/latest?instance=${encodeURIComponent(inst.name)}`),
        ]);
        if (dbObsResponse.ok) {
            const ct = dbObsResponse.headers.get('content-type') || '';
            if (ct.includes('application/json')) dbObsMetrics = await dbObsResponse.json();
        }
        if (ccResp.ok) {
            const ct = ccResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await ccResp.json();
                ccStats = payload && payload.stats ? payload.stats : null;
                if (window.updateSourceBadge) window.updateSourceBadge('pgDataSourceBadge', ccResp.headers.get('X-Data-Source'));
            }
        }
        if (ccHistResp.ok) {
            const ct = ccHistResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await ccHistResp.json();
                ccHistory = payload && payload.history ? payload.history : null;
            }
        }
        if (replLagResp.ok) {
            const ct = replLagResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await replLagResp.json();
                replLagSeries = payload && payload.series ? payload.series : null;
            }
        }
        if (blockingResp.ok) {
            const ct = blockingResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await blockingResp.json();
                blockingTree = payload && payload.blocking_tree ? payload.blocking_tree : [];
            }
        }
        if (diskResp.ok) {
            const ct = diskResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await diskResp.json();
                diskStats = payload && payload.stats ? payload.stats : [];
            }
        }
        if (backupResp.ok) {
            const ct = backupResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await backupResp.json();
                backupLatest = payload && payload.latest ? payload.latest : null;
            }
        }
        if (logsResp.ok) {
            const ct = logsResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await logsResp.json();
                logSummary = payload && payload.summary ? payload.summary : null;
            }
        }
        if (poolerResp.ok) {
            const ct = poolerResp.headers.get('content-type') || '';
            if (ct.includes('application/json')) {
                const payload = await poolerResp.json();
                poolerLatest = payload && payload.latest ? payload.latest : null;
            }
        }
    } catch (e) {
        console.error("PG CC/db-observation fetch failed:", e);
    }

    // Update server info in the header (keep Last Update row intact)
    if (serverInfo.version && serverInfo.uptime) {
        const uptimeEl = document.getElementById('pg-uptime');
        const verEl = document.getElementById('pg-version');
        if (uptimeEl) {
            uptimeEl.innerHTML = `Uptime: <strong>${window.escapeHtml ? window.escapeHtml(serverInfo.uptime) : serverInfo.uptime}</strong>`;
        }
        if (verEl) {
            verEl.innerHTML = `Version: <strong>PostgreSQL ${window.escapeHtml ? window.escapeHtml(String(serverInfo.version)) : serverInfo.version}</strong>`;
        }
    }

    // Update metric cards with real data - use direct selectors for compact layout
    // TPS
    const tpsCard = document.querySelector('.metric-value[data-metric="tps"]');
    if (tpsCard) {
        if (overviewData.last_tps !== undefined && overviewData.last_tps > 0) {
            tpsCard.innerHTML = overviewData.last_tps.toFixed(1);
        } else {
            tpsCard.innerHTML = '0.0';
        }
    }
    
    // Connections
    const connCard = document.querySelector('.metric-value[data-metric="connections"]');
    if (connCard) {
        if (overviewData.total_connections !== undefined && overviewData.active_connections !== undefined) {
            connCard.innerHTML = overviewData.active_connections + '<span style="font-size:0.5em"> / ' + overviewData.total_connections + ' Max</span>';
        } else {
            connCard.innerHTML = '0<span style="font-size:0.5em"> / 0 Max</span>';
        }
    }
    
    // Cache Hit
    const cacheCard = document.querySelector('.metric-value[data-metric="cache-hit"]');
    if (cacheCard) {
        if (overviewData.last_cache_hit_pct !== undefined && overviewData.last_cache_hit_pct >= 0) {
            cacheCard.innerHTML = overviewData.last_cache_hit_pct.toFixed(1) + '<span>%</span>';
        } else {
            cacheCard.innerHTML = '0.0<span>%</span>';
        }
    }
    
    // System Resources (CPU/Memory)
    const systemCard = document.querySelector('.metric-value[data-metric="system-cpu"]');
    if (systemCard) {
        if (systemStats.cpu_usage !== undefined) {
            systemCard.innerHTML = systemStats.cpu_usage.toFixed(1) + '<span>%</span>';
        } else {
            systemCard.innerHTML = 'N/A';
        }
    }
    const memCard = document.querySelector('.metric-value[data-metric="system-mem"]');
    if (memCard) {
        if (systemStats.memory_usage !== undefined) {
            memCard.innerHTML = systemStats.memory_usage.toFixed(1) + '<span>%</span>';
        } else {
            memCard.innerHTML = 'N/A';
        }
    }

    // Disk free (local-only, only when configured and collected)
    try {
        const diskCell = document.getElementById('pgDiskFreeCell');
        const diskVal = document.querySelector('.metric-value[data-metric="disk-free"]');
        const diskSub = document.getElementById('pgDiskFreeSub');
        if (diskCell && diskVal && diskSub && Array.isArray(diskStats) && diskStats.length > 0) {
            const latestByMount = {};
            diskStats.forEach(r => {
                const m = (r.mount_name || '').toString();
                if (!m) return;
                if (!latestByMount[m]) latestByMount[m] = r;
            });
            const pick = latestByMount.wal || latestByMount.data || Object.values(latestByMount)[0];
            const freeBytes = Number(pick?.free_bytes || 0);
            const usedPct = Number(pick?.used_pct || 0);
            const mount = (pick?.mount_name || '').toString() || 'disk';
            const fmtGB = (b) => (b / 1024 / 1024 / 1024).toFixed(1) + ' GB';
            if (freeBytes > 0) {
                diskVal.innerHTML = `${fmtGB(freeBytes)} <span style="font-size:0.75em" class="text-muted">free</span>`;
                // Time-to-full (rough): use WAL rate if available (MB/min).
                const walRateMBm = Number(ccStats?.wal_rate_mb_per_min || 0);
                let ttf = '';
                if (walRateMBm > 0 && mount === 'wal') {
                    const freeMB = freeBytes / 1024 / 1024;
                    const minutes = freeMB / walRateMBm;
                    if (isFinite(minutes) && minutes > 0) {
                        const hours = Math.floor(minutes / 60);
                        const days = Math.floor(hours / 24);
                        const hRem = hours % 24;
                        ttf = days > 0 ? ` • ~${days}d ${hRem}h to full` : ` • ~${hours}h to full`;
                    }
                }
                diskSub.textContent = `${mount} • ${usedPct.toFixed(0)}% used${ttf}`;
                diskCell.style.display = '';
                diskCell.style.borderColor = usedPct >= 90 ? 'var(--danger)' : (usedPct >= 80 ? 'var(--warning)' : 'var(--success)');
            }
        }
    } catch (e) {
        // non-fatal
    }

    // DB size + growth
    try {
        const sizeVal = document.querySelector('.metric-value[data-metric="db-size"]');
        const sizeSub = document.getElementById('pgDbSizeSub');
        const fmtBytes = (b) => {
            const x = Number(b || 0);
            if (!Number.isFinite(x) || x <= 0) return '0 B';
            const units = ['B', 'KB', 'MB', 'GB', 'TB'];
            let u = 0;
            let v = x;
            while (v >= 1024 && u < units.length - 1) { v /= 1024; u++; }
            return v.toFixed(u >= 3 ? 2 : 1) + ' ' + units[u];
        };
        if (sizeVal && dbSize && dbSize.total_bytes != null) {
            sizeVal.innerHTML = fmtBytes(dbSize.total_bytes);
            if (sizeSub) {
                const g = Number(dbSize.growth_bytes_per_hr || 0);
                sizeSub.textContent = (Number.isFinite(g) && Math.abs(g) > 0)
                    ? (g >= 0 ? '+' : '') + fmtBytes(g) + '/hr'
                    : 'growth: --';
            }
        } else {
            if (sizeVal) sizeVal.innerHTML = '--';
            if (sizeSub) sizeSub.textContent = '--';
        }
    } catch (e) {
        // non-fatal
    }

    // Backup status (reported by external backup job)
    try {
        const backupCell = document.getElementById('pgBackupCell');
        const backupVal = document.querySelector('.metric-value[data-metric="backup-status"]');
        const backupSub = document.getElementById('pgBackupSub');
        if (backupCell && backupVal && backupSub && backupLatest) {
            const status = String(backupLatest.status || '').toLowerCase();
            const tool = String(backupLatest.tool || '');
            const btype = String(backupLatest.backup_type || '');
            const capTs = backupLatest.capture_timestamp ? new Date(backupLatest.capture_timestamp) : null;
            const ageMin = capTs ? Math.floor((Date.now() - capTs.getTime()) / 60000) : null;

            backupVal.textContent = status || '--';
            backupSub.textContent = `${tool}${btype ? ' • ' + btype : ''}${ageMin !== null ? ' • ' + ageMin + 'm ago' : ''}`;
            backupCell.style.display = '';
            // Backup age warning: if last success is too old, warn even if status is "success".
            let border = (status === 'failed') ? 'var(--danger)' : (status === 'warning' ? 'var(--warning)' : 'var(--success)');
            if (status === 'success' && ageMin !== null) {
                if (ageMin > (24 * 60)) border = 'var(--danger)';
                else if (ageMin > (12 * 60)) border = 'var(--warning)';
            }
            backupCell.style.borderColor = border;
        }
    } catch (e) {
        // non-fatal
    }

    // Pooler (PgBouncer) health
    try {
        const cell = document.getElementById('pgPoolerCell');
        const val = document.querySelector('.metric-value[data-metric="pooler-wait"]');
        const sub = document.getElementById('pgPoolerSub');
        if (cell && val && sub && poolerLatest) {
            const w = Number(poolerLatest.cl_waiting || 0);
            const ca = Number(poolerLatest.cl_active || 0);
            const su = Number(poolerLatest.sv_used || 0);
            const mw = Number(poolerLatest.maxwait_seconds || 0);
            const pools = Number(poolerLatest.total_pools || 0);
            val.textContent = `${w} waiting`;
            sub.textContent = `cl:${ca} • sv_used:${su} • maxwait:${mw.toFixed(0)}s • pools:${pools}`;
            cell.style.display = '';
            cell.style.borderColor = (w > 0 || mw >= 10) ? 'var(--danger)' : (w > 0 || mw >= 2 ? 'var(--warning)' : 'var(--success)');
        }
    } catch (e) {
        // non-fatal
    }

    // Critical errors (last 60m) from log shipper events
    try {
        const cell = document.getElementById('pgCritErrorsCell');
        const val = document.querySelector('.metric-value[data-metric="crit-errors"]');
        const sub = document.getElementById('pgCritErrorsSub');
        if (cell && val && sub && logSummary) {
            const errCnt = Number(logSummary.error_count || 0);
            const fatalCnt = Number(logSummary.fatal_count || 0);
            const panicCnt = Number(logSummary.panic_count || 0);
            const authCnt = Number(logSummary.auth_fail_count || 0);
            const oomCnt = Number(logSummary.oom_count || 0);
            const total = errCnt + fatalCnt + panicCnt;

            if ((total + authCnt + oomCnt) > 0) {
                val.textContent = String(total);
                sub.textContent = `F:${fatalCnt} P:${panicCnt} E:${errCnt} • auth:${authCnt} oom:${oomCnt}`;
                cell.style.display = '';
                cell.style.borderColor = (panicCnt > 0 || fatalCnt > 0) ? 'var(--danger)' : (errCnt > 0 ? 'var(--warning)' : 'var(--success)');
                if (logSummary.last_message) {
                    cell.title = `Last event: ${logSummary.last_event_at || ''}\n${String(logSummary.last_message).slice(0, 220)}`;
                }
            }
        }
    } catch (e) {
        // non-fatal
    }

    // Update DBA Health Cards
    updateDBAHealthCards(dbObsMetrics);

    // Update Control Center strip cells (if present in template)
    try {
        const setText = (sel, v) => { const el = document.querySelector(sel); if (el) el.textContent = v; };
        if (ccStats) {
            // These selectors correspond to data-metric bindings in the template.
            const walRateEl = document.querySelector('[data-metric="wal-rate"]');
            if (walRateEl) walRateEl.textContent = (Number(ccStats.wal_rate_mb_per_min || 0)).toFixed(1) + ' MB/min';
            const walSizeEl = document.querySelector('[data-metric="wal-size"]');
            if (walSizeEl) walSizeEl.textContent = (Number(ccStats.wal_size_mb || 0)).toFixed(0) + ' MB';
            const replLagEl = document.querySelector('[data-metric="repl-lag"]');
            if (replLagEl) replLagEl.textContent = (Number(ccStats.max_replication_lag_seconds || 0)).toFixed(1) + ' s';
            const cpEl = document.querySelector('[data-metric="checkpoint-pressure"]');
            if (cpEl) cpEl.textContent = (Number(ccStats.checkpoint_req_ratio || 0)).toFixed(2);
            const xidEl = document.querySelector('[data-metric="xid-age"]');
            if (xidEl) xidEl.textContent = (Number(ccStats.xid_age || 0)).toLocaleString();

            const blkEl = document.querySelector('[data-metric="blocking-sessions"]');
            if (blkEl) blkEl.textContent = String(ccStats.blocking_sessions || 0);

            const awEl = document.querySelector('[data-metric="sessions-aw"]');
            if (awEl) awEl.textContent = `${ccStats.active_sessions || 0} / ${ccStats.waiting_sessions || 0}`;

            // Health badge
            const hs = document.getElementById('pgHealthScoreBadge');
            if (hs) {
                const score = Number(ccStats.health_score || 0);
                const status = (ccStats.health_status || '').toString() || (score >= 90 ? 'Healthy' : (score >= 70 ? 'Watch' : 'At Risk'));
                hs.textContent = `${score} ${status}`;
                hs.classList.remove('badge-success', 'badge-warning', 'badge-danger');
                if (score >= 90) hs.classList.add('badge-success');
                else if (score >= 70) hs.classList.add('badge-warning');
                else hs.classList.add('badge-danger');
            }

            // Severity: replication lag + xid + blocking
            const replCell = document.querySelector('[data-metric="repl-lag"]')?.closest('.strip-metric-cell');
            if (replCell) {
                const lagS = Number(ccStats.max_replication_lag_seconds || 0);
                replCell.style.borderColor = lagS >= 60 ? 'var(--danger)' : (lagS >= 10 ? 'var(--warning)' : 'var(--success)');
            }
            const xidCell = document.querySelector('[data-metric="xid-wraparound"]')?.closest('.strip-metric-cell');
            if (xidCell) {
                const pct = Number(ccStats.xid_wraparound_pct || 0);
                xidCell.style.borderColor = pct >= 80 ? 'var(--danger)' : (pct >= 60 ? 'var(--warning)' : 'var(--success)');
            }
            const blkCell = document.querySelector('[data-metric="blocking-sessions"]')?.closest('.strip-metric-cell');
            if (blkCell) {
                const n = Number(ccStats.blocking_sessions || 0);
                blkCell.style.borderColor = n > 0 ? 'var(--danger)' : 'var(--success)';
            }
        }
    } catch (e) {
        // non-fatal
    }

    // Render blocking mini table (top blockers)
    try {
        const tbody = document.getElementById('pgBlockingMiniTbody');
        if (tbody) {
            const flatten = (nodes, out=[]) => {
                (nodes || []).forEach(n => { out.push(n); flatten(n.blocked_by, out); });
                return out;
            };
            const all = flatten(blockingTree, []);
            const blockers = all.filter(n => n.blocked_by && n.blocked_by.length > 0);
            blockers.sort((a,b) => (b.blocked_by?.length||0) - (a.blocked_by?.length||0));
            const top = blockers.slice(0, 5);
            if (top.length === 0) {
                tbody.innerHTML = '<tr><td colspan="3" class="text-muted">No blocking detected</td></tr>';
            } else {
                tbody.innerHTML = top.map(n => `
                    <tr>
                        <td>${n.pid}</td>
                        <td class="text-muted">${(n.wait_event || '').slice(0, 24)}</td>
                        <td class="text-muted">${(n.query || '').replace(/</g,'&lt;').slice(0, 48)}</td>
                    </tr>
                `).join('');
            }
        }
    } catch (e) {}

    // Control Center charts from Timescale history
    try {
        const hist = ccHistory;
        if (hist && hist.labels && hist.labels.length) {
            const labels2 = hist.labels;

            const makeLine = (id, label, data, color) => {
                const el = document.getElementById(id);
                if (!el || !window.Chart) return;
                if (window.currentCharts[id]) window.currentCharts[id].destroy();
                window.currentCharts[id] = new Chart(el, {
                    type: 'line',
                    data: { labels: labels2, datasets: [{ label, data, borderColor: color, backgroundColor: color, tension: 0.25, pointRadius: 0 }] },
                    options: { responsive:true, maintainAspectRatio:false, plugins:{ legend:{ display:false }}, scales:{ x:{ display:true }, y:{ beginAtZero:true } } }
                });
            };

            makeLine('pgWalRateChart', 'WAL MB/min', hist.wal_rate_mb_per_min || [], window.getCSSVar ? window.getCSSVar('--accent-blue') : '#3b82f6');
            makeLine('pgAutovacChart', 'Autovacuum', hist.autovacuum_workers || [], window.getCSSVar ? window.getCSSVar('--warning') : '#f59e0b');
            makeLine('pgDeadTupleChart', 'Dead tuple %', hist.dead_tuple_ratio_pct || [], window.getCSSVar ? window.getCSSVar('--danger') : '#ef4444');
        }

        // Replication lag detail chart (per replica), only show if series exists
        const card = document.getElementById('pgReplLagChartCard');
        const ctx = document.getElementById('pgReplLagDetailChart');
        if (card && ctx && replLagSeries && Object.keys(replLagSeries).length) {
            card.style.display = 'block';
            if (window.currentCharts.pgReplLagDetailChart) window.currentCharts.pgReplLagDetailChart.destroy();
            const seriesArr = Object.values(replLagSeries);
            const labels = seriesArr[0]?.labels || [];
            const palette = ['#3b82f6','#10b981','#f59e0b','#ef4444','#a855f7'];
            const datasets = seriesArr.map((s, idx) => ({
                label: s.replica_name,
                data: s.lag_mb || [],
                borderColor: palette[idx % palette.length],
                backgroundColor: palette[idx % palette.length],
                tension: 0.25,
                pointRadius: 0
            }));
            window.currentCharts.pgReplLagDetailChart = new Chart(ctx, {
                type: 'line',
                data: { labels, datasets },
                options: { responsive:true, maintainAspectRatio:false, plugins:{ legend:{ display:true }}, scales:{ x:{ display:true }, y:{ beginAtZero:true } } }
            });
        } else if (card) {
            card.style.display = 'none';
        }
    } catch (e) {
        console.error('PG CC chart render failed:', e);
    }
    
    // Handle replication data - only show if it's a standby server
    const replCard = document.querySelector('.metric-card[data-metric="replication"]');
    if (replCard) {
        if (overviewData.replication_status && overviewData.replication_status !== 'primary') {
            replCard.style.display = 'block';
            const replValueCard = replCard.querySelector('.metric-value');
            const replTrendCard = replCard.querySelector('.metric-trend');
            
            if (replValueCard && overviewData.replication_lag_mb !== undefined) {
                replValueCard.innerHTML = overviewData.replication_lag_mb.toFixed(1) + '<span style="font-size:0.5em"> MB Lag</span>';
            } else if (replValueCard && replValueCard.innerHTML === 'Loading...') {
                replValueCard.innerHTML = '0.0<span style="font-size:0.5em"> MB Lag</span>';
            }
            if (replTrendCard && overviewData.replication_status) {
                replTrendCard.innerHTML = '<i class="fa-solid fa-exclamation"></i> ' + overviewData.replication_status;
            } else if (replTrendCard && replTrendCard.innerHTML.includes('Loading')) {
                replTrendCard.innerHTML = '<i class="fa-solid fa-exclamation"></i> Standby';
            }
        } else {
            replCard.style.display = 'none';
        }
    }

    // Fetch dashboard time series data for charts
    try {
        const response = await window.apiClient.authenticatedFetch(
            `/api/postgres/dashboard?instance=${encodeURIComponent(inst.name)}&database=${encodeURIComponent(database)}`
        );
        if (response.ok) {
            const contentType = response.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                const payload = await response.json();
                if (payload?.labels?.length) labels = payload.labels;
                if (payload?.tps?.length) tps = payload.tps;
                if (payload?.cache_hit_pct?.length) cacheHitPct = payload.cache_hit_pct;
            }
        } else {
            console.error("Failed to load PG throughput dashboard:", response.status);
        }
    } catch (e) {
        console.error("PG throughput dashboard fetch failed:", e);
    }

    // Update source badge + last update time (best-effort; some endpoints may not send X-Data-Source yet).
    try {
        const tEl = document.getElementById('pgLastRefreshTime');
        if (tEl) tEl.textContent = new Date().toLocaleTimeString();
    } catch (e) {
        // non-fatal
    }

    // Initialize charts only if canvas elements exist
    const tpsCtx = document.getElementById('pgTpsChart');
    if (tpsCtx) {
        const gradTps = tpsCtx.getContext('2d').createLinearGradient(0,0,0,300);
        gradTps.addColorStop(0, 'rgba(16, 185, 129, 0.4)');
        gradTps.addColorStop(1, 'rgba(16, 185, 129, 0.0)');

        window.currentCharts.pgTps = new Chart(tpsCtx.getContext('2d'), {
            type: 'line',
            data: {
                labels,
                datasets: [
                    {
                        label: 'Transactions/sec (TPS)',
                        data: tps,
                        borderColor: window.getCSSVar('--success'),
                        backgroundColor: gradTps,
                        fill: true,
                        tension: 0.25,
                        pointRadius: 0,
                        yAxisID: 'y',
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: { mode: 'index', intersect: false },
                plugins: { legend: { position: 'top' } },
                scales: {
                    y: {
                        beginAtZero: true,
                        title: { display: true, text: 'TPS' },
                    }
                }
            }
        });
    }

    // Cache Hit % chart removed in v2 Control Center (now WAL + autovac + dead tuples are higher value).

    // Connection State Chart
    const pgConnCtx = document.getElementById('pgConnChart');
    if (pgConnCtx) {
        window.currentCharts.pgConn = new Chart(pgConnCtx.getContext('2d'), {
            type: 'doughnut', 
            data: { 
                labels: ['Active', 'Idle', 'Idle in Transaction'], 
                datasets: [{ 
                    data: [
                        overviewData.active_connections || 0, 
                        overviewData.idle_connections || 0, 
                        dbObsMetrics.idle_in_transaction_cnt || 0
                    ], 
                    backgroundColor: [window.getCSSVar('--accent-blue'), window.getCSSVar('--success'), '#ff6b35'], 
                    borderWidth: 0
                }]
            },
            options: { 
                responsive: true, 
                maintainAspectRatio: false, 
                cutout: '70%', 
                plugins: {
                    legend: { position: 'bottom' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                const total = context.dataset.data.reduce((a, b) => a + b, 0);
                                const percentage = total > 0 ? ((context.parsed / total) * 100).toFixed(1) : 0;
                                return context.label + ': ' + context.parsed + ' (' + percentage + '%)';
                            }
                        }
                    }
                }
            }
        });
    }

    // Resource Usage chart removed (CPU/RAM already shown in tiles; replaced by Autovacuum + Dead Tuples charts).
}

function updateDBAHealthCards(metrics) {
    if (!metrics || Object.keys(metrics).length === 0) return;

    // XID Wraparound
    const xidCard = document.querySelector('.metric-value[data-metric="xid-wraparound"]');
    const xidTrend = document.getElementById('xid-trend');
    const xidParent = document.getElementById('metric-xid-wraparound');
    if (xidCard && metrics.xid_wraparound_pct !== undefined) {
        const xidPct = parseFloat(metrics.xid_wraparound_pct) || 0;
        xidCard.innerHTML = xidPct.toFixed(1) + '<span>%</span>';
        if (xidPct > 80) {
            xidCard.style.color = 'var(--danger)';
            if (xidTrend) xidTrend.innerHTML = '<i class="fa-solid fa-exclamation-triangle text-danger"></i> CRITICAL';
            if (xidParent) xidParent.className = 'metric-card glass-panel status-danger';
        } else if (xidPct > 50) {
            xidCard.style.color = 'var(--warning)';
            if (xidTrend) xidTrend.innerHTML = '<i class="fa-solid fa-exclamation text-warning"></i> Warning';
            if (xidParent) xidParent.className = 'metric-card glass-panel status-warning';
        } else {
            xidCard.style.color = 'var(--success)';
            if (xidTrend) xidTrend.innerHTML = '<i class="fa-solid fa-check"></i> Safe';
            if (xidParent) xidParent.className = 'metric-card glass-panel status-healthy';
        }
    }

    // WAL Fails
    const walCard = document.querySelector('.metric-value[data-metric="wal-fails"]');
    const walTrend = document.getElementById('wal-trend');
    const walParent = document.getElementById('metric-wal-fails');
    if (walCard && metrics.wal_fails !== undefined) {
        const walFails = parseInt(metrics.wal_fails) || 0;
        walCard.innerHTML = walFails;
        if (walFails > 0) {
            walCard.style.color = 'var(--danger)';
            if (walTrend) walTrend.innerHTML = '<i class="fa-solid fa-exclamation-triangle text-danger"></i> FAILED';
            if (walParent) walParent.className = 'metric-card glass-panel status-danger';
        } else {
            walCard.style.color = 'var(--success)';
            if (walTrend) walTrend.innerHTML = '<i class="fa-solid fa-check"></i> Healthy';
            if (walParent) walParent.className = 'metric-card glass-panel status-healthy';
        }
    }

    // Max Table Bloat
    const bloatCard = document.querySelector('.metric-value[data-metric="max-bloat"]');
    const bloatTrend = document.getElementById('bloat-trend');
    const bloatParent = document.getElementById('metric-table-bloat');
    if (bloatCard && metrics.max_table_bloat_pct !== undefined) {
        const bloatPct = parseFloat(metrics.max_table_bloat_pct) || 0;
        bloatCard.innerHTML = bloatPct.toFixed(1) + '<span>%</span>';
        if (bloatPct > 20) {
            bloatCard.style.color = 'var(--warning)';
            if (bloatTrend) bloatTrend.innerHTML = '<i class="fa-solid fa-exclamation text-warning"></i> High Bloat';
            if (bloatParent) bloatParent.className = 'metric-card glass-panel status-warning';
        } else {
            bloatCard.style.color = 'var(--success)';
            if (bloatTrend) bloatTrend.innerHTML = '<i class="fa-solid fa-check"></i> Clean';
            if (bloatParent) bloatParent.className = 'metric-card glass-panel status-healthy';
        }
    }
}
