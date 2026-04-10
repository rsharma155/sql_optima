window.PgLogsView = async function PgLogsView() {
  const inst = window.appState?.config?.instances?.[window.appState.currentInstanceIdx];
  const instanceName = inst?.name;
  if (!instanceName) {
    window.routerOutlet.innerHTML = `<div class="page-view active"><h3 class="text-warning">Please select a Postgres instance first</h3></div>`;
    return;
  }

  const html = await window.loadTemplate('/pages/pg_logs.html');
  window.routerOutlet.innerHTML = html;
  if (window.renderStatusStrip) window.renderStatusStrip(window.routerOutlet, inst);

  const els = {
    sev: document.getElementById('pgLogsSeverity'),
    limit: document.getElementById('pgLogsLimit'),
    search: document.getElementById('pgLogsSearch'),
    meta: document.getElementById('pgLogsMeta'),
    tbody: document.getElementById('pgLogsTbody'),
  };

  const fmtTs = (s) => {
    try { return new Date(s).toLocaleString(); } catch { return s || ''; }
  };
  const sevClass = (s) => {
    const v = (s || '').toLowerCase();
    if (v === 'panic' || v === 'fatal') return 'text-danger';
    if (v === 'error') return 'text-warning';
    if (v === 'warning') return 'text-muted';
    return 'text-muted';
  };
  const esc = (v) => window.escapeHtml(v || '');

  let cached = [];
  const applyFilter = () => {
    const q = (els.search.value || '').trim().toLowerCase();
    const rows = q
      ? cached.filter((r) => {
          const hay = [
            r.severity, r.sqlstate, r.message,
            r.database_name, r.user_name, r.application_name, r.client_addr,
          ].join(' ').toLowerCase();
          return hay.includes(q);
        })
      : cached;

    if (!rows.length) {
      els.tbody.innerHTML = `<tr><td colspan="6" class="text-center text-muted">No matching events</td></tr>`;
      return;
    }

    els.tbody.innerHTML = rows.map((r) => {
      const dbUser = [r.database_name, r.user_name].filter(Boolean).join(' / ') || '-';
      const appClient = [r.application_name, r.client_addr].filter(Boolean).join(' / ') || '-';
      const rawJson = r.raw ? JSON.stringify(r.raw, null, 2) : '';
      const title = rawJson ? `title="${esc(rawJson)}"` : '';
      return `
        <tr ${title} style="cursor:${rawJson ? 'help' : 'default'};">
          <td>${esc(fmtTs(r.capture_timestamp))}</td>
          <td class="${sevClass(r.severity)}">${esc((r.severity || '').toUpperCase())}</td>
          <td><code>${esc(r.sqlstate || '')}</code></td>
          <td>${esc(r.message || '')}</td>
          <td>${esc(dbUser)}</td>
          <td>${esc(appClient)}</td>
        </tr>
      `;
    }).join('');
  };

  const refresh = async () => {
    els.meta.textContent = 'Loading…';
    els.tbody.innerHTML = `<tr><td colspan="6" class="text-center text-muted">Loading…</td></tr>`;
    const severity = els.sev.value || 'error';
    const limit = parseInt(els.limit.value || '200', 10) || 200;

    let payload;
    try {
      payload = await window.apiClient.authenticatedFetch(
        `/api/postgres/logs/recent?instance=${encodeURIComponent(instanceName)}&severity=${encodeURIComponent(severity)}&limit=${encodeURIComponent(String(limit))}`
      );
    } catch (e) {
      els.meta.textContent = 'Failed to load log events';
      els.tbody.innerHTML = `<tr><td colspan="6" class="text-center text-danger">Error: ${esc(e?.message || 'request failed')}</td></tr>`;
      return;
    }

    cached = Array.isArray(payload?.events) ? payload.events : [];
    els.meta.textContent = `${cached.length} event(s) • source: ${payload?.source || 'unknown'}`;
    applyFilter();
  };

  els.sev.addEventListener('change', refresh);
  els.limit.addEventListener('change', refresh);
  els.search.addEventListener('input', applyFilter);

  await refresh();
};

