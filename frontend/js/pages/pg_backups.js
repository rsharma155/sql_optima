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
  };

  els.limit.addEventListener('change', refresh);
  els.search.addEventListener('input', applyFilter);

  await refresh();
};

