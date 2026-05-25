/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose : SQL Server HA & Replication monitoring dashboard.
 *           Full redesign per replication_HA/high_level_requirement.md.
 *
 *           Layout (top-to-bottom, incident-priority order):
 *             ROW 0  Sticky activation banner
 *             ROW 1  Executive KPI cards (6)
 *             ROW 2  RPO trend | RTO trend | Failover readiness checklist
 *             ROW 3  HA topology diagram | Replica health table
 *             ROW 4  Replication KPI cards (conditional)
 *             ROW 5  Replication latency trend | Backlog growth trend
 *             ROW 6  Replication topology table
 *             ROW 7  Replicated articles table
 *             ROW 8  Database coverage panel
 *             ROW 9  Failover history timeline
 *
 *           Page gate: first call is /api/sqlserver/ha/feature-status.
 *           If ha_enabled=false AND replication_enabled=false the page shows
 *           a "not configured" message and no further queries run.
 *
 * Author  : Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

/* ============================================================
   MODULE-LEVEL CHART REGISTRY
   ============================================================ */
const _haCharts = {};

/* ============================================================
   ENTRY POINT
   ============================================================ */
window.HADashboardView = async function () {
    const instance = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
    if (!instance) {
        window.routerOutlet.innerHTML = `<div class="page-view active"><div class="alert alert-warning">Select a SQL Server instance first.</div></div>`;
        return;
    }

    // Redirect if this is a Postgres instance
    if (instance.type === 'postgres') {
        if (typeof window.PgBackupsView === 'function') {
            return window.PgBackupsView();
        }
    }

    const isAlreadyOnPage = !!document.getElementById('ha-page-root');
    
    // Clear previous registered intervals and charts only if NOT a sub-refresh
    if (!isAlreadyOnPage) {
        window.clearAllIntervals();
        Object.values(_haCharts).forEach(c => c && c.destroy && c.destroy());
        Object.keys(_haCharts).forEach(k => delete _haCharts[k]);
        window.routerOutlet.innerHTML = _buildPageSkeleton(instance.name);
    } else {
        // Just show loading state on existing banner
        _setInner('ha-banner', `<i class="fa-solid fa-circle-notch fa-spin"></i>&nbsp;Refreshing data for <strong>${window.escapeHtml(instance.name)}</strong>&hellip;`);
        _bannerColor('loading');
    }

    // Feature gate — first call, determines page visibility
    let feature;
    try {
        const res = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/feature-status?instance=${encodeURIComponent(instance.name)}`
        );
        feature = await res.json();
    } catch (e) {
        _setInner('ha-banner', 'Unable to reach HA status endpoint.');
        _bannerColor('red');
        return;
    }

    if (!feature || (!feature.ha_enabled && !feature.replication_enabled)) {
        _renderNoHAPage(instance.name);
        return;
    }

    // Apply section visibility and page title based on detected mode before data loads.
    _applyRenderMode(feature, instance.name);
    _initHATabs(feature);
    window.appState.haFeature = feature;

    if (typeof window.initPageTimePicker === 'function') {
        window.initPageTimePicker();
    }

    await _loadDashboard(instance, feature);

    // Only set up intervals if they don't exist (avoid duplicates on manual refresh)
    if (!isAlreadyOnPage) {
        window.registerInterval(() => _refreshKPISection(instance, feature), 30000);
        window.registerInterval(() => _refreshCharts(instance, feature), 60000);
        window.registerInterval(() => _refreshTables(instance, feature), 120000);
    }
};

/* ============================================================
   PAGE SKELETON
   ============================================================ */
function _buildPageSkeleton(instanceName) {
    return `
<div class="page-view active dashboard-sky-theme" id="ha-page-root">

  <!-- ROW 0: sticky status banner -->
  <div id="ha-banner" class="ha-banner ha-banner-loading">
    <div class="ha-banner-main">
      <i class="fa-solid fa-circle-notch fa-spin"></i>&nbsp;Loading status for <strong>${window.escapeHtml(instanceName)}</strong>&hellip;
    </div>
    <div id="ha-feature-badges" class="ha-feature-badges"></div>
  </div>

  <!-- Page header — title and subtitle updated by _applyRenderMode() after feature detection -->
  <div class="page-title flex-between dashboard-page-title-compact" style="margin-top:0.5rem;">
    <div class="dashboard-title-line">
      <h1>
        <i class="fa-solid fa-shield-halved text-accent" id="ha-page-icon"></i>
        <span id="ha-page-title">HA &amp; Replication</span>
      </h1>
      <p class="subtitle" id="ha-page-subtitle">Instance: ${window.escapeHtml(instanceName)}</p>
    </div>
    <div style="display:flex; gap:10px; align-items:center;">
       <div id="time-picker-insertion-point"></div>
       <button class="btn btn-sm btn-outline text-accent" data-action="call" data-fn="HADashboardView">
         <i class="fa-solid fa-rotate"></i> Refresh
       </button>
    </div>
  </div>

  <!-- ROW 1: KPI cards — fill full width -->
  <div class="mt-2" id="ha-kpi-coverage-row">
    <div id="ha-kpi-row" class="ha-kpi-flex">${_kpiSkeleton(6)}</div>
  </div>

  <!-- ROW 1b: Database protection coverage (Gold / Silver / Bronze / Red) -->
  <div class="glass-panel mt-2" id="ha-coverage-panel">
    <div class="ha-card-hdr">
      Database Protection Coverage
      <span class="ha-badge-info" title="Gold = AG + replication; Silver = AG only; Bronze = replication only; Red = neither HA nor replication, or stale full backup">Gold · Silver · Bronze · Red</span>
    </div>
    <div id="ha-coverage-table" class="mt-2"><p class="text-muted small p-2">Loading&hellip;</p></div>
  </div>

  <!-- Technology tabs: Always On | Log Shipping | Replication -->
  <div class="ha-tech-tabs mt-2" id="ha-tech-tabs" role="tablist" aria-label="HA and DR technologies"></div>

  <!-- TAB: Always On AG -->
  <div id="ha-tab-panel-ag" class="ha-tab-panel" data-ha-panel="ag">
  <!-- ROW 2: RPO / RTO charts + failover readiness (HA mode only) -->
  <div class="ha-row-3col mt-2" id="ha-rpo-rto-row">
    <div class="glass-panel ha-chart-card">
      <div class="ha-card-hdr">
        RPO Trend &mdash; Data Loss Risk
        <span class="ha-badge-info">Trend &nbsp;|&nbsp; Y: seconds of data loss</span>
      </div>
      <div class="chart-container" style="height:140px;"><canvas id="rpoChart"></canvas></div>
    </div>
    <div class="glass-panel ha-chart-card">
      <div class="ha-card-hdr">
        RTO Trend &mdash; Est. Failover Time
        <span class="ha-badge-info">Trend &nbsp;|&nbsp; Y: seconds to failover</span>
      </div>
      <div class="chart-container" style="height:140px;"><canvas id="rtoChart"></canvas></div>
    </div>
    <div class="glass-panel ha-chart-card" id="ha-readiness-card">
      <div class="ha-card-hdr">Failover Readiness <span class="ha-badge-live">live</span></div>
      <div id="ha-readiness-body" class="ha-readiness-body">
        ${Array(7).fill('<div class="ha-skeleton-line"></div>').join('')}
      </div>
    </div>
  </div>

  <!-- ROW 4: Topology diagram + Replica health table (HA mode only) -->
  <div class="ha-row-topo mt-2" id="ha-topology-row">
    <div class="glass-panel ha-topo-card" id="ha-topology-card">
      <div class="ha-card-hdr">AG Topology</div>
      <div id="ha-topo-svg" style="min-height:240px;display:flex;align-items:center;justify-content:center;">
        <span class="text-muted small">Loading&hellip;</span>
      </div>
    </div>
    <div class="glass-panel ha-replica-card">
      <div class="ha-card-hdr">Replica Health</div>
      <div id="ha-replica-table" class="mt-2"></div>
    </div>
  </div>
  </div><!-- /ha-tab-panel-ag -->

  <!-- TAB: Log Shipping -->
  <div id="ha-tab-panel-logshipping" class="ha-tab-panel" data-ha-panel="logshipping" style="display:none;">
    <div class="glass-panel mt-2">
      <div class="ha-card-hdr">
        Log Shipping Monitor
        <span class="ha-badge-info" title="From msdb log_shipping_monitor_* tables via collector">Timescale · 5 min</span>
      </div>
      <p class="text-muted small px-2" style="margin:0 0 0.5rem 0;">
        Primary backup and secondary restore/copy lag. Alert when restore delay exceeds threshold.
      </p>
      <div id="ha-logshipping-table"><p class="text-muted small p-2">Loading&hellip;</p></div>
    </div>
  </div>

  <!-- TAB: Replication -->
  <div id="ha-tab-panel-replication" class="ha-tab-panel" data-ha-panel="replication" style="display:none;">
  <div id="ha-repl-section" style="display:none;">
    <!-- ROW 5: Replication KPI cards -->
    <div id="ha-repl-kpi-row" class="ha-kpi-row mt-2">${_kpiSkeleton(6)}</div>

    <!-- ROW 6: Latency + Backlog charts -->
    <div class="grid-2 mt-2" id="ha-repl-charts-row">
      <div class="glass-panel ha-chart-card">
        <div class="ha-card-hdr">
          Replication Latency Trend
          <span class="ha-badge-info">Trend &nbsp;|&nbsp; Y: latency in seconds</span>
        </div>
        <div class="chart-container" style="height:130px;"><canvas id="replLatencyChart"></canvas></div>
      </div>
      <div class="glass-panel ha-chart-card">
        <div class="ha-card-hdr">
          Undelivered Commands Backlog
          <span class="ha-badge-info">Trend &nbsp;|&nbsp; Y: command count</span>
        </div>
        <div class="chart-container" style="height:130px;"><canvas id="replBacklogChart"></canvas></div>
      </div>
    </div>

    <!-- ROW 7: Replication topology table -->
    <div class="glass-panel mt-2" id="ha-repl-topo-panel">
      <div class="ha-card-hdr">Replication Topology</div>
      <div id="ha-repl-topo-table"></div>
    </div>

    <!-- ROW 8: Replicated articles -->
    <div class="glass-panel mt-2" id="ha-articles-panel">
      <div class="ha-card-hdr">Replicated Tables (Articles)</div>
      <div id="ha-articles-table"></div>
    </div>
  </div>
  </div><!-- /ha-repl-section -->
  </div><!-- /ha-tab-panel-replication -->

  <!-- ROW 9: Alerts Timeline + Failover History (side by side) -->
  <div class="ha-row-2col mt-2" id="ha-bottom-row">
    <div class="glass-panel" id="ha-alerts-panel">
      <div class="ha-card-hdr">Alerts Timeline <span class="ha-badge-info">Merged HA &amp; Repl</span></div>
      <div id="ha-alerts-timeline" class="mt-2"></div>
    </div>
    <div class="glass-panel" id="ha-failover-panel">
      <div class="ha-card-hdr">Failover History <span class="ha-badge-info">History</span></div>
      <div id="ha-failover-history" class="mt-2"></div>
    </div>
  </div>

</div>
${_haStyles()}`;
}

/* ============================================================
   NOT CONFIGURED PAGE
   ============================================================ */
function _renderNoHAPage(instanceName) {
    _setInner('ha-banner',
        `<i class="fa-solid fa-circle-info"></i>&nbsp;No HA or Replication configured on <strong>${window.escapeHtml(instanceName)}</strong>. ` +
        `This page activates automatically when AlwaysOn AG, Log Shipping, or Replication is detected (checks run every 15 min).`
    );
    _bannerColor('grey');
    const sectionsToHide = [
        '#ha-kpi-coverage-row', '#ha-rpo-rto-row', '#ha-topology-row',
        '#ha-repl-section', '#ha-bottom-row',
    ];
    sectionsToHide.forEach(sel => {
        const el = document.querySelector(sel);
        if (el) el.style.display = 'none';
    });
}

/* ============================================================
   RENDER MODE — applied immediately after feature detection
   Hides irrelevant sections so layout is correct before data loads.
   ============================================================ */
function _applyRenderMode(feature, instanceName) {
    const haOnly    = feature.ha_enabled && !feature.replication_enabled;
    const replOnly  = !feature.ha_enabled && feature.replication_enabled;
    const both      = feature.ha_enabled && feature.replication_enabled;

    // ── Page title & subtitle ────────────────────────────────
    const titleEl    = document.getElementById('ha-page-title');
    const subtitleEl = document.getElementById('ha-page-subtitle');
    const iconEl     = document.getElementById('ha-page-icon');
    const safeInst   = window.escapeHtml(instanceName);

    const tech = _featureTechSubtitle(feature);
    if (replOnly) {
        if (titleEl)    titleEl.textContent = 'HA & DR — Replication';
        if (iconEl)     iconEl.className = 'fa-solid fa-clone text-accent';
        if (subtitleEl) subtitleEl.textContent = `Instance: ${safeInst}  |  ${tech}`;
    } else {
        if (titleEl)    titleEl.textContent = 'HA & DR';
        if (iconEl)     iconEl.className = 'fa-solid fa-shield-halved text-accent';
        if (subtitleEl) subtitleEl.textContent = `Instance: ${safeInst}  |  ${tech}`;
    }

    // ── Section visibility ───────────────────────────────────
    // #ha-kpi-row is the universal KPI slot: HA KPIs in HA/both modes,
    // Replication KPIs in replication-only mode. Never hide it.
    //
    // Always On / FCI / mirroring panels (not log-shipping-only).
    const showAG = !!(feature.ag_enabled || feature.fci_enabled || feature.mirroring_enabled);
    ['#ha-rpo-rto-row', '#ha-topology-row', '#ha-failover-panel'].forEach(sel => {
        const el = document.querySelector(sel);
        if (el) el.style.display = showAG ? '' : 'none';
    });
    const agPanel = document.getElementById('ha-tab-panel-ag');
    if (agPanel && !showAG) agPanel.style.display = 'none';

    // Replication KPI row inside ha-repl-section is redundant in replication-only mode.
    const replKpiRow = document.getElementById('ha-repl-kpi-row');
    if (replKpiRow) replKpiRow.style.display = replOnly ? 'none' : '';

    // Tab panels control replication / log shipping visibility (_initHATabs).
}

/* ============================================================
   FULL LOAD — parallel fetch of all sections
   ============================================================ */
async function _loadDashboard(instance, feature) {
    _renderFeatureBadges(feature);
    await Promise.all([
        _refreshKPISection(instance, feature),
        _refreshCharts(instance, feature),
        _refreshTables(instance, feature),
        _loadDatabaseCoverage(instance.name),
    ]);
}

/* ============================================================
   SECTION REFRESHERS (called on timer too)
   ============================================================ */
async function _refreshKPISection(instance, feature) {
    if (!feature.ha_enabled) {
        // Replication-only mode: render replication KPIs into the main #ha-kpi-row
        // so they sit on the same row as the Database Protection Coverage panel.
        _renderFeatureBadges(feature);
        _bannerColor('green');
        _setInner('ha-banner',
            '<i class="fa-solid fa-circle-dot"></i>&nbsp;Replication active &mdash; monitoring publisher&nbsp;&rarr;&nbsp;subscriber pipeline');
        _loadReplKPIs(instance.name, 'ha-kpi-row');
        return;
    }

    try {
        const from = window.appState.fromTs ? new Date(window.appState.fromTs).toISOString() : new Date(Date.now() - 86400000).toISOString();
        const to   = window.appState.toTs   ? new Date(window.appState.toTs).toISOString()   : new Date().toISOString();
        const qs   = `instance=${encodeURIComponent(instance.name)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;

        const res = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/dashboard-summary?${qs}`
        );
        const data = await res.json();
        if (data.error) return;

        _renderBanner(data.banner, feature);
        _renderKPICards(data.kpis, feature);
        _renderReadiness(data.failover_readiness);

        if (feature.replication_enabled) {
            _loadReplKPIs(instance.name);
        }
    } catch (e) {
        console.error('[HA] KPI refresh error', e);
    }
}

async function _refreshCharts(instance, feature) {
    const iname = instance.name;
    
    // Use selected time range if available
    let from = window.appState.fromTs ? new Date(window.appState.fromTs).toISOString() : new Date(Date.now() - 86400000).toISOString();
    let to   = window.appState.toTs   ? new Date(window.appState.toTs).toISOString()   : new Date().toISOString();
    
    const qs = `instance=${encodeURIComponent(iname)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;

    if (feature.ha_enabled) {
        _fetchLineChart(`/api/sqlserver/ha/rpo/trend?${qs}`, 'rpoChart', 'RPO (seconds)', 'rpo_seconds', '#4ade80', 's');
        _fetchLineChart(`/api/sqlserver/ha/rto/trend?${qs}`, 'rtoChart', 'Est. RTO (lag+30s trend)', 'estimated_rto_seconds', '#60a5fa', 's');
    }
    if (feature.replication_enabled) {
        _fetchReplCharts(iname, encodeURIComponent(from), encodeURIComponent(to));
    }
}

async function _refreshTables(instance, feature) {
    const iname = instance.name;
    if (feature.ha_enabled) {
        _loadReplicaTable(iname);      // also drives the topology SVG diagram
        _loadTopologyDiagram(iname);
        _loadFailoverHistory(iname);
    }
    _loadAlertsTimeline(iname);
    _loadDatabaseCoverage(iname);
    if (feature.log_shipping_enabled) {
        _loadLogShipping(iname);
    }
    if (feature.replication_enabled) {
        _loadReplTopologyTable(iname);
        _loadArticlesTable(iname);
    }
}

/* ============================================================
   BANNER
   ============================================================ */
function _renderBanner(banner, feature) {
    if (!banner) return;
    const main = document.querySelector('#ha-banner .ha-banner-main');
    const html = `<i class="fa-solid fa-circle-dot"></i>&nbsp;${window.escapeHtml(banner.text || 'HA Active')}`;
    if (main) {
        main.innerHTML = html;
    } else {
        _setInner('ha-banner',
            `<div class="ha-banner-main">${html}</div><div id="ha-feature-badges" class="ha-feature-badges"></div>`);
    }
    _bannerColor(banner.color || 'green');
    if (feature) _renderFeatureBadges(feature);
}

function _bannerColor(color) {
    const el = document.getElementById('ha-banner');
    if (!el) return;
    el.className = 'ha-banner ha-banner-' + (color || 'green');
}

/* ============================================================
   KPI CARDS
   ============================================================ */
function _renderKPICards(kpis, feature) {
    if (!kpis) return;
    const healthy = kpis.healthy_replicas ?? 0;
    const total   = kpis.total_replicas ?? 0;
    const cards = [
        {
            icon: 'fa-crown',
            label: 'Primary Replica',
            value: kpis.primary_server || '—',
            sub: `Since last failover: ${kpis.uptime_human || '—'}`,
            valueClass: 'text-accent',
        },
        {
            icon: 'fa-heart-pulse',
            label: 'Healthy Replicas',
            value: `${healthy} / ${total}`,
            sub: healthy === total ? 'All healthy' : 'Some unhealthy',
            valueClass: healthy === total ? 'text-success' : 'text-danger',
        },
        {
            icon: 'fa-link',
            label: 'Sync Mode',
            value: kpis.sync_mode || '—',
            sub: '',
            valueClass: 'text-muted',
        },
        {
            icon: 'fa-database',
            label: 'Current RPO',
            value: kpis.rpo_seconds != null ? `${kpis.rpo_seconds}s` : '—',
            sub: kpis.rpo_worst_24h > 0 ? `Max lag · Worst 24h: ${kpis.rpo_worst_24h}s` : 'Max secondary lag',
            valueClass: _thresholdClass(kpis.rpo_threshold),
        },
        {
            icon: 'fa-stopwatch',
            label: 'Est. RTO',
            value: kpis.estimated_rto_seconds != null ? `~${kpis.estimated_rto_seconds}s` : '—',
            sub: 'Failover time estimate',
            valueClass: _thresholdClass(kpis.rto_threshold),
        },
        {
            icon: 'fa-shield',
            label: 'Failover Ready',
            value: kpis.failover_ready ? 'YES' : 'NO',
            sub: kpis.last_failover_at ? `Last: ${_relTime(kpis.last_failover_at)}` : 'Safe to failover',
            valueClass: kpis.failover_ready ? 'text-success' : 'text-danger',
        },
    ];
    _setInner('ha-kpi-row', cards.map(_kpiCardHtml).join(''));
}

function _kpiCardHtml(c) {
    return `<div class="glass-panel ha-kpi-card">
  <div class="ha-kpi-icon ${c.valueClass || ''}"><i class="fa-solid ${c.icon}"></i></div>
  <div class="ha-kpi-label">${c.label}</div>
  <div class="ha-kpi-value ${c.valueClass || ''}">${window.escapeHtml(String(c.value))}</div>
  ${c.sub ? `<div class="ha-kpi-sub">${window.escapeHtml(c.sub)}</div>` : ''}
</div>`;
}

function _kpiSkeleton(n) {
    return Array.from({ length: n }, () =>
        `<div class="glass-panel ha-kpi-card ha-skeleton-card">
           <div class="ha-skeleton-line"></div>
           <div class="ha-skeleton-line short"></div>
         </div>`
    ).join('');
}

/* ============================================================
   FAILOVER READINESS CHECKLIST
   ============================================================ */
function _renderReadiness(readiness) {
    if (!readiness) return;
    const iconMap = { pass: '✓', fail: '✗', unknown: '?' };
    const clsMap  = { pass: 'ha-check-pass', fail: 'ha-check-fail', unknown: 'ha-check-unknown' };
    const checksHtml = (readiness.checks || []).map(c => `
<div class="ha-check-row">
  <span class="ha-check-icon ${clsMap[c.status] || ''}">${iconMap[c.status] || '?'}</span>
  <span class="ha-check-name">${window.escapeHtml(c.name)}</span>
  ${c.detail ? `<span class="ha-check-detail">${window.escapeHtml(c.detail)}</span>` : ''}
</div>`).join('');

    const readyClass = readiness.ready ? 'ha-ready-yes' : 'ha-ready-no';
    const readyText  = readiness.ready ? '✓  FAILOVER READY: YES' : '✗  FAILOVER READY: NO';

    _setInner('ha-readiness-body', `
<div class="ha-checks-list">${checksHtml}</div>
<div class="ha-ready-verdict ${readyClass}">${readyText}</div>`);

    const card = document.getElementById('ha-readiness-card');
    if (card) {
        card.classList.toggle('ha-card-danger-pulse', !readiness.ready);
    }
}

/* ============================================================
   REPLICA HEALTH TABLE
   ============================================================ */
async function _loadReplicaTable(instanceName) {
    try {
        const res  = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/replicas?instance=${encodeURIComponent(instanceName)}`
        );
        const data = await res.json();
        _renderReplicaTable(data.replicas || []);
        _renderTopologyFromReplicas(data.replicas || []);
    } catch (e) {
        _setInner('ha-replica-table', '<p class="text-muted small">Replica data unavailable.</p>');
    }
}

function _renderReplicaTable(replicas) {
    if (!replicas.length) {
        _setInner('ha-replica-table', '<p class="text-muted small p-2">No replica data &mdash; collector may not have run yet.</p>');
        return;
    }
    const rows = replicas.map(r => {
        const lagCls  = (r.secondary_lag_seconds > 60) ? 'ha-row-danger' :
                        (r.secondary_lag_seconds > 30) ? 'ha-row-warn' : '';
        const connCls = r.connected_state_desc !== 'CONNECTED' ? 'ha-row-danger' : '';
        const rowCls  = connCls || lagCls;
        return `<tr class="${rowCls}">
  <td><strong>${window.escapeHtml(r.replica_server_name)}</strong></td>
  <td><span class="badge ${r.role_desc === 'PRIMARY' ? 'badge-accent' : 'badge-outline'}">${window.escapeHtml(r.role_desc)}</span></td>
  <td>${window.escapeHtml(_shortSyncMode(r.availability_mode_desc))}</td>
  <td>${_syncHealthBadge(r.synchronization_health_desc)}</td>
  <td class="text-right">${_fmtKB(r.log_send_queue_kb)}</td>
  <td class="text-right">${_fmtKB(r.redo_queue_kb)}</td>
  <td class="text-right ${r.secondary_lag_seconds > 30 ? 'text-warning' : ''}">${r.role_desc === 'PRIMARY' ? '—' : r.secondary_lag_seconds + 's'}</td>
  <td><span class="badge ${r.connected_state_desc === 'CONNECTED' ? 'badge-success' : 'badge-danger'}">${window.escapeHtml(r.connected_state_desc)}</span></td>
</tr>`;
    }).join('');

    _setInner('ha-replica-table', `
<div class="ha-table-wrap">
  <table class="ha-table" id="replica-tbl">
    <thead><tr>
      <th>Replica</th><th>Role</th><th>Sync Mode</th><th>Health</th>
      <th class="text-right">Send Queue</th>
      <th class="text-right">Redo Queue</th>
      <th class="text-right">Commit Lag</th>
      <th>Connected</th>
    </tr></thead>
    <tbody>${rows}</tbody>
  </table>
</div>`);
    _attachTableSort('ha-replica-table');
}

/* ============================================================
   HA TOPOLOGY SVG DIAGRAM
   ============================================================ */
async function _loadTopologyDiagram(instanceName) {
    // topology is rendered from replica data; _loadReplicaTable also calls this
}

function _renderTopologyFromReplicas(replicas) {
    if (!replicas.length) {
        _setInner('ha-topo-svg', '<span class="text-muted small p-2">No topology data yet.</span>');
        return;
    }

    // Group by AG name
    const ags = {};
    replicas.forEach(r => {
        const ag = r.ag_name || 'AlwaysOn AG';
        if (!ags[ag]) ags[ag] = [];
        ags[ag].push(r);
    });

    const agName      = Object.keys(ags)[0];
    const members     = ags[agName] || [];
    const primary     = members.find(m => m.role_desc === 'PRIMARY');
    const secondaries = members.filter(m => m.role_desc !== 'PRIMARY');

    // Layout constants — wider canvas, larger nodes
    const W   = 480;
    const PR  = 52;   // primary radius
    const SR  = 44;   // secondary radius
    const pH  = 120;  // primary Y center
    const px  = 80;   // primary X center
    const sx  = W - 84; // secondary X center
    const gap = 110;
    const dynH = Math.max(250, secondaries.length * gap + 80);

    const esc = s => (s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');

    const parts = [];

    secondaries.forEach((s, i) => {
        const sy         = 50 + i * gap;
        const isAsync    = s.availability_mode_desc && s.availability_mode_desc.includes('ASYNC');
        const healthy    = s.synchronization_health_desc === 'HEALTHY';
        const connected  = s.connected_state_desc === 'CONNECTED';
        const lineColor  = !connected ? '#f87171' : isAsync ? '#fbbf24' : '#4ade80';
        const dash       = isAsync ? '8,4' : 'none';
        const lagLabel   = s.secondary_lag_seconds > 0 ? `${s.secondary_lag_seconds}s lag` : 'in sync';
        const nodeStroke = !healthy ? '#f87171' : !connected ? '#fbbf24' : '#4ade80';
        const midX       = (px + sx) / 2;
        const midY       = (pH + sy) / 2;

        // Build a rich hover tooltip using SVG <title>
        const tip = [
            `Role: SECONDARY`,
            `Server: ${s.replica_server_name}`,
            `AG: ${agName}`,
            `Sync mode: ${s.availability_mode_desc || '—'}`,
            `Health: ${s.synchronization_health_desc || '—'}`,
            `Connection: ${s.connected_state_desc || '—'}`,
            `Commit lag: ${s.secondary_lag_seconds != null ? s.secondary_lag_seconds + 's' : '—'}`,
            `Send queue: ${_fmtKB(s.log_send_queue_kb)}`,
            `Redo queue: ${_fmtKB(s.redo_queue_kb)}`,
        ].join('\n');

        parts.push(`
<line x1="${px + PR}" y1="${pH}" x2="${sx - SR}" y2="${sy}"
      stroke="${lineColor}" stroke-width="2" stroke-dasharray="${dash}" opacity="0.85"/>
<rect x="${midX - 26}" y="${midY - 11}" width="52" height="20" rx="5"
      fill="#0f172a" opacity="0.80"/>
<text x="${midX}" y="${midY + 2}" text-anchor="middle" font-size="11" fill="${lineColor}"
      font-weight="700">${esc(lagLabel)}</text>
<g class="ha-topo-node" style="cursor:default;">
  <title>${esc(tip)}</title>
  <circle cx="${sx}" cy="${sy}" r="${SR}" fill="#1a2540"
          stroke="${nodeStroke}" stroke-width="3"/>
  <circle cx="${sx}" cy="${sy}" r="${SR}" fill="transparent"
          stroke="${nodeStroke}" stroke-width="${SR * 2}" opacity="0" class="ha-topo-hit"/>
  <text x="${sx}" y="${sy - 20}" text-anchor="middle" font-size="18" fill="${nodeStroke}">⬡</text>
  <text x="${sx}" y="${sy - 2}" text-anchor="middle" font-size="11" fill="#e2e8f0" font-weight="700">
    ${esc(_truncate(s.replica_server_name, 12))}
  </text>
  <text x="${sx}" y="${sy + 13}" text-anchor="middle" font-size="9.5" fill="#94a3b8">Secondary</text>
  <text x="${sx}" y="${sy + 27}" text-anchor="middle" font-size="10" font-weight="700"
        fill="${isAsync ? '#fbbf24' : '#4ade80'}">${isAsync ? 'ASYNC' : 'SYNC'}</text>
</g>`);
    });

    // Primary node
    const pName  = primary ? primary.replica_server_name : 'Primary';
    const uptime = primary && primary.uptime_human ? primary.uptime_human : '—';
    const pTip   = [
        `Role: PRIMARY`,
        `Server: ${pName}`,
        `AG: ${agName}`,
        `Uptime: ${uptime}`,
        `Sync mode: ${primary ? primary.availability_mode_desc || '—' : '—'}`,
        `Health: ${primary ? primary.synchronization_health_desc || '—' : '—'}`,
    ].join('\n');

    parts.push(`
<g class="ha-topo-node" style="cursor:default;">
  <title>${esc(pTip)}</title>
  <!-- Outer glow ring -->
  <circle cx="${px}" cy="${pH}" r="${PR + 8}" fill="none" stroke="#818cf8" stroke-width="1.5" opacity="0.35"/>
  <circle cx="${px}" cy="${pH}" r="${PR}" fill="#1a1040" stroke="#818cf8" stroke-width="3"/>
  <!-- Crown / star icon -->
  <text x="${px}" y="${pH - 20}" text-anchor="middle" font-size="22" fill="#fbbf24">★</text>
  <text x="${px}" y="${pH - 1}" text-anchor="middle" font-size="11" fill="#e2e8f0" font-weight="700">
    ${esc(_truncate(pName, 12))}
  </text>
  <text x="${px}" y="${pH + 14}" text-anchor="middle" font-size="9.5" fill="#818cf8" font-weight="700">
    PRIMARY
  </text>
  <text x="${px}" y="${pH + 28}" text-anchor="middle" font-size="8.5" fill="#64748b">
    ${esc(_truncate(agName, 14))}
  </text>
</g>`);

    _setInner('ha-topo-svg', `
<svg viewBox="0 0 ${W} ${dynH}" xmlns="http://www.w3.org/2000/svg"
     style="width:100%;height:auto;" role="img" aria-label="AG Topology: ${esc(agName)}">
  <defs>
    <filter id="topo-glow">
      <feGaussianBlur stdDeviation="2" result="blur"/>
      <feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>
    </filter>
  </defs>
  ${parts.join('')}
</svg>`);
}

/* ============================================================
   TREND CHARTS (line)
   ============================================================ */
function _setChartMessage(canvasId, message) {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;
    const parent = canvas.parentElement;
    if (!parent) return;

    let overlay = parent.querySelector('.chart-no-data-overlay');
    if (!overlay) {
        overlay = document.createElement('div');
        overlay.className = 'chart-no-data-overlay';
        overlay.style.cssText = 'position:absolute; top:0; left:0; width:100%; height:100%; display:flex; flex-direction:column; align-items:center; justify-content:center; background:rgba(20,20,35,0.7); z-index:10; color:#94a3b8; font-size:0.8rem; pointer-events:none; border-radius:8px; text-align:center; padding:15px;';
        parent.style.position = 'relative';
        parent.appendChild(overlay);
    }

    if (message) {
        overlay.style.display = 'flex';
        overlay.innerHTML = `<i class="fa-solid fa-info-circle" style="font-size:1.2rem;margin-bottom:8px;"></i><span>${window.escapeHtml(message)}</span>`;
        canvas.style.opacity = '0.1';
    } else {
        overlay.style.display = 'none';
        canvas.style.opacity = '1';
    }
}

/* ============================================================
   TECHNOLOGY TABS (Always On | Log Shipping | Replication)
   ============================================================ */
function _initHATabs(feature) {
    const tabsEl = document.getElementById('ha-tech-tabs');
    if (!tabsEl) return;

    const defs = [
        { id: 'ag', label: 'Always On', show: !!(feature.ag_enabled || feature.fci_enabled || feature.mirroring_enabled) },
        { id: 'logshipping', label: 'Log Shipping', show: !!feature.log_shipping_enabled },
        { id: 'replication', label: 'Replication', show: !!feature.replication_enabled },
    ].filter(t => t.show);

    if (defs.length === 0) {
        tabsEl.style.display = 'none';
        return;
    }

    if (defs.length === 1) {
        tabsEl.style.display = 'none';
        _switchHATab(defs[0].id, feature);
        return;
    }

    tabsEl.style.display = 'flex';
    tabsEl.innerHTML = defs.map((t, i) =>
        `<button type="button" class="ha-tab${i === 0 ? ' active' : ''}" data-ha-tab="${t.id}" role="tab" aria-selected="${i === 0}">${t.label}</button>`
    ).join('');

    tabsEl.querySelectorAll('.ha-tab').forEach(btn => {
        btn.addEventListener('click', () => _switchHATab(btn.dataset.haTab, feature));
    });
    _switchHATab(defs[0].id, feature);
}

function _switchHATab(tabId, feature) {
    const feat = feature || window.appState.haFeature;
    document.querySelectorAll('.ha-tab-panel').forEach(p => {
        p.style.display = 'none';
    });
    const panel = document.getElementById(`ha-tab-panel-${tabId}`);
    if (panel) panel.style.display = '';

    document.querySelectorAll('#ha-tech-tabs .ha-tab').forEach(btn => {
        const on = btn.dataset.haTab === tabId;
        btn.classList.toggle('active', on);
        btn.setAttribute('aria-selected', on ? 'true' : 'false');
    });

    const replSection = document.getElementById('ha-repl-section');
    if (replSection) {
        replSection.style.display = tabId === 'replication' && feat?.replication_enabled ? '' : 'none';
    }

    if (tabId === 'logshipping' && feat?.log_shipping_enabled) {
        const inst = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
        if (inst) _loadLogShipping(inst.name);
    }
}

async function _loadLogShipping(instanceName) {
    const host = document.getElementById('ha-logshipping-table');
    if (!host) return;
    try {
        const res = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/log-shipping?instance=${encodeURIComponent(instanceName)}`
        );
        if (!res.ok) {
            _setInner('ha-logshipping-table', '<p class="text-muted small p-2">Log shipping data unavailable.</p>');
            return;
        }
        const data = await res.json();
        _renderLogShippingTable(data.log_shipping || []);
    } catch (e) {
        console.warn('[HA] Log shipping load failed', e);
        _setInner('ha-logshipping-table', '<p class="text-muted small p-2">Failed to load log shipping health.</p>');
    }
}

function _renderLogShippingTable(rows) {
    if (!rows.length) {
        _setInner('ha-logshipping-table',
            '<p class="text-muted small p-2">No log shipping rows in the last hour &mdash; confirm log shipping is configured and collector <code>sqlserver_log_shipping</code> is active.</p>');
        return;
    }
    const fmtTime = (t) => {
        if (!t) return '—';
        const d = new Date(t);
        return isNaN(d.getTime()) ? '—' : d.toLocaleString();
    };
    const tr = rows.map(r => {
        const delay = +(r.restore_delay_minutes ?? 0);
        const thresh = +(r.restore_threshold_minutes ?? 0);
        const behind = thresh > 0 && delay > thresh;
        const delayCls = behind ? 'text-danger' : (delay > 0 ? 'text-warning' : 'text-success');
        const role = r.is_primary ? 'Primary' : 'Secondary';
        const pair = r.is_primary
            ? `${window.escapeHtml(r.primary_database || '')}`
            : `${window.escapeHtml(r.primary_database || '')} → ${window.escapeHtml(r.secondary_database || '')}`;
        const remote = r.is_primary
            ? (r.secondary_server || '—')
            : (r.primary_server || '—');
        return `<tr class="${behind ? 'ha-row-danger' : ''}">
  <td>${role}</td>
  <td>${pair}</td>
  <td class="text-muted small">${window.escapeHtml(remote)}</td>
  <td>${fmtTime(r.last_backup_date)}</td>
  <td>${fmtTime(r.last_copied_date)}</td>
  <td>${fmtTime(r.last_restore_date)}</td>
  <td class="${delayCls}">${delay}m / ${thresh || '—'}m</td>
  <td>${behind ? '<span class="text-danger">Behind</span>' : '<span class="text-success">OK</span>'}</td>
</tr>`;
    }).join('');
    _setInner('ha-logshipping-table', `<table class="ha-table ha-logshipping-table">
<thead><tr>
  <th>Role</th><th>Database</th><th>Remote server</th>
  <th>Last backup</th><th>Last copy</th><th>Last restore</th>
  <th>Delay / threshold</th><th>Status</th>
</tr></thead><tbody>${tr}</tbody></table>`);
}

function _featureTechSubtitle(feature) {
    const parts = [];
    if (feature.ag_enabled) parts.push('Always On');
    if (feature.log_shipping_enabled) parts.push('Log Shipping');
    if (feature.fci_enabled) parts.push('FCI');
    if (feature.mirroring_enabled) parts.push('Mirroring');
    if (feature.replication_enabled && feature.replication_types?.length) {
        parts.push(feature.replication_types.join(' · '));
    } else if (feature.replication_enabled) {
        parts.push('Replication');
    }
    return parts.length ? parts.join(' · ') : 'HA / DR features';
}

function _renderFeatureBadges(feature) {
    if (!feature) return;
    const badges = [];
    if (feature.ag_enabled) badges.push('<span class="ha-tech-badge">AG</span>');
    if (feature.log_shipping_enabled) badges.push('<span class="ha-tech-badge">Log Shipping</span>');
    if (feature.fci_enabled) badges.push('<span class="ha-tech-badge">FCI</span>');
    if (feature.mirroring_enabled) badges.push('<span class="ha-tech-badge">Mirroring</span>');
    if (feature.replication_enabled) badges.push('<span class="ha-tech-badge">Replication</span>');
    const el = document.getElementById('ha-feature-badges');
    if (el) el.innerHTML = badges.join('');
}

function _pointTime(p) {
    const t = p?.timestamp ?? p?.bucket;
    if (!t) return '';
    return typeof t === 'string' ? t : new Date(t).toISOString();
}

/** Max latency/backlog per minute across all publications (fixes multi-series collision). */
function _aggregateReplTrendByBucket(pts) {
    const byBucket = new Map();
    for (const p of pts) {
        const key = _pointTime(p);
        if (!key) continue;
        let row = byBucket.get(key);
        if (!row) {
            row = { timestamp: p.timestamp || p.bucket, max_latency_seconds: 0, peak_backlog: 0 };
            byBucket.set(key, row);
        }
        row.max_latency_seconds = Math.max(row.max_latency_seconds, +(p.max_latency_seconds ?? 0));
        row.peak_backlog = Math.max(row.peak_backlog, +(p.peak_backlog ?? 0));
    }
    return Array.from(byBucket.values()).sort((a, b) => _pointTime(a).localeCompare(_pointTime(b)));
}

async function _loadDatabaseCoverage(instanceName) {
    const panel = document.getElementById('ha-coverage-panel');
    if (!panel) return;
    try {
        const res = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/database-coverage?instance=${encodeURIComponent(instanceName)}`
        );
        const data = await res.json();
        _renderDatabaseCoverage(data.coverage || []);
    } catch (e) {
        _setInner('ha-coverage-table', '<p class="text-muted small p-2">Coverage data unavailable.</p>');
    }
}

function _renderDatabaseCoverage(rows) {
    if (!rows.length) {
        _setInner('ha-coverage-table',
            '<p class="text-muted small p-2">No database coverage data &mdash; AG/replication collectors may not have run yet.</p>');
        return;
    }
    const tierClass = { Gold: 'ha-tier-gold', Silver: 'ha-tier-silver', Bronze: 'ha-tier-bronze', Red: 'ha-tier-red' };
    const tr = rows.map(r => {
        const tier = r.protection_level || 'Red';
        const backupOk = r.backup_fresh_ok === true;
        const backupCell = backupOk
            ? '<span class="text-success">OK (&lt;24h)</span>'
            : '<span class="text-danger" title="No full backup within 24 hours on this database">Stale</span>';
        const staleWarn = !backupOk && tier !== 'Red'
            ? ' <span class="ha-badge-warn" title="In AG/replication but full backup older than 24h">backup stale</span>'
            : '';
        return `<tr>
  <td>${window.escapeHtml(r.database_name || '')}</td>
  <td>${r.in_ha ? '✓' : '—'}</td>
  <td>${r.in_replication ? '✓' : '—'}</td>
  <td>${backupCell}</td>
  <td><span class="ha-tier-pill ${tierClass[tier] || 'ha-tier-red'}">${window.escapeHtml(tier)}</span>${staleWarn}</td>
</tr>`;
    }).join('');
    _setInner('ha-coverage-table', `<table class="ha-table ha-coverage-table">
<thead><tr><th>Database</th><th>AG</th><th>Replication</th><th>Full backup</th><th>Tier</th></tr></thead>
<tbody>${tr}</tbody></table>`);
}

async function _fetchLineChart(url, canvasId, label, dataKey, color, unit = 's') {
    try {
        const res  = await window.apiClient.authenticatedFetch(url);
        const resp = await res.json();
        const pts  = resp.data || [];
        if (pts.length === 0) {
            _setChartMessage(canvasId, "No trend data available for the selected range.");
            if (_haCharts[canvasId]) { _haCharts[canvasId].destroy(); delete _haCharts[canvasId]; }
            return;
        }
        _setChartMessage(canvasId, null);
        const labels = pts.map(p => _shortTime(p.timestamp || p.bucket));
        const values = pts.map(p => +(p[dataKey] ?? 0));
        _renderLineChart(canvasId, label, labels, values, color, unit);
    } catch (e) {
        console.warn(`[HA] Chart ${canvasId} load failed`, e);
        _setChartMessage(canvasId, "Error loading chart data.");
    }
}

async function _fetchReplCharts(instanceName, from, to) {
    const qs = `instance=${encodeURIComponent(instanceName)}&from=${from}&to=${to}`;
    try {
        const res  = await window.apiClient.authenticatedFetch(`/api/sqlserver/ha/replication/backlog/trend?${qs}`);
        const resp = await res.json();
        const pts  = resp.data || [];

        const chartsRow = document.getElementById('ha-repl-charts-row');
        if (pts.length === 0) {
            if (chartsRow) chartsRow.style.display = 'none';
            return;
        }
        if (chartsRow) chartsRow.style.display = '';
        _setChartMessage('replLatencyChart', null);
        _setChartMessage('replBacklogChart', null);

        const agg = _aggregateReplTrendByBucket(pts);
        _renderLineChart('replLatencyChart', 'Latency (s) — instance max',
            agg.map(p => _shortTime(p.timestamp || p.bucket)), agg.map(p => p.max_latency_seconds), '#a78bfa', 's');
        _renderLineChart('replBacklogChart', 'Undelivered Commands — instance max',
            agg.map(p => _shortTime(p.timestamp || p.bucket)), agg.map(p => p.peak_backlog), '#f97316', 'cmds');
    } catch (e) {
        console.warn('[HA] Replication charts load failed', e);
        _setChartMessage('replLatencyChart', "Error loading replication trend");
        _setChartMessage('replBacklogChart', "Error loading backlog trend");
    }
}

function _renderLineChart(canvasId, label, labels, data, color, unit = 's') {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;
    if (_haCharts[canvasId]) { _haCharts[canvasId].destroy(); }

    const plugins = [];
    if (canvasId === 'rtoChart') {
        plugins.push({
            id: 'slaLine',
            afterDraw: (chart) => {
                const { ctx, chartArea: { left, right }, scales: { y } } = chart;
                const yPos = y.getPixelForValue(120);
                if (yPos < chart.chartArea.top || yPos > chart.chartArea.bottom) return;
                ctx.save();
                ctx.strokeStyle = '#f8717188';
                ctx.setLineDash([5, 5]);
                ctx.lineWidth = 1;
                ctx.beginPath();
                ctx.moveTo(left, yPos);
                ctx.lineTo(right, yPos);
                ctx.stroke();
                ctx.fillStyle = '#f87171';
                ctx.font = '10px sans-serif';
                ctx.fillText('SLA (120s)', left + 5, yPos - 5);
                ctx.restore();
            }
        });
    }

    _haCharts[canvasId] = new Chart(canvas.getContext('2d'), {
        type: 'line',
        plugins: plugins,
        data: {
            labels,
            datasets: [{
                label,
                data,
                borderColor: color,
                backgroundColor: color + '22',
                borderWidth: 1.5,
                pointRadius: 0,
                fill: true,
                tension: 0.4,
            }],
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            plugins: {
                legend: { display: false },
                tooltip: {
                    callbacks: {
                        label(ctx) {
                            const vals = ctx.dataset.data.filter(v => v != null && !isNaN(v));
                            const suffix = unit === 'cmds' ? '' : (unit || 's');
                            const fmt = (v) => unit === 'cmds' ? String(Math.round(v)) : `${v}${suffix}`;
                            if (!vals.length) return `${ctx.dataset.label}: ${fmt(ctx.parsed.y)}`;
                            const min = Math.min(...vals);
                            const avg = vals.reduce((a, b) => a + b, 0) / vals.length;
                            const max = Math.max(...vals);
                            const u = unit === 'cmds' ? ' cmds' : 's';
                            return [
                                `${ctx.dataset.label}: ${fmt(ctx.parsed.y)}`,
                                `Min: ${fmt(min)}  Avg: ${fmt(avg)}  Max: ${fmt(max)}`,
                            ];
                        },
                    },
                },
            },
            scales: {
                x: { display: false },
                y: {
                    beginAtZero: true,
                    ticks: { color: '#94a3b8', font: { size: 10 } },
                    grid: { color: 'rgba(255,255,255,0.05)' },
                    title: {
                        display: true,
                        text: canvasId === 'rpoChart' ? 'Data Loss (s)'
                            : canvasId === 'rtoChart' ? 'Failover Time (s)'
                            : canvasId === 'replLatencyChart' ? 'Latency (s)'
                            : canvasId === 'replBacklogChart' ? 'Commands'
                            : 'Value',
                        color: '#64748b',
                        font: { size: 9 },
                    },
                },
            },
        },
    });
}

/* ============================================================
   REPLICATION KPIs
   ============================================================ */
// targetId: where to render — 'ha-kpi-row' in replication-only mode,
//           'ha-repl-kpi-row' in HA + Replication mode.
async function _loadReplKPIs(instanceName, targetId = 'ha-repl-kpi-row') {
    try {
        const res = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/replication/kpis?instance=${encodeURIComponent(instanceName)}`
        );
        const k = await res.json();
        const issueColor = (k.issue_count || 0) > 0 ? 'text-danger' : 'text-success';
        const cards = [
            { icon: 'fa-upload',               label: 'Publishers',    value: k.publishers     || 0, valueClass: 'text-accent' },
            { icon: 'fa-download',             label: 'Subscribers',   value: k.subscribers    || 0, valueClass: 'text-accent' },
            { icon: 'fa-newspaper',            label: 'Publications',  value: k.publications   || 0, valueClass: 'text-muted'  },
            { icon: 'fa-database',             label: 'Replicated DBs',value: k.replicated_dbs || 0, valueClass: 'text-muted'  },
            { icon: 'fa-table',                label: 'Articles',      value: k.articles       || 0, valueClass: 'text-muted'  },
            { icon: 'fa-triangle-exclamation', label: 'Issues',        value: k.issue_count    || 0, valueClass: issueColor    },
        ];
        const breakdown = k.type_breakdown || {};
        const typeHtml = Object.entries(breakdown).length
            ? `<div style="grid-column:1/-1;font-size:0.71rem;color:var(--color-muted);margin-top:4px;display:flex;gap:8px;flex-wrap:wrap;">`
              + Object.entries(breakdown).map(([t, n]) =>
                  `<span class="ha-badge-type">${window.escapeHtml(t)}: ${n}</span>`).join('')
              + `</div>`
            : '';
        _setInner(targetId, cards.map(_kpiCardHtml).join('') + typeHtml);
    } catch (e) {
        console.warn('[HA] Replication KPIs error', e);
    }
}

/* ============================================================
   REPLICATION TOPOLOGY TABLE
   ============================================================ */
async function _loadReplTopologyTable(instanceName) {
    try {
        const res  = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/replication/topology?instance=${encodeURIComponent(instanceName)}`
        );
        const data = await res.json();
        _renderReplTopologyTable(data.topology || []);
    } catch (e) {
        _setInner('ha-repl-topo-table', '<p class="text-muted small">Topology unavailable.</p>');
    }
}

function _renderReplTopologyTable(rows) {
    const panel = document.getElementById('ha-repl-topo-panel');
    if (!rows.length) {
        if (panel) panel.style.display = 'none';
        return;
    }
    if (panel) panel.style.display = '';

    const cols = [
        { label: 'Publisher',   key: 'publisher' },
        { label: 'Subscriber',  key: 'subscriber' },
        { label: 'Publication', key: 'publication' },
        { label: 'DB',          key: 'publication_db' },
        { label: 'Latency',     key: 'latency_seconds', type: 'number', align: 'right', format: (v) => v + 's' },
        { label: 'Throughput',  key: 'delivery_rate_cmds_sec', type: 'number', align: 'right', format: (v) => v + ' cmd/s' },
        { label: 'Status',      key: 'agent_status', format: (v) => `<span class="badge ${_agentBadge(v)}">${window.escapeHtml(v)}</span>` },
        { label: 'Last Sync',   key: 'last_sync_time', format: (v) => v ? _relTime(v) : '—' },
    ];

    _renderDataTable('ha-repl-topo-table', 'repl-topo-tbl', cols, rows, {
        rowClass: (r) => (r.has_error || r.agent_status === 'Failed') ? 'ha-row-danger' :
                         r.agent_status === 'Retrying' ? 'ha-row-warn'   : ''
    });
}

/* ============================================================
   REPLICATED ARTICLES TABLE
   ============================================================ */
async function _loadArticlesTable(instanceName) {
    try {
        const res  = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/replication/articles?instance=${encodeURIComponent(instanceName)}`
        );
        const data = await res.json();
        _renderArticlesTable(data.articles || []);
    } catch (e) {
        _setInner('ha-articles-table', '<p class="text-muted small">Articles unavailable.</p>');
    }
}

