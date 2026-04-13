/**
 * PostgreSQL EXPLAIN plan analyzer — analyze/optimize APIs + plan graph + SQL-linked hints (heuristic).
 */
window.PgExplainView = function PgExplainView() {
    const inst = window.appState.config?.instances?.[window.appState.currentInstanceIdx] || { name: '--', type: 'postgres' };
    /** Escape for HTML; must preserve numeric 0 (global escapeHtml used to use !x which hid 0). */
    function esc(s) {
        if (s === null || s === undefined) return '';
        var str = String(s);
        return window.escapeHtml ? window.escapeHtml(str) : str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;');
    }

    window.routerOutlet.innerHTML = `
        <div class="page-view active dashboard-sky-theme pg-explain-page">
            <div class="page-title flex-between dashboard-page-title-compact">
                <div class="dashboard-title-line" style="flex:1; min-width:0;">
                    <h1><i class="fa-solid fa-diagram-project text-accent"></i> EXPLAIN Plan Analyzer</h1>
                    <span class="subtitle">Instance: <span class="text-accent">${esc(inst.name)}</span> — paste a JSON plan and SQL for analysis, optimization report, and optional <strong>index advisor</strong> (HypoPG when the server can reach your DB).</span>
                </div>
                <div class="flex-between dashboard-page-title-actions" style="gap:0.5rem; flex-wrap:wrap;">
                    <button type="button" class="btn btn-sm btn-outline" onclick="window.appNavigate('pg-queries')"><i class="fa-solid fa-bolt"></i> Query Performance</button>
                    <button type="button" class="btn btn-sm btn-outline text-accent" onclick="window.PgExplainView()"><i class="fa-solid fa-rotate-right"></i> Reset</button>
                </div>
            </div>

            <div class="glass-panel mt-3" style="padding:1rem 1.1rem;">
                <div class="pg-explain-layout">
                    <div class="pg-explain-field">
                        <label class="pg-explain-field-label" for="pgExplainQuery">Optional: original SQL (strongly recommended)</label>
                        <p class="pg-explain-field-hint">Required for <strong>Index advisor &amp; rewrites</strong>. Also enables SQL line excerpts and heuristic index hints. Not executed on the server.</p>
                        <textarea id="pgExplainQuery" class="pg-explain-textarea pg-explain-textarea--sql" rows="4" spellcheck="false" placeholder="SELECT u.id, u.name FROM users u WHERE u.status = 'active';"></textarea>
                    </div>

                    <div class="pg-explain-field">
                    <!--
                        <div class="pg-explain-plan-banner" role="note">
                            <i class="fa-solid fa-paste" style="margin-top:0.15rem; color:var(--accent);"></i>
                            <span><strong>Paste JSON here</strong> — required. Output from <code>EXPLAIN (…, FORMAT JSON)</code> only.</span>
                        </div>
                        -->
                        <label class="pg-explain-field-label" for="pgExplainPlan">Execution plan <span class="badge badge-danger" style="font-size:0.65rem; vertical-align:middle;">Required</span></label>
                        <p class="pg-explain-field-hint" style="margin-bottom: 0.5rem;"><strong>Note:</strong> Run <code>EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) …</code> in your database client (JSON is required for this page). Max ~512 KB. Must be valid JSON (tree + metrics + SQL insights are driven from the JSON plan).</p>
                        <!--
                        <p class="pg-explain-field-hint">Max ~512 KB. Must be valid JSON (tree + metrics + SQL insights are driven from the JSON plan).</p>
                        -->
                        <textarea id="pgExplainPlan" class="pg-explain-textarea pg-explain-textarea--plan" rows="12" spellcheck="false" placeholder='Example: [ { &quot;Plan&quot;: { &quot;Node Type&quot;: &quot;Seq Scan&quot;, ... }, &quot;Planning Time&quot;: 0.1, &quot;Execution Time&quot;: 2.3 } ]'></textarea>
                    </div>

                    <div class="pg-explain-actions mt-2">
                        <button type="button" class="btn btn-accent" id="pgExplainBtnAnalyze"><i class="fa-solid fa-magnifying-glass-chart"></i> Analyze plan</button>
                        <button type="button" class="btn btn-outline text-accent" id="pgExplainBtnOptimize"><i class="fa-solid fa-wand-magic-sparkles"></i> Optimization report</button>
                        <button type="button" class="btn btn-outline text-accent" id="pgExplainBtnIndexAdvisor" title="Uses JSON plan + SQL; connects to DB when a single Postgres instance is configured or you pass instance name via UI selection"><i class="fa-solid fa-compass-drafting"></i> Index advisor &amp; rewrites</button>
                    </div>
                    <p class="pg-explain-field-hint" style="margin-top: 0.5rem; font-size: 0.85rem;"><strong>Tip:</strong> Use <strong>Analyze plan</strong> for findings + <strong>Plan map</strong> + <strong>Metrics</strong>. Use <strong>Optimization report</strong> for the narrative score. Use <strong>Index advisor &amp; rewrites</strong> for structured index DDL and query rewrite suggestions (requires <code>EXPLAIN (FORMAT JSON)</code> output and SQL text).</p>
                    <div id="pgExplainErr" class="alert alert-danger" style="display:none; font-size:0.82rem; margin:0;"></div>
                </div>
            </div>

            <div id="pgExplainResultsHost" class="mt-3" style="display:none;"></div>
        </div>
    `;

    const planEl = document.getElementById('pgExplainPlan');
    const queryEl = document.getElementById('pgExplainQuery');
    const errEl = document.getElementById('pgExplainErr');
    const host = document.getElementById('pgExplainResultsHost');

    function showErr(msg) {
        if (!errEl) return;
        errEl.style.display = msg ? 'block' : 'none';
        errEl.textContent = msg || '';
    }

    function severityClass(sev) {
        const s = String(sev || '').toLowerCase();
        if (s === 'critical') return 'badge-danger';
        if (s === 'high') return 'badge-danger';
        if (s === 'medium') return 'badge-warning';
        if (s === 'low') return 'badge-info';
        return 'badge-outline';
    }

    function severityToTone(sev) {
        var s = String(sev || '').toLowerCase();
        if (s === 'critical' || s === 'high') return 'danger';
        if (s === 'medium') return 'warn';
        if (s === 'low') return 'info';
        return 'muted';
    }

    function operatorIconClass(nodeType) {
        var t = String(nodeType || '').toLowerCase();
        if (t.indexOf('seq scan') >= 0 || t.indexOf('scan') >= 0) return 'fa-table';
        if (t.indexOf('index') >= 0) return 'fa-magnifying-glass';
        if (t.indexOf('nested loop') >= 0) return 'fa-link';
        if (t.indexOf('hash join') >= 0 || t.indexOf('merge join') >= 0 || t.indexOf('join') >= 0) return 'fa-code-branch';
        if (t.indexOf('sort') >= 0) return 'fa-arrow-down-a-z';
        if (t.indexOf('aggregate') >= 0 || t.indexOf('group') >= 0) return 'fa-layer-group';
        if (t.indexOf('append') >= 0 || t.indexOf('union') >= 0) return 'fa-diagram-project';
        if (t.indexOf('limit') >= 0) return 'fa-gauge';
        return 'fa-square';
    }

    function formatNum(n) {
        if (n == null || n === '' || Number.isNaN(Number(n))) return '—';
        var x = Number(n);
        if (Math.abs(x) >= 10000) return x.toExponential(2);
        return x.toLocaleString(undefined, { maximumFractionDigits: 2 });
    }

    function formatMsMaybe(ms) {
        if (ms == null || ms === '') return '—';
        var x = Number(ms);
        if (Number.isNaN(x)) return '—';
        return x.toFixed(2) + ' ms';
    }

    // Some plan sources don't provide stable per-node `id` (often 0/missing).
    // We synthesize a UI-stable id so the graph/table can render all nodes.
    var __pgExplainUIDSeq = 0;
    function ensureNodeUID(node) {
        if (!node || typeof node !== 'object') return null;
        if (node.__pg_uid) return node.__pg_uid;
        __pgExplainUIDSeq += 1;
        Object.defineProperty(node, '__pg_uid', { value: __pgExplainUIDSeq, enumerable: false, configurable: true });
        return node.__pg_uid;
    }

    function nodeUIKey(node) {
        if (!node || typeof node !== 'object') return '';
        // Prefer planner id when it's present and non-zero.
        if (typeof node.id === 'number' && node.id > 0) return 'p' + String(node.id);
        var u = ensureNodeUID(node);
        return 'u' + String(u || 0);
    }

    /** Deep key normalize: PG "Node Type", camelCase, etc. → snake_case (matches backend normalizer). */
    function pgJsonKeyToSnake(k) {
        if (k === 'Plan') return 'plan';
        if (k === 'Plans') return 'plans';
        if (/\s/.test(k)) return k.replace(/\s+/g, '_').toLowerCase();
        if (/_/.test(k)) return k.toLowerCase();
        return k
            .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
            .replace(/([A-Z])([A-Z][a-z])/g, '$1_$2')
            .toLowerCase();
    }

    function normalizePgExplainObject(v) {
        if (v === null || v === undefined) return v;
        if (Array.isArray(v)) return v.map(normalizePgExplainObject);
        if (typeof v !== 'object') return v;
        var out = {};
        Object.keys(v).forEach(function (k) {
            var nk = pgJsonKeyToSnake(k);
            if (Object.prototype.hasOwnProperty.call(out, nk)) return;
            out[nk] = normalizePgExplainObject(v[k]);
        });
        return out;
    }

    /**
     * Copy common EXPLAIN JSON key variants onto the snake_case fields the UI expects.
     * Handles native PostgreSQL ("Node Type"), camelCase ("NodeType"), and mis-nested { Plan: {...} } children.
     */
    function hydratePlanTree(root) {
        function h(n) {
            if (!n || typeof n !== 'object') return;
            if (n.node_type == null || n.node_type === '') {
                if (n['Node Type'] != null && n['Node Type'] !== '') n.node_type = n['Node Type'];
                else if (n.NodeType != null && n.NodeType !== '') n.node_type = n.NodeType;
                else if (n.nodetype != null && n.nodetype !== '') n.node_type = n.nodetype;
            }
            if (n.relation_name == null && n['Relation Name'] != null) n.relation_name = n['Relation Name'];
            if (n.relation_name == null && n.RelationName != null) n.relation_name = n.RelationName;
            if (n.index_name == null && n['Index Name'] != null) n.index_name = n['Index Name'];
            if (n.join_type == null && n['Join Type'] != null) n.join_type = n['Join Type'];
            if (n.filter == null && n.Filter != null) n.filter = n.Filter;
            if (n.plan_rows == null && n['Plan Rows'] != null) n.plan_rows = n['Plan Rows'];
            if (n.plan_rows == null && n.PlanRows != null) n.plan_rows = n.PlanRows;
            if (n.plan_width == null && n['Plan Width'] != null) n.plan_width = n['Plan Width'];
            if (n.startup_cost == null && n['Startup Cost'] != null) n.startup_cost = Number(n['Startup Cost']);
            if (n.startup_cost == null && n.StartupCost != null) n.startup_cost = Number(n.StartupCost);
            if (n.total_cost == null && n['Total Cost'] != null) n.total_cost = Number(n['Total Cost']);
            if (n.total_cost == null && n.TotalCost != null) n.total_cost = Number(n.TotalCost);
            if (n.actual_rows == null && n['Actual Rows'] != null) n.actual_rows = n['Actual Rows'];
            if (n.actual_loops == null && n['Actual Loops'] != null) n.actual_loops = n['Actual Loops'];
            if (n.actual_startup_time == null && n['Actual Startup Time'] != null) n.actual_startup_time = Number(n['Actual Startup Time']);
            if (n.actual_total_time == null && n['Actual Total Time'] != null) n.actual_total_time = Number(n['Actual Total Time']);
            if ((!n.plans || !n.plans.length) && n.Plans && n.Plans.length) n.plans = n.Plans;
            var kids = n.plans || [];
            for (var i = 0; i < kids.length; i++) {
                var ch = kids[i];
                if (ch && typeof ch === 'object' && (ch['Plan'] || ch.Plan) && (ch.node_type == null || ch.node_type === '') && !ch.plans && !ch.Plans) {
                    var inner = ch['Plan'] != null ? ch['Plan'] : ch.Plan;
                    if (inner && typeof inner === 'object') {
                        kids[i] = inner;
                    }
                }
                h(kids[i]);
            }
        }
        h(root);
    }

    function planTreeHasAnyNodeType(node) {
        if (!node || typeof node !== 'object') return false;
        if (node.node_type != null && String(node.node_type).trim() !== '') return true;
        var kids = node.plans || [];
        for (var i = 0; i < kids.length; i++) {
            if (planTreeHasAnyNodeType(kids[i])) return true;
        }
        return false;
    }

    /** Rebuild a nested plan from API plan_graph (flat nodes + parent_id) when result.plan.plan lacks fields. */
    function planGraphToRoot(graph) {
        if (!graph || !graph.nodes || graph.nodes.length === 0) return null;
        var list = graph.nodes;
        var byGid = {};
        for (var i = 0; i < list.length; i++) {
            var gn = list[i];
            var o = {
                id: gn.planner_node_id != null ? gn.planner_node_id : 0,
                node_type: gn.node_type || '',
                relation_name: gn.relation_name || '',
                alias: gn.alias || '',
                index_name: gn.index_name || '',
                total_cost: gn.total_cost != null ? Number(gn.total_cost) : 0,
                plan_rows: gn.plan_rows != null ? Number(gn.plan_rows) : 0,
                actual_rows: gn.actual_rows != null ? gn.actual_rows : null,
                actual_total_time: gn.actual_total_time_ms != null ? Number(gn.actual_total_time_ms) : null,
                exclusive_time: gn.exclusive_time_ms != null ? Number(gn.exclusive_time_ms) : null,
                inclusive_time: gn.actual_total_time_ms != null ? Number(gn.actual_total_time_ms) : null,
                filter: gn.filter || '',
                plans: []
            };
            byGid[gn.id] = o;
        }
        var root = null;
        for (var j = 0; j < list.length; j++) {
            var g = list[j];
            var node = byGid[g.id];
            if (!node) continue;
            var pid = g.parent_id;
            if (pid == null || pid === '') {
                root = node;
            } else {
                var p = byGid[pid];
                if (p) p.plans.push(node);
            }
        }
        return root;
    }

    function pickAnalyzePlanRoot(data) {
        var r = data.result;
        if (!r) return null;
        var cand = [];
        if (r.plan && r.plan.plan) cand.push(r.plan.plan);
        if (r.plan_tree) cand.push(r.plan_tree);
        for (var i = 0; i < cand.length; i++) {
            var root = normalizePgExplainObject(cand[i]);
            hydratePlanTree(root);
            if (planTreeHasAnyNodeType(root)) return root;
        }
        var g = planGraphToRoot(data.plan_graph);
        if (g) {
            g = normalizePgExplainObject(g);
            hydratePlanTree(g);
            if (planTreeHasAnyNodeType(g)) return g;
        }
        if (cand.length) {
            var fb = normalizePgExplainObject(cand[0]);
            hydratePlanTree(fb);
            return fb;
        }
        return g;
    }

    function flattenPlanRows(node, out) {
        if (!node || typeof node !== 'object') return;
        ensureNodeUID(node);
        out.push(node);
        var kids = node.plans || [];
        for (var i = 0; i < kids.length; i++) {
            flattenPlanRows(kids[i], out);
        }
    }

    function findingsByNode(findings) {
        var m = {};
        (findings || []).forEach(function (f) {
            if (f.node_id == null) return;
            var k = f.node_id;
            if (!m[k]) m[k] = [];
            m[k].push(f);
        });
        return m;
    }

    function heatStyleExclusive(ex, maxEx) {
        var x = ex != null ? Number(ex) : 0;
        if (!maxEx || maxEx <= 0 || !x || Number.isNaN(x)) return '';
        var t = Math.min(1, x / maxEx);
        var a = 0.06 + t * 0.48;
        return 'background: rgba(239, 68, 68, ' + a + ');';
    }

    function rowsCrossCol(planRows, actualRows) {
        var pr = planRows != null ? Number(planRows) : NaN;
        var ar = actualRows != null ? Number(actualRows) : NaN;
        if (!pr || pr <= 0 || Number.isNaN(pr)) return '—';
        if (Number.isNaN(ar)) return '—';
        var r = ar / pr;
        var arrow = r > 1.2 ? '↑ ' : (r < 0.8 ? '↓ ' : '');
        return arrow + r.toFixed(2);
    }

    function bufferSummary(node) {
        if (!node) return '—';
        var b = node.buffers;
        if (b && typeof b === 'object') {
            var parts = [];
            if (b.shared_hit) parts.push('hit ' + b.shared_hit);
            if (b.shared_read) parts.push('read ' + b.shared_read);
            if (b.shared_written) parts.push('written ' + b.shared_written);
            if (b.temp_read) parts.push('temp r ' + b.temp_read);
            if (b.temp_written) parts.push('temp w ' + b.temp_written);
            if (parts.length) return parts.join(' · ');
        }
        var sh = node.shared_hit_buffers;
        var sr = node.shared_read_buffers;
        if (sh || sr) {
            var p2 = [];
            if (sh) p2.push('hit ' + sh);
            if (sr) p2.push('read ' + sr);
            return p2.join(' · ');
        }
        return '—';
    }

    function collectBadges(node, findings) {
        var out = [];
        var seen = {};
        function add(label, tone) {
            var k = String(label).toLowerCase();
            if (seen[k]) return;
            seen[k] = 1;
            out.push({ label: label, tone: tone });
        }
        (findings || []).forEach(function (f) {
            var shortType = (f.type || '').replace(/_/g, ' ');
            if (shortType.length > 28) shortType = shortType.slice(0, 26) + '…';
            add(shortType, severityToTone(f.severity));
        });
        var pr = node.plan_rows || 0;
        var ar = node.actual_rows != null ? node.actual_rows : 0;
        if (pr > 100 && pr > 0) {
            var ratio = ar / pr;
            if (ratio > 10 || ratio < 0.1) add('mis-estimate', 'info');
        }
        var nt = node.node_type || '';
        if (nt.indexOf('Seq Scan') >= 0 && (Number(node.exclusive_time) > 5 || (node.rows_removed_by_filter || 0) > 5000)) {
            add('slow scan', 'warn');
        }
        var tc = node.total_cost != null ? Number(node.total_cost) : 0;
        if (tc >= 1000) {
            add('high cost', 'warn');
        }
        var pr = node.plan_rows != null ? Number(node.plan_rows) : 0;
        var ar = node.actual_rows != null ? Number(node.actual_rows) : 0;
        if (pr > 0 && ar > 0 && (ar / pr >= 5 || pr / ar >= 5)) {
            add('row est. ×5', 'info');
        }
        return out;
    }

    function operatorKind(node) {
        var t = String(node?.node_type || '').toLowerCase();
        if (t.indexOf('sort') >= 0) return 'sort';
        if (t.indexOf('aggregate') >= 0 || t.indexOf('group') >= 0 || t.indexOf('windowagg') >= 0) return 'aggregate';
        if (t.indexOf('merge join') >= 0) return 'merge';
        if (t.indexOf('hash join') >= 0 || t.indexOf('nested loop') >= 0 || t.indexOf('join') >= 0) return 'join';
        if (t.indexOf('scan') >= 0) return 'scan';
        return 'other';
    }

    function nodeLabel(node) {
        var s = String(node?.node_type || '?');
        if (node?.relation_name) s += ' · ' + node.relation_name;
        return s;
    }

    function nodeSubtitle(node) {
        if (!node || typeof node !== 'object') return '';
        if (node.relation_name) return String(node.relation_name) + (node.alias ? (' AS ' + node.alias) : '');
        if (node.index_name) return 'Index: ' + String(node.index_name);
        if (node.join_type) return String(node.join_type);
        if (node.sort_method) return 'Sort: ' + String(node.sort_method);
        return '';
    }

    function estimateNodeWidthPx(node) {
        // Rough estimate (no DOM measurement): keep tiles compact but allow wider titles.
        var title = String(nodeLabel(node) || '');
        var sub = String(nodeSubtitle(node) || '');
        var maxLen = Math.max(title.length, sub.length);
        // ~6.2px/char + base paddings/icons
        var w = 88 + Math.min(120, Math.round(maxLen * 6.2));
        return Math.max(118, Math.min(190, w));
    }

    function safeRows(node) {
        var r = node?.actual_rows != null ? Number(node.actual_rows) : Number(node?.plan_rows || 0);
        if (!Number.isFinite(r) || r < 0) return 0;
        return r;
    }

    function flowRows(node) {
        // Approximate rows flowing through this node into its parent.
        // Prefer actual rows × loops when present.
        var rows = node?.actual_rows != null ? Number(node.actual_rows) : Number(node?.plan_rows || 0);
        var loops = node?.actual_loops != null ? Number(node.actual_loops) : 1;
        if (!Number.isFinite(rows) || rows < 0) rows = 0;
        if (!Number.isFinite(loops) || loops < 1) loops = 1;
        var v = rows * loops;
        if (!Number.isFinite(v) || v < 0) return 0;
        return v;
    }

    function strokeWidthForRows(rows, maxRows) {
        if (!maxRows || maxRows <= 0) return 2.2;
        var r = Math.max(0, Number(rows || 0));
        var t = Math.log10(r + 1) / Math.log10(maxRows + 1);
        var w = 2.2 + t * 7.8;
        return Math.max(2.2, Math.min(10, w));
    }

    function buildPlanLayout(root) {
        var nodes = [];
        var byId = {};
        function walk(n, depth, parentId) {
            if (!n || typeof n !== 'object') return;
            ensureNodeUID(n);
            var id = nodeUIKey(n);
            if (!byId[id]) {
                byId[id] = { id: id, node: n, depth: depth, parentId: parentId ? String(parentId) : null, children: [], x: 0, y: 0, w: 150, leafOrder: -1 };
                nodes.push(byId[id]);
            }
            var rec = byId[id];
            rec.depth = depth;
            rec.parentId = parentId ? String(parentId) : null;
            rec.children = [];
            var kids = n.plans || [];
            for (var i = 0; i < kids.length; i++) {
                ensureNodeUID(kids[i]);
                rec.children.push(nodeUIKey(kids[i]));
                walk(kids[i], depth + 1, id);
            }
        }
        walk(root, 0, null);

        var leafY = 0;
        function assignY(id) {
            var r = byId[id];
            if (!r) return 0;
            if (!r.children || r.children.length === 0) {
                r.leafOrder = leafY++;
                r.y = r.leafOrder;
                return r.y;
            }
            var ys = [];
            for (var i = 0; i < r.children.length; i++) ys.push(assignY(r.children[i]));
            var minY = Math.min.apply(null, ys);
            var maxY = Math.max.apply(null, ys);
            r.y = (minY + maxY) / 2;
            return r.y;
        }
        assignY(nodeUIKey(root));

        var maxDepth = 0;
        var maxRows = 0;
        nodes.forEach(function (r) {
            if (r.depth > maxDepth) maxDepth = r.depth;
            var rr = flowRows(r.node);
            if (rr > maxRows) maxRows = rr;
        });

        // Estimate widths, then compute per-depth column widths.
        var depthMaxW = new Array(maxDepth + 1).fill(0);
        nodes.forEach(function (r) {
            r.w = estimateNodeWidthPx(r.node);
            depthMaxW[r.depth] = Math.max(depthMaxW[r.depth], r.w);
        });

        var gapX = 30; // tighter gaps between columns
        var rowH = 88; // tighter vertical spacing
        var padX = 24;
        var padY = 20;
        var depthX = new Array(maxDepth + 1).fill(0);
        var acc = padX;
        for (var d = 0; d <= maxDepth; d++) {
            depthX[d] = acc;
            acc += depthMaxW[d] + gapX;
        }

        nodes.forEach(function (r) {
            r.x = depthX[r.depth];
            r.y = padY + r.y * rowH;
        });

        var width = acc + padX;
        var height = padY * 2 + Math.max(1, leafY) * rowH + 120;
        return { nodes: nodes, byId: byId, width: width, height: height, maxRows: maxRows };
    }

    function renderHorizontalPlanGraph(root, idxById, fnByNode) {
        var layout = buildPlanLayout(root);
        var maxRows = layout.maxRows || 0;
        var totalCost = (root && root.total_cost != null) ? Number(root.total_cost) : 0;

        var edges = [];
        edges.push('<svg class="pg-explain-graph-svg" width="' + esc(layout.width) + '" height="' + esc(layout.height) + '" viewBox="0 0 ' + esc(layout.width) + ' ' + esc(layout.height) + '" xmlns="http://www.w3.org/2000/svg">');
        // Important: marker must NOT scale with stroke-width, otherwise thick edges create huge arrowheads.
        edges.push('<defs><marker id="pgExplainArrow" markerUnits="userSpaceOnUse" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="10" markerHeight="10" orient="auto">' +
            '<path d="M 0 0 L 10 5 L 0 10 z" fill="rgba(100,116,139,0.78)"/></marker></defs>');

        layout.nodes.forEach(function (r) {
            if (!r.parentId) return;
            var p = layout.byId[r.parentId];
            if (!p) return;
            var x1 = r.x + (r.w || 150);
            var y1 = r.y + 30;
            var x2 = p.x;
            var y2 = p.y + 30;
            var dx = Math.max(30, Math.abs(x2 - x1) * 0.5);
            var c1x = x1 + dx;
            var c1y = y1;
            var c2x = x2 - dx;
            var c2y = y2;
            var w = strokeWidthForRows(flowRows(r.node), maxRows);
            edges.push(
                '<path class="pg-explain-graph-edge" data-edge-from="' + esc(r.id) + '" data-edge-to="' + esc(p.id) + '" d="M ' + esc(x1) + ' ' + esc(y1) + ' C ' + esc(c1x) + ' ' + esc(c1y) + ', ' + esc(c2x) + ' ' + esc(c2y) + ', ' + esc(x2) + ' ' + esc(y2) + '" ' +
                'fill="none" stroke="rgba(100,116,139,0.75)" stroke-width="' + esc(w) + '" marker-end="url(#pgExplainArrow)" stroke-linecap="round"/>'
            );
        });
        edges.push('</svg>');

        var tiles = layout.nodes.map(function (r) {
            var n = r.node;
            var kind = operatorKind(n);
            var icon = operatorIconClass(n.node_type);
            var k = nodeUIKey(n);
            var idx = idxById[k] || 0;
            var findings = (typeof n.id === 'number' && n.id > 0) ? (fnByNode[n.id] || []) : [];
            var badges = collectBadges(n, findings);
            var badgeHtml = badges.slice(0, 3).map(function (b) {
                return '<span class="pg-explain-pg-badge pg-explain-pg-badge--' + esc(b.tone) + '">' + esc(b.label) + '</span>';
            }).join('');

            var flags = [];
            if (kind === 'sort') flags.push('<span class="pg-explain-op-flag pg-explain-op-flag--sort">SORT</span>');
            if (kind === 'aggregate') flags.push('<span class="pg-explain-op-flag pg-explain-op-flag--agg">AGG</span>');
            if (kind === 'merge') flags.push('<span class="pg-explain-op-flag pg-explain-op-flag--merge">MERGE</span>');
            if (kind === 'join') flags.push('<span class="pg-explain-op-flag pg-explain-op-flag--join">JOIN</span>');
            if (n.filter && kind !== 'scan') flags.push('<span class="pg-explain-op-flag pg-explain-op-flag--filter">FILTER</span>');

            var title = nodeLabel(n);
            var sub = nodeSubtitle(n);

            var est = n.plan_rows != null ? String(n.plan_rows) : '—';
            var act = n.actual_rows != null ? String(n.actual_rows) : '—';
            var ex = n.exclusive_time != null ? Number(n.exclusive_time).toFixed(2) + ' ms' : '';
            var costPct = '';
            if (totalCost && n.total_cost != null) {
                var pct = (Number(n.total_cost) / totalCost) * 100;
                if (Number.isFinite(pct)) costPct = pct.toFixed(1) + '%';
            }

            return '<div class="pg-explain-op pg-explain-op--' + esc(kind) + '" role="button" tabindex="0" data-node-key="' + esc(r.id) + '" style="left:' + esc(r.x) + 'px; top:' + esc(r.y) + 'px; width:' + esc(r.w || 150) + 'px;">' +
                '<div class="pg-explain-op-hd">' +
                '<span class="pg-explain-op-icon"><i class="fa-solid ' + esc(icon) + '"></i></span>' +
                '<div class="pg-explain-op-txt">' +
                '<div class="pg-explain-op-title" title="' + esc(title) + '">' + esc(title) + '</div>' +
                (sub ? '<div class="pg-explain-op-sub text-muted">' + esc(sub) + '</div>' : '') +
                '</div>' +
                '<span class="pg-explain-op-idx">#' + esc(idx) + '</span>' +
                '</div>' +
                '<div class="pg-explain-op-m3">' +
                '<div class="pg-explain-op-mrow"><span class="pg-explain-op-mlab">Cost</span><span class="pg-explain-op-mval">' + (costPct ? esc(costPct) : '—') + '</span></div>' +
                '<div class="pg-explain-op-mrow"><span class="pg-explain-op-mlab">Est</span><span class="pg-explain-op-mval">' + esc(est) + '</span></div>' +
                '<div class="pg-explain-op-mrow"><span class="pg-explain-op-mlab">Act</span><span class="pg-explain-op-mval">' + esc(act) + '</span></div>' +
                '</div>' +
                (flags.length ? '<div class="pg-explain-op-flags">' + flags.join('') + '</div>' : '') +
                (badgeHtml ? '<div class="pg-explain-op-badges">' + badgeHtml + '</div>' : '') +
                '</div>';
        }).join('');

        return '<div class="pg-explain-graph-scroll"><div class="pg-explain-graph-wrap" style="width:' + esc(layout.width) + 'px; height:' + esc(layout.height) + 'px;">' +
            edges.join('') +
            '<div class="pg-explain-graph-tiles">' + tiles + '</div>' +
            '<div id="pgExplainGraphTooltip" class="pg-explain-graph-tooltip" style="display:none;"></div>' +
            '</div></div>';
    }

    function renderPgaCardTree(node, idxById, fnByNode, depth) {
        if (!node || typeof node !== 'object') return '';
        var idx = idxById[node.id] || 0;
        var kids = node.plans || [];
        var badges = collectBadges(node, fnByNode[node.id] || []);
        var estCost = node.total_cost != null ? formatNum(node.total_cost) : '—';
        var estRows = node.plan_rows != null ? String(node.plan_rows) : '—';
        var actRows = node.actual_rows != null ? String(node.actual_rows) : '—';
        var collapseBtn = kids.length
            ? '<button type="button" class="pg-explain-pg-collapse" aria-expanded="true" title="Collapse children">−</button>'
            : '<span class="pg-explain-pg-collapse-spacer" aria-hidden="true"></span>';
        var badgeHtml = badges.map(function (b) {
            return '<span class="pg-explain-pg-badge pg-explain-pg-badge--' + esc(b.tone) + '">' + esc(b.label) + '</span>';
        }).join('');
        var icon = operatorIconClass(node.node_type);

        var card =
            '<div class="pg-explain-pg-card" role="button" tabindex="0" data-planner-id="' + esc(node.id) + '">' +
            '<div class="pg-explain-pg-card-hd">' +
            collapseBtn +
            '<span class="pg-explain-pg-icon" aria-hidden="true"><i class="fa-solid ' + esc(icon) + '"></i></span>' +
            '<span class="pg-explain-pg-title">' + esc(node.node_type || '?') + '</span>' +
            '<span class="pg-explain-pg-idx" title="Order in metrics table">' + esc(idx) + '</span>' +
            '</div>' +
            '<div class="pg-explain-pg-metrics">' +
            '<div><span class="text-muted">Est. cost</span><br/><span class="pg-explain-pg-metric-val">' + esc(estCost) + '</span></div>' +
            '<div><span class="text-muted">Est. rows</span><br/><span class="pg-explain-pg-metric-val">' + esc(estRows) + '</span></div>' +
            '<div><span class="text-muted">Actual rows</span><br/><span class="pg-explain-pg-metric-val">' + esc(actRows) + '</span></div>' +
            '</div>' +
            (badgeHtml ? '<div class="pg-explain-pg-badges">' + badgeHtml + '</div>' : '') +
            '</div>';

        var childrenHtml = '';
        if (kids.length) {
            childrenHtml = '<div class="pg-explain-pg-children">' + kids.map(function (k) {
                return renderPgaCardTree(k, idxById, fnByNode, depth + 1);
            }).join('') + '</div>';
        }
        return '<div class="pg-explain-pg-item" style="--pg-depth:' + depth + '">' + card + childrenHtml + '</div>';
    }

    function renderDetailPanel(node, idx, findings) {
        if (!node) {
            return '<div class="pg-explain-detail-empty text-muted" style="padding:1rem;">Select a plan step on the left.</div>';
        }
        var type = esc(node.node_type || '?');
        var rel = node.relation_name ? esc(node.relation_name) : '';
        var alias = node.alias ? esc(node.alias) : '';
        var target = '';
        if (rel) {
            target = 'on ' + rel;
            if (alias) target += ' AS ' + alias;
        }
        var ins = (findings || []).map(function (f) {
            return '<div class="pg-explain-detail-insight"><span class="badge ' + severityClass(f.severity) + '">' + esc(f.severity) + '</span> ' +
                '<strong>' + esc(f.type || '') + '</strong><div class="mt-1">' + esc(f.message || '') + '</div>' +
                (f.suggestion ? '<div class="text-muted mt-1" style="font-size:0.74rem;">' + esc(f.suggestion) + '</div>' : '') +
                '</div>';
        }).join('');

        var overview =
            (ins ? '<div class="pg-explain-detail-section"><h4>Insights</h4>' + ins + '</div>' : '<p class="text-muted">No analyzer findings for this node.</p>') +
            '<div class="pg-explain-detail-section"><h4>Estimates vs actual</h4>' +
            '<div class="pg-explain-detail-kv"><span>Startup cost</span><span>' + esc(formatNum(node.startup_cost)) + '</span></div>' +
            '<div class="pg-explain-detail-kv"><span>Total cost</span><span>' + esc(formatNum(node.total_cost)) + '</span></div>' +
            '<div class="pg-explain-detail-kv"><span>Exclusive time</span><span>' + esc(formatMsMaybe(node.exclusive_time)) + '</span></div>' +
            '<div class="pg-explain-detail-kv"><span>Inclusive time</span><span>' + esc(formatMsMaybe(node.inclusive_time != null ? node.inclusive_time : node.actual_total_time)) + '</span></div>' +
            '<div class="pg-explain-detail-kv"><span>Actual loops</span><span>' + esc(node.actual_loops != null ? node.actual_loops : '—') + '</span></div>' +
            '</div>' +
            (node.filter ? '<div class="pg-explain-detail-section"><h4>Filter</h4><pre class="pg-explain-detail-pre">' + esc(node.filter) + '</pre></div>' : '') +
            (node.rows_removed_by_filter ? '<div class="pg-explain-detail-section"><h4>Rows removed by filter</h4><p><strong>' + esc(node.rows_removed_by_filter) + '</strong></p></div>' : '');

        var bufLines = [];
        if (node.buffers && typeof node.buffers === 'object') {
            var b = node.buffers;
            if (b.shared_hit != null) bufLines.push('shared hit = ' + b.shared_hit);
            if (b.shared_read != null) bufLines.push('shared read = ' + b.shared_read);
            if (b.shared_written != null) bufLines.push('shared written = ' + b.shared_written);
            if (b.shared_dirtied != null) bufLines.push('shared dirtied = ' + b.shared_dirtied);
            if (b.local_hit != null) bufLines.push('local hit = ' + b.local_hit);
            if (b.temp_read != null) bufLines.push('temp read = ' + b.temp_read);
            if (b.temp_written != null) bufLines.push('temp written = ' + b.temp_written);
        }
        if (node.shared_hit_buffers != null) bufLines.push('shared hit (flat) = ' + node.shared_hit_buffers);
        if (node.shared_read_buffers != null) bufLines.push('shared read (flat) = ' + node.shared_read_buffers);
        var ioBody = bufLines.length
            ? '<pre class="pg-explain-detail-pre">' + esc(bufLines.join('\n')) + '</pre>'
            : '<p class="text-muted">No per-node buffer stats in this plan.</p>';

        var condParts = [];
        if (node.index_cond) condParts.push('Index condition:\n' + node.index_cond);
        if (node.join_filter) condParts.push('Join filter:\n' + node.join_filter);
        if (node.hash_cond) condParts.push('Hash condition:\n' + node.hash_cond);
        if (node.merge_cond) condParts.push('Merge condition:\n' + node.merge_cond);
        var condBody = condParts.length
            ? condParts.map(function (c) { return '<pre class="pg-explain-detail-pre">' + esc(c) + '</pre>'; }).join('')
            : '<p class="text-muted">No extra conditions on this node.</p>';

        function renderAllPropsTable(n) {
            if (!n || typeof n !== 'object') return '<p class="text-muted">No properties.</p>';
            var keys = Object.keys(n).filter(function (k) {
                return k !== 'plans' && k !== 'parent' && k !== 'depth' && k !== '__pg_uid';
            }).sort();
            if (!keys.length) return '<p class="text-muted">No properties.</p>';
            var rows = keys.map(function (k) {
                var v = n[k];
                var display = '';
                if (v == null) display = '—';
                else if (typeof v === 'string') display = v;
                else display = JSON.stringify(v);
                return '<tr><td class="pg-explain-prop-k">' + esc(k) + '</td><td class="pg-explain-prop-v"><pre class="pg-explain-prop-pre">' + esc(display) + '</pre></td></tr>';
            }).join('');
            return '<div class="pg-explain-prop-wrap"><table class="pg-explain-prop-table"><tbody>' + rows + '</tbody></table></div>';
        }

        var propsBody = renderAllPropsTable(node);
        var srcBody = '<pre class="pg-explain-detail-pre pg-explain-detail-pre--json">' + esc(JSON.stringify(node, null, 2)) + '</pre>';

        return '<div class="pg-explain-detail-root">' +
            '<div class="pg-explain-detail-hd">' + type + ' <span class="text-muted" style="font-weight:500;">#' + esc(idx) + '</span></div>' +
            (target ? '<div class="pg-explain-detail-target text-muted">' + esc(target) + '</div>' : '') +
            '<div class="pg-explain-detail-tabs" role="tablist">' +
            '<button type="button" class="pg-explain-detail-tab pg-explain-detail-tab--active" data-detail-tab="overview" role="tab">Overview</button>' +
            '<button type="button" class="pg-explain-detail-tab" data-detail-tab="io" role="tab">I/O &amp; buffers</button>' +
            '<button type="button" class="pg-explain-detail-tab" data-detail-tab="cond" role="tab">Conditions</button>' +
            '<button type="button" class="pg-explain-detail-tab" data-detail-tab="props" role="tab">Properties</button>' +
            '<button type="button" class="pg-explain-detail-tab" data-detail-tab="src" role="tab">Source</button>' +
            '</div>' +
            '<div class="pg-explain-detail-pane" data-detail-pane="overview" style="display:block;">' + overview + '</div>' +
            '<div class="pg-explain-detail-pane" data-detail-pane="io" style="display:none;">' + ioBody + '</div>' +
            '<div class="pg-explain-detail-pane" data-detail-pane="cond" style="display:none;">' + condBody + '</div>' +
            '<div class="pg-explain-detail-pane" data-detail-pane="props" style="display:none;">' + propsBody + '</div>' +
            '<div class="pg-explain-detail-pane" data-detail-pane="src" style="display:none;">' + srcBody + '</div>' +
            '</div>';
    }

    function renderDepeszTable(flat, idxById, planMeta, maxEx) {
        var rows = flat.map(function (node) {
            var ex = node.exclusive_time != null ? Number(node.exclusive_time) : 0;
            var inc = node.inclusive_time != null ? node.inclusive_time : node.actual_total_time;
            var heat = heatStyleExclusive(ex, maxEx);
            var planR = node.plan_rows;
            var actR = node.actual_rows;
            var rc = rowsCrossCol(planR, actR);
            var buf = bufferSummary(node);
            var nodeTxt = esc(node.node_type || '');
            if (node.relation_name) nodeTxt += ' → ' + esc(node.relation_name);
            if (node.alias) nodeTxt += ' ' + esc(node.alias);
            var k = nodeUIKey(node);
            return '<tr data-node-key="' + esc(k) + '" class="pg-explain-dpez-row" style="cursor:pointer;">' +
                '<td>' + esc(idxById[k]) + '</td>' +
                '<td style="' + esc(heat) + '">' + esc(formatMsMaybe(ex)) + '</td>' +
                '<td>' + esc(formatMsMaybe(inc)) + '</td>' +
                '<td>' + esc(rc) + '</td>' +
                '<td class="text-end">' + (actR != null ? esc(actR) : '—') + '</td>' +
                '<td class="text-end">' + (planR != null ? esc(planR) : '—') + '</td>' +
                '<td class="text-end">' + (node.actual_loops != null ? esc(node.actual_loops) : '—') + '</td>' +
                '<td class="text-muted pg-explain-dpez-buf">' + esc(buf) + '</td>' +
                '<td class="pg-explain-dpez-node">' + nodeTxt + '</td></tr>';
        }).join('');

        var foot = '';
        if (planMeta && (planMeta.planning_time_ms != null || planMeta.execution_time_ms != null)) {
            foot = '<tfoot><tr><td colspan="9" class="text-muted pg-explain-dpez-foot">' +
                'Planning: ' + esc(formatMsMaybe(planMeta.planning_time_ms)) +
                ' · Execution: ' + esc(formatMsMaybe(planMeta.execution_time_ms)) +
                '</td></tr></tfoot>';
        }

        return '<div class="pg-explain-dpez-wrap"><table class="pg-explain-dpez-table"><thead><tr>' +
            '<th>#</th><th title="Time spent in this node (exclusive)">Exclusive</th><th title="Total time including children">Inclusive</th>' +
            '<th>Rows ×</th><th class="text-end">Act. rows</th><th class="text-end">Est. rows</th><th class="text-end">Loops</th><th>Buffers</th><th>Node</th>' +
            '</tr></thead><tbody>' + rows + '</tbody>' + foot + '</table></div>';
    }

    function initPlanPanels(planRoot, analyzerFindings, planMeta) {
        var treeHost = document.getElementById('pgExplainMapTreeHost');
        var tableHost = document.getElementById('pgExplainDepeszHost');
        if (!treeHost || !tableHost) return;

        if (!planRoot) {
            treeHost.innerHTML = '<p class="text-muted" style="padding:0.5rem;">No structured plan tree. Paste <code>EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)</code> output for the card map and metrics table.</p>';
            tableHost.innerHTML = '<p class="text-muted" style="padding:0.5rem;">Metrics table requires a JSON plan.</p>';
            return;
        }

        planRoot = normalizePgExplainObject(planRoot);
        hydratePlanTree(planRoot);

        var flat = [];
        flattenPlanRows(planRoot, flat);
        var idxById = {};
        flat.forEach(function (n, i) {
            idxById[nodeUIKey(n)] = i + 1;
        });
        var fnByNode = findingsByNode(analyzerFindings || []);
        var maxEx = 0;
        flat.forEach(function (n) {
            var ex = n.exclusive_time != null ? Number(n.exclusive_time) : 0;
            if (ex > maxEx) maxEx = ex;
        });

        treeHost.innerHTML = renderHorizontalPlanGraph(planRoot, idxById, fnByNode);
        tableHost.innerHTML = renderDepeszTable(flat, idxById, planMeta || {}, maxEx);

        var selectedId = flat.length ? flat[0].id : null;
        var selectedKey = flat.length ? nodeUIKey(flat[0]) : null;

        function closeNodeModal() {
            var host = document.getElementById('pgExplainNodeModalHost');
            if (host) host.innerHTML = '';
        }

        function openNodeModal(nodeKey) {
            var node = null;
            var idx = 0;
            for (var i = 0; i < flat.length; i++) {
                if (String(nodeUIKey(flat[i])) === String(nodeKey)) {
                    node = flat[i];
                    idx = idxById[nodeUIKey(flat[i])] || i + 1;
                    break;
                }
            }
            if (!node) return;
            var plannerFindings = (node && typeof node.id === 'number' && node.id > 0) ? (fnByNode[node.id] || []) : [];
            var host = document.getElementById('pgExplainNodeModalHost');
            if (!host) return;
            host.innerHTML =
                '<div class="pg-explain-modal-backdrop" data-modal-close="1">' +
                '<div class="pg-explain-modal" role="dialog" aria-modal="true">' +
                '<div class="pg-explain-modal-hd">' +
                '<div class="pg-explain-modal-title">' + esc(node.node_type || 'Plan node') + (node.relation_name ? ' <span class="text-muted">· ' + esc(node.relation_name) + '</span>' : '') + '</div>' +
                '<button type="button" class="pg-explain-modal-x" aria-label="Close" data-modal-close="1">×</button>' +
                '</div>' +
                '<div class="pg-explain-modal-body"><div class="pg-explain-modal-content">' + renderDetailPanel(node, idx, plannerFindings) + '</div></div>' +
                '</div></div>';

            var backdrop = host.querySelector('.pg-explain-modal-backdrop');
            if (backdrop) {
                backdrop.addEventListener('click', function (ev) {
                    if (ev.target && ev.target.getAttribute('data-modal-close') === '1') closeNodeModal();
                });
            }
            var x = host.querySelector('.pg-explain-modal-x');
            if (x) x.addEventListener('click', closeNodeModal);
            // enable tab click handlers inside modal
            var modalRoot = host.querySelector('.pg-explain-modal-body');
            if (modalRoot) bindDetailTabClicks(modalRoot);
            window.addEventListener('keydown', function escClose(e) {
                if (e.key === 'Escape') {
                    closeNodeModal();
                    window.removeEventListener('keydown', escClose);
                }
            });
        }

        function selectNodeKey(nodeKey) {
            selectedKey = nodeKey;
            treeHost.querySelectorAll('.pg-explain-op').forEach(function (c) {
                c.classList.toggle('pg-explain-op--selected', String(c.getAttribute('data-node-key')) === String(nodeKey));
            });
            tableHost.querySelectorAll('.pg-explain-dpez-row').forEach(function (tr) {
                tr.classList.toggle('pg-explain-dpez-row--selected', String(tr.getAttribute('data-node-key')) === String(nodeKey));
            });
        }

        function showTooltip(ev, nodeKey) {
            var tip = treeHost.querySelector('#pgExplainGraphTooltip');
            if (!tip) return;
            var node = null;
            for (var i = 0; i < flat.length; i++) {
                if (String(nodeUIKey(flat[i])) === String(nodeKey)) { node = flat[i]; break; }
            }
            if (!node) return;
            var plannerId = (typeof node.id === 'number' && node.id > 0) ? node.id : null;
            var fns = plannerId != null ? (fnByNode[plannerId] || []) : [];
            var head = '<div class="pg-explain-tip-title"><strong>' + esc(node.node_type || '?') + '</strong>' + (node.relation_name ? ' <span class="text-muted">· ' + esc(node.relation_name) + '</span>' : '') + '</div>';
            var body = '<div class="pg-explain-tip-grid">' +
                '<div><span class="text-muted">Exclusive</span><br/><strong>' + esc(formatMsMaybe(node.exclusive_time)) + '</strong></div>' +
                '<div><span class="text-muted">Inclusive</span><br/><strong>' + esc(formatMsMaybe(node.inclusive_time != null ? node.inclusive_time : node.actual_total_time)) + '</strong></div>' +
                '<div><span class="text-muted">Act rows</span><br/><strong>' + esc(node.actual_rows != null ? node.actual_rows : '—') + '</strong></div>' +
                '<div><span class="text-muted">Est rows</span><br/><strong>' + esc(node.plan_rows != null ? node.plan_rows : '—') + '</strong></div>' +
                '</div>';
            var extra = '';
            if (node.index_name) extra += '<div class="pg-explain-tip-filter"><span class="text-muted">Index</span><br/>' + esc(String(node.index_name)) + '</div>';
            var fx = fns.length ? '<div class="pg-explain-tip-findings">' + fns.slice(0, 2).map(function (f) {
                return '<div class="pg-explain-tip-f"><span class="badge ' + severityClass(f.severity) + '">' + esc(f.severity) + '</span> ' + esc(f.type || '') + '</div>';
            }).join('') + '</div>' : '';
            var filter = node.filter ? '<div class="pg-explain-tip-filter"><span class="text-muted">Filter</span><br/>' + esc(String(node.filter).slice(0, 160)) + '</div>' : '';
            tip.innerHTML = head + body + extra + fx + filter;
            tip.style.display = 'block';
            var rect = treeHost.getBoundingClientRect();
            var x = ev.clientX - rect.left + treeHost.scrollLeft + 14;
            var y = ev.clientY - rect.top + treeHost.scrollTop + 14;
            tip.style.left = x + 'px';
            tip.style.top = y + 'px';
        }

        function hideTooltip() {
            var tip = treeHost.querySelector('#pgExplainGraphTooltip');
            if (!tip) return;
            tip.style.display = 'none';
        }

        treeHost.querySelectorAll('.pg-explain-op').forEach(function (op) {
            op.addEventListener('click', function () {
                var k = op.getAttribute('data-node-key');
                selectNodeKey(k);
                openNodeModal(k);
            });
            op.addEventListener('keydown', function (ev) {
                if (ev.key === 'Enter' || ev.key === ' ') {
                    ev.preventDefault();
                    var k2 = op.getAttribute('data-node-key');
                    selectNodeKey(k2);
                    openNodeModal(k2);
                }
            });
            op.addEventListener('mousemove', function (ev) {
                showTooltip(ev, op.getAttribute('data-node-key'));
            });
            op.addEventListener('mouseenter', function (ev) {
                showTooltip(ev, op.getAttribute('data-node-key'));
                var pid = op.getAttribute('data-node-key');
                treeHost.querySelectorAll('.pg-explain-graph-edge').forEach(function (p) {
                    var from = p.getAttribute('data-edge-from');
                    var to = p.getAttribute('data-edge-to');
                    var isAdj = String(from) === String(pid) || String(to) === String(pid);
                    p.classList.toggle('pg-explain-graph-edge--hl', isAdj);
                    p.classList.toggle('pg-explain-graph-edge--dim', !isAdj);
                });
            });
            op.addEventListener('mouseleave', function () {
                hideTooltip();
                treeHost.querySelectorAll('.pg-explain-graph-edge').forEach(function (p) {
                    p.classList.remove('pg-explain-graph-edge--hl');
                    p.classList.remove('pg-explain-graph-edge--dim');
                });
            });
        });

        tableHost.querySelectorAll('.pg-explain-dpez-row').forEach(function (tr) {
            tr.addEventListener('click', function () {
                selectNodeKey(tr.getAttribute('data-node-key'));
            });
        });

        if (selectedKey != null) selectNodeKey(selectedKey);
    }

    function bindDetailTabClicks(detailHost) {
        detailHost.querySelectorAll('.pg-explain-detail-tab').forEach(function (tab) {
            tab.addEventListener('click', function () {
                var id = tab.getAttribute('data-detail-tab');
                detailHost.querySelectorAll('.pg-explain-detail-tab').forEach(function (x) {
                    x.classList.toggle('pg-explain-detail-tab--active', x === tab);
                });
                detailHost.querySelectorAll('.pg-explain-detail-pane').forEach(function (p) {
                    p.style.display = p.getAttribute('data-detail-pane') === id ? 'block' : 'none';
                });
            });
        });
    }

    function sqlInsightsPanel(sqlCtx, fullSql) {
        var sqlBlock = '';
        if (fullSql && fullSql.trim()) {
            sqlBlock = '<div class="mt-2"><strong class="pg-explain-field-label">Your SQL</strong><pre class="pg-explain-full-sql">' + esc(fullSql) + '</pre></div>';
        }
        var heur = (sqlCtx && sqlCtx.heuristic_insights) ? sqlCtx.heuristic_insights : [];
        var heurHtml = '';
        if (heur.length) {
            heurHtml = '<h3 class="pg-explain-field-label mt-2" style="font-size:0.88rem;">Plan-driven hints (JSON)</h3>' +
                '<p class="text-muted" style="font-size:0.72rem;">From the execution plan tree (not the database rules engine).</p>' +
                heur.map(function (h) {
                    var sev = h.severity || 'info';
                    return '<div class="glass-panel pg-explain-sql-card mb-2" style="padding:0.65rem 0.85rem;">' +
                        '<div class="pg-explain-sql-card-hd"><span class="badge ' + severityClass(sev) + '">' + esc(sev) + '</span> ' +
                        '<strong>' + esc(h.code || '') + '</strong> · ' + esc(h.title || '') + '</div>' +
                        (h.message ? '<div class="mt-1" style="font-size:0.82rem;">' + esc(h.message) + '</div>' : '') +
                        (h.suggested_index_sql ? '<div class="mt-2"><strong style="font-size:0.78rem;">Suggested index</strong><pre class="pg-explain-ddl">' + esc(h.suggested_index_sql) + '</pre></div>' : '') +
                        (h.rewrite_hint ? '<div class="mt-2 text-muted" style="font-size:0.78rem;"><strong>Rewrite:</strong> ' + esc(h.rewrite_hint) + '</div>' : '') +
                        '</div>';
                }).join('');
        }
        if (!sqlCtx || (!sqlCtx.findings || !sqlCtx.findings.length)) {
            if (!heur.length) {
                return sqlBlock + '<p class="text-muted mt-2">No SQL-linked or plan-driven insights yet. Add SQL text and run <strong>Analyze plan</strong>.</p>';
            }
            var disc0 = '<div class="alert alert-info pg-explain-disclaimer" style="font-size:0.76rem;">' + esc(sqlCtx.disclaimer || '') + '</div>';
            return disc0 + heurHtml + sqlBlock;
        }
        var disc = '<div class="alert alert-info pg-explain-disclaimer" style="font-size:0.76rem;">' + esc(sqlCtx.disclaimer || '') + '</div>';
        var cards = sqlCtx.findings.map(function (c) {
            var head = '<div class="pg-explain-sql-card-hd"><span class="badge ' + severityClass(c.severity) + '">' + esc(c.severity) + '</span> ' +
                '<strong>' + esc(c.finding_type || '') + '</strong>';
            if (c.planner_node_type) head += ' <span class="text-muted" style="font-size:0.72rem;">· ' + esc(c.planner_node_type) + '</span>';
            head += '</div>';
            var msg = c.message ? '<div class="mt-1" style="font-size:0.82rem;">' + esc(c.message) + '</div>' : '';
            var loc = '';
            if (c.sql_excerpt) {
                loc = '<div class="mt-2"><span class="text-muted" style="font-size:0.72rem;">Matched SQL (line ' + esc(c.line_start) + ', confidence: ' + esc(c.match_confidence || 'n/a') + ')</span>' +
                    '<pre class="pg-explain-sql-snippet">' + esc(c.sql_excerpt) + '</pre></div>';
            }
            var ddl = '';
            if (c.suggested_index_sql) {
                ddl = '<div class="mt-2"><strong style="font-size:0.78rem;">Suggested index</strong><pre class="pg-explain-ddl">' + esc(c.suggested_index_sql) + '</pre></div>';
            }
            var rh = '';
            if (c.rewrite_hint) {
                rh = '<div class="mt-2 text-muted" style="font-size:0.78rem;"><strong>Focus / rewrite:</strong> ' + esc(c.rewrite_hint) + '</div>';
            }
            return '<div class="glass-panel pg-explain-sql-card mb-2" style="padding:0.65rem 0.85rem;">' + head + msg + loc + ddl + rh + '</div>';
        }).join('');
        return disc + heurHtml + sqlBlock + '<div class="mt-3"><h3 class="pg-explain-field-label" style="font-size:0.88rem;">Analyzer-linked findings</h3>' + cards + '</div>';
    }

    function tabBar(mode, showSqlTab, hasMetrics) {
        var tabs;
        if (mode === 'optimize') {
            tabs = [
                { id: 'report', label: 'Report', icon: 'fa-file-lines' },
                { id: 'map', label: 'Plan map', icon: 'fa-border-all' },
                ...(hasMetrics ? [{ id: 'table', label: 'Metrics', icon: 'fa-table' }] : [])
            ];
        } else if (mode === 'index') {
            tabs = [
                { id: 'index', label: 'Index & rewrites', icon: 'fa-compass-drafting' },
                { id: 'map', label: 'Plan map', icon: 'fa-border-all' },
                ...(hasMetrics ? [{ id: 'table', label: 'Metrics', icon: 'fa-table' }] : [])
            ];
        } else {
            tabs = [
                { id: 'findings', label: 'Findings', icon: 'fa-triangle-exclamation' },
                { id: 'map', label: 'Plan map', icon: 'fa-border-all' },
                ...(hasMetrics ? [{ id: 'table', label: 'Metrics', icon: 'fa-table' }] : [])
            ];
        }
        if (showSqlTab) {
            tabs.push({ id: 'sql', label: 'SQL insights', icon: 'fa-code' });
        }
        return '<div class="pg-explain-tab-bar" role="tablist">' + tabs.map(function (t, i) {
            var active = i === 0 ? ' pg-explain-tab--active' : '';
            return '<button type="button" class="pg-explain-tab' + active + '" data-pg-tab="' + esc(t.id) + '" role="tab">' +
                '<i class="fa-solid ' + esc(t.icon) + '"></i> ' + esc(t.label) + '</button>';
        }).join('') + '</div>';
    }

    function switchPgExplainTab(tabId) {
        var root = document.getElementById('pgExplainResultsInner');
        if (!root) return;
        root.querySelectorAll('.pg-explain-tab').forEach(function (b) {
            b.classList.toggle('pg-explain-tab--active', b.getAttribute('data-pg-tab') === tabId);
        });
        root.querySelectorAll('.pg-explain-tab-panel').forEach(function (p) {
            p.style.display = p.getAttribute('data-panel') === tabId ? 'block' : 'none';
        });
    }

    function mountResultsShell(mode, innerFindingsOrReport, planRoot, sqlCtx, fullSql, vizOpts) {
        vizOpts = vizOpts || {};
        var hasSql = !!(fullSql && String(fullSql).trim().length);
        var hasHeur = !!(sqlCtx && sqlCtx.heuristic_insights && sqlCtx.heuristic_insights.length);
        var showSqlTab = hasSql || hasHeur;
        var hasMetrics = !!(planRoot && typeof planRoot === 'object');
        var bar = tabBar(mode, showSqlTab, hasMetrics);
        var firstPanel = mode === 'optimize' ? 'report' : (mode === 'index' ? 'index' : 'findings');
        var findingsPanel = '<div class="pg-explain-tab-panel" data-panel="' + firstPanel + '">' + innerFindingsOrReport + '</div>';
        var mapPanel = '<div class="pg-explain-tab-panel" data-panel="map" style="display:none;">' +
            '<p class="text-muted pg-explain-map-intro">SSMS-style operator tiles: click an operator to view properties, buffers, filters, and analyzer findings.</p>' +
            '<div class="pg-explain-plan-map-full" id="pgExplainMapTreeHost"></div>' +
            '<div id="pgExplainNodeModalHost"></div>' +
            '</div>';
        var tablePanel = hasMetrics
            ? '<div class="pg-explain-tab-panel" data-panel="table" style="display:none;">' +
                (hasSql
                    ? '<div class="mt-2"><strong class="pg-explain-field-label">Your SQL</strong><pre class="pg-explain-full-sql">' + esc(String(fullSql || '')) + '</pre></div>'
                    : '<p class="text-muted mt-2" style="font-size:0.78rem;">No SQL pasted — the table below uses timings and row estimates from the plan JSON only.</p>') +
                '<div id="pgExplainDepeszHost" class="mt-2"></div></div>'
            : '';
        var sqlPanel = '<div class="pg-explain-tab-panel" data-panel="sql" style="display:none;">' + sqlInsightsPanel(sqlCtx || {}, fullSql) + '</div>';

        host.innerHTML = '<div id="pgExplainResultsInner" class="glass-panel pg-explain-results-inner" style="padding:0.85rem 1rem;">' + bar +
            findingsPanel + mapPanel + tablePanel + (showSqlTab ? sqlPanel : '') + '</div>';
        host.style.display = 'block';
        document.getElementById('pgExplainResultsInner').querySelectorAll('.pg-explain-tab').forEach(function (btn) {
            btn.addEventListener('click', function () {
                switchPgExplainTab(btn.getAttribute('data-pg-tab'));
            });
        });

        initPlanPanels(planRoot, vizOpts.analyzerFindings || [], vizOpts.planMeta || {});
    }

    function renderAnalyze(data) {
        var r = data.result;
        if (!r) {
            host.innerHTML = '<div class="alert alert-warning">No result</div>';
            host.style.display = 'block';
            return;
        }
        var sum = r.summary || {};
        var findings = r.findings || [];
        var fc = sum.findings_count || {};
        var ctxList = (data.sql_context && data.sql_context.findings) ? data.sql_context.findings : [];

        var planRoot = pickAnalyzePlanRoot(data);

        // If summary numbers are missing but the plan tree has costs/times (variant JSON keys), fill gaps for display.
        if (planRoot && typeof planRoot === 'object') {
            hydratePlanTree(planRoot);
            if ((sum.total_cost == null || sum.total_cost === '') && planRoot.total_cost != null && !Number.isNaN(Number(planRoot.total_cost))) {
                sum.total_cost = Number(planRoot.total_cost);
            }
            if ((sum.execution_time_ms == null || sum.execution_time_ms === '') && r.plan && r.plan.execution_time != null && !Number.isNaN(Number(r.plan.execution_time))) {
                sum.execution_time_ms = Number(r.plan.execution_time);
            }
            if ((sum.planning_time_ms == null || sum.planning_time_ms === '') && r.plan && r.plan.planning_time != null && !Number.isNaN(Number(r.plan.planning_time))) {
                sum.planning_time_ms = Number(r.plan.planning_time);
            }
            var fc = 0;
            var scanC = 0;
            var joinC = 0;
            (function walkSummary(n) {
                if (!n || typeof n !== 'object') return;
                fc += 1;
                var t = String(n.node_type || '').toLowerCase();
                if (t.indexOf('scan') >= 0) scanC += 1;
                if (t.indexOf('join') >= 0 || t.indexOf('nested loop') >= 0) joinC += 1;
                (n.plans || []).forEach(walkSummary);
            })(planRoot);
            if (sum.node_count == null || sum.node_count === '') sum.node_count = fc;
            if (sum.scan_count == null || sum.scan_count === '') sum.scan_count = scanC;
            if (sum.join_count == null || sum.join_count === '') sum.join_count = joinC;
        }

        var findingsHtml = findings.map(function (f, i) {
            var ctx = ctxList[i];
            var extra = '';
            if (ctx) {
                if (ctx.sql_excerpt) {
                    extra += '<div class="mt-2"><span class="text-muted" style="font-size:0.72rem;">SQL (line ' + esc(ctx.line_start) + ', ' + esc(ctx.match_confidence || '') + ')</span><pre class="pg-explain-sql-snippet">' + esc(ctx.sql_excerpt) + '</pre></div>';
                }
                if (ctx.suggested_index_sql) {
                    extra += '<div class="mt-2"><strong style="font-size:0.78rem;">Suggested index</strong><pre class="pg-explain-ddl">' + esc(ctx.suggested_index_sql) + '</pre></div>';
                }
                if (ctx.rewrite_hint) {
                    extra += '<div class="mt-2 text-muted" style="font-size:0.78rem;"><strong>Focus:</strong> ' + esc(ctx.rewrite_hint) + '</div>';
                }
            } else if (f.suggestion) {
                extra += '<div class="text-muted mt-1" style="font-size:0.78rem;"><i class="fa-solid fa-lightbulb"></i> ' + esc(f.suggestion) + '</div>';
            }
            return '<div class="glass-panel mb-2" style="padding:0.6rem 0.75rem;">' +
                '<span class="badge ' + severityClass(f.severity) + '">' + esc(f.severity) + '</span> ' +
                '<strong>' + esc(f.type || '') + '</strong>' +
                '<div class="mt-1" style="font-size:0.82rem;">' + esc(f.message || '') + '</div>' + extra + '</div>';
        }).join('');

        var summaryBlock =
            '<div class="chart-card glass-panel mb-3" style="padding:0.75rem;">' +
            '<div class="card-header"><h3 style="font-size:0.9rem; margin:0;">Summary</h3></div>' +
            '<div style="display:grid; grid-template-columns:repeat(auto-fill,minmax(140px,1fr)); gap:0.5rem; font-size:0.78rem;">' +
            '<div><span class="text-muted">Total cost</span><br/><strong>' + esc(sum.total_cost) + '</strong></div>' +
            '<div><span class="text-muted">Execution</span><br/><strong>' + esc(sum.execution_time_ms) + ' ms</strong></div>' +
            '<div><span class="text-muted">Planning</span><br/><strong>' + esc(sum.planning_time_ms) + ' ms</strong></div>' +
            '<div><span class="text-muted">Nodes</span><br/><strong>' + esc(sum.node_count) + '</strong></div>' +
            '<div><span class="text-muted">Scans</span><br/><strong>' + esc(sum.scan_count) + '</strong></div>' +
            '<div><span class="text-muted">Joins</span><br/><strong>' + esc(sum.join_count) + '</strong></div>' +
            '</div>' +
            '<div class="mt-2 text-muted" style="font-size:0.72rem;">Findings: critical ' + esc(fc.critical) + ' · high ' + esc(fc.high) + ' · medium ' + esc(fc.medium) + ' · low ' + esc(fc.low) + ' · info ' + esc(fc.info) + '</div>' +
            '</div>' +
            '<h3 style="font-size:0.9rem;">Findings</h3>' + (findingsHtml || '<p class="text-muted">No findings.</p>');

        var planMeta = {};
        if (r.plan) {
            if (r.plan.planning_time != null) planMeta.planning_time_ms = r.plan.planning_time;
            if (r.plan.execution_time != null) planMeta.execution_time_ms = r.plan.execution_time;
        }
        mountResultsShell('analyze', summaryBlock, planRoot, data.sql_context, (queryEl && queryEl.value) ? queryEl.value : '', {
            analyzerFindings: findings,
            planMeta: planMeta
        });
    }

    function renderOptimize(data) {
        var rep = data.report;
        if (!rep) {
            host.innerHTML = '<div class="alert alert-warning">No report</div>';
            host.style.display = 'block';
            return;
        }
        var ex = rep.executive_summary || {};
        var score = rep.performance_score || {};
        var top = rep.top_issues || [];
        var recs = rep.recommendations || [];
        var conf = rep.confidence_score || {};

        var issuesHtml = top.map(function (issue) {
            return '<div class="glass-panel mb-2" style="padding:0.6rem 0.75rem;">' +
                '<span class="badge ' + severityClass(issue.severity) + '">' + esc(issue.severity) + '</span> ' +
                '<strong>' + esc(issue.title) + '</strong>' +
                '<div class="text-muted mt-1" style="font-size:0.78rem;">' + esc(issue.impact || '') + '</div>' +
                '<div class="mt-1" style="font-size:0.8rem;">' + esc(issue.description || '') + '</div>' +
                '</div>';
        }).join('');

        var recsHtml = recs.map(function (rec, i) {
            return '<div class="glass-panel mb-2" style="padding:0.6rem 0.75rem;">' +
                '<strong>' + esc(rec.issue_title || ('Recommendation ' + (i + 1))) + '</strong>' +
                '<div class="mt-1" style="font-size:0.8rem;">' + esc(rec.action || '') + '</div>' +
                (rec.sql_example ? '<pre class="mt-1 pg-explain-ddl" style="max-height:140px;">' + esc(rec.sql_example) + '</pre>' : '') +
                '<div class="text-muted mt-1" style="font-size:0.72rem;">' + esc(rec.expected_impact || '') + '</div>' +
                '</div>';
        }).join('');

        var sqlNote = '';
        if (data && data.sql_context && data.sql_context.findings && data.sql_context.findings.length) {
            var idxCount = data.sql_context.findings.filter(function (f) { return !!f.suggested_index_sql; }).length;
            var exCount = data.sql_context.findings.filter(function (f) { return !!f.sql_excerpt; }).length;
            sqlNote =
                '<div class="glass-panel mb-3" style="padding:0.65rem 0.75rem; font-size:0.78rem;">' +
                '<strong>SQL-linked hints</strong><div class="text-muted mt-1">Matched excerpts: <strong>' + esc(exCount) + '</strong> · Index DDL suggestions: <strong>' + esc(idxCount) + '</strong></div>' +
                '<div class="text-muted mt-1">Open <strong>SQL insights</strong> to see line matches and concrete <code>CREATE INDEX</code> statements.</div>' +
                '</div>';
        } else if (queryEl && queryEl.value && queryEl.value.trim()) {
            sqlNote =
                '<div class="glass-panel mb-3 text-muted" style="padding:0.65rem 0.75rem; font-size:0.78rem;">' +
                '<strong>SQL provided</strong> — no SQL-linked hints were generated for this plan. If possible, paste a JSON plan from <code>EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)</code> so relation names/filters can be matched more reliably.' +
                '</div>';
        }

        var reportBlock =
            '<div class="chart-card glass-panel mb-3" style="padding:0.75rem;">' +
            '<div class="card-header"><h3 style="font-size:0.9rem; margin:0;">Performance score</h3></div>' +
            '<div style="font-size:1.5rem; font-weight:700;">' + esc(score.score) + ' <span class="badge badge-info" style="font-size:0.65rem;">' + esc(score.label) + '</span></div>' +
            '</div>' +
            sqlNote +
            '<div class="chart-card glass-panel mb-3" style="padding:0.75rem;">' +
            '<div class="card-header"><h3 style="font-size:0.9rem; margin:0;">Executive summary</h3></div>' +
            '<p style="font-size:0.82rem; margin:0.25rem 0;"><strong>Runtime:</strong> ' + esc(ex.total_execution_time) + '</p>' +
            '<p style="font-size:0.82rem; margin:0.25rem 0;"><strong>Primary bottleneck:</strong> ' + esc(ex.primary_bottleneck) + '</p>' +
            '<p style="font-size:0.82rem; margin:0.25rem 0;"><strong>Diagnosis:</strong> ' + esc(ex.overall_diagnosis) + '</p>' +
            '</div>' +
            '<h3 style="font-size:0.9rem;">Top issues</h3>' + (issuesHtml || '<p class="text-muted">None listed.</p>') +
            '<h3 class="mt-3" style="font-size:0.9rem;">Recommendations</h3>' + (recsHtml || '<p class="text-muted">None listed.</p>') +
            '<div class="glass-panel mt-3 text-muted" style="padding:0.6rem; font-size:0.75rem;">' +
            '<strong>Confidence:</strong> ' + esc(conf.level) +
            (conf.factors && conf.factors.length ? ' — ' + esc(conf.factors.join(', ')) : '') +
            '</div>' +
            '<p class="text-muted mt-2" style="font-size:0.72rem;">Open <strong>SQL insights</strong> for line-linked hints tied to analyzer findings (same order as internal rules).</p>';

        var planRoot = data.plan_root || null;
        mountResultsShell('optimize', reportBlock, planRoot, data.sql_context, (queryEl && queryEl.value) ? queryEl.value : '', {
            analyzerFindings: data.analyzer_findings || [],
            planMeta: data.plan_meta || {}
        });
    }

    async function postJSON(url, body) {
        const res = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
        const text = await res.text();
        let json;
        try {
            json = JSON.parse(text);
        } catch (e) {
            throw new Error(text.slice(0, 200) || 'Invalid JSON response');
        }
        if (!res.ok || json.success === false) {
            throw new Error(json.error || ('HTTP ' + res.status));
        }
        return json;
    }

    document.getElementById('pgExplainBtnAnalyze').onclick = async function () {
        showErr('');
        const planRaw = (planEl && planEl.value) ? planEl.value.trim() : '';
        if (!planRaw) {
            showErr('Paste a JSON plan first.');
            return;
        }
        if (!planRaw.startsWith('{') && !planRaw.startsWith('[')) {
            showErr('Only JSON plans are supported. Use EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) in your SQL client.');
            return;
        }
        var planPayload;
        try {
            planPayload = JSON.parse(planRaw);
        } catch (e) {
            showErr('Invalid JSON plan: ' + (e.message || String(e)));
            return;
        }
        host.innerHTML = '<div class="glass-panel p-3"><div class="spinner"></div> Analyzing…</div>';
        host.style.display = 'block';
        try {
            const data = await postJSON('/api/postgres/explain/analyze', {
                query: (queryEl && queryEl.value) ? queryEl.value.trim() : '',
                plan: planPayload
            });
            renderAnalyze(data);
        } catch (e) {
            host.style.display = 'none';
            showErr(e.message || String(e));
        }
    };

    document.getElementById('pgExplainBtnOptimize').onclick = async function () {
        showErr('');
        const planRaw = (planEl && planEl.value) ? planEl.value.trim() : '';
        if (!planRaw) {
            showErr('Paste a JSON plan first.');
            return;
        }
        if (!planRaw.startsWith('{') && !planRaw.startsWith('[')) {
            showErr('Only JSON plans are supported. Use EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON).');
            return;
        }
        var planPayload;
        try {
            planPayload = JSON.parse(planRaw);
        } catch (e) {
            showErr('Invalid JSON plan: ' + (e.message || String(e)));
            return;
        }
        host.innerHTML = '<div class="glass-panel p-3"><div class="spinner"></div> Building report…</div>';
        host.style.display = 'block';
        try {
            const data = await postJSON('/api/postgres/explain/optimize', {
                query: (queryEl && queryEl.value) ? queryEl.value.trim() : '',
                plan: planPayload
            });
            renderOptimize(data);
        } catch (e) {
            host.style.display = 'none';
            showErr(e.message || String(e));
        }
    };

    function statusBadgeClass(status) {
        var s = String(status || '').toLowerCase();
        if (s === 'recommended') return 'badge-success';
        if (s === 'not_recommended') return 'badge-warning';
        if (s === 'error') return 'badge-danger';
        return 'badge-secondary';
    }

    function shouldHideAdvisorReasonLine(s) {
        var t = String(s || '').trim();
        if (!t) return true;
        return /^(heuristic estimate\b|estimated or measured cost improvement|execution plan shape changed|high confidence recommendation)/i.test(t);
    }

    /** Index advisor: DDL + non-boilerplate reasoning only (no confidence score / evidence grid). */
    function renderIndexRecommendationCard(title, rec) {
        if (!rec) return '';
        var reasons = (rec.reasoning && rec.reasoning.length)
            ? rec.reasoning.filter(function (x) { return !shouldHideAdvisorReasonLine(x); })
            : [];
        var reasonsHtml = reasons.length
            ? '<ul class="mt-1 mb-0" style="font-size:0.78rem;">' + reasons.map(function (x) {
                return '<li>' + esc(x) + '</li>';
            }).join('') + '</ul>'
            : '';
        return '<div class="glass-panel mb-2" style="padding:0.65rem 0.75rem;">' +
            '<strong style="font-size:0.85rem;">' + esc(title) + '</strong> ' +
            '<span class="text-muted" style="font-size:0.72rem;">· ' + esc(rec.table || '') + ' · ' + esc(rec.index_method || '') + '</span>' +
            reasonsHtml +
            (rec.index_statement ? '<pre class="pg-explain-ddl mt-2" style="max-height:180px;">' + esc(rec.index_statement) + '</pre>' : '') +
            '</div>';
    }

    function renderIndexAdvisor(data) {
        var st = data.recommendation_status || '';
        var dsnNote = '';
        if (data.dsn_note) {
            dsnNote = '<div class="alert alert-info" style="font-size:0.76rem;">' + esc(data.dsn_note) + '</div>';
        } else if (data.dsn_resolved === false) {
            dsnNote = '<div class="alert alert-info" style="font-size:0.76rem;">No Postgres DSN was resolved from config. Results use heuristics only. Configure one Postgres instance or pass <code>database_dsn</code> via API.</div>';
        }
        var diag = data.diagnostics || {};
        var diagHtml = '<div class="glass-panel mb-2" style="padding:0.55rem 0.7rem; font-size:0.74rem;">' +
            '<strong>Diagnostics</strong> ' +
            '<span class="text-muted">· parsed SQL: ' + esc(diag.ParsedSQL ? 'yes' : 'no') +
            ' · parsed plan: ' + esc(diag.ParsedPlan ? 'yes' : 'no') +
            ' · query supported: ' + esc(diag.QuerySupported ? 'yes' : 'no') +
            ' · HypoPG: ' + esc(diag.HypoPGAvailable ? 'yes' : 'no') +
            '</span></div>';

        var top = renderIndexRecommendationCard('Top recommendation', data.top_recommendation);
        var alts = (data.alternatives || []).map(function (a, i) {
            return renderIndexRecommendationCard('Alternative ' + (i + 1), a);
        }).join('');

        var rewrites = (data.query_rewrites || []).map(function (qr) {
            var rules = (qr.applied_rules || []).join(', ');
            return '<div class="glass-panel mb-2" style="padding:0.65rem 0.75rem;">' +
                '<strong style="font-size:0.82rem;">Query rewrite</strong>' +
                (rules ? '<div class="text-muted mt-1" style="font-size:0.72rem;">Rules: <code>' + esc(rules) + '</code></div>' : '') +
                '<div class="mt-2"><span class="text-muted" style="font-size:0.72rem;">Original</span><pre class="pg-explain-sql-snippet">' + esc(qr.original_query || '') + '</pre></div>' +
                '<div class="mt-1"><span class="text-muted" style="font-size:0.72rem;">Rewritten</span><pre class="pg-explain-ddl">' + esc(qr.rewritten_query || '') + '</pre></div>' +
                '</div>';
        }).join('');

        var rejections = '';
        if (data.rejections && data.rejections.length) {
            rejections = '<h4 class="mt-3" style="font-size:0.85rem;">Rejected candidates</h4>' + data.rejections.map(function (r) {
                return '<div class="glass-panel mb-2 text-muted" style="padding:0.5rem 0.65rem; font-size:0.74rem;">' +
                    esc(r.rejection_reason || '') + '</div>';
            }).join('');
        }

        var head =
            '<div class="chart-card glass-panel mb-3" style="padding:0.75rem;">' +
            '<div class="card-header"><h3 style="font-size:0.9rem; margin:0;">Index advisor</h3></div>' +
            '<p style="font-size:0.82rem; margin:0.35rem 0 0 0;">' +
            '<span class="badge ' + statusBadgeClass(st) + '">' + esc(st) + '</span>' +
            ' <span class="text-muted" style="font-size:0.75rem;">Structured DDL and rewrites from the embedded advisor (review before applying).</span>' +
            '</p></div>' +
            dsnNote + diagHtml;

        var body = (top || '<p class="text-muted">No top index recommendation (thresholds or plan may not warrant it).</p>') +
            alts +
            (rewrites ? '<h4 class="mt-3" style="font-size:0.85rem;">Query rewrites</h4>' + rewrites : '') +
            rejections;

        var planRoot = null;
        try {
            var raw = (planEl && planEl.value) ? planEl.value.trim() : '';
            if (raw && (raw.startsWith('{') || raw.startsWith('['))) {
                planRoot = JSON.parse(raw);
                if (Array.isArray(planRoot) && planRoot.length === 1) planRoot = planRoot[0];
                planRoot = normalizePgExplainObject(planRoot);
                hydratePlanTree(planRoot);
                if (planRoot && planRoot.plan && typeof planRoot.plan === 'object' && !planTreeHasAnyNodeType(planRoot)) {
                    planRoot = planRoot.plan;
                    hydratePlanTree(planRoot);
                }
            }
        } catch (e2) { planRoot = null; }

        mountResultsShell('index', head + body, planRoot, null, (queryEl && queryEl.value) ? queryEl.value : '', {
            analyzerFindings: [],
            planMeta: {}
        });
    }

    document.getElementById('pgExplainBtnIndexAdvisor').onclick = async function () {
        showErr('');
        var q = (queryEl && queryEl.value) ? queryEl.value.trim() : '';
        if (!q) {
            showErr('Index advisor requires SQL text (paste your query above).');
            return;
        }
        const planRaw = (planEl && planEl.value) ? planEl.value.trim() : '';
        if (!planRaw) {
            showErr('Paste a JSON execution plan first (EXPLAIN … FORMAT JSON).');
            return;
        }
        if (!planRaw.startsWith('{') && !planRaw.startsWith('[')) {
            showErr('Index advisor needs JSON plan output, not text. Re-run EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) … in your client.');
            return;
        }
        var planPayload;
        try {
            planPayload = JSON.parse(planRaw);
        } catch (e) {
            showErr('Invalid JSON plan: ' + (e.message || String(e)));
            return;
        }
        host.innerHTML = '<div class="glass-panel p-3"><div class="spinner"></div> Running index advisor…</div>';
        host.style.display = 'block';
        try {
            var body = { query: q, plan: planPayload };
            if (inst && String(inst.type || '').toLowerCase() === 'postgres' && inst.name) {
                body.instance_name = inst.name;
            }
            const data = await postJSON('/api/postgres/explain/index-advisor', body);
            renderIndexAdvisor(data);
        } catch (e) {
            host.style.display = 'none';
            showErr(e.message || String(e));
        }
    };
};
