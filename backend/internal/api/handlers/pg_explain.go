package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain"
	"github.com/yourorg/pg_explain_analyze/analyzer"
	"github.com/yourorg/pg_explain_analyze/parser"
	"github.com/yourorg/pg_explain_analyze/types"
)

const maxExplainPlanBodyBytes = 512 * 1024

var (
	pgExplainStdAnalyzer *analyzer.Analyzer
	pgExplainOptAnalyzer *analyzer.OptimizationAnalyzer
)

func init() {
	pgExplainStdAnalyzer = analyzer.New()
	pgExplainOptAnalyzer = analyzer.NewOptimizationAnalyzer()
}

type pgExplainRequest struct {
	Query string `json:"query,omitempty"`
	Plan  any    `json:"plan"`
}

func writeExplainJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func parseExplainPlanInput(input any) (*types.Plan, string, error) {
	switch p := input.(type) {
	case string:
		if len(p) == 0 {
			return nil, "", fmt.Errorf("plan text is empty")
		}
		trim := strings.TrimSpace(p)
		// If the user pasted JSON as a string, unwrap common Postgres FORMAT JSON shape: [ { ... } ]
		if strings.HasPrefix(trim, "[") || strings.HasPrefix(trim, "{") {
			normalized, nerr := normalizeExplainJSONPayload([]byte(trim))
			if nerr == nil {
				pl, err := parser.ParseJSON(string(normalized))
				return pl, string(normalized), err
			}
		}
		pl, err := parser.ParseText(p)
		return pl, p, err
	case map[string]interface{}:
		planJSON, err := json.Marshal(p)
		if err != nil {
			return nil, "", err
		}
		normalized, nerr := normalizeExplainJSONPayload(planJSON)
		if nerr != nil {
			return nil, "", nerr
		}
		s := string(normalized)
		pl, err := parser.ParseJSON(s)
		return pl, s, err
	case []interface{}:
		planJSON, err := json.Marshal(p)
		if err != nil {
			return nil, "", err
		}
		normalized, nerr := normalizeExplainJSONPayload(planJSON)
		if nerr != nil {
			return nil, "", nerr
		}
		s := string(normalized)
		pl, err := parser.ParseJSON(s)
		return pl, s, err
	default:
		return nil, "", fmt.Errorf("plan must be a JSON string, object, or array")
	}
}

// normalizeExplainJSONPayload unwraps common Postgres FORMAT JSON output:
// EXPLAIN (FORMAT JSON) returns a top-level JSON array with a single object element.
func normalizeExplainJSONPayload(raw []byte) ([]byte, error) {
	var asArray []any
	if err := json.Unmarshal(raw, &asArray); err == nil {
		if len(asArray) == 0 {
			return nil, fmt.Errorf("JSON plan array is empty")
		}
		if len(asArray) == 1 {
			return json.Marshal(asArray[0])
		}
		// Some tools may return multiple statements; keep as-is and let parser error clearly.
		return raw, nil
	}
	// Not an array; keep as-is.
	return raw, nil
}

// PgExplainAnalyze parses EXPLAIN output (text or JSON) and returns findings + summary.
func PgExplainAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeExplainJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxExplainPlanBodyBytes)

	var req pgExplainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
		return
	}

	var result *types.AnalysisResult
	var err error
	switch plan := req.Plan.(type) {
	case string:
		result, err = pgExplainStdAnalyzer.AnalyzeFromText(plan)
	case map[string]interface{}:
		planJSON, jerr := json.Marshal(plan)
		if jerr != nil {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": jerr.Error()})
			return
		}
		normalized, nerr := normalizeExplainJSONPayload(planJSON)
		if nerr != nil {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": nerr.Error()})
			return
		}
		result, err = pgExplainStdAnalyzer.AnalyzeFromJSON(string(normalized))
	case []interface{}:
		planJSON, jerr := json.Marshal(plan)
		if jerr != nil {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": jerr.Error()})
			return
		}
		normalized, nerr := normalizeExplainJSONPayload(planJSON)
		if nerr != nil {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": nerr.Error()})
			return
		}
		result, err = pgExplainStdAnalyzer.AnalyzeFromJSON(string(normalized))
	default:
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "plan must be a string, object, or array"})
		return
	}
	if err != nil {
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
		return
	}

	flat := result.FlattenNodes()
	result.Query = req.Query
	if raw, ok := req.Plan.(string); ok {
		result.RawPlan = raw
	} else if req.Plan != nil {
		if b, jerr := json.Marshal(req.Plan); jerr == nil {
			result.RawPlan = string(b)
		}
	}

	g := explain.BuildPlanGraph(result.Plan.Plan)
	resp := map[string]any{
		"success":      true,
		"result":       result,
		"plan_graph":   g,
		"plan_mermaid": explain.MermaidFlowchart(g),
	}
	if strings.TrimSpace(req.Query) != "" {
		resp["sql_context"] = explain.AugmentFindingsWithSQL(req.Query, result.Findings, flat)
	}

	writeExplainJSON(w, http.StatusOK, resp)
}

// PgExplainOptimize returns a structured optimization report for the given plan.
func PgExplainOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeExplainJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxExplainPlanBodyBytes)

	var req pgExplainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
		return
	}

	plan, _, err := parseExplainPlanInput(req.Plan)
	if err != nil {
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
		return
	}
	if req.Query != "" {
		plan.Query = req.Query
	}

	aresult := pgExplainStdAnalyzer.Analyze(plan)
	flat := aresult.FlattenNodes()
	report := pgExplainOptAnalyzer.GenerateOptimizationReport(plan, aresult.Findings)

	g := explain.BuildPlanGraph(plan.Plan)
	resp := map[string]any{
		"success":           true,
		"report":            report,
		"plan_root":         plan.Plan,
		"plan_graph":        g,
		"plan_mermaid":      explain.MermaidFlowchart(g),
		"analyzer_findings": aresult.Findings,
		"plan_meta": map[string]float64{
			"planning_time_ms":  plan.PlanningTime,
			"execution_time_ms": plan.ExecutionTime,
		},
	}
	if strings.TrimSpace(req.Query) != "" {
		resp["sql_context"] = explain.AugmentFindingsWithSQL(req.Query, aresult.Findings, flat)
	}

	writeExplainJSON(w, http.StatusOK, resp)
}