function _renderArticlesTable(articles) {
    const panel = document.getElementById('ha-articles-panel');
    if (!articles.length) {
        if (panel) panel.style.display = 'none';
        return;
    }
    if (panel) panel.style.display = '';

    const cols = [
        { label: 'Database',   key: 'database_name' },
        { label: 'Table',      key: (r) => `${r.schema_name}.${r.table_name}` },
        { label: 'Publication',key: 'publication' },
        { label: 'Subscriber', key: 'subscriber' },
        { label: 'Rows/s',     key: 'rows_per_sec', type: 'number', align: 'right' },
        { label: 'Latency',    key: 'latency_seconds', type: 'number', align: 'right', format: (v) => v + 's' },
        { label: 'Conflicts',  key: 'conflicts_detected', type: 'number', align: 'right', 
                               class: (v) => v > 0 ? 'text-danger' : '' },
        { label: 'Status',     key: 'status', format: (v) => `<span class="badge ${_agentBadge(v)}">${window.escapeHtml(v)}</span>` },
    ];

    _renderDataTable('ha-articles-table', 'articles-tbl', cols, articles);
}

/* ============================================================
   GENERIC DATA TABLE (Sorting, Filtering, Pagination)
   ============================================================ */
function _renderDataTable(containerId, tableId, cols, rows, options = {}) {
    const stateKey = `tbl_state_${tableId}`;
    if (!window._haTblState) window._haTblState = {};
    const state = window._haTblState[stateKey] || {
        page: 1,
        pageSize: 15,
        sortCol: -1,
        sortAsc: true,
        filters: {}
    };
    window._haTblState[stateKey] = state;

    // Apply filtering
    let filtered = [...rows];
    Object.keys(state.filters).forEach(key => {
        const val = state.filters[key].toLowerCase();
        if (!val) return;
        filtered = filtered.filter(r => {
            const col = cols.find(c => c.key === key || (typeof c.key === 'function' && c.label === key));
            let cellVal = '';
            if (typeof col.key === 'function') cellVal = col.key(r);
            else cellVal = r[col.key];
            return String(cellVal || '').toLowerCase().includes(val);
        });
    });

    // Apply sorting
    if (state.sortCol !== -1) {
        const col = cols[state.sortCol];
        filtered.sort((a, b) => {
            let av, bv;
            if (typeof col.key === 'function') { av = col.key(a); bv = col.key(b); }
            else { av = a[col.key]; bv = b[col.key]; }
            
            if (col.type === 'number') {
                return state.sortAsc ? (av - bv) : (bv - av);
            }
            return state.sortAsc ? String(av).localeCompare(String(bv)) : String(bv).localeCompare(String(av));
        });
    }

    // Pagination
    const totalPages = Math.ceil(filtered.length / state.pageSize);
    if (state.page > totalPages) state.page = Math.max(1, totalPages);
    const start = (state.page - 1) * state.pageSize;
    const paged = filtered.slice(start, start + state.pageSize);

    // Build Header — separate sort row and filter row for clean grid look
    const ths = cols.map((c, i) => {
        const sortIcon = state.sortCol === i ? (state.sortAsc ? ' ▲' : ' ▼') : ' ⇅';
        const alignCls = c.align === 'right' ? 'text-right' : '';
        return `<th class="ha-th-sort ${alignCls}" data-idx="${i}">
            <span class="ha-th-label">${c.label}</span><span class="ha-th-sort-icon">${sortIcon}</span>
        </th>`;
    }).join('');

    const filterInputs = cols.map((c) => {
        const colKey = typeof c.key === 'function' ? c.label : c.key;
        const alignCls = c.align === 'right' ? 'text-right' : '';
        return `<th class="ha-th-filter ${alignCls}">
            <input type="text" class="ha-search-input" placeholder="&#128269; filter…"
                   data-col="${colKey}"
                   value="${window.escapeHtml(String(state.filters[colKey] || ''))}">
        </th>`;
    }).join('');

    // Build Body
    const trs = paged.map(r => {
        const rowCls = options.rowClass ? options.rowClass(r) : '';
        const tds = cols.map(c => {
            let v;
            if (typeof c.key === 'function') v = c.key(r);
            else v = r[c.key];
            
            const cellCls = (typeof c.class === 'function' ? c.class(v) : c.class) || '';
            const alignCls = c.align === 'right' ? 'text-right' : '';
            const formatted = c.format ? c.format(v) : window.escapeHtml(String(v ?? ''));
            return `<td class="${alignCls} ${cellCls}">${formatted}</td>`;
        }).join('');
        return `<tr class="${rowCls}">${tds}</tr>`;
    }).join('');

    // Pagination controls
    let paginationHtml = '';
    if (totalPages > 1) {
        paginationHtml = `
            <div class="flex-between mt-2 px-1">
                <div class="small text-muted">Showing ${start + 1} to ${Math.min(start + state.pageSize, filtered.length)} of ${filtered.length}</div>
                <div class="flex-center gap-2">
                    <button class="btn btn-xs btn-outline" ${state.page === 1 ? 'disabled' : ''} data-page="${state.page - 1}">Prev</button>
                    <span class="small">Page ${state.page} of ${totalPages}</span>
                    <button class="btn btn-xs btn-outline" ${state.page === totalPages ? 'disabled' : ''} data-page="${state.page + 1}">Next</button>
                </div>
            </div>`;
    }

    _setInner(containerId, `
        <div class="ha-datagrid-wrap">
            <table class="ha-datagrid" id="${tableId}">
                <thead>
                    <tr class="ha-thead-sort">${ths}</tr>
                    <tr class="ha-thead-filter">${filterInputs}</tr>
                </thead>
                <tbody>${trs || '<tr><td colspan="' + cols.length + '" class="text-center p-4 text-muted">No matching rows</td></tr>'}</tbody>
            </table>
        </div>
        ${paginationHtml}
    `);

    // Attach events
    const container = document.getElementById(containerId);
    container.querySelectorAll('.ha-th-sort').forEach(th => {
        th.addEventListener('click', () => {
            const idx = parseInt(th.dataset.idx);
            if (state.sortCol === idx) state.sortAsc = !state.sortAsc;
            else { state.sortCol = idx; state.sortAsc = true; }
            _renderDataTable(containerId, tableId, cols, rows, options);
        });
    });

    container.querySelectorAll('.ha-search-input').forEach(input => {
        input.addEventListener('click', (e) => e.stopPropagation());
        input.addEventListener('input', () => {
            const col = input.dataset.col;
            state.filters[col] = input.value;
            state.page = 1;
            clearTimeout(window._haTblTimer);
            window._haTblTimer = setTimeout(() => {
                _renderDataTable(containerId, tableId, cols, rows, options);
            }, 350);
        });
    });

    container.querySelectorAll('[data-page]').forEach(btn => {
        btn.addEventListener('click', () => {
            state.page = parseInt(btn.dataset.page);
            _renderDataTable(containerId, tableId, cols, rows, options);
        });
    });
}



