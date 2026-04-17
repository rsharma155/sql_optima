/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Central event-delegation handler that replaces inline onclick/onsubmit/onchange
 *   attributes (which violate CSP 'script-src 'self'') with declarative data-action attributes.
 *
 * Usage in HTML / JS templates:
 *   data-action="navigate"   data-route="pg-dashboard"
 *   data-action="navigate-back"
 *   data-action="reload"
 *   data-action="call"       data-fn="PgDashboardView"
 *   data-action="close-id"   data-target="some-element-id"
 *   data-action="remove-closest" data-selector=".form-wrapper"
 *   data-action="scroll-id"  data-target="SomeAnchorId"
 *   data-action="show-admin-tab"  data-tab="users"
 *   data-action="show-settings-tab" data-tab="dashboards"
 *   data-action="show-query-modal-direct" data-key="cacheKey"
 *   data-action="jobs-detail"  data-idx="0"
 *   data-action="test-server"  data-id="serverId"
 *   data-action="patch-server-active" data-id="serverId" data-active="true"
 *   data-action="show-edit-server" data-id="serverId"
 *   data-action="rotate-server" data-id="serverId"
 *   data-action="delete-server" data-id="serverId"
 *   data-action="delete-user"   data-id="userId"
 *   data-action="change-user-role" data-id="userId" data-role="admin"
 *   data-action="open-widget-editor" data-id="widgetId"
 *   data-action="restore-widget-default" data-id="widgetId"
 *   data-action="test-widget-query" data-id="widgetId"
 *   data-action="save-widget-sql" data-id="widgetId"
 *   data-action="close-widget-editor"
 *   data-action="refresh-dynamic-dashboard" data-section=""
 *   data-action="copy-text"  data-text="some sql text"
 *   data-action="toggle-next"   (toggles display:none on nextElementSibling)
 *   data-action="pg-cpu-query-details" data-idx="0"
 *   data-action="pg-load-dead-trend" data-schema="public" data-table="mytable"
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function () {
    'use strict';

    function dispatch(el, event) {
        const action = el.dataset.action;
        if (!action) return;

        // Prevent default for <a> elements to avoid navigation
        if (el.tagName === 'A') event.preventDefault();

        switch (action) {
            case 'navigate': {
                const route = el.dataset.route;
                if (route && window.appNavigate) window.appNavigate(route);
                const also = el.dataset.alsoCall;
                if (also && typeof window[also] === 'function') window[also]();
                break;
            }
            case 'navigate-back': {
                if (window.appNavigateBack) window.appNavigateBack();
                break;
            }
            case 'reload': {
                location.reload();
                break;
            }
            case 'call': {
                const fn = el.dataset.fn;
                if (!fn || typeof window[fn] !== 'function') break;
                if (el.dataset.stopPropagation) event.stopPropagation();
                if (el.dataset.passEl) { window[fn](el); break; }
                if (el.dataset.argFrom) {
                    const parts = el.dataset.argFrom.split('.');
                    let v = window;
                    for (const p of parts) v = v && v[p];
                    window[fn](v);
                    break;
                }
                const arg = el.dataset.arg;
                const idx = el.dataset.idx;
                if (idx != null) { window[fn](parseInt(idx, 10)); break; }
                if (el.dataset.id != null && el.dataset.active != null) {
                    window[fn](el.dataset.id, el.dataset.active === 'true');
                    break;
                }
                if (el.dataset.id != null) { window[fn](el.dataset.id); break; }
                if (el.dataset.metric && el.dataset.value && el.dataset.sample) {
                    window[fn](el.dataset.metric, el.dataset.value, decodeURIComponent(el.dataset.sample));
                    break;
                }
                if (arg != null) {
                    window[fn](arg === 'true' ? true : arg === 'false' ? false : arg);
                    break;
                }
                window[fn]();
                break;
            }
            case 'close-id': {
                const target = document.getElementById(el.dataset.target);
                if (target) target.remove();
                break;
            }
            case 'remove-closest': {
                const sel = el.dataset.selector;
                const ancestor = sel ? el.closest(sel) : el.parentElement;
                if (ancestor) ancestor.remove();
                break;
            }
            case 'scroll-id': {
                const anchor = document.getElementById(el.dataset.target);
                if (anchor) anchor.scrollIntoView({ behavior: 'smooth' });
                break;
            }
            case 'show-admin-tab': {
                if (window.showAdminTab) window.showAdminTab(el.dataset.tab);
                break;
            }
            case 'show-settings-tab': {
                if (window.showSettingsTab) window.showSettingsTab(el.dataset.tab);
                break;
            }
            case 'show-query-modal-direct': {
                const key = el.dataset.key;
                const cache = el.dataset.cache;
                const encodedQuery = el.dataset.encodedQuery;
                let queryText;
                if (encodedQuery) {
                    queryText = decodeURIComponent(encodedQuery);
                } else if (cache && key && window.appState && window.appState[cache]) {
                    queryText = window.appState[cache][key];
                } else if (key && window.appState) {
                    queryText = (window.appState.emQueryCache && window.appState.emQueryCache[key])
                        || (window.appState.queryCache && window.appState.queryCache[key]);
                }
                const fnName = el.dataset.fn || 'showQueryModalDirect';
                if (queryText != null && typeof window[fnName] === 'function') window[fnName](queryText);
                break;
            }
            case 'jobs-detail': {
                const idx = parseInt(el.dataset.idx, 10);
                const msgs = window.appState && window.appState.jobFailureMessages;
                const msg = (msgs && msgs[idx] != null) ? msgs[idx] : (el.dataset.msg || '');
                if (window.showJobFailureDetail) window.showJobFailureDetail(idx, msg);
                break;
            }
            case 'test-server': {
                if (window.testAdminServer) window.testAdminServer(el.dataset.id);
                break;
            }
            case 'patch-server-active': {
                if (window.patchServerActive)
                    window.patchServerActive(el.dataset.id, el.dataset.active === 'true');
                break;
            }
            case 'show-edit-server': {
                if (window.showEditServerForm) window.showEditServerForm(el.dataset.id);
                break;
            }
            case 'rotate-server': {
                if (window.rotateAdminServerPrompt) window.rotateAdminServerPrompt(el.dataset.id);
                break;
            }
            case 'delete-server': {
                if (window.deleteAdminServer) window.deleteAdminServer(el.dataset.id);
                break;
            }
            case 'delete-user': {
                if (window.deleteUser) window.deleteUser(parseInt(el.dataset.id, 10));
                break;
            }
            case 'change-user-role': {
                if (window.changeUserRole)
                    window.changeUserRole(parseInt(el.dataset.id, 10), el.dataset.role);
                break;
            }
            case 'open-widget-editor': {
                if (window.openWidgetEditor) window.openWidgetEditor(el.dataset.id);
                break;
            }
            case 'restore-widget-default': {
                if (window.restoreWidgetDefault) window.restoreWidgetDefault(el.dataset.id);
                break;
            }
            case 'test-widget-query': {
                if (window.testWidgetQuery) window.testWidgetQuery(el.dataset.id);
                break;
            }
            case 'save-widget-sql': {
                if (window.saveWidgetSql) window.saveWidgetSql(el.dataset.id);
                break;
            }
            case 'close-widget-editor': {
                if (window.closeWidgetEditor) window.closeWidgetEditor();
                break;
            }
            case 'refresh-dynamic-dashboard': {
                if (window.refreshDynamicDashboard)
                    window.refreshDynamicDashboard(el.dataset.section || '');
                break;
            }
            case 'copy-text': {
                let text = el.dataset.text || '';
                if (el.dataset.encoded) text = decodeURIComponent(text);
                navigator.clipboard.writeText(text).then(function () {
                    const orig = el.innerHTML;
                    el.innerHTML = '<i class="fa-solid fa-check"></i> Copied';
                    setTimeout(function () { el.innerHTML = orig; }, 2000);
                }).catch(function () {});
                break;
            }
            case 'toggle-next': {
                const sib = el.nextElementSibling;
                if (sib) sib.style.display = sib.style.display === 'none' ? 'block' : 'none';
                break;
            }
            case 'pg-cpu-query-details': {
                const idx = parseInt(el.dataset.idx, 10);
                if (window.pgCpuShowQueryDetails) window.pgCpuShowQueryDetails(idx);
                break;
            }
            case 'pg-load-dead-trend': {
                if (window.pgLoadTableDeadTrend)
                    window.pgLoadTableDeadTrend(el.dataset.schema || '', el.dataset.table || '');
                break;
            }
            case 'sort-pg-queries': {
                if (window.sortPgQueries) window.sortPgQueries(el, el.dataset.sort);
                break;
            }
            case 'show-create-user': {
                if (window.showCreateUserForm) window.showCreateUserForm();
                break;
            }
            case 'show-add-server': {
                if (window.showAddServerForm) window.showAddServerForm();
                break;
            }
            case 'test-server-add-draft': {
                if (window.testServerAddDraft) window.testServerAddDraft();
                break;
            }
            case 'submit-add-server': {
                if (window.submitAddServer) window.submitAddServer();
                break;
            }
            case 'load-admin-servers': {
                if (window.loadAdminServers) window.loadAdminServers();
                break;
            }
            case 'submit-edit-server': {
                if (window.submitEditServer) window.submitEditServer();
                break;
            }
            case 'logout': {
                if (window._auth && window._auth.logout) window._auth.logout();
                break;
            }
            case 'new-dashboard': {
                if (window.createNewDashboard) window.createNewDashboard();
                break;
            }
            case 'toggle-metric-selection': {
                if (window.toggleMetricSelection) window.toggleMetricSelection(el);
                break;
            }
            case 'guardrails-modal': {
                if (window.showGuardrailsModal)
                    window.showGuardrailsModal(el.dataset.category, el.dataset.catid);
                break;
            }
            case 'refresh-best-practices': {
                if (window.refreshBestPractices) window.refreshBestPractices();
                break;
            }
            case 'refresh-alerts': {
                if (window.refreshAlerts) window.refreshAlerts();
                break;
            }
            case 'dismiss-alert-banner': {
                if (window.dismissSqlDashboardAlertBanner)
                    window.dismissSqlDashboardAlertBanner(parseInt(el.dataset.hours || '1', 10));
                break;
            }
            case 'pg-storage-navigate': {
                if (window.appNavigate) window.appNavigate(el.dataset.route);
                break;
            }
            case 'toggle-section': {
                const sib = el.parentElement && el.parentElement.nextElementSibling;
                if (sib) sib.classList.toggle('hidden');
                const icon = el.querySelector('i.fa-chevron-down, i.fa-chevron-up, i.fa-chevron');
                if (icon) {
                    icon.classList.toggle('fa-chevron-down');
                    icon.classList.toggle('fa-chevron-up');
                }
                break;
            }
            case 'copy-closest-fix': {
                const drawer = el.closest('.drawer-content');
                if (drawer && drawer.dataset.fix) {
                    navigator.clipboard.writeText(drawer.dataset.fix).then(function () {
                        el.innerHTML = '<i class="fa-solid fa-check"></i> Copied';
                        setTimeout(function () { el.innerHTML = '<i class="fa-solid fa-copy"></i> Copy'; }, 2000);
                    }).catch(function () {});
                }
                break;
            }
            default:
                break;
        }
    }

    // Single delegated listener on document for click events
    document.addEventListener('click', function (e) {
        let el = e.target;
        while (el && el !== document.body) {
            if (el.dataset && el.dataset.action) {
                dispatch(el, e);
                return;
            }
            el = el.parentElement;
        }
    });

    // Keyboard accessibility: also handle Enter/Space on elements with data-action
    document.addEventListener('keydown', function (e) {
        if (e.key !== 'Enter' && e.key !== ' ') return;
        let el = e.target;
        while (el && el !== document.body) {
            if (el.dataset && el.dataset.action) {
                e.preventDefault();
                dispatch(el, e);
                return;
            }
            el = el.parentElement;
        }
    });

    // Handle form submit delegation
    document.addEventListener('submit', function (e) {
        let el = e.target;
        if (el.dataset && el.dataset.submitAction) {
            const fn = el.dataset.submitAction;
            if (typeof window[fn] === 'function') {
                e.preventDefault();
                window[fn](e);
            }
        }
    });

    // Handle select onchange delegation
    document.addEventListener('change', function (e) {
        const el = e.target;
        if (el.dataset && el.dataset.changeAction) {
            const fn = el.dataset.changeAction;
            if (typeof window[fn] === 'function') {
                const metric = el.dataset.metric;
                if (metric) {
                    // For toggleCollection: checkbox → pass checked bool; for updateCollectionInterval: pass value
                    if (el.type === 'checkbox') {
                        window[fn](metric, el.checked);
                    } else {
                        window[fn](metric, el.value);
                    }
                } else {
                    window[fn](el.value);
                }
            }
        }
    });

})();
