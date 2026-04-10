# PostgreSQL EXPLAIN Analyzer — Integration plan (SQL Optima / DB Monitor UI)

This document ties together **`todo.md`** (Query tuning workflow), the standalone project at **`pg_explain_analyze/`**, and **`how_to_integrate.md`**. It is the roadmap for embedding plan parsing, findings, and optimization reports into this repository’s Go API and SPA.

---

## 1. Goals (from `todo.md`)

| Target | Meaning |
|--------|--------|
| **EXPLAIN workflow (guardrailed)** | Ability to obtain `EXPLAIN (ANALYZE, BUFFERS, …)` safely: timeouts, size limits, optional role/admin checks. |
| **Optional plan storage** | Persist plans for before/after comparison (Timescale or Postgres table). |
| **UX** | From **Query Performance** (`pg_stat_statements`), open a flow: paste or fetch plan → **Analyze** → **Optimize** (report). |

The **`pg_explain_analyze`** library covers **parsing + static analysis + optimization report generation** on **already captured** plan text or JSON. It does **not** connect to a database by itself.

---

## 2. What `pg_explain_analyze` provides today

| Layer | Packages | Role |
|-------|-----------|------|
| **Types** | `types` | `Plan`, `AnalysisResult`, `OptimizationReport`, `AnalyzerConfig`, findings, summaries. |
| **Parser** | `parser` | `ParseText`, `ParseJSON` for PostgreSQL EXPLAIN output. |
| **Analyzer** | `analyzer` | `New()`, `NewWithConfig()`, `AnalyzeFromText`, `AnalyzeFromJSON`, `Analyze(*Plan)`. |
| **Optimization** | `analyzer` (`NewOptimizationAnalyzer`, `GenerateOptimizationReport`) | Rich report on top of findings. |
| **Compare** | `comparator` | Diff two plans (used by reference server **`/api/v1/compare`**). |

Reference HTTP implementation: **`pg_explain_analyze/api/server.go`** (analyze, optimize, compare, parse, health).

---

## 3. Gap analysis vs `how_to_integrate.md`

| Topic | In guide? | In code? | Action |
|-------|-----------|----------|--------|
| `go get` module path | `github.com/yourorg/pg_explain_analyze` | Same placeholder | Use **`replace`** in monorepo (see §4) or publish/rename module. |
| Analyze / optimize handlers | Yes | Yes (`api/server.go`) | Port handlers into `sql_monitoring_UI` router (§5). |
| **`/api/v1/compare`** | **Was missing** | Yes | Documented in updated `how_to_integrate.md`. |
| **`/api/v1/parse`** | **Was missing** | Yes | Same. |
| `NewWithConfig` / `DefaultConfig` | Yes | Yes (`types.DefaultConfig`) | Expose optional config via env or YAML later. |
| Error JSON `success` field | Example used ad-hoc | Was string `"false"` | **Fixed** in reference server: boolean `false` + `Content-Type`. |
| `go test ./...` | Documented | Failed (duplicate `main` in `test/`) | **Fixed**: `//go:build ignore` on duplicate sample. |
| Execute EXPLAIN on live DB | Mentioned in “Common use cases” only | Not in library | **Phase 2** in this repo (§6). |

---

## 4. Dependency wiring (recommended for this monorepo)

**Option A — `replace` (fastest for local development)**

In `sql_monitoring_UI/backend/go.mod`:

```go
require github.com/yourorg/pg_explain_analyze v0.0.0

replace github.com/yourorg/pg_explain_analyze => ../../pg_explain_analyze
```

Adjust the relative path if your checkout layout differs.

**Option B — Vendor / copy**  
Copy `analyzer`, `parser`, `types`, `comparator` under `internal/third_party/…` and rewrite imports. Higher maintenance; avoid unless policy requires it.

**Option C — Publish module**  
Replace `github.com/yourorg/pg_explain_analyze` with a real module path and `go get` from Git.

---

## 5. Backend integration phases

### Phase 5a — Stateless plan API (low risk)

Add handlers (new file e.g. `internal/api/handlers/pg_explain.go` or methods on `PostgresHandlers`) that mirror the reference server:

| Method | Path (proposal) | Body | Response |
|--------|------------------|------|----------|
| POST | `/api/postgres/explain/analyze` | `{ "query"?, "plan": string \| object }` | `{ "success", "result" }` |
| POST | `/api/postgres/explain/optimize` | same | `{ "success", "report" }` |
| POST | `/api/postgres/explain/compare` | `{ "plan1", "plan2" }` | `{ "success", "diff" }` |
| POST | `/api/postgres/explain/parse` | `{ "plan": string \| object }` | `{ "success", "plan" }` |