/* ============================================================
   ALERTS TIMELINE
   ============================================================ */
async function _loadAlertsTimeline(instanceName) {
    try {
        const from = window.appState.fromTs ? new Date(window.appState.fromTs).toISOString() : new Date(Date.now() - 86400000).toISOString();
        const to   = window.appState.toTs   ? new Date(window.appState.toTs).toISOString()   : new Date().toISOString();
        const res  = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/alerts-timeline?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
        );
        const data = await res.json();
        _renderAlertsTimeline(data.data || []);
    } catch (e) {
        _setInner('ha-alerts-timeline', '<p class="text-muted small">Alerts unavailable.</p>');
    }
}

function _renderAlertsTimeline(alerts) {
    if (!alerts.length) {
        _setInner('ha-alerts-timeline',
            '<p class="text-muted small p-2">No alerts in the selected range &mdash; system healthy.</p>');
        return;
    }
    const rows = alerts.map(a => {
        const dotCls = a.severity === 'critical' ? 'ha-dot-danger' : 'ha-dot-warn';
        return `
<div class="ha-timeline-row">
  <div class="ha-timeline-dot ${dotCls}"></div>
  <div class="ha-timeline-body">
    <span class="badge badge-outline" style="font-size:0.6rem;padding:1px 4px;">${window.escapeHtml(a.category)}</span>
    <strong>${window.escapeHtml(a.title)}</strong>
    <span class="text-muted" style="margin:0 4px">:</span>
    <span class="small">${window.escapeHtml(a.detail)}</span>
  </div>
  <div class="ha-timeline-time text-muted small">${_relTime(a.timestamp)}</div>
</div>`;
    }).join('');
    _setInner('ha-alerts-timeline', `<div class="ha-timeline">${rows}</div>`);
}

