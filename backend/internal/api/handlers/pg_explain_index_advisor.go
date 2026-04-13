package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/missing_index/advisor"
)

const maxIndexAdvisorBodyBytes = 512 * 1024

var (
	pgIndexAdvisorOnce   sync.Once
	pgIndexAdvisorClient *advisor.Client
)

func pgIndexAdvisorClientSingleton() *advisor.Client {
	pgIndexAdvisorOnce.Do(func() {
		pgIndexAdvisorClient = advisor.New()
	})
	return pgIndexAdvisorClient
}

type pgExplainIndexRequest struct {
	Query        string `json:"query"`
	Plan         any    `json:"plan"`
	DatabaseDSN  string `json:"database_dsn,omitempty"`
	InstanceName string `json:"instance_name,omitempty"`
	QueryParams  []any  `json:"query_params,omitempty"`
	Options      *struct {
		MaxCandidates      int     `json:"max_candidates"`
		MinImprovementPct  float64 `json:"min_improvement_pct"`
		StatementTimeoutMs int     `json:"statement_timeout_ms"`
		IncludeColumns     *bool   `json:"include_columns"`
	} `json:"options,omitempty"`
}

func resolvePostgresDSN(cfg *config.Config, explicitDSN, instanceName string) string {
	dsn := strings.TrimSpace(explicitDSN)
	if dsn != "" {
		return dsn
	}
	name := strings.TrimSpace(instanceName)
	var inst config.Instance
	var ok bool
	if name != "" {
		inst, ok = cfg.PostgresInstanceByName(name)
	} else {
		inst, ok = cfg.DefaultPostgresInstance()
	}
	if !ok {
		return ""
	}
	return postgresURLFromInstance(inst)
}

func postgresURLFromInstance(inst config.Instance) string {
	port := inst.Port
	if port == 0 {
		port = 5432
	}
	sslmode := inst.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	dbname := "postgres"
	if len(inst.Databases) > 0 {
		dbname = inst.Databases[0]
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(inst.User, inst.Password),
		Host:   fmt.Sprintf("%s:%d", inst.Host, port),
		Path:   "/" + dbname,
	}
	q := url.Values{}
	q.Set("sslmode", sslmode)
	u.RawQuery = q.Encode()
	return u.String()
}

// executionPlanMapForAdvisor normalizes pasted EXPLAIN JSON to a single object with a "Plan" key.
func executionPlanMapForAdvisor(plan any) (map[string]any, error) {
	var raw []byte
	var err error
	switch p := plan.(type) {
	case string:
		trim := strings.TrimSpace(p)
		if !strings.HasPrefix(trim, "{") && !strings.HasPrefix(trim, "[") {
			return nil, fmt.Errorf("index advisor needs a JSON execution plan (EXPLAIN … FORMAT JSON), not text")
		}
		raw = []byte(trim)
	case map[string]interface{}:
		raw, err = json.Marshal(p)
		if err != nil {
			return nil, err
		}
	case []interface{}:
		raw, err = json.Marshal(p)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("plan must be JSON object or array")
	}
	normalized, nerr := normalizeExplainJSONPayload(raw)
	if nerr != nil {
		return nil, nerr
	}
	unwrapped, uerr := unwrapExplainJSONPreserveKeys(normalized)
	if uerr != nil {
		return nil, uerr
	}
	var out map[string]any
	if err := json.Unmarshal(unwrapped, &out); err != nil {
		return nil, err
	}
	if out == nil || out["Plan"] == nil {
		return nil, fmt.Errorf("JSON plan must contain a Plan object (use EXPLAIN (FORMAT JSON) output)")
	}
	return out, nil
}

func toAdvisorOptions(o *pgExplainIndexRequest) *advisor.Options {
	if o == nil || o.Options == nil {
		return nil
	}
	return &advisor.Options{
		MaxCandidates:      o.Options.MaxCandidates,
		MinImprovementPct:  o.Options.MinImprovementPct,
		StatementTimeoutMs: o.Options.StatementTimeoutMs,
		IncludeColumns:     o.Options.IncludeColumns,
	}
}

// PgExplainIndexAdvisor runs the pg_missing_index advisor (indexes + query rewrites) on JSON plans.
func PgExplainIndexAdvisor(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeExplainJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "error": "method not allowed"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxIndexAdvisorBodyBytes)

		var req pgExplainIndexRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
			return
		}
		queryText := strings.TrimSpace(req.Query)
		if queryText == "" {
			if m, ok := req.Plan.(map[string]interface{}); ok {
				for _, key := range []string{"query_text", "queryText", "query", "sql_text"} {
					if s, ok := m[key].(string); ok {
						queryText = strings.TrimSpace(s)
						if queryText != "" {
							break
						}
					}
				}
			}
		}
		if queryText == "" {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "query is required (paste SQL or use query_text in the same JSON as execution_plan_json)"})
			return
		}
		req.Query = queryText
		planMap, err := executionPlanMapForAdvisor(req.Plan)
		if err != nil {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
			return
		}

		dsn := resolvePostgresDSN(cfg, req.DatabaseDSN, req.InstanceName)

		payload, err := pgIndexAdvisorClientSingleton().Analyze(context.Background(), dsn, strings.TrimSpace(req.Query), planMap, req.QueryParams, toAdvisorOptions(&req))
		if err != nil {
			writeExplainJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
			return
		}
		payload["success"] = true
		if dsn != "" {
			payload["dsn_resolved"] = true
		} else {
			payload["dsn_resolved"] = false
			payload["dsn_note"] = "No database DSN was resolved; HypoPG and catalog checks were skipped (heuristic scoring only). Set instance_name to a configured Postgres instance or pass database_dsn."
		}
		writeExplainJSON(w, http.StatusOK, payload)
	}
}
