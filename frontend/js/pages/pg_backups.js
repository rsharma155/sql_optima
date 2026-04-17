/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: PostgreSQL backup status and history page.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

window.PgBackupsView = async function PgBackupsView() {
  const inst = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
  const instanceName = inst?.name;
  if (!instanceName) {
    window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select a Postgres instance first</h3></div>`;
    return;
  }

  const html = await window.loadTemplate('/pages/pg_backups.html');
  window.routerOutlet.innerHTML = html;
  if (window.renderStatusStrip) window.renderStatusStrip(window.routerOutlet, inst);

  const els = {
    limit: document.getElementById('pgBackupLimit'),
    search: document.getElementById('pgBackupSearch'),
    meta: document.getElementById('pgBackupMeta'),
    tbody: document.getElementById('pgBackupTbody'),
  };

  const esc = (v) => window.escapeHtml(v || '');
  const fmtTs = (s) => {
    try { return new Date(s).toLocaleString(); } catch { return s || ''; }
  };
  const fmtBytes = (n) => {
    const v = Number(n || 0);
    if (!isFinite(v) || v <= 0) return '-';
    const units = ['B','KB','MB','GB','TB'];
    let x = v, i = 0;
    while (x >= 1024 && i < units.length - 1) { x /= 1024; i++; }
    return `${x.toFixed(i >= 2 ? 2 : 0)} ${units[i]}`;
  };
  const statusClass = (s) => {
    const v = (s || '').toLowerCase();
    if (v === 'success') return 'text-success';
    if (v === 'failed') return 'text-danger';
    if (v === 'partial') return 'text-warning';
    return 'text-muted';
  };

  let cached = [];
  const applyFilter = () => {
    const q = (els.search.value || '').trim().toLowerCase();
    const rows = q
      ? cached.filter((r) => {
          const hay = [r.tool, r.backup_type, r.status, r.error_message].join(' ').toLowerCase();
          return hay.includes(q);
        })
      : cached;
    if (!rows.length) {
      els.tbody.innerHTML = `<tr><td colspan="7" class="text-center text-muted">No matching rows</td></tr>`;
      return;
    }
    els.tbody.innerHTML = rows.map((r) => {
      return `
        <tr>
          <td>${esc(fmtTs(r.started_at))}</td>
          <td>${esc(fmtTs(r.finished_at))}</td>
          <td class="${statusClass(r.status)}">${esc((r.status || '').toUpperCase())}</td>
          <td>${esc(r.tool || '-')}</td>
          <td>${esc(r.backup_type || '-')}</td>
          <td>${esc(fmtBytes(r.size_bytes))}</td>
          <td>${esc(r.error_message || '')}</td>
        </tr>
      `;
    }).join('');
  };

  const refresh = async () => {
    els.meta.textContent = 'Loading…';
    els.tbody.innerHTML = `<tr><td colspan="7" class="text-center text-muted">Loading…</td></tr>`;
    const limit = parseInt(els.limit.value || '100', 10) || 100;
    let payload;
    try {
      payload = await window.apiClient.authenticatedFetch(
        `/api/postgres/backups/history?instance=${encodeURIComponent(instanceName)}&limit=${encodeURIComponent(String(limit))}`
      );
    } catch (e) {
      els.meta.textContent = 'Failed to load backup history';
      els.tbody.innerHTML = `<tr><td colspan="7" class="text-center text-danger">Error: ${esc(e?.message || 'request failed')}</td></tr>`;
      return;
    }
    cached = Array.isArray(payload?.runs) ? payload.runs : (Array.isArray(payload) ? payload : []);
    els.meta.textContent = `${cached.length} run(s)`;
    applyFilter();

    // RPO / WAL Archiver Risk strip
    window.apiClient.authenticatedFetch(`/api/postgres/wal/archiver-risk?instance=${encodeURIComponent(instanceName)}`)
      .then(r => r.ok ? r.json() : null)
      .then(walPayload => {
        const strip = document.getElementById('pgBackupRPOStrip');
        if (!strip || !walPayload?.risk) return;
        const risk = walPayload.risk;
        const ageSec = risk.last_archived_age;
        const lvl = risk.risk_level || 'low';
        const cls = lvl === 'critical' ? 'alert-danger' : lvl === 'high' ? 'alert-warning' : lvl === 'medium' ? 'alert-warning' : 'alert-success';
        const icon = lvl === 'critical' || lvl === 'high' ? 'fa-triangle-exclamation' : lvl === 'medium' ? 'fa-circle-info' : 'fa-circle-check';
        const fmtAge = (s) => {
          if (!isFinite(s) || s < 0) return '—';
          if (s < 60) return `${Math.round(s)}s`;
          if (s < 3600) return `${(s/60).toFixed(1)}m`;
          return `${(s/3600).toFixed(2)}h`;
        };
        const rpoColor = ageSec < 300 ? 'text-success' : ageSec < 1800 ? 'text-warning' : 'text-danger';
        strip.innerHTML = `
          <div class="alert ${cls}" style="padding:.5rem .75rem;border-radius:6px;font-size:0.82rem;display:flex;align-items:center;gap:.75rem;flex-wrap:wrap;">
            <i class="fa-solid ${icon}"></i>
            <strong>RPO Posture: ${lvl.toUpperCase()}</strong>
            <span>WAL archive lag: <strong class="${rpoColor}">${fmtAge(ageSec)}</strong></span>
            <span class="text-muted">&nbsp;|&nbsp; Archived: <strong>${Number(risk.archived_count || 0).toLocaleString()}</strong></span>
            <span class="text-muted">&nbsp;|&nbsp; Failed: <strong class="${Number(risk.failed_count) > 0 ? 'text-danger' : ''}">${Number(risk.failed_count || 0)}</strong></span>
            ${Number(risk.max_retained_slot_mb || 0) > 0
              ? `<span class="text-muted">&nbsp;|&nbsp; Slot retention: <strong class="${Number(risk.max_retained_slot_mb) >= 256 ? 'text-warning' : ''}">${Number(risk.max_retained_slot_mb).toFixed(1)} MB</strong></span>`
              : ''}
          </div>`;
      })
      .catch(() => {});
  };

  els.limit.addEventListener('change', refresh);
  els.search.addEventListener('input', applyFilter);

  await refresh();
};