/* ============================================================
   FAILOVER HISTORY TIMELINE
   ============================================================ */
async function _loadFailoverHistory(instanceName) {
    try {
        const from = window.appState.fromTs ? new Date(window.appState.fromTs).toISOString() : new Date(Date.now() - 86400000).toISOString();
        const to   = window.appState.toTs   ? new Date(window.appState.toTs).toISOString()   : new Date().toISOString();
        const res  = await window.apiClient.authenticatedFetch(
            `/api/sqlserver/ha/failover-history?instance=${encodeURIComponent(instanceName)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
        );
        const data = await res.json();
        _renderFailoverHistory(data.events || []);
    } catch (e) {
        _setInner('ha-failover-history', '<p class="text-muted small">Failover history unavailable.</p>');
    }
}

function _renderFailoverHistory(events) {
    if (!events.length) {
        _setInner('ha-failover-history',
            '<p class="text-muted small p-2">No failover events in the selected range &mdash; good stability.</p>');
        return;
    }
    const rows = events.map(e => `
<div class="ha-timeline-row">
  <div class="ha-timeline-dot ha-dot-warn"></div>
  <div class="ha-timeline-body">
    <strong>${window.escapeHtml(e.ag_name)}</strong>
    <span class="text-muted" style="margin:0 4px">|</span>
    ${window.escapeHtml(e.previous_primary)} &rarr; <strong>${window.escapeHtml(e.new_primary)}</strong>
    <span class="ha-badge-type">${window.escapeHtml(e.failover_type)}</span>
    ${e.failover_duration_seconds > 0
        ? `<span class="text-muted small">(${e.failover_duration_seconds}s)</span>` : ''}
  </div>
  <div class="ha-timeline-time text-muted small">${_relTime(e.capture_timestamp)}</div>
</div>`).join('');
    _setInner('ha-failover-history', `<div class="ha-timeline">${rows}</div>`);
}

/* ============================================================
   TABLE SORT (numeric-aware, shift-click multi-sort)
   ============================================================ */
function _attachTableSort(containerId) {
    const container = document.getElementById(containerId);
    if (!container) return;
    const table = container.querySelector('table');
    if (!table) return;
    let sortCol = -1, sortAsc = true;

    table.querySelectorAll('th').forEach((th, idx) => {
        th.style.cursor = 'pointer';
        th.addEventListener('click', () => {
            if (sortCol === idx) sortAsc = !sortAsc;
            else { sortCol = idx; sortAsc = true; }

            const tbody = table.querySelector('tbody');
            const rows  = Array.from(tbody.querySelectorAll('tr'));
            rows.sort((a, b) => {
                const av = (a.cells[idx] && a.cells[idx].textContent.trim()) || '';
                const bv = (b.cells[idx] && b.cells[idx].textContent.trim()) || '';
                const an = parseFloat(av.replace(/[^0-9.-]/g, ''));
                const bn = parseFloat(bv.replace(/[^0-9.-]/g, ''));
                const cmp = (!isNaN(an) && !isNaN(bn)) ? an - bn : av.localeCompare(bv);
                return sortAsc ? cmp : -cmp;
            });
            rows.forEach(r => tbody.appendChild(r));

            // Update sort indicators
            table.querySelectorAll('th').forEach((h, i) => {
                h.textContent = h.textContent.replace(/ [▲▼]$/, '');
                if (i === idx) h.textContent += sortAsc ? ' ▲' : ' ▼';
            });
        });
    });
}

/* ============================================================
   UTILITY HELPERS
   ============================================================ */
function _setInner(id, html) {
    const el = document.getElementById(id);
    if (el) el.innerHTML = html;
}

function _fmtKB(kb) {
    if (kb == null || kb === 0) return '0';
    if (kb < 1024)        return `${kb} KB`;
    if (kb < 1048576)     return `${(kb / 1024).toFixed(1)} MB`;
    return `${(kb / 1048576).toFixed(2)} GB`;
}

function _shortSyncMode(desc) {
    if (!desc) return '—';
    return desc.includes('ASYNC') ? 'Async' : 'Sync';
}

function _shortTime(ts) {
    if (!ts) return '';
    const d = new Date(ts);
    const now = new Date();
    const isToday = d.getDate() === now.getDate() &&
                    d.getMonth() === now.getMonth() &&
                    d.getFullYear() === now.getFullYear();
    
    const time = `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`;
    if (isToday) return time;
    return `${d.getMonth() + 1}/${d.getDate()} ${time}`;
}

function _relTime(ts) {
    if (!ts) return '—';
    const diff = Date.now() - new Date(ts).getTime();
    if (diff < 60000)    return `${Math.floor(diff / 1000)}s ago`;
    if (diff < 3600000)  return `${Math.floor(diff / 60000)}m ago`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
    return new Date(ts).toLocaleDateString();
}

function _truncate(s, n) {
    if (!s) return '';
    return s.length > n ? s.slice(0, n) + '…' : s;
}

function _thresholdClass(threshold) {
    if (threshold === 'red')    return 'text-danger';
    if (threshold === 'yellow') return 'text-warning';
    return 'text-success';
}

function _syncHealthBadge(health) {
    const cls = health === 'HEALTHY'           ? 'badge-success' :
                health === 'PARTIALLY_HEALTHY' ? 'badge-warning' : 'badge-danger';
    return `<span class="badge ${cls}">${window.escapeHtml(health || 'UNKNOWN')}</span>`;
}

function _agentBadge(status) {
    if (!status) return 'badge-outline';
    switch (status.toLowerCase()) {
        case 'running': case 'idle': return 'badge-success';
        case 'retrying':             return 'badge-warning';
        case 'failed':               return 'badge-danger';
        default:                     return 'badge-outline';
    }
}

/* ============================================================
   SCOPED CSS — injected once per page mount
   ============================================================ */
function _haStyles() {
    if (document.getElementById('ha-dashboard-styles')) return '';
    return `<style id="ha-dashboard-styles">
/* ── Sticky banner ── */
.ha-banner {
    position: sticky; top: 0; z-index: 20;
    padding: 10px 16px; font-size: 0.82rem; font-weight: 500;
    border-radius: 0 0 6px 6px;
    display: flex; align-items: center; justify-content: space-between; gap: 12px;
    flex-wrap: wrap;
    transition: background 0.3s, border-color 0.3s;
}
.ha-banner-main { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ha-feature-badges { display: flex; flex-wrap: wrap; gap: 6px; flex-shrink: 0; }
.ha-tech-badge {
    font-size: 0.62rem; padding: 2px 8px; border-radius: 4px;
    background: rgba(255,255,255,0.08); border: 1px solid rgba(148,163,184,0.35);
    color: #94a3b8; text-transform: uppercase; letter-spacing: 0.04em;
}
.ha-tier-pill { font-size: 0.72rem; font-weight: 600; padding: 2px 8px; border-radius: 4px; }
.ha-tier-gold { background: #eab30822; color: #facc15; border: 1px solid #eab30855; }
.ha-tier-silver { background: #94a3b822; color: #cbd5e1; border: 1px solid #94a3b855; }
.ha-tier-bronze { background: #d9770622; color: #fb923c; border: 1px solid #d9770655; }
.ha-tier-red { background: #dc262622; color: #f87171; border: 1px solid #dc262655; }
.ha-badge-warn { font-size: 0.62rem; color: #fbbf24; margin-left: 4px; }
.ha-coverage-table { width: 100%; font-size: 0.78rem; }
.ha-coverage-table th { text-align: left; color: var(--color-muted); font-weight: 600; padding: 6px 8px; }
.ha-coverage-table td { padding: 6px 8px; border-top: 1px solid rgba(148,163,184,0.12); }

/* ── Technology tabs ── */
.ha-tech-tabs {
    display: flex; flex-wrap: wrap; gap: 6px;
    padding: 4px 0;
}
.ha-tab {
    font-size: 0.75rem; font-weight: 600; padding: 6px 14px;
    border-radius: 6px; border: 1px solid rgba(148,163,184,0.35);
    background: rgba(255,255,255,0.04); color: var(--color-muted);
    cursor: pointer; transition: background 0.15s, color 0.15s;
}
.ha-tab:hover { color: var(--text-primary); background: rgba(255,255,255,0.08); }
.ha-tab.active {
    color: #38bdf8; border-color: #38bdf866;
    background: rgba(56,189,248,0.12);
}
.ha-tab-panel { display: block; }
.ha-logshipping-table { width: 100%; font-size: 0.78rem; }
.ha-banner-green  { background:#16a34a18; border-bottom:2px solid #16a34a; color:#4ade80; }
.ha-banner-yellow { background:#d9770618; border-bottom:2px solid #d97706; color:#fbbf24; }
.ha-banner-red    { background:#dc262618; border-bottom:2px solid #dc2626; color:#f87171; }
.ha-banner-grey,.ha-banner-loading {
    background:rgba(255,255,255,0.03); border-bottom:1px solid rgba(255,255,255,0.1); color:var(--color-muted);
}

/* ── KPI row — fills full width ── */
.ha-kpi-flex {
    display:flex; flex-wrap:wrap; gap:0.6rem; align-items:stretch; width:100%;
}
/* Each individual KPI card */
.ha-kpi-flex > .ha-kpi-card { flex:1 1 130px; min-width:110px; }

/* Legacy inner grid used in ha-repl-kpi-row (inside repl section) */
.ha-kpi-row {
    display:grid;
    grid-template-columns:repeat(auto-fill,minmax(130px,1fr));
    gap:0.6rem;
}

@media(max-width:650px)  { .ha-kpi-flex > .ha-kpi-card { flex:1 1 45%; } }

.ml-2 { margin-left: 0.5rem; }
.ml-3 { margin-left: 1rem; }


/* ── Live badge ── */
.ha-badge-live {
    font-size:0.58rem; background:rgba(74,222,128,0.15);
    color:#4ade80; border:1px solid #4ade8040;
    padding:1px 5px; border-radius:4px; font-weight:500;
    text-transform:uppercase; letter-spacing:0.04em;
}

.ha-kpi-card {
    padding:0.85rem 0.75rem; text-align:center;
    display:flex; flex-direction:column; align-items:center; gap:3px;
}
.ha-kpi-icon  { font-size:1.25rem; margin-bottom:1px; }
.ha-kpi-label { font-size:0.67rem; text-transform:uppercase; letter-spacing:0.05em; color:var(--color-muted); }
.ha-kpi-value { font-size:1.1rem; font-weight:700; line-height:1.2; }
.ha-kpi-sub   { font-size:0.67rem; color:var(--color-muted); }
.ha-skeleton-card { min-height:90px; opacity:0.35; }

/* ── Multi-column rows ── */
.ha-row-3col { display:grid; grid-template-columns:1fr 1fr 1fr; gap:0.75rem; }
.ha-row-2col { display:grid; grid-template-columns:1fr 1fr; gap:0.75rem; }
@media(max-width:900px) { .ha-row-3col { grid-template-columns:1fr 1fr; } }
@media(max-width:800px) { .ha-row-2col { grid-template-columns:1fr; } }
@media(max-width:600px) { .ha-row-3col,.ha-kpi-row { grid-template-columns:1fr; } }

/* ── Topology row ── */
.ha-row-topo { display:grid; grid-template-columns:360px 1fr; gap:0.75rem; }
@media(max-width:900px) { .ha-row-topo { grid-template-columns:1fr; } }
.ha-topo-card,.ha-replica-card,.ha-chart-card { padding:0.85rem; }

/* ── Card header ── */
.ha-card-hdr {
    font-size:0.75rem; font-weight:600; text-transform:uppercase;
    letter-spacing:0.04em; color:var(--color-muted); margin-bottom:0.5rem;
    display:flex; align-items:center; justify-content:space-between;
}
.ha-badge-info {
    font-size:0.6rem; background:rgba(255,255,255,0.07);
    padding:1px 6px; border-radius:4px; font-weight:400;
    text-transform:none; letter-spacing:0;
}

/* ── Failover readiness ── */
.ha-readiness-body  { display:flex; flex-direction:column; gap:2px; }
.ha-checks-list     { display:flex; flex-direction:column; gap:4px; }
.ha-check-row       { display:flex; align-items:flex-start; gap:6px; font-size:0.77rem; }
.ha-check-icon      { width:16px; text-align:center; font-weight:700; flex-shrink:0; margin-top:1px; }
.ha-check-pass      { color:#4ade80; }
.ha-check-fail      { color:#f87171; }
.ha-check-unknown   { color:var(--color-muted); }
.ha-check-name      { flex:1; }
.ha-check-detail    { font-size:0.67rem; color:#f87171; margin-left:4px; }
.ha-ready-verdict   {
    margin-top:10px; padding:8px 10px; border-radius:6px;
    font-size:0.82rem; font-weight:700; text-align:center;
}
.ha-ready-yes { background:#16a34a22; color:#4ade80; border:1px solid #16a34a44; }
.ha-ready-no  { background:#dc262622; color:#f87171; border:1px solid #dc262644; }
@keyframes ha-danger-pulse {
    0%,100% { box-shadow:0 0 0 0 rgba(220,38,38,0.25); }
    50%      { box-shadow:0 0 0 8px rgba(220,38,38,0);  }
}
.ha-card-danger-pulse { animation:ha-danger-pulse 2s ease-in-out infinite; border:1px solid #dc262660 !important; }

/* ── Row highlight ── */
.ha-row-warn   { background:rgba(245,158,11,0.07) !important; }
.ha-row-danger { background:rgba(239,68,68,0.08)  !important; }
.text-right    { text-align:right; }

/* ── Standard data grid ── */
.ha-datagrid-wrap {
    overflow-x:auto;
    border:1px solid rgba(148,163,184,0.25);
    border-radius:6px;
    box-shadow: inset 0 0 0 0 transparent;
}
.ha-datagrid {
    width:100%; border-collapse:collapse; font-size:0.78rem;
}
.ha-datagrid thead th {
    background:rgba(99,102,241,0.10);
    border-bottom:2px solid rgba(148,163,184,0.30);
    border-right:1px solid rgba(148,163,184,0.18);
    color:var(--color-muted); font-weight:700;
    text-transform:uppercase; letter-spacing:0.05em;
    white-space:nowrap;
}
.ha-datagrid thead th:last-child { border-right:none; }
.ha-thead-sort th { padding:8px 10px 5px; }
.ha-thead-filter th {
    padding:4px 6px 6px;
    background:rgba(255,255,255,0.04);
    border-top:1px solid rgba(148,163,184,0.15);
    border-bottom:1px solid rgba(148,163,184,0.22);
    border-right:1px solid rgba(148,163,184,0.12);
}
.ha-thead-filter th:last-child { border-right:none; }
.ha-th-sort { cursor:pointer; user-select:none; }
.ha-th-sort:hover { background:rgba(99,102,241,0.15) !important; }
.ha-th-label { margin-right:4px; }
.ha-th-sort-icon { font-size:0.65rem; color:#64748b; }
.ha-datagrid tbody td {
    padding:6px 10px;
    border-bottom:1px solid rgba(148,163,184,0.13);
    border-right:1px solid rgba(148,163,184,0.10);
    vertical-align:middle;
}
.ha-datagrid tbody td:last-child { border-right:none; }
.ha-datagrid tbody tr:hover td { background:rgba(99,102,241,0.07); }
.ha-datagrid tbody tr:nth-child(even) td { background:rgba(255,255,255,0.02); }
.ha-datagrid tbody tr:nth-child(even):hover td { background:rgba(99,102,241,0.07); }
.ha-datagrid tbody tr:last-child td { border-bottom:none; }

/* ── Column filter input ── */
.ha-search-input {
    background:rgba(255,255,255,0.07); border:1px solid rgba(148,163,184,0.2);
    color:var(--color-text); border-radius:4px; padding:3px 7px;
    font-size:0.72rem; width:100%; min-width:50px; box-sizing:border-box;
}
.ha-search-input:focus { outline:none; border-color:var(--color-accent); background:rgba(255,255,255,0.12); }
.ha-search-input::placeholder { color:#475569; }

/* ── Legacy table (replica health, simple sort only) ── */
.ha-table-wrap { overflow-x:auto; border:1px solid rgba(148,163,184,0.25); border-radius:6px; }
.ha-table {
    width:100%; border-collapse:collapse; font-size:0.8rem;
}
.ha-table thead { background:rgba(99,102,241,0.10); }
.ha-table th {
    border-bottom:2px solid rgba(148,163,184,0.30);
    border-right:1px solid rgba(148,163,184,0.18);
    padding:8px 10px; color:var(--color-muted); font-weight:700;
    text-transform:uppercase; font-size:0.68rem; letter-spacing:0.05em; white-space:nowrap;
}
.ha-table th:last-child { border-right:none; }
.ha-table td {
    padding:6px 10px;
    border-bottom:1px solid rgba(148,163,184,0.13);
    border-right:1px solid rgba(148,163,184,0.10);
    vertical-align:middle;
}
.ha-table td:last-child { border-right:none; }
.ha-table tbody tr:hover td { background:rgba(99,102,241,0.07); }
.ha-table tbody tr:nth-child(even) td { background:rgba(255,255,255,0.02); }
.ha-table tbody tr:nth-child(even):hover td { background:rgba(99,102,241,0.07); }
.ha-table tbody tr:last-child td { border-bottom:none; }

/* ── Timeline ── */
.ha-timeline     { display:flex; flex-direction:column; gap:6px; }
.ha-timeline-row { display:flex; align-items:center; gap:10px; font-size:0.77rem; padding:3px 0; }
.ha-timeline-dot { width:10px; height:10px; border-radius:50%; flex-shrink:0; }
.ha-dot-warn     { background:#fbbf24; }
.ha-dot-danger   { background:#f87171; }
.ha-timeline-body { flex:1; }
.ha-timeline-time { white-space:nowrap; }
.ha-badge-type {
    font-size:0.62rem; background:rgba(255,255,255,0.07);
    padding:1px 5px; border-radius:3px; margin-left:6px;
}

/* ── Skeleton ── */
.ha-skeleton-line {
    height:10px; border-radius:4px; background:rgba(255,255,255,0.08);
    margin-bottom:6px;
    animation:ha-skel 1.5s ease-in-out infinite;
}
.ha-skeleton-line.short { width:60%; }
@keyframes ha-skel { 0%,100%{opacity:.4} 50%{opacity:.9} }

.p-2  { padding:0.5rem; }
.mb-4 { margin-bottom:1.5rem; }
</style>`;
}