**Implementation notes**

- Reuse logic from `pg_explain_analyze/api/server.go` (`parsePlanInput`, string vs `map[string]interface{}` branches).
- Register on **`publicAPI`** only if you accept unauthenticated plan uploads (risk: large payloads). Prefer **`protectedAPI`** (JWT) for anything beyond internal dashboards.
- **Limits**: `http.MaxBytesReader` (e.g. 512 KiB–2 MiB), reject empty `plan`.
- **Singletons**: one `*analyzer.Analyzer` and `*analyzer.OptimizationAnalyzer` per process (same as reference server).

### Phase 5b — Guardrailed `EXPLAIN` execution (matches `todo.md`)

New **protected** endpoint, e.g. `POST /api/postgres/explain/execute`:

- Query params: `instance`, optional `database`.
- Body: `{ "sql": "...", "options": { "analyze": true, "buffers": true, "format": "text|json" } }`.
- Server uses existing Postgres connection pool for that instance.
- **Safety**:
  - Wrap in transaction + `ROLLBACK` after EXPLAIN when using `ANALYZE` (avoid committing side effects).
  - `SET LOCAL statement_timeout = '5s'` (configurable).
  - Allow-list: only `EXPLAIN` prefix after trim/validate; block `;` chaining multiple statements if needed.
  - Optional: require admin JWT or a dedicated `explain` permission.
- Response: raw plan string or JSON → client can call **analyze/optimize** on same payload or server chains internally.

### Phase 5c — Optional persistence

- Table: `postgres_explain_plans` (instance, fingerprint, plan_text, format, created_at, user_id).
- Link from `pg_stat_statements` fingerprint in UI; “compare with saved plan” uses **`/explain/compare`**.

---

## 6. Frontend integration (SPA)

| Step | Detail |
|------|--------|
| **Entry** | On **Query Performance** (`pg-queries`), add “Analyze plan” for a selected statement: modal with textarea (paste) or result from **execute** endpoint when Phase 5b exists. |
| **Calls** | `POST .../explain/analyze` and `POST .../explain/optimize` with `authenticatedFetch`. |
| **Rendering** | Either: (1) port key panels from `pg_explain_analyze/frontend/src` (React) into a small Vite island, or (2) implement minimal vanilla/Chart-less panels (findings list + executive summary) to stay consistent with current stack. |
| **Plan graph** | Optional: load PEV2/reactflow only on this route to avoid bloating global bundle. |

---

## 7. Configuration (analyzer thresholds)

Map **`types.AnalyzerConfig`** to env or `config.yaml` later, e.g.:

- `EXPLAIN_ANALYZER_SEQ_SCAN_ROWS`
- `EXPLAIN_ANALYZER_ROW_MISMATCH_RATIO`
- `EXPLAIN_ANALYZER_NESTED_LOOP_ROWS`
- `EXPLAIN_ANALYZER_WORK_MEM_MB`

Construct `analyzer.NewWithConfig(&types.AnalyzerConfig{...})` when any override is set.

---

## 8. Testing strategy

| Layer | Command |
|-------|---------|
| Library | `cd pg_explain_analyze && go test ./...` |
| New handlers | Table-driven tests: golden EXPLAIN snippets → JSON shape / HTTP 400 on oversize body |
| E2E | Manual: capture real `EXPLAIN (ANALYZE, BUFFERS, VERBOSE, FORMAT JSON)` from a dev cluster |

---

## 9. Rollout order (recommended)

1. Add `replace` + implement **Phase 5a** (analyze + optimize + optional compare) behind JWT.  
2. Ship minimal UI: paste plan → show findings + top recommendations.  
3. Add **Phase 5b** execute endpoint + wire “Get EXPLAIN from server” button.  
4. Optional **Phase 5c** storage + compare workflow.

---

## 10. Files to touch in `sql_monitoring_UI` (checklist)

- [ ] `backend/go.mod` — `require` + `replace`  
- [ ] `internal/api/handlers/pg_explain.go` (new) — handlers + limits  
- [ ] `internal/api/router.go` — register routes on public or protected subrouter  
- [ ] `frontend/js/pages/pg_queries.js` (or dedicated page) — modal / panel  
- [ ] `project_details.md` — document new routes  
- [ ] `todo.md` — check off Query tuning items as you complete phases  

---

## 11. Reference reading

- `pg_explain_analyze/how_to_integrate.md` (usage + API; updated for compare/parse and monorepo notes)  
- `pg_explain_analyze/api/server.go` (canonical handler behavior)  
- `sql_monitoring_UI/todo.md` — **P1 → Query tuning workflow**
